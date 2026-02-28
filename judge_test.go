package semdragons

import (
	"context"
	"testing"
)

func TestAutomatedJudge_FormatCheck_NonNilPasses(t *testing.T) {
	judge := NewAutomatedJudge()
	ctx := context.Background()

	input := JudgeInput{
		Judge:     Judge{ID: "auto", Type: JudgeAutomated},
		Quest:     Quest{ID: "test-quest"},
		Output:    map[string]string{"result": "data"},
		Criterion: ReviewCriterion{Name: "format", Threshold: 0.9},
	}

	output, err := judge.Evaluate(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Score != 1.0 {
		t.Errorf("expected score 1.0 for valid output, got %f", output.Score)
	}
	if !output.Passed {
		t.Error("expected format check to pass for valid output")
	}
}

func TestAutomatedJudge_FormatCheck_NilFails(t *testing.T) {
	judge := NewAutomatedJudge()
	ctx := context.Background()

	input := JudgeInput{
		Judge:     Judge{ID: "auto", Type: JudgeAutomated},
		Quest:     Quest{ID: "test-quest"},
		Output:    nil,
		Criterion: ReviewCriterion{Name: "format", Threshold: 0.9},
	}

	output, err := judge.Evaluate(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Score != 0.0 {
		t.Errorf("expected score 0.0 for nil output, got %f", output.Score)
	}
	if output.Passed {
		t.Error("expected format check to fail for nil output")
	}
}

func TestAutomatedJudge_CompletenessCheck(t *testing.T) {
	judge := NewAutomatedJudge()
	ctx := context.Background()

	// Test with map having entries
	input := JudgeInput{
		Judge:     Judge{ID: "auto", Type: JudgeAutomated},
		Quest:     Quest{ID: "test-quest"},
		Output:    map[string]string{"key": "value"},
		Criterion: ReviewCriterion{Name: "completeness", Threshold: 0.8},
	}

	output, err := judge.Evaluate(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Score != 1.0 {
		t.Errorf("expected score 1.0 for populated map, got %f", output.Score)
	}
	if !output.Passed {
		t.Error("expected completeness check to pass for populated map")
	}
}

func TestAutomatedJudge_CompletenessCheck_EmptyMapFails(t *testing.T) {
	judge := NewAutomatedJudge()
	ctx := context.Background()

	input := JudgeInput{
		Judge:     Judge{ID: "auto", Type: JudgeAutomated},
		Quest:     Quest{ID: "test-quest"},
		Output:    map[string]string{},
		Criterion: ReviewCriterion{Name: "completeness", Threshold: 0.8},
	}

	output, err := judge.Evaluate(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Score != 0.0 {
		t.Errorf("expected score 0.0 for empty map, got %f", output.Score)
	}
	if output.Passed {
		t.Error("expected completeness check to fail for empty map")
	}
}

func TestAutomatedJudge_NonEmptyCheck(t *testing.T) {
	judge := NewAutomatedJudge()
	ctx := context.Background()

	// Test with meaningful string content
	input := JudgeInput{
		Judge:     Judge{ID: "auto", Type: JudgeAutomated},
		Quest:     Quest{ID: "test-quest"},
		Output:    "This is a meaningful response with enough content",
		Criterion: ReviewCriterion{Name: "non_empty", Threshold: 0.5},
	}

	output, err := judge.Evaluate(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Score < 0.5 {
		t.Errorf("expected score >= 0.5 for meaningful string, got %f", output.Score)
	}
	if !output.Passed {
		t.Error("expected non_empty check to pass for meaningful string")
	}
}

func TestAutomatedJudge_NonEmptyCheck_EmptyStringFails(t *testing.T) {
	judge := NewAutomatedJudge()
	ctx := context.Background()

	input := JudgeInput{
		Judge:     Judge{ID: "auto", Type: JudgeAutomated},
		Quest:     Quest{ID: "test-quest"},
		Output:    "",
		Criterion: ReviewCriterion{Name: "non_empty", Threshold: 0.5},
	}

	output, err := judge.Evaluate(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Score != 0.0 {
		t.Errorf("expected score 0.0 for empty string, got %f", output.Score)
	}
	if output.Passed {
		t.Error("expected non_empty check to fail for empty string")
	}
}

func TestLLMJudge_ReturnsPlaceholder(t *testing.T) {
	judge := NewLLMJudge(nil) // Stub mode
	ctx := context.Background()

	input := JudgeInput{
		Judge:     Judge{ID: "llm", Type: JudgeLLM},
		Quest:     Quest{ID: "test-quest"},
		Output:    "some output",
		Criterion: ReviewCriterion{Name: "quality", Threshold: 0.6},
	}

	output, err := judge.Evaluate(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Stub returns 0.75
	if output.Score != 0.75 {
		t.Errorf("expected LLM stub score 0.75, got %f", output.Score)
	}
	if !output.Passed {
		t.Error("expected LLM judge to pass with 0.75 score and 0.6 threshold")
	}
	if output.Pending {
		t.Error("LLM judge should not be pending")
	}
}

func TestLLMJudge_NilOutputReturnsZero(t *testing.T) {
	judge := NewLLMJudge(nil) // Stub mode
	ctx := context.Background()

	input := JudgeInput{
		Judge:     Judge{ID: "llm", Type: JudgeLLM},
		Quest:     Quest{ID: "test-quest"},
		Output:    nil,
		Criterion: ReviewCriterion{Name: "quality", Threshold: 0.6},
	}

	output, err := judge.Evaluate(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Score != 0.0 {
		t.Errorf("expected score 0.0 for nil output, got %f", output.Score)
	}
	if output.Passed {
		t.Error("expected LLM judge to fail for nil output")
	}
}

func TestHumanJudge_ReturnsPending(t *testing.T) {
	judge := NewHumanJudge()
	ctx := context.Background()

	input := JudgeInput{
		Judge:     Judge{ID: "human", Type: JudgeHuman},
		Quest:     Quest{ID: "test-quest"},
		Output:    "some output",
		Criterion: ReviewCriterion{Name: "creativity", Threshold: 0.6},
	}

	output, err := judge.Evaluate(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !output.Pending {
		t.Error("expected human judge to return pending=true")
	}
	if output.Passed {
		t.Error("expected human judge to not pass while pending")
	}
}

func TestJudgeRegistry_GetJudges(t *testing.T) {
	registry := NewJudgeRegistry()

	tests := []struct {
		judgeType JudgeType
		exists    bool
	}{
		{JudgeAutomated, true},
		{JudgeLLM, true},
		{JudgeHuman, true},
		{"unknown", false},
	}

	for _, tt := range tests {
		judge, ok := registry.Get(tt.judgeType)
		if ok != tt.exists {
			t.Errorf("registry.Get(%s): got exists=%v, want %v", tt.judgeType, ok, tt.exists)
		}
		if tt.exists && judge == nil {
			t.Errorf("registry.Get(%s): returned nil judge", tt.judgeType)
		}
	}
}
