package promptmanager

import (
	"testing"

	"github.com/c360studio/semdragons/domain"
)

// testCatalog returns a DomainCatalog for testing with known content.
func testCatalog() *DomainCatalog {
	return &DomainCatalog{
		DomainID:   domain.DomainSoftware,
		SystemBase: "You are a developer.",
		TierGuardrails: map[domain.TrustTier]string{
			domain.TierApprentice: "Junior guardrails",
			domain.TierJourneyman: "Mid-level guardrails",
			domain.TierExpert:     "Senior guardrails",
			domain.TierMaster:     "Staff guardrails",
		},
		SkillFragments: map[domain.SkillTag]string{
			domain.SkillCodeGen:    "Coding instructions",
			domain.SkillCodeReview: "Review instructions",
			domain.SkillAnalysis:   "Analysis instructions",
		},
		JudgeSystemBase: "You are a code reviewer.",
	}
}

func TestRegisterDomainCatalog_FragmentCount(t *testing.T) {
	reg := NewPromptRegistry()
	reg.RegisterDomainCatalog(testCatalog())

	// 1 system base + 4 tier guardrails + 3 skill fragments = 8
	want := 8
	if got := reg.FragmentCount(); got != want {
		t.Errorf("FragmentCount() = %d, want %d", got, want)
	}
}

func TestGetFragmentsForContext_SystemBase(t *testing.T) {
	reg := NewPromptRegistry()
	reg.RegisterDomainCatalog(testCatalog())

	ctx := AssemblyContext{
		Tier: domain.TierApprentice,
	}

	fragments := reg.GetFragmentsForContext(ctx)

	// Should include system base (ungated) + apprentice guardrails
	var hasSystemBase bool
	for _, f := range fragments {
		if f.Category == CategorySystemBase {
			hasSystemBase = true
			if f.Content != "You are a developer." {
				t.Errorf("SystemBase content = %q, want %q", f.Content, "You are a developer.")
			}
		}
	}
	if !hasSystemBase {
		t.Error("expected SystemBase fragment in results")
	}
}

func TestGetFragmentsForContext_TierGating(t *testing.T) {
	reg := NewPromptRegistry()
	reg.RegisterDomainCatalog(testCatalog())

	tests := []struct {
		name        string
		tier        domain.TrustTier
		wantContent string
	}{
		{"apprentice gets junior guardrails", domain.TierApprentice, "Junior guardrails"},
		{"journeyman gets mid-level guardrails", domain.TierJourneyman, "Mid-level guardrails"},
		{"expert gets senior guardrails", domain.TierExpert, "Senior guardrails"},
		{"master gets staff guardrails", domain.TierMaster, "Staff guardrails"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := AssemblyContext{Tier: tt.tier}
			fragments := reg.GetFragmentsForContext(ctx)

			var guardrailContent string
			guardrailCount := 0
			for _, f := range fragments {
				if f.Category == CategoryTierGuardrails {
					guardrailContent = f.Content
					guardrailCount++
				}
			}

			if guardrailCount != 1 {
				t.Fatalf("expected 1 tier guardrail, got %d", guardrailCount)
			}
			if guardrailContent != tt.wantContent {
				t.Errorf("guardrail = %q, want %q", guardrailContent, tt.wantContent)
			}
		})
	}
}

func TestGetFragmentsForContext_GrandmasterGetsNoGuardrails(t *testing.T) {
	reg := NewPromptRegistry()
	reg.RegisterDomainCatalog(testCatalog())

	// Grandmaster has no guardrails defined in test catalog
	ctx := AssemblyContext{Tier: domain.TierGrandmaster}
	fragments := reg.GetFragmentsForContext(ctx)

	for _, f := range fragments {
		if f.Category == CategoryTierGuardrails {
			t.Errorf("grandmaster should get no guardrails, got %q", f.Content)
		}
	}
}

func TestGetFragmentsForContext_SkillGating(t *testing.T) {
	reg := NewPromptRegistry()
	reg.RegisterDomainCatalog(testCatalog())

	tests := []struct {
		name       string
		skills     map[domain.SkillTag]domain.SkillProficiency
		questSkills []domain.SkillTag
		wantCount  int
		wantIDs    []string
	}{
		{
			name:        "quest requiring code_gen gets coding fragment",
			questSkills: []domain.SkillTag{domain.SkillCodeGen},
			wantCount:   1,
		},
		{
			name:        "quest with no matching skills gets none",
			questSkills: []domain.SkillTag{domain.SkillResearch},
			wantCount:   0,
		},
		{
			name:        "quest with multiple matching skills gets multiple",
			questSkills: []domain.SkillTag{domain.SkillCodeGen, domain.SkillAnalysis},
			wantCount:   2,
		},
		{
			name:        "quest required skills match",
			questSkills: []domain.SkillTag{domain.SkillCodeReview},
			wantCount:   1,
		},
		{
			name:      "agent skills alone do not match (only quest skills matter)",
			skills:    map[domain.SkillTag]domain.SkillProficiency{domain.SkillCodeGen: {}},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := AssemblyContext{
				Tier:           domain.TierExpert,
				Skills:         tt.skills,
				RequiredSkills: tt.questSkills,
			}
			fragments := reg.GetFragmentsForContext(ctx)

			skillCount := 0
			for _, f := range fragments {
				if f.Category == CategorySkillContext {
					skillCount++
				}
			}
			if skillCount != tt.wantCount {
				t.Errorf("skill fragments = %d, want %d", skillCount, tt.wantCount)
			}
		})
	}
}

func TestGetFragmentsForContext_ProviderGating(t *testing.T) {
	reg := NewPromptRegistry()

	// Register a provider-gated fragment
	reg.Register(&PromptFragment{
		ID:        "anthropic-hint",
		Category:  CategoryProviderHints,
		Content:   "Use XML tags for structure.",
		Providers: []string{"anthropic"},
	})

	tests := []struct {
		name     string
		provider string
		want     bool
	}{
		{"anthropic matches", "anthropic", true},
		{"openai excluded", "openai", false},
		{"empty provider excluded", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := AssemblyContext{Provider: tt.provider}
			fragments := reg.GetFragmentsForContext(ctx)

			found := false
			for _, f := range fragments {
				if f.ID == "anthropic-hint" {
					found = true
				}
			}
			if found != tt.want {
				t.Errorf("found anthropic-hint = %v, want %v", found, tt.want)
			}
		})
	}
}

func TestGetFragmentsForContext_GuildGating(t *testing.T) {
	reg := NewPromptRegistry()

	guildID := domain.GuildID("guild-data")
	reg.Register(&PromptFragment{
		ID:       "guild-data-knowledge",
		Category: CategoryGuildKnowledge,
		Content:  "Data guild best practices.",
		GuildID:  &guildID,
	})

	tests := []struct {
		name  string
		guild domain.GuildID
		want  bool
	}{
		{"member gets fragment", domain.GuildID("guild-data"), true},
		{"non-member excluded", domain.GuildID("guild-other"), false},
		{"no guild excluded", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := AssemblyContext{Guild: tt.guild}
			fragments := reg.GetFragmentsForContext(ctx)

			found := false
			for _, f := range fragments {
				if f.ID == "guild-data-knowledge" {
					found = true
				}
			}
			if found != tt.want {
				t.Errorf("found guild fragment = %v, want %v", found, tt.want)
			}
		})
	}
}

func TestGetFragmentsForContext_CategoryOrdering(t *testing.T) {
	reg := NewPromptRegistry()
	reg.RegisterDomainCatalog(testCatalog())

	ctx := AssemblyContext{
		Tier:           domain.TierExpert,
		Skills:         map[domain.SkillTag]domain.SkillProficiency{domain.SkillCodeGen: {}},
		RequiredSkills: []domain.SkillTag{domain.SkillCodeGen},
	}

	fragments := reg.GetFragmentsForContext(ctx)

	// Verify ordering: SystemBase < TierGuardrails < SkillContext
	var lastCategory FragmentCategory = -1
	for _, f := range fragments {
		if f.Category < lastCategory {
			t.Errorf("fragment %q (category %d) appears after category %d", f.ID, f.Category, lastCategory)
		}
		lastCategory = f.Category
	}
}

func TestRegisterProviderStyles(t *testing.T) {
	reg := NewPromptRegistry()
	reg.RegisterProviderStyles()

	tests := []struct {
		provider       string
		wantXML        bool
		wantMarkdown   bool
	}{
		{"anthropic", true, false},
		{"openai", false, true},
		{"ollama", false, true},
		{"unknown", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			style := reg.GetStyle(tt.provider)
			if style.PreferXML != tt.wantXML {
				t.Errorf("PreferXML = %v, want %v", style.PreferXML, tt.wantXML)
			}
			if style.PreferMarkdown != tt.wantMarkdown {
				t.Errorf("PreferMarkdown = %v, want %v", style.PreferMarkdown, tt.wantMarkdown)
			}
		})
	}
}

func TestRegisterFragment_Direct(t *testing.T) {
	reg := NewPromptRegistry()

	reg.Register(&PromptFragment{
		ID:       "custom-fragment",
		Category: CategoryPersona,
		Content:  "Custom persona content.",
	})

	if got := reg.FragmentCount(); got != 1 {
		t.Errorf("FragmentCount() = %d, want 1", got)
	}

	// Fragment should match any context (no gating)
	fragments := reg.GetFragmentsForContext(AssemblyContext{})
	if len(fragments) != 1 {
		t.Fatalf("expected 1 fragment, got %d", len(fragments))
	}
	if fragments[0].Content != "Custom persona content." {
		t.Errorf("content = %q, want %q", fragments[0].Content, "Custom persona content.")
	}
}

func TestRegisterDomainCatalog_EmptySystemBase(t *testing.T) {
	reg := NewPromptRegistry()
	reg.RegisterDomainCatalog(&DomainCatalog{
		DomainID:   "empty",
		SystemBase: "",
	})

	// No system base fragment should be registered
	if got := reg.FragmentCount(); got != 0 {
		t.Errorf("FragmentCount() = %d, want 0 for empty catalog", got)
	}
}

func TestGetFragmentsForContext_CombinedGating(t *testing.T) {
	reg := NewPromptRegistry()

	// Fragment gated by both tier AND provider
	expertTier := domain.TierExpert
	reg.Register(&PromptFragment{
		ID:        "expert-anthropic-only",
		Category:  CategoryProviderHints,
		Content:   "Expert Anthropic hint.",
		MinTier:   &expertTier,
		MaxTier:   &expertTier,
		Providers: []string{"anthropic"},
	})

	tests := []struct {
		name     string
		tier     domain.TrustTier
		provider string
		want     bool
	}{
		{"expert+anthropic matches", domain.TierExpert, "anthropic", true},
		{"expert+openai excluded", domain.TierExpert, "openai", false},
		{"apprentice+anthropic excluded", domain.TierApprentice, "anthropic", false},
		{"master+anthropic excluded", domain.TierMaster, "anthropic", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := AssemblyContext{Tier: tt.tier, Provider: tt.provider}
			fragments := reg.GetFragmentsForContext(ctx)

			found := false
			for _, f := range fragments {
				if f.ID == "expert-anthropic-only" {
					found = true
				}
			}
			if found != tt.want {
				t.Errorf("found = %v, want %v", found, tt.want)
			}
		})
	}
}
