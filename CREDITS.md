# Credits

pig-warp is a Go port for [PiG](https://github.com/MichaelKinsy/PiG) of three Pi extensions. The unmodified originals are vendored under `port/oracle/`; the Go code is a rewrite of their observable behaviour, not a copy.

## @juicesharp/rpiv-warp

Upstream: https://github.com/juicesharp/rpiv-mono (packages/rpiv-warp), commit `46dbdfd78ab631bbf3ce41c2618bb173a5ec2725`, by juicesharp, MIT.

Ported: Warp detection and the broken-build gate, OSC 777 payloads for every lifecycle event, the tab-title spinner, the heartbeat and idle timers, the blocking-tool badge and its ESC drain, and the config file. Not ported: `createWorkflowQuestionTransport` (an export for rpiv-pi, which has no PiG counterpart) and the Windows stdout transport.

```
MIT License

Copyright (c) 2026 juicesharp

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

## pi-branch-out

Upstream: https://github.com/w-winter/dot314 (extensions/branch-out), commit `0f39f12239b544225b23a4cb0d7bc1927a0efa5f`, by w-winter, MIT; itself derived from davidgasquez/dotfiles `branch-term.ts`.

Ported: the `/branch [--model <query>] [message]` command, the Warp Tab Config launch, model resolution, and the child-side launch entry with the auto-submit countdown. Not ported: the cmux, tmux, iTerm2, Terminal.app, Ghostty, Orca and Alacritty backends, the rotating layout policies, `config.json` and the `--branch-out-terminal` flag.

```
Derived from davidgasquez/dotfiles (https://github.com/davidgasquez/dotfiles/blob/main/agents/pi/extensions/branch-term.ts)
Renamed and substantially extended as extensions/branch-out/ in this repository.

The MIT License (MIT)

Copyright (c) 2016 David Gasquez

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

## pi-warp-kitty-images

Upstream: https://github.com/monotykamary/pi-warp-kitty-images, commit `23225e287a3d0c5d1d0c6f12fad906d48a443c6f`, by monotykamary, MIT (per its README; the repository ships no LICENSE file).

Nothing to port: Pi 1.0 and PiG both detect Warp natively for Kitty images, true color and hyperlinks, and the upstream extension is a compatibility no-op. It is credited because this package replaces it in a Warp setup.
