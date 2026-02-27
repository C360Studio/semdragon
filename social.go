package semdragons

import (
	"time"
)

// =============================================================================
// PARTY - A temporary group formed to tackle a quest
// =============================================================================

type PartyStatus string

const (
	PartyForming   PartyStatus = "forming"    // Recruiting members
	PartyActive    PartyStatus = "active"     // On a quest
	PartyDisbanded PartyStatus = "disbanded"  // Quest complete or failed
)

type PartyRole string

const (
	RoleLead     PartyRole = "lead"      // Decomposes quest, coordinates, faces the boss
	RoleExecutor PartyRole = "executor"  // Does the actual sub-quest work
	RoleReviewer PartyRole = "reviewer"  // Internal QA before boss battle
	RoleScout    PartyRole = "scout"     // Research, context gathering, recon
)

// Party is a temporary group of agents formed to tackle a quest together.
// The party lead is responsible for decomposing the quest and rolling up results.
type Party struct {
	ID      PartyID     `json:"id"`
	Name    string      `json:"name"`      // Auto-generated or lead-chosen
	Status  PartyStatus `json:"status"`
	QuestID QuestID     `json:"quest_id"`  // The quest this party was formed for

	// Composition
	Lead    AgentID       `json:"lead"`
	Members []PartyMember `json:"members"`

	// Coordination
	Strategy    string              `json:"strategy"`      // The lead's plan of attack
	SubQuestMap map[QuestID]AgentID `json:"sub_quest_map"` // Who's doing what
	SharedContext []ContextItem      `json:"shared_context"` // Party-wide knowledge

	// Results
	SubResults map[QuestID]interface{} `json:"sub_results,omitempty"` // Collected sub-quest outputs
	RollupResult interface{}            `json:"rollup_result,omitempty"` // Lead's combined result

	FormedAt    time.Time  `json:"formed_at"`
	DisbandedAt *time.Time `json:"disbanded_at,omitempty"`
}

type PartyMember struct {
	AgentID AgentID   `json:"agent_id"`
	Role    PartyRole `json:"role"`
	Skills  []SkillTag `json:"skills"` // Why they were recruited
	JoinedAt time.Time `json:"joined_at"`
}

type ContextItem struct {
	Key       string      `json:"key"`
	Value     interface{} `json:"value"`
	AddedBy   AgentID     `json:"added_by"`
	AddedAt   time.Time   `json:"added_at"`
}

// =============================================================================
// GUILD - Persistent specialization clusters
// =============================================================================

type GuildStatus string

const (
	GuildActive   GuildStatus = "active"
	GuildInactive GuildStatus = "inactive"
)

type GuildRank string

const (
	GuildRankInitiate  GuildRank = "initiate"   // Just joined, proving themselves
	GuildRankMember    GuildRank = "member"     // Established contributor
	GuildRankVeteran   GuildRank = "veteran"    // Proven track record
	GuildRankOfficer   GuildRank = "officer"    // Can recruit, manage guild quests
	GuildRankMaster    GuildRank = "guildmaster" // Leads the guild
)

// Guild is a persistent group of agents that specialize in a domain.
// Guilds provide: routing priority, shared knowledge, reputation, and mentorship.
type Guild struct {
	ID          GuildID     `json:"id"`
	Name        string      `json:"name"`        // "Data Wranglers", "Code Reviewers", etc.
	Description string      `json:"description"`
	Status      GuildStatus `json:"status"`

	// Specialization
	Domain       string     `json:"domain"`         // Primary domain
	Skills       []SkillTag `json:"skills"`         // Skills this guild covers
	QuestTypes   []string   `json:"quest_types"`    // Types of quests they handle

	// Membership
	Members      []GuildMember `json:"members"`
	MaxMembers   int           `json:"max_members"`

	// Reputation - guild-level trust
	Reputation    float64 `json:"reputation"`      // 0.0-1.0, affects quest priority
	QuestsHandled int     `json:"quests_handled"`
	SuccessRate   float64 `json:"success_rate"`    // Completed / (Completed + Failed)

	// Guild Hall - shared knowledge and resources
	Library      []LibraryEntry `json:"library"`     // Shared patterns, templates, context
	SharedTools  []string       `json:"shared_tools"` // Tool IDs available to members

	// Formation rules
	MinLevelToJoin  int       `json:"min_level_to_join"`
	RequiredSkills  []SkillTag `json:"required_skills"`  // Must have at least one
	AutoRecruit     bool       `json:"auto_recruit"`     // Automatically invite qualifying agents

	CreatedAt time.Time `json:"created_at"`
}

type GuildMember struct {
	AgentID  AgentID   `json:"agent_id"`
	Rank     GuildRank `json:"rank"`
	GuildXP  int64     `json:"guild_xp"`    // XP within this guild specifically
	JoinedAt time.Time `json:"joined_at"`
}

type LibraryEntry struct {
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Content     interface{} `json:"content"`     // Prompt templates, patterns, examples
	Category    string      `json:"category"`    // "template", "pattern", "example", "context"
	AddedBy     AgentID     `json:"added_by"`
	UseCount    int         `json:"use_count"`   // How often it's been referenced
	Effectiveness float64  `json:"effectiveness"` // Correlation with quest success when used
	AddedAt     time.Time   `json:"added_at"`
}
