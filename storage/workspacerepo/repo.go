package workspacerepo

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// gitCommandTimeout bounds individual git operations. A stalled filesystem
// or large merge should not hang the caller indefinitely.
const gitCommandTimeout = 30 * time.Second

// instanceIDPattern matches valid quest instance IDs: alphanumeric plus hyphens.
// Rejects path separators, dots (which are entity ID delimiters), shell
// metacharacters, and leading dashes (which git interprets as flags).
var instanceIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// FileEntry represents a file in a quest worktree.
type FileEntry struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// TreeEntry represents a node in a nested file tree.
type TreeEntry struct {
	Name     string       `json:"name"`
	Path     string       `json:"path"`
	IsDir    bool         `json:"is_dir"`
	Size     int64        `json:"size,omitempty"`
	Children []*TreeEntry `json:"children,omitempty"`
}

// WorkspaceRepo manages a bare git repository with per-quest worktrees.
// Each quest gets its own branch and worktree directory, enabling parallel
// agents to commit independently with zero contention.
type WorkspaceRepo struct {
	repoDir      string
	worktreesDir string
	logger       *slog.Logger

	// mu serializes repo-level git operations (init, merge, worktree add/remove).
	// git uses filesystem-level locking internally, but concurrent worktree add
	// against the same bare repo can hit lock contention under load.
	mu sync.Mutex
}

// New creates a WorkspaceRepo. Call Init to initialize the bare repo.
func New(repoDir, worktreesDir string, logger *slog.Logger) *WorkspaceRepo {
	return &WorkspaceRepo{
		repoDir:      repoDir,
		worktreesDir: worktreesDir,
		logger:       logger,
	}
}

// WorktreesDir returns the base directory where per-quest worktrees are created.
func (w *WorkspaceRepo) WorktreesDir() string {
	return w.worktreesDir
}

// Init ensures the bare repository and worktrees directory exist.
// Safe to call multiple times — no-op if already initialized.
func (w *WorkspaceRepo) Init(ctx context.Context) error {
	if err := os.MkdirAll(w.worktreesDir, 0o755); err != nil {
		return fmt.Errorf("create worktrees dir: %w", err)
	}

	// If bare repo already exists, nothing to do.
	if _, err := os.Stat(filepath.Join(w.repoDir, "HEAD")); err == nil {
		return nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Double-check after lock acquisition.
	if _, err := os.Stat(filepath.Join(w.repoDir, "HEAD")); err == nil {
		return nil
	}

	if err := os.MkdirAll(w.repoDir, 0o755); err != nil {
		return fmt.Errorf("create repo dir: %w", err)
	}

	// Bootstrap: init a normal repo with an initial commit, then clone as bare.
	// A bare repo with no commits makes `git worktree add` fail.
	tmpInit, err := os.MkdirTemp("", "workspace-init-*")
	if err != nil {
		return fmt.Errorf("create temp dir for init: %w", err)
	}
	defer os.RemoveAll(tmpInit)

	if err := w.gitInDir(ctx, tmpInit, "init", "-b", "main"); err != nil {
		return fmt.Errorf("git init: %w", err)
	}
	if err := w.gitInDir(ctx, tmpInit, "config", "user.name", "semdragons"); err != nil {
		return fmt.Errorf("set user.name: %w", err)
	}
	if err := w.gitInDir(ctx, tmpInit, "config", "user.email", "system@semdragons"); err != nil {
		return fmt.Errorf("set user.email: %w", err)
	}

	readmePath := filepath.Join(tmpInit, ".gitkeep")
	if err := os.WriteFile(readmePath, []byte(""), 0o644); err != nil {
		return fmt.Errorf("write .gitkeep: %w", err)
	}
	if err := w.gitInDir(ctx, tmpInit, "add", ".gitkeep"); err != nil {
		return fmt.Errorf("git add .gitkeep: %w", err)
	}
	if err := w.gitInDir(ctx, tmpInit, "commit", "-m", "initial commit"); err != nil {
		return fmt.Errorf("git commit initial: %w", err)
	}

	// Clone to a temp location first, then rename atomically. This avoids
	// a crash window where repoDir has been removed but clone hasn't finished.
	// Use the system temp dir — the parent of repoDir may not be writable
	// (e.g., Docker volume mount owned by root when running as non-root user).
	tmpBare, err := os.MkdirTemp("", "workspace-bare-*")
	if err != nil {
		return fmt.Errorf("create temp bare dir: %w", err)
	}

	cmd := exec.CommandContext(ctx, "git", "clone", "--bare", tmpInit, tmpBare)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, cloneErr := cmd.CombinedOutput(); cloneErr != nil {
		os.RemoveAll(tmpBare)
		return fmt.Errorf("git clone --bare: %s: %s", cloneErr, strings.TrimSpace(string(out)))
	}

	// Atomic swap: remove empty target, rename temp into place.
	if err := os.RemoveAll(w.repoDir); err != nil {
		os.RemoveAll(tmpBare)
		return fmt.Errorf("remove empty repo dir: %w", err)
	}
	if err := os.Rename(tmpBare, w.repoDir); err != nil {
		return fmt.Errorf("rename bare repo into place: %w", err)
	}

	w.logger.Info("workspace repo initialized", "repo_dir", w.repoDir)
	return nil
}

// CreateWorktree creates a new git worktree and branch for a quest.
// The branch is named quest/{questInstanceID} and the worktree is placed
// at worktreesDir/quest-{questInstanceID}.
//
// Serialized via mu because git worktree add modifies shared state in the
// bare repo (.git/worktrees/ directory).
func (w *WorkspaceRepo) CreateWorktree(ctx context.Context, questInstanceID string) error {
	if err := validateInstanceID(questInstanceID); err != nil {
		return err
	}

	branch := w.branchName(questInstanceID)
	worktree := w.worktreePath(questInstanceID)

	// Check if worktree already exists (idempotent).
	if _, err := os.Stat(filepath.Join(worktree, ".git")); err == nil {
		w.logger.Debug("worktree already exists", "quest_id", questInstanceID)
		return nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Create worktree with a new branch from main.
	if err := w.gitBare(ctx, "worktree", "add", "-b", branch, worktree, "main"); err != nil {
		return fmt.Errorf("create worktree for quest %s: %w", questInstanceID, err)
	}

	w.logger.Info("worktree created", "quest_id", questInstanceID, "branch", branch, "path", worktree)
	return nil
}

// ConfigureIdentity sets the git user.name and user.email in a quest worktree.
func (w *WorkspaceRepo) ConfigureIdentity(ctx context.Context, questInstanceID, name, email string) error {
	if err := validateInstanceID(questInstanceID); err != nil {
		return err
	}
	worktree := w.worktreePath(questInstanceID)
	if err := w.gitInDir(ctx, worktree, "config", "user.name", name); err != nil {
		return fmt.Errorf("set user.name: %w", err)
	}
	if err := w.gitInDir(ctx, worktree, "config", "user.email", email); err != nil {
		return fmt.Errorf("set user.email: %w", err)
	}
	return nil
}

// FinalizeWorktree stages any uncommitted files and creates a commit with
// quest/agent metadata. Returns the commit hash. If there are no changes
// to commit, returns the current HEAD hash.
func (w *WorkspaceRepo) FinalizeWorktree(ctx context.Context, questInstanceID, agentID string) (string, error) {
	if err := validateInstanceID(questInstanceID); err != nil {
		return "", err
	}
	worktree := w.worktreePath(questInstanceID)

	// Stage all changes.
	if err := w.gitInDir(ctx, worktree, "add", "-A"); err != nil {
		return "", fmt.Errorf("git add -A: %w", err)
	}

	// Check if there are staged changes.
	statusOut, err := w.gitOutput(ctx, worktree, "status", "--porcelain")
	if err != nil {
		return "", fmt.Errorf("git status: %w", err)
	}

	if strings.TrimSpace(statusOut) != "" {
		msg := fmt.Sprintf("quest %s finalized by agent %s", questInstanceID, agentID)
		if err := w.gitInDir(ctx, worktree, "commit", "-m", msg,
			"--author", fmt.Sprintf("%s <agent@semdragons>", agentID)); err != nil {
			return "", fmt.Errorf("git commit: %w", err)
		}
	}

	hash, err := w.gitOutput(ctx, worktree, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(hash), nil
}

// MergeToMain merges the quest branch into main. This is the quality gate:
// only boss-battle-approved work enters main (and thus the semsource graph).
// Returns the merge commit hash. On merge conflict, returns an error.
func (w *WorkspaceRepo) MergeToMain(ctx context.Context, questInstanceID string) (string, error) {
	if err := validateInstanceID(questInstanceID); err != nil {
		return "", err
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	branch := w.branchName(questInstanceID)

	tmpDir, err := os.MkdirTemp("", "workspace-merge-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir for merge: %w", err)
	}
	defer func() {
		// Use background context for cleanup — the caller's ctx may be cancelled
		// during shutdown, but we must still remove the temporary worktree.
		cleanCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if rmErr := w.gitBare(cleanCtx, "worktree", "remove", "--force", tmpDir); rmErr != nil {
			w.logger.Warn("failed to remove temporary merge worktree",
				"path", tmpDir, "error", rmErr)
		}
		os.RemoveAll(tmpDir)
	}()

	if err := w.gitBare(ctx, "worktree", "add", tmpDir, "main"); err != nil {
		return "", fmt.Errorf("checkout main worktree: %w", err)
	}

	if err := w.gitInDir(ctx, tmpDir, "merge", branch, "-m",
		fmt.Sprintf("merge quest/%s: approved via boss battle", questInstanceID)); err != nil {
		return "", fmt.Errorf("merge %s into main: %w", branch, err)
	}

	hash, err := w.gitOutput(ctx, tmpDir, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("get merge hash: %w", err)
	}

	w.logger.Info("quest branch merged to main",
		"quest_id", questInstanceID, "branch", branch, "merge_hash", strings.TrimSpace(hash))
	return strings.TrimSpace(hash), nil
}

// MergedFiles returns the list of file paths changed by a merge commit.
// Used by the indexing watcher to correlate semsource entities to quest artifacts.
func (w *WorkspaceRepo) MergedFiles(ctx context.Context, mergeHash string) ([]string, error) {
	out, err := w.gitOutput(ctx, w.repoDir, "diff-tree", "--name-only", "-r", mergeHash+"^.."+mergeHash)
	if err != nil {
		return nil, fmt.Errorf("git diff-tree: %w", err)
	}

	var files []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// RemoveWorktree removes the worktree and deletes the branch.
func (w *WorkspaceRepo) RemoveWorktree(ctx context.Context, questInstanceID string) error {
	if err := validateInstanceID(questInstanceID); err != nil {
		return err
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	worktree := w.worktreePath(questInstanceID)
	branch := w.branchName(questInstanceID)

	if err := w.gitBare(ctx, "worktree", "remove", "--force", worktree); err != nil {
		if _, statErr := os.Stat(worktree); os.IsNotExist(statErr) {
			w.logger.Debug("worktree already removed", "quest_id", questInstanceID)
		} else {
			return fmt.Errorf("remove worktree: %w", err)
		}
	}

	// Delete the branch (best-effort, may fail if already merged/deleted).
	if err := w.gitBare(ctx, "branch", "-D", branch); err != nil {
		w.logger.Debug("could not delete branch (may already be deleted)",
			"branch", branch, "error", err)
	}

	w.logger.Info("worktree removed", "quest_id", questInstanceID)
	return nil
}

// ListQuestFiles returns a flat list of all tracked files in the quest worktree.
// Uses git ls-files to only return committed/staged files, not untracked debris.
func (w *WorkspaceRepo) ListQuestFiles(questInstanceID string) ([]FileEntry, error) {
	if err := validateInstanceID(questInstanceID); err != nil {
		return nil, err
	}
	worktree := w.worktreePath(questInstanceID)

	if _, err := os.Stat(worktree); os.IsNotExist(err) {
		return nil, fmt.Errorf("worktree does not exist: %s", questInstanceID)
	}

	// Use git ls-files to list only tracked files. This is more accurate than
	// filepath.Walk since it skips untracked files, .git internals, and .gitkeep.
	ctx, cancel := context.WithTimeout(context.Background(), gitCommandTimeout)
	defer cancel()

	out, err := w.gitOutput(ctx, worktree, "ls-files")
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}

	var files []FileEntry
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == ".gitkeep" {
			continue
		}
		fullPath := filepath.Join(worktree, filepath.FromSlash(line))
		info, statErr := os.Stat(fullPath)
		var size int64
		if statErr == nil {
			size = info.Size()
		}
		files = append(files, FileEntry{
			Path: line,
			Size: size,
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files, nil
}

// ListWorktreeIDs returns the instance IDs of all quest worktrees currently on disk.
func (w *WorkspaceRepo) ListWorktreeIDs() ([]string, error) {
	entries, err := os.ReadDir(w.worktreesDir)
	if err != nil {
		return nil, fmt.Errorf("read worktrees dir: %w", err)
	}

	var ids []string
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "quest-") {
			continue
		}
		ids = append(ids, strings.TrimPrefix(entry.Name(), "quest-"))
	}
	return ids, nil
}

// ReadFile reads a file from a quest worktree.
func (w *WorkspaceRepo) ReadFile(questInstanceID, path string) ([]byte, error) {
	if err := validateInstanceID(questInstanceID); err != nil {
		return nil, err
	}

	worktree := w.worktreePath(questInstanceID)
	fullPath := filepath.Join(worktree, filepath.FromSlash(path))

	// Resolve both the worktree root and the requested file to their real
	// on-disk paths (following all symlinks). Then verify the file is still
	// inside the worktree. This catches: ".." traversal, absolute path
	// injection, and symlinks pointing outside the worktree.
	realWorktree, err := filepath.EvalSymlinks(worktree)
	if err != nil {
		return nil, fmt.Errorf("resolve worktree: %w", err)
	}
	realPath, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		return nil, fmt.Errorf("resolve path %s: %w", path, err)
	}
	if !strings.HasPrefix(realPath, realWorktree+string(filepath.Separator)) {
		return nil, fmt.Errorf("path traversal not allowed: %s", path)
	}

	data, err := os.ReadFile(realPath)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", path, err)
	}
	return data, nil
}

// FileTree returns a nested tree structure of files in the quest worktree.
func (w *WorkspaceRepo) FileTree(questInstanceID string) ([]*TreeEntry, error) {
	files, err := w.ListQuestFiles(questInstanceID)
	if err != nil {
		return nil, err
	}

	root := &TreeEntry{IsDir: true}
	for _, f := range files {
		parts := strings.Split(f.Path, string(filepath.Separator))
		current := root
		for i, part := range parts {
			isLast := i == len(parts)-1
			if isLast {
				current.Children = append(current.Children, &TreeEntry{
					Name: part,
					Path: f.Path,
					Size: f.Size,
				})
			} else {
				var dir *TreeEntry
				for _, child := range current.Children {
					if child.IsDir && child.Name == part {
						dir = child
						break
					}
				}
				if dir == nil {
					dirPath := strings.Join(parts[:i+1], string(filepath.Separator))
					dir = &TreeEntry{
						Name:  part,
						Path:  dirPath,
						IsDir: true,
					}
					current.Children = append(current.Children, dir)
				}
				current = dir
			}
		}
	}

	sortTree(root)
	return root.Children, nil
}

// WorktreeExists checks if a worktree directory exists for the given quest.
func (w *WorkspaceRepo) WorktreeExists(questInstanceID string) bool {
	if validateInstanceID(questInstanceID) != nil {
		return false
	}
	worktree := w.worktreePath(questInstanceID)
	_, err := os.Stat(filepath.Join(worktree, ".git"))
	return err == nil
}

// WorktreePath returns the filesystem path for a quest's worktree.
func (w *WorkspaceRepo) WorktreePath(questInstanceID string) string {
	return w.worktreePath(questInstanceID)
}

// PruneCompleted removes worktrees for quests in terminal states older than
// the retention period. The questStatusFn callback determines the current
// quest status for each worktree.
func (w *WorkspaceRepo) PruneCompleted(ctx context.Context, retentionDays int, questStatusFn func(instanceID string) (status string, completedAt time.Time)) error {
	ids, err := w.ListWorktreeIDs()
	if err != nil {
		return err
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	pruned := 0

	for _, instanceID := range ids {
		status, completedAt := questStatusFn(instanceID)
		if !isTerminalStatus(status) {
			continue
		}
		if completedAt.After(cutoff) {
			continue
		}

		if err := w.RemoveWorktree(ctx, instanceID); err != nil {
			w.logger.Warn("failed to prune worktree",
				"quest_id", instanceID, "error", err)
			continue
		}
		pruned++
	}

	if pruned > 0 {
		w.logger.Info("pruned completed worktrees", "count", pruned)
	}
	return nil
}

// validateInstanceID rejects instance IDs that could cause path traversal,
// git flag injection, or filesystem escapes.
func validateInstanceID(id string) error {
	if !instanceIDPattern.MatchString(id) {
		return fmt.Errorf("invalid quest instance ID: %q", id)
	}
	return nil
}

// branchName returns the git branch name for a quest.
func (w *WorkspaceRepo) branchName(questInstanceID string) string {
	return "quest/" + questInstanceID
}

// worktreePath returns the filesystem path for a quest worktree.
func (w *WorkspaceRepo) worktreePath(questInstanceID string) string {
	return filepath.Join(w.worktreesDir, "quest-"+questInstanceID)
}

// gitBare runs a git command in the bare repository with a bounded timeout.
func (w *WorkspaceRepo) gitBare(ctx context.Context, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, gitCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", w.repoDir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// gitInDir runs a git command in a specific directory with a bounded timeout.
func (w *WorkspaceRepo) gitInDir(ctx context.Context, dir string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, gitCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// gitOutput runs a git command and returns stdout with a bounded timeout.
func (w *WorkspaceRepo) gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, gitCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("%s: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", err
	}
	return string(out), nil
}

// isTerminalStatus checks if a quest status is terminal (safe to prune).
func isTerminalStatus(status string) bool {
	switch status {
	case "completed", "failed", "abandoned":
		return true
	default:
		return false
	}
}

// sortTree recursively sorts tree entries: directories first, then alphabetical.
func sortTree(node *TreeEntry) {
	if node == nil {
		return
	}
	sort.Slice(node.Children, func(i, j int) bool {
		if node.Children[i].IsDir != node.Children[j].IsDir {
			return node.Children[i].IsDir
		}
		return node.Children[i].Name < node.Children[j].Name
	})
	for _, child := range node.Children {
		if child.IsDir {
			sortTree(child)
		}
	}
}
