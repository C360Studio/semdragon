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

// AgentID uniquely identifies an agent in the system.
type AgentID string

// GuildID uniquely identifies a guild.
type GuildID string

// QuestID uniquely identifies a quest on the board.
type QuestID string

// PartyID uniquely identifies a party of agents.
type PartyID string

// BattleID uniquely identifies a boss battle (review session).
type BattleID string

// AgentStatus represents the current state of an agent in the system.
type AgentStatus string

const (
	// AgentIdle indicates the agent is available to claim quests.
	AgentIdle AgentStatus = "idle"
	// AgentOnQuest indicates the agent is currently executing a quest.
	AgentOnQuest AgentStatus = "on_quest"
	// AgentInBattle indicates the agent is facing a boss battle (review).
	AgentInBattle AgentStatus = "in_battle"
	// AgentCooldown indicates the agent failed and is cooling down before retry.
	AgentCooldown AgentStatus = "cooldown"
	// AgentRetired indicates permadeath from catastrophic failure.
	AgentRetired AgentStatus = "retired"
)

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
	Tier      TrustTier  `json:"tier"`             // Derived from level
	Equipment []Tool     `json:"equipment"`        // Tools this agent can use
	Skills    []SkillTag `json:"skills,omitempty"` // DEPRECATED: Use SkillProficiencies instead
	Guilds    []GuildID  `json:"guilds"`           // Guild memberships

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
	if a.SkillProficiencies != nil {
		_, exists := a.SkillProficiencies[skill]
		return exists
	}
	// Fall back to legacy Skills slice for backward compatibility
	for _, s := range a.Skills {
		if s == skill {
			return true
		}
	}
	return false
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

// GetSkillTags returns all skills the agent has (from both new and legacy fields).
func (a *Agent) GetSkillTags() []SkillTag {
	if a.SkillProficiencies != nil && len(a.SkillProficiencies) > 0 {
		skills := make([]SkillTag, 0, len(a.SkillProficiencies))
		for skill := range a.SkillProficiencies {
			skills = append(skills, skill)
		}
		return skills
	}
	return a.Skills
}

// MigrateSkills converts legacy Skills slice to SkillProficiencies map.
// Each skill is initialized at Novice level with zero progress.
// This is idempotent - calling it multiple times has no effect if already migrated.
func (a *Agent) MigrateSkills() {
	if a.SkillProficiencies == nil {
		a.SkillProficiencies = make(map[SkillTag]SkillProficiency)
	}
	for _, skill := range a.Skills {
		if _, exists := a.SkillProficiencies[skill]; !exists {
			a.SkillProficiencies[skill] = SkillProficiency{
				Level:      ProficiencyNovice,
				Progress:   0,
				TotalXP:    0,
				QuestsUsed: 0,
			}
		}
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

// SkillTag represents a domain of competence.
type SkillTag string

// Skill tags for common agent competencies.
const (
	SkillCodeGen       SkillTag = "code_generation"
	SkillCodeReview    SkillTag = "code_review"
	SkillDataTransform SkillTag = "data_transformation"
	SkillSummarization SkillTag = "summarization"
	SkillResearch      SkillTag = "research"
	SkillPlanning      SkillTag = "planning"
	SkillCustomerComms SkillTag = "customer_communications"
	SkillAnalysis      SkillTag = "analysis"
	SkillTraining      SkillTag = "training" // Can lead training parties as mentor
)

// -----------------------------------------------------------------------------
// Agent Persona - Character identity that shapes behavior, NOT progression
// -----------------------------------------------------------------------------
//
// DESIGN PRINCIPLE: Persona affects TRAJECTORY, not PROGRESSION.
//
// Persona influences (trajectory):
//   - Communication and output style
//   - Problem-solving approach
//   - Quest type preferences (soft attraction via boids, not hard gates)
//   - Party compatibility (complementary personalities)
//   - Guild culture fit
//
// Persona does NOT influence (progression):
//   - XP calculations
//   - Boss battle verdicts
//   - Skill proficiency gains
//   - Tier capabilities
//   - Review outcomes
//   - Level progression
//
// This ensures fair competition: agents succeed based on demonstrated
// competence, not character backstory. A well-written persona makes an
// agent more interesting, not more powerful.

// AgentPersona defines an agent's character identity and behavioral style.
type AgentPersona struct {
	// SystemPrompt is injected into the agent's LLM calls to shape behavior.
	// Should describe thinking style, communication approach, problem-solving
	// preferences - NOT claims of capability or expertise.
	SystemPrompt string `json:"system_prompt"`

	// Backstory provides RPG flavor text and may hint at guild affinity.
	// Example: "Forged in the data mines of the Analytics Guild..."
	Backstory string `json:"backstory"`

	// Traits are personality descriptors that affect style, not power.
	// Examples: "methodical", "creative", "terse", "thorough", "playful"
	// These may influence party formation (complementary traits work well).
	Traits []string `json:"traits,omitempty"`

	// Style describes communication preferences.
	// Examples: "formal", "casual", "technical", "narrative"
	Style string `json:"style,omitempty"`
}

// -----------------------------------------------------------------------------
// Skill Proficiency - Skills have levels that improve through use
// -----------------------------------------------------------------------------

// ProficiencyLevel represents mastery level of a skill (1-5).
type ProficiencyLevel int

const (
	// ProficiencyNovice is basic familiarity (default starting level).
	ProficiencyNovice ProficiencyLevel = 1
	// ProficiencyApprentice is developing competence.
	ProficiencyApprentice ProficiencyLevel = 2
	// ProficiencyJourneyman is solid working knowledge.
	ProficiencyJourneyman ProficiencyLevel = 3
	// ProficiencyExpert is high mastery.
	ProficiencyExpert ProficiencyLevel = 4
	// ProficiencyMaster is peak proficiency.
	ProficiencyMaster ProficiencyLevel = 5
)

// ProficiencyLevelNames maps levels to human-readable names.
var ProficiencyLevelNames = map[ProficiencyLevel]string{
	ProficiencyNovice:     "Novice",
	ProficiencyApprentice: "Apprentice",
	ProficiencyJourneyman: "Journeyman",
	ProficiencyExpert:     "Expert",
	ProficiencyMaster:     "Master",
}

// SkillProficiency tracks an agent's mastery of a specific skill.
type SkillProficiency struct {
	Level      ProficiencyLevel `json:"level"`
	Progress   int              `json:"progress"`    // 0-99 points toward next level
	TotalXP    int64            `json:"total_xp"`    // Lifetime XP earned using this skill
	QuestsUsed int              `json:"quests_used"` // Number of quests using this skill
	LastUsed   *time.Time       `json:"last_used,omitempty"`
}

// ProgressPercent returns progress as a percentage (0-100).
func (sp SkillProficiency) ProgressPercent() float64 {
	if sp.Level >= ProficiencyMaster {
		return 100.0
	}
	return float64(sp.Progress)
}

// CanLevelUp returns true if progress is sufficient for level up.
func (sp SkillProficiency) CanLevelUp() bool {
	return sp.Progress >= 100 && sp.Level < ProficiencyMaster
}

// -----------------------------------------------------------------------------
// Trust Tiers - What agents are allowed to do based on level
// -----------------------------------------------------------------------------

// TrustTier represents an agent's trust level derived from their level.
type TrustTier int

// Trust tier constants defining capability gates.
const (
	// TierApprentice is for levels 1-5: read-only, summarize, simple transforms.
	TierApprentice TrustTier = iota
	// TierJourneyman is for levels 6-10: can call tools, make API requests.
	TierJourneyman
	// TierExpert is for levels 11-15: can spend money, modify state, write to prod.
	TierExpert
	// TierMaster is for levels 16-18: can supervise agents, decompose quests, lead parties.
	TierMaster
	// TierGrandmaster is for levels 19-20: can act as DM delegate, create quests, manage guilds.
	TierGrandmaster
)

// TierFromLevel returns the trust tier for a given agent level.
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

// TierPerms maps each trust tier to its allowed permissions.
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

// Tool represents a capability an agent can use.
type Tool struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	MinTier     TrustTier      `json:"min_tier"`
	Category    string         `json:"category"`
	Dangerous   bool           `json:"dangerous"`
	Config      map[string]any `json:"config"`
}

// -----------------------------------------------------------------------------
// Quest - A unit of work posted to the quest board
// -----------------------------------------------------------------------------

// QuestStatus represents the lifecycle state of a quest.
type QuestStatus string

// Quest status constants.
const (
	// QuestPosted indicates the quest is on the board, unclaimed.
	QuestPosted QuestStatus = "posted"
	// QuestClaimed indicates the quest is claimed by an agent/party.
	QuestClaimed QuestStatus = "claimed"
	// QuestInProgress indicates the quest is actively being worked.
	QuestInProgress QuestStatus = "in_progress"
	// QuestInReview indicates the quest is in boss battle (review) phase.
	QuestInReview QuestStatus = "in_review"
	// QuestCompleted indicates the quest is done and reviewed.
	QuestCompleted QuestStatus = "completed"
	// QuestFailed indicates the quest failed and may be re-posted.
	QuestFailed QuestStatus = "failed"
	// QuestEscalated indicates TPK - needs higher-level attention.
	QuestEscalated QuestStatus = "escalated"
	// QuestCancelled indicates the quest was withdrawn.
	QuestCancelled QuestStatus = "cancelled"
)

// QuestDifficulty represents the challenge level of a quest.
type QuestDifficulty int

// Quest difficulty constants.
const (
	// DifficultyTrivial is for level 1-5 agents.
	DifficultyTrivial QuestDifficulty = iota
	// DifficultyEasy is for level 3-7 agents.
	DifficultyEasy
	// DifficultyModerate is for level 6-10 agents.
	DifficultyModerate
	// DifficultyHard is for level 10-14 agents.
	DifficultyHard
	// DifficultyEpic is for level 14-18 agents.
	DifficultyEpic
	// DifficultyLegendary is for level 18-20 agents or requires a party.
	DifficultyLegendary
)

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

// -----------------------------------------------------------------------------
// Boss Battle - Quality gates disguised as encounters
// -----------------------------------------------------------------------------

// ReviewLevel indicates the rigor of the boss battle review.
type ReviewLevel int

// Review level constants.
const (
	// ReviewAuto uses automated checks only (the goblin).
	ReviewAuto ReviewLevel = iota
	// ReviewStandard uses LLM-as-judge review (the ogre).
	ReviewStandard
	// ReviewStrict uses a multi-judge panel (the dragon).
	ReviewStrict
	// ReviewHuman requires a human reviewer (the DM themselves).
	ReviewHuman
)

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

// BattleStatus represents the state of a boss battle.
type BattleStatus string

// Battle status constants.
const (
	// BattleActive indicates the battle is in progress.
	BattleActive BattleStatus = "active"
	// BattleVictory indicates the agent passed review.
	BattleVictory BattleStatus = "victory"
	// BattleDefeat indicates the agent failed review.
	BattleDefeat BattleStatus = "defeat"
	// BattleRetreat indicates the agent requested a re-do.
	BattleRetreat BattleStatus = "retreat"
)

// BattleVerdict holds the outcome of a boss battle.
type BattleVerdict struct {
	Passed       bool    `json:"passed"`
	QualityScore float64 `json:"quality_score"`
	XPAwarded    int64   `json:"xp_awarded"`
	XPPenalty    int64   `json:"xp_penalty"`
	Feedback     string  `json:"feedback"`
	LevelChange  int     `json:"level_change"`
}

// ReviewCriterion defines a single evaluation criterion for a boss battle.
type ReviewCriterion struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Weight      float64 `json:"weight"`
	Threshold   float64 `json:"threshold"`
}

// ReviewResult holds a judge's evaluation of a single criterion.
type ReviewResult struct {
	CriterionName string  `json:"criterion_name"`
	Score         float64 `json:"score"`
	Passed        bool    `json:"passed"`
	Reasoning     string  `json:"reasoning"`
	JudgeID       string  `json:"judge_id"`
}

// Judge represents an evaluator for boss battles.
type Judge struct {
	ID     string         `json:"id"`
	Type   JudgeType      `json:"type"`
	Config map[string]any `json:"config"`
}

// JudgeType indicates the kind of judge (automated, LLM, or human).
type JudgeType string

// Judge type constants.
const (
	// JudgeAutomated uses rule-based checks.
	JudgeAutomated JudgeType = "automated"
	// JudgeLLM uses LLM-as-judge evaluation.
	JudgeLLM JudgeType = "llm"
	// JudgeHuman requires a human reviewer.
	JudgeHuman JudgeType = "human"
)
