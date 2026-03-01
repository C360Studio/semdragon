package dm_approval

// =============================================================================
// CONFIGURATION
// =============================================================================

// ComponentName is the registered name for this component.
const ComponentName = "dm_approval"

// Config holds all configuration for the DM Approval component.
type Config struct {
	// Board identity
	Org      string `json:"org"`
	Platform string `json:"platform"`
	Board    string `json:"board"`

	// Approval settings
	ApprovalTimeoutMin int  `json:"approval_timeout_min"` // Approval timeout in minutes
	AutoApprove        bool `json:"auto_approve"`         // Auto-approve for testing
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Org:                "default",
		Platform:           "local",
		Board:              "main",
		ApprovalTimeoutMin: 30,
		AutoApprove:        false,
	}
}
