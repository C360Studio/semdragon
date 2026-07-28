//go:build integration

package questdag

// =============================================================================
// INTEGRATION TEST — SeedUnits drives the real gated-DAG executor (task 3.1)
// =============================================================================
// Proves that units seeded by SeedUnits are consumable by the semstreams
// gated-DAG executor: seed a diamond DAG (a -> {b,c} -> d), stand up graph-ingest
// + the real executor over the dagunit prefix, and assert the executor dispatches
// each unit exactly once in dependency order as prerequisites complete. This is
// the framework-side proof that questdag's unit minting, entity creation, and
// gateddag.unit.depends-on edge seeding are correct — the "means" half of the D5
// boundary meeting the framework "ordering" half.
//
// The test plays the consumer's work loop: on each dispatch it writes the
// gateddag.unit.completed marker on that unit (what a real dispatch handler does after
// the sub-quest's agentic loop finishes), which unlocks dependents.
//
// Requires Docker (NATS via testcontainers).
// =============================================================================

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadregistry"
	"github.com/c360studio/semstreams/pkg/lifecycle"
	"github.com/c360studio/semstreams/pkg/ownership"
	gateddagexec "github.com/c360studio/semstreams/processor/gated-dag"
	graphingest "github.com/c360studio/semstreams/processor/graph-ingest"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semdragons/domain"
)

func TestSeedUnits_DispatchesInDependencyOrder(t *testing.T) {
	ctx := context.Background()
	board := &domain.BoardConfig{Org: "test", Platform: "integration", Board: "dagseed"}

	tc := natsclient.NewTestClient(t,
		natsclient.WithKV(), natsclient.WithFileStorage(),
		natsclient.WithKVBuckets(graph.BucketEntityStates),
		natsclient.WithStreams(natsclient.TestStreamConfig{Name: "ENTITY", Subjects: []string{"entity.>"}}),
	)
	client := tc.Client

	// graph-ingest: persists unit entities + markers, serves the prefix query.
	startGraphIngest(t, ctx, client)

	// Lifecycle Manager: the executor requires it as the Watch re-eval/recovery
	// substrate even in markers-only mode (no FanOutInstanceID).
	mgr := lifecycle.NewManager(client, slog.Default())

	// Consumer: record each dispatched unit ID in arrival order and complete it
	// (the work-done marker that unlocks dependents). Synchronous marker write in
	// the callback, mirroring the framework's own fullstack test.
	var (
		mu         sync.Mutex
		dispatched []string
	)
	decodeUnit := newDispatchDecoder(t)
	const dispatchSubject = "gateddag.dispatch.questdagtest"
	sub, err := client.Subscribe(ctx, dispatchSubject, func(cbCtx context.Context, msg *nats.Msg) {
		unitID, ok := decodeUnit(msg.Data)
		if !ok {
			return
		}
		mu.Lock()
		dispatched = append(dispatched, unitID)
		mu.Unlock()
		addCompletedMarker(cbCtx, client, unitID)
	})
	if err != nil {
		t.Fatalf("subscribe to dispatch subject: %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	// The executor over the shared dagunit prefix.
	cfg := gateddagexec.Config{
		UnitEntityPrefix:     UnitPrefix(board),
		DispatchSubject:      dispatchSubject,
		BackstopInterval:     "1s",
		DispatchDedupeWindow: "1s", // must be >= backstop (ADR-070)
	}
	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal executor config: %v", err)
	}
	exec, err := gateddagexec.NewComponent(cfgJSON, component.Dependencies{NATSClient: client, LifecycleManager: mgr, Logger: slog.Default()})
	if err != nil {
		t.Fatalf("create gated-dag executor: %v", err)
	}
	if err := exec.Initialize(); err != nil {
		t.Fatalf("executor Initialize: %v", err)
	}
	if err := exec.Start(ctx); err != nil {
		t.Fatalf("executor Start: %v", err)
	}
	t.Cleanup(func() { _ = exec.Stop(5 * time.Second) })

	// Let the executor's watchers attach before seeding.
	time.Sleep(750 * time.Millisecond)

	// Diamond: a -> {b, c} -> d.
	dag := DAG{Nodes: []Node{
		{ID: "a", Objective: "root task"},
		{ID: "b", Objective: "left task", DependsOn: []string{"a"}},
		{ID: "c", Objective: "right task", DependsOn: []string{"a"}},
		{ID: "d", Objective: "join task", DependsOn: []string{"b", "c"}},
	}}
	seed, err := SeedUnits(ctx, client, newIntegrationCompletionWriter(t, client), board, SeedInput{
		DAG:           dag,
		ParentQuestID: board.QuestEntityID("parent1"),
		ExecutionID:   "exec-diamond-1",
		NodeSubQuests: map[string]string{
			"a": board.QuestEntityID("suba"),
			"b": board.QuestEntityID("subb"),
			"c": board.QuestEntityID("subc"),
			"d": board.QuestEntityID("subd"),
		},
	})
	if err != nil {
		t.Fatalf("SeedUnits: %v", err)
	}
	if len(seed.NodeUnits) != 4 {
		t.Fatalf("expected 4 node units, got %d", len(seed.NodeUnits))
	}

	// Invert the mapping so we can label dispatched unit IDs by node.
	unitToNode := make(map[string]string, len(seed.NodeUnits))
	for node, unit := range seed.NodeUnits {
		unitToNode[unit] = node
	}

	// Wait for all four units to dispatch.
	order := waitForDispatches(t, &mu, &dispatched, 4, 25*time.Second)

	// Label by node and assert exactly-once + dependency ordering.
	nodeOrder := make([]string, 0, len(order))
	seen := map[string]int{}
	for _, unitID := range order {
		node, ok := unitToNode[unitID]
		if !ok {
			t.Fatalf("dispatched an unknown unit %q (not one of the seeded units)", unitID)
		}
		seen[node]++
		nodeOrder = append(nodeOrder, node)
	}
	for _, node := range []string{"a", "b", "c", "d"} {
		if seen[node] != 1 {
			t.Errorf("node %q dispatched %d times, want exactly 1 (order: %v)", node, seen[node], nodeOrder)
		}
	}

	idx := map[string]int{}
	for i, node := range nodeOrder {
		idx[node] = i
	}
	// a before b and c; b and c before d. (b vs c order is unconstrained.)
	assertBefore(t, idx, nodeOrder, "a", "b")
	assertBefore(t, idx, nodeOrder, "a", "c")
	assertBefore(t, idx, nodeOrder, "b", "d")
	assertBefore(t, idx, nodeOrder, "c", "d")
}

func TestNATSUnitCompletionWriter_ReplacesAndConfirmsOneMarker(t *testing.T) {
	ctx := context.Background()
	board := &domain.BoardConfig{Org: "test", Platform: "integration", Board: "completion"}
	tc := natsclient.NewTestClient(t,
		natsclient.WithKV(), natsclient.WithFileStorage(),
		natsclient.WithKVBuckets(graph.BucketEntityStates),
		natsclient.WithStreams(natsclient.TestStreamConfig{Name: "ENTITY", Subjects: []string{"entity.>"}}),
	)
	client := tc.Client
	startGraphIngest(t, ctx, client)

	plan, err := buildUnitSeedPlan(board, SeedInput{
		DAG:           DAG{Nodes: []Node{{ID: "a", Objective: "work"}}},
		ParentQuestID: board.QuestEntityID("parent"),
		ExecutionID:   "completion-writer-test",
	})
	if err != nil {
		t.Fatalf("build unit seed plan: %v", err)
	}
	if err := createUnitEntity(ctx, client, plan.ActivationID, plan.ActivationTriples); err != nil {
		t.Fatalf("create activation: %v", err)
	}
	writer := newIntegrationCompletionWriter(t, client)
	if err := writer.Complete(ctx, plan.ActivationID); err != nil {
		t.Fatalf("first Complete: %v", err)
	}
	if err := writer.Complete(ctx, plan.ActivationID); err != nil {
		t.Fatalf("replayed Complete: %v", err)
	}

	entity, err := readUnitEntity(ctx, client, plan.ActivationID)
	if err != nil {
		t.Fatalf("read activation: %v", err)
	}
	completed := 0
	for _, triple := range entity.Triples {
		if triple.Predicate != frameworkGatedDAGDefaults().CompletedPredicate {
			continue
		}
		completed++
		if triple.Source != unitCompletionOwner {
			t.Errorf("completion source = %q, want %q", triple.Source, unitCompletionOwner)
		}
	}
	if completed != 1 {
		t.Errorf("completion marker count = %d, want 1", completed)
	}
	activated, err := activationState(entity, plan.ActivationID)
	if err != nil {
		t.Fatalf("activationState: %v", err)
	}
	if !activated {
		t.Error("activation was not confirmed complete")
	}
}

func TestSeedUnits_ExecutorSeesNoPartialManifestAtEveryCreateBoundary(t *testing.T) {
	ctx := context.Background()
	board := &domain.BoardConfig{Org: "test", Platform: "integration", Board: "partialseed"}
	tc := natsclient.NewTestClient(t,
		natsclient.WithKV(), natsclient.WithFileStorage(),
		natsclient.WithKVBuckets(graph.BucketEntityStates),
		natsclient.WithStreams(natsclient.TestStreamConfig{Name: "ENTITY", Subjects: []string{"entity.>"}}),
	)
	client := tc.Client
	startGraphIngest(t, ctx, client)
	recorder := startRecordingUnitExecutor(t, ctx, client, board)

	dag := DAG{Nodes: []Node{
		{ID: "a", Objective: "first"},
		{ID: "b", Objective: "second", DependsOn: []string{"a"}},
	}}
	for boundary := 1; boundary <= len(dag.Nodes)+1; boundary++ {
		t.Run(fmt.Sprintf("create-%d", boundary), func(t *testing.T) {
			in := SeedInput{
				DAG:           dag,
				ParentQuestID: board.QuestEntityID(fmt.Sprintf("parent-%d", boundary)),
				ExecutionID:   fmt.Sprintf("partial-boundary-%d", boundary),
			}
			plan, err := buildUnitSeedPlan(board, in)
			if err != nil {
				t.Fatalf("buildUnitSeedPlan: %v", err)
			}
			plannedIDs := make(map[string]struct{}, len(plan.WorkUnits))
			for _, unit := range plan.WorkUnits {
				plannedIDs[unit.ID] = struct{}{}
			}

			interrupted := errors.New("simulated lost create acknowledgement")
			createCalls := 0
			create := func(ctx context.Context, id string, triples []message.Triple) error {
				createCalls++
				if err := createUnitEntity(ctx, client, id, triples); err != nil {
					return err
				}
				if createCalls == boundary {
					return interrupted
				}
				return nil
			}
			completion := newIntegrationCompletionWriter(t, client)
			_, err = seedUnitsWithSeams(
				ctx,
				board,
				in,
				create,
				func(ctx context.Context, id string) (*graph.EntityState, error) {
					return readUnitEntity(ctx, client, id)
				},
				func(ctx context.Context, executionID string) ([]graph.EntityState, error) {
					return listUnitsForExecution(ctx, client, board, executionID)
				},
				completion,
			)
			if !errors.Is(err, interrupted) {
				t.Fatalf("partial seed error = %v, want interruption", err)
			}

			// The real executor is already watching. Give both its watcher and
			// backstop a chance to evaluate the partial manifest.
			time.Sleep(1500 * time.Millisecond)
			if got := recorder.count(plannedIDs); got != 0 {
				t.Fatalf("partial manifest dispatched %d work units before activation", got)
			}

			result, err := SeedUnits(ctx, client, completion, board, in)
			if err != nil {
				t.Fatalf("replay SeedUnits: %v", err)
			}
			if !reflect.DeepEqual(result.NodeUnits, plan.NodeUnits) {
				t.Errorf("replay IDs = %#v, want %#v", result.NodeUnits, plan.NodeUnits)
			}
			manifest, err := listUnitsForExecution(ctx, client, board, in.ExecutionID)
			if err != nil {
				t.Fatalf("list converged manifest: %v", err)
			}
			if got, want := len(manifest), len(plan.WorkUnits)+1; got != want {
				t.Fatalf("manifest cardinality = %d, want %d", got, want)
			}
			for i := range manifest {
				if manifest[i].IsStub() || !manifest[i].MessageType.Equal(unitMessageType) {
					t.Errorf("manifest entity %s is a stub or has wrong envelope", manifest[i].ID)
				}
			}
			recorder.waitFor(t, plannedIDs, len(plan.WorkUnits), 10*time.Second)
		})
	}
}

func TestSeedUnits_LiveActivatedManifestCorruption(t *testing.T) {
	ctx := context.Background()
	board := &domain.BoardConfig{Org: "test", Platform: "integration", Board: "manifest"}
	tc := natsclient.NewTestClient(t,
		natsclient.WithKV(), natsclient.WithFileStorage(),
		natsclient.WithKVBuckets(graph.BucketEntityStates),
		natsclient.WithStreams(natsclient.TestStreamConfig{Name: "ENTITY", Subjects: []string{"entity.>"}}),
	)
	client := tc.Client
	startGraphIngest(t, ctx, client)
	completion := newIntegrationCompletionWriter(t, client)

	newInput := func(name string) SeedInput {
		return SeedInput{
			DAG:           DAG{Nodes: []Node{{ID: "a", Objective: "work"}}},
			ParentQuestID: board.QuestEntityID("parent-" + name),
			ExecutionID:   "manifest-" + name,
		}
	}

	t.Run("missing", func(t *testing.T) {
		in := newInput("missing")
		result, err := SeedUnits(ctx, client, completion, board, in)
		if err != nil {
			t.Fatalf("initial seed: %v", err)
		}
		body, err := json.Marshal(graph.DeleteEntityRequest{EntityID: result.NodeUnits["a"]})
		if err != nil {
			t.Fatalf("marshal delete: %v", err)
		}
		if _, err := client.RequestWithRetryClassified(
			ctx,
			graphingest.SubjectEntityDelete,
			body,
			seedTimeout,
			natsclient.DefaultRetryConfig(),
		); err != nil {
			t.Fatalf("delete manifest unit: %v", err)
		}
		if _, err := SeedUnits(ctx, client, completion, board, in); !errors.Is(err, errUnitSeedCorruption) {
			t.Fatalf("missing replay error = %v, want corruption", err)
		}
	})

	t.Run("mismatch", func(t *testing.T) {
		in := newInput("mismatch")
		result, err := SeedUnits(ctx, client, completion, board, in)
		if err != nil {
			t.Fatalf("initial seed: %v", err)
		}
		unitID := result.NodeUnits["a"]
		body, err := json.Marshal(graph.UpdateEntityWithTriplesRequest{
			Entity: &graph.EntityState{ID: unitID},
			AddTriples: []message.Triple{{
				Subject: unitID, Predicate: PredUnitNodeID, Object: "collision", Confidence: 1,
			}},
		})
		if err != nil {
			t.Fatalf("marshal mismatch: %v", err)
		}
		if _, err := client.RequestWithRetryClassified(
			ctx,
			graphingest.SubjectEntityUpdateWithTriples,
			body,
			seedTimeout,
			natsclient.DefaultRetryConfig(),
		); err != nil {
			t.Fatalf("mutate manifest unit: %v", err)
		}
		if _, err := SeedUnits(ctx, client, completion, board, in); !errors.Is(err, errUnitSeedCorruption) {
			t.Fatalf("mismatch replay error = %v, want corruption", err)
		}
	})

	t.Run("unexpected", func(t *testing.T) {
		in := newInput("unexpected")
		result, err := SeedUnits(ctx, client, completion, board, in)
		if err != nil {
			t.Fatalf("initial seed: %v", err)
		}
		extraID := board.DagUnitEntityID("0123456789abcdef0123456789abcdef")
		extraTriples := baseUnitTriples(
			extraID,
			"unexpected",
			in.ParentQuestID,
			in.ExecutionID,
			"",
			"",
		)
		extraTriples = append(extraTriples, referenceTriple(extraID, result.ActivationUnitID))
		if err := createUnitEntity(ctx, client, extraID, extraTriples); err != nil {
			t.Fatalf("create unexpected manifest unit: %v", err)
		}
		if _, err := SeedUnits(ctx, client, completion, board, in); !errors.Is(err, errUnitSeedCorruption) {
			t.Fatalf("unexpected replay error = %v, want corruption", err)
		}
	})
}

// =============================================================================
// HELPERS
// =============================================================================

func startGraphIngest(t *testing.T, ctx context.Context, client *natsclient.Client) {
	t.Helper()
	js, err := client.JetStream()
	if err != nil {
		t.Fatalf("get JetStream for graph-ingest: %v", err)
	}
	if _, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     "ENTITY",
		Subjects: []string{"entity.>"},
	}); err != nil {
		t.Fatalf("ensure graph-ingest ENTITY stream: %v", err)
	}
	cfg, err := json.Marshal(graphingest.DefaultConfig())
	if err != nil {
		t.Fatalf("marshal graph-ingest config: %v", err)
	}
	gi, err := graphingest.CreateGraphIngest(cfg, component.Dependencies{NATSClient: client})
	if err != nil {
		t.Fatalf("create graph-ingest: %v", err)
	}
	lc, ok := gi.(component.LifecycleComponent)
	if !ok {
		t.Fatalf("graph-ingest %T is not a LifecycleComponent", gi)
	}
	if err := lc.Initialize(); err != nil {
		t.Fatalf("graph-ingest Initialize: %v", err)
	}
	if err := lc.Start(ctx); err != nil {
		t.Fatalf("graph-ingest Start: %v", err)
	}
	t.Cleanup(func() { _ = lc.Stop(5 * time.Second) })
	time.Sleep(500 * time.Millisecond) // let mutation handlers attach
}

func newIntegrationCompletionWriter(t *testing.T, client *natsclient.Client) unitCompletionWriter {
	t.Helper()
	writer, err := newNATSUnitCompletionWriter(
		client,
		ownership.ExpectedOwnerToken(unitCompletionOwner, "integration-test"),
	)
	if err != nil {
		t.Fatalf("create completion writer: %v", err)
	}
	return writer
}

type unitDispatchRecorder struct {
	mu  sync.Mutex
	ids []string
}

func (r *unitDispatchRecorder) record(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ids = append(r.ids, id)
}

func (r *unitDispatchRecorder) count(ids map[string]struct{}) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, id := range r.ids {
		if _, ok := ids[id]; ok {
			count++
		}
	}
	return count
}

func (r *unitDispatchRecorder) waitFor(
	t *testing.T,
	ids map[string]struct{},
	want int,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if r.count(ids) >= want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d planned dispatches; got %d", want, r.count(ids))
}

func startRecordingUnitExecutor(
	t *testing.T,
	ctx context.Context,
	client *natsclient.Client,
	board *domain.BoardConfig,
) *unitDispatchRecorder {
	t.Helper()
	recorder := &unitDispatchRecorder{}
	decodeUnit := newDispatchDecoder(t)
	dispatchSubject := "gateddag.dispatch." + board.Board
	sub, err := client.Subscribe(ctx, dispatchSubject, func(cbCtx context.Context, msg *nats.Msg) {
		unitID, ok := decodeUnit(msg.Data)
		if !ok {
			return
		}
		recorder.record(unitID)
		addCompletedMarker(cbCtx, client, unitID)
	})
	if err != nil {
		t.Fatalf("subscribe to dispatch subject: %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	cfg := gateddagexec.Config{
		UnitEntityPrefix:     UnitPrefix(board),
		DispatchSubject:      dispatchSubject,
		BackstopInterval:     "1s",
		DispatchDedupeWindow: "1s",
	}
	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal executor config: %v", err)
	}
	exec, err := gateddagexec.NewComponent(cfgJSON, component.Dependencies{
		NATSClient:       client,
		LifecycleManager: lifecycle.NewManager(client, slog.Default()),
		Logger:           slog.Default(),
	})
	if err != nil {
		t.Fatalf("create gated-dag executor: %v", err)
	}
	if err := exec.Initialize(); err != nil {
		t.Fatalf("executor Initialize: %v", err)
	}
	if err := exec.Start(ctx); err != nil {
		t.Fatalf("executor Start: %v", err)
	}
	t.Cleanup(func() { _ = exec.Stop(5 * time.Second) })
	time.Sleep(750 * time.Millisecond)
	return recorder
}

// newDispatchDecoder returns a decoder that pulls the unit ID out of a dispatch
// envelope, keyed only on the gated-DAG payload registry.
func newDispatchDecoder(t *testing.T) func([]byte) (string, bool) {
	t.Helper()
	reg := payloadregistry.New()
	if err := gateddagexec.RegisterPayloads(reg); err != nil {
		t.Fatalf("register gated-dag payloads: %v", err)
	}
	dec := message.NewDecoder(reg)
	return func(data []byte) (string, bool) {
		base, err := dec.Decode(data)
		if err != nil {
			return "", false
		}
		dm, ok := base.Payload().(*gateddagexec.DispatchMessage)
		if !ok {
			return "", false
		}
		return dm.UnitEntityID, true
	}
}

// addCompletedMarker writes the gateddag.unit.completed marker on a unit (the work-done
// signal a real dispatch handler emits after the sub-quest loop finishes).
func addCompletedMarker(ctx context.Context, client *natsclient.Client, unitID string) {
	body, err := json.Marshal(graph.AddTripleRequest{
		Triple: message.Triple{Subject: unitID, Predicate: frameworkGatedDAGDefaults().CompletedPredicate, Object: true, Confidence: 1.0},
	})
	if err != nil {
		return
	}
	_, _ = client.Request(ctx, graphingest.SubjectTripleAdd, body, 5*time.Second)
}

func waitForDispatches(t *testing.T, mu *sync.Mutex, dispatched *[]string, want int, timeout time.Duration) []string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(*dispatched)
		mu.Unlock()
		if n >= want {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	out := make([]string, len(*dispatched))
	copy(out, *dispatched)
	if len(out) < want {
		t.Fatalf("timed out: got %d/%d dispatches (%v)", len(out), want, out)
	}
	return out
}

func assertBefore(t *testing.T, idx map[string]int, order []string, earlier, later string) {
	t.Helper()
	if idx[earlier] >= idx[later] {
		t.Errorf("expected %q dispatched before %q, got order %v", earlier, later, order)
	}
}
