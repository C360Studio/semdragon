//go:build integration

package api

// =============================================================================
// INTEGRATION TESTS - SSE Handler
// =============================================================================
// These tests require Docker for NATS via testcontainers.
// Run with: go test -tags=integration ./service/api/...
// =============================================================================

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go/jetstream"
)

func newSSETestService(t *testing.T) (*Service, *natsclient.Client) {
	t.Helper()

	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage())
	client := testClient.Client

	boardConfig := &domain.BoardConfig{
		Org:      "test",
		Platform: "dev",
		Board:    "board1",
	}

	// Ensure the board bucket exists
	ctx := context.Background()
	_, err := client.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket: boardConfig.BucketName(),
	})
	if err != nil {
		t.Fatalf("create bucket: %v", err)
	}

	svc := &Service{
		nats:        client,
		boardConfig: boardConfig,
		config:      Config{Board: "board1", MaxEntities: 100},
		logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	return svc, client
}

// readSSEEvent reads the next SSE event from a buffered reader.
// Returns event type and data payload.
func readSSEEvent(t *testing.T, scanner *bufio.Scanner, timeout time.Duration) (eventType, data string) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "event: ") {
				eventType = strings.TrimPrefix(line, "event: ")
			} else if strings.HasPrefix(line, "data: ") {
				data = strings.TrimPrefix(line, "data: ")
			} else if line == "" && eventType != "" {
				return
			}
		}
	}()

	select {
	case <-done:
		return eventType, data
	case <-time.After(timeout):
		t.Fatal("timeout waiting for SSE event")
		return "", ""
	}
}

func TestHandleEvents_SSEHeaders(t *testing.T) {
	svc, _ := newSSETestService(t)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/game/events", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	// Run handler in background; cancel after we get the first flush.
	// handlerDone signals when the goroutine exits so we can safely read rec.
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		svc.handleEvents(rec, req)
	}()

	// Wait for headers to be set, then cancel and wait for handler to exit.
	time.Sleep(200 * time.Millisecond)
	cancel()
	<-handlerDone

	resp := rec.Result()
	defer resp.Body.Close()

	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", got)
	}
	if got := resp.Header.Get("Connection"); got != "keep-alive" {
		t.Errorf("Connection = %q, want keep-alive", got)
	}
	if got := resp.Header.Get("X-Accel-Buffering"); got != "no" {
		t.Errorf("X-Accel-Buffering = %q, want no", got)
	}
}

func TestHandleEvents_ConnectedEvent(t *testing.T) {
	svc, _ := newSSETestService(t)

	server := httptest.NewServer(http.HandlerFunc(svc.handleEvents))
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	eventType, data := readSSEEvent(t, scanner, 5*time.Second)

	if eventType != "connected" {
		t.Errorf("first event type = %q, want connected", eventType)
	}

	var payload map[string]string
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		t.Fatalf("unmarshal connected data: %v", err)
	}
	if payload["message"] != "Watching for changes" {
		t.Errorf("connected message = %q, want 'Watching for changes'", payload["message"])
	}
}

func TestHandleEvents_KVChange(t *testing.T) {
	svc, client := newSSETestService(t)
	bucketName := svc.boardConfig.BucketName()

	server := httptest.NewServer(http.HandlerFunc(svc.handleEvents))
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)

	// Read connected event
	readSSEEvent(t, scanner, 5*time.Second)

	// Read initial_sync_complete event
	evtType, _ := readSSEEvent(t, scanner, 5*time.Second)
	if evtType != "kv_change" {
		t.Fatalf("expected kv_change for initial_sync_complete, got %q", evtType)
	}

	// Write a KV entry
	ctx := context.Background()
	kv, err := client.GetKeyValueBucket(ctx, bucketName)
	if err != nil {
		t.Fatalf("get bucket: %v", err)
	}

	testEntity := map[string]string{"name": "test-agent", "status": "idle"}
	entityJSON, _ := json.Marshal(testEntity)
	if _, err := kv.Put(ctx, "test.dev.game.board1.agent.agent-1", entityJSON); err != nil {
		t.Fatalf("put: %v", err)
	}

	// Read the kv_change event
	evtType, data := readSSEEvent(t, scanner, 5*time.Second)
	if evtType != "kv_change" {
		t.Fatalf("event type = %q, want kv_change", evtType)
	}

	var event kvWatchEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}

	if event.Key != "test.dev.game.board1.agent.agent-1" {
		t.Errorf("key = %q, want test.dev.game.board1.agent.agent-1", event.Key)
	}
	if event.Operation != "create" {
		t.Errorf("operation = %q, want create", event.Operation)
	}
	if event.Revision != 1 {
		t.Errorf("revision = %d, want 1", event.Revision)
	}
	if event.Bucket != bucketName {
		t.Errorf("bucket = %q, want %q", event.Bucket, bucketName)
	}
}

func TestHandleEvents_InitialSyncComplete(t *testing.T) {
	svc, _ := newSSETestService(t)

	server := httptest.NewServer(http.HandlerFunc(svc.handleEvents))
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)

	// Read connected event
	readSSEEvent(t, scanner, 5*time.Second)

	// Read initial_sync_complete
	_, data := readSSEEvent(t, scanner, 5*time.Second)

	var event kvWatchEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if event.Operation != "initial_sync_complete" {
		t.Errorf("operation = %q, want initial_sync_complete", event.Operation)
	}
}

func TestHandleEvents_DeleteOperation(t *testing.T) {
	svc, client := newSSETestService(t)
	bucketName := svc.boardConfig.BucketName()

	// Pre-populate a key so we can delete it
	ctx := context.Background()
	kv, err := client.GetKeyValueBucket(ctx, bucketName)
	if err != nil {
		t.Fatalf("get bucket: %v", err)
	}
	if _, err := kv.Put(ctx, "test.dev.game.board1.quest.q1", []byte(`{"title":"test"}`)); err != nil {
		t.Fatalf("put: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(svc.handleEvents))
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)

	// Read connected
	readSSEEvent(t, scanner, 5*time.Second)
	// Read the existing key (create event)
	readSSEEvent(t, scanner, 5*time.Second)
	// Read initial_sync_complete
	readSSEEvent(t, scanner, 5*time.Second)

	// Delete the key
	if err := kv.Delete(ctx, "test.dev.game.board1.quest.q1"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Read delete event
	evtType, data := readSSEEvent(t, scanner, 5*time.Second)
	if evtType != "kv_change" {
		t.Fatalf("event type = %q, want kv_change", evtType)
	}

	var event kvWatchEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if event.Operation != "delete" {
		t.Errorf("operation = %q, want delete", event.Operation)
	}
	if event.Key != "test.dev.game.board1.quest.q1" {
		t.Errorf("key = %q, want test.dev.game.board1.quest.q1", event.Key)
	}
}

func TestHandleEvents_UpdateOperation(t *testing.T) {
	svc, client := newSSETestService(t)
	bucketName := svc.boardConfig.BucketName()

	// Pre-populate a key
	ctx := context.Background()
	kv, err := client.GetKeyValueBucket(ctx, bucketName)
	if err != nil {
		t.Fatalf("get bucket: %v", err)
	}
	if _, err := kv.Put(ctx, "test.dev.game.board1.agent.a1", []byte(`{"status":"idle"}`)); err != nil {
		t.Fatalf("put: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(svc.handleEvents))
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)

	// Read connected, existing key, initial_sync_complete
	readSSEEvent(t, scanner, 5*time.Second)
	readSSEEvent(t, scanner, 5*time.Second)
	readSSEEvent(t, scanner, 5*time.Second)

	// Update the key
	if _, err := kv.Put(ctx, "test.dev.game.board1.agent.a1", []byte(`{"status":"busy"}`)); err != nil {
		t.Fatalf("put update: %v", err)
	}

	_, data := readSSEEvent(t, scanner, 5*time.Second)

	var event kvWatchEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if event.Operation != "update" {
		t.Errorf("operation = %q, want update", event.Operation)
	}
	if event.Revision != 2 {
		t.Errorf("revision = %d, want 2", event.Revision)
	}

	// Verify value contains the updated data
	var value map[string]string
	if err := json.Unmarshal(event.Value, &value); err != nil {
		t.Fatalf("unmarshal value: %v", err)
	}
	if value["status"] != "busy" {
		t.Errorf("value.status = %q, want busy", value["status"])
	}
}

func TestDetectOperation(t *testing.T) {
	tests := []struct {
		name      string
		op        jetstream.KeyValueOp
		revision  uint64
		wantOp    string
	}{
		{"create", jetstream.KeyValuePut, 1, "create"},
		{"update", jetstream.KeyValuePut, 5, "update"},
		{"delete", jetstream.KeyValueDelete, 3, "delete"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &mockKVEntry{op: tt.op, revision: tt.revision}
			got := detectOperation(entry)
			if got != tt.wantOp {
				t.Errorf("detectOperation() = %q, want %q", got, tt.wantOp)
			}
		})
	}
}

// mockKVEntry implements jetstream.KeyValueEntry for unit testing detectOperation.
type mockKVEntry struct {
	op       jetstream.KeyValueOp
	revision uint64
}

func (m *mockKVEntry) Bucket() string                       { return "" }
func (m *mockKVEntry) Key() string                          { return "" }
func (m *mockKVEntry) Value() []byte                        { return nil }
func (m *mockKVEntry) Revision() uint64                     { return m.revision }
func (m *mockKVEntry) Created() time.Time                   { return time.Time{} }
func (m *mockKVEntry) Delta() uint64                        { return 0 }
func (m *mockKVEntry) Operation() jetstream.KeyValueOp      { return m.op }
