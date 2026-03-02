// Package domain provides shared primitive types for semdragons.
// Entity structs (Quest, Agent, etc.) live in their owning processors.
package domain

import (
	"time"
)

// =============================================================================
// ENTITY ID TYPES
// =============================================================================

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

// PeerReviewID uniquely identifies a peer review between two agents.
type PeerReviewID string

// =============================================================================
// AGENT PRIMITIVES
// =============================================================================

// AgentStatus represents the current state of an agent in the system.
type AgentStatus string

// Agent status values.
const (
	AgentIdle     AgentStatus = "idle"
	AgentOnQuest  AgentStatus = "on_quest"
	AgentInBattle AgentStatus = "in_battle"
	AgentCooldown AgentStatus = "cooldown"
	AgentRetired        AgentStatus = "retired"
	AgentPendingReview  AgentStatus = "pending_review"
)

// PeerReviewStatus represents the lifecycle state of a peer review.
type PeerReviewStatus string

// PeerReviewPending and related constants define peer review lifecycle states.
const (
	PeerReviewPending   PeerReviewStatus = "pending"
	PeerReviewPartial   PeerReviewStatus = "partial"
	PeerReviewCompleted PeerReviewStatus = "completed"
)

// ReviewDirection indicates which party is reviewing which.
type ReviewDirection string

// ReviewDirectionLeaderToMember and related constants define review direction values.
const (
	ReviewDirectionLeaderToMember ReviewDirection = "leader_to_member"
	ReviewDirectionMemberToLeader ReviewDirection = "member_to_leader"
	ReviewDirectionDMToAgent      ReviewDirection = "dm_to_agent"
)

// =============================================================================
// TRUST TIERS
// =============================================================================

// TrustTier represents an agent's trust level derived from their level.
type TrustTier int

// Trust tier levels derived from agent experience.
const (
	TierApprentice  TrustTier = iota // TierApprentice covers levels 1-5.
	TierJourneyman                   // TierJourneyman covers levels 6-10.
	TierExpert                       // TierExpert covers levels 11-15.
	TierMaster                       // TierMaster covers levels 16-18.
	TierGrandmaster                  // TierGrandmaster covers levels 19-20.
)

// String returns the lowercase name for the trust tier.
// These names are stable and match model registry capability key segments
// (e.g., "agent-work.expert" uses tier.String() == "expert").
func (t TrustTier) String() string {
	switch t {
	case TierApprentice:
		return "apprentice"
	case TierJourneyman:
		return "journeyman"
	case TierExpert:
		return "expert"
	case TierMaster:
		return "master"
	case TierGrandmaster:
		return "grandmaster"
	default:
		return "apprentice"
	}
}

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

// =============================================================================
// SKILL PRIMITIVES
// =============================================================================

// SkillTag represents a domain of competence.
type SkillTag string

// Common skill tags used to tag quests and agent capabilities.
const (
	SkillCodeGen       SkillTag = "code_generation"
	SkillCodeReview    SkillTag = "code_review"
	SkillDataTransform SkillTag = "data_transformation"
	SkillSummarization SkillTag = "summarization"
	SkillResearch      SkillTag = "research"
	SkillPlanning      SkillTag = "planning"
	SkillCustomerComms SkillTag = "customer_communications"
	SkillAnalysis      SkillTag = "analysis"
	SkillTraining      SkillTag = "training"
)

// ProficiencyLevel represents mastery level of a skill (1-5).
type ProficiencyLevel int

// Proficiency levels from beginner to mastery.
const (
	ProficiencyNovice     ProficiencyLevel = 1
	ProficiencyApprentice ProficiencyLevel = 2
	ProficiencyJourneyman ProficiencyLevel = 3
	ProficiencyExpert     ProficiencyLevel = 4
	ProficiencyMaster     ProficiencyLevel = 5
)

// proficiencyLevelNames maps levels to human-readable names.
var proficiencyLevelNames = map[ProficiencyLevel]string{
	ProficiencyNovice:     "Novice",
	ProficiencyApprentice: "Apprentice",
	ProficiencyJourneyman: "Journeyman",
	ProficiencyExpert:     "Expert",
	ProficiencyMaster:     "Master",
}

// ProficiencyLevelName returns the human-readable name for a proficiency level.
func ProficiencyLevelName(level ProficiencyLevel) string {
	return proficiencyLevelNames[level]
}

// SkillProficiency tracks an agent's mastery of a specific skill.
type SkillProficiency struct {
	Level      ProficiencyLevel `json:"level"`
	Progress   int              `json:"progress"`
	TotalXP    int64            `json:"total_xp"`
	QuestsUsed int              `json:"quests_used"`
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

// =============================================================================
// QUEST PRIMITIVES
// =============================================================================

// QuestStatus represents the lifecycle state of a quest.
type QuestStatus string

// Quest lifecycle status values.
const (
	QuestPosted     QuestStatus = "posted"
	QuestClaimed    QuestStatus = "claimed"
	QuestInProgress QuestStatus = "in_progress"
	QuestInReview   QuestStatus = "in_review"
	QuestCompleted  QuestStatus = "completed"
	QuestFailed     QuestStatus = "failed"
	QuestEscalated  QuestStatus = "escalated"
	QuestCancelled  QuestStatus = "cancelled"
)

// QuestDifficulty represents the challenge level of a quest.
type QuestDifficulty int

// Quest difficulty tiers matched to agent level ranges.
const (
	DifficultyTrivial   QuestDifficulty = iota // DifficultyTrivial suits levels 1-5.
	DifficultyEasy                             // DifficultyEasy suits levels 3-7.
	DifficultyModerate                         // DifficultyModerate suits levels 6-10.
	DifficultyHard                             // DifficultyHard suits levels 10-14.
	DifficultyEpic                             // DifficultyEpic suits levels 14-18.
	DifficultyLegendary                        // DifficultyLegendary suits levels 18-20 or requires a party.
)

// =============================================================================
// BOSS BATTLE PRIMITIVES
// =============================================================================

// ReviewLevel indicates the rigor of the boss battle review.
type ReviewLevel int

// Review rigor levels from automated to human-required.
const (
	ReviewAuto     ReviewLevel = iota // ReviewAuto uses automated checks only.
	ReviewStandard                    // ReviewStandard uses an LLM-as-judge.
	ReviewStrict                      // ReviewStrict uses a multi-judge panel.
	ReviewHuman                       // ReviewHuman requires a human reviewer.
)

// BattleStatus represents the state of a boss battle.
type BattleStatus string

// Boss battle outcome values.
const (
	BattleActive  BattleStatus = "active"
	BattleVictory BattleStatus = "victory"
	BattleDefeat  BattleStatus = "defeat"
	BattleRetreat BattleStatus = "retreat"
)

// JudgeType indicates the kind of judge.
type JudgeType string

// Judge kind values for boss battle evaluation.
const (
	JudgeAutomated JudgeType = "automated"
	JudgeLLM       JudgeType = "llm"
	JudgeHuman     JudgeType = "human"
)

// ReviewCriterion defines a single evaluation criterion.
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

// =============================================================================
// PARTY PRIMITIVES
// =============================================================================

// PartyStatus represents the current state of a party.
type PartyStatus string

// Party lifecycle status values.
const (
	PartyForming   PartyStatus = "forming"
	PartyActive    PartyStatus = "active"
	PartyDisbanded PartyStatus = "disbanded"
)

// PartyRole represents a member's role within a party.
type PartyRole string

// Party member role values.
const (
	RoleLead     PartyRole = "lead"
	RoleExecutor PartyRole = "executor"
	RoleReviewer PartyRole = "reviewer"
	RoleScout    PartyRole = "scout"
)

// =============================================================================
// GUILD PRIMITIVES
// =============================================================================

// GuildStatus represents the current state of a guild.
type GuildStatus string

// Guild lifecycle status values.
const (
	GuildActive   GuildStatus = "active"
	GuildInactive GuildStatus = "inactive"
)

// GuildRank represents a member's rank within a guild.
type GuildRank string

// Guild member rank values from lowest to highest.
const (
	GuildRankInitiate GuildRank = "initiate"
	GuildRankMember   GuildRank = "member"
	GuildRankVeteran  GuildRank = "veteran"
	GuildRankOfficer  GuildRank = "officer"
	GuildRankMaster   GuildRank = "guildmaster"
)

// GuildBonusRate returns the XP bonus rate for this guild rank.
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
		return 0.10
	}
}

// =============================================================================
// TIER PERMISSIONS
// =============================================================================

// TierPermissions defines what a trust tier is allowed to do.
type TierPermissions struct {
	CanClaimQuestTier QuestDifficulty
	CanLeadParty      bool
	CanDecomposeQuest bool
	CanSupervise      bool
	CanActAsDM        bool
}

// tierPerms maps each trust tier to its allowed permissions.
var tierPerms = map[TrustTier]TierPermissions{
	TierApprentice: {
		CanClaimQuestTier: DifficultyTrivial,
	},
	TierJourneyman: {
		CanClaimQuestTier: DifficultyModerate,
	},
	TierExpert: {
		CanClaimQuestTier: DifficultyHard,
	},
	TierMaster: {
		CanClaimQuestTier: DifficultyEpic,
		CanLeadParty:      true,
		CanDecomposeQuest: true,
		CanSupervise:      true,
	},
	TierGrandmaster: {
		CanClaimQuestTier: DifficultyLegendary,
		CanLeadParty:      true,
		CanDecomposeQuest: true,
		CanSupervise:      true,
		CanActAsDM:        true,
	},
}

// TierPermissionsFor returns the permissions for the given trust tier.
func TierPermissionsFor(tier TrustTier) TierPermissions {
	return tierPerms[tier]
}

// =============================================================================
// TOOL
// =============================================================================

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

// =============================================================================
// XP HELPERS
// =============================================================================

// DefaultXPForDifficulty returns base XP for quest difficulty.
func DefaultXPForDifficulty(difficulty QuestDifficulty) int64 {
	switch difficulty {
	case DifficultyTrivial:
		return 10
	case DifficultyEasy:
		return 25
	case DifficultyModerate:
		return 50
	case DifficultyHard:
		return 100
	case DifficultyEpic:
		return 200
	case DifficultyLegendary:
		return 500
	default:
		return 25
	}
}

// TierFromDifficulty returns the minimum trust tier for a difficulty level.
func TierFromDifficulty(difficulty QuestDifficulty) TrustTier {
	switch difficulty {
	case DifficultyTrivial:
		return TierApprentice
	case DifficultyEasy:
		return TierApprentice
	case DifficultyModerate:
		return TierJourneyman
	case DifficultyHard:
		return TierExpert
	case DifficultyEpic:
		return TierMaster
	case DifficultyLegendary:
		return TierGrandmaster
	default:
		return TierApprentice
	}
}
