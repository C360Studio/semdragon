package dm_worldstate

// =============================================================================
// CONFIGURATION
// =============================================================================

// ComponentName is the registered name for this component.
const ComponentName = "dm_worldstate"

// Config holds all configuration for the DM WorldState component.
type Config struct {
	// Board identity
	Org      string `json:"org"`
	Platform string `json:"platform"`
	Board    string `json:"board"`

	// World state settings
	MaxEntitiesPerQuery int `json:"max_entities_per_query"` // Limit on entity queries
	RefreshIntervalSec  int `json:"refresh_interval_sec"`   // Auto-refresh interval
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Org:                 "default",
		Platform:            "local",
		Board:               "main",
		MaxEntitiesPerQuery: 1000,
		RefreshIntervalSec:  60,
	}
}
