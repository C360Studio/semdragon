package questdag

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/c360studio/semstreams/pkg/ownership"
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
	PredDependsOn = semcompat.GatedDAGDependsOnPredicate
	// PredCompleted marks a unit's work done (presence-based; Object ignored).
	PredCompleted = semcompat.GatedDAGCompletedPredicate
	// PredFailed marks a unit's work failed terminally.
	PredFailed = semcompat.GatedDAGFailedPredicate
	// PredDirtied resets a unit for re-dispatch (Dirtied precedence re-derives
	// it Ready over any stale terminal marker).
	PredDirtied = semcompat.GatedDAGDirtiedPredicate
	// PredClaim is the executor's durable in-flight record (executor-owned).
	PredClaim = semcompat.GatedDAGClaimPredicate
)

// questdag-owned provenance on a unit entity. The framework treats a unit as an
// opaque reference (it dispatches only the unit entity ID); these triples let
// the dispatch handler turn that reference into sub-quest work and let questdag
// re-derive the quest.dag.* projection from the unit set.
const (
	// PredUnitNodeID is the DAG node ID this unit represents.
	PredUnitNodeID = "questdag.unit.node-id"
	// PredUnitParentQuest is the parent party quest's entity ID.
	PredUnitParentQuest = "questdag.unit.parent-quest"
	// PredUnitExecutionID is the DAG execution ID (one fan-out per decomposition).
	PredUnitExecutionID = "questdag.unit.execution-id"
	// PredUnitSubQuest is the sub-quest entity ID this unit dispatches, when
	// the sub-quest has been posted.
	PredUnitSubQuest = "questdag.unit.sub-quest"
	// PredUnitPlanDigest binds the activation latch to the immutable,
	// order-invariant seed manifest. Work units are verified through the exact
	// execution manifest and their own immutable provenance/dependency triples.
	PredUnitPlanDigest = "questdag.unit.plan-digest"
)

const (
	seedTimeout         = 5 * time.Second
	activationNodeID    = "activation"
	unitCompletionOwner = "questdag-completion"
	unitIDHexBytes      = 16
)

var (
	unitMessageType       = message.Type{Domain: "questdag", Category: "unit", Version: "v1"}
	errUnitSeedCorruption = errors.New("questdag unit seed corruption")
)

// unitCompletionWriter is the product-owned terminal-marker boundary required
// by the SemStreams gated-DAG consumer contract. SemStreams owns evaluation;
// questdag owns declaring that its activation latch is complete.
type unitCompletionWriter interface {
	Complete(ctx context.Context, unitID string) error
}

type unitCreateFunc func(context.Context, string, []message.Triple) error
type unitReadFunc func(context.Context, string) (*graph.EntityState, error)
type unitListFunc func(context.Context, string) ([]graph.EntityState, error)
type unitCompletionUpdateFunc func(
	context.Context,
	string,
	message.Triple,
	ownership.OwnerToken,
) error

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
	// NodeUnits maps each DAG node ID to its deterministic unit entity ID.
	NodeUnits map[string]string
	// ActivationUnitID is the deterministic self-held unit whose completion
	// releases the fully verified manifest.
	ActivationUnitID string
	// PlanDigest is the canonical manifest digest stamped on the activation
	// latch.
	PlanDigest string
}

type unitSeedPlan struct {
	Digest            string
	ActivationID      string
	ActivationTriples []message.Triple
	NodeUnits         map[string]string
	WorkUnits         []plannedUnit
}

type plannedUnit struct {
	NodeID  string
	ID      string
	Triples []message.Triple
}

// SeedUnits creates a deterministic, replay-safe gated-DAG manifest behind an
// activation latch. The completion writer is injected because terminal-marker
// ownership belongs to the SemStreams consumer, not to the framework executor.
func SeedUnits(
	ctx context.Context,
	client *natsclient.Client,
	completion unitCompletionWriter,
	board *domain.BoardConfig,
	in SeedInput,
) (*SeedResult, error) {
	if client == nil {
		return nil, fmt.Errorf("questdag: SeedUnits client is required")
	}
	if completion == nil {
		return nil, fmt.Errorf("questdag: SeedUnits completion writer is required")
	}
	return seedUnitsWithSeams(
		ctx,
		board,
		in,
		func(ctx context.Context, id string, triples []message.Triple) error {
			return createUnitEntity(ctx, client, id, triples)
		},
		func(ctx context.Context, id string) (*graph.EntityState, error) {
			return readUnitEntity(ctx, client, id)
		},
		func(ctx context.Context, executionID string) ([]graph.EntityState, error) {
			return listUnitsForExecution(ctx, client, board, executionID)
		},
		completion,
	)
}

func seedUnitsWithSeams(
	ctx context.Context,
	board *domain.BoardConfig,
	in SeedInput,
	create unitCreateFunc,
	read unitReadFunc,
	list unitListFunc,
	completion unitCompletionWriter,
) (*SeedResult, error) {
	plan, err := buildUnitSeedPlan(board, in)
	if err != nil {
		return nil, err
	}

	activation, err := createOrVerifyUnit(ctx, create, read, plan.ActivationID, plan.ActivationTriples)
	if err != nil {
		return nil, fmt.Errorf("questdag: activation latch: %w", err)
	}
	activated, err := activationState(activation, plan.ActivationID)
	if err != nil {
		return nil, err
	}

	if activated {
		if err := verifyManifest(ctx, read, list, in.ExecutionID, plan); err != nil {
			return nil, fmt.Errorf("questdag: activated manifest: %w", err)
		}
		return seedResult(plan), nil
	}

	for _, unit := range plan.WorkUnits {
		if _, err := createOrVerifyUnit(ctx, create, read, unit.ID, unit.Triples); err != nil {
			return nil, fmt.Errorf("questdag: seed unit for node %q: %w", unit.NodeID, err)
		}
	}

	// Every unit is read authoritatively before the one release commit.
	if err := verifyManifest(ctx, read, list, in.ExecutionID, plan); err != nil {
		return nil, err
	}

	// A concurrent replay may have released the latch while this caller verified
	// the manifest. Re-read before committing so ordinary sequential replay is a
	// no-op and concurrent activation converges safely.
	activation, err = read(ctx, plan.ActivationID)
	if err != nil {
		return nil, fmt.Errorf("questdag: read activation before release: %w", err)
	}
	if err := verifyImmutableUnit(activation, plan.ActivationID, plan.ActivationTriples); err != nil {
		return nil, err
	}
	activated, err = activationState(activation, plan.ActivationID)
	if err != nil {
		return nil, err
	}
	if !activated {
		if err := completion.Complete(ctx, plan.ActivationID); err != nil {
			return nil, fmt.Errorf("questdag: release activation: %w", err)
		}
	}

	activation, err = read(ctx, plan.ActivationID)
	if err != nil {
		return nil, fmt.Errorf("questdag: confirm activation: %w", err)
	}
	if err := verifyImmutableUnit(activation, plan.ActivationID, plan.ActivationTriples); err != nil {
		return nil, err
	}
	activated, err = activationState(activation, plan.ActivationID)
	if err != nil {
		return nil, err
	}
	if !activated {
		return nil, fmt.Errorf("%w: activation completion marker was not committed", errUnitSeedCorruption)
	}

	return seedResult(plan), nil
}

func seedResult(plan *unitSeedPlan) *SeedResult {
	nodeUnits := make(map[string]string, len(plan.NodeUnits))
	for nodeID, unitID := range plan.NodeUnits {
		nodeUnits[nodeID] = unitID
	}
	return &SeedResult{
		NodeUnits:        nodeUnits,
		ActivationUnitID: plan.ActivationID,
		PlanDigest:       plan.Digest,
	}
}

func buildUnitSeedPlan(board *domain.BoardConfig, in SeedInput) (*unitSeedPlan, error) {
	if board == nil {
		return nil, fmt.Errorf("questdag: board config is required")
	}
	if err := in.DAG.Validate(); err != nil {
		return nil, fmt.Errorf("questdag: refusing to seed invalid DAG: %w", err)
	}
	if in.ParentQuestID == "" {
		return nil, fmt.Errorf("questdag: SeedInput.ParentQuestID is required")
	}
	if in.ExecutionID == "" {
		return nil, fmt.Errorf("questdag: SeedInput.ExecutionID is required")
	}

	nodes := canonicalNodes(in.DAG.Nodes)
	canonicalDAG := DAG{Nodes: nodes}
	if err := canonicalDAG.Validate(); err != nil {
		return nil, fmt.Errorf("questdag: canonical DAG is invalid: %w", err)
	}
	digest, err := planDigest(board, in, nodes)
	if err != nil {
		return nil, err
	}

	nodeUnits := make(map[string]string, len(nodes))
	for _, node := range nodes {
		nodeUnits[node.ID] = deterministicUnitID(board, in.ExecutionID, "node", node.ID)
	}
	activationID := deterministicUnitID(board, in.ExecutionID, "activation", "")
	activationTriples := baseUnitTriples(
		activationID,
		activationNodeID,
		in.ParentQuestID,
		in.ExecutionID,
		digest,
		"",
	)
	activationTriples = append(activationTriples, referenceTriple(activationID, activationID))

	topological, err := topologicalNodes(nodes)
	if err != nil {
		return nil, err
	}
	work := make([]plannedUnit, 0, len(topological))
	for _, node := range topological {
		unitID := nodeUnits[node.ID]
		triples := baseUnitTriples(
			unitID,
			node.ID,
			in.ParentQuestID,
			in.ExecutionID,
			"",
			in.NodeSubQuests[node.ID],
		)
		// The activation latch is always the first dependency. Declared
		// prerequisites follow in canonical node-ID order.
		triples = append(triples, referenceTriple(unitID, activationID))
		for _, dependency := range node.DependsOn {
			triples = append(triples, referenceTriple(unitID, nodeUnits[dependency]))
		}
		work = append(work, plannedUnit{NodeID: node.ID, ID: unitID, Triples: triples})
	}

	return &unitSeedPlan{
		Digest:            digest,
		ActivationID:      activationID,
		ActivationTriples: activationTriples,
		NodeUnits:         nodeUnits,
		WorkUnits:         work,
	}, nil
}

func canonicalNodes(nodes []Node) []Node {
	result := make([]Node, len(nodes))
	for i, node := range nodes {
		result[i] = node
		result[i].DependsOn = sortedUnique(node.DependsOn)
		result[i].Skills = sortedUnique(node.Skills)
		result[i].Acceptance = sortedUnique(node.Acceptance)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func sortedUnique(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	if len(result) < 2 {
		return result
	}
	write := 1
	for read := 1; read < len(result); read++ {
		if result[read] == result[write-1] {
			continue
		}
		result[write] = result[read]
		write++
	}
	return result[:write]
}

func topologicalNodes(nodes []Node) ([]Node, error) {
	byID := make(map[string]Node, len(nodes))
	indegree := make(map[string]int, len(nodes))
	dependents := make(map[string][]string, len(nodes))
	for _, node := range nodes {
		byID[node.ID] = node
		indegree[node.ID] = len(node.DependsOn)
		for _, dependency := range node.DependsOn {
			dependents[dependency] = append(dependents[dependency], node.ID)
		}
	}
	var ready []string
	for id, degree := range indegree {
		if degree == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)
	result := make([]Node, 0, len(nodes))
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		result = append(result, byID[id])
		for _, dependent := range dependents[id] {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				ready = append(ready, dependent)
				sort.Strings(ready)
			}
		}
	}
	if len(result) != len(nodes) {
		return nil, fmt.Errorf("questdag: canonical DAG contains a cycle")
	}
	return result, nil
}

func planDigest(board *domain.BoardConfig, in SeedInput, nodes []Node) (string, error) {
	type digestNode struct {
		ID         string   `json:"id"`
		Objective  string   `json:"objective"`
		Skills     []string `json:"skills"`
		Difficulty int      `json:"difficulty"`
		Acceptance []string `json:"acceptance"`
		DependsOn  []string `json:"depends_on"`
		SubQuest   string   `json:"sub_quest"`
	}
	manifest := struct {
		Board         string       `json:"board"`
		ParentQuestID string       `json:"parent_quest_id"`
		ExecutionID   string       `json:"execution_id"`
		Nodes         []digestNode `json:"nodes"`
	}{
		Board:         fmt.Sprintf("%s.%s.game.%s", board.Org, board.Platform, board.Board),
		ParentQuestID: in.ParentQuestID,
		ExecutionID:   in.ExecutionID,
		Nodes:         make([]digestNode, 0, len(nodes)),
	}
	for _, node := range nodes {
		manifest.Nodes = append(manifest.Nodes, digestNode{
			ID: node.ID, Objective: node.Objective, Skills: node.Skills,
			Difficulty: node.Difficulty, Acceptance: node.Acceptance,
			DependsOn: node.DependsOn, SubQuest: in.NodeSubQuests[node.ID],
		})
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf("questdag: marshal canonical seed plan: %w", err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

func deterministicUnitID(board *domain.BoardConfig, executionID, kind, nodeID string) string {
	material := fmt.Sprintf(
		"%s\x00%s\x00%s\x00%s\x00%s\x00%s",
		board.Org,
		board.Platform,
		board.Board,
		executionID,
		kind,
		nodeID,
	)
	sum := sha256.Sum256([]byte(material))
	return board.DagUnitEntityID(fmt.Sprintf("%x", sum[:unitIDHexBytes]))
}

func baseUnitTriples(unitID, nodeID, parentQuestID, executionID, digest, subQuestID string) []message.Triple {
	triples := []message.Triple{
		{Subject: unitID, Predicate: PredUnitNodeID, Object: nodeID, Confidence: 1},
		{Subject: unitID, Predicate: PredUnitParentQuest, Object: parentQuestID, Confidence: 1, Datatype: semcompat.EntityReferenceDatatype},
		{Subject: unitID, Predicate: PredUnitExecutionID, Object: executionID, Confidence: 1},
	}
	if digest != "" {
		triples = append(triples, message.Triple{
			Subject: unitID, Predicate: PredUnitPlanDigest, Object: digest, Confidence: 1,
		})
	}
	if subQuestID != "" {
		triples = append(triples, message.Triple{
			Subject: unitID, Predicate: PredUnitSubQuest, Object: subQuestID,
			Confidence: 1, Datatype: semcompat.EntityReferenceDatatype,
		})
	}
	return triples
}

func referenceTriple(subject, object string) message.Triple {
	return message.Triple{
		Subject: subject, Predicate: PredDependsOn, Object: object,
		Confidence: 1, Datatype: semcompat.EntityReferenceDatatype,
	}
}

func createOrVerifyUnit(
	ctx context.Context,
	create unitCreateFunc,
	read unitReadFunc,
	id string,
	expected []message.Triple,
) (*graph.EntityState, error) {
	err := create(ctx, id, expected)
	if err != nil && !hasErrorCode(err, graph.ErrorCodeEntityExists) {
		return nil, err
	}
	entity, readErr := read(ctx, id)
	if readErr != nil {
		if err != nil && hasErrorCode(readErr, graph.ErrorCodeEntityNotFound) {
			return nil, fmt.Errorf(
				"%w: duplicate %s was not found on authoritative read-back: %w",
				errUnitSeedCorruption,
				id,
				readErr,
			)
		}
		return nil, fmt.Errorf("questdag: authoritative read-back for %s: %w", id, readErr)
	}
	if verifyErr := verifyImmutableUnit(entity, id, expected); verifyErr != nil {
		return nil, verifyErr
	}
	return entity, nil
}

func verifyManifest(
	ctx context.Context,
	read unitReadFunc,
	list unitListFunc,
	executionID string,
	plan *unitSeedPlan,
) error {
	activation, err := read(ctx, plan.ActivationID)
	if err != nil {
		if hasErrorCode(err, graph.ErrorCodeEntityNotFound) {
			return fmt.Errorf(
				"%w: activation manifest %s is missing: %w",
				errUnitSeedCorruption,
				plan.ActivationID,
				err,
			)
		}
		return fmt.Errorf("questdag: read activation manifest: %w", err)
	}
	if err := verifyImmutableUnit(activation, plan.ActivationID, plan.ActivationTriples); err != nil {
		return err
	}
	for _, unit := range plan.WorkUnits {
		entity, err := read(ctx, unit.ID)
		if err != nil {
			if hasErrorCode(err, graph.ErrorCodeEntityNotFound) {
				return fmt.Errorf(
					"%w: manifest unit %q (%s) is missing: %w",
					errUnitSeedCorruption,
					unit.NodeID,
					unit.ID,
					err,
				)
			}
			return fmt.Errorf("questdag: read manifest unit %q: %w", unit.NodeID, err)
		}
		if err := verifyImmutableUnit(entity, unit.ID, unit.Triples); err != nil {
			return err
		}
	}
	entities, err := list(ctx, executionID)
	if err != nil {
		return fmt.Errorf("questdag: list execution manifest: %w", err)
	}
	expectedIDs := make(map[string]struct{}, len(plan.WorkUnits)+1)
	expectedIDs[plan.ActivationID] = struct{}{}
	for _, unit := range plan.WorkUnits {
		expectedIDs[unit.ID] = struct{}{}
	}
	actualIDs := make(map[string]struct{}, len(entities))
	for i := range entities {
		entity := &entities[i]
		if entityHasExecution(entity, executionID) {
			actualIDs[entity.ID] = struct{}{}
		}
	}
	if len(actualIDs) != len(expectedIDs) {
		return fmt.Errorf(
			"%w: execution %q manifest cardinality = %d, want %d",
			errUnitSeedCorruption,
			executionID,
			len(actualIDs),
			len(expectedIDs),
		)
	}
	for id := range expectedIDs {
		if _, ok := actualIDs[id]; !ok {
			return fmt.Errorf("%w: execution %q manifest is missing %s", errUnitSeedCorruption, executionID, id)
		}
	}
	return nil
}

func entityHasExecution(entity *graph.EntityState, executionID string) bool {
	if entity == nil {
		return false
	}
	for _, triple := range entity.Triples {
		if triple.Predicate == PredUnitExecutionID && triple.Object == executionID {
			return true
		}
	}
	return false
}

func verifyImmutableUnit(entity *graph.EntityState, id string, expected []message.Triple) error {
	if entity == nil || entity.ID != id {
		return fmt.Errorf("%w: unit %s missing or has wrong identity", errUnitSeedCorruption, id)
	}
	if entity.IsStub() || !entity.MessageType.Equal(unitMessageType) {
		return fmt.Errorf(
			"%w: unit %s is not a real non-stub %s entity",
			errUnitSeedCorruption,
			id,
			unitMessageType.Key(),
		)
	}
	want := immutableTripleKeys(expected)
	got := immutableTripleKeys(entity.Triples)
	if len(got) != len(want) {
		return fmt.Errorf("%w: unit %s immutable triple cardinality mismatch", errUnitSeedCorruption, id)
	}
	for i := range want {
		if want[i] != got[i] {
			return fmt.Errorf("%w: unit %s immutable contract mismatch", errUnitSeedCorruption, id)
		}
	}
	return nil
}

func immutableTripleKeys(triples []message.Triple) []string {
	immutable := map[string]struct{}{
		PredUnitNodeID: {}, PredUnitParentQuest: {}, PredUnitExecutionID: {},
		PredUnitSubQuest: {}, PredUnitPlanDigest: {}, PredDependsOn: {},
	}
	result := make([]string, 0, len(triples))
	for _, triple := range triples {
		if _, ok := immutable[triple.Predicate]; !ok {
			continue
		}
		object, err := json.Marshal(triple.Object)
		if err != nil {
			object = []byte(fmt.Sprintf("%#v", triple.Object))
		}
		result = append(
			result,
			triple.Subject+"\x00"+triple.Predicate+"\x00"+string(object)+"\x00"+triple.Datatype,
		)
	}
	sort.Strings(result)
	return result
}

func activationState(entity *graph.EntityState, expectedID string) (bool, error) {
	if entity == nil || entity.ID != expectedID || entity.IsStub() || !entity.MessageType.Equal(unitMessageType) {
		return false, fmt.Errorf("%w: activation is missing, malformed, or a stub", errUnitSeedCorruption)
	}
	completed := 0
	for _, triple := range entity.Triples {
		switch triple.Predicate {
		case PredCompleted:
			completed++
		case PredFailed, PredDirtied:
			return false, fmt.Errorf("%w: activation carries conflicting terminal/reset marker %s", errUnitSeedCorruption, triple.Predicate)
		}
	}
	if completed > 1 {
		return false, fmt.Errorf("%w: activation carries %d completion markers", errUnitSeedCorruption, completed)
	}
	return completed == 1, nil
}

func hasErrorCode(err error, code string) bool {
	var classified *errs.ClassifiedError
	return errors.As(err, &classified) && classified.Code == code
}

// createUnitEntity atomically births one complete real unit. Duplicate IDs are
// classified by graph-ingest and reconciled through authoritative read-back by
// createOrVerifyUnit.
func createUnitEntity(ctx context.Context, client *natsclient.Client, id string, triples []message.Triple) error {
	body, err := json.Marshal(graph.CreateEntityWithTriplesRequest{
		Entity: &graph.EntityState{
			ID: id, Version: 1, MessageType: unitMessageType, Triples: triples,
		},
		Triples: triples,
	})
	if err != nil {
		return fmt.Errorf("marshal create request: %w", err)
	}
	if _, err := client.RequestWithRetryClassified(
		ctx,
		graphingest.SubjectEntityCreateWithTriples,
		body,
		seedTimeout,
		natsclient.DefaultRetryConfig(),
	); err != nil {
		return fmt.Errorf("create %s: %w", id, err)
	}
	return nil
}

func readUnitEntity(ctx context.Context, client *natsclient.Client, id string) (*graph.EntityState, error) {
	body, err := json.Marshal(struct {
		ID string `json:"id"`
	}{ID: id})
	if err != nil {
		return nil, fmt.Errorf("marshal entity read: %w", err)
	}
	data, err := client.RequestClassified(ctx, "graph.ingest.query.entity", body, seedTimeout)
	if err != nil {
		return nil, err
	}
	var entity graph.EntityState
	if err := json.Unmarshal(data, &entity); err != nil {
		return nil, fmt.Errorf("unmarshal entity %s: %w", id, err)
	}
	return &entity, nil
}

func listUnitsForExecution(
	ctx context.Context,
	client *natsclient.Client,
	board *domain.BoardConfig,
	executionID string,
) ([]graph.EntityState, error) {
	var (
		cursor   string
		entities []graph.EntityState
	)
	for {
		body, err := json.Marshal(graph.PrefixQueryRequest{
			Prefix: UnitPrefix(board),
			Limit:  graph.MaxPrefixQueryLimit,
			Cursor: cursor,
		})
		if err != nil {
			return nil, fmt.Errorf("marshal unit prefix query: %w", err)
		}
		data, err := client.RequestClassified(ctx, "graph.ingest.query.prefix", body, seedTimeout)
		if err != nil {
			return nil, err
		}
		var response graph.PrefixQueryResponse
		if err := json.Unmarshal(data, &response); err != nil {
			return nil, fmt.Errorf("unmarshal unit prefix response: %w", err)
		}
		for i := range response.Entities {
			if entityHasExecution(&response.Entities[i], executionID) {
				entities = append(entities, response.Entities[i])
			}
		}
		if response.NextCursor == "" {
			return entities, nil
		}
		if response.NextCursor == cursor {
			return nil, fmt.Errorf("questdag: unit prefix query returned a repeated cursor")
		}
		cursor = response.NextCursor
	}
}

type natsUnitCompletionWriter struct {
	ownerToken ownership.OwnerToken
	read       unitReadFunc
	update     unitCompletionUpdateFunc
}

func newNATSUnitCompletionWriter(
	client *natsclient.Client,
	ownerToken ownership.OwnerToken,
) (unitCompletionWriter, error) {
	if client == nil {
		return nil, fmt.Errorf("questdag: completion writer client is required")
	}
	if ownerToken.IsZero() {
		return nil, fmt.Errorf("questdag: %s owner token is required", unitCompletionOwner)
	}
	owner, _, ok := strings.Cut(ownerToken.Wire(), "#")
	if !ok || owner != unitCompletionOwner {
		return nil, fmt.Errorf("questdag: completion owner token must belong to %s", unitCompletionOwner)
	}
	return &natsUnitCompletionWriter{
		ownerToken: ownerToken,
		read: func(ctx context.Context, id string) (*graph.EntityState, error) {
			return readUnitEntity(ctx, client, id)
		},
		update: func(
			ctx context.Context,
			id string,
			marker message.Triple,
			token ownership.OwnerToken,
		) error {
			return updateUnitCompletion(ctx, client, id, marker, token)
		},
	}, nil
}

func (w *natsUnitCompletionWriter) Complete(ctx context.Context, unitID string) error {
	parsed, err := domain.ParseEntityID(unitID)
	if err != nil || parsed.Domain != "game" || parsed.Type != domain.EntityTypeDagUnit {
		return errs.ClassifiedCode(
			errs.ErrorInvalid,
			graph.ErrorCodeInvalidRequest,
			fmt.Errorf("questdag: completion target %q is not a dagunit entity", unitID),
		)
	}
	marker := message.Triple{
		Subject: unitID, Predicate: PredCompleted, Object: true,
		Source: unitCompletionOwner, Timestamp: time.Now(), Confidence: 1,
	}
	before, err := w.read(ctx, unitID)
	if err != nil {
		return fmt.Errorf("questdag: read activation before completion: %w", err)
	}
	activated, err := activationState(before, unitID)
	if err != nil {
		return err
	}
	if activated {
		return nil
	}

	requestErr := w.update(ctx, unitID, marker, w.ownerToken)
	if requestErr != nil && (hasErrorCode(requestErr, graph.ErrorCodeEntityNotFound) ||
		hasErrorCode(requestErr, graph.ErrorCodeInvalidRequest) ||
		hasErrorCode(requestErr, graph.ErrorCodeOwnerLeaseStale)) {
		return requestErr
	}

	// beta.135 cannot validate a decoded success echo structurally. Never trust
	// it: normal success, degraded/malformed success, and transport-ambiguous
	// failure all resolve only through an authoritative entity read. Classified
	// structural and owner failures above retain their original classification.
	entity, readErr := w.read(ctx, unitID)
	if readErr != nil {
		if requestErr != nil {
			return fmt.Errorf("%w: completion read-back failed: %w", requestErr, readErr)
		}
		return readErr
	}
	activated, err = activationState(entity, unitID)
	if err != nil {
		return err
	}
	if !activated {
		if requestErr != nil {
			return requestErr
		}
		return fmt.Errorf("%w: completion read-back lacks marker", errUnitSeedCorruption)
	}
	return nil
}

func updateUnitCompletion(
	ctx context.Context,
	client *natsclient.Client,
	unitID string,
	marker message.Triple,
	ownerToken ownership.OwnerToken,
) error {
	body, err := json.Marshal(graph.UpdateEntityWithTriplesRequest{
		Entity:        &graph.EntityState{ID: unitID},
		RemoveTriples: []string{PredCompleted},
		AddTriples:    []message.Triple{marker},
		OwnerToken:    ownerToken.Wire(),
	})
	if err != nil {
		return fmt.Errorf("marshal activation completion: %w", err)
	}
	_, err = client.RequestWithRetryClassified(
		ctx,
		graphingest.SubjectEntityUpdateWithTriples,
		body,
		seedTimeout,
		natsclient.DefaultRetryConfig(),
	)
	return err
}
