# Quest Lifecycle

Quests are the fundamental unit of work in semdragons. Every quest flows through a state
machine managed by the `questboard` processor, with quality gates enforced by `bossbattle`
and execution driven by `questbridge` + `questtools`.

## Contents

- [Creating Quests](#creating-quests)
- [Quest Spec Format](#quest-spec-format)
- [Quest Fields Reference](#quest-fields-reference)
- [Decomposability Classification](#decomposability-classification)
- [Difficulty and XP Table](#difficulty-and-xp-table)
- [Review Levels](#review-levels)
- [Quest Chains and Dependencies](#quest-chains-and-dependencies)
- [Lifecycle State Machine](#lifecycle-state-machine)
- [Boss Battle Evaluation](#boss-battle-evaluation)
- [Party Quests and DAG Decomposition](#party-quests-and-dag-decomposition)
- [Artifacts](#artifacts)
- [Scenarios as Acceptance Criteria](#scenarios-as-acceptance-criteria)
- [Further Reading](#further-reading)

---

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

The system classifies the quest from its scenario dependency graph. In this example,
two independent scenarios plus one dependent summary scenario produce a `mixed`
classification — a smaller party quest (see [Decomposability Classification](#decomposability-classification)).

### Via DM Chat

`POST /game/dm/chat` in `quest` mode accepts natural language and produces a `QuestBrief`
with goal, requirements, and structured scenarios. The DM prompt teaches scenario-dependency
thinking so the classification is derived from output, not declared separately.
See [ADR-001](adr/001-dm-chat-routing.md) for details.

### Via Quest Chains

Submit multiple interdependent quests as a batch. Dependencies between chain entries
use 0-based array indices referencing other entries in the same submission:

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

Chain validation: at least 1 quest, maximum 50; each entry needs a title and goal;
`depends_on` indices must be in `[0, len-1)`, no self-references, no duplicates, no
cycles (validated via topological sort).

---

## Quest Spec Format

Quest creation uses a structured spec (see [ADR-007](adr/007-scenario-driven-quest-specs.md)):

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

Scenario `depends_on` uses names, not indices. Names must be unique within the quest;
cycles and unknown name references are rejected at creation time.

---

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
| `parent_quest` | QuestID | `nil` | Set on sub-quests; links back to decomposing party quest |
| `party_id` | PartyID | `nil` | Set on sub-quests; hides them from the public board |

---

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

---

## Difficulty and XP Table

| Level | Name | Base XP | Min Tier | Agent Level Range |
|-------|------|---------|----------|-------------------|
| 0 | Trivial | 10 | Apprentice | 1-5 |
| 1 | Easy | 25 | Apprentice | 3-7 |
| 2 | Moderate | 50 | Journeyman | 6-10 |
| 3 | Hard | 100 | Expert | 10-14 |
| 4 | Epic | 200 | Master | 14-18 |
| 5 | Legendary | 500 | Grandmaster | 18-20 (or party) |

---

## Review Levels

| Level | Name | Judges | Criteria | When to Use |
|-------|------|--------|----------|-------------|
| 0 | Auto | 1 automated | Single "acceptance" check (threshold 0.5) | Low-stakes, trivial quests |
| 1 | Standard | 1 automated + 1 LLM | Correctness (0.5/0.7), Completeness (0.3/0.6), Quality (0.2/0.5) | Default for most work |
| 2 | Strict | 1 automated + 2 LLMs | Correctness (0.4/0.8), Completeness (0.3/0.8), Quality (0.2/0.7), Style (0.1/0.6) | Production-critical output |
| 3 | Human | 1 automated + 1 LLM + 1 human | Same as Strict weights but with human sign-off | Compliance, sensitive decisions |

Criteria column format: `weight/threshold`. A criterion passes when its score meets the
threshold. The weighted sum of criterion scores produces the overall quality score.

---

## Quest Chains and Dependencies

Quests can declare dependencies via `depends_on`, which holds QuestIDs of quests that must
reach `completed` status before this quest becomes available for claiming.

In a chain submission, `depends_on` uses 0-based array indices. `PostQuestChain` resolves
them to real QuestIDs in a two-pass process: first it posts all quests to get real IDs,
then it emits updates with resolved dependency references.

`AvailableQuests` filters out dependency-blocked quests: only quests whose `depends_on`
entries are all `completed` appear in results. Cycle detection uses Kahn's algorithm.

### Quest Data Flow

Quests carry structured input and output data via the `quest.data.input` and
`quest.data.output` predicates. These are stored as triples on the quest entity and
reconstructed by `QuestFromEntityState`.

- **Input** (`quest.data.input`) — data provided when the quest is created. Included in
  the agent's prompt context so the LLM has material to work with.
- **Output** (`quest.data.output`) — the agent's result, set when the quest is submitted
  or completed. Available for downstream quests in a chain.

In quest chains, a downstream quest can reference its dependency's output as input. The
`questbridge` processor reads the completed dependency's output and injects it into the
dependent quest's `TaskMessage` before dispatching to the agentic loop.

---

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
               ┌─────────┐ ┌────────────────┐
               │ posted   │ │ pending_triage  │
               │ (repost) │ └────────────────┘
               └─────────┘
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
| `in_progress` | `failed` / `posted` | `FailQuest` | Repost if attempts < max, else pending_triage |
| `in_review` | `completed` | `CompleteQuest` (victory) | Sets verdict, `completed_at`, duration |
| `in_review` | `failed` / `posted` | `FailQuest` (defeat) | Repost if attempts < max, else pending_triage |
| any active | `escalated` | `EscalateQuest` | Flags for DM attention, terminal state |

When a quest exhausts its retry budget (`attempts >= max_attempts`), it moves to
`pending_triage`. The DM triage processor routes it along one of four recovery paths:
salvage (preserve partial work and retry), TPK (clear and retry with anti-pattern
context), escalate (require human attention), or terminal (mark as impossible).

---

## Boss Battle Evaluation

When a quest transitions to `in_review`, the `bossbattle` processor detects the state
change via KV watch and automatically starts a battle:

1. **Detection** — KV watcher compares cached quest status against new status. On
   transition to `in_review`, checks `needs_review` and `review_level`.
2. **Battle creation** — `buildBattle` constructs criteria and judges based on review
   level.
3. **Agent status** — The claiming agent is set to `in_battle` status.
4. **Evaluation** — `BattleEvaluator.Evaluate()` runs all judges against the quest output.
   - *Automated judge* — heuristic scoring (output present = 0.8 base score)
   - *LLM judge* — LLM-as-judge evaluation against criteria
   - *Human judge* — returns `Pending` status, pausing until human submits verdict
5. **Persistence** — Criteria and results are stored as indexed triples on the battle
   entity. Criteria use predicates like `battle.criteria.{i}.name`,
   `battle.criteria.{i}.weight`, `battle.criteria.{i}.threshold`. Results use
   `battle.result.{i}.score`, `battle.result.{i}.passed`, `battle.result.{i}.reasoning`.
   The overall verdict is stored under `battle.verdict.*` predicates.
6. **Verdict** — Weighted scores produce a quality score. All criteria must meet their
   threshold for the verdict to pass.
7. **Quest transition** — Victory sets quest to `completed` with verdict attached.
   Defeat calls `FailQuest`, which either reposts or sends to `pending_triage`.

### Failure and Retry

When a quest fails, `questboard` checks `attempts < max_attempts`. If retryable, the
quest resets to `posted` and the agent returns to `idle`. If the budget is exhausted,
the quest moves to `pending_triage` for DM triage. Failure types: `quality` (boss
battle defeat), `timeout`, `error`, `abandoned`.

---

## Party Quests and DAG Decomposition

Party quests are quests where `party_required` is true. They require a party lead (Master
tier or above) who decomposes the quest into a DAG of sub-quests before any work begins.

### How Party Quest Execution Works

1. **Party formation** — The DM or boids engine assembles a party for the quest. The
   lead agent is the highest-level member with `CanDecomposeQuest` permission (Master+).

2. **Lead receives decompose tool** — When the parent quest transitions to `in_progress`,
   `questbridge` dispatches it to the lead's agentic loop with the `decompose_quest` tool
   available and set as required (`tool_choice: required`). The lead must call this tool
   before doing anything else.

3. **DAG proposal** — The lead returns a JSON DAG with up to 20 nodes. Each node has:

   | Field | Type | Description |
   |-------|------|-------------|
   | `id` | string | Unique node identifier within the DAG |
   | `objective` | string | What the sub-quest must accomplish |
   | `skills` | []string | Skills required to work on this node |
   | `difficulty` | int (0-5) | Challenge level for recruiting |
   | `acceptance` | []string | Criteria the lead will evaluate during review |
   | `depends_on` | []string | Node IDs that must complete before this one is ready |

4. **Validation and posting** — `questbridge` extracts the DAG from the lead's tool
   output, calls `questboard.PostSubQuests` to create sub-quest entities with `PartyID`
   set, then persists the DAG state as `quest.dag.*` predicates on the parent quest entity.

5. **Reactive execution** — `questdagexec` watches KV transitions on sub-quest entities.
   Nodes with no unmet dependencies start as `ready`; all others start `pending`.
   When `questdagexec` detects a `ready` node, it calls `ClaimAndStartForParty` to
   assign it to the best-matched party member.

6. **Lead review gate** — After a sub-quest completes its boss battle (or bypasses it),
   the node enters `pending_review`. DataDragon receives a `review_sub_quest` tool call
   with the member's output and the node's acceptance criteria. The lead rates on a 1-5
   scale across three questions. Average >= 3.0 accepts the node; below that rejects it
   with corrective feedback injected into the member's next dispatch prompt. The lead
   can also answer member clarification questions via `answer_clarification`.

7. **Rollup** — When all nodes reach `completed`, `questdagexec` sends the lead a
   `rollup_outputs` tool call containing all node outputs in topological order. The lead
   synthesises a final result, which is submitted as the parent quest's output.

8. **Parent quest completion** — `questdagexec` submits the rollup result to the parent
   quest and disbands the party. The parent quest proceeds to `in_review` if it has
   `require_review` set, or directly to `completed`.

### DAG Node State Machine

```
pending → ready → assigned → in_progress → pending_review → completed
                                                         └─→ rejected → assigned (retry)
                                                         └─→ failed (exhausted retries)
```

Clarification pauses the node at `awaiting_clarification` until the lead answers, then
returns to `assigned` for re-dispatch.

### DAG State Persistence

All DAG state is stored as `quest.dag.*` predicates on the parent quest entity in NATS KV
(no separate KV bucket): `quest.dag.execution_id`, `quest.dag.definition` (full JSON),
`quest.dag.node_quest_ids`, `quest.dag.node_states`, `quest.dag.node_assignees`,
`quest.dag.node_retries`. Clarification exchanges are stored on the sub-quest entity
under `quest.dag.clarifications` to keep the parent entity bounded.

---

## Artifacts

Agents produce files during quest execution. These files are preserved after the quest
completes so you can inspect the work.

Any file written by an agent to its sandbox directory during tool execution is an
artifact. Everything written to the sandbox is snapshotted on quest completion and
stored in the filestore under `quests/{quest_id}/`. Keys are flat paths relative to
that prefix (e.g. `quests/abc123/src/main.go`).

**Via the dashboard** — the `/workspace` page lists all quests with artifacts, shows a
file tree for each quest, and displays file contents inline.

**Via the REST API** — three endpoints:

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/game/workspace` | List quests that have artifacts, with file counts |
| `GET` | `/game/workspace/tree?quest={id}` | Nested file tree for a single quest |
| `GET` | `/game/workspace/file?quest={id}&path={path}` | Serve a single artifact file |

The `tree` endpoint returns a nested `workspaceEntry` structure (`name`, `path`, `is_dir`,
`size`, `children`). The `file` endpoint serves raw content with MIME type inferred from
the extension. Path traversal (`..`) is rejected.

---

## Scenarios as Acceptance Criteria

Scenarios replace the old flat `acceptance` string array. They serve the same two
purposes but with richer structure:

1. **Agent prompting** — The `promptmanager` injects scenarios into the quest context.
   For solo (sequential/trivial) quests, scenarios appear as ordered acceptance
   criteria. For party quests, the party lead receives scenarios as structured
   decomposition material instead of having to invent a breakdown from prose.
2. **Boss battle evaluation** — Scenario names and descriptions feed the LLM judge's
   evaluation prompt, providing concrete standards beyond generic review criteria.

A minimal quest with no scenarios still works — the `goal` and `requirements` fields
serve as the acceptance signal. Scenarios are optional but strongly recommended for
any non-trivial quest because they give the system enough structure to make the
party vs solo routing decision accurately.

Example (parallel → party quest, two independent scenarios):

```json
{
  "title": "Build user notification service",
  "goal": "Users receive real-time notifications across email and in-app channels",
  "scenarios": [
    {"name": "In-app delivery",          "skills": ["code_generation"]},
    {"name": "Email with preference check", "skills": ["code_generation", "data_transformation"]}
  ]
}
```

Example (sequential → solo, each scenario depends on the previous):

```json
{
  "title": "Migrate legacy auth to OAuth2",
  "goal": "Replace custom token auth with OAuth2 PKCE flow without breaking sessions",
  "scenarios": [
    {"name": "OAuth2 provider integration"},
    {"name": "Session migration bridge",   "depends_on": ["OAuth2 provider integration"]},
    {"name": "Cutover and rollback",       "depends_on": ["Session migration bridge"]}
  ],
  "difficulty": 4
}
```

---

## Further Reading

- [01-GETTING-STARTED.md](01-GETTING-STARTED.md) — Setup, walkthrough, debugging
- [02-DESIGN.md](02-DESIGN.md) — Architecture, concept map, agentic execution path
- [04-PARTIES.md](04-PARTIES.md) — Party formation and peer reviews
- [05-BOIDS.md](05-BOIDS.md) — Emergent quest-claiming behavior
- [07-MODEL-REGISTRY.md](07-MODEL-REGISTRY.md) — Capability routing including `quest-execution-sequential`
- [adr/007-scenario-driven-quest-specs.md](adr/007-scenario-driven-quest-specs.md) — Design rationale and empirical basis
- [Swagger UI](/docs) — Live API documentation at `/docs`
