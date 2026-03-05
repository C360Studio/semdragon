package boidengine

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

	// Boid rule weights
	SeparationWeight float64 `json:"separation_weight" schema:"type:float,description:Avoid quest overlap weight"`
	AlignmentWeight  float64 `json:"alignment_weight" schema:"type:float,description:Align with peers weight"`
	CohesionWeight   float64 `json:"cohesion_weight" schema:"type:float,description:Move toward skill clusters weight"`
	HungerWeight     float64 `json:"hunger_weight" schema:"type:float,description:Idle time urgency weight"`
	AffinityWeight   float64 `json:"affinity_weight" schema:"type:float,description:Skill/guild match weight"`
	CautionWeight    float64 `json:"caution_weight" schema:"type:float,description:Avoid over-leveled quests weight"`

	// Timing
	UpdateIntervalMs int `json:"update_interval_ms" schema:"type:int,description:How often to recompute suggestions"`
	NeighborRadius   int `json:"neighbor_radius" schema:"type:int,description:How many nearby agents to consider"`

	// Suggestions
	MaxSuggestionsPerAgent int `json:"max_suggestions_per_agent" schema:"type:int,description:Max ranked suggestions per agent (default 3)"`

	// Observability
	// BoidSuggestionsBucket is the NATS KV bucket where suggestions are persisted per agent.
	// Empty string disables KV persistence. Suggestions auto-expire after 5 minutes.
	BoidSuggestionsBucket string `json:"boid_suggestions_bucket" schema:"type:string,description:KV bucket for persisting boid suggestions (empty disables)"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() Config {
	rules := DefaultBoidRules()
	return Config{
		Org:                    "default",
		Platform:               "local",
		Board:                  "main",
		SeparationWeight:       rules.SeparationWeight,
		AlignmentWeight:        rules.AlignmentWeight,
		CohesionWeight:         rules.CohesionWeight,
		HungerWeight:           rules.HungerWeight,
		AffinityWeight:         rules.AffinityWeight,
		CautionWeight:          rules.CautionWeight,
		UpdateIntervalMs:       1000,
		NeighborRadius:         rules.NeighborRadius,
		MaxSuggestionsPerAgent: 3,
		BoidSuggestionsBucket:  "BOID_SUGGESTIONS",
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

// ToBoidRules converts component config to local BoidRules.
func (c *Config) ToBoidRules() BoidRules {
	return BoidRules{
		SeparationWeight: c.SeparationWeight,
		AlignmentWeight:  c.AlignmentWeight,
		CohesionWeight:   c.CohesionWeight,
		HungerWeight:     c.HungerWeight,
		AffinityWeight:   c.AffinityWeight,
		CautionWeight:    c.CautionWeight,
		NeighborRadius:   c.NeighborRadius,
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
	if c.UpdateIntervalMs < 1 {
		return errors.New("update_interval_ms must be at least 1")
	}
	if c.NeighborRadius < 1 {
		return errors.New("neighbor_radius must be at least 1")
	}
	// All weights should be non-negative
	if c.SeparationWeight < 0 {
		return errors.New("separation_weight must be non-negative")
	}
	if c.AlignmentWeight < 0 {
		return errors.New("alignment_weight must be non-negative")
	}
	if c.CohesionWeight < 0 {
		return errors.New("cohesion_weight must be non-negative")
	}
	if c.HungerWeight < 0 {
		return errors.New("hunger_weight must be non-negative")
	}
	if c.AffinityWeight < 0 {
		return errors.New("affinity_weight must be non-negative")
	}
	if c.CautionWeight < 0 {
		return errors.New("caution_weight must be non-negative")
	}
	return nil
}
