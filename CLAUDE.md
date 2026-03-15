# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Semdragons is an agentic workflow coordination framework modeled as a tabletop RPG, built on semstreams for observability. Work items are quests, agents are adventurers who earn XP and level up, quality reviews are boss battles, and trust is derived from demonstrated competence rather than declared roles.

**Core insight**: Make building and coordinating agents fun while keeping results serious.

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

**Key design choices**:
- Pull-based (not push-based) work distribution - agents claim quests based on capability
- Trust tiers derived from level (earned through XP, not declared)
- Boids engine provides emergent quest-claiming without central assignment
- All events map to semstreams trajectories for full observability

## Critical: Semstreams Integration

**This is an event-based reactive framework. Do not fall into OO traps.**

### Use Existing Utility Packages
Semstreams provides solid utility packages - use them, don't reinvent:
- `natsclient` - NATS connection handling, KV, typed subjects
- `vocabulary` - Predicate registration and metadata
- `errs` - Error handling patterns with classification
- Other shared utilities in the semstreams ecosystem

### Vocabulary System: Three-Part Predicates

All predicates follow **three-level dotted notation**: `domain.category.property`

**Semdragons predicates:**
```go
// Quest lifecycle
quest.lifecycle.posted      // Quest added to board
quest.lifecycle.claimed     // Agent claimed quest
quest.lifecycle.started     // Work began
quest.lifecycle.submitted   // Result submitted for review
quest.lifecycle.completed   // Quest finished successfully
quest.lifecycle.failed      // Quest failed
quest.lifecycle.escalated   // Needs higher attention
quest.lifecycle.abandoned   // Agent gave up

// Agent progression
agent.progression.xp        // XP earned/lost
agent.progression.levelup   // Level increased
agent.progression.leveldown // Level decreased
agent.progression.death     // Agent permadeath

// Boss battles
battle.review.started       // Review began
battle.review.verdict       // Review result
battle.review.victory       // Passed review
battle.review.defeat        // Failed review

// Social
party.formation.created     // Party formed
party.formation.disbanded   // Party dissolved
guild.membership.joined     // Agent joined guild
guild.membership.promoted   // Rank increased
```

**Rules:**
- Dots only for separation (no underscores, colons)
- All lowercase
- `domain.category.property` format enables NATS wildcards

### Entity IDs: Six-Part Structure

Entity IDs follow **six-level dotted notation** for federated management:

```
org.platform.domain.system.type.instance
```

**Semdragons entity ID mapping:**
```go
// org      = organization namespace (e.g., "c360")
// platform = deployment instance (e.g., "prod", "dev")
// domain   = always "game" for semdragons
// system   = quest board instance (e.g., "board1")
// type     = entity type (quest, agent, party, guild, battle)
// instance = unique identifier

// Examples:
c360.prod.game.board1.quest.abc123
c360.prod.game.board1.agent.dragon
c360.prod.game.board1.party.epic001
c360.prod.game.board1.guild.datawranglers
c360.prod.game.board1.battle.b789
```

**Hierarchical prefixes enable wildcard queries:**
```go
c360.prod.game.board1.quest.*        // All quests on board1
c360.prod.game.board1.agent.>        // All agents and their events
c360.prod.game.>                     // All game entities across systems
```

### KV Key Patterns

NATS KV keys use dots for hierarchy (KV is streams under the hood):

```go
// Entity state by type and ID
quest.state.{quest_id}               // Quest current state
agent.state.{agent_id}               // Agent current state
party.state.{party_id}               // Party current state

// Indices for queries (use wildcards)
quest.status.posted.{quest_id}       // Posted quests index
quest.status.claimed.{quest_id}      // Claimed quests index
quest.agent.{agent_id}.{quest_id}    // Quests by agent
quest.guild.{guild_id}.{quest_id}    // Quests with guild priority

// Stats and aggregates
stats.board.current                  // Current board statistics

// DAG execution state (stored as quest.dag.* predicates on parent quest entity)
// quest.dag.execution_id             // DAG execution identifier
// quest.dag.definition               // QuestDAG JSON (nodes, dependencies)
// quest.dag.node_quest_ids           // map[nodeID]subQuestID
// quest.dag.node_states              // map[nodeID]state (pending/ready/assigned/completed/failed)
// quest.dag.node_assignees           // map[nodeID]agentID
// quest.dag.node_retries             // map[nodeID]retriesRemaining
```

### Think Reactive, Not Objects
- Events flow through the system; don't model everything as objects with methods
- Handlers react to events; they don't call methods on other services
- State changes emit events; consumers react independently
- Avoid request/response patterns when pub/sub fits better

### The NATS KV "Twofer"

NATS KV buckets are backed by JetStream streams. This gives us unified state + event semantics:

| Interface | Purpose | Example |
|-----------|---------|---------|
| **KV Get/Put** | Current state queries | `Get("c360.prod.game.board1.quest.abc")` |
| **KV Watch** | Event subscription | `Watch("c360.prod.game.board1.quest.*")` |
| **Stream Replay** | Historical reconstruction | Replay from revision N |

**Key insight**: We don't need separate event streams. The entity state bucket IS the event log.

### Predicates as Events

The predicate index (3-part keys like `quest.status.claimed`) acts as event channels:

| Traditional Event | Predicate Watch |
|-------------------|-----------------|
| `quest.lifecycle.posted` | `quest.status.posted` |
| `quest.lifecycle.claimed` | `quest.status.claimed` |
| `agent.progression.levelup` | `agent.progression.level` |

**The predicate IS the event type.** Processors watch predicates instead of subscribing to event subjects.

### Three Subscription Patterns

1. **Entity-level**: Watch one entity (`c360.prod.game.board1.quest.abc123`)
2. **Type-level**: Watch all of a type (`c360.prod.game.board1.quest.*`)
3. **Predicate-level**: Watch a predicate index (`quest.status.claimed`)

Processors cache last-known state in memory to detect what changed on each watch update.

See `/docs/02-DESIGN.md` for full details.

### Debugging: Message Logger, Traces, and Trajectories

**Semstreams provides powerful debugging tools - use them.**

#### Message Logger
Enable structured logging of all NATS messages for debugging:
```go
// In tests or debug mode
client := natsclient.NewTestClient(t,
    natsclient.WithKV(),
    natsclient.WithMessageLogger(os.Stdout),  // Log all messages
)
```

The message logger shows:
- Subject patterns and payloads
- KV operations (Put, Update, Delete) with revision numbers
- Timing information for performance debugging

#### Trajectories for Quest Tracing
Every quest has a `TrajectoryID` linking to semstreams traces:
```go
// Quest state includes trajectory reference
quest.TrajectoryID = "traj-xyz"

// Use trajectory to trace full quest history:
// - All events emitted during execution
// - State transitions with timestamps
// - Parent/child relationships for sub-quests
```

#### Message Logger SSE for Live Debugging
The backend exposes a Server-Sent Events endpoint that streams KV changes in real-time.
No NATS CLI required — use curl or the dashboard:
```bash
# Watch all entity state changes
curl -N http://localhost:8080/message-logger/kv/semdragons-local-dev-board1/watch?pattern=*

# Watch all quest state changes
curl -N http://localhost:8080/message-logger/kv/semdragons-local-dev-board1/watch?pattern=quest.state.*

# Watch a specific entity
curl -N http://localhost:8080/message-logger/kv/semdragons-local-dev-board1/watch?pattern=quest.state.abc123
```

The dashboard at `http://localhost:5173` subscribes to this same SSE endpoint automatically
and displays live entity updates in the event feed.

#### Debug Patterns
1. **State not updating?** Check KV revisions - CAS conflicts mean concurrent updates
2. **Event not received?** Verify predicate registration and subject patterns
3. **Index stale?** Presence keys should be empty values, not missing
4. **Test flaky?** Check for timing issues in event propagation

### Payload Registry is Critical
The payload registry is how we pass typed data across the event bus. Even though NATS transmits raw bytes, the payload registry enables:
- Immediate unmarshalling to concrete types
- Type-safe event handling
- Self-documenting event schemas

**Always register payload types.** Without proper registry entries, you lose the ability to work with typed data and fall back to manual byte parsing.

## Trust Tiers

| Level | Tier | Capabilities |
|-------|------|--------------|
| 1-5 | Apprentice | Read-only, summarize, classify |
| 6-10 | Journeyman | Tools, API requests, staging writes |
| 11-15 | Expert | Prod writes, deployments, money |
| 16-18 | Master | Agent supervision, quest decomposition, party lead |
| 19-20 | Grandmaster | DM delegation, guild management |

## Package Structure

The root `semdragons` package re-exports type aliases from `domain/` and provides entity helpers:
- `types.go` — Type aliases (AgentID, QuestID, TrustTier, etc.) re-exported from `domain/`
- `graphable.go` + `graphclient.go` — Entity state serialization and NATS KV read/write
- `entityid.go` — Six-part entity ID construction helpers
- `dm.go` — DungeonMaster interface and GameEvent definitions
- `social.go` — Party and Guild structure types
- `vocab.go` — Vocabulary predicate registration
- `domain.go` + `config.go` — Domain configuration types and BoardConfig

**`domain/`** is the authoritative source for all enum types (`SkillTag`, `TrustTier`, `QuestDifficulty`, `QuestStatus`, `DMMode`, etc.) and `BoardConfig`. Prefer importing from `domain/` directly in processors; root package aliases are for external consumer convenience.

**`domains/`** (plural) contains three concrete domain implementations: `software.go`, `dnd.go`, `research.go`. Each defines a `DomainConfig` (vocabulary mapping + skill taxonomy) and a `DomainCatalog` (prompt fragments for `promptmanager`). See `/docs/06-DOMAINS.md` for details.

**`processor/`** contains 17 reactive components registered via `componentregistry/`, plus `promptmanager` and `boardcontrol` which are libraries (not standalone components). Each processor follows the same structure: `component.go`, `config.go`, `register.go`, `handler.go`. See "Processor Architecture Patterns" below.

Two components form the **agentic integration layer** that bridges quest lifecycle to semstreams' event-driven LLM execution:
- `questbridge` — Watches quest entities for `in_progress` transitions, assembles `TaskMessage` (prompt, tools, metadata), publishes to AGENT JetStream stream, handles loop completion/failure events. Uses KV twofer bootstrap protocol for crash recovery via QUEST_LOOPS bucket.
- `questtools` — Consumes `tool.execute.*` from AGENT stream, enforces tier/skill/sandbox gates via `executor.ToolRegistry`, publishes `tool.result.*` back. Reconstructs agent/quest context from `ToolCall.Metadata` to avoid KV round-trips on the hot path.
- `questdagexec` — Reactive DAG execution for party quest decompositions — watches sub-quest KV transitions, drives node assignment via `ClaimQuestForParty`, dispatches lead review tool calls, aggregates outputs for rollup, and escalates the parent quest on node exhaustion. DAG state stored as `quest.dag.*` predicates on the parent quest entity in the graph.

**`processor/dmworldstate/`** aggregates all entity state into a single world-state snapshot consumed by the REST API's `/api/game/world` endpoint.

**`service/api/`** implements the REST API as a `service.Service` that registers HTTP handlers with the semstreams service manager. It reads entity state via `GraphClient` and delegates writes using `EmitEntity`/`EmitEntityUpdate`. API docs are auto-generated from the OpenAPI spec in `service/api/openapi.go` and served at `/docs` (Swagger UI) and `/openapi.json`.

**`config/`** contains two JSON config files:
- `semdragons.json` — Default runtime config (platform, services, streams, components, model_registry)
- `models/` — Per-provider model registry overlays (gemini.json, openai.json, anthropic.json, etc.)

See `/docs/07-MODEL-REGISTRY.md` for LLM provider configuration details.

**`componentregistry/`** is the single location that imports all processors and wires them into the semstreams component registry. Register new processors here.

**`cmd/semdragons/`** is the binary entry point: CLI flags, config loading, NATS connection, stream/bucket init, service manager setup, graceful shutdown.

**`ui/`** is the SvelteKit 5 dashboard. Vite proxies `/game`, `/health`, and `/message-logger` to the backend. Uses a single `worldStore.svelte.ts` reactive store fed by SSE via the message-logger endpoint.

Documentation in `/docs/`:
- `01-GETTING-STARTED.md` — Prerequisites, Docker Compose quickstart, first quest walkthrough
- `02-DESIGN.md` — Architecture, concept map, trust tiers, example flows, death mechanics
- `03-QUESTS.md` — Quest creation, lifecycle state machine, difficulty/XP, boss battles, chains
- `04-PARTIES.md` — Party formation, roles, peer reviews, feedback loop into prompts
- `05-BOIDS.md` — Emergent quest-claiming, six rules, guild/reputation integration, tuning guide
- `06-DOMAINS.md` — Domain system, prompt catalogs, skill taxonomies
- `07-MODEL-REGISTRY.md` — LLM provider configuration, capabilities, fallback chains
- `08-SANDBOX-REPOS.md` — Sandbox-owned git repos, worktree-per-quest model, merge-to-main quality gate, semsource integration
- `adr/001-dm-chat-routing.md` — DM chat mode routing and orchestration design
- API docs served live at `/docs` (Swagger UI) and `/openapi.json` — defined in `service/api/openapi.go`

## Development Commands

Uses [go-task](https://taskfile.dev) (`Taskfile.yml` + `taskfiles/`). Run `task --list` for all targets.

```bash
task build                  # Build all packages
task test                   # Run unit tests only (fast, no Docker)
task test:integration       # Run integration tests only (requires Docker)
task test:all               # Run all tests (unit + integration)
task test:one -- TestName   # Run specific unit test
task test:one-integration -- TestName  # Run specific integration test
task lint                   # Run revive + go vet
task check                  # Full check: fmt, tidy, lint, test-all
task test:coverage          # Generate coverage report (includes integration)
task docker:up              # Start stack with mock LLM
task docker:down            # Stop stack
task e2e                    # E2E tests with mock LLM
task e2e:gemini             # E2E with Gemini
task e2e:spec -- name       # Single spec against running stack
```

### E2E Active Monitoring Protocol (MANDATORY)

**E2E tests with real LLM providers are long-running. You MUST monitor them actively — never block in foreground waiting for completion.**

#### Launch Pattern
1. Run `DEBUG=1 task e2e:gemini` (or other provider) via `run_in_background: true`
2. `DEBUG=1` keeps the stack alive after tests finish for post-mortem inspection

#### Monitor Three Data Sources In Parallel While Tests Run
Check every 20-30s. Do NOT wait for test completion before investigating.

1. **Test output**: `TaskOutput` (non-blocking) to see which test is running, pass/fail, timing
2. **Backend logs** (filtered — debug is extremely noisy):
   ```bash
   docker compose -f docker/compose.yml logs --since=30s backend 2>&1 | \
     grep -iE '(quest|agentic|loop|bridge|model|error|fail|complet|submit|tool|clarif|400|429)' | \
     grep -v 'community\|pivot\|k-core\|structural\|graph-cluster\|predicate index\|embedding' | tail -30
   ```
3. **Trajectories** — fetch when a quest enters agentic loop:
   ```bash
   # Find loop_id from questbridge log: "published TaskMessage ... loop_id=..."
   curl -s http://localhost:8081/game/trajectories/{loop_id}
   ```

#### Dump Evidence to Files
For post-mortem analysis, dump logs and trajectory data to `/tmp/` files rather than depending on Playwright assertions or terminal output:
```bash
docker compose -f docker/compose.yml logs backend > /tmp/e2e-backend.log 2>&1
curl -s http://localhost:8081/game/trajectories/{loop_id} > /tmp/e2e-trajectory.json
curl -s http://localhost:8081/game/world > /tmp/e2e-world.json
```

#### Error Artifacts
- `ui/test-results/*/error-context.md` — page snapshot at failure time

#### Rules
- **Always use task commands** (`task e2e:gemini`, etc.) — never raw docker compose
- **Abort early** if logs show the quest is stuck in a loop, hitting errors, or burning tokens on clarification cycles
- **Report findings with evidence** — quote specific log lines, trajectory data, model responses. Never guess at root cause when data is available.
- **Quest lifecycle trace**: posted → claimed → in_progress → (agentic loop) → submitted/completed/failed/escalated. Cross-reference timestamps across all three data sources.

### Test Categories

Tests are separated using Go build tags for faster feedback loops:

| Category | Tag | Docker Required | Files |
|----------|-----|-----------------|-------|
| **Unit** | (none) | No | Root: `graphable_test.go`, `validation_test.go`, `reconstruction_test.go`, `namegen_test.go`, `trajectory_test.go`. Processors: `processor/promptmanager/*_test.go`, `processor/executor/executor_test.go` |
| **Integration** | `//go:build integration` | Yes (NATS) | `processor/*/component_test.go` (questboard, bossbattle, agentprogression, agentstore, autonomy, boidengine, guildformation, partycoord, etc.) |

**During development**: Use `task test` for fast iteration (unit tests only).
**Before committing**: Use `task test:all` to run the full suite.

Integration tests use `natsclient.NewTestClient(t, natsclient.WithKV())` which spins up NATS via testcontainers.

Module: `github.com/c360studio/semdragons`
Depends on: `github.com/c360studio/semstreams`

## Key Interfaces

Core interfaces are defined inline in domain packages and satisfied by processor components registered via `componentregistry/`. The primary entry points:

- **`GraphClient`** (`graphclient.go`) — Entity state read/write against NATS KV
- **`service/api.Service`** — REST API backed by GraphClient and processor components
- **`processor/questboard.Component`** — Quest lifecycle (post, claim, start, submit, complete, fail, abandon)
- **`processor/bossbattle.Component`** — Review evaluation triggered by quest submission
- **`processor/autonomy.Component`** — Heartbeat-driven agent decision loop with DM approval gate
- **`processor/boidengine.Component`** — Periodic boid attraction computation, publishes suggestions per agent

## Processor Architecture Patterns

Every processor in `processor/` follows the same structure. When adding a new processor:

**Component struct**: Fields are config pointer, deps references, graph client, logger, board config, optional component references, atomic state (`running` bool, `stop` chan), and metric atomics.

**`register.go`**: Single exported function `Register(registry *component.Registry) error` using `registry.Register(ComponentName, NewFromConfig)`.

**`NewFromConfig`**: Receives raw JSON config and `component.Dependencies`. Call `DefaultConfig()`, unmarshal if rawConfig is non-empty, validate, and construct. Never store the full `deps` struct beyond initialization.

**`Start`/`Stop` lifecycle**: `Start(ctx)` launches KV watchers and goroutines, sets `running` atomic. `Stop(timeout)` closes stop channel and waits for goroutines.

**KV watchers**: Use `deps.NATSClient.WatchKV(ctx, bucketName, pattern)`. Cache last-known entity state in a map keyed by entity ID. On each watch update, compare new state to cached state to detect what changed.

**Lock ordering**: If the component has both a lifecycle mutex and a per-entity map mutex, always acquire lifecycle mutex first. Document this in a comment.

**Emitting state changes**: Use `graph.EmitEntityUpdate(ctx, entity, "domain.verb")` for updates, `graph.EmitEntity(ctx, entity, "domain.verb")` for new entities. The predicate string becomes the event type in the trajectory.

**Registration**: After implementing, add your processor's `Register` function to both `RegisterAll` and `RegisterProcessors` in `componentregistry/register.go`, and optionally add config to `config/semdragons.json`.

## Code Patterns

**Fluent Builders**: Use for complex object construction
```go
quest := NewQuest("title").
    Difficulty(DifficultyEpic).
    RequireSkills(SkillAnalysis).
    XP(500).
    ReviewAs(ReviewStrict).
    Build()
```

**Strong Typing**: Semantic ID types (AgentID, QuestID, etc.) prevent mixing identifiers

**Context-First**: All I/O operations take `context.Context` as first parameter

**Lint Warnings**: Always fix lint warnings properly—never game them. Warnings exist because they often point to code smells and anti-patterns. If revive flags something, fix the underlying issue rather than suppressing or working around it. When a warning seems wrong for a specific case, add a `//nolint` directive with a comment explaining why.

**Unused Parameters (`_`)**: Never silence linter warnings by blindly using `_` for unused parameters. This is lazy and breaks lifecycle control. Instead:
- **If it's context**: You almost certainly need it. Add cancellation checks in loops, pass it to called functions, or use it for timeouts.
- **If it's a callback parameter**: Use it to extract data (e.g., parse `msg.Subject` or `msg.Data`).
- **If truly unused**: Add a comment explaining WHY it's not needed (e.g., "Mock uses in-memory lookup - no context needed for synchronous local access").
- **If reserved for future use**: Document the intent (e.g., "TODO: Use strategy to adjust scoring when implemented").

Bad: `func foo(_ context.Context)` - Silences linter without thought
Good: Check `ctx.Done()` in loops, use for timeouts, pass to callees

**Interfaces Over Implementations**: Core domain defined as interfaces with multiple possible backends

## Skills

Practical helpers in `.claude/skills/`:

| Skill | Use When |
|-------|----------|
| `/payload` | Registering a new event payload type |
| `/event-handler` | Creating a reactive handler for an event |
| `/game-event` | Defining a new GameEvent with trajectory mapping |
| `/quest-handler` | Handling quest lifecycle events (claim, complete, fail) |
| `/utils` | Quick reference for semstreams utility packages |

## Open Items

API endpoints that return 501 Not Implemented:
- `POST /api/game/dm/intervene/{questId}` — DM quest intervention

Components enabled in the default config (`config/semdragons.json`):
- `graph-ingest`, `graph-index`, `graph-query` — semstreams entity persistence and indexing
- `questboard` — quest lifecycle state machine
- `bossbattle` — automated review evaluation
- `agent_progression` — XP calculation and leveling
- `agent_store` — XP marketplace (tools, consumables)
- `guildformation` — auto guild clustering
- `boidengine` — emergent quest-claiming suggestions
- `agentic-loop`, `agentic-model` — semstreams event-driven LLM loop orchestration
- `questbridge`, `questtools` — quest-to-LLM bridge (requires model_registry and AGENT stream)
- `questdagexec` — party quest DAG execution (requires questbridge, questtools)

Processors registered but excluded from the default config (opt-in):
- `executor` — synchronous LLM execution (superseded by questbridge+questtools for event-driven execution)
- `autonomy` — depends on executor; DM approval gate is implemented but untested with real LLM calls
- `seeding` — requires explicit config; `ModeTrainingArena` needs LLM, `ModeTieredRoster` works without
- `partycoord` — party lifecycle management (form, assign, merge, disband)
- `dmworldstate` — world state aggregation (used by API but can run standalone)
- `dmsession`, `dmapproval`, `dmpartyformation` — DM session management (functional, not in default config)

**Adding a new processor**: implement in `processor/<name>/`, register in `componentregistry/register.go`, optionally add to `config/semdragons.json`.

**Adding a new API endpoint**: add handler in `service/api/handlers.go`, register route in `service/api/service.go` `RegisterHTTPHandlers()`.
