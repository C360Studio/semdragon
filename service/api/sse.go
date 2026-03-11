package api

// SSE endpoint for real-time entity updates.
//
// Streams KV bucket changes via Server-Sent Events from the board's
// entity state bucket. Replaces the debug message-logger endpoint
// with a production-ready, per-service SSE handler.
//
// Wire format matches the message-logger for frontend compatibility:
//
//	event: connected
//	id: 1
//	data: {"message":"Watching for changes"}
//
//	event: kv_change
//	id: 42
//	data: {"bucket":"...","key":"...","operation":"update","value":{...},"revision":5,"timestamp":"..."}

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// kvWatchEvent represents a KV change event sent via SSE.
type kvWatchEvent struct {
	Bucket    string          `json:"bucket"`
	Key       string          `json:"key"`
	Operation string          `json:"operation"` // "create", "update", "delete", "initial_sync_complete"
	Value     json.RawMessage `json:"value,omitempty"`
	Revision  uint64          `json:"revision"`
	Timestamp time.Time       `json:"timestamp"`
}

// handleEvents streams real-time entity updates via SSE.
// GET /game/events
func (s *Service) handleEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	bucketName := s.boardConfig.BucketName()

	// SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// Get KV bucket
	kv, err := s.nats.GetKeyValueBucket(ctx, bucketName)
	if err != nil {
		s.sendSSEError(w, "Failed to access KV bucket", err)
		return
	}

	// Create watcher with an independent context so we control the lifecycle
	// exclusively via Stop(). Using the request context causes NATS to tear
	// down the subscription on disconnect, making Stop() return "invalid
	// subscription" — a benign but noisy race.
	watchCtx, watchCancel := context.WithCancel(context.Background())
	watcher, err := kv.WatchAll(watchCtx)
	if err != nil {
		watchCancel()
		s.sendSSEError(w, "Failed to create watcher", err)
		return
	}
	defer func() {
		watchCancel()
		if stopErr := watcher.Stop(); stopErr != nil {
			s.logger.Warn("failed to stop KV watcher", "error", stopErr)
		}
	}()

	// Buffered channel for backpressure
	eventChan := make(chan *kvWatchEvent, 100)
	go s.consumeWatchUpdates(ctx, watcher, bucketName, eventChan)

	// Stream events to client
	var eventID atomic.Uint64

	// Send connected event
	connData, _ := json.Marshal(map[string]string{
		"message": "Watching for changes",
	})
	fmt.Fprintf(w, "event: connected\nid: %d\ndata: %s\n\n", eventID.Add(1), connData)
	fmt.Fprintf(w, "retry: 5000\n\n")
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// Extend the write deadline before each write so the server's default
	// WriteTimeout (10s) doesn't kill long-lived SSE connections.
	rc := http.NewResponseController(w)
	extendDeadline := func() {
		_ = rc.SetWriteDeadline(time.Now().Add(30 * time.Second))
	}

	// Heartbeat keeps the connection alive during quiet periods.
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("SSE client disconnected")
			return

		case <-heartbeat.C:
			extendDeadline()
			fmt.Fprintf(w, ": keepalive\n\n")
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}

		case event, ok := <-eventChan:
			if !ok {
				errData, _ := json.Marshal(map[string]string{
					"error": "Watcher closed unexpectedly",
				})
				extendDeadline()
				fmt.Fprintf(w, "event: error\ndata: %s\n\n", errData)
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
				return
			}

			data, err := json.Marshal(event)
			if err != nil {
				s.logger.Error("failed to marshal KV event", "error", err)
				continue
			}

			extendDeadline()
			fmt.Fprintf(w, "event: kv_change\nid: %d\ndata: %s\n\n", eventID.Add(1), data)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}
}

// consumeWatchUpdates reads from the KV watcher and forwards events to the channel.
func (s *Service) consumeWatchUpdates(
	ctx context.Context,
	watcher jetstream.KeyWatcher,
	bucket string,
	eventChan chan *kvWatchEvent,
) {
	defer close(eventChan)

	for {
		select {
		case <-ctx.Done():
			return

		case entry, ok := <-watcher.Updates():
			if !ok {
				return
			}

			if entry == nil {
				// nil signals initial sync complete
				select {
				case eventChan <- &kvWatchEvent{
					Bucket:    bucket,
					Operation: "initial_sync_complete",
				}:
				case <-ctx.Done():
					return
				}
				continue
			}

			event := &kvWatchEvent{
				Bucket:    bucket,
				Key:       entry.Key(),
				Operation: detectOperation(entry),
				Revision:  entry.Revision(),
				Timestamp: entry.Created(),
			}

			if entry.Operation() != jetstream.KeyValueDelete {
				if json.Valid(entry.Value()) {
					event.Value = entry.Value()
				} else {
					event.Value, _ = json.Marshal(string(entry.Value()))
				}
			}

			select {
			case eventChan <- event:
			case <-ctx.Done():
				return
			default:
				s.logger.Warn("SSE buffer full, dropping event",
					"bucket", bucket,
					"key", entry.Key(),
					"revision", entry.Revision())
			}
		}
	}
}

// detectOperation determines the operation type from a KV entry.
func detectOperation(entry jetstream.KeyValueEntry) string {
	switch entry.Operation() {
	case jetstream.KeyValuePut:
		if entry.Revision() == 1 {
			return "create"
		}
		return "update"
	case jetstream.KeyValueDelete:
		return "delete"
	default:
		return "unknown"
	}
}

// sendSSEError sends an error event via SSE.
func (s *Service) sendSSEError(w http.ResponseWriter, message string, err error) {
	errorData := map[string]string{"error": message}
	if err != nil {
		errorData["details"] = err.Error()
	}
	data, _ := json.Marshal(errorData)
	fmt.Fprintf(w, "event: error\ndata: %s\n\n", data)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}
