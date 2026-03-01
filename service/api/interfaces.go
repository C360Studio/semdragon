package api

import (
	"context"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/graph"
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
