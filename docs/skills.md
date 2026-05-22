# Tools and Skills

CogriaClaw separates two things that are often conflated:

- **Tools** are function-calling primitives the model invokes directly. They're
  Go code. Built in: `http_get`, `read_file`, `run_script`.
- **Skills** are `SKILL.md` folders of markdown instructions (plus optional
  bundled scripts/docs). The model is shown each skill's name and description;
  when a request matches, it reads the `SKILL.md` and follows it, **using tools**
  to act.

In short: tools are the building blocks; skills are recipes written in markdown
that tell the model how to combine them. This mirrors
[Anthropic's Agent Skills](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/overview).

## How a skill runs (progressive disclosure)

1. **Always loaded** — every skill's `name` + `description` is injected into the
   system prompt at startup (cheap; the model just knows the skill exists).
2. **On match** — when a request matches a description, the model calls
   `read_file` on that skill's `SKILL.md` to load the full instructions.
3. **On demand** — the instructions may tell it to read more bundled files
   (`read_file`) or execute bundled scripts (`run_script`); only the output
   enters the context.

## Writing a skill

Create a folder under the skills directory (`~/.cogriaclaw/skills/` once
installed, or `./skills/` in the project) with a `SKILL.md`:

```
skills/
  weather/
    SKILL.md
    scripts/
      fetch.py        # optional
    REFERENCE.md      # optional bundled doc
```

`SKILL.md` needs YAML frontmatter (`name`, `description`) followed by markdown
instructions:

```markdown
---
name: weather
description: Get the current weather for a city. Use when the user asks about weather or temperature.
---

# Weather

1. Run the fetch script: `run_script` with path `weather/scripts/fetch.py` and
   args `["<city>"]`.
2. Summarize the JSON it prints in one sentence, in the user's language.
```

- **`description` is the trigger** — write what it does *and when to use it*. The
  model decides whether to load the skill based on this line alone.
- Reference bundled files and scripts by their path **relative to the skills
  root** (e.g. `weather/scripts/fetch.py`), since that's what `read_file` /
  `run_script` expect.

See [`skills/server-time/`](../skills/server-time/) for a complete example.

Drop a new skill folder in and run `cogriaclaw reload` — no rebuild needed.

## The skill-access tools

| Tool | What it does | Notes |
|---|---|---|
| `read_file` | Read a file under the skills root | Read-only, jailed to the skills directory. Always available when skills exist. |
| `run_script` | Execute a script bundled in a skill folder | Jailed to the skills directory, with a timeout and output cap. **Opt-in** via `skills.exec.enabled`. |

`run_script` chooses an interpreter by extension (`.sh`→bash, `.py`→python3,
`.js`→node, `.rb`→ruby, `.pl`→perl) or runs the file directly if it's
executable.

## Configuration

```yaml
tools:
  http_get:
    enabled: true
    config:
      user_agent: cogriaclaw/0.1
      timeout_sec: 10
      max_bytes: 4096

skills:
  dir: skills          # resolved relative to the config file
  exec:
    enabled: true      # allow run_script (it executes code — see security note)
    timeout_sec: 30
    max_output_bytes: 8192
```

## Adding a built-in tool

Tools that aren't skills (a CRM lookup, a database query) are Go code:

1. Add a file in `internal/tool/` with a factory `func NewX(raw map[string]any)
   (Tool, error)` returning a `Tool{Name, Description, InputSchema, Run}`. Read
   any secrets from `raw` inside the factory — they stay in the closure and
   never reach the model.
2. Register it in `main.go`'s `toolFactories` map.
3. Enable it under `tools:` in the config.

See `internal/tool/http_get.go` for the pattern.

## Security

`run_script` executes code. A script bundled in a skill can do anything its
interpreter allows. Only install skills you trust — the same assumption
Anthropic's Agent Skills make. The sandboxing here limits *which* files can be
run (jailed to the skills directory) and for how long (timeout), **not** what a
trusted script does once started. If you don't need script execution, leave
`skills.exec.enabled: false`; `read_file` (read-only) still works.
