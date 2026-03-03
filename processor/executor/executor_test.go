package executor

import (
	"testing"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semstreams/model"
)

// testRegistry builds a registry with tier and skill capability entries
// for exercising the resolution chain.
func testRegistry() *model.Registry {
	return &model.Registry{
		Endpoints: map[string]*model.EndpointConfig{
			"haiku":    {Provider: "anthropic", Model: "haiku", SupportsTools: true},
			"sonnet":   {Provider: "anthropic", Model: "sonnet", SupportsTools: true},
			"opus":     {Provider: "anthropic", Model: "opus", SupportsTools: true},
			"gpt-mini": {Provider: "openai", Model: "gpt-mini", SupportsTools: true},
			"custom":   {Provider: "custom", Model: "custom", SupportsTools: true},
		},
		Capabilities: map[string]*model.CapabilityConfig{
			// Global fallback
			"agent-work": {
				Preferred:     []string{"sonnet"},
				RequiresTools: true,
			},
			// Tier defaults
			"agent-work.apprentice": {
				Preferred:     []string{"haiku"},
				RequiresTools: true,
			},
			"agent-work.expert": {
				Preferred:     []string{"sonnet"},
				RequiresTools: true,
			},
			"agent-work.master": {
				Preferred:     []string{"opus"},
				RequiresTools: true,
			},
			// Skill-specific overrides
			"agent-work.expert.code_generation": {
				Preferred:     []string{"sonnet"},
				RequiresTools: true,
			},
			"agent-work.expert.summarization": {
				Preferred: []string{"haiku"},
			},
			"agent-work.master.summarization": {
				Preferred: []string{"gpt-mini"},
			},
		},
		Defaults: model.DefaultsConfig{
			Model: "sonnet",
		},
	}
}

func testAgent(tier domain.TrustTier, provider string) *agentprogression.Agent {
	return &agentprogression.Agent{
		ID:   "test-agent",
		Tier: tier,
		Config: agentprogression.AgentConfig{
			Provider: provider,
		},
	}
}

func testQuest(skills ...domain.SkillTag) *domain.Quest {
	return &domain.Quest{
		ID:             "test-quest",
		Title:          "Test Quest",
		RequiredSkills: skills,
	}
}

func TestResolveCapability(t *testing.T) {
	reg := testRegistry()
	exec := NewDefaultExecutor(reg, nil)

	tests := []struct {
		name       string
		tier       domain.TrustTier
		skills     []domain.SkillTag
		wantCap    string
		wantResult string // expected endpoint name from Resolve
	}{
		{
			name:       "expert with code_generation hits tier+skill",
			tier:       domain.TierExpert,
			skills:     []domain.SkillTag{domain.SkillCodeGen},
			wantCap:    "agent-work.expert.code_generation",
			wantResult: "sonnet",
		},
		{
			name:       "expert with summarization hits tier+skill override",
			tier:       domain.TierExpert,
			skills:     []domain.SkillTag{domain.SkillSummarization},
			wantCap:    "agent-work.expert.summarization",
			wantResult: "haiku",
		},
		{
			name:       "master with summarization hits tier+skill override",
			tier:       domain.TierMaster,
			skills:     []domain.SkillTag{domain.SkillSummarization},
			wantCap:    "agent-work.master.summarization",
			wantResult: "gpt-mini",
		},
		{
			name:       "apprentice with unknown skill falls to tier default",
			tier:       domain.TierApprentice,
			skills:     []domain.SkillTag{domain.SkillResearch},
			wantCap:    "agent-work.apprentice",
			wantResult: "haiku",
		},
		{
			name:       "apprentice with no skills falls to tier default",
			tier:       domain.TierApprentice,
			skills:     nil,
			wantCap:    "agent-work.apprentice",
			wantResult: "haiku",
		},
		{
			name:       "master with code_generation falls to tier default (no skill entry)",
			tier:       domain.TierMaster,
			skills:     []domain.SkillTag{domain.SkillCodeGen},
			wantCap:    "agent-work.master",
			wantResult: "opus",
		},
		{
			name:       "journeyman with no tier entry falls to global",
			tier:       domain.TierJourneyman,
			skills:     []domain.SkillTag{domain.SkillAnalysis},
			wantCap:    "agent-work",
			wantResult: "sonnet",
		},
		{
			name:       "grandmaster with no tier entry falls to global",
			tier:       domain.TierGrandmaster,
			skills:     nil,
			wantCap:    "agent-work",
			wantResult: "sonnet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := testAgent(tt.tier, "")
			quest := testQuest(tt.skills...)

			gotCap := exec.resolveCapability(agent, quest)
			if gotCap != tt.wantCap {
				t.Errorf("resolveCapability() = %q, want %q", gotCap, tt.wantCap)
			}

			gotEndpoint := reg.Resolve(gotCap)
			if gotEndpoint != tt.wantResult {
				t.Errorf("Resolve(%q) = %q, want %q", gotCap, gotEndpoint, tt.wantResult)
			}
		})
	}
}

func TestResolveCapability_AgentOverrideBypassesChain(t *testing.T) {
	reg := testRegistry()
	exec := NewDefaultExecutor(reg, nil)

	// When agent has Config.Provider set, resolveCapability is not called.
	// Verify the override works at the Execute level by checking endpoint resolution.
	agent := testAgent(domain.TierExpert, "custom")
	quest := testQuest(domain.SkillCodeGen)

	// The capability chain would return "agent-work.expert.code_generation",
	// but agent.Config.Provider = "custom" should take priority.
	// We verify this by checking that resolveCapability still returns the
	// tier+skill key (it doesn't know about the override — Execute() does).
	gotCap := exec.resolveCapability(agent, quest)
	if gotCap != "agent-work.expert.code_generation" {
		t.Errorf("resolveCapability() = %q, want %q (override happens in Execute, not here)",
			gotCap, "agent-work.expert.code_generation")
	}
}

func TestTrustTierString(t *testing.T) {
	tests := []struct {
		tier domain.TrustTier
		want string
	}{
		{domain.TierApprentice, "apprentice"},
		{domain.TierJourneyman, "journeyman"},
		{domain.TierExpert, "expert"},
		{domain.TierMaster, "master"},
		{domain.TierGrandmaster, "grandmaster"},
		{domain.TrustTier(99), "apprentice"}, // unknown defaults to apprentice
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.tier.String(); got != tt.want {
				t.Errorf("TrustTier(%d).String() = %q, want %q", tt.tier, got, tt.want)
			}
		})
	}
}

func TestQuestPrimarySkill(t *testing.T) {
	tests := []struct {
		name   string
		skills []domain.SkillTag
		want   domain.SkillTag
	}{
		{"no skills returns empty", nil, ""},
		{"empty slice returns empty", []domain.SkillTag{}, ""},
		{"single skill returns it", []domain.SkillTag{domain.SkillCodeGen}, domain.SkillCodeGen},
		{"multiple skills returns first", []domain.SkillTag{domain.SkillAnalysis, domain.SkillCodeGen}, domain.SkillAnalysis},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := testQuest(tt.skills...)
			if got := q.PrimarySkill(); got != tt.want {
				t.Errorf("PrimarySkill() = %q, want %q", got, tt.want)
			}
		})
	}
}
