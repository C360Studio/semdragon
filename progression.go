package semdragons

import (
	"context"
	"log/slog"
	"time"
)

// =============================================================================
// PROGRESSION MANAGER - XP and Level Handling
// =============================================================================
// The ProgressionManager bridges battle evaluation to agent progression.
// It:
// 1. Loads agent state from storage
// 2. Gets streak count for bonus calculation
// 3. Calls XPEngine for XP calculation
// 4. Updates agent with new XP/level
// 5. Persists changes and emits events
// =============================================================================

// ProgressionManager handles XP awards, penalties, and level transitions.
type ProgressionManager struct {
	storage  *Storage
	xpEngine XPEngine
	events   *EventPublisher
	logger   *slog.Logger
}

// NewProgressionManager creates a new progression manager.
func NewProgressionManager(storage *Storage, xpEngine XPEngine, events *EventPublisher) *ProgressionManager {
	return &ProgressionManager{
		storage:  storage,
		xpEngine: xpEngine,
		events:   events,
		logger:   slog.Default(),
	}
}

// WithLogger sets a custom logger for the progression manager.
func (pm *ProgressionManager) WithLogger(l *slog.Logger) *ProgressionManager {
	pm.logger = l
	return pm
}

// ProgressionContext contains everything needed for progression processing.
type ProgressionContext struct {
	Quest      Quest         `json:"quest"`
	AgentID    AgentID       `json:"agent_id"`
	Verdict    BattleVerdict `json:"verdict"`
	Duration   time.Duration `json:"duration"`
	FailType   FailureType   `json:"fail_type,omitempty"`
	IsGuildQuest bool        `json:"is_guild_quest"`
}

// ProgressionResult holds the outcome of progression processing.
type ProgressionResult struct {
	Award       *XPAward    `json:"award,omitempty"`
	Penalty     *XPPenalty  `json:"penalty,omitempty"`
	LevelEvent  *LevelEvent `json:"level_event,omitempty"`
	XPBefore    int64       `json:"xp_before"`
	XPAfter     int64       `json:"xp_after"`
	LevelBefore int         `json:"level_before"`
	LevelAfter  int         `json:"level_after"`
	Streak      int         `json:"streak"`
}

// ProcessSuccess handles XP award and level up on quest success.
func (pm *ProgressionManager) ProcessSuccess(ctx context.Context, pctx ProgressionContext) (*ProgressionResult, error) {
	agentInstance := ExtractInstance(string(pctx.AgentID))

	// Get and update streak atomically
	streak, err := pm.storage.IncrementAgentStreak(ctx, agentInstance)
	if err != nil {
		streak = 1 // Default if streak tracking fails
	}

	// Variables to capture during atomic update
	var (
		award       XPAward
		levelEvent  LevelEvent
		xpBefore    int64
		levelBefore int
		xpAfter     int64
		levelAfter  int
		xpToLevel   int64
	)

	// Atomically update agent state
	err = pm.storage.UpdateAgent(ctx, agentInstance, func(agent *Agent) error {
		// Capture before state
		xpBefore = agent.XP
		levelBefore = agent.Level

		// Build XP context with current agent state
		xpCtx := XPContext{
			Quest:        pctx.Quest,
			Agent:        *agent,
			BattleResult: pctx.Verdict,
			Duration:     pctx.Duration,
			Streak:       streak,
			IsGuildQuest: pctx.IsGuildQuest,
			Attempt:      pctx.Quest.Attempts,
		}

		// Calculate XP award
		award = pm.xpEngine.CalculateXP(xpCtx)

		// Apply XP (mutates agent)
		levelEvent = pm.xpEngine.ApplyXP(agent, award.TotalXP)

		// Update agent stats
		agent.Stats.QuestsCompleted++
		agent.Stats.BossesDefeated++
		agent.Stats.TotalXPEarned += award.TotalXP
		agent.UpdatedAt = time.Now()

		// Capture after state for result
		xpAfter = agent.XP
		levelAfter = agent.Level
		xpToLevel = agent.XPToLevel

		return nil
	})
	if err != nil {
		return nil, err
	}

	result := &ProgressionResult{
		Award:       &award,
		XPBefore:    xpBefore,
		XPAfter:     xpAfter,
		LevelBefore: levelBefore,
		LevelAfter:  levelAfter,
		Streak:      streak,
	}

	// Emit XP event
	if pm.events != nil {
		pm.events.PublishAgentXP(ctx, AgentXPPayload{
			AgentID:     pctx.AgentID,
			QuestID:     pctx.Quest.ID,
			Award:       &award,
			XPDelta:     award.TotalXP,
			XPBefore:    xpBefore,
			XPAfter:     xpAfter,
			LevelBefore: levelBefore,
			LevelAfter:  levelAfter,
			Timestamp:   time.Now(),
		})

		// Emit level up event if applicable
		if levelEvent.Direction == "up" {
			result.LevelEvent = &levelEvent
			pm.events.PublishAgentLevelUp(ctx, AgentLevelPayload{
				AgentID:   pctx.AgentID,
				QuestID:   pctx.Quest.ID,
				OldLevel:  levelEvent.OldLevel,
				NewLevel:  levelEvent.NewLevel,
				OldTier:   levelEvent.OldTier,
				NewTier:   levelEvent.NewTier,
				XPCurrent: xpAfter,
				XPToLevel: xpToLevel,
				Timestamp: time.Now(),
			})
		}
	}

	return result, nil
}

// ProcessFailure handles XP penalty and cooldown on quest failure.
func (pm *ProgressionManager) ProcessFailure(ctx context.Context, pctx ProgressionContext) (*ProgressionResult, error) {
	agentInstance := ExtractInstance(string(pctx.AgentID))

	// Reset streak on failure - not critical, will be reset on next success anyway
	if err := pm.storage.ResetAgentStreak(ctx, agentInstance); err != nil {
		pm.logger.Debug("failed to reset agent streak", "agent", pctx.AgentID, "error", err)
	}

	// Variables to capture during atomic update
	var (
		penalty        XPPenalty
		levelDownEvent *LevelEvent
		xpBefore       int64
		levelBefore    int
		xpAfter        int64
		levelAfter     int
		xpToLevel      int64
		cooldownUntil  *time.Time
	)

	// Atomically update agent state
	err := pm.storage.UpdateAgent(ctx, agentInstance, func(agent *Agent) error {
		// Capture before state
		xpBefore = agent.XP
		levelBefore = agent.Level

		// Build penalty context with current agent state
		penaltyCtx := PenaltyContext{
			Quest:       pctx.Quest,
			Agent:       *agent,
			FailureType: pctx.FailType,
			Attempt:     pctx.Quest.Attempts,
		}

		// Calculate penalty
		penalty = pm.xpEngine.CalculatePenalty(penaltyCtx)

		// Apply XP penalty - ApplyXP handles XP reduction but won't level down automatically
		// Level downs are handled separately via CheckLevelDown based on failure patterns
		pm.xpEngine.ApplyXP(agent, -penalty.XPLost)

		// Update agent stats
		agent.Stats.QuestsFailed++
		agent.Stats.BossesFailed++

		// Handle cooldown
		if penalty.CooldownDur > 0 {
			cooldownEnd := time.Now().Add(penalty.CooldownDur)
			agent.CooldownUntil = &cooldownEnd
			agent.Status = AgentCooldown
			cooldownUntil = &cooldownEnd
		}

		// Handle permadeath
		if penalty.Permadeath {
			agent.Status = AgentRetired
			agent.DeathCount++
		}

		// Check for level down based on failure rate patterns, not just XP loss
		if penalty.LevelLoss && agent.Level > 1 {
			levelDownEvent = pm.xpEngine.CheckLevelDown(agent)
		}

		agent.UpdatedAt = time.Now()

		// Capture after state for result
		xpAfter = agent.XP
		levelAfter = agent.Level
		xpToLevel = agent.XPToLevel

		return nil
	})
	if err != nil {
		return nil, err
	}

	result := &ProgressionResult{
		Penalty:     &penalty,
		XPBefore:    xpBefore,
		XPAfter:     xpAfter,
		LevelBefore: levelBefore,
		LevelAfter:  levelAfter,
		Streak:      0, // Reset on failure
	}

	// Emit events
	if pm.events != nil {
		pm.events.PublishAgentXP(ctx, AgentXPPayload{
			AgentID:     pctx.AgentID,
			QuestID:     pctx.Quest.ID,
			Penalty:     &penalty,
			XPDelta:     -penalty.XPLost,
			XPBefore:    xpBefore,
			XPAfter:     xpAfter,
			LevelBefore: levelBefore,
			LevelAfter:  levelAfter,
			Timestamp:   time.Now(),
		})

		// Emit cooldown event
		if penalty.CooldownDur > 0 && cooldownUntil != nil {
			pm.events.PublishAgentCooldown(ctx, AgentCooldownPayload{
				AgentID:       pctx.AgentID,
				QuestID:       pctx.Quest.ID,
				FailType:      pctx.FailType,
				CooldownUntil: *cooldownUntil,
				Duration:      penalty.CooldownDur,
				Timestamp:     time.Now(),
			})
		}

		// Emit level down event if applicable
		if levelDownEvent != nil {
			result.LevelEvent = levelDownEvent
			pm.events.PublishAgentLevelDown(ctx, AgentLevelPayload{
				AgentID:   pctx.AgentID,
				QuestID:   pctx.Quest.ID,
				OldLevel:  levelDownEvent.OldLevel,
				NewLevel:  levelDownEvent.NewLevel,
				OldTier:   levelDownEvent.OldTier,
				NewTier:   levelDownEvent.NewTier,
				XPCurrent: xpAfter,
				XPToLevel: xpToLevel,
				Timestamp: time.Now(),
			})
		}
	}

	return result, nil
}

// GetAgentProgression returns current progression state for an agent.
func (pm *ProgressionManager) GetAgentProgression(ctx context.Context, agentID AgentID) (*AgentProgressionState, error) {
	agentInstance := ExtractInstance(string(agentID))

	agent, err := pm.storage.GetAgent(ctx, agentInstance)
	if err != nil {
		return nil, err
	}

	streak, err := pm.storage.GetAgentStreak(ctx, agentInstance)
	if err != nil {
		streak = 0
	}

	return &AgentProgressionState{
		AgentID:   agentID,
		Level:     agent.Level,
		XP:        agent.XP,
		XPToLevel: agent.XPToLevel,
		Tier:      agent.Tier,
		Streak:    streak,
		Stats:     agent.Stats,
	}, nil
}

// AgentProgressionState represents current progression for an agent.
type AgentProgressionState struct {
	AgentID   AgentID    `json:"agent_id"`
	Level     int        `json:"level"`
	XP        int64      `json:"xp"`
	XPToLevel int64      `json:"xp_to_level"`
	Tier      TrustTier  `json:"tier"`
	Streak    int        `json:"streak"`
	Stats     AgentStats `json:"stats"`
}
