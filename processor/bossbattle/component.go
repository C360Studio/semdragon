// Package bossbattle provides a native semstreams component for boss battle
// (quality review) management. It reacts to quest submission events, runs
// evaluation judges, and emits battle verdict events.
package bossbattle

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/model"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/internal/util"
	"github.com/c360studio/semdragons/processor/promptmanager"
	"github.com/c360studio/semdragons/processor/tokenbudget"
)

// =============================================================================
// COMPONENT - BossBattle as native semstreams processor
// =============================================================================
// Implements Discoverable + LifecycleComponent interfaces.
// Watches quest entity state via KV for in_review transitions.
// Runs battle evaluations and emits battle state via entity writes.
// =============================================================================

// Component implements the BossBattle processor as a semstreams component.
type Component struct {
	config      *Config
	deps        component.Dependencies
	graph       *semdragons.GraphClient
	evaluator   BattleEvaluator
	catalog     *promptmanager.DomainCatalog
	registry    model.RegistryReader
	assembler   *promptmanager.PromptAssembler
	logger      *slog.Logger
	boardConfig *domain.BoardConfig

	// Token budget enforcement
	tokenLedger *tokenbudget.TokenLedger

	// KV watcher for quest entity state changes (entity-centric architecture)
	questWatch  jetstream.KeyWatcher
	watchDoneCh chan struct{}

	// Quest state cache for detecting transitions
	questCache sync.Map // map[entityID]domain.QuestStatus

	// Battle tracking
	activeBattles sync.Map // map[BattleID]*activeBattle

	// Internal state
	running  atomic.Bool
	mu       sync.RWMutex
	stopChan chan struct{}

	// Metrics
	battlesStarted   atomic.Uint64
	battlesCompleted atomic.Uint64
	battlesVictory   atomic.Uint64
	battlesDefeat    atomic.Uint64
	errorsCount      atomic.Int64
	lastActivity     atomic.Value // time.Time
	startTime        time.Time
}

// activeBattle tracks an in-progress battle.
type activeBattle struct {
	battle    *BossBattle
	quest     *domain.Quest
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
// Entity-centric: watches ENTITY_STATES KV for quest status transitions to in_review.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "quest-state-watch",
			Direction:   component.DirectionInput,
			Required:    true,
			Description: "Quest entity state changes via KV watch (detects in_review transitions)",
			Config: &component.KVWritePort{
				Bucket: "", // ENTITY_STATES bucket, watched dynamically
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
				Subject: domain.PredicateBattleVerdict,
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

// SetTokenLedger injects the shared token ledger for budget enforcement.
// Must be called before Initialize. Ignored if the component is already running.
func (c *Component) SetTokenLedger(l *tokenbudget.TokenLedger) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running.Load() {
		c.logger.Warn("SetTokenLedger called while running; ignored")
		return
	}
	c.tokenLedger = l
}

// Initialize performs one-time setup. No I/O operations here.
func (c *Component) Initialize() error {
	if c.config == nil {
		return errors.New("config not set")
	}

	if c.deps.NATSClient == nil {
		return errors.New("NATS client required")
	}

	c.boardConfig = c.config.ToBoardConfig()
	c.catalog = c.config.DomainCatalog
	c.registry = c.deps.ModelRegistry

	if c.catalog != nil {
		promptRegistry := promptmanager.NewPromptRegistry()
		promptRegistry.RegisterProviderStyles()
		c.assembler = promptmanager.NewPromptAssembler(promptRegistry)
		c.evaluator = NewDomainAwareEvaluator(c.catalog, c.registry, c.assembler, c.tokenLedger, c.deps.NATSClient)
	} else {
		c.evaluator = NewDefaultBattleEvaluator()
	}
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

	// Create graph client
	if err := c.createGraphClient(ctx); err != nil {
		return err
	}

	c.startTime = time.Now()
	c.running.Store(true)
	c.lastActivity.Store(time.Now())

	// Watch quest entity type for status transitions to in_review (entity-centric)
	if c.config.AutoStartOnSubmit {
		watcher, err := c.graph.WatchEntityType(ctx, domain.EntityTypeQuest)
		if err != nil {
			c.running.Store(false)
			return errs.Wrap(err, "BossBattle", "Start", "watch quest entity type")
		}
		c.questWatch = watcher
		c.watchDoneCh = make(chan struct{})
		go c.processQuestWatchUpdates()
	}

	c.logger.Info("bossbattle component started",
		"org", c.config.Org,
		"platform", c.config.Platform,
		"board", c.config.Board,
		"auto_start", c.config.AutoStartOnSubmit)

	return nil
}

// Stop gracefully shuts down the component.
func (c *Component) Stop(timeout time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running.Load() {
		return nil
	}

	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	// Close stop channel
	close(c.stopChan)

	// Stop KV watcher
	if c.questWatch != nil {
		c.questWatch.Stop()
	}

	// Wait for watch goroutine to finish with timeout
	if c.watchDoneCh != nil {
		select {
		case <-c.watchDoneCh:
		case <-time.After(timeout):
			c.logger.Warn("stop timed out waiting for KV watcher")
		}
	}

	// Cancel all active battles
	c.activeBattles.Range(func(_, value any) bool {
		if ab, ok := value.(*activeBattle); ok {
			ab.cancel()
		}
		return true
	})

	c.running.Store(false)
	c.logger.Info("bossbattle component stopped")

	return nil
}
