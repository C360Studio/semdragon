package questdagexec

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/c360studio/semstreams/agentic"
)

const clarificationToolName = "answer_clarification"

// ClarificationExecutor implements the answer_clarification tool.
// The party lead calls this tool after receiving a clarification question from
// a party member whose sub-quest was routed to NodeAwaitingClarification.
// The executor validates that both required arguments are present and non-empty,
// then returns a JSON envelope that onClarificationAnswered parses to store the
// exchange and re-dispatch the member's sub-quest.
//
// All public methods are safe for concurrent use — the struct holds no mutable state.
type ClarificationExecutor struct{}

// NewClarificationExecutor constructs a ClarificationExecutor.
func NewClarificationExecutor() *ClarificationExecutor {
	return &ClarificationExecutor{}
}

// Execute validates the clarification arguments and returns an answer JSON envelope.
//
// Argument validation errors are surfaced as non-nil ToolResult.Error strings
// rather than Go errors. The LLM receives the error and can correct its output.
// A Go error is only returned for infrastructure failures — none arise here.
func (e *ClarificationExecutor) Execute(_ context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	subQuestID, ok := stringArg(call.Arguments, "sub_quest_id")
	if !ok || subQuestID == "" {
		return clarificationErrorResult(call, `missing required argument "sub_quest_id"`), nil
	}

	answer, ok := stringArg(call.Arguments, "answer")
	if !ok || answer == "" {
		return clarificationErrorResult(call, `missing required argument "answer"`), nil
	}

	type result struct {
		SubQuestID string `json:"sub_quest_id"`
		Answer     string `json:"answer"`
	}

	return clarificationJSONResult(call, result{
		SubQuestID: subQuestID,
		Answer:     answer,
	})
}

// ListTools returns the single tool definition for answer_clarification.
func (e *ClarificationExecutor) ListTools() []agentic.ToolDefinition {
	return []agentic.ToolDefinition{{
		Name:        clarificationToolName,
		Description: "Answer a party member's clarification question about their sub-quest. Provide a clear answer that resolves their question so they can continue working.",
		Parameters: map[string]any{
			"type":     "object",
			"required": []string{"sub_quest_id", "answer"},
			"properties": map[string]any{
				"sub_quest_id": map[string]any{
					"type":        "string",
					"description": "The ID of the sub-quest the member is asking about",
				},
				"answer": map[string]any{
					"type":        "string",
					"description": "Your answer to the member's question. Be specific and actionable.",
				},
			},
		},
	}}
}

// -- helpers --

// clarificationJSONResult marshals v to JSON and returns a successful ToolResult.
func clarificationJSONResult(call agentic.ToolCall, v any) (agentic.ToolResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return clarificationErrorResult(call, fmt.Sprintf("failed to marshal result: %s", err)), nil
	}
	return agentic.ToolResult{
		CallID:  call.ID,
		Content: string(data),
		LoopID:  call.LoopID,
		TraceID: call.TraceID,
	}, nil
}

// clarificationErrorResult returns a ToolResult carrying an error message.
// ToolResult.Error is forwarded to the LLM so it can correct its arguments.
func clarificationErrorResult(call agentic.ToolCall, msg string) agentic.ToolResult {
	return agentic.ToolResult{
		CallID:  call.ID,
		Error:   msg,
		LoopID:  call.LoopID,
		TraceID: call.TraceID,
	}
}
