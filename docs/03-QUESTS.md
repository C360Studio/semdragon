# Quest Lifecycle

Quests are the fundamental unit of work in semdragons. Every quest flows through a state
machine managed by the `questboard` processor, with quality gates enforced by `bossbattle`.

## Creating Quests

### Via the REST API

Quest creation uses a structured spec (`QuestBrief`) with a required `goal` and optional
`requirements` and `scenarios`. See [Quest Spec Format](#quest-spec-format) below for
the full structure.

```bash
curl -s -X POST http://localhost:8080/game/quests \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Analyze Q3 revenue trends",
    "goal": "Identify revenue anomalies and produce a summary for stakeholders",
    "requirements": ["Include year-over-year comparison", "Produce chart-ready data"],
    "scenarios": [
      {
        "name": "YoY comparison table",
        "description": "Query Q3 data for current and prior year, produce structured comparison",
        "skills": ["data_transformation"]
      },
      {
        "name": "Anomaly detection",
        "description": "Flag line items deviating more than 15% from the prior year trend",
        "skills": ["analysis"]
      },
      {
        "name": "Executive summary",
        "description": "Write a 300-word summary suitable for VP audience citing key numbers",
        "skills": ["summarization"],
        "depends_on": ["YoY comparison table", "Anomaly detection"]
      }
    ],
    "difficulty": 3,
    "skills": ["analysis", "data_transformation", "summarization"]
  }' | jq .
```

The system classifies the quest automatically from the scenario dependency graph
(see [Decomposability Classification](#decomposability-classification)). In this example,
the two independent scenarios and one dependent summary scenario produce a `mixed`
classification — a smaller party quest.

### Via DM Chat

The `POST /game/dm/chat` endpoint in `quest` mode accepts natural language and produces
a `QuestBrief` with goal, requirements, and structured scenarios. The DM prompt teaches
scenario-dependency thinking so the classification is derived from the output, not
declared separately. See [ADR-001](adr/001-dm-chat-routing.md) for chat mode details.

### Via Quest Chains

Submit multiple interdependent quests as a batch. Each entry is a full quest spec
with `goal`, `requirements`, and `scenarios`. Dependencies between chain entries use
0-based array indices referencing other entries in the same chain:

```bash
curl -s -X POST http://localhost:8080/game/quests/chain \
  -H "Content-Type: application/json" \
  -d '{
    "quests": [
      {
        "title": "Extract raw data",
        "goal": "Pull and normalise Q3 source records",
        "skills": ["data_transformation"]
      },
      {
        "title": "Analyze trends",
        "goal": "Identify year-over-year anomalies from the extracted data",
        "skills": ["analysis"],
        "depends_on": [0]
      },
      {
        "title": "Write summary",
        "goal": "Produce an executive-ready summary from the analysis",
        "skills": ["summarization"],
        "depends_on": [1]
      }
    ]
  }' | jq .
```

Validation rules for chains:
- At least 1 quest, maximum 50
- Each entry needs a title and a goal
- `depends_on` indices must be in range `[0, len-1)` and cannot self-reference
- No duplicate dependencies per entry
- No dependency cycles (validated via topological sort)

## Quest Spec Format

Quest creation replaced the old flat `description`/`acceptance` fields with a structured
spec (see [ADR-007](adr/007-scenario-driven-quest-specs.md)):

### `QuestBrief` Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `title` | string | yes | Short name for the work |
| `goal` | string | yes | The desired outcome — the "why" this work matters |
| `requirements` | []string | no | Concrete constraints that must be satisfied |
| `scenarios` | []QuestScenario | no | Testable outcomes that prove requirements are met |
| `difficulty` | int (0-5) | no | Challenge level; default `0` (Trivial) |
| `skills` | []SkillTag | no | Aggregate skills across all scenarios |
| `hints` | QuestHints | no | Manual overrides (e.g. `party_required`) |
| `depends_on` | []QuestID | no | Must complete before this quest is claimable |

### `QuestScenario` Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Short identifier (used in `depends_on` references) |
| `description` | string | yes | What completing this scenario looks like |
| `skills` | []string | no | Skills specifically needed for this scenario |
| `depends_on` | []string | no | Names of other scenarios that must complete first |

Scenario `depends_on` uses names, not indices. Names must be unique within a quest.
Cycles and references to non-existent scenario names are rejected at creation time.

## Quest Fields Reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `title` | string | required | Short description of the work |
| `goal` | string | required | The desired outcome |
| `requirements` | []string | `[]` | Concrete constraints the output must satisfy |
| `scenarios` | []QuestScenario | `[]` | Testable outcomes for the agent or party |
| `difficulty` | int (0-5) | `0` (Trivial) | Challenge level (see table below) |
| `required_skills` | []SkillTag | `[]` | Skills needed to complete the quest |
| `required_tools` | []string | `[]` | Tool IDs the agent must have |
| `min_tier` | TrustTier | from difficulty | Minimum trust tier to claim |
| `party_required` | bool | from classification | Requires a party (set by decomposability heuristic) |
| `min_party_size` | int | `0` | Minimum party members |
| `base_xp` | int64 | from difficulty | XP awarded on completion |
| `constraints.require_review` | bool | `false` | Send to boss battle on submission |
| `constraints.review_level` | int (0-3) | `0` (Auto) | Review rigor (see table below) |
| `constraints.max_duration` | duration | `0` (none) | Time limit for execution |
| `constraints.max_tokens` | int | `0` (none) | Token budget for LLM calls |
| `constraints.max_cost` | float64 | `0` (none) | Cost budget |
| `allowed_tools` | []string | `[]` (all) | Tool whitelist (empty = all allowed) |
| `guild_priority` | GuildID | `nil` | Guild gets first claim opportunity |
| `depends_on` | []QuestID | `[]` | Must complete before this quest is claimable |
| `max_attempts` | int | config default | Retry limit before permanent failure |

## Decomposability Classification

After the quest spec is submitted, the system classifies it deterministically from the
scenario dependency graph — no LLM call required. This drives the party vs solo routing
decision (see [ADR-007](adr/007-scenario-driven-quest-specs.md) for the empirical basis).

| Class | Condition | Staffing |
|-------|-----------|----------|
| `parallel` | No scenario has `depends_on` | Party quest — independent scenarios run concurrently |
| `sequential` | All scenarios form a single dependency chain | Solo quest routed to `quest-execution-sequential` |
| `mixed` | Some scenarios are independent, some depend on others | Smaller party quest |
| `trivial` | Zero or one scenario | Solo quest |

The classification is stored as `quest.routing.class` on the quest entity and is visible
in the graph and trajectory. `party_required` is set automatically from the heuristic.

**Manual override:** Set `hints.party_required: true` in the quest spec to force a party
quest regardless of classification. The system honours it with a warning log. The
heuristic is a default, not a gate.

Sequential quests are routed to the `quest-execution-sequential` capability in the model
registry, which can be configured to prefer higher-capability models. Research shows
disproportionate returns from model intelligence on sequential reasoning tasks — a
stronger model beats adding more agents. See
[07-MODEL-REGISTRY.md](07-MODEL-REGISTRY.md#capabilities) for configuration details.

## Difficulty and XP Table

| Level | Name | Base XP | Min Tier | Agent Level Range |
|-------|------|---------|----------|-------------------|
| 0 | Trivial | 10 | Apprentice | 1-5 |
| 1 | Easy | 25 | Apprentice | 3-7 |
| 2 | Moderate | 50 | Journeyman | 6-10 |
| 3 | Hard | 100 | Expert | 10-14 |
| 4 | Epic | 200 | Master | 14-18 |
| 5 | Legendary | 500 | Grandmaster | 18-20 (or party) |

## Review Levels

| Level | Name | Judges | Criteria | When to Use |
|-------|------|--------|----------|-------------|
| 0 | Auto | 1 automated | Single "acceptance" check (threshold 0.5) | Low-stakes, trivial quests |
| 1 | Standard | 1 automated + 1 LLM | Correctness (0.5/0.7), Completeness (0.3/0.6), Quality (0.2/0.5) | Default for most work |
| 2 | Strict | 1 automated + 2 LLMs | Correctness (0.4/0.8), Completeness (0.3/0.8), Quality (0.2/0.7), Style (0.1/0.6) | Production-critical output |
| 3 | Human | 1 automated + 1 LLM + 1 human | Same as Strict weights but with human sign-off | Compliance, sensitive decisions |

The "Criteria" column shows `weight/threshold`. A criterion passes when its score meets
the threshold. The weighted sum of scores produces the overall quality score.

## Quest Chains and Dependencies

Quests can declare dependencies via `depends_on`, which holds QuestIDs of quests that must
reach `completed` status before this quest becomes available for claiming.

In a chain submission, `depends_on` uses 0-based array indices that the `PostQuestChain`
handler resolves to real QuestIDs in a two-pass process:

1. **First pass**: Post all quests without dependencies (generates real IDs)
2. **Second pass**: Resolve index references to real QuestIDs and emit updates

When an agent queries `AvailableQuests`, dependency-blocked quests are filtered out: each
`depends_on` entry is checked against the entity map and only quests whose dependencies are
all `completed` appear in the results.

Cycle detection uses Kahn's algorithm (topological sort) during validation. If the sorted
count doesn't match the chain size, a cycle exists and the chain is rejected.

### Quest Data Flow

Quests carry structured input and output data via the `quest.data.input` and
`quest.data.output` predicates. These are stored as triples on the quest entity and
reconstructed by `QuestFromEntityState`.

- **Input** (`quest.data.input`): Data provided when the quest is created. Included in the
  agent's prompt context so the LLM has material to work with.
- **Output** (`quest.data.output`): The agent's result, set when the quest is submitted or
  completed. Available for downstream quests in a chain.

In quest chains, a downstream quest can reference its dependency's output as input. The
`questbridge` processor reads the completed dependency's output and injects it into the
dependent quest's `TaskMessage` before dispatching to the agentic loop.

## Lifecycle State Machine

```
                    ┌──────────────────────────────┐
                    v                              |
               ┌─────────┐    claim           ┌────────┐
   POST ──────>│ posted   │──────────────────>│ claimed │
               └─────────┘                    └────────┘
                    ^                              |
                    |  abandon / fail+retry    start|
                    |  (repost)                     v
                    |                         ┌────────────┐
                    └─────────────────────────│in_progress  │
                                              └────────────┘
                                                   |
                                              submit|
                                                   v
                              ┌──────────┬─────────────────────┐
                              | review?  |                     |
                              v yes      v no                  |
                         ┌──────────┐  ┌───────────┐           |
                         │in_review │  │ completed  │           |
                         └──────────┘  └───────────┘           |
                              |                                |
                     battle   |                           escalate
                     verdict  |                                |
                    ┌────┴────┐                                v
                    v         v                          ┌───────────┐
              ┌───────────┐ ┌────────┐                   │ escalated │
              │ completed │ │ failed │                   └───────────┘
              └───────────┘ └────────┘
                              |
                   attempts < max?
                    ┌────┴────┐
                    v yes     v no
               ┌─────────┐ ┌────────┐
               │ posted   │ │ failed │
               │ (repost) │ │(final) │
               └─────────┘ └────────┘
```

### Valid Transitions

| From | To | Trigger | Handler |
|------|----|---------|---------|
| (new) | `posted` | `PostQuest` | Generates entity ID, sets defaults, emits to KV |
| `posted` | `claimed` | `ClaimQuest` | Validates tier/skills, sets agent, increments attempts |
| `claimed` | `in_progress` | `StartQuest` | Sets `started_at` timestamp |
| `claimed` | `posted` | `AbandonQuest` | Resets agent to idle, clears assignment |
| `in_progress` | `in_review` | `SubmitResult` (review required) | Sets output, status to `in_review` |
| `in_progress` | `completed` | `SubmitResult` (no review) | Sets output, `completed_at`, duration |
| `in_progress` | `posted` | `AbandonQuest` | Resets agent to idle, clears assignment |
| `in_progress` | `failed` / `posted` | `FailQuest` | Repost if attempts < max, else terminal fail |
| `in_review` | `completed` | `CompleteQuest` (victory) | Sets verdict, `completed_at`, duration |
| `in_review` | `failed` / `posted` | `FailQuest` (defeat) | Repost if attempts < max, else terminal fail |
| any (except completed/cancelled/escalated) | `escalated` | `EscalateQuest` | Flags for DM attention, terminal state |

## Boss Battle Evaluation

When a quest transitions to `in_review`, the `bossbattle` processor detects the state
change via KV watch and automatically starts a battle:

1. **Detection**: KV watcher compares cached quest status against new status. On transition
   to `in_review`, checks `needs_review` and `review_level`.
2. **Battle creation**: `buildBattle` constructs criteria and judges based on review level.
3. **Agent status**: The claiming agent is set to `in_battle` status.
4. **Evaluation**: `BattleEvaluator.Evaluate()` runs all judges against the quest output.
   - **Automated judge**: Heuristic scoring (output present = 0.8 base score)
   - **LLM judge**: LLM-as-judge evaluation against criteria (TODO: full implementation)
   - **Human judge**: Returns `Pending` status, pausing until human submits verdict
5. **Verdict**: Weighted scores produce a quality score. All criteria must meet their
   threshold for the verdict to pass.
6. **Quest transition**: Victory sets quest to `completed` with verdict. Defeat sets quest
   to `failed` with failure reason from feedback.

## Failure and Retry

When a quest fails (boss battle defeat or explicit `FailQuest` call):

1. Check `attempts < max_attempts`
2. **If retryable**: Reset to `posted` status, clear agent assignment, clear output.
   The quest goes back on the board for any eligible agent to claim.
   The agent is set back to `idle` with no current quest.
3. **If exhausted**: Set to terminal `failed` status. The agent progression processor
   handles XP penalties.

Failure types:
- `quality` — Boss battle defeat (output didn't meet criteria)
- `timeout` — Exceeded time limit
- `error` — Unexpected execution error
- `abandoned` — Agent gave up

## Party Sub-Quests

Sub-quests created by the `questdagexec` processor during DAG execution have distinct
visibility and claiming rules that differ from quests posted directly to the board.

### Visibility: Hidden from the Public Board

Any quest with a non-empty `PartyID` field is excluded from the results returned by
`AvailableQuests`. This prevents regular agents from seeing — and accidentally claiming
— work that belongs to a party's internal DAG. The filter happens in the questboard
handler before results are returned; the quests still exist in KV and are visible to
party members querying by quest ID directly.

This is intentional. Party sub-quests are not "available" work in the pull-based sense.
They are directed assignments made by the lead through the DAG decomposition.

### Dependency Gates on Sub-Quests

Party sub-quests carry `DependsOn` entries referencing other nodes in the same DAG.
A sub-quest with unmet dependencies cannot be claimed — `ClaimQuest` will return an
error if any entry in `DependsOn` has not yet reached `completed` status. This is the
same gate that governs public quest chains (see [Quest Chains and
Dependencies](#quest-chains-and-dependencies)), applied consistently here.

`questdagexec` handles dependency tracking reactively: when a node completes, it
resolves which downstream nodes now have all dependencies satisfied and transitions them
from `pending` to `ready`. The processor then issues `ClaimQuestForParty` on those
nodes — agents do not need to poll.

### `ClaimQuestForParty`: Lead-Directed Claiming

Standard `ClaimQuest` is pull-based: the agent decides what to claim. For party
sub-quests, `questdagexec` uses `ClaimQuestForParty` instead, which:

1. Bypasses the public availability filter (the quest has a `PartyID`).
2. Verifies the target agent is a current member of the owning party.
3. Validates the agent's tier and skills against the sub-quest requirements — the lead
   cannot route work to an agent who lacks the capability.
4. Sets the sub-quest's `AgentID` and transitions status to `claimed` atomically via
   CAS to prevent race conditions when multiple nodes become ready simultaneously.

This is why DAG execution is reactive rather than push-based: `questdagexec` watches KV
transitions and calls `ClaimQuestForParty` when conditions are met, rather than the
lead manually dispatching each assignment.

For the full DAG node state machine and review gate, see
[04-PARTIES.md — DAG Execution Lifecycle](04-PARTIES.md#dag-execution-lifecycle).

## Scenarios as Acceptance Criteria

Scenarios replace the old flat `acceptance` string array. They serve the same two
purposes but with richer structure:

1. **Agent prompting**: The `promptmanager` injects scenarios into the quest context.
   For solo (sequential/trivial) quests, scenarios appear as ordered acceptance
   criteria. For party quests, the party lead receives scenarios as structured
   decomposition material instead of having to invent a breakdown from prose.
2. **Boss battle evaluation**: Scenario names and descriptions feed the LLM judge's
   evaluation prompt, providing concrete standards beyond generic review criteria.

A minimal quest with no scenarios still works — the `goal` and `requirements` fields
serve as the acceptance signal. Scenarios are optional but strongly recommended for
any non-trivial quest because they give the system enough structure to make the
party vs solo routing decision accurately.

Example (parallel → party quest):

```json
{
  "title": "Build user notification service",
  "goal": "Users receive real-time notifications across email and in-app channels",
  "requirements": ["Delivery within 5 seconds", "Per-event preferences"],
  "scenarios": [
    {
      "name": "In-app delivery",
      "description": "Event fires, notification appears in feed within 5s",
      "skills": ["code_generation"]
    },
    {
      "name": "Email with preference check",
      "description": "System checks prefs, sends email only if enabled for that event type",
      "skills": ["code_generation", "data_transformation"]
    }
  ]
}
```

Example (sequential → solo, high-tier model):

```json
{
  "title": "Migrate legacy auth to OAuth2",
  "goal": "Replace custom token auth with OAuth2 PKCE flow without breaking sessions",
  "scenarios": [
    {
      "name": "OAuth2 provider integration",
      "description": "Configure provider, implement PKCE flow, verify token exchange"
    },
    {
      "name": "Session migration bridge",
      "description": "Adapter validates both old tokens and OAuth2 tokens during transition",
      "depends_on": ["OAuth2 provider integration"]
    },
    {
      "name": "Cutover and rollback",
      "description": "Switch to OAuth2-only, verify rollback restores old auth",
      "depends_on": ["Session migration bridge"]
    }
  ],
  "difficulty": 4
}
```

## Further Reading

- [01-GETTING-STARTED.md](01-GETTING-STARTED.md) — Setup, walkthrough, debugging
- [04-PARTIES.md](04-PARTIES.md) — Party formation and peer reviews
- [05-BOIDS.md](05-BOIDS.md) — Emergent quest-claiming behavior
- [07-MODEL-REGISTRY.md](07-MODEL-REGISTRY.md) — Capability routing including `quest-execution-sequential`
- [adr/007-scenario-driven-quest-specs.md](adr/007-scenario-driven-quest-specs.md) — Design rationale and empirical basis
- [Swagger UI](/docs) — Live API documentation at `/docs`
- [02-DESIGN.md](02-DESIGN.md) — Architecture and concept map
