package promptmanager

import (
	"fmt"
	"strings"
)

// =============================================================================
// BUILT-IN FRAGMENTS - Cross-domain fragments registered by components
// =============================================================================
// These fragments apply across all domains and are not part of any DomainCatalog.
// Call RegisterBuiltinFragments after RegisterProviderStyles and RegisterDomainCatalog.
// =============================================================================

// RegisterBuiltinFragments registers cross-domain fragments into the registry.
// Currently registers:
//   - Party lead tool directive (CategoryToolDirective) — enforces decompose_quest
//     calls for party lead agents on party-required quests. Includes quest scenarios
//     when present so the lead can map them directly to sub-quests.
//   - Sub-quest executor directive (CategoryToolDirective) — guides party member
//     agents to complete work and submit results via [INTENT: work_product].
//   - Solo agent scenario directive (CategoryToolDirective) — renders quest scenarios
//     as a structured work plan for solo (non-party, non-sub-quest) agents.
//   - Solo agent work output directive (CategoryToolDirective) — reinforces that all
//     solo agents must submit finished work, not descriptions or plans.
//   - Gemini/OpenAI tool enforcement hint (CategoryProviderHints) — reinforces
//     immediate tool-call behaviour for providers that may otherwise respond with text.
//
// Call this once per registry, after RegisterDomainCatalog and RegisterProviderStyles.
func RegisterBuiltinFragments(r *PromptRegistry) {
	registerPartyLeadDirective(r)
	registerPartyLeadProviderHints(r)
	registerSubQuestExecutorDirective(r)
	registerSoloAgentScenarioDirective(r)
	registerSoloAgentWorkOutputDirective(r)
}

// partyLeadDirectiveBase is the mandatory tool-call instruction for party leads
// when no quest scenarios are available. It appears in CategoryToolDirective (50) —
// after SystemBase (0) but before all other categories — so models that
// short-circuit on the first actionable directive encounter it immediately.
const partyLeadDirectiveBase = `You are a PARTY LEAD coordinating a team of agents.

YOUR WORKFLOW:
1. FIRST ACTION: Call the decompose_quest tool to break the quest into independent sub-quests.
   - Each sub-quest must be a self-contained unit of work with a clear objective.
   - Do NOT include a "combine" or "synthesize" step — that is YOUR responsibility.
   - Sub-quests should produce independent outputs (code, analysis, etc.).
2. REVIEW PHASE: You will review each completed sub-quest via review_sub_quest.
   - Accept work that meets the objective; reject with specific feedback if it does not.
3. SYNTHESIS: After all sub-quests are accepted, YOU synthesize the final deliverable.
   - Combine the sub-quest outputs into a single coherent result.
   - Respond with [INTENT: work_product] followed by the combined deliverable.
   - This is YOUR work product — you are accountable for the final quality.

RULES:
- Your FIRST action MUST be to call decompose_quest. Do NOT write text first.
- Do NOT delegate synthesis/combination to a sub-quest — you have all the context from reviews.
- If you respond with text instead of calling decompose_quest, the system will fail.

Call decompose_quest now.`

// partyLeadProviderHint reinforces immediate tool-call behaviour for providers
// (Gemini, OpenAI) that may otherwise emit a text response before calling a tool.
const partyLeadProviderHint = `When instructed to call a specific tool, you MUST call that tool as your first action. Do not provide a text response before calling the tool.`

// isPartyLead is the shared Condition gate for both party lead fragments.
func isPartyLead(ctx AssemblyContext) bool {
	return ctx.PartyRequired && ctx.IsPartyLead
}

// buildPartyLeadDirective constructs the party lead directive, injecting quest
// scenario details when present so the lead can map them to sub-quests directly.
func buildPartyLeadDirective(ctx AssemblyContext) string {
	if len(ctx.QuestScenarios) == 0 {
		return partyLeadDirectiveBase
	}

	var b strings.Builder
	b.WriteString("You are a PARTY LEAD coordinating a team of agents.\n")

	// Inject the structured quest specification when scenarios are available.
	b.WriteString("\nQUEST SPECIFICATION:\n")
	if ctx.QuestGoal != "" {
		b.WriteString(fmt.Sprintf("Goal: %s\n", ctx.QuestGoal))
	}
	if len(ctx.QuestRequirements) > 0 {
		b.WriteString("Requirements:\n")
		for _, req := range ctx.QuestRequirements {
			b.WriteString(fmt.Sprintf("- %s\n", req))
		}
	}

	b.WriteString("\nScenarios (use these to guide your decomposition):\n")
	for i, s := range ctx.QuestScenarios {
		line := fmt.Sprintf("%d. %s: %s", i+1, s.Name, s.Description)
		if len(s.Skills) > 0 {
			line += fmt.Sprintf(" [skills: %s]", strings.Join(s.Skills, ", "))
		}
		if len(s.DependsOn) > 0 {
			line += fmt.Sprintf(" [depends_on: %s]", strings.Join(s.DependsOn, ", "))
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\nMap scenarios to sub-quests. Preserve dependency order.\n")

	b.WriteString(`
YOUR WORKFLOW:
1. FIRST ACTION: Call the decompose_quest tool to break the quest into independent sub-quests.
   - Use the scenarios above as your decomposition blueprint.
   - Each scenario should become one sub-quest. Preserve the depends_on relationships.
   - Do NOT include a "combine" or "synthesize" step — that is YOUR responsibility.
2. REVIEW PHASE: You will review each completed sub-quest via review_sub_quest.
   - Accept work that meets the objective; reject with specific feedback if it does not.
3. SYNTHESIS: After all sub-quests are accepted, YOU synthesize the final deliverable.
   - Combine the sub-quest outputs into a single coherent result.
   - Respond with [INTENT: work_product] followed by the combined deliverable.
   - This is YOUR work product — you are accountable for the final quality.

RULES:
- Your FIRST action MUST be to call decompose_quest. Do NOT write text first.
- Do NOT delegate synthesis/combination to a sub-quest — you have all the context from reviews.
- If you respond with text instead of calling decompose_quest, the system will fail.

Call decompose_quest now.`)

	return b.String()
}

func registerPartyLeadDirective(r *PromptRegistry) {
	r.Register(&PromptFragment{
		ID:          "builtin.party-lead.tool-directive",
		Category:    CategoryToolDirective,
		Priority:    0,
		Condition:   isPartyLead,
		ContentFunc: buildPartyLeadDirective,
	})
}

// subQuestExecutorDirective guides party member agents working on sub-quests.
// Without this, agents spin through tools without knowing how to signal completion.
const subQuestExecutorDirective = `You are executing a SUB-QUEST assigned to you by a party lead.

COMPLETION RULES:
1. Complete the task described in the quest objective.
2. Use available tools (read_file, write_file, patch_file, etc.) as needed to understand the context.
3. When you have finished, respond with [INTENT: work_product] followed by your complete deliverable.
4. Your deliverable MUST contain the actual work output — code, analysis, or results — not a description of what you did.
5. If the task requires code, include BOTH implementation AND tests directly in your deliverable. Your reviewer can only see what you include — they cannot access external files.
6. Do NOT ask clarifying questions unless the objective is truly ambiguous. Default to reasonable assumptions.
7. Do NOT explore endlessly. Complete the work in as few iterations as possible.
8. If the task is simple enough to answer directly (e.g., write a function), respond immediately with [INTENT: work_product] and the result.`

// isSubQuestExecutor returns true for agents working on sub-quests within a
// party DAG (party members executing DAG nodes, not the lead).
func isSubQuestExecutor(ctx AssemblyContext) bool {
	return ctx.IsSubQuest
}

func registerSubQuestExecutorDirective(r *PromptRegistry) {
	r.Register(&PromptFragment{
		ID:        "builtin.sub-quest-executor.tool-directive",
		Category:  CategoryToolDirective,
		Content:   subQuestExecutorDirective,
		Priority:  0,
		Condition: isSubQuestExecutor,
	})
}

func registerPartyLeadProviderHints(r *PromptRegistry) {
	// Gemini and OpenAI require an explicit reminder because they tend to emit
	// a text preamble before calling tools when given strong narrative context.
	// Anthropic models follow the directive above without additional prompting.
	r.Register(&PromptFragment{
		ID:        "builtin.party-lead.provider-hint",
		Category:  CategoryProviderHints,
		Content:   partyLeadProviderHint,
		Priority:  0,
		Providers: []string{"gemini", "openai"},
		Condition: isPartyLead,
	})
}

// isSoloAgentWithScenarios returns true for solo agents (not a party lead or
// sub-quest executor) who have at least one quest scenario defined. These agents
// receive a structured work plan rather than an open-ended quest description.
func isSoloAgentWithScenarios(ctx AssemblyContext) bool {
	return !ctx.PartyRequired && !ctx.IsSubQuest && len(ctx.QuestScenarios) > 0
}

// buildSoloAgentScenarioDirective renders the quest scenarios as a structured
// work plan for solo agents. Dependencies are included so the agent knows
// which scenarios must be completed before others can start.
func buildSoloAgentScenarioDirective(ctx AssemblyContext) string {
	var b strings.Builder
	b.WriteString("WORK PLAN:\n")
	b.WriteString("Complete the following scenarios, respecting dependencies.\n")

	if ctx.QuestGoal != "" {
		b.WriteString(fmt.Sprintf("\nGoal: %s\n", ctx.QuestGoal))
	}
	if len(ctx.QuestRequirements) > 0 {
		b.WriteString("Requirements:\n")
		for _, req := range ctx.QuestRequirements {
			b.WriteString(fmt.Sprintf("- %s\n", req))
		}
	}

	b.WriteString("\nScenarios:\n")
	for i, s := range ctx.QuestScenarios {
		line := fmt.Sprintf("%d. %s: %s", i+1, s.Name, s.Description)
		if len(s.DependsOn) > 0 {
			line += fmt.Sprintf(" [depends_on: %s]", strings.Join(s.DependsOn, ", "))
		}
		b.WriteString(line + "\n")
	}

	b.WriteString("\nComplete all scenarios and submit your deliverable using the submit_work_product tool.")
	return b.String()
}

func registerSoloAgentScenarioDirective(r *PromptRegistry) {
	r.Register(&PromptFragment{
		ID:          "builtin.solo-agent.scenario-directive",
		Category:    CategoryToolDirective,
		Priority:    10, // After party lead directive (0) but distinct
		Condition:   isSoloAgentWithScenarios,
		ContentFunc: buildSoloAgentScenarioDirective,
	})
}

// buildSoloAgentWorkOutputDirective reinforces that solo agents must produce actual
// work products (code, analysis, results) — not descriptions or plans. This is
// the solo-agent equivalent of subQuestExecutorDirective rule #4 and applies to
// all solo agents regardless of whether they have scenarios.
//
// When MaxIterations is set, injects a tool-use budget so the agent plans its
// work instead of exploring open-endedly.
func buildSoloAgentWorkOutputDirective(ctx AssemblyContext) string {
	var b strings.Builder
	b.WriteString("COMPLETION RULES:\n")

	if ctx.MaxIterations > 0 {
		b.WriteString(fmt.Sprintf("0. You have a budget of %d tool-use rounds. Plan your work to finish well within this budget. Do NOT explore open-endedly.\n", ctx.MaxIterations))
	}

	b.WriteString(`1. Use available tools (read_file, etc.) to understand the task, then produce your deliverable.
2. Your deliverable MUST be the finished work output — code, implementation, analysis, or results.
3. Do NOT submit a description of what you would do. Do NOT submit a plan. Submit the completed work itself.
4. If the task requires code, include BOTH the implementation AND tests in your deliverable. Your reviewer can only evaluate what you include in the deliverable text — they cannot access external files.
5. Format code deliverables with clear sections (e.g., "## Implementation" and "## Tests") so your reviewer can assess completeness.
6. Complete the work in as few iterations as possible — avoid unnecessary exploration.`)
	return b.String()
}

// isSoloAgent returns true for agents that are NOT a party lead or sub-quest executor.
func isSoloAgent(ctx AssemblyContext) bool {
	return !ctx.PartyRequired && !ctx.IsSubQuest
}

func registerSoloAgentWorkOutputDirective(r *PromptRegistry) {
	r.Register(&PromptFragment{
		ID:          "builtin.solo-agent.work-output-directive",
		Category:    CategoryToolDirective,
		ContentFunc: buildSoloAgentWorkOutputDirective,
		Priority:    15, // After scenario directive (10)
		Condition:   isSoloAgent,
	})
}
