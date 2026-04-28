//go:build integration

package questdagexec

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/partycoord"
	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadbuiltins"
)

// =============================================================================
// INTEGRATION TESTS — QuestDAGExec Component
// =============================================================================
// These tests require Docker for NATS via testcontainers.
// Run with: go test -tags=integration -race ./processor/questdagexec/...
//
// DAG state is now stored as quest.dag.* predicates on the parent quest entity
// in the ENTITY_STATES bucket rather than in a dedicated QUEST_DAGS bucket.
// =============================================================================

// setupIntegrationComponent creates a Component with real NATS KV for integration tests.
func setupIntegrationComponent(t *testing.T, client *natsclient.Client, boardName string) *Component {
	t.Helper()

	deps := component.Dependencies{
		NATSClient:      client,
		PayloadRegistry: payloadbuiltins.NewTestRegistry(t),
	}

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = boardName

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}

	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	gc := semdragons.NewGraphClient(client, comp.boardConfig)
	if err := gc.EnsureBucket(context.Background()); err != nil {
		t.Fatalf("EnsureBucket failed: %v", err)
	}

	return comp
}

// waitForCondition polls until check returns true or timeout expires.
func waitForCondition(t *testing.T, timeout time.Duration, check func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out: %s", msg)
}

// TestComponent_Lifecycle verifies basic start/stop with real NATS.
func TestComponent_Lifecycle(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client

	comp := setupIntegrationComponent(t, client, "lifecycle")

	if err := comp.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer comp.Stop(5 * time.Second)

	health := comp.Health()
	if !health.Healthy {
		t.Error("Component should be healthy after start")
	}
	if health.Status != "running" {
		t.Errorf("Health.Status = %q, want %q", health.Status, "running")
	}
}

// TestComponent_DAGWatchAndTransition verifies the full DAG state machine with
// real NATS KV: write a parent quest with quest.dag.* predicates → component
// indexes it and assigns ready nodes → write sub-quest entity transitions →
// component drives DAG to completion and triggers rollup.
//
// This test exercises the single-goroutine event loop with two producer
// goroutines (quest KV watcher, AGENT stream consumer) feeding real async KV
// events, verifying the full DAG lifecycle end-to-end with real NATS.
func TestComponent_DAGWatchAndTransition(t *testing.T) {
	testClient := natsclient.NewTestClient(t,
		natsclient.WithKV(), natsclient.WithFileStorage(),
		natsclient.WithKVBuckets(graph.BucketEntityStates),
		natsclient.WithStreams(natsclient.TestStreamConfig{
			Name:     "AGENT",
			Subjects: []string{"agent.task.>", "agent.complete.>", "agent.failed.>", "tool.execute.>", "tool.result.>"},
		}),
	)
	client := testClient.Client
	ctx := context.Background()

	// Set up component with mock sibling refs.
	comp := setupIntegrationComponent(t, client, "dagwatch")

	boardConfig := comp.boardConfig
	gc := semdragons.NewGraphClient(client, boardConfig)

	// Mock questboard that tracks rollup calls.
	qb := &mockQuestBoardRef{}
	// Mock partycoord with a party containing a lead + 2 members.
	partyEntityID := domain.PartyID(boardConfig.PartyEntityID("party1"))
	pc := &mockPartyCoordRef{
		parties: map[domain.PartyID]*partycoord.Party{
			partyEntityID: {
				Lead: domain.AgentID(boardConfig.AgentEntityID("lead1")),
				Members: []partycoord.PartyMember{
					{
						AgentID: domain.AgentID(boardConfig.AgentEntityID("lead1")),
						Role:    domain.RoleLead,
						Skills:  []domain.SkillTag{"code_generation"},
					},
					{
						AgentID: domain.AgentID(boardConfig.AgentEntityID("member1")),
						Role:    domain.RoleExecutor,
						Skills:  []domain.SkillTag{"code_generation"},
					},
					{
						AgentID: domain.AgentID(boardConfig.AgentEntityID("member2")),
						Role:    domain.RoleExecutor,
						Skills:  []domain.SkillTag{"code_generation"},
					},
				},
			},
		},
	}
	comp.deps.ComponentRegistry = &mockComponentRegistry{qb: qb, pc: pc}

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer comp.Stop(5 * time.Second)

	// --- Seed sub-quest entities (posted status) ---
	subQuest1ID := boardConfig.QuestEntityID("sub1")
	subQuest2ID := boardConfig.QuestEntityID("sub2")
	parentQuestID := boardConfig.QuestEntityID("parent1")

	for _, sqID := range []string{subQuest1ID, subQuest2ID} {
		sq := &domain.Quest{
			ID:          domain.QuestID(sqID),
			Title:       "Sub-quest " + sqID,
			Status:      domain.QuestPosted,
			ParentQuest: &[]domain.QuestID{domain.QuestID(parentQuestID)}[0],
		}
		if err := gc.EmitEntity(ctx, sq, "quest.lifecycle.posted"); err != nil {
			t.Fatalf("emit sub-quest %s: %v", sqID, err)
		}
	}

	// --- Write parent quest with DAG predicates ---
	// This mirrors what questbridge.handleDAGDecomposition writes.
	dag := QuestDAG{
		Nodes: []QuestNode{
			{ID: "node-1", Objective: "First task", Skills: []string{"code_generation"}},
			{ID: "node-2", Objective: "Second task", Skills: []string{"code_generation"}},
		},
	}
	nodeStates := map[string]string{
		"node-1": NodeReady,
		"node-2": NodeReady,
	}
	nodeQuestIDs := map[string]string{
		"node-1": subQuest1ID,
		"node-2": subQuest2ID,
	}
	nodeRetries := map[string]int{"node-1": 2, "node-2": 2}

	parentQuest := &domain.Quest{
		ID:               domain.QuestID(parentQuestID),
		Title:            "Parent Quest",
		Status:           domain.QuestInProgress,
		DAGExecutionID:   "exec-integration-1",
		DAGDefinition:    dag,
		DAGNodeQuestIDs:  nodeQuestIDs,
		DAGNodeStates:    nodeStates,
		DAGNodeAssignees: map[string]string{},
		DAGNodeRetries:   nodeRetries,
		PartyID:          &partyEntityID,
	}
	if err := gc.EmitEntity(ctx, parentQuest, "quest.dag.decomposed"); err != nil {
		t.Fatalf("emit parent quest with DAG state: %v", err)
	}

	// --- Wait for component to index DAG and assign nodes ---
	// Verify by checking that ClaimAndStart was called for both sub-quests.
	waitForCondition(t, 10*time.Second, func() bool {
		return qb.ClaimCallCount() >= 2
	}, "both DAG nodes to be assigned (ClaimAndStartForParty called twice)")

	// --- Simulate sub-quest transitions: posted → claimed → in_progress → in_review → completed ---
	// The component watches quest entity KV for these transitions.
	for _, sqID := range []string{subQuest1ID, subQuest2ID} {
		for _, status := range []domain.QuestStatus{
			domain.QuestClaimed,
			domain.QuestInProgress,
			domain.QuestInReview,
			domain.QuestCompleted,
		} {
			sq := &domain.Quest{
				ID:     domain.QuestID(sqID),
				Title:  "Sub-quest " + sqID,
				Status: status,
				Output: fmt.Sprintf("output for %s", sqID),
			}
			if status == domain.QuestCompleted {
				now := time.Now()
				sq.CompletedAt = &now
			}
			if emitErr := gc.EmitEntityUpdate(ctx, sq, "quest.lifecycle."+string(status)); emitErr != nil {
				t.Fatalf("emit sub-quest %s status %s: %v", sqID, status, emitErr)
			}
			// Small delay to let async KV events propagate.
			time.Sleep(100 * time.Millisecond)
		}
	}

	// --- Wait for synthesis dispatch ---
	// Both nodes are complete, so dispatchLeadSynthesis publishes a TaskMessage
	// to the AGENT stream. We need to simulate the agentic-loop completing the
	// synthesis by publishing a LoopCompletedEvent with "synthesis-" prefix.
	waitForCondition(t, 10*time.Second, func() bool {
		return comp.nodesCompleted.Load() == 2
	}, "both nodes to be marked completed")

	// Small delay for synthesis dispatch to hit the stream.
	time.Sleep(200 * time.Millisecond)

	// Publish a synthetic LoopCompletedEvent to agent.complete.*.
	parentKey := strings.ReplaceAll(parentQuestID, ".", "-")
	synthesisLoopID := fmt.Sprintf("synthesis-%s-test", parentKey)
	synthResult := "Combined output from all sub-quests"
	completedEvt := &agentic.LoopCompletedEvent{
		LoopID:  synthesisLoopID,
		TaskID:  parentQuestID,
		Result:  synthResult,
		Outcome: agentic.OutcomeSuccess,
	}
	baseMsg := message.NewBaseMessage(completedEvt.Schema(), completedEvt, "test")
	data, marshalErr := json.Marshal(baseMsg)
	if marshalErr != nil {
		t.Fatalf("marshal synthesis LoopCompletedEvent: %v", marshalErr)
	}
	if pubErr := client.PublishToStream(ctx, fmt.Sprintf("agent.complete.%s", parentKey), data); pubErr != nil {
		t.Fatalf("publish synthesis completion: %v", pubErr)
	}

	// --- Wait for rollup ---
	waitForCondition(t, 15*time.Second, func() bool {
		return qb.SubmitCallCount() > 0
	}, "rollup submit call to questboard")

	// Verify rollup was called with the parent quest ID and synthesized result.
	sc := qb.GetSubmitCall(0)
	if sc.questID != domain.QuestID(parentQuestID) {
		t.Errorf("rollup quest ID = %q, want %q", sc.questID, parentQuestID)
	}
	if sc.result != synthResult {
		t.Errorf("rollup result = %q, want %q", sc.result, synthResult)
	}

	// Verify party was disbanded.
	if pc.DisbandCallCount() != 1 {
		t.Errorf("expected 1 disband call, got %d", pc.DisbandCallCount())
	}

	// Verify metrics.
	if comp.nodesCompleted.Load() != 2 {
		t.Errorf("nodesCompleted = %d, want 2", comp.nodesCompleted.Load())
	}
	if comp.rollupsTriggered.Load() != 1 {
		t.Errorf("rollupsTriggered = %d, want 1", comp.rollupsTriggered.Load())
	}
}

// TestComponent_HealthAndPorts verifies component metadata and port definitions.
func TestComponent_HealthAndPorts(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))

	comp := setupIntegrationComponent(t, testClient.Client, "ports")

	meta := comp.Meta()
	if meta.Name != ComponentName {
		t.Errorf("Meta.Name = %q, want %q", meta.Name, ComponentName)
	}

	inputs := comp.InputPorts()
	if len(inputs) != 1 {
		t.Errorf("InputPorts count = %d, want 1 (quest-entities only)", len(inputs))
	}

	outputs := comp.OutputPorts()
	if len(outputs) != 1 {
		t.Errorf("OutputPorts count = %d, want 1", len(outputs))
	}

	schema := comp.ConfigSchema()
	if _, ok := schema.Properties["org"]; !ok {
		t.Error("ConfigSchema missing 'org' property")
	}
	if _, ok := schema.Properties["quest_dags_bucket"]; ok {
		t.Error("ConfigSchema should not contain 'quest_dags_bucket' (bucket eliminated)")
	}
}

// mockQuestBoardRef and mockPartyCoordRef are defined in handler_test.go.
// The integration tests in this file reuse them since they're in the same package.

// Verify mocks satisfy interfaces at compile time.
var (
	_ QuestBoardRef = (*mockQuestBoardRef)(nil)
	_ PartyCoordRef = (*mockPartyCoordRef)(nil)
)
