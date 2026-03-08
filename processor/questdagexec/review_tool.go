package questdagexec

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/c360studio/semstreams/agentic"
)

const reviewToolName = "review_sub_quest"

// reviewAcceptVerdict and reviewRejectVerdict are the only valid verdict strings.
// The tool definition uses an enum so the LLM should always produce one of these,
// but we validate explicitly to catch any LLM hallucination.
const (
	reviewAcceptVerdict = "accept"
	reviewRejectVerdict = "reject"
)

// reviewAvgThreshold is the average rating below which an explanation is required.
// Matches the ADR specification: avg < 3.0 → explanation required.
const reviewAvgThreshold = 3.0

// ReviewExecutor implements the review_sub_quest tool.
// The party lead calls this tool after reviewing a party member's sub-quest output.
// It validates the three-question rating (Q1-Q3, scale 1-5), enforces the
// explanation requirement when ratings are low, and returns a verdict envelope
// that the questdagexec processor reads to advance or retry the DAG node.
//
// All public methods are safe for concurrent use — the struct holds no mutable state.
type ReviewExecutor struct{}

// NewReviewExecutor constructs a ReviewExecutor.
func NewReviewExecutor() *ReviewExecutor {
	return &ReviewExecutor{}
}

// Execute validates the review arguments and returns a verdict JSON envelope.
//
// Argument validation errors are surfaced as non-nil ToolResult.Error strings
// rather than Go errors. The LLM receives the error and can correct its output.
// A Go error is only returned for infrastructure failures — none arise here.
func (e *ReviewExecutor) Execute(_ context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	subQuestID, ok := stringArg(call.Arguments, "sub_quest_id")
	if !ok || subQuestID == "" {
		return reviewErrorResult(call, `missing required argument "sub_quest_id"`), nil
	}

	ratingsRaw, ok := call.Arguments["ratings"]
	if !ok || ratingsRaw == nil {
		return reviewErrorResult(call, `missing required argument "ratings"`), nil
	}

	ratingsMap, ok := ratingsRaw.(map[string]any)
	if !ok {
		return reviewErrorResult(call, fmt.Sprintf("ratings must be an object, got %T", ratingsRaw)), nil
	}

	q1, err := intRating(ratingsMap, "q1")
	if err != nil {
		return reviewErrorResult(call, fmt.Sprintf("ratings.q1: %s", err)), nil
	}
	q2, err := intRating(ratingsMap, "q2")
	if err != nil {
		return reviewErrorResult(call, fmt.Sprintf("ratings.q2: %s", err)), nil
	}
	q3, err := intRating(ratingsMap, "q3")
	if err != nil {
		return reviewErrorResult(call, fmt.Sprintf("ratings.q3: %s", err)), nil
	}

	avgRating := (float64(q1) + float64(q2) + float64(q3)) / 3.0

	verdict, ok := stringArg(call.Arguments, "verdict")
	if !ok || verdict == "" {
		return reviewErrorResult(call, `missing required argument "verdict"`), nil
	}
	if verdict != reviewAcceptVerdict && verdict != reviewRejectVerdict {
		return reviewErrorResult(call, fmt.Sprintf(`verdict must be "accept" or "reject", got %q`, verdict)), nil
	}

	explanation, _ := stringArg(call.Arguments, "explanation")

	// Explanation is required when the average rating is below threshold and
	// the verdict is reject. Accepting with low ratings (unusual but allowed) does
	// not require explanation — the lead is overriding the threshold deliberately.
	if verdict == reviewRejectVerdict && avgRating < reviewAvgThreshold && explanation == "" {
		return reviewErrorResult(call, fmt.Sprintf(
			"explanation is required when avg rating (%.2f) is below %.1f",
			avgRating, reviewAvgThreshold,
		)), nil
	}

	// Anti-inflation guard: perfect scores (all 5s) require explicit justification.
	// LLMs are sycophantic by default and give inflated ratings. This structural
	// guardrail forces the lead to explain WHY the work is truly exceptional rather
	// than rubber-stamping every submission. The explanation is stored on the peer
	// review entity and becomes part of the agent's permanent record.
	//
	// Intentionally conservative: only the exact {5,5,5} pattern is rejected.
	// Near-perfect scores like {5,5,4} (avg 4.67) are allowed without explanation.
	// If telemetry shows score clustering at 4.67+, consider expanding the threshold.
	if q1 == 5 && q2 == 5 && q3 == 5 && explanation == "" {
		return reviewErrorResult(call, "all-5 ratings require an explanation justifying why "+
			"the work is truly exceptional across all three criteria. A score of 5 means the "+
			"work significantly exceeded expectations — most competent work deserves a 3 or 4. "+
			"Provide an explanation or adjust your ratings to reflect the actual quality."), nil
	}

	type result struct {
		Verdict     string  `json:"verdict"`
		AvgRating   float64 `json:"avg_rating"`
		SubQuestID  string  `json:"sub_quest_id"`
		Explanation string  `json:"explanation,omitempty"`
	}

	return reviewJSONResult(call, result{
		Verdict:     verdict,
		AvgRating:   avgRating,
		SubQuestID:  subQuestID,
		Explanation: explanation,
	})
}

// ListTools returns the single tool definition for review_sub_quest.
func (e *ReviewExecutor) ListTools() []agentic.ToolDefinition {
	return []agentic.ToolDefinition{{
		Name:        reviewToolName,
		Description: "Review a party member's sub-quest output. Rate honestly: 3 = meets expectations (standard competent work), 5 = exceptional (rare). Inflated scores become part of the agent's permanent record and mislead future leads. If avg < 3.0, explanation is required.",
		Parameters: map[string]any{
			"type":     "object",
			"required": []string{"sub_quest_id", "ratings", "verdict"},
			"properties": map[string]any{
				"sub_quest_id": map[string]any{
					"type":        "string",
					"description": "The ID of the sub-quest being reviewed",
				},
				"ratings": map[string]any{
					"type":        "object",
					"description": "Three-question rating from the lead (each 1-5)",
					"required":    []string{"q1", "q2", "q3"},
					"properties": map[string]any{
						"q1": map[string]any{
							"type":        "integer",
							"description": "Task quality: Did the output meet acceptance criteria? 1=wrong/missing, 2=significant gaps, 3=meets requirements, 4=exceeds (thorough), 5=exceptional (rare)",
							"minimum":     1,
							"maximum":     5,
						},
						"q2": map[string]any{
							"type":        "integer",
							"description": "Communication: Were assumptions stated and output clearly organized? 1=incoherent, 2=unclear, 3=adequate, 4=well-structured, 5=exemplary",
							"minimum":     1,
							"maximum":     5,
						},
						"q3": map[string]any{
							"type":        "integer",
							"description": "Completeness: Did the agent deliver everything needed without gaps? 1=mostly missing, 2=incomplete, 3=complete, 4=thorough with edge cases, 5=comprehensive beyond requirements",
							"minimum":     1,
							"maximum":     5,
						},
					},
				},
				"explanation": map[string]any{
					"type":        "string",
					"description": "Corrective feedback for the member. Required when avg rating < 3.0 and verdict is reject. Also required when all ratings are 5 — justify why the work is truly exceptional.",
				},
				"verdict": map[string]any{
					"type":        "string",
					"description": "accept or reject. Accept advances the DAG node; reject resets it for retry with your feedback.",
					"enum":        []string{reviewAcceptVerdict, reviewRejectVerdict},
				},
			},
		},
	}}
}

// -- helpers --

// intRating extracts and validates an integer rating (1-5) from the ratings map.
// JSON numbers unmarshal as float64 in map[string]any, so we accept float64 and
// convert to int after bounds checking.
func intRating(m map[string]any, key string) (int, error) {
	v, ok := m[key]
	if !ok {
		return 0, fmt.Errorf("missing required field %q", key)
	}
	f, ok := v.(float64)
	if !ok {
		return 0, fmt.Errorf("must be a number, got %T", v)
	}
	n := int(f)
	if n < 1 || n > 5 {
		return 0, fmt.Errorf("must be between 1 and 5, got %d", n)
	}
	return n, nil
}

// reviewJSONResult marshals v to JSON and returns a successful ToolResult.
func reviewJSONResult(call agentic.ToolCall, v any) (agentic.ToolResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return reviewErrorResult(call, fmt.Sprintf("failed to marshal result: %s", err)), nil
	}
	return agentic.ToolResult{
		CallID:  call.ID,
		Content: string(data),
		LoopID:  call.LoopID,
		TraceID: call.TraceID,
	}, nil
}

// reviewErrorResult returns a ToolResult carrying an error message.
// ToolResult.Error is forwarded to the LLM so it can correct its arguments.
func reviewErrorResult(call agentic.ToolCall, msg string) agentic.ToolResult {
	return agentic.ToolResult{
		CallID:  call.ID,
		Error:   msg,
		LoopID:  call.LoopID,
		TraceID: call.TraceID,
	}
}
