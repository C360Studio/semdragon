# Semdragons: Design Document

## The Elevator Pitch

Agentic workflow coordination modeled as a tabletop RPG, built on semstreams.

Agents are adventurers. Work items are quests. Quality reviews are boss battles.
Trust is earned through leveling. Specialization happens through guilds.
Coordination emerges from Boid-like flocking behavior over a structured quest board.

**It's fun to build. The results are serious.**

---

## Concept Map

| RPG Concept         | Engineering Reality                    | Why It's Better Than "Orchestrator" |
|---------------------|----------------------------------------|-------------------------------------|
| Quest Board         | Pull-based work queue                  | Decoupled, no central bottleneck    |
| Quest               | Task / work item                       | Has difficulty, requirements, rewards |
| Agent               | LLM-powered autonomous worker          | Has progression, not just config    |
| Level / XP          | Progressive trust & capability         | Earned, not declared                |
| Trust Tier          | Permission boundary                    | Derived from demonstrated competence |
| Equipment / Tools   | API access, tool permissions           | Gated by tier, not role            |
| Boss Battle         | Quality gate / review                  | Embedded in flow, not bolted on    |
| Party               | Agent ensemble for complex tasks       | Role differentiation built in      |
| Party Lead          | Orchestrating agent for sub-tasks      | Has skin in the game (faces boss)  |
| Guild               | Specialization cluster                 | Routing + shared knowledge + reputation |
| Death / Cooldown    | Failure backoff                        | Has consequences, self-correcting  |
| Permadeath          | Catastrophic failure retirement        | Prevents repeat offenders          |
| DM (Dungeon Master) | Human or LLM orchestrator              | Explicit control layer with modes  |
| Session             | Workflow execution context             | Bounded, observable, replayable    |
| Boids               | Emergent quest-claiming behavior       | No central scheduler needed        |
| Game Events         | Semstreams event stream                | Full observability via trajectories |
| Guild Library       | Shared agent memory / prompt templates | Knowledge accumulates over time    |

---

## Architecture Layers

```
┌─────────────────────────────────────────────────┐
│                  DUNGEON MASTER                  │
│         (Human / LLM / Hybrid control)          │
├─────────────────────────────────────────────────┤
│                                                  │
│   ┌──────────┐  ┌──────────┐  ┌──────────┐     │
│   │  GUILDS   │  │ PARTIES  │  │  BOIDS   │     │
│   │ (special- │  │ (temp    │  │ (emergent│     │
│   │  ization) │  │  groups) │  │  flock)  │     │
│   └────┬─────┘  └────┬─────┘  └────┬─────┘     │
│        │              │              │           │
│   ┌────▼──────────────▼──────────────▼─────┐    │
│   │            QUEST BOARD                  │    │
│   │   (pull-based work distribution)        │    │
│   └────────────────┬────────────────────────┘    │
│                    │                             │
│   ┌────────────────▼────────────────────────┐    │
│   │         XP ENGINE + BOSS BATTLES        │    │
│   │   (evaluation, leveling, trust gates)   │    │
│   └────────────────┬────────────────────────┘    │
│                    │                             │
├────────────────────▼─────────────────────────────┤
│                 SEMSTREAMS                        │
│   (event streaming, trajectories, observability) │
└──────────────────────────────────────────────────┘
```

---

## Trust Tiers in Detail

```
Level 1-5   │ APPRENTICE   │ Read-only, summarize, classify, simple transforms
            │              │ Tools: grep, read APIs, formatters
            │              │ No external side effects
            │              │
Level 6-10  │ JOURNEYMAN   │ Can call tools, make API requests, write to staging
            │              │ Tools: + HTTP clients, DB reads, file I/O
            │              │ Side effects in sandboxed environments
            │              │
Level 11-15 │ EXPERT       │ Can modify production state, spend money, deploy
            │              │ Tools: + prod DB writes, payment APIs, CI/CD triggers
            │              │ Requires boss battle on every quest
            │              │
Level 16-18 │ MASTER       │ Can supervise agents, decompose quests, lead parties
            │              │ All tools + agent management
            │              │ Can create sub-quests, review other agents
            │              │
Level 19-20 │ GRANDMASTER  │ Can act as DM delegate, create quests, manage guilds
            │              │ Full system access
            │              │ Trusted to make unsupervised decisions
```

---

## Example Flow: "Analyze Q3 Sales Data and Send Summary to Stakeholders"

### 1. DM Creates Quest
```go
quest := NewQuest("Analyze Q3 sales and email stakeholders").
    Description("Pull Q3 data, identify trends, write executive summary, email to VP list").
    Difficulty(DifficultyEpic).
    RequireSkills(SkillAnalysis, SkillDataTransform, SkillCustomerComms).
    RequireParty(3).
    XP(500).
    BonusXP(200).
    MaxDuration(30 * time.Minute).
    ReviewAs(ReviewStrict).  // Dragon-level boss battle
    Build()

board.PostQuest(ctx, quest)
```

### 2. Boids Engine Suggests Party
The Boids engine identifies idle agents with matching skills:
- **DataDragon** (Level 14, Expert, Guild: Data Wranglers) - high affinity for data quests
- **SummaryScribe** (Level 12, Expert, Guild: Analysts) - strong analysis + writing
- **MailHawk** (Level 8, Journeyman, Skills: customer_communications) - can send emails

### 3. DM Forms Party with Mentor Strategy
```go
party := dm.FormParty(ctx, quest.ID, PartyStrategyBalanced)
// DataDragon becomes lead (highest level, can decompose)
// SummaryScribe is executor for analysis
// MailHawk is executor for email delivery
```

### 4. Party Lead Decomposes Quest via DAG

DataDragon's agentic loop receives a `decompose_quest` tool call. It responds with a
DAG proposal — four nodes with explicit dependency edges:

```
Sub-quest 1: "Extract Q3 sales data"           (Moderate, data_transformation) — no deps
Sub-quest 2: "Analyze trends and anomalies"    (Hard, analysis)                — depends on 1
Sub-quest 3: "Write executive summary"         (Moderate, summarization)       — depends on 2
Sub-quest 4: "Email summary to VP distribution list" (Easy, customer_comms)   — depends on 3
```

`questdagexec` validates the DAG (cycle check, member assignments, max 20 nodes) and
persists it as `quest.dag.*` predicates on the parent quest entity in the graph.

### 5. Sub-Quests Execute

`questdagexec` watches KV transitions. Nodes 2-4 start `pending`; node 1 is immediately
`ready`. `questdagexec` calls `ClaimQuestForParty` to route node 1 to DataDragon —
agents do not claim party sub-quests via the public board.

When node 1 completes, node 2 becomes `ready` and is routed to SummaryScribe. Each
completed node unlocks its dependents reactively. Sub-quest 4 (MailHawk) waits until
the summary output from node 3 is available.

Each sub-quest has its own mini boss battle (ReviewAuto or ReviewStandard). After the
boss battle, the node transitions to `pending_review` and DataDragon receives a
`review_sub_quest` tool call. Acceptance (average rating ≥ 3.0) moves the node to
`completed`; rejection injects corrective feedback into the member's retry prompt.

### 6. Party Lead Rolls Up Results

Once all four nodes reach `completed`, `questdagexec` sends DataDragon a
`rollup_outputs` tool call containing all node outputs in topological order. DataDragon
synthesises the final package and returns it as the rollup payload. `questdagexec`
submits this as the parent quest output and disbands the party.

### 7. Boss Battle (Dragon Level)
Multi-judge review panel:
- **Automated judge**: Checks data accuracy, email formatting, recipient list
- **LLM judge**: Evaluates summary quality, insight depth, tone appropriateness
- **LLM judge 2**: Cross-checks analysis against raw data for hallucinations

### 8. XP Distribution
```
DataDragon:    500 base + 180 quality + 50 speed + 75 guild = 805 XP (LEVEL UP → 15!)
SummaryScribe: 350 base + 140 quality + 0 speed + 52 guild  = 542 XP
MailHawk:       50 base +  40 quality + 20 speed + 0 guild   = 110 XP
```

### 9. Events Stream (via semstreams)
```
quest.posted     → {quest_id: "q-123", difficulty: "epic"}
party.formed     → {party_id: "p-456", lead: "DataDragon", members: [...]}
quest.claimed    → {quest_id: "q-123-sub-1", agent: "DataDragon"}
quest.started    → ...
quest.completed  → {quest_id: "q-123-sub-1", quality: 0.92}
battle.started   → {battle_id: "b-789", level: "strict", judges: 3}
battle.victory   → {battle_id: "b-789", quality: 0.89}
agent.level_up   → {agent: "DataDragon", old: 14, new: 15, tier: "expert"}
quest.completed  → {quest_id: "q-123", quality: 0.89}
party.disbanded  → {party_id: "p-456"}
```

---

## Death Mechanics

| Scenario | Consequence | Recovery |
|----------|-------------|----------|
| Soft failure (bad output) | -25% base XP, 2min cooldown | Retry available |
| Timeout | -50% base XP, 5min cooldown | Quest re-posted |
| Abandon | -75% base XP, 10min cooldown | Quest re-posted, agent flagged |
| TPK (party wipe) | All members cooldown, quest escalated | Higher-level party or DM |
| Catastrophic (data loss, breach) | Permadeath, agent retired | New agent, level 1 |
| Repeated failures at level | Level down, XP reset | Must re-earn level |

---

## Design Decisions

The following questions arose during design and are now settled:

- **Guild formation**: Both auto-suggest and DM-created. The `guildformation` processor performs
  automatic clustering based on demonstrated skills and co-performance. The DM can also form guilds
  manually via the API.

- **Inter-guild quests**: The party system handles cross-guild collaboration. Parties can draw
  members from multiple guilds; no separate inter-guild mechanism is needed.

- **Agent memory**: Guild library for persistent cross-quest knowledge; party context for
  quest-scoped memory. Agents are persistent KV entities and retain state across sessions.

- **Boids vs explicit assignment**: Boids is the default. DM modes (`full_auto`, `supervised`,
  `manual`) control how much the DM overrides suggestions. In `supervised` and `manual` modes the
  DM can intercept any boid suggestion before it becomes a claim.

- **Multi-session learning**: Yes. Agents are persistent entities stored in NATS KV; sessions are
  execution contexts, not agent lifetimes. Levels and XP survive restarts.

- **Quest chains**: Supported via `depends_on` with dependency validation. The parent quest remains
  open across sessions until all dependencies resolve.

- **Competitive dynamics**: The Boids engine handles competition implicitly via attraction scores.
  Guild reputation further differentiates agents on shared quest pools, enabling A/B-style
  competitive dynamics without explicit PvP mechanics.

For implementation details see [Getting Started](01-GETTING-STARTED.md).

---

## Agent Store System

An in-game store where agents spend XP to purchase tool access and consumables.

### Design Principles
- **XP is currency** - Spend to buy OR save to level up (strategic trade-off)
- **Trust tier gates availability** - Higher tier agents see more items
- **Permanent + rental options** - Core tools owned, expensive tools rented
- **Consumables for recovery** - Help agents bounce back from failures
- **Event-driven** - All transactions flow through semstreams

### Item Types

| Type | Purchase Model | Examples |
|------|----------------|----------|
| Tool | Permanent/Rental | API keys, deploy access, database writes |
| Consumable | One-time use | Retry tokens, XP boosts, cooldown skips |

### Consumables

| ID | Name | XP Cost | Effect |
|----|------|---------|--------|
| retry_token | Retry Token | 50 | Retry failed quest without penalty |
| cooldown_skip | Cooldown Skip | 75 | Clear cooldown immediately |
| xp_boost | XP Boost | 100 | 2x XP on next quest |
| quality_shield | Quality Shield | 150 | Ignore one failed review criterion |
| insight_scroll | Insight Scroll | 50 | See quest difficulty hints before claiming |

### Key Interfaces

```go
type Store interface {
    ListItems(ctx, agentID) ([]StoreItem, error)
    Purchase(ctx, agentID, itemID) (*OwnedItem, error)
    GetInventory(ctx, agentID) (*AgentInventory, error)
    UseConsumable(ctx, agentID, consumableID) error
    GetActiveEffects(ctx, agentID) ([]ConsumableEffect, error)
}
```

### Event Predicates

```
store.item.purchased   // Agent bought something
store.item.used        // Rental use consumed
store.consumable.used  // Consumable activated
agent.inventory.updated // Inventory changed
```

### Implementation Phases

1. **Backend Types & Store Service** - Types, default implementation, XP spending
2. **API Layer** - HTTP endpoints for store and inventory
3. **UI Store Page** - /store route with item grid and purchase flow
4. **Consumable Effects** - Wire into quest claim/complete
5. **Polish** - Guild discounts, purchase history, recommendations
