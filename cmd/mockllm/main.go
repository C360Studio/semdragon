// Package main implements a mock OpenAI-compatible chat completions server for
// E2E testing of the semdragons project. It responds to both the agentic-model
// component (which uses the go-openai SDK) and the DM chat handler (which calls
// {BaseURL}/chat/completions directly).
//
// PORT env var controls the listen address (default: 9090).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	port := os.Getenv("PORT")
	if port == "" {
		port = "9090"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)

	// agentic-model uses the go-openai SDK, which appends /chat/completions to
	// the configured BaseURL. DM chat's callOpenAICompat also appends
	// /chat/completions to the endpoint URL, so both paths must be handled.
	mux.HandleFunc("POST /v1/chat/completions", handleChatCompletions(logger))
	mux.HandleFunc("POST /chat/completions", handleChatCompletions(logger))

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	slog.Info("Mock LLM server starting", "addr", srv.Addr)

	stopCh := make(chan struct{})
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		slog.Info("Received signal, shutting down", "signal", sig)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("Shutdown error", "error", err)
		}
		close(stopCh)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}

	<-stopCh
	slog.Info("Mock LLM server stopped")
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	// Intentionally no body — callers check status code only.
	w.WriteHeader(http.StatusOK)
}
