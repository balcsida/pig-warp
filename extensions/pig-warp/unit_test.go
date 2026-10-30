package pig_warp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseWarpVersionAndGate(t *testing.T) {
	for _, tc := range []struct {
		raw    string
		broken bool
	}{
		{"v0.2026.3.25.8.24.stable_5", true},  // equal to the threshold is gated
		{"v0.2026.3.25.8.24.preview_5", true}, // preview shares the threshold
		{"v0.2026.3.25.8.24.stable_6", false}, // one revision above
		{"v0.2026.3.26.0.0.stable_1", false},
		{"v0.2025.12.31.23.59.stable_9", true},
		{"v0.2026.3.25.8.24.dev_5", false}, // dev is never gated
		{"garbage", false},                 // unparseable is treated as fine
		{"", false},
	} {
		if got := isBrokenVersion(tc.raw); got != tc.broken {
			t.Errorf("isBrokenVersion(%q) = %v, want %v", tc.raw, got, tc.broken)
		}
	}
	tuple, channel, ok := parseWarpVersion("v0.2026.3.25.8.24.stable_5")
	if !ok || channel != "stable" || tuple != (versionTuple{2026, 3, 25, 8, 24, 5, 5}) {
		t.Errorf("parseWarpVersion = %v %q %v", tuple, channel, ok)
	}
}

func TestNegotiateProtocolVersion(t *testing.T) {
	for raw, want := range map[string]int{"": 1, "x": 1, "1": 1, "2": 1, "0": 0} {
		t.Setenv("WARP_CLI_AGENT_PROTOCOL_VERSION", raw)
		if got := negotiateProtocolVersion(); got != want {
			t.Errorf("negotiateProtocolVersion(%q) = %d, want %d", raw, got, want)
		}
	}
}

func TestSupportsStructured(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	t.Setenv("WARP_CLI_AGENT_PROTOCOL_VERSION", "1")
	t.Setenv("WARP_CLIENT_VERSION", "v0.2026.4.1.8.24.stable_1")
	if supportsStructured() {
		t.Fatal("not Warp")
	}
	t.Setenv("TERM_PROGRAM", "WarpTerminal")
	if !supportsStructured() {
		t.Fatal("working Warp")
	}
	t.Setenv("WARP_CLIENT_VERSION", "v0.2026.3.25.8.24.stable_5")
	if supportsStructured() {
		t.Fatal("broken build")
	}
	t.Setenv("WARP_CLIENT_VERSION", "")
	t.Setenv("WARP_CLI_AGENT_PROTOCOL_VERSION", "")
	if supportsStructured() {
		t.Fatal("no structured protocol")
	}
}

func TestPayloadJSON(t *testing.T) {
	t.Setenv("WARP_CLI_AGENT_PROTOCOL_VERSION", "1")
	s := session{id: "sess-1", cwd: "/home/me/proj"}
	for name, got := range map[string]string{
		"start":    s.sessionStart().json(),
		"prompt":   s.promptSubmit("").json(),
		"stop":     s.stop(nil).json(),
		"idle":     s.idlePrompt("done").json(),
		"tool":     s.toolComplete("ask_user_question", map[string]any{"q": "<a&b>"}).json(),
		"toolNoIn": s.toolComplete("ask_user_question", nil).json(),
	} {
		want := map[string]string{
			"start":    `{"v":1,"agent":"pi","event":"session_start","session_id":"sess-1","cwd":"/home/me/proj","project":"proj"}`,
			"prompt":   `{"v":1,"agent":"pi","event":"prompt_submit","session_id":"sess-1","cwd":"/home/me/proj","project":"proj","query":""}`,
			"stop":     `{"v":1,"agent":"pi","event":"stop","session_id":"sess-1","cwd":"/home/me/proj","project":"proj","query":"","response":""}`,
			"idle":     `{"v":1,"agent":"pi","event":"idle_prompt","session_id":"sess-1","cwd":"/home/me/proj","project":"proj","summary":"done"}`,
			"tool":     `{"v":1,"agent":"pi","event":"tool_complete","session_id":"sess-1","cwd":"/home/me/proj","project":"proj","tool_name":"ask_user_question","tool_input":{"q":"<a&b>"}}`,
			"toolNoIn": `{"v":1,"agent":"pi","event":"tool_complete","session_id":"sess-1","cwd":"/home/me/proj","project":"proj","tool_name":"ask_user_question"}`,
		}[name]
		if got != want {
			t.Errorf("%s:\n got %s\nwant %s", name, got, want)
		}
	}
	if got := formatOSC777(oscTitle, "{}"); got != "\x1b]777;notify;warp://cli-agent;{}\a" {
		t.Errorf("formatOSC777 = %q", got)
	}
}

func TestTextHelpers(t *testing.T) {
	if got := truncate(strings.Repeat("x", 201)); len(got) != 200 || !strings.HasSuffix(got, "...") {
		t.Errorf("truncate: %d %q", len(got), got[190:])
	}
	if got := truncate("short"); got != "short" {
		t.Errorf("truncate short = %q", got)
	}
	block := "<skill name=\"commit\" location=\"/s/SKILL.md\">\nbody\n</skill>\n\nSkill input: the config change"
	if got := summarizeSkillBlock(block); got != "/skill:commit the config change" {
		t.Errorf("summarizeSkillBlock = %q", got)
	}
	if got := summarizeSkillBlock("<skill name=\"commit\" location=\"/s\">\nbody\n</skill>"); got != "/skill:commit" {
		t.Errorf("summarizeSkillBlock bare = %q", got)
	}
	if got := summarizeSkillBlock("plain"); got != "plain" {
		t.Errorf("summarizeSkillBlock plain = %q", got)
	}
	branch := []map[string]any{
		{"type": "message", "message": map[string]any{"role": "user", "content": "first"}},
		{"type": "message", "message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "thinking", "thinking": "hmm"}, map[string]any{"type": "text", "text": "reply"}}}},
		{"type": "message", "message": map[string]any{"role": "user", "content": []any{map[string]any{"type": "image"}}}}, // empty text is skipped
		{"type": "custom", "customType": "x"},
	}
	if got := lastMessageText(branch, "user"); got != "first" {
		t.Errorf("lastUserText = %q", got)
	}
	if got := lastMessageText(branch, "assistant"); got != "reply" {
		t.Errorf("lastAssistantText = %q", got)
	}
	if got := jsTrim("\ufeff x \u0085"); got != "x \u0085" {
		t.Errorf("jsTrim = %q", got)
	}
}

func TestLoadConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	blocking, hb := loadConfig()
	if !blocking["ask_user_question"] || len(blocking) != 1 || hb != 15000 {
		t.Fatalf("defaults: %v %d", blocking, hb)
	}
	write := func(dir, body string) {
		p := filepath.Join(dir, "pig-warp", "config.json")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(home, ".config"), `{"blockingTools": ["a", "", "b"], "heartbeatMs": 5000}`)
	if blocking, hb = loadConfig(); !blocking["a"] || !blocking["b"] || len(blocking) != 2 || hb != 5000 {
		t.Fatalf("config: %v %d", blocking, hb)
	}
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if _, hb = loadConfig(); hb != 5000 { // XDG without the file falls back to ~/.config
		t.Fatalf("fallback: %d", hb)
	}
	write(xdg, `{"blockingTools": [], "heartbeatMs": 0}`)
	if blocking, hb = loadConfig(); len(blocking) != 0 || hb != 0 {
		t.Fatalf("xdg: %v %d", blocking, hb)
	}
	write(xdg, `{"heartbeatMs": -3}`)
	if _, hb = loadConfig(); hb != 15000 {
		t.Fatalf("negative heartbeat: %d", hb)
	}
	write(xdg, `not json`)
	if blocking, hb = loadConfig(); !blocking["ask_user_question"] || hb != 15000 {
		t.Fatalf("malformed: %v %d", blocking, hb)
	}
}

func TestParseBranchArgs(t *testing.T) {
	for _, tc := range []struct{ in, model, msg string }{
		{"", "", ""},
		{"  explore the auth edge cases ", "", "explore the auth edge cases"},
		{"--model flash", "flash", ""},
		{"--model gemini/flash-2.5 try rewriting", "gemini/flash-2.5", "try rewriting"},
		{"try --model flash", "", "try --model flash"},
		{"--model", "", "--model"},
	} {
		model, msg := parseBranchArgs(tc.in)
		if model != tc.model || msg != tc.msg {
			t.Errorf("parseBranchArgs(%q) = %q %q, want %q %q", tc.in, model, msg, tc.model, tc.msg)
		}
	}
}

func TestResolveClosestModel(t *testing.T) {
	models := []modelRef{
		{Provider: "anthropic", ID: "claude-opus-5-5", Name: "Claude Opus 5.5"},
		{Provider: "anthropic", ID: "claude-sonnet-5-5", Name: "Claude Sonnet 5.5"},
		{Provider: "google", ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash"},
		{Provider: "google", ID: "gemini-2.5-flash-lite", Name: "Gemini 2.5 Flash Lite"},
	}
	for _, tc := range []struct{ query, want string }{
		{"anthropic/claude-opus-5-5", "anthropic/claude-opus-5-5"},
		{"claude-sonnet-5-5", "anthropic/claude-sonnet-5-5"},
		{"flash", "google/gemini-2.5-flash"}, // substring, shorter id wins the tie-break
		{"google/flash-lite", "google/gemini-2.5-flash-lite"},
		{"gemini/flash", ""},                    // unknown provider scope
		{"opus55", "anthropic/claude-opus-5-5"}, // collapsed match
		{"nope", ""},
		{"", ""},
	} {
		got := ""
		if m := resolveClosestModel(tc.query, models, "anthropic"); m != nil {
			got = m.Provider + "/" + m.ID
		}
		if got != tc.want {
			t.Errorf("resolveClosestModel(%q) = %q, want %q", tc.query, got, tc.want)
		}
	}
	// The preferred provider breaks a tie between equal matches.
	same := []modelRef{{Provider: "b", ID: "m"}, {Provider: "a", ID: "m"}}
	if m := resolveClosestModel("m", same, "a"); m == nil || m.Provider != "a" {
		t.Errorf("preferred provider: %+v", m)
	}
}

func TestReadLaunch(t *testing.T) {
	entry := func(kind, id string) map[string]any {
		return map[string]any{"type": "custom", "customType": kind, "data": map[string]any{"launchId": id, "message": "hi", "autoSubmitSeconds": 10.0}}
	}
	if l := readLaunch([]map[string]any{entry(launchType, "L1")}); l == nil || l.LaunchID != "L1" || l.Message != "hi" || l.AutoSubmitSeconds != 10 {
		t.Fatalf("launch: %+v", l)
	}
	if l := readLaunch([]map[string]any{entry(launchType, "L1"), entry(launchAppliedType, "L1")}); l != nil {
		t.Fatalf("applied launch returned: %+v", l)
	}
	if l := readLaunch([]map[string]any{entry(launchType, "L1"), entry(launchAppliedType, "L1"), entry(launchType, "L2")}); l == nil || l.LaunchID != "L2" {
		t.Fatalf("latest launch: %+v", l)
	}
	if l := readLaunch([]map[string]any{entry(launchType, "")}); l != nil {
		t.Fatalf("launch without id: %+v", l)
	}
}

func TestTomlStringAndEditableInput(t *testing.T) {
	s := tomlString("a \"q\" \\ \x7f <&>")
	if s != `"a \"q\" \\ \u007f <&>"` {
		t.Errorf("tomlString = %s", s)
	}
	var back string
	if err := json.Unmarshal([]byte(s), &back); err != nil || back != "a \"q\" \\ \x7f <&>" {
		t.Errorf("round trip: %q %v", back, err)
	}
	for in, want := range map[string]bool{"": false, "a": true, "\x7f": true, "\x1b": false, "\x1b[A": false, "\r": true, "\n": true, "\b": true, "ab": true, "\x01": false} {
		if got := isEditableInput(in); got != want {
			t.Errorf("isEditableInput(%q) = %v", in, got)
		}
	}
}
