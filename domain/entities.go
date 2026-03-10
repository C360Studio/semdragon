package domain

import "time"

// =============================================================================
// ENTITY STRUCTS
// =============================================================================

// =============================================================================
// QUEST
// =============================================================================

// Quest represents a unit of work on the quest board.
type Quest struct {
	ID          QuestID         `json:"id"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Status      QuestStatus     `json:"status"`
	Difficulty  QuestDifficulty `json:"difficulty"`

	// Requirements
	RequiredSkills []SkillTag `json:"required_skills"`
	RequiredTools  []string   `json:"required_tools"` // Tool IDs
	MinTier        TrustTier  `json:"min_tier"`
	PartyRequired  bool       `json:"party_required"` // Too big for solo
	MinPartySize   int        `json:"min_party_size"`

	// Rewards
	BaseXP  int64 `json:"base_xp"`
	BonusXP int64 `json:"bonus_xp"` // For exceptional quality
	GuildXP int64 `json:"guild_xp"` // XP toward guild reputation

	// Execution context - the actual work
	Input        any              `json:"input"`                   // Quest payload
	Output       any              `json:"output"`                  // Result when completed
	Constraints  QuestConstraints `json:"constraints"`
	AllowedTools []string         `json:"allowed_tools,omitempty"` // Tool whitelist for execution (empty = all allowed)

	// Quest chain / decomposition
	ParentQuest  *QuestID  `json:"parent_quest,omitempty"`  // If this is a sub-quest
	SubQuests    []QuestID `json:"sub_quests,omitempty"`    // If decomposed
	DecomposedBy *AgentID  `json:"decomposed_by,omitempty"` // Party lead who broke it down
	DependsOn    []QuestID `json:"depends_on,omitempty"`    // Sibling dependencies (waits for these to complete)
	Acceptance   []string  `json:"acceptance,omitempty"`    // Domain-flexible acceptance criteria

	// Quest spec (from QuestBrief)
	Goal                 string               `json:"goal,omitempty"`
	Requirements         []string             `json:"requirements,omitempty"`
	Scenarios            []QuestScenario      `json:"scenarios,omitempty"`
	DecomposabilityClass DecomposabilityClass `json:"decomposability_class,omitempty"`

	// Assignment
	ClaimedBy     *AgentID `json:"claimed_by,omitempty"`
	PartyID       *PartyID `json:"party_id,omitempty"`
	GuildPriority *GuildID `json:"guild_priority,omitempty"` // Guild gets first dibs

	// Lifecycle
	PostedAt    time.Time  `json:"posted_at"`
	ClaimedAt   *time.Time `json:"claimed_at,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Deadline    *time.Time `json:"deadline,omitempty"`

	// Failure tracking
	Attempts      int         `json:"attempts"`
	MaxAttempts   int         `json:"max_attempts"`
	Escalated     bool        `json:"escalated"`
	FailureReason string      `json:"failure_reason,omitempty"`
	FailureType   FailureType `json:"failure_type,omitempty"`

	// Failure recovery (populated by DM triage at terminal boundary)
	FailureHistory  []FailureRecord `json:"failure_history,omitempty"`
	RecoveryPath    RecoveryPath    `json:"recovery_path,omitempty"`
	FailureAnalysis string          `json:"failure_analysis,omitempty"`
	SalvagedOutput  any             `json:"salvaged_output,omitempty"`
	AntiPatterns    []string        `json:"anti_patterns,omitempty"`

	// Verdict (set on completion after boss battle)
	Verdict *BattleVerdict `json:"verdict,omitempty"`

	// Duration of quest execution (from start to completion)
	Duration time.Duration `json:"duration,omitempty"`

	// Observability — LoopID is the agentic-loop execution ID, also the key
	// in AGENT_TRAJECTORIES KV bucket for trajectory lookup.
	LoopID string `json:"loop_id,omitempty"`

	// DAG execution state (party quest decomposition).
	// Stored as any to avoid importing questdagexec types into domain.
	// Parent quest fields:
	DAGExecutionID    string `json:"dag_execution_id,omitempty"`
	DAGDefinition     any    `json:"dag_definition,omitempty"`      // QuestDAG JSON
	DAGNodeQuestIDs   any    `json:"dag_node_quest_ids,omitempty"`  // map[string]string
	DAGNodeStates     any    `json:"dag_node_states,omitempty"`     // map[string]string
	DAGNodeAssignees  any    `json:"dag_node_assignees,omitempty"`  // map[string]string
	DAGCompletedNodes any    `json:"dag_completed_nodes,omitempty"` // []string
	DAGFailedNodes    any    `json:"dag_failed_nodes,omitempty"`    // []string
	DAGNodeRetries    any    `json:"dag_node_retries,omitempty"`    // map[string]int

	// Sub-quest DAG fields:
	DAGNodeID         string `json:"dag_node_id,omitempty"`
	DAGClarifications any    `json:"dag_clarifications,omitempty"` // []ClarificationExchange

	// DM clarification exchanges (non-DAG quests or parent party quests).
	// Stored as any to keep domain package free of processor-type imports.
	DMClarifications any `json:"dm_clarifications,omitempty"` // []ClarificationExchange

	// Context metadata — populated by questbridge after prompt assembly.
	// Shows what went into the context window for this quest execution.
	ContextTokenCount int      `json:"context_token_count,omitempty"`
	ContextSources    []string `json:"context_sources,omitempty"`  // Fragment IDs used in assembly
	ContextEntities   []string `json:"context_entities,omitempty"` // Entity IDs referenced in context
}

// ClarificationExchange records a single clarification Q&A round between
// an agent and a DM (or party lead). Used for both DM clarifications on
// standalone/parent quests and DAG clarifications on sub-quests.
type ClarificationExchange struct {
	Question string    `json:"question"`
	Answer   string    `json:"answer"`
	AskedAt  time.Time `json:"asked_at"`
}

// PrimarySkill returns the first required skill for this quest, or empty if none.
// Used by the executor to build tier+skill capability keys for model resolution.
func (q *Quest) PrimarySkill() SkillTag {
	if len(q.RequiredSkills) > 0 {
		return q.RequiredSkills[0]
	}
	return ""
}

// QuestConstraints defines limits and requirements for quest execution.
type QuestConstraints struct {
	MaxDuration   time.Duration `json:"max_duration"`
	MaxCost       float64       `json:"max_cost"`
	MaxTokens     int           `json:"max_tokens"`
	RequireReview bool          `json:"require_review"`
	ReviewLevel   ReviewLevel   `json:"review_level"`
}

// FailureType categorizes quest failures.
type FailureType string

const (
	// FailureQuality indicates the quest output did not meet quality standards.
	FailureQuality FailureType = "quality"
	// FailureTimeout indicates the quest exceeded its time limit.
	FailureTimeout FailureType = "timeout"
	// FailureError indicates an unexpected error during execution.
	FailureError FailureType = "error"
	// FailureAbandoned indicates the agent abandoned the quest.
	FailureAbandoned FailureType = "abandoned"
)

// RecoveryPath categorizes the DM's triage decision for a failed quest.
type RecoveryPath string

const (
	// RecoverySalvage preserves partial work and enriches retry context.
	RecoverySalvage RecoveryPath = "salvage"
	// RecoveryTPK clears output but injects anti-pattern warnings.
	RecoveryTPK RecoveryPath = "tpk"
	// RecoveryEscalate requires human DM attention regardless of auto mode.
	RecoveryEscalate RecoveryPath = "escalate"
	// RecoveryTerminal marks the quest as truly impossible.
	RecoveryTerminal RecoveryPath = "terminal"
)

// FailureRecord captures one attempt's failure for DM triage context.
type FailureRecord struct {
	Attempt       int         `json:"attempt"`
	FailureType   FailureType `json:"failure_type"`
	FailureReason string      `json:"failure_reason"`
	Output        any         `json:"output,omitempty"`
	AgentID       AgentID     `json:"agent_id,omitempty"`
	LoopID        string      `json:"loop_id,omitempty"`
	TriageVerdict string      `json:"triage_verdict,omitempty"`
	Timestamp     time.Time   `json:"timestamp"`
}

// BattleVerdict holds the outcome of a boss battle.
// This type lives in domain because Quest.Verdict references it,
// making it a shared contract between questboard and bossbattle processors.
type BattleVerdict struct {
	Passed          bool    `json:"passed"`
	QualityScore    float64 `json:"quality_score"`
	XPAwarded       int64   `json:"xp_awarded"`
	XPPenalty       int64   `json:"xp_penalty"`
	Feedback    string `json:"feedback"`
	LevelChange int    `json:"level_change"`
}
