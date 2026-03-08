package promptmanager

// =============================================================================
// BUILT-IN FRAGMENTS - Cross-domain fragments registered by components
// =============================================================================
// These fragments apply across all domains and are not part of any DomainCatalog.
// Call RegisterBuiltinFragments after RegisterProviderStyles and RegisterDomainCatalog.
// =============================================================================

// RegisterBuiltinFragments registers cross-domain fragments into the registry.
// Currently registers:
//   - Party lead tool directive (CategoryToolDirective) — enforces decompose_quest
//     calls for party lead agents on party-required quests.
//   - Sub-quest executor directive (CategoryToolDirective) — guides party member
//     agents to complete work and submit results via [INTENT: work_product].
//   - Gemini/OpenAI tool enforcement hint (CategoryProviderHints) — reinforces
//     immediate tool-call behaviour for providers that may otherwise respond with text.
//
// Call this once per registry, after RegisterDomainCatalog and RegisterProviderStyles.
func RegisterBuiltinFragments(r *PromptRegistry) {
	registerPartyLeadDirective(r)
	registerPartyLeadProviderHints(r)
	registerSubQuestExecutorDirective(r)
}

// partyLeadDirective is the mandatory tool-call instruction for party leads.
// It appears in CategoryToolDirective (50) — after SystemBase (0) but before
// all other categories — so models that short-circuit on the first actionable
// directive encounter it immediately.
const partyLeadDirective = `You are a PARTY LEAD coordinating a team of agents.

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

func registerPartyLeadDirective(r *PromptRegistry) {
	r.Register(&PromptFragment{
		ID:        "builtin.party-lead.tool-directive",
		Category:  CategoryToolDirective,
		Content:   partyLeadDirective,
		Priority:  0,
		Condition: isPartyLead,
	})
}

// subQuestExecutorDirective guides party member agents working on sub-quests.
// Without this, agents spin through tools without knowing how to signal completion.
const subQuestExecutorDirective = `You are executing a SUB-QUEST assigned to you by a party lead.

COMPLETION RULES:
1. Complete the task described in the quest objective.
2. Use available tools (read_file, write_file, patch_file, etc.) as needed to do the work.
3. When you have finished, respond with [INTENT: work_product] followed by your complete deliverable.
4. Your deliverable should contain the actual work output — code, analysis, or results — not a description of what you did.
5. Do NOT ask clarifying questions unless the objective is truly ambiguous. Default to reasonable assumptions.
6. Do NOT explore endlessly. Complete the work in as few iterations as possible.
7. If the task is simple enough to answer directly (e.g., write a function), respond immediately with [INTENT: work_product] and the result.`

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
