package domain

import (
	"reflect"
	"testing"
	"time"

	"github.com/c360studio/semstreams/message"
)

// TestDomainGraphableRelationshipsUseExplicitEntityReferences is the reviewed
// inventory of scalar graph edges emitted by domain Graphable projections.
//
// Collection-valued quest.context.entities remains provenance metadata until
// the task 2.2 cardinality ledger decides whether to flatten it. RequiredTools
// and SharedTools are tool-registry identifiers, not graph entity IDs.
func TestDomainGraphableRelationshipsUseExplicitEntityReferences(t *testing.T) {
	const wantDatatype = "@id" // message.EntityReferenceDatatype in SemStreams beta.158.

	agent1 := AgentID("c360.local.game.board.agent.agent-1")
	agent2 := AgentID("c360.local.game.board.agent.agent-2")
	agent3 := AgentID("c360.local.game.board.agent.agent-3")
	agent4 := AgentID("c360.local.game.board.agent.agent-4")
	party := PartyID("c360.local.game.board.party.party-1")
	guild := GuildID("c360.local.game.board.guild.guild-1")
	quest1 := QuestID("c360.local.game.board.quest.quest-1")
	quest2 := QuestID("c360.local.game.board.quest.quest-2")
	quest3 := QuestID("c360.local.game.board.quest.quest-3")
	quest4 := QuestID("c360.local.game.board.quest.quest-4")
	quest5 := QuestID("c360.local.game.board.quest.quest-5")
	quest6 := QuestID("c360.local.game.board.quest.quest-6")
	produced := "c360.local.knowledge.semsource.document.doc-1"
	produced2 := "c360.local.knowledge.semsource.document.doc-2"
	tool1 := "registry.local.agentic.tools.tool.web-search"
	tool2 := "registry.local.agentic.tools.tool.code-search"
	now := time.Unix(1_700_000_000, 0).UTC()

	tests := []struct {
		name       string
		entity     interface{ Triples() []message.Triple }
		predicates []string
	}{
		{
			name: "quest",
			entity: &Quest{
				Name:                 "quest",
				Title:                "Quest",
				Description:          "Fully populated contract fixture",
				Status:               QuestStatus("claimed"),
				Difficulty:           QuestDifficulty(1),
				QuestType:            QuestType("red-team"),
				ID:                   quest1,
				ClaimedBy:            &agent1,
				PartyID:              &party,
				GuildPriority:        &guild,
				RedTeamTarget:        &quest2,
				RedTeamQuestID:       &quest3,
				RedTeamStatus:        "pending",
				ParentQuest:          &quest4,
				SubQuests:            []QuestID{quest5, quest6},
				DecomposedBy:         &agent2,
				DependsOn:            []QuestID{quest5, quest6},
				Acceptance:           []string{"passes"},
				RequiredSkills:       []SkillTag{SkillResearch},
				MinTier:              TierExpert,
				PartyRequired:        true,
				MinPartySize:         2,
				BaseXP:               100,
				Input:                "input",
				Output:               "output",
				PostedAt:             now,
				ClaimedAt:            &now,
				StartedAt:            &now,
				CompletedAt:          &now,
				Deadline:             &now,
				Attempts:             1,
				MaxAttempts:          3,
				Escalated:            true,
				FailureReason:        "failure",
				FailureType:          FailureError,
				RecoveryPath:         RecoverySalvage,
				FailureAnalysis:      "analysis",
				SalvagedOutput:       "salvaged",
				AntiPatterns:         []string{"none"},
				FailureHistory:       []FailureRecord{{Attempt: 1, Timestamp: now}},
				Verdict:              &BattleVerdict{Passed: true},
				Duration:             time.Second,
				TurnsUsed:            1,
				TokensPrompt:         2,
				TokensCompletion:     3,
				LoopID:               "loop-1",
				DAGExecutionID:       "execution-1",
				DAGDefinition:        map[string]any{"nodes": 2},
				DAGNodeQuestIDs:      map[string]string{"node": string(quest5)},
				DAGNodeStates:        map[string]string{"node": "ready"},
				DAGNodeAssignees:     map[string]string{"node": string(agent2)},
				DAGCompletedNodes:    []string{"node"},
				DAGFailedNodes:       []string{"other"},
				DAGNodeRetries:       map[string]int{"node": 1},
				DAGNodeID:            "node",
				DAGClarifications:    []string{"clarification"},
				DMClarifications:     []string{"answer"},
				ContextTokenCount:    10,
				ContextSources:       []string{"source"},
				Repo:                 "repo",
				ArtifactsMerged:      "commit",
				ArtifactsIndexed:     true,
				ProducedEntities:     []string{produced, produced2},
				Goal:                 "goal",
				Requirements:         []string{"requirement"},
				Scenarios:            []QuestScenario{{Name: "scenario"}},
				DecomposabilityClass: DecomposableParallel,
				// These are intentionally not scalar graph edges.
				RequiredTools:   []string{tool1, tool2},
				ContextEntities: []string{produced, produced2},
			},
			predicates: []string{
				"quest.assignment.agent",
				"quest.assignment.party",
				"quest.priority.guild",
				"quest.classification.red_team_target",
				"quest.classification.red_team_quest_id",
				"quest.parent.quest",
				"quest.dependency.quest",
				PredicateQuestProduced,
			},
		},
		{
			name: "guild",
			entity: &Guild{
				ID:        guild,
				FoundedBy: agent1,
				Members: []GuildMember{
					{AgentID: agent2},
				},
				Applications: []GuildApplication{
					{
						ID:          "application-1",
						ApplicantID: agent3,
						ReviewedBy:  &agent4,
					},
				},
				// Tool registry identifiers are intentionally scalar.
				Name:              "guild",
				Description:       "Fully populated contract fixture",
				Status:            GuildActive,
				MaxMembers:        10,
				MinLevel:          1,
				Founded:           now,
				Culture:           "culture",
				Motto:             "motto",
				Reputation:        1,
				QuestsHandled:     1,
				SuccessRate:       1,
				QuestsFailed:      1,
				QuestTypes:        []string{"research"},
				PreferredClients:  []string{"client"},
				QuorumSize:        1,
				FormationDeadline: &now,
				CreatedAt:         now,
				Lessons:           []Lesson{{ID: "lesson"}},
				SharedTools:       []string{tool1, tool2},
			},
			predicates: []string{
				"guild.founding.agent",
				"guild.membership.agent",
				"guild.application.application-1.applicant",
				"guild.application.application-1.reviewed_by",
			},
		},
		{
			name: "peer review",
			entity: &PeerReview{
				ID:       PeerReviewID("c360.local.game.board.peerreview.review-1"),
				Status:   PeerReviewCompleted,
				QuestID:  quest1,
				PartyID:  &party,
				LeaderID: agent1,
				MemberID: agent2,
				LeaderReview: &ReviewSubmission{
					ReviewerID:  agent1,
					RevieweeID:  agent2,
					Ratings:     ReviewRatings{Q1: 5, Q2: 5, Q3: 5},
					SubmittedAt: now,
				},
				MemberReview: &ReviewSubmission{
					ReviewerID:  agent2,
					RevieweeID:  agent1,
					Ratings:     ReviewRatings{Q1: 5, Q2: 5, Q3: 5},
					SubmittedAt: now,
				},
				LeaderAvgRating: 5,
				MemberAvgRating: 5,
				CreatedAt:       now,
				CompletedAt:     &now,
			},
			predicates: []string{
				"review.assignment.quest",
				"review.assignment.leader",
				"review.assignment.member",
				"review.assignment.party",
			},
		},
	}

	const reviewedRelationshipPredicates = 16
	inventory := make(map[string]struct{}, reviewedRelationshipPredicates)
	for _, tt := range tests {
		for _, predicate := range tt.predicates {
			if _, duplicate := inventory[predicate]; duplicate {
				t.Fatalf("relationship predicate %q appears more than once in the inventory", predicate)
			}
			inventory[predicate] = struct{}{}
		}
	}
	if len(inventory) != reviewedRelationshipPredicates {
		t.Fatalf(
			"relationship inventory contains %d predicates, want %d",
			len(inventory),
			reviewedRelationshipPredicates,
		)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			byPredicate := make(map[string][]message.Triple)
			for _, triple := range tt.entity.Triples() {
				byPredicate[triple.Predicate] = append(byPredicate[triple.Predicate], triple)
			}

			for _, predicate := range tt.predicates {
				triples := byPredicate[predicate]
				if len(triples) == 0 {
					t.Errorf("reviewed relationship %q was not emitted", predicate)
					continue
				}
				for _, triple := range triples {
					if triple.Datatype != wantDatatype {
						t.Errorf(
							"relationship %q datatype = %q, want %q",
							predicate,
							triple.Datatype,
							wantDatatype,
						)
					}
					if _, ok := triple.Object.(string); !ok {
						t.Errorf(
							"relationship %q object type = %T, want string entity ID",
							predicate,
							triple.Object,
						)
					}
				}
			}

			for _, triple := range tt.entity.Triples() {
				object, scalarString := triple.Object.(string)
				if triple.Datatype == wantDatatype {
					if _, reviewed := inventory[triple.Predicate]; !reviewed {
						t.Errorf("typed relationship %q is missing from the reviewed inventory", triple.Predicate)
					}
					if !scalarString || !IsValidEntityID(object) {
						t.Errorf(
							"typed relationship %q object = %#v, want canonical six-part entity ID",
							triple.Predicate,
							triple.Object,
						)
					}
					continue
				}
				if scalarString && IsValidEntityID(object) {
					if _, classified := reviewedScalarNonEdges[triple.Predicate]; !classified {
						t.Errorf(
							"entity-ID-shaped scalar %q on %q lacks @id and a reviewed non-edge classification",
							object,
							triple.Predicate,
						)
					}
				}
			}
		})
	}
}

var reviewedScalarNonEdges = map[string]string{
	"quest.tool.required": "tool-registry identifier, not a graph entity",
	"guild.resource.tool": "tool-registry identifier, not a graph entity",
}

// TestDomainGraphableRegistryAndProvenanceIDsRemainScalar records the reviewed
// non-edge classification alongside the edge inventory above. These values
// must not acquire @id typing without first changing their cardinality and
// reconstruction contracts in the task 2.2 ledger.
func TestDomainGraphableRegistryAndProvenanceIDsRemainScalar(t *testing.T) {
	produced := "c360.local.knowledge.semsource.document.doc-1"
	produced2 := "c360.local.knowledge.semsource.document.doc-2"
	tool1 := "registry.local.agentic.tools.tool.web-search"
	tool2 := "registry.local.agentic.tools.tool.code-search"
	tests := []struct {
		name      string
		entity    interface{ Triples() []message.Triple }
		predicate string
		objects   []any
	}{
		{
			name: "required tool is a tool-registry identifier",
			entity: &Quest{
				ID:            QuestID("c360.local.game.board.quest.quest-1"),
				RequiredTools: []string{tool1, tool2},
			},
			predicate: "quest.tool.required",
			objects:   []any{tool1, tool2},
		},
		{
			name: "context entities are collection-valued provenance metadata",
			entity: &Quest{
				ID:              QuestID("c360.local.game.board.quest.quest-1"),
				ContextEntities: []string{produced, produced2},
			},
			predicate: "quest.context.entities",
			objects:   []any{[]string{produced, produced2}},
		},
		{
			name: "shared tool is a tool-registry identifier",
			entity: &Guild{
				ID:          GuildID("c360.local.game.board.guild.guild-1"),
				SharedTools: []string{tool1, tool2},
			},
			predicate: "guild.resource.tool",
			objects:   []any{tool1, tool2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []any
			for _, triple := range tt.entity.Triples() {
				if triple.Predicate != tt.predicate {
					continue
				}
				got = append(got, triple.Object)
				if triple.Datatype != "" {
					t.Errorf(
						"reviewed non-edge %q datatype = %q, want empty",
						tt.predicate,
						triple.Datatype,
					)
				}
			}
			if !reflect.DeepEqual(got, tt.objects) {
				t.Errorf("reviewed non-edge %q objects = %#v, want %#v", tt.predicate, got, tt.objects)
			}
		})
	}
}
