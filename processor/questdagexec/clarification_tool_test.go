package questdagexec

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/agentic"
)

func TestClarificationToolDefinition(t *testing.T) {
	t.Parallel()

	exec := NewClarificationExecutor()
	tools := exec.ListTools()

	if len(tools) != 1 {
		t.Fatalf("ListTools() returned %d tools, want 1", len(tools))
	}

	tool := tools[0]

	if tool.Name != clarificationToolName {
		t.Errorf("tool name = %q, want %q", tool.Name, clarificationToolName)
	}

	if tool.Description == "" {
		t.Error("tool description is empty")
	}

	if tool.Parameters == nil {
		t.Fatal("tool parameters are nil")
	}

	required, ok := tool.Parameters["required"].([]string)
	if !ok {
		t.Fatal("tool parameters[\"required\"] is not []string")
	}

	wantRequired := map[string]bool{"sub_quest_id": true, "answer": true}
	if len(required) != len(wantRequired) {
		t.Errorf("required fields count = %d, want %d", len(required), len(wantRequired))
	}
	for _, field := range required {
		if !wantRequired[field] {
			t.Errorf("unexpected required field %q", field)
		}
	}

	props, ok := tool.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatal("tool parameters[\"properties\"] is not map[string]any")
	}

	for _, field := range []string{"sub_quest_id", "answer"} {
		prop, ok := props[field].(map[string]any)
		if !ok {
			t.Errorf("property %q is missing or not a map", field)
			continue
		}
		if prop["type"] != "string" {
			t.Errorf("property %q type = %v, want \"string\"", field, prop["type"])
		}
		if desc, _ := prop["description"].(string); desc == "" {
			t.Errorf("property %q has empty description", field)
		}
	}
}

func TestClarificationExecutorListTools(t *testing.T) {
	t.Parallel()

	exec := NewClarificationExecutor()
	tools := exec.ListTools()

	if len(tools) != 1 {
		t.Fatalf("ListTools() returned %d tools, want 1", len(tools))
	}

	tool := tools[0]
	if tool.Name != "answer_clarification" {
		t.Errorf("tool name = %q, want %q", tool.Name, "answer_clarification")
	}
}

func TestClarificationExecutorExecute(t *testing.T) {
	t.Parallel()

	exec := NewClarificationExecutor()

	makeCall := func(args map[string]any) agentic.ToolCall {
		return agentic.ToolCall{
			ID:        "clarify-call-1",
			Name:      clarificationToolName,
			Arguments: args,
		}
	}

	tests := []struct {
		name        string
		args        map[string]any
		wantToolErr bool
		errContains string
		checkResult func(t *testing.T, content string)
	}{
		{
			name: "valid sub_quest_id and answer returns JSON envelope",
			args: map[string]any{
				"sub_quest_id": "sq-abc",
				"answer":       "Use JSON with keys: name, score, summary.",
			},
			checkResult: func(t *testing.T, content string) {
				t.Helper()
				var r struct {
					SubQuestID string `json:"sub_quest_id"`
					Answer     string `json:"answer"`
				}
				if err := json.Unmarshal([]byte(content), &r); err != nil {
					t.Fatalf("content is not valid JSON: %v\ncontent: %s", err, content)
				}
				if r.SubQuestID != "sq-abc" {
					t.Errorf("sub_quest_id = %q, want %q", r.SubQuestID, "sq-abc")
				}
				if r.Answer != "Use JSON with keys: name, score, summary." {
					t.Errorf("answer = %q, want the exact answer text", r.Answer)
				}
			},
		},
		{
			name: "missing sub_quest_id returns ToolResult.Error",
			args: map[string]any{
				"answer": "some answer",
			},
			wantToolErr: true,
			errContains: "sub_quest_id",
		},
		{
			name: "empty sub_quest_id returns ToolResult.Error",
			args: map[string]any{
				"sub_quest_id": "",
				"answer":       "some answer",
			},
			wantToolErr: true,
			errContains: "sub_quest_id",
		},
		{
			name: "missing answer returns ToolResult.Error",
			args: map[string]any{
				"sub_quest_id": "sq-abc",
			},
			wantToolErr: true,
			errContains: "answer",
		},
		{
			name: "empty answer returns ToolResult.Error",
			args: map[string]any{
				"sub_quest_id": "sq-abc",
				"answer":       "",
			},
			wantToolErr: true,
			errContains: "answer",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			call := makeCall(tc.args)
			result, err := exec.Execute(context.Background(), call)
			if err != nil {
				t.Fatalf("Execute() returned unexpected Go error: %v", err)
			}

			if tc.wantToolErr {
				if result.Error == "" {
					t.Fatalf("Execute() ToolResult.Error is empty, want error containing %q", tc.errContains)
				}
				if tc.errContains != "" && !strings.Contains(strings.ToLower(result.Error), strings.ToLower(tc.errContains)) {
					t.Fatalf("ToolResult.Error = %q, want it to contain %q", result.Error, tc.errContains)
				}
				if result.Content != "" {
					t.Fatalf("ToolResult.Content should be empty on error, got %q", result.Content)
				}
				return
			}

			if result.Error != "" {
				t.Fatalf("Execute() unexpected ToolResult.Error: %q", result.Error)
			}
			if result.Content == "" {
				t.Fatal("ToolResult.Content is empty on success")
			}

			if tc.checkResult != nil {
				tc.checkResult(t, result.Content)
			}
		})
	}

	// Verify call metadata (ID, LoopID, TraceID) is propagated correctly.
	t.Run("call IDs propagated to result", func(t *testing.T) {
		t.Parallel()
		call := agentic.ToolCall{
			ID:      "clarify-cid",
			Name:    clarificationToolName,
			LoopID:  "clarify-loop-7",
			TraceID: "trace-8",
			Arguments: map[string]any{
				"sub_quest_id": "sq-abc",
				"answer":       "The output format should be a markdown table.",
			},
		}
		result, err := exec.Execute(context.Background(), call)
		if err != nil {
			t.Fatalf("Execute() error: %v", err)
		}
		if result.CallID != "clarify-cid" {
			t.Errorf("CallID = %q, want %q", result.CallID, "clarify-cid")
		}
		if result.LoopID != "clarify-loop-7" {
			t.Errorf("LoopID = %q, want %q", result.LoopID, "clarify-loop-7")
		}
		if result.TraceID != "trace-8" {
			t.Errorf("TraceID = %q, want %q", result.TraceID, "trace-8")
		}
	})
}
