# Keeping model aliases current

## Problem

`GET /v1/models` without query params lists **YAML aliases only**. If aliases point at retired vendor ids, clients (Cursor, Claude Code) see garbage.

## Solutions

### 1. Live list (runtime — preferred for clients)

With the gateway running and subscription credentials loaded:

```bash
curl -sk -H "Authorization: Bearer local-dev" \
  https://127.0.0.1:8787/v1/models?live=1 | jq -r '.data[].id' | sort
```

This merges config aliases with each provider’s live `GET /models` (openai / openai_compat / anthropic). Use these full `provider/id` values in clients, or retarget short aliases.

### 2. Refresh script

```bash
./examples/scripts/refresh-model-catalog.sh
```

### 3. Human / agent update of example YAML

Follow **AGENTS.md → Model aliases must stay current**: check vendor docs, update `examples/configs/claude-code-subscriptions.yaml` (and multi), helpers, website guides, date stamp.

Do not invent model ids from memory.
