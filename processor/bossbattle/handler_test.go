package bossbattle

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/promptmanager"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
)

// =============================================================================
// CONFIG TESTS
// =============================================================================

func TestDefaultConfig_Values(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Org != "default" {
		t.Errorf("Org = %q, want %q", cfg.Org, "default")
	}
	if cfg.Platform != "local" {
		t.Errorf("Platform = %q, want %q", cfg.Platform, "local")
	}
	if cfg.Board != "main" {
		t.Errorf("Board = %q, want %q", cfg.Board, "main")
	}
	if cfg.DefaultTimeout != 5*time.Minute {
		t.Errorf("DefaultTimeout = %v, want %v", cfg.DefaultTimeout, 5*time.Minute)
	}
	if cfg.MaxConcurrent != 10 {
		t.Errorf("MaxConcurrent = %d, want %d", cfg.MaxConcurrent, 10)
	}
	if !cfg.AutoStartOnSubmit {
		t.Error("AutoStartOnSubmit should be true by default")
	}
	if !cfg.RequireReviewLevel {
		t.Error("RequireReviewLevel should be true by default")
	}
}

func TestDefaultConfig_Validate_Passes(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("DefaultConfig().Validate() = %v, want nil", err)
	}
}

func TestConfig_Validate_RequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name:    "empty org",
			mutate:  func(c *Config) { c.Org = "" },
			wantErr: "org is required",
		},
		{
			name:    "empty platform",
			mutate:  func(c *Config) { c.Platform = "" },
			wantErr: "platform is required",
		},
		{
			name:    "empty board",
			mutate:  func(c *Config) { c.Board = "" },
			wantErr: "board is required",
		},
		{
			name:    "zero default_timeout",
			mutate:  func(c *Config) { c.DefaultTimeout = 0 },
			wantErr: "default_timeout must be positive",
		},
		{
			name:    "negative default_timeout",
			mutate:  func(c *Config) { c.DefaultTimeout = -time.Second },
			wantErr: "default_timeout must be positive",
		},
		{
			name:    "zero max_concurrent",
			mutate:  func(c *Config) { c.MaxConcurrent = 0 },
			wantErr: "max_concurrent must be at least 1",
		},
		{
			name:    "negative max_concurrent",
			mutate:  func(c *Config) { c.MaxConcurrent = -1 },
			wantErr: "max_concurrent must be at least 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.mutate(&cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("Validate() returned nil, want error containing %q", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("Validate() error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestConfig_ToBoardConfig(t *testing.T) {
	cfg := Config{
		Org:      "c360",
		Platform: "prod",
		Board:    "main",
	}

	bc := cfg.ToBoardConfig()

	if bc.Org != cfg.Org {
		t.Errorf("BoardConfig.Org = %q, want %q", bc.Org, cfg.Org)
	}
	if bc.Platform != cfg.Platform {
		t.Errorf("BoardConfig.Platform = %q, want %q", bc.Platform, cfg.Platform)
	}
	if bc.Board != cfg.Board {
		t.Errorf("BoardConfig.Board = %q, want %q", bc.Board, cfg.Board)
	}
}

// =============================================================================
// BOSS BATTLE TRIPLES TESTS
// =============================================================================

// newTestBattle builds a full battle for use in triple-inspection tests.
func newTestBattle() *BossBattle {
	started := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	return &BossBattle{
		ID:        "c360.prod.game.board1.battle.abc123",
		QuestID:   "c360.prod.game.board1.quest.q001",
		AgentID:   "c360.prod.game.board1.agent.warrior",
		Level:     domain.ReviewStandard,
		Status:    domain.BattleActive,
		StartedAt: started,
		Criteria: []domain.ReviewCriterion{
			{Name: "correctness", Description: "Is it correct?", Weight: 0.5, Threshold: 0.7},
			{Name: "completeness", Description: "Is it complete?", Weight: 0.5, Threshold: 0.6},
		},
		Judges: []domain.Judge{
			{ID: "judge-auto", Type: domain.JudgeAutomated, Config: map[string]any{}},
			{ID: "judge-llm-1", Type: domain.JudgeLLM, Config: map[string]any{}},
		},
	}
}

// tripleByPredicate finds the first triple with the given predicate.
func tripleByPredicate(triples []message.Triple, predicate string) (message.Triple, bool) {
	for _, tr := range triples {
		if tr.Predicate == predicate {
			return tr, true
		}
	}
	return message.Triple{}, false
}

func TestBossBattle_Triples_MinimalBattle(t *testing.T) {
	b := &BossBattle{
		ID:        "battle-min",
		QuestID:   "quest-1",
		AgentID:   "agent-1",
		Level:     domain.ReviewAuto,
		Status:    domain.BattleActive,
		StartedAt: time.Now(),
	}

	triples := b.Triples()

	if len(triples) == 0 {
		t.Fatal("Triples() returned empty slice for minimal battle")
	}

	// Required predicates must always appear.
	requiredPredicates := []string{
		"battle.assignment.quest",
		"battle.assignment.agent",
		"battle.status.state",
		"battle.review.level",
		"battle.lifecycle.started_at",
		"battle.criteria.count",
	}

	for _, pred := range requiredPredicates {
		if _, found := tripleByPredicate(triples, pred); !found {
			t.Errorf("missing required predicate %q in minimal battle triples", pred)
		}
	}
}

func TestBossBattle_Triples_EntityID(t *testing.T) {
	b := newTestBattle()
	triples := b.Triples()

	for _, tr := range triples {
		if tr.Subject != string(b.ID) {
			t.Errorf("triple subject = %q, want %q (EntityID)", tr.Subject, string(b.ID))
		}
	}
}

func TestBossBattle_Triples_CorePredicates(t *testing.T) {
	b := newTestBattle()
	triples := b.Triples()

	tests := []struct {
		predicate string
		wantObj   any
	}{
		{"battle.assignment.quest", string(b.QuestID)},
		{"battle.assignment.agent", string(b.AgentID)},
		{"battle.status.state", string(b.Status)},
		{"battle.review.level", int(b.Level)},
		{"battle.criteria.count", len(b.Criteria)},
	}

	for _, tt := range tests {
		t.Run(tt.predicate, func(t *testing.T) {
			tr, found := tripleByPredicate(triples, tt.predicate)
			if !found {
				t.Fatalf("predicate %q not found in triples", tt.predicate)
			}
			if tr.Object != tt.wantObj {
				t.Errorf("Object = %v (%T), want %v (%T)", tr.Object, tr.Object, tt.wantObj, tt.wantObj)
			}
		})
	}
}

func TestBossBattle_Triples_Source(t *testing.T) {
	b := newTestBattle()
	triples := b.Triples()

	for _, tr := range triples {
		if tr.Source != "bossbattle" {
			t.Errorf("triple source = %q, want %q", tr.Source, "bossbattle")
		}
		if tr.Confidence != 1.0 {
			t.Errorf("triple confidence = %f, want 1.0", tr.Confidence)
		}
		if tr.Timestamp.IsZero() {
			t.Error("triple timestamp should not be zero")
		}
	}
}

func TestBossBattle_Triples_StartedAt(t *testing.T) {
	b := newTestBattle()
	triples := b.Triples()

	tr, found := tripleByPredicate(triples, "battle.lifecycle.started_at")
	if !found {
		t.Fatal("battle.lifecycle.started_at predicate not found")
	}
	got, ok := tr.Object.(string)
	if !ok {
		t.Fatalf("started_at Object type = %T, want string", tr.Object)
	}
	want := b.StartedAt.Format(time.RFC3339)
	if got != want {
		t.Errorf("started_at = %q, want %q", got, want)
	}
}

func TestBossBattle_Triples_Judges(t *testing.T) {
	b := newTestBattle()
	triples := b.Triples()

	// Expects indexed judge predicates for each judge.
	for i, judge := range b.Judges {
		idPred := fmt.Sprintf("battle.judge.%d.id", i)
		typePred := fmt.Sprintf("battle.judge.%d.type", i)

		idTriple, idFound := tripleByPredicate(triples, idPred)
		if !idFound {
			t.Errorf("judge[%d] id predicate %q not found", i, idPred)
			continue
		}
		if idTriple.Object != judge.ID {
			t.Errorf("judge[%d] id = %v, want %q", i, idTriple.Object, judge.ID)
		}

		typeTriple, typeFound := tripleByPredicate(triples, typePred)
		if !typeFound {
			t.Errorf("judge[%d] type predicate %q not found", i, typePred)
			continue
		}
		if typeTriple.Object != string(judge.Type) {
			t.Errorf("judge[%d] type = %v, want %q", i, typeTriple.Object, string(judge.Type))
		}
	}
}

func TestBossBattle_Triples_NoJudges(t *testing.T) {
	b := &BossBattle{
		ID:        "battle-nojudge",
		QuestID:   "q1",
		AgentID:   "a1",
		Level:     domain.ReviewAuto,
		Status:    domain.BattleActive,
		StartedAt: time.Now(),
	}
	triples := b.Triples()

	// No judge predicates should appear when Judges is nil.
	for _, tr := range triples {
		if len(tr.Predicate) >= 12 && tr.Predicate[:12] == "battle.judge" {
			t.Errorf("unexpected judge predicate %q when no judges set", tr.Predicate)
		}
	}
}

func TestBossBattle_Triples_Verdict(t *testing.T) {
	b := newTestBattle()
	b.Verdict = &domain.BattleVerdict{
		Passed:       true,
		QualityScore: 0.85,
		XPAwarded:    150,
		XPPenalty:    0,
		Feedback:     "Well done",
	}
	triples := b.Triples()

	tests := []struct {
		predicate string
		wantObj   any
	}{
		{"battle.verdict.passed", b.Verdict.Passed},
		{"battle.verdict.quality_score", b.Verdict.QualityScore},
		{"battle.verdict.xp_awarded", b.Verdict.XPAwarded},
		{"battle.verdict.xp_penalty", b.Verdict.XPPenalty},
		{"battle.verdict.feedback", b.Verdict.Feedback},
	}

	for _, tt := range tests {
		t.Run(tt.predicate, func(t *testing.T) {
			tr, found := tripleByPredicate(triples, tt.predicate)
			if !found {
				t.Fatalf("predicate %q not found in triples", tt.predicate)
			}
			if tr.Object != tt.wantObj {
				t.Errorf("Object = %v, want %v", tr.Object, tt.wantObj)
			}
		})
	}
}

func TestBossBattle_Triples_VerdictNoFeedback(t *testing.T) {
	b := newTestBattle()
	b.Verdict = &domain.BattleVerdict{
		Passed:       false,
		QualityScore: 0.4,
		Feedback:     "", // empty — should be omitted
	}
	triples := b.Triples()

	// battle.verdict.feedback must not appear when feedback is empty.
	for _, tr := range triples {
		if tr.Predicate == "battle.verdict.feedback" {
			t.Error("battle.verdict.feedback should not be emitted when feedback is empty")
		}
	}

	// Other verdict predicates must still appear.
	if _, found := tripleByPredicate(triples, "battle.verdict.passed"); !found {
		t.Error("battle.verdict.passed should be present even when feedback is empty")
	}
}

func TestBossBattle_Triples_NoVerdict(t *testing.T) {
	b := newTestBattle()
	b.Verdict = nil
	triples := b.Triples()

	verdictPredicates := []string{
		"battle.verdict.passed",
		"battle.verdict.quality_score",
		"battle.verdict.xp_awarded",
		"battle.verdict.xp_penalty",
		"battle.verdict.feedback",
	}

	for _, pred := range verdictPredicates {
		if _, found := tripleByPredicate(triples, pred); found {
			t.Errorf("predicate %q should not appear when Verdict is nil", pred)
		}
	}
}

func TestBossBattle_Triples_CompletedAt(t *testing.T) {
	b := newTestBattle()
	completed := time.Date(2026, 1, 15, 11, 0, 0, 0, time.UTC)
	b.CompletedAt = &completed
	triples := b.Triples()

	tr, found := tripleByPredicate(triples, "battle.lifecycle.completed_at")
	if !found {
		t.Fatal("battle.lifecycle.completed_at not found when CompletedAt is set")
	}
	got, ok := tr.Object.(string)
	if !ok {
		t.Fatalf("completed_at Object type = %T, want string", tr.Object)
	}
	want := completed.Format(time.RFC3339)
	if got != want {
		t.Errorf("completed_at = %q, want %q", got, want)
	}
}

func TestBossBattle_Triples_NoCompletedAt(t *testing.T) {
	b := newTestBattle()
	b.CompletedAt = nil
	triples := b.Triples()

	if _, found := tripleByPredicate(triples, "battle.lifecycle.completed_at"); found {
		t.Error("battle.lifecycle.completed_at should not appear when CompletedAt is nil")
	}
}

func TestBossBattle_Triples_Results(t *testing.T) {
	b := newTestBattle()
	b.Results = []domain.ReviewResult{
		{CriterionName: "correctness", Score: 0.9, Passed: true, Reasoning: "ok", JudgeID: "j1"},
		{CriterionName: "completeness", Score: 0.5, Passed: false, Reasoning: "incomplete", JudgeID: "j1"},
	}
	triples := b.Triples()

	countTriple, found := tripleByPredicate(triples, "battle.results.count")
	if !found {
		t.Fatal("battle.results.count not found")
	}
	if countTriple.Object != len(b.Results) {
		t.Errorf("battle.results.count = %v, want %d", countTriple.Object, len(b.Results))
	}

	passedTriple, found := tripleByPredicate(triples, "battle.results.passed")
	if !found {
		t.Fatal("battle.results.passed not found")
	}
	// One result passed, one failed.
	if passedTriple.Object != 1 {
		t.Errorf("battle.results.passed = %v, want 1", passedTriple.Object)
	}
}

func TestBossBattle_Triples_ResultsAllPassed(t *testing.T) {
	b := newTestBattle()
	b.Results = []domain.ReviewResult{
		{CriterionName: "correctness", Score: 0.9, Passed: true},
		{CriterionName: "completeness", Score: 0.8, Passed: true},
	}
	triples := b.Triples()

	passedTriple, found := tripleByPredicate(triples, "battle.results.passed")
	if !found {
		t.Fatal("battle.results.passed not found")
	}
	if passedTriple.Object != 2 {
		t.Errorf("battle.results.passed = %v, want 2", passedTriple.Object)
	}
}

func TestBossBattle_Triples_NoResults(t *testing.T) {
	b := newTestBattle()
	b.Results = nil
	triples := b.Triples()

	for _, tr := range triples {
		if tr.Predicate == "battle.results.count" || tr.Predicate == "battle.results.passed" {
			t.Errorf("predicate %q should not appear when Results is nil", tr.Predicate)
		}
	}
}

func TestBossBattle_EntityID(t *testing.T) {
	b := &BossBattle{ID: "c360.prod.game.board1.battle.abc"}
	if got := b.EntityID(); got != string(b.ID) {
		t.Errorf("EntityID() = %q, want %q", got, string(b.ID))
	}
}

// battleToEntityState converts a BossBattle to graph.EntityState via Triples()
// for round-trip reconstruction tests.
func battleToEntityState(b *BossBattle) *graph.EntityState {
	return &graph.EntityState{
		ID:      b.EntityID(),
		Triples: b.Triples(),
	}
}

func TestBossBattle_LoopID_RoundTrip(t *testing.T) {
	b := newTestBattle()
	b.LoopID = "battle-c360-prod-game-board1-battle-abc123-deadbeef"
	triples := b.Triples()

	// Verify the triple is emitted
	found := false
	for _, tr := range triples {
		if tr.Predicate == "battle.execution.loop_id" {
			found = true
			if tr.Object != b.LoopID {
				t.Errorf("loop_id triple object = %v, want %q", tr.Object, b.LoopID)
			}
		}
	}
	if !found {
		t.Error("battle.execution.loop_id triple not found")
	}

	// Verify round-trip through reconstruction
	entity := battleToEntityState(b)
	reconstructed := BattleFromEntityState(entity)
	if reconstructed.LoopID != b.LoopID {
		t.Errorf("reconstructed LoopID = %q, want %q", reconstructed.LoopID, b.LoopID)
	}
}

func TestBossBattle_LoopID_EmptyNotEmitted(t *testing.T) {
	b := newTestBattle()
	b.LoopID = "" // heuristic evaluator — no trajectory
	triples := b.Triples()

	for _, tr := range triples {
		if tr.Predicate == "battle.execution.loop_id" {
			t.Error("battle.execution.loop_id should not be emitted when LoopID is empty")
		}
	}
}

func TestBossBattle_JudgeRoundTrip(t *testing.T) {
	b := newTestBattle()
	// newTestBattle has 2 judges: judge-auto (Automated) and judge-llm-1 (LLM)

	entity := battleToEntityState(b)
	reconstructed := BattleFromEntityState(entity)

	if len(reconstructed.Judges) != len(b.Judges) {
		t.Fatalf("reconstructed %d judges, want %d", len(reconstructed.Judges), len(b.Judges))
	}
	for i, j := range reconstructed.Judges {
		if j.ID != b.Judges[i].ID {
			t.Errorf("judge[%d].ID = %q, want %q", i, j.ID, b.Judges[i].ID)
		}
		if j.Type != b.Judges[i].Type {
			t.Errorf("judge[%d].Type = %q, want %q", i, j.Type, b.Judges[i].Type)
		}
	}
}

func TestBossBattle_JudgeLegacyFallback(t *testing.T) {
	// Simulate legacy data with unindexed "battle.judge.id" predicate
	entity := &graph.EntityState{
		ID: "c360.prod.game.board1.battle.legacy",
		Triples: []message.Triple{
			{Subject: "c360.prod.game.board1.battle.legacy", Predicate: "battle.status.state", Object: "active"},
			{Subject: "c360.prod.game.board1.battle.legacy", Predicate: "battle.judge.id", Object: "judge-old"},
		},
	}
	b := BattleFromEntityState(entity)
	if len(b.Judges) != 1 {
		t.Fatalf("got %d judges, want 1", len(b.Judges))
	}
	if b.Judges[0].ID != "judge-old" {
		t.Errorf("judge ID = %q, want %q", b.Judges[0].ID, "judge-old")
	}
}

func TestBossBattle_JudgeNoJudges(t *testing.T) {
	entity := &graph.EntityState{
		ID: "c360.prod.game.board1.battle.nojudge",
		Triples: []message.Triple{
			{Subject: "c360.prod.game.board1.battle.nojudge", Predicate: "battle.status.state", Object: "active"},
		},
	}
	b := BattleFromEntityState(entity)
	if len(b.Judges) != 0 {
		t.Errorf("got %d judges, want 0", len(b.Judges))
	}
}

// =============================================================================
// DEFAULT BATTLE EVALUATOR TESTS
// =============================================================================

// newEvaluatorBattle builds a battle for evaluator unit tests using the
// component's own defaultCriteria/defaultJudges to avoid duplication.
// When judges are explicitly supplied they override the defaults.
func newEvaluatorBattle(level domain.ReviewLevel, overrideJudges ...domain.Judge) *BossBattle {
	comp := &Component{config: &Config{}}
	criteria := comp.defaultCriteria(level)

	var judges []domain.Judge
	if len(overrideJudges) > 0 {
		judges = overrideJudges
	} else {
		// Use only automated judges so we never accidentally hit the human-judge path.
		for _, j := range comp.defaultJudges(level) {
			if j.Type == domain.JudgeAutomated {
				judges = append(judges, j)
			}
		}
	}

	return &BossBattle{
		ID:        domain.BattleID("battle-eval"),
		QuestID:   "quest-eval",
		AgentID:   "agent-eval",
		Level:     level,
		Status:    domain.BattleActive,
		Criteria:  criteria,
		Judges:    judges,
		StartedAt: time.Now(),
	}
}

func TestDefaultBattleEvaluator_Evaluate_WithOutput(t *testing.T) {
	e := NewDefaultBattleEvaluator()
	battle := newEvaluatorBattle(domain.ReviewStandard)

	result, err := e.Evaluate(context.Background(), battle, nil, "some output")
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if result == nil {
		t.Fatal("Evaluate() returned nil result")
	}
	if result.Pending {
		t.Error("Evaluate() should not be pending for automated-only judges")
	}
	// Score must be exactly 0.8 for non-nil output per current implementation.
	for i, r := range result.Results {
		if r.Score != 0.8 {
			t.Errorf("result[%d].Score = %f, want 0.8", i, r.Score)
		}
	}
}

func TestDefaultBattleEvaluator_Evaluate_WithoutOutput(t *testing.T) {
	e := NewDefaultBattleEvaluator()
	battle := newEvaluatorBattle(domain.ReviewStandard)

	result, err := e.Evaluate(context.Background(), battle, nil, nil)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	// Score must be 0.0 when output is nil.
	for i, r := range result.Results {
		if r.Score != 0.0 {
			t.Errorf("result[%d].Score = %f, want 0.0", i, r.Score)
		}
	}
}

func TestDefaultBattleEvaluator_Evaluate_ResultCount(t *testing.T) {
	e := NewDefaultBattleEvaluator()
	battle := newEvaluatorBattle(domain.ReviewStrict)

	result, err := e.Evaluate(context.Background(), battle, nil, "output")
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(result.Results) != len(battle.Criteria) {
		t.Errorf("len(Results) = %d, want %d (one per criterion)", len(result.Results), len(battle.Criteria))
	}
}

func TestDefaultBattleEvaluator_Evaluate_CriterionNames(t *testing.T) {
	e := NewDefaultBattleEvaluator()
	battle := newEvaluatorBattle(domain.ReviewStandard)

	result, err := e.Evaluate(context.Background(), battle, nil, "output")
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	for i, r := range result.Results {
		if r.CriterionName != battle.Criteria[i].Name {
			t.Errorf("result[%d].CriterionName = %q, want %q", i, r.CriterionName, battle.Criteria[i].Name)
		}
	}
}

func TestDefaultBattleEvaluator_Evaluate_PassedBasedOnThreshold(t *testing.T) {
	e := NewDefaultBattleEvaluator()

	// ReviewAuto has threshold 0.5 — score 0.8 (with output) must pass.
	battleAuto := newEvaluatorBattle(domain.ReviewAuto)
	resultAuto, err := e.Evaluate(context.Background(), battleAuto, nil, "output")
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	for i, r := range resultAuto.Results {
		if !r.Passed {
			t.Errorf("ReviewAuto result[%d] should pass (score 0.8 >= threshold 0.5)", i)
		}
	}

	// ReviewStrict has thresholds >= 0.6 — score 0.8 should still pass.
	battleStrict := newEvaluatorBattle(domain.ReviewStrict)
	resultStrict, err := e.Evaluate(context.Background(), battleStrict, nil, "output")
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	for i, r := range resultStrict.Results {
		if !r.Passed {
			t.Errorf("ReviewStrict result[%d] should pass (score 0.8 >= threshold)", i)
		}
	}
}

func TestDefaultBattleEvaluator_Evaluate_FailedWhenNoOutput(t *testing.T) {
	e := NewDefaultBattleEvaluator()

	// ReviewStrict has thresholds >= 0.6 — score 0.0 (no output) must fail.
	battle := newEvaluatorBattle(domain.ReviewStrict)
	result, err := e.Evaluate(context.Background(), battle, nil, nil)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	for i, r := range result.Results {
		if r.Passed {
			t.Errorf("result[%d] should fail with no output against strict thresholds", i)
		}
	}
}

func TestDefaultBattleEvaluator_Evaluate_WeightedScore(t *testing.T) {
	e := NewDefaultBattleEvaluator()

	// Build a battle with known weights to verify weighted score calculation.
	// With output=non-nil each criterion gets score 0.8:
	//   criterion-a: weight 0.6 → contribution 0.8 * 0.6 = 0.48
	//   criterion-b: weight 0.4 → contribution 0.8 * 0.4 = 0.32
	//   total = 0.80
	battle := &BossBattle{
		ID:        "battle-weighted",
		QuestID:   "q1",
		AgentID:   "a1",
		Level:     domain.ReviewStandard,
		Status:    domain.BattleActive,
		StartedAt: time.Now(),
		Criteria: []domain.ReviewCriterion{
			{Name: "criterion-a", Weight: 0.6, Threshold: 0.5},
			{Name: "criterion-b", Weight: 0.4, Threshold: 0.5},
		},
		Judges: []domain.Judge{
			{ID: "judge-auto", Type: domain.JudgeAutomated},
		},
	}

	result, err := e.Evaluate(context.Background(), battle, nil, "some output")
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	const wantScore = 0.8 // 0.8*0.6 + 0.8*0.4
	const epsilon = 1e-9
	diff := result.Verdict.QualityScore - wantScore
	if diff < -epsilon || diff > epsilon {
		t.Errorf("QualityScore = %f, want %f", result.Verdict.QualityScore, wantScore)
	}
}

func TestDefaultBattleEvaluator_Evaluate_AllPassedVerdictPassed(t *testing.T) {
	e := NewDefaultBattleEvaluator()
	// ReviewAuto threshold 0.5 — score 0.8 means all pass.
	battle := newEvaluatorBattle(domain.ReviewAuto)

	result, err := e.Evaluate(context.Background(), battle, nil, "output")
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !result.Verdict.Passed {
		t.Error("Verdict.Passed should be true when all criteria pass")
	}
}

func TestDefaultBattleEvaluator_Evaluate_AnyFailedVerdictFailed(t *testing.T) {
	e := NewDefaultBattleEvaluator()
	// ReviewStrict has threshold 0.8 for correctness — score 0.0 (no output) fails.
	battle := newEvaluatorBattle(domain.ReviewStrict)

	result, err := e.Evaluate(context.Background(), battle, nil, nil)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if result.Verdict.Passed {
		t.Error("Verdict.Passed should be false when any criterion fails")
	}
}

func TestDefaultBattleEvaluator_Evaluate_HumanJudgePending(t *testing.T) {
	e := NewDefaultBattleEvaluator()

	humanJudge := domain.Judge{ID: "judge-human", Type: domain.JudgeHuman}
	battle := newEvaluatorBattle(domain.ReviewHuman, humanJudge)

	result, err := e.Evaluate(context.Background(), battle, nil, "output")
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !result.Pending {
		t.Error("Evaluate() should be pending when a human judge is present")
	}
	if result.PendingJudge != humanJudge.ID {
		t.Errorf("PendingJudge = %q, want %q", result.PendingJudge, humanJudge.ID)
	}
}

func TestDefaultBattleEvaluator_Evaluate_HumanJudgePendingVerdictZero(t *testing.T) {
	e := NewDefaultBattleEvaluator()

	// When pending the verdict struct should be its zero value (not filled in).
	humanJudge := domain.Judge{ID: "judge-human-2", Type: domain.JudgeHuman}
	battle := newEvaluatorBattle(domain.ReviewHuman, humanJudge)

	result, err := e.Evaluate(context.Background(), battle, nil, "output")
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if result.Verdict.Passed {
		t.Error("Verdict.Passed should be false (zero value) for pending human review")
	}
	if result.Verdict.QualityScore != 0 {
		t.Errorf("Verdict.QualityScore = %f, want 0 for pending human review", result.Verdict.QualityScore)
	}
}

func TestDefaultBattleEvaluator_Evaluate_CancelledContext(t *testing.T) {
	e := NewDefaultBattleEvaluator()
	battle := newEvaluatorBattle(domain.ReviewStandard)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately before calling Evaluate.

	_, err := e.Evaluate(ctx, battle, nil, "output")
	if err == nil {
		t.Error("Evaluate() should return error for cancelled context")
	}
	if err != context.Canceled {
		t.Errorf("Evaluate() error = %v, want context.Canceled", err)
	}
}

func TestDefaultBattleEvaluator_Evaluate_VerdictFeedback(t *testing.T) {
	e := NewDefaultBattleEvaluator()
	battle := newEvaluatorBattle(domain.ReviewAuto)

	result, err := e.Evaluate(context.Background(), battle, nil, "output")
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if result.Verdict.Feedback == "" {
		t.Error("Verdict.Feedback should be non-empty for completed evaluation")
	}
}

func TestDefaultBattleEvaluator_Evaluate_JudgeIDSet(t *testing.T) {
	e := NewDefaultBattleEvaluator()
	battle := newEvaluatorBattle(domain.ReviewAuto)

	result, err := e.Evaluate(context.Background(), battle, nil, "output")
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	for i, r := range result.Results {
		if r.JudgeID == "" {
			t.Errorf("result[%d].JudgeID is empty", i)
		}
	}
}

func TestDefaultBattleEvaluator_Evaluate_EmptyCriteria(t *testing.T) {
	e := NewDefaultBattleEvaluator()
	battle := &BossBattle{
		ID:        "battle-nocrit",
		QuestID:   "q1",
		AgentID:   "a1",
		Level:     domain.ReviewAuto,
		Status:    domain.BattleActive,
		StartedAt: time.Now(),
		Criteria:  nil,
		Judges:    []domain.Judge{{ID: "judge-auto", Type: domain.JudgeAutomated}},
	}

	result, err := e.Evaluate(context.Background(), battle, nil, "output")
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(result.Results) != 0 {
		t.Errorf("len(Results) = %d, want 0 for empty criteria", len(result.Results))
	}
	// allPassed stays true when there are no criteria (no failures accumulated).
	if !result.Verdict.Passed {
		t.Error("Verdict.Passed should be true when no criteria exist (vacuously all passed)")
	}
}

// =============================================================================
// DEFAULT CRITERIA TESTS
// =============================================================================

func TestDefaultCriteria_ReviewAuto(t *testing.T) {
	comp := &Component{config: &Config{}}
	criteria := comp.defaultCriteria(domain.ReviewAuto)

	if len(criteria) != 1 {
		t.Fatalf("ReviewAuto: len(criteria) = %d, want 1", len(criteria))
	}
	if criteria[0].Name != "acceptance" {
		t.Errorf("ReviewAuto criteria[0].Name = %q, want %q", criteria[0].Name, "acceptance")
	}
	if criteria[0].Weight != 1.0 {
		t.Errorf("ReviewAuto criteria[0].Weight = %f, want 1.0", criteria[0].Weight)
	}
}

func TestDefaultCriteria_ReviewStandard(t *testing.T) {
	comp := &Component{config: &Config{}}
	criteria := comp.defaultCriteria(domain.ReviewStandard)

	if len(criteria) != 3 {
		t.Fatalf("ReviewStandard: len(criteria) = %d, want 3", len(criteria))
	}

	names := []string{"correctness", "completeness", "quality"}
	for i, name := range names {
		if criteria[i].Name != name {
			t.Errorf("ReviewStandard criteria[%d].Name = %q, want %q", i, criteria[i].Name, name)
		}
	}
}

func TestDefaultCriteria_ReviewStrict(t *testing.T) {
	comp := &Component{config: &Config{}}
	criteria := comp.defaultCriteria(domain.ReviewStrict)

	if len(criteria) != 4 {
		t.Fatalf("ReviewStrict: len(criteria) = %d, want 4", len(criteria))
	}

	names := []string{"correctness", "completeness", "quality", "style"}
	for i, name := range names {
		if criteria[i].Name != name {
			t.Errorf("ReviewStrict criteria[%d].Name = %q, want %q", i, criteria[i].Name, name)
		}
	}
}

func TestDefaultCriteria_ReviewHuman(t *testing.T) {
	comp := &Component{config: &Config{}}
	criteria := comp.defaultCriteria(domain.ReviewHuman)

	if len(criteria) != 4 {
		t.Fatalf("ReviewHuman: len(criteria) = %d, want 4", len(criteria))
	}

	names := []string{"correctness", "completeness", "quality", "style"}
	for i, name := range names {
		if criteria[i].Name != name {
			t.Errorf("ReviewHuman criteria[%d].Name = %q, want %q", i, criteria[i].Name, name)
		}
	}
}

func TestDefaultCriteria_WeightsSumToOne(t *testing.T) {
	comp := &Component{config: &Config{}}

	levels := []domain.ReviewLevel{
		domain.ReviewAuto,
		domain.ReviewStandard,
		domain.ReviewStrict,
		domain.ReviewHuman,
	}

	for _, level := range levels {
		criteria := comp.defaultCriteria(level)
		total := 0.0
		for _, c := range criteria {
			total += c.Weight
		}
		const epsilon = 1e-9
		if total < 1.0-epsilon || total > 1.0+epsilon {
			t.Errorf("ReviewLevel %d: weights sum = %f, want 1.0", level, total)
		}
	}
}

func TestDefaultCriteria_ThresholdsValid(t *testing.T) {
	comp := &Component{config: &Config{}}

	levels := []domain.ReviewLevel{
		domain.ReviewAuto,
		domain.ReviewStandard,
		domain.ReviewStrict,
		domain.ReviewHuman,
	}

	for _, level := range levels {
		criteria := comp.defaultCriteria(level)
		for _, c := range criteria {
			if c.Threshold < 0 || c.Threshold > 1 {
				t.Errorf("ReviewLevel %d criterion %q: threshold %f out of [0,1]", level, c.Name, c.Threshold)
			}
			if c.Weight <= 0 {
				t.Errorf("ReviewLevel %d criterion %q: weight %f must be positive", level, c.Name, c.Weight)
			}
		}
	}
}

// =============================================================================
// DEFAULT JUDGES TESTS
// =============================================================================

func TestDefaultJudges_ReviewAuto(t *testing.T) {
	comp := &Component{config: &Config{}}
	judges := comp.defaultJudges(domain.ReviewAuto)

	if len(judges) != 1 {
		t.Fatalf("ReviewAuto: len(judges) = %d, want 1", len(judges))
	}
	if judges[0].Type != domain.JudgeAutomated {
		t.Errorf("ReviewAuto judges[0].Type = %q, want %q", judges[0].Type, domain.JudgeAutomated)
	}
}

func TestDefaultJudges_ReviewStandard(t *testing.T) {
	comp := &Component{config: &Config{}}
	judges := comp.defaultJudges(domain.ReviewStandard)

	if len(judges) != 2 {
		t.Fatalf("ReviewStandard: len(judges) = %d, want 2", len(judges))
	}

	wantTypes := []domain.JudgeType{domain.JudgeAutomated, domain.JudgeLLM}
	for i, want := range wantTypes {
		if judges[i].Type != want {
			t.Errorf("ReviewStandard judges[%d].Type = %q, want %q", i, judges[i].Type, want)
		}
	}
}

func TestDefaultJudges_ReviewStrict(t *testing.T) {
	comp := &Component{config: &Config{}}
	judges := comp.defaultJudges(domain.ReviewStrict)

	if len(judges) != 3 {
		t.Fatalf("ReviewStrict: len(judges) = %d, want 3", len(judges))
	}

	wantTypes := []domain.JudgeType{domain.JudgeAutomated, domain.JudgeLLM, domain.JudgeLLM}
	for i, want := range wantTypes {
		if judges[i].Type != want {
			t.Errorf("ReviewStrict judges[%d].Type = %q, want %q", i, judges[i].Type, want)
		}
	}
}

func TestDefaultJudges_ReviewHuman(t *testing.T) {
	comp := &Component{config: &Config{}}
	judges := comp.defaultJudges(domain.ReviewHuman)

	if len(judges) != 3 {
		t.Fatalf("ReviewHuman: len(judges) = %d, want 3", len(judges))
	}

	// Must include exactly one human judge.
	humanCount := 0
	for _, j := range judges {
		if j.Type == domain.JudgeHuman {
			humanCount++
		}
	}
	if humanCount != 1 {
		t.Errorf("ReviewHuman: humanCount = %d, want 1", humanCount)
	}
}

func TestDefaultJudges_AllHaveIDs(t *testing.T) {
	comp := &Component{config: &Config{}}

	levels := []domain.ReviewLevel{
		domain.ReviewAuto,
		domain.ReviewStandard,
		domain.ReviewStrict,
		domain.ReviewHuman,
	}

	for _, level := range levels {
		judges := comp.defaultJudges(level)
		for i, j := range judges {
			if j.ID == "" {
				t.Errorf("ReviewLevel %d judges[%d].ID is empty", level, i)
			}
		}
	}
}

// =============================================================================
// PAYLOAD VALIDATE TESTS
// =============================================================================

func TestBattleRetreatPayload_Validate_Valid(t *testing.T) {
	p := &BattleRetreatPayload{
		Battle:      BossBattle{ID: "battle-1"},
		Reason:      "agent gave up",
		RetreatedAt: time.Now(),
	}
	if err := p.Validate(); err != nil {
		t.Errorf("Validate() on valid payload = %v, want nil", err)
	}
}

func TestBattleRetreatPayload_Validate_MissingBattleID(t *testing.T) {
	p := &BattleRetreatPayload{
		Battle:      BossBattle{ID: ""}, // empty ID
		Reason:      "some reason",
		RetreatedAt: time.Now(),
	}
	err := p.Validate()
	if err == nil {
		t.Fatal("Validate() should return error when BattleID is empty")
	}
	if err.Error() != "battle_id required" {
		t.Errorf("Validate() error = %q, want %q", err.Error(), "battle_id required")
	}
}

func TestBattleRetreatPayload_Validate_MissingRetreatedAt(t *testing.T) {
	p := &BattleRetreatPayload{
		Battle:      BossBattle{ID: "battle-2"},
		Reason:      "some reason",
		RetreatedAt: time.Time{}, // zero value
	}
	err := p.Validate()
	if err == nil {
		t.Fatal("Validate() should return error when RetreatedAt is zero")
	}
	if err.Error() != "retreated_at required" {
		t.Errorf("Validate() error = %q, want %q", err.Error(), "retreated_at required")
	}
}

func TestBattleRetreatPayload_EntityID(t *testing.T) {
	p := &BattleRetreatPayload{
		Battle: BossBattle{ID: "c360.prod.game.b1.battle.xyz"},
	}
	if got := p.EntityID(); got != string(p.Battle.ID) {
		t.Errorf("EntityID() = %q, want %q", got, string(p.Battle.ID))
	}
}

func TestBattleRetreatPayload_Triples_ContainsBattleAndRetreatPredicates(t *testing.T) {
	retreatedAt := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	p := &BattleRetreatPayload{
		Battle: BossBattle{
			ID:        "battle-retreat-1",
			QuestID:   "q-retreat",
			AgentID:   "a-retreat",
			Level:     domain.ReviewStandard,
			Status:    domain.BattleRetreat,
			StartedAt: retreatedAt.Add(-5 * time.Minute),
		},
		Reason:      "timeout exceeded",
		RetreatedAt: retreatedAt,
	}

	triples := p.Triples()

	if len(triples) == 0 {
		t.Fatal("Triples() returned empty slice")
	}

	// Retreat-specific predicates.
	outcomeTriple, found := tripleByPredicate(triples, "battle.outcome")
	if !found {
		t.Fatal("battle.outcome not found in retreat payload triples")
	}
	if outcomeTriple.Object != "retreat" {
		t.Errorf("battle.outcome = %v, want %q", outcomeTriple.Object, "retreat")
	}

	reasonTriple, found := tripleByPredicate(triples, "battle.retreat.reason")
	if !found {
		t.Fatal("battle.retreat.reason not found in retreat payload triples")
	}
	if reasonTriple.Object != p.Reason {
		t.Errorf("battle.retreat.reason = %v, want %q", reasonTriple.Object, p.Reason)
	}

	// Must also include the underlying battle triples.
	if _, found := tripleByPredicate(triples, "battle.assignment.quest"); !found {
		t.Error("battle.assignment.quest not found — BossBattle.Triples() not included")
	}
}

func TestBattleRetreatPayload_Schema(t *testing.T) {
	p := &BattleRetreatPayload{}
	schema := p.Schema()

	if schema.Domain != "semdragons" {
		t.Errorf("Schema.Domain = %q, want %q", schema.Domain, "semdragons")
	}
	if schema.Category != "battle.retreat" {
		t.Errorf("Schema.Category = %q, want %q", schema.Category, "battle.retreat")
	}
	if schema.Version != "v1" {
		t.Errorf("Schema.Version = %q, want %q", schema.Version, "v1")
	}
}

// =============================================================================
// SHOULD AUTO PASS TESTS
// =============================================================================

func TestShouldAutoPass_NilCatalog(t *testing.T) {
	comp := &Component{config: &Config{}, catalog: nil}
	quest := &domain.Quest{Difficulty: domain.DifficultyTrivial}

	if comp.shouldAutoPass(quest) {
		t.Error("shouldAutoPass() = true, want false when catalog is nil")
	}
}

func TestShouldAutoPass_NilReviewConfig(t *testing.T) {
	comp := &Component{
		config:  &Config{},
		catalog: &promptmanager.DomainCatalog{ReviewConfig: nil},
	}
	quest := &domain.Quest{Difficulty: domain.DifficultyTrivial}

	if comp.shouldAutoPass(quest) {
		t.Error("shouldAutoPass() = true, want false when ReviewConfig is nil")
	}
}

func TestShouldAutoPass_EmptyAutoPassDifficulties(t *testing.T) {
	comp := &Component{
		config: &Config{},
		catalog: &promptmanager.DomainCatalog{
			ReviewConfig: &promptmanager.ReviewConfig{
				AutoPassDifficulties: []domain.QuestDifficulty{},
			},
		},
	}
	quest := &domain.Quest{Difficulty: domain.DifficultyTrivial}

	if comp.shouldAutoPass(quest) {
		t.Error("shouldAutoPass() = true, want false for empty AutoPassDifficulties")
	}
}

func TestShouldAutoPass_DifficultyMatches(t *testing.T) {
	tests := []struct {
		name            string
		autoPassList    []domain.QuestDifficulty
		questDifficulty domain.QuestDifficulty
		want            bool
	}{
		{
			name:            "trivial in list",
			autoPassList:    []domain.QuestDifficulty{domain.DifficultyTrivial, domain.DifficultyEasy},
			questDifficulty: domain.DifficultyTrivial,
			want:            true,
		},
		{
			name:            "easy in list",
			autoPassList:    []domain.QuestDifficulty{domain.DifficultyTrivial, domain.DifficultyEasy},
			questDifficulty: domain.DifficultyEasy,
			want:            true,
		},
		{
			name:            "single-entry list matches",
			autoPassList:    []domain.QuestDifficulty{domain.DifficultyModerate},
			questDifficulty: domain.DifficultyModerate,
			want:            true,
		},
		{
			name:            "difficulty not in list",
			autoPassList:    []domain.QuestDifficulty{domain.DifficultyTrivial, domain.DifficultyEasy},
			questDifficulty: domain.DifficultyHard,
			want:            false,
		},
		{
			name:            "epic not in trivial-only list",
			autoPassList:    []domain.QuestDifficulty{domain.DifficultyTrivial},
			questDifficulty: domain.DifficultyEpic,
			want:            false,
		},
		{
			name:            "legendary not in any standard list",
			autoPassList:    []domain.QuestDifficulty{domain.DifficultyTrivial, domain.DifficultyEasy, domain.DifficultyModerate},
			questDifficulty: domain.DifficultyLegendary,
			want:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp := &Component{
				config: &Config{},
				catalog: &promptmanager.DomainCatalog{
					ReviewConfig: &promptmanager.ReviewConfig{
						AutoPassDifficulties: tt.autoPassList,
					},
				},
			}
			quest := &domain.Quest{Difficulty: tt.questDifficulty}

			got := comp.shouldAutoPass(quest)
			if got != tt.want {
				t.Errorf("shouldAutoPass() = %v, want %v (difficulty=%v, list=%v)",
					got, tt.want, tt.questDifficulty, tt.autoPassList)
			}
		})
	}
}

// =============================================================================
// RESOLVE CRITERIA TESTS
// =============================================================================

func TestResolveCriteria_NilCatalog_FallsBackToDefaults(t *testing.T) {
	comp := &Component{config: &Config{}, catalog: nil}

	for _, level := range []domain.ReviewLevel{
		domain.ReviewAuto,
		domain.ReviewStandard,
		domain.ReviewStrict,
		domain.ReviewHuman,
	} {
		got := comp.resolveCriteria(level)
		want := comp.defaultCriteria(level)

		if len(got) != len(want) {
			t.Errorf("level %d: resolveCriteria() len=%d, want %d (nil catalog fallback)",
				level, len(got), len(want))
			continue
		}
		for i := range got {
			if got[i].Name != want[i].Name {
				t.Errorf("level %d criteria[%d].Name = %q, want %q", level, i, got[i].Name, want[i].Name)
			}
		}
	}
}

func TestResolveCriteria_NilReviewConfig_FallsBackToDefaults(t *testing.T) {
	comp := &Component{
		config:  &Config{},
		catalog: &promptmanager.DomainCatalog{ReviewConfig: nil},
	}

	got := comp.resolveCriteria(domain.ReviewStandard)
	want := comp.defaultCriteria(domain.ReviewStandard)

	if len(got) != len(want) {
		t.Fatalf("resolveCriteria() len=%d, want %d (nil ReviewConfig fallback)", len(got), len(want))
	}
}

func TestResolveCriteria_EmptyCriteria_FallsBackToDefaults(t *testing.T) {
	comp := &Component{
		config: &Config{},
		catalog: &promptmanager.DomainCatalog{
			ReviewConfig: &promptmanager.ReviewConfig{
				DefaultCriteria: []domain.ReviewCriterion{},
			},
		},
	}

	got := comp.resolveCriteria(domain.ReviewStandard)
	want := comp.defaultCriteria(domain.ReviewStandard)

	if len(got) != len(want) {
		t.Fatalf("resolveCriteria() len=%d, want %d (empty criteria fallback)", len(got), len(want))
	}
}

func TestResolveCriteria_CatalogCriteriaReturned(t *testing.T) {
	catalogCriteria := []domain.ReviewCriterion{
		{Name: "domain-check", Description: "Domain-specific check", Weight: 0.6, Threshold: 0.75},
		{Name: "domain-style", Description: "Domain-specific style", Weight: 0.4, Threshold: 0.65},
	}
	comp := &Component{
		config: &Config{},
		catalog: &promptmanager.DomainCatalog{
			ReviewConfig: &promptmanager.ReviewConfig{
				DefaultCriteria: catalogCriteria,
			},
		},
	}

	got := comp.resolveCriteria(domain.ReviewStandard)

	if len(got) != len(catalogCriteria) {
		t.Fatalf("resolveCriteria() len=%d, want %d", len(got), len(catalogCriteria))
	}
	for i, want := range catalogCriteria {
		if got[i].Name != want.Name {
			t.Errorf("criteria[%d].Name = %q, want %q", i, got[i].Name, want.Name)
		}
		if got[i].Weight != want.Weight {
			t.Errorf("criteria[%d].Weight = %f, want %f", i, got[i].Weight, want.Weight)
		}
		if got[i].Threshold != want.Threshold {
			t.Errorf("criteria[%d].Threshold = %f, want %f", i, got[i].Threshold, want.Threshold)
		}
	}
}

func TestResolveCriteria_CatalogCriteriaReturnsCopy(t *testing.T) {
	// Mutating the returned slice must not affect the catalog's original slice.
	catalogCriteria := []domain.ReviewCriterion{
		{Name: "original", Weight: 0.5, Threshold: 0.5},
	}
	comp := &Component{
		config: &Config{},
		catalog: &promptmanager.DomainCatalog{
			ReviewConfig: &promptmanager.ReviewConfig{
				DefaultCriteria: catalogCriteria,
			},
		},
	}

	got := comp.resolveCriteria(domain.ReviewStandard)
	got[0].Name = "mutated"

	// The catalog's original slice must be untouched.
	if comp.catalog.ReviewConfig.DefaultCriteria[0].Name != "original" {
		t.Error("resolveCriteria() returned the original slice instead of a copy; mutation affected the catalog")
	}
}

func TestResolveCriteria_LevelIgnoredWhenCatalogHasCriteria(t *testing.T) {
	// When the catalog provides criteria the level parameter is irrelevant —
	// the same criteria must be returned regardless of which level is passed.
	catalogCriteria := []domain.ReviewCriterion{
		{Name: "catalog-criterion", Weight: 1.0, Threshold: 0.6},
	}
	comp := &Component{
		config: &Config{},
		catalog: &promptmanager.DomainCatalog{
			ReviewConfig: &promptmanager.ReviewConfig{
				DefaultCriteria: catalogCriteria,
			},
		},
	}

	for _, level := range []domain.ReviewLevel{
		domain.ReviewAuto,
		domain.ReviewStandard,
		domain.ReviewStrict,
		domain.ReviewHuman,
	} {
		got := comp.resolveCriteria(level)
		if len(got) != 1 || got[0].Name != "catalog-criterion" {
			t.Errorf("level %d: resolveCriteria() = %v, want catalog criteria regardless of level", level, got)
		}
	}
}

// =============================================================================
// RESOLVE JUDGES TESTS
// =============================================================================

func TestResolveJudges_NilCatalog_FallsBackToDefaults(t *testing.T) {
	comp := &Component{config: &Config{}, catalog: nil}

	for _, level := range []domain.ReviewLevel{
		domain.ReviewAuto,
		domain.ReviewStandard,
		domain.ReviewStrict,
		domain.ReviewHuman,
	} {
		got := comp.resolveJudges(level)
		want := comp.defaultJudges(level)

		if len(got) != len(want) {
			t.Errorf("level %d: resolveJudges() len=%d, want %d (nil catalog fallback)",
				level, len(got), len(want))
			continue
		}
		for i := range got {
			if got[i].ID != want[i].ID {
				t.Errorf("level %d judges[%d].ID = %q, want %q", level, i, got[i].ID, want[i].ID)
			}
		}
	}
}

func TestResolveJudges_NilReviewConfig_FallsBackToDefaults(t *testing.T) {
	comp := &Component{
		config:  &Config{},
		catalog: &promptmanager.DomainCatalog{ReviewConfig: nil},
	}

	got := comp.resolveJudges(domain.ReviewStandard)
	want := comp.defaultJudges(domain.ReviewStandard)

	if len(got) != len(want) {
		t.Fatalf("resolveJudges() len=%d, want %d (nil ReviewConfig fallback)", len(got), len(want))
	}
}

func TestResolveJudges_EmptyJudges_FallsBackToDefaults(t *testing.T) {
	comp := &Component{
		config: &Config{},
		catalog: &promptmanager.DomainCatalog{
			ReviewConfig: &promptmanager.ReviewConfig{
				DefaultJudges: []domain.Judge{},
			},
		},
	}

	got := comp.resolveJudges(domain.ReviewStandard)
	want := comp.defaultJudges(domain.ReviewStandard)

	if len(got) != len(want) {
		t.Fatalf("resolveJudges() len=%d, want %d (empty judges fallback)", len(got), len(want))
	}
}

func TestResolveJudges_CatalogJudgesReturned(t *testing.T) {
	catalogJudges := []domain.Judge{
		{ID: "domain-judge-1", Type: domain.JudgeLLM, Config: map[string]any{"model": "gpt-4"}},
		{ID: "domain-judge-2", Type: domain.JudgeAutomated, Config: map[string]any{}},
	}
	comp := &Component{
		config: &Config{},
		catalog: &promptmanager.DomainCatalog{
			ReviewConfig: &promptmanager.ReviewConfig{
				DefaultJudges: catalogJudges,
			},
		},
	}

	got := comp.resolveJudges(domain.ReviewStandard)

	if len(got) != len(catalogJudges) {
		t.Fatalf("resolveJudges() len=%d, want %d", len(got), len(catalogJudges))
	}
	for i, want := range catalogJudges {
		if got[i].ID != want.ID {
			t.Errorf("judges[%d].ID = %q, want %q", i, got[i].ID, want.ID)
		}
		if got[i].Type != want.Type {
			t.Errorf("judges[%d].Type = %q, want %q", i, got[i].Type, want.Type)
		}
	}
}

func TestResolveJudges_CatalogJudgesReturnsCopy(t *testing.T) {
	// Mutating the returned slice must not affect the catalog's original slice.
	catalogJudges := []domain.Judge{
		{ID: "original-judge", Type: domain.JudgeAutomated},
	}
	comp := &Component{
		config: &Config{},
		catalog: &promptmanager.DomainCatalog{
			ReviewConfig: &promptmanager.ReviewConfig{
				DefaultJudges: catalogJudges,
			},
		},
	}

	got := comp.resolveJudges(domain.ReviewStandard)
	got[0].ID = "mutated-judge"

	// The catalog's original slice must be untouched.
	if comp.catalog.ReviewConfig.DefaultJudges[0].ID != "original-judge" {
		t.Error("resolveJudges() returned the original slice instead of a copy; mutation affected the catalog")
	}
}

func TestResolveJudges_LevelIgnoredWhenCatalogHasJudges(t *testing.T) {
	// When the catalog provides judges the level parameter is irrelevant —
	// the same judges must be returned regardless of which level is passed.
	// Include a human judge so the ReviewHuman safety-append doesn't add an extra one.
	catalogJudges := []domain.Judge{
		{ID: "catalog-judge", Type: domain.JudgeAutomated},
		{ID: "catalog-human", Type: domain.JudgeHuman},
	}
	comp := &Component{
		config: &Config{},
		catalog: &promptmanager.DomainCatalog{
			ReviewConfig: &promptmanager.ReviewConfig{
				DefaultJudges: catalogJudges,
			},
		},
	}

	for _, level := range []domain.ReviewLevel{
		domain.ReviewAuto,
		domain.ReviewStandard,
		domain.ReviewStrict,
		domain.ReviewHuman,
	} {
		got := comp.resolveJudges(level)
		if len(got) != 2 || got[0].ID != "catalog-judge" || got[1].ID != "catalog-human" {
			t.Errorf("level %d: resolveJudges() = %v, want catalog judges regardless of level", level, got)
		}
	}
}

// =============================================================================
// PER-LEVEL CRITERIA/JUDGES OVERRIDE TESTS
// =============================================================================

func TestResolveCriteria_PerLevelOverrideTakesPrecedence(t *testing.T) {
	defaultCriteria := []domain.ReviewCriterion{
		{Name: "default-check", Weight: 1.0, Threshold: 0.5},
	}
	strictCriteria := []domain.ReviewCriterion{
		{Name: "strict-precision", Weight: 0.6, Threshold: 0.9},
		{Name: "strict-rigor", Weight: 0.4, Threshold: 0.85},
	}
	comp := &Component{
		config: &Config{},
		catalog: &promptmanager.DomainCatalog{
			ReviewConfig: &promptmanager.ReviewConfig{
				DefaultCriteria: defaultCriteria,
				CriteriaByLevel: map[domain.ReviewLevel][]domain.ReviewCriterion{
					domain.ReviewStrict: strictCriteria,
				},
			},
		},
	}

	// Strict should use the per-level override
	got := comp.resolveCriteria(domain.ReviewStrict)
	if len(got) != 2 {
		t.Fatalf("resolveCriteria(Strict) len=%d, want 2", len(got))
	}
	if got[0].Name != "strict-precision" {
		t.Errorf("criteria[0].Name = %q, want %q", got[0].Name, "strict-precision")
	}

	// Standard should fall back to default criteria (no per-level override)
	gotStd := comp.resolveCriteria(domain.ReviewStandard)
	if len(gotStd) != 1 || gotStd[0].Name != "default-check" {
		t.Errorf("resolveCriteria(Standard) = %v, want default criteria", gotStd)
	}
}

func TestResolveCriteria_PerLevelOverrideReturnsCopy(t *testing.T) {
	strictCriteria := []domain.ReviewCriterion{
		{Name: "original", Weight: 1.0, Threshold: 0.5},
	}
	comp := &Component{
		config: &Config{},
		catalog: &promptmanager.DomainCatalog{
			ReviewConfig: &promptmanager.ReviewConfig{
				CriteriaByLevel: map[domain.ReviewLevel][]domain.ReviewCriterion{
					domain.ReviewStrict: strictCriteria,
				},
			},
		},
	}

	got := comp.resolveCriteria(domain.ReviewStrict)
	got[0].Name = "mutated"

	if comp.catalog.ReviewConfig.CriteriaByLevel[domain.ReviewStrict][0].Name != "original" {
		t.Error("per-level criteria returned original slice instead of copy")
	}
}

func TestResolveCriteria_EmptyPerLevelFallsToDefault(t *testing.T) {
	comp := &Component{
		config: &Config{},
		catalog: &promptmanager.DomainCatalog{
			ReviewConfig: &promptmanager.ReviewConfig{
				DefaultCriteria: []domain.ReviewCriterion{
					{Name: "default", Weight: 1.0, Threshold: 0.5},
				},
				CriteriaByLevel: map[domain.ReviewLevel][]domain.ReviewCriterion{
					domain.ReviewStrict: {}, // empty per-level = fall through to default
				},
			},
		},
	}

	got := comp.resolveCriteria(domain.ReviewStrict)
	if len(got) != 1 || got[0].Name != "default" {
		t.Errorf("empty per-level should fall through to DefaultCriteria, got %v", got)
	}
}

func TestResolveJudges_PerLevelOverrideTakesPrecedence(t *testing.T) {
	defaultJudges := []domain.Judge{
		{ID: "default-judge", Type: domain.JudgeAutomated},
	}
	humanJudges := []domain.Judge{
		{ID: "human-judge", Type: domain.JudgeHuman},
		{ID: "llm-judge", Type: domain.JudgeLLM},
	}
	comp := &Component{
		config: &Config{},
		catalog: &promptmanager.DomainCatalog{
			ReviewConfig: &promptmanager.ReviewConfig{
				DefaultJudges: defaultJudges,
				JudgesByLevel: map[domain.ReviewLevel][]domain.Judge{
					domain.ReviewHuman: humanJudges,
				},
			},
		},
	}

	// Human should use the per-level override
	got := comp.resolveJudges(domain.ReviewHuman)
	if len(got) != 2 {
		t.Fatalf("resolveJudges(Human) len=%d, want 2", len(got))
	}
	if got[0].ID != "human-judge" {
		t.Errorf("judges[0].ID = %q, want %q", got[0].ID, "human-judge")
	}

	// Standard should fall back to default judges
	gotStd := comp.resolveJudges(domain.ReviewStandard)
	if len(gotStd) != 1 || gotStd[0].ID != "default-judge" {
		t.Errorf("resolveJudges(Standard) = %v, want default judges", gotStd)
	}
}

func TestResolveJudges_PerLevelOverrideReturnsCopy(t *testing.T) {
	humanJudges := []domain.Judge{
		{ID: "original-judge", Type: domain.JudgeHuman},
	}
	comp := &Component{
		config: &Config{},
		catalog: &promptmanager.DomainCatalog{
			ReviewConfig: &promptmanager.ReviewConfig{
				JudgesByLevel: map[domain.ReviewLevel][]domain.Judge{
					domain.ReviewHuman: humanJudges,
				},
			},
		},
	}

	got := comp.resolveJudges(domain.ReviewHuman)
	got[0].ID = "mutated-judge"

	if comp.catalog.ReviewConfig.JudgesByLevel[domain.ReviewHuman][0].ID != "original-judge" {
		t.Error("per-level judges returned original slice instead of copy")
	}
}

// =============================================================================
// DEFAULT REVIEW LEVEL TESTS
// =============================================================================

func TestBuildBattle_DefaultReviewLevel_UsedWhenQuestIsAuto(t *testing.T) {
	comp := &Component{
		config: &Config{},
		catalog: &promptmanager.DomainCatalog{
			ReviewConfig: &promptmanager.ReviewConfig{
				DefaultReviewLevel: domain.ReviewStandard,
				DefaultCriteria: []domain.ReviewCriterion{
					{Name: "test", Weight: 1.0, Threshold: 0.5},
				},
				DefaultJudges: []domain.Judge{
					{ID: "j1", Type: domain.JudgeAutomated},
				},
			},
		},
	}

	quest := &domain.Quest{
		ID:    "quest-1",
		Title: "Test Quest",
		Constraints: domain.QuestConstraints{
			ReviewLevel: domain.ReviewAuto, // quest doesn't specify
		},
	}

	battle := comp.buildBattle("battle-1", quest)

	if battle.Level != domain.ReviewStandard {
		t.Errorf("battle.Level = %d, want %d (domain default)", battle.Level, domain.ReviewStandard)
	}
}

func TestBuildBattle_ExplicitReviewLevel_NotOverridden(t *testing.T) {
	comp := &Component{
		config: &Config{},
		catalog: &promptmanager.DomainCatalog{
			ReviewConfig: &promptmanager.ReviewConfig{
				DefaultReviewLevel: domain.ReviewStandard,
				DefaultCriteria: []domain.ReviewCriterion{
					{Name: "test", Weight: 1.0, Threshold: 0.5},
				},
				DefaultJudges: []domain.Judge{
					{ID: "j1", Type: domain.JudgeAutomated},
				},
			},
		},
	}

	quest := &domain.Quest{
		ID:    "quest-2",
		Title: "Test Quest",
		Constraints: domain.QuestConstraints{
			ReviewLevel: domain.ReviewStrict, // quest specifies strict
		},
	}

	battle := comp.buildBattle("battle-2", quest)

	if battle.Level != domain.ReviewStrict {
		t.Errorf("battle.Level = %d, want %d (quest-specified)", battle.Level, domain.ReviewStrict)
	}
}

func TestBuildBattle_NoCatalog_KeepsAutoLevel(t *testing.T) {
	comp := &Component{
		config:  &Config{},
		catalog: nil,
	}

	quest := &domain.Quest{
		ID:    "quest-3",
		Title: "Test Quest",
		Constraints: domain.QuestConstraints{
			ReviewLevel: domain.ReviewAuto,
		},
	}

	battle := comp.buildBattle("battle-3", quest)

	if battle.Level != domain.ReviewAuto {
		t.Errorf("battle.Level = %d, want %d (no catalog, stays auto)", battle.Level, domain.ReviewAuto)
	}
}

// =============================================================================
// JSON PARSING & EVALUATOR HELPER TESTS
// =============================================================================

func TestExtractJSON_MarkdownCodeBlock(t *testing.T) {
	input := "Here is my evaluation:\n```json\n{\"criteria\": []}\n```\nDone."
	got := extractJSON(input)
	if got != `{"criteria": []}` {
		t.Errorf("extractJSON() = %q, want %q", got, `{"criteria": []}`)
	}
}

func TestExtractJSON_BareJSON(t *testing.T) {
	input := `Some text {"criteria": []} more text`
	got := extractJSON(input)
	if got != `{"criteria": []}` {
		t.Errorf("extractJSON() = %q, want %q", got, `{"criteria": []}`)
	}
}

func TestExtractJSON_NoJSON(t *testing.T) {
	input := "No JSON here at all"
	got := extractJSON(input)
	if got != "" {
		t.Errorf("extractJSON() = %q, want empty", got)
	}
}

func TestClampScore(t *testing.T) {
	tests := []struct {
		input float64
		want  float64
	}{
		{0.5, 0.5},
		{0.0, 0.0},
		{1.0, 1.0},
		{-0.5, 0.0},
		{1.5, 1.0},
	}
	for _, tt := range tests {
		got := clampScore(tt.input)
		if got != tt.want {
			t.Errorf("clampScore(%f) = %f, want %f", tt.input, got, tt.want)
		}
	}
}

func TestFormatOutputForJudge_Nil(t *testing.T) {
	got := formatOutputForJudge(nil)
	if got != "No output was provided." {
		t.Errorf("formatOutputForJudge(nil) = %q", got)
	}
}

func TestFormatOutputForJudge_String(t *testing.T) {
	got := formatOutputForJudge("hello")
	if got != "## Quest Output\n\nhello" {
		t.Errorf("formatOutputForJudge(string) = %q", got)
	}
}

func TestFormatOutputForJudge_Map(t *testing.T) {
	got := formatOutputForJudge(map[string]any{"key": "val"})
	if got == "" || got == "No output was provided." {
		t.Errorf("formatOutputForJudge(map) should format as JSON, got %q", got)
	}
}

func TestParseJudgeResponse_ValidJSON(t *testing.T) {
	battle := &BossBattle{
		Criteria: []domain.ReviewCriterion{
			{Name: "correctness", Threshold: 0.7},
			{Name: "completeness", Threshold: 0.6},
		},
	}
	content := `{"criteria": [{"name": "correctness", "score": 0.9, "reasoning": "Good"}, {"name": "completeness", "score": 0.5, "reasoning": "Missing parts"}], "overall_feedback": "Mostly good"}`

	results, _, feedback, peerRatings, err := parseJudgeResponse(content, battle)
	if err != nil {
		t.Fatalf("parseJudgeResponse() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if feedback != "Mostly good" {
		t.Errorf("feedback = %q, want %q", feedback, "Mostly good")
	}
	if peerRatings != nil {
		t.Error("peerRatings should be nil when not provided in response")
	}

	// correctness: 0.9 >= 0.7 → passed
	if results[0].Score != 0.9 || !results[0].Passed {
		t.Errorf("correctness: score=%f passed=%v, want 0.9/true", results[0].Score, results[0].Passed)
	}
	// completeness: 0.5 < 0.6 → failed
	if results[1].Score != 0.5 || results[1].Passed {
		t.Errorf("completeness: score=%f passed=%v, want 0.5/false", results[1].Score, results[1].Passed)
	}
}

func TestParseJudgeResponse_WrappedInCodeBlock(t *testing.T) {
	battle := &BossBattle{
		Criteria: []domain.ReviewCriterion{
			{Name: "quality", Threshold: 0.5},
		},
	}
	content := "Here is my evaluation:\n```json\n{\"criteria\": [{\"name\": \"quality\", \"score\": 0.8, \"reasoning\": \"Good\"}], \"overall_feedback\": \"OK\"}\n```"

	results, _, _, _, err := parseJudgeResponse(content, battle)
	if err != nil {
		t.Fatalf("parseJudgeResponse() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
}

func TestParseJudgeResponse_NoJSON(t *testing.T) {
	battle := &BossBattle{}
	_, _, _, _, err := parseJudgeResponse("I cannot evaluate this.", battle)
	if err == nil {
		t.Error("parseJudgeResponse() should fail when no JSON present")
	}
}

func TestParseJudgeResponse_UnknownCriterionSkipped(t *testing.T) {
	battle := &BossBattle{
		Criteria: []domain.ReviewCriterion{
			{Name: "known", Threshold: 0.5},
		},
	}
	content := `{"criteria": [{"name": "known", "score": 0.7, "reasoning": "OK"}, {"name": "unknown", "score": 0.9, "reasoning": "??"}]}`

	results, _, _, _, err := parseJudgeResponse(content, battle)
	if err != nil {
		t.Fatalf("parseJudgeResponse() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1 (unknown should be skipped)", len(results))
	}
	if results[0].CriterionName != "known" {
		t.Errorf("result[0].CriterionName = %q, want %q", results[0].CriterionName, "known")
	}
}

func TestParseJudgeResponse_ClampScores(t *testing.T) {
	battle := &BossBattle{
		Criteria: []domain.ReviewCriterion{
			{Name: "c1", Threshold: 0.5},
			{Name: "c2", Threshold: 0.5},
		},
	}
	content := `{"criteria": [{"name": "c1", "score": 1.5, "reasoning": "over"}, {"name": "c2", "score": -0.3, "reasoning": "under"}]}`

	results, _, _, _, err := parseJudgeResponse(content, battle)
	if err != nil {
		t.Fatalf("parseJudgeResponse() error = %v", err)
	}
	if results[0].Score != 1.0 {
		t.Errorf("clamped over score = %f, want 1.0", results[0].Score)
	}
	if results[1].Score != 0.0 {
		t.Errorf("clamped under score = %f, want 0.0", results[1].Score)
	}
}

// =============================================================================
// DOMAIN-AWARE EVALUATOR FALLBACK TESTS
// =============================================================================

func TestDomainAwareEvaluator_NoLLMJudge_FallsBackToHeuristic(t *testing.T) {
	catalog := &promptmanager.DomainCatalog{
		JudgeSystemBase: "test judge",
		ReviewConfig: &promptmanager.ReviewConfig{
			DefaultCriteria: []domain.ReviewCriterion{
				{Name: "test", Weight: 1.0, Threshold: 0.5},
			},
		},
	}
	e := NewDomainAwareEvaluator(catalog, nil, nil, nil)

	battle := &BossBattle{
		ID:      "b1",
		QuestID: "q1",
		Criteria: []domain.ReviewCriterion{
			{Name: "test", Weight: 1.0, Threshold: 0.5},
		},
		Judges: []domain.Judge{
			{ID: "auto", Type: domain.JudgeAutomated}, // no LLM judge
		},
	}

	result, err := e.Evaluate(context.Background(), battle, &domain.Quest{}, "output")
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	// Should use heuristic scoring (0.8 for non-nil output)
	if result.Results[0].Score != 0.8 {
		t.Errorf("Score = %f, want 0.8 (heuristic fallback)", result.Results[0].Score)
	}
}

func TestDomainAwareEvaluator_NilRegistry_FallsBackToHeuristic(t *testing.T) {
	catalog := &promptmanager.DomainCatalog{
		JudgeSystemBase: "test judge",
	}
	e := NewDomainAwareEvaluator(catalog, nil, nil, nil) // nil registry, nil tokenLedger, nil nats

	battle := &BossBattle{
		ID:      "b1",
		QuestID: "q1",
		Criteria: []domain.ReviewCriterion{
			{Name: "test", Weight: 1.0, Threshold: 0.5},
		},
		Judges: []domain.Judge{
			{ID: "llm", Type: domain.JudgeLLM}, // has LLM judge but no registry
		},
	}

	result, err := e.Evaluate(context.Background(), battle, &domain.Quest{}, "output")
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if result.Results[0].Score != 0.8 {
		t.Errorf("Score = %f, want 0.8 (heuristic fallback)", result.Results[0].Score)
	}
}

func TestDomainAwareEvaluator_HasLLMJudge(t *testing.T) {
	e := &DomainAwareEvaluator{}

	noLLM := &BossBattle{
		Judges: []domain.Judge{{ID: "auto", Type: domain.JudgeAutomated}},
	}
	if e.hasLLMJudge(noLLM) {
		t.Error("hasLLMJudge() = true for automated-only battle")
	}

	withLLM := &BossBattle{
		Judges: []domain.Judge{
			{ID: "auto", Type: domain.JudgeAutomated},
			{ID: "llm", Type: domain.JudgeLLM},
		},
	}
	if !e.hasLLMJudge(withLLM) {
		t.Error("hasLLMJudge() = false for battle with LLM judge")
	}
}

func TestDomainAwareEvaluator_MergeResults_AllScored(t *testing.T) {
	e := &DomainAwareEvaluator{}
	battle := &BossBattle{
		Criteria: []domain.ReviewCriterion{
			{Name: "c1", Threshold: 0.5, Weight: 0.5},
			{Name: "c2", Threshold: 0.5, Weight: 0.5},
		},
	}
	llmResults := []domain.ReviewResult{
		{CriterionName: "c1", Score: 0.9, Passed: true, JudgeID: "judge-llm"},
		{CriterionName: "c2", Score: 0.7, Passed: true, JudgeID: "judge-llm"},
	}

	merged := e.mergeResults(battle, llmResults)
	if len(merged) != 2 {
		t.Fatalf("len(merged) = %d, want 2", len(merged))
	}
	for _, r := range merged {
		if r.JudgeID != "judge-llm" {
			t.Errorf("merged result %q should be from judge-llm, got %q", r.CriterionName, r.JudgeID)
		}
	}
}

func TestDomainAwareEvaluator_MergeResults_MissingFallsBack(t *testing.T) {
	e := &DomainAwareEvaluator{}
	battle := &BossBattle{
		Criteria: []domain.ReviewCriterion{
			{Name: "c1", Threshold: 0.5, Weight: 0.5},
			{Name: "c2", Threshold: 0.5, Weight: 0.5},
		},
	}
	llmResults := []domain.ReviewResult{
		{CriterionName: "c1", Score: 0.9, Passed: true, JudgeID: "judge-llm"},
		// c2 not scored by LLM
	}

	merged := e.mergeResults(battle, llmResults)
	if len(merged) != 2 {
		t.Fatalf("len(merged) = %d, want 2", len(merged))
	}
	if merged[0].JudgeID != "judge-llm" {
		t.Errorf("c1 should be from LLM, got %q", merged[0].JudgeID)
	}
	if merged[1].JudgeID != "judge-auto" {
		t.Errorf("c2 should fall back to heuristic, got %q", merged[1].JudgeID)
	}
}

// =============================================================================
// isRedTeamEligible TESTS
// =============================================================================
// isRedTeamEligible only reads Quest fields — a zero-value Component is safe.

func TestIsRedTeamEligible_NormalQuestWithReview(t *testing.T) {
	c := &Component{}
	parent := domain.QuestID("parent-q")
	tests := []struct {
		name  string
		quest *domain.Quest
		want  bool
	}{
		{
			name: "require_review true normal quest",
			quest: &domain.Quest{
				ID: "q1",
				Constraints: domain.QuestConstraints{
					RequireReview: true,
				},
			},
			want: true,
		},
		{
			name: "require_review false",
			quest: &domain.Quest{
				ID: "q2",
				Constraints: domain.QuestConstraints{
					RequireReview: false,
				},
			},
			want: false,
		},
		{
			name: "sub-quest (ParentQuest set)",
			quest: &domain.Quest{
				ID:          "q3",
				ParentQuest: &parent,
				Constraints: domain.QuestConstraints{
					RequireReview: true,
				},
			},
			want: false,
		},
		{
			name: "red-team quest type",
			quest: &domain.Quest{
				ID:        "q4",
				QuestType: domain.QuestTypeRedTeam,
				Constraints: domain.QuestConstraints{
					RequireReview: true,
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.isRedTeamEligible(tt.quest)
			if got != tt.want {
				t.Errorf("isRedTeamEligible() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestIsRedTeamEligible_SubQuestTakesPrecedence verifies that a quest which is
// BOTH a sub-quest and has RequireReview=true is still ineligible — ParentQuest
// check fires before QuestType.
func TestIsRedTeamEligible_SubQuestTakesPrecedence(t *testing.T) {
	c := &Component{}
	parent := domain.QuestID("parent-q")
	quest := &domain.Quest{
		ID:          "q-sub-rt",
		QuestType:   domain.QuestTypeRedTeam,
		ParentQuest: &parent,
		Constraints: domain.QuestConstraints{
			RequireReview: true,
		},
	}

	if c.isRedTeamEligible(quest) {
		t.Error("sub-quest that is also a red-team quest should not be eligible")
	}
}

