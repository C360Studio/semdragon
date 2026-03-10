// Package semdragons provides graph client utilities for interacting with the semstreams graph system.
package semdragons

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semdragons/domain"
)

// GraphClient provides access to the semstreams graph system for semdragons components.
// It wraps natsclient.KVStore for all KV operations, inheriting CAS support, timeouts,
// and retry semantics. Entity-specific methods handle marshaling between Graphable
// entities and the KV bucket.
//
// Design: Components use GraphClient instead of direct storage operations.
// Entity writes go to the ENTITY_STATES KV bucket (shared with the graph pipeline),
// which acts as both the source of truth and the event stream (the NATS KV "Twofer").
type GraphClient struct {
	nats   *natsclient.Client
	config *domain.BoardConfig
	bucket jetstream.KeyValue    // raw bucket for KVBucket() backward compat
	store  *natsclient.KVStore   // CAS-aware KV operations
}

// NewGraphClient creates a new graph client for interacting with the semstreams graph system.
func NewGraphClient(nats *natsclient.Client, config *domain.BoardConfig) *GraphClient {
	return &GraphClient{
		nats:   nats,
		config: config,
	}
}

// Config returns the board configuration.
func (gc *GraphClient) Config() *domain.BoardConfig {
	return gc.config
}

// EnsureBucket creates the board-specific KV bucket if it doesn't exist.
// Uses CreateKeyValueBucket (get-or-create) so it's idempotent.
// History is set to 10 per the JetStream tuning guide for entity state audit trails.
// Call this at startup before any reads or writes.
func (gc *GraphClient) EnsureBucket(ctx context.Context) error {
	bucket, err := gc.nats.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      gc.config.BucketName(),
		Description: fmt.Sprintf("Entity states for board %s", gc.config.Board),
		History:     10,
	})
	if err != nil {
		return fmt.Errorf("ensure bucket %s: %w", gc.config.BucketName(), err)
	}
	gc.bucket = bucket
	gc.store = gc.nats.NewKVStore(bucket)
	return nil
}

// ensureStore returns the cached KVStore, creating it on first access.
func (gc *GraphClient) ensureStore(ctx context.Context) (*natsclient.KVStore, error) {
	if gc.store != nil {
		return gc.store, nil
	}
	bucket, err := gc.nats.GetKeyValueBucket(ctx, gc.config.BucketName())
	if err != nil {
		return nil, fmt.Errorf("get bucket %s: %w", gc.config.BucketName(), err)
	}
	gc.bucket = bucket
	gc.store = gc.nats.NewKVStore(bucket)
	return gc.store, nil
}

// marshalEntityState converts a Graphable entity into serialized EntityState bytes.
func marshalEntityState(entity graph.Graphable, eventType string) ([]byte, error) {
	entityState := &graph.EntityState{
		ID: entity.EntityID(),
		Triples: entity.Triples(),
		MessageType: message.Type{
			Domain:   "semdragons",
			Category: eventType,
			Version:  "v1",
		},
		Version:   1,
		UpdatedAt: time.Now(),
	}
	return json.Marshal(entityState)
}

// unmarshalEntityState deserializes bytes into an EntityState.
func unmarshalEntityState(data []byte) (*graph.EntityState, error) {
	var entity graph.EntityState
	if err := json.Unmarshal(data, &entity); err != nil {
		return nil, fmt.Errorf("unmarshal entity: %w", err)
	}
	return &entity, nil
}

// =============================================================================
// ENTITY EMISSION
// =============================================================================

// EmitEntity publishes a Graphable entity to the graph system (last-writer-wins).
// Writes entity state to the ENTITY_STATES KV bucket. KV watchers are notified
// immediately, enabling the entity-centric reactive pattern.
//
// For contested writes (e.g., quest claims), use EmitEntityCAS instead.
func (gc *GraphClient) EmitEntity(ctx context.Context, entity graph.Graphable, eventType string) error {
	data, err := marshalEntityState(entity, eventType)
	if err != nil {
		return err
	}

	store, err := gc.ensureStore(ctx)
	if err != nil {
		return err
	}

	if _, err := store.Put(ctx, entity.EntityID(), data); err != nil {
		return fmt.Errorf("put entity state: %w", err)
	}
	return nil
}

// EmitEntityUpdate publishes an updated entity state (last-writer-wins).
// This is used when an entity's state changes (e.g., quest claimed, agent leveled up).
func (gc *GraphClient) EmitEntityUpdate(ctx context.Context, entity graph.Graphable, eventType string) error {
	return gc.EmitEntity(ctx, entity, eventType)
}

// EmitEntityCAS writes entity state only if the KV revision matches (Compare-And-Swap).
// Returns natsclient.ErrKVRevisionMismatch if the entity was modified since it was read.
// Use this for contested writes where multiple processors may update the same entity
// concurrently (e.g., quest claims).
//
// The revision parameter should come from a prior GetEntityDirectWithRevision or
// GetQuestWithRevision call.
func (gc *GraphClient) EmitEntityCAS(ctx context.Context, entity graph.Graphable, eventType string, revision uint64) error {
	data, err := marshalEntityState(entity, eventType)
	if err != nil {
		return err
	}

	store, err := gc.ensureStore(ctx)
	if err != nil {
		return err
	}

	if _, err := store.Update(ctx, entity.EntityID(), data, revision); err != nil {
		return err // natsclient returns ErrKVRevisionMismatch on conflict
	}
	return nil
}

// =============================================================================
// DIRECT ENTITY STATE ACCESS
// =============================================================================

// GetEntityDirect retrieves an entity directly from the ENTITY_STATES KV bucket.
// This bypasses the graph-query layer for faster reads when the entity ID is known.
func (gc *GraphClient) GetEntityDirect(ctx context.Context, entityID string) (*graph.EntityState, error) {
	store, err := gc.ensureStore(ctx)
	if err != nil {
		return nil, err
	}

	entry, err := store.Get(ctx, entityID)
	if err != nil {
		return nil, fmt.Errorf("get entity: %w", err)
	}

	return unmarshalEntityState(entry.Value)
}

// GetEntityDirectWithRevision retrieves an entity and its KV revision for CAS operations.
// The returned revision should be passed to EmitEntityCAS for optimistic concurrency control.
func (gc *GraphClient) GetEntityDirectWithRevision(ctx context.Context, entityID string) (*graph.EntityState, uint64, error) {
	store, err := gc.ensureStore(ctx)
	if err != nil {
		return nil, 0, err
	}

	entry, err := store.Get(ctx, entityID)
	if err != nil {
		return nil, 0, fmt.Errorf("get entity: %w", err)
	}

	entity, err := unmarshalEntityState(entry.Value)
	if err != nil {
		return nil, 0, err
	}

	return entity, entry.Revision, nil
}

// =============================================================================
// HELPER METHODS FOR SEMDRAGONS ENTITIES
// =============================================================================

// GetQuest retrieves a quest by its quest ID (instance portion).
func (gc *GraphClient) GetQuest(ctx context.Context, questID domain.QuestID) (*graph.EntityState, error) {
	instance := domain.ExtractInstance(string(questID))
	entityID := gc.config.QuestEntityID(instance)
	return gc.GetEntityDirect(ctx, entityID)
}

// GetQuestWithRevision retrieves a quest and its KV revision for CAS operations.
func (gc *GraphClient) GetQuestWithRevision(ctx context.Context, questID domain.QuestID) (*graph.EntityState, uint64, error) {
	instance := domain.ExtractInstance(string(questID))
	entityID := gc.config.QuestEntityID(instance)
	return gc.GetEntityDirectWithRevision(ctx, entityID)
}

// GetAgent retrieves an agent by its agent ID (instance portion).
func (gc *GraphClient) GetAgent(ctx context.Context, agentID domain.AgentID) (*graph.EntityState, error) {
	instance := domain.ExtractInstance(string(agentID))
	entityID := gc.config.AgentEntityID(instance)
	return gc.GetEntityDirect(ctx, entityID)
}

// GetAgentWithRevision retrieves an agent and its KV revision for CAS operations.
func (gc *GraphClient) GetAgentWithRevision(ctx context.Context, agentID domain.AgentID) (*graph.EntityState, uint64, error) {
	instance := domain.ExtractInstance(string(agentID))
	entityID := gc.config.AgentEntityID(instance)
	return gc.GetEntityDirectWithRevision(ctx, entityID)
}

// GetParty retrieves a party by its party ID (instance portion).
func (gc *GraphClient) GetParty(ctx context.Context, partyID domain.PartyID) (*graph.EntityState, error) {
	instance := domain.ExtractInstance(string(partyID))
	entityID := gc.config.PartyEntityID(instance)
	return gc.GetEntityDirect(ctx, entityID)
}

// GetGuild retrieves a guild by its guild ID (instance portion).
func (gc *GraphClient) GetGuild(ctx context.Context, guildID domain.GuildID) (*graph.EntityState, error) {
	instance := domain.ExtractInstance(string(guildID))
	entityID := gc.config.GuildEntityID(instance)
	return gc.GetEntityDirect(ctx, entityID)
}

// GetBattle retrieves a battle by its battle ID (instance portion).
func (gc *GraphClient) GetBattle(ctx context.Context, battleID domain.BattleID) (*graph.EntityState, error) {
	instance := domain.ExtractInstance(string(battleID))
	entityID := gc.config.BattleEntityID(instance)
	return gc.GetEntityDirect(ctx, entityID)
}

// GetStoreItem retrieves a store item by its item ID (instance portion).
func (gc *GraphClient) GetStoreItem(ctx context.Context, itemID string) (*graph.EntityState, error) {
	entityID := gc.config.StoreItemEntityID(itemID)
	return gc.GetEntityDirect(ctx, entityID)
}

// ListStoreItemsByPrefix retrieves all store items on this board from KV.
func (gc *GraphClient) ListStoreItemsByPrefix(ctx context.Context, limit int) ([]graph.EntityState, error) {
	return gc.ListEntitiesByType(ctx, domain.EntityTypeStoreItem, limit)
}

// GetPeerReview retrieves a peer review by its review ID (instance portion).
func (gc *GraphClient) GetPeerReview(ctx context.Context, reviewID domain.PeerReviewID) (*graph.EntityState, error) {
	instance := domain.ExtractInstance(string(reviewID))
	entityID := gc.config.PeerReviewEntityID(instance)
	return gc.GetEntityDirect(ctx, entityID)
}

// ListPeerReviewsByPrefix retrieves all peer reviews on this board from KV.
func (gc *GraphClient) ListPeerReviewsByPrefix(ctx context.Context, limit int) ([]graph.EntityState, error) {
	return gc.ListEntitiesByType(ctx, domain.EntityTypePeerReview, limit)
}

// ListEntitiesByType retrieves all entities of a given type directly from the
// ENTITY_STATES KV bucket. This reads from the source of truth without requiring
// the graph-ingest query service to be running.
func (gc *GraphClient) ListEntitiesByType(ctx context.Context, entityType string, limit int) ([]graph.EntityState, error) {
	if limit <= 0 {
		limit = 100
	}

	store, err := gc.ensureStore(ctx)
	if err != nil {
		return nil, err
	}

	prefix := gc.config.TypePrefix(entityType) + "."
	keys, err := store.KeysByPrefix(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("list keys for %s: %w", entityType, err)
	}

	entities := make([]graph.EntityState, 0, len(keys))
	for i, key := range keys {
		if i >= limit {
			break
		}
		entry, err := store.Get(ctx, key)
		if err != nil {
			continue // Key may have been deleted between list and get
		}
		entity, err := unmarshalEntityState(entry.Value)
		if err != nil {
			continue
		}
		entities = append(entities, *entity)
	}

	return entities, nil
}

// ListQuestsByPrefix retrieves all quests on this board from KV.
func (gc *GraphClient) ListQuestsByPrefix(ctx context.Context, limit int) ([]graph.EntityState, error) {
	return gc.ListEntitiesByType(ctx, domain.EntityTypeQuest, limit)
}

// ListAgentsByPrefix retrieves all agents on this board from KV.
func (gc *GraphClient) ListAgentsByPrefix(ctx context.Context, limit int) ([]graph.EntityState, error) {
	return gc.ListEntitiesByType(ctx, domain.EntityTypeAgent, limit)
}

// ListGuildsByPrefix retrieves all guilds on this board from KV.
func (gc *GraphClient) ListGuildsByPrefix(ctx context.Context, limit int) ([]graph.EntityState, error) {
	return gc.ListEntitiesByType(ctx, domain.EntityTypeGuild, limit)
}

// ListPartiesByPrefix retrieves all parties on this board from KV.
func (gc *GraphClient) ListPartiesByPrefix(ctx context.Context, limit int) ([]graph.EntityState, error) {
	return gc.ListEntitiesByType(ctx, domain.EntityTypeParty, limit)
}

// =============================================================================
// ENTITY STATE KV - Direct entity state management (entity-centric architecture)
// =============================================================================
// The NATS KV "Twofer": KV buckets are backed by JetStream streams, giving us
// unified state + event semantics. Put = update state AND emit event. Watch =
// subscribe to state changes. No separate event payloads needed.
// =============================================================================

// PutEntityState writes entity state directly to the ENTITY_STATES KV bucket.
// This is the primary write path for the entity-centric architecture.
// KV watchers will be notified of this change immediately.
func (gc *GraphClient) PutEntityState(ctx context.Context, entity graph.Graphable, eventType string) error {
	return gc.EmitEntity(ctx, entity, eventType)
}

// WatchEntityType watches for changes to entities of a specific type on this board.
// Returns a KeyWatcher that emits updates when entities are created, updated, or deleted.
// The pattern matches all entities of the type, e.g., "c360.prod.game.board1.quest.*".
//
// Callers must call watcher.Stop() when done.
func (gc *GraphClient) WatchEntityType(ctx context.Context, entityType string) (jetstream.KeyWatcher, error) {
	store, err := gc.ensureStore(ctx)
	if err != nil {
		return nil, err
	}

	pattern := gc.config.TypePrefix(entityType) + ".*"
	return store.Watch(ctx, pattern)
}

// WatchEntity watches for changes to a single entity by its full entity ID.
// Returns a KeyWatcher that emits updates when the entity changes.
//
// Callers must call watcher.Stop() when done.
func (gc *GraphClient) WatchEntity(ctx context.Context, entityID string) (jetstream.KeyWatcher, error) {
	store, err := gc.ensureStore(ctx)
	if err != nil {
		return nil, err
	}

	return store.Watch(ctx, entityID)
}

// DecodeEntityState unmarshals a KV entry value into an EntityState.
// Used by KV watchers to process entity state changes.
func DecodeEntityState(entry jetstream.KeyValueEntry) (*graph.EntityState, error) {
	if entry == nil || entry.Value() == nil {
		return nil, nil
	}
	return unmarshalEntityState(entry.Value())
}

// =============================================================================
// KV ACCESS FOR NON-ENTITY STATE
// =============================================================================

// KVBucket returns the board-specific KV bucket for watching entity state changes.
// Processors use this for KV Watch operations with TypePrefix-based key patterns.
func (gc *GraphClient) KVBucket(ctx context.Context) (jetstream.KeyValue, error) {
	if gc.bucket != nil {
		return gc.bucket, nil
	}
	// Ensure store is initialized (which also sets bucket)
	if _, err := gc.ensureStore(ctx); err != nil {
		return nil, err
	}
	return gc.bucket, nil
}

// Client returns the underlying NATS client for advanced operations.
func (gc *GraphClient) Client() *natsclient.Client {
	return gc.nats
}
