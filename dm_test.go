//go:build integration

package semdragons

import (
	"context"
	"testing"
	"time"

	"github.com/c360studio/semstreams/natsclient"
)

// =============================================================================
// DM TEST HELPERS
// =============================================================================

func setupDMTestBoard(t *testing.T) (*NATSQuestBoard, *natsclient.Client) {
	t.Helper()
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "dm",
	}

	ctx := context.Background()
	board, err := NewNATSQuestBoard(ctx, tc.Client, config)
	if err != nil {
		t.Fatalf("failed to create board: %v", err)
	}

	return board, tc.Client
}

func createTestAgent(t *testing.T, storage *Storage, level int, skills []SkillTag) *Agent {
	t.Helper()
	instance := GenerateInstance()
	agentID := AgentID(storage.Config().AgentEntityID(instance))

	agent := &Agent{
		ID:        agentID,
		Name:      "test-agent-" + instance,
		Status:    AgentIdle,
		Level:     level,
		XP:        0,
		XPToLevel: 100,
		Tier:      TierFromLevel(level),
		Skills:    skills,
		Stats:     AgentStats{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	ctx := context.Background()
	if err := storage.PutAgent(ctx, instance, agent); err != nil {
		t.Fatalf("failed to create test agent: %v", err)
	}

	return agent
}

func createTestQuest(t *testing.T, board *NATSQuestBoard, difficulty QuestDifficulty, skills []SkillTag) *Quest {
	t.Helper()
	quest := NewQuest("Test Quest").
		Difficulty(difficulty).
		RequireSkills(skills...).
		Build()

	ctx := context.Background()
	posted, err := board.PostQuest(ctx, quest)
	if err != nil {
		t.Fatalf("failed to create test quest: %v", err)
	}

	return posted
}

// =============================================================================
// MANUAL DM TESTS
// =============================================================================

func TestManualDM_SessionLifecycle(t *testing.T) {
	board, tc := setupDMTestBoard(t)

	// Create manual DM with auto-approve mock
	mockRouter := NewMockApprovalRouter()
	mockRouter.SetAutoApprove(true)

	dm := NewManualDungeonMaster(ManualDMConfig{
		BaseDMConfig: BaseDMConfig{
			Client: tc,
			Board:  board,
		},
		ApprovalRouter: mockRouter,
	})

	ctx := context.Background()

	// Start session
	session, err := dm.StartSession(ctx, SessionConfig{
		Mode:        DMManual,
		Name:        "test-session",
		Description: "Unit test session",
	})
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	if session.ID == "" {
		t.Error("session ID should not be empty")
	}
	if !session.Active {
		t.Error("session should be active")
	}
	if session.Config.Mode != DMManual {
		t.Errorf("expected mode DMManual, got %s", session.Config.Mode)
	}

	// Verify session is tracked
	sessions := dm.ListActiveSessions()
	if len(sessions) != 1 {
		t.Errorf("expected 1 active session, got %d", len(sessions))
	}

	// End session
	summary, err := dm.EndSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("EndSession failed: %v", err)
	}

	if summary.SessionID != session.ID {
		t.Errorf("expected session ID %s, got %s", session.ID, summary.SessionID)
	}

	// Verify session is no longer active
	sessions = dm.ListActiveSessions()
	if len(sessions) != 0 {
		t.Errorf("expected 0 active sessions, got %d", len(sessions))
	}
}

func TestManualDM_CreateQuest(t *testing.T) {
	board, tc := setupDMTestBoard(t)

	// Create manual DM with auto-approve mock
	mockRouter := NewMockApprovalRouter()
	mockRouter.SetAutoApprove(true)

	dm := NewManualDungeonMaster(ManualDMConfig{
		BaseDMConfig: BaseDMConfig{
			Client: tc,
			Board:  board,
		},
		ApprovalRouter: mockRouter,
	})

	ctx := context.Background()

	// Start a session first
	_, err := dm.StartSession(ctx, SessionConfig{
		Mode: DMManual,
		Name: "test-session",
	})
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	// Create quest with hints
	hints := QuestHints{
		SuggestedDifficulty: ptrTo(DifficultyHard),
		SuggestedSkills:     []SkillTag{SkillCodeGen, SkillAnalysis},
	}

	quest, err := dm.CreateQuest(ctx, "Implement new feature", hints)
	if err != nil {
		t.Fatalf("CreateQuest failed: %v", err)
	}

	if quest == nil {
		t.Fatal("quest should not be nil")
	}
	if quest.Title != "Implement new feature" {
		t.Errorf("expected title 'Implement new feature', got %s", quest.Title)
	}
	if quest.Difficulty != DifficultyHard {
		t.Errorf("expected difficulty Hard, got %v", quest.Difficulty)
	}
	if quest.Status != QuestPosted {
		t.Errorf("expected status Posted, got %s", quest.Status)
	}
}

func TestManualDM_CreateQuest_Rejected(t *testing.T) {
	board, tc := setupDMTestBoard(t)

	// Create manual DM with rejection
	mockRouter := NewMockApprovalRouter()
	// Pre-configure rejection for the first request
	mockRouter.SetResponse("any", &ApprovalResponse{
		Approved: false,
		Reason:   "test rejection",
	})

	dm := NewManualDungeonMaster(ManualDMConfig{
		BaseDMConfig: BaseDMConfig{
			Client: tc,
			Board:  board,
		},
		ApprovalRouter: mockRouter,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// This should fail because context times out (no auto-approve)
	_, err := dm.CreateQuest(ctx, "Test objective", QuestHints{})
	if err == nil {
		t.Error("expected error from rejected/timed-out approval")
	}
}

func TestManualDM_RecruitAgent(t *testing.T) {
	board, tc := setupDMTestBoard(t)

	mockRouter := NewMockApprovalRouter()
	mockRouter.SetAutoApprove(true)

	dm := NewManualDungeonMaster(ManualDMConfig{
		BaseDMConfig: BaseDMConfig{
			Client: tc,
			Board:  board,
		},
		ApprovalRouter: mockRouter,
	})

	ctx := context.Background()

	// Start session
	_, err := dm.StartSession(ctx, SessionConfig{Mode: DMManual, Name: "test"})
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	// Recruit agent
	config := AgentConfig{
		Provider:    "test",
		Model:       "test-model",
		Temperature: 0.7,
	}

	agent, err := dm.RecruitAgent(ctx, config)
	if err != nil {
		t.Fatalf("RecruitAgent failed: %v", err)
	}

	if agent == nil {
		t.Fatal("agent should not be nil")
	}
	if agent.Level != 1 {
		t.Errorf("expected level 1, got %d", agent.Level)
	}
	if agent.Tier != TierApprentice {
		t.Errorf("expected TierApprentice, got %v", agent.Tier)
	}
	if agent.Status != AgentIdle {
		t.Errorf("expected status Idle, got %s", agent.Status)
	}
}

func TestManualDM_FormParty(t *testing.T) {
	board, tc := setupDMTestBoard(t)

	mockRouter := NewMockApprovalRouter()
	mockRouter.SetAutoApprove(true)

	dm := NewManualDungeonMaster(ManualDMConfig{
		BaseDMConfig: BaseDMConfig{
			Client: tc,
			Board:  board,
		},
		ApprovalRouter: mockRouter,
	})

	ctx := context.Background()

	// Create agents (need at least one that can lead - level 16+)
	storage := board.Storage()
	lead := createTestAgent(t, storage, 16, []SkillTag{SkillCodeGen, SkillPlanning})
	member1 := createTestAgent(t, storage, 5, []SkillTag{SkillCodeGen})
	member2 := createTestAgent(t, storage, 8, []SkillTag{SkillAnalysis})

	// Create a quest requiring a party
	quest := NewQuest("Complex Feature").
		Difficulty(DifficultyEpic).
		RequireSkills(SkillCodeGen, SkillAnalysis).
		RequireParty(2).
		Build()

	posted, err := board.PostQuest(ctx, quest)
	if err != nil {
		t.Fatalf("PostQuest failed: %v", err)
	}

	// Start session
	_, err = dm.StartSession(ctx, SessionConfig{Mode: DMManual, Name: "test"})
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	// Form party
	party, err := dm.FormParty(ctx, posted.ID, PartyStrategyBalanced)
	if err != nil {
		t.Fatalf("FormParty failed: %v", err)
	}

	if party == nil {
		t.Fatal("party should not be nil")
	}
	if party.Lead != lead.ID {
		t.Errorf("expected lead %s, got %s", lead.ID, party.Lead)
	}
	if len(party.Members) < 2 {
		t.Errorf("expected at least 2 members, got %d", len(party.Members))
	}

	// Verify lead is in members
	hasLead := false
	for _, m := range party.Members {
		if m.AgentID == lead.ID && m.Role == RoleLead {
			hasLead = true
			break
		}
	}
	if !hasLead {
		t.Error("party should have lead in members with RoleLead")
	}

	_ = member1 // Used in createTestAgent
	_ = member2 // Used in createTestAgent
}

func TestManualDM_EvaluateAgent(t *testing.T) {
	board, tc := setupDMTestBoard(t)

	dm := NewManualDungeonMaster(ManualDMConfig{
		BaseDMConfig: BaseDMConfig{
			Client: tc,
			Board:  board,
		},
	})

	ctx := context.Background()
	storage := board.Storage()

	// Create agent with good stats
	agent := createTestAgent(t, storage, 10, []SkillTag{SkillCodeGen})

	// Update stats for evaluation
	instance := ExtractInstance(string(agent.ID))
	storage.UpdateAgent(ctx, instance, func(a *Agent) error {
		a.Stats.QuestsCompleted = 10
		a.Stats.QuestsFailed = 2
		a.Stats.BossesDefeated = 8
		a.Stats.BossesFailed = 2
		a.Stats.AvgQualityScore = 0.85
		a.Stats.AvgEfficiency = 0.9
		return nil
	})

	// Evaluate
	eval, err := dm.EvaluateAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("EvaluateAgent failed: %v", err)
	}

	if eval == nil {
		t.Fatal("evaluation should not be nil")
	}
	if eval.AgentID != agent.ID {
		t.Errorf("expected agent ID %s, got %s", agent.ID, eval.AgentID)
	}
	if eval.CurrentLevel != 10 {
		t.Errorf("expected current level 10, got %d", eval.CurrentLevel)
	}
	if len(eval.Strengths) == 0 {
		t.Error("expected some strengths identified")
	}
	if eval.Recommendation == "" {
		t.Error("expected a recommendation")
	}
}

// =============================================================================
// PARTY FORMATION ENGINE TESTS
// =============================================================================

func TestPartyFormation_Balanced(t *testing.T) {
	board, _ := setupDMTestBoard(t)
	storage := board.Storage()

	engine := NewPartyFormationEngine(NewDefaultBoidEngine(), storage)

	ctx := context.Background()

	// Create diverse agents
	lead := createTestAgent(t, storage, 16, []SkillTag{SkillCodeGen, SkillPlanning})
	coder := createTestAgent(t, storage, 8, []SkillTag{SkillCodeGen, SkillCodeReview})
	analyst := createTestAgent(t, storage, 6, []SkillTag{SkillAnalysis, SkillResearch})

	agents := []Agent{*lead, *coder, *analyst}

	// Quest requiring multiple skills
	quest := NewQuest("Multi-skill Quest").
		Difficulty(DifficultyEpic).
		RequireSkills(SkillCodeGen, SkillAnalysis).
		RequireParty(2).
		Build()

	party, err := engine.FormParty(ctx, &quest, PartyStrategyBalanced, agents)
	if err != nil {
		t.Fatalf("FormParty failed: %v", err)
	}

	if party.Lead != lead.ID {
		t.Errorf("expected lead %s, got %s", lead.ID, party.Lead)
	}

	// Verify skills are covered
	coveredSkills := make(map[SkillTag]bool)
	for _, m := range party.Members {
		for _, a := range agents {
			if a.ID == m.AgentID {
				for _, s := range a.Skills {
					coveredSkills[s] = true
				}
			}
		}
	}

	if !coveredSkills[SkillCodeGen] {
		t.Error("CodeGen skill should be covered")
	}
	if !coveredSkills[SkillAnalysis] {
		t.Error("Analysis skill should be covered")
	}
}

func TestPartyFormation_Mentor(t *testing.T) {
	board, _ := setupDMTestBoard(t)
	storage := board.Storage()

	engine := NewPartyFormationEngine(NewDefaultBoidEngine(), storage)

	ctx := context.Background()

	// Create mentor (high level) and apprentices (low level)
	mentor := createTestAgent(t, storage, 18, []SkillTag{SkillCodeGen, SkillPlanning})
	apprentice1 := createTestAgent(t, storage, 3, []SkillTag{SkillCodeGen})
	apprentice2 := createTestAgent(t, storage, 5, []SkillTag{SkillAnalysis})

	agents := []Agent{*mentor, *apprentice1, *apprentice2}

	quest := NewQuest("Learning Quest").
		Difficulty(DifficultyHard).
		RequireParty(2).
		Build()

	party, err := engine.FormParty(ctx, &quest, PartyStrategyMentor, agents)
	if err != nil {
		t.Fatalf("FormParty failed: %v", err)
	}

	// Mentor should be lead
	if party.Lead != mentor.ID {
		t.Errorf("expected mentor %s as lead, got %s", mentor.ID, party.Lead)
	}

	// Should include lower-level members
	hasApprentice := false
	for _, m := range party.Members {
		if m.AgentID == apprentice1.ID || m.AgentID == apprentice2.ID {
			hasApprentice = true
			break
		}
	}
	if !hasApprentice {
		t.Error("party should include apprentice members")
	}
}

func TestPartyFormation_Minimal(t *testing.T) {
	board, _ := setupDMTestBoard(t)
	storage := board.Storage()

	engine := NewPartyFormationEngine(NewDefaultBoidEngine(), storage)

	ctx := context.Background()

	// Create several agents
	lead := createTestAgent(t, storage, 16, []SkillTag{SkillCodeGen})
	agent1 := createTestAgent(t, storage, 8, []SkillTag{SkillCodeGen})
	agent2 := createTestAgent(t, storage, 6, []SkillTag{SkillAnalysis})
	agent3 := createTestAgent(t, storage, 4, []SkillTag{SkillResearch})

	agents := []Agent{*lead, *agent1, *agent2, *agent3}

	// Quest with minimum party size
	quest := NewQuest("Minimal Quest").
		Difficulty(DifficultyHard).
		RequireParty(2).
		Build()

	party, err := engine.FormParty(ctx, &quest, PartyStrategyMinimal, agents)
	if err != nil {
		t.Fatalf("FormParty failed: %v", err)
	}

	// Should have exactly MinPartySize members
	if len(party.Members) != 2 {
		t.Errorf("expected 2 members for minimal party, got %d", len(party.Members))
	}
}

// =============================================================================
// WORLD STATE TESTS
// =============================================================================

func TestWorldState_Aggregation(t *testing.T) {
	board, tc := setupDMTestBoard(t)
	storage := board.Storage()

	dm := NewManualDungeonMaster(ManualDMConfig{
		BaseDMConfig: BaseDMConfig{
			Client: tc,
			Board:  board,
		},
	})

	ctx := context.Background()

	// Create some agents
	createTestAgent(t, storage, 10, []SkillTag{SkillCodeGen})
	createTestAgent(t, storage, 5, []SkillTag{SkillAnalysis})

	// Create some quests
	createTestQuest(t, board, DifficultyModerate, []SkillTag{SkillCodeGen})
	createTestQuest(t, board, DifficultyHard, []SkillTag{SkillAnalysis})

	// Get world state
	world, err := dm.WorldState(ctx)
	if err != nil {
		t.Fatalf("WorldState failed: %v", err)
	}

	if len(world.Agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(world.Agents))
	}
	if len(world.Quests) != 2 {
		t.Errorf("expected 2 quests, got %d", len(world.Quests))
	}
	if world.Stats.OpenQuests != 2 {
		t.Errorf("expected 2 open quests, got %d", world.Stats.OpenQuests)
	}
	if world.Stats.IdleAgents != 2 {
		t.Errorf("expected 2 idle agents, got %d", world.Stats.IdleAgents)
	}
}

func TestWorldState_FilteredQueries(t *testing.T) {
	board, tc := setupDMTestBoard(t)
	storage := board.Storage()

	dm := NewManualDungeonMaster(ManualDMConfig{
		BaseDMConfig: BaseDMConfig{
			Client: tc,
			Board:  board,
		},
	})

	ctx := context.Background()

	// Create agents with different levels/tiers
	createTestAgent(t, storage, 3, []SkillTag{SkillCodeGen})                 // Apprentice
	createTestAgent(t, storage, 8, []SkillTag{SkillCodeGen})                 // Journeyman
	createTestAgent(t, storage, 12, []SkillTag{SkillCodeGen, SkillAnalysis}) // Expert

	// Test GetAgentsByTier
	apprentices, err := dm.GetAgentsByTier(ctx, TierApprentice)
	if err != nil {
		t.Fatalf("GetAgentsByTier failed: %v", err)
	}
	if len(apprentices) != 1 {
		t.Errorf("expected 1 apprentice, got %d", len(apprentices))
	}

	// Test GetAgentsBySkill
	coders, err := dm.GetAgentsBySkill(ctx, SkillCodeGen)
	if err != nil {
		t.Fatalf("GetAgentsBySkill failed: %v", err)
	}
	if len(coders) != 3 {
		t.Errorf("expected 3 agents with CodeGen skill, got %d", len(coders))
	}

	analysts, err := dm.GetAgentsBySkill(ctx, SkillAnalysis)
	if err != nil {
		t.Fatalf("GetAgentsBySkill failed: %v", err)
	}
	if len(analysts) != 1 {
		t.Errorf("expected 1 agent with Analysis skill, got %d", len(analysts))
	}
}

// =============================================================================
// RETIRE AGENT TESTS
// =============================================================================

func TestManualDM_RetireAgent(t *testing.T) {
	board, tc := setupDMTestBoard(t)
	storage := board.Storage()

	mockRouter := NewMockApprovalRouter()
	mockRouter.SetAutoApprove(true)

	dm := NewManualDungeonMaster(ManualDMConfig{
		BaseDMConfig: BaseDMConfig{
			Client: tc,
			Board:  board,
		},
		ApprovalRouter: mockRouter,
	})

	ctx := context.Background()

	// Start session
	_, err := dm.StartSession(ctx, SessionConfig{Name: "test"})
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	// Create an agent
	agent := createTestAgent(t, storage, 5, []SkillTag{SkillCodeGen})

	// Retire the agent
	err = dm.RetireAgent(ctx, agent.ID, "test retirement")
	if err != nil {
		t.Fatalf("RetireAgent failed: %v", err)
	}

	// Verify agent is retired
	instance := ExtractInstance(string(agent.ID))
	retired, err := storage.GetAgent(ctx, instance)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if retired.Status != AgentRetired {
		t.Errorf("expected status AgentRetired, got %s", retired.Status)
	}
}

func TestManualDM_RetireAgent_Denied(t *testing.T) {
	board, tc := setupDMTestBoard(t)
	storage := board.Storage()

	mockRouter := NewMockApprovalRouter()
	// Don't set autoApprove - mock will wait for context cancellation

	dm := NewManualDungeonMaster(ManualDMConfig{
		BaseDMConfig: BaseDMConfig{
			Client: tc,
			Board:  board,
		},
		ApprovalRouter: mockRouter,
	})

	// Create an agent first with background context
	agent := createTestAgent(t, storage, 5, []SkillTag{SkillCodeGen})

	// Use a short timeout context - approval will fail due to timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Try to retire - should fail due to context timeout
	err := dm.RetireAgent(ctx, agent.ID, "test retirement")
	if err == nil {
		t.Fatal("expected error when approval times out")
	}

	// Verify agent is NOT retired
	instance := ExtractInstance(string(agent.ID))
	notRetired, err := storage.GetAgent(context.Background(), instance)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if notRetired.Status == AgentRetired {
		t.Error("agent should not be retired when approval times out")
	}
}

// =============================================================================
// INTERVENTION TESTS
// =============================================================================

func TestManualDM_Intervene(t *testing.T) {
	board, tc := setupDMTestBoard(t)

	mockRouter := NewMockApprovalRouter()
	mockRouter.SetAutoApprove(true)

	dm := NewManualDungeonMaster(ManualDMConfig{
		BaseDMConfig: BaseDMConfig{
			Client: tc,
			Board:  board,
		},
		ApprovalRouter: mockRouter,
	})

	ctx := context.Background()

	// Start session
	_, err := dm.StartSession(ctx, SessionConfig{Name: "test"})
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	// Create a quest
	quest := createTestQuest(t, board, DifficultyModerate, []SkillTag{SkillCodeGen})

	// Intervene with assist
	action := Intervention{
		Type:   InterventionAssist,
		Reason: "Agent needs help",
	}

	err = dm.Intervene(ctx, quest.ID, action)
	if err != nil {
		t.Fatalf("Intervene failed: %v", err)
	}
}

// =============================================================================
// ESCALATION TESTS
// =============================================================================

func TestManualDM_HandleEscalation(t *testing.T) {
	board, tc := setupDMTestBoard(t)

	mockRouter := NewMockApprovalRouter()
	mockRouter.SetAutoApprove(true)
	mockRouter.SetSelectedID("dm_complete")

	dm := NewManualDungeonMaster(ManualDMConfig{
		BaseDMConfig: BaseDMConfig{
			Client: tc,
			Board:  board,
		},
		ApprovalRouter: mockRouter,
	})

	ctx := context.Background()

	// Start session
	_, err := dm.StartSession(ctx, SessionConfig{Name: "test"})
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	// Create a quest
	quest := createTestQuest(t, board, DifficultyEpic, []SkillTag{SkillCodeGen})

	// Handle escalation
	result, err := dm.HandleEscalation(ctx, quest.ID)
	if err != nil {
		t.Fatalf("HandleEscalation failed: %v", err)
	}

	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.QuestID != quest.ID {
		t.Errorf("expected quest ID %s, got %s", quest.ID, result.QuestID)
	}
	if result.Resolution != "completed_by_dm" {
		t.Errorf("expected resolution 'completed_by_dm', got %s", result.Resolution)
	}
	if !result.DMCompleted {
		t.Error("expected DMCompleted to be true")
	}
}

func TestManualDM_HandleEscalation_Cancel(t *testing.T) {
	board, tc := setupDMTestBoard(t)
	storage := board.Storage()

	mockRouter := NewMockApprovalRouter()
	mockRouter.SetAutoApprove(true)
	mockRouter.SetSelectedID("cancel")

	dm := NewManualDungeonMaster(ManualDMConfig{
		BaseDMConfig: BaseDMConfig{
			Client: tc,
			Board:  board,
		},
		ApprovalRouter: mockRouter,
	})

	ctx := context.Background()

	// Start session
	_, err := dm.StartSession(ctx, SessionConfig{Name: "test"})
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	// Create a quest
	quest := createTestQuest(t, board, DifficultyEpic, []SkillTag{SkillCodeGen})

	// Handle escalation with cancel
	result, err := dm.HandleEscalation(ctx, quest.ID)
	if err != nil {
		t.Fatalf("HandleEscalation failed: %v", err)
	}

	if result.Resolution != "cancelled" {
		t.Errorf("expected resolution 'cancelled', got %s", result.Resolution)
	}

	// Verify quest is cancelled in storage
	instance := ExtractInstance(string(quest.ID))
	cancelled, err := storage.GetQuest(ctx, instance)
	if err != nil {
		t.Fatalf("GetQuest failed: %v", err)
	}
	if cancelled.Status != QuestCancelled {
		t.Errorf("expected status QuestCancelled, got %s", cancelled.Status)
	}
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

func ptrTo[T any](v T) *T {
	return &v
}
