package questbridge

import (
	"github.com/c360studio/semdragons/domain"
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

	// SandboxURL is the HTTP base URL for the sandbox container.
	// When set, questbridge creates per-quest workspaces before dispatch and
	// snapshots workspace files to the filestore on completion.
	SandboxURL string `json:"sandbox_url,omitempty"`

	// EscalationTimeoutMins is how long a quest can stay escalated (waiting
	// for DM clarification) before the agent is released and the quest reposted.
	// Prevents agents from being stuck indefinitely if the DM never responds.
	// 0 disables the timeout (agents wait forever). Default: 30.
	EscalationTimeoutMins int `json:"escalation_timeout_mins,omitempty"`

	// DMMode controls how the DM handles agent clarification requests.
	// "full_auto" = auto-answer via LLM; "" or other = wait for human DM.
	DMMode domain.DMMode `json:"dm_mode,omitempty"`

	// MaxClarificationRounds limits how many times an agent can ask for
	// clarification on a single quest before it is force-failed.
	// Prevents infinite clarification loops. Default: 3. 0 = unlimited.
	MaxClarificationRounds int `json:"max_clarification_rounds,omitempty"`

	// ConsumerNameSuffix allows unique consumer names in tests to avoid
	// durable consumer conflicts when multiple test instances run concurrently.
	ConsumerNameSuffix string `json:"consumer_name_suffix,omitempty"`

	// DeleteConsumerOnStop removes the durable consumer from the server on Stop.
	// Useful in tests to avoid stale consumer state between runs.
	DeleteConsumerOnStop bool `json:"delete_consumer_on_stop,omitempty"`

	// EntityContextBudget is the maximum token budget for entity knowledge injection.
	// Entity knowledge (agent identity, quest details, party/guild context) is appended
	// to the system prompt. 0 disables entity context injection. Default: 2000.
	EntityContextBudget int `json:"entity_context_budget,omitempty"`

	// SemsourceURL is the HTTP base URL of the semsource service.
	// When set, questbridge self-initializes a ManifestClient to inject
	// graph knowledge into agent prompts.
	SemsourceURL string `json:"semsource_url,omitempty"`

	// GraphQLURL is the graph-gateway GraphQL endpoint for the graph manifest client.
	// When set, questbridge injects a summary of graph contents into entity knowledge
	// so agents know what's queryable via graph_search.
	GraphQLURL string `json:"graphql_url,omitempty"`

	// DependencyContextBudget is the maximum token budget per predecessor when
	// building structured dependency context. Only applies when EnableStructuredDeps
	// is true. 0 falls back to the default of 800 tokens.
	DependencyContextBudget int `json:"dependency_context_budget,omitempty"`

	// EnableStructuredDeps activates the three-tier dependency context cascade
	// (structured → summary → raw) in place of the legacy raw-output injection.
	// When false (default), loadDependencyOutputs is used for backward compat.
	EnableStructuredDeps bool `json:"enable_structured_deps,omitempty"`

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
		SandboxDir:       "",
		EnableBuiltins:          true,
		MaxIterations:           20,
		DefaultRole:             "general",
		EscalationTimeoutMins:   30,
		MaxClarificationRounds:  3,
		EntityContextBudget:     2000,
		DependencyContextBudget: 800,
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
