package seeding

import (
	"errors"

	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// CONFIGURATION
// =============================================================================

// Mode determines the seeding strategy.
type Mode string

const (
	// ModeTrainingArena uses real LLM execution for gradual progression.
	ModeTrainingArena Mode = "training_arena"
	// ModeTieredRoster instantly creates agents at target levels.
	ModeTieredRoster Mode = "tiered_roster"
)

// Config holds all configuration for the Seeding component.
type Config struct {
	// Board identity
	Org      string `json:"org"`
	Platform string `json:"platform"`
	Board    string `json:"board"`

	// Seeding mode
	Mode       Mode `json:"mode"`
	DryRun     bool `json:"dry_run"`    // Log actions without executing
	Idempotent bool `json:"idempotent"` // Skip existing agents by name

	// Mode-specific configs
	Arena  *ArenaConfig  `json:"arena,omitempty"`
	Roster *RosterConfig `json:"roster,omitempty"`
}

// ArenaConfig configures the training arena seeding mode.
type ArenaConfig struct {
	// TargetDistribution defines desired final agent levels.
	TargetDistribution LevelDistribution `json:"target_distribution"`

	// MaxTrainingQuests caps total quests per agent.
	MaxTrainingQuests int `json:"max_training_quests"`

	// XPMultiplier accelerates XP gain for faster training.
	XPMultiplier float64 `json:"xp_multiplier"`

	// QuestDomain selects which training quest template to use.
	QuestDomain string `json:"quest_domain"`

	// UseMentoredTraining enables NPC mentors for bootstrap.
	UseMentoredTraining bool `json:"use_mentored_training"`
}

// RosterConfig configures the tiered roster seeding mode.
type RosterConfig struct {
	// Agents to create
	Agents []AgentSpec `json:"agents"`

	// Guilds to create
	Guilds []GuildSpec `json:"guilds"`
}

// LevelDistribution defines how many agents at each level.
type LevelDistribution struct {
	Level1To5   int `json:"level_1_5"`   // Apprentices
	Level6To10  int `json:"level_6_10"`  // Journeymen
	Level11To15 int `json:"level_11_15"` // Experts
	Level16To18 int `json:"level_16_18"` // Masters
	Level19To20 int `json:"level_19_20"` // Grandmasters
}

// Total returns the total number of agents in the distribution.
func (d LevelDistribution) Total() int {
	return d.Level1To5 + d.Level6To10 + d.Level11To15 + d.Level16To18 + d.Level19To20
}

// AgentSpec describes an agent to seed.
type AgentSpec struct {
	Name        string            `json:"name"`
	Level       int               `json:"level"`
	Skills      []domain.SkillTag `json:"skills"`
	GuildID     string            `json:"guild_id,omitempty"`
	IsNPC       bool              `json:"is_npc"`
	Count       int               `json:"count"`                  // How many of this type to create
	NamePattern string            `json:"name_pattern,omitempty"` // e.g., "analyst-{n}"
}

// GuildSpec describes a guild to seed.
type GuildSpec struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Description     string            `json:"description"`
	Specializations []domain.SkillTag `json:"specializations"`
	MinLevel        int               `json:"min_level"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Org:        "default",
		Platform:   "local",
		Board:      "main",
		Mode:       ModeTieredRoster,
		DryRun:     false,
		Idempotent: true,
	}
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	switch c.Mode {
	case ModeTrainingArena:
		if c.Arena == nil {
			return errors.New("arena config required for training_arena mode")
		}
		return c.Arena.Validate()
	case ModeTieredRoster:
		if c.Roster == nil {
			return errors.New("roster config required for tiered_roster mode")
		}
		return c.Roster.Validate()
	default:
		return errors.New("invalid seeding mode")
	}
}

// Validate checks the arena config.
func (c *ArenaConfig) Validate() error {
	if c.TargetDistribution.Total() == 0 {
		return errors.New("target distribution must have at least one agent")
	}
	if c.MaxTrainingQuests <= 0 {
		c.MaxTrainingQuests = 100
	}
	if c.XPMultiplier <= 0 {
		c.XPMultiplier = 1.0
	}
	return nil
}

// Validate checks the roster config.
func (c *RosterConfig) Validate() error {
	if len(c.Agents) == 0 && len(c.Guilds) == 0 {
		return errors.New("roster must have at least one agent or guild")
	}
	return nil
}

// ToBoardConfig converts processor config to domain BoardConfig.
func (c *Config) ToBoardConfig() *domain.BoardConfig {
	return &domain.BoardConfig{
		Org:      c.Org,
		Platform: c.Platform,
		Board:    c.Board,
	}
}
