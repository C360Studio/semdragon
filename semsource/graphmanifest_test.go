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
		"source.content.language": 10,
		"source.content.text":    5,
		"source.metadata.path":   3,
		"doc.section.title":      2,
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
	if len(got.PredicateFamilies) != 2 {
		t.Errorf("expected 2 families (source, doc), got %d: %v", len(got.PredicateFamilies), got.PredicateFamilies)
	}
	if got.PredicateFamilies["source"] != 18 {
		t.Errorf("expected source family count 18, got %d", got.PredicateFamilies["source"])
	}
	if got.PredicateFamilies["doc"] != 2 {
		t.Errorf("expected doc family count 2, got %d", got.PredicateFamilies["doc"])
	}

	// Verify two-level categories.
	if got.PredicateCategories["source.content"] != 15 {
		t.Errorf("expected source.content count 15, got %d", got.PredicateCategories["source.content"])
	}
	if got.PredicateCategories["source.metadata"] != 3 {
		t.Errorf("expected source.metadata count 3, got %d", got.PredicateCategories["source.metadata"])
	}
}

func TestGraphManifestFetch_FiltersGamePredicates(t *testing.T) {
	srv := newGraphManifestServer(t, makePredicatesResponse(map[string]int{
		"quest.lifecycle.posted":    10,
		"quest.lifecycle.completed": 5,
		"agent.progression.xp":     3,
		"battle.review.verdict":    2,
		"source.content.language":  8,
		"doc.section.title":        4,
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

	// Game predicates (quest, agent, battle) should be filtered out.
	if _, ok := got.PredicateFamilies["quest"]; ok {
		t.Error("quest family should be filtered out")
	}
	if _, ok := got.PredicateFamilies["agent"]; ok {
		t.Error("agent family should be filtered out")
	}
	if _, ok := got.PredicateFamilies["battle"]; ok {
		t.Error("battle family should be filtered out")
	}

	// Non-game predicates should remain.
	if got.PredicateFamilies["source"] != 8 {
		t.Errorf("expected source count 8, got %d", got.PredicateFamilies["source"])
	}
	if got.PredicateFamilies["doc"] != 4 {
		t.Errorf("expected doc count 4, got %d", got.PredicateFamilies["doc"])
	}
	if got.TotalPredicates != 2 {
		t.Errorf("expected 2 filtered predicates, got %d", got.TotalPredicates)
	}
}

func TestGraphManifestFetch_AllGamePredicates_EmptyManifest(t *testing.T) {
	srv := newGraphManifestServer(t, makePredicatesResponse(map[string]int{
		"quest.lifecycle.posted": 10,
		"agent.progression.xp":  3,
		"battle.review.verdict": 2,
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
	if len(got.PredicateFamilies) != 0 {
		t.Errorf("expected empty families when all are game predicates, got %v", got.PredicateFamilies)
	}

	// FormatForPrompt should return "" for empty manifest.
	c.cached = got
	c.cachedAt = time.Now()
	if s := c.FormatForPrompt(context.Background()); s != "" {
		t.Errorf("expected empty prompt for all-game manifest, got %q", s)
	}
}

func TestGraphManifestFetch_PredicateWithoutDot(t *testing.T) {
	srv := newGraphManifestServer(t, makePredicatesResponse(map[string]int{
		"standalone":              7,
		"source.content.language": 3,
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
	c.cached = &GraphManifest{
		PredicateFamilies:   map[string]int{"source": 1},
		PredicateCategories: map[string]int{"source.content": 1},
		TotalPredicates:     1,
	}
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
	c.cached = &GraphManifest{
		PredicateFamilies:   map[string]int{"source": 1},
		PredicateCategories: map[string]int{"source.content": 1},
		TotalPredicates:     1,
	}
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
		PredicateFamilies:   map[string]int{"source": 10, "doc": 3},
		PredicateCategories: map[string]int{"source.content": 7, "source.metadata": 3, "doc.section": 3},
		TotalPredicates:     5,
	}
	c.cachedAt = time.Now()

	got := c.FormatForPrompt(context.Background())
	if got == "" {
		t.Fatal("expected non-empty prompt text")
	}

	for _, want := range []string{
		"Graph Contents",
		"source (10): content, metadata",
		"doc (3): section",
		"graph_search",
		"predicate",
		"prefix",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt missing %q:\n%s", want, got)
		}
	}

	// Game-world families should NOT appear.
	for _, blocked := range []string{"agent", "quest", "battle", "party", "guild", "store", "review"} {
		if strings.Contains(got, blocked) {
			t.Errorf("prompt should not contain game family %q:\n%s", blocked, got)
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
