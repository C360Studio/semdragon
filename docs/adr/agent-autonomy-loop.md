# ADR: Agent Status Lifecycle and Autonomy Processor

## Status
Proposed

## Context

Two related problems prevent agents from behaving autonomously:

### Problem 1: The Open Loop

The boid engine computes attraction scores on a 1-second ticker and publishes `SuggestedClaim` payloads to `boid.suggestions.{agent}` NATS subjects. Nothing subscribes to these subjects. There is no idle detection, no autonomy loop, and no mechanism for agents to act on their own. The system is purely reactive to state changes: if nothing changes, nothing happens.

```
Boid Engine ──compute──> boid.suggestions.agent-xyz ──> /dev/null

Agent (idle) ────────────────────────────────────────> sits forever
```

The boid engine is the brain, but there are no legs.

### Problem 2: Broken Status Lifecycle

`AgentStatus` defines five states (`idle`, `on_quest`, `in_battle`, `cooldown`, `retired`) but almost no processor manages transitions between them:

| Transition | Who Should Do It | What Actually Happens |
|------------|------------------|-----------------------|
| `idle → on_quest` | Quest claimed | API handler does it (`handlers.go:320`), QuestBoard processor does NOT |
| `on_quest → in_battle` | Boss battle starts | Nobody sets it |
| `in_battle → idle` | Battle victory | Nobody sets it |
| `in_battle → cooldown` | Battle defeat + penalty | Nobody sets it |
| `on_quest → idle` | Quest completed (no review) | Nobody sets it |
| `on_quest → cooldown` | Quest failed + penalty | Nobody sets it (penalty `CooldownDur` is calculated but never written) |
| `cooldown → idle` | Cooldown expires | Nobody checks or transitions |
| any → `retired` | Agent retirement | API handler does it |

**Consequences of the broken lifecycle:**
- The boid engine skips non-idle agents (`boids.go:140`), but agents are never set to non-idle by the processor layer, so this filter is inert
- The quest board requires `AgentIdle` to claim (`handler.go:857`), but since status is never changed from idle, agents could theoretically claim while already executing a quest
- `MaxConcurrent` quest checks (`handler.go:874`) are dead code because the idle check rejects first
- `CooldownUntil` is checked but never written by any processor
- `WorldStats` counts by status (`world.go:230`) but the counts are meaningless since status is always `idle`

### Problem 3: Idle-Gated Autonomy Is Wrong

Even with a working status lifecycle, gating all autonomy actions on `AgentIdle` would be incorrect. Agents should be able to take certain actions in every non-terminal state:

- An agent **on a quest** should be able to buy a quality shield before their boss battle
- An agent **in a boss battle** should be able to use a consumable (quality shield)
- An agent **on cooldown** should be able to shop, join a guild, or use a cooldown skip consumable

### What Exists Today

| Processor | Pattern | Gap |
|-----------|---------|-----|
| boidengine | Ticker-driven, publishes to `boid.suggestions.*` | Nobody subscribes |
| agentstore | KV watch on agent XP, logs affordable items | No nudge to purchase; no status checks |
| guildformation | KV watch on agent level, triggers auto-formation | No periodic re-check; no status checks |
| dmworldstate | On-demand queries, `RefreshIntervalSec` unused | No proactive refresh |
| agentprogression | KV watch on quest completion/failure | Reactive only; does not update agent status |
| bossbattle | KV watch on quest submission | Reactive only; does not update agent status |
| partycoord | KV watch on quest claims | Reactive only |
| executor | JetStream consumer | Reactive only; does not update agent status |

## Decision

Introduce two things:

1. **Agent Status Lifecycle** — wire status transitions into existing processors so `AgentStatus` reflects reality
2. **Agent Autonomy Processor** (`processor/autonomy/`) — state-aware heartbeat that evaluates what actions an agent can take based on current status

### Part 1: Agent Status Lifecycle

Status transitions are wired into the processors that already handle the underlying events. No new processor is needed for status management — it is a cross-cutting concern handled by each processor at the point where it processes the relevant event.

#### State Machine

```
                    ┌──────────┐
          ┌────────>│   idle   │<────────────────────┐
          │         └────┬─────┘                     │
          │              │                           │
          │         claim quest                 cooldown expires
          │              │                           │
          │              ▼                           │
          │         ┌──────────┐              ┌──────────┐
          │         │ on_quest │──quest fail──>│ cooldown │
          │         └────┬─────┘   (penalty)  └──────────┘
          │              │                           ▲
          │         submit for                       │
          │          review                     battle defeat
          │              │                      (penalty)
          │              ▼                           │
          │         ┌──────────┐                     │
          └─────────│in_battle │─────────────────────┘
        battle      └──────────┘
        victory

  Special transitions:
    any ──retire──> retired (terminal)
    on_quest ──complete (no review)──> idle
    cooldown ──cooldown_skip consumable──> idle
```

#### Who Writes Each Transition

| Transition | Processor | Trigger |
|------------|-----------|---------|
| `idle → on_quest` | **questboard** | `ClaimQuest` succeeds → update agent entity |
| `on_quest → in_battle` | **bossbattle** | Quest enters `in_review` → update agent entity |
| `in_battle → idle` | **bossbattle** | Battle verdict = victory → update agent entity |
| `in_battle → cooldown` | **agentprogression** | Battle defeat + `CooldownDur > 0` → update agent entity with status + `CooldownUntil` |
| `on_quest → idle` | **agentprogression** | Quest completed (victory, no review needed) → update agent entity |
| `on_quest → cooldown` | **agentprogression** | Quest failed + `CooldownDur > 0` → update agent entity with status + `CooldownUntil` |
| `on_quest → idle` | **agentprogression** | Quest failed + no cooldown → update agent entity |
| `cooldown → idle` | **autonomy** | Heartbeat detects `CooldownUntil` has passed → update agent entity |
| any → `retired` | **api handler** | Retirement endpoint (already works) |
| `cooldown → idle` | **agentstore** | `cooldown_skip` consumable used → update agent entity |

**Key design choice**: Each processor updates the agent entity via `graph.EmitEntityUpdate` at the point where it already processes the triggering event. This is a small addition to existing handlers, not a new event chain. The status field becomes a derived fact that is kept in sync with the events that cause transitions.

### Part 2: Agent Autonomy Processor

#### Why Per-Agent Heartbeat Over Global Clock

A global clock (tick every N seconds, evaluate all agents) has three problems:

1. **Thundering herd**: All agents evaluate simultaneously, creating burst load on the quest board. With 50 agents, a single tick produces 50 near-simultaneous claim attempts.

2. **Unfair timing**: An agent that became idle 1ms before the tick waits the same interval as one idle for 29 seconds. The global clock is blind to individual agent timelines.

3. **God-loop smell**: A global clock that iterates all agents and decides what each should do is a central orchestrator, violating the pull-based architecture.

Per-agent heartbeats solve all three:
- **Staggered load**: Each agent's heartbeat fires at its own cadence.
- **Fair evaluation**: Timing is relative to when each agent's state last changed.
- **Pull-based**: Each agent "pulls" its own evaluation cycle.

#### Architecture

```
                    ┌─────────────────────────────────────┐
                    │       AGENT AUTONOMY PROCESSOR       │
                    ├─────────────────────────────────────┤
                    │                                      │
  KV Watch ────────>│  Agent State Cache                   │
  (agent.*)         │  (status, tier, quests, cooldown)    │
                    │                                      │
  NATS Sub ────────>│  Suggestion Cache                    │
  (boid.suggestions │  (latest per agent)                  │
   .*)              │                                      │
                    │  Per-Agent Heartbeat Timers           │
                    │  ┌─────┐ ┌─────┐ ┌─────┐           │
                    │  │ ag1 │ │ ag2 │ │ ag3 │           │
                    │  └──┬──┘ └──┬──┘ └──┬──┘           │
                    │     ▼      ▼      ▼                │
                    │  evaluateAutonomy(agentID)          │
                    │     │                               │
                    │     ├─ state-aware action matrix    │
                    │     │                               │
                    │     ├─ idle: claim, shop, guild     │
                    │     ├─ on_quest: shop, use           │
                    │     ├─ in_battle: use consumable    │
                    │     ├─ cooldown: shop, guild, use   │
                    │     └─ retired: (no heartbeat)      │
                    │                                      │
                    └─────────────────────────────────────┘
```

#### State-Aware Action Matrix

The core insight: **every non-terminal state has valid autonomous actions**. The evaluation is not "is the agent idle?" but "what can this agent do right now?"

| Action | idle | on_quest | in_battle | cooldown | retired |
|--------|------|----------|-----------|----------|---------|
| Claim quest | Yes | No | No | No | No |
| Shop (tools) | Yes | Yes | No | Yes | No |
| Shop (consumables) | Yes | Yes (strategic) | No | Yes | No |
| Use consumable | Yes | Yes | Yes (quality shield) | Yes (cooldown skip) | No |
| Join guild | Yes | Yes | No | Yes | No |
| Idle heartbeat | Yes | No | No | No | No |

**Rationale for each restriction:**
- **in_battle**: Agent is being evaluated. Only consumable use (quality shield) is valid — you can't go shopping or claim quests during a review.
- **cooldown**: Agent is penalized but not incapacitated. Shopping and social actions are encouraged — "downtime activities" while waiting.
- **on_quest**: Agents handle one quest at a time. Shopping mid-quest lets agents prepare for the upcoming boss battle.
- **retired**: Terminal state. Heartbeat is never started.

#### Heartbeat Lifecycle

Heartbeats are **not idle-only**. Every non-retired agent gets a heartbeat. The heartbeat cadence varies by state:

```
Agent state changes (any KV watch update)
    │
    ▼
Determine heartbeat parameters from new state
    │
    ├── retired → cancel heartbeat, remove from cache
    │
    ├── idle → fast heartbeat (base_interval_ms)
    │          Primary goal: find quests
    │
    ├── on_quest → slow heartbeat (on_quest_interval_ms)
    │              Primary goal: strategic shopping, consumable use
    │
    ├── in_battle → very slow heartbeat (in_battle_interval_ms)
    │               Primary goal: consumable use only
    │
    └── cooldown → medium heartbeat (cooldown_interval_ms)
                   Primary goal: shopping, guild, cooldown skip
```

**Heartbeat parameters:**

| Parameter | Default | Description |
|-----------|---------|-------------|
| `initial_delay_ms` | 2000 | Grace period before first evaluation after any state change |
| `idle_interval_ms` | 5000 | Heartbeat interval when idle (quest-seeking mode) |
| `on_quest_interval_ms` | 30000 | Heartbeat interval when on quest (low-urgency) |
| `in_battle_interval_ms` | 60000 | Heartbeat interval when in battle (consumable check only) |
| `cooldown_interval_ms` | 15000 | Heartbeat interval when on cooldown (downtime activities) |
| `max_interval_ms` | 60000 | Maximum backoff interval for idle agents |
| `backoff_factor` | 1.5 | Multiplicative backoff when no action is taken (idle only) |

Backoff applies **only to idle agents** (repeated no-action evaluations). Non-idle agents use fixed intervals because their state is temporary and will change via external events.

#### Autonomy Evaluation Cycle

Each heartbeat triggers a state-aware evaluation:

```go
func (c *Component) evaluateAutonomy(agentID string) {
    agent := c.getAgent(agentID)
    if agent == nil || agent.Status == AgentRetired {
        c.cancelHeartbeat(agentID)
        return
    }

    // Build action set for current state
    actions := c.actionsForState(agent.Status)

    // Evaluate in priority order
    for _, action := range actions {
        if action.shouldExecute(agent) {
            action.execute(agent)
            return
        }
    }

    // Nothing to do
    if agent.Status == AgentIdle {
        c.emitIdleHeartbeat(agent)
        c.backoffHeartbeat(agentID)
    }
}

func (c *Component) actionsForState(status AgentStatus) []Action {
    switch status {
    case AgentIdle:
        return []Action{
            c.claimQuestAction,      // Highest priority
            c.useConsumableAction,   // Use stored items
            c.shopAction,            // Buy tools/consumables
            c.joinGuildAction,       // Social
        }
    case AgentOnQuest:
        return []Action{
            c.shopStrategicAction,   // Buy quality shield before boss battle
            c.useConsumableAction,   // Use XP boost, insight scroll
            c.joinGuildAction,       // Social (guild discount)
        }
    case AgentInBattle:
        return []Action{
            c.useConsumableAction,   // Quality shield only
        }
    case AgentCooldown:
        return []Action{
            c.useCooldownSkipAction, // Highest priority: end cooldown early
            c.shopAction,            // Downtime shopping
            c.joinGuildAction,       // Social
        }
    default:
        return nil
    }
}
```

**Priority order per state:**

**Idle**: Claim quest > use consumable > shop > join guild
- Quest claiming is the primary purpose. Everything else is opportunistic.

**On Quest**: Shop strategic > use consumable > join guild
- Strategic shopping (quality shield) prepares for the upcoming boss battle. Agents focus on one quest at a time.

**In Battle**: Use consumable (quality shield only)
- Very limited. The quality shield is the only meaningful action during review.

**Cooldown**: Use cooldown skip > shop > join guild
- Getting back to work is highest priority. Shopping and social fill the remaining downtime.

#### Cooldown Expiry Detection

The autonomy processor is responsible for detecting cooldown expiry and transitioning agents back to idle. This is checked on each cooldown heartbeat:

```go
func (c *Component) checkCooldownExpiry(agent *Agent) bool {
    if agent.Status != AgentCooldown {
        return false
    }
    if agent.CooldownUntil == nil || time.Now().After(*agent.CooldownUntil) {
        // Cooldown expired — transition to idle
        c.transitionToIdle(agent)
        return true
    }
    return false
}
```

This is appropriate because:
- No other processor has a periodic check against `CooldownUntil`
- The autonomy processor already has per-agent timers
- The cooldown heartbeat interval (15s) is granular enough for timely detection

#### Idle Time Tracking

Idle time is derived from agent entity state, not stored separately. The processor caches `idleSince` per agent:

```go
type agentTracker struct {
    agent        *semdragons.Agent
    idleSince    time.Time     // When agent entered idle (process-local)
    heartbeat    *time.Timer   // Per-agent heartbeat timer
    interval     time.Duration // Current heartbeat interval
    suggestion   *SuggestedClaim // Latest boid suggestion (idle agents only)
}
```

The `idleSince` timestamp is process-local, not persisted. On restart, all idle agents bootstrap with `idleSince = now`, which provides a natural grace period.

#### Boid Suggestion Consumption

The processor subscribes to `boid.suggestions.>` (NATS pub/sub, not KV — suggestions are ephemeral recommendations):

```go
c.deps.NATSClient.Subscribe("boid.suggestions.>", func(msg *nats.Msg) {
    var suggestion boidengine.SuggestedClaim
    json.Unmarshal(msg.Data, &suggestion)

    instance := semdragons.ExtractInstance(string(suggestion.AgentID))
    c.setSuggestion(instance, &suggestion)

    // Reset heartbeat for this agent (new information available)
    c.resetHeartbeat(instance)
})
```

Only the latest suggestion per agent is cached. The boid engine recomputes every second, so older suggestions are stale.

#### Intent Events vs Direct Actions

The autonomy processor emits **intent predicates** that route through DM approval:

```
Autonomy proc ──> emit agent.autonomy.claimintent ──> DM approval ──> QuestBoard.ClaimQuest
```

In FullAuto DM mode, the processor short-circuits directly to the target API. The DM mode determines mediation:

| DM Mode | Intent Handling |
|---------|-----------------|
| FullAuto | Direct API call (quest board, store, guild) |
| Supervised | Intent emitted, auto-approved unless flagged |
| Guided | Intent emitted, DM reviews before acting |
| Manual | Intent emitted, DM must explicitly approve |

#### Trust Tier Gating

Pre-flight checks prevent noisy rejected intents:

| Action | Tier Gate | Rationale |
|--------|-----------|-----------|
| Claim quest | `agent.Tier >= quest.MinTier` | Enforced by quest board, checked early |
| Shop store | `agent.Tier >= item.MinTier` | Apprentices see fewer items |
| Join guild | `agent.Level >= guild.MinLevel` | Guild minimum level |
| Lead party claim | `agent.Tier >= TierMaster` | Party leadership requires Master+ |

### New Predicates

Following the three-part `domain.category.property` convention:

```go
// Agent status lifecycle predicates
const (
    // PredicateAgentStatusChanged - Agent status transitioned.
    PredicateAgentStatusChanged = "agent.status.changed"
)

// Agent autonomy predicates
const (
    // PredicateAutonomyEvaluated - Heartbeat fired, agent evaluated.
    PredicateAutonomyEvaluated = "agent.autonomy.evaluated"

    // PredicateAutonomyClaimIntent - Agent intends to claim a quest.
    PredicateAutonomyClaimIntent = "agent.autonomy.claimintent"

    // PredicateAutonomyShopIntent - Agent intends to purchase from store.
    PredicateAutonomyShopIntent = "agent.autonomy.shopintent"

    // PredicateAutonomyGuildIntent - Agent intends to join a guild.
    PredicateAutonomyGuildIntent = "agent.autonomy.guildintent"

    // PredicateAutonomyUseIntent - Agent intends to use a consumable.
    PredicateAutonomyUseIntent = "agent.autonomy.useintent"

    // PredicateAutonomyIdle - Agent evaluated, nothing actionable.
    PredicateAutonomyIdle = "agent.autonomy.idle"
)
```

Note: `agent.autonomy.useintent` is new compared to v1 — consumable use is now an autonomous action, not just a request-driven API call.

### Shopping Heuristics

Shopping behavior varies by agent state:

**Idle shopping** — conservative, surplus XP only:
```go
func (c *Component) shouldShopIdle(agent *Agent) bool {
    xpSurplus := agent.XP - agent.XPToLevel
    if xpSurplus < c.config.MinXPSurplusForShopping {
        return false
    }
    return c.hasUsefulAffordableItems(agent, xpSurplus)
}
```

**On-quest strategic shopping** — targeted consumable acquisition:
```go
func (c *Component) shouldShopOnQuest(agent *Agent) bool {
    // Only buy consumables that help with the current quest
    // Quality shield: protects against failed review criterion
    // XP boost: doubles XP from current quest completion
    if !agent.HasConsumable("quality_shield") && c.canAfford(agent, "quality_shield") {
        return true
    }
    if !agent.HasConsumable("xp_boost") && c.canAfford(agent, "xp_boost") {
        return true
    }
    return false
}
```

**Cooldown shopping** — broad browsing, nothing else to do:
```go
func (c *Component) shouldShopCooldown(agent *Agent) bool {
    // More liberal during cooldown — agent has nothing better to do
    // Buy anything useful at any XP level (no surplus requirement)
    return c.hasUsefulAffordableItems(agent, agent.XP)
}
```

### Interaction with Existing Processors

```
              Status Lifecycle Writes              Autonomy Processor
              ─────────────────────               ──────────────────
Boid Engine ───────────────────────────────────> suggestions consumed
                                                        │
QuestBoard ── claim ── writes on_quest ─────────────────┤
                                                        │
BossBattle ── start ── writes in_battle                 ├── intent events
             verdict ── writes idle/cooldown             │
                                                        │
AgentProg  ── quest complete ── writes idle     ────────┤
             quest fail ── writes cooldown               │
                                                        │
Autonomy   ── cooldown expired ── writes idle           │
             evaluates actions per state ───────────────┘
                                                        │
                                                        ▼
                                              Quest Board / Store / Guild
```

**No existing processor needs new dependencies.** Each processor adds a small status-write to its existing event handler. The autonomy processor is the only new component.

### Knowledge Graph Alignment

The store system must follow the same entity-centric architecture as every other processor. All state is
entity state in KV. The KV write IS the event.

**Principles:**

1. **All processor state must be entity state in KV.** In-memory caches (sync.Map) are read-through
   optimizations, not sources of truth. On restart, state is reconstructed from KV.

2. **Entities have triples; relationships are edges between entities.** "Agent owns tool" is a triple on
   the agent entity with the storeitem entity ID as its object — a proper graph edge:
   `(agent.dragon) --owns--> (storeitem.web_search)`.

3. **Store items are entities.** Type `storeitem` with 6-part entity IDs (e.g.,
   `c360.prod.game.board1.storeitem.web_search`). Seeded to KV at startup. Queryable by any processor via
   `graph.GetEntityDirect()`.

4. **Inventory is agent entity triples, not a separate entity.** Follows the existing pattern where guild
   memberships and skill proficiencies are agent triples. New predicates use the dynamic prefix pattern:
   - `agent.inventory.tool.{itemID}` → entity ref to storeitem (relationship edge)
   - `agent.inventory.consumable.{itemID}` → count owned
   - `agent.inventory.total_spent` → int64
   - `agent.effects.{effectType}.remaining` → quests remaining

5. **Purchase and UseConsumable use read-modify-write.** Read agent from KV → reconstruct → mutate
   inventory + XP → `EmitEntityUpdate`. Same pattern as every other processor.

6. **"The KV write IS the event" applies to store operations.** No separate event streams needed — KV
   watchers see inventory changes as agent entity updates.

## Consequences

### Positive

- Agent status reflects reality — `WorldStats`, boid engine filtering, and quest board validation all become meaningful
- Closes the boid engine's open loop: suggestions are consumed and acted upon
- Agents are autonomous in every non-terminal state, not just idle
- Mid-quest shopping enables strategic consumable use (quality shields, XP boosts)
- Cooldown is productive time (shopping, guild joining, cooldown skip)
- Per-agent heartbeats prevent thundering herd
- Observable via standard predicates

### Negative

- Status writes in existing processors are a cross-cutting change (touches questboard, bossbattle, agentprogression, agentstore)
- Multiple heartbeat cadences add configuration complexity
- Strategic shopping heuristics are opinionated and may need tuning

### Neutral

- Boid suggestions remain ephemeral NATS messages (correct: recommendations, not facts)
- `idleSince` is process-local, not persisted (lost on restart = grace period)
- Heartbeat timers: one per non-retired agent. For 1,000 agents with Go's `time.AfterFunc` this is trivial

## Implementation Plan

### Phase 0: Agent Status Lifecycle (prerequisite)

Wire status transitions into existing processors. This is the foundation — without it, the autonomy processor's state-aware evaluation is meaningless.

**Changes to existing processors:**

1. **questboard** `ClaimQuest`: After quest entity update, also update agent entity with `Status: AgentOnQuest`, set `CurrentQuest`

2. **bossbattle**: On battle start (`quest.status → in_review`), update agent entity with `Status: AgentInBattle`. On battle victory, update agent with `Status: AgentIdle`, clear `CurrentQuest`. On battle defeat, delegate to agentprogression (which handles penalties).

3. **agentprogression**: On quest completed, update agent entity with `Status: AgentIdle`, clear `CurrentQuest`. On quest failed with `CooldownDur > 0`, update agent with `Status: AgentCooldown`, `CooldownUntil: now + CooldownDur`. On quest failed without cooldown, same as completed.

4. **questboard** `validateAgentCanClaim`: Replace hard idle gate with state-aware validation (reject `retired`, `in_battle`, `cooldown`, `on_quest`). Agents handle one quest at a time.

5. **agentstore** `UseConsumable`: When `cooldown_skip` is used, update agent with `Status: AgentIdle`, clear `CooldownUntil`.

**Testing (Phase 0):**

| Test | Type | Description |
|------|------|-------------|
| `TestStatusTransition_ClaimSetsOnQuest` | Integration | Claim quest → agent status = on_quest |
| `TestStatusTransition_CompleteSetsIdle` | Integration | Complete quest → agent status = idle |
| `TestStatusTransition_FailSetsCooldown` | Integration | Fail quest with penalty → agent status = cooldown, CooldownUntil set |
| `TestStatusTransition_BattleStartSetsInBattle` | Integration | Submit for review → agent status = in_battle |
| `TestStatusTransition_BattleVictorySetsIdle` | Integration | Battle victory → agent status = idle |
| `TestCooldownSkipTransition` | Integration | Use cooldown_skip → status = idle, CooldownUntil cleared |

### Phase 1: Autonomy Processor Skeleton

Create `processor/autonomy/` following the standard component pattern:

| File | Purpose |
|------|---------|
| `component.go` | Component struct, Discoverable + LifecycleComponent interfaces |
| `config.go` | Config with per-state heartbeat intervals, backoff, thresholds |
| `register.go` | Factory, registry integration |
| `handler.go` | KV watch for agent state, NATS sub for boid suggestions |
| `heartbeat.go` | Per-agent timer management (start, cancel, reset, backoff) |
| `evaluation.go` | State-aware `evaluateAutonomy` with action matrix |
| `actions.go` | Individual action implementations (claim, shop, use, guild) |
| `payloads.go` | Intent event payload types |

**Deliverables:**
- Component boots, watches agent state, subscribes to boid suggestions
- Per-agent heartbeats start on state change with correct cadence per state
- Evaluation stubs emit heartbeat predicate only (no actions yet)
- Cooldown expiry detection transitions agents to idle

### Phase 2: Quest Claim Integration

Wire quest claiming for idle agents:

1. Consume boid suggestions, cache latest per agent
2. Pre-flight tier/skill validation
3. Emit `agent.autonomy.claimintent` or call `QuestBoard.ClaimQuest` (DM mode dependent)

**Deliverables:**
- Idle agents claim quests based on boid suggestions
- Integration test: post quest → boid suggests → autonomy claims

### Phase 3: Store Shopping and Consumable Use

Wire state-aware shopping and consumable use:

1. Idle: conservative surplus-based shopping
2. On-quest: strategic consumable shopping (quality shield, XP boost)
3. Cooldown: liberal browsing, cooldown skip priority
4. In-battle: quality shield use only

**Deliverables:**
- Agents shop and use consumables based on state and context
- Strategic quality shield purchase before boss battles

### Phase 4: Guild Joining

Wire guild joining for idle, on-quest, and cooldown agents:

1. Check unguilded + Journeyman+
2. Emit `agent.autonomy.guildintent` or call `GuildFormation.JoinGuild`

**Deliverables:**
- Unguilded agents join guilds during any non-battle state

### Phase 5: Predicate Registration and Observability

1. Add lifecycle and autonomy predicates to `domain/vocab.go`
2. Register in `RegisterVocabulary()`
3. Register intent payload types in payload registry
4. Wire intents into DM approval processor for non-FullAuto modes

### Phase 6: Testing

| Test | Type | Description |
|------|------|-------------|
| `TestHeartbeatCadenceByState` | Unit | Idle = fast, on_quest = slow, cooldown = medium |
| `TestHeartbeatCancelOnRetired` | Unit | Retired agents get no heartbeat |
| `TestActionMatrixIdle` | Unit | Idle agents can claim, shop, join guild |
| `TestActionMatrixOnQuest` | Unit | On-quest agents can shop strategic, use, join guild |
| `TestActionMatrixInBattle` | Unit | In-battle agents can only use consumable |
| `TestActionMatrixCooldown` | Unit | Cooldown agents can skip, shop, join guild |
| `TestBackoffIdleOnly` | Unit | Backoff applies only to idle, not other states |
| `TestBoidSuggestionResetHeartbeat` | Unit | New suggestion resets backed-off heartbeat |
| `TestCooldownExpiryDetection` | Integration | Cooldown expires → autonomy transitions to idle → claims quest |
| `TestStrategicShopping` | Integration | Agent on quest buys quality shield before boss battle |
| `TestCooldownSkipAutonomy` | Integration | Cooldown agent uses cooldown_skip, becomes idle, claims quest |
| `TestFullLifecycle` | Integration | Idle → claim → execute → review → victory → idle → claim again |

## References

- `processor/boidengine/` — Suggestion output, `SuggestedClaim` struct
- `processor/questboard/handler.go:856` — `validateAgentCanClaim` (to be updated)
- `processor/agentprogression/handler.go` — XP processing (to add status writes)
- `processor/bossbattle/` — Battle lifecycle (to add status writes)
- `processor/agentstore/` — `Purchase`, `UseConsumable` APIs
- `processor/guildformation/` — `JoinGuild` API
- `service/api/handlers.go:320` — Existing status write pattern (API layer)
- `domain/vocab.go` — Predicate registration
- `docs/adr/entity-centric-architecture.md` — KV watch patterns
- `dm.go` — DungeonMaster modes
