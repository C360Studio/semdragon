package semdragons

import (
	"testing"
	"time"

	"github.com/c360studio/semstreams/graph"
)

func TestQuestRoundTrip_ReviewConstraints(t *testing.T) {
	tests := []struct {
		name          string
		requireReview bool
		reviewLevel   ReviewLevel
	}{
		{
			name:          "review required standard",
			requireReview: true,
			reviewLevel:   ReviewStandard,
		},
		{
			name:          "review not required",
			requireReview: false,
			reviewLevel:   ReviewAuto,
		},
		{
			name:          "review required strict",
			requireReview: true,
			reviewLevel:   ReviewStrict,
		},
		{
			name:          "human review",
			requireReview: true,
			reviewLevel:   ReviewHuman,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := &Quest{
				ID:          QuestID("test.dev.game.board1.quest.q1"),
				Title:       "Round Trip Test",
				Description: "Testing reconstruction",
				Status:      QuestPosted,
				Difficulty:  DifficultyModerate,
				BaseXP:      200,
				MaxAttempts: 3,
				PostedAt:    time.Now().Truncate(time.Second),
				Constraints: QuestConstraints{
					RequireReview: tt.requireReview,
					ReviewLevel:   tt.reviewLevel,
				},
			}

			// Serialize to triples and reconstruct
			entity := &graph.EntityState{
				ID:      string(original.ID),
				Triples: original.Triples(),
			}

			reconstructed := QuestFromEntityState(entity)

			if reconstructed.Constraints.RequireReview != tt.requireReview {
				t.Errorf("RequireReview = %v, want %v", reconstructed.Constraints.RequireReview, tt.requireReview)
			}
			if reconstructed.Constraints.ReviewLevel != tt.reviewLevel {
				t.Errorf("ReviewLevel = %v, want %v", reconstructed.Constraints.ReviewLevel, tt.reviewLevel)
			}
		})
	}
}

func TestQuestRoundTrip_FullFields(t *testing.T) {
	claimedAt := time.Now().Truncate(time.Second)
	agentID := AgentID("test.dev.game.board1.agent.a1")
	guildID := GuildID("test.dev.game.board1.guild.g1")

	original := &Quest{
		ID:             QuestID("test.dev.game.board1.quest.q1"),
		Title:          "Full Round Trip",
		Description:    "All fields set",
		Status:         QuestClaimed,
		Difficulty:     DifficultyHard,
		BaseXP:         500,
		MaxAttempts:    5,
		Attempts:       1,
		PostedAt:       time.Now().Truncate(time.Second),
		ClaimedAt:      &claimedAt,
		ClaimedBy:      &agentID,
		GuildPriority:  &guildID,
		RequiredSkills: []SkillTag{SkillCodeGen, SkillAnalysis},
		RequiredTools:  []string{"tool-a", "tool-b"},
		MinTier:        TierJourneyman,
		PartyRequired:  true,
		TrajectoryID:   "traj-xyz",
		Constraints: QuestConstraints{
			RequireReview: true,
			ReviewLevel:   ReviewStrict,
		},
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	r := QuestFromEntityState(entity)

	if r.Title != original.Title {
		t.Errorf("Title = %q, want %q", r.Title, original.Title)
	}
	if r.Status != original.Status {
		t.Errorf("Status = %q, want %q", r.Status, original.Status)
	}
	if r.Difficulty != original.Difficulty {
		t.Errorf("Difficulty = %v, want %v", r.Difficulty, original.Difficulty)
	}
	if r.BaseXP != original.BaseXP {
		t.Errorf("BaseXP = %v, want %v", r.BaseXP, original.BaseXP)
	}
	if r.Attempts != original.Attempts {
		t.Errorf("Attempts = %v, want %v", r.Attempts, original.Attempts)
	}
	if r.MaxAttempts != original.MaxAttempts {
		t.Errorf("MaxAttempts = %v, want %v", r.MaxAttempts, original.MaxAttempts)
	}
	if r.ClaimedBy == nil || *r.ClaimedBy != agentID {
		t.Errorf("ClaimedBy = %v, want %v", r.ClaimedBy, &agentID)
	}
	if r.GuildPriority == nil || *r.GuildPriority != guildID {
		t.Errorf("GuildPriority = %v, want %v", r.GuildPriority, &guildID)
	}
	if r.MinTier != original.MinTier {
		t.Errorf("MinTier = %v, want %v", r.MinTier, original.MinTier)
	}
	if r.PartyRequired != original.PartyRequired {
		t.Errorf("PartyRequired = %v, want %v", r.PartyRequired, original.PartyRequired)
	}
	if r.TrajectoryID != original.TrajectoryID {
		t.Errorf("TrajectoryID = %q, want %q", r.TrajectoryID, original.TrajectoryID)
	}
	if r.Constraints.RequireReview != original.Constraints.RequireReview {
		t.Errorf("RequireReview = %v, want %v", r.Constraints.RequireReview, original.Constraints.RequireReview)
	}
	if r.Constraints.ReviewLevel != original.Constraints.ReviewLevel {
		t.Errorf("ReviewLevel = %v, want %v", r.Constraints.ReviewLevel, original.Constraints.ReviewLevel)
	}
	if len(r.RequiredSkills) != len(original.RequiredSkills) {
		t.Errorf("RequiredSkills len = %d, want %d", len(r.RequiredSkills), len(original.RequiredSkills))
	}
	if len(r.RequiredTools) != len(original.RequiredTools) {
		t.Errorf("RequiredTools len = %d, want %d", len(r.RequiredTools), len(original.RequiredTools))
	}
}

func TestAgentRoundTrip(t *testing.T) {
	questID := QuestID("test.dev.game.board1.quest.q1")
	partyID := PartyID("test.dev.game.board1.party.p1")
	cooldown := time.Now().Add(time.Hour).Truncate(time.Second)

	original := &Agent{
		ID:          AgentID("test.dev.game.board1.agent.a1"),
		Name:        "roundtripper",
		DisplayName: "Round Tripper",
		Status:      AgentOnQuest,
		Level:       10,
		XP:          1500,
		XPToLevel:   2000,
		Tier:        TierJourneyman,
		DeathCount:  2,
		IsNPC:       true,
		Guilds:      []GuildID{"test.dev.game.board1.guild.g1"},
		SkillProficiencies: map[SkillTag]SkillProficiency{
			SkillCodeGen:  {Level: ProficiencyJourneyman, TotalXP: 500},
			SkillAnalysis: {Level: ProficiencyNovice, TotalXP: 100},
		},
		CurrentQuest:  &questID,
		CurrentParty:  &partyID,
		CooldownUntil: &cooldown,
		Stats: AgentStats{
			QuestsCompleted: 15,
			QuestsFailed:    3,
			BossesDefeated:  10,
			TotalXPEarned:   5000,
		},
		CreatedAt: time.Now().Truncate(time.Second),
		UpdatedAt: time.Now().Truncate(time.Second),
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	r := AgentFromEntityState(entity)

	if r.Name != original.Name {
		t.Errorf("Name = %q, want %q", r.Name, original.Name)
	}
	if r.DisplayName != original.DisplayName {
		t.Errorf("DisplayName = %q, want %q", r.DisplayName, original.DisplayName)
	}
	if r.Status != original.Status {
		t.Errorf("Status = %q, want %q", r.Status, original.Status)
	}
	if r.Level != original.Level {
		t.Errorf("Level = %v, want %v", r.Level, original.Level)
	}
	if r.Tier != original.Tier {
		t.Errorf("Tier = %v, want %v", r.Tier, original.Tier)
	}
	if r.IsNPC != original.IsNPC {
		t.Errorf("IsNPC = %v, want %v", r.IsNPC, original.IsNPC)
	}
	if r.CurrentQuest == nil || *r.CurrentQuest != questID {
		t.Errorf("CurrentQuest = %v, want %v", r.CurrentQuest, &questID)
	}
	if len(r.SkillProficiencies) != len(original.SkillProficiencies) {
		t.Errorf("SkillProficiencies len = %d, want %d", len(r.SkillProficiencies), len(original.SkillProficiencies))
	}
}

func TestBattleRoundTrip(t *testing.T) {
	completedAt := time.Now().Truncate(time.Second)

	original := &BossBattle{
		ID:          BattleID("test.dev.game.board1.battle.b1"),
		QuestID:     QuestID("test.dev.game.board1.quest.q1"),
		AgentID:     AgentID("test.dev.game.board1.agent.a1"),
		Status:      BattleVictory,
		Level:       ReviewStandard,
		StartedAt:   time.Now().Truncate(time.Second),
		CompletedAt: &completedAt,
		Verdict: &BattleVerdict{
			Passed:       true,
			QualityScore: 0.85,
			XPAwarded:    150,
			Feedback:     "Great work",
		},
		Judges: []Judge{
			{ID: "judge-1"},
			{ID: "judge-2"},
		},
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	r := BattleFromEntityState(entity)

	if r.QuestID != original.QuestID {
		t.Errorf("QuestID = %q, want %q", r.QuestID, original.QuestID)
	}
	if r.Status != original.Status {
		t.Errorf("Status = %q, want %q", r.Status, original.Status)
	}
	if r.Verdict == nil {
		t.Fatal("Verdict is nil")
	}
	if r.Verdict.Passed != original.Verdict.Passed {
		t.Errorf("Verdict.Passed = %v, want %v", r.Verdict.Passed, original.Verdict.Passed)
	}
	if r.Verdict.QualityScore != original.Verdict.QualityScore {
		t.Errorf("Verdict.QualityScore = %v, want %v", r.Verdict.QualityScore, original.Verdict.QualityScore)
	}
	if r.Verdict.Feedback != original.Verdict.Feedback {
		t.Errorf("Verdict.Feedback = %q, want %q", r.Verdict.Feedback, original.Verdict.Feedback)
	}
	if len(r.Judges) != len(original.Judges) {
		t.Errorf("Judges len = %d, want %d", len(r.Judges), len(original.Judges))
	}
}

func TestQuestFromEntityState_NilReturnsNil(t *testing.T) {
	if got := QuestFromEntityState(nil); got != nil {
		t.Errorf("QuestFromEntityState(nil) = %v, want nil", got)
	}
}

func TestAgentFromEntityState_NilReturnsNil(t *testing.T) {
	if got := AgentFromEntityState(nil); got != nil {
		t.Errorf("AgentFromEntityState(nil) = %v, want nil", got)
	}
}

func TestBattleFromEntityState_NilReturnsNil(t *testing.T) {
	if got := BattleFromEntityState(nil); got != nil {
		t.Errorf("BattleFromEntityState(nil) = %v, want nil", got)
	}
}
