# QuestBoard Implementation Plan (Revised)

## Overview

Implement `QuestBoard` interface backed by semstreams/NATS using:
- **Vocabulary predicates** (3-part dotted notation) for event subjects
- **Entity IDs** (6-part dotted notation) for all game entities
- **KV keys** (dotted, wildcard-friendly) for state and indices
- **Reactive patterns** - emit events, handlers react independently

## Semstreams Integration Patterns

### Entity IDs (6-part)

All semdragons entities use the semstreams entity ID structure:

```
org.platform.domain.system.type.instance
```

**Mapping for semdragons:**

| Part | Value | Notes |
|------|-------|-------|
| org | configurable | Organization namespace (e.g., "c360") |
| platform | configurable | Deployment (e.g., "prod", "dev") |
| domain | "game" | Always "game" for semdragons |
| system | board name | Quest board instance (e.g., "board1") |
| type | entity type | quest, agent, party, guild, battle |
| instance | unique id | Random hex or meaningful name |

**Examples:**
```go
c360.prod.game.board1.quest.abc123
c360.prod.game.board1.agent.datadragon
c360.prod.game.board1.party.epic001
c360.prod.game.board1.guild.datawranglers
c360.prod.game.board1.battle.b789
```

**Wildcard queries:**
```go
c360.prod.game.board1.quest.*      // All quests on this board
c360.prod.game.board1.agent.>      // All agent entities and sub-keys
c360.prod.game.*.quest.*           // All quests across all boards
```

### Predicates (3-part)

Event subjects use vocabulary predicates:

```go
// Quest lifecycle
quest.lifecycle.posted
quest.lifecycle.claimed
quest.lifecycle.started
quest.lifecycle.submitted
quest.lifecycle.completed
quest.lifecycle.failed
quest.lifecycle.escalated
quest.lifecycle.abandoned

// Boss battles
battle.review.started
battle.review.verdict

// Agent progression (emitted by XP engine, not quest board)
agent.progression.xp
agent.progression.levelup
```

### KV Key Structure

Single KV bucket with hierarchical dotted keys:

**Bucket:** `semdragons.{org}.{platform}.{board}`

**State keys:**
```go
quest.{instance}                   // Full quest state JSON
agent.{instance}                   // Full agent state JSON
party.{instance}                   // Full party state JSON
guild.{instance}                   // Full guild state JSON
battle.{instance}                  // Full battle state JSON
```

**Index keys (presence = membership):**
```go
idx.quest.status.posted.{instance}     // Posted quests
idx.quest.status.claimed.{instance}    // Claimed quests
idx.quest.status.progress.{instance}   // In-progress quests
idx.quest.status.review.{instance}     // In-review quests
idx.quest.status.completed.{instance}  // Completed quests
idx.quest.status.failed.{instance}     // Failed quests
idx.quest.status.escalated.{instance}  // Escalated quests

idx.quest.agent.{agent}.{quest}        // Quests by agent
idx.quest.guild.{guild}.{quest}        // Quests with guild priority
idx.quest.parent.{parent}.{child}      // Sub-quest relationships
```

**Why presence-based indices?**
- List keys with prefix: `idx.quest.status.posted.*`
- No need to maintain JSON arrays (atomic updates without CAS)
- Wildcard delete: remove `idx.quest.status.posted.{id}` on claim

## Architecture

```
┌─────────────────────────────────────────────────┐
│              NATSQuestBoard                      │
│  (implements QuestBoard interface)              │
├─────────────────────────────────────────────────┤
│  Configuration:                                 │
│  - Org, Platform, BoardName (entity ID prefix)  │
│  - KV Bucket name derived from config           │
├─────────────────────────────────────────────────┤
│  State (single KV bucket):                      │
│  - quest.{id}, agent.{id}, etc.                │
│  - idx.quest.status.{status}.{id}              │
├─────────────────────────────────────────────────┤
│  Events (NATS subjects):                        │
│  - quest.lifecycle.posted                       │
│  - quest.lifecycle.claimed                      │
│  - etc.                                         │
└─────────────────────────────────────────────────┘
```

## Files to Create/Modify

| File | Purpose |
|------|---------|
| `vocab.go` | Register semdragons predicates with vocabulary system |
| `entityid.go` | Entity ID helpers for semdragons types |
| `storage.go` | **Rewrite:** Single bucket, dotted keys, presence indices |
| `events.go` | **Rewrite:** Use vocabulary predicates for subjects |
| `board.go` | **Rewrite:** Config-based, uses entity IDs |
| `board_posting.go` | Update to use new patterns |
| `board_claiming.go` | Update to use new patterns |
| `board_execution.go` | Update to use new patterns |
| `board_lifecycle.go` | Update to use new patterns |
| `board_queries.go` | Update to use new patterns |
| `board_test.go` | Update tests for new patterns |

## Implementation Order

### Phase 1: Foundation

1. **vocab.go** - Register predicates
   ```go
   const (
       PredicateQuestPosted    = "quest.lifecycle.posted"
       PredicateQuestClaimed   = "quest.lifecycle.claimed"
       // ...
   )

   func RegisterPredicates() {
       vocabulary.Register(PredicateQuestPosted,
           vocabulary.WithDescription("Quest added to board"),
           vocabulary.WithDataType("QuestPostedPayload"))
       // ...
   }
   ```

2. **entityid.go** - Entity ID helpers
   ```go
   type BoardConfig struct {
       Org      string
       Platform string
       Board    string
   }

   func (c *BoardConfig) QuestID(instance string) string {
       return fmt.Sprintf("%s.%s.game.%s.quest.%s",
           c.Org, c.Platform, c.Board, instance)
   }

   func (c *BoardConfig) BucketName() string {
       return fmt.Sprintf("semdragons.%s.%s.%s",
           c.Org, c.Platform, c.Board)
   }
   ```

### Phase 2: Storage Layer

3. **storage.go** - Rewrite with single bucket + dotted keys
   ```go
   type Storage struct {
       kv     *natsclient.KVStore
       config *BoardConfig
   }

   func (s *Storage) QuestKey(id string) string {
       return "quest." + id
   }

   func (s *Storage) IndexKey(status QuestStatus, id string) string {
       return fmt.Sprintf("idx.quest.status.%s.%s", status, id)
   }

   // Presence-based index operations
   func (s *Storage) AddToIndex(ctx context.Context, key string) error {
       _, err := s.kv.Put(ctx, key, []byte("1"))
       return err
   }

   func (s *Storage) RemoveFromIndex(ctx context.Context, key string) error {
       return s.kv.Delete(ctx, key)
   }

   func (s *Storage) ListIndex(ctx context.Context, prefix string) ([]string, error) {
       keys, err := s.kv.Keys(ctx)
       // Filter by prefix, extract instance IDs
   }
   ```

### Phase 3: Events

4. **events.go** - Use vocabulary predicates
   ```go
   var (
       SubjectQuestPosted = natsclient.NewSubject[QuestPostedPayload](
           PredicateQuestPosted)
       SubjectQuestClaimed = natsclient.NewSubject[QuestClaimedPayload](
           PredicateQuestClaimed)
       // ...
   )
   ```

### Phase 4: Board Implementation

5. **board.go** - Config-driven, entity IDs
   ```go
   type NATSQuestBoard struct {
       client  *natsclient.Client
       config  *BoardConfig
       storage *Storage
       events  *EventPublisher
   }

   func NewNATSQuestBoard(ctx context.Context, client *natsclient.Client,
       config BoardConfig) (*NATSQuestBoard, error) {
       // Create single bucket with config-derived name
       // Initialize storage with dotted key helpers
   }
   ```

6. **board_*.go** - Update all methods to use:
   - `config.QuestID(instance)` for entity IDs
   - `storage.IndexKey(status, id)` for indices
   - Vocabulary predicates for events

### Phase 5: Tests

7. **board_test.go** - Update with:
   - Test config setup
   - Verify entity ID format
   - Verify KV key patterns
   - Verify event subjects

## Key Patterns

### Creating a Quest
```go
func (b *NATSQuestBoard) PostQuest(ctx context.Context, quest Quest) (*Quest, error) {
    // Generate instance ID
    instance := randomHex(8)

    // Set full entity ID
    quest.ID = QuestID(b.config.QuestID(instance))

    // Store quest state
    key := b.storage.QuestKey(instance)
    data, _ := json.Marshal(quest)
    b.storage.kv.Put(ctx, key, data)

    // Add to posted index (presence-based)
    indexKey := b.storage.IndexKey(QuestPosted, instance)
    b.storage.AddToIndex(ctx, indexKey)

    // Emit event with vocabulary predicate
    b.events.Publish(ctx, SubjectQuestPosted, QuestPostedPayload{
        Quest:    quest,
        PostedAt: time.Now(),
    })

    return &quest, nil
}
```

### Claiming a Quest
```go
func (b *NATSQuestBoard) ClaimQuest(ctx context.Context, questID QuestID, agentID AgentID) error {
    instance := extractInstance(questID) // Get last part of entity ID

    // Load and update quest atomically
    err := b.storage.UpdateQuest(ctx, instance, func(q *Quest) error {
        if q.Status != QuestPosted {
            return errors.New("quest not available")
        }
        q.Status = QuestClaimed
        q.ClaimedBy = &agentID
        return nil
    })

    // Move between indices
    b.storage.RemoveFromIndex(ctx, b.storage.IndexKey(QuestPosted, instance))
    b.storage.AddToIndex(ctx, b.storage.IndexKey(QuestClaimed, instance))

    // Add to agent's quest list
    agentInstance := extractInstance(agentID)
    b.storage.AddToIndex(ctx, fmt.Sprintf("idx.quest.agent.%s.%s", agentInstance, instance))

    // Emit event
    b.events.Publish(ctx, SubjectQuestClaimed, ...)
}
```

### Listing Posted Quests
```go
func (b *NATSQuestBoard) AvailableQuests(ctx context.Context, ...) ([]Quest, error) {
    // List all keys matching the index prefix
    prefix := "idx.quest.status.posted."
    keys, _ := b.storage.ListKeysWithPrefix(ctx, prefix)

    quests := make([]Quest, 0, len(keys))
    for _, key := range keys {
        instance := strings.TrimPrefix(key, prefix)
        quest, _ := b.storage.GetQuest(ctx, instance)
        quests = append(quests, *quest)
    }
    return quests, nil
}
```

## Verification

1. **Entity IDs**: `nats kv get bucket quest.abc123` returns quest with full entity ID
2. **Predicates**: `nats sub "quest.lifecycle.>"` shows all quest events
3. **Indices**: `nats kv keys bucket | grep "idx.quest.status.posted"` lists posted quests
4. **Wildcards**: Keys support NATS wildcard queries

## Migration Notes

The existing implementation has:
- Separate buckets per entity type → merge to single bucket
- Colon separators in keys → change to dots
- Simple IDs like `q-abc123` → full entity IDs
- Generic event subjects → vocabulary predicates

Clean slate approach: delete existing board_*.go files, rewrite with correct patterns.
