package promptmanager

import (
	"fmt"
	"strings"

	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// JUDGE PROMPT ASSEMBLY - LLM-as-judge prompts for boss battles
// =============================================================================

// AssembleJudgePrompt builds an LLM-as-judge prompt from review criteria.
// The judgeBase comes from the domain's JudgeSystemBase, providing domain-appropriate
// framing (e.g., "senior code reviewer" vs "Fleet Command officer").
//
// Provider-aware: XML rubric for Anthropic, markdown table for OpenAI/Ollama.
func (a *PromptAssembler) AssembleJudgePrompt(
	judgeBase string,
	criteria []domain.ReviewCriterion,
	questTitle, questDesc, provider string,
	checklist ...ChecklistItem,
) AssembledPrompt {
	return a.assembleJudgePromptWithAcceptance(judgeBase, criteria, questTitle, questDesc, provider, nil, checklist...)
}

// AssembleJudgePromptWithAcceptance builds a judge prompt that includes acceptance criteria.
// The acceptance criteria are the ground truth for what the quest must deliver — the judge
// should fail submissions that don't meet them, regardless of internal quality.
func (a *PromptAssembler) AssembleJudgePromptWithAcceptance(
	judgeBase string,
	criteria []domain.ReviewCriterion,
	questTitle, questDesc, provider string,
	acceptance []string,
	checklist ...ChecklistItem,
) AssembledPrompt {
	return a.assembleJudgePromptWithAcceptance(judgeBase, criteria, questTitle, questDesc, provider, acceptance, checklist...)
}

func (a *PromptAssembler) assembleJudgePromptWithAcceptance(
	judgeBase string,
	criteria []domain.ReviewCriterion,
	questTitle, questDesc, provider string,
	acceptance []string,
	checklist ...ChecklistItem,
) AssembledPrompt {
	style := a.registry.GetStyle(provider)

	var sections []string

	// Judge system base
	if judgeBase != "" {
		sections = append(sections, formatSection("System", judgeBase, style))
	}

	// Evaluation rubric
	if len(criteria) > 0 {
		rubric := formatRubric(criteria, style)
		sections = append(sections, formatSection("Evaluation Criteria", rubric, style))
	}

	// Structural checklist — binary pass/fail requirements
	hasChecklist := len(checklist) > 0
	if hasChecklist {
		cl := formatChecklist(checklist, style)
		sections = append(sections, formatSection("Structural Requirements", cl, style))
	}

	// Instructions — includes peer review ratings (DM reviewing the agent)
	var instructions strings.Builder
	instructions.WriteString("Evaluate the submission against each criterion. " +
		"Score each criterion from 0.0 to 1.0. " +
		"Provide specific reasoning for each score. " +
		"A criterion passes if its score meets or exceeds its threshold.\n\n")

	if hasChecklist {
		instructions.WriteString("IMPORTANT: Also check each structural requirement. " +
			"These are BINARY (pass/fail). ANY structural requirement failure is an AUTOMATIC DEFEAT " +
			"regardless of criteria scores.\n\n")
	}

	instructions.WriteString("Additionally, provide peer review ratings on a 1-5 scale for these three questions:\n" +
		"  Q1: " + domain.LeaderToMemberQuestions[0] + "\n" +
		"  Q2: " + domain.LeaderToMemberQuestions[1] + "\n" +
		"  Q3: " + domain.LeaderToMemberQuestions[2] + "\n\n" +
		"WHY HONEST RATINGS MATTER:\n" +
		"These ratings become this agent's permanent record and determine future assignments. " +
		"If you inflate scores, underperforming agents get trusted with harder work — and when " +
		"they fail, it costs everyone. Honest reviews help agents learn from mistakes. " +
		"Dishonest reviews guarantee they repeat them.\n\n" +
		"RATING CALIBRATION:\n" +
		"  1 = Unacceptable — fundamentally wrong, missing, or unusable\n" +
		"  2 = Below expectations — significant gaps, errors, or missing requirements\n" +
		"  3 = Meets expectations — correct, complete, does what was asked (this is the baseline for competent work)\n" +
		"  4 = Exceeds expectations — well-structured, thorough, handles edge cases\n" +
		"  5 = Exceptional — production-quality, elegant, rare (most good work is a 3 or 4, not a 5)\n" +
		"Rate honestly. A 3 for solid work is correct — not a 5.\n\n")

	instructions.WriteString("Respond with ONLY a JSON object in this exact format:\n" +
		"```json\n" +
		"{\"criteria\": [{\"name\": \"<criterion_name>\", \"score\": <0.0-1.0>, \"reasoning\": \"<explanation>\"}], ")
	if hasChecklist {
		instructions.WriteString("\"checklist\": [{\"name\": \"<item_name>\", \"passed\": true/false, \"reasoning\": \"<explanation>\"}], ")
	}
	instructions.WriteString("\"overall_feedback\": \"<summary>\", " +
		"\"peer_review\": {\"q1\": <1-5>, \"q2\": <1-5>, \"q3\": <1-5>}}\n" +
		"```")

	sections = append(sections, formatSection("Instructions", instructions.String(), style))

	// Quest context for judge
	var questParts []string
	if questTitle != "" {
		questParts = append(questParts, fmt.Sprintf("Title: %s", questTitle))
	}
	if questDesc != "" {
		questParts = append(questParts, fmt.Sprintf("Description: %s", questDesc))
	}
	if len(acceptance) > 0 {
		questParts = append(questParts, "Acceptance Criteria (the submission MUST meet ALL of these):")
		for _, a := range acceptance {
			questParts = append(questParts, fmt.Sprintf("  - %s", a))
		}
		questParts = append(questParts, "FAIL the submission if any acceptance criterion is not met, regardless of other quality scores.")
	}
	if len(questParts) > 0 {
		sections = append(sections, formatSection("Quest", strings.Join(questParts, "\n"), style))
	}

	return AssembledPrompt{
		SystemMessage: strings.Join(sections, "\n\n"),
		FragmentsUsed: []string{"judge-system-base", "judge-rubric", "judge-instructions"},
	}
}

// formatRubric formats review criteria as a structured rubric.
func formatRubric(criteria []domain.ReviewCriterion, style ProviderStyle) string {
	if style.PreferXML {
		return formatRubricXML(criteria)
	}
	if style.PreferMarkdown {
		return formatRubricMarkdown(criteria)
	}
	return formatRubricPlain(criteria)
}

// formatRubricXML formats criteria as XML elements (preferred by Anthropic).
func formatRubricXML(criteria []domain.ReviewCriterion) string {
	var b strings.Builder
	for _, c := range criteria {
		fmt.Fprintf(&b, "<criterion>\n")
		fmt.Fprintf(&b, "  <name>%s</name>\n", c.Name)
		fmt.Fprintf(&b, "  <description>%s</description>\n", c.Description)
		fmt.Fprintf(&b, "  <weight>%.2f</weight>\n", c.Weight)
		fmt.Fprintf(&b, "  <threshold>%.2f</threshold>\n", c.Threshold)
		fmt.Fprintf(&b, "</criterion>\n")
	}
	return b.String()
}

// formatRubricMarkdown formats criteria as a markdown table (preferred by OpenAI/Ollama).
func formatRubricMarkdown(criteria []domain.ReviewCriterion) string {
	var b strings.Builder
	b.WriteString("| Criterion | Description | Weight | Threshold |\n")
	b.WriteString("|-----------|-------------|--------|-----------|\n")
	for _, c := range criteria {
		fmt.Fprintf(&b, "| %s | %s | %.2f | %.2f |\n", c.Name, c.Description, c.Weight, c.Threshold)
	}
	return b.String()
}

// formatRubricPlain formats criteria as a simple list.
func formatRubricPlain(criteria []domain.ReviewCriterion) string {
	var b strings.Builder
	for i, c := range criteria {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "- %s (weight: %.2f, threshold: %.2f): %s", c.Name, c.Weight, c.Threshold, c.Description)
	}
	return b.String()
}

// =============================================================================
// STRUCTURAL CHECKLIST FORMATTING
// =============================================================================

// formatChecklist formats checklist items using provider-appropriate style.
func formatChecklist(items []ChecklistItem, style ProviderStyle) string {
	if style.PreferXML {
		return formatChecklistXML(items)
	}
	if style.PreferMarkdown {
		return formatChecklistMarkdown(items)
	}
	return formatChecklistPlain(items)
}

func formatChecklistXML(items []ChecklistItem) string {
	var b strings.Builder
	b.WriteString("Each requirement MUST pass. Any failure = automatic defeat.\n")
	for _, item := range items {
		fmt.Fprintf(&b, "<requirement>\n  <name>%s</name>\n  <check>%s</check>\n</requirement>\n", item.Name, item.Requirement)
	}
	return b.String()
}

func formatChecklistMarkdown(items []ChecklistItem) string {
	var b strings.Builder
	b.WriteString("Each requirement MUST pass. Any failure = automatic defeat.\n\n")
	b.WriteString("| Requirement | Check |\n")
	b.WriteString("|-------------|-------|\n")
	for _, item := range items {
		fmt.Fprintf(&b, "| %s | %s |\n", item.Name, item.Requirement)
	}
	return b.String()
}

func formatChecklistPlain(items []ChecklistItem) string {
	var b strings.Builder
	b.WriteString("Each requirement MUST pass. Any failure = automatic defeat.\n")
	for _, item := range items {
		fmt.Fprintf(&b, "- %s: %s\n", item.Name, item.Requirement)
	}
	return b.String()
}
