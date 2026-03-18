package redteam

import (
	"errors"
	"time"

	"github.com/c360studio/semdragons/domain"
)

// Config holds the red-team review processor configuration.
type Config struct {
	// BoardConfig contains org, platform, board for entity IDs and bucket naming.
	Org      string `json:"org" schema:"type:string,description:Organization namespace"`
	Platform string `json:"platform" schema:"type:string,description:Platform/environment name"`
	Board    string `json:"board" schema:"type:string,description:Quest board name"`

	// Domain selects which DomainCatalog to use for review framing.
	Domain string `json:"domain,omitempty"`

	// MinDifficulty is the minimum quest difficulty that triggers red-team review.
	// Quests below this threshold skip red-team and go straight to boss battle.
	MinDifficulty domain.QuestDifficulty `json:"min_difficulty" schema:"type:int,description:Minimum quest difficulty for red-team review (0=trivial through 5=legendary)"`

	// ClaimTimeoutSec is how long (seconds) to wait for an agent to claim the
	// red-team quest before skipping and proceeding to boss battle.
	ClaimTimeoutSec int `json:"claim_timeout_seconds" schema:"type:int,description:Max seconds to wait for red-team claim before skip"`

	// ExecutionTimeoutSec is the total time (seconds) allowed for red-team quest
	// execution from posting to completion, including claim time.
	ExecutionTimeoutSec int `json:"execution_timeout_seconds" schema:"type:int,description:Total seconds for red-team quest lifecycle"`

	// PreferCrossGuild routes red-team quests to guilds different from the
	// implementing agent's guild. When false, any guild can red-team.
	PreferCrossGuild bool `json:"prefer_cross_guild" schema:"type:bool,description:Prefer guilds other than the implementer's guild"`

	// PartyTimeoutMultiplier scales the execution timeout for party red-team
	// quests which need multiple LLM calls (decompose + sub-quests + review + synthesis).
	PartyTimeoutMultiplier int `json:"party_timeout_multiplier,omitempty" schema:"type:int,description:Timeout multiplier for party red-team quests (default 3)"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Org:              "default",
		Platform:         "local",
		Board:            "main",
		MinDifficulty:      domain.DifficultyModerate,
		ClaimTimeoutSec:    120,
		ExecutionTimeoutSec: 300,
		PreferCrossGuild:       true,
		PartyTimeoutMultiplier: 3,
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

// ClaimTimeout returns the claim timeout as a time.Duration.
func (c *Config) ClaimTimeout() time.Duration {
	return time.Duration(c.ClaimTimeoutSec) * time.Second
}

// ExecutionTimeout returns the execution timeout as a time.Duration.
func (c *Config) ExecutionTimeout() time.Duration {
	return time.Duration(c.ExecutionTimeoutSec) * time.Second
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
	if c.ClaimTimeoutSec <= 0 {
		return errors.New("claim_timeout_seconds must be positive")
	}
	if c.ExecutionTimeoutSec <= 0 {
		return errors.New("execution_timeout_seconds must be positive")
	}
	if c.ExecutionTimeoutSec <= c.ClaimTimeoutSec {
		return errors.New("execution_timeout_seconds must be greater than claim_timeout_seconds")
	}
	return nil
}
