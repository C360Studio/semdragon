package guildformation

import (
	"errors"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/natsclient"
)

// =============================================================================
// GUILD FORMATION PAYLOADS
// =============================================================================
// Event payloads for guild lifecycle and membership events.
// =============================================================================

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
	Guild     domain.Guild   `json:"guild"`
	FounderID domain.AgentID `json:"founder_id"`
	Timestamp time.Time      `json:"timestamp"`
	Trace     TraceInfo      `json:"trace,omitempty"`
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

// Validate checks the payload for required fields.
func (p *GuildDisbandedPayload) Validate() error {
	if p.GuildID == "" {
		return errors.New("guild_id required")
	}
	return nil
}
