package semdragons

import (
	"context"
	"sync"

	"github.com/c360studio/semstreams/natsclient"
)

// =============================================================================
// TRAJECTORY MANAGER - Trace context management for quest observability
// =============================================================================
// TraceManager provides W3C-compatible trace context for quest lifecycles.
// Each quest gets a unique trace ID, and lifecycle events become spans.
//
// Span Hierarchy:
//   quest (root span)
//   ├── claimed
//   ├── started
//   ├── battle (if triggered)
//   │   └── verdict
//   └── completed/failed
// =============================================================================

// TraceManager manages trace contexts for quests and their events.
// It maintains a mapping from quest IDs to their root trace contexts,
// enabling child spans for lifecycle events.
type TraceManager struct {
	mu     sync.RWMutex
	traces map[QuestID]*natsclient.TraceContext
}

// NewTraceManager creates a new trace manager.
func NewTraceManager() *TraceManager {
	return &TraceManager{
		traces: make(map[QuestID]*natsclient.TraceContext),
	}
}

// StartQuestTrace creates a new trace context for a quest.
// This should be called when a quest is posted to the board.
// Returns the trace context with the quest's root span.
func (tm *TraceManager) StartQuestTrace(questID QuestID) *natsclient.TraceContext {
	tc := natsclient.NewTraceContext()

	tm.mu.Lock()
	tm.traces[questID] = tc
	tm.mu.Unlock()

	return tc
}

// StartQuestTraceWithParent creates a trace context that inherits from a parent.
// This is used for sub-quests that should be linked to their parent quest's trace.
func (tm *TraceManager) StartQuestTraceWithParent(questID QuestID, parentQuestID QuestID) *natsclient.TraceContext {
	tm.mu.RLock()
	parentTC := tm.traces[parentQuestID]
	tm.mu.RUnlock()

	var tc *natsclient.TraceContext
	if parentTC != nil {
		// Child span of parent quest's trace
		tc = parentTC.NewSpan()
	} else {
		// No parent trace found, start fresh
		tc = natsclient.NewTraceContext()
	}

	tm.mu.Lock()
	tm.traces[questID] = tc
	tm.mu.Unlock()

	return tc
}

// GetQuestTrace returns the trace context for a quest.
// Returns nil if no trace exists for the quest.
func (tm *TraceManager) GetQuestTrace(questID QuestID) *natsclient.TraceContext {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.traces[questID]
}

// NewEventSpan creates a child span for a quest lifecycle event.
// The returned context contains the new span's trace context.
// Returns the original context if no trace exists for the quest.
func (tm *TraceManager) NewEventSpan(ctx context.Context, questID QuestID) (context.Context, *natsclient.TraceContext) {
	tm.mu.RLock()
	parentTC := tm.traces[questID]
	tm.mu.RUnlock()

	if parentTC == nil {
		return ctx, nil
	}

	childTC := parentTC.NewSpan()
	return natsclient.ContextWithTrace(ctx, childTC), childTC
}

// EndQuestTrace removes the trace context for a completed/failed quest.
// This should be called when a quest reaches a terminal state.
func (tm *TraceManager) EndQuestTrace(questID QuestID) {
	tm.mu.Lock()
	delete(tm.traces, questID)
	tm.mu.Unlock()
}

// ActiveTraces returns the number of currently tracked traces.
// Useful for debugging and monitoring.
func (tm *TraceManager) ActiveTraces() int {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return len(tm.traces)
}

// =============================================================================
// TRACE INFO - Portable trace identifiers for payloads
// =============================================================================

// TraceInfo contains trace identifiers that can be embedded in payloads.
// This enables event consumers to correlate events with quest traces.
type TraceInfo struct {
	TraceID      string `json:"trace_id,omitempty"`
	SpanID       string `json:"span_id,omitempty"`
	ParentSpanID string `json:"parent_span_id,omitempty"`
}

// TraceInfoFromContext extracts trace info from a context.
func TraceInfoFromContext(ctx context.Context) TraceInfo {
	tc, ok := natsclient.TraceContextFromContext(ctx)
	if !ok || tc == nil {
		return TraceInfo{}
	}
	return TraceInfo{
		TraceID:      tc.TraceID,
		SpanID:       tc.SpanID,
		ParentSpanID: tc.ParentSpanID,
	}
}

// TraceInfoFromTraceContext converts a TraceContext to TraceInfo.
func TraceInfoFromTraceContext(tc *natsclient.TraceContext) TraceInfo {
	if tc == nil {
		return TraceInfo{}
	}
	return TraceInfo{
		TraceID:      tc.TraceID,
		SpanID:       tc.SpanID,
		ParentSpanID: tc.ParentSpanID,
	}
}

// IsEmpty returns true if no trace information is present.
func (ti TraceInfo) IsEmpty() bool {
	return ti.TraceID == ""
}

// =============================================================================
// CONTEXT HELPERS
// =============================================================================

// ContextWithQuestTrace returns a context with the quest's trace context.
// If no trace exists for the quest, the original context is returned.
func (tm *TraceManager) ContextWithQuestTrace(ctx context.Context, questID QuestID) context.Context {
	tc := tm.GetQuestTrace(questID)
	if tc == nil {
		return ctx
	}
	return natsclient.ContextWithTrace(ctx, tc)
}
