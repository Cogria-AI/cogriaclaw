# Operations Guide

What CogriaClaw does, and how to install, run, and maintain it.

## What it does

- **Inbound** — receives WhatsApp messages, drops anything outside the
  allowlist, routes the rest to an LLM that can call tools and follow skills,
  and replies. Per-chat short-term memory; `/new` starts a fresh session.
- **Outbound / triggered** — an HTTP API lets other systems send a message
  directly or trigger a tool and announce the result to a chat.
- **Single process** — one Go binary, ~19 MB resident, near-0% CPU at idle
  (it's I/O-bound: waiting on WhatsApp and the LLM).

Capabilities: QR login, auto-reconnect, message dedup, DM/group allowlists,
group @-mention gating, LID→phone resolution, any OpenAI-compatible LLM,
tool-use, SKILL.md skills with sandboxed script execution, HTTP control API,
single-instance guard, hot config reload, self-install as a native service.

## First-time setup

From the project directory (after `make build`):

```sh
./cogriaclaw install     # copies binary → ~/.local/bin, config/skills/session → ~/.cogriaclaw, registers a service
```

- **If already logged in** (a session was copied), the service starts
  immediately.
- **If not logged in**, install prints the next steps:
  ```sh
  cogriaclaw run         # scan the QR (WhatsApp → Linked Devices), then Ctrl+C
  cogriaclaw start       # run as a background service
  ```

QR login can't happen inside a background service (there's no terminal to show
the code), so logging in is always a foreground `run` step.

## Where things live

| Path | What |
|---|---|
| `~/.local/bin/cogriaclaw` | the installed binary (on PATH) |
| `~/.cogriaclaw/config.yaml` | active config — **edit this one**, not the project copy |
| `~/.cogriaclaw/data/` | WhatsApp session DB + PID file — **back this up** |
| `~/.cogriaclaw/skills/` | installed skills |
| `~/.cogriaclaw/cogriaclaw.log` | service log (macOS) |

## Commands

| Command | Effect |
|---|---|
| `cogriaclaw run` | foreground; logs to terminal; stops when the terminal closes |
| `cogriaclaw install` | install to home dir + register service |
| `cogriaclaw start` | start the background service (and enable at login/boot) |
| `cogriaclaw stop` | stop it |
| `cogriaclaw restart` | restart it |
| `cogriaclaw reload` | re-read config without dropping the WhatsApp connection (SIGHUP) |
| `cogriaclaw status` | running or not (with PID) |
| `cogriaclaw uninstall` | stop + remove the service |
| `cogriaclaw help` / `version` | — |

Run vs service: `run` is foreground (dev/login). The service (after `install`)
runs in the background under launchd/systemd — survives logout/reboot and
restarts on crash.

## Day-to-day maintenance

**Change config or model** — edit `~/.cogriaclaw/config.yaml`, then:
```sh
cogriaclaw reload
```
Hot-applies filter, LLM, conversation, tools, skills. Changing `api.listen`,
`data.dir`, or the WhatsApp account needs `cogriaclaw restart`.

**Add/edit a skill** — drop or edit a folder in `~/.cogriaclaw/skills/`, then
`cogriaclaw reload`.

**View logs**
```sh
tail -f ~/.cogriaclaw/cogriaclaw.log     # macOS
journalctl --user -u cogriaclaw -f        # Linux
```

**Update to a new version**
```sh
cd <project> && git pull && make build
./cogriaclaw install      # refreshes the installed binary; service restarts
```

**Back up / migrate** — copy `~/.cogriaclaw/` (config + session) to the new host.
With the session present, `install` there starts without a QR re-scan.

## Troubleshooting

- **`status` says "not running" right after install** — on macOS, a LaunchAgent
  can't execute a binary on an external/`noowners` volume (fails with exit code
  78). `install` avoids this by copying the binary to `~/.local/bin` on the
  internal disk. If you registered a service by hand pointing at `/Volumes/...`,
  `uninstall` and re-`install`.
- **`command not found: cogriaclaw`** — `~/.local/bin` isn't on your PATH. Add:
  ```sh
  echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc && source ~/.zshrc
  ```
- **"already running (pid N)"** — the single-instance guard. An instance is
  already up; use `reload`/`stop`/`status` instead of starting another.
- **Logged out / session expired** — `cogriaclaw stop`, then `cogriaclaw run` to
  re-scan the QR, then `cogriaclaw start`.
- **Control command can't find the instance** — control commands resolve config
  from `~/.cogriaclaw/config.yaml` (installed) or `./config.yaml`. Pass `-config`
  if you use a non-standard location.
