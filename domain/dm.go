package domain

import (
	"time"
)

// =============================================================================
// DM MODE - How much autonomy the DM has
// =============================================================================

// DMMode determines how much autonomy the DM has.
type DMMode string

const (
	// DMFullAuto indicates the LLM makes all decisions.
	DMFullAuto DMMode = "full_auto"
	// DMAssisted indicates the LLM proposes, human approves critical decisions.
	DMAssisted DMMode = "assisted"
	// DMSupervised indicates humans make key decisions, LLM handles routine.
	DMSupervised DMMode = "supervised"
	// DMManual indicates human DM with LLM as advisor only.
	DMManual DMMode = "manual"
)

// =============================================================================
// SESSION TYPES
// =============================================================================

// SessionConfig holds configuration for a DM session.
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

// Session represents an active DM session.
type Session struct {
	ID         string        `json:"id"`
	Config     SessionConfig `json:"config"`
	WorldState *WorldState   `json:"world_state"`
	Active     bool          `json:"active"`
}

// SessionSummary contains aggregate statistics for a completed session.
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

// =============================================================================
// WORLD STATE
// =============================================================================

// WorldState contains the complete state of the game world.
// Note: Uses any slices to avoid circular dependencies with
// entity types defined in their owning processors.
type WorldState struct {
	Agents  []any      `json:"agents"`
	Quests  []any      `json:"quests"`
	Parties []any      `json:"parties"`
	Guilds  []any      `json:"guilds"`
	Battles []any      `json:"battles"`
	Stats   WorldStats `json:"stats"`
}

// WorldStats contains aggregate statistics about the game world.
type WorldStats struct {
	ActiveAgents   int     `json:"active_agents"`
	IdleAgents     int     `json:"idle_agents"`
	CooldownAgents int     `json:"cooldown_agents"`
	RetiredAgents  int     `json:"retired_agents"`
	OpenQuests     int     `json:"open_quests"`
	ActiveQuests   int     `json:"active_quests"`
	CompletionRate float64 `json:"completion_rate"`
	AvgQuality     float64 `json:"avg_quality"`
	ActiveParties  int     `json:"active_parties"`
	ActiveGuilds   int     `json:"active_guilds"`
}

// =============================================================================
// GAME EVENTS
// =============================================================================

// GameEventType categorizes events in the game event stream.
type GameEventType string

const (
	// Quest events
	EventQuestPosted    GameEventType = "quest.posted"
	EventQuestClaimed   GameEventType = "quest.claimed"
	EventQuestStarted   GameEventType = "quest.started"
	EventQuestCompleted GameEventType = "quest.completed"
	EventQuestFailed    GameEventType = "quest.failed"
	EventQuestEscalated GameEventType = "quest.escalated"

	// Agent events
	EventAgentRecruited  GameEventType = "agent.recruited"
	EventAgentLevelUp    GameEventType = "agent.level_up"
	EventAgentLevelDown  GameEventType = "agent.level_down"
	EventAgentDeath      GameEventType = "agent.death"
	EventAgentPermadeath GameEventType = "agent.permadeath"
	EventAgentRevived    GameEventType = "agent.revived"

	// Battle events
	EventBattleStarted GameEventType = "battle.started"
	EventBattleVictory GameEventType = "battle.victory"
	EventBattleDefeat  GameEventType = "battle.defeat"

	// Party events
	EventPartyFormed    GameEventType = "party.formed"
	EventPartyDisbanded GameEventType = "party.disbanded"

	// Guild events
	EventGuildCreated GameEventType = "guild.created"
	EventGuildJoined  GameEventType = "guild.joined"

	// DM events
	EventDMIntervention GameEventType = "dm.intervention"
	EventDMEscalation   GameEventType = "dm.escalation"
	EventDMSessionStart GameEventType = "dm.session_start"
	EventDMSessionEnd   GameEventType = "dm.session_end"
)

// GameEvent represents an event in the game event stream.
type GameEvent struct {
	Type      GameEventType `json:"type"`
	Timestamp int64         `json:"timestamp"` // Unix millis
	SessionID string        `json:"session_id"`
	Data      any           `json:"data"`

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

// EventFilter specifies criteria for filtering game events.
type EventFilter struct {
	Types   []GameEventType `json:"types,omitempty"`
	QuestID *QuestID        `json:"quest_id,omitempty"`
	AgentID *AgentID        `json:"agent_id,omitempty"`
	GuildID *GuildID        `json:"guild_id,omitempty"`
}

// =============================================================================
// INTERVENTION TYPES
// =============================================================================

// InterventionType categorizes the kind of DM intervention.
type InterventionType string

const (
	InterventionAssist   InterventionType = "assist"
	InterventionRedirect InterventionType = "redirect"
	InterventionTakeover InterventionType = "takeover"
	InterventionAbort    InterventionType = "abort"
	InterventionAugment  InterventionType = "augment"
)

// Intervention represents a DM action on an ongoing quest.
type Intervention struct {
	Type    InterventionType `json:"type"`
	Reason  string           `json:"reason"`
	Payload any              `json:"payload,omitempty"`
}

// InterventionContext provides context for suggesting interventions.
type InterventionContext struct {
	Duration     time.Duration `json:"duration"`
	Attempts     int           `json:"attempts"`
	LastError    string        `json:"last_error"`
	AgentHistory []any         `json:"agent_history"` // Quest history
}

// EscalationAttempt records a previous attempt to resolve an escalation.
type EscalationAttempt struct {
	Intervention Intervention `json:"intervention"`
	Timestamp    time.Time    `json:"timestamp"`
	Outcome      string       `json:"outcome"`
}

// =============================================================================
// APPROVAL TYPES
// =============================================================================

// ApprovalType categorizes the kind of approval being requested.
type ApprovalType string

const (
	ApprovalQuestCreate        ApprovalType = "quest_create"
	ApprovalQuestDecomposition ApprovalType = "quest_decomposition"
	ApprovalPartyFormation     ApprovalType = "party_formation"
	ApprovalBattleVerdict      ApprovalType = "battle_verdict"
	ApprovalAgentRecruit       ApprovalType = "agent_recruit"
	ApprovalAgentRetire        ApprovalType = "agent_retire"
	ApprovalIntervention       ApprovalType = "intervention"
	ApprovalEscalation         ApprovalType = "escalation"
)

// ApprovalRequest represents a request for human approval.
type ApprovalRequest struct {
	ID         string            `json:"id"`
	SessionID  string            `json:"session_id"`
	Type       ApprovalType      `json:"type"`
	Title      string            `json:"title"`
	Details    string            `json:"details"`
	Suggestion any               `json:"suggestion,omitempty"`
	Payload    any               `json:"payload,omitempty"`
	Options    []ApprovalOption  `json:"options,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	ExpiresAt  *time.Time        `json:"expires_at,omitempty"`
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
	SelectedID  string            `json:"selected_id,omitempty"`
	Overrides   map[string]any    `json:"overrides,omitempty"`
	Reason      string            `json:"reason,omitempty"`
	RespondedBy string            `json:"responded_by,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	RespondedAt time.Time         `json:"responded_at"`
}

// ApprovalFilter specifies criteria for filtering approval responses.
type ApprovalFilter struct {
	SessionID string         `json:"session_id,omitempty"`
	Types     []ApprovalType `json:"types,omitempty"`
}

// =============================================================================
// PARTY STRATEGY
// =============================================================================

// PartyStrategy determines how a party is composed.
type PartyStrategy string

const (
	PartyStrategyBalanced   PartyStrategy = "balanced"
	PartyStrategySpecialist PartyStrategy = "specialist"
	PartyStrategyMentor     PartyStrategy = "mentor"
	PartyStrategyMinimal    PartyStrategy = "minimal"
)

// =============================================================================
// QUEST HINTS
// =============================================================================

// QuestHints provides optional guidance for quest creation.
type QuestHints struct {
	SuggestedDifficulty *QuestDifficulty `json:"suggested_difficulty,omitempty"`
	SuggestedSkills     []SkillTag       `json:"suggested_skills,omitempty"`
	PreferGuild         *GuildID         `json:"prefer_guild,omitempty"`
	RequireHumanReview  bool             `json:"require_human_review"`
	Budget              float64          `json:"budget"`
	Deadline            string           `json:"deadline,omitempty"`
}

// =============================================================================
// AGENT EVALUATION
// =============================================================================

// AgentEvaluation contains a performance assessment of an agent.
type AgentEvaluation struct {
	AgentID          AgentID  `json:"agent_id"`
	CurrentLevel     int      `json:"current_level"`
	RecommendedLevel int      `json:"recommended_level"`
	Strengths        []string `json:"strengths"`
	Weaknesses       []string `json:"weaknesses"`
	Recommendation   string   `json:"recommendation"`
}
