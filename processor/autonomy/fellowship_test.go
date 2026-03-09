package autonomy

import (
	"testing"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
)

// =============================================================================
// FELLOWSHIP SCORING TESTS
// =============================================================================
// Tests for the fellowship scoring system used by createGuildAction.
// No Docker required. Run with: go test ./processor/autonomy/...
// =============================================================================

func makeAgent(id string, level int, skills []domain.SkillTag) *agentprogression.Agent {
	proficiencies := make(map[domain.SkillTag]domain.SkillProficiency, len(skills))
	for _, s := range skills {
		proficiencies[s] = domain.SkillProficiency{Level: 1}
	}
	return &agentprogression.Agent{
		ID:                 domain.AgentID(id),
		Name:               id,
		Level:              level,
		SkillProficiencies: proficiencies,
	}
}

func TestSkillComplementarity_FullyDisjoint(t *testing.T) {
	a := makeAgent("a", 10, []domain.SkillTag{domain.SkillAnalysis, domain.SkillCodeGen})
	b := makeAgent("b", 10, []domain.SkillTag{domain.SkillResearch, domain.SkillPlanning})

	result := skillComplementarity(a, b)

	// 4 unique, 0 shared → 4/4 = 1.0
	if result != 1.0 {
		t.Errorf("fully disjoint skills: got %.2f, want 1.00", result)
	}
}

func TestSkillComplementarity_FullOverlap(t *testing.T) {
	skills := []domain.SkillTag{domain.SkillAnalysis, domain.SkillCodeGen}
	a := makeAgent("a", 10, skills)
	b := makeAgent("b", 10, skills)

	result := skillComplementarity(a, b)

	// 2 unique, 2 shared → 0/2 = 0.0
	if result != 0.0 {
		t.Errorf("full overlap: got %.2f, want 0.00", result)
	}
}

func TestSkillComplementarity_PartialOverlap(t *testing.T) {
	a := makeAgent("a", 10, []domain.SkillTag{domain.SkillAnalysis, domain.SkillCodeGen})
	b := makeAgent("b", 10, []domain.SkillTag{domain.SkillCodeGen, domain.SkillResearch})

	result := skillComplementarity(a, b)

	// 3 total, 1 shared → 2/3 ≈ 0.667
	want := 2.0 / 3.0
	if result < want-0.01 || result > want+0.01 {
		t.Errorf("partial overlap: got %.3f, want %.3f", result, want)
	}
}

func TestSkillComplementarity_BothEmpty(t *testing.T) {
	a := makeAgent("a", 10, nil)
	b := makeAgent("b", 10, nil)

	result := skillComplementarity(a, b)

	if result != 0.5 {
		t.Errorf("both empty: got %.2f, want 0.50", result)
	}
}

func TestAverageReputation_NoPeerReviews(t *testing.T) {
	a := makeAgent("a", 10, nil)
	b := makeAgent("b", 10, nil)

	result := averageReputation(a, b)

	// Both default to 0.5 → average 0.5
	if result != 0.5 {
		t.Errorf("no reviews: got %.2f, want 0.50", result)
	}
}

func TestAverageReputation_HighReviews(t *testing.T) {
	a := makeAgent("a", 10, nil)
	a.Stats.PeerReviewAvg = 5.0
	a.Stats.PeerReviewCount = 5
	b := makeAgent("b", 10, nil)
	b.Stats.PeerReviewAvg = 5.0
	b.Stats.PeerReviewCount = 3

	result := averageReputation(a, b)

	// (5-1)/4 = 1.0 each → avg 1.0
	if result != 1.0 {
		t.Errorf("high reviews: got %.2f, want 1.00", result)
	}
}

func TestAverageReputation_MixedReviews(t *testing.T) {
	a := makeAgent("a", 10, nil)
	a.Stats.PeerReviewAvg = 5.0
	a.Stats.PeerReviewCount = 5
	b := makeAgent("b", 10, nil)
	// b has no reviews → defaults to 0.5

	result := averageReputation(a, b)

	// (1.0 + 0.5) / 2 = 0.75
	if result != 0.75 {
		t.Errorf("mixed reviews: got %.2f, want 0.75", result)
	}
}

func TestScoreFellowship_DiverseUnguilded(t *testing.T) {
	// Two unguilded agents with disjoint skills and good reviews should have high fellowship.
	a := makeAgent("a", 16, []domain.SkillTag{domain.SkillAnalysis})
	a.Stats.PeerReviewAvg = 4.5
	a.Stats.PeerReviewCount = 5
	b := makeAgent("b", 15, []domain.SkillTag{domain.SkillCodeGen})
	b.Stats.PeerReviewAvg = 4.0
	b.Stats.PeerReviewCount = 3

	score := scoreFellowship(a, b, nil, 0)

	// Expect high score: fully disjoint skills (1.0 * 0.4) + high reputation + close level + unguilded
	if score < 0.7 {
		t.Errorf("diverse unguilded fellowship: got %.3f, want >= 0.70", score)
	}
}

func TestScoreFellowship_SameSkillsGuilded(t *testing.T) {
	// Two guilded agents with identical skills should have low fellowship.
	skills := []domain.SkillTag{domain.SkillAnalysis}
	a := makeAgent("a", 16, skills)
	b := makeAgent("b", 16, skills)
	b.Guild = domain.GuildID("existing-guild")

	score := scoreFellowship(a, b, nil, 1)

	// Same skills (0.0 * 0.4) + low guild need (0.3 * 0.15) → low score
	if score > 0.5 {
		t.Errorf("same skills guilded: got %.3f, want <= 0.50", score)
	}
}

func TestScoreFellowship_FounderGuildPenalty(t *testing.T) {
	// Peer already in founder's guild should get extra penalty.
	a := makeAgent("a", 16, []domain.SkillTag{domain.SkillAnalysis})
	b := makeAgent("b", 16, []domain.SkillTag{domain.SkillCodeGen})
	b.Guild = domain.GuildID("founders-guild")

	guild := &domain.Guild{
		ID:        "founders-guild",
		FoundedBy: "a",
		Members: []domain.GuildMember{
			{AgentID: "a"},
			{AgentID: "b"},
		},
	}

	scoreWithPenalty := scoreFellowship(a, b, []*domain.Guild{guild}, 1)
	scoreWithoutPenalty := scoreFellowship(a, b, nil, 1)

	if scoreWithPenalty >= scoreWithoutPenalty {
		t.Errorf("founder guild penalty should reduce score: with=%.3f, without=%.3f",
			scoreWithPenalty, scoreWithoutPenalty)
	}
}

func TestScoreFellowship_LevelProximity(t *testing.T) {
	// Same level should score higher than distant levels.
	a := makeAgent("a", 16, []domain.SkillTag{domain.SkillAnalysis})
	near := makeAgent("near", 16, []domain.SkillTag{domain.SkillCodeGen})
	far := makeAgent("far", 5, []domain.SkillTag{domain.SkillCodeGen})

	scoreNear := scoreFellowship(a, near, nil, 0)
	scoreFar := scoreFellowship(a, far, nil, 0)

	if scoreNear <= scoreFar {
		t.Errorf("near level should score higher: near=%.3f, far=%.3f", scoreNear, scoreFar)
	}
}

func TestSelectFellowshipCandidates_PrefersDiversity(t *testing.T) {
	founder := makeAgent("founder", 16, []domain.SkillTag{domain.SkillAnalysis})

	// Candidate with new skill should be preferred over higher-scored same-skill candidate.
	highScoreSameSkill := fellowCandidate{
		agent: makeAgent("same", 15, []domain.SkillTag{domain.SkillAnalysis}),
		score: 0.9,
	}
	lowScoreNewSkill := fellowCandidate{
		agent: makeAgent("new", 15, []domain.SkillTag{domain.SkillCodeGen}),
		score: 0.5,
	}

	candidates := []fellowCandidate{highScoreSameSkill, lowScoreNewSkill}
	selected := selectFellowshipCandidates(founder, candidates, 1)

	if len(selected) != 1 {
		t.Fatalf("selected count: got %d, want 1", len(selected))
	}
	if selected[0].agent.ID != "new" {
		t.Errorf("should prefer diverse skill: got %s, want 'new'", selected[0].agent.ID)
	}
}

func TestSelectFellowshipCandidates_FillsFromScore(t *testing.T) {
	founder := makeAgent("founder", 16, []domain.SkillTag{domain.SkillAnalysis})

	// All candidates have same skill as founder — diversity pass finds none.
	// Should fall back to score-ordered fill.
	c1 := fellowCandidate{
		agent: makeAgent("c1", 15, []domain.SkillTag{domain.SkillAnalysis}),
		score: 0.8,
	}
	c2 := fellowCandidate{
		agent: makeAgent("c2", 15, []domain.SkillTag{domain.SkillAnalysis}),
		score: 0.6,
	}

	candidates := []fellowCandidate{c1, c2}
	selected := selectFellowshipCandidates(founder, candidates, 1)

	if len(selected) != 1 {
		t.Fatalf("selected count: got %d, want 1", len(selected))
	}
	// Should get c1 (higher score) from the fill pass
	if selected[0].agent.ID != "c1" {
		t.Errorf("should fill by score: got %s, want 'c1'", selected[0].agent.ID)
	}
}

func TestSelectFellowshipCandidates_UnderLimit(t *testing.T) {
	founder := makeAgent("founder", 16, nil)
	candidates := []fellowCandidate{
		{agent: makeAgent("c1", 15, nil), score: 0.8},
	}

	selected := selectFellowshipCandidates(founder, candidates, 5)

	// Under limit — should return all
	if len(selected) != 1 {
		t.Fatalf("under limit: got %d, want 1", len(selected))
	}
}

func TestIsMemberOf(t *testing.T) {
	guild := &domain.Guild{
		Members: []domain.GuildMember{
			{AgentID: "agent-a"},
			{AgentID: "agent-b"},
		},
	}

	if !isMemberOf(guild, "agent-a") {
		t.Error("agent-a should be a member")
	}
	if isMemberOf(guild, "agent-c") {
		t.Error("agent-c should not be a member")
	}
}

func TestLevelAbs(t *testing.T) {
	tests := []struct {
		in, want int
	}{
		{0, 0}, {5, 5}, {-5, 5}, {-1, 1},
	}
	for _, tt := range tests {
		if got := levelAbs(tt.in); got != tt.want {
			t.Errorf("levelAbs(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestAgentGuildName(t *testing.T) {
	agent := makeAgent("test-agent", 16, nil)
	agent.Name = "Dragon"
	agent.DisplayName = ""

	if got := agentGuildName(agent); got != "Dragon's Guild" {
		t.Errorf("got %q, want %q", got, "Dragon's Guild")
	}

	agent.DisplayName = "Flame Drake"
	if got := agentGuildName(agent); got != "Flame Drake's Guild" {
		t.Errorf("got %q, want %q", got, "Flame Drake's Guild")
	}
}
