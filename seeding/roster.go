package seeding

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/c360studio/semdragons"
)

// =============================================================================
// ROSTER SEEDER - Instant agent creation at target levels
// =============================================================================

// RosterSeeder creates agents from roster templates.
type RosterSeeder struct {
	storage    *semdragons.Storage
	config     *RosterConfig
	logger     *slog.Logger
	onProgress func(ProgressEvent)
}

// NewRosterSeeder creates a new roster seeder.
func NewRosterSeeder(storage *semdragons.Storage, config *RosterConfig) *RosterSeeder {
	return &RosterSeeder{
		storage: storage,
		config:  config,
		logger:  slog.Default(),
	}
}

// Seed creates agents and guilds from the roster template.
func (r *RosterSeeder) Seed(ctx context.Context, dryRun, idempotent bool) (*Result, error) {
	result := &Result{
		Mode:    ModeTieredRoster,
		Success: true,
		Agents:  make([]AgentSummary, 0),
	}

	// Create guilds first (agents may reference them)
	if err := r.seedGuilds(ctx, dryRun, result); err != nil {
		result.Success = false
		result.Errors = append(result.Errors, fmt.Sprintf("guild seeding failed: %v", err))
		return result, err
	}

	// Create agents
	if err := r.seedAgents(ctx, dryRun, idempotent, result); err != nil {
		result.Success = false
		result.Errors = append(result.Errors, fmt.Sprintf("agent seeding failed: %v", err))
		return result, err
	}

	return result, nil
}

// seedGuilds creates guilds from the roster template.
func (r *RosterSeeder) seedGuilds(ctx context.Context, dryRun bool, result *Result) error {
	for i, spec := range r.config.Guilds {
		if r.onProgress != nil {
			r.onProgress(ProgressEvent{
				Phase:   "guilds",
				Current: i + 1,
				Total:   len(r.config.Guilds),
				Percent: float64(i+1) / float64(len(r.config.Guilds)) * 100,
				Message: fmt.Sprintf("Creating guild: %s", spec.Name),
			})
		}

		if dryRun {
			r.logger.Info("dry run: would create guild",
				"id", spec.ID,
				"name", spec.Name,
			)
			result.GuildsCreated++
			continue
		}

		now := time.Now()
		guild := &semdragons.Guild{
			ID:          semdragons.GuildID(spec.ID),
			Name:        spec.Name,
			Description: spec.Description,
			Status:      semdragons.GuildActive,
			Founded:     now,
			Culture:     spec.Culture,
			Members:     []semdragons.GuildMember{},
			MinLevel:    spec.MinLevel,
			MaxMembers:  20,
			Reputation:  0.5,
			CreatedAt:   now,
		}

		if err := r.storage.PutGuild(ctx, spec.ID, guild); err != nil {
			return fmt.Errorf("failed to create guild %s: %w", spec.ID, err)
		}

		r.logger.Info("created guild",
			"id", spec.ID,
			"name", spec.Name,
		)
		result.GuildsCreated++
	}

	return nil
}

// seedAgents creates agents from the roster template.
func (r *RosterSeeder) seedAgents(ctx context.Context, dryRun, idempotent bool, result *Result) error {
	totalAgents := 0
	for _, spec := range r.config.Agents {
		totalAgents += spec.Count
	}

	agentNum := 0
	for _, spec := range r.config.Agents {
		for i := 1; i <= spec.Count; i++ {
			agentNum++

			// Generate name from pattern
			name := r.expandNamePattern(spec.NamePattern, i)

			if r.onProgress != nil {
				r.onProgress(ProgressEvent{
					Phase:     "agents",
					Current:   agentNum,
					Total:     totalAgents,
					Percent:   float64(agentNum) / float64(totalAgents) * 100,
					Message:   fmt.Sprintf("Creating agent: %s", name),
					AgentName: name,
				})
			}

			// Check for existing agent if idempotent
			if idempotent && !dryRun {
				existing, _ := r.findAgentByName(ctx, name)
				if existing != nil {
					r.logger.Debug("skipping existing agent",
						"name", name,
						"id", existing.ID,
					)
					result.AgentsSkipped++
					result.Agents = append(result.Agents, AgentSummary{
						ID:     existing.ID,
						Name:   existing.Name,
						Level:  existing.Level,
						Tier:   existing.Tier,
						Skills: existing.GetSkillTags(),
						IsNPC:  existing.IsNPC,
					})
					continue
				}
			}

			if dryRun {
				r.logger.Info("dry run: would create agent",
					"name", name,
					"level", spec.Level,
					"skills", spec.Skills,
					"is_npc", spec.IsNPC,
				)
				result.AgentsCreated++
				if spec.IsNPC {
					result.NPCsSpawned++
				}
				continue
			}

			// Create agent
			agent, err := r.createAgent(ctx, name, spec)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("failed to create %s: %v", name, err))
				continue
			}

			result.AgentsCreated++
			if spec.IsNPC {
				result.NPCsSpawned++
			}

			result.Agents = append(result.Agents, AgentSummary{
				ID:     agent.ID,
				Name:   agent.Name,
				Level:  agent.Level,
				Tier:   agent.Tier,
				Skills: agent.GetSkillTags(),
				IsNPC:  agent.IsNPC,
			})

			r.logger.Info("created agent",
				"id", agent.ID,
				"name", name,
				"level", spec.Level,
				"tier", agent.Tier,
				"is_npc", agent.IsNPC,
			)
		}
	}

	return nil
}

// expandNamePattern replaces {n} with the agent number.
func (r *RosterSeeder) expandNamePattern(pattern string, n int) string {
	return strings.ReplaceAll(pattern, "{n}", fmt.Sprintf("%d", n))
}

// findAgentByName searches for an existing agent by name.
func (r *RosterSeeder) findAgentByName(ctx context.Context, name string) (*semdragons.Agent, error) {
	// List all agents and find by name
	// This is O(n) but acceptable for seeding operations
	agents, err := r.storage.ListAllAgents(ctx)
	if err != nil {
		return nil, err
	}

	for _, agent := range agents {
		if agent.Name == name {
			return agent, nil
		}
	}

	return nil, nil
}

// createAgent creates a new agent from the spec.
func (r *RosterSeeder) createAgent(ctx context.Context, name string, spec AgentSpec) (*semdragons.Agent, error) {
	instance := semdragons.GenerateInstance()
	boardConfig := r.storage.Config()
	agentID := semdragons.AgentID(boardConfig.AgentEntityID(instance))

	agent := &semdragons.Agent{
		ID:     agentID,
		Name:   name,
		Config: spec.Config,
		IsNPC:  spec.IsNPC,
		Stats:  semdragons.AgentStats{},
	}

	// Initialize at target level
	initializeAgentAtLevel(agent, spec.Level)

	// Initialize skills
	initializeSkillProficiencies(agent, spec.Skills)

	// Add to guild if specified
	if spec.GuildID != "" {
		agent.Guilds = []semdragons.GuildID{semdragons.GuildID(spec.GuildID)}

		// Also add to guild's member list
		if err := r.addAgentToGuild(ctx, agent, spec.GuildID); err != nil {
			r.logger.Warn("failed to add agent to guild",
				"agent", name,
				"guild", spec.GuildID,
				"error", err,
			)
		}
	}

	// Store agent
	if err := r.storage.PutAgent(ctx, instance, agent); err != nil {
		return nil, fmt.Errorf("failed to store agent: %w", err)
	}

	return agent, nil
}

// addAgentToGuild adds an agent to a guild's member list.
func (r *RosterSeeder) addAgentToGuild(ctx context.Context, agent *semdragons.Agent, guildID string) error {
	return r.storage.UpdateGuild(ctx, guildID, func(guild *semdragons.Guild) error {
		// Check if already a member
		for _, member := range guild.Members {
			if member.AgentID == agent.ID {
				return nil
			}
		}

		guild.Members = append(guild.Members, semdragons.GuildMember{
			AgentID:      agent.ID,
			Rank:         semdragons.GuildRankInitiate,
			JoinedAt:     time.Now(),
			Contribution: 0,
		})

		return nil
	})
}

// =============================================================================
// PRESET ROSTERS
// =============================================================================

// DevTeamRoster returns a preset roster for a development team.
func DevTeamRoster(config semdragons.AgentConfig) *RosterConfig {
	return &RosterConfig{
		Name:        "dev-team",
		Description: "Standard development team with juniors, seniors, and a mentor",
		Agents: []AgentSpec{
			{
				NamePattern: "junior-dev-{n}",
				Count:       3,
				Level:       3,
				Skills:      []semdragons.SkillTag{semdragons.SkillCodeGen},
				Config:      config,
			},
			{
				NamePattern: "mid-dev-{n}",
				Count:       2,
				Level:       7,
				Skills:      []semdragons.SkillTag{semdragons.SkillCodeGen, semdragons.SkillCodeReview},
				Config:      config,
			},
			{
				NamePattern: "senior-dev-{n}",
				Count:       1,
				Level:       12,
				Skills:      []semdragons.SkillTag{semdragons.SkillCodeGen, semdragons.SkillCodeReview, semdragons.SkillPlanning},
				Config:      config,
			},
			{
				NamePattern: "mentor-{n}",
				Count:       1,
				Level:       8,
				Skills:      []semdragons.SkillTag{semdragons.SkillTraining, semdragons.SkillCodeReview},
				Config:      config,
			},
		},
	}
}

// BootstrapMentorRoster returns a roster for bootstrap NPC mentors.
func BootstrapMentorRoster(count int, config semdragons.AgentConfig) *RosterConfig {
	return &RosterConfig{
		Name:        "bootstrap-mentors",
		Description: "NPC mentors for training arena bootstrap",
		Agents: []AgentSpec{
			{
				NamePattern: "trainer-npc-{n}",
				Count:       count,
				Level:       8, // Journeyman+
				Skills:      []semdragons.SkillTag{semdragons.SkillTraining},
				Config:      config,
				IsNPC:       true,
			},
		},
	}
}
