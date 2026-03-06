package questdagexec

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/c360studio/semstreams/agentic"
)

const decomposeToolName = "decompose_quest"

// DecomposeExecutor implements the decompose_quest tool.
// It is a pure validation passthrough: the LLM proposes the DAG structure in
// its tool call arguments, the executor validates it, and returns the validated
// DAG as JSON in ToolResult.Content. No sub-quests are posted here — that
// happens in questbridge when the lead's agentic loop completes.
//
// All public methods are safe for concurrent use — the struct holds no mutable
// state.
type DecomposeExecutor struct{}

// NewDecomposeExecutor constructs a DecomposeExecutor.
func NewDecomposeExecutor() *DecomposeExecutor {
	return &DecomposeExecutor{}
}

// Execute validates the QuestDAG provided by the LLM and returns it as JSON.
//
// Argument validation errors are surfaced as non-nil ToolResult.Error strings
// rather than Go errors. Go errors are reserved for infrastructure failures
// that the dispatcher should treat as fatal — none arise in this passthrough
// implementation.
func (e *DecomposeExecutor) Execute(_ context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	goal, ok := stringArg(call.Arguments, "goal")
	if !ok || goal == "" {
		return decomposeErrorResult(call, `missing required argument "goal"`), nil
	}

	rawNodes, ok := call.Arguments["nodes"]
	if !ok {
		return decomposeErrorResult(call, `missing required argument "nodes"`), nil
	}

	dag, err := parseQuestNodes(rawNodes)
	if err != nil {
		return decomposeErrorResult(call, fmt.Sprintf("invalid nodes argument: %s", err)), nil
	}

	if err := dag.Validate(); err != nil {
		return decomposeErrorResult(call, fmt.Sprintf("invalid dag: %s", err)), nil
	}

	response := map[string]any{
		"goal": goal,
		"dag":  dag,
	}

	return decomposeJSONResult(call, response)
}

// ListTools returns the single tool definition for decompose_quest.
func (e *DecomposeExecutor) ListTools() []agentic.ToolDefinition {
	return []agentic.ToolDefinition{{
		Name:        decomposeToolName,
		Description: "Decompose a complex party quest into a DAG of sub-quests. Provide the goal and a list of quest nodes with their dependencies. The validated DAG is returned for execution by the party.",
		Parameters: map[string]any{
			"type":     "object",
			"required": []string{"goal", "nodes"},
			"properties": map[string]any{
				"goal": map[string]any{
					"type":        "string",
					"description": "High-level decomposition rationale for the party quest",
				},
				"nodes": map[string]any{
					"type":        "array",
					"description": "Sub-quest nodes forming the DAG",
					"items": map[string]any{
						"type":     "object",
						"required": []string{"id", "objective"},
						"properties": map[string]any{
							"id": map[string]any{
								"type":        "string",
								"description": "Unique node identifier within this DAG",
							},
							"objective": map[string]any{
								"type":        "string",
								"description": "What the sub-quest must accomplish",
							},
							"skills": map[string]any{
								"type":        "array",
								"description": "Required skill tags for the sub-quest (e.g. code_generation)",
								"items":       map[string]any{"type": "string"},
							},
							"difficulty": map[string]any{
								"type":        "integer",
								"description": "Difficulty level 0-5 (0=trivial, 5=legendary)",
							},
							"acceptance": map[string]any{
								"type":        "array",
								"description": "Acceptance criteria the lead will evaluate during review",
								"items":       map[string]any{"type": "string"},
							},
							"depends_on": map[string]any{
								"type":        "array",
								"description": "IDs of nodes that must complete before this one",
								"items":       map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
	}}
}

// -- helpers --

// parseQuestNodes converts the raw "nodes" argument (a []any from JSON
// unmarshalling into map[string]any) into a QuestDAG.
// Each element must be a map[string]any with at least "id" and "objective".
func parseQuestNodes(raw any) (QuestDAG, error) {
	slice, ok := raw.([]any)
	if !ok {
		return QuestDAG{}, fmt.Errorf("nodes must be an array, got %T", raw)
	}
	if len(slice) == 0 {
		return QuestDAG{}, fmt.Errorf("nodes array must not be empty")
	}

	nodes := make([]QuestNode, 0, len(slice))
	for i, item := range slice {
		m, ok := item.(map[string]any)
		if !ok {
			return QuestDAG{}, fmt.Errorf("nodes[%d] must be an object, got %T", i, item)
		}

		id, ok := stringField(m, "id")
		if !ok || id == "" {
			return QuestDAG{}, fmt.Errorf("nodes[%d]: missing required field \"id\"", i)
		}
		objective, ok := stringField(m, "objective")
		if !ok || objective == "" {
			return QuestDAG{}, fmt.Errorf("nodes[%d]: missing required field \"objective\"", i)
		}

		var dependsOn []string
		if rawDeps, exists := m["depends_on"]; exists && rawDeps != nil {
			deps, ok := rawDeps.([]any)
			if !ok {
				return QuestDAG{}, fmt.Errorf("nodes[%d]: depends_on must be an array, got %T", i, rawDeps)
			}
			for j, dep := range deps {
				s, ok := dep.(string)
				if !ok {
					return QuestDAG{}, fmt.Errorf("nodes[%d].depends_on[%d] must be a string, got %T", i, j, dep)
				}
				dependsOn = append(dependsOn, s)
			}
		}

		var skills []string
		if rawSkills, exists := m["skills"]; exists && rawSkills != nil {
			sl, ok := rawSkills.([]any)
			if !ok {
				return QuestDAG{}, fmt.Errorf("nodes[%d]: skills must be an array, got %T", i, rawSkills)
			}
			for j, sk := range sl {
				s, ok := sk.(string)
				if !ok {
					return QuestDAG{}, fmt.Errorf("nodes[%d].skills[%d] must be a string, got %T", i, j, sk)
				}
				skills = append(skills, s)
			}
		}

		var acceptance []string
		if rawAcc, exists := m["acceptance"]; exists && rawAcc != nil {
			al, ok := rawAcc.([]any)
			if !ok {
				return QuestDAG{}, fmt.Errorf("nodes[%d]: acceptance must be an array, got %T", i, rawAcc)
			}
			for j, ac := range al {
				s, ok := ac.(string)
				if !ok {
					return QuestDAG{}, fmt.Errorf("nodes[%d].acceptance[%d] must be a string, got %T", i, j, ac)
				}
				acceptance = append(acceptance, s)
			}
		}

		difficulty := 0
		if rawDiff, exists := m["difficulty"]; exists && rawDiff != nil {
			// JSON numbers unmarshal as float64 in map[string]any.
			if f, ok := rawDiff.(float64); ok {
				difficulty = int(f)
			}
		}

		nodes = append(nodes, QuestNode{
			ID:         id,
			Objective:  objective,
			Skills:     skills,
			Difficulty: difficulty,
			Acceptance: acceptance,
			DependsOn:  dependsOn,
		})
	}

	return QuestDAG{Nodes: nodes}, nil
}

// decomposeJSONResult marshals v to JSON and returns a successful ToolResult.
func decomposeJSONResult(call agentic.ToolCall, v any) (agentic.ToolResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return decomposeErrorResult(call, fmt.Sprintf("failed to marshal result: %s", err)), nil
	}
	return agentic.ToolResult{
		CallID:  call.ID,
		Content: string(data),
		LoopID:  call.LoopID,
		TraceID: call.TraceID,
	}, nil
}

// decomposeErrorResult returns a ToolResult carrying an error message.
// ToolResult.Error is forwarded to the LLM as structured feedback so it can
// correct its output. This is not a Go error — Go errors from Execute signal
// infrastructure failures, not validation failures.
func decomposeErrorResult(call agentic.ToolCall, msg string) agentic.ToolResult {
	return agentic.ToolResult{
		CallID:  call.ID,
		Error:   msg,
		LoopID:  call.LoopID,
		TraceID: call.TraceID,
	}
}

// stringArg extracts a string value from the top-level arguments map by key.
// Returns ("", false) when the key is absent or the value is not a string.
func stringArg(args map[string]any, key string) (string, bool) {
	v, ok := args[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// stringField extracts a string value from an object field map by key.
// Returns ("", false) when the key is absent or the value is not a string.
func stringField(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}
