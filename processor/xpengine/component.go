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
//
// File organization:
// - component.go: Core Component struct, interfaces, lifecycle
// - config.go: Config struct, defaults, validation
// - handler.go: Event handlers and XP processing methods
// - register.go: Factory and registry registration
// =============================================================================

// Component implements the XPEngine as a semstreams processor.
type Component struct {
	config      *Config
	deps        component.Dependencies
	graph       *semdragons.GraphClient
	events      *semdragons.EventPublisher
	xpEngine    semdragons.XPEngine
	logger      *slog.Logger
	boardConfig *semdragons.BoardConfig

	// Subscriptions
	completedSub *natsclient.Subscription

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

	// Create graph client
	if err := c.createGraphClient(ctx); err != nil {
		return err
	}

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
