package guildformation

import (
	"errors"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/types"
)

// =============================================================================
// GUILD FORMATION PAYLOADS
// =============================================================================
// Event payloads for guild lifecycle and membership events.
// =============================================================================

// Ensure Graphable implementations
var (
	_ graph.Graphable = (*GuildCreatedPayload)(nil)
	_ graph.Graphable = (*GuildJoinedPayload)(nil)
	_ graph.Graphable = (*GuildLeftPayload)(nil)
	_ graph.Graphable = (*GuildPromotedPayload)(nil)
	_ graph.Graphable = (*GuildDisbandedPayload)(nil)
)

// Typed subjects for guild formation and membership events.
var (
	SubjectGuildCreated   = natsclient.NewSubject[GuildCreatedPayload](domain.PredicateGuildCreated)
	SubjectGuildJoined    = natsclient.NewSubject[GuildJoinedPayload](domain.PredicateGuildJoined)
	SubjectGuildLeft      = natsclient.NewSubject[GuildLeftPayload](domain.PredicateGuildLeft)
	SubjectGuildPromoted  = natsclient.NewSubject[GuildPromotedPayload](domain.PredicateGuildPromoted)
	SubjectGuildDisbanded = natsclient.NewSubject[GuildDisbandedPayload]("guild.lifecycle.disbanded")
)

// --- TraceInfo for observability ---

// TraceInfo contains trace context for observability.
type TraceInfo struct {
	TrajectoryID string `json:"trajectory_id,omitempty"`
	SpanID       string `json:"span_id,omitempty"`
	ParentSpanID string `json:"parent_span_id,omitempty"`
}

// =============================================================================
// GUILD CREATED PAYLOAD
// =============================================================================

// GuildCreatedPayload is emitted when a new guild is formed.
type GuildCreatedPayload struct {
	Guild     semdragons.Guild `json:"guild"`
	FounderID domain.AgentID   `json:"founder_id"`
	Timestamp time.Time        `json:"timestamp"`
	Trace     TraceInfo        `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *GuildCreatedPayload) EntityID() string {
	return string(p.Guild.ID)
}

// Triples returns semantic facts about this event.
func (p *GuildCreatedPayload) Triples() []message.Triple {
	triples := p.Guild.Triples()
	triples = append(triples, message.Triple{
		Subject:    p.EntityID(),
		Predicate:  "guild.event.created_by",
		Object:     string(p.FounderID),
		Source:     "guildformation",
		Timestamp:  p.Timestamp,
		Confidence: 1.0,
	})
	return triples
}

// Schema returns the type schema for this payload.
func (p *GuildCreatedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "guild.created", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *GuildCreatedPayload) Validate() error {
	if p.Guild.ID == "" {
		return errors.New("guild_id required")
	}
	if p.FounderID == "" {
		return errors.New("founder_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// =============================================================================
// GUILD JOINED PAYLOAD
// =============================================================================

// GuildJoinedPayload is emitted when an agent joins a guild.
type GuildJoinedPayload struct {
	GuildID   domain.GuildID   `json:"guild_id"`
	GuildName string           `json:"guild_name"`
	AgentID   domain.AgentID   `json:"agent_id"`
	Rank      domain.GuildRank `json:"rank"`
	Timestamp time.Time        `json:"timestamp"`
	Trace     TraceInfo        `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *GuildJoinedPayload) EntityID() string {
	return string(p.GuildID)
}

// Triples returns semantic facts about this event.
func (p *GuildJoinedPayload) Triples() []message.Triple {
	return []message.Triple{
		{
			Subject:    string(p.GuildID),
			Predicate:  "guild.membership.member",
			Object:     string(p.AgentID),
			Source:     "guildformation",
			Timestamp:  p.Timestamp,
			Confidence: 1.0,
		},
		{
			Subject:    string(p.GuildID),
			Predicate:  "guild.member." + string(p.AgentID) + ".rank",
			Object:     string(p.Rank),
			Source:     "guildformation",
			Timestamp:  p.Timestamp,
			Confidence: 1.0,
		},
	}
}

// Schema returns the type schema for this payload.
func (p *GuildJoinedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "guild.joined", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *GuildJoinedPayload) Validate() error {
	if p.GuildID == "" {
		return errors.New("guild_id required")
	}
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	return nil
}

// =============================================================================
// GUILD LEFT PAYLOAD
// =============================================================================

// GuildLeftPayload is emitted when an agent leaves a guild.
type GuildLeftPayload struct {
	GuildID   domain.GuildID `json:"guild_id"`
	GuildName string         `json:"guild_name"`
	AgentID   domain.AgentID `json:"agent_id"`
	Reason    string         `json:"reason"`
	Timestamp time.Time      `json:"timestamp"`
	Trace     TraceInfo      `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *GuildLeftPayload) EntityID() string {
	return string(p.GuildID)
}

// Triples returns semantic facts about this event.
func (p *GuildLeftPayload) Triples() []message.Triple {
	return []message.Triple{
		{
			Subject:    string(p.GuildID),
			Predicate:  "guild.membership.left",
			Object:     string(p.AgentID),
			Source:     "guildformation",
			Timestamp:  p.Timestamp,
			Confidence: 1.0,
		},
	}
}

// Schema returns the type schema for this payload.
func (p *GuildLeftPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "guild.left", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *GuildLeftPayload) Validate() error {
	if p.GuildID == "" {
		return errors.New("guild_id required")
	}
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	return nil
}

// =============================================================================
// GUILD PROMOTED PAYLOAD
// =============================================================================

// GuildPromotedPayload is emitted when a member is promoted.
type GuildPromotedPayload struct {
	GuildID   domain.GuildID   `json:"guild_id"`
	GuildName string           `json:"guild_name"`
	AgentID   domain.AgentID   `json:"agent_id"`
	OldRank   domain.GuildRank `json:"old_rank"`
	NewRank   domain.GuildRank `json:"new_rank"`
	Timestamp time.Time        `json:"timestamp"`
	Trace     TraceInfo        `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *GuildPromotedPayload) EntityID() string {
	return string(p.GuildID)
}

// Triples returns semantic facts about this event.
func (p *GuildPromotedPayload) Triples() []message.Triple {
	return []message.Triple{
		{
			Subject:    string(p.GuildID),
			Predicate:  "guild.member." + string(p.AgentID) + ".rank",
			Object:     string(p.NewRank),
			Source:     "guildformation",
			Timestamp:  p.Timestamp,
			Confidence: 1.0,
		},
	}
}

// Schema returns the type schema for this payload.
func (p *GuildPromotedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "guild.promoted", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *GuildPromotedPayload) Validate() error {
	if p.GuildID == "" {
		return errors.New("guild_id required")
	}
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	return nil
}

// =============================================================================
// GUILD DISBANDED PAYLOAD
// =============================================================================

// GuildDisbandedPayload is emitted when a guild is disbanded.
type GuildDisbandedPayload struct {
	GuildID          domain.GuildID `json:"guild_id"`
	GuildName        string         `json:"guild_name"`
	Reason           string         `json:"reason"`
	FinalMemberCount int            `json:"final_member_count"`
	Timestamp        time.Time      `json:"timestamp"`
	Trace            TraceInfo      `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *GuildDisbandedPayload) EntityID() string {
	return string(p.GuildID)
}

// Triples returns semantic facts about this event.
func (p *GuildDisbandedPayload) Triples() []message.Triple {
	return []message.Triple{
		{
			Subject:    string(p.GuildID),
			Predicate:  "guild.status.state",
			Object:     string(domain.GuildInactive),
			Source:     "guildformation",
			Timestamp:  p.Timestamp,
			Confidence: 1.0,
		},
		{
			Subject:    string(p.GuildID),
			Predicate:  "guild.lifecycle.disbanded_at",
			Object:     p.Timestamp.Format(time.RFC3339),
			Source:     "guildformation",
			Timestamp:  p.Timestamp,
			Confidence: 1.0,
		},
	}
}

// Schema returns the type schema for this payload.
func (p *GuildDisbandedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "guild.disbanded", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *GuildDisbandedPayload) Validate() error {
	if p.GuildID == "" {
		return errors.New("guild_id required")
	}
	return nil
}
