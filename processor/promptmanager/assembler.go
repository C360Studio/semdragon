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
			var text string
			if f.ContentFunc != nil {
				text = f.ContentFunc(ctx)
			} else {
				text = f.Content
			}
			if text == "" {
				continue
			}
			content.WriteString(text)
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

	// Inject failure recovery context when a quest has been triaged after previous failures.
	// This gives the agent explicit knowledge of what went wrong and what to build on.
	if len(ctx.FailureHistory) > 0 {
		var recovery strings.Builder
		recovery.WriteString("IMPORTANT: This quest has been attempted before and failed. Learn from previous failures:\n")
		for _, fh := range ctx.FailureHistory {
			recovery.WriteString(fmt.Sprintf("\n--- Attempt %d (failed: %s) ---\n%s\n", fh.Attempt, fh.FailureType, fh.FailureReason))
			if fh.TriageVerdict != "" {
				recovery.WriteString(fmt.Sprintf("DM Assessment: %s\n", fh.TriageVerdict))
			}
		}
		if ctx.FailureAnalysis != "" {
			recovery.WriteString(fmt.Sprintf("\nDM Failure Analysis: %s\n", ctx.FailureAnalysis))
		}
		if ctx.SalvagedOutput != "" {
			recovery.WriteString(fmt.Sprintf("\nSalvaged Work (from previous attempt — build on this, do NOT redo it):\n%s\n", ctx.SalvagedOutput))
		}
		if len(ctx.AntiPatterns) > 0 {
			recovery.WriteString("\nDO NOT repeat these mistakes:\n")
			for _, ap := range ctx.AntiPatterns {
				recovery.WriteString(fmt.Sprintf("- %s\n", ap))
			}
		}
		// If any previous failure looks like the agent submitted a question instead
		// of work, remind them about the correct tool.
		if failuresMentionNoWork(ctx.FailureHistory) {
			recovery.WriteString("\nCRITICAL: Your previous submission was rejected because you submitted a question or request for information instead of completed work. ")
			recovery.WriteString("If you need more information, use the ask_clarification tool — you will NOT be penalized. ")
			recovery.WriteString("Only use submit_work_product when you have actual finished work (code, analysis, results) to deliver.\n")
		}
		sections = append(sections, formatSection("Failure Recovery", recovery.String(), style))
		usedIDs = append(usedIDs, "failure-recovery-context")
	}

	// Inject dependency context from completed predecessor DAG nodes.
	// DependencyContexts (structured-deps mode) takes precedence over the legacy
	// DependencyOutputs slice — only one will be non-empty at a time.
	if len(ctx.DependencyContexts) > 0 {
		var deps strings.Builder
		deps.WriteString("The following predecessor tasks have been completed. Use their outputs as context for your work:\n")
		for _, dep := range ctx.DependencyContexts {
			switch dep.ResolutionMode {
			case "structured":
				deps.WriteString(fmt.Sprintf("\n--- Predecessor: %s [%s] ---\n", dep.Objective, dep.NodeID))
				deps.WriteString(dep.Summary)
			case "summary":
				deps.WriteString(fmt.Sprintf("\n--- Predecessor: %s [%s] ---\n", dep.Objective, dep.NodeID))
				if dep.Summary != "" {
					deps.WriteString(fmt.Sprintf("(Artifacts pending indexing)\n\"%s\"\n", dep.Summary))
					deps.WriteString("→ Full details via graph_search once indexing completes\n")
				}
			default: // "raw" or unset
				deps.WriteString(fmt.Sprintf("\n--- Predecessor: %s [%s] ---\n", dep.Objective, dep.NodeID))
				if dep.RawOutput != "" {
					deps.WriteString(dep.RawOutput + "\n")
				}
			}
		}
		sections = append(sections, formatSection("Dependency Outputs", deps.String(), style))
		usedIDs = append(usedIDs, "dependency-contexts")
	} else if len(ctx.DependencyOutputs) > 0 {
		// Legacy path: raw output injection without structured-deps cascade.
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
// It instructs the LLM to use terminal tools to signal completion or clarification
// so questbridge can route the output appropriately.
const responseFormatInstruction = `When you have completed your work, you MUST call the submit_work_product tool with your deliverable.
When you need more information before you can proceed, call the ask_clarification tool with your question.

DELIVERABLE RULES:
- Your deliverable MUST contain the actual work output: code, implementation, analysis, or results.
- Do NOT submit a description of what you would do or a plan to do it — submit the finished work itself.
- If the task requires code, include the FULL source code AND tests directly in the deliverable text. Your reviewer can only see what you put in the deliverable — they cannot access external files.
- Do NOT write your final answer as plain text — always use one of these tools to signal completion or request clarification.`

// =============================================================================
// INTERNAL HELPERS
// =============================================================================

// failuresMentionNoWork returns true if any failure reason indicates the agent
// submitted a question or empty response instead of actual work. Used to inject
// explicit tool guidance (use ask_clarification) into the retry prompt.
func failuresMentionNoWork(failures []FailureHistorySummary) bool {
	for _, f := range failures {
		lower := strings.ToLower(f.FailureReason)
		for _, phrase := range []string{
			"submitted a question",
			"submitted a request for",
			"no code was",
			"no work was",
			"submission is empty",
			"completely empty",
			"did not provide any code",
			"did not provide the requested",
			"submitted a plan",
			"asking for guidance",
			"asking for instructions",
			"request for information",
			"request for more",
		} {
			if strings.Contains(lower, phrase) {
				return true
			}
		}
	}
	return false
}

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
	case CategoryFailureRecovery:
		return "Failure Recovery"
	case CategorySkillContext:
		return "Skills"
	case CategoryToolGuidance:
		return "Tool Guidance"
	case CategoryGuildKnowledge:
		return "Guild Knowledge"
	case CategoryReviewBrief:
		return "Review"
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
