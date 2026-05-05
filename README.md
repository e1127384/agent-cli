# Local CLI Agent

A local-first CLI agent for macOS / MacBook M4 that:

- uses a local Qwen/Qwen3 API on `localhost`
- loads release skills from markdown files
- runs safe tools through a registry
- monitors tasks continuously using configurable intervals
- writes reports and audit logs
- asks consent before risky/manual actions

## Run

```bash
go mod tidy
go run ./cmd/agent --help
```

## Commands

```bash
go run ./cmd/agent skill list
go run ./cmd/agent skill run mq-health-check
go run ./cmd/agent run "check MQ health"
go run ./cmd/agent monitor start
go run ./cmd/agent monitor run-once
go run ./cmd/agent monitor run mq-health-check
```

## Build

```bash
go build -o agent ./cmd/agent
./agent monitor start
```

## Configure monitoring interval

Edit `config.yaml`:

```yaml
tasks:
  - name: "mq-health-check"
    skill: "mq-health-check"
    enabled: true
    interval: "1m"
```

Supported examples: `30s`, `1m`, `5m`, `15m`, `1h`.

## Skill format

Skills live in `skills/release/*.md`.

Each skill uses YAML front matter followed by instructions.

```markdown
---
name: mq-health-check
version: 1.0.0
status: release
risk: medium
requires_consent: false
allowed_tools:
  - shell.readonly
  - http.get
  - file.read
---

# Skill: MQ Health Check
...
```

## Safety design

The LLM can suggest. The Go runtime decides. Tools execute only through an allowlisted registry. High-risk actions require human consent.
