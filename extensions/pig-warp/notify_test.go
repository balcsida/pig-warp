package pig_warp_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	pigwarp "github.com/balcsida/pig-warp/extensions/pig-warp"
)

func startNotifyRig(t *testing.T, s *sessionState) (*Host, *writes) {
	t.Helper()
	w := captureWrites(t)
	h := StartHost(t, pigwarp.Extension(), HostOptions{Cwd: "/home/me/proj", OnCallValue: s.answer})
	// A real host ends every session with session_shutdown, which stops the timers; without it a timer from one test writes into the next.
	t.Cleanup(func() { h.FireAll("session_shutdown", nil) })
	return h, w
}

func TestRegistersNothingOutsideWarp(t *testing.T) {
	warpEnv(t)
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	h := StartHost(t, pigwarp.Extension(), HostOptions{})
	for _, ev := range []string{"before_agent_start", "agent_start", "agent_end", "tool_call", "tool_execution_end"} {
		if h.Registered(ev) {
			t.Errorf("%s registered outside Warp", ev)
		}
	}
	if !h.cmds["branch"] {
		t.Error("/branch should still be registered")
	}
}

func TestRegistersNothingOnBrokenBuild(t *testing.T) {
	warpEnv(t)
	t.Setenv("WARP_CLIENT_VERSION", "v0.2026.3.25.8.24.stable_5")
	h := StartHost(t, pigwarp.Extension(), HostOptions{})
	if h.Registered("agent_start") {
		t.Error("agent_start registered on a broken build")
	}
}

func TestSessionStartEmitsOnlyOnStartup(t *testing.T) {
	warpEnv(t)
	h, w := startNotifyRig(t, &sessionState{})
	for _, reason := range []string{"new", "resume", "fork", "reload"} {
		h.FireAll("session_start", map[string]any{"reason": reason})
	}
	if got := w.events(); len(got) != 0 {
		t.Fatalf("emitted on non-startup reasons: %v", got)
	}
	h.FireAll("session_start", map[string]any{"reason": "startup"})
	if got := w.events(); !reflect.DeepEqual(got, []string{"session_start"}) {
		t.Fatalf("events = %v", got)
	}
	p := w.payloads()[0]
	if p["session_id"] != "sess-1" || p["cwd"] != "/home/me/proj" || p["project"] != "proj" || p["agent"] != "pi" || p["v"] != 1.0 {
		t.Errorf("payload = %v", p)
	}
}

func TestTurnLifecycle(t *testing.T) {
	warpEnv(t)
	*pigwarp.IdleDelay = 20 * time.Millisecond
	*pigwarp.FrameInterval = 5 * time.Millisecond
	s := &sessionState{name: ptr("  ")}
	h, w := startNotifyRig(t, s)

	h.Fire("before_agent_start", map[string]any{"systemPromptOptions": map[string]any{}, "messageType": "user", "prompt": "<skill name=\"commit\" location=\"/s\">\nb\n</skill>\n\nSkill input: it"})
	h.Fire("agent_start", nil)
	if got := w.events(); !reflect.DeepEqual(got, []string{"session_start", "prompt_submit"}) {
		t.Fatalf("agent_start events = %v", got)
	}
	if q := w.payloads()[1]["query"]; q != "/skill:commit it" {
		t.Errorf("query = %v", q)
	}
	// The spinner pushed the title and animates with the unnamed suffix (a blank name is unnamed).
	waitFor(t, "a spinner frame", func() bool {
		for _, x := range w.all() {
			if x == "\x1b]0;⠋ - proj\a" {
				return true
			}
		}
		return false
	})
	if all := w.all(); all[2] != pigwarp.PushTitleStack {
		t.Errorf("expected title push after the payloads, got %q", all[2])
	}

	s.mu.Lock()
	s.branch = []map[string]any{
		{"type": "message", "message": map[string]any{"role": "user", "content": "do it"}},
		{"type": "message", "message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": "done"}}}},
	}
	s.mu.Unlock()
	h.Fire("agent_end", nil)
	stop := w.payloads()[len(w.payloads())-1]
	if stop["event"] != "stop" || stop["query"] != "do it" || stop["response"] != "done" {
		t.Errorf("stop = %v", stop)
	}
	all := w.all()
	if all[len(all)-1] != pigwarp.PopTitleStack {
		t.Errorf("expected title pop after stop, got %q", all[len(all)-1])
	}
	waitFor(t, "idle_prompt", func() bool { return w.count("idle_prompt") == 1 })
	idle := w.payloads()[len(w.payloads())-1]
	if idle["summary"] != "done" {
		t.Errorf("idle = %v", idle)
	}
	// No further frames once stopped.
	frames := len(w.all())
	time.Sleep(20 * time.Millisecond)
	if len(w.all()) != frames {
		t.Error("spinner still writing after agent_end")
	}
}

func TestNamedSessionSpinnerSuffix(t *testing.T) {
	warpEnv(t)
	*pigwarp.FrameInterval = 5 * time.Millisecond
	h, w := startNotifyRig(t, &sessionState{name: ptr(" fix auth ")})
	h.Fire("agent_start", nil)
	waitFor(t, "a named frame", func() bool {
		for _, x := range w.all() {
			if x == "\x1b]0;⠋ - fix auth - proj\a" {
				return true
			}
		}
		return false
	})
	h.Fire("agent_end", nil)
}

func TestCustomRunSuppressesAnnouncements(t *testing.T) {
	warpEnv(t)
	h, w := startNotifyRig(t, &sessionState{})
	h.Fire("before_agent_start", map[string]any{"systemPromptOptions": map[string]any{}, "messageType": "user", "prompt": "first"})
	h.Fire("agent_start", nil)
	h.Fire("agent_end", nil)
	h.Fire("before_agent_start", map[string]any{"systemPromptOptions": map[string]any{}, "messageType": "custom"})
	h.Fire("agent_start", nil)
	h.Fire("agent_start", nil) // a continuation without a new before_agent_start stays suppressed
	h.Fire("agent_end", nil)
	h.Fire("before_agent_start", map[string]any{"systemPromptOptions": map[string]any{}, "prompt": "second"}) // no messageType (Pi before 1.2) is a user run
	h.Fire("agent_start", nil)
	want := []string{"session_start", "prompt_submit", "stop", "stop", "session_start", "prompt_submit"}
	if got := w.events(); !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	if q := w.payloads()[5]["query"]; q != "second" {
		t.Errorf("query = %v", q)
	}
	h.Fire("agent_end", nil)
}

func TestIdleTimerCancelledByNextTurn(t *testing.T) {
	warpEnv(t)
	*pigwarp.IdleDelay = 50 * time.Millisecond
	h, w := startNotifyRig(t, &sessionState{})
	h.Fire("agent_end", nil)
	h.Fire("agent_start", nil)
	time.Sleep(100 * time.Millisecond)
	if n := w.count("idle_prompt"); n != 0 {
		t.Fatalf("idle_prompt emitted %d times after agent_start", n)
	}
	h.Fire("agent_end", nil)
	h.Fire("session_shutdown", nil)
	time.Sleep(100 * time.Millisecond)
	if n := w.count("idle_prompt"); n != 0 {
		t.Fatalf("idle_prompt emitted %d times after session_shutdown", n)
	}
}

func TestBlockingToolBadge(t *testing.T) {
	warpEnv(t)
	h, w := startNotifyRig(t, &sessionState{})
	h.Fire("agent_start", nil)
	h.Fire("tool_call", map[string]any{"toolName": "bash", "toolCallId": "c0", "input": map[string]any{"command": "ls"}})
	h.Fire("tool_execution_end", map[string]any{"toolName": "bash", "toolCallId": "c0"})
	if got := w.events(); !reflect.DeepEqual(got, []string{"session_start", "prompt_submit"}) {
		t.Fatalf("non-blocking tool emitted: %v", got)
	}
	h.Fire("tool_call", map[string]any{"toolName": "ask_user_question", "toolCallId": "c1", "input": map[string]any{"question": "why?"}})
	all := w.all()
	if all[len(all)-1] != pigwarp.PopTitleStack || payloadOf(all[len(all)-2])["event"] != "question_asked" {
		t.Fatalf("tool_call should emit question_asked then pop the title: %q", all[len(all)-2:])
	}
	h.Fire("tool_execution_end", map[string]any{"toolName": "ask_user_question", "toolCallId": "c1"})
	all = w.all()
	done := payloadOf(all[len(all)-2])
	if all[len(all)-1] != pigwarp.PushTitleStack || done["event"] != "tool_complete" || done["tool_name"] != "ask_user_question" {
		t.Fatalf("tool_execution_end: %q", all[len(all)-2:])
	}
	if !reflect.DeepEqual(done["tool_input"], map[string]any{"question": "why?"}) {
		t.Errorf("tool_input = %v", done["tool_input"])
	}
	h.Fire("agent_end", nil)
	if n := w.count("tool_complete"); n != 1 {
		t.Errorf("answered tool re-drained at agent_end: %d tool_complete", n)
	}
}

func TestAbortedBlockingToolDrainedAtAgentEnd(t *testing.T) {
	warpEnv(t)
	h, w := startNotifyRig(t, &sessionState{})
	h.Fire("agent_start", nil)
	h.Fire("tool_call", map[string]any{"toolName": "ask_user_question", "toolCallId": "c1", "input": map[string]any{"q": 1.0}})
	h.Fire("agent_end", nil) // ESC: no tool_execution_end
	want := []string{"session_start", "prompt_submit", "question_asked", "tool_complete", "stop"}
	if got := w.events(); !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	if in := w.payloads()[3]["tool_input"]; !reflect.DeepEqual(in, map[string]any{"q": 1.0}) {
		t.Errorf("drained tool_input = %v", in)
	}
	h.Fire("agent_end", nil)
	if n := w.count("tool_complete"); n != 1 {
		t.Errorf("second agent_end re-drained: %d", n)
	}
}

func TestHeartbeat(t *testing.T) {
	warpEnv(t)
	cfg := filepath.Join(os.Getenv("HOME"), ".config", "pig-warp")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "config.json"), []byte(`{"heartbeatMs": 10, "blockingTools": ["my_tool"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	h, w := startNotifyRig(t, &sessionState{})
	h.Fire("before_agent_start", map[string]any{"systemPromptOptions": map[string]any{}, "messageType": "user", "prompt": "work"})
	h.Fire("agent_start", nil)
	waitFor(t, "heartbeats", func() bool { return w.count("prompt_submit") >= 3 })
	if q := w.payloads()[len(w.payloads())-1]["query"]; q != "work" {
		t.Errorf("heartbeat query = %v", q)
	}
	// A configured blocking tool pauses the heartbeat; its end resumes it.
	h.Fire("tool_call", map[string]any{"toolName": "my_tool", "toolCallId": "c1"})
	time.Sleep(15 * time.Millisecond)
	paused := w.count("prompt_submit")
	time.Sleep(40 * time.Millisecond)
	if n := w.count("prompt_submit"); n != paused {
		t.Fatalf("heartbeat ran during a blocking tool: %d -> %d", paused, n)
	}
	h.Fire("tool_execution_end", map[string]any{"toolName": "my_tool", "toolCallId": "c1"})
	waitFor(t, "resumed heartbeat", func() bool { return w.count("prompt_submit") > paused })
	h.Fire("agent_end", nil)
	time.Sleep(15 * time.Millisecond)
	stopped := w.count("prompt_submit")
	time.Sleep(40 * time.Millisecond)
	if n := w.count("prompt_submit"); n != stopped {
		t.Fatalf("heartbeat ran after agent_end: %d -> %d", stopped, n)
	}
	// ask_user_question is no longer blocking once the config replaces the list.
	h.Fire("tool_call", map[string]any{"toolName": "ask_user_question", "toolCallId": "c2"})
	if w.count("question_asked") != 1 {
		t.Error("default blocking tool still active with a configured list")
	}
}

func TestShutdownStopsEverything(t *testing.T) {
	warpEnv(t)
	*pigwarp.FrameInterval = 5 * time.Millisecond
	h, w := startNotifyRig(t, &sessionState{})
	h.Fire("before_agent_start", map[string]any{"systemPromptOptions": map[string]any{}, "messageType": "user", "prompt": "x"})
	h.Fire("agent_start", nil)
	waitFor(t, "a frame", func() bool {
		return strings.Contains(strings.Join(w.all(), ""), "\x1b]0;")
	})
	h.FireAll("session_shutdown", nil)
	all := w.all()
	if all[len(all)-1] != pigwarp.PopTitleStack {
		t.Fatalf("shutdown did not pop the title: %q", all[len(all)-1])
	}
	n := len(all)
	time.Sleep(20 * time.Millisecond)
	if len(w.all()) != n {
		t.Error("writes continued after shutdown")
	}
}

func TestSubagentChildRegistersNothing(t *testing.T) {
	for _, env := range []string{"PIG_SUBAGENT", "TINTINWEB_SUBAGENT_DEPTH"} {
		t.Run(env, func(t *testing.T) {
			warpEnv(t)
			t.Setenv(env, "1")
			if h := StartHost(t, pigwarp.Extension(), HostOptions{}); h.Registered("agent_start") {
				t.Error("agent_start registered in a subagent child")
			}
		})
	}
}

// busEmit delivers a pi.events emit to the extension's listener on channel, as the host does.
func busEmit(t *testing.T, h *Host, channel string, data map[string]any) {
	t.Helper()
	for _, c := range h.CallsTo("events.on") {
		if c.Args["channel"] == channel {
			raw, _ := json.Marshal(data)
			if _, failure := h.roundTrip(map[string]any{"method": "events.dispatch", "args": map[string]any{"handlerId": c.Args["handlerId"], "channel": channel, "json": json.RawMessage(raw)}}); failure != "" {
				t.Fatalf("%s: %s", channel, failure)
			}
			return
		}
	}
	t.Fatalf("no listener on %s", channel)
}

// lastTitle is the last OSC 0 title written after the writes counted by from.
func lastTitle(w *writes, from int) string {
	all := w.all()
	for i := len(all) - 1; i >= from; i-- {
		if strings.HasPrefix(all[i], "\x1b]0;") {
			return all[i]
		}
	}
	return ""
}

func waitTitle(t *testing.T, w *writes, title string) {
	t.Helper()
	from := len(w.all())
	waitFor(t, title, func() bool { return lastTitle(w, from) == "\x1b]0;"+title+"\a" })
}

func TestForegroundSubagentInTitle(t *testing.T) {
	warpEnv(t)
	*pigwarp.FrameInterval = 5 * time.Millisecond
	h, w := startNotifyRig(t, &sessionState{})
	h.Fire("agent_start", nil)
	waitTitle(t, w, "⠋ - proj")
	// list and get run no agent.
	h.Fire("tool_execution_start", map[string]any{"toolCallId": "c0", "toolName": "subagent", "args": map[string]any{"action": "list"}})
	h.Fire("tool_execution_start", map[string]any{"toolCallId": "c1", "toolName": "subagent", "args": map[string]any{"agent": "scout", "task": "look"}})
	waitTitle(t, w, "⠋ scout - proj")
	h.Fire("tool_execution_start", map[string]any{"toolCallId": "c2", "toolName": "Agent", "args": map[string]any{"subagent_type": "Explore"}})
	waitTitle(t, w, "⠋ scout +1 - proj")
	h.Fire("tool_execution_end", map[string]any{"toolCallId": "c1", "toolName": "subagent"})
	waitTitle(t, w, "⠋ Explore - proj")
	h.Fire("tool_execution_end", map[string]any{"toolCallId": "c2", "toolName": "Agent"})
	waitTitle(t, w, "⠋ - proj")
}

func TestBackgroundSubagentKeepsSpinnerPastTurn(t *testing.T) {
	warpEnv(t)
	*pigwarp.IdleDelay = time.Hour
	*pigwarp.FrameInterval = 5 * time.Millisecond
	h, w := startNotifyRig(t, &sessionState{})
	h.Fire("agent_start", nil)
	busEmit(t, h, "subagents:created", map[string]any{"id": "a1", "type": "Explore", "isBackground": true})
	busEmit(t, h, "subagents:created", map[string]any{"id": "a2", "type": "Plan", "isBackground": true})
	h.Fire("agent_end", nil)
	waitTitle(t, w, "⠋ Explore +1 - proj")
	busEmit(t, h, "subagents:failed", map[string]any{"id": "a1"})
	waitTitle(t, w, "⠋ Plan - proj")
	busEmit(t, h, "subagents:completed", map[string]any{"id": "a2"})
	if all := w.all(); all[len(all)-1] != pigwarp.PopTitleStack {
		t.Errorf("expected title pop once the last agent settled, got %q", all[len(all)-1])
	}
}

func todoSnapshot(tasks ...map[string]any) map[string]any {
	list := []any{}
	for _, t := range tasks {
		list = append(list, t)
	}
	return map[string]any{"action": "update", "params": map[string]any{}, "tasks": list, "nextId": len(tasks) + 1}
}

func TestTodoTaskInTitle(t *testing.T) {
	warpEnv(t)
	*pigwarp.FrameInterval = 5 * time.Millisecond
	// A resumed session: the branch's last todo snapshot has a task in progress, by subject only.
	s := &sessionState{branch: []map[string]any{
		{"type": "message", "message": map[string]any{"role": "toolResult", "toolName": "todo", "details": todoSnapshot(
			map[string]any{"id": 1.0, "subject": "Write tests", "status": "completed"},
			map[string]any{"id": 2.0, "subject": "Implement", "status": "in_progress"},
		)}},
		{"type": "message", "message": map[string]any{"role": "toolResult", "toolName": "read", "details": map[string]any{}}},
	}}
	h, w := startNotifyRig(t, s)
	h.Fire("agent_start", nil)
	waitTitle(t, w, "⠋ Implement - proj")

	// A todo result switches to its first task in progress, by activeForm.
	h.Fire("tool_execution_end", map[string]any{"toolCallId": "t1", "toolName": "todo", "result": map[string]any{"details": todoSnapshot(
		map[string]any{"id": 2.0, "subject": "Implement", "status": "completed"},
		map[string]any{"id": 3.0, "subject": "Release", "activeForm": "Releasing", "status": "in_progress"},
	)}})
	waitTitle(t, w, "⠋ Releasing - proj")
	h.Fire("tool_execution_start", map[string]any{"toolCallId": "c1", "toolName": "subagent", "args": map[string]any{"agent": "worker"}})
	waitTitle(t, w, "⠋ Releasing · worker - proj")

	// A result without a snapshot (an error) keeps the task; one with nothing in progress clears it.
	h.Fire("tool_execution_end", map[string]any{"toolCallId": "t2", "toolName": "todo", "result": map[string]any{}})
	h.Fire("tool_execution_end", map[string]any{"toolCallId": "t3", "toolName": "todo", "result": map[string]any{"details": todoSnapshot(
		map[string]any{"id": 3.0, "subject": "Release", "status": "completed"},
	)}})
	waitTitle(t, w, "⠋ worker - proj")
}
