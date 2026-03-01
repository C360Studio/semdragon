package autonomy

import (
	"context"
	"testing"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/processor/boidengine"
	"github.com/c360studio/semstreams/component"
)

// =============================================================================
// UNIT TESTS - Action matrix, interval mapping, config, backoff
// =============================================================================
// No Docker required. Run with: go test ./processor/autonomy/...
// =============================================================================

// newTestComponent creates a minimal Component for unit tests (no NATS/graph).
func newTestComponent() *Component {
	cfg := DefaultConfig()
	return &Component{
		config:   &cfg,
		trackers: make(map[string]*agentTracker),
	}
}

func TestActionsForState_Idle(t *testing.T) {
	c := newTestComponent()
	actions := c.actionsForState(semdragons.AgentIdle)
	want := []string{"claim_quest", "use_consumable", "shop", "join_guild"}

	if len(actions) != len(want) {
		t.Fatalf("idle actions: got %d, want %d", len(actions), len(want))
	}

	for i, act := range actions {
		if act.name != want[i] {
			t.Errorf("idle action[%d] = %q, want %q", i, act.name, want[i])
		}
	}
}

func TestActionsForState_OnQuest(t *testing.T) {
	c := newTestComponent()
	actions := c.actionsForState(semdragons.AgentOnQuest)
	want := []string{"claim_concurrent", "shop_strategic", "use_consumable", "join_guild"}

	if len(actions) != len(want) {
		t.Fatalf("on_quest actions: got %d, want %d", len(actions), len(want))
	}

	for i, act := range actions {
		if act.name != want[i] {
			t.Errorf("on_quest action[%d] = %q, want %q", i, act.name, want[i])
		}
	}
}

func TestActionsForState_InBattle(t *testing.T) {
	c := newTestComponent()
	actions := c.actionsForState(semdragons.AgentInBattle)
	want := []string{"use_consumable"}

	if len(actions) != len(want) {
		t.Fatalf("in_battle actions: got %d, want %d", len(actions), len(want))
	}

	if actions[0].name != want[0] {
		t.Errorf("in_battle action[0] = %q, want %q", actions[0].name, want[0])
	}
}

func TestActionsForState_Cooldown(t *testing.T) {
	c := newTestComponent()
	actions := c.actionsForState(semdragons.AgentCooldown)
	want := []string{"use_cooldown_skip", "shop", "join_guild"}

	if len(actions) != len(want) {
		t.Fatalf("cooldown actions: got %d, want %d", len(actions), len(want))
	}

	for i, act := range actions {
		if act.name != want[i] {
			t.Errorf("cooldown action[%d] = %q, want %q", i, act.name, want[i])
		}
	}
}

func TestActionsForState_Retired(t *testing.T) {
	c := newTestComponent()
	actions := c.actionsForState(semdragons.AgentRetired)
	if actions != nil {
		t.Errorf("retired actions: got %v, want nil", actions)
	}
}

func TestActionStubs_AllReturnFalse(t *testing.T) {
	c := newTestComponent()
	stubs := []action{
		// claimQuestAction is no longer a stub — skip it
		c.claimConcurrentAction(),
		c.shopAction(),
		c.shopStrategicAction(),
		c.useConsumableAction(),
		c.useCooldownSkipAction(),
		c.joinGuildAction(),
	}

	agent := &semdragons.Agent{
		ID:     "test.local.game.board1.agent.stub",
		Status: semdragons.AgentIdle,
		Level:  5,
	}
	tracker := &agentTracker{agent: agent}

	for _, stub := range stubs {
		if stub.shouldExecute(agent, tracker) {
			t.Errorf("action %q shouldExecute returned true, want false (stub)", stub.name)
		}
	}
}

func TestClaimQuestAction_ShouldExecute_WithSuggestions(t *testing.T) {
	c := newTestComponent()
	act := c.claimQuestAction()

	agent := &semdragons.Agent{
		ID:     "test.local.game.board1.agent.claim1",
		Status: semdragons.AgentIdle,
		Level:  5,
	}
	tracker := &agentTracker{
		agent: agent,
		suggestions: []boidengine.SuggestedClaim{
			{QuestID: "test.local.game.board1.quest.q1", Score: 3.0},
		},
	}

	if !act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should return true when idle with suggestions")
	}
}

func TestClaimQuestAction_ShouldExecute_NoSuggestions(t *testing.T) {
	c := newTestComponent()
	act := c.claimQuestAction()

	agent := &semdragons.Agent{
		ID:     "test.local.game.board1.agent.claim2",
		Status: semdragons.AgentIdle,
		Level:  5,
	}
	tracker := &agentTracker{agent: agent}

	if act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should return false when no suggestions")
	}
}

func TestClaimQuestAction_ShouldExecute_NotIdle(t *testing.T) {
	c := newTestComponent()
	act := c.claimQuestAction()

	agent := &semdragons.Agent{
		ID:     "test.local.game.board1.agent.claim3",
		Status: semdragons.AgentOnQuest,
		Level:  5,
	}
	tracker := &agentTracker{
		agent: agent,
		suggestions: []boidengine.SuggestedClaim{
			{QuestID: "test.local.game.board1.quest.q1", Score: 3.0},
		},
	}

	if act.shouldExecute(agent, tracker) {
		t.Error("shouldExecute should return false when not idle")
	}
}

func TestIntervalForStatus(t *testing.T) {
	cfg := DefaultConfig()

	tests := []struct {
		status semdragons.AgentStatus
		want   time.Duration
	}{
		{semdragons.AgentIdle, 5 * time.Second},
		{semdragons.AgentOnQuest, 30 * time.Second},
		{semdragons.AgentInBattle, 60 * time.Second},
		{semdragons.AgentCooldown, 15 * time.Second},
		{semdragons.AgentRetired, 0},
	}

	for _, tt := range tests {
		got := cfg.IntervalForStatus(tt.status)
		if got != tt.want {
			t.Errorf("IntervalForStatus(%v) = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.InitialDelayMs != 2000 {
		t.Errorf("InitialDelayMs = %d, want 2000", cfg.InitialDelayMs)
	}
	if cfg.IdleIntervalMs != 5000 {
		t.Errorf("IdleIntervalMs = %d, want 5000", cfg.IdleIntervalMs)
	}
	if cfg.OnQuestIntervalMs != 30000 {
		t.Errorf("OnQuestIntervalMs = %d, want 30000", cfg.OnQuestIntervalMs)
	}
	if cfg.InBattleIntervalMs != 60000 {
		t.Errorf("InBattleIntervalMs = %d, want 60000", cfg.InBattleIntervalMs)
	}
	if cfg.CooldownIntervalMs != 15000 {
		t.Errorf("CooldownIntervalMs = %d, want 15000", cfg.CooldownIntervalMs)
	}
	if cfg.MaxIntervalMs != 60000 {
		t.Errorf("MaxIntervalMs = %d, want 60000", cfg.MaxIntervalMs)
	}
	if cfg.BackoffFactor != 1.5 {
		t.Errorf("BackoffFactor = %f, want 1.5", cfg.BackoffFactor)
	}
}

func TestBackoffOnlyIdle(t *testing.T) {
	cfg := DefaultConfig()
	comp := &Component{
		config:   &cfg,
		trackers: make(map[string]*agentTracker),
	}

	idleInterval := cfg.IntervalForStatus(semdragons.AgentIdle)

	// Set up idle agent tracker
	comp.trackers["idle-agent"] = &agentTracker{
		agent:    &semdragons.Agent{Status: semdragons.AgentIdle},
		interval: idleInterval,
	}

	// Set up on_quest agent tracker
	questInterval := cfg.IntervalForStatus(semdragons.AgentOnQuest)
	comp.trackers["quest-agent"] = &agentTracker{
		agent:    &semdragons.Agent{Status: semdragons.AgentOnQuest},
		interval: questInterval,
	}

	// Backoff both
	comp.backoffHeartbeat("idle-agent")
	comp.backoffHeartbeat("quest-agent")

	// Idle agent should have grown interval
	idleTracker := comp.trackers["idle-agent"]
	expectedIdleInterval := time.Duration(float64(idleInterval) * cfg.BackoffFactor)
	if idleTracker.interval != expectedIdleInterval {
		t.Errorf("idle backoff: interval = %v, want %v", idleTracker.interval, expectedIdleInterval)
	}

	// On-quest agent should be unchanged
	questTracker := comp.trackers["quest-agent"]
	if questTracker.interval != questInterval {
		t.Errorf("quest backoff: interval = %v, want %v (unchanged)", questTracker.interval, questInterval)
	}
}

func TestBackoffCapsAtMax(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxIntervalMs = 10000 // 10s cap for easy testing
	comp := &Component{
		config:   &cfg,
		trackers: make(map[string]*agentTracker),
	}

	// Start with interval already near max
	comp.trackers["agent"] = &agentTracker{
		agent:    &semdragons.Agent{Status: semdragons.AgentIdle},
		interval: 9 * time.Second,
	}

	comp.backoffHeartbeat("agent")

	maxInterval := time.Duration(cfg.MaxIntervalMs) * time.Millisecond
	tracker := comp.trackers["agent"]
	if tracker.interval > maxInterval {
		t.Errorf("backoff exceeded max: got %v, max %v", tracker.interval, maxInterval)
	}
	if tracker.interval != maxInterval {
		t.Errorf("backoff should cap at max: got %v, want %v", tracker.interval, maxInterval)
	}
}

// =============================================================================
// PORT AND SCHEMA TESTS - No infrastructure required
// =============================================================================

func TestComponent_InputOutputPorts(t *testing.T) {
	comp := &Component{}

	inputs := comp.InputPorts()
	if len(inputs) != 2 {
		t.Fatalf("InputPorts: got %d, want 2", len(inputs))
	}

	// Verify input port names
	portNames := map[string]bool{}
	for _, p := range inputs {
		portNames[p.Name] = true
	}
	if !portNames["agent-state-watch"] {
		t.Error("missing input port: agent-state-watch")
	}
	if !portNames["boid-suggestions"] {
		t.Error("missing input port: boid-suggestions")
	}

	outputs := comp.OutputPorts()
	if len(outputs) != 3 {
		t.Fatalf("OutputPorts: got %d, want 3", len(outputs))
	}

	outputNames := map[string]bool{}
	for _, p := range outputs {
		outputNames[p.Name] = true
	}
	if !outputNames["autonomy-evaluated"] {
		t.Error("missing output port: autonomy-evaluated")
	}
	if !outputNames["autonomy-idle"] {
		t.Error("missing output port: autonomy-idle")
	}
	if !outputNames["claim-state"] {
		t.Error("missing output port: claim-state")
	}
}

func TestComponent_ConfigSchema(t *testing.T) {
	comp := &Component{}
	schema := comp.ConfigSchema()

	// Check required fields
	for _, req := range []string{"org", "platform", "board"} {
		found := false
		for _, r := range schema.Required {
			if r == req {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("required field %q not in ConfigSchema.Required", req)
		}
	}

	// Check heartbeat properties exist
	heartbeatProps := []string{
		"initial_delay_ms", "idle_interval_ms", "on_quest_interval_ms",
		"in_battle_interval_ms", "cooldown_interval_ms", "max_interval_ms",
		"backoff_factor",
	}
	for _, prop := range heartbeatProps {
		if _, ok := schema.Properties[prop]; !ok {
			t.Errorf("missing heartbeat property %q in ConfigSchema", prop)
		}
	}
}

// =============================================================================
// PAYLOAD GRAPHABLE TESTS
// =============================================================================

func TestEvaluatedPayload_Graphable(t *testing.T) {
	now := time.Now()
	p := &EvaluatedPayload{
		AgentID:     "test.local.game.board1.agent.eval1",
		AgentStatus: "idle",
		ActionTaken: "none",
		Interval:    5 * time.Second,
		Timestamp:   now,
	}

	// EntityID returns the agent ID string.
	if got := p.EntityID(); got != "test.local.game.board1.agent.eval1" {
		t.Errorf("EntityID() = %q, want agent ID", got)
	}

	// Triples returns exactly 3 triples.
	triples := p.Triples()
	if len(triples) != 3 {
		t.Fatalf("Triples() returned %d triples, want 3", len(triples))
	}

	// Every triple must reference the agent as subject.
	for i, tr := range triples {
		if tr.Subject != "test.local.game.board1.agent.eval1" {
			t.Errorf("triple[%d].Subject = %q, want agent ID", i, tr.Subject)
		}
	}

	// All three expected predicates must be present.
	predicates := make(map[string]bool, len(triples))
	for _, tr := range triples {
		predicates[tr.Predicate] = true
	}
	for _, want := range []string{
		"agent.autonomy.status",
		"agent.autonomy.action",
		"agent.autonomy.interval_ms",
	} {
		if !predicates[want] {
			t.Errorf("missing triple predicate %q", want)
		}
	}

	// Schema returns the correct domain and category.
	s := p.Schema()
	if s.Domain != "semdragons" || s.Category != "autonomy.evaluated" {
		t.Errorf("Schema() = {Domain:%q Category:%q}, want semdragons/autonomy.evaluated", s.Domain, s.Category)
	}

	// Validate succeeds for a fully populated payload.
	if err := p.Validate(); err != nil {
		t.Errorf("Validate() unexpected error: %v", err)
	}

	// Validate fails when AgentID is absent.
	missingID := &EvaluatedPayload{Timestamp: now}
	if err := missingID.Validate(); err == nil {
		t.Error("Validate() should fail with empty AgentID")
	}

	// Validate fails when Timestamp is the zero value.
	missingTS := &EvaluatedPayload{AgentID: "test.local.game.board1.agent.eval1"}
	if err := missingTS.Validate(); err == nil {
		t.Error("Validate() should fail with zero Timestamp")
	}
}

func TestIdlePayload_Graphable(t *testing.T) {
	now := time.Now()
	p := &IdlePayload{
		AgentID:       "test.local.game.board1.agent.idle1",
		IdleDuration:  30 * time.Second,
		HasSuggestion: false,
		BackoffMs:     7500,
		Timestamp:     now,
	}

	// EntityID returns the agent ID string.
	if got := p.EntityID(); got != "test.local.game.board1.agent.idle1" {
		t.Errorf("EntityID() = %q, want agent ID", got)
	}

	// Triples returns exactly 3 triples.
	triples := p.Triples()
	if len(triples) != 3 {
		t.Fatalf("Triples() returned %d triples, want 3", len(triples))
	}

	// Every triple must reference the agent as subject.
	for i, tr := range triples {
		if tr.Subject != "test.local.game.board1.agent.idle1" {
			t.Errorf("triple[%d].Subject = %q, want agent ID", i, tr.Subject)
		}
	}

	// All three expected predicates must be present.
	predicates := make(map[string]bool, len(triples))
	for _, tr := range triples {
		predicates[tr.Predicate] = true
	}
	for _, want := range []string{
		"agent.autonomy.idle_duration_ms",
		"agent.autonomy.has_suggestion",
		"agent.autonomy.backoff_ms",
	} {
		if !predicates[want] {
			t.Errorf("missing triple predicate %q", want)
		}
	}

	// Schema returns the correct domain and category.
	s := p.Schema()
	if s.Domain != "semdragons" || s.Category != "autonomy.idle" {
		t.Errorf("Schema() = {Domain:%q Category:%q}, want semdragons/autonomy.idle", s.Domain, s.Category)
	}

	// Validate succeeds for a fully populated payload.
	if err := p.Validate(); err != nil {
		t.Errorf("Validate() unexpected error: %v", err)
	}

	// Validate fails when AgentID is absent.
	missingID := &IdlePayload{Timestamp: now}
	if err := missingID.Validate(); err == nil {
		t.Error("Validate() should fail with empty AgentID")
	}

	// Validate fails when Timestamp is the zero value.
	missingTS := &IdlePayload{AgentID: "test.local.game.board1.agent.idle1"}
	if err := missingTS.Validate(); err == nil {
		t.Error("Validate() should fail with zero Timestamp")
	}
}

// =============================================================================
// HEARTBEAT RESET TESTS
// =============================================================================

func TestResetHeartbeatInterval_ResetsBackedOffInterval(t *testing.T) {
	cfg := DefaultConfig()
	comp := &Component{
		config:   &cfg,
		trackers: make(map[string]*agentTracker),
	}

	idleInterval := cfg.IntervalForStatus(semdragons.AgentIdle)

	// Create a tracker whose interval has been backed off twice.
	backedOff := time.Duration(float64(idleInterval) * cfg.BackoffFactor * cfg.BackoffFactor)
	comp.trackers["agent"] = &agentTracker{
		agent:    &semdragons.Agent{Status: semdragons.AgentIdle},
		interval: backedOff,
	}

	comp.resetHeartbeatInterval("agent")

	if got := comp.trackers["agent"].interval; got != idleInterval {
		t.Errorf("interval after reset = %v, want base idle interval %v", got, idleInterval)
	}
}

func TestResetHeartbeatInterval_NoopWhenAlreadyAtBase(t *testing.T) {
	cfg := DefaultConfig()
	comp := &Component{
		config:   &cfg,
		trackers: make(map[string]*agentTracker),
	}

	idleInterval := cfg.IntervalForStatus(semdragons.AgentIdle)

	// Tracker already at the base idle interval — reset should leave it unchanged.
	comp.trackers["agent"] = &agentTracker{
		agent:    &semdragons.Agent{Status: semdragons.AgentIdle},
		interval: idleInterval,
	}

	comp.resetHeartbeatInterval("agent")

	if got := comp.trackers["agent"].interval; got != idleInterval {
		t.Errorf("interval after no-op reset = %v, want %v (unchanged)", got, idleInterval)
	}
}

// =============================================================================
// COOLDOWN EXPIRY TESTS
// =============================================================================

func TestCheckCooldownExpiry_NilCooldownUntil(t *testing.T) {
	cfg := DefaultConfig()
	comp := &Component{
		config: &cfg,
	}

	// An agent in cooldown with no CooldownUntil set must return false
	// without touching the graph client (which is nil here).
	agent := &semdragons.Agent{
		ID:            "test.local.game.board1.agent.nilcd",
		Status:        semdragons.AgentCooldown,
		CooldownUntil: nil,
	}

	ctx := context.Background()
	if comp.checkCooldownExpiry(ctx, agent) {
		t.Error("checkCooldownExpiry should return false when CooldownUntil is nil")
	}
}

func TestCheckCooldownExpiry_FutureCooldownUntil(t *testing.T) {
	cfg := DefaultConfig()
	comp := &Component{
		config: &cfg,
	}

	// Cooldown that expires one hour from now should not be cleared.
	future := time.Now().Add(1 * time.Hour)
	agent := &semdragons.Agent{
		ID:            "test.local.game.board1.agent.futurecd",
		Status:        semdragons.AgentCooldown,
		CooldownUntil: &future,
	}

	ctx := context.Background()
	if comp.checkCooldownExpiry(ctx, agent) {
		t.Error("checkCooldownExpiry should return false when cooldown has not expired")
	}
}

// =============================================================================
// FACTORY / REGISTRATION TESTS
// =============================================================================

func TestFactory_NilNATSClient(t *testing.T) {
	deps := component.Dependencies{} // NATSClient is nil by default
	_, err := Factory(nil, deps)
	if err == nil {
		t.Error("Factory should return an error when NATSClient is nil")
	}
}

func TestNewFromConfig_NilNATSClient(t *testing.T) {
	cfg := DefaultConfig()
	deps := component.Dependencies{} // NATSClient is nil by default
	_, err := NewFromConfig(cfg, deps)
	if err == nil {
		t.Error("NewFromConfig should return an error when NATSClient is nil")
	}
}

// =============================================================================
// ACTIONS FOR STATE - UNKNOWN STATUS
// =============================================================================

func TestActionsForState_Unknown(t *testing.T) {
	c := newTestComponent()
	actions := c.actionsForState("some_unknown_status")
	if actions != nil {
		t.Errorf("unknown status actions: got %v, want nil", actions)
	}
}
