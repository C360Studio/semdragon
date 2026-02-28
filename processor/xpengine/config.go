package xpengine

import (
	"errors"

	semdragons "github.com/c360studio/semdragons"
)

// Config holds the component configuration.
type Config struct {
	// BoardConfig contains org, platform, board for entity IDs and bucket naming.
	Org      string `json:"org" schema:"type:string,description:Organization namespace"`
	Platform string `json:"platform" schema:"type:string,description:Platform/environment name"`
	Board    string `json:"board" schema:"type:string,description:Quest board name"`

	// XP calculation multipliers
	QualityMultiplier  float64 `json:"quality_multiplier" schema:"type:float,description:Quality bonus multiplier"`
	SpeedMultiplier    float64 `json:"speed_multiplier" schema:"type:float,description:Speed bonus multiplier"`
	StreakMultiplier   float64 `json:"streak_multiplier" schema:"type:float,description:Streak bonus multiplier"`
	RetryPenaltyRate   float64 `json:"retry_penalty_rate" schema:"type:float,description:XP penalty per retry attempt"`
	FailurePenaltyRate float64 `json:"failure_penalty_rate" schema:"type:float,description:XP penalty for failures"`
	LevelDownThreshold int     `json:"level_down_threshold" schema:"type:int,description:Consecutive failures for demotion"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Org:                "default",
		Platform:           "local",
		Board:              "main",
		QualityMultiplier:  2.0,
		SpeedMultiplier:    0.5,
		StreakMultiplier:   0.1,
		RetryPenaltyRate:   0.25,
		FailurePenaltyRate: 0.5,
		LevelDownThreshold: 3,
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

// ToXPEngine creates the underlying XP engine with configured parameters.
func (c *Config) ToXPEngine() *semdragons.DefaultXPEngine {
	return &semdragons.DefaultXPEngine{
		QualityMultiplier:  c.QualityMultiplier,
		SpeedMultiplier:    c.SpeedMultiplier,
		StreakMultiplier:   c.StreakMultiplier,
		RetryPenaltyRate:   c.RetryPenaltyRate,
		FailurePenaltyRate: c.FailurePenaltyRate,
		LevelDownThreshold: c.LevelDownThreshold,
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
	if c.QualityMultiplier < 0 {
		return errors.New("quality_multiplier must be non-negative")
	}
	if c.SpeedMultiplier < 0 {
		return errors.New("speed_multiplier must be non-negative")
	}
	if c.LevelDownThreshold < 1 {
		return errors.New("level_down_threshold must be at least 1")
	}
	return nil
}
