// Package xpengine provides a native semstreams component for XP calculation and
// agent progression management. It reacts to quest completion and battle verdict
// events, calculates XP awards/penalties, and emits progression events.
package xpengine

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/internal/util"
)

// =============================================================================
// COMPONENT - XPEngine as native semstreams processor
// =============================================================================
// Implements Discoverable + LifecycleComponent interfaces.
// Subscribes to quest.lifecycle.completed and battle.review.victory events.
// Calculates XP awards and emits agent.progression.* events.
// =============================================================================

// Config holds the component configuration.
type Config struct {
	// BoardConfig contains org, platform, board for entity IDs and bucket naming.
	Org      string `json:"org" schema:"type:string,description:Organization namespace"`
	Platform string `json:"platform" schema:"type:string,description:Platform/environment name"`
	Board    string `json:"board" schema:"type:string,description:Quest board name"`

	// XP calculation multipliers
	QualityMultiplier  float64 `json:"quality_multiplier" schema:"type:float,description:Quality bonus multiplier"`
	SpeedMultiplier    float64 `json:"speed_multiplier" schema:"type:float,description:Speed bonus multiplier"`
	StreakMultiplier   float64 `json:"streak_multiplier" schema:"type:float,description:Streak bonus multiplier"`
	RetryPenaltyRate   float64 `json:"retry_penalty_rate" schema:"type:float,description:XP penalty per retry attempt"`
	FailurePenaltyRate float64 `json:"failure_penalty_rate" schema:"type:float,description:XP penalty for failures"`
	LevelDownThreshold int     `json:"level_down_threshold" schema:"type:int,description:Consecutive failures for demotion"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Org:                "default",
		Platform:           "local",
		Board:              "main",
		QualityMultiplier:  2.0,
		SpeedMultiplier:    0.5,
		StreakMultiplier:   0.1,
		RetryPenaltyRate:   0.25,
		FailurePenaltyRate: 0.5,
		LevelDownThreshold: 3,
	}
}

// ToBoardConfig converts component config to semdragons BoardConfig.
func (c *Config) ToBoardConfig() *semdragons.BoardConfig {
	return &semdragons.BoardConfig{
		Org:      c.Org,
		Platform: c.Platform,
		Board:    c.Board,
	}
}

// ToXPEngine creates the underlying XP engine with configured parameters.
func (c *Config) ToXPEngine() *semdragons.DefaultXPEngine {
	return &semdragons.DefaultXPEngine{
		QualityMultiplier:  c.QualityMultiplier,
		SpeedMultiplier:    c.SpeedMultiplier,
		StreakMultiplier:   c.StreakMultiplier,
		RetryPenaltyRate:   c.RetryPenaltyRate,
		FailurePenaltyRate: c.FailurePenaltyRate,
		LevelDownThreshold: c.LevelDownThreshold,
	}
}

// Component implements the XPEngine as a semstreams processor.
type Component struct {
	config      *Config
	deps        component.Dependencies
	storage     *semdragons.Storage
	events      *semdragons.EventPublisher
	xpEngine    semdragons.XPEngine
	logger      *slog.Logger
	boardConfig *semdragons.BoardConfig

	// Subscriptions
	completedSub *natsclient.Subscription
	// Note: Battle verdict handling could be added in the future via:
	// victorySub *natsclient.Subscription - for battle.review.victory events
	// defeatSub *natsclient.Subscription - for battle.review.defeat events

	// Internal state
	running  atomic.Bool
	mu       sync.RWMutex
	stopChan chan struct{}

	// Metrics
	messagesProcessed atomic.Uint64
	errorsCount       atomic.Int64
	lastActivity      atomic.Value // time.Time
	startTime         time.Time
}

// ensure Component implements the required interfaces.
var (
	_ component.Discoverable       = (*Component)(nil)
	_ component.LifecycleComponent = (*Component)(nil)
)

// =============================================================================
// DISCOVERABLE INTERFACE
// =============================================================================

// Meta returns basic component information.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "xpengine",
		Type:        "processor",
		Description: "XP calculation and agent progression management",
		Version:     "1.0.0",
	}
}

// InputPorts returns the ports this component accepts data on.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "quest-completed",
			Direction:   component.DirectionInput,
			Required:    true,
			Description: "Quest completion events triggering XP calculation",
			Config: &component.NATSPort{
				Subject: semdragons.PredicateQuestCompleted,
			},
		},
		{
			Name:        "battle-victory",
			Direction:   component.DirectionInput,
			Required:    false,
			Description: "Battle victory events for XP awards",
			Config: &component.NATSPort{
				Subject: semdragons.PredicateBattleVictory,
			},
		},
		{
			Name:        "battle-defeat",
			Direction:   component.DirectionInput,
			Required:    false,
			Description: "Battle defeat events for XP penalties",
			Config: &component.NATSPort{
				Subject: semdragons.PredicateBattleDefeat,
			},
		},
	}
}

// OutputPorts returns the ports this component produces data on.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "agent-xp",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "XP change events",
			Config: &component.JetStreamPort{
				StreamName:      "AGENT_EVENTS",
				Subjects:        []string{"agent.progression.>"},
				Storage:         "file",
				RetentionPolicy: "limits",
				RetentionDays:   30,
				Replicas:        1,
			},
		},
		{
			Name:        "agent-state",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "Agent state updates in KV",
			Config: &component.KVWritePort{
				Bucket: "", // Set dynamically from config
			},
		},
	}
}

// ConfigSchema returns the configuration schema for this component.
func (c *Component) ConfigSchema() component.ConfigSchema {
	return component.ConfigSchema{
		Properties: map[string]component.PropertySchema{
			"org": {
				Type:        "string",
				Description: "Organization namespace",
				Default:     "default",
				Category:    "basic",
			},
			"platform": {
				Type:        "string",
				Description: "Platform/environment name",
				Default:     "local",
				Category:    "basic",
			},
			"board": {
				Type:        "string",
				Description: "Quest board name",
				Default:     "main",
				Category:    "basic",
			},
			"quality_multiplier": {
				Type:        "float",
				Description: "Quality bonus multiplier (default 2.0)",
				Default:     2.0,
				Category:    "advanced",
			},
			"speed_multiplier": {
				Type:        "float",
				Description: "Speed bonus multiplier (default 0.5)",
				Default:     0.5,
				Category:    "advanced",
			},
			"streak_multiplier": {
				Type:        "float",
				Description: "Streak bonus multiplier (default 0.1)",
				Default:     0.1,
				Category:    "advanced",
			},
			"retry_penalty_rate": {
				Type:        "float",
				Description: "XP penalty rate per retry attempt",
				Default:     0.25,
				Category:    "advanced",
			},
			"failure_penalty_rate": {
				Type:        "float",
				Description: "XP penalty rate for failures",
				Default:     0.5,
				Category:    "advanced",
			},
			"level_down_threshold": {
				Type:        "int",
				Description: "Consecutive failures before level demotion",
				Default:     3,
				Minimum:     util.IntPtr(1),
				Category:    "advanced",
			},
		},
		Required: []string{"org", "platform", "board"},
	}
}

// Health returns current health status.
func (c *Component) Health() component.HealthStatus {
	status := component.HealthStatus{
		Healthy:    c.running.Load(),
		LastCheck:  time.Now(),
		ErrorCount: int(c.errorsCount.Load()),
		Uptime:     time.Since(c.startTime),
	}

	if c.running.Load() {
		status.Status = "running"
	} else {
		status.Status = "stopped"
	}

	if c.errorsCount.Load() > 0 {
		status.LastError = "errors encountered during processing"
	}

	return status
}

// DataFlow returns current data flow metrics.
func (c *Component) DataFlow() component.FlowMetrics {
	metrics := component.FlowMetrics{
		MessagesPerSecond: 0,
		BytesPerSecond:    0,
		ErrorRate:         0,
	}

	if lastTime, ok := c.lastActivity.Load().(time.Time); ok {
		metrics.LastActivity = lastTime
	}

	processed := c.messagesProcessed.Load()
	uptime := time.Since(c.startTime).Seconds()
	if uptime > 0 {
		metrics.MessagesPerSecond = float64(processed) / uptime
	}

	if processed > 0 {
		metrics.ErrorRate = float64(c.errorsCount.Load()) / float64(processed)
	}

	return metrics
}

// =============================================================================
// LIFECYCLE INTERFACE
// =============================================================================

// Initialize performs one-time setup. No I/O operations here.
func (c *Component) Initialize() error {
	if c.config == nil {
		return errors.New("config not set")
	}

	if c.deps.NATSClient == nil {
		return errors.New("NATS client required")
	}

	c.boardConfig = c.config.ToBoardConfig()
	c.xpEngine = c.config.ToXPEngine()
	c.stopChan = make(chan struct{})

	return nil
}

// Start begins component operation with the given context.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running.Load() {
		return errors.New("component already running")
	}

	// Create storage (KV bucket)
	storage, err := semdragons.CreateStorage(ctx, c.deps.NATSClient, c.boardConfig)
	if err != nil {
		return errs.Wrap(err, "XPEngine", "Start", "create storage")
	}
	c.storage = storage

	// Create event publisher
	c.events = semdragons.NewEventPublisher(c.deps.NATSClient)

	// Subscribe to quest completed events
	completedSub, err := semdragons.SubjectQuestCompleted.Subscribe(ctx, c.deps.NATSClient, c.handleQuestCompleted)
	if err != nil {
		return errs.Wrap(err, "XPEngine", "Start", "subscribe to quest.lifecycle.completed")
	}
	c.completedSub = completedSub

	c.startTime = time.Now()
	c.running.Store(true)
	c.lastActivity.Store(time.Now())

	c.logger.Info("xpengine component started",
		"org", c.config.Org,
		"platform", c.config.Platform,
		"board", c.config.Board)

	return nil
}

// Stop gracefully shuts down the component.
// The timeout parameter is part of the LifecycleComponent interface but is not
// currently used as unsubscription is synchronous.
func (c *Component) Stop(_ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running.Load() {
		return nil
	}

	// Close stop channel
	close(c.stopChan)

	// Unsubscribe from events
	if c.completedSub != nil {
		c.completedSub.Unsubscribe()
	}

	c.running.Store(false)
	c.logger.Info("xpengine component stopped")

	return nil
}

// =============================================================================
// EVENT HANDLERS
// =============================================================================

// handleQuestCompleted processes quest completion events and awards XP.
// The typed Subject.Subscribe handles unmarshaling, so we receive the payload directly.
func (c *Component) handleQuestCompleted(ctx context.Context, payload semdragons.QuestCompletedPayload) error {
	if !c.running.Load() {
		return nil
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	// Skip if no agent (shouldn't happen, but be defensive)
	if payload.AgentID == "" {
		return nil
	}

	// Load agent
	agentInstance := semdragons.ExtractInstance(string(payload.AgentID))
	agent, err := c.storage.GetAgent(ctx, agentInstance)
	if err != nil {
		c.errorsCount.Add(1)
		c.logger.Error("failed to load agent", "agent", payload.AgentID, "error", err)
		return nil // Don't return error to avoid NATS redelivery for data issues
	}

	// Get streak
	streak, _ := c.storage.GetAgentStreak(ctx, agentInstance)
	streak++ // Increment for this success

	// Determine if this is a guild quest
	isGuildQuest := payload.Quest.GuildPriority != nil
	var guildRank semdragons.GuildRank
	if isGuildQuest && len(agent.Guilds) > 0 {
		// Get rank in the priority guild
		guildInstance := semdragons.ExtractInstance(string(*payload.Quest.GuildPriority))
		guild, guildErr := c.storage.GetGuild(ctx, guildInstance)
		if guildErr == nil {
			for _, m := range guild.Members {
				if m.AgentID == payload.AgentID {
					guildRank = m.Rank
					break
				}
			}
		}
	}

	// Build XP context
	xpCtx := semdragons.XPContext{
		Quest:        payload.Quest,
		Agent:        *agent,
		BattleResult: payload.Verdict,
		Duration:     payload.Duration,
		Streak:       streak,
		IsGuildQuest: isGuildQuest,
		GuildRank:    guildRank,
		Attempt:      payload.Quest.Attempts,
	}

	// Calculate XP
	award := c.xpEngine.CalculateXP(xpCtx)

	// Apply XP
	xpBefore := agent.XP
	levelBefore := agent.Level
	levelEvent := c.xpEngine.ApplyXP(agent, award.TotalXP)

	// Update streak
	c.storage.SetAgentStreak(ctx, agentInstance, streak)

	// Persist agent
	if err := c.storage.PutAgent(ctx, agentInstance, agent); err != nil {
		c.errorsCount.Add(1)
		c.logger.Error("failed to save agent", "agent", payload.AgentID, "error", err)
		return nil // Don't return error to avoid NATS redelivery for data issues
	}

	// Emit XP event
	c.events.PublishAgentXP(ctx, semdragons.AgentXPPayload{
		AgentID:     payload.AgentID,
		QuestID:     payload.Quest.ID,
		Award:       &award,
		XPDelta:     award.TotalXP,
		XPBefore:    xpBefore,
		XPAfter:     agent.XP,
		LevelBefore: levelBefore,
		LevelAfter:  agent.Level,
		Timestamp:   time.Now(),
		Trace:       payload.Trace,
	})

	// Emit level up event if applicable
	if levelEvent.Direction == "up" {
		c.events.PublishAgentLevelUp(ctx, semdragons.AgentLevelPayload{
			AgentID:   payload.AgentID,
			QuestID:   payload.Quest.ID,
			OldLevel:  levelEvent.OldLevel,
			NewLevel:  levelEvent.NewLevel,
			OldTier:   levelEvent.OldTier,
			NewTier:   levelEvent.NewTier,
			XPCurrent: agent.XP,
			XPToLevel: agent.XPToLevel,
			Timestamp: time.Now(),
			Trace:     payload.Trace,
		})
	}

	c.logger.Debug("processed quest completion",
		"agent", payload.AgentID,
		"quest", payload.Quest.ID,
		"xp_awarded", award.TotalXP,
		"new_level", agent.Level)

	return nil
}

// ProcessFailure handles quest failure events and applies XP penalties.
func (c *Component) ProcessFailure(ctx context.Context, agentID semdragons.AgentID, quest semdragons.Quest, failType semdragons.FailureType) (*semdragons.XPPenalty, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	// Load agent
	agentInstance := semdragons.ExtractInstance(string(agentID))
	agent, err := c.storage.GetAgent(ctx, agentInstance)
	if err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "XPEngine", "ProcessFailure", "load agent")
	}

	// Build penalty context
	penaltyCtx := semdragons.PenaltyContext{
		Quest:       quest,
		Agent:       *agent,
		FailureType: failType,
		Attempt:     quest.Attempts,
	}

	// Calculate penalty
	penalty := c.xpEngine.CalculatePenalty(penaltyCtx)

	// Apply penalty
	xpBefore := agent.XP
	levelBefore := agent.Level
	c.xpEngine.ApplyXP(agent, -penalty.XPLost)

	// Apply cooldown
	if penalty.CooldownDur > 0 {
		cooldownUntil := time.Now().Add(penalty.CooldownDur)
		agent.CooldownUntil = &cooldownUntil
	}

	// Reset streak
	c.storage.ResetAgentStreak(ctx, agentInstance)

	// Handle permadeath
	if penalty.Permadeath {
		agent.Status = semdragons.AgentRetired
		agent.DeathCount++
	}

	// Persist agent
	if err := c.storage.PutAgent(ctx, agentInstance, agent); err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "XPEngine", "ProcessFailure", "save agent")
	}

	// Emit XP event
	c.events.PublishAgentXP(ctx, semdragons.AgentXPPayload{
		AgentID:     agentID,
		QuestID:     quest.ID,
		Penalty:     &penalty,
		XPDelta:     -penalty.XPLost,
		XPBefore:    xpBefore,
		XPAfter:     agent.XP,
		LevelBefore: levelBefore,
		LevelAfter:  agent.Level,
		Timestamp:   time.Now(),
	})

	// Emit cooldown event if applicable
	if penalty.CooldownDur > 0 {
		c.events.PublishAgentCooldown(ctx, semdragons.AgentCooldownPayload{
			AgentID:       agentID,
			QuestID:       quest.ID,
			FailType:      failType,
			CooldownUntil: *agent.CooldownUntil,
			Duration:      penalty.CooldownDur,
			Timestamp:     time.Now(),
		})
	}

	return &penalty, nil
}

// CalculateXP exposes the underlying XP calculation for external use.
func (c *Component) CalculateXP(ctx semdragons.XPContext) semdragons.XPAward {
	return c.xpEngine.CalculateXP(ctx)
}

// Storage returns the underlying storage for external access.
func (c *Component) Storage() *semdragons.Storage {
	return c.storage
}

