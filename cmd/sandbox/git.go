package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// =============================================================================
// GIT REQUEST / RESPONSE TYPES
// =============================================================================

// CommitAllRequest is the body for POST /workspace/{questID}/git/commit-all.
type CommitAllRequest struct {
	Message string `json:"message"`
}

// CommitAllResponse is returned from POST /workspace/{questID}/git/commit-all.
type CommitAllResponse struct {
	CommitHash   string `json:"commit_hash"`
	FilesChanged int    `json:"files_changed"`
}

// MergeToMainResponse is returned from POST /workspace/{questID}/merge-to-main.
type MergeToMainResponse struct {
	CommitHash   string   `json:"commit_hash"`
	FilesChanged []string `json:"files_changed"`
}

// ReposListResponse is returned from GET /repos.
type ReposListResponse struct {
	Repos []string `json:"repos"`
}

// GitLogEntry represents one commit in the log.
type GitLogEntry struct {
	Hash    string `json:"hash"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Subject string `json:"subject"`
}

// GitLogResponse is returned from GET /workspace/{questID}/git/log.
type GitLogResponse struct {
	Commits []GitLogEntry `json:"commits"`
}

// GitDiffResponse is returned from GET /workspace/{questID}/git/diff.
type GitDiffResponse struct {
	Diff string `json:"diff"`
}

// GitStatusResponse is returned from GET /workspace/{questID}/git/status.
type GitStatusResponse struct {
	Status string `json:"status"`
	Clean  bool   `json:"clean"`
}

// =============================================================================
// HANDLERS
// =============================================================================

// handleListRepos returns the names of all git repos found under s.reposDir.
func (s *Server) handleListRepos(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(s.reposDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("read repos dir: %v", err))
		return
	}

	var repos []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		gitDir := filepath.Join(s.reposDir, e.Name(), ".git")
		if _, statErr := os.Stat(gitDir); statErr == nil {
			repos = append(repos, e.Name())
		}
	}

	if repos == nil {
		repos = []string{} // Return [] not null in JSON.
	}
	writeJSON(w, http.StatusOK, ReposListResponse{Repos: repos})
}

// handleGitLog returns the commit history of a quest workspace.
// Query param: limit (default 20).
func (s *Server) handleGitLog(w http.ResponseWriter, r *http.Request) {
	questID, dir, ok := s.resolveWorktree(w, r)
	if !ok {
		return
	}

	limit := 20
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if n, err := strconv.Atoi(lStr); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	// Pretty format: hash|author|date|subject
	cmd := fmt.Sprintf("git log --pretty=format:'%%H|%%an|%%ai|%%s' -n %d", limit)
	res := execCommand(r.Context(), dir, cmd, 15*time.Second, s.maxOutputBytes)
	if res.ExitCode != 0 {
		writeError(w, http.StatusInternalServerError,
			fmt.Sprintf("git log: %s", res.Stderr))
		return
	}

	var commits []GitLogEntry
	for _, line := range strings.Split(strings.TrimSpace(res.Stdout), "\n") {
		line = strings.Trim(line, "'")
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		entry := GitLogEntry{}
		if len(parts) > 0 {
			entry.Hash = parts[0]
		}
		if len(parts) > 1 {
			entry.Author = parts[1]
		}
		if len(parts) > 2 {
			entry.Date = parts[2]
		}
		if len(parts) > 3 {
			entry.Subject = parts[3]
		}
		commits = append(commits, entry)
	}
	if commits == nil {
		commits = []GitLogEntry{}
	}

	s.logger.Info("git log", "quest_id", questID, "commits", len(commits))
	writeJSON(w, http.StatusOK, GitLogResponse{Commits: commits})
}

// handleGitDiff returns the uncommitted diff in the quest workspace.
func (s *Server) handleGitDiff(w http.ResponseWriter, r *http.Request) {
	questID, dir, ok := s.resolveWorktree(w, r)
	if !ok {
		return
	}

	res := execCommand(r.Context(), dir, "git diff HEAD", 15*time.Second, s.maxOutputBytes)
	if res.ExitCode != 0 {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("git diff: %s", res.Stderr))
		return
	}

	s.logger.Info("git diff", "quest_id", questID, "diff_bytes", len(res.Stdout))
	writeJSON(w, http.StatusOK, GitDiffResponse{Diff: res.Stdout})
}

// handleGitStatus returns the working-tree status in the quest workspace.
func (s *Server) handleGitStatus(w http.ResponseWriter, r *http.Request) {
	questID, dir, ok := s.resolveWorktree(w, r)
	if !ok {
		return
	}

	res := execCommand(r.Context(), dir, "git status --short", 15*time.Second, s.maxOutputBytes)
	if res.ExitCode != 0 {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("git status: %s", res.Stderr))
		return
	}

	status := strings.TrimSpace(res.Stdout)
	s.logger.Info("git status", "quest_id", questID, "clean", status == "")
	writeJSON(w, http.StatusOK, GitStatusResponse{
		Status: status,
		Clean:  status == "",
	})
}

// handleGitCommitAll stages all changes in the quest workspace and commits them.
// Body: {"message": "..."}.
// Returns the commit hash and number of changed files, or an empty hash if
// there was nothing to commit.
func (s *Server) handleGitCommitAll(w http.ResponseWriter, r *http.Request) {
	questID, dir, ok := s.resolveWorktree(w, r)
	if !ok {
		return
	}

	var req CommitAllRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Message == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}

	// Stage everything.
	res := execCommand(r.Context(), dir, "git add -A", 15*time.Second, s.maxOutputBytes)
	if res.ExitCode != 0 {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("git add: %s", res.Stderr))
		return
	}

	// Check if there is anything staged.
	statusRes := execCommand(r.Context(), dir, "git status --porcelain", 10*time.Second, s.maxOutputBytes)
	if statusRes.ExitCode != 0 {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("git status: %s", statusRes.Stderr))
		return
	}
	if strings.TrimSpace(statusRes.Stdout) == "" {
		// Nothing to commit.
		s.logger.Info("git commit-all: nothing to commit", "quest_id", questID)
		writeJSON(w, http.StatusOK, CommitAllResponse{CommitHash: "", FilesChanged: 0})
		return
	}

	// Count changed files.
	filesChanged := countLines(statusRes.Stdout)

	// Commit.
	commitCmd := fmt.Sprintf("git commit -m %s", shellQuote(req.Message))
	commitRes := execCommand(r.Context(), dir, commitCmd, 30*time.Second, s.maxOutputBytes)
	if commitRes.ExitCode != 0 {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("git commit: %s", commitRes.Stderr))
		return
	}

	// Retrieve the new HEAD hash.
	hashRes := execCommand(r.Context(), dir, "git rev-parse HEAD", 10*time.Second, 64)
	commitHash := strings.TrimSpace(hashRes.Stdout)

	s.logger.Info("git commit-all", "quest_id", questID, "hash", commitHash, "files", filesChanged)
	writeJSON(w, http.StatusOK, CommitAllResponse{
		CommitHash:   commitHash,
		FilesChanged: filesChanged,
	})
}

// handleMergeToMain merges the quest branch into main in the backing repo.
// This is a privileged endpoint called by the backend after boss battle victory.
// Returns 404 if the quest has no repo mapping, 409 on merge conflict.
func (s *Server) handleMergeToMain(w http.ResponseWriter, r *http.Request) {
	questID := r.PathValue("questID")
	if questID == "" || !isValidQuestID(questID) {
		writeError(w, http.StatusBadRequest, "invalid quest ID")
		return
	}

	s.mu.Lock()
	repo, hasRepo := s.questRepos[questID]
	s.mu.Unlock()

	if !hasRepo {
		writeError(w, http.StatusNotFound,
			fmt.Sprintf("quest %q has no repo-backed workspace", questID))
		return
	}

	repoPath := filepath.Join(s.reposDir, repo)
	branch := questBranch(questID)
	dir := filepath.Join(s.workspace, questID)

	rm := s.getRepoMutex(repo)
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Best-effort auto-commit any uncommitted work in the worktree before merging.
	autoCommitCmd := `git add -A && git diff --cached --quiet || git commit -m "auto-commit before merge"`
	execCommand(r.Context(), dir, autoCommitCmd, 30*time.Second, s.maxOutputBytes) //nolint:errcheck // best-effort

	// Switch repo to main.
	checkoutRes := execCommand(r.Context(), repoPath, "git checkout main", 15*time.Second, s.maxOutputBytes)
	if checkoutRes.ExitCode != 0 {
		writeError(w, http.StatusInternalServerError,
			fmt.Sprintf("git checkout main: %s", checkoutRes.Stderr))
		return
	}

	// Merge the quest branch.
	mergeMsg := fmt.Sprintf("quest %s: approved via boss battle", questID)
	mergeCmd := fmt.Sprintf("git merge %s --no-edit -m %s", branch, shellQuote(mergeMsg))
	mergeRes := execCommand(r.Context(), repoPath, mergeCmd, 30*time.Second, s.maxOutputBytes)
	if mergeRes.ExitCode != 0 {
		// Abort the failed merge to leave the repo clean.
		execCommand(r.Context(), repoPath, "git merge --abort", 10*time.Second, s.maxOutputBytes) //nolint:errcheck // best-effort
		writeJSON(w, http.StatusConflict, ErrorResponse{
			Error: fmt.Sprintf("merge conflict: %s", mergeRes.Stderr),
		})
		return
	}

	// Get the merge commit hash.
	hashRes := execCommand(r.Context(), repoPath, "git rev-parse HEAD", 10*time.Second, 64)
	commitHash := strings.TrimSpace(hashRes.Stdout)

	// List files changed by the merge.
	diffTreeRes := execCommand(r.Context(), repoPath,
		"git diff-tree --name-only -r HEAD^..HEAD",
		10*time.Second, s.maxOutputBytes)
	var filesChanged []string
	for _, line := range strings.Split(strings.TrimSpace(diffTreeRes.Stdout), "\n") {
		if line != "" {
			filesChanged = append(filesChanged, line)
		}
	}
	if filesChanged == nil {
		filesChanged = []string{}
	}

	s.logger.Info("merge-to-main completed",
		"quest_id", questID, "repo", repo, "branch", branch,
		"commit_hash", commitHash, "files_changed", len(filesChanged))

	writeJSON(w, http.StatusOK, MergeToMainResponse{
		CommitHash:   commitHash,
		FilesChanged: filesChanged,
	})
}

// =============================================================================
// INTERNAL HELPERS
// =============================================================================

// resolveWorktree validates the questID path parameter, ensures the workspace
// directory exists, and returns (questID, absDir, true) on success.
// On failure it writes the error response and returns (_, _, false).
func (s *Server) resolveWorktree(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	questID := r.PathValue("questID")
	if questID == "" || !isValidQuestID(questID) {
		writeError(w, http.StatusBadRequest, "invalid quest ID")
		return "", "", false
	}

	dir := filepath.Join(s.workspace, questID)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound,
			fmt.Sprintf("workspace %q does not exist", questID))
		return "", "", false
	}

	return questID, dir, true
}

// shellQuote wraps s in single quotes, escaping any embedded single quotes.
// This is safe for passing literal arguments to sh -c commands.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// countLines returns the number of non-empty lines in s.
func countLines(s string) int {
	n := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

