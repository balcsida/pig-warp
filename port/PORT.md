# Port notes

Kind: 1 (Pi TypeScript extensions), ported to Go on the public PiG extension SDK. Proof is layer 1 only: Go tests through a fake PiG host (`fakehost_test.go`, the pigpen template) plus pure unit tests. No `pigeq` differential traces were recorded against Pi, so parity claims rest on the tests and the line citations below, not on golden traces.

## Identities

| What | Value |
|---|---|
| rpiv-warp | https://github.com/juicesharp/rpiv-mono `packages/rpiv-warp`, commit `46dbdfd78ab631bbf3ce41c2618bb173a5ec2725` (2.15.0, MIT) |
| pi-branch-out | https://github.com/w-winter/dot314 `extensions/branch-out/index.ts`, commit `0f39f12239b544225b23a4cb0d7bc1927a0efa5f` (0.1.8, MIT) |
| pi-warp-kitty-images | https://github.com/monotykamary/pi-warp-kitty-images, commit `23225e287a3d0c5d1d0c6f12fad906d48a443c6f` (1.0.8, MIT per README) |
| PiG | `pig 0.5.3+1.4.0.74f46864`, SDK `github.com/MichaelKinsy/PiG/extensions/sdk v0.5.3` |
| Go | go1.27.3 darwin/arm64 |

Vendored originals: `port/oracle/` (byte for byte, with licenses).

## Contract table

Upstream line numbers refer to the vendored files.

| ID | Upstream | Behaviour | Go | Test | Disposition |
|---|---|---|---|---|---|
| W1 | protocol.ts `detectWarpEnvironment`, `supportsStructured` | `TERM_PROGRAM=WarpTerminal`, non-empty `WARP_CLI_AGENT_PROTOCOL_VERSION`, client version not at or below the per-channel broken build | `supportsStructured` | `TestSupportsStructured`, `TestParseWarpVersionAndGate` | mapped |
| W2 | protocol.ts `negotiateProtocolVersion` | `min(env, 1)`, unparseable is 1 | `negotiateProtocolVersion` | `TestNegotiateProtocolVersion` | mapped |
| W3 | index.ts:189 | outside Warp or on a broken build no handler registers | `Extension` | `TestRegistersNothingOutsideWarp`, `TestRegistersNothingOnBrokenBuild` | mapped |
| W4 | index.ts:195 | `session_start` emits only for `reason === "startup"` | notify.go | `TestSessionStartEmitsOnlyOnStartup` | mapped |
| W5 | index.ts:200, 210 | `before_agent_start` captures the prompt; a `custom` run keeps the previous query and skips `session_start`/`prompt_submit` at `agent_start` | notify.go | `TestCustomRunSuppressesAnnouncements` | mapped |
| W6 | index.ts:210 | `agent_start`: `session_start` then `prompt_submit`, spinner start, idle cancel, heartbeat start | notify.go | `TestTurnLifecycle`, `TestIdleTimerCancelledByNextTurn` | mapped |
| W7 | index.ts:219 | `agent_end`: one `tool_complete` per outstanding blocking call, then `stop` with last user/assistant text, spinner stop, heartbeat stop, `idle_prompt` after 300 ms | notify.go | `TestTurnLifecycle`, `TestAbortedBlockingToolDrainedAtAgentEnd` | mapped |
| W8 | index.ts:232, 239 | blocking tool call: `question_asked`, spinner stop, heartbeat pause; its end: `tool_complete` with the captured input, spinner and heartbeat resume | notify.go | `TestBlockingToolBadge`, `TestHeartbeat` | mapped |
| W9 | index.ts:247 | `session_shutdown` cancels timers and stops the spinner | notify.go | `TestShutdownStopsEverything` | mapped |
| W10 | payload.ts | envelope `{v, agent:"pi", event, session_id, cwd, project}` plus per-event fields, 200-char truncation, skill-block collapse | payload.go | `TestPayloadJSON`, `TestTextHelpers` | mapped; truncation counts runes, upstream counts UTF-16 units |
| W11 | title-spinner.ts | push title, PiG's `⠋ ⠙ ⠹ ⠸ ⠼ ⠴ ⠦ ⠧ ⠇ ⠏` every 80 ms (upstream: `⠴ ⠦ ⠖ ⠲` every 160 ms) with ` - <repo>` or ` - <session> - <repo>`, pop on stop; idempotent | spinner.go | `TestTurnLifecycle`, `TestNamedSessionSpinnerSuffix` | diverged |
| W12 | warp-notify.ts | open `/dev/tty`, write, close; every failure swallowed | osc.go | implicit (tests replace the writer) | mapped; Windows stdout path dropped (fuse hazard, untested upstream) |
| W13 | config.ts | `blockingTools`, `heartbeatMs` (0 disables, invalid is default) from `$XDG_CONFIG_HOME` or `~/.config` | config.go | `TestLoadConfig`, `TestHeartbeat` | mapped; directory renamed `rpiv-warp` to `pig-warp` |
| W15 | none | subagent children (`PIG_SUBAGENT`, `TINTINWEB_SUBAGENT_DEPTH`) register no notification handler | `Extension` | `TestSubagentChildRegistersNothing` | added: children share the parent's `/dev/tty` |
| W16 | none | the title names subagents: `subagent`/`Agent` tool start and end, tintinweb's `subagents:created`/`completed`/`failed` bus events; the spinner runs while any is running, past the turn | notify.go | `TestForegroundSubagentInTitle`, `TestBackgroundSubagentKeepsSpinnerPastTurn` | added |
| W17 | none | the title names pigpen-todo's task in progress, from `todo` results' `details.tasks` and, at `agent_start`, the branch's last snapshot | notify.go | `TestTodoTaskInTitle` | added; tintinweb-tasks not followed (no snapshot in its results) |
| W14 | index.ts:70 `createWorkflowQuestionTransport` | run-keyed badge API for rpiv-pi | none | none | excluded: no consumer in PiG |
| B1 | index.ts:484 `parseBranchArgs` | leading `--model <query>`, rest is the message | `parseBranchArgs` | `TestParseBranchArgs` | mapped |
| B2 | index.ts:1343 | `/branch` refuses an unpersisted session and an empty session with notices | branch.go | `TestBranchRefusals` | mapped |
| B3 | index.ts:1352 | fork via `SessionManager.open(...).createBranchedSession(leaf)`, launch entry `branch-out-launch` only with a model query or message | branch.go | `TestBranchLaunchesWarpTabThatResumesTheFork`, `TestBranchQueuesModelAndDraft` | mapped |
| B4 | index.ts:1112 `openInWarp` | Tab Config under `~/.warp/tab_configs` or XDG, `0600`, `rm -f` then resume, reserved-template refusal, `open`/`xdg-open warp://tab_config/<name>`, config removed on failure | `openInWarp` | `TestBranchLaunchesWarpTabThatResumesTheFork`, `TestBranchRejectsReservedTemplateAndFailedOpener` | mapped; resumes with `pig --session`, not `pi`; always tab mode |
| B5 | index.ts:1248 `applyPendingBranchLaunch` | child marks the launch applied, switches model, shows the notice, prefills the draft, starts the countdown | `applyPendingLaunch` | `TestChildAppliesLaunchAndEscCancelsCountdown` | mapped; model switch untested (fake host carries no model) |
| B6 | index.ts:560 `startCountdown`, 531 `autoSubmitDraft` | status line, Esc consumed and cancels, typing cancels, Enter clears silently, submit as follow-up when busy, empty draft warns | `countdown` | `TestTypingCancelsAndEnterKeepsTheDraft`, `TestCountdownSubmitsTheDraft` | mapped |
| B7 | index.ts:1328 | session switch, fork, tree and shutdown clear the countdown | branch.go | `TestSessionSwitchClearsCountdown` | mapped |
| B8 | index.ts:259-482 model resolution | scoring, provider-scoped queries, tie-breaks | branch.go | `TestResolveClosestModel` | mapped; candidates come from `ctx.ScopedModels()` instead of re-reading `enabledModels` settings |
| B9 | index.ts:171, 763-1110, 1167 | other backends, routing, layout state, config file, `--branch-out-terminal` | none | none | excluded: Warp package |
| K1 | pi-warp-kitty-images extensions/index.ts | `getCapabilities()` on `session_start` in TUI mode | none | none | skipped: built in (PiG `tui/terminal_image.go` detects Warp) |

## Findings

- PiG's SDK validates `before_agent_start` payloads: a fired event needs `systemPromptOptions`, which the fake host tests supply.
- The fake host does not fire `session_shutdown` on stop, so tests fire it at cleanup; otherwise an idle timer from one test writes into the next.
