package domain

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

func TestQuestFromEntityState_NilReturnsNil(t *testing.T) {
	if got := QuestFromEntityState(nil); got != nil {
		t.Errorf("QuestFromEntityState(nil) = %v, want nil", got)
	}
}

func TestQuestRoundTrip_DependsOnAndAcceptance(t *testing.T) {
	dep1 := QuestID("test.dev.game.board1.quest.dep1")
	dep2 := QuestID("test.dev.game.board1.quest.dep2")

	original := &Quest{
		ID:          QuestID("test.dev.game.board1.quest.q1"),
		Title:       "Depends On Test",
		Description: "Testing DependsOn and Acceptance round-trip",
		Status:      QuestPosted,
		Difficulty:  DifficultyModerate,
		BaseXP:      200,
		MaxAttempts: 3,
		PostedAt:    time.Now().Truncate(time.Second),
		DependsOn:   []QuestID{dep1, dep2},
		Acceptance: []string{
			"All unit tests pass",
			"Code review approved",
			"Documentation updated",
		},
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	r := QuestFromEntityState(entity)

	if len(r.DependsOn) != 2 {
		t.Fatalf("DependsOn len = %d, want 2", len(r.DependsOn))
	}
	if r.DependsOn[0] != dep1 {
		t.Errorf("DependsOn[0] = %q, want %q", r.DependsOn[0], dep1)
	}
	if r.DependsOn[1] != dep2 {
		t.Errorf("DependsOn[1] = %q, want %q", r.DependsOn[1], dep2)
	}

	if len(r.Acceptance) != 3 {
		t.Fatalf("Acceptance len = %d, want 3", len(r.Acceptance))
	}
	if r.Acceptance[0] != "All unit tests pass" {
		t.Errorf("Acceptance[0] = %q, want %q", r.Acceptance[0], "All unit tests pass")
	}
	if r.Acceptance[1] != "Code review approved" {
		t.Errorf("Acceptance[1] = %q, want %q", r.Acceptance[1], "Code review approved")
	}
	if r.Acceptance[2] != "Documentation updated" {
		t.Errorf("Acceptance[2] = %q, want %q", r.Acceptance[2], "Documentation updated")
	}
}

func TestQuestRoundTrip_EmptyDependsOnAndAcceptance(t *testing.T) {
	original := &Quest{
		ID:          QuestID("test.dev.game.board1.quest.q1"),
		Title:       "No Deps Test",
		Status:      QuestPosted,
		Difficulty:  DifficultyEasy,
		BaseXP:      50,
		MaxAttempts: 3,
		PostedAt:    time.Now().Truncate(time.Second),
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	r := QuestFromEntityState(entity)

	if len(r.DependsOn) != 0 {
		t.Errorf("DependsOn len = %d, want 0", len(r.DependsOn))
	}
	if len(r.Acceptance) != 0 {
		t.Errorf("Acceptance len = %d, want 0", len(r.Acceptance))
	}
}
