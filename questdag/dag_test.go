package questdag

import (
	"strings"
	"testing"
)

func TestDAGValidate(t *testing.T) {
	tests := []struct {
		name    string
		dag     DAG
		wantErr string // substring; "" means expect success
	}{
		{
			name:    "empty dag",
			dag:     DAG{},
			wantErr: "at least one node",
		},
		{
			name: "single node no deps",
			dag:  DAG{Nodes: []Node{{ID: "a", Objective: "do a"}}},
		},
		{
			name: "linear chain",
			dag: DAG{Nodes: []Node{
				{ID: "a", Objective: "do a"},
				{ID: "b", Objective: "do b", DependsOn: []string{"a"}},
				{ID: "c", Objective: "do c", DependsOn: []string{"b"}},
			}},
		},
		{
			name: "diamond",
			dag: DAG{Nodes: []Node{
				{ID: "a", Objective: "do a"},
				{ID: "b", Objective: "do b", DependsOn: []string{"a"}},
				{ID: "c", Objective: "do c", DependsOn: []string{"a"}},
				{ID: "d", Objective: "do d", DependsOn: []string{"b", "c"}},
			}},
		},
		{
			name: "empty objective",
			dag: DAG{Nodes: []Node{
				{ID: "a", Objective: ""},
			}},
			wantErr: "objective must not be empty",
		},
		{
			name: "duplicate id",
			dag: DAG{Nodes: []Node{
				{ID: "a", Objective: "do a"},
				{ID: "a", Objective: "do a again"},
			}},
			wantErr: "duplicate node ID",
		},
		{
			name: "self reference",
			dag: DAG{Nodes: []Node{
				{ID: "a", Objective: "do a", DependsOn: []string{"a"}},
			}},
			wantErr: "depends on itself",
		},
		{
			name: "unknown dependency",
			dag: DAG{Nodes: []Node{
				{ID: "a", Objective: "do a", DependsOn: []string{"ghost"}},
			}},
			wantErr: "unknown node",
		},
		{
			name: "two node cycle",
			dag: DAG{Nodes: []Node{
				{ID: "a", Objective: "do a", DependsOn: []string{"b"}},
				{ID: "b", Objective: "do b", DependsOn: []string{"a"}},
			}},
			wantErr: "cycle",
		},
		{
			name: "three node cycle",
			dag: DAG{Nodes: []Node{
				{ID: "a", Objective: "do a", DependsOn: []string{"c"}},
				{ID: "b", Objective: "do b", DependsOn: []string{"a"}},
				{ID: "c", Objective: "do c", DependsOn: []string{"b"}},
			}},
			wantErr: "cycle",
		},
		{
			name:    "exceeds max nodes",
			dag:     DAG{Nodes: makeNodes(MaxNodes + 1)},
			wantErr: "maximum node count",
		},
		{
			name: "at max nodes ok",
			dag:  DAG{Nodes: makeNodes(MaxNodes)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.dag.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// makeNodes returns n independent (dependency-free) nodes with unique IDs.
func makeNodes(n int) []Node {
	nodes := make([]Node, n)
	for i := range nodes {
		nodes[i] = Node{ID: nodeID(i), Objective: "work"}
	}
	return nodes
}

func nodeID(i int) string {
	return "n" + string(rune('0'+i%10)) + string(rune('a'+i/10))
}

func TestExtractDAG(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		wantOK    bool
		wantGoal  string
		wantNodes int
	}{
		{
			name:      "clean envelope",
			output:    `{"goal":"ship it","dag":{"nodes":[{"id":"a","objective":"do a"}]}}`,
			wantOK:    true,
			wantGoal:  "ship it",
			wantNodes: 1,
		},
		{
			name:      "wrapped in prose and fence",
			output:    "Here is my plan:\n```json\n{\"goal\":\"build\",\"dag\":{\"nodes\":[{\"id\":\"a\",\"objective\":\"do a\"},{\"id\":\"b\",\"objective\":\"do b\",\"depends_on\":[\"a\"]}]}}\n```\nLet me know.",
			wantOK:    true,
			wantGoal:  "build",
			wantNodes: 2,
		},
		{
			name:   "missing goal key",
			output: `{"dag":{"nodes":[{"id":"a","objective":"do a"}]}}`,
			wantOK: false,
		},
		{
			name:   "missing dag key",
			output: `{"goal":"x"}`,
			wantOK: false,
		},
		{
			name:   "not json",
			output: "I could not decompose this quest.",
			wantOK: false,
		},
		{
			name:   "empty",
			output: "",
			wantOK: false,
		},
		{
			name:   "structurally invalid dag rejected",
			output: `{"goal":"x","dag":{"nodes":[{"id":"a","objective":"do a","depends_on":["a"]}]}}`,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dag, goal, ok := ExtractDAG(tt.output)
			if ok != tt.wantOK {
				t.Fatalf("ExtractDAG() ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if goal != tt.wantGoal {
				t.Errorf("goal = %q, want %q", goal, tt.wantGoal)
			}
			if dag == nil || len(dag.Nodes) != tt.wantNodes {
				t.Fatalf("nodes = %v, want %d", dag, tt.wantNodes)
			}
		})
	}
}
