package questbridge

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGraphSourceRoutingUsesCanonicalSegmentBoundaries(t *testing.T) {
	registry := newRoutingTestRegistry()
	setGraphSourceReady(t, registry, "osh", true)
	setGraphSourceReady(t, registry, "workspace", true)
	setGraphSourceReady(t, registry, "other", true)

	tests := []struct {
		name       string
		queryType  string
		value      string
		wantSource string
	}{
		{"entity one segment descendant", "entity", "osh.platform.domain.system.type.instance", "osh"},
		{"entity one segment collision", "entity", "oshx.platform.domain.system.type.instance", "local"},
		{"relationships one segment descendant", "relationships", "osh.platform.domain.system.type.instance", "osh"},
		{"relationships one segment collision", "relationships", "oshx.platform.domain.system.type.instance", "local"},
		{"entity two segments descendant", "entity", "local.semsource.domain.system.type.instance", "workspace"},
		{"entity two segments collision", "entity", "local.semsourcex.domain.system.type.instance", "local"},
		{"relationships two segments descendant", "relationships", "local.semsource.domain.system.type.instance", "workspace"},
		{"relationships two segments collision", "relationships", "local.semsourcex.domain.system.type.instance", "local"},
		{"prefix one segment exact", "prefix", "osh", "osh"},
		{"prefix one segment descendant", "prefix", "osh.asset", "osh"},
		{"prefix one segment collision", "prefix", "oshx", "local"},
		{"prefix two segments exact", "prefix", "local.semsource", "workspace"},
		{"prefix two segments descendant", "prefix", "local.semsource.asset", "workspace"},
		{"prefix two segments collision", "prefix", "local.semsourcex", "local"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var entityID, prefix string
			if tt.queryType == "prefix" {
				prefix = tt.value
			} else {
				entityID = tt.value
			}
			sources := registry.SourcesForQuery(tt.queryType, entityID, prefix)
			if len(sources) != 1 || sources[0].Name != tt.wantSource {
				t.Fatalf("sources = %v, want exactly %q", graphSourceNames(sources), tt.wantSource)
			}
		})
	}
}

func TestGraphSourceRoutingPreservesUnreadyOwnership(t *testing.T) {
	registry := newRoutingTestRegistry()
	setGraphSourceReady(t, registry, "other", true)

	for _, queryType := range []string{"entity", "relationships", "prefix"} {
		t.Run(queryType+" matching unready owner", func(t *testing.T) {
			var entityID, prefix string
			if queryType == "prefix" {
				prefix = "osh.asset"
			} else {
				entityID = "osh.platform.domain.system.type.instance"
			}
			if got := registry.SourcesForQuery(queryType, entityID, prefix); len(got) != 0 {
				t.Fatalf("matching unready owner routed to %v, want no targets", graphSourceNames(got))
			}
		})

		t.Run(queryType+" unowned local fallback", func(t *testing.T) {
			var entityID, prefix string
			if queryType == "prefix" {
				prefix = "unowned.asset"
			} else {
				entityID = "unowned.platform.domain.system.type.instance"
			}
			got := registry.SourcesForQuery(queryType, entityID, prefix)
			if len(got) != 1 || got[0].Name != "local" {
				t.Fatalf("unowned query routed to %v, want exactly local", graphSourceNames(got))
			}
		})
	}
}

func TestFetchExampleIDsForwardsCanonicalPrefixUnchanged(t *testing.T) {
	queries := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var payload struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		queries <- payload.Query
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"entitiesByPrefix":[]}}`)
	}))
	t.Cleanup(server.Close)

	registry := NewGraphSourceRegistry([]GraphSource{{
		Name: "local", Type: "local", GraphQLURL: server.URL, AlwaysQuery: true,
	}}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	for _, prefix := range []string{"osh", "local.semsource"} {
		registry.fetchExampleIDs(context.Background(), &GraphSource{
			Name: prefix, Type: "semsource", EntityPrefix: prefix,
		})
		var query string
		select {
		case query = <-queries:
		default:
			t.Fatalf("fetchExampleIDs returned without issuing a GraphQL query for %q", prefix)
		}
		want := `entitiesByPrefix(prefix: "` + prefix + `", limit: 50)`
		if !strings.Contains(query, want) {
			t.Errorf("GraphQL query = %q, want canonical prefix fragment %q", query, want)
		}
	}
}

func TestFormatSummaryUsesCanonicalPrefixInDisplayAndExample(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch req.URL.Path {
		case "/summary":
			_, _ = io.WriteString(w, `{
				"namespace":"local.semsource",
				"phase":"ready",
				"total_entities":1,
				"domains":[{
					"domain":"weather",
					"entity_count":1,
					"types":[{"type":"station","count":1}]
				}]
			}`)
		default:
			_, _ = io.WriteString(w, `{"data":{"entitiesByPrefix":[]}}`)
		}
	}))
	t.Cleanup(server.Close)

	registry := NewGraphSourceRegistry([]GraphSource{
		{Name: "local", Type: "local", GraphQLURL: server.URL, AlwaysQuery: true},
		{
			Name: "workspace", Type: "semsource", GraphQLURL: server.URL,
			StatusURL: server.URL + "/status", EntityPrefix: "local.semsource",
		},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	setGraphSourceReady(t, registry, "workspace", true)

	formatted := registry.FormatSummaryForPrompt(context.Background())
	for _, want := range []string{
		"workspace (prefix: local.semsource):",
		`Use "prefix" to scope by source (e.g. "local.semsource.weather")`,
	} {
		if !strings.Contains(formatted, want) {
			t.Errorf("formatted summary missing %q:\n%s", want, formatted)
		}
	}
}

func newRoutingTestRegistry() *GraphSourceRegistry {
	return NewGraphSourceRegistry([]GraphSource{
		{Name: "local", Type: "local", GraphQLURL: "http://local/graphql", AlwaysQuery: true},
		{Name: "osh", Type: "semsource", GraphQLURL: "http://osh/graphql", EntityPrefix: "osh"},
		{Name: "workspace", Type: "semsource", GraphQLURL: "http://workspace/graphql", EntityPrefix: "local.semsource"},
		{Name: "other", Type: "semsource", GraphQLURL: "http://other/graphql", EntityPrefix: "other"},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func setGraphSourceReady(t *testing.T, registry *GraphSourceRegistry, name string, ready bool) {
	t.Helper()
	for _, source := range registry.sources {
		if source.Name == name {
			source.ready.Store(ready)
			return
		}
	}
	t.Fatalf("graph source %q not found", name)
}

func graphSourceNames(sources []*GraphSource) []string {
	names := make([]string, len(sources))
	for i, source := range sources {
		names[i] = source.Name
	}
	return names
}
