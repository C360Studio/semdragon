package questdagexec

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/agentic"
)

func TestDecomposeToolExecute(t *testing.T) {
	t.Parallel()

	exec := NewDecomposeExecutor()

	// validCall builds a ToolCall with the given arguments map.
	validCall := func(args map[string]any) agentic.ToolCall {
		return agentic.ToolCall{
			ID:        "call-1",
			Name:      "decompose_quest",
			Arguments: args,
		}
	}

	// validNodes is a well-formed nodes array that passes validation.
	validNodes := []any{
		map[string]any{
			"id":        "a",
			"objective": "Write the add function",
			"skills":    []any{"code_generation"},
		},
		map[string]any{
			"id":        "b",
			"objective": "Write the subtract function",
			"skills":    []any{"code_generation"},
		},
		map[string]any{
			"id":        "c",
			"objective": "Combine into a module",
			"depends_on": []any{"a", "b"},
		},
	}

	tests := []struct {
		name        string
		args        map[string]any
		wantErr     bool // Go-level error (infrastructure failure)
		wantToolErr bool // ToolResult.Error set (validation failure)
		errContains string
		checkResult func(t *testing.T, result agentic.ToolResult)
	}{
		{
			name: "valid decomposition returns validated DAG JSON",
			args: map[string]any{
				"goal":  "Build a math module",
				"nodes": validNodes,
			},
			checkResult: func(t *testing.T, result agentic.ToolResult) {
				t.Helper()
				if result.Error != "" {
					t.Fatalf("unexpected ToolResult.Error: %q", result.Error)
				}
				if result.Content == "" {
					t.Fatal("ToolResult.Content is empty, want DAG JSON")
				}
				// Content must be valid JSON containing goal and dag.
				var envelope map[string]any
				if err := json.Unmarshal([]byte(result.Content), &envelope); err != nil {
					t.Fatalf("ToolResult.Content is not valid JSON: %v", err)
				}
				if _, ok := envelope["goal"]; !ok {
					t.Error("ToolResult.Content missing 'goal' field")
				}
				if _, ok := envelope["dag"]; !ok {
					t.Error("ToolResult.Content missing 'dag' field")
				}
			},
		},
		{
			name: "missing goal returns ToolResult.Error",
			args: map[string]any{
				"nodes": validNodes,
			},
			wantToolErr: true,
			errContains: "goal",
		},
		{
			name: "empty goal returns ToolResult.Error",
			args: map[string]any{
				"goal":  "",
				"nodes": validNodes,
			},
			wantToolErr: true,
			errContains: "goal",
		},
		{
			name: "missing nodes returns ToolResult.Error",
			args: map[string]any{
				"goal": "Build something",
			},
			wantToolErr: true,
			errContains: "nodes",
		},
		{
			name: "empty nodes array returns ToolResult.Error",
			args: map[string]any{
				"goal":  "Build something",
				"nodes": []any{},
			},
			wantToolErr: true,
			errContains: "empty",
		},
		{
			name: "nodes with cycle returns ToolResult.Error with validation message",
			args: map[string]any{
				"goal": "Cyclic DAG",
				"nodes": []any{
					map[string]any{"id": "x", "objective": "X", "depends_on": []any{"y"}},
					map[string]any{"id": "y", "objective": "Y", "depends_on": []any{"x"}},
				},
			},
			wantToolErr: true,
			errContains: "cycle",
		},
		{
			name: "node with unknown dep ref returns ToolResult.Error",
			args: map[string]any{
				"goal": "Bad ref",
				"nodes": []any{
					map[string]any{"id": "a", "objective": "A", "depends_on": []any{"ghost"}},
				},
			},
			wantToolErr: true,
			errContains: "unknown",
		},
		{
			name: "21 nodes exceeds max returns ToolResult.Error",
			args: map[string]any{
				"goal":  "Too many nodes",
				"nodes": buildRawNodes(21),
			},
			wantToolErr: true,
			errContains: "maximum",
		},
		{
			name: "node with empty objective returns ToolResult.Error",
			args: map[string]any{
				"goal": "Empty objective",
				"nodes": []any{
					map[string]any{"id": "a", "objective": ""},
				},
			},
			wantToolErr: true,
			errContains: "objective",
		},
		{
			name: "nodes is not an array returns ToolResult.Error",
			args: map[string]any{
				"goal":  "Bad nodes type",
				"nodes": "not-an-array",
			},
			wantToolErr: true,
			errContains: "array",
		},
		{
			name: "call ID is propagated to ToolResult",
			args: map[string]any{
				"goal":  "Build something",
				"nodes": validNodes,
			},
			checkResult: func(t *testing.T, result agentic.ToolResult) {
				t.Helper()
				if result.CallID != "call-1" {
					t.Fatalf("ToolResult.CallID = %q, want %q", result.CallID, "call-1")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			call := validCall(tc.args)
			result, err := exec.Execute(context.Background(), call)
			if tc.wantErr {
				if err == nil {
					t.Fatal("Execute() returned nil error, want Go error")
				}
				return
			}
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
				// A validation error must never set Content.
				if result.Content != "" {
					t.Fatalf("ToolResult.Content should be empty on error, got %q", result.Content)
				}
				return
			}

			if tc.checkResult != nil {
				tc.checkResult(t, result)
			}
		})
	}

	// Separate sub-test to verify LoopID/TraceID propagation on both success and error paths.
	t.Run("loop and trace IDs propagated on success", func(t *testing.T) {
		t.Parallel()
		call := agentic.ToolCall{
			ID:      "cid",
			Name:    "decompose_quest",
			LoopID:  "loop-42",
			TraceID: "trace-99",
			Arguments: map[string]any{
				"goal":  "Build something",
				"nodes": validNodes,
			},
		}
		result, err := exec.Execute(context.Background(), call)
		if err != nil {
			t.Fatalf("Execute() error: %v", err)
		}
		if result.LoopID != "loop-42" {
			t.Errorf("LoopID = %q, want %q", result.LoopID, "loop-42")
		}
		if result.TraceID != "trace-99" {
			t.Errorf("TraceID = %q, want %q", result.TraceID, "trace-99")
		}
	})

	t.Run("loop and trace IDs propagated on error", func(t *testing.T) {
		t.Parallel()
		call := agentic.ToolCall{
			ID:      "cid",
			Name:    "decompose_quest",
			LoopID:  "loop-42",
			TraceID: "trace-99",
			Arguments: map[string]any{
				"goal":  "",
				"nodes": validNodes,
			},
		}
		result, err := exec.Execute(context.Background(), call)
		if err != nil {
			t.Fatalf("Execute() error: %v", err)
		}
		if result.LoopID != "loop-42" {
			t.Errorf("LoopID = %q, want %q", result.LoopID, "loop-42")
		}
		if result.TraceID != "trace-99" {
			t.Errorf("TraceID = %q, want %q", result.TraceID, "trace-99")
		}
	})
}

func TestDecomposeToolListTools(t *testing.T) {
	t.Parallel()
	exec := NewDecomposeExecutor()
	tools := exec.ListTools()
	if len(tools) != 1 {
		t.Fatalf("ListTools() returned %d tools, want 1", len(tools))
	}
	tool := tools[0]
	if tool.Name != "decompose_quest" {
		t.Errorf("tool name = %q, want %q", tool.Name, "decompose_quest")
	}
	if tool.Description == "" {
		t.Error("tool description is empty")
	}
	if tool.Parameters == nil {
		t.Error("tool parameters are nil")
	}
}

// buildRawNodes produces n raw node maps suitable for the nodes argument.
// Each node is independent (no dependencies) to keep the helper simple.
func buildRawNodes(n int) []any {
	nodes := make([]any, n)
	for i := range nodes {
		nodes[i] = map[string]any{
			"id":        fmt.Sprintf("n%d", i+1),
			"objective": fmt.Sprintf("Node %d", i+1),
		}
	}
	return nodes
}
