package dmsession

import (
	"testing"
	"time"

	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// CONFIG TESTS
// =============================================================================
// Tests for DefaultConfig() - verifies sensible defaults without any external
// dependencies. No NATS or Docker required.
// =============================================================================

func TestDefaultConfig_SensibleDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Org == "" {
		t.Error("Org must not be empty")
	}
	if cfg.Platform == "" {
		t.Error("Platform must not be empty")
	}
	if cfg.Board == "" {
		t.Error("Board must not be empty")
	}
	if cfg.DefaultMode == "" {
		t.Error("DefaultMode must not be empty")
	}
	if cfg.MaxConcurrent <= 0 {
		t.Errorf("MaxConcurrent = %d, want > 0", cfg.MaxConcurrent)
	}
	if cfg.SessionTimeoutMin <= 0 {
		t.Errorf("SessionTimeoutMin = %d, want > 0", cfg.SessionTimeoutMin)
	}
}

func TestDefaultConfig_ExpectedValues(t *testing.T) {
	cfg := DefaultConfig()

	tests := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"Org", cfg.Org, "default"},
		{"Platform", cfg.Platform, "local"},
		{"Board", cfg.Board, "main"},
		{"DefaultMode", cfg.DefaultMode, "manual"},
		{"MaxConcurrent", cfg.MaxConcurrent, 10},
		{"AutoEscalate", cfg.AutoEscalate, true},
		{"SessionTimeoutMin", cfg.SessionTimeoutMin, 60},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

// =============================================================================
// SESSION KEY TESTS
// =============================================================================
// Pure formatting test - no external dependencies.
// =============================================================================

func TestSessionKey_Format(t *testing.T) {
	// Build a minimal SessionManager for calling SessionKey.
	// NewSessionManager requires a live NATS client, but SessionKey is a pure
	// string formatter so we can construct the struct directly.
	sm := &SessionManager{}

	tests := []struct {
		instance string
		want     string
	}{
		{"abc123", "session.abc123"},
		{"xyz-789", "session.xyz-789"},
		{"", "session."},
		{"some.instance", "session.some.instance"},
	}

	for _, tt := range tests {
		t.Run(tt.instance, func(t *testing.T) {
			got := sm.SessionKey(tt.instance)
			if got != tt.want {
				t.Errorf("SessionKey(%q) = %q, want %q", tt.instance, got, tt.want)
			}
		})
	}
}

// =============================================================================
// CONTAINS EVENT TYPE HELPER TESTS
// =============================================================================
// These package-level helpers are pure predicates over slices of
// domain.GameEventType. They require no external dependencies.
// =============================================================================

// --- containsQuestEventTypes ---

func TestContainsQuestEventTypes_EmptySlice(t *testing.T) {
	if containsQuestEventTypes(nil) {
		t.Error("nil slice: expected false, got true")
	}
	if containsQuestEventTypes([]domain.GameEventType{}) {
		t.Error("empty slice: expected false, got true")
	}
}

func TestContainsQuestEventTypes_QuestTypesOnly(t *testing.T) {
	questTypes := []domain.GameEventType{
		domain.EventQuestPosted,
		domain.EventQuestClaimed,
		domain.EventQuestStarted,
		domain.EventQuestCompleted,
		domain.EventQuestFailed,
		domain.EventQuestEscalated,
	}

	for _, et := range questTypes {
		t.Run(string(et), func(t *testing.T) {
			if !containsQuestEventTypes([]domain.GameEventType{et}) {
				t.Errorf("containsQuestEventTypes([%q]) = false, want true", et)
			}
		})
	}
}

func TestContainsQuestEventTypes_NonQuestTypesOnly(t *testing.T) {
	nonQuestTypes := []domain.GameEventType{
		domain.EventAgentRecruited,
		domain.EventAgentLevelUp,
		domain.EventBattleStarted,
		domain.EventBattleVictory,
		domain.EventPartyFormed,
		domain.EventGuildCreated,
	}

	if containsQuestEventTypes(nonQuestTypes) {
		t.Error("non-quest types only: expected false, got true")
	}
}

func TestContainsQuestEventTypes_MixedTypes(t *testing.T) {
	mixed := []domain.GameEventType{
		domain.EventAgentLevelUp,
		domain.EventBattleDefeat,
		domain.EventQuestFailed, // one quest type in the mix
	}

	if !containsQuestEventTypes(mixed) {
		t.Error("mixed types including quest event: expected true, got false")
	}
}

func TestContainsQuestEventTypes_AllNonQuestNeverReturnsTrue(t *testing.T) {
	// Verify all known non-quest types together still return false.
	types := []domain.GameEventType{
		domain.EventAgentRecruited,
		domain.EventAgentLevelUp,
		domain.EventAgentLevelDown,
		domain.EventAgentDeath,
		domain.EventAgentPermadeath,
		domain.EventAgentRevived,
		domain.EventBattleStarted,
		domain.EventBattleVictory,
		domain.EventBattleDefeat,
	}

	if containsQuestEventTypes(types) {
		t.Error("all-non-quest types: expected false, got true")
	}
}

// --- containsAgentEventTypes ---

func TestContainsAgentEventTypes_EmptySlice(t *testing.T) {
	if containsAgentEventTypes(nil) {
		t.Error("nil slice: expected false, got true")
	}
	if containsAgentEventTypes([]domain.GameEventType{}) {
		t.Error("empty slice: expected false, got true")
	}
}

func TestContainsAgentEventTypes_AgentTypesOnly(t *testing.T) {
	agentTypes := []domain.GameEventType{
		domain.EventAgentRecruited,
		domain.EventAgentLevelUp,
		domain.EventAgentLevelDown,
		domain.EventAgentDeath,
		domain.EventAgentPermadeath,
		domain.EventAgentRevived,
	}

	for _, et := range agentTypes {
		t.Run(string(et), func(t *testing.T) {
			if !containsAgentEventTypes([]domain.GameEventType{et}) {
				t.Errorf("containsAgentEventTypes([%q]) = false, want true", et)
			}
		})
	}
}

func TestContainsAgentEventTypes_NonAgentTypesOnly(t *testing.T) {
	nonAgentTypes := []domain.GameEventType{
		domain.EventQuestPosted,
		domain.EventQuestCompleted,
		domain.EventBattleStarted,
		domain.EventBattleVictory,
		domain.EventPartyFormed,
	}

	if containsAgentEventTypes(nonAgentTypes) {
		t.Error("non-agent types only: expected false, got true")
	}
}

func TestContainsAgentEventTypes_MixedTypes(t *testing.T) {
	mixed := []domain.GameEventType{
		domain.EventQuestPosted,
		domain.EventBattleStarted,
		domain.EventAgentDeath, // one agent type in the mix
	}

	if !containsAgentEventTypes(mixed) {
		t.Error("mixed types including agent event: expected true, got false")
	}
}

// --- containsBattleEventTypes ---

func TestContainsBattleEventTypes_EmptySlice(t *testing.T) {
	if containsBattleEventTypes(nil) {
		t.Error("nil slice: expected false, got true")
	}
	if containsBattleEventTypes([]domain.GameEventType{}) {
		t.Error("empty slice: expected false, got true")
	}
}

func TestContainsBattleEventTypes_BattleTypesOnly(t *testing.T) {
	battleTypes := []domain.GameEventType{
		domain.EventBattleStarted,
		domain.EventBattleVictory,
		domain.EventBattleDefeat,
	}

	for _, et := range battleTypes {
		t.Run(string(et), func(t *testing.T) {
			if !containsBattleEventTypes([]domain.GameEventType{et}) {
				t.Errorf("containsBattleEventTypes([%q]) = false, want true", et)
			}
		})
	}
}

func TestContainsBattleEventTypes_NonBattleTypesOnly(t *testing.T) {
	nonBattleTypes := []domain.GameEventType{
		domain.EventQuestPosted,
		domain.EventAgentRecruited,
		domain.EventPartyFormed,
		domain.EventGuildCreated,
	}

	if containsBattleEventTypes(nonBattleTypes) {
		t.Error("non-battle types only: expected false, got true")
	}
}

func TestContainsBattleEventTypes_MixedTypes(t *testing.T) {
	mixed := []domain.GameEventType{
		domain.EventQuestCompleted,
		domain.EventAgentLevelUp,
		domain.EventBattleVictory, // one battle type in the mix
	}

	if !containsBattleEventTypes(mixed) {
		t.Error("mixed types including battle event: expected true, got false")
	}
}

// =============================================================================
// PAYLOAD VALIDATE TESTS
// =============================================================================

func TestSessionStartPayload_Validate_Valid(t *testing.T) {
	p := &SessionStartPayload{
		SessionID: "c360.prod.game.board1.session.abc123",
		Config: domain.SessionConfig{
			Mode: domain.DMManual,
			Name: "Adventure Session",
		},
		StartedAt: time.Now(),
	}

	if err := p.Validate(); err != nil {
		t.Errorf("Validate() unexpected error: %v", err)
	}
}

func TestSessionStartPayload_Validate_MissingSessionID(t *testing.T) {
	p := &SessionStartPayload{
		SessionID: "",
		StartedAt: time.Now(),
	}

	if err := p.Validate(); err == nil {
		t.Error("Validate() expected error for empty session_id, got nil")
	}
}

func TestSessionStartPayload_Validate_ZeroStartedAt(t *testing.T) {
	p := &SessionStartPayload{
		SessionID: "session-001",
		StartedAt: time.Time{}, // zero value
	}

	if err := p.Validate(); err == nil {
		t.Error("Validate() expected error for zero started_at, got nil")
	}
}

func TestSessionStartPayload_Validate_BothFieldsMissing(t *testing.T) {
	p := &SessionStartPayload{}

	if err := p.Validate(); err == nil {
		t.Error("Validate() expected error for zero-value payload, got nil")
	}
}

func TestSessionEndPayload_Validate_Valid(t *testing.T) {
	p := &SessionEndPayload{
		SessionID: "c360.prod.game.board1.session.xyz789",
		Summary: domain.SessionSummary{
			SessionID:       "c360.prod.game.board1.session.xyz789",
			QuestsCompleted: 5,
			QuestsFailed:    1,
		},
		EndedAt: time.Now(),
	}

	if err := p.Validate(); err != nil {
		t.Errorf("Validate() unexpected error: %v", err)
	}
}

func TestSessionEndPayload_Validate_MissingSessionID(t *testing.T) {
	p := &SessionEndPayload{
		SessionID: "",
		EndedAt:   time.Now(),
	}

	if err := p.Validate(); err == nil {
		t.Error("Validate() expected error for empty session_id, got nil")
	}
}

func TestSessionEndPayload_Validate_ZeroEndedAt(t *testing.T) {
	p := &SessionEndPayload{
		SessionID: "session-001",
		EndedAt:   time.Time{}, // zero value
	}

	if err := p.Validate(); err == nil {
		t.Error("Validate() expected error for zero ended_at, got nil")
	}
}

func TestSessionEndPayload_Validate_BothFieldsMissing(t *testing.T) {
	p := &SessionEndPayload{}

	if err := p.Validate(); err == nil {
		t.Error("Validate() expected error for zero-value payload, got nil")
	}
}

// =============================================================================
// PAYLOAD TRIPLES TESTS
// =============================================================================

func TestSessionStartPayload_Triples_ContainsExpectedPredicates(t *testing.T) {
	now := time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC)
	p := &SessionStartPayload{
		SessionID: "c360.prod.game.board1.session.abc123",
		Config: domain.SessionConfig{
			Mode: domain.DMManual,
			Name: "My Session",
		},
		StartedAt: now,
	}

	triples := p.Triples()

	if len(triples) == 0 {
		t.Fatal("Triples() returned empty slice")
	}

	// Build a lookup by predicate for assertion convenience.
	// Object is any, so we store it as-is and cast to string where needed.
	byPredicate := make(map[string]any, len(triples))
	for _, tr := range triples {
		byPredicate[tr.Predicate] = tr.Object
	}

	// Every triple must carry the correct subject.
	for _, tr := range triples {
		if tr.Subject != p.SessionID {
			t.Errorf("triple subject = %q, want %q (predicate: %q)", tr.Subject, p.SessionID, tr.Predicate)
		}
	}

	// Verify the mode triple.
	if raw, ok := byPredicate["dm.session.mode"]; !ok {
		t.Error("missing dm.session.mode triple")
	} else if got, _ := raw.(string); got != string(domain.DMManual) {
		t.Errorf("dm.session.mode = %q, want %q", got, string(domain.DMManual))
	}

	// Verify the name triple.
	if raw, ok := byPredicate["dm.session.name"]; !ok {
		t.Error("missing dm.session.name triple")
	} else if got, _ := raw.(string); got != "My Session" {
		t.Errorf("dm.session.name = %q, want %q", got, "My Session")
	}

	// Verify the start timestamp triple uses the session-start predicate.
	if _, ok := byPredicate[domain.PredicateSessionStart]; !ok {
		t.Errorf("missing %q triple", domain.PredicateSessionStart)
	}
}

func TestSessionStartPayload_Triples_TimestampFormat(t *testing.T) {
	fixedTime := time.Date(2026, 3, 2, 15, 30, 0, 0, time.UTC)
	p := &SessionStartPayload{
		SessionID: "session-ts-test",
		StartedAt: fixedTime,
	}

	triples := p.Triples()

	for _, tr := range triples {
		if tr.Predicate == domain.PredicateSessionStart {
			want := fixedTime.Format(time.RFC3339)
			got, _ := tr.Object.(string)
			if got != want {
				t.Errorf("start timestamp triple = %q, want %q", got, want)
			}
			return
		}
	}

	t.Errorf("no triple with predicate %q found", domain.PredicateSessionStart)
}

func TestSessionStartPayload_EntityID(t *testing.T) {
	p := &SessionStartPayload{
		SessionID: "c360.prod.game.board1.session.myid",
	}

	if got := p.EntityID(); got != p.SessionID {
		t.Errorf("EntityID() = %q, want %q", got, p.SessionID)
	}
}

func TestSessionStartPayload_Schema(t *testing.T) {
	p := &SessionStartPayload{}

	if got := p.Schema(); got != "session.start" {
		t.Errorf("Schema() = %q, want %q", got, "session.start")
	}
}

func TestSessionEndPayload_Triples_ContainsExpectedPredicates(t *testing.T) {
	now := time.Date(2026, 3, 2, 18, 0, 0, 0, time.UTC)
	p := &SessionEndPayload{
		SessionID: "c360.prod.game.board1.session.xyz789",
		Summary: domain.SessionSummary{
			SessionID:       "c360.prod.game.board1.session.xyz789",
			QuestsCompleted: 7,
			QuestsFailed:    2,
		},
		EndedAt: now,
	}

	triples := p.Triples()

	if len(triples) == 0 {
		t.Fatal("Triples() returned empty slice")
	}

	// Build a lookup by predicate.
	// Object is any, so we store it as-is and cast to string where needed.
	byPredicate := make(map[string]any, len(triples))
	for _, tr := range triples {
		byPredicate[tr.Predicate] = tr.Object
	}

	// Every triple must carry the correct subject.
	for _, tr := range triples {
		if tr.Subject != p.SessionID {
			t.Errorf("triple subject = %q, want %q (predicate: %q)", tr.Subject, p.SessionID, tr.Predicate)
		}
	}

	// Verify the end timestamp triple.
	if _, ok := byPredicate[domain.PredicateSessionEnd]; !ok {
		t.Errorf("missing %q triple", domain.PredicateSessionEnd)
	}

	// Verify quests_completed triple.
	if raw, ok := byPredicate["dm.session.quests_completed"]; !ok {
		t.Error("missing dm.session.quests_completed triple")
	} else if got, _ := raw.(string); got != "7" {
		t.Errorf("dm.session.quests_completed = %q, want %q", got, "7")
	}

	// Verify quests_failed triple.
	if raw, ok := byPredicate["dm.session.quests_failed"]; !ok {
		t.Error("missing dm.session.quests_failed triple")
	} else if got, _ := raw.(string); got != "2" {
		t.Errorf("dm.session.quests_failed = %q, want %q", got, "2")
	}
}

func TestSessionEndPayload_Triples_TimestampFormat(t *testing.T) {
	fixedTime := time.Date(2026, 3, 2, 20, 45, 0, 0, time.UTC)
	p := &SessionEndPayload{
		SessionID: "session-end-ts-test",
		EndedAt:   fixedTime,
	}

	triples := p.Triples()

	for _, tr := range triples {
		if tr.Predicate == domain.PredicateSessionEnd {
			want := fixedTime.Format(time.RFC3339)
			got, _ := tr.Object.(string)
			if got != want {
				t.Errorf("end timestamp triple object = %q, want %q", got, want)
			}
			return
		}
	}

	t.Errorf("no triple with predicate %q found", domain.PredicateSessionEnd)
}

func TestSessionEndPayload_Triples_ZeroSummary(t *testing.T) {
	// A zero-value summary must still produce valid triples with "0" as the counts.
	p := &SessionEndPayload{
		SessionID: "session-zero",
		Summary:   domain.SessionSummary{},
		EndedAt:   time.Now(),
	}

	triples := p.Triples()

	byPredicate := make(map[string]any, len(triples))
	for _, tr := range triples {
		byPredicate[tr.Predicate] = tr.Object
	}

	if got, _ := byPredicate["dm.session.quests_completed"].(string); got != "0" {
		t.Errorf("dm.session.quests_completed = %q, want %q", got, "0")
	}
	if got, _ := byPredicate["dm.session.quests_failed"].(string); got != "0" {
		t.Errorf("dm.session.quests_failed = %q, want %q", got, "0")
	}
}

func TestSessionEndPayload_EntityID(t *testing.T) {
	p := &SessionEndPayload{
		SessionID: "c360.prod.game.board1.session.myid",
	}

	if got := p.EntityID(); got != p.SessionID {
		t.Errorf("EntityID() = %q, want %q", got, p.SessionID)
	}
}

func TestSessionEndPayload_Schema(t *testing.T) {
	p := &SessionEndPayload{}

	if got := p.Schema(); got != "session.end" {
		t.Errorf("Schema() = %q, want %q", got, "session.end")
	}
}
