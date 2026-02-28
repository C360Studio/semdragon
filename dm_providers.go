package semdragons

import (
	"context"
	"sync"
	"time"
)

// =============================================================================
// DM PROVIDERS - LLM and Approval interfaces for DM decision making
// =============================================================================
// These interfaces abstract the decision-making capabilities needed by
// different DM modes. ManualDM uses ApprovalRouter for all decisions,
// while FullAutoDM would use LLMProvider for automated decisions.
// =============================================================================

// -----------------------------------------------------------------------------
// LLM Provider - For automated decision making
// -----------------------------------------------------------------------------

// LLMProvider abstracts LLM capabilities for automated DM decisions.
// This interface enables different LLM backends (Anthropic, OpenAI, local)
// to be plugged into the DM system.
type LLMProvider interface {
	// DecideQuestParameters determines quest configuration from an objective.
	// Returns difficulty, required skills, XP, review level, etc.
	DecideQuestParameters(ctx context.Context, objective string, hints QuestHints) (*QuestDecision, error)

	// ReviewDecomposition evaluates a party lead's breakdown of a quest.
	// Returns approved/modified sub-quests.
	ReviewDecomposition(ctx context.Context, parent Quest, subQuests []Quest) (*DecompositionReview, error)

	// EvaluateAgent assesses an agent's current performance and trajectory.
	EvaluateAgent(ctx context.Context, agent Agent, recentQuests []Quest) (*AgentEvaluation, error)

	// SuggestIntervention recommends how to help a struggling quest.
	SuggestIntervention(ctx context.Context, quest Quest, context InterventionContext) (*Intervention, error)

	// HandleEscalation decides how to resolve an escalated quest.
	HandleEscalation(ctx context.Context, quest Quest, attempts []EscalationAttempt) (*EscalationDecision, error)
}

// QuestDecision holds LLM-generated quest parameters.
type QuestDecision struct {
	Difficulty     QuestDifficulty `json:"difficulty"`
	RequiredSkills []SkillTag      `json:"required_skills"`
	BaseXP         int64           `json:"base_xp"`
	ReviewLevel    ReviewLevel     `json:"review_level"`
	PartyRequired  bool            `json:"party_required"`
	MinPartySize   int             `json:"min_party_size"`
	GuildPriority  *GuildID        `json:"guild_priority,omitempty"`
	Reasoning      string          `json:"reasoning"`
}

// DecompositionReview holds the result of reviewing sub-quest breakdown.
type DecompositionReview struct {
	Approved   bool    `json:"approved"`
	SubQuests  []Quest `json:"sub_quests"` // May be modified
	Feedback   string  `json:"feedback"`
	Suggestion string  `json:"suggestion,omitempty"`
}

// InterventionContext provides context for suggesting interventions.
type InterventionContext struct {
	Duration     time.Duration `json:"duration"`      // How long quest has been running
	Attempts     int           `json:"attempts"`      // Number of attempts so far
	LastError    string        `json:"last_error"`    // Most recent failure reason
	AgentHistory []Quest       `json:"agent_history"` // Agent's recent quest history
}

// EscalationAttempt records a previous attempt to resolve an escalation.
type EscalationAttempt struct {
	Intervention Intervention `json:"intervention"`
	Timestamp    time.Time    `json:"timestamp"`
	Outcome      string       `json:"outcome"`
}

// EscalationDecision describes how to resolve an escalated quest.
type EscalationDecision struct {
	Resolution   string   `json:"resolution"` // "reassign", "decompose", "complete_by_dm", "cancel"
	NewPartyID   *PartyID `json:"new_party_id,omitempty"`
	SubQuests    []Quest  `json:"sub_quests,omitempty"` // If decomposing
	DMCompletion any      `json:"dm_completion,omitempty"`
	Reasoning    string   `json:"reasoning"`
}

// -----------------------------------------------------------------------------
// Approval Router - For human-in-the-loop decisions
// -----------------------------------------------------------------------------

// ApprovalRouter handles human-in-the-loop approval workflows.
// It routes approval requests to humans and collects their responses.
type ApprovalRouter interface {
	// RequestApproval sends an approval request and waits for response.
	// This blocks until a response is received or context is cancelled.
	RequestApproval(ctx context.Context, req ApprovalRequest) (*ApprovalResponse, error)

	// WatchApprovals subscribes to approval responses for a session.
	// Returns a channel that receives responses as they arrive.
	WatchApprovals(ctx context.Context, filter ApprovalFilter) (<-chan ApprovalResponse, error)

	// GetPendingApprovals returns all pending approval requests for a session.
	GetPendingApprovals(ctx context.Context, sessionID string) ([]ApprovalRequest, error)
}

// ApprovalType categorizes the kind of approval being requested.
type ApprovalType string

// Approval type constants.
const (
	// ApprovalQuestCreate requests approval for quest parameters.
	ApprovalQuestCreate ApprovalType = "quest_create"
	// ApprovalQuestDecomposition requests approval for sub-quest breakdown.
	ApprovalQuestDecomposition ApprovalType = "quest_decomposition"
	// ApprovalPartyFormation requests approval for party composition.
	ApprovalPartyFormation ApprovalType = "party_formation"
	// ApprovalBattleVerdict requests approval/override of battle verdict.
	ApprovalBattleVerdict ApprovalType = "battle_verdict"
	// ApprovalAgentRecruit requests approval for new agent recruitment.
	ApprovalAgentRecruit ApprovalType = "agent_recruit"
	// ApprovalAgentRetire requests approval for agent retirement.
	ApprovalAgentRetire ApprovalType = "agent_retire"
	// ApprovalIntervention requests approval for DM intervention.
	ApprovalIntervention ApprovalType = "intervention"
	// ApprovalEscalation requests decision on escalated quest.
	ApprovalEscalation ApprovalType = "escalation"
)

// ApprovalRequest represents a request for human approval.
type ApprovalRequest struct {
	ID         string            `json:"id"`
	SessionID  string            `json:"session_id"`
	Type       ApprovalType      `json:"type"`
	Title      string            `json:"title"`
	Details    string            `json:"details"`
	Suggestion any               `json:"suggestion,omitempty"` // Pre-computed suggestion (e.g., boids ranking)
	Payload    any               `json:"payload,omitempty"`    // Type-specific data
	Options    []ApprovalOption  `json:"options,omitempty"`    // Available choices
	Metadata   map[string]string `json:"metadata,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	ExpiresAt  *time.Time        `json:"expires_at,omitempty"` // Optional timeout
}

// ApprovalOption represents a choice available in an approval request.
type ApprovalOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	IsDefault   bool   `json:"is_default,omitempty"`
}

// ApprovalResponse contains the human's decision.
type ApprovalResponse struct {
	RequestID   string            `json:"request_id"`
	SessionID   string            `json:"session_id"`
	Approved    bool              `json:"approved"`
	SelectedID  string            `json:"selected_id,omitempty"`  // ID of selected option
	Overrides   map[string]any    `json:"overrides,omitempty"`    // Modified values
	Reason      string            `json:"reason,omitempty"`       // Human-provided reasoning
	RespondedBy string            `json:"responded_by,omitempty"` // Who approved
	Metadata    map[string]string `json:"metadata,omitempty"`
	RespondedAt time.Time         `json:"responded_at"`
}

// ApprovalFilter specifies criteria for filtering approval responses.
type ApprovalFilter struct {
	SessionID string         `json:"session_id,omitempty"`
	Types     []ApprovalType `json:"types,omitempty"`
}

// -----------------------------------------------------------------------------
// Mock Implementations - For testing
// -----------------------------------------------------------------------------

// MockLLMProvider is a test implementation of LLMProvider.
type MockLLMProvider struct {
	DecideQuestParametersFunc func(ctx context.Context, objective string, hints QuestHints) (*QuestDecision, error)
	ReviewDecompositionFunc   func(ctx context.Context, parent Quest, subQuests []Quest) (*DecompositionReview, error)
	EvaluateAgentFunc         func(ctx context.Context, agent Agent, recentQuests []Quest) (*AgentEvaluation, error)
	SuggestInterventionFunc   func(ctx context.Context, quest Quest, context InterventionContext) (*Intervention, error)
	HandleEscalationFunc      func(ctx context.Context, quest Quest, attempts []EscalationAttempt) (*EscalationDecision, error)
}

// DecideQuestParameters implements LLMProvider.
func (m *MockLLMProvider) DecideQuestParameters(ctx context.Context, objective string, hints QuestHints) (*QuestDecision, error) {
	if m.DecideQuestParametersFunc != nil {
		return m.DecideQuestParametersFunc(ctx, objective, hints)
	}
	// Default: use hints or sensible defaults
	decision := &QuestDecision{
		Difficulty:  DifficultyModerate,
		BaseXP:      100,
		ReviewLevel: ReviewStandard,
		Reasoning:   "mock decision",
	}
	if hints.SuggestedDifficulty != nil {
		decision.Difficulty = *hints.SuggestedDifficulty
		decision.BaseXP = DefaultXPForDifficulty(*hints.SuggestedDifficulty)
	}
	if len(hints.SuggestedSkills) > 0 {
		decision.RequiredSkills = hints.SuggestedSkills
	}
	if hints.PreferGuild != nil {
		decision.GuildPriority = hints.PreferGuild
	}
	if hints.RequireHumanReview {
		decision.ReviewLevel = ReviewHuman
	}
	return decision, nil
}

// ReviewDecomposition implements LLMProvider.
func (m *MockLLMProvider) ReviewDecomposition(ctx context.Context, parent Quest, subQuests []Quest) (*DecompositionReview, error) {
	if m.ReviewDecompositionFunc != nil {
		return m.ReviewDecompositionFunc(ctx, parent, subQuests)
	}
	return &DecompositionReview{
		Approved:  true,
		SubQuests: subQuests,
		Feedback:  "mock approval",
	}, nil
}

// EvaluateAgent implements LLMProvider.
func (m *MockLLMProvider) EvaluateAgent(ctx context.Context, agent Agent, recentQuests []Quest) (*AgentEvaluation, error) {
	if m.EvaluateAgentFunc != nil {
		return m.EvaluateAgentFunc(ctx, agent, recentQuests)
	}
	return &AgentEvaluation{
		AgentID:          agent.ID,
		CurrentLevel:     agent.Level,
		RecommendedLevel: agent.Level,
		Recommendation:   "maintain",
	}, nil
}

// SuggestIntervention implements LLMProvider.
func (m *MockLLMProvider) SuggestIntervention(ctx context.Context, quest Quest, ictx InterventionContext) (*Intervention, error) {
	if m.SuggestInterventionFunc != nil {
		return m.SuggestInterventionFunc(ctx, quest, ictx)
	}
	return &Intervention{
		Type:   InterventionAssist,
		Reason: "mock suggestion",
	}, nil
}

// HandleEscalation implements LLMProvider.
func (m *MockLLMProvider) HandleEscalation(ctx context.Context, quest Quest, attempts []EscalationAttempt) (*EscalationDecision, error) {
	if m.HandleEscalationFunc != nil {
		return m.HandleEscalationFunc(ctx, quest, attempts)
	}
	return &EscalationDecision{
		Resolution: "reassign",
		Reasoning:  "mock escalation handling",
	}, nil
}

// MockApprovalRouter is a test implementation of ApprovalRouter.
type MockApprovalRouter struct {
	mu                sync.Mutex
	responses         map[string]*ApprovalResponse // Pre-configured responses by request ID
	pendingRequests   []ApprovalRequest
	autoApprove       bool   // If true, auto-approve all requests
	defaultSelectedID string // Default SelectedID for auto-approved responses
}

// NewMockApprovalRouter creates a new mock approval router.
func NewMockApprovalRouter() *MockApprovalRouter {
	return &MockApprovalRouter{
		responses: make(map[string]*ApprovalResponse),
	}
}

// SetAutoApprove configures auto-approval mode for testing.
func (m *MockApprovalRouter) SetAutoApprove(auto bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.autoApprove = auto
}

// SetSelectedID configures the default SelectedID for auto-approved responses.
func (m *MockApprovalRouter) SetSelectedID(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaultSelectedID = id
}

// SetResponse pre-configures a response for a specific request ID.
func (m *MockApprovalRouter) SetResponse(requestID string, resp *ApprovalResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses[requestID] = resp
}

// RequestApproval implements ApprovalRouter.
func (m *MockApprovalRouter) RequestApproval(ctx context.Context, req ApprovalRequest) (*ApprovalResponse, error) {
	m.mu.Lock()
	m.pendingRequests = append(m.pendingRequests, req)

	// Check for pre-configured response
	if resp, ok := m.responses[req.ID]; ok {
		delete(m.responses, req.ID)
		m.mu.Unlock()
		return resp, nil
	}

	// Auto-approve if configured
	if m.autoApprove {
		selectedID := m.defaultSelectedID
		m.mu.Unlock()
		return &ApprovalResponse{
			RequestID:   req.ID,
			SessionID:   req.SessionID,
			Approved:    true,
			SelectedID:  selectedID,
			Reason:      "auto-approved",
			RespondedAt: time.Now(),
		}, nil
	}

	m.mu.Unlock()

	// Wait for external response or context cancellation
	<-ctx.Done()
	return nil, ctx.Err()
}

// WatchApprovals implements ApprovalRouter.
// Mock returns a closed channel immediately - no context needed for synchronous mock behavior.
func (m *MockApprovalRouter) WatchApprovals(_ context.Context, _ ApprovalFilter) (<-chan ApprovalResponse, error) {
	ch := make(chan ApprovalResponse)
	close(ch)
	return ch, nil
}

// GetPendingApprovals implements ApprovalRouter.
// Mock uses in-memory lookup - no context needed for synchronous local access.
func (m *MockApprovalRouter) GetPendingApprovals(_ context.Context, sessionID string) ([]ApprovalRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var pending []ApprovalRequest
	for _, req := range m.pendingRequests {
		if req.SessionID == sessionID {
			pending = append(pending, req)
		}
	}
	return pending, nil
}

// ClearPending clears all pending requests (for test cleanup).
func (m *MockApprovalRouter) ClearPending() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pendingRequests = nil
}
