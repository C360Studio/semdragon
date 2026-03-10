package questboard

import (
	"errors"

	"github.com/c360studio/semdragons/domain"
)

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

	// AutoPartyAboveDifficulty sets the difficulty threshold above which quests
	// automatically get PartyRequired=true. Nil means disabled (no auto-party).
	AutoPartyAboveDifficulty *domain.QuestDifficulty `json:"auto_party_above_difficulty,omitempty" schema:"type:int,description:Auto-party difficulty threshold (nil=disabled)"`

	// Triage configures DM triage for failed quests at the terminal boundary.
	Triage TriageConfig `json:"triage" schema:"type:object,description:DM triage configuration for failed quests"`
}

// TriageConfig controls when and how DM triage is applied to quests
// that have exhausted all retry attempts.
type TriageConfig struct {
	// Enabled activates the triage gate. When false, terminal failures
	// go directly to QuestFailed as before.
	Enabled bool `json:"enabled" schema:"type:bool,description:Enable DM triage for terminal failures"`

	// MinDifficultyForTriage skips triage for trivial quests. Quests below
	// this difficulty go straight to terminal failed.
	MinDifficultyForTriage domain.QuestDifficulty `json:"min_difficulty_for_triage" schema:"type:int,description:Minimum difficulty to trigger triage"`

	// TriageTimeoutMins is how long a quest can sit in pending_triage
	// before auto-applying the TPK recovery path.
	TriageTimeoutMins int `json:"triage_timeout_mins" schema:"type:int,description:Timeout before auto-TPK (minutes)"`

	// DMMode controls triage behavior:
	//   full_auto  — LLM auto-triages immediately
	//   assisted   — LLM proposes, human approves via approval queue
	//   supervised — Human triages; LLM provides suggested analysis
	//   manual     — Human triages via API, no LLM suggestion
	DMMode domain.DMMode `json:"dm_mode" schema:"type:string,description:DM mode for triage (full_auto/assisted/supervised/manual)"`

	// SessionID is the DM session ID for approval requests (assisted/supervised modes).
	SessionID string `json:"session_id" schema:"type:string,description:DM session ID for approval routing"`

	// Capability is the model registry capability used to resolve the triage LLM endpoint.
	// Defaults to "dm-chat" if empty.
	Capability string `json:"capability" schema:"type:string,description:Model registry capability for triage LLM"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Org:                "default",
		Platform:           "local",
		Board:              "main",
		DefaultMaxAttempts: 3,
		EnableEvaluation:   true,
		Triage: TriageConfig{
			Enabled:                false, // opt-in
			MinDifficultyForTriage: domain.DifficultyModerate,
			TriageTimeoutMins:      30,
			DMMode:                 domain.DMFullAuto,
		},
	}
}

// ToBoardConfig converts component config to semdragons BoardConfig.
func (c *Config) ToBoardConfig() *domain.BoardConfig {
	return &domain.BoardConfig{
		Org:      c.Org,
		Platform: c.Platform,
		Board:    c.Board,
	}
}

// Validate checks the configuration for required fields and valid values.
func (c *Config) Validate() error {
	if c.Org == "" {
		return errors.New("org is required")
	}
	if c.Platform == "" {
		return errors.New("platform is required")
	}
	if c.Board == "" {
		return errors.New("board is required")
	}
	if c.DefaultMaxAttempts < 1 {
		return errors.New("default_max_attempts must be at least 1")
	}
	return nil
}
