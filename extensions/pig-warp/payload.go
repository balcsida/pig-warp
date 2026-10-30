package pig_warp

// Warp structured-payload composition. upstream: rpiv-warp/payload.ts. Pure
// data: no I/O.

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

const (
	agentID       = "pi" // Warp routes CLIAgent::Pi to its default listener (payload.ts:22)
	truncateLimit = 200
)

// payload is the OSC 777 body; field order is the JSON order upstream emits.
type payload struct {
	V         int     `json:"v"`
	Agent     string  `json:"agent"`
	Event     string  `json:"event"`
	SessionID string  `json:"session_id"`
	Cwd       string  `json:"cwd"`
	Project   string  `json:"project"`
	Query     *string `json:"query,omitempty"`
	Response  *string `json:"response,omitempty"`
	Summary   *string `json:"summary,omitempty"`
	ToolName  *string `json:"tool_name,omitempty"`
	ToolInput any     `json:"tool_input,omitempty"`
}

func (p payload) json() string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(p)
	return strings.TrimSuffix(buf.String(), "\n")
}

// session is the identity every payload carries, read from the host at handler time so timers can emit after the handler returned.
type session struct {
	id  string
	cwd string
}

func (s session) envelope(event string) payload {
	return payload{V: negotiateProtocolVersion(), Agent: agentID, Event: event, SessionID: s.id, Cwd: s.cwd, Project: filepath.Base(s.cwd)}
}

func str(s string) *string { return &s }

func (s session) sessionStart() payload  { return s.envelope("session_start") }
func (s session) questionAsked() payload { return s.envelope("question_asked") }
func (s session) promptSubmit(query string) payload {
	p := s.envelope("prompt_submit")
	p.Query = str(summarizeSkillBlock(query))
	return p
}
func (s session) stop(branch []map[string]any) payload {
	p := s.envelope("stop")
	p.Query, p.Response = str(lastMessageText(branch, "user")), str(lastMessageText(branch, "assistant"))
	return p
}
func (s session) idlePrompt(summary string) payload {
	p := s.envelope("idle_prompt")
	p.Summary = str(summary)
	return p
}
func (s session) toolComplete(name string, input any) payload {
	p := s.envelope("tool_complete")
	p.ToolName, p.ToolInput = str(name), input
	return p
}

func truncate(s string) string {
	r := []rune(s)
	if len(r) <= truncateLimit {
		return s
	}
	return string(r[:truncateLimit-3]) + "..."
}

// summarizeSkillBlock collapses a `<skill ...>` wrapper back to `/skill:<name> <args>` so a toast shows what the human typed (payload.ts:81).
func summarizeSkillBlock(text string) string {
	parsed := sdk.ParseSkillBlock(text)
	if parsed == nil {
		return text
	}
	if parsed.UserMessage == nil {
		return "/skill:" + parsed.Name
	}
	return "/skill:" + parsed.Name + " " + strings.TrimPrefix(*parsed.UserMessage, "Skill input: ")
}

// messageText joins the text blocks of a message's content (payload.ts:64).
func messageText(content any) string {
	if s, ok := content.(string); ok {
		return s
	}
	blocks, _ := content.([]any)
	var parts []string
	for _, b := range blocks {
		m, _ := b.(map[string]any)
		if m["type"] == "text" {
			t, _ := m["text"].(string)
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n")
}

// lastMessageText reverse-scans the branch for the last non-empty message of role, truncated (payload.ts:114).
func lastMessageText(branch []map[string]any, role string) string {
	for i := len(branch) - 1; i >= 0; i-- {
		msg, _ := branch[i]["message"].(map[string]any)
		if branch[i]["type"] != "message" || msg["role"] != role {
			continue
		}
		if text := summarizeSkillBlock(messageText(msg["content"])); text != "" {
			return truncate(text)
		}
	}
	return ""
}
