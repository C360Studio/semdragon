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
			"semembed": {
				Provider:  "openai",
				URL:       "http://semembed:8081/v1",
				Model:     "BAAI/bge-small-en-v1.5",
				MaxTokens: 0,
			},
		},
		Capabilities: map[string]*model.CapabilityConfig{
			// Global fallback (unchanged, backwards compatible)
			"agent-work": {
				Description:   "Agent quest execution with tool calling",
				Preferred:     []string{"ollama-tools"},
				Fallback:      []string{"ollama"},
				RequiresTools: true,
			},

			// Tier defaults — all resolve to local Ollama in dev,
			// but the key structure exercises the resolution chain.
			"agent-work.apprentice": {
				Description:   "Apprentice tier: small/fast models",
				Preferred:     []string{"ollama"},
				Fallback:      []string{"ollama-tools"},
				RequiresTools: true,
			},
			"agent-work.journeyman": {
				Description:   "Journeyman tier: mid-tier models",
				Preferred:     []string{"ollama-tools"},
				Fallback:      []string{"ollama"},
				RequiresTools: true,
			},
			"agent-work.expert": {
				Description:   "Expert tier: full models",
				Preferred:     []string{"ollama-tools"},
				Fallback:      []string{"ollama"},
				RequiresTools: true,
			},
			"agent-work.master": {
				Description:   "Master tier: frontier models",
				Preferred:     []string{"ollama-tools"},
				RequiresTools: true,
			},
			"agent-work.grandmaster": {
				Description:   "Grandmaster tier: frontier+ models",
				Preferred:     []string{"ollama-tools"},
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
			"dm-chat": {
				Description: "DM conversational assistant for quest building",
				Preferred:   []string{"ollama-tools"},
				Fallback:    []string{"ollama"},
			},
			"agent-eval": {
				Description: "Agent performance assessment",
				Preferred:   []string{"ollama"},
			},
			"embedding": {
				Description: "Vector embeddings for graph semantic search",
				Preferred:   []string{"semembed"},
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
			"haiku": {
				Provider:      "anthropic",
				Model:         "claude-haiku-4-5-20251001",
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
			"gpt-mini": {
				Provider:      "openai",
				URL:           "https://api.openai.com/v1",
				Model:         "gpt-4o-mini",
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
			"gemini-flash-lite": {
				Provider:  "google",
				Model:     "gemini-2.5-flash-lite",
				MaxTokens: 1048576,
				APIKeyEnv: "GEMINI_API_KEY",
			},
			"gpt-nano": {
				Provider:  "openai",
				URL:       "https://api.openai.com/v1",
				Model:     "gpt-4.1-nano",
				MaxTokens: 1048576,
				APIKeyEnv: "OPENAI_API_KEY",
			},
			"semembed": {
				Provider:  "openai",
				URL:       "http://semembed:8081/v1",
				Model:     "BAAI/bge-small-en-v1.5",
				MaxTokens: 0,
			},
		},
		Capabilities: map[string]*model.CapabilityConfig{
			// Global fallback (unchanged, backwards compatible)
			"agent-work": {
				Description:   "Agent quest execution with tool calling",
				Preferred:     []string{"claude-4", "gpt-4o"},
				Fallback:      []string{"ollama"},
				RequiresTools: true,
			},

			// Tier defaults — tier sets the model budget ceiling
			"agent-work.apprentice": {
				Description:   "Apprentice tier: small/fast models",
				Preferred:     []string{"haiku", "gpt-mini"},
				Fallback:      []string{"ollama"},
				RequiresTools: true,
			},
			"agent-work.journeyman": {
				Description:   "Journeyman tier: mid-tier models",
				Preferred:     []string{"claude-4", "gpt-mini"},
				Fallback:      []string{"haiku"},
				RequiresTools: true,
			},
			"agent-work.expert": {
				Description:   "Expert tier: full models",
				Preferred:     []string{"claude-4", "gpt-4o"},
				Fallback:      []string{"haiku"},
				RequiresTools: true,
			},
			"agent-work.master": {
				Description:   "Master tier: frontier models",
				Preferred:     []string{"claude-4"},
				Fallback:      []string{"gpt-4o"},
				RequiresTools: true,
			},
			"agent-work.grandmaster": {
				Description:   "Grandmaster tier: frontier+ models",
				Preferred:     []string{"claude-4"},
				RequiresTools: true,
			},

			// Skill-specific overrides — skill picks model within tier ceiling
			"agent-work.expert.code_generation": {
				Description:   "Expert code generation: strong tool use",
				Preferred:     []string{"claude-4", "gpt-4o"},
				RequiresTools: true,
			},
			"agent-work.expert.summarization": {
				Description:   "Expert summarization: cheap for simple work",
				Preferred:     []string{"haiku", "gpt-mini"},
				RequiresTools: false,
			},
			"agent-work.master.summarization": {
				Description:   "Master summarization: frontier not needed",
				Preferred:     []string{"haiku"},
				RequiresTools: false,
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
			"dm-chat": {
				Description: "DM conversational assistant for quest building",
				Preferred:   []string{"claude-4"},
				Fallback:    []string{"gpt-4o", "ollama"},
			},
			"agent-eval": {
				Description: "Agent performance assessment",
				Preferred:   []string{"claude-4"},
				Fallback:    []string{"gpt-4o"},
			},
			"embedding": {
				Description: "Vector embeddings for graph semantic search",
				Preferred:   []string{"semembed"},
			},
			"community_summary": {
				Description: "LLM community summaries for graph clustering",
				Preferred:   []string{"gemini-flash-lite", "gpt-nano", "haiku"},
			},
			"query_classification": {
				Description: "NLQ classification for graph queries",
				Preferred:   []string{"gemini-flash-lite", "gpt-nano", "haiku"},
			},
		},
		Defaults: model.DefaultsConfig{
			Model: "claude-4",
		},
	}
}
