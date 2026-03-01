package dm_session

// =============================================================================
// CONFIGURATION
// =============================================================================

// ComponentName is the registered name for this component.
const ComponentName = "dm_session"

// Config holds all configuration for the DM Session component.
type Config struct {
	// Board identity
	Org      string `json:"org"`
	Platform string `json:"platform"`
	Board    string `json:"board"`

	// Session settings
	DefaultMode       string `json:"default_mode"`        // Default DM mode
	MaxConcurrent     int    `json:"max_concurrent"`      // Max concurrent quests
	AutoEscalate      bool   `json:"auto_escalate"`       // Auto-escalate after max attempts
	SessionTimeoutMin int    `json:"session_timeout_min"` // Session timeout in minutes
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Org:               "default",
		Platform:          "local",
		Board:             "main",
		DefaultMode:       "manual",
		MaxConcurrent:     10,
		AutoEscalate:      true,
		SessionTimeoutMin: 60,
	}
}
