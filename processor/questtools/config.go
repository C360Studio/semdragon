package questtools

import (
	semdragons "github.com/c360studio/semdragons"
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
	// ConsumerNameSuffix disambiguates multiple instances consuming the same stream.
	ConsumerNameSuffix   string `json:"consumer_name_suffix,omitempty"`
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
func (c *Config) ToBoardConfig() *semdragons.BoardConfig {
	return &semdragons.BoardConfig{
		Org:      c.Org,
		Platform: c.Platform,
		Board:    c.Board,
	}
}
