package questdagexec

import (
	"context"
	"errors"
	"testing"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/partycoord"
)

// =============================================================================
// MOCK IMPLEMENTATIONS
// =============================================================================

// mockAgentLister implements AgentLister with a fixed list of agents.
type mockAgentLister struct {
	agents []*agentprogression.Agent
	err    error
}

func (m *mockAgentLister) ListIdleAgents(_ context.Context) ([]*agentprogression.Agent, error) {
	return m.agents, m.err
}

// mockPartyJoiner implements PartyJoiner and records join calls.
type mockPartyJoiner struct {
	calls []joinCall
	err   error
}

type joinCall struct {
	partyID domain.PartyID
	agentID domain.AgentID
	role    domain.PartyRole
}

func (m *mockPartyJoiner) JoinParty(_ context.Context, partyID domain.PartyID, agentID domain.AgentID, role domain.PartyRole) error {
	if m.err != nil {
		return m.err
	}
	m.calls = append(m.calls, joinCall{partyID, agentID, role})
	return nil
}

// mockPartyMemberLister implements PartyMemberLister with a pre-built party.
type mockPartyMemberLister struct {
	party *partycoord.Party
	found bool
}

func (m *mockPartyMemberLister) GetParty(_ domain.PartyID) (*partycoord.Party, bool) {
	return m.party, m.found
}

// mockTaskAssigner implements TaskAssigner and records assign calls.
type mockTaskAssigner struct {
	calls []assignCall
	err   error
}

type assignCall struct {
	partyID    domain.PartyID
	subQuestID domain.QuestID
	agentID    domain.AgentID
	rationale  string
}

func (m *mockTaskAssigner) AssignTask(_ context.Context, partyID domain.PartyID, subQuestID domain.QuestID, agentID domain.AgentID, rationale string) error {
	if m.err != nil {
		return m.err
	}
	m.calls = append(m.calls, assignCall{partyID, subQuestID, agentID, rationale})
	return nil
}

// mockQuestClaimer implements QuestClaimerAndStarter and records claim calls.
type mockQuestClaimer struct {
	calls []claimCall
	err   error
}

type claimCall struct {
	questID    domain.QuestID
	partyID    domain.PartyID
	assignedTo domain.AgentID
}

func (m *mockQuestClaimer) ClaimAndStartForParty(_ context.Context, questID domain.QuestID, partyID domain.PartyID, assignedTo domain.AgentID) error {
	if m.err != nil {
		return m.err
	}
	m.calls = append(m.calls, claimCall{questID, partyID, assignedTo})
	return nil
}

// =============================================================================
// HELPERS
// =============================================================================

// makeAgent constructs a test agent with the given ID, level, and skills.
func makeAgent(id string, level int, skills ...domain.SkillTag) *agentprogression.Agent {
	a := &agentprogression.Agent{
		ID:                 domain.AgentID(id),
		Level:              level,
		Status:             domain.AgentIdle,
		SkillProficiencies: make(map[domain.SkillTag]domain.SkillProficiency),
	}
	for _, s := range skills {
		a.SkillProficiencies[s] = domain.SkillProficiency{Level: domain.ProficiencyNovice}
	}
	return a
}

// makeNode constructs a test QuestNode with the given ID, difficulty, and skills.
func makeNode(id string, difficulty int, skills ...string) QuestNode {
	return QuestNode{
		ID:         id,
		Objective:  "Objective for " + id,
		Difficulty: difficulty,
		Skills:     skills,
	}
}

// makeDAGState builds a minimal DAGExecutionState for use in tests.
// Nodes start in NodeReady state (as if promoteReadyNodes already ran).
func makeDAGState(partyID string, nodes []QuestNode) *DAGExecutionState {
	nodeStates := make(map[string]string, len(nodes))
	nodeQuestIDs := make(map[string]string, len(nodes))
	nodeAssignees := make(map[string]string, len(nodes))

	for _, n := range nodes {
		nodeStates[n.ID] = NodeReady
		nodeQuestIDs[n.ID] = "quest-" + n.ID
	}

	return &DAGExecutionState{
		ExecutionID:  "exec-test",
		PartyID:      partyID,
		DAG:          QuestDAG{Nodes: nodes},
		NodeStates:   nodeStates,
		NodeQuestIDs: nodeQuestIDs,
		NodeAssignees: nodeAssignees,
	}
}

// =============================================================================
// RecruitMembers TESTS
// =============================================================================

func TestRecruitMembers(t *testing.T) {
	t.Parallel()

	const partyID = "party-abc"

	tests := []struct {
		name         string
		nodes        []QuestNode
		idleAgents   []*agentprogression.Agent
		listErr      error
		joinErr      error
		wantErr      bool
		errContains  string
		wantJoins    int
		wantAgentIDs []domain.AgentID // which agents should be joined (order not guaranteed)
	}{
		// -----------------------------------------------------------------
		// Happy path: 3 idle agents, 2 sub-quests
		// -----------------------------------------------------------------
		{
			name: "3 agents 2 nodes — 2 agents recruited with correct skill matching",
			nodes: []QuestNode{
				makeNode("n1", 0, string(domain.SkillCodeGen)),
				makeNode("n2", 0, string(domain.SkillAnalysis)),
			},
			idleAgents: []*agentprogression.Agent{
				makeAgent("agent-codegen", 1, domain.SkillCodeGen),
				makeAgent("agent-analysis", 1, domain.SkillAnalysis),
				makeAgent("agent-generic", 1), // no skills
			},
			wantJoins:    2,
			wantAgentIDs: []domain.AgentID{"agent-codegen", "agent-analysis"},
		},
		// -----------------------------------------------------------------
		// No idle agents at all
		// -----------------------------------------------------------------
		{
			name:        "no idle agents returns error",
			nodes:       []QuestNode{makeNode("n1", 0)},
			idleAgents:  []*agentprogression.Agent{},
			wantErr:     true,
			errContains: "no idle agents",
		},
		// -----------------------------------------------------------------
		// ListIdleAgents returns an infrastructure error
		// -----------------------------------------------------------------
		{
			name:        "list agents error propagates",
			nodes:       []QuestNode{makeNode("n1", 0)},
			listErr:     errors.New("NATS timeout"),
			wantErr:     true,
			errContains: "NATS timeout",
		},
		// -----------------------------------------------------------------
		// Not enough agents for all nodes
		// -----------------------------------------------------------------
		{
			name: "fewer agents than nodes — partial shortfall error",
			nodes: []QuestNode{
				makeNode("n1", 0),
				makeNode("n2", 0),
				makeNode("n3", 0),
			},
			idleAgents: []*agentprogression.Agent{
				makeAgent("agent-a", 1),
				makeAgent("agent-b", 1),
				// only 2 agents for 3 nodes
			},
			wantErr:     true,
			errContains: "1 node(s) could not be staffed",
		},
		// -----------------------------------------------------------------
		// Trust tier gate: agent level too low for node difficulty
		// -----------------------------------------------------------------
		{
			name: "agent tier too low for hard node — skipped in favour of eligible agent",
			nodes: []QuestNode{
				// DifficultyHard (3) requires TierExpert (level 11+)
				makeNode("n1", int(domain.DifficultyHard)),
			},
			idleAgents: []*agentprogression.Agent{
				makeAgent("agent-low", 1),   // TierApprentice — ineligible
				makeAgent("agent-high", 12), // TierExpert — eligible
			},
			wantJoins:    1,
			wantAgentIDs: []domain.AgentID{"agent-high"},
		},
		// -----------------------------------------------------------------
		// JoinParty propagates errors
		// -----------------------------------------------------------------
		{
			name:        "join party error propagates",
			nodes:       []QuestNode{makeNode("n1", 0)},
			idleAgents:  []*agentprogression.Agent{makeAgent("agent-a", 1)},
			joinErr:     errors.New("party not found"),
			wantErr:     true,
			errContains: "party not found",
		},
		// -----------------------------------------------------------------
		// Single node, single agent — exact match
		// -----------------------------------------------------------------
		{
			name: "single agent single node — recruited",
			nodes: []QuestNode{
				makeNode("n1", 0, string(domain.SkillResearch)),
			},
			idleAgents: []*agentprogression.Agent{
				makeAgent("agent-researcher", 1, domain.SkillResearch),
			},
			wantJoins:    1,
			wantAgentIDs: []domain.AgentID{"agent-researcher"},
		},
		// -----------------------------------------------------------------
		// Agents are not double-assigned across nodes
		// -----------------------------------------------------------------
		{
			name: "single agent cannot be assigned twice",
			nodes: []QuestNode{
				makeNode("n1", 0),
				makeNode("n2", 0),
			},
			idleAgents: []*agentprogression.Agent{
				makeAgent("agent-only", 1),
			},
			wantErr:     true,
			errContains: "1 node(s) could not be staffed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dagState := makeDAGState(partyID, tc.nodes)

			joiner := &mockPartyJoiner{err: tc.joinErr}
			deps := RecruitmentDeps{
				Agents: &mockAgentLister{
					agents: tc.idleAgents,
					err:    tc.listErr,
				},
				PartyJoins: joiner,
			}

			err := RecruitMembers(context.Background(), dagState, deps)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("RecruitMembers() returned nil, want error containing %q", tc.errContains)
				}
				if tc.errContains != "" {
					if !containsString(err.Error(), tc.errContains) {
						t.Fatalf("error = %q, want it to contain %q", err.Error(), tc.errContains)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("RecruitMembers() unexpected error: %v", err)
			}

			if len(joiner.calls) != tc.wantJoins {
				t.Fatalf("JoinParty called %d times, want %d", len(joiner.calls), tc.wantJoins)
			}

			// Verify each expected agent was joined exactly once.
			joinedSet := make(map[domain.AgentID]int, len(joiner.calls))
			for _, c := range joiner.calls {
				joinedSet[c.agentID]++
				if c.role != domain.RoleExecutor {
					t.Errorf("agent %s joined with role %q, want %q", c.agentID, c.role, domain.RoleExecutor)
				}
				if c.partyID != domain.PartyID(partyID) {
					t.Errorf("agent %s joined party %q, want %q", c.agentID, c.partyID, partyID)
				}
			}

			for _, wantID := range tc.wantAgentIDs {
				if joinedSet[wantID] != 1 {
					t.Errorf("agent %s: joined %d times, want 1 (joined set: %v)", wantID, joinedSet[wantID], joinedSet)
				}
			}
		})
	}
}

// TestRecruitMembersPrefersBestSkillMatch verifies that when multiple agents
// are available for a node, the one with more matching skills is preferred.
func TestRecruitMembersPrefersBestSkillMatch(t *testing.T) {
	t.Parallel()

	dagState := makeDAGState("party-skill", []QuestNode{
		makeNode("n1", 0, string(domain.SkillCodeGen), string(domain.SkillCodeReview)),
	})

	// agent-expert has both required skills; agent-partial has only one.
	agents := []*agentprogression.Agent{
		makeAgent("agent-partial", 1, domain.SkillCodeGen),
		makeAgent("agent-expert", 1, domain.SkillCodeGen, domain.SkillCodeReview),
	}

	joiner := &mockPartyJoiner{}
	deps := RecruitmentDeps{
		Agents:     &mockAgentLister{agents: agents},
		PartyJoins: joiner,
	}

	if err := RecruitMembers(context.Background(), dagState, deps); err != nil {
		t.Fatalf("RecruitMembers() unexpected error: %v", err)
	}

	if len(joiner.calls) != 1 {
		t.Fatalf("expected 1 join call, got %d", len(joiner.calls))
	}
	if joiner.calls[0].agentID != "agent-expert" {
		t.Errorf("best-fit agent = %q, want %q", joiner.calls[0].agentID, "agent-expert")
	}
}

// =============================================================================
// AssignReadyNodes TESTS
// =============================================================================

func TestAssignReadyNodes(t *testing.T) {
	t.Parallel()

	const partyID = "party-assign"

	// Two executor members covering different skills.
	executorA := partycoord.PartyMember{
		AgentID: "agent-alpha",
		Role:    domain.RoleExecutor,
		Skills:  []domain.SkillTag{domain.SkillCodeGen},
	}
	executorB := partycoord.PartyMember{
		AgentID: "agent-beta",
		Role:    domain.RoleExecutor,
		Skills:  []domain.SkillTag{domain.SkillAnalysis},
	}
	leadMember := partycoord.PartyMember{
		AgentID: "agent-lead",
		Role:    domain.RoleLead,
	}

	party := &partycoord.Party{
		ID:      domain.PartyID(partyID),
		Lead:    "agent-lead",
		Members: []partycoord.PartyMember{leadMember, executorA, executorB},
	}

	tests := []struct {
		name         string
		setupState   func() *DAGExecutionState
		party        *partycoord.Party
		partyFound   bool
		assignErr    error
		claimErr     error
		wantErr      bool
		errContains  string
		wantAssigns  int
		wantClaims   int
		checkState   func(t *testing.T, dagState *DAGExecutionState)
	}{
		// -----------------------------------------------------------------
		// Happy path: ready node assigned to best-fit member
		// -----------------------------------------------------------------
		{
			name: "one ready node assigned to skill-matched member",
			setupState: func() *DAGExecutionState {
				nodes := []QuestNode{
					makeNode("n1", 0, string(domain.SkillCodeGen)),
				}
				s := makeDAGState(partyID, nodes)
				// Node already in NodeReady (set by makeDAGState).
				return s
			},
			party:       party,
			partyFound:  true,
			wantAssigns: 1,
			wantClaims:  1,
			checkState: func(t *testing.T, s *DAGExecutionState) {
				t.Helper()
				if s.NodeStates["n1"] != NodeAssigned {
					t.Errorf("node n1 state = %q, want %q", s.NodeStates["n1"], NodeAssigned)
				}
				if s.NodeAssignees["n1"] != string(executorA.AgentID) {
					t.Errorf("node n1 assignee = %q, want %q", s.NodeAssignees["n1"], executorA.AgentID)
				}
			},
		},
		// -----------------------------------------------------------------
		// No ready nodes — nothing to assign
		// -----------------------------------------------------------------
		{
			name: "no ready nodes — no-op",
			setupState: func() *DAGExecutionState {
				// n1 is already assigned; n2 depends on n1 so it is blocked.
				// DAGReadyNodes only returns nodes in NodePending state whose
				// deps are all NodeCompleted — neither condition is met here.
				return &DAGExecutionState{
					ExecutionID: "exec-noop",
					PartyID:     partyID,
					DAG: QuestDAG{Nodes: []QuestNode{
						{ID: "n1", Objective: "Step 1"},
						{ID: "n2", Objective: "Step 2", DependsOn: []string{"n1"}},
					}},
					// n1 in_progress (not pending) — not returned by DAGReadyNodes.
					// n2 pending but blocked by n1 which is not completed.
					NodeStates:    map[string]string{"n1": NodeInProgress, "n2": NodePending},
					NodeQuestIDs:  map[string]string{"n1": "quest-n1", "n2": "quest-n2"},
					NodeAssignees: map[string]string{},
				}
			},
			party:       party,
			partyFound:  true,
			wantAssigns: 0,
			wantClaims:  0,
		},
		// -----------------------------------------------------------------
		// Party not found
		// -----------------------------------------------------------------
		{
			name: "party not found returns error",
			setupState: func() *DAGExecutionState {
				return makeDAGState(partyID, []QuestNode{makeNode("n1", 0)})
			},
			partyFound:  false,
			wantErr:     true,
			errContains: "party",
		},
		// -----------------------------------------------------------------
		// AssignTask error propagates
		// -----------------------------------------------------------------
		{
			name: "assign task error propagates",
			setupState: func() *DAGExecutionState {
				return makeDAGState(partyID, []QuestNode{makeNode("n1", 0)})
			},
			party:       party,
			partyFound:  true,
			assignErr:   errors.New("party not active"),
			wantErr:     true,
			errContains: "party not active",
		},
		// -----------------------------------------------------------------
		// ClaimAndStartForParty error propagates
		// -----------------------------------------------------------------
		{
			name: "claim quest error propagates",
			setupState: func() *DAGExecutionState {
				return makeDAGState(partyID, []QuestNode{makeNode("n1", 0)})
			},
			party:       party,
			partyFound:  true,
			claimErr:    errors.New("quest already claimed"),
			wantErr:     true,
			errContains: "quest already claimed",
		},
		// -----------------------------------------------------------------
		// Two ready nodes assigned without double-booking a member
		// -----------------------------------------------------------------
		{
			name: "two ready nodes assigned to different members",
			setupState: func() *DAGExecutionState {
				nodes := []QuestNode{
					makeNode("n1", 0, string(domain.SkillCodeGen)),
					makeNode("n2", 0, string(domain.SkillAnalysis)),
				}
				s := makeDAGState(partyID, nodes)
				// Both nodes are independently pending (no deps).
				return s
			},
			party:       party,
			partyFound:  true,
			wantAssigns: 2,
			wantClaims:  2,
			checkState: func(t *testing.T, s *DAGExecutionState) {
				t.Helper()
				if s.NodeStates["n1"] != NodeAssigned {
					t.Errorf("node n1 state = %q, want %q", s.NodeStates["n1"], NodeAssigned)
				}
				if s.NodeStates["n2"] != NodeAssigned {
					t.Errorf("node n2 state = %q, want %q", s.NodeStates["n2"], NodeAssigned)
				}
				// The two nodes must not be assigned to the same member.
				if s.NodeAssignees["n1"] == s.NodeAssignees["n2"] {
					t.Errorf("both nodes assigned to same member %q — expected different members", s.NodeAssignees["n1"])
				}
			},
		},
		// -----------------------------------------------------------------
		// Already-assigned members (from prior cycle) are excluded
		// -----------------------------------------------------------------
		{
			name: "already-assigned member excluded from new assignment",
			setupState: func() *DAGExecutionState {
				nodes := []QuestNode{
					makeNode("n1", 0),
					makeNode("n2", 0),
				}
				s := makeDAGState(partyID, nodes)
				// n1 is already assigned to executorA.
				s.NodeStates["n1"] = NodeAssigned
				s.NodeAssignees["n1"] = string(executorA.AgentID)
				// Only n2 is pending.
				return s
			},
			party:       party,
			partyFound:  true,
			wantAssigns: 1,
			wantClaims:  1,
			checkState: func(t *testing.T, s *DAGExecutionState) {
				t.Helper()
				if s.NodeStates["n2"] != NodeAssigned {
					t.Errorf("node n2 state = %q, want %q", s.NodeStates["n2"], NodeAssigned)
				}
				// executorA is already assigned to n1 — n2 must go to executorB.
				if s.NodeAssignees["n2"] == string(executorA.AgentID) {
					t.Errorf("node n2 was assigned to already-busy agent %q", executorA.AgentID)
				}
			},
		},
		// -----------------------------------------------------------------
		// Lead member is never assigned executor work
		// -----------------------------------------------------------------
		{
			name: "lead is not assigned executor work even when sole available member",
			setupState: func() *DAGExecutionState {
				return makeDAGState(partyID, []QuestNode{makeNode("n1", 0)})
			},
			party: &partycoord.Party{
				ID:      domain.PartyID(partyID),
				Lead:    "agent-lead",
				Members: []partycoord.PartyMember{leadMember}, // only the lead
			},
			partyFound:  true,
			wantErr:     true,
			errContains: "no available party member",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dagState := tc.setupState()

			tasker := &mockTaskAssigner{err: tc.assignErr}
			claimer := &mockQuestClaimer{err: tc.claimErr}
			deps := AssignmentDeps{
				Members:     &mockPartyMemberLister{party: tc.party, found: tc.partyFound},
				Tasks:       tasker,
				QuestClaims: claimer,
			}

			err := AssignReadyNodes(context.Background(), dagState, deps)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("AssignReadyNodes() returned nil, want error containing %q", tc.errContains)
				}
				if tc.errContains != "" && !containsString(err.Error(), tc.errContains) {
					t.Fatalf("error = %q, want it to contain %q", err.Error(), tc.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("AssignReadyNodes() unexpected error: %v", err)
			}

			if len(tasker.calls) != tc.wantAssigns {
				t.Fatalf("AssignTask called %d times, want %d", len(tasker.calls), tc.wantAssigns)
			}
			if len(claimer.calls) != tc.wantClaims {
				t.Fatalf("ClaimAndStartForParty called %d times, want %d", len(claimer.calls), tc.wantClaims)
			}

			if tc.checkState != nil {
				tc.checkState(t, dagState)
			}
		})
	}
}

// TestAssignReadyNodesSkillMismatchAssignsBestAvailable verifies that when no
// party member matches the node's required skill, the function still assigns
// the best available member rather than failing.
func TestAssignReadyNodesSkillMismatchAssignsBestAvailable(t *testing.T) {
	t.Parallel()

	const partyID = "party-mismatch"

	// Node requires planning skill; neither member has it.
	member1 := partycoord.PartyMember{
		AgentID: "agent-1",
		Role:    domain.RoleExecutor,
		Skills:  []domain.SkillTag{domain.SkillCodeGen},
	}
	member2 := partycoord.PartyMember{
		AgentID: "agent-2",
		Role:    domain.RoleExecutor,
		Skills:  []domain.SkillTag{domain.SkillAnalysis},
	}

	dagState := makeDAGState(partyID, []QuestNode{
		makeNode("n1", 0, string(domain.SkillPlanning)),
	})

	party := &partycoord.Party{
		ID:      domain.PartyID(partyID),
		Members: []partycoord.PartyMember{member1, member2},
	}

	tasker := &mockTaskAssigner{}
	claimer := &mockQuestClaimer{}
	deps := AssignmentDeps{
		Members:     &mockPartyMemberLister{party: party, found: true},
		Tasks:       tasker,
		QuestClaims: claimer,
	}

	if err := AssignReadyNodes(context.Background(), dagState, deps); err != nil {
		t.Fatalf("AssignReadyNodes() unexpected error: %v", err)
	}

	// An assignment must have been made despite no skill match.
	if len(tasker.calls) != 1 {
		t.Fatalf("AssignTask called %d times, want 1", len(tasker.calls))
	}
	if dagState.NodeStates["n1"] != NodeAssigned {
		t.Errorf("node n1 state = %q, want %q", dagState.NodeStates["n1"], NodeAssigned)
	}
	if dagState.NodeAssignees["n1"] == "" {
		t.Error("node n1 has no assignee")
	}
}

// =============================================================================
// UNIT TESTS FOR INTERNAL HELPERS
// =============================================================================

func TestScoreAgentForNode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		agent     *agentprogression.Agent
		node      QuestNode
		wantScore int
	}{
		{
			name:      "node no skills — any agent scores 1",
			agent:     makeAgent("a", 1),
			node:      makeNode("n", 0),
			wantScore: 1,
		},
		{
			name:      "no skill overlap — score 0",
			agent:     makeAgent("a", 1, domain.SkillCodeGen),
			node:      makeNode("n", 0, string(domain.SkillAnalysis)),
			wantScore: 0,
		},
		{
			name:      "partial overlap — score equals matched count",
			agent:     makeAgent("a", 1, domain.SkillCodeGen, domain.SkillAnalysis),
			node:      makeNode("n", 0, string(domain.SkillCodeGen), string(domain.SkillResearch)),
			wantScore: 1,
		},
		{
			name:      "full overlap — score equals node skill count",
			agent:     makeAgent("a", 1, domain.SkillCodeGen, domain.SkillAnalysis),
			node:      makeNode("n", 0, string(domain.SkillCodeGen), string(domain.SkillAnalysis)),
			wantScore: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := scoreAgentForNode(tc.agent, tc.node)
			if got != tc.wantScore {
				t.Errorf("scoreAgentForNode() = %d, want %d", got, tc.wantScore)
			}
		})
	}
}

func TestAgentMeetsTier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		level int
		diff  domain.QuestDifficulty
		want  bool
	}{
		{"trivial accepts apprentice", 1, domain.DifficultyTrivial, true},
		{"moderate rejects apprentice", 3, domain.DifficultyModerate, false},
		{"moderate accepts journeyman", 7, domain.DifficultyModerate, true},
		{"hard rejects journeyman", 8, domain.DifficultyHard, false},
		{"hard accepts expert", 12, domain.DifficultyHard, true},
		{"epic rejects expert", 14, domain.DifficultyEpic, false},
		{"epic accepts master", 17, domain.DifficultyEpic, true},
		{"legendary accepts grandmaster", 20, domain.DifficultyLegendary, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			agent := makeAgent("a", tc.level)
			node := makeNode("n", int(tc.diff))
			got := agentMeetsTier(agent, node)
			if got != tc.want {
				t.Errorf("agentMeetsTier(level=%d, diff=%d) = %v, want %v", tc.level, tc.diff, got, tc.want)
			}
		})
	}
}

// =============================================================================
// HELPERS
// =============================================================================

// containsString returns true if s contains substr.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || stringContains(s, substr))
}

// stringContains is a stdlib-free substring check used to avoid importing
// the strings package solely for testing convenience.
func stringContains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
