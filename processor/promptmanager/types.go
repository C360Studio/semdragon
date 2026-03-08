// Package promptmanager provides domain-aware prompt composition for quest execution.
// It assembles system prompts from domain catalogs, tier guardrails, skill context,
// and agent persona — replacing hardcoded string concatenation with a gated fragment system.
package promptmanager

import (
	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// FRAGMENT CATEGORIES - Controls assembly ordering (lower = earlier)
// =============================================================================

// FragmentCategory controls assembly ordering. Fragments are sorted by category
// first, then by priority within category.
type FragmentCategory int

const (
	// CategorySystemBase is the domain identity fragment ("You are a developer...").
	CategorySystemBase FragmentCategory = 0
	// CategoryToolDirective contains mandatory tool-call instructions that must
	// appear early in the prompt — before provider hints — so models that short-
	// circuit on the first actionable directive see them first. Used for party
	// lead decompose_quest enforcement.
	CategoryToolDirective FragmentCategory = 50
	// CategoryProviderHints contains provider-specific formatting instructions.
	CategoryProviderHints FragmentCategory = 100
	// CategoryTierGuardrails contains behavioral bounds for the agent's trust tier.
	CategoryTierGuardrails FragmentCategory = 200
	// CategoryPeerFeedback contains low-rating warnings from recent peer reviews.
	// Injected directly by the assembler from AssemblyContext.PeerFeedback, not
	// from the fragment registry, so it can carry runtime data (ratings, text).
	CategoryPeerFeedback FragmentCategory = 250
	// CategorySkillContext contains instructions for quest-required skills.
	CategorySkillContext FragmentCategory = 300
	// CategoryGuildKnowledge contains guild library knowledge fragments.
	CategoryGuildKnowledge FragmentCategory = 400
	// CategoryPersona contains agent character/personality overrides.
	CategoryPersona FragmentCategory = 500
	// CategoryQuestContext contains quest title, description, and constraints.
	CategoryQuestContext FragmentCategory = 600
)

// =============================================================================
// PROMPT FRAGMENT - Atomic unit of prompt composition
// =============================================================================

// PromptFragment is the atomic unit of prompt composition.
// Fragments are gated by tier, skills, provider, guild, and optional Condition —
// only matching fragments are included in the assembled prompt.
type PromptFragment struct {
	ID       string
	Category FragmentCategory
	Content  string
	Priority int // Ordering within category (lower = first)

	// Gating (nil/empty = matches all)
	MinTier   *domain.TrustTier
	MaxTier   *domain.TrustTier
	Skills    []domain.SkillTag // Agent must have >= 1
	Providers []string          // "anthropic", "openai", "ollama"
	GuildID   *domain.GuildID

	// Condition is an optional runtime predicate evaluated after all structural
	// gates pass. If non-nil, the fragment is included only when Condition returns
	// true. Use this for context fields (e.g. PartyRequired, IsPartyLead) that
	// have no corresponding struct gate.
	Condition func(AssemblyContext) bool
}

// =============================================================================
// PROVIDER STYLE - Formatting conventions per LLM provider
// =============================================================================

// ProviderStyle controls formatting per provider.
type ProviderStyle struct {
	Provider       string
	PreferXML      bool // Anthropic: wrap sections in XML tags
	PreferMarkdown bool // OpenAI/Ollama: markdown headers
}

// =============================================================================
// ASSEMBLY CONTEXT - Input to prompt assembly
// =============================================================================

// AssemblyContext is the input to prompt assembly. It provides all the information
// needed to select and compose the right fragments for a specific execution.
type AssemblyContext struct {
	// Agent identity and capabilities
	AgentID      domain.AgentID
	Tier         domain.TrustTier
	Level        int
	Skills       map[domain.SkillTag]domain.SkillProficiency
	Guilds       []domain.GuildID
	SystemPrompt string // from AgentConfig (override)
	PersonaPrompt string // from AgentPersona

	// Quest details
	QuestTitle       string
	QuestDescription string
	QuestInput       any
	RequiredSkills   []domain.SkillTag
	MaxDuration      string
	MaxTokens        int

	// PeerFeedback carries low-rated peer review questions to be surfaced as
	// warnings in the assembled prompt. Only questions with below-threshold ratings
	// should be included; the assembler emits them verbatim without further filtering.
	PeerFeedback []PeerFeedbackSummary `json:"peer_feedback,omitempty"`

	// Party context
	PartyRequired bool // Quest requires party collaboration
	IsPartyLead   bool // This agent is the party lead (Master+ tier)

	// ClarificationAnswers carries previous Q&A exchanges between the member
	// agent and the party lead. Populated by questbridge from the sub-quest
	// entity's quest.dag.clarifications predicate when re-dispatching a
	// sub-quest after clarification. The assembler renders them as a
	// "Previous Clarifications" section so the agent has context.
	ClarificationAnswers []ClarificationAnswer `json:"clarification_answers,omitempty"`

	// Resolution
	Provider string // from resolved endpoint ("anthropic", "openai", etc.)
}

// ClarificationAnswer is a single Q&A exchange from a party clarification loop.
type ClarificationAnswer struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// PeerFeedbackSummary describes a single peer-review question on which the agent
// received a below-threshold average rating. It is included in AssemblyContext so
// the assembler can inject corrective guidance into the system prompt.
type PeerFeedbackSummary struct {
	Question    string  `json:"question"`
	AvgRating   float64 `json:"avg_rating"`
	Explanation string  `json:"explanation"`
}

// =============================================================================
// ASSEMBLED PROMPT - Output of prompt assembly
// =============================================================================

// AssembledPrompt is the output of prompt assembly.
type AssembledPrompt struct {
	SystemMessage string   // The composed system prompt
	UserMessage   string   // The user message (quest input)
	FragmentsUsed []string // Fragment IDs for observability
}
