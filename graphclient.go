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
)

// GraphClient provides access to the semstreams graph system for semdragons components.
// It wraps the natsclient and provides methods for emitting Graphable entities
// and querying the graph.
//
// Design: Components use GraphClient instead of direct storage operations.
// Entities are emitted as events to JetStream, where graph-ingest consumes them
// and persists EntityState. Queries go through graph-ingest's request handlers.
type GraphClient struct {
	nats   *natsclient.Client
	config *BoardConfig
}

// NewGraphClient creates a new graph client for interacting with the semstreams graph system.
func NewGraphClient(nats *natsclient.Client, config *BoardConfig) *GraphClient {
	return &GraphClient{
		nats:   nats,
		config: config,
	}
}

// Config returns the board configuration.
func (gc *GraphClient) Config() *BoardConfig {
	return gc.config
}

// =============================================================================
// ENTITY EMISSION
// =============================================================================

// entityPayload wraps a Graphable entity for BaseMessage compatibility.
// BaseMessage requires a Payload interface, but we want to emit Graphable entities.
type entityPayload struct {
	Entity graph.Graphable `json:"entity"`
}

// MarshalJSON implements json.Marshaler for BaseMessage Payload requirement.
func (ep entityPayload) MarshalJSON() ([]byte, error) {
	return json.Marshal(ep.Entity)
}

// EmitEntity publishes a Graphable entity to the graph system via JetStream.
// The entity is published to "entity.{category}" where category comes from eventType.
// graph-ingest subscribes to "entity.>" and persists the EntityState.
//
// Example:
//
//	err := gc.EmitEntity(ctx, &quest, "quest.posted")
//	// Publishes to "entity.quest.posted"
func (gc *GraphClient) EmitEntity(ctx context.Context, entity graph.Graphable, eventType string) error {
	msgType := message.Type{
		Domain:   "semdragons",
		Category: eventType,
		Version:  "v1",
	}

	// Create EntityState directly from Graphable
	entityState := &graph.EntityState{
		ID:          entity.EntityID(),
		Triples:     entity.Triples(),
		MessageType: msgType,
		Version:     1,
		UpdatedAt:   time.Now(),
	}

	// Serialize and publish
	data, err := json.Marshal(entityState)
	if err != nil {
		return fmt.Errorf("marshal entity state: %w", err)
	}

	subject := fmt.Sprintf("entity.%s", eventType)
	return gc.nats.PublishToStream(ctx, subject, data)
}

// EmitEntityUpdate publishes an updated entity state.
// This is used when an entity's state changes (e.g., quest claimed, agent leveled up).
func (gc *GraphClient) EmitEntityUpdate(ctx context.Context, entity graph.Graphable, eventType string) error {
	return gc.EmitEntity(ctx, entity, eventType)
}

// =============================================================================
// ENTITY QUERIES
// =============================================================================

// entityQueryRequest is the request format for graph.ingest.query.entity
type entityQueryRequest struct {
	ID string `json:"id"`
}

// entityQueryResponse is the response format from graph.ingest.query.entity
type entityQueryResponse struct {
	Entity *graph.EntityState `json:"entity"`
	Error  string             `json:"error,omitempty"`
}

// GetEntity retrieves an entity by its full 6-part entity ID.
// Returns nil if the entity is not found.
func (gc *GraphClient) GetEntity(ctx context.Context, entityID string) (*graph.EntityState, error) {
	req := entityQueryRequest{ID: entityID}
	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	respData, err := gc.nats.Request(ctx, "graph.ingest.query.entity", reqData, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("query entity: %w", err)
	}

	var resp entityQueryResponse
	if err := json.Unmarshal(respData, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if resp.Error != "" {
		return nil, fmt.Errorf("query error: %s", resp.Error)
	}

	return resp.Entity, nil
}

// batchQueryRequest is the request format for graph.ingest.query.batch
type batchQueryRequest struct {
	IDs []string `json:"ids"`
}

// batchQueryResponse is the response format from graph.ingest.query.batch
type batchQueryResponse struct {
	Entities []graph.EntityState `json:"entities"`
	Error    string              `json:"error,omitempty"`
}

// BatchGet retrieves multiple entities by their entity IDs.
// Returns only the entities that were found.
func (gc *GraphClient) BatchGet(ctx context.Context, ids []string) ([]graph.EntityState, error) {
	if len(ids) == 0 {
		return []graph.EntityState{}, nil
	}

	req := batchQueryRequest{IDs: ids}
	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	respData, err := gc.nats.Request(ctx, "graph.ingest.query.batch", reqData, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("batch query: %w", err)
	}

	var resp batchQueryResponse
	if err := json.Unmarshal(respData, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if resp.Error != "" {
		return nil, fmt.Errorf("query error: %s", resp.Error)
	}

	return resp.Entities, nil
}

// prefixQueryRequest is the request format for graph.ingest.query.prefix
type prefixQueryRequest struct {
	Prefix string `json:"prefix"`
	Limit  int    `json:"limit,omitempty"`
}

// prefixQueryResponse is the response format from graph.ingest.query.prefix
type prefixQueryResponse struct {
	Entities []graph.EntityState `json:"entities"`
	Error    string              `json:"error,omitempty"`
}

// QueryByPrefix retrieves all entities matching an entity ID prefix.
// This enables hierarchical queries like "all quests on board1":
//
//	entities, err := gc.QueryByPrefix(ctx, "c360.prod.game.board1.quest", 100)
func (gc *GraphClient) QueryByPrefix(ctx context.Context, prefix string, limit int) ([]graph.EntityState, error) {
	if limit <= 0 {
		limit = 100
	}

	req := prefixQueryRequest{Prefix: prefix, Limit: limit}
	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	respData, err := gc.nats.Request(ctx, "graph.ingest.query.prefix", reqData, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("prefix query: %w", err)
	}

	var resp prefixQueryResponse
	if err := json.Unmarshal(respData, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if resp.Error != "" {
		return nil, fmt.Errorf("query error: %s", resp.Error)
	}

	return resp.Entities, nil
}

// =============================================================================
// PREDICATE INDEX QUERIES
// =============================================================================

// QueryByPredicate retrieves entity IDs that have a specific predicate.
// This uses the PREDICATE_INDEX KV bucket maintained by graph-index.
//
// Example: Find all entities with quest.status.state predicate:
//
//	ids, err := gc.QueryByPredicate(ctx, "quest.status.state")
func (gc *GraphClient) QueryByPredicate(ctx context.Context, predicate string) ([]string, error) {
	bucket, err := gc.nats.GetKeyValueBucket(ctx, graph.BucketPredicateIndex)
	if err != nil {
		return nil, fmt.Errorf("get predicate index bucket: %w", err)
	}

	entry, err := bucket.Get(ctx, predicate)
	if err != nil {
		// Key not found means no entities have this predicate
		return []string{}, nil
	}

	var ids []string
	if err := json.Unmarshal(entry.Value(), &ids); err != nil {
		return nil, fmt.Errorf("unmarshal predicate index: %w", err)
	}

	return ids, nil
}

// =============================================================================
// DIRECT ENTITY STATE ACCESS
// =============================================================================

// GetEntityDirect retrieves an entity directly from the ENTITY_STATES KV bucket.
// This bypasses the graph-query layer for faster reads when the entity ID is known.
func (gc *GraphClient) GetEntityDirect(ctx context.Context, entityID string) (*graph.EntityState, error) {
	bucket, err := gc.nats.GetKeyValueBucket(ctx, graph.BucketEntityStates)
	if err != nil {
		return nil, fmt.Errorf("get entity states bucket: %w", err)
	}

	entry, err := bucket.Get(ctx, entityID)
	if err != nil {
		return nil, fmt.Errorf("get entity: %w", err)
	}

	var entity graph.EntityState
	if err := json.Unmarshal(entry.Value(), &entity); err != nil {
		return nil, fmt.Errorf("unmarshal entity: %w", err)
	}

	return &entity, nil
}

// =============================================================================
// HELPER METHODS FOR SEMDRAGONS ENTITIES
// =============================================================================

// GetQuest retrieves a quest by its quest ID (instance portion).
func (gc *GraphClient) GetQuest(ctx context.Context, questID QuestID) (*graph.EntityState, error) {
	instance := ExtractInstance(string(questID))
	entityID := gc.config.QuestEntityID(instance)
	return gc.GetEntityDirect(ctx, entityID)
}

// GetAgent retrieves an agent by its agent ID (instance portion).
func (gc *GraphClient) GetAgent(ctx context.Context, agentID AgentID) (*graph.EntityState, error) {
	instance := ExtractInstance(string(agentID))
	entityID := gc.config.AgentEntityID(instance)
	return gc.GetEntityDirect(ctx, entityID)
}

// GetParty retrieves a party by its party ID (instance portion).
func (gc *GraphClient) GetParty(ctx context.Context, partyID PartyID) (*graph.EntityState, error) {
	instance := ExtractInstance(string(partyID))
	entityID := gc.config.PartyEntityID(instance)
	return gc.GetEntityDirect(ctx, entityID)
}

// GetGuild retrieves a guild by its guild ID (instance portion).
func (gc *GraphClient) GetGuild(ctx context.Context, guildID GuildID) (*graph.EntityState, error) {
	instance := ExtractInstance(string(guildID))
	entityID := gc.config.GuildEntityID(instance)
	return gc.GetEntityDirect(ctx, entityID)
}

// GetBattle retrieves a battle by its battle ID (instance portion).
func (gc *GraphClient) GetBattle(ctx context.Context, battleID BattleID) (*graph.EntityState, error) {
	instance := ExtractInstance(string(battleID))
	entityID := gc.config.BattleEntityID(instance)
	return gc.GetEntityDirect(ctx, entityID)
}

// ListQuestsByPrefix retrieves all quests on this board.
func (gc *GraphClient) ListQuestsByPrefix(ctx context.Context, limit int) ([]graph.EntityState, error) {
	prefix := gc.config.TypePrefix(EntityTypeQuest)
	return gc.QueryByPrefix(ctx, prefix, limit)
}

// ListAgentsByPrefix retrieves all agents on this board.
func (gc *GraphClient) ListAgentsByPrefix(ctx context.Context, limit int) ([]graph.EntityState, error) {
	prefix := gc.config.TypePrefix(EntityTypeAgent)
	return gc.QueryByPrefix(ctx, prefix, limit)
}

// ListGuildsByPrefix retrieves all guilds on this board.
func (gc *GraphClient) ListGuildsByPrefix(ctx context.Context, limit int) ([]graph.EntityState, error) {
	prefix := gc.config.TypePrefix(EntityTypeGuild)
	return gc.QueryByPrefix(ctx, prefix, limit)
}

// ListPartiesByPrefix retrieves all parties on this board.
func (gc *GraphClient) ListPartiesByPrefix(ctx context.Context, limit int) ([]graph.EntityState, error) {
	prefix := gc.config.TypePrefix(EntityTypeParty)
	return gc.QueryByPrefix(ctx, prefix, limit)
}

// =============================================================================
// KV ACCESS FOR NON-ENTITY STATE
// =============================================================================

// KVBucket returns the KV bucket for the board.
// Used for simple key-value data that doesn't need full entity treatment.
func (gc *GraphClient) KVBucket(ctx context.Context) (jetstream.KeyValue, error) {
	return gc.nats.GetKeyValueBucket(ctx, gc.config.BucketName())
}

// Client returns the underlying NATS client for advanced operations.
func (gc *GraphClient) Client() *natsclient.Client {
	return gc.nats
}
