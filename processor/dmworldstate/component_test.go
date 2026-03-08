//go:build integration

package dmworldstate

// =============================================================================
// INTEGRATION TESTS - DM WorldState Component
// =============================================================================
// These tests require Docker for NATS via testcontainers.
// Run with: go test -tags=integration ./processor/dmworldstate/...
// =============================================================================

import (
	"context"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"

	semdragons "github.com/c360studio/semdragons"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/bossbattle"
)

// =============================================================================
// LIFECYCLE TESTS
// =============================================================================

func TestComponent_Lifecycle(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	deps := component.Dependencies{
		NATSClient: client,
	}

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "dmworld-lifecycle"

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}

	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	gc := semdragons.NewGraphClient(client, comp.boardConfig)
	if err := gc.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket failed: %v", err)
	}

	meta := comp.Meta()
	if meta.Name != ComponentName {
		t.Errorf("Meta.Name = %q, want %q", meta.Name, ComponentName)
	}
	if meta.Type != "processor" {
		t.Errorf("Meta.Type = %q, want %q", meta.Type, "processor")
	}

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	health := comp.Health()
	if !health.Healthy {
		t.Error("component should be healthy after start")
	}
	if health.Status != "running" {
		t.Errorf("Health.Status = %q, want %q", health.Status, "running")
	}

	if err := comp.Stop(5 * time.Second); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	health = comp.Health()
	if health.Healthy {
		t.Error("component should not be healthy after stop")
	}
	if health.Status != "stopped" {
		t.Errorf("Health.Status = %q, want %q", health.Status, "stopped")
	}
}

// =============================================================================
// PORT AND SCHEMA TESTS
// =============================================================================

func TestComponent_InputOutputPorts(t *testing.T) {
	comp := &Component{}

	// dmworldstate is a query-only component with no ports.
	inputs := comp.InputPorts()
	if len(inputs) != 0 {
		t.Errorf("expected 0 input ports, got %d", len(inputs))
	}

	outputs := comp.OutputPorts()
	if len(outputs) != 0 {
		t.Errorf("expected 0 output ports, got %d", len(outputs))
	}
}

func TestComponent_ConfigSchema(t *testing.T) {
	comp := &Component{}
	schema := comp.ConfigSchema()

	requiredFields := []string{"org", "platform", "board"}
	for _, field := range requiredFields {
		found := false
		for _, req := range schema.Required {
			if req == field {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("field %q should be required in ConfigSchema", field)
		}
	}

	expectedProps := []string{"org", "platform", "board", "max_entities_per_query"}
	for _, prop := range expectedProps {
		if _, exists := schema.Properties[prop]; !exists {
			t.Errorf("missing property %q in ConfigSchema", prop)
		}
	}
}

// =============================================================================
// WORLD STATE AGGREGATION
// =============================================================================

func TestWorldState_EmptyBoard_ReturnsValidState(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupWorldComponent(t, client, "world-empty")
	defer comp.Stop(5 * time.Second)

	state, err := comp.WorldState(ctx)
	if err != nil {
		t.Fatalf("WorldState failed: %v", err)
	}

	if state == nil {
		t.Fatal("WorldState should not return nil")
	}
	// An empty board has no entities; stats should all be zero.
	if state.Stats.ActiveAgents != 0 {
		t.Errorf("Stats.ActiveAgents = %d, want 0", state.Stats.ActiveAgents)
	}
	if state.Stats.OpenQuests != 0 {
		t.Errorf("Stats.OpenQuests = %d, want 0", state.Stats.OpenQuests)
	}
}

func TestWorldState_AgentCounts_Accurate(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupWorldComponent(t, client, "world-agentcounts")
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.boardConfig)

	// Create two idle, one on-quest, one retired, one cooldown.
	putTestAgent(t, ctx, gc, comp.boardConfig, "idle-1", domain.AgentIdle, nil)
	putTestAgent(t, ctx, gc, comp.boardConfig, "idle-2", domain.AgentIdle, nil)
	putTestAgent(t, ctx, gc, comp.boardConfig, "on-quest", domain.AgentOnQuest, nil)
	putTestAgent(t, ctx, gc, comp.boardConfig, "retired-agent", domain.AgentRetired, nil)

	cooldownUntil := time.Now().Add(1 * time.Hour)
	putTestAgent(t, ctx, gc, comp.boardConfig, "cooldown-agent", domain.AgentCooldown, &cooldownUntil)

	state, err := comp.WorldState(ctx)
	if err != nil {
		t.Fatalf("WorldState failed: %v", err)
	}

	// Idle agents count toward both ActiveAgents and IdleAgents.
	if state.Stats.IdleAgents != 2 {
		t.Errorf("Stats.IdleAgents = %d, want 2", state.Stats.IdleAgents)
	}
	// ActiveAgents = idle(2) + on_quest(1) = 3.
	if state.Stats.ActiveAgents != 3 {
		t.Errorf("Stats.ActiveAgents = %d, want 3", state.Stats.ActiveAgents)
	}
	if state.Stats.CooldownAgents != 1 {
		t.Errorf("Stats.CooldownAgents = %d, want 1", state.Stats.CooldownAgents)
	}
	if state.Stats.RetiredAgents != 1 {
		t.Errorf("Stats.RetiredAgents = %d, want 1", state.Stats.RetiredAgents)
	}
}

func TestWorldState_QuestCounts_Accurate(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupWorldComponent(t, client, "world-questcounts")
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.boardConfig)

	// Create quests in various active states (only active statuses appear in world state).
	putTestQuest(t, ctx, gc, comp.boardConfig, "posted-1", domain.QuestPosted)
	putTestQuest(t, ctx, gc, comp.boardConfig, "posted-2", domain.QuestPosted)
	putTestQuest(t, ctx, gc, comp.boardConfig, "in-progress", domain.QuestInProgress)
	putTestQuest(t, ctx, gc, comp.boardConfig, "in-review", domain.QuestInReview)
	putTestQuest(t, ctx, gc, comp.boardConfig, "escalated", domain.QuestEscalated)
	// Completed quests are not included in active quest list.
	putTestQuest(t, ctx, gc, comp.boardConfig, "completed", domain.QuestCompleted)
	putTestQuest(t, ctx, gc, comp.boardConfig, "failed-q", domain.QuestFailed)

	state, err := comp.WorldState(ctx)
	if err != nil {
		t.Fatalf("WorldState failed: %v", err)
	}

	// OpenQuests = posted quests only.
	if state.Stats.OpenQuests != 2 {
		t.Errorf("Stats.OpenQuests = %d, want 2", state.Stats.OpenQuests)
	}
	// ActiveQuests = claimed + in_progress + in_review statuses.
	if state.Stats.ActiveQuests != 2 {
		t.Errorf("Stats.ActiveQuests = %d, want 2 (in_progress + in_review)", state.Stats.ActiveQuests)
	}
}

func TestWorldState_Agents_PopulatedInState(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupWorldComponent(t, client, "world-agents-list")
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.boardConfig)
	putTestAgent(t, ctx, gc, comp.boardConfig, "agent-for-list", domain.AgentIdle, nil)

	state, err := comp.WorldState(ctx)
	if err != nil {
		t.Fatalf("WorldState failed: %v", err)
	}

	if len(state.Agents) == 0 {
		t.Error("WorldState.Agents should contain at least one agent")
	}
}

// =============================================================================
// GET IDLE AGENTS
// =============================================================================

func TestGetIdleAgents_ReturnsOnlyIdle(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupWorldComponent(t, client, "world-idle-agents")
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.boardConfig)

	putTestAgent(t, ctx, gc, comp.boardConfig, "idle-a", domain.AgentIdle, nil)
	putTestAgent(t, ctx, gc, comp.boardConfig, "idle-b", domain.AgentIdle, nil)
	putTestAgent(t, ctx, gc, comp.boardConfig, "on-quest-c", domain.AgentOnQuest, nil)
	putTestAgent(t, ctx, gc, comp.boardConfig, "retired-d", domain.AgentRetired, nil)

	// Idle agent with active cooldown should NOT be returned.
	cooldownUntil := time.Now().Add(1 * time.Hour)
	putTestAgent(t, ctx, gc, comp.boardConfig, "cooldown-e", domain.AgentCooldown, &cooldownUntil)

	idle, err := comp.GetIdleAgents(ctx)
	if err != nil {
		t.Fatalf("GetIdleAgents failed: %v", err)
	}

	if len(idle) != 2 {
		t.Errorf("idle agent count = %d, want 2", len(idle))
	}
	for _, a := range idle {
		if a.Status != domain.AgentIdle {
			t.Errorf("agent %v has status %v, want %v", a.ID, a.Status, domain.AgentIdle)
		}
		if a.CooldownUntil != nil {
			t.Errorf("idle agent %v should have nil CooldownUntil", a.ID)
		}
	}
}

func TestGetIdleAgents_EmptyBoard_ReturnsEmpty(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupWorldComponent(t, client, "world-idle-empty")
	defer comp.Stop(5 * time.Second)

	idle, err := comp.GetIdleAgents(ctx)
	if err != nil {
		t.Fatalf("GetIdleAgents failed: %v", err)
	}
	if len(idle) != 0 {
		t.Errorf("idle agent count = %d, want 0 on empty board", len(idle))
	}
}

// =============================================================================
// GET ESCALATED QUESTS
// =============================================================================

func TestGetEscalatedQuests_ReturnsOnlyEscalated(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupWorldComponent(t, client, "world-escalated")
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.boardConfig)

	putTestQuest(t, ctx, gc, comp.boardConfig, "escalated-q1", domain.QuestEscalated)
	putTestQuest(t, ctx, gc, comp.boardConfig, "escalated-q2", domain.QuestEscalated)
	putTestQuest(t, ctx, gc, comp.boardConfig, "posted-q", domain.QuestPosted)
	putTestQuest(t, ctx, gc, comp.boardConfig, "in-progress-q", domain.QuestInProgress)

	escalated, err := comp.GetEscalatedQuests(ctx)
	if err != nil {
		t.Fatalf("GetEscalatedQuests failed: %v", err)
	}

	if len(escalated) != 2 {
		t.Errorf("escalated quest count = %d, want 2", len(escalated))
	}
	for _, q := range escalated {
		if q.Status != domain.QuestEscalated {
			t.Errorf("quest %v has status %v, want %v", q.ID, q.Status, domain.QuestEscalated)
		}
	}
}

func TestGetEscalatedQuests_EmptyBoard_ReturnsEmpty(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupWorldComponent(t, client, "world-escalated-empty")
	defer comp.Stop(5 * time.Second)

	escalated, err := comp.GetEscalatedQuests(ctx)
	if err != nil {
		t.Fatalf("GetEscalatedQuests failed: %v", err)
	}
	if len(escalated) != 0 {
		t.Errorf("escalated count = %d, want 0 on empty board", len(escalated))
	}
}

// =============================================================================
// GET PENDING BATTLES
// =============================================================================

func TestGetPendingBattles_EmptyBoard_ReturnsEmpty(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupWorldComponent(t, client, "world-battles-empty")
	defer comp.Stop(5 * time.Second)

	battles, err := comp.GetPendingBattles(ctx)
	if err != nil {
		t.Fatalf("GetPendingBattles failed: %v", err)
	}
	if len(battles) != 0 {
		t.Errorf("pending battle count = %d, want 0 on empty board", len(battles))
	}
}

func TestGetPendingBattles_ActiveBattleWithNoVerdict_Returned(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupWorldComponent(t, client, "world-battles-pending")
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.boardConfig)

	// Put an active BossBattle with no verdict into KV.
	putTestBattle(t, ctx, gc, comp.boardConfig, "active-battle", domain.BattleActive, false)
	// Put a completed battle (has verdict) - should not appear.
	putTestBattle(t, ctx, gc, comp.boardConfig, "completed-battle", domain.BattleVictory, true)

	battles, err := comp.GetPendingBattles(ctx)
	if err != nil {
		t.Fatalf("GetPendingBattles failed: %v", err)
	}

	if len(battles) != 1 {
		t.Errorf("pending battle count = %d, want 1 (active without verdict)", len(battles))
	}
	if len(battles) > 0 && battles[0].Status != domain.BattleActive {
		t.Errorf("pending battle status = %v, want %v", battles[0].Status, domain.BattleActive)
	}
}

// =============================================================================
// OPERATION GUARD
// =============================================================================

func TestWorldState_FailsWhenNotRunning(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	deps := component.Dependencies{NATSClient: client}
	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "world-not-running"

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	// Intentionally do NOT call Start.

	_, err = comp.WorldState(ctx)
	if err == nil {
		t.Fatal("WorldState should error when component is not running")
	}
}

// =============================================================================
// HELPERS
// =============================================================================

func setupWorldComponent(t *testing.T, client *natsclient.Client, boardName string) *Component {
	t.Helper()

	deps := component.Dependencies{NATSClient: client}
	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = boardName

	ctx := context.Background()

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	gc := semdragons.NewGraphClient(client, comp.boardConfig)
	if err := gc.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket failed: %v", err)
	}

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	return comp
}

func putTestAgent(
	t *testing.T,
	ctx context.Context,
	gc *semdragons.GraphClient,
	config *domain.BoardConfig,
	name string,
	status domain.AgentStatus,
	cooldownUntil *time.Time,
) *agentprogression.Agent {
	t.Helper()

	instance := domain.GenerateInstance()
	agentID := domain.AgentID(config.AgentEntityID(instance))

	agent := &agentprogression.Agent{
		ID:            agentID,
		Name:          name,
		Level:         5,
		Tier:          domain.TierApprentice,
		Status:        status,
		CooldownUntil: cooldownUntil,
	}

	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("putTestAgent: PutEntityState failed: %v", err)
	}
	return agent
}

func putTestQuest(
	t *testing.T,
	ctx context.Context,
	gc *semdragons.GraphClient,
	config *domain.BoardConfig,
	title string,
	status domain.QuestStatus,
) *domain.Quest {
	t.Helper()

	instance := domain.GenerateInstance()
	questID := domain.QuestID(config.QuestEntityID(instance))

	escalated := status == domain.QuestEscalated
	quest := &domain.Quest{
		ID:         questID,
		Title:      title,
		Difficulty: domain.DifficultyTrivial,
		Status:     status,
		Escalated:  escalated,
	}

	if err := gc.PutEntityState(ctx, quest, "quest.lifecycle.posted"); err != nil {
		t.Fatalf("putTestQuest: PutEntityState failed: %v", err)
	}
	return quest
}

func putTestBattle(
	t *testing.T,
	ctx context.Context,
	gc *semdragons.GraphClient,
	config *domain.BoardConfig,
	name string,
	status domain.BattleStatus,
	hasVerdict bool,
) *bossbattle.BossBattle {
	t.Helper()

	instance := domain.GenerateInstance()
	battleID := domain.BattleID(config.BattleEntityID(instance))

	battle := &bossbattle.BossBattle{
		ID:     battleID,
		Status: status,
	}

	if hasVerdict {
		verdict := domain.BattleVerdict{
			Passed:       true,
			QualityScore: 0.9,
			XPAwarded:    100,
		}
		battle.Verdict = &verdict
	}

	if err := gc.PutEntityState(ctx, battle, "battle.review.started"); err != nil {
		t.Fatalf("putTestBattle: PutEntityState failed: %v", err)
	}
	return battle
}
