package domain

import "time"

// =============================================================================
// GUILD
// =============================================================================

// Guild is a persistent social organization of agents with mixed composition.
// Guilds provide: party access, quest routing, shared knowledge, reputation, and mentorship.
// Natural diversity pressure: quests require diverse skills, so homogeneous guilds fail.
type Guild struct {
	ID          GuildID     `json:"id"`
	Name        string      `json:"name"` // "Dragon Slayers", "Code Crafters", etc.
	Description string      `json:"description"`
	Status      GuildStatus `json:"status"`

	// Social organization (mixed skills/approaches)
	Members    []GuildMember `json:"members"`
	MaxMembers int           `json:"max_members"`
	MinLevel   int           `json:"min_level"` // Minimum agent level to join

	// Founding
	Founded   time.Time `json:"founded"`
	FoundedBy AgentID   `json:"founded_by"`

	// Guild culture and identity
	Culture string `json:"culture"` // "We ship quality code"
	Motto   string `json:"motto,omitempty"`

	// Earned through collective quest success
	Reputation    float64 `json:"reputation"` // 0.0-1.0, affects quest priority
	QuestsHandled int     `json:"quests_handled"`
	SuccessRate   float64 `json:"success_rate"` // Completed / (Completed + Failed)
	QuestsFailed  int     `json:"quests_failed"`

	// Guild Hall - shared knowledge and resources
	SharedTools []string `json:"shared_tools"` // Tool IDs available to members

	// Quest routing (clients trust certain guilds)
	QuestTypes       []string `json:"quest_types,omitempty"` // Types of quests they handle
	PreferredClients []string `json:"preferred_clients,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

// GuildMember represents an agent's membership in a guild.
type GuildMember struct {
	AgentID      AgentID   `json:"agent_id"`
	Rank         GuildRank `json:"rank"`
	JoinedAt     time.Time `json:"joined_at"`
	Contribution float64   `json:"contribution"` // XP contributed via guild quests
}
