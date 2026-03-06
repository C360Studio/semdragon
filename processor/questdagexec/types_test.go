package questdagexec

import (
	"fmt"
	"strings"
	"testing"
)

// =============================================================================
// QuestDAG.Validate() tests
// =============================================================================

func TestQuestDAGValidate(t *testing.T) {
	t.Parallel()

	// node is a convenience builder for tests.
	node := func(id, objective string, deps ...string) QuestNode {
		return QuestNode{ID: id, Objective: objective, DependsOn: deps}
	}

	tests := []struct {
		name        string
		dag         QuestDAG
		wantErr     bool
		errContains string
	}{
		// -------------------------------------------------------------------------
		// Valid DAGs
		// -------------------------------------------------------------------------
		{
			name: "single node",
			dag: QuestDAG{Nodes: []QuestNode{
				node("a", "Do thing A"),
			}},
		},
		{
			name: "linear chain A→B→C",
			dag: QuestDAG{Nodes: []QuestNode{
				node("a", "Do A"),
				node("b", "Do B", "a"),
				node("c", "Do C", "b"),
			}},
		},
		{
			name: "diamond A→B, A→C, B→D, C→D",
			dag: QuestDAG{Nodes: []QuestNode{
				node("a", "Root"),
				node("b", "Left branch", "a"),
				node("c", "Right branch", "a"),
				node("d", "Join", "b", "c"),
			}},
		},
		{
			name: "node with skills, difficulty, acceptance",
			dag: QuestDAG{Nodes: []QuestNode{
				{
					ID:         "x",
					Objective:  "Implement auth",
					Skills:     []string{"code_generation"},
					Difficulty: 3,
					Acceptance: []string{"all tests pass"},
				},
			}},
		},
		{
			name: "exactly 20 nodes (max allowed)",
			dag:  dagWithNNodes(20),
		},

		// -------------------------------------------------------------------------
		// Invalid: structural bounds
		// -------------------------------------------------------------------------
		{
			name:        "empty dag",
			dag:         QuestDAG{},
			wantErr:     true,
			errContains: "at least one node",
		},
		{
			name:        "21 nodes exceeds max",
			dag:         dagWithNNodes(21),
			wantErr:     true,
			errContains: "exceeds maximum node count",
		},

		// -------------------------------------------------------------------------
		// Invalid: node ID problems
		// -------------------------------------------------------------------------
		{
			name: "duplicate node IDs",
			dag: QuestDAG{Nodes: []QuestNode{
				node("dup", "First"),
				node("dup", "Second"),
			}},
			wantErr:     true,
			errContains: `duplicate node ID "dup"`,
		},

		// -------------------------------------------------------------------------
		// Invalid: dependency reference problems
		// -------------------------------------------------------------------------
		{
			name: "unknown dependency reference",
			dag: QuestDAG{Nodes: []QuestNode{
				node("a", "Do A", "nonexistent"),
			}},
			wantErr:     true,
			errContains: "unknown node",
		},
		{
			name: "self-reference",
			dag: QuestDAG{Nodes: []QuestNode{
				node("a", "Do A", "a"),
			}},
			wantErr:     true,
			errContains: "depends on itself",
		},

		// -------------------------------------------------------------------------
		// Invalid: empty objective
		// -------------------------------------------------------------------------
		{
			name: "empty objective",
			dag: QuestDAG{Nodes: []QuestNode{
				{ID: "a", Objective: ""},
			}},
			wantErr:     true,
			errContains: "objective must not be empty",
		},

		// -------------------------------------------------------------------------
		// Invalid: cycles
		// -------------------------------------------------------------------------
		{
			name: "simple cycle A→B→A",
			dag: QuestDAG{Nodes: []QuestNode{
				node("a", "A", "b"),
				node("b", "B", "a"),
			}},
			wantErr:     true,
			errContains: "cycle detected",
		},
		{
			name: "indirect cycle A→B→C→A",
			dag: QuestDAG{Nodes: []QuestNode{
				node("a", "A", "c"),
				node("b", "B", "a"),
				node("c", "C", "b"),
			}},
			wantErr:     true,
			errContains: "cycle detected",
		},
		{
			name: "cycle not involving all nodes: D→E→D with unrelated F",
			dag: QuestDAG{Nodes: []QuestNode{
				node("d", "D", "e"),
				node("e", "E", "d"),
				node("f", "F"),
			}},
			wantErr:     true,
			errContains: "cycle detected",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.dag.Validate()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Validate() returned nil, want error containing %q", tc.errContains)
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Fatalf("Validate() error = %q, want it to contain %q", err.Error(), tc.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() returned unexpected error: %v", err)
			}
		})
	}
}

// dagWithNNodes builds a simple linear chain of n nodes for bound-testing.
// The chain has no branches: n1→n2→...→nn.
func dagWithNNodes(n int) QuestDAG {
	nodes := make([]QuestNode, n)
	for i := range nodes {
		id := fmt.Sprintf("n%d", i+1)
		nodes[i] = QuestNode{ID: id, Objective: "Node " + id}
		if i > 0 {
			nodes[i].DependsOn = []string{fmt.Sprintf("n%d", i)}
		}
	}
	return QuestDAG{Nodes: nodes}
}

// =============================================================================
// DAGReadyNodes tests
// =============================================================================

func TestDAGReadyNodes(t *testing.T) {
	t.Parallel()

	diamond := QuestDAG{Nodes: []QuestNode{
		{ID: "a", Objective: "Root"},
		{ID: "b", Objective: "Left", DependsOn: []string{"a"}},
		{ID: "c", Objective: "Right", DependsOn: []string{"a"}},
		{ID: "d", Objective: "Join", DependsOn: []string{"b", "c"}},
	}}

	chain := QuestDAG{Nodes: []QuestNode{
		{ID: "a", Objective: "Step 1"},
		{ID: "b", Objective: "Step 2", DependsOn: []string{"a"}},
		{ID: "c", Objective: "Step 3", DependsOn: []string{"b"}},
	}}

	tests := []struct {
		name       string
		dag        QuestDAG
		nodeStates map[string]string
		want       []string // sorted for determinism
	}{
		{
			name: "all pending no deps — all ready",
			dag: QuestDAG{Nodes: []QuestNode{
				{ID: "x", Objective: "X"},
				{ID: "y", Objective: "Y"},
			}},
			nodeStates: map[string]string{
				"x": NodePending,
				"y": NodePending,
			},
			want: []string{"x", "y"},
		},
		{
			name: "diamond: only root ready initially",
			dag:  diamond,
			nodeStates: map[string]string{
				"a": NodePending,
				"b": NodePending,
				"c": NodePending,
				"d": NodePending,
			},
			want: []string{"a"},
		},
		{
			name: "diamond: root completed, b and c become ready",
			dag:  diamond,
			nodeStates: map[string]string{
				"a": NodeCompleted,
				"b": NodePending,
				"c": NodePending,
				"d": NodePending,
			},
			want: []string{"b", "c"},
		},
		{
			name: "diamond: only one branch done, join not ready yet",
			dag:  diamond,
			nodeStates: map[string]string{
				"a": NodeCompleted,
				"b": NodeCompleted,
				"c": NodePending,
				"d": NodePending,
			},
			want: []string{"c"},
		},
		{
			name: "diamond: both branches done, join ready",
			dag:  diamond,
			nodeStates: map[string]string{
				"a": NodeCompleted,
				"b": NodeCompleted,
				"c": NodeCompleted,
				"d": NodePending,
			},
			want: []string{"d"},
		},
		{
			name: "linear chain: first step ready, rest blocked",
			dag:  chain,
			nodeStates: map[string]string{
				"a": NodePending,
				"b": NodePending,
				"c": NodePending,
			},
			want: []string{"a"},
		},
		{
			name: "linear chain: step a done, b ready, c still blocked",
			dag:  chain,
			nodeStates: map[string]string{
				"a": NodeCompleted,
				"b": NodePending,
				"c": NodePending,
			},
			want: []string{"b"},
		},
		{
			name: "node in_progress is not ready (not pending)",
			dag: QuestDAG{Nodes: []QuestNode{
				{ID: "a", Objective: "A"},
			}},
			nodeStates: map[string]string{
				"a": NodeInProgress,
			},
			want: []string{},
		},
		{
			name: "failed dep blocks downstream",
			dag: QuestDAG{Nodes: []QuestNode{
				{ID: "a", Objective: "A"},
				{ID: "b", Objective: "B", DependsOn: []string{"a"}},
			}},
			nodeStates: map[string]string{
				"a": NodeFailed,
				"b": NodePending,
			},
			want: []string{},
		},
		{
			name: "node missing from state map treated as not completed",
			dag: QuestDAG{Nodes: []QuestNode{
				{ID: "a", Objective: "A"},
				{ID: "b", Objective: "B", DependsOn: []string{"a"}},
			}},
			// "a" not in map — treated as empty string, not NodeCompleted
			nodeStates: map[string]string{
				"b": NodePending,
			},
			want: []string{},
		},
		{
			name: "empty state map — zero-dep nodes are ready",
			dag: QuestDAG{Nodes: []QuestNode{
				{ID: "a", Objective: "A"},
			}},
			nodeStates: map[string]string{
				"a": NodePending,
			},
			want: []string{"a"},
		},
		{
			name: "all nodes completed — nothing ready",
			dag:  diamond,
			nodeStates: map[string]string{
				"a": NodeCompleted,
				"b": NodeCompleted,
				"c": NodeCompleted,
				"d": NodeCompleted,
			},
			want: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := DAGReadyNodes(tc.dag, tc.nodeStates)
			// Convert to sets for order-independent comparison.
			if !stringSlicesEqualAsSet(got, tc.want) {
				t.Fatalf("DAGReadyNodes() = %v, want %v", got, tc.want)
			}
		})
	}
}

// stringSlicesEqualAsSet returns true if a and b contain the same elements
// (ignoring order). Both nil and empty slice are treated as equal.
func stringSlicesEqualAsSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[string]int, len(a))
	for _, s := range a {
		counts[s]++
	}
	for _, s := range b {
		counts[s]--
		if counts[s] < 0 {
			return false
		}
	}
	return true
}
