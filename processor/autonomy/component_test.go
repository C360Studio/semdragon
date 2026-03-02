//go:build integration

package autonomy

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentstore"
	"github.com/c360studio/semdragons/processor/boidengine"
	"github.com/c360studio/semdragons/processor/guildformation"
)

// =============================================================================
// INTEGRATION TESTS - Autonomy Component
// =============================================================================
// These tests require Docker for NATS via testcontainers.
// Run with: go test -tags=integration -count=1 -v ./processor/autonomy/...
// =============================================================================

func TestComponent_Lifecycle(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client

	comp := setupComponent(t, client, "lifecycle")
	defer comp.Stop(5 * time.Second)

	// Verify health after start
	health := comp.Health()
	if !health.Healthy {
		t.Errorf("Health.Healthy = false after start")
	}
	if health.Status != "running" {
		t.Errorf("Health.Status = %q, want %q", health.Status, "running")
	}

	// Verify stop
	if err := comp.Stop(5 * time.Second); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	health = comp.Health()
	if health.Healthy {
		t.Error("Health.Healthy = true after stop")
	}

	// Verify re-start works (create fresh component)
	comp2 := setupComponent(t, client, "lifecycle2")
	defer comp2.Stop(5 * time.Second)
}

func TestCooldownExpiryTransitionsToIdle(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	// Use very short intervals for testing
	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "cooldown"
	config.InitialDelayMs = 100
	config.CooldownIntervalMs = 200

	comp := setupComponentWithConfig(t, client, config)
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.BoardConfig())

	// Create agent in cooldown with expired CooldownUntil
	instance := semdragons.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(instance))
	expired := time.Now().Add(-1 * time.Hour) // Already expired
	agent := &semdragons.Agent{
		ID:            agentID,
		Name:          "cooldown-agent",
		Status:        semdragons.AgentCooldown,
		Level:         3,
		Tier:          semdragons.TierApprentice,
		CooldownUntil: &expired,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	// Wait for KV watch to pick up the agent and heartbeat to fire
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for cooldown expiry transition")
		case <-time.After(100 * time.Millisecond):
			entity, err := gc.GetAgent(ctx, agentID)
			if err != nil {
				continue
			}
			updated := semdragons.AgentFromEntityState(entity)
			if updated != nil && updated.Status == semdragons.AgentIdle {
				// Cooldown expired successfully
				if updated.CooldownUntil != nil {
					t.Error("CooldownUntil should be nil after expiry")
				}
				return
			}
		}
	}
}

func TestCooldownNotExpiredStaysCooldown(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "noexpiry"
	config.InitialDelayMs = 100
	config.CooldownIntervalMs = 200

	comp := setupComponentWithConfig(t, client, config)
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.BoardConfig())

	// Create agent in cooldown with future CooldownUntil
	instance := semdragons.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(instance))
	future := time.Now().Add(1 * time.Hour) // Not expired
	agent := &semdragons.Agent{
		ID:            agentID,
		Name:          "future-cooldown-agent",
		Status:        semdragons.AgentCooldown,
		Level:         3,
		Tier:          semdragons.TierApprentice,
		CooldownUntil: &future,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	// Wait for at least 2 heartbeats, then verify status unchanged
	time.Sleep(600 * time.Millisecond)

	entity, err := gc.GetAgent(ctx, agentID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	updated := semdragons.AgentFromEntityState(entity)
	if updated == nil {
		t.Fatal("Failed to reconstruct agent")
	}
	if updated.Status != semdragons.AgentCooldown {
		t.Errorf("Status = %v, want %v (cooldown not expired)", updated.Status, semdragons.AgentCooldown)
	}
}

func TestBoidSuggestionCached(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "boidcache"
	config.InitialDelayMs = 5000 // Long delay so heartbeat doesn't consume the suggestion

	comp := setupComponentWithConfig(t, client, config)
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.BoardConfig())

	// Create idle agent
	instance := semdragons.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(instance))
	agent := &semdragons.Agent{
		ID:        agentID,
		Name:      "boid-test-agent",
		Status:    semdragons.AgentIdle,
		Level:     5,
		Tier:      semdragons.TierApprentice,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	// Wait for agent to be tracked
	waitForTracker(t, comp, instance, 3*time.Second)

	// Publish boid suggestions (ranked list)
	suggestions := []boidengine.SuggestedClaim{
		{
			AgentID:    agentID,
			QuestID:    "test.integration.game.boidcache.quest.q123",
			Score:      3.14,
			Confidence: 0.8,
			Reason:     "test suggestion rank 1",
		},
		{
			AgentID:    agentID,
			QuestID:    "test.integration.game.boidcache.quest.q456",
			Score:      2.0,
			Confidence: 0.5,
			Reason:     "test suggestion rank 2",
		},
	}
	data, err := json.Marshal(suggestions)
	if err != nil {
		t.Fatalf("Marshal suggestions: %v", err)
	}
	subject := "boid.suggestions." + instance
	if err := client.Publish(ctx, subject, data); err != nil {
		t.Fatalf("Publish suggestions: %v", err)
	}

	// Wait for suggestions to be cached
	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for suggestion cache")
		case <-time.After(50 * time.Millisecond):
			comp.trackersMu.RLock()
			tracker := comp.trackers[instance]
			hasSuggestions := tracker != nil && len(tracker.suggestions) > 0
			comp.trackersMu.RUnlock()
			if hasSuggestions {
				comp.trackersMu.RLock()
				cached := comp.trackers[instance].suggestions
				comp.trackersMu.RUnlock()
				if len(cached) != 2 {
					t.Errorf("cached suggestions count = %d, want 2", len(cached))
				}
				if string(cached[0].QuestID) != string(suggestions[0].QuestID) {
					t.Errorf("cached[0].QuestID = %v, want %v", cached[0].QuestID, suggestions[0].QuestID)
				}
				return
			}
		}
	}
}

func TestHeartbeatStartsOnAgentCreation(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "hbcreate"
	config.InitialDelayMs = 5000 // Long delay so heartbeat doesn't fire during test

	comp := setupComponentWithConfig(t, client, config)
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.BoardConfig())

	// Create agent
	instance := semdragons.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(instance))
	agent := &semdragons.Agent{
		ID:        agentID,
		Name:      "heartbeat-test-agent",
		Status:    semdragons.AgentIdle,
		Level:     1,
		Tier:      semdragons.TierApprentice,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	// Wait for tracker to appear
	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for heartbeat tracker creation")
		case <-time.After(50 * time.Millisecond):
			comp.trackersMu.RLock()
			tracker, exists := comp.trackers[instance]
			comp.trackersMu.RUnlock()
			if exists {
				expectedInterval := config.IntervalForStatus(semdragons.AgentIdle)
				if tracker.interval != expectedInterval {
					t.Errorf("tracker interval = %v, want %v", tracker.interval, expectedInterval)
				}
				if tracker.heartbeat == nil {
					t.Error("tracker.heartbeat is nil, expected a timer")
				}
				return
			}
		}
	}
}

func TestHeartbeatCancelledOnRetire(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "hbretire"
	config.InitialDelayMs = 5000

	comp := setupComponentWithConfig(t, client, config)
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.BoardConfig())

	// Create idle agent first
	instance := semdragons.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(instance))
	agent := &semdragons.Agent{
		ID:        agentID,
		Name:      "retire-test-agent",
		Status:    semdragons.AgentIdle,
		Level:     1,
		Tier:      semdragons.TierApprentice,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	// Wait for tracker to appear
	waitForTracker(t, comp, instance, 3*time.Second)

	// Retire agent
	agent.Status = semdragons.AgentRetired
	agent.UpdatedAt = time.Now()
	if err := gc.PutEntityState(ctx, agent, "agent.status.retired"); err != nil {
		t.Fatalf("Failed to retire agent: %v", err)
	}

	// Wait for heartbeat to be set to nil (retired = interval 0 = no heartbeat)
	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for retired heartbeat cancellation")
		case <-time.After(50 * time.Millisecond):
			comp.trackersMu.RLock()
			tracker, exists := comp.trackers[instance]
			comp.trackersMu.RUnlock()
			if exists && tracker.heartbeat == nil {
				return // Retired agents have no heartbeat timer
			}
		}
	}
}

// =============================================================================
// AUTONOMOUS QUEST CLAIM TESTS
// =============================================================================

func TestAutonomousQuestClaim(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "autoclaim"
	config.InitialDelayMs = 100
	config.IdleIntervalMs = 200

	comp := setupComponentWithConfig(t, client, config)
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.BoardConfig())

	// Create a posted quest
	questInstance := semdragons.GenerateInstance()
	questID := semdragons.QuestID(comp.BoardConfig().QuestEntityID(questInstance))
	quest := &semdragons.Quest{
		ID:        questID,
		Title:     "Auto-claim test quest",
		Status:    semdragons.QuestPosted,
		PostedAt:  time.Now(),
		MinTier:   semdragons.TierApprentice,
		BaseXP:    100,
	}
	if err := gc.PutEntityState(ctx, quest, "quest.lifecycle.posted"); err != nil {
		t.Fatalf("Failed to create test quest: %v", err)
	}

	// Create idle agent
	agentInstance := semdragons.GenerateInstance()
	agentID := semdragons.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	agent := &semdragons.Agent{
		ID:        agentID,
		Name:      "autoclaim-agent",
		Status:    semdragons.AgentIdle,
		Level:     5,
		Tier:      semdragons.TierApprentice,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	// Wait for agent to be tracked
	waitForTracker(t, comp, agentInstance, 3*time.Second)

	// Publish boid suggestion pointing at the quest
	suggestions := []boidengine.SuggestedClaim{
		{
			AgentID:    agentID,
			QuestID:    questID,
			Score:      5.0,
			Confidence: 0.9,
			Reason:     "test autoclaim suggestion",
		},
	}
	data, err := json.Marshal(suggestions)
	if err != nil {
		t.Fatalf("Marshal suggestions: %v", err)
	}
	subject := "boid.suggestions." + agentInstance
	if err := client.Publish(ctx, subject, data); err != nil {
		t.Fatalf("Publish suggestions: %v", err)
	}

	// Wait for autonomous claim: quest should become claimed, agent should be on_quest
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for autonomous quest claim")
		case <-time.After(100 * time.Millisecond):
			// Check quest status
			questEntity, err := gc.GetQuest(ctx, domain.QuestID(questID))
			if err != nil {
				continue
			}
			updatedQuest := semdragons.QuestFromEntityState(questEntity)
			if updatedQuest == nil || updatedQuest.Status != semdragons.QuestClaimed {
				continue
			}

			// Quest is claimed! Verify agent is on_quest
			agentEntity, err := gc.GetAgent(ctx, domain.AgentID(agentID))
			if err != nil {
				t.Fatalf("GetAgent failed: %v", err)
			}
			updatedAgent := semdragons.AgentFromEntityState(agentEntity)
			if updatedAgent == nil {
				t.Fatal("Failed to reconstruct agent")
			}
			if updatedAgent.Status != semdragons.AgentOnQuest {
				t.Errorf("Agent status = %v, want %v", updatedAgent.Status, semdragons.AgentOnQuest)
			}
			if updatedAgent.CurrentQuest == nil || *updatedAgent.CurrentQuest != questID {
				t.Errorf("Agent CurrentQuest = %v, want %v", updatedAgent.CurrentQuest, questID)
			}
			if updatedQuest.ClaimedBy == nil || semdragons.AgentID(*updatedQuest.ClaimedBy) != agentID {
				t.Errorf("Quest ClaimedBy = %v, want %v", updatedQuest.ClaimedBy, agentID)
			}
			return
		}
	}
}

func TestAutonomousQuestClaim_FallsThrough(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "fallthrough"
	config.InitialDelayMs = 100
	config.IdleIntervalMs = 200

	comp := setupComponentWithConfig(t, client, config)
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.BoardConfig())

	// Create a stale quest (already claimed — will be skipped)
	staleInstance := semdragons.GenerateInstance()
	staleQuestID := semdragons.QuestID(comp.BoardConfig().QuestEntityID(staleInstance))
	staleAgentID := semdragons.AgentID("some.other.agent")
	now := time.Now()
	staleQuest := &semdragons.Quest{
		ID:        staleQuestID,
		Title:     "Already claimed quest",
		Status:    semdragons.QuestClaimed,
		ClaimedBy: &staleAgentID,
		ClaimedAt: &now,
		PostedAt:  now,
	}
	if err := gc.PutEntityState(ctx, staleQuest, "quest.claimed"); err != nil {
		t.Fatalf("Failed to create stale quest: %v", err)
	}

	// Create a good quest (still posted — will be claimed as fallthrough)
	goodInstance := semdragons.GenerateInstance()
	goodQuestID := semdragons.QuestID(comp.BoardConfig().QuestEntityID(goodInstance))
	goodQuest := &semdragons.Quest{
		ID:       goodQuestID,
		Title:    "Good fallthrough quest",
		Status:   semdragons.QuestPosted,
		PostedAt: time.Now(),
		MinTier:  semdragons.TierApprentice,
		BaseXP:   100,
	}
	if err := gc.PutEntityState(ctx, goodQuest, "quest.lifecycle.posted"); err != nil {
		t.Fatalf("Failed to create good quest: %v", err)
	}

	// Create idle agent
	agentInstance := semdragons.GenerateInstance()
	agentID := semdragons.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	agent := &semdragons.Agent{
		ID:        agentID,
		Name:      "fallthrough-agent",
		Status:    semdragons.AgentIdle,
		Level:     5,
		Tier:      semdragons.TierApprentice,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	// Wait for agent to be tracked
	waitForTracker(t, comp, agentInstance, 3*time.Second)

	// Publish boid suggestions: rank 1 = stale, rank 2 = good
	suggestions := []boidengine.SuggestedClaim{
		{AgentID: agentID, QuestID: staleQuestID, Score: 5.0, Confidence: 0.9, Reason: "stale top pick"},
		{AgentID: agentID, QuestID: goodQuestID, Score: 3.0, Confidence: 0.5, Reason: "good fallback"},
	}
	data, _ := json.Marshal(suggestions)
	if err := client.Publish(ctx, "boid.suggestions."+agentInstance, data); err != nil {
		t.Fatalf("Publish suggestions: %v", err)
	}

	// Wait for agent to claim the good quest (rank 2 fallthrough)
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for fallthrough quest claim")
		case <-time.After(100 * time.Millisecond):
			questEntity, err := gc.GetQuest(ctx, domain.QuestID(goodQuestID))
			if err != nil {
				continue
			}
			updatedQuest := semdragons.QuestFromEntityState(questEntity)
			if updatedQuest != nil && updatedQuest.Status == semdragons.QuestClaimed {
				if updatedQuest.ClaimedBy == nil || semdragons.AgentID(*updatedQuest.ClaimedBy) != agentID {
					t.Errorf("Good quest claimed by wrong agent: %v", updatedQuest.ClaimedBy)
				}
				return
			}
		}
	}
}

func TestAutonomousQuestClaim_AllStale(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "allstale"
	config.InitialDelayMs = 100
	config.IdleIntervalMs = 200

	comp := setupComponentWithConfig(t, client, config)
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.BoardConfig())

	// Create stale quest
	staleInstance := semdragons.GenerateInstance()
	staleQuestID := semdragons.QuestID(comp.BoardConfig().QuestEntityID(staleInstance))
	otherAgent := semdragons.AgentID("other.agent")
	claimTime := time.Now()
	staleQuest := &semdragons.Quest{
		ID:        staleQuestID,
		Title:     "Already claimed",
		Status:    semdragons.QuestClaimed,
		ClaimedBy: &otherAgent,
		ClaimedAt: &claimTime,
		PostedAt:  time.Now(),
	}
	if err := gc.PutEntityState(ctx, staleQuest, "quest.claimed"); err != nil {
		t.Fatalf("Failed to create stale quest: %v", err)
	}

	// Create idle agent
	agentInstance := semdragons.GenerateInstance()
	agentID := semdragons.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	agent := &semdragons.Agent{
		ID:        agentID,
		Name:      "allstale-agent",
		Status:    semdragons.AgentIdle,
		Level:     5,
		Tier:      semdragons.TierApprentice,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	waitForTracker(t, comp, agentInstance, 3*time.Second)

	// Publish suggestion for stale quest only
	suggestions := []boidengine.SuggestedClaim{
		{AgentID: agentID, QuestID: staleQuestID, Score: 5.0, Confidence: 0.9, Reason: "only option"},
	}
	data, _ := json.Marshal(suggestions)
	if err := client.Publish(ctx, "boid.suggestions."+agentInstance, data); err != nil {
		t.Fatalf("Publish suggestions: %v", err)
	}

	// Wait for a few heartbeats — agent should remain idle (no viable claim)
	time.Sleep(800 * time.Millisecond)

	agentEntity, err := gc.GetAgent(ctx, domain.AgentID(agentID))
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	updatedAgent := semdragons.AgentFromEntityState(agentEntity)
	if updatedAgent == nil {
		t.Fatal("Failed to reconstruct agent")
	}
	if updatedAgent.Status != semdragons.AgentIdle {
		t.Errorf("Agent status = %v, want %v (should remain idle when all suggestions stale)",
			updatedAgent.Status, semdragons.AgentIdle)
	}
}

// =============================================================================
// AUTONOMOUS STORE ACTION TESTS
// =============================================================================

func TestAutonomousIdleShopping(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "idleshop"
	config.InitialDelayMs = 100
	config.IdleIntervalMs = 200
	config.MinXPSurplusForShopping = 50

	// Set up agentstore alongside autonomy — inject before Start
	store := setupStoreComponent(t, client, "idleshop")
	defer store.Stop(5 * time.Second)

	comp := setupComponentWithDeps(t, client, config, store, nil)
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.BoardConfig())

	// Create idle agent with XP surplus (web_search costs 50 XP, apprentice tier)
	agentInstance := semdragons.GenerateInstance()
	agentID := semdragons.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	agent := &semdragons.Agent{
		ID:        agentID,
		Name:      "shopper-agent",
		Status:    semdragons.AgentIdle,
		Level:     5,
		XP:        500,
		XPToLevel: 300, // surplus = 200 > threshold 50
		Tier:      semdragons.TierApprentice,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	waitForTracker(t, comp, agentInstance, 3*time.Second)

	// Wait for a purchase to happen via heartbeat
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for autonomous purchase")
		case <-time.After(100 * time.Millisecond):
			agentEntity, err := gc.GetAgent(ctx, domain.AgentID(agentID))
			if err != nil {
				continue
			}
			updated := semdragons.AgentFromEntityState(agentEntity)
			if updated != nil && updated.TotalSpent > 0 {
				t.Logf("agent purchased item, total_spent=%d", updated.TotalSpent)
				return
			}
		}
	}
}

func TestAutonomousCooldownSkip(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "cdskip"
	config.InitialDelayMs = 100
	config.CooldownIntervalMs = 200
	config.CooldownSkipMinRemainingMs = 30000

	store := setupStoreComponent(t, client, "cdskip")
	defer store.Stop(5 * time.Second)

	comp := setupComponentWithDeps(t, client, config, store, nil)
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.BoardConfig())

	// Create agent in cooldown with future expiry and a cooldown_skip consumable
	agentInstance := semdragons.GenerateInstance()
	agentID := semdragons.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	future := time.Now().Add(1 * time.Hour) // well above 30s threshold
	agent := &semdragons.Agent{
		ID:            agentID,
		Name:          "cdskip-agent",
		Status:        semdragons.AgentCooldown,
		Level:         3,
		Tier:          semdragons.TierApprentice,
		CooldownUntil: &future,
		Consumables:   map[string]int{"cooldown_skip": 1},
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	// Seed inventory in store so UseConsumable can find it
	inv := store.GetInventory(domain.AgentID(agentID))
	inv.Consumables["cooldown_skip"] = 1

	waitForTracker(t, comp, agentInstance, 3*time.Second)

	// Wait for cooldown skip to happen
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for autonomous cooldown skip")
		case <-time.After(100 * time.Millisecond):
			agentEntity, err := gc.GetAgent(ctx, domain.AgentID(agentID))
			if err != nil {
				continue
			}
			updated := semdragons.AgentFromEntityState(agentEntity)
			if updated != nil && updated.Status == semdragons.AgentIdle {
				if updated.CooldownUntil != nil {
					t.Error("CooldownUntil should be nil after skip")
				}
				return
			}
		}
	}
}

func TestAutonomousConsumableUse_InBattle(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "battleuse"
	config.InitialDelayMs = 100
	config.InBattleIntervalMs = 200

	store := setupStoreComponent(t, client, "battleuse")
	defer store.Stop(5 * time.Second)

	comp := setupComponentWithDeps(t, client, config, store, nil)
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.BoardConfig())

	// Create agent in battle with quality_shield consumable
	agentInstance := semdragons.GenerateInstance()
	agentID := semdragons.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	agent := &semdragons.Agent{
		ID:          agentID,
		Name:        "battle-agent",
		Status:      semdragons.AgentInBattle,
		Level:       7,
		Tier:        semdragons.TierJourneyman,
		Consumables: map[string]int{"quality_shield": 1},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	// Seed inventory in store
	inv := store.GetInventory(domain.AgentID(agentID))
	inv.Consumables["quality_shield"] = 1

	waitForTracker(t, comp, agentInstance, 3*time.Second)

	// Wait for consumable use
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for autonomous quality_shield use in battle")
		case <-time.After(100 * time.Millisecond):
			effects := store.GetActiveEffects(domain.AgentID(agentID))
			if len(effects) > 0 && effects[0].Effect.Type == agentstore.ConsumableQualityShield {
				t.Log("agent used quality_shield in battle")
				return
			}
		}
	}
}

func TestNoShoppingWhenNoSurplus(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "nosurplus"
	config.InitialDelayMs = 100
	config.IdleIntervalMs = 200
	config.MinXPSurplusForShopping = 50

	store := setupStoreComponent(t, client, "nosurplus")
	defer store.Stop(5 * time.Second)

	comp := setupComponentWithDeps(t, client, config, store, nil)
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.BoardConfig())

	// Create idle agent with no surplus (XP < XPToLevel)
	agentInstance := semdragons.GenerateInstance()
	agentID := semdragons.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	agent := &semdragons.Agent{
		ID:        agentID,
		Name:      "poor-agent",
		Status:    semdragons.AgentIdle,
		Level:     2,
		XP:        50,
		XPToLevel: 200, // no surplus
		Tier:      semdragons.TierApprentice,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	waitForTracker(t, comp, agentInstance, 3*time.Second)

	// Wait for several heartbeats, verify no purchase
	time.Sleep(800 * time.Millisecond)

	agentEntity, err := gc.GetAgent(ctx, domain.AgentID(agentID))
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	updated := semdragons.AgentFromEntityState(agentEntity)
	if updated == nil {
		t.Fatal("Failed to reconstruct agent")
	}
	if updated.TotalSpent != 0 {
		t.Errorf("TotalSpent = %d, want 0 (no surplus to shop with)", updated.TotalSpent)
	}
}

// =============================================================================
// AUTONOMOUS GUILD JOINING TESTS
// =============================================================================

func TestAutonomousGuildJoining(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "guildjoin"
	config.InitialDelayMs = 100
	config.IdleIntervalMs = 200
	config.GuildJoinMinLevel = 3

	guilds := setupGuildComponent(t, client, "guildjoin")
	defer guilds.Stop(5 * time.Second)

	// SetGuilds must be called before Start (rejected while running).
	comp := setupComponentWithDeps(t, client, config, nil, guilds)
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.BoardConfig())

	// Create a guild to join
	guild, err := guilds.CreateGuild(ctx, guildformation.CreateGuildParams{
		Name:      "Test Guild Alpha",
		Culture:   "Test culture",
		FounderID: "test.integration.game.guildjoin.agent.founder",
		MinLevel:  1,
	})
	if err != nil {
		t.Fatalf("CreateGuild failed: %v", err)
	}

	// Create idle, unguilded agent at level 5 (above GuildJoinMinLevel=3)
	agentInstance := semdragons.GenerateInstance()
	agentID := semdragons.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	agent := &semdragons.Agent{
		ID:        agentID,
		Name:      "guild-joiner",
		Status:    semdragons.AgentIdle,
		Level:     5,
		Tier:      semdragons.TierApprentice,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	waitForTracker(t, comp, agentInstance, 3*time.Second)

	// Wait for the agent to autonomously join the guild
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for autonomous guild join")
		case <-time.After(100 * time.Millisecond):
			agentGuilds := guilds.GetAgentGuilds(domain.AgentID(agentID))
			if len(agentGuilds) > 0 {
				// Verify they joined the correct guild
				found := false
				for _, g := range agentGuilds {
					if g == domain.GuildID(guild.ID) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("agent joined wrong guild, got %v, want %v", agentGuilds, guild.ID)
				}
				return
			}
		}
	}
}

func TestNoGuildJoiningBelowMinLevel(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "guildlowlvl"
	config.InitialDelayMs = 100
	config.IdleIntervalMs = 200
	config.GuildJoinMinLevel = 10 // High threshold

	guilds := setupGuildComponent(t, client, "guildlowlvl")
	defer guilds.Stop(5 * time.Second)

	comp := setupComponentWithDeps(t, client, config, nil, guilds)
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.BoardConfig())

	// Create a guild
	if _, err := guilds.CreateGuild(ctx, guildformation.CreateGuildParams{
		Name:      "High Level Guild",
		Culture:   "Veterans only",
		FounderID: "test.integration.game.guildlowlvl.agent.founder",
		MinLevel:  1,
	}); err != nil {
		t.Fatalf("CreateGuild failed: %v", err)
	}

	// Create low-level agent (level 3 < GuildJoinMinLevel 10)
	agentInstance := semdragons.GenerateInstance()
	agentID := semdragons.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	agent := &semdragons.Agent{
		ID:        agentID,
		Name:      "lowlevel-agent",
		Status:    semdragons.AgentIdle,
		Level:     3,
		Tier:      semdragons.TierApprentice,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	waitForTracker(t, comp, agentInstance, 3*time.Second)

	// Wait for several heartbeats — agent should NOT join
	time.Sleep(800 * time.Millisecond)

	agentGuilds := guilds.GetAgentGuilds(domain.AgentID(agentID))
	if len(agentGuilds) > 0 {
		t.Errorf("agent at level 3 should not join guild (min level = 10), but joined %v", agentGuilds)
	}
}

func TestNoGuildJoiningAtMaxGuilds(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "guildmax"
	config.InitialDelayMs = 100
	config.IdleIntervalMs = 200
	config.GuildJoinMinLevel = 1
	config.MaxGuildsPerAgent = 1 // Max 1 guild

	guilds := setupGuildComponent(t, client, "guildmax")
	defer guilds.Stop(5 * time.Second)

	comp := setupComponentWithDeps(t, client, config, nil, guilds)
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.BoardConfig())

	// Create two guilds
	if _, err := guilds.CreateGuild(ctx, guildformation.CreateGuildParams{
		Name:      "Guild One",
		Culture:   "First guild",
		FounderID: "test.integration.game.guildmax.agent.founder1",
		MinLevel:  1,
	}); err != nil {
		t.Fatalf("CreateGuild 1 failed: %v", err)
	}
	if _, err := guilds.CreateGuild(ctx, guildformation.CreateGuildParams{
		Name:      "Guild Two",
		Culture:   "Second guild",
		FounderID: "test.integration.game.guildmax.agent.founder2",
		MinLevel:  1,
	}); err != nil {
		t.Fatalf("CreateGuild 2 failed: %v", err)
	}

	// Create agent already in 1 guild (at max)
	agentInstance := semdragons.GenerateInstance()
	agentID := semdragons.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	agent := &semdragons.Agent{
		ID:        agentID,
		Name:      "maxguild-agent",
		Status:    semdragons.AgentIdle,
		Level:     5,
		Tier:      semdragons.TierApprentice,
		Guilds:    []semdragons.GuildID{"existing-guild"}, // already at max (1)
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	waitForTracker(t, comp, agentInstance, 3*time.Second)

	// Wait for several heartbeats — agent should NOT join another guild
	time.Sleep(800 * time.Millisecond)

	agentGuilds := guilds.GetAgentGuilds(domain.AgentID(agentID))
	if len(agentGuilds) > 0 {
		t.Errorf("agent at MaxGuildsPerAgent should not join more guilds, but joined %v", agentGuilds)
	}
}

func TestNoGuildJoiningWithoutComponent(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "noguildcomp"
	config.InitialDelayMs = 100
	config.IdleIntervalMs = 200

	comp := setupComponentWithConfig(t, client, config)
	defer comp.Stop(5 * time.Second)
	// No SetGuilds call — guilds remains nil

	gc := semdragons.NewGraphClient(client, comp.BoardConfig())

	// Create idle agent
	agentInstance := semdragons.GenerateInstance()
	agentID := semdragons.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	agent := &semdragons.Agent{
		ID:        agentID,
		Name:      "no-guild-comp-agent",
		Status:    semdragons.AgentIdle,
		Level:     10,
		Tier:      semdragons.TierJourneyman,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	waitForTracker(t, comp, agentInstance, 3*time.Second)

	// Wait for several heartbeats — no panics, no errors
	time.Sleep(800 * time.Millisecond)

	// Component should still be healthy
	health := comp.Health()
	if !health.Healthy {
		t.Error("component should remain healthy with nil guilds component")
	}
}

func TestNoShoppingWithoutStore(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "nostore"
	config.InitialDelayMs = 100
	config.IdleIntervalMs = 200

	comp := setupComponentWithConfig(t, client, config)
	defer comp.Stop(5 * time.Second)
	// No SetStore call — store remains nil

	gc := semdragons.NewGraphClient(client, comp.BoardConfig())

	// Create rich idle agent — should NOT shop because store is nil
	agentInstance := semdragons.GenerateInstance()
	agentID := semdragons.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	agent := &semdragons.Agent{
		ID:        agentID,
		Name:      "no-store-agent",
		Status:    semdragons.AgentIdle,
		Level:     10,
		XP:        9999,
		XPToLevel: 100,
		Tier:      semdragons.TierJourneyman,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	waitForTracker(t, comp, agentInstance, 3*time.Second)

	// Wait for several heartbeats
	time.Sleep(800 * time.Millisecond)

	agentEntity, err := gc.GetAgent(ctx, domain.AgentID(agentID))
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	updated := semdragons.AgentFromEntityState(agentEntity)
	if updated == nil {
		t.Fatal("Failed to reconstruct agent")
	}
	if updated.TotalSpent != 0 {
		t.Errorf("TotalSpent = %d, want 0 (store is nil)", updated.TotalSpent)
	}
}

// =============================================================================
// HELPERS
// =============================================================================

// setupGuildComponent creates and starts a guildformation component for integration tests.
func setupGuildComponent(t *testing.T, client *natsclient.Client, board string) *guildformation.Component {
	t.Helper()

	deps := component.Dependencies{NATSClient: client}
	guildCfg := guildformation.DefaultConfig()
	guildCfg.Org = "test"
	guildCfg.Platform = "integration"
	guildCfg.Board = board
	guildCfg.EnableAutoFormation = false // No KV watcher needed; only CRUD operations

	gc, err := guildformation.NewFromConfig(guildCfg, deps)
	if err != nil {
		t.Fatalf("guildformation NewFromConfig failed: %v", err)
	}
	if err := gc.Initialize(); err != nil {
		t.Fatalf("guildformation Initialize failed: %v", err)
	}
	if err := gc.Start(context.Background()); err != nil {
		t.Fatalf("guildformation Start failed: %v", err)
	}
	return gc
}

// setupStoreComponent creates and starts an agentstore component for integration tests.
// Ensures the board-specific KV bucket exists before Start (required for WatchEntityType).
func setupStoreComponent(t *testing.T, client *natsclient.Client, board string) *agentstore.Component {
	t.Helper()

	deps := component.Dependencies{NATSClient: client}
	storeCfg := agentstore.DefaultConfig()
	storeCfg.Org = "test"
	storeCfg.Platform = "integration"
	storeCfg.Board = board

	store, err := agentstore.NewFromConfig(storeCfg, deps)
	if err != nil {
		t.Fatalf("agentstore NewFromConfig failed: %v", err)
	}
	if err := store.Initialize(); err != nil {
		t.Fatalf("agentstore Initialize failed: %v", err)
	}

	// Ensure board-specific KV bucket exists before Start
	boardCfg := &domain.BoardConfig{Org: "test", Platform: "integration", Board: board}
	gc := semdragons.NewGraphClient(client, boardCfg)
	if err := gc.EnsureBucket(context.Background()); err != nil {
		t.Fatalf("EnsureBucket for agentstore failed: %v", err)
	}

	if err := store.Start(context.Background()); err != nil {
		t.Fatalf("agentstore Start failed: %v", err)
	}
	return store
}

// waitForTracker polls until the autonomy component has created a heartbeat
// tracker for the given agent instance, or calls t.Fatalf on timeout.
func waitForTracker(t *testing.T, comp *Component, instance string, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for agent tracker: %s", instance)
		case <-time.After(50 * time.Millisecond):
			comp.trackersMu.RLock()
			_, exists := comp.trackers[instance]
			comp.trackersMu.RUnlock()
			if exists {
				return
			}
		}
	}
}

func setupComponent(t *testing.T, client *natsclient.Client, name string) *Component {
	t.Helper()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = name

	return setupComponentWithConfig(t, client, config)
}

// setupComponentWithDeps creates a component with store and/or guilds injected BEFORE Start.
// SetStore/SetGuilds must be called before Start because the running guard rejects them.
func setupComponentWithDeps(t *testing.T, client *natsclient.Client, config Config, store *agentstore.Component, guilds *guildformation.Component) *Component {
	t.Helper()

	deps := component.Dependencies{NATSClient: client}
	ctx := context.Background()

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	gc := semdragons.NewGraphClient(client, comp.BoardConfig())
	if err := gc.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket failed: %v", err)
	}

	// Inject dependencies BEFORE Start
	if store != nil {
		comp.SetStore(store)
	}
	if guilds != nil {
		comp.SetGuilds(guilds)
	}

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	return comp
}

func setupComponentWithConfig(t *testing.T, client *natsclient.Client, config Config) *Component {
	t.Helper()

	deps := component.Dependencies{
		NATSClient: client,
	}

	ctx := context.Background()

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}

	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Ensure board-specific KV bucket exists
	gc := semdragons.NewGraphClient(client, comp.BoardConfig())
	if err := gc.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket failed: %v", err)
	}

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	return comp
}
