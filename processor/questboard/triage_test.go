package questboard

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/model"
)

// =============================================================================
// TRIAGE PROMPT CONSTRUCTION TESTS
// =============================================================================

func TestBuildTriageSystemPrompt(t *testing.T) {
	prompt := buildTriageSystemPrompt()

	if !strings.Contains(prompt, "Dungeon Master") {
		t.Error("expected DM role in system prompt")
	}
	if !strings.Contains(prompt, "salvage") {
		t.Error("expected salvage path description")
	}
	if !strings.Contains(prompt, "tpk") {
		t.Error("expected tpk path description")
	}
	if !strings.Contains(prompt, "escalate") {
		t.Error("expected escalate path description")
	}
	if !strings.Contains(prompt, "terminal") {
		t.Error("expected terminal path description")
	}
	if !strings.Contains(prompt, "JSON") {
		t.Error("expected JSON response format instruction")
	}
}

func TestBuildTriageUserMessage(t *testing.T) {
	quest := &domain.Quest{
		ID:          "test.dev.game.board1.quest.q1",
		Title:       "Build caching layer",
		Goal:        "Implement distributed cache",
		Difficulty:  domain.DifficultyHard,
		Attempts:    3,
		MaxAttempts: 3,
		FailureReason: "Timeout after 30 minutes",
		FailureType:   domain.FailureTimeout,
		Output:        "Partial implementation of cache interface",
		Requirements:  []string{"Support TTL", "Handle 10k req/s"},
		Acceptance:    []string{"All tests pass", "Benchmarks meet target"},
		FailureHistory: []domain.FailureRecord{
			{Attempt: 1, FailureType: domain.FailureError, FailureReason: "Loop error"},
			{Attempt: 2, FailureType: domain.FailureTimeout, FailureReason: "Timed out"},
		},
	}

	msg := buildTriageUserMessage(quest)

	if !strings.Contains(msg, "Build caching layer") {
		t.Error("expected quest title")
	}
	if !strings.Contains(msg, "Implement distributed cache") {
		t.Error("expected quest goal")
	}
	if !strings.Contains(msg, "3/3") {
		t.Error("expected attempts ratio")
	}
	if !strings.Contains(msg, "Timeout after 30 minutes") {
		t.Error("expected failure reason")
	}
	if !strings.Contains(msg, "timeout") {
		t.Error("expected failure type")
	}
	if !strings.Contains(msg, "FAILURE HISTORY") {
		t.Error("expected failure history section")
	}
	if !strings.Contains(msg, "Attempt 1") {
		t.Error("expected attempt 1 in history")
	}
	if !strings.Contains(msg, "Attempt 2") {
		t.Error("expected attempt 2 in history")
	}
	if !strings.Contains(msg, "Partial implementation") {
		t.Error("expected agent output")
	}
	if !strings.Contains(msg, "Support TTL") {
		t.Error("expected requirements")
	}
	if !strings.Contains(msg, "All tests pass") {
		t.Error("expected acceptance criteria")
	}
}

func TestBuildTriageUserMessage_DescriptionFallback(t *testing.T) {
	quest := &domain.Quest{
		ID:          "test.dev.game.board1.quest.q1",
		Title:       "Simple quest",
		Description: "Uses description when no goal",
		Attempts:    1,
		MaxAttempts: 1,
	}

	msg := buildTriageUserMessage(quest)

	if !strings.Contains(msg, "Uses description when no goal") {
		t.Error("expected description as fallback for goal")
	}
}

func TestBuildTriageUserMessage_LongOutputTruncated(t *testing.T) {
	quest := &domain.Quest{
		ID:          "test.dev.game.board1.quest.q1",
		Title:       "Long output",
		Output:      strings.Repeat("x", 3000),
		Attempts:    1,
		MaxAttempts: 1,
	}

	msg := buildTriageUserMessage(quest)

	if !strings.Contains(msg, "[truncated]") {
		t.Error("expected truncation marker for long output")
	}
	outputIdx := strings.Index(msg, "AGENT OUTPUT")
	if outputIdx < 0 {
		t.Fatal("expected AGENT OUTPUT section")
	}
	afterOutput := msg[outputIdx:]
	if len(afterOutput) > 2200 {
		t.Errorf("output section too long: %d chars (expected ~2000)", len(afterOutput))
	}
}

// =============================================================================
// TRIAGE RESPONSE PARSING TESTS
// =============================================================================

func TestParseTriageResponse_ValidJSON(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    triageLLMResponse
	}{
		{
			name:    "salvage path",
			content: `{"path":"salvage","analysis":"Good partial work","salvaged_output":"Cache interface done","anti_patterns":[]}`,
			want: triageLLMResponse{
				Path:           "salvage",
				Analysis:       "Good partial work",
				SalvagedOutput: "Cache interface done",
				AntiPatterns:   []string{},
			},
		},
		{
			name:    "tpk path with anti-patterns",
			content: `{"path":"tpk","analysis":"Wrong approach entirely","salvaged_output":"","anti_patterns":["Ignored requirements","Used wrong API"]}`,
			want: triageLLMResponse{
				Path:         "tpk",
				Analysis:     "Wrong approach entirely",
				AntiPatterns: []string{"Ignored requirements", "Used wrong API"},
			},
		},
		{
			name:    "escalate path",
			content: `{"path":"escalate","analysis":"Quest definition is ambiguous","salvaged_output":"","anti_patterns":[]}`,
			want: triageLLMResponse{
				Path:     "escalate",
				Analysis: "Quest definition is ambiguous",
			},
		},
		{
			name:    "terminal path",
			content: `{"path":"terminal","analysis":"Quest is no longer relevant","salvaged_output":"","anti_patterns":[]}`,
			want: triageLLMResponse{
				Path:     "terminal",
				Analysis: "Quest is no longer relevant",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := parseTriageResponse(tt.content)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.Path != tt.want.Path {
				t.Errorf("Path = %q, want %q", resp.Path, tt.want.Path)
			}
			if resp.Analysis != tt.want.Analysis {
				t.Errorf("Analysis = %q, want %q", resp.Analysis, tt.want.Analysis)
			}
		})
	}
}

func TestParseTriageResponse_MarkdownFences(t *testing.T) {
	content := "```json\n{\"path\":\"salvage\",\"analysis\":\"Wrapped in fences\",\"salvaged_output\":\"\",\"anti_patterns\":[]}\n```"

	resp, err := parseTriageResponse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Path != "salvage" {
		t.Errorf("Path = %q, want %q", resp.Path, "salvage")
	}
	if resp.Analysis != "Wrapped in fences" {
		t.Errorf("Analysis = %q, want %q", resp.Analysis, "Wrapped in fences")
	}
}

func TestParseTriageResponse_InvalidPath(t *testing.T) {
	content := `{"path":"retry","analysis":"Not a valid path","salvaged_output":"","anti_patterns":[]}`

	_, err := parseTriageResponse(content)
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
	if !strings.Contains(err.Error(), "invalid recovery path") {
		t.Errorf("expected 'invalid recovery path' error, got: %v", err)
	}
}

func TestParseTriageResponse_InvalidJSON(t *testing.T) {
	_, err := parseTriageResponse("not json at all")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseTriageResponse_EmptyContent(t *testing.T) {
	_, err := parseTriageResponse("")
	if err == nil {
		t.Fatal("expected error for empty content")
	}
}

// =============================================================================
// LLM RESPONSE TO DECISION CONVERSION
// =============================================================================

func TestLLMResponseToDecision_Salvage(t *testing.T) {
	resp := &triageLLMResponse{
		Path:           "salvage",
		Analysis:       "Good work so far",
		SalvagedOutput: "Partial implementation",
		AntiPatterns:   nil,
	}

	decision := llmResponseToDecision(resp)

	if decision.Path != domain.RecoverySalvage {
		t.Errorf("Path = %v, want %v", decision.Path, domain.RecoverySalvage)
	}
	if decision.Analysis != "Good work so far" {
		t.Errorf("Analysis = %q, want %q", decision.Analysis, "Good work so far")
	}
	if decision.SalvagedOutput == nil {
		t.Fatal("SalvagedOutput should not be nil for salvage path")
	}
}

func TestLLMResponseToDecision_TPK(t *testing.T) {
	resp := &triageLLMResponse{
		Path:         "tpk",
		Analysis:     "Total failure",
		AntiPatterns: []string{"Bad approach"},
	}

	decision := llmResponseToDecision(resp)

	if decision.Path != domain.RecoveryTPK {
		t.Errorf("Path = %v, want %v", decision.Path, domain.RecoveryTPK)
	}
	if len(decision.AntiPatterns) != 1 || decision.AntiPatterns[0] != "Bad approach" {
		t.Errorf("AntiPatterns = %v, want [Bad approach]", decision.AntiPatterns)
	}
	if decision.SalvagedOutput != nil {
		t.Errorf("SalvagedOutput should be nil for TPK (empty string), got %v", decision.SalvagedOutput)
	}
}

// =============================================================================
// AUTO-TRIAGE DM MODE ROUTING
// =============================================================================

func TestAutoTriage_ManualMode_SkipsLLM(t *testing.T) {
	comp := &Component{
		config: &Config{
			Triage: TriageConfig{
				Enabled: true,
				DMMode:  domain.DMManual,
			},
		},
		logger: slog.Default(),
	}

	quest := &domain.Quest{
		ID:     "test.dev.game.board1.quest.q1",
		Title:  "Test quest",
		Status: domain.QuestPendingTriage,
	}

	err := comp.autoTriage(context.Background(), quest)
	if err != nil {
		t.Fatalf("expected nil error for manual mode, got: %v", err)
	}
}

func TestAutoTriage_SupervisedMode_SkipsLLM(t *testing.T) {
	comp := &Component{
		config: &Config{
			Triage: TriageConfig{
				Enabled: true,
				DMMode:  domain.DMSupervised,
			},
		},
		logger: slog.Default(),
	}

	quest := &domain.Quest{
		ID:     "test.dev.game.board1.quest.q1",
		Title:  "Test quest",
		Status: domain.QuestPendingTriage,
	}

	err := comp.autoTriage(context.Background(), quest)
	if err != nil {
		t.Fatalf("expected nil error for supervised mode, got: %v", err)
	}
}

// =============================================================================
// MOCK LLM SERVER FOR TRIAGE
// =============================================================================

func TestCallTriageLLM_OpenAICompat(t *testing.T) {
	triageResp := triageLLMResponse{
		Path:           "salvage",
		Analysis:       "Partial work is usable",
		SalvagedOutput: "Cache interface implementation",
		AntiPatterns:   nil,
	}
	// The LLM returns the JSON as a string in the content field, so we need to
	// JSON-encode it twice: once for the triage response, once for the content string.
	innerJSON, _ := json.Marshal(triageResp)
	contentStr, _ := json.Marshal(string(innerJSON))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			t.Errorf("expected /chat/completions path, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"choices":[{"message":{"content":%s}}]}`, string(contentStr))
	}))
	defer server.Close()

	endpoint := &model.EndpointConfig{
		Provider: "openai",
		Model:    "gpt-4",
		URL:      server.URL,
	}

	quest := &domain.Quest{
		ID:            "test.dev.game.board1.quest.q1",
		Title:         "Test quest",
		Goal:          "Do something",
		Attempts:      3,
		MaxAttempts:   3,
		FailureReason: "Timed out",
	}

	content, err := callTriageLLMEndpoint(context.Background(), endpoint, buildTriageSystemPrompt(), buildTriageUserMessage(quest))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp, err := parseTriageResponse(content)
	if err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Path != "salvage" {
		t.Errorf("Path = %q, want %q", resp.Path, "salvage")
	}
	if resp.Analysis != "Partial work is usable" {
		t.Errorf("Analysis = %q, want %q", resp.Analysis, "Partial work is usable")
	}
}

func TestCallTriageLLM_Anthropic(t *testing.T) {
	triageResp := triageLLMResponse{
		Path:         "tpk",
		Analysis:     "Completely wrong approach",
		AntiPatterns: []string{"Used deprecated API"},
	}
	innerJSON, _ := json.Marshal(triageResp)
	contentStr, _ := json.Marshal(string(innerJSON))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Error("expected x-api-key header")
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Error("expected anthropic-version header")
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"content":[{"text":%s}]}`, string(contentStr))
	}))
	defer server.Close()

	t.Setenv("TEST_ANTHROPIC_KEY", "test-key")

	endpoint := &model.EndpointConfig{
		Provider:  "anthropic",
		Model:     "claude-sonnet-4-6",
		URL:       server.URL,
		APIKeyEnv: "TEST_ANTHROPIC_KEY",
	}

	content, err := callTriageLLMEndpoint(context.Background(), endpoint, buildTriageSystemPrompt(), "test user message")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp, err := parseTriageResponse(content)
	if err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Path != "tpk" {
		t.Errorf("Path = %q, want %q", resp.Path, "tpk")
	}
}

func TestCallTriageLLM_NoAPIKey(t *testing.T) {
	endpoint := &model.EndpointConfig{
		Provider:  "openai",
		Model:     "gpt-4",
		URL:       "http://localhost:9999",
		APIKeyEnv: "NONEXISTENT_KEY_FOR_TEST",
	}

	t.Setenv("NONEXISTENT_KEY_FOR_TEST", "")

	_, err := callTriageLLMEndpoint(context.Background(), endpoint, "system", "user")
	if err == nil {
		t.Fatal("expected error when API key is missing")
	}
	if !strings.Contains(err.Error(), "API key") {
		t.Errorf("expected API key error, got: %v", err)
	}
}

// =============================================================================
// TRIAGE CONFIG DEFAULTS
// =============================================================================

func TestTriageConfig_Defaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Triage.Enabled {
		t.Error("triage should be disabled by default")
	}
	if cfg.Triage.MinDifficultyForTriage != domain.DifficultyModerate {
		t.Errorf("MinDifficultyForTriage = %v, want %v", cfg.Triage.MinDifficultyForTriage, domain.DifficultyModerate)
	}
	if cfg.Triage.TriageTimeoutMins != 30 {
		t.Errorf("TriageTimeoutMins = %d, want 30", cfg.Triage.TriageTimeoutMins)
	}
	if cfg.Triage.DMMode != domain.DMFullAuto {
		t.Errorf("DMMode = %q, want %q", cfg.Triage.DMMode, domain.DMFullAuto)
	}
}

// =============================================================================
// TRUNCATE HELPER
// =============================================================================

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("truncate short = %q, want %q", got, "short")
	}
	if got := truncate("exactly10!", 10); got != "exactly10!" {
		t.Errorf("truncate exact = %q, want %q", got, "exactly10!")
	}
	if got := truncate("this is longer than ten", 10); got != "this is lo..." {
		t.Errorf("truncate long = %q, want %q", got, "this is lo...")
	}
}
