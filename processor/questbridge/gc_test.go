package questbridge

// =============================================================================
// UNIT TESTS — Stuck Quest GC: FindActiveLoop, handleLoopCancelled dispatch
// =============================================================================
// These tests exercise the stuck-quest GC helpers without requiring Docker or a
// live NATS connection.
//
// Counter-increment and mapping-cleanup behavior for handleLoopCancelled when a
// mapping IS found are covered by the integration tests (TestLoopFailure*) in
// component_test.go, because those paths require a live GraphClient.
//
// Run with: go test ./processor/questbridge/ -run TestGC -v
// =============================================================================

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/nats-io/nats.go/jetstream"
)

// =============================================================================
// FindActiveLoop
// =============================================================================

// TestGCFindActiveLoop_Hit verifies that a mapping stored in activeLoops is
// returned with found=true and the correct loopID.
func TestGCFindActiveLoop_Hit(t *testing.T) {
	comp := &Component{}

	questKey := "test.dev.game.board1.quest.abc123"
	loopID := "loop-xyz"
	comp.activeLoops.Store(questKey, &QuestLoopMapping{LoopID: loopID})

	got, found := comp.FindActiveLoop(questKey)
	if !found {
		t.Fatal("FindActiveLoop: expected found=true, got false")
	}
	if got != loopID {
		t.Errorf("FindActiveLoop: loopID = %q, want %q", got, loopID)
	}
}

// TestGCFindActiveLoop_Miss verifies that an absent key returns found=false
// and an empty loop ID without touching any external state.
func TestGCFindActiveLoop_Miss(t *testing.T) {
	comp := &Component{}

	loopID, found := comp.FindActiveLoop("test.dev.game.board1.quest.nonexistent")
	if found {
		t.Errorf("FindActiveLoop: expected found=false for unknown key, got loopID=%q", loopID)
	}
	if loopID != "" {
		t.Errorf("FindActiveLoop: expected empty loopID, got %q", loopID)
	}
}

// TestGCFindActiveLoop_NilMapping verifies that a nil *QuestLoopMapping value
// stored in activeLoops is treated as a miss (nil pointer safety guard).
func TestGCFindActiveLoop_NilMapping(t *testing.T) {
	comp := &Component{}

	questKey := "test.dev.game.board1.quest.nilentry"
	// Simulates a corrupt or partially written entry.
	comp.activeLoops.Store(questKey, (*QuestLoopMapping)(nil))

	loopID, found := comp.FindActiveLoop(questKey)
	if found {
		t.Errorf("FindActiveLoop: expected found=false for nil mapping, got loopID=%q", loopID)
	}
	if loopID != "" {
		t.Errorf("FindActiveLoop: expected empty loopID for nil mapping, got %q", loopID)
	}
}

// TestGCFindActiveLoop_MultipleKeys verifies that independent keys in the
// shared sync.Map do not interfere with each other.
func TestGCFindActiveLoop_MultipleKeys(t *testing.T) {
	comp := &Component{}

	pairs := []struct{ key, loop string }{
		{"test.dev.game.board1.quest.q1", "loop-aaa"},
		{"test.dev.game.board1.quest.q2", "loop-bbb"},
		{"test.dev.game.board1.quest.q3", "loop-ccc"},
	}
	for _, p := range pairs {
		comp.activeLoops.Store(p.key, &QuestLoopMapping{LoopID: p.loop})
	}

	for _, p := range pairs {
		t.Run(p.key, func(t *testing.T) {
			got, found := comp.FindActiveLoop(p.key)
			if !found {
				t.Errorf("FindActiveLoop(%q): expected found=true", p.key)
			}
			if got != p.loop {
				t.Errorf("FindActiveLoop(%q): loopID = %q, want %q", p.key, got, p.loop)
			}
		})
	}
}

// =============================================================================
// handleLoopCancelled — mapping-miss early return
// =============================================================================

// TestGCHandleLoopCancelled_NoMapping verifies that handleLoopCancelled returns
// early without modifying counters when there is no mapping for the event's
// TaskID. This exercises the "orphan loop" path without requiring a live graph.
func TestGCHandleLoopCancelled_NoMapping(t *testing.T) {
	comp := gcComponent(t)
	ctx := context.Background()

	event := &agentic.LoopCancelledEvent{
		LoopID:      "loop-orphan",
		TaskID:      "test.dev.game.board1.quest.orphan",
		Outcome:     "cancelled",
		CancelledBy: "admin",
		CancelledAt: time.Now(),
	}

	beforeFailed := comp.loopsFailed.Load()
	beforeErrors := comp.errorsCount.Load()
	comp.handleLoopCancelled(ctx, event)

	if comp.loopsFailed.Load() != beforeFailed {
		t.Errorf("loopsFailed incremented for orphan loop: before=%d after=%d",
			beforeFailed, comp.loopsFailed.Load())
	}
	if comp.errorsCount.Load() != beforeErrors {
		t.Errorf("errorsCount changed for orphan loop: before=%d after=%d",
			beforeErrors, comp.errorsCount.Load())
	}
}

// =============================================================================
// handleLoopCompleted — BaseMessage dispatch for LoopCancelledEvent
// =============================================================================

// TestGCHandleLoopCompleted_DispatchesCancelledEvent verifies that a
// LoopCancelledEvent wrapped in a BaseMessage envelope is correctly dispatched
// by handleLoopCompleted to handleLoopCancelled (observable: no unhandled error
// counter increment for the dispatch step; the missing-mapping path returns
// early cleanly).
func TestGCHandleLoopCompleted_DispatchesCancelledEvent(t *testing.T) {
	comp := gcComponent(t)
	ctx := context.Background()

	// No mapping stored — handleLoopCancelled returns early after the dispatch.
	event := &agentic.LoopCancelledEvent{
		LoopID:      "loop-dispatch-check",
		TaskID:      "test.dev.game.board1.quest.dispatch",
		Outcome:     "cancelled",
		CancelledBy: "api",
		CancelledAt: time.Now(),
	}

	baseMsg := message.NewBaseMessage(event.Schema(), event, "test")
	data, err := json.Marshal(baseMsg)
	if err != nil {
		t.Fatalf("failed to marshal LoopCancelledEvent BaseMessage: %v", err)
	}

	// errorsCount must NOT increment: a LoopCancelledEvent should never fall
	// through to the "unexpected payload type" error branch.
	beforeErrors := comp.errorsCount.Load()
	comp.handleLoopCompleted(ctx, data)

	if comp.errorsCount.Load() != beforeErrors {
		t.Errorf("errorsCount incremented — LoopCancelledEvent was not recognised by handleLoopCompleted: before=%d after=%d",
			beforeErrors, comp.errorsCount.Load())
	}
}

// TestGCHandleLoopCompleted_UnknownPayloadIncrementsError verifies that an
// unrecognised payload type increments errorsCount (proving the dispatch guard
// works for non-cancel events).
func TestGCHandleLoopCompleted_UnknownPayloadIncrementsError(t *testing.T) {
	comp := gcComponent(t)
	ctx := context.Background()

	// Build a BaseMessage with a completely different payload type — use a
	// LoopFailedEvent (already registered) to get a valid envelope, then
	// marshal it as if it were from a different schema domain so the payload
	// registry won't match either LoopCompletedEvent or LoopCancelledEvent.
	// The easiest approach: send raw JSON that decodes to a BaseMessage whose
	// schema matches no registered payload.
	raw := []byte(`{"schema":{"domain":"unknown","category":"bogus","version":"0"},"payload":{}}`)

	beforeErrors := comp.errorsCount.Load()
	comp.handleLoopCompleted(ctx, raw)

	if comp.errorsCount.Load() != beforeErrors+1 {
		t.Errorf("errorsCount: expected %d, got %d (unknown payload should increment error counter)",
			beforeErrors+1, comp.errorsCount.Load())
	}
}

// =============================================================================
// HELPERS
// =============================================================================

// stubKeyValue is a minimal jetstream.KeyValue stub. It returns ErrKeyNotFound
// on Get (simulating no KV mapping present) and silently succeeds on Delete.
// All other methods panic — they are not exercised by the GC unit tests.
//
// This prevents nil pointer dereferences in findMapping (bucket.Get) and
// cleanupMapping (bucket.Delete) when the quest-loop bucket is not backed by
// a live NATS connection.
type stubKeyValue struct{}

func (stubKeyValue) Get(_ context.Context, _ string) (jetstream.KeyValueEntry, error) {
	return nil, jetstream.ErrKeyNotFound
}

func (stubKeyValue) Delete(_ context.Context, _ string, _ ...jetstream.KVDeleteOpt) error {
	return nil
}

func (stubKeyValue) GetRevision(_ context.Context, _ string, _ uint64) (jetstream.KeyValueEntry, error) {
	panic("stubKeyValue.GetRevision not implemented")
}

func (stubKeyValue) Create(_ context.Context, _ string, _ []byte, _ ...jetstream.KVCreateOpt) (uint64, error) {
	panic("stubKeyValue.Create not implemented")
}

func (stubKeyValue) Update(_ context.Context, _ string, _ []byte, _ uint64) (uint64, error) {
	panic("stubKeyValue.Update not implemented")
}

func (stubKeyValue) Put(_ context.Context, _ string, _ []byte) (uint64, error) {
	panic("stubKeyValue.Put not implemented")
}

func (stubKeyValue) PutString(_ context.Context, _ string, _ string) (uint64, error) {
	panic("stubKeyValue.PutString not implemented")
}

func (stubKeyValue) Purge(_ context.Context, _ string, _ ...jetstream.KVDeleteOpt) error {
	panic("stubKeyValue.Purge not implemented")
}

func (stubKeyValue) Watch(_ context.Context, _ string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	panic("stubKeyValue.Watch not implemented")
}

func (stubKeyValue) WatchAll(_ context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	panic("stubKeyValue.WatchAll not implemented")
}

func (stubKeyValue) WatchFiltered(_ context.Context, _ []string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	panic("stubKeyValue.WatchFiltered not implemented")
}

func (stubKeyValue) Keys(_ context.Context, _ ...jetstream.WatchOpt) ([]string, error) {
	panic("stubKeyValue.Keys not implemented")
}

func (stubKeyValue) ListKeys(_ context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyLister, error) {
	panic("stubKeyValue.ListKeys not implemented")
}

func (stubKeyValue) ListKeysFiltered(_ context.Context, _ ...string) (jetstream.KeyLister, error) {
	panic("stubKeyValue.ListKeysFiltered not implemented")
}

func (stubKeyValue) History(_ context.Context, _ string, _ ...jetstream.WatchOpt) ([]jetstream.KeyValueEntry, error) {
	panic("stubKeyValue.History not implemented")
}

func (stubKeyValue) Bucket() string { return "stub-quest-loops" }

func (stubKeyValue) PurgeDeletes(_ context.Context, _ ...jetstream.KVPurgeOpt) error {
	panic("stubKeyValue.PurgeDeletes not implemented")
}

func (stubKeyValue) Status(_ context.Context) (jetstream.KeyValueStatus, error) {
	panic("stubKeyValue.Status not implemented")
}

// gcComponent returns a Component with only the dependencies required by the
// stuck-quest GC unit tests:
//   - logger (discards output)
//   - questLoopsBucket (stub: Get → ErrKeyNotFound, Delete → no-op)
//
// The graph field is intentionally left nil. Tests that call handleLoopCancelled
// with a found mapping will panic because failQuest reaches graph.GetQuest.
// Those paths are covered by the integration tests (TestLoopFailure*).
func gcComponent(t *testing.T) *Component {
	t.Helper()
	return &Component{
		logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		questLoopsBucket: stubKeyValue{},
	}
}
