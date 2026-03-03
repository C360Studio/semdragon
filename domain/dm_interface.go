package domain

import "context"

// DungeonMaster is the orchestration interface.
// In full-auto mode, this is backed by a capable LLM.
// In human-in-the-loop mode, some methods route to human approval.
//
// Note: Agent, Party, and BossBattle types live in their owning processor
// packages (agentprogression, partycoord, bossbattle). Methods that return
// these types use any until callers are migrated to the concrete processor types.
type DungeonMaster interface {
	// --- Session Management ---
	StartSession(ctx context.Context, config SessionConfig) (*Session, error)
	EndSession(ctx context.Context, sessionID string) (*SessionSummary, error)

	// --- Quest Management ---
	CreateQuest(ctx context.Context, objective string, hints QuestHints) (*Quest, error)
	ReviewQuestDecomposition(ctx context.Context, parentID QuestID, subQuests []Quest) ([]Quest, error)

	// --- Agent Management ---
	// RecruitAgent creates a new agent. Returns the concrete agent type from agentprogression package.
	RecruitAgent(ctx context.Context, config AgentConfig) (any, error)
	RetireAgent(ctx context.Context, agentID AgentID, reason string) error
	EvaluateAgent(ctx context.Context, agentID AgentID) (*AgentEvaluation, error)

	// --- Party Management ---
	// FormParty creates a new party. Returns the concrete party type from partycoord package.
	FormParty(ctx context.Context, questID QuestID, strategy PartyStrategy) (any, error)

	// --- Intervention ---
	Intervene(ctx context.Context, questID QuestID, action Intervention) error
	HandleEscalation(ctx context.Context, questID QuestID) (*EscalationResult, error)
	// HandleBossBattle returns the concrete boss battle type from bossbattle package.
	HandleBossBattle(ctx context.Context, questID QuestID, submission any) (any, error)

	// --- Observation ---
	WorldState(ctx context.Context) (*WorldState, error)
	WatchEvents(ctx context.Context, filter EventFilter) (<-chan GameEvent, error)
}

// AgentConfig holds the actual implementation details behind the RPG facade.
// This stub lives in domain for the DungeonMaster interface; the canonical definition
// is in processor/agentprogression.
type AgentConfig struct {
	Provider     string            `json:"provider"`
	Model        string            `json:"model"`
	SystemPrompt string            `json:"system_prompt"`
	Temperature  float64           `json:"temperature"`
	MaxTokens    int               `json:"max_tokens"`
	Metadata     map[string]string `json:"metadata"`
}

// EscalationResult describes how an escalated quest was resolved.
type EscalationResult struct {
	QuestID     QuestID  `json:"quest_id"`
	Resolution  string   `json:"resolution"` // "reassigned", "completed_by_dm", "cancelled"
	NewPartyID  *PartyID `json:"new_party_id,omitempty"`
	DMCompleted bool     `json:"dm_completed"` // DM did it themselves
}

// =============================================================================
// TYPED ACCESSORS - extract concrete types from WorldState's []any slices
//
// WorldState.Quests contains Quest values, Guilds contains Guild values, etc.
// These accessors handle both value and pointer types for resilience.
// =============================================================================

// TypedQuests extracts Quest values from a WorldState's Quests slice.
func TypedQuests(ws *WorldState) []Quest {
	if ws == nil {
		return nil
	}
	quests := make([]Quest, 0, len(ws.Quests))
	for _, q := range ws.Quests {
		switch v := q.(type) {
		case Quest:
			quests = append(quests, v)
		case *Quest:
			quests = append(quests, *v)
		}
	}
	return quests
}

// TypedGuilds extracts Guild values from a WorldState's Guilds slice.
func TypedGuilds(ws *WorldState) []Guild {
	if ws == nil {
		return nil
	}
	guilds := make([]Guild, 0, len(ws.Guilds))
	for _, g := range ws.Guilds {
		switch v := g.(type) {
		case Guild:
			guilds = append(guilds, v)
		case *Guild:
			guilds = append(guilds, *v)
		}
	}
	return guilds
}
