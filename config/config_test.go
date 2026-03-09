package config_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"net/url"
)

// Minimal structs to unmarshal the model_registry section of E2E config files.
// We only decode what we need to validate; other config fields are ignored.
type testConfig struct {
	ModelRegistry struct {
		Endpoints    map[string]testEndpoint    `json:"endpoints"`
		Capabilities map[string]testCapability  `json:"capabilities"`
		Defaults     struct {
			Model string `json:"model"`
		} `json:"defaults"`
	} `json:"model_registry"`
}

type testEndpoint struct {
	Provider  string `json:"provider"`
	URL       string `json:"url"`
	Model     string `json:"model"`
	APIKeyEnv string `json:"api_key_env"`
	MaxTokens int    `json:"max_tokens"`
}

type testCapability struct {
	Preferred     []string `json:"preferred"`
	RequiresTools bool     `json:"requires_tools"`
}

// knownBadModels lists specific model ID strings that must not appear in any
// E2E config. These are stale versioned IDs that have been superseded.
var knownBadModels = []string{
	"claude-sonnet-4-5-20250514",
	"claude-sonnet-4-6-20250527",
	"claude-opus-4-6-20250527",
}

// knownBadDateSuffixes are date strings that must not appear as suffixes in any
// model ID. A model ID like "claude-foo-20250514" would be caught by this list.
var knownBadDateSuffixes = []string{
	"20250514",
	"20250527",
}

// knownAPIKeyPatterns are the env var names we expect for cloud providers.
// Any non-empty api_key_env must be one of these.
var knownAPIKeyPatterns = []string{
	"ANTHROPIC_API_KEY",
	"OPENAI_API_KEY",
	"GEMINI_API_KEY",
}

// loadE2EConfigs finds all semdragons-e2e*.json files in the config directory
// (which is also the package directory for tests) and parses them.
func loadE2EConfigs(t *testing.T) map[string]testConfig {
	t.Helper()

	matches, err := filepath.Glob("semdragons-e2e*.json")
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no semdragons-e2e*.json files found; tests must run in the config/ directory")
	}

	configs := make(map[string]testConfig, len(matches))
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var cfg testConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		configs[filepath.Base(path)] = cfg
	}
	return configs
}

// TestE2EConfigs_EndpointURLsValid verifies that every endpoint URL in each
// E2E config file is a syntactically valid, absolute HTTP/HTTPS URL.
// Ollama endpoints use http:// which is intentional for local dev.
func TestE2EConfigs_EndpointURLsValid(t *testing.T) {
	configs := loadE2EConfigs(t)

	for cfgName, cfg := range configs {
		t.Run(cfgName, func(t *testing.T) {
			if len(cfg.ModelRegistry.Endpoints) == 0 {
				t.Error("model_registry.endpoints is empty")
				return
			}

			for endpointName, ep := range cfg.ModelRegistry.Endpoints {
				if ep.URL == "" {
					t.Errorf("endpoint %q: url is empty", endpointName)
					continue
				}

				parsed, err := url.Parse(ep.URL)
				if err != nil {
					t.Errorf("endpoint %q: url %q failed to parse: %v", endpointName, ep.URL, err)
					continue
				}

				if parsed.Scheme == "" {
					t.Errorf("endpoint %q: url %q has no scheme", endpointName, ep.URL)
				}
				if parsed.Host == "" {
					t.Errorf("endpoint %q: url %q has no host", endpointName, ep.URL)
				}
				if parsed.Scheme != "http" && parsed.Scheme != "https" {
					t.Errorf("endpoint %q: url %q has unexpected scheme %q (want http or https)",
						endpointName, ep.URL, parsed.Scheme)
				}
			}
		})
	}
}

// TestE2EConfigs_ModelIDsNotStale asserts that no endpoint uses a model ID
// containing a known-stale date suffix or an exact known-bad identifier.
// Versioned model IDs like "claude-sonnet-4-6-20250527" are replaced by
// rolling aliases like "claude-sonnet-4-6"; using the old versioned form
// means the config is out of date.
func TestE2EConfigs_ModelIDsNotStale(t *testing.T) {
	configs := loadE2EConfigs(t)

	for cfgName, cfg := range configs {
		t.Run(cfgName, func(t *testing.T) {
			for endpointName, ep := range cfg.ModelRegistry.Endpoints {
				model := ep.Model
				if model == "" {
					t.Errorf("endpoint %q: model is empty", endpointName)
					continue
				}

				// Check exact known-bad IDs first.
				for _, bad := range knownBadModels {
					if model == bad {
						t.Errorf("endpoint %q: model %q is a known-stale ID — use the rolling alias instead",
							endpointName, model)
					}
				}

				// Check for stale date suffixes embedded anywhere in the model ID.
				for _, suffix := range knownBadDateSuffixes {
					if strings.Contains(model, suffix) {
						t.Errorf("endpoint %q: model %q contains stale date suffix %q",
							endpointName, model, suffix)
					}
				}
			}
		})
	}
}

// TestE2EConfigs_CapabilitiesResolvable ensures that every preferred endpoint
// name listed under model_registry.capabilities actually exists in the
// endpoints map. A capability pointing to a non-existent endpoint would cause
// a runtime failure when the model registry tries to resolve it.
func TestE2EConfigs_CapabilitiesResolvable(t *testing.T) {
	configs := loadE2EConfigs(t)

	for cfgName, cfg := range configs {
		t.Run(cfgName, func(t *testing.T) {
			if len(cfg.ModelRegistry.Capabilities) == 0 {
				t.Error("model_registry.capabilities is empty")
				return
			}

			for capName, cap := range cfg.ModelRegistry.Capabilities {
				if len(cap.Preferred) == 0 {
					t.Errorf("capability %q: preferred list is empty", capName)
					continue
				}

				for _, preferred := range cap.Preferred {
					if _, ok := cfg.ModelRegistry.Endpoints[preferred]; !ok {
						t.Errorf("capability %q: preferred endpoint %q not found in endpoints map",
							capName, preferred)
					}
				}
			}

			// Verify the defaults.model endpoint also resolves, if set.
			if defaultModel := cfg.ModelRegistry.Defaults.Model; defaultModel != "" {
				if _, ok := cfg.ModelRegistry.Endpoints[defaultModel]; !ok {
					t.Errorf("defaults.model %q not found in endpoints map", defaultModel)
				}
			}
		})
	}
}

// TestE2EConfigs_APIKeyEnvPlausible checks that every endpoint with a non-empty
// api_key_env uses a recognized env var name. Endpoints without api_key_env
// (e.g. local Ollama) are skipped — they are expected to have no key.
func TestE2EConfigs_APIKeyEnvPlausible(t *testing.T) {
	configs := loadE2EConfigs(t)

	// Build a quick lookup set for O(1) checks.
	validKeys := make(map[string]bool, len(knownAPIKeyPatterns))
	for _, k := range knownAPIKeyPatterns {
		validKeys[k] = true
	}
	validKeyList := strings.Join(knownAPIKeyPatterns, ", ")

	for cfgName, cfg := range configs {
		t.Run(cfgName, func(t *testing.T) {
			for endpointName, ep := range cfg.ModelRegistry.Endpoints {
				if ep.APIKeyEnv == "" {
					// No key required (e.g. Ollama local endpoints). This is valid.
					continue
				}

				if !validKeys[ep.APIKeyEnv] {
					t.Errorf("endpoint %q: api_key_env %q is not a recognized key name (known: %s)",
						endpointName, ep.APIKeyEnv, validKeyList)
				}

				// Sanity: the env var name should be UPPER_SNAKE_CASE.
				if ep.APIKeyEnv != strings.ToUpper(ep.APIKeyEnv) {
					t.Errorf("endpoint %q: api_key_env %q is not uppercase",
						endpointName, ep.APIKeyEnv)
				}
			}
		})
	}
}

// TestE2EConfigs_StructureComplete is a quick smoke-test that every E2E config
// file has non-empty endpoints, capabilities, and a default model. This guards
// against accidentally committing a half-written config.
func TestE2EConfigs_StructureComplete(t *testing.T) {
	configs := loadE2EConfigs(t)

	for cfgName, cfg := range configs {
		t.Run(cfgName, func(t *testing.T) {
			checks := []struct {
				name string
				fail bool
			}{
				{"has at least one endpoint", len(cfg.ModelRegistry.Endpoints) == 0},
				{"has at least one capability", len(cfg.ModelRegistry.Capabilities) == 0},
				{"defaults.model is set", cfg.ModelRegistry.Defaults.Model == ""},
			}

			for _, c := range checks {
				if c.fail {
					t.Errorf("%s: FAIL", c.name)
				}
			}
		})
	}
}

// TestE2EConfigs_EndpointModelsNonEmpty guards against blank model fields,
// which would cause silent failures when the model registry constructs API
// requests (most providers treat an empty model as an error or fallback).
func TestE2EConfigs_EndpointModelsNonEmpty(t *testing.T) {
	configs := loadE2EConfigs(t)

	for cfgName, cfg := range configs {
		t.Run(cfgName, func(t *testing.T) {
			for endpointName, ep := range cfg.ModelRegistry.Endpoints {
				if ep.Model == "" {
					t.Errorf("endpoint %q: model field is empty", endpointName)
				}
				if ep.Provider == "" {
					t.Errorf("endpoint %q: provider field is empty", endpointName)
				}
				// Providers must be lowercase identifiers; mixed-case often
				// indicates a copy-paste error (e.g. "Anthropic" instead of "anthropic").
				if ep.Provider != strings.ToLower(ep.Provider) {
					t.Errorf("endpoint %q: provider %q should be lowercase",
						endpointName, ep.Provider)
				}
			}
		})
	}
}

// minContextWindow is the minimum acceptable max_tokens value for any endpoint.
// The agentic-loop uses max_tokens as the context window budget. A value below
// 32k causes aggressive context compaction that breaks tool_call/tool_result
// message pairing, leading to Anthropic 400 errors and similar provider failures.
const minContextWindow = 32000

// TestE2EConfigs_ContextWindowsSane guards against copy-paste errors where
// max_tokens is set to a small output-token value (e.g. 8192) instead of the
// actual model context window (e.g. 200000 for Claude, 1M for Gemini).
func TestE2EConfigs_ContextWindowsSane(t *testing.T) {
	configs := loadE2EConfigs(t)

	for cfgName, cfg := range configs {
		t.Run(cfgName, func(t *testing.T) {
			for endpointName, ep := range cfg.ModelRegistry.Endpoints {
				if ep.MaxTokens < minContextWindow {
					t.Errorf("endpoint %q: max_tokens=%d is below minimum %d — "+
						"this should be the model context window, not output token limit",
						endpointName, ep.MaxTokens, minContextWindow)
				}
			}
		})
	}
}

// knownProviders is the set of provider identifiers the runtime understands.
// An unrecognized provider string will fail at startup, not at config load time,
// making this test a useful early-warning check.
var knownProviders = map[string]bool{
	"anthropic": true,
	"openai":    true,
	"ollama":    true,
	"gemini":    true,
}

// TestE2EConfigs_ProvidersRecognized ensures each endpoint's provider field is
// one the runtime can actually handle.
func TestE2EConfigs_ProvidersRecognized(t *testing.T) {
	configs := loadE2EConfigs(t)

	knownList := func() string {
		names := make([]string, 0, len(knownProviders))
		for k := range knownProviders {
			names = append(names, k)
		}
		return fmt.Sprintf("%v", names)
	}

	for cfgName, cfg := range configs {
		t.Run(cfgName, func(t *testing.T) {
			for endpointName, ep := range cfg.ModelRegistry.Endpoints {
				if !knownProviders[ep.Provider] {
					t.Errorf("endpoint %q: provider %q is not in known provider list %s",
						endpointName, ep.Provider, knownList())
				}
			}
		})
	}
}
