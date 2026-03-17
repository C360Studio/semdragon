package boidengine

import (
	"math"
	"testing"


	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
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
	agent1 := result[domain.AgentID("agent1")]
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
	agent2 := result[domain.AgentID("agent2")]
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
	agent1 := result[domain.AgentID("agent1")]
	agent2 := result[domain.AgentID("agent2")]

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
	agent1 := result[domain.AgentID("agent1")]

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
	agent1 := result[domain.AgentID("agent1")]

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
func baseAttractionFor(t *testing.T, agent agentprogression.Agent) QuestAttraction {
	t.Helper()

	engine := NewDefaultBoidEngine()
	rules := DefaultBoidRules()

	quest := domain.Quest{
		ID:             "q1",
		Status:         domain.QuestPosted,
		RequiredSkills: []domain.SkillTag{"code_gen"},
	}
	agents := []agentprogression.Agent{agent}
	quests := []domain.Quest{quest}

	attractions := engine.ComputeAttractions(agents, quests, rules)
	if len(attractions) == 0 {
		t.Fatal("expected at least one attraction, got none")
	}
	return attractions[0]
}

// agentWithSkill creates an Agent with a single code_gen skill proficiency entry.
// Agent.SkillProficiencies is the authoritative skill store used by HasSkill().
func agentWithSkill(id domain.AgentID, stats agentprogression.AgentStats) agentprogression.Agent {
	return agentprogression.Agent{
		ID:     id,
		Status: domain.AgentIdle,
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			"code_gen": {},
		},
		Stats: stats,
	}
}

func TestAffinityScore_PositivePeerReputation(t *testing.T) {
	// avg=5.0 → reputationMod = (5.0-3.0)/2.0 = 1.0 → multiplier = 1.0 + 1.0*0.3 = 1.3
	// base AffinityScore = 1.5 → boosted = 1.5 * 1.3 = 1.95
	agent := agentWithSkill("agent-pos", agentprogression.AgentStats{
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
	agent := agentWithSkill("agent-neg", agentprogression.AgentStats{
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
	agent := agentWithSkill("agent-neutral", agentprogression.AgentStats{
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

// =============================================================================
// CAUTION SCORE (OVERQUALIFICATION) UNIT TESTS
// =============================================================================

func TestCautionScore_OverqualifiedAgent(t *testing.T) {
	// Grandmaster (tier 4) vs apprentice quest (tier 0):
	// tierDiff = 0 - 4 = -4, overqualified = 3
	// CautionScore = -3 * 0.5 * 0.9 = -1.35
	engine := NewDefaultBoidEngine()
	rules := DefaultBoidRules()

	agent := agentprogression.Agent{
		ID:     "grandmaster",
		Status: domain.AgentIdle,
		Tier:   domain.TierGrandmaster, // tier 4
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			"code_gen": {},
		},
	}
	quest := domain.Quest{
		ID:             "trivial",
		Status:         domain.QuestPosted,
		MinTier:        domain.TierApprentice, // tier 0
		RequiredSkills: []domain.SkillTag{"code_gen"},
	}

	attractions := engine.ComputeAttractions([]agentprogression.Agent{agent}, []domain.Quest{quest}, rules)
	if len(attractions) == 0 {
		t.Fatal("expected attraction result")
	}

	attr := attractions[0]
	// overqualified = (4-0) - 1 = 3, penalty = -3 * 0.5 * 0.9 = -1.35
	const wantCaution = -1.35
	if math.Abs(attr.CautionScore-wantCaution) > 1e-9 {
		t.Errorf("CautionScore = %.6f, want %.6f (grandmaster should be penalized for trivial quest)", attr.CautionScore, wantCaution)
	}
}

func TestCautionScore_OneTierAbove(t *testing.T) {
	// Journeyman (tier 1) vs apprentice quest (tier 0):
	// tierDiff = 0 - 1 = -1, which is NOT < -1, so agent gets small bonus
	engine := NewDefaultBoidEngine()
	rules := DefaultBoidRules()

	agent := agentprogression.Agent{
		ID:     "journeyman",
		Status: domain.AgentIdle,
		Tier:   domain.TierJourneyman, // tier 1
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			"code_gen": {},
		},
	}
	quest := domain.Quest{
		ID:             "easy",
		Status:         domain.QuestPosted,
		MinTier:        domain.TierApprentice, // tier 0
		RequiredSkills: []domain.SkillTag{"code_gen"},
	}

	attractions := engine.ComputeAttractions([]agentprogression.Agent{agent}, []domain.Quest{quest}, rules)
	if len(attractions) == 0 {
		t.Fatal("expected attraction result")
	}

	// One tier above: small bonus (0.2 * 0.9 = 0.18)
	const wantCaution = 0.18
	if math.Abs(attractions[0].CautionScore-wantCaution) > 1e-9 {
		t.Errorf("CautionScore = %.6f, want %.6f (one tier above should get small bonus)", attractions[0].CautionScore, wantCaution)
	}
}

func TestCautionScore_SameTier(t *testing.T) {
	// Apprentice (tier 0) vs apprentice quest (tier 0):
	// tierDiff = 0, small bonus
	engine := NewDefaultBoidEngine()
	rules := DefaultBoidRules()

	agent := agentprogression.Agent{
		ID:     "apprentice",
		Status: domain.AgentIdle,
		Tier:   domain.TierApprentice,
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			"code_gen": {},
		},
	}
	quest := domain.Quest{
		ID:             "matching",
		Status:         domain.QuestPosted,
		MinTier:        domain.TierApprentice,
		RequiredSkills: []domain.SkillTag{"code_gen"},
	}

	attractions := engine.ComputeAttractions([]agentprogression.Agent{agent}, []domain.Quest{quest}, rules)
	if len(attractions) == 0 {
		t.Fatal("expected attraction result")
	}

	const wantCaution = 0.18 // 0.2 * 0.9
	if math.Abs(attractions[0].CautionScore-wantCaution) > 1e-9 {
		t.Errorf("CautionScore = %.6f, want %.6f (same tier should get small bonus)", attractions[0].CautionScore, wantCaution)
	}
}

func TestCautionScore_UnderLeveled(t *testing.T) {
	// Apprentice (tier 0) vs expert quest (tier 2):
	// tierDiff = 2 - 0 = 2, penalty = -2 * 0.9 = -1.8
	engine := NewDefaultBoidEngine()
	rules := DefaultBoidRules()

	agent := agentprogression.Agent{
		ID:     "apprentice",
		Status: domain.AgentIdle,
		Tier:   domain.TierApprentice,
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			"code_gen": {},
		},
	}
	quest := domain.Quest{
		ID:             "hard",
		Status:         domain.QuestPosted,
		MinTier:        domain.TierExpert, // tier 2
		RequiredSkills: []domain.SkillTag{"code_gen"},
	}

	attractions := engine.ComputeAttractions([]agentprogression.Agent{agent}, []domain.Quest{quest}, rules)
	if len(attractions) == 0 {
		t.Fatal("expected attraction result")
	}

	const wantCaution = -1.8 // -2 * 0.9
	if math.Abs(attractions[0].CautionScore-wantCaution) > 1e-9 {
		t.Errorf("CautionScore = %.6f, want %.6f (under-leveled should get strong penalty)", attractions[0].CautionScore, wantCaution)
	}
}

func TestAffinityScore_NoPeerReviews(t *testing.T) {
	// PeerReviewCount=0 → modifier branch is skipped entirely, no division by zero.
	// AffinityScore equals the unmodified base value.
	agent := agentWithSkill("agent-noreviews", agentprogression.AgentStats{
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

// =============================================================================
// CROSS-GUILD AFFINITY BONUS (RED-TEAM) UNIT TESTS
// =============================================================================
// These tests exercise the red-team cross-guild bonus branch inside
// computeAttraction. The setup uses a single-agent, single-quest pair so the
// AffinityScore arithmetic is deterministic and easy to verify.
//
// Cross-guild bonus rules (from boids.go):
//   - Different guild from GuildPriority → crossGuildBonus = skillMatch * 1.5
//   - No GuildPriority but agent is guilded → crossGuildBonus = skillMatch * 0.5
//   - Same guild as GuildPriority → crossGuildBonus = 0 (no bonus, no penalty)
//   - Not a red-team quest → crossGuildBonus = 0
//
// All agents below have 1 matching skill (code_gen), no peer reviews, and
// the quest has no GuildPriority overriding the affinity path unless noted.
// Base skill match (no guild, no cross-guild): skillMatch=1, guildMatch=0 → 1*1.5 = 1.5
// =============================================================================

// makeRedTeamAttraction is a helper that runs ComputeAttractions for a single
// agent against a red-team quest and returns the resulting QuestAttraction.
func makeRedTeamAttraction(t *testing.T, agent agentprogression.Agent, guildPriority *domain.GuildID) QuestAttraction {
	t.Helper()

	engine := NewDefaultBoidEngine()
	rules := DefaultBoidRules()

	quest := domain.Quest{
		ID:             "rt-quest",
		Status:         domain.QuestPosted,
		QuestType:      domain.QuestTypeRedTeam,
		RequiredSkills: []domain.SkillTag{"code_gen"},
		GuildPriority:  guildPriority,
	}
	agents := []agentprogression.Agent{agent}
	quests := []domain.Quest{quest}

	attractions := engine.ComputeAttractions(agents, quests, rules)
	if len(attractions) == 0 {
		t.Fatal("expected at least one attraction, got none")
	}
	return attractions[0]
}

func TestCrossGuildBonus_DifferentGuildGetBonus(t *testing.T) {
	// Agent is in "redguild"; quest GuildPriority is "blueguild".
	// crossGuildBonus = skillMatch(1) * 1.5 = 1.5
	// AffinityScore = (1 + 0 + 1.5) * 1.5 = 3.75
	blueGuild := domain.GuildID("blueguild")
	agent := agentprogression.Agent{
		ID:     "agent-red",
		Status: domain.AgentIdle,
		Guild:  "redguild",
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			"code_gen": {},
		},
	}

	attr := makeRedTeamAttraction(t, agent, &blueGuild)

	const want = 3.75
	if math.Abs(attr.AffinityScore-want) > 1e-9 {
		t.Errorf("AffinityScore = %.6f, want %.6f (cross-guild 1.5x bonus should apply)", attr.AffinityScore, want)
	}
}

func TestCrossGuildBonus_NoGuildPriorityModerateBonus(t *testing.T) {
	// No GuildPriority on quest, agent is guilded.
	// crossGuildBonus = skillMatch(1) * 0.5 = 0.5
	// AffinityScore = (1 + 0 + 0.5) * 1.5 = 2.25
	agent := agentprogression.Agent{
		ID:     "agent-any-guild",
		Status: domain.AgentIdle,
		Guild:  "someguild",
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			"code_gen": {},
		},
	}

	attr := makeRedTeamAttraction(t, agent, nil)

	const want = 2.25
	if math.Abs(attr.AffinityScore-want) > 1e-9 {
		t.Errorf("AffinityScore = %.6f, want %.6f (moderate 0.5x bonus for guilded, no priority)", attr.AffinityScore, want)
	}
}

func TestCrossGuildBonus_NormalQuestNoBonus(t *testing.T) {
	// Normal (non-red-team) quest: cross-guild bonus is always zero.
	// Base affinity: skillMatch=1, guildMatch=0, crossGuildBonus=0 → 1.0 * 1.5 = 1.5
	engine := NewDefaultBoidEngine()
	rules := DefaultBoidRules()

	blueGuild := domain.GuildID("blueguild")
	agent := agentprogression.Agent{
		ID:     "agent-normal",
		Status: domain.AgentIdle,
		Guild:  "redguild",
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			"code_gen": {},
		},
	}
	quest := domain.Quest{
		ID:             "normal-quest",
		Status:         domain.QuestPosted,
		QuestType:      domain.QuestTypeNormal,
		RequiredSkills: []domain.SkillTag{"code_gen"},
		GuildPriority:  &blueGuild,
	}

	attractions := engine.ComputeAttractions([]agentprogression.Agent{agent}, []domain.Quest{quest}, rules)
	if len(attractions) == 0 {
		t.Fatal("expected at least one attraction, got none")
	}

	// No cross-guild bonus for normal quest; guildMatch=0 (priority is blueGuild, agent is in redguild)
	const want = 1.5
	if math.Abs(attractions[0].AffinityScore-want) > 1e-9 {
		t.Errorf("AffinityScore = %.6f, want %.6f (non-red-team quest must not apply cross-guild bonus)", attractions[0].AffinityScore, want)
	}
}

func TestCrossGuildBonus_SameGuildNoBonus(t *testing.T) {
	// Agent is in the SAME guild as the blue-team GuildPriority.
	// guildMatch applies (agent.Guild == *quest.GuildPriority), so no crossGuildBonus.
	// guildMatch = 1.0 (base, no guild context for rank/reputation boost in this test)
	// AffinityScore = (1 + 1 + 0) * 1.5 = 3.0
	blueGuild := domain.GuildID("blueguild")
	agent := agentprogression.Agent{
		ID:     "agent-blue",
		Status: domain.AgentIdle,
		Guild:  "blueguild",
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			"code_gen": {},
		},
	}

	attr := makeRedTeamAttraction(t, agent, &blueGuild)

	// Same guild → guildMatch = 1.0 (no engine.guilds context → no rank/rep multiplier)
	// crossGuildBonus = 0 (same guild)
	// AffinityScore = (1 + 1 + 0) * 1.5 = 3.0
	const want = 3.0
	if math.Abs(attr.AffinityScore-want) > 1e-9 {
		t.Errorf("AffinityScore = %.6f, want %.6f (same guild should get standard guild match, not cross-guild bonus)", attr.AffinityScore, want)
	}
}

func TestCrossGuildBonus_UnguildedAgentNoBonus(t *testing.T) {
	// Unguilded agent on a red-team quest with GuildPriority set.
	// agent.Guild == "" → the else-if branch (agent.Guild != "") is false → crossGuildBonus = 0
	// guildMatch = 0 (no priority match)
	// AffinityScore = (1 + 0 + 0) * 1.5 = 1.5
	blueGuild := domain.GuildID("blueguild")
	agent := agentprogression.Agent{
		ID:     "agent-unguilded",
		Status: domain.AgentIdle,
		Guild:  "", // no guild
		SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
			"code_gen": {},
		},
	}

	attr := makeRedTeamAttraction(t, agent, &blueGuild)

	const want = 1.5
	if math.Abs(attr.AffinityScore-want) > 1e-9 {
		t.Errorf("AffinityScore = %.6f, want %.6f (unguilded agent should get no cross-guild bonus)", attr.AffinityScore, want)
	}
}
