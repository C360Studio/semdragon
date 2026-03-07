package questbridge

import (
	"github.com/c360studio/semdragons/processor/promptmanager"

	"github.com/c360studio/semdragons/domain"
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
	QuestDagsBucket  string `json:"quest_dags_bucket"`

	// Execution settings
	SandboxDir     string `json:"sandbox_dir"`
	EnableBuiltins bool   `json:"enable_builtins"`
	MaxIterations  int    `json:"max_iterations"`
	DefaultRole    string `json:"default_role"`

	// EscalationTimeoutMins is how long a quest can stay escalated (waiting
	// for DM clarification) before the agent is released and the quest reposted.
	// Prevents agents from being stuck indefinitely if the DM never responds.
	// 0 disables the timeout (agents wait forever). Default: 30.
	EscalationTimeoutMins int `json:"escalation_timeout_mins,omitempty"`

	// ConsumerNameSuffix allows unique consumer names in tests to avoid
	// durable consumer conflicts when multiple test instances run concurrently.
	ConsumerNameSuffix string `json:"consumer_name_suffix,omitempty"`

	// DeleteConsumerOnStop removes the durable consumer from the server on Stop.
	// Useful in tests to avoid stale consumer state between runs.
	DeleteConsumerOnStop bool `json:"delete_consumer_on_stop,omitempty"`

	// Domain selects which DomainCatalog to inject (e.g. "software", "dnd", "research").
	Domain string `json:"domain,omitempty"`

	// DomainCatalog enables domain-aware prompt assembly when set.
	// Not serialized to JSON — resolved from Domain at construction time.
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
		QuestDagsBucket:  "QUEST_DAGS",
		SandboxDir:       "",
		EnableBuiltins:   true,
		MaxIterations:         20,
		DefaultRole:           "general",
		EscalationTimeoutMins: 30,
	}
}

// ToBoardConfig converts processor config to the domain BoardConfig.
func (c *Config) ToBoardConfig() *domain.BoardConfig {
	return &domain.BoardConfig{
		Org:      c.Org,
		Platform: c.Platform,
		Board:    c.Board,
	}
}
