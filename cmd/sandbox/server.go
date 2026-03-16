package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// repoMutex holds a per-repo mutex used to serialise worktree and merge
// operations that modify the shared .git directory.
type repoMutex struct {
	mu sync.Mutex
}

// Server handles sandbox HTTP API requests.
type Server struct {
	workspace      string
	reposDir       string
	defaultTimeout time.Duration
	maxTimeout     time.Duration
	maxOutputBytes int
	maxFileSize    int64
	logger         *slog.Logger

	// questRepos maps quest ID → repo name for worktree-backed workspaces.
	// Protected by mu.
	mu         sync.Mutex
	questRepos map[string]string

	// repoMutexes holds a per-repo mutex to serialise git worktree and merge
	// operations. Protected by mu (acquired briefly only to read/insert).
	repoMutexes map[string]*repoMutex
}

// =============================================================================
// REQUEST / RESPONSE TYPES
// =============================================================================

// FileWriteRequest is the body for PUT /file.
type FileWriteRequest struct {
	QuestID string `json:"quest_id"`
	Path    string `json:"path"`    // Relative to /workspace/{quest_id}/
	Content string `json:"content"` // File content
}

// FileReadRequest is used for GET /file query parameters.
type FileReadRequest struct {
	QuestID string `json:"quest_id"`
	Path    string `json:"path"`
}

// FileResponse is returned from GET /file.
type FileResponse struct {
	Content string `json:"content"`
	Size    int    `json:"size"`
}

// ExecRequest is the body for POST /exec.
type ExecRequest struct {
	QuestID   string `json:"quest_id"`
	Command   string `json:"command"`
	TimeoutMs int    `json:"timeout_ms,omitempty"` // 0 = use default
}

// ExecResponse is returned from POST /exec.
type ExecResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	TimedOut bool   `json:"timed_out"`
}

// ListRequest is the body for POST /list.
type ListRequest struct {
	QuestID string `json:"quest_id"`
	Path    string `json:"path"` // Relative to workspace, empty = root
}

// ListEntry represents a single directory entry.
type ListEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

// ListResponse is returned from POST /list.
type ListResponse struct {
	Entries []ListEntry `json:"entries"`
}

// SearchRequest is the body for POST /search.
type SearchRequest struct {
	QuestID      string `json:"quest_id"`
	Pattern      string `json:"pattern"`
	Path         string `json:"path"`                    // Relative to workspace, empty = root
	FileGlob     string `json:"file_glob,omitempty"`     // e.g., "*.go"
	ContextLines int    `json:"context_lines,omitempty"` // Lines of context around matches
}

// SearchResponse is returned from POST /search.
type SearchResponse struct {
	Output string `json:"output"` // grep-style output
}

// ErrorResponse is returned on errors.
type ErrorResponse struct {
	Error string `json:"error"`
}

// =============================================================================
// ROUTE REGISTRATION
// =============================================================================

// RegisterRoutes registers all sandbox API endpoints.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("PUT /file", s.handleWriteFile)
	mux.HandleFunc("GET /file", s.handleReadFile)
	mux.HandleFunc("POST /exec", s.handleExec)
	mux.HandleFunc("POST /list", s.handleList)
	mux.HandleFunc("POST /search", s.handleSearch)
	mux.HandleFunc("POST /workspace/{questID}", s.handleCreateWorkspace)
	mux.HandleFunc("DELETE /workspace/{questID}", s.handleDeleteWorkspace)
	mux.HandleFunc("GET /workspace/{questID}", s.handleListWorkspaceFiles)

	// Git / repo endpoints.
	mux.HandleFunc("GET /repos", s.handleListRepos)
	mux.HandleFunc("GET /workspace/{questID}/git/log", s.handleGitLog)
	mux.HandleFunc("GET /workspace/{questID}/git/diff", s.handleGitDiff)
	mux.HandleFunc("GET /workspace/{questID}/git/status", s.handleGitStatus)
	mux.HandleFunc("POST /workspace/{questID}/git/commit-all", s.handleGitCommitAll)
	mux.HandleFunc("POST /workspace/{questID}/merge-to-main", s.handleMergeToMain)
}

// =============================================================================
// HANDLERS
// =============================================================================

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleWriteFile(w http.ResponseWriter, r *http.Request) {
	var req FileWriteRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.QuestID == "" || req.Path == "" {
		writeError(w, http.StatusBadRequest, "quest_id and path are required")
		return
	}
	if int64(len(req.Content)) > s.maxFileSize {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("file exceeds max size (%d bytes)", s.maxFileSize))
		return
	}

	absPath, err := s.resolveQuestPath(req.QuestID, req.Path)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	// Create parent directories.
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("create directory: %v", err))
		return
	}

	if err := os.WriteFile(absPath, []byte(req.Content), 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("write file: %v", err))
		return
	}

	s.logger.Info("file written", "quest_id", req.QuestID, "path", req.Path, "size", len(req.Content))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReadFile(w http.ResponseWriter, r *http.Request) {
	questID := r.URL.Query().Get("quest_id")
	path := r.URL.Query().Get("path")
	if questID == "" || path == "" {
		writeError(w, http.StatusBadRequest, "quest_id and path query params are required")
		return
	}

	absPath, err := s.resolveQuestPath(questID, path)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "file not found")
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("read file: %v", err))
		return
	}

	// Cap read size to prevent huge responses.
	if len(data) > s.maxOutputBytes {
		data = data[:s.maxOutputBytes]
	}

	writeJSON(w, http.StatusOK, FileResponse{
		Content: string(data),
		Size:    len(data),
	})
}

func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
	var req ExecRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.QuestID == "" || req.Command == "" {
		writeError(w, http.StatusBadRequest, "quest_id and command are required")
		return
	}

	workDir := filepath.Join(s.workspace, req.QuestID)
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("workspace %q does not exist", req.QuestID))
		return
	}

	timeout := s.defaultTimeout
	if req.TimeoutMs > 0 {
		requested := time.Duration(req.TimeoutMs) * time.Millisecond
		if requested > s.maxTimeout {
			requested = s.maxTimeout
		}
		timeout = requested
	}

	result := execCommand(r.Context(), workDir, req.Command, timeout, s.maxOutputBytes)

	s.logger.Info("command executed",
		"quest_id", req.QuestID,
		"command", truncate(req.Command, 100),
		"exit_code", result.ExitCode,
		"timed_out", result.TimedOut,
	)

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	var req ListRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.QuestID == "" {
		writeError(w, http.StatusBadRequest, "quest_id is required")
		return
	}

	dirPath := req.Path
	if dirPath == "" {
		dirPath = "."
	}

	absPath, err := s.resolveQuestPath(req.QuestID, dirPath)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "directory not found")
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("list directory: %v", err))
		return
	}

	var result []ListEntry
	for _, e := range entries {
		info, infoErr := e.Info()
		var size int64
		if infoErr == nil {
			size = info.Size()
		}
		result = append(result, ListEntry{
			Name:  e.Name(),
			IsDir: e.IsDir(),
			Size:  size,
		})
	}

	writeJSON(w, http.StatusOK, ListResponse{Entries: result})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	var req SearchRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.QuestID == "" || req.Pattern == "" {
		writeError(w, http.StatusBadRequest, "quest_id and pattern are required")
		return
	}

	searchPath := req.Path
	if searchPath == "" {
		searchPath = "."
	}

	absPath, err := s.resolveQuestPath(req.QuestID, searchPath)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	// Build grep command. Use grep -rn for recursive search with line numbers.
	// Shell-quote user-supplied pattern and glob to prevent injection.
	args := []string{"grep", "-rn"}
	if req.ContextLines > 0 {
		args = append(args, fmt.Sprintf("-C%d", req.ContextLines))
	}
	if req.FileGlob != "" {
		args = append(args, "--include="+shellQuote(req.FileGlob))
	}
	args = append(args, "--", shellQuote(req.Pattern), shellQuote(absPath))

	cmd := strings.Join(args, " ")
	result := execCommand(r.Context(), filepath.Join(s.workspace, req.QuestID), cmd, 10*time.Second, s.maxOutputBytes)

	writeJSON(w, http.StatusOK, SearchResponse{Output: result.Stdout})
}

func (s *Server) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	questID := r.PathValue("questID")
	if questID == "" {
		writeError(w, http.StatusBadRequest, "quest ID is required")
		return
	}
	if !isValidQuestID(questID) {
		writeError(w, http.StatusBadRequest, "invalid quest ID")
		return
	}

	dir := filepath.Join(s.workspace, questID)
	repo := r.URL.Query().Get("repo")

	if repo != "" {
		// Repo-backed workspace: create a git worktree on a new quest branch.
		if !isValidRepoName(repo) {
			writeError(w, http.StatusBadRequest, "invalid repo name")
			return
		}
		repoPath := filepath.Join(s.reposDir, repo)
		if _, err := os.Stat(filepath.Join(repoPath, ".git")); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("repo %q not found or not a git repository", repo))
			return
		}

		branch := questBranch(questID)

		rm := s.getRepoMutex(repo)
		rm.mu.Lock()
		defer rm.mu.Unlock()

		// If this quest was retried, the branch and worktree already exist
		// from a previous attempt. Reuse them — the partial work is valuable
		// context for the next agent.
		if _, statErr := os.Stat(dir); statErr == nil {
			s.logger.Info("reusing existing worktree from previous attempt",
				"quest_id", questID, "repo", repo, "branch", branch, "path", dir)

			// Re-record the mapping (may have been cleaned up between attempts).
			s.mu.Lock()
			s.questRepos[questID] = repo
			s.mu.Unlock()

			writeJSON(w, http.StatusOK, map[string]string{
				"status": "reused",
				"repo":   repo,
				"branch": branch,
			})
			return
		}

		// Create the quest branch from main.
		res := execCommand(r.Context(), repoPath,
			fmt.Sprintf("git branch %s main", branch),
			15*time.Second, s.maxOutputBytes)
		if res.ExitCode != 0 {
			writeError(w, http.StatusInternalServerError,
				fmt.Sprintf("create branch %q: %s", branch, res.Stderr))
			return
		}

		// Add a worktree at the workspace path.
		res = execCommand(r.Context(), repoPath,
			fmt.Sprintf("git worktree add %s %s", dir, branch),
			15*time.Second, s.maxOutputBytes)
		if res.ExitCode != 0 {
			// Clean up the branch we just created.
			execCommand(r.Context(), repoPath, //nolint:errcheck // best-effort cleanup
				fmt.Sprintf("git branch -D %s", branch),
				10*time.Second, s.maxOutputBytes)
			writeError(w, http.StatusInternalServerError,
				fmt.Sprintf("git worktree add: %s", res.Stderr))
			return
		}

		// Configure git identity in the worktree so commits work without user config.
		configCmd := `git config user.email "sandbox@semdragons" && git config user.name "Sandbox"`
		execCommand(r.Context(), dir, configCmd, 10*time.Second, s.maxOutputBytes) //nolint:errcheck // best-effort

		// Record quest → repo mapping.
		s.mu.Lock()
		s.questRepos[questID] = repo
		s.mu.Unlock()

		s.logger.Info("repo-backed workspace created",
			"quest_id", questID, "repo", repo, "branch", branch, "path", dir)
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "created",
			"repo":   repo,
			"branch": branch,
		})
		return
	}

	// Plain workspace (backward-compatible path).

	// Check if the directory already exists (e.g., bind-mounted git worktree).
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		s.logger.Info("workspace already exists (worktree)", "quest_id", questID, "path", dir)
		writeJSON(w, http.StatusOK, map[string]string{"status": "exists", "path": dir})
		return
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("create workspace: %v", err))
		return
	}

	// Initialise a bare git repo so agents can commit work in plain workspaces too.
	execCommand(r.Context(), dir, "git init", 10*time.Second, s.maxOutputBytes) //nolint:errcheck // best-effort

	s.logger.Info("workspace created", "quest_id", questID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "created", "path": dir})
}

func (s *Server) handleDeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	questID := r.PathValue("questID")
	if questID == "" {
		writeError(w, http.StatusBadRequest, "quest ID is required")
		return
	}
	if !isValidQuestID(questID) {
		writeError(w, http.StatusBadRequest, "invalid quest ID")
		return
	}

	if err := s.removeWorkspace(r.Context(), questID); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("delete workspace: %v", err))
		return
	}

	s.logger.Info("workspace deleted", "quest_id", questID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// removeWorkspace tears down a quest workspace. If the quest is backed by a
// repo worktree, it removes the worktree and deletes the quest branch; otherwise
// it removes the directory.
func (s *Server) removeWorkspace(ctx context.Context, questID string) error {
	s.mu.Lock()
	repo, hasRepo := s.questRepos[questID]
	if hasRepo {
		delete(s.questRepos, questID)
	}
	s.mu.Unlock()

	dir := filepath.Join(s.workspace, questID)
	branch := questBranch(questID)

	if hasRepo {
		repoPath := filepath.Join(s.reposDir, repo)
		rm := s.getRepoMutex(repo)
		rm.mu.Lock()
		defer rm.mu.Unlock()

		// Remove worktree entry from .git/worktrees.
		res := execCommand(ctx, repoPath,
			fmt.Sprintf("git worktree remove --force %s", dir),
			15*time.Second, s.maxOutputBytes)
		if res.ExitCode != 0 {
			// Fall back to plain removal if worktree remove failed (e.g., already gone).
			s.logger.Warn("git worktree remove failed — falling back to os.RemoveAll",
				"quest_id", questID, "stderr", res.Stderr)
			os.RemoveAll(dir) //nolint:errcheck // best-effort
		}

		// Delete the quest branch from the repo.
		res = execCommand(ctx, repoPath,
			fmt.Sprintf("git branch -D %s", branch),
			10*time.Second, s.maxOutputBytes)
		if res.ExitCode != 0 {
			s.logger.Warn("git branch -D failed",
				"quest_id", questID, "branch", branch, "stderr", res.Stderr)
		}
		return nil
	}

	return os.RemoveAll(dir)
}

// handleListWorkspaceFiles returns all files in a quest workspace (recursive).
// Used by the boss battle judge to inspect what the agent produced.
func (s *Server) handleListWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	questID := r.PathValue("questID")
	if questID == "" {
		writeError(w, http.StatusBadRequest, "quest ID is required")
		return
	}

	dir := filepath.Join(s.workspace, questID)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	var files []ListEntry
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // skip errors
		}
		// Skip .git directory contents — worktrees contain git metadata
		// that should not be exposed as quest artifacts.
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil || rel == "." {
			return nil
		}
		info, infoErr := d.Info()
		var size int64
		if infoErr == nil {
			size = info.Size()
		}
		files = append(files, ListEntry{
			Name:  rel,
			IsDir: d.IsDir(),
			Size:  size,
		})
		return nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("walk workspace: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, ListResponse{Entries: files})
}

// =============================================================================
// PATH RESOLUTION
// =============================================================================

// resolveQuestPath resolves a relative path within a quest's workspace,
// preventing directory traversal attacks. Rejects absolute paths outright.
func (s *Server) resolveQuestPath(questID, relPath string) (string, error) {
	if !isValidQuestID(questID) {
		return "", fmt.Errorf("invalid quest ID")
	}

	// Reject absolute paths — all paths must be relative to the workspace.
	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("absolute paths are not allowed")
	}

	questRoot := filepath.Join(s.workspace, questID)
	absPath := filepath.Join(questRoot, filepath.Clean(relPath))

	// Ensure the resolved path is within the quest workspace.
	if !strings.HasPrefix(absPath, questRoot+string(filepath.Separator)) && absPath != questRoot {
		return "", fmt.Errorf("path escapes quest workspace")
	}

	return absPath, nil
}

// isValidQuestID checks that a quest ID contains only safe characters.
// Entity IDs use dots, alphanumerics, and hyphens (e.g., "c360.prod.game.board1.quest.abc123").
func isValidQuestID(id string) bool {
	if id == "" || len(id) > 256 {
		return false
	}
	for _, c := range id {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

// isValidRepoName checks that a repo name is a simple directory name with no
// path separators or special characters.
func isValidRepoName(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	// Disallow names that could be traversal attempts.
	return name != "." && name != ".." && !strings.Contains(name, "..")
}

// questBranch derives the git branch name for a quest. The full entity ID uses
// dots as separators; we use only the last segment as the branch suffix so that
// branch names stay short and valid.
// e.g. "c360.prod.game.board1.quest.abc123" → "quest/abc123"
func questBranch(questID string) string {
	parts := strings.Split(questID, ".")
	last := parts[len(parts)-1]
	return "quest/" + last
}

// getRepoMutex returns the per-repo mutex for repo, creating it if necessary.
// The outer s.mu is held only briefly to look up / insert the entry.
func (s *Server) getRepoMutex(repo string) *repoMutex {
	s.mu.Lock()
	rm, ok := s.repoMutexes[repo]
	if !ok {
		rm = &repoMutex{}
		s.repoMutexes[repo] = rm
	}
	s.mu.Unlock()
	return rm
}

// =============================================================================
// JSON HELPERS
// =============================================================================

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 2*1024*1024)) // 2 MB max request body
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	return json.Unmarshal(body, v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck // best-effort response
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: msg})
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
