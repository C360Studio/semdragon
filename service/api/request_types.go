package api

import "github.com/c360studio/semdragons/domain"

// =============================================================================
// REQUEST BODY TYPES — Named structs for OpenAPI schema generation
// =============================================================================
// These types mirror the anonymous structs decoded in handlers.go.
// They exist so that reflect.TypeOf() can produce named schemas in the OpenAPI spec.

// CreateQuestRequest is the request body for POST /quests.
type CreateQuestRequest struct {
	Objective string            `json:"objective" description:"The quest objective/title"`
	Hints     *CreateQuestHints `json:"hints,omitempty" description:"Optional hints for quest configuration"`
}

// CreateQuestHints provides optional configuration hints when creating a quest.
type CreateQuestHints struct {
	SuggestedDifficulty *int     `json:"suggested_difficulty,omitempty" description:"Difficulty level 0-5"`
	SuggestedSkills     []string `json:"suggested_skills,omitempty" description:"Skill tags to require"`
	PreferGuild         *string  `json:"prefer_guild,omitempty" description:"Guild ID for priority routing"`
	RequireHumanReview  bool     `json:"require_human_review" description:"Whether to require human review"`
	ReviewLevel         *int     `json:"review_level,omitempty" description:"Review level 0-3"`
	Budget              float64  `json:"budget" description:"Cost budget for the quest"`
	Deadline            string   `json:"deadline,omitempty" description:"ISO 8601 deadline"`
	PartyRequired       bool     `json:"party_required" description:"Whether the quest requires a party"`
	MinPartySize        *int     `json:"min_party_size,omitempty" description:"Minimum party size (2-5)"`
}

// CreateQuestChainRequest is the request body for POST /quests/chain.
// Uses the domain.QuestChainBrief type directly.
type CreateQuestChainRequest = domain.QuestChainBrief

// ClaimQuestRequest is the request body for POST /quests/{id}/claim.
type ClaimQuestRequest struct {
	AgentID string `json:"agent_id" description:"ID of the agent claiming the quest"`
}

// SubmitQuestRequest is the request body for POST /quests/{id}/submit.
type SubmitQuestRequest struct {
	Output string `json:"output" description:"The quest result output"`
}

// FailQuestRequest is the request body for POST /quests/{id}/fail.
type FailQuestRequest struct {
	Reason string `json:"reason,omitempty" description:"Reason for failure"`
}

// AbandonQuestRequest is the request body for POST /quests/{id}/abandon.
type AbandonQuestRequest struct {
	Reason string `json:"reason,omitempty" description:"Reason for abandoning"`
}

// RecruitAgentRequest is the request body for POST /agents.
type RecruitAgentRequest struct {
	Name        string   `json:"name" description:"Unique agent name"`
	DisplayName string   `json:"display_name,omitempty" description:"Character display name"`
	Skills      []string `json:"skills,omitempty" description:"Initial skill tags"`
	IsNPC       bool     `json:"is_npc" description:"Whether this is an NPC agent"`
	Level       *int     `json:"level,omitempty" description:"Initial level (1-20, default 1). Higher levels grant higher trust tiers."`
}

// PurchaseItemRequest is the request body for POST /store/purchase.
type PurchaseItemRequest struct {
	AgentID string `json:"agent_id" description:"ID of the purchasing agent"`
	ItemID  string `json:"item_id" description:"ID of the item to purchase"`
}

// UseConsumableRequest is the request body for POST /agents/{id}/inventory/use.
type UseConsumableRequest struct {
	ConsumableID string `json:"consumable_id" description:"ID of the consumable to use"`
	QuestID      string `json:"quest_id,omitempty" description:"Quest to apply the effect to"`
}

// CreateReviewRequest is the request body for POST /reviews.
type CreateReviewRequest struct {
	QuestID    string  `json:"quest_id" description:"Quest being reviewed"`
	PartyID    *string `json:"party_id,omitempty" description:"Party ID if applicable"`
	LeaderID   string  `json:"leader_id" description:"Leader agent ID"`
	MemberID   string  `json:"member_id" description:"Member agent ID"`
	IsSoloTask bool    `json:"is_solo_task" description:"Whether this is a solo task review"`
}

// SubmitReviewRequest is the request body for POST /reviews/{id}/submit.
type SubmitReviewRequest struct {
	ReviewerID  string              `json:"reviewer_id" description:"ID of the reviewing agent"`
	Ratings     domain.ReviewRatings `json:"ratings" description:"Ratings for each criterion (1-5)"`
	Explanation string              `json:"explanation,omitempty" description:"Required if average rating < 3.0"`
}

// DMChatRequest is the request body for POST /dm/chat.
type DMChatRequest struct {
	Message   string              `json:"message" description:"User message to the DM"`
	Mode      string              `json:"mode,omitempty" description:"Chat mode: converse or quest"`
	Context   []DMChatContextRef  `json:"context,omitempty" description:"Entity references for context"`
	History   []DMChatHistoryItem `json:"history,omitempty" description:"Previous conversation turns"`
	SessionID string              `json:"session_id,omitempty" description:"Hex session ID for multi-turn"`
}

// DMChatContextRef references a game entity for DM context.
type DMChatContextRef struct {
	Type string `json:"type" description:"Entity type: agent, quest, battle, guild"`
	ID   string `json:"id" description:"Entity ID"`
}

// DMChatHistoryItem is a previous message in the DM conversation.
type DMChatHistoryItem struct {
	Role    string `json:"role" description:"Message role: user or dm"`
	Content string `json:"content" description:"Message content"`
}

// SetTokenBudgetRequest is the request body for POST /board/tokens/budget.
type SetTokenBudgetRequest struct {
	GlobalHourlyLimit int64 `json:"global_hourly_limit" description:"New hourly token limit (0 = unlimited)"`
}

// =============================================================================
// RESPONSE TYPES — Named structs for responses that use anonymous types in handlers
// =============================================================================

// WorldStateResponse is a properly typed version of domain.WorldState for OpenAPI.
// The domain type uses []any to avoid circular imports; this provides typed fields.
type WorldStateResponse struct {
	Agents  []any           `json:"agents" description:"List of agent entities"`
	Quests  []any           `json:"quests" description:"List of quest entities"`
	Parties []any           `json:"parties" description:"List of party entities"`
	Guilds  []any           `json:"guilds" description:"List of guild entities"`
	Battles []any           `json:"battles" description:"List of battle entities"`
	Stats   domain.WorldStats `json:"stats" description:"Aggregate board statistics"`
}

// DMChatResponse is the response body for POST /dm/chat.
type DMChatResponse struct {
	Message    string                  `json:"message" description:"DM response text"`
	Mode       string                  `json:"mode" description:"Active chat mode"`
	QuestBrief *domain.QuestBrief      `json:"quest_brief,omitempty" description:"Extracted quest brief if detected"`
	QuestChain *domain.QuestChainBrief `json:"quest_chain,omitempty" description:"Extracted quest chain if detected"`
	ToolsUsed  []string                `json:"tools_used,omitempty" description:"Tool names used during this turn"`
	SessionID  string                  `json:"session_id" description:"Session ID for multi-turn"`
	TraceInfo  TraceInfoResponse       `json:"trace_info" description:"Observability trace context"`
}

// TraceInfoResponse holds trace context for a response.
type TraceInfoResponse struct {
	TraceID      string `json:"trace_id,omitempty" description:"Trace ID"`
	SpanID       string `json:"span_id,omitempty" description:"Span ID"`
	ParentSpanID string `json:"parent_span_id,omitempty" description:"Parent span ID"`
}

// PurchaseResponse is the response body for POST /store/purchase.
type PurchaseResponse struct {
	Success      bool   `json:"success" description:"Whether the purchase succeeded"`
	Item         any    `json:"item,omitempty" description:"Purchased store item"`
	XPSpent      int64  `json:"xp_spent,omitempty" description:"XP spent on purchase"`
	XPRemaining  int64  `json:"xp_remaining,omitempty" description:"Remaining XP balance"`
	Inventory    any    `json:"inventory,omitempty" description:"Updated agent inventory"`
	Error        string `json:"error,omitempty" description:"Error message if failed"`
}

// UseConsumableResponse is the response body for POST /agents/{id}/inventory/use.
type UseConsumableResponse struct {
	Success       bool  `json:"success" description:"Whether the consumable was used"`
	Remaining     int   `json:"remaining,omitempty" description:"Remaining count of this consumable"`
	ActiveEffects []any `json:"active_effects,omitempty" description:"Currently active effects"`
	Error         string `json:"error,omitempty" description:"Error message if failed"`
}

// BoardStatusResponse is the response body for board control endpoints.
type BoardStatusResponse struct {
	Paused   bool    `json:"paused" description:"Whether the board is currently paused"`
	PausedAt *string `json:"paused_at" description:"RFC 3339 timestamp when board was paused, or null"`
	PausedBy *string `json:"paused_by" description:"Identifier of who paused the board, or null"`
}

// ModelResolveResponse is the response body for GET /models?resolve=capability.
type ModelResolveResponse struct {
	Capability    string   `json:"capability" description:"The requested capability key"`
	EndpointName  string   `json:"endpoint_name" description:"Resolved endpoint name"`
	Model         string   `json:"model,omitempty" description:"Model identifier at the resolved endpoint"`
	Provider      string   `json:"provider,omitempty" description:"Provider type (openai, ollama, anthropic, etc.)"`
	FallbackChain []string `json:"fallback_chain,omitempty" description:"Ordered fallback endpoint names for this capability"`
}

// ModelEndpointSummary describes a single model endpoint in the registry.
type ModelEndpointSummary struct {
	Name            string `json:"name" description:"Endpoint name"`
	Provider        string `json:"provider" description:"Provider type"`
	Model           string `json:"model" description:"Model identifier"`
	MaxTokens       int    `json:"max_tokens" description:"Maximum context window size"`
	SupportsTools   bool   `json:"supports_tools" description:"Whether the endpoint supports tool calling"`
	ReasoningEffort string `json:"reasoning_effort,omitempty" description:"Reasoning effort level (none, low, medium, high)"`
}

// ModelRegistrySummary is the response body for GET /models (no query params).
type ModelRegistrySummary struct {
	Endpoints    []ModelEndpointSummary `json:"endpoints" description:"All configured model endpoints"`
	Capabilities []string              `json:"capabilities" description:"All configured capability keys"`
}
