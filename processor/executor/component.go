package executor

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/promptmanager"
	"github.com/c360studio/semdragons/processor/questboard"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/model"
	"github.com/c360studio/semstreams/pkg/errs"
)

// =============================================================================
// COMPONENT - Executor as native semstreams processor
// =============================================================================
// Implements Discoverable + LifecycleComponent interfaces.
// Manages quest execution through LLM calls and tool invocations.
// =============================================================================

// Component implements the Executor processor as a semstreams component.
type Component struct {
	config      *Config
	deps        component.Dependencies
	graph       *semdragons.GraphClient
	logger      *slog.Logger
	boardConfig *semdragons.BoardConfig

	// Execution infrastructure
	registry     model.RegistryReader
	toolRegistry *ToolRegistry
	executor     *DefaultExecutor

	// Internal state
	running  atomic.Bool
	mu       sync.RWMutex
	stopChan chan struct{}

	// Metrics
	executionsStarted   atomic.Uint64
	executionsCompleted atomic.Uint64
	executionsFailed    atomic.Uint64
	toolCallsTotal      atomic.Uint64
	errorsCount         atomic.Int64
	lastActivity        atomic.Value // time.Time
	startTime           time.Time
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
		Name:        "executor",
		Type:        "processor",
		Description: "Quest execution engine for agent LLM calls",
		Version:     "1.0.0",
	}
}

// InputPorts returns the ports this component accepts data on.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "quest-events",
			Direction:   component.DirectionInput,
			Required:    true,
			Description: "Quest events triggering execution",
			Config: &component.NATSPort{
				Subject: domain.PredicateQuestStarted,
			},
		},
	}
}

// OutputPorts returns the ports this component produces data on.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "execution-events",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "Execution lifecycle events",
			Config: &component.NATSPort{
				Subject: domain.PredicateExecutionCompleted,
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
			"max_turns": {
				Type:        "int",
				Description: "Maximum tool-call loops per execution",
				Default:     20,
				Category:    "execution",
			},
			"max_tokens": {
				Type:        "int",
				Description: "Token budget per execution",
				Default:     50000,
				Category:    "execution",
			},
			"sandbox_dir": {
				Type:        "string",
				Description: "Base directory for file operations",
				Default:     "",
				Category:    "execution",
			},
			"enable_builtins": {
				Type:        "bool",
				Description: "Register built-in tools",
				Default:     true,
				Category:    "execution",
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
		status.LastError = "errors encountered during execution"
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

	executions := c.executionsCompleted.Load() + c.executionsFailed.Load()
	uptime := time.Since(c.startTime).Seconds()
	if uptime > 0 {
		metrics.MessagesPerSecond = float64(executions) / uptime
	}

	if executions > 0 {
		metrics.ErrorRate = float64(c.errorsCount.Load()) / float64(executions)
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
		return errs.Wrap(err, "Executor", "Start", "create graph client")
	}

	// Create tool registry
	c.toolRegistry = NewToolRegistry()
	if c.config.SandboxDir != "" {
		c.toolRegistry.SetSandboxDir(c.config.SandboxDir)
	}
	if c.config.EnableBuiltins {
		c.toolRegistry.RegisterBuiltins()
	}

	// Create prompt assembler if domain catalog is configured
	opts := []Option{
		WithMaxTurns(c.config.MaxTurns),
		WithMaxTokens(c.config.MaxTokens),
	}
	if c.config.DomainCatalog != nil {
		promptRegistry := promptmanager.NewPromptRegistry()
		promptRegistry.RegisterProviderStyles()
		promptRegistry.RegisterDomainCatalog(c.config.DomainCatalog)
		opts = append(opts, WithPromptAssembler(promptmanager.NewPromptAssembler(promptRegistry)))
	}

	// Create executor
	c.executor = NewDefaultExecutor(c.registry, c.toolRegistry, opts...)

	c.startTime = time.Now()
	c.running.Store(true)
	c.lastActivity.Store(time.Now())

	c.logger.Info("executor component started",
		"org", c.config.Org,
		"platform", c.config.Platform,
		"board", c.config.Board,
		"max_turns", c.config.MaxTurns,
		"max_tokens", c.config.MaxTokens)

	return nil
}

// Stop gracefully shuts down the component.
// Timeout is unused: shutdown is non-blocking with no background goroutines to wait for.
func (c *Component) Stop(_ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running.Load() {
		return nil
	}

	close(c.stopChan)

	c.running.Store(false)
	c.logger.Info("executor component stopped")

	return nil
}

// createGraphClient creates the graph client for the component.
// Context is unused: NewGraphClient is a synchronous in-memory constructor.
func (c *Component) createGraphClient(_ context.Context) error {
	c.graph = semdragons.NewGraphClient(c.deps.NATSClient, c.boardConfig)
	return nil
}

// =============================================================================
// PUBLIC API
// =============================================================================

// Execute runs a quest for an agent and returns the result.
func (c *Component) Execute(ctx context.Context, agent *agentprogression.Agent, quest *questboard.Quest) (*ExecutionResult, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	c.executionsStarted.Add(1)
	now := time.Now()

	// Publish execution started event
	if err := SubjectExecutionStarted.Publish(ctx, c.deps.NATSClient, ExecutionStartedPayload{
		QuestID:    quest.ID,
		QuestTitle: quest.Title,
		AgentID:    agent.ID,
		AgentName:  agent.Name,
		LoopID:     "", // Will be set by executor
		MaxTurns:   c.config.MaxTurns,
		MaxTokens:  c.config.MaxTokens,
		ToolCount:  len(c.toolRegistry.GetToolsForQuest(quest, agent)),
		Timestamp:  now,
	}); err != nil {
		c.errorsCount.Add(1)
		// Don't fail for event failure
	}

	// Execute the quest
	result, err := c.executor.Execute(ctx, agent, quest)
	if err != nil {
		c.executionsFailed.Add(1)
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "Executor", "Execute", "run quest")
	}

	c.toolCallsTotal.Add(uint64(result.ToolCalls))
	c.lastActivity.Store(time.Now())

	// Publish completion or failure event
	if result.Status == StatusComplete {
		c.executionsCompleted.Add(1)
		if err := SubjectExecutionCompleted.Publish(ctx, c.deps.NATSClient, ExecutionCompletedPayload{
			QuestID:          quest.ID,
			AgentID:          agent.ID,
			LoopID:           result.LoopID,
			Status:           result.Status,
			TotalTurns:       len(result.Trajectory),
			TotalToolCalls:   result.ToolCalls,
			PromptTokens:     result.TokenUsage.PromptTokens,
			CompletionTokens: result.TokenUsage.CompletionTokens,
			Duration:         result.Duration,
			Timestamp:        time.Now(),
		}); err != nil {
			c.errorsCount.Add(1)
		}
	} else {
		c.executionsFailed.Add(1)
		if err := SubjectExecutionFailed.Publish(ctx, c.deps.NATSClient, ExecutionFailedPayload{
			QuestID:        quest.ID,
			AgentID:        agent.ID,
			LoopID:         result.LoopID,
			Status:         result.Status,
			Error:          result.Error,
			TurnsCompleted: len(result.Trajectory),
			ToolCallsMade:  result.ToolCalls,
			Duration:       result.Duration,
			Timestamp:      time.Now(),
		}); err != nil {
			c.errorsCount.Add(1)
		}
	}

	c.logger.Info("quest execution completed",
		"quest_id", quest.ID,
		"agent_id", agent.ID,
		"status", result.Status,
		"tool_calls", result.ToolCalls,
		"duration", result.Duration)

	return result, nil
}

// GetToolRegistry returns the tool registry for external tool registration.
func (c *Component) GetToolRegistry() *ToolRegistry {
	return c.toolRegistry
}

// SetModelRegistry sets the model registry for LLM endpoint resolution.
func (c *Component) SetModelRegistry(registry model.RegistryReader) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.registry = registry
}

// GetExecutor returns the underlying executor for direct access.
func (c *Component) GetExecutor() AgentExecutor {
	return c.executor
}
