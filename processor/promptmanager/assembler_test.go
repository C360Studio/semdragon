package promptmanager

import (
	"strings"
	"testing"

	"github.com/c360studio/semdragons/domain"
)

func newTestAssembler() (*PromptAssembler, *PromptRegistry) {
	reg := NewPromptRegistry()
	reg.RegisterProviderStyles()
	reg.RegisterDomainCatalog(testCatalog())
	return NewPromptAssembler(reg), reg
}

func TestAssembleSystemPrompt_BasicAssembly(t *testing.T) {
	assembler, _ := newTestAssembler()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier: domain.TierApprentice,
	})

	if result.SystemMessage == "" {
		t.Fatal("expected non-empty system message")
	}

	// Should contain system base
	if !strings.Contains(result.SystemMessage, "You are a developer.") {
		t.Error("missing system base in output")
	}

	// Should contain apprentice guardrails
	if !strings.Contains(result.SystemMessage, "Junior guardrails") {
		t.Error("missing apprentice guardrails in output")
	}

	// Should track used fragments
	if len(result.FragmentsUsed) < 2 {
		t.Errorf("expected >= 2 fragments used, got %d", len(result.FragmentsUsed))
	}
}

func TestAssembleSystemPrompt_TierSpecificGuardrails(t *testing.T) {
	assembler, _ := newTestAssembler()

	tests := []struct {
		name        string
		tier        domain.TrustTier
		wantContain string
		wantExclude string
	}{
		{
			name:        "apprentice gets junior",
			tier:        domain.TierApprentice,
			wantContain: "Junior guardrails",
			wantExclude: "Senior guardrails",
		},
		{
			name:        "expert gets senior",
			tier:        domain.TierExpert,
			wantContain: "Senior guardrails",
			wantExclude: "Junior guardrails",
		},
		{
			name:        "master gets staff",
			tier:        domain.TierMaster,
			wantContain: "Staff guardrails",
			wantExclude: "Mid-level guardrails",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := assembler.AssembleSystemPrompt(AssemblyContext{
				Tier: tt.tier,
			})

			if !strings.Contains(result.SystemMessage, tt.wantContain) {
				t.Errorf("expected %q in output", tt.wantContain)
			}
			if strings.Contains(result.SystemMessage, tt.wantExclude) {
				t.Errorf("did not expect %q in output", tt.wantExclude)
			}
		})
	}
}

func TestAssembleSystemPrompt_SkillContext(t *testing.T) {
	assembler, _ := newTestAssembler()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:           domain.TierExpert,
		Skills:         map[domain.SkillTag]domain.SkillProficiency{domain.SkillCodeGen: {}},
		RequiredSkills: []domain.SkillTag{domain.SkillCodeGen},
	})

	if !strings.Contains(result.SystemMessage, "Coding instructions") {
		t.Error("expected coding skill context in output")
	}
}

func TestAssembleSystemPrompt_AnthropicXMLFormatting(t *testing.T) {
	assembler, _ := newTestAssembler()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:     domain.TierExpert,
		Provider: "anthropic",
	})

	// Anthropic should use XML tags
	if !strings.Contains(result.SystemMessage, "<system>") {
		t.Error("expected XML <system> tag for Anthropic provider")
	}
	if !strings.Contains(result.SystemMessage, "</system>") {
		t.Error("expected XML </system> tag for Anthropic provider")
	}
	if !strings.Contains(result.SystemMessage, "<tier_guardrails>") {
		t.Error("expected XML <tier_guardrails> tag for Anthropic provider")
	}
}

func TestAssembleSystemPrompt_OpenAIMarkdownFormatting(t *testing.T) {
	assembler, _ := newTestAssembler()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:     domain.TierExpert,
		Provider: "openai",
	})

	// OpenAI should use markdown headers
	if !strings.Contains(result.SystemMessage, "## System") {
		t.Error("expected markdown ## System header for OpenAI provider")
	}
	if !strings.Contains(result.SystemMessage, "## Tier Guardrails") {
		t.Error("expected markdown ## Tier Guardrails header for OpenAI provider")
	}
}

func TestAssembleSystemPrompt_DefaultFormatting(t *testing.T) {
	assembler, _ := newTestAssembler()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:     domain.TierExpert,
		Provider: "custom-provider",
	})

	// Default should use simple label:content format
	if !strings.Contains(result.SystemMessage, "System:\n") {
		t.Error("expected default 'System:' label format")
	}
}

func TestAssembleSystemPrompt_AgentOverrides(t *testing.T) {
	assembler, _ := newTestAssembler()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:          domain.TierExpert,
		SystemPrompt:  "Custom agent system prompt.",
		PersonaPrompt: "You speak like a pirate.",
	})

	if !strings.Contains(result.SystemMessage, "Custom agent system prompt.") {
		t.Error("expected agent system prompt override in output")
	}
	if !strings.Contains(result.SystemMessage, "You speak like a pirate.") {
		t.Error("expected persona prompt in output")
	}

	// Overrides should come AFTER domain fragments
	sysIdx := strings.Index(result.SystemMessage, "You are a developer.")
	overrideIdx := strings.Index(result.SystemMessage, "Custom agent system prompt.")
	if overrideIdx < sysIdx {
		t.Error("agent override should come after domain system base")
	}
}

func TestAssembleSystemPrompt_QuestContext(t *testing.T) {
	assembler, _ := newTestAssembler()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:             domain.TierExpert,
		QuestTitle:       "Fix the login bug",
		QuestDescription: "Users can't log in with SSO",
		MaxDuration:      "30m",
		MaxTokens:        10000,
		RequiredSkills:   []domain.SkillTag{domain.SkillCodeGen},
	})

	if !strings.Contains(result.SystemMessage, "Fix the login bug") {
		t.Error("expected quest title in output")
	}
	if !strings.Contains(result.SystemMessage, "Users can't log in with SSO") {
		t.Error("expected quest description in output")
	}
	if !strings.Contains(result.SystemMessage, "30m") {
		t.Error("expected time limit in output")
	}
	if !strings.Contains(result.SystemMessage, "10000") {
		t.Error("expected token budget in output")
	}
}

func TestAssembleSystemPrompt_UserMessage(t *testing.T) {
	assembler, _ := newTestAssembler()

	tests := []struct {
		name        string
		input       any
		description string
		want        string
	}{
		{
			name:        "nil input uses description",
			input:       nil,
			description: "Fix the bug",
			want:        "Fix the bug",
		},
		{
			name:        "string input used directly",
			input:       "Detailed instructions here",
			description: "Fix the bug",
			want:        "Detailed instructions here",
		},
		{
			name:        "non-string input formatted",
			input:       map[string]string{"file": "main.go"},
			description: "Fix the bug",
			want:        "Quest input:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := assembler.AssembleSystemPrompt(AssemblyContext{
				Tier:             domain.TierExpert,
				QuestInput:       tt.input,
				QuestDescription: tt.description,
			})

			if !strings.Contains(result.UserMessage, tt.want) {
				t.Errorf("UserMessage = %q, want to contain %q", result.UserMessage, tt.want)
			}
		})
	}
}

func TestAssembleSystemPrompt_EmptyQuestContext(t *testing.T) {
	assembler, _ := newTestAssembler()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier: domain.TierExpert,
	})

	// No quest title/description → no quest section
	if strings.Contains(result.SystemMessage, "Title:") {
		t.Error("should not have quest context when no title/description set")
	}
}

func TestAssembleSystemPrompt_FragmentOrdering(t *testing.T) {
	assembler, _ := newTestAssembler()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:             domain.TierExpert,
		Skills:           map[domain.SkillTag]domain.SkillProficiency{domain.SkillCodeGen: {}},
		RequiredSkills:   []domain.SkillTag{domain.SkillCodeGen},
		QuestTitle:       "Test quest",
		QuestDescription: "Test description",
		SystemPrompt:     "Agent override",
	})

	msg := result.SystemMessage

	// SystemBase before TierGuardrails before SkillContext before agent override before quest
	baseIdx := strings.Index(msg, "You are a developer.")
	guardrailIdx := strings.Index(msg, "Senior guardrails")
	skillIdx := strings.Index(msg, "Coding instructions")
	overrideIdx := strings.Index(msg, "Agent override")
	questIdx := strings.Index(msg, "Test quest")

	if baseIdx >= guardrailIdx {
		t.Error("system base should come before tier guardrails")
	}
	if guardrailIdx >= skillIdx {
		t.Error("tier guardrails should come before skill context")
	}
	if skillIdx >= overrideIdx {
		t.Error("skill context should come before agent override")
	}
	if overrideIdx >= questIdx {
		t.Error("agent override should come before quest context")
	}
}

// =============================================================================
// PEER FEEDBACK INJECTION TESTS
// =============================================================================

func TestAssembly_WithPeerFeedback(t *testing.T) {
	assembler, _ := newTestAssembler()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier: domain.TierExpert,
		PeerFeedback: []PeerFeedbackSummary{
			{Question: "Communicates clearly", AvgRating: 2.1, Explanation: "Responses were too terse."},
		},
	})

	if !strings.Contains(result.SystemMessage, "Peer Feedback") {
		t.Error("expected 'Peer Feedback' section header in output")
	}
	if !strings.Contains(result.SystemMessage, "You MUST address these") {
		t.Error("expected mandatory warning preamble in peer feedback section")
	}
}

func TestAssembly_WithPeerFeedback_XMLFormat(t *testing.T) {
	assembler, _ := newTestAssembler()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:     domain.TierExpert,
		Provider: "anthropic",
		PeerFeedback: []PeerFeedbackSummary{
			{Question: "Meets deadlines", AvgRating: 1.8},
		},
	})

	// Anthropic provider must wrap the section in XML tags.
	if !strings.Contains(result.SystemMessage, "<peer_feedback>") {
		t.Error("expected <peer_feedback> XML open tag for Anthropic provider")
	}
	if !strings.Contains(result.SystemMessage, "</peer_feedback>") {
		t.Error("expected </peer_feedback> XML close tag for Anthropic provider")
	}
}

func TestAssembly_WithPeerFeedback_MarkdownFormat(t *testing.T) {
	assembler, _ := newTestAssembler()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:     domain.TierExpert,
		Provider: "openai",
		PeerFeedback: []PeerFeedbackSummary{
			{Question: "Asks clarifying questions", AvgRating: 2.5},
		},
	})

	// OpenAI provider must use a markdown header.
	if !strings.Contains(result.SystemMessage, "## Peer Feedback") {
		t.Error("expected '## Peer Feedback' markdown header for OpenAI provider")
	}
}

func TestAssembly_WithoutPeerFeedback(t *testing.T) {
	assembler, _ := newTestAssembler()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:         domain.TierExpert,
		PeerFeedback: nil, // explicitly empty
	})

	if strings.Contains(result.SystemMessage, "Peer Feedback") {
		t.Error("should not include 'Peer Feedback' section when no feedback provided")
	}
	if strings.Contains(result.SystemMessage, "peer-feedback-warnings") {
		t.Error("should not include peer-feedback-warnings in FragmentsUsed when no feedback")
	}
}

func TestAssembly_PeerFeedbackContent(t *testing.T) {
	assembler, _ := newTestAssembler()

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier: domain.TierExpert,
		PeerFeedback: []PeerFeedbackSummary{
			{Question: "Code quality", AvgRating: 1.5, Explanation: "Too many magic numbers."},
			{Question: "Documentation", AvgRating: 2.0, Explanation: ""},
		},
	})

	// Each question must appear with its rating.
	if !strings.Contains(result.SystemMessage, "Code quality") {
		t.Error("expected first question 'Code quality' in output")
	}
	if !strings.Contains(result.SystemMessage, "1.5/5.0") {
		t.Error("expected rating '1.5/5.0' in output")
	}
	if !strings.Contains(result.SystemMessage, "Too many magic numbers.") {
		t.Error("expected explanation 'Too many magic numbers.' in output")
	}
	if !strings.Contains(result.SystemMessage, "Documentation") {
		t.Error("expected second question 'Documentation' in output")
	}
	if !strings.Contains(result.SystemMessage, "2.0/5.0") {
		t.Error("expected rating '2.0/5.0' in output")
	}

	// FragmentsUsed must include the synthetic peer-feedback-warnings ID.
	found := false
	for _, id := range result.FragmentsUsed {
		if id == "peer-feedback-warnings" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'peer-feedback-warnings' in FragmentsUsed")
	}
}

func TestAssembleSystemPrompt_DnDDomain(t *testing.T) {
	// Verify a completely different domain produces different output
	dndCatalog := &DomainCatalog{
		DomainID:   "dnd",
		SystemBase: "You are an adventurer.",
		TierGuardrails: map[domain.TrustTier]string{
			domain.TierApprentice: "You are a Novice.",
			domain.TierExpert:     "You are a Veteran.",
		},
		SkillFragments: map[domain.SkillTag]string{
			"melee": "Swing your sword.",
		},
	}

	reg := NewPromptRegistry()
	reg.RegisterDomainCatalog(dndCatalog)
	assembler := NewPromptAssembler(reg)

	result := assembler.AssembleSystemPrompt(AssemblyContext{
		Tier:           domain.TierApprentice,
		RequiredSkills: []domain.SkillTag{"melee"},
	})

	if !strings.Contains(result.SystemMessage, "You are an adventurer.") {
		t.Error("expected D&D system base")
	}
	if !strings.Contains(result.SystemMessage, "You are a Novice.") {
		t.Error("expected D&D novice guardrails")
	}
	if !strings.Contains(result.SystemMessage, "Swing your sword.") {
		t.Error("expected D&D melee skill context")
	}

	// Should NOT contain software domain content
	if strings.Contains(result.SystemMessage, "developer") {
		t.Error("D&D domain should not contain software vocabulary")
	}
}
