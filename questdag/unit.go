package questdag

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	graphingest "github.com/c360studio/semstreams/processor/graph-ingest"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/internal/semcompat"
)

// Framework gated-DAG marker predicates (processor/gated-dag config defaults).
// questdag seeds PredDependsOn edges at unit creation; the framework executor
// reads all of these authoritatively each evaluation to derive a unit's status.
// The dispatch handler (and reset/retry logic) writes the terminal/dirtied
// markers. These MUST match the gated-dag component's configured predicates.
const (
	// PredDependsOn is one edge per prerequisite; Object = the prerequisite
	// unit's entity ID. All prerequisites must be Done for a unit to dispatch.
	PredDependsOn = "gateddag.depends_on"
	// PredCompleted marks a unit's work done (presence-based; Object ignored).
	PredCompleted = "gateddag.completed"
	// PredFailed marks a unit's work failed terminally.
	PredFailed = "gateddag.failed"
	// PredDirtied resets a unit for re-dispatch (Dirtied precedence re-derives
	// it Ready over any stale terminal marker).
	PredDirtied = "gateddag.dirtied"
	// PredClaim is the executor's durable in-flight record (executor-owned).
	PredClaim = "gateddag.claim"
)

// questdag-owned provenance on a unit entity. The framework treats a unit as an
// opaque reference (it dispatches only the unit entity ID); these triples let
// the dispatch handler turn that reference into sub-quest work and let questdag
// re-derive the quest.dag.* projection from the unit set.
const (
	// PredUnitNodeID is the DAG node ID this unit represents.
	PredUnitNodeID = "dagunit.node_id"
	// PredUnitParentQuest is the parent party quest's entity ID.
	PredUnitParentQuest = "dagunit.parent_quest"
	// PredUnitExecutionID is the DAG execution ID (one fan-out per decomposition).
	PredUnitExecutionID = "dagunit.execution_id"
	// PredUnitSubQuest is the sub-quest entity ID this unit dispatches, when
	// the sub-quest has been posted.
	PredUnitSubQuest = "dagunit.sub_quest"
)

// seedTimeout bounds each unit-create request/reply through graph-ingest. A
// single KV upsert; anything slower is a stalled ingest, not a slow write.
const seedTimeout = 5 * time.Second

// UnitPrefix returns the entity-ID prefix scoping every DAG unit on a board —
// the value to pass as the gated-DAG executor's UnitEntityPrefix. All units
// across all concurrent DAGs share this prefix; per-DAG isolation comes from
// the depends_on edge set, which is DAG-local (a node depends only on its
// siblings), so one shared executor evaluates every DAG correctly.
func UnitPrefix(board *domain.BoardConfig) string {
	return board.TypePrefix(domain.EntityTypeDagUnit)
}

// SeedInput describes one party-DAG decomposition to seed as gated-DAG units.
type SeedInput struct {
	// DAG is the validated decomposition (SeedUnits re-validates defensively).
	DAG DAG
	// ParentQuestID is the parent party quest's 6-part entity ID.
	ParentQuestID string
	// ExecutionID identifies this fan-out (one per decomposition).
	ExecutionID string
	// NodeSubQuests maps a node ID to its already-posted sub-quest entity ID.
	// A node with no entry is seeded without a sub-quest reference (the
	// dispatch handler resolves it via the parent's node_quest_ids projection).
	NodeSubQuests map[string]string
}

// SeedResult reports the node→unit mapping SeedUnits created, for building the
// parent quest's quest.dag.* projection.
type SeedResult struct {
	// NodeUnits maps each DAG node ID to its minted unit entity ID.
	NodeUnits map[string]string
}

// SeedUnits mints one gated-DAG unit entity per node and creates it in the
// graph with its gateddag.depends_on edges (translated from node IDs to
// prerequisite unit IDs) plus questdag provenance. It returns the node→unit
// mapping. Unit IDs are minted for every node up front so an edge can reference
// a prerequisite that appears later in the node slice. The gated-DAG executor
// watching UnitPrefix then dispatches each unit exactly once, in dependency
// order, as prerequisites complete.
//
// SeedUnits creates entities via graph-ingest's create (upsert) mutation, so a
// prerequisite that a sibling's edge referenced-before-birth (materialized as a
// referential stub) is overwritten with its real triples regardless of node
// order; the executor excludes stubs from dispatch until then.
func SeedUnits(ctx context.Context, client *natsclient.Client, board *domain.BoardConfig, in SeedInput) (*SeedResult, error) {
	if err := in.DAG.Validate(); err != nil {
		return nil, fmt.Errorf("questdag: refusing to seed invalid DAG: %w", err)
	}
	if in.ParentQuestID == "" {
		return nil, fmt.Errorf("questdag: SeedInput.ParentQuestID is required")
	}
	if in.ExecutionID == "" {
		return nil, fmt.Errorf("questdag: SeedInput.ExecutionID is required")
	}

	// Mint every unit ID first so depends_on edges can be resolved.
	nodeUnits := make(map[string]string, len(in.DAG.Nodes))
	for _, node := range in.DAG.Nodes {
		nodeUnits[node.ID] = board.DagUnitEntityID(domain.GenerateInstance())
	}

	// Create each unit entity with provenance + edges.
	for _, node := range in.DAG.Nodes {
		unitID := nodeUnits[node.ID]
		triples := buildUnitTriples(unitID, node, nodeUnits, in)
		if err := createUnitEntity(ctx, client, unitID, triples); err != nil {
			return nil, fmt.Errorf("questdag: seed unit for node %q: %w", node.ID, err)
		}
	}

	return &SeedResult{NodeUnits: nodeUnits}, nil
}

func buildUnitTriples(unitID string, node Node, nodeUnits map[string]string, in SeedInput) []message.Triple {
	triples := []message.Triple{
		{Subject: unitID, Predicate: PredUnitNodeID, Object: node.ID, Confidence: 1.0},
		{Subject: unitID, Predicate: PredUnitParentQuest, Object: in.ParentQuestID, Confidence: 1.0, Datatype: semcompat.EntityReferenceDatatype},
		{Subject: unitID, Predicate: PredUnitExecutionID, Object: in.ExecutionID, Confidence: 1.0},
	}
	if sub := in.NodeSubQuests[node.ID]; sub != "" {
		triples = append(triples, message.Triple{
			Subject: unitID, Predicate: PredUnitSubQuest, Object: sub, Confidence: 1.0, Datatype: semcompat.EntityReferenceDatatype,
		})
	}
	for _, dep := range node.DependsOn {
		depUnit := nodeUnits[dep]
		triples = append(triples, message.Triple{
			Subject: unitID, Predicate: PredDependsOn, Object: depUnit, Confidence: 1.0, Datatype: semcompat.EntityReferenceDatatype,
		})
	}
	return triples
}

// createUnitEntity upserts a unit entity with its triples via graph-ingest's
// entity-create mutation (the same path graph-ingest's CreateEntity uses).
func createUnitEntity(ctx context.Context, client *natsclient.Client, id string, triples []message.Triple) error {
	body, err := json.Marshal(graph.CreateEntityRequest{
		Entity: &graph.EntityState{ID: id, Version: 1, Triples: triples},
	})
	if err != nil {
		return fmt.Errorf("marshal create request: %w", err)
	}
	if _, err := client.Request(ctx, graphingest.SubjectEntityCreate, body, seedTimeout); err != nil {
		return fmt.Errorf("create %s: %w", id, err)
	}
	return nil
}
