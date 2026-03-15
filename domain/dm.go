package domain

import (
	"encoding/json"
	"fmt"
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

// ValidDMMode returns true if the given DMMode is one of the known constants.
func ValidDMMode(mode DMMode) bool {
	switch mode {
	case DMFullAuto, DMAssisted, DMSupervised, DMManual:
		return true
	default:
		return false
	}
}

// =============================================================================
// CHAT MODE - Which DM chat behavior is active
// =============================================================================

// ChatMode determines the behavior of the DM chat endpoint.
type ChatMode string

const (
	// ChatModeConverse is the default mode: safe Q&A, no structured output.
	ChatModeConverse ChatMode = "converse"
	// ChatModeQuest enables quest/chain creation with structured JSON output.
	ChatModeQuest ChatMode = "quest"
)

// ValidChatMode returns true if the given ChatMode is one of the known constants.
func ValidChatMode(m ChatMode) bool {
	switch m {
	case ChatModeConverse, ChatModeQuest:
		return true
	default:
		return false
	}
}

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

	// Token budget (populated by API service when ledger is available)
	TokensUsedHourly  int64   `json:"tokens_used_hourly"`
	TokensLimitHourly int64   `json:"tokens_limit_hourly"`
	TokenBudgetPct    float64 `json:"token_budget_pct"`
	TokenBreaker      string  `json:"token_breaker"`

	// Cost estimation (populated by API service when ledger + pricing is available)
	CostUsedHourlyUSD float64 `json:"cost_used_hourly_usd"`
	CostTotalUSD      float64 `json:"cost_total_usd"`
}

// =============================================================================
// GAME EVENTS
// =============================================================================

// GameEventType categorizes events in the game event stream.
type GameEventType string

// Game event type values covering quests, agents, battles, parties, guilds, and DM actions.
const (
	// EventQuestPosted fires when a quest is added to the board.
	EventQuestPosted    GameEventType = "quest.posted"
	EventQuestClaimed   GameEventType = "quest.claimed"
	EventQuestStarted   GameEventType = "quest.started"
	EventQuestCompleted GameEventType = "quest.completed"
	EventQuestFailed    GameEventType = "quest.failed"
	EventQuestEscalated GameEventType = "quest.escalated"

	// EventAgentRecruited fires when a new agent joins the roster.
	EventAgentRecruited  GameEventType = "agent.recruited"
	EventAgentLevelUp    GameEventType = "agent.level_up"
	EventAgentLevelDown  GameEventType = "agent.level_down"
	EventAgentDeath      GameEventType = "agent.death"
	EventAgentPermadeath GameEventType = "agent.permadeath"
	EventAgentRevived    GameEventType = "agent.revived"

	// EventBattleStarted fires when a boss battle review begins.
	EventBattleStarted GameEventType = "battle.started"
	EventBattleVictory GameEventType = "battle.victory"
	EventBattleDefeat  GameEventType = "battle.defeat"

	// EventPartyFormed fires when agents form a party.
	EventPartyFormed    GameEventType = "party.formed"
	EventPartyDisbanded GameEventType = "party.disbanded"

	// EventGuildCreated fires when a new guild is established.
	EventGuildCreated GameEventType = "guild.created"
	EventGuildJoined  GameEventType = "guild.joined"

	// EventQuestPendingTriage fires when a quest enters triage after exhausting retries.
	EventQuestPendingTriage GameEventType = "quest.pending_triage"
	// EventQuestTriaged fires when a DM triage decision is applied.
	EventQuestTriaged GameEventType = "quest.triaged"

	// EventDMIntervention fires when the DM acts on an ongoing quest.
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
	SpanID string `json:"span_id"`
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

// DM intervention kind values.
const (
	InterventionAssist   InterventionType = "assist"
	InterventionRedirect InterventionType = "redirect"
	InterventionTakeover InterventionType = "takeover"
	InterventionAbort    InterventionType = "abort"
	InterventionAugment  InterventionType = "augment"
	InterventionTriage   InterventionType = "triage"
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

// Approval request kind values.
const (
	ApprovalQuestCreate        ApprovalType = "quest_create"
	ApprovalQuestDecomposition ApprovalType = "quest_decomposition"
	ApprovalPartyFormation     ApprovalType = "party_formation"
	ApprovalBattleVerdict      ApprovalType = "battle_verdict"
	ApprovalAgentRecruit       ApprovalType = "agent_recruit"
	ApprovalAgentRetire        ApprovalType = "agent_retire"
	ApprovalIntervention       ApprovalType = "intervention"
	ApprovalEscalation         ApprovalType = "escalation"
	ApprovalAutonomyClaim      ApprovalType = "autonomy_claim"
	ApprovalAutonomyShop       ApprovalType = "autonomy_shop"
	ApprovalAutonomyGuild      ApprovalType = "autonomy_guild"
	ApprovalAutonomyGuildCreate ApprovalType = "autonomy_guild_create"
	ApprovalAutonomyUse        ApprovalType = "autonomy_use"
	ApprovalFailureTriage      ApprovalType = "failure_triage"
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

// Party composition strategy values.
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
	ReviewLevel         *ReviewLevel     `json:"review_level,omitempty"`
	Budget              float64          `json:"budget"`
	Deadline            string           `json:"deadline,omitempty"`
	PartyRequired       bool             `json:"party_required"`
	MinPartySize        *int             `json:"min_party_size,omitempty"`
}

// =============================================================================
// QUEST SCENARIO - A named execution scenario within a quest brief
// =============================================================================

// QuestScenario is a named sub-goal within a QuestBrief, used to express
// parallelism, sequencing, or conditional paths at the briefing stage.
// Dependencies refer to other scenarios by name within the same brief.
type QuestScenario struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Skills      []string `json:"skills,omitempty"`
	DependsOn   []string `json:"depends_on,omitempty"`
}

// UnmarshalJSON handles LLM output quirks where depends_on or skills may be
// a single string instead of an array (Gemini does this for single values).
func (s *QuestScenario) UnmarshalJSON(data []byte) error {
	// Use a raw struct to avoid infinite recursion.
	var raw struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Skills      json.RawMessage `json:"skills,omitempty"`
		DependsOn   json.RawMessage `json:"depends_on,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	s.Name = raw.Name
	s.Description = raw.Description
	s.Skills = flexStringSlice(raw.Skills)
	s.DependsOn = flexStringSlice(raw.DependsOn)
	return nil
}

// flexStringSlice unmarshals a JSON value that may be a string or []string.
func flexStringSlice(data json.RawMessage) []string {
	if len(data) == 0 {
		return nil
	}
	var arr []string
	if json.Unmarshal(data, &arr) == nil {
		return arr
	}
	var single string
	if json.Unmarshal(data, &single) == nil && single != "" {
		return []string{single}
	}
	return nil
}

// =============================================================================
// DECOMPOSABILITY - Classification of scenario dependency structure
// =============================================================================

// DecomposabilityClass categorises the dependency shape of a quest's scenarios.
type DecomposabilityClass string

const (
	// DecomposableParallel indicates all scenarios are independent (no depends_on).
	DecomposableParallel DecomposabilityClass = "parallel"
	// DecomposableSequential indicates scenarios form a single linear chain.
	DecomposableSequential DecomposabilityClass = "sequential"
	// DecomposableMixed indicates a mix of parallel and sequential dependencies.
	DecomposableMixed DecomposabilityClass = "mixed"
	// DecomposableTrivial indicates 0 or 1 scenarios — no structure to classify.
	DecomposableTrivial DecomposabilityClass = "trivial"
)

// ClassifyDecomposability inspects the scenario dependency graph of a QuestBrief
// and returns the appropriate DecomposabilityClass.
//
// Rules:
//   - 0 or 1 scenarios → Trivial
//   - No scenarios have depends_on → Parallel
//   - Exactly one root and each non-root depends on exactly the immediately
//     preceding scenario (single linear chain) → Sequential
//   - Otherwise → Mixed
func ClassifyDecomposability(brief *QuestBrief) DecomposabilityClass {
	if brief == nil || len(brief.Scenarios) <= 1 {
		return DecomposableTrivial
	}

	if countRoots(brief.Scenarios) == len(brief.Scenarios) {
		// Every scenario is a root — no dependencies at all.
		return DecomposableParallel
	}

	if isLinearChain(brief.Scenarios) {
		return DecomposableSequential
	}

	return DecomposableMixed
}

// countRoots returns the number of scenarios that have no depends_on entries.
func countRoots(scenarios []QuestScenario) int {
	count := 0
	for _, s := range scenarios {
		if len(s.DependsOn) == 0 {
			count++
		}
	}
	return count
}

// isLinearChain returns true when scenarios form a single linear chain
// regardless of declaration order. A linear chain has one root, one tail,
// and every node has at most one predecessor and one successor.
func isLinearChain(scenarios []QuestScenario) bool {
	if countRoots(scenarios) != 1 {
		return false
	}
	// Build adjacency: each node has at most one child (successor).
	// Also verify each non-root has exactly one dependency.
	children := make(map[string]string, len(scenarios))
	for _, s := range scenarios {
		if len(s.DependsOn) == 0 {
			continue // root
		}
		if len(s.DependsOn) != 1 {
			return false // branching dependency — not linear
		}
		parent := s.DependsOn[0]
		if _, hasChild := children[parent]; hasChild {
			return false // parent already has a child — fork in the chain
		}
		children[parent] = s.Name
	}
	return true
}

// MaxParallelWidth computes the largest number of scenarios that can execute
// concurrently — the widest level in a topological ordering of the dependency
// graph. Use this to size party recruitment: a width of 3 means 3 agents can
// work simultaneously, so recruiting fewer wastes parallelism.
//
// Returns 0 for nil/empty scenarios, 1 for a single scenario.
// Assumes the graph is a valid DAG (call ValidateScenarioDependencies first).
func MaxParallelWidth(scenarios []QuestScenario) int {
	n := len(scenarios)
	if n <= 1 {
		return n
	}

	// Build name→index map and adjacency/in-degree.
	idx := make(map[string]int, n)
	for i, s := range scenarios {
		idx[s.Name] = i
	}

	inDegree := make([]int, n)
	adj := make([][]int, n)
	for i, s := range scenarios {
		for _, dep := range s.DependsOn {
			if depIdx, ok := idx[dep]; ok {
				adj[depIdx] = append(adj[depIdx], i)
				inDegree[i]++
			}
		}
	}

	// BFS by topological level — track the widest frontier.
	queue := make([]int, 0, n)
	for i, d := range inDegree {
		if d == 0 {
			queue = append(queue, i)
		}
	}

	maxWidth := 0
	for len(queue) > 0 {
		if len(queue) > maxWidth {
			maxWidth = len(queue)
		}
		next := make([]int, 0, n)
		for _, node := range queue {
			for _, child := range adj[node] {
				inDegree[child]--
				if inDegree[child] == 0 {
					next = append(next, child)
				}
			}
		}
		queue = next
	}

	return maxWidth
}

// =============================================================================
// QUEST BRIEF - Structured quest creation input
// =============================================================================

// QuestBrief is the JSON-friendly input for creating a single quest.
// Both human-authored JSON and chat-assembled quests produce this structure.
//
// Goal replaces the old Description field; Requirements replaces Acceptance.
// Scenarios express named sub-goals and their dependency relationships within
// the quest. Use ClassifyDecomposability to inspect the scenario graph shape.
type QuestBrief struct {
	Name         string           `json:"name,omitempty"`
	Title        string           `json:"title"`
	Goal         string           `json:"goal"`
	Requirements []string         `json:"requirements,omitempty"`
	Scenarios    []QuestScenario  `json:"scenarios,omitempty"`
	Difficulty   *QuestDifficulty `json:"difficulty,omitempty"`
	Skills       []SkillTag       `json:"skills,omitempty"`
	Hints        *QuestHints      `json:"hints,omitempty"`
	DependsOn    []QuestID        `json:"depends_on,omitempty"`
	Repo         string           `json:"repo,omitempty"` // Target repository name
}

// QuestChainBrief defines multiple interdependent quests submitted as one batch.
type QuestChainBrief struct {
	Quests []QuestChainEntry `json:"quests"`
}

// QuestChainEntry is one quest within a chain. DependsOn uses array indices
// (0-based) referring to other entries in the same chain.
//
// Goal replaces Description; Requirements replaces Acceptance; Scenarios
// express named sub-goals within each chain entry.
type QuestChainEntry struct {
	Name         string           `json:"name,omitempty"`
	Title        string           `json:"title"`
	Goal         string           `json:"goal,omitempty"`
	Requirements []string         `json:"requirements,omitempty"`
	Scenarios    []QuestScenario  `json:"scenarios,omitempty"`
	Difficulty   *QuestDifficulty `json:"difficulty,omitempty"`
	Skills       []SkillTag       `json:"skills,omitempty"`
	DependsOn    []int            `json:"depends_on,omitempty"`
	Hints        *QuestHints      `json:"hints,omitempty"`
	Repo         string           `json:"repo,omitempty"` // Target repository name
}

// maxChainSize is the maximum number of quests in a single chain submission.
const maxChainSize = 50

// ValidateQuestBrief checks that a QuestBrief has all required fields.
// When scenarios are present, scenario dependency graph integrity is also verified.
func ValidateQuestBrief(b *QuestBrief) error {
	if b == nil {
		return fmt.Errorf("quest brief is nil")
	}
	if b.Title == "" {
		return fmt.Errorf("quest brief: title is required")
	}
	if b.Goal == "" {
		return fmt.Errorf("quest brief: goal is required")
	}
	if b.Difficulty != nil {
		if err := validateDifficultyRange(*b.Difficulty); err != nil {
			return fmt.Errorf("quest brief: %w", err)
		}
	}
	if len(b.Scenarios) > 0 {
		if err := ValidateScenarioDependencies(b.Scenarios); err != nil {
			return fmt.Errorf("quest brief: %w", err)
		}
	}
	return nil
}

// ValidateScenarioDependencies checks that a slice of QuestScenario values is
// internally consistent:
//   - All scenario names are non-empty
//   - No duplicate names
//   - All depends_on references name an existing scenario in the same slice
//   - No self-references
//   - No cycles (Kahn's algorithm topological sort)
func ValidateScenarioDependencies(scenarios []QuestScenario) error {
	// Single pass: validate names, descriptions, build name→index map.
	idx := make(map[string]int, len(scenarios))
	for i, s := range scenarios {
		if s.Name == "" {
			return fmt.Errorf("scenarios: name must not be empty")
		}
		if s.Description == "" {
			return fmt.Errorf("scenarios: %q description must not be empty", s.Name)
		}
		if _, dup := idx[s.Name]; dup {
			return fmt.Errorf("scenarios: duplicate name %q", s.Name)
		}
		idx[s.Name] = i
	}

	// Validate individual dependency entries.
	for _, s := range scenarios {
		depSeen := make(map[string]bool, len(s.DependsOn))
		for _, dep := range s.DependsOn {
			if dep == s.Name {
				return fmt.Errorf("scenarios: %q cannot depend on itself", s.Name)
			}
			if _, known := idx[dep]; !known {
				return fmt.Errorf("scenarios: %q depends on unknown scenario %q", s.Name, dep)
			}
			if depSeen[dep] {
				return fmt.Errorf("scenarios: %q has duplicate dependency %q", s.Name, dep)
			}
			depSeen[dep] = true
		}
	}

	// Cycle detection via Kahn's algorithm (topological sort).
	n := len(scenarios)
	inDegree := make([]int, n)
	adj := make([][]int, n)
	for i, s := range scenarios {
		for _, dep := range s.DependsOn {
			depIdx := idx[dep]
			adj[depIdx] = append(adj[depIdx], i)
			inDegree[i]++
		}
	}

	queue := make([]int, 0, n)
	for i, d := range inDegree {
		if d == 0 {
			queue = append(queue, i)
		}
	}

	visited := 0
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		visited++
		for _, next := range adj[node] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}

	if visited != n {
		return fmt.Errorf("scenario dependency cycle detected")
	}

	return nil
}

// ValidateQuestChainBrief checks that a QuestChainBrief is well-formed:
// non-empty, all titles present, index bounds valid, no self-references,
// no duplicate deps, valid difficulty, no cycles.
func ValidateQuestChainBrief(chain *QuestChainBrief) error {
	if chain == nil {
		return fmt.Errorf("quest chain brief is nil")
	}
	n := len(chain.Quests)
	if n == 0 {
		return fmt.Errorf("quest chain brief: at least one quest is required")
	}
	if n > maxChainSize {
		return fmt.Errorf("quest chain brief: exceeds maximum of %d quests", maxChainSize)
	}

	for i, entry := range chain.Quests {
		if entry.Title == "" {
			return fmt.Errorf("quest chain brief: quest[%d] title is required", i)
		}
		if entry.Difficulty != nil {
			if err := validateDifficultyRange(*entry.Difficulty); err != nil {
				return fmt.Errorf("quest chain brief: quest[%d] %w", i, err)
			}
		}
		if len(entry.Scenarios) > 0 {
			if err := ValidateScenarioDependencies(entry.Scenarios); err != nil {
				return fmt.Errorf("quest chain brief: quest[%d] %w", i, err)
			}
		}
		seen := make(map[int]bool, len(entry.DependsOn))
		for _, dep := range entry.DependsOn {
			if dep < 0 || dep >= n {
				return fmt.Errorf("quest chain brief: quest[%d] depends_on index %d out of range [0,%d)", i, dep, n)
			}
			if dep == i {
				return fmt.Errorf("quest chain brief: quest[%d] cannot depend on itself", i)
			}
			if seen[dep] {
				return fmt.Errorf("quest chain brief: quest[%d] duplicate depends_on index %d", i, dep)
			}
			seen[dep] = true
		}
	}

	// Cycle detection via topological sort (Kahn's algorithm)
	inDegree := make([]int, n)
	adj := make([][]int, n)
	for i, entry := range chain.Quests {
		for _, dep := range entry.DependsOn {
			adj[dep] = append(adj[dep], i)
			inDegree[i]++
		}
	}

	queue := make([]int, 0, n)
	for i, d := range inDegree {
		if d == 0 {
			queue = append(queue, i)
		}
	}

	visited := 0
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		visited++
		for _, next := range adj[node] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}

	if visited != n {
		return fmt.Errorf("quest chain brief: dependency cycle detected")
	}

	return nil
}

func validateDifficultyRange(d QuestDifficulty) error {
	if d < DifficultyTrivial || d > DifficultyLegendary {
		return fmt.Errorf("difficulty %d out of range [%d,%d]", d, DifficultyTrivial, DifficultyLegendary)
	}
	return nil
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
