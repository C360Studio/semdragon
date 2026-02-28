// Package seeding provides environment bootstrapping for semdragons.
// Supports two modes:
//   - Training Arena: Real LLM execution with progressive skill development
//   - Tiered Roster: Instant seeding at target levels from templates
package seeding

import (
	"github.com/c360studio/semdragons"
)

// =============================================================================
// CONFIGURATION TYPES
// =============================================================================

// Mode determines the seeding strategy.
type Mode string

const (
	// ModeTrainingArena uses real LLM execution for gradual progression.
	ModeTrainingArena Mode = "training_arena"
	// ModeTieredRoster instantly creates agents at target levels.
	ModeTieredRoster Mode = "tiered_roster"
)

// Config is the top-level seeding configuration.
type Config struct {
	Mode       Mode          `json:"mode"`
	DryRun     bool          `json:"dry_run"`    // Log actions without executing
	Idempotent bool          `json:"idempotent"` // Skip existing agents by name
	Arena      *ArenaConfig  `json:"arena,omitempty"`
	Roster     *RosterConfig `json:"roster,omitempty"`
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	switch c.Mode {
	case ModeTrainingArena:
		if c.Arena == nil {
			return ErrArenaConfigRequired
		}
		return c.Arena.Validate()
	case ModeTieredRoster:
		if c.Roster == nil {
			return ErrRosterConfigRequired
		}
		return c.Roster.Validate()
	default:
		return ErrInvalidMode
	}
}

// =============================================================================
// TRAINING ARENA CONFIG
// =============================================================================

// ArenaConfig configures the training arena seeding mode.
type ArenaConfig struct {
	// AgentConfigs are the LLM configurations for agents to create.
	AgentConfigs []semdragons.AgentConfig `json:"agent_configs"`

	// TargetDistribution defines desired final agent levels.
	TargetDistribution LevelDistribution `json:"target_distribution"`

	// MaxTrainingQuests caps total quests per agent to prevent runaway costs.
	MaxTrainingQuests int `json:"max_training_quests"`

	// XPMultiplier accelerates XP gain for faster training (default: 1.0).
	XPMultiplier float64 `json:"xp_multiplier"`

	// QuestDomain selects which training quest template to use ("code", "research", etc.).
	QuestDomain string `json:"quest_domain"`

	// QuestFile is an optional path to custom quest templates JSON.
	QuestFile string `json:"quest_file,omitempty"`

	// JudgeConfig is the LLM configuration for the arena judge.
	JudgeConfig semdragons.AgentConfig `json:"judge_config"`

	// BootstrapMentors is the number of NPC mentors to spawn if no real mentors available.
	BootstrapMentors int `json:"bootstrap_mentors"`

	// UseMentoredTraining enables mentored party formation for training.
	UseMentoredTraining bool `json:"use_mentored_training"`

	// TraineesPerMentor is the party size for mentored training (default: 3).
	TraineesPerMentor int `json:"trainees_per_mentor"`
}

// Validate checks that the arena configuration is valid.
func (c *ArenaConfig) Validate() error {
	if len(c.AgentConfigs) == 0 {
		return ErrNoAgentConfigs
	}
	if c.MaxTrainingQuests <= 0 {
		return ErrInvalidMaxQuests
	}
	if c.QuestDomain == "" && c.QuestFile == "" {
		return ErrNoQuestSource
	}
	return nil
}

// LevelDistribution defines target agent level requirements.
type LevelDistribution struct {
	MinLevel int `json:"min_level"` // All agents reach at least this level
	MaxLevel int `json:"max_level"` // Optional cap (0 = no cap)

	// Distribution by tier (optional, overrides min/max if set)
	ApprenticeCount  int `json:"apprentice_count,omitempty"`  // Levels 1-5
	JourneymanCount  int `json:"journeyman_count,omitempty"`  // Levels 6-10
	ExpertCount      int `json:"expert_count,omitempty"`      // Levels 11-15
	MasterCount      int `json:"master_count,omitempty"`      // Levels 16-18
	GrandmasterCount int `json:"grandmaster_count,omitempty"` // Levels 19-20
}

// =============================================================================
// TIERED ROSTER CONFIG
// =============================================================================

// RosterConfig configures the tiered roster seeding mode.
type RosterConfig struct {
	// Name identifies this roster template.
	Name string `json:"name"`

	// Description explains the roster's purpose.
	Description string `json:"description,omitempty"`

	// Agents defines the agents to create.
	Agents []AgentSpec `json:"agents"`

	// Guilds defines guilds to create (optional).
	Guilds []GuildSpec `json:"guilds,omitempty"`
}

// Validate checks that the roster configuration is valid.
func (c *RosterConfig) Validate() error {
	if c.Name == "" {
		return ErrNoRosterName
	}
	if len(c.Agents) == 0 {
		return ErrNoAgentSpecs
	}
	for _, spec := range c.Agents {
		if err := spec.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// AgentSpec defines a template for creating agents.
type AgentSpec struct {
	// NamePattern is the naming template (e.g., "analyst-{n}" creates analyst-1, analyst-2, ...).
	NamePattern string `json:"name_pattern"`

	// Count is how many agents to create from this spec.
	Count int `json:"count"`

	// Level is the starting level for these agents.
	Level int `json:"level"`

	// Skills are the initial skills (all at Novice proficiency).
	Skills []semdragons.SkillTag `json:"skills"`

	// Config is the LLM configuration for these agents.
	Config semdragons.AgentConfig `json:"config"`

	// IsNPC marks these agents as NPCs (for bootstrap mentors).
	IsNPC bool `json:"is_npc,omitempty"`

	// GuildID assigns these agents to a guild.
	GuildID string `json:"guild_id,omitempty"`
}

// Validate checks that the agent spec is valid.
func (s *AgentSpec) Validate() error {
	if s.NamePattern == "" {
		return ErrNoNamePattern
	}
	if s.Count <= 0 {
		return ErrInvalidCount
	}
	if s.Level < 1 || s.Level > 20 {
		return ErrInvalidLevel
	}
	return nil
}

// GuildSpec defines a template for creating guilds.
type GuildSpec struct {
	// ID is the guild identifier.
	ID string `json:"id"`

	// Name is the guild display name.
	Name string `json:"name"`

	// Description explains the guild's purpose.
	Description string `json:"description"`

	// Culture describes the guild's values and approach.
	Culture string `json:"culture,omitempty"`

	// MinLevel is the minimum agent level for joining.
	MinLevel int `json:"min_level"`
}

// =============================================================================
// ERRORS
// =============================================================================

// Error types for seeding configuration.
type configError string

func (e configError) Error() string { return string(e) }

// Seeding configuration errors.
const (
	// ErrInvalidMode indicates an unknown seeding mode was specified.
	ErrInvalidMode = configError("invalid seeding mode")
	// ErrArenaConfigRequired indicates arena config is needed for training_arena mode.
	ErrArenaConfigRequired = configError("arena config required for training_arena mode")
	// ErrRosterConfigRequired indicates roster config is needed for tiered_roster mode.
	ErrRosterConfigRequired = configError("roster config required for tiered_roster mode")
	// ErrNoAgentConfigs indicates no agent configurations were provided.
	ErrNoAgentConfigs = configError("at least one agent config required")
	// ErrInvalidMaxQuests indicates max_training_quests was not positive.
	ErrInvalidMaxQuests = configError("max_training_quests must be positive")
	// ErrNoQuestSource indicates neither quest_domain nor quest_file was provided.
	ErrNoQuestSource = configError("quest_domain or quest_file required")
	// ErrNoRosterName indicates the roster name was empty.
	ErrNoRosterName = configError("roster name required")
	// ErrNoAgentSpecs indicates no agent specs were provided in the roster.
	ErrNoAgentSpecs = configError("at least one agent spec required")
	// ErrNoNamePattern indicates the agent spec has no name pattern.
	ErrNoNamePattern = configError("name_pattern required")
	// ErrInvalidCount indicates the agent count was not positive.
	ErrInvalidCount = configError("count must be positive")
	// ErrInvalidLevel indicates the level was outside the valid 1-20 range.
	ErrInvalidLevel = configError("level must be between 1 and 20")
)
