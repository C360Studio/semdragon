package questtools

import (
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/executor"
)

// Config holds all configuration for the questtools processor.
type Config struct {
	Org              string `json:"org"`
	Platform         string `json:"platform"`
	Board            string `json:"board"`
	StreamName       string `json:"stream_name"`
	QuestLoopsBucket string `json:"quest_loops_bucket"`
	Timeout          string `json:"timeout"`
	EnableBuiltins   bool   `json:"enable_builtins"`
	SandboxDir       string `json:"sandbox_dir"`
	// GraphQLURL is the graph-gateway GraphQL endpoint for the graph_search tool.
	// Used as fallback when the global GraphSourceRegistry is not configured.
	// When empty and no global registry, graph_search is not registered.
	GraphQLURL string `json:"graphql_url,omitempty"`
	// Search configures the web_search tool. When nil/empty provider, web_search
	// is not registered. Supports "brave" (more providers can be added).
	Search *executor.SearchConfig `json:"search,omitempty"`
	// SandboxURL is the HTTP base URL for the sandbox container.
	// When set, file/exec tools proxy through the sandbox instead of operating
	// on the local filesystem. Example: "http://sandbox:8090"
	SandboxURL string `json:"sandbox_url,omitempty"`
	// ConsumerNameSuffix disambiguates multiple instances consuming the same stream.
	ConsumerNameSuffix   string `json:"consumer_name_suffix,omitempty"`
	DeleteConsumerOnStop bool   `json:"delete_consumer_on_stop,omitempty"`
	// HTTPTextMaxChars limits characters returned after HTML-to-text conversion.
	// 0 means use the package default (20000).
	HTTPTextMaxChars int `json:"http_text_max_chars,omitempty"`
	// HTTPPersistToGraph enables persisting fetched HTML pages to the knowledge graph.
	// Requires a valid NATS connection. Defaults to true.
	HTTPPersistToGraph bool `json:"http_persist_to_graph"`
	// ExploreMaxIterations limits how many tool calls the explore sub-agent can make.
	// Default: 8.
	ExploreMaxIterations int `json:"explore_max_iterations,omitempty"`
	// ExploreTimeout is the maximum duration to wait for an explore sub-agent to complete.
	// Format: Go duration string (e.g. "120s", "2m"). Default: "120s".
	ExploreTimeout string `json:"explore_timeout,omitempty"`
	// ExploreCapability is the model registry capability key for explore sub-agents.
	// Default: "explore".
	ExploreCapability string `json:"explore_capability,omitempty"`
}

// DefaultConfig returns a Config with safe production defaults.
func DefaultConfig() Config {
	return Config{
		Org:                  "default",
		Platform:             "local",
		Board:                "main",
		StreamName:           "AGENT",
		QuestLoopsBucket:     "QUEST_LOOPS",
		Timeout:              "60s",
		EnableBuiltins:       true,
		HTTPTextMaxChars:     20000,
		HTTPPersistToGraph:   true,
		ExploreMaxIterations: 8,
		ExploreTimeout:       "120s",
		ExploreCapability:    "explore",
	}
}

// ExploreTimeoutDuration parses ExploreTimeout as a Go duration.
// Returns 120s on parse failure or zero/negative value.
func (c *Config) ExploreTimeoutDuration() time.Duration {
	d, err := time.ParseDuration(c.ExploreTimeout)
	if err != nil || d <= 0 {
		return 120 * time.Second
	}
	return d
}

// ToBoardConfig converts this Config into a BoardConfig for graph operations.
func (c *Config) ToBoardConfig() *domain.BoardConfig {
	return &domain.BoardConfig{
		Org:      c.Org,
		Platform: c.Platform,
		Board:    c.Board,
	}
}
