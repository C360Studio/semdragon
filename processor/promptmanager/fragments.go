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
//   - Gemini/OpenAI tool enforcement hint (CategoryProviderHints) — reinforces
//     immediate tool-call behaviour for providers that may otherwise respond with text.
//
// Call this once per registry, after RegisterDomainCatalog and RegisterProviderStyles.
func RegisterBuiltinFragments(r *PromptRegistry) {
	registerPartyLeadDirective(r)
	registerPartyLeadProviderHints(r)
}

// partyLeadDirective is the mandatory tool-call instruction for party leads.
// It appears in CategoryToolDirective (50) — after SystemBase (0) but before
// all other categories — so models that short-circuit on the first actionable
// directive encounter it immediately.
const partyLeadDirective = `You are a PARTY LEAD. Your FIRST action MUST be to call the decompose_quest tool.

RULES:
1. Do NOT write any text response.
2. Do NOT attempt to complete the quest yourself.
3. Call the decompose_quest tool with a DAG of sub-quests.
4. Each sub-quest must have a clear objective, required skills, and acceptance criteria.
5. If you respond with text instead of calling decompose_quest, the system will fail.
6. After sub-quests complete, you will review each output via review_sub_quest.

Your primary tools: decompose_quest (to plan work) and review_sub_quest (to evaluate results). Call decompose_quest now.`

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
