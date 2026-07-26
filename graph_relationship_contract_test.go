package semdragons_test

import (
	"testing"
	"time"

	"github.com/c360studio/semstreams/message"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/autonomy"
	"github.com/c360studio/semdragons/processor/bossbattle"
	"github.com/c360studio/semdragons/processor/executor"
	"github.com/c360studio/semdragons/processor/partycoord"
)

type graphProjection interface {
	Triples() []message.Triple
}

func TestPersistedAndEventGraphProjectionsTypeEntityReferences(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	agent1 := domain.AgentID("c360.local.game.board.agent.agent-1")
	agent2 := domain.AgentID("c360.local.game.board.agent.agent-2")
	quest1 := domain.QuestID("c360.local.game.board.quest.quest-1")
	quest2 := domain.QuestID("c360.local.game.board.quest.quest-2")
	party := domain.PartyID("c360.local.game.board.party.party-1")
	guild := domain.GuildID("c360.local.game.board.guild.guild-1")
	storeItem := "c360.local.game.board.storeitem.item-1"

	tests := []struct {
		name             string
		entity           graphProjection
		wantReferences   map[string]int
		wantNoReferences bool
	}{
		{
			name: "persisted agent",
			entity: &agentprogression.Agent{
				ID:           agent1,
				Guild:        guild,
				CurrentQuest: &quest1,
				CurrentParty: &party,
				OwnedTools: map[string]agentprogression.OwnedTool{
					"item-1": {StoreItemID: storeItem, PurchasedAt: now},
				},
				ActiveEffects: []agentprogression.AgentEffect{{EffectType: "boost", QuestID: &quest2}},
				CreatedAt:     now,
				UpdatedAt:     now,
			},
			wantReferences: map[string]int{
				"agent.membership.guild":      1,
				"agent.assignment.quest":      1,
				"agent.membership.party":      1,
				"agent.inventory.tool.item-1": 1,
				"agent.effects.boost.quest":   1,
			},
		},
		{
			name: "persisted party",
			entity: &partycoord.Party{
				ID:      party,
				QuestID: quest1,
				Lead:    agent1,
				Members: []partycoord.PartyMember{{AgentID: agent1}, {AgentID: agent2}},
				SubQuestMap: map[domain.QuestID]domain.AgentID{
					quest2: agent2,
				},
				FormedAt: now,
			},
			wantReferences: map[string]int{
				"party.assignment.quest":             1,
				"party.membership.lead":              1,
				"party.membership.member":            2,
				"party.assignment." + string(quest2): 1,
			},
		},
		{
			name: "persisted boss battle",
			entity: &bossbattle.BossBattle{
				ID:        domain.BattleID("c360.local.game.board.battle.battle-1"),
				QuestID:   quest1,
				AgentID:   agent1,
				StartedAt: now,
			},
			wantReferences: map[string]int{
				"battle.assignment.quest": 1,
				"battle.assignment.agent": 1,
			},
		},
		{
			name:           "autonomy claim event",
			entity:         &autonomy.ClaimIntentPayload{AgentID: agent1, QuestID: quest1, Timestamp: now},
			wantReferences: map[string]int{"agent.autonomy.claim-quest": 1},
		},
		{
			name:           "autonomy guild event",
			entity:         &autonomy.GuildIntentPayload{AgentID: agent1, GuildID: string(guild), Timestamp: now},
			wantReferences: map[string]int{"agent.autonomy.guild-join": 1},
		},
		{
			name:           "autonomy use event",
			entity:         &autonomy.UseIntentPayload{AgentID: agent1, QuestID: &quest1, Timestamp: now},
			wantReferences: map[string]int{"agent.autonomy.use-quest": 1},
		},
		{
			name:             "autonomy shop event",
			entity:           &autonomy.ShopIntentPayload{AgentID: agent1, ItemID: "item-1", Timestamp: now},
			wantNoReferences: true,
		},
		{
			name: "autonomy guild-create event",
			entity: &autonomy.GuildCreateIntentPayload{
				AgentID: agent1, CandidateIDs: []domain.AgentID{agent2}, Timestamp: now,
			},
			wantNoReferences: true,
		},
		{
			name: "execution started event",
			entity: &executor.ExecutionStartedPayload{
				QuestID: quest1, AgentID: agent1, Timestamp: now,
			},
			wantReferences: map[string]int{"execution.agent.id": 1},
		},
		// These graph-bound event projections intentionally emit no references.
		{name: "autonomy evaluated event", entity: &autonomy.EvaluatedPayload{AgentID: agent1, Timestamp: now}, wantNoReferences: true},
		{name: "autonomy idle event", entity: &autonomy.IdlePayload{AgentID: agent1, Timestamp: now}, wantNoReferences: true},
		{name: "execution completed event", entity: &executor.ExecutionCompletedPayload{QuestID: quest1, AgentID: agent1, Timestamp: now}, wantNoReferences: true},
		{name: "execution failed event", entity: &executor.ExecutionFailedPayload{QuestID: quest1, AgentID: agent1, Timestamp: now}, wantNoReferences: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			references := 0
			seen := make(map[string]int, len(tt.wantReferences))
			for _, triple := range tt.entity.Triples() {
				object, scalarString := triple.Object.(string)
				if scalarString && domain.IsValidEntityID(object) {
					references++
					seen[triple.Predicate]++
					if triple.Datatype != "@id" {
						t.Errorf("entity reference %q on %q lacks @id", object, triple.Predicate)
					}
				}
				if triple.Datatype == "@id" && (!scalarString || !domain.IsValidEntityID(object)) {
					t.Errorf("typed relationship %q has non-entity object %#v", triple.Predicate, triple.Object)
				}
			}
			if references == 0 && !tt.wantNoReferences {
				t.Errorf("fixture emitted no entity references; add an explicit no-reference classification")
			}
			if references != 0 && tt.wantNoReferences {
				t.Errorf("projection emitted %d entity references, want none", references)
			}
			for predicate, wantCount := range tt.wantReferences {
				if seen[predicate] != wantCount {
					t.Errorf(
						"relationship %q cardinality = %d, want %d",
						predicate,
						seen[predicate],
						wantCount,
					)
				}
			}
			for predicate := range seen {
				if _, reviewed := tt.wantReferences[predicate]; !reviewed {
					t.Errorf("relationship %q is missing from the projection inventory", predicate)
				}
			}
		})
	}
}
