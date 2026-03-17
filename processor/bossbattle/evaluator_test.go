package bossbattle

import (
	"strings"
	"testing"
)

// =============================================================================
// formatCombinedOutput TESTS
// =============================================================================

func TestFormatCombinedOutput_BothPresent(t *testing.T) {
	output := formatCombinedOutput("the implementation", "the red team found issues")

	if !strings.Contains(output, "## Quest Output") {
		t.Error("output should contain '## Quest Output' section")
	}
	if !strings.Contains(output, "the implementation") {
		t.Error("output should contain the quest output text")
	}
	if !strings.Contains(output, "## Red-Team Review Findings") {
		t.Error("output should contain '## Red-Team Review Findings' section")
	}
	if !strings.Contains(output, "the red team found issues") {
		t.Error("output should contain the red-team findings text")
	}
	// Separator between the two sections
	if !strings.Contains(output, "---") {
		t.Error("output should contain a separator between sections")
	}
	// Judge guidance preamble
	if !strings.Contains(output, "apply your own judgment") {
		t.Error("output should contain judge guidance preamble")
	}
}

func TestFormatCombinedOutput_QuestOutputOnly(t *testing.T) {
	// When there are no red-team findings the caller won't call formatCombinedOutput
	// directly; but if it does, the function should still render both sections cleanly.
	output := formatCombinedOutput("quest result here", "")

	if !strings.Contains(output, "## Quest Output") {
		t.Error("output should contain '## Quest Output' section")
	}
	if !strings.Contains(output, "quest result here") {
		t.Error("output should contain the quest output text")
	}
	if !strings.Contains(output, "## Red-Team Review Findings") {
		t.Error("output should contain the red-team section header even when empty")
	}
}

func TestFormatCombinedOutput_RedTeamAsString(t *testing.T) {
	rtFindings := "Critical vulnerability found in auth flow."
	output := formatCombinedOutput("some output", rtFindings)

	if !strings.Contains(output, rtFindings) {
		t.Errorf("output should contain the string findings %q", rtFindings)
	}
	// String findings must NOT be JSON-wrapped
	if strings.Contains(output, "```json") {
		t.Error("string red-team findings should not be JSON-wrapped")
	}
}

func TestFormatCombinedOutput_RedTeamAsJSONObject(t *testing.T) {
	rtFindings := map[string]any{
		"severity": "high",
		"issues":   []string{"SQL injection", "XSS"},
	}
	output := formatCombinedOutput("some output", rtFindings)

	// JSON object findings must be rendered in a code block
	if !strings.Contains(output, "```json") {
		t.Error("map red-team findings should be rendered as a JSON code block")
	}
	if !strings.Contains(output, "severity") {
		t.Error("output should contain the JSON key 'severity'")
	}
}

func TestFormatCombinedOutput_NilQuestOutputWithRedTeam(t *testing.T) {
	output := formatCombinedOutput(nil, "red team said something")

	if !strings.Contains(output, "## Quest Output") {
		t.Error("output should contain '## Quest Output' section")
	}
	if !strings.Contains(output, "## Red-Team Review Findings") {
		t.Error("output should contain '## Red-Team Review Findings' section")
	}
	if !strings.Contains(output, "red team said something") {
		t.Error("output should contain the red-team text")
	}
}

// =============================================================================
// formatOutputForJudge — combined output dispatch TESTS
// =============================================================================

func TestFormatOutputForJudge_NilOutput(t *testing.T) {
	result := formatOutputForJudge(nil)
	if result != "No output was provided." {
		t.Errorf("got %q, want %q", result, "No output was provided.")
	}
}

func TestFormatOutputForJudge_StringOutput(t *testing.T) {
	result := formatOutputForJudge("plain result")
	if !strings.Contains(result, "## Quest Output") {
		t.Error("should contain '## Quest Output' section")
	}
	if !strings.Contains(result, "plain result") {
		t.Error("should contain the original string")
	}
}

func TestFormatOutputForJudge_CombinedMapDispatch(t *testing.T) {
	// A map with both "quest_output" and "red_team_findings" must trigger
	// formatCombinedOutput, which produces the two-section format.
	combined := map[string]any{
		"quest_output":      "the work",
		"red_team_findings": "the review",
	}
	result := formatOutputForJudge(combined)

	if !strings.Contains(result, "## Quest Output") {
		t.Error("combined map should produce '## Quest Output' section")
	}
	if !strings.Contains(result, "## Red-Team Review Findings") {
		t.Error("combined map should produce '## Red-Team Review Findings' section")
	}
}

func TestFormatOutputForJudge_MapWithoutRedTeam(t *testing.T) {
	// A map that doesn't have the red_team_findings key falls through to JSON
	// marshalling — it should NOT be treated as a combined output.
	plain := map[string]any{
		"result": "some value",
	}
	result := formatOutputForJudge(plain)

	if strings.Contains(result, "## Red-Team Review Findings") {
		t.Error("plain map should not produce a red-team section")
	}
	if !strings.Contains(result, "## Quest Output") {
		t.Error("plain map should still produce a quest output section")
	}
}
