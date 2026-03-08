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

	// Expect exactly 2 built-in fragments: directive + provider hint.
	if got := reg.FragmentCount(); got != 2 {
		t.Errorf("RegisterBuiltinFragments registered %d fragments, want 2", got)
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
