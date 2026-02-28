package guildformation

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

	// Guild formation settings
	MinFounderLevel     int     `json:"min_founder_level" schema:"type:int,description:Minimum level to found guild"`
	FoundingXPCost      int64   `json:"founding_xp_cost" schema:"type:int,description:XP cost to found guild"`
	DefaultMaxMembers   int     `json:"default_max_members" schema:"type:int,description:Default max members per guild"`
	MinClusterSize      int     `json:"min_cluster_size" schema:"type:int,description:Minimum agents for cluster suggestion"`
	MinClusterStrength  float64 `json:"min_cluster_strength" schema:"type:float,description:Minimum Jaccard similarity for clusters"`
	MinAgentLevel       int     `json:"min_agent_level" schema:"type:int,description:Minimum agent level for guild consideration"`
	RequireQualityScore float64 `json:"require_quality_score" schema:"type:float,description:Minimum avg quality score"`
	TotalSkillCount     int     `json:"total_skill_count" schema:"type:int,description:Total skills for diversity calculation"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() Config {
	defaults := semdragons.DefaultFormationConfig()
	return Config{
		Org:                 "default",
		Platform:            "local",
		Board:               "main",
		MinFounderLevel:     defaults.MinFounderLevel,
		FoundingXPCost:      defaults.FoundingXPCost,
		DefaultMaxMembers:   defaults.DefaultMaxMembers,
		MinClusterSize:      defaults.MinClusterSize,
		MinClusterStrength:  defaults.MinClusterStrength,
		MinAgentLevel:       defaults.MinAgentLevel,
		RequireQualityScore: defaults.RequireQualityScore,
		TotalSkillCount:     defaults.TotalSkillCount,
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

// ToFormationConfig converts component config to semdragons GuildFormationConfig.
func (c *Config) ToFormationConfig() semdragons.GuildFormationConfig {
	return semdragons.GuildFormationConfig{
		MinFounderLevel:     c.MinFounderLevel,
		FoundingXPCost:      c.FoundingXPCost,
		DefaultMaxMembers:   c.DefaultMaxMembers,
		MinClusterSize:      c.MinClusterSize,
		MinClusterStrength:  c.MinClusterStrength,
		MinAgentLevel:       c.MinAgentLevel,
		RequireQualityScore: c.RequireQualityScore,
		TotalSkillCount:     c.TotalSkillCount,
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
	if c.MinFounderLevel < 1 {
		return errors.New("min_founder_level must be at least 1")
	}
	if c.FoundingXPCost < 0 {
		return errors.New("founding_xp_cost cannot be negative")
	}
	if c.DefaultMaxMembers < 1 {
		return errors.New("default_max_members must be at least 1")
	}
	if c.MinClusterSize < 2 {
		return errors.New("min_cluster_size must be at least 2")
	}
	if c.MinClusterStrength < 0 || c.MinClusterStrength > 1 {
		return errors.New("min_cluster_strength must be between 0 and 1")
	}
	if c.MinAgentLevel < 1 {
		return errors.New("min_agent_level must be at least 1")
	}
	if c.RequireQualityScore < 0 || c.RequireQualityScore > 1 {
		return errors.New("require_quality_score must be between 0 and 1")
	}
	if c.TotalSkillCount < 1 {
		return errors.New("total_skill_count must be at least 1")
	}
	return nil
}
