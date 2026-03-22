package executor

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
)

// graphQLMockResponse is a convenience type for building mock GraphQL responses
// in test server handlers.
type graphQLMockResponse struct {
	Data   map[string]any `json:"data,omitempty"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors,omitempty"`
}

// newGraphSearchServer starts an httptest.Server that parses the incoming
// GraphQL query and returns the response produced by handlerFn.  The caller
// is responsible for calling srv.Close().
func newGraphSearchServer(t *testing.T, handlerFn func(query string) graphQLMockResponse) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusInternalServerError)
			return
		}
		var gqlReq graphQLRequest
		if err := json.Unmarshal(body, &gqlReq); err != nil {
			http.Error(w, "bad JSON", http.StatusBadRequest)
			return
		}
		resp := handlerFn(gqlReq.Query)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			// Best-effort; response may already be partially written.
			t.Logf("mock server encode error: %v", err)
		}
	}))
}

// agentAndQuest returns a minimal Apprentice agent and empty quest for handler calls.
func agentAndQuest() (*agentprogression.Agent, *domain.Quest) {
	return &agentprogression.Agent{Tier: domain.TierApprentice}, &domain.Quest{}
}

// TestGraphSearchHandler covers argument validation and HTTP interaction for the
// graph_search tool.
func TestGraphSearchHandler(t *testing.T) {
	t.Parallel()

	// --- argument validation (no HTTP server needed) ---

	t.Run("missing query_type", func(t *testing.T) {
		t.Parallel()
		reg := NewToolRegistry()
		reg.RegisterGraphSearch("http://unused.local/graphql")

		agent, quest := agentAndQuest()
		call := makeToolCall("graph_search", map[string]any{})
		result := reg.Execute(context.Background(), call, quest, agent)

		if result.Error == "" {
			t.Fatal("expected error for missing query_type, got none")
		}
		assertContains(t, result.Error, "query_type")
	})

	t.Run("invalid query_type", func(t *testing.T) {
		t.Parallel()
		reg := NewToolRegistry()
		reg.RegisterGraphSearch("http://unused.local/graphql")

		agent, quest := agentAndQuest()
		call := makeToolCall("graph_search", map[string]any{
			"query_type": "bogus",
		})
		result := reg.Execute(context.Background(), call, quest, agent)

		if result.Error == "" {
			t.Fatal("expected error for invalid query_type, got none")
		}
		assertContains(t, result.Error, "bogus")
	})

	t.Run("entity query missing entity_id", func(t *testing.T) {
		t.Parallel()
		reg := NewToolRegistry()
		reg.RegisterGraphSearch("http://unused.local/graphql")

		agent, quest := agentAndQuest()
		call := makeToolCall("graph_search", map[string]any{
			"query_type": "entity",
		})
		result := reg.Execute(context.Background(), call, quest, agent)

		if result.Error == "" {
			t.Fatal("expected error for missing entity_id, got none")
		}
		assertContains(t, result.Error, "entity_id")
	})

	t.Run("prefix query missing prefix", func(t *testing.T) {
		t.Parallel()
		reg := NewToolRegistry()
		reg.RegisterGraphSearch("http://unused.local/graphql")

		agent, quest := agentAndQuest()
		call := makeToolCall("graph_search", map[string]any{
			"query_type": "prefix",
		})
		result := reg.Execute(context.Background(), call, quest, agent)

		if result.Error == "" {
			t.Fatal("expected error for missing prefix, got none")
		}
		assertContains(t, result.Error, "prefix")
	})

	t.Run("predicate query missing predicate", func(t *testing.T) {
		t.Parallel()
		reg := NewToolRegistry()
		reg.RegisterGraphSearch("http://unused.local/graphql")

		agent, quest := agentAndQuest()
		call := makeToolCall("graph_search", map[string]any{
			"query_type": "predicate",
		})
		result := reg.Execute(context.Background(), call, quest, agent)

		if result.Error == "" {
			t.Fatal("expected error for missing predicate, got none")
		}
		assertContains(t, result.Error, "predicate")
	})

	t.Run("relationships query missing entity_id", func(t *testing.T) {
		t.Parallel()
		reg := NewToolRegistry()
		reg.RegisterGraphSearch("http://unused.local/graphql")

		agent, quest := agentAndQuest()
		call := makeToolCall("graph_search", map[string]any{
			"query_type": "relationships",
		})
		result := reg.Execute(context.Background(), call, quest, agent)

		if result.Error == "" {
			t.Fatal("expected error for missing entity_id in relationships query, got none")
		}
		assertContains(t, result.Error, "entity_id")
	})

	t.Run("search query missing search_text", func(t *testing.T) {
		t.Parallel()
		reg := NewToolRegistry()
		reg.RegisterGraphSearch("http://unused.local/graphql")

		agent, quest := agentAndQuest()
		call := makeToolCall("graph_search", map[string]any{
			"query_type": "search",
		})
		result := reg.Execute(context.Background(), call, quest, agent)

		if result.Error == "" {
			t.Fatal("expected error for missing search_text, got none")
		}
		assertContains(t, result.Error, "search_text")
	})

	// --- successful HTTP interactions ---

	t.Run("entity query success", func(t *testing.T) {
		t.Parallel()

		srv := newGraphSearchServer(t, func(_ string) graphQLMockResponse {
			// The entity query uses the "entity" GraphQL field with typed triples.
			return graphQLMockResponse{
				Data: map[string]any{
					"entity": map[string]any{
						"id": "c360.prod.game.board1.quest.abc123",
						"triples": []any{
							map[string]any{"predicate": "quest.lifecycle.status", "object": "posted"},
							map[string]any{"predicate": "quest.identity.title", "object": "Test Quest"},
						},
					},
				},
			}
		})
		defer srv.Close()

		reg := NewToolRegistry()
		reg.RegisterGraphSearch(srv.URL)

		agent, quest := agentAndQuest()
		call := makeToolCall("graph_search", map[string]any{
			"query_type": "entity",
			"entity_id":  "c360.prod.game.board1.quest.abc123",
		})
		result := reg.Execute(context.Background(), call, quest, agent)

		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		assertContains(t, result.Content, "c360.prod.game.board1.quest.abc123")
		assertContains(t, result.Content, "Entity:")
		assertContains(t, result.Content, "status: posted")
		assertContains(t, result.Content, "title: Test Quest")
	})

	t.Run("search query success", func(t *testing.T) {
		t.Parallel()

		srv := newGraphSearchServer(t, func(_ string) graphQLMockResponse {
			return graphQLMockResponse{
				Data: map[string]any{
					"globalSearch": map[string]any{
						"entities": []any{
							map[string]any{"id": "c360.prod.game.board1.quest.abc123", "type": "quest"},
							map[string]any{"id": "c360.prod.game.board1.agent.dragon", "type": "agent"},
						},
						"count": 2,
					},
				},
			}
		})
		defer srv.Close()

		reg := NewToolRegistry()
		reg.RegisterGraphSearch(srv.URL)

		agent, quest := agentAndQuest()
		call := makeToolCall("graph_search", map[string]any{
			"query_type":  "search",
			"search_text": "dragon quest",
		})
		result := reg.Execute(context.Background(), call, quest, agent)

		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		assertContains(t, result.Content, "Entities")
		assertContains(t, result.Content, "c360.prod.game.board1.quest.abc123")
		assertContains(t, result.Content, "c360.prod.game.board1.agent.dragon")
	})

	t.Run("globalSearch with answer field prioritized", func(t *testing.T) {
		t.Parallel()

		srv := newGraphSearchServer(t, func(_ string) graphQLMockResponse {
			return graphQLMockResponse{
				Data: map[string]any{
					"globalSearch": map[string]any{
						"answer":       "Input validation best practices include allowlist-based filtering, boundary-layer sanitization, and schema conformance checks.",
						"answer_model": "claude-haiku",
						"entity_digests": []any{
							map[string]any{"id": "osh.code.function.validate_input", "type": "function", "label": "validate_input()", "relevance": 0.95},
							map[string]any{"id": "osh.code.function.sanitize_html", "type": "function", "label": "sanitize_html()", "relevance": 0.82},
						},
						"community_summaries": []any{
							map[string]any{
								"communityId":  "c1",
								"summary":      "Input validation patterns cluster around boundary-layer checks.",
								"keywords":     []any{"validation", "sanitization"},
								"relevance":    0.9,
								"member_count": 12,
								"entities": []any{
									map[string]any{"id": "osh.code.function.validate_input", "type": "function", "label": "validate_input()", "relevance": 0.95},
								},
							},
						},
						"entities": []any{
							map[string]any{"id": "osh.code.function.validate_input", "type": "function"},
						},
						"count": 12,
					},
				},
			}
		})
		defer srv.Close()

		reg := NewToolRegistry()
		reg.RegisterGraphSearch(srv.URL)

		agent, quest := agentAndQuest()
		call := makeToolCall("graph_search", map[string]any{
			"query_type":  "nlq",
			"search_text": "how does input validation work",
		})
		result := reg.Execute(context.Background(), call, quest, agent)

		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		// Answer should be the primary content.
		assertContains(t, result.Content, "Input validation best practices")
		// Community representative entities should appear for follow-up.
		assertContains(t, result.Content, "validate_input()")
		assertContains(t, result.Content, "12 in cluster")
		// Bare entity IDs should NOT appear when answer is present.
		if strings.Contains(result.Content, "[function] osh.code.function.validate_input") {
			t.Error("bare entity IDs should not appear when answer field is present")
		}
	})

	t.Run("globalSearch with community_summaries fallback (no answer)", func(t *testing.T) {
		t.Parallel()

		srv := newGraphSearchServer(t, func(_ string) graphQLMockResponse {
			return graphQLMockResponse{
				Data: map[string]any{
					"globalSearch": map[string]any{
						"community_summaries": []any{
							map[string]any{
								"communityId":  "c1",
								"summary":      "Validation utilities handle input sanitization across the codebase.",
								"keywords":     []any{"validation"},
								"relevance":    0.85,
								"member_count": 8,
								"entities": []any{
									map[string]any{"id": "osh.code.struct.Validator", "type": "struct", "label": "Validator", "relevance": 0.9},
								},
							},
						},
						"entities": []any{
							map[string]any{"id": "osh.code.struct.Validator", "type": "struct"},
						},
						"count": 8,
					},
				},
			}
		})
		defer srv.Close()

		reg := NewToolRegistry()
		reg.RegisterGraphSearch(srv.URL)

		agent, quest := agentAndQuest()
		call := makeToolCall("graph_search", map[string]any{
			"query_type":  "search",
			"search_text": "validator",
		})
		result := reg.Execute(context.Background(), call, quest, agent)

		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		// Community summary should be shown as primary content (no answer field).
		assertContains(t, result.Content, "Validation utilities")
		// Representative entity with label should appear.
		assertContains(t, result.Content, "Validator")
		assertContains(t, result.Content, "8 in cluster")
	})

	t.Run("globalSearch with entity_digests fallback (no answer, no communities)", func(t *testing.T) {
		t.Parallel()

		srv := newGraphSearchServer(t, func(_ string) graphQLMockResponse {
			return graphQLMockResponse{
				Data: map[string]any{
					"globalSearch": map[string]any{
						"entity_digests": []any{
							map[string]any{"id": "osh.code.function.parse_json", "type": "function", "label": "parse_json()", "relevance": 0.88},
							map[string]any{"id": "osh.code.function.parse_xml", "type": "function", "label": "parse_xml()", "relevance": 0.72},
						},
						"entities": []any{
							map[string]any{"id": "osh.code.function.parse_json", "type": "function"},
							map[string]any{"id": "osh.code.function.parse_xml", "type": "function"},
						},
						"count": 2,
					},
				},
			}
		})
		defer srv.Close()

		reg := NewToolRegistry()
		reg.RegisterGraphSearch(srv.URL)

		agent, quest := agentAndQuest()
		call := makeToolCall("graph_search", map[string]any{
			"query_type":  "search",
			"search_text": "parser",
		})
		result := reg.Execute(context.Background(), call, quest, agent)

		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		// Entity digests with labels should appear.
		assertContains(t, result.Content, "parse_json()")
		assertContains(t, result.Content, "parse_xml()")
		assertContains(t, result.Content, "Matched entities")
	})

	t.Run("graphql error response", func(t *testing.T) {
		t.Parallel()

		srv := newGraphSearchServer(t, func(_ string) graphQLMockResponse {
			return graphQLMockResponse{
				Errors: []struct {
					Message string `json:"message"`
				}{
					{Message: "entity not found in graph"},
				},
			}
		})
		defer srv.Close()

		reg := NewToolRegistry()
		reg.RegisterGraphSearch(srv.URL)

		agent, quest := agentAndQuest()
		call := makeToolCall("graph_search", map[string]any{
			"query_type": "entity",
			"entity_id":  "c360.prod.game.board1.quest.nonexistent",
		})
		result := reg.Execute(context.Background(), call, quest, agent)

		if result.Error == "" {
			t.Fatal("expected error from GraphQL errors response, got none")
		}
		assertContains(t, result.Error, "entity not found in graph")
	})

	t.Run("http error", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}))
		defer srv.Close()

		reg := NewToolRegistry()
		reg.RegisterGraphSearch(srv.URL)

		agent, quest := agentAndQuest()
		call := makeToolCall("graph_search", map[string]any{
			"query_type": "entity",
			"entity_id":  "c360.prod.game.board1.quest.abc123",
		})
		result := reg.Execute(context.Background(), call, quest, agent)

		if result.Error == "" {
			t.Fatal("expected error for HTTP 500, got none")
		}
		assertContains(t, result.Error, "500")
	})

	t.Run("limit capping", func(t *testing.T) {
		t.Parallel()

		// The mock captures the decoded query so we can inspect the inline limit.
		var capturedQuery string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			var gqlReq graphQLRequest
			_ = json.Unmarshal(body, &gqlReq)
			capturedQuery = gqlReq.Query

			resp := graphQLMockResponse{
				Data: map[string]any{
					"entitiesByPrefix": []any{},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer srv.Close()

		reg := NewToolRegistry()
		reg.RegisterGraphSearch(srv.URL)

		agent, quest := agentAndQuest()
		call := makeToolCall("graph_search", map[string]any{
			"query_type": "prefix",
			"prefix":     "c360.prod.game",
			"limit":      float64(999), // far above the 100 cap
		})
		result := reg.Execute(context.Background(), call, quest, agent)

		// The call itself should succeed (empty result is fine).
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}

		// Verify the capped limit appears in the inline query (should be 100, not 999).
		if capturedQuery == "" {
			t.Fatal("mock server did not capture query")
		}
		if !strings.Contains(capturedQuery, "limit: 100") {
			t.Errorf("query should contain capped limit of 100, got: %s", capturedQuery)
		}
	})

	t.Run("search maxCommunities capped at 5", func(t *testing.T) {
		t.Parallel()

		var capturedQuery string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			var gqlReq graphQLRequest
			_ = json.Unmarshal(body, &gqlReq)
			capturedQuery = gqlReq.Query

			resp := graphQLMockResponse{
				Data: map[string]any{
					"globalSearch": map[string]any{
						"entities": []any{},
						"count":    0,
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer srv.Close()

		reg := NewToolRegistry()
		reg.RegisterGraphSearch(srv.URL)

		agent, quest := agentAndQuest()
		call := makeToolCall("graph_search", map[string]any{
			"query_type":  "search",
			"search_text": "test query",
			"limit":       float64(50), // above the maxCommunities cap of 5
		})
		result := reg.Execute(context.Background(), call, quest, agent)

		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}

		if capturedQuery == "" {
			t.Fatal("mock server did not capture query")
		}
		if !strings.Contains(capturedQuery, "maxCommunities: 5") {
			t.Errorf("query should contain maxCommunities: 5, got: %s", capturedQuery)
		}
	})
}
