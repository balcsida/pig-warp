package pig_warp_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	pigwarp "github.com/balcsida/pig-warp/extensions/pig-warp"
)

func startBranchRig(t *testing.T, s *sessionState, cwd string) *Host {
	t.Helper()
	captureWrites(t)
	return StartHost(t, pigwarp.Extension(), HostOptions{Cwd: cwd, OnCallValue: s.answer})
}

func tabConfigDir(t *testing.T) string {
	t.Helper()
	home := os.Getenv("HOME")
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, ".warp", "tab_configs")
	}
	return filepath.Join(home, ".local", "share", "warp-terminal", "tab_configs")
}

// field reads one TOML field the way upstream's test does: the generated subset uses JSON-compatible strings and arrays.
func field(t *testing.T, text, name string) any {
	t.Helper()
	m := regexp.MustCompile(`(?m)^` + name + ` = (.+)$`).FindStringSubmatch(text)
	if m == nil {
		t.Fatalf("missing Tab Config field %s in:\n%s", name, text)
	}
	var v any
	if err := json.Unmarshal([]byte(m[1]), &v); err != nil {
		t.Fatalf("field %s: %v", name, err)
	}
	return v
}

func TestBranchLaunchesWarpTabThatResumesTheFork(t *testing.T) {
	warpEnv(t)
	home := os.Getenv("HOME")
	cwd := filepath.Join(home, "project's \"quoted\" $HOME & café\\folder\x7f")
	if err := os.Mkdir(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	forkFile := filepath.Join(home, "fork's \"quoted\"; touch INJECTED #\x7f.jsonl")
	s := &sessionState{sessionFile: ptr(filepath.Join(home, "session.jsonl")), leafID: ptr("leaf-1"), forkFile: forkFile}
	h := startBranchRig(t, s, cwd)

	if failure := h.Command("branch", ""); failure != "" {
		t.Fatal(failure)
	}
	execs := h.CallsTo("exec")
	if len(execs) != 1 {
		t.Fatalf("exec calls = %v", execs)
	}
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	args := execs[0].Args["args"].([]any)
	if execs[0].Args["command"] != opener || len(args) != 1 || !strings.HasPrefix(args[0].(string), "warp://tab_config/tmp_pi_branch_") {
		t.Fatalf("exec = %v", execs[0].Args)
	}
	name := strings.TrimPrefix(args[0].(string), "warp://tab_config/")
	configFile := filepath.Join(tabConfigDir(t), name+".toml")
	raw, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "\x7f") {
		t.Error("raw DEL in the Tab Config")
	}
	if !strings.Contains(text, "[[panes]]") || field(t, text, "type") != "terminal" || field(t, text, "directory") != cwd {
		t.Errorf("config:\n%s", text)
	}
	if info, _ := os.Stat(configFile); info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o", info.Mode().Perm())
	}
	if g := s.graph(); len(g) != 2 || g[0]["op"] != "open" || g[1]["method"] != "createBranchedSession" {
		t.Errorf("graph calls = %v", g)
	}
	if got := notices(h); len(got) != 1 || got[0] != "Created fork and requested terminal launch" {
		t.Errorf("notices = %v", got)
	}

	// Run the emitted command with a harmless `pig` substitute: broken quoting or missing cleanup must fail.
	bin := filepath.Join(home, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	fake := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$RESULT_FILE\"\npwd >> \"$RESULT_FILE\"\n"
	if err := os.WriteFile(filepath.Join(bin, "pig"), []byte(fake), 0o700); err != nil {
		t.Fatal(err)
	}
	var commands []string
	for _, c := range field(t, text, "commands").([]any) {
		commands = append(commands, c.(string))
	}
	resultFile := filepath.Join(home, "result.txt")
	cmd := exec.Command("sh", "-c", strings.Join(commands, " && "))
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "RESULT_FILE="+resultFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("command failed: %v\n%s", err, out)
	}
	result, _ := os.ReadFile(resultFile)
	realCwd, _ := filepath.EvalSymlinks(cwd)
	if want := "--session\n" + forkFile + "\n" + realCwd + "\n"; string(result) != want {
		t.Errorf("fake pig saw:\n%s\nwant:\n%s", result, want)
	}
	if _, err := os.Stat(configFile); !os.IsNotExist(err) {
		t.Error("startup did not remove the consumed config")
	}
	if _, err := os.Stat(filepath.Join(cwd, "INJECTED")); !os.IsNotExist(err) {
		t.Error("shell injection through the fork path")
	}
}

func TestBranchQueuesModelAndDraft(t *testing.T) {
	warpEnv(t)
	home := os.Getenv("HOME")
	s := &sessionState{sessionFile: ptr("/s/session.jsonl"), leafID: ptr("leaf-1"), forkFile: filepath.Join(home, "fork.jsonl")}
	h := startBranchRig(t, s, home)
	if failure := h.Command("branch", "--model flash  explore the auth edge cases"); failure != "" {
		t.Fatal(failure)
	}
	g := s.graph()
	if len(g) != 4 || g[2]["op"] != "open" || g[3]["method"] != "appendCustomEntry" {
		t.Fatalf("graph calls = %v", g)
	}
	args := g[3]["args"].([]any)
	data := args[1].(map[string]any)
	if args[0] != pigwarp.LaunchType || data["message"] != "explore the auth edge cases" || data["autoSubmitSeconds"] != 10.0 || data["launchId"] == "" {
		t.Errorf("launch entry = %v", data)
	}
	// With no session model the query cannot resolve; the child keeps the session model.
	if data["modelNotice"] != "Model 'flash' did not resolve; branch will use the session model." || data["targetModel"] != nil {
		t.Errorf("model resolution = %v", data)
	}
	if got := notices(h); len(got) != 1 || got[0] != "Created fork and requested terminal launch (draft queued)" {
		t.Errorf("notices = %v", got)
	}
}

func TestBranchRefusals(t *testing.T) {
	warpEnv(t)
	s := &sessionState{}
	h := startBranchRig(t, s, os.Getenv("HOME"))
	if failure := h.Command("branch", ""); failure != "" {
		t.Fatal(failure)
	}
	if got := notices(h); len(got) != 1 || got[0] != "Session is not persisted. Restart without --no-session." {
		t.Errorf("notices = %v", got)
	}
	s.mu.Lock()
	s.sessionFile = ptr("/s/session.jsonl")
	s.mu.Unlock()
	if failure := h.Command("branch", ""); failure != "" {
		t.Fatal(failure)
	}
	if got := notices(h); len(got) != 2 || got[1] != "No messages yet. Nothing to branch." {
		t.Errorf("notices = %v", got)
	}
	if len(s.graph()) != 0 || len(h.CallsTo("exec")) != 0 {
		t.Error("refused command touched the session or the terminal")
	}

	s.mu.Lock()
	s.leafID = ptr("leaf-1")
	s.forkFile = "/s/fork.jsonl"
	s.mu.Unlock()
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	if failure := h.Command("branch", ""); !strings.Contains(failure, "not WarpTerminal") {
		t.Errorf("outside Warp: %q", failure)
	}
	t.Setenv("TERM_PROGRAM", "WarpTerminal")
	if entries, _ := os.ReadDir(tabConfigDir(t)); len(entries) != 0 {
		t.Error("config left behind")
	}
}

func TestBranchRejectsReservedTemplateAndFailedOpener(t *testing.T) {
	warpEnv(t)
	home := os.Getenv("HOME")
	reserved := filepath.Join(home, "project-{{autogenerated_branch_name}}")
	s := &sessionState{sessionFile: ptr("/s/session.jsonl"), leafID: ptr("leaf-1"), forkFile: filepath.Join(reserved, "fork.jsonl")}
	h := startBranchRig(t, s, home)
	if failure := h.Command("branch", ""); !strings.Contains(failure, "{{autogenerated_branch_name}}") {
		t.Errorf("reserved fork path: %q", failure)
	}
	if len(h.CallsTo("exec")) != 0 {
		t.Error("opener called for a rejected launch")
	}
	if entries, _ := os.ReadDir(tabConfigDir(t)); len(entries) != 0 {
		t.Error("config written for a rejected launch")
	}

	s.mu.Lock()
	s.forkFile = "/s/fork.jsonl"
	s.exec = func(string, []string) (map[string]any, string) {
		return map[string]any{"stdout": "", "stderr": "opener unavailable", "code": 1}, ""
	}
	s.mu.Unlock()
	if failure := h.Command("branch", ""); failure != "opener unavailable" {
		t.Errorf("failed opener: %q", failure)
	}
	if entries, _ := os.ReadDir(tabConfigDir(t)); len(entries) != 0 {
		t.Error("config kept after a failed launch")
	}
	s.mu.Lock()
	s.exec = func(string, []string) (map[string]any, string) { return nil, "exec exploded" }
	s.mu.Unlock()
	if failure := h.Command("branch", ""); failure != "exec exploded" {
		t.Errorf("host exec error: %q", failure)
	}
	if entries, _ := os.ReadDir(tabConfigDir(t)); len(entries) != 0 {
		t.Error("config kept after a host error")
	}
}

func launchBranch(id, message string, seconds float64) []map[string]any {
	return []map[string]any{{"type": "custom", "customType": pigwarp.LaunchType, "data": map[string]any{"launchId": id, "message": message, "autoSubmitSeconds": seconds}}}
}

func TestChildAppliesLaunchAndEscCancelsCountdown(t *testing.T) {
	warpEnv(t)
	s := &sessionState{sessionFile: ptr("/s/fork.jsonl"), branch: launchBranch("L1", "  explore  ", 10)}
	h := startBranchRig(t, s, os.Getenv("HOME"))
	h.FireAll("session_start", map[string]any{"reason": "startup"})

	applied := h.CallsTo("appendEntry")
	if len(applied) != 1 || applied[0].Args["customType"] != pigwarp.AppliedType || applied[0].Args["data"].(map[string]any)["launchId"] != "L1" {
		t.Fatalf("applied entry = %v", applied)
	}
	if edits := h.CallsTo("ui.setEditorText"); len(edits) != 1 || edits[0].Args["text"] != "explore" {
		t.Fatalf("editor = %v", edits)
	}
	status := h.CallsTo("ui.setStatus")
	if len(status) != 1 || status[0].Args["key"] != pigwarp.StatusKey || !strings.Contains(status[0].Args["text"].(string), "branch auto-submit in 10s") {
		t.Fatalf("status = %v", status)
	}
	if terminalInput(t, h, "\x1b[A") { // a cursor key is neither Esc nor editing
		t.Error("cursor key consumed")
	}
	if !terminalInput(t, h, "\x1b") {
		t.Error("Esc not consumed")
	}
	status = h.CallsTo("ui.setStatus")
	if last := status[len(status)-1].Args; len(status) != 2 || last["key"] != pigwarp.StatusKey || last["text"] != nil {
		t.Errorf("status not cleared: %v", status)
	}
	if got := notices(h); len(got) != 1 || got[0] != "Auto-submit cancelled" {
		t.Errorf("notices = %v", got)
	}
	// Esc again is an ordinary key now.
	if terminalInput(t, h, "\x1b") {
		t.Error("Esc consumed after the countdown ended")
	}

	// The applied launch is not re-applied on the next start.
	h.FireAll("session_start", map[string]any{"reason": "resume"})
	if len(h.CallsTo("ui.setEditorText")) != 1 {
		t.Error("launch applied twice")
	}
}

func TestTypingCancelsAndEnterKeepsTheDraft(t *testing.T) {
	warpEnv(t)
	s := &sessionState{sessionFile: ptr("/s/fork.jsonl"), branch: launchBranch("L1", "hi", 10)}
	h := startBranchRig(t, s, os.Getenv("HOME"))
	h.FireAll("session_start", map[string]any{"reason": "startup"})
	if terminalInput(t, h, "x") {
		t.Error("typed character consumed")
	}
	if got := notices(h); len(got) != 1 || got[0] != "Auto-submit cancelled" {
		t.Errorf("notices = %v", got)
	}

	s2 := &sessionState{sessionFile: ptr("/s/fork.jsonl"), branch: launchBranch("L2", "hi", 10)}
	h2 := startBranchRig(t, s2, os.Getenv("HOME"))
	h2.FireAll("session_start", map[string]any{"reason": "startup"})
	if terminalInput(t, h2, "\r") {
		t.Error("Enter consumed")
	}
	if got := notices(h2); len(got) != 0 {
		t.Errorf("Enter should cancel silently: %v", got)
	}
	if status := h2.CallsTo("ui.setStatus"); len(status) != 2 {
		t.Errorf("status = %v", status)
	}
}

func TestCountdownSubmitsTheDraft(t *testing.T) {
	warpEnv(t)
	s := &sessionState{sessionFile: ptr("/s/fork.jsonl"), branch: launchBranch("L1", "hi", 1), idle: true}
	h := startBranchRig(t, s, os.Getenv("HOME"))
	h.FireAll("session_start", map[string]any{"reason": "startup"})
	s.mu.Lock()
	s.editor = "hi there " // the user may have edited through other means; the draft is read back
	s.mu.Unlock()
	waitFor(t, "auto-submit", func() bool { return len(h.CallsTo("sendUserMessage")) == 1 })
	sent := h.CallsTo("sendUserMessage")[0].Args
	if opts, _ := sent["options"].(map[string]any); sent["content"] != "hi there" || opts["deliverAs"] != "" {
		t.Errorf("sent = %v", sent)
	}
	if edits := h.CallsTo("ui.setEditorText"); edits[len(edits)-1].Args["text"] != "" {
		t.Error("editor not cleared before sending")
	}

	// A busy session gets the draft as a follow-up; an empty draft is reported instead.
	s2 := &sessionState{sessionFile: ptr("/s/fork.jsonl"), branch: launchBranch("L2", "hi", 1)}
	h2 := startBranchRig(t, s2, os.Getenv("HOME"))
	h2.FireAll("session_start", map[string]any{"reason": "startup"})
	s2.mu.Lock()
	s2.editor = ""
	s2.mu.Unlock()
	waitFor(t, "empty draft notice", func() bool {
		for _, n := range notices(h2) {
			if n == "Draft is empty" {
				return true
			}
		}
		return false
	})
	if len(h2.CallsTo("sendUserMessage")) != 0 {
		t.Error("empty draft sent")
	}

	s3 := &sessionState{sessionFile: ptr("/s/fork.jsonl"), branch: launchBranch("L3", "go", 1)}
	h3 := startBranchRig(t, s3, os.Getenv("HOME"))
	h3.FireAll("session_start", map[string]any{"reason": "startup"})
	waitFor(t, "follow-up", func() bool { return len(h3.CallsTo("sendUserMessage")) == 1 })
	if opts, _ := h3.CallsTo("sendUserMessage")[0].Args["options"].(map[string]any); opts["deliverAs"] != "followUp" {
		t.Errorf("busy session: %v", h3.CallsTo("sendUserMessage")[0].Args)
	}
}

func TestSessionSwitchClearsCountdown(t *testing.T) {
	warpEnv(t)
	s := &sessionState{sessionFile: ptr("/s/fork.jsonl"), branch: launchBranch("L1", "hi", 10)}
	h := startBranchRig(t, s, os.Getenv("HOME"))
	h.FireAll("session_start", map[string]any{"reason": "startup"})
	h.FireAll("session_before_switch", map[string]any{"reason": "new"})
	if status := h.CallsTo("ui.setStatus"); len(status) != 2 || status[1].Args["text"] != nil {
		t.Fatalf("status = %v", status)
	}
	time.Sleep(1100 * time.Millisecond)
	if len(h.CallsTo("ui.setStatus")) != 2 {
		t.Error("countdown ticked after the session switch")
	}
}
