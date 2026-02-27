package semdragons

import (
	"context"
)

// =============================================================================
// DUNGEON MASTER - Orchestration that knows it's orchestration
// =============================================================================
// The DM is the only place where top-down control lives. Everything else is
// emergent (quest board pulls, guild formation, party composition).
// The DM can be a human, an LLM, or a hybrid. Full auto = LLM DM.
// =============================================================================

// DungeonMaster is the orchestration interface.
// In full-auto mode, this is backed by a capable LLM.
// In human-in-the-loop mode, some methods route to human approval.
type DungeonMaster interface {
	// --- Session Management ---

	// StartSession begins a new game session (workflow execution context).
	StartSession(ctx context.Context, config SessionConfig) (*Session, error)

	// EndSession wraps up a session, collects final stats.
	EndSession(ctx context.Context, sessionID string) (*SessionSummary, error)

	// --- Quest Management ---

	// CreateQuest crafts a quest from a high-level objective.
	// The DM decides difficulty, required skills, review level, XP rewards.
	CreateQuest(ctx context.Context, objective string, hints QuestHints) (*Quest, error)

	// ReviewQuestDecomposition approves or modifies a party lead's sub-quest breakdown.
	ReviewQuestDecomposition(ctx context.Context, parentID QuestID, subQuests []Quest) ([]Quest, error)

	// --- Agent Management ---

	// RecruitAgent brings a new agent into the world at level 1.
	RecruitAgent(ctx context.Context, config AgentConfig) (*Agent, error)

	// RetireAgent permanently removes an agent (permadeath or manual removal).
	RetireAgent(ctx context.Context, agentID AgentID, reason string) error

	// EvaluateAgent runs an ad-hoc assessment of an agent's current performance.
	EvaluateAgent(ctx context.Context, agentID AgentID) (*AgentEvaluation, error)

	// --- Party Management ---

	// FormParty assembles a party for a quest. The DM picks composition.
	FormParty(ctx context.Context, questID QuestID, strategy PartyStrategy) (*Party, error)

	// --- Intervention ---

	// Intervene allows the DM to step into any ongoing quest.
	// This is the "DM override" - can redirect, assist, or take over.
	Intervene(ctx context.Context, questID QuestID, action Intervention) error

	// HandleEscalation deals with escalated quests (TPK scenarios).
	HandleEscalation(ctx context.Context, questID QuestID) (*EscalationResult, error)

	// HandleBossBattle runs or delegates a boss battle for a completed quest.
	HandleBossBattle(ctx context.Context, questID QuestID, submission interface{}) (*BossBattle, error)

	// --- Observation ---

	// WorldState returns the current state of everything.
	WorldState(ctx context.Context) (*WorldState, error)

	// WatchEvents subscribes to the event stream (backed by semstreams).
	WatchEvents(ctx context.Context, filter EventFilter) (<-chan GameEvent, error)
}

// DMMode determines how much autonomy the DM has.
type DMMode string

const (
	DMFullAuto   DMMode = "full_auto"   // LLM makes all decisions
	DMAssisted   DMMode = "assisted"    // LLM proposes, human approves critical decisions
	DMSupervised DMMode = "supervised"  // Human makes key decisions, LLM handles routine
	DMManual     DMMode = "manual"      // Human DM with LLM as advisor only
)

type SessionConfig struct {
	Mode           DMMode            `json:"mode"`
	Name           string            `json:"name"`
	Description    string            `json:"description"`
	DMModel        string            `json:"dm_model"`        // LLM model for DM decisions
	MaxConcurrent  int               `json:"max_concurrent"`  // Max quests running at once
	AutoEscalate   bool              `json:"auto_escalate"`   // Auto-escalate after max attempts
	TrajectoryMode string            `json:"trajectory_mode"` // semstreams trajectory config
	Metadata       map[string]string `json:"metadata"`
}

type Session struct {
	ID          string        `json:"id"`
	Config      SessionConfig `json:"config"`
	WorldState  *WorldState   `json:"world_state"`
	Active      bool          `json:"active"`
}

type SessionSummary struct {
	SessionID       string  `json:"session_id"`
	QuestsCompleted int     `json:"quests_completed"`
	QuestsFailed    int     `json:"quests_failed"`
	QuestsEscalated int     `json:"quests_escalated"`
	AgentsActive    int     `json:"agents_active"`
	TotalXPAwarded  int64   `json:"total_xp_awarded"`
	AvgQuality      float64 `json:"avg_quality"`
	LevelUps        int     `json:"level_ups"`
	LevelDowns      int     `json:"level_downs"`
	Deaths          int     `json:"deaths"`
}

type QuestHints struct {
	SuggestedDifficulty *QuestDifficulty `json:"suggested_difficulty,omitempty"`
	SuggestedSkills     []SkillTag       `json:"suggested_skills,omitempty"`
	PreferGuild         *GuildID         `json:"prefer_guild,omitempty"`
	RequireHumanReview  bool             `json:"require_human_review"`
	Budget              float64          `json:"budget"`
	Deadline            string           `json:"deadline,omitempty"`
}

type PartyStrategy string

const (
	PartyStrategyBalanced   PartyStrategy = "balanced"    // Mix of skills
	PartyStrategySpecialist PartyStrategy = "specialist"  // All same guild
	PartyStrategyMentor     PartyStrategy = "mentor"      // High-level lead + apprentices
	PartyStrategyMinimal    PartyStrategy = "minimal"     // Smallest viable party
)

type Intervention struct {
	Type    InterventionType `json:"type"`
	Reason  string           `json:"reason"`
	Payload interface{}      `json:"payload,omitempty"`
}

type InterventionType string

const (
	InterventionAssist    InterventionType = "assist"     // Give the agent a hint
	InterventionRedirect  InterventionType = "redirect"   // Change approach
	InterventionTakeover  InterventionType = "takeover"   // DM finishes it
	InterventionAbort     InterventionType = "abort"      // Kill the quest
	InterventionAugment   InterventionType = "augment"    // Add resources/tools
)

type EscalationResult struct {
	QuestID     QuestID `json:"quest_id"`
	Resolution  string  `json:"resolution"`   // "reassigned", "completed_by_dm", "cancelled"
	NewPartyID  *PartyID `json:"new_party_id,omitempty"`
	DMCompleted bool    `json:"dm_completed"` // DM did it themselves
}

type AgentEvaluation struct {
	AgentID          AgentID  `json:"agent_id"`
	CurrentLevel     int      `json:"current_level"`
	RecommendedLevel int      `json:"recommended_level"`
	Strengths        []string `json:"strengths"`
	Weaknesses       []string `json:"weaknesses"`
	Recommendation   string   `json:"recommendation"` // "promote", "maintain", "demote", "retire"
}

// =============================================================================
// WORLD STATE - Everything the DM can see
// =============================================================================

type WorldState struct {
	Agents   []Agent   `json:"agents"`
	Quests   []Quest   `json:"quests"`
	Parties  []Party   `json:"parties"`
	Guilds   []Guild   `json:"guilds"`
	Battles  []BossBattle `json:"battles"`
	Stats    WorldStats   `json:"stats"`
}

type WorldStats struct {
	ActiveAgents    int     `json:"active_agents"`
	IdleAgents      int     `json:"idle_agents"`
	CooldownAgents  int     `json:"cooldown_agents"`
	RetiredAgents   int     `json:"retired_agents"`
	OpenQuests      int     `json:"open_quests"`
	ActiveQuests    int     `json:"active_quests"`
	CompletionRate  float64 `json:"completion_rate"`
	AvgQuality      float64 `json:"avg_quality"`
	ActiveParties   int     `json:"active_parties"`
	ActiveGuilds    int     `json:"active_guilds"`
}

// =============================================================================
// GAME EVENTS - The event stream (maps to semstreams)
// =============================================================================

type GameEventType string

const (
	// Quest lifecycle
	EventQuestPosted    GameEventType = "quest.posted"
	EventQuestClaimed   GameEventType = "quest.claimed"
	EventQuestStarted   GameEventType = "quest.started"
	EventQuestCompleted GameEventType = "quest.completed"
	EventQuestFailed    GameEventType = "quest.failed"
	EventQuestEscalated GameEventType = "quest.escalated"

	// Agent lifecycle
	EventAgentRecruited  GameEventType = "agent.recruited"
	EventAgentLevelUp    GameEventType = "agent.level_up"
	EventAgentLevelDown  GameEventType = "agent.level_down"
	EventAgentDeath      GameEventType = "agent.death"       // Cooldown triggered
	EventAgentPermadeath GameEventType = "agent.permadeath"  // Retired permanently
	EventAgentRevived    GameEventType = "agent.revived"     // Back from cooldown

	// Battle lifecycle
	EventBattleStarted  GameEventType = "battle.started"
	EventBattleVictory  GameEventType = "battle.victory"
	EventBattleDefeat   GameEventType = "battle.defeat"

	// Social
	EventPartyFormed    GameEventType = "party.formed"
	EventPartyDisbanded GameEventType = "party.disbanded"
	EventGuildCreated   GameEventType = "guild.created"
	EventGuildJoined    GameEventType = "guild.joined"

	// DM actions
	EventDMIntervention  GameEventType = "dm.intervention"
	EventDMEscalation    GameEventType = "dm.escalation"
	EventDMSessionStart  GameEventType = "dm.session_start"
	EventDMSessionEnd    GameEventType = "dm.session_end"
)

type GameEvent struct {
	Type      GameEventType      `json:"type"`
	Timestamp int64              `json:"timestamp"` // Unix millis
	SessionID string             `json:"session_id"`
	Data      interface{}        `json:"data"`

	// References for easy filtering
	QuestID  *QuestID  `json:"quest_id,omitempty"`
	AgentID  *AgentID  `json:"agent_id,omitempty"`
	PartyID  *PartyID  `json:"party_id,omitempty"`
	GuildID  *GuildID  `json:"guild_id,omitempty"`
	BattleID *BattleID `json:"battle_id,omitempty"`

	// Semstreams integration
	TrajectoryID string `json:"trajectory_id"`
	SpanID       string `json:"span_id"`
}

type EventFilter struct {
	Types    []GameEventType `json:"types,omitempty"`
	QuestID  *QuestID        `json:"quest_id,omitempty"`
	AgentID  *AgentID        `json:"agent_id,omitempty"`
	GuildID  *GuildID        `json:"guild_id,omitempty"`
}
