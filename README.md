<div align="right">

**English** | [简体中文](./README.zh-CN.md)

</div>

# cogriaclaw

> A minimalist bridge between WhatsApp and Large Language Models. Lean. Practical. Not bloated.

`cogriaclaw` is a single-binary Go service that wires a WhatsApp account to an LLM and a set of pluggable skills. It does one thing well: receive messages from approved contacts or groups, route them through an LLM with tool-use, run the chosen skill, and reply. A small HTTP API lets external systems push messages or trigger tasks on demand.

## Principles

- **Lean** — single static binary, no runtime dependencies, target under 1k lines of Go for the core
- **Practical** — solves real bot needs (allowlists, group mention gating, tool use, HTTP triggers) and stops there
- **Not bloated** — no plugin marketplaces, no multi-channel abstractions, no "memory framework". If you don't need it, it isn't here

## How it's different

- No Puppeteer or headless browser — uses [whatsmeow](https://github.com/tulir/whatsmeow), a pure-Go implementation of the WhatsApp Web protocol
- One config file. One process. One thing to debug

## Features

- QR-code login on first run, session persisted locally
- Inbound message filtering by contact (E.164) and group (JID), with optional mention-only gate for groups
- LLM dispatch with tool-use; skills are registered as tools the model can call
- HTTP API: send a message directly, or trigger a named task and announce the result to a chat
- Auto-reconnect; deduplicates messages across restarts

## Status

Early development. Architecture is settled; implementation in progress.

## Disclaimer

cogriaclaw is **not affiliated with** WhatsApp, Meta, or Anthropic. It uses the third-party [whatsmeow](https://github.com/tulir/whatsmeow) library to interact with WhatsApp's web protocol; running this software may violate WhatsApp's Terms of Service and could result in account suspension. Provided "as is" without warranty (see [LICENSE](./LICENSE)). Intended for personal, educational, and authorized-automation use only — not for unsolicited mass messaging.

## License

[MIT](./LICENSE)
