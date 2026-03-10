package executor_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/executor"
	"github.com/c360studio/semstreams/agentic"
)

// =============================================================================
// Test helpers
// =============================================================================

// newSandboxTestServer creates a mock sandbox HTTP server with canned handlers.
func newSandboxTestServer(t *testing.T) (*httptest.Server, *executor.SandboxClient) {
	t.Helper()
	mux := http.NewServeMux()

	// GET /file — return file content
	mux.HandleFunc("GET /file", func(w http.ResponseWriter, r *http.Request) {
		questID := r.URL.Query().Get("quest_id")
		path := r.URL.Query().Get("path")
		if questID == "" || path == "" {
			http.Error(w, `{"error":"missing params"}`, http.StatusBadRequest)
			return
		}

		// Return canned content based on path.
		content := "hello from " + path
		if path == "large.txt" {
			// Return a large file to test truncation.
			content = ""
			for i := 0; i < 200000; i++ {
				content += "x"
			}
		}
		if path == "multi-line.go" {
			content = "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"
		}
		if path == "patchable.go" {
			content = "package main\n\nfunc old() {}\n"
		}

		json.NewEncoder(w).Encode(map[string]any{
			"content": content,
			"size":    len(content),
		})
	})

	// PUT /file — accept writes
	mux.HandleFunc("PUT /file", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			QuestID string `json:"quest_id"`
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"bad json"}`, http.StatusBadRequest)
			return
		}
		if req.QuestID == "" || req.Path == "" {
			http.Error(w, `{"error":"missing fields"}`, http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// POST /exec — return canned exec results
	mux.HandleFunc("POST /exec", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			QuestID   string `json:"quest_id"`
			Command   string `json:"command"`
			TimeoutMS int    `json:"timeout_ms"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"bad json"}`, http.StatusBadRequest)
			return
		}

		resp := map[string]any{
			"stdout":    "ok\n",
			"stderr":    "",
			"exit_code": 0,
			"timed_out": false,
		}

		// Simulate test failure for specific commands.
		if req.Command == "go test ./fail/..." {
			resp["stdout"] = "FAIL: TestSomething"
			resp["exit_code"] = 1
		}

		json.NewEncoder(w).Encode(resp)
	})

	// POST /list — return canned directory listing
	mux.HandleFunc("POST /list", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"entries": []map[string]any{
				{"name": "main.go", "is_dir": false, "size": 100},
				{"name": "pkg", "is_dir": true, "size": 0},
				{"name": "README.md", "is_dir": false, "size": 50},
			},
		})
	})

	// POST /search — return canned search output
	mux.HandleFunc("POST /search", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"output": "main.go:1:package main\nmain.go:5:func main() {\n",
		})
	})

	// POST /workspace/{id} — create workspace
	mux.HandleFunc("POST /workspace/{id}", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "created"})
	})

	// DELETE /workspace/{id} — delete workspace
	mux.HandleFunc("DELETE /workspace/{id}", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
	})

	// GET /workspace/{id} — list all files recursively
	mux.HandleFunc("GET /workspace/{id}", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"entries": []map[string]any{
				{"name": "main.go", "is_dir": false, "size": 100},
				{"name": "src/lib.go", "is_dir": false, "size": 200},
				{"name": "src/lib_test.go", "is_dir": false, "size": 150},
				{"name": "README.md", "is_dir": false, "size": 50},
			},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := executor.NewSandboxClient(srv.URL)
	return srv, client
}

// testCall creates a tool call with quest_id metadata.
func testCall(name string, args map[string]any) agentic.ToolCall {
	return agentic.ToolCall{
		ID:   "call-123",
		Name: name,
		Arguments: args,
		Metadata: map[string]any{
			"quest_id": "test-quest-1",
		},
	}
}

// testAgent creates an agent with the given tier and skills.
func testAgent(tier domain.TrustTier, skills ...domain.SkillTag) *agentprogression.Agent {
	profs := make(map[domain.SkillTag]domain.SkillProficiency)
	for _, s := range skills {
		profs[s] = domain.SkillProficiency{Level: 1}
	}
	return &agentprogression.Agent{
		Tier:               tier,
		SkillProficiencies: profs,
	}
}

var testQuest = &domain.Quest{ID: "test-quest-1"}

// =============================================================================
// Workspace management tests
// =============================================================================

func TestSandboxClient_CreateWorkspace(t *testing.T) {
	_, client := newSandboxTestServer(t)
	if err := client.CreateWorkspace(context.Background(), "quest-abc"); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
}

func TestSandboxClient_DeleteWorkspace(t *testing.T) {
	_, client := newSandboxTestServer(t)
	if err := client.DeleteWorkspace(context.Background(), "quest-abc"); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}
}

// =============================================================================
// Sandbox tool handler tests
// =============================================================================

func TestSandboxReadFile(t *testing.T) {
	_, client := newSandboxTestServer(t)
	reg := executor.NewToolRegistry()
	reg.RegisterSandboxTools(client)

	call := testCall("read_file", map[string]any{"path": "main.go"})
	result := reg.Execute(context.Background(), call, testQuest, testAgent(domain.TierApprentice))

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.Content == "" {
		t.Fatal("expected non-empty content")
	}
	if result.Content != "hello from main.go" {
		t.Errorf("got %q, want %q", result.Content, "hello from main.go")
	}
}

func TestSandboxReadFile_MissingQuestID(t *testing.T) {
	_, client := newSandboxTestServer(t)
	reg := executor.NewToolRegistry()
	reg.RegisterSandboxTools(client)

	call := agentic.ToolCall{
		ID:        "call-123",
		Name:      "read_file",
		Arguments: map[string]any{"path": "main.go"},
		// No metadata — quest_id missing
	}
	agent := testAgent(domain.TierApprentice)
	result := reg.Execute(context.Background(), call, testQuest, agent)

	if result.Error == "" {
		t.Fatal("expected error for missing quest_id")
	}
}

func TestSandboxReadFileRange(t *testing.T) {
	_, client := newSandboxTestServer(t)
	reg := executor.NewToolRegistry()
	reg.RegisterSandboxTools(client)

	call := testCall("read_file_range", map[string]any{
		"path":       "multi-line.go",
		"start_line": float64(3),
		"end_line":   float64(5),
	})
	result := reg.Execute(context.Background(), call, testQuest, testAgent(domain.TierApprentice))

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.Content == "" {
		t.Fatal("expected non-empty content")
	}
	// Should contain line numbers.
	if !strings.Contains(result.Content, "3\t") {
		t.Errorf("expected line 3 in output, got: %s", result.Content)
	}
}

func TestSandboxWriteFile(t *testing.T) {
	_, client := newSandboxTestServer(t)
	reg := executor.NewToolRegistry()
	reg.RegisterSandboxTools(client)

	call := testCall("write_file", map[string]any{
		"path":    "output.go",
		"content": "package main\n",
	})
	agent := testAgent(domain.TierExpert, domain.SkillCodeGen)
	result := reg.Execute(context.Background(), call, testQuest, agent)

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Content, "Successfully wrote") {
		t.Errorf("unexpected content: %s", result.Content)
	}
}

func TestSandboxWriteFile_TierGated(t *testing.T) {
	_, client := newSandboxTestServer(t)
	reg := executor.NewToolRegistry()
	reg.RegisterSandboxTools(client)

	call := testCall("write_file", map[string]any{
		"path":    "output.go",
		"content": "package main\n",
	})
	// Apprentice can't write files.
	result := reg.Execute(context.Background(), call, testQuest, testAgent(domain.TierApprentice))
	if result.Error == "" {
		t.Fatal("expected tier gate error")
	}
}

func TestSandboxPatchFile(t *testing.T) {
	_, client := newSandboxTestServer(t)
	reg := executor.NewToolRegistry()
	reg.RegisterSandboxTools(client)

	call := testCall("patch_file", map[string]any{
		"path":     "patchable.go",
		"old_text": "func old() {}",
		"new_text": "func new() {}",
	})
	agent := testAgent(domain.TierJourneyman, domain.SkillCodeGen)
	result := reg.Execute(context.Background(), call, testQuest, agent)

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Content, "Successfully patched") {
		t.Errorf("unexpected content: %s", result.Content)
	}
}

func TestSandboxListDirectory(t *testing.T) {
	_, client := newSandboxTestServer(t)
	reg := executor.NewToolRegistry()
	reg.RegisterSandboxTools(client)

	call := testCall("list_directory", map[string]any{"path": "."})
	result := reg.Execute(context.Background(), call, testQuest, testAgent(domain.TierApprentice))

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Content, "main.go") {
		t.Errorf("expected main.go in output, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "[dir]") {
		t.Errorf("expected [dir] marker in output, got: %s", result.Content)
	}
}

func TestSandboxGlobFiles(t *testing.T) {
	_, client := newSandboxTestServer(t)
	reg := executor.NewToolRegistry()
	reg.RegisterSandboxTools(client)

	call := testCall("glob_files", map[string]any{"pattern": "**/*.go"})
	result := reg.Execute(context.Background(), call, testQuest, testAgent(domain.TierApprentice))

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Content, ".go") {
		t.Errorf("expected .go files in output, got: %s", result.Content)
	}
}

func TestSandboxSearchText(t *testing.T) {
	_, client := newSandboxTestServer(t)
	reg := executor.NewToolRegistry()
	reg.RegisterSandboxTools(client)

	call := testCall("search_text", map[string]any{
		"pattern": "main",
		"path":    ".",
	})
	result := reg.Execute(context.Background(), call, testQuest, testAgent(domain.TierApprentice))

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Content, "main.go") {
		t.Errorf("expected main.go in search results, got: %s", result.Content)
	}
}

func TestSandboxRunTests(t *testing.T) {
	_, client := newSandboxTestServer(t)
	reg := executor.NewToolRegistry()
	reg.RegisterSandboxTools(client)

	call := testCall("run_tests", map[string]any{"command": "go test ./..."})
	agent := testAgent(domain.TierExpert, domain.SkillCodeGen, domain.SkillCodeReview)
	result := reg.Execute(context.Background(), call, testQuest, agent)

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
}

func TestSandboxRunTests_FailedTests(t *testing.T) {
	_, client := newSandboxTestServer(t)
	reg := executor.NewToolRegistry()
	reg.RegisterSandboxTools(client)

	call := testCall("run_tests", map[string]any{"command": "go test ./fail/..."})
	agent := testAgent(domain.TierExpert, domain.SkillCodeGen, domain.SkillCodeReview)
	result := reg.Execute(context.Background(), call, testQuest, agent)

	if result.Error == "" {
		t.Fatal("expected error for failed tests")
	}
	if !strings.Contains(result.Error, "exit code") {
		t.Errorf("expected exit code error, got: %s", result.Error)
	}
}

func TestSandboxRunTests_ShellMetaRejected(t *testing.T) {
	_, client := newSandboxTestServer(t)
	reg := executor.NewToolRegistry()
	reg.RegisterSandboxTools(client)

	call := testCall("run_tests", map[string]any{"command": "go test ./... && rm -rf /"})
	agent := testAgent(domain.TierExpert, domain.SkillCodeGen, domain.SkillCodeReview)
	result := reg.Execute(context.Background(), call, testQuest, agent)

	if result.Error == "" {
		t.Fatal("expected error for shell metacharacters")
	}
}

func TestSandboxRunCommand(t *testing.T) {
	_, client := newSandboxTestServer(t)
	reg := executor.NewToolRegistry()
	reg.RegisterSandboxTools(client)

	call := testCall("run_command", map[string]any{"command": "echo hello"})
	agent := testAgent(domain.TierMaster)
	result := reg.Execute(context.Background(), call, testQuest, agent)

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
}

func TestSandboxRunCommand_TierGated(t *testing.T) {
	_, client := newSandboxTestServer(t)
	reg := executor.NewToolRegistry()
	reg.RegisterSandboxTools(client)

	call := testCall("run_command", map[string]any{"command": "echo hello"})
	// Expert can't run arbitrary commands.
	result := reg.Execute(context.Background(), call, testQuest, testAgent(domain.TierExpert))
	if result.Error == "" {
		t.Fatal("expected tier gate error for Expert running run_command")
	}
}

func TestSandboxDeleteFile(t *testing.T) {
	_, client := newSandboxTestServer(t)
	reg := executor.NewToolRegistry()
	reg.RegisterSandboxTools(client)

	call := testCall("delete_file", map[string]any{"path": "old.go"})
	agent := testAgent(domain.TierJourneyman, domain.SkillCodeGen)
	result := reg.Execute(context.Background(), call, testQuest, agent)

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Content, "Deleted") {
		t.Errorf("expected Deleted in content, got: %s", result.Content)
	}
}

func TestSandboxRenameFile(t *testing.T) {
	_, client := newSandboxTestServer(t)
	reg := executor.NewToolRegistry()
	reg.RegisterSandboxTools(client)

	call := testCall("rename_file", map[string]any{
		"old_path": "old.go",
		"new_path": "new.go",
	})
	agent := testAgent(domain.TierJourneyman, domain.SkillCodeGen)
	result := reg.Execute(context.Background(), call, testQuest, agent)

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Content, "Renamed") {
		t.Errorf("expected Renamed in content, got: %s", result.Content)
	}
}

func TestSandboxCreateDirectory(t *testing.T) {
	_, client := newSandboxTestServer(t)
	reg := executor.NewToolRegistry()
	reg.RegisterSandboxTools(client)

	call := testCall("create_directory", map[string]any{"path": "src/pkg"})
	agent := testAgent(domain.TierJourneyman, domain.SkillCodeGen)
	result := reg.Execute(context.Background(), call, testQuest, agent)

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Content, "Created directory") {
		t.Errorf("expected Created directory in content, got: %s", result.Content)
	}
}

func TestSandboxContextCancelled(t *testing.T) {
	_, client := newSandboxTestServer(t)
	reg := executor.NewToolRegistry()
	reg.RegisterSandboxTools(client)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	call := testCall("read_file", map[string]any{"path": "main.go"})
	result := reg.Execute(ctx, call, testQuest, testAgent(domain.TierApprentice))

	if result.Error == "" {
		t.Fatal("expected error for cancelled context")
	}
	if !strings.Contains(result.Error, "cancelled") {
		t.Errorf("expected cancelled error, got: %s", result.Error)
	}
}

// TestSandboxToolsOverwriteBuiltins verifies that sandbox tools replace
// builtin handlers when both are registered.
func TestSandboxToolsOverwriteBuiltins(t *testing.T) {
	_, client := newSandboxTestServer(t)
	reg := executor.NewToolRegistry()
	reg.RegisterBuiltins()
	reg.RegisterSandboxTools(client)

	// read_file should work via sandbox, not local filesystem.
	call := testCall("read_file", map[string]any{"path": "main.go"})
	result := reg.Execute(context.Background(), call, testQuest, testAgent(domain.TierApprentice))

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	// The mock sandbox returns "hello from main.go" — the local handler
	// would fail because there's no file at that path.
	if result.Content != "hello from main.go" {
		t.Errorf("got %q — sandbox handler not used?", result.Content)
	}

	// Terminal tools should still be registered from RegisterBuiltins.
	tool := reg.Get("submit_work_product")
	if tool == nil {
		t.Fatal("submit_work_product should still be registered after sandbox tools overwrite")
	}
}

func TestSandboxLintCheck(t *testing.T) {
	_, client := newSandboxTestServer(t)
	reg := executor.NewToolRegistry()
	reg.RegisterSandboxTools(client)

	call := testCall("lint_check", map[string]any{"command": "go vet ./..."})
	agent := testAgent(domain.TierExpert, domain.SkillCodeReview)
	result := reg.Execute(context.Background(), call, testQuest, agent)

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
}

func TestSandboxLintCheck_InvalidCommand(t *testing.T) {
	_, client := newSandboxTestServer(t)
	reg := executor.NewToolRegistry()
	reg.RegisterSandboxTools(client)

	call := testCall("lint_check", map[string]any{"command": "cat /etc/passwd"})
	agent := testAgent(domain.TierExpert, domain.SkillCodeReview)
	result := reg.Execute(context.Background(), call, testQuest, agent)

	if result.Error == "" {
		t.Fatal("expected error for non-lint command")
	}
}

func TestSandboxReadFile_ServerError(t *testing.T) {
	// Create a server that returns 500 for all requests.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`)) //nolint:errcheck
	}))
	t.Cleanup(srv.Close)

	client := executor.NewSandboxClient(srv.URL)
	reg := executor.NewToolRegistry()
	reg.RegisterSandboxTools(client)

	call := testCall("read_file", map[string]any{"path": "main.go"})
	result := reg.Execute(context.Background(), call, testQuest, testAgent(domain.TierApprentice))

	if result.Error == "" {
		t.Fatal("expected error for server 500")
	}
	if !strings.Contains(result.Error, "failed to read file") {
		t.Errorf("expected 'failed to read file' error, got: %s", result.Error)
	}
}

