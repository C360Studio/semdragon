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
	// CategoryFailureRecovery contains previous attempt failure context,
	// salvaged output, and anti-patterns. Injected by the assembler when
	// a quest has failure history from DM triage.
	CategoryFailureRecovery FragmentCategory = 275
	// CategorySkillContext contains instructions for quest-required skills.
	CategorySkillContext FragmentCategory = 300
	// CategoryToolGuidance contains advisory guidance on when to use which tool.
	// Placed after skill context so agents understand their capabilities first.
	CategoryToolGuidance FragmentCategory = 325
	// CategoryGuildKnowledge contains guild library knowledge fragments.
	CategoryGuildKnowledge FragmentCategory = 400
	// CategoryReviewBrief contains a compact summary of how the agent's work
	// will be evaluated — review level, scoring criteria, and peer review
	// dimensions. Placed after guild knowledge so agents see it before
	// starting work, but kept intentionally short to minimize context cost.
	CategoryReviewBrief FragmentCategory = 450
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

	// ContentFunc is an optional function that generates fragment content
	// dynamically from the AssemblyContext. When set, it takes precedence over
	// Content. Use this for fragments that need runtime data injected (e.g.
	// quest scenarios, goal text).
	ContentFunc func(AssemblyContext) string
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
	Guild        domain.GuildID
	SystemPrompt string // from AgentConfig (override)
	PersonaPrompt string // from AgentPersona

	// Quest details
	QuestTitle           string
	QuestDescription     string
	QuestInput           any
	RequiredSkills       []domain.SkillTag
	MaxDuration          string
	MaxTokens            int
	QuestGoal         string
	QuestRequirements []string
	QuestScenarios    []domain.QuestScenario
	// DecomposabilityClass is used by questbridge for model routing (sequential
	// quests route to stronger models) and available to fragments for future use.
	DecomposabilityClass domain.DecomposabilityClass

	// PeerFeedback carries low-rated peer review questions to be surfaced as
	// warnings in the assembled prompt. Only questions with below-threshold ratings
	// should be included; the assembler emits them verbatim without further filtering.
	PeerFeedback []PeerFeedbackSummary `json:"peer_feedback,omitempty"`

	// Party context
	PartyRequired bool // Quest requires party collaboration
	IsPartyLead   bool // This agent is the party lead (Master+ tier)
	IsSubQuest    bool // This quest is a sub-quest within a party DAG

	// ClarificationAnswers carries previous Q&A exchanges between the member
	// agent and the party lead. Populated by questbridge from the sub-quest
	// entity's quest.dag.clarifications predicate when re-dispatching a
	// sub-quest after clarification. The assembler renders them as a
	// "Previous Clarifications" section so the agent has context.
	ClarificationAnswers []ClarificationAnswer `json:"clarification_answers,omitempty"`

	// ClarificationSource identifies who answered the clarification (e.g., "DM", "party lead").
	// Used by the assembler to produce context-aware section headers.
	ClarificationSource string `json:"clarification_source,omitempty"`

	// DependencyOutputs carries outputs from predecessor DAG nodes. When a
	// sub-quest depends on completed nodes, their outputs are loaded from the
	// graph and injected into the prompt so the agent has context about what
	// predecessor steps produced.
	DependencyOutputs []DependencyOutput `json:"dependency_outputs,omitempty"`

	// DependencyContexts is the richer alternative to DependencyOutputs, used
	// when EnableStructuredDeps is true in questbridge config. Each entry
	// carries a ResolutionMode ("structured", "summary", "raw") that drives
	// how the assembler formats it. When this slice is non-empty the assembler
	// renders it in place of DependencyOutputs.
	DependencyContexts []DependencyContext `json:"dependency_contexts,omitempty"`

	// StructuralChecklist carries domain-specific pass/fail requirements that
	// agents should self-check before submitting. These same items are enforced
	// during boss battle review — any failure is automatic defeat.
	StructuralChecklist []ChecklistItem `json:"structural_checklist,omitempty"`

	// Failure recovery context — injected when a quest has been triaged by the DM
	// after exhausting retry attempts.
	FailureHistory  []FailureHistorySummary `json:"failure_history,omitempty"`
	SalvagedOutput  string                  `json:"salvaged_output,omitempty"`
	FailureAnalysis string                  `json:"failure_analysis,omitempty"`
	RecoveryPath    string                  `json:"recovery_path,omitempty"`
	AntiPatterns    []string                `json:"anti_patterns,omitempty"`

	// Review awareness — tells the agent how their work will be evaluated.
	// Populated from the domain's ReviewConfig at dispatch time.
	ReviewLevel    domain.ReviewLevel      `json:"review_level,omitempty"`
	ReviewCriteria []domain.ReviewCriterion `json:"review_criteria,omitempty"`

	// AvailableToolNames lists the tool names available to this agent for this quest.
	// Used by the tool selection guidance fragment to generate contextual guidance.
	AvailableToolNames []string `json:"available_tool_names,omitempty"`

	// Iteration budget — tells the agent how many tool-use rounds it has.
	// Set from questbridge/executor config so agents plan work accordingly.
	MaxIterations int `json:"max_iterations,omitempty"`

	// Resolution
	Provider string // from resolved endpoint ("anthropic", "openai", etc.)
}

// ClarificationAnswer is a single Q&A exchange from a party clarification loop.
type ClarificationAnswer struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// DependencyOutput represents the output from a completed predecessor DAG node.
// Injected into the system prompt so dependent nodes have context about what
// their upstream steps produced.
type DependencyOutput struct {
	NodeID    string `json:"node_id"`
	Objective string `json:"objective"`
	Output    string `json:"output"`
}

// DependencyContext provides structured context from a predecessor quest.
// When semsource has indexed the predecessor's artifacts, the summary contains
// compact identity-level info (function signatures, types, exports). The agent
// can drill into details on-demand via graph_search using the EntityRefs.
type DependencyContext struct {
	NodeID         string   `json:"node_id"`
	Objective      string   `json:"objective"`
	Summary        string   `json:"summary"`
	EntityRefs     []string `json:"entity_refs"`
	RawOutput      string   `json:"raw_output"`
	ResolutionMode string   `json:"resolution_mode"` // "structured", "summary", "raw"
}

// PeerFeedbackSummary describes a single peer-review question on which the agent
// received a below-threshold average rating. It is included in AssemblyContext so
// the assembler can inject corrective guidance into the system prompt.
type PeerFeedbackSummary struct {
	Question    string  `json:"question"`
	AvgRating   float64 `json:"avg_rating"`
	Explanation string  `json:"explanation"`
}

// FailureHistorySummary is a lightweight record of one previous attempt's failure,
// used in the prompt to give agents context about what went wrong before.
type FailureHistorySummary struct {
	Attempt       int    `json:"attempt"`
	FailureType   string `json:"failure_type"`
	FailureReason string `json:"failure_reason"`
	TriageVerdict string `json:"triage_verdict,omitempty"`
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
