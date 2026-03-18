// Package redteam provides a reactive processor that posts adversarial review
// quests when normal quests are submitted for review. A guild (or ad-hoc party)
// is attracted to the red-team quest via the boid engine. The red-team's findings
// are attached to the original quest before the boss battle evaluates it.
//
// Non-blocking: if no agent claims the red-team quest within the claim timeout,
// or if execution exceeds the execution timeout, the processor emits a "skipped"
// signal and the boss battle proceeds without red-team findings.
package redteam

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
)

// QuestBoardRef is the minimal interface needed to post and manage red-team quests.
type QuestBoardRef interface {
	PostQuest(ctx context.Context, quest domain.Quest) (*domain.Quest, error)
	FailQuest(ctx context.Context, questID domain.QuestID, reason string) error
}

// Component implements the red-team review processor.
type Component struct {
	config      *Config
	deps        component.Dependencies
	graph       *semdragons.GraphClient
	logger      *slog.Logger
	boardConfig *domain.BoardConfig

	// QuestBoard reference for posting red-team quests.
	questBoard QuestBoardRef

	// KV watcher for quest entity state changes.
	questWatch  jetstream.KeyWatcher
	watchDoneCh chan struct{}

	// Quest state cache for detecting transitions.
	questCache sync.Map // map[entityID]domain.QuestStatus

	// Track pending red-team quests: original quest ID → red-team quest ID.
	pendingReviews sync.Map // map[domain.QuestID]*pendingRedTeam

	// Internal state.
	running  atomic.Bool
	mu       sync.RWMutex
	stopChan chan struct{}

	// Metrics.
	reviewsPosted  atomic.Uint64
	reviewsSkipped atomic.Uint64
	reviewsDone    atomic.Uint64
	errorsCount    atomic.Int64
	lastActivity   atomic.Value // time.Time
	startTime      time.Time
}

// pendingRedTeam tracks a red-team quest awaiting completion.
type pendingRedTeam struct {
	RedTeamQuestID domain.QuestID
	OriginalQuest  domain.QuestID
	PostedAt       time.Time
	cancel         context.CancelFunc
}

// ensure Component implements the required interfaces.
var (
	_ component.Discoverable       = (*Component)(nil)
	_ component.LifecycleComponent = (*Component)(nil)
)

// SetQuestBoard injects the questboard reference for posting quests.
// If not called, the component resolves questboard from the ComponentRegistry
// during Start(). Must be called before Start().
func (c *Component) SetQuestBoard(qb QuestBoardRef) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.questBoard = qb
}

// resolveQuestBoard returns the cached questboard reference.
// Resolved once during Start() — safe to call from the watcher goroutine
// without synchronization since it's immutable after initialization.
func (c *Component) resolveQuestBoard() QuestBoardRef {
	return c.questBoard
}

// Meta returns basic component information.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        ComponentName,
		Type:        "processor",
		Description: "Red-team review: posts adversarial review quests for submitted work",
		Version:     "1.0.0",
	}
}

// InputPorts returns the ports this component accepts data on.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "quest-state-watch",
			Direction:   component.DirectionInput,
			Required:    true,
			Description: "Quest entity state changes via KV watch (detects in_review transitions)",
			Config: &component.KVWritePort{
				Bucket: "",
			},
		},
	}
}

// OutputPorts returns the ports this component produces data on.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "redteam-posted",
			Direction:   component.DirectionOutput,
			Required:    false,
			Description: "Red-team quest posted events",
			Config: &component.NATSPort{
				Subject: domain.PredicateRedTeamPosted,
			},
		},
		{
			Name:        "redteam-completed",
			Direction:   component.DirectionOutput,
			Required:    false,
			Description: "Red-team review completed events",
			Config: &component.NATSPort{
				Subject: domain.PredicateRedTeamCompleted,
			},
		},
	}
}

// ConfigSchema returns the configuration schema for this component.
func (c *Component) ConfigSchema() component.ConfigSchema {
	return component.ConfigSchema{
		Properties: map[string]component.PropertySchema{
			"org":               {Type: "string", Description: "Organization namespace", Default: "default", Category: "basic"},
			"platform":          {Type: "string", Description: "Platform/environment name", Default: "local", Category: "basic"},
			"board":             {Type: "string", Description: "Quest board name", Default: "main", Category: "basic"},
			"min_difficulty":    {Type: "int", Description: "Minimum quest difficulty for red-team review", Default: 2, Category: "basic"},
			"claim_timeout":     {Type: "duration", Description: "Max wait for claim before skip", Default: "2m", Category: "advanced"},
			"execution_timeout": {Type: "duration", Description: "Total time for red-team lifecycle", Default: "5m", Category: "advanced"},
			"prefer_cross_guild": {Type: "bool", Description: "Prefer cross-guild reviewers", Default: true, Category: "basic"},
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
	return status
}

// DataFlow returns current data flow metrics.
func (c *Component) DataFlow() component.FlowMetrics {
	metrics := component.FlowMetrics{}
	if lastTime, ok := c.lastActivity.Load().(time.Time); ok {
		metrics.LastActivity = lastTime
	}
	posted := c.reviewsPosted.Load()
	uptime := time.Since(c.startTime).Seconds()
	if uptime > 0 {
		metrics.MessagesPerSecond = float64(posted) / uptime
	}
	return metrics
}

// Initialize performs one-time setup.
func (c *Component) Initialize() error {
	if c.config == nil {
		return errors.New("config not set")
	}
	if c.deps.NATSClient == nil {
		return errors.New("NATS client required")
	}
	c.boardConfig = c.config.ToBoardConfig()
	c.stopChan = make(chan struct{})
	return nil
}

// Start begins component operation.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running.Load() {
		return errors.New("component already running")
	}

	// Create graph client for entity reads/writes.
	c.graph = semdragons.NewGraphClient(c.deps.NATSClient, c.boardConfig)

	// Resolve questboard from registry if not explicitly set.
	if c.questBoard == nil && c.deps.ComponentRegistry != nil {
		if comp := c.deps.ComponentRegistry.Component("questboard"); comp != nil {
			if ref, ok := comp.(QuestBoardRef); ok {
				c.questBoard = ref
			} else {
				c.logger.Warn("questboard component does not implement QuestBoardRef")
			}
		}
	}

	c.startTime = time.Now()
	c.running.Store(true)
	c.lastActivity.Store(time.Now())

	// Watch quest entity type for status transitions to in_review.
	watcher, err := c.graph.WatchEntityType(ctx, domain.EntityTypeQuest)
	if err != nil {
		c.running.Store(false)
		return errs.Wrap(err, "redteam", "Start", "watch quest entity type")
	}
	c.questWatch = watcher
	c.watchDoneCh = make(chan struct{})
	go c.processQuestWatchUpdates()

	c.logger.Info("redteam component started",
		"org", c.config.Org,
		"platform", c.config.Platform,
		"board", c.config.Board,
		"min_difficulty", c.config.MinDifficulty,
		"claim_timeout", c.config.ClaimTimeout(),
		"execution_timeout", c.config.ExecutionTimeout())

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

	close(c.stopChan)

	if c.questWatch != nil {
		c.questWatch.Stop()
	}

	if c.watchDoneCh != nil {
		select {
		case <-c.watchDoneCh:
		case <-time.After(timeout):
			c.logger.Warn("stop timed out waiting for KV watcher")
		}
	}

	// Cancel all pending red-team timeouts.
	c.pendingReviews.Range(func(_, value any) bool {
		if p, ok := value.(*pendingRedTeam); ok && p.cancel != nil {
			p.cancel()
		}
		return true
	})

	c.running.Store(false)
	c.logger.Info("redteam component stopped",
		"reviews_posted", c.reviewsPosted.Load(),
		"reviews_done", c.reviewsDone.Load(),
		"reviews_skipped", c.reviewsSkipped.Load())

	return nil
}

