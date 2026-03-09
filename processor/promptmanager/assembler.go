package promptmanager

import (
	"fmt"
	"strings"
)

// =============================================================================
// PROMPT ASSEMBLER - Fragment composition with provider-aware formatting
// =============================================================================

// PromptAssembler composes prompt fragments into system and user messages.
type PromptAssembler struct {
	registry *PromptRegistry
}

// NewPromptAssembler creates a new assembler backed by the given registry.
func NewPromptAssembler(registry *PromptRegistry) *PromptAssembler {
	return &PromptAssembler{registry: registry}
}

// AssembleSystemPrompt composes fragments into a system prompt.
//
// Assembly order (by category):
//
//	SystemBase → ToolDirective → ProviderHints → TierGuardrails → SkillContext →
//	GuildKnowledge → [AgentConfig.SystemPrompt] → [Persona] → QuestContext
//
// Agent-level overrides (SystemPrompt, PersonaPrompt) are appended after domain
// fragments — they're the "last word" for per-agent customization.
func (a *PromptAssembler) AssembleSystemPrompt(ctx AssemblyContext) AssembledPrompt {
	fragments := a.registry.GetFragmentsForContext(ctx)
	style := a.registry.GetStyle(ctx.Provider)

	var sections []string
	var usedIDs []string

	// Group fragments by category for formatting
	groups := groupByCategory(fragments)

	// Emit each category group with appropriate formatting
	for _, cat := range sortedCategories(groups) {
		frags := groups[cat]
		label := categoryLabel(cat)

		var content strings.Builder
		for _, f := range frags {
			if content.Len() > 0 {
				content.WriteByte('\n')
			}
			content.WriteString(f.Content)
			usedIDs = append(usedIDs, f.ID)
		}

		sections = append(sections, formatSection(label, content.String(), style))
	}

	// Inject peer feedback warnings after registry fragments and before agent overrides.
	// Low ratings are surfaced as explicit directives so the agent corrects the
	// behaviors that peers flagged — not as soft suggestions.
	if len(ctx.PeerFeedback) > 0 {
		var warnings strings.Builder
		warnings.WriteString("In recent tasks, your peers rated you low on the following. You MUST address these:\n")
		for _, fb := range ctx.PeerFeedback {
			warnings.WriteString(fmt.Sprintf("- %s (%.1f/5.0)", fb.Question, fb.AvgRating))
			if fb.Explanation != "" {
				warnings.WriteString(fmt.Sprintf(": %s", fb.Explanation))
			}
			warnings.WriteByte('\n')
		}
		sections = append(sections, formatSection("Peer Feedback", warnings.String(), style))
		usedIDs = append(usedIDs, "peer-feedback-warnings")
	}

	// Inject dependency outputs from completed predecessor DAG nodes.
	// These appear before clarifications and agent overrides so the agent
	// knows what predecessor steps produced.
	if len(ctx.DependencyOutputs) > 0 {
		var deps strings.Builder
		deps.WriteString("The following predecessor tasks have been completed. Use their outputs as context for your work:\n")
		for _, dep := range ctx.DependencyOutputs {
			deps.WriteString(fmt.Sprintf("\n--- %s: %s ---\n%s\n", dep.NodeID, dep.Objective, dep.Output))
		}
		sections = append(sections, formatSection("Dependency Outputs", deps.String(), style))
		usedIDs = append(usedIDs, "dependency-outputs")
	}

	// Inject structural checklist so agents self-check before submitting.
	if len(ctx.StructuralChecklist) > 0 {
		style := a.registry.GetStyle(ctx.Provider)
		var reqs strings.Builder
		reqs.WriteString("Your submission will be checked against these MANDATORY requirements. " +
			"Any failure is an automatic review defeat. Self-check before submitting:\n")
		for _, item := range ctx.StructuralChecklist {
			reqs.WriteString(fmt.Sprintf("- %s: %s\n", item.Name, item.Requirement))
		}
		sections = append(sections, formatSection("Structural Requirements", reqs.String(), style))
		usedIDs = append(usedIDs, "structural-checklist")
	}

	// Inject clarification answers from previous interactions (party lead or DM).
	// These appear before agent overrides so the agent has context for its retry.
	if len(ctx.ClarificationAnswers) > 0 {
		var clarifications strings.Builder
		source := ctx.ClarificationSource
		if source == "" {
			source = "The DM"
		}
		clarifications.WriteString(source + " answered your previous questions:\n")
		for i, ca := range ctx.ClarificationAnswers {
			clarifications.WriteString(fmt.Sprintf("\nQ%d: %s\nA%d: %s\n", i+1, ca.Question, i+1, ca.Answer))
		}
		clarifications.WriteString("\nUse these answers to complete your task. Do NOT ask the same questions again.")
		sections = append(sections, formatSection("Previous Clarifications", clarifications.String(), style))
		usedIDs = append(usedIDs, "clarification-answers")
	}

	// Append agent-level overrides (not from registry — per-agent customization)
	if ctx.SystemPrompt != "" {
		sections = append(sections, formatSection("Agent Configuration", ctx.SystemPrompt, style))
	}
	if ctx.PersonaPrompt != "" {
		sections = append(sections, formatSection("Persona", ctx.PersonaPrompt, style))
	}

	// Quest context is always last before the response format instruction.
	if questCtx := buildQuestContext(ctx); questCtx != "" {
		sections = append(sections, formatSection("Quest", questCtx, style))
	}

	// Response format: instruct the LLM to self-classify its output intent.
	// This enables downstream routing (work product → boss battle review,
	// clarification → DM escalation) without fragile heuristics.
	sections = append(sections, formatSection("Response Format", responseFormatInstruction, style))
	usedIDs = append(usedIDs, "response-format-intent")

	return AssembledPrompt{
		SystemMessage: strings.Join(sections, "\n\n"),
		UserMessage:   buildUserMessage(ctx),
		FragmentsUsed: usedIDs,
	}
}

// =============================================================================
// RESPONSE FORMAT INSTRUCTION
// =============================================================================

// responseFormatInstruction is injected into every assembled system prompt.
// It instructs the LLM to self-classify its output so questbridge can route
// work products to review and clarification requests to the DM.
const responseFormatInstruction = `Begin every response with exactly one intent declaration on its own line:

[INTENT: work_product] — You are delivering a completed result for the quest.
[INTENT: clarification] — You need more information from the quest issuer before you can proceed.

After the intent line, provide your response content. Do not omit the intent line.`

// =============================================================================
// INTERNAL HELPERS
// =============================================================================

// groupByCategory groups fragments into a map keyed by category.
func groupByCategory(fragments []*PromptFragment) map[FragmentCategory][]*PromptFragment {
	groups := make(map[FragmentCategory][]*PromptFragment)
	for _, f := range fragments {
		groups[f.Category] = append(groups[f.Category], f)
	}
	return groups
}

// sortedCategories returns category keys in ascending order.
func sortedCategories(groups map[FragmentCategory][]*PromptFragment) []FragmentCategory {
	cats := make([]FragmentCategory, 0, len(groups))
	for cat := range groups {
		cats = append(cats, cat)
	}
	// Categories are ints — natural sort is ascending
	for i := 1; i < len(cats); i++ {
		for j := i; j > 0 && cats[j] < cats[j-1]; j-- {
			cats[j], cats[j-1] = cats[j-1], cats[j]
		}
	}
	return cats
}

// categoryLabel returns a human-readable label for a fragment category.
func categoryLabel(cat FragmentCategory) string {
	switch cat {
	case CategorySystemBase:
		return "System"
	case CategoryToolDirective:
		return "Tool Directive"
	case CategoryProviderHints:
		return "Provider"
	case CategoryTierGuardrails:
		return "Tier Guardrails"
	case CategoryPeerFeedback:
		return "Peer Feedback"
	case CategorySkillContext:
		return "Skills"
	case CategoryGuildKnowledge:
		return "Guild Knowledge"
	case CategoryPersona:
		return "Persona"
	case CategoryQuestContext:
		return "Quest"
	default:
		return "Context"
	}
}

// formatSection wraps content with provider-appropriate delimiters.
func formatSection(label, content string, style ProviderStyle) string {
	if style.PreferXML {
		tag := strings.ReplaceAll(strings.ToLower(label), " ", "_")
		return fmt.Sprintf("<%s>\n%s\n</%s>", tag, content, tag)
	}
	if style.PreferMarkdown {
		return fmt.Sprintf("## %s\n%s", label, content)
	}
	// Default: simple labeled section
	return fmt.Sprintf("%s:\n%s", label, content)
}

// buildQuestContext constructs the quest context section.
func buildQuestContext(ctx AssemblyContext) string {
	if ctx.QuestTitle == "" && ctx.QuestDescription == "" {
		return ""
	}

	var parts []string
	if ctx.QuestTitle != "" {
		parts = append(parts, fmt.Sprintf("Title: %s", ctx.QuestTitle))
	}
	if ctx.QuestDescription != "" {
		parts = append(parts, fmt.Sprintf("Description: %s", ctx.QuestDescription))
	}
	if ctx.MaxDuration != "" {
		parts = append(parts, fmt.Sprintf("Time limit: %s", ctx.MaxDuration))
	}
	if ctx.MaxTokens > 0 {
		parts = append(parts, fmt.Sprintf("Token budget: %d", ctx.MaxTokens))
	}
	if len(ctx.RequiredSkills) > 0 {
		skills := make([]string, len(ctx.RequiredSkills))
		for i, s := range ctx.RequiredSkills {
			skills[i] = string(s)
		}
		parts = append(parts, fmt.Sprintf("Required skills: %s", strings.Join(skills, ", ")))
	}
	return strings.Join(parts, "\n")
}

// buildUserMessage constructs the user message from quest input.
func buildUserMessage(ctx AssemblyContext) string {
	if ctx.QuestInput == nil {
		return ctx.QuestDescription
	}

	if s, ok := ctx.QuestInput.(string); ok {
		return s
	}

	return fmt.Sprintf("Quest input:\n%v\n\nPlease complete the quest: %s", ctx.QuestInput, ctx.QuestDescription)
}
