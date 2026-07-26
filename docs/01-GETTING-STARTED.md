# Getting Started

Semdragons is an agentic workflow coordination framework built around a tabletop RPG metaphor.
Work items are *quests*, autonomous LLM processes are *adventurers* (agents) who earn XP and level
up, and quality reviews are *boss battles*. Shipped Phase 1 is reactive only after assignment:
post a quest, explicitly claim it for an agent through the REST API or a test fixture, and
explicitly start it. The processors then execute it.

> **Phase-1 runtime status:** Bossbattle, agentprogression, agentstore, boidengine, and redteam
> are not activated by any shipped profile. There is no autonomous Boid claim, guild
> selection, or adversarial review. A review-required quest enters `in_review`, then the
> mechanics-stub rule completes it with `review-skipped-mechanics-disabled`. Their retained
> product and API surfaces remain dormant until the Phase-2 authoritative projections and
> owner handoffs are implemented.

This guide gets you from zero to your first completed quest.

## Prerequisites

| Tool | Minimum version | Check |
|------|----------------|-------|
| Docker + Docker Compose | Docker 24+ | `docker --version && docker compose version` |
| Go | 1.26.3+ | `go version` |
| Node.js | 20+ | `node --version` |
| go-task | any | `task --version` |

Install go-task:

```bash
brew install go-task          # macOS
# or
go install github.com/go-task/task/v3/cmd/task@latest
```

No LLM API key is required for the mock quick start. Real quest execution needs at least one
provider — see [Using Real LLM Providers](#using-real-llm-providers) below.

## Quick Start with Mock LLM

The mock LLM returns deterministic, canned responses. After an explicit REST API or test-fixture
claim and start, it exercises the current Phase-1 execution and lifecycle-rule completion path
without any API keys or costs.

### Step 1: Start the stack

```bash
task docker:up
```

This command builds and starts seven containers:

| Service | Image | Ports | Purpose |
|---------|-------|-------|---------|
| `nats` | `nats:2.12-alpine` | 4222, 8222 | NATS JetStream — message broker and KV store |
| `mockllm` | Go binary (built locally) | 9090 | OpenAI-compatible mock server with canned responses |
| `semembed` | `ghcr.io/c360studio/semembed` | 8083 | Local embedding model for semantic search |
| `sandbox` | Built locally | internal | Isolated execution environment for agent file tools |
| `backend` | Built locally | 8081 | REST API + all reactive processors |
| `ui` | Built locally | 5173 (internal) | SvelteKit dashboard (Vite dev server) |
| `caddy` | `caddy:2-alpine` | 80 | Reverse proxy — serves UI and API same-origin |

When the command returns, all services have passed their health checks. The backend port is also
exposed directly at `http://localhost:8081` if you want to call the API without going through
Caddy.

Access points:

| URL | What |
|-----|------|
| `http://localhost` | Dashboard (via Caddy) |
| `http://localhost/game/` | REST API |
| `http://localhost/docs` | Swagger UI — interactive REST API reference |
| `http://localhost/openapi.json` | OpenAPI spec (machine-readable) |
| `http://localhost:8222` | NATS monitoring UI |

### Step 2: Open the dashboard

Navigate to `http://localhost`. On first load you will see:

- **Agents panel** — a roster of seeded agents (approximately 20) at various levels and skill sets.
  These are populated automatically by the `SEED_AGENTS=true` default in the Docker environment.
- **Quest board** — empty. No quests have been posted yet.
- **Event feed** — live SSE stream of all KV state changes, updating in real time.

### Step 3: Post a quest via DM chat

The Dungeon Master (DM) chat is the primary interface for posting quests. It accepts natural
language and converts it to a structured quest using the LLM (or mock).

1. Click the **DM Chat** button or navigate to the chat panel.
2. Type a request such as:

   ```
   Post a quest to analyze a test dataset and generate a summary report.
   ```

3. The mock LLM responds with a `quest_brief` JSON block that the backend parses automatically.
   The quest is created and posted to the board.

You can also post a quest directly via the API:

```bash
curl -s -X POST http://localhost/game/quests \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Analyze test dataset",
    "goal": "Process the test dataset and generate a summary report",
    "difficulty": 2,
    "skills": ["analysis", "summarization"]
  }' | jq .
```

### Step 4: Explicitly claim, start, and watch the pipeline

After the quest is posted, use Swagger UI or the REST API to call
`POST /game/quests/{id}/claim` with the selected `agent_id`, then
`POST /game/quests/{id}/start`. A test fixture may drive the same two transitions. Only after
those explicit actions do the reactive processors execute the quest. You will see these state
transitions in the dashboard event feed:

1. **`posted`** — Quest appears on the board. Shipped Phase 1 keeps `boidengine` disabled, so the
   quest remains posted until it is explicitly claimed.
2. **`claimed`** — Claim the quest and select an agent through the REST API or a test fixture.
   Autonomous boid-driven claim and guild selection are unavailable. The lifecycle operator is
   an additional path only when its separately gated component is enabled.
3. **`in_progress`** — The questbridge processor detects the transition and dispatches the quest
   after the same API or fixture explicitly starts it. The mock LLM calls a tool, receives a
   result, then emits a completion message.
4. **`in_review`** — The agent submits review-required work. The mechanics-stub rule records
   that review is skipped while mechanics are disabled.
5. **`completed`** — The lifecycle rule completes the quest without creating a battle, peer
   review, red-team quest, guild lesson, XP award, reputation change, artifact merge/index
   mutation, or store mutation.

After explicit claim and start, the full loop with the mock LLM typically completes in under
30 seconds.

### Step 5: See the results

- **Quest detail** — Click the quest to see its submitted output and Phase-1 review-skipped marker.
- **Trajectory timeline** — Each quest execution generates a trajectory (trace). Click
  **Trajectory** on the quest detail page to see the full tool call audit: which tools were
  called, what arguments were passed, and what results came back.
- **Agent card** — Click the agent to see its retained profile and quest history.
- **Live SSE** — Watch raw state changes stream in:

  ```bash
  curl -N "http://localhost/game/events"
  ```

## Using Real LLM Providers

Switch from the mock to a real provider when you want agents to do actual work.

### Step 1: Set your API key

Copy `.env.example` to `.env` and uncomment the key for your provider:

```bash
cp .env.example .env
# Edit .env — e.g., uncomment:
#   GEMINI_API_KEY=your-key-here
```

### Step 2: Start with the provider-specific command

```bash
task docker:up:gemini       # Google Gemini (GEMINI_API_KEY in .env)
task docker:up:anthropic    # Anthropic Claude (ANTHROPIC_API_KEY in .env)
task docker:up:openai       # OpenAI (OPENAI_API_KEY in .env)
task docker:up:ollama       # Local Ollama (requires: ollama serve)
```

Each command loads the corresponding model config file from `config/models/` automatically by
setting `SEMDRAGONS_MODELS=/etc/semdragons/models/<provider>.json` inside the container. If you
are running the backend outside Docker and want to switch providers, set this env var yourself:

```bash
SEMDRAGONS_MODELS=config/models/gemini.json go run ./cmd/semdragons
```

The mock LLM container is not started when using a real provider.

### Step 3: Restart after switching

```bash
task docker:down
task docker:up:gemini       # or whichever provider you chose
```

**Cost warning:** Each quest execution calls the LLM for agent work. Shipped Phase-1 profiles do
not make a second bossbattle judge call. A busy board can still burn through tokens quickly, so
start with Ollama or Gemini Flash and set billing alerts on cloud providers.

See [07-MODEL-REGISTRY.md](07-MODEL-REGISTRY.md) for advanced configuration: multi-model
setups, capability-based routing, fallback chains, and self-hosted OpenAI-compatible services.

## Running Services Individually

For backend development, run services outside Docker to get faster rebuild cycles.

### 1. Start NATS

```bash
# Using the included JetStream config
nats-server -c docker/nats-server.conf

# Or via Docker alone
docker compose -f docker/compose.yml up nats -d
```

### 2. Start the backend

```bash
go run ./cmd/semdragons --log-format=text --log-level=debug
```

The backend reads `config/semdragons.json` by default. The KV bucket name is derived from the
platform config: `semdragons-{org}-{platform}-{board}`. With defaults: `semdragons-local-dev-board1`.

Key CLI flags:

| Flag | Env var | Default | Description |
|------|---------|---------|-------------|
| `--config`, `-c` | `SEMDRAGONS_CONFIG` | `config/semdragons.json` | Config file path |
| `--models` | `SEMDRAGONS_MODELS` | — | Model registry overlay file (e.g., `config/models/gemini.json`) |
| `--log-level` | `SEMDRAGONS_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `--log-format` | `SEMDRAGONS_LOG_FORMAT` | `json` | `json` or `text` |
| `--debug` | — | — | Enable debug logging and pprof endpoint |
| `--shutdown-timeout` | — | `30s` | Graceful shutdown duration |
| `--version`, `-v` | — | — | Print version and exit |
| `--validate` | — | — | Validate config and exit |

NATS connection env vars (take priority over config file):

| Variable | Purpose |
|----------|---------|
| `SEMDRAGONS_NATS_URLS` | NATS server URLs, comma-separated |
| `SEMDRAGONS_NATS_USER` + `SEMDRAGONS_NATS_PASS` | Username/password auth |
| `SEMDRAGONS_NATS_TOKEN` | Token auth |

**Sandbox note:** The default config sets `sandbox_dir: "/workspace"` on both `questbridge`
and `questtools`. That path exists inside Docker but not on your host. When running locally,
update both entries in `config/semdragons.json` to a directory you control:

```bash
mkdir -p /tmp/semdragons-workspace
# Then set "sandbox_dir": "/tmp/semdragons-workspace" under both questbridge and questtools
```

### 3. Start the frontend

```bash
cd ui
npm install     # first time only
npm run dev     # Vite dev server at http://localhost:5173
```

Vite proxies `/game`, `/health`, and `/graph-gateway` to `http://localhost:8081` (the backend's
host-mapped port). Set the `BACKEND_URL` env var to override the proxy target. When running via
Docker Compose, Caddy handles proxying instead of Vite.

## Configuration Overview

The primary config file is `config/semdragons.json`. Top-level structure:

```json
{
  "platform": { "org": "local", "id": "dev", "environment": "development" },
  "nats": { "urls": ["nats://localhost:4222"] },
  "services": { "service-manager": { ... }, "game": { ... } },
  "model_registry": { "endpoints": { ... }, "capabilities": { ... } },
  "streams": { "GRAPH": { ... }, "AGENT": { ... } },
  "components": { "questboard": { ... }, "rule-processor": { ... }, ... }
}
```

### model_registry

Defines which LLM endpoints are available and how capabilities map to them. The default config
ships with two Ollama endpoints:

```json
"model_registry": {
  "endpoints": {
    "ollama-coder": {
      "provider": "ollama",
      "url": "http://localhost:11434/v1",
      "model": "qwen2.5-coder:7b",
      "max_tokens": 32768,
      "supports_tools": true
    }
  },
  "capabilities": {
    "agent-work": {
      "preferred": ["ollama-coder"],
      "requires_tools": true
    },
    "boss-battle": {
      "preferred": ["ollama-qwen3"],
      "fallback": ["ollama-coder"]
    }
  },
  "defaults": { "model": "ollama-coder", "capability": "agent-work" }
}
```

Replace endpoint URLs and models to switch providers. See [07-MODEL-REGISTRY.md](07-MODEL-REGISTRY.md).

### Components

The `components` section enables or disables each reactive processor. The default config includes
`questboard`, the complete quest `rule-processor` pack, `guildformation`, `agentic-loop`,
`agentic-model`, `questbridge`, `questtools`, `questdagexec`, `autonomy`, and `partycoord`.
The registered `agent_progression`, `agent_store`, `boidengine`, and `redteam` components are
disabled in every shipped profile, and bossbattle remains absent, during Phase 1. Their product
policy and models remain for Phase 2. Without boidengine, posting alone does not select an
agent; use the REST API or a test fixture to claim and start. Without redteam, review-required
work is resolved only by mechanics-stub and creates no adversarial-review or guild-lesson side
effect. Every shipped profile enables the authenticated lifecycle operator at
`/lifecycle-gateway/workflows` with WebSocket streaming disabled. The gateway is an operator
and break-glass surface; use the product REST endpoints for normal quest commands.

### Key env vars

| Variable | Purpose |
|----------|---------|
| `GEMINI_API_KEY` | Google Gemini API key |
| `ANTHROPIC_API_KEY` | Anthropic Claude API key |
| `OPENAI_API_KEY` | OpenAI API key |
| `BRAVE_SEARCH_API_KEY` | Enables the `web_search` tool for agents |
| `SEMDRAGONS_NATS_URLS` | Override NATS connection URL |
| `SEMDRAGONS_MODELS` | Model registry overlay file path (set automatically by provider task variants) |
| `SEMDRAGONS_API_KEY` | Auth key for writes and the lifecycle-gateway namespace (empty = dev mode, no auth) |
| `SEED_AGENTS` | Seed starter agents on boot (default: `true`) |
| `SEED_E2E` | Full seed: agents + guilds + store catalog (default: `false`) |
| `WORKSPACE` | Host directory mounted into the backend at `/workspace` |

## Development Commands

| Command | Description |
|---------|-------------|
| `task build` | Build all Go packages |
| `task test` | Unit tests — no Docker required |
| `task test:integration` | Integration tests — requires Docker (NATS via testcontainers) |
| `task test:all` | All tests (unit + integration) |
| `task test:one -- TestName` | Single unit test by name |
| `task test:one-integration -- TestName` | Single integration test by name |
| `task test:coverage` | Coverage report to `coverage.html` |
| `task lint` | `revive` + `go vet` |
| `task check` | Full check: fmt, tidy, lint, test-all |
| `task docker:up` | Start stack with mock LLM |
| `task docker:down` | Stop stack and remove volumes |
| `task docker:logs` | Tail backend logs |
| `task e2e` | Full E2E suite with mock LLM (both test tiers) |
| `task e2e:spec -- name` | Single Playwright spec against a running stack |
| `task e2e:ui` | Playwright interactive UI mode |
| `task e2e:gemini` | E2E with Gemini (requires `GEMINI_API_KEY`) |

## Testing

### Unit tests

No Docker required. These run against in-process mocks and cover the root package helpers,
promptmanager fragments, and executor logic.

```bash
task test
```

### Integration tests

Require Docker. Each test uses testcontainers to spin up a real NATS server. Tests cover all
processor components end-to-end against actual JetStream streams and KV buckets.

```bash
task test:integration
```

Integration tests are gated with the `//go:build integration` tag so `task test` never pulls
them in by accident.

### E2E tests

Playwright tests drive the full stack: dashboard UI, REST API, and the agentic loop. The mock
LLM makes these deterministic.

Positive red-team lifecycle specs are opt-in Phase-2 coverage. They remain skipped against
shipped Phase-1 profiles and are not evidence that redteam is active.

```bash
task e2e               # Full suite (installs Playwright browsers on first run)
task e2e:spec -- quest-pipeline   # Single spec file
task e2e:ui            # Interactive Playwright UI for debugging
```

The mock LLM routes requests by inspecting the incoming messages with regular expressions:

- A message that looks like a quest creation request returns a `quest_brief` JSON block.
- A message requesting multiple quests returns a `quest_chain` block.
- An agentic loop turn with tool results returns a `completionContent` string that signals
  the loop to finish.
- A party lead decomposition request returns a `dagDecompositionArgs` tool call.

This routing is deterministic — the same prompt pattern always produces the same response —
which makes E2E tests reliable without relying on any live LLM service.

## Troubleshooting

| Symptom | Likely cause | Fix |
|---------|-------------|-----|
| Quest remains `posted` | Expected before assignment | Claim/start explicitly; leave `boidengine` disabled |
| Quest stuck in `in_progress` | LLM unreachable or misconfigured | Check `docker logs semdragon-backend`; verify model endpoint |
| No agents visible on dashboard | `SEED_AGENTS` not set | Default is `true`; verify the backend env var |
| NATS connection refused | NATS not running on port 4222 | Run `docker compose -f docker/compose.yml up nats -d` |
| `sandbox_dir` errors locally | Default path `/workspace` does not exist on host | Set `sandbox_dir` in config to a local path |
| Quest stuck in `in_review` | Incomplete Phase-1 rules | Confirm `rule-processor` loads all four quest rule files |
| Store endpoints return 503 | Expected Phase-1 quarantine | Leave agentstore disabled until Phase 2 |

### Watch live NATS state changes

The backend exposes an SSE endpoint that streams KV changes in real time — no NATS CLI needed:

```bash
# All entity state changes
curl -N "http://localhost/game/events"

# Quest state changes only (direct to backend port)
curl -N "http://localhost:8081/message-logger/kv/semdragons-local-dev-board1/watch?pattern=quest.state.*"

# One specific entity
curl -N "http://localhost:8081/message-logger/kv/semdragons-local-dev-board1/watch?pattern=quest.state.abc123"
```

### Check backend logs

```bash
task docker:logs                         # Tail backend logs
docker compose -f docker/compose.yml logs -f backend   # Direct compose command
```

## What's Next

- [02-DESIGN.md](02-DESIGN.md) — Architecture deep dive: reactive event model, trust tiers, boid
  algorithm, death mechanics
- [03-QUESTS.md](03-QUESTS.md) — Quest creation API, lifecycle rules, and dormant Phase-2 battle model
- [04-PARTIES.md](04-PARTIES.md) — Party formation, DAG decomposition, peer reviews, feedback loop
- [07-MODEL-REGISTRY.md](07-MODEL-REGISTRY.md) — LLM provider setup, capability routing, fallback
  chains, self-hosted services
- [Swagger UI](http://localhost/docs) — Live API reference served at `/docs` when the stack is
  running
