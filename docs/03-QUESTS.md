# Quest Lifecycle

Quests are the fundamental unit of work in semdragons. Every quest flows through a state
machine managed by the `questboard` processor, with quality gates enforced by `bossbattle`.

## Creating Quests

### Via the REST API

```bash
curl -s -X POST http://localhost:8080/api/game/quests \
  -H "Content-Type: application/json" \
  -d '{
    "objective": "Analyze Q3 revenue trends",
    "difficulty": 3,
    "skills": ["analysis", "data_transformation"],
    "acceptance": ["Include year-over-year comparison", "Produce chart-ready data"],
    "review_level": 2
  }' | jq .
```

### Via DM Chat (in development)

The `POST /api/game/dm/chat` endpoint accepts natural language and produces a `QuestBrief`
with structured fields including `Acceptance` criteria and `DependsOn` references.

### Via Quest Chains

Submit multiple interdependent quests as a batch. Dependencies use 0-based array indices
referencing other entries in the same chain:

```bash
curl -s -X POST http://localhost:8080/api/game/quests/chain \
  -H "Content-Type: application/json" \
  -d '{
    "quests": [
      {"title": "Extract raw data", "skills": ["data_transformation"]},
      {"title": "Clean and normalize", "skills": ["data_transformation"], "depends_on": [0]},
      {"title": "Analyze trends", "skills": ["analysis"], "depends_on": [1]},
      {"title": "Write summary", "skills": ["summarization"], "depends_on": [2]}
    ]
  }' | jq .
```

Validation rules for chains:
- At least 1 quest, maximum 50
- Each entry needs a title
- `depends_on` indices must be in range `[0, len-1)` and cannot self-reference
- No duplicate dependencies per entry
- No dependency cycles (validated via topological sort)

## Quest Fields Reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `title` | string | required | Short description of the work |
| `description` | string | `""` | Detailed instructions |
| `difficulty` | int (0-5) | `0` (Trivial) | Challenge level (see table below) |
| `required_skills` | []SkillTag | `[]` | Skills needed to complete the quest |
| `required_tools` | []string | `[]` | Tool IDs the agent must have |
| `min_tier` | TrustTier | from difficulty | Minimum trust tier to claim |
| `party_required` | bool | `false` | Requires a party (too complex for solo) |
| `min_party_size` | int | `0` | Minimum party members |
| `base_xp` | int64 | from difficulty | XP awarded on completion |
| `constraints.require_review` | bool | `false` | Send to boss battle on submission |
| `constraints.review_level` | int (0-3) | `0` (Auto) | Review rigor (see table below) |
| `constraints.max_duration` | duration | `0` (none) | Time limit for execution |
| `constraints.max_tokens` | int | `0` (none) | Token budget for LLM calls |
| `constraints.max_cost` | float64 | `0` (none) | Cost budget |
| `allowed_tools` | []string | `[]` (all) | Tool whitelist (empty = all allowed) |
| `guild_priority` | GuildID | `nil` | Guild gets first claim opportunity |
| `acceptance` | []string | `[]` | Domain-flexible acceptance criteria |
| `depends_on` | []QuestID | `[]` | Must complete before this quest is claimable |
| `max_attempts` | int | config default | Retry limit before permanent failure |

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

## Acceptance Criteria

The `acceptance` field holds domain-flexible strings that describe what "done" looks like.
These are not validated against a schema; they serve two purposes:

1. **Agent prompting**: The `promptmanager` includes acceptance criteria in the quest
   context fragment so the LLM knows what to aim for.
2. **Boss battle evaluation**: Acceptance criteria feed into the evaluation prompt for
   LLM judges, providing concrete standards beyond the generic review criteria.

Examples:
```json
["Include year-over-year comparison", "All numbers sourced with citations"]
["Tests pass with >80% coverage", "No lint warnings"]
["Response under 500 words", "Executive-friendly tone"]
```

## Further Reading

- [01-GETTING-STARTED.md](01-GETTING-STARTED.md) — Setup, walkthrough, debugging
- [04-PARTIES.md](04-PARTIES.md) — Party formation and peer reviews
- [05-BOIDS.md](05-BOIDS.md) — Emergent quest-claiming behavior
- [Swagger UI](/docs) — Live API documentation at `/docs`
- [02-DESIGN.md](02-DESIGN.md) — Architecture and concept map
