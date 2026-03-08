# ADR-003: QuestDAGExec Single-Goroutine Event Loop Refactor

## Status: Accepted

## Context

The `questdagexec` component drives party quest DAG execution — the core feature enabling agents to decompose complex quests into sub-quests, assign them to party members, review outputs, and roll up results. After weeks of debugging, five structural issues were identified:

1. **Three concurrent goroutines** (`watchLoop`, `watchDAGBucket`, `watchReviewCompletions`) all mutate `DAGExecutionState` through per-DAG mutexes
2. **KV feedback loop**: writing to QUEST_DAGS triggers watch events that re-enter the handler
3. **Two-phase commit**: `ClaimQuestForParty` then `StartQuest` are separate calls with a crash-vulnerable gap
4. **Dual source of truth**: in-memory `dagCache` vs QUEST_DAGS KV bucket diverge under load
5. **Split DAG ownership**: questbridge creates DAGs, questdagexec drives them, both write to QUEST_DAGS

Every bug fixed (data races, feedback loops, missing StartQuest, CAS conflicts, thread-unsafe mocks) maps directly to a design cause, not an implementation oversight. A single-writer sequential coordinator eliminates all five root causes.

## Decision

Refactor questdagexec to use a **single-goroutine event loop** with three producer goroutines feeding a unified event channel.

### Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                    questdagexec Component                      │
│                                                                │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────┐   │
│  │ Quest Entity  │  │ QUEST_DAGS   │  │ AGENT Stream     │   │
│  │ KV Watcher    │  │ KV Watcher   │  │ Review Consumer  │   │
│  │ (producer)    │  │ (producer)   │  │ (producer)       │   │
│  └──────┬───────┘  └──────┬───────┘  └────────┬─────────┘   │
│         │                  │                    │              │
│         └──────────────────┼────────────────────┘              │
│                            │                                   │
│                    ┌───────▼───────┐                           │
│                    │  chan dagEvent │                           │
│                    └───────┬───────┘                           │
│                            │                                   │
│                    ┌───────▼───────┐                           │
│                    │  Event Loop   │ ← single goroutine        │
│                    │  (handler)    │   no mutexes needed        │
│                    └───────┬───────┘                           │
│                            │                                   │
│              ┌─────────────┼─────────────┐                    │
│              │             │             │                     │
│         dagCache    QUEST_DAGS KV   trajectories              │
│        (in-memory)   (persist)      (observability)           │
│                                                                │
└──────────────────────────────────────────────────────────────┘
```

### Four PRs (incremental, each independently testable)

**PR 1: Prompt fix** — Elevate party lead tool directive above quest context. Numbered rules for cross-model reliability. Remove duplicate from quest section.

**PR 2: Atomic ClaimAndStartForParty** — Single KV write transitions sub-quest from `posted` → `in_progress`. Eliminates two-phase commit gap. Bootstrap recovery for stuck `claimed` sub-quests.

**PR 3: DAGInitPayload handoff** — Questbridge writes minimal `DAGInitPayload` to QUEST_DAGS. Questdagexec builds full `DAGExecutionState` from it. Clear ownership boundary.

**PR 4: Single-goroutine event loop** — Three watchers become event producers. One goroutine processes all events sequentially. Delete mutexes, CAS retry, revision guards, merge logic.

### What gets deleted

- `dagMutexes sync.Map` and `dagMutex()` helper
- `reloadDAGRevision` and merge logic (`nodeStateRank`, `mergeNodeLists`)
- CAS retry loop in `persistDAGState`
- Revision comparison guard in `processDAGBucketEntry`
- `buildDAGExecutionState` in questbridge (moves to questdagexec)
- `partyLeadDecomposeInstruction` in assembler.go (moves to fragment registry)

### What stays

- `QUEST_DAGS` KV bucket (purpose-built for DAG state, clean SSE observability)
- `QuestDAG`, `QuestNode`, `DAGExecutionState`, `DAGReadyNodes`, `Validate` (types are solid)
- `AssignReadyNodes` in recruitment.go (with updated interface)
- Sub-quest posting in questbridge (natural unit of work with loop completion)

## Consequences

**Positive:**
- Eliminates all concurrency bugs by construction (single writer)
- Simpler testing (event handlers are synchronous functions)
- Better observability (see Observability section below)
- Clean ownership boundary (questbridge detects + posts, questdagexec drives)
- Crash recovery is straightforward (bootstrap reads QUEST_DAGS, replays into event loop)

**Negative:**
- Event loop is a throughput bottleneck if processing hundreds of concurrent DAGs (acceptable for current scale, can shard by execution ID later)
- Four PRs means ~1 week of incremental work
- Producer goroutines still need minimal testing for NATS adapter logic

**Risk mitigation:**
- Each PR is independently deployable and testable
- PR 1 (prompt fix) and PR 2 (atomic claim+start) provide immediate value without the full refactor
- Integration tests with `-race` validate the concurrency model at each stage

## Observability

DAG orchestration events are NOT LLM events. They must not be crammed into semstreams trajectories, which are LLM execution traces (prompts, tool calls, model responses). The three observability layers:

### 1. QUEST_DAGS KV bucket (primary — already exists)

Every `persistDAGState` call writes the full `DAGExecutionState` to the QUEST_DAGS bucket. With the single-goroutine event loop, every state transition produces exactly one KV write. Operators watch DAG progress in real-time via message-logger SSE:

```bash
# All DAG mutations
curl -N http://localhost:8080/message-logger/kv/QUEST_DAGS/watch?pattern=*

# Specific DAG execution
curl -N http://localhost:8080/message-logger/kv/QUEST_DAGS/watch?pattern=<execution-id>
```

Bucket history (10 revisions) provides replay of the full execution timeline per DAG.

### 2. eventType annotations on quest entity writes (secondary)

When the event loop transitions sub-quest status via `EmitEntityUpdate`, use DAG-specific eventType strings to distinguish DAG-driven transitions from normal quest transitions:

- `quest.dag.node_accepted` — sub-quest accepted by lead review
- `quest.dag.node_rejected` — sub-quest rejected, feedback injected
- `quest.dag.node_failed` — sub-quest exhausted retries

These appear in the entity state bucket and are visible via message-logger SSE on the board bucket.

### 3. Structured logging (operational)

Every log from the event loop includes `execution_id`, `parent_quest_id`, `party_id` at DAG level, plus `node_id` and `sub_quest_id` at node level.

### Trace correlation (data links, not trajectory nesting)

| Relationship | How recorded |
|---|---|
| Parent quest → DAG | `DAGExecutionState.ParentQuestID` in QUEST_DAGS |
| DAG → Sub-quests | `DAGExecutionState.NodeQuestIDs` map |
| Sub-quest → Parent | `quest.parent.quest` predicate on sub-quest entity |
| Sub-quest → LLM trace | `quest.execution.loop_id` predicate → `AGENT_TRAJECTORIES` |
| Review loop → Sub-quest | `TaskMessage.Metadata["sub_quest_id"]` |
| Review loop → LLM trace | `TaskMessage.LoopID` → `AGENT_TRAJECTORIES` |

Operators reconstruct the full picture by reading the QUEST_DAGS entry (lists all sub-quests and states), then following `loop_id` links to individual LLM trajectories in `AGENT_TRAJECTORIES`.

### What NOT to do

- Do not create a "DAG trajectory" — no LLM execution to trace
- Do not emit DAG events as separate NATS pub/sub messages — KV twofer already provides event semantics
- Do not add DAG progress predicates to the parent quest entity — DAG execution state belongs in QUEST_DAGS, not on the quest entity
