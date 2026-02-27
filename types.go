package semdragons

import (
	"time"
)

// =============================================================================
// CORE DOMAIN TYPES
// =============================================================================
// Semdragons: Agentic coordination modeled as a tabletop RPG
// Built on top of semstreams for observability, trajectories, and event streaming
// =============================================================================

// -----------------------------------------------------------------------------
// Agent - An autonomous worker that claims and executes quests
// -----------------------------------------------------------------------------

type AgentID string
type GuildID string
type QuestID string
type PartyID string
type BattleID string

// AgentStatus represents the current state of an agent in the system.
type AgentStatus string

const (
	AgentIdle     AgentStatus = "idle"      // Available to claim quests
	AgentOnQuest  AgentStatus = "on_quest"  // Currently executing a quest
	AgentInBattle AgentStatus = "in_battle" // Facing a boss battle (review)
	AgentCooldown AgentStatus = "cooldown"  // Dead/failed - cooling down before retry
	AgentRetired  AgentStatus = "retired"   // Permadeath - catastrophic failure
)

// Agent represents an autonomous worker in the semdragons system.
// Agents earn XP, level up, join guilds, and claim quests from the board.
type Agent struct {
	ID     AgentID     `json:"id"`
	Name   string      `json:"name"`
	Status AgentStatus `json:"status"`

	// Progression
	Level      int   `json:"level"`       // 1-20, determines trust/capability tier
	XP         int64 `json:"xp"`          // Current experience points
	XPToLevel  int64 `json:"xp_to_level"` // XP needed for next level
	DeathCount int   `json:"death_count"` // Lifetime deaths - reputation scar

	// Capabilities & Trust
	Tier      TrustTier   `json:"tier"`      // Derived from level
	Equipment []Tool      `json:"equipment"` // Tools this agent can use
	Skills    []SkillTag  `json:"skills"`    // What this agent is good at
	Guilds    []GuildID   `json:"guilds"`    // Guild memberships

	// State
	CurrentQuest  *QuestID   `json:"current_quest,omitempty"`
	CurrentParty  *PartyID   `json:"current_party,omitempty"`
	CooldownUntil *time.Time `json:"cooldown_until,omitempty"`

	// Stats - lifetime tracking for the DM and observability
	Stats AgentStats `json:"stats"`

	// Backing config - what actually powers this agent
	Config AgentConfig `json:"config"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AgentConfig holds the actual implementation details behind the RPG facade.
type AgentConfig struct {
	Provider    string            `json:"provider"`     // "openai", "anthropic", "local", etc.
	Model       string            `json:"model"`        // "claude-sonnet-4-5-20250514", "gpt-4o", etc.
	SystemPrompt string           `json:"system_prompt"`
	Temperature float64           `json:"temperature"`
	MaxTokens   int               `json:"max_tokens"`
	Metadata    map[string]string `json:"metadata"`     // Arbitrary config
}

type AgentStats struct {
	QuestsCompleted  int     `json:"quests_completed"`
	QuestsFailed     int     `json:"quests_failed"`
	BossesDefeated   int     `json:"bosses_defeated"`
	BossesFailed     int     `json:"bosses_failed"`
	TotalXPEarned    int64   `json:"total_xp_earned"`
	AvgQualityScore  float64 `json:"avg_quality_score"`  // 0.0-1.0
	AvgEfficiency    float64 `json:"avg_efficiency"`      // Time vs estimate
	PartiesLed       int     `json:"parties_led"`
	QuestsDecomposed int     `json:"quests_decomposed"`
}

// SkillTag represents a domain of competence.
type SkillTag string

const (
	SkillCodeGen       SkillTag = "code_generation"
	SkillCodeReview    SkillTag = "code_review"
	SkillDataTransform SkillTag = "data_transformation"
	SkillSummarization SkillTag = "summarization"
	SkillResearch      SkillTag = "research"
	SkillPlanning      SkillTag = "planning"
	SkillCustomerComms SkillTag = "customer_communications"
	SkillAnalysis      SkillTag = "analysis"
	// Extend as needed - these should be domain-specific
)

// -----------------------------------------------------------------------------
// Trust Tiers - What agents are allowed to do based on level
// -----------------------------------------------------------------------------

type TrustTier int

const (
	TierApprentice TrustTier = iota // Level 1-5:   Read-only, summarize, simple transforms
	TierJourneyman                   // Level 6-10:  Can call tools, make API requests
	TierExpert                       // Level 11-15: Can spend money, modify state, write to prod
	TierMaster                       // Level 16-18: Can supervise agents, decompose quests, lead parties
	TierGrandmaster                  // Level 19-20: Can act as DM delegate, create quests, manage guilds
)

func TierFromLevel(level int) TrustTier {
	switch {
	case level <= 5:
		return TierApprentice
	case level <= 10:
		return TierJourneyman
	case level <= 15:
		return TierExpert
	case level <= 18:
		return TierMaster
	default:
		return TierGrandmaster
	}
}

// TierPermissions defines what a trust tier is allowed to do.
type TierPermissions struct {
	CanUseTool        func(Tool) bool
	CanClaimQuestTier QuestDifficulty
	CanLeadParty      bool
	CanDecomposeQuest bool
	CanSupervise      bool
	CanActAsDM        bool
	MaxConcurrent     int // Max concurrent quests
}

var TierPerms = map[TrustTier]TierPermissions{
	TierApprentice: {
		CanClaimQuestTier: DifficultyTrivial,
		MaxConcurrent:     1,
	},
	TierJourneyman: {
		CanClaimQuestTier: DifficultyModerate,
		MaxConcurrent:     2,
	},
	TierExpert: {
		CanClaimQuestTier: DifficultyHard,
		MaxConcurrent:     3,
	},
	TierMaster: {
		CanClaimQuestTier: DifficultyEpic,
		CanLeadParty:      true,
		CanDecomposeQuest: true,
		CanSupervise:      true,
		MaxConcurrent:     4,
	},
	TierGrandmaster: {
		CanClaimQuestTier: DifficultyLegendary,
		CanLeadParty:      true,
		CanDecomposeQuest: true,
		CanSupervise:      true,
		CanActAsDM:        true,
		MaxConcurrent:     5,
	},
}

// -----------------------------------------------------------------------------
// Tools / Equipment - What agents have access to
// -----------------------------------------------------------------------------

type Tool struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	MinTier     TrustTier `json:"min_tier"`     // Minimum tier to equip
	Category    string    `json:"category"`      // "read", "write", "execute", "financial"
	Dangerous   bool      `json:"dangerous"`     // Requires extra review when used
	Config      map[string]interface{} `json:"config"`
}

// -----------------------------------------------------------------------------
// Quest - A unit of work posted to the quest board
// -----------------------------------------------------------------------------

type QuestStatus string

const (
	QuestPosted     QuestStatus = "posted"      // On the board, unclaimed
	QuestClaimed    QuestStatus = "claimed"      // Claimed by an agent/party
	QuestInProgress QuestStatus = "in_progress"  // Actively being worked
	QuestInReview   QuestStatus = "in_review"    // Boss battle phase
	QuestCompleted  QuestStatus = "completed"    // Done and reviewed
	QuestFailed     QuestStatus = "failed"       // Failed - may be re-posted
	QuestEscalated  QuestStatus = "escalated"    // TPK - needs higher-level attention
	QuestCancelled  QuestStatus = "cancelled"    // Withdrawn
)

type QuestDifficulty int

const (
	DifficultyTrivial   QuestDifficulty = iota // Level 1-5 agents
	DifficultyEasy                              // Level 3-7
	DifficultyModerate                          // Level 6-10
	DifficultyHard                              // Level 10-14
	DifficultyEpic                              // Level 14-18
	DifficultyLegendary                         // Level 18-20, or party required
)

// Quest represents a unit of work on the quest board.
type Quest struct {
	ID          QuestID         `json:"id"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Status      QuestStatus     `json:"status"`
	Difficulty  QuestDifficulty `json:"difficulty"`

	// Requirements
	RequiredSkills []SkillTag    `json:"required_skills"`
	RequiredTools  []string      `json:"required_tools"`   // Tool IDs
	MinTier        TrustTier     `json:"min_tier"`
	PartyRequired  bool          `json:"party_required"`   // Too big for solo
	MinPartySize   int           `json:"min_party_size"`

	// Rewards
	BaseXP    int64 `json:"base_xp"`
	BonusXP   int64 `json:"bonus_xp"`    // For exceptional quality
	GuildXP   int64 `json:"guild_xp"`    // XP toward guild reputation

	// Execution context - the actual work
	Input       interface{}       `json:"input"`        // Quest payload
	Output      interface{}       `json:"output"`       // Result when completed
	Constraints QuestConstraints  `json:"constraints"`

	// Quest chain / decomposition
	ParentQuest   *QuestID   `json:"parent_quest,omitempty"`  // If this is a sub-quest
	SubQuests     []QuestID  `json:"sub_quests,omitempty"`    // If decomposed
	DecomposedBy  *AgentID   `json:"decomposed_by,omitempty"` // Party lead who broke it down

	// Assignment
	ClaimedBy    *AgentID  `json:"claimed_by,omitempty"`
	PartyID      *PartyID  `json:"party_id,omitempty"`
	GuildPriority *GuildID `json:"guild_priority,omitempty"` // Guild gets first dibs

	// Lifecycle
	PostedAt    time.Time  `json:"posted_at"`
	ClaimedAt   *time.Time `json:"claimed_at,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Deadline    *time.Time `json:"deadline,omitempty"`

	// Failure tracking
	Attempts    int  `json:"attempts"`
	MaxAttempts int  `json:"max_attempts"`
	Escalated   bool `json:"escalated"`

	// Observability - links back to semstreams
	TrajectoryID string `json:"trajectory_id"` // semstreams trajectory reference
}

type QuestConstraints struct {
	MaxDuration   time.Duration `json:"max_duration"`
	MaxCost       float64       `json:"max_cost"`        // Budget limit (API calls, etc.)
	MaxTokens     int           `json:"max_tokens"`
	RequireReview bool          `json:"require_review"`   // Must face boss battle
	ReviewLevel   ReviewLevel   `json:"review_level"`     // How tough the boss battle is
}

// -----------------------------------------------------------------------------
// Boss Battle - Quality gates disguised as encounters
// -----------------------------------------------------------------------------

type ReviewLevel int

const (
	ReviewAuto     ReviewLevel = iota // Automated checks only (the goblin)
	ReviewStandard                     // LLM-as-judge review (the ogre)
	ReviewStrict                       // Multi-judge panel (the dragon)
	ReviewHuman                        // Human reviewer required (the DM themselves)
)

type BossBattle struct {
	ID        BattleID    `json:"id"`
	QuestID   QuestID     `json:"quest_id"`
	AgentID   AgentID     `json:"agent_id"`
	Level     ReviewLevel `json:"level"`
	Status    BattleStatus `json:"status"`

	// The review itself
	Criteria  []ReviewCriterion `json:"criteria"`
	Results   []ReviewResult    `json:"results,omitempty"`
	Verdict   *BattleVerdict    `json:"verdict,omitempty"`

	// Judges
	Judges    []Judge           `json:"judges"`

	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type BattleStatus string

const (
	BattleActive    BattleStatus = "active"
	BattleVictory   BattleStatus = "victory"    // Passed review
	BattleDefeat    BattleStatus = "defeat"     // Failed review
	BattleRetreat   BattleStatus = "retreat"    // Agent requested re-do
)

type BattleVerdict struct {
	Passed       bool    `json:"passed"`
	QualityScore float64 `json:"quality_score"` // 0.0-1.0
	XPAwarded    int64   `json:"xp_awarded"`
	XPPenalty    int64   `json:"xp_penalty"`    // Deducted on failure
	Feedback     string  `json:"feedback"`      // What to improve
	LevelChange  int     `json:"level_change"`  // +1, 0, or -1
}

type ReviewCriterion struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Weight      float64 `json:"weight"`       // How much this matters (0.0-1.0)
	Threshold   float64 `json:"threshold"`    // Min score to pass (0.0-1.0)
}

type ReviewResult struct {
	CriterionName string  `json:"criterion_name"`
	Score         float64 `json:"score"`       // 0.0-1.0
	Passed        bool    `json:"passed"`
	Reasoning     string  `json:"reasoning"`
	JudgeID       string  `json:"judge_id"`
}

type Judge struct {
	ID       string    `json:"id"`
	Type     JudgeType `json:"type"`
	Config   map[string]interface{} `json:"config"`
}

type JudgeType string

const (
	JudgeAutomated JudgeType = "automated"  // Rule-based checks
	JudgeLLM       JudgeType = "llm"        // LLM-as-judge
	JudgeHuman     JudgeType = "human"      // Human reviewer
)
