# pig-warp

[Warp](https://www.warp.dev/) terminal integration for [PiG](https://github.com/MichaelKinsy/PiG), as one native Go extension. It combines three Pi extensions:

- [`@juicesharp/rpiv-warp`](https://github.com/juicesharp/rpiv-mono/tree/main/packages/rpiv-warp): Warp tells you when PiG needs you. An OS toast when a turn ends or a question is waiting, a Blocked badge on the tab, and an animated tab title while a turn runs.
- [`pi-branch-out`](https://github.com/w-winter/dot314/tree/main/packages/pi-branch-out), Warp backend: `/branch` forks the current session into a new Warp tab, optionally with another model and a queued first message.
- [`pi-warp-kitty-images`](https://github.com/monotykamary/pi-warp-kitty-images): nothing to port. PiG detects Warp natively and enables Kitty images, true color and hyperlinks itself.

Outside Warp the notifications register nothing and write nothing. Installing it everywhere costs nothing.

## Install

```sh
pig install git:github.com/balcsida/pig-warp
```

Restart your PiG session inside Warp. Go is needed to build the extension from source.

## Notifications

Six lifecycle moments reach Warp as `OSC 777` cli-agent notifications: session start, prompt submitted, question asked, tool finished, turn stopped, gone idle. Each carries the project name, your query and the assistant's reply, so the toast reads `Pi needs attention — Waiting for your answer` or quotes the finished reply.

- The tab title shows a braille spinner for as long as the turn runs (`⠴ - <repo>`, or `⠴ - <session> - <repo>` for a named session), then PiG's own title comes back.
- A blocking tool (`ask_user_question` by default) lights Warp's Blocked badge; an ESC-aborted question is drained at turn end so the badge never sticks.
- The running prompt is re-announced every 15 seconds so Warp never marks a busy tab idle.

Warp builds at or below `v0.2026.3.25.8.24.stable_5` (stable and preview) advertise the protocol but do not render notifications; they are gated off until you upgrade.

### Configuration

Optional. Create `~/.config/pig-warp/config.json` (or `$XDG_CONFIG_HOME/pig-warp/config.json`):

```json
{ "blockingTools": ["ask_user_question", "my_custom_tool"], "heartbeatMs": 5000 }
```

| Key | What it does | Default |
| --- | --- | --- |
| `blockingTools` | Tool names that raise the Blocked badge when called and clear it when they finish. `[]` turns the badge off. | `["ask_user_question"]` |
| `heartbeatMs` | How often the running prompt is re-announced. `0` disables it. | `15000` |

A missing or malformed file means the defaults. The file is read once when the extension loads.

## `/branch`

```text
/branch [--model <query>] [message]
```

Forks the session at its current leaf and opens the fork in a new Warp tab in the same directory, through a one-shot [Tab Config](https://docs.warp.dev/terminal/windows/tab-configs/) that the new tab deletes as it starts. The parent keeps running.

- `--model <query>`: the child's model. A full `provider/model-id`, or a fuzzy query scored against the session's scoped models (or the current provider's models when no scope is set). An unresolvable query keeps the parent's model and says so.
- `message`: prefilled in the child's editor with a 10 second auto-submit countdown shown in the status line. Typing or `Esc` cancels it, `Enter` sends immediately.

Requirements: Warp on macOS or Linux on the same machine as PiG, a persisted session (not `--no-session`), at least one message, and `pig` on the new tab's `PATH`. If a launch never opens a tab, delete the leftover `tmp_pi_branch_*.toml` from `~/.warp/tab_configs/` (macOS) or `${XDG_DATA_HOME:-$HOME/.local/share}/warp-terminal/tab_configs/` (Linux).

## What is not ported

- rpiv-warp's `createWorkflowQuestionTransport`, an export for `@juicesharp/rpiv-pi` workflow runs, which have no PiG counterpart, and its best-effort Windows stdout transport. On Windows nothing is written.
- pi-branch-out's other terminal backends (cmux, tmux, iTerm2, Terminal.app, Ghostty, Orca, Alacritty), its rotating split layouts, its `config.json` and the `--branch-out-terminal` flag. This is a Warp package; inside Warp, run PiG in tmux if you want splits.

## Development

```sh
cd extensions/pig-warp
go test -race ./...
```

`go test` resolves the PiG SDK through a local, git-ignored `go.work` that points at the SDK `pig extension init` stages under `$PIG_HOME/state/pigsdk`. Validate the package with `pig install ./extensions/pig-warp --validate-only --json` and `pig package validate .`. The upstream sources are vendored unchanged under `port/oracle/`; `port/PORT.md` maps every ported behaviour to its upstream line.

## License

MIT. See [LICENSE](LICENSE) and [CREDITS.md](CREDITS.md) for the upstream authors and licenses.
