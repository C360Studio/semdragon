package boidengine

import (
	"testing"

	semdragons "github.com/c360studio/semdragons"
)

// =============================================================================
// SuggestTopN UNIT TESTS
// =============================================================================

func TestSuggestTopN_ReturnsRankedList(t *testing.T) {
	engine := NewDefaultBoidEngine()

	attractions := []QuestAttraction{
		{AgentID: "agent1", QuestID: "quest1", TotalScore: 5.0},
		{AgentID: "agent1", QuestID: "quest2", TotalScore: 3.0},
		{AgentID: "agent1", QuestID: "quest3", TotalScore: 1.0},
		{AgentID: "agent2", QuestID: "quest1", TotalScore: 4.0},
		{AgentID: "agent2", QuestID: "quest2", TotalScore: 2.0},
	}

	result := engine.SuggestTopN(attractions, 3)
	if result == nil {
		t.Fatal("SuggestTopN returned nil")
	}

	// Agent1 should have 3 suggestions
	agent1 := result[semdragons.AgentID("agent1")]
	if len(agent1) != 3 {
		t.Fatalf("agent1 got %d suggestions, want 3", len(agent1))
	}

	// Verify sorted by score descending
	if agent1[0].Score != 5.0 {
		t.Errorf("agent1[0].Score = %f, want 5.0", agent1[0].Score)
	}
	if agent1[1].Score != 3.0 {
		t.Errorf("agent1[1].Score = %f, want 3.0", agent1[1].Score)
	}
	if agent1[2].Score != 1.0 {
		t.Errorf("agent1[2].Score = %f, want 1.0", agent1[2].Score)
	}

	// Agent2 should have 2 suggestions (only 2 quests available)
	agent2 := result[semdragons.AgentID("agent2")]
	if len(agent2) != 2 {
		t.Fatalf("agent2 got %d suggestions, want 2", len(agent2))
	}
	if agent2[0].Score != 4.0 {
		t.Errorf("agent2[0].Score = %f, want 4.0", agent2[0].Score)
	}
}

func TestSuggestTopN_MultipleAgentsSameQuest(t *testing.T) {
	engine := NewDefaultBoidEngine()

	// Both agents rank quest1 highest
	attractions := []QuestAttraction{
		{AgentID: "agent1", QuestID: "quest1", TotalScore: 5.0},
		{AgentID: "agent2", QuestID: "quest1", TotalScore: 4.0},
	}

	result := engine.SuggestTopN(attractions, 3)

	// Both agents should have quest1 in their suggestions
	agent1 := result[semdragons.AgentID("agent1")]
	agent2 := result[semdragons.AgentID("agent2")]

	if len(agent1) == 0 || agent1[0].QuestID != "quest1" {
		t.Error("agent1 should have quest1 as top suggestion")
	}
	if len(agent2) == 0 || agent2[0].QuestID != "quest1" {
		t.Error("agent2 should have quest1 as top suggestion (not removed from pool)")
	}
}

func TestSuggestTopN_ConfidenceCalculation(t *testing.T) {
	engine := NewDefaultBoidEngine()

	attractions := []QuestAttraction{
		{AgentID: "agent1", QuestID: "quest1", TotalScore: 10.0},
		{AgentID: "agent1", QuestID: "quest2", TotalScore: 5.0},
	}

	result := engine.SuggestTopN(attractions, 3)
	agent1 := result[semdragons.AgentID("agent1")]

	if len(agent1) != 2 {
		t.Fatalf("got %d suggestions, want 2", len(agent1))
	}

	// Confidence for rank 1: (10 - 5) / 10 = 0.5
	expectedConfidence := 0.5
	if agent1[0].Confidence != expectedConfidence {
		t.Errorf("rank 1 confidence = %f, want %f", agent1[0].Confidence, expectedConfidence)
	}

	// Rank 2 confidence should be lower
	if agent1[1].Confidence >= agent1[0].Confidence {
		t.Errorf("rank 2 confidence (%f) should be less than rank 1 (%f)",
			agent1[1].Confidence, agent1[0].Confidence)
	}
}

func TestSuggestTopN_EmptyAttractions(t *testing.T) {
	engine := NewDefaultBoidEngine()
	result := engine.SuggestTopN(nil, 3)
	if result != nil {
		t.Errorf("expected nil for empty attractions, got %v", result)
	}
}

func TestSuggestTopN_ZeroN(t *testing.T) {
	engine := NewDefaultBoidEngine()
	attractions := []QuestAttraction{
		{AgentID: "agent1", QuestID: "quest1", TotalScore: 5.0},
	}
	result := engine.SuggestTopN(attractions, 0)
	if result != nil {
		t.Errorf("expected nil for n=0, got %v", result)
	}
}

func TestSuggestTopN_LimitToN(t *testing.T) {
	engine := NewDefaultBoidEngine()

	attractions := []QuestAttraction{
		{AgentID: "agent1", QuestID: "quest1", TotalScore: 5.0},
		{AgentID: "agent1", QuestID: "quest2", TotalScore: 4.0},
		{AgentID: "agent1", QuestID: "quest3", TotalScore: 3.0},
		{AgentID: "agent1", QuestID: "quest4", TotalScore: 2.0},
		{AgentID: "agent1", QuestID: "quest5", TotalScore: 1.0},
	}

	result := engine.SuggestTopN(attractions, 2)
	agent1 := result[semdragons.AgentID("agent1")]

	if len(agent1) != 2 {
		t.Fatalf("got %d suggestions, want 2 (limited by n)", len(agent1))
	}
	if agent1[0].QuestID != "quest1" {
		t.Errorf("rank 1 quest = %s, want quest1", agent1[0].QuestID)
	}
	if agent1[1].QuestID != "quest2" {
		t.Errorf("rank 2 quest = %s, want quest2", agent1[1].QuestID)
	}
}
