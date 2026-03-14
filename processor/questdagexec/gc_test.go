package questdagexec

// =============================================================================
// GC / Stuck-Quest Unit Tests
//
// These tests cover the garbage-collection and failure-recovery handlers
// introduced in the stuck-quest GC feature:
//
//   - classifyCompletedEvent / classifyFailedEvent / classifyCancelledEvent
//   - parseLeadLoopCompletion (via a minimal jetstream.Msg mock)
//   - onReviewFailed    — retry logic (reviewRetries map)
//   - onSynthesisFailed — mechanical rollup fallback
//   - onClarificationFailed — reset node to NodeAssigned
//   - onDAGTimedOut     — cache cleanup and scan-by-ParentQuestID path
//   - pruneReviewRetries
//   - CancelDAGForQuest
//
// All tests are pure-unit: no NATS connection, no Docker.
// Integration tests for the live event loop live in component_test.go.
// =============================================================================

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// =============================================================================
// MOCK jetstream.Msg — minimal implementation for parseLeadLoopCompletion tests
// =============================================================================

// mockJetMsg implements jetstream.Msg so we can pass serialised payloads
// directly to parseLeadLoopCompletion without a live NATS connection.
// Only Data() is exercised by the code under test; the rest are no-ops.
type mockJetMsg struct {
	data []byte
}

func (m *mockJetMsg) Data() []byte                               { return m.data }
func (m *mockJetMsg) Subject() string                            { return "" }
func (m *mockJetMsg) Reply() string                              { return "" }
func (m *mockJetMsg) Headers() natsgo.Header                     { return nil }
func (m *mockJetMsg) Metadata() (*jetstream.MsgMetadata, error)  { return nil, nil }
func (m *mockJetMsg) Ack() error                                 { return nil }
func (m *mockJetMsg) DoubleAck(_ context.Context) error          { return nil }
func (m *mockJetMsg) Nak() error                                 { return nil }
func (m *mockJetMsg) NakWithDelay(_ time.Duration) error         { return nil }
func (m *mockJetMsg) InProgress() error                          { return nil }
func (m *mockJetMsg) Term() error                                { return nil }
func (m *mockJetMsg) TermWithReason(_ string) error              { return nil }

// marshalBaseMessage wraps payload in a BaseMessage and returns the JSON bytes.
func marshalBaseMessage(t *testing.T, payload message.Payload) []byte {
	t.Helper()
	base := message.NewBaseMessage(payload.Schema(), payload, "test")
	data, err := json.Marshal(base)
	if err != nil {
		t.Fatalf("marshalBaseMessage: %v", err)
	}
	return data
}

// newLoggerDiscard returns a logger that swallows all output.
func newLoggerDiscard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newTestComponentWithRetries constructs a Component identical to newTestComponent
// but also initialises reviewRetries, which is needed for onReviewFailed tests.
// boardConfig is populated from DefaultConfig so subQuestEntityKey resolves correctly.
func newTestComponentWithRetries(qb *mockQuestBoardRef, pc *mockPartyCoordRef) *Component {
	c := newTestComponent(qb, pc)
	c.reviewRetries = make(map[string]bool)
	cfg := DefaultConfig()
	c.boardConfig = cfg.ToBoardConfig()
	c.logger = newLoggerDiscard()
	return c
}

// =============================================================================
// 1. classifyCompletedEvent
// =============================================================================

func TestGCClassifyCompletedEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		loopID     string
		result     string
		wantType   dagEventType
		wantLoopID string
		wantResult string
		wantEmpty  bool // expect zero-value dagEvent (non-lead loop)
	}{
		{
			name:       "review prefix",
			loopID:     "review-some-id-nuid12345678901234567",
			result:     `{"verdict":"accept"}`,
			wantType:   dagEventReviewCompleted,
			wantLoopID: "review-some-id-nuid12345678901234567",
			wantResult: `{"verdict":"accept"}`,
		},
		{
			name:       "clarify prefix",
			loopID:     "clarify-some-id-nuid12345678901234567",
			result:     "the answer is 42",
			wantType:   dagEventClarificationAnswered,
			wantLoopID: "clarify-some-id-nuid12345678901234567",
			wantResult: "the answer is 42",
		},
		{
			name:       "synthesis prefix",
			loopID:     "synthesis-some-id-nuid12345678901234",
			result:     "combined output",
			wantType:   dagEventSynthesisCompleted,
			wantLoopID: "synthesis-some-id-nuid12345678901234",
			wantResult: "combined output",
		},
		{
			name:      "unknown prefix returns zero value",
			loopID:    "worker-loop-abc",
			result:    "output",
			wantEmpty: true,
		},
		{
			name:      "empty loop ID returns zero value",
			loopID:    "",
			result:    "",
			wantEmpty: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			evt := classifyCompletedEvent(&agentic.LoopCompletedEvent{
				LoopID: tc.loopID,
				TaskID: "task-1",
				Result: tc.result,
			})

			if tc.wantEmpty {
				if evt.LoopID != "" || evt.Type != 0 {
					t.Errorf("expected zero-value dagEvent, got Type=%v LoopID=%q", evt.Type, evt.LoopID)
				}
				return
			}

			if evt.Type != tc.wantType {
				t.Errorf("Type = %v, want %v", evt.Type, tc.wantType)
			}
			if evt.LoopID != tc.wantLoopID {
				t.Errorf("LoopID = %q, want %q", evt.LoopID, tc.wantLoopID)
			}
			if evt.Result != tc.wantResult {
				t.Errorf("Result = %q, want %q", evt.Result, tc.wantResult)
			}
		})
	}
}

// =============================================================================
// 2. classifyFailedEvent
// =============================================================================

func TestGCClassifyFailedEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		loopID       string
		errField     string
		reasonField  string
		wantType     dagEventType
		wantReason   string
		wantEmpty    bool
	}{
		{
			name:       "review prefix — Error field preferred",
			loopID:     "review-abc-nuid12345678901234567890",
			errField:   "LLM timeout",
			reasonField: "timed out",
			wantType:   dagEventReviewFailed,
			wantReason: "LLM timeout",
		},
		{
			name:       "review prefix — falls back to Reason when Error is empty",
			loopID:     "review-abc-nuid12345678901234567890",
			errField:   "",
			reasonField: "max iterations reached",
			wantType:   dagEventReviewFailed,
			wantReason: "max iterations reached",
		},
		{
			name:       "clarify prefix",
			loopID:     "clarify-abc-nuid12345678901234567890",
			errField:   "network error",
			wantType:   dagEventClarificationFailed,
			wantReason: "network error",
		},
		{
			name:       "synthesis prefix",
			loopID:     "synthesis-abc-nuid12345678901234567",
			errField:   "model error",
			wantType:   dagEventSynthesisFailed,
			wantReason: "model error",
		},
		{
			name:      "unknown prefix returns zero value",
			loopID:    "worker-loop-abc",
			errField:  "some error",
			wantEmpty: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			evt := classifyFailedEvent(&agentic.LoopFailedEvent{
				LoopID: tc.loopID,
				TaskID: "task-1",
				Error:  tc.errField,
				Reason: tc.reasonField,
			})

			if tc.wantEmpty {
				if evt.LoopID != "" {
					t.Errorf("expected zero-value dagEvent, got LoopID=%q", evt.LoopID)
				}
				return
			}

			if evt.Type != tc.wantType {
				t.Errorf("Type = %v, want %v", evt.Type, tc.wantType)
			}
			if evt.ErrorReason != tc.wantReason {
				t.Errorf("ErrorReason = %q, want %q", evt.ErrorReason, tc.wantReason)
			}
			if evt.LoopID != tc.loopID {
				t.Errorf("LoopID = %q, want %q", evt.LoopID, tc.loopID)
			}
		})
	}
}

// =============================================================================
// 3. classifyCancelledEvent
// =============================================================================

func TestGCClassifyCancelledEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		loopID      string
		cancelledBy string
		wantType    dagEventType
		wantReason  string
		wantEmpty   bool
	}{
		{
			name:        "review prefix with CancelledBy",
			loopID:      "review-abc-nuid12345678901234567890",
			cancelledBy: "admin",
			wantType:    dagEventReviewFailed,
			wantReason:  "cancelled: admin",
		},
		{
			name:        "review prefix with empty CancelledBy",
			loopID:      "review-abc-nuid12345678901234567890",
			cancelledBy: "",
			wantType:    dagEventReviewFailed,
			wantReason:  "cancelled",
		},
		{
			name:        "clarify prefix with CancelledBy",
			loopID:      "clarify-abc-nuid12345678901234567890",
			cancelledBy: "user123",
			wantType:    dagEventClarificationFailed,
			wantReason:  "cancelled: user123",
		},
		{
			name:        "synthesis prefix",
			loopID:      "synthesis-abc-nuid12345678901234567",
			cancelledBy: "",
			wantType:    dagEventSynthesisFailed,
			wantReason:  "cancelled",
		},
		{
			name:      "unknown prefix returns zero value",
			loopID:    "agentic-loop-xyz",
			wantEmpty: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			evt := classifyCancelledEvent(&agentic.LoopCancelledEvent{
				LoopID:      tc.loopID,
				TaskID:      "task-1",
				CancelledBy: tc.cancelledBy,
			})

			if tc.wantEmpty {
				if evt.LoopID != "" {
					t.Errorf("expected zero-value dagEvent, got LoopID=%q", evt.LoopID)
				}
				return
			}

			if evt.Type != tc.wantType {
				t.Errorf("Type = %v, want %v", evt.Type, tc.wantType)
			}
			if evt.ErrorReason != tc.wantReason {
				t.Errorf("ErrorReason = %q, want %q", evt.ErrorReason, tc.wantReason)
			}
		})
	}
}

// =============================================================================
// 4. parseLeadLoopCompletion
// =============================================================================

func TestGCParseLeadLoopCompletion(t *testing.T) {
	t.Parallel()

	c := &Component{logger: newLoggerDiscard()}

	// Ensure agentic payload types are registered before the test runs.
	// The agentic package self-registers via init(), so importing it is enough —
	// but we call a harmless method to guarantee the init has fired.
	_ = (&agentic.LoopCompletedEvent{}).Schema()

	t.Run("LoopCompletedEvent with review prefix", func(t *testing.T) {
		t.Parallel()

		payload := &agentic.LoopCompletedEvent{
			LoopID: "review-local-dev-game-board1-quest-abc-nuid12345678901",
			TaskID: "task-1",
			Result: `{"verdict":"accept","sub_quest_id":"abc"}`,
		}
		msg := &mockJetMsg{data: marshalBaseMessage(t, payload)}
		evt, ok := parseLeadLoopCompletion(c, msg)
		if !ok {
			t.Fatal("parseLeadLoopCompletion returned ok=false for valid review completion")
		}
		if evt.Type != dagEventReviewCompleted {
			t.Errorf("Type = %v, want dagEventReviewCompleted", evt.Type)
		}
		if evt.LoopID != payload.LoopID {
			t.Errorf("LoopID = %q, want %q", evt.LoopID, payload.LoopID)
		}
	})

	t.Run("LoopFailedEvent with review prefix", func(t *testing.T) {
		t.Parallel()

		payload := &agentic.LoopFailedEvent{
			LoopID: "review-local-dev-game-board1-quest-def-nuid12345678901",
			TaskID: "task-2",
			Error:  "LLM context overflow",
		}
		msg := &mockJetMsg{data: marshalBaseMessage(t, payload)}
		evt, ok := parseLeadLoopCompletion(c, msg)
		if !ok {
			t.Fatal("parseLeadLoopCompletion returned ok=false for LoopFailedEvent with review prefix")
		}
		if evt.Type != dagEventReviewFailed {
			t.Errorf("Type = %v, want dagEventReviewFailed", evt.Type)
		}
		if evt.ErrorReason != "LLM context overflow" {
			t.Errorf("ErrorReason = %q, want %q", evt.ErrorReason, "LLM context overflow")
		}
	})

	t.Run("LoopCancelledEvent with clarify prefix", func(t *testing.T) {
		t.Parallel()

		payload := &agentic.LoopCancelledEvent{
			LoopID:      "clarify-local-dev-game-board1-quest-ghi-nuid123456789",
			TaskID:      "task-3",
			CancelledBy: "dm",
		}
		msg := &mockJetMsg{data: marshalBaseMessage(t, payload)}
		evt, ok := parseLeadLoopCompletion(c, msg)
		if !ok {
			t.Fatal("parseLeadLoopCompletion returned ok=false for LoopCancelledEvent with clarify prefix")
		}
		if evt.Type != dagEventClarificationFailed {
			t.Errorf("Type = %v, want dagEventClarificationFailed", evt.Type)
		}
		if evt.ErrorReason != "cancelled: dm" {
			t.Errorf("ErrorReason = %q, want %q", evt.ErrorReason, "cancelled: dm")
		}
	})

	t.Run("LoopCompletedEvent with synthesis prefix", func(t *testing.T) {
		t.Parallel()

		payload := &agentic.LoopCompletedEvent{
			LoopID: "synthesis-local-dev-game-board1-quest-parent-nuid12345",
			TaskID: "task-4",
			Result: "All sub-quest outputs combined.",
		}
		msg := &mockJetMsg{data: marshalBaseMessage(t, payload)}
		evt, ok := parseLeadLoopCompletion(c, msg)
		if !ok {
			t.Fatal("parseLeadLoopCompletion returned ok=false for synthesis completion")
		}
		if evt.Type != dagEventSynthesisCompleted {
			t.Errorf("Type = %v, want dagEventSynthesisCompleted", evt.Type)
		}
	})

	t.Run("non-lead loop ID returns false", func(t *testing.T) {
		t.Parallel()

		payload := &agentic.LoopCompletedEvent{
			LoopID: "worker-loop-abc123",
			TaskID: "task-5",
			Result: "some output",
		}
		msg := &mockJetMsg{data: marshalBaseMessage(t, payload)}
		_, ok := parseLeadLoopCompletion(c, msg)
		if ok {
			t.Error("parseLeadLoopCompletion returned ok=true for non-lead loop ID, want false")
		}
	})

	t.Run("garbage bytes return false", func(t *testing.T) {
		t.Parallel()

		msg := &mockJetMsg{data: []byte("this is not json at all }{garbage")}
		_, ok := parseLeadLoopCompletion(c, msg)
		if ok {
			t.Error("parseLeadLoopCompletion returned ok=true for unparseable data, want false")
		}
	})

	t.Run("empty data returns false", func(t *testing.T) {
		t.Parallel()

		msg := &mockJetMsg{data: []byte{}}
		_, ok := parseLeadLoopCompletion(c, msg)
		if ok {
			t.Error("parseLeadLoopCompletion returned ok=true for empty data, want false")
		}
	})
}

// =============================================================================
// 5. onReviewFailed — retry logic
// =============================================================================

func TestGCOnReviewFailed(t *testing.T) {
	t.Parallel()

	// Construct a sub-quest ID that survives the extractSubQuestFromLoopID
	// round-trip. The loop ID format is:
	//   "review-{org}-{platform}-{domain}-{board}-quest-{instance}-{nuid22chars}"
	// extractSubQuestFromLeadLoopID strips "review-", strips the last "-{nuid}",
	// then rejoins the first 6 dash-separated parts with dots.
	//
	// With org=default, platform=local we get IDs like:
	//   "default.local.game.main.quest.qnode1"
	// The sanitised form for the loop ID is:
	//   "default-local-game-main-quest-qnode1"
	const subQuestInstance = "qnode1"
	const nuid22 = "A1B2C3D4E5F6G7H8I9J0KL" // 22-char NUID lookalike
	loopID := "review-default-local-game-main-quest-" + subQuestInstance + "-" + nuid22

	// Build a DAGExecutionState that contains a sub-quest whose entity key
	// the component can look up in dagBySubQuest.
	cfg := DefaultConfig()
	boardCfg := cfg.ToBoardConfig()
	subQuestEntityKey := boardCfg.QuestEntityID(subQuestInstance)

	dag := makeFullDAGState("exec-rf1", "parent-rf1", "party-rf1", []QuestNode{
		makeNode("n1", 0),
	})
	// Point n1's NodeQuestIDs at the short instance so findNodeForQuest matches.
	dag.NodeQuestIDs["n1"] = subQuestInstance
	dag.NodeStates["n1"] = NodePendingReview

	t.Run("first failure sets reviewRetries before graph load", func(t *testing.T) {
		t.Parallel()

		qb := &mockQuestBoardRef{}
		c := newTestComponentWithRetries(qb, nil)
		c.dagCache[dag.ExecutionID] = dag
		c.dagBySubQuest[subQuestEntityKey] = dag

		evt := dagEvent{
			Type:        dagEventReviewFailed,
			LoopID:      loopID,
			ErrorReason: "LLM timeout",
		}

		// onReviewFailed sets reviewRetries[retryKey] = true and then calls
		// c.graph.GetQuest which panics on a nil receiver. Catch that panic and
		// verify the retry key was set before the graph call.
		func() {
			defer func() { recover() }() //nolint:errcheck // expected panic from nil graph
			c.onReviewFailed(context.Background(), evt)
		}()

		retryKey := "review-retry-n1-" + dag.ExecutionID
		if _, set := c.reviewRetries[retryKey]; !set {
			t.Errorf("reviewRetries[%q] not set after first failure", retryKey)
		}
		// No handleNodeFailed path should have been taken yet (would call escalate).
		if len(qb.escalateCalls) != 0 {
			t.Errorf("EscalateQuest called %d times, want 0 on first failure", len(qb.escalateCalls))
		}
	})

	t.Run("second failure invokes handleNodeFailed (escalates parent)", func(t *testing.T) {
		t.Parallel()

		qb := &mockQuestBoardRef{}
		c := newTestComponentWithRetries(qb, nil)

		// Clone dag to avoid sharing state with the parallel sub-test above.
		dag2 := makeFullDAGState("exec-rf2", "parent-rf2", "party-rf2", []QuestNode{
			makeNode("n1", 0),
		})
		dag2.NodeQuestIDs["n1"] = subQuestInstance
		dag2.NodeStates["n1"] = NodePendingReview
		dag2.NodeRetries["n1"] = 0 // no retries → handleNodeFailed escalates

		subQuestEntityKey2 := boardCfg.QuestEntityID(subQuestInstance)
		c.dagCache[dag2.ExecutionID] = dag2
		c.dagBySubQuest[subQuestEntityKey2] = dag2

		// Pre-set the retry key as if the first failure already ran.
		retryKey := "review-retry-n1-" + dag2.ExecutionID
		c.reviewRetries[retryKey] = true

		evt := dagEvent{
			Type:        dagEventReviewFailed,
			LoopID:      "review-default-local-game-main-quest-" + subQuestInstance + "-" + nuid22,
			ErrorReason: "second LLM timeout",
		}

		// onReviewFailed calls handleNodeFailed (escalation) then persistDAGState.
		// With a nil graph client persistDAGState panics; catch it and verify the
		// state mutations performed by handleNodeFailed before the panic.
		func() {
			defer func() { recover() }() //nolint:errcheck // expected panic from nil graph
			c.onReviewFailed(context.Background(), evt)
		}()

		// With retries=0, handleNodeFailed should escalate the parent.
		if len(qb.escalateCalls) != 1 {
			t.Errorf("EscalateQuest called %d times, want 1 on second failure", len(qb.escalateCalls))
		}
		if dag2.NodeStates["n1"] != NodeFailed {
			t.Errorf("node state = %q, want NodeFailed after second review failure", dag2.NodeStates["n1"])
		}
	})
}

// =============================================================================
// 6. onSynthesisFailed — mechanical rollup fallback
// =============================================================================

func TestGCOnSynthesisFailed(t *testing.T) {
	t.Parallel()

	qb := &mockQuestBoardRef{}
	c := newTestComponentWithRetries(qb, nil)

	// Build a complete DAG (all nodes NodeCompleted) so extractExecutionIDFromSynthesisLoop
	// finds it by matching the parent quest key in the loop ID.
	dag := makeFullDAGState("exec-sf1", "parent.quest.sf1", "party-sf1", []QuestNode{
		makeNode("n1", 0),
		makeNode("n2", 0),
	})
	dag.NodeStates["n1"] = NodeCompleted
	dag.NodeStates["n2"] = NodeCompleted
	dag.CompletedNodes = []string{"n1", "n2"}
	c.dagCache[dag.ExecutionID] = dag

	// Loop ID must embed the parent quest key (dots→dashes) so the extractor
	// can match it to the correct DAG in dagCache.
	evt := dagEvent{
		Type:        dagEventSynthesisFailed,
		LoopID:      "synthesis-parent-quest-sf1-nuid12345678901234",
		ErrorReason: "model error",
	}

	c.onSynthesisFailed(context.Background(), evt)

	// triggerRollup calls SubmitResult on the questboard.
	if qb.SubmitCallCount() < 1 {
		t.Error("onSynthesisFailed: expected triggerRollup to call SubmitResult, got 0 calls")
	}
}

// =============================================================================
// 7. onClarificationFailed — reset node to NodeAssigned
// =============================================================================

func TestGCOnClarificationFailed(t *testing.T) {
	t.Parallel()

	const subQuestInstance = "cqnode1"
	const nuid22 = "A1B2C3D4E5F6G7H8I9J0KL"
	loopID := "clarify-default-local-game-main-quest-" + subQuestInstance + "-" + nuid22

	cfg := DefaultConfig()
	boardCfg := cfg.ToBoardConfig()
	subQuestEntityKey := boardCfg.QuestEntityID(subQuestInstance)

	dag := makeFullDAGState("exec-cf1", "parent-cf1", "party-cf1", []QuestNode{
		makeNode("n1", 0),
	})
	dag.NodeQuestIDs["n1"] = subQuestInstance
	dag.NodeStates["n1"] = NodeAwaitingClarification

	qb := &mockQuestBoardRef{}
	c := newTestComponentWithRetries(qb, nil)
	c.dagCache[dag.ExecutionID] = dag
	c.dagBySubQuest[subQuestEntityKey] = dag

	evt := dagEvent{
		Type:        dagEventClarificationFailed,
		LoopID:      loopID,
		ErrorReason: "lead loop timed out",
	}

	// onClarificationFailed mutates node state before calling persistDAGState.
	// With a nil graph client persistDAGState panics. We catch that panic here
	// and verify the node state was mutated prior to the panic.
	func() {
		defer func() { recover() }() //nolint:errcheck // expected panic from nil graph
		c.onClarificationFailed(context.Background(), evt)
	}()

	if dag.NodeStates["n1"] != NodeAssigned {
		t.Errorf("node state = %q after clarification failure, want %q (NodeAssigned) — mutation must occur before persistDAGState",
			dag.NodeStates["n1"], NodeAssigned)
	}
}

// =============================================================================
// 8. onDAGTimedOut — cache cleanup
// =============================================================================

func TestGCOnDAGTimedOut(t *testing.T) {
	t.Parallel()

	t.Run("lookup by executionID cleans up cache", func(t *testing.T) {
		t.Parallel()

		qb := &mockQuestBoardRef{}
		c := newTestComponentWithRetries(qb, nil)

		dag := makeFullDAGState("exec-to1", "parent-to1", "", []QuestNode{
			makeNode("n1", 0),
			makeNode("n2", 0),
		})
		// Use states that cleanupDAGSubQuests handles without calling GetQuest:
		// NodePending and NodeAssigned only call qb.FailQuest, which is mocked.
		dag.NodeStates["n1"] = NodePending
		dag.NodeStates["n2"] = NodeAssigned
		c.dagCache[dag.ExecutionID] = dag
		c.dagBySubQuest["sub-quest-key-n1"] = dag
		c.dagBySubQuest["sub-quest-key-n2"] = dag
		c.dagStartTimes.Store(dag.ExecutionID, time.Now())

		// Add review retry entries that should be pruned.
		c.reviewRetries["review-retry-n1-"+dag.ExecutionID] = true
		c.reviewRetries["review-retry-n2-"+dag.ExecutionID] = true

		evt := dagEvent{
			Type:        dagEventDAGTimedOut,
			EntityKey:   dag.ExecutionID,
			ErrorReason: "30m0s",
		}

		c.onDAGTimedOut(context.Background(), evt)

		// dagCache entry removed.
		if _, stillPresent := c.dagCache[dag.ExecutionID]; stillPresent {
			t.Error("dagCache entry still present after timeout cleanup")
		}
		// dagBySubQuest entries removed.
		if _, stillPresent := c.dagBySubQuest["sub-quest-key-n1"]; stillPresent {
			t.Error("dagBySubQuest[n1] still present after timeout cleanup")
		}
		if _, stillPresent := c.dagBySubQuest["sub-quest-key-n2"]; stillPresent {
			t.Error("dagBySubQuest[n2] still present after timeout cleanup")
		}
		// dagStartTimes entry deleted.
		if _, loaded := c.dagStartTimes.Load(dag.ExecutionID); loaded {
			t.Error("dagStartTimes entry still present after timeout cleanup")
		}
		// completedDAGKeys stores the parent quest ID.
		if _, stored := c.completedDAGKeys.Load(dag.ParentQuestID); !stored {
			t.Errorf("completedDAGKeys does not contain %q after timeout", dag.ParentQuestID)
		}
		// reviewRetries pruned.
		if len(c.reviewRetries) != 0 {
			t.Errorf("reviewRetries not pruned: %v", c.reviewRetries)
		}
		// Parent quest escalated.
		if len(qb.escalateCalls) != 1 {
			t.Errorf("EscalateQuest called %d times, want 1", len(qb.escalateCalls))
		}
	})

	t.Run("lookup by ParentQuestID (API cancel path)", func(t *testing.T) {
		t.Parallel()

		qb := &mockQuestBoardRef{}
		c := newTestComponentWithRetries(qb, nil)

		dag := makeFullDAGState("exec-to2", "parent-to2-quest-id", "", []QuestNode{
			makeNode("n1", 0),
		})
		// NodeCompleted — cleanupDAGSubQuests skips terminal nodes, no graph call.
		dag.NodeStates["n1"] = NodeCompleted
		c.dagCache[dag.ExecutionID] = dag

		// The event carries the parent quest entity key, not the execution ID.
		// onDAGTimedOut must scan dagCache to find the matching DAG.
		evt := dagEvent{
			Type:        dagEventDAGTimedOut,
			EntityKey:   dag.ParentQuestID, // not the executionID
			ErrorReason: "cancelled via API",
		}

		c.onDAGTimedOut(context.Background(), evt)

		// Even though the key was the ParentQuestID, the DAG should be found and
		// the dagCache entry removed.
		if _, stillPresent := c.dagCache[dag.ExecutionID]; stillPresent {
			t.Error("dagCache entry still present after API-cancel timeout (ParentQuestID path)")
		}
	})

	t.Run("unknown key does nothing", func(t *testing.T) {
		t.Parallel()

		c := newTestComponentWithRetries(nil, nil)

		evt := dagEvent{
			Type:        dagEventDAGTimedOut,
			EntityKey:   "totally-unknown-execution-id",
			ErrorReason: "30m0s",
		}

		// Should not panic or call anything.
		c.onDAGTimedOut(context.Background(), evt)

		// dagCache still empty.
		if len(c.dagCache) != 0 {
			t.Errorf("dagCache unexpectedly non-empty: %v", c.dagCache)
		}
	})
}

// =============================================================================
// 9. pruneReviewRetries
// =============================================================================

func TestGCPruneReviewRetries(t *testing.T) {
	t.Parallel()

	c := newTestComponentWithRetries(nil, nil)

	// Populate entries for two different DAGs.
	c.reviewRetries["review-retry-n1-exec-A"] = true
	c.reviewRetries["review-retry-n2-exec-A"] = true
	c.reviewRetries["review-retry-n1-exec-B"] = true
	c.reviewRetries["review-retry-n3-exec-B"] = true

	// Prune only exec-A.
	c.pruneReviewRetries("exec-A")

	// exec-A entries must be gone.
	if _, found := c.reviewRetries["review-retry-n1-exec-A"]; found {
		t.Error("reviewRetries[review-retry-n1-exec-A] still present after pruning exec-A")
	}
	if _, found := c.reviewRetries["review-retry-n2-exec-A"]; found {
		t.Error("reviewRetries[review-retry-n2-exec-A] still present after pruning exec-A")
	}

	// exec-B entries must remain.
	if _, found := c.reviewRetries["review-retry-n1-exec-B"]; !found {
		t.Error("reviewRetries[review-retry-n1-exec-B] incorrectly removed")
	}
	if _, found := c.reviewRetries["review-retry-n3-exec-B"]; !found {
		t.Error("reviewRetries[review-retry-n3-exec-B] incorrectly removed")
	}

	// Total remaining should be 2.
	if len(c.reviewRetries) != 2 {
		t.Errorf("reviewRetries has %d entries after prune, want 2: %v", len(c.reviewRetries), c.reviewRetries)
	}
}

// =============================================================================
// 10. CancelDAGForQuest — sends dagEventDAGTimedOut to events channel
// =============================================================================

func TestGCCancelDAGForQuest(t *testing.T) {
	t.Parallel()

	c := newTestComponentWithRetries(nil, nil)
	c.events = make(chan dagEvent, 8)
	// stopChan must be initialised to avoid blocking on the select in CancelDAGForQuest.
	c.stopChan = make(chan struct{})

	parentKey := "default.local.game.main.quest.parent123"

	c.CancelDAGForQuest(context.Background(), parentKey)

	select {
	case evt := <-c.events:
		if evt.Type != dagEventDAGTimedOut {
			t.Errorf("event type = %v, want dagEventDAGTimedOut", evt.Type)
		}
		if evt.EntityKey != parentKey {
			t.Errorf("EntityKey = %q, want %q", evt.EntityKey, parentKey)
		}
		if evt.ErrorReason != "cancelled via API" {
			t.Errorf("ErrorReason = %q, want %q", evt.ErrorReason, "cancelled via API")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for cancel event on events channel")
	}
}

// =============================================================================
// 11. CancelDAGForQuest — returns promptly when ctx is cancelled and channel is full
// =============================================================================

func TestGCCancelDAGForQuestChannelFull(t *testing.T) {
	t.Parallel()

	c := newTestComponentWithRetries(nil, nil)
	// Unbuffered channel with no reader — CancelDAGForQuest cannot send immediately.
	c.events = make(chan dagEvent) // deliberately unbuffered
	c.stopChan = make(chan struct{})

	// Cancel the context so CancelDAGForQuest exits via the ctx.Done() branch.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately before calling

	done := make(chan struct{})
	go func() {
		defer close(done)
		c.CancelDAGForQuest(ctx, "some-parent-key")
	}()

	select {
	case <-done:
		// CancelDAGForQuest returned without blocking — test passes.
	case <-time.After(2 * time.Second):
		t.Error("CancelDAGForQuest blocked for >2s with cancelled context")
	}
}

// =============================================================================
// HELPERS
// =============================================================================

// =============================================================================
// classifyCancelledEvent — empty CancelledBy produces "cancelled" (not "cancelled: ")
// =============================================================================

func TestGCClassifyCancelledEventNoCancelledBy(t *testing.T) {
	t.Parallel()

	evt := classifyCancelledEvent(&agentic.LoopCancelledEvent{
		LoopID:      "review-a-b-c-d-e-f-nuid12345678901234",
		TaskID:      "task-x",
		CancelledBy: "",
	})

	if evt.ErrorReason != "cancelled" {
		t.Errorf("ErrorReason = %q, want %q (not %q)", evt.ErrorReason, "cancelled", "cancelled: ")
	}
}

// =============================================================================
// classifyFailedEvent — Reason fallback when Error is empty
// =============================================================================

func TestGCClassifyFailedEventReasonFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		loopID     string
		errField   string
		reason     string
		wantReason string
	}{
		{
			name:       "Error takes precedence",
			loopID:     "review-a-b-c-d-e-f-nuid12345678901234",
			errField:   "primary error",
			reason:     "fallback reason",
			wantReason: "primary error",
		},
		{
			name:       "falls back to Reason when Error is empty",
			loopID:     "review-a-b-c-d-e-f-nuid12345678901234",
			errField:   "",
			reason:     "only reason",
			wantReason: "only reason",
		},
		{
			name:       "both empty gives empty ErrorReason",
			loopID:     "review-a-b-c-d-e-f-nuid12345678901234",
			errField:   "",
			reason:     "",
			wantReason: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			evt := classifyFailedEvent(&agentic.LoopFailedEvent{
				LoopID: tc.loopID,
				TaskID: "task-y",
				Error:  tc.errField,
				Reason: tc.reason,
			})

			if evt.ErrorReason != tc.wantReason {
				t.Errorf("ErrorReason = %q, want %q", evt.ErrorReason, tc.wantReason)
			}
		})
	}
}

// Verify the mock satisfies the jetstream.Msg interface at compile time.
var _ jetstream.Msg = (*mockJetMsg)(nil)

// Avoid unused import of domain.
var _ = domain.QuestPosted
