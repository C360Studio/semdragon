package questtools

import (
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
	// When empty, graph_search is not registered. Example: "http://localhost:8082/graphql"
	GraphQLURL string `json:"graphql_url,omitempty"`
	// Search configures the web_search tool. When nil/empty provider, web_search
	// is not registered. Supports "brave" (more providers can be added).
	Search *executor.SearchConfig `json:"search,omitempty"`
	// ConsumerNameSuffix disambiguates multiple instances consuming the same stream.
	ConsumerNameSuffix string `json:"consumer_name_suffix,omitempty"`
	DeleteConsumerOnStop bool   `json:"delete_consumer_on_stop,omitempty"`
}

// DefaultConfig returns a Config with safe production defaults.
func DefaultConfig() Config {
	return Config{
		Org:              "default",
		Platform:         "local",
		Board:            "main",
		StreamName:       "AGENT",
		QuestLoopsBucket: "QUEST_LOOPS",
		Timeout:          "60s",
		EnableBuiltins:   true,
	}
}

// ToBoardConfig converts this Config into a BoardConfig for graph operations.
func (c *Config) ToBoardConfig() *domain.BoardConfig {
	return &domain.BoardConfig{
		Org:      c.Org,
		Platform: c.Platform,
		Board:    c.Board,
	}
}
