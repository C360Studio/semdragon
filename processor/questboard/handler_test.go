package questboard

import (
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/message"
)

// =============================================================================
// CONFIG TESTS
// =============================================================================

func TestDefaultConfig_Defaults(t *testing.T) {
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
	if cfg.DefaultMaxAttempts != 3 {
		t.Errorf("DefaultMaxAttempts = %d, want 3", cfg.DefaultMaxAttempts)
	}
	if !cfg.EnableEvaluation {
		t.Error("EnableEvaluation should be true by default")
	}
}

func TestDefaultConfig_IsValid(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("DefaultConfig().Validate() = %v, want nil", err)
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name: "valid config",
			cfg: Config{
				Org:                "myorg",
				Platform:           "prod",
				Board:              "alpha",
				DefaultMaxAttempts: 2,
			},
			wantErr: "",
		},
		{
			name: "missing org",
			cfg: Config{
				Platform:           "prod",
				Board:              "alpha",
				DefaultMaxAttempts: 1,
			},
			wantErr: "org is required",
		},
		{
			name: "missing platform",
			cfg: Config{
				Org:                "myorg",
				Board:              "alpha",
				DefaultMaxAttempts: 1,
			},
			wantErr: "platform is required",
		},
		{
			name: "missing board",
			cfg: Config{
				Org:                "myorg",
				Platform:           "prod",
				DefaultMaxAttempts: 1,
			},
			wantErr: "board is required",
		},
		{
			name: "zero max attempts",
			cfg: Config{
				Org:                "myorg",
				Platform:           "prod",
				Board:              "alpha",
				DefaultMaxAttempts: 0,
			},
			wantErr: "default_max_attempts must be at least 1",
		},
		{
			name: "negative max attempts",
			cfg: Config{
				Org:                "myorg",
				Platform:           "prod",
				Board:              "alpha",
				DefaultMaxAttempts: -5,
			},
			wantErr: "default_max_attempts must be at least 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Errorf("Validate() = nil, want error %q", tt.wantErr)
				return
			}
			if err.Error() != tt.wantErr {
				t.Errorf("Validate() = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestConfig_ToBoardConfig(t *testing.T) {
	cfg := Config{
		Org:                "c360",
		Platform:           "prod",
		Board:              "board1",
		DefaultMaxAttempts: 5,
		EnableEvaluation:   true,
	}

	bc := cfg.ToBoardConfig()

	if bc == nil {
		t.Fatal("ToBoardConfig() returned nil")
	}
	if bc.Org != "c360" {
		t.Errorf("BoardConfig.Org = %q, want %q", bc.Org, "c360")
	}
	if bc.Platform != "prod" {
		t.Errorf("BoardConfig.Platform = %q, want %q", bc.Platform, "prod")
	}
	if bc.Board != "board1" {
		t.Errorf("BoardConfig.Board = %q, want %q", bc.Board, "board1")
	}
}

func TestConfig_ToBoardConfig_ProducesCorrectEntityIDs(t *testing.T) {
	cfg := Config{
		Org:                "c360",
		Platform:           "dev",
		Board:              "main",
		DefaultMaxAttempts: 3,
	}

	bc := cfg.ToBoardConfig()
	questID := bc.QuestEntityID("abc123")
	agentID := bc.AgentEntityID("xyz789")

	if questID != "c360.dev.game.main.quest.abc123" {
		t.Errorf("QuestEntityID = %q, want %q", questID, "c360.dev.game.main.quest.abc123")
	}
	if agentID != "c360.dev.game.main.agent.xyz789" {
		t.Errorf("AgentEntityID = %q, want %q", agentID, "c360.dev.game.main.agent.xyz789")
	}
}

func TestConfig_ToBoardConfig_DefaultMaxAttemptsNotCarried(t *testing.T) {
	// DefaultMaxAttempts and EnableEvaluation live on the component Config, not
	// the BoardConfig.  Verify ToBoardConfig only maps the identity fields.
	cfg := Config{
		Org:                "org",
		Platform:           "plat",
		Board:              "brd",
		DefaultMaxAttempts: 99,
		EnableEvaluation:   false,
	}
	bc := cfg.ToBoardConfig()
	// The board config should be a pure org/platform/board struct — we just
	// confirm it is non-nil and the identity fields came through.
	if bc.Org != cfg.Org || bc.Platform != cfg.Platform || bc.Board != cfg.Board {
		t.Errorf("ToBoardConfig identity mismatch: got %+v", bc)
	}
}

// =============================================================================
// QUEST ENTITY ID TESTS
// =============================================================================

func TestQuest_EntityID_ReturnsID(t *testing.T) {
	q := domain.Quest{
		ID: domain.QuestID("c360.dev.game.board1.quest.abc123"),
	}

	if q.EntityID() != "c360.dev.game.board1.quest.abc123" {
		t.Errorf("EntityID() = %q, want %q", q.EntityID(), "c360.dev.game.board1.quest.abc123")
	}
}

func TestQuest_EntityID_EmptyID(t *testing.T) {
	q := domain.Quest{}
	if q.EntityID() != "" {
		t.Errorf("EntityID() on zero-value quest = %q, want empty string", q.EntityID())
	}
}

// =============================================================================
// QUEST PRIMARY SKILL TESTS
// =============================================================================

func TestQuest_PrimarySkill(t *testing.T) {
	tests := []struct {
		name   string
		skills []domain.SkillTag
		want   domain.SkillTag
	}{
		{
			name:   "no skills returns empty",
			skills: nil,
			want:   "",
		},
		{
			name:   "empty slice returns empty",
			skills: []domain.SkillTag{},
			want:   "",
		},
		{
			name:   "single skill returns that skill",
			skills: []domain.SkillTag{domain.SkillCodeGen},
			want:   domain.SkillCodeGen,
		},
		{
			name:   "multiple skills returns first",
			skills: []domain.SkillTag{domain.SkillAnalysis, domain.SkillCodeGen, domain.SkillResearch},
			want:   domain.SkillAnalysis,
		},
		{
			name:   "research as only skill",
			skills: []domain.SkillTag{domain.SkillResearch},
			want:   domain.SkillResearch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := domain.Quest{RequiredSkills: tt.skills}
			got := q.PrimarySkill()
			if got != tt.want {
				t.Errorf("PrimarySkill() = %q, want %q", got, tt.want)
			}
		})
	}
}

// =============================================================================
// QUEST TRIPLES TESTS
// =============================================================================

// triplesByPredicate indexes triples by predicate for easy lookup.
func triplesByPredicate(triples []message.Triple) map[string]message.Triple {
	m := make(map[string]message.Triple, len(triples))
	for _, tr := range triples {
		m[tr.Predicate] = tr
	}
	return m
}

// triplesForPredicate collects all triples matching a predicate (for repeated predicates).
func triplesForPredicate(triples []message.Triple, predicate string) []message.Triple {
	var result []message.Triple
	for _, tr := range triples {
		if tr.Predicate == predicate {
			result = append(result, tr)
		}
	}
	return result
}

func TestQuest_Triples_MinimalQuest(t *testing.T) {
	q := &domain.Quest{
		ID:       domain.QuestID("c360.dev.game.board1.quest.minimal"),
		Title:    "Minimal Quest",
		Status:   domain.QuestPosted,
		PostedAt: time.Now(),
	}

	triples := q.Triples()

	if len(triples) == 0 {
		t.Fatal("Triples() returned no triples")
	}

	idx := triplesByPredicate(triples)

	// All triples should have the correct subject.
	for _, tr := range triples {
		if tr.Subject != "c360.dev.game.board1.quest.minimal" {
			t.Errorf("triple subject = %q, want %q", tr.Subject, "c360.dev.game.board1.quest.minimal")
		}
	}

	// Core mandatory predicates must be present.
	mandatory := []string{
		"quest.identity.title",
		"quest.identity.description",
		"quest.status.state",
		"quest.difficulty.level",
		"quest.tier.minimum",
		"quest.party.required",
		"quest.xp.base",
		"quest.attempts.current",
		"quest.attempts.max",
		"quest.lifecycle.posted_at",
		"quest.review.level",
		"quest.review.needs_review",
	}
	for _, pred := range mandatory {
		if _, ok := idx[pred]; !ok {
			t.Errorf("mandatory predicate %q not found in triples", pred)
		}
	}

	// Title value
	if v, ok := idx["quest.identity.title"].Object.(string); !ok || v != "Minimal Quest" {
		t.Errorf("quest.identity.title = %v, want %q", idx["quest.identity.title"].Object, "Minimal Quest")
	}

	// Status value
	if v, ok := idx["quest.status.state"].Object.(string); !ok || v != string(domain.QuestPosted) {
		t.Errorf("quest.status.state = %v, want %q", idx["quest.status.state"].Object, domain.QuestPosted)
	}
}

func TestQuest_Triples_FullQuest(t *testing.T) {
	claimedBy := domain.AgentID("c360.dev.game.board1.agent.hero")
	partyID := domain.PartyID("c360.dev.game.board1.party.team1")
	guildID := domain.GuildID("c360.dev.game.board1.guild.coders")
	parentID := domain.QuestID("c360.dev.game.board1.quest.parent1")
	claimedAt := time.Now().Add(-1 * time.Hour)
	startedAt := time.Now().Add(-30 * time.Minute)
	completedAt := time.Now()
	verdict := &domain.BattleVerdict{
		Passed:       true,
		QualityScore: 0.92,
		XPAwarded:    250,
		Feedback:     "Excellent work",
	}

	q := &domain.Quest{
		ID:             domain.QuestID("c360.dev.game.board1.quest.full"),
		Title:          "Full Quest",
		Description:    "A fully populated quest for testing",
		Status:         domain.QuestCompleted,
		Difficulty:     domain.DifficultyModerate,
		RequiredSkills: []domain.SkillTag{domain.SkillCodeGen, domain.SkillAnalysis},
		RequiredTools:  []string{"linter", "formatter"},
		MinTier:        domain.TierJourneyman,
		PartyRequired:  false,
		BaseXP:         300,
		BonusXP:        50,
		GuildXP:        25,
		Constraints: domain.QuestConstraints{
			RequireReview: true,
			ReviewLevel:   domain.ReviewStandard,
			MaxTokens:     4000,
		},
		ClaimedBy:     &claimedBy,
		PartyID:       &partyID,
		GuildPriority: &guildID,
		ParentQuest:   &parentID,
		DependsOn:     []domain.QuestID{"c360.dev.game.board1.quest.dep1"},
		Acceptance:    []string{"All tests pass", "Reviewed by expert"},
		PostedAt:      time.Now().Add(-2 * time.Hour),
		ClaimedAt:     &claimedAt,
		StartedAt:     &startedAt,
		CompletedAt:   &completedAt,
		Attempts:      1,
		MaxAttempts:   3,
		LoopID:  "quest-test-loop-abc123",
		Verdict: verdict,
		Duration:      45 * time.Minute,
	}

	triples := q.Triples()
	idx := triplesByPredicate(triples)

	// Identity predicates.
	if v, _ := idx["quest.identity.title"].Object.(string); v != "Full Quest" {
		t.Errorf("quest.identity.title = %q, want %q", v, "Full Quest")
	}
	if v, _ := idx["quest.identity.description"].Object.(string); v != "A fully populated quest for testing" {
		t.Errorf("quest.identity.description = %q, want %q", v, "A fully populated quest for testing")
	}

	// Status and difficulty.
	if v, _ := idx["quest.status.state"].Object.(string); v != string(domain.QuestCompleted) {
		t.Errorf("quest.status.state = %q, want %q", v, domain.QuestCompleted)
	}
	if v, _ := idx["quest.difficulty.level"].Object.(int); v != int(domain.DifficultyModerate) {
		t.Errorf("quest.difficulty.level = %d, want %d", v, domain.DifficultyModerate)
	}

	// Requirements.
	if v, _ := idx["quest.tier.minimum"].Object.(int); v != int(domain.TierJourneyman) {
		t.Errorf("quest.tier.minimum = %d, want %d", v, domain.TierJourneyman)
	}
	if v, _ := idx["quest.party.required"].Object.(bool); v != false {
		t.Errorf("quest.party.required = %v, want false", v)
	}

	// Rewards.
	if v, _ := idx["quest.xp.base"].Object.(int64); v != 300 {
		t.Errorf("quest.xp.base = %d, want 300", v)
	}

	// Assignment predicates.
	if v, _ := idx["quest.assignment.agent"].Object.(string); v != string(claimedBy) {
		t.Errorf("quest.assignment.agent = %q, want %q", v, claimedBy)
	}
	if v, _ := idx["quest.assignment.party"].Object.(string); v != string(partyID) {
		t.Errorf("quest.assignment.party = %q, want %q", v, partyID)
	}
	if v, _ := idx["quest.priority.guild"].Object.(string); v != string(guildID) {
		t.Errorf("quest.priority.guild = %q, want %q", v, guildID)
	}
	if v, _ := idx["quest.parent.quest"].Object.(string); v != string(parentID) {
		t.Errorf("quest.parent.quest = %q, want %q", v, parentID)
	}

	// Observability.
	if v, _ := idx["quest.execution.loop_id"].Object.(string); v != "quest-test-loop-abc123" {
		t.Errorf("quest.execution.loop_id = %q, want %q", v, "quest-test-loop-abc123")
	}

	// Review constraints.
	if v, _ := idx["quest.review.needs_review"].Object.(bool); !v {
		t.Error("quest.review.needs_review should be true")
	}
	if v, _ := idx["quest.review.level"].Object.(int); v != int(domain.ReviewStandard) {
		t.Errorf("quest.review.level = %d, want %d", v, domain.ReviewStandard)
	}

	// Verdict predicates.
	if v, _ := idx["quest.verdict.passed"].Object.(bool); !v {
		t.Error("quest.verdict.passed should be true")
	}
	if v, _ := idx["quest.verdict.score"].Object.(float64); v != 0.92 {
		t.Errorf("quest.verdict.score = %v, want 0.92", v)
	}
	if v, _ := idx["quest.verdict.xp_awarded"].Object.(int64); v != 250 {
		t.Errorf("quest.verdict.xp_awarded = %d, want 250", v)
	}
	if v, _ := idx["quest.verdict.feedback"].Object.(string); v != "Excellent work" {
		t.Errorf("quest.verdict.feedback = %q, want %q", v, "Excellent work")
	}

	// Duration.
	if v, _ := idx["quest.duration"].Object.(string); !strings.Contains(v, "m") {
		t.Errorf("quest.duration = %q, want a duration string containing 'm'", v)
	}

	// Lifecycle timestamps.
	if _, ok := idx["quest.lifecycle.claimed_at"]; !ok {
		t.Error("quest.lifecycle.claimed_at predicate not found")
	}
	if _, ok := idx["quest.lifecycle.started_at"]; !ok {
		t.Error("quest.lifecycle.started_at predicate not found")
	}
	if _, ok := idx["quest.lifecycle.completed_at"]; !ok {
		t.Error("quest.lifecycle.completed_at predicate not found")
	}

	// Skills — repeated predicates.
	skillTriples := triplesForPredicate(triples, "quest.skill.required")
	if len(skillTriples) != 2 {
		t.Errorf("quest.skill.required count = %d, want 2", len(skillTriples))
	}

	// Tools — repeated predicates.
	toolTriples := triplesForPredicate(triples, "quest.tool.required")
	if len(toolTriples) != 2 {
		t.Errorf("quest.tool.required count = %d, want 2", len(toolTriples))
	}

	// DependsOn — repeated predicates.
	depTriples := triplesForPredicate(triples, "quest.dependency.quest")
	if len(depTriples) != 1 {
		t.Errorf("quest.dependency.quest count = %d, want 1", len(depTriples))
	}
	if v, _ := depTriples[0].Object.(string); v != "c360.dev.game.board1.quest.dep1" {
		t.Errorf("quest.dependency.quest = %q, want %q", v, "c360.dev.game.board1.quest.dep1")
	}

	// Acceptance criteria — repeated predicates.
	acceptTriples := triplesForPredicate(triples, "quest.acceptance.criterion")
	if len(acceptTriples) != 2 {
		t.Errorf("quest.acceptance.criterion count = %d, want 2", len(acceptTriples))
	}
}

func TestQuest_Triples_EscalatedAndFailureInfo(t *testing.T) {
	q := &domain.Quest{
		ID:            domain.QuestID("c360.dev.game.board1.quest.escalated"),
		Title:         "Escalated Quest",
		Status:        domain.QuestEscalated,
		Escalated:     true,
		FailureReason: "Something went wrong",
		FailureType:   domain.FailureQuality,
		PostedAt:      time.Now(),
	}

	triples := q.Triples()
	idx := triplesByPredicate(triples)

	if v, _ := idx["quest.failure.escalated"].Object.(bool); !v {
		t.Error("quest.failure.escalated should be true")
	}
	if v, _ := idx["quest.failure.reason"].Object.(string); v != "Something went wrong" {
		t.Errorf("quest.failure.reason = %q, want %q", v, "Something went wrong")
	}
	if v, _ := idx["quest.failure.type"].Object.(string); v != string(domain.FailureQuality) {
		t.Errorf("quest.failure.type = %q, want %q", v, domain.FailureQuality)
	}
}

func TestQuest_Triples_NoEscalationWhenNotEscalated(t *testing.T) {
	q := &domain.Quest{
		ID:       domain.QuestID("c360.dev.game.board1.quest.normal"),
		Title:    "Normal Quest",
		Status:   domain.QuestPosted,
		Escalated: false,
		PostedAt: time.Now(),
	}

	triples := q.Triples()

	for _, tr := range triples {
		if tr.Predicate == "quest.failure.escalated" {
			t.Error("quest.failure.escalated should not appear when Escalated is false")
		}
	}
}

func TestQuest_Triples_NoOptionalTimestampsWhenNil(t *testing.T) {
	q := &domain.Quest{
		ID:       domain.QuestID("c360.dev.game.board1.quest.notimestamps"),
		Title:    "No Timestamps Quest",
		Status:   domain.QuestPosted,
		PostedAt: time.Now(),
		// ClaimedAt, StartedAt, CompletedAt all nil
	}

	triples := q.Triples()
	idx := triplesByPredicate(triples)

	if _, ok := idx["quest.lifecycle.claimed_at"]; ok {
		t.Error("quest.lifecycle.claimed_at should not appear when ClaimedAt is nil")
	}
	if _, ok := idx["quest.lifecycle.started_at"]; ok {
		t.Error("quest.lifecycle.started_at should not appear when StartedAt is nil")
	}
	if _, ok := idx["quest.lifecycle.completed_at"]; ok {
		t.Error("quest.lifecycle.completed_at should not appear when CompletedAt is nil")
	}
}

func TestQuest_Triples_NoVerdictWhenNil(t *testing.T) {
	q := &domain.Quest{
		ID:       domain.QuestID("c360.dev.game.board1.quest.noeverdict"),
		Title:    "No Verdict Quest",
		Status:   domain.QuestInProgress,
		PostedAt: time.Now(),
		Verdict:  nil,
	}

	triples := q.Triples()
	idx := triplesByPredicate(triples)

	if _, ok := idx["quest.verdict.passed"]; ok {
		t.Error("quest.verdict.passed should not appear when Verdict is nil")
	}
	if _, ok := idx["quest.verdict.score"]; ok {
		t.Error("quest.verdict.score should not appear when Verdict is nil")
	}
}

func TestQuest_Triples_NoOptionalRelationshipsWhenNil(t *testing.T) {
	q := &domain.Quest{
		ID:       domain.QuestID("c360.dev.game.board1.quest.norels"),
		Title:    "No Relations Quest",
		Status:   domain.QuestPosted,
		PostedAt: time.Now(),
		// ClaimedBy, PartyID, GuildPriority, ParentQuest all nil
	}

	triples := q.Triples()
	idx := triplesByPredicate(triples)

	optionalPreds := []string{
		"quest.assignment.agent",
		"quest.assignment.party",
		"quest.priority.guild",
		"quest.parent.quest",
		"quest.execution.loop_id",
	}
	for _, pred := range optionalPreds {
		if _, ok := idx[pred]; ok {
			t.Errorf("predicate %q should not appear when field is nil/empty", pred)
		}
	}
}

func TestQuest_Triples_NoDurationWhenZero(t *testing.T) {
	q := &domain.Quest{
		ID:       domain.QuestID("c360.dev.game.board1.quest.nodur"),
		Title:    "No Duration Quest",
		Status:   domain.QuestPosted,
		PostedAt: time.Now(),
		Duration: 0,
	}

	triples := q.Triples()
	idx := triplesByPredicate(triples)

	if _, ok := idx["quest.duration"]; ok {
		t.Error("quest.duration should not appear when Duration is zero")
	}
}

func TestQuest_Triples_NoFailureInfoWhenEmpty(t *testing.T) {
	q := &domain.Quest{
		ID:            domain.QuestID("c360.dev.game.board1.quest.nofail"),
		Title:         "No Failure Quest",
		Status:        domain.QuestPosted,
		PostedAt:      time.Now(),
		FailureReason: "",
		FailureType:   "",
	}

	triples := q.Triples()
	idx := triplesByPredicate(triples)

	if _, ok := idx["quest.failure.reason"]; ok {
		t.Error("quest.failure.reason should not appear when FailureReason is empty")
	}
	if _, ok := idx["quest.failure.type"]; ok {
		t.Error("quest.failure.type should not appear when domain.FailureType is empty")
	}
}

func TestQuest_Triples_MultipleSkills(t *testing.T) {
	skills := []domain.SkillTag{
		domain.SkillCodeGen,
		domain.SkillAnalysis,
		domain.SkillResearch,
	}
	q := &domain.Quest{
		ID:             domain.QuestID("c360.dev.game.board1.quest.multiskill"),
		Title:          "Multi-Skill Quest",
		Status:         domain.QuestPosted,
		RequiredSkills: skills,
		PostedAt:       time.Now(),
	}

	triples := q.Triples()
	skillTriples := triplesForPredicate(triples, "quest.skill.required")

	if len(skillTriples) != 3 {
		t.Errorf("quest.skill.required count = %d, want 3", len(skillTriples))
	}

	// Verify each skill appears once.
	found := make(map[domain.SkillTag]bool)
	for _, tr := range skillTriples {
		if v, ok := tr.Object.(string); ok {
			found[domain.SkillTag(v)] = true
		}
	}
	for _, skill := range skills {
		if !found[skill] {
			t.Errorf("skill %q not found in quest.skill.required triples", skill)
		}
	}
}

func TestQuest_Triples_MultipleDependencies(t *testing.T) {
	deps := []domain.QuestID{
		"c360.dev.game.board1.quest.dep1",
		"c360.dev.game.board1.quest.dep2",
	}
	q := &domain.Quest{
		ID:        domain.QuestID("c360.dev.game.board1.quest.witheps"),
		Title:     "Quest with Dependencies",
		Status:    domain.QuestPosted,
		DependsOn: deps,
		PostedAt:  time.Now(),
	}

	triples := q.Triples()
	depTriples := triplesForPredicate(triples, "quest.dependency.quest")

	if len(depTriples) != 2 {
		t.Errorf("quest.dependency.quest count = %d, want 2", len(depTriples))
	}

	found := make(map[string]bool)
	for _, tr := range depTriples {
		if v, ok := tr.Object.(string); ok {
			found[v] = true
		}
	}
	for _, dep := range deps {
		if !found[string(dep)] {
			t.Errorf("dependency %q not found in quest.dependency.quest triples", dep)
		}
	}
}

func TestQuest_Triples_SourceIsQuestboard(t *testing.T) {
	q := &domain.Quest{
		ID:       domain.QuestID("c360.dev.game.board1.quest.srctest"),
		Title:    "Source Test Quest",
		Status:   domain.QuestPosted,
		PostedAt: time.Now(),
	}

	triples := q.Triples()
	for _, tr := range triples {
		if tr.Source != "questboard" {
			t.Errorf("triple %q source = %q, want %q", tr.Predicate, tr.Source, "questboard")
		}
	}
}

func TestQuest_Triples_ConfidenceIsOne(t *testing.T) {
	q := &domain.Quest{
		ID:       domain.QuestID("c360.dev.game.board1.quest.conftest"),
		Title:    "Confidence Test Quest",
		Status:   domain.QuestPosted,
		PostedAt: time.Now(),
	}

	triples := q.Triples()
	for _, tr := range triples {
		if tr.Confidence != 1.0 {
			t.Errorf("triple %q confidence = %v, want 1.0", tr.Predicate, tr.Confidence)
		}
	}
}

func TestQuest_Triples_TimestampsAreRFC3339(t *testing.T) {
	postedAt := time.Now().Truncate(time.Second)
	claimedAt := postedAt.Add(5 * time.Minute)
	startedAt := postedAt.Add(10 * time.Minute)
	completedAt := postedAt.Add(30 * time.Minute)

	claimedBy := domain.AgentID("c360.dev.game.board1.agent.a1")

	q := &domain.Quest{
		ID:          domain.QuestID("c360.dev.game.board1.quest.tstest"),
		Title:       "Timestamp Test",
		Status:      domain.QuestCompleted,
		PostedAt:    postedAt,
		ClaimedAt:   &claimedAt,
		StartedAt:   &startedAt,
		CompletedAt: &completedAt,
		ClaimedBy:   &claimedBy,
	}

	triples := q.Triples()
	idx := triplesByPredicate(triples)

	tsPredicates := map[string]time.Time{
		"quest.lifecycle.posted_at":    postedAt,
		"quest.lifecycle.claimed_at":   claimedAt,
		"quest.lifecycle.started_at":   startedAt,
		"quest.lifecycle.completed_at": completedAt,
	}

	for pred, wantTime := range tsPredicates {
		tr, ok := idx[pred]
		if !ok {
			t.Errorf("predicate %q not found", pred)
			continue
		}
		v, ok := tr.Object.(string)
		if !ok {
			t.Errorf("%q object is not a string: %T", pred, tr.Object)
			continue
		}
		parsed, err := time.Parse(time.RFC3339, v)
		if err != nil {
			t.Errorf("%q value %q is not valid RFC3339: %v", pred, v, err)
			continue
		}
		if !parsed.Equal(wantTime) {
			t.Errorf("%q time = %v, want %v", pred, parsed, wantTime)
		}
	}
}

// =============================================================================
// FAILURE TYPE CONSTANT TESTS
// =============================================================================

func TestFailureType_Constants(t *testing.T) {
	tests := []struct {
		name  string
		value domain.FailureType
		want  string
	}{
		{name: "quality", value: domain.FailureQuality, want: "quality"},
		{name: "timeout", value: domain.FailureTimeout, want: "timeout"},
		{name: "error", value: domain.FailureError, want: "error"},
		{name: "abandoned", value: domain.FailureAbandoned, want: "abandoned"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.value) != tt.want {
				t.Errorf("domain.FailureType %q = %q, want %q", tt.name, tt.value, tt.want)
			}
		})
	}
}
