// Package pig_warp is a Go PiG port of three Pi extensions for the Warp
// terminal: @juicesharp/rpiv-warp (OSC 777 notifications and a tab-title
// spinner), pi-branch-out's Warp backend (/branch forks the session into a
// new Warp tab) and pi-warp-kitty-images (nothing to port: PiG detects Warp
// natively for Kitty images).
package pig_warp

import (
	"os"
	"path/filepath"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

// Extension returns the extension. Outside Warp, or on a known-broken Warp
// build, no notification handler is registered (index.ts:189); /branch is
// always registered and refuses outside Warp.
func Extension() *sdk.Extension {
	e := sdk.New("pig-warp")
	registerBranch(e)
	// A subagent child shares the parent's terminal; only the parent speaks to Warp.
	if supportsStructured() && os.Getenv("PIG_SUBAGENT") == "" && os.Getenv("TINTINWEB_SUBAGENT_DEPTH") == "" {
		registerNotify(e)
	}
	return e
}

func filepathBase(p string) string { return filepath.Base(p) }

// jsTrim is JavaScript's String.prototype.trim: it strips U+FEFF and keeps U+0085, unlike strings.TrimSpace.
func jsTrim(s string) string {
	return strings.Trim(s, "\t\n\v\f\r \u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000\ufeff")
}
