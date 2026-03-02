package promptmanager

import (
	"strings"
	"testing"

	"github.com/c360studio/semdragons/domain"
)

func testCriteria() []domain.ReviewCriterion {
	return []domain.ReviewCriterion{
		{Name: "Correctness", Description: "Output is correct", Weight: 0.4, Threshold: 0.7},
		{Name: "Quality", Description: "Code quality meets standards", Weight: 0.3, Threshold: 0.6},
		{Name: "Completeness", Description: "All requirements addressed", Weight: 0.3, Threshold: 0.5},
	}
}

func TestAssembleJudgePrompt_Basic(t *testing.T) {
	assembler, _ := newTestAssembler()

	result := assembler.AssembleJudgePrompt(
		"You are a senior code reviewer.",
		testCriteria(),
		"Fix the login bug",
		"Users can't log in",
		"",
	)

	if result.SystemMessage == "" {
		t.Fatal("expected non-empty system message")
	}
	if !strings.Contains(result.SystemMessage, "senior code reviewer") {
		t.Error("expected judge base in output")
	}
	if !strings.Contains(result.SystemMessage, "Correctness") {
		t.Error("expected criterion name in output")
	}
	if !strings.Contains(result.SystemMessage, "Fix the login bug") {
		t.Error("expected quest title in output")
	}
}

func TestAssembleJudgePrompt_AnthropicXMLRubric(t *testing.T) {
	assembler, _ := newTestAssembler()

	result := assembler.AssembleJudgePrompt(
		"You are a reviewer.",
		testCriteria(),
		"Test", "Test desc",
		"anthropic",
	)

	if !strings.Contains(result.SystemMessage, "<criterion>") {
		t.Error("expected XML criterion tags for Anthropic")
	}
	if !strings.Contains(result.SystemMessage, "<name>Correctness</name>") {
		t.Error("expected XML name element")
	}
	if !strings.Contains(result.SystemMessage, "<weight>0.40</weight>") {
		t.Error("expected XML weight element")
	}
}

func TestAssembleJudgePrompt_OpenAIMarkdownRubric(t *testing.T) {
	assembler, _ := newTestAssembler()

	result := assembler.AssembleJudgePrompt(
		"You are a reviewer.",
		testCriteria(),
		"Test", "Test desc",
		"openai",
	)

	if !strings.Contains(result.SystemMessage, "| Criterion |") {
		t.Error("expected markdown table header for OpenAI")
	}
	if !strings.Contains(result.SystemMessage, "| Correctness |") {
		t.Error("expected criterion row in markdown table")
	}
}

func TestAssembleJudgePrompt_DefaultPlainRubric(t *testing.T) {
	assembler, _ := newTestAssembler()

	result := assembler.AssembleJudgePrompt(
		"You are a reviewer.",
		testCriteria(),
		"Test", "Test desc",
		"custom",
	)

	if !strings.Contains(result.SystemMessage, "- Correctness (weight: 0.40") {
		t.Error("expected plain rubric format")
	}
}

func TestAssembleJudgePrompt_NoCriteria(t *testing.T) {
	assembler, _ := newTestAssembler()

	result := assembler.AssembleJudgePrompt(
		"You are a reviewer.",
		nil,
		"Test", "Test desc",
		"",
	)

	// Should still have base and instructions, just no rubric
	if !strings.Contains(result.SystemMessage, "You are a reviewer.") {
		t.Error("expected judge base even without criteria")
	}
	if !strings.Contains(result.SystemMessage, "Evaluate the submission") {
		t.Error("expected instructions even without criteria")
	}
}

func TestAssembleJudgePrompt_EmptyBase(t *testing.T) {
	assembler, _ := newTestAssembler()

	result := assembler.AssembleJudgePrompt(
		"",
		testCriteria(),
		"Test", "Test desc",
		"",
	)

	// Should have rubric and instructions but no system section
	if strings.Contains(result.SystemMessage, "System:") && !strings.Contains(result.SystemMessage, "Evaluation") {
		t.Error("should not have system section when base is empty")
	}
	if !strings.Contains(result.SystemMessage, "Correctness") {
		t.Error("expected criteria even without judge base")
	}
}

func TestAssembleJudgePrompt_DomainSpecificJudge(t *testing.T) {
	// Verify different domains produce different judge framing
	tests := []struct {
		name     string
		base     string
		contains string
	}{
		{
			name:     "software domain",
			base:     "You are a senior code reviewer evaluating a developer's work output.",
			contains: "code reviewer",
		},
		{
			name:     "dnd domain",
			base:     "You are an ancient sage evaluating an adventurer's quest performance.",
			contains: "ancient sage",
		},
		{
			name:     "research domain",
			base:     "You are a peer reviewer evaluating a researcher's study output.",
			contains: "peer reviewer",
		},
	}

	assembler, _ := newTestAssembler()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := assembler.AssembleJudgePrompt(
				tt.base,
				testCriteria(),
				"Test Quest",
				"Test Description",
				"",
			)

			if !strings.Contains(result.SystemMessage, tt.contains) {
				t.Errorf("expected %q in judge prompt", tt.contains)
			}
		})
	}
}

func TestAssembleJudgePrompt_FragmentsUsed(t *testing.T) {
	assembler, _ := newTestAssembler()

	result := assembler.AssembleJudgePrompt(
		"You are a reviewer.",
		testCriteria(),
		"Test", "Test desc",
		"",
	)

	if len(result.FragmentsUsed) == 0 {
		t.Error("expected non-empty FragmentsUsed for observability")
	}
}
