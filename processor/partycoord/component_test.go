//go:build integration

package partycoord

import (
	"context"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// INTEGRATION TESTS - PartyCoord Component
// =============================================================================
// These tests require Docker for NATS via testcontainers.
// Run with: go test -tags=integration ./processor/partycoord/...
// =============================================================================

func TestComponent_Lifecycle(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	deps := component.Dependencies{
		NATSClient: client,
	}

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "lifecycle"

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}

	// Test Initialize
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Ensure board-specific KV bucket exists (mirrors main.go startup)
	gc := semdragons.NewGraphClient(client, comp.boardConfig)
	if err := gc.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket failed: %v", err)
	}

	// Test Meta
	meta := comp.Meta()
	if meta.Name != ComponentName {
		t.Errorf("Meta.Name = %q, want %q", meta.Name, ComponentName)
	}
	if meta.Type != "processor" {
		t.Errorf("Meta.Type = %q, want %q", meta.Type, "processor")
	}
	if meta.Version == "" {
		t.Error("Meta.Version should not be empty")
	}

	// Test Start
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Verify running health
	health := comp.Health()
	if !health.Healthy {
		t.Error("Component should be healthy after start")
	}
	if health.Status != "running" {
		t.Errorf("Health.Status = %q, want %q", health.Status, "running")
	}

	// Test double-start is rejected
	err = comp.Start(ctx)
	if err == nil {
		t.Error("Second Start should return an error")
	}

	// Test Stop
	if err := comp.Stop(5 * time.Second); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Verify stopped
	health = comp.Health()
	if health.Healthy {
		t.Error("Component should not be healthy after stop")
	}
	if health.Status != "stopped" {
		t.Errorf("Health.Status = %q, want %q", health.Status, "stopped")
	}

	// Verify idempotent stop
	if err := comp.Stop(5 * time.Second); err != nil {
		t.Errorf("Second Stop should not error: %v", err)
	}
}

func TestComponent_InputOutputPorts(t *testing.T) {
	comp := &Component{}

	inputs := comp.InputPorts()
	if len(inputs) == 0 {
		t.Error("Should have at least one input port defined")
	}

	// Verify quest-state port is present and required
	hasQuestState := false
	for _, port := range inputs {
		if port.Name == "quest-state" {
			hasQuestState = true
			if !port.Required {
				t.Error("quest-state port should be required")
			}
			if port.Direction != component.DirectionInput {
				t.Errorf("quest-state port direction = %v, want input", port.Direction)
			}
		}
	}
	if !hasQuestState {
		t.Error("Missing quest-state input port")
	}

	outputs := comp.OutputPorts()
	if len(outputs) < 2 {
		t.Errorf("Should have at least 2 output ports, got %d", len(outputs))
	}

	expectedOutputs := map[string]bool{
		"party-formed":    false,
		"party-disbanded": false,
		"party-state":     false,
	}
	for _, port := range outputs {
		if _, expected := expectedOutputs[port.Name]; expected {
			expectedOutputs[port.Name] = true
			if !port.Required {
				t.Errorf("Output port %q should be required", port.Name)
			}
			if port.Direction != component.DirectionOutput {
				t.Errorf("Port %q direction = %v, want output", port.Name, port.Direction)
			}
		}
	}
	for name, found := range expectedOutputs {
		if !found {
			t.Errorf("Missing expected output port %q", name)
		}
	}
}

func TestComponent_ConfigSchema(t *testing.T) {
	comp := &Component{}
	schema := comp.ConfigSchema()

	// Check required fields
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
			t.Errorf("Field %q should be in Required list", field)
		}
	}

	// Check expected properties are declared
	expectedProps := []string{"org", "platform", "board", "default_max_party_size", "formation_timeout", "auto_form_parties"}
	for _, prop := range expectedProps {
		if _, exists := schema.Properties[prop]; !exists {
			t.Errorf("Missing property %q in ConfigSchema", prop)
		}
	}
}

func TestComponent_FormParty(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupPartyComponent(t, client, "formparty")
	defer comp.Stop(5 * time.Second)

	leadID := makePartyAgentID(t, comp.boardConfig, "party-lead")
	questID := makePartyQuestID(t, comp.boardConfig, "party-quest")

	party, err := comp.FormParty(ctx, questID, leadID)
	if err != nil {
		t.Fatalf("FormParty failed: %v", err)
	}

	// Verify returned party
	if party.ID == "" {
		t.Error("Party ID should be set")
	}
	if party.QuestID != questID {
		t.Errorf("QuestID = %v, want %v", party.QuestID, questID)
	}
	if party.Lead != leadID {
		t.Errorf("Lead = %v, want %v", party.Lead, leadID)
	}
	if party.Status != domain.PartyForming {
		t.Errorf("Status = %v, want %v", party.Status, domain.PartyForming)
	}
	if party.FormedAt.IsZero() {
		t.Error("FormedAt should be set")
	}

	// Lead should be the first member with Lead role
	if len(party.Members) != 1 {
		t.Fatalf("Members count = %d, want 1 (lead)", len(party.Members))
	}
	if party.Members[0].AgentID != leadID {
		t.Errorf("Lead should be first member")
	}
	if party.Members[0].Role != domain.RoleLead {
		t.Errorf("Lead member role = %v, want %v", party.Members[0].Role, domain.RoleLead)
	}

	// Party should be retrievable from active parties
	retrieved, ok := comp.GetParty(party.ID)
	if !ok {
		t.Fatal("GetParty: party not found after formation")
	}
	if retrieved.QuestID != questID {
		t.Errorf("Retrieved QuestID = %v, want %v", retrieved.QuestID, questID)
	}

	// Metrics
	flow := comp.DataFlow()
	if flow.LastActivity.IsZero() {
		t.Error("LastActivity should be set after FormParty")
	}
}

func TestComponent_FormPartyWhenNotRunning(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	deps := component.Dependencies{NATSClient: client}
	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "notrunning"

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	// Deliberately do NOT start the component

	leadID := domain.AgentID("test.integration.game.notrunning.agent.lead1")
	questID := domain.QuestID("test.integration.game.notrunning.quest.q1")
	_, err = comp.FormParty(ctx, questID, leadID)
	if err == nil {
		t.Error("FormParty should fail when component is not running")
	}
}

func TestComponent_JoinParty(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupPartyComponent(t, client, "joinparty")
	defer comp.Stop(5 * time.Second)

	leadID := makePartyAgentID(t, comp.boardConfig, "join-lead")
	member1ID := makePartyAgentID(t, comp.boardConfig, "join-member1")
	member2ID := makePartyAgentID(t, comp.boardConfig, "join-member2")
	questID := makePartyQuestID(t, comp.boardConfig, "join-quest")

	party, err := comp.FormParty(ctx, questID, leadID)
	if err != nil {
		t.Fatalf("FormParty failed: %v", err)
	}

	if err := comp.JoinParty(ctx, party.ID, member1ID, domain.RoleExecutor); err != nil {
		t.Fatalf("JoinParty member1 failed: %v", err)
	}
	if err := comp.JoinParty(ctx, party.ID, member2ID, domain.RoleExecutor); err != nil {
		t.Fatalf("JoinParty member2 failed: %v", err)
	}

	// Verify membership
	updated, ok := comp.GetParty(party.ID)
	if !ok {
		t.Fatal("GetParty: party not found after joins")
	}
	if len(updated.Members) != 3 {
		t.Fatalf("Members count = %d, want 3 (lead + 2 members)", len(updated.Members))
	}

	// Check that new members have correct role
	memberCount := 0
	for _, m := range updated.Members {
		if m.AgentID == member1ID || m.AgentID == member2ID {
			if m.Role != domain.RoleExecutor {
				t.Errorf("Member role = %v, want %v", m.Role, domain.RoleExecutor)
			}
			if m.JoinedAt.IsZero() {
				t.Error("JoinedAt should be set")
			}
			memberCount++
		}
	}
	if memberCount != 2 {
		t.Errorf("Found %d test members, want 2", memberCount)
	}
}

func TestComponent_JoinPartyNotFound(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupPartyComponent(t, client, "joinnotfound")
	defer comp.Stop(5 * time.Second)

	ghost := domain.PartyID("test.integration.game.joinnotfound.party.ghost")
	agentID := makePartyAgentID(t, comp.boardConfig, "lonely")

	err := comp.JoinParty(ctx, ghost, agentID, domain.RoleExecutor)
	if err == nil {
		t.Error("JoinParty should fail for non-existent party")
	}
}

func TestComponent_DisbandParty(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupPartyComponent(t, client, "disbandparty")
	defer comp.Stop(5 * time.Second)

	leadID := makePartyAgentID(t, comp.boardConfig, "disband-lead")
	memberID := makePartyAgentID(t, comp.boardConfig, "disband-member")
	questID := makePartyQuestID(t, comp.boardConfig, "disband-quest")

	party, err := comp.FormParty(ctx, questID, leadID)
	if err != nil {
		t.Fatalf("FormParty failed: %v", err)
	}
	if err := comp.JoinParty(ctx, party.ID, memberID, domain.RoleExecutor); err != nil {
		t.Fatalf("JoinParty failed: %v", err)
	}

	// Verify party is in active list
	active := comp.ListActiveParties()
	if len(active) != 1 {
		t.Errorf("Active parties before disband = %d, want 1", len(active))
	}

	if err := comp.DisbandParty(ctx, party.ID, "quest completed"); err != nil {
		t.Fatalf("DisbandParty failed: %v", err)
	}

	// Party should no longer be retrievable as active
	_, ok := comp.GetParty(party.ID)
	if ok {
		t.Error("GetParty should return false after disbanding")
	}

	// Active parties list should be empty
	active = comp.ListActiveParties()
	if len(active) != 0 {
		t.Errorf("Active parties after disband = %d, want 0", len(active))
	}
}

func TestComponent_DisbandPartyNotFound(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupPartyComponent(t, client, "disbandnotfound")
	defer comp.Stop(5 * time.Second)

	ghost := domain.PartyID("test.integration.game.disbandnotfound.party.ghost")
	err := comp.DisbandParty(ctx, ghost, "never existed")
	if err == nil {
		t.Error("DisbandParty should fail for non-existent party")
	}
}

func TestComponent_ListActiveParties(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupPartyComponent(t, client, "listparties")
	defer comp.Stop(5 * time.Second)

	// No parties initially
	parties := comp.ListActiveParties()
	if len(parties) != 0 {
		t.Errorf("Initial party count = %d, want 0", len(parties))
	}

	l1 := makePartyAgentID(t, comp.boardConfig, "list-lead1")
	l2 := makePartyAgentID(t, comp.boardConfig, "list-lead2")
	l3 := makePartyAgentID(t, comp.boardConfig, "list-lead3")

	q1 := makePartyQuestID(t, comp.boardConfig, "list-q1")
	q2 := makePartyQuestID(t, comp.boardConfig, "list-q2")
	q3 := makePartyQuestID(t, comp.boardConfig, "list-q3")

	// Form three parties
	var formed []*Party
	for i, params := range []struct {
		lead    domain.AgentID
		questID domain.QuestID
	}{
		{l1, q1}, {l2, q2}, {l3, q3},
	} {
		p, err := comp.FormParty(ctx, params.questID, params.lead)
		if err != nil {
			t.Fatalf("FormParty %d failed: %v", i, err)
		}
		formed = append(formed, p)
	}

	parties = comp.ListActiveParties()
	if len(parties) != 3 {
		t.Errorf("Active party count = %d, want 3", len(parties))
	}

	// Disband one
	if err := comp.DisbandParty(ctx, formed[1].ID, "done"); err != nil {
		t.Fatalf("DisbandParty failed: %v", err)
	}

	parties = comp.ListActiveParties()
	if len(parties) != 2 {
		t.Errorf("Active party count after one disband = %d, want 2", len(parties))
	}
}

func TestComponent_DecomposeQuest(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupPartyComponent(t, client, "decompose")
	defer comp.Stop(5 * time.Second)

	leadID := makePartyAgentID(t, comp.boardConfig, "decomp-lead")
	questID := makePartyQuestID(t, comp.boardConfig, "decomp-quest")
	sq1 := makePartyQuestID(t, comp.boardConfig, "sub1")
	sq2 := makePartyQuestID(t, comp.boardConfig, "sub2")

	party, err := comp.FormParty(ctx, questID, leadID)
	if err != nil {
		t.Fatalf("FormParty failed: %v", err)
	}

	subQuests := []domain.QuestID{sq1, sq2}
	if err := comp.DecomposeQuest(ctx, party.ID, subQuests, "parallel approach"); err != nil {
		t.Fatalf("DecomposeQuest failed: %v", err)
	}

	// Strategy should be updated on the in-memory party
	updated, ok := comp.GetParty(party.ID)
	if !ok {
		t.Fatal("GetParty: party not found after decomposition")
	}
	if updated.Strategy != "parallel approach" {
		t.Errorf("Strategy = %q, want %q", updated.Strategy, "parallel approach")
	}
}

func TestComponent_DecomposeQuestPartyNotFound(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupPartyComponent(t, client, "decompnotfound")
	defer comp.Stop(5 * time.Second)

	ghost := domain.PartyID("test.integration.game.decompnotfound.party.ghost")
	err := comp.DecomposeQuest(ctx, ghost, []domain.QuestID{"sq1"}, "strategy")
	if err == nil {
		t.Error("DecomposeQuest should fail for non-existent party")
	}
}

func TestComponent_AssignTask(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupPartyComponent(t, client, "assigntask")
	defer comp.Stop(5 * time.Second)

	leadID := makePartyAgentID(t, comp.boardConfig, "assign-lead")
	member1ID := makePartyAgentID(t, comp.boardConfig, "assign-m1")
	member2ID := makePartyAgentID(t, comp.boardConfig, "assign-m2")
	questID := makePartyQuestID(t, comp.boardConfig, "assign-quest")
	sq1 := makePartyQuestID(t, comp.boardConfig, "assign-sub1")
	sq2 := makePartyQuestID(t, comp.boardConfig, "assign-sub2")

	party, err := comp.FormParty(ctx, questID, leadID)
	if err != nil {
		t.Fatalf("FormParty failed: %v", err)
	}

	if err := comp.JoinParty(ctx, party.ID, member1ID, domain.RoleExecutor); err != nil {
		t.Fatalf("JoinParty member1 failed: %v", err)
	}
	if err := comp.JoinParty(ctx, party.ID, member2ID, domain.RoleExecutor); err != nil {
		t.Fatalf("JoinParty member2 failed: %v", err)
	}

	if err := comp.AssignTask(ctx, party.ID, sq1, member1ID, "best analyst"); err != nil {
		t.Fatalf("AssignTask sq1 failed: %v", err)
	}
	if err := comp.AssignTask(ctx, party.ID, sq2, member2ID, "experienced coder"); err != nil {
		t.Fatalf("AssignTask sq2 failed: %v", err)
	}

	// Verify sub-quest assignments recorded in party state
	updated, ok := comp.GetParty(party.ID)
	if !ok {
		t.Fatal("GetParty: party not found after task assignment")
	}

	if len(updated.SubQuestMap) != 2 {
		t.Errorf("SubQuestMap size = %d, want 2", len(updated.SubQuestMap))
	}
	if updated.SubQuestMap[sq1] != member1ID {
		t.Errorf("SubQuestMap[sq1] = %v, want %v", updated.SubQuestMap[sq1], member1ID)
	}
	if updated.SubQuestMap[sq2] != member2ID {
		t.Errorf("SubQuestMap[sq2] = %v, want %v", updated.SubQuestMap[sq2], member2ID)
	}
}

func TestComponent_AssignTaskPartyNotFound(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupPartyComponent(t, client, "assignnotfound")
	defer comp.Stop(5 * time.Second)

	ghost := domain.PartyID("test.integration.game.assignnotfound.party.ghost")
	sq := domain.QuestID("test.integration.game.assignnotfound.quest.sub1")
	agent := makePartyAgentID(t, comp.boardConfig, "some-agent")

	err := comp.AssignTask(ctx, ghost, sq, agent, "rationale")
	if err == nil {
		t.Error("AssignTask should fail for non-existent party")
	}
}

func TestComponent_SubmitResult(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupPartyComponent(t, client, "submitresult")
	defer comp.Stop(5 * time.Second)

	leadID := makePartyAgentID(t, comp.boardConfig, "submit-lead")
	memberID := makePartyAgentID(t, comp.boardConfig, "submit-member")
	questID := makePartyQuestID(t, comp.boardConfig, "submit-quest")
	sq := makePartyQuestID(t, comp.boardConfig, "submit-sub")

	party, err := comp.FormParty(ctx, questID, leadID)
	if err != nil {
		t.Fatalf("FormParty failed: %v", err)
	}

	if err := comp.JoinParty(ctx, party.ID, memberID, domain.RoleExecutor); err != nil {
		t.Fatalf("JoinParty failed: %v", err)
	}

	result := map[string]any{"output": "analysis complete", "confidence": 0.95}
	if err := comp.SubmitResult(ctx, party.ID, memberID, sq, result); err != nil {
		t.Fatalf("SubmitResult failed: %v", err)
	}

	// Verify result was recorded in party state
	updated, ok := comp.GetParty(party.ID)
	if !ok {
		t.Fatal("GetParty: party not found after result submission")
	}
	if _, exists := updated.SubResults[sq]; !exists {
		t.Error("SubResults should contain the submitted result")
	}
}

func TestComponent_SubmitResultPartyNotFound(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupPartyComponent(t, client, "submitnotfound")
	defer comp.Stop(5 * time.Second)

	ghost := domain.PartyID("test.integration.game.submitnotfound.party.ghost")
	member := makePartyAgentID(t, comp.boardConfig, "ghost-member")
	sq := domain.QuestID("test.integration.game.submitnotfound.quest.sub1")

	err := comp.SubmitResult(ctx, ghost, member, sq, "result")
	if err == nil {
		t.Error("SubmitResult should fail for non-existent party")
	}
}

func TestComponent_RollupLifecycle(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupPartyComponent(t, client, "rollup")
	defer comp.Stop(5 * time.Second)

	leadID := makePartyAgentID(t, comp.boardConfig, "rollup-lead")
	memberID := makePartyAgentID(t, comp.boardConfig, "rollup-member")
	questID := makePartyQuestID(t, comp.boardConfig, "rollup-quest")
	sq := makePartyQuestID(t, comp.boardConfig, "rollup-sub")

	party, err := comp.FormParty(ctx, questID, leadID)
	if err != nil {
		t.Fatalf("FormParty failed: %v", err)
	}

	if err := comp.JoinParty(ctx, party.ID, memberID, domain.RoleExecutor); err != nil {
		t.Fatalf("JoinParty failed: %v", err)
	}

	// Submit a sub-result
	if err := comp.SubmitResult(ctx, party.ID, memberID, sq, "sub-result-data"); err != nil {
		t.Fatalf("SubmitResult failed: %v", err)
	}

	// Start rollup
	if err := comp.StartRollup(ctx, party.ID); err != nil {
		t.Fatalf("StartRollup failed: %v", err)
	}

	// Complete rollup with a combined result
	rollupResult := map[string]any{"combined": "final synthesis", "quality": 0.9}
	if err := comp.CompleteRollup(ctx, party.ID, rollupResult); err != nil {
		t.Fatalf("CompleteRollup failed: %v", err)
	}

	// Verify rollup result was stored
	updated, ok := comp.GetParty(party.ID)
	if !ok {
		t.Fatal("GetParty: party not found after rollup")
	}
	if updated.RollupResult == nil {
		t.Error("RollupResult should be set after CompleteRollup")
	}
}

func TestComponent_StartRollupPartyNotFound(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupPartyComponent(t, client, "startrollupnotfound")
	defer comp.Stop(5 * time.Second)

	ghost := domain.PartyID("test.integration.game.startrollupnotfound.party.ghost")
	err := comp.StartRollup(ctx, ghost)
	if err == nil {
		t.Error("StartRollup should fail for non-existent party")
	}
}

func TestComponent_CompleteRollupPartyNotFound(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupPartyComponent(t, client, "completerollupnotfound")
	defer comp.Stop(5 * time.Second)

	ghost := domain.PartyID("test.integration.game.completerollupnotfound.party.ghost")
	err := comp.CompleteRollup(ctx, ghost, "result")
	if err == nil {
		t.Error("CompleteRollup should fail for non-existent party")
	}
}

func TestComponent_FullPartyWorkflow(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupPartyComponent(t, client, "fullworkflow")
	defer comp.Stop(5 * time.Second)

	leadID := makePartyAgentID(t, comp.boardConfig, "wf-lead")
	m1ID := makePartyAgentID(t, comp.boardConfig, "wf-m1")
	m2ID := makePartyAgentID(t, comp.boardConfig, "wf-m2")
	questID := makePartyQuestID(t, comp.boardConfig, "wf-quest")
	sq1 := makePartyQuestID(t, comp.boardConfig, "wf-sub1")
	sq2 := makePartyQuestID(t, comp.boardConfig, "wf-sub2")

	// 1. Form party
	party, err := comp.FormParty(ctx, questID, leadID)
	if err != nil {
		t.Fatalf("FormParty failed: %v", err)
	}
	t.Logf("Formed party: %s", party.ID)

	// 2. Members join
	if err := comp.JoinParty(ctx, party.ID, m1ID, domain.RoleExecutor); err != nil {
		t.Fatalf("JoinParty m1 failed: %v", err)
	}
	if err := comp.JoinParty(ctx, party.ID, m2ID, domain.RoleExecutor); err != nil {
		t.Fatalf("JoinParty m2 failed: %v", err)
	}
	t.Log("Members joined")

	// 3. Lead decomposes quest
	if err := comp.DecomposeQuest(ctx, party.ID, []domain.QuestID{sq1, sq2}, "divide and conquer"); err != nil {
		t.Fatalf("DecomposeQuest failed: %v", err)
	}
	t.Log("Quest decomposed")

	// 4. Assign sub-quests
	if err := comp.AssignTask(ctx, party.ID, sq1, m1ID, "best for analysis"); err != nil {
		t.Fatalf("AssignTask sq1 failed: %v", err)
	}
	if err := comp.AssignTask(ctx, party.ID, sq2, m2ID, "best for implementation"); err != nil {
		t.Fatalf("AssignTask sq2 failed: %v", err)
	}
	t.Log("Tasks assigned")

	// 5. Members submit results
	if err := comp.SubmitResult(ctx, party.ID, m1ID, sq1, "analysis done"); err != nil {
		t.Fatalf("SubmitResult m1 failed: %v", err)
	}
	if err := comp.SubmitResult(ctx, party.ID, m2ID, sq2, "implementation done"); err != nil {
		t.Fatalf("SubmitResult m2 failed: %v", err)
	}
	t.Log("Results submitted")

	// 6. Lead rolls up
	if err := comp.StartRollup(ctx, party.ID); err != nil {
		t.Fatalf("StartRollup failed: %v", err)
	}
	if err := comp.CompleteRollup(ctx, party.ID, "combined deliverable"); err != nil {
		t.Fatalf("CompleteRollup failed: %v", err)
	}
	t.Log("Rollup completed")

	// 7. Verify final party state
	final, ok := comp.GetParty(party.ID)
	if !ok {
		t.Fatal("GetParty: party not found after workflow")
	}
	if len(final.Members) != 3 {
		t.Errorf("Final member count = %d, want 3", len(final.Members))
	}
	if len(final.SubQuestMap) != 2 {
		t.Errorf("Final SubQuestMap size = %d, want 2", len(final.SubQuestMap))
	}
	if len(final.SubResults) != 2 {
		t.Errorf("Final SubResults size = %d, want 2", len(final.SubResults))
	}
	if final.RollupResult == nil {
		t.Error("RollupResult should be set")
	}
	if final.Strategy != "divide and conquer" {
		t.Errorf("Strategy = %q, want %q", final.Strategy, "divide and conquer")
	}
	t.Log("Full workflow verified")

	// 8. Disband party
	if err := comp.DisbandParty(ctx, party.ID, "quest complete"); err != nil {
		t.Fatalf("DisbandParty failed: %v", err)
	}

	_, ok = comp.GetParty(party.ID)
	if ok {
		t.Error("Party should not be active after disband")
	}
}

func TestComponent_AutoFormPartyOnQuestClaimed(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	// Use a config with auto-form enabled (default)
	comp := setupPartyComponent(t, client, "autoform")
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.boardConfig)

	// Create an agent
	agentInstance := "autoform-agent-" + semdragons.GenerateInstance()
	agentID := domain.AgentID(comp.boardConfig.AgentEntityID(agentInstance))
	agent := &semdragons.Agent{
		ID:     agentID,
		Name:   "autoform-agent",
		Level:  5,
		Tier:   semdragons.TierApprentice,
		Status: semdragons.AgentIdle,
	}
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	// Create a party-required quest in "claimed" state, simulating what questboard would do
	questInstance := "autoform-quest-" + semdragons.GenerateInstance()
	questID := domain.QuestID(comp.boardConfig.QuestEntityID(questInstance))
	now := time.Now()
	claimedBy := agentID
	quest := &semdragons.Quest{
		ID:            questID,
		Title:         "Multi-Agent Task",
		Status:        semdragons.QuestClaimed,
		Difficulty:    semdragons.DifficultyModerate,
		PartyRequired: true,
		ClaimedBy:     &claimedBy,
		ClaimedAt:     &now,
		PostedAt:      now,
	}
	if err := gc.PutEntityState(ctx, quest, "quest.lifecycle.claimed"); err != nil {
		t.Fatalf("Failed to create test quest: %v", err)
	}

	// Poll for auto-formed party to appear in active parties
	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			t.Fatal("Timed out waiting for auto-formed party after quest claimed")
		case <-ticker.C:
			parties := comp.ListActiveParties()
			for _, p := range parties {
				if p.QuestID == questID {
					// Verify party was formed with correct lead
					if p.Lead != agentID {
						t.Errorf("Auto-formed party lead = %v, want %v", p.Lead, agentID)
					}
					if p.Status != domain.PartyForming {
						t.Errorf("Auto-formed party status = %v, want %v", p.Status, domain.PartyForming)
					}
					t.Logf("Auto-formed party %s for quest %s", p.ID, questID)
					return
				}
			}
		}
	}
}

func TestComponent_AutoFormSkipsNonPartyQuests(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupPartyComponent(t, client, "skipnonparty")
	defer comp.Stop(5 * time.Second)

	gc := semdragons.NewGraphClient(client, comp.boardConfig)

	agentInstance := "skip-agent-" + semdragons.GenerateInstance()
	agentID := domain.AgentID(comp.boardConfig.AgentEntityID(agentInstance))
	claimedBy := agentID

	// A regular (non-party-required) quest
	questInstance := "skip-quest-" + semdragons.GenerateInstance()
	questID := domain.QuestID(comp.boardConfig.QuestEntityID(questInstance))
	now := time.Now()
	quest := &semdragons.Quest{
		ID:            questID,
		Title:         "Solo Task",
		Status:        semdragons.QuestClaimed,
		Difficulty:    semdragons.DifficultyTrivial,
		PartyRequired: false, // Should NOT trigger auto-formation
		ClaimedBy:     &claimedBy,
		ClaimedAt:     &now,
		PostedAt:      now,
	}
	if err := gc.PutEntityState(ctx, quest, "quest.lifecycle.claimed"); err != nil {
		t.Fatalf("Failed to create test quest: %v", err)
	}

	// Wait a reasonable time for any spurious auto-formation
	time.Sleep(200 * time.Millisecond)

	parties := comp.ListActiveParties()
	if len(parties) != 0 {
		t.Errorf("Expected no parties for non-party-required quest, got %d", len(parties))
	}
}

func TestComponent_HealthReflectsErrors(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client

	comp := setupPartyComponent(t, client, "healtherror")
	defer comp.Stop(5 * time.Second)

	// Artificially inject error count
	comp.errorsCount.Add(5)

	health := comp.Health()
	if health.ErrorCount != 5 {
		t.Errorf("Health.ErrorCount = %d, want 5", health.ErrorCount)
	}
	if health.LastError == "" {
		t.Error("Health.LastError should be set when errors > 0")
	}
	// Component should still be considered healthy (running) despite error count
	if !health.Healthy {
		t.Error("Component should still be healthy while running, even with error count")
	}
}

// =============================================================================
// HELPERS
// =============================================================================

func setupPartyComponent(t *testing.T, client *natsclient.Client, board string) *Component {
	t.Helper()

	deps := component.Dependencies{
		NATSClient: client,
	}

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = board
	config.AutoFormParties = true

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}

	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Ensure board-specific KV bucket exists (mirrors main.go startup).
	// partycoord's Start performs a KV watch so the bucket must exist first.
	gc := semdragons.NewGraphClient(client, comp.boardConfig)
	ctx := context.Background()
	if err := gc.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket failed: %v", err)
	}

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	return comp
}

// makePartyAgentID constructs a fully-qualified agent entity ID for tests.
func makePartyAgentID(t *testing.T, cfg *semdragons.BoardConfig, suffix string) domain.AgentID {
	t.Helper()
	instance := suffix + "-" + semdragons.GenerateInstance()
	return domain.AgentID(cfg.AgentEntityID(instance))
}

// makePartyQuestID constructs a fully-qualified quest entity ID for tests.
func makePartyQuestID(t *testing.T, cfg *semdragons.BoardConfig, suffix string) domain.QuestID {
	t.Helper()
	instance := suffix + "-" + semdragons.GenerateInstance()
	return domain.QuestID(cfg.QuestEntityID(instance))
}
