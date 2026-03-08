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

	// Instructions — includes peer review ratings (DM reviewing the agent)
	instructions := "Evaluate the submission against each criterion. " +
		"Score each criterion from 0.0 to 1.0. " +
		"Provide specific reasoning for each score. " +
		"A criterion passes if its score meets or exceeds its threshold.\n\n" +
		"Additionally, provide peer review ratings on a 1-5 scale for these three questions:\n" +
		"  Q1: " + domain.LeaderToMemberQuestions[0] + "\n" +
		"  Q2: " + domain.LeaderToMemberQuestions[1] + "\n" +
		"  Q3: " + domain.LeaderToMemberQuestions[2] + "\n\n" +
		"Respond with ONLY a JSON object in this exact format:\n" +
		"```json\n" +
		"{\"criteria\": [{\"name\": \"<criterion_name>\", \"score\": <0.0-1.0>, \"reasoning\": \"<explanation>\"}], " +
		"\"overall_feedback\": \"<summary>\", " +
		"\"peer_review\": {\"q1\": <1-5>, \"q2\": <1-5>, \"q3\": <1-5>}}\n" +
		"```"
	sections = append(sections, formatSection("Instructions", instructions, style))

	// Quest context for judge
	var questParts []string
	if questTitle != "" {
		questParts = append(questParts, fmt.Sprintf("Title: %s", questTitle))
	}
	if questDesc != "" {
		questParts = append(questParts, fmt.Sprintf("Description: %s", questDesc))
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
