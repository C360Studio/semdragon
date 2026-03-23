# Semdragons

Semdragons is an agentic workflow coordination framework built on
[semstreams](https://github.com/c360studio/semstreams). It models work as a tabletop RPG:
agents are adventurers who earn XP and level up through demonstrated competence, work items
are quests pulled from a shared board, quality gates are boss battles, and large tasks
decompose into party quests with a lead agent coordinating a DAG of sub-quests. The RPG
framing is genuine — trust tiers are earned, not declared, and emergent Boid-like behavior
handles quest distribution without central scheduling.

## Quick Start

**Prerequisites**: [Docker](https://docs.docker.com/get-docker/),
[Go 1.25+](https://go.dev/dl/), [Node 20+](https://nodejs.org/),
[go-task](https://taskfile.dev/#/installation)

```bash
# Start the full stack — no API key needed, mock LLM included
task docker:up

# Open the dashboard
open http://localhost

# Stop everything
task docker:down
```

The mock LLM responds with canned completions so you can watch the full quest pipeline
without spending tokens. Once the stack is up:

- Dashboard: `http://localhost`
- Swagger UI: `http://localhost/docs`

1. Open the DM Chat panel and type: `Post a quest to write a hello world function`
2. Watch the Quests view — agents claim the quest and begin executing within seconds
3. The Battles view shows the boss battle review once the agent submits its result
4. Trajectories show the full tool call audit trail for each quest execution

To use a real LLM instead, copy `.env.example` to `.env`, add your key, and pick a provider:

```bash
task docker:up:gemini      # GEMINI_API_KEY
task docker:up:openai      # OPENAI_API_KEY
task docker:up:anthropic   # ANTHROPIC_API_KEY
task docker:up:ollama      # local Ollama, no key needed
```

## Architecture at a Glance

```
┌─────────────────────────────────────────────────┐
│              SVELTE DASHBOARD (:5173)            │
│  quests · agents · battles · store · guilds     │
│  trajectories · workspace · parties · settings  │
├─────────────────────────────────────────────────┤
│           REST API  (:8080/game/)               │
│  quests · agents · battles · store · world      │
│  DM chat · peer reviews · board control         │
├─────────────────────────────────────────────────┤
│              DUNGEON MASTER LAYER               │
│  dmsession · dmapproval · dmworldstate          │
│  dmpartyformation · autonomy                    │
├─────────────────────────────────────────────────┤
│   GUILDS          PARTIES          BOIDS        │
│   guildformation  partycoord       boidengine   │
├─────────────────────────────────────────────────┤
│           QUEST BOARD + EXECUTION               │
│  questboard · bossbattle · redteam              │
│  agentprogression · agentstore                  │
│  questbridge · questtools · questdagexec        │
│  promptmanager                                  │
├─────────────────────────────────────────────────┤
│              AGENTIC LOOP                        │
│  questbridge → agentic-loop → questtools        │
├─────────────────────────────────────────────────┤
│                  SEMSTREAMS                      │
│  NATS JetStream · KV (entity state + events)   │
│  graph-ingest · graph-index · graph-query       │
│  agentic-loop · agentic-model                   │
└─────────────────────────────────────────────────┘
```

Each layer reacts to events emitted by the layer below — nothing polls, nothing calls down
the stack. The key infrastructure insight: **NATS KV buckets are backed by JetStream
streams**, so the entity state store is simultaneously the event log. A single KV Watch on
`quest.state.*` gives you both current state and a replay of every transition. No separate
event bus is needed.

## Key Concepts

| Concept | What It Is |
|---------|------------|
| **Quest** | A unit of work with an objective, difficulty, required skills, and XP reward |
| **Agent** | An LLM-powered worker with a level, XP total, skill tags, and a trust tier |
| **Guild** | A specialization cluster agents join based on demonstrated performance patterns |
| **Party** | A temporary ensemble assembled for a complex quest that decomposes into a DAG |
| **Boss Battle** | The quality gate — automated or LLM-based review that runs after an agent submits |
| **Boids** | Emergent quest-claiming: agents attract toward quests matching their skills, no scheduler needed |
| **Trust Tier** | Permission boundary (Apprentice through Grandmaster) derived from level, not declared |
| **Trajectory** | A semstreams trace linking every event, tool call, and state transition for a quest |
| **Dungeon Master** | The human or hybrid controller: posts quests, sets policy, intervenes via chat |
| **Sandbox** | An isolated container where agents run code, write files, and execute shell commands |
| **Artifact** | A file produced by an agent during quest execution, stored in the sandbox workspace |
| **Red-Team Review** | Adversarial guild review that runs before the boss battle; extracts lessons into guild knowledge |
| **Guild Lessons** | Indexed knowledge (by skill + category) persisted from red-team reviews and injected into future agent prompts |
| **Explore** | A read-only sub-agent spawned via the `explore` tool for multi-step discovery without consuming the parent agent's iteration budget |

## Trust Tiers

| Level | Tier | Capabilities |
|-------|------|--------------|
| 1-5 | Apprentice | `submit_work`, `ask_clarification`, `explore`, `graph_query`, `graph_search`, `graph_multi_query`, `graph_summary` |
| 6-10 | Journeyman | All Apprentice tools + `bash`, `http_request`, `web_search` |
| 11-15 | Expert | All Journeyman tools; eligible for production-critical quests |
| 16-18 | Master | All Expert tools + `decompose_quest`, `review_sub_quest`, `answer_clarification` (party lead) |
| 19-20 | Grandmaster | DM delegation, guild management |

## Project Structure

```
semdragons/
├── cmd/
│   ├── semdragons/         # Binary entry point: CLI flags, config loading, graceful shutdown
│   └── mockllm/            # OpenAI-compatible mock server with canned responses (E2E testing)
├── domain/                 # Authoritative enums: SkillTag, TrustTier, QuestStatus, DMMode, etc.
├── domains/                # Domain implementations: software.go, dnd.go, research.go
├── processor/              # Reactive components — each watches KV, reacts to state changes
│   ├── questboard/         # Quest lifecycle state machine (post, claim, start, submit, complete)
│   ├── bossbattle/         # Review evaluation triggered on quest submission
│   ├── questbridge/        # Quest-to-LLM bridge: assembles TaskMessage, publishes to AGENT stream
│   ├── questtools/         # Tool execution with tier/skill/sandbox gates
│   ├── questdagexec/       # DAG execution for party quest decomposition
│   ├── agentprogression/   # XP calculation and leveling on quest outcome
│   ├── agentstore/         # XP marketplace: tools and consumables
│   ├── autonomy/           # Heartbeat-driven agent decision loop
│   ├── boidengine/         # Periodic boid attraction computation, publishes per-agent suggestions
│   ├── guildformation/     # Auto guild clustering from shared performance patterns
│   ├── redteam/            # Guild red-team adversarial review before boss battle
│   ├── partycoord/         # Party lifecycle: form, assign, merge, disband
│   ├── promptmanager/      # Fragment-based domain-aware prompt assembly (library, not standalone)
│   ├── boardcontrol/       # Board state management (pause/resume)
│   ├── tokenbudget/        # Token budget ledger — hourly spend tracking and circuit breaker (library, not standalone)
│   ├── dmsession/          # DM session lifecycle
│   ├── dmapproval/         # DM approval gate via NATS request/reply
│   ├── dmpartyformation/   # DM-initiated party formation
│   ├── dmworldstate/       # Aggregated world state snapshot for /api/game/world
│   ├── executor/           # Synchronous LLM execution (superseded by questbridge for new work)
│   └── seeding/            # Environment bootstrapping for dev/test
├── service/api/            # REST API handlers + Swagger UI at /docs + /openapi.json
├── componentregistry/      # Single file that imports and wires all processors
├── config/
│   ├── semdragons.json     # Default runtime config: streams, components, model_registry
│   └── models/             # Per-provider model registry overlays (gemini.json, openai.json, etc.)
├── ui/                     # SvelteKit 5 dashboard
│   ├── src/routes/         # Pages: agents, quests, battles, store, guilds, settings,
│   │                       #        graph, trajectories, workspace, parties
│   └── e2e/specs/          # Playwright E2E specs
├── docs/                   # Numbered guides + adr/ for architecture decisions
├── docker/                 # Compose files, Dockerfiles, Caddyfile, NATS config
└── *.go                    # Core types, entity IDs, graph client, vocab (root package)
```

## Docker Services

| Service | Port | Profile | Purpose |
|---------|------|---------|---------|
| `nats` | 4222, 8222 | always | NATS JetStream — message broker, KV store, event log |
| `backend` | 8081 (host) | always | Go API server — all game logic and processors |
| `ui` | 5173 | always | SvelteKit dashboard — live-updating via SSE |
| `caddy` | 80 | always | Reverse proxy — routes `/`, `/game`, `/message-logger` |
| `mockllm` | 9090 | `mock` | OpenAI-compatible mock LLM — enables `task docker:up` without an API key |
| `sandbox` | 8090 | always | Isolated execution container — agents run code here |
| `semembed` | 8083 | always | Rust embedding server (BGE model) — vector search for graph queries |
| `nats-semsource` | — | `semsource` | Separate NATS cluster for semsource entity ingestion |
| `semsource` | — | `semsource` | Repo/doc ingestion pipeline — streams entities into the graph |

`task docker:up` activates the `mock` profile. Real LLM tasks (`docker:up:gemini`, etc.) run
without the mock profile so the backend routes to the configured provider instead.

## Development

```bash
task build                    # Build all packages
task test                     # Unit tests only — no Docker, fast feedback
task test:integration         # Integration tests — requires Docker (testcontainers)
task test:all                 # Full suite: unit + integration
task test:one -- TestName     # Run one test by name
task lint                     # revive + go vet
task check                    # fmt + tidy + lint + test:all
task e2e                      # Playwright E2E suite against Docker stack with mock LLM
task e2e:gemini               # E2E against Docker stack with Gemini
task e2e:pros:gemini          # Tier 3 epic run — full Pros roster with Gemini
task docker:logs              # Tail backend logs
task docker:logs:all          # Tail all service logs
```

**Test categories**: Unit tests (no build tag, no Docker) live alongside source files and in
`processor/promptmanager/` and `processor/executor/`. Integration tests use
`//go:build integration` and spin up NATS via testcontainers. Use `task test` during
development; run `task test:all` before committing.

## Using Real LLM Providers

Provider configuration lives in `docs/07-MODEL-REGISTRY.md`. The short version:

1. Copy `.env.example` to `.env`
2. Set the key for your chosen provider:

```bash
GEMINI_API_KEY=your-key-here
OPENAI_API_KEY=your-key-here
ANTHROPIC_API_KEY=your-key-here
BRAVE_SEARCH_API_KEY=your-key-here   # optional, enables web search tool
```

3. Start with the matching task variant (`task docker:up:gemini`, etc.)

Model routing, fallback chains, and per-tier model selection are configured via overlay
files in `config/models/` (e.g., `gemini.json`, `openai.json`). See [Model Registry docs](docs/07-MODEL-REGISTRY.md) for full details.

## API

Swagger UI is served at `http://localhost/docs` (or `http://localhost:8081/docs` directly)
while the stack is running. The raw OpenAPI spec is at `/openapi.json`.

Key endpoint groups under `/game/`:

| Group | Endpoints |
|-------|-----------|
| Quests | `GET /quests`, `POST /quests`, `POST /quests/{id}/claim`, `/start`, `/submit`, `/complete`, `/fail`, `/abandon` |
| Artifacts | `GET /quests/{id}/artifacts`, `/artifacts/list`, `/artifacts/{path...}` |
| Agents | `GET /agents`, `GET /agents/{id}`, `POST /agents`, `POST /agents/{id}/retire` |
| Battles | `GET /battles`, `GET /battles/{id}` |
| Parties | `GET /parties`, `GET /parties/{id}` |
| Guilds | `GET /guilds`, `GET /guilds/{id}` |
| DM | `POST /dm/chat`, `GET /dm/sessions/{id}`, `POST /dm/triage/{questId}` |
| Peer Reviews | `GET /reviews`, `POST /reviews`, `POST /reviews/{id}/submit` |
| Store | `GET /store`, `POST /store/purchase` |
| Board | `GET /board/status`, `POST /board/pause`, `POST /board/resume`, `GET /board/tokens` |
| World | `GET /world` — full aggregated snapshot of all entity state |
| Settings | `GET /settings`, `POST /settings` |
| Trajectories | `GET /trajectories/{id}` |
| Events (SSE) | `GET /events` — real-time entity updates, same feed the dashboard uses |

## Documentation

| Document | Description |
|----------|-------------|
| [Getting Started](docs/01-GETTING-STARTED.md) | Prerequisites, Docker Compose walkthrough, first quest |
| [Design](docs/02-DESIGN.md) | Architecture, concept map, trust tiers, red-team review, DM attention, death mechanics |
| [Quests](docs/03-QUESTS.md) | Quest lifecycle state machine, difficulty/XP table, boss battles, chains |
| [Parties](docs/04-PARTIES.md) | Party formation, DAG decomposition, peer reviews, feedback loop |
| [Boids](docs/05-BOIDS.md) | Emergent quest-claiming rules, guild/reputation integration, tuning guide |
| [Domains](docs/06-DOMAINS.md) | Domain configs, agent archetypes, skill taxonomies, prompt catalogs |
| [Model Registry](docs/07-MODEL-REGISTRY.md) | LLM provider config, capability routing, fallback chains |
| [Sandbox Repos](docs/08-SANDBOX-REPOS.md) | Git-backed artifact storage: worktree lifecycle, quality gate via merge, semsource integration |
| [ADR-001: DM Chat Routing](docs/adr/001-dm-chat-routing.md) | DM chat mode routing and orchestration |
| [ADR-002: Party Quest DAG](docs/adr/002-party-quest-dag-execution.md) | Reactive DAG execution architecture |
| [ADR-003: DAG Refactor](docs/adr/003-questdagexec-refactor.md) | Single-goroutine event loop replacing concurrent model |
| [ADR-004: Clarification Loop](docs/adr/004-party-clarification-loop.md) | Sub-quest clarification routing to party lead |
| [ADR-005: Guild Founding Quorum](docs/adr/005-guild-founding-quorum.md) | Founder-driven guild formation with quorum gate |
| [ADR-006: Guild Founding Quests](docs/adr/006-guild-founding-quests.md) | LLM-driven guild identity and recruitment |
| [ADR-007: Scenario-Driven Specs](docs/adr/007-scenario-driven-quest-specs.md) | Scenario-driven quest specifications and decomposability |
| [ADR-008: Sandbox Container](docs/adr/008-sandbox-container.md) | Isolated execution container for agent code |
| [ADR-009: Provider Adapters](docs/adr/009-provider-adapters.md) | Provider adapter pattern for agentic-model |
| [ADR-010: Tool Choice](docs/adr/010-tool-choice.md) | Tool choice enforcement via API |

Module: `github.com/c360studio/semdragons`
