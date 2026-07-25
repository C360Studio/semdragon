package dmsession

import (
	"encoding/json"
	"reflect"
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
// PAYLOAD JSON WIRE TESTS
// =============================================================================

func TestSessionStartPayload_JSONWireContract(t *testing.T) {
	startedAt := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	payload := SessionStartPayload{
		SessionID: "c360.prod.game.board1.session.abc123",
		Config: domain.SessionConfig{
			Mode: domain.DMManual,
			Name: "Adventure Session",
		},
		StartedAt: startedAt,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal SessionStartPayload: %v", err)
	}
	assertExactJSONKeys(t, data, "session_id", "config", "started_at")

	var decoded SessionStartPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal SessionStartPayload: %v", err)
	}
	if decoded.SessionID != payload.SessionID || !reflect.DeepEqual(decoded.Config, payload.Config) ||
		!decoded.StartedAt.Equal(payload.StartedAt) {
		t.Errorf("decoded start payload = %#v, want %#v", decoded, payload)
	}
}

func TestSessionEndPayload_JSONWireContractPreservesSummaryCounters(t *testing.T) {
	endedAt := time.Date(2026, 7, 25, 13, 0, 0, 0, time.UTC)
	payload := SessionEndPayload{
		SessionID: "c360.prod.game.board1.session.abc123",
		Summary: domain.SessionSummary{
			SessionID:       "c360.prod.game.board1.session.abc123",
			QuestsCompleted: 7,
			QuestsFailed:    2,
		},
		EndedAt: endedAt,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal SessionEndPayload: %v", err)
	}
	assertExactJSONKeys(t, data, "session_id", "summary", "ended_at")

	var envelope struct {
		Summary json.RawMessage `json:"summary"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("unmarshal end envelope: %v", err)
	}
	var summary map[string]json.RawMessage
	if err := json.Unmarshal(envelope.Summary, &summary); err != nil {
		t.Fatalf("unmarshal end summary: %v", err)
	}
	for key, want := range map[string]int{"quests_completed": 7, "quests_failed": 2} {
		raw, ok := summary[key]
		if !ok {
			t.Errorf("summary JSON missing wire property %q", key)
			continue
		}
		var got int
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Errorf("decode summary property %q: %v", key, err)
		} else if got != want {
			t.Errorf("summary property %q = %d, want %d", key, got, want)
		}
	}

	var decoded SessionEndPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal SessionEndPayload: %v", err)
	}
	if decoded.SessionID != payload.SessionID || !reflect.DeepEqual(decoded.Summary, payload.Summary) ||
		!decoded.EndedAt.Equal(payload.EndedAt) {
		t.Errorf("decoded end payload = %#v, want %#v", decoded, payload)
	}
}

func assertExactJSONKeys(t *testing.T, data []byte, want ...string) {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatalf("decode JSON object: %v", err)
	}
	got := make(map[string]struct{}, len(object))
	for key := range object {
		got[key] = struct{}{}
	}
	expected := make(map[string]struct{}, len(want))
	for _, key := range want {
		expected[key] = struct{}{}
	}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("JSON keys = %v, want exactly %v", got, expected)
	}
}
