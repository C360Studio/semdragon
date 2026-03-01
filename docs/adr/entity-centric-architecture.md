# ADR: Entity-Centric Architecture (KV as Unified State + Event Store)

## Status
Accepted

## Context
Semdragons needs clarity on what constitutes an "entity" vs an "event". The confusion stems from NATS KV being a wrapper over streams, which gives us both state storage AND event semantics from the same infrastructure.

Previously, the system used dual emission: entity state updates to KV AND separate event payloads to subjects. This created triple redundancy where the same semantic fact was stored multiple ways.

## Decision

Adopt an entity-centric architecture where:
1. **Entities are the only first-class citizens** - everything tracked is an entity
2. **KV watches replace event subscriptions** - processors react to entity state changes
3. **Predicates act as event channels** - watch 3-part predicates to react to specific changes
4. **No separate event payloads** - triples capture all context

## The NATS KV "Twofer"

NATS KV buckets are backed by JetStream streams. This gives us unified state + event semantics:

```
┌─────────────────────────────────────────────────────────────┐
│              ENTITY STATE KV BUCKET                          │
│         (backed by JetStream under the hood)                │
│         Keys: 6-part entity IDs (org.plat.dom.sys.type.id)  │
├─────────────────────────────────────────────────────────────┤
│  Interface A: KV                                             │
│  - Get("c360.prod.game.board1.quest.abc") → current state   │
│  - Put("c360.prod.game.board1.quest.abc", state) → update   │
│  - Keys("c360.prod.game.board1.quest.*") → list quests      │
├─────────────────────────────────────────────────────────────┤
│  Interface B: Watch (event subscription)                     │
│  - Watch("c360.prod.game.board1.quest.*") → quest changes   │
│  - Each change has: key, value, revision, operation         │
│  - Processors react to entity state changes                 │
├─────────────────────────────────────────────────────────────┤
│  Interface C: Stream (history/replay)                        │
│  - Replay from revision N                                    │
│  - Reconstruct state at any point in time                   │
│  - Full audit trail built-in                                │
└─────────────────────────────────────────────────────────────┘
```

**Key insight**: We don't need separate event streams. The entity state bucket IS the event log.

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

**The predicate IS the event type.** No separate event definitions needed.

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

## Triples Capture Event Context

Graph triples have: `(subject, predicate, object, source, timestamp, confidence)`

When agent earns XP from quest.abc:
```
(agent.dragon, xp.total, 1500, quest.abc, T1, 1.0)
(agent.dragon, xp.base_award, 100, quest.abc, T1, 1.0)
(agent.dragon, xp.quality_bonus, 50, quest.abc, T1, 1.0)
(agent.dragon, xp.speed_bonus, 25, quest.abc, T1, 1.0)
```

The triples ARE the event - no separate payload needed. The `source` field captures WHY.

## Processor Pattern

```go
type QuestBoardProcessor struct {
    cfg          *domain.BoardConfig
    entityKV     nats.KeyValue       // Entity state bucket
    predicateKV  nats.KeyValue       // Predicate index bucket
    questCache   map[string]*Quest   // Last known state for diffing
    graph        *graph.Client
}

func (p *QuestBoardProcessor) Run(ctx context.Context) error {
    // Watch all quests for this board (6-part pattern)
    questPattern := p.cfg.TypePrefix("quest") + ".*"
    questWatcher, _ := p.entityKV.Watch(questPattern)

    // Watch claimed predicate specifically (3-part)
    claimedWatcher, _ := p.predicateKV.Watch("quest.status.claimed")

    for {
        select {
        case entry := <-questWatcher.Updates():
            p.handleQuestChange(ctx, entry)
        case entry := <-claimedWatcher.Updates():
            p.handleClaimEvent(ctx, entry)
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}

func (p *QuestBoardProcessor) handleQuestChange(ctx context.Context, entry nats.KeyValueEntry) {
    quest := decodeQuest(entry.Value())
    prev := p.questCache[entry.Key()]  // Compare with cached state
    p.questCache[entry.Key()] = quest  // Update cache

    // Detect transitions by diffing prev vs quest
    if prev != nil && prev.Status != quest.Status {
        // Status changed - react accordingly
    }
}
```

## Change Detection

Each processor caches last known state in memory. On watch update, diff against cache to detect what changed.

## Consequences

### Positive
- Single mental model: everything is an entity
- No duplicate storage or emission
- Built-in audit trail via KV stream backing
- Predicates serve as typed event channels

### Negative
- Processors must cache last-known state for diffing
- Predicate index watching requires understanding graph internals
- Migration effort from current dual-emission pattern

### Neutral
- Change detection shifts from event payloads to state diffing

## Migration

1. Remove event payload types from `domain/`
2. Remove event subject definitions
3. Update processors to watch KV instead of subscribing to subjects
4. Enrich entity triples to capture context previously in event payloads

## References

- See CLAUDE.md section "The NATS KV Twofer" for quick reference
- `domain/config.go` for entity ID builders
