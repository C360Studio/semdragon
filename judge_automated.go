package semdragons

import (
	"context"
	"fmt"
	"reflect"
)

// =============================================================================
// AUTOMATED JUDGE - Rule-based evaluation checks
// =============================================================================
// The automated judge runs deterministic checks against quest output.
// Built-in checkers:
// - format: Output is non-nil and has expected structure
// - completeness: Required fields are present
// - non_empty: Meaningful content exists
// =============================================================================

// AutomatedJudge evaluates output using rule-based checks.
type AutomatedJudge struct {
	checkers map[string]Checker
}

// NewAutomatedJudge creates an automated judge with default checkers.
func NewAutomatedJudge() *AutomatedJudge {
	j := &AutomatedJudge{
		checkers: make(map[string]Checker),
	}

	// Register built-in checkers
	j.RegisterChecker(&FormatChecker{})
	j.RegisterChecker(&CompletenessChecker{})
	j.RegisterChecker(&NonEmptyChecker{})

	return j
}

// Type returns the judge type.
func (j *AutomatedJudge) Type() JudgeType {
	return JudgeAutomated
}

// RegisterChecker adds a custom checker.
func (j *AutomatedJudge) RegisterChecker(c Checker) {
	j.checkers[c.Name()] = c
}

// Evaluate runs the appropriate checker for the criterion.
func (j *AutomatedJudge) Evaluate(ctx context.Context, input JudgeInput) (*JudgeOutput, error) {
	checker, ok := j.checkers[input.Criterion.Name]
	if !ok {
		// No specific checker - use generic format check
		checker = &FormatChecker{}
	}

	score, reasoning, err := checker.Check(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("checker %s failed: %w", input.Criterion.Name, err)
	}

	return &JudgeOutput{
		Score:     score,
		Passed:    score >= input.Criterion.Threshold,
		Reasoning: reasoning,
		Pending:   false,
	}, nil
}

// =============================================================================
// BUILT-IN CHECKERS
// =============================================================================

// FormatChecker validates that output exists and has expected structure.
type FormatChecker struct{}

func (c *FormatChecker) Name() string { return "format" }

func (c *FormatChecker) Check(_ context.Context, input JudgeInput) (float64, string, error) {
	if input.Output == nil {
		return 0.0, "Output is nil", nil
	}

	// Check for zero values using reflection
	v := reflect.ValueOf(input.Output)
	if !v.IsValid() {
		return 0.0, "Output is invalid", nil
	}

	// Handle pointers
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return 0.0, "Output pointer is nil", nil
		}
		v = v.Elem()
	}

	// Check common zero cases
	switch v.Kind() {
	case reflect.String:
		if v.String() == "" {
			return 0.5, "Output is empty string", nil
		}
	case reflect.Map:
		if v.Len() == 0 {
			return 0.5, "Output is empty map", nil
		}
	case reflect.Slice, reflect.Array:
		if v.Len() == 0 {
			return 0.5, "Output is empty slice/array", nil
		}
	}

	return 1.0, "Output has valid format", nil
}

// CompletenessChecker validates that required fields are present.
type CompletenessChecker struct{}

func (c *CompletenessChecker) Name() string { return "completeness" }

func (c *CompletenessChecker) Check(_ context.Context, input JudgeInput) (float64, string, error) {
	if input.Output == nil {
		return 0.0, "Output is nil - cannot check completeness", nil
	}

	v := reflect.ValueOf(input.Output)
	if !v.IsValid() {
		return 0.0, "Output is invalid", nil
	}

	// Handle pointers
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return 0.0, "Output pointer is nil", nil
		}
		v = v.Elem()
	}

	// For structs, check that exported fields have non-zero values
	if v.Kind() == reflect.Struct {
		totalFields := 0
		nonZeroFields := 0

		for i := range v.NumField() {
			field := v.Field(i)
			if !field.CanInterface() {
				continue // Skip unexported fields
			}
			totalFields++
			if !field.IsZero() {
				nonZeroFields++
			}
		}

		if totalFields == 0 {
			return 1.0, "No exported fields to check", nil
		}

		score := float64(nonZeroFields) / float64(totalFields)
		return score, fmt.Sprintf("%d/%d fields populated", nonZeroFields, totalFields), nil
	}

	// For maps, check that expected keys exist
	if v.Kind() == reflect.Map {
		if v.Len() == 0 {
			return 0.0, "Output map is empty", nil
		}
		return 1.0, fmt.Sprintf("Output map has %d entries", v.Len()), nil
	}

	// For other types, just check non-zero
	if v.IsZero() {
		return 0.0, "Output is zero value", nil
	}

	return 1.0, "Output is complete", nil
}

// NonEmptyChecker validates that output contains meaningful content.
type NonEmptyChecker struct{}

func (c *NonEmptyChecker) Name() string { return "non_empty" }

func (c *NonEmptyChecker) Check(_ context.Context, input JudgeInput) (float64, string, error) {
	if input.Output == nil {
		return 0.0, "Output is nil", nil
	}

	v := reflect.ValueOf(input.Output)
	if !v.IsValid() {
		return 0.0, "Output is invalid", nil
	}

	// Handle pointers
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return 0.0, "Output pointer is nil", nil
		}
		v = v.Elem()
	}

	// Check for meaningful content based on type
	switch v.Kind() {
	case reflect.String:
		s := v.String()
		if s == "" {
			return 0.0, "Output string is empty", nil
		}
		if len(s) < 10 {
			return 0.5, "Output string is very short", nil
		}
		return 1.0, fmt.Sprintf("Output string has %d characters", len(s)), nil

	case reflect.Map:
		if v.Len() == 0 {
			return 0.0, "Output map is empty", nil
		}
		return 1.0, fmt.Sprintf("Output map has %d entries", v.Len()), nil

	case reflect.Slice, reflect.Array:
		if v.Len() == 0 {
			return 0.0, "Output slice/array is empty", nil
		}
		return 1.0, fmt.Sprintf("Output has %d elements", v.Len()), nil

	case reflect.Struct:
		// Count non-zero fields
		nonZero := 0
		for i := range v.NumField() {
			field := v.Field(i)
			if field.CanInterface() && !field.IsZero() {
				nonZero++
			}
		}
		if nonZero == 0 {
			return 0.0, "All struct fields are zero values", nil
		}
		return 1.0, fmt.Sprintf("Struct has %d non-zero fields", nonZero), nil
	}

	// For other types, just check non-zero
	if v.IsZero() {
		return 0.0, "Output is zero value", nil
	}

	return 1.0, "Output contains content", nil
}

// =============================================================================
// GENERIC CRITERIA CHECKERS
// =============================================================================

// CorrectnessChecker provides a basic correctness check.
// In practice, this would need domain-specific logic or LLM evaluation.
type CorrectnessChecker struct{}

func (c *CorrectnessChecker) Name() string { return "correctness" }

func (c *CorrectnessChecker) Check(_ context.Context, input JudgeInput) (float64, string, error) {
	// Automated correctness checking is limited
	// We can only verify format/structure, not semantic correctness
	// This is why LLM judges are used for correctness at higher review levels

	if input.Output == nil {
		return 0.0, "Cannot verify correctness: output is nil", nil
	}

	// Basic check: output exists and is non-zero
	v := reflect.ValueOf(input.Output)
	if !v.IsValid() || (v.Kind() == reflect.Ptr && v.IsNil()) {
		return 0.0, "Cannot verify correctness: invalid output", nil
	}

	// Automated check can only confirm format, not correctness
	return 0.7, "Format valid - semantic correctness requires LLM evaluation", nil
}

// QualityChecker provides a basic quality check.
type QualityChecker struct{}

func (c *QualityChecker) Name() string { return "quality" }

func (c *QualityChecker) Check(_ context.Context, input JudgeInput) (float64, string, error) {
	// Quality is subjective - automated check can only look at basic metrics
	if input.Output == nil {
		return 0.0, "Cannot assess quality: output is nil", nil
	}

	// For strings, check length as a proxy for thoroughness
	v := reflect.ValueOf(input.Output)
	if v.Kind() == reflect.String {
		s := v.String()
		switch {
		case len(s) == 0:
			return 0.0, "Empty output", nil
		case len(s) < 50:
			return 0.4, "Very brief output", nil
		case len(s) < 200:
			return 0.6, "Short output", nil
		case len(s) < 500:
			return 0.8, "Moderate length output", nil
		default:
			return 0.9, "Detailed output", nil
		}
	}

	return 0.7, "Quality assessment requires LLM evaluation", nil
}

// RobustnessChecker provides a basic robustness check.
type RobustnessChecker struct{}

func (c *RobustnessChecker) Name() string { return "robustness" }

func (c *RobustnessChecker) Check(_ context.Context, input JudgeInput) (float64, string, error) {
	// Robustness (edge case handling) is difficult to check automatically
	// This would require test cases or LLM evaluation

	if input.Output == nil {
		return 0.0, "Cannot assess robustness: output is nil", nil
	}

	return 0.7, "Robustness assessment requires LLM evaluation", nil
}

// CreativityChecker provides a basic creativity check.
type CreativityChecker struct{}

func (c *CreativityChecker) Name() string { return "creativity" }

func (c *CreativityChecker) Check(_ context.Context, input JudgeInput) (float64, string, error) {
	// Creativity is highly subjective - automated check can't assess this
	if input.Output == nil {
		return 0.0, "Cannot assess creativity: output is nil", nil
	}

	return 0.6, "Creativity assessment requires human evaluation", nil
}
