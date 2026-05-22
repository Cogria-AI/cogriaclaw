# Configuration

CogriaClaw reads a single YAML file. When run without `-config`, it uses
`~/.cogriaclaw/config.yaml` if present (the installed location), otherwise
`./config.yaml`. Start from [`config.example.yaml`](../config.example.yaml).

After editing the config of a running instance, apply it without dropping the
WhatsApp connection:

```sh
cogriaclaw reload
```

`reload` hot-applies `filter`, `llm`, `conversation`, `tools`, and `skills`.
Changing `api.listen`, `data.dir`, or the WhatsApp account needs a full restart.

## Environment interpolation

Any `${ENV_NAME}` in `llm.api_key`, `llm.base_url`, `api.token`, or under
`tools.*.config` is replaced with that environment variable. Keep secrets in the
environment instead of the file:

```yaml
llm:
  api_key: ${KIMI_API_KEY}
```

---

## `llm` — the language model

CogriaClaw talks to any **OpenAI Chat Completions-compatible** endpoint. You
switch backends entirely through config — no code change.

```yaml
llm:
  base_url: https://api.kimi.com/coding/v1   # empty = OpenAI's default endpoint
  api_key: ${KIMI_API_KEY}
  model: kimi-for-coding
  headers:                                   # optional extra request headers
    User-Agent: KimiCLI/0.77
  extra_body:                                # optional provider-specific body fields
    thinking:
      type: disabled
  max_tokens: 4096
  max_tool_hops: 5                           # tool-use rounds per message (1–20)
  system_prompt: |
    You are an assistant operating inside a WhatsApp chat. Be concise.
```

| Field | Meaning |
|---|---|
| `base_url` | The provider's OpenAI-compatible endpoint. Empty = OpenAI default. |
| `api_key` | Bearer key; supports `${ENV_NAME}`. |
| `model` | Model ID as the provider names it. |
| `headers` | Extra HTTP headers sent on every request. |
| `extra_body` | Extra JSON fields merged into every request body. |
| `max_tokens` | Cap on the response. Reasoning models also spend this on hidden reasoning — keep it generous. |
| `max_tool_hops` | How many tool-call rounds the model may take before a forced final answer. |
| `system_prompt` | Base instructions. The skills catalogue is appended automatically. |

### Switching provider

Only `base_url` + `model` + `api_key` change. Base URLs are stable; check each
provider's docs for current model IDs.

| Provider | `base_url` | example `model` |
|---|---|---|
| OpenAI | *(empty)* | `gpt-4o` |
| Kimi (coding) | `https://api.kimi.com/coding/v1` | `kimi-for-coding` |
| Moonshot | `https://api.moonshot.cn/v1` | see Moonshot platform |
| DeepSeek | `https://api.deepseek.com/v1` | `deepseek-chat` |
| OpenRouter | `https://openrouter.ai/api/v1` | `moonshotai/kimi-k2.5` |
| Groq | `https://api.groq.com/openai/v1` | a Groq-hosted model |
| Ollama (local) | `http://localhost:11434/v1` | `llama3.1` |

### `headers` and `extra_body` — provider quirks

Some providers need extras that aren't standard OpenAI fields:

- **Kimi's coding endpoint** gates non-coding-agent clients — it requires
  `User-Agent: KimiCLI/0.77`. It's also a reasoning model whose tool-call turns
  must round-trip `reasoning_content`; setting `extra_body.thinking.type:
  disabled` avoids that and gives snappier chat replies.
- Other reasoning models expose their own toggles (`reasoning_effort`,
  `enable_thinking`, …) — put them under `extra_body`.

---

## `whatsapp`

```yaml
whatsapp:
  device_name: cogriaclaw   # label shown in WhatsApp → Linked Devices
```

## `filter` — who the bot listens to

```yaml
filter:
  allowed_dms:
    - "+447700900123"          # E.164; whitespace/hyphens ignored
  allowed_groups:
    - "120363012345678901@g.us"  # full group JID
  group_require_mention: false   # true = only reply in groups when @-mentioned
```

The bot starts only if at least one of `allowed_dms` / `allowed_groups` is set —
it refuses a fully-open inbound. Messages from anyone else are dropped. To find a
group's JID, set `log_level: debug` and read the `drop reason=group-not-in-allowlist`
line.

## `conversation` — short-term memory

```yaml
conversation:
  enabled: true
  reset_command: "/new"     # send this in chat to start a fresh session
  max_turns: 0              # 0 = unlimited until reset; N caps growth
  idle_ttl_minutes: 0       # 0 = never auto-expire
```

History is per-chat and in-memory only — a restart clears every session.

## `api`, `tools`, `skills`

- `api` — the HTTP control surface. See [api.md](./api.md).
- `tools` / `skills` — function-calling primitives and SKILL.md folders. See
  [skills.md](./skills.md).

## `data` and `log_level`

```yaml
log_level: info     # debug | info | warn | error
data:
  dir: data         # session DB + pid file; resolved relative to this config file
```
