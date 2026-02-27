package semdragons

import (
	"context"
	"testing"
	"time"

	"github.com/c360studio/semstreams/natsclient"
)

// TestQuestLifecycle tests the full quest lifecycle:
// post → claim → start → submit → complete
func TestQuestLifecycle(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "lifecycle",
	}

	board, err := NewNATSQuestBoard(ctx, tc.Client, config)
	if err != nil {
		t.Fatalf("failed to create board: %v", err)
	}

	// Create a test agent
	agentInstance := GenerateInstance()
	agent := &Agent{
		ID:     AgentID(config.AgentEntityID(agentInstance)),
		Name:   "TestAgent",
		Level:  10,
		Tier:   TierJourneyman,
		Status: AgentIdle,
		Skills: []SkillTag{SkillAnalysis, SkillCodeGen},
	}
	if err := board.Storage().PutAgent(ctx, agentInstance, agent); err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// 1. Post a quest
	quest := NewQuest("Test Quest").
		Description("A test quest for the lifecycle test").
		Difficulty(DifficultyModerate).
		RequireSkills(SkillAnalysis).
		XP(100).
		ReviewAs(ReviewAuto).
		Build()

	posted, err := board.PostQuest(ctx, quest)
	if err != nil {
		t.Fatalf("PostQuest failed: %v", err)
	}

	if posted.ID == "" {
		t.Error("posted quest has no ID")
	}
	if posted.Status != QuestPosted {
		t.Errorf("expected status %s, got %s", QuestPosted, posted.Status)
	}

	// 2. Verify quest is available
	available, err := board.AvailableQuests(ctx, agent.ID, QuestFilter{})
	if err != nil {
		t.Fatalf("AvailableQuests failed: %v", err)
	}
	if len(available) != 1 {
		t.Errorf("expected 1 available quest, got %d", len(available))
	}

	// 3. Claim the quest
	if err := board.ClaimQuest(ctx, posted.ID, agent.ID); err != nil {
		t.Fatalf("ClaimQuest failed: %v", err)
	}

	claimed, err := board.GetQuest(ctx, posted.ID)
	if err != nil {
		t.Fatalf("GetQuest failed: %v", err)
	}
	if claimed.Status != QuestClaimed {
		t.Errorf("expected status %s, got %s", QuestClaimed, claimed.Status)
	}
	if claimed.ClaimedBy == nil || *claimed.ClaimedBy != agent.ID {
		t.Error("quest not claimed by expected agent")
	}

	// 4. Start the quest
	if err := board.StartQuest(ctx, posted.ID); err != nil {
		t.Fatalf("StartQuest failed: %v", err)
	}

	started, err := board.GetQuest(ctx, posted.ID)
	if err != nil {
		t.Fatalf("GetQuest failed: %v", err)
	}
	if started.Status != QuestInProgress {
		t.Errorf("expected status %s, got %s", QuestInProgress, started.Status)
	}
	if started.StartedAt == nil {
		t.Error("started_at not set")
	}

	// 5. Submit result (triggers boss battle since review is enabled)
	// With auto-evaluation enabled, the evaluator runs immediately and
	// completes the quest if evaluation passes
	result := map[string]string{"analysis": "complete"}
	battle, err := board.SubmitResult(ctx, posted.ID, result)
	if err != nil {
		t.Fatalf("SubmitResult failed: %v", err)
	}

	if battle == nil {
		t.Error("expected boss battle to be created")
	}

	// With ReviewAuto and valid output, the evaluator completes immediately
	// Battle status should be victory (auto-evaluation passed)
	if battle != nil && battle.Status != BattleVictory {
		t.Errorf("expected battle status %s, got %s", BattleVictory, battle.Status)
	}

	// Quest should be completed (auto-evaluation passed and completed the quest)
	completed, err := board.GetQuest(ctx, posted.ID)
	if err != nil {
		t.Fatalf("GetQuest failed: %v", err)
	}
	if completed.Status != QuestCompleted {
		t.Errorf("expected status %s, got %s", QuestCompleted, completed.Status)
	}
	if completed.CompletedAt == nil {
		t.Error("completed_at not set")
	}
}

// TestClaimQuestBasic tests basic claiming flow
func TestClaimQuestBasic(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "claimbasic",
	}

	board, err := NewNATSQuestBoard(ctx, tc.Client, config)
	if err != nil {
		t.Fatalf("failed to create board: %v", err)
	}

	// Create agent
	agentInstance := GenerateInstance()
	agent := &Agent{
		ID:     AgentID(config.AgentEntityID(agentInstance)),
		Level:  5,
		Tier:   TierApprentice,
		Status: AgentIdle,
	}
	if err := board.Storage().PutAgent(ctx, agentInstance, agent); err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Post quest
	quest := NewQuest("Simple Task").
		Difficulty(DifficultyTrivial).
		Build()

	posted, err := board.PostQuest(ctx, quest)
	if err != nil {
		t.Fatalf("PostQuest failed: %v", err)
	}

	// Claim quest
	if err := board.ClaimQuest(ctx, posted.ID, agent.ID); err != nil {
		t.Fatalf("ClaimQuest failed: %v", err)
	}

	// Verify claim
	claimed, err := board.GetQuest(ctx, posted.ID)
	if err != nil {
		t.Fatalf("GetQuest failed: %v", err)
	}
	if claimed.Status != QuestClaimed {
		t.Errorf("expected status %s, got %s", QuestClaimed, claimed.Status)
	}
	if claimed.ClaimedBy == nil {
		t.Error("quest should have claimer")
	}
}

// TestCannotClaimAlreadyClaimed tests that double-claiming fails
func TestCannotClaimAlreadyClaimed(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "doubleclaim",
	}

	board, err := NewNATSQuestBoard(ctx, tc.Client, config)
	if err != nil {
		t.Fatalf("failed to create board: %v", err)
	}

	// Create two agents
	agent1Instance := GenerateInstance()
	agent1 := &Agent{
		ID:     AgentID(config.AgentEntityID(agent1Instance)),
		Level:  5,
		Tier:   TierApprentice,
		Status: AgentIdle,
	}
	if err := board.Storage().PutAgent(ctx, agent1Instance, agent1); err != nil {
		t.Fatalf("failed to create agent1: %v", err)
	}

	agent2Instance := GenerateInstance()
	agent2 := &Agent{
		ID:     AgentID(config.AgentEntityID(agent2Instance)),
		Level:  5,
		Tier:   TierApprentice,
		Status: AgentIdle,
	}
	if err := board.Storage().PutAgent(ctx, agent2Instance, agent2); err != nil {
		t.Fatalf("failed to create agent2: %v", err)
	}

	// Post quest
	quest := NewQuest("Contested Task").
		Difficulty(DifficultyTrivial).
		Build()

	posted, err := board.PostQuest(ctx, quest)
	if err != nil {
		t.Fatalf("PostQuest failed: %v", err)
	}

	// First claim should succeed
	if err := board.ClaimQuest(ctx, posted.ID, agent1.ID); err != nil {
		t.Fatalf("first ClaimQuest failed: %v", err)
	}

	// Second claim should fail
	err = board.ClaimQuest(ctx, posted.ID, agent2.ID)
	if err == nil {
		t.Error("expected error when claiming already-claimed quest")
	}
}

// TestAbandonQuest tests quest abandonment
func TestAbandonQuest(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "abandon",
	}

	board, err := NewNATSQuestBoard(ctx, tc.Client, config)
	if err != nil {
		t.Fatalf("failed to create board: %v", err)
	}

	// Create agent
	agentInstance := GenerateInstance()
	agent := &Agent{
		ID:     AgentID(config.AgentEntityID(agentInstance)),
		Level:  10,
		Tier:   TierJourneyman,
		Status: AgentIdle,
	}
	if err := board.Storage().PutAgent(ctx, agentInstance, agent); err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Post and claim quest
	quest := NewQuest("Abandon Test").
		Difficulty(DifficultyModerate).
		Build()

	posted, err := board.PostQuest(ctx, quest)
	if err != nil {
		t.Fatalf("PostQuest failed: %v", err)
	}

	if err := board.ClaimQuest(ctx, posted.ID, agent.ID); err != nil {
		t.Fatalf("ClaimQuest failed: %v", err)
	}

	// Abandon the quest
	if err := board.AbandonQuest(ctx, posted.ID, "changed my mind"); err != nil {
		t.Fatalf("AbandonQuest failed: %v", err)
	}

	// Verify quest is back on board
	abandoned, err := board.GetQuest(ctx, posted.ID)
	if err != nil {
		t.Fatalf("GetQuest failed: %v", err)
	}
	if abandoned.Status != QuestPosted {
		t.Errorf("expected status %s, got %s", QuestPosted, abandoned.Status)
	}
	if abandoned.ClaimedBy != nil {
		t.Error("quest should not have claimer after abandon")
	}
}

// TestFailQuest tests quest failure and reposting
func TestFailQuest(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "fail",
	}

	board, err := NewNATSQuestBoard(ctx, tc.Client, config)
	if err != nil {
		t.Fatalf("failed to create board: %v", err)
	}

	// Create agent
	agentInstance := GenerateInstance()
	agent := &Agent{
		ID:     AgentID(config.AgentEntityID(agentInstance)),
		Level:  10,
		Tier:   TierJourneyman,
		Status: AgentIdle,
	}
	if err := board.Storage().PutAgent(ctx, agentInstance, agent); err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Post quest with 2 max attempts
	quest := NewQuest("Fail Test").
		Difficulty(DifficultyModerate).
		NoReview().
		MaxRetries(2).
		Build()

	posted, err := board.PostQuest(ctx, quest)
	if err != nil {
		t.Fatalf("PostQuest failed: %v", err)
	}

	// First attempt - claim, start, fail
	if err := board.ClaimQuest(ctx, posted.ID, agent.ID); err != nil {
		t.Fatalf("ClaimQuest failed: %v", err)
	}
	if err := board.StartQuest(ctx, posted.ID); err != nil {
		t.Fatalf("StartQuest failed: %v", err)
	}
	if err := board.FailQuest(ctx, posted.ID, "first failure"); err != nil {
		t.Fatalf("FailQuest failed: %v", err)
	}

	// Quest should be reposted (attempt 1 < max 2)
	failed1, err := board.GetQuest(ctx, posted.ID)
	if err != nil {
		t.Fatalf("GetQuest failed: %v", err)
	}
	if failed1.Status != QuestPosted {
		t.Errorf("expected status %s after first failure, got %s", QuestPosted, failed1.Status)
	}
	if failed1.Attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", failed1.Attempts)
	}

	// Second attempt - claim, start, fail
	if err := board.ClaimQuest(ctx, posted.ID, agent.ID); err != nil {
		t.Fatalf("ClaimQuest failed: %v", err)
	}
	if err := board.StartQuest(ctx, posted.ID); err != nil {
		t.Fatalf("StartQuest failed: %v", err)
	}
	if err := board.FailQuest(ctx, posted.ID, "second failure"); err != nil {
		t.Fatalf("FailQuest failed: %v", err)
	}

	// Quest should be permanently failed (attempt 2 >= max 2)
	failed2, err := board.GetQuest(ctx, posted.ID)
	if err != nil {
		t.Fatalf("GetQuest failed: %v", err)
	}
	if failed2.Status != QuestFailed {
		t.Errorf("expected status %s after max attempts, got %s", QuestFailed, failed2.Status)
	}
}

// TestEscalateQuest tests quest escalation
func TestEscalateQuest(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "escalate",
	}

	board, err := NewNATSQuestBoard(ctx, tc.Client, config)
	if err != nil {
		t.Fatalf("failed to create board: %v", err)
	}

	// Create agent
	agentInstance := GenerateInstance()
	agent := &Agent{
		ID:     AgentID(config.AgentEntityID(agentInstance)),
		Level:  10,
		Tier:   TierJourneyman,
		Status: AgentIdle,
	}
	if err := board.Storage().PutAgent(ctx, agentInstance, agent); err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Post and claim quest
	quest := NewQuest("Escalate Test").
		Difficulty(DifficultyModerate).
		Build()

	posted, err := board.PostQuest(ctx, quest)
	if err != nil {
		t.Fatalf("PostQuest failed: %v", err)
	}

	if err := board.ClaimQuest(ctx, posted.ID, agent.ID); err != nil {
		t.Fatalf("ClaimQuest failed: %v", err)
	}

	if err := board.StartQuest(ctx, posted.ID); err != nil {
		t.Fatalf("StartQuest failed: %v", err)
	}

	// Escalate the quest
	if err := board.EscalateQuest(ctx, posted.ID, "need higher-level help"); err != nil {
		t.Fatalf("EscalateQuest failed: %v", err)
	}

	// Verify quest is escalated
	escalated, err := board.GetQuest(ctx, posted.ID)
	if err != nil {
		t.Fatalf("GetQuest failed: %v", err)
	}
	if escalated.Status != QuestEscalated {
		t.Errorf("expected status %s, got %s", QuestEscalated, escalated.Status)
	}
	if !escalated.Escalated {
		t.Error("escalated flag should be true")
	}
}

// TestBoardStats tests board statistics
func TestBoardStats(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "stats",
	}

	board, err := NewNATSQuestBoard(ctx, tc.Client, config)
	if err != nil {
		t.Fatalf("failed to create board: %v", err)
	}

	// Create agent
	agentInstance := GenerateInstance()
	agent := &Agent{
		ID:     AgentID(config.AgentEntityID(agentInstance)),
		Level:  10,
		Tier:   TierJourneyman,
		Status: AgentIdle,
	}
	if err := board.Storage().PutAgent(ctx, agentInstance, agent); err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Post 3 quests
	for range 3 {
		quest := NewQuest("Stats Quest").
			Difficulty(DifficultyModerate).
			NoReview().
			Build()
		if _, err := board.PostQuest(ctx, quest); err != nil {
			t.Fatalf("PostQuest failed: %v", err)
		}
	}

	stats, err := board.BoardStats(ctx)
	if err != nil {
		t.Fatalf("BoardStats failed: %v", err)
	}

	if stats.TotalPosted != 3 {
		t.Errorf("expected 3 posted, got %d", stats.TotalPosted)
	}
}

// TestSubQuestDecomposition tests quest decomposition
func TestSubQuestDecomposition(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "subquest",
	}

	board, err := NewNATSQuestBoard(ctx, tc.Client, config)
	if err != nil {
		t.Fatalf("failed to create board: %v", err)
	}

	// Create a Master-tier agent (can decompose quests)
	masterInstance := GenerateInstance()
	master := &Agent{
		ID:     AgentID(config.AgentEntityID(masterInstance)),
		Name:   "PartyLead",
		Level:  17,
		Tier:   TierMaster,
		Status: AgentIdle,
	}
	if err := board.Storage().PutAgent(ctx, masterInstance, master); err != nil {
		t.Fatalf("failed to create master agent: %v", err)
	}

	// Create an Apprentice-tier agent (cannot decompose quests)
	apprenticeInstance := GenerateInstance()
	apprentice := &Agent{
		ID:     AgentID(config.AgentEntityID(apprenticeInstance)),
		Name:   "Newbie",
		Level:  3,
		Tier:   TierApprentice,
		Status: AgentIdle,
	}
	if err := board.Storage().PutAgent(ctx, apprenticeInstance, apprentice); err != nil {
		t.Fatalf("failed to create apprentice agent: %v", err)
	}

	// Post an epic quest
	epic := NewQuest("Epic Quest").
		Difficulty(DifficultyEpic).
		Build()

	posted, err := board.PostQuest(ctx, epic)
	if err != nil {
		t.Fatalf("PostQuest failed: %v", err)
	}

	// Claim with master
	if err := board.ClaimQuest(ctx, posted.ID, master.ID); err != nil {
		t.Fatalf("ClaimQuest failed: %v", err)
	}

	// Try to decompose with apprentice (should fail)
	subQuests := []Quest{
		NewQuest("Sub-task 1").Difficulty(DifficultyModerate).Build(),
		NewQuest("Sub-task 2").Difficulty(DifficultyModerate).Build(),
	}
	if _, err := board.PostSubQuests(ctx, posted.ID, subQuests, apprentice.ID); err == nil {
		t.Error("expected error when apprentice tries to decompose quest")
	}

	// Decompose with master (should succeed)
	posted2, err := board.PostSubQuests(ctx, posted.ID, subQuests, master.ID)
	if err != nil {
		t.Fatalf("PostSubQuests failed: %v", err)
	}

	if len(posted2) != 2 {
		t.Errorf("expected 2 sub-quests, got %d", len(posted2))
	}

	// Verify parent has sub-quest IDs
	parent, err := board.GetQuest(ctx, posted.ID)
	if err != nil {
		t.Fatalf("GetQuest failed: %v", err)
	}
	if len(parent.SubQuests) != 2 {
		t.Errorf("expected parent to have 2 sub-quests, got %d", len(parent.SubQuests))
	}

	// Verify sub-quests reference parent
	for _, sq := range posted2 {
		if sq.ParentQuest == nil || *sq.ParentQuest != posted.ID {
			t.Errorf("sub-quest should reference parent %s", posted.ID)
		}
	}
}

// TestNoReviewQuestCompletesImmediately tests quests without review
func TestNoReviewQuestCompletesImmediately(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "noreview",
	}

	board, err := NewNATSQuestBoard(ctx, tc.Client, config)
	if err != nil {
		t.Fatalf("failed to create board: %v", err)
	}

	// Create agent
	agentInstance := GenerateInstance()
	agent := &Agent{
		ID:     AgentID(config.AgentEntityID(agentInstance)),
		Level:  10,
		Tier:   TierJourneyman,
		Status: AgentIdle,
	}
	if err := board.Storage().PutAgent(ctx, agentInstance, agent); err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Post quest without review
	quest := NewQuest("No Review Quest").
		Difficulty(DifficultyModerate).
		NoReview().
		Build()

	posted, err := board.PostQuest(ctx, quest)
	if err != nil {
		t.Fatalf("PostQuest failed: %v", err)
	}

	// Claim and start
	if err := board.ClaimQuest(ctx, posted.ID, agent.ID); err != nil {
		t.Fatalf("ClaimQuest failed: %v", err)
	}
	if err := board.StartQuest(ctx, posted.ID); err != nil {
		t.Fatalf("StartQuest failed: %v", err)
	}

	// Submit - should complete immediately (no boss battle)
	result := "task done"
	battle, err := board.SubmitResult(ctx, posted.ID, result)
	if err != nil {
		t.Fatalf("SubmitResult failed: %v", err)
	}

	if battle != nil {
		t.Error("expected no boss battle for NoReview quest")
	}

	// Check quest is completed
	completed, err := board.GetQuest(ctx, posted.ID)
	if err != nil {
		t.Fatalf("GetQuest failed: %v", err)
	}
	if completed.Status != QuestCompleted {
		t.Errorf("expected status %s, got %s", QuestCompleted, completed.Status)
	}
}

// Helper functions

func ptrTime(t time.Time) *time.Time {
	return &t
}
