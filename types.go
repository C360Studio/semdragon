package semdragons

import (
	"time"

	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// PRIMITIVE TYPE ALIASES - domain/ is the single source of truth
// =============================================================================

// AgentID uniquely identifies an agent in the system.
type AgentID = domain.AgentID

// GuildID uniquely identifies a guild.
type GuildID = domain.GuildID

// QuestID uniquely identifies a quest on the board.
type QuestID = domain.QuestID

// PartyID uniquely identifies a party of agents.
type PartyID = domain.PartyID

// BattleID uniquely identifies a boss battle (review session).
type BattleID = domain.BattleID

// AgentStatus represents the current state of an agent in the system.
type AgentStatus = domain.AgentStatus

// AgentIdle and related constants define agent status values.
const (
	AgentIdle     = domain.AgentIdle
	AgentOnQuest  = domain.AgentOnQuest
	AgentInBattle = domain.AgentInBattle
	AgentCooldown = domain.AgentCooldown
	AgentRetired  = domain.AgentRetired
)

// TrustTier represents an agent's trust level derived from their level.
type TrustTier = domain.TrustTier

// TierApprentice and related constants define trust tier levels.
const (
	TierApprentice  = domain.TierApprentice
	TierJourneyman  = domain.TierJourneyman
	TierExpert      = domain.TierExpert
	TierMaster      = domain.TierMaster
	TierGrandmaster = domain.TierGrandmaster
)

// TierFromLevel returns the trust tier for a given agent level.
func TierFromLevel(level int) TrustTier {
	return domain.TierFromLevel(level)
}

// SkillTag represents a domain of competence.
type SkillTag = domain.SkillTag

// SkillCodeGen and related constants define skill tag values.
const (
	SkillCodeGen       = domain.SkillCodeGen
	SkillCodeReview    = domain.SkillCodeReview
	SkillDataTransform = domain.SkillDataTransform
	SkillSummarization = domain.SkillSummarization
	SkillResearch      = domain.SkillResearch
	SkillPlanning      = domain.SkillPlanning
	SkillCustomerComms = domain.SkillCustomerComms
	SkillAnalysis      = domain.SkillAnalysis
	SkillTraining      = domain.SkillTraining
)

// ProficiencyLevel represents mastery level of a skill (1-5).
type ProficiencyLevel = domain.ProficiencyLevel

// ProficiencyNovice and related constants define proficiency levels.
const (
	ProficiencyNovice     = domain.ProficiencyNovice
	ProficiencyApprentice = domain.ProficiencyApprentice
	ProficiencyJourneyman = domain.ProficiencyJourneyman
	ProficiencyExpert     = domain.ProficiencyExpert
	ProficiencyMaster     = domain.ProficiencyMaster
)

// ProficiencyLevelName returns the human-readable name for a proficiency level.
func ProficiencyLevelName(level ProficiencyLevel) string {
	return domain.ProficiencyLevelName(level)
}

// SkillProficiency tracks an agent's mastery of a specific skill.
type SkillProficiency = domain.SkillProficiency

// QuestStatus represents the lifecycle state of a quest.
type QuestStatus = domain.QuestStatus

// QuestPosted and related constants define quest status values.
const (
	QuestPosted     = domain.QuestPosted
	QuestClaimed    = domain.QuestClaimed
	QuestInProgress = domain.QuestInProgress
	QuestInReview   = domain.QuestInReview
	QuestCompleted  = domain.QuestCompleted
	QuestFailed     = domain.QuestFailed
	QuestEscalated  = domain.QuestEscalated
	QuestCancelled  = domain.QuestCancelled
)

// QuestDifficulty represents the challenge level of a quest.
type QuestDifficulty = domain.QuestDifficulty

// DifficultyTrivial and related constants define quest difficulty levels.
const (
	DifficultyTrivial   = domain.DifficultyTrivial
	DifficultyEasy      = domain.DifficultyEasy
	DifficultyModerate  = domain.DifficultyModerate
	DifficultyHard      = domain.DifficultyHard
	DifficultyEpic      = domain.DifficultyEpic
	DifficultyLegendary = domain.DifficultyLegendary
)

// ReviewLevel indicates the rigor of the boss battle review.
type ReviewLevel = domain.ReviewLevel

// ReviewAuto and related constants define review level values.
const (
	ReviewAuto     = domain.ReviewAuto
	ReviewStandard = domain.ReviewStandard
	ReviewStrict   = domain.ReviewStrict
	ReviewHuman    = domain.ReviewHuman
)

// BattleStatus represents the state of a boss battle.
type BattleStatus = domain.BattleStatus

// BattleActive and related constants define battle status values.
const (
	BattleActive  = domain.BattleActive
	BattleVictory = domain.BattleVictory
	BattleDefeat  = domain.BattleDefeat
	BattleRetreat = domain.BattleRetreat
)

// JudgeType indicates the kind of judge (automated, LLM, or human).
type JudgeType = domain.JudgeType

// JudgeAutomated and related constants define judge type values.
const (
	JudgeAutomated = domain.JudgeAutomated
	JudgeLLM       = domain.JudgeLLM
	JudgeHuman     = domain.JudgeHuman
)

// ReviewCriterion defines a single evaluation criterion for a boss battle.
type ReviewCriterion = domain.ReviewCriterion

// ReviewResult holds a judge's evaluation of a single criterion.
type ReviewResult = domain.ReviewResult

// Tool represents a capability an agent can use.
type Tool = domain.Tool

// =============================================================================
// XP HELPERS (re-exported from domain)
// =============================================================================

// DefaultXPForDifficulty returns base XP for quest difficulty.
func DefaultXPForDifficulty(difficulty QuestDifficulty) int64 {
	return domain.DefaultXPForDifficulty(difficulty)
}

// TierFromDifficulty returns the minimum trust tier for a difficulty level.
func TierFromDifficulty(difficulty QuestDifficulty) TrustTier {
	return domain.TierFromDifficulty(difficulty)
}

// =============================================================================
// TIER PERMISSIONS - domain/ is the single source of truth
// =============================================================================

// TierPermissions defines what a trust tier is allowed to do.
type TierPermissions = domain.TierPermissions

// TierPermissionsFor returns the permissions for the given trust tier.
func TierPermissionsFor(tier TrustTier) TierPermissions {
	return domain.TierPermissionsFor(tier)
}

// =============================================================================
// ENTITY STRUCTS (root-owned, reference aliased primitives)
// =============================================================================

// Agent represents an autonomous worker in the semdragons system.
// Agents earn XP, level up, join guilds, and claim quests from the board.
type Agent struct {
	ID          AgentID     `json:"id"`
	Name        string      `json:"name"`         // System identifier
	DisplayName string      `json:"display_name"` // Character name chosen by agent (e.g., "Shadowweaver")
	Status      AgentStatus `json:"status"`

	// Persona defines the agent's character identity and behavioral style.
	// IMPORTANT: Persona affects trajectory (how the agent works) but NOT progression
	// (XP, levels, boss battle outcomes). See AgentPersona documentation.
	Persona *AgentPersona `json:"persona,omitempty"`

	// Progression
	Level      int   `json:"level"`       // 1-20, determines trust/capability tier
	XP         int64 `json:"xp"`          // Current experience points
	XPToLevel  int64 `json:"xp_to_level"` // XP needed for next level
	DeathCount int   `json:"death_count"` // Lifetime deaths - reputation scar

	// Capabilities & Trust
	Tier      TrustTier `json:"tier"`      // Derived from level
	Equipment []Tool    `json:"equipment"` // Tools this agent can use
	Guilds    []GuildID `json:"guilds"`    // Guild memberships

	// Skill Proficiencies - tracks mastery level for each skill
	SkillProficiencies map[SkillTag]SkillProficiency `json:"skill_proficiencies"`

	// State
	CurrentQuest  *QuestID   `json:"current_quest,omitempty"`
	CurrentParty  *PartyID   `json:"current_party,omitempty"`
	CooldownUntil *time.Time `json:"cooldown_until,omitempty"`

	// Stats - lifetime tracking for the DM and observability
	Stats AgentStats `json:"stats"`

	// Backing config - what actually powers this agent
	Config AgentConfig `json:"config"`

	// NPC flag - true for bootstrap/trainer NPCs that phase out when real agents are ready
	IsNPC bool `json:"is_npc,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// HasSkill returns true if the agent has the specified skill (at any proficiency level).
func (a *Agent) HasSkill(skill SkillTag) bool {
	if a.SkillProficiencies == nil {
		return false
	}
	_, exists := a.SkillProficiencies[skill]
	return exists
}

// GetProficiency returns the proficiency for a skill.
// Returns a zero-value SkillProficiency if the agent doesn't have the skill.
func (a *Agent) GetProficiency(skill SkillTag) SkillProficiency {
	if a.SkillProficiencies != nil {
		if prof, exists := a.SkillProficiencies[skill]; exists {
			return prof
		}
	}
	return SkillProficiency{}
}

// GetSkillTags returns all skills the agent has.
func (a *Agent) GetSkillTags() []SkillTag {
	if a.SkillProficiencies == nil {
		return nil
	}
	skills := make([]SkillTag, 0, len(a.SkillProficiencies))
	for skill := range a.SkillProficiencies {
		skills = append(skills, skill)
	}
	return skills
}

// EnsureSkillProficiencies initializes the SkillProficiencies map if nil.
// This is idempotent - calling it multiple times has no effect if already initialized.
func (a *Agent) EnsureSkillProficiencies() {
	if a.SkillProficiencies == nil {
		a.SkillProficiencies = make(map[SkillTag]SkillProficiency)
	}
}

// AddSkill adds a new skill to the agent at Novice level.
// If the skill already exists, this is a no-op.
func (a *Agent) AddSkill(skill SkillTag) {
	if a.SkillProficiencies == nil {
		a.SkillProficiencies = make(map[SkillTag]SkillProficiency)
	}
	if _, exists := a.SkillProficiencies[skill]; !exists {
		a.SkillProficiencies[skill] = SkillProficiency{
			Level:      ProficiencyNovice,
			Progress:   0,
			TotalXP:    0,
			QuestsUsed: 0,
		}
	}
}

// AgentConfig holds the actual implementation details behind the RPG facade.
type AgentConfig struct {
	Provider     string            `json:"provider"` // "openai", "anthropic", "local", etc.
	Model        string            `json:"model"`    // "claude-sonnet-4-5-20250514", "gpt-4o", etc.
	SystemPrompt string            `json:"system_prompt"`
	Temperature  float64           `json:"temperature"`
	MaxTokens    int               `json:"max_tokens"`
	Metadata     map[string]string `json:"metadata"` // Arbitrary config
}

// AgentStats tracks lifetime performance metrics for an agent.
type AgentStats struct {
	QuestsCompleted  int     `json:"quests_completed"`
	QuestsFailed     int     `json:"quests_failed"`
	BossesDefeated   int     `json:"bosses_defeated"`
	BossesFailed     int     `json:"bosses_failed"`
	TotalXPEarned    int64   `json:"total_xp_earned"`
	TotalXPSpent     int64   `json:"total_xp_spent"` // XP spent in store
	AvgQualityScore  float64 `json:"avg_quality_score"`
	AvgEfficiency    float64 `json:"avg_efficiency"`
	PartiesLed       int     `json:"parties_led"`
	QuestsDecomposed int     `json:"quests_decomposed"`
}

// AgentPersona defines an agent's character identity and behavioral style.
type AgentPersona struct {
	SystemPrompt string   `json:"system_prompt"`
	Backstory    string   `json:"backstory"`
	Traits       []string `json:"traits,omitempty"`
	Style        string   `json:"style,omitempty"`
}

// Quest represents a unit of work on the quest board.
type Quest struct {
	ID          QuestID         `json:"id"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Status      QuestStatus     `json:"status"`
	Difficulty  QuestDifficulty `json:"difficulty"`

	// Requirements
	RequiredSkills []SkillTag `json:"required_skills"`
	RequiredTools  []string   `json:"required_tools"` // Tool IDs
	MinTier        TrustTier  `json:"min_tier"`
	PartyRequired  bool       `json:"party_required"` // Too big for solo
	MinPartySize   int        `json:"min_party_size"`

	// Rewards
	BaseXP  int64 `json:"base_xp"`
	BonusXP int64 `json:"bonus_xp"` // For exceptional quality
	GuildXP int64 `json:"guild_xp"` // XP toward guild reputation

	// Execution context - the actual work
	Input        any              `json:"input"`  // Quest payload
	Output       any              `json:"output"` // Result when completed
	Constraints  QuestConstraints `json:"constraints"`
	AllowedTools []string         `json:"allowed_tools,omitempty"` // Tool whitelist for execution (empty = all allowed)

	// Quest chain / decomposition
	ParentQuest  *QuestID  `json:"parent_quest,omitempty"`  // If this is a sub-quest
	SubQuests    []QuestID `json:"sub_quests,omitempty"`    // If decomposed
	DecomposedBy *AgentID  `json:"decomposed_by,omitempty"` // Party lead who broke it down

	// Assignment
	ClaimedBy     *AgentID `json:"claimed_by,omitempty"`
	PartyID       *PartyID `json:"party_id,omitempty"`
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

// QuestConstraints defines limits and requirements for quest execution.
type QuestConstraints struct {
	MaxDuration   time.Duration `json:"max_duration"`
	MaxCost       float64       `json:"max_cost"`
	MaxTokens     int           `json:"max_tokens"`
	RequireReview bool          `json:"require_review"`
	ReviewLevel   ReviewLevel   `json:"review_level"`
}

// BossBattle represents a quality gate review session.
type BossBattle struct {
	ID      BattleID     `json:"id"`
	QuestID QuestID      `json:"quest_id"`
	AgentID AgentID      `json:"agent_id"`
	Level   ReviewLevel  `json:"level"`
	Status  BattleStatus `json:"status"`

	Criteria    []ReviewCriterion `json:"criteria"`
	Results     []ReviewResult    `json:"results,omitempty"`
	Verdict     *BattleVerdict    `json:"verdict,omitempty"`
	Judges      []Judge           `json:"judges"`
	StartedAt   time.Time         `json:"started_at"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
}

// BattleVerdict holds the outcome of a boss battle.
type BattleVerdict struct {
	Passed       bool    `json:"passed"`
	QualityScore float64 `json:"quality_score"`
	XPAwarded    int64   `json:"xp_awarded"`
	XPPenalty    int64   `json:"xp_penalty"`
	Feedback     string  `json:"feedback"`
	LevelChange  int     `json:"level_change"`
}

// Judge represents an evaluator for boss battles.
type Judge struct {
	ID     string         `json:"id"`
	Type   JudgeType      `json:"type"`
	Config map[string]any `json:"config"`
}
