package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
)

// TestBuiltinToolTierAlignment verifies that each tool registered by RegisterBuiltins
// enforces the trust tier documented in the trust tier table.
//
// Tier intent by tool category:
//
//	Apprentice (1-5) — read-only operations safe for any agent
//	Journeyman (6-10) — staging writes and external API access
//	Expert    (11-15) — production file writes, test execution
//	Master    (16-18) — unrestricted shell, party lead operations
func TestBuiltinToolTierAlignment(t *testing.T) {
	t.Parallel()

	cases := []struct {
		tool     string
		wantTier domain.TrustTier
		reason   string
	}{
		// Apprentice — terminal tools safe for every agent.
		{tool: "submit_work", wantTier: domain.TierApprentice, reason: "all tiers can submit work"},
		{tool: "ask_clarification", wantTier: domain.TierApprentice, reason: "all tiers can ask questions"},
		{tool: "submit_findings", wantTier: domain.TierApprentice, reason: "all tiers can submit explore findings"},

		// Journeyman — network access and shell execution require demonstrated trust.
		{tool: "http_request", wantTier: domain.TierJourneyman, reason: "network access requires level 6+"},
		{tool: "bash", wantTier: domain.TierJourneyman, reason: "sandbox-constrained shell execution"},

		// Master — party-lead DAG operations require level 16+.
		{tool: "decompose_quest", wantTier: domain.TierMaster, reason: "only party leads (Master+) can decompose quests"},
		{tool: "review_sub_quest", wantTier: domain.TierMaster, reason: "only party leads (Master+) can review sub-quests"},
		{tool: "answer_clarification", wantTier: domain.TierMaster, reason: "only party leads (Master+) can answer clarifications"},
	}

	reg := NewToolRegistry()
	reg.RegisterBuiltins()

	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			t.Parallel()

			tool := reg.Get(tc.tool)
			if tool == nil {
				t.Fatalf("tool %q not found in registry after RegisterBuiltins", tc.tool)
			}

			if tool.MinTier != tc.wantTier {
				t.Errorf(
					"tool %q MinTier = %s (%d), want %s (%d): %s",
					tc.tool,
					tool.MinTier, tool.MinTier,
					tc.wantTier, tc.wantTier,
					tc.reason,
				)
			}
		})
	}
}

// TestBuiltinToolCount asserts that the total number of tools registered by
// RegisterBuiltins matches the expected count. A mismatch here means a tool
// was added (or removed) from RegisterBuiltins without updating
// TestBuiltinToolTierAlignment — update both together.
func TestBuiltinToolCount(t *testing.T) {
	t.Parallel()

	// RegisterBuiltins registers:
	//   submit_work, ask_clarification, submit_findings — 3 terminal tools (Apprentice)
	//   http_request, bash                              — 2 Journeyman tools
	//   decompose_quest, review_sub_quest,
	//   answer_clarification                            — 3 DAG tools (Master)
	//
	// web_search is excluded — registered conditionally via RegisterWebSearch.
	// graph_query is excluded — requires a live EntityQueryFunc (RegisterGraphQuery).
	// explore is excluded — registered separately via RegisterExplore (questtools Start).
	const wantCount = 8

	reg := NewToolRegistry()
	reg.RegisterBuiltins()

	got := len(reg.ListAll())
	if got != wantCount {
		t.Errorf(
			"RegisterBuiltins registered %d tools, want %d; "+
				"update TestBuiltinToolTierAlignment to cover any new tools",
			got, wantCount,
		)
	}
}

// mockSearchProvider is a test SearchProvider that returns canned results.
type mockSearchProvider struct {
	results []SearchResult
	err     error
}

func (m *mockSearchProvider) Name() string { return "mock" }

func (m *mockSearchProvider) Search(_ context.Context, _ string, _ int) ([]SearchResult, error) {
	return m.results, m.err
}

// TestWebSearchHandler verifies that web_search calls the provider and formats results.
// web_search is registered via RegisterWebSearch (not RegisterBuiltins).
func TestWebSearchHandler(t *testing.T) {
	t.Parallel()

	// web_search requires SkillResearch — use Apprentice tier which is sufficient.
	agent := &agentprogression.Agent{
		Tier: domain.TierApprentice,
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			domain.SkillResearch: {Level: domain.ProficiencyNovice},
		},
	}
	quest := &domain.Quest{}

	t.Run("valid query returns formatted results", func(t *testing.T) {
		t.Parallel()
		reg := NewToolRegistry()
		reg.RegisterWebSearch(&mockSearchProvider{
			results: []SearchResult{
				{Title: "Go Concurrency Patterns", URL: "https://go.dev/blog/concurrency", Description: "Blog post about Go concurrency"},
			},
		})

		call := makeToolCall("web_search", map[string]any{"query": "Go concurrency patterns"})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		assertContains(t, result.Content, "Go Concurrency Patterns")
		assertContains(t, result.Content, "https://go.dev/blog/concurrency")
	})

	t.Run("provider error is surfaced", func(t *testing.T) {
		t.Parallel()
		reg := NewToolRegistry()
		reg.RegisterWebSearch(&mockSearchProvider{
			err: fmt.Errorf("API rate limited"),
		})

		call := makeToolCall("web_search", map[string]any{"query": "test"})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected error, got none")
		}
		assertContains(t, result.Error, "API rate limited")
	})

	t.Run("empty query returns argument error", func(t *testing.T) {
		t.Parallel()
		reg := NewToolRegistry()
		reg.RegisterWebSearch(&mockSearchProvider{})

		call := makeToolCall("web_search", map[string]any{"query": ""})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected error for empty query, got none")
		}
		assertContains(t, result.Error, "query argument is required")
	})

	t.Run("no results returns empty message", func(t *testing.T) {
		t.Parallel()
		reg := NewToolRegistry()
		reg.RegisterWebSearch(&mockSearchProvider{results: nil})

		call := makeToolCall("web_search", map[string]any{"query": "nonexistent topic"})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		assertContains(t, result.Content, "No results found")
	})
}

// TestSubmitWorkProductHandler verifies the submit_work terminal tool:
// valid submissions produce JSON with type=work_product and StopLoop=true;
// summary is required; deliverable is optional (for file-based work).
func TestSubmitWorkProductHandler(t *testing.T) {
	t.Parallel()

	reg := NewToolRegistry()
	reg.RegisterBuiltins()

	agent := &agentprogression.Agent{Tier: domain.TierApprentice}
	quest := &domain.Quest{}

	cases := []struct {
		name           string
		args           map[string]any
		wantErr        string // non-empty means we expect an error containing this substring
		wantType       string // expected "type" field in JSON
		wantSummary    bool   // whether "summary" key must be present
		wantDelivrable bool   // whether "deliverable" key must be present
		stopLoop       bool   // expected StopLoop value on success
	}{
		{
			name: "deliverable with summary",
			args: map[string]any{
				"deliverable": "Here is the code",
				"summary":     "Implemented feature",
			},
			wantType:       "work_product",
			wantSummary:    true,
			wantDelivrable: true,
			stopLoop:       true,
		},
		{
			name: "summary only (file-based work)",
			args: map[string]any{
				"summary": "Built auth module with JWT and wrote tests",
			},
			wantType:       "work_product",
			wantSummary:    true,
			wantDelivrable: false,
			stopLoop:       true,
		},
		{
			name: "deliverable only (legacy compat)",
			args: map[string]any{
				"deliverable": "result content",
			},
			wantType:       "work_product",
			wantSummary:    false,
			wantDelivrable: true,
			stopLoop:       true,
		},
		{
			name:    "empty summary and empty deliverable",
			args:    map[string]any{"summary": "", "deliverable": ""},
			wantErr: "at least one",
		},
		{
			name:    "no arguments",
			args:    map[string]any{},
			wantErr: "at least one",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			call := makeToolCall("submit_work", tc.args)
			result := reg.Execute(context.Background(), call, quest, agent)

			if tc.wantErr != "" {
				if result.Error == "" {
					t.Fatalf("expected error containing %q, got none", tc.wantErr)
				}
				assertContains(t, result.Error, tc.wantErr)
				return
			}

			if result.Error != "" {
				t.Fatalf("unexpected error: %s", result.Error)
			}
			if !result.StopLoop {
				t.Error("expected StopLoop=true, got false")
			}

			var payload map[string]string
			if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
				t.Fatalf("Content is not valid JSON: %v — content: %s", err, result.Content)
			}
			if payload["type"] != tc.wantType {
				t.Errorf("type = %q, want %q", payload["type"], tc.wantType)
			}
			_, hasSummary := payload["summary"]
			if hasSummary != tc.wantSummary {
				t.Errorf("summary present = %v, want %v", hasSummary, tc.wantSummary)
			}
			_, hasDeliverable := payload["deliverable"]
			if hasDeliverable != tc.wantDelivrable {
				t.Errorf("deliverable present = %v, want %v", hasDeliverable, tc.wantDelivrable)
			}
		})
	}
}

// TestAskClarificationHandler verifies the ask_clarification terminal tool:
// valid questions produce JSON with type=clarification and StopLoop=true;
// missing or empty question returns an error.
func TestAskClarificationHandler(t *testing.T) {
	t.Parallel()

	reg := NewToolRegistry()
	reg.RegisterBuiltins()

	agent := &agentprogression.Agent{Tier: domain.TierApprentice}
	quest := &domain.Quest{}

	cases := []struct {
		name     string
		args     map[string]any
		wantErr  string // non-empty means we expect an error containing this substring
		wantType string // expected "type" field in JSON
		stopLoop bool   // expected StopLoop value on success
	}{
		{
			name:     "valid question",
			args:     map[string]any{"question": "What format?"},
			wantType: "clarification",
			stopLoop: true,
		},
		{
			name:    "empty question",
			args:    map[string]any{"question": ""},
			wantErr: "question",
		},
		{
			name:    "missing question",
			args:    map[string]any{},
			wantErr: "question",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			call := makeToolCall("ask_clarification", tc.args)
			result := reg.Execute(context.Background(), call, quest, agent)

			if tc.wantErr != "" {
				if result.Error == "" {
					t.Fatalf("expected error containing %q, got none", tc.wantErr)
				}
				assertContains(t, result.Error, tc.wantErr)
				return
			}

			if result.Error != "" {
				t.Fatalf("unexpected error: %s", result.Error)
			}
			if !result.StopLoop {
				t.Error("expected StopLoop=true, got false")
			}

			var payload map[string]string
			if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
				t.Fatalf("Content is not valid JSON: %v — content: %s", err, result.Content)
			}
			if payload["type"] != tc.wantType {
				t.Errorf("type = %q, want %q", payload["type"], tc.wantType)
			}
		})
	}
}

// TestRegisterWebSearchConditional verifies that RegisterBuiltins does not include
// web_search, and that RegisterWebSearch adds it to the registry.
// This enforces the contract that web_search is opt-in (requires a provider).
func TestRegisterWebSearchConditional(t *testing.T) {
	t.Parallel()

	t.Run("RegisterBuiltins does not include web_search", func(t *testing.T) {
		t.Parallel()

		reg := NewToolRegistry()
		reg.RegisterBuiltins()

		if tool := reg.Get("web_search"); tool != nil {
			t.Error("web_search should not be registered by RegisterBuiltins, but it was found")
		}
	})

	t.Run("RegisterWebSearch adds web_search to registry", func(t *testing.T) {
		t.Parallel()

		reg := NewToolRegistry()
		reg.RegisterWebSearch(&mockSearchProvider{})

		tool := reg.Get("web_search")
		if tool == nil {
			t.Fatal("web_search should be registered after RegisterWebSearch, but it was not found")
		}
		if tool.Definition.Name != "web_search" {
			t.Errorf("tool name = %q, want %q", tool.Definition.Name, "web_search")
		}
	})
}

// =============================================================================
// httpRequestHandler tests
// =============================================================================

// TestHTTPRequestHandler verifies that http_request validates its arguments
// and enforces SSRF protection (blocking private/loopback addresses).
func TestHTTPRequestHandler(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	reg := NewToolRegistryWithSandbox(tmpDir)
	reg.RegisterBuiltins()

	// http_request requires TierJourneyman and no specific skill.
	agent := &agentprogression.Agent{Tier: domain.TierJourneyman}
	quest := &domain.Quest{}

	t.Run("empty url returns argument error", func(t *testing.T) {
		t.Parallel()
		call := makeToolCall("http_request", map[string]any{
			"url":          "",
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected error for empty url, got none")
		}
		assertContains(t, result.Error, "url argument is required")
	})

	t.Run("url without http prefix returns error", func(t *testing.T) {
		t.Parallel()
		call := makeToolCall("http_request", map[string]any{
			"url":          "ftp://example.com",
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected error for non-http url, got none")
		}
		assertContains(t, result.Error, "url must start with http://")
	})

	t.Run("invalid method returns error", func(t *testing.T) {
		t.Parallel()
		call := makeToolCall("http_request", map[string]any{
			"url":          "https://example.com",
			"method":       "DELETE",
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected error for DELETE method, got none")
		}
		assertContains(t, result.Error, "method must be GET or POST")
	})

	t.Run("localhost is blocked by SSRF protection", func(t *testing.T) {
		t.Parallel()
		// The custom httpToolClient transport blocks private and loopback addresses.
		// 127.0.0.1 is always a loopback address; even if a server were running
		// there the dial would be rejected before any connection is made.
		call := makeToolCall("http_request", map[string]any{
			"url":          "http://127.0.0.1:12345/",
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected SSRF-rejection error for localhost, got none")
		}
		// The error should mention the request failing, not an argument error.
		assertNotContains(t, result.Error, "url argument is required")
		assertNotContains(t, result.Error, "method must be")
	})

	t.Run("tool spec exposes format parameter with all view modes", func(t *testing.T) {
		t.Parallel()
		// Inspect the registered http_request tool's parameter schema to ensure
		// the format argument is wired with the expected enum values. Agents
		// rely on this enum to know which view modes are available; if it
		// regresses we lose the markdown/summary/links/headings/raw API surface
		// silently.
		tool := reg.Get("http_request")
		if tool == nil {
			t.Fatal("http_request not registered")
		}
		props, ok := tool.Definition.Parameters["properties"].(map[string]any)
		if !ok {
			t.Fatalf("properties wrong shape: %T", tool.Definition.Parameters["properties"])
		}
		formatProp, ok := props["format"].(map[string]any)
		if !ok {
			t.Fatal("format parameter missing from http_request schema")
		}
		enum, ok := formatProp["enum"].([]any)
		if !ok {
			t.Fatalf("format.enum wrong shape: %T", formatProp["enum"])
		}
		want := map[string]bool{"markdown": true, "summary": true, "links": true, "headings": true, "raw": true}
		got := map[string]bool{}
		for _, v := range enum {
			s, _ := v.(string)
			got[s] = true
		}
		for k := range want {
			if !got[k] {
				t.Errorf("format enum missing %q; got %v", k, got)
			}
		}
		for k := range got {
			if !want[k] {
				t.Errorf("format enum has unexpected %q", k)
			}
		}
	})
}

// =============================================================================
// runCommandHandler tests
// =============================================================================

// TestRunCommandHandler verifies that run_command executes shell commands and
// returns output on success, and an error on failure.
func TestRunCommandHandler(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	reg := NewToolRegistryWithSandbox(tmpDir)
	reg.RegisterBuiltins()

	agent := &agentprogression.Agent{Tier: domain.TierMaster}
	quest := &domain.Quest{}

	t.Run("successful echo command returns output", func(t *testing.T) {
		t.Parallel()
		call := makeToolCall("bash", map[string]any{
			"command":      "echo hello",
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		assertContains(t, result.Content, "hello")
	})

	t.Run("failing command returns error with output", func(t *testing.T) {
		t.Parallel()
		call := makeToolCall("bash", map[string]any{
			"command":      "exit 1",
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected error for failing command, got none")
		}
		assertContains(t, result.Error, "command failed")
	})
}

// =============================================================================
// graphQueryHandler tests
// =============================================================================

// TestGraphQueryHandler verifies graph_query argument validation, entity type
// enforcement, limit capping, and successful query path via a mock queryFn.
func TestGraphQueryHandler(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Apprentice tier is sufficient — graph_query has no skill requirement.
	agent := &agentprogression.Agent{Tier: domain.TierApprentice}
	quest := &domain.Quest{}

	t.Run("valid query returns formatted results", func(t *testing.T) {
		t.Parallel()
		reg := NewToolRegistryWithSandbox(tmpDir)
		reg.RegisterBuiltins()
		reg.RegisterGraphQuery(func(_ context.Context, entityType string, limit int) (string, error) {
			return fmt.Sprintf("Found 1 %s(s) (limit=%d):\n\n--- test.entity ---\n{}", entityType, limit), nil
		})

		call := makeToolCall("graph_query", map[string]any{
			"entity_type":  "agent",
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		assertContains(t, result.Content, "agent")
	})

	t.Run("invalid entity_type returns error", func(t *testing.T) {
		t.Parallel()
		reg := NewToolRegistryWithSandbox(tmpDir)
		reg.RegisterBuiltins()
		reg.RegisterGraphQuery(func(_ context.Context, _ string, _ int) (string, error) {
			return "should not be called", nil
		})

		call := makeToolCall("graph_query", map[string]any{
			"entity_type":  "spaceship",
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected error for invalid entity_type, got none")
		}
		assertContains(t, result.Error, "invalid entity_type")
	})

	t.Run("empty entity_type returns required error", func(t *testing.T) {
		t.Parallel()
		reg := NewToolRegistryWithSandbox(tmpDir)
		reg.RegisterBuiltins()
		reg.RegisterGraphQuery(func(_ context.Context, _ string, _ int) (string, error) {
			return "should not be called", nil
		})

		call := makeToolCall("graph_query", map[string]any{
			"entity_type":  "",
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error == "" {
			t.Fatal("expected error for empty entity_type, got none")
		}
		assertContains(t, result.Error, "entity_type argument is required")
	})

	t.Run("limit is capped at 100", func(t *testing.T) {
		t.Parallel()
		var capturedLimit int
		reg := NewToolRegistryWithSandbox(tmpDir)
		reg.RegisterBuiltins()
		reg.RegisterGraphQuery(func(_ context.Context, _ string, limit int) (string, error) {
			capturedLimit = limit
			return "ok", nil
		})

		call := makeToolCall("graph_query", map[string]any{
			"entity_type":  "quest",
			"limit":        float64(9999),
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		if capturedLimit != 100 {
			t.Errorf("limit = %d, want 100 (capped)", capturedLimit)
		}
	})

	t.Run("custom limit within range is respected", func(t *testing.T) {
		t.Parallel()
		var capturedLimit int
		reg := NewToolRegistryWithSandbox(tmpDir)
		reg.RegisterBuiltins()
		reg.RegisterGraphQuery(func(_ context.Context, _ string, limit int) (string, error) {
			capturedLimit = limit
			return "ok", nil
		})

		call := makeToolCall("graph_query", map[string]any{
			"entity_type":  "guild",
			"limit":        float64(42),
			"_sandbox_dir": tmpDir,
		})
		result := reg.Execute(context.Background(), call, quest, agent)
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}
		if capturedLimit != 42 {
			t.Errorf("limit = %d, want 42", capturedLimit)
		}
	})
}

// =============================================================================
// FormatEntitySummary tests
// =============================================================================

// TestFormatEntitySummary verifies that FormatEntitySummary returns an empty
// string for an empty slice and returns formatted text for a non-empty slice.
func TestFormatEntitySummary(t *testing.T) {
	t.Parallel()

	t.Run("empty slice returns empty string", func(t *testing.T) {
		t.Parallel()
		result := FormatEntitySummary([]graph.EntityState{}, "agent")
		if result != "" {
			t.Errorf("expected empty string for empty slice, got %q", result)
		}
	})

	t.Run("non-empty slice returns formatted output", func(t *testing.T) {
		t.Parallel()
		entities := []graph.EntityState{
			{
				ID: "c360.prod.game.board1.agent.dragon",
				Triples: []message.Triple{
					{
						Subject:   "c360.prod.game.board1.agent.dragon",
						Predicate: "agent.progression.level",
						Object:    float64(10),
					},
					{
						Subject:   "c360.prod.game.board1.agent.dragon",
						Predicate: "agent.identity.name",
						Object:    "Dragon",
					},
				},
			},
		}
		result := FormatEntitySummary(entities, "agent")
		assertContains(t, result, "c360.prod.game.board1.agent.dragon")
		assertContains(t, result, "agent.progression.level")
		assertContains(t, result, "agent.identity.name")
		assertContains(t, result, "Found 1 agent(s)")
	})
}

// =============================================================================
// GetToolsForQuest tests
// =============================================================================

// TestGetToolsForQuest verifies that GetToolsForQuest correctly filters tools
// by agent tier, agent skills, and quest AllowedTools list.
func TestGetToolsForQuest(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	t.Run("apprentice tier only sees apprentice tools", func(t *testing.T) {
		t.Parallel()
		reg := NewToolRegistryWithSandbox(tmpDir)
		reg.RegisterBuiltins()

		agent := &agentprogression.Agent{Tier: domain.TierApprentice}
		quest := &domain.Quest{}

		tools := reg.GetToolsForQuest(quest, agent)
		names := toolNames(tools)

		// Apprentice sees terminal tools but not Journeyman-gated tools.
		assertContainsStr(t, names, "submit_work")
		assertContainsStr(t, names, "ask_clarification")
		assertNotContainsStr(t, names, "bash") // TierJourneyman required
	})

	t.Run("journeyman tier sees shell and network tools", func(t *testing.T) {
		t.Parallel()
		reg := NewToolRegistryWithSandbox(tmpDir)
		reg.RegisterBuiltins()

		agent := &agentprogression.Agent{Tier: domain.TierJourneyman}
		quest := &domain.Quest{}

		tools := reg.GetToolsForQuest(quest, agent)
		names := toolNames(tools)

		assertContainsStr(t, names, "bash")
		assertContainsStr(t, names, "http_request")
	})

	t.Run("quest AllowedTools restricts available tools", func(t *testing.T) {
		t.Parallel()
		reg := NewToolRegistryWithSandbox(tmpDir)
		reg.RegisterBuiltins()

		agent := &agentprogression.Agent{Tier: domain.TierJourneyman}
		quest := &domain.Quest{AllowedTools: []string{"bash"}}

		tools := reg.GetToolsForQuest(quest, agent)
		names := toolNames(tools)

		assertContainsStr(t, names, "bash")
		assertNotContainsStr(t, names, "http_request") // not in AllowedTools
	})
}

// =============================================================================
// SetSandboxDir / GetSandboxDir tests
// =============================================================================

// TestSandboxDirGetterSetter verifies that SetSandboxDir and GetSandboxDir
// round-trip correctly and that the registry starts with an empty sandbox.
func TestSandboxDirGetterSetter(t *testing.T) {
	t.Parallel()

	reg := NewToolRegistry()

	if got := reg.GetSandboxDir(); got != "" {
		t.Errorf("initial sandbox dir = %q, want empty string", got)
	}

	reg.SetSandboxDir("/tmp/mybox")
	if got := reg.GetSandboxDir(); got != "/tmp/mybox" {
		t.Errorf("after SetSandboxDir, got %q, want %q", got, "/tmp/mybox")
	}
}

// =============================================================================
// containsToolName tests
// =============================================================================

// TestContainsToolName verifies the containsToolName helper.
func TestContainsToolName(t *testing.T) {
	t.Parallel()

	allowed := []string{"bash", "http_request", "submit_work"}

	if !containsToolName(allowed, "bash") {
		t.Error("expected bash to be found in allowed list")
	}
	if containsToolName(allowed, "read_file") {
		t.Error("expected read_file NOT to be found in allowed list")
	}
	if containsToolName([]string{}, "any") {
		t.Error("expected empty list to return false")
	}
}

// =============================================================================
// Helpers
// =============================================================================

// toolNames extracts the name strings from a slice of ToolDefinitions.
func toolNames(tools []agentic.ToolDefinition) []string {
	names := make([]string, len(tools))
	for i, td := range tools {
		names[i] = td.Name
	}
	return names
}

// assertContainsStr checks that a string slice contains the expected element.
func assertContainsStr(t *testing.T, slice []string, want string) {
	t.Helper()
	for _, s := range slice {
		if s == want {
			return
		}
	}
	t.Errorf("expected %q in %v", want, slice)
}

// assertNotContainsStr checks that a string slice does NOT contain the element.
func assertNotContainsStr(t *testing.T, slice []string, want string) {
	t.Helper()
	for _, s := range slice {
		if s == want {
			t.Errorf("expected %q NOT to be in %v", want, slice)
			return
		}
	}
}

// mustWriteFile creates intermediate directories and writes content to path.
func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mustWriteFile MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("mustWriteFile WriteFile: %v", err)
	}
}

// makeToolCall constructs a minimal agentic.ToolCall for testing handlers directly.
func makeToolCall(name string, args map[string]any) agentic.ToolCall {
	return agentic.ToolCall{
		ID:        "test-call-id",
		Name:      name,
		Arguments: args,
	}
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected %q to contain %q", s, substr)
	}
}

func assertNotContains(t *testing.T, s, substr string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Errorf("expected %q NOT to contain %q", s, substr)
	}
}
