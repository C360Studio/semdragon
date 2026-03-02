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
//	SystemBase → ProviderHints → TierGuardrails → SkillContext →
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

	// Append agent-level overrides (not from registry — per-agent customization)
	if ctx.SystemPrompt != "" {
		sections = append(sections, formatSection("Agent Configuration", ctx.SystemPrompt, style))
	}
	if ctx.PersonaPrompt != "" {
		sections = append(sections, formatSection("Persona", ctx.PersonaPrompt, style))
	}

	// Quest context is always last
	if questCtx := buildQuestContext(ctx); questCtx != "" {
		sections = append(sections, formatSection("Quest", questCtx, style))
	}

	return AssembledPrompt{
		SystemMessage: strings.Join(sections, "\n\n"),
		UserMessage:   buildUserMessage(ctx),
		FragmentsUsed: usedIDs,
	}
}

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
	case CategoryProviderHints:
		return "Provider"
	case CategoryTierGuardrails:
		return "Tier Guardrails"
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
