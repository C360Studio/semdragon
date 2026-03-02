package partycoord

import (
	"strings"
	"testing"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/message"
)

// =============================================================================
// CONFIG TESTS
// =============================================================================

func TestDefaultConfig(t *testing.T) {
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
	if cfg.DefaultMaxPartySize != 5 {
		t.Errorf("DefaultMaxPartySize = %d, want 5", cfg.DefaultMaxPartySize)
	}
	if cfg.FormationTimeout != 10*time.Minute {
		t.Errorf("FormationTimeout = %v, want 10m", cfg.FormationTimeout)
	}
	if cfg.RollupTimeout != 5*time.Minute {
		t.Errorf("RollupTimeout = %v, want 5m", cfg.RollupTimeout)
	}
	if !cfg.AutoFormParties {
		t.Error("AutoFormParties should be true by default")
	}
	if cfg.MinMembersForParty != 2 {
		t.Errorf("MinMembersForParty = %d, want 2", cfg.MinMembersForParty)
	}
	if !cfg.RequireLeadApproval {
		t.Error("RequireLeadApproval should be true by default")
	}
}

func TestConfig_ToBoardConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{
			name: "default config converts correctly",
			cfg:  DefaultConfig(),
		},
		{
			name: "custom config converts correctly",
			cfg: Config{
				Org:      "myorg",
				Platform: "staging",
				Board:    "alpha",
			},
		},
		{
			name: "empty strings pass through",
			cfg: Config{
				Org:      "",
				Platform: "",
				Board:    "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bc := tt.cfg.ToBoardConfig()

			if bc == nil {
				t.Fatal("ToBoardConfig returned nil")
			}
			if bc.Org != tt.cfg.Org {
				t.Errorf("BoardConfig.Org = %q, want %q", bc.Org, tt.cfg.Org)
			}
			if bc.Platform != tt.cfg.Platform {
				t.Errorf("BoardConfig.Platform = %q, want %q", bc.Platform, tt.cfg.Platform)
			}
			if bc.Board != tt.cfg.Board {
				t.Errorf("BoardConfig.Board = %q, want %q", bc.Board, tt.cfg.Board)
			}
		})
	}
}

func TestConfig_ToBoardConfig_ReturnsPointer(t *testing.T) {
	cfg := DefaultConfig()
	bc1 := cfg.ToBoardConfig()
	bc2 := cfg.ToBoardConfig()

	// Each call should return an independent pointer.
	if bc1 == bc2 {
		t.Error("ToBoardConfig should return a new pointer on each call")
	}
	bc1.Org = "mutated"
	if bc2.Org == "mutated" {
		t.Error("Mutating one BoardConfig should not affect the other")
	}
}

// =============================================================================
// PARTY.TRIPLES TESTS
// =============================================================================

// tripleForPredicate finds the first triple with the given predicate.
func tripleForPredicate(triples []message.Triple, predicate string) (message.Triple, bool) {
	for _, tr := range triples {
		if tr.Predicate == predicate {
			return tr, true
		}
	}
	return message.Triple{}, false
}

// allTriplesForPredicate collects all triples matching a predicate.
func allTriplesForPredicate(triples []message.Triple, predicate string) []message.Triple {
	var result []message.Triple
	for _, tr := range triples {
		if tr.Predicate == predicate {
			result = append(result, tr)
		}
	}
	return result
}

func TestParty_Triples_CoreFields(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	partyID := domain.PartyID("c360.prod.game.board1.party.p001")
	questID := domain.QuestID("c360.prod.game.board1.quest.q001")
	leadID := domain.AgentID("c360.prod.game.board1.agent.dragon")

	party := &Party{
		ID:       partyID,
		Name:     "Iron Wolves",
		Status:   domain.PartyForming,
		QuestID:  questID,
		Lead:     leadID,
		Members:  []PartyMember{},
		FormedAt: now,
	}

	triples := party.Triples()

	if len(triples) == 0 {
		t.Fatal("Triples() returned empty slice")
	}

	// All triples should have the correct subject.
	for _, tr := range triples {
		if tr.Subject != string(partyID) {
			t.Errorf("Triple subject = %q, want %q", tr.Subject, string(partyID))
		}
		if tr.Source != "partycoord" {
			t.Errorf("Triple source = %q, want %q", tr.Source, "partycoord")
		}
		if tr.Confidence != 1.0 {
			t.Errorf("Triple confidence = %f, want 1.0", tr.Confidence)
		}
	}

	// Verify name triple.
	tr, ok := tripleForPredicate(triples, "party.identity.name")
	if !ok {
		t.Error("Missing party.identity.name triple")
	} else if tr.Object != "Iron Wolves" {
		t.Errorf("party.identity.name = %v, want %q", tr.Object, "Iron Wolves")
	}

	// Verify status triple.
	tr, ok = tripleForPredicate(triples, "party.status.state")
	if !ok {
		t.Error("Missing party.status.state triple")
	} else if tr.Object != string(domain.PartyForming) {
		t.Errorf("party.status.state = %v, want %q", tr.Object, domain.PartyForming)
	}

	// Verify quest relationship triple.
	tr, ok = tripleForPredicate(triples, "party.quest")
	if !ok {
		t.Error("Missing party.quest triple")
	} else if tr.Object != string(questID) {
		t.Errorf("party.quest = %v, want %q", tr.Object, questID)
	}

	// Verify lead triple.
	tr, ok = tripleForPredicate(triples, "party.lead")
	if !ok {
		t.Error("Missing party.lead triple")
	} else if tr.Object != string(leadID) {
		t.Errorf("party.lead = %v, want %q", tr.Object, leadID)
	}

	// Verify membership count triple.
	tr, ok = tripleForPredicate(triples, "party.membership.count")
	if !ok {
		t.Error("Missing party.membership.count triple")
	} else if tr.Object != 0 {
		t.Errorf("party.membership.count = %v, want 0", tr.Object)
	}

	// Verify formed_at triple.
	tr, ok = tripleForPredicate(triples, "party.lifecycle.formed_at")
	if !ok {
		t.Error("Missing party.lifecycle.formed_at triple")
	} else if tr.Object != now.Format(time.RFC3339) {
		t.Errorf("party.lifecycle.formed_at = %v, want %q", tr.Object, now.Format(time.RFC3339))
	}

	// Verify context count triple.
	_, ok = tripleForPredicate(triples, "party.context.count")
	if !ok {
		t.Error("Missing party.context.count triple")
	}

	// Verify results count triple.
	_, ok = tripleForPredicate(triples, "party.results.count")
	if !ok {
		t.Error("Missing party.results.count triple")
	}
}

func TestParty_Triples_StrategyOmittedWhenEmpty(t *testing.T) {
	party := &Party{
		ID:       "c360.prod.game.board1.party.p002",
		Strategy: "",
		FormedAt: time.Now(),
	}

	triples := party.Triples()

	_, ok := tripleForPredicate(triples, "party.strategy")
	if ok {
		t.Error("party.strategy triple should be absent when strategy is empty")
	}
}

func TestParty_Triples_StrategyIncludedWhenSet(t *testing.T) {
	party := &Party{
		ID:       "c360.prod.game.board1.party.p003",
		Strategy: "divide and conquer",
		FormedAt: time.Now(),
	}

	triples := party.Triples()

	tr, ok := tripleForPredicate(triples, "party.strategy")
	if !ok {
		t.Error("party.strategy triple should be present when strategy is set")
	} else if tr.Object != "divide and conquer" {
		t.Errorf("party.strategy = %v, want %q", tr.Object, "divide and conquer")
	}
}

func TestParty_Triples_MemberRelationships(t *testing.T) {
	leadID := domain.AgentID("c360.prod.game.board1.agent.lead")
	memberID := domain.AgentID("c360.prod.game.board1.agent.member1")
	now := time.Now()

	party := &Party{
		ID:      "c360.prod.game.board1.party.p004",
		FormedAt: now,
		Members: []PartyMember{
			{AgentID: leadID, Role: domain.RoleLead, JoinedAt: now},
			{AgentID: memberID, Role: domain.RoleExecutor, JoinedAt: now},
		},
	}

	triples := party.Triples()

	// Membership count should be 2.
	tr, ok := tripleForPredicate(triples, "party.membership.count")
	if !ok {
		t.Fatal("Missing party.membership.count triple")
	}
	if tr.Object != 2 {
		t.Errorf("party.membership.count = %v, want 2", tr.Object)
	}

	// Membership triples for each member.
	memberTriples := allTriplesForPredicate(triples, "party.membership.member")
	if len(memberTriples) != 2 {
		t.Errorf("party.membership.member count = %d, want 2", len(memberTriples))
	}

	// Role triples for each member.
	leadRolePredicate := "party.member." + string(leadID) + ".role"
	tr, ok = tripleForPredicate(triples, leadRolePredicate)
	if !ok {
		t.Errorf("Missing role triple for lead: %q", leadRolePredicate)
	} else if tr.Object != string(domain.RoleLead) {
		t.Errorf("lead role = %v, want %q", tr.Object, domain.RoleLead)
	}

	memberRolePredicate := "party.member." + string(memberID) + ".role"
	tr, ok = tripleForPredicate(triples, memberRolePredicate)
	if !ok {
		t.Errorf("Missing role triple for member: %q", memberRolePredicate)
	} else if tr.Object != string(domain.RoleExecutor) {
		t.Errorf("member role = %v, want %q", tr.Object, domain.RoleExecutor)
	}
}

func TestParty_Triples_SubQuestAssignments(t *testing.T) {
	agentID := domain.AgentID("c360.prod.game.board1.agent.analyst")
	questID := domain.QuestID("c360.prod.game.board1.quest.sub1")

	party := &Party{
		ID:       "c360.prod.game.board1.party.p005",
		FormedAt: time.Now(),
		SubQuestMap: map[domain.QuestID]domain.AgentID{
			questID: agentID,
		},
	}

	triples := party.Triples()

	predicate := "party.assignment." + string(questID)
	tr, ok := tripleForPredicate(triples, predicate)
	if !ok {
		t.Errorf("Missing assignment triple for predicate %q", predicate)
	} else if tr.Object != string(agentID) {
		t.Errorf("assignment triple object = %v, want %q", tr.Object, agentID)
	}
}

func TestParty_Triples_DisbandedAt(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	disbanded := now.Add(1 * time.Hour)

	partyWithDisband := &Party{
		ID:          "c360.prod.game.board1.party.p006",
		FormedAt:    now,
		DisbandedAt: &disbanded,
	}

	partyWithoutDisband := &Party{
		ID:       "c360.prod.game.board1.party.p007",
		FormedAt: now,
	}

	t.Run("disbanded_at triple present when set", func(t *testing.T) {
		triples := partyWithDisband.Triples()
		tr, ok := tripleForPredicate(triples, "party.lifecycle.disbanded_at")
		if !ok {
			t.Error("party.lifecycle.disbanded_at triple should be present when DisbandedAt is set")
		} else if tr.Object != disbanded.Format(time.RFC3339) {
			t.Errorf("disbanded_at = %v, want %q", tr.Object, disbanded.Format(time.RFC3339))
		}
	})

	t.Run("disbanded_at triple absent when not set", func(t *testing.T) {
		triples := partyWithoutDisband.Triples()
		_, ok := tripleForPredicate(triples, "party.lifecycle.disbanded_at")
		if ok {
			t.Error("party.lifecycle.disbanded_at triple should be absent when DisbandedAt is nil")
		}
	})
}

func TestParty_Triples_SubResultsCount(t *testing.T) {
	partyNoResults := &Party{
		ID:         "c360.prod.game.board1.party.p008",
		FormedAt:   time.Now(),
		SubResults: make(map[domain.QuestID]any),
	}
	partyWithResults := &Party{
		ID:       "c360.prod.game.board1.party.p009",
		FormedAt: time.Now(),
		SubResults: map[domain.QuestID]any{
			"sq1": "done",
			"sq2": "done",
		},
	}

	t.Run("zero sub results", func(t *testing.T) {
		triples := partyNoResults.Triples()
		tr, ok := tripleForPredicate(triples, "party.results.count")
		if !ok {
			t.Fatal("Missing party.results.count triple")
		}
		if tr.Object != 0 {
			t.Errorf("party.results.count = %v, want 0", tr.Object)
		}
	})

	t.Run("two sub results", func(t *testing.T) {
		triples := partyWithResults.Triples()
		tr, ok := tripleForPredicate(triples, "party.results.count")
		if !ok {
			t.Fatal("Missing party.results.count triple")
		}
		if tr.Object != 2 {
			t.Errorf("party.results.count = %v, want 2", tr.Object)
		}
	})
}

// =============================================================================
// PAYLOAD VALIDATE TESTS
// =============================================================================

func TestPartyFormedPayload_Validate(t *testing.T) {
	validParty := Party{
		ID:       "c360.prod.game.board1.party.p001",
		FormedAt: time.Now(),
	}

	tests := []struct {
		name    string
		payload PartyFormedPayload
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid payload",
			payload: PartyFormedPayload{Party: validParty, FormedAt: time.Now()},
			wantErr: false,
		},
		{
			name: "missing party ID",
			payload: PartyFormedPayload{
				Party:    Party{ID: ""},
				FormedAt: time.Now(),
			},
			wantErr: true,
			errMsg:  "party_id required",
		},
		{
			name: "zero formed_at",
			payload: PartyFormedPayload{
				Party:    validParty,
				FormedAt: time.Time{},
			},
			wantErr: true,
			errMsg:  "formed_at required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.payload.Validate()
			if tt.wantErr {
				if err == nil {
					t.Error("Validate() expected error, got nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
	}
}

func TestPartyDisbandedPayload_Validate(t *testing.T) {
	tests := []struct {
		name    string
		payload PartyDisbandedPayload
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid payload",
			payload: PartyDisbandedPayload{
				PartyID:     "c360.prod.game.board1.party.p001",
				QuestID:     "c360.prod.game.board1.quest.q001",
				Reason:      "quest completed",
				DisbandedAt: time.Now(),
			},
			wantErr: false,
		},
		{
			name: "missing party_id",
			payload: PartyDisbandedPayload{
				PartyID:     "",
				DisbandedAt: time.Now(),
			},
			wantErr: true,
			errMsg:  "party_id required",
		},
		{
			name: "zero disbanded_at",
			payload: PartyDisbandedPayload{
				PartyID:     "c360.prod.game.board1.party.p001",
				DisbandedAt: time.Time{},
			},
			wantErr: true,
			errMsg:  "disbanded_at required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.payload.Validate()
			if tt.wantErr {
				if err == nil {
					t.Error("Validate() expected error, got nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
	}
}

func TestPartyJoinedPayload_Validate(t *testing.T) {
	tests := []struct {
		name    string
		payload PartyJoinedPayload
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid payload",
			payload: PartyJoinedPayload{
				PartyID:  "c360.prod.game.board1.party.p001",
				AgentID:  "c360.prod.game.board1.agent.a001",
				Role:     domain.RoleExecutor,
				JoinedAt: time.Now(),
			},
			wantErr: false,
		},
		{
			name: "missing party_id",
			payload: PartyJoinedPayload{
				PartyID:  "",
				AgentID:  "c360.prod.game.board1.agent.a001",
				JoinedAt: time.Now(),
			},
			wantErr: true,
			errMsg:  "party_id required",
		},
		{
			name: "missing agent_id",
			payload: PartyJoinedPayload{
				PartyID:  "c360.prod.game.board1.party.p001",
				AgentID:  "",
				JoinedAt: time.Now(),
			},
			wantErr: true,
			errMsg:  "agent_id required",
		},
		{
			name: "zero joined_at",
			payload: PartyJoinedPayload{
				PartyID:  "c360.prod.game.board1.party.p001",
				AgentID:  "c360.prod.game.board1.agent.a001",
				JoinedAt: time.Time{},
			},
			wantErr: true,
			errMsg:  "joined_at required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.payload.Validate()
			if tt.wantErr {
				if err == nil {
					t.Error("Validate() expected error, got nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
	}
}

func TestPartyQuestDecomposedPayload_Validate(t *testing.T) {
	tests := []struct {
		name    string
		payload PartyQuestDecomposedPayload
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid payload",
			payload: PartyQuestDecomposedPayload{
				PartyID:     "c360.prod.game.board1.party.p001",
				LeadID:      "c360.prod.game.board1.agent.lead",
				ParentQuest: "c360.prod.game.board1.quest.q001",
				SubQuests:   []domain.QuestID{"sq1", "sq2"},
				Strategy:    "parallel",
				Timestamp:   time.Now(),
			},
			wantErr: false,
		},
		{
			name: "missing party_id",
			payload: PartyQuestDecomposedPayload{
				PartyID:     "",
				LeadID:      "c360.prod.game.board1.agent.lead",
				ParentQuest: "q001",
				SubQuests:   []domain.QuestID{"sq1"},
				Timestamp:   time.Now(),
			},
			wantErr: true,
			errMsg:  "party_id required",
		},
		{
			name: "missing lead_id",
			payload: PartyQuestDecomposedPayload{
				PartyID:     "c360.prod.game.board1.party.p001",
				LeadID:      "",
				ParentQuest: "q001",
				SubQuests:   []domain.QuestID{"sq1"},
				Timestamp:   time.Now(),
			},
			wantErr: true,
			errMsg:  "lead_id required",
		},
		{
			name: "missing parent_quest",
			payload: PartyQuestDecomposedPayload{
				PartyID:     "c360.prod.game.board1.party.p001",
				LeadID:      "c360.prod.game.board1.agent.lead",
				ParentQuest: "",
				SubQuests:   []domain.QuestID{"sq1"},
				Timestamp:   time.Now(),
			},
			wantErr: true,
			errMsg:  "parent_quest required",
		},
		{
			name: "empty sub_quests",
			payload: PartyQuestDecomposedPayload{
				PartyID:     "c360.prod.game.board1.party.p001",
				LeadID:      "c360.prod.game.board1.agent.lead",
				ParentQuest: "q001",
				SubQuests:   []domain.QuestID{},
				Timestamp:   time.Now(),
			},
			wantErr: true,
			errMsg:  "sub_quests required",
		},
		{
			name: "nil sub_quests",
			payload: PartyQuestDecomposedPayload{
				PartyID:     "c360.prod.game.board1.party.p001",
				LeadID:      "c360.prod.game.board1.agent.lead",
				ParentQuest: "q001",
				SubQuests:   nil,
				Timestamp:   time.Now(),
			},
			wantErr: true,
			errMsg:  "sub_quests required",
		},
		{
			name: "zero timestamp",
			payload: PartyQuestDecomposedPayload{
				PartyID:     "c360.prod.game.board1.party.p001",
				LeadID:      "c360.prod.game.board1.agent.lead",
				ParentQuest: "q001",
				SubQuests:   []domain.QuestID{"sq1"},
				Timestamp:   time.Time{},
			},
			wantErr: true,
			errMsg:  "timestamp required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.payload.Validate()
			if tt.wantErr {
				if err == nil {
					t.Error("Validate() expected error, got nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
	}
}

func TestPartyTaskAssignedPayload_Validate(t *testing.T) {
	tests := []struct {
		name    string
		payload PartyTaskAssignedPayload
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid payload",
			payload: PartyTaskAssignedPayload{
				PartyID:    "c360.prod.game.board1.party.p001",
				LeadID:     "c360.prod.game.board1.agent.lead",
				AssignedTo: "c360.prod.game.board1.agent.member",
				SubQuestID: "c360.prod.game.board1.quest.sub1",
				Rationale:  "best analyst",
				Timestamp:  time.Now(),
			},
			wantErr: false,
		},
		{
			name: "missing party_id",
			payload: PartyTaskAssignedPayload{
				PartyID:    "",
				LeadID:     "lead",
				AssignedTo: "member",
				SubQuestID: "sub1",
				Timestamp:  time.Now(),
			},
			wantErr: true,
			errMsg:  "party_id required",
		},
		{
			name: "missing lead_id",
			payload: PartyTaskAssignedPayload{
				PartyID:    "p001",
				LeadID:     "",
				AssignedTo: "member",
				SubQuestID: "sub1",
				Timestamp:  time.Now(),
			},
			wantErr: true,
			errMsg:  "lead_id required",
		},
		{
			name: "missing assigned_to",
			payload: PartyTaskAssignedPayload{
				PartyID:    "p001",
				LeadID:     "lead",
				AssignedTo: "",
				SubQuestID: "sub1",
				Timestamp:  time.Now(),
			},
			wantErr: true,
			errMsg:  "assigned_to required",
		},
		{
			name: "missing sub_quest_id",
			payload: PartyTaskAssignedPayload{
				PartyID:    "p001",
				LeadID:     "lead",
				AssignedTo: "member",
				SubQuestID: "",
				Timestamp:  time.Now(),
			},
			wantErr: true,
			errMsg:  "sub_quest_id required",
		},
		{
			name: "zero timestamp",
			payload: PartyTaskAssignedPayload{
				PartyID:    "p001",
				LeadID:     "lead",
				AssignedTo: "member",
				SubQuestID: "sub1",
				Timestamp:  time.Time{},
			},
			wantErr: true,
			errMsg:  "timestamp required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.payload.Validate()
			if tt.wantErr {
				if err == nil {
					t.Error("Validate() expected error, got nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
	}
}

func TestPartyResultSubmittedPayload_Validate(t *testing.T) {
	tests := []struct {
		name    string
		payload PartyResultSubmittedPayload
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid payload",
			payload: PartyResultSubmittedPayload{
				PartyID:    "c360.prod.game.board1.party.p001",
				MemberID:   "c360.prod.game.board1.agent.member",
				SubQuestID: "c360.prod.game.board1.quest.sub1",
				Result:     "analysis done",
				Timestamp:  time.Now(),
			},
			wantErr: false,
		},
		{
			name: "missing party_id",
			payload: PartyResultSubmittedPayload{
				PartyID:    "",
				MemberID:   "member",
				SubQuestID: "sub1",
				Timestamp:  time.Now(),
			},
			wantErr: true,
			errMsg:  "party_id required",
		},
		{
			name: "missing member_id",
			payload: PartyResultSubmittedPayload{
				PartyID:    "p001",
				MemberID:   "",
				SubQuestID: "sub1",
				Timestamp:  time.Now(),
			},
			wantErr: true,
			errMsg:  "member_id required",
		},
		{
			name: "missing sub_quest_id",
			payload: PartyResultSubmittedPayload{
				PartyID:    "p001",
				MemberID:   "member",
				SubQuestID: "",
				Timestamp:  time.Now(),
			},
			wantErr: true,
			errMsg:  "sub_quest_id required",
		},
		{
			name: "zero timestamp",
			payload: PartyResultSubmittedPayload{
				PartyID:    "p001",
				MemberID:   "member",
				SubQuestID: "sub1",
				Timestamp:  time.Time{},
			},
			wantErr: true,
			errMsg:  "timestamp required",
		},
		{
			name: "nil result is valid",
			payload: PartyResultSubmittedPayload{
				PartyID:    "p001",
				MemberID:   "member",
				SubQuestID: "sub1",
				Result:     nil,
				Timestamp:  time.Now(),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.payload.Validate()
			if tt.wantErr {
				if err == nil {
					t.Error("Validate() expected error, got nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
	}
}

func TestPartyRollupStartedPayload_Validate(t *testing.T) {
	tests := []struct {
		name    string
		payload PartyRollupStartedPayload
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid payload",
			payload: PartyRollupStartedPayload{
				PartyID:         "c360.prod.game.board1.party.p001",
				LeadID:          "c360.prod.game.board1.agent.lead",
				ParentQuestID:   "c360.prod.game.board1.quest.q001",
				SubResultsCount: 3,
				Timestamp:       time.Now(),
			},
			wantErr: false,
		},
		{
			name: "zero sub_results_count is valid",
			payload: PartyRollupStartedPayload{
				PartyID:         "p001",
				LeadID:          "lead",
				ParentQuestID:   "q001",
				SubResultsCount: 0,
				Timestamp:       time.Now(),
			},
			wantErr: false,
		},
		{
			name: "missing party_id",
			payload: PartyRollupStartedPayload{
				PartyID:       "",
				LeadID:        "lead",
				ParentQuestID: "q001",
				Timestamp:     time.Now(),
			},
			wantErr: true,
			errMsg:  "party_id required",
		},
		{
			name: "missing lead_id",
			payload: PartyRollupStartedPayload{
				PartyID:       "p001",
				LeadID:        "",
				ParentQuestID: "q001",
				Timestamp:     time.Now(),
			},
			wantErr: true,
			errMsg:  "lead_id required",
		},
		{
			name: "missing parent_quest_id",
			payload: PartyRollupStartedPayload{
				PartyID:       "p001",
				LeadID:        "lead",
				ParentQuestID: "",
				Timestamp:     time.Now(),
			},
			wantErr: true,
			errMsg:  "parent_quest_id required",
		},
		{
			name: "zero timestamp",
			payload: PartyRollupStartedPayload{
				PartyID:       "p001",
				LeadID:        "lead",
				ParentQuestID: "q001",
				Timestamp:     time.Time{},
			},
			wantErr: true,
			errMsg:  "timestamp required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.payload.Validate()
			if tt.wantErr {
				if err == nil {
					t.Error("Validate() expected error, got nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
	}
}

func TestPartyRollupCompletedPayload_Validate(t *testing.T) {
	tests := []struct {
		name    string
		payload PartyRollupCompletedPayload
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid payload with result",
			payload: PartyRollupCompletedPayload{
				PartyID:       "c360.prod.game.board1.party.p001",
				LeadID:        "c360.prod.game.board1.agent.lead",
				ParentQuestID: "c360.prod.game.board1.quest.q001",
				RollupResult:  "final synthesis",
				Timestamp:     time.Now(),
			},
			wantErr: false,
		},
		{
			name: "nil rollup_result is valid",
			payload: PartyRollupCompletedPayload{
				PartyID:       "p001",
				LeadID:        "lead",
				ParentQuestID: "q001",
				RollupResult:  nil,
				Timestamp:     time.Now(),
			},
			wantErr: false,
		},
		{
			name: "missing party_id",
			payload: PartyRollupCompletedPayload{
				PartyID:       "",
				LeadID:        "lead",
				ParentQuestID: "q001",
				Timestamp:     time.Now(),
			},
			wantErr: true,
			errMsg:  "party_id required",
		},
		{
			name: "missing lead_id",
			payload: PartyRollupCompletedPayload{
				PartyID:       "p001",
				LeadID:        "",
				ParentQuestID: "q001",
				Timestamp:     time.Now(),
			},
			wantErr: true,
			errMsg:  "lead_id required",
		},
		{
			name: "missing parent_quest_id",
			payload: PartyRollupCompletedPayload{
				PartyID:       "p001",
				LeadID:        "lead",
				ParentQuestID: "",
				Timestamp:     time.Now(),
			},
			wantErr: true,
			errMsg:  "parent_quest_id required",
		},
		{
			name: "zero timestamp",
			payload: PartyRollupCompletedPayload{
				PartyID:       "p001",
				LeadID:        "lead",
				ParentQuestID: "q001",
				Timestamp:     time.Time{},
			},
			wantErr: true,
			errMsg:  "timestamp required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.payload.Validate()
			if tt.wantErr {
				if err == nil {
					t.Error("Validate() expected error, got nil")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, want it to contain %q", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
	}
}

// =============================================================================
// PAYLOAD TRIPLES TESTS
// =============================================================================

func TestPartyFormedPayload_Triples(t *testing.T) {
	partyID := domain.PartyID("c360.prod.game.board1.party.p001")
	questID := domain.QuestID("c360.prod.game.board1.quest.q001")
	leadID := domain.AgentID("c360.prod.game.board1.agent.lead")
	now := time.Now()

	party := Party{
		ID:      partyID,
		Name:    "Test Party",
		Status:  domain.PartyForming,
		QuestID: questID,
		Lead:    leadID,
		FormedAt: now,
	}
	payload := &PartyFormedPayload{Party: party, FormedAt: now}

	triples := payload.Triples()
	if len(triples) == 0 {
		t.Fatal("PartyFormedPayload.Triples() returned empty slice")
	}

	// Delegates to party.Triples() — verify at least the identity name is present.
	_, ok := tripleForPredicate(triples, "party.identity.name")
	if !ok {
		t.Error("party.identity.name triple should be present in PartyFormedPayload.Triples()")
	}
}

func TestPartyFormedPayload_EntityID(t *testing.T) {
	partyID := domain.PartyID("c360.prod.game.board1.party.p001")
	payload := &PartyFormedPayload{
		Party:    Party{ID: partyID},
		FormedAt: time.Now(),
	}
	if payload.EntityID() != string(partyID) {
		t.Errorf("EntityID() = %q, want %q", payload.EntityID(), string(partyID))
	}
}

func TestPartyDisbandedPayload_Triples(t *testing.T) {
	partyID := domain.PartyID("c360.prod.game.board1.party.p001")
	now := time.Now().Truncate(time.Second)

	payload := &PartyDisbandedPayload{
		PartyID:     partyID,
		QuestID:     "c360.prod.game.board1.quest.q001",
		Reason:      "quest failed",
		DisbandedAt: now,
	}

	triples := payload.Triples()
	if len(triples) == 0 {
		t.Fatal("PartyDisbandedPayload.Triples() returned empty slice")
	}

	// All triples should have partyID as subject.
	for _, tr := range triples {
		if tr.Subject != string(partyID) {
			t.Errorf("Triple subject = %q, want %q", tr.Subject, string(partyID))
		}
	}

	// Verify status is "disbanded".
	tr, ok := tripleForPredicate(triples, "party.status.state")
	if !ok {
		t.Error("Missing party.status.state triple")
	} else if tr.Object != string(domain.PartyDisbanded) {
		t.Errorf("party.status.state = %v, want %q", tr.Object, domain.PartyDisbanded)
	}

	// Verify disbanded_at.
	tr, ok = tripleForPredicate(triples, "party.lifecycle.disbanded_at")
	if !ok {
		t.Error("Missing party.lifecycle.disbanded_at triple")
	} else if tr.Object != now.Format(time.RFC3339) {
		t.Errorf("disbanded_at = %v, want %q", tr.Object, now.Format(time.RFC3339))
	}

	// Verify reason triple.
	tr, ok = tripleForPredicate(triples, "party.disband.reason")
	if !ok {
		t.Error("Missing party.disband.reason triple")
	} else if tr.Object != "quest failed" {
		t.Errorf("disband reason = %v, want %q", tr.Object, "quest failed")
	}
}

func TestPartyDisbandedPayload_EntityID(t *testing.T) {
	partyID := domain.PartyID("c360.prod.game.board1.party.p001")
	payload := &PartyDisbandedPayload{PartyID: partyID, DisbandedAt: time.Now()}
	if payload.EntityID() != string(partyID) {
		t.Errorf("EntityID() = %q, want %q", payload.EntityID(), string(partyID))
	}
}

func TestPartyJoinedPayload_Triples(t *testing.T) {
	partyID := domain.PartyID("c360.prod.game.board1.party.p001")
	agentID := domain.AgentID("c360.prod.game.board1.agent.a001")
	now := time.Now()

	payload := &PartyJoinedPayload{
		PartyID:  partyID,
		AgentID:  agentID,
		Role:     domain.RoleExecutor,
		JoinedAt: now,
	}

	triples := payload.Triples()
	if len(triples) == 0 {
		t.Fatal("PartyJoinedPayload.Triples() returned empty slice")
	}

	// Verify membership triple with party as subject.
	tr, ok := tripleForPredicate(triples, "party.membership.member")
	if !ok {
		t.Error("Missing party.membership.member triple")
	} else {
		if tr.Subject != string(partyID) {
			t.Errorf("membership subject = %q, want %q", tr.Subject, string(partyID))
		}
		if tr.Object != string(agentID) {
			t.Errorf("membership object = %v, want %q", tr.Object, agentID)
		}
	}

	// Verify role triple.
	rolePredicate := "party.member." + string(agentID) + ".role"
	tr, ok = tripleForPredicate(triples, rolePredicate)
	if !ok {
		t.Errorf("Missing role triple: %q", rolePredicate)
	} else if tr.Object != string(domain.RoleExecutor) {
		t.Errorf("role = %v, want %q", tr.Object, domain.RoleExecutor)
	}

	// Verify agent's party membership back-reference (agent as subject).
	found := false
	for _, tr := range triples {
		if tr.Subject == string(agentID) && tr.Predicate == "agent.membership.party" {
			found = true
			if tr.Object != string(partyID) {
				t.Errorf("agent.membership.party = %v, want %q", tr.Object, partyID)
			}
		}
	}
	if !found {
		t.Error("Missing agent.membership.party triple with agent as subject")
	}
}

func TestPartyJoinedPayload_EntityID(t *testing.T) {
	partyID := domain.PartyID("c360.prod.game.board1.party.p001")
	payload := &PartyJoinedPayload{
		PartyID:  partyID,
		AgentID:  "agent-1",
		JoinedAt: time.Now(),
	}
	if payload.EntityID() != string(partyID) {
		t.Errorf("EntityID() = %q, want %q", payload.EntityID(), string(partyID))
	}
}

func TestPartyQuestDecomposedPayload_Triples(t *testing.T) {
	partyID := domain.PartyID("c360.prod.game.board1.party.p001")
	leadID := domain.AgentID("c360.prod.game.board1.agent.lead")
	parentQuest := domain.QuestID("c360.prod.game.board1.quest.parent")
	sq1 := domain.QuestID("sq1")
	sq2 := domain.QuestID("sq2")
	now := time.Now()

	payload := &PartyQuestDecomposedPayload{
		PartyID:     partyID,
		LeadID:      leadID,
		ParentQuest: parentQuest,
		SubQuests:   []domain.QuestID{sq1, sq2},
		Strategy:    "parallel approach",
		Timestamp:   now,
	}

	triples := payload.Triples()
	if len(triples) == 0 {
		t.Fatal("PartyQuestDecomposedPayload.Triples() returned empty slice")
	}

	// All core triples should have partyID as subject.
	tr, ok := tripleForPredicate(triples, "party.coordination.decomposed_by")
	if !ok {
		t.Error("Missing party.coordination.decomposed_by triple")
	} else if tr.Object != string(leadID) {
		t.Errorf("decomposed_by = %v, want %q", tr.Object, leadID)
	}

	tr, ok = tripleForPredicate(triples, "party.coordination.parent_quest")
	if !ok {
		t.Error("Missing party.coordination.parent_quest triple")
	} else if tr.Object != string(parentQuest) {
		t.Errorf("parent_quest = %v, want %q", tr.Object, parentQuest)
	}

	tr, ok = tripleForPredicate(triples, "party.coordination.sub_quest_count")
	if !ok {
		t.Error("Missing party.coordination.sub_quest_count triple")
	} else if tr.Object != 2 {
		t.Errorf("sub_quest_count = %v, want 2", tr.Object)
	}

	tr, ok = tripleForPredicate(triples, "party.strategy")
	if !ok {
		t.Error("Missing party.strategy triple")
	} else if tr.Object != "parallel approach" {
		t.Errorf("strategy = %v, want %q", tr.Object, "parallel approach")
	}

	// One triple per sub-quest.
	subQuestTriples := allTriplesForPredicate(triples, "party.coordination.sub_quest")
	if len(subQuestTriples) != 2 {
		t.Errorf("sub_quest triples count = %d, want 2", len(subQuestTriples))
	}
}

func TestPartyQuestDecomposedPayload_EntityID(t *testing.T) {
	partyID := domain.PartyID("c360.prod.game.board1.party.p001")
	payload := &PartyQuestDecomposedPayload{PartyID: partyID}
	if payload.EntityID() != string(partyID) {
		t.Errorf("EntityID() = %q, want %q", payload.EntityID(), string(partyID))
	}
}

func TestPartyTaskAssignedPayload_Triples(t *testing.T) {
	partyID := domain.PartyID("c360.prod.game.board1.party.p001")
	assignedTo := domain.AgentID("c360.prod.game.board1.agent.analyst")
	subQuestID := domain.QuestID("c360.prod.game.board1.quest.sub1")
	now := time.Now()

	payload := &PartyTaskAssignedPayload{
		PartyID:    partyID,
		LeadID:     "c360.prod.game.board1.agent.lead",
		AssignedTo: assignedTo,
		SubQuestID: subQuestID,
		Rationale:  "best for this task",
		Timestamp:  now,
	}

	triples := payload.Triples()
	if len(triples) == 0 {
		t.Fatal("PartyTaskAssignedPayload.Triples() returned empty slice")
	}

	// Party assignment triple (party as subject).
	assignPredicate := "party.assignment." + string(subQuestID)
	tr, ok := tripleForPredicate(triples, assignPredicate)
	if !ok {
		t.Errorf("Missing assignment triple: %q", assignPredicate)
	} else {
		if tr.Subject != string(partyID) {
			t.Errorf("assignment subject = %q, want %q", tr.Subject, string(partyID))
		}
		if tr.Object != string(assignedTo) {
			t.Errorf("assignment object = %v, want %q", tr.Object, assignedTo)
		}
	}

	// Sub-quest assignment triples (sub-quest as subject).
	agentTripleFound := false
	partyTripleFound := false
	for _, tr := range triples {
		if tr.Subject != string(subQuestID) {
			continue
		}
		switch tr.Predicate {
		case "quest.assignment.agent":
			agentTripleFound = true
			if tr.Object != string(assignedTo) {
				t.Errorf("quest.assignment.agent = %v, want %q", tr.Object, assignedTo)
			}
		case "quest.assignment.party":
			partyTripleFound = true
			if tr.Object != string(partyID) {
				t.Errorf("quest.assignment.party = %v, want %q", tr.Object, partyID)
			}
		}
	}
	if !agentTripleFound {
		t.Error("Missing quest.assignment.agent triple with sub-quest as subject")
	}
	if !partyTripleFound {
		t.Error("Missing quest.assignment.party triple with sub-quest as subject")
	}
}

func TestPartyTaskAssignedPayload_EntityID(t *testing.T) {
	partyID := domain.PartyID("c360.prod.game.board1.party.p001")
	payload := &PartyTaskAssignedPayload{PartyID: partyID}
	if payload.EntityID() != string(partyID) {
		t.Errorf("EntityID() = %q, want %q", payload.EntityID(), string(partyID))
	}
}

func TestPartyResultSubmittedPayload_Triples(t *testing.T) {
	partyID := domain.PartyID("c360.prod.game.board1.party.p001")
	memberID := domain.AgentID("c360.prod.game.board1.agent.member")
	subQuestID := domain.QuestID("c360.prod.game.board1.quest.sub1")
	now := time.Now()

	payload := &PartyResultSubmittedPayload{
		PartyID:      partyID,
		MemberID:     memberID,
		SubQuestID:   subQuestID,
		Result:       "analysis done",
		QualityScore: 0.9,
		Timestamp:    now,
	}

	triples := payload.Triples()
	if len(triples) == 0 {
		t.Fatal("PartyResultSubmittedPayload.Triples() returned empty slice")
	}

	// Submitted_by triple.
	submittedPredicate := "party.result." + string(subQuestID) + ".submitted_by"
	tr, ok := tripleForPredicate(triples, submittedPredicate)
	if !ok {
		t.Errorf("Missing submitted_by triple: %q", submittedPredicate)
	} else {
		if tr.Subject != string(partyID) {
			t.Errorf("submitted_by subject = %q, want %q", tr.Subject, string(partyID))
		}
		if tr.Object != string(memberID) {
			t.Errorf("submitted_by object = %v, want %q", tr.Object, memberID)
		}
	}

	// Quality triple.
	qualityPredicate := "party.result." + string(subQuestID) + ".quality"
	tr, ok = tripleForPredicate(triples, qualityPredicate)
	if !ok {
		t.Errorf("Missing quality triple: %q", qualityPredicate)
	} else if tr.Object != 0.9 {
		t.Errorf("quality = %v, want 0.9", tr.Object)
	}
}

func TestPartyResultSubmittedPayload_EntityID(t *testing.T) {
	partyID := domain.PartyID("c360.prod.game.board1.party.p001")
	payload := &PartyResultSubmittedPayload{PartyID: partyID}
	if payload.EntityID() != string(partyID) {
		t.Errorf("EntityID() = %q, want %q", payload.EntityID(), string(partyID))
	}
}

func TestPartyRollupStartedPayload_Triples(t *testing.T) {
	partyID := domain.PartyID("c360.prod.game.board1.party.p001")
	now := time.Now().Truncate(time.Second)

	payload := &PartyRollupStartedPayload{
		PartyID:         partyID,
		LeadID:          "c360.prod.game.board1.agent.lead",
		ParentQuestID:   "c360.prod.game.board1.quest.q001",
		SubResultsCount: 3,
		Timestamp:       now,
	}

	triples := payload.Triples()
	if len(triples) == 0 {
		t.Fatal("PartyRollupStartedPayload.Triples() returned empty slice")
	}

	// All triples should reference the party.
	for _, tr := range triples {
		if tr.Subject != string(partyID) {
			t.Errorf("Triple subject = %q, want %q", tr.Subject, string(partyID))
		}
	}

	// started_at triple.
	tr, ok := tripleForPredicate(triples, "party.rollup.started_at")
	if !ok {
		t.Error("Missing party.rollup.started_at triple")
	} else if tr.Object != now.Format(time.RFC3339) {
		t.Errorf("rollup started_at = %v, want %q", tr.Object, now.Format(time.RFC3339))
	}

	// sub_results_count triple.
	tr, ok = tripleForPredicate(triples, "party.rollup.sub_results_count")
	if !ok {
		t.Error("Missing party.rollup.sub_results_count triple")
	} else if tr.Object != 3 {
		t.Errorf("sub_results_count = %v, want 3", tr.Object)
	}
}

func TestPartyRollupStartedPayload_EntityID(t *testing.T) {
	partyID := domain.PartyID("c360.prod.game.board1.party.p001")
	payload := &PartyRollupStartedPayload{PartyID: partyID}
	if payload.EntityID() != string(partyID) {
		t.Errorf("EntityID() = %q, want %q", payload.EntityID(), string(partyID))
	}
}

func TestPartyRollupCompletedPayload_Triples(t *testing.T) {
	partyID := domain.PartyID("c360.prod.game.board1.party.p001")
	now := time.Now().Truncate(time.Second)

	payload := &PartyRollupCompletedPayload{
		PartyID:       partyID,
		LeadID:        "c360.prod.game.board1.agent.lead",
		ParentQuestID: "c360.prod.game.board1.quest.q001",
		RollupResult:  "final synthesis",
		Timestamp:     now,
	}

	triples := payload.Triples()
	if len(triples) == 0 {
		t.Fatal("PartyRollupCompletedPayload.Triples() returned empty slice")
	}

	// All triples should reference the party.
	for _, tr := range triples {
		if tr.Subject != string(partyID) {
			t.Errorf("Triple subject = %q, want %q", tr.Subject, string(partyID))
		}
	}

	// completed_at triple.
	tr, ok := tripleForPredicate(triples, "party.rollup.completed_at")
	if !ok {
		t.Error("Missing party.rollup.completed_at triple")
	} else if tr.Object != now.Format(time.RFC3339) {
		t.Errorf("rollup completed_at = %v, want %q", tr.Object, now.Format(time.RFC3339))
	}

	// status triple.
	tr, ok = tripleForPredicate(triples, "party.rollup.status")
	if !ok {
		t.Error("Missing party.rollup.status triple")
	} else if tr.Object != "completed" {
		t.Errorf("rollup status = %v, want %q", tr.Object, "completed")
	}
}

func TestPartyRollupCompletedPayload_EntityID(t *testing.T) {
	partyID := domain.PartyID("c360.prod.game.board1.party.p001")
	payload := &PartyRollupCompletedPayload{PartyID: partyID}
	if payload.EntityID() != string(partyID) {
		t.Errorf("EntityID() = %q, want %q", payload.EntityID(), string(partyID))
	}
}

// =============================================================================
// PAYLOAD SCHEMA TESTS
// =============================================================================

func TestPayload_Schema(t *testing.T) {
	t.Run("PartyFormedPayload schema", func(t *testing.T) {
		p := &PartyFormedPayload{}
		s := p.Schema()
		if s.Domain != "semdragons" {
			t.Errorf("Domain = %q, want %q", s.Domain, "semdragons")
		}
		if s.Category != "party.formed" {
			t.Errorf("Category = %q, want %q", s.Category, "party.formed")
		}
		if s.Version != "v1" {
			t.Errorf("Version = %q, want %q", s.Version, "v1")
		}
	})

	t.Run("PartyDisbandedPayload schema", func(t *testing.T) {
		p := &PartyDisbandedPayload{}
		s := p.Schema()
		if s.Domain != "semdragons" {
			t.Errorf("Domain = %q, want %q", s.Domain, "semdragons")
		}
		if s.Category != "party.disbanded" {
			t.Errorf("Category = %q, want %q", s.Category, "party.disbanded")
		}
		if s.Version != "v1" {
			t.Errorf("Version = %q, want %q", s.Version, "v1")
		}
	})

	t.Run("PartyJoinedPayload schema", func(t *testing.T) {
		p := &PartyJoinedPayload{}
		s := p.Schema()
		if s.Domain != "semdragons" {
			t.Errorf("Domain = %q, want %q", s.Domain, "semdragons")
		}
		if s.Category != "party.joined" {
			t.Errorf("Category = %q, want %q", s.Category, "party.joined")
		}
		if s.Version != "v1" {
			t.Errorf("Version = %q, want %q", s.Version, "v1")
		}
	})

	t.Run("PartyQuestDecomposedPayload schema", func(t *testing.T) {
		p := &PartyQuestDecomposedPayload{}
		s := p.Schema()
		if s.Domain != "semdragons" {
			t.Errorf("Domain = %q, want %q", s.Domain, "semdragons")
		}
		if s.Category != "party.decomposed" {
			t.Errorf("Category = %q, want %q", s.Category, "party.decomposed")
		}
		if s.Version != "v1" {
			t.Errorf("Version = %q, want %q", s.Version, "v1")
		}
	})

	t.Run("PartyTaskAssignedPayload schema", func(t *testing.T) {
		p := &PartyTaskAssignedPayload{}
		s := p.Schema()
		if s.Domain != "semdragons" {
			t.Errorf("Domain = %q, want %q", s.Domain, "semdragons")
		}
		if s.Category != "party.assigned" {
			t.Errorf("Category = %q, want %q", s.Category, "party.assigned")
		}
		if s.Version != "v1" {
			t.Errorf("Version = %q, want %q", s.Version, "v1")
		}
	})

	t.Run("PartyResultSubmittedPayload schema", func(t *testing.T) {
		p := &PartyResultSubmittedPayload{}
		s := p.Schema()
		if s.Domain != "semdragons" {
			t.Errorf("Domain = %q, want %q", s.Domain, "semdragons")
		}
		if s.Category != "party.resultsubmitted" {
			t.Errorf("Category = %q, want %q", s.Category, "party.resultsubmitted")
		}
		if s.Version != "v1" {
			t.Errorf("Version = %q, want %q", s.Version, "v1")
		}
	})

	t.Run("PartyRollupStartedPayload schema", func(t *testing.T) {
		p := &PartyRollupStartedPayload{}
		s := p.Schema()
		if s.Domain != "semdragons" {
			t.Errorf("Domain = %q, want %q", s.Domain, "semdragons")
		}
		if s.Category != "party.rollupstarted" {
			t.Errorf("Category = %q, want %q", s.Category, "party.rollupstarted")
		}
		if s.Version != "v1" {
			t.Errorf("Version = %q, want %q", s.Version, "v1")
		}
	})

	t.Run("PartyRollupCompletedPayload schema", func(t *testing.T) {
		p := &PartyRollupCompletedPayload{}
		s := p.Schema()
		if s.Domain != "semdragons" {
			t.Errorf("Domain = %q, want %q", s.Domain, "semdragons")
		}
		if s.Category != "party.rollupcompleted" {
			t.Errorf("Category = %q, want %q", s.Category, "party.rollupcompleted")
		}
		if s.Version != "v1" {
			t.Errorf("Version = %q, want %q", s.Version, "v1")
		}
	})
}

// =============================================================================
// MAYBEFORMPARTY LOGIC TESTS
// =============================================================================

// TestMaybeFormParty_Logic tests the pure decision logic of maybeFormParty
// without any NATS side effects. It exercises the guard conditions directly
// by constructing Quest values and verifying the outcome via ListActiveParties.
//
// Because maybeFormParty calls FormParty which requires the component to be
// running (and a NATS client for graph.EmitEntity), these tests use a nil
// graph client path only for the guard-condition checks that return early
// before any NATS call.
//
// For the actual formation path we rely on the integration test suite
// (component_test.go). Here we test only the early-return branches.

func TestMaybeFormParty_SkipsNonPartyRequired(t *testing.T) {
	// maybeFormParty returns immediately when PartyRequired is false.
	// We can verify this without a real component by observing that
	// the party list stays empty.
	comp := &Component{}
	comp.running.Store(true)

	curr := &semdragons.Quest{
		ID:            "c360.prod.game.board1.quest.q1",
		Status:        semdragons.QuestClaimed,
		PartyRequired: false, // Should skip
	}

	// This must not panic and must not form any party.
	comp.maybeFormParty(nil, curr)

	// Active parties map is still empty.
	parties := comp.ListActiveParties()
	if len(parties) != 0 {
		t.Errorf("Expected 0 parties when PartyRequired=false, got %d", len(parties))
	}
}

func TestMaybeFormParty_SkipsWhenStatusUnchanged(t *testing.T) {
	comp := &Component{}
	comp.running.Store(true)

	prev := &semdragons.Quest{
		ID:            "c360.prod.game.board1.quest.q2",
		Status:        semdragons.QuestClaimed,
		PartyRequired: true,
	}
	curr := &semdragons.Quest{
		ID:            "c360.prod.game.board1.quest.q2",
		Status:        semdragons.QuestClaimed, // Same status as prev — no transition
		PartyRequired: true,
	}

	comp.maybeFormParty(prev, curr)

	parties := comp.ListActiveParties()
	if len(parties) != 0 {
		t.Errorf("Expected 0 parties when status unchanged, got %d", len(parties))
	}
}

func TestMaybeFormParty_SkipsWhenStatusIsNotClaimed(t *testing.T) {
	comp := &Component{}
	comp.running.Store(true)

	prev := &semdragons.Quest{
		ID:            "c360.prod.game.board1.quest.q3",
		Status:        semdragons.QuestPosted,
		PartyRequired: true,
	}
	curr := &semdragons.Quest{
		ID:            "c360.prod.game.board1.quest.q3",
		Status:        semdragons.QuestInProgress, // Not "claimed"
		PartyRequired: true,
	}

	comp.maybeFormParty(prev, curr)

	parties := comp.ListActiveParties()
	if len(parties) != 0 {
		t.Errorf("Expected 0 parties when status is not claimed, got %d", len(parties))
	}
}

func TestMaybeFormParty_SkipsWhenClaimedByNil(t *testing.T) {
	comp := &Component{}
	comp.running.Store(true)

	prev := &semdragons.Quest{
		ID:            "c360.prod.game.board1.quest.q4",
		Status:        semdragons.QuestPosted,
		PartyRequired: true,
	}
	curr := &semdragons.Quest{
		ID:            "c360.prod.game.board1.quest.q4",
		Status:        semdragons.QuestClaimed,
		PartyRequired: true,
		ClaimedBy:     nil, // No claimer — should skip
	}

	comp.maybeFormParty(prev, curr)

	parties := comp.ListActiveParties()
	if len(parties) != 0 {
		t.Errorf("Expected 0 parties when ClaimedBy is nil, got %d", len(parties))
	}
}

func TestMaybeFormParty_SkipsWhenPartyAlreadyAssigned(t *testing.T) {
	comp := &Component{}
	comp.running.Store(true)

	agent := domain.AgentID("c360.prod.game.board1.agent.a1")
	existingParty := domain.PartyID("c360.prod.game.board1.party.existing")

	prev := &semdragons.Quest{
		ID:            "c360.prod.game.board1.quest.q5",
		Status:        semdragons.QuestPosted,
		PartyRequired: true,
	}
	curr := &semdragons.Quest{
		ID:            "c360.prod.game.board1.quest.q5",
		Status:        semdragons.QuestClaimed,
		PartyRequired: true,
		ClaimedBy:     &agent,
		PartyID:       &existingParty, // Party already assigned — should skip
	}

	comp.maybeFormParty(prev, curr)

	parties := comp.ListActiveParties()
	if len(parties) != 0 {
		t.Errorf("Expected 0 parties when PartyID already set, got %d", len(parties))
	}
}
