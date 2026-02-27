package semdragons

import (
	"math"
	"time"
)

// =============================================================================
// XP ENGINE - The evaluation framework wearing a cloak
// =============================================================================
// XP = f(quest_difficulty, quality_score, efficiency, streak, guild_bonus)
//
// This is the hardest part to get right. The XP function IS your evaluation
// framework. Get this wrong and agents game the system or stagnate.
// =============================================================================

// XPEngine calculates experience points and manages leveling.
type XPEngine interface {
	// CalculateXP computes XP earned from a quest completion + boss battle result.
	CalculateXP(ctx XPContext) XPAward

	// CalculatePenalty computes XP loss from failure.
	CalculatePenalty(ctx PenaltyContext) XPPenalty

	// ApplyXP adds/removes XP from an agent and handles level transitions.
	ApplyXP(agent *Agent, delta int64) LevelEvent

	// XPToNextLevel returns XP required for the next level.
	XPToNextLevel(currentLevel int) int64

	// CheckLevelDown determines if an agent should lose a level due to poor performance.
	CheckLevelDown(agent *Agent) *LevelEvent
}

// XPContext contains everything needed to calculate XP for a quest completion.
type XPContext struct {
	Quest        Quest         `json:"quest"`
	Agent        Agent         `json:"agent"`
	BattleResult BattleVerdict `json:"battle_result"`
	Duration     time.Duration `json:"duration"`      // How long it took
	EstDuration  time.Duration `json:"est_duration"`  // How long it should have taken
	Streak       int           `json:"streak"`        // Consecutive successes
	IsGuildQuest bool          `json:"is_guild_quest"`
	Attempt      int           `json:"attempt"`       // Which attempt (1st, 2nd, etc.)
}

// XPAward holds the breakdown of XP earned from a quest completion.
type XPAward struct {
	BaseXP         int64  `json:"base_xp"`
	QualityBonus   int64  `json:"quality_bonus"`
	SpeedBonus     int64  `json:"speed_bonus"`
	StreakBonus    int64  `json:"streak_bonus"`
	GuildBonus     int64  `json:"guild_bonus"`
	AttemptPenalty int64  `json:"attempt_penalty"`
	TotalXP        int64  `json:"total_xp"`
	Breakdown      string `json:"breakdown"`
}

// PenaltyContext contains information needed to calculate XP penalties.
type PenaltyContext struct {
	Quest       Quest       `json:"quest"`
	Agent       Agent       `json:"agent"`
	FailureType FailureType `json:"failure_type"`
	Attempt     int         `json:"attempt"`
}

// FailureType categorizes quest failures for penalty calculation.
type FailureType string

// Failure type constants.
const (
	// FailureSoft indicates bad output that can be retried.
	FailureSoft FailureType = "soft"
	// FailureTimeout indicates the quest took too long.
	FailureTimeout FailureType = "timeout"
	// FailureAbandon indicates the agent gave up.
	FailureAbandon FailureType = "abandon"
	// FailureCatastrophic indicates data loss, security breach, etc.
	FailureCatastrophic FailureType = "catastrophic"
)

// XPPenalty holds the consequences of a quest failure.
type XPPenalty struct {
	XPLost      int64         `json:"xp_lost"`
	CooldownDur time.Duration `json:"cooldown_duration"`
	LevelLoss   bool          `json:"level_loss"`
	Permadeath  bool          `json:"permadeath"`
	Reason      string        `json:"reason"`
}

// LevelEvent records a level change for an agent.
type LevelEvent struct {
	AgentID   AgentID   `json:"agent_id"`
	OldLevel  int       `json:"old_level"`
	NewLevel  int       `json:"new_level"`
	OldTier   TrustTier `json:"old_tier"`
	NewTier   TrustTier `json:"new_tier"`
	Direction string    `json:"direction"`
	XPCurrent int64     `json:"xp_current"`
	XPNeeded  int64     `json:"xp_needed"`
}

// =============================================================================
// DEFAULT XP ENGINE - A reasonable starting implementation
// =============================================================================

// DefaultXPEngine is the standard XP calculation implementation with tunable parameters.
type DefaultXPEngine struct {
	QualityMultiplier  float64
	SpeedMultiplier    float64
	StreakMultiplier   float64
	GuildBonusRate     float64
	RetryPenaltyRate   float64
	FailurePenaltyRate float64
	LevelDownThreshold int
}

// NewDefaultXPEngine creates an XP engine with sensible defaults.
func NewDefaultXPEngine() *DefaultXPEngine {
	return &DefaultXPEngine{
		QualityMultiplier:  2.0,
		SpeedMultiplier:    0.5,
		StreakMultiplier:    0.1,
		GuildBonusRate:     0.15,
		RetryPenaltyRate:   0.25,
		FailurePenaltyRate: 0.5,
		LevelDownThreshold: 3,
	}
}

// CalculateXP computes XP earned from a quest completion with all bonuses.
func (e *DefaultXPEngine) CalculateXP(ctx XPContext) XPAward {
	base := ctx.Quest.BaseXP

	// Quality bonus: quality_score * multiplier * base
	// A perfect quality score doubles the XP (at default multiplier)
	qualityBonus := int64(ctx.BattleResult.QualityScore * e.QualityMultiplier * float64(base))

	// Speed bonus: if faster than estimate, bonus proportional to time saved
	var speedBonus int64
	if ctx.EstDuration > 0 && ctx.Duration < ctx.EstDuration {
		timeSaved := float64(ctx.EstDuration-ctx.Duration) / float64(ctx.EstDuration)
		speedBonus = int64(timeSaved * e.SpeedMultiplier * float64(base))
	}

	// Streak bonus: 10% per consecutive success, capped at 100%
	streakMult := math.Min(float64(ctx.Streak)*e.StreakMultiplier, 1.0)
	streakBonus := int64(streakMult * float64(base))

	// Guild bonus: flat 15% for guild quests
	var guildBonus int64
	if ctx.IsGuildQuest {
		guildBonus = int64(e.GuildBonusRate * float64(base))
	}

	// Retry penalty: 25% less per retry attempt
	attemptPenalty := int64(float64(ctx.Attempt-1) * e.RetryPenaltyRate * float64(base))

	total := base + qualityBonus + speedBonus + streakBonus + guildBonus - attemptPenalty
	if total < 1 {
		total = 1 // Always earn at least 1 XP for completing
	}

	return XPAward{
		BaseXP:         base,
		QualityBonus:   qualityBonus,
		SpeedBonus:     speedBonus,
		StreakBonus:    streakBonus,
		GuildBonus:     guildBonus,
		AttemptPenalty: attemptPenalty,
		TotalXP:        total,
	}
}

// CalculatePenalty computes XP loss and cooldown based on failure type.
func (e *DefaultXPEngine) CalculatePenalty(ctx PenaltyContext) XPPenalty {
	base := ctx.Quest.BaseXP

	switch ctx.FailureType {
	case FailureCatastrophic:
		return XPPenalty{
			XPLost:      base * 5,
			CooldownDur: 0, // No cooldown - permadeath
			LevelLoss:   true,
			Permadeath:  true,
			Reason:      "Catastrophic failure - agent retired",
		}

	case FailureAbandon:
		return XPPenalty{
			XPLost:      int64(float64(base) * e.FailurePenaltyRate * 1.5),
			CooldownDur: 10 * time.Minute,
			Reason:      "Quest abandoned - extended cooldown",
		}

	case FailureTimeout:
		return XPPenalty{
			XPLost:      int64(float64(base) * e.FailurePenaltyRate),
			CooldownDur: 5 * time.Minute,
			Reason:      "Quest timed out",
		}

	default: // FailureSoft
		return XPPenalty{
			XPLost:      int64(float64(base) * e.FailurePenaltyRate * 0.5),
			CooldownDur: 2 * time.Minute,
			Reason:      "Quest output rejected by review",
		}
	}
}

// ApplyXP adds or removes XP from an agent and handles level transitions.
func (e *DefaultXPEngine) ApplyXP(agent *Agent, delta int64) LevelEvent {
	event := LevelEvent{
		AgentID:   agent.ID,
		OldLevel:  agent.Level,
		OldTier:   agent.Tier,
		Direction: "none",
	}

	agent.XP += delta
	if agent.XP < 0 {
		agent.XP = 0
	}

	// Check level up
	for agent.XP >= agent.XPToLevel && agent.Level < 20 {
		agent.XP -= agent.XPToLevel
		agent.Level++
		agent.XPToLevel = e.XPToNextLevel(agent.Level)
		event.Direction = "up"
	}

	// Check level down (XP can't go below 0, but consecutive failures trigger this)
	// Level down is handled separately via CheckLevelDown

	event.NewLevel = agent.Level
	event.NewTier = TierFromLevel(agent.Level)
	event.XPCurrent = agent.XP
	event.XPNeeded = agent.XPToLevel

	// Update agent tier
	agent.Tier = event.NewTier

	return event
}

// XPToNextLevel uses a gentle exponential curve.
// Level 1->2: 100 XP, Level 19->20: ~5000 XP
func (e *DefaultXPEngine) XPToNextLevel(currentLevel int) int64 {
	// base * (level ^ 1.5) gives a nice curve
	return int64(100.0 * math.Pow(float64(currentLevel), 1.5))
}

// CheckLevelDown determines if an agent should lose a level due to poor performance.
func (e *DefaultXPEngine) CheckLevelDown(agent *Agent) *LevelEvent {
	// If agent has N consecutive failures (tracked externally), drop a level
	// This is called by the DM, not automatically
	if agent.Level <= 1 {
		return nil
	}

	recentFailRate := float64(agent.Stats.BossesFailed) / math.Max(float64(agent.Stats.BossesDefeated+agent.Stats.BossesFailed), 1.0)

	// If failing more than 60% of boss battles at current level, consider demotion
	if recentFailRate > 0.6 && (agent.Stats.BossesDefeated+agent.Stats.BossesFailed) >= e.LevelDownThreshold {
		agent.Level--
		agent.Tier = TierFromLevel(agent.Level)
		agent.XP = 0 // Reset XP at new level
		agent.XPToLevel = e.XPToNextLevel(agent.Level)

		return &LevelEvent{
			AgentID:   agent.ID,
			OldLevel:  agent.Level + 1,
			NewLevel:  agent.Level,
			OldTier:   TierFromLevel(agent.Level + 1),
			NewTier:   agent.Tier,
			Direction: "down",
			XPCurrent: 0,
			XPNeeded:  agent.XPToLevel,
		}
	}

	return nil
}
