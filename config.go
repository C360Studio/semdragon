package semdragons

import (
	"encoding/json"
	"os"

	"github.com/c360studio/semstreams/model"
)

// LoadModelRegistry loads a model registry from a JSON configuration file.
// If the file doesn't exist or can't be read, returns a default registry
// configured for local development with Ollama.
func LoadModelRegistry(path string) (*model.Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		// Fall back to defaults for local development
		return DefaultModelRegistry(), nil
	}

	var reg model.Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, err
	}

	if err := reg.Validate(); err != nil {
		return nil, err
	}

	return &reg, nil
}

// DefaultModelRegistry returns a default registry configured for local
// development using Ollama. This enables development without requiring
// API keys for cloud providers.
func DefaultModelRegistry() *model.Registry {
	return &model.Registry{
		Endpoints: map[string]*model.EndpointConfig{
			"ollama": {
				Provider:      "ollama",
				URL:           "http://localhost:11434/v1",
				Model:         "llama3.2",
				MaxTokens:     8192,
				SupportsTools: false,
			},
			"ollama-tools": {
				Provider:      "ollama",
				URL:           "http://localhost:11434/v1",
				Model:         "llama3.1",
				MaxTokens:     131072,
				SupportsTools: true,
				ToolFormat:    "openai",
			},
		},
		Capabilities: map[string]*model.CapabilityConfig{
			"agent-work": {
				Description:   "Agent quest execution with tool calling",
				Preferred:     []string{"ollama-tools"},
				Fallback:      []string{"ollama"},
				RequiresTools: true,
			},
			"boss-battle": {
				Description: "Quest output evaluation by LLM judge",
				Preferred:   []string{"ollama"},
			},
			"quest-design": {
				Description: "DM quest parameter decisions",
				Preferred:   []string{"ollama"},
			},
			"agent-eval": {
				Description: "Agent performance assessment",
				Preferred:   []string{"ollama"},
			},
		},
		Defaults: model.DefaultsConfig{
			Model: "ollama",
		},
	}
}

// ProductionModelRegistry returns a registry configured for production
// use with Claude and GPT-4o. Requires ANTHROPIC_API_KEY and/or
// OPENAI_API_KEY environment variables.
func ProductionModelRegistry() *model.Registry {
	return &model.Registry{
		Endpoints: map[string]*model.EndpointConfig{
			"claude-4": {
				Provider:      "anthropic",
				Model:         "claude-sonnet-4-5-20250514",
				MaxTokens:     200000,
				SupportsTools: true,
				ToolFormat:    "anthropic",
				APIKeyEnv:     "ANTHROPIC_API_KEY",
			},
			"gpt-4o": {
				Provider:      "openai",
				URL:           "https://api.openai.com/v1",
				Model:         "gpt-4o",
				MaxTokens:     128000,
				SupportsTools: true,
				ToolFormat:    "openai",
				APIKeyEnv:     "OPENAI_API_KEY",
			},
			"ollama": {
				Provider:      "ollama",
				URL:           "http://localhost:11434/v1",
				Model:         "llama3.2",
				MaxTokens:     8192,
				SupportsTools: false,
			},
		},
		Capabilities: map[string]*model.CapabilityConfig{
			"agent-work": {
				Description:   "Agent quest execution with tool calling",
				Preferred:     []string{"claude-4", "gpt-4o"},
				Fallback:      []string{"ollama"},
				RequiresTools: true,
			},
			"boss-battle": {
				Description: "Quest output evaluation by LLM judge",
				Preferred:   []string{"claude-4"},
				Fallback:    []string{"gpt-4o", "ollama"},
			},
			"quest-design": {
				Description: "DM quest parameter decisions",
				Preferred:   []string{"claude-4"},
				Fallback:    []string{"gpt-4o"},
			},
			"agent-eval": {
				Description: "Agent performance assessment",
				Preferred:   []string{"claude-4"},
				Fallback:    []string{"gpt-4o"},
			},
		},
		Defaults: model.DefaultsConfig{
			Model: "claude-4",
		},
	}
}
