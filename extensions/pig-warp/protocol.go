package pig_warp

// Warp detection and protocol negotiation. upstream: rpiv-warp/protocol.ts.
// Every function reads the environment fresh; nothing is cached.

import (
	"os"
	"regexp"
	"strconv"
)

// pluginMaxProtocolVersion is the highest OSC 777 protocol version this extension speaks (protocol.ts:46).
const pluginMaxProtocolVersion = 1

type versionTuple [7]int

// brokenVersions is the last Warp build per channel that advertises the protocol but does not render notifications (protocol.ts:53).
var brokenVersions = map[string]*versionTuple{
	"stable":  {2026, 3, 25, 8, 24, 5, 5},
	"preview": {2026, 3, 25, 8, 24, 5, 5},
	"dev":     nil,
}

var versionRe = regexp.MustCompile(`^v0\.(\d{4})\.(\d{1,2})\.(\d{1,2})\.(\d{1,2})\.(\d{1,2})\.(stable|preview|dev)_(\d+)$`)

func isWarpTerminal() bool { return os.Getenv("TERM_PROGRAM") == "WarpTerminal" }

// negotiateProtocolVersion is min(WARP_CLI_AGENT_PROTOCOL_VERSION, pluginMaxProtocolVersion); unparseable falls back to the maximum (protocol.ts:93).
func negotiateProtocolVersion() int {
	v, err := strconv.Atoi(os.Getenv("WARP_CLI_AGENT_PROTOCOL_VERSION"))
	if err != nil || v > pluginMaxProtocolVersion {
		return pluginMaxProtocolVersion
	}
	return v
}

// parseWarpVersion reads `v0.YYYY.M.D.H.M.(stable|preview|dev)_N`. The last two tuple slots both hold N, as upstream's do (protocol.ts:104).
func parseWarpVersion(raw string) (tuple versionTuple, channel string, ok bool) {
	m := versionRe.FindStringSubmatch(raw)
	if m == nil {
		return tuple, "", false
	}
	for i, idx := range []int{1, 2, 3, 4, 5, 7, 7} {
		tuple[i], _ = strconv.Atoi(m[idx])
	}
	return tuple, m[6], true
}

// tupleLeq is element-wise <=, true on equal (protocol.ts:121).
func tupleLeq(a, b versionTuple) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return true
}

func isBrokenVersion(raw string) bool {
	tuple, channel, ok := parseWarpVersion(raw)
	if !ok {
		return false
	}
	threshold := brokenVersions[channel]
	return threshold != nil && tupleLeq(tuple, *threshold)
}

// supportsStructured is true inside Warp with the structured protocol advertised and a non-broken client build (protocol.ts:149, 155).
func supportsStructured() bool {
	return isWarpTerminal() && os.Getenv("WARP_CLI_AGENT_PROTOCOL_VERSION") != "" && !isBrokenVersion(os.Getenv("WARP_CLIENT_VERSION"))
}
