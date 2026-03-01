package seeding

import (
	"errors"
	"fmt"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/types"
)

// =============================================================================
// SEEDING PAYLOADS
// =============================================================================
// Event payloads for seeding operations.
// =============================================================================

// Ensure Graphable implementations
var (
	_ graph.Graphable = (*SeedingStartedPayload)(nil)
	_ graph.Graphable = (*SeedingCompletedPayload)(nil)
	_ graph.Graphable = (*AgentSeededPayload)(nil)
	_ graph.Graphable = (*GuildSeededPayload)(nil)
)

// --- Typed Subjects ---

var (
	SubjectSeedingStarted   = natsclient.NewSubject[SeedingStartedPayload]("seeding.lifecycle.started")
	SubjectSeedingCompleted = natsclient.NewSubject[SeedingCompletedPayload]("seeding.lifecycle.completed")
	SubjectAgentSeeded      = natsclient.NewSubject[AgentSeededPayload]("seeding.agent.created")
	SubjectGuildSeeded      = natsclient.NewSubject[GuildSeededPayload]("seeding.guild.created")
)

// --- TraceInfo for observability ---

// TraceInfo contains trace context for observability.
type TraceInfo struct {
	TrajectoryID string `json:"trajectory_id,omitempty"`
	SpanID       string `json:"span_id,omitempty"`
	ParentSpanID string `json:"parent_span_id,omitempty"`
}

// =============================================================================
// SEEDING STARTED PAYLOAD
// =============================================================================

// SeedingStartedPayload is emitted when a seeding operation begins.
type SeedingStartedPayload struct {
	SessionID string    `json:"session_id"`
	Mode      Mode      `json:"mode"`
	DryRun    bool      `json:"dry_run"`
	Timestamp time.Time `json:"timestamp"`
	Trace     TraceInfo `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *SeedingStartedPayload) EntityID() string {
	return "seeding." + p.SessionID
}

// Triples returns semantic facts about this event.
func (p *SeedingStartedPayload) Triples() []message.Triple {
	return []message.Triple{
		{
			Subject:    p.EntityID(),
			Predicate:  "seeding.lifecycle.started",
			Object:     p.Timestamp.Format(time.RFC3339),
			Source:     "seeding",
			Timestamp:  p.Timestamp,
			Confidence: 1.0,
		},
		{
			Subject:    p.EntityID(),
			Predicate:  "seeding.mode",
			Object:     string(p.Mode),
			Source:     "seeding",
			Timestamp:  p.Timestamp,
			Confidence: 1.0,
		},
	}
}

// Schema returns the type schema for this payload.
func (p *SeedingStartedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "seeding.started", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *SeedingStartedPayload) Validate() error {
	if p.SessionID == "" {
		return errors.New("session_id required")
	}
	return nil
}

// =============================================================================
// SEEDING COMPLETED PAYLOAD
// =============================================================================

// SeedingCompletedPayload is emitted when a seeding operation completes.
type SeedingCompletedPayload struct {
	SessionID     string        `json:"session_id"`
	Mode          Mode          `json:"mode"`
	Success       bool          `json:"success"`
	AgentsCreated int           `json:"agents_created"`
	GuildsCreated int           `json:"guilds_created"`
	Duration      time.Duration `json:"duration"`
	Errors        []string      `json:"errors,omitempty"`
	Timestamp     time.Time     `json:"timestamp"`
	Trace         TraceInfo     `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *SeedingCompletedPayload) EntityID() string {
	return "seeding." + p.SessionID
}

// Triples returns semantic facts about this event.
func (p *SeedingCompletedPayload) Triples() []message.Triple {
	return []message.Triple{
		{
			Subject:    p.EntityID(),
			Predicate:  "seeding.lifecycle.completed",
			Object:     p.Timestamp.Format(time.RFC3339),
			Source:     "seeding",
			Timestamp:  p.Timestamp,
			Confidence: 1.0,
		},
		{
			Subject:    p.EntityID(),
			Predicate:  "seeding.stats.agents_created",
			Object:     p.AgentsCreated,
			Source:     "seeding",
			Timestamp:  p.Timestamp,
			Confidence: 1.0,
		},
		{
			Subject:    p.EntityID(),
			Predicate:  "seeding.stats.guilds_created",
			Object:     p.GuildsCreated,
			Source:     "seeding",
			Timestamp:  p.Timestamp,
			Confidence: 1.0,
		},
	}
}

// Schema returns the type schema for this payload.
func (p *SeedingCompletedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "seeding.completed", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *SeedingCompletedPayload) Validate() error {
	if p.SessionID == "" {
		return errors.New("session_id required")
	}
	return nil
}

// =============================================================================
// AGENT SEEDED PAYLOAD
// =============================================================================

// AgentSeededPayload is emitted when an agent is seeded.
type AgentSeededPayload struct {
	AgentID   domain.AgentID    `json:"agent_id"`
	AgentName string            `json:"agent_name"`
	Level     int               `json:"level"`
	Tier      domain.TrustTier  `json:"tier"`
	Skills    []domain.SkillTag `json:"skills"`
	GuildID   domain.GuildID    `json:"guild_id,omitempty"`
	IsNPC     bool              `json:"is_npc"`
	SessionID string            `json:"session_id"`
	Timestamp time.Time         `json:"timestamp"`
	Trace     TraceInfo         `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *AgentSeededPayload) EntityID() string {
	return string(p.AgentID)
}

// Triples returns semantic facts about this event.
func (p *AgentSeededPayload) Triples() []message.Triple {
	triples := []message.Triple{
		{
			Subject:    p.EntityID(),
			Predicate:  "agent.identity.name",
			Object:     p.AgentName,
			Source:     "seeding",
			Timestamp:  p.Timestamp,
			Confidence: 1.0,
		},
		{
			Subject:    p.EntityID(),
			Predicate:  "agent.progression.level",
			Object:     p.Level,
			Source:     "seeding",
			Timestamp:  p.Timestamp,
			Confidence: 1.0,
		},
		{
			Subject:    p.EntityID(),
			Predicate:  "agent.progression.tier",
			Object:     fmt.Sprintf("%d", p.Tier),
			Source:     "seeding",
			Timestamp:  p.Timestamp,
			Confidence: 1.0,
		},
	}

	for _, skill := range p.Skills {
		triples = append(triples, message.Triple{
			Subject:    p.EntityID(),
			Predicate:  "agent.skills.has",
			Object:     string(skill),
			Source:     "seeding",
			Timestamp:  p.Timestamp,
			Confidence: 1.0,
		})
	}

	if p.GuildID != "" {
		triples = append(triples, message.Triple{
			Subject:    p.EntityID(),
			Predicate:  "agent.guild.member",
			Object:     string(p.GuildID),
			Source:     "seeding",
			Timestamp:  p.Timestamp,
			Confidence: 1.0,
		})
	}

	return triples
}

// Schema returns the type schema for this payload.
func (p *AgentSeededPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "seeding.agent", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *AgentSeededPayload) Validate() error {
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	return nil
}

// =============================================================================
// GUILD SEEDED PAYLOAD
// =============================================================================

// GuildSeededPayload is emitted when a guild is seeded.
type GuildSeededPayload struct {
	GuildID         domain.GuildID    `json:"guild_id"`
	GuildName       string            `json:"guild_name"`
	Description     string            `json:"description"`
	Specializations []domain.SkillTag `json:"specializations"`
	SessionID       string            `json:"session_id"`
	Timestamp       time.Time         `json:"timestamp"`
	Trace           TraceInfo         `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *GuildSeededPayload) EntityID() string {
	return string(p.GuildID)
}

// Triples returns semantic facts about this event.
func (p *GuildSeededPayload) Triples() []message.Triple {
	triples := []message.Triple{
		{
			Subject:    p.EntityID(),
			Predicate:  "guild.identity.name",
			Object:     p.GuildName,
			Source:     "seeding",
			Timestamp:  p.Timestamp,
			Confidence: 1.0,
		},
		{
			Subject:    p.EntityID(),
			Predicate:  "guild.status.state",
			Object:     string(domain.GuildActive),
			Source:     "seeding",
			Timestamp:  p.Timestamp,
			Confidence: 1.0,
		},
	}

	for _, skill := range p.Specializations {
		triples = append(triples, message.Triple{
			Subject:    p.EntityID(),
			Predicate:  "guild.specialization",
			Object:     string(skill),
			Source:     "seeding",
			Timestamp:  p.Timestamp,
			Confidence: 1.0,
		})
	}

	return triples
}

// Schema returns the type schema for this payload.
func (p *GuildSeededPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "seeding.guild", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *GuildSeededPayload) Validate() error {
	if p.GuildID == "" {
		return errors.New("guild_id required")
	}
	return nil
}
