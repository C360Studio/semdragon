package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", ":8090", "HTTP listen address")
	workspace := flag.String("workspace", "/workspace", "Root workspace directory")
	reposDir := flag.String("repos-dir", "/repos", "Directory containing bare/shared git repositories")
	defaultTimeout := flag.Duration("timeout", 30*time.Second, "Default command execution timeout")
	maxTimeout := flag.Duration("max-timeout", 5*time.Minute, "Maximum allowed command timeout")
	cleanupInterval := flag.Duration("cleanup-interval", 1*time.Hour, "Interval for orphan workspace cleanup")
	cleanupAge := flag.Duration("cleanup-age", 24*time.Hour, "Remove workspaces older than this")
	maxOutputBytes := flag.Int("max-output", 100*1024, "Maximum stdout/stderr capture size in bytes")
	maxFileSize := flag.Int64("max-file-size", 1*1024*1024, "Maximum file size for write operations in bytes")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// Ensure workspace root exists.
	if err := os.MkdirAll(*workspace, 0o755); err != nil {
		slog.Error("failed to create workspace root", "path", *workspace, "error", err)
		os.Exit(1)
	}

	// Ensure repos dir exists.
	if err := os.MkdirAll(*reposDir, 0o755); err != nil {
		slog.Error("failed to create repos dir", "path", *reposDir, "error", err)
		os.Exit(1)
	}

	// Scan repos dir: validate each subdir is a git repo or init an empty one.
	scanReposDir(*reposDir, logger)

	srv := &Server{
		workspace:      *workspace,
		reposDir:       *reposDir,
		defaultTimeout: *defaultTimeout,
		maxTimeout:     *maxTimeout,
		maxOutputBytes: *maxOutputBytes,
		maxFileSize:    *maxFileSize,
		logger:         logger,
		questRepos:     make(map[string]string),
		repoMutexes:    make(map[string]*repoMutex),
	}

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	httpServer := &http.Server{
		Addr:         *addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: *maxTimeout + 5*time.Second, // Allow for exec timeout + overhead
		IdleTimeout:  60 * time.Second,
	}

	// Start orphan workspace cleanup.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.CleanupLoop(ctx, *cleanupInterval, *cleanupAge)

	// Graceful shutdown.
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("shutting down sandbox server")
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown error", "error", err)
		}
	}()

	slog.Info("sandbox server starting", "addr", *addr, "workspace", *workspace, "repos_dir", *reposDir)
	fmt.Fprintf(os.Stderr, "sandbox server listening on %s\n", *addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

// scanReposDir inspects each subdirectory of dir. If it already contains a
// .git directory it is treated as a valid repo and logged. If the directory is
// completely empty, git init is run so the sandbox can immediately create
// worktrees from it.
func scanReposDir(dir string, logger *slog.Logger) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		logger.Warn("repos-dir: could not read directory", "path", dir, "error", err)
		return
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		repoPath := filepath.Join(dir, e.Name())
		gitDir := filepath.Join(repoPath, ".git")

		if _, statErr := os.Stat(gitDir); statErr == nil {
			// Already a git repo.
			logger.Info("repos-dir: discovered repo", "name", e.Name(), "path", repoPath)
			continue
		}

		// Check if the directory is empty — if so, initialise it.
		children, readErr := os.ReadDir(repoPath)
		if readErr != nil {
			logger.Warn("repos-dir: cannot read subdir", "name", e.Name(), "error", readErr)
			continue
		}
		if len(children) == 0 {
			result := execCommand(
				context.Background(),
				repoPath,
				"git init && git commit --allow-empty -m 'initial commit'",
				15*time.Second,
				4096,
			)
			if result.ExitCode != 0 {
				logger.Warn("repos-dir: git init failed", "name", e.Name(), "stderr", result.Stderr)
			} else {
				logger.Info("repos-dir: initialised empty repo", "name", e.Name(), "path", repoPath)
			}
		} else {
			logger.Warn("repos-dir: subdir has no .git and is not empty — skipping", "name", e.Name())
		}
	}
}
