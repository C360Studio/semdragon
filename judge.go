package semdragons

import (
	"context"
	"sync"

	"github.com/c360studio/semstreams/model"
)

// =============================================================================
// JUDGE - Evaluators for boss battle criteria
// =============================================================================
// Judges evaluate quest output against review criteria. Each judge type
// provides a different evaluation approach:
// - Automated: Rule-based checks (format, completeness)
// - LLM: AI-powered evaluation
// - Human: Pending human review
// =============================================================================

// JudgeEvaluator evaluates quest output against a review criterion.
type JudgeEvaluator interface {
	// Evaluate runs the judge against the provided input.
	Evaluate(ctx context.Context, input JudgeInput) (*JudgeOutput, error)

	// Type returns the judge type (automated, llm, human).
	Type() JudgeType
}

// JudgeInput contains everything a judge needs to evaluate.
type JudgeInput struct {
	Judge     Judge           `json:"judge"`
	Quest     Quest           `json:"quest"`
	Output    any             `json:"output"`
	Criterion ReviewCriterion `json:"criterion"`
}

// JudgeOutput is the result of a judge evaluation.
type JudgeOutput struct {
	Score     float64 `json:"score"`     // 0.0-1.0
	Passed    bool    `json:"passed"`    // Met the criterion threshold
	Reasoning string  `json:"reasoning"` // Explanation for the score
	Pending   bool    `json:"pending"`   // True if awaiting external input (human)
}

// JudgeRegistry holds available judge implementations.
// It is safe for concurrent access.
type JudgeRegistry struct {
	mu     sync.RWMutex
	judges map[JudgeType]JudgeEvaluator
}

// NewJudgeRegistry creates a new registry with default judges.
// The LLM judge uses a stub implementation (no model registry).
// Use NewJudgeRegistryWithLLM to provide a real model registry.
func NewJudgeRegistry() *JudgeRegistry {
	return &JudgeRegistry{
		judges: map[JudgeType]JudgeEvaluator{
			JudgeAutomated: NewAutomatedJudge(),
			JudgeLLM:       NewLLMJudge(nil), // Stub mode
			JudgeHuman:     NewHumanJudge(),
		},
	}
}

// NewJudgeRegistryWithLLM creates a registry with a real LLM judge.
func NewJudgeRegistryWithLLM(registry model.RegistryReader) *JudgeRegistry {
	return &JudgeRegistry{
		judges: map[JudgeType]JudgeEvaluator{
			JudgeAutomated: NewAutomatedJudge(),
			JudgeLLM:       NewLLMJudge(registry),
			JudgeHuman:     NewHumanJudge(),
		},
	}
}

// Get returns the judge for the given type.
func (r *JudgeRegistry) Get(jt JudgeType) (JudgeEvaluator, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	j, ok := r.judges[jt]
	return j, ok
}

// Register adds or replaces a judge implementation.
func (r *JudgeRegistry) Register(jt JudgeType, j JudgeEvaluator) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.judges[jt] = j
}

// --- Checker Interface for Automated Judges ---

// Checker is a pluggable check function for automated evaluation.
type Checker interface {
	// Name returns the checker identifier (matches criterion name).
	Name() string

	// Check evaluates the output and returns a score with reasoning.
	Check(ctx context.Context, input JudgeInput) (score float64, reasoning string, err error)
}
