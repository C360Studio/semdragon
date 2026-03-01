package guildformation

import (
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/message"
)

// =============================================================================
// GUILD - A permanent specialization group for agents
// =============================================================================
// Guild is the core entity owned by the guildformation processor.
// It implements graph.Graphable for persistence in the semstreams graph system.
// =============================================================================

// Guild is a permanent organization of agents specializing in related skills.
// Members gain XP bonuses on quests matching guild specialization.
type Guild struct {
	ID          domain.GuildID   `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Status      domain.GuildStatus `json:"status"`

	// Specialization
	Specializations []domain.SkillTag `json:"specializations"` // Skills this guild focuses on
	FocusArea       string            `json:"focus_area"`      // General area of expertise

	// Leadership
	Guildmaster domain.AgentID `json:"guildmaster"`
	Officers    []domain.AgentID `json:"officers,omitempty"`

	// Membership
	Members []GuildMember `json:"members"`

	// Stats
	QuestsCompleted int64   `json:"quests_completed"`
	TotalXPEarned   int64   `json:"total_xp_earned"`
	Reputation      float64 `json:"reputation"` // 0.0 to 1.0

	// Lifecycle
	FoundedAt   time.Time  `json:"founded_at"`
	DisbandedAt *time.Time `json:"disbanded_at,omitempty"`
}

// GuildMember represents an agent's membership in a guild.
type GuildMember struct {
	AgentID   domain.AgentID    `json:"agent_id"`
	Rank      domain.GuildRank  `json:"rank"`
	JoinedAt  time.Time         `json:"joined_at"`
	QuestsFor int               `json:"quests_for"` // Quests completed for this guild
	XPEarned  int64             `json:"xp_earned"`  // XP earned while in guild
}

// =============================================================================
// GUILD METHODS
// =============================================================================

// GetMember returns a member by agent ID, or nil if not a member.
func (g *Guild) GetMember(agentID domain.AgentID) *GuildMember {
	for i := range g.Members {
		if g.Members[i].AgentID == agentID {
			return &g.Members[i]
		}
	}
	return nil
}

// IsMember checks if an agent is a member of this guild.
func (g *Guild) IsMember(agentID domain.AgentID) bool {
	return g.GetMember(agentID) != nil
}

// IsOfficer checks if an agent is an officer of this guild.
func (g *Guild) IsOfficer(agentID domain.AgentID) bool {
	for _, officer := range g.Officers {
		if officer == agentID {
			return true
		}
	}
	return agentID == g.Guildmaster
}

// MemberCount returns the number of members.
func (g *Guild) MemberCount() int {
	return len(g.Members)
}

// HasSpecialization checks if the guild specializes in a skill.
func (g *Guild) HasSpecialization(skill domain.SkillTag) bool {
	for _, s := range g.Specializations {
		if s == skill {
			return true
		}
	}
	return false
}

// =============================================================================
// GRAPHABLE IMPLEMENTATION
// =============================================================================

// EntityID returns the 6-part entity ID for this guild.
func (g *Guild) EntityID() string {
	return string(g.ID)
}

// Triples returns all semantic facts about this guild.
func (g *Guild) Triples() []message.Triple {
	now := time.Now()
	source := "guildformation"
	entityID := g.EntityID()

	triples := []message.Triple{
		// Identity
		{Subject: entityID, Predicate: "guild.identity.name", Object: g.Name, Source: source, Timestamp: now, Confidence: 1.0},

		// Status
		{Subject: entityID, Predicate: "guild.status.state", Object: string(g.Status), Source: source, Timestamp: now, Confidence: 1.0},

		// Leadership
		{Subject: entityID, Predicate: "guild.leadership.guildmaster", Object: string(g.Guildmaster), Source: source, Timestamp: now, Confidence: 1.0},

		// Membership
		{Subject: entityID, Predicate: "guild.membership.count", Object: len(g.Members), Source: source, Timestamp: now, Confidence: 1.0},

		// Stats
		{Subject: entityID, Predicate: "guild.stats.quests_completed", Object: g.QuestsCompleted, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "guild.stats.total_xp", Object: g.TotalXPEarned, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "guild.stats.reputation", Object: g.Reputation, Source: source, Timestamp: now, Confidence: 1.0},

		// Lifecycle
		{Subject: entityID, Predicate: "guild.lifecycle.founded_at", Object: g.FoundedAt.Format(time.RFC3339), Source: source, Timestamp: now, Confidence: 1.0},
	}

	// Description
	if g.Description != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "guild.identity.description", Object: g.Description,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Focus area
	if g.FocusArea != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "guild.focus_area", Object: g.FocusArea,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Specializations
	for _, skill := range g.Specializations {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "guild.specialization", Object: string(skill),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Officers
	for _, officer := range g.Officers {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "guild.leadership.officer", Object: string(officer),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Members
	for _, member := range g.Members {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "guild.membership.member", Object: string(member.AgentID),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "guild.member." + string(member.AgentID) + ".rank", Object: string(member.Rank),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Disbanded time if set
	if g.DisbandedAt != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "guild.lifecycle.disbanded_at", Object: g.DisbandedAt.Format(time.RFC3339),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	return triples
}
