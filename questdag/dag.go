// Package questdag is the semdragons domain layer over the semstreams gated-DAG
// executor (processor/gated-dag). It extracts and validates a party lead's
// decomposition into a DAG, mints per-run unit entity IDs, and seeds unit
// entities carrying gateddag.depends_on edges so the framework executor
// dispatches sub-quests in dependency order, exactly once, with restart
// recovery and stall detection.
//
// It is the "means" half of the D5 framework/consumer boundary: the framework
// (processor/gated-dag) owns ordering, claim-before-dispatch, restart recovery,
// and stall detection; questdag owns DAG extraction/validation, unit minting +
// seeding, the dispatch-subject handler, the lead review gate, clarification
// routing, escalation policy, and rollup. It supersedes the hand-rolled event
// loop in processor/questdagexec (deleted once its callers are migrated).
package questdag

import "fmt"

// MaxNodes caps the number of nodes accepted in a single party quest DAG. Party
// quests are smaller collaborative efforts — 20 nodes is already a large party.
// A stricter bound than semspec's general 100 guards against resource
// exhaustion from malformed LLM-generated DAGs.
const MaxNodes = 20

// DAG is a directed acyclic graph of sub-quest nodes proposed by a party lead
// via the decompose_quest tool. questdag validates it, then mints one gated-DAG
// unit entity per node.
type DAG struct {
	Nodes []Node `json:"nodes"`
}

// Node is one sub-quest within a party DAG. Each node maps to one sub-quest
// entity (party-scoped, invisible to the public board) and one gated-DAG unit
// entity carrying its dependency edges.
type Node struct {
	// ID uniquely identifies this node within the DAG. It keys the
	// node→unit and node→sub-quest maps and is the Object of sibling
	// depends_on edges.
	ID string `json:"id"`

	// Objective is what the sub-quest must accomplish; becomes the sub-quest
	// Title and Description. Must be non-empty.
	Objective string `json:"objective"`

	// Skills lists the SkillTag values required to work the node. Used to
	// match the best party member at dispatch.
	Skills []string `json:"skills,omitempty"`

	// Difficulty is a 0-5 integer matching QuestDifficulty. Zero inherits the
	// parent quest's difficulty at sub-quest creation.
	Difficulty int `json:"difficulty,omitempty"`

	// Acceptance lists the criteria the lead evaluates at review.
	Acceptance []string `json:"acceptance,omitempty"`

	// DependsOn lists node IDs that must complete before this node is
	// dispatchable. Zero-length means immediately dispatchable.
	DependsOn []string `json:"depends_on,omitempty"`
}

// Validate checks the DAG for structural correctness (dag-execution spec:
// "DAG proposal validation"). It rejects: zero nodes; more than MaxNodes;
// duplicate node IDs; an empty Objective; a self-reference; a DependsOn entry
// referencing an unknown node; and any cycle. Every error names the offending
// node for debuggability. The cycle check is a three-color DFS ported from the
// proven questdagexec/semspec implementation.
func (d *DAG) Validate() error {
	if len(d.Nodes) == 0 {
		return fmt.Errorf("dag must contain at least one node")
	}
	if len(d.Nodes) > MaxNodes {
		return fmt.Errorf("dag exceeds maximum node count (%d > %d)", len(d.Nodes), MaxNodes)
	}

	// Index node IDs for O(1) membership + duplicate detection.
	nodeIndex := make(map[string]struct{}, len(d.Nodes))
	for _, n := range d.Nodes {
		if _, exists := nodeIndex[n.ID]; exists {
			return fmt.Errorf("duplicate node ID %q", n.ID)
		}
		nodeIndex[n.ID] = struct{}{}
	}

	// Validate each node's objective and dependency references.
	for _, n := range d.Nodes {
		if n.Objective == "" {
			return fmt.Errorf("node %q: objective must not be empty", n.ID)
		}
		for _, dep := range n.DependsOn {
			if dep == n.ID {
				return fmt.Errorf("node %q depends on itself", n.ID)
			}
			if _, exists := nodeIndex[dep]; !exists {
				return fmt.Errorf("node %q depends on unknown node %q", n.ID, dep)
			}
		}
	}

	// Cycle detection via recursive DFS with three-color marking:
	// white=unvisited, gray=in current path, black=done. A gray→gray edge is a cycle.
	adj := make(map[string][]string, len(d.Nodes))
	for _, n := range d.Nodes {
		adj[n.ID] = n.DependsOn
	}
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int, len(d.Nodes))
	var visit func(id string) error
	visit = func(id string) error {
		color[id] = gray
		for _, dep := range adj[id] {
			switch color[dep] {
			case gray:
				return fmt.Errorf("cycle detected: node %q and node %q are in a cycle", id, dep)
			case white:
				if err := visit(dep); err != nil {
					return err
				}
			}
			// black: already fully explored, no cycle through this path.
		}
		color[id] = black
		return nil
	}
	for _, n := range d.Nodes {
		if color[n.ID] == white {
			if err := visit(n.ID); err != nil {
				return err
			}
		}
	}

	return nil
}
