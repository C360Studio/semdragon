package executor

import (
	"errors"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/internal/semcompat"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/types"
)

// =============================================================================
// EXECUTOR PAYLOADS
// =============================================================================
// Live raw typed-subject event payloads for quest execution operations. Their
// Graphable methods declare prospective graph shapes; no graph-ingest route is
// registered here.
// =============================================================================

// Ensure Graphable implementations.
var (
	_ graph.Graphable = (*ExecutionStartedPayload)(nil)
	_ graph.Graphable = (*ExecutionCompletedPayload)(nil)
	_ graph.Graphable = (*ExecutionFailedPayload)(nil)
)

// Typed subjects for executor lifecycle events.
var (
	SubjectExecutionStarted   = natsclient.NewSubject[ExecutionStartedPayload](domain.PredicateExecutionStarted)
	SubjectExecutionCompleted = natsclient.NewSubject[ExecutionCompletedPayload](domain.PredicateExecutionCompleted)
	SubjectExecutionFailed    = natsclient.NewSubject[ExecutionFailedPayload](domain.PredicateExecutionFailed)
)

// --- TraceInfo for observability ---

// TraceInfo contains trace context for observability.
type TraceInfo struct {
	TrajectoryID string `json:"trajectory_id,omitempty"`
	SpanID       string `json:"span_id,omitempty"`
	ParentSpanID string `json:"parent_span_id,omitempty"`
}

// =============================================================================
// EXECUTION STARTED PAYLOAD
// =============================================================================

// ExecutionStartedPayload contains data for execution.started events.
type ExecutionStartedPayload struct {
	QuestID    domain.QuestID `json:"quest_id"`
	QuestTitle string         `json:"quest_title"`
	AgentID    domain.AgentID `json:"agent_id"`
	AgentName  string         `json:"agent_name"`
	LoopID     string         `json:"loop_id"`
	MaxTurns   int            `json:"max_turns"`
	MaxTokens  int            `json:"max_tokens"`
	ToolCount  int            `json:"tool_count"` // Number of tools available
	Timestamp  time.Time      `json:"timestamp"`
	Trace      TraceInfo      `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *ExecutionStartedPayload) EntityID() string { return string(p.QuestID) }

// Triples returns semantic triples for this event.
func (p *ExecutionStartedPayload) Triples() []message.Triple {
	source := "executor"
	entityID := string(p.QuestID)

	return []message.Triple{
		{Subject: entityID, Predicate: domain.PredicateExecutionStarted, Object: true, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: domain.PredicateExecutionAgentID, Object: string(p.AgentID), Source: source, Timestamp: p.Timestamp, Confidence: 1.0, Datatype: semcompat.EntityReferenceDatatype},
		{Subject: entityID, Predicate: domain.PredicateExecutionLoopID, Object: p.LoopID, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: domain.PredicateExecutionConfigMaxTurns, Object: p.MaxTurns, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: domain.PredicateExecutionConfigMaxTokens, Object: p.MaxTokens, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: domain.PredicateExecutionToolsAvailable, Object: p.ToolCount, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

// Schema returns the type schema for this payload.
func (p *ExecutionStartedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "execution.started", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *ExecutionStartedPayload) Validate() error {
	if p.QuestID == "" {
		return errors.New("quest_id required")
	}
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// =============================================================================
// EXECUTION COMPLETED PAYLOAD
// =============================================================================

// ExecutionCompletedPayload contains data for execution.completed events.
type ExecutionCompletedPayload struct {
	QuestID          domain.QuestID  `json:"quest_id"`
	AgentID          domain.AgentID  `json:"agent_id"`
	LoopID           string          `json:"loop_id"`
	Status           ExecutionStatus `json:"status"`
	TotalTurns       int             `json:"total_turns"`
	TotalToolCalls   int             `json:"total_tool_calls"`
	PromptTokens     int             `json:"prompt_tokens"`
	CompletionTokens int             `json:"completion_tokens"`
	Duration         time.Duration   `json:"duration"`
	Timestamp        time.Time       `json:"timestamp"`
	Trace            TraceInfo       `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *ExecutionCompletedPayload) EntityID() string { return string(p.QuestID) }

// Triples returns semantic triples for this event.
func (p *ExecutionCompletedPayload) Triples() []message.Triple {
	source := "executor"
	entityID := string(p.QuestID)

	return []message.Triple{
		{Subject: entityID, Predicate: domain.PredicateExecutionCompleted, Object: true, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: domain.PredicateExecutionStatusFinal, Object: string(p.Status), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: domain.PredicateExecutionMetricsTurns, Object: p.TotalTurns, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: domain.PredicateExecutionMetricsToolCalls, Object: p.TotalToolCalls, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: domain.PredicateExecutionTokensPrompt, Object: p.PromptTokens, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: domain.PredicateExecutionTokensCompletion, Object: p.CompletionTokens, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: domain.PredicateExecutionDurationMS, Object: p.Duration.Milliseconds(), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

// Schema returns the type schema for this payload.
func (p *ExecutionCompletedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "execution.completed", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *ExecutionCompletedPayload) Validate() error {
	if p.QuestID == "" {
		return errors.New("quest_id required")
	}
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// =============================================================================
// EXECUTION FAILED PAYLOAD
// =============================================================================

// ExecutionFailedPayload contains data for execution.failed events.
type ExecutionFailedPayload struct {
	QuestID        domain.QuestID  `json:"quest_id"`
	AgentID        domain.AgentID  `json:"agent_id"`
	LoopID         string          `json:"loop_id"`
	Status         ExecutionStatus `json:"status"`
	Error          string          `json:"error"`
	TurnsCompleted int             `json:"turns_completed"`
	ToolCallsMade  int             `json:"tool_calls_made"`
	Duration       time.Duration   `json:"duration"`
	Timestamp      time.Time       `json:"timestamp"`
	Trace          TraceInfo       `json:"trace,omitempty"`
}

// EntityID returns the entity ID for this event.
func (p *ExecutionFailedPayload) EntityID() string { return string(p.QuestID) }

// Triples returns semantic triples for this event.
func (p *ExecutionFailedPayload) Triples() []message.Triple {
	source := "executor"
	entityID := string(p.QuestID)

	return []message.Triple{
		{Subject: entityID, Predicate: domain.PredicateExecutionFailed, Object: true, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: domain.PredicateExecutionStatusFinal, Object: string(p.Status), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: domain.PredicateExecutionFailureError, Object: p.Error, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: domain.PredicateExecutionMetricsTurns, Object: p.TurnsCompleted, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: domain.PredicateExecutionMetricsToolCalls, Object: p.ToolCallsMade, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: domain.PredicateExecutionDurationMS, Object: p.Duration.Milliseconds(), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

// Schema returns the type schema for this payload.
func (p *ExecutionFailedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "execution.failed", Version: "v1"}
}

// Validate checks the payload for required fields.
func (p *ExecutionFailedPayload) Validate() error {
	if p.QuestID == "" {
		return errors.New("quest_id required")
	}
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}
