package workspacerepo

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestRepo(t *testing.T) *WorkspaceRepo {
	t.Helper()
	repoDir := filepath.Join(t.TempDir(), "workspace.git")
	worktreesDir := filepath.Join(t.TempDir(), "quest-worktrees")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	repo := New(repoDir, worktreesDir, logger)
	if err := repo.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return repo
}

func TestInit_IdempotentCalls(t *testing.T) {
	repo := newTestRepo(t)
	// Second Init should be a no-op.
	if err := repo.Init(context.Background()); err != nil {
		t.Fatalf("second Init: %v", err)
	}
}

func TestCreateWorktree(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.CreateWorktree(ctx, "abc123"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	// Verify worktree directory exists with .git marker.
	worktree := repo.worktreePath("abc123")
	if _, err := os.Stat(filepath.Join(worktree, ".git")); err != nil {
		t.Fatalf("worktree .git missing: %v", err)
	}

	// Verify correct branch.
	out, err := repo.gitOutput(ctx, worktree, "branch", "--show-current")
	if err != nil {
		t.Fatalf("git branch: %v", err)
	}
	if got := strings.TrimSpace(out); got != "quest/abc123" {
		t.Errorf("branch = %q, want quest/abc123", got)
	}
}

func TestCreateWorktree_Idempotent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.CreateWorktree(ctx, "idem1"); err != nil {
		t.Fatalf("first CreateWorktree: %v", err)
	}
	// Second call should succeed (no-op).
	if err := repo.CreateWorktree(ctx, "idem1"); err != nil {
		t.Fatalf("second CreateWorktree: %v", err)
	}
}

func TestFinalizeWorktree(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.CreateWorktree(ctx, "fin1"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	// Write a file in the worktree.
	worktree := repo.worktreePath("fin1")
	if err := os.WriteFile(filepath.Join(worktree, "hello.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	hash, err := repo.FinalizeWorktree(ctx, "fin1", "agent-dragon")
	if err != nil {
		t.Fatalf("FinalizeWorktree: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty commit hash")
	}

	// Verify commit message.
	msg, err := repo.gitOutput(ctx, worktree, "log", "-1", "--pretty=%s")
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	if got := strings.TrimSpace(msg); got != "quest fin1 finalized by agent agent-dragon" {
		t.Errorf("commit message = %q", got)
	}
}

func TestFinalizeWorktree_NoChanges(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.CreateWorktree(ctx, "nochange"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	// Finalize with no changes should return HEAD hash without error.
	hash, err := repo.FinalizeWorktree(ctx, "nochange", "agent-1")
	if err != nil {
		t.Fatalf("FinalizeWorktree: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash even with no changes")
	}
}

func TestRemoveWorktree(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.CreateWorktree(ctx, "rem1"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	if err := repo.RemoveWorktree(ctx, "rem1"); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}

	// Directory should be gone.
	worktree := repo.worktreePath("rem1")
	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Errorf("worktree dir still exists after removal")
	}
}

func TestRemoveWorktree_AlreadyRemoved(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	// Removing a non-existent worktree should not error.
	if err := repo.RemoveWorktree(ctx, "nonexistent"); err != nil {
		t.Fatalf("RemoveWorktree on non-existent: %v", err)
	}
}

func TestListQuestFiles(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.CreateWorktree(ctx, "list1"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	worktree := repo.worktreePath("list1")
	if err := os.MkdirAll(filepath.Join(worktree, "pkg", "auth"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "pkg", "auth", "handler.go"), []byte("package auth"), 0o644); err != nil {
		t.Fatalf("write handler.go: %v", err)
	}
	// Stage files so git ls-files sees them.
	if err := repo.gitInDir(ctx, worktree, "add", "-A"); err != nil {
		t.Fatalf("git add: %v", err)
	}

	files, err := repo.ListQuestFiles("list1")
	if err != nil {
		t.Fatalf("ListQuestFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %+v", len(files), files)
	}
	// Sorted alphabetically.
	if files[0].Path != "main.go" {
		t.Errorf("files[0] = %s, want main.go", files[0].Path)
	}
	if files[1].Path != filepath.Join("pkg", "auth", "handler.go") {
		t.Errorf("files[1] = %s, want pkg/auth/handler.go", files[1].Path)
	}
}

func TestReadFile(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.CreateWorktree(ctx, "read1"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	worktree := repo.worktreePath("read1")
	content := []byte("hello world")
	os.WriteFile(filepath.Join(worktree, "test.txt"), content, 0o644)

	data, err := repo.ReadFile("read1", "test.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("content = %q, want %q", data, content)
	}
}

func TestReadFile_PathTraversal(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.CreateWorktree(ctx, "traverse1"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	_, err := repo.ReadFile("traverse1", "../../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestFileTree(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.CreateWorktree(ctx, "tree1"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	worktree := repo.worktreePath("tree1")
	if err := os.MkdirAll(filepath.Join(worktree, "src"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "README.md"), []byte("# test"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "src", "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	// Stage files so git ls-files sees them.
	if err := repo.gitInDir(ctx, worktree, "add", "-A"); err != nil {
		t.Fatalf("git add: %v", err)
	}

	tree, err := repo.FileTree("tree1")
	if err != nil {
		t.Fatalf("FileTree: %v", err)
	}

	// Should have dir "src" first, then file "README.md".
	if len(tree) != 2 {
		t.Fatalf("expected 2 root entries, got %d", len(tree))
	}
	if !tree[0].IsDir || tree[0].Name != "src" {
		t.Errorf("tree[0] = %+v, want dir 'src'", tree[0])
	}
	if tree[1].IsDir || tree[1].Name != "README.md" {
		t.Errorf("tree[1] = %+v, want file 'README.md'", tree[1])
	}
}

func TestMergeToMain(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.CreateWorktree(ctx, "merge1"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	// Write and commit a file on the quest branch.
	worktree := repo.worktreePath("merge1")
	os.WriteFile(filepath.Join(worktree, "feature.go"), []byte("package feature"), 0o644)
	if _, err := repo.FinalizeWorktree(ctx, "merge1", "agent-x"); err != nil {
		t.Fatalf("FinalizeWorktree: %v", err)
	}

	mergeHash, err := repo.MergeToMain(ctx, "merge1")
	if err != nil {
		t.Fatalf("MergeToMain: %v", err)
	}
	if mergeHash == "" {
		t.Fatal("expected non-empty merge hash")
	}

	// Verify the file is on main by checking via bare repo.
	out, err := repo.gitOutput(ctx, repo.repoDir, "show", "main:feature.go")
	if err != nil {
		t.Fatalf("git show main:feature.go: %v", err)
	}
	if strings.TrimSpace(out) != "package feature" {
		t.Errorf("feature.go on main = %q", out)
	}
}

func TestConcurrentCreateWorktree(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	ids := []string{"c1", "c2", "c3", "c4", "c5"}
	errs := make(chan error, len(ids))

	for _, id := range ids {
		go func(qid string) {
			errs <- repo.CreateWorktree(ctx, qid)
		}(id)
	}

	for range ids {
		if err := <-errs; err != nil {
			t.Errorf("concurrent CreateWorktree: %v", err)
		}
	}

	// Verify all worktrees exist.
	for _, id := range ids {
		if !repo.WorktreeExists(id) {
			t.Errorf("worktree %s missing after concurrent create", id)
		}
	}
}

func TestPruneCompleted(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	// Create two worktrees.
	if err := repo.CreateWorktree(ctx, "prune-ok"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	if err := repo.CreateWorktree(ctx, "prune-keep"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	statusFn := func(instanceID string) (string, time.Time) {
		switch instanceID {
		case "prune-ok":
			return "completed", time.Now().AddDate(0, 0, -60) // 60 days ago
		case "prune-keep":
			return "in_progress", time.Time{}
		default:
			return "", time.Time{}
		}
	}

	if err := repo.PruneCompleted(ctx, 30, statusFn); err != nil {
		t.Fatalf("PruneCompleted: %v", err)
	}

	// prune-ok should be removed.
	if repo.WorktreeExists("prune-ok") {
		t.Error("prune-ok worktree still exists after pruning")
	}
	// prune-keep should still exist.
	if !repo.WorktreeExists("prune-keep") {
		t.Error("prune-keep worktree was incorrectly pruned")
	}
}

func TestWorktreeExists(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if repo.WorktreeExists("nonexistent") {
		t.Error("WorktreeExists returned true for non-existent worktree")
	}

	if err := repo.CreateWorktree(ctx, "exists1"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	if !repo.WorktreeExists("exists1") {
		t.Error("WorktreeExists returned false after creation")
	}
}

func TestValidateQuestID(t *testing.T) {
	valid := []string{
		"abc123",
		"quest-abc-def",
		"MyQuest1",
		"a",
		"local.dev.game.board1.quest.abc123", // full entity ID with dots
		"c360.prod.game.board1.quest.xyz",
	}
	for _, id := range valid {
		if err := validateQuestID(id); err != nil {
			t.Errorf("validateQuestID(%q) = %v, want nil", id, err)
		}
	}

	invalid := []struct {
		id   string
		desc string
	}{
		{"", "empty string"},
		{"../etc", "path traversal"},
		{"-flag", "leading dash"},
		{"has/slash", "forward slash"},
		{`has\backslash`, "backslash"},
		{"has space", "space"},
	}
	for _, tc := range invalid {
		if err := validateQuestID(tc.id); err == nil {
			t.Errorf("validateQuestID(%q) = nil, want error (%s)", tc.id, tc.desc)
		}
	}
}

func TestMergedFiles(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.CreateWorktree(ctx, "mf1"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	worktree := repo.worktreePath("mf1")
	if err := os.WriteFile(filepath.Join(worktree, "alpha.go"), []byte("package main"), 0o644); err != nil {
		t.Fatalf("write alpha.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "beta.go"), []byte("package main"), 0o644); err != nil {
		t.Fatalf("write beta.go: %v", err)
	}

	if _, err := repo.FinalizeWorktree(ctx, "mf1", "agent-mf"); err != nil {
		t.Fatalf("FinalizeWorktree: %v", err)
	}

	mergeHash, err := repo.MergeToMain(ctx, "mf1")
	if err != nil {
		t.Fatalf("MergeToMain: %v", err)
	}

	files, err := repo.MergedFiles(ctx, mergeHash)
	if err != nil {
		t.Fatalf("MergedFiles: %v", err)
	}

	// Build a set for order-independent comparison.
	got := make(map[string]bool, len(files))
	for _, f := range files {
		got[f] = true
	}
	for _, want := range []string{"alpha.go", "beta.go"} {
		if !got[want] {
			t.Errorf("MergedFiles missing %q; got %v", want, files)
		}
	}
}

func TestListWorktreeIDs(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	ids := []string{"lw-one", "lw-two", "lw-three"}
	for _, id := range ids {
		if err := repo.CreateWorktree(ctx, id); err != nil {
			t.Fatalf("CreateWorktree(%s): %v", id, err)
		}
	}

	listed, err := repo.ListWorktreeIDs()
	if err != nil {
		t.Fatalf("ListWorktreeIDs: %v", err)
	}
	if len(listed) != 3 {
		t.Fatalf("expected 3 worktree IDs, got %d: %v", len(listed), listed)
	}
	listedSet := make(map[string]bool, len(listed))
	for _, id := range listed {
		listedSet[id] = true
	}
	for _, id := range ids {
		if !listedSet[id] {
			t.Errorf("ListWorktreeIDs missing %q; got %v", id, listed)
		}
	}

	// Remove one; verify count drops to 2.
	if err := repo.RemoveWorktree(ctx, "lw-two"); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}
	listed2, err := repo.ListWorktreeIDs()
	if err != nil {
		t.Fatalf("ListWorktreeIDs after remove: %v", err)
	}
	if len(listed2) != 2 {
		t.Fatalf("expected 2 worktree IDs after removal, got %d: %v", len(listed2), listed2)
	}
	if listedSet2 := func() map[string]bool {
		m := make(map[string]bool, len(listed2))
		for _, id := range listed2 {
			m[id] = true
		}
		return m
	}(); listedSet2["lw-two"] {
		t.Error("lw-two still listed after removal")
	}
}

func TestReadFile_AbsolutePathRejected(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.CreateWorktree(ctx, "abs1"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	_, err := repo.ReadFile("abs1", "/etc/passwd")
	if err == nil {
		t.Fatal("expected error when reading absolute path /etc/passwd, got nil")
	}
}

func TestCreateWorktree_InvalidID(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	invalid := []string{"../escape", "-flag"}
	for _, id := range invalid {
		if err := repo.CreateWorktree(ctx, id); err == nil {
			t.Errorf("CreateWorktree(%q) = nil, want error", id)
		}
	}
}

func TestReadFile_SymlinkEscape(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.CreateWorktree(ctx, "sym1"); err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	// Create a file outside the worktree that the symlink will point to.
	secretDir := t.TempDir()
	secretFile := filepath.Join(secretDir, "secret.txt")
	if err := os.WriteFile(secretFile, []byte("secret-data"), 0o644); err != nil {
		t.Fatalf("write secret file: %v", err)
	}

	// Place a symlink inside the worktree pointing at the secret file.
	worktree := repo.worktreePath("sym1")
	linkPath := filepath.Join(worktree, "escape.txt")
	if err := os.Symlink(secretFile, linkPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	// ReadFile must reject the symlink because its resolved target lies outside
	// the worktree root. The implementation uses filepath.EvalSymlinks before the
	// prefix check so the real on-disk path is what gets validated.
	_, err := repo.ReadFile("sym1", "escape.txt")
	if err == nil {
		t.Fatal("expected error: ReadFile should reject symlink pointing outside worktree")
	}
}

