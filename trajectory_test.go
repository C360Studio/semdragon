package semdragons

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

func TestTraceManager_StartQuestTrace(t *testing.T) {
	tm := NewTraceManager()

	questID := QuestID("test.quest.1")
	tc := tm.StartQuestTrace(questID)

	if tc == nil {
		t.Fatal("StartQuestTrace should return trace context")
	}
	if tc.TraceID == "" {
		t.Error("TraceID should not be empty")
	}
	if tc.SpanID == "" {
		t.Error("SpanID should not be empty")
	}
	if tc.ParentSpanID != "" {
		t.Error("Root span should not have parent")
	}
	if !tc.Sampled {
		t.Error("Should be sampled by default")
	}

	// Verify we can retrieve it
	retrieved := tm.GetQuestTrace(questID)
	if retrieved != tc {
		t.Error("GetQuestTrace should return the same trace context")
	}

	// Verify active count
	if tm.ActiveTraces() != 1 {
		t.Errorf("expected 1 active trace, got %d", tm.ActiveTraces())
	}
}

func TestTraceManager_StartQuestTraceWithParent(t *testing.T) {
	tm := NewTraceManager()

	parentID := QuestID("test.quest.parent")
	childID := QuestID("test.quest.child")

	// Start parent trace
	parentTC := tm.StartQuestTrace(parentID)

	// Start child trace with parent
	childTC := tm.StartQuestTraceWithParent(childID, parentID)

	if childTC == nil {
		t.Fatal("child trace context should not be nil")
	}

	// Child should share same trace ID
	if childTC.TraceID != parentTC.TraceID {
		t.Errorf("child TraceID (%s) should match parent (%s)",
			childTC.TraceID, parentTC.TraceID)
	}

	// Child should have parent's span as its parent
	if childTC.ParentSpanID != parentTC.SpanID {
		t.Errorf("child ParentSpanID (%s) should be parent's SpanID (%s)",
			childTC.ParentSpanID, parentTC.SpanID)
	}

	// Child should have different span ID
	if childTC.SpanID == parentTC.SpanID {
		t.Error("child SpanID should differ from parent SpanID")
	}

	// Both should be tracked
	if tm.ActiveTraces() != 2 {
		t.Errorf("expected 2 active traces, got %d", tm.ActiveTraces())
	}
}

func TestTraceManager_StartQuestTraceWithParent_NoParent(t *testing.T) {
	tm := NewTraceManager()

	// Start child trace with non-existent parent
	childID := QuestID("test.quest.orphan")
	nonExistentParent := QuestID("test.quest.missing")

	childTC := tm.StartQuestTraceWithParent(childID, nonExistentParent)

	if childTC == nil {
		t.Fatal("child trace context should not be nil")
	}

	// Should create a new trace (no parent to inherit from)
	if childTC.ParentSpanID != "" {
		t.Error("orphan quest should not have parent span")
	}
}

func TestTraceManager_NewEventSpan(t *testing.T) {
	tm := NewTraceManager()
	ctx := context.Background()

	questID := QuestID("test.quest.1")
	rootTC := tm.StartQuestTrace(questID)

	// Create event span
	newCtx, eventTC := tm.NewEventSpan(ctx, questID)

	if eventTC == nil {
		t.Fatal("event span trace context should not be nil")
	}

	// Event should share same trace ID
	if eventTC.TraceID != rootTC.TraceID {
		t.Error("event TraceID should match quest TraceID")
	}

	// Event should be child of quest's span
	if eventTC.ParentSpanID != rootTC.SpanID {
		t.Errorf("event ParentSpanID (%s) should match quest SpanID (%s)",
			eventTC.ParentSpanID, rootTC.SpanID)
	}

	// Context should contain the trace
	if newCtx == ctx {
		t.Error("new context should differ from original")
	}
}

func TestTraceManager_NewEventSpan_NoTrace(t *testing.T) {
	tm := NewTraceManager()
	ctx := context.Background()

	// No trace registered
	questID := QuestID("test.quest.missing")

	newCtx, eventTC := tm.NewEventSpan(ctx, questID)

	// Should return nil trace and original context
	if eventTC != nil {
		t.Error("event trace context should be nil for missing quest")
	}
	if newCtx != ctx {
		t.Error("context should be unchanged when no trace exists")
	}
}

func TestTraceManager_EndQuestTrace(t *testing.T) {
	tm := NewTraceManager()

	questID := QuestID("test.quest.1")
	tm.StartQuestTrace(questID)

	if tm.ActiveTraces() != 1 {
		t.Fatal("expected 1 active trace before end")
	}

	// End the trace
	tm.EndQuestTrace(questID)

	if tm.ActiveTraces() != 0 {
		t.Errorf("expected 0 active traces after end, got %d", tm.ActiveTraces())
	}

	// Getting ended trace should return nil
	if tm.GetQuestTrace(questID) != nil {
		t.Error("ended trace should return nil")
	}
}

func TestTraceManager_ContextWithQuestTrace(t *testing.T) {
	tm := NewTraceManager()
	ctx := context.Background()

	questID := QuestID("test.quest.1")
	tc := tm.StartQuestTrace(questID)

	// Get context with trace
	traceCtx := tm.ContextWithQuestTrace(ctx, questID)

	if traceCtx == ctx {
		t.Error("trace context should differ from original")
	}

	// Verify trace info can be extracted
	info := TraceInfoFromContext(traceCtx)
	if info.TraceID != tc.TraceID {
		t.Errorf("extracted TraceID (%s) should match original (%s)",
			info.TraceID, tc.TraceID)
	}
}

func TestTraceManager_ContextWithQuestTrace_NoTrace(t *testing.T) {
	tm := NewTraceManager()
	ctx := context.Background()

	// No trace registered
	questID := QuestID("test.quest.missing")

	traceCtx := tm.ContextWithQuestTrace(ctx, questID)

	// Should return original context
	if traceCtx != ctx {
		t.Error("context should be unchanged when no trace exists")
	}
}

func TestTraceInfo(t *testing.T) {
	tm := NewTraceManager()

	questID := QuestID("test.quest.1")
	tc := tm.StartQuestTrace(questID)

	info := TraceInfoFromTraceContext(tc)

	if info.TraceID != tc.TraceID {
		t.Error("TraceID mismatch")
	}
	if info.SpanID != tc.SpanID {
		t.Error("SpanID mismatch")
	}
	if info.ParentSpanID != tc.ParentSpanID {
		t.Error("ParentSpanID mismatch")
	}
	if info.IsEmpty() {
		t.Error("info should not be empty")
	}

	// Nil trace context
	nilInfo := TraceInfoFromTraceContext(nil)
	if !nilInfo.IsEmpty() {
		t.Error("nil trace context should produce empty TraceInfo")
	}
}

func TestTraceManager_ConcurrentAccess(t *testing.T) {
	tm := NewTraceManager()
	var wg sync.WaitGroup

	// Concurrent writes and reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			questID := QuestID(fmt.Sprintf("quest.%d", n))

			// Start trace
			tm.StartQuestTrace(questID)

			// Create event span
			ctx := context.Background()
			tm.NewEventSpan(ctx, questID)

			// Get trace
			tm.GetQuestTrace(questID)

			// Check active count
			tm.ActiveTraces()

			// End trace
			tm.EndQuestTrace(questID)
		}(i)
	}

	wg.Wait()

	if tm.ActiveTraces() != 0 {
		t.Errorf("expected 0 active traces after all complete, got %d", tm.ActiveTraces())
	}
}
