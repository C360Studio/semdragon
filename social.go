package semdragons

import (
	"time"
)

// =============================================================================
// PARTY - A temporary group formed to tackle a quest
// =============================================================================

// PartyStatus represents the current state of a party.
type PartyStatus string

const (
	// PartyForming indicates the party is recruiting members.
	PartyForming PartyStatus = "forming"
	// PartyActive indicates the party is on a quest.
	PartyActive PartyStatus = "active"
	// PartyDisbanded indicates the quest is complete or failed.
	PartyDisbanded PartyStatus = "disbanded"
)

// PartyRole represents a member's role within a party.
type PartyRole string

const (
	// RoleLead decomposes quest, coordinates, and faces the boss.
	RoleLead PartyRole = "lead"
	// RoleExecutor does the actual sub-quest work.
	RoleExecutor PartyRole = "executor"
	// RoleReviewer provides internal QA before boss battle.
	RoleReviewer PartyRole = "reviewer"
	// RoleScout handles research, context gathering, and recon.
	RoleScout PartyRole = "scout"
)

// Party is a temporary group of agents formed to tackle a quest together.
// The party lead is responsible for decomposing the quest and rolling up results.
type Party struct {
	ID      PartyID     `json:"id"`
	Name    string      `json:"name"` // Auto-generated or lead-chosen
	Status  PartyStatus `json:"status"`
	QuestID QuestID     `json:"quest_id"` // The quest this party was formed for

	// Composition
	Lead    AgentID       `json:"lead"`
	Members []PartyMember `json:"members"`

	// Coordination
	Strategy      string              `json:"strategy"`       // The lead's plan of attack
	SubQuestMap   map[QuestID]AgentID `json:"sub_quest_map"`  // Who's doing what
	SharedContext []ContextItem       `json:"shared_context"` // Party-wide knowledge

	// Results
	SubResults   map[QuestID]any `json:"sub_results,omitempty"`   // Collected sub-quest outputs
	RollupResult any             `json:"rollup_result,omitempty"` // Lead's combined result

	FormedAt    time.Time  `json:"formed_at"`
	DisbandedAt *time.Time `json:"disbanded_at,omitempty"`
}

// PartyMember represents an agent's membership in a party.
type PartyMember struct {
	AgentID  AgentID    `json:"agent_id"`
	Role     PartyRole  `json:"role"`
	Skills   []SkillTag `json:"skills"` // Why they were recruited
	JoinedAt time.Time  `json:"joined_at"`
}

// ContextItem represents a piece of shared knowledge in a party.
type ContextItem struct {
	Key     string    `json:"key"`
	Value   any       `json:"value"`
	AddedBy AgentID   `json:"added_by"`
	AddedAt time.Time `json:"added_at"`
}

// =============================================================================
// GUILD - Persistent specialization clusters
// =============================================================================

// GuildStatus represents the current state of a guild.
type GuildStatus string

const (
	// GuildActive indicates the guild is actively accepting quests.
	GuildActive GuildStatus = "active"
	// GuildInactive indicates the guild is not accepting quests.
	GuildInactive GuildStatus = "inactive"
)

// GuildRank represents a member's rank within a guild.
type GuildRank string

const (
	// GuildRankInitiate indicates a new member proving themselves.
	GuildRankInitiate GuildRank = "initiate"
	// GuildRankMember indicates an established contributor.
	GuildRankMember GuildRank = "member"
	// GuildRankVeteran indicates a member with proven track record.
	GuildRankVeteran GuildRank = "veteran"
	// GuildRankOfficer can recruit and manage guild quests.
	GuildRankOfficer GuildRank = "officer"
	// GuildRankMaster leads the guild.
	GuildRankMaster GuildRank = "guildmaster"
)

// GuildBonusRate returns the XP bonus rate for this guild rank.
// Higher ranks earn more from guild quests as reward for their investment.
//
//	Initiate:    10%
//	Member:      15%
//	Veteran:     18%
//	Officer:     20%
//	Guildmaster: 25%
func (r GuildRank) GuildBonusRate() float64 {
	switch r {
	case GuildRankInitiate:
		return 0.10
	case GuildRankMember:
		return 0.15
	case GuildRankVeteran:
		return 0.18
	case GuildRankOfficer:
		return 0.20
	case GuildRankMaster:
		return 0.25
	default:
		return 0.10 // Unknown rank gets initiate rate
	}
}

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
	Library     []LibraryEntry `json:"library"`      // Shared patterns, templates, context
	SharedTools []string       `json:"shared_tools"` // Tool IDs available to members

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

// LibraryEntry represents a shared resource in a guild's library.
type LibraryEntry struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	Content       any       `json:"content"`  // Prompt templates, patterns, examples
	Category      string    `json:"category"` // "template", "pattern", "example", "context"
	AddedBy       AgentID   `json:"added_by"`
	UseCount      int       `json:"use_count"`     // How often it's been referenced
	Effectiveness float64   `json:"effectiveness"` // Correlation with quest success when used
	AddedAt       time.Time `json:"added_at"`
}
