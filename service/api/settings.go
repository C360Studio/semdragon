package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"slices"

	"github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/model"
	"github.com/c360studio/semstreams/types"
)

// =============================================================================
// RESPONSE TYPES
// =============================================================================

// SettingsResponse is the response body for GET /settings.
type SettingsResponse struct {
	Platform       PlatformInfo         `json:"platform"`
	NATS           NATSInfo             `json:"nats"`
	Models         ModelRegistryView    `json:"models"`
	Components     []ComponentInfoView  `json:"components"`
	Workspace      WorkspaceInfoView    `json:"workspace"`
	TokenBudget    *TokenBudgetView     `json:"token_budget,omitempty"`
	WebsocketInput WebsocketInputView   `json:"websocket_input"`
	SearchConfig   SearchConfigView     `json:"search_config"`
}

// PlatformInfo describes the deployment identity.
type PlatformInfo struct {
	Org      string `json:"org" description:"Organization namespace"`
	Platform string `json:"platform" description:"Platform/deployment ID"`
	Board    string `json:"board" description:"Quest board name"`
}

// NATSInfo shows NATS connection status.
type NATSInfo struct {
	Connected bool    `json:"connected" description:"Whether NATS is connected"`
	URL       string  `json:"url" description:"NATS server URL"`
	LatencyMs float64 `json:"latency_ms,omitempty" description:"Round-trip time in milliseconds"`
}

// ModelRegistryView exposes the model registry without API key values.
type ModelRegistryView struct {
	Endpoints    []ModelEndpointView          `json:"endpoints" description:"Configured LLM endpoints"`
	Capabilities map[string]CapabilityView    `json:"capabilities" description:"Capability routing"`
	Defaults     ModelDefaultsView            `json:"defaults" description:"Default model settings"`
}

// ModelEndpointView describes a single endpoint. API key values are never exposed.
type ModelEndpointView struct {
	Name                   string  `json:"name" description:"Endpoint name"`
	Provider               string  `json:"provider" description:"Provider type (anthropic, openai, ollama, openrouter)"`
	Model                  string  `json:"model" description:"Model identifier"`
	URL                    string  `json:"url,omitempty" description:"API endpoint URL"`
	MaxTokens              int     `json:"max_tokens" description:"Max context window"`
	SupportsTools          bool    `json:"supports_tools" description:"Whether tools/function calling is supported"`
	ToolFormat             string  `json:"tool_format,omitempty" description:"Tool format (anthropic or openai)"`
	APIKeyEnv              string  `json:"api_key_env,omitempty" description:"Environment variable name for API key"`
	APIKeySet              bool    `json:"api_key_set" description:"Whether the API key env var has a value"`
	Stream                 bool    `json:"stream,omitempty" description:"SSE streaming enabled"`
	ReasoningEffort        string  `json:"reasoning_effort,omitempty" description:"Reasoning effort level"`
	InputPricePer1MTokens  float64 `json:"input_price_per_1m_tokens,omitempty" description:"Cost per 1M input tokens USD"`
	OutputPricePer1MTokens float64 `json:"output_price_per_1m_tokens,omitempty" description:"Cost per 1M output tokens USD"`
}

// CapabilityView describes a capability routing entry.
type CapabilityView struct {
	Description   string   `json:"description" description:"What this capability is for"`
	Preferred     []string `json:"preferred" description:"Preferred endpoint chain"`
	Fallback      []string `json:"fallback,omitempty" description:"Fallback endpoint chain"`
	RequiresTools bool     `json:"requires_tools,omitempty" description:"Whether endpoints must support tools"`
}

// ModelDefaultsView shows default model settings.
type ModelDefaultsView struct {
	Model      string `json:"model" description:"Default endpoint name"`
	Capability string `json:"capability,omitempty" description:"Default capability"`
}

// ComponentInfoView describes a registered component's status.
type ComponentInfoView struct {
	Name         string `json:"name" description:"Component instance name"`
	Type         string `json:"type" description:"Component type (processor, input, output, storage)"`
	Enabled      bool   `json:"enabled" description:"Whether the component is in the config"`
	Running      bool   `json:"running" description:"Whether the component is instantiated"`
	Healthy      bool   `json:"healthy" description:"Whether the component reports healthy"`
	Status       string `json:"status,omitempty" description:"Health status string"`
	UptimeSecs   int64  `json:"uptime_seconds,omitempty" description:"Seconds since component started"`
	ErrorCount   int    `json:"error_count,omitempty" description:"Total error count"`
	LastError    string `json:"last_error,omitempty" description:"Most recent error message"`
}

// WorkspaceInfoView describes workspace directory status.
type WorkspaceInfoView struct {
	Dir      string `json:"dir" description:"Workspace directory path"`
	Exists   bool   `json:"exists" description:"Whether the directory exists"`
	Writable bool   `json:"writable" description:"Whether the directory is writable"`
}

// TokenBudgetView describes token budget config.
type TokenBudgetView struct {
	GlobalHourlyLimit int64 `json:"global_hourly_limit" description:"Hourly token limit (0 = unlimited)"`
}

// WebsocketInputView describes the websocket input component status.
type WebsocketInputView struct {
	Enabled   bool   `json:"enabled" description:"Whether the websocket input is enabled"`
	URL       string `json:"url" description:"WebSocket server URL"`
	Connected bool   `json:"connected" description:"Whether currently connected (only meaningful when enabled)"`
	Healthy   bool   `json:"healthy" description:"Component health status"`
	Status    string `json:"status,omitempty" description:"Human-readable status"`
}

// SearchConfigView exposes search configuration without revealing the raw API key.
type SearchConfigView struct {
	Provider  string `json:"provider" description:"Search provider (e.g. brave)"`
	APIKeySet bool   `json:"api_key_set" description:"Whether an API key is configured"`
	BaseURL   string `json:"base_url,omitempty" description:"Custom API endpoint"`
}

// =============================================================================
// HEALTH TYPES
// =============================================================================

// HealthResponse is the response body for GET /settings/health.
type HealthResponse struct {
	Overall   string          `json:"overall" description:"Overall health: healthy, degraded, unhealthy"`
	Checks    []HealthCheck   `json:"checks" description:"Individual health checks"`
	Checklist []ChecklistItem `json:"checklist" description:"Onboarding prerequisite checklist"`
}

// HealthCheck is a single validation check result.
type HealthCheck struct {
	Name    string `json:"name" description:"Check identifier"`
	Status  string `json:"status" description:"ok, warning, or error"`
	Message string `json:"message" description:"Human-readable status message"`
}

// ChecklistItem is a single onboarding prerequisite.
type ChecklistItem struct {
	Label    string `json:"label" description:"Prerequisite description"`
	Met      bool   `json:"met" description:"Whether the prerequisite is satisfied"`
	HelpText string `json:"help_text,omitempty" description:"Guidance when not met"`
}

// =============================================================================
// UPDATE TYPES
// =============================================================================

// UpdateSettingsRequest is the request body for POST /settings.
type UpdateSettingsRequest struct {
	ModelRegistry  *ModelRegistryUpdate  `json:"model_registry,omitempty" description:"Model registry changes"`
	TokenBudget    *TokenBudgetView      `json:"token_budget,omitempty" description:"Token budget changes"`
	WebsocketInput *WebsocketInputUpdate `json:"websocket_input,omitempty" description:"WebSocket input changes"`
	SearchConfig   *SearchConfigUpdate   `json:"search_config,omitempty" description:"Web search configuration changes"`
}

// WebsocketInputUpdate describes mutations to the websocket input component.
type WebsocketInputUpdate struct {
	Enabled *bool   `json:"enabled,omitempty" description:"Enable or disable the websocket input"`
	URL     *string `json:"url,omitempty" description:"WebSocket server URL to connect to"`
}

// SearchConfigUpdate describes mutations to the web search configuration.
type SearchConfigUpdate struct {
	Provider *string `json:"provider,omitempty" description:"Search provider type"`
	APIKey   *string `json:"api_key,omitempty" description:"API key for the provider"`
	BaseURL  *string `json:"base_url,omitempty" description:"Custom API endpoint"`
}

// ModelRegistryUpdate describes mutations to the model registry.
type ModelRegistryUpdate struct {
	Endpoints    map[string]*EndpointUpdate    `json:"endpoints,omitempty" description:"Endpoint add/update/remove"`
	Capabilities map[string]*CapabilityUpdate  `json:"capabilities,omitempty" description:"Capability add/update/remove"`
	Defaults     *ModelDefaultsView            `json:"defaults,omitempty" description:"Default model changes"`
}

// EndpointUpdate describes a single endpoint mutation.
type EndpointUpdate struct {
	Provider               string         `json:"provider" description:"Provider type"`
	URL                    string         `json:"url,omitempty" description:"API endpoint URL"`
	Model                  string         `json:"model" description:"Model identifier"`
	MaxTokens              int            `json:"max_tokens" description:"Max context window"`
	SupportsTools          bool           `json:"supports_tools" description:"Tool calling support"`
	ToolFormat             string         `json:"tool_format,omitempty" description:"Tool format"`
	APIKeyEnv              string         `json:"api_key_env,omitempty" description:"API key env var name"`
	APIKeyValue            *string        `json:"api_key_value,omitempty" description:"Actual key value — written to .env, never stored in config"`
	Stream                 bool           `json:"stream,omitempty" description:"SSE streaming"`
	ReasoningEffort        string         `json:"reasoning_effort,omitempty" description:"Reasoning effort"`
	InputPricePer1MTokens  float64        `json:"input_price_per_1m_tokens,omitempty" description:"Input cost"`
	OutputPricePer1MTokens float64        `json:"output_price_per_1m_tokens,omitempty" description:"Output cost"`
	Options                map[string]any `json:"options,omitempty" description:"Provider-specific options"`
	Remove                 bool           `json:"remove,omitempty" description:"Set true to remove this endpoint"`
}

// CapabilityUpdate describes a single capability mutation.
type CapabilityUpdate struct {
	Description   string   `json:"description,omitempty" description:"Capability description"`
	Preferred     []string `json:"preferred,omitempty" description:"Preferred endpoint chain"`
	Fallback      []string `json:"fallback,omitempty" description:"Fallback endpoint chain"`
	RequiresTools bool     `json:"requires_tools,omitempty" description:"Require tool support"`
	Remove        bool     `json:"remove,omitempty" description:"Set true to remove this capability"`
}

// =============================================================================
// HANDLERS
// =============================================================================

// handleGetSettings returns the current runtime configuration.
// API key values are never included — only the env var name and whether it is set.
func (s *Service) handleGetSettings(w http.ResponseWriter, _ *http.Request) {
	resp := s.assembleSettingsResponse()
	s.writeJSON(w, resp)
}

// handleSettingsHealth runs live validation checks and returns an onboarding checklist.
func (s *Service) handleSettingsHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var checks []HealthCheck
	hasError := false
	hasWarning := false

	// 1. NATS connection
	if s.nats.IsHealthy() {
		rtt, err := s.nats.RTT()
		if err != nil {
			checks = append(checks, HealthCheck{Name: "nats", Status: "warning", Message: "Connected but RTT check failed"})
			hasWarning = true
		} else if rtt > 100*time.Millisecond {
			checks = append(checks, HealthCheck{Name: "nats", Status: "warning", Message: fmt.Sprintf("Connected, RTT %.1fms (high)", float64(rtt.Microseconds())/1000)})
			hasWarning = true
		} else {
			checks = append(checks, HealthCheck{Name: "nats", Status: "ok", Message: fmt.Sprintf("Connected, RTT %.1fms", float64(rtt.Microseconds())/1000)})
		}
	} else {
		checks = append(checks, HealthCheck{Name: "nats", Status: "error", Message: "Not connected"})
		hasError = true
	}

	// 2. LLM endpoint API key checks
	if s.models != nil {
		for _, name := range s.models.ListEndpoints() {
			ep := s.models.GetEndpoint(name)
			if ep == nil {
				continue
			}
			checkName := "llm_endpoint:" + name
			if ep.APIKeyEnv == "" {
				// Local endpoint (ollama), no key needed
				checks = append(checks, HealthCheck{Name: checkName, Status: "ok", Message: fmt.Sprintf("Configured (%s, no API key needed)", ep.Provider)})
			} else if os.Getenv(ep.APIKeyEnv) != "" {
				checks = append(checks, HealthCheck{Name: checkName, Status: "ok", Message: fmt.Sprintf("%s is set", ep.APIKeyEnv)})
			} else {
				checks = append(checks, HealthCheck{Name: checkName, Status: "error", Message: fmt.Sprintf("%s is not set", ep.APIKeyEnv)})
				hasError = true
			}
		}
	}

	// 3. Workspace directory
	wsDir := s.config.WorkspaceDir
	if wsDir == "" {
		checks = append(checks, HealthCheck{Name: "workspace", Status: "warning", Message: "No workspace directory configured"})
		hasWarning = true
	} else if info, err := os.Stat(wsDir); err != nil {
		checks = append(checks, HealthCheck{Name: "workspace", Status: "error", Message: fmt.Sprintf("%s does not exist", wsDir)})
		hasError = true
	} else if !info.IsDir() {
		checks = append(checks, HealthCheck{Name: "workspace", Status: "error", Message: fmt.Sprintf("%s is not a directory", wsDir)})
		hasError = true
	} else {
		// Check writable
		tmpFile, tmpErr := os.CreateTemp(wsDir, ".settings-health-check-*")
		if tmpErr != nil {
			checks = append(checks, HealthCheck{Name: "workspace", Status: "warning", Message: fmt.Sprintf("%s exists but is not writable", wsDir)})
			hasWarning = true
		} else {
			_ = tmpFile.Close()
			_ = os.Remove(tmpFile.Name())
			checks = append(checks, HealthCheck{Name: "workspace", Status: "ok", Message: fmt.Sprintf("%s exists and is writable", wsDir)})
		}
	}

	// 4. AGENT stream
	js, jsErr := s.nats.JetStream()
	if jsErr == nil {
		streamCtx, streamCancel := context.WithTimeout(ctx, 3*time.Second)
		_, streamErr := js.Stream(streamCtx, "AGENT")
		streamCancel()
		if streamErr != nil {
			checks = append(checks, HealthCheck{Name: "stream:AGENT", Status: "error", Message: "AGENT stream not found"})
			hasError = true
		} else {
			checks = append(checks, HealthCheck{Name: "stream:AGENT", Status: "ok", Message: "Stream exists"})
		}
	}

	// 5. Entity state bucket
	bucketName := s.boardConfig.BucketName()
	bucketCtx, bucketCancel := context.WithTimeout(ctx, 3*time.Second)
	_, bucketErr := s.nats.GetKeyValueBucket(bucketCtx, bucketName)
	bucketCancel()
	if bucketErr != nil {
		checks = append(checks, HealthCheck{Name: "bucket:entity_state", Status: "error", Message: fmt.Sprintf("Bucket %s not found", bucketName)})
		hasError = true
	} else {
		checks = append(checks, HealthCheck{Name: "bucket:entity_state", Status: "ok", Message: "Bucket exists"})
	}

	// Overall
	overall := "healthy"
	if hasWarning {
		overall = "degraded"
	}
	if hasError {
		overall = "unhealthy"
	}

	// Onboarding checklist
	checklist := s.buildChecklist(ctx)

	s.writeJSON(w, HealthResponse{
		Overall:   overall,
		Checks:    checks,
		Checklist: checklist,
	})
}

// handleUpdateSettings mutates runtime-mutable settings.
// Auth required. Returns the updated SettingsResponse.
func (s *Service) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	var req UpdateSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Token budget update (independent of model registry)
	if req.TokenBudget != nil && s.tokenLedger != nil {
		if err := s.tokenLedger.SetBudget(r.Context(), req.TokenBudget.GlobalHourlyLimit); err != nil {
			s.writeError(w, fmt.Sprintf("failed to set budget: %v", err), http.StatusBadRequest)
			return
		}
		// Keep in-memory config in sync so GET /settings reflects the new value
		// even when the config file on disk is read-only.
		if s.config.TokenBudget != nil {
			s.config.TokenBudget.GlobalHourlyLimit = req.TokenBudget.GlobalHourlyLimit
		}
	}

	// Model registry update
	if req.ModelRegistry != nil {
		if err := s.applyModelRegistryUpdate(r.Context(), req.ModelRegistry); err != nil {
			s.writeError(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	// Websocket input update
	if req.WebsocketInput != nil {
		if err := s.applyWebsocketInputUpdate(r.Context(), req.WebsocketInput); err != nil {
			s.writeError(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Search config update
	if req.SearchConfig != nil {
		if err := s.applySearchConfigUpdate(r.Context(), req.SearchConfig); err != nil {
			s.writeError(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Return updated state
	resp := s.assembleSettingsResponse()
	s.writeJSON(w, resp)
}

// =============================================================================
// INTERNAL HELPERS
// =============================================================================

// assembleSettingsResponse builds the full settings response from current state.
func (s *Service) assembleSettingsResponse() SettingsResponse {
	resp := SettingsResponse{
		Platform: PlatformInfo{
			Org:      s.boardConfig.Org,
			Platform: s.boardConfig.Platform,
			Board:    s.boardConfig.Board,
		},
	}

	// NATS status
	resp.NATS = NATSInfo{
		Connected: s.nats.IsHealthy(),
		URL:       s.nats.URLs(),
	}
	if rtt, err := s.nats.RTT(); err == nil {
		resp.NATS.LatencyMs = float64(rtt.Microseconds()) / 1000
	}

	// Model registry
	resp.Models = s.assembleModelRegistryView()

	// Components
	resp.Components = s.assembleComponentList()

	// Workspace
	wsDir := s.config.WorkspaceDir
	resp.Workspace = WorkspaceInfoView{Dir: wsDir}
	if wsDir != "" {
		if info, err := os.Stat(wsDir); err == nil && info.IsDir() {
			resp.Workspace.Exists = true
			tmpFile, tmpErr := os.CreateTemp(wsDir, ".settings-check-*")
			if tmpErr == nil {
				resp.Workspace.Writable = true
				_ = tmpFile.Close()
				_ = os.Remove(tmpFile.Name())
			}
		}
	}

	// Token budget
	if s.config.TokenBudget != nil {
		resp.TokenBudget = &TokenBudgetView{
			GlobalHourlyLimit: s.config.TokenBudget.GlobalHourlyLimit,
		}
	} else if s.tokenLedger != nil {
		stats := s.tokenLedger.Stats()
		resp.TokenBudget = &TokenBudgetView{
			GlobalHourlyLimit: stats.HourlyLimit,
		}
	}

	// Websocket input
	resp.WebsocketInput = s.assembleWebsocketInputView()

	// Search config
	resp.SearchConfig = s.assembleSearchConfigView()

	return resp
}

// websocketInputComponentName is the config key for the websocket input component.
const websocketInputComponentName = "websocket_input"

func (s *Service) assembleWebsocketInputView() WebsocketInputView {
	view := WebsocketInputView{}

	// Read config from the config manager
	cfg := s.getComponentConfig(websocketInputComponentName)
	if cfg != nil {
		view.Enabled = cfg.Enabled
		view.URL = extractWebsocketURL(cfg.Config)
	}

	// Read health from component registry (only if running)
	if s.componentDeps != nil && s.componentDeps.ComponentRegistry != nil {
		if comp := s.componentDeps.ComponentRegistry.Component(websocketInputComponentName); comp != nil {
			health := comp.Health()
			view.Healthy = health.Healthy
			view.Connected = health.Healthy // ws client reports healthy when connected
			view.Status = health.Status
		}
	}

	return view
}

// questtoolsComponentName is the config key for the questtools component.
const questtoolsComponentName = "questtools"

// assembleSearchConfigView reads the questtools component config and returns a view
// that exposes search configuration without revealing the raw API key value.
// The API key is looked up from the env var named by api_key_env.
func (s *Service) assembleSearchConfigView() SearchConfigView {
	cfg := s.getComponentConfig(questtoolsComponentName)
	if cfg == nil {
		return SearchConfigView{}
	}

	var raw struct {
		Search *struct {
			Provider  string `json:"provider"`
			APIKeyEnv string `json:"api_key_env"`
			BaseURL   string `json:"base_url"`
		} `json:"search"`
	}
	if err := json.Unmarshal(cfg.Config, &raw); err != nil || raw.Search == nil {
		return SearchConfigView{}
	}

	apiKeySet := raw.Search.APIKeyEnv != "" && os.Getenv(raw.Search.APIKeyEnv) != ""
	return SearchConfigView{
		Provider:  raw.Search.Provider,
		APIKeySet: apiKeySet,
		BaseURL:   raw.Search.BaseURL,
	}
}

// applySearchConfigUpdate mutates the questtools component's search configuration.
// Changes are pushed to KV and saved to disk following the same pattern as applyWebsocketInputUpdate.
func (s *Service) applySearchConfigUpdate(ctx context.Context, update *SearchConfigUpdate) error {
	if s.componentDeps == nil || s.componentDeps.Manager == nil {
		return fmt.Errorf("config manager unavailable")
	}

	safeCfg := s.componentDeps.Manager.GetConfig()
	if safeCfg == nil {
		return fmt.Errorf("config not available")
	}
	cfg := safeCfg.Get()
	if cfg == nil {
		return fmt.Errorf("config not available")
	}

	cloned := cfg.Clone()
	if cloned.Components == nil {
		cloned.Components = make(config.ComponentConfigs)
	}

	cc, exists := cloned.Components[questtoolsComponentName]
	if !exists {
		cc = types.ComponentConfig{}
	}

	// Unmarshal existing search config from the component's raw JSON.
	var compCfg map[string]any
	if len(cc.Config) > 0 {
		if err := json.Unmarshal(cc.Config, &compCfg); err != nil {
			return fmt.Errorf("failed to parse questtools config: %w", err)
		}
	}
	if compCfg == nil {
		compCfg = make(map[string]any)
	}

	// Extract existing search sub-object or start fresh.
	searchRaw, _ := compCfg["search"].(map[string]any)
	if searchRaw == nil {
		searchRaw = make(map[string]any)
	}

	// Apply updates.
	if update.Provider != nil {
		searchRaw["provider"] = *update.Provider
	}
	if update.APIKey != nil && *update.APIKey != "" {
		// Derive the env var name from the provider. Default to BRAVE_SEARCH_API_KEY
		// when the provider has not been set yet in this request.
		provider, _ := searchRaw["provider"].(string)
		if update.Provider != nil {
			provider = *update.Provider
		}
		envVarName := searchAPIKeyEnvVar(provider)

		// Write the key value to .env and set it in the process environment.
		// The raw key is never persisted in config JSON or NATS KV.
		if err := writeEnvVar(envVarName, *update.APIKey); err != nil {
			return fmt.Errorf("failed to write search API key to .env: %w", err)
		}
		// Store only the env var name in config.
		searchRaw["api_key_env"] = envVarName
	}
	if update.BaseURL != nil {
		searchRaw["base_url"] = *update.BaseURL
	}

	compCfg["search"] = searchRaw

	// Re-marshal the modified config back into the component's Config field.
	rawConfig, err := json.Marshal(compCfg)
	if err != nil {
		return fmt.Errorf("failed to serialize questtools config: %w", err)
	}
	cc.Config = rawConfig
	cloned.Components[questtoolsComponentName] = cc

	// Update in-memory.
	if err := safeCfg.Update(cloned); err != nil {
		return fmt.Errorf("failed to update config: %w", err)
	}

	// Push to KV for multi-node propagation.
	pushCtx, pushCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pushCancel()
	if err := s.componentDeps.Manager.PushToKV(pushCtx); err != nil {
		s.logger.Warn("failed to push config to KV", "error", err)
	}

	// Persist to disk.
	if configPath := os.Getenv("SEMDRAGONS_CONFIG"); configPath != "" {
		if err := cloned.SaveToFile(configPath); err != nil {
			s.logger.Warn("failed to save config to disk", "error", err)
		}
	}

	return nil
}

// getComponentConfig retrieves a component's config from the config manager.
func (s *Service) getComponentConfig(name string) *types.ComponentConfig {
	if s.componentDeps == nil || s.componentDeps.Manager == nil {
		return nil
	}
	safeCfg := s.componentDeps.Manager.GetConfig()
	if safeCfg == nil {
		return nil
	}
	cfg := safeCfg.Get()
	if cfg == nil {
		return nil
	}
	cc, exists := cfg.Components[name]
	if !exists {
		return nil
	}
	return &cc
}

// extractWebsocketURL parses the websocket URL from component config JSON.
func extractWebsocketURL(rawConfig json.RawMessage) string {
	if len(rawConfig) == 0 {
		return ""
	}
	var wsConfig struct {
		Client struct {
			URL string `json:"url"`
		} `json:"client"`
	}
	if err := json.Unmarshal(rawConfig, &wsConfig); err != nil {
		return ""
	}
	return wsConfig.Client.URL
}

func (s *Service) assembleModelRegistryView() ModelRegistryView {
	view := ModelRegistryView{
		Endpoints:    []ModelEndpointView{},
		Capabilities: make(map[string]CapabilityView),
		Defaults:     ModelDefaultsView{},
	}

	if s.models == nil {
		return view
	}

	// Endpoints
	for _, name := range s.models.ListEndpoints() {
		ep := s.models.GetEndpoint(name)
		if ep == nil {
			continue
		}
		epView := ModelEndpointView{
			Name:                   name,
			Provider:               ep.Provider,
			Model:                  ep.Model,
			URL:                    ep.URL,
			MaxTokens:              ep.MaxTokens,
			SupportsTools:          ep.SupportsTools,
			ToolFormat:             ep.ToolFormat,
			APIKeyEnv:              ep.APIKeyEnv,
			APIKeySet:              ep.APIKeyEnv != "" && os.Getenv(ep.APIKeyEnv) != "",
			Stream:                 ep.Stream,
			ReasoningEffort:        ep.ReasoningEffort,
			InputPricePer1MTokens:  ep.InputPricePer1MTokens,
			OutputPricePer1MTokens: ep.OutputPricePer1MTokens,
		}
		view.Endpoints = append(view.Endpoints, epView)
	}

	// Capabilities
	for _, capName := range s.models.ListCapabilities() {
		chain := s.models.GetFallbackChain(capName)
		capView := CapabilityView{
			Preferred: chain,
		}

		// Try to get full capability config from concrete registry
		if reg, ok := s.models.(*model.Registry); ok && reg.Capabilities != nil {
			if capCfg, exists := reg.Capabilities[capName]; exists {
				capView.Description = capCfg.Description
				capView.Preferred = capCfg.Preferred
				capView.Fallback = capCfg.Fallback
				capView.RequiresTools = capCfg.RequiresTools
			}
		}

		view.Capabilities[capName] = capView
	}

	// Defaults
	if reg, ok := s.models.(*model.Registry); ok {
		view.Defaults = ModelDefaultsView{
			Model:      reg.Defaults.Model,
			Capability: reg.Defaults.Capability,
		}
	}

	return view
}

func (s *Service) assembleComponentList() []ComponentInfoView {
	var components []ComponentInfoView

	if s.componentDeps == nil || s.componentDeps.ComponentRegistry == nil {
		return components
	}

	for name, comp := range s.componentDeps.ComponentRegistry.ListComponents() {
		meta := comp.Meta()
		health := comp.Health()

		components = append(components, ComponentInfoView{
			Name:       name,
			Type:       meta.Type,
			Enabled:    true, // present in registry = enabled
			Running:    true, // listed = instantiated
			Healthy:    health.Healthy,
			Status:     health.Status,
			UptimeSecs: int64(health.Uptime.Seconds()),
			ErrorCount: health.ErrorCount,
			LastError:  health.LastError,
		})
	}

	return components
}

func (s *Service) buildChecklist(ctx context.Context) []ChecklistItem {
	items := []ChecklistItem{
		{Label: "NATS connected", Met: s.nats.IsHealthy()},
	}

	// LLM endpoint configured
	endpointConfigured := false
	apiKeyReady := false
	if s.models != nil {
		endpoints := s.models.ListEndpoints()
		endpointConfigured = len(endpoints) > 0
		for _, name := range endpoints {
			ep := s.models.GetEndpoint(name)
			if ep == nil {
				continue
			}
			if ep.APIKeyEnv == "" {
				// Ollama or local — no key needed
				apiKeyReady = true
				break
			}
			if os.Getenv(ep.APIKeyEnv) != "" {
				apiKeyReady = true
				break
			}
		}
	}
	items = append(items, ChecklistItem{
		Label: "LLM endpoint configured",
		Met:   endpointConfigured,
		HelpText: func() string {
			if endpointConfigured {
				return ""
			}
			return "Add at least one model endpoint in config/semdragons.json model_registry.endpoints"
		}(),
	})
	items = append(items, ChecklistItem{
		Label: "API key set for LLM provider",
		Met:   apiKeyReady,
		HelpText: func() string {
			if apiKeyReady {
				return ""
			}
			return "Set the API key environment variable for your LLM provider (e.g., ANTHROPIC_API_KEY, GEMINI_API_KEY), or use Ollama which needs no key"
		}(),
	})

	// Workspace
	wsOk := false
	wsDir := s.config.WorkspaceDir
	if wsDir != "" {
		if info, err := os.Stat(wsDir); err == nil && info.IsDir() {
			tmpFile, tmpErr := os.CreateTemp(wsDir, ".checklist-*")
			if tmpErr == nil {
				wsOk = true
				_ = tmpFile.Close()
				_ = os.Remove(tmpFile.Name())
			}
		}
	}
	items = append(items, ChecklistItem{
		Label: "Workspace directory exists and is writable",
		Met:   wsOk,
		HelpText: func() string {
			if wsOk {
				return ""
			}
			if wsDir == "" {
				return "Set workspace_dir in config/semdragons.json services.game.config. For local dev: mkdir -p .workspace"
			}
			return fmt.Sprintf("Create directory %s or update workspace_dir in config", wsDir)
		}(),
	})

	// At least one agent
	agentMet := false
	agents, err := s.graph.ListAgentsByPrefix(ctx, 1)
	if err == nil && len(agents) > 0 {
		agentMet = true
	}
	items = append(items, ChecklistItem{
		Label: "At least one agent recruited",
		Met:   agentMet,
		HelpText: func() string {
			if agentMet {
				return ""
			}
			return "POST /api/game/agents to recruit your first agent, or use SEED_AGENTS=true"
		}(),
	})

	// At least one quest
	questMet := false
	quests, err := s.graph.ListQuestsByPrefix(ctx, 1)
	if err == nil && len(quests) > 0 {
		questMet = true
	}
	items = append(items, ChecklistItem{
		Label: "At least one quest posted",
		Met:   questMet,
		HelpText: func() string {
			if questMet {
				return ""
			}
			return "POST /api/game/quests with a title and goal to create your first quest"
		}(),
	})

	return items
}

// applyModelRegistryUpdate applies model registry mutations from a POST request.
func (s *Service) applyModelRegistryUpdate(ctx context.Context, update *ModelRegistryUpdate) error {
	// Get current registry as concrete type
	reg, ok := s.models.(*model.Registry)
	if !ok {
		return fmt.Errorf("model registry is not mutable")
	}

	// Deep copy by marshaling/unmarshaling
	data, err := json.Marshal(reg)
	if err != nil {
		return fmt.Errorf("failed to serialize registry: %w", err)
	}
	var updated model.Registry
	if err := json.Unmarshal(data, &updated); err != nil {
		return fmt.Errorf("failed to clone registry: %w", err)
	}

	// Apply endpoint changes
	if update.Endpoints != nil {
		if updated.Endpoints == nil {
			updated.Endpoints = make(map[string]*model.EndpointConfig)
		}
		for name, ep := range update.Endpoints {
			if ep.Remove {
				delete(updated.Endpoints, name)
				continue
			}

			// When the caller provides an actual key value, persist it to .env
			// and set it in the process environment. The raw value is never stored
			// in config JSON or pushed to NATS KV — only the env var name travels
			// through the config pipeline.
			apiKeyEnv := ep.APIKeyEnv
			if ep.APIKeyValue != nil && *ep.APIKeyValue != "" {
				if apiKeyEnv == "" {
					return fmt.Errorf("api_key_value provided for endpoint %q but api_key_env is not set", name)
				}
				if err := writeEnvVar(apiKeyEnv, *ep.APIKeyValue); err != nil {
					return fmt.Errorf("failed to write API key for endpoint %q to .env: %w", name, err)
				}
			}

			updated.Endpoints[name] = &model.EndpointConfig{
				Provider:               ep.Provider,
				URL:                    ep.URL,
				Model:                  ep.Model,
				MaxTokens:              ep.MaxTokens,
				SupportsTools:          ep.SupportsTools,
				ToolFormat:             ep.ToolFormat,
				APIKeyEnv:              apiKeyEnv,
				Stream:                 ep.Stream,
				ReasoningEffort:        ep.ReasoningEffort,
				InputPricePer1MTokens:  ep.InputPricePer1MTokens,
				OutputPricePer1MTokens: ep.OutputPricePer1MTokens,
				Options:                ep.Options,
			}
		}
	}

	// Apply capability changes
	if update.Capabilities != nil {
		if updated.Capabilities == nil {
			updated.Capabilities = make(map[string]*model.CapabilityConfig)
		}
		for name, cap := range update.Capabilities {
			if cap.Remove {
				delete(updated.Capabilities, name)
				continue
			}
			updated.Capabilities[name] = &model.CapabilityConfig{
				Description:   cap.Description,
				Preferred:     cap.Preferred,
				Fallback:      cap.Fallback,
				RequiresTools: cap.RequiresTools,
			}
		}
	}

	// Apply defaults
	if update.Defaults != nil {
		updated.Defaults = model.DefaultsConfig{
			Model:      update.Defaults.Model,
			Capability: update.Defaults.Capability,
		}
	}

	// Validate — checks that capabilities reference existing endpoints, etc.
	if err := updated.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Validate that removed endpoints aren't referenced by remaining capabilities
	if update.Endpoints != nil {
		for name, ep := range update.Endpoints {
			if !ep.Remove {
				continue
			}
			for capName, cap := range updated.Capabilities {
				if slices.Contains(cap.Preferred, name) {
					return fmt.Errorf("cannot remove endpoint %q: referenced by capability %q preferred chain", name, capName)
				}
				if slices.Contains(cap.Fallback, name) {
					return fmt.Errorf("cannot remove endpoint %q: referenced by capability %q fallback chain", name, capName)
				}
			}
		}
	}

	// Apply: update in-memory config via manager
	if s.componentDeps != nil && s.componentDeps.Manager != nil {
		safeCfg := s.componentDeps.Manager.GetConfig()
		if safeCfg != nil {
			cfg := safeCfg.Get()
			if cfg != nil {
				cloned := cfg.Clone()
				cloned.ModelRegistry = &updated

				if updateErr := safeCfg.Update(cloned); updateErr != nil {
					return fmt.Errorf("failed to update config: %w", updateErr)
				}

				// Persist to KV for multi-node propagation
				pushCtx, pushCancel := context.WithTimeout(ctx, 5*time.Second)
				defer pushCancel()
				if pushErr := s.componentDeps.Manager.PushToKV(pushCtx); pushErr != nil {
					s.logger.Warn("failed to push config to KV", "error", pushErr)
					// Non-fatal — in-memory update succeeded
				}

				// Persist to disk if config path is known
				if configPath := os.Getenv("SEMDRAGONS_CONFIG"); configPath != "" {
					if saveErr := cloned.SaveToFile(configPath); saveErr != nil {
						s.logger.Warn("failed to save config to disk", "error", saveErr)
						// Non-fatal — in-memory and KV updates succeeded
					}
				}
			}
		}
	}

	// Update local reference so subsequent GET reflects changes
	s.models = &updated

	return nil
}

// searchAPIKeyEnvVar returns the conventional environment variable name for a
// search provider's API key. Falls back to BRAVE_SEARCH_API_KEY for unknown
// or empty provider names because Brave is the only supported provider today.
func searchAPIKeyEnvVar(provider string) string {
	switch provider {
	case "brave":
		return "BRAVE_SEARCH_API_KEY"
	default:
		return "BRAVE_SEARCH_API_KEY"
	}
}

// applyWebsocketInputUpdate mutates the websocket_input component config.
// Changes are pushed to KV, which the component manager picks up via watch_config.
func (s *Service) applyWebsocketInputUpdate(ctx context.Context, update *WebsocketInputUpdate) error {
	if s.componentDeps == nil || s.componentDeps.Manager == nil {
		return fmt.Errorf("config manager unavailable")
	}

	safeCfg := s.componentDeps.Manager.GetConfig()
	if safeCfg == nil {
		return fmt.Errorf("config not available")
	}
	cfg := safeCfg.Get()
	if cfg == nil {
		return fmt.Errorf("config not available")
	}

	cloned := cfg.Clone()
	if cloned.Components == nil {
		cloned.Components = make(config.ComponentConfigs)
	}

	// Get existing component config or create a default
	cc, exists := cloned.Components[websocketInputComponentName]
	if !exists {
		cc = types.ComponentConfig{
			Config: json.RawMessage(`{
				"mode": "client",
				"client": {
					"url": "ws://localhost:9090/ws",
					"reconnect": {"enabled": true, "initial_delay": "1s", "max_delay": "30s", "multiplier": 2.0}
				},
				"ports": {
					"outputs": [{"name": "ws_data", "subject": "graph.ingest.entity", "type": "jetstream", "stream_name": "GRAPH"}]
				}
			}`),
		}
	}

	// Apply enabled change
	if update.Enabled != nil {
		cc.Enabled = *update.Enabled
	}

	// Apply URL change
	if update.URL != nil {
		url := *update.URL
		if url == "" {
			return fmt.Errorf("websocket URL cannot be empty")
		}
		rawConfig, err := updateWebsocketURL(cc.Config, url)
		if err != nil {
			return fmt.Errorf("failed to update websocket URL: %w", err)
		}
		cc.Config = rawConfig
	}

	cloned.Components[websocketInputComponentName] = cc

	// Update in-memory
	if err := safeCfg.Update(cloned); err != nil {
		return fmt.Errorf("failed to update config: %w", err)
	}

	// Push to KV — component manager watches this and will start/stop/reconfigure
	pushCtx, pushCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pushCancel()
	if err := s.componentDeps.Manager.PushToKV(pushCtx); err != nil {
		s.logger.Warn("failed to push config to KV", "error", err)
	}

	// Persist to disk
	if configPath := os.Getenv("SEMDRAGONS_CONFIG"); configPath != "" {
		if err := cloned.SaveToFile(configPath); err != nil {
			s.logger.Warn("failed to save config to disk", "error", err)
		}
	}

	return nil
}

// updateWebsocketURL replaces the client URL in a websocket component config JSON.
func updateWebsocketURL(rawConfig json.RawMessage, newURL string) (json.RawMessage, error) {
	// Parse the full config, update client.url, re-marshal
	var wsConfig map[string]any
	if len(rawConfig) > 0 {
		if err := json.Unmarshal(rawConfig, &wsConfig); err != nil {
			return nil, err
		}
	}
	if wsConfig == nil {
		wsConfig = make(map[string]any)
	}

	client, ok := wsConfig["client"].(map[string]any)
	if !ok {
		client = make(map[string]any)
	}
	client["url"] = newURL
	wsConfig["client"] = client

	return json.Marshal(wsConfig)
}
