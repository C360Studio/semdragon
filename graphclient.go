// Package semdragons provides read-only graph access for product components.
package semdragons

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semdragons/domain"
)

// GraphClient is a read-only domain facade over SemStreams authoritative graph
// state. graph-ingest exclusively creates and writes ENTITY_STATES; this reader
// binds through the framework catalog and never exposes the raw bucket.
type GraphClient struct {
	nats   *natsclient.Client
	config *domain.BoardConfig

	mu     sync.Mutex
	bucket jetstream.KeyValue
}

// NewGraphClient creates a read-only graph client.
func NewGraphClient(nats *natsclient.Client, config *domain.BoardConfig) *GraphClient {
	return &GraphClient{nats: nats, config: config}
}

// Config returns the board configuration.
func (gc *GraphClient) Config() *domain.BoardConfig {
	return gc.config
}

// openBucket binds ENTITY_STATES through the SemStreams reader seam. An absent
// bucket remains a classified not-ready error naming graph-ingest as owner.
func (gc *GraphClient) openBucket(ctx context.Context) (jetstream.KeyValue, error) {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	if gc.bucket != nil {
		return gc.bucket, nil
	}
	if gc.nats == nil {
		return nil, fmt.Errorf("open entity states: NATS client is required")
	}
	bucket, err := graph.OpenCatalogBucket(ctx, gc.nats, graph.BucketEntityStates)
	if err != nil {
		return nil, fmt.Errorf("open entity states: %w", err)
	}
	gc.bucket = bucket
	return bucket, nil
}

// Ready reports whether the framework catalog bucket is available. It never
// creates or reconciles the bucket.
func (gc *GraphClient) Ready(ctx context.Context) error {
	_, err := gc.openBucket(ctx)
	return err
}

func unmarshalEntityState(data []byte) (*graph.EntityState, error) {
	var entity graph.EntityState
	if err := json.Unmarshal(data, &entity); err != nil {
		return nil, fmt.Errorf("unmarshal entity: %w", err)
	}
	return &entity, nil
}

// GetEntityDirect reads an entity by its canonical ID.
func (gc *GraphClient) GetEntityDirect(ctx context.Context, entityID string) (*graph.EntityState, error) {
	bucket, err := gc.openBucket(ctx)
	if err != nil {
		return nil, err
	}
	entry, err := bucket.Get(ctx, entityID)
	if err != nil {
		return nil, fmt.Errorf("get entity: %w", err)
	}
	return unmarshalEntityState(entry.Value())
}

// GetEntityDirectWithRevision returns an entity and the catalog entry revision.
// The revision is read evidence only; Semdragon does not write the bucket.
func (gc *GraphClient) GetEntityDirectWithRevision(ctx context.Context, entityID string) (*graph.EntityState, uint64, error) {
	bucket, err := gc.openBucket(ctx)
	if err != nil {
		return nil, 0, err
	}
	entry, err := bucket.Get(ctx, entityID)
	if err != nil {
		return nil, 0, fmt.Errorf("get entity: %w", err)
	}
	entity, err := unmarshalEntityState(entry.Value())
	if err != nil {
		return nil, 0, err
	}
	return entity, entry.Revision(), nil
}

// GetQuest reads a quest by its domain ID.
func (gc *GraphClient) GetQuest(ctx context.Context, id domain.QuestID) (*graph.EntityState, error) {
	return gc.GetEntityDirect(ctx, gc.config.QuestEntityID(domain.ExtractInstance(string(id))))
}

// GetQuestWithRevision reads a quest and its catalog entry revision.
func (gc *GraphClient) GetQuestWithRevision(ctx context.Context, id domain.QuestID) (*graph.EntityState, uint64, error) {
	return gc.GetEntityDirectWithRevision(ctx, gc.config.QuestEntityID(domain.ExtractInstance(string(id))))
}

// GetAgent reads an agent by its domain ID.
func (gc *GraphClient) GetAgent(ctx context.Context, id domain.AgentID) (*graph.EntityState, error) {
	return gc.GetEntityDirect(ctx, gc.config.AgentEntityID(domain.ExtractInstance(string(id))))
}

// GetAgentWithRevision reads an agent and its catalog entry revision.
func (gc *GraphClient) GetAgentWithRevision(ctx context.Context, id domain.AgentID) (*graph.EntityState, uint64, error) {
	return gc.GetEntityDirectWithRevision(ctx, gc.config.AgentEntityID(domain.ExtractInstance(string(id))))
}

// GetParty reads a party by its domain ID.
func (gc *GraphClient) GetParty(ctx context.Context, id domain.PartyID) (*graph.EntityState, error) {
	return gc.GetEntityDirect(ctx, gc.config.PartyEntityID(domain.ExtractInstance(string(id))))
}

// GetGuild reads a guild by its domain ID.
func (gc *GraphClient) GetGuild(ctx context.Context, id domain.GuildID) (*graph.EntityState, error) {
	return gc.GetEntityDirect(ctx, gc.config.GuildEntityID(domain.ExtractInstance(string(id))))
}

// GetBattle reads a battle by its domain ID.
func (gc *GraphClient) GetBattle(ctx context.Context, id domain.BattleID) (*graph.EntityState, error) {
	return gc.GetEntityDirect(ctx, gc.config.BattleEntityID(domain.ExtractInstance(string(id))))
}

// GetStoreItem reads a store item by its instance ID.
func (gc *GraphClient) GetStoreItem(ctx context.Context, id string) (*graph.EntityState, error) {
	return gc.GetEntityDirect(ctx, gc.config.StoreItemEntityID(id))
}

// GetPeerReview reads a peer review by its domain ID.
func (gc *GraphClient) GetPeerReview(ctx context.Context, id domain.PeerReviewID) (*graph.EntityState, error) {
	return gc.GetEntityDirect(ctx, gc.config.PeerReviewEntityID(domain.ExtractInstance(string(id))))
}

// ListEntitiesByType lists canonical entities whose IDs share the board/type prefix.
func (gc *GraphClient) ListEntitiesByType(ctx context.Context, entityType string, limit int) ([]graph.EntityState, error) {
	if limit <= 0 {
		limit = 100
	}
	bucket, err := gc.openBucket(ctx)
	if err != nil {
		return nil, err
	}
	prefix := gc.config.TypePrefix(entityType) + "."
	keys, err := natsclient.FilteredKeys(ctx, bucket, prefix+">")
	if err != nil {
		return nil, fmt.Errorf("list keys for %s: %w", entityType, err)
	}
	entities := make([]graph.EntityState, 0, min(limit, len(keys)))
	for _, key := range keys {
		if len(entities) >= limit {
			break
		}
		entry, getErr := bucket.Get(ctx, key)
		if getErr != nil {
			continue
		}
		entity, decodeErr := unmarshalEntityState(entry.Value())
		if decodeErr == nil {
			entities = append(entities, *entity)
		}
	}
	return entities, nil
}

// ListStoreItemsByPrefix lists store items on the configured board.
func (gc *GraphClient) ListStoreItemsByPrefix(ctx context.Context, limit int) ([]graph.EntityState, error) {
	return gc.ListEntitiesByType(ctx, domain.EntityTypeStoreItem, limit)
}

// ListPeerReviewsByPrefix lists peer reviews on the configured board.
func (gc *GraphClient) ListPeerReviewsByPrefix(ctx context.Context, limit int) ([]graph.EntityState, error) {
	return gc.ListEntitiesByType(ctx, domain.EntityTypePeerReview, limit)
}

// ListQuestsByPrefix lists quests on the configured board.
func (gc *GraphClient) ListQuestsByPrefix(ctx context.Context, limit int) ([]graph.EntityState, error) {
	return gc.ListEntitiesByType(ctx, domain.EntityTypeQuest, limit)
}

// ListAgentsByPrefix lists agents on the configured board.
func (gc *GraphClient) ListAgentsByPrefix(ctx context.Context, limit int) ([]graph.EntityState, error) {
	return gc.ListEntitiesByType(ctx, domain.EntityTypeAgent, limit)
}

// ListGuildsByPrefix lists guilds on the configured board.
func (gc *GraphClient) ListGuildsByPrefix(ctx context.Context, limit int) ([]graph.EntityState, error) {
	return gc.ListEntitiesByType(ctx, domain.EntityTypeGuild, limit)
}

// ListPartiesByPrefix lists parties on the configured board.
func (gc *GraphClient) ListPartiesByPrefix(ctx context.Context, limit int) ([]graph.EntityState, error) {
	return gc.ListEntitiesByType(ctx, domain.EntityTypeParty, limit)
}

// WatchEntityType watches canonical entities of one type. Callers must stop the watcher.
func (gc *GraphClient) WatchEntityType(ctx context.Context, entityType string) (jetstream.KeyWatcher, error) {
	bucket, err := gc.openBucket(ctx)
	if err != nil {
		return nil, err
	}
	return bucket.Watch(ctx, gc.config.TypePrefix(entityType)+".*")
}

// WatchEntity watches one canonical entity. Callers must stop the watcher.
func (gc *GraphClient) WatchEntity(ctx context.Context, entityID string) (jetstream.KeyWatcher, error) {
	bucket, err := gc.openBucket(ctx)
	if err != nil {
		return nil, err
	}
	return bucket.Watch(ctx, entityID)
}

// WatchBoard watches all canonical entities scoped to this board.
func (gc *GraphClient) WatchBoard(ctx context.Context) (jetstream.KeyWatcher, error) {
	bucket, err := gc.openBucket(ctx)
	if err != nil {
		return nil, err
	}
	return bucket.Watch(ctx, gc.config.Prefix()+".>")
}

// DecodeEntityState decodes a watcher entry.
func DecodeEntityState(entry jetstream.KeyValueEntry) (*graph.EntityState, error) {
	if entry == nil || entry.Value() == nil {
		return nil, nil
	}
	return unmarshalEntityState(entry.Value())
}
