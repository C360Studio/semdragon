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
			s.cleanupOrphanWorkspaces(maxAge)
		}
	}
}

// cleanupOrphanWorkspaces removes quest workspace directories that haven't been
// modified in longer than maxAge.
func (s *Server) cleanupOrphanWorkspaces(maxAge time.Duration) {
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

		if info.ModTime().Before(cutoff) {
			dir := filepath.Join(s.workspace, entry.Name())
			if removeErr := os.RemoveAll(dir); removeErr != nil {
				slog.Warn("cleanup: failed to remove workspace",
					"dir", entry.Name(), "error", removeErr)
			} else {
				removed++
			}
		}
	}

	if removed > 0 {
		slog.Info("cleanup: removed orphan workspaces", "count", removed)
	}
}
