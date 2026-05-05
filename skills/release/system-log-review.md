---
name: system-log-review
version: 1.0.0
status: release
risk: low
requires_consent: false
allowed_tools:
  - shell.readonly
  - file.read
---

# Skill: System Log Review

## Goal
Review local system condition and recent logs using safe read-only operations.

## Safe Checks

Suggested commands:

- `date`
- `uptime`
- `df -h`
- `ps aux | head -20`

## Decision Rules

- Report `OK` if there are no obvious issues.
- Report `Warning` if disk, CPU, memory, or logs show abnormal patterns.
- Never perform cleanup or deletion. If cleanup is required, ask for consent.
