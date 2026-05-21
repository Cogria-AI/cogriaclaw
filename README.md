<div align="right">

**English** | [简体中文](./README.zh-CN.md)

</div>

# cogriaclaw

> A minimalist bridge between WhatsApp and Large Language Models. Lean. Practical. Not bloated.

`cogriaclaw` is a single-binary Go service that wires a WhatsApp account to an LLM. It receives messages from approved contacts or groups, routes them through an LLM that can call tools and follow skills, and replies. A small HTTP API lets external systems push messages or trigger tasks on demand.

## Principles

- **Lean** — single static binary, no runtime dependencies, CGO-free
- **Practical** — allowlists, group mention gating, tool use, skills, HTTP triggers — and stops there
- **Not bloated** — no plugin marketplaces, no multi-channel abstractions, no "memory framework". If you don't need it, it isn't here

## How it's different

- No Puppeteer or headless browser — uses [whatsmeow](https://github.com/tulir/whatsmeow), a pure-Go implementation of the WhatsApp Web protocol
- Any OpenAI-compatible LLM (Kimi, Moonshot, DeepSeek, OpenAI, Groq, OpenRouter, local Ollama…) — pick a backend in config, no code change
- nginx-style process control: `reload` config without dropping the connection; self-installs as a launchd/systemd service
- One config file. One process. One thing to debug

## Quick start

Requires Go 1.23+ to build.

```sh
git clone https://github.com/Cogria-AI/cogriaclaw
cd cogriaclaw
go build -o cogriaclaw .

cp config.example.yaml config.yaml   # then edit: allowlist, LLM key, etc.
./cogriaclaw run                      # scan the QR with WhatsApp → Linked Devices
```

On first run a QR code prints in the terminal — scan it from **WhatsApp → Settings → Linked Devices → Link a Device**. The session is saved under `data/`, so later starts connect without a QR.

Message the linked account from an allowlisted number and it replies via the LLM.

### Run it as a service

```sh
./cogriaclaw install      # registers a launchd (macOS) / systemd (Linux) user service
./cogriaclaw status
./cogriaclaw reload       # re-read config without dropping the WhatsApp connection
./cogriaclaw stop
./cogriaclaw uninstall
```

`run` is foreground (logs to the terminal, stops when the terminal closes). `install` runs it in the background under the OS supervisor — survives logout/reboot, restarts on crash. See `cogriaclaw help` for all commands.

## Configuration

Everything lives in one `config.yaml` (see [`config.example.yaml`](./config.example.yaml)). Highlights:

- **`filter`** — `allowed_dms` (E.164 numbers) and `allowed_groups` (group JIDs). Inbound from anyone else is dropped. `group_require_mention` gates group replies to @-mentions.
- **`llm`** — `base_url` + `api_key` + `model` selects any OpenAI-compatible backend. `headers` and `extra_body` cover provider quirks. `${ENV_NAME}` interpolation keeps keys out of the file.
- **`conversation`** — short-term in-memory history per chat; `reset_command` (default `/new`) starts a fresh session. Nothing is persisted.
- **`api`** — optional HTTP control surface (see below); bind to localhost.

## Tools and skills

Two layers:

- **Tools** are function-calling primitives the model invokes directly — `http_get`, plus `read_file` and `run_script` (both jailed to the skills directory). Built in Go.
- **Skills** are `SKILL.md` folders of markdown instructions (+ optional bundled scripts) under `skills/`. The model is shown each skill's name and description; when a request matches, it reads the `SKILL.md` and follows it, using tools to act. This mirrors [Anthropic's Agent Skills](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/overview) progressive-disclosure model.

See [`skills/server-time/`](./skills/server-time/) for a worked example. `run_script` (folder-scoped execution) is opt-in via `skills.exec.enabled`.

## HTTP API

Enable by setting `api.listen` (and a bearer `api.token`). Bind to localhost; put any public exposure behind your own tunnel/proxy.

| Endpoint | Auth | Purpose |
|---|---|---|
| `GET /healthz` | none | liveness + WhatsApp connection status |
| `POST /send` | bearer | send a message directly, bypassing the LLM |
| `POST /trigger` | bearer | run a tool; optionally announce the result to a chat |

```sh
curl -XPOST localhost:8787/send -H "Authorization: Bearer $TOKEN" \
  -d '{"to":"+447700900123","text":"hello"}'

curl -XPOST localhost:8787/trigger -H "Authorization: Bearer $TOKEN" \
  -d '{"tool":"http_get","input":{"url":"https://example.com"},"notify":{"to":"+447700900123"}}'
```

## Documentation

- [Operations Guide](./docs/operations.md) — install, commands, logs, updating, troubleshooting
- [Configuration](./docs/configuration.md) — config reference; changing the LLM model/provider/parameters
- [Tools and Skills](./docs/skills.md) — the tool-vs-skill model, writing a SKILL.md
- [HTTP API](./docs/api.md) — endpoints and triggering tasks

## Disclaimer

cogriaclaw is **not affiliated with** WhatsApp, Meta, or Anthropic. It uses the third-party [whatsmeow](https://github.com/tulir/whatsmeow) library to interact with WhatsApp's web protocol; running this software may violate WhatsApp's Terms of Service and could result in account suspension. Provided "as is" without warranty (see [LICENSE](./LICENSE)). Intended for personal, educational, and authorized-automation use only — not for unsolicited mass messaging.

## License

[MIT](./LICENSE)
