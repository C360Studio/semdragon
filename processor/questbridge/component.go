package questbridge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/processor/boardcontrol"
	"github.com/c360studio/semdragons/processor/executor"
	"github.com/c360studio/semdragons/processor/promptmanager"
	"github.com/c360studio/semdragons/processor/tokenbudget"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/model"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semdragons/domain"
)

// SubQuestPoster is the narrow interface questbridge needs from questboard.
// Using an interface rather than a direct questboard.Component reference avoids
// an import cycle and allows test mocking without starting a full component.
type SubQuestPoster interface {
	PostSubQuests(ctx context.Context, parentID domain.QuestID, subQuests []domain.Quest, decomposer domain.AgentID) ([]domain.Quest, error)
}

// =============================================================================
// COMPONENT - QuestBridge as a native semstreams processor
// =============================================================================
// Implements Discoverable + LifecycleComponent interfaces.
// Watches quest entities in KV for in_progress transitions and bridges them
// to the semstreams agentic-loop by publishing TaskMessages to the AGENT stream.
// Also consumes loop completion/failure events to emit executor lifecycle events.
// =============================================================================

// QuestLoopMapping tracks the relationship between a quest and its agentic loop.
// Persisted in the QUEST_LOOPS KV bucket for crash recovery.
type QuestLoopMapping struct {
	LoopID     string           `json:"loop_id"`
	QuestID    domain.QuestID   `json:"quest_id"`
	AgentID    domain.AgentID   `json:"agent_id"`
	SandboxDir string           `json:"sandbox_dir,omitempty"`
	TrustTier  domain.TrustTier `json:"trust_tier"`
	StartedAt  time.Time        `json:"started_at"`
}

// Component implements the QuestBridge processor as a semstreams component.
type Component struct {
	config      *Config
	deps        component.Dependencies
	graph       *semdragons.GraphClient
	logger      *slog.Logger
	boardConfig *domain.BoardConfig

	// Execution infrastructure
	registry        model.RegistryReader
	toolRegistry    *executor.ToolRegistry
	promptAssembler *promptmanager.PromptAssembler

	// QUEST_LOOPS KV bucket for crash recovery
	questLoopsBucket jetstream.KeyValue

	// questBoard posts sub-quests when a lead agent completes a DAG decomposition.
	// Optional: nil means DAG output from party quests is treated as normal output.
	questBoard SubQuestPoster

	// Board pause integration
	pauseChecker boardcontrol.PauseChecker // Optional: nil means always-running
	resumeSub    *natsclient.Subscription  // Subscription to board.control.resumed

	// Token budget enforcement
	tokenLedger *tokenbudget.TokenLedger

	// questCache stores the last-known status string keyed by entity KV key.
	// Populated during bootstrap; used to detect in_progress transitions.
	// Lock ordering: mu must be held before any per-entry operations if both
	// mu and a questCache entry are needed simultaneously.
	questCache  sync.Map // string → string (entity key → status)
	activeLoops sync.Map // string → *QuestLoopMapping (quest entity key → mapping)

	// escalatedAt tracks when quests entered escalated status (entity key → time.Time).
	// Used by sweepStaleEscalations to detect quests waiting too long for DM response.
	escalatedAt sync.Map

	// Internal state
	running  atomic.Bool
	mu       sync.RWMutex
	stopChan chan struct{}
	wg       sync.WaitGroup

	// Metrics
	tasksPublished atomic.Uint64
	loopsCompleted atomic.Uint64
	loopsFailed    atomic.Uint64
	errorsCount    atomic.Int64
	lastActivity   atomic.Value // stores time.Time
	startTime      time.Time
}

// Ensure Component implements the required interfaces at compile time.
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
		Description: "Bridges quest lifecycle to agentic loop execution",
		Version:     "1.0.0",
	}
}

// InputPorts returns the ports this component accepts data on.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "quest-entities",
			Direction:   component.DirectionInput,
			Required:    true,
			Description: "Quest entity state changes via KV watch",
			Config: &component.KVWatchPort{
				Bucket: "", // ENTITY_STATES bucket, watched dynamically via graph client
			},
		},
		{
			Name:        "loop-completions",
			Direction:   component.DirectionInput,
			Required:    true,
			Description: "Agentic loop completion/failure events from AGENT stream",
			Config: &component.JetStreamPort{
				StreamName: "AGENT",
				Subjects:   []string{"agent.complete.>", "agent.failed.>"},
			},
		},
	}
}

// OutputPorts returns the ports this component produces data on.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "agent-tasks",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "TaskMessage published to AGENT stream for agentic-loop consumption",
			Config: &component.JetStreamPort{
				StreamName: "AGENT",
				Subjects:   []string{"agent.task.*"},
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
			"max_iterations":     {Type: "int", Description: "Max iterations per agentic loop", Default: 20, Category: "execution"},
			"enable_builtins":    {Type: "bool", Description: "Register built-in tools", Default: true, Category: "execution"},
			"default_role":       {Type: "string", Description: "Default agent role", Default: "general", Category: "execution"},
			"sandbox_dir":        {Type: "string", Description: "Base directory for file operations", Default: "", Category: "execution"},
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

// DataFlow returns current data flow metrics.
func (c *Component) DataFlow() component.FlowMetrics {
	metrics := component.FlowMetrics{}
	if lastTime, ok := c.lastActivity.Load().(time.Time); ok {
		metrics.LastActivity = lastTime
	}
	published := c.tasksPublished.Load()
	uptime := time.Since(c.startTime).Seconds()
	if uptime > 0 {
		metrics.MessagesPerSecond = float64(published) / uptime
	}
	if published > 0 {
		metrics.ErrorRate = float64(c.errorsCount.Load()) / float64(published)
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

	// Create graph client (synchronous, no I/O).
	c.graph = semdragons.NewGraphClient(c.deps.NATSClient, c.boardConfig)

	if err := c.graph.EnsureBucket(ctx); err != nil {
		return fmt.Errorf("ensure board bucket: %w", err)
	}

	// Attach model registry from deps if available.
	c.registry = c.deps.ModelRegistry

	// Create tool registry for tool definition filtering.
	c.toolRegistry = executor.NewToolRegistry()
	if c.config.SandboxDir != "" {
		c.toolRegistry.SetSandboxDir(c.config.SandboxDir)
	}
	if c.config.EnableBuiltins {
		c.toolRegistry.RegisterBuiltins()
	}

	// Create prompt assembler when a domain catalog is provided.
	// Follows the same pattern as executor component.go.
	if c.config.DomainCatalog != nil {
		promptRegistry := promptmanager.NewPromptRegistry()
		promptRegistry.RegisterProviderStyles()
		promptRegistry.RegisterDomainCatalog(c.config.DomainCatalog)
		promptmanager.RegisterBuiltinFragments(promptRegistry)
		c.promptAssembler = promptmanager.NewPromptAssembler(promptRegistry)
	}

	// Get or create the QUEST_LOOPS KV bucket for crash-recovery mappings.
	bucket, err := c.deps.NATSClient.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      c.config.QuestLoopsBucket,
		Description: "Quest to agentic-loop mapping for crash recovery",
		History:     5,
	})
	if err != nil {
		return fmt.Errorf("create QUEST_LOOPS bucket: %w", err)
	}
	c.questLoopsBucket = bucket

	// Subscribe to board resume notifications for reconciliation.
	if c.pauseChecker != nil {
		sub, subErr := c.deps.NATSClient.Subscribe(ctx, boardcontrol.ResumeSubject(), func(msgCtx context.Context, _ *nats.Msg) {
			c.logger.Info("board resumed, reconciling deferred quests")
			c.reconcileOrphanedQuests(msgCtx)
		})
		if subErr != nil {
			c.logger.Warn("failed to subscribe to resume notifications", "error", subErr)
		} else {
			c.resumeSub = sub
		}
	}

	c.startTime = time.Now()
	c.running.Store(true)
	c.lastActivity.Store(time.Now())

	// Start KV watcher for quest entities (implements the KV twofer bootstrap protocol).
	c.wg.Add(2)
	go func() { defer c.wg.Done(); c.watchLoop(ctx) }()

	// Start JetStream consumer for loop completion/failure events.
	go func() { defer c.wg.Done(); c.consumeCompletions(ctx) }()

	// Start periodic sweep for escalated quests that timed out waiting for DM.
	if c.config.EscalationTimeoutMins > 0 {
		c.wg.Add(1)
		go func() { defer c.wg.Done(); c.sweepStaleEscalations(ctx) }()
	}

	c.logger.Info("questbridge component started",
		"org", c.config.Org,
		"platform", c.config.Platform,
		"board", c.config.Board,
		"stream", c.config.StreamName,
		"max_iterations", c.config.MaxIterations)

	return nil
}

// SetTokenLedger injects the shared token ledger for budget enforcement.
// When the budget is exceeded, new quest dispatches are deferred.
// Must be called before Start.
func (c *Component) SetTokenLedger(l *tokenbudget.TokenLedger) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running.Load() {
		c.logger.Warn("SetTokenLedger called while running; ignored")
		return
	}
	c.tokenLedger = l
}

// SetQuestBoard injects the quest board poster for sub-quest creation.
// When set, party quest completions that produce a valid DAG will trigger
// sub-quest posting instead of transitioning directly to in_review.
// Safe to call before or after Start — the reference is checked lazily when needed.
func (c *Component) SetQuestBoard(qb SubQuestPoster) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.questBoard = qb
}

// SetPauseChecker injects the board pause checker. When paused, quest
// transitions to in_progress are deferred. On resume, reconciliation fires.
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

// Stop gracefully shuts down the component.
func (c *Component) Stop(timeout time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running.Load() {
		return nil
	}

	close(c.stopChan)
	c.running.Store(false)

	if c.resumeSub != nil {
		c.resumeSub.Unsubscribe()
	}

	done := make(chan struct{})
	go func() { c.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(timeout):
		c.logger.Warn("graceful shutdown timed out", "timeout", timeout)
	}

	c.logger.Info("questbridge component stopped",
		"tasks_published", c.tasksPublished.Load(),
		"loops_completed", c.loopsCompleted.Load(),
		"loops_failed", c.loopsFailed.Load())

	return nil
}
