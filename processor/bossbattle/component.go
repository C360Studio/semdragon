// Package bossbattle provides a native semstreams component for boss battle
// (quality review) management. It reacts to quest submission events, runs
// evaluation judges, and emits battle verdict events.
package bossbattle

import (
	"context"
	"errors"
	"fmt"
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
// COMPONENT - BossBattle as native semstreams processor
// =============================================================================
// Implements Discoverable + LifecycleComponent interfaces.
// Subscribes to quest.lifecycle.submitted events.
// Runs battle evaluations and emits battle.review.* events.
// =============================================================================

// Config holds the component configuration.
type Config struct {
	// BoardConfig contains org, platform, board for entity IDs and bucket naming.
	Org      string `json:"org" schema:"type:string,description:Organization namespace"`
	Platform string `json:"platform" schema:"type:string,description:Platform/environment name"`
	Board    string `json:"board" schema:"type:string,description:Quest board name"`

	// Battle settings
	DefaultTimeout     time.Duration `json:"default_timeout" schema:"type:duration,description:Default battle timeout"`
	MaxConcurrent      int           `json:"max_concurrent" schema:"type:int,description:Max concurrent battles"`
	AutoStartOnSubmit  bool          `json:"auto_start_on_submit" schema:"type:bool,description:Auto-start battles on submission"`
	RequireReviewLevel bool          `json:"require_review_level" schema:"type:bool,description:Only battle quests with review level set"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Org:                "default",
		Platform:           "local",
		Board:              "main",
		DefaultTimeout:     5 * time.Minute,
		MaxConcurrent:      10,
		AutoStartOnSubmit:  true,
		RequireReviewLevel: true,
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

// Component implements the BossBattle processor as a semstreams component.
type Component struct {
	config      *Config
	deps        component.Dependencies
	storage     *semdragons.Storage
	events      *semdragons.EventPublisher
	evaluator   semdragons.BattleEvaluator
	logger      *slog.Logger
	boardConfig *semdragons.BoardConfig

	// Subscriptions
	submittedSub *natsclient.Subscription

	// Battle tracking
	activeBattles sync.Map // map[BattleID]*activeBattle

	// Internal state
	running  atomic.Bool
	mu       sync.RWMutex
	stopChan chan struct{}

	// Metrics
	battlesStarted    atomic.Uint64
	battlesCompleted  atomic.Uint64
	battlesVictory    atomic.Uint64
	battlesDefeat     atomic.Uint64
	errorsCount       atomic.Int64
	lastActivity      atomic.Value // time.Time
	startTime         time.Time
}

// activeBattle tracks an in-progress battle.
// Note: We only store the cancel function, not the context itself, to avoid
// holding references to parent contexts that may outlive the battle.
type activeBattle struct {
	battle    *semdragons.BossBattle
	quest     *semdragons.Quest
	output    any
	startTime time.Time
	cancel    context.CancelFunc
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
		Name:        "bossbattle",
		Type:        "processor",
		Description: "Boss battle (quality review) management and evaluation",
		Version:     "1.0.0",
	}
}

// InputPorts returns the ports this component accepts data on.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "quest-submitted",
			Direction:   component.DirectionInput,
			Required:    true,
			Description: "Quest submission events triggering battles",
			Config: &component.NATSPort{
				Subject: semdragons.PredicateQuestSubmitted,
			},
		},
	}
}

// OutputPorts returns the ports this component produces data on.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "battle-started",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "Battle started events",
			Config: &component.JetStreamPort{
				StreamName:      "BATTLE_EVENTS",
				Subjects:        []string{"battle.review.>"},
				Storage:         "file",
				RetentionPolicy: "limits",
				RetentionDays:   30,
				Replicas:        1,
			},
		},
		{
			Name:        "battle-verdict",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "Battle verdict events",
			Config: &component.NATSPort{
				Subject: semdragons.PredicateBattleVerdict,
			},
		},
		{
			Name:        "battle-state",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "Battle state updates in KV",
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
			"default_timeout": {
				Type:        "duration",
				Description: "Default battle timeout (default 5m)",
				Default:     "5m",
				Category:    "advanced",
			},
			"max_concurrent": {
				Type:        "int",
				Description: "Maximum concurrent battles (default 10)",
				Default:     10,
				Minimum:     util.IntPtr(1),
				Category:    "advanced",
			},
			"auto_start_on_submit": {
				Type:        "bool",
				Description: "Auto-start battles on quest submission",
				Default:     true,
				Category:    "advanced",
			},
			"require_review_level": {
				Type:        "bool",
				Description: "Only start battles for quests with review level set",
				Default:     true,
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
		status.LastError = "errors encountered during battle processing"
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

	completed := c.battlesCompleted.Load()
	uptime := time.Since(c.startTime).Seconds()
	if uptime > 0 {
		metrics.MessagesPerSecond = float64(completed) / uptime
	}

	if completed > 0 {
		metrics.ErrorRate = float64(c.errorsCount.Load()) / float64(completed)
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
	c.evaluator = semdragons.NewDefaultBattleEvaluator()
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
		return errs.Wrap(err, "BossBattle", "Start", "create storage")
	}
	c.storage = storage

	// Create event publisher
	c.events = semdragons.NewEventPublisher(c.deps.NATSClient)

	// Subscribe to quest submitted events (triggers battles)
	if c.config.AutoStartOnSubmit {
		submittedSub, err := semdragons.SubjectQuestSubmitted.Subscribe(ctx, c.deps.NATSClient, c.handleQuestSubmitted)
		if err != nil {
			return errs.Wrap(err, "BossBattle", "Start", "subscribe to quest.lifecycle.submitted")
		}
		c.submittedSub = submittedSub
	}

	c.startTime = time.Now()
	c.running.Store(true)
	c.lastActivity.Store(time.Now())

	c.logger.Info("bossbattle component started",
		"org", c.config.Org,
		"platform", c.config.Platform,
		"board", c.config.Board,
		"auto_start", c.config.AutoStartOnSubmit)

	return nil
}

// Stop gracefully shuts down the component.
// The timeout parameter is part of the LifecycleComponent interface but is not
// currently used as cleanup is synchronous and quick.
func (c *Component) Stop(_ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running.Load() {
		return nil
	}

	// Close stop channel
	close(c.stopChan)

	// Cancel all active battles
	c.activeBattles.Range(func(_, value any) bool {
		if ab, ok := value.(*activeBattle); ok {
			ab.cancel()
		}
		return true
	})

	// Unsubscribe from events
	if c.submittedSub != nil {
		c.submittedSub.Unsubscribe()
	}

	c.running.Store(false)
	c.logger.Info("bossbattle component stopped")

	return nil
}

// =============================================================================
// EVENT HANDLERS
// =============================================================================

// handleQuestSubmitted processes quest submission events and starts battles.
func (c *Component) handleQuestSubmitted(ctx context.Context, payload semdragons.QuestSubmittedPayload) error {
	if !c.running.Load() {
		return nil
	}

	c.lastActivity.Store(time.Now())

	// Check if quest requires review
	if c.config.RequireReviewLevel && payload.Quest.Constraints.ReviewLevel == semdragons.ReviewAuto {
		c.logger.Debug("skipping battle for quest without review level",
			"quest", payload.Quest.ID)
		return nil
	}

	// Start battle
	battle, err := c.StartBattle(ctx, &payload.Quest, payload.Result)
	if err != nil {
		c.errorsCount.Add(1)
		c.logger.Error("failed to start battle",
			"quest", payload.Quest.ID,
			"error", err)
		return nil // Don't return error to avoid NATS redelivery
	}

	c.logger.Debug("started battle for submitted quest",
		"quest", payload.Quest.ID,
		"battle", battle.ID)

	return nil
}

// =============================================================================
// BATTLE OPERATIONS
// =============================================================================

// StartBattle initiates a boss battle for a quest.
func (c *Component) StartBattle(ctx context.Context, quest *semdragons.Quest, output any) (*semdragons.BossBattle, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	// Generate battle ID
	battleID := semdragons.BattleID(c.boardConfig.EntityID("battle", c.generateID()))

	// Build battle from quest review level
	battle := c.buildBattle(battleID, quest)

	// Store battle
	battleInstance := semdragons.ExtractInstance(string(battle.ID))
	if err := c.storage.PutBattle(ctx, battleInstance, battle); err != nil {
		return nil, errs.Wrap(err, "BossBattle", "StartBattle", "store battle")
	}

	// Add to active battles
	battleCtx, cancel := context.WithTimeout(ctx, c.config.DefaultTimeout)
	ab := &activeBattle{
		battle:    battle,
		quest:     quest,
		output:    output,
		startTime: time.Now(),
		cancel:    cancel,
	}
	c.activeBattles.Store(battle.ID, ab)

	// Emit battle started event
	c.events.PublishBattleStarted(ctx, semdragons.BattleStartedPayload{
		Battle:    *battle,
		Quest:     *quest,
		StartedAt: battle.StartedAt,
	})

	c.battlesStarted.Add(1)

	// Run evaluation asynchronously with its own context
	go c.runEvaluation(battleCtx, ab)

	return battle, nil
}

// buildBattle constructs a BossBattle from quest settings.
func (c *Component) buildBattle(id semdragons.BattleID, quest *semdragons.Quest) *semdragons.BossBattle {
	now := time.Now()

	reviewLevel := quest.Constraints.ReviewLevel

	// Default criteria based on review level
	criteria := c.defaultCriteria(reviewLevel)
	judges := c.defaultJudges(reviewLevel)

	// Get agent ID (handle pointer)
	var agentID semdragons.AgentID
	if quest.ClaimedBy != nil {
		agentID = *quest.ClaimedBy
	}

	return &semdragons.BossBattle{
		ID:        id,
		QuestID:   quest.ID,
		AgentID:   agentID,
		Level:     reviewLevel,
		Status:    semdragons.BattleActive,
		Criteria:  criteria,
		Judges:    judges,
		StartedAt: now,
	}
}

// defaultCriteria returns standard review criteria for a level.
func (c *Component) defaultCriteria(level semdragons.ReviewLevel) []semdragons.ReviewCriterion {
	switch level {
	case semdragons.ReviewStrict:
		return []semdragons.ReviewCriterion{
			{Name: "correctness", Description: "Output is factually correct", Weight: 0.4, Threshold: 0.8},
			{Name: "completeness", Description: "All requirements addressed", Weight: 0.3, Threshold: 0.8},
			{Name: "quality", Description: "High quality, production-ready", Weight: 0.2, Threshold: 0.7},
			{Name: "style", Description: "Follows conventions and best practices", Weight: 0.1, Threshold: 0.6},
		}
	case semdragons.ReviewHuman:
		return []semdragons.ReviewCriterion{
			{Name: "correctness", Description: "Output is factually correct", Weight: 0.3, Threshold: 0.8},
			{Name: "completeness", Description: "All requirements addressed", Weight: 0.3, Threshold: 0.8},
			{Name: "quality", Description: "High quality, production-ready", Weight: 0.2, Threshold: 0.7},
			{Name: "style", Description: "Follows conventions and best practices", Weight: 0.2, Threshold: 0.6},
		}
	case semdragons.ReviewStandard:
		return []semdragons.ReviewCriterion{
			{Name: "correctness", Description: "Output is factually correct", Weight: 0.5, Threshold: 0.7},
			{Name: "completeness", Description: "Key requirements addressed", Weight: 0.3, Threshold: 0.6},
			{Name: "quality", Description: "Acceptable quality", Weight: 0.2, Threshold: 0.5},
		}
	default: // ReviewAuto
		return []semdragons.ReviewCriterion{
			{Name: "acceptance", Description: "Output is acceptable", Weight: 1.0, Threshold: 0.5},
		}
	}
}

// defaultJudges returns standard judges for a review level.
func (c *Component) defaultJudges(level semdragons.ReviewLevel) []semdragons.Judge {
	switch level {
	case semdragons.ReviewStrict:
		return []semdragons.Judge{
			{ID: "judge-auto", Type: semdragons.JudgeAutomated, Config: map[string]any{}},
			{ID: "judge-llm-1", Type: semdragons.JudgeLLM, Config: map[string]any{}},
			{ID: "judge-llm-2", Type: semdragons.JudgeLLM, Config: map[string]any{}},
		}
	case semdragons.ReviewHuman:
		return []semdragons.Judge{
			{ID: "judge-auto", Type: semdragons.JudgeAutomated, Config: map[string]any{}},
			{ID: "judge-llm-1", Type: semdragons.JudgeLLM, Config: map[string]any{}},
			{ID: "judge-human", Type: semdragons.JudgeHuman, Config: map[string]any{}},
		}
	case semdragons.ReviewStandard:
		return []semdragons.Judge{
			{ID: "judge-auto", Type: semdragons.JudgeAutomated, Config: map[string]any{}},
			{ID: "judge-llm-1", Type: semdragons.JudgeLLM, Config: map[string]any{}},
		}
	default: // ReviewAuto
		return []semdragons.Judge{
			{ID: "judge-auto", Type: semdragons.JudgeAutomated, Config: nil},
		}
	}
}

// runEvaluation performs the actual battle evaluation.
func (c *Component) runEvaluation(ctx context.Context, ab *activeBattle) {
	defer func() {
		ab.cancel()
		c.activeBattles.Delete(ab.battle.ID)
	}()

	// Run evaluation
	result, err := c.evaluator.Evaluate(ctx, ab.battle, ab.quest, ab.output)

	now := time.Now()
	ab.battle.CompletedAt = &now

	if err != nil {
		c.errorsCount.Add(1)
		c.logger.Error("battle evaluation failed",
			"battle", ab.battle.ID,
			"error", err)

		// Mark as failed with default verdict (defeat)
		ab.battle.Status = semdragons.BattleDefeat
		ab.battle.Verdict = &semdragons.BattleVerdict{
			Passed:       false,
			QualityScore: 0,
			Feedback:     fmt.Sprintf("Evaluation error: %v", err),
		}
	} else if result.Pending {
		// Battle awaiting human review - keep active
		// The battle stays in Active status until human provides verdict
		c.logger.Info("battle awaiting human review",
			"battle", ab.battle.ID,
			"pending_judge", result.PendingJudge)
		// Don't complete - leave in active state, don't remove from tracking
		return
	} else {
		// Complete with verdict - use Victory or Defeat based on result
		if result.Verdict.Passed {
			ab.battle.Status = semdragons.BattleVictory
		} else {
			ab.battle.Status = semdragons.BattleDefeat
		}
		ab.battle.Results = result.Results
		ab.battle.Verdict = &result.Verdict
	}

	// Persist final battle state (only if component is still running)
	// After shutdown, we skip persistence to avoid race conditions
	if c.running.Load() {
		battleInstance := semdragons.ExtractInstance(string(ab.battle.ID))
		// Use a short timeout for final persistence since the battle context may be cancelled
		persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := c.storage.PutBattle(persistCtx, battleInstance, ab.battle); err != nil {
			c.errorsCount.Add(1)
			c.logger.Error("failed to persist battle result",
				"battle", ab.battle.ID,
				"error", err)
		}
		cancel()

		// Emit verdict event
		c.events.PublishBattleVerdict(persistCtx, semdragons.BattleVerdictPayload{
			Battle:  *ab.battle,
			Quest:   *ab.quest,
			Verdict: *ab.battle.Verdict,
			EndedAt: now,
		})
	} else {
		c.logger.Debug("skipping battle persistence after shutdown",
			"battle", ab.battle.ID)
	}

	c.battlesCompleted.Add(1)
	if ab.battle.Verdict.Passed {
		c.battlesVictory.Add(1)
	} else {
		c.battlesDefeat.Add(1)
	}

	c.logger.Info("battle completed",
		"battle", ab.battle.ID,
		"quest", ab.quest.ID,
		"passed", ab.battle.Verdict.Passed,
		"quality", ab.battle.Verdict.QualityScore)
}

// GetBattle retrieves a battle by ID.
func (c *Component) GetBattle(ctx context.Context, id semdragons.BattleID) (*semdragons.BossBattle, error) {
	instance := semdragons.ExtractInstance(string(id))
	return c.storage.GetBattle(ctx, instance)
}

// ListActiveBattles returns all currently active battles.
func (c *Component) ListActiveBattles() []*semdragons.BossBattle {
	var battles []*semdragons.BossBattle
	c.activeBattles.Range(func(_, value any) bool {
		if ab, ok := value.(*activeBattle); ok {
			battles = append(battles, ab.battle)
		}
		return true
	})
	return battles
}

// Stats returns battle statistics.
func (c *Component) Stats() BattleStats {
	return BattleStats{
		Started:   c.battlesStarted.Load(),
		Completed: c.battlesCompleted.Load(),
		Victory:   c.battlesVictory.Load(),
		Defeat:    c.battlesDefeat.Load(),
		Active:    c.countActiveBattles(),
		Errors:    c.errorsCount.Load(),
	}
}

// BattleStats holds battle statistics.
type BattleStats struct {
	Started   uint64 `json:"started"`
	Completed uint64 `json:"completed"`
	Victory   uint64 `json:"victory"`
	Defeat    uint64 `json:"defeat"`
	Active    int    `json:"active"`
	Errors    int64  `json:"errors"`
}

// Storage returns the underlying storage for external access.
func (c *Component) Storage() *semdragons.Storage {
	return c.storage
}

// =============================================================================
// HELPERS
// =============================================================================

func (c *Component) generateID() string {
	return semdragons.GenerateInstance()
}

func (c *Component) countActiveBattles() int {
	count := 0
	c.activeBattles.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}
