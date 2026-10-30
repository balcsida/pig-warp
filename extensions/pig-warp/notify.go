package pig_warp

// Lifecycle handlers that emit OSC 777 notifications. upstream: rpiv-warp/index.ts.

import (
	"fmt"
	"strings"
	"sync"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

// idleDelay is the pause after agent_end before idle_prompt (index.ts:122); a variable so tests can shorten it.
var idleDelay = 300 * time.Millisecond

// subagentTools maps a subagent tool to the argument naming its agent: pigpen-subagents' `subagent` and tintinweb-subagents' `Agent`.
var subagentTools = map[string]string{"subagent": "agent", "Agent": "subagent_type"}

// runningAgent is a subagent shown in the title, keyed by its tool call or, for a tintinweb background agent, its agent id.
type runningAgent struct{ key, name string }

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
	// turn is true from agent_start to agent_end; suffix is the title after the frame, from the last turn.
	turn   bool
	suffix string
	agents []runningAgent
	// task is pigpen-todo's task in progress, shown before the agents.
	task string
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
		branch, err := ctx.SessionManager().GetBranch(nil)
		if err != nil {
			return nil, err
		}
		n.mu.Lock()
		defer n.mu.Unlock()
		n.task = replayTask(branch)
		if !n.customRun {
			emit(s.sessionStart()) // defensive re-announce
			emit(s.promptSubmit(n.pendingQuery))
		}
		n.turn, n.suffix = true, suffix
		n.refreshSpinner()
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
		n.turn = false
		n.refreshSpinner()
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
		n.refreshSpinner()
		n.stopHeartbeat()
		return nil, nil
	})

	e.OnEvent(sdk.EventToolExecutionStart, func(_ sdk.Context, data map[string]any) (any, error) {
		name, _ := data["toolName"].(string)
		arg, ok := subagentTools[name]
		args, _ := data["args"].(map[string]any)
		if !ok || args["action"] != nil { // list and get run no agent
			return nil, nil
		}
		id, _ := data["toolCallId"].(string)
		agent, _ := args[arg].(string)
		n.mu.Lock()
		defer n.mu.Unlock()
		n.addAgent(id, agent)
		return nil, nil
	})

	e.OnEvent(sdk.EventToolExecutionEnd, func(ctx sdk.Context, data map[string]any) (any, error) {
		name, _ := data["toolName"].(string)
		id, _ := data["toolCallId"].(string)
		if _, ok := subagentTools[name]; ok {
			n.mu.Lock()
			defer n.mu.Unlock()
			n.removeAgent(id)
			return nil, nil
		}
		if name == "todo" {
			result, _ := data["result"].(map[string]any)
			if task, ok := todoTask(result["details"]); ok {
				n.mu.Lock()
				defer n.mu.Unlock()
				n.task = task
				n.refreshSpinner()
			}
			return nil, nil
		}
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
		n.suffix = suffix
		n.refreshSpinner()
		n.startHeartbeat(s)
		return nil, nil
	})

	e.OnEvent(sdk.EventSessionShutdown, func(sdk.Context, map[string]any) (any, error) {
		n.mu.Lock()
		defer n.mu.Unlock()
		n.cancelIdle()
		n.stopHeartbeat()
		n.pendingQuery, n.customRun, n.pending = "", false, nil
		n.turn, n.agents, n.task = false, nil, ""
		n.spinner.end()
		return nil, nil
	})

	// tintinweb-subagents runs agents in the background past the turn; its bus events bracket them (created is background only).
	bus := e.Events()
	_, _ = bus.On("subagents:created", func(_ sdk.Context, data any) error {
		m, _ := data.(map[string]any)
		id, _ := m["id"].(string)
		typ, _ := m["type"].(string)
		n.mu.Lock()
		defer n.mu.Unlock()
		n.addAgent(id, typ)
		return nil
	})
	settled := func(_ sdk.Context, data any) error {
		m, _ := data.(map[string]any)
		id, _ := m["id"].(string)
		n.mu.Lock()
		defer n.mu.Unlock()
		n.removeAgent(id)
		return nil
	}
	_, _ = bus.On("subagents:completed", settled)
	_, _ = bus.On("subagents:failed", settled)
}

// callers hold n.mu
func (n *notifier) addAgent(key, name string) {
	if name = jsTrim(name); name == "" {
		name = "agent"
	}
	n.agents = append(n.agents, runningAgent{key, name})
	n.refreshSpinner()
}

// callers hold n.mu
func (n *notifier) removeAgent(key string) {
	for i, a := range n.agents {
		if a.key == key {
			n.agents = append(n.agents[:i], n.agents[i+1:]...)
			n.refreshSpinner()
			return
		}
	}
}

// refreshSpinner runs the spinner during a turn that waits on no question, and while any subagent runs. The title
// names the task in progress and the first subagent, counting the rest: `⠋ Running tests · scout +2 - <repo>`.
// callers hold n.mu
func (n *notifier) refreshSpinner() {
	var parts []string
	if n.task != "" {
		parts = append(parts, n.task)
	}
	switch len(n.agents) {
	case 0:
	case 1:
		parts = append(parts, n.agents[0].name)
	default:
		parts = append(parts, fmt.Sprintf("%s +%d", n.agents[0].name, len(n.agents)-1))
	}
	label := ""
	if len(parts) > 0 {
		label = " " + strings.Join(parts, " · ")
	}
	if (n.turn && len(n.pending) == 0) || len(n.agents) > 0 {
		n.spinner.show(label + n.suffix)
	} else {
		n.spinner.end()
	}
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

// todoTask reads pigpen-todo's snapshot (every `todo` result carries the whole list in details.tasks): the first task in
// progress, by its activeForm or else its subject. ok is false when details is no snapshot.
func todoTask(details any) (task string, ok bool) {
	m, _ := details.(map[string]any)
	tasks, ok := m["tasks"].([]any)
	if !ok {
		return "", false
	}
	for _, t := range tasks {
		t, _ := t.(map[string]any)
		if t["status"] != "in_progress" {
			continue
		}
		if s, _ := t["activeForm"].(string); jsTrim(s) != "" {
			return jsTrim(s), true
		}
		s, _ := t["subject"].(string)
		return jsTrim(s), true
	}
	return "", true
}

// replayTask is the task in progress of the last `todo` snapshot on the branch, as pigpen-todo rebuilds its list.
func replayTask(branch []map[string]any) string {
	for i := len(branch) - 1; i >= 0; i-- {
		msg, _ := branch[i]["message"].(map[string]any)
		if branch[i]["type"] != "message" || msg["role"] != "toolResult" || msg["toolName"] != "todo" {
			continue
		}
		if task, ok := todoTask(msg["details"]); ok {
			return task
		}
	}
	return ""
}
