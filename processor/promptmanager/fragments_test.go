package promptmanager

import (
	"strings"
	"testing"

	"github.com/c360studio/semdragons/domain"
)

// newTestAssemblerWithBuiltins returns an assembler that has both domain catalog
// fragments and the built-in fragments registered — matching production setup.
func newTestAssemblerWithBuiltins() (*PromptAssembler, *PromptRegistry) {
	reg := NewPromptRegistry()
	reg.RegisterProviderStyles()
	reg.RegisterDomainCatalog(testCatalog())
	RegisterBuiltinFragments(reg)
	return NewPromptAssembler(reg), reg
}

// =============================================================================
// CategoryToolDirective ordering tests
// =============================================================================

func TestCategoryToolDirective_SortsBeforeProviderHints(t *testing.T) {
	if CategoryToolDirective >= CategoryProviderHints {
		t.Errorf("CategoryToolDirective (%d) must be less than CategoryProviderHints (%d)",
			CategoryToolDirective, CategoryProviderHints)
	}
}

func TestCategoryToolDirective_SortsAfterSystemBase(t *testing.T) {
	if CategoryToolDirective <= CategorySystemBase {
		t.Errorf("CategoryToolDirective (%d) must be greater than CategorySystemBase (%d)",
			CategoryToolDirective, CategorySystemBase)
	}
}

// =============================================================================
// Party lead directive fragment tests
// =============================================================================

func TestRegisterBuiltinFragments_FragmentsRegistered(t *testing.T) {
	reg := NewPromptRegistry()
	RegisterBuiltinFragments(reg)

	// Expect exactly 5 built-in fragments:
	//   - party lead tool directive
	//   - party lead provider hint
	//   - sub-quest executor directive
	//   - solo agent scenario directive
	//   - solo agent work output directive
	if got := reg.FragmentCount(); got != 5 {
		t.Errorf("RegisterBuiltinFragments registered %d fragments, want 5", got)
	}
}

func TestPartyLeadDirective_IncludedForPartyLead(t *testing.T) {
	assembler, _ := newTestAssemblerWithBuiltins()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:          domain.TierMaster,
		Provider:      "openai",
		PartyRequired: true,
		IsPartyLead:   true,
		QuestTitle:    "Build the feature",
	})

	if !strings.Contains(result.SystemMessage, "decompose_quest") {
		t.Error("expected decompose_quest directive in party lead prompt")
	}
	if !strings.Contains(result.SystemMessage, "PARTY LEAD") {
		t.Error("expected PARTY LEAD heading in directive")
	}
	if !strings.Contains(result.SystemMessage, "Call decompose_quest now") {
		t.Error("expected 'Call decompose_quest now' imperative in directive")
	}

	found := false
	for _, id := range result.FragmentsUsed {
		if id == "builtin.party-lead.tool-directive" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'builtin.party-lead.tool-directive' in FragmentsUsed")
	}
}

func TestPartyLeadDirective_ExcludedForNonPartyLead(t *testing.T) {
	assembler, _ := newTestAssemblerWithBuiltins()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:          domain.TierMaster,
		Provider:      "openai",
		PartyRequired: false,
		IsPartyLead:   false,
		QuestTitle:    "Build the feature",
	})

	if strings.Contains(result.SystemMessage, "PARTY LEAD") {
		t.Error("party lead directive should not appear for non-party-lead agents")
	}
}

func TestPartyLeadDirective_ExcludedWhenPartyRequiredButNotLead(t *testing.T) {
	assembler, _ := newTestAssemblerWithBuiltins()

	// Party member (not the lead): PartyRequired=true, IsPartyLead=false.
	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:          domain.TierExpert,
		Provider:      "openai",
		PartyRequired: true,
		IsPartyLead:   false,
		QuestTitle:    "Build the feature",
	})

	if strings.Contains(result.SystemMessage, "PARTY LEAD") {
		t.Error("party lead directive should not appear for non-lead party members")
	}
}

func TestPartyLeadDirective_AppearsBeforeQuestContext(t *testing.T) {
	assembler, _ := newTestAssemblerWithBuiltins()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:             domain.TierMaster,
		Provider:         "openai",
		PartyRequired:    true,
		IsPartyLead:      true,
		QuestTitle:       "Build the feature",
		QuestDescription: "Do important work",
	})

	directiveIdx := strings.Index(result.SystemMessage, "decompose_quest")
	questIdx := strings.Index(result.SystemMessage, "Build the feature")

	if directiveIdx < 0 || questIdx < 0 {
		t.Fatal("expected both directive and quest context in output")
	}
	if directiveIdx >= questIdx {
		t.Error("tool directive should appear before quest context")
	}
}

func TestPartyLeadDirective_AppearsAfterSystemBase(t *testing.T) {
	assembler, _ := newTestAssemblerWithBuiltins()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:          domain.TierMaster,
		Provider:      "openai",
		PartyRequired: true,
		IsPartyLead:   true,
		QuestTitle:    "Build the feature",
	})

	systemIdx := strings.Index(result.SystemMessage, "You are a developer.")
	directiveIdx := strings.Index(result.SystemMessage, "PARTY LEAD")

	if systemIdx < 0 || directiveIdx < 0 {
		t.Fatal("expected both system base and party lead directive in output")
	}
	if systemIdx >= directiveIdx {
		t.Error("system base should appear before tool directive")
	}
}

// =============================================================================
// Provider hint fragment tests
// =============================================================================

func TestPartyLeadProviderHint_IncludedForGemini(t *testing.T) {
	assembler, _ := newTestAssemblerWithBuiltins()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:          domain.TierMaster,
		Provider:      "gemini",
		PartyRequired: true,
		IsPartyLead:   true,
	})

	if !strings.Contains(result.SystemMessage, "MUST call that tool as your first action") {
		t.Error("expected provider hint for gemini party lead")
	}

	found := false
	for _, id := range result.FragmentsUsed {
		if id == "builtin.party-lead.provider-hint" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'builtin.party-lead.provider-hint' in FragmentsUsed for gemini")
	}
}

func TestPartyLeadProviderHint_IncludedForOpenAI(t *testing.T) {
	assembler, _ := newTestAssemblerWithBuiltins()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:          domain.TierMaster,
		Provider:      "openai",
		PartyRequired: true,
		IsPartyLead:   true,
	})

	if !strings.Contains(result.SystemMessage, "MUST call that tool as your first action") {
		t.Error("expected provider hint for openai party lead")
	}
}

func TestPartyLeadProviderHint_ExcludedForAnthropic(t *testing.T) {
	assembler, _ := newTestAssemblerWithBuiltins()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:          domain.TierMaster,
		Provider:      "anthropic",
		PartyRequired: true,
		IsPartyLead:   true,
	})

	// Directive is still included, but provider hint is not.
	if !strings.Contains(result.SystemMessage, "decompose_quest") {
		t.Error("expected party lead directive even for anthropic")
	}
	if strings.Contains(result.SystemMessage, "MUST call that tool as your first action") {
		t.Error("provider hint should be excluded for anthropic — it follows the directive without extra enforcement")
	}
}

func TestPartyLeadProviderHint_ExcludedForNonPartyLead(t *testing.T) {
	assembler, _ := newTestAssemblerWithBuiltins()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:     domain.TierMaster,
		Provider: "gemini",
	})

	if strings.Contains(result.SystemMessage, "MUST call that tool as your first action") {
		t.Error("provider hint should not appear for non-party-lead agents")
	}
}

// =============================================================================
// Condition gate unit tests
// =============================================================================

func TestConditionGating_TrueConditionMatchesAlways(t *testing.T) {
	reg := NewPromptRegistry()
	reg.Register(&PromptFragment{
		ID:        "always-match",
		Category:  CategoryToolDirective,
		Content:   "Always included.",
		Condition: func(_ AssemblyContext) bool { return true },
	})

	fragments := reg.GetFragmentsForContext(AssemblyContext{})
	if len(fragments) != 1 {
		t.Errorf("expected 1 fragment with always-true condition, got %d", len(fragments))
	}
}

func TestConditionGating_FalseConditionExcludesAlways(t *testing.T) {
	reg := NewPromptRegistry()
	reg.Register(&PromptFragment{
		ID:        "never-match",
		Category:  CategoryToolDirective,
		Content:   "Never included.",
		Condition: func(_ AssemblyContext) bool { return false },
	})

	fragments := reg.GetFragmentsForContext(AssemblyContext{})
	if len(fragments) != 0 {
		t.Errorf("expected 0 fragments with always-false condition, got %d", len(fragments))
	}
}

func TestConditionGating_NilConditionMatchesAll(t *testing.T) {
	reg := NewPromptRegistry()
	reg.Register(&PromptFragment{
		ID:        "no-condition",
		Category:  CategoryToolDirective,
		Content:   "No condition.",
		Condition: nil,
	})

	// Should match any context when Condition is nil.
	fragments := reg.GetFragmentsForContext(AssemblyContext{})
	if len(fragments) != 1 {
		t.Errorf("expected 1 fragment with nil condition, got %d", len(fragments))
	}
}

func TestConditionGating_EvaluatedAfterStructuralGates(t *testing.T) {
	reg := NewPromptRegistry()

	callCount := 0
	expertTier := domain.TierExpert

	// Fragment gated by tier AND condition — condition should only fire for experts.
	reg.Register(&PromptFragment{
		ID:      "expert-condition",
		Category: CategoryToolDirective,
		Content: "Expert with condition.",
		MinTier: &expertTier,
		MaxTier: &expertTier,
		Condition: func(_ AssemblyContext) bool {
			callCount++
			return true
		},
	})

	// Non-expert context: tier gate should prevent condition from being called.
	_ = reg.GetFragmentsForContext(AssemblyContext{Tier: domain.TierApprentice})
	if callCount != 0 {
		t.Errorf("Condition should not be called when structural tier gate fails, called %d times", callCount)
	}

	// Expert context: condition should be evaluated.
	_ = reg.GetFragmentsForContext(AssemblyContext{Tier: domain.TierExpert})
	if callCount != 1 {
		t.Errorf("Condition should be called exactly once for matching tier, called %d times", callCount)
	}
}

// =============================================================================
// Sub-quest executor directive tests
// =============================================================================

func TestSubQuestExecutorDirective_IncludedForSubQuest(t *testing.T) {
	assembler, _ := newTestAssemblerWithBuiltins()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:       domain.TierJourneyman,
		Provider:   "gemini",
		IsSubQuest: true,
		QuestTitle: "Convert celsius to fahrenheit",
	})

	if !strings.Contains(result.SystemMessage, "SUB-QUEST") {
		t.Error("expected SUB-QUEST directive in sub-quest agent prompt")
	}
	if !strings.Contains(result.SystemMessage, "INTENT: work_product") {
		t.Error("expected work_product intent instruction in sub-quest directive")
	}
}

func TestSubQuestExecutorDirective_ExcludedForNonSubQuest(t *testing.T) {
	assembler, _ := newTestAssemblerWithBuiltins()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:       domain.TierJourneyman,
		Provider:   "gemini",
		IsSubQuest: false,
		QuestTitle: "Regular quest",
	})

	if strings.Contains(result.SystemMessage, "SUB-QUEST") {
		t.Error("sub-quest directive should not appear for regular quests")
	}
}

func TestSubQuestExecutorDirective_ExcludedForPartyLead(t *testing.T) {
	assembler, _ := newTestAssemblerWithBuiltins()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:          domain.TierMaster,
		Provider:      "openai",
		PartyRequired: true,
		IsPartyLead:   true,
		IsSubQuest:    false,
		QuestTitle:    "Decompose this quest",
	})

	if strings.Contains(result.SystemMessage, "SUB-QUEST") {
		t.Error("sub-quest directive should not appear for party leads")
	}
	if !strings.Contains(result.SystemMessage, "PARTY LEAD") {
		t.Error("party lead directive should be present")
	}
}

// =============================================================================
// Party lead directive — scenario injection tests
// =============================================================================

func TestPartyLeadDirective_WithScenarios_InjectsQuestSpec(t *testing.T) {
	assembler, _ := newTestAssemblerWithBuiltins()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:          domain.TierMaster,
		Provider:      "openai",
		PartyRequired: true,
		IsPartyLead:   true,
		QuestTitle:    "Build the system",
		QuestGoal:     "Deliver a working auth service",
		QuestRequirements: []string{
			"Must use JWT tokens",
			"Must support OAuth2",
		},
		QuestScenarios: []domain.QuestScenario{
			{Name: "Design", Description: "Design the auth schema", Skills: []string{"architecture"}},
			{Name: "Implement", Description: "Write the auth service", Skills: []string{"coding"}, DependsOn: []string{"Design"}},
		},
	})

	if !strings.Contains(result.SystemMessage, "QUEST SPECIFICATION") {
		t.Error("expected QUEST SPECIFICATION heading in party lead directive when scenarios present")
	}
	if !strings.Contains(result.SystemMessage, "Deliver a working auth service") {
		t.Error("expected quest goal in party lead directive")
	}
	if !strings.Contains(result.SystemMessage, "Must use JWT tokens") {
		t.Error("expected requirements in party lead directive")
	}
	if !strings.Contains(result.SystemMessage, "Design: Design the auth schema") {
		t.Error("expected first scenario in party lead directive")
	}
	if !strings.Contains(result.SystemMessage, "Implement: Write the auth service") {
		t.Error("expected second scenario in party lead directive")
	}
	if !strings.Contains(result.SystemMessage, "[depends_on: Design]") {
		t.Error("expected depends_on in scenario listing")
	}
	if !strings.Contains(result.SystemMessage, "Map scenarios to sub-quests") {
		t.Error("expected decomposition instruction when scenarios present")
	}
	if !strings.Contains(result.SystemMessage, "decompose_quest") {
		t.Error("expected decompose_quest directive even with scenarios")
	}
}

func TestPartyLeadDirective_WithoutScenarios_UsesBaseDirective(t *testing.T) {
	assembler, _ := newTestAssemblerWithBuiltins()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:          domain.TierMaster,
		Provider:      "openai",
		PartyRequired: true,
		IsPartyLead:   true,
		QuestTitle:    "Build the feature",
	})

	// Without scenarios, base directive is used — no QUEST SPECIFICATION heading.
	if strings.Contains(result.SystemMessage, "QUEST SPECIFICATION") {
		t.Error("QUEST SPECIFICATION should not appear when no scenarios are present")
	}
	// The base directive text must still be present.
	if !strings.Contains(result.SystemMessage, "PARTY LEAD") {
		t.Error("expected PARTY LEAD heading in base directive")
	}
	if !strings.Contains(result.SystemMessage, "decompose_quest") {
		t.Error("expected decompose_quest instruction in base directive")
	}
}

func TestPartyLeadDirective_ScenarioSkillsRendered(t *testing.T) {
	assembler, _ := newTestAssemblerWithBuiltins()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:          domain.TierMaster,
		Provider:      "openai",
		PartyRequired: true,
		IsPartyLead:   true,
		QuestScenarios: []domain.QuestScenario{
			{Name: "Research", Description: "Gather requirements", Skills: []string{"analysis", "communication"}},
		},
	})

	if !strings.Contains(result.SystemMessage, "[skills: analysis, communication]") {
		t.Error("expected skill list rendered in scenario line")
	}
}

// =============================================================================
// Solo agent scenario directive tests
// =============================================================================

func TestSoloAgentScenarioDirective_IncludedWhenScenariosPresent(t *testing.T) {
	assembler, _ := newTestAssemblerWithBuiltins()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:          domain.TierJourneyman,
		Provider:      "openai",
		PartyRequired: false,
		IsSubQuest:    false,
		QuestTitle:    "Implement feature",
		QuestGoal:     "Build a caching layer",
		QuestRequirements: []string{
			"Use Redis",
			"TTL configurable",
		},
		QuestScenarios: []domain.QuestScenario{
			{Name: "Setup", Description: "Configure Redis connection"},
			{Name: "Cache", Description: "Implement cache reads/writes", DependsOn: []string{"Setup"}},
		},
	})

	if !strings.Contains(result.SystemMessage, "WORK PLAN") {
		t.Error("expected WORK PLAN directive for solo agent with scenarios")
	}
	if !strings.Contains(result.SystemMessage, "Build a caching layer") {
		t.Error("expected quest goal in work plan")
	}
	if !strings.Contains(result.SystemMessage, "Use Redis") {
		t.Error("expected requirements in work plan")
	}
	if !strings.Contains(result.SystemMessage, "Setup: Configure Redis connection") {
		t.Error("expected first scenario in work plan")
	}
	if !strings.Contains(result.SystemMessage, "Cache: Implement cache reads/writes") {
		t.Error("expected second scenario in work plan")
	}
	if !strings.Contains(result.SystemMessage, "[depends_on: Setup]") {
		t.Error("expected depends_on rendered in work plan scenario")
	}

	found := false
	for _, id := range result.FragmentsUsed {
		if id == "builtin.solo-agent.scenario-directive" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'builtin.solo-agent.scenario-directive' in FragmentsUsed")
	}
}

func TestSoloAgentScenarioDirective_ExcludedWhenNoScenarios(t *testing.T) {
	assembler, _ := newTestAssemblerWithBuiltins()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:          domain.TierJourneyman,
		Provider:      "openai",
		PartyRequired: false,
		IsSubQuest:    false,
		QuestTitle:    "Do some work",
	})

	if strings.Contains(result.SystemMessage, "WORK PLAN") {
		t.Error("solo agent scenario directive should not appear when no scenarios are present")
	}
}

func TestSoloAgentScenarioDirective_ExcludedForSubQuest(t *testing.T) {
	assembler, _ := newTestAssemblerWithBuiltins()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:       domain.TierJourneyman,
		Provider:   "openai",
		IsSubQuest: true,
		QuestScenarios: []domain.QuestScenario{
			{Name: "Step1", Description: "Do step 1"},
		},
	})

	// Sub-quest executor directive should be present, not the solo scenario directive.
	if strings.Contains(result.SystemMessage, "WORK PLAN") {
		t.Error("solo scenario directive should not appear for sub-quests")
	}
	if !strings.Contains(result.SystemMessage, "SUB-QUEST") {
		t.Error("sub-quest executor directive should be present for sub-quests")
	}
}

func TestSoloAgentScenarioDirective_ExcludedForPartyLead(t *testing.T) {
	assembler, _ := newTestAssemblerWithBuiltins()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:          domain.TierMaster,
		Provider:      "openai",
		PartyRequired: true,
		IsPartyLead:   true,
		QuestScenarios: []domain.QuestScenario{
			{Name: "Step1", Description: "Do step 1"},
		},
	})

	// Party lead directive should be present; solo scenario directive should not.
	if strings.Contains(result.SystemMessage, "WORK PLAN") {
		t.Error("solo scenario directive should not appear for party leads")
	}
	if !strings.Contains(result.SystemMessage, "PARTY LEAD") {
		t.Error("party lead directive should be present")
	}
}

// =============================================================================
// Old partyLeadDecomposeInstruction no longer injected via quest context
// =============================================================================

func TestQuestContext_NoLongerContainsPartyLeadInstruction(t *testing.T) {
	// Verify the old hardcoded instruction is gone from the quest context section.
	// The directive now lives in CategoryToolDirective (earlier in the prompt).
	assembler, _ := newTestAssemblerWithBuiltins()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:             domain.TierMaster,
		Provider:         "openai",
		PartyRequired:    true,
		IsPartyLead:      true,
		QuestTitle:       "Build the feature",
		QuestDescription: "Important work",
	})

	// The directive must appear before quest context — it is NOT embedded in it.
	directiveIdx := strings.Index(result.SystemMessage, "PARTY LEAD")
	questIdx := strings.Index(result.SystemMessage, "Build the feature")

	if directiveIdx < 0 {
		t.Fatal("PARTY LEAD directive not found")
	}
	if questIdx < 0 {
		t.Fatal("quest context not found")
	}
	if directiveIdx > questIdx {
		t.Error("PARTY LEAD directive is appearing inside quest context (after quest title) — it should precede it")
	}
}
