---
name: mq-health-check
version: 1.0.0
status: release
risk: medium
requires_consent: false
allowed_tools:
  - shell.readonly
  - file.read
  - http.get
---

# Skill: MQ Health Check

## Goal
Check MQ or messaging platform health and report abnormal conditions.

## Safe Checks
Use read-only checks only.

Suggested read-only commands if applicable:

- `date`
- `uptime`
- `ps aux | grep -i mq`
- `df -h`
- `tail -100 ./logs/mq-error.log`

## Decision Rules

- If all checks look normal, status should be `OK`.
- If logs show errors, queue depth is high, or a channel appears unhealthy, status should be `Warning`.
- If a production-impacting outage is detected, status should be `Critical`.

## Manual Intervention

Do not restart queue managers, channels, containers, or services directly.
If restart or configuration change is needed, create a proposed high-risk action and ask for consent.

## Report Format

Return findings, risk level, recommended action, and whether consent is required.
