package semdragons

import (
	"testing"
	"time"
)

// =============================================================================
// TEST HELPERS
// =============================================================================

func newTestAgent(id string, level int, skills ...SkillTag) Agent {
	return Agent{
		ID:     AgentID(id),
		Name:   id,
		Level:  level,
		Tier:   TierFromLevel(level),
		Status: AgentIdle,
		Skills: skills,
	}
}

func newTestQuest(id string, difficulty QuestDifficulty, skills ...SkillTag) Quest {
	return Quest{
		ID:             QuestID(id),
		Title:          id,
		Status:         QuestPosted,
		Difficulty:     difficulty,
		MinTier:        tierForDifficulty(difficulty),
		RequiredSkills: skills,
	}
}

func tierForDifficulty(d QuestDifficulty) TrustTier {
	switch d {
	case DifficultyTrivial:
		return TierApprentice
	case DifficultyEasy, DifficultyModerate:
		return TierJourneyman
	case DifficultyHard:
		return TierExpert
	case DifficultyEpic:
		return TierMaster
	case DifficultyLegendary:
		return TierGrandmaster
	default:
		return TierApprentice
	}
}

func ptrAgentID(id AgentID) *AgentID {
	return &id
}

// =============================================================================
// RULE: SEPARATION
// =============================================================================

func TestRuleSeparation_AvailableQuest(t *testing.T) {
	engine := NewDefaultBoidEngine()
	agent := newTestAgent("agent1", 10)
	quest := newTestQuest("quest1", DifficultyModerate)
	// Quest not claimed

	score := engine.ruleSeparation(agent, quest)
	if score != 0 {
		t.Errorf("expected 0 for available quest, got %f", score)
	}
}

func TestRuleSeparation_ClaimedByOther(t *testing.T) {
	engine := NewDefaultBoidEngine()
	agent := newTestAgent("agent1", 10)
	quest := newTestQuest("quest1", DifficultyModerate)
	otherAgent := AgentID("agent2")
	quest.ClaimedBy = &otherAgent

	score := engine.ruleSeparation(agent, quest)
	if score != -1 {
		t.Errorf("expected -1 for quest claimed by other, got %f", score)
	}
}

func TestRuleSeparation_ClaimedBySelf(t *testing.T) {
	engine := NewDefaultBoidEngine()
	agent := newTestAgent("agent1", 10)
	quest := newTestQuest("quest1", DifficultyModerate)
	quest.ClaimedBy = ptrAgentID(agent.ID)

	score := engine.ruleSeparation(agent, quest)
	if score != 0 {
		t.Errorf("expected 0 for quest claimed by self, got %f", score)
	}
}

// =============================================================================
// RULE: AFFINITY
// =============================================================================

func TestRuleAffinity_PerfectMatch(t *testing.T) {
	engine := NewDefaultBoidEngine()
	agent := newTestAgent("agent1", 10, SkillAnalysis, SkillCodeGen)
	quest := newTestQuest("quest1", DifficultyModerate, SkillAnalysis, SkillCodeGen)

	score := engine.ruleAffinity(agent, quest)
	if score != 1.0 {
		t.Errorf("expected 1.0 for perfect match, got %f", score)
	}
}

func TestRuleAffinity_PartialMatch(t *testing.T) {
	engine := NewDefaultBoidEngine()
	agent := newTestAgent("agent1", 10, SkillAnalysis)
	quest := newTestQuest("quest1", DifficultyModerate, SkillAnalysis, SkillCodeGen)

	score := engine.ruleAffinity(agent, quest)
	if score != 0.5 {
		t.Errorf("expected 0.5 for 1/2 match, got %f", score)
	}
}

func TestRuleAffinity_NoMatch(t *testing.T) {
	engine := NewDefaultBoidEngine()
	agent := newTestAgent("agent1", 10, SkillResearch)
	quest := newTestQuest("quest1", DifficultyModerate, SkillAnalysis, SkillCodeGen)

	score := engine.ruleAffinity(agent, quest)
	if score != 0 {
		t.Errorf("expected 0 for no match, got %f", score)
	}
}

func TestRuleAffinity_NoSkillsRequired(t *testing.T) {
	engine := NewDefaultBoidEngine()
	agent := newTestAgent("agent1", 10, SkillAnalysis)
	quest := newTestQuest("quest1", DifficultyModerate) // No required skills

	score := engine.ruleAffinity(agent, quest)
	if score != 1.0 {
		t.Errorf("expected 1.0 when no skills required, got %f", score)
	}
}

// =============================================================================
// RULE: CAUTION
// =============================================================================

func TestRuleCaution_MeetsTier(t *testing.T) {
	engine := NewDefaultBoidEngine()
	agent := newTestAgent("agent1", 12, SkillAnalysis) // Expert tier
	quest := newTestQuest("quest1", DifficultyHard)    // Requires Expert

	score := engine.ruleCaution(agent, quest)
	if score != 0 {
		t.Errorf("expected 0 when agent meets tier, got %f", score)
	}
}

func TestRuleCaution_ExceedsTier(t *testing.T) {
	engine := NewDefaultBoidEngine()
	agent := newTestAgent("agent1", 17, SkillAnalysis)   // Master tier
	quest := newTestQuest("quest1", DifficultyModerate) // Requires Journeyman

	score := engine.ruleCaution(agent, quest)
	if score != 0 {
		t.Errorf("expected 0 when agent exceeds tier, got %f", score)
	}
}

func TestRuleCaution_OneTierBelow(t *testing.T) {
	engine := NewDefaultBoidEngine()
	agent := newTestAgent("agent1", 8, SkillAnalysis) // Journeyman
	quest := newTestQuest("quest1", DifficultyHard)   // Requires Expert

	score := engine.ruleCaution(agent, quest)
	expected := -0.33
	if score < expected-0.01 || score > expected+0.01 {
		t.Errorf("expected ~%f for 1 tier below, got %f", expected, score)
	}
}

func TestRuleCaution_TwoTiersBelow(t *testing.T) {
	engine := NewDefaultBoidEngine()
	agent := newTestAgent("agent1", 3, SkillAnalysis) // Apprentice
	quest := newTestQuest("quest1", DifficultyHard)   // Requires Expert

	score := engine.ruleCaution(agent, quest)
	expected := -0.66
	if score < expected-0.01 || score > expected+0.01 {
		t.Errorf("expected ~%f for 2 tiers below, got %f", expected, score)
	}
}

func TestRuleCaution_CappedAtMinusOne(t *testing.T) {
	engine := NewDefaultBoidEngine()
	agent := newTestAgent("agent1", 1, SkillAnalysis)      // Apprentice
	quest := newTestQuest("quest1", DifficultyLegendary) // Requires Grandmaster

	score := engine.ruleCaution(agent, quest)
	if score != -1.0 {
		t.Errorf("expected -1.0 (capped), got %f", score)
	}
}

// =============================================================================
// RULE: HUNGER
// =============================================================================

func TestRuleHunger_JustIdle(t *testing.T) {
	engine := NewDefaultBoidEngine()
	agent := newTestAgent("agent1", 10)
	now := time.Now()
	idleInfo := &AgentIdleInfo{IdleSince: now.Add(-1 * time.Second)}

	score := engine.ruleHunger(agent, idleInfo, now)
	if score > 0.02 {
		t.Errorf("expected near 0 for just idle, got %f", score)
	}
}

func TestRuleHunger_Idle30Min(t *testing.T) {
	engine := NewDefaultBoidEngine()
	agent := newTestAgent("agent1", 10)
	now := time.Now()
	idleInfo := &AgentIdleInfo{IdleSince: now.Add(-30 * time.Minute)}

	score := engine.ruleHunger(agent, idleInfo, now)
	if score < 0.49 || score > 0.51 {
		t.Errorf("expected ~0.5 for 30 min idle, got %f", score)
	}
}

func TestRuleHunger_Idle2Hours(t *testing.T) {
	engine := NewDefaultBoidEngine()
	agent := newTestAgent("agent1", 10)
	now := time.Now()
	idleInfo := &AgentIdleInfo{IdleSince: now.Add(-2 * time.Hour)}

	score := engine.ruleHunger(agent, idleInfo, now)
	if score != 1.0 {
		t.Errorf("expected 1.0 (capped) for 2 hours idle, got %f", score)
	}
}

func TestRuleHunger_NoIdleInfo(t *testing.T) {
	engine := NewDefaultBoidEngine()
	agent := newTestAgent("agent1", 10)
	now := time.Now()

	score := engine.ruleHunger(agent, nil, now)
	if score != 0 {
		t.Errorf("expected 0 when no idle info, got %f", score)
	}
}

func TestRuleHunger_FutureIdleSince(t *testing.T) {
	engine := NewDefaultBoidEngine()
	agent := newTestAgent("agent1", 10)
	now := time.Now()
	idleInfo := &AgentIdleInfo{IdleSince: now.Add(1 * time.Hour)} // Future!

	score := engine.ruleHunger(agent, idleInfo, now)
	if score != 0 {
		t.Errorf("expected 0 for future idle time, got %f", score)
	}
}

// =============================================================================
// RULE: COHESION
// =============================================================================

func TestRuleCohesion_GuildMatch(t *testing.T) {
	engine := NewDefaultBoidEngine()
	guildID := GuildID("data-guild")
	agent := newTestAgent("agent1", 10)
	agent.Guilds = []GuildID{guildID}
	quest := newTestQuest("quest1", DifficultyModerate)
	quest.GuildPriority = &guildID

	score := engine.ruleCohesion(agent, quest)
	if score != 1.0 {
		t.Errorf("expected 1.0 for guild match, got %f", score)
	}
}

func TestRuleCohesion_GuildMismatch(t *testing.T) {
	engine := NewDefaultBoidEngine()
	agent := newTestAgent("agent1", 10)
	agent.Guilds = []GuildID{"other-guild"}
	priorityGuild := GuildID("data-guild")
	quest := newTestQuest("quest1", DifficultyModerate)
	quest.GuildPriority = &priorityGuild

	score := engine.ruleCohesion(agent, quest)
	if score != 0.2 {
		t.Errorf("expected 0.2 for guild mismatch, got %f", score)
	}
}

func TestRuleCohesion_NoGuildPriority(t *testing.T) {
	engine := NewDefaultBoidEngine()
	agent := newTestAgent("agent1", 10)
	agent.Guilds = []GuildID{"any-guild"}
	quest := newTestQuest("quest1", DifficultyModerate)
	// No GuildPriority set

	score := engine.ruleCohesion(agent, quest)
	if score != 0.5 {
		t.Errorf("expected 0.5 for no guild priority, got %f", score)
	}
}

func TestRuleCohesion_AgentNoGuild_QuestHasPriority(t *testing.T) {
	engine := NewDefaultBoidEngine()
	agent := newTestAgent("agent1", 10)
	// Agent has no guilds
	priorityGuild := GuildID("data-guild")
	quest := newTestQuest("quest1", DifficultyModerate)
	quest.GuildPriority = &priorityGuild

	score := engine.ruleCohesion(agent, quest)
	if score != 0.2 {
		t.Errorf("expected 0.2 when agent not in guild, got %f", score)
	}
}

// =============================================================================
// RULE: ALIGNMENT
// =============================================================================

func TestRuleAlignment_NoNeighbors(t *testing.T) {
	engine := NewDefaultBoidEngine()
	agent := newTestAgent("agent1", 10, SkillAnalysis)
	quest := newTestQuest("quest1", DifficultyModerate, SkillAnalysis)
	// Only agent in the system
	allAgents := []Agent{agent}
	allQuests := []Quest{quest}

	score := engine.ruleAlignment(agent, quest, allAgents, allQuests, 5)
	if score != 0 {
		t.Errorf("expected 0 for no neighbors, got %f", score)
	}
}

func TestRuleAlignment_NeighborsOnRelatedQuests(t *testing.T) {
	engine := NewDefaultBoidEngine()
	neighborRadius := 5

	// Agent with Analysis skill
	agent := newTestAgent("agent1", 10, SkillAnalysis)

	// Two neighbors with overlapping skills, on related quests
	neighbor1 := newTestAgent("neighbor1", 10, SkillAnalysis, SkillCodeGen)
	neighbor1.Status = AgentOnQuest
	neighbor2 := newTestAgent("neighbor2", 10, SkillAnalysis)
	neighbor2.Status = AgentOnQuest

	// Quest that agent is considering
	targetQuest := newTestQuest("target", DifficultyModerate, SkillAnalysis)

	// Related quest that neighbor1 is on
	relatedQuest1 := newTestQuest("related1", DifficultyModerate, SkillAnalysis)
	relatedQuest1.Status = QuestInProgress
	relatedQuest1.ClaimedBy = ptrAgentID(neighbor1.ID)

	// Related quest that neighbor2 is on
	relatedQuest2 := newTestQuest("related2", DifficultyModerate, SkillAnalysis)
	relatedQuest2.Status = QuestInProgress
	relatedQuest2.ClaimedBy = ptrAgentID(neighbor2.ID)

	allAgents := []Agent{agent, neighbor1, neighbor2}
	allQuests := []Quest{targetQuest, relatedQuest1, relatedQuest2}

	score := engine.ruleAlignment(agent, targetQuest, allAgents, allQuests, neighborRadius)
	// 2 neighbors on related quests, radius 5 = 2/5 = 0.4
	if score < 0.39 || score > 0.41 {
		t.Errorf("expected ~0.4 for 2 neighbors on related quests, got %f", score)
	}
}

func TestRuleAlignment_NeighborsOnUnrelatedQuests(t *testing.T) {
	engine := NewDefaultBoidEngine()
	neighborRadius := 5

	agent := newTestAgent("agent1", 10, SkillAnalysis)
	neighbor := newTestAgent("neighbor1", 10, SkillAnalysis)
	neighbor.Status = AgentOnQuest

	// Quest agent is considering (Analysis skill)
	targetQuest := newTestQuest("target", DifficultyModerate, SkillAnalysis)

	// Neighbor is on an unrelated quest (different skills)
	unrelatedQuest := newTestQuest("unrelated", DifficultyModerate, SkillResearch)
	unrelatedQuest.Status = QuestInProgress
	unrelatedQuest.ClaimedBy = ptrAgentID(neighbor.ID)

	allAgents := []Agent{agent, neighbor}
	allQuests := []Quest{targetQuest, unrelatedQuest}

	score := engine.ruleAlignment(agent, targetQuest, allAgents, allQuests, neighborRadius)
	if score != 0 {
		t.Errorf("expected 0 for neighbors on unrelated quests, got %f", score)
	}
}

func TestRuleAlignment_CappedAtOne(t *testing.T) {
	engine := NewDefaultBoidEngine()
	neighborRadius := 2 // Small radius

	agent := newTestAgent("agent1", 10, SkillAnalysis)

	// 5 neighbors on related quests
	var allAgents []Agent
	var allQuests []Quest
	allAgents = append(allAgents, agent)

	targetQuest := newTestQuest("target", DifficultyModerate, SkillAnalysis)
	allQuests = append(allQuests, targetQuest)

	for i := range 5 {
		neighbor := newTestAgent("neighbor"+string(rune('1'+i)), 10, SkillAnalysis)
		neighbor.Status = AgentOnQuest
		allAgents = append(allAgents, neighbor)

		relatedQuest := newTestQuest("related"+string(rune('1'+i)), DifficultyModerate, SkillAnalysis)
		relatedQuest.Status = QuestInProgress
		relatedQuest.ClaimedBy = ptrAgentID(neighbor.ID)
		allQuests = append(allQuests, relatedQuest)
	}

	score := engine.ruleAlignment(agent, targetQuest, allAgents, allQuests, neighborRadius)
	if score != 1.0 {
		t.Errorf("expected 1.0 (capped), got %f", score)
	}
}

// =============================================================================
// HELPER: findNeighbors
// =============================================================================

func TestFindNeighbors_SharedSkill(t *testing.T) {
	engine := NewDefaultBoidEngine()

	agent := newTestAgent("agent1", 10, SkillAnalysis)
	neighbor := newTestAgent("neighbor1", 10, SkillAnalysis, SkillCodeGen)
	stranger := newTestAgent("stranger1", 10, SkillResearch) // No overlap

	allAgents := []Agent{agent, neighbor, stranger}
	neighbors := engine.findNeighbors(agent, allAgents)

	if len(neighbors) != 1 {
		t.Fatalf("expected 1 neighbor, got %d", len(neighbors))
	}
	if neighbors[0].ID != neighbor.ID {
		t.Errorf("expected neighbor1, got %s", neighbors[0].ID)
	}
}

func TestFindNeighbors_NoSkills(t *testing.T) {
	engine := NewDefaultBoidEngine()

	agent := newTestAgent("agent1", 10) // No skills
	neighbor := newTestAgent("neighbor1", 10, SkillAnalysis)

	allAgents := []Agent{agent, neighbor}
	neighbors := engine.findNeighbors(agent, allAgents)

	if len(neighbors) != 0 {
		t.Errorf("expected 0 neighbors when agent has no skills, got %d", len(neighbors))
	}
}

func TestFindNeighbors_ExcludesSelf(t *testing.T) {
	engine := NewDefaultBoidEngine()

	agent := newTestAgent("agent1", 10, SkillAnalysis)
	allAgents := []Agent{agent}

	neighbors := engine.findNeighbors(agent, allAgents)
	if len(neighbors) != 0 {
		t.Errorf("expected 0 neighbors (self excluded), got %d", len(neighbors))
	}
}

// =============================================================================
// COMPUTE ATTRACTIONS
// =============================================================================

func TestComputeAttractions_RanksByTotalScore(t *testing.T) {
	engine := NewDefaultBoidEngine()

	// Agent with perfect skill match for quest1, no match for quest2
	agent := newTestAgent("agent1", 10, SkillAnalysis)
	agent.Status = AgentIdle

	quest1 := newTestQuest("quest1", DifficultyModerate, SkillAnalysis)
	quest2 := newTestQuest("quest2", DifficultyModerate, SkillResearch)

	agents := []Agent{agent}
	quests := []Quest{quest1, quest2}

	attractions := engine.ComputeAttractions(agents, quests, DefaultBoidRules())

	if len(attractions) != 2 {
		t.Fatalf("expected 2 attractions, got %d", len(attractions))
	}

	// First should be quest1 (better affinity match)
	if attractions[0].QuestID != quest1.ID {
		t.Errorf("expected quest1 ranked first due to skill match, got %s", attractions[0].QuestID)
	}
	if attractions[0].TotalScore <= attractions[1].TotalScore {
		t.Error("expected first attraction to have higher score")
	}
}

func TestComputeAttractions_ExcludesNonIdleAgents(t *testing.T) {
	engine := NewDefaultBoidEngine()

	idleAgent := newTestAgent("idle", 10, SkillAnalysis)
	idleAgent.Status = AgentIdle

	busyAgent := newTestAgent("busy", 10, SkillAnalysis)
	busyAgent.Status = AgentOnQuest

	quest := newTestQuest("quest1", DifficultyModerate, SkillAnalysis)

	agents := []Agent{idleAgent, busyAgent}
	quests := []Quest{quest}

	attractions := engine.ComputeAttractions(agents, quests, DefaultBoidRules())

	// Only idle agent should have attractions
	for _, a := range attractions {
		if a.AgentID == busyAgent.ID {
			t.Error("busy agent should not have attractions")
		}
	}
	if len(attractions) != 1 {
		t.Errorf("expected 1 attraction (idle agent only), got %d", len(attractions))
	}
}

func TestComputeAttractions_ExcludesClaimedQuests(t *testing.T) {
	engine := NewDefaultBoidEngine()

	agent := newTestAgent("agent1", 10, SkillAnalysis)
	agent.Status = AgentIdle

	availableQuest := newTestQuest("available", DifficultyModerate, SkillAnalysis)

	claimedQuest := newTestQuest("claimed", DifficultyModerate, SkillAnalysis)
	claimedQuest.Status = QuestClaimed
	otherAgent := AgentID("other")
	claimedQuest.ClaimedBy = &otherAgent

	agents := []Agent{agent}
	quests := []Quest{availableQuest, claimedQuest}

	attractions := engine.ComputeAttractions(agents, quests, DefaultBoidRules())

	// Only available quest should have attractions
	for _, a := range attractions {
		if a.QuestID == claimedQuest.ID {
			t.Error("claimed quest should not be in attractions")
		}
	}
	if len(attractions) != 1 {
		t.Errorf("expected 1 attraction (available quest only), got %d", len(attractions))
	}
}

func TestComputeAttractionsWithContext_AppliesHunger(t *testing.T) {
	engine := NewDefaultBoidEngine()

	hungryAgent := newTestAgent("hungry", 10, SkillAnalysis)
	hungryAgent.Status = AgentIdle

	freshAgent := newTestAgent("fresh", 10, SkillAnalysis)
	freshAgent.Status = AgentIdle

	quest := newTestQuest("quest1", DifficultyModerate, SkillAnalysis)

	agents := []Agent{hungryAgent, freshAgent}
	quests := []Quest{quest}

	now := time.Now()
	idleInfo := map[AgentID]AgentIdleInfo{
		hungryAgent.ID: {IdleSince: now.Add(-1 * time.Hour)},  // Very hungry
		freshAgent.ID:  {IdleSince: now.Add(-1 * time.Minute)}, // Just started
	}

	attractions := engine.ComputeAttractionsWithContext(agents, quests, DefaultBoidRules(), idleInfo, now)

	// Find attractions for each agent
	var hungryScore, freshScore float64
	for _, a := range attractions {
		if a.AgentID == hungryAgent.ID {
			hungryScore = a.Hunger
		}
		if a.AgentID == freshAgent.ID {
			freshScore = a.Hunger
		}
	}

	if hungryScore <= freshScore {
		t.Errorf("hungry agent should have higher hunger score: hungry=%f, fresh=%f", hungryScore, freshScore)
	}
}

// =============================================================================
// SUGGEST CLAIMS
// =============================================================================

func TestSuggestClaims_GreedyOneToOne(t *testing.T) {
	engine := NewDefaultBoidEngine()

	agent1 := newTestAgent("agent1", 10, SkillAnalysis)
	agent1.Status = AgentIdle
	agent2 := newTestAgent("agent2", 10, SkillCodeGen)
	agent2.Status = AgentIdle

	quest1 := newTestQuest("quest1", DifficultyModerate, SkillAnalysis)
	quest2 := newTestQuest("quest2", DifficultyModerate, SkillCodeGen)

	agents := []Agent{agent1, agent2}
	quests := []Quest{quest1, quest2}

	attractions := engine.ComputeAttractions(agents, quests, DefaultBoidRules())
	claims := engine.SuggestClaims(attractions)

	// Should assign each agent to one quest
	if len(claims) != 2 {
		t.Fatalf("expected 2 claims, got %d", len(claims))
	}

	// Verify no duplicates
	agentClaimed := make(map[AgentID]bool)
	questClaimed := make(map[QuestID]bool)
	for _, claim := range claims {
		if agentClaimed[claim.AgentID] {
			t.Errorf("agent %s claimed multiple quests", claim.AgentID)
		}
		if questClaimed[claim.QuestID] {
			t.Errorf("quest %s claimed by multiple agents", claim.QuestID)
		}
		agentClaimed[claim.AgentID] = true
		questClaimed[claim.QuestID] = true
	}
}

func TestSuggestClaims_ResolvesConflicts(t *testing.T) {
	engine := NewDefaultBoidEngine()

	// Two agents both best-suited for the same quest
	agent1 := newTestAgent("agent1", 12, SkillAnalysis) // Higher level
	agent1.Status = AgentIdle
	agent2 := newTestAgent("agent2", 8, SkillAnalysis) // Lower level
	agent2.Status = AgentIdle

	// Only one quest available
	quest := newTestQuest("quest1", DifficultyModerate, SkillAnalysis)

	agents := []Agent{agent1, agent2}
	quests := []Quest{quest}

	attractions := engine.ComputeAttractions(agents, quests, DefaultBoidRules())
	claims := engine.SuggestClaims(attractions)

	// Only one agent should get the quest
	if len(claims) != 1 {
		t.Fatalf("expected 1 claim (conflict resolved), got %d", len(claims))
	}

	// Higher-scoring agent should win
	if claims[0].QuestID != quest.ID {
		t.Errorf("wrong quest assigned")
	}
}

func TestSuggestClaims_ConfidenceCalculation(t *testing.T) {
	engine := NewDefaultBoidEngine()

	// Agent with perfect match for one quest, no match for another
	agent := newTestAgent("agent1", 10, SkillAnalysis)
	agent.Status = AgentIdle

	perfectMatch := newTestQuest("perfect", DifficultyModerate, SkillAnalysis)
	noMatch := newTestQuest("nomatch", DifficultyModerate, SkillResearch)

	agents := []Agent{agent}
	quests := []Quest{perfectMatch, noMatch}

	attractions := engine.ComputeAttractions(agents, quests, DefaultBoidRules())
	claims := engine.SuggestClaims(attractions)

	if len(claims) != 1 {
		t.Fatalf("expected 1 claim, got %d", len(claims))
	}

	// Should claim the perfect match quest with high confidence
	if claims[0].QuestID != perfectMatch.ID {
		t.Error("should claim the better match")
	}
	if claims[0].Confidence < 0.1 {
		t.Errorf("expected positive confidence, got %f", claims[0].Confidence)
	}
}

func TestSuggestClaims_EmptyAttractions(t *testing.T) {
	engine := NewDefaultBoidEngine()

	claims := engine.SuggestClaims(nil)
	if len(claims) != 0 {
		t.Errorf("expected 0 claims for empty attractions, got %d", len(claims))
	}

	claims = engine.SuggestClaims([]QuestAttraction{})
	if len(claims) != 0 {
		t.Errorf("expected 0 claims for empty attractions, got %d", len(claims))
	}
}

// =============================================================================
// UPDATE RULES
// =============================================================================

func TestUpdateRules(t *testing.T) {
	engine := NewDefaultBoidEngine()

	// Start with default rules
	original := engine.rules
	if original.NeighborRadius != 5 {
		t.Errorf("expected default NeighborRadius 5, got %d", original.NeighborRadius)
	}

	// Update rules
	newRules := BoidRules{
		SeparationWeight: 2.0,
		AlignmentWeight:  0.5,
		CohesionWeight:   0.3,
		HungerWeight:     1.5,
		AffinityWeight:   2.0,
		CautionWeight:    0.5,
		NeighborRadius:   10,
		UpdateInterval:   500,
	}
	engine.UpdateRules(newRules)

	if engine.rules.NeighborRadius != 10 {
		t.Errorf("expected updated NeighborRadius 10, got %d", engine.rules.NeighborRadius)
	}
	if engine.rules.SeparationWeight != 2.0 {
		t.Errorf("expected updated SeparationWeight 2.0, got %f", engine.rules.SeparationWeight)
	}
}

// =============================================================================
// INTERFACE COMPLIANCE
// =============================================================================

func TestDefaultBoidEngine_ImplementsBoidEngine(_ *testing.T) {
	var _ BoidEngine = (*DefaultBoidEngine)(nil)
}
