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
	_ graph.Graphable = (*GuildCreateIntentPayload)(nil)
)

// Typed subjects for autonomy events.
var (
	SubjectAutonomyEvaluated         = natsclient.NewSubject[EvaluatedPayload](domain.PredicateAutonomyEvaluated)
	SubjectAutonomyIdle              = natsclient.NewSubject[IdlePayload](domain.PredicateAutonomyIdle)
	SubjectAutonomyClaimIntent       = natsclient.NewSubject[ClaimIntentPayload](domain.PredicateAutonomyClaimIntent)
	SubjectAutonomyShopIntent        = natsclient.NewSubject[ShopIntentPayload](domain.PredicateAutonomyShopIntent)
	SubjectAutonomyGuildIntent       = natsclient.NewSubject[GuildIntentPayload](domain.PredicateAutonomyGuildIntent)
	SubjectAutonomyUseIntent         = natsclient.NewSubject[UseIntentPayload](domain.PredicateAutonomyUseIntent)
	SubjectAutonomyGuildCreateIntent = natsclient.NewSubject[GuildCreateIntentPayload](domain.PredicateGuildProposed)
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
		{Subject: entityID, Predicate: domain.PredicateAutonomyStatus, Object: string(p.AgentStatus), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: domain.PredicateAutonomyAction, Object: p.ActionTaken, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: domain.PredicateAutonomyIntervalMS, Object: p.Interval.Milliseconds(), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
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
		{Subject: entityID, Predicate: domain.PredicateAutonomyIdleDurationMS, Object: p.IdleDuration.Milliseconds(), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: domain.PredicateAutonomyHasSuggestion, Object: p.HasSuggestion, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: domain.PredicateAutonomyBackoffMS, Object: p.BackoffMs, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
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
		{Subject: entityID, Predicate: domain.PredicateAutonomyClaimQuest, Object: string(p.QuestID), Source: source, Timestamp: p.Timestamp, Confidence: 1.0, Datatype: message.EntityReferenceDatatype},
		{Subject: entityID, Predicate: domain.PredicateAutonomyClaimScore, Object: p.Score, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: domain.PredicateAutonomyClaimRank, Object: p.SuggestionRank, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
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
		{Subject: entityID, Predicate: domain.PredicateAutonomyShopItem, Object: p.ItemID, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: domain.PredicateAutonomyShopCost, Object: p.XPCost, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: domain.PredicateAutonomyShopBudget, Object: p.Budget, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: domain.PredicateAutonomyShopStrategic, Object: p.Strategic, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
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
		{Subject: entityID, Predicate: domain.PredicateAutonomyGuildJoin, Object: p.GuildID, Source: source, Timestamp: p.Timestamp, Confidence: 1.0, Datatype: message.EntityReferenceDatatype},
		{Subject: entityID, Predicate: domain.PredicateAutonomyGuildScore, Object: p.Score, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: domain.PredicateAutonomyGuildChoices, Object: p.ChoicesEvaluated, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
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
	AgentID      domain.AgentID     `json:"agent_id"`
	ConsumableID string             `json:"consumable_id"`
	AgentStatus  domain.AgentStatus `json:"agent_status"`
	QuestID      *domain.QuestID    `json:"quest_id,omitempty"` // Set when used during a quest
	Timestamp    time.Time          `json:"timestamp"`
	Trace        TraceInfo          `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *UseIntentPayload) EntityID() string { return string(p.AgentID) }

// Triples returns semantic triples for this event.
func (p *UseIntentPayload) Triples() []message.Triple {
	source := "autonomy"
	entityID := string(p.AgentID)
	triples := []message.Triple{
		{Subject: entityID, Predicate: domain.PredicateAutonomyUseConsumable, Object: p.ConsumableID, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: domain.PredicateAutonomyUseStatus, Object: string(p.AgentStatus), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
	if p.QuestID != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: domain.PredicateAutonomyUseQuest, Object: string(*p.QuestID), Source: source, Timestamp: p.Timestamp, Confidence: 1.0, Datatype: message.EntityReferenceDatatype,
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

// =============================================================================
// GUILD CREATE INTENT PAYLOAD
// =============================================================================

// GuildCreateIntentPayload is emitted when an agent proposes founding a guild.
type GuildCreateIntentPayload struct {
	AgentID          domain.AgentID     `json:"agent_id"`
	GuildName        string             `json:"guild_name"`
	CandidateIDs     []domain.AgentID   `json:"candidate_ids"`
	FellowshipScores map[string]float64 `json:"fellowship_scores"` // agentID → score
	Timestamp        time.Time          `json:"timestamp"`
	Trace            TraceInfo          `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *GuildCreateIntentPayload) EntityID() string { return string(p.AgentID) }

// Triples returns semantic triples for this event.
func (p *GuildCreateIntentPayload) Triples() []message.Triple {
	source := "autonomy"
	entityID := string(p.AgentID)
	return []message.Triple{
		{Subject: entityID, Predicate: domain.PredicateAutonomyGuildCreate, Object: p.GuildName, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: domain.PredicateAutonomyGuildCreateCandidates, Object: len(p.CandidateIDs), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

// Schema returns the type schema for this payload.
func (p *GuildCreateIntentPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "autonomy.guildcreateintent", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *GuildCreateIntentPayload) Validate() error {
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.GuildName == "" {
		return errors.New("guild_name required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}
