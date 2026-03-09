package questbridge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/model"
)

// mockRegistry satisfies model.RegistryReader for unit tests.
type mockRegistry struct {
	endpoints map[string]*model.EndpointConfig
	caps      map[string]string // capability -> endpoint name
}

func (m *mockRegistry) Resolve(cap string) string { return m.caps[cap] }
func (m *mockRegistry) GetEndpoint(name string) *model.EndpointConfig {
	return m.endpoints[name]
}
// GetFallbackChain satisfies model.RegistryReader — tests don't exercise fallback chains,
// so the key parameter is genuinely unused in this mock.
func (m *mockRegistry) GetFallbackChain(_ string) []string { return nil }

// GetMaxTokens satisfies model.RegistryReader — tests hardcode token limits on the endpoint directly,
// so the name parameter is genuinely unused in this mock.
func (m *mockRegistry) GetMaxTokens(_ string) int { return 0 }
func (m *mockRegistry) GetDefault() string                  { return "" }
func (m *mockRegistry) ListCapabilities() []string          { return nil }
func (m *mockRegistry) ListEndpoints() []string             { return nil }

// anthropicOKResponse returns a minimal valid Anthropic Messages API response body.
func anthropicOKResponse(text string) string {
	return `{"content":[{"type":"text","text":"` + text + `"}]}`
}

// openaiOKResponse returns a minimal valid OpenAI chat completions response body.
func openaiOKResponse(content string) string {
	return `{"choices":[{"message":{"role":"assistant","content":"` + content + `"}}]}`
}

// =============================================================================
// TestDMAnswerer_Anthropic_RequestFormat
// =============================================================================

func TestDMAnswerer_Anthropic_RequestFormat(t *testing.T) {
	var capturedReq *http.Request
	var capturedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		body, _ := io.ReadAll(r.Body)
		capturedBody = body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(anthropicOKResponse("You must gather your party before venturing forth.")))
	}))
	defer srv.Close()

	reg := &mockRegistry{
		caps: map[string]string{"dm-chat": "anthropic-claude"},
		endpoints: map[string]*model.EndpointConfig{
			"anthropic-claude": {
				Provider:  "anthropic",
				URL:       srv.URL + "/v1",
				Model:     "claude-3-5-sonnet-20241022",
				MaxTokens: 4096,
				APIKeyEnv: "TEST_ANTHROPIC_KEY",
			},
		},
	}

	t.Setenv("TEST_ANTHROPIC_KEY", "sk-ant-test-key")

	answerer := newRegistryAnswerer(reg)
	answer, err := answerer.AnswerClarification(
		context.Background(),
		"Slay the Dragon",
		"Travel to the mountain and defeat the ancient dragon.",
		"Which mountain?",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if answer == "" {
		t.Fatal("expected non-empty answer")
	}

	// Verify URL path ends with /messages.
	if !strings.HasSuffix(capturedReq.URL.Path, "/messages") {
		t.Errorf("expected path to end with /messages, got %q", capturedReq.URL.Path)
	}

	// Verify Anthropic-specific headers are present.
	if capturedReq.Header.Get("x-api-key") == "" {
		t.Error("expected x-api-key header to be set")
	}
	if capturedReq.Header.Get("x-api-key") != "sk-ant-test-key" {
		t.Errorf("x-api-key = %q, want %q", capturedReq.Header.Get("x-api-key"), "sk-ant-test-key")
	}
	if got := capturedReq.Header.Get("anthropic-version"); got != "2023-06-01" {
		t.Errorf("anthropic-version = %q, want %q", got, "2023-06-01")
	}

	// Authorization header must NOT be set for Anthropic (uses x-api-key instead).
	if auth := capturedReq.Header.Get("Authorization"); auth != "" {
		t.Errorf("Authorization header should not be set for Anthropic, got %q", auth)
	}

	// Verify request body structure.
	var body map[string]any
	if err := json.Unmarshal(capturedBody, &body); err != nil {
		t.Fatalf("could not unmarshal request body: %v", err)
	}

	// system must be a top-level field (not in messages array).
	system, ok := body["system"]
	if !ok {
		t.Error("expected top-level 'system' field in Anthropic request body")
	}
	if _, isStr := system.(string); !isStr || system == "" {
		t.Errorf("'system' field should be a non-empty string, got %T %v", system, system)
	}

	// messages array must exist and contain only user role entries.
	rawMessages, ok := body["messages"]
	if !ok {
		t.Fatal("expected 'messages' field in request body")
	}
	messages, ok := rawMessages.([]any)
	if !ok {
		t.Fatalf("'messages' should be an array, got %T", rawMessages)
	}
	if len(messages) == 0 {
		t.Fatal("messages array must not be empty")
	}
	firstMsg, ok := messages[0].(map[string]any)
	if !ok {
		t.Fatalf("first message should be an object, got %T", messages[0])
	}
	if role := firstMsg["role"]; role != "user" {
		t.Errorf("first message role = %q, want %q", role, "user")
	}

	// Verify that model and max_tokens are set.
	if body["model"] == nil || body["model"] == "" {
		t.Error("expected 'model' field in request body")
	}
	if body["max_tokens"] == nil {
		t.Error("expected 'max_tokens' field in Anthropic request body")
	}
}

// =============================================================================
// TestDMAnswerer_Anthropic_URLConstruction
// =============================================================================

func TestDMAnswerer_Anthropic_URLConstruction(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		wantSuffix  string
	}{
		{
			name:       "base URL without trailing slash",
			baseURL:    "", // replaced below with srv.URL + "/v1"
			wantSuffix: "/v1/messages",
		},
		{
			name:       "base URL with trailing slash",
			baseURL:    "", // replaced below with srv.URL + "/v1/"
			wantSuffix: "/v1/messages",
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var capturedPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(anthropicOKResponse("answer")))
			}))
			defer srv.Close()

			var base string
			switch i {
			case 0:
				base = srv.URL + "/v1" // no trailing slash
			case 1:
				base = srv.URL + "/v1/" // with trailing slash
			}

			reg := &mockRegistry{
				caps: map[string]string{"dm-chat": "ep"},
				endpoints: map[string]*model.EndpointConfig{
					"ep": {
						Provider:  "anthropic",
						URL:       base,
						Model:     "claude-3-haiku-20240307",
						MaxTokens: 1024,
						APIKeyEnv: "TEST_ANTHROPIC_KEY_URL",
					},
				},
			}
			t.Setenv("TEST_ANTHROPIC_KEY_URL", "sk-ant-url-test")

			answerer := newRegistryAnswerer(reg)
			_, err := answerer.AnswerClarification(context.Background(), "T", "D", "Q")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !strings.HasSuffix(capturedPath, "/messages") {
				t.Errorf("path %q does not end with /messages", capturedPath)
			}
			// Both variants must produce the same clean path without double slashes.
			if strings.Contains(capturedPath, "//") {
				t.Errorf("path contains double slash: %q", capturedPath)
			}
		})
	}
}

// =============================================================================
// TestDMAnswerer_OpenAICompat_RequestFormat
// =============================================================================

func TestDMAnswerer_OpenAICompat_RequestFormat(t *testing.T) {
	var capturedReq *http.Request
	var capturedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		body, _ := io.ReadAll(r.Body)
		capturedBody = body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(openaiOKResponse("Here is your answer.")))
	}))
	defer srv.Close()

	reg := &mockRegistry{
		caps: map[string]string{"dm-chat": "openai-gpt4"},
		endpoints: map[string]*model.EndpointConfig{
			"openai-gpt4": {
				Provider:  "openai",
				URL:       srv.URL + "/v1",
				Model:     "gpt-4o",
				MaxTokens: 4096,
				APIKeyEnv: "TEST_OPENAI_KEY",
			},
		},
	}

	t.Setenv("TEST_OPENAI_KEY", "sk-openai-test-key")

	answerer := newRegistryAnswerer(reg)
	answer, err := answerer.AnswerClarification(
		context.Background(),
		"Build the Widget",
		"Create a widget that processes data efficiently.",
		"What data format should we use?",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if answer == "" {
		t.Fatal("expected non-empty answer")
	}

	// Verify URL path ends with /chat/completions.
	if !strings.HasSuffix(capturedReq.URL.Path, "/chat/completions") {
		t.Errorf("expected path to end with /chat/completions, got %q", capturedReq.URL.Path)
	}

	// Verify Authorization: Bearer header.
	auth := capturedReq.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		t.Errorf("expected Authorization: Bearer ..., got %q", auth)
	}
	if auth != "Bearer sk-openai-test-key" {
		t.Errorf("Authorization = %q, want %q", auth, "Bearer sk-openai-test-key")
	}

	// Verify body structure: system message must be in messages array (not top-level).
	var body map[string]any
	if err := json.Unmarshal(capturedBody, &body); err != nil {
		t.Fatalf("could not unmarshal request body: %v", err)
	}

	// OpenAI compat: no top-level system field.
	if _, hasSystem := body["system"]; hasSystem {
		t.Error("OpenAI request must not have a top-level 'system' field")
	}

	rawMessages, ok := body["messages"]
	if !ok {
		t.Fatal("expected 'messages' field in request body")
	}
	messages, ok := rawMessages.([]any)
	if !ok {
		t.Fatalf("'messages' should be an array, got %T", rawMessages)
	}

	// Must contain at least a system message and a user message.
	if len(messages) < 2 {
		t.Fatalf("expected at least 2 messages (system + user), got %d", len(messages))
	}

	roles := make([]string, 0, len(messages))
	for _, m := range messages {
		msg, ok := m.(map[string]any)
		if !ok {
			t.Fatalf("message should be an object, got %T", m)
		}
		roles = append(roles, msg["role"].(string))
	}

	if roles[0] != "system" {
		t.Errorf("first message role = %q, want %q", roles[0], "system")
	}
	hasUser := false
	for _, r := range roles {
		if r == "user" {
			hasUser = true
		}
	}
	if !hasUser {
		t.Error("messages must contain at least one user message")
	}

	// User message content must include quest context.
	userMsg := findMessageByRole(messages, "user")
	if userMsg == nil {
		t.Fatal("no user message found")
	}
	content, _ := userMsg["content"].(string)
	if !strings.Contains(content, "Build the Widget") {
		t.Errorf("user message content should contain quest title, got: %q", content)
	}
}

// findMessageByRole returns the first message in the slice with the given role.
func findMessageByRole(messages []any, role string) map[string]any {
	for _, m := range messages {
		msg, ok := m.(map[string]any)
		if !ok {
			continue
		}
		if msg["role"] == role {
			return msg
		}
	}
	return nil
}

// =============================================================================
// TestDMAnswerer_ProviderRouting
// =============================================================================

func TestDMAnswerer_ProviderRouting(t *testing.T) {
	tests := []struct {
		name         string
		provider     string
		wantPathSfx  string
	}{
		{
			name:        "anthropic provider routes to /messages",
			provider:    "anthropic",
			wantPathSfx: "/messages",
		},
		{
			name:        "openai provider routes to /chat/completions",
			provider:    "openai",
			wantPathSfx: "/chat/completions",
		},
		{
			name:        "empty provider defaults to openai compat route",
			provider:    "",
			wantPathSfx: "/chat/completions",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var capturedPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				if tc.provider == "anthropic" {
					_, _ = w.Write([]byte(anthropicOKResponse("routed")))
				} else {
					_, _ = w.Write([]byte(openaiOKResponse("routed")))
				}
			}))
			defer srv.Close()

			ep := &model.EndpointConfig{
				Provider:  tc.provider,
				URL:       srv.URL + "/v1",
				Model:     "test-model",
				MaxTokens: 1024,
				APIKeyEnv: "TEST_ROUTING_KEY",
			}

			reg := &mockRegistry{
				caps:      map[string]string{"dm-chat": "ep"},
				endpoints: map[string]*model.EndpointConfig{"ep": ep},
			}
			t.Setenv("TEST_ROUTING_KEY", "test-routing-key")

			answerer := newRegistryAnswerer(reg)
			_, err := answerer.AnswerClarification(context.Background(), "T", "D", "Q")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !strings.HasSuffix(capturedPath, tc.wantPathSfx) {
				t.Errorf("provider=%q: path %q does not end with %q", tc.provider, capturedPath, tc.wantPathSfx)
			}
		})
	}
}

// =============================================================================
// TestDMAnswerer_APIKeyResolution
// =============================================================================

func TestDMAnswerer_APIKeyResolution(t *testing.T) {
	t.Run("empty api_key_env with no env var set returns error before HTTP call", func(t *testing.T) {
		httpCalled := false
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			httpCalled = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(openaiOKResponse("should not reach here")))
		}))
		defer srv.Close()

		reg := &mockRegistry{
			caps: map[string]string{"dm-chat": "ep"},
			endpoints: map[string]*model.EndpointConfig{
				"ep": {
					Provider:  "openai",
					URL:       srv.URL + "/v1",
					Model:     "gpt-4o-mini",
					MaxTokens: 1024,
					// APIKeyEnv is set to a name but the env var itself is absent.
					APIKeyEnv: "DEFINITELY_NOT_SET_XYZ_12345",
				},
			},
		}

		// Ensure the env var is not set (use t.Setenv to guarantee cleanup, but don't assign a value).
		// We simply do not call t.Setenv so the variable remains unset.
		answerer := newRegistryAnswerer(reg)
		_, err := answerer.AnswerClarification(context.Background(), "T", "D", "Q")
		if err == nil {
			t.Fatal("expected error when required API key env var is not set, got nil")
		}
		if !strings.Contains(err.Error(), "DEFINITELY_NOT_SET_XYZ_12345") {
			t.Errorf("error should mention the missing env var name, got: %v", err)
		}
		if httpCalled {
			t.Error("HTTP server should not have been called when API key is missing")
		}
	})

	t.Run("set env var is used as API key in request", func(t *testing.T) {
		var capturedAuth string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(openaiOKResponse("got it")))
		}))
		defer srv.Close()

		reg := &mockRegistry{
			caps: map[string]string{"dm-chat": "ep"},
			endpoints: map[string]*model.EndpointConfig{
				"ep": {
					Provider:  "openai",
					URL:       srv.URL + "/v1",
					Model:     "gpt-4o-mini",
					MaxTokens: 1024,
					APIKeyEnv: "TEST_DM_API_KEY_SET",
				},
			},
		}

		const wantKey = "sk-actual-secret-key"
		t.Setenv("TEST_DM_API_KEY_SET", wantKey)

		answerer := newRegistryAnswerer(reg)
		_, err := answerer.AnswerClarification(context.Background(), "T", "D", "Q")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		wantAuth := "Bearer " + wantKey
		if capturedAuth != wantAuth {
			t.Errorf("Authorization header = %q, want %q", capturedAuth, wantAuth)
		}
	})
}
