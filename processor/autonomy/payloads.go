package autonomy

import (
	"errors"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/types"
)

// =============================================================================
// AUTONOMY PAYLOADS
// =============================================================================
// Event payloads for autonomy heartbeat operations. Each implements
// graph.Graphable for automatic persistence via graph-ingest.
// =============================================================================

// Ensure Graphable implementations.
var (
	_ graph.Graphable = (*EvaluatedPayload)(nil)
	_ graph.Graphable = (*IdlePayload)(nil)
	_ graph.Graphable = (*ClaimIntentPayload)(nil)
	_ graph.Graphable = (*ShopIntentPayload)(nil)
	_ graph.Graphable = (*GuildIntentPayload)(nil)
	_ graph.Graphable = (*UseIntentPayload)(nil)
)

// Typed subjects for autonomy events.
var (
	SubjectAutonomyEvaluated   = natsclient.NewSubject[EvaluatedPayload](domain.PredicateAutonomyEvaluated)
	SubjectAutonomyIdle        = natsclient.NewSubject[IdlePayload](domain.PredicateAutonomyIdle)
	SubjectAutonomyClaimIntent = natsclient.NewSubject[ClaimIntentPayload](domain.PredicateAutonomyClaimIntent)
	SubjectAutonomyShopIntent  = natsclient.NewSubject[ShopIntentPayload](domain.PredicateAutonomyShopIntent)
	SubjectAutonomyGuildIntent = natsclient.NewSubject[GuildIntentPayload](domain.PredicateAutonomyGuildIntent)
	SubjectAutonomyUseIntent   = natsclient.NewSubject[UseIntentPayload](domain.PredicateAutonomyUseIntent)
)

// --- TraceInfo for observability ---

// TraceInfo contains trace context for observability.
type TraceInfo struct {
	TrajectoryID string `json:"trajectory_id,omitempty"`
	SpanID       string `json:"span_id,omitempty"`
	ParentSpanID string `json:"parent_span_id,omitempty"`
}

// =============================================================================
// AUTONOMY EVALUATED PAYLOAD
// =============================================================================

// EvaluatedPayload is emitted on every heartbeat evaluation.
type EvaluatedPayload struct {
	AgentID     domain.AgentID     `json:"agent_id"`
	AgentStatus domain.AgentStatus `json:"agent_status"`
	ActionTaken string             `json:"action_taken"` // "none", "cooldown_expired", or action name
	Interval    time.Duration      `json:"interval"`     // Current heartbeat interval
	Timestamp   time.Time          `json:"timestamp"`
	Trace       TraceInfo          `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *EvaluatedPayload) EntityID() string { return string(p.AgentID) }

// Triples returns semantic triples for this event.
func (p *EvaluatedPayload) Triples() []message.Triple {
	source := "autonomy"
	entityID := string(p.AgentID)

	return []message.Triple{
		{Subject: entityID, Predicate: "agent.autonomy.status", Object: string(p.AgentStatus), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.autonomy.action", Object: p.ActionTaken, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.autonomy.interval_ms", Object: p.Interval.Milliseconds(), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

// Schema returns the type schema for this payload.
func (p *EvaluatedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "autonomy.evaluated", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *EvaluatedPayload) Validate() error {
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// =============================================================================
// AUTONOMY IDLE PAYLOAD
// =============================================================================

// IdlePayload is emitted when an idle agent has nothing actionable.
type IdlePayload struct {
	AgentID       domain.AgentID `json:"agent_id"`
	IdleDuration  time.Duration  `json:"idle_duration"`
	HasSuggestion bool           `json:"has_suggestion"`
	BackoffMs     int64          `json:"backoff_ms"` // Next interval after backoff
	Timestamp     time.Time      `json:"timestamp"`
	Trace         TraceInfo      `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *IdlePayload) EntityID() string { return string(p.AgentID) }

// Triples returns semantic triples for this event.
func (p *IdlePayload) Triples() []message.Triple {
	source := "autonomy"
	entityID := string(p.AgentID)

	return []message.Triple{
		{Subject: entityID, Predicate: "agent.autonomy.idle_duration_ms", Object: p.IdleDuration.Milliseconds(), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.autonomy.has_suggestion", Object: p.HasSuggestion, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.autonomy.backoff_ms", Object: p.BackoffMs, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

// Schema returns the type schema for this payload.
func (p *IdlePayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "autonomy.idle", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *IdlePayload) Validate() error {
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// =============================================================================
// INTENT PAYLOADS
// =============================================================================
// Intent payloads are emitted after each autonomous action succeeds. They record
// what the agent chose to do and why (quest score, item cost, guild ranking, etc.).
// In Phase 5a these are purely observability events. Phase 5b may move emission
// to before the action for DM approval routing.
//
// Validation follows the same minimal contract as EvaluatedPayload/IdlePayload:
// AgentID + Timestamp required. Domain-specific fields are always populated by
// the emit site inside execute functions, never by external callers.
// =============================================================================

// ClaimIntentPayload is emitted when an agent claims a quest.
type ClaimIntentPayload struct {
	AgentID        domain.AgentID `json:"agent_id"`
	QuestID        domain.QuestID `json:"quest_id"`
	Score          float64        `json:"score"`           // Boid suggestion score
	SuggestionRank int            `json:"suggestion_rank"` // 1-based rank in suggestion list
	Timestamp      time.Time      `json:"timestamp"`
	Trace          TraceInfo      `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *ClaimIntentPayload) EntityID() string { return string(p.AgentID) }

// Triples returns semantic triples for this event.
func (p *ClaimIntentPayload) Triples() []message.Triple {
	source := "autonomy"
	entityID := string(p.AgentID)
	return []message.Triple{
		{Subject: entityID, Predicate: "agent.autonomy.claim_quest", Object: string(p.QuestID), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.autonomy.claim_score", Object: p.Score, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.autonomy.claim_rank", Object: p.SuggestionRank, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

// Schema returns the type schema for this payload.
func (p *ClaimIntentPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "autonomy.claimintent", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *ClaimIntentPayload) Validate() error {
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// =============================================================================
// SHOP INTENT PAYLOAD
// =============================================================================

// ShopIntentPayload is emitted when an agent purchases an item.
type ShopIntentPayload struct {
	AgentID   domain.AgentID `json:"agent_id"`
	ItemID    string         `json:"item_id"`
	ItemName  string         `json:"item_name"`
	XPCost    int64          `json:"xp_cost"`
	Budget    int64          `json:"budget"`
	Strategic bool           `json:"strategic"` // true if mid-quest strategic purchase
	Timestamp time.Time      `json:"timestamp"`
	Trace     TraceInfo      `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *ShopIntentPayload) EntityID() string { return string(p.AgentID) }

// Triples returns semantic triples for this event.
func (p *ShopIntentPayload) Triples() []message.Triple {
	source := "autonomy"
	entityID := string(p.AgentID)
	return []message.Triple{
		{Subject: entityID, Predicate: "agent.autonomy.shop_item", Object: p.ItemID, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.autonomy.shop_cost", Object: p.XPCost, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.autonomy.shop_budget", Object: p.Budget, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.autonomy.shop_strategic", Object: p.Strategic, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

// Schema returns the type schema for this payload.
func (p *ShopIntentPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "autonomy.shopintent", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *ShopIntentPayload) Validate() error {
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// =============================================================================
// GUILD INTENT PAYLOAD
// =============================================================================

// GuildIntentPayload is emitted when an agent joins a guild.
type GuildIntentPayload struct {
	AgentID          domain.AgentID `json:"agent_id"`
	GuildID          string         `json:"guild_id"`
	GuildName        string         `json:"guild_name"`
	Score            float64        `json:"score"`
	ChoicesEvaluated int            `json:"choices_evaluated"`
	Timestamp        time.Time      `json:"timestamp"`
	Trace            TraceInfo      `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *GuildIntentPayload) EntityID() string { return string(p.AgentID) }

// Triples returns semantic triples for this event.
func (p *GuildIntentPayload) Triples() []message.Triple {
	source := "autonomy"
	entityID := string(p.AgentID)
	return []message.Triple{
		{Subject: entityID, Predicate: "agent.autonomy.guild_join", Object: p.GuildID, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.autonomy.guild_score", Object: p.Score, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.autonomy.guild_choices", Object: p.ChoicesEvaluated, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

// Schema returns the type schema for this payload.
func (p *GuildIntentPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "autonomy.guildintent", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *GuildIntentPayload) Validate() error {
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// =============================================================================
// USE INTENT PAYLOAD
// =============================================================================

// UseIntentPayload is emitted when an agent uses a consumable.
type UseIntentPayload struct {
	AgentID      domain.AgentID      `json:"agent_id"`
	ConsumableID string              `json:"consumable_id"`
	AgentStatus  domain.AgentStatus  `json:"agent_status"`
	QuestID      *domain.QuestID     `json:"quest_id,omitempty"` // Set when used during a quest
	Timestamp    time.Time           `json:"timestamp"`
	Trace        TraceInfo           `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *UseIntentPayload) EntityID() string { return string(p.AgentID) }

// Triples returns semantic triples for this event.
func (p *UseIntentPayload) Triples() []message.Triple {
	source := "autonomy"
	entityID := string(p.AgentID)
	triples := []message.Triple{
		{Subject: entityID, Predicate: "agent.autonomy.use_consumable", Object: p.ConsumableID, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.autonomy.use_status", Object: string(p.AgentStatus), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
	if p.QuestID != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "agent.autonomy.use_quest", Object: string(*p.QuestID), Source: source, Timestamp: p.Timestamp, Confidence: 1.0,
		})
	}
	return triples
}

// Schema returns the type schema for this payload.
func (p *UseIntentPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "autonomy.useintent", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *UseIntentPayload) Validate() error {
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}
