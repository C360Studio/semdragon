package semsource

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWsURLToHTTP(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"ws with /ws path", "ws://semsource:9090/ws", "http://semsource:9090"},
		{"wss with /ws path", "wss://semsource.example.com/ws", "https://semsource.example.com"},
		{"ws without /ws path", "ws://localhost:9090", "http://localhost:9090"},
		{"ws with trailing slash", "ws://host:9090/", "http://host:9090"},
		{"empty", "", ""},
		{"http not accepted", "http://host:9090", ""},
		{"whitespace trimmed", "  ws://host:9090/ws  ", "http://host:9090"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wsURLToHTTP(tt.in)
			if got != tt.want {
				t.Errorf("wsURLToHTTP(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNewManifestClient_EmptyURL(t *testing.T) {
	mc := NewManifestClient("", nil)
	if mc != nil {
		t.Error("expected nil for empty URL")
	}
}

func TestNewManifestClient_InvalidURL(t *testing.T) {
	mc := NewManifestClient("http://not-websocket", nil)
	if mc != nil {
		t.Error("expected nil for non-ws URL")
	}
}

func TestFetch_Success(t *testing.T) {
	payload := ManifestPayload{
		Sources: []SourceManifest{
			{Name: "test-repo", Type: "git_repo", Description: "A test repo", Status: "active"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != manifestPath {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	mc := &ManifestClient{
		baseURL:  srv.URL,
		cacheTTL: defaultCacheTTL,
		logger:   testLogger(),
	}

	got := mc.Fetch(context.Background())
	if got == nil {
		t.Fatal("expected non-nil manifest")
	}
	if len(got.Sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(got.Sources))
	}
	if got.Sources[0].Name != "test-repo" {
		t.Errorf("expected test-repo, got %q", got.Sources[0].Name)
	}
}

func TestFetch_CacheFresh(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		json.NewEncoder(w).Encode(ManifestPayload{
			Sources: []SourceManifest{{Name: "repo"}},
		})
	}))
	defer srv.Close()

	mc := &ManifestClient{
		baseURL:  srv.URL,
		cacheTTL: 1 * time.Hour,
		logger:   testLogger(),
	}

	mc.Fetch(context.Background())
	mc.Fetch(context.Background())

	if calls != 1 {
		t.Errorf("expected 1 HTTP call (cached), got %d", calls)
	}
}

func TestFetch_CacheExpired(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		json.NewEncoder(w).Encode(ManifestPayload{
			Sources: []SourceManifest{{Name: "repo"}},
		})
	}))
	defer srv.Close()

	mc := &ManifestClient{
		baseURL:  srv.URL,
		cacheTTL: 1 * time.Millisecond,
		logger:   testLogger(),
	}

	mc.Fetch(context.Background())
	time.Sleep(5 * time.Millisecond)
	mc.Fetch(context.Background())

	if calls != 2 {
		t.Errorf("expected 2 HTTP calls (cache expired), got %d", calls)
	}
}

func TestFetch_ServerDown_ReturnsNil(t *testing.T) {
	mc := &ManifestClient{
		baseURL:  "http://127.0.0.1:1", // nothing listening
		cacheTTL: defaultCacheTTL,
		logger:   testLogger(),
	}

	got := mc.Fetch(context.Background())
	if got != nil {
		t.Error("expected nil when server unreachable")
	}
}

func TestFetch_ServerDown_ReturnsStaleCacheIfAvailable(t *testing.T) {
	payload := ManifestPayload{
		Sources: []SourceManifest{{Name: "stale-repo"}},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(payload)
	}))

	mc := &ManifestClient{
		baseURL:  srv.URL,
		cacheTTL: 1 * time.Millisecond,
		logger:   testLogger(),
	}

	// Populate cache.
	mc.Fetch(context.Background())
	time.Sleep(5 * time.Millisecond)

	// Shut down server.
	srv.Close()

	// Should return stale cache.
	got := mc.Fetch(context.Background())
	if got == nil {
		t.Fatal("expected stale cache, got nil")
	}
	if got.Sources[0].Name != "stale-repo" {
		t.Errorf("expected stale-repo, got %q", got.Sources[0].Name)
	}
}

func TestFormatForPrompt_Empty(t *testing.T) {
	mc := &ManifestClient{
		baseURL:  "http://127.0.0.1:1", // nothing listening
		cacheTTL: defaultCacheTTL,
		logger:   testLogger(),
	}
	if s := mc.FormatForPrompt(context.Background()); s != "" {
		t.Errorf("expected empty string, got %q", s)
	}
}

func TestFormatForPrompt_WithSources(t *testing.T) {
	// Use a server that returns the manifest so FormatForPrompt's internal Fetch works.
	payload := ManifestPayload{
		Sources: []SourceManifest{
			{Name: "myapp", Type: "git_repo", Description: "Main application", Status: "active", EntityTypes: []string{"file", "function"}},
			{Name: "specs", Type: "document", Description: "API specs", Status: "indexing"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	mc := &ManifestClient{
		baseURL:  srv.URL,
		cacheTTL: 1 * time.Hour,
		logger:   testLogger(),
	}

	got := mc.FormatForPrompt(context.Background())
	if got == "" {
		t.Fatal("expected non-empty prompt text")
	}

	for _, want := range []string{
		"Available Knowledge Sources",
		"myapp (git_repo)",
		"Main application",
		"file, function",
		"specs (document) [indexing]",
		"API specs",
	} {
		if !contains(got, want) {
			t.Errorf("prompt missing %q:\n%s", want, got)
		}
	}
}

func TestRefresh(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		json.NewEncoder(w).Encode(ManifestPayload{
			Sources: []SourceManifest{{Name: "repo"}},
		})
	}))
	defer srv.Close()

	mc := &ManifestClient{
		baseURL:  srv.URL,
		cacheTTL: 1 * time.Hour,
		logger:   testLogger(),
	}

	mc.Refresh(context.Background())
	mc.Refresh(context.Background())

	if calls != 2 {
		t.Errorf("expected 2 HTTP calls from Refresh, got %d", calls)
	}

	// Cache should be populated after refresh.
	if mc.cached == nil {
		t.Error("expected cache populated after Refresh")
	}
}

func testLogger() *slog.Logger {
	return slog.Default()
}
