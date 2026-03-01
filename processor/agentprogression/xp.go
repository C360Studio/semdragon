// Package agent_progression implements XP calculation and agent progression.
package agentprogression

import (
	"fmt"
	"math"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/questboard"
)

// =============================================================================
// XP ENGINE INTERFACE
// =============================================================================

// XPEngine calculates XP awards and penalties.
type XPEngine interface {
	// CalculateXP returns the XP breakdown for a completed quest.
	CalculateXP(ctx XPContext) XPAward

	// CalculatePenalty returns the penalty for a failed quest.
	CalculatePenalty(ctx PenaltyContext) XPPenalty

	// ApplyXP applies XP change to agent and returns level change details.
	ApplyXP(agent *Agent, xpDelta int64) LevelEvent
}

// XPContext provides context for XP calculation.
type XPContext struct {
	Quest        questboard.Quest          `json:"quest"`
	Agent        Agent                     `json:"agent"`
	BattleResult *questboard.BattleVerdict `json:"battle_result,omitempty"`
	Duration     time.Duration             `json:"duration"`
	Streak       int                       `json:"streak"`
	IsGuildQuest bool                      `json:"is_guild_quest"`
	GuildRank    domain.GuildRank          `json:"guild_rank,omitempty"`
	Attempt      int                       `json:"attempt"`
}

// PenaltyContext provides context for penalty calculation.
type PenaltyContext struct {
	Quest       questboard.Quest `json:"quest"`
	Agent       Agent            `json:"agent"`
	FailureType FailureType      `json:"failure_type"`
	Attempt     int              `json:"attempt"`
}

// LevelEvent records a level change.
type LevelEvent struct {
	Direction string           `json:"direction"` // "up", "down", "none"
	OldLevel  int              `json:"old_level"`
	NewLevel  int              `json:"new_level"`
	OldTier   domain.TrustTier `json:"old_tier"`
	NewTier   domain.TrustTier `json:"new_tier"`
}

// =============================================================================
// DEFAULT XP ENGINE
// =============================================================================

// DefaultXPEngine implements XPEngine with standard formulas.
type DefaultXPEngine struct {
	QualityMultiplier  float64 `json:"quality_multiplier"`
	SpeedMultiplier    float64 `json:"speed_multiplier"`
	StreakMultiplier   float64 `json:"streak_multiplier"`
	GuildBonusRate     float64 `json:"guild_bonus_rate"`
	RetryPenaltyRate   float64 `json:"retry_penalty_rate"`
	FailurePenaltyRate float64 `json:"failure_penalty_rate"`
	LevelDownThreshold int     `json:"level_down_threshold"`
}

// DefaultXPEngineConfig returns sensible defaults.
func DefaultXPEngineConfig() DefaultXPEngine {
	return DefaultXPEngine{
		QualityMultiplier:  2.0,
		SpeedMultiplier:    0.5,
		StreakMultiplier:   0.1,
		GuildBonusRate:     0.15,
		RetryPenaltyRate:   0.25,
		FailurePenaltyRate: 0.5,
		LevelDownThreshold: 3,
	}
}

// NewDefaultXPEngine creates a new XP engine with default settings.
func NewDefaultXPEngine() *DefaultXPEngine {
	e := DefaultXPEngineConfig()
	return &e
}

// CalculateXP calculates XP for a completed quest.
func (e *DefaultXPEngine) CalculateXP(ctx XPContext) XPAward {
	award := XPAward{
		BaseXP: ctx.Quest.BaseXP,
	}

	// Quality bonus from boss battle
	if ctx.BattleResult != nil && ctx.BattleResult.Passed {
		award.QualityBonus = int64(float64(ctx.Quest.BaseXP) * ctx.BattleResult.QualityScore * e.QualityMultiplier)
	}

	// Speed bonus - completing under expected time
	expectedDur := expectedDuration(ctx.Quest.Difficulty)
	if ctx.Duration > 0 && ctx.Duration < expectedDur {
		speedRatio := float64(expectedDur-ctx.Duration) / float64(expectedDur)
		award.SpeedBonus = int64(float64(ctx.Quest.BaseXP) * speedRatio * e.SpeedMultiplier)
	}

	// Streak bonus
	if ctx.Streak > 1 {
		award.StreakBonus = int64(float64(ctx.Quest.BaseXP) * float64(ctx.Streak-1) * e.StreakMultiplier)
		if award.StreakBonus > ctx.Quest.BaseXP { // Cap at 100% base
			award.StreakBonus = ctx.Quest.BaseXP
		}
	}

	// Guild bonus
	if ctx.IsGuildQuest {
		guildMultiplier := guildRankMultiplier(ctx.GuildRank)
		award.GuildBonus = int64(float64(ctx.Quest.GuildXP) * guildMultiplier * e.GuildBonusRate)
	}

	// Attempt penalty (retries reduce reward)
	if ctx.Attempt > 1 {
		retryPenalty := float64(ctx.Attempt-1) * e.RetryPenaltyRate
		if retryPenalty > 0.75 { // Cap at 75% reduction
			retryPenalty = 0.75
		}
		award.AttemptPenalty = int64(float64(award.BaseXP+award.QualityBonus) * retryPenalty)
	}

	// Calculate total
	award.TotalXP = award.BaseXP + award.QualityBonus + award.SpeedBonus + award.StreakBonus + award.GuildBonus - award.AttemptPenalty
	if award.TotalXP < 0 {
		award.TotalXP = 0
	}

	// Build breakdown description
	award.Breakdown = e.buildBreakdown(award)

	return award
}

// CalculatePenalty calculates the penalty for quest failure.
func (e *DefaultXPEngine) CalculatePenalty(ctx PenaltyContext) XPPenalty {
	penalty := XPPenalty{}

	// Base XP loss scaled by failure type
	baseLoss := float64(ctx.Quest.BaseXP) * e.FailurePenaltyRate

	switch ctx.FailureType {
	case FailureSoft:
		penalty.XPLost = int64(baseLoss * 0.25)
		penalty.CooldownDur = 1 * time.Minute
	case FailureTimeout:
		penalty.XPLost = int64(baseLoss * 0.5)
		penalty.CooldownDur = 5 * time.Minute
	case FailureAbandon:
		penalty.XPLost = int64(baseLoss * 0.75)
		penalty.CooldownDur = 15 * time.Minute
	case FailureCatastrophic:
		penalty.XPLost = int64(baseLoss * 2.0)
		penalty.CooldownDur = 1 * time.Hour
		// Repeated catastrophic failures can cause permadeath
		if ctx.Agent.Stats.QuestsFailed > 0 &&
			ctx.Agent.Stats.QuestsFailed%(e.LevelDownThreshold*3) == 0 {
			penalty.Permadeath = true
		}
		penalty.LevelLoss = true
	}

	// Scale by difficulty
	difficultyScale := 1.0 + float64(ctx.Quest.Difficulty)*0.2
	penalty.XPLost = int64(float64(penalty.XPLost) * difficultyScale)

	penalty.Reason = fmt.Sprintf("%s failure on %s quest", ctx.FailureType, ctx.Quest.Title)

	return penalty
}

// ApplyXP applies XP change and handles leveling.
func (e *DefaultXPEngine) ApplyXP(agent *Agent, xpDelta int64) LevelEvent {
	event := LevelEvent{
		Direction: "none",
		OldLevel:  agent.Level,
		NewLevel:  agent.Level,
		OldTier:   agent.Tier,
		NewTier:   agent.Tier,
	}

	agent.XP += xpDelta
	if agent.XP < 0 {
		agent.XP = 0
	}

	// Check for level up
	for agent.XP >= agent.XPToLevel && agent.Level < 20 {
		agent.XP -= agent.XPToLevel
		agent.Level++
		agent.XPToLevel = xpForLevel(agent.Level + 1)
		event.Direction = "up"
		event.NewLevel = agent.Level
	}

	// Check for level down (only if XP is negative enough)
	for agent.XP < 0 && agent.Level > 1 {
		agent.Level--
		agent.XPToLevel = xpForLevel(agent.Level + 1)
		agent.XP = agent.XPToLevel - 1 // Start near top of new level
		event.Direction = "down"
		event.NewLevel = agent.Level
	}

	// Update tier based on new level
	agent.Tier = domain.TierFromLevel(agent.Level)
	event.NewTier = agent.Tier

	return event
}

// buildBreakdown creates a human-readable breakdown of XP calculation.
func (e *DefaultXPEngine) buildBreakdown(award XPAward) string {
	parts := []string{fmt.Sprintf("Base: %d", award.BaseXP)}
	if award.QualityBonus > 0 {
		parts = append(parts, fmt.Sprintf("Quality: +%d", award.QualityBonus))
	}
	if award.SpeedBonus > 0 {
		parts = append(parts, fmt.Sprintf("Speed: +%d", award.SpeedBonus))
	}
	if award.StreakBonus > 0 {
		parts = append(parts, fmt.Sprintf("Streak: +%d", award.StreakBonus))
	}
	if award.GuildBonus > 0 {
		parts = append(parts, fmt.Sprintf("Guild: +%d", award.GuildBonus))
	}
	if award.AttemptPenalty > 0 {
		parts = append(parts, fmt.Sprintf("Retry: -%d", award.AttemptPenalty))
	}
	return fmt.Sprintf("%v = %d XP", parts, award.TotalXP)
}

// Ensure DefaultXPEngine implements XPEngine.
var _ XPEngine = (*DefaultXPEngine)(nil)

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

// xpForLevel returns XP required to reach a given level.
func xpForLevel(level int) int64 {
	// Exponential curve: each level requires ~50% more XP
	return int64(100 * math.Pow(1.5, float64(level-1)))
}

// expectedDuration returns expected completion time based on difficulty.
func expectedDuration(difficulty domain.QuestDifficulty) time.Duration {
	switch difficulty {
	case domain.DifficultyTrivial:
		return 5 * time.Minute
	case domain.DifficultyEasy:
		return 15 * time.Minute
	case domain.DifficultyModerate:
		return 30 * time.Minute
	case domain.DifficultyHard:
		return 1 * time.Hour
	case domain.DifficultyEpic:
		return 2 * time.Hour
	case domain.DifficultyLegendary:
		return 4 * time.Hour
	default:
		return 30 * time.Minute
	}
}

// guildRankMultiplier returns XP bonus multiplier for guild rank.
func guildRankMultiplier(rank domain.GuildRank) float64 {
	switch rank {
	case domain.GuildRankInitiate:
		return 0.75
	case domain.GuildRankMember:
		return 1.0
	case domain.GuildRankVeteran:
		return 1.25
	case domain.GuildRankOfficer:
		return 1.5
	case domain.GuildRankMaster:
		return 2.0
	default:
		return 1.0
	}
}

// DefaultXPForDifficulty returns base XP for quest difficulty.
func DefaultXPForDifficulty(difficulty domain.QuestDifficulty) int64 {
	switch difficulty {
	case domain.DifficultyTrivial:
		return 10
	case domain.DifficultyEasy:
		return 25
	case domain.DifficultyModerate:
		return 50
	case domain.DifficultyHard:
		return 100
	case domain.DifficultyEpic:
		return 200
	case domain.DifficultyLegendary:
		return 500
	default:
		return 25
	}
}

// TierFromDifficulty returns the minimum trust tier for a difficulty level.
func TierFromDifficulty(difficulty domain.QuestDifficulty) domain.TrustTier {
	switch difficulty {
	case domain.DifficultyTrivial:
		return domain.TierApprentice
	case domain.DifficultyEasy:
		return domain.TierApprentice
	case domain.DifficultyModerate:
		return domain.TierJourneyman
	case domain.DifficultyHard:
		return domain.TierExpert
	case domain.DifficultyEpic:
		return domain.TierMaster
	case domain.DifficultyLegendary:
		return domain.TierGrandmaster
	default:
		return domain.TierApprentice
	}
}
