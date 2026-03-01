package semdragons

import (
	"context"

	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// DM TYPE ALIASES - domain/ is the single source of truth
// =============================================================================

// DMMode determines how much autonomy the DM has.
type DMMode = domain.DMMode

// DMFullAuto and related constants define DM operation modes.
const (
	DMFullAuto   = domain.DMFullAuto
	DMAssisted   = domain.DMAssisted
	DMSupervised = domain.DMSupervised
	DMManual     = domain.DMManual
)

// SessionConfig holds configuration for a DM session.
type SessionConfig = domain.SessionConfig

// SessionSummary contains aggregate statistics for a completed session.
type SessionSummary = domain.SessionSummary

// GameEventType categorizes events in the game event stream.
type GameEventType = domain.GameEventType

// EventQuestPosted and related constants define game event types.
const (
	EventQuestPosted    = domain.EventQuestPosted
	EventQuestClaimed   = domain.EventQuestClaimed
	EventQuestStarted   = domain.EventQuestStarted
	EventQuestCompleted = domain.EventQuestCompleted
	EventQuestFailed    = domain.EventQuestFailed
	EventQuestEscalated = domain.EventQuestEscalated

	EventAgentRecruited  = domain.EventAgentRecruited
	EventAgentLevelUp    = domain.EventAgentLevelUp
	EventAgentLevelDown  = domain.EventAgentLevelDown
	EventAgentDeath      = domain.EventAgentDeath
	EventAgentPermadeath = domain.EventAgentPermadeath
	EventAgentRevived    = domain.EventAgentRevived

	EventBattleStarted = domain.EventBattleStarted
	EventBattleVictory = domain.EventBattleVictory
	EventBattleDefeat  = domain.EventBattleDefeat

	EventPartyFormed    = domain.EventPartyFormed
	EventPartyDisbanded = domain.EventPartyDisbanded
	EventGuildCreated   = domain.EventGuildCreated
	EventGuildJoined    = domain.EventGuildJoined

	EventDMIntervention = domain.EventDMIntervention
	EventDMEscalation   = domain.EventDMEscalation
	EventDMSessionStart = domain.EventDMSessionStart
	EventDMSessionEnd   = domain.EventDMSessionEnd
)

// GameEvent represents an event in the game event stream.
type GameEvent = domain.GameEvent

// EventFilter specifies criteria for filtering game events.
type EventFilter = domain.EventFilter

// InterventionType categorizes the kind of DM intervention.
type InterventionType = domain.InterventionType

// InterventionAssist and related constants define intervention types.
const (
	InterventionAssist   = domain.InterventionAssist
	InterventionRedirect = domain.InterventionRedirect
	InterventionTakeover = domain.InterventionTakeover
	InterventionAbort    = domain.InterventionAbort
	InterventionAugment  = domain.InterventionAugment
)

// Intervention represents a DM action on an ongoing quest.
type Intervention = domain.Intervention

// QuestHints provides optional guidance for quest creation.
type QuestHints = domain.QuestHints

// AgentEvaluation contains a performance assessment of an agent.
type AgentEvaluation = domain.AgentEvaluation

// PartyStrategy determines how a party is composed.
type PartyStrategy = domain.PartyStrategy

// PartyStrategyBalanced and related constants define party strategy values.
const (
	PartyStrategyBalanced   = domain.PartyStrategyBalanced
	PartyStrategySpecialist = domain.PartyStrategySpecialist
	PartyStrategyMentor     = domain.PartyStrategyMentor
	PartyStrategyMinimal    = domain.PartyStrategyMinimal
)

// ApprovalType categorizes the kind of approval being requested.
type ApprovalType = domain.ApprovalType

// ApprovalQuestCreate and related constants define approval type values.
const (
	ApprovalQuestCreate        = domain.ApprovalQuestCreate
	ApprovalQuestDecomposition = domain.ApprovalQuestDecomposition
	ApprovalPartyFormation     = domain.ApprovalPartyFormation
	ApprovalBattleVerdict      = domain.ApprovalBattleVerdict
	ApprovalAgentRecruit       = domain.ApprovalAgentRecruit
	ApprovalAgentRetire        = domain.ApprovalAgentRetire
	ApprovalIntervention       = domain.ApprovalIntervention
	ApprovalEscalation         = domain.ApprovalEscalation
)

// ApprovalRequest represents a request for human approval.
type ApprovalRequest = domain.ApprovalRequest

// ApprovalOption represents a choice available in an approval request.
type ApprovalOption = domain.ApprovalOption

// ApprovalResponse contains the human's decision.
type ApprovalResponse = domain.ApprovalResponse

// ApprovalFilter specifies criteria for filtering approval responses.
type ApprovalFilter = domain.ApprovalFilter

// InterventionContext provides context for suggesting interventions.
type InterventionContext = domain.InterventionContext

// EscalationAttempt records a previous attempt to resolve an escalation.
type EscalationAttempt = domain.EscalationAttempt

// =============================================================================
// ROOT-OWNED TYPES (unique to root package)
// =============================================================================

// DungeonMaster is the orchestration interface.
// In full-auto mode, this is backed by a capable LLM.
// In human-in-the-loop mode, some methods route to human approval.
type DungeonMaster interface {
	// --- Session Management ---
	StartSession(ctx context.Context, config SessionConfig) (*Session, error)
	EndSession(ctx context.Context, sessionID string) (*SessionSummary, error)

	// --- Quest Management ---
	CreateQuest(ctx context.Context, objective string, hints QuestHints) (*Quest, error)
	ReviewQuestDecomposition(ctx context.Context, parentID QuestID, subQuests []Quest) ([]Quest, error)

	// --- Agent Management ---
	RecruitAgent(ctx context.Context, config AgentConfig) (*Agent, error)
	RetireAgent(ctx context.Context, agentID AgentID, reason string) error
	EvaluateAgent(ctx context.Context, agentID AgentID) (*AgentEvaluation, error)

	// --- Party Management ---
	FormParty(ctx context.Context, questID QuestID, strategy PartyStrategy) (*Party, error)

	// --- Intervention ---
	Intervene(ctx context.Context, questID QuestID, action Intervention) error
	HandleEscalation(ctx context.Context, questID QuestID) (*EscalationResult, error)
	HandleBossBattle(ctx context.Context, questID QuestID, submission any) (*BossBattle, error)

	// --- Observation ---
	WorldState(ctx context.Context) (*WorldState, error)
	WatchEvents(ctx context.Context, filter EventFilter) (<-chan GameEvent, error)
}

// Session represents an active DM session.
type Session = domain.Session

// WorldState contains the complete state of the game world.
// Uses []any slices to avoid circular deps between domain and root entity types.
// Use the Typed* accessor functions to extract concrete entity types.
type WorldState = domain.WorldState

// WorldStats contains aggregate statistics about the game world.
type WorldStats = domain.WorldStats

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
// WorldState.Agents contains Agent values, Quests contains Quest values, etc.
// The dm_worldstate processor stores typed values in []any slices.
// These accessors handle both value and pointer types for resilience.
// =============================================================================

// TypedAgents extracts Agent values from a WorldState's Agents slice.
func TypedAgents(ws *WorldState) []Agent {
	if ws == nil {
		return nil
	}
	agents := make([]Agent, 0, len(ws.Agents))
	for _, a := range ws.Agents {
		switch v := a.(type) {
		case Agent:
			agents = append(agents, v)
		case *Agent:
			agents = append(agents, *v)
		}
	}
	return agents
}

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

// TypedParties extracts Party values from a WorldState's Parties slice.
func TypedParties(ws *WorldState) []Party {
	if ws == nil {
		return nil
	}
	parties := make([]Party, 0, len(ws.Parties))
	for _, p := range ws.Parties {
		switch v := p.(type) {
		case Party:
			parties = append(parties, v)
		case *Party:
			parties = append(parties, *v)
		}
	}
	return parties
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

// TypedBattles extracts BossBattle values from a WorldState's Battles slice.
func TypedBattles(ws *WorldState) []BossBattle {
	if ws == nil {
		return nil
	}
	battles := make([]BossBattle, 0, len(ws.Battles))
	for _, b := range ws.Battles {
		switch v := b.(type) {
		case BossBattle:
			battles = append(battles, v)
		case *BossBattle:
			battles = append(battles, *v)
		}
	}
	return battles
}
