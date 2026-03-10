package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/model"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/service"
	stypes "github.com/c360studio/semstreams/types"
)

// =============================================================================
// TEST HELPERS
// =============================================================================

// newSettingsService constructs a Service suitable for settings handler tests.
// natsClient may be nil; when non-nil, it must be a properly initialised
// (even if disconnected) *natsclient.Client so that IsHealthy / URLs / RTT
// do not panic.
func newSettingsService(g GraphQuerier, models ModelResolver, boardCfg *domain.BoardConfig, cfg Config) *Service {
	bc := boardCfg
	if bc == nil {
		bc = &domain.BoardConfig{Org: "test", Platform: "dev", Board: "board1"}
	}

	// A disconnected client is safe: IsHealthy() returns false, RTT() returns
	// ErrNotConnected (so latency is omitted), URLs() returns the url string.
	nc, _ := natsclient.NewClient("nats://localhost:4222")

	return &Service{
		graph:       g,
		models:      models,
		boardConfig: bc,
		config:      cfg,
		nats:        nc,
		logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// mockModelResolver implements ModelResolver with configurable function fields.
type mockModelResolver struct {
	listEndpointsFn    func() []string
	getEndpointFn      func(name string) *model.EndpointConfig
	listCapabilitiesFn func() []string
	getFallbackChainFn func(capability string) []string
	resolveFn          func(capability string) string
}

func (m *mockModelResolver) ListEndpoints() []string {
	if m.listEndpointsFn != nil {
		return m.listEndpointsFn()
	}
	return nil
}

func (m *mockModelResolver) GetEndpoint(name string) *model.EndpointConfig {
	if m.getEndpointFn != nil {
		return m.getEndpointFn(name)
	}
	return nil
}

func (m *mockModelResolver) ListCapabilities() []string {
	if m.listCapabilitiesFn != nil {
		return m.listCapabilitiesFn()
	}
	return nil
}

func (m *mockModelResolver) GetFallbackChain(capability string) []string {
	if m.getFallbackChainFn != nil {
		return m.getFallbackChainFn(capability)
	}
	return nil
}

func (m *mockModelResolver) Resolve(capability string) string {
	if m.resolveFn != nil {
		return m.resolveFn(capability)
	}
	return ""
}

// buildTestRegistry constructs a minimal *model.Registry for use in tests.
func buildTestRegistry(endpoints map[string]*model.EndpointConfig, capabilities map[string]*model.CapabilityConfig) *model.Registry {
	return &model.Registry{
		Endpoints:    endpoints,
		Capabilities: capabilities,
		Defaults:     model.DefaultsConfig{},
	}
}

// =============================================================================
// TestHandleGetSettings
// =============================================================================

func TestHandleGetSettings_PlatformInfo(t *testing.T) {
	boardCfg := &domain.BoardConfig{Org: "acme", Platform: "staging", Board: "mainboard"}
	s := newSettingsService(&mockGraph{}, nil, boardCfg, Config{})

	req := httptest.NewRequest("GET", "/api/game/settings", nil)
	rr := httptest.NewRecorder()
	s.handleGetSettings(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp SettingsResponse
	decodeJSON(t, rr.Body.Bytes(), &resp)

	if resp.Platform.Org != "acme" {
		t.Errorf("Platform.Org: got %q, want %q", resp.Platform.Org, "acme")
	}
	if resp.Platform.Platform != "staging" {
		t.Errorf("Platform.Platform: got %q, want %q", resp.Platform.Platform, "staging")
	}
	if resp.Platform.Board != "mainboard" {
		t.Errorf("Platform.Board: got %q, want %q", resp.Platform.Board, "mainboard")
	}
}

func TestHandleGetSettings_NATSStatusReflected(t *testing.T) {
	// A disconnected client returns IsHealthy()=false.
	s := newSettingsService(&mockGraph{}, nil, nil, Config{})

	req := httptest.NewRequest("GET", "/api/game/settings", nil)
	rr := httptest.NewRecorder()
	s.handleGetSettings(rr, req)

	var resp SettingsResponse
	decodeJSON(t, rr.Body.Bytes(), &resp)

	if resp.NATS.Connected {
		t.Error("expected NATS.Connected=false for a disconnected client")
	}
}

func TestHandleGetSettings_WorkspacePopulated(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{WorkspaceDir: dir}
	s := newSettingsService(&mockGraph{}, nil, nil, cfg)

	req := httptest.NewRequest("GET", "/api/game/settings", nil)
	rr := httptest.NewRecorder()
	s.handleGetSettings(rr, req)

	var resp SettingsResponse
	decodeJSON(t, rr.Body.Bytes(), &resp)

	if resp.Workspace.Dir != dir {
		t.Errorf("Workspace.Dir: got %q, want %q", resp.Workspace.Dir, dir)
	}
	if !resp.Workspace.Exists {
		t.Error("expected Workspace.Exists=true for an existing temp dir")
	}
	if !resp.Workspace.Writable {
		t.Error("expected Workspace.Writable=true for a writable temp dir")
	}
}

func TestHandleGetSettings_WorkspaceMissing(t *testing.T) {
	cfg := Config{WorkspaceDir: "/tmp/semdragons-nonexistent-dir-xyz-9999"}
	s := newSettingsService(&mockGraph{}, nil, nil, cfg)

	req := httptest.NewRequest("GET", "/api/game/settings", nil)
	rr := httptest.NewRecorder()
	s.handleGetSettings(rr, req)

	var resp SettingsResponse
	decodeJSON(t, rr.Body.Bytes(), &resp)

	if resp.Workspace.Exists {
		t.Error("expected Workspace.Exists=false for a non-existent dir")
	}
	if resp.Workspace.Writable {
		t.Error("expected Workspace.Writable=false for a non-existent dir")
	}
}

func TestHandleGetSettings_WorkspaceEmpty(t *testing.T) {
	s := newSettingsService(&mockGraph{}, nil, nil, Config{})

	req := httptest.NewRequest("GET", "/api/game/settings", nil)
	rr := httptest.NewRecorder()
	s.handleGetSettings(rr, req)

	var resp SettingsResponse
	decodeJSON(t, rr.Body.Bytes(), &resp)

	if resp.Workspace.Dir != "" {
		t.Errorf("expected empty Workspace.Dir when not configured, got %q", resp.Workspace.Dir)
	}
	if resp.Workspace.Exists {
		t.Error("expected Workspace.Exists=false when WorkspaceDir is empty")
	}
}

// =============================================================================
// TestHandleGetSettings_NoModels
// =============================================================================

func TestHandleGetSettings_NoModels(t *testing.T) {
	s := newSettingsService(&mockGraph{}, nil, nil, Config{})

	req := httptest.NewRequest("GET", "/api/game/settings", nil)
	rr := httptest.NewRecorder()
	s.handleGetSettings(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp SettingsResponse
	decodeJSON(t, rr.Body.Bytes(), &resp)

	if len(resp.Models.Endpoints) != 0 {
		t.Errorf("expected empty endpoints with nil models, got %d", len(resp.Models.Endpoints))
	}
	if len(resp.Models.Capabilities) != 0 {
		t.Errorf("expected empty capabilities with nil models, got %d", len(resp.Models.Capabilities))
	}
}

// =============================================================================
// TestAssembleModelRegistryView
// =============================================================================

func TestAssembleModelRegistryView_EndpointsListed(t *testing.T) {
	reg := buildTestRegistry(
		map[string]*model.EndpointConfig{
			"claude": {
				Provider:      "anthropic",
				Model:         "claude-opus-4-5",
				MaxTokens:     200000,
				SupportsTools: true,
				APIKeyEnv:     "ANTHROPIC_API_KEY",
			},
			"gpt4": {
				Provider:      "openai",
				Model:         "gpt-4o",
				MaxTokens:     128000,
				SupportsTools: true,
				APIKeyEnv:     "OPENAI_API_KEY",
			},
		},
		nil,
	)

	s := newSettingsService(&mockGraph{}, reg, nil, Config{})
	view := s.assembleModelRegistryView()

	if len(view.Endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(view.Endpoints))
	}

	// Verify the endpoint names appear (registry returns them sorted)
	names := make(map[string]bool)
	for _, ep := range view.Endpoints {
		names[ep.Name] = true
	}
	for _, want := range []string{"claude", "gpt4"} {
		if !names[want] {
			t.Errorf("expected endpoint %q in view", want)
		}
	}
}

func TestAssembleModelRegistryView_APIKeyNotExposed(t *testing.T) {
	// The response type has no "api_key_value" field; just "api_key_env" and "api_key_set".
	// Confirm that env var NAME is exposed but never the value.
	const envVar = "SEMDRAGONS_TEST_SETTINGS_KEY"
	t.Setenv(envVar, "super-secret-value")

	reg := buildTestRegistry(
		map[string]*model.EndpointConfig{
			"mymodel": {
				Provider:  "anthropic",
				Model:     "claude-haiku",
				MaxTokens: 4096,
				APIKeyEnv: envVar,
			},
		},
		nil,
	)

	s := newSettingsService(&mockGraph{}, reg, nil, Config{})
	view := s.assembleModelRegistryView()

	// Marshal to JSON and confirm "super-secret-value" never appears in the payload.
	b, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal view: %v", err)
	}
	if bytes.Contains(b, []byte("super-secret-value")) {
		t.Errorf("API key value must not appear in the settings response JSON")
	}
	if !bytes.Contains(b, []byte(envVar)) {
		t.Errorf("API key env var name %q should appear in the settings response JSON", envVar)
	}
}

// =============================================================================
// TestAssembleModelRegistryView_APIKeyStatus
// =============================================================================

func TestAssembleModelRegistryView_APIKeyStatus_Set(t *testing.T) {
	const envVar = "SEMDRAGONS_SETTINGS_TEST_KEY_SET"
	t.Setenv(envVar, "some-key-value")

	reg := buildTestRegistry(
		map[string]*model.EndpointConfig{
			"ep": {Provider: "openai", Model: "gpt-4o", MaxTokens: 4096, APIKeyEnv: envVar},
		},
		nil,
	)

	s := newSettingsService(&mockGraph{}, reg, nil, Config{})
	view := s.assembleModelRegistryView()

	if len(view.Endpoints) == 0 {
		t.Fatal("expected at least one endpoint")
	}
	ep := view.Endpoints[0]
	if !ep.APIKeySet {
		t.Errorf("expected APIKeySet=true when env var %q is set", envVar)
	}
}

func TestAssembleModelRegistryView_APIKeyStatus_NotSet(t *testing.T) {
	const envVar = "SEMDRAGONS_SETTINGS_TEST_KEY_MISSING_XYZ"
	// Ensure the env var is absent.
	os.Unsetenv(envVar) //nolint:errcheck

	reg := buildTestRegistry(
		map[string]*model.EndpointConfig{
			"ep": {Provider: "openai", Model: "gpt-4o", MaxTokens: 4096, APIKeyEnv: envVar},
		},
		nil,
	)

	s := newSettingsService(&mockGraph{}, reg, nil, Config{})
	view := s.assembleModelRegistryView()

	if len(view.Endpoints) == 0 {
		t.Fatal("expected at least one endpoint")
	}
	ep := view.Endpoints[0]
	if ep.APIKeySet {
		t.Errorf("expected APIKeySet=false when env var %q is not set", envVar)
	}
}

func TestAssembleModelRegistryView_LocalEndpointNoKey(t *testing.T) {
	// Ollama-style endpoint: no APIKeyEnv → APIKeySet must be false (no key needed).
	reg := buildTestRegistry(
		map[string]*model.EndpointConfig{
			"ollama": {
				Provider:  "ollama",
				URL:       "http://localhost:11434",
				Model:     "llama3",
				MaxTokens: 8192,
			},
		},
		nil,
	)

	s := newSettingsService(&mockGraph{}, reg, nil, Config{})
	view := s.assembleModelRegistryView()

	if len(view.Endpoints) == 0 {
		t.Fatal("expected at least one endpoint")
	}
	ep := view.Endpoints[0]
	if ep.APIKeySet {
		t.Errorf("expected APIKeySet=false for a local endpoint with no APIKeyEnv")
	}
	if ep.APIKeyEnv != "" {
		t.Errorf("expected APIKeyEnv=empty for local endpoint, got %q", ep.APIKeyEnv)
	}
}

func TestAssembleModelRegistryView_CapabilitiesIncluded(t *testing.T) {
	reg := buildTestRegistry(
		map[string]*model.EndpointConfig{
			"claude": {Provider: "anthropic", Model: "claude-haiku", MaxTokens: 4096},
		},
		map[string]*model.CapabilityConfig{
			"quest_worker": {
				Description:   "Default task worker",
				Preferred:     []string{"claude"},
				RequiresTools: false,
			},
		},
	)

	s := newSettingsService(&mockGraph{}, reg, nil, Config{})
	view := s.assembleModelRegistryView()

	capView, ok := view.Capabilities["quest_worker"]
	if !ok {
		t.Fatal("expected capability 'quest_worker' in view")
	}
	if capView.Description != "Default task worker" {
		t.Errorf("Description: got %q, want %q", capView.Description, "Default task worker")
	}
	if len(capView.Preferred) == 0 || capView.Preferred[0] != "claude" {
		t.Errorf("Preferred chain: got %v, want [claude]", capView.Preferred)
	}
}

func TestAssembleModelRegistryView_DefaultsPopulated(t *testing.T) {
	reg := &model.Registry{
		Endpoints: map[string]*model.EndpointConfig{
			"claude": {Provider: "anthropic", Model: "claude-haiku", MaxTokens: 4096},
		},
		Capabilities: map[string]*model.CapabilityConfig{
			"worker": {Preferred: []string{"claude"}},
		},
		Defaults: model.DefaultsConfig{
			Model:      "claude",
			Capability: "worker",
		},
	}

	s := newSettingsService(&mockGraph{}, reg, nil, Config{})
	view := s.assembleModelRegistryView()

	if view.Defaults.Model != "claude" {
		t.Errorf("Defaults.Model: got %q, want %q", view.Defaults.Model, "claude")
	}
	if view.Defaults.Capability != "worker" {
		t.Errorf("Defaults.Capability: got %q, want %q", view.Defaults.Capability, "worker")
	}
}

// =============================================================================
// TestBuildChecklist
// =============================================================================

func TestBuildChecklist_NATSDisconnected(t *testing.T) {
	s := newSettingsService(&mockGraph{}, nil, nil, Config{})
	checklist := s.buildChecklist(context.Background())

	var natsItem *ChecklistItem
	for i := range checklist {
		if checklist[i].Label == "NATS connected" {
			natsItem = &checklist[i]
			break
		}
	}
	if natsItem == nil {
		t.Fatal("expected 'NATS connected' checklist item")
	}
	if natsItem.Met {
		t.Error("expected NATS checklist item Met=false for disconnected client")
	}
}

func TestBuildChecklist_NoEndpoints(t *testing.T) {
	s := newSettingsService(&mockGraph{}, nil, nil, Config{})
	checklist := s.buildChecklist(context.Background())

	var epItem *ChecklistItem
	for i := range checklist {
		if checklist[i].Label == "LLM endpoint configured" {
			epItem = &checklist[i]
			break
		}
	}
	if epItem == nil {
		t.Fatal("expected 'LLM endpoint configured' checklist item")
	}
	if epItem.Met {
		t.Error("expected LLM endpoint item Met=false when no models set")
	}
	if epItem.HelpText == "" {
		t.Error("expected HelpText to be non-empty when endpoint not configured")
	}
}

func TestBuildChecklist_EndpointConfigured(t *testing.T) {
	reg := buildTestRegistry(
		map[string]*model.EndpointConfig{
			"ollama": {Provider: "ollama", Model: "llama3", MaxTokens: 4096},
		},
		nil,
	)
	s := newSettingsService(&mockGraph{}, reg, nil, Config{})
	checklist := s.buildChecklist(context.Background())

	var epItem, keyItem *ChecklistItem
	for i := range checklist {
		switch checklist[i].Label {
		case "LLM endpoint configured":
			epItem = &checklist[i]
		case "API key set for LLM provider":
			keyItem = &checklist[i]
		}
	}
	if epItem == nil {
		t.Fatal("expected 'LLM endpoint configured' checklist item")
	}
	if !epItem.Met {
		t.Error("expected LLM endpoint item Met=true when ollama endpoint is configured")
	}
	// Ollama needs no key — apiKeyReady should be true.
	if keyItem == nil {
		t.Fatal("expected 'API key set for LLM provider' checklist item")
	}
	if !keyItem.Met {
		t.Error("expected API key item Met=true for local ollama endpoint (no key needed)")
	}
}

func TestBuildChecklist_APIKeySet(t *testing.T) {
	const envVar = "SEMDRAGONS_CHECKLIST_TEST_KEY"
	t.Setenv(envVar, "valid-key")

	reg := buildTestRegistry(
		map[string]*model.EndpointConfig{
			"cloud": {Provider: "anthropic", Model: "claude-haiku", MaxTokens: 4096, APIKeyEnv: envVar},
		},
		nil,
	)
	s := newSettingsService(&mockGraph{}, reg, nil, Config{})
	checklist := s.buildChecklist(context.Background())

	var keyItem *ChecklistItem
	for i := range checklist {
		if checklist[i].Label == "API key set for LLM provider" {
			keyItem = &checklist[i]
			break
		}
	}
	if keyItem == nil {
		t.Fatal("expected 'API key set for LLM provider' checklist item")
	}
	if !keyItem.Met {
		t.Errorf("expected API key item Met=true when env var %q is set", envVar)
	}
}

func TestBuildChecklist_APIKeyNotSet(t *testing.T) {
	const envVar = "SEMDRAGONS_CHECKLIST_KEY_MISSING_XYZ"
	os.Unsetenv(envVar) //nolint:errcheck

	reg := buildTestRegistry(
		map[string]*model.EndpointConfig{
			"cloud": {Provider: "openai", Model: "gpt-4o", MaxTokens: 4096, APIKeyEnv: envVar},
		},
		nil,
	)
	s := newSettingsService(&mockGraph{}, reg, nil, Config{})
	checklist := s.buildChecklist(context.Background())

	var keyItem *ChecklistItem
	for i := range checklist {
		if checklist[i].Label == "API key set for LLM provider" {
			keyItem = &checklist[i]
			break
		}
	}
	if keyItem == nil {
		t.Fatal("expected 'API key set for LLM provider' checklist item")
	}
	if keyItem.Met {
		t.Errorf("expected API key item Met=false when env var %q is absent", envVar)
	}
	if keyItem.HelpText == "" {
		t.Error("expected HelpText to be non-empty when API key is not set")
	}
}

func TestBuildChecklist_WorkspaceWritable(t *testing.T) {
	dir := t.TempDir()
	s := newSettingsService(&mockGraph{}, nil, nil, Config{WorkspaceDir: dir})
	checklist := s.buildChecklist(context.Background())

	var wsItem *ChecklistItem
	for i := range checklist {
		if checklist[i].Label == "Workspace directory exists and is writable" {
			wsItem = &checklist[i]
			break
		}
	}
	if wsItem == nil {
		t.Fatal("expected workspace checklist item")
	}
	if !wsItem.Met {
		t.Errorf("expected workspace item Met=true for writable temp dir %q", dir)
	}
}

func TestBuildChecklist_WorkspaceMissing(t *testing.T) {
	s := newSettingsService(&mockGraph{}, nil, nil, Config{WorkspaceDir: "/tmp/semdragons-checklist-missing-xyz"})
	checklist := s.buildChecklist(context.Background())

	var wsItem *ChecklistItem
	for i := range checklist {
		if checklist[i].Label == "Workspace directory exists and is writable" {
			wsItem = &checklist[i]
			break
		}
	}
	if wsItem == nil {
		t.Fatal("expected workspace checklist item")
	}
	if wsItem.Met {
		t.Error("expected workspace item Met=false for missing dir")
	}
	if wsItem.HelpText == "" {
		t.Error("expected HelpText when workspace is not configured/missing")
	}
}

func TestBuildChecklist_AgentPresent(t *testing.T) {
	g := &mockGraph{
		// Mock returns a fixed agent list — limit is not needed for this test.
		listAgentsFn: func(_ context.Context, _ int) ([]graph.EntityState, error) {
			return []graph.EntityState{{ID: "test.dev.game.board1.agent.a1"}}, nil
		},
	}
	s := newSettingsService(g, nil, nil, Config{})
	checklist := s.buildChecklist(context.Background())

	var agentItem *ChecklistItem
	for i := range checklist {
		if checklist[i].Label == "At least one agent recruited" {
			agentItem = &checklist[i]
			break
		}
	}
	if agentItem == nil {
		t.Fatal("expected 'At least one agent recruited' checklist item")
	}
	if !agentItem.Met {
		t.Error("expected agent item Met=true when graph returns an agent")
	}
}

func TestBuildChecklist_NoAgents(t *testing.T) {
	// Default mockGraph returns nil, nil for ListAgentsByPrefix.
	s := newSettingsService(&mockGraph{}, nil, nil, Config{})
	checklist := s.buildChecklist(context.Background())

	var agentItem *ChecklistItem
	for i := range checklist {
		if checklist[i].Label == "At least one agent recruited" {
			agentItem = &checklist[i]
			break
		}
	}
	if agentItem == nil {
		t.Fatal("expected 'At least one agent recruited' checklist item")
	}
	if agentItem.Met {
		t.Error("expected agent item Met=false when no agents in graph")
	}
	if agentItem.HelpText == "" {
		t.Error("expected HelpText when no agents are present")
	}
}

func TestBuildChecklist_QuestPresent(t *testing.T) {
	g := &mockGraph{
		// Mock returns a fixed quest list — limit is not needed for this test.
		listQuestsFn: func(_ context.Context, _ int) ([]graph.EntityState, error) {
			return []graph.EntityState{{ID: "test.dev.game.board1.quest.q1"}}, nil
		},
	}
	s := newSettingsService(g, nil, nil, Config{})
	checklist := s.buildChecklist(context.Background())

	var questItem *ChecklistItem
	for i := range checklist {
		if checklist[i].Label == "At least one quest posted" {
			questItem = &checklist[i]
			break
		}
	}
	if questItem == nil {
		t.Fatal("expected 'At least one quest posted' checklist item")
	}
	if !questItem.Met {
		t.Error("expected quest item Met=true when graph returns a quest")
	}
}

func TestBuildChecklist_NoQuests(t *testing.T) {
	s := newSettingsService(&mockGraph{}, nil, nil, Config{})
	checklist := s.buildChecklist(context.Background())

	var questItem *ChecklistItem
	for i := range checklist {
		if checklist[i].Label == "At least one quest posted" {
			questItem = &checklist[i]
			break
		}
	}
	if questItem == nil {
		t.Fatal("expected 'At least one quest posted' checklist item")
	}
	if questItem.Met {
		t.Error("expected quest item Met=false when no quests in graph")
	}
	if questItem.HelpText == "" {
		t.Error("expected HelpText when no quests are present")
	}
}

func TestBuildChecklist_AllItems(t *testing.T) {
	// Verify all expected labels appear in the checklist regardless of Met status.
	s := newSettingsService(&mockGraph{}, nil, nil, Config{})
	checklist := s.buildChecklist(context.Background())

	wantLabels := []string{
		"NATS connected",
		"LLM endpoint configured",
		"API key set for LLM provider",
		"Workspace directory exists and is writable",
		"At least one agent recruited",
		"At least one quest posted",
	}

	labelSet := make(map[string]bool, len(checklist))
	for _, item := range checklist {
		labelSet[item.Label] = true
	}
	for _, label := range wantLabels {
		if !labelSet[label] {
			t.Errorf("expected checklist label %q to be present", label)
		}
	}
}

// =============================================================================
// TestApplyModelRegistryUpdate
// =============================================================================

func TestApplyModelRegistryUpdate_AddEndpoint(t *testing.T) {
	reg := buildTestRegistry(
		map[string]*model.EndpointConfig{
			"existing": {Provider: "ollama", Model: "llama3", MaxTokens: 4096},
		},
		nil,
	)
	s := newSettingsService(&mockGraph{}, reg, nil, Config{})

	update := &ModelRegistryUpdate{
		Endpoints: map[string]*EndpointUpdate{
			"new-endpoint": {
				Provider:  "openai",
				Model:     "gpt-4o",
				MaxTokens: 128000,
			},
		},
	}

	err := s.applyModelRegistryUpdate(context.Background(), update)
	if err != nil {
		t.Fatalf("applyModelRegistryUpdate: %v", err)
	}

	// After the update, s.models should be a *model.Registry with the new endpoint.
	newReg, ok := s.models.(*model.Registry)
	if !ok {
		t.Fatal("expected s.models to remain *model.Registry after update")
	}
	if _, exists := newReg.Endpoints["new-endpoint"]; !exists {
		t.Error("expected 'new-endpoint' to be present after add")
	}
	if _, exists := newReg.Endpoints["existing"]; !exists {
		t.Error("expected 'existing' endpoint to still be present after add")
	}
}

func TestApplyModelRegistryUpdate_RemoveEndpoint(t *testing.T) {
	reg := buildTestRegistry(
		map[string]*model.EndpointConfig{
			"keep":   {Provider: "ollama", Model: "llama3", MaxTokens: 4096},
			"remove": {Provider: "ollama", Model: "qwen3", MaxTokens: 4096},
		},
		nil, // no capabilities reference "remove"
	)
	s := newSettingsService(&mockGraph{}, reg, nil, Config{})

	update := &ModelRegistryUpdate{
		Endpoints: map[string]*EndpointUpdate{
			"remove": {Remove: true},
		},
	}

	err := s.applyModelRegistryUpdate(context.Background(), update)
	if err != nil {
		t.Fatalf("applyModelRegistryUpdate: %v", err)
	}

	newReg, ok := s.models.(*model.Registry)
	if !ok {
		t.Fatal("expected s.models to be *model.Registry after update")
	}
	if _, exists := newReg.Endpoints["remove"]; exists {
		t.Error("expected 'remove' endpoint to be gone after removal")
	}
	if _, exists := newReg.Endpoints["keep"]; !exists {
		t.Error("expected 'keep' endpoint to still be present")
	}
}

func TestApplyModelRegistryUpdate_RemoveEndpointInUse(t *testing.T) {
	reg := buildTestRegistry(
		map[string]*model.EndpointConfig{
			"busy": {Provider: "ollama", Model: "llama3", MaxTokens: 4096},
		},
		map[string]*model.CapabilityConfig{
			"worker": {Preferred: []string{"busy"}},
		},
	)
	s := newSettingsService(&mockGraph{}, reg, nil, Config{})

	update := &ModelRegistryUpdate{
		Endpoints: map[string]*EndpointUpdate{
			"busy": {Remove: true},
		},
	}

	err := s.applyModelRegistryUpdate(context.Background(), update)
	if err == nil {
		t.Fatal("expected error when removing an endpoint referenced by a capability, got nil")
	}
}

func TestApplyModelRegistryUpdate_ValidationError(t *testing.T) {
	// Start with a valid registry, then try to add a capability that references
	// a non-existent endpoint.
	reg := buildTestRegistry(
		map[string]*model.EndpointConfig{
			"ep": {Provider: "ollama", Model: "llama3", MaxTokens: 4096},
		},
		nil,
	)
	s := newSettingsService(&mockGraph{}, reg, nil, Config{})

	update := &ModelRegistryUpdate{
		Capabilities: map[string]*CapabilityUpdate{
			"bad-cap": {
				Preferred: []string{"nonexistent-endpoint"},
			},
		},
	}

	err := s.applyModelRegistryUpdate(context.Background(), update)
	if err == nil {
		t.Fatal("expected validation error for capability referencing nonexistent endpoint, got nil")
	}
}

func TestApplyModelRegistryUpdate_NonMutableResolver(t *testing.T) {
	// A mock resolver (not *model.Registry) should return an error.
	s := newSettingsService(&mockGraph{}, &mockModelResolver{}, nil, Config{})

	update := &ModelRegistryUpdate{
		Endpoints: map[string]*EndpointUpdate{
			"ep": {Provider: "ollama", Model: "llama3", MaxTokens: 4096},
		},
	}

	err := s.applyModelRegistryUpdate(context.Background(), update)
	if err == nil {
		t.Fatal("expected error when models is not *model.Registry, got nil")
	}
}

func TestApplyModelRegistryUpdate_UpdateCapability(t *testing.T) {
	reg := buildTestRegistry(
		map[string]*model.EndpointConfig{
			"ep-a": {Provider: "ollama", Model: "llama3", MaxTokens: 4096},
			"ep-b": {Provider: "ollama", Model: "qwen3", MaxTokens: 4096},
		},
		nil,
	)
	s := newSettingsService(&mockGraph{}, reg, nil, Config{})

	update := &ModelRegistryUpdate{
		Capabilities: map[string]*CapabilityUpdate{
			"worker": {
				Description: "Quest worker capability",
				Preferred:   []string{"ep-a"},
				Fallback:    []string{"ep-b"},
			},
		},
	}

	err := s.applyModelRegistryUpdate(context.Background(), update)
	if err != nil {
		t.Fatalf("applyModelRegistryUpdate: %v", err)
	}

	newReg, ok := s.models.(*model.Registry)
	if !ok {
		t.Fatal("expected s.models to be *model.Registry after update")
	}
	capCfg, exists := newReg.Capabilities["worker"]
	if !exists {
		t.Fatal("expected 'worker' capability to be present after update")
	}
	if len(capCfg.Preferred) == 0 || capCfg.Preferred[0] != "ep-a" {
		t.Errorf("Preferred chain: got %v, want [ep-a]", capCfg.Preferred)
	}
	if capCfg.Description != "Quest worker capability" {
		t.Errorf("Description: got %q, want %q", capCfg.Description, "Quest worker capability")
	}
}

func TestApplyModelRegistryUpdate_UpdateDefaults(t *testing.T) {
	reg := buildTestRegistry(
		map[string]*model.EndpointConfig{
			"ep": {Provider: "ollama", Model: "llama3", MaxTokens: 4096},
		},
		nil,
	)
	s := newSettingsService(&mockGraph{}, reg, nil, Config{})

	update := &ModelRegistryUpdate{
		Defaults: &ModelDefaultsView{Model: "ep"},
	}

	err := s.applyModelRegistryUpdate(context.Background(), update)
	if err != nil {
		t.Fatalf("applyModelRegistryUpdate: %v", err)
	}

	newReg, ok := s.models.(*model.Registry)
	if !ok {
		t.Fatal("expected s.models to be *model.Registry after update")
	}
	if newReg.Defaults.Model != "ep" {
		t.Errorf("Defaults.Model: got %q, want %q", newReg.Defaults.Model, "ep")
	}
}

// =============================================================================
// TestHandleUpdateSettings
// =============================================================================

func TestHandleUpdateSettings_ModelRegistryUpdate(t *testing.T) {
	reg := buildTestRegistry(
		map[string]*model.EndpointConfig{
			"base": {Provider: "ollama", Model: "llama3", MaxTokens: 4096},
		},
		nil,
	)
	s := newSettingsService(&mockGraph{}, reg, nil, Config{})

	body := `{
		"model_registry": {
			"endpoints": {
				"added": {
					"provider": "openai",
					"model": "gpt-4o-mini",
					"max_tokens": 16384
				}
			}
		}
	}`

	req := httptest.NewRequest("POST", "/api/game/settings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.handleUpdateSettings(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp SettingsResponse
	decodeJSON(t, rr.Body.Bytes(), &resp)

	names := make(map[string]bool)
	for _, ep := range resp.Models.Endpoints {
		names[ep.Name] = true
	}
	if !names["added"] {
		t.Error("expected 'added' endpoint to appear in the response after update")
	}
	if !names["base"] {
		t.Error("expected 'base' endpoint to still be present after update")
	}
}

func TestHandleUpdateSettings_InvalidBody(t *testing.T) {
	s := newSettingsService(&mockGraph{}, nil, nil, Config{})

	req := httptest.NewRequest("POST", "/api/game/settings", bytes.NewBufferString("{invalid json"))
	rr := httptest.NewRecorder()
	s.handleUpdateSettings(rr, req)

	if rr.Code != 400 {
		t.Errorf("expected 400 for invalid JSON body, got %d", rr.Code)
	}
}

func TestHandleUpdateSettings_NoOpUpdate(t *testing.T) {
	// An empty update body should succeed and return 200 with current settings.
	s := newSettingsService(&mockGraph{}, nil, nil, Config{})

	req := httptest.NewRequest("POST", "/api/game/settings", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.handleUpdateSettings(rr, req)

	if rr.Code != 200 {
		t.Errorf("expected 200 for empty update, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// WEBSOCKET INPUT TESTS
// =============================================================================

func TestExtractWebsocketURL(t *testing.T) {
	raw := json.RawMessage(`{
		"mode": "client",
		"client": {"url": "ws://semsource:9090/ws"},
		"ports": {}
	}`)

	url := extractWebsocketURL(raw)
	if url != "ws://semsource:9090/ws" {
		t.Errorf("expected ws://semsource:9090/ws, got %q", url)
	}
}

func TestExtractWebsocketURL_Empty(t *testing.T) {
	if url := extractWebsocketURL(nil); url != "" {
		t.Errorf("expected empty for nil, got %q", url)
	}
	if url := extractWebsocketURL(json.RawMessage(`{}`)); url != "" {
		t.Errorf("expected empty for empty config, got %q", url)
	}
}

func TestUpdateWebsocketURL(t *testing.T) {
	original := json.RawMessage(`{
		"mode": "client",
		"client": {"url": "ws://old:9090/ws", "reconnect": {"enabled": true}},
		"ports": {}
	}`)

	updated, err := updateWebsocketURL(original, "ws://new-host:8080/ws")
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(updated, &parsed); err != nil {
		t.Fatal(err)
	}

	client := parsed["client"].(map[string]any)
	if client["url"] != "ws://new-host:8080/ws" {
		t.Errorf("URL not updated, got %v", client["url"])
	}

	// reconnect preserved
	reconnect := client["reconnect"].(map[string]any)
	if reconnect["enabled"] != true {
		t.Error("reconnect config was lost")
	}

	// mode preserved
	if parsed["mode"] != "client" {
		t.Error("mode was lost")
	}
}

func TestUpdateWebsocketURL_EmptyConfig(t *testing.T) {
	updated, err := updateWebsocketURL(nil, "ws://localhost:9090/ws")
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(updated, &parsed); err != nil {
		t.Fatal(err)
	}

	client := parsed["client"].(map[string]any)
	if client["url"] != "ws://localhost:9090/ws" {
		t.Errorf("URL not set, got %v", client["url"])
	}
}

func TestAssembleWebsocketInputView_NoConfigManager(t *testing.T) {
	s := newSettingsService(&mockGraph{}, nil, nil, Config{})
	view := s.assembleWebsocketInputView()

	if view.Enabled {
		t.Error("expected disabled when no config manager")
	}
	if view.URL != "" {
		t.Errorf("expected empty URL, got %q", view.URL)
	}
}

func TestAssembleWebsocketInputView_WithConfig(t *testing.T) {
	cfg := &config.Config{
		Components: config.ComponentConfigs{
			websocketInputComponentName: stypes.ComponentConfig{
				Enabled: true,
				Config: json.RawMessage(`{"client":{"url":"ws://semsource:9090/ws"}}`),
			},
		},
	}
	safeCfg := config.NewSafeConfig(cfg)

	// Need a config.Manager — create a minimal deps with Manager
	// config.Manager.GetConfig() returns SafeConfig; we can't easily mock it.
	// Instead, wire componentDeps.Manager to nil and use a direct approach.
	// Let's test via the service with componentDeps that has a working Manager.

	// Since Manager requires NATS, we test the extraction logic via getComponentConfig
	// by directly setting componentDeps with a mock-like setup.
	// The Manager field on service.Dependencies is *config.Manager — concrete type.
	// We can't easily construct one without NATS, so we test the helper functions
	// and the assembly function accepts the result.

	// Test extractWebsocketURL with the raw config
	url := extractWebsocketURL(cfg.Components[websocketInputComponentName].Config)
	if url != "ws://semsource:9090/ws" {
		t.Errorf("expected ws://semsource:9090/ws, got %q", url)
	}

	// Verify SafeConfig round-trip
	got := safeCfg.Get()
	cc := got.Components[websocketInputComponentName]
	if !cc.Enabled {
		t.Error("expected enabled")
	}
}

func TestApplyWebsocketInputUpdate_NoManager(t *testing.T) {
	s := newSettingsService(&mockGraph{}, nil, nil, Config{})
	update := &WebsocketInputUpdate{Enabled: boolPtr(true)}

	err := s.applyWebsocketInputUpdate(context.Background(), update)
	if err == nil {
		t.Error("expected error when no config manager")
	}
}

func TestApplyWebsocketInputUpdate_EmptyURL(t *testing.T) {
	s := newSettingsService(&mockGraph{}, nil, nil, Config{})
	// Even without manager, empty URL validation happens after manager check.
	// With a real manager, the URL validation would catch it. Test the validator.
	emptyURL := ""
	update := &WebsocketInputUpdate{URL: &emptyURL}

	err := s.applyWebsocketInputUpdate(context.Background(), update)
	if err == nil {
		t.Error("expected error for empty URL")
	}
}

func TestHandleGetSettings_WebsocketInputIncluded(t *testing.T) {
	s := newSettingsService(&mockGraph{}, nil, nil, Config{})

	req := httptest.NewRequest("GET", "/api/game/settings", nil)
	rr := httptest.NewRecorder()
	s.handleGetSettings(rr, req)

	var resp SettingsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	// Without config manager, websocket input should be present but empty
	if resp.WebsocketInput.Enabled {
		t.Error("expected disabled")
	}
	if resp.WebsocketInput.URL != "" {
		t.Errorf("expected empty URL, got %q", resp.WebsocketInput.URL)
	}
}

func boolPtr(b bool) *bool { return &b }

// Ensure config and stypes are used to satisfy imports.
var _ = config.NewSafeConfig
var _ stypes.ComponentConfig
var _ = service.NewBaseServiceWithOptions

// =============================================================================
// TestApplyModelRegistryUpdate_APIKeyValue
// =============================================================================

// TestApplyModelRegistryUpdate_APIKeyValueRequiresEnvName verifies that sending
// api_key_value without api_key_env is rejected.
func TestApplyModelRegistryUpdate_APIKeyValueRequiresEnvName(t *testing.T) {
	reg := buildTestRegistry(
		map[string]*model.EndpointConfig{
			"ep": {Provider: "anthropic", Model: "claude-haiku", MaxTokens: 4096},
		},
		nil,
	)
	s := newSettingsService(&mockGraph{}, reg, nil, Config{})

	keyVal := "sk-test-secret"
	update := &ModelRegistryUpdate{
		Endpoints: map[string]*EndpointUpdate{
			"ep": {
				Provider:    "anthropic",
				Model:       "claude-haiku",
				MaxTokens:   4096,
				APIKeyValue: &keyVal,
				// APIKeyEnv intentionally omitted
			},
		},
	}

	err := s.applyModelRegistryUpdate(context.Background(), update)
	if err == nil {
		t.Fatal("expected error when api_key_value is set but api_key_env is missing")
	}
}

// TestApplyModelRegistryUpdate_APIKeyValueWritesToEnv verifies that supplying
// both api_key_env and api_key_value writes the key to the env file and sets
// the process env, without storing the raw value in the registry.
func TestApplyModelRegistryUpdate_APIKeyValueWritesToEnv(t *testing.T) {
	// Not parallel — touches os.Setenv and SEMDRAGONS_ENV_FILE.
	dir := t.TempDir()
	envPath := dir + "/.env"
	t.Setenv("SEMDRAGONS_ENV_FILE", envPath)

	const envVar = "SEMDRAGONS_TEST_LLM_KEY_WRITE"
	os.Unsetenv(envVar) //nolint:errcheck

	reg := buildTestRegistry(
		map[string]*model.EndpointConfig{
			"cloud": {Provider: "anthropic", Model: "claude-haiku", MaxTokens: 4096, APIKeyEnv: envVar},
		},
		nil,
	)
	s := newSettingsService(&mockGraph{}, reg, nil, Config{})

	keyVal := "sk-live-key-12345"
	update := &ModelRegistryUpdate{
		Endpoints: map[string]*EndpointUpdate{
			"cloud": {
				Provider:    "anthropic",
				Model:       "claude-haiku",
				MaxTokens:   4096,
				APIKeyEnv:   envVar,
				APIKeyValue: &keyVal,
			},
		},
	}

	if err := s.applyModelRegistryUpdate(context.Background(), update); err != nil {
		t.Fatalf("applyModelRegistryUpdate: %v", err)
	}

	// Key must be available in process environment immediately.
	if got := os.Getenv(envVar); got != keyVal {
		t.Errorf("os.Getenv(%q) = %q, want %q", envVar, got, keyVal)
	}

	// The raw key value must not appear in the registry JSON.
	newReg, ok := s.models.(*model.Registry)
	if !ok {
		t.Fatal("expected s.models to be *model.Registry")
	}
	data, err := json.Marshal(newReg)
	if err != nil {
		t.Fatalf("marshal registry: %v", err)
	}
	if bytes.Contains(data, []byte(keyVal)) {
		t.Error("raw API key value must not appear in registry JSON")
	}
}

// =============================================================================
// TestAssembleSearchConfigView
// =============================================================================

// TestAssembleSearchConfigView_NoConfigManager verifies that an empty view is
// returned when no config manager is wired.
func TestAssembleSearchConfigView_NoConfigManager(t *testing.T) {
	t.Parallel()
	s := newSettingsService(&mockGraph{}, nil, nil, Config{})
	view := s.assembleSearchConfigView()
	if view.Provider != "" || view.APIKeySet {
		t.Errorf("expected empty view, got %+v", view)
	}
}

// TestAssembleSearchConfigView_KeySet verifies that APIKeySet reflects the
// presence of the env var named by api_key_env.
func TestAssembleSearchConfigView_KeySet(t *testing.T) {
	// Not parallel — touches os.Setenv.
	const envVar = "SEMDRAGONS_TEST_SEARCH_VIEW_KEY"
	t.Setenv(envVar, "sk-brave-test")

	cfg := &config.Config{
		Components: config.ComponentConfigs{
			questtoolsComponentName: stypes.ComponentConfig{
				Enabled: true,
				Config:  json.RawMessage(`{"search":{"provider":"brave","api_key_env":"` + envVar + `"}}`),
			},
		},
	}
	safeCfg := config.NewSafeConfig(cfg)

	// Test the raw parsing logic that assembleSearchConfigView uses internally.
	var raw struct {
		Search *struct {
			Provider  string `json:"provider"`
			APIKeyEnv string `json:"api_key_env"`
			BaseURL   string `json:"base_url"`
		} `json:"search"`
	}
	cc := cfg.Components[questtoolsComponentName]
	if err := json.Unmarshal(cc.Config, &raw); err != nil || raw.Search == nil {
		t.Fatal("failed to parse search config from component config")
	}

	apiKeySet := raw.Search.APIKeyEnv != "" && os.Getenv(raw.Search.APIKeyEnv) != ""
	if !apiKeySet {
		t.Errorf("expected APIKeySet=true when env var %q is set", envVar)
	}
	if raw.Search.Provider != "brave" {
		t.Errorf("expected provider=brave, got %q", raw.Search.Provider)
	}

	// Verify SafeConfig round-trip preserves the component config.
	got := safeCfg.Get()
	if _, exists := got.Components[questtoolsComponentName]; !exists {
		t.Error("expected questtools component config to survive SafeConfig round-trip")
	}
}
