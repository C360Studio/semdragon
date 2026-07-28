package bossbattle

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"

	"github.com/c360studio/semdragons/domain"
)

var bossBattleScalarPredicateRenames = map[string]string{
	"battle.lifecycle.started_at":   "battle.lifecycle.started-at",
	"battle.execution.loop_id":      "battle.execution.loop-id",
	"battle.verdict.quality_score":  "battle.verdict.quality-score",
	"battle.verdict.xp_awarded":     "battle.verdict.xp-awarded",
	"battle.verdict.xp_penalty":     "battle.verdict.xp-penalty",
	"battle.verdict.level_change":   "battle.verdict.level-change",
	"battle.lifecycle.completed_at": "battle.lifecycle.completed-at",
}

func TestBossBattleScalarPredicateMappingIsCanonicalOnly(t *testing.T) {
	t.Parallel()

	if len(bossBattleScalarPredicateRenames) != 7 {
		t.Fatalf("scalar rename contract has %d entries, want 7", len(bossBattleScalarPredicateRenames))
	}

	source, err := os.ReadFile("battle.go")
	if err != nil {
		t.Fatalf("read battle.go: %v", err)
	}
	text := string(source)
	for stale, canonical := range bossBattleScalarPredicateRenames {
		if strings.Contains(text, `"`+stale+`"`) {
			t.Errorf("battle.go retains stale predicate %q", stale)
		}
		if got := strings.Count(text, `"`+canonical+`"`); got != 2 {
			t.Errorf("battle.go canonical predicate %q occurrences = %d, want producer and reconstruction", canonical, got)
		}
	}
	if strings.Contains(text, `"battle.verdict.score"`) {
		t.Error("battle.go retains the battle.verdict.score reconstruction alias")
	}
}

func TestBossBattleCanonicalScalarTriplesPreserveValuesTypesAndDatatypes(t *testing.T) {
	t.Parallel()

	battle := canonicalScalarBattle()
	triples := battle.Triples()
	tests := []struct {
		predicate string
		want      any
	}{
		{predicate: "battle.lifecycle.started-at", want: battle.StartedAt.Format(time.RFC3339)},
		{predicate: "battle.execution.loop-id", want: battle.LoopID},
		{predicate: "battle.verdict.quality-score", want: battle.Verdict.QualityScore},
		{predicate: "battle.verdict.xp-awarded", want: battle.Verdict.XPAwarded},
		{predicate: "battle.verdict.xp-penalty", want: battle.Verdict.XPPenalty},
		{predicate: "battle.verdict.level-change", want: battle.Verdict.LevelChange},
		{predicate: "battle.lifecycle.completed-at", want: battle.CompletedAt.Format(time.RFC3339)},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.predicate, func(t *testing.T) {
			t.Parallel()

			triple, ok := tripleByPredicate(triples, tt.predicate)
			if !ok {
				t.Fatalf("missing canonical predicate %q", tt.predicate)
			}
			if triple.Object != tt.want {
				t.Errorf("%s object = %#v (%T), want %#v (%T)", tt.predicate, triple.Object, triple.Object, tt.want, tt.want)
			}
			if triple.Datatype != "" {
				t.Errorf("%s datatype = %q, want untyped scalar", tt.predicate, triple.Datatype)
			}
		})
	}

	for stale := range bossBattleScalarPredicateRenames {
		if _, ok := tripleByPredicate(triples, stale); ok {
			t.Errorf("producer emitted stale predicate %q", stale)
		}
	}
	if _, ok := tripleByPredicate(triples, "battle.verdict.score"); ok {
		t.Error("producer emitted obsolete battle.verdict.score")
	}
}

func TestBossBattleAssignmentRelationshipsRemainEntityReferences(t *testing.T) {
	t.Parallel()

	battle := canonicalScalarBattle()
	triples := battle.Triples()
	expectedTargets := map[string]string{
		"battle.assignment.quest": string(battle.QuestID),
		"battle.assignment.agent": string(battle.AgentID),
	}
	observed := make(map[string]int, len(expectedTargets))
	for _, triple := range triples {
		expectedTarget, relationship := expectedTargets[triple.Predicate]
		if !relationship {
			if triple.Datatype != "" {
				t.Errorf("non-relationship %q datatype = %q, want empty", triple.Predicate, triple.Datatype)
			}
			continue
		}

		observed[triple.Predicate]++
		if triple.Object != expectedTarget {
			t.Errorf("%s object = %#v, want canonical target %q", triple.Predicate, triple.Object, expectedTarget)
		}
		if triple.Datatype != message.EntityReferenceDatatype {
			t.Errorf("%s datatype = %q, want %q", triple.Predicate, triple.Datatype, message.EntityReferenceDatatype)
		}
	}
	for predicate := range expectedTargets {
		if observed[predicate] != 1 {
			t.Errorf("relationship %q observed %d times, want exactly once", predicate, observed[predicate])
		}
	}
}

func TestBossBattleCanonicalScalarTriplesRoundTrip(t *testing.T) {
	t.Parallel()

	want := canonicalScalarBattle()
	got := BattleFromEntityState(&graph.EntityState{
		ID:      want.EntityID(),
		Triples: want.Triples(),
	})

	if got.StartedAt != want.StartedAt {
		t.Errorf("StartedAt = %v, want %v", got.StartedAt, want.StartedAt)
	}
	if got.LoopID != want.LoopID {
		t.Errorf("LoopID = %q, want %q", got.LoopID, want.LoopID)
	}
	if got.CompletedAt == nil || !got.CompletedAt.Equal(*want.CompletedAt) {
		t.Errorf("CompletedAt = %v, want %v", got.CompletedAt, want.CompletedAt)
	}
	if got.Verdict == nil {
		t.Fatal("Verdict is nil after canonical round trip")
	}
	if got.Verdict.QualityScore != want.Verdict.QualityScore {
		t.Errorf("QualityScore = %v, want %v", got.Verdict.QualityScore, want.Verdict.QualityScore)
	}
	if got.Verdict.XPAwarded != want.Verdict.XPAwarded {
		t.Errorf("XPAwarded = %d, want %d", got.Verdict.XPAwarded, want.Verdict.XPAwarded)
	}
	if got.Verdict.XPPenalty != want.Verdict.XPPenalty {
		t.Errorf("XPPenalty = %d, want %d", got.Verdict.XPPenalty, want.Verdict.XPPenalty)
	}
	if got.Verdict.LevelChange != want.Verdict.LevelChange {
		t.Errorf("LevelChange = %d, want %d", got.Verdict.LevelChange, want.Verdict.LevelChange)
	}
}

func TestBattleFromEntityStateRejectsStaleScalarPredicatesAndVerdictScoreAlias(t *testing.T) {
	t.Parallel()

	const entityID = "c360.prod.game.board1.battle.stale"
	got := BattleFromEntityState(&graph.EntityState{
		ID: entityID,
		Triples: []message.Triple{
			{Subject: entityID, Predicate: "battle.verdict.passed", Object: true},
			{Subject: entityID, Predicate: "battle.lifecycle.started_at", Object: "2026-07-25T12:00:00Z"},
			{Subject: entityID, Predicate: "battle.execution.loop_id", Object: "stale-loop"},
			{Subject: entityID, Predicate: "battle.verdict.quality_score", Object: 0.91},
			{Subject: entityID, Predicate: "battle.verdict.score", Object: 0.92},
			{Subject: entityID, Predicate: "battle.verdict.xp_awarded", Object: int64(140)},
			{Subject: entityID, Predicate: "battle.verdict.xp_penalty", Object: int64(12)},
			{Subject: entityID, Predicate: "battle.verdict.level_change", Object: 2},
			{Subject: entityID, Predicate: "battle.lifecycle.completed_at", Object: "2026-07-25T12:30:00Z"},
		},
	})

	if !got.StartedAt.IsZero() {
		t.Errorf("stale started_at changed StartedAt to %v", got.StartedAt)
	}
	if got.LoopID != "" {
		t.Errorf("stale loop_id changed LoopID to %q", got.LoopID)
	}
	if got.CompletedAt != nil {
		t.Errorf("stale completed_at changed CompletedAt to %v", got.CompletedAt)
	}
	if got.Verdict == nil || !got.Verdict.Passed {
		t.Fatal("valid battle.verdict.passed did not reconstruct the verdict")
	}
	if got.Verdict.QualityScore != 0 ||
		got.Verdict.XPAwarded != 0 ||
		got.Verdict.XPPenalty != 0 ||
		got.Verdict.LevelChange != 0 {
		t.Errorf("stale verdict scalars changed canonical-only reconstruction: %#v", got.Verdict)
	}
}

func TestBossBattleIndexedPredicateFamiliesRemainBlockedBySemStreams683(t *testing.T) {
	t.Parallel()

	// SemStreams #683 blocks a bounded framework primitive for indexed collections.
	// These runtime-formatted criteria/judge/result predicates are hidden from the
	// beta.158 static scanner and intentionally remain unchanged in this scalar packet.
	battle := canonicalScalarBattle()
	battle.Results = []domain.ReviewResult{{
		CriterionName: "correctness",
		JudgeID:       "judge-auto",
		Score:         0.9,
		Passed:        true,
		Reasoning:     "correct",
	}}
	triples := battle.Triples()

	for _, predicate := range []string{
		"battle.criteria.0.name",
		"battle.criteria.0.description",
		"battle.criteria.0.weight",
		"battle.criteria.0.threshold",
		"battle.judge.0.id",
		"battle.judge.0.type",
		"battle.result.0.criterion_name",
		"battle.result.0.judge_id",
		"battle.result.0.score",
		"battle.result.0.passed",
		"battle.result.0.reasoning",
	} {
		if _, ok := tripleByPredicate(triples, predicate); !ok {
			t.Errorf("indexed-family blocker predicate %q changed before SemStreams #683 is resolved", predicate)
		}
	}
}

func canonicalScalarBattle() *BossBattle {
	battle := newTestBattle()
	completed := time.Date(2026, time.July, 25, 12, 30, 0, 0, time.UTC)
	battle.LoopID = "battle-loop-canonical"
	battle.CompletedAt = &completed
	battle.Verdict = &domain.BattleVerdict{
		Passed:       true,
		QualityScore: 0.91,
		XPAwarded:    140,
		XPPenalty:    12,
		Feedback:     "canonical",
		LevelChange:  2,
	}
	return battle
}
