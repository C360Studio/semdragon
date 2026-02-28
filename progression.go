package semdragons

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360studio/semstreams/natsclient"
)

// =============================================================================
// PROGRESSION MANAGER - XP and Level Handling
// =============================================================================
// The ProgressionManager bridges battle evaluation to agent progression.
// It:
// 1. Loads agent state from graph
// 2. Gets streak count for bonus calculation
// 3. Calls XPEngine for XP calculation
// 4. Updates agent with new XP/level
// 5. Emits changes to graph and publishes events
// =============================================================================

// ProgressionManager handles XP awards, penalties, and level transitions.
type ProgressionManager struct {
	graph       *GraphClient
	client      *natsclient.Client
	config      *BoardConfig
	xpEngine    XPEngine
	skillEngine *SkillProgressionEngine
	events      *EventPublisher
	logger      *slog.Logger
}

// NewProgressionManager creates a new progression manager.
func NewProgressionManager(graph *GraphClient, client *natsclient.Client, config *BoardConfig, xpEngine XPEngine, events *EventPublisher) *ProgressionManager {
	return &ProgressionManager{
		graph:       graph,
		client:      client,
		config:      config,
		xpEngine:    xpEngine,
		skillEngine: NewSkillProgressionEngine(),
		events:      events,
		logger:      slog.Default(),
	}
}

// WithSkillEngine sets a custom skill progression engine.
func (pm *ProgressionManager) WithSkillEngine(se *SkillProgressionEngine) *ProgressionManager {
	pm.skillEngine = se
	return pm
}

// WithLogger sets a custom logger for the progression manager.
func (pm *ProgressionManager) WithLogger(l *slog.Logger) *ProgressionManager {
	pm.logger = l
	return pm
}

// ProgressionContext contains everything needed for progression processing.
type ProgressionContext struct {
	Quest        Quest         `json:"quest"`
	AgentID      AgentID       `json:"agent_id"`
	Verdict      BattleVerdict `json:"verdict"`
	Duration     time.Duration `json:"duration"`
	FailType     FailureType   `json:"fail_type,omitempty"`
	IsGuildQuest bool          `json:"is_guild_quest"`
	IsMentored   bool          `json:"is_mentored"` // True if agent was in a mentored training party
}

// ProgressionResult holds the outcome of progression processing.
type ProgressionResult struct {
	Award             *XPAward                 `json:"award,omitempty"`
	Penalty           *XPPenalty               `json:"penalty,omitempty"`
	LevelEvent        *LevelEvent              `json:"level_event,omitempty"`
	SkillImprovements []SkillImprovementResult `json:"skill_improvements,omitempty"`
	XPBefore          int64                    `json:"xp_before"`
	XPAfter           int64                    `json:"xp_after"`
	LevelBefore       int                      `json:"level_before"`
	LevelAfter        int                      `json:"level_after"`
	Streak            int                      `json:"streak"`
}

// ProcessSuccess handles XP award and level up on quest success.
func (pm *ProgressionManager) ProcessSuccess(ctx context.Context, pctx ProgressionContext) (*ProgressionResult, error) {
	// Load agent from graph
	agent, err := pm.getAgent(ctx, pctx.AgentID)
	if err != nil {
		return nil, fmt.Errorf("load agent: %w", err)
	}

	// Get and update streak
	streak, err := pm.incrementStreak(ctx, pctx.AgentID)
	if err != nil {
		streak = 1 // Default if streak tracking fails
	}

	// Capture before state
	xpBefore := agent.XP
	levelBefore := agent.Level

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
	award := pm.xpEngine.CalculateXP(xpCtx)

	// Apply XP (mutates agent)
	levelEvent := pm.xpEngine.ApplyXP(agent, award.TotalXP)

	// Process skill improvements if engine is available
	var skillImprovements []SkillImprovementResult
	if pm.skillEngine != nil && len(pctx.Quest.RequiredSkills) > 0 {
		skillCtx := SkillProgressionContext{
			Agent:      agent,
			Quest:      &pctx.Quest,
			Quality:    pctx.Verdict.QualityScore,
			Duration:   pctx.Duration,
			IsMentored: pctx.IsMentored,
		}
		skillImprovements = pm.skillEngine.ProcessQuestCompletion(skillCtx)
	}

	// Update agent stats
	agent.Stats.QuestsCompleted++
	agent.Stats.BossesDefeated++
	agent.Stats.TotalXPEarned += award.TotalXP
	agent.UpdatedAt = time.Now()

	// Emit updated agent to graph
	if err := pm.graph.EmitEntityUpdate(ctx, agent, "agent.progression.xp"); err != nil {
		return nil, fmt.Errorf("emit agent update: %w", err)
	}

	result := &ProgressionResult{
		Award:             &award,
		SkillImprovements: skillImprovements,
		XPBefore:          xpBefore,
		XPAfter:           agent.XP,
		LevelBefore:       levelBefore,
		LevelAfter:        agent.Level,
		Streak:            streak,
	}

	// Emit XP event
	if pm.events != nil {
		pm.events.PublishAgentXP(ctx, AgentXPPayload{
			AgentID:     pctx.AgentID,
			QuestID:     pctx.Quest.ID,
			Award:       &award,
			XPDelta:     award.TotalXP,
			XPBefore:    xpBefore,
			XPAfter:     agent.XP,
			LevelBefore: levelBefore,
			LevelAfter:  agent.Level,
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
				XPCurrent: agent.XP,
				XPToLevel: agent.XPToLevel,
				Timestamp: time.Now(),
			})
		}

		// Emit skill progression events
		if len(skillImprovements) > 0 {
			pm.events.PublishSkillProgression(ctx, SkillProgressionPayload{
				AgentID:   pctx.AgentID,
				QuestID:   pctx.Quest.ID,
				Results:   skillImprovements,
				Timestamp: time.Now(),
			})

			// Emit individual skill level up events
			for _, improvement := range skillImprovements {
				if improvement.LeveledUp {
					pm.events.PublishSkillLevelUp(ctx, SkillLevelUpPayload{
						AgentID:   pctx.AgentID,
						QuestID:   pctx.Quest.ID,
						Skill:     improvement.Skill,
						OldLevel:  improvement.OldLevel,
						NewLevel:  improvement.NewLevel,
						Timestamp: time.Now(),
					})
				}
			}
		}
	}

	return result, nil
}

// ProcessFailure handles XP penalty and cooldown on quest failure.
func (pm *ProgressionManager) ProcessFailure(ctx context.Context, pctx ProgressionContext) (*ProgressionResult, error) {
	// Load agent from graph
	agent, err := pm.getAgent(ctx, pctx.AgentID)
	if err != nil {
		return nil, fmt.Errorf("load agent: %w", err)
	}

	// Reset streak on failure
	if err := pm.resetStreak(ctx, pctx.AgentID); err != nil {
		pm.logger.Debug("failed to reset agent streak", "agent", pctx.AgentID, "error", err)
	}

	// Capture before state
	xpBefore := agent.XP
	levelBefore := agent.Level

	// Build penalty context with current agent state
	penaltyCtx := PenaltyContext{
		Quest:       pctx.Quest,
		Agent:       *agent,
		FailureType: pctx.FailType,
		Attempt:     pctx.Quest.Attempts,
	}

	// Calculate penalty
	penalty := pm.xpEngine.CalculatePenalty(penaltyCtx)

	// Apply XP penalty
	pm.xpEngine.ApplyXP(agent, -penalty.XPLost)

	// Update agent stats
	agent.Stats.QuestsFailed++
	agent.Stats.BossesFailed++

	var cooldownUntil *time.Time
	var levelDownEvent *LevelEvent

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

	// Check for level down based on failure rate patterns
	if penalty.LevelLoss && agent.Level > 1 {
		levelDownEvent = pm.xpEngine.CheckLevelDown(agent)
	}

	agent.UpdatedAt = time.Now()

	// Emit updated agent to graph
	if err := pm.graph.EmitEntityUpdate(ctx, agent, "agent.progression.penalty"); err != nil {
		return nil, fmt.Errorf("emit agent update: %w", err)
	}

	result := &ProgressionResult{
		Penalty:     &penalty,
		XPBefore:    xpBefore,
		XPAfter:     agent.XP,
		LevelBefore: levelBefore,
		LevelAfter:  agent.Level,
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
			XPAfter:     agent.XP,
			LevelBefore: levelBefore,
			LevelAfter:  agent.Level,
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
				XPCurrent: agent.XP,
				XPToLevel: agent.XPToLevel,
				Timestamp: time.Now(),
			})
		}
	}

	return result, nil
}

// GetAgentProgression returns current progression state for an agent.
func (pm *ProgressionManager) GetAgentProgression(ctx context.Context, agentID AgentID) (*AgentProgressionState, error) {
	agent, err := pm.getAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}

	streak, err := pm.getStreak(ctx, agentID)
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

// =============================================================================
// HELPER METHODS
// =============================================================================

func (pm *ProgressionManager) getAgent(ctx context.Context, agentID AgentID) (*Agent, error) {
	entity, err := pm.graph.GetAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}
	return AgentFromEntityState(entity), nil
}

// Streak tracking uses KV directly since it's a simple counter
func (pm *ProgressionManager) streakKey(agentID AgentID) string {
	instance := ExtractInstance(string(agentID))
	return fmt.Sprintf("agent.streak.%s", instance)
}

func (pm *ProgressionManager) getStreak(ctx context.Context, agentID AgentID) (int, error) {
	bucket, err := pm.client.GetKeyValueBucket(ctx, pm.config.BucketName())
	if err != nil {
		return 0, err
	}

	entry, err := bucket.Get(ctx, pm.streakKey(agentID))
	if err != nil {
		return 0, nil // No streak yet
	}

	var streak int
	if err := json.Unmarshal(entry.Value(), &streak); err != nil {
		return 0, err
	}
	return streak, nil
}

func (pm *ProgressionManager) incrementStreak(ctx context.Context, agentID AgentID) (int, error) {
	current, _ := pm.getStreak(ctx, agentID)
	newStreak := current + 1

	bucket, err := pm.client.GetKeyValueBucket(ctx, pm.config.BucketName())
	if err != nil {
		return newStreak, err
	}

	data, _ := json.Marshal(newStreak)
	_, err = bucket.Put(ctx, pm.streakKey(agentID), data)
	return newStreak, err
}

func (pm *ProgressionManager) resetStreak(ctx context.Context, agentID AgentID) error {
	bucket, err := pm.client.GetKeyValueBucket(ctx, pm.config.BucketName())
	if err != nil {
		return err
	}

	data, _ := json.Marshal(0)
	_, err = bucket.Put(ctx, pm.streakKey(agentID), data)
	return err
}
