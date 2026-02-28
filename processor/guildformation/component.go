// Package guildformation provides a native semstreams component for guild
// management and auto-formation suggestions. It wraps the GuildFormationEngine
// and exposes it as a semstreams component.
package guildformation

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/errs"

	semdragons "github.com/c360studio/semdragons"
)

// =============================================================================
// COMPONENT - GuildFormation as native semstreams processor
// =============================================================================
// Implements Discoverable + LifecycleComponent interfaces.
// Wraps the GuildFormationEngine to provide guild management as a component.
// =============================================================================

// Config holds the component configuration.
type Config struct {
	// BoardConfig contains org, platform, board for entity IDs and bucket naming.
	Org      string `json:"org" schema:"type:string,description:Organization namespace"`
	Platform string `json:"platform" schema:"type:string,description:Platform/environment name"`
	Board    string `json:"board" schema:"type:string,description:Quest board name"`

	// Guild formation settings
	MinFounderLevel     int     `json:"min_founder_level" schema:"type:int,description:Minimum level to found guild"`
	FoundingXPCost      int64   `json:"founding_xp_cost" schema:"type:int,description:XP cost to found guild"`
	DefaultMaxMembers   int     `json:"default_max_members" schema:"type:int,description:Default max members per guild"`
	MinClusterSize      int     `json:"min_cluster_size" schema:"type:int,description:Minimum agents for cluster suggestion"`
	MinClusterStrength  float64 `json:"min_cluster_strength" schema:"type:float,description:Minimum Jaccard similarity for clusters"`
	MinAgentLevel       int     `json:"min_agent_level" schema:"type:int,description:Minimum agent level for guild consideration"`
	RequireQualityScore float64 `json:"require_quality_score" schema:"type:float,description:Minimum avg quality score"`
	TotalSkillCount     int     `json:"total_skill_count" schema:"type:int,description:Total skills for diversity calculation"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() Config {
	defaults := semdragons.DefaultFormationConfig()
	return Config{
		Org:                 "default",
		Platform:            "local",
		Board:               "main",
		MinFounderLevel:     defaults.MinFounderLevel,
		FoundingXPCost:      defaults.FoundingXPCost,
		DefaultMaxMembers:   defaults.DefaultMaxMembers,
		MinClusterSize:      defaults.MinClusterSize,
		MinClusterStrength:  defaults.MinClusterStrength,
		MinAgentLevel:       defaults.MinAgentLevel,
		RequireQualityScore: defaults.RequireQualityScore,
		TotalSkillCount:     defaults.TotalSkillCount,
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

// ToFormationConfig converts component config to semdragons GuildFormationConfig.
func (c *Config) ToFormationConfig() semdragons.GuildFormationConfig {
	return semdragons.GuildFormationConfig{
		MinFounderLevel:     c.MinFounderLevel,
		FoundingXPCost:      c.FoundingXPCost,
		DefaultMaxMembers:   c.DefaultMaxMembers,
		MinClusterSize:      c.MinClusterSize,
		MinClusterStrength:  c.MinClusterStrength,
		MinAgentLevel:       c.MinAgentLevel,
		RequireQualityScore: c.RequireQualityScore,
		TotalSkillCount:     c.TotalSkillCount,
	}
}

// Component implements GuildFormation as a semstreams processor.
type Component struct {
	config      *Config
	deps        component.Dependencies
	storage     *semdragons.Storage
	events      *semdragons.EventPublisher
	engine      *semdragons.DefaultGuildFormationEngine
	logger      *slog.Logger
	boardConfig *semdragons.BoardConfig

	// Internal state
	running atomic.Bool
	mu      sync.RWMutex

	// Metrics
	guildsCreated   atomic.Uint64
	membersAdded    atomic.Uint64
	suggestionsEmit atomic.Uint64
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
		Name:        "guildformation",
		Type:        "processor",
		Description: "Guild management and auto-formation suggestions",
		Version:     "1.0.0",
	}
}

// InputPorts returns the ports this component accepts data on.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "agent-state",
			Direction:   component.DirectionInput,
			Required:    true,
			Description: "Agent state for cluster detection",
			Config: &component.KVWatchPort{
				Bucket: "", // Set dynamically from config
			},
		},
	}
}

// OutputPorts returns the ports this component produces data on.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "guild-suggested",
			Direction:   component.DirectionOutput,
			Required:    false,
			Description: "Guild formation suggestions",
			Config: &component.NATSPort{
				Subject: semdragons.PredicateGuildSuggested,
			},
		},
		{
			Name:        "guild-joined",
			Direction:   component.DirectionOutput,
			Required:    false,
			Description: "Guild membership events",
			Config: &component.NATSPort{
				Subject: semdragons.PredicateGuildAutoJoined,
			},
		},
		{
			Name:        "guild-state",
			Direction:   component.DirectionOutput,
			Required:    true,
			Description: "Guild state updates in KV",
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
			"min_founder_level": {
				Type:        "int",
				Description: "Minimum level to found guild (default 11)",
				Default:     11,
				Category:    "founding",
			},
			"founding_xp_cost": {
				Type:        "int",
				Description: "XP cost to found guild (default 500)",
				Default:     500,
				Category:    "founding",
			},
			"default_max_members": {
				Type:        "int",
				Description: "Default max members per guild (default 20)",
				Default:     20,
				Category:    "membership",
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
		status.LastError = "errors encountered during guild operations"
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

	operations := c.guildsCreated.Load() + c.membersAdded.Load()
	uptime := time.Since(c.startTime).Seconds()
	if uptime > 0 {
		metrics.MessagesPerSecond = float64(operations) / uptime
	}

	if operations > 0 {
		metrics.ErrorRate = float64(c.errorsCount.Load()) / float64(operations)
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
		return errs.Wrap(err, "GuildFormation", "Start", "create storage")
	}
	c.storage = storage

	// Create event publisher
	c.events = semdragons.NewEventPublisher(c.deps.NATSClient)

	// Create formation engine
	formationConfig := c.config.ToFormationConfig()
	c.engine = semdragons.NewGuildFormationEngine(c.storage, c.events, formationConfig)
	c.engine.WithLogger(c.logger)

	c.startTime = time.Now()
	c.running.Store(true)
	c.lastActivity.Store(time.Now())

	c.logger.Info("guildformation component started",
		"org", c.config.Org,
		"platform", c.config.Platform,
		"board", c.config.Board)

	return nil
}

// Stop gracefully shuts down the component.
// The timeout parameter is part of the LifecycleComponent interface but is not
// used as this component has no background goroutines requiring coordination.
func (c *Component) Stop(_ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running.Load() {
		return nil
	}

	c.running.Store(false)
	c.logger.Info("guildformation component stopped")

	return nil
}

// =============================================================================
// GUILD OPERATIONS (delegated to engine)
// =============================================================================

// FoundGuild creates a new guild.
func (c *Component) FoundGuild(ctx context.Context, founderID semdragons.AgentID, name, culture string) (*semdragons.Guild, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())

	guild, err := c.engine.FoundGuild(ctx, founderID, name, culture)
	if err != nil {
		c.errorsCount.Add(1)
		return nil, err
	}

	c.guildsCreated.Add(1)
	return guild, nil
}

// InviteToGuild sends an invitation to an agent.
func (c *Component) InviteToGuild(ctx context.Context, inviterID semdragons.AgentID, guildID semdragons.GuildID, inviteeID semdragons.AgentID) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())

	if err := c.engine.InviteToGuild(ctx, inviterID, guildID, inviteeID); err != nil {
		c.errorsCount.Add(1)
		return err
	}

	c.membersAdded.Add(1)
	return nil
}

// LeaveGuild removes an agent from a guild.
func (c *Component) LeaveGuild(ctx context.Context, agentID semdragons.AgentID, guildID semdragons.GuildID) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())

	if err := c.engine.LeaveGuild(ctx, agentID, guildID); err != nil {
		c.errorsCount.Add(1)
		return err
	}

	return nil
}

// PromoteMember promotes a guild member.
func (c *Component) PromoteMember(ctx context.Context, promoterID semdragons.AgentID, guildID semdragons.GuildID, memberID semdragons.AgentID, newRank semdragons.GuildRank) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())

	if err := c.engine.PromoteMember(ctx, promoterID, guildID, memberID, newRank); err != nil {
		c.errorsCount.Add(1)
		return err
	}

	return nil
}

// DetectSkillClusters suggests potential guild formations.
func (c *Component) DetectSkillClusters(ctx context.Context) ([]semdragons.GuildSuggestion, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())

	// Load all agents
	agents, err := c.storage.ListAllAgents(ctx)
	if err != nil {
		c.errorsCount.Add(1)
		return nil, err
	}

	suggestions := c.engine.DetectSkillClusters(ctx, agents)
	return suggestions, nil
}

// EvaluateGuildDiversity calculates guild skill coverage.
func (c *Component) EvaluateGuildDiversity(ctx context.Context, guildID semdragons.GuildID) (*semdragons.GuildDiversityReport, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())

	report, err := c.engine.EvaluateGuildDiversity(ctx, guildID)
	if err != nil {
		c.errorsCount.Add(1)
		return nil, err
	}

	return report, nil
}

// =============================================================================
// ACCESSORS
// =============================================================================

// Storage returns the underlying storage for external access.
func (c *Component) Storage() *semdragons.Storage {
	return c.storage
}

// Engine returns the underlying formation engine.
func (c *Component) Engine() *semdragons.DefaultGuildFormationEngine {
	return c.engine
}

// Stats returns guild formation statistics.
func (c *Component) Stats() GuildStats {
	return GuildStats{
		GuildsCreated:   c.guildsCreated.Load(),
		MembersAdded:    c.membersAdded.Load(),
		SuggestionsEmit: c.suggestionsEmit.Load(),
		Errors:          c.errorsCount.Load(),
	}
}

// GuildStats holds guild formation statistics.
type GuildStats struct {
	GuildsCreated   uint64 `json:"guilds_created"`
	MembersAdded    uint64 `json:"members_added"`
	SuggestionsEmit uint64 `json:"suggestions_emit"`
	Errors          int64  `json:"errors"`
}
