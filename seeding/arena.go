package seeding

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360studio/semdragons"
)

// =============================================================================
// ARENA SEEDER - Progressive training with real LLM execution
// =============================================================================
// The training arena:
// 1. Creates agents at Level 1
// 2. Spawns NPC mentors if needed for bootstrap
// 3. Forms training parties (mentor + trainees)
// 4. Runs training quests with real LLM execution
// 5. LLM judge evaluates results
// 6. XP awarded, skills improve
// 7. Repeat until target levels reached
// =============================================================================

// ArenaSeeder runs progressive training sessions.
type ArenaSeeder struct {
	board      semdragons.QuestBoard
	storage    *semdragons.Storage
	config     *ArenaConfig
	logger     *slog.Logger
	onProgress func(ProgressEvent)

	// Training state
	templates *QuestTemplates
	judge     *ArenaJudge
}

// NewArenaSeeder creates a new arena seeder.
func NewArenaSeeder(board semdragons.QuestBoard, storage *semdragons.Storage, config *ArenaConfig) *ArenaSeeder {
	return &ArenaSeeder{
		board:   board,
		storage: storage,
		config:  config,
		logger:  slog.Default(),
	}
}

// Seed runs the training arena to develop agents.
func (a *ArenaSeeder) Seed(ctx context.Context, dryRun, idempotent bool) (*Result, error) {
	result := &Result{
		Mode:    ModeTrainingArena,
		Success: true,
		Agents:  make([]AgentSummary, 0),
	}

	// Load quest templates
	if err := a.loadTemplates(); err != nil {
		result.Success = false
		result.Errors = append(result.Errors, fmt.Sprintf("failed to load templates: %v", err))
		return result, err
	}

	// Initialize judge
	a.judge = NewArenaJudge(a.config.JudgeConfig)

	// Create initial agents
	agents, err := a.createInitialAgents(ctx, dryRun, idempotent, result)
	if err != nil {
		result.Success = false
		result.Errors = append(result.Errors, fmt.Sprintf("failed to create agents: %v", err))
		return result, err
	}

	if dryRun {
		return result, nil
	}

	// Check if we need bootstrap mentors
	if a.config.UseMentoredTraining {
		if err := a.ensureMentorsAvailable(ctx, result); err != nil {
			a.logger.Warn("failed to ensure mentors", "error", err)
		}
	}

	// Run training rounds
	if err := a.runTrainingRounds(ctx, agents, result); err != nil {
		result.Success = false
		result.Errors = append(result.Errors, fmt.Sprintf("training failed: %v", err))
		return result, err
	}

	return result, nil
}

// loadTemplates loads quest templates from configuration.
func (a *ArenaSeeder) loadTemplates() error {
	var err error

	if a.config.QuestFile != "" {
		a.templates, err = LoadQuestTemplatesFromFile(a.config.QuestFile)
	} else {
		a.templates, err = LoadQuestTemplates(a.config.QuestDomain)
	}

	return err
}

// createInitialAgents creates agents at Level 1.
func (a *ArenaSeeder) createInitialAgents(ctx context.Context, dryRun, idempotent bool, result *Result) ([]*semdragons.Agent, error) {
	var agents []*semdragons.Agent

	for i, config := range a.config.AgentConfigs {
		name := fmt.Sprintf("trainee-%d", i+1)

		if a.onProgress != nil {
			a.onProgress(ProgressEvent{
				Phase:     "agents",
				Current:   i + 1,
				Total:     len(a.config.AgentConfigs),
				Percent:   float64(i+1) / float64(len(a.config.AgentConfigs)) * 100,
				Message:   fmt.Sprintf("Creating trainee: %s", name),
				AgentName: name,
			})
		}

		// Check for existing if idempotent
		if idempotent && !dryRun {
			existing, _ := a.findAgentByName(ctx, name)
			if existing != nil {
				a.logger.Debug("skipping existing agent", "name", name)
				result.AgentsSkipped++
				agents = append(agents, existing)
				result.Agents = append(result.Agents, AgentSummary{
					ID:     existing.ID,
					Name:   existing.Name,
					Level:  existing.Level,
					Tier:   existing.Tier,
					Skills: existing.GetSkillTags(),
				})
				continue
			}
		}

		if dryRun {
			a.logger.Info("dry run: would create trainee",
				"name", name,
				"config", config.Model,
			)
			result.AgentsCreated++
			continue
		}

		// Create agent at Level 1
		agent, err := a.createTrainee(ctx, name, config)
		if err != nil {
			return nil, fmt.Errorf("failed to create %s: %w", name, err)
		}

		agents = append(agents, agent)
		result.AgentsCreated++
		result.Agents = append(result.Agents, AgentSummary{
			ID:     agent.ID,
			Name:   agent.Name,
			Level:  agent.Level,
			Tier:   agent.Tier,
			Skills: agent.GetSkillTags(),
		})

		a.logger.Info("created trainee",
			"id", agent.ID,
			"name", name,
		)
	}

	return agents, nil
}

// createTrainee creates a new trainee agent at Level 1.
func (a *ArenaSeeder) createTrainee(ctx context.Context, name string, config semdragons.AgentConfig) (*semdragons.Agent, error) {
	instance := semdragons.GenerateInstance()
	boardConfig := a.storage.Config()
	agentID := semdragons.AgentID(boardConfig.AgentEntityID(instance))

	agent := &semdragons.Agent{
		ID:     agentID,
		Name:   name,
		Config: config,
		Stats:  semdragons.AgentStats{},
	}

	// Initialize at Level 1
	initializeAgentAtLevel(agent, 1)

	// Initialize empty skill proficiencies (will be populated as training progresses)
	agent.SkillProficiencies = make(map[semdragons.SkillTag]semdragons.SkillProficiency)

	// Store agent
	if err := a.storage.PutAgent(ctx, instance, agent); err != nil {
		return nil, fmt.Errorf("failed to store agent: %w", err)
	}

	return agent, nil
}

// findAgentByName searches for an existing agent by name.
func (a *ArenaSeeder) findAgentByName(ctx context.Context, name string) (*semdragons.Agent, error) {
	agents, err := a.storage.ListAllAgents(ctx)
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

// ensureMentorsAvailable checks for mentors and spawns NPCs if needed.
func (a *ArenaSeeder) ensureMentorsAvailable(ctx context.Context, result *Result) error {
	// Look for agents with training skill at Journeyman+ level
	mentors, err := a.findAvailableMentors(ctx)
	if err != nil {
		return err
	}

	if len(mentors) > 0 {
		a.logger.Info("found available mentors", "count", len(mentors))
		return nil
	}

	// No mentors available - spawn NPCs
	if a.config.BootstrapMentors <= 0 {
		a.logger.Warn("no mentors available and bootstrap disabled")
		return nil
	}

	a.logger.Info("spawning bootstrap mentor NPCs", "count", a.config.BootstrapMentors)

	// Use roster seeder to create NPC mentors
	rosterConfig := BootstrapMentorRoster(a.config.BootstrapMentors, a.config.JudgeConfig)
	roster := NewRosterSeeder(a.storage, rosterConfig)
	roster.logger = a.logger

	rosterResult, err := roster.Seed(ctx, false, true)
	if err != nil {
		return fmt.Errorf("failed to spawn NPC mentors: %w", err)
	}

	result.NPCsSpawned += rosterResult.NPCsSpawned

	return nil
}

// findAvailableMentors finds agents with training skill who can mentor.
func (a *ArenaSeeder) findAvailableMentors(ctx context.Context) ([]*semdragons.Agent, error) {
	agents, err := a.storage.ListAllAgents(ctx)
	if err != nil {
		return nil, err
	}

	var mentors []*semdragons.Agent
	for _, agent := range agents {
		// Check if has training skill and is Journeyman+
		if !agent.HasSkill(semdragons.SkillTraining) {
			continue
		}
		if semdragons.TierFromLevel(agent.Level) < semdragons.TierJourneyman {
			continue
		}
		if agent.Status != semdragons.AgentIdle {
			continue
		}

		mentors = append(mentors, agent)
	}

	return mentors, nil
}

// runTrainingRounds executes training quest rounds until targets are reached.
func (a *ArenaSeeder) runTrainingRounds(ctx context.Context, agents []*semdragons.Agent, result *Result) error {
	questsRun := 0
	maxQuests := a.config.MaxTrainingQuests * len(agents)

	for questsRun < maxQuests {
		// Check if all agents have reached target level
		if a.allAgentsAtTarget(agents) {
			a.logger.Info("all agents reached target level")
			break
		}

		// Select next quest template
		template := a.templates.SelectForLevel(a.avgAgentLevel(agents))
		if template == nil {
			a.logger.Warn("no suitable quest template found")
			break
		}

		if a.onProgress != nil {
			a.onProgress(ProgressEvent{
				Phase:      "training",
				Current:    questsRun + 1,
				Total:      maxQuests,
				Percent:    float64(questsRun+1) / float64(maxQuests) * 100,
				Message:    fmt.Sprintf("Running training quest: %s", template.Title),
				QuestTitle: template.Title,
			})
		}

		// Run training round for each agent
		for _, agent := range agents {
			if a.agentAtTarget(agent) {
				continue
			}

			if err := a.runTrainingQuest(ctx, agent, template, result); err != nil {
				a.logger.Warn("training quest failed",
					"agent", agent.Name,
					"quest", template.Title,
					"error", err,
				)
			}

			questsRun++
			result.QuestsCompleted++

			if questsRun >= maxQuests {
				break
			}
		}
	}

	// Update result with final agent states
	result.Agents = make([]AgentSummary, 0, len(agents))
	for _, agent := range agents {
		// Refresh agent state from storage
		instance := semdragons.ExtractInstance(string(agent.ID))
		refreshed, err := a.storage.GetAgent(ctx, instance)
		if err != nil {
			refreshed = agent
		}

		result.Agents = append(result.Agents, AgentSummary{
			ID:     refreshed.ID,
			Name:   refreshed.Name,
			Level:  refreshed.Level,
			Tier:   refreshed.Tier,
			Skills: refreshed.GetSkillTags(),
			IsNPC:  refreshed.IsNPC,
		})
	}

	return nil
}

// runTrainingQuest executes a single training quest for an agent.
func (a *ArenaSeeder) runTrainingQuest(ctx context.Context, agent *semdragons.Agent, template *QuestTemplate, result *Result) error {
	// Create quest from template
	quest := template.ToQuest()

	// Apply XP multiplier
	if a.config.XPMultiplier > 0 {
		quest.BaseXP = int64(float64(quest.BaseXP) * a.config.XPMultiplier)
	}

	// Post quest
	posted, err := a.board.PostQuest(ctx, quest)
	if err != nil {
		return fmt.Errorf("failed to post quest: %w", err)
	}

	// Claim quest
	if err := a.board.ClaimQuest(ctx, posted.ID, agent.ID); err != nil {
		return fmt.Errorf("failed to claim quest: %w", err)
	}

	// Start quest
	if err := a.board.StartQuest(ctx, posted.ID); err != nil {
		return fmt.Errorf("failed to start quest: %w", err)
	}

	// Execute quest (this is where the real LLM would be called)
	// For now, we simulate with the judge
	output, err := a.executeQuest(ctx, agent, template)
	if err != nil {
		return fmt.Errorf("failed to execute quest: %w", err)
	}

	// Submit result - this triggers boss battle and progression
	_, err = a.board.SubmitResult(ctx, posted.ID, output)
	if err != nil {
		return fmt.Errorf("failed to submit result: %w", err)
	}

	return nil
}

// executeQuest runs the actual quest execution.
// In a real implementation, this would call the LLM.
func (a *ArenaSeeder) executeQuest(ctx context.Context, agent *semdragons.Agent, template *QuestTemplate) (any, error) {
	// For now, return a simulated result
	// In production, this would:
	// 1. Build prompt from template.Input
	// 2. Call agent's LLM (agent.Config)
	// 3. Return LLM response

	return map[string]any{
		"response":   "Simulated training response",
		"agent":      agent.Name,
		"quest":      template.Title,
		"difficulty": template.Difficulty,
		"timestamp":  time.Now(),
	}, nil
}

// allAgentsAtTarget checks if all agents have reached the target level.
func (a *ArenaSeeder) allAgentsAtTarget(agents []*semdragons.Agent) bool {
	for _, agent := range agents {
		if !a.agentAtTarget(agent) {
			return false
		}
	}
	return true
}

// agentAtTarget checks if an agent has reached the target level.
func (a *ArenaSeeder) agentAtTarget(agent *semdragons.Agent) bool {
	return agent.Level >= a.config.TargetDistribution.MinLevel
}

// avgAgentLevel returns the average level of agents.
func (a *ArenaSeeder) avgAgentLevel(agents []*semdragons.Agent) int {
	if len(agents) == 0 {
		return 1
	}

	total := 0
	for _, agent := range agents {
		total += agent.Level
	}
	return total / len(agents)
}
