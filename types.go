package semdragons

import (
	"fmt"
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

// PeerReviewID uniquely identifies a peer review.
type PeerReviewID = domain.PeerReviewID

// AgentStatus represents the current state of an agent in the system.
type AgentStatus = domain.AgentStatus

// AgentIdle and related constants define agent status values.
const (
	AgentIdle          = domain.AgentIdle
	AgentOnQuest       = domain.AgentOnQuest
	AgentInBattle      = domain.AgentInBattle
	AgentCooldown      = domain.AgentCooldown
	AgentRetired       = domain.AgentRetired
	AgentPendingReview = domain.AgentPendingReview
)

// PeerReviewStatus represents the lifecycle state of a peer review.
type PeerReviewStatus = domain.PeerReviewStatus

// PeerReviewPending and related constants define peer review lifecycle states.
const (
	PeerReviewPending   = domain.PeerReviewPending
	PeerReviewPartial   = domain.PeerReviewPartial
	PeerReviewCompleted = domain.PeerReviewCompleted
)

// ReviewDirection indicates which party is reviewing which.
type ReviewDirection = domain.ReviewDirection

// ReviewDirectionLeaderToMember and related constants define review direction values.
const (
	ReviewDirectionLeaderToMember = domain.ReviewDirectionLeaderToMember
	ReviewDirectionMemberToLeader = domain.ReviewDirectionMemberToLeader
	ReviewDirectionDMToAgent      = domain.ReviewDirectionDMToAgent
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

	// Store inventory (reconstructed from agent entity triples)
	OwnedTools    map[string]OwnedTool `json:"owned_tools,omitempty"`
	Consumables   map[string]int       `json:"consumables,omitempty"`
	TotalSpent    int64                `json:"total_spent"`
	ActiveEffects []AgentEffect        `json:"active_effects,omitempty"`

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

// OwnedTool tracks an agent's purchased tool (stored as agent entity triples).
type OwnedTool struct {
	StoreItemID   string    `json:"store_item_id"`  // Entity ref → storeitem entity ID
	XPSpent       int64     `json:"xp_spent"`       // XP paid for this tool
	UsesRemaining int       `json:"uses_remaining"` // -1 = permanent
	PurchasedAt   time.Time `json:"purchased_at"`
}

// AgentEffect tracks a consumable effect currently active on an agent.
type AgentEffect struct {
	EffectType      string   `json:"effect_type"`        // xp_boost, quality_shield, etc.
	QuestsRemaining int      `json:"quests_remaining"`   // Quests until effect expires
	QuestID         *QuestID `json:"quest_id,omitempty"` // Quest that triggered the effect
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
	PeerReviewAvg    float64 `json:"peer_review_avg"`
	PeerReviewCount  int     `json:"peer_review_count"`
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
	Input        any              `json:"input"`                   // Quest payload
	Output       any              `json:"output"`                  // Result when completed
	Constraints  QuestConstraints `json:"constraints"`
	AllowedTools []string         `json:"allowed_tools,omitempty"` // Tool whitelist for execution (empty = all allowed)

	// Quest chain / decomposition
	ParentQuest  *QuestID  `json:"parent_quest,omitempty"`  // If this is a sub-quest
	SubQuests    []QuestID `json:"sub_quests,omitempty"`    // If decomposed
	DecomposedBy *AgentID  `json:"decomposed_by,omitempty"` // Party lead who broke it down
	DependsOn    []QuestID `json:"depends_on,omitempty"`    // Sibling dependencies (waits for these to complete)
	Acceptance   []string  `json:"acceptance,omitempty"`    // Domain-flexible acceptance criteria

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

// =============================================================================
// PEER REVIEW
// =============================================================================

// ReviewRatings holds the 3 peer review ratings (1-5 scale).
type ReviewRatings struct {
	Q1 int `json:"q1"` // Quality/Clarity
	Q2 int `json:"q2"` // Communication/Support
	Q3 int `json:"q3"` // Autonomy/Fairness
}

// Average returns the mean of the three ratings.
func (r ReviewRatings) Average() float64 {
	return float64(r.Q1+r.Q2+r.Q3) / 3.0
}

// Validate checks that ratings are in range and explanation is provided when required.
func (r ReviewRatings) Validate(explanation string) error {
	for i, v := range []int{r.Q1, r.Q2, r.Q3} {
		if v < 1 || v > 5 {
			return fmt.Errorf("rating Q%d must be between 1 and 5, got %d", i+1, v)
		}
	}
	if r.Average() < 3.0 && explanation == "" {
		return fmt.Errorf("explanation required when average rating is below 3.0")
	}
	return nil
}

// ReviewSubmission represents one party's submitted review.
type ReviewSubmission struct {
	ReviewerID  AgentID         `json:"reviewer_id"`
	RevieweeID  AgentID         `json:"reviewee_id"`
	Direction   ReviewDirection `json:"direction"`
	Ratings     ReviewRatings   `json:"ratings"`
	Explanation string          `json:"explanation,omitempty"`
	SubmittedAt time.Time       `json:"submitted_at"`
}

// PeerReview is the entity tracking bidirectional review between two agents.
type PeerReview struct {
	ID              PeerReviewID      `json:"id"`
	Status          PeerReviewStatus  `json:"status"`
	QuestID         QuestID           `json:"quest_id"`
	PartyID         *PartyID          `json:"party_id,omitempty"`
	LeaderID        AgentID           `json:"leader_id"`
	MemberID        AgentID           `json:"member_id"`
	IsSoloTask      bool              `json:"is_solo_task"`
	LeaderReview    *ReviewSubmission `json:"leader_review,omitempty"`
	MemberReview    *ReviewSubmission `json:"member_review,omitempty"`
	LeaderAvgRating float64           `json:"leader_avg_rating"`
	MemberAvgRating float64           `json:"member_avg_rating"`
	CreatedAt       time.Time         `json:"created_at"`
	CompletedAt     *time.Time        `json:"completed_at,omitempty"`
}

// LeaderToMemberQuestions are the review questions for leader reviewing member.
var LeaderToMemberQuestions = [3]string{
	"Task quality — did the deliverable meet acceptance criteria?",
	"Communication — were blockers surfaced promptly?",
	"Autonomy — did they work independently without excessive hand-holding?",
}

// MemberToLeaderQuestions are the review questions for member reviewing leader.
var MemberToLeaderQuestions = [3]string{
	"Clarity — was the task well-defined with clear acceptance criteria?",
	"Support — were blockers unblocked promptly?",
	"Fairness — was the task appropriate for my level/skills?",
}
