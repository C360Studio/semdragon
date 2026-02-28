package semdragons

import (
	"context"
	"testing"
	"time"

	"github.com/c360studio/semstreams/natsclient"
)

// TestPartyCoordinator_DecomposeQuest tests the decomposition flow:
// Lead decomposes quest → event emitted with sub-quest IDs
func TestPartyCoordinator_DecomposeQuest(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "partycoord",
	}

	storage, err := CreateStorage(ctx, tc.Client, &config)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	// Create party and coordinator
	partyInstance := GenerateInstance()
	partyID := PartyID(config.PartyEntityID(partyInstance))

	party := &Party{
		ID:     partyID,
		Name:   "Test Party",
		Status: PartyActive,
		Lead:   "lead-agent",
		Members: []PartyMember{
			{AgentID: "lead-agent", Role: RoleLead},
			{AgentID: "member-1", Role: RoleExecutor},
			{AgentID: "member-2", Role: RoleExecutor},
		},
	}
	if err := storage.PutParty(ctx, partyInstance, party); err != nil {
		t.Fatalf("failed to create party: %v", err)
	}

	coordinator := NewPartyCoordinator(tc.Client, storage, partyID)

	// Decompose quest
	parentQuestID := QuestID("parent-quest-1")
	subQuests := []QuestID{"sub-quest-1", "sub-quest-2", "sub-quest-3"}

	err = coordinator.DecomposeQuest(ctx, "lead-agent", parentQuestID, subQuests, "divide and conquer")
	if err != nil {
		t.Fatalf("DecomposeQuest failed: %v", err)
	}

	// Verify sub-quest map was created
	subQuestMap, err := coordinator.GetAssignments(ctx)
	if err != nil {
		t.Fatalf("GetAssignments failed: %v", err)
	}
	if len(subQuestMap) != 3 {
		t.Errorf("expected 3 sub-quests in map, got %d", len(subQuestMap))
	}

	// All should be unassigned initially
	for _, sq := range subQuests {
		if assigned := subQuestMap[sq]; assigned != "" {
			t.Errorf("sub-quest %s should be unassigned, got %s", sq, assigned)
		}
	}
}

// TestPartyCoordinator_AssignTask tests the assignment flow:
// Lead assigns member → member receives event
func TestPartyCoordinator_AssignTask(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "partyassign",
	}

	storage, err := CreateStorage(ctx, tc.Client, &config)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	partyInstance := GenerateInstance()
	partyID := PartyID(config.PartyEntityID(partyInstance))

	party := &Party{
		ID:     partyID,
		Name:   "Assign Test Party",
		Status: PartyActive,
		Lead:   "lead-agent",
		Members: []PartyMember{
			{AgentID: "lead-agent", Role: RoleLead},
			{AgentID: "worker-1", Role: RoleExecutor},
		},
	}
	if err := storage.PutParty(ctx, partyInstance, party); err != nil {
		t.Fatalf("failed to create party: %v", err)
	}

	coordinator := NewPartyCoordinator(tc.Client, storage, partyID)

	// First decompose
	subQuests := []QuestID{"task-a", "task-b"}
	if err := coordinator.DecomposeQuest(ctx, "lead-agent", "parent", subQuests, "split work"); err != nil {
		t.Fatalf("DecomposeQuest failed: %v", err)
	}

	// Assign task to member
	err = coordinator.AssignTask(ctx, "lead-agent", "worker-1", "task-a", "good skill match", nil, "start with the basics")
	if err != nil {
		t.Fatalf("AssignTask failed: %v", err)
	}

	// Verify assignment in storage
	assignments, err := coordinator.GetAssignments(ctx)
	if err != nil {
		t.Fatalf("GetAssignments failed: %v", err)
	}
	if assignments["task-a"] != "worker-1" {
		t.Errorf("expected task-a assigned to worker-1, got %s", assignments["task-a"])
	}
	if assignments["task-b"] != "" {
		t.Errorf("task-b should still be unassigned")
	}
}

// TestPartyCoordinator_ShareContext tests context sharing:
// Member shares insight → context stored
func TestPartyCoordinator_ShareContext(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "partyctx",
	}

	storage, err := CreateStorage(ctx, tc.Client, &config)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	partyInstance := GenerateInstance()
	partyID := PartyID(config.PartyEntityID(partyInstance))

	party := &Party{
		ID:     partyID,
		Name:   "Context Test Party",
		Status: PartyActive,
		Lead:   "lead",
	}
	if err := storage.PutParty(ctx, partyInstance, party); err != nil {
		t.Fatalf("failed to create party: %v", err)
	}

	coordinator := NewPartyCoordinator(tc.Client, storage, partyID)

	// Share some context
	item1 := ContextItem{
		Key:     "api_endpoint",
		Value:   "https://api.example.com/v2",
		AddedBy: "lead",
		AddedAt: time.Now(),
	}
	if err := coordinator.ShareContext(ctx, "lead", item1, []QuestID{"task-a"}); err != nil {
		t.Fatalf("ShareContext failed: %v", err)
	}

	item2 := ContextItem{
		Key:     "auth_token",
		Value:   "secret123",
		AddedBy: "member-1",
		AddedAt: time.Now(),
	}
	if err := coordinator.ShareContext(ctx, "member-1", item2, nil); err != nil {
		t.Fatalf("ShareContext failed: %v", err)
	}

	// Retrieve shared context
	sharedContext, err := coordinator.GetSharedContext(ctx)
	if err != nil {
		t.Fatalf("GetSharedContext failed: %v", err)
	}
	if len(sharedContext) != 2 {
		t.Errorf("expected 2 context items, got %d", len(sharedContext))
	}
	if sharedContext[0].Key != "api_endpoint" {
		t.Errorf("expected first context key 'api_endpoint', got %s", sharedContext[0].Key)
	}
}

// TestPartyCoordinator_HelpRequestAndGuidance tests the help flow:
// Member requests help → lead responds with guidance
func TestPartyCoordinator_HelpRequestAndGuidance(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "partyhelp",
	}

	storage, err := CreateStorage(ctx, tc.Client, &config)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	partyInstance := GenerateInstance()
	partyID := PartyID(config.PartyEntityID(partyInstance))

	party := &Party{
		ID:     partyID,
		Name:   "Help Test Party",
		Status: PartyActive,
		Lead:   "lead",
	}
	if err := storage.PutParty(ctx, partyInstance, party); err != nil {
		t.Fatalf("failed to create party: %v", err)
	}

	coordinator := NewPartyCoordinator(tc.Client, storage, partyID)

	// Member requests help (this just emits an event)
	err = coordinator.RequestHelp(ctx, "member-1", "task-1", "blocker", "Cannot access the database", "high")
	if err != nil {
		t.Fatalf("RequestHelp failed: %v", err)
	}

	// Lead issues guidance (this also just emits an event)
	err = coordinator.IssueGuidance(ctx, "lead", "member-1", "task-1", "redirect", "Use the staging database credentials from secrets manager")
	if err != nil {
		t.Fatalf("IssueGuidance failed: %v", err)
	}
}

// TestPartyCoordinator_ResultCollection tests result submission flow:
// Member submits result → stored in SubResults
func TestPartyCoordinator_ResultCollection(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "partyresults",
	}

	storage, err := CreateStorage(ctx, tc.Client, &config)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	partyInstance := GenerateInstance()
	partyID := PartyID(config.PartyEntityID(partyInstance))

	party := &Party{
		ID:     partyID,
		Name:   "Results Test Party",
		Status: PartyActive,
		Lead:   "lead",
	}
	if err := storage.PutParty(ctx, partyInstance, party); err != nil {
		t.Fatalf("failed to create party: %v", err)
	}

	coordinator := NewPartyCoordinator(tc.Client, storage, partyID)

	// Set up sub-quests
	subQuests := []QuestID{
		QuestID(config.QuestEntityID("sub1")),
		QuestID(config.QuestEntityID("sub2")),
	}
	if err := coordinator.DecomposeQuest(ctx, "lead", "parent", subQuests, "test"); err != nil {
		t.Fatalf("DecomposeQuest failed: %v", err)
	}

	// Member 1 submits result
	result1 := map[string]any{"output": "analysis complete", "confidence": 0.95}
	if err := coordinator.SubmitResult(ctx, "member-1", subQuests[0], result1, 0.9); err != nil {
		t.Fatalf("SubmitResult failed: %v", err)
	}

	// Check if all results collected (should be false - only 1 of 2)
	allCollected, err := coordinator.AreAllResultsCollected(ctx)
	if err != nil {
		t.Fatalf("AreAllResultsCollected failed: %v", err)
	}
	if allCollected {
		t.Error("expected not all results collected yet")
	}

	// Member 2 submits result
	result2 := map[string]any{"output": "data processed", "rows": 1000}
	if err := coordinator.SubmitResult(ctx, "member-2", subQuests[1], result2, 0.85); err != nil {
		t.Fatalf("SubmitResult failed: %v", err)
	}

	// Now all should be collected
	allCollected, err = coordinator.AreAllResultsCollected(ctx)
	if err != nil {
		t.Fatalf("AreAllResultsCollected failed: %v", err)
	}
	if !allCollected {
		t.Error("expected all results collected")
	}

	// Verify results
	results, err := coordinator.GetCollectedResults(ctx)
	if err != nil {
		t.Fatalf("GetCollectedResults failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

// TestPartyCoordinator_RollupFlow tests the rollup flow:
// All results collected → lead performs rollup → rollup completed
func TestPartyCoordinator_RollupFlow(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "partyrollup",
	}

	storage, err := CreateStorage(ctx, tc.Client, &config)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	partyInstance := GenerateInstance()
	partyID := PartyID(config.PartyEntityID(partyInstance))

	party := &Party{
		ID:     partyID,
		Name:   "Rollup Test Party",
		Status: PartyActive,
		Lead:   "lead",
	}
	if err := storage.PutParty(ctx, partyInstance, party); err != nil {
		t.Fatalf("failed to create party: %v", err)
	}

	coordinator := NewPartyCoordinator(tc.Client, storage, partyID)

	parentQuestID := QuestID(config.QuestEntityID("parent"))
	subQuests := []QuestID{
		QuestID(config.QuestEntityID("sub1")),
		QuestID(config.QuestEntityID("sub2")),
	}

	// Setup: decompose and collect results
	if err := coordinator.DecomposeQuest(ctx, "lead", parentQuestID, subQuests, "test"); err != nil {
		t.Fatalf("DecomposeQuest failed: %v", err)
	}

	if err := coordinator.SubmitResult(ctx, "m1", subQuests[0], "result1", 0.9); err != nil {
		t.Fatalf("SubmitResult failed: %v", err)
	}
	if err := coordinator.SubmitResult(ctx, "m2", subQuests[1], "result2", 0.85); err != nil {
		t.Fatalf("SubmitResult failed: %v", err)
	}

	// Start rollup
	if err := coordinator.StartRollup(ctx, "lead", parentQuestID); err != nil {
		t.Fatalf("StartRollup failed: %v", err)
	}

	// Complete rollup
	rollupResult := map[string]any{
		"combined":    "result1 + result2",
		"total_score": 0.875,
	}
	memberContrib := map[AgentID]float64{
		"m1": 0.5,
		"m2": 0.5,
	}

	if err := coordinator.CompleteRollup(ctx, "lead", parentQuestID, rollupResult, memberContrib); err != nil {
		t.Fatalf("CompleteRollup failed: %v", err)
	}

	// Verify rollup result stored
	storedRollup, err := coordinator.GetRollupResult(ctx)
	if err != nil {
		t.Fatalf("GetRollupResult failed: %v", err)
	}

	rollupMap, ok := storedRollup.(map[string]any)
	if !ok {
		t.Fatalf("expected rollup result to be map, got %T", storedRollup)
	}
	if rollupMap["combined"] != "result1 + result2" {
		t.Errorf("unexpected rollup result: %v", rollupMap)
	}
}

// TestPartyCoordinator_ProgressReporting tests progress updates
func TestPartyCoordinator_ProgressReporting(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "partyprog",
	}

	storage, err := CreateStorage(ctx, tc.Client, &config)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	partyInstance := GenerateInstance()
	partyID := PartyID(config.PartyEntityID(partyInstance))

	party := &Party{
		ID:     partyID,
		Name:   "Progress Test Party",
		Status: PartyActive,
		Lead:   "lead",
	}
	if err := storage.PutParty(ctx, partyInstance, party); err != nil {
		t.Fatalf("failed to create party: %v", err)
	}

	coordinator := NewPartyCoordinator(tc.Client, storage, partyID)

	// Report progress (just emits event, no state change to verify)
	err = coordinator.ReportProgress(ctx, "member-1", "task-1", 25, "on_track", "Just getting started")
	if err != nil {
		t.Fatalf("ReportProgress failed: %v", err)
	}

	err = coordinator.ReportProgress(ctx, "member-1", "task-1", 50, "on_track", "Halfway there")
	if err != nil {
		t.Fatalf("ReportProgress failed: %v", err)
	}

	err = coordinator.ReportProgress(ctx, "member-1", "task-1", 75, "ahead", "Making good progress")
	if err != nil {
		t.Fatalf("ReportProgress failed: %v", err)
	}
}
