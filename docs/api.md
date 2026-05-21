# HTTP API

An optional HTTP control surface so external systems can push messages or
trigger tools on demand — without going through the LLM. This is how you wire
cogriaclaw into other backends: "on event X, make the bot do Y and tell someone
in WhatsApp."

## Enabling

```yaml
api:
  listen: "127.0.0.1:8787"        # empty = API disabled
  token: ${COGRIACLAW_API_TOKEN}  # required when listen is set
```

cogriaclaw refuses to start if `listen` is set without a `token`. **Bind to
localhost.** To reach it from outside, put it behind your own tunnel or reverse
proxy (Cloudflare Tunnel, frp, nginx) — that's deliberately out of scope here.

`/send` and `/trigger` require `Authorization: Bearer <token>`. `/healthz` does
not.

## Endpoints

### `GET /healthz`

Liveness + WhatsApp connection status. No auth.

```sh
curl localhost:8787/healthz
```
```json
{ "ok": true, "connected": true, "self": "447843199974", "uptime_s": 123 }
```

### `POST /send`

Send a message directly, bypassing the LLM. Bearer auth.

| Field | Required | Notes |
|---|---|---|
| `to` | yes | E.164 (`+447700900123`) or group JID (`...@g.us`) |
| `text` | yes | message body |

```sh
curl -XPOST localhost:8787/send \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"to":"+447700900123","text":"hello"}'
```
```json
{ "ok": true, "to": "447700900123@s.whatsapp.net" }
```

### `POST /trigger`

Run a named tool and (optionally) announce the result to a chat. Bearer auth.
This runs a **tool** directly (not the LLM); the tool's string result is
returned, and if `notify.to` is given, also sent to that chat.

| Field | Required | Notes |
|---|---|---|
| `tool` | yes | a registered tool name (e.g. `http_get`) |
| `input` | no | JSON object matching the tool's input schema (default `{}`) |
| `notify.to` | no | E.164 or group JID to send the result to |

```sh
curl -XPOST localhost:8787/trigger \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
        "tool": "http_get",
        "input": { "url": "https://example.com" },
        "notify": { "to": "+447700900123" }
      }'
```
```json
{ "ok": true, "result": "HTTP 200 OK\n...", "notified": true }
```

## Responses and errors

All responses are JSON. Errors carry `{ "ok": false, "error": "..." }` with an
HTTP status:

| Status | When |
|---|---|
| 400 | bad/invalid JSON body, missing required field, bad `to` |
| 401 | missing/wrong bearer token |
| 404 | unknown tool (`/trigger`) |
| 500 | tool failed |
| 502 | WhatsApp send failed (`/send`) |
| 503 | protected endpoint hit but no token configured |

The request body is capped at 1 MiB and unknown JSON fields are rejected.

## Example: trigger a task from another service

A cron job or webhook handler can have the bot run a tool and report into a
group:

```sh
curl -XPOST localhost:8787/trigger \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"tool":"http_get","input":{"url":"https://status.internal/summary"},
       "notify":{"to":"120363012345678901@g.us"}}'
```
