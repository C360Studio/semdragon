package bossbattle

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/c360studio/semdragons/domain"
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
		Judges: []Judge{
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
	b.Verdict = &BattleVerdict{
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
	b.Verdict = &BattleVerdict{
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

// =============================================================================
// DEFAULT BATTLE EVALUATOR TESTS
// =============================================================================

// newEvaluatorBattle builds a battle for evaluator unit tests using the
// component's own defaultCriteria/defaultJudges to avoid duplication.
// When judges are explicitly supplied they override the defaults.
func newEvaluatorBattle(level domain.ReviewLevel, overrideJudges ...Judge) *BossBattle {
	comp := &Component{config: &Config{}}
	criteria := comp.defaultCriteria(level)

	var judges []Judge
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
		Judges: []Judge{
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

	humanJudge := Judge{ID: "judge-human", Type: domain.JudgeHuman}
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
	humanJudge := Judge{ID: "judge-human-2", Type: domain.JudgeHuman}
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
		Judges:    []Judge{{ID: "judge-auto", Type: domain.JudgeAutomated}},
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
// QUEST FROM ENTITY STATE FOR BATTLE TESTS
// =============================================================================

func TestQuestFromEntityStateForBattle_NilEntity(t *testing.T) {
	got := questFromEntityStateForBattle(nil)
	if got != nil {
		t.Errorf("questFromEntityStateForBattle(nil) = %v, want nil", got)
	}
}

func TestQuestFromEntityStateForBattle_IDPreserved(t *testing.T) {
	entity := &graph.EntityState{
		ID:      "c360.prod.game.board1.quest.abc",
		Triples: []message.Triple{},
	}
	quest := questFromEntityStateForBattle(entity)

	if quest == nil {
		t.Fatal("questFromEntityStateForBattle() returned nil for non-nil entity")
	}
	if string(quest.ID) != entity.ID {
		t.Errorf("quest.ID = %q, want %q", quest.ID, entity.ID)
	}
}

func TestQuestFromEntityStateForBattle_ParsesTriples(t *testing.T) {
	agentID := "c360.prod.game.board1.agent.warrior"
	entity := &graph.EntityState{
		ID: "c360.prod.game.board1.quest.q1",
		Triples: []message.Triple{
			{Predicate: "quest.identity.title", Object: "Kill the Dragon"},
			{Predicate: "quest.identity.description", Object: "Slay the dragon"},
			{Predicate: "quest.status.state", Object: "in_review"},
			{Predicate: "quest.difficulty.level", Object: float64(2)},
			{Predicate: "quest.tier.minimum", Object: float64(1)},
			{Predicate: "quest.xp.base", Object: float64(100)},
			{Predicate: "quest.assignment.agent", Object: agentID},
			{Predicate: "quest.review.level", Object: float64(2)},
			{Predicate: "quest.review.needs_review", Object: true},
			{Predicate: "quest.attempts.current", Object: float64(1)},
			{Predicate: "quest.attempts.max", Object: float64(3)},
			{Predicate: "quest.observability.trajectory_id", Object: "traj-abc"},
		},
	}

	quest := questFromEntityStateForBattle(entity)
	if quest == nil {
		t.Fatal("questFromEntityStateForBattle() returned nil")
	}

	if quest.Title != "Kill the Dragon" {
		t.Errorf("Title = %q, want %q", quest.Title, "Kill the Dragon")
	}
	if quest.Description != "Slay the dragon" {
		t.Errorf("Description = %q, want %q", quest.Description, "Slay the dragon")
	}
	if quest.Status != domain.QuestInReview {
		t.Errorf("Status = %q, want %q", quest.Status, domain.QuestInReview)
	}
	if quest.Difficulty != domain.QuestDifficulty(2) {
		t.Errorf("Difficulty = %v, want %v", quest.Difficulty, domain.QuestDifficulty(2))
	}
	if quest.BaseXP != 100 {
		t.Errorf("BaseXP = %d, want 100", quest.BaseXP)
	}
	if quest.ClaimedBy == nil || string(*quest.ClaimedBy) != agentID {
		t.Errorf("ClaimedBy = %v, want %q", quest.ClaimedBy, agentID)
	}
	if quest.Constraints.ReviewLevel != domain.ReviewLevel(2) {
		t.Errorf("Constraints.ReviewLevel = %v, want %v", quest.Constraints.ReviewLevel, domain.ReviewLevel(2))
	}
	if !quest.Constraints.RequireReview {
		t.Error("Constraints.RequireReview should be true")
	}
	if quest.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", quest.Attempts)
	}
	if quest.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", quest.MaxAttempts)
	}
	if quest.TrajectoryID != "traj-abc" {
		t.Errorf("TrajectoryID = %q, want %q", quest.TrajectoryID, "traj-abc")
	}
}

func TestQuestFromEntityStateForBattle_NonStringObjectsIgnored(t *testing.T) {
	entity := &graph.EntityState{
		ID: "quest-42",
		Triples: []message.Triple{
			// Wrong type for quest.identity.title — should be silently ignored.
			{Predicate: "quest.identity.title", Object: 123},
		},
	}

	quest := questFromEntityStateForBattle(entity)
	if quest == nil {
		t.Fatal("questFromEntityStateForBattle() returned nil")
	}
	if quest.Title != "" {
		t.Errorf("Title = %q, want empty string for non-string triple object", quest.Title)
	}
}

func TestQuestFromEntityStateForBattle_EmptyTriples(t *testing.T) {
	entity := &graph.EntityState{
		ID:      "quest-empty",
		Triples: []message.Triple{},
	}

	quest := questFromEntityStateForBattle(entity)
	if quest == nil {
		t.Fatal("questFromEntityStateForBattle() returned nil for empty triples")
	}
	// Fields should be zero values.
	if quest.Title != "" {
		t.Errorf("Title = %q, want empty", quest.Title)
	}
	if quest.ClaimedBy != nil {
		t.Error("ClaimedBy should be nil for empty triples")
	}
}

func TestQuestFromEntityStateForBattle_AgentIDNotSetWhenMissing(t *testing.T) {
	entity := &graph.EntityState{
		ID: "quest-noagent",
		Triples: []message.Triple{
			{Predicate: "quest.identity.title", Object: "Unassigned Quest"},
			{Predicate: "quest.status.state", Object: "in_review"},
		},
	}

	quest := questFromEntityStateForBattle(entity)
	if quest == nil {
		t.Fatal("questFromEntityStateForBattle() returned nil")
	}
	if quest.ClaimedBy != nil {
		t.Errorf("ClaimedBy = %v, want nil when quest.assignment.agent is absent", quest.ClaimedBy)
	}
}
