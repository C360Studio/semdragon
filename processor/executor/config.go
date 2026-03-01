package executor

import (
	semdragons "github.com/c360studio/semdragons"
)

// =============================================================================
// CONFIGURATION
// =============================================================================

// Config holds all configuration for the Executor component.
type Config struct {
	// Board identity
	Org      string `json:"org"`
	Platform string `json:"platform"`
	Board    string `json:"board"`

	// Execution settings
	MaxTurns      int    `json:"max_turns"`       // Maximum tool-call loops per execution
	MaxTokens     int    `json:"max_tokens"`      // Token budget per execution
	SandboxDir    string `json:"sandbox_dir"`     // Base directory for file operations
	EnableBuiltins bool   `json:"enable_builtins"` // Register built-in tools
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Org:            "default",
		Platform:       "local",
		Board:          "main",
		MaxTurns:       20,
		MaxTokens:      50000,
		SandboxDir:     "",
		EnableBuiltins: true,
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
