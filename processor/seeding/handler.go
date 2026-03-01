package seeding

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/pkg/errs"
)

// =============================================================================
// SEEDING HANDLERS
// =============================================================================

// Result holds the outcome of a seeding operation.
type Result struct {
	SessionID     string        `json:"session_id"`
	Mode          Mode          `json:"mode"`
	Success       bool          `json:"success"`
	AgentsCreated int           `json:"agents_created"`
	AgentsSkipped int           `json:"agents_skipped"` // Idempotent skips
	GuildsCreated int           `json:"guilds_created"`
	Errors        []string      `json:"errors,omitempty"`
	Duration      time.Duration `json:"duration"`
}

// Seed executes the seeding operation based on configuration.
func (c *Component) Seed(ctx context.Context) (*Result, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	start := time.Now()
	sessionID := domain.GenerateInstance()

	result := &Result{
		SessionID: sessionID,
		Mode:      c.config.Mode,
		Success:   true,
	}

	c.seedingSessions.Add(1)
	c.lastActivity.Store(start)

	// Publish seeding started event
	if err := SubjectSeedingStarted.Publish(ctx, c.deps.NATSClient, SeedingStartedPayload{
		SessionID: sessionID,
		Mode:      c.config.Mode,
		DryRun:    c.config.DryRun,
		Timestamp: start,
	}); err != nil {
		c.errorsCount.Add(1)
		// Don't fail for event failure
	}

	c.logger.Info("starting seeding",
		"session_id", sessionID,
		"mode", c.config.Mode,
		"dry_run", c.config.DryRun,
		"idempotent", c.config.Idempotent,
	)

	// Execute based on mode
	var err error
	switch c.config.Mode {
	case ModeTieredRoster:
		err = c.seedRoster(ctx, sessionID, result)
	case ModeTrainingArena:
		err = c.seedArena(ctx, sessionID, result)
	default:
		err = errors.New("invalid seeding mode")
	}

	if err != nil {
		result.Success = false
		result.Errors = append(result.Errors, err.Error())
		c.errorsCount.Add(1)
	}

	result.Duration = time.Since(start)

	// Publish seeding completed event
	if pubErr := SubjectSeedingCompleted.Publish(ctx, c.deps.NATSClient, SeedingCompletedPayload{
		SessionID:     sessionID,
		Mode:          c.config.Mode,
		Success:       result.Success,
		AgentsCreated: result.AgentsCreated,
		GuildsCreated: result.GuildsCreated,
		Duration:      result.Duration,
		Errors:        result.Errors,
		Timestamp:     time.Now(),
	}); pubErr != nil {
		c.errorsCount.Add(1)
	}

	c.logger.Info("seeding completed",
		"session_id", sessionID,
		"success", result.Success,
		"agents_created", result.AgentsCreated,
		"guilds_created", result.GuildsCreated,
		"duration", result.Duration,
	)

	return result, err
}

// =============================================================================
// TIERED ROSTER SEEDING
// =============================================================================

// seedRoster creates agents and guilds from the roster template.
func (c *Component) seedRoster(ctx context.Context, sessionID string, result *Result) error {
	roster := c.config.Roster
	if roster == nil {
		return errors.New("roster config not set")
	}

	// Create guilds first (agents may reference them)
	for _, spec := range roster.Guilds {
		if err := c.seedGuild(ctx, sessionID, spec, result); err != nil {
			return errs.Wrap(err, "seeding", "seedRoster", "seed guild")
		}
	}

	// Create agents
	for _, spec := range roster.Agents {
		count := spec.Count
		if count == 0 {
			count = 1
		}

		for i := 0; i < count; i++ {
			name := spec.Name
			if spec.NamePattern != "" {
				name = strings.ReplaceAll(spec.NamePattern, "{n}", fmt.Sprintf("%d", i+1))
			} else if count > 1 {
				name = fmt.Sprintf("%s-%d", spec.Name, i+1)
			}

			if err := c.seedAgent(ctx, sessionID, name, spec, result); err != nil {
				return errs.Wrap(err, "seeding", "seedRoster", "seed agent")
			}
		}
	}

	return nil
}

// seedGuild creates a single guild.
func (c *Component) seedGuild(ctx context.Context, sessionID string, spec GuildSpec, result *Result) error {
	// Check for cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if c.config.DryRun {
		c.logger.Info("dry run: would create guild",
			"id", spec.ID,
			"name", spec.Name,
		)
		result.GuildsCreated++
		return nil
	}

	now := time.Now()
	instance := domain.GenerateInstance()
	guildID := domain.GuildID(c.boardConfig.GuildEntityID(instance))

	// Publish guild seeded event
	if err := SubjectGuildSeeded.Publish(ctx, c.deps.NATSClient, GuildSeededPayload{
		GuildID:         guildID,
		GuildName:       spec.Name,
		Description:     spec.Description,
		Specializations: spec.Specializations,
		SessionID:       sessionID,
		Timestamp:       now,
	}); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "seeding", "seedGuild", "publish event")
	}

	c.guildsSeeded.Add(1)
	result.GuildsCreated++

	c.logger.Info("seeded guild",
		"guild_id", guildID,
		"name", spec.Name,
	)

	return nil
}

// seedAgent creates a single agent.
func (c *Component) seedAgent(ctx context.Context, sessionID, name string, spec AgentSpec, result *Result) error {
	// Check for cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if c.config.DryRun {
		c.logger.Info("dry run: would create agent",
			"name", name,
			"level", spec.Level,
		)
		result.AgentsCreated++
		return nil
	}

	now := time.Now()
	instance := domain.GenerateInstance()
	agentID := domain.AgentID(c.boardConfig.AgentEntityID(instance))

	// Calculate tier from level
	tier := domain.TierFromLevel(spec.Level)

	// Parse guild ID if specified
	var guildID domain.GuildID
	if spec.GuildID != "" {
		guildID = domain.GuildID(spec.GuildID)
	}

	// Publish agent seeded event
	if err := SubjectAgentSeeded.Publish(ctx, c.deps.NATSClient, AgentSeededPayload{
		AgentID:   agentID,
		AgentName: name,
		Level:     spec.Level,
		Tier:      tier,
		Skills:    spec.Skills,
		GuildID:   guildID,
		IsNPC:     spec.IsNPC,
		SessionID: sessionID,
		Timestamp: now,
	}); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "seeding", "seedAgent", "publish event")
	}

	c.agentsSeeded.Add(1)
	result.AgentsCreated++

	c.logger.Info("seeded agent",
		"agent_id", agentID,
		"name", name,
		"level", spec.Level,
		"tier", tier,
	)

	return nil
}

// =============================================================================
// TRAINING ARENA SEEDING
// =============================================================================

// seedArena runs progressive training sessions.
// Note: This is a simplified implementation. Full training arena would involve
// actual LLM execution, quest posting, and XP calculation.
func (c *Component) seedArena(ctx context.Context, sessionID string, result *Result) error {
	arena := c.config.Arena
	if arena == nil {
		return errors.New("arena config not set")
	}

	// For now, training arena creates agents at Level 1
	// A full implementation would run training quests to level them up

	dist := arena.TargetDistribution

	// Create apprentices (Level 1-5)
	for i := 0; i < dist.Level1To5; i++ {
		spec := AgentSpec{
			Name:  fmt.Sprintf("apprentice-%d", i+1),
			Level: 1,
			IsNPC: false,
		}
		if err := c.seedAgent(ctx, sessionID, spec.Name, spec, result); err != nil {
			return err
		}
	}

	// Create journeymen (Level 6-10)
	for i := 0; i < dist.Level6To10; i++ {
		spec := AgentSpec{
			Name:  fmt.Sprintf("journeyman-%d", i+1),
			Level: 6,
			IsNPC: false,
		}
		if err := c.seedAgent(ctx, sessionID, spec.Name, spec, result); err != nil {
			return err
		}
	}

	// Create experts (Level 11-15)
	for i := 0; i < dist.Level11To15; i++ {
		spec := AgentSpec{
			Name:  fmt.Sprintf("expert-%d", i+1),
			Level: 11,
			IsNPC: false,
		}
		if err := c.seedAgent(ctx, sessionID, spec.Name, spec, result); err != nil {
			return err
		}
	}

	// Create masters (Level 16-18)
	for i := 0; i < dist.Level16To18; i++ {
		spec := AgentSpec{
			Name:  fmt.Sprintf("master-%d", i+1),
			Level: 16,
			IsNPC: false,
		}
		if err := c.seedAgent(ctx, sessionID, spec.Name, spec, result); err != nil {
			return err
		}
	}

	// Create grandmasters (Level 19-20)
	for i := 0; i < dist.Level19To20; i++ {
		spec := AgentSpec{
			Name:  fmt.Sprintf("grandmaster-%d", i+1),
			Level: 19,
			IsNPC: false,
		}
		if err := c.seedAgent(ctx, sessionID, spec.Name, spec, result); err != nil {
			return err
		}
	}

	return nil
}

// =============================================================================
// UTILITY METHODS
// =============================================================================

// GetStats returns seeding statistics.
func (c *Component) GetStats() SeedingStats {
	return SeedingStats{
		Sessions:     c.seedingSessions.Load(),
		AgentsSeeded: c.agentsSeeded.Load(),
		GuildsSeeded: c.guildsSeeded.Load(),
		Errors:       c.errorsCount.Load(),
		Uptime:       time.Since(c.startTime),
	}
}

// SeedingStats holds seeding statistics.
type SeedingStats struct {
	Sessions     uint64        `json:"sessions"`
	AgentsSeeded uint64        `json:"agents_seeded"`
	GuildsSeeded uint64        `json:"guilds_seeded"`
	Errors       int64         `json:"errors"`
	Uptime       time.Duration `json:"uptime"`
}
