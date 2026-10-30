package pig_warp

// Lifecycle handlers that emit OSC 777 notifications. upstream: rpiv-warp/index.ts.

import (
	"sync"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

// idleDelay is the pause after agent_end before idle_prompt (index.ts:122); a variable so tests can shorten it.
var idleDelay = 300 * time.Millisecond

type pendingCall struct {
	id, name string
	input    any
}

type notifier struct {
	blocking  map[string]bool
	heartbeat time.Duration

	mu           sync.Mutex
	pendingQuery string
	// customRun is true while the run was started by sendMessage with triggerTurn, which carries no prompt (index.ts:31).
	customRun bool
	// pending holds blocking-tool calls in arrival order; an ESC-aborted tool never fires tool_execution_end, so agent_end drains them (index.ts:40).
	pending []pendingCall
	idle    *time.Timer
	hbStop  chan struct{}
	spinner spinner
}

func registerNotify(e *sdk.Extension) {
	n := &notifier{}
	var hb int
	n.blocking, hb = loadConfig()
	n.heartbeat = time.Duration(hb) * time.Millisecond

	e.OnEvent(sdk.EventSessionStart, func(ctx sdk.Context, data map[string]any) (any, error) {
		if data["reason"] != "startup" {
			return nil, nil
		}
		s, err := current(ctx)
		if err != nil {
			return nil, err
		}
		emit(s.sessionStart())
		return nil, nil
	})

	e.OnEvent(sdk.EventBeforeAgentStart, func(_ sdk.Context, data map[string]any) (any, error) {
		n.mu.Lock()
		defer n.mu.Unlock()
		if data["messageType"] == "custom" {
			n.customRun = true
			return nil, nil
		}
		n.customRun = false
		n.pendingQuery, _ = data["prompt"].(string)
		return nil, nil
	})

	e.OnEvent(sdk.EventAgentStart, func(ctx sdk.Context, _ map[string]any) (any, error) {
		s, err := current(ctx)
		if err != nil {
			return nil, err
		}
		suffix, err := titleSuffix(ctx)
		if err != nil {
			return nil, err
		}
		n.mu.Lock()
		defer n.mu.Unlock()
		if !n.customRun {
			emit(s.sessionStart()) // defensive re-announce
			emit(s.promptSubmit(n.pendingQuery))
		}
		n.spinner.start(suffix)
		n.cancelIdle()
		n.startHeartbeat(s)
		return nil, nil
	})

	e.OnEvent(sdk.EventAgentEnd, func(ctx sdk.Context, _ map[string]any) (any, error) {
		s, err := current(ctx)
		if err != nil {
			return nil, err
		}
		branch, err := ctx.SessionManager().GetBranch(nil)
		if err != nil {
			return nil, err
		}
		n.mu.Lock()
		defer n.mu.Unlock()
		for _, call := range n.pending {
			emit(s.toolComplete(call.name, call.input))
		}
		n.pending = nil
		emit(s.stop(branch))
		n.spinner.end()
		n.stopHeartbeat()
		n.cancelIdle()
		summary := lastMessageText(branch, "assistant")
		n.idle = time.AfterFunc(idleDelay, func() { emit(s.idlePrompt(summary)) })
		return nil, nil
	})

	e.OnEvent(sdk.EventToolCall, func(ctx sdk.Context, data map[string]any) (any, error) {
		name, _ := data["toolName"].(string)
		if !n.blocking[name] {
			return nil, nil
		}
		s, err := current(ctx)
		if err != nil {
			return nil, err
		}
		id, _ := data["toolCallId"].(string)
		input, _ := data["input"].(map[string]any)
		n.mu.Lock()
		defer n.mu.Unlock()
		n.pending = append(n.pending, pendingCall{id: id, name: name, input: objectOrNil(input)})
		emit(s.questionAsked())
		n.spinner.end()
		n.stopHeartbeat()
		return nil, nil
	})

	e.OnEvent(sdk.EventToolExecutionEnd, func(ctx sdk.Context, data map[string]any) (any, error) {
		name, _ := data["toolName"].(string)
		if !n.blocking[name] {
			return nil, nil
		}
		s, err := current(ctx)
		if err != nil {
			return nil, err
		}
		suffix, err := titleSuffix(ctx)
		if err != nil {
			return nil, err
		}
		id, _ := data["toolCallId"].(string)
		n.mu.Lock()
		defer n.mu.Unlock()
		var input any
		for i, call := range n.pending {
			if call.id == id {
				input = call.input
				n.pending = append(n.pending[:i], n.pending[i+1:]...)
				break
			}
		}
		emit(s.toolComplete(name, input))
		n.spinner.start(suffix)
		n.startHeartbeat(s)
		return nil, nil
	})

	e.OnEvent(sdk.EventSessionShutdown, func(sdk.Context, map[string]any) (any, error) {
		n.mu.Lock()
		defer n.mu.Unlock()
		n.cancelIdle()
		n.stopHeartbeat()
		n.pendingQuery, n.customRun, n.pending = "", false, nil
		n.spinner.end()
		return nil, nil
	})
}

// objectOrNil keeps `tool_input` absent when the input was not an object (index.ts:150), and `{}` when it was empty.
func objectOrNil(m map[string]any) any {
	if m == nil {
		return nil
	}
	return m
}

func current(ctx sdk.Context) (session, error) {
	id, err := ctx.GetSessionID()
	return session{id: id, cwd: ctx.Cwd()}, err
}

// titleSuffix mirrors Pi's tab title: ` - <repo>`, or ` - <session> - <repo>` once named (index.ts:169).
func titleSuffix(ctx sdk.Context) (string, error) {
	name, err := ctx.GetSessionName()
	if err != nil {
		return "", err
	}
	project := " - " + filepathBase(ctx.Cwd())
	if name != nil && jsTrim(*name) != "" {
		return " - " + jsTrim(*name) + project, nil
	}
	return project, nil
}

// callers hold n.mu
func (n *notifier) cancelIdle() {
	if n.idle != nil {
		n.idle.Stop()
		n.idle = nil
	}
}

// startHeartbeat re-announces the running prompt every heartbeat so Warp does not mark the tab idle (index.ts:134).
func (n *notifier) startHeartbeat(s session) {
	n.stopHeartbeat()
	if n.heartbeat <= 0 {
		return
	}
	stop := make(chan struct{})
	n.hbStop = stop
	go func() {
		t := time.NewTicker(n.heartbeat)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				n.mu.Lock()
				query := n.pendingQuery
				n.mu.Unlock()
				emit(s.promptSubmit(query))
			}
		}
	}()
}

func (n *notifier) stopHeartbeat() {
	if n.hbStop != nil {
		close(n.hbStop)
		n.hbStop = nil
	}
}
