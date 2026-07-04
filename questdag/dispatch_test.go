//go:build integration

package questdag

// =============================================================================
// INTEGRATION TEST — dispatch handler → ClaimAndStartForParty (task 3.2)
// =============================================================================
// Proves the reactive half: a real gated-DAG executor dispatches seeded units,
// and questdag's durable dispatch handler claim-starts each unit's sub-quest on
// the best-skill-matched party member (lead excluded). Two independent units
// with distinct skill requirements must land on the two distinct skill-matching
// members; the lead is never assigned executor work. questboard/partycoord are
// mocked via the narrow interfaces so the test asserts the exact claim-start
// arguments; entity reads (units, sub-quests, agent availability) hit real
// graph-ingest.
//
// Requires Docker (NATS via testcontainers).
// =============================================================================

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/lifecycle"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/partycoord"
)

func TestDispatchHandler_ClaimStartsBestSkillMember(t *testing.T) {
	ctx := context.Background()
	board := &domain.BoardConfig{Org: "test", Platform: "integration", Board: "dagdispatch"}

	tc := natsclient.NewTestClient(t,
		natsclient.WithKV(), natsclient.WithFileStorage(),
		natsclient.WithKVBuckets(graph.BucketEntityStates),
		natsclient.WithStreams(natsclient.TestStreamConfig{Name: "ENTITY", Subjects: []string{"entity.>"}}),
	)
	client := tc.Client

	startGraphIngest(t, ctx, client)
	mgr := lifecycle.NewManager(client, slog.Default())

	gc := semdragons.NewGraphClient(client, board)
	if err := gc.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket: %v", err)
	}

	// Party: a lead + two members with distinct skills. The lead holds BOTH
	// skills but must never be assigned executor work.
	leadID := domain.AgentID(board.AgentEntityID("lead"))
	goMemberID := domain.AgentID(board.AgentEntityID("gopher"))
	pyMemberID := domain.AgentID(board.AgentEntityID("pythonista"))
	partyID := domain.PartyID(board.PartyEntityID("p1"))

	for _, id := range []domain.AgentID{leadID, goMemberID, pyMemberID} {
		createIdleAgent(t, gc, id)
	}

	party := &partycoord.Party{
		ID:   partyID,
		Lead: leadID,
		Members: []partycoord.PartyMember{
			{AgentID: leadID, Role: domain.RoleLead, Skills: []domain.SkillTag{"go", "python"}},
			{AgentID: goMemberID, Role: domain.RoleExecutor, Skills: []domain.SkillTag{"go"}},
			{AgentID: pyMemberID, Role: domain.RoleExecutor, Skills: []domain.SkillTag{"python"}},
		},
	}

	// Two posted, party-scoped sub-quests with distinct skill requirements.
	subGo := domain.QuestID(board.QuestEntityID("subgo"))
	subPy := domain.QuestID(board.QuestEntityID("subpy"))
	createPostedSubQuest(t, gc, subGo, partyID, []domain.SkillTag{"go"})
	createPostedSubQuest(t, gc, subPy, partyID, []domain.SkillTag{"python"})

	// Seed two independent units mapping to the two sub-quests.
	_, err := SeedUnits(ctx, client, board, SeedInput{
		DAG: DAG{Nodes: []Node{
			{ID: "go-node", Objective: "do go work", Skills: []string{"go"}},
			{ID: "py-node", Objective: "do python work", Skills: []string{"python"}},
		}},
		ParentQuestID: board.QuestEntityID("parent"),
		ExecutionID:   "exec-dispatch-1",
		NodeSubQuests: map[string]string{"go-node": string(subGo), "py-node": string(subPy)},
	})
	if err != nil {
		t.Fatalf("SeedUnits: %v", err)
	}

	// questdag component with mocked questboard/partycoord.
	qb := &mockQuestBoard{}
	pc := &mockParty{party: party}
	cfg := DefaultConfig()
	cfg.Org, cfg.Platform, cfg.Board = board.Org, board.Platform, board.Board
	cfg.DispatchSubject = "gateddag.dispatch.dagdispatch"
	cfg.ConsumerName = "questdag-dispatch-test"
	cfg.BackstopInterval = "1s"
	cfg.DispatchDedupeWindow = "1s"
	cfg.AckWaitSeconds = 6
	cfg.HeartbeatSeconds = 2

	comp, err := NewFromConfig(cfg, component.Dependencies{NATSClient: client, LifecycleManager: mgr})
	if err != nil {
		t.Fatalf("NewFromConfig: %v", err)
	}
	comp.questBoardOverride = qb
	comp.partyOverride = pc

	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = comp.Stop(5 * time.Second) })

	// Wait for both sub-quests to be claim-started.
	claims := qb.waitForClaims(t, 2, 25*time.Second)

	byQuest := map[domain.QuestID]domain.AgentID{}
	for _, c := range claims {
		if _, dup := byQuest[c.quest]; dup {
			t.Errorf("sub-quest %s claim-started more than once", c.quest)
		}
		byQuest[c.quest] = c.agent
		if c.party != partyID {
			t.Errorf("claim for %s used party %s, want %s", c.quest, c.party, partyID)
		}
		if c.agent == leadID {
			t.Errorf("lead %s was assigned executor work for %s (lead must be excluded)", leadID, c.quest)
		}
	}
	if byQuest[subGo] != goMemberID {
		t.Errorf("go sub-quest assigned to %s, want the go member %s", byQuest[subGo], goMemberID)
	}
	if byQuest[subPy] != pyMemberID {
		t.Errorf("python sub-quest assigned to %s, want the python member %s", byQuest[subPy], pyMemberID)
	}
}

// =============================================================================
// TEST DOUBLES + FIXTURES
// =============================================================================

type claimRecord struct {
	quest domain.QuestID
	party domain.PartyID
	agent domain.AgentID
}

type mockQuestBoard struct {
	mu     sync.Mutex
	claims []claimRecord
}

func (m *mockQuestBoard) ClaimAndStartForParty(_ context.Context, questID domain.QuestID, partyID domain.PartyID, assignedTo domain.AgentID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.claims = append(m.claims, claimRecord{quest: questID, party: partyID, agent: assignedTo})
	return nil
}

func (m *mockQuestBoard) waitForClaims(t *testing.T, want int, timeout time.Duration) []claimRecord {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		n := len(m.claims)
		m.mu.Unlock()
		if n >= want {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]claimRecord, len(m.claims))
	copy(out, m.claims)
	if len(out) < want {
		t.Fatalf("timed out: got %d/%d claim-starts", len(out), want)
	}
	return out
}

type mockParty struct {
	party *partycoord.Party
}

func (m *mockParty) GetParty(id domain.PartyID) (*partycoord.Party, bool) {
	if m.party != nil && m.party.ID == id {
		return m.party, true
	}
	return nil, false
}

func (m *mockParty) AssignTask(_ context.Context, _ domain.PartyID, _ domain.QuestID, _ domain.AgentID, _ string) error {
	return nil
}

func createIdleAgent(t *testing.T, gc *semdragons.GraphClient, id domain.AgentID) {
	t.Helper()
	agent := &agentprogression.Agent{ID: id, Name: string(id), Level: 8, Tier: domain.TierJourneyman, Status: domain.AgentIdle}
	if err := gc.PutEntityState(context.Background(), agent, "agent.identity.created"); err != nil {
		t.Fatalf("create idle agent %s: %v", id, err)
	}
}

func createPostedSubQuest(t *testing.T, gc *semdragons.GraphClient, id domain.QuestID, partyID domain.PartyID, skills []domain.SkillTag) {
	t.Helper()
	quest := &domain.Quest{
		ID:             id,
		Title:          "sub-quest " + string(id),
		Description:    "party sub-quest",
		Status:         domain.QuestPosted,
		Difficulty:     domain.DifficultyTrivial,
		MaxAttempts:    3,
		PartyID:        &partyID,
		RequiredSkills: skills,
	}
	if err := gc.PutEntityState(context.Background(), quest, "quest.posted"); err != nil {
		t.Fatalf("create posted sub-quest %s: %v", id, err)
	}
}
