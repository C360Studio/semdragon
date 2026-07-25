package guildformation

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// DefaultConfig
// =============================================================================

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Org != "default" {
		t.Errorf("Org = %q; want %q", cfg.Org, "default")
	}
	if cfg.Platform != "local" {
		t.Errorf("Platform = %q; want %q", cfg.Platform, "local")
	}
	if cfg.Board != "main" {
		t.Errorf("Board = %q; want %q", cfg.Board, "main")
	}
	if cfg.MinMembersForFormation != 3 {
		t.Errorf("MinMembersForFormation = %d; want 3", cfg.MinMembersForFormation)
	}
	if cfg.MaxGuildSize != 20 {
		t.Errorf("MaxGuildSize = %d; want 20", cfg.MaxGuildSize)
	}
}

func TestDefaultConfig_ToBoardConfig(t *testing.T) {
	cfg := DefaultConfig()
	bc := cfg.ToBoardConfig()

	if bc == nil {
		t.Fatal("ToBoardConfig() returned nil")
	}
	if bc.Org != cfg.Org {
		t.Errorf("BoardConfig.Org = %q; want %q", bc.Org, cfg.Org)
	}
	if bc.Platform != cfg.Platform {
		t.Errorf("BoardConfig.Platform = %q; want %q", bc.Platform, cfg.Platform)
	}
	if bc.Board != cfg.Board {
		t.Errorf("BoardConfig.Board = %q; want %q", bc.Board, cfg.Board)
	}
}

// =============================================================================
// GuildCreatedPayload.Validate
// =============================================================================

func TestGuildCreatedPayload_Validate_Valid(t *testing.T) {
	p := &GuildCreatedPayload{
		Guild:     domain.Guild{ID: "guild-1"},
		FounderID: "agent-1",
		Timestamp: time.Now(),
	}
	if err := p.Validate(); err != nil {
		t.Errorf("Validate() returned unexpected error: %v", err)
	}
}

func TestGuildCreatedPayload_Validate_MissingGuildID(t *testing.T) {
	p := &GuildCreatedPayload{
		Guild:     domain.Guild{ID: ""},
		FounderID: "agent-1",
		Timestamp: time.Now(),
	}
	if err := p.Validate(); err == nil {
		t.Error("Validate() expected error for missing guild_id, got nil")
	}
}

func TestGuildCreatedPayload_Validate_MissingFounderID(t *testing.T) {
	p := &GuildCreatedPayload{
		Guild:     domain.Guild{ID: "guild-1"},
		FounderID: "",
		Timestamp: time.Now(),
	}
	if err := p.Validate(); err == nil {
		t.Error("Validate() expected error for missing founder_id, got nil")
	}
}

func TestGuildCreatedPayload_Validate_ZeroTimestamp(t *testing.T) {
	p := &GuildCreatedPayload{
		Guild:     domain.Guild{ID: "guild-1"},
		FounderID: "agent-1",
		Timestamp: time.Time{},
	}
	if err := p.Validate(); err == nil {
		t.Error("Validate() expected error for zero timestamp, got nil")
	}
}

// =============================================================================
// GuildJoinedPayload.Validate
// =============================================================================

func TestGuildJoinedPayload_Validate_Valid(t *testing.T) {
	p := &GuildJoinedPayload{
		GuildID:   "guild-1",
		AgentID:   "agent-1",
		Timestamp: time.Now(),
	}
	if err := p.Validate(); err != nil {
		t.Errorf("Validate() returned unexpected error: %v", err)
	}
}

func TestGuildJoinedPayload_Validate_MissingGuildID(t *testing.T) {
	p := &GuildJoinedPayload{
		GuildID:   "",
		AgentID:   "agent-1",
		Timestamp: time.Now(),
	}
	if err := p.Validate(); err == nil {
		t.Error("Validate() expected error for missing guild_id, got nil")
	}
}

func TestGuildJoinedPayload_Validate_MissingAgentID(t *testing.T) {
	p := &GuildJoinedPayload{
		GuildID:   "guild-1",
		AgentID:   "",
		Timestamp: time.Now(),
	}
	if err := p.Validate(); err == nil {
		t.Error("Validate() expected error for missing agent_id, got nil")
	}
}

// =============================================================================
// GuildLeftPayload.Validate
// =============================================================================

func TestGuildLeftPayload_Validate_Valid(t *testing.T) {
	p := &GuildLeftPayload{
		GuildID:   "guild-1",
		AgentID:   "agent-1",
		Timestamp: time.Now(),
	}
	if err := p.Validate(); err != nil {
		t.Errorf("Validate() returned unexpected error: %v", err)
	}
}

func TestGuildLeftPayload_Validate_MissingGuildID(t *testing.T) {
	p := &GuildLeftPayload{
		GuildID: "",
		AgentID: "agent-1",
	}
	if err := p.Validate(); err == nil {
		t.Error("Validate() expected error for missing guild_id, got nil")
	}
}

func TestGuildLeftPayload_Validate_MissingAgentID(t *testing.T) {
	p := &GuildLeftPayload{
		GuildID: "guild-1",
		AgentID: "",
	}
	if err := p.Validate(); err == nil {
		t.Error("Validate() expected error for missing agent_id, got nil")
	}
}

// =============================================================================
// GuildPromotedPayload.Validate
// =============================================================================

func TestGuildPromotedPayload_Validate_Valid(t *testing.T) {
	p := &GuildPromotedPayload{
		GuildID:   "guild-1",
		AgentID:   "agent-1",
		OldRank:   domain.GuildRankInitiate,
		NewRank:   domain.GuildRankMember,
		Timestamp: time.Now(),
	}
	if err := p.Validate(); err != nil {
		t.Errorf("Validate() returned unexpected error: %v", err)
	}
}

func TestGuildPromotedPayload_Validate_MissingGuildID(t *testing.T) {
	p := &GuildPromotedPayload{
		GuildID: "",
		AgentID: "agent-1",
		NewRank: domain.GuildRankMember,
	}
	if err := p.Validate(); err == nil {
		t.Error("Validate() expected error for missing guild_id, got nil")
	}
}

func TestGuildPromotedPayload_Validate_MissingAgentID(t *testing.T) {
	p := &GuildPromotedPayload{
		GuildID: "guild-1",
		AgentID: "",
		NewRank: domain.GuildRankMember,
	}
	if err := p.Validate(); err == nil {
		t.Error("Validate() expected error for missing agent_id, got nil")
	}
}

// =============================================================================
// GuildDisbandedPayload.Validate
// =============================================================================

func TestGuildDisbandedPayload_Validate_Valid(t *testing.T) {
	p := &GuildDisbandedPayload{
		GuildID:   "guild-1",
		Reason:    "quest objectives met",
		Timestamp: time.Now(),
	}
	if err := p.Validate(); err != nil {
		t.Errorf("Validate() returned unexpected error: %v", err)
	}
}

func TestGuildDisbandedPayload_Validate_MissingGuildID(t *testing.T) {
	p := &GuildDisbandedPayload{
		GuildID:   "",
		Reason:    "some reason",
		Timestamp: time.Now(),
	}
	if err := p.Validate(); err == nil {
		t.Error("Validate() expected error for missing guild_id, got nil")
	}
}

// GuildDisbandedPayload.Validate only requires GuildID — Reason and Timestamp
// are optional fields, so a missing reason must not be an error.
func TestGuildDisbandedPayload_Validate_EmptyReasonOK(t *testing.T) {
	p := &GuildDisbandedPayload{
		GuildID:   "guild-1",
		Reason:    "",
		Timestamp: time.Now(),
	}
	if err := p.Validate(); err != nil {
		t.Errorf("Validate() returned unexpected error for empty reason: %v", err)
	}
}

func TestGuildEventPayload_JSONWireContracts(t *testing.T) {
	eventTime := time.Date(2026, 7, 25, 15, 0, 0, 0, time.UTC)
	foundedAt := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	joinedAt := time.Date(2025, 2, 3, 4, 5, 6, 0, time.UTC)
	createdAt := time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)
	guildID := domain.GuildID("c360.prod.game.board1.guild.guild-1")
	agentID := domain.AgentID("c360.prod.game.board1.agent.agent-1")
	guild := domain.Guild{
		ID:          guildID,
		Name:        "Wire Guild",
		Description: "guild JSON contract",
		Status:      domain.GuildActive,
		Members: []domain.GuildMember{{
			AgentID: agentID, Rank: domain.GuildRankVeteran, JoinedAt: joinedAt, Contribution: 3.5,
		}},
		MaxMembers:       12,
		MinLevel:         4,
		Founded:          foundedAt,
		FoundedBy:        agentID,
		QuorumSize:       3,
		Culture:          "ship carefully",
		Motto:            "contracts endure",
		Reputation:       0.75,
		QuestsHandled:    10,
		SuccessRate:      0.8,
		QuestsFailed:     2,
		SharedTools:      []string{"lint"},
		QuestTypes:       []string{"code"},
		PreferredClients: []string{"c360"},
		CreatedAt:        createdAt,
	}

	wantGuild := map[string]any{
		"id":          string(guildID),
		"name":        "Wire Guild",
		"description": "guild JSON contract",
		"status":      string(domain.GuildActive),
		"members": []any{map[string]any{
			"agent_id": string(agentID), "rank": string(domain.GuildRankVeteran),
			"joined_at": joinedAt.Format(time.RFC3339), "contribution": 3.5,
		}},
		"max_members":       12,
		"min_level":         4,
		"founded":           foundedAt.Format(time.RFC3339),
		"founded_by":        string(agentID),
		"quorum_size":       3,
		"culture":           "ship carefully",
		"motto":             "contracts endure",
		"reputation":        0.75,
		"quests_handled":    10,
		"success_rate":      0.8,
		"quests_failed":     2,
		"shared_tools":      []any{"lint"},
		"quest_types":       []any{"code"},
		"preferred_clients": []any{"c360"},
		"created_at":        createdAt.Format(time.RFC3339),
	}
	trace := func(suffix string) TraceInfo {
		return TraceInfo{
			TrajectoryID: "trajectory-" + suffix,
			SpanID:       "span-" + suffix,
			ParentSpanID: "parent-" + suffix,
		}
	}
	wantTrace := func(suffix string) map[string]any {
		return map[string]any{
			"trajectory_id":  "trajectory-" + suffix,
			"span_id":        "span-" + suffix,
			"parent_span_id": "parent-" + suffix,
		}
	}

	tests := []struct {
		name    string
		payload any
		want    map[string]any
	}{
		{
			name: "created",
			payload: GuildCreatedPayload{
				Guild: guild, FounderID: agentID, Timestamp: eventTime, Trace: trace("created"),
			},
			want: map[string]any{
				"guild": wantGuild, "founder_id": string(agentID),
				"timestamp": eventTime.Format(time.RFC3339), "trace": wantTrace("created"),
			},
		},
		{
			name: "joined",
			payload: GuildJoinedPayload{
				GuildID: guildID, GuildName: guild.Name, AgentID: agentID,
				Rank: domain.GuildRankVeteran, Timestamp: eventTime, Trace: trace("joined"),
			},
			want: map[string]any{
				"guild_id": string(guildID), "guild_name": guild.Name, "agent_id": string(agentID),
				"rank": string(domain.GuildRankVeteran), "timestamp": eventTime.Format(time.RFC3339),
				"trace": wantTrace("joined"),
			},
		},
		{
			name: "left",
			payload: GuildLeftPayload{
				GuildID: guildID, GuildName: guild.Name, AgentID: agentID,
				Reason: "retired", Timestamp: eventTime, Trace: trace("left"),
			},
			want: map[string]any{
				"guild_id": string(guildID), "guild_name": guild.Name, "agent_id": string(agentID),
				"reason": "retired", "timestamp": eventTime.Format(time.RFC3339),
				"trace": wantTrace("left"),
			},
		},
		{
			name: "promoted",
			payload: GuildPromotedPayload{
				GuildID: guildID, GuildName: guild.Name, AgentID: agentID,
				OldRank: domain.GuildRankInitiate, NewRank: domain.GuildRankVeteran,
				Timestamp: eventTime, Trace: trace("promoted"),
			},
			want: map[string]any{
				"guild_id": string(guildID), "guild_name": guild.Name, "agent_id": string(agentID),
				"old_rank": string(domain.GuildRankInitiate), "new_rank": string(domain.GuildRankVeteran),
				"timestamp": eventTime.Format(time.RFC3339), "trace": wantTrace("promoted"),
			},
		},
		{
			name: "disbanded",
			payload: GuildDisbandedPayload{
				GuildID: guildID, GuildName: guild.Name, Reason: "mission complete",
				FinalMemberCount: 4, Timestamp: eventTime, Trace: trace("disbanded"),
			},
			want: map[string]any{
				"guild_id": string(guildID), "guild_name": guild.Name, "reason": "mission complete",
				"final_member_count": 4, "timestamp": eventTime.Format(time.RFC3339),
				"trace": wantTrace("disbanded"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertExactGuildEventJSON(t, tt.payload, tt.want)
		})
	}
}

func assertExactGuildEventJSON(t *testing.T, payload any, want map[string]any) {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal guild event payload: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode guild event JSON object: %v", err)
	}
	if !reflect.DeepEqual(got, normalizeGuildJSON(t, want)) {
		t.Errorf("guild event JSON = %#v, want exact wire object %#v", got, want)
	}
}

func normalizeGuildJSON(t *testing.T, value any) any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal expected guild JSON: %v", err)
	}
	var normalized any
	if err := json.Unmarshal(data, &normalized); err != nil {
		t.Fatalf("normalize expected guild JSON: %v", err)
	}
	return normalized
}

// =============================================================================
// isMember and getMember helpers (internal, tested via white-box access)
// =============================================================================

func TestIsMember(t *testing.T) {
	guild := &domain.Guild{
		Members: []domain.GuildMember{
			{AgentID: "agent-1", Rank: domain.GuildRankMaster},
			{AgentID: "agent-2", Rank: domain.GuildRankInitiate},
		},
	}

	if !isMember(guild, "agent-1") {
		t.Error("isMember: expected true for agent-1, got false")
	}
	if !isMember(guild, "agent-2") {
		t.Error("isMember: expected true for agent-2, got false")
	}
	if isMember(guild, "agent-3") {
		t.Error("isMember: expected false for agent-3, got true")
	}
}

func TestIsMember_EmptyMembers(t *testing.T) {
	guild := &domain.Guild{Members: nil}
	if isMember(guild, "agent-1") {
		t.Error("isMember: expected false for empty members slice, got true")
	}
}

func TestGetMember_ReturnsPointerToSliceElement(t *testing.T) {
	guild := &domain.Guild{
		Members: []domain.GuildMember{
			{AgentID: "agent-1", Rank: domain.GuildRankInitiate},
		},
	}

	m := getMember(guild, "agent-1")
	if m == nil {
		t.Fatal("getMember: expected non-nil pointer, got nil")
	}
	// Mutating through the pointer must affect the original slice.
	m.Rank = domain.GuildRankVeteran
	if guild.Members[0].Rank != domain.GuildRankVeteran {
		t.Error("getMember: pointer mutation did not propagate to slice")
	}
}

func TestGetMember_NilForAbsentAgent(t *testing.T) {
	guild := &domain.Guild{
		Members: []domain.GuildMember{
			{AgentID: "agent-1"},
		},
	}
	m := getMember(guild, "agent-99")
	if m != nil {
		t.Errorf("getMember: expected nil for absent agent, got %+v", m)
	}
}
