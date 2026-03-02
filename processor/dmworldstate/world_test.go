package dmworldstate

// =============================================================================
// UNIT TESTS - DM WorldState: config, aggregator construction, stats computation
// =============================================================================
// No Docker or NATS required. Run with: go test ./processor/dmworldstate/ -v
// =============================================================================

import (
	"log/slog"
	"testing"

	semdragons "github.com/c360studio/semdragons"
)

// =============================================================================
// DEFAULT CONFIG TESTS
// =============================================================================

func TestDefaultConfig_SensibleDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Org != "default" {
		t.Errorf("Org = %q, want %q", cfg.Org, "default")
	}
	if cfg.Platform != "local" {
		t.Errorf("Platform = %q, want %q", cfg.Platform, "local")
	}
	if cfg.Board != "main" {
		t.Errorf("Board = %q, want %q", cfg.Board, "main")
	}
	if cfg.MaxEntitiesPerQuery != 1000 {
		t.Errorf("MaxEntitiesPerQuery = %d, want 1000", cfg.MaxEntitiesPerQuery)
	}
	if cfg.RefreshIntervalSec != 60 {
		t.Errorf("RefreshIntervalSec = %d, want 60", cfg.RefreshIntervalSec)
	}
}

func TestDefaultConfig_PositiveValues(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MaxEntitiesPerQuery <= 0 {
		t.Errorf("MaxEntitiesPerQuery must be positive, got %d", cfg.MaxEntitiesPerQuery)
	}
	if cfg.RefreshIntervalSec <= 0 {
		t.Errorf("RefreshIntervalSec must be positive, got %d", cfg.RefreshIntervalSec)
	}
}

// =============================================================================
// NEW WORLD STATE AGGREGATOR TESTS
// =============================================================================

func TestNewWorldStateAggregator_ClampZeroMaxEntities(t *testing.T) {
	agg := NewWorldStateAggregator(nil, 0, slog.Default())

	if agg.maxEntities != 1000 {
		t.Errorf("maxEntities = %d, want 1000 (clamped from 0)", agg.maxEntities)
	}
}

func TestNewWorldStateAggregator_ClampNegativeMaxEntities(t *testing.T) {
	agg := NewWorldStateAggregator(nil, -500, slog.Default())

	if agg.maxEntities != 1000 {
		t.Errorf("maxEntities = %d, want 1000 (clamped from -500)", agg.maxEntities)
	}
}

func TestNewWorldStateAggregator_PositiveMaxEntitiesUnchanged(t *testing.T) {
	agg := NewWorldStateAggregator(nil, 250, slog.Default())

	if agg.maxEntities != 250 {
		t.Errorf("maxEntities = %d, want 250 (positive value should be kept)", agg.maxEntities)
	}
}

func TestNewWorldStateAggregator_NilLoggerUsesDefault(t *testing.T) {
	// A nil logger must not panic; NewWorldStateAggregator substitutes slog.Default().
	agg := NewWorldStateAggregator(nil, 100, nil)

	if agg.logger == nil {
		t.Error("logger should not be nil when nil is passed (should fall back to slog.Default())")
	}
}

func TestNewWorldStateAggregator_ExplicitLogger(t *testing.T) {
	logger := slog.Default()
	agg := NewWorldStateAggregator(nil, 100, logger)

	if agg.logger == nil {
		t.Error("logger should not be nil when an explicit logger is provided")
	}
}

// =============================================================================
// COMPUTE WORLD STATS TESTS
// =============================================================================

// newTestAggregator creates a minimal WorldStateAggregator suitable for
// calling computeWorldStats without a real NATS connection.
func newTestAggregator() *WorldStateAggregator {
	return NewWorldStateAggregator(nil, 1000, slog.Default())
}

func TestComputeWorldStats_AllEmpty(t *testing.T) {
	agg := newTestAggregator()
	stats := agg.computeWorldStats(
		[]*semdragons.Agent{},
		[]semdragons.Quest{},
		[]semdragons.Party{},
		[]semdragons.Guild{},
	)

	if stats.ActiveAgents != 0 {
		t.Errorf("ActiveAgents = %d, want 0", stats.ActiveAgents)
	}
	if stats.IdleAgents != 0 {
		t.Errorf("IdleAgents = %d, want 0", stats.IdleAgents)
	}
	if stats.CooldownAgents != 0 {
		t.Errorf("CooldownAgents = %d, want 0", stats.CooldownAgents)
	}
	if stats.RetiredAgents != 0 {
		t.Errorf("RetiredAgents = %d, want 0", stats.RetiredAgents)
	}
	if stats.OpenQuests != 0 {
		t.Errorf("OpenQuests = %d, want 0", stats.OpenQuests)
	}
	if stats.ActiveQuests != 0 {
		t.Errorf("ActiveQuests = %d, want 0", stats.ActiveQuests)
	}
	if stats.CompletionRate != 0.0 {
		t.Errorf("CompletionRate = %f, want 0.0", stats.CompletionRate)
	}
	if stats.AvgQuality != 0.0 {
		t.Errorf("AvgQuality = %f, want 0.0", stats.AvgQuality)
	}
	if stats.ActiveParties != 0 {
		t.Errorf("ActiveParties = %d, want 0", stats.ActiveParties)
	}
	if stats.ActiveGuilds != 0 {
		t.Errorf("ActiveGuilds = %d, want 0", stats.ActiveGuilds)
	}
}

// =============================================================================
// AGENT STATUS BRANCHING
// =============================================================================

func TestComputeWorldStats_IdleAgent_CountsActiveAndIdle(t *testing.T) {
	agg := newTestAggregator()
	agents := []*semdragons.Agent{
		{ID: "a1", Status: semdragons.AgentIdle},
	}

	stats := agg.computeWorldStats(agents, nil, nil, nil)

	// Idle counts toward both ActiveAgents and IdleAgents.
	if stats.ActiveAgents != 1 {
		t.Errorf("ActiveAgents = %d, want 1", stats.ActiveAgents)
	}
	if stats.IdleAgents != 1 {
		t.Errorf("IdleAgents = %d, want 1", stats.IdleAgents)
	}
	if stats.CooldownAgents != 0 {
		t.Errorf("CooldownAgents = %d, want 0", stats.CooldownAgents)
	}
	if stats.RetiredAgents != 0 {
		t.Errorf("RetiredAgents = %d, want 0", stats.RetiredAgents)
	}
}

func TestComputeWorldStats_OnQuestAgent_CountsActiveOnly(t *testing.T) {
	agg := newTestAggregator()
	agents := []*semdragons.Agent{
		{ID: "a1", Status: semdragons.AgentOnQuest},
	}

	stats := agg.computeWorldStats(agents, nil, nil, nil)

	if stats.ActiveAgents != 1 {
		t.Errorf("ActiveAgents = %d, want 1", stats.ActiveAgents)
	}
	if stats.IdleAgents != 0 {
		t.Errorf("IdleAgents = %d, want 0 (on_quest does not increment idle)", stats.IdleAgents)
	}
}

func TestComputeWorldStats_InBattleAgent_CountsActiveOnly(t *testing.T) {
	agg := newTestAggregator()
	agents := []*semdragons.Agent{
		{ID: "a1", Status: semdragons.AgentInBattle},
	}

	stats := agg.computeWorldStats(agents, nil, nil, nil)

	if stats.ActiveAgents != 1 {
		t.Errorf("ActiveAgents = %d, want 1", stats.ActiveAgents)
	}
	if stats.IdleAgents != 0 {
		t.Errorf("IdleAgents = %d, want 0 (in_battle does not increment idle)", stats.IdleAgents)
	}
}

func TestComputeWorldStats_CooldownAgent_CountsCooldownOnly(t *testing.T) {
	agg := newTestAggregator()
	agents := []*semdragons.Agent{
		{ID: "a1", Status: semdragons.AgentCooldown},
	}

	stats := agg.computeWorldStats(agents, nil, nil, nil)

	if stats.CooldownAgents != 1 {
		t.Errorf("CooldownAgents = %d, want 1", stats.CooldownAgents)
	}
	if stats.ActiveAgents != 0 {
		t.Errorf("ActiveAgents = %d, want 0 (cooldown is not active)", stats.ActiveAgents)
	}
}

func TestComputeWorldStats_RetiredAgent_CountsRetiredOnly(t *testing.T) {
	agg := newTestAggregator()
	agents := []*semdragons.Agent{
		{ID: "a1", Status: semdragons.AgentRetired},
	}

	stats := agg.computeWorldStats(agents, nil, nil, nil)

	if stats.RetiredAgents != 1 {
		t.Errorf("RetiredAgents = %d, want 1", stats.RetiredAgents)
	}
	if stats.ActiveAgents != 0 {
		t.Errorf("ActiveAgents = %d, want 0 (retired is not active)", stats.ActiveAgents)
	}
}

func TestComputeWorldStats_NilAgentInSlice_IsSkipped(t *testing.T) {
	agg := newTestAggregator()
	// A nil pointer in the slice must not panic; it must be silently skipped.
	agents := []*semdragons.Agent{
		nil,
		{ID: "a2", Status: semdragons.AgentIdle},
		nil,
	}

	stats := agg.computeWorldStats(agents, nil, nil, nil)

	// Only the non-nil idle agent should be counted.
	if stats.ActiveAgents != 1 {
		t.Errorf("ActiveAgents = %d, want 1 (nil agents skipped)", stats.ActiveAgents)
	}
	if stats.IdleAgents != 1 {
		t.Errorf("IdleAgents = %d, want 1 (nil agents skipped)", stats.IdleAgents)
	}
}

func TestComputeWorldStats_MixedAgentStatuses(t *testing.T) {
	agg := newTestAggregator()
	agents := []*semdragons.Agent{
		{ID: "idle-1", Status: semdragons.AgentIdle},
		{ID: "idle-2", Status: semdragons.AgentIdle},
		{ID: "on-quest", Status: semdragons.AgentOnQuest},
		{ID: "in-battle", Status: semdragons.AgentInBattle},
		{ID: "cooldown", Status: semdragons.AgentCooldown},
		{ID: "retired", Status: semdragons.AgentRetired},
	}

	stats := agg.computeWorldStats(agents, nil, nil, nil)

	// idle(2) + on_quest(1) + in_battle(1) = 4 active
	if stats.ActiveAgents != 4 {
		t.Errorf("ActiveAgents = %d, want 4", stats.ActiveAgents)
	}
	if stats.IdleAgents != 2 {
		t.Errorf("IdleAgents = %d, want 2", stats.IdleAgents)
	}
	if stats.CooldownAgents != 1 {
		t.Errorf("CooldownAgents = %d, want 1", stats.CooldownAgents)
	}
	if stats.RetiredAgents != 1 {
		t.Errorf("RetiredAgents = %d, want 1", stats.RetiredAgents)
	}
}

// =============================================================================
// QUEST STATUS BRANCHING
// =============================================================================

func TestComputeWorldStats_PostedQuest_CountsOpen(t *testing.T) {
	agg := newTestAggregator()
	quests := []semdragons.Quest{
		{ID: "q1", Status: semdragons.QuestPosted},
	}

	stats := agg.computeWorldStats(nil, quests, nil, nil)

	if stats.OpenQuests != 1 {
		t.Errorf("OpenQuests = %d, want 1", stats.OpenQuests)
	}
	if stats.ActiveQuests != 0 {
		t.Errorf("ActiveQuests = %d, want 0", stats.ActiveQuests)
	}
}

func TestComputeWorldStats_ClaimedQuest_CountsActive(t *testing.T) {
	agg := newTestAggregator()
	quests := []semdragons.Quest{
		{ID: "q1", Status: semdragons.QuestClaimed},
	}

	stats := agg.computeWorldStats(nil, quests, nil, nil)

	if stats.ActiveQuests != 1 {
		t.Errorf("ActiveQuests = %d, want 1", stats.ActiveQuests)
	}
	if stats.OpenQuests != 0 {
		t.Errorf("OpenQuests = %d, want 0", stats.OpenQuests)
	}
}

func TestComputeWorldStats_InProgressQuest_CountsActive(t *testing.T) {
	agg := newTestAggregator()
	quests := []semdragons.Quest{
		{ID: "q1", Status: semdragons.QuestInProgress},
	}

	stats := agg.computeWorldStats(nil, quests, nil, nil)

	if stats.ActiveQuests != 1 {
		t.Errorf("ActiveQuests = %d, want 1", stats.ActiveQuests)
	}
}

func TestComputeWorldStats_InReviewQuest_CountsActive(t *testing.T) {
	agg := newTestAggregator()
	quests := []semdragons.Quest{
		{ID: "q1", Status: semdragons.QuestInReview},
	}

	stats := agg.computeWorldStats(nil, quests, nil, nil)

	if stats.ActiveQuests != 1 {
		t.Errorf("ActiveQuests = %d, want 1", stats.ActiveQuests)
	}
}

func TestComputeWorldStats_CompletedQuest_AffectsCompletionRate(t *testing.T) {
	agg := newTestAggregator()
	// Two quests total: one completed, one posted. Completion rate = 0.5.
	quests := []semdragons.Quest{
		{ID: "q1", Status: semdragons.QuestCompleted},
		{ID: "q2", Status: semdragons.QuestPosted},
	}

	stats := agg.computeWorldStats(nil, quests, nil, nil)

	const want = 0.5
	if stats.CompletionRate != want {
		t.Errorf("CompletionRate = %f, want %f", stats.CompletionRate, want)
	}
	// Completed quests do not appear in OpenQuests or ActiveQuests.
	if stats.OpenQuests != 1 {
		t.Errorf("OpenQuests = %d, want 1", stats.OpenQuests)
	}
}

func TestComputeWorldStats_CompletionRate_ZeroGuard(t *testing.T) {
	agg := newTestAggregator()

	// An empty quest slice must not produce a NaN or divide-by-zero panic.
	stats := agg.computeWorldStats(nil, []semdragons.Quest{}, nil, nil)

	if stats.CompletionRate != 0.0 {
		t.Errorf("CompletionRate = %f, want 0.0 when quest list is empty", stats.CompletionRate)
	}
}

func TestComputeWorldStats_CompletionRate_AllCompleted(t *testing.T) {
	agg := newTestAggregator()
	quests := []semdragons.Quest{
		{ID: "q1", Status: semdragons.QuestCompleted},
		{ID: "q2", Status: semdragons.QuestCompleted},
		{ID: "q3", Status: semdragons.QuestCompleted},
	}

	stats := agg.computeWorldStats(nil, quests, nil, nil)

	if stats.CompletionRate != 1.0 {
		t.Errorf("CompletionRate = %f, want 1.0 when all quests completed", stats.CompletionRate)
	}
}

func TestComputeWorldStats_CompletionRate_NoneCompleted(t *testing.T) {
	agg := newTestAggregator()
	quests := []semdragons.Quest{
		{ID: "q1", Status: semdragons.QuestPosted},
		{ID: "q2", Status: semdragons.QuestInProgress},
	}

	stats := agg.computeWorldStats(nil, quests, nil, nil)

	if stats.CompletionRate != 0.0 {
		t.Errorf("CompletionRate = %f, want 0.0 when no quests completed", stats.CompletionRate)
	}
}

// =============================================================================
// AVERAGE QUALITY TESTS
// =============================================================================

func TestComputeWorldStats_AvgQuality_OnlyAgentsWithCompletedQuests(t *testing.T) {
	agg := newTestAggregator()
	// Agent with QuestsCompleted > 0 contributes to avg quality.
	// Agent with QuestsCompleted == 0 is excluded from the quality average.
	agents := []*semdragons.Agent{
		{
			ID:     "a1",
			Status: semdragons.AgentIdle,
			Stats:  semdragons.AgentStats{QuestsCompleted: 5, AvgQualityScore: 0.8},
		},
		{
			ID:     "a2",
			Status: semdragons.AgentIdle,
			Stats:  semdragons.AgentStats{QuestsCompleted: 0, AvgQualityScore: 0.9},
		},
	}

	stats := agg.computeWorldStats(agents, nil, nil, nil)

	// Only a1 qualifies: avg = 0.8 / 1 = 0.8
	if stats.AvgQuality != 0.8 {
		t.Errorf("AvgQuality = %f, want 0.8 (agent with zero completed quests excluded)", stats.AvgQuality)
	}
}

func TestComputeWorldStats_AvgQuality_MultipleQualifiedAgents(t *testing.T) {
	agg := newTestAggregator()
	agents := []*semdragons.Agent{
		{
			ID:     "a1",
			Status: semdragons.AgentIdle,
			Stats:  semdragons.AgentStats{QuestsCompleted: 3, AvgQualityScore: 0.6},
		},
		{
			ID:     "a2",
			Status: semdragons.AgentIdle,
			Stats:  semdragons.AgentStats{QuestsCompleted: 7, AvgQualityScore: 1.0},
		},
	}

	stats := agg.computeWorldStats(agents, nil, nil, nil)

	// (0.6 + 1.0) / 2 = 0.8
	const want = 0.8
	if stats.AvgQuality != want {
		t.Errorf("AvgQuality = %f, want %f", stats.AvgQuality, want)
	}
}

func TestComputeWorldStats_AvgQuality_ZeroGuard_NoQualifiedAgents(t *testing.T) {
	agg := newTestAggregator()
	// All agents have zero completed quests — no division should occur.
	agents := []*semdragons.Agent{
		{
			ID:     "a1",
			Status: semdragons.AgentIdle,
			Stats:  semdragons.AgentStats{QuestsCompleted: 0, AvgQualityScore: 0.9},
		},
	}

	stats := agg.computeWorldStats(agents, nil, nil, nil)

	if stats.AvgQuality != 0.0 {
		t.Errorf("AvgQuality = %f, want 0.0 when no agents have completed quests", stats.AvgQuality)
	}
}

func TestComputeWorldStats_AvgQuality_NilAgentSkipped(t *testing.T) {
	agg := newTestAggregator()
	// Nil agents in the slice must not panic during quality computation.
	agents := []*semdragons.Agent{
		nil,
		{
			ID:     "a2",
			Status: semdragons.AgentIdle,
			Stats:  semdragons.AgentStats{QuestsCompleted: 2, AvgQualityScore: 0.75},
		},
	}

	stats := agg.computeWorldStats(agents, nil, nil, nil)

	if stats.AvgQuality != 0.75 {
		t.Errorf("AvgQuality = %f, want 0.75 (nil agent skipped)", stats.AvgQuality)
	}
}

// =============================================================================
// PARTY AND GUILD COUNTS
// =============================================================================

func TestComputeWorldStats_PartyCounts(t *testing.T) {
	agg := newTestAggregator()
	parties := []semdragons.Party{
		{ID: "p1"},
		{ID: "p2"},
		{ID: "p3"},
	}

	stats := agg.computeWorldStats(nil, nil, parties, nil)

	if stats.ActiveParties != 3 {
		t.Errorf("ActiveParties = %d, want 3", stats.ActiveParties)
	}
}

func TestComputeWorldStats_GuildCounts(t *testing.T) {
	agg := newTestAggregator()
	guilds := []semdragons.Guild{
		{ID: "g1"},
		{ID: "g2"},
	}

	stats := agg.computeWorldStats(nil, nil, nil, guilds)

	if stats.ActiveGuilds != 2 {
		t.Errorf("ActiveGuilds = %d, want 2", stats.ActiveGuilds)
	}
}

func TestComputeWorldStats_EmptyPartiesAndGuilds(t *testing.T) {
	agg := newTestAggregator()

	stats := agg.computeWorldStats(nil, nil, []semdragons.Party{}, []semdragons.Guild{})

	if stats.ActiveParties != 0 {
		t.Errorf("ActiveParties = %d, want 0", stats.ActiveParties)
	}
	if stats.ActiveGuilds != 0 {
		t.Errorf("ActiveGuilds = %d, want 0", stats.ActiveGuilds)
	}
}

// =============================================================================
// FULL SCENARIO TESTS
// =============================================================================

// TestComputeWorldStats_FullScenario exercises all counters together so that
// we catch any accidental cross-contamination between fields.
func TestComputeWorldStats_FullScenario(t *testing.T) {
	agg := newTestAggregator()

	agents := []*semdragons.Agent{
		{ID: "idle-1", Status: semdragons.AgentIdle, Stats: semdragons.AgentStats{QuestsCompleted: 10, AvgQualityScore: 0.9}},
		{ID: "idle-2", Status: semdragons.AgentIdle, Stats: semdragons.AgentStats{QuestsCompleted: 5, AvgQualityScore: 0.7}},
		{ID: "on-quest", Status: semdragons.AgentOnQuest},
		{ID: "in-battle", Status: semdragons.AgentInBattle},
		{ID: "cooldown", Status: semdragons.AgentCooldown},
		{ID: "retired", Status: semdragons.AgentRetired},
	}

	quests := []semdragons.Quest{
		{ID: "q1", Status: semdragons.QuestPosted},
		{ID: "q2", Status: semdragons.QuestClaimed},
		{ID: "q3", Status: semdragons.QuestInProgress},
		{ID: "q4", Status: semdragons.QuestInReview},
		{ID: "q5", Status: semdragons.QuestCompleted},
		{ID: "q6", Status: semdragons.QuestCompleted},
	}

	parties := []semdragons.Party{{ID: "p1"}, {ID: "p2"}}
	guilds := []semdragons.Guild{{ID: "g1"}}

	stats := agg.computeWorldStats(agents, quests, parties, guilds)

	// Agent counts: idle(2)+on_quest(1)+in_battle(1) = 4 active.
	if stats.ActiveAgents != 4 {
		t.Errorf("ActiveAgents = %d, want 4", stats.ActiveAgents)
	}
	if stats.IdleAgents != 2 {
		t.Errorf("IdleAgents = %d, want 2", stats.IdleAgents)
	}
	if stats.CooldownAgents != 1 {
		t.Errorf("CooldownAgents = %d, want 1", stats.CooldownAgents)
	}
	if stats.RetiredAgents != 1 {
		t.Errorf("RetiredAgents = %d, want 1", stats.RetiredAgents)
	}

	// Quest counts: 1 open, 3 active (claimed+in_progress+in_review).
	if stats.OpenQuests != 1 {
		t.Errorf("OpenQuests = %d, want 1", stats.OpenQuests)
	}
	if stats.ActiveQuests != 3 {
		t.Errorf("ActiveQuests = %d, want 3", stats.ActiveQuests)
	}

	// CompletionRate = 2 completed / 6 total = 1/3 ≈ 0.333...
	const wantRate = 2.0 / 6.0
	if stats.CompletionRate != wantRate {
		t.Errorf("CompletionRate = %f, want %f", stats.CompletionRate, wantRate)
	}

	// AvgQuality from two qualified agents: (0.9 + 0.7) / 2 = 0.8.
	const wantQuality = 0.8
	if stats.AvgQuality != wantQuality {
		t.Errorf("AvgQuality = %f, want %f", stats.AvgQuality, wantQuality)
	}

	if stats.ActiveParties != 2 {
		t.Errorf("ActiveParties = %d, want 2", stats.ActiveParties)
	}
	if stats.ActiveGuilds != 1 {
		t.Errorf("ActiveGuilds = %d, want 1", stats.ActiveGuilds)
	}
}
