package partycoord

import (
	"time"

	semdragons "github.com/c360studio/semdragons"
)

// =============================================================================
// CONFIGURATION
// =============================================================================

// Config holds all configuration for the PartyCoord component.
type Config struct {
	// Board identity
	Org      string `json:"org"`
	Platform string `json:"platform"`
	Board    string `json:"board"`

	// Party settings
	DefaultMaxPartySize int           `json:"default_max_party_size"`
	FormationTimeout    time.Duration `json:"formation_timeout"`
	RollupTimeout       time.Duration `json:"rollup_timeout"`

	// Auto-party settings
	AutoFormParties      bool `json:"auto_form_parties"`
	MinMembersForParty   int  `json:"min_members_for_party"`
	RequireLeadApproval  bool `json:"require_lead_approval"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Org:                  "default",
		Platform:             "local",
		Board:                "main",
		DefaultMaxPartySize:  5,
		FormationTimeout:     10 * time.Minute,
		RollupTimeout:        5 * time.Minute,
		AutoFormParties:      true,
		MinMembersForParty:   2,
		RequireLeadApproval:  true,
	}
}

// ToBoardConfig converts processor config to domain BoardConfig.
func (c *Config) ToBoardConfig() *semdragons.BoardConfig {
	return &semdragons.BoardConfig{
		Org:      c.Org,
		Platform: c.Platform,
		Board:    c.Board,
	}
}
