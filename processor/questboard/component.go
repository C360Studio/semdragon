// Package questboard provides a native semstreams component for quest lifecycle management.
// This processor handles quest posting, claiming, execution, and completion as events
// flow through the system via JetStream, with state persisted in NATS KV.
package questboard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
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
// COMPONENT - QuestBoard as native semstreams processor
// =============================================================================
// Implements Discoverable + LifecycleComponent interfaces.
// Manages quest lifecycle: post → claim → start → submit → complete/fail.
// State stored in KV, events emitted via JetStream subjects.
// =============================================================================

// Config holds the component configuration.
type Config struct {
	// BoardConfig contains org, platform, board for entity IDs and bucket naming.
	Org      string `json:"org" schema:"type:string,description:Organization namespace"`
	Platform string `json:"platform" schema:"type:string,description:Platform/environment name"`
	Board    string `json:"board" schema:"type:string,description:Quest board name"`

	// DefaultMaxAttempts for quests without explicit setting.
	DefaultMaxAttempts int `json:"default_max_attempts" schema:"type:int,description:Default max attempts for quests"`

	// EnableEvaluation enables automatic boss battle evaluation on submission.
	EnableEvaluation bool `json:"enable_evaluation" schema:"type:bool,description:Enable automatic evaluation"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Org:                "default",
		Platform:           "local",
		Board:              "main",
		DefaultMaxAttempts: 3,
		EnableEvaluation:   true,
	}
}

// ToBoardConfig converts component config to semdragons BoardConfig.
func (c *Config) ToBoardConfig() *semdragons.BoardConfig {
	return &semdragons.BoardConfig{
		Org:      c.Org,
		Platform: c.Platform,
		Board:    c.Board,
	}
}

// Component implements the QuestBoard as a semstreams processor.
type Component struct {
	config  *Config
	deps    component.Dependencies
	storage *semdragons.Storage
	events  *semdragons.EventPublisher
	traces  *semdragons.TraceManager
	logger  *slog.Logger

	// Internal state
	boardConfig *semdragons.BoardConfig
	running     atomic.Bool
	mu          sync.RWMutex

	// Metrics
	messagesProcessed atomic.Uint64
	errorsCount       atomic.Int64
	lastActivity      atomic.Value // time.Time
	startTime         time.Time
}

// ensure Component implements the required interfaces.
var (
	_ component.Discoverable      = (*Component)(nil)
	_ component.LifecycleComponent = (*Component)(nil)
)

// =============================================================================
// DISCOVERABLE INTERFACE
// =============================================================================

// Meta returns basic component information.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "questboard",
		Type:        "processor",
		Description: "Quest lifecycle management - posting, claiming, execution, completion",
		Version:     "1.0.0",
	}
}

// InputPorts returns the ports this component accepts data on.
// QuestBoard is primarily API-driven, but can also react to events.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "quest-commands",
			Direction:   component.DirectionInput,
			Required:    false,
			Description: "Command messages for quest operations (post, claim, start, submit, complete, fail)",
			Config: &component.NATSRequestPort{
				Subject: "questboard.command.>",
				Timeout: "30s",
			},
		},
	}
}

// OutputPorts returns the ports this component produces data on.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "quest-lifecycle",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "Quest lifecycle events (posted, claimed, started, submitted, completed, failed, escalated, abandoned)",
			Config: &component.JetStreamPort{
				StreamName:      "QUEST_EVENTS",
				Subjects:        []string{"quest.lifecycle.>"},
				Storage:         "file",
				RetentionPolicy: "limits",
				RetentionDays:   30,
				Replicas:        1,
			},
		},
		{
			Name:        "battle-events",
			Direction:   component.DirectionOutput,
			Required:    false,
			Description: "Boss battle review events (started, verdict)",
			Config: &component.JetStreamPort{
				StreamName:      "BATTLE_EVENTS",
				Subjects:        []string{"battle.review.>"},
				Storage:         "file",
				RetentionPolicy: "limits",
				RetentionDays:   30,
				Replicas:        1,
			},
		},
		{
			Name:        "quest-state",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "Quest state persisted in KV",
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
				Description: "Organization namespace (e.g., 'c360')",
				Default:     "default",
				Category:    "basic",
			},
			"platform": {
				Type:        "string",
				Description: "Platform/environment name (e.g., 'prod', 'dev')",
				Default:     "local",
				Category:    "basic",
			},
			"board": {
				Type:        "string",
				Description: "Quest board name",
				Default:     "main",
				Category:    "basic",
			},
			"default_max_attempts": {
				Type:        "int",
				Description: "Default maximum attempts for quests",
				Default:     3,
				Minimum:     util.IntPtr(1),
				Category:    "basic",
			},
			"enable_evaluation": {
				Type:        "bool",
				Description: "Enable automatic boss battle evaluation",
				Default:     true,
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
	c.traces = semdragons.NewTraceManager()

	return nil
}

// Start begins component operation with the given context.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running.Load() {
		return errors.New("component already running")
	}

	// Create storage (KV bucket)
	storage, err := semdragons.CreateStorage(ctx, c.deps.NATSClient, c.boardConfig)
	if err != nil {
		return errs.Wrap(err, "QuestBoard", "Start", "create storage")
	}
	c.storage = storage

	// Create event publisher
	c.events = semdragons.NewEventPublisher(c.deps.NATSClient)

	c.startTime = time.Now()
	c.running.Store(true)
	c.lastActivity.Store(time.Now())

	c.logger.Info("questboard component started",
		"org", c.config.Org,
		"platform", c.config.Platform,
		"board", c.config.Board,
		"bucket", c.boardConfig.BucketName())

	return nil
}

// Stop gracefully shuts down the component.
// The timeout parameter is part of the LifecycleComponent interface but is not
// currently needed as this component has no background goroutines to wait for.
func (c *Component) Stop(_ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running.Load() {
		return nil
	}

	c.running.Store(false)
	c.logger.Info("questboard component stopped")

	return nil
}

// =============================================================================
// QUEST BOARD OPERATIONS
// =============================================================================

// Storage returns the underlying storage for external access.
func (c *Component) Storage() *semdragons.Storage {
	return c.storage
}

// BoardConfig returns the board configuration.
func (c *Component) BoardConfig() *semdragons.BoardConfig {
	return c.boardConfig
}

// PostQuest adds a new quest to the board.
func (c *Component) PostQuest(ctx context.Context, quest semdragons.Quest) (*semdragons.Quest, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	// Generate instance ID if not provided
	instance := semdragons.ExtractInstance(string(quest.ID))
	if instance == "" || instance == string(quest.ID) {
		instance = semdragons.GenerateInstance()
	}

	// Set full entity ID
	quest.ID = semdragons.QuestID(c.boardConfig.QuestEntityID(instance))

	// Create trace context for this quest
	var tc *natsclient.TraceContext
	if quest.ParentQuest != nil {
		tc = c.traces.StartQuestTraceWithParent(quest.ID, *quest.ParentQuest)
	} else {
		tc = c.traces.StartQuestTrace(quest.ID)
	}
	quest.TrajectoryID = tc.TraceID

	// Set defaults
	quest.Status = semdragons.QuestPosted
	quest.PostedAt = time.Now()
	if quest.MaxAttempts == 0 {
		quest.MaxAttempts = c.config.DefaultMaxAttempts
	}
	if quest.BaseXP == 0 {
		quest.BaseXP = semdragons.DefaultXPForDifficulty(quest.Difficulty)
	}
	if quest.MinTier == 0 {
		quest.MinTier = semdragons.TierFromDifficulty(quest.Difficulty)
	}

	// Store quest
	if err := c.storage.PutQuest(ctx, instance, &quest); err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "QuestBoard", "PostQuest", "store quest")
	}

	// Add to posted index
	if err := c.storage.AddQuestStatusIndex(ctx, semdragons.QuestPosted, instance); err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "QuestBoard", "PostQuest", "add to posted index")
	}

	// Add to guild priority index if applicable
	if quest.GuildPriority != nil {
		guildInstance := semdragons.ExtractInstance(string(*quest.GuildPriority))
		if err := c.storage.AddGuildQuestIndex(ctx, guildInstance, instance); err != nil {
			c.logger.Debug("failed to add guild quest index", "quest", quest.ID, "guild", *quest.GuildPriority, "error", err)
		}
	}

	// Emit event with trace context
	if err := c.events.PublishQuestPosted(ctx, semdragons.QuestPostedPayload{
		Quest:    quest,
		PostedAt: quest.PostedAt,
		Trace:    semdragons.TraceInfoFromTraceContext(tc),
	}); err != nil {
		c.logger.Debug("failed to publish quest posted event", "quest", quest.ID, "error", err)
	}

	return &quest, nil
}

// PostSubQuests decomposes a parent quest into sub-quests.
func (c *Component) PostSubQuests(ctx context.Context, parentID semdragons.QuestID, subQuests []semdragons.Quest, decomposer semdragons.AgentID) ([]semdragons.Quest, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	parentInstance := semdragons.ExtractInstance(string(parentID))

	// Load parent quest
	parent, err := c.storage.GetQuest(ctx, parentInstance)
	if err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "QuestBoard", "PostSubQuests", "load parent")
	}

	if parent.Status != semdragons.QuestClaimed && parent.Status != semdragons.QuestInProgress {
		return nil, fmt.Errorf("parent must be claimed or in_progress")
	}

	// Validate decomposer permissions
	decomposerInstance := semdragons.ExtractInstance(string(decomposer))
	agent, err := c.storage.GetAgent(ctx, decomposerInstance)
	if err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "QuestBoard", "PostSubQuests", "load decomposer")
	}

	perms := semdragons.TierPerms[semdragons.TierFromLevel(agent.Level)]
	if !perms.CanDecomposeQuest {
		return nil, errors.New("agent cannot decompose quests (requires Master+ tier)")
	}

	// Post each sub-quest
	posted := make([]semdragons.Quest, 0, len(subQuests))
	subQuestIDs := make([]semdragons.QuestID, 0, len(subQuests))

	for _, sq := range subQuests {
		sq.ParentQuest = &parentID
		sq.DecomposedBy = &decomposer

		result, err := c.PostQuest(ctx, sq)
		if err != nil {
			c.errorsCount.Add(1)
			return nil, errs.Wrap(err, "QuestBoard", "PostSubQuests", "post sub-quest")
		}

		posted = append(posted, *result)
		subQuestIDs = append(subQuestIDs, result.ID)

		// Add parent-child index
		childInstance := semdragons.ExtractInstance(string(result.ID))
		c.storage.AddToIndex(ctx, c.storage.ParentQuestIndexKey(parentInstance, childInstance))
	}

	// Update parent with sub-quest IDs
	parent.SubQuests = subQuestIDs
	parent.DecomposedBy = &decomposer
	if err := c.storage.PutQuest(ctx, parentInstance, parent); err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "QuestBoard", "PostSubQuests", "update parent")
	}

	return posted, nil
}

// AvailableQuests returns quests an agent is eligible to claim.
func (c *Component) AvailableQuests(ctx context.Context, agentID semdragons.AgentID, opts semdragons.QuestFilter) ([]semdragons.Quest, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())
	agentInstance := semdragons.ExtractInstance(string(agentID))

	agent, err := c.storage.GetAgent(ctx, agentInstance)
	if err != nil {
		return nil, errs.Wrap(err, "QuestBoard", "AvailableQuests", "load agent")
	}

	// Check agent can claim
	if agent.Status != semdragons.AgentIdle {
		return []semdragons.Quest{}, nil
	}
	if agent.CooldownUntil != nil && time.Now().Before(*agent.CooldownUntil) {
		return []semdragons.Quest{}, nil
	}

	// Get posted quest instances
	postedInstances, err := c.storage.ListQuestsByStatus(ctx, semdragons.QuestPosted)
	if err != nil {
		return nil, errs.Wrap(err, "QuestBoard", "AvailableQuests", "list posted")
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}

	agentTier := semdragons.TierFromLevel(agent.Level)
	available := make([]semdragons.Quest, 0, limit)

	// Get guild priority quests
	guildPriorityMap := make(map[string]bool)
	for _, guildID := range agent.Guilds {
		guildInstance := semdragons.ExtractInstance(string(guildID))
		guildQuests, _ := c.storage.ListQuestsByGuild(ctx, guildInstance)
		for _, qid := range guildQuests {
			guildPriorityMap[qid] = true
		}
	}

	// Sort: guild priority first
	sortedInstances := make([]string, 0, len(postedInstances))
	for _, inst := range postedInstances {
		if guildPriorityMap[inst] {
			sortedInstances = append([]string{inst}, sortedInstances...)
		} else {
			sortedInstances = append(sortedInstances, inst)
		}
	}

	for _, instance := range sortedInstances {
		if len(available) >= limit {
			break
		}

		quest, err := c.storage.GetQuest(ctx, instance)
		if err != nil {
			continue
		}

		// Filter checks
		if quest.Status != semdragons.QuestPosted {
			continue
		}
		if agentTier < quest.MinTier {
			continue
		}
		if quest.PartyRequired && (opts.PartyOnly == nil || !*opts.PartyOnly) {
			continue
		}
		if opts.MinDifficulty != nil && quest.Difficulty < *opts.MinDifficulty {
			continue
		}
		if opts.MaxDifficulty != nil && quest.Difficulty > *opts.MaxDifficulty {
			continue
		}
		if opts.GuildID != nil {
			guildInstance := semdragons.ExtractInstance(string(*opts.GuildID))
			if quest.GuildPriority == nil || semdragons.ExtractInstance(string(*quest.GuildPriority)) != guildInstance {
				continue
			}
		}

		// Check skills match
		if len(opts.Skills) > 0 {
			hasSkill := false
			for _, reqSkill := range opts.Skills {
				if slices.Contains(quest.RequiredSkills, reqSkill) {
					hasSkill = true
					break
				}
			}
			if !hasSkill {
				continue
			}
		}

		available = append(available, *quest)
	}

	return available, nil
}

// ClaimQuest assigns a quest to an agent.
func (c *Component) ClaimQuest(ctx context.Context, questID semdragons.QuestID, agentID semdragons.AgentID) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	questInstance := semdragons.ExtractInstance(string(questID))
	agentInstance := semdragons.ExtractInstance(string(agentID))

	// Load agent
	agent, err := c.storage.GetAgent(ctx, agentInstance)
	if err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "ClaimQuest", "load agent")
	}

	var updatedQuest semdragons.Quest

	// Atomic update
	err = c.storage.UpdateQuest(ctx, questInstance, func(quest *semdragons.Quest) error {
		if quest.Status != semdragons.QuestPosted {
			return fmt.Errorf("quest not available: %s", quest.Status)
		}

		if err := c.validateAgentCanClaim(agent, quest); err != nil {
			return err
		}

		now := time.Now()
		quest.Status = semdragons.QuestClaimed
		quest.ClaimedBy = &agentID
		quest.ClaimedAt = &now
		quest.Attempts++

		updatedQuest = *quest
		return nil
	})

	if err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "ClaimQuest", "update quest")
	}

	// Update indices
	c.storage.MoveQuestStatus(ctx, questInstance, semdragons.QuestPosted, semdragons.QuestClaimed)
	c.storage.AddAgentQuestIndex(ctx, agentInstance, questInstance)

	// Create span for claim event and emit
	if updatedQuest.ClaimedAt != nil {
		_, tc := c.traces.NewEventSpan(ctx, questID)
		c.events.PublishQuestClaimed(ctx, semdragons.QuestClaimedPayload{
			Quest:     updatedQuest,
			AgentID:   agentID,
			ClaimedAt: *updatedQuest.ClaimedAt,
			Trace:     semdragons.TraceInfoFromTraceContext(tc),
		})
	}

	return nil
}

// ClaimQuestForParty assigns a quest to a party.
func (c *Component) ClaimQuestForParty(ctx context.Context, questID semdragons.QuestID, partyID semdragons.PartyID) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	questInstance := semdragons.ExtractInstance(string(questID))
	partyInstance := semdragons.ExtractInstance(string(partyID))

	party, err := c.storage.GetParty(ctx, partyInstance)
	if err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "ClaimQuestForParty", "load party")
	}

	leadInstance := semdragons.ExtractInstance(string(party.Lead))
	agent, err := c.storage.GetAgent(ctx, leadInstance)
	if err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "ClaimQuestForParty", "load lead")
	}

	perms := semdragons.TierPerms[semdragons.TierFromLevel(agent.Level)]
	if !perms.CanLeadParty {
		return errors.New("party lead cannot lead parties (requires Master+ tier)")
	}

	var updatedQuest semdragons.Quest

	err = c.storage.UpdateQuest(ctx, questInstance, func(quest *semdragons.Quest) error {
		if quest.Status != semdragons.QuestPosted {
			return fmt.Errorf("quest not available: %s", quest.Status)
		}

		if quest.MinPartySize > 0 && len(party.Members) < quest.MinPartySize {
			return errors.New("party too small")
		}

		if semdragons.TierFromLevel(agent.Level) < quest.MinTier {
			return errors.New("party lead tier too low")
		}

		now := time.Now()
		quest.Status = semdragons.QuestClaimed
		quest.ClaimedBy = &party.Lead
		quest.PartyID = &partyID
		quest.ClaimedAt = &now
		quest.Attempts++

		updatedQuest = *quest
		return nil
	})

	if err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "ClaimQuestForParty", "update quest")
	}

	c.storage.MoveQuestStatus(ctx, questInstance, semdragons.QuestPosted, semdragons.QuestClaimed)

	// Create span for claim event
	_, tc := c.traces.NewEventSpan(ctx, questID)
	c.events.PublishQuestClaimed(ctx, semdragons.QuestClaimedPayload{
		Quest:     updatedQuest,
		AgentID:   party.Lead,
		PartyID:   &partyID,
		ClaimedAt: *updatedQuest.ClaimedAt,
		Trace:     semdragons.TraceInfoFromTraceContext(tc),
	})

	return nil
}

// AbandonQuest returns a quest to the board.
func (c *Component) AbandonQuest(ctx context.Context, questID semdragons.QuestID, reason string) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	questInstance := semdragons.ExtractInstance(string(questID))

	var updatedQuest semdragons.Quest
	var agentID semdragons.AgentID
	var partyID *semdragons.PartyID

	err := c.storage.UpdateQuest(ctx, questInstance, func(quest *semdragons.Quest) error {
		if quest.Status != semdragons.QuestClaimed && quest.Status != semdragons.QuestInProgress {
			return fmt.Errorf("quest not abandonable: %s", quest.Status)
		}

		if quest.ClaimedBy != nil {
			agentID = *quest.ClaimedBy
		}
		partyID = quest.PartyID

		quest.Status = semdragons.QuestPosted
		quest.ClaimedBy = nil
		quest.PartyID = nil
		quest.ClaimedAt = nil
		quest.StartedAt = nil

		updatedQuest = *quest
		return nil
	})

	if err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "AbandonQuest", "update quest")
	}

	// Update indices
	c.storage.RemoveQuestStatusIndex(ctx, semdragons.QuestClaimed, questInstance)
	c.storage.RemoveQuestStatusIndex(ctx, semdragons.QuestInProgress, questInstance)
	c.storage.AddQuestStatusIndex(ctx, semdragons.QuestPosted, questInstance)

	if agentID != "" {
		agentInstance := semdragons.ExtractInstance(string(agentID))
		c.storage.RemoveAgentQuestIndex(ctx, agentInstance, questInstance)
	}

	// Create span for abandon event
	_, abandonTC := c.traces.NewEventSpan(ctx, questID)
	c.events.PublishQuestAbandoned(ctx, semdragons.QuestAbandonedPayload{
		Quest:       updatedQuest,
		AgentID:     agentID,
		PartyID:     partyID,
		Reason:      reason,
		AbandonedAt: time.Now(),
		Trace:       semdragons.TraceInfoFromTraceContext(abandonTC),
	})

	return nil
}

// StartQuest marks a quest as in-progress.
func (c *Component) StartQuest(ctx context.Context, questID semdragons.QuestID) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	questInstance := semdragons.ExtractInstance(string(questID))

	var updatedQuest semdragons.Quest
	var agentID semdragons.AgentID
	var partyID *semdragons.PartyID

	err := c.storage.UpdateQuest(ctx, questInstance, func(quest *semdragons.Quest) error {
		if quest.Status != semdragons.QuestClaimed {
			return fmt.Errorf("quest not claimed: %s", quest.Status)
		}

		now := time.Now()
		quest.Status = semdragons.QuestInProgress
		quest.StartedAt = &now

		if quest.ClaimedBy != nil {
			agentID = *quest.ClaimedBy
		}
		partyID = quest.PartyID

		updatedQuest = *quest
		return nil
	})

	if err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "StartQuest", "update quest")
	}

	c.storage.MoveQuestStatus(ctx, questInstance, semdragons.QuestClaimed, semdragons.QuestInProgress)

	if updatedQuest.StartedAt != nil {
		_, tc := c.traces.NewEventSpan(ctx, questID)
		c.events.PublishQuestStarted(ctx, semdragons.QuestStartedPayload{
			Quest:     updatedQuest,
			AgentID:   agentID,
			PartyID:   partyID,
			StartedAt: *updatedQuest.StartedAt,
			Trace:     semdragons.TraceInfoFromTraceContext(tc),
		})
	}

	return nil
}

// SubmitResult submits quest output for review.
func (c *Component) SubmitResult(ctx context.Context, questID semdragons.QuestID, result any) (*semdragons.BossBattle, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	questInstance := semdragons.ExtractInstance(string(questID))

	var updatedQuest semdragons.Quest
	var agentID semdragons.AgentID
	var needsReview bool

	err := c.storage.UpdateQuest(ctx, questInstance, func(quest *semdragons.Quest) error {
		if quest.Status != semdragons.QuestInProgress {
			return fmt.Errorf("quest not in_progress: %s", quest.Status)
		}

		quest.Output = result
		needsReview = quest.Constraints.RequireReview

		if needsReview {
			quest.Status = semdragons.QuestInReview
		} else {
			now := time.Now()
			quest.Status = semdragons.QuestCompleted
			quest.CompletedAt = &now
		}

		if quest.ClaimedBy != nil {
			agentID = *quest.ClaimedBy
		}

		updatedQuest = *quest
		return nil
	})

	if err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "QuestBoard", "SubmitResult", "update quest")
	}

	// Update indices
	c.storage.RemoveQuestStatusIndex(ctx, semdragons.QuestInProgress, questInstance)
	if needsReview {
		c.storage.AddQuestStatusIndex(ctx, semdragons.QuestInReview, questInstance)
	} else {
		c.storage.AddQuestStatusIndex(ctx, semdragons.QuestCompleted, questInstance)
	}

	var battle *semdragons.BossBattle
	var battleID *semdragons.BattleID

	if needsReview {
		battle = c.createBossBattle(&updatedQuest, agentID)
		id := battle.ID
		battleID = &id

		// Store battle state
		battleInstance := semdragons.ExtractInstance(string(battle.ID))
		if err := c.storage.PutBattle(ctx, battleInstance, battle); err != nil {
			c.logger.Debug("failed to store battle state", "battle", battle.ID, "error", err)
		}

		_, battleTC := c.traces.NewEventSpan(ctx, questID)
		c.events.PublishBattleStarted(ctx, semdragons.BattleStartedPayload{
			Battle:    *battle,
			Quest:     updatedQuest,
			StartedAt: battle.StartedAt,
			Trace:     semdragons.TraceInfoFromTraceContext(battleTC),
		})
	}

	_, submitTC := c.traces.NewEventSpan(ctx, questID)
	c.events.PublishQuestSubmitted(ctx, semdragons.QuestSubmittedPayload{
		Quest:       updatedQuest,
		AgentID:     agentID,
		Result:      result,
		SubmittedAt: time.Now(),
		BattleID:    battleID,
		Trace:       semdragons.TraceInfoFromTraceContext(submitTC),
	})

	// If quest completed directly (no review), end the trace
	if !needsReview {
		c.traces.EndQuestTrace(questID)
	}

	return battle, nil
}

// CompleteQuest marks a quest as successfully completed.
func (c *Component) CompleteQuest(ctx context.Context, questID semdragons.QuestID, verdict semdragons.BattleVerdict) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	questInstance := semdragons.ExtractInstance(string(questID))

	var updatedQuest semdragons.Quest
	var agentID semdragons.AgentID
	var partyID *semdragons.PartyID
	var duration time.Duration

	err := c.storage.UpdateQuest(ctx, questInstance, func(quest *semdragons.Quest) error {
		if quest.Status != semdragons.QuestInReview && quest.Status != semdragons.QuestInProgress {
			return fmt.Errorf("quest not completable: %s", quest.Status)
		}

		now := time.Now()
		quest.Status = semdragons.QuestCompleted
		quest.CompletedAt = &now

		if quest.StartedAt != nil {
			duration = now.Sub(*quest.StartedAt)
		}

		if quest.ClaimedBy != nil {
			agentID = *quest.ClaimedBy
		}
		partyID = quest.PartyID

		updatedQuest = *quest
		return nil
	})

	if err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "CompleteQuest", "update quest")
	}

	// Update indices
	c.storage.RemoveQuestStatusIndex(ctx, semdragons.QuestInReview, questInstance)
	c.storage.RemoveQuestStatusIndex(ctx, semdragons.QuestInProgress, questInstance)
	c.storage.AddQuestStatusIndex(ctx, semdragons.QuestCompleted, questInstance)

	if agentID != "" {
		agentInstance := semdragons.ExtractInstance(string(agentID))
		c.storage.RemoveAgentQuestIndex(ctx, agentInstance, questInstance)
	}

	// Create final span for completion event
	_, completeTC := c.traces.NewEventSpan(ctx, questID)
	c.events.PublishQuestCompleted(ctx, semdragons.QuestCompletedPayload{
		Quest:       updatedQuest,
		AgentID:     agentID,
		PartyID:     partyID,
		Verdict:     verdict,
		CompletedAt: *updatedQuest.CompletedAt,
		Duration:    duration,
		Trace:       semdragons.TraceInfoFromTraceContext(completeTC),
	})

	// End trace for this quest (terminal state)
	c.traces.EndQuestTrace(questID)

	return nil
}

// FailQuest marks a quest as failed.
func (c *Component) FailQuest(ctx context.Context, questID semdragons.QuestID, reason string) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	questInstance := semdragons.ExtractInstance(string(questID))

	var updatedQuest semdragons.Quest
	var agentID semdragons.AgentID
	var partyID *semdragons.PartyID
	var reposted bool

	err := c.storage.UpdateQuest(ctx, questInstance, func(quest *semdragons.Quest) error {
		if quest.Status != semdragons.QuestInProgress && quest.Status != semdragons.QuestInReview {
			return fmt.Errorf("quest not failable: %s", quest.Status)
		}

		if quest.ClaimedBy != nil {
			agentID = *quest.ClaimedBy
		}
		partyID = quest.PartyID

		if quest.Attempts < quest.MaxAttempts {
			quest.Status = semdragons.QuestPosted
			quest.ClaimedBy = nil
			quest.PartyID = nil
			quest.ClaimedAt = nil
			quest.StartedAt = nil
			quest.Output = nil
			reposted = true
		} else {
			quest.Status = semdragons.QuestFailed
		}

		updatedQuest = *quest
		return nil
	})

	if err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "FailQuest", "update quest")
	}

	// Update indices
	c.storage.RemoveQuestStatusIndex(ctx, semdragons.QuestInProgress, questInstance)
	c.storage.RemoveQuestStatusIndex(ctx, semdragons.QuestInReview, questInstance)

	if reposted {
		c.storage.AddQuestStatusIndex(ctx, semdragons.QuestPosted, questInstance)
	} else {
		c.storage.AddQuestStatusIndex(ctx, semdragons.QuestFailed, questInstance)
	}

	if agentID != "" {
		agentInstance := semdragons.ExtractInstance(string(agentID))
		c.storage.RemoveAgentQuestIndex(ctx, agentInstance, questInstance)
	}

	// Create span for failure event
	_, failTC := c.traces.NewEventSpan(ctx, questID)
	c.events.PublishQuestFailed(ctx, semdragons.QuestFailedPayload{
		Quest:    updatedQuest,
		AgentID:  agentID,
		PartyID:  partyID,
		Reason:   reason,
		FailType: semdragons.FailureSoft,
		FailedAt: time.Now(),
		Attempt:  updatedQuest.Attempts,
		Reposted: reposted,
		Trace:    semdragons.TraceInfoFromTraceContext(failTC),
	})

	// End trace if quest reached terminal state (not reposted)
	if !reposted {
		c.traces.EndQuestTrace(questID)
	}

	return nil
}

// EscalateQuest flags a quest for higher-level attention.
func (c *Component) EscalateQuest(ctx context.Context, questID semdragons.QuestID, reason string) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	questInstance := semdragons.ExtractInstance(string(questID))

	var updatedQuest semdragons.Quest
	var agentID semdragons.AgentID
	var partyID *semdragons.PartyID

	err := c.storage.UpdateQuest(ctx, questInstance, func(quest *semdragons.Quest) error {
		if quest.Status == semdragons.QuestCompleted || quest.Status == semdragons.QuestCancelled || quest.Status == semdragons.QuestEscalated {
			return fmt.Errorf("quest cannot be escalated: %s", quest.Status)
		}

		if quest.ClaimedBy != nil {
			agentID = *quest.ClaimedBy
		}
		partyID = quest.PartyID

		quest.Status = semdragons.QuestEscalated
		quest.Escalated = true

		updatedQuest = *quest
		return nil
	})

	if err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "EscalateQuest", "update quest")
	}

	// Remove from all active indices
	for _, status := range []semdragons.QuestStatus{semdragons.QuestPosted, semdragons.QuestClaimed, semdragons.QuestInProgress, semdragons.QuestInReview, semdragons.QuestFailed} {
		c.storage.RemoveQuestStatusIndex(ctx, status, questInstance)
	}
	c.storage.AddQuestStatusIndex(ctx, semdragons.QuestEscalated, questInstance)

	if agentID != "" {
		agentInstance := semdragons.ExtractInstance(string(agentID))
		c.storage.RemoveAgentQuestIndex(ctx, agentInstance, questInstance)
	}

	// Create span for escalation event
	_, escTC := c.traces.NewEventSpan(ctx, questID)
	c.events.PublishQuestEscalated(ctx, semdragons.QuestEscalatedPayload{
		Quest:       updatedQuest,
		AgentID:     agentID,
		PartyID:     partyID,
		Reason:      reason,
		EscalatedAt: time.Now(),
		Attempts:    updatedQuest.Attempts,
		Trace:       semdragons.TraceInfoFromTraceContext(escTC),
	})

	// End trace for escalated quest (terminal state requiring DM attention)
	c.traces.EndQuestTrace(questID)

	return nil
}

// GetQuest returns a quest by ID.
func (c *Component) GetQuest(ctx context.Context, questID semdragons.QuestID) (*semdragons.Quest, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	instance := semdragons.ExtractInstance(string(questID))
	return c.storage.GetQuest(ctx, instance)
}

// BoardStats returns current board statistics.
func (c *Component) BoardStats(ctx context.Context) (*semdragons.BoardStats, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	stats, err := c.storage.GetBoardStats(ctx)
	if err != nil {
		return nil, err
	}

	// Initialize maps if nil
	if stats.ByDifficulty == nil {
		stats.ByDifficulty = make(map[semdragons.QuestDifficulty]int)
	}
	if stats.BySkill == nil {
		stats.BySkill = make(map[semdragons.SkillTag]int)
	}

	// Count by status
	for _, status := range []semdragons.QuestStatus{semdragons.QuestPosted, semdragons.QuestClaimed, semdragons.QuestInProgress, semdragons.QuestCompleted, semdragons.QuestFailed, semdragons.QuestEscalated} {
		instances, err := c.storage.ListQuestsByStatus(ctx, status)
		if err != nil {
			continue
		}
		count := len(instances)
		switch status {
		case semdragons.QuestPosted:
			stats.TotalPosted = count
		case semdragons.QuestClaimed:
			stats.TotalClaimed = count
		case semdragons.QuestInProgress:
			stats.TotalInProgress = count
		case semdragons.QuestCompleted:
			stats.TotalCompleted = count
		case semdragons.QuestFailed:
			stats.TotalFailed = count
		case semdragons.QuestEscalated:
			stats.TotalEscalated = count
		}
	}

	return stats, nil
}

// =============================================================================
// HELPERS
// =============================================================================

func (c *Component) validateAgentCanClaim(agent *semdragons.Agent, quest *semdragons.Quest) error {
	if agent.Status != semdragons.AgentIdle {
		return fmt.Errorf("agent not idle: %s", agent.Status)
	}

	if agent.CooldownUntil != nil && time.Now().Before(*agent.CooldownUntil) {
		return errors.New("agent on cooldown")
	}

	if semdragons.TierFromLevel(agent.Level) < quest.MinTier {
		return errors.New("agent tier too low")
	}

	if quest.PartyRequired {
		return errors.New("quest requires party")
	}

	perms := semdragons.TierPerms[semdragons.TierFromLevel(agent.Level)]
	if agent.CurrentQuest != nil && perms.MaxConcurrent <= 1 {
		return errors.New("agent at concurrent quest limit")
	}

	if len(quest.RequiredSkills) > 0 {
		hasSkill := false
		for _, required := range quest.RequiredSkills {
			if agent.HasSkill(required) {
				hasSkill = true
				break
			}
		}
		if !hasSkill {
			return errors.New("agent lacks required skills")
		}
	}

	return nil
}

func (c *Component) createBossBattle(quest *semdragons.Quest, agentID semdragons.AgentID) *semdragons.BossBattle {
	instance := semdragons.GenerateInstance()
	battleID := semdragons.BattleID(c.boardConfig.BattleEntityID(instance))

	battle := &semdragons.BossBattle{
		ID:        battleID,
		QuestID:   quest.ID,
		AgentID:   agentID,
		Level:     quest.Constraints.ReviewLevel,
		Status:    semdragons.BattleActive,
		StartedAt: time.Now(),
	}

	switch quest.Constraints.ReviewLevel {
	case semdragons.ReviewAuto:
		battle.Criteria = []semdragons.ReviewCriterion{
			{Name: "format", Description: "Output format validation", Weight: 0.5, Threshold: 0.9},
			{Name: "completeness", Description: "All required fields present", Weight: 0.5, Threshold: 0.9},
		}
		battle.Judges = []semdragons.Judge{
			{ID: "judge-auto", Type: semdragons.JudgeAutomated, Config: map[string]any{}},
		}

	case semdragons.ReviewStandard:
		battle.Criteria = []semdragons.ReviewCriterion{
			{Name: "correctness", Description: "Output is correct", Weight: 0.4, Threshold: 0.7},
			{Name: "quality", Description: "Output quality", Weight: 0.3, Threshold: 0.6},
			{Name: "completeness", Description: "All requirements met", Weight: 0.3, Threshold: 0.8},
		}
		battle.Judges = []semdragons.Judge{
			{ID: "judge-auto", Type: semdragons.JudgeAutomated, Config: map[string]any{}},
			{ID: "judge-llm-1", Type: semdragons.JudgeLLM, Config: map[string]any{}},
		}

	case semdragons.ReviewStrict:
		battle.Criteria = []semdragons.ReviewCriterion{
			{Name: "correctness", Description: "Output is correct", Weight: 0.3, Threshold: 0.8},
			{Name: "quality", Description: "Output quality", Weight: 0.25, Threshold: 0.75},
			{Name: "completeness", Description: "All requirements met", Weight: 0.25, Threshold: 0.85},
			{Name: "robustness", Description: "Edge cases handled", Weight: 0.2, Threshold: 0.7},
		}
		battle.Judges = []semdragons.Judge{
			{ID: "judge-auto", Type: semdragons.JudgeAutomated, Config: map[string]any{}},
			{ID: "judge-llm-1", Type: semdragons.JudgeLLM, Config: map[string]any{}},
			{ID: "judge-llm-2", Type: semdragons.JudgeLLM, Config: map[string]any{}},
		}

	case semdragons.ReviewHuman:
		battle.Criteria = []semdragons.ReviewCriterion{
			{Name: "correctness", Description: "Output is correct", Weight: 0.3, Threshold: 0.8},
			{Name: "quality", Description: "Output quality", Weight: 0.25, Threshold: 0.75},
			{Name: "completeness", Description: "All requirements met", Weight: 0.25, Threshold: 0.85},
			{Name: "creativity", Description: "Thoughtful approach", Weight: 0.2, Threshold: 0.6},
		}
		battle.Judges = []semdragons.Judge{
			{ID: "judge-auto", Type: semdragons.JudgeAutomated, Config: map[string]any{}},
			{ID: "judge-llm-1", Type: semdragons.JudgeLLM, Config: map[string]any{}},
			{ID: "judge-human", Type: semdragons.JudgeHuman, Config: map[string]any{}},
		}
	}

	return battle
}
