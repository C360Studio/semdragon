package agentstore

import (
	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// CONFIGURATION
// =============================================================================

// Config holds all configuration for the AgentStore component.
type Config struct {
	// Board identity
	Org      string `json:"org"`
	Platform string `json:"platform"`
	Board    string `json:"board"`

	// Store settings
	EnableGuildDiscounts bool `json:"enable_guild_discounts"`
	DefaultRentalUses    int  `json:"default_rental_uses"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Org:                  "default",
		Platform:             "local",
		Board:                "main",
		EnableGuildDiscounts: true,
		DefaultRentalUses:    10,
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
