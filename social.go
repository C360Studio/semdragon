package semdragons

import (
	"time"

	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// PARTY/GUILD PRIMITIVE ALIASES - domain/ is the single source of truth
// =============================================================================

// PartyStatus represents the current state of a party.
type PartyStatus = domain.PartyStatus

// PartyForming and related constants define party status values.
const (
	PartyForming   = domain.PartyForming
	PartyActive    = domain.PartyActive
	PartyDisbanded = domain.PartyDisbanded
)

// PartyRole represents a member's role within a party.
type PartyRole = domain.PartyRole

// RoleLead and related constants define party role values.
const (
	RoleLead     = domain.RoleLead
	RoleExecutor = domain.RoleExecutor
	RoleReviewer = domain.RoleReviewer
	RoleScout    = domain.RoleScout
)

// GuildStatus represents the current state of a guild.
type GuildStatus = domain.GuildStatus

// GuildActive and related constants define guild status values.
const (
	GuildActive   = domain.GuildActive
	GuildInactive = domain.GuildInactive
)

// GuildRank represents a member's rank within a guild.
type GuildRank = domain.GuildRank

// GuildRankInitiate and related constants define guild rank values.
const (
	GuildRankInitiate = domain.GuildRankInitiate
	GuildRankMember   = domain.GuildRankMember
	GuildRankVeteran  = domain.GuildRankVeteran
	GuildRankOfficer  = domain.GuildRankOfficer
	GuildRankMaster   = domain.GuildRankMaster
)

// =============================================================================
// ENTITY STRUCTS (root-owned, reference aliased primitives)
// =============================================================================

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
