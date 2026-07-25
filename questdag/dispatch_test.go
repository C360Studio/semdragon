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
	"errors"
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
	_, err := SeedUnits(ctx, client, newIntegrationCompletionWriter(t, client), board, SeedInput{
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

func TestDispatchHandler_ConcurrentCallbacksReservePartyMember(t *testing.T) {
	ctx := context.Background()
	board := &domain.BoardConfig{Org: "test", Platform: "integration", Board: "dagreserve"}
	tc := natsclient.NewTestClient(t,
		natsclient.WithKV(), natsclient.WithFileStorage(),
		natsclient.WithKVBuckets(graph.BucketEntityStates),
		natsclient.WithStreams(natsclient.TestStreamConfig{Name: "ENTITY", Subjects: []string{"entity.>"}}),
	)
	client := tc.Client
	startGraphIngest(t, ctx, client)
	gc := semdragons.NewGraphClient(client, board)
	if err := gc.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket: %v", err)
	}

	leadID := domain.AgentID(board.AgentEntityID("lead"))
	memberID := domain.AgentID(board.AgentEntityID("member"))
	partyID := domain.PartyID(board.PartyEntityID("p1"))
	createIdleAgent(t, gc, leadID)
	createIdleAgent(t, gc, memberID)
	party := &partycoord.Party{
		ID: partyID, Lead: leadID,
		Members: []partycoord.PartyMember{
			{AgentID: leadID, Role: domain.RoleLead},
			{AgentID: memberID, Role: domain.RoleExecutor, Skills: []domain.SkillTag{"go"}},
		},
	}

	subA := domain.QuestID(board.QuestEntityID("sub-a"))
	subB := domain.QuestID(board.QuestEntityID("sub-b"))
	createPostedSubQuest(t, gc, subA, partyID, []domain.SkillTag{"go"})
	createPostedSubQuest(t, gc, subB, partyID, []domain.SkillTag{"go"})
	seeded, err := SeedUnits(ctx, client, newIntegrationCompletionWriter(t, client), board, SeedInput{
		DAG: DAG{Nodes: []Node{
			{ID: "a", Objective: "first", Skills: []string{"go"}},
			{ID: "b", Objective: "second", Skills: []string{"go"}},
		}},
		ParentQuestID: board.QuestEntityID("parent"),
		ExecutionID:   "reserve-exec",
		NodeSubQuests: map[string]string{"a": string(subA), "b": string(subB)},
	})
	if err != nil {
		t.Fatalf("SeedUnits: %v", err)
	}

	qb := newBlockingQuestBoard()
	comp := &Component{
		graph:              gc,
		logger:             slog.Default(),
		decodeUnit:         func(data []byte) (string, bool) { return string(data), true },
		questBoardOverride: qb,
		partyOverride:      &mockParty{party: party},
	}

	firstDone := make(chan error, 1)
	go func() { firstDone <- comp.handleDispatch(ctx, []byte(seeded.NodeUnits["a"])) }()
	select {
	case <-qb.firstEntered:
	case <-ctxDeadline(t, 5*time.Second):
		t.Fatal("first dispatch did not reach claim-start")
	}

	secondDone := make(chan error, 1)
	go func() { secondDone <- comp.handleDispatch(ctx, []byte(seeded.NodeUnits["b"])) }()
	select {
	case <-qb.secondEntered:
		t.Fatal("concurrent dispatch double-booked the reserved party member")
	case err := <-secondDone:
		if err == nil {
			t.Fatal("second dispatch succeeded while the only member was reserved")
		}
	case <-ctxDeadline(t, 5*time.Second):
		t.Fatal("second dispatch did not resolve while first claim-start was blocked")
	}

	close(qb.releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("first dispatch: %v", err)
	}
}

func TestDispatchHandler_FailedClaimStartReleasesReservation(t *testing.T) {
	ctx := context.Background()
	board := &domain.BoardConfig{Org: "test", Platform: "integration", Board: "dagrelease"}
	tc := natsclient.NewTestClient(t,
		natsclient.WithKV(), natsclient.WithFileStorage(),
		natsclient.WithKVBuckets(graph.BucketEntityStates),
		natsclient.WithStreams(natsclient.TestStreamConfig{Name: "ENTITY", Subjects: []string{"entity.>"}}),
	)
	client := tc.Client
	startGraphIngest(t, ctx, client)
	gc := semdragons.NewGraphClient(client, board)
	if err := gc.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket: %v", err)
	}

	leadID := domain.AgentID(board.AgentEntityID("lead"))
	memberID := domain.AgentID(board.AgentEntityID("member"))
	partyID := domain.PartyID(board.PartyEntityID("p1"))
	createIdleAgent(t, gc, leadID)
	createIdleAgent(t, gc, memberID)
	party := &partycoord.Party{ID: partyID, Lead: leadID, Members: []partycoord.PartyMember{
		{AgentID: leadID, Role: domain.RoleLead},
		{AgentID: memberID, Role: domain.RoleExecutor},
	}}

	subA := domain.QuestID(board.QuestEntityID("sub-a"))
	subB := domain.QuestID(board.QuestEntityID("sub-b"))
	createPostedSubQuest(t, gc, subA, partyID, nil)
	createPostedSubQuest(t, gc, subB, partyID, nil)
	seeded, err := SeedUnits(ctx, client, newIntegrationCompletionWriter(t, client), board, SeedInput{
		DAG: DAG{Nodes: []Node{
			{ID: "a", Objective: "first"},
			{ID: "b", Objective: "second"},
		}},
		ParentQuestID: board.QuestEntityID("parent"),
		ExecutionID:   "release-exec",
		NodeSubQuests: map[string]string{"a": string(subA), "b": string(subB)},
	})
	if err != nil {
		t.Fatalf("SeedUnits: %v", err)
	}

	qb := &failOnceQuestBoard{err: errors.New("claim-start failed")}
	comp := &Component{
		graph: gc, logger: slog.Default(),
		decodeUnit:         func(data []byte) (string, bool) { return string(data), true },
		questBoardOverride: qb,
		partyOverride:      &mockParty{party: party},
	}
	if err := comp.handleDispatch(ctx, []byte(seeded.NodeUnits["a"])); err == nil {
		t.Fatal("first dispatch unexpectedly succeeded")
	}
	if err := comp.handleDispatch(ctx, []byte(seeded.NodeUnits["b"])); err != nil {
		t.Fatalf("second dispatch could not reuse reservation after failed claim-start: %v", err)
	}
	if qb.calls != 2 {
		t.Fatalf("claim-start calls = %d, want 2", qb.calls)
	}
}

func TestDispatchHandler_CommittedReservationFollowsQuestPhase(t *testing.T) {
	ctx := context.Background()
	board := &domain.BoardConfig{Org: "test", Platform: "integration", Board: "dagcommit"}
	tc := natsclient.NewTestClient(t,
		natsclient.WithKV(), natsclient.WithFileStorage(),
		natsclient.WithKVBuckets(graph.BucketEntityStates),
		natsclient.WithStreams(natsclient.TestStreamConfig{Name: "ENTITY", Subjects: []string{"entity.>"}}),
	)
	client := tc.Client
	startGraphIngest(t, ctx, client)
	gc := semdragons.NewGraphClient(client, board)
	if err := gc.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket: %v", err)
	}

	leadID := domain.AgentID(board.AgentEntityID("lead"))
	memberID := domain.AgentID(board.AgentEntityID("member"))
	partyID := domain.PartyID(board.PartyEntityID("p1"))
	createIdleAgent(t, gc, leadID)
	createIdleAgent(t, gc, memberID)
	party := &partycoord.Party{ID: partyID, Lead: leadID, Members: []partycoord.PartyMember{
		{AgentID: leadID, Role: domain.RoleLead},
		{AgentID: memberID, Role: domain.RoleExecutor},
	}}

	subA := domain.QuestID(board.QuestEntityID("sub-a"))
	subB := domain.QuestID(board.QuestEntityID("sub-b"))
	createPostedSubQuest(t, gc, subA, partyID, nil)
	createPostedSubQuest(t, gc, subB, partyID, nil)
	seeded, err := SeedUnits(ctx, client, newIntegrationCompletionWriter(t, client), board, SeedInput{
		DAG: DAG{Nodes: []Node{
			{ID: "a", Objective: "first"},
			{ID: "b", Objective: "second"},
		}},
		ParentQuestID: board.QuestEntityID("parent"),
		ExecutionID:   "commit-exec",
		NodeSubQuests: map[string]string{"a": string(subA), "b": string(subB)},
	})
	if err != nil {
		t.Fatalf("SeedUnits: %v", err)
	}

	qb := &phaseWritingQuestBoard{graph: gc}
	comp := &Component{
		graph: gc, logger: slog.Default(),
		decodeUnit:         func(data []byte) (string, bool) { return string(data), true },
		questBoardOverride: qb,
		partyOverride:      &mockParty{party: party},
	}

	if err := comp.handleDispatch(ctx, []byte(seeded.NodeUnits["a"])); err != nil {
		t.Fatalf("first dispatch: %v", err)
	}
	if qb.calls != 1 {
		t.Fatalf("claim-start calls after first dispatch = %d, want 1", qb.calls)
	}

	// The claim-start mock leaves the authoritative quest in claimed. A
	// committed reservation must keep the still-idle graph agent unavailable.
	if err := comp.handleDispatch(ctx, []byte(seeded.NodeUnits["b"])); err == nil {
		t.Fatal("second dispatch succeeded while first quest remained claimed")
	}
	if qb.calls != 1 {
		t.Fatalf("claimed reservation reached claim-start again: calls=%d, want 1", qb.calls)
	}

	writePartySubQuestPhase(t, gc, subA, partyID, &memberID, domain.QuestInProgress)
	if err := comp.handleDispatch(ctx, []byte(seeded.NodeUnits["b"])); err == nil {
		t.Fatal("second dispatch succeeded while first quest remained in_progress")
	}
	if qb.calls != 1 {
		t.Fatalf("in_progress reservation reached claim-start again: calls=%d, want 1", qb.calls)
	}

	// Once the authoritative quest leaves claimed/in_progress, selection must
	// retire the committed reservation and permit the member to be reused.
	writePartySubQuestPhase(t, gc, subA, partyID, nil, domain.QuestCompleted)
	if err := comp.handleDispatch(ctx, []byte(seeded.NodeUnits["b"])); err != nil {
		t.Fatalf("second dispatch could not reuse member after first quest completed: %v", err)
	}
	if qb.calls != 2 {
		t.Fatalf("claim-start calls after reservation retirement = %d, want 2", qb.calls)
	}
}

// =============================================================================
// TEST DOUBLES + FIXTURES
// =============================================================================

func ctxDeadline(t *testing.T, timeout time.Duration) <-chan time.Time {
	t.Helper()
	return time.After(timeout)
}

type blockingQuestBoard struct {
	mu            sync.Mutex
	calls         int
	firstEntered  chan struct{}
	secondEntered chan struct{}
	releaseFirst  chan struct{}
}

func newBlockingQuestBoard() *blockingQuestBoard {
	return &blockingQuestBoard{
		firstEntered:  make(chan struct{}),
		secondEntered: make(chan struct{}),
		releaseFirst:  make(chan struct{}),
	}
}

func (m *blockingQuestBoard) ClaimAndStartForParty(_ context.Context, _ domain.QuestID, _ domain.PartyID, _ domain.AgentID) error {
	m.mu.Lock()
	m.calls++
	call := m.calls
	m.mu.Unlock()
	if call == 1 {
		close(m.firstEntered)
		<-m.releaseFirst
	} else {
		close(m.secondEntered)
	}
	return nil
}

type failOnceQuestBoard struct {
	calls int
	err   error
}

func (m *failOnceQuestBoard) ClaimAndStartForParty(_ context.Context, _ domain.QuestID, _ domain.PartyID, _ domain.AgentID) error {
	m.calls++
	if m.calls == 1 {
		return m.err
	}
	return nil
}

type phaseWritingQuestBoard struct {
	graph *semdragons.GraphClient
	calls int
}

func (m *phaseWritingQuestBoard) ClaimAndStartForParty(ctx context.Context, questID domain.QuestID, partyID domain.PartyID, assignedTo domain.AgentID) error {
	m.calls++
	quest := &domain.Quest{
		ID:          questID,
		Title:       "claimed sub-quest " + string(questID),
		Description: "claimed by deterministic questboard test double",
		Status:      domain.QuestClaimed,
		Difficulty:  domain.DifficultyTrivial,
		MaxAttempts: 3,
		PartyID:     &partyID,
		ClaimedBy:   &assignedTo,
	}
	return m.graph.PutEntityState(ctx, quest, "quest.claimed")
}

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

func writePartySubQuestPhase(t *testing.T, gc *semdragons.GraphClient, id domain.QuestID, partyID domain.PartyID, claimedBy *domain.AgentID, status domain.QuestStatus) {
	t.Helper()
	quest := &domain.Quest{
		ID:          id,
		Title:       "sub-quest " + string(id),
		Description: "party sub-quest",
		Status:      status,
		Difficulty:  domain.DifficultyTrivial,
		MaxAttempts: 3,
		PartyID:     &partyID,
		ClaimedBy:   claimedBy,
	}
	if err := gc.PutEntityState(context.Background(), quest, "quest.phase.test"); err != nil {
		t.Fatalf("write sub-quest %s phase %s: %v", id, status, err)
	}
}
