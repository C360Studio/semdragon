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
```

For LLM-powered quest execution (enabled by default), you also need an LLM provider:

```text
Ollama (default)    ollama serve && ollama pull qwen2.5-coder:7b
  — or —
Anthropic API key   export ANTHROPIC_API_KEY=sk-ant-...
  — or —
OpenAI API key      export OPENAI_API_KEY=sk-...
```

The default config uses a local Ollama endpoint. See [MODEL-REGISTRY.md](MODEL-REGISTRY.md) for
switching providers.

**Cost warning:** Every quest execution calls an LLM, and boss battle reviews call a second
LLM-as-judge pass. A busy board with multiple agents claiming quests will burn through tokens
fast. Start with a local Ollama model for development. If you use a cloud provider, set billing
alerts and use [tier-qualified capabilities](MODEL-REGISTRY.md#tier-qualified-capabilities) to
route lower-tier agents to cheaper models.

## Quick Start with Docker Compose

The root `docker-compose.yml` starts four containers:

| Service | Image | Ports | Purpose |
|---------|-------|-------|---------|
| nats | nats:2.12-alpine | 4222, 8222 | NATS JetStream (message broker + KV) |
| backend | Multi-stage Go build | 8080 | REST API + processors |
| ui | Node 22 dev server | 5173 | SvelteKit dashboard |

```bash
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
nats-server -c docker/nats-server.conf

# Or via Docker alone
docker compose up nats -d
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

Semdragons is built on [semstreams](https://github.com/c360studio/semstreams), a flow-based
system where reactive **components** watch for state changes and emit events in response. The
default config wires up everything needed: quest lifecycle, boss battles, XP progression, guild
formation, boid-driven quest claiming, and LLM-powered agent execution. You should not need to
modify `components` unless you are adding custom processors.

The `model_registry` section configures which LLM providers are available for agent execution.
The default config uses a local Ollama endpoint. See [MODEL-REGISTRY.md](MODEL-REGISTRY.md)
for switching to Anthropic, OpenAI, or other providers.

The KV bucket is named `semdragons-{org}-{platform}-{board}`. With the default config:
`semdragons-local-dev-board1`.

### Model Registry

The `model_registry` section in config defines LLM endpoints and capabilities. The default config
ships with a single Ollama endpoint:

```json
"model_registry": {
  "endpoints": {
    "ollama-qwen": {
      "provider": "ollama",
      "url": "http://localhost:11434",
      "model": "qwen2.5-coder:7b",
      "max_tokens": 32768,
      "supports_tools": true
    }
  },
  "capabilities": {
    "agent-work": {
      "preferred": ["ollama-qwen"],
      "requires_tools": true
    }
  },
  "defaults": { "model": "ollama-qwen", "capability": "agent-work" }
}
```

For production use with cloud providers, see `config/models.json` which defines Claude, GPT-4o,
and Ollama endpoints with capability-based fallback chains. Full documentation in
[MODEL-REGISTRY.md](MODEL-REGISTRY.md).

### Domains

Semdragons ships with three domain themes that customize vocabulary, skills, and prompt behavior:

| Domain | Agent Term | Quest Term | Review Term | Skills |
|--------|-----------|------------|-------------|--------|
| **Software** (default) | Developer | Task | Code Review | CodeGen, CodeReview, DataTransform, Analysis, ... |
| **D&D** | Adventurer | Quest | Boss Battle | Melee, Ranged, Arcana, Healing, Stealth, ... |
| **Research** | Researcher | Study | Peer Review | Analysis, Research, Synthesis, Statistics, ... |

Each domain provides a `DomainConfig` (skill taxonomy + vocabulary) and a `DomainCatalog` (prompt
fragments gated by trust tier and skill). See [DOMAINS.md](DOMAINS.md) for full details.

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

Semdragons is a reactive system — you seed the board with agents, post a quest, and watch
the processors do the work. This section shows two paths: letting the system work end-to-end
(recommended first), then driving transitions manually for debugging.

### Watch the System Work (seeded board)

Start with `SEED_E2E=true` to pre-populate agents and guilds, then post a quest and watch
the reactive pipeline in action.

```bash
# 1. Start with seeded data
SEED_E2E=true docker compose up -d

# Verify everything is running
curl -s http://localhost:8080/health
open http://localhost:5173
```

```bash
# 2. Post a quest (agents are already on the board)
curl -s -X POST http://localhost:8080/api/game/quests \
  -H "Content-Type: application/json" \
  -d '{
    "objective": "Write a hello world function",
    "difficulty": 0,
    "skills": ["code_generation"]
  }' | jq .
```

```bash
# 3. Watch the system react via SSE
curl -N http://localhost:8080/message-logger/kv/semdragons-local-dev-board1/watch?pattern=*
```

You should see a chain of state changes as the processors react:

1. **Quest posted** — `quest.state.*` key appears with status `posted`
2. **Boid suggestion** — `boidengine` computes attractions, suggests a claim
3. **Agent claims** — quest status changes to `claimed`, agent status to `on_quest`
4. **Quest starts** — `questbridge` detects `in_progress`, dispatches to LLM via AGENT stream
5. **LLM executes** — agent works the quest (requires a configured LLM provider)
6. **Result submitted** — quest transitions to `in_review` (if review required) or `completed`
7. **Boss battle** — `bossbattle` evaluates quality, emits verdict
8. **XP awarded** — `agentprogression` updates agent XP and potentially levels up

The dashboard at `http://localhost:5173` shows all of this in real-time via the event feed.

```bash
# 4. Check the final world state
curl -s http://localhost:8080/api/game/world | jq .
```

**Note**: Steps 5-8 require a configured LLM provider (Ollama, Anthropic, or OpenAI). Without
one, the quest will be claimed but stall at `in_progress`. See the [Model Registry](#model-registry)
section for setup. DM-elevated questions may also need human response depending on the
configured DM mode.

### Manual API Walkthrough (for debugging)

For understanding individual state transitions or testing specific edge cases, you can drive
the lifecycle manually with curl. This bypasses the boid engine and LLM execution.

```bash
# 1. Recruit an agent
curl -s -X POST http://localhost:8080/api/game/agents \
  -H "Content-Type: application/json" \
  -d '{"name": "Aria", "skills": ["code_generation"]}' | jq .

# Save the agent ID: "id": "local.dev.game.board1.agent.abc123"
```

```bash
# 2. Post a quest
curl -s -X POST http://localhost:8080/api/game/quests \
  -H "Content-Type: application/json" \
  -d '{"objective": "Write a hello world function"}' | jq .

# Save the quest ID: "id": "local.dev.game.board1.quest.xyz789"
```

```bash
# 3. Claim (use full IDs from steps 1-2)
curl -s -X POST http://localhost:8080/api/game/quests/{QUEST_ID}/claim \
  -H "Content-Type: application/json" \
  -d '{"agent_id": "{AGENT_ID}"}'

# 4. Start
curl -s -X POST http://localhost:8080/api/game/quests/{QUEST_ID}/start

# 5. Submit result
curl -s -X POST http://localhost:8080/api/game/quests/{QUEST_ID}/submit \
  -H "Content-Type: application/json" \
  -d '{"output": "func hello() string { return \"hello world\" }"}'

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

### Message Logger (SSE)

The backend exposes a Server-Sent Events endpoint for live KV watching. No extra tooling
required — use curl or the dashboard:

```bash
# Watch all entity state changes
curl -N http://localhost:8080/message-logger/kv/semdragons-local-dev-board1/watch?pattern=*

# Watch all quest state changes
curl -N http://localhost:8080/message-logger/kv/semdragons-local-dev-board1/watch?pattern=quest.state.*

# Watch a specific entity
curl -N http://localhost:8080/message-logger/kv/semdragons-local-dev-board1/watch?pattern=agent.state.{id}
```

The Vite dev proxy routes `/message-logger` to the backend, so the dashboard at
`http://localhost:5173` receives these same live updates automatically in the event feed.

### Common Issues

| Symptom | Cause | Fix |
|---------|-------|-----|
| "bucket not found" on first request | KV bucket created on first entity write | Post a quest or recruit an agent first |
| Agent can't claim quest | Tier too low for quest difficulty | Check `min_tier` on quest vs agent `tier` |
| 503 on store endpoints | `agent_store` component not running | Ensure it's enabled in config |
| SSE not connecting | Backend not running or Vite proxy misconfigured | Check `BACKEND_URL` and that `/message-logger` responds |
| Quest stuck in `in_review` | `bossbattle` processor not running | Ensure it's enabled in config |

## Further Reading

- [QUESTS.md](QUESTS.md) — Quest creation, lifecycle state machine, difficulty/XP table, boss battles
- [PARTIES.md](PARTIES.md) — Party formation, roles, peer reviews, feedback loop
- [BOIDS.md](BOIDS.md) — Emergent quest-claiming behavior, six rules, tuning guide
- [DESIGN.md](DESIGN.md) — Architecture, concept map, trust tiers, death mechanics
- [DOMAINS.md](DOMAINS.md) — Domain themes (software, D&D, research), prompt catalogs
- [MODEL-REGISTRY.md](MODEL-REGISTRY.md) — LLM provider config, capabilities, fallback chains
- [Swagger UI](/docs) — Live API documentation at `/docs`
