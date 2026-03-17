package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semstreams/graph"
)

// =============================================================================
// Lesson struct JSON round-trip
// =============================================================================

func TestLesson_JSONRoundTrip(t *testing.T) {
	rtQuestID := QuestID("c360.prod.game.board1.quest.rt001")
	original := Lesson{
		ID:           "lesson-q1-0",
		Skill:        SkillCodeReview,
		Category:     LessonSecurity,
		Summary:      "SQL injection risk in query builder",
		Detail:       "User input passed directly without sanitization",
		Severity:     LessonSeverityCritical,
		Positive:     false,
		QuestID:      QuestID("c360.prod.game.board1.quest.q1"),
		DiscoveredBy: AgentID("c360.prod.game.board1.agent.redteamer"),
		GuildID:      GuildID("c360.prod.game.board1.guild.g1"),
		RedTeamQuest: &rtQuestID,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}

	var restored Lesson
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}

	if restored.ID != original.ID {
		t.Errorf("ID = %q, want %q", restored.ID, original.ID)
	}
	if restored.Skill != original.Skill {
		t.Errorf("Skill = %q, want %q", restored.Skill, original.Skill)
	}
	if restored.Category != original.Category {
		t.Errorf("Category = %q, want %q", restored.Category, original.Category)
	}
	if restored.Summary != original.Summary {
		t.Errorf("Summary = %q, want %q", restored.Summary, original.Summary)
	}
	if restored.Detail != original.Detail {
		t.Errorf("Detail = %q, want %q", restored.Detail, original.Detail)
	}
	if restored.Severity != original.Severity {
		t.Errorf("Severity = %q, want %q", restored.Severity, original.Severity)
	}
	if restored.Positive != original.Positive {
		t.Errorf("Positive = %v, want %v", restored.Positive, original.Positive)
	}
	if restored.QuestID != original.QuestID {
		t.Errorf("QuestID = %q, want %q", restored.QuestID, original.QuestID)
	}
	if restored.DiscoveredBy != original.DiscoveredBy {
		t.Errorf("DiscoveredBy = %q, want %q", restored.DiscoveredBy, original.DiscoveredBy)
	}
	if restored.GuildID != original.GuildID {
		t.Errorf("GuildID = %q, want %q", restored.GuildID, original.GuildID)
	}
	if restored.RedTeamQuest == nil {
		t.Fatal("RedTeamQuest is nil after round-trip, want non-nil")
	}
	if *restored.RedTeamQuest != rtQuestID {
		t.Errorf("RedTeamQuest = %q, want %q", *restored.RedTeamQuest, rtQuestID)
	}
}

func TestLesson_JSONRoundTrip_NoOptionalFields(t *testing.T) {
	// RedTeamQuest and Detail are omitempty — ensure they round-trip as nil/empty.
	original := Lesson{
		ID:       "lesson-min",
		Summary:  "Minimal lesson",
		Category: LessonQuality,
		Severity: LessonSeverityInfo,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}

	var restored Lesson
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}

	if restored.RedTeamQuest != nil {
		t.Errorf("RedTeamQuest = %v, want nil", restored.RedTeamQuest)
	}
	if restored.Detail != "" {
		t.Errorf("Detail = %q, want empty", restored.Detail)
	}
}

// =============================================================================
// LessonCategory constants
// =============================================================================

func TestLessonCategory_Constants(t *testing.T) {
	categories := []LessonCategory{
		LessonCorrectness,
		LessonCompleteness,
		LessonQuality,
		LessonSecurity,
		LessonPerformance,
		LessonIntegration,
		LessonDocumentation,
		LessonBestPractice,
	}

	seen := make(map[LessonCategory]bool)
	for _, c := range categories {
		if c == "" {
			t.Errorf("LessonCategory constant is empty string")
		}
		if seen[c] {
			t.Errorf("duplicate LessonCategory constant: %q", c)
		}
		seen[c] = true
	}
}

// =============================================================================
// LessonSeverity constants
// =============================================================================

func TestLessonSeverity_Constants(t *testing.T) {
	severities := []LessonSeverity{
		LessonSeverityInfo,
		LessonSeverityWarning,
		LessonSeverityCritical,
	}

	seen := make(map[LessonSeverity]bool)
	for _, s := range severities {
		if s == "" {
			t.Errorf("LessonSeverity constant is empty string")
		}
		if seen[s] {
			t.Errorf("duplicate LessonSeverity constant: %q", s)
		}
		seen[s] = true
	}
}

// =============================================================================
// asLessonsSlice — in-process typed slice path
// =============================================================================

func TestAsLessonsSlice_Nil(t *testing.T) {
	result := asLessonsSlice(nil)
	if result != nil {
		t.Errorf("asLessonsSlice(nil) = %v, want nil", result)
	}
}

func TestAsLessonsSlice_DirectTypedSlice(t *testing.T) {
	lessons := []Lesson{
		{
			ID:       "lesson-1",
			Skill:    SkillCodeGen,
			Category: LessonCorrectness,
			Summary:  "Off-by-one error",
			Severity: LessonSeverityWarning,
		},
	}

	result := asLessonsSlice(lessons)
	if len(result) != 1 {
		t.Fatalf("asLessonsSlice typed = %d lessons, want 1", len(result))
	}
	if result[0].ID != "lesson-1" {
		t.Errorf("ID = %q, want %q", result[0].ID, "lesson-1")
	}
	if result[0].Summary != "Off-by-one error" {
		t.Errorf("Summary = %q", result[0].Summary)
	}
}

func TestAsLessonsSlice_KVRoundTripMapSlice(t *testing.T) {
	// Simulate what NATS KV returns after JSON round-trip: []any of map[string]any.
	rtQuestID := "c360.prod.game.board1.quest.rt099"
	raw := []any{
		map[string]any{
			"id":             "lesson-kv-1",
			"skill":          "code_review",
			"category":       "security",
			"summary":        "Unsafe deserialization",
			"detail":         "Object graph deserialized without type validation",
			"severity":       "critical",
			"positive":       false,
			"quest_id":       "c360.prod.game.board1.quest.q99",
			"discovered_by":  "c360.prod.game.board1.agent.agent1",
			"guild_id":       "c360.prod.game.board1.guild.g99",
			"red_team_quest": rtQuestID,
		},
		map[string]any{
			"id":       "lesson-kv-2",
			"summary":  "Strong type safety",
			"category": "quality",
			"severity": "info",
			"positive": true,
		},
	}

	result := asLessonsSlice(raw)
	if len(result) != 2 {
		t.Fatalf("asLessonsSlice KV = %d lessons, want 2", len(result))
	}

	l0 := result[0]
	if l0.ID != "lesson-kv-1" {
		t.Errorf("l0.ID = %q, want %q", l0.ID, "lesson-kv-1")
	}
	if l0.Skill != SkillCodeReview {
		t.Errorf("l0.Skill = %q, want %q", l0.Skill, SkillCodeReview)
	}
	if l0.Category != LessonSecurity {
		t.Errorf("l0.Category = %q, want %q", l0.Category, LessonSecurity)
	}
	if l0.Summary != "Unsafe deserialization" {
		t.Errorf("l0.Summary = %q", l0.Summary)
	}
	if l0.Detail != "Object graph deserialized without type validation" {
		t.Errorf("l0.Detail = %q", l0.Detail)
	}
	if l0.Severity != LessonSeverityCritical {
		t.Errorf("l0.Severity = %q, want %q", l0.Severity, LessonSeverityCritical)
	}
	if l0.Positive {
		t.Error("l0.Positive should be false")
	}
	if l0.QuestID != QuestID("c360.prod.game.board1.quest.q99") {
		t.Errorf("l0.QuestID = %q", l0.QuestID)
	}
	if l0.DiscoveredBy != AgentID("c360.prod.game.board1.agent.agent1") {
		t.Errorf("l0.DiscoveredBy = %q", l0.DiscoveredBy)
	}
	if l0.GuildID != GuildID("c360.prod.game.board1.guild.g99") {
		t.Errorf("l0.GuildID = %q", l0.GuildID)
	}
	if l0.RedTeamQuest == nil {
		t.Fatal("l0.RedTeamQuest is nil, want non-nil")
	}
	if string(*l0.RedTeamQuest) != rtQuestID {
		t.Errorf("l0.RedTeamQuest = %q, want %q", *l0.RedTeamQuest, rtQuestID)
	}

	l1 := result[1]
	if l1.ID != "lesson-kv-2" {
		t.Errorf("l1.ID = %q, want %q", l1.ID, "lesson-kv-2")
	}
	if !l1.Positive {
		t.Error("l1.Positive should be true")
	}
}

func TestAsLessonsSlice_SkipsEntriesWithNoSummary(t *testing.T) {
	// Entries without a non-empty "summary" should be skipped by asLessonsSlice.
	raw := []any{
		map[string]any{
			// no "summary" key — should be skipped
			"id":       "lesson-nosummary",
			"category": "quality",
		},
		map[string]any{
			"id":      "lesson-has-summary",
			"summary": "Real finding",
		},
	}

	result := asLessonsSlice(raw)
	if len(result) != 1 {
		t.Fatalf("asLessonsSlice = %d lessons, want 1 (entry without summary skipped)", len(result))
	}
	if result[0].ID != "lesson-has-summary" {
		t.Errorf("result[0].ID = %q, want %q", result[0].ID, "lesson-has-summary")
	}
}

func TestAsLessonsSlice_NonMapItemsSkipped(t *testing.T) {
	raw := []any{
		"unexpected string item",
		42,
		map[string]any{
			"id":      "real-lesson",
			"summary": "Real lesson summary",
		},
	}

	result := asLessonsSlice(raw)
	if len(result) != 1 {
		t.Fatalf("asLessonsSlice = %d lessons, want 1", len(result))
	}
	if result[0].ID != "real-lesson" {
		t.Errorf("result[0].ID = %q, want %q", result[0].ID, "real-lesson")
	}
}

// =============================================================================
// Guild Triples + GuildFromEntityState round-trip for Lessons
// =============================================================================

func TestGuildRoundTrip_WithLessons(t *testing.T) {
	rtQuestID := QuestID("c360.prod.game.board1.quest.rt001")
	original := &Guild{
		ID:     GuildID("c360.prod.game.board1.guild.g1"),
		Name:   "Code Crafters",
		Status: GuildActive,
		Lessons: []Lesson{
			{
				ID:           "lesson-q1-0",
				Skill:        SkillCodeReview,
				Category:     LessonSecurity,
				Summary:      "Always sanitize user input",
				Detail:       "Prevents SQL injection and XSS",
				Severity:     LessonSeverityCritical,
				Positive:     false,
				QuestID:      QuestID("c360.prod.game.board1.quest.q1"),
				DiscoveredBy: AgentID("c360.prod.game.board1.agent.reviewer"),
				GuildID:      GuildID("c360.prod.game.board1.guild.g1"),
				RedTeamQuest: &rtQuestID,
			},
			{
				ID:       "lesson-q1-1",
				Skill:    SkillCodeGen,
				Category: LessonBestPractice,
				Summary:  "Use context cancellation for all I/O",
				Severity: LessonSeverityInfo,
				Positive: true,
				QuestID:  QuestID("c360.prod.game.board1.quest.q1"),
				GuildID:  GuildID("c360.prod.game.board1.guild.g1"),
			},
		},
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	restored := GuildFromEntityState(entity)
	if restored == nil {
		t.Fatal("GuildFromEntityState returned nil")
	}

	if len(restored.Lessons) != 2 {
		t.Fatalf("Lessons len = %d, want 2", len(restored.Lessons))
	}

	l0 := restored.Lessons[0]
	if l0.ID != "lesson-q1-0" {
		t.Errorf("Lessons[0].ID = %q, want %q", l0.ID, "lesson-q1-0")
	}
	if l0.Skill != SkillCodeReview {
		t.Errorf("Lessons[0].Skill = %q, want %q", l0.Skill, SkillCodeReview)
	}
	if l0.Category != LessonSecurity {
		t.Errorf("Lessons[0].Category = %q, want %q", l0.Category, LessonSecurity)
	}
	if l0.Summary != "Always sanitize user input" {
		t.Errorf("Lessons[0].Summary = %q", l0.Summary)
	}
	if l0.Detail != "Prevents SQL injection and XSS" {
		t.Errorf("Lessons[0].Detail = %q", l0.Detail)
	}
	if l0.Severity != LessonSeverityCritical {
		t.Errorf("Lessons[0].Severity = %q, want %q", l0.Severity, LessonSeverityCritical)
	}
	if l0.Positive {
		t.Error("Lessons[0].Positive should be false")
	}
	if l0.QuestID != QuestID("c360.prod.game.board1.quest.q1") {
		t.Errorf("Lessons[0].QuestID = %q", l0.QuestID)
	}
	if l0.DiscoveredBy != AgentID("c360.prod.game.board1.agent.reviewer") {
		t.Errorf("Lessons[0].DiscoveredBy = %q", l0.DiscoveredBy)
	}
	if l0.RedTeamQuest == nil {
		t.Fatal("Lessons[0].RedTeamQuest is nil, want non-nil")
	}
	if *l0.RedTeamQuest != rtQuestID {
		t.Errorf("Lessons[0].RedTeamQuest = %q, want %q", *l0.RedTeamQuest, rtQuestID)
	}

	l1 := restored.Lessons[1]
	if l1.ID != "lesson-q1-1" {
		t.Errorf("Lessons[1].ID = %q, want %q", l1.ID, "lesson-q1-1")
	}
	if !l1.Positive {
		t.Error("Lessons[1].Positive should be true")
	}
	if l1.RedTeamQuest != nil {
		t.Errorf("Lessons[1].RedTeamQuest should be nil, got %v", l1.RedTeamQuest)
	}
}

func TestGuildRoundTrip_NoLessons(t *testing.T) {
	original := &Guild{
		ID:     GuildID("c360.prod.game.board1.guild.empty"),
		Name:   "Empty Guild",
		Status: GuildActive,
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	restored := GuildFromEntityState(entity)
	if restored == nil {
		t.Fatal("GuildFromEntityState returned nil")
	}
	if len(restored.Lessons) != 0 {
		t.Errorf("Lessons = %v, want empty", restored.Lessons)
	}
}

// =============================================================================
// Quest Triples + QuestFromEntityState round-trip for QuestType fields
// =============================================================================

func TestQuestRoundTrip_QuestTypeNormal(t *testing.T) {
	original := &Quest{
		ID:       QuestID("c360.prod.game.board1.quest.normal1"),
		Title:    "Normal Quest",
		Status:   QuestPosted,
		PostedAt: time.Now().Truncate(time.Second),
	}
	// QuestType zero value is QuestTypeNormal ("").

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	restored := QuestFromEntityState(entity)
	if restored.QuestType != QuestTypeNormal {
		t.Errorf("QuestType = %q, want %q (empty/normal)", restored.QuestType, QuestTypeNormal)
	}
	if restored.RedTeamTarget != nil {
		t.Errorf("RedTeamTarget = %v, want nil", restored.RedTeamTarget)
	}
	if restored.RedTeamQuestID != nil {
		t.Errorf("RedTeamQuestID = %v, want nil", restored.RedTeamQuestID)
	}
}

func TestQuestRoundTrip_QuestTypeRedTeam(t *testing.T) {
	targetID := QuestID("c360.prod.game.board1.quest.target1")
	original := &Quest{
		ID:            QuestID("c360.prod.game.board1.quest.rt001"),
		Title:         "Red-Team Review: Target Quest",
		Status:        QuestPosted,
		PostedAt:      time.Now().Truncate(time.Second),
		QuestType:     QuestTypeRedTeam,
		RedTeamTarget: &targetID,
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	restored := QuestFromEntityState(entity)
	if restored.QuestType != QuestTypeRedTeam {
		t.Errorf("QuestType = %q, want %q", restored.QuestType, QuestTypeRedTeam)
	}
	if restored.RedTeamTarget == nil {
		t.Fatal("RedTeamTarget is nil, want non-nil")
	}
	if *restored.RedTeamTarget != targetID {
		t.Errorf("RedTeamTarget = %q, want %q", *restored.RedTeamTarget, targetID)
	}
	if restored.RedTeamQuestID != nil {
		t.Errorf("RedTeamQuestID = %v, want nil (not set)", restored.RedTeamQuestID)
	}
}

func TestQuestRoundTrip_RedTeamQuestIDPointer(t *testing.T) {
	// A normal quest that has been assigned a red-team review quest.
	rtQuestID := QuestID("c360.prod.game.board1.quest.rt002")
	original := &Quest{
		ID:             QuestID("c360.prod.game.board1.quest.original1"),
		Title:          "Original Quest With Red-Team Review",
		Status:         QuestInReview,
		PostedAt:       time.Now().Truncate(time.Second),
		QuestType:      QuestTypeNormal,
		RedTeamQuestID: &rtQuestID,
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	restored := QuestFromEntityState(entity)
	if restored.QuestType != QuestTypeNormal {
		t.Errorf("QuestType = %q, want normal", restored.QuestType)
	}
	if restored.RedTeamQuestID == nil {
		t.Fatal("RedTeamQuestID is nil, want non-nil")
	}
	if *restored.RedTeamQuestID != rtQuestID {
		t.Errorf("RedTeamQuestID = %q, want %q", *restored.RedTeamQuestID, rtQuestID)
	}
}

func TestQuestRoundTrip_BothRedTeamPointers(t *testing.T) {
	// A quest with both pointers set — round-trip must be lossless.
	targetID := QuestID("c360.prod.game.board1.quest.targetX")
	rtQuestID := QuestID("c360.prod.game.board1.quest.rtX")
	original := &Quest{
		ID:             QuestID("c360.prod.game.board1.quest.both"),
		Title:          "Both Pointers",
		Status:         QuestPosted,
		PostedAt:       time.Now().Truncate(time.Second),
		QuestType:      QuestTypeRedTeam,
		RedTeamTarget:  &targetID,
		RedTeamQuestID: &rtQuestID,
	}

	entity := &graph.EntityState{
		ID:      string(original.ID),
		Triples: original.Triples(),
	}

	restored := QuestFromEntityState(entity)
	if restored.QuestType != QuestTypeRedTeam {
		t.Errorf("QuestType = %q, want %q", restored.QuestType, QuestTypeRedTeam)
	}
	if restored.RedTeamTarget == nil || *restored.RedTeamTarget != targetID {
		t.Errorf("RedTeamTarget = %v, want %q", restored.RedTeamTarget, targetID)
	}
	if restored.RedTeamQuestID == nil || *restored.RedTeamQuestID != rtQuestID {
		t.Errorf("RedTeamQuestID = %v, want %q", restored.RedTeamQuestID, rtQuestID)
	}
}

// =============================================================================
// QuestType constant correctness
// =============================================================================

func TestQuestType_ZeroValueIsNormal(t *testing.T) {
	var qt QuestType
	if qt != QuestTypeNormal {
		t.Errorf("zero value QuestType = %q, want QuestTypeNormal (%q)", qt, QuestTypeNormal)
	}
}

func TestQuestType_RedTeamConstant(t *testing.T) {
	if QuestTypeRedTeam == "" {
		t.Error("QuestTypeRedTeam should not be empty string (reserved for normal)")
	}
	if QuestTypeRedTeam == QuestTypeNormal {
		t.Error("QuestTypeRedTeam must differ from QuestTypeNormal")
	}
}
