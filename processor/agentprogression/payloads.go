package agentprogression

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
// AGENT PROGRESSION PAYLOADS
// =============================================================================
// Event payloads for agent progression events. Each implements graph.Graphable
// for automatic persistence via graph-ingest.
// =============================================================================

// Ensure Graphable implementations
var (
	_ graph.Graphable = (*AgentXPPayload)(nil)
	_ graph.Graphable = (*AgentLevelUpPayload)(nil)
	_ graph.Graphable = (*AgentLevelDownPayload)(nil)
	_ graph.Graphable = (*AgentCooldownPayload)(nil)
	_ graph.Graphable = (*AgentDeathPayload)(nil)
	_ graph.Graphable = (*AgentReadyPayload)(nil)
)

// Typed subjects for agent progression events.
var (
	SubjectAgentXP        = natsclient.NewSubject[AgentXPPayload](domain.PredicateAgentXP)
	SubjectAgentLevelUp   = natsclient.NewSubject[AgentLevelUpPayload](domain.PredicateAgentLevelUp)
	SubjectAgentLevelDown = natsclient.NewSubject[AgentLevelDownPayload](domain.PredicateAgentLevelDown)
	SubjectAgentCooldown  = natsclient.NewSubject[AgentCooldownPayload](domain.PredicateAgentCooldown)
	SubjectAgentDeath     = natsclient.NewSubject[AgentDeathPayload](domain.PredicateAgentDeath)
	SubjectAgentReady     = natsclient.NewSubject[AgentReadyPayload](domain.PredicateAgentReady)
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

// =============================================================================
// AGENT LEVEL UP PAYLOAD
// =============================================================================

// AgentLevelUpPayload contains data for agent.progression.levelup events.
type AgentLevelUpPayload struct {
	AgentID   domain.AgentID   `json:"agent_id"`
	QuestID   domain.QuestID   `json:"quest_id"`
	OldLevel  int              `json:"old_level"`
	NewLevel  int              `json:"new_level"`
	OldTier   domain.TrustTier `json:"old_tier"`
	NewTier   domain.TrustTier `json:"new_tier"`
	XPCurrent int64            `json:"xp_current"`
	XPToLevel int64            `json:"xp_to_level"`
	Timestamp time.Time        `json:"timestamp"`
	Trace     TraceInfo        `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *AgentLevelUpPayload) EntityID() string { return string(p.AgentID) }

// Triples returns semantic triples for this event.
func (p *AgentLevelUpPayload) Triples() []message.Triple {
	source := "agent_progression"
	entityID := string(p.AgentID)

	triples := []message.Triple{
		{Subject: entityID, Predicate: "agent.progression.level", Object: p.NewLevel, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.progression.tier", Object: int(p.NewTier), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.progression.xp.current", Object: p.XPCurrent, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.progression.xp.to_level", Object: p.XPToLevel, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}

	// Record level change
	triples = append(triples, message.Triple{
		Subject: entityID, Predicate: "agent.progression.levelup.from", Object: p.OldLevel,
		Source: source, Timestamp: p.Timestamp, Confidence: 1.0,
	})

	// Record tier change if applicable
	if p.OldTier != p.NewTier {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "agent.progression.tier_change", Object: "promoted",
			Source: source, Timestamp: p.Timestamp, Confidence: 1.0,
		})
	}

	return triples
}

// Schema returns the type schema for this payload.
func (p *AgentLevelUpPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "agent.levelup", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *AgentLevelUpPayload) Validate() error {
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// =============================================================================
// AGENT LEVEL DOWN PAYLOAD
// =============================================================================

// AgentLevelDownPayload contains data for agent.progression.leveldown events.
type AgentLevelDownPayload struct {
	AgentID   domain.AgentID   `json:"agent_id"`
	QuestID   domain.QuestID   `json:"quest_id"`
	OldLevel  int              `json:"old_level"`
	NewLevel  int              `json:"new_level"`
	OldTier   domain.TrustTier `json:"old_tier"`
	NewTier   domain.TrustTier `json:"new_tier"`
	XPCurrent int64            `json:"xp_current"`
	XPToLevel int64            `json:"xp_to_level"`
	Reason    string           `json:"reason"`
	Timestamp time.Time        `json:"timestamp"`
	Trace     TraceInfo        `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *AgentLevelDownPayload) EntityID() string { return string(p.AgentID) }

// Triples returns semantic triples for this event.
func (p *AgentLevelDownPayload) Triples() []message.Triple {
	source := "agent_progression"
	entityID := string(p.AgentID)

	triples := []message.Triple{
		{Subject: entityID, Predicate: "agent.progression.level", Object: p.NewLevel, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.progression.tier", Object: int(p.NewTier), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.progression.xp.current", Object: p.XPCurrent, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.progression.xp.to_level", Object: p.XPToLevel, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}

	// Record level change
	triples = append(triples, message.Triple{
		Subject: entityID, Predicate: "agent.progression.leveldown.from", Object: p.OldLevel,
		Source: source, Timestamp: p.Timestamp, Confidence: 1.0,
	})

	// Record tier change if applicable
	if p.OldTier != p.NewTier {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "agent.progression.tier_change", Object: "demoted",
			Source: source, Timestamp: p.Timestamp, Confidence: 1.0,
		})
	}

	// Record reason
	if p.Reason != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "agent.progression.leveldown.reason", Object: p.Reason,
			Source: source, Timestamp: p.Timestamp, Confidence: 1.0,
		})
	}

	return triples
}

// Schema returns the type schema for this payload.
func (p *AgentLevelDownPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "agent.leveldown", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *AgentLevelDownPayload) Validate() error {
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// =============================================================================
// AGENT COOLDOWN PAYLOAD
// =============================================================================

// AgentCooldownPayload contains data for agent.progression.cooldown events.
type AgentCooldownPayload struct {
	AgentID       domain.AgentID `json:"agent_id"`
	QuestID       domain.QuestID `json:"quest_id"`
	FailType      FailureType    `json:"fail_type"`
	CooldownUntil time.Time      `json:"cooldown_until"`
	Duration      time.Duration  `json:"duration"`
	Timestamp     time.Time      `json:"timestamp"`
	Trace         TraceInfo      `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *AgentCooldownPayload) EntityID() string { return string(p.AgentID) }

// Triples returns semantic triples for this event.
func (p *AgentCooldownPayload) Triples() []message.Triple {
	source := "agent_progression"
	entityID := string(p.AgentID)

	return []message.Triple{
		{Subject: entityID, Predicate: "agent.status.state", Object: string(domain.AgentCooldown), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.status.cooldown_until", Object: p.CooldownUntil.Format(time.RFC3339), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.cooldown.duration", Object: p.Duration.String(), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.cooldown.fail_type", Object: string(p.FailType), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.cooldown.quest", Object: string(p.QuestID), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

// Schema returns the type schema for this payload.
func (p *AgentCooldownPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "agent.cooldown", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *AgentCooldownPayload) Validate() error {
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// =============================================================================
// AGENT DEATH PAYLOAD
// =============================================================================

// AgentDeathPayload contains data for agent.progression.death events.
type AgentDeathPayload struct {
	AgentID    domain.AgentID `json:"agent_id"`
	QuestID    domain.QuestID `json:"quest_id"`
	FinalLevel int            `json:"final_level"`
	FinalXP    int64          `json:"final_xp"`
	Cause      string         `json:"cause"`
	Timestamp  time.Time      `json:"timestamp"`
	Trace      TraceInfo      `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *AgentDeathPayload) EntityID() string { return string(p.AgentID) }

// Triples returns semantic triples for this event.
func (p *AgentDeathPayload) Triples() []message.Triple {
	source := "agent_progression"
	entityID := string(p.AgentID)

	return []message.Triple{
		{Subject: entityID, Predicate: "agent.status.state", Object: string(domain.AgentRetired), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.death.cause", Object: p.Cause, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.death.final_level", Object: p.FinalLevel, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.death.final_xp", Object: p.FinalXP, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.death.quest", Object: string(p.QuestID), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

// Schema returns the type schema for this payload.
func (p *AgentDeathPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "agent.death", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *AgentDeathPayload) Validate() error {
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// =============================================================================
// AGENT READY PAYLOAD
// =============================================================================

// AgentReadyPayload contains data for agent.progression.ready events.
type AgentReadyPayload struct {
	AgentID        domain.AgentID `json:"agent_id"`
	CooldownEnded  time.Time      `json:"cooldown_ended"`
	CooldownReason string         `json:"cooldown_reason,omitempty"`
	Timestamp      time.Time      `json:"timestamp"`
	Trace          TraceInfo      `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *AgentReadyPayload) EntityID() string { return string(p.AgentID) }

// Triples returns semantic triples for this event.
func (p *AgentReadyPayload) Triples() []message.Triple {
	source := "agent_progression"
	entityID := string(p.AgentID)

	return []message.Triple{
		{Subject: entityID, Predicate: "agent.status.state", Object: string(domain.AgentIdle), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

// Schema returns the type schema for this payload.
func (p *AgentReadyPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "agent.ready", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *AgentReadyPayload) Validate() error {
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}
