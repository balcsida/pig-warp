package pig_warp

// OSC transport. upstream: rpiv-warp/warp-notify.ts. Every write opens the
// controlling terminal, writes, and closes; any failure is swallowed so a
// notification can never reach the agent loop. Windows has no /dev/tty, so
// nothing is written there (upstream's stdout fallback is untested and would
// be a fuse hazard in a Piglet).

import "os"

const (
	oscTitle = "warp://cli-agent" // index.ts:60
	ttyPath  = "/dev/tty"
)

// writeRaw is the single transport; tests replace it to capture bytes.
var writeRaw = func(s string) {
	f, err := os.OpenFile(ttyPath, os.O_WRONLY, 0)
	if err != nil {
		return
	}
	_, _ = f.WriteString(s)
	_ = f.Close()
}

func formatOSC777(title, body string) string { return "\x1b]777;notify;" + title + ";" + body + "\a" }
func formatOSC0(title string) string         { return "\x1b]0;" + title + "\a" }

const (
	pushTitleStack = "\x1b[22;0t"
	popTitleStack  = "\x1b[23;0t"
)

func emit(p payload) { writeRaw(formatOSC777(oscTitle, p.json())) }
