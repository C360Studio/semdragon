package dm_approval

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
// COMPONENT - DM Approval as native semstreams processor
// =============================================================================

// Component implements the DM Approval processor as a semstreams component.
type Component struct {
	config      *Config
	deps        component.Dependencies
	boardConfig *semdragons.BoardConfig
	logger      *slog.Logger

	// Approval infrastructure
	router *NATSApprovalRouter

	// Internal state
	running  atomic.Bool
	mu       sync.RWMutex
	stopChan chan struct{}

	// Metrics
	requestsReceived atomic.Uint64
	responsesHandled atomic.Uint64
	errorsCount      atomic.Int64
	lastActivity     atomic.Value // time.Time
	startTime        time.Time
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
		Description: "Human-in-the-loop approval routing",
		Version:     "1.0.0",
	}
}

// InputPorts returns the ports this component accepts data on.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "approval-requests",
			Direction:   component.DirectionInput,
			Required:    true,
			Description: "Incoming approval requests",
			Config: &component.NATSPort{
				Subject: "approval.request.>",
			},
		},
	}
}

// OutputPorts returns the ports this component produces data on.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "approval-responses",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "Approval responses",
			Config: &component.NATSPort{
				Subject: "approval.response.>",
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
			"approval_timeout_min": {
				Type:        "int",
				Description: "Approval timeout in minutes",
				Default:     30,
				Category:    "approval",
			},
			"auto_approve": {
				Type:        "bool",
				Description: "Auto-approve all requests (for testing)",
				Default:     false,
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

	total := c.requestsReceived.Load() + c.responsesHandled.Load()
	uptime := time.Since(c.startTime).Seconds()
	if uptime > 0 {
		metrics.MessagesPerSecond = float64(total) / uptime
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
func (c *Component) Start(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running.Load() {
		return errors.New("component already running")
	}

	// Create approval router
	c.router = NewNATSApprovalRouter(c.deps.NATSClient, c.boardConfig, c.logger)

	c.startTime = time.Now()
	c.running.Store(true)
	c.lastActivity.Store(time.Now())

	c.logger.Info("dm_approval component started",
		"org", c.config.Org,
		"platform", c.config.Platform,
		"board", c.config.Board)

	return nil
}

// Stop gracefully shuts down the component.
func (c *Component) Stop(_ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running.Load() {
		return nil
	}

	close(c.stopChan)
	c.running.Store(false)
	c.logger.Info("dm_approval component stopped")

	return nil
}

// =============================================================================
// PUBLIC API
// =============================================================================

// RequestApproval sends an approval request and waits for response.
func (c *Component) RequestApproval(ctx context.Context, req domain.ApprovalRequest) (*domain.ApprovalResponse, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	// Auto-approve if configured
	if c.config.AutoApprove {
		c.requestsReceived.Add(1)
		c.responsesHandled.Add(1)
		c.lastActivity.Store(time.Now())
		return &domain.ApprovalResponse{
			RequestID:   req.ID,
			SessionID:   req.SessionID,
			Approved:    true,
			Reason:      "auto-approved",
			RespondedAt: time.Now(),
		}, nil
	}

	resp, err := c.router.RequestApproval(ctx, req)
	if err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "DMApproval", "RequestApproval", "send request")
	}

	c.requestsReceived.Add(1)
	c.responsesHandled.Add(1)
	c.lastActivity.Store(time.Now())

	return resp, nil
}

// WatchApprovals subscribes to approval responses for a session.
func (c *Component) WatchApprovals(ctx context.Context, filter domain.ApprovalFilter) (<-chan domain.ApprovalResponse, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	return c.router.WatchApprovals(ctx, filter)
}

// GetPendingApprovals returns all pending approval requests for a session.
func (c *Component) GetPendingApprovals(ctx context.Context, sessionID string) ([]domain.ApprovalRequest, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	return c.router.GetPendingApprovals(ctx, sessionID)
}

// RespondToApproval allows external systems to respond to pending approvals.
func (c *Component) RespondToApproval(ctx context.Context, sessionID, approvalID string, resp domain.ApprovalResponse) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	if err := c.router.RespondToApproval(ctx, sessionID, approvalID, resp); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "DMApproval", "RespondToApproval", "send response")
	}

	c.responsesHandled.Add(1)
	c.lastActivity.Store(time.Now())

	return nil
}

// GetRouter returns the underlying approval router.
func (c *Component) GetRouter() *NATSApprovalRouter {
	return c.router
}
