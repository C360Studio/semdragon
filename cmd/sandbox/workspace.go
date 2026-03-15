package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// CleanupLoop periodically removes orphaned workspace directories older than maxAge.
// This catches cases where questbridge crashes before sending a cleanup request.
func (s *Server) CleanupLoop(ctx context.Context, interval, maxAge time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.cleanupOrphanWorkspaces(ctx, maxAge)
		}
	}
}

// cleanupOrphanWorkspaces removes quest workspace directories that haven't been
// modified in longer than maxAge. For repo-backed workspaces it runs
// git worktree remove and deletes the quest branch; for plain workspaces it
// falls back to os.RemoveAll.
func (s *Server) cleanupOrphanWorkspaces(ctx context.Context, maxAge time.Duration) {
	entries, err := os.ReadDir(s.workspace)
	if err != nil {
		slog.Warn("cleanup: failed to read workspace root", "error", err)
		return
	}

	cutoff := time.Now().Add(-maxAge)
	removed := 0

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}

		if !info.ModTime().Before(cutoff) {
			continue
		}

		questID := entry.Name()

		// Use removeWorkspace so that repo-backed worktrees are torn down
		// correctly (git worktree remove + branch delete).
		if removeErr := s.removeWorkspace(ctx, questID); removeErr != nil {
			slog.Warn("cleanup: failed to remove workspace",
				"dir", questID, "error", removeErr)
		} else {
			removed++
			slog.Info("cleanup: removed orphan workspace", "dir", questID)
		}
	}

	// Also prune stale entries from questRepos whose workspace directories
	// no longer exist (e.g., removed out-of-band).
	s.pruneStaleQuestRepos()

	if removed > 0 {
		slog.Info("cleanup: removed orphan workspaces", "count", removed)
	}
}

// pruneStaleQuestRepos removes entries from s.questRepos whose workspace
// directory no longer exists on disk. This prevents the map from growing
// unboundedly after out-of-band workspace removals.
func (s *Server) pruneStaleQuestRepos() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for questID := range s.questRepos {
		dir := filepath.Join(s.workspace, questID)
		if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
			delete(s.questRepos, questID)
			slog.Info("cleanup: pruned stale questRepos entry", "quest_id", questID)
		}
	}
}
