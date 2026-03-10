package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	workspace := t.TempDir()
	srv := &Server{
		workspace:      workspace,
		defaultTimeout: 10 * time.Second,
		maxTimeout:     30 * time.Second,
		maxOutputBytes: 100 * 1024,
		maxFileSize:    1 * 1024 * 1024,
		logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	return srv, workspace
}

func setupHTTP(t *testing.T) (*Server, *http.ServeMux, string) {
	t.Helper()
	srv, workspace := newTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)
	return srv, mux, workspace
}

func doRequest(t *testing.T, mux *http.ServeMux, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		reader = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func decodeResponse[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(w.Body).Decode(&v); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, w.Body.String())
	}
	return v
}

// =============================================================================
// HEALTH
// =============================================================================

func TestHealth(t *testing.T) {
	_, mux, _ := setupHTTP(t)
	w := doRequest(t, mux, "GET", "/health", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// =============================================================================
// WORKSPACE LIFECYCLE
// =============================================================================

func TestCreateAndDeleteWorkspace(t *testing.T) {
	_, mux, workspace := setupHTTP(t)

	// Create workspace.
	w := doRequest(t, mux, "POST", "/workspace/test-quest-123", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("create: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	dir := filepath.Join(workspace, "test-quest-123")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatal("workspace directory was not created")
	}

	// Create again (idempotent).
	w = doRequest(t, mux, "POST", "/workspace/test-quest-123", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("re-create: expected 200, got %d", w.Code)
	}

	// Delete workspace.
	w = doRequest(t, mux, "DELETE", "/workspace/test-quest-123", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d", w.Code)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("workspace directory was not deleted")
	}
}

func TestCreateWorkspace_InvalidID(t *testing.T) {
	_, mux, _ := setupHTTP(t)

	// Semicolon is not a valid quest ID character.
	w := doRequest(t, mux, "POST", "/workspace/quest;rm", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// =============================================================================
// FILE OPERATIONS
// =============================================================================

func TestWriteAndReadFile(t *testing.T) {
	_, mux, workspace := setupHTTP(t)

	// Create workspace first.
	os.MkdirAll(filepath.Join(workspace, "q1"), 0o755)

	// Write a file.
	w := doRequest(t, mux, "PUT", "/file", FileWriteRequest{
		QuestID: "q1",
		Path:    "main.go",
		Content: "package main\n\nfunc main() {}\n",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("write: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Read it back.
	w = doRequest(t, mux, "GET", "/file?quest_id=q1&path=main.go", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("read: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	resp := decodeResponse[FileResponse](t, w)
	if resp.Content != "package main\n\nfunc main() {}\n" {
		t.Errorf("content mismatch: got %q", resp.Content)
	}
}

func TestWriteFile_CreatesParentDirs(t *testing.T) {
	_, mux, workspace := setupHTTP(t)
	os.MkdirAll(filepath.Join(workspace, "q1"), 0o755)

	w := doRequest(t, mux, "PUT", "/file", FileWriteRequest{
		QuestID: "q1",
		Path:    "src/pkg/util.go",
		Content: "package pkg\n",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	data, err := os.ReadFile(filepath.Join(workspace, "q1", "src", "pkg", "util.go"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(data) != "package pkg\n" {
		t.Errorf("unexpected content: %q", string(data))
	}
}

func TestReadFile_NotFound(t *testing.T) {
	_, mux, workspace := setupHTTP(t)
	os.MkdirAll(filepath.Join(workspace, "q1"), 0o755)

	w := doRequest(t, mux, "GET", "/file?quest_id=q1&path=nope.go", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestWriteFile_PathTraversal(t *testing.T) {
	_, mux, workspace := setupHTTP(t)
	os.MkdirAll(filepath.Join(workspace, "q1"), 0o755)

	w := doRequest(t, mux, "PUT", "/file", FileWriteRequest{
		QuestID: "q1",
		Path:    "../../etc/passwd",
		Content: "hacked",
	})
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestWriteFile_ExceedsMaxSize(t *testing.T) {
	srv, workspace := newTestServer(t)
	srv.maxFileSize = 10 // 10 bytes max
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)
	os.MkdirAll(filepath.Join(workspace, "q1"), 0o755)

	w := doRequest(t, mux, "PUT", "/file", FileWriteRequest{
		QuestID: "q1",
		Path:    "big.txt",
		Content: "this content is longer than 10 bytes",
	})
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", w.Code, w.Body.String())
	}
}

// =============================================================================
// COMMAND EXECUTION
// =============================================================================

func TestExec_SimpleCommand(t *testing.T) {
	_, mux, workspace := setupHTTP(t)
	os.MkdirAll(filepath.Join(workspace, "q1"), 0o755)

	w := doRequest(t, mux, "POST", "/exec", ExecRequest{
		QuestID: "q1",
		Command: "echo hello world",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	resp := decodeResponse[ExecResponse](t, w)
	if resp.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d (stderr: %s)", resp.ExitCode, resp.Stderr)
	}
	if resp.Stdout != "hello world\n" {
		t.Errorf("expected 'hello world\\n', got %q", resp.Stdout)
	}
}

func TestExec_NonZeroExit(t *testing.T) {
	_, mux, workspace := setupHTTP(t)
	os.MkdirAll(filepath.Join(workspace, "q1"), 0o755)

	w := doRequest(t, mux, "POST", "/exec", ExecRequest{
		QuestID: "q1",
		Command: "exit 42",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	resp := decodeResponse[ExecResponse](t, w)
	if resp.ExitCode != 42 {
		t.Errorf("expected exit code 42, got %d", resp.ExitCode)
	}
}

func TestExec_Timeout(t *testing.T) {
	_, mux, workspace := setupHTTP(t)
	os.MkdirAll(filepath.Join(workspace, "q1"), 0o755)

	w := doRequest(t, mux, "POST", "/exec", ExecRequest{
		QuestID:   "q1",
		Command:   "sleep 60",
		TimeoutMs: 500, // 500ms timeout
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	resp := decodeResponse[ExecResponse](t, w)
	if !resp.TimedOut {
		t.Error("expected timed_out=true")
	}
}

func TestExec_WorkspaceNotFound(t *testing.T) {
	_, mux, _ := setupHTTP(t)

	w := doRequest(t, mux, "POST", "/exec", ExecRequest{
		QuestID: "nonexistent",
		Command: "echo hello",
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExec_WorkingDirectory(t *testing.T) {
	_, mux, workspace := setupHTTP(t)
	questDir := filepath.Join(workspace, "q1")
	os.MkdirAll(questDir, 0o755)

	// Write a file, then verify command runs in the workspace dir.
	os.WriteFile(filepath.Join(questDir, "marker.txt"), []byte("found"), 0o644)

	w := doRequest(t, mux, "POST", "/exec", ExecRequest{
		QuestID: "q1",
		Command: "cat marker.txt",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	resp := decodeResponse[ExecResponse](t, w)
	if resp.Stdout != "found" {
		t.Errorf("expected 'found', got %q", resp.Stdout)
	}
}

// =============================================================================
// DIRECTORY LISTING
// =============================================================================

func TestList(t *testing.T) {
	_, mux, workspace := setupHTTP(t)
	questDir := filepath.Join(workspace, "q1")
	os.MkdirAll(filepath.Join(questDir, "subdir"), 0o755)
	os.WriteFile(filepath.Join(questDir, "file.go"), []byte("test"), 0o644)

	w := doRequest(t, mux, "POST", "/list", ListRequest{QuestID: "q1"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	resp := decodeResponse[ListResponse](t, w)
	if len(resp.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(resp.Entries))
	}

	names := map[string]bool{}
	for _, e := range resp.Entries {
		names[e.Name] = true
	}
	if !names["file.go"] || !names["subdir"] {
		t.Errorf("unexpected entries: %+v", resp.Entries)
	}
}

// =============================================================================
// WORKSPACE FILE LISTING (recursive)
// =============================================================================

func TestListWorkspaceFiles(t *testing.T) {
	_, mux, workspace := setupHTTP(t)
	questDir := filepath.Join(workspace, "q1")
	os.MkdirAll(filepath.Join(questDir, "src"), 0o755)
	os.WriteFile(filepath.Join(questDir, "main.go"), []byte("package main"), 0o644)
	os.WriteFile(filepath.Join(questDir, "src", "lib.go"), []byte("package src"), 0o644)

	w := doRequest(t, mux, "GET", "/workspace/q1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	resp := decodeResponse[ListResponse](t, w)
	// Expect: main.go, src/, src/lib.go
	if len(resp.Entries) < 3 {
		t.Fatalf("expected at least 3 entries, got %d: %+v", len(resp.Entries), resp.Entries)
	}
}

// =============================================================================
// QUEST ID VALIDATION
// =============================================================================

func TestIsValidQuestID(t *testing.T) {
	tests := []struct {
		id    string
		valid bool
	}{
		{"simple-quest", true},
		{"c360.prod.game.board1.quest.abc123", true},
		{"quest_with_underscores", true},
		{"", false},
		{"../../etc", false},
		{"quest with spaces", false},
		{"quest;rm -rf /", false},
	}
	for _, tc := range tests {
		got := isValidQuestID(tc.id)
		if got != tc.valid {
			t.Errorf("isValidQuestID(%q) = %v, want %v", tc.id, got, tc.valid)
		}
	}
}

// =============================================================================
// PATH RESOLUTION
// =============================================================================

func TestResolveQuestPath(t *testing.T) {
	srv, workspace := newTestServer(t)

	tests := []struct {
		name    string
		questID string
		path    string
		wantErr bool
	}{
		{"simple file", "q1", "main.go", false},
		{"nested file", "q1", "src/pkg/util.go", false},
		{"traversal attempt", "q1", "../../etc/passwd", true},
		{"absolute path rejected", "q1", "/etc/passwd", true},
		{"dot path", "q1", ".", false},
		{"invalid quest ID", "../evil", "file.go", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := srv.resolveQuestPath(tc.questID, tc.path)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got path %q", result)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Result should be within workspace.
			expected := filepath.Join(workspace, tc.questID)
			if tc.path != "." {
				expected = filepath.Join(expected, tc.path)
			}
			if result != expected {
				t.Errorf("got %q, want %q", result, expected)
			}
		})
	}
}
