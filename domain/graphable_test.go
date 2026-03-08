package domain

import (
	"testing"
	"time"

	"github.com/c360studio/semstreams/graph"
)

func TestQuestTriples_ReviewPredicates(t *testing.T) {
	tests := []struct {
		name            string
		requireReview   bool
		reviewLevel     ReviewLevel
		wantNeedsReview bool
		wantLevel       int
	}{
		{
			name:            "review required with standard level",
			requireReview:   true,
			reviewLevel:     ReviewStandard,
			wantNeedsReview: true,
			wantLevel:       int(ReviewStandard),
		},
		{
			name:            "review not required",
			requireReview:   false,
			reviewLevel:     ReviewAuto,
			wantNeedsReview: false,
			wantLevel:       int(ReviewAuto),
		},
		{
			name:            "strict review",
			requireReview:   true,
			reviewLevel:     ReviewStrict,
			wantNeedsReview: true,
			wantLevel:       int(ReviewStrict),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &Quest{
				ID:       QuestID("test.dev.game.board1.quest.q1"),
				Title:    "Test Quest",
				Status:   QuestPosted,
				PostedAt: time.Now(),
				Constraints: QuestConstraints{
					RequireReview: tt.requireReview,
					ReviewLevel:   tt.reviewLevel,
				},
			}

			triples := q.Triples()

			var foundNeedsReview, foundLevel bool
			for _, triple := range triples {
				switch triple.Predicate {
				case "quest.review.needs_review":
					foundNeedsReview = true
					got, ok := triple.Object.(bool)
					if !ok {
						t.Errorf("quest.review.needs_review: expected bool, got %T", triple.Object)
					} else if got != tt.wantNeedsReview {
						t.Errorf("quest.review.needs_review = %v, want %v", got, tt.wantNeedsReview)
					}
				case "quest.review.level":
					foundLevel = true
					got, ok := triple.Object.(int)
					if !ok {
						t.Errorf("quest.review.level: expected int, got %T", triple.Object)
					} else if got != tt.wantLevel {
						t.Errorf("quest.review.level = %v, want %v", got, tt.wantLevel)
					}
				}
			}

			if !foundNeedsReview {
				t.Error("quest.review.needs_review triple not found")
			}
			if !foundLevel {
				t.Error("quest.review.level triple not found")
			}
		})
	}
}

func TestQuestRoundTrip_DMClarifications(t *testing.T) {
	askedAt := time.Now().Truncate(time.Second)
	clarifications := []ClarificationExchange{
		{
			Question: "What is the expected output format?",
			Answer:   "Return a JSON object with keys: result, confidence.",
			AskedAt:  askedAt,
		},
		{
			Question: "Should the agent include reasoning steps?",
			Answer:   "Yes, include a brief chain-of-thought.",
			AskedAt:  askedAt,
		},
	}

	original := &Quest{
		ID:               QuestID("test.dev.game.board1.quest.q1"),
		Title:            "DM Clarification Round-Trip",
		Status:           QuestInProgress,
		PostedAt:         time.Now().Truncate(time.Second),
		DMClarifications: clarifications,
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	r := QuestFromEntityState(entity)

	if r.DMClarifications == nil {
		t.Fatal("DMClarifications is nil, want non-nil")
	}
}

func TestQuestRoundTrip_DMClarificationsNil(t *testing.T) {
	// A quest without DM clarifications should not emit the predicate.
	original := &Quest{
		ID:       QuestID("test.dev.game.board1.quest.q2"),
		Title:    "No DM Clarifications",
		Status:   QuestPosted,
		PostedAt: time.Now().Truncate(time.Second),
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	r := QuestFromEntityState(entity)

	if r.DMClarifications != nil {
		t.Errorf("DMClarifications = %v, want nil", r.DMClarifications)
	}

	// Also verify no quest.dm.clarifications triple was emitted.
	for _, triple := range entity.Triples {
		if triple.Predicate == "quest.dm.clarifications" {
			t.Errorf("unexpected triple emitted: predicate %q with object %v", triple.Predicate, triple.Object)
		}
	}
}

func TestGuildRoundTrip_WithApplications(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	deadline := now.Add(5 * time.Minute)
	reviewedAt := now.Add(2 * time.Minute)
	reviewerID := AgentID("c360.dev.game.board1.agent.founder1")

	original := &Guild{
		ID:          GuildID("c360.dev.game.board1.guild.testguild"),
		Name:        "Test Guild",
		Description: "A guild for testing",
		Status:      GuildPending,
		Members: []GuildMember{
			{AgentID: AgentID("c360.dev.game.board1.agent.founder1"), Rank: GuildRankMaster, Contribution: 100.0},
		},
		MaxMembers:        20,
		MinLevel:          3,
		Founded:           now,
		FoundedBy:         AgentID("c360.dev.game.board1.agent.founder1"),
		QuorumSize:        3,
		FormationDeadline: &deadline,
		Culture:           "Ship quality code",
		Motto:             "Test all the things",
		Reputation:        0.5,
		CreatedAt:         now,
		Applications: []GuildApplication{
			{
				ID:          "app1",
				GuildID:     GuildID("c360.dev.game.board1.guild.testguild"),
				ApplicantID: AgentID("c360.dev.game.board1.agent.candidate1"),
				Status:      ApplicationPending,
				Message:     "I bring data wrangling skills",
				Skills:      []SkillTag{SkillAnalysis, SkillCodeGen},
				Level:       5,
				Tier:        TierApprentice,
				AppliedAt:   now,
			},
			{
				ID:          "app2",
				GuildID:     GuildID("c360.dev.game.board1.guild.testguild"),
				ApplicantID: AgentID("c360.dev.game.board1.agent.candidate2"),
				Status:      ApplicationAccepted,
				Message:     "Experienced reviewer",
				Skills:      []SkillTag{SkillCodeReview},
				Level:       8,
				Tier:        TierJourneyman,
				ReviewedBy:  &reviewerID,
				Reason:      "Good skill complement",
				AppliedAt:   now,
				ReviewedAt:  &reviewedAt,
			},
		},
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	r := GuildFromEntityState(entity)

	// Core fields
	if r.ID != original.ID {
		t.Errorf("ID = %v, want %v", r.ID, original.ID)
	}
	if r.Name != original.Name {
		t.Errorf("Name = %v, want %v", r.Name, original.Name)
	}
	if r.Status != GuildPending {
		t.Errorf("Status = %v, want %v", r.Status, GuildPending)
	}
	if r.QuorumSize != 3 {
		t.Errorf("QuorumSize = %d, want 3", r.QuorumSize)
	}
	if r.FormationDeadline == nil {
		t.Fatal("FormationDeadline is nil, want non-nil")
	}
	if !r.FormationDeadline.Equal(deadline) {
		t.Errorf("FormationDeadline = %v, want %v", r.FormationDeadline, deadline)
	}

	// Members
	if len(r.Members) != 1 {
		t.Fatalf("Members count = %d, want 1", len(r.Members))
	}

	// Applications
	if len(r.Applications) != 2 {
		t.Fatalf("Applications count = %d, want 2", len(r.Applications))
	}

	// Find apps by ID (map iteration order is non-deterministic)
	appByID := make(map[string]GuildApplication)
	for _, app := range r.Applications {
		appByID[app.ID] = app
	}

	app1, ok := appByID["app1"]
	if !ok {
		t.Fatal("app1 not found in reconstructed applications")
	}
	if app1.Status != ApplicationPending {
		t.Errorf("app1.Status = %v, want %v", app1.Status, ApplicationPending)
	}
	if app1.Message != "I bring data wrangling skills" {
		t.Errorf("app1.Message = %v, want 'I bring data wrangling skills'", app1.Message)
	}
	if len(app1.Skills) != 2 {
		t.Errorf("app1.Skills count = %d, want 2", len(app1.Skills))
	}
	if app1.Level != 5 {
		t.Errorf("app1.Level = %d, want 5", app1.Level)
	}
	if app1.Tier != TierApprentice {
		t.Errorf("app1.Tier = %v, want %v", app1.Tier, TierApprentice)
	}

	app2, ok := appByID["app2"]
	if !ok {
		t.Fatal("app2 not found in reconstructed applications")
	}
	if app2.Status != ApplicationAccepted {
		t.Errorf("app2.Status = %v, want %v", app2.Status, ApplicationAccepted)
	}
	if app2.ReviewedBy == nil {
		t.Fatal("app2.ReviewedBy is nil, want non-nil")
	}
	if *app2.ReviewedBy != reviewerID {
		t.Errorf("app2.ReviewedBy = %v, want %v", *app2.ReviewedBy, reviewerID)
	}
	if app2.Reason != "Good skill complement" {
		t.Errorf("app2.Reason = %v, want 'Good skill complement'", app2.Reason)
	}
	if app2.ReviewedAt == nil {
		t.Fatal("app2.ReviewedAt is nil, want non-nil")
	}
}
