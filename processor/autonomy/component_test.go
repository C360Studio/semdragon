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
	"github.com/c360studio/semdragons/processor/boidengine"
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

	// Publish boid suggestion
	suggestion := boidengine.SuggestedClaim{
		AgentID:    agentID,
		QuestID:    "test.integration.game.boidcache.quest.q123",
		Score:      3.14,
		Confidence: 0.8,
		Reason:     "test suggestion",
	}
	data, err := json.Marshal(suggestion)
	if err != nil {
		t.Fatalf("Marshal suggestion: %v", err)
	}
	subject := "boid.suggestions." + instance
	if err := client.Publish(ctx, subject, data); err != nil {
		t.Fatalf("Publish suggestion: %v", err)
	}

	// Wait for suggestion to be cached
	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for suggestion cache")
		case <-time.After(50 * time.Millisecond):
			comp.trackersMu.RLock()
			tracker := comp.trackers[instance]
			hasSuggestion := tracker != nil && tracker.suggestion != nil
			comp.trackersMu.RUnlock()
			if hasSuggestion {
				comp.trackersMu.RLock()
				cached := comp.trackers[instance].suggestion
				comp.trackersMu.RUnlock()
				if string(cached.QuestID) != string(suggestion.QuestID) {
					t.Errorf("cached quest = %v, want %v", cached.QuestID, suggestion.QuestID)
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
// HELPERS
// =============================================================================

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
