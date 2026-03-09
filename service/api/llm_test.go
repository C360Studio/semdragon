package api

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

// capturedRequest holds the parts of an incoming HTTP request that tests
// need to assert against without holding a reference to the closed body.
type capturedRequest struct {
	method  string
	path    string
	headers http.Header
	body    map[string]any
}

// captureRequest reads and parses the incoming request body as JSON, storing
// everything a test needs in a capturedRequest value.
func captureRequest(t *testing.T, r *http.Request) capturedRequest {
	t.Helper()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("parse request body as JSON: %v", err)
	}
	return capturedRequest{
		method:  r.Method,
		path:    r.URL.Path,
		headers: r.Header.Clone(),
		body:    body,
	}
}

// anthropicOKResponse returns a minimal valid Anthropic Messages API response
// with the given content text and token counts.
func anthropicOKResponse(t *testing.T, text string, inputTokens, outputTokens int) string {
	t.Helper()
	resp := map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": text},
		},
		"usage": map[string]any{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		},
	}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal anthropic response: %v", err)
	}
	return string(b)
}

// openAIOKResponse returns a minimal valid OpenAI chat completions response.
func openAIOKResponse(t *testing.T, content string, promptTokens, completionTokens int) string {
	t.Helper()
	resp := map[string]any{
		"choices": []map[string]any{
			{
				"message": map[string]any{
					"role":    "assistant",
					"content": content,
				},
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     promptTokens,
			"completion_tokens": completionTokens,
		},
	}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal openai response: %v", err)
	}
	return string(b)
}

// =============================================================================
// callAnthropic tests
// =============================================================================

func TestCallAnthropic_RequestFormat(t *testing.T) {
	const (
		apiKey       = "test-anthropic-key"
		systemPrompt = "You are the Dungeon Master."
		modelName    = "claude-opus-4-5"
	)

	var captured capturedRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = captureRequest(t, r)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, anthropicOKResponse(t, "response text", 10, 20)) //nolint:errcheck
	}))
	defer srv.Close()

	endpoint := &model.EndpointConfig{
		Provider: "anthropic",
		URL:      srv.URL,
		Model:    modelName,
	}
	messages := []ChatMessage{
		{Role: "user", Content: "Hello"},
	}

	_, err := callAnthropic(context.Background(), endpoint, apiKey, systemPrompt, messages)
	if err != nil {
		t.Fatalf("callAnthropic returned unexpected error: %v", err)
	}

	// Path must end with /messages.
	if !strings.HasSuffix(captured.path, "/messages") {
		t.Errorf("expected path to end with /messages, got %q", captured.path)
	}

	// Authentication header.
	if got := captured.headers.Get("x-api-key"); got != apiKey {
		t.Errorf("x-api-key header: got %q, want %q", got, apiKey)
	}

	// Anthropic version header.
	if got := captured.headers.Get("anthropic-version"); got != "2023-06-01" {
		t.Errorf("anthropic-version header: got %q, want %q", got, "2023-06-01")
	}

	// Content-Type header.
	if got := captured.headers.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type header: got %q, want application/json", got)
	}

	// No Authorization header (that is OpenAI-style).
	if got := captured.headers.Get("Authorization"); got != "" {
		t.Errorf("Authorization header should be absent for Anthropic calls, got %q", got)
	}

	// Required body fields.
	for _, field := range []string{"system", "messages", "model", "max_tokens"} {
		if _, ok := captured.body[field]; !ok {
			t.Errorf("request body missing required field %q", field)
		}
	}

	// system field must match the argument.
	if got := captured.body["system"]; got != systemPrompt {
		t.Errorf("body.system: got %q, want %q", got, systemPrompt)
	}

	// model field must match endpoint.Model.
	if got := captured.body["model"]; got != modelName {
		t.Errorf("body.model: got %q, want %q", got, modelName)
	}
}

func TestCallAnthropic_URLConstruction(t *testing.T) {
	tests := []struct {
		name    string
		baseURL func(serverURL string) string
		wantSuffix string
	}{
		{
			name:       "no trailing slash",
			baseURL:    func(u string) string { return u },
			wantSuffix: "/messages",
		},
		{
			name:       "trailing slash stripped",
			baseURL:    func(u string) string { return u + "/" },
			wantSuffix: "/messages",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, anthropicOKResponse(t, "ok", 1, 1)) //nolint:errcheck
			}))
			defer srv.Close()

			endpoint := &model.EndpointConfig{
				Provider: "anthropic",
				URL:      tc.baseURL(srv.URL),
				Model:    "claude-3-5-haiku-latest",
			}

			_, err := callAnthropic(context.Background(), endpoint, "key", "sys", nil)
			if err != nil {
				t.Fatalf("callAnthropic: %v", err)
			}

			if !strings.HasSuffix(gotPath, tc.wantSuffix) {
				t.Errorf("URL path: got %q, want suffix %q", gotPath, tc.wantSuffix)
			}
		})
	}
}

func TestCallAnthropic_ResponseParsing(t *testing.T) {
	const (
		wantContent      = "Quest accepted, adventurer."
		wantPromptTok    = 42
		wantComplTok     = 17
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, anthropicOKResponse(t, wantContent, wantPromptTok, wantComplTok)) //nolint:errcheck
	}))
	defer srv.Close()

	endpoint := &model.EndpointConfig{
		Provider: "anthropic",
		URL:      srv.URL,
		Model:    "claude-opus-4-5",
	}

	result, err := callAnthropic(context.Background(), endpoint, "key", "sys", []ChatMessage{
		{Role: "user", Content: "Begin the quest."},
	})
	if err != nil {
		t.Fatalf("callAnthropic: %v", err)
	}

	if result.Content != wantContent {
		t.Errorf("Content: got %q, want %q", result.Content, wantContent)
	}
	if result.PromptTokens != wantPromptTok {
		t.Errorf("PromptTokens: got %d, want %d", result.PromptTokens, wantPromptTok)
	}
	if result.CompletionTokens != wantComplTok {
		t.Errorf("CompletionTokens: got %d, want %d", result.CompletionTokens, wantComplTok)
	}
}

func TestCallAnthropic_ErrorResponse(t *testing.T) {
	const errBody = `{"type":"error","error":{"type":"invalid_request_error","message":"bad request"}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, errBody) //nolint:errcheck
	}))
	defer srv.Close()

	endpoint := &model.EndpointConfig{
		Provider: "anthropic",
		URL:      srv.URL,
		Model:    "claude-opus-4-5",
	}

	_, err := callAnthropic(context.Background(), endpoint, "key", "sys", nil)
	if err == nil {
		t.Fatal("expected error for 400 response, got nil")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error should contain status code 400, got: %v", err)
	}
	if !strings.Contains(err.Error(), errBody) {
		t.Errorf("error should contain response body, got: %v", err)
	}
}

// =============================================================================
// callOpenAICompat tests
// =============================================================================

func TestCallOpenAICompat_RequestFormat(t *testing.T) {
	const (
		apiKey       = "test-openai-key"
		systemPrompt = "You control the dungeon."
		modelName    = "gpt-4o"
	)

	var captured capturedRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = captureRequest(t, r)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, openAIOKResponse(t, "response", 10, 5)) //nolint:errcheck
	}))
	defer srv.Close()

	endpoint := &model.EndpointConfig{
		Provider: "openai",
		URL:      srv.URL,
		Model:    modelName,
	}
	messages := []ChatMessage{
		{Role: "user", Content: "What lurks here?"},
		{Role: "dm", Content: "A dragon."},
		{Role: "user", Content: "I attack!"},
	}

	_, err := callOpenAICompat(context.Background(), endpoint, apiKey, systemPrompt, messages)
	if err != nil {
		t.Fatalf("callOpenAICompat returned unexpected error: %v", err)
	}

	// Path must end with /chat/completions.
	if !strings.HasSuffix(captured.path, "/chat/completions") {
		t.Errorf("expected path to end with /chat/completions, got %q", captured.path)
	}

	// Authorization header must use Bearer scheme.
	wantAuth := "Bearer " + apiKey
	if got := captured.headers.Get("Authorization"); got != wantAuth {
		t.Errorf("Authorization header: got %q, want %q", got, wantAuth)
	}

	// No x-api-key header (that is Anthropic-style).
	if got := captured.headers.Get("x-api-key"); got != "" {
		t.Errorf("x-api-key header should be absent for OpenAI calls, got %q", got)
	}

	// Body must have a messages array.
	rawMessages, ok := captured.body["messages"]
	if !ok {
		t.Fatal("request body missing 'messages' field")
	}
	msgs, ok := rawMessages.([]any)
	if !ok {
		t.Fatalf("body.messages is not an array, got %T", rawMessages)
	}

	// First message must be the system prompt.
	if len(msgs) == 0 {
		t.Fatal("body.messages is empty, expected at least a system message")
	}
	first, ok := msgs[0].(map[string]any)
	if !ok {
		t.Fatalf("body.messages[0] is not an object, got %T", msgs[0])
	}
	if first["role"] != "system" {
		t.Errorf("body.messages[0].role: got %v, want system", first["role"])
	}
	if first["content"] != systemPrompt {
		t.Errorf("body.messages[0].content: got %v, want %q", first["content"], systemPrompt)
	}

	// "dm" role in input must be mapped to "assistant".
	// messages[2] in the original becomes index 2 in body after the prepended system message.
	if len(msgs) < 3 {
		t.Fatalf("expected at least 3 messages, got %d", len(msgs))
	}
	dmMsg, ok := msgs[2].(map[string]any)
	if !ok {
		t.Fatalf("body.messages[2] is not an object, got %T", msgs[2])
	}
	if dmMsg["role"] != "assistant" {
		t.Errorf("'dm' role should be mapped to 'assistant', got %v", dmMsg["role"])
	}
}

func TestCallOpenAICompat_ResponseParsing(t *testing.T) {
	const (
		wantContent  = "You slay the dragon and earn 500 XP."
		wantPrompt   = 30
		wantCompl    = 12
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, openAIOKResponse(t, wantContent, wantPrompt, wantCompl)) //nolint:errcheck
	}))
	defer srv.Close()

	endpoint := &model.EndpointConfig{
		Provider: "openai",
		URL:      srv.URL,
		Model:    "gpt-4o",
	}

	result, err := callOpenAICompat(context.Background(), endpoint, "key", "sys", []ChatMessage{
		{Role: "user", Content: "Do I survive?"},
	})
	if err != nil {
		t.Fatalf("callOpenAICompat: %v", err)
	}

	if result.Content != wantContent {
		t.Errorf("Content: got %q, want %q", result.Content, wantContent)
	}
	if result.PromptTokens != wantPrompt {
		t.Errorf("PromptTokens: got %d, want %d", result.PromptTokens, wantPrompt)
	}
	if result.CompletionTokens != wantCompl {
		t.Errorf("CompletionTokens: got %d, want %d", result.CompletionTokens, wantCompl)
	}
}

func TestCallOpenAICompat_ErrorResponse(t *testing.T) {
	const errBody = `{"error":{"message":"internal server error","type":"server_error"}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, errBody) //nolint:errcheck
	}))
	defer srv.Close()

	endpoint := &model.EndpointConfig{
		Provider: "openai",
		URL:      srv.URL,
		Model:    "gpt-4o",
	}

	_, err := callOpenAICompat(context.Background(), endpoint, "key", "sys", nil)
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should contain status code 500, got: %v", err)
	}
	if !strings.Contains(err.Error(), errBody) {
		t.Errorf("error should contain response body, got: %v", err)
	}
}

// =============================================================================
// callLLM routing tests
// =============================================================================

func TestCallLLM_ProviderRouting(t *testing.T) {
	tests := []struct {
		name         string
		provider     string
		wantPathSuff string
	}{
		{
			name:         "anthropic provider hits /messages",
			provider:     "anthropic",
			wantPathSuff: "/messages",
		},
		{
			name:         "openai provider hits /chat/completions",
			provider:     "openai",
			wantPathSuff: "/chat/completions",
		},
		{
			name:         "empty provider defaults to openai-compat /chat/completions",
			provider:     "",
			wantPathSuff: "/chat/completions",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath string

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				// Drain body so the client doesn't error.
				io.Copy(io.Discard, r.Body) //nolint:errcheck
				w.Header().Set("Content-Type", "application/json")

				// Return the appropriate response shape based on path.
				if strings.HasSuffix(r.URL.Path, "/messages") {
					io.WriteString(w, anthropicOKResponse(t, "ok", 1, 1)) //nolint:errcheck
				} else {
					io.WriteString(w, openAIOKResponse(t, "ok", 1, 1)) //nolint:errcheck
				}
			}))
			defer srv.Close()

			const envKey = "TEST_LLM_ROUTING_API_KEY"
			t.Setenv(envKey, "routing-test-key")

			endpoint := &model.EndpointConfig{
				Provider:  tc.provider,
				URL:       srv.URL,
				Model:     "test-model",
				APIKeyEnv: envKey,
			}

			_, err := callLLM(context.Background(), endpoint, "sys", []ChatMessage{
				{Role: "user", Content: "test"},
			})
			if err != nil {
				t.Fatalf("callLLM: %v", err)
			}

			if !strings.HasSuffix(gotPath, tc.wantPathSuff) {
				t.Errorf("provider %q: path got %q, want suffix %q", tc.provider, gotPath, tc.wantPathSuff)
			}
		})
	}
}

func TestCallLLM_MissingAPIKey(t *testing.T) {
	endpoint := &model.EndpointConfig{
		Provider:  "anthropic",
		URL:       "http://localhost:9999",
		Model:     "claude-opus-4-5",
		APIKeyEnv: "SEMDRAGONS_MISSING_KEY_XYZ",
	}

	// Ensure the env var is definitely absent.
	t.Setenv("SEMDRAGONS_MISSING_KEY_XYZ", "")

	_, err := callLLM(context.Background(), endpoint, "sys", nil)
	if err == nil {
		t.Fatal("expected error when API key env var is empty, got nil")
	}
	if !strings.Contains(err.Error(), "SEMDRAGONS_MISSING_KEY_XYZ") {
		t.Errorf("error should mention the missing env var name, got: %v", err)
	}
}

func TestCallLLM_NilEndpoint(t *testing.T) {
	_, err := callLLM(context.Background(), nil, "sys", nil)
	if err == nil {
		t.Fatal("expected error for nil endpoint, got nil")
	}
}
