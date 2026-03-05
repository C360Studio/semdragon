# Semdragons

Agentic workflow coordination modeled as a tabletop RPG, built on [semstreams](https://github.com/c360studio/semstreams).

**Agents are adventurers. Work items are quests. Quality reviews are boss battles.**

Trust is earned through leveling. Specialization happens through guilds. Coordination emerges from Boid-like flocking behavior over a structured quest board.

*It's fun to build. The results are serious.*

## Quick Start

```go
import "github.com/c360studio/semdragons"

// Create a quest
quest := semdragons.NewQuest("Analyze Q3 sales data").
    Description("Pull data, identify trends, write executive summary").
    Difficulty(semdragons.DifficultyHard).
    RequireSkills(semdragons.SkillAnalysis, semdragons.SkillDataTransform).
    XP(250).
    ReviewAs(semdragons.ReviewStrict).
    Build()

// Post to board - agents will claim based on capability
board.PostQuest(ctx, quest)
```

See [docs/GETTING-STARTED.md](docs/GETTING-STARTED.md) for full setup, environment variables, and a first-quest walkthrough.

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
│     (quests, agents, battles, store, guilds)     │
├─────────────────────────────────────────────────┤
│            REST API  (:8080/api/game/)            │
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
make build                    # Build all packages
make test                     # Unit tests only (no Docker)
make test-integration         # Integration tests (requires Docker)
make test-all                 # All tests
make lint                     # revive + go vet
make check                    # fmt, tidy, lint, test-all
make e2e                      # Full E2E suite (Playwright + Docker)
```

## Project Structure

```
semdragons/
├── docker-compose.yml      # Full stack: nats + mockllm + backend + ui
├── docker-compose.cloud.yml  # Cloud LLM override (Gemini/Anthropic/OpenAI)
├── docker-compose.ollama.yml # Local Ollama override
├── docker/                 # Dockerfiles + infrastructure config
│   ├── backend.Dockerfile  #   Multi-stage Go build
│   ├── mockllm.Dockerfile  #   Mock LLM server
│   └── nats-server.conf    #   NATS JetStream config
├── cmd/semdragons/         # Binary entry point + CLI
├── componentregistry/      # Registers all processors with semstreams
├── config/                 # Runtime config + model registry (semdragons.json, models.json)
├── domain/                 # Enums, config types, vocabulary (source of truth)
├── domains/                # Domain implementations: software, dnd, research
├── processor/              # 17 reactive event processors
│   ├── agentprogression/   #   XP and leveling on quest outcome
│   ├── agentstore/         #   XP marketplace: tools, consumables
│   ├── autonomy/           #   Heartbeat-driven agent decision loop
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
│   ├── questtools/         #   Tool execution with tier/skill gates
│   └── seeding/            #   Environment bootstrapping
├── service/api/            # REST API handlers
├── ui/                     # SvelteKit 5 dashboard + Playwright E2E
│   ├── src/routes/         #   Pages: agents, quests, battles, store, guilds
│   └── e2e/specs/          #   12 Playwright specs
├── docs/                   # Design, getting started, quests, parties, boids, domains, model registry, API ref
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
