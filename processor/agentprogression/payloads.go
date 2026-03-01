package agentprogression

import (
	"errors"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/pkg/types"
)

// =============================================================================
// AGENT PROGRESSION PAYLOADS
// =============================================================================
// Event payloads for agent progression events. Each implements graph.Graphable
// for automatic persistence via graph-ingest.
// =============================================================================

// Ensure Graphable implementations
var (
	_ graph.Graphable = (*AgentXPPayload)(nil)
)

// --- TraceInfo for observability ---

// TraceInfo contains trace context for observability.
type TraceInfo struct {
	TrajectoryID string `json:"trajectory_id,omitempty"`
	SpanID       string `json:"span_id,omitempty"`
	ParentSpanID string `json:"parent_span_id,omitempty"`
}

// =============================================================================
// XP CALCULATION TYPES
// =============================================================================

// XPAward holds the breakdown of XP earned from a quest completion.
type XPAward struct {
	BaseXP         int64  `json:"base_xp"`
	QualityBonus   int64  `json:"quality_bonus"`
	SpeedBonus     int64  `json:"speed_bonus"`
	StreakBonus    int64  `json:"streak_bonus"`
	GuildBonus     int64  `json:"guild_bonus"`
	AttemptPenalty int64  `json:"attempt_penalty"`
	TotalXP        int64  `json:"total_xp"`
	Breakdown      string `json:"breakdown"`
}

// XPPenalty holds the consequences of a quest failure.
type XPPenalty struct {
	XPLost      int64         `json:"xp_lost"`
	CooldownDur time.Duration `json:"cooldown_duration"`
	LevelLoss   bool          `json:"level_loss"`
	Permadeath  bool          `json:"permadeath"`
	Reason      string        `json:"reason"`
}

// FailureType categorizes quest failures for penalty calculation.
type FailureType string

const (
	// FailureSoft indicates bad output that can be retried.
	FailureSoft FailureType = "soft"
	// FailureTimeout indicates the quest took too long.
	FailureTimeout FailureType = "timeout"
	// FailureAbandon indicates the agent gave up.
	FailureAbandon FailureType = "abandon"
	// FailureCatastrophic indicates data loss, security breach, etc.
	FailureCatastrophic FailureType = "catastrophic"
)

// =============================================================================
// AGENT XP PAYLOAD
// =============================================================================

// AgentXPPayload contains data for agent.progression.xp events.
// Used as a Graphable entity to emit agent XP state via PutEntityState.
type AgentXPPayload struct {
	AgentID     domain.AgentID `json:"agent_id"`
	QuestID     domain.QuestID `json:"quest_id"`
	Award       *XPAward       `json:"award,omitempty"`   // Set on success
	Penalty     *XPPenalty     `json:"penalty,omitempty"` // Set on failure
	XPDelta     int64          `json:"xp_delta"`
	XPBefore    int64          `json:"xp_before"`
	XPAfter     int64          `json:"xp_after"`
	LevelBefore int            `json:"level_before"`
	LevelAfter  int            `json:"level_after"`
	Timestamp   time.Time      `json:"timestamp"`
	Trace       TraceInfo      `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *AgentXPPayload) EntityID() string { return string(p.AgentID) }

// Triples returns semantic triples for this event.
func (p *AgentXPPayload) Triples() []message.Triple {
	source := "agent_progression"
	entityID := string(p.AgentID)

	triples := []message.Triple{
		{Subject: entityID, Predicate: "agent.progression.xp.delta", Object: p.XPDelta, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.progression.xp.current", Object: p.XPAfter, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.progression.level", Object: p.LevelAfter, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}

	// Link to quest that triggered this XP change
	if p.QuestID != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "agent.progression.xp.quest", Object: string(p.QuestID),
			Source: source, Timestamp: p.Timestamp, Confidence: 1.0,
		})
	}

	// Add award breakdown if present
	if p.Award != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "agent.progression.xp.awarded", Object: p.Award.TotalXP,
			Source: source, Timestamp: p.Timestamp, Confidence: 1.0,
		})
	}

	// Add penalty info if present
	if p.Penalty != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "agent.progression.xp.penalty", Object: p.Penalty.XPLost,
			Source: source, Timestamp: p.Timestamp, Confidence: 1.0,
		})
	}

	return triples
}

// Schema returns the type schema for this payload.
func (p *AgentXPPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "agent.xp", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *AgentXPPayload) Validate() error {
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.QuestID == "" {
		return errors.New("quest_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}
