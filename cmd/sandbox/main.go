package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", ":8090", "HTTP listen address")
	workspace := flag.String("workspace", "/workspace", "Root workspace directory")
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

	srv := &Server{
		workspace:      *workspace,
		defaultTimeout: *defaultTimeout,
		maxTimeout:     *maxTimeout,
		maxOutputBytes: *maxOutputBytes,
		maxFileSize:    *maxFileSize,
		logger:         logger,
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

	slog.Info("sandbox server starting", "addr", *addr, "workspace", *workspace)
	fmt.Fprintf(os.Stderr, "sandbox server listening on %s\n", *addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
