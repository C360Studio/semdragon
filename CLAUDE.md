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
```

### Think Reactive, Not Objects
- Events flow through the system; don't model everything as objects with methods
- Handlers react to events; they don't call methods on other services
- State changes emit events; consumers react independently
- Avoid request/response patterns when pub/sub fits better

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

#### KV Watch for Live Debugging
Watch state changes in real-time:
```bash
# NATS CLI - watch all quest state changes
nats kv watch semdragons-org-platform-board "quest.state.>"

# Watch specific quest
nats kv watch semdragons-org-platform-board "quest.state.q-abc123"

# Watch all indices
nats kv watch semdragons-org-platform-board "quest.status.>"
```

#### Event Subscription for Debugging
Subscribe to events during debugging:
```bash
# All quest lifecycle events
nats sub "quest.lifecycle.>"

# All events for a specific quest
nats sub "*.*.*.quest.abc123"
```

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

## Current State

The project has core types and interfaces defined in the root `semdragons` package:
- `types.go` - Core domain types (Agent, Quest, TrustTier, etc.)
- `questboard.go` - QuestBoard interface and QuestBuilder
- `xp.go` - XPEngine interface and DefaultXPEngine
- `boids.go` - BoidEngine interface and rules
- `dm.go` - DungeonMaster interface and GameEvents
- `social.go` - Party and Guild structures

Design documentation in `/docs/DESIGN.md`.

## Development Commands

```bash
make build          # Build all packages
make test           # Run unit tests only (fast, no Docker)
make test-integration  # Run integration tests only (requires Docker)
make test-all       # Run all tests (unit + integration)
make test-one TEST=TestName  # Run specific unit test
make test-one-integration TEST=TestName  # Run specific integration test
make lint           # Run golangci-lint
make check          # Full check: fmt, tidy, lint, test-all
make coverage       # Generate coverage report (includes integration)
```

### Test Categories

Tests are separated using Go build tags for faster feedback loops:

| Category | Tag | Docker Required | Files |
|----------|-----|-----------------|-------|
| **Unit** | (none) | No | `evaluator_test.go`, `boids_test.go`, `trajectory_test.go`, `skill_progression_test.go`, `judge_test.go` |
| **Integration** | `//go:build integration` | Yes (NATS) | `store_test.go`, `board_test.go`, `party_coordination_test.go`, `progression_test.go`, `guild_formation_test.go`, `namegen_test.go`, `dm_test.go` |

**During development**: Use `make test` for fast iteration (unit tests only).
**Before committing**: Use `make test-all` to run the full suite.

Integration tests use `natsclient.NewTestClient(t, natsclient.WithKV())` which spins up NATS via testcontainers.

Module: `github.com/c360studio/semdragons`
Depends on: `github.com/c360studio/semstreams`

## Key Interfaces

- **QuestBoard**: Pull-based work distribution (post, claim, submit, complete)
- **XPEngine**: Calculate XP, apply bonuses/penalties, manage leveling
- **BoidEngine**: Compute quest attractions, suggest claims, emergent behavior
- **DungeonMaster**: Top-down orchestration with 4 modes (FullAuto → Manual)

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

## Roadmap (from DESIGN.md)

- [x] Implement QuestBoard backed by semstreams
- [x] Implement DefaultBoidEngine with six rules
- [x] Wire up XP engine with real boss battle evaluators
- [x] Build DM interface (ManualDM complete, automation modes pending)
- [x] Semstreams integration: Map GameEvents to trajectory spans
- [x] Build guild auto-formation based on agent performance clustering
- [x] Dashboard: "The DM's scrying pool"
- [x] Agent Store System: XP-based marketplace for tools and consumables
