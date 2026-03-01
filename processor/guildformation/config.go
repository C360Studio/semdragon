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
	MinMembersForFormation int  `json:"min_members_for_formation"`
	MaxGuildSize           int  `json:"max_guild_size"`
	EnableAutoFormation    bool `json:"enable_auto_formation"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Org:                    "default",
		Platform:               "local",
		Board:                  "main",
		MinMembersForFormation: 3,
		MaxGuildSize:           20,
		EnableAutoFormation:    true,
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
