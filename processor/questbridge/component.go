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
	"github.com/c360studio/semdragons/semsource"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/model"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/storage"
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

// QuestFailer is the narrow interface questbridge needs from questboard
// to delegate failure transitions through the triage gate.
type QuestFailer interface {
	FailQuest(ctx context.Context, questID domain.QuestID, reason string) error
}

// ClarificationAnswerer abstracts LLM inference for auto-DM clarification answering.
// The default implementation uses the model registry's "dm-chat" capability
// with a simple OpenAI-compatible HTTP call.
type ClarificationAnswerer interface {
	AnswerClarification(ctx context.Context, questTitle, questDescription string, question string) (string, error)
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
	registry            model.RegistryReader
	toolRegistry        *executor.ToolRegistry
	promptAssembler     *promptmanager.PromptAssembler

	// QUEST_LOOPS KV bucket for crash recovery
	questLoopsBucket jetstream.KeyValue

	// clarificationAnswerer auto-answers agent clarification questions when DMMode
	// is full_auto. Optional: nil means escalated quests wait for human DM response.
	clarificationAnswerer ClarificationAnswerer

	// Sandbox workspace lifecycle management.
	// When sandboxClient is non-nil, questbridge creates per-quest workspaces
	// before dispatch and snapshots files to the artifact store on completion.
	// The artifact store is resolved lazily via ComponentRegistry at snapshot time.
	sandboxClient *executor.SandboxClient

	// Semsource manifest client for injecting graph knowledge into agent prompts.
	// Optional: nil means manifest section is omitted from entity knowledge.
	manifestClient *semsource.ManifestClient

	// Graph manifest client for injecting graph-gateway contents summary.
	// Optional: nil means graph contents section is omitted from entity knowledge.
	graphManifestClient *semsource.GraphManifestClient

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
			"sandbox_url":        {Type: "string", Description: "Sandbox container HTTP URL for workspace lifecycle", Default: "", Category: "execution"},
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

	// Initialize auto-DM clarification answerer when in full_auto mode.
	if c.config.DMMode == domain.DMFullAuto && c.registry != nil && c.clarificationAnswerer == nil {
		c.clarificationAnswerer = newRegistryAnswerer(c.registry)
		c.logger.Info("auto-DM clarification answerer enabled (full_auto mode)")
	}

	// Build a local tool registry as fallback. toolsForQuest() resolves
	// the registry lazily from questtools via ComponentRegistry at runtime,
	// so this registry is only used when questtools is not available.
	c.toolRegistry = executor.NewToolRegistry()
	if c.config.SandboxDir != "" {
		c.toolRegistry.SetSandboxDir(c.config.SandboxDir)
	}
	if c.config.EnableBuiltins {
		c.toolRegistry.RegisterBuiltins()
	}

	// Create sandbox client when sandbox_url is configured.
	if c.config.SandboxURL != "" {
		c.sandboxClient = executor.NewSandboxClient(c.config.SandboxURL)
		c.logger.Info("sandbox workspace lifecycle enabled", "sandbox_url", c.config.SandboxURL)
	}

	// Self-initialize semsource manifest client when semsource_url is configured.
	// This replaces the old SetManifestClient setter injection path.
	if c.config.SemsourceURL != "" {
		c.manifestClient = semsource.NewManifestClient(c.config.SemsourceURL, c.logger)
		c.logger.Info("semsource manifest client initialized", "semsource_url", c.config.SemsourceURL)
	}

	// Self-initialize graph manifest client when graphql_url is configured.
	if c.config.GraphQLURL != "" {
		c.graphManifestClient = semsource.NewGraphManifestClient(c.config.GraphQLURL, c.logger)
		c.logger.Info("graph manifest client initialized", "graphql_url", c.config.GraphQLURL)
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

// resolveQuestBoard resolves questboard's SubQuestPoster from the ComponentRegistry.
// Returns nil when registry is unavailable or questboard doesn't implement the interface.
func (c *Component) resolveQuestBoard() SubQuestPoster {
	if c.deps.ComponentRegistry == nil {
		return nil
	}
	comp := c.deps.ComponentRegistry.Component("questboard")
	if comp == nil {
		return nil
	}
	ref, ok := comp.(SubQuestPoster)
	if !ok {
		c.logger.Warn("questboard component does not implement SubQuestPoster",
			"type", comp.Meta().Type)
		return nil
	}
	return ref
}

// resolveQuestFailer resolves questboard's QuestFailer from the ComponentRegistry.
// Returns nil when registry is unavailable or questboard doesn't implement the interface.
func (c *Component) resolveQuestFailer() QuestFailer {
	if c.deps.ComponentRegistry == nil {
		return nil
	}
	comp := c.deps.ComponentRegistry.Component("questboard")
	if comp == nil {
		return nil
	}
	ref, ok := comp.(QuestFailer)
	if !ok {
		c.logger.Warn("questboard component does not implement QuestFailer",
			"type", comp.Meta().Type)
		return nil
	}
	return ref
}

// resolveToolRegistrySource resolves questtools' ToolRegistrySource from the ComponentRegistry.
// Returns nil when registry is unavailable or questtools doesn't implement the interface.
func (c *Component) resolveToolRegistrySource() ToolRegistrySource {
	if c.deps.ComponentRegistry == nil {
		return nil
	}
	comp := c.deps.ComponentRegistry.Component("questtools")
	if comp == nil {
		return nil
	}
	ref, ok := comp.(ToolRegistrySource)
	if !ok {
		c.logger.Warn("questtools component does not implement ToolRegistrySource",
			"type", comp.Meta().Type)
		return nil
	}
	return ref
}

// SetClarificationAnswerer injects the auto-DM answerer. When set and DMMode
// is full_auto, escalated quests are automatically answered via LLM.
// Safe to call before or after Start — the reference is checked lazily when needed.
func (c *Component) SetClarificationAnswerer(ca ClarificationAnswerer) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clarificationAnswerer = ca
}

// ToolRegistrySource provides tool definitions. Implemented by questtools.Component.
// Using an interface avoids an import cycle and allows test mocking.
type ToolRegistrySource interface {
	ToolRegistry() *executor.ToolRegistry
}

// getArtifactStore resolves the filestore lazily from the ComponentRegistry.
// Returns nil when the registry is unavailable or filestore is not running.
// Called at snapshot time so a restarted filestore component is always current.
func (c *Component) getArtifactStore() storage.Store {
	return domain.ResolveArtifactStore(c.deps.ComponentRegistry, c.logger)
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
