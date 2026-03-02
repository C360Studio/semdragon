# Getting Started

Semdragons runs as a single Go binary backed by NATS JetStream. The dashboard is a SvelteKit app
served separately. Docker Compose is the fastest path to a running system.

## Prerequisites

```text
Go 1.25+            go version
Docker + Compose    docker --version && docker compose version
Node.js 22+         node --version
revive              go install github.com/mgechev/revive@latest
goimports           go install golang.org/x/tools/cmd/goimports@latest
NATS CLI (optional) brew install nats-io/nats-tools/nats
```

## Quick Start with Docker Compose

The `ui/docker-compose.yml` starts three containers:

| Service | Image | Ports | Purpose |
|---------|-------|-------|---------|
| nats | nats:2.12-alpine | 4222, 8222 | NATS JetStream (message broker + KV) |
| backend | Multi-stage Go build | 8080 | REST API + processors |
| ui | Node 22 dev server | 5173 | SvelteKit dashboard |

```bash
cd ui
docker compose up -d

# Verify
docker compose ps
curl http://localhost:8080/health
open http://localhost:5173
```

Environment variables the compose file reads:

- `SEED_E2E` — set to `true` to pre-populate agents, quests, and guilds for testing
- `SEMDRAGONS_API_KEY` — auth key for write endpoints (empty = dev mode, no auth required)

After startup:

- Dashboard: `http://localhost:5173`
- REST API: `http://localhost:8080/api/game/`
- NATS monitor: `http://localhost:8222`

## Running Services Individually

For backend development, run each service outside Docker.

### 1. Start NATS

```bash
# Using the included config (JetStream enabled, port 4222, monitor 8222)
nats-server -c ui/nats-server.conf

# Or via Docker alone
cd ui && docker compose up nats -d
```

### 2. Start the Backend

```bash
go run ./cmd/semdragons --log-format=text --log-level=debug
```

CLI flags:

| Flag | Env var | Default | Description |
|------|---------|---------|-------------|
| `--config`, `-c` | `SEMDRAGONS_CONFIG` | `config/semdragons.json` | Config file path |
| `--log-level` | `SEMDRAGONS_LOG_LEVEL` | `info` | debug, info, warn, error |
| `--log-format` | `SEMDRAGONS_LOG_FORMAT` | `json` | json or text |
| `--shutdown-timeout` | — | `30s` | Graceful shutdown timeout |
| `--validate` | — | — | Validate config and exit |
| `--version` | — | — | Print version and exit |

NATS connection (env vars only, never in config files):

| Variable | Purpose |
|----------|---------|
| `SEMDRAGONS_NATS_URLS` | NATS server URLs (comma-separated) |
| `SEMSTREAMS_NATS_URLS` | Fallback NATS URLs |
| `SEMDRAGONS_NATS_USER` + `SEMDRAGONS_NATS_PASS` | Username/password auth |
| `SEMDRAGONS_NATS_TOKEN` | Token auth |

Priority: `SEMDRAGONS_NATS_URLS` > `SEMSTREAMS_NATS_URLS` > config file `nats.urls` >
`nats://localhost:4222`.

### 3. Start the Frontend

```bash
cd ui
npm install     # first time only
npm run dev     # Vite dev server at http://localhost:5173
```

Vite proxies these paths to the backend (default `http://localhost:8080`):

- `/game` — REST API
- `/health` — Health check
- `/message-logger` — SSE event stream

Set `BACKEND_URL` to override the proxy target.

## Configuration

The default config file is `config/semdragons.json`. Structure:

```json
{
  "platform": { "org": "local", "id": "dev", "environment": "development" },
  "nats": { "urls": ["nats://localhost:4222"] },
  "services": { ... },
  "streams": { "GRAPH": { ... } },
  "components": { ... }
}
```

**Active components in the default config:**

- `graph-ingest`, `graph-index`, `graph-query` — semstreams entity persistence
- `questboard` — quest lifecycle state machine
- `bossbattle` — automated review evaluation
- `agent_progression` — XP calculation and leveling
- `agent_store` — XP marketplace (tools, consumables)
- `guildformation` — auto guild clustering
- `boidengine` — emergent quest-claiming suggestions

**Opt-in components** (registered but not in the default config):

- `executor` — LLM-powered quest execution (requires `ANTHROPIC_API_KEY` or equivalent)
- `autonomy` — heartbeat-driven agent decision loop (depends on executor)
- `seeding` — environment bootstrapping (requires explicit config)
- `dmsession`, `dmapproval`, `dmpartyformation` — DM session management

The KV bucket is named `semdragons-{org}-{platform}-{board}`. With the default config:
`semdragons-local-dev-board1`.

## Running Tests

```bash
make test                     # Unit tests only (no Docker)
make test-integration         # Integration tests (requires Docker — NATS via testcontainers)
make test-all                 # All tests (unit + integration)
make test-one TEST=TestName   # Specific unit test
make test-one-integration TEST=TestName  # Specific integration test
make test-race                # All tests with race detector
make lint                     # revive + go vet
make check                    # fmt + tidy + lint + test-all
make coverage                 # HTML coverage report → coverage.html
```

Frontend tests:

```bash
cd ui
npm run test                  # Vitest unit tests
npm run check                 # TypeScript + Svelte type-check
npm run lint                  # ESLint
```

## First Quest Walkthrough

Verify the system works by driving a quest through its full lifecycle with curl. Start the backend
first (Docker Compose or individual services).

```bash
# 1. Recruit an agent
curl -s -X POST http://localhost:8080/api/game/agents \
  -H "Content-Type: application/json" \
  -d '{"name": "Aria", "skills": ["code_gen"]}' | jq .

# Save the agent ID from the response — it looks like:
# "id": "local.dev.game.board1.agent.abc123"
```

```bash
# 2. Post a quest
curl -s -X POST http://localhost:8080/api/game/quests \
  -H "Content-Type: application/json" \
  -d '{"objective": "Write a hello world function"}' | jq .

# Save the quest ID from the response — it looks like:
# "id": "local.dev.game.board1.quest.xyz789"
```

```bash
# 3. Claim the quest (use the full IDs from steps 1 and 2)
curl -s -X POST http://localhost:8080/api/game/quests/{QUEST_ID}/claim \
  -H "Content-Type: application/json" \
  -d '{"agent_id": "{AGENT_ID}"}'
```

```bash
# 4. Start the quest
curl -s -X POST http://localhost:8080/api/game/quests/{QUEST_ID}/start
```

```bash
# 5. Submit a result
curl -s -X POST http://localhost:8080/api/game/quests/{QUEST_ID}/submit \
  -H "Content-Type: application/json" \
  -d '{"output": "func hello() string { return \"hello world\" }"}'
```

```bash
# 6. Check world state
curl -s http://localhost:8080/api/game/world | jq .
```

When `SEMDRAGONS_API_KEY` is set, write endpoints (POST) require the header
`X-API-Key: <value>`. In dev mode (empty key), auth is skipped.

## E2E Tests

E2E tests use Playwright and require both backend and frontend running.

```bash
# Full lifecycle: start containers → run tests → tear down
make e2e

# Interactive debugging with Playwright UI
make e2e-ui

# Headed mode (shows browser)
make e2e-headed

# Install Playwright browsers (first time)
make e2e-install
```

The 12 spec files in `ui/e2e/specs/` cover agent lifecycle, quest lifecycle, boss battles, SSE
events, store purchases, tier gates, world state, and navigation. Backend-dependent specs
auto-skip when no backend is reachable.

## Debugging

### NATS CLI

```bash
# Watch all quest state changes
nats kv watch semdragons-local-dev-board1 "quest.state.>"

# Watch a specific entity
nats kv watch semdragons-local-dev-board1 "agent.state.{id}"

# Subscribe to quest lifecycle events
nats sub "quest.lifecycle.>"

# List all keys in the board bucket
nats kv ls semdragons-local-dev-board1
```

### Message Logger (HTTP)

The backend exposes an SSE endpoint for live KV watching:

```text
GET http://localhost:8080/message-logger/kv/semdragons-local-dev-board1/watch?pattern=*
```

The Vite dev proxy routes `/message-logger` to the backend, so the dashboard SSE also uses
this path.

### Common Issues

| Symptom | Cause | Fix |
|---------|-------|-----|
| "bucket not found" on first request | KV bucket created on first entity write | Post a quest or recruit an agent first |
| Agent can't claim quest | Tier too low for quest difficulty | Check `min_tier` on quest vs agent `tier` |
| 503 on store endpoints | `agent_store` component not running | Ensure it's enabled in config |
| SSE not connecting | Backend not running or Vite proxy misconfigured | Check `BACKEND_URL` and that `/message-logger` responds |
| Quest stuck in `in_review` | `bossbattle` processor not running | Ensure it's enabled in config |
