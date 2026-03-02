package questbridge

import (
	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/processor/promptmanager"
)

// Config holds all configuration for the QuestBridge component.
type Config struct {
	// Board identity
	Org      string `json:"org"`
	Platform string `json:"platform"`
	Board    string `json:"board"`

	// Stream and KV settings
	StreamName       string `json:"stream_name"`
	QuestLoopsBucket string `json:"quest_loops_bucket"`

	// Execution settings
	SandboxDir     string `json:"sandbox_dir"`
	EnableBuiltins bool   `json:"enable_builtins"`
	MaxIterations  int    `json:"max_iterations"`
	DefaultRole    string `json:"default_role"`

	// ConsumerNameSuffix allows unique consumer names in tests to avoid
	// durable consumer conflicts when multiple test instances run concurrently.
	ConsumerNameSuffix string `json:"consumer_name_suffix,omitempty"`

	// DeleteConsumerOnStop removes the durable consumer from the server on Stop.
	// Useful in tests to avoid stale consumer state between runs.
	DeleteConsumerOnStop bool `json:"delete_consumer_on_stop,omitempty"`

	// DomainCatalog enables domain-aware prompt assembly when set.
	// Not serialized to JSON — injected at construction time.
	DomainCatalog *promptmanager.DomainCatalog `json:"-"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Org:              "default",
		Platform:         "local",
		Board:            "main",
		StreamName:       "AGENT",
		QuestLoopsBucket: "QUEST_LOOPS",
		SandboxDir:       "",
		EnableBuiltins:   true,
		MaxIterations:    20,
		DefaultRole:      "general",
	}
}

// ToBoardConfig converts processor config to the domain BoardConfig.
func (c *Config) ToBoardConfig() *semdragons.BoardConfig {
	return &semdragons.BoardConfig{
		Org:      c.Org,
		Platform: c.Platform,
		Board:    c.Board,
	}
}
