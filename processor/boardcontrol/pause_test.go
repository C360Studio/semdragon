package boardcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"io"
	"testing"
	"time"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go/jetstream"
)

// =============================================================================
// Mock implementations
// =============================================================================

// mockKeyValue implements jetstream.KeyValue using function fields so each test
// can supply exactly the behavior it needs without coupling test cases together.
type mockKeyValue struct {
	getFn   func(ctx context.Context, key string) (jetstream.KeyValueEntry, error)
	putFn   func(ctx context.Context, key string, value []byte) (uint64, error)
	watchFn func(ctx context.Context, key string, opts ...jetstream.WatchOpt) (jetstream.KeyWatcher, error)
}

func (m *mockKeyValue) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	if m.getFn != nil {
		return m.getFn(ctx, key)
	}
	return nil, jetstream.ErrKeyNotFound
}

func (m *mockKeyValue) Put(ctx context.Context, key string, value []byte) (uint64, error) {
	if m.putFn != nil {
		return m.putFn(ctx, key, value)
	}
	return 1, nil
}

func (m *mockKeyValue) Watch(ctx context.Context, key string, opts ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	if m.watchFn != nil {
		return m.watchFn(ctx, key, opts...)
	}
	return newClosedWatcher(), nil
}

// Remaining jetstream.KeyValue methods — not called by pause.go.
func (m *mockKeyValue) GetRevision(_ context.Context, _ string, _ uint64) (jetstream.KeyValueEntry, error) {
	return nil, errors.New("not implemented")
}
func (m *mockKeyValue) Create(_ context.Context, _ string, _ []byte, _ ...jetstream.KVCreateOpt) (uint64, error) {
	return 0, errors.New("not implemented")
}
func (m *mockKeyValue) Update(_ context.Context, _ string, _ []byte, _ uint64) (uint64, error) {
	return 0, errors.New("not implemented")
}
func (m *mockKeyValue) Delete(_ context.Context, _ string, _ ...jetstream.KVDeleteOpt) error {
	return errors.New("not implemented")
}
func (m *mockKeyValue) Purge(_ context.Context, _ string, _ ...jetstream.KVDeleteOpt) error {
	return errors.New("not implemented")
}
func (m *mockKeyValue) WatchAll(_ context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	return nil, errors.New("not implemented")
}
func (m *mockKeyValue) WatchFiltered(_ context.Context, _ []string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	return nil, errors.New("not implemented")
}
func (m *mockKeyValue) PutString(_ context.Context, _ string, _ string) (uint64, error) {
	return 0, errors.New("not implemented")
}
func (m *mockKeyValue) Keys(_ context.Context, _ ...jetstream.WatchOpt) ([]string, error) {
	return nil, errors.New("not implemented")
}
func (m *mockKeyValue) ListKeys(_ context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyLister, error) {
	return nil, errors.New("not implemented")
}
func (m *mockKeyValue) ListKeysFiltered(_ context.Context, _ ...string) (jetstream.KeyLister, error) {
	return nil, errors.New("not implemented")
}
func (m *mockKeyValue) History(_ context.Context, _ string, _ ...jetstream.WatchOpt) ([]jetstream.KeyValueEntry, error) {
	return nil, errors.New("not implemented")
}
func (m *mockKeyValue) Bucket() string        { return "BOARD_CONTROL" }
func (m *mockKeyValue) PurgeDeletes(_ context.Context, _ ...jetstream.KVPurgeOpt) error {
	return errors.New("not implemented")
}
func (m *mockKeyValue) Status(_ context.Context) (jetstream.KeyValueStatus, error) {
	return nil, errors.New("not implemented")
}

// mockKeyWatcher implements jetstream.KeyWatcher using a channel so tests can
// drive state changes synchronously without goroutine races.
type mockKeyWatcher struct {
	updates chan jetstream.KeyValueEntry
	stopped bool
}

func newOpenWatcher() *mockKeyWatcher {
	return &mockKeyWatcher{
		updates: make(chan jetstream.KeyValueEntry, 16),
	}
}

// newClosedWatcher returns a watcher whose channel is already closed, which
// causes watchLoop to exit immediately — useful for tests that only need
// Start to succeed without a running goroutine.
func newClosedWatcher() *mockKeyWatcher {
	w := &mockKeyWatcher{
		updates: make(chan jetstream.KeyValueEntry, 1),
	}
	close(w.updates)
	return w
}

func (w *mockKeyWatcher) Updates() <-chan jetstream.KeyValueEntry { return w.updates }
func (w *mockKeyWatcher) Stop() error {
	w.stopped = true
	return nil
}

// mockKeyValueEntry implements jetstream.KeyValueEntry for in-memory test data.
type mockKeyValueEntry struct {
	key       string
	value     []byte
	revision  uint64
	operation jetstream.KeyValueOp
}

func newEntry(key string, value []byte) *mockKeyValueEntry {
	return &mockKeyValueEntry{key: key, value: value, revision: 1, operation: jetstream.KeyValuePut}
}

func (e *mockKeyValueEntry) Bucket() string                { return "BOARD_CONTROL" }
func (e *mockKeyValueEntry) Key() string                   { return e.key }
func (e *mockKeyValueEntry) Value() []byte                 { return e.value }
func (e *mockKeyValueEntry) Revision() uint64              { return e.revision }
func (e *mockKeyValueEntry) Created() time.Time            { return time.Time{} }
func (e *mockKeyValueEntry) Delta() uint64                 { return 0 }
func (e *mockKeyValueEntry) Operation() jetstream.KeyValueOp { return e.operation }

// =============================================================================
// Test helpers
// =============================================================================

// discardLogger returns a slog.Logger that discards all output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// disconnectedNATSClient returns a *natsclient.Client that has never connected.
// Calling Publish on it returns ErrNotConnected (non-fatal for Resume), which
// exercises the warning-log path without requiring a real NATS server.
func disconnectedNATSClient(t *testing.T) *natsclient.Client {
	t.Helper()
	c, err := natsclient.NewClient("nats://localhost:4222")
	if err != nil {
		t.Fatalf("natsclient.NewClient: %v", err)
	}
	return c
}

// marshalState encodes a BoardState to JSON bytes and panics on error —
// acceptable in test helpers where malformed data is a programmer mistake.
func marshalState(t *testing.T, st BoardState) []byte {
	t.Helper()
	b, err := json.Marshal(st)
	if err != nil {
		t.Fatalf("marshal BoardState: %v", err)
	}
	return b
}

// newControllerForTest constructs a Controller wired with the given mock bucket
// and a disconnected NATS client. Tests that exercise Pause or Resume must pass
// a bucket whose putFn is set; tests that don't need Put can leave it nil.
func newControllerForTest(t *testing.T, bucket jetstream.KeyValue) *Controller {
	t.Helper()
	return NewController(bucket, disconnectedNATSClient(t), discardLogger())
}

// =============================================================================
// TestController_Paused
// =============================================================================

func TestController_Paused(t *testing.T) {
	t.Run("default state is not paused", func(t *testing.T) {
		c := newControllerForTest(t, &mockKeyValue{})
		if c.Paused() {
			t.Error("Paused() = true; want false for a freshly created controller")
		}
	})
}

// =============================================================================
// TestController_State
// =============================================================================

func TestController_State(t *testing.T) {
	t.Run("reflects running state when not paused", func(t *testing.T) {
		c := newControllerForTest(t, &mockKeyValue{})
		st := c.State()
		if st.Paused {
			t.Error("State().Paused = true; want false")
		}
		if st.PausedAt != nil {
			t.Errorf("State().PausedAt = %v; want nil", *st.PausedAt)
		}
		if st.PausedBy != nil {
			t.Errorf("State().PausedBy = %v; want nil", *st.PausedBy)
		}
	})

	t.Run("reflects paused state after applyState", func(t *testing.T) {
		c := newControllerForTest(t, &mockKeyValue{})
		ts := "2026-03-03T00:00:00Z"
		actor := "dm"
		c.applyState(BoardState{Paused: true, PausedAt: &ts, PausedBy: &actor})

		st := c.State()
		if !st.Paused {
			t.Error("State().Paused = false; want true")
		}
		if st.PausedAt == nil || *st.PausedAt != ts {
			t.Errorf("State().PausedAt = %v; want %q", st.PausedAt, ts)
		}
		if st.PausedBy == nil || *st.PausedBy != actor {
			t.Errorf("State().PausedBy = %v; want %q", st.PausedBy, actor)
		}
	})

	t.Run("PausedAt and PausedBy are nil in running state", func(t *testing.T) {
		c := newControllerForTest(t, &mockKeyValue{})
		c.applyState(BoardState{Paused: false})
		st := c.State()
		if st.PausedAt != nil {
			t.Errorf("State().PausedAt = %v; want nil after running state", *st.PausedAt)
		}
		if st.PausedBy != nil {
			t.Errorf("State().PausedBy = %v; want nil after running state", *st.PausedBy)
		}
	})
}

// =============================================================================
// TestController_Pause
// =============================================================================

func TestController_Pause(t *testing.T) {
	tests := []struct {
		name      string
		actor     string
		wantActor bool // whether PausedBy should be non-nil
	}{
		{
			name:      "with named actor",
			actor:     "dungeon-master",
			wantActor: true,
		},
		{
			name:      "with empty actor",
			actor:     "",
			wantActor: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Truncate to the second boundary because Pause() formats the timestamp
		// with time.RFC3339 (whole-second precision). Using truncation avoids a
		// race where time.Now() is at sub-second precision ahead of the truncated
		// timestamp written into the state.
		before := time.Now().UTC().Truncate(time.Second)

			var capturedKey string
			var capturedValue []byte
			bucket := &mockKeyValue{
				putFn: func(_ context.Context, key string, value []byte) (uint64, error) {
					capturedKey = key
					capturedValue = value
					return 1, nil
				},
			}
			c := newControllerForTest(t, bucket)

			st, err := c.Pause(context.Background(), tt.actor)
			after := time.Now().UTC()

			if err != nil {
				t.Fatalf("Pause() error = %v; want nil", err)
			}

			// Returned state must reflect paused.
			if !st.Paused {
				t.Error("returned BoardState.Paused = false; want true")
			}

			// Actor field.
			if tt.wantActor {
				if st.PausedBy == nil || *st.PausedBy != tt.actor {
					t.Errorf("returned BoardState.PausedBy = %v; want %q", st.PausedBy, tt.actor)
				}
			} else {
				if st.PausedBy != nil {
					t.Errorf("returned BoardState.PausedBy = %v; want nil for empty actor", *st.PausedBy)
				}
			}

			// Timestamp must be a valid RFC3339 time within the test window.
			if st.PausedAt == nil {
				t.Fatal("returned BoardState.PausedAt = nil; want non-nil")
			}
			pausedAt, parseErr := time.Parse(time.RFC3339, *st.PausedAt)
			if parseErr != nil {
				t.Fatalf("PausedAt %q is not valid RFC3339: %v", *st.PausedAt, parseErr)
			}
			if pausedAt.Before(before) || pausedAt.After(after) {
				t.Errorf("PausedAt %v is outside test window [%v, %v]", pausedAt, before, after)
			}

			// KV must have been written with the correct key.
			if capturedKey != boardStateKey {
				t.Errorf("Put key = %q; want %q", capturedKey, boardStateKey)
			}

			// The value written to KV must round-trip to a consistent BoardState.
			var written BoardState
			if jsonErr := json.Unmarshal(capturedValue, &written); jsonErr != nil {
				t.Fatalf("unmarshal KV value: %v", jsonErr)
			}
			if !written.Paused {
				t.Error("written BoardState.Paused = false; want true")
			}

			// Atomic state must reflect the new pause.
			if !c.Paused() {
				t.Error("c.Paused() = false after Pause(); want true")
			}
			inMemory := c.State()
			if !inMemory.Paused {
				t.Error("c.State().Paused = false after Pause(); want true")
			}
		})
	}

	t.Run("propagates KV put error", func(t *testing.T) {
		putErr := errors.New("kv unavailable")
		bucket := &mockKeyValue{
			putFn: func(_ context.Context, _ string, _ []byte) (uint64, error) {
				return 0, putErr
			},
		}
		c := newControllerForTest(t, bucket)
		_, err := c.Pause(context.Background(), "actor")
		if err == nil {
			t.Fatal("Pause() error = nil; want non-nil when Put fails")
		}
		if !errors.Is(err, putErr) {
			t.Errorf("Pause() error = %v; want to wrap %v", err, putErr)
		}
		// Atomic state must remain unpaused when persistence failed.
		if c.Paused() {
			t.Error("c.Paused() = true after Put failure; want false")
		}
	})
}

// =============================================================================
// TestController_Resume
// =============================================================================

func TestController_Resume(t *testing.T) {
	t.Run("clears pause state and persists to KV", func(t *testing.T) {
		var capturedKey string
		var capturedValue []byte
		bucket := &mockKeyValue{
			putFn: func(_ context.Context, key string, value []byte) (uint64, error) {
				capturedKey = key
				capturedValue = value
				return 2, nil
			},
		}
		c := newControllerForTest(t, bucket)

		// Start paused so Resume has something to change.
		ts := "2026-03-03T00:00:00Z"
		actor := "dm"
		c.applyState(BoardState{Paused: true, PausedAt: &ts, PausedBy: &actor})

		// Resume — publish will fail because the client is not connected; this
		// is non-fatal and the state update must still succeed.
		st, err := c.Resume(context.Background())
		if err != nil {
			t.Fatalf("Resume() error = %v; want nil", err)
		}

		if st.Paused {
			t.Error("returned BoardState.Paused = true; want false")
		}

		// KV written with correct key.
		if capturedKey != boardStateKey {
			t.Errorf("Put key = %q; want %q", capturedKey, boardStateKey)
		}

		// Value round-trips to not-paused state.
		var written BoardState
		if jsonErr := json.Unmarshal(capturedValue, &written); jsonErr != nil {
			t.Fatalf("unmarshal KV value: %v", jsonErr)
		}
		if written.Paused {
			t.Error("written BoardState.Paused = true; want false")
		}

		// Atomic state cleared.
		if c.Paused() {
			t.Error("c.Paused() = true after Resume(); want false")
		}
		inMemory := c.State()
		if inMemory.Paused {
			t.Error("c.State().Paused = true after Resume(); want false")
		}
		if inMemory.PausedAt != nil {
			t.Errorf("c.State().PausedAt = %v; want nil after Resume()", *inMemory.PausedAt)
		}
		if inMemory.PausedBy != nil {
			t.Errorf("c.State().PausedBy = %v; want nil after Resume()", *inMemory.PausedBy)
		}
	})

	t.Run("propagates KV put error", func(t *testing.T) {
		putErr := errors.New("kv unavailable")
		bucket := &mockKeyValue{
			putFn: func(_ context.Context, _ string, _ []byte) (uint64, error) {
				return 0, putErr
			},
		}
		c := newControllerForTest(t, bucket)

		// Arrange: start paused.
		ts := "2026-03-03T00:00:00Z"
		c.applyState(BoardState{Paused: true, PausedAt: &ts})

		_, err := c.Resume(context.Background())
		if err == nil {
			t.Fatal("Resume() error = nil; want non-nil when Put fails")
		}
		if !errors.Is(err, putErr) {
			t.Errorf("Resume() error = %v; want to wrap %v", err, putErr)
		}
		// Atomic state must remain paused when persistence failed.
		if !c.Paused() {
			t.Error("c.Paused() = false after Put failure; want true (state not changed)")
		}
	})
}

// =============================================================================
// TestController_Start_NoExistingState
// =============================================================================

func TestController_Start_NoExistingState(t *testing.T) {
	t.Run("key not found defaults to running", func(t *testing.T) {
		bucket := &mockKeyValue{
			getFn: func(_ context.Context, _ string) (jetstream.KeyValueEntry, error) {
				return nil, jetstream.ErrKeyNotFound
			},
			watchFn: func(_ context.Context, _ string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
				return newClosedWatcher(), nil
			},
		}
		c := newControllerForTest(t, bucket)

		if err := c.Start(context.Background()); err != nil {
			t.Fatalf("Start() error = %v; want nil", err)
		}
		defer c.Stop()

		if c.Paused() {
			t.Error("Paused() = true after Start() with no existing key; want false")
		}
	})

	t.Run("string 'key not found' error defaults to running", func(t *testing.T) {
		// isKeyNotFound also checks the error string, so a wrapped error whose
		// message contains "key not found" must also be treated as absent.
		wrappedNotFound := errors.New("nats: key not found (10037)")
		bucket := &mockKeyValue{
			getFn: func(_ context.Context, _ string) (jetstream.KeyValueEntry, error) {
				return nil, wrappedNotFound
			},
			watchFn: func(_ context.Context, _ string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
				return newClosedWatcher(), nil
			},
		}
		c := newControllerForTest(t, bucket)

		if err := c.Start(context.Background()); err != nil {
			t.Fatalf("Start() error = %v; want nil for key-not-found", err)
		}
		defer c.Stop()

		if c.Paused() {
			t.Error("Paused() = true; want false when key absent")
		}
	})

	t.Run("unexpected get error is returned", func(t *testing.T) {
		getErr := errors.New("NATS connection lost")
		bucket := &mockKeyValue{
			getFn: func(_ context.Context, _ string) (jetstream.KeyValueEntry, error) {
				return nil, getErr
			},
		}
		c := newControllerForTest(t, bucket)

		err := c.Start(context.Background())
		if err == nil {
			t.Fatal("Start() error = nil; want non-nil for unexpected get error")
		}
		if !errors.Is(err, getErr) {
			t.Errorf("Start() error = %v; want to wrap %v", err, getErr)
		}
	})

	t.Run("watch error is returned", func(t *testing.T) {
		watchErr := errors.New("watch setup failed")
		bucket := &mockKeyValue{
			getFn: func(_ context.Context, _ string) (jetstream.KeyValueEntry, error) {
				return nil, jetstream.ErrKeyNotFound
			},
			watchFn: func(_ context.Context, _ string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
				return nil, watchErr
			},
		}
		c := newControllerForTest(t, bucket)

		err := c.Start(context.Background())
		if err == nil {
			t.Fatal("Start() error = nil; want non-nil when Watch fails")
		}
		if !errors.Is(err, watchErr) {
			t.Errorf("Start() error = %v; want to wrap %v", err, watchErr)
		}
	})
}

// =============================================================================
// TestController_Start_ExistingPausedState
// =============================================================================

func TestController_Start_ExistingPausedState(t *testing.T) {
	t.Run("loads paused state from KV", func(t *testing.T) {
		ts := "2026-03-03T12:00:00Z"
		actor := "test-dm"
		existing := BoardState{Paused: true, PausedAt: &ts, PausedBy: &actor}
		data := marshalState(t, existing)

		bucket := &mockKeyValue{
			getFn: func(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
				if key != boardStateKey {
					return nil, jetstream.ErrKeyNotFound
				}
				return newEntry(key, data), nil
			},
			watchFn: func(_ context.Context, _ string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
				return newClosedWatcher(), nil
			},
		}
		c := newControllerForTest(t, bucket)

		if err := c.Start(context.Background()); err != nil {
			t.Fatalf("Start() error = %v; want nil", err)
		}
		defer c.Stop()

		if !c.Paused() {
			t.Error("Paused() = false; want true when KV contains paused state")
		}
		st := c.State()
		if !st.Paused {
			t.Error("State().Paused = false; want true")
		}
		if st.PausedAt == nil || *st.PausedAt != ts {
			t.Errorf("State().PausedAt = %v; want %q", st.PausedAt, ts)
		}
		if st.PausedBy == nil || *st.PausedBy != actor {
			t.Errorf("State().PausedBy = %v; want %q", st.PausedBy, actor)
		}
	})

	t.Run("loads running state from KV", func(t *testing.T) {
		running := BoardState{Paused: false}
		data := marshalState(t, running)

		bucket := &mockKeyValue{
			getFn: func(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
				return newEntry(key, data), nil
			},
			watchFn: func(_ context.Context, _ string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
				return newClosedWatcher(), nil
			},
		}
		c := newControllerForTest(t, bucket)

		if err := c.Start(context.Background()); err != nil {
			t.Fatalf("Start() error = %v; want nil", err)
		}
		defer c.Stop()

		if c.Paused() {
			t.Error("Paused() = true; want false when KV contains running state")
		}
	})

	t.Run("invalid JSON in KV is silently ignored, defaults to running", func(t *testing.T) {
		bucket := &mockKeyValue{
			getFn: func(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
				return newEntry(key, []byte("not-json")), nil
			},
			watchFn: func(_ context.Context, _ string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
				return newClosedWatcher(), nil
			},
		}
		c := newControllerForTest(t, bucket)

		// Corrupt data should not cause an error — Start should succeed and leave
		// the controller in the default (running) state.
		if err := c.Start(context.Background()); err != nil {
			t.Fatalf("Start() error = %v; want nil for corrupt KV data", err)
		}
		defer c.Stop()

		if c.Paused() {
			t.Error("Paused() = true; want false when KV data is corrupt")
		}
	})
}

// =============================================================================
// TestController_Stop
// =============================================================================

func TestController_Stop(t *testing.T) {
	t.Run("stop is idempotent", func(t *testing.T) {
		watcher := newClosedWatcher()
		bucket := &mockKeyValue{
			getFn: func(_ context.Context, _ string) (jetstream.KeyValueEntry, error) {
				return nil, jetstream.ErrKeyNotFound
			},
			watchFn: func(_ context.Context, _ string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
				return watcher, nil
			},
		}
		c := newControllerForTest(t, bucket)

		if err := c.Start(context.Background()); err != nil {
			t.Fatalf("Start() error = %v", err)
		}

		// Calling Stop multiple times must not panic or deadlock.
		done := make(chan struct{})
		go func() {
			c.Stop()
			c.Stop()
			c.Stop()
			close(done)
		}()

		select {
		case <-done:
			// Success.
		case <-time.After(2 * time.Second):
			t.Fatal("Stop() deadlocked or timed out")
		}
	})

	t.Run("stop without start does not panic", func(t *testing.T) {
		c := newControllerForTest(t, &mockKeyValue{})
		// Stop before Start must be safe: the stopChan is created in NewController
		// and closing it when the wg count is zero simply unblocks Wait immediately.
		c.Stop()
	})
}

// =============================================================================
// TestController_WatcherUpdates
// =============================================================================

func TestController_WatcherUpdates(t *testing.T) {
	t.Run("watcher delivers state transitions to atomic fields", func(t *testing.T) {
		watcher := newOpenWatcher()

		bucket := &mockKeyValue{
			getFn: func(_ context.Context, _ string) (jetstream.KeyValueEntry, error) {
				return nil, jetstream.ErrKeyNotFound
			},
			watchFn: func(_ context.Context, _ string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
				return watcher, nil
			},
		}
		c := newControllerForTest(t, bucket)

		if err := c.Start(context.Background()); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer c.Stop()

		// Confirm starting state is not paused.
		if c.Paused() {
			t.Fatal("Paused() = true before watch update; want false")
		}

		// Deliver a nil sentinel (end-of-replay marker) — must be ignored.
		watcher.updates <- nil

		// Deliver a paused state update via the watcher channel.
		ts := "2026-03-03T15:00:00Z"
		actor := "watcher-test"
		pausedState := BoardState{Paused: true, PausedAt: &ts, PausedBy: &actor}
		data := marshalState(t, pausedState)
		watcher.updates <- newEntry(boardStateKey, data)

		// Poll with a timeout to avoid flaky races — the watchLoop goroutine
		// processes the channel asynchronously.
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if c.Paused() {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		if !c.Paused() {
			t.Error("Paused() = false after watcher delivered pause; want true")
		}

		st := c.State()
		if st.PausedBy == nil || *st.PausedBy != actor {
			t.Errorf("State().PausedBy = %v; want %q", st.PausedBy, actor)
		}

		// Deliver a resume update.
		resumeState := BoardState{Paused: false}
		resumeData := marshalState(t, resumeState)
		watcher.updates <- newEntry(boardStateKey, resumeData)

		deadline = time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if !c.Paused() {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		if c.Paused() {
			t.Error("Paused() = true after watcher delivered resume; want false")
		}
	})

	t.Run("invalid JSON in watcher entry is skipped", func(t *testing.T) {
		watcher := newOpenWatcher()

		bucket := &mockKeyValue{
			getFn: func(_ context.Context, _ string) (jetstream.KeyValueEntry, error) {
				return nil, jetstream.ErrKeyNotFound
			},
			watchFn: func(_ context.Context, _ string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
				return watcher, nil
			},
		}
		c := newControllerForTest(t, bucket)

		if err := c.Start(context.Background()); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer c.Stop()

		// Send corrupt JSON — watchLoop logs a warning and continues.
		watcher.updates <- newEntry(boardStateKey, []byte("bad-json"))

		// Follow with a valid pause update to confirm the loop is still running.
		ts := "2026-03-03T16:00:00Z"
		pausedState := BoardState{Paused: true, PausedAt: &ts}
		data := marshalState(t, pausedState)
		watcher.updates <- newEntry(boardStateKey, data)

		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if c.Paused() {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		if !c.Paused() {
			t.Error("Paused() = false; watchLoop should continue past corrupt entries")
		}
	})

	t.Run("closed watcher channel terminates watchLoop", func(t *testing.T) {
		watcher := newOpenWatcher()

		bucket := &mockKeyValue{
			getFn: func(_ context.Context, _ string) (jetstream.KeyValueEntry, error) {
				return nil, jetstream.ErrKeyNotFound
			},
			watchFn: func(_ context.Context, _ string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
				return watcher, nil
			},
		}
		c := newControllerForTest(t, bucket)

		if err := c.Start(context.Background()); err != nil {
			t.Fatalf("Start() error = %v", err)
		}

		// Closing the updates channel is the "connection closed" path in watchLoop.
		close(watcher.updates)

		done := make(chan struct{})
		go func() {
			c.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			// watchLoop exited cleanly.
		case <-time.After(2 * time.Second):
			t.Fatal("watchLoop did not exit after channel closed")
		}
	})
}

// =============================================================================
// TestIsKeyNotFound
// =============================================================================

func TestIsKeyNotFound(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		want    bool
	}{
		{
			name: "nil error returns false",
			err:  nil,
			want: false,
		},
		{
			name: "jetstream.ErrKeyNotFound returns true via errors.Is",
			err:  jetstream.ErrKeyNotFound,
			want: true,
		},
		{
			name: "wrapped jetstream.ErrKeyNotFound returns true",
			err:  errors.Join(errors.New("outer"), jetstream.ErrKeyNotFound),
			want: true,
		},
		{
			name: "string containing 'key not found' returns true",
			err:  errors.New("nats: key not found (10037)"),
			want: true,
		},
		{
			name: "unrelated error returns false",
			err:  errors.New("connection reset by peer"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isKeyNotFound(tt.err)
			if got != tt.want {
				t.Errorf("isKeyNotFound(%v) = %v; want %v", tt.err, got, tt.want)
			}
		})
	}
}

// =============================================================================
// TestResumeSubject
// =============================================================================

func TestResumeSubject(t *testing.T) {
	got := ResumeSubject()
	if got != "board.control.resumed" {
		t.Errorf("ResumeSubject() = %q; want %q", got, "board.control.resumed")
	}
}

// =============================================================================
// TestEnsureBucket
// =============================================================================

// mockNATSForBucket is a thin wrapper so we can intercept CreateKeyValueBucket
// without a real NATS server. Because natsclient.Client is a concrete struct
// with unexported fields we cannot embed or subtype it; instead EnsureBucket is
// tested via an interface that matches the single method it needs.
//
// NOTE: EnsureBucket takes *natsclient.Client, so we cannot inject a mock
// without changing the production signature. The test below therefore uses a
// disconnected client and verifies the error path — confirming that EnsureBucket
// correctly delegates to CreateKeyValueBucket and propagates errors.
func TestEnsureBucket(t *testing.T) {
	t.Run("returns error when client is not connected", func(t *testing.T) {
		// A disconnected client returns ErrNotConnected from CreateKeyValueBucket.
		nats := disconnectedNATSClient(t)
		_, err := EnsureBucket(context.Background(), nats)
		if err == nil {
			t.Fatal("EnsureBucket() error = nil; want non-nil for disconnected client")
		}
		if !errors.Is(err, natsclient.ErrNotConnected) {
			t.Errorf("EnsureBucket() error = %v; want to wrap ErrNotConnected", err)
		}
	})
}

// =============================================================================
// TestPauseCheckerInterface
// =============================================================================

// TestPauseCheckerInterface verifies that *Controller satisfies the PauseChecker
// interface at compile time. If the interface changes, this test will not compile.
func TestPauseCheckerInterface(t *testing.T) {
	t.Helper() // Compile-time interface satisfaction check — no assertions needed.
	var _ PauseChecker = (*Controller)(nil)
}
