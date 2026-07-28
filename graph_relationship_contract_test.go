package semdragons_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/message"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/agentstore"
	"github.com/c360studio/semdragons/processor/autonomy"
	"github.com/c360studio/semdragons/processor/bossbattle"
	"github.com/c360studio/semdragons/processor/executor"
	"github.com/c360studio/semdragons/processor/partycoord"
	"github.com/c360studio/semdragons/semsource"
)

const (
	relationshipMethod = "relationship"
	noEdgeMethod       = "no-edge"
	externalMethod     = "external-pass-through"

	guildApplicationSentinel = "relationship-contract-application"
	agentInventorySentinel   = "relationship-contract-item"
	agentEffectSentinel      = "relationship-contract-effect"
	partySubQuestSentinel    = "c360.local.game.board.quest.relationship-contract-subquest"

	questMethod            = "domain/graphable.go|Quest|Triples"
	guildMethod            = "domain/graphable.go|Guild|Triples"
	peerReviewMethod       = "domain/graphable.go|PeerReview|Triples"
	semsourceMethod        = "semsource/payload.go|EntityPayload|Triples"
	agentMethod            = "processor/agentprogression/agent.go|Agent|Triples"
	storeItemMethod        = "processor/agentstore/store.go|StoreItem|Triples"
	agentInventoryMethod   = "processor/agentstore/store.go|AgentInventory|Triples"
	partyMethod            = "processor/partycoord/party.go|Party|Triples"
	battleMethod           = "processor/bossbattle/battle.go|BossBattle|Triples"
	evaluatedMethod        = "processor/autonomy/payloads.go|EvaluatedPayload|Triples"
	idleMethod             = "processor/autonomy/payloads.go|IdlePayload|Triples"
	claimIntentMethod      = "processor/autonomy/payloads.go|ClaimIntentPayload|Triples"
	shopIntentMethod       = "processor/autonomy/payloads.go|ShopIntentPayload|Triples"
	guildIntentMethod      = "processor/autonomy/payloads.go|GuildIntentPayload|Triples"
	useIntentMethod        = "processor/autonomy/payloads.go|UseIntentPayload|Triples"
	guildCreateMethod      = "processor/autonomy/payloads.go|GuildCreateIntentPayload|Triples"
	executionStartedMethod = "processor/executor/payloads.go|ExecutionStartedPayload|Triples"
	executionDoneMethod    = "processor/executor/payloads.go|ExecutionCompletedPayload|Triples"
	executionFailedMethod  = "processor/executor/payloads.go|ExecutionFailedPayload|Triples"
	webContentMethod       = "processor/executor/tools.go|webContentEntity|Triples"

	webContentBuilderSite = "processor/executor/tools.go|buildWebContentEntity"
	baseUnitTriplesSite   = "questdag/unit.go|baseUnitTriples"
	referenceTripleSite   = "questdag/unit.go|referenceTriple"
)

var expectedTriplesMethods = map[string]string{
	questMethod:            relationshipMethod,
	guildMethod:            relationshipMethod,
	peerReviewMethod:       relationshipMethod,
	semsourceMethod:        externalMethod,
	agentMethod:            relationshipMethod,
	storeItemMethod:        noEdgeMethod,
	agentInventoryMethod:   noEdgeMethod,
	partyMethod:            relationshipMethod,
	battleMethod:           relationshipMethod,
	evaluatedMethod:        noEdgeMethod,
	idleMethod:             noEdgeMethod,
	claimIntentMethod:      relationshipMethod,
	shopIntentMethod:       noEdgeMethod,
	guildIntentMethod:      relationshipMethod,
	useIntentMethod:        relationshipMethod,
	guildCreateMethod:      noEdgeMethod,
	executionStartedMethod: relationshipMethod,
	executionDoneMethod:    noEdgeMethod,
	executionFailedMethod:  noEdgeMethod,
	webContentMethod:       relationshipMethod,
}

var expectedReferenceSites = map[string]int{
	questMethod:            8,
	guildMethod:            4,
	peerReviewMethod:       4,
	agentMethod:            5,
	partyMethod:            4,
	battleMethod:           2,
	claimIntentMethod:      1,
	guildIntentMethod:      1,
	useIntentMethod:        1,
	executionStartedMethod: 1,
	webContentBuilderSite:  2,
	baseUnitTriplesSite:    2,
	referenceTripleSite:    1,
}

type relationshipShapeSpec struct {
	producer string
	deferred string
}

var expectedRelationshipShapes = map[string]relationshipShapeSpec{
	// Domain: Quest (8), Guild (4), PeerReview (4).
	"quest.assignment.agent":                 {producer: questMethod},
	"quest.assignment.party":                 {producer: questMethod},
	"quest.priority.guild":                   {producer: questMethod},
	"quest.classification.red-team-target":   {producer: questMethod},
	"quest.classification.red-team-quest-id": {producer: questMethod},
	"quest.parent.quest":                     {producer: questMethod},
	"quest.dependency.quest":                 {producer: questMethod},
	"quest.relationship.produced":            {producer: questMethod},
	"guild.founding.agent":                   {producer: guildMethod},
	"guild.membership.agent":                 {producer: guildMethod},
	"guild.application.{id}.applicant":       {producer: guildMethod, deferred: "SemStreams #313"},
	"guild.application.{id}.reviewed_by":     {producer: guildMethod, deferred: "SemStreams #313; preserve reviewed_by"},
	"review.assignment.quest":                {producer: peerReviewMethod},
	"review.assignment.leader":               {producer: peerReviewMethod},
	"review.assignment.member":               {producer: peerReviewMethod},
	"review.assignment.party":                {producer: peerReviewMethod},
	// Agent (5), Party (4), Battle (2).
	"agent.membership.guild":    {producer: agentMethod},
	"agent.assignment.quest":    {producer: agentMethod},
	"agent.membership.party":    {producer: agentMethod},
	"agent.inventory.tool.{id}": {producer: agentMethod, deferred: "Semdragon #4 / SemStreams #313"},
	"agent.effects.{id}.quest":  {producer: agentMethod, deferred: "Semdragon #4 / SemStreams #313"},
	"party.assignment.quest":    {producer: partyMethod},
	"party.membership.lead":     {producer: partyMethod},
	"party.membership.member":   {producer: partyMethod},
	"party.assignment.{id}":     {producer: partyMethod, deferred: "Semdragon #4 / SemStreams #313"},
	"battle.assignment.quest":   {producer: battleMethod},
	"battle.assignment.agent":   {producer: battleMethod},
	// Autonomy (3), executor/web (3), questdag (3).
	"agent.autonomy.claim-quest": {producer: claimIntentMethod},
	"agent.autonomy.guild-join":  {producer: guildIntentMethod},
	"agent.autonomy.use-quest":   {producer: useIntentMethod},
	"execution.agent.id":         {producer: executionStartedMethod},
	"source.doc.fetched-by":      {producer: webContentBuilderSite},
	"source.doc.quest-id":        {producer: webContentBuilderSite},
	"questdag.unit.parent-quest": {producer: baseUnitTriplesSite},
	"questdag.unit.sub-quest":    {producer: baseUnitTriplesSite},
	"gateddag.unit.depends-on":   {producer: referenceTripleSite},
}

type graphProjection interface {
	Triples() []message.Triple
}

type projectionFixture struct {
	name             string
	producer         string
	entity           graphProjection
	expectedShapes   map[string]int
	explicitNonEdges map[string]bool
}

func TestRepositoryRelationshipContract(t *testing.T) {
	t.Parallel()

	t.Run("production AST inventory is exact", func(t *testing.T) {
		t.Parallel()

		sources := loadProductionGoSources(t)
		methods, referenceSites, scanViolations := scanRelationshipAST(sources)
		assertNoContractViolations(t, scanViolations)
		assertExactStringSet(t, "Triples method", methods, expectedTriplesMethods)
		assertExactCounts(t, "entity-reference constructor", referenceSites, expectedReferenceSites)

		classCounts := map[string]int{}
		for _, class := range expectedTriplesMethods {
			classCounts[class]++
		}
		wantClassCounts := map[string]int{
			relationshipMethod: 11,
			noEdgeMethod:       8,
			externalMethod:     1,
		}
		assertExactCounts(t, "Triples classification", classCounts, wantClassCounts)
		if len(expectedRelationshipShapes) != 36 {
			t.Fatalf("relationship shape inventory = %d, want exactly 36", len(expectedRelationshipShapes))
		}
		shapeCounts := map[string]int{}
		for _, spec := range expectedRelationshipShapes {
			shapeCounts[spec.producer]++
		}
		wantShapeCounts := map[string]int{
			questMethod:            8,
			guildMethod:            4,
			peerReviewMethod:       4,
			agentMethod:            5,
			partyMethod:            4,
			battleMethod:           2,
			claimIntentMethod:      1,
			guildIntentMethod:      1,
			useIntentMethod:        1,
			executionStartedMethod: 1,
			webContentBuilderSite:  2,
			baseUnitTriplesSite:    2,
			referenceTripleSite:    1,
		}
		assertExactCounts(t, "relationship shape producer", shapeCounts, wantShapeCounts)
	})

	t.Run("relationship fixtures cover exact root shapes", func(t *testing.T) {
		t.Parallel()

		fixtures := relationshipProjectionFixtures()
		for _, fixture := range fixtures {
			fixture := fixture
			if class := expectedTriplesMethods[fixture.producer]; class != relationshipMethod {
				t.Errorf("%s producer class = %q, want %q", fixture.producer, class, relationshipMethod)
			}
			t.Run(fixture.name, func(t *testing.T) {
				t.Parallel()

				violations, _ := validateProjectionFixture(fixture)
				assertNoContractViolations(t, violations)
			})
		}

		// Parallel subtests cannot safely publish into the parent after return,
		// so independently compare fixture declarations here.
		declared := map[string]int{}
		for _, fixture := range fixtures {
			for predicate, count := range fixture.expectedShapes {
				shape := normalizeFixturePredicate(fixture.producer, predicate)
				declared[shape] += count
			}
		}
		for shape, spec := range expectedRelationshipShapes {
			if spec.producer == webContentBuilderSite ||
				spec.producer == baseUnitTriplesSite ||
				spec.producer == referenceTripleSite {
				continue // Covered in package-local focused tests.
			}
			if declared[shape] == 0 {
				t.Errorf("root behavioral fixture missing shape %q", shape)
			}
		}
		if len(declared) != 31 {
			t.Errorf("root behavioral shapes = %d, want 31 (plus web 2 and questdag 3)", len(declared))
		}
	})

	t.Run("no-edge methods stay edge-free", func(t *testing.T) {
		t.Parallel()

		fixtures := noEdgeProjectionFixtures()
		assertNoContractViolations(t, noEdgeFixtureInventoryViolations(fixtures))
		for _, fixture := range fixtures {
			fixture := fixture
			t.Run(fixture.name, func(t *testing.T) {
				t.Parallel()
				violations, _ := validateProjectionFixture(fixture)
				assertNoContractViolations(t, violations)
			})
		}
	})

	t.Run("semsource is identity-preserving external pass-through", func(t *testing.T) {
		t.Parallel()

		if class := expectedTriplesMethods[semsourceMethod]; class != externalMethod {
			t.Fatalf("%s producer class = %q, want %q", semsourceMethod, class, externalMethod)
		}
		original := []message.Triple{{
			Subject:   "external.prod.sensor.station.observation.obs-1",
			Predicate: "external.sensor.related",
			Object:    "external.prod.sensor.station.device.device-1",
			Datatype:  "@id",
		}}
		payload := &semsource.EntityPayload{
			ID:         "external.prod.sensor.station.observation.obs-1",
			TripleData: original,
		}
		got := payload.Triples()
		if len(got) != 1 || &got[0] != &original[0] {
			t.Errorf("semsource Triples did not preserve the external slice identity")
		}
		if got[0] != original[0] {
			t.Errorf("semsource Triples changed external triple: %#v", got[0])
		}
		// Decoded external relationship validation remains deferred to
		// Semdragon #4; this fixture is deliberately not classified no-edge.
	})
}

func TestRepositoryRelationshipContractRejectsMutations(t *testing.T) {
	t.Parallel()

	t.Run("synthetic new Triples method", func(t *testing.T) {
		t.Parallel()
		source := `package synthetic
import semmsg "github.com/c360studio/semstreams/message"
type Surprise struct{}
func (*Surprise) Triples() []semmsg.Triple { return nil }
`
		methods, _, scanViolations := scanRelationshipAST(map[string][]byte{
			"synthetic/surprise.go": []byte(source),
		})
		violations := append(scanViolations, exactStringSetViolations(methods, map[string]string{})...)
		if len(violations) == 0 {
			t.Fatal("synthetic Triples method escaped the AST inventory")
		}
	})

	t.Run("synthetic direct constructor", func(t *testing.T) {
		t.Parallel()
		source := `package synthetic
import (
  "github.com/c360studio/semstreams/message"
)
func surprise() message.Triple {
  return message.Triple{Datatype: message.EntityReferenceDatatype}
}
`
		_, references, scanViolations := scanRelationshipAST(map[string][]byte{
			"questdag/synthetic_relationship.go": []byte(source),
		})
		violations := append(scanViolations, exactCountViolations(references, map[string]int{})...)
		if len(violations) == 0 {
			t.Fatal("synthetic direct @id constructor escaped the AST inventory")
		}
	})

	t.Run("synthetic direct Datatype assignment", func(t *testing.T) {
		t.Parallel()
		source := `package synthetic
import (
  "github.com/c360studio/semstreams/message"
)
func surprise() message.Triple {
  triple := message.Triple{}
  triple.Datatype = message.EntityReferenceDatatype
  return triple
}
`
		_, references, scanViolations := scanRelationshipAST(map[string][]byte{
			"synthetic/assignment.go": []byte(source),
		})
		assertNoContractViolations(t, scanViolations)
		if references["synthetic/assignment.go|surprise"] != 1 {
			t.Fatalf("direct Datatype assignment references = %v, want one inventoried write", references)
		}
	})

	t.Run("synthetic keyed Datatype indirection", func(t *testing.T) {
		t.Parallel()
		source := `package synthetic
import "github.com/c360studio/semstreams/message"
const entityReference = "@id"
func surprise() message.Triple {
  return message.Triple{Datatype: entityReference}
}
`
		_, _, violations := scanRelationshipAST(map[string][]byte{
			"synthetic/keyed_indirection.go": []byte(source),
		})
		if !containsViolation(violations, "unsupported Datatype expression") {
			t.Fatalf("violations = %v, want unsupported Datatype expression", violations)
		}
	})

	t.Run("synthetic assignment Datatype indirection", func(t *testing.T) {
		t.Parallel()
		source := `package synthetic
import (
  "github.com/c360studio/semstreams/message"
)
func surprise() message.Triple {
  datatype := message.EntityReferenceDatatype
  triple := message.Triple{}
  triple.Datatype = datatype
  return triple
}
`
		_, _, violations := scanRelationshipAST(map[string][]byte{
			"synthetic/assignment_indirection.go": []byte(source),
		})
		if !containsViolation(violations, "unsupported Datatype expression") {
			t.Fatalf("violations = %v, want unsupported Datatype expression", violations)
		}
	})

	t.Run("synthetic Triples result alias", func(t *testing.T) {
		t.Parallel()
		source := `package synthetic
import "github.com/c360studio/semstreams/message"
type TripleSlice = []message.Triple
type Surprise struct{}
func (*Surprise) Triples() TripleSlice { return nil }
`
		_, _, violations := scanRelationshipAST(map[string][]byte{
			"synthetic/alias.go": []byte(source),
		})
		if !containsViolation(violations, "unsupported Triples signature") {
			t.Fatalf("violations = %v, want unsupported Triples signature", violations)
		}
	})

	t.Run("synthetic Triples dot import", func(t *testing.T) {
		t.Parallel()
		source := `package synthetic
import . "github.com/c360studio/semstreams/message"
type Surprise struct{}
func (*Surprise) Triples() []Triple { return nil }
`
		_, _, violations := scanRelationshipAST(map[string][]byte{
			"synthetic/dot_import.go": []byte(source),
		})
		if !containsViolation(violations, "unsupported Triples signature") {
			t.Fatalf("violations = %v, want unsupported Triples signature", violations)
		}
	})

	t.Run("generated production source stays inventoried", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		source := `// Code generated by relationship mutation. DO NOT EDIT.
package synthetic
import "github.com/c360studio/semstreams/message"
type Generated struct{}
func (*Generated) Triples() []message.Triple { return nil }
`
		path := filepath.Join(root, "generated.go")
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Fatalf("write generated mutation: %v", err)
		}
		sources := loadProductionGoSourcesAt(t, root)
		if _, included := sources["generated.go"]; !included {
			t.Fatal("generated production Go escaped source inventory")
		}
		methods, _, violations := scanRelationshipAST(sources)
		assertNoContractViolations(t, violations)
		if _, inventoried := methods["generated.go|Generated|Triples"]; !inventoried {
			t.Fatalf("generated Triples method escaped AST inventory: %v", methods)
		}
	})

	t.Run("no-edge producer inventory rejects duplicates", func(t *testing.T) {
		t.Parallel()
		fixtures := noEdgeProjectionFixtures()
		fixtures[len(fixtures)-1] = fixtures[0]
		violations := noEdgeFixtureInventoryViolations(fixtures)
		if !containsViolation(violations, "no-edge fixture producer") {
			t.Fatalf("violations = %v, want exact no-edge fixture producer failure", violations)
		}
	})

	mutations := []struct {
		name     string
		fixture  projectionFixture
		fragment string
	}{
		{
			name: "unknown relationship shape",
			fixture: mutationFixture(message.Triple{
				Predicate: "agent.relationship.unreviewed",
				Object:    "c360.local.game.board.guild.unreviewed",
				Datatype:  "@id",
			}),
			fragment: "unknown @id relationship",
		},
		{
			name: "missing relationship datatype",
			fixture: mutationFixture(message.Triple{
				Predicate: "agent.membership.guild",
				Object:    "c360.local.game.board.guild.untyped",
			}),
			fragment: "missing @id",
		},
		{
			name: "invalid typed object",
			fixture: mutationFixture(message.Triple{
				Predicate: "agent.membership.guild",
				Object:    42,
				Datatype:  "@id",
			}),
			fragment: "invalid entity object",
		},
		{
			name: "unknown dynamic predicate",
			fixture: mutationFixture(message.Triple{
				Predicate: "agent.inventory.tool.not-the-sentinel",
				Object:    "c360.local.game.board.storeitem.unreviewed",
				Datatype:  "@id",
			}),
			fragment: "unknown @id relationship",
		},
		{
			name: "defined string entity object",
			fixture: mutationFixture(message.Triple{
				Predicate: "agent.relationship.typed-scalar",
				Object:    domain.AgentID("c360.local.game.board.agent.typed-scalar"),
			}),
			fragment: "unclassified entity-looking scalar",
		},
	}
	for _, tt := range mutations {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			violations, _ := validateProjectionFixture(tt.fixture)
			if !containsViolation(violations, tt.fragment) {
				t.Fatalf("violations = %v, want fragment %q", violations, tt.fragment)
			}
		})
	}
}

type staticProjection []message.Triple

func (projection staticProjection) Triples() []message.Triple {
	return projection
}

func mutationFixture(triple message.Triple) projectionFixture {
	return projectionFixture{
		name:     "mutation",
		producer: agentMethod,
		entity:   staticProjection{triple},
		expectedShapes: map[string]int{
			"agent.membership.guild": 1,
		},
	}
}

func relationshipProjectionFixtures() []projectionFixture {
	now := time.Unix(1_700_000_000, 0).UTC()
	agent1 := domain.AgentID("c360.local.game.board.agent.agent-1")
	agent2 := domain.AgentID("c360.local.game.board.agent.agent-2")
	agent3 := domain.AgentID("c360.local.game.board.agent.agent-3")
	quest1 := domain.QuestID("c360.local.game.board.quest.quest-1")
	quest2 := domain.QuestID(partySubQuestSentinel)
	quest3 := domain.QuestID("c360.local.game.board.quest.quest-3")
	quest4 := domain.QuestID("c360.local.game.board.quest.quest-4")
	partyID := domain.PartyID("c360.local.game.board.party.party-1")
	guildID := domain.GuildID("c360.local.game.board.guild.guild-1")
	storeItemID := "c360.local.game.board.storeitem.item-1"
	producedID := "c360.local.web.agent.doc.document-1"

	reviewedBy := agent2
	return []projectionFixture{
		{
			name:     "Quest",
			producer: questMethod,
			entity: &domain.Quest{
				ID:               quest1,
				ClaimedBy:        &agent1,
				PartyID:          &partyID,
				GuildPriority:    &guildID,
				RedTeamTarget:    &quest2,
				RedTeamQuestID:   &quest3,
				ParentQuest:      &quest4,
				DependsOn:        []domain.QuestID{quest2},
				ProducedEntities: []string{producedID},
				PostedAt:         now,
			},
			expectedShapes: map[string]int{
				"quest.assignment.agent":                 1,
				"quest.assignment.party":                 1,
				"quest.priority.guild":                   1,
				"quest.classification.red-team-target":   1,
				"quest.classification.red-team-quest-id": 1,
				"quest.parent.quest":                     1,
				"quest.dependency.quest":                 1,
				"quest.relationship.produced":            1,
			},
		},
		{
			name:     "Guild",
			producer: guildMethod,
			entity: &domain.Guild{
				ID:        guildID,
				FoundedBy: agent1,
				Members:   []domain.GuildMember{{AgentID: agent2}},
				Applications: []domain.GuildApplication{{
					ID: guildApplicationSentinel, ApplicantID: agent3, ReviewedBy: &reviewedBy,
				}},
				Founded:   now,
				CreatedAt: now,
			},
			expectedShapes: map[string]int{
				"guild.founding.agent":   1,
				"guild.membership.agent": 1,
				"guild.application." + guildApplicationSentinel + ".applicant":   1,
				"guild.application." + guildApplicationSentinel + ".reviewed_by": 1,
			},
		},
		{
			name:     "PeerReview",
			producer: peerReviewMethod,
			entity: &domain.PeerReview{
				ID:        "c360.local.game.board.review.review-1",
				QuestID:   quest1,
				PartyID:   &partyID,
				LeaderID:  agent1,
				MemberID:  agent2,
				CreatedAt: now,
			},
			expectedShapes: map[string]int{
				"review.assignment.quest":  1,
				"review.assignment.leader": 1,
				"review.assignment.member": 1,
				"review.assignment.party":  1,
			},
		},
		{
			name:     "Agent",
			producer: agentMethod,
			entity: &agentprogression.Agent{
				ID:           agent1,
				Guild:        guildID,
				CurrentQuest: &quest1,
				CurrentParty: &partyID,
				SkillProficiencies: map[domain.SkillTag]domain.SkillProficiency{
					"coding": {},
				},
				OwnedTools: map[string]agentprogression.OwnedTool{
					agentInventorySentinel: {StoreItemID: storeItemID, PurchasedAt: now},
				},
				ActiveEffects: []agentprogression.AgentEffect{{
					EffectType: agentEffectSentinel, QuestID: &quest2,
				}},
				CreatedAt: now,
				UpdatedAt: now,
			},
			expectedShapes: map[string]int{
				"agent.membership.guild":                          1,
				"agent.assignment.quest":                          1,
				"agent.membership.party":                          1,
				"agent.inventory.tool." + agentInventorySentinel:  1,
				"agent.effects." + agentEffectSentinel + ".quest": 1,
			},
			explicitNonEdges: map[string]bool{
				"agent.skill.proficiency": true,
			},
		},
		{
			name: "persisted party", producer: partyMethod,
			entity: &partycoord.Party{
				ID:      partyID,
				QuestID: quest1,
				Lead:    agent1,
				Members: []partycoord.PartyMember{{AgentID: agent2}},
				SubQuestMap: map[domain.QuestID]domain.AgentID{
					domain.QuestID(partySubQuestSentinel): agent3,
				},
				FormedAt: now,
			},
			expectedShapes: map[string]int{
				"party.assignment.quest":             1,
				"party.membership.lead":              1,
				"party.membership.member":            1,
				"party.assignment." + string(quest2): 1,
			},
		},
		{
			name:     "BossBattle",
			producer: battleMethod,
			entity: &bossbattle.BossBattle{
				ID:        "c360.local.game.board.battle.battle-1",
				QuestID:   quest1,
				AgentID:   agent1,
				StartedAt: now,
				Criteria: []domain.ReviewCriterion{{
					Name: string(quest2), Description: string(quest3), Weight: 1, Threshold: 0.5,
				}},
				Judges: []domain.Judge{{ID: string(agent2), Type: domain.JudgeAutomated}},
				Results: []domain.ReviewResult{{
					CriterionName: string(quest2), JudgeID: string(agent2), Score: 1, Passed: true,
					Reasoning: string(quest3),
				}},
			},
			expectedShapes: map[string]int{
				"battle.assignment.quest": 1,
				"battle.assignment.agent": 1,
			},
			explicitNonEdges: battleCollectionNonEdges(),
		},
		{
			name:     "ClaimIntentPayload",
			producer: claimIntentMethod,
			entity:   &autonomy.ClaimIntentPayload{AgentID: agent1, QuestID: quest1, Timestamp: now},
			expectedShapes: map[string]int{
				"agent.autonomy.claim-quest": 1,
			},
		},
		{
			name:     "GuildIntentPayload",
			producer: guildIntentMethod,
			entity:   &autonomy.GuildIntentPayload{AgentID: agent1, GuildID: string(guildID), Timestamp: now},
			expectedShapes: map[string]int{
				"agent.autonomy.guild-join": 1,
			},
		},
		{
			name:     "UseIntentPayload",
			producer: useIntentMethod,
			entity:   &autonomy.UseIntentPayload{AgentID: agent1, QuestID: &quest1, Timestamp: now},
			expectedShapes: map[string]int{
				"agent.autonomy.use-quest": 1,
			},
		},
		{
			name:     "ExecutionStartedPayload",
			producer: executionStartedMethod,
			entity:   &executor.ExecutionStartedPayload{QuestID: quest1, AgentID: agent1, Timestamp: now},
			expectedShapes: map[string]int{
				"execution.agent.id": 1,
			},
		},
	}
}

func noEdgeProjectionFixtures() []projectionFixture {
	now := time.Unix(1_700_000_000, 0).UTC()
	agentID := domain.AgentID("c360.local.game.board.agent.agent-no-edge")
	questID := domain.QuestID("c360.local.game.board.quest.quest-no-edge")
	storeItemID := "c360.local.game.board.storeitem.item-no-edge"
	return []projectionFixture{
		{
			name:     "StoreItem alias",
			producer: storeItemMethod,
			entity: &agentstore.StoreItem{
				ID: "item-no-edge", ToolID: storeItemID,
				BoardConfig: &domain.BoardConfig{Org: "c360", Platform: "local", Board: "board"},
			},
			explicitNonEdges: map[string]bool{"store.item.tool-id": true},
		},
		{
			name:     "AgentInventory alias",
			producer: agentInventoryMethod,
			entity: &agentstore.AgentInventory{
				AgentID: agentID,
				OwnedTools: map[string]agentstore.OwnedItem{
					storeItemID: {ItemID: storeItemID, PurchasedAt: now},
				},
			},
			explicitNonEdges: map[string]bool{"agent.inventory.tool": true},
		},
		{name: "EvaluatedPayload", producer: evaluatedMethod, entity: &autonomy.EvaluatedPayload{AgentID: agentID, Timestamp: now}},
		{name: "IdlePayload", producer: idleMethod, entity: &autonomy.IdlePayload{AgentID: agentID, Timestamp: now}},
		{
			name:     "ShopIntentPayload",
			producer: shopIntentMethod,
			entity:   &autonomy.ShopIntentPayload{AgentID: agentID, ItemID: storeItemID, Timestamp: now},
			explicitNonEdges: map[string]bool{
				"agent.autonomy.shop-item": true,
			},
		},
		{
			name: guildCreateMethod, producer: guildCreateMethod,
			entity: &autonomy.GuildCreateIntentPayload{
				AgentID: agentID, CandidateIDs: []domain.AgentID{"c360.local.game.board.agent.candidate"}, Timestamp: now,
			},
		},
		{
			name: executionDoneMethod, producer: executionDoneMethod,
			entity: &executor.ExecutionCompletedPayload{QuestID: questID, AgentID: agentID, Timestamp: now},
		},
		{
			name: executionFailedMethod, producer: executionFailedMethod,
			entity: &executor.ExecutionFailedPayload{QuestID: questID, AgentID: agentID, Timestamp: now},
		},
	}
}

func battleCollectionNonEdges() map[string]bool {
	// Scanner-hidden indexed criteria/judge/result collections remain deferred
	// to SemStreams #683 and are explicitly scalar, not graph relationships.
	return map[string]bool{
		"battle.criteria.count":          true,
		"battle.criteria.0.name":         true,
		"battle.criteria.0.description":  true,
		"battle.criteria.0.weight":       true,
		"battle.criteria.0.threshold":    true,
		"battle.judge.0.id":              true,
		"battle.judge.0.type":            true,
		"battle.results.count":           true,
		"battle.results.passed":          true,
		"battle.result.0.criterion_name": true,
		"battle.result.0.judge_id":       true,
		"battle.result.0.score":          true,
		"battle.result.0.passed":         true,
		"battle.result.0.reasoning":      true,
	}
}

func validateProjectionFixture(fixture projectionFixture) ([]string, map[string]int) {
	var violations []string
	expectedShapes := make(map[string]int, len(fixture.expectedShapes))
	for predicate, count := range fixture.expectedShapes {
		shape := normalizeFixturePredicate(fixture.producer, predicate)
		expectedShapes[shape] += count
	}
	seen := make(map[string]int, len(expectedShapes))
	for _, triple := range fixture.entity.Triples() {
		shape := normalizeFixturePredicate(fixture.producer, triple.Predicate)
		_, expectedRelationship := expectedShapes[shape]
		object, stringObject := stringObjectValue(triple.Object)
		validEntityObject := stringObject && domain.IsValidEntityID(object)
		explicitNonEdge := fixture.explicitNonEdges[triple.Predicate]

		if triple.Datatype == "@id" {
			if !validEntityObject {
				violations = append(violations,
					fmt.Sprintf("%s %q has @id with invalid entity object %#v", fixture.name, triple.Predicate, triple.Object))
			}
			if !expectedRelationship {
				violations = append(violations,
					fmt.Sprintf("%s unknown @id relationship %q (shape %q)", fixture.name, triple.Predicate, shape))
			} else {
				seen[shape]++
			}
			continue
		}
		if expectedRelationship {
			violations = append(violations,
				fmt.Sprintf("%s relationship %q is missing @id", fixture.name, triple.Predicate))
		}
		if explicitNonEdge && triple.Datatype != "" {
			violations = append(violations,
				fmt.Sprintf("%s explicit non-edge %q has datatype %q", fixture.name, triple.Predicate, triple.Datatype))
		}
		if validEntityObject && !explicitNonEdge {
			violations = append(violations,
				fmt.Sprintf("%s unclassified entity-looking scalar %q on %q", fixture.name, object, triple.Predicate))
		}
	}
	for shape, wantCount := range expectedShapes {
		if seen[shape] != wantCount {
			violations = append(violations,
				fmt.Sprintf("%s shape %q cardinality = %d, want %d", fixture.name, shape, seen[shape], wantCount))
		}
	}
	return violations, seen
}

func normalizeFixturePredicate(producer, predicate string) string {
	// Producer- and fixture-exact normalization only. Do not generalize these
	// unresolved dynamic predicate families or validate the normalized labels
	// as canonical predicates.
	switch {
	case producer == guildMethod &&
		predicate == "guild.application."+guildApplicationSentinel+".applicant":
		return "guild.application.{id}.applicant"
	case producer == guildMethod &&
		predicate == "guild.application."+guildApplicationSentinel+".reviewed_by":
		return "guild.application.{id}.reviewed_by"
	case producer == agentMethod &&
		predicate == "agent.inventory.tool."+agentInventorySentinel:
		return "agent.inventory.tool.{id}"
	case producer == agentMethod &&
		predicate == "agent.effects."+agentEffectSentinel+".quest":
		return "agent.effects.{id}.quest"
	case producer == partyMethod &&
		predicate == "party.assignment."+partySubQuestSentinel:
		return "party.assignment.{id}"
	default:
		return predicate
	}
}

func stringObjectValue(object any) (string, bool) {
	if object == nil {
		return "", false
	}
	value := reflect.ValueOf(object)
	if value.Kind() != reflect.String {
		return "", false
	}
	return value.String(), true
}

func loadProductionGoSources(t *testing.T) map[string][]byte {
	t.Helper()
	return loadProductionGoSourcesAt(t, relationshipRepositoryRoot(t))
}

func loadProductionGoSourcesAt(t *testing.T, root string) map[string][]byte {
	t.Helper()
	sources := map[string][]byte{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path == root {
				return nil
			}
			name := entry.Name()
			if strings.HasPrefix(name, ".") || skippedProductionDirectory(name) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		sources[filepath.ToSlash(relative)] = data
		return nil
	})
	if err != nil {
		t.Fatalf("load production Go sources: %v", err)
	}
	return sources
}

func noEdgeFixtureInventoryViolations(fixtures []projectionFixture) []string {
	got := map[string]int{}
	for _, fixture := range fixtures {
		got[fixture.producer]++
	}
	want := map[string]int{}
	for method, class := range expectedTriplesMethods {
		if class == noEdgeMethod {
			want[method] = 1
		}
	}
	var violations []string
	for _, violation := range exactCountViolations(got, want) {
		violations = append(violations, "no-edge fixture producer: "+violation)
	}
	return violations
}

func skippedProductionDirectory(name string) bool {
	switch name {
	case "vendor", "node_modules", "cache", "caches", "build",
		"dist", "test-results", "playwright-report":
		return true
	default:
		return false
	}
}

func relationshipRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve relationship contract path")
	}
	return filepath.Dir(currentFile)
}

func scanRelationshipAST(sources map[string][]byte) (map[string]string, map[string]int, []string) {
	methods := map[string]string{}
	referenceSites := map[string]int{}
	var violations []string

	paths := make([]string, 0, len(sources))
	for path := range sources {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	for _, path := range paths {
		file, err := parser.ParseFile(token.NewFileSet(), path, sources[path], parser.ParseComments)
		if err != nil {
			violations = append(violations, fmt.Sprintf("parse %s: %v", path, err))
			continue
		}
		imports := importAliases(file)

		for _, declaration := range file.Decls {
			function, isFunction := declaration.(*ast.FuncDecl)
			if !isFunction {
				count, datatypeViolations := scanDatatypeWrites(declaration, imports)
				if count > 0 {
					referenceSites[path+"|<package>"] += count
				}
				for _, violation := range datatypeViolations {
					violations = append(violations, path+"|<package>: "+violation)
				}
				continue
			}
			receiver := receiverTypeName(function)
			if isTriplesCandidate(function) {
				if !isTriplesMethod(function, imports) {
					violations = append(
						violations,
						triplesMethodKey(path, receiver)+": unsupported Triples signature",
					)
				} else {
					key := triplesMethodKey(path, receiver)
					if _, duplicate := methods[key]; duplicate {
						violations = append(violations, "duplicate Triples method key "+key)
					}
					methods[key] = expectedTriplesMethods[key]
				}
			}
			count, datatypeViolations := scanDatatypeWrites(function, imports)
			site := path + "|" + function.Name.Name
			if receiver != "" {
				site = path + "|" + receiver + "|" + function.Name.Name
			}
			if count > 0 {
				referenceSites[site] += count
			}
			for _, violation := range datatypeViolations {
				violations = append(violations, site+": "+violation)
			}
		}
	}
	return methods, referenceSites, violations
}

func isTriplesCandidate(function *ast.FuncDecl) bool {
	return function.Recv != nil && function.Name.Name == "Triples"
}

func importAliases(file *ast.File) map[string]string {
	aliases := map[string]string{}
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := filepath.Base(path)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		aliases[name] = path
	}
	return aliases
}

func isTriplesMethod(function *ast.FuncDecl, imports map[string]string) bool {
	if function.Recv == nil || function.Name.Name != "Triples" ||
		function.Type.Params == nil || len(function.Type.Params.List) != 0 ||
		function.Type.Results == nil || len(function.Type.Results.List) != 1 {
		return false
	}
	result := function.Type.Results.List[0].Type
	array, ok := result.(*ast.ArrayType)
	if !ok || array.Len != nil {
		return false
	}
	selector, ok := array.Elt.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Triple" {
		return false
	}
	alias, ok := selector.X.(*ast.Ident)
	return ok && imports[alias.Name] == "github.com/c360studio/semstreams/message"
}

func receiverTypeName(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) != 1 {
		return ""
	}
	var receiverName func(ast.Expr) string
	receiverName = func(expression ast.Expr) string {
		switch value := expression.(type) {
		case *ast.Ident:
			return value.Name
		case *ast.StarExpr:
			return receiverName(value.X)
		case *ast.IndexExpr:
			return receiverName(value.X)
		case *ast.IndexListExpr:
			return receiverName(value.X)
		default:
			return ""
		}
	}
	return receiverName(function.Recv.List[0].Type)
}

func triplesMethodKey(path, receiver string) string {
	return path + "|" + receiver + "|Triples"
}

func scanDatatypeWrites(node ast.Node, imports map[string]string) (int, []string) {
	count := 0
	var violations []string
	ast.Inspect(node, func(child ast.Node) bool {
		switch value := child.(type) {
		case *ast.KeyValueExpr:
			key, ok := value.Key.(*ast.Ident)
			if !ok || key.Name != "Datatype" {
				return true
			}
			entityReference, supported := classifyDatatypeExpression(value.Value, imports)
			if !supported {
				violations = append(violations, "unsupported Datatype expression in keyed field")
			} else if entityReference {
				count++
			}
		case *ast.AssignStmt:
			for index, left := range value.Lhs {
				if !isDatatypeAssignmentTarget(left) {
					continue
				}
				if len(value.Rhs) != len(value.Lhs) {
					violations = append(violations, "unsupported Datatype expression in assignment")
					continue
				}
				entityReference, supported := classifyDatatypeExpression(value.Rhs[index], imports)
				if !supported {
					violations = append(violations, "unsupported Datatype expression in assignment")
				} else if entityReference {
					count++
				}
			}
		}
		return true
	})
	return count, violations
}

func isDatatypeAssignmentTarget(expression ast.Expr) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	return ok && selector.Sel.Name == "Datatype"
}

func classifyDatatypeExpression(expression ast.Expr, imports map[string]string) (entityReference, supported bool) {
	switch value := expression.(type) {
	case *ast.BasicLit:
		if value.Kind != token.STRING {
			return false, false
		}
		literal, err := strconv.Unquote(value.Value)
		return literal == "@id", err == nil
	case *ast.SelectorExpr:
		alias, ok := value.X.(*ast.Ident)
		if !ok || value.Sel.Name != "EntityReferenceDatatype" {
			return false, false
		}
		importPath := imports[alias.Name]
		entityReference = importPath == "github.com/c360studio/semstreams/message"
		return entityReference, entityReference
	default:
		return false, false
	}
}

func assertExactStringSet(t *testing.T, _ string, got, want map[string]string) {
	t.Helper()
	assertNoContractViolations(t, exactStringSetViolations(got, want))
}

func exactStringSetViolations(got, want map[string]string) []string {
	var violations []string
	for key := range got {
		if _, ok := want[key]; !ok {
			violations = append(violations, "unexpected "+key)
		}
	}
	for key := range want {
		if _, ok := got[key]; !ok {
			violations = append(violations, "missing "+key)
		}
	}
	sort.Strings(violations)
	return violations
}

func assertExactCounts(t *testing.T, label string, got, want map[string]int) {
	t.Helper()
	violations := exactCountViolations(got, want)
	for index := range violations {
		violations[index] = label + ": " + violations[index]
	}
	assertNoContractViolations(t, violations)
}

func exactCountViolations(got, want map[string]int) []string {
	var violations []string
	for key, gotCount := range got {
		wantCount, ok := want[key]
		if !ok {
			violations = append(violations, fmt.Sprintf("unexpected %s count %d", key, gotCount))
		} else if gotCount != wantCount {
			violations = append(violations, fmt.Sprintf("%s count = %d, want %d", key, gotCount, wantCount))
		}
	}
	for key, wantCount := range want {
		if _, ok := got[key]; !ok {
			violations = append(violations, fmt.Sprintf("missing %s count %d", key, wantCount))
		}
	}
	sort.Strings(violations)
	return violations
}

func assertNoContractViolations(t *testing.T, violations []string) {
	t.Helper()
	for _, violation := range violations {
		t.Error(violation)
	}
}

func containsViolation(violations []string, fragment string) bool {
	for _, violation := range violations {
		if strings.Contains(violation, fragment) {
			return true
		}
	}
	return false
}
