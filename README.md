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
│                  DUNGEON MASTER                  │
│         (Human / LLM / Hybrid control)          │
├─────────────────────────────────────────────────┤
│   GUILDS        PARTIES         BOIDS           │
│   (specialization) (temp groups) (emergent flock)│
├─────────────────────────────────────────────────┤
│              QUEST BOARD                         │
│        (pull-based work distribution)           │
├─────────────────────────────────────────────────┤
│         XP ENGINE + BOSS BATTLES                │
│    (evaluation, leveling, trust gates)          │
├─────────────────────────────────────────────────┤
│                 SEMSTREAMS                       │
│   (event streaming, trajectories, observability)│
└─────────────────────────────────────────────────┘
```

**Key design choices:**
- **Pull-based**: Agents claim quests based on capability, not pushed assignments
- **Earned trust**: Tiers derived from XP, not declared roles
- **Emergent coordination**: Boids engine suggests claims without central scheduling
- **Full observability**: All events map to semstreams trajectories

## Development

```bash
make build      # Build all packages
make test       # Run all tests (requires Docker for testcontainers)
make lint       # Run golangci-lint
make check      # Full check: fmt, tidy, lint, test
```

## Project Structure

```
semdragons/
├── types.go        # Core domain types (Agent, Quest, TrustTier)
├── questboard.go   # QuestBoard interface and QuestBuilder
├── board.go        # NATSQuestBoard implementation
├── storage.go      # KV bucket and key patterns
├── events.go       # Event payloads and publishing
├── vocab.go        # Vocabulary predicates registration
├── entityid.go     # 6-part entity ID helpers
├── xp.go           # XP engine and leveling
├── boids.go        # Boid engine for emergent behavior
├── dm.go           # Dungeon Master interface
├── social.go       # Party and Guild structures
└── docs/
    └── DESIGN.md   # Full design document
```

## Dependencies

- [semstreams](https://github.com/c360studio/semstreams) - Event streaming and observability
- NATS JetStream - Message broker and KV store

## License

MIT
