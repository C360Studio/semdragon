# DAG Execution: Lessons Learned

The `questdagexec` refactor exposed structural patterns that recur across the codebase.
This document captures those lessons so that the next processor is built right the first
time, and existing processors can be improved incrementally.

Read [ADR-003](adr/003-questdagexec-refactor.md) for the specific context that drove
most of these observations.

---

## 1. The KV Twofer Principle

NATS KV buckets are backed by JetStream streams. **The entity state bucket IS the event
log.** A `WatchKV` subscriber receives both current state and a replay of every mutation —
you get point-in-time reads *and* event subscription from one bucket, with one connection,
under one retention policy.

When you create a separate KV bucket for a specific concern, you opt out of this guarantee.
You now have two buckets with independent lifecycles, separate CAS revision spaces, and a
new class of consistency bugs whenever they drift apart.

### QUEST_DAGS: the completed migration

The `QUEST_DAGS` bucket stored full `DAGExecutionState` objects alongside the entity state
bucket that held the parent quest. This meant:

- A separate watcher goroutine for `QUEST_DAGS` in addition to the quest entity watcher
- Its own bootstrap protocol, CAS retry logic, and revision merge code
- A KV write feedback loop: questdagexec wrote to `QUEST_DAGS`, its own watcher fired,
  which called `processDAGBucketEntry` again — an infinite loop requiring a revision
  comparison guard

The refactor moves minimal init payload to `QUEST_DAGS` (written once by questbridge on
DAG creation) and keeps the authoritative running state there, separate from the parent
quest entity. The in-memory `dagCache` is a hot-path projection — the bucket is the source
of truth.

**Key takeaway**: the revision comparison guard, the merge logic (`nodeStateRank`,
`mergeNodeLists`), and the CAS retry loop in `persistDAGState` existed *entirely* because
two goroutines were contending on the same bucket. The single-goroutine event loop
(section 2) eliminates that contention, making the guards unnecessary.

### QUEST_LOOPS: next candidate

`questbridge` maintains a `QUEST_LOOPS` KV bucket mapping quest IDs to agentic loop IDs.
Every field in `QuestLoopMapping` (loop ID, agent ID, trust tier, started-at) already
exists on the quest entity or can be added as a predicate.

| `QuestLoopMapping` field | Equivalent predicate |
|--------------------------|----------------------|
| `LoopID` | `quest.execution.loop_id` |
| `AgentID` | `quest.agent.assigned` (already exists) |
| `TrustTier` | readable from agent entity |
| `StartedAt` | `quest.lifecycle.started` timestamp |

Adding `quest.execution.loop_id` eliminates the bucket, its watcher goroutine, and the
class of consistency bugs where the loops bucket and entity state disagree after a crash.
The bootstrap path collapses to the existing quest entity watcher — the same one
questbridge already runs.

### BOID_SUGGESTIONS: optional removal

`boidengine` optionally persists suggestion payloads to a KV bucket with a 5-minute TTL.
Agents consume suggestions through NATS pub/sub; the KV bucket exists purely for
debugging visibility. The message-logger SSE endpoint already captures the same pub/sub
traffic in real time. The bucket adds no operational value and can be removed without
changing any agent behavior.

### When a separate bucket is justified

Only create a purpose-built bucket when:

1. The data has a fundamentally different lifecycle than any entity — for example,
   `AGENT_TRAJECTORIES` is managed entirely by the semstreams agentic-loop, not by
   semdragons code.
2. The data is consumed by infrastructure outside your control (semstreams internals,
   external tooling).
3. The data needs independent TTL or retention from entity state.

If none of these apply, store state as predicates on the relevant entity and let the
entity watcher drive your handler.

---

## 2. Single Event Loop Over Multiple Goroutines and Mutexes

### questdagexec: the reference pattern

```
┌────────────────────────────────────────────┐
│              questdagexec                  │
│                                            │
│  ┌─────────────┐  ┌─────────────────────┐  │
│  │ Quest KV    │  │ AGENT Stream review │  │
│  │ watcher     │  │ consumer            │  │
│  │ (producer)  │  │ (producer)          │  │
│  └──────┬──────┘  └──────────┬──────────┘  │
│         │                    │             │
│         └────────┬───────────┘             │
│                  │                         │
│          ┌───────▼────────┐                │
│          │ chan dagEvent   │                │
│          └───────┬────────┘                │
│                  │                         │
│          ┌───────▼────────┐                │
│          │  runEventLoop  │ ← one goroutine │
│          │  (no mutexes)  │                │
│          └────────────────┘                │
└────────────────────────────────────────────┘
```

Producer goroutines only read from NATS and write to the event channel — they hold no
shared state. The event loop owns `dagCache`, `dagBySubQuest`, and `questCache` as plain
maps. Because only one goroutine touches them, no mutexes are needed. Metric atomics are
always safe from any goroutine.

The package-level comment states this explicitly so reviewers know it is deliberate:

```go
// Architecture: two producer goroutines feed a unified chan dagEvent.
// A single event loop goroutine processes events sequentially, eliminating
// data races by construction. dagCache, dagBySubQuest, and questCache are
// plain maps — no mutexes needed because only the event loop accesses them.
```

### questbridge: the anti-pattern

`questbridge` runs three goroutines that share state through `sync.Map`:

- `watchLoop` — KV watcher, writes to `questCache` and `activeLoops`
- `consumeCompletions` — JetStream consumer, reads/writes `activeLoops` and `escalatedAt`
- `sweepStaleEscalations` — periodic sweep, reads `escalatedAt`

`sync.Map` provides per-key atomicity but prevents batch reads. Reading a snapshot of
`activeLoops` for the sweep requires either a full `Range` traversal or an additional
mutex. Any operation that must read two maps atomically (e.g., "is quest X in
activeLoops *and* not in escalatedAt?") has a TOCTOU window.

The fix is the same pattern questdagexec uses: define a typed event union, feed it
through a channel, process in a single goroutine.

### boidengine: the anti-pattern

`boidengine` uses three `RWMutex`-protected maps (`agentsMu`, `questsMu`, `guildsMu`)
and a fourth `mu` protecting configuration state. Every computation tick acquires all
three read locks, iterates all cached data, and releases. As agent and quest counts grow,
lock contention grows with them.

An event-loop design caches only what changed since the last update, processes deltas,
and computes scores on-demand — no read locks required.

### The guideline

If your component has more than two goroutines touching shared state, reach for the
event-loop-with-channel pattern. The overhead of sending an event through a channel is
negligible. The overhead of debugging a data race or a sync.Map TOCTOU window is not.

```go
// Typed event union for your component.
type myEvent struct {
    kind string // "entity_update" | "review_completed" | "tick"
    // ... payload fields
}

// Start launches producers and the event loop.
func (c *Component) Start(ctx context.Context) error {
    events := make(chan myEvent, 64)
    go c.produceKVEvents(ctx, events)
    go c.produceStreamEvents(ctx, events)
    go c.runEventLoop(ctx, events)
    return nil
}

// runEventLoop is the sole owner of all mutable component state.
func (c *Component) runEventLoop(ctx context.Context, events <-chan myEvent) {
    cache := make(map[string]*myEntity) // no mutex needed
    for {
        select {
        case <-ctx.Done():
            return
        case ev := <-events:
            c.handle(ctx, ev, cache)
        }
    }
}
```

---

## 3. Narrow Interfaces Over Direct Component Pointers

### questdagexec: the reference pattern

`questdagexec` declares only the methods it actually calls:

```go
// QuestBoardRef is the narrow interface questdagexec needs from questboard.
type QuestBoardRef interface {
    SubmitResult(ctx context.Context, questID domain.QuestID, result any) error
    FailQuest(ctx context.Context, questID domain.QuestID, reason string) error
    EscalateQuest(ctx context.Context, questID domain.QuestID, reason string) error
    ClaimAndStartForParty(
        ctx context.Context,
        questID domain.QuestID,
        partyID domain.PartyID,
        assignedTo domain.AgentID,
    ) error
    RepostForRetry(ctx context.Context, questID domain.QuestID) error
}
```

Benefits:

- Tests implement the interface with a minimal mock struct — no real component needed
- Adding a method to `questboard.Component` does not recompile `questdagexec`
- The interface is self-documenting: a reader knows exactly what cross-component calls
  occur without reading the dependency's source

### autonomy: the anti-pattern

```go
// processor/autonomy/component.go
store    *agentstore.Component   // concrete type
guilds   *guildformation.Component
approval *dmapproval.Component
```

Direct pointers to concrete types mean:

- Tests must construct real `agentstore`, `guildformation`, and `dmapproval` components
  or use fragile injection via `Set*` methods
- Any change to `agentstore.Component` (new field, changed constructor) recompiles
  `autonomy`
- The full surface area of each component is visible inside `autonomy`, making it easy
  to accidentally call methods that should be out-of-scope

The fix is the same as `questdagexec`: define a narrow interface per dependency, set it
via a typed `Set*` method that accepts the interface.

---

## 4. Party Lead Prompt Engineering

### The decompose_quest directive must be unconditional

If the lead prompt allows any prose response before calling `decompose_quest`, some
models will emit a planning narrative and never call the tool. The directive must be the
first numbered rule in the system prompt and must forbid text-only responses:

```
1. Your FIRST action MUST be to call decompose_quest.
   Do not write a plan. Do not summarize the quest. Call the tool immediately.
```

Provider-specific reinforcement helps with models that emit preambles:

| Provider | Behavior | Mitigation |
|----------|----------|-----------|
| Anthropic | Follows directive reliably | No extra hint needed |
| OpenAI | Follows with minor drift | Restate rule in user turn |
| Gemini Flash | Reliable with explicit rule | Add "Call it NOW." emphasis |
| Gemini Pro | Reliable but slow | Use low reasoning effort |

### Sub-quest executor directive

Without an explicit completion signal, sub-quest agents iterate through tool calls until
the loop timeout. The `[INTENT: work_product]` marker in the task message tells the
agent to produce a deliverable and stop:

```
[INTENT: work_product]
Produce the requested artifact and return it as your final message.
Do NOT explore endlessly. Simple tasks should be answered immediately.
```

### Lead synthesis, not delegation

The party lead reviews every sub-quest output — it has full context. Routing the
"combine results" step to a separate sub-quest agent fails because:

1. The combining agent cannot directly access peer outputs (the lead must inject them)
2. The lead must review the combination anyway, creating a circular dependency
3. It burns retry budget on an avoidable step

After all nodes reach `completed`, `questdagexec` dispatches a `rollup_outputs` tool
call to the lead's agentic loop with all outputs in topological order. The lead
synthesises directly.

### Clarification flow

Party leads can ask clarification questions rather than calling `decompose_quest`.
The routing rules are:

| Source | Routes to | Reason |
|--------|-----------|--------|
| Parent quest clarification | DM | Lead lacks the context to answer its own question |
| Sub-quest clarification | Party lead | Lead has full DAG context |

Auto-DM (`full_auto` mode) answers clarifications via LLM and resumes the quest. Guard
the auto-answer path: only respond when output contains `[INTENT: clarification]` —
DAG node exhaustion escalations are terminal and must not be auto-answered.

Set a maximum clarification round count (default 3) to prevent infinite loops when the
model cannot make progress.

---

## 5. Model Tiering for Token Efficiency

### The E2E Gemini pattern

`reasoning_effort` is per-endpoint configuration, not per-request. Create separate
endpoint entries for different effort levels rather than mixing effort on one endpoint.

| Tier | Endpoint | Model | Reasoning | Rationale |
|------|----------|-------|-----------|-----------|
| Apprentice | `gemini-lite` | flash-lite | default | Read-only, classification |
| Journeyman | `gemini-flash` | flash | low | Standard agent execution |
| Expert | `gemini-flash` → `gemini-pro` | flash / pro | low | Complex tasks, fallback |
| Master / Grandmaster | `gemini-pro-low` | pro | low | Decomposition, synthesis |
| Boss Battle | `gemini-pro-low` | pro | low | Review evaluation |
| DM clarification | `gemini-pro` | pro | medium | Context-aware answering |

### Key insights

- Party leads need pro-quality responses for decomposition, but decomposition is
  *structured* output (JSON DAG), not deep reasoning — use low effort.
- DM clarification answering needs medium reasoning — the model must understand
  accumulated quest context and give a meaningful answer.
- Worker agents (journeyman) use flash — fast and cheap for well-scoped tasks.
- Gemini Flash does not call `decompose_quest` reliably when the prompt is ambiguous —
  the directive (section 4) matters more than model choice.

### Passing reasoning_effort through

The `dm_answerer` capability (and any capability that overrides effort) must pass
`reasoning_effort` from the endpoint config through to the API request body. Forgetting
this means all DM clarifications run at the provider default, ignoring your config.

---

## 6. CAS Safety for Shared Entities

When multiple processors write to the same entity, use `EmitEntityCAS` with a bounded
retry loop:

```go
const maxCASRetries = 3

func (c *Component) updateQuestDAGState(
    ctx context.Context,
    questID domain.QuestID,
    update func(*semdragons.Quest),
) error {
    for attempt := range maxCASRetries {
        entityState, revision, err := c.graph.GetQuestWithRevision(ctx, questID)
        if err != nil {
            return fmt.Errorf("read quest: %w", err)
        }
        quest := semdragons.QuestFromEntityState(entityState)
        update(quest)
        err = c.graph.EmitEntityCAS(ctx, quest, "quest.dag.updated", revision)
        if errors.Is(err, natsclient.ErrKVRevisionMismatch) {
            if attempt == maxCASRetries-1 {
                return fmt.Errorf("CAS conflict after %d attempts", maxCASRetries)
            }
            continue // retry with fresh read
        }
        return err
    }
    return nil
}
```

No locks needed — the NATS KV revision is the serialisation primitive. The pattern is
identical to `questboard.ClaimQuest`. Apply it whenever two processors legitimately race
on the same entity (e.g., questboard writes status, questdagexec writes DAG fields).

---

## 7. State Caching Guidelines

### Acceptable caching

| Pattern | Example | Why it is fine |
|---------|---------|----------------|
| Transition detection | `questCache` in questbridge/questdagexec | Last-known status only — not authoritative |
| Hot-path projection | `dagCache` in questdagexec | Avoids KV round-trips; graph is source of truth |
| Score projection | Boid attraction scores | Recomputed each tick from fresh data |

The graph is always the source of truth. In-memory caches are conveniences.

### Over-caching anti-patterns

**Full entity copies with RWMutex locks** (boidengine):

`boidengine` caches complete `Agent`, `Quest`, and `Guild` entities in three separate
`RWMutex`-guarded maps. At scale, every entity update acquires a write lock, every
compute tick acquires three read locks, and GC pressure grows with the entity population.
Cache scores and metadata, not full entities.

**Four sync.Maps for derived state** (agentstore):

`agentstore` maintains `agentXPCache`, `catalog`, `inventories`, and `activeEffects` as
four `sync.Map` projections built from KV state. Consider whether direct graph reads
suffice for the access patterns that drive these projections, and whether the event-loop
pattern would let you replace `sync.Map` with plain maps.

**Per-entity mutex in a sync.Map** (agentprogression):

Storing per-agent `sync.Mutex` values inside a `sync.Map` combines two forms of
concurrency overhead. The event-loop pattern serialises access without any locks.

### The guideline

If you are caching full entity state, you are building a second database. The graph
already *is* the database. Cache the minimum needed for decision-making — usually the
last-known status and one or two derived values.

---

## 8. Cleanup Candidates

### Immediate (low risk, isolated changes)

- [ ] Remove the unused `QuestLoopsBucket` field from `questtools` config — it is
  cargo-culted from an earlier design and has no effect
- [ ] Remove optional `BOID_SUGGESTIONS` KV persistence from `boidengine` — the
  message-logger SSE already captures pub/sub traffic at no extra cost
- [ ] Replace `autonomy`'s concrete component pointer fields with narrow interfaces
  (mirrors the questdagexec pattern)

### Medium term (targeted refactors)

- [ ] Eliminate `QUEST_LOOPS` bucket — store loop ID as `quest.execution.loop_id`
  predicate on the quest entity; remove the separate watcher goroutine from questbridge
- [ ] Consolidate questbridge into the event-loop pattern — single goroutine plus typed
  event channel replaces three goroutines and three `sync.Map`s
- [ ] Reduce boidengine cache scope — cache attraction scores and last-computed metadata
  per agent-quest pair, not full entity copies

### Investigate

- [ ] Whether `dmworldstate` can be replaced by direct `GraphClient` queries in the API
  handlers — its world-state snapshot may be redundant given live KV reads
- [ ] Whether agentstore's four `sync.Map` projections could be collapsed into an
  event-loop design, trading projection writes for on-demand reads from the graph
- [ ] Unified component dependency injection via semstreams registry — replace the
  current `Set*` method convention with a standard wiring pattern that works before
  `StartAll()` without caller coordination

---

## Further Reading

- [ADR-003](adr/003-questdagexec-refactor.md) — Single-goroutine event loop design and
  the five root causes it eliminates
- [ADR-004](adr/004-party-clarification-loop.md) — Clarification routing and auto-DM
  guard conditions
- [04-PARTIES.md](04-PARTIES.md) — DAG node state machine and lead review gate
- [07-MODEL-REGISTRY.md](07-MODEL-REGISTRY.md) — Endpoint configuration and
  reasoning effort settings
