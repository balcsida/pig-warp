package pig_warp

// Optional config at $XDG_CONFIG_HOME/pig-warp/config.json (falls back to
// ~/.config/pig-warp/config.json). upstream: rpiv-warp/config.ts. A missing
// or malformed file is the defaults; the file is read once at registration.

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const defaultHeartbeatMs = 15000

var defaultBlockingTools = []string{"ask_user_question"}

type config struct {
	BlockingTools *[]string `json:"blockingTools"`
	HeartbeatMs   *float64  `json:"heartbeatMs"`
}

func configPath() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(dir) {
		if p := filepath.Join(dir, "pig-warp", "config.json"); fileExists(p) {
			return p
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "pig-warp", "config.json")
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// loadConfig returns the blocking tool set and heartbeat (0 = disabled).
func loadConfig() (blocking map[string]bool, heartbeatMs int) {
	var c config
	if raw, err := os.ReadFile(configPath()); err == nil {
		_ = json.Unmarshal(raw, &c)
	}
	heartbeatMs = defaultHeartbeatMs
	if c.HeartbeatMs != nil && *c.HeartbeatMs == 0 {
		heartbeatMs = 0 // explicitly disabled
	} else if c.HeartbeatMs != nil && *c.HeartbeatMs > 0 {
		heartbeatMs = int(*c.HeartbeatMs)
	}
	tools := defaultBlockingTools
	if c.BlockingTools != nil {
		tools = *c.BlockingTools
	}
	blocking = map[string]bool{}
	for _, t := range tools {
		if t != "" {
			blocking[t] = true
		}
	}
	return blocking, heartbeatMs
}
