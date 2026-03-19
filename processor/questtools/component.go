package questtools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/executor"
	"github.com/c360studio/semdragons/processor/questbridge"
	"github.com/c360studio/semstreams/component"
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
	// When sandbox_url is configured, register sandbox-proxied versions of
	// file/exec tools. These overwrite the local filesystem handlers from
	// RegisterBuiltins while leaving terminal and DAG tools untouched.
	if c.config.SandboxURL != "" {
		sandboxClient := executor.NewSandboxClient(c.config.SandboxURL)
		c.toolRegistry.RegisterSandboxTools(sandboxClient)
		c.logger.Info("sandbox tools registered", "sandbox_url", c.config.SandboxURL)
	}
	if c.config.Search != nil && c.config.Search.Provider != "" {
		sp, err := executor.NewSearchProvider(*c.config.Search)
		if err != nil {
			c.logger.Warn("web_search tool disabled", "reason", err.Error())
		} else {
			c.toolRegistry.RegisterWebSearch(sp)
			c.logger.Info("web_search tool registered", "provider", c.config.Search.Provider)
		}
	}

	// Register graph_query tool backed by the board KV bucket.
	gc := semdragons.NewGraphClient(c.deps.NATSClient, c.boardConfig)
	c.toolRegistry.RegisterGraphQuery(c.buildGraphQueryFunc(gc))

	// Register graph_search tool — use global registry for multi-source routing,
	// single-URL fallback when registry is not configured.
	if reg := questbridge.GlobalGraphSources(); reg != nil {
		c.toolRegistry.RegisterGraphSearchWithRouter(reg)
	} else if c.config.GraphQLURL != "" {
		c.toolRegistry.RegisterGraphSearch(c.config.GraphQLURL)
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

// ToolRegistry returns the component's tool registry. Used by questbridge to
// share tool definitions instead of building a duplicate registry.
// Returns nil before Start has been called.
func (c *Component) ToolRegistry() *executor.ToolRegistry {
	return c.toolRegistry
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

// buildGraphQueryFunc returns an EntityQueryFunc that reads entities from the
// board KV bucket and formats them as a compact text summary for agents.
func (c *Component) buildGraphQueryFunc(gc *semdragons.GraphClient) executor.EntityQueryFunc {
	return func(ctx context.Context, entityType string, limit int) (string, error) {
		entities, err := gc.ListEntitiesByType(ctx, entityType, limit)
		if err != nil {
			return "", fmt.Errorf("list %s entities: %w", entityType, err)
		}
		return executor.FormatEntitySummary(entities, entityType), nil
	}
}
