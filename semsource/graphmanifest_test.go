package semsource

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewGraphManifestClient_EmptyURL(t *testing.T) {
	c := NewGraphManifestClient("", nil)
	if c != nil {
		t.Error("expected nil for empty URL")
	}
}

func TestNewGraphManifestClient_WhitespaceURL(t *testing.T) {
	c := NewGraphManifestClient("   ", nil)
	if c != nil {
		t.Error("expected nil for whitespace URL")
	}
}

func TestGraphManifestFetch_Success(t *testing.T) {
	srv := newGraphManifestServer(t, makePredicatesResponse(map[string]int{
		"quest.lifecycle.posted":    10,
		"quest.lifecycle.completed": 5,
		"agent.progression.xp":     3,
		"battle.review.verdict":    2,
	}))
	defer srv.Close()

	c := &GraphManifestClient{
		graphqlURL: srv.URL,
		logger:     testLogger(),
	}

	got := c.Fetch(context.Background())
	if got == nil {
		t.Fatal("expected non-nil manifest")
	}
	if got.TotalPredicates != 4 {
		t.Errorf("expected 4 total predicates, got %d", got.TotalPredicates)
	}
	if len(got.PredicateFamilies) != 3 {
		t.Errorf("expected 3 families (quest, agent, battle), got %d", len(got.PredicateFamilies))
	}
	if got.PredicateFamilies["quest"] != 15 {
		t.Errorf("expected quest family count 15, got %d", got.PredicateFamilies["quest"])
	}
	if got.PredicateFamilies["agent"] != 3 {
		t.Errorf("expected agent family count 3, got %d", got.PredicateFamilies["agent"])
	}
}

func TestGraphManifestFetch_PredicateWithoutDot(t *testing.T) {
	srv := newGraphManifestServer(t, makePredicatesResponse(map[string]int{
		"standalone": 7,
		"quest.lifecycle.posted": 3,
	}))
	defer srv.Close()

	c := &GraphManifestClient{
		graphqlURL: srv.URL,
		logger:     testLogger(),
	}

	got := c.Fetch(context.Background())
	if got == nil {
		t.Fatal("expected non-nil manifest")
	}
	if got.PredicateFamilies["standalone"] != 7 {
		t.Errorf("expected standalone family count 7, got %d", got.PredicateFamilies["standalone"])
	}
}

func TestGraphManifestFetch_Cached(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		json.NewEncoder(w).Encode(predicatesResponse{})
	}))
	defer srv.Close()

	c := &GraphManifestClient{
		graphqlURL: srv.URL,
		logger:     testLogger(),
	}
	// Prime cache.
	c.cached = &GraphManifest{PredicateFamilies: map[string]int{"quest": 1}, TotalPredicates: 1}
	c.cachedAt = time.Now()

	got := c.Fetch(context.Background())
	if got == nil {
		t.Fatal("expected cached manifest")
	}
	if calls != 0 {
		t.Errorf("expected 0 HTTP calls (cached), got %d", calls)
	}
}

func TestGraphManifestFetch_ServerDown_ReturnsStaleCacheIfAvailable(t *testing.T) {
	c := &GraphManifestClient{
		graphqlURL: "http://127.0.0.1:1", // nothing listening
		logger:     testLogger(),
	}
	c.cached = &GraphManifest{PredicateFamilies: map[string]int{"quest": 1}, TotalPredicates: 1}
	c.cachedAt = time.Time{} // expired

	got := c.Fetch(context.Background())
	if got == nil {
		t.Fatal("expected stale cache, got nil")
	}
}

func TestGraphManifestFetch_GraphQLErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data":   nil,
			"errors": []map[string]string{{"message": "field not found"}},
		})
	}))
	defer srv.Close()

	c := &GraphManifestClient{
		graphqlURL: srv.URL,
		logger:     testLogger(),
	}

	got := c.Fetch(context.Background())
	if got != nil {
		t.Error("expected nil when GraphQL returns errors")
	}
}

func TestGraphManifestFetch_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`not json at all`))
	}))
	defer srv.Close()

	c := &GraphManifestClient{
		graphqlURL: srv.URL,
		logger:     testLogger(),
	}

	got := c.Fetch(context.Background())
	if got != nil {
		t.Error("expected nil for malformed JSON")
	}
}

func TestGraphManifestFormatForPrompt_WithData(t *testing.T) {
	c := &GraphManifestClient{
		graphqlURL: "http://unused",
		logger:     testLogger(),
	}
	c.cached = &GraphManifest{
		PredicateFamilies: map[string]int{"quest": 10, "agent": 3, "battle": 2},
		TotalPredicates:   12,
	}
	c.cachedAt = time.Now()

	got := c.FormatForPrompt(context.Background())
	if got == "" {
		t.Fatal("expected non-empty prompt text")
	}

	for _, want := range []string{
		"Graph Contents",
		"agent",
		"quest",
		"battle",
		"graph_search",
		"12 total predicates",
		"15 entities across 3 predicate families",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt missing %q:\n%s", want, got)
		}
	}
}

func TestGraphManifestFormatForPrompt_Empty(t *testing.T) {
	c := &GraphManifestClient{
		graphqlURL: "http://127.0.0.1:1",
		logger:     testLogger(),
	}
	if s := c.FormatForPrompt(context.Background()); s != "" {
		t.Errorf("expected empty string, got %q", s)
	}
}

func TestGraphManifestFormatForPrompt_NilManifest(t *testing.T) {
	c := &GraphManifestClient{
		graphqlURL: "http://127.0.0.1:1",
		logger:     testLogger(),
	}
	c.cached = &GraphManifest{PredicateFamilies: map[string]int{}}
	c.cachedAt = time.Now()

	if s := c.FormatForPrompt(context.Background()); s != "" {
		t.Errorf("expected empty string for empty families, got %q", s)
	}
}

// makePredicatesResponse builds a predicatesResponse from a predicate→entityCount map.
func makePredicatesResponse(predicates map[string]int) predicatesResponse {
	var resp predicatesResponse
	for pred, count := range predicates {
		resp.Data.Predicates.Predicates = append(resp.Data.Predicates.Predicates, predicateEntry{
			Predicate:   pred,
			EntityCount: count,
		})
	}
	resp.Data.Predicates.Total = len(predicates)
	return resp
}

func newGraphManifestServer(t *testing.T, resp predicatesResponse) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ct := r.Header.Get("Content-Type")
		if !strings.Contains(ct, "application/json") {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}
