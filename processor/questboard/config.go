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
