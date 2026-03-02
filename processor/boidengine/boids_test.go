package boidengine

import (
	"math"
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

// =============================================================================
// PEER REPUTATION MODIFIER UNIT TESTS
// =============================================================================
// These tests call computeAttraction directly via ComputeAttractions, using a
// single-agent, single-quest scenario where the skill/guild match produces a
// known base AffinityScore that we can inspect through TotalScore arithmetic.
// =============================================================================

// baseAttractionFor is a helper that builds the minimal agent/quest pair needed
// to produce a non-zero attraction and returns the computed QuestAttraction.
// The agent has one matching skill (code_gen); no guild priority on the quest so
// guildMatch=0. Expected base AffinityScore = 1.0 (one skill match) * AffinityWeight
// (1.5) = 1.5.
func baseAttractionFor(t *testing.T, agent semdragons.Agent) QuestAttraction {
	t.Helper()

	engine := NewDefaultBoidEngine()
	rules := DefaultBoidRules()

	quest := semdragons.Quest{
		ID:             "q1",
		Status:         semdragons.QuestPosted,
		RequiredSkills: []semdragons.SkillTag{"code_gen"},
	}
	agents := []semdragons.Agent{agent}
	quests := []semdragons.Quest{quest}

	attractions := engine.ComputeAttractions(agents, quests, rules)
	if len(attractions) == 0 {
		t.Fatal("expected at least one attraction, got none")
	}
	return attractions[0]
}

// agentWithSkill creates an Agent with a single code_gen skill proficiency entry.
// Agent.SkillProficiencies is the authoritative skill store used by HasSkill().
func agentWithSkill(id semdragons.AgentID, stats semdragons.AgentStats) semdragons.Agent {
	return semdragons.Agent{
		ID:     id,
		Status: semdragons.AgentIdle,
		SkillProficiencies: map[semdragons.SkillTag]semdragons.SkillProficiency{
			"code_gen": {},
		},
		Stats: stats,
	}
}

func TestAffinityScore_PositivePeerReputation(t *testing.T) {
	// avg=5.0 → reputationMod = (5.0-3.0)/2.0 = 1.0 → multiplier = 1.0 + 1.0*0.3 = 1.3
	// base AffinityScore = 1.5 → boosted = 1.5 * 1.3 = 1.95
	agent := agentWithSkill("agent-pos", semdragons.AgentStats{
		PeerReviewAvg:   5.0,
		PeerReviewCount: 3,
	})

	attr := baseAttractionFor(t, agent)

	const want = 1.95
	if math.Abs(attr.AffinityScore-want) > 1e-9 {
		t.Errorf("AffinityScore = %.6f, want %.6f (±30%% boost for avg=5.0)", attr.AffinityScore, want)
	}
}

func TestAffinityScore_NegativePeerReputation(t *testing.T) {
	// avg=1.0 → reputationMod = (1.0-3.0)/2.0 = -1.0 → multiplier = 1.0 + (-1.0)*0.3 = 0.7
	// base AffinityScore = 1.5 → reduced = 1.5 * 0.7 = 1.05
	agent := agentWithSkill("agent-neg", semdragons.AgentStats{
		PeerReviewAvg:   1.0,
		PeerReviewCount: 2,
	})

	attr := baseAttractionFor(t, agent)

	const want = 1.05
	if math.Abs(attr.AffinityScore-want) > 1e-9 {
		t.Errorf("AffinityScore = %.6f, want %.6f (±30%% reduction for avg=1.0)", attr.AffinityScore, want)
	}
}

func TestAffinityScore_NeutralPeerReputation(t *testing.T) {
	// avg=3.0 → reputationMod = (3.0-3.0)/2.0 = 0.0 → multiplier = 1.0
	// AffinityScore is unchanged from its base value.
	agent := agentWithSkill("agent-neutral", semdragons.AgentStats{
		PeerReviewAvg:   3.0,
		PeerReviewCount: 5,
	})

	attr := baseAttractionFor(t, agent)

	// Base affinity: 1 skill match * 1.5 weight = 1.5 — no change
	const want = 1.5
	if math.Abs(attr.AffinityScore-want) > 1e-9 {
		t.Errorf("AffinityScore = %.6f, want %.6f (neutral avg=3.0 must leave score unchanged)", attr.AffinityScore, want)
	}
}

func TestAffinityScore_NoPeerReviews(t *testing.T) {
	// PeerReviewCount=0 → modifier branch is skipped entirely, no division by zero.
	// AffinityScore equals the unmodified base value.
	agent := agentWithSkill("agent-noreviews", semdragons.AgentStats{
		PeerReviewAvg:   0.0, // zero-value; irrelevant when Count==0
		PeerReviewCount: 0,
	})

	attr := baseAttractionFor(t, agent)

	// Base affinity: 1 skill match * 1.5 weight = 1.5 — unmodified
	const want = 1.5
	if math.Abs(attr.AffinityScore-want) > 1e-9 {
		t.Errorf("AffinityScore = %.6f, want %.6f (count=0 must leave score unchanged)", attr.AffinityScore, want)
	}
}
