// Package questdagexec implements reactive DAG execution for party quests.
// When a lead agent decomposes a party quest into a DAG of sub-quests, this
// package provides the types, validation, and ready-node detection that drive
// execution. The processor component (Phase 3) watches sub-quest KV state and
// uses these primitives to dispatch work and detect terminal conditions.
//
// DAG types are ported from semspec/tools/decompose with semdragons adaptations:
//   - QuestNode replaces TaskNode: adds Skills, Difficulty, Acceptance fields
//   - Max nodes is 20 (party quests are smaller than semspec's 100-node DAGs)
//   - FileScope is not required (agents use sandbox directories managed by questtools)
//   - DAGExecutionState tracks node-to-subquest mapping and lead assignment
package questdagexec

import (
	"fmt"
	"time"
)

// =============================================================================
// NODE STATE CONSTANTS
// =============================================================================

// Node state constants describe the lifecycle of a DAG node within party
// quest execution. These map to sub-quest status transitions but are tracked
// separately in DAGExecutionState so the DAG executor can reason about
// dependency gates without re-parsing quest entity state on every event.
const (
	// NodePending means dependencies are not yet met; the sub-quest exists
	// but cannot be assigned to a party member.
	NodePending = "pending"

	// NodeReady means all dependencies are completed; the node is eligible
	// for assignment to a party member.
	NodeReady = "ready"

	// NodeAssigned means the lead has assigned this node to a party member
	// and the sub-quest has been claimed on their behalf.
	NodeAssigned = "assigned"

	// NodeInProgress means the party member is actively working on the
	// sub-quest (questbridge has dispatched it to the agentic loop).
	NodeInProgress = "in_progress"

	// NodePendingReview means the party member submitted output and the lead
	// must review it before the DAG can advance.
	NodePendingReview = "pending_review"

	// NodeCompleted means the lead accepted the output; this node's completion
	// may unblock downstream nodes.
	NodeCompleted = "completed"

	// NodeRejected means the lead rejected the output; the node transitions
	// back to NodeAssigned with corrective feedback injected into the member's
	// next dispatch prompt.
	NodeRejected = "rejected"

	// NodeFailed means the node exhausted retries or reassignment failed;
	// downstream nodes depending on this one are permanently blocked.
	NodeFailed = "failed"

	// NodeAwaitingClarification means the party member asked a clarifying question.
	// The lead receives the question via the AGENT stream and provides an answer.
	// The node transitions back to NodeAssigned after the lead responds.
	NodeAwaitingClarification = "awaiting_clarification"
)

// =============================================================================
// CLARIFICATION TYPES
// =============================================================================

// ClarificationExchange records one question-and-answer exchange between a
// party member and the party lead during sub-quest execution. The member asks
// a clarifying question (surfaced via the escalated quest's output field) and
// the lead answers via the answer_clarification tool.
//
// Exchanges are persisted in DAGExecutionState.NodeClarifications and injected
// into the member's prompt context when the sub-quest is retried after the
// lead answers, reducing repeated questions in subsequent attempts.
type ClarificationExchange struct {
	Question string    `json:"question"`
	Answer   string    `json:"answer"`
	AskedAt  time.Time `json:"asked_at"`
}

// =============================================================================
// DAG TYPES
// =============================================================================

// maxQuestDAGNodes caps the number of nodes accepted in a single party quest DAG.
// Party quests are smaller collaborative efforts — 20 nodes is already a large
// party. Semspec allows 100 nodes for general DAGs; we use a stricter bound here
// to prevent resource exhaustion from malformed LLM-generated DAGs.
const maxQuestDAGNodes = 20

// QuestDAG represents a directed acyclic graph of sub-quests within a party quest.
// The lead agent proposes the DAG via the decompose_quest tool; the tool validates
// it and the questdagexec processor drives execution reactively as deps resolve.
type QuestDAG struct {
	Nodes []QuestNode `json:"nodes"`
}

// QuestNode represents a single sub-quest within a party quest DAG.
// Each node corresponds to one quest entity posted to the questboard with the
// party's ID set, making it invisible to the public board and only claimable
// via lead assignment through ClaimAndStartForParty.
type QuestNode struct {
	// ID uniquely identifies this node within the DAG. Used as the key in
	// DAGExecutionState.NodeStates and NodeQuestIDs maps.
	ID string `json:"id"`

	// Objective is a human-readable description of what the sub-quest must
	// accomplish. Becomes the quest's Title and Description when posted.
	// Must be non-empty.
	Objective string `json:"objective"`

	// Skills lists the SkillTag values required to work on this sub-quest.
	// Used for recruiting party members and matching them to nodes.
	Skills []string `json:"skills,omitempty"`

	// Difficulty is a 0-5 integer matching QuestDifficulty constants.
	// Defaults to 0 (Trivial) if unset.
	Difficulty int `json:"difficulty,omitempty"`

	// Acceptance lists the criteria the lead will evaluate during review.
	// Surfaced to the member's prompt via the quest description and to the
	// lead's review prompt as the evaluation rubric.
	Acceptance []string `json:"acceptance,omitempty"`

	// DependsOn lists node IDs that must reach NodeCompleted state before
	// this node becomes NodeReady. Zero-length means immediately ready.
	DependsOn []string `json:"depends_on,omitempty"`
}

// Validate checks the DAG for structural correctness. It returns an error if:
//   - The DAG contains zero nodes
//   - The DAG exceeds maxQuestDAGNodes (20)
//   - Any node has a duplicate ID
//   - Any node's DependsOn references an unknown node ID
//   - Any node depends on itself (self-reference)
//   - The graph contains a cycle (detected via DFS three-color marking)
//   - Any node has an empty Objective
//
// All returned errors include the offending node ID for debuggability.
// The algorithm is ported directly from semspec/tools/decompose/types.go.
func (d *QuestDAG) Validate() error {
	if len(d.Nodes) == 0 {
		return fmt.Errorf("dag must contain at least one node")
	}
	if len(d.Nodes) > maxQuestDAGNodes {
		return fmt.Errorf("dag exceeds maximum node count (%d > %d)", len(d.Nodes), maxQuestDAGNodes)
	}

	// Build an index of node IDs for O(1) membership checks and duplicate detection.
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

	// Build an adjacency list for cycle detection.
	adj := make(map[string][]string, len(d.Nodes))
	for _, n := range d.Nodes {
		adj[n.ID] = n.DependsOn
	}

	// Detect cycles via recursive DFS with three-color marking:
	//   white (0) = unvisited, gray (1) = in current path, black (2) = done.
	// A back-edge from a gray node to another gray node indicates a cycle.
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
			// black: already fully explored, no cycle through this path
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

// =============================================================================
// READY-NODE DETECTION
// =============================================================================

// DAGReadyNodes returns the IDs of nodes that are in NodePending state and have
// all their dependencies in NodeCompleted state. Nodes with zero dependencies are
// immediately ready when their state is NodePending.
//
// This function is the authoritative gate for party sub-quest dispatch: the DAG
// executor only assigns sub-quests whose dependencies have all been accepted by
// the lead. It is ported from semspec/workflow/reactive/dag_execution.go.
func DAGReadyNodes(dag QuestDAG, nodeStates map[string]string) []string {
	ready := make([]string, 0)
	for _, node := range dag.Nodes {
		if nodeStates[node.ID] != NodePending {
			continue
		}
		allDepsComplete := true
		for _, dep := range node.DependsOn {
			if nodeStates[dep] != NodeCompleted {
				allDepsComplete = false
				break
			}
		}
		if allDepsComplete {
			ready = append(ready, node.ID)
		}
	}
	return ready
}

// =============================================================================
// DAG EXECUTION STATE
// =============================================================================

// DAGExecutionState is the in-memory representation of a party quest DAG
// execution. It is keyed by ExecutionID in the dagCache map and is the
// primary in-memory state machine for the questdagexec event loop.
//
// Persistent state is stored as quest.dag.* predicates on the parent quest
// entity in the graph (ENTITY_STATES KV bucket). questdagexec reads these
// predicates on startup and reconstructs DAGExecutionState via
// dagStateFromQuest. After each mutation the event loop calls persistDAGState
// which performs a CAS read-modify-write on the parent quest entity.
//
// NodeStates is the primary state machine: the processor reads it to determine
// which nodes to dispatch, which are pending review, and when rollup triggers.
// NodeQuestIDs and NodeAssignees allow the processor to correlate KV watch
// events on sub-quest entities back to their DAG node.
//
// Concurrency: All mutable fields are ONLY accessed from the event loop
// goroutine. No mutexes are needed — the single-writer design eliminates
// data races by construction. See ADR-003.
type DAGExecutionState struct {
	// ExecutionID uniquely identifies this DAG execution. Used as the KV key.
	ExecutionID string `json:"execution_id"`

	// ParentQuestID is the entity ID of the parent quest that was decomposed.
	// After all nodes complete, the processor submits a rollup result to this quest.
	ParentQuestID string `json:"parent_quest_id"`

	// PartyID is the entity ID of the party executing this DAG.
	// Used when calling partycoord.AssignTask and partycoord.DisbandParty.
	PartyID string `json:"party_id"`

	// DAG is the validated quest DAG proposed by the lead agent.
	DAG QuestDAG `json:"dag"`

	// NodeStates maps node ID to its current state constant (NodePending,
	// NodeReady, NodeAssigned, NodeInProgress, NodePendingReview,
	// NodeCompleted, NodeRejected, NodeFailed).
	NodeStates map[string]string `json:"node_states"`

	// NodeQuestIDs maps DAG node ID to the sub-quest entity ID posted on the
	// questboard. Allows the processor to correlate KV watch events back to
	// the DAG node when a sub-quest status changes.
	NodeQuestIDs map[string]string `json:"node_quest_ids"`

	// NodeAssignees maps DAG node ID to the agent entity ID assigned to it.
	// Set by the processor when the lead assigns a sub-quest to a party member.
	NodeAssignees map[string]string `json:"node_assignees"`

	// CompletedNodes is the ordered list of node IDs that have been accepted
	// by the lead (NodeCompleted). Used for rollup to collect sub-quest outputs
	// in the order they completed.
	CompletedNodes []string `json:"completed_nodes"`

	// FailedNodes is the list of node IDs that exhausted retries or could not
	// be reassigned. If non-empty when all other nodes complete, the DAG
	// fails rather than rolling up.
	FailedNodes []string `json:"failed_nodes"`

	// NodeRetries maps node ID to the number of retries remaining for that node.
	// Decremented each time the lead rejects the output and the node is reset to
	// NodeAssigned. When it reaches zero, the next rejection sets NodeFailed.
	NodeRetries map[string]int `json:"node_retries"`

	// NodeClarifications tracks clarification Q&A exchanges per node.
	// Appended when the lead answers a clarification; injected into the
	// member's prompt on the next dispatch so they have context for the retry.
	NodeClarifications map[string][]ClarificationExchange `json:"node_clarifications,omitempty"`
}
