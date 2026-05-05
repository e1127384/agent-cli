---
name: local-api-health
version: 1.0.0
status: release
risk: low
requires_consent: false
allowed_tools:
  - http.get
  - shell.readonly
---

# Skill: Local API Health

## Goal
Check whether a local HTTP API is reachable and healthy.

## Safe Checks

Use `http.get` for health endpoints such as:

- `http://localhost:8000/health`
- `http://localhost:8000/v1/models`

## Decision Rules

- If API responds successfully, status is `OK`.
- If API is unreachable or slow, status is `Warning`.
- If the API is required for production workflow and unavailable, status is `Critical`.

## Manual Intervention

If service restart is needed, ask for consent before any restart command.
