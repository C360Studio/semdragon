//go:build integration

package semdragons

import (
	"context"
	"testing"
	"time"

	"github.com/c360studio/semstreams/natsclient"
)

func TestProgression_XPAwarded(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "progression",
	}

	storage, err := CreateStorage(ctx, tc.Client, &config)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	events := NewEventPublisher(tc.Client)
	xpEngine := NewDefaultXPEngine()
	pm := NewProgressionManager(storage, xpEngine, events)

	// Create agent
	agentInstance := GenerateInstance()
	agent := &Agent{
		ID:        AgentID(config.AgentEntityID(agentInstance)),
		Name:      "TestAgent",
		Level:     5,
		XP:        50,
		XPToLevel: xpEngine.XPToNextLevel(5),
		Tier:      TierApprentice,
		Status:    AgentIdle,
	}
	if err := storage.PutAgent(ctx, agentInstance, agent); err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Process success
	quest := Quest{
		ID:       QuestID(config.QuestEntityID("quest1")),
		Title:    "Test Quest",
		BaseXP:   100,
		Attempts: 1,
	}

	pctx := ProgressionContext{
		Quest:   quest,
		AgentID: agent.ID,
		Verdict: BattleVerdict{
			Passed:       true,
			QualityScore: 0.9,
		},
		Duration: 5 * time.Minute,
	}

	result, err := pm.ProcessSuccess(ctx, pctx)
	if err != nil {
		t.Fatalf("ProcessSuccess failed: %v", err)
	}

	if result.Award == nil {
		t.Fatal("expected XP award")
	}

	if result.Award.TotalXP <= 0 {
		t.Errorf("expected positive XP award, got %d", result.Award.TotalXP)
	}

	if result.XPAfter <= result.XPBefore {
		t.Errorf("expected XP to increase: before=%d, after=%d", result.XPBefore, result.XPAfter)
	}

	// Verify agent was updated
	updated, err := storage.GetAgent(ctx, agentInstance)
	if err != nil {
		t.Fatalf("failed to get updated agent: %v", err)
	}

	if updated.XP != result.XPAfter {
		t.Errorf("agent XP not persisted: got %d, want %d", updated.XP, result.XPAfter)
	}

	if updated.Stats.QuestsCompleted != 1 {
		t.Errorf("expected QuestsCompleted=1, got %d", updated.Stats.QuestsCompleted)
	}
}

func TestProgression_LevelUp(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "levelup",
	}

	storage, err := CreateStorage(ctx, tc.Client, &config)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	events := NewEventPublisher(tc.Client)
	xpEngine := NewDefaultXPEngine()
	pm := NewProgressionManager(storage, xpEngine, events)

	// Create agent close to leveling up
	agentInstance := GenerateInstance()
	xpToLevel := xpEngine.XPToNextLevel(1)
	agent := &Agent{
		ID:        AgentID(config.AgentEntityID(agentInstance)),
		Name:      "TestAgent",
		Level:     1,
		XP:        xpToLevel - 10, // Close to level up
		XPToLevel: xpToLevel,
		Tier:      TierApprentice,
		Status:    AgentIdle,
	}
	if err := storage.PutAgent(ctx, agentInstance, agent); err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Process success with high quality to ensure level up
	quest := Quest{
		ID:       QuestID(config.QuestEntityID("quest1")),
		Title:    "Level Up Quest",
		BaseXP:   500, // Large XP to trigger level up
		Attempts: 1,
	}

	pctx := ProgressionContext{
		Quest:   quest,
		AgentID: agent.ID,
		Verdict: BattleVerdict{
			Passed:       true,
			QualityScore: 1.0,
		},
		Duration: 5 * time.Minute,
	}

	result, err := pm.ProcessSuccess(ctx, pctx)
	if err != nil {
		t.Fatalf("ProcessSuccess failed: %v", err)
	}

	if result.LevelAfter <= result.LevelBefore {
		t.Errorf("expected level to increase: before=%d, after=%d",
			result.LevelBefore, result.LevelAfter)
	}

	// Verify agent level was updated
	updated, err := storage.GetAgent(ctx, agentInstance)
	if err != nil {
		t.Fatalf("failed to get updated agent: %v", err)
	}

	if updated.Level != result.LevelAfter {
		t.Errorf("agent level not persisted: got %d, want %d", updated.Level, result.LevelAfter)
	}
}

func TestProgression_StreakBonus(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "streak",
	}

	storage, err := CreateStorage(ctx, tc.Client, &config)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	events := NewEventPublisher(tc.Client)
	xpEngine := NewDefaultXPEngine()
	pm := NewProgressionManager(storage, xpEngine, events)

	// Create agent
	agentInstance := GenerateInstance()
	agent := &Agent{
		ID:        AgentID(config.AgentEntityID(agentInstance)),
		Name:      "TestAgent",
		Level:     5,
		XP:        0,
		XPToLevel: xpEngine.XPToNextLevel(5),
		Tier:      TierApprentice,
		Status:    AgentIdle,
	}
	if err := storage.PutAgent(ctx, agentInstance, agent); err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	quest := Quest{
		ID:       QuestID(config.QuestEntityID("quest1")),
		Title:    "Streak Quest",
		BaseXP:   100,
		Attempts: 1,
	}

	pctx := ProgressionContext{
		Quest:   quest,
		AgentID: agent.ID,
		Verdict: BattleVerdict{
			Passed:       true,
			QualityScore: 0.8,
		},
		Duration: 5 * time.Minute,
	}

	// First success - streak 1
	result1, err := pm.ProcessSuccess(ctx, pctx)
	if err != nil {
		t.Fatalf("first ProcessSuccess failed: %v", err)
	}

	if result1.Streak != 1 {
		t.Errorf("expected streak 1, got %d", result1.Streak)
	}

	// Second success - streak 2
	result2, err := pm.ProcessSuccess(ctx, pctx)
	if err != nil {
		t.Fatalf("second ProcessSuccess failed: %v", err)
	}

	if result2.Streak != 2 {
		t.Errorf("expected streak 2, got %d", result2.Streak)
	}

	// Streak bonus should increase XP earned
	if result2.Award.StreakBonus <= 0 {
		t.Errorf("expected positive streak bonus, got %d", result2.Award.StreakBonus)
	}
}

func TestProgression_FailureResetsStreak(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "failstreak",
	}

	storage, err := CreateStorage(ctx, tc.Client, &config)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	events := NewEventPublisher(tc.Client)
	xpEngine := NewDefaultXPEngine()
	pm := NewProgressionManager(storage, xpEngine, events)

	// Create agent with existing streak
	agentInstance := GenerateInstance()
	agent := &Agent{
		ID:        AgentID(config.AgentEntityID(agentInstance)),
		Name:      "TestAgent",
		Level:     5,
		XP:        500,
		XPToLevel: xpEngine.XPToNextLevel(5),
		Tier:      TierApprentice,
		Status:    AgentIdle,
	}
	if err := storage.PutAgent(ctx, agentInstance, agent); err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Set up a streak
	if err := storage.SetAgentStreak(ctx, agentInstance, 5); err != nil {
		t.Fatalf("failed to set streak: %v", err)
	}

	quest := Quest{
		ID:       QuestID(config.QuestEntityID("quest1")),
		Title:    "Failed Quest",
		BaseXP:   100,
		Attempts: 1,
	}

	pctx := ProgressionContext{
		Quest:    quest,
		AgentID:  agent.ID,
		FailType: FailureSoft,
	}

	result, err := pm.ProcessFailure(ctx, pctx)
	if err != nil {
		t.Fatalf("ProcessFailure failed: %v", err)
	}

	if result.Streak != 0 {
		t.Errorf("expected streak to be reset to 0, got %d", result.Streak)
	}

	// Verify streak was reset in storage
	streak, err := storage.GetAgentStreak(ctx, agentInstance)
	if err != nil {
		t.Fatalf("failed to get streak: %v", err)
	}

	if streak != 0 {
		t.Errorf("expected persisted streak 0, got %d", streak)
	}
}

func TestProgression_Cooldown(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "cooldown",
	}

	storage, err := CreateStorage(ctx, tc.Client, &config)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	events := NewEventPublisher(tc.Client)
	xpEngine := NewDefaultXPEngine()
	pm := NewProgressionManager(storage, xpEngine, events)

	agentInstance := GenerateInstance()
	agent := &Agent{
		ID:        AgentID(config.AgentEntityID(agentInstance)),
		Name:      "TestAgent",
		Level:     5,
		XP:        500,
		XPToLevel: xpEngine.XPToNextLevel(5),
		Tier:      TierApprentice,
		Status:    AgentIdle,
	}
	if err := storage.PutAgent(ctx, agentInstance, agent); err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	quest := Quest{
		ID:       QuestID(config.QuestEntityID("quest1")),
		Title:    "Abandoned Quest",
		BaseXP:   100,
		Attempts: 1,
	}

	pctx := ProgressionContext{
		Quest:    quest,
		AgentID:  agent.ID,
		FailType: FailureAbandon, // Abandonment triggers cooldown
	}

	result, err := pm.ProcessFailure(ctx, pctx)
	if err != nil {
		t.Fatalf("ProcessFailure failed: %v", err)
	}

	if result.Penalty == nil {
		t.Fatal("expected penalty")
	}

	if result.Penalty.CooldownDur == 0 {
		t.Error("expected cooldown duration for abandonment")
	}

	// Verify agent status
	updated, err := storage.GetAgent(ctx, agentInstance)
	if err != nil {
		t.Fatalf("failed to get updated agent: %v", err)
	}

	if updated.Status != AgentCooldown {
		t.Errorf("expected agent status %s, got %s", AgentCooldown, updated.Status)
	}

	if updated.CooldownUntil == nil {
		t.Error("expected cooldown_until to be set")
	}
}

func TestProgression_GetAgentProgression(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx := context.Background()

	config := BoardConfig{
		Org:      "test",
		Platform: "unit",
		Board:    "getprog",
	}

	storage, err := CreateStorage(ctx, tc.Client, &config)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	events := NewEventPublisher(tc.Client)
	xpEngine := NewDefaultXPEngine()
	pm := NewProgressionManager(storage, xpEngine, events)

	agentInstance := GenerateInstance()
	agentID := AgentID(config.AgentEntityID(agentInstance))
	agent := &Agent{
		ID:        agentID,
		Name:      "TestAgent",
		Level:     10,
		XP:        250,
		XPToLevel: xpEngine.XPToNextLevel(10),
		Tier:      TierJourneyman,
		Status:    AgentIdle,
		Stats: AgentStats{
			QuestsCompleted: 15,
			BossesDefeated:  12,
		},
	}
	if err := storage.PutAgent(ctx, agentInstance, agent); err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Set a streak
	if err := storage.SetAgentStreak(ctx, agentInstance, 3); err != nil {
		t.Fatalf("failed to set streak: %v", err)
	}

	state, err := pm.GetAgentProgression(ctx, agentID)
	if err != nil {
		t.Fatalf("GetAgentProgression failed: %v", err)
	}

	if state.Level != 10 {
		t.Errorf("expected level 10, got %d", state.Level)
	}

	if state.XP != 250 {
		t.Errorf("expected XP 250, got %d", state.XP)
	}

	if state.Streak != 3 {
		t.Errorf("expected streak 3, got %d", state.Streak)
	}

	if state.Stats.QuestsCompleted != 15 {
		t.Errorf("expected QuestsCompleted 15, got %d", state.Stats.QuestsCompleted)
	}
}
