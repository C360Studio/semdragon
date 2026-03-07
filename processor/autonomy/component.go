// Package autonomy provides a native semstreams component for agent autonomy.
// It watches agent state and boid suggestions, manages per-agent heartbeat timers,
// evaluates possible actions on each heartbeat, and detects cooldown expiry.
package autonomy

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
	"github.com/nats-io/nats.go/jetstream"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentstore"
	"github.com/c360studio/semdragons/processor/boardcontrol"
	"github.com/c360studio/semdragons/processor/dmapproval"
	"github.com/c360studio/semdragons/processor/guildformation"
)

// =============================================================================
// COMPONENT - Autonomy processor as native semstreams component
// =============================================================================
// Watches agent state via KV and boid suggestions via NATS pub/sub.
// Manages per-agent heartbeat timers with state-dependent cadence.
// Evaluates what actions each agent can take on each heartbeat.
// Detects cooldown expiry and transitions agents back to idle.
// =============================================================================

// Component implements the autonomy processor as a semstreams component.
//
// Lock ordering: mu must be acquired before trackersMu when both are needed.
// mu guards lifecycle operations (Start/Stop); trackersMu guards the per-agent
// tracker map. Never hold trackersMu across I/O operations.
type Component struct {
	config      *Config
	deps        component.Dependencies
	graph       *semdragons.GraphClient
	logger      *slog.Logger
	boardConfig *domain.BoardConfig
	store        *agentstore.Component      // Optional: nil disables shopping/consumable actions
	guilds       *guildformation.Component // Optional: nil disables guild joining
	approval     *dmapproval.Component     // Optional: nil auto-approves all actions
	pauseChecker boardcontrol.PauseChecker // Optional: nil means always-running

	// KV watcher for agent entity state changes
	agentWatch  jetstream.KeyWatcher
	watchDoneCh chan struct{}

	// NATS subscription for boid suggestions
	boidSub *natsclient.Subscription

	// Per-agent heartbeat trackers
	trackers   map[string]*agentTracker
	trackersMu sync.RWMutex

	// Internal state
	running  atomic.Bool
	mu       sync.RWMutex
	stopChan chan struct{}
	stopOnce sync.Once

	// Metrics
	evaluationsRun   atomic.Uint64
	cooldownsExpired atomic.Uint64
	errorsCount      atomic.Int64
	lastActivity     atomic.Value // time.Time
	startTime        time.Time
}

// Ensure Component implements the required interfaces.
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
		Name:        ComponentName,
		Type:        "processor",
		Description: "Agent autonomy heartbeat and action evaluation",
		Version:     "1.0.0",
	}
}

// InputPorts returns the ports this component accepts data on.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "agent-state-watch",
			Direction:   component.DirectionInput,
			Required:    true,
			Description: "Agent entity state changes via KV watch",
			Config: &component.KVWatchPort{
				Bucket: "", // ENTITY_STATES bucket, watched dynamically via graph client
			},
		},
		{
			Name:        "boid-suggestions",
			Direction:   component.DirectionInput,
			Required:    false,
			Description: "Boid engine quest suggestions via NATS pub/sub",
			Config: &component.NATSPort{
				Subject: "boid.suggestions.*",
			},
		},
	}
}

// OutputPorts returns the ports this component produces data on.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "autonomy-evaluated",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "Heartbeat evaluation events",
			Config: &component.NATSPort{
				Subject: domain.PredicateAutonomyEvaluated,
			},
		},
		{
			Name:        "autonomy-idle",
			Direction:   component.DirectionOutput,
			Required:    false,
			Description: "Agent idle with nothing actionable",
			Config: &component.NATSPort{
				Subject: domain.PredicateAutonomyIdle,
			},
		},
		{
			Name:        "claim-intent",
			Direction:   component.DirectionOutput,
			Required:    false,
			Description: "Agent intends to claim a quest",
			Config: &component.NATSPort{
				Subject: domain.PredicateAutonomyClaimIntent,
			},
		},
		{
			Name:        "shop-intent",
			Direction:   component.DirectionOutput,
			Required:    false,
			Description: "Agent intends to purchase an item",
			Config: &component.NATSPort{
				Subject: domain.PredicateAutonomyShopIntent,
			},
		},
		{
			Name:        "guild-intent",
			Direction:   component.DirectionOutput,
			Required:    false,
			Description: "Agent intends to join a guild",
			Config: &component.NATSPort{
				Subject: domain.PredicateAutonomyGuildIntent,
			},
		},
		{
			Name:        "guild-create-intent",
			Direction:   component.DirectionOutput,
			Required:    false,
			Description: "Agent proposes founding a guild",
			Config: &component.NATSPort{
				Subject: domain.PredicateGuildProposed,
			},
		},
		{
			Name:        "use-intent",
			Direction:   component.DirectionOutput,
			Required:    false,
			Description: "Agent intends to use a consumable",
			Config: &component.NATSPort{
				Subject: domain.PredicateAutonomyUseIntent,
			},
		},
		{
			Name:        "claim-state",
			Direction:   component.DirectionOutput,
			Required:    false,
			Description: "Quest and agent entity state updates from autonomous claims",
			Config: &component.KVWritePort{
				Bucket: "",
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
			"initial_delay_ms": {
				Type:        "int",
				Description: "Delay before first heartbeat (ms)",
				Default:     2000,
				Category:    "heartbeat",
			},
			"idle_interval_ms": {
				Type:        "int",
				Description: "Heartbeat interval when idle (ms)",
				Default:     5000,
				Category:    "heartbeat",
			},
			"on_quest_interval_ms": {
				Type:        "int",
				Description: "Heartbeat interval when on quest (ms)",
				Default:     30000,
				Category:    "heartbeat",
			},
			"in_battle_interval_ms": {
				Type:        "int",
				Description: "Heartbeat interval when in battle (ms)",
				Default:     60000,
				Category:    "heartbeat",
			},
			"cooldown_interval_ms": {
				Type:        "int",
				Description: "Heartbeat interval when on cooldown (ms)",
				Default:     15000,
				Category:    "heartbeat",
			},
			"max_interval_ms": {
				Type:        "int",
				Description: "Maximum heartbeat interval (ms)",
				Default:     60000,
				Category:    "heartbeat",
			},
			"backoff_factor": {
				Type:        "float",
				Description: "Backoff multiplier for idle agents",
				Default:     1.5,
				Category:    "heartbeat",
			},
			"min_xp_surplus_for_shopping": {
				Type:        "int",
				Description: "Minimum XP surplus above XPToLevel to trigger idle shopping",
				Default:     50,
				Category:    "shopping",
			},
			"max_shop_spend_ratio": {
				Type:        "float",
				Description: "Fraction of XP surplus to spend when idle shopping",
				Default:     0.5,
				Category:    "shopping",
			},
			"cooldown_shop_min_xp": {
				Type:        "int",
				Description: "Minimum XP to shop during cooldown",
				Default:     25,
				Category:    "shopping",
			},
			"strategic_shop_max_cost": {
				Type:        "int",
				Description: "Maximum XP to spend on strategic mid-quest purchase",
				Default:     200,
				Category:    "shopping",
			},
			"cooldown_skip_min_remaining_ms": {
				Type:        "int",
				Description: "Minimum remaining cooldown in milliseconds to justify using skip consumable",
				Default:     30000,
				Category:    "shopping",
			},
			"max_guilds_per_agent": {
				Type:        "int",
				Description: "Maximum guilds an agent can autonomously join",
				Default:     3,
				Category:    "guild",
			},
			"guild_join_min_level": {
				Type:        "int",
				Description: "Minimum agent level to autonomously join guilds",
				Default:     3,
				Category:    "guild",
			},
			"guild_suggestions_n": {
				Type:        "int",
				Description: "Number of guild choices to evaluate before picking",
				Default:     5,
				Category:    "guild",
			},
			"guild_create_min_level": {
				Type:        "int",
				Description: "Minimum agent level to propose founding a guild (Master tier)",
				Default:     16,
				Category:    "guild",
			},
			"guild_create_min_fellows": {
				Type:        "int",
				Description: "Minimum fellowship candidates to propose a guild",
				Default:     3,
				Category:    "guild",
			},
			"guild_create_max_founders": {
				Type:        "int",
				Description: "Maximum founding members to invite when creating a guild",
				Default:     6,
				Category:    "guild",
			},
			"dm_mode": {
				Type:        "string",
				Description: "DM mode governing approval behavior (full_auto, assisted, supervised, manual)",
				Default:     "full_auto",
				Category:    "approval",
			},
			"session_id": {
				Type:        "string",
				Description: "DM session ID for routing approval requests",
				Default:     "",
				Category:    "approval",
			},
			"approval_timeout_ms": {
				Type:        "int",
				Description: "Timeout for DM approval requests (ms)",
				Default:     300000,
				Category:    "approval",
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
		status.LastError = "errors encountered during autonomy evaluation"
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

	evaluations := c.evaluationsRun.Load()
	uptime := time.Since(c.startTime).Seconds()
	if uptime > 0 {
		metrics.MessagesPerSecond = float64(evaluations) / uptime
	}

	if evaluations > 0 {
		metrics.ErrorRate = float64(c.errorsCount.Load()) / float64(evaluations)
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

	if c.config.DMMode != "" && !domain.ValidDMMode(c.config.DMMode) {
		return fmt.Errorf("invalid dm_mode %q: must be one of full_auto, assisted, supervised, manual", c.config.DMMode)
	}

	c.boardConfig = c.config.ToBoardConfig()
	c.stopChan = make(chan struct{})
	c.watchDoneCh = make(chan struct{})
	c.trackers = make(map[string]*agentTracker)

	return nil
}

// Start begins component operation with the given context.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running.Load() {
		return errors.New("component already running")
	}

	// Warn if approval mode is configured but SessionID is empty — actions will
	// silently auto-approve, which defeats the purpose of supervised/manual mode.
	if (c.config.DMMode == domain.DMSupervised || c.config.DMMode == domain.DMManual) && c.config.SessionID == "" {
		c.logger.Warn("dm_mode is set to approval-required mode but session_id is empty; all actions will auto-approve",
			"dm_mode", c.config.DMMode)
	}

	// Create graph client for entity state reads/writes
	c.graph = semdragons.NewGraphClient(c.deps.NATSClient, c.boardConfig)

	// Start KV watcher for agent entity state
	watcher, err := c.graph.WatchEntityType(ctx, domain.EntityTypeAgent)
	if err != nil {
		return errs.Wrap(err, "autonomy", "Start", "watch agent entity type")
	}
	c.agentWatch = watcher

	// Subscribe to boid suggestions (NATS pub/sub, not KV)
	boidSub, err := c.deps.NATSClient.Subscribe(ctx, "boid.suggestions.>", c.handleBoidSuggestion)
	if err != nil {
		c.agentWatch.Stop()
		return errs.Wrap(err, "autonomy", "Start", "subscribe boid suggestions")
	}
	c.boidSub = boidSub

	c.startTime = time.Now()
	c.running.Store(true)
	c.lastActivity.Store(time.Now())

	// Start watch goroutine
	go c.processAgentWatchUpdates()

	c.logger.Info("autonomy component started",
		"org", c.config.Org,
		"platform", c.config.Platform,
		"board", c.config.Board)

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

	// Signal stop using sync.Once to prevent double-close panic
	c.stopOnce.Do(func() {
		close(c.stopChan)
	})

	// Unsubscribe from boid suggestions
	if c.boidSub != nil {
		c.boidSub.Unsubscribe()
	}

	// Stop KV watcher to unblock the watch goroutine
	if c.agentWatch != nil {
		c.agentWatch.Stop()
	}

	// Wait for watch goroutine to finish with timeout
	if c.watchDoneCh != nil {
		select {
		case <-c.watchDoneCh:
		case <-time.After(timeout):
			c.logger.Warn("autonomy stop timed out waiting for KV watcher")
		}
	}

	// Set running=false BEFORE cancelling timers so that any timer callback
	// that fires between Stop() and cancelAllHeartbeats() sees the flag and
	// returns immediately without calling evaluateAutonomy.
	c.running.Store(false)
	c.cancelAllHeartbeats()

	c.logger.Info("autonomy component stopped")

	return nil
}

// BoardConfig returns the board configuration.
func (c *Component) BoardConfig() *domain.BoardConfig {
	return c.boardConfig
}

// SetStore injects the agent store component for shopping and consumable actions.
// When store is nil, all shopping/consumable actions safely return shouldExecute=false.
// SetStore is ignored once the component is running to prevent concurrent store access.
func (c *Component) SetStore(store *agentstore.Component) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running.Load() {
		c.logger.Warn("SetStore called while running; ignored")
		return
	}
	c.store = store
}

// SetGuilds injects the guild formation component for autonomous guild joining.
// When guilds is nil, joinGuildAction safely returns shouldExecute=false.
// SetGuilds is ignored once the component is running to prevent concurrent access.
func (c *Component) SetGuilds(guilds *guildformation.Component) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running.Load() {
		c.logger.Warn("SetGuilds called while running; ignored")
		return
	}
	c.guilds = guilds
}

// SetApproval injects the DM approval component for non-FullAuto modes.
// When approval is nil, all actions auto-approve (same as FullAuto behavior).
// SetApproval is ignored once the component is running to prevent concurrent access.
func (c *Component) SetApproval(approval *dmapproval.Component) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running.Load() {
		c.logger.Warn("SetApproval called while running; ignored")
		return
	}
	c.approval = approval
}

// SetPauseChecker injects the board pause checker. When paused, heartbeat
// callbacks skip evaluation and re-arm the timer for the next tick.
// SetPauseChecker is ignored once the component is running.
func (c *Component) SetPauseChecker(pc boardcontrol.PauseChecker) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running.Load() {
		c.logger.Warn("SetPauseChecker called while running; ignored")
		return
	}
	c.pauseChecker = pc
}
