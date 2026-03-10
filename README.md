# Semdragons

Agentic workflow coordination modeled as a tabletop RPG, built on [semstreams](https://github.com/c360studio/semstreams).

**Agents are adventurers. Work items are quests. Quality reviews are boss battles.**

Trust is earned through leveling. Specialization happens through guilds. Coordination emerges from Boid-like flocking behavior over a structured quest board.

*It's fun to build. The results are serious.*

## Quick Start

```bash
# 1. Explore with mock LLM (no API key needed)
task docker:up
open http://localhost

# 2. Or use a real provider — pick one:
cp .env.example .env           # then set your API key
task docker:up:gemini          # GEMINI_API_KEY
task docker:up:anthropic       # ANTHROPIC_API_KEY
task docker:up:openai          # OPENAI_API_KEY
task docker:up:ollama          # local, no key needed

# 3. Post a quest and watch agents work
curl -s -X POST http://localhost/game/quests \
  -H "Content-Type: application/json" \
  -d '{"objective": "Write a hello world function", "difficulty": 0}'

# 4. Stop
task docker:down
```

See [Getting Started](docs/01-GETTING-STARTED.md) for prerequisites, environment variables, and a full walkthrough.

## Core Concepts

| RPG Concept | Engineering Reality |
|-------------|---------------------|
| Quest Board | Pull-based work queue (no central bottleneck) |
| Quest | Task with difficulty, requirements, and rewards |
| Agent | LLM-powered worker with progression |
| Level / XP | Progressive trust earned through performance |
| Trust Tier | Permission boundary derived from competence |
| Boss Battle | Quality gate embedded in workflow |
| Party | Agent ensemble for complex tasks |
| Guild | Specialization cluster with shared knowledge |
| Boids | Emergent quest-claiming behavior |

## Trust Tiers

| Level | Tier | Capabilities |
|-------|------|--------------|
| 1-5 | Apprentice | Read-only, summarize, classify |
| 6-10 | Journeyman | Tools, API requests, staging writes |
| 11-15 | Expert | Production writes, deployments, spend money |
| 16-18 | Master | Supervise agents, decompose quests, lead parties |
| 19-20 | Grandmaster | Act as DM delegate, manage guilds |

## Architecture

```
┌─────────────────────────────────────────────────┐
│              SVELTE DASHBOARD (:5173)            │
│  (quests, agents, battles, store, guilds,        │
│   settings, graph, trajectories)                 │
├─────────────────────────────────────────────────┤
│            REST API  (:8080/game/)                 │
│   quests · agents · battles · store · world      │
├─────────────────────────────────────────────────┤
│   DUNGEON MASTER LAYER                           │
│   dmsession · dmapproval · dmworldstate          │
│   dmpartyformation · autonomy                    │
├─────────────────────────────────────────────────┤
│   GUILDS        PARTIES         BOIDS            │
│   guildformation  partycoord    boidengine       │
├─────────────────────────────────────────────────┤
│         QUEST BOARD + EXECUTION                  │
│   questboard · bossbattle · agentprogression     │
│   agentstore · questbridge · questtools          │
├─────────────────────────────────────────────────┤
│              PROMPT ASSEMBLY                     │
│   promptmanager · domains (software/dnd/research)│
├─────────────────────────────────────────────────┤
│                 SEMSTREAMS                        │
│   NATS JetStream · KV (entity state + events)   │
│   graph-ingest · graph-index · graph-query       │
└─────────────────────────────────────────────────┘
```

**Key design choices:**
- **Pull-based**: Agents claim quests based on capability, not pushed assignments
- **Earned trust**: Tiers derived from XP, not declared roles
- **Emergent coordination**: Boids engine suggests claims without central scheduling
- **Full observability**: All events map to semstreams trajectories
- **Domain-configurable**: Software, D&D, and research themes ship out of the box
- **Model registry**: Route LLM calls to Anthropic, OpenAI, or Ollama with capability-based fallback chains
- **Event-driven execution**: questbridge + questtools bridge quest lifecycle to LLM loops via AGENT JetStream stream

## Development

```bash
task build                    # Build all packages
task test                     # Unit tests only (no Docker)
task test:integration         # Integration tests (requires Docker)
task test:all                 # All tests
task lint                     # revive + go vet
task check                    # fmt, tidy, lint, test-all
task e2e                      # Full E2E suite (Playwright + Docker)
```

## Project Structure

```
semdragons/
├── Taskfile.yml            # Task runner (build, test, docker, e2e targets)
├── docker/                 # Compose files, Dockerfiles, infrastructure config
│   ├── compose.yml         #   Full stack: nats + mockllm + backend + ui
│   ├── Caddyfile           #   Reverse proxy config
│   ├── ui.Dockerfile       #   SvelteKit production build
│   ├── backend.Dockerfile  #   Multi-stage Go build
│   ├── mockllm.Dockerfile  #   Mock LLM server
│   └── nats-server.conf    #   NATS JetStream config
├── cmd/semdragons/         # Binary entry point + CLI
├── componentregistry/      # Registers all processors with semstreams
├── config/                 # Runtime config + model registry (semdragons.json, models.json)
├── domain/                 # Enums, config types, vocabulary (source of truth)
├── domains/                # Domain implementations: software, dnd, research
├── processor/              # 18 reactive processors + 2 libraries
│   ├── agentprogression/   #   XP and leveling on quest outcome
│   ├── agentstore/         #   XP marketplace: tools, consumables
│   ├── autonomy/           #   Heartbeat-driven agent decision loop
│   ├── boardcontrol/       #   Play/pause board state (library)
│   ├── boidengine/         #   Periodic boid attraction computation
│   ├── bossbattle/         #   Review evaluation (automated + LLM + human)
│   ├── dmapproval/         #   DM approval gate (NATS request/reply)
│   ├── dmpartyformation/   #   DM-controlled party assembly
│   ├── dmsession/          #   DM session lifecycle
│   ├── dmworldstate/       #   Aggregated world state for /world
│   ├── executor/           #   LLM prompt assembly + tool execution
│   ├── guildformation/     #   Auto guild clustering from performance
│   ├── partycoord/         #   Party lifecycle (form/assign/merge/disband)
│   ├── promptmanager/      #   Fragment-based domain-aware prompts (library)
│   ├── questboard/         #   Quest lifecycle state machine
│   ├── questbridge/        #   Quest-to-LLM bridge (AGENT stream)
│   ├── questdagexec/       #   DAG execution for party quest decomposition
│   ├── questtools/         #   Tool execution with tier/skill gates
│   ├── seeding/            #   Environment bootstrapping
│   └── tokenbudget/        #   Token budget tracking for context management
├── service/api/            # REST API handlers
├── ui/                     # SvelteKit 5 dashboard + Playwright E2E
│   ├── src/routes/         #   Pages: agents, quests, battles, store, guilds,
│   │                       #         settings, graph, trajectories, workspace, parties
│   └── e2e/specs/          #   Playwright E2E specs
├── docs/                   # Numbered guides (01-08) + adr/ for architecture decisions
└── *.go                    # Core types, entity IDs, graph client, vocab
```

## Dependencies

- [semstreams](https://github.com/c360studio/semstreams) — Event streaming and observability
- NATS JetStream — Message broker and KV store
- SvelteKit 5 — Dashboard UI
- Playwright — E2E testing

Module: `github.com/c360studio/semdragons`

## License

MIT
