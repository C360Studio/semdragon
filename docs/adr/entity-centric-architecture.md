# ADR: Entity-Centric Architecture (KV as Unified State + Event Store)

## Status
Accepted (revised)

## Context
Semdragons needs clarity on what constitutes an "entity" vs an "event". The confusion stems from NATS KV being a wrapper over streams, which gives us both state storage AND event semantics from the same infrastructure.

Previously, the system used dual emission: entity state updates to KV AND separate event payloads to subjects. This created redundancy where the same semantic fact was stored multiple ways.

## Decision

Adopt an entity-centric architecture where:
1. **Entities are the only first-class citizens** - everything tracked is an entity
2. **KV watches for facts** - processors react to entity state changes via KV Watch
3. **JetStream streams for requests** - work items and commands use stream consumers
4. **Predicates act as event channels** - watch 3-part predicates to react to specific changes
5. **Triples capture entity context** - entity state is the source of truth for facts

### The Streams vs KV Heuristic

Not everything is a KV watch. The sharp test from the semstreams Streams vs KV doc:

```
"Is this a fact about the world or a request to do something?"

  Fact about the world  ────────────► KV Watch (twofer)
  Request to do something ──────────► JetStream Stream
```

**The restart test reinforces this:** If replaying every message since the beginning of time would be correct and harmless, use KV watch. If it would be catastrophic (duplicate LLM calls, re-executed tools, double-sent approvals), use a JetStream stream.

### Classification of Semdragons Communications

| Communication | Primitive | Reason |
|--------------|-----------|--------|
| Quest state changed (posted, claimed, completed) | KV Watch | Fact; fan-out; fast; idempotent on replay |
| Agent state changed (XP, level, skills) | KV Watch | Fact; fan-out; fast; idempotent on replay |
| Party/Guild state changed | KV Watch | Fact; fan-out; fast |
| Battle state changed | KV Watch | Fact; fan-out; fast |
| Execute a quest (run LLM, tools) | JetStream Stream | Request; queue; expensive; has side effects |
| Seed a board (create agents, guilds) | JetStream Stream | Request; queue; side effects; replay = duplicates |
| Start/end DM session | JetStream Stream | Request; session lifecycle; replay = duplicate sessions |
| Approve an action (human-in-the-loop) | JetStream Stream | Request; once; replay = re-prompt humans |
| Form a party for quest | JetStream Stream | Request; queue; side effects |

## The NATS KV "Twofer"

NATS KV buckets are backed by JetStream streams. This gives us unified state + event semantics:

```
┌─────────────────────────────────────────────────────────────────┐
│              ENTITY STATE KV BUCKET                              │
│         (backed by JetStream under the hood)                    │
│         Keys: 6-part entity IDs (org.plat.dom.sys.type.id)      │
├─────────────────────────────────────────────────────────────────┤
│  Interface A: KV                                                 │
│  - Get("c360.prod.game.board1.quest.abc") → current state       │
│  - Put("c360.prod.game.board1.quest.abc", state) → update       │
│  - Keys("c360.prod.game.board1.quest.*") → list quests          │
├─────────────────────────────────────────────────────────────────┤
│  Interface B: Watch (event subscription)                         │
│  - Watch("c360.prod.game.board1.quest.*") → quest changes       │
│  - Each change has: key, value, revision, operation             │
│  - Processors react to entity state changes                     │
├─────────────────────────────────────────────────────────────────┤
│  Interface C: Stream (history/replay)                            │
│  - Replay from revision N                                        │
│  - Reconstruct state at any point in time                       │
│  - Full audit trail built-in                                    │
└─────────────────────────────────────────────────────────────────┘
```

**Key insight**: For entity state, we don't need separate event streams. The entity state bucket IS the event log.

## Three Subscription Patterns

| Pattern | Key Format | Use Case |
|---------|------------|----------|
| **Entity-level** | `c360.prod.game.board1.quest.abc123` | Track one specific entity |
| **Type-level** | `c360.prod.game.board1.quest.*` | Watch all entities of a type |
| **Predicate-level** | `quest.status.claimed` | React to specific state transitions |

### Predicates as Events

The predicate index (3-part keys) acts as event channels:

| Old Event Subject | New Predicate Watch |
|-------------------|---------------------|
| `quest.lifecycle.posted` | `quest.status.posted` |
| `quest.lifecycle.claimed` | `quest.status.claimed` |
| `quest.lifecycle.completed` | `quest.status.completed` |
| `agent.progression.levelup` | `agent.progression.level` |
| `party.formation.created` | `party.status.active` |

**The predicate IS the event type** for state-change facts.

## Entities

Every tracked concept is an entity with 6-part ID:

**Format**: `org.platform.domain.system.type.instance`

| Entity | Example Key | State Contains |
|--------|-------------|----------------|
| Quest | `c360.prod.game.board1.quest.abc123` | status, agent, result, timestamps |
| Agent | `c360.prod.game.board1.agent.dragon` | XP, level, skills, cooldowns |
| Party | `c360.prod.game.board1.party.epic01` | members, active quest |
| Guild | `c360.prod.game.board1.guild.coders` | members, ranks |
| Battle | `c360.prod.game.board1.battle.b789` | state, verdict |

## Triples Capture Entity Context

Graph triples have: `(subject, predicate, object, source, timestamp, confidence)`

When agent earns XP from quest.abc:
```
(agent.dragon, xp.total, 1500, quest.abc, T1, 1.0)
(agent.dragon, xp.base_award, 100, quest.abc, T1, 1.0)
(agent.dragon, xp.quality_bonus, 50, quest.abc, T1, 1.0)
(agent.dragon, xp.speed_bonus, 25, quest.abc, T1, 1.0)
```

The triples capture entity state including the `source` of WHY state changed. For entity state changes (facts), triples are the payload — no separate event payload is needed.

For work requests (JetStream streams), typed payloads remain appropriate since these carry instructions, not state.

## Processor Patterns

### State-Reactive Processor (KV Watch)

For processors that react to entity state changes (facts about the world):

```go
type AgentProgressionProcessor struct {
    cfg        *domain.BoardConfig
    entityKV   nats.KeyValue       // ENTITY_STATES bucket
    questCache map[string]*Quest   // Last known state for diffing
    graph      *GraphClient
}

func (p *AgentProgressionProcessor) Run(ctx context.Context) error {
    // Watch all quests for this board (type-level pattern)
    questPattern := p.cfg.TypePrefix("quest") + ".*"
    questWatcher, _ := p.entityKV.Watch(questPattern)

    for {
        select {
        case entry := <-questWatcher.Updates():
            p.handleQuestChange(ctx, entry)
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}

func (p *AgentProgressionProcessor) handleQuestChange(ctx context.Context, entry nats.KeyValueEntry) {
    quest := decodeQuest(entry.Value())
    prev := p.questCache[entry.Key()]
    p.questCache[entry.Key()] = quest

    // Detect transitions by diffing prev vs current
    if prev != nil && prev.Status != quest.Status {
        if quest.Status == QuestCompleted {
            p.awardXP(ctx, quest)
        }
    }
}
```

### Request-Driven Processor (JetStream Stream)

For processors that handle work requests (commands with side effects):

```go
type ExecutorProcessor struct {
    taskConsumer jetstream.ConsumeContext  // quest execution requests
    graph        *GraphClient              // entity state for reads/writes
}

func (p *ExecutorProcessor) Run(ctx context.Context) error {
    // Consume task messages from JetStream stream
    // DeliverPolicy: "new" — don't replay on restart
    p.taskConsumer, _ = consumer.Consume(func(msg jetstream.Msg) {
        task := decodeTask(msg.Data())
        p.executeQuest(ctx, task)
        msg.Ack()  // Explicit ack after completion
    })
}
```

### Mixed Processor

Some processors legitimately use both — KV watches for state they react to, JetStream for work they dispatch:

```go
type BoidEngineProcessor struct {
    // State watches (KV — facts)
    agentWatch jetstream.KeyWatcher  // React to agent state changes
    questWatch jetstream.KeyWatcher  // React to quest state changes

    // Derived state (in-memory cache for diffing)
    agents map[string]*Agent
    quests map[string]*Quest

    // Output (KV — publish derived facts)
    graph *GraphClient  // Write boid suggestions as entity state
}
```

## Change Detection

Each state-reactive processor caches last known state in memory. On watch update, diff against cache to detect what changed.

## Consequences

### Positive
- Single mental model: entities for facts, streams for requests
- No duplicate storage for entity state
- Built-in audit trail via KV stream backing
- Predicates serve as typed event channels for state facts
- JetStream gives exactly-once, ack, backpressure for work items

### Negative
- Processors must cache last-known state for diffing
- Developers must apply the streams-vs-KV heuristic correctly
- Migration effort from old patterns

### Neutral
- Change detection for facts shifts from event payloads to state diffing
- Work request payloads remain as typed structs (not a violation)

## Migration

### For state-reactive processors (KV Watch migration)
1. Replace NATS subject subscriptions with KV Watch on ENTITY_STATES
2. Add in-memory state cache for diffing
3. Use `ExtractInstance()` to parse 6-part entity ID keys
4. Remove dead event payload types that duplicated entity state

### For request-driven processors (already correct)
1. Keep JetStream stream consumers for work items
2. Keep typed request/response payloads (these are commands, not state)
3. Ensure entity state reads use GraphClient (direct KV), not graph-ingest request-reply

### Dead code cleanup
1. Remove event payload types that are defined but never published
2. Remove GraphClient methods that require graph-ingest but have no callers
3. Remove vocabulary registrations for dead payload types

## Current Compliance

| Processor | Pattern | Status |
|-----------|---------|--------|
| bossbattle | KV Watch + state cache | Fully compliant |
| agentprogression | KV Watch + state cache | Fully compliant |
| boidengine | KV Watch + state cache | Compliant (key extraction needs fix) |
| questboard | API-driven + KV emit | Compliant (request-reply API is intentional) |
| executor | JetStream consumer | Correct primitive (request) |
| seeding | JetStream consumer | Correct primitive (request) |
| dmsession | JetStream request-reply | Correct primitive (request) |
| dmapproval | JetStream request-reply | Correct primitive (request) |
| dmpartyformation | JetStream request-reply | Correct primitive (request) |
| dmworldstate | On-demand queries | Needs fix: uses graph-ingest request-reply |
| agentstore | NATS Subscribe on predicate | Needs fix: should KV Watch agent entity state |
| guildformation | NATS Subscribe on predicate | Needs fix: should KV Watch agent entity state |
| partycoord | NATS Subscribe on predicate | Mixed: state reactions need KV Watch, party commands are correct as streams |

## References

- See CLAUDE.md section "The NATS KV Twofer" for quick reference
- `domain/config.go` for entity ID builders
- semstreams `docs/concepts/streams-vs-kv.md` for the full streams-vs-KV heuristic
