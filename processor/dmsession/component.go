package dmsession

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/errs"
)

// =============================================================================
// COMPONENT - DM Session as native semstreams processor
// =============================================================================
// Implements Discoverable + LifecycleComponent interfaces.
// Manages DM session lifecycle and event watching.
// =============================================================================

// Component implements the DM Session processor as a semstreams component.
type Component struct {
	config      *Config
	deps        component.Dependencies
	boardConfig *semdragons.BoardConfig
	logger      *slog.Logger

	// Session management
	manager *SessionManager

	// Internal state
	running  atomic.Bool
	mu       sync.RWMutex
	stopChan chan struct{}

	// Metrics
	sessionsStarted atomic.Uint64
	sessionsEnded   atomic.Uint64
	errorsCount     atomic.Int64
	lastActivity    atomic.Value // time.Time
	startTime       time.Time
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
		Name:        ComponentName,
		Type:        "processor",
		Description: "DM session lifecycle management",
		Version:     "1.0.0",
	}
}

// InputPorts returns the ports this component accepts data on.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "session-commands",
			Direction:   component.DirectionInput,
			Required:    false,
			Description: "Session management commands",
			Config: &component.NATSPort{
				Subject: "dm.session.command.>",
			},
		},
	}
}

// OutputPorts returns the ports this component produces data on.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "session-events",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "Session lifecycle events",
			Config: &component.NATSPort{
				Subject: domain.PredicateSessionStart,
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
			"default_mode": {
				Type:        "string",
				Description: "Default DM mode (manual, supervised, assisted, full_auto)",
				Default:     "manual",
				Category:    "session",
			},
			"max_concurrent": {
				Type:        "int",
				Description: "Maximum concurrent quests",
				Default:     10,
				Category:    "session",
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
		status.LastError = "errors encountered during session management"
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

	sessions := c.sessionsStarted.Load() + c.sessionsEnded.Load()
	uptime := time.Since(c.startTime).Seconds()
	if uptime > 0 {
		metrics.MessagesPerSecond = float64(sessions) / uptime
	}

	if sessions > 0 {
		metrics.ErrorRate = float64(c.errorsCount.Load()) / float64(sessions)
	}

	return metrics
}

// =============================================================================
// LIFECYCLE INTERFACE
// =============================================================================

// Initialize performs one-time setup.
func (c *Component) Initialize() error {
	if c.config == nil {
		return errors.New("config not set")
	}

	if c.deps.NATSClient == nil {
		return errors.New("NATS client required")
	}

	c.boardConfig = &semdragons.BoardConfig{
		Org:      c.config.Org,
		Platform: c.config.Platform,
		Board:    c.config.Board,
	}
	c.stopChan = make(chan struct{})

	return nil
}

// Start begins component operation.
func (c *Component) Start(ctx context.Context) error {
	// Check for cancellation before starting
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running.Load() {
		return errors.New("component already running")
	}

	// Create session manager
	c.manager = NewSessionManager(c.deps.NATSClient, c.boardConfig, c.logger)

	c.startTime = time.Now()
	c.running.Store(true)
	c.lastActivity.Store(time.Now())

	c.logger.Info("dm_session component started",
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

	close(c.stopChan)

	// End all active sessions
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for _, session := range c.manager.ListActiveSessions() {
		if _, err := c.manager.EndSession(ctx, session.ID); err != nil {
			c.logger.Warn("failed to end session on shutdown", "session_id", session.ID, "error", err)
		}
	}

	c.running.Store(false)
	c.logger.Info("dm_session component stopped")

	return nil
}

// =============================================================================
// PUBLIC API
// =============================================================================

// StartSession begins a new DM session.
func (c *Component) StartSession(ctx context.Context, config domain.SessionConfig) (*domain.Session, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	session, err := c.manager.StartSession(ctx, config)
	if err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "DMSession", "StartSession", "create session")
	}

	c.sessionsStarted.Add(1)
	c.lastActivity.Store(time.Now())

	return session, nil
}

// EndSession wraps up a DM session.
func (c *Component) EndSession(ctx context.Context, sessionID string) (*domain.SessionSummary, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	summary, err := c.manager.EndSession(ctx, sessionID)
	if err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "DMSession", "EndSession", "end session")
	}

	c.sessionsEnded.Add(1)
	c.lastActivity.Store(time.Now())

	return summary, nil
}

// GetSession retrieves a session by ID.
func (c *Component) GetSession(ctx context.Context, sessionID string) (*domain.Session, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	return c.manager.GetSession(ctx, sessionID)
}

// ListActiveSessions returns all active sessions.
func (c *Component) ListActiveSessions() []*domain.Session {
	if !c.running.Load() {
		return nil
	}

	return c.manager.ListActiveSessions()
}

// WatchEvents subscribes to the game event stream.
func (c *Component) WatchEvents(ctx context.Context, filter domain.EventFilter) (<-chan domain.GameEvent, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	return c.manager.WatchEvents(ctx, filter)
}

// GetSessionManager returns the underlying session manager.
func (c *Component) GetSessionManager() *SessionManager {
	return c.manager
}
