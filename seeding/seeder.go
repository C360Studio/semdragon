package seeding

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360studio/semdragons"
)

// =============================================================================
// SEEDER - Main interface for environment seeding
// =============================================================================

// Seeder orchestrates environment seeding based on configuration.
type Seeder struct {
	board   semdragons.QuestBoard
	storage *semdragons.Storage
	config  *Config
	logger  *slog.Logger

	// Sub-seeders
	arena  *ArenaSeeder
	roster *RosterSeeder
}

// Result holds the outcome of a seeding operation.
type Result struct {
	Mode            Mode    `json:"mode"`
	Success         bool           `json:"success"`
	AgentsCreated   int            `json:"agents_created"`
	AgentsSkipped   int            `json:"agents_skipped"` // Idempotent skips
	GuildsCreated   int            `json:"guilds_created"`
	NPCsSpawned     int            `json:"npcs_spawned"`
	QuestsCompleted int            `json:"quests_completed"` // Arena only
	Errors          []string       `json:"errors,omitempty"`
	Duration        time.Duration  `json:"duration"`
	Agents          []AgentSummary `json:"agents"`
}

// AgentSummary provides a brief summary of a seeded agent.
type AgentSummary struct {
	ID     semdragons.AgentID    `json:"id"`
	Name   string                `json:"name"`
	Level  int                   `json:"level"`
	Tier   semdragons.TrustTier  `json:"tier"`
	Skills []semdragons.SkillTag `json:"skills"`
	IsNPC  bool                  `json:"is_npc"`
}

// NewSeeder creates a new seeder with the given configuration.
func NewSeeder(board semdragons.QuestBoard, storage *semdragons.Storage, config *Config) (*Seeder, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	s := &Seeder{
		board:   board,
		storage: storage,
		config:  config,
		logger:  slog.Default(),
	}

	// Initialize appropriate sub-seeder
	switch config.Mode {
	case ModeTrainingArena:
		s.arena = NewArenaSeeder(board, storage, config.Arena)
	case ModeTieredRoster:
		s.roster = NewRosterSeeder(storage, config.Roster)
	}

	return s, nil
}

// WithLogger sets a custom logger.
func (s *Seeder) WithLogger(l *slog.Logger) *Seeder {
	s.logger = l
	if s.arena != nil {
		s.arena.logger = l
	}
	if s.roster != nil {
		s.roster.logger = l
	}
	return s
}

// Seed executes the seeding operation.
func (s *Seeder) Seed(ctx context.Context) (*Result, error) {
	start := time.Now()

	s.logger.Info("starting seeding",
		"mode", s.config.Mode,
		"dry_run", s.config.DryRun,
		"idempotent", s.config.Idempotent,
	)

	var result *Result
	var err error

	switch s.config.Mode {
	case ModeTrainingArena:
		result, err = s.arena.Seed(ctx, s.config.DryRun, s.config.Idempotent)
	case ModeTieredRoster:
		result, err = s.roster.Seed(ctx, s.config.DryRun, s.config.Idempotent)
	default:
		return nil, ErrInvalidMode
	}

	if result != nil {
		result.Duration = time.Since(start)
		result.Mode = s.config.Mode
	}

	if err != nil {
		s.logger.Error("seeding failed",
			"mode", s.config.Mode,
			"error", err,
			"duration", time.Since(start),
		)
		return result, err
	}

	s.logger.Info("seeding completed",
		"mode", s.config.Mode,
		"agents_created", result.AgentsCreated,
		"agents_skipped", result.AgentsSkipped,
		"guilds_created", result.GuildsCreated,
		"npcs_spawned", result.NPCsSpawned,
		"duration", result.Duration,
	)

	return result, nil
}

// SeedWithProgress executes seeding and reports progress via callback.
func (s *Seeder) SeedWithProgress(ctx context.Context, progress func(ProgressEvent)) (*Result, error) {
	// Wrap the sub-seeders with progress reporting
	switch s.config.Mode {
	case ModeTrainingArena:
		s.arena.onProgress = progress
	case ModeTieredRoster:
		s.roster.onProgress = progress
	}

	return s.Seed(ctx)
}

// ProgressEvent reports seeding progress.
type ProgressEvent struct {
	Phase      string  `json:"phase"` // "agents", "guilds", "training", "complete"
	Current    int     `json:"current"`
	Total      int     `json:"total"`
	Percent    float64 `json:"percent"`
	Message    string  `json:"message"`
	AgentName  string  `json:"agent_name,omitempty"`
	QuestTitle string  `json:"quest_title,omitempty"`
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

// calculateXPForLevel returns the total XP needed to reach a level from level 1.
func calculateXPForLevel(targetLevel int) int64 {
	xpEngine := semdragons.NewDefaultXPEngine()
	var totalXP int64
	for level := 1; level < targetLevel; level++ {
		totalXP += xpEngine.XPToNextLevel(level)
	}
	return totalXP
}

// initializeAgentAtLevel sets up an agent at a specific level with appropriate XP.
func initializeAgentAtLevel(agent *semdragons.Agent, level int) {
	xpEngine := semdragons.NewDefaultXPEngine()

	agent.Level = level
	agent.Tier = semdragons.TierFromLevel(level)
	agent.XPToLevel = xpEngine.XPToNextLevel(level)
	agent.XP = 0 // Start at 0 XP within the current level
	agent.Status = semdragons.AgentIdle
	agent.CreatedAt = time.Now()
	agent.UpdatedAt = time.Now()
}

// initializeSkillProficiencies sets up the skill proficiencies map.
func initializeSkillProficiencies(agent *semdragons.Agent, skills []semdragons.SkillTag) {
	if agent.SkillProficiencies == nil {
		agent.SkillProficiencies = make(map[semdragons.SkillTag]semdragons.SkillProficiency)
	}

	for _, skill := range skills {
		agent.SkillProficiencies[skill] = semdragons.SkillProficiency{
			Level:      semdragons.ProficiencyNovice,
			Progress:   0,
			TotalXP:    0,
			QuestsUsed: 0,
		}
	}
}
