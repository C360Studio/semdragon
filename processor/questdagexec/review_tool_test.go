package questdagexec

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/agentic"
)

// reviewResult is the JSON shape returned by a successful review_sub_quest call.
type reviewResult struct {
	Verdict      string  `json:"verdict"`
	AvgRating    float64 `json:"avg_rating"`
	SubQuestID   string  `json:"sub_quest_id"`
	Explanation  string  `json:"explanation,omitempty"`
}

func TestReviewToolExecute(t *testing.T) {
	t.Parallel()

	exec := NewReviewExecutor()

	// makeCall builds a ToolCall with the given arguments.
	makeCall := func(args map[string]any) agentic.ToolCall {
		return agentic.ToolCall{
			ID:        "review-call-1",
			Name:      "review_sub_quest",
			Arguments: args,
		}
	}

	// goodRatings is a valid ratings object where avg >= 3.0.
	goodRatings := map[string]any{"q1": float64(4), "q2": float64(4), "q3": float64(4)}

	// badRatings is a valid ratings object where avg < 3.0.
	badRatings := map[string]any{"q1": float64(2), "q2": float64(2), "q3": float64(2)}

	// borderRatings averages exactly 3.0 (accept threshold).
	borderRatings := map[string]any{"q1": float64(3), "q2": float64(3), "q3": float64(3)}

	tests := []struct {
		name        string
		args        map[string]any
		wantToolErr bool
		errContains string
		checkResult func(t *testing.T, r reviewResult)
	}{
		// -------------------------------------------------------------------------
		// Valid accept paths
		// -------------------------------------------------------------------------
		{
			name: "accept verdict with good ratings",
			args: map[string]any{
				"sub_quest_id": "sq-abc",
				"ratings":      goodRatings,
				"verdict":      "accept",
			},
			checkResult: func(t *testing.T, r reviewResult) {
				t.Helper()
				if r.Verdict != "accept" {
					t.Errorf("Verdict = %q, want %q", r.Verdict, "accept")
				}
				if r.SubQuestID != "sq-abc" {
					t.Errorf("SubQuestID = %q, want %q", r.SubQuestID, "sq-abc")
				}
				wantAvg := (4.0 + 4.0 + 4.0) / 3.0
				if r.AvgRating != wantAvg {
					t.Errorf("AvgRating = %.4f, want %.4f", r.AvgRating, wantAvg)
				}
			},
		},
		{
			name: "accept verdict at border avg 3.0 without explanation",
			args: map[string]any{
				"sub_quest_id": "sq-border",
				"ratings":      borderRatings,
				"verdict":      "accept",
			},
			checkResult: func(t *testing.T, r reviewResult) {
				t.Helper()
				if r.Verdict != "accept" {
					t.Errorf("Verdict = %q, want %q", r.Verdict, "accept")
				}
				wantAvg := 3.0
				if r.AvgRating != wantAvg {
					t.Errorf("AvgRating = %.4f, want %.4f", r.AvgRating, wantAvg)
				}
			},
		},
		{
			name: "accept with low ratings still works — explicit verdict overrides threshold",
			args: map[string]any{
				"sub_quest_id": "sq-override",
				"ratings":      badRatings,
				"explanation":  "Acceptable despite low scores due to circumstances",
				"verdict":      "accept",
			},
			checkResult: func(t *testing.T, r reviewResult) {
				t.Helper()
				if r.Verdict != "accept" {
					t.Errorf("Verdict = %q, want %q", r.Verdict, "accept")
				}
			},
		},

		// -------------------------------------------------------------------------
		// Valid reject paths
		// -------------------------------------------------------------------------
		{
			name: "reject verdict with explanation",
			args: map[string]any{
				"sub_quest_id": "sq-reject",
				"ratings":      badRatings,
				"explanation":  "Output was missing error handling",
				"verdict":      "reject",
			},
			checkResult: func(t *testing.T, r reviewResult) {
				t.Helper()
				if r.Verdict != "reject" {
					t.Errorf("Verdict = %q, want %q", r.Verdict, "reject")
				}
				if r.SubQuestID != "sq-reject" {
					t.Errorf("SubQuestID = %q, want %q", r.SubQuestID, "sq-reject")
				}
				wantAvg := (2.0 + 2.0 + 2.0) / 3.0
				if r.AvgRating != wantAvg {
					t.Errorf("AvgRating = %.4f, want %.4f", r.AvgRating, wantAvg)
				}
				if r.Explanation != "Output was missing error handling" {
					t.Errorf("Explanation = %q, want explanation to be preserved", r.Explanation)
				}
			},
		},
		{
			name: "reject verdict with good ratings and explanation",
			args: map[string]any{
				"sub_quest_id": "sq-highreject",
				"ratings":      goodRatings,
				"explanation":  "Rejected on strategic grounds despite good execution",
				"verdict":      "reject",
			},
			checkResult: func(t *testing.T, r reviewResult) {
				t.Helper()
				if r.Verdict != "reject" {
					t.Errorf("Verdict = %q, want %q", r.Verdict, "reject")
				}
			},
		},

		// -------------------------------------------------------------------------
		// Explanation required when avg < 3.0
		// -------------------------------------------------------------------------
		{
			name: "reject without explanation when avg < 3.0 returns ToolResult.Error",
			args: map[string]any{
				"sub_quest_id": "sq-noexpl",
				"ratings":      badRatings,
				"verdict":      "reject",
			},
			wantToolErr: true,
			errContains: "explanation",
		},
		{
			name: "accept without explanation when avg < 3.0 is allowed",
			args: map[string]any{
				"sub_quest_id": "sq-lowaccept",
				"ratings":      badRatings,
				"verdict":      "accept",
			},
			// accept with low rating but no explanation is allowed — verdict overrides
			checkResult: func(t *testing.T, r reviewResult) {
				t.Helper()
				if r.Verdict != "accept" {
					t.Errorf("Verdict = %q, want %q", r.Verdict, "accept")
				}
			},
		},

		// -------------------------------------------------------------------------
		// Rating range validation
		// -------------------------------------------------------------------------
		{
			name: "rating of 0 is out of range",
			args: map[string]any{
				"sub_quest_id": "sq-zero",
				"ratings":      map[string]any{"q1": float64(0), "q2": float64(3), "q3": float64(3)},
				"verdict":      "accept",
			},
			wantToolErr: true,
			errContains: "1",
		},
		{
			name: "rating of 6 is out of range",
			args: map[string]any{
				"sub_quest_id": "sq-six",
				"ratings":      map[string]any{"q1": float64(6), "q2": float64(3), "q3": float64(3)},
				"verdict":      "accept",
			},
			wantToolErr: true,
			errContains: "5",
		},
		{
			name: "rating of 1 is valid minimum",
			args: map[string]any{
				"sub_quest_id": "sq-min",
				"ratings":      map[string]any{"q1": float64(1), "q2": float64(1), "q3": float64(1)},
				"explanation":  "Very poor output",
				"verdict":      "reject",
			},
			checkResult: func(t *testing.T, r reviewResult) {
				t.Helper()
				wantAvg := 1.0
				if r.AvgRating != wantAvg {
					t.Errorf("AvgRating = %.4f, want %.4f", r.AvgRating, wantAvg)
				}
			},
		},
		{
			name: "rating of 5 is valid maximum",
			args: map[string]any{
				"sub_quest_id": "sq-max",
				"ratings":      map[string]any{"q1": float64(5), "q2": float64(5), "q3": float64(5)},
				"verdict":      "accept",
			},
			checkResult: func(t *testing.T, r reviewResult) {
				t.Helper()
				wantAvg := 5.0
				if r.AvgRating != wantAvg {
					t.Errorf("AvgRating = %.4f, want %.4f", r.AvgRating, wantAvg)
				}
			},
		},
		{
			name: "mixed ratings average correctly",
			args: map[string]any{
				"sub_quest_id": "sq-mixed",
				"ratings":      map[string]any{"q1": float64(3), "q2": float64(4), "q3": float64(5)},
				"verdict":      "accept",
			},
			checkResult: func(t *testing.T, r reviewResult) {
				t.Helper()
				wantAvg := (3.0 + 4.0 + 5.0) / 3.0
				if r.AvgRating != wantAvg {
					t.Errorf("AvgRating = %.4f, want %.4f", r.AvgRating, wantAvg)
				}
			},
		},

		// -------------------------------------------------------------------------
		// Missing required arguments
		// -------------------------------------------------------------------------
		{
			name: "missing sub_quest_id returns ToolResult.Error",
			args: map[string]any{
				"ratings": goodRatings,
				"verdict": "accept",
			},
			wantToolErr: true,
			errContains: "sub_quest_id",
		},
		{
			name: "empty sub_quest_id returns ToolResult.Error",
			args: map[string]any{
				"sub_quest_id": "",
				"ratings":      goodRatings,
				"verdict":      "accept",
			},
			wantToolErr: true,
			errContains: "sub_quest_id",
		},
		{
			name: "missing ratings returns ToolResult.Error",
			args: map[string]any{
				"sub_quest_id": "sq-abc",
				"verdict":      "accept",
			},
			wantToolErr: true,
			errContains: "ratings",
		},
		{
			name: "missing verdict returns ToolResult.Error",
			args: map[string]any{
				"sub_quest_id": "sq-abc",
				"ratings":      goodRatings,
			},
			wantToolErr: true,
			errContains: "verdict",
		},

		// -------------------------------------------------------------------------
		// Invalid verdict string
		// -------------------------------------------------------------------------
		{
			name: "invalid verdict string returns ToolResult.Error",
			args: map[string]any{
				"sub_quest_id": "sq-abc",
				"ratings":      goodRatings,
				"verdict":      "maybe",
			},
			wantToolErr: true,
			errContains: "verdict",
		},
		{
			name: "verdict is case sensitive — Accept is invalid",
			args: map[string]any{
				"sub_quest_id": "sq-abc",
				"ratings":      goodRatings,
				"verdict":      "Accept",
			},
			wantToolErr: true,
			errContains: "verdict",
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

			var r reviewResult
			if err := json.Unmarshal([]byte(result.Content), &r); err != nil {
				t.Fatalf("ToolResult.Content is not valid JSON: %v\ncontent: %s", err, result.Content)
			}

			if tc.checkResult != nil {
				tc.checkResult(t, r)
			}
		})
	}

	// Verify call metadata (ID, LoopID, TraceID) is propagated correctly.
	t.Run("call IDs propagated to result", func(t *testing.T) {
		t.Parallel()
		call := agentic.ToolCall{
			ID:      "review-cid",
			Name:    "review_sub_quest",
			LoopID:  "loop-7",
			TraceID: "trace-8",
			Arguments: map[string]any{
				"sub_quest_id": "sq-abc",
				"ratings":      goodRatings,
				"verdict":      "accept",
			},
		}
		result, err := exec.Execute(context.Background(), call)
		if err != nil {
			t.Fatalf("Execute() error: %v", err)
		}
		if result.CallID != "review-cid" {
			t.Errorf("CallID = %q, want %q", result.CallID, "review-cid")
		}
		if result.LoopID != "loop-7" {
			t.Errorf("LoopID = %q, want %q", result.LoopID, "loop-7")
		}
		if result.TraceID != "trace-8" {
			t.Errorf("TraceID = %q, want %q", result.TraceID, "trace-8")
		}
	})
}

func TestReviewToolListTools(t *testing.T) {
	t.Parallel()
	exec := NewReviewExecutor()
	tools := exec.ListTools()
	if len(tools) != 1 {
		t.Fatalf("ListTools() returned %d tools, want 1", len(tools))
	}
	tool := tools[0]
	if tool.Name != "review_sub_quest" {
		t.Errorf("tool name = %q, want %q", tool.Name, "review_sub_quest")
	}
	if tool.Description == "" {
		t.Error("tool description is empty")
	}
	if tool.Parameters == nil {
		t.Error("tool parameters are nil")
	}
}
