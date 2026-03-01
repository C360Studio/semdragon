package dmpartyformation

// =============================================================================
// CONFIGURATION
// =============================================================================

// ComponentName is the registered name for this component.
const ComponentName = "dm_partyformation"

// Config holds all configuration for the DM Party Formation component.
type Config struct {
	// Board identity
	Org      string `json:"org"`
	Platform string `json:"platform"`
	Board    string `json:"board"`

	// Party formation settings
	DefaultStrategy   string `json:"default_strategy"`     // Default party strategy
	MaxPartySize      int    `json:"max_party_size"`       // Maximum party size
	MinMembersForLead int    `json:"min_members_for_lead"` // Minimum members for party lead
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Org:               "default",
		Platform:          "local",
		Board:             "main",
		DefaultStrategy:   "balanced",
		MaxPartySize:      5,
		MinMembersForLead: 1,
	}
}
