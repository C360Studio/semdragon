package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Server handles sandbox HTTP API requests.
type Server struct {
	workspace      string
	defaultTimeout time.Duration
	maxTimeout     time.Duration
	maxOutputBytes int
	maxFileSize    int64
	logger         *slog.Logger
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
	args := []string{"grep", "-rn"}
	if req.ContextLines > 0 {
		args = append(args, fmt.Sprintf("-C%d", req.ContextLines))
	}
	if req.FileGlob != "" {
		args = append(args, "--include="+req.FileGlob)
	}
	args = append(args, "--", req.Pattern, absPath)

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
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("create workspace: %v", err))
		return
	}

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

	dir := filepath.Join(s.workspace, questID)
	if err := os.RemoveAll(dir); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("delete workspace: %v", err))
		return
	}

	s.logger.Info("workspace deleted", "quest_id", questID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
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
