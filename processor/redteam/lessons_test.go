package redteam

import (
	"strings"
	"testing"

	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// parseFindings tests
// =============================================================================

func TestParseFindings_Nil(t *testing.T) {
	result := parseFindings(nil)
	if result != nil {
		t.Errorf("parseFindings(nil) = %v, want nil", result)
	}
}

func TestParseFindings_StructuredJSONArray(t *testing.T) {
	// Simulate what an LLM returns: a slice of finding maps (already decoded
	// from JSON by the agentic-loop before storing as quest output).
	output := []any{
		map[string]any{
			"skill":    "code_review",
			"category": "security",
			"summary":  "SQL injection risk in query builder",
			"detail":   "User input passed directly to SQL without sanitization",
			"severity": "critical",
			"positive": false,
		},
		map[string]any{
			"skill":    "code_generation",
			"category": "quality",
			"summary":  "Good use of interface abstractions",
			"detail":   "Interface boundaries are well-defined",
			"severity": "info",
			"positive": true,
		},
	}

	findings := parseFindings(output)
	if len(findings) != 2 {
		t.Fatalf("parseFindings returned %d findings, want 2", len(findings))
	}

	f0 := findings[0]
	if f0.Skill != domain.SkillCodeReview {
		t.Errorf("findings[0].Skill = %q, want %q", f0.Skill, domain.SkillCodeReview)
	}
	if f0.Category != domain.LessonSecurity {
		t.Errorf("findings[0].Category = %q, want %q", f0.Category, domain.LessonSecurity)
	}
	if f0.Summary != "SQL injection risk in query builder" {
		t.Errorf("findings[0].Summary = %q, want %q", f0.Summary, "SQL injection risk in query builder")
	}
	if f0.Detail != "User input passed directly to SQL without sanitization" {
		t.Errorf("findings[0].Detail = %q", f0.Detail)
	}
	if f0.Severity != domain.LessonSeverityCritical {
		t.Errorf("findings[0].Severity = %q, want %q", f0.Severity, domain.LessonSeverityCritical)
	}
	if f0.Positive {
		t.Error("findings[0].Positive = true, want false")
	}

	f1 := findings[1]
	if !f1.Positive {
		t.Error("findings[1].Positive = false, want true")
	}
	if f1.Summary != "Good use of interface abstractions" {
		t.Errorf("findings[1].Summary = %q", f1.Summary)
	}
}

func TestParseFindings_StructuredObjectWithRisksAndStrengths(t *testing.T) {
	output := map[string]any{
		"risks": []any{
			map[string]any{
				"summary":  "No input validation",
				"category": "correctness",
				"severity": "warning",
				"skill":    "code_review",
			},
		},
		"strengths": []any{
			map[string]any{
				"summary": "Well-documented API",
				"skill":   "code_review",
			},
		},
	}

	findings := parseFindings(output)
	if len(findings) != 2 {
		t.Fatalf("parseFindings returned %d findings, want 2", len(findings))
	}

	// Risks come first, then strengths.
	risk := findings[0]
	if risk.Positive {
		t.Error("risk finding should have Positive=false")
	}
	if risk.Summary != "No input validation" {
		t.Errorf("risk.Summary = %q, want %q", risk.Summary, "No input validation")
	}
	if risk.Category != domain.LessonCorrectness {
		t.Errorf("risk.Category = %q, want %q", risk.Category, domain.LessonCorrectness)
	}
	if risk.Severity != domain.LessonSeverityWarning {
		t.Errorf("risk.Severity = %q, want %q", risk.Severity, domain.LessonSeverityWarning)
	}

	strength := findings[1]
	if !strength.Positive {
		t.Error("strength finding should have Positive=true")
	}
	if strength.Summary != "Well-documented API" {
		t.Errorf("strength.Summary = %q, want %q", strength.Summary, "Well-documented API")
	}
}

func TestParseFindings_StructuredObjectRisksOnlyStrings(t *testing.T) {
	output := map[string]any{
		"risks": []any{
			"Missing error handling",
			"No test coverage",
		},
		"strengths": []any{
			"Clean architecture",
		},
	}

	findings := parseFindings(output)
	if len(findings) != 3 {
		t.Fatalf("parseFindings returned %d findings, want 3", len(findings))
	}

	if findings[0].Positive {
		t.Error("risk finding should be negative")
	}
	if findings[0].Summary != "Missing error handling" {
		t.Errorf("findings[0].Summary = %q", findings[0].Summary)
	}
	if findings[0].Severity != domain.LessonSeverityInfo {
		t.Errorf("findings[0].Severity = %q, want info (default for string entries)", findings[0].Severity)
	}

	if !findings[2].Positive {
		t.Error("strength finding should be positive")
	}
	if findings[2].Summary != "Clean architecture" {
		t.Errorf("findings[2].Summary = %q", findings[2].Summary)
	}
}

func TestParseFindings_StructuredObjectNoRisksOrStrengths(t *testing.T) {
	// A map with no "risks" or "strengths" keys: extractFromStructured finds
	// nothing, returns nil. parseFindings returns nil (the map unmarshalled
	// successfully — it is not treated as an unrecognized type).
	output := map[string]any{
		"verdict": "needs work",
		"score":   0.6,
	}

	findings := parseFindings(output)
	if len(findings) != 0 {
		t.Fatalf("parseFindings returned %d findings, want 0 (no risks/strengths keys)", len(findings))
	}
}

func TestParseFindings_PlainStringFallback(t *testing.T) {
	output := "The implementation has several issues with error handling."

	findings := parseFindings(output)
	if len(findings) != 1 {
		t.Fatalf("parseFindings returned %d findings, want 1", len(findings))
	}

	f := findings[0]
	if f.Summary != output {
		t.Errorf("finding.Summary = %q, want %q", f.Summary, output)
	}
	if f.Category != domain.LessonQuality {
		t.Errorf("finding.Category = %q, want %q", f.Category, domain.LessonQuality)
	}
	if f.Severity != domain.LessonSeverityInfo {
		t.Errorf("finding.Severity = %q, want %q", f.Severity, domain.LessonSeverityInfo)
	}
}

func TestParseFindings_DirectFindingSlice(t *testing.T) {
	// When the output is already a []finding (in-process), JSON marshal
	// round-trips it correctly.
	findings := []finding{
		{
			Skill:    domain.SkillCodeGen,
			Category: domain.LessonCorrectness,
			Summary:  "Off-by-one error in loop bounds",
			Severity: domain.LessonSeverityWarning,
			Positive: false,
		},
	}

	result := parseFindings(findings)
	if len(result) != 1 {
		t.Fatalf("parseFindings returned %d findings, want 1", len(result))
	}
	if result[0].Summary != "Off-by-one error in loop bounds" {
		t.Errorf("finding.Summary = %q", result[0].Summary)
	}
}

// =============================================================================
// normalizeFindings tests
// =============================================================================

func TestNormalizeFindings_DefaultsEmptyCategory(t *testing.T) {
	input := []finding{
		{Summary: "Something happened", Category: "", Severity: domain.LessonSeverityWarning},
	}

	result := normalizeFindings(input)
	if result[0].Category != domain.LessonQuality {
		t.Errorf("normalized Category = %q, want %q", result[0].Category, domain.LessonQuality)
	}
	// Severity already set — must not be overwritten.
	if result[0].Severity != domain.LessonSeverityWarning {
		t.Errorf("normalized Severity = %q, want %q", result[0].Severity, domain.LessonSeverityWarning)
	}
}

func TestNormalizeFindings_DefaultsEmptySeverity(t *testing.T) {
	input := []finding{
		{Summary: "Something happened", Category: domain.LessonSecurity, Severity: ""},
	}

	result := normalizeFindings(input)
	if result[0].Severity != domain.LessonSeverityInfo {
		t.Errorf("normalized Severity = %q, want %q", result[0].Severity, domain.LessonSeverityInfo)
	}
	// Category already set — must not be overwritten.
	if result[0].Category != domain.LessonSecurity {
		t.Errorf("normalized Category = %q, want %q", result[0].Category, domain.LessonSecurity)
	}
}

func TestNormalizeFindings_PreservesExistingValues(t *testing.T) {
	input := []finding{
		{
			Summary:  "Critical vuln",
			Category: domain.LessonSecurity,
			Severity: domain.LessonSeverityCritical,
		},
	}

	result := normalizeFindings(input)
	if result[0].Category != domain.LessonSecurity {
		t.Errorf("Category should not change, got %q", result[0].Category)
	}
	if result[0].Severity != domain.LessonSeverityCritical {
		t.Errorf("Severity should not change, got %q", result[0].Severity)
	}
}

func TestNormalizeFindings_DefaultsBothEmptyFields(t *testing.T) {
	input := []finding{
		{Summary: "Some observation", Category: "", Severity: ""},
	}

	result := normalizeFindings(input)
	if result[0].Category != domain.LessonQuality {
		t.Errorf("Category = %q, want %q", result[0].Category, domain.LessonQuality)
	}
	if result[0].Severity != domain.LessonSeverityInfo {
		t.Errorf("Severity = %q, want %q", result[0].Severity, domain.LessonSeverityInfo)
	}
}

func TestNormalizeFindings_MultipleFindings(t *testing.T) {
	input := []finding{
		{Summary: "Finding A", Category: "", Severity: ""},
		{Summary: "Finding B", Category: domain.LessonPerformance, Severity: domain.LessonSeverityCritical},
		{Summary: "Finding C", Category: "", Severity: domain.LessonSeverityWarning},
	}

	result := normalizeFindings(input)

	if result[0].Category != domain.LessonQuality {
		t.Errorf("result[0].Category = %q, want %q", result[0].Category, domain.LessonQuality)
	}
	if result[0].Severity != domain.LessonSeverityInfo {
		t.Errorf("result[0].Severity = %q, want %q", result[0].Severity, domain.LessonSeverityInfo)
	}

	if result[1].Category != domain.LessonPerformance {
		t.Errorf("result[1].Category should not change")
	}
	if result[1].Severity != domain.LessonSeverityCritical {
		t.Errorf("result[1].Severity should not change")
	}

	if result[2].Category != domain.LessonQuality {
		t.Errorf("result[2].Category = %q, want %q", result[2].Category, domain.LessonQuality)
	}
	if result[2].Severity != domain.LessonSeverityWarning {
		t.Errorf("result[2].Severity should not change, got %q", result[2].Severity)
	}
}

// =============================================================================
// extractSection tests
// =============================================================================

func TestExtractSection_SliceOfMaps(t *testing.T) {
	section := []any{
		map[string]any{
			"summary":  "Race condition in handler",
			"category": "correctness",
			"severity": "critical",
			"skill":    "code_review",
			"detail":   "Shared state accessed without lock",
		},
	}

	results := extractSection(section, false)
	if len(results) != 1 {
		t.Fatalf("extractSection returned %d results, want 1", len(results))
	}
	f := results[0]
	if f.Summary != "Race condition in handler" {
		t.Errorf("Summary = %q", f.Summary)
	}
	if f.Category != domain.LessonCorrectness {
		t.Errorf("Category = %q, want %q", f.Category, domain.LessonCorrectness)
	}
	if f.Severity != domain.LessonSeverityCritical {
		t.Errorf("Severity = %q, want %q", f.Severity, domain.LessonSeverityCritical)
	}
	if f.Skill != domain.SkillCodeReview {
		t.Errorf("Skill = %q, want %q", f.Skill, domain.SkillCodeReview)
	}
	if f.Detail != "Shared state accessed without lock" {
		t.Errorf("Detail = %q", f.Detail)
	}
	if f.Positive {
		t.Error("Positive should be false for risk findings")
	}
}

func TestExtractSection_SliceOfMaps_PositiveFindings(t *testing.T) {
	section := []any{
		map[string]any{
			"summary": "Excellent test coverage",
		},
	}

	results := extractSection(section, true)
	if len(results) != 1 {
		t.Fatalf("extractSection returned %d results, want 1", len(results))
	}
	if !results[0].Positive {
		t.Error("Positive should be true for strength findings")
	}
}

func TestExtractSection_SliceOfStrings(t *testing.T) {
	section := []any{
		"Missing context propagation",
		"No timeout on external calls",
		"", // empty string should be skipped
	}

	results := extractSection(section, false)
	if len(results) != 2 {
		t.Fatalf("extractSection returned %d results, want 2 (empty string skipped)", len(results))
	}
	if results[0].Summary != "Missing context propagation" {
		t.Errorf("results[0].Summary = %q", results[0].Summary)
	}
	if results[0].Severity != domain.LessonSeverityInfo {
		t.Errorf("results[0].Severity = %q, want info", results[0].Severity)
	}
	if results[0].Positive {
		t.Error("results[0].Positive should be false")
	}
	if results[1].Summary != "No timeout on external calls" {
		t.Errorf("results[1].Summary = %q", results[1].Summary)
	}
}

func TestExtractSection_PlainString(t *testing.T) {
	results := extractSection("All error paths return appropriate codes", true)
	if len(results) != 1 {
		t.Fatalf("extractSection returned %d results, want 1", len(results))
	}
	if results[0].Summary != "All error paths return appropriate codes" {
		t.Errorf("Summary = %q", results[0].Summary)
	}
	if !results[0].Positive {
		t.Error("Positive should be true")
	}
	if results[0].Severity != domain.LessonSeverityInfo {
		t.Errorf("Severity = %q, want info", results[0].Severity)
	}
}

func TestExtractSection_PlainEmptyString(t *testing.T) {
	results := extractSection("", false)
	if len(results) != 0 {
		t.Errorf("extractSection(\"\") = %v, want empty", results)
	}
}

func TestExtractSection_MapMissingSummary_Skipped(t *testing.T) {
	// A map entry without a "summary" key should be discarded.
	section := []any{
		map[string]any{
			"category": "security",
			"severity": "critical",
			// no "summary" key
		},
	}

	results := extractSection(section, false)
	if len(results) != 0 {
		t.Errorf("extractSection with missing summary = %v, want empty", results)
	}
}

func TestExtractSection_UnknownType_ReturnsNil(t *testing.T) {
	results := extractSection(42, false)
	if len(results) != 0 {
		t.Errorf("extractSection(int) = %v, want nil/empty", results)
	}
}

// =============================================================================
// singleFinding tests
// =============================================================================

func TestSingleFinding_ShortOutput(t *testing.T) {
	output := "The implementation is missing retry logic."

	findings := singleFinding(output)
	if len(findings) != 1 {
		t.Fatalf("singleFinding returned %d findings, want 1", len(findings))
	}
	if findings[0].Summary != output {
		t.Errorf("Summary = %q, want %q", findings[0].Summary, output)
	}
	if findings[0].Category != domain.LessonQuality {
		t.Errorf("Category = %q, want %q", findings[0].Category, domain.LessonQuality)
	}
	if findings[0].Severity != domain.LessonSeverityInfo {
		t.Errorf("Severity = %q, want %q", findings[0].Severity, domain.LessonSeverityInfo)
	}
}

func TestSingleFinding_TruncatesLongOutput(t *testing.T) {
	// Build a string that exceeds 200 characters.
	longOutput := strings.Repeat("x", 250)

	findings := singleFinding(longOutput)
	if len(findings) != 1 {
		t.Fatalf("singleFinding returned %d findings, want 1", len(findings))
	}

	summary := findings[0].Summary
	if len(summary) > 203 { // 200 chars + "..."
		t.Errorf("summary length = %d, want <= 203 after truncation", len(summary))
	}
	if !strings.HasSuffix(summary, "...") {
		t.Errorf("truncated summary should end with '...', got %q", summary)
	}
}

func TestSingleFinding_ExactlyAtTruncationBoundary(t *testing.T) {
	// Exactly 200 characters should not be truncated.
	output200 := strings.Repeat("a", 200)
	findings := singleFinding(output200)
	if findings[0].Summary != output200 {
		t.Errorf("200-char summary should not be truncated, got length %d", len(findings[0].Summary))
	}

	// 201 characters should be truncated.
	output201 := strings.Repeat("a", 201)
	findings201 := singleFinding(output201)
	if !strings.HasSuffix(findings201[0].Summary, "...") {
		t.Error("201-char summary should be truncated with '...'")
	}
}

// =============================================================================
// End-to-end parseFindings integration scenarios
// =============================================================================

func TestParseFindings_ParsedFindingsGetNormalizedDefaults(t *testing.T) {
	// A JSON array finding with missing category/severity should get defaults.
	output := []any{
		map[string]any{
			"summary": "Unhandled edge case",
			// no category or severity
		},
	}

	findings := parseFindings(output)
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1", len(findings))
	}
	if findings[0].Category != domain.LessonQuality {
		t.Errorf("Category = %q, want %q (default)", findings[0].Category, domain.LessonQuality)
	}
	if findings[0].Severity != domain.LessonSeverityInfo {
		t.Errorf("Severity = %q, want %q (default)", findings[0].Severity, domain.LessonSeverityInfo)
	}
}

func TestParseFindings_StructuredObjectMixedSections(t *testing.T) {
	// Object with only "strengths" (no risks) should still return results.
	output := map[string]any{
		"strengths": []any{
			map[string]any{
				"summary":  "Good encapsulation",
				"severity": "info",
			},
		},
	}

	findings := parseFindings(output)
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1", len(findings))
	}
	if !findings[0].Positive {
		t.Error("strength finding should be positive")
	}
}

func TestParseFindings_StructuredObjectEmptyRisksAndStrengths(t *testing.T) {
	// Empty arrays for both keys: extractFromStructured sees the keys but the
	// sections produce no findings, so it returns nil. parseFindings returns nil.
	output := map[string]any{
		"risks":     []any{},
		"strengths": []any{},
	}

	findings := parseFindings(output)
	if len(findings) != 0 {
		t.Fatalf("got %d findings, want 0 (empty sections produce no findings)", len(findings))
	}
}
