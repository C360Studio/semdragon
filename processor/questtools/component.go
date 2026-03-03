package questtools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semdragons/processor/executor"
	"github.com/c360studio/semstreams/component"

	"github.com/c360studio/semdragons/domain"
)

// Component consumes tool.execute.* messages from the AGENT JetStream stream,
// enforces tier/skill/sandbox gates via executor.ToolRegistry, and publishes
// tool.result.* responses back to the same stream.
//
// Lock ordering: mu (lifecycle) must always be acquired before any per-call locking.
type Component struct {
	config       *Config
	deps         component.Dependencies
	toolRegistry *executor.ToolRegistry
	logger       *slog.Logger
	boardConfig  *domain.BoardConfig

	// Lifecycle
	running   atomic.Bool
	startTime time.Time
	mu        sync.RWMutex

	// Metrics tracked with atomics to avoid locking on the hot path.
	toolsExecuted atomic.Uint64
	toolsFailed   atomic.Uint64
	errorsCount   atomic.Int64
	lastActivity  atomic.Value // stores time.Time
}

// Compile-time interface assertions.
var (
	_ component.Discoverable       = (*Component)(nil)
	_ component.LifecycleComponent = (*Component)(nil)
)

// Meta returns static component metadata.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        ComponentName,
		Type:        "processor",
		Description: "Tool execution gateway with tier/skill authorization",
		Version:     "1.0.0",
	}
}

// InputPorts declares the AGENT JetStream subjects this component reads.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "tool-execute",
			Direction:   component.DirectionInput,
			Required:    true,
			Description: "Tool execution requests from agentic-loop",
			Config: component.JetStreamPort{
				StreamName: "AGENT",
				Subjects:   []string{"tool.execute.*"},
			},
		},
	}
}

// OutputPorts declares the AGENT JetStream subjects this component writes.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "tool-result",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "Tool execution results back to agentic-loop",
			Config: component.JetStreamPort{
				StreamName: "AGENT",
				Subjects:   []string{"tool.result.*"},
			},
		},
	}
}

// ConfigSchema returns the configuration schema for this component.
func (c *Component) ConfigSchema() component.ConfigSchema {
	return component.ConfigSchema{
		Properties: map[string]component.PropertySchema{
			"org":                {Type: "string", Description: "Organization namespace", Default: "default", Category: "basic"},
			"platform":           {Type: "string", Description: "Platform/environment name", Default: "local", Category: "basic"},
			"board":              {Type: "string", Description: "Quest board name", Default: "main", Category: "basic"},
			"stream_name":        {Type: "string", Description: "AGENT stream name", Default: "AGENT", Category: "basic"},
			"quest_loops_bucket": {Type: "string", Description: "QUEST_LOOPS KV bucket name", Default: "QUEST_LOOPS", Category: "basic"},
			"timeout":            {Type: "string", Description: "Tool execution timeout duration", Default: "60s", Category: "execution"},
			"enable_builtins":    {Type: "bool", Description: "Register built-in file/search tools", Default: true, Category: "execution"},
			"sandbox_dir":        {Type: "string", Description: "Restrict file ops to this directory", Default: "", Category: "execution"},
		},
		Required: []string{"org", "platform", "board"},
	}
}

// Health returns current health status.
func (c *Component) Health() component.HealthStatus {
	running := c.running.Load()
	status := component.HealthStatus{
		Healthy:    running,
		LastCheck:  time.Now(),
		ErrorCount: int(c.errorsCount.Load()),
		Uptime:     time.Since(c.startTime),
	}
	if running {
		status.Status = "running"
	} else {
		status.Status = "stopped"
	}
	return status
}

// DataFlow returns throughput metrics.
func (c *Component) DataFlow() component.FlowMetrics {
	metrics := component.FlowMetrics{}
	if lastTime, ok := c.lastActivity.Load().(time.Time); ok {
		metrics.LastActivity = lastTime
	}
	executed := c.toolsExecuted.Load()
	uptime := time.Since(c.startTime).Seconds()
	if uptime > 0 {
		metrics.MessagesPerSecond = float64(executed) / uptime
	}
	return metrics
}

// Initialize performs one-time, non-I/O setup. Called before Start.
func (c *Component) Initialize() error {
	if c.config == nil {
		return errors.New("config not set")
	}
	if c.deps.NATSClient == nil {
		return errors.New("NATS client required")
	}
	c.boardConfig = c.config.ToBoardConfig()
	return nil
}

// Start begins consuming tool.execute.* messages and sets up infrastructure.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running.Load() {
		return errors.New("component already running")
	}

	c.toolRegistry = executor.NewToolRegistry()
	if c.config.SandboxDir != "" {
		c.toolRegistry.SetSandboxDir(c.config.SandboxDir)
	}
	if c.config.EnableBuiltins {
		c.toolRegistry.RegisterBuiltins()
	}

	c.startTime = time.Now()
	c.running.Store(true)
	c.lastActivity.Store(time.Now())

	if err := c.startConsumer(ctx); err != nil {
		c.running.Store(false)
		return fmt.Errorf("start tool execute consumer: %w", err)
	}

	c.logger.Info("questtools component started",
		"org", c.config.Org,
		"platform", c.config.Platform,
		"board", c.config.Board,
		"stream", c.config.StreamName,
		"enable_builtins", c.config.EnableBuiltins)

	return nil
}

// Stop signals the consumer goroutine and marks the component as stopped.
func (c *Component) Stop(_ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running.Load() {
		return nil
	}

	c.running.Store(false)
	c.logger.Info("questtools component stopped")
	return nil
}
