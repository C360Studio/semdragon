package questdagexec

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"testing"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/partycoord"
)

// =============================================================================
// MOCK IMPLEMENTATIONS for QuestBoardRef and PartyCoordRef
// =============================================================================

type mockQuestBoardRef struct {
	mu                 sync.Mutex
	submitCalls        []submitCall
	failCalls          []failCall
	escalateCalls      []escalateCall
	claimAndStartCalls []claimForPartyCall
	submitErr          error
	failErr            error
	escalateErr        error
	claimAndStartErr   error
}

type submitCall struct {
	questID domain.QuestID
	result  any
}

type failCall struct {
	questID domain.QuestID
	reason  string
}

type escalateCall struct {
	questID domain.QuestID
	reason  string
}

type claimForPartyCall struct {
	questID    domain.QuestID
	partyID    domain.PartyID
	assignedTo domain.AgentID
}

func (m *mockQuestBoardRef) SubmitResult(_ context.Context, questID domain.QuestID, result any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.submitCalls = append(m.submitCalls, submitCall{questID, result})
	return m.submitErr
}

func (m *mockQuestBoardRef) FailQuest(_ context.Context, questID domain.QuestID, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failCalls = append(m.failCalls, failCall{questID, reason})
	return m.failErr
}

func (m *mockQuestBoardRef) EscalateQuest(_ context.Context, questID domain.QuestID, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.escalateCalls = append(m.escalateCalls, escalateCall{questID, reason})
	return m.escalateErr
}

func (m *mockQuestBoardRef) ClaimAndStartForParty(_ context.Context, questID domain.QuestID, partyID domain.PartyID, assignedTo domain.AgentID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.claimAndStartCalls = append(m.claimAndStartCalls, claimForPartyCall{questID, partyID, assignedTo})
	return m.claimAndStartErr
}

func (m *mockQuestBoardRef) RepostForRetry(_ context.Context, _ domain.QuestID) error {
	return nil
}

// SubmitCallCount returns the number of submit calls (thread-safe).
func (m *mockQuestBoardRef) SubmitCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.submitCalls)
}

// ClaimCallCount returns the number of ClaimAndStart calls (thread-safe).
func (m *mockQuestBoardRef) ClaimCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.claimAndStartCalls)
}

// GetSubmitCall returns the i-th submit call (thread-safe).
func (m *mockQuestBoardRef) GetSubmitCall(i int) submitCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.submitCalls[i]
}

type mockPartyCoordRef struct {
	mu           sync.Mutex
	joinCalls    []partyJoinCall
	assignCalls  []partyAssignCall
	parties      map[domain.PartyID]*partycoord.Party
	disbandCalls []disbandCall
	joinErr      error
	assignErr    error
	disbandErr   error
}

type partyJoinCall struct {
	partyID domain.PartyID
	agentID domain.AgentID
	role    domain.PartyRole
}

type partyAssignCall struct {
	partyID    domain.PartyID
	subQuestID domain.QuestID
	agentID    domain.AgentID
	rationale  string
}

type disbandCall struct {
	partyID domain.PartyID
	reason  string
}

func (m *mockPartyCoordRef) JoinParty(_ context.Context, partyID domain.PartyID, agentID domain.AgentID, role domain.PartyRole) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.joinCalls = append(m.joinCalls, partyJoinCall{partyID, agentID, role})
	return m.joinErr
}

func (m *mockPartyCoordRef) AssignTask(_ context.Context, partyID domain.PartyID, subQuestID domain.QuestID, agentID domain.AgentID, rationale string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.assignCalls = append(m.assignCalls, partyAssignCall{partyID, subQuestID, agentID, rationale})
	return m.assignErr
}

func (m *mockPartyCoordRef) GetParty(partyID domain.PartyID) (*partycoord.Party, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.parties == nil {
		return nil, false
	}
	p, ok := m.parties[partyID]
	return p, ok
}

func (m *mockPartyCoordRef) DisbandParty(_ context.Context, partyID domain.PartyID, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.disbandCalls = append(m.disbandCalls, disbandCall{partyID, reason})
	return m.disbandErr
}

// DisbandCallCount returns the number of disband calls (thread-safe).
func (m *mockPartyCoordRef) DisbandCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.disbandCalls)
}

// =============================================================================
// COMPONENT FACTORY HELPERS
// =============================================================================

// newTestComponent constructs a Component with mocked sibling refs suitable
// for unit tests. The graph client is nil since handler functions that load
// quest entities from KV are tested via integration tests.
//
// Plain maps are initialized so event loop handler methods can be called
// directly without nil-map panics.
func newTestComponent(qb *mockQuestBoardRef, pc *mockPartyCoordRef) *Component {
	config := DefaultConfig()
	c := &Component{
		config:        &config,
		logger:        slog.Default(),
		dagCache:      make(map[string]*DAGExecutionState),
		dagBySubQuest: make(map[string]*DAGExecutionState),
		questCache:    make(map[string]string),
	}
	// Assign through interface only when non-nil to avoid the Go
	// "typed nil wraps to non-nil interface" trap.
	if qb != nil {
		c.questBoardRef = qb
	}
	if pc != nil {
		c.partyCoord = pc
	}
	return c
}

// =============================================================================
// DAG STATE FACTORY HELPERS
// =============================================================================

// makeFullDAGState builds a DAGExecutionState ready for handler tests.
// All nodes start in NodePending; NodeQuestIDs maps nodeID → "quest-"+nodeID.
func makeFullDAGState(executionID, parentQuestID, partyID string, nodes []QuestNode) *DAGExecutionState {
	nodeStates := make(map[string]string, len(nodes))
	nodeQuestIDs := make(map[string]string, len(nodes))
	nodeAssignees := make(map[string]string)
	nodeRetries := make(map[string]int, len(nodes))

	for _, n := range nodes {
		nodeStates[n.ID] = NodePending
		nodeQuestIDs[n.ID] = "quest-" + n.ID
		nodeRetries[n.ID] = 2
	}

	return &DAGExecutionState{
		ExecutionID:   executionID,
		ParentQuestID: parentQuestID,
		PartyID:       partyID,
		DAG:           QuestDAG{Nodes: nodes},
		NodeStates:    nodeStates,
		NodeQuestIDs:  nodeQuestIDs,
		NodeAssignees: nodeAssignees,
		NodeRetries:   nodeRetries,
	}
}

// =============================================================================
// findDAGForSubQuest TESTS
// =============================================================================

func TestFindDAGForSubQuest(t *testing.T) {
	t.Parallel()

	c := newTestComponent(nil, nil)

	dag := makeFullDAGState("exec-1", "parent-1", "party-1", []QuestNode{
		makeNode("n1", 0),
	})

	// Store with a known key directly in the plain map.
	c.dagBySubQuest["quest.local.dev.game.board1.quest.abc"] = dag

	tests := []struct {
		name      string
		entityKey string
		wantNil   bool
	}{
		{
			name:      "known sub-quest key returns DAG",
			entityKey: "quest.local.dev.game.board1.quest.abc",
			wantNil:   false,
		},
		{
			name:      "unknown sub-quest key returns nil",
			entityKey: "quest.local.dev.game.board1.quest.zzz",
			wantNil:   true,
		},
		{
			name:      "empty key returns nil",
			entityKey: "",
			wantNil:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := c.findDAGForSubQuest(tc.entityKey)
			if tc.wantNil && result != nil {
				t.Errorf("findDAGForSubQuest(%q) = non-nil, want nil", tc.entityKey)
			}
			if !tc.wantNil && result == nil {
				t.Errorf("findDAGForSubQuest(%q) = nil, want non-nil", tc.entityKey)
			}
		})
	}
}

// =============================================================================
// findNodeForQuest TESTS
// =============================================================================

func TestFindNodeForQuest(t *testing.T) {
	t.Parallel()

	c := newTestComponent(nil, nil)

	dag := makeFullDAGState("exec-2", "parent-2", "party-2", []QuestNode{
		makeNode("n1", 0),
		makeNode("n2", 0),
		makeNode("n3", 0),
	})
	// Override to use realistic IDs.
	dag.NodeQuestIDs["n1"] = "local.dev.game.board1.quest.q1abc"
	dag.NodeQuestIDs["n2"] = "local.dev.game.board1.quest.q2def"
	dag.NodeQuestIDs["n3"] = "q3ghi"

	tests := []struct {
		name       string
		questID    string
		wantNodeID string
	}{
		{
			name:       "exact full-entity-ID match",
			questID:    "local.dev.game.board1.quest.q1abc",
			wantNodeID: "n1",
		},
		{
			name:       "instance-only match against full-entity-ID value",
			questID:    "q2def",
			wantNodeID: "n2",
		},
		{
			name:       "full-entity-ID match against instance-only value",
			questID:    "local.dev.game.board1.quest.q3ghi",
			wantNodeID: "n3",
		},
		{
			name:       "instance-only match against instance-only value",
			questID:    "q3ghi",
			wantNodeID: "n3",
		},
		{
			name:       "unknown quest ID returns empty string",
			questID:    "unknown-quest",
			wantNodeID: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := c.findNodeForQuest(dag, tc.questID)
			if got != tc.wantNodeID {
				t.Errorf("findNodeForQuest(%q) = %q, want %q", tc.questID, got, tc.wantNodeID)
			}
		})
	}
}

// =============================================================================
// isDAGComplete TESTS
// =============================================================================

func TestIsDAGComplete(t *testing.T) {
	t.Parallel()

	c := newTestComponent(nil, nil)

	nodes := []QuestNode{
		makeNode("n1", 0),
		makeNode("n2", 0),
		makeNode("n3", 0),
	}

	tests := []struct {
		name       string
		setupState func() *DAGExecutionState
		want       bool
	}{
		{
			name: "all nodes completed returns true",
			setupState: func() *DAGExecutionState {
				dag := makeFullDAGState("exec-3", "p", "party", nodes)
				dag.NodeStates["n1"] = NodeCompleted
				dag.NodeStates["n2"] = NodeCompleted
				dag.NodeStates["n3"] = NodeCompleted
				return dag
			},
			want: true,
		},
		{
			name: "one node still in_progress returns false",
			setupState: func() *DAGExecutionState {
				dag := makeFullDAGState("exec-4", "p", "party", nodes)
				dag.NodeStates["n1"] = NodeCompleted
				dag.NodeStates["n2"] = NodeCompleted
				dag.NodeStates["n3"] = NodeInProgress
				return dag
			},
			want: false,
		},
		{
			name: "one node failed returns false",
			setupState: func() *DAGExecutionState {
				dag := makeFullDAGState("exec-5", "p", "party", nodes)
				dag.NodeStates["n1"] = NodeCompleted
				dag.NodeStates["n2"] = NodeCompleted
				dag.NodeStates["n3"] = NodeFailed
				return dag
			},
			want: false,
		},
		{
			name: "all nodes pending returns false",
			setupState: func() *DAGExecutionState {
				return makeFullDAGState("exec-6", "p", "party", nodes)
			},
			want: false,
		},
		{
			name: "empty DAG returns false",
			setupState: func() *DAGExecutionState {
				return makeFullDAGState("exec-7", "p", "party", nil)
			},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dagState := tc.setupState()
			got := c.isDAGComplete(dagState)
			if got != tc.want {
				t.Errorf("isDAGComplete() = %v, want %v", got, tc.want)
			}
		})
	}
}

// =============================================================================
// handleNodeCompleted TESTS
// =============================================================================

func TestHandleNodeCompleted(t *testing.T) {
	t.Parallel()

	t.Run("single node — marks completed and triggers rollup", func(t *testing.T) {
		t.Parallel()

		qb := &mockQuestBoardRef{}
		config := DefaultConfig()
		c := &Component{
			config:        &config,
			questBoardRef: qb,
			partyCoord:    &mockPartyCoordRef{},
			logger:        slog.Default(),
			dagCache:      make(map[string]*DAGExecutionState),
			dagBySubQuest: make(map[string]*DAGExecutionState),
			questCache:    make(map[string]string),
		}

		dag := makeFullDAGState("exec-c1", "parent-quest-1", "party-c1", []QuestNode{
			makeNode("n1", 0),
		})
		dag.NodeStates["n1"] = NodeInProgress

		c.handleNodeCompleted(context.Background(), dag, "n1")

		if dag.NodeStates["n1"] != NodeCompleted {
			t.Errorf("node state = %q, want %q", dag.NodeStates["n1"], NodeCompleted)
		}
		if len(dag.CompletedNodes) != 1 || dag.CompletedNodes[0] != "n1" {
			t.Errorf("CompletedNodes = %v, want [n1]", dag.CompletedNodes)
		}
		// Single node DAG is complete — rollup should fire.
		if len(qb.submitCalls) != 1 {
			t.Errorf("SubmitResult called %d times, want 1 (single-node DAG)", len(qb.submitCalls))
		}
	})

	t.Run("first of two nodes — recomputes ready nodes", func(t *testing.T) {
		t.Parallel()

		qb := &mockQuestBoardRef{}
		pc := &mockPartyCoordRef{}
		c := newTestComponent(qb, pc)

		// n1 has no deps; n2 depends on n1.
		dag := makeFullDAGState("exec-c2", "parent-quest-2", "party-c2", []QuestNode{
			{ID: "n1", Objective: "Step 1"},
			{ID: "n2", Objective: "Step 2", DependsOn: []string{"n1"}},
		})
		dag.NodeStates["n1"] = NodeInProgress

		c.handleNodeCompleted(context.Background(), dag, "n1")

		if dag.NodeStates["n1"] != NodeCompleted {
			t.Errorf("n1 state = %q, want completed", dag.NodeStates["n1"])
		}
		// n2 was pending with n1 as dep; after n1 completes it should be ready.
		if dag.NodeStates["n2"] != NodeReady {
			t.Errorf("n2 state = %q, want ready (deps satisfied)", dag.NodeStates["n2"])
		}
	})

	t.Run("second node completes — DAG complete flag set", func(t *testing.T) {
		t.Parallel()

		qb := &mockQuestBoardRef{}
		pc := &mockPartyCoordRef{}
		config := DefaultConfig()
		c := &Component{
			config:        &config,
			questBoardRef: qb,
			partyCoord:    pc,
			logger:        slog.Default(),
			dagCache:      make(map[string]*DAGExecutionState),
			dagBySubQuest: make(map[string]*DAGExecutionState),
			questCache:    make(map[string]string),
		}

		dag := makeFullDAGState("exec-c3", "parent-quest-3", "party-c3", []QuestNode{
			makeNode("n1", 0),
			makeNode("n2", 0),
		})
		dag.NodeStates["n1"] = NodeCompleted
		dag.CompletedNodes = []string{"n1"}
		dag.NodeStates["n2"] = NodeInProgress

		// After n2 completes, isDAGComplete returns true and triggerRollup fires.
		// With a nil graph client, GetQuest will return an error but SubmitResult
		// is still called with an empty outputs map.
		c.handleNodeCompleted(context.Background(), dag, "n2")

		if dag.NodeStates["n2"] != NodeCompleted {
			t.Errorf("n2 state = %q, want completed", dag.NodeStates["n2"])
		}
		if len(dag.CompletedNodes) != 2 {
			t.Errorf("CompletedNodes length = %d, want 2", len(dag.CompletedNodes))
		}
		// SubmitResult should have been called on questBoard.
		if len(qb.submitCalls) != 1 {
			t.Errorf("SubmitResult called %d times, want 1", len(qb.submitCalls))
		}
		if qb.submitCalls[0].questID != "parent-quest-3" {
			t.Errorf("SubmitResult questID = %q, want %q", qb.submitCalls[0].questID, "parent-quest-3")
		}
		// DisbandParty should have been called.
		if len(pc.disbandCalls) != 1 {
			t.Errorf("DisbandParty called %d times, want 1", len(pc.disbandCalls))
		}
	})
}

// =============================================================================
// handleNodeFailed TESTS
// =============================================================================

func TestHandleNodeFailed(t *testing.T) {
	t.Parallel()

	t.Run("retries remaining — node reset to pending for retry", func(t *testing.T) {
		t.Parallel()

		qb := &mockQuestBoardRef{}
		c := newTestComponent(qb, nil)

		dag := makeFullDAGState("exec-f1", "parent-f1", "party-f1", []QuestNode{
			makeNode("n1", 0),
		})
		dag.NodeStates["n1"] = NodeInProgress
		dag.NodeRetries["n1"] = 2

		c.handleNodeFailed(context.Background(), dag, "n1")

		// retryNodeAssignment sets the node to NodePending then immediately
		// calls promoteReadyNodes. Since n1 has no dependencies it is promoted
		// to NodeReady in the same call. The ready state signals that it is
		// eligible for re-assignment on the next assignment pass.
		if dag.NodeStates["n1"] != NodeReady {
			t.Errorf("node state = %q after retry, want %q (no-dep node promoted to ready)", dag.NodeStates["n1"], NodeReady)
		}
		if dag.NodeRetries["n1"] != 1 {
			t.Errorf("retries = %d, want 1", dag.NodeRetries["n1"])
		}
		// No escalation when retries remain.
		if len(qb.escalateCalls) != 0 {
			t.Errorf("EscalateQuest called %d times, want 0", len(qb.escalateCalls))
		}
		// Node should not be in FailedNodes.
		if len(dag.FailedNodes) != 0 {
			t.Errorf("FailedNodes = %v, want empty", dag.FailedNodes)
		}
	})

	t.Run("no retries remaining — node marked failed and parent escalated", func(t *testing.T) {
		t.Parallel()

		qb := &mockQuestBoardRef{}
		c := newTestComponent(qb, nil)

		dag := makeFullDAGState("exec-f2", "parent-f2", "party-f2", []QuestNode{
			makeNode("n1", 0),
		})
		dag.NodeStates["n1"] = NodeInProgress
		dag.NodeRetries["n1"] = 0

		c.handleNodeFailed(context.Background(), dag, "n1")

		if dag.NodeStates["n1"] != NodeFailed {
			t.Errorf("node state = %q, want %q", dag.NodeStates["n1"], NodeFailed)
		}
		if len(dag.FailedNodes) != 1 || dag.FailedNodes[0] != "n1" {
			t.Errorf("FailedNodes = %v, want [n1]", dag.FailedNodes)
		}
		if len(qb.escalateCalls) != 1 {
			t.Errorf("EscalateQuest called %d times, want 1", len(qb.escalateCalls))
		}
		if qb.escalateCalls[0].questID != "parent-f2" {
			t.Errorf("escalate questID = %q, want %q", qb.escalateCalls[0].questID, "parent-f2")
		}
		if !stringContains(qb.escalateCalls[0].reason, "n1") {
			t.Errorf("escalate reason %q does not mention node ID", qb.escalateCalls[0].reason)
		}
	})

	t.Run("retry countdown: 2 → 1 → 0 then escalation", func(t *testing.T) {
		t.Parallel()

		qb := &mockQuestBoardRef{}
		c := newTestComponent(qb, nil)

		dag := makeFullDAGState("exec-f3", "parent-f3", "party-f3", []QuestNode{
			makeNode("n1", 0),
		})
		dag.NodeRetries["n1"] = 2

		// First failure: retries 2→1.
		dag.NodeStates["n1"] = NodeInProgress
		c.handleNodeFailed(context.Background(), dag, "n1")
		if dag.NodeRetries["n1"] != 1 {
			t.Fatalf("retries after 1st fail = %d, want 1", dag.NodeRetries["n1"])
		}
		if len(qb.escalateCalls) != 0 {
			t.Fatalf("unexpected escalation after 1st fail")
		}

		// Second failure: retries 1→0.
		dag.NodeStates["n1"] = NodeInProgress
		c.handleNodeFailed(context.Background(), dag, "n1")
		if dag.NodeRetries["n1"] != 0 {
			t.Fatalf("retries after 2nd fail = %d, want 0", dag.NodeRetries["n1"])
		}
		if len(qb.escalateCalls) != 0 {
			t.Fatalf("unexpected escalation after 2nd fail")
		}

		// Third failure: no retries left — escalate.
		dag.NodeStates["n1"] = NodeInProgress
		c.handleNodeFailed(context.Background(), dag, "n1")
		if dag.NodeStates["n1"] != NodeFailed {
			t.Errorf("node state = %q after exhausting retries, want failed", dag.NodeStates["n1"])
		}
		if len(qb.escalateCalls) != 1 {
			t.Errorf("EscalateQuest called %d times after exhausting retries, want 1", len(qb.escalateCalls))
		}
	})
}

// =============================================================================
// triggerRollup TESTS
// =============================================================================

func TestTriggerRollup(t *testing.T) {
	t.Parallel()

	t.Run("rollup calls SubmitResult and DisbandParty", func(t *testing.T) {
		t.Parallel()

		qb := &mockQuestBoardRef{}
		pc := &mockPartyCoordRef{}
		c := newTestComponent(qb, pc)

		dag := makeFullDAGState("exec-r1", "parent-r1", "party-r1", []QuestNode{
			makeNode("n1", 0),
			makeNode("n2", 0),
		})
		dag.NodeStates["n1"] = NodeCompleted
		dag.NodeStates["n2"] = NodeCompleted
		dag.CompletedNodes = []string{"n1", "n2"}

		// Trigger rollup — with nil graph client, GetQuest returns errors
		// so outputs will be empty, but SubmitResult and DisbandParty should
		// still be called.
		c.triggerRollup(context.Background(), dag)

		if len(qb.submitCalls) != 1 {
			t.Errorf("SubmitResult called %d times, want 1", len(qb.submitCalls))
		}
		if qb.submitCalls[0].questID != "parent-r1" {
			t.Errorf("SubmitResult questID = %q, want %q", qb.submitCalls[0].questID, "parent-r1")
		}
		if len(pc.disbandCalls) != 1 {
			t.Errorf("DisbandParty called %d times, want 1", len(pc.disbandCalls))
		}
		if pc.disbandCalls[0].partyID != "party-r1" {
			t.Errorf("DisbandParty partyID = %q, want %q", pc.disbandCalls[0].partyID, "party-r1")
		}
	})

	t.Run("rollup with nil questBoardRef — no panic", func(t *testing.T) {
		t.Parallel()

		pc := &mockPartyCoordRef{}
		// Use the config struct directly so questBoardRef is a true nil interface,
		// not a typed nil wrapped in an interface.
		config := DefaultConfig()
		c := &Component{
			config:        &config,
			partyCoord:    pc,
			logger:        slog.Default(),
			dagCache:      make(map[string]*DAGExecutionState),
			dagBySubQuest: make(map[string]*DAGExecutionState),
			questCache:    make(map[string]string),
		}

		dag := makeFullDAGState("exec-r2", "parent-r2", "party-r2", []QuestNode{
			makeNode("n1", 0),
		})
		dag.CompletedNodes = []string{"n1"}

		// Should not panic even without questBoardRef.
		c.triggerRollup(context.Background(), dag)

		if len(pc.disbandCalls) != 1 {
			t.Errorf("DisbandParty called %d times, want 1 (even without questBoardRef)", len(pc.disbandCalls))
		}
	})

	t.Run("rollup with nil partyCoord — no panic", func(t *testing.T) {
		t.Parallel()

		qb := &mockQuestBoardRef{}
		// Use the config struct directly so partyCoord is a true nil interface.
		config := DefaultConfig()
		c := &Component{
			config:        &config,
			questBoardRef: qb,
			logger:        slog.Default(),
			dagCache:      make(map[string]*DAGExecutionState),
			dagBySubQuest: make(map[string]*DAGExecutionState),
			questCache:    make(map[string]string),
		}

		dag := makeFullDAGState("exec-r3", "parent-r3", "", []QuestNode{
			makeNode("n1", 0),
		})
		dag.CompletedNodes = []string{"n1"}

		// Should not panic even without partyCoord.
		c.triggerRollup(context.Background(), dag)

		if len(qb.submitCalls) != 1 {
			t.Errorf("SubmitResult called %d times, want 1", len(qb.submitCalls))
		}
	})
}

// =============================================================================
// buildLeadReviewSystemPrompt TESTS
// =============================================================================

func TestBuildLeadReviewSystemPrompt(t *testing.T) {
	t.Parallel()

	t.Run("includes objective, criteria, and output", func(t *testing.T) {
		t.Parallel()

		objective := "Implement the auth module"
		acceptance := []string{"All tests pass", "No secrets in code"}
		output := "Here is my implementation..."

		result := buildLeadReviewSystemPrompt(objective, acceptance, output)

		if !stringContains(result, objective) {
			t.Errorf("system prompt missing objective %q", objective)
		}
		if !stringContains(result, acceptance[0]) {
			t.Errorf("system prompt missing acceptance criterion %q", acceptance[0])
		}
		if !stringContains(result, output) {
			t.Errorf("system prompt missing member output")
		}
		if !stringContains(result, "review_sub_quest") {
			t.Errorf("system prompt missing tool name instruction")
		}
	})

	t.Run("empty output produces failure message", func(t *testing.T) {
		t.Parallel()

		result := buildLeadReviewSystemPrompt("Do something", nil, "")
		if !stringContains(result, "did not provide output") {
			t.Errorf("system prompt should note missing output, got: %q", result)
		}
	})

	t.Run("no acceptance criteria omits criteria section", func(t *testing.T) {
		t.Parallel()

		result := buildLeadReviewSystemPrompt("Objective", nil, "Some output")
		if stringContains(result, "Acceptance criteria:") {
			t.Errorf("system prompt should not include acceptance criteria section when none provided")
		}
	})
}

// =============================================================================
// createReviewEntity TESTS
// =============================================================================

func TestCreateReviewEntity(t *testing.T) {
	t.Parallel()

	// createReviewEntity with a nil graph client must not panic and must
	// silently return — the nil-guard is the first line of the function.
	t.Run("nil graph — no panic", func(t *testing.T) {
		t.Parallel()

		c := newTestComponent(nil, nil)
		// c.graph is nil (not set in newTestComponent)
		dag := makeFullDAGState("exec-pr1", "parent-pr1", "party-pr1", []QuestNode{
			makeNode("n1", 0),
		})
		dag.NodeAssignees["n1"] = "agent-member"

		// Should not panic.
		c.createReviewEntity(context.Background(), dag, "n1")
	})

	t.Run("missing member ID — skips creation", func(t *testing.T) {
		t.Parallel()

		c := newTestComponent(nil, nil)
		dag := makeFullDAGState("exec-pr2", "parent-pr2", "party-pr2", []QuestNode{
			makeNode("n1", 0),
		})
		// No assignee set — memberID will be empty.
		// graph is also nil so even if we reached emit it would return.
		c.createReviewEntity(context.Background(), dag, "n1")
		// Reaching here without panic is the assertion.
	})

	t.Run("missing leader ID — skips creation", func(t *testing.T) {
		t.Parallel()

		c := newTestComponent(nil, nil)
		// partyCoord is nil so findLeadAgentID returns "".
		dag := makeFullDAGState("exec-pr3", "parent-pr3", "party-pr3", []QuestNode{
			makeNode("n1", 0),
		})
		dag.NodeAssignees["n1"] = "agent-member"

		c.createReviewEntity(context.Background(), dag, "n1")
		// Reaching here without panic is the assertion.
	})

	// Verify that handleNodeFailed invokes createReviewEntity at retry==0 by
	// asserting the entire failure path does not panic when graph is nil and
	// that escalation still fires (the primary observable side-effect).
	t.Run("handleNodeFailed at zero retries calls createReviewEntity path without panic", func(t *testing.T) {
		t.Parallel()

		qb := &mockQuestBoardRef{}
		c := newTestComponent(qb, nil)

		dag := makeFullDAGState("exec-pr4", "parent-pr4", "party-pr4", []QuestNode{
			makeNode("n1", 0),
		})
		dag.NodeStates["n1"] = NodeInProgress
		dag.NodeRetries["n1"] = 0
		dag.NodeAssignees["n1"] = "member-agent"

		// handleNodeFailed should escalate and also call createReviewEntity
		// (which no-ops with nil graph). Neither should panic.
		c.handleNodeFailed(context.Background(), dag, "n1")

		if dag.NodeStates["n1"] != NodeFailed {
			t.Errorf("node state = %q, want %q", dag.NodeStates["n1"], NodeFailed)
		}
		if len(qb.escalateCalls) != 1 {
			t.Errorf("EscalateQuest called %d times, want 1", len(qb.escalateCalls))
		}
	})
}

// =============================================================================
// promoteReadyNodes TESTS
// =============================================================================

func TestPromoteReadyNodes(t *testing.T) {
	t.Parallel()

	c := newTestComponent(nil, nil)

	t.Run("nodes with all deps completed promoted to ready", func(t *testing.T) {
		t.Parallel()

		dag := &DAGExecutionState{
			DAG: QuestDAG{Nodes: []QuestNode{
				{ID: "n1", Objective: "Step 1"},
				{ID: "n2", Objective: "Step 2", DependsOn: []string{"n1"}},
				{ID: "n3", Objective: "Step 3", DependsOn: []string{"n1", "n2"}},
			}},
			NodeStates: map[string]string{
				"n1": NodeCompleted,
				"n2": NodePending,
				"n3": NodePending,
			},
		}

		c.promoteReadyNodes(dag)

		// n2's dep (n1) is completed — should be promoted to ready.
		if dag.NodeStates["n2"] != NodeReady {
			t.Errorf("n2 state = %q, want ready", dag.NodeStates["n2"])
		}
		// n3 depends on n2 which is still pending — stays pending.
		if dag.NodeStates["n3"] != NodePending {
			t.Errorf("n3 state = %q, want pending (n2 not yet completed)", dag.NodeStates["n3"])
		}
	})

	t.Run("no deps — immediately promoted to ready", func(t *testing.T) {
		t.Parallel()

		dag := &DAGExecutionState{
			DAG: QuestDAG{Nodes: []QuestNode{
				{ID: "n1", Objective: "Solo step"},
			}},
			NodeStates: map[string]string{"n1": NodePending},
		}

		c.promoteReadyNodes(dag)

		if dag.NodeStates["n1"] != NodeReady {
			t.Errorf("n1 state = %q, want ready (no deps)", dag.NodeStates["n1"])
		}
	})
}

// =============================================================================
// Event loop sequential correctness — replaces TestConcurrentNodeCompletions
// =============================================================================
// The old TestConcurrentNodeCompletions launched multiple goroutines each
// calling handleNodeCompleted while holding a per-DAG mutex. With the single-
// goroutine event loop there is no concurrent access to DAG state, so that
// specific test no longer makes sense.
//
// Instead we verify that sequential processing of N node completions via the
// event loop handlers produces the correct final state without data races.
// =============================================================================

// TestSequentialNodeCompletions verifies that calling handleNodeCompleted N times
// sequentially (as the event loop does) produces correct CompletedNodes and
// eventually triggers rollup exactly once.
func TestSequentialNodeCompletions(t *testing.T) {
	t.Parallel()

	const numNodes = 10

	nodes := make([]QuestNode, numNodes)
	for i := range nodes {
		nodes[i] = QuestNode{ID: fmt.Sprintf("n%d", i), Objective: fmt.Sprintf("Step %d", i)}
	}

	qb := &mockQuestBoardRef{}
	pc := &mockPartyCoordRef{
		parties: map[domain.PartyID]*partycoord.Party{
			"party-1": {Lead: "lead-1"},
		},
	}
	c := newTestComponent(qb, pc)

	dagState := makeFullDAGState("exec-seq", "parent-seq", "party-1", nodes)
	for _, n := range nodes {
		dagState.NodeStates[n.ID] = NodeInProgress
	}
	// Register in dagCache so indexDAGState-dependent paths can find it.
	c.dagCache[dagState.ExecutionID] = dagState

	ctx := context.Background()

	// Call handleNodeCompleted sequentially for each node — mimicking the
	// single-goroutine event loop processing N completion events.
	for i := 0; i < numNodes; i++ {
		c.handleNodeCompleted(ctx, dagState, fmt.Sprintf("n%d", i))
	}

	// Verify all nodes are completed.
	for _, n := range nodes {
		if dagState.NodeStates[n.ID] != NodeCompleted {
			t.Errorf("node %s state = %q, want %q", n.ID, dagState.NodeStates[n.ID], NodeCompleted)
		}
	}

	// Verify CompletedNodes has all entries.
	if len(dagState.CompletedNodes) != numNodes {
		t.Errorf("CompletedNodes length = %d, want %d", len(dagState.CompletedNodes), numNodes)
	}

	// Verify rollup was triggered exactly once (when the last node completed).
	if qb.SubmitCallCount() != 1 {
		t.Errorf("expected 1 rollup submit call, got %d", qb.SubmitCallCount())
	}
}

// =============================================================================
// dagStateFromQuest TESTS
// =============================================================================

// TestDagStateFromQuest verifies that dagStateFromQuest correctly reconstructs
// a DAGExecutionState from a domain.Quest with DAG fields set.
func TestDagStateFromQuest(t *testing.T) {
	t.Parallel()

	twoNodeDAG := QuestDAG{
		Nodes: []QuestNode{
			{ID: "n1", Objective: "task 1"},
			{ID: "n2", Objective: "task 2", DependsOn: []string{"n1"}},
		},
	}

	t.Run("fresh quest — seeds node states from DAG definition", func(t *testing.T) {
		t.Parallel()

		partyID := domain.PartyID("party-1")
		quest := &domain.Quest{
			ID:              domain.QuestID("parent-q-1"),
			PartyID:         &partyID,
			DAGExecutionID:  "exec-init-1",
			DAGDefinition:   twoNodeDAG,
			DAGNodeQuestIDs: map[string]string{"n1": "sq-1", "n2": "sq-2"},
			// NodeStates absent — dagStateFromQuest must seed from DAG.
		}

		state := dagStateFromQuest(quest, 2)
		if state == nil {
			t.Fatal("dagStateFromQuest returned nil")
		}
		if state.ExecutionID != "exec-init-1" {
			t.Errorf("ExecutionID = %q, want exec-init-1", state.ExecutionID)
		}
		if state.ParentQuestID != "parent-q-1" {
			t.Errorf("ParentQuestID = %q, want parent-q-1", state.ParentQuestID)
		}
		if state.PartyID != "party-1" {
			t.Errorf("PartyID = %q, want party-1", state.PartyID)
		}
		if len(state.NodeStates) != 2 {
			t.Fatalf("NodeStates length = %d, want 2", len(state.NodeStates))
		}
		// n1 has no deps → NodeReady; n2 depends on n1 → NodePending
		if state.NodeStates["n1"] != NodeReady {
			t.Errorf("n1 state = %q, want %q", state.NodeStates["n1"], NodeReady)
		}
		if state.NodeStates["n2"] != NodePending {
			t.Errorf("n2 state = %q, want %q", state.NodeStates["n2"], NodePending)
		}
		if len(state.NodeRetries) != 2 {
			t.Errorf("NodeRetries length = %d, want 2", len(state.NodeRetries))
		}
		if state.NodeRetries["n1"] != 2 || state.NodeRetries["n2"] != 2 {
			t.Errorf("NodeRetries = %v, want all 2", state.NodeRetries)
		}
	})

	t.Run("quest with existing node states — preserves them", func(t *testing.T) {
		t.Parallel()

		partyID := domain.PartyID("party-2")
		quest := &domain.Quest{
			ID:              domain.QuestID("parent-q-2"),
			PartyID:         &partyID,
			DAGExecutionID:  "exec-2",
			DAGDefinition:   twoNodeDAG,
			DAGNodeQuestIDs: map[string]string{"n1": "sq-1", "n2": "sq-2"},
			DAGNodeStates:   map[string]string{"n1": NodeCompleted, "n2": NodeInProgress},
			DAGNodeAssignees: map[string]string{"n2": "agent-x"},
			DAGCompletedNodes: []string{"n1"},
			DAGNodeRetries:  map[string]int{"n1": 1, "n2": 2},
		}

		state := dagStateFromQuest(quest, 3)
		if state == nil {
			t.Fatal("dagStateFromQuest returned nil")
		}
		if state.NodeStates["n1"] != NodeCompleted {
			t.Errorf("n1 state = %q, want %q", state.NodeStates["n1"], NodeCompleted)
		}
		if state.NodeStates["n2"] != NodeInProgress {
			t.Errorf("n2 state = %q, want %q", state.NodeStates["n2"], NodeInProgress)
		}
		if state.NodeAssignees["n2"] != "agent-x" {
			t.Errorf("n2 assignee = %q, want agent-x", state.NodeAssignees["n2"])
		}
		if len(state.CompletedNodes) != 1 || state.CompletedNodes[0] != "n1" {
			t.Errorf("CompletedNodes = %v, want [n1]", state.CompletedNodes)
		}
		if state.NodeRetries["n1"] != 1 {
			t.Errorf("n1 retries = %d, want 1", state.NodeRetries["n1"])
		}
	})

	t.Run("nil quest returns nil", func(t *testing.T) {
		t.Parallel()
		state := dagStateFromQuest(nil, 2)
		if state != nil {
			t.Error("expected nil for nil quest")
		}
	})

	t.Run("quest with empty DAGExecutionID returns nil", func(t *testing.T) {
		t.Parallel()
		quest := &domain.Quest{ID: "q1"}
		state := dagStateFromQuest(quest, 2)
		if state != nil {
			t.Error("expected nil when DAGExecutionID is empty")
		}
	})

	t.Run("diamond DAG — only root nodes seeded as ready", func(t *testing.T) {
		t.Parallel()

		diamondDAG := QuestDAG{
			Nodes: []QuestNode{
				{ID: "a", Objective: "first"},
				{ID: "b", Objective: "second", DependsOn: []string{"a"}},
				{ID: "c", Objective: "third", DependsOn: []string{"a"}},
				{ID: "d", Objective: "final", DependsOn: []string{"b", "c"}},
			},
		}
		partyID := domain.PartyID("party-3")
		quest := &domain.Quest{
			ID:             domain.QuestID("parent-q-3"),
			PartyID:        &partyID,
			DAGExecutionID: "exec-build-1",
			DAGDefinition:  diamondDAG,
			DAGNodeQuestIDs: map[string]string{
				"a": "sq-a", "b": "sq-b", "c": "sq-c", "d": "sq-d",
			},
		}

		state := dagStateFromQuest(quest, 3)
		if state == nil {
			t.Fatal("dagStateFromQuest returned nil")
		}
		if state.ExecutionID != "exec-build-1" {
			t.Errorf("ExecutionID = %q", state.ExecutionID)
		}

		// Node "a" has no deps → Ready; b,c,d depend on something → Pending
		if state.NodeStates["a"] != NodeReady {
			t.Errorf("node a state = %q, want %q", state.NodeStates["a"], NodeReady)
		}
		for _, id := range []string{"b", "c", "d"} {
			if state.NodeStates[id] != NodePending {
				t.Errorf("node %s state = %q, want %q", id, state.NodeStates[id], NodePending)
			}
		}

		// All retries set to 3
		for _, id := range []string{"a", "b", "c", "d"} {
			if state.NodeRetries[id] != 3 {
				t.Errorf("node %s retries = %d, want 3", id, state.NodeRetries[id])
			}
		}

		// Completed/Failed should be empty slices (not nil)
		if state.CompletedNodes == nil || len(state.CompletedNodes) != 0 {
			t.Errorf("CompletedNodes = %v, want empty slice", state.CompletedNodes)
		}
		if state.FailedNodes == nil || len(state.FailedNodes) != 0 {
			t.Errorf("FailedNodes = %v, want empty slice", state.FailedNodes)
		}

		// NodeAssignees should be initialized but empty
		if state.NodeAssignees == nil {
			t.Error("NodeAssignees is nil, want initialized empty map")
		}

		// NodeQuestIDs should be carried through
		if state.NodeQuestIDs["a"] != "sq-a" {
			t.Errorf("NodeQuestIDs[a] = %q, want sq-a", state.NodeQuestIDs["a"])
		}
	})
}

// TestAnyToStringMap verifies that anyToStringMap handles map[string]any
// after a JSON round-trip (the typical case from KV reconstruction).
func TestAnyToStringMap(t *testing.T) {
	t.Parallel()

	t.Run("already typed map[string]string", func(t *testing.T) {
		t.Parallel()
		in := map[string]string{"k1": "v1", "k2": "v2"}
		out := anyToStringMap(in)
		if out["k1"] != "v1" || out["k2"] != "v2" {
			t.Errorf("anyToStringMap = %v, want %v", out, in)
		}
	})

	t.Run("map[string]any with string values (JSON round-trip)", func(t *testing.T) {
		t.Parallel()
		in := map[string]any{"k1": "v1", "k2": "v2"}
		out := anyToStringMap(in)
		if out["k1"] != "v1" || out["k2"] != "v2" {
			t.Errorf("anyToStringMap = %v, want k1→v1, k2→v2", out)
		}
	})

	t.Run("nil returns nil", func(t *testing.T) {
		t.Parallel()
		if anyToStringMap(nil) != nil {
			t.Error("expected nil for nil input")
		}
	})
}

// TestAnyToIntMap verifies that anyToIntMap handles the float64 encoding
// that JSON uses for all numbers.
func TestAnyToIntMap(t *testing.T) {
	t.Parallel()

	t.Run("map[string]any with float64 values (JSON round-trip)", func(t *testing.T) {
		t.Parallel()
		in := map[string]any{"n1": float64(2), "n2": float64(0)}
		out := anyToIntMap(in)
		if out["n1"] != 2 || out["n2"] != 0 {
			t.Errorf("anyToIntMap = %v, want n1→2, n2→0", out)
		}
	})

	t.Run("nil returns nil", func(t *testing.T) {
		t.Parallel()
		if anyToIntMap(nil) != nil {
			t.Error("expected nil for nil input")
		}
	})
}

// TestAnyToStringSlice verifies that anyToStringSlice handles []any after
// JSON round-trip.
func TestAnyToStringSlice(t *testing.T) {
	t.Parallel()

	t.Run("[]any with string elements (JSON round-trip)", func(t *testing.T) {
		t.Parallel()
		in := []any{"n1", "n2", "n3"}
		out := anyToStringSlice(in)
		if len(out) != 3 || out[0] != "n1" || out[2] != "n3" {
			t.Errorf("anyToStringSlice = %v, want [n1 n2 n3]", out)
		}
	})

	t.Run("nil returns nil", func(t *testing.T) {
		t.Parallel()
		if anyToStringSlice(nil) != nil {
			t.Error("expected nil for nil input")
		}
	})
}

// mustMarshal is a test helper that marshals v to JSON or fails the test.
func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustMarshal: %v", err)
	}
	return data
}
