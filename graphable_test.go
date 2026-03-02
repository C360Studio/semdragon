package semdragons

import (
	"testing"
	"time"
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

func TestAgentTriples_ContainsExpectedPredicates(t *testing.T) {
	a := &Agent{
		ID:          AgentID("test.dev.game.board1.agent.a1"),
		Name:        "TestAgent",
		DisplayName: "Test",
		Status:      AgentIdle,
		Level:       5,
		XP:          250,
		XPToLevel:   500,
		Tier:        TierApprentice,
		SkillProficiencies: map[SkillTag]SkillProficiency{
			SkillCodeGen: {Level: ProficiencyNovice, TotalXP: 100},
		},
	}

	triples := a.Triples()

	expected := map[string]bool{
		"agent.identity.name":                  false,
		"agent.identity.display_name":          false,
		"agent.status.state":                   false,
		"agent.progression.level":              false,
		"agent.progression.xp.current":         false,
		"agent.progression.xp.to_level":        false,
		"agent.progression.tier":               false,
		"agent.skill.code_generation.level":    false,
		"agent.skill.code_generation.total_xp": false,
	}

	for _, triple := range triples {
		if _, ok := expected[triple.Predicate]; ok {
			expected[triple.Predicate] = true
		}
	}

	for pred, found := range expected {
		if !found {
			t.Errorf("expected predicate %q not found in agent triples", pred)
		}
	}
}

func TestAgentTriples_IncludesInventory(t *testing.T) {
	questID := QuestID("test.dev.game.board1.quest.q1")
	a := &Agent{
		ID:     AgentID("test.dev.game.board1.agent.a1"),
		Name:   "StoreAgent",
		Status: AgentIdle,
		Level:  5,
		OwnedTools: map[string]OwnedTool{
			"web_search": {
				StoreItemID:   "test.dev.game.board1.storeitem.web_search",
				XPSpent:       50,
				UsesRemaining: -1,
				PurchasedAt:   time.Now(),
			},
		},
		Consumables: map[string]int{
			"xp_boost": 2,
		},
		TotalSpent: 150,
		ActiveEffects: []AgentEffect{
			{EffectType: "xp_boost", QuestsRemaining: 1, QuestID: &questID},
		},
	}

	triples := a.Triples()

	expected := map[string]bool{
		"agent.inventory.tool.web_search":              false,
		"agent.inventory.tool.web_search.xp_spent":     false,
		"agent.inventory.tool.web_search.uses":          false,
		"agent.inventory.tool.web_search.purchased_at": false,
		"agent.inventory.consumable.xp_boost":          false,
		"agent.inventory.total_spent":                  false,
		"agent.effects.xp_boost.remaining":             false,
		"agent.effects.xp_boost.quest":                 false,
	}

	for _, triple := range triples {
		if _, ok := expected[triple.Predicate]; ok {
			expected[triple.Predicate] = true
		}
	}

	for pred, found := range expected {
		if !found {
			t.Errorf("expected inventory predicate %q not found in agent triples", pred)
		}
	}

	// Verify specific values
	for _, triple := range triples {
		switch triple.Predicate {
		case "agent.inventory.tool.web_search":
			if got := triple.Object.(string); got != "test.dev.game.board1.storeitem.web_search" {
				t.Errorf("tool entity ref = %q, want storeitem entity ID", got)
			}
		case "agent.inventory.consumable.xp_boost":
			if got := triple.Object.(int); got != 2 {
				t.Errorf("consumable count = %d, want 2", got)
			}
		case "agent.inventory.total_spent":
			if got := triple.Object.(int64); got != 150 {
				t.Errorf("total_spent = %d, want 150", got)
			}
		}
	}
}

func TestAgentTriples_EmptyInventory(t *testing.T) {
	a := &Agent{
		ID:     AgentID("test.dev.game.board1.agent.a1"),
		Name:   "NoItems",
		Status: AgentIdle,
		Level:  1,
	}

	triples := a.Triples()

	// Should NOT have inventory triples when maps are nil/empty
	for _, triple := range triples {
		if triple.Predicate == "agent.inventory.total_spent" {
			t.Error("agent with zero TotalSpent should not emit agent.inventory.total_spent")
		}
	}
}

func TestBattleTriples_WithVerdict(t *testing.T) {
	completedAt := time.Now()
	b := &BossBattle{
		ID:          BattleID("test.dev.game.board1.battle.b1"),
		QuestID:     QuestID("test.dev.game.board1.quest.q1"),
		AgentID:     AgentID("test.dev.game.board1.agent.a1"),
		Status:      BattleVictory,
		Level:       ReviewStandard,
		StartedAt:   time.Now(),
		CompletedAt: &completedAt,
		Verdict: &BattleVerdict{
			Passed:       true,
			QualityScore: 0.85,
			XPAwarded:    150,
			Feedback:     "Well done",
		},
	}

	triples := b.Triples()

	expected := map[string]bool{
		"battle.assignment.quest":       false,
		"battle.assignment.agent":       false,
		"battle.status.state":           false,
		"battle.review.level":           false,
		"battle.verdict.passed":         false,
		"battle.verdict.score":          false,
		"battle.verdict.xp_awarded":     false,
		"battle.verdict.feedback":       false,
		"battle.lifecycle.started_at":   false,
		"battle.lifecycle.completed_at": false,
	}

	for _, triple := range triples {
		if _, ok := expected[triple.Predicate]; ok {
			expected[triple.Predicate] = true
		}
	}

	for pred, found := range expected {
		if !found {
			t.Errorf("expected predicate %q not found in battle triples", pred)
		}
	}
}
