package guildformation

import (
	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// CONFIGURATION
// =============================================================================

// Config holds all configuration for the GuildFormation component.
type Config struct {
	// Board identity
	Org      string `json:"org"`
	Platform string `json:"platform"`
	Board    string `json:"board"`

	// Guild settings
	MinMembersForFormation int `json:"min_members_for_formation"`
	MaxGuildSize           int `json:"max_guild_size"`

	// Founding quorum settings
	EnableQuorumFormation bool `json:"enable_quorum_formation"`
	MinFoundingMembers    int  `json:"min_founding_members"`
	FormationTimeoutSec   int  `json:"formation_timeout_sec"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Org:                    "default",
		Platform:               "local",
		Board:                  "main",
		MinMembersForFormation: 3,
		MaxGuildSize:           20,
		EnableQuorumFormation:  false,
		MinFoundingMembers:     3,
		FormationTimeoutSec:    300,
	}
}

// ToBoardConfig converts processor config to domain BoardConfig.
func (c *Config) ToBoardConfig() *domain.BoardConfig {
	return &domain.BoardConfig{
		Org:      c.Org,
		Platform: c.Platform,
		Board:    c.Board,
	}
}
