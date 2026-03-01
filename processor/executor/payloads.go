package executor

import (
	"errors"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/types"
)

// =============================================================================
// EXECUTOR PAYLOADS
// =============================================================================
// Event payloads for quest execution operations. Each implements graph.Graphable
// for automatic persistence via graph-ingest.
// =============================================================================

// Ensure Graphable implementations.
var (
	_ graph.Graphable = (*ExecutionStartedPayload)(nil)
	_ graph.Graphable = (*ExecutionCompletedPayload)(nil)
	_ graph.Graphable = (*ExecutionFailedPayload)(nil)
	_ graph.Graphable = (*ToolCallPayload)(nil)
	_ graph.Graphable = (*ToolResultPayload)(nil)
)

// --- Typed Subjects ---

var (
	SubjectExecutionStarted   = natsclient.NewSubject[ExecutionStartedPayload](domain.PredicateExecutionStarted)
	SubjectExecutionCompleted = natsclient.NewSubject[ExecutionCompletedPayload](domain.PredicateExecutionCompleted)
	SubjectExecutionFailed    = natsclient.NewSubject[ExecutionFailedPayload](domain.PredicateExecutionFailed)
	SubjectToolCall           = natsclient.NewSubject[ToolCallPayload](domain.PredicateToolCall)
	SubjectToolResult         = natsclient.NewSubject[ToolResultPayload](domain.PredicateToolResult)
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

func (p *ExecutionStartedPayload) EntityID() string { return string(p.QuestID) }

func (p *ExecutionStartedPayload) Triples() []message.Triple {
	source := "executor"
	entityID := string(p.QuestID)

	return []message.Triple{
		{Subject: entityID, Predicate: "execution.lifecycle.started", Object: true, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "execution.agent.id", Object: string(p.AgentID), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "execution.loop.id", Object: p.LoopID, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "execution.config.max_turns", Object: p.MaxTurns, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "execution.config.max_tokens", Object: p.MaxTokens, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "execution.tools.available", Object: p.ToolCount, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

func (p *ExecutionStartedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "execution.started", Version: "v1"}
}

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

func (p *ExecutionCompletedPayload) EntityID() string { return string(p.QuestID) }

func (p *ExecutionCompletedPayload) Triples() []message.Triple {
	source := "executor"
	entityID := string(p.QuestID)

	return []message.Triple{
		{Subject: entityID, Predicate: "execution.lifecycle.completed", Object: true, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "execution.status.final", Object: string(p.Status), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "execution.metrics.turns", Object: p.TotalTurns, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "execution.metrics.tool_calls", Object: p.TotalToolCalls, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "execution.tokens.prompt", Object: p.PromptTokens, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "execution.tokens.completion", Object: p.CompletionTokens, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "execution.duration.ms", Object: p.Duration.Milliseconds(), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

func (p *ExecutionCompletedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "execution.completed", Version: "v1"}
}

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

func (p *ExecutionFailedPayload) EntityID() string { return string(p.QuestID) }

func (p *ExecutionFailedPayload) Triples() []message.Triple {
	source := "executor"
	entityID := string(p.QuestID)

	return []message.Triple{
		{Subject: entityID, Predicate: "execution.lifecycle.failed", Object: true, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "execution.status.final", Object: string(p.Status), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "execution.failure.error", Object: p.Error, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "execution.metrics.turns", Object: p.TurnsCompleted, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "execution.metrics.tool_calls", Object: p.ToolCallsMade, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "execution.duration.ms", Object: p.Duration.Milliseconds(), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

func (p *ExecutionFailedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "execution.failed", Version: "v1"}
}

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

// =============================================================================
// TOOL CALL PAYLOAD
// =============================================================================

// ToolCallPayload contains data for execution.tool.call events.
type ToolCallPayload struct {
	QuestID   domain.QuestID `json:"quest_id"`
	AgentID   domain.AgentID `json:"agent_id"`
	LoopID    string         `json:"loop_id"`
	Turn      int            `json:"turn"`
	CallID    string         `json:"call_id"`
	ToolName  string         `json:"tool_name"`
	Arguments map[string]any `json:"arguments,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Trace     TraceInfo      `json:"trace,omitempty"`
}

func (p *ToolCallPayload) EntityID() string { return string(p.QuestID) }

func (p *ToolCallPayload) Triples() []message.Triple {
	source := "executor"
	entityID := string(p.QuestID)

	return []message.Triple{
		{Subject: entityID, Predicate: "execution.tool.call", Object: p.ToolName, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "execution.tool.call_id", Object: p.CallID, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "execution.tool.turn", Object: p.Turn, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

func (p *ToolCallPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "execution.tool.call", Version: "v1"}
}

func (p *ToolCallPayload) Validate() error {
	if p.QuestID == "" {
		return errors.New("quest_id required")
	}
	if p.ToolName == "" {
		return errors.New("tool_name required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// =============================================================================
// TOOL RESULT PAYLOAD
// =============================================================================

// ToolResultPayload contains data for execution.tool.result events.
type ToolResultPayload struct {
	QuestID   domain.QuestID `json:"quest_id"`
	AgentID   domain.AgentID `json:"agent_id"`
	LoopID    string         `json:"loop_id"`
	CallID    string         `json:"call_id"`
	ToolName  string         `json:"tool_name"`
	Success   bool           `json:"success"`
	Error     string         `json:"error,omitempty"`
	Duration  time.Duration  `json:"duration"`
	Timestamp time.Time      `json:"timestamp"`
	Trace     TraceInfo      `json:"trace,omitempty"`
}

func (p *ToolResultPayload) EntityID() string { return string(p.QuestID) }

func (p *ToolResultPayload) Triples() []message.Triple {
	source := "executor"
	entityID := string(p.QuestID)

	triples := []message.Triple{
		{Subject: entityID, Predicate: "execution.tool.result", Object: p.ToolName, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "execution.tool.success", Object: p.Success, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "execution.tool.duration_ms", Object: p.Duration.Milliseconds(), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}

	if p.Error != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "execution.tool.error", Object: p.Error,
			Source: source, Timestamp: p.Timestamp, Confidence: 1.0,
		})
	}

	return triples
}

func (p *ToolResultPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "execution.tool.result", Version: "v1"}
}

func (p *ToolResultPayload) Validate() error {
	if p.QuestID == "" {
		return errors.New("quest_id required")
	}
	if p.CallID == "" {
		return errors.New("call_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}
