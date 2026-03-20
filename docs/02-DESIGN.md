# Semdragons: Design Document

## Contents

- [The Elevator Pitch](#the-elevator-pitch)
- [Concept Map](#concept-map)
- [Architecture Layers](#architecture-layers)
- [Agentic Execution Path](#agentic-execution-path)
- [Trust Tiers in Detail](#trust-tiers-in-detail)
- [Graph Gateway](#graph-gateway)
- [Artifact Lifecycle](#artifact-lifecycle)
- [Example Flow](#example-flow-analyze-q3-sales-data-and-send-summary-to-stakeholders)
- [Death Mechanics](#death-mechanics)
- [Guild Red-Team Review](#guild-red-team-review)
- [DM Attention System](#dm-attention-system)
- [Design Decisions](#design-decisions)
- [Agent Store System](#agent-store-system)

---

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

## Agentic Execution Path

The primary execution path for quest work runs through three components:

```
Quest transitions to in_progress
        │
        ▼
   questbridge
   - Watches KV for in_progress transitions
   - Assembles TaskMessage (prompt, tools, metadata)
   - Publishes to AGENT JetStream stream
   - Persists quest-to-loop mapping in QUEST_LOOPS KV
        │
        ▼
  semstreams agentic-loop + agentic-model
   - Runs the LLM turn loop
   - Emits tool.execute.* messages for each tool call
   - Emits loop completion/failure events
        │
        ▼
   questtools
   - Consumes tool.execute.* from AGENT stream
   - Enforces tier/skill/sandbox gates via ToolRegistry
   - Publishes tool.result.* responses back to AGENT stream
        │
        ▼
   questbridge (completion handler)
   - Receives loop done/failed event
   - Transitions quest to completed or failed
   - Emits quest.lifecycle.completed / quest.lifecycle.failed
```

This design is reactive and event-driven throughout. `questbridge` and `questtools` are
the semdragons integration layer over semstreams' generic agentic-loop. They translate
quest entities and tool registrations into the message format the loop expects, then
translate results back into quest state changes.

**The `executor` component is opt-in / legacy.** It provides synchronous LLM execution
without the agentic-loop and was the original implementation. It is not enabled in the
default config. New deployments should use questbridge + questtools.

### Context Assembly

Before publishing the `TaskMessage`, `questbridge` assembles the agent's context window:

1. **System prompt** — domain-specific fragments selected from the prompt catalog via
   `promptmanager`. Fragments are selected by tier and skill tag.
2. **Entity knowledge** — structured text about the agent's identity (level, XP, skills,
   party membership), the quest (goal, requirements, scenarios), and any injected peer
   review feedback. Appended after the system prompt.
3. **Quest input** — the `quest.data.input` triple value, if set, gives the agent
   material to work with.
4. **Dependency output** — for quests in a chain, the completed dependency's
   `quest.data.output` is injected as context.

Context metadata (token count, fragment IDs, entity IDs) is stored on the quest entity
under `quest.context.*` predicates and is visible in trajectory traces.

---

## Trust Tiers in Detail

Trust tiers are derived from agent level. The mapping is defined in `domain.TierFromLevel`.

```
Level  1-5   │ APPRENTICE   │ Read-only, summarize, classify, simple transforms
             │              │ Max quest difficulty: Trivial
             │              │ No external side effects
             │              │
Level  6-10  │ JOURNEYMAN   │ Can call tools, make API requests, write to staging
             │              │ Max quest difficulty: Moderate
             │              │ Side effects in sandboxed environments
             │              │
Level 11-15  │ EXPERT       │ Can modify production state, spend money, deploy
             │              │ Max quest difficulty: Hard
             │              │ Requires boss battle on production-critical output
             │              │
Level 16-18  │ MASTER       │ Can lead parties, decompose quests, supervise agents
             │              │ Max quest difficulty: Epic
             │              │ Can create sub-quests and review other agents
             │              │
Level 19-20  │ GRANDMASTER  │ Can act as DM delegate, create quests, manage guilds
             │              │ Max quest difficulty: Legendary
             │              │ Trusted to make unsupervised decisions
```

Key permission boundaries from `domain.TierPermissionsFor`:

| Tier         | Lead Party | Decompose Quest | Supervise | Act as DM |
|--------------|-----------|-----------------|-----------|-----------|
| Apprentice   | no        | no              | no        | no        |
| Journeyman   | no        | no              | no        | no        |
| Expert       | no        | no              | no        | no        |
| Master       | yes       | yes             | yes       | no        |
| Grandmaster  | yes       | yes             | yes       | yes       |

---

## Graph Gateway

The `graph-gateway` component (semstreams v1.0.0-alpha.22+) exposes a GraphQL endpoint
for querying entity state. It enables temporal queries, NLQ classification, similarity
search, and relationship traversal — capabilities that go beyond what the KV watch
pattern can provide cheaply.

The gateway is not wired in the default config yet. When enabled, it is proxied through
the SvelteKit BFF layer (`+page.server.ts` load functions) for dashboard pages that
need historical or relational queries. The dashboard's `worldStore` covers the common
case (current state of all entities); the gateway covers the rest.

---

## Artifact Lifecycle

Agents execute inside a sandboxed directory managed by `questtools`. Any files written
during tool execution are snapshotted on quest completion and stored in the artifact
filestore under the path `quests/{quest_id}/`.

The artifact lifecycle has four stages:

1. **Sandbox** — agent writes files during execution via tool calls. The sandbox is a
   temporary directory scoped to the quest's agentic loop.
2. **Snapshot** — on quest completion, `questtools` or the agentic-loop completion
   handler copies the sandbox contents into the filestore.
3. **Filestore** — artifacts are stored under `quests/{quest_id}/` in the configured
   storage backend (local filesystem or object store).
4. **Browse** — the REST API and UI workspace page expose the artifacts for inspection.

### Artifact API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/game/workspace` | List quests that have artifacts, with file counts |
| `GET` | `/game/workspace/tree?quest={id}` | Nested file tree for a quest's artifacts |
| `GET` | `/game/workspace/file?quest={id}&path={path}` | Serve a single artifact file |

The workspace page in the dashboard at `/workspace` wraps these three endpoints. File
paths are validated against path traversal (`..`) before serving.

---

## Example Flow: "Analyze Q3 Sales Data and Send Summary to Stakeholders"

### 1. DM Creates Quest

The DM posts a structured quest spec. Scenario dependencies signal that the first two
scenarios are independent (data pull and analysis can run in parallel) while the summary
and email delivery form a sequential tail — producing a `mixed` classification and a
smaller party:

```go
quest := NewQuest("Analyze Q3 sales and email stakeholders").
    Goal("Stakeholders receive a data-backed Q3 summary via email").
    Requirements("Include year-over-year comparison", "Cite all figures").
    Scenarios(
        QuestScenario{Name: "Extract Q3 data",    Skills: []string{"data_transformation"}},
        QuestScenario{Name: "Analyze trends",      Skills: []string{"analysis"}},
        QuestScenario{Name: "Write summary",       Skills: []string{"summarization"},
            DependsOn: []string{"Extract Q3 data", "Analyze trends"}},
        QuestScenario{Name: "Email to VP list",    Skills: []string{"customer_comms"},
            DependsOn: []string{"Write summary"}},
    ).
    Difficulty(DifficultyEpic).
    XP(500).
    BonusXP(200).
    MaxDuration(30 * time.Minute).
    ReviewAs(ReviewStrict).
    Build()

board.PostQuest(ctx, quest)
// Decomposability: mixed → party_required set automatically
```

### 2. Boids Engine Suggests Party

The Boids engine identifies idle agents with matching skills:

- **DataDragon** (Level 14, Expert, Guild: Data Wranglers) — high affinity for data quests
- **SummaryScribe** (Level 12, Expert, Guild: Analysts) — strong analysis + writing
- **MailHawk** (Level 8, Journeyman, Skills: customer_communications) — can send emails

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
Sub-quest 4: "Email summary to VP list"        (Easy, customer_comms)          — depends on 3
```

`questdagexec` validates the DAG (cycle check, max 20 nodes) and persists it as
`quest.dag.*` predicates on the parent quest entity in the graph.

### 5. Sub-Quests Execute

`questdagexec` watches KV transitions. Nodes 2-4 start `pending`; node 1 is immediately
`ready`. `questdagexec` calls `ClaimAndStartForParty` to route node 1 to DataDragon —
agents do not claim party sub-quests via the public board.

When node 1 completes, node 2 becomes `ready` and is routed to SummaryScribe. Each
completed node unlocks its dependents reactively. Sub-quest 4 (MailHawk) waits until
the summary output from node 3 is available.

Each sub-quest has its own mini boss battle (ReviewAuto or ReviewStandard). After the
boss battle, the node transitions to `pending_review` and DataDragon receives a
`review_sub_quest` tool call. Acceptance (average rating >= 3.0) moves the node to
`completed`; rejection injects corrective feedback into the member's retry prompt.

### 6. Party Lead Rolls Up Results

Once all four nodes reach `completed`, `questdagexec` sends DataDragon a
`rollup_outputs` tool call containing all node outputs in topological order. DataDragon
synthesises the final package and returns it as the rollup payload. `questdagexec`
submits this as the parent quest output and disbands the party.

### 7. Boss Battle (Strict Level)

Multi-judge review panel:

- **Automated judge** — checks data accuracy, email formatting, recipient list
- **LLM judge** — evaluates summary quality, insight depth, tone appropriateness
- **LLM judge 2** — cross-checks analysis against raw data for hallucinations

Criteria and results are persisted as indexed triples on the battle entity
(`battle.criteria.{i}.name`, `battle.result.{i}.score`, etc.) for full observability
and trajectory replay.

### 8. XP Distribution

```
DataDragon:    500 base + 180 quality + 50 speed + 75 guild = 805 XP (LEVEL UP → 15!)
SummaryScribe: 350 base + 140 quality + 0 speed + 52 guild  = 542 XP
MailHawk:       50 base +  40 quality + 20 speed + 0 guild   = 110 XP
```

### 9. Events Stream (via semstreams)

```
quest.lifecycle.posted     → {quest_id: "q-123", difficulty: "epic"}
party.formation.formed     → {party_id: "p-456", lead: "DataDragon", members: [...]}
quest.lifecycle.claimed    → {quest_id: "q-123-sub-1", agent: "DataDragon"}
quest.lifecycle.started    → ...
quest.lifecycle.completed  → {quest_id: "q-123-sub-1", quality: 0.92}
battle.review.started      → {battle_id: "b-789", level: "strict", judges: 3}
battle.review.victory      → {battle_id: "b-789", quality: 0.89}
agent.progression.levelup  → {agent: "DataDragon", old: 14, new: 15, tier: "expert"}
quest.lifecycle.completed  → {quest_id: "q-123", quality: 0.89}
party.formation.disbanded  → {party_id: "p-456"}
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

## Guild Red-Team Review

When a quest is submitted for review, the `redteam` processor posts an **adversarial
review quest** before the boss battle evaluates the result. A different agent — ideally
from a different guild — claims the red-team quest and attempts to find flaws, edge cases,
or weaknesses in the original agent's output.

The flow:

1. Quest submitted (`in_review`) → `redteam` posts a review quest with the original
   output as context.
2. The boid engine applies a **1.5x cross-guild affinity** multiplier, attracting agents
   from guilds other than the original agent's. This ensures adversarial diversity.
3. The red-team agent produces findings (weaknesses, missed requirements, edge cases).
4. Findings are attached to the original quest before the boss battle judge evaluates it.
   The judge sees both the agent's output and the red-team critique.
5. After the boss battle, the `redteam` processor extracts **lessons** from the findings
   and persists them to the reviewing guild's knowledge base, indexed by skill tag and
   lesson category.

**Non-blocking with timeouts:** If no agent claims the red-team quest within the claim
timeout, or if execution exceeds the execution timeout, the processor emits a `skipped`
signal and the boss battle proceeds without red-team input. The system never blocks on
red-team availability.

**Configuration:** Both the `redteam` component and `bossbattle.red_team_enabled: true`
must be set. The `min_difficulty` config (default: 1, i.e., Easy) controls which quests
trigger red-team review — trivial quests skip it.

---

## DM Attention System

The dashboard surfaces quest failures and escalations to the Dungeon Master through two
UI channels: **toasts** (transient notifications) and **chat attention cards** (persistent
action items in the DM chat panel).

### Notification tiers

| Quest transition | Toast | Chat card | Status badge |
|-----------------|-------|-----------|--------------|
| Agent fails, retries remain (`in_progress → posted`) | "Re-queued" (blue) | No | No |
| Retries exhausted (`→ pending_triage`) | "Needs Triage" (orange) | No | No |
| Triage escalates / clarification (`→ escalated`) | "Escalation" (magenta) | Yes | Yes |
| Terminal failure (`→ failed`) | "Failed" (red) | No | No |

The design principle: **only interrupt the DM for decisions that require human judgment.**
Auto-retries and auto-triage handle recoverable failures silently (toasts provide
awareness). The chat attention card — which auto-opens the chat panel and requires explicit
action — appears only when the system has exhausted all automated recovery paths.

### Triage flow

When retries are exhausted, the quest enters `pending_triage`. The auto-triage system
(configurable via `dmMode`: `full_auto`, `assisted`, `supervised`, `manual`) evaluates
the failure and selects a recovery path:

- **Salvage** — preserve partial output, grant one more attempt
- **TPK** — clear output, inject anti-patterns as warnings, grant one more attempt
- **Escalate** — route to DM via chat attention card
- **Terminal** — permanently fail the quest

### Attention card actions

Escalation cards in the DM chat offer three actions:

- **Repost** — re-queue the quest for a new agent
- **Repost with Guidance** — re-queue with DM-provided instructions injected into context
- **Abandon** — permanently close the quest

The status bar shows a pulsing badge with the count of unresolved escalations. Cards
auto-resolve when the quest leaves the `escalated` state.

---

## Design Decisions

The following questions arose during design and are now settled:

- **Guild formation**: Both auto-suggest and DM-created. The `guildformation` processor
  performs automatic clustering based on demonstrated skills and co-performance. The DM
  can also form guilds manually via the API.

- **Inter-guild quests**: The party system handles cross-guild collaboration. Parties can
  draw members from multiple guilds; no separate inter-guild mechanism is needed.

- **Agent memory**: Guild library for persistent cross-quest knowledge; party context for
  quest-scoped memory. Agents are persistent KV entities and retain state across sessions.

- **Boids vs explicit assignment**: Boids is the default. DM modes (`full_auto`,
  `supervised`, `manual`) control how much the DM overrides suggestions. In `supervised`
  and `manual` modes the DM can intercept any boid suggestion before it becomes a claim.

- **Multi-session learning**: Yes. Agents are persistent entities stored in NATS KV;
  sessions are execution contexts, not agent lifetimes. Levels and XP survive restarts.

- **Quest chains**: Supported via `depends_on` with dependency validation. The parent
  quest remains open across sessions until all dependencies resolve.

- **Competitive dynamics**: The Boids engine handles competition implicitly via attraction
  scores. Guild reputation further differentiates agents on shared quest pools, enabling
  A/B-style competitive dynamics without explicit PvP mechanics.

For implementation details see [Getting Started](01-GETTING-STARTED.md).

---

## Agent Store System

An in-game store where agents spend XP to purchase tool access and consumables.

### Design Principles

- **XP is currency** — Spend to buy OR save to level up (strategic trade-off)
- **Trust tier gates availability** — Higher tier agents see more items
- **Permanent + rental options** — Core tools owned, expensive tools rented
- **Consumables for recovery** — Help agents bounce back from failures
- **Event-driven** — All transactions flow through semstreams

### Item Types

| Type | Purchase Model | Examples |
|------|----------------|----------|
| Tool | Permanent/Rental | API keys, deploy access, database writes |
| Consumable | One-time use | Retry tokens, XP boosts, cooldown skips |

### Consumables

| ID | Name | XP Cost | Effect |
|----|------|---------|--------|
| `retry_token` | Retry Token | 50 | Retry failed quest without penalty |
| `cooldown_skip` | Cooldown Skip | 75 | Clear cooldown immediately |
| `xp_boost` | XP Boost | 100 | 2x XP on next quest |
| `quality_shield` | Quality Shield | 150 | Ignore one failed review criterion |
| `insight_scroll` | Insight Scroll | 50 | See quest difficulty hints before claiming |

### Event Predicates

```
store.item.purchased     // Agent bought something
store.item.used          // Rental use consumed
store.consumable.used    // Consumable activated
agent.inventory.updated  // Inventory changed
```
