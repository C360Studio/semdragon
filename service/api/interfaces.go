package api

import (
	"context"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentstore"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/model"
)

// GraphQuerier abstracts GraphClient read/write operations used by handlers.
// Extracting this interface enables handlers to be tested without a live NATS connection.
type GraphQuerier interface {
	Config() *semdragons.BoardConfig
	GetQuest(ctx context.Context, id semdragons.QuestID) (*graph.EntityState, error)
	GetAgent(ctx context.Context, id semdragons.AgentID) (*graph.EntityState, error)
	GetBattle(ctx context.Context, id semdragons.BattleID) (*graph.EntityState, error)
	ListQuestsByPrefix(ctx context.Context, limit int) ([]graph.EntityState, error)
	ListAgentsByPrefix(ctx context.Context, limit int) ([]graph.EntityState, error)
	ListEntitiesByType(ctx context.Context, entityType string, limit int) ([]graph.EntityState, error)
	EmitEntity(ctx context.Context, entity graph.Graphable, eventType string) error
	EmitEntityUpdate(ctx context.Context, entity graph.Graphable, eventType string) error
}

// WorldStateProvider abstracts WorldStateAggregator for testing.
// The concrete *dmworldstate.WorldStateAggregator satisfies this interface.
type WorldStateProvider interface {
	WorldState(ctx context.Context) (*domain.WorldState, error)
}

// ModelResolver abstracts model.Registry for endpoint resolution in handlers.
// The concrete *model.Registry satisfies this interface.
type ModelResolver interface {
	Resolve(capability string) string
	GetEndpoint(name string) *model.EndpointConfig
}

// TrajectoryQuerier abstracts trajectory KV lookups for handler testing.
// Returns raw JSON bytes to avoid coupling to the agentic.Trajectory type.
type TrajectoryQuerier interface {
	GetTrajectory(ctx context.Context, id string) ([]byte, error)
}

// DMSessionReader abstracts DM session KV reads for handler testing.
type DMSessionReader interface {
	GetSession(ctx context.Context, sessionID string) (*DMChatSession, error)
}

// StoreProvider abstracts agentstore.Component for handler testing.
// The concrete *agentstore.Component satisfies this interface.
type StoreProvider interface {
	ListItems(agentTier domain.TrustTier) []agentstore.StoreItem
	Catalog() []agentstore.StoreItem
	GetItem(itemID string) (*agentstore.StoreItem, bool)
	Purchase(ctx context.Context, agentID domain.AgentID, itemID string,
		currentXP int64, currentLevel int, agentGuilds []domain.GuildID) (*agentstore.OwnedItem, error)
	CanAfford(itemID string, currentXP int64) (bool, int64)
	GetInventory(agentID domain.AgentID) *agentstore.AgentInventory
	UseConsumable(ctx context.Context, agentID domain.AgentID,
		consumableID string, questID *domain.QuestID) error
	GetActiveEffects(agentID domain.AgentID) []agentstore.ActiveEffect
}
