package agentprogression

import (
	"strings"
	"testing"

	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// XP ENGINE UNIT TESTS
// =============================================================================
// Tests for DefaultXPEngine.CalculateXP, including peer review bonus integration.
// No external dependencies — runs as part of the fast unit test suite.
// =============================================================================

// baseQuest returns a minimal Quest suitable for XP calculations.
func baseQuest() domain.Quest {
	return domain.Quest{
		ID:         "quest-test-001",
		Title:      "Test Quest",
		Difficulty: domain.DifficultyModerate,
		BaseXP:     100,
		GuildXP:    50,
	}
}

// baseAgent returns a minimal Agent suitable for XP calculations.
func baseAgent() Agent {
	return Agent{
		ID:    "agent-test-001",
		Level: 5,
		XP:    200,
		Tier:  domain.TierApprentice,
	}
}

// float64Ptr is a convenience helper to take the address of a float64 literal.
func float64Ptr(v float64) *float64 {
	return &v
}

// =============================================================================
// PEER REVIEW BONUS TESTS
// =============================================================================

// TestXPCalculation_PeerReviewBonus_High verifies that a score of 5.0 produces
// a positive PeerReviewBonus equal to 30% of BaseXP (the maximum bonus).
func TestXPCalculation_PeerReviewBonus_High(t *testing.T) {
	engine := NewDefaultXPEngine()
	ctx := XPContext{
		Quest:           baseQuest(),
		Agent:           baseAgent(),
		Attempt:         1,
		PeerReviewScore: float64Ptr(5.0),
	}

	award := engine.CalculateXP(ctx)

	// modifier = (5.0 - 3.0) / 2.0 = 1.0; bonus = 100 * 1.0 * 0.3 = 30
	expectedBonus := int64(30)
	if award.PeerReviewBonus != expectedBonus {
		t.Errorf("PeerReviewBonus = %d, want %d", award.PeerReviewBonus, expectedBonus)
	}
	if award.PeerReviewBonus <= 0 {
		t.Errorf("expected positive PeerReviewBonus for score 5.0, got %d", award.PeerReviewBonus)
	}
	// TotalXP must include the bonus
	if award.TotalXP != award.BaseXP+award.PeerReviewBonus {
		t.Errorf("TotalXP = %d, want BaseXP(%d) + PeerReviewBonus(%d) = %d",
			award.TotalXP, award.BaseXP, award.PeerReviewBonus, award.BaseXP+award.PeerReviewBonus)
	}
}

// TestXPCalculation_PeerReviewBonus_Neutral verifies that a score of 3.0 produces
// a PeerReviewBonus of exactly 0 (the neutral midpoint).
func TestXPCalculation_PeerReviewBonus_Neutral(t *testing.T) {
	engine := NewDefaultXPEngine()
	ctx := XPContext{
		Quest:           baseQuest(),
		Agent:           baseAgent(),
		Attempt:         1,
		PeerReviewScore: float64Ptr(3.0),
	}

	award := engine.CalculateXP(ctx)

	// modifier = (3.0 - 3.0) / 2.0 = 0.0; bonus = 0
	if award.PeerReviewBonus != 0 {
		t.Errorf("PeerReviewBonus = %d, want 0 for neutral score 3.0", award.PeerReviewBonus)
	}
	if award.TotalXP != award.BaseXP {
		t.Errorf("TotalXP = %d, want BaseXP = %d for neutral score", award.TotalXP, award.BaseXP)
	}
}

// TestXPCalculation_PeerReviewPenalty_Low verifies that a score of 1.0 produces
// a negative PeerReviewBonus equal to -30% of BaseXP (the maximum penalty).
func TestXPCalculation_PeerReviewPenalty_Low(t *testing.T) {
	engine := NewDefaultXPEngine()
	ctx := XPContext{
		Quest:           baseQuest(),
		Agent:           baseAgent(),
		Attempt:         1,
		PeerReviewScore: float64Ptr(1.0),
	}

	award := engine.CalculateXP(ctx)

	// modifier = (1.0 - 3.0) / 2.0 = -1.0; bonus = 100 * -1.0 * 0.3 = -30
	expectedBonus := int64(-30)
	if award.PeerReviewBonus != expectedBonus {
		t.Errorf("PeerReviewBonus = %d, want %d", award.PeerReviewBonus, expectedBonus)
	}
	if award.PeerReviewBonus >= 0 {
		t.Errorf("expected negative PeerReviewBonus for score 1.0, got %d", award.PeerReviewBonus)
	}
	// TotalXP must be reduced but floored at 0
	expectedTotal := award.BaseXP + award.PeerReviewBonus
	if expectedTotal < 0 {
		expectedTotal = 0
	}
	if award.TotalXP != expectedTotal {
		t.Errorf("TotalXP = %d, want %d", award.TotalXP, expectedTotal)
	}
}

// TestXPCalculation_PeerReviewNil verifies that a nil PeerReviewScore leaves
// PeerReviewBonus at 0 and has no effect on TotalXP.
func TestXPCalculation_PeerReviewNil(t *testing.T) {
	engine := NewDefaultXPEngine()
	ctx := XPContext{
		Quest:           baseQuest(),
		Agent:           baseAgent(),
		Attempt:         1,
		PeerReviewScore: nil, // explicitly nil
	}

	award := engine.CalculateXP(ctx)

	if award.PeerReviewBonus != 0 {
		t.Errorf("PeerReviewBonus = %d, want 0 when PeerReviewScore is nil", award.PeerReviewBonus)
	}
	if award.TotalXP != award.BaseXP {
		t.Errorf("TotalXP = %d, want BaseXP = %d when no peer review score is set",
			award.TotalXP, award.BaseXP)
	}
}

// TestXPCalculation_PeerReviewInBreakdown verifies that a non-zero PeerReviewBonus
// appears in the Breakdown string with the correct sign prefix.
func TestXPCalculation_PeerReviewInBreakdown(t *testing.T) {
	engine := NewDefaultXPEngine()

	t.Run("positive_bonus_in_breakdown", func(t *testing.T) {
		ctx := XPContext{
			Quest:           baseQuest(),
			Agent:           baseAgent(),
			Attempt:         1,
			PeerReviewScore: float64Ptr(5.0),
		}
		award := engine.CalculateXP(ctx)

		if !strings.Contains(award.Breakdown, "PeerReview: +") {
			t.Errorf("Breakdown %q does not contain 'PeerReview: +'", award.Breakdown)
		}
	})

	t.Run("negative_penalty_in_breakdown", func(t *testing.T) {
		ctx := XPContext{
			Quest:           baseQuest(),
			Agent:           baseAgent(),
			Attempt:         1,
			PeerReviewScore: float64Ptr(1.0),
		}
		award := engine.CalculateXP(ctx)

		if !strings.Contains(award.Breakdown, "PeerReview: -") {
			t.Errorf("Breakdown %q does not contain 'PeerReview: -'", award.Breakdown)
		}
	})

	t.Run("neutral_score_omitted_from_breakdown", func(t *testing.T) {
		ctx := XPContext{
			Quest:           baseQuest(),
			Agent:           baseAgent(),
			Attempt:         1,
			PeerReviewScore: float64Ptr(3.0),
		}
		award := engine.CalculateXP(ctx)

		if strings.Contains(award.Breakdown, "PeerReview") {
			t.Errorf("Breakdown %q should not mention PeerReview for neutral score 3.0", award.Breakdown)
		}
	})
}

// TestXPCalculation_PeerReviewWithOtherBonuses verifies that the peer review
// bonus stacks correctly alongside quality, speed, streak, and guild bonuses.
func TestXPCalculation_PeerReviewWithOtherBonuses(t *testing.T) {
	engine := NewDefaultXPEngine()

	// Build a context that triggers every bonus path.
	quest := baseQuest()
	quest.GuildXP = 80

	ctx := XPContext{
		Quest:        quest,
		Agent:        baseAgent(),
		Streak:       3, // streak bonus: 2 * 0.1 * 100 = 20
		Attempt:      1,
		IsGuildQuest: true,
		GuildRank:    domain.GuildRankMember, // multiplier 1.0; guild bonus = 80 * 1.0 * 0.15 = 12
		BattleResult: &domain.BattleVerdict{
			Passed:       true,
			QualityScore: 1.0, // quality bonus = 100 * 1.0 * 2.0 = 200
		},
		PeerReviewScore: float64Ptr(5.0), // peer bonus = 100 * 1.0 * 0.3 = 30
	}

	award := engine.CalculateXP(ctx)

	// Each bonus must be independently computed.
	if award.QualityBonus <= 0 {
		t.Errorf("expected positive QualityBonus, got %d", award.QualityBonus)
	}
	if award.StreakBonus <= 0 {
		t.Errorf("expected positive StreakBonus, got %d", award.StreakBonus)
	}
	if award.GuildBonus <= 0 {
		t.Errorf("expected positive GuildBonus, got %d", award.GuildBonus)
	}
	if award.PeerReviewBonus <= 0 {
		t.Errorf("expected positive PeerReviewBonus, got %d", award.PeerReviewBonus)
	}

	// TotalXP must be the sum of all contributions.
	expectedTotal := award.BaseXP + award.QualityBonus + award.SpeedBonus + award.StreakBonus + award.GuildBonus - award.AttemptPenalty + award.PeerReviewBonus
	if expectedTotal < 0 {
		expectedTotal = 0
	}
	if award.TotalXP != expectedTotal {
		t.Errorf("TotalXP = %d, want %d (sum of all bonuses)", award.TotalXP, expectedTotal)
	}

	// Breakdown must mention PeerReview when bonus is non-zero.
	if !strings.Contains(award.Breakdown, "PeerReview") {
		t.Errorf("Breakdown %q should contain 'PeerReview' when peer bonus is non-zero", award.Breakdown)
	}
}
