package pig_warp_test

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	pigwarp "github.com/balcsida/pig-warp/extensions/pig-warp"
)

// warpEnv puts the test inside a working Warp with a temporary home (no user config is read).
func warpEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("TERM_PROGRAM", "WarpTerminal")
	t.Setenv("WARP_CLI_AGENT_PROTOCOL_VERSION", "1")
	t.Setenv("WARP_CLIENT_VERSION", "v0.2026.4.1.8.24.stable_1")
}

// writes captures everything the extension writes to the terminal.
type writes struct {
	mu   sync.Mutex
	list []string
}

func captureWrites(t *testing.T) *writes {
	t.Helper()
	w := &writes{}
	old := *pigwarp.WriteRaw
	*pigwarp.WriteRaw = func(s string) {
		w.mu.Lock()
		defer w.mu.Unlock()
		w.list = append(w.list, s)
	}
	t.Cleanup(func() { *pigwarp.WriteRaw = old })
	return w
}

func (w *writes) all() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.list...)
}

// events returns the OSC 777 event names written so far, in order.
func (w *writes) events() []string {
	var out []string
	for _, s := range w.all() {
		if p := payloadOf(s); p != nil {
			out = append(out, p["event"].(string))
		}
	}
	return out
}

func payloadOf(s string) map[string]any {
	const prefix = "\x1b]777;notify;warp://cli-agent;"
	if !strings.HasPrefix(s, prefix) {
		return nil
	}
	var p map[string]any
	if json.Unmarshal([]byte(strings.TrimSuffix(strings.TrimPrefix(s, prefix), "\a")), &p) != nil {
		return nil
	}
	return p
}

func (w *writes) payloads() []map[string]any {
	var out []map[string]any
	for _, s := range w.all() {
		if p := payloadOf(s); p != nil {
			out = append(out, p)
		}
	}
	return out
}

func (w *writes) count(event string) int {
	n := 0
	for _, e := range w.events() {
		if e == event {
			n++
		}
	}
	return n
}

// waitFor polls cond until it holds or the deadline passes.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// sessionState is the fake host's session, answered through OnCallValue.
type sessionState struct {
	mu          sync.Mutex
	name        *string
	sessionFile *string
	leafID      *string
	branch      []map[string]any
	editor      string
	idle        bool
	forkFile    string
	exec        func(command string, args []string) (map[string]any, string)
	graphCalls  []map[string]any
}

func (s *sessionState) answer(method string, args map[string]any) (any, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch method {
	case "getSessionID":
		return map[string]any{"sessionId": "sess-1"}, ""
	case "getSessionName":
		return map[string]any{"name": s.name}, ""
	case "getSessionFile":
		return map[string]any{"sessionFile": s.sessionFile}, ""
	case "getLeafID":
		return map[string]any{"leafId": s.leafID}, ""
	case "sessionRead":
		if args["method"] == "getBranch" {
			return append([]map[string]any{}, s.branch...), ""
		}
	case "appendEntry":
		s.branch = append(s.branch, map[string]any{"type": "custom", "customType": args["customType"], "data": args["data"]})
	case "sessionGraph.new":
		s.graphCalls = append(s.graphCalls, args)
		return map[string]any{"handle": "h1"}, ""
	case "sessionGraph.call":
		s.graphCalls = append(s.graphCalls, args)
		switch args["method"] {
		case "createBranchedSession":
			return s.forkFile, ""
		case "appendCustomEntry":
			return "entry-1", ""
		}
	case "sessionGraph.release":
		return map[string]any{}, ""
	case "ui.getEditorText":
		return map[string]any{"text": s.editor}, ""
	case "ui.setEditorText":
		s.editor, _ = args["text"].(string)
	case "isIdle":
		return map[string]any{"idle": s.idle}, ""
	case "getModelInfo":
		return map[string]any{}, ""
	case "getScopedModels":
		return []any{}, ""
	case "exec":
		if s.exec != nil {
			var argv []string
			for _, a := range args["args"].([]any) {
				argv = append(argv, a.(string))
			}
			return s.exec(args["command"].(string), argv)
		}
		return map[string]any{"stdout": "", "stderr": "", "code": 0}, ""
	}
	return map[string]any{}, ""
}

func (s *sessionState) graph() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]map[string]any(nil), s.graphCalls...)
}

func ptr(s string) *string { return &s }

// terminalInput delivers raw terminal input the way the host does and reports whether it was consumed.
func terminalInput(t *testing.T, h *Host, data string) bool {
	t.Helper()
	raw, failure := h.roundTrip(map[string]any{"method": "terminal_input", "args": map[string]any{"data": data}})
	if failure != "" {
		t.Fatalf("terminal_input: %s", failure)
	}
	var v struct {
		Consume bool `json:"consume"`
	}
	_ = json.Unmarshal(raw, &v)
	return v.Consume
}

func notices(h *Host) []string {
	var out []string
	for _, c := range h.CallsTo("ui.notify") {
		msg, _ := c.Args["message"].(string)
		out = append(out, msg)
	}
	return out
}
