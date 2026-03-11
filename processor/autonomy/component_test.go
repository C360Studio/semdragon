//go:build integration

package autonomy

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/agentstore"
	"github.com/c360studio/semdragons/processor/boidengine"
	"github.com/c360studio/semdragons/processor/dmapproval"
	"github.com/c360studio/semdragons/processor/guildformation"
)

// =============================================================================
// INTEGRATION TESTS - Autonomy Component
// =============================================================================
// These tests require Docker for NATS via testcontainers.
// Run with: go test -tags=integration -count=1 -v ./processor/autonomy/...
// =============================================================================

func TestComponent_Lifecycle(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
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
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
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
	instance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(instance))
	expired := time.Now().Add(-1 * time.Hour) // Already expired
	agent := &agentprogression.Agent{
		ID:            agentID,
		Name:          "cooldown-agent",
		Status:        domain.AgentCooldown,
		Level:         3,
		Tier:          domain.TierApprentice,
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
			updated := agentprogression.AgentFromEntityState(entity)
			if updated != nil && updated.Status == domain.AgentIdle {
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
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
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
	instance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(instance))
	future := time.Now().Add(1 * time.Hour) // Not expired
	agent := &agentprogression.Agent{
		ID:            agentID,
		Name:          "future-cooldown-agent",
		Status:        domain.AgentCooldown,
		Level:         3,
		Tier:          domain.TierApprentice,
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
	updated := agentprogression.AgentFromEntityState(entity)
	if updated == nil {
		t.Fatal("Failed to reconstruct agent")
	}
	if updated.Status != domain.AgentCooldown {
		t.Errorf("Status = %v, want %v (cooldown not expired)", updated.Status, domain.AgentCooldown)
	}
}

func TestBoidSuggestionCached(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
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
	instance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(instance))
	agent := &agentprogression.Agent{
		ID:        agentID,
		Name:      "boid-test-agent",
		Status:    domain.AgentIdle,
		Level:     5,
		Tier:      domain.TierApprentice,
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
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
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
	instance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(instance))
	agent := &agentprogression.Agent{
		ID:        agentID,
		Name:      "heartbeat-test-agent",
		Status:    domain.AgentIdle,
		Level:     1,
		Tier:      domain.TierApprentice,
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
				expectedInterval := config.IntervalForStatus(domain.AgentIdle)
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
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
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
	instance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(instance))
	agent := &agentprogression.Agent{
		ID:        agentID,
		Name:      "retire-test-agent",
		Status:    domain.AgentIdle,
		Level:     1,
		Tier:      domain.TierApprentice,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	// Wait for tracker to appear
	waitForTracker(t, comp, instance, 3*time.Second)

	// Retire agent
	agent.Status = domain.AgentRetired
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
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
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
	questInstance := domain.GenerateInstance()
	questID := domain.QuestID(comp.BoardConfig().QuestEntityID(questInstance))
	quest := &domain.Quest{
		ID:       questID,
		Title:    "Auto-claim test quest",
		Status:   domain.QuestPosted,
		PostedAt: time.Now(),
		MinTier:  domain.TierApprentice,
		BaseXP:   100,
	}
	if err := gc.PutEntityState(ctx, quest, "quest.lifecycle.posted"); err != nil {
		t.Fatalf("Failed to create test quest: %v", err)
	}

	// Create idle agent
	agentInstance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	agent := &agentprogression.Agent{
		ID:        agentID,
		Name:      "autoclaim-agent",
		Status:    domain.AgentIdle,
		Level:     5,
		Tier:      domain.TierApprentice,
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
			updatedQuest := domain.QuestFromEntityState(questEntity)
			// Accept QuestInProgress too: executeClaimQuest transitions claimed→in_progress immediately.
			if updatedQuest == nil || (updatedQuest.Status != domain.QuestClaimed && updatedQuest.Status != domain.QuestInProgress) {
				continue
			}

			// Quest is claimed (or already started)! Verify agent is on_quest
			agentEntity, err := gc.GetAgent(ctx, domain.AgentID(agentID))
			if err != nil {
				t.Fatalf("GetAgent failed: %v", err)
			}
			updatedAgent := agentprogression.AgentFromEntityState(agentEntity)
			if updatedAgent == nil {
				t.Fatal("Failed to reconstruct agent")
			}
			if updatedAgent.Status != domain.AgentOnQuest {
				t.Errorf("Agent status = %v, want %v", updatedAgent.Status, domain.AgentOnQuest)
			}
			if updatedAgent.CurrentQuest == nil || *updatedAgent.CurrentQuest != questID {
				t.Errorf("Agent CurrentQuest = %v, want %v", updatedAgent.CurrentQuest, questID)
			}
			if updatedQuest.ClaimedBy == nil || domain.AgentID(*updatedQuest.ClaimedBy) != agentID {
				t.Errorf("Quest ClaimedBy = %v, want %v", updatedQuest.ClaimedBy, agentID)
			}
			return
		}
	}
}

func TestAutonomousQuestClaim_FallsThrough(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
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
	staleInstance := domain.GenerateInstance()
	staleQuestID := domain.QuestID(comp.BoardConfig().QuestEntityID(staleInstance))
	staleAgentID := domain.AgentID("some.other.agent")
	now := time.Now()
	staleQuest := &domain.Quest{
		ID:        staleQuestID,
		Title:     "Already claimed quest",
		Status:    domain.QuestClaimed,
		ClaimedBy: &staleAgentID,
		ClaimedAt: &now,
		PostedAt:  now,
	}
	if err := gc.PutEntityState(ctx, staleQuest, "quest.claimed"); err != nil {
		t.Fatalf("Failed to create stale quest: %v", err)
	}

	// Create a good quest (still posted — will be claimed as fallthrough)
	goodInstance := domain.GenerateInstance()
	goodQuestID := domain.QuestID(comp.BoardConfig().QuestEntityID(goodInstance))
	goodQuest := &domain.Quest{
		ID:       goodQuestID,
		Title:    "Good fallthrough quest",
		Status:   domain.QuestPosted,
		PostedAt: time.Now(),
		MinTier:  domain.TierApprentice,
		BaseXP:   100,
	}
	if err := gc.PutEntityState(ctx, goodQuest, "quest.lifecycle.posted"); err != nil {
		t.Fatalf("Failed to create good quest: %v", err)
	}

	// Create idle agent
	agentInstance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	agent := &agentprogression.Agent{
		ID:        agentID,
		Name:      "fallthrough-agent",
		Status:    domain.AgentIdle,
		Level:     5,
		Tier:      domain.TierApprentice,
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
			updatedQuest := domain.QuestFromEntityState(questEntity)
			// Accept QuestInProgress too: executeClaimQuest transitions claimed→in_progress immediately.
			if updatedQuest != nil && (updatedQuest.Status == domain.QuestClaimed || updatedQuest.Status == domain.QuestInProgress) {
				if updatedQuest.ClaimedBy == nil || domain.AgentID(*updatedQuest.ClaimedBy) != agentID {
					t.Errorf("Good quest claimed by wrong agent: %v", updatedQuest.ClaimedBy)
				}
				return
			}
		}
	}
}

func TestAutonomousQuestClaim_AllStale(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
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
	staleInstance := domain.GenerateInstance()
	staleQuestID := domain.QuestID(comp.BoardConfig().QuestEntityID(staleInstance))
	otherAgent := domain.AgentID("other.agent")
	claimTime := time.Now()
	staleQuest := &domain.Quest{
		ID:        staleQuestID,
		Title:     "Already claimed",
		Status:    domain.QuestClaimed,
		ClaimedBy: &otherAgent,
		ClaimedAt: &claimTime,
		PostedAt:  time.Now(),
	}
	if err := gc.PutEntityState(ctx, staleQuest, "quest.claimed"); err != nil {
		t.Fatalf("Failed to create stale quest: %v", err)
	}

	// Create idle agent
	agentInstance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	agent := &agentprogression.Agent{
		ID:        agentID,
		Name:      "allstale-agent",
		Status:    domain.AgentIdle,
		Level:     5,
		Tier:      domain.TierApprentice,
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
	updatedAgent := agentprogression.AgentFromEntityState(agentEntity)
	if updatedAgent == nil {
		t.Fatal("Failed to reconstruct agent")
	}
	if updatedAgent.Status != domain.AgentIdle {
		t.Errorf("Agent status = %v, want %v (should remain idle when all suggestions stale)",
			updatedAgent.Status, domain.AgentIdle)
	}
}

// =============================================================================
// AUTONOMOUS STORE ACTION TESTS
// =============================================================================

func TestAutonomousIdleShopping(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
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
	agentInstance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	agent := &agentprogression.Agent{
		ID:        agentID,
		Name:      "shopper-agent",
		Status:    domain.AgentIdle,
		Level:     5,
		XP:        500,
		XPToLevel: 300, // surplus = 200 > threshold 50
		Tier:      domain.TierApprentice,
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
			updated := agentprogression.AgentFromEntityState(agentEntity)
			if updated != nil && updated.TotalSpent > 0 {
				t.Logf("agent purchased item, total_spent=%d", updated.TotalSpent)
				return
			}
		}
	}
}

func TestAutonomousCooldownSkip(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
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
	agentInstance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	future := time.Now().Add(1 * time.Hour) // well above 30s threshold
	agent := &agentprogression.Agent{
		ID:            agentID,
		Name:          "cdskip-agent",
		Status:        domain.AgentCooldown,
		Level:         3,
		Tier:          domain.TierApprentice,
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
	inv.SetConsumable("cooldown_skip", 1)

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
			updated := agentprogression.AgentFromEntityState(agentEntity)
			if updated != nil && updated.Status == domain.AgentIdle {
				if updated.CooldownUntil != nil {
					t.Error("CooldownUntil should be nil after skip")
				}
				return
			}
		}
	}
}

func TestAutonomousConsumableUse_InBattle(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
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
	agentInstance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	agent := &agentprogression.Agent{
		ID:          agentID,
		Name:        "battle-agent",
		Status:      domain.AgentInBattle,
		Level:       7,
		Tier:        domain.TierJourneyman,
		Consumables: map[string]int{"quality_shield": 1},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	// Seed inventory in store
	inv := store.GetInventory(domain.AgentID(agentID))
	inv.SetConsumable("quality_shield", 1)

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
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
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
	agentInstance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	agent := &agentprogression.Agent{
		ID:        agentID,
		Name:      "poor-agent",
		Status:    domain.AgentIdle,
		Level:     2,
		XP:        50,
		XPToLevel: 200, // no surplus
		Tier:      domain.TierApprentice,
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
	updated := agentprogression.AgentFromEntityState(agentEntity)
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
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
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
	agentInstance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	agent := &agentprogression.Agent{
		ID:        agentID,
		Name:      "guild-joiner",
		Status:    domain.AgentIdle,
		Level:     5,
		Tier:      domain.TierApprentice,
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
			agentGuild := guilds.GetAgentGuild(domain.AgentID(agentID))
			if agentGuild != "" {
				// Verify they joined the correct guild
				if agentGuild != domain.GuildID(guild.ID) {
					t.Errorf("agent joined wrong guild, got %v, want %v", agentGuild, guild.ID)
				}
				return
			}
		}
	}
}

func TestNoGuildJoiningBelowMinLevel(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
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
	agentInstance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	agent := &agentprogression.Agent{
		ID:        agentID,
		Name:      "lowlevel-agent",
		Status:    domain.AgentIdle,
		Level:     3,
		Tier:      domain.TierApprentice,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	waitForTracker(t, comp, agentInstance, 3*time.Second)

	// Wait for several heartbeats — agent should NOT join
	time.Sleep(800 * time.Millisecond)

	agentGuild := guilds.GetAgentGuild(domain.AgentID(agentID))
	if agentGuild != "" {
		t.Errorf("agent at level 3 should not join guild (min level = 10), but joined %v", agentGuild)
	}
}

func TestNoGuildJoiningWhenAlreadyGuilded(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "guildmax"
	config.InitialDelayMs = 100
	config.IdleIntervalMs = 200
	config.GuildJoinMinLevel = 1

	guilds := setupGuildComponent(t, client, "guildmax")
	defer guilds.Stop(5 * time.Second)

	comp := setupComponentWithDeps(t, client, config, nil, guilds)
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.BoardConfig())

	// Create two guilds — the agent should not join either since it already has one
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

	// Create agent already belonging to a guild — single-guild semantics prevent joining another
	agentInstance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	agent := &agentprogression.Agent{
		ID:        agentID,
		Name:      "alreadyguilded-agent",
		Status:    domain.AgentIdle,
		Level:     5,
		Tier:      domain.TierApprentice,
		Guild:     "existing-guild", // already in a guild
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	waitForTracker(t, comp, agentInstance, 3*time.Second)

	// Wait for several heartbeats — agent should NOT join another guild
	time.Sleep(800 * time.Millisecond)

	agentGuild := guilds.GetAgentGuild(domain.AgentID(agentID))
	if agentGuild != "" {
		t.Errorf("agent already in a guild should not join another, but joined %v", agentGuild)
	}
}

func TestNoGuildJoiningWithoutComponent(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
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
	agentInstance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	agent := &agentprogression.Agent{
		ID:        agentID,
		Name:      "no-guild-comp-agent",
		Status:    domain.AgentIdle,
		Level:     10,
		Tier:      domain.TierJourneyman,
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
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
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
	agentInstance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	agent := &agentprogression.Agent{
		ID:        agentID,
		Name:      "no-store-agent",
		Status:    domain.AgentIdle,
		Level:     10,
		XP:        9999,
		XPToLevel: 100,
		Tier:      domain.TierJourneyman,
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
	updated := agentprogression.AgentFromEntityState(agentEntity)
	if updated == nil {
		t.Fatal("Failed to reconstruct agent")
	}
	if updated.TotalSpent != 0 {
		t.Errorf("TotalSpent = %d, want 0 (store is nil)", updated.TotalSpent)
	}
}

// =============================================================================
// INTENT EMISSION TESTS
// =============================================================================

func TestClaimIntentEmitted(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "claimintent"
	config.InitialDelayMs = 100
	config.IdleIntervalMs = 200

	comp := setupComponentWithConfig(t, client, config)
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.BoardConfig())

	// Subscribe to claim intent before triggering action
	intentCh := make(chan *ClaimIntentPayload, 1)
	sub, err := client.Subscribe(ctx, domain.PredicateAutonomyClaimIntent, func(_ context.Context, msg *nats.Msg) {
		var payload ClaimIntentPayload
		if err := json.Unmarshal(msg.Data, &payload); err == nil {
			select {
			case intentCh <- &payload:
			default:
			}
		}
	})
	if err != nil {
		t.Fatalf("Subscribe claim intent failed: %v", err)
	}
	defer sub.Unsubscribe()

	// Create a posted quest
	questInstance := domain.GenerateInstance()
	questID := domain.QuestID(comp.BoardConfig().QuestEntityID(questInstance))
	quest := &domain.Quest{
		ID:       questID,
		Title:    "Intent Test Quest",
		Status:   domain.QuestPosted,
		PostedAt: time.Now(),
	}
	if err := gc.PutEntityState(ctx, quest, "quest.posted"); err != nil {
		t.Fatalf("Failed to create quest: %v", err)
	}

	// Create idle agent
	agentInstance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	agent := &agentprogression.Agent{
		ID:        agentID,
		Name:      "claim-intent-agent",
		Status:    domain.AgentIdle,
		Level:     5,
		Tier:      domain.TierApprentice,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	waitForTracker(t, comp, agentInstance, 3*time.Second)

	// Publish boid suggestion
	suggestions := []boidengine.SuggestedClaim{
		{AgentID: agentID, QuestID: questID, Score: 4.2, Confidence: 0.9, Reason: "test"},
	}
	data, _ := json.Marshal(suggestions)
	if err := client.Publish(ctx, "boid.suggestions."+agentInstance, data); err != nil {
		t.Fatalf("Publish suggestions: %v", err)
	}

	// Wait for claim intent event
	select {
	case payload := <-intentCh:
		if payload.AgentID != domain.AgentID(agentID) {
			t.Errorf("ClaimIntent AgentID = %q, want %q", payload.AgentID, agentID)
		}
		if payload.QuestID != questID {
			t.Errorf("ClaimIntent QuestID = %q, want %q", payload.QuestID, questID)
		}
		if payload.Score != 4.2 {
			t.Errorf("ClaimIntent Score = %f, want 4.2", payload.Score)
		}
		if payload.SuggestionRank != 1 {
			t.Errorf("ClaimIntent SuggestionRank = %d, want 1", payload.SuggestionRank)
		}
		t.Log("claim intent received successfully")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for claim intent event")
	}
}

func TestShopIntentEmitted(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "shopintent"
	config.InitialDelayMs = 100
	config.IdleIntervalMs = 200
	config.MinXPSurplusForShopping = 50

	store := setupStoreComponent(t, client, "shopintent")
	defer store.Stop(5 * time.Second)

	comp := setupComponentWithDeps(t, client, config, store, nil)
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.BoardConfig())

	// Subscribe to shop intent
	intentCh := make(chan *ShopIntentPayload, 1)
	sub, err := client.Subscribe(ctx, domain.PredicateAutonomyShopIntent, func(_ context.Context, msg *nats.Msg) {
		var payload ShopIntentPayload
		if err := json.Unmarshal(msg.Data, &payload); err == nil {
			select {
			case intentCh <- &payload:
			default:
			}
		}
	})
	if err != nil {
		t.Fatalf("Subscribe shop intent failed: %v", err)
	}
	defer sub.Unsubscribe()

	// Create idle agent with XP surplus
	agentInstance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	agent := &agentprogression.Agent{
		ID:        agentID,
		Name:      "shop-intent-agent",
		Status:    domain.AgentIdle,
		Level:     5,
		XP:        500,
		XPToLevel: 300,
		Tier:      domain.TierApprentice,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	waitForTracker(t, comp, agentInstance, 3*time.Second)

	// Wait for shop intent event
	select {
	case payload := <-intentCh:
		if payload.AgentID != domain.AgentID(agentID) {
			t.Errorf("ShopIntent AgentID = %q, want %q", payload.AgentID, agentID)
		}
		if payload.ItemID == "" {
			t.Error("ShopIntent ItemID should not be empty")
		}
		if payload.XPCost <= 0 {
			t.Errorf("ShopIntent XPCost = %d, want > 0", payload.XPCost)
		}
		if payload.Strategic {
			t.Error("ShopIntent Strategic should be false for idle shopping")
		}
		t.Logf("shop intent received: item=%s cost=%d", payload.ItemName, payload.XPCost)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for shop intent event")
	}
}

func TestGuildIntentEmitted(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "guildintent"
	config.InitialDelayMs = 100
	config.IdleIntervalMs = 200
	config.GuildJoinMinLevel = 3

	guilds := setupGuildComponent(t, client, "guildintent")
	defer guilds.Stop(5 * time.Second)

	comp := setupComponentWithDeps(t, client, config, nil, guilds)
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.BoardConfig())

	// Subscribe to guild intent
	intentCh := make(chan *GuildIntentPayload, 1)
	sub, err := client.Subscribe(ctx, domain.PredicateAutonomyGuildIntent, func(_ context.Context, msg *nats.Msg) {
		var payload GuildIntentPayload
		if err := json.Unmarshal(msg.Data, &payload); err == nil {
			select {
			case intentCh <- &payload:
			default:
			}
		}
	})
	if err != nil {
		t.Fatalf("Subscribe guild intent failed: %v", err)
	}
	defer sub.Unsubscribe()

	// Create a guild
	guild, err := guilds.CreateGuild(ctx, guildformation.CreateGuildParams{
		Name:      "Intent Test Guild",
		Culture:   "Testing",
		FounderID: "test.integration.game.guildintent.agent.founder",
		MinLevel:  1,
	})
	if err != nil {
		t.Fatalf("CreateGuild failed: %v", err)
	}

	// Create idle unguilded agent
	agentInstance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	agent := &agentprogression.Agent{
		ID:        agentID,
		Name:      "guild-intent-agent",
		Status:    domain.AgentIdle,
		Level:     5,
		Tier:      domain.TierApprentice,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	waitForTracker(t, comp, agentInstance, 3*time.Second)

	// Wait for guild intent event
	select {
	case payload := <-intentCh:
		if payload.AgentID != domain.AgentID(agentID) {
			t.Errorf("GuildIntent AgentID = %q, want %q", payload.AgentID, agentID)
		}
		if payload.GuildID != string(guild.ID) {
			t.Errorf("GuildIntent GuildID = %q, want %q", payload.GuildID, guild.ID)
		}
		if payload.Score <= 0 {
			t.Errorf("GuildIntent Score = %f, want > 0", payload.Score)
		}
		if payload.ChoicesEvaluated < 1 {
			t.Errorf("GuildIntent ChoicesEvaluated = %d, want >= 1", payload.ChoicesEvaluated)
		}
		t.Logf("guild intent received: guild=%s score=%.3f", payload.GuildName, payload.Score)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for guild intent event")
	}
}

func TestUseIntentEmitted(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "useintent"
	config.InitialDelayMs = 100
	config.InBattleIntervalMs = 200

	store := setupStoreComponent(t, client, "useintent")
	defer store.Stop(5 * time.Second)

	comp := setupComponentWithDeps(t, client, config, store, nil)
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.BoardConfig())

	// Subscribe to use intent
	intentCh := make(chan *UseIntentPayload, 1)
	sub, err := client.Subscribe(ctx, domain.PredicateAutonomyUseIntent, func(_ context.Context, msg *nats.Msg) {
		var payload UseIntentPayload
		if err := json.Unmarshal(msg.Data, &payload); err == nil {
			select {
			case intentCh <- &payload:
			default:
			}
		}
	})
	if err != nil {
		t.Fatalf("Subscribe use intent failed: %v", err)
	}
	defer sub.Unsubscribe()

	// Create agent in battle with quality_shield consumable
	agentInstance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	agent := &agentprogression.Agent{
		ID:          agentID,
		Name:        "use-intent-agent",
		Status:      domain.AgentInBattle,
		Level:       7,
		Tier:        domain.TierJourneyman,
		Consumables: map[string]int{"quality_shield": 1},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	// Seed inventory in store
	inv := store.GetInventory(domain.AgentID(agentID))
	inv.SetConsumable("quality_shield", 1)

	waitForTracker(t, comp, agentInstance, 3*time.Second)

	// Wait for use intent event
	select {
	case payload := <-intentCh:
		if payload.AgentID != domain.AgentID(agentID) {
			t.Errorf("UseIntent AgentID = %q, want %q", payload.AgentID, agentID)
		}
		if payload.ConsumableID != "quality_shield" {
			t.Errorf("UseIntent ConsumableID = %q, want %q", payload.ConsumableID, "quality_shield")
		}
		if payload.AgentStatus != domain.AgentInBattle {
			t.Errorf("UseIntent AgentStatus = %q, want %q", payload.AgentStatus, domain.AgentInBattle)
		}
		t.Log("use intent received: consumable=quality_shield")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for use intent event")
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

// =============================================================================
// APPROVAL GATE INTEGRATION TESTS
// =============================================================================

func TestClaimApprovalGate_FullAuto(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "approvefa"
	config.InitialDelayMs = 100
	config.IdleIntervalMs = 200
	config.DMMode = domain.DMFullAuto // default — no approval needed

	comp := setupComponentWithConfig(t, client, config)
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.BoardConfig())

	// Create a posted quest
	questInstance := domain.GenerateInstance()
	questID := domain.QuestID(comp.BoardConfig().QuestEntityID(questInstance))
	quest := &domain.Quest{
		ID:       questID,
		Title:    "FullAuto approval test quest",
		Status:   domain.QuestPosted,
		PostedAt: time.Now(),
		MinTier:  domain.TierApprentice,
		BaseXP:   100,
	}
	if err := gc.PutEntityState(ctx, quest, "quest.lifecycle.posted"); err != nil {
		t.Fatalf("Failed to create test quest: %v", err)
	}

	// Create idle agent
	agentInstance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	agent := &agentprogression.Agent{
		ID:        agentID,
		Name:      "fullauto-agent",
		Status:    domain.AgentIdle,
		Level:     5,
		Tier:      domain.TierApprentice,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	waitForTracker(t, comp, agentInstance, 3*time.Second)

	// Publish boid suggestion
	suggestions := []boidengine.SuggestedClaim{
		{AgentID: agentID, QuestID: questID, Score: 5.0, Confidence: 0.9, Reason: "test fullauto claim"},
	}
	data, err := json.Marshal(suggestions)
	if err != nil {
		t.Fatalf("Marshal suggestions: %v", err)
	}
	if err := client.Publish(ctx, "boid.suggestions."+agentInstance, data); err != nil {
		t.Fatalf("Publish suggestions: %v", err)
	}

	// Wait for autonomous claim — should succeed without any approval
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for FullAuto claim (no approval needed)")
		case <-time.After(100 * time.Millisecond):
			questEntity, err := gc.GetQuest(ctx, domain.QuestID(questID))
			if err != nil {
				continue
			}
			updatedQuest := domain.QuestFromEntityState(questEntity)
			// Accept QuestInProgress too: executeClaimQuest transitions claimed→in_progress immediately.
			if updatedQuest != nil && (updatedQuest.Status == domain.QuestClaimed || updatedQuest.Status == domain.QuestInProgress) {
				return // success: claimed without approval
			}
		}
	}
}

func TestClaimApprovalGate_Supervised_Approved(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	// Set up dmapproval component with auto-approve
	approvalCfg := dmapproval.DefaultConfig()
	approvalCfg.Org = "test"
	approvalCfg.Platform = "integration"
	approvalCfg.Board = "approvesu"
	approvalCfg.AutoApprove = true // Auto-approve all requests

	deps := component.Dependencies{NATSClient: client}
	approvalComp, err := dmapproval.NewFromConfig(approvalCfg, deps)
	if err != nil {
		t.Fatalf("dmapproval NewFromConfig failed: %v", err)
	}
	if err := approvalComp.Initialize(); err != nil {
		t.Fatalf("dmapproval Initialize failed: %v", err)
	}
	if err := approvalComp.Start(ctx); err != nil {
		t.Fatalf("dmapproval Start failed: %v", err)
	}
	defer approvalComp.Stop(5 * time.Second)

	// Set up autonomy component with supervised mode
	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "approvesu"
	config.InitialDelayMs = 100
	config.IdleIntervalMs = 200
	config.DMMode = domain.DMSupervised
	config.SessionID = "test.integration.game.approvesu.session.s1"
	config.ApprovalTimeoutMs = 5000 // 5 second timeout

	autonomyComp := setupComponentWithDeps(t, client, config, nil, nil, approvalComp)
	defer autonomyComp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, autonomyComp.BoardConfig())

	// Create a posted quest
	questInstance := domain.GenerateInstance()
	questID := domain.QuestID(autonomyComp.BoardConfig().QuestEntityID(questInstance))
	quest := &domain.Quest{
		ID:       questID,
		Title:    "Supervised approval test quest",
		Status:   domain.QuestPosted,
		PostedAt: time.Now(),
		MinTier:  domain.TierApprentice,
		BaseXP:   100,
	}
	if err := gc.PutEntityState(ctx, quest, "quest.lifecycle.posted"); err != nil {
		t.Fatalf("Failed to create test quest: %v", err)
	}

	// Create idle agent
	agentInstance := domain.GenerateInstance()
	agentID := domain.AgentID(autonomyComp.BoardConfig().AgentEntityID(agentInstance))
	agent := &agentprogression.Agent{
		ID:        agentID,
		Name:      "supervised-agent",
		Status:    domain.AgentIdle,
		Level:     5,
		Tier:      domain.TierApprentice,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	waitForTracker(t, autonomyComp, agentInstance, 3*time.Second)

	// Publish boid suggestion
	suggestions := []boidengine.SuggestedClaim{
		{AgentID: agentID, QuestID: questID, Score: 5.0, Confidence: 0.9, Reason: "test supervised claim"},
	}
	sugData, err := json.Marshal(suggestions)
	if err != nil {
		t.Fatalf("Marshal suggestions: %v", err)
	}
	if err := client.Publish(ctx, "boid.suggestions."+agentInstance, sugData); err != nil {
		t.Fatalf("Publish suggestions: %v", err)
	}

	// Wait for claim to succeed (auto-approved by dmapproval)
	deadline := time.After(10 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for supervised + auto-approved claim")
		case <-time.After(100 * time.Millisecond):
			questEntity, err := gc.GetQuest(ctx, domain.QuestID(questID))
			if err != nil {
				continue
			}
			updatedQuest := domain.QuestFromEntityState(questEntity)
			// Accept QuestInProgress too: executeClaimQuest transitions claimed→in_progress immediately.
			if updatedQuest != nil && (updatedQuest.Status == domain.QuestClaimed || updatedQuest.Status == domain.QuestInProgress) {
				return // success: approved and claimed
			}
		}
	}
}

func TestClaimApprovalGate_Supervised_Denied(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	// Set up dmapproval component (NOT auto-approve)
	approvalCfg := dmapproval.DefaultConfig()
	approvalCfg.Org = "test"
	approvalCfg.Platform = "integration"
	approvalCfg.Board = "denysu"

	deps := component.Dependencies{NATSClient: client}
	approvalComp, err := dmapproval.NewFromConfig(approvalCfg, deps)
	if err != nil {
		t.Fatalf("dmapproval NewFromConfig failed: %v", err)
	}
	if err := approvalComp.Initialize(); err != nil {
		t.Fatalf("dmapproval Initialize failed: %v", err)
	}
	if err := approvalComp.Start(ctx); err != nil {
		t.Fatalf("dmapproval Start failed: %v", err)
	}
	defer approvalComp.Stop(5 * time.Second)

	sessionID := "test.integration.game.denysu.session.deny1"

	// Subscribe to approval requests and auto-deny them
	sessionInstance := domain.ExtractInstance(sessionID)
	approvalSubject := "approval.request." + sessionInstance
	denySub, subErr := client.Subscribe(ctx, approvalSubject, func(_ context.Context, msg *nats.Msg) {
		var envelope struct {
			Request domain.ApprovalRequest `json:"request"`
			ReplyTo string                 `json:"reply_to"`
		}
		if err := json.Unmarshal(msg.Data, &envelope); err != nil {
			return
		}
		// Respond with denial
		resp := domain.ApprovalResponse{
			RequestID:   envelope.Request.ID,
			SessionID:   envelope.Request.SessionID,
			Approved:    false,
			SelectedID:  "deny",
			Reason:      "denied by test",
			RespondedAt: time.Now(),
		}
		respData, err := json.Marshal(resp)
		if err != nil {
			return
		}
		if err := client.Publish(ctx, envelope.ReplyTo, respData); err != nil {
			t.Logf("failed to publish denial response: %v", err)
		}
	})
	if subErr != nil {
		t.Fatalf("Subscribe to approval requests failed: %v", subErr)
	}
	defer denySub.Unsubscribe()

	// Set up autonomy component with supervised mode
	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "denysu"
	config.InitialDelayMs = 100
	config.IdleIntervalMs = 200
	config.DMMode = domain.DMSupervised
	config.SessionID = sessionID
	config.ApprovalTimeoutMs = 5000

	autonomyComp := setupComponentWithDeps(t, client, config, nil, nil, approvalComp)
	defer autonomyComp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, autonomyComp.BoardConfig())

	// Create a posted quest
	questInstance := domain.GenerateInstance()
	questID := domain.QuestID(autonomyComp.BoardConfig().QuestEntityID(questInstance))
	quest := &domain.Quest{
		ID:       questID,
		Title:    "Denied claim test quest",
		Status:   domain.QuestPosted,
		PostedAt: time.Now(),
		MinTier:  domain.TierApprentice,
		BaseXP:   100,
	}
	if err := gc.PutEntityState(ctx, quest, "quest.lifecycle.posted"); err != nil {
		t.Fatalf("Failed to create test quest: %v", err)
	}

	// Create idle agent
	agentInstance := domain.GenerateInstance()
	agentID := domain.AgentID(autonomyComp.BoardConfig().AgentEntityID(agentInstance))
	agent := &agentprogression.Agent{
		ID:        agentID,
		Name:      "denied-agent",
		Status:    domain.AgentIdle,
		Level:     5,
		Tier:      domain.TierApprentice,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	waitForTracker(t, autonomyComp, agentInstance, 3*time.Second)

	// Publish boid suggestion
	suggestions := []boidengine.SuggestedClaim{
		{AgentID: agentID, QuestID: questID, Score: 5.0, Confidence: 0.9, Reason: "test denied claim"},
	}
	sugData, err := json.Marshal(suggestions)
	if err != nil {
		t.Fatalf("Marshal suggestions: %v", err)
	}
	if err := client.Publish(ctx, "boid.suggestions."+agentInstance, sugData); err != nil {
		t.Fatalf("Publish suggestions: %v", err)
	}

	// Wait a few heartbeats — quest should NOT be claimed
	time.Sleep(2 * time.Second)

	questEntity, err := gc.GetQuest(ctx, domain.QuestID(questID))
	if err != nil {
		t.Fatalf("GetQuest failed: %v", err)
	}
	updatedQuest := domain.QuestFromEntityState(questEntity)
	if updatedQuest == nil {
		t.Fatal("Failed to reconstruct quest")
	}
	if updatedQuest.Status != domain.QuestPosted {
		t.Errorf("Quest status = %v, want %v (claim should have been denied)", updatedQuest.Status, domain.QuestPosted)
	}

	// Verify agent is still idle
	agentEntity, err := gc.GetAgent(ctx, domain.AgentID(agentID))
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	updatedAgent := agentprogression.AgentFromEntityState(agentEntity)
	if updatedAgent == nil {
		t.Fatal("Failed to reconstruct agent")
	}
	if updatedAgent.Status != domain.AgentIdle {
		t.Errorf("Agent status = %v, want %v (should stay idle after denied claim)", updatedAgent.Status, domain.AgentIdle)
	}
}

// =============================================================================
// ADR PHASE 6 COMPLETION TESTS
// =============================================================================

func TestStrategicShopping(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "stratshop"
	config.InitialDelayMs = 100
	config.OnQuestIntervalMs = 200
	config.StrategicShopMaxCost = 200

	store := setupStoreComponent(t, client, "stratshop")
	defer store.Stop(5 * time.Second)

	comp := setupComponentWithDeps(t, client, config, store, nil)
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.BoardConfig())

	// Create a claimed quest for the agent to reference
	questInstance := domain.GenerateInstance()
	questID := domain.QuestID(comp.BoardConfig().QuestEntityID(questInstance))
	agentInstance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	claimedAt := time.Now()
	quest := &domain.Quest{
		ID:        questID,
		Title:     "Strategic shopping test quest",
		Status:    domain.QuestClaimed,
		ClaimedBy: &agentID,
		ClaimedAt: &claimedAt,
		PostedAt:  time.Now(),
		MinTier:   domain.TierApprentice,
		BaseXP:    100,
	}
	if err := gc.PutEntityState(ctx, quest, "quest.lifecycle.claimed"); err != nil {
		t.Fatalf("Failed to create test quest: %v", err)
	}

	// Create agent on quest: Level 7 (Journeyman), XP 500, no consumables
	agent := &agentprogression.Agent{
		ID:           agentID,
		Name:         "strategic-shopper",
		Status:       domain.AgentOnQuest,
		Level:        7,
		XP:           500,
		XPToLevel:    400,
		Tier:         domain.TierJourneyman,
		CurrentQuest: &questID,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	waitForTracker(t, comp, agentInstance, 3*time.Second)

	// Wait for strategic purchase (quality_shield costs 150 XP, within StrategicShopMaxCost=200)
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for strategic purchase")
		case <-time.After(100 * time.Millisecond):
			agentEntity, err := gc.GetAgent(ctx, domain.AgentID(agentID))
			if err != nil {
				continue
			}
			updated := agentprogression.AgentFromEntityState(agentEntity)
			if updated == nil {
				continue
			}
			if updated.TotalSpent > 0 {
				t.Logf("agent strategically purchased item, total_spent=%d", updated.TotalSpent)
				return
			}
			// Also check consumable count as alternative signal
			if len(updated.Consumables) > 0 {
				t.Logf("agent has consumables after strategic purchase: %v", updated.Consumables)
				return
			}
		}
	}
}

func TestFullLifecycle(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "fullcycle"
	config.InitialDelayMs = 100
	config.IdleIntervalMs = 200

	comp := setupComponentWithConfig(t, client, config)
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.BoardConfig())

	// Create two posted quests
	quest1Instance := domain.GenerateInstance()
	quest1ID := domain.QuestID(comp.BoardConfig().QuestEntityID(quest1Instance))
	quest1 := &domain.Quest{
		ID:       quest1ID,
		Title:    "Lifecycle quest 1",
		Status:   domain.QuestPosted,
		PostedAt: time.Now(),
		MinTier:  domain.TierApprentice,
		BaseXP:   100,
	}
	if err := gc.PutEntityState(ctx, quest1, "quest.lifecycle.posted"); err != nil {
		t.Fatalf("Failed to create quest1: %v", err)
	}

	quest2Instance := domain.GenerateInstance()
	quest2ID := domain.QuestID(comp.BoardConfig().QuestEntityID(quest2Instance))
	quest2 := &domain.Quest{
		ID:       quest2ID,
		Title:    "Lifecycle quest 2",
		Status:   domain.QuestPosted,
		PostedAt: time.Now(),
		MinTier:  domain.TierApprentice,
		BaseXP:   100,
	}
	if err := gc.PutEntityState(ctx, quest2, "quest.lifecycle.posted"); err != nil {
		t.Fatalf("Failed to create quest2: %v", err)
	}

	// Create idle agent
	agentInstance := domain.GenerateInstance()
	agentID := domain.AgentID(comp.BoardConfig().AgentEntityID(agentInstance))
	agent := &agentprogression.Agent{
		ID:        agentID,
		Name:      "lifecycle-agent",
		Status:    domain.AgentIdle,
		Level:     5,
		Tier:      domain.TierApprentice,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	waitForTracker(t, comp, agentInstance, 3*time.Second)

	// Step 1: Publish boid suggestion for quest1
	suggestions1 := []boidengine.SuggestedClaim{
		{AgentID: agentID, QuestID: quest1ID, Score: 5.0, Confidence: 0.9, Reason: "lifecycle claim 1"},
	}
	data1, err := json.Marshal(suggestions1)
	if err != nil {
		t.Fatalf("Marshal suggestions1: %v", err)
	}
	if err := client.Publish(ctx, "boid.suggestions."+agentInstance, data1); err != nil {
		t.Fatalf("Publish suggestions1: %v", err)
	}

	// Step 2: Poll until quest1 is claimed and agent is on_quest
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for quest1 claim")
		case <-time.After(100 * time.Millisecond):
			questEntity, err := gc.GetQuest(ctx, domain.QuestID(quest1ID))
			if err != nil {
				continue
			}
			updatedQuest := domain.QuestFromEntityState(questEntity)
			// Accept QuestInProgress too: executeClaimQuest transitions claimed→in_progress immediately.
			if updatedQuest != nil && (updatedQuest.Status == domain.QuestClaimed || updatedQuest.Status == domain.QuestInProgress) {
				// Verify agent is on_quest
				agentEntity, err := gc.GetAgent(ctx, domain.AgentID(agentID))
				if err != nil {
					t.Fatalf("GetAgent after quest1 claim failed: %v", err)
				}
				updatedAgent := agentprogression.AgentFromEntityState(agentEntity)
				if updatedAgent == nil {
					t.Fatal("Failed to reconstruct agent after quest1 claim")
				}
				if updatedAgent.Status != domain.AgentOnQuest {
					t.Errorf("Agent status = %v after quest1 claim, want %v", updatedAgent.Status, domain.AgentOnQuest)
				}
				goto quest1Claimed
			}
		}
	}
quest1Claimed:
	t.Log("quest1 claimed successfully")

	// Step 3: Simulate quest completion — write agent back to idle (what agentprogression would do)
	agent.Status = domain.AgentIdle
	agent.CurrentQuest = nil
	agent.UpdatedAt = time.Now()
	if err := gc.EmitEntityUpdate(ctx, agent, "agent.progression.completed"); err != nil {
		t.Fatalf("Failed to emit agent idle transition: %v", err)
	}

	// Brief pause for KV watch to pick up the state change
	time.Sleep(300 * time.Millisecond)

	// Step 4: Publish boid suggestion for quest2
	suggestions2 := []boidengine.SuggestedClaim{
		{AgentID: agentID, QuestID: quest2ID, Score: 4.0, Confidence: 0.8, Reason: "lifecycle claim 2"},
	}
	data2, err := json.Marshal(suggestions2)
	if err != nil {
		t.Fatalf("Marshal suggestions2: %v", err)
	}
	if err := client.Publish(ctx, "boid.suggestions."+agentInstance, data2); err != nil {
		t.Fatalf("Publish suggestions2: %v", err)
	}

	// Step 5: Poll until quest2 is claimed
	deadline = time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for quest2 claim")
		case <-time.After(100 * time.Millisecond):
			questEntity, err := gc.GetQuest(ctx, domain.QuestID(quest2ID))
			if err != nil {
				continue
			}
			updatedQuest := domain.QuestFromEntityState(questEntity)
			// Accept QuestInProgress too: executeClaimQuest transitions claimed→in_progress immediately.
			if updatedQuest != nil && (updatedQuest.Status == domain.QuestClaimed || updatedQuest.Status == domain.QuestInProgress) {
				if updatedQuest.ClaimedBy == nil || domain.AgentID(*updatedQuest.ClaimedBy) != agentID {
					t.Errorf("quest2 ClaimedBy = %v, want %v", updatedQuest.ClaimedBy, agentID)
				}
				t.Log("quest2 claimed successfully — full lifecycle complete")
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

// setupComponentWithDeps creates a component with store, guilds, and optional approval
// registered in a ComponentRegistry so the component resolves them lazily at call time.
func setupComponentWithDeps(t *testing.T, client *natsclient.Client, config Config, store *agentstore.Component, guilds *guildformation.Component, approval ...*dmapproval.Component) *Component {
	t.Helper()

	// Build a registry with the sibling components so lazy resolvers find them.
	reg := component.NewRegistry()
	if store != nil {
		if err := reg.RegisterInstance(agentstore.ComponentName, store); err != nil {
			t.Fatalf("register agentstore: %v", err)
		}
	}
	if guilds != nil {
		if err := reg.RegisterInstance(guildformation.ComponentName, guilds); err != nil {
			t.Fatalf("register guildformation: %v", err)
		}
	}
	if len(approval) > 0 && approval[0] != nil {
		if err := reg.RegisterInstance(dmapproval.ComponentName, approval[0]); err != nil {
			t.Fatalf("register dmapproval: %v", err)
		}
	}

	deps := component.Dependencies{
		NATSClient:        client,
		ComponentRegistry: reg,
	}
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
