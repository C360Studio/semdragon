package questdag

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sync"
	"testing"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/c360studio/semstreams/pkg/ownership"
)

func TestBuildUnitSeedPlanIsDeterministicAndOrderInvariant(t *testing.T) {
	board := seedTestBoard()
	inA := seedTestInput([]Node{
		{ID: "c", Objective: "third", DependsOn: []string{"b", "a"}},
		{ID: "b", Objective: "second", Skills: []string{"review", "code"}, Acceptance: []string{"two", "one"}, DependsOn: []string{"a"}},
		{ID: "a", Objective: "first"},
	})
	inB := seedTestInput([]Node{
		{ID: "a", Objective: "first"},
		{ID: "c", Objective: "third", DependsOn: []string{"a", "b"}},
		{ID: "b", Objective: "second", Skills: []string{"code", "review"}, Acceptance: []string{"one", "two"}, DependsOn: []string{"a"}},
	})

	planA, err := buildUnitSeedPlan(board, inA)
	if err != nil {
		t.Fatalf("buildUnitSeedPlan(A): %v", err)
	}
	planB, err := buildUnitSeedPlan(board, inB)
	if err != nil {
		t.Fatalf("buildUnitSeedPlan(B): %v", err)
	}

	if planA.Digest != planB.Digest {
		t.Errorf("order changed plan digest: %q != %q", planA.Digest, planB.Digest)
	}
	if planA.ActivationID != planB.ActivationID {
		t.Errorf("order changed activation ID: %q != %q", planA.ActivationID, planB.ActivationID)
	}
	if !reflect.DeepEqual(planA.NodeUnits, planB.NodeUnits) {
		t.Errorf("order changed node IDs: %#v != %#v", planA.NodeUnits, planB.NodeUnits)
	}

	hexInstance := regexp.MustCompile(`^[0-9a-f]{32}$`)
	for _, id := range append([]string{planA.ActivationID}, mapValues(planA.NodeUnits)...) {
		// The currently compiled beta.135 prerequisite does not expose
		// graph.ValidateDecodedEntityState. This pins the six-part entity
		// structure plus the beta.158 validator-safe lowercase 128-bit hex
		// instance contract without vendoring the newer validator. Runtime
		// validator proof remains tracked with the beta.158 prerequisite work.
		if !domain.IsValidEntityID(id) {
			t.Errorf("planned unit ID %q is not a valid entity ID", id)
		}
		if instance := domain.ExtractInstance(id); !hexInstance.MatchString(instance) {
			t.Errorf("planned unit instance %q is not validator-safe 128-bit hex", instance)
		}
	}
}

func TestSeedUnitsWithSeamsCreatesAtomicUnitsBehindActivationLatch(t *testing.T) {
	board := seedTestBoard()
	in := seedTestInput([]Node{
		{ID: "b", Objective: "dependent", DependsOn: []string{"a"}},
		{ID: "a", Objective: "prerequisite"},
	})
	store := newFakeUnitSeedStore()

	result, err := seedUnitsWithSeams(
		context.Background(),
		board,
		in,
		store.create,
		store.read,
		store.list,
		store,
	)
	if err != nil {
		t.Fatalf("seedUnitsWithSeams(): %v", err)
	}
	plan, err := buildUnitSeedPlan(board, in)
	if err != nil {
		t.Fatalf("buildUnitSeedPlan(): %v", err)
	}

	wantCreateOrder := []string{
		plan.ActivationID,
		plan.NodeUnits["a"],
		plan.NodeUnits["b"],
	}
	if !reflect.DeepEqual(store.createOrder, wantCreateOrder) {
		t.Errorf("create order = %#v, want topological %#v", store.createOrder, wantCreateOrder)
	}
	if !reflect.DeepEqual(result.NodeUnits, plan.NodeUnits) {
		t.Errorf("result node units = %#v, want %#v", result.NodeUnits, plan.NodeUnits)
	}
	if result.ActivationUnitID != plan.ActivationID || result.PlanDigest != plan.Digest {
		t.Errorf("result activation/digest = %q/%q, want %q/%q", result.ActivationUnitID, result.PlanDigest, plan.ActivationID, plan.Digest)
	}

	activation := store.entities[plan.ActivationID]
	assertExactReferences(t, activation, frameworkGatedDAGDefaults().DependsOnPredicate, []string{plan.ActivationID})
	assertPredicateValue(t, activation, PredUnitPlanDigest, plan.Digest)

	prerequisite := store.entities[plan.NodeUnits["a"]]
	assertExactReferences(t, prerequisite, frameworkGatedDAGDefaults().DependsOnPredicate, []string{plan.ActivationID})
	dependent := store.entities[plan.NodeUnits["b"]]
	assertExactReferences(t, dependent, frameworkGatedDAGDefaults().DependsOnPredicate, []string{
		plan.ActivationID,
		plan.NodeUnits["a"],
	})
	for nodeID, unitID := range plan.NodeUnits {
		entity := store.entities[unitID]
		assertPredicateValue(t, entity, PredUnitNodeID, nodeID)
		if digest := entity.GetTriple(PredUnitPlanDigest); digest != nil {
			t.Errorf("work unit %q unexpectedly carries latch-only plan digest", unitID)
		}
		assertPredicateValue(t, entity, PredUnitExecutionID, in.ExecutionID)
		if got := entity.GetTriple(frameworkGatedDAGDefaults().CompletedPredicate); got != nil {
			t.Errorf("work unit %q was born completed", unitID)
		}
	}
	if store.completeCalls != 1 {
		t.Errorf("activation release commits = %d, want 1", store.completeCalls)
	}
	if activation.GetTriple(frameworkGatedDAGDefaults().CompletedPredicate) == nil {
		t.Error("activation completion marker was not confirmed")
	}
}

func TestSeedUnitsWithSeamsConvergesAfterEveryCreateInterruption(t *testing.T) {
	board := seedTestBoard()
	in := seedTestInput([]Node{
		{ID: "c", Objective: "third", DependsOn: []string{"b"}},
		{ID: "b", Objective: "second", DependsOn: []string{"a"}},
		{ID: "a", Objective: "first"},
	})
	plan, err := buildUnitSeedPlan(board, in)
	if err != nil {
		t.Fatalf("buildUnitSeedPlan(): %v", err)
	}
	wantEntities := len(plan.WorkUnits) + 1

	for boundary := 1; boundary <= wantEntities; boundary++ {
		t.Run(fmt.Sprintf("create-%d", boundary), func(t *testing.T) {
			store := newFakeUnitSeedStore()
			interrupted := errors.New("simulated lost create reply")
			store.failCreateAt = boundary
			store.failCreateErr = interrupted
			store.failCreateAfterPersist = true

			_, err := seedUnitsWithSeams(
				context.Background(), board, in,
				store.create, store.read, store.list, store,
			)
			if !errors.Is(err, interrupted) {
				t.Fatalf("interrupted seed error = %v, want transient cause", err)
			}
			if entity := store.entity(plan.ActivationID); entity != nil && entity.GetTriple(frameworkGatedDAGDefaults().CompletedPredicate) != nil {
				t.Fatal("partial manifest released activation")
			}

			store.clearCreateFailure()
			got, err := seedUnitsWithSeams(
				context.Background(), board, in,
				store.create, store.read, store.list, store,
			)
			if err != nil {
				t.Fatalf("replay after create %d: %v", boundary, err)
			}
			if !reflect.DeepEqual(got.NodeUnits, plan.NodeUnits) {
				t.Errorf("replay IDs = %#v, want %#v", got.NodeUnits, plan.NodeUnits)
			}
			if got := store.entityCount(); got != wantEntities {
				t.Errorf("entity count = %d, want %d (no duplicates)", got, wantEntities)
			}
			if got := store.completionCommits(); got != 1 {
				t.Errorf("release commits = %d, want 1", got)
			}
		})
	}
}

func TestSeedUnitsWithSeamsConcurrentSamePlanConverges(t *testing.T) {
	board := seedTestBoard()
	in := seedTestInput([]Node{
		{ID: "b", Objective: "second", DependsOn: []string{"a"}},
		{ID: "a", Objective: "first"},
	})
	plan, err := buildUnitSeedPlan(board, in)
	if err != nil {
		t.Fatalf("buildUnitSeedPlan(): %v", err)
	}
	store := newFakeUnitSeedStore()

	const callers = 8
	errsCh := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := seedUnitsWithSeams(
				context.Background(), board, in,
				store.create, store.read, store.list, store,
			)
			errsCh <- err
		}()
	}
	wg.Wait()
	close(errsCh)
	for err := range errsCh {
		if err != nil {
			t.Errorf("concurrent replay: %v", err)
		}
	}
	if got, want := store.entityCount(), len(plan.WorkUnits)+1; got != want {
		t.Errorf("entity count = %d, want %d", got, want)
	}
	if got := store.completionCommits(); got != 1 {
		t.Errorf("release commits = %d, want 1", got)
	}
}

func TestCreateOrVerifyUnitPreservesClassifiedReadFailures(t *testing.T) {
	board := seedTestBoard()
	in := seedTestInput([]Node{{ID: "a", Objective: "work"}})
	plan, err := buildUnitSeedPlan(board, in)
	if err != nil {
		t.Fatalf("buildUnitSeedPlan(): %v", err)
	}
	duplicate := errs.ClassifiedCode(
		errs.ErrorInvalid, graph.ErrorCodeEntityExists, errors.New("duplicate"),
	)
	transient := errors.New("temporary query failure")
	missing := errs.ClassifiedCode(
		errs.ErrorInvalid, graph.ErrorCodeEntityNotFound, errors.New("missing"),
	)

	tests := []struct {
		name           string
		read           unitReadFunc
		wantCause      error
		wantCorruption bool
	}{
		{
			name: "transient",
			read: func(context.Context, string) (*graph.EntityState, error) {
				return nil, transient
			},
			wantCause: transient,
		},
		{
			name: "confirmed missing",
			read: func(context.Context, string) (*graph.EntityState, error) {
				return nil, missing
			},
			wantCause:      missing,
			wantCorruption: true,
		},
		{
			name: "immutable collision",
			read: func(context.Context, string) (*graph.EntityState, error) {
				entity := entityFromTriples(plan.ActivationID, plan.ActivationTriples)
				entity.Triples[0].Object = "collision"
				return entity, nil
			},
			wantCorruption: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := createOrVerifyUnit(
				context.Background(),
				func(context.Context, string, []message.Triple) error { return duplicate },
				test.read,
				plan.ActivationID,
				plan.ActivationTriples,
			)
			if test.wantCause != nil && !errors.Is(err, test.wantCause) {
				t.Errorf("error = %v, want cause %v", err, test.wantCause)
			}
			if got := errors.Is(err, errUnitSeedCorruption); got != test.wantCorruption {
				t.Errorf("corruption = %v, want %v (error: %v)", got, test.wantCorruption, err)
			}
		})
	}
}

func TestActivatedReplayPreservesTransientManifestFailures(t *testing.T) {
	board := seedTestBoard()
	in := seedTestInput([]Node{{ID: "a", Objective: "work"}})
	store := newFakeUnitSeedStore()
	if _, err := seedUnitsWithSeams(
		context.Background(), board, in,
		store.create, store.read, store.list, store,
	); err != nil {
		t.Fatalf("initial seed: %v", err)
	}
	plan, err := buildUnitSeedPlan(board, in)
	if err != nil {
		t.Fatalf("buildUnitSeedPlan: %v", err)
	}
	readTransient := errors.New("temporary read failure")
	listTransient := errors.New("temporary list failure")
	tests := []struct {
		name      string
		transient error
		read      unitReadFunc
		list      unitListFunc
	}{
		{
			name:      "read",
			transient: readTransient,
			read: func(ctx context.Context, id string) (*graph.EntityState, error) {
				if id == plan.WorkUnits[0].ID {
					return nil, readTransient
				}
				return store.read(ctx, id)
			},
			list: store.list,
		},
		{
			name:      "list",
			transient: listTransient,
			read:      store.read,
			list: func(context.Context, string) ([]graph.EntityState, error) {
				return nil, listTransient
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := seedUnitsWithSeams(
				context.Background(), board, in,
				store.create, test.read, test.list, store,
			)
			if !errors.Is(err, test.transient) {
				t.Fatalf("activated replay error = %v, want transient %s cause", err, test.name)
			}
			if errors.Is(err, errUnitSeedCorruption) {
				t.Fatalf("transient %s failure was mislabeled corruption: %v", test.name, err)
			}
		})
	}
}

func TestVerifyImmutableUnitRequiresExactEnvelopeSubjectAndLatchDigest(t *testing.T) {
	board := seedTestBoard()
	in := seedTestInput([]Node{{ID: "a", Objective: "work"}})
	plan, err := buildUnitSeedPlan(board, in)
	if err != nil {
		t.Fatalf("buildUnitSeedPlan(): %v", err)
	}
	work := plan.WorkUnits[0]
	if workDigest := entityFromTriples(work.ID, work.Triples).GetTriple(PredUnitPlanDigest); workDigest != nil {
		t.Fatal("work unit carries activation-only plan digest")
	}
	tests := []struct {
		name   string
		mutate func(*graph.EntityState)
	}{
		{
			name: "wrong envelope",
			mutate: func(entity *graph.EntityState) {
				entity.MessageType = message.Type{Domain: "other", Category: "unit", Version: "v1"}
			},
		},
		{
			name: "wrong immutable subject",
			mutate: func(entity *graph.EntityState) {
				entity.Triples[0].Subject = board.DagUnitEntityID("ffffffffffffffffffffffffffffffff")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			entity := entityFromTriples(plan.ActivationID, plan.ActivationTriples)
			test.mutate(entity)
			if err := verifyImmutableUnit(entity, plan.ActivationID, plan.ActivationTriples); !errors.Is(err, errUnitSeedCorruption) {
				t.Fatalf("verifyImmutableUnit() = %v, want corruption", err)
			}
		})
	}
}

func TestNATSUnitCompletionWriterAuthoritativeReadbackCases(t *testing.T) {
	board := seedTestBoard()
	in := seedTestInput([]Node{{ID: "a", Objective: "work"}})
	plan, err := buildUnitSeedPlan(board, in)
	if err != nil {
		t.Fatalf("buildUnitSeedPlan(): %v", err)
	}
	inactive := entityFromTriples(plan.ActivationID, plan.ActivationTriples)
	inactive.Version = 7
	completed := inactive.Clone()
	completed.Version = 8
	completed.Triples = append(completed.Triples, message.Triple{
		Subject: completed.ID, Predicate: frameworkGatedDAGDefaults().CompletedPredicate, Object: true,
		Source: unitCompletionOwner, Confidence: 1,
	})
	transient := errors.New("lost update reply")
	missing := errs.ClassifiedCode(
		errs.ErrorInvalid, graph.ErrorCodeEntityNotFound, errors.New("missing"),
	)
	ownerStale := errs.ClassifiedCode(
		errs.ErrorInvalid, graph.ErrorCodeOwnerLeaseStale, errors.New("owner stale"),
	)
	revision := errs.ClassifiedCode(
		errs.ErrorInvalid, graph.ErrorCodeRevisionMismatch, errors.New("revision race"),
	)

	tests := []struct {
		name           string
		reads          []readResult
		updateErr      error
		wantCause      error
		wantCorruption bool
		wantUpdates    int
	}{
		{name: "confirmed success", reads: []readResult{{entity: inactive}, {entity: completed}}, wantUpdates: 1},
		{name: "lost reply confirmed", reads: []readResult{{entity: inactive}, {entity: completed}}, updateErr: transient, wantUpdates: 1},
		{name: "revision race confirmed", reads: []readResult{{entity: inactive}, {entity: completed}}, updateErr: revision, wantUpdates: 1},
		{name: "revision without marker", reads: []readResult{{entity: inactive}, {entity: inactive}}, updateErr: revision, wantCause: revision, wantUpdates: 1},
		{name: "lost reply then missing", reads: []readResult{{entity: inactive}, {err: missing}}, updateErr: transient, wantCause: missing, wantUpdates: 1},
		{name: "missing before update", reads: []readResult{{err: missing}}, wantCause: missing},
		{name: "owner stale", reads: []readResult{{entity: inactive}}, updateErr: ownerStale, wantCause: ownerStale, wantUpdates: 1},
		{name: "success missing marker", reads: []readResult{{entity: inactive}, {entity: inactive}}, wantCorruption: true, wantUpdates: 1},
		{
			name:           "failed activation",
			reads:          []readResult{{entity: entityWithMarker(inactive, frameworkGatedDAGDefaults().FailedPredicate)}},
			wantCorruption: true,
		},
		{
			name:           "dirtied activation",
			reads:          []readResult{{entity: entityWithMarker(inactive, frameworkGatedDAGDefaults().DirtiedPredicate)}},
			wantCorruption: true,
		},
		{
			name: "malformed readback",
			reads: []readResult{
				{entity: inactive},
				{entity: func() *graph.EntityState {
					entity := completed.Clone()
					entity.MessageType = message.Type{Domain: "wrong", Category: "unit", Version: "v1"}
					return entity
				}()},
			},
			wantCorruption: true,
			wantUpdates:    1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				readIndex int
				updates   int
			)
			writer := &natsUnitCompletionWriter{
				ownerToken: ownership.ExpectedOwnerToken(unitCompletionOwner, "test"),
				read: func(context.Context, string) (*graph.EntityState, error) {
					if readIndex >= len(test.reads) {
						t.Fatalf("unexpected authoritative read %d", readIndex+1)
					}
					result := test.reads[readIndex]
					readIndex++
					return result.entity, result.err
				},
				update: func(
					_ context.Context,
					id string,
					marker message.Triple,
					token ownership.OwnerToken,
				) error {
					updates++
					if id != plan.ActivationID || marker.Subject != id ||
						marker.Predicate != frameworkGatedDAGDefaults().CompletedPredicate || marker.Source != unitCompletionOwner {
						t.Errorf("completion update marker = %#v", marker)
					}
					if token.IsZero() {
						t.Error("completion update omitted owner token")
					}
					return test.updateErr
				},
			}
			err := writer.Complete(context.Background(), plan.ActivationID)
			if test.wantCause != nil && !errors.Is(err, test.wantCause) {
				t.Errorf("Complete() error = %v, want cause %v", err, test.wantCause)
			}
			if test.wantCause == nil && !test.wantCorruption && err != nil {
				t.Errorf("Complete() unexpected error: %v", err)
			}
			if got := errors.Is(err, errUnitSeedCorruption); got != test.wantCorruption {
				t.Errorf("corruption = %v, want %v (error: %v)", got, test.wantCorruption, err)
			}
			if updates != test.wantUpdates {
				t.Errorf("updates = %d, want %d", updates, test.wantUpdates)
			}
		})
	}
}

func TestSeedUnitsWithSeamsAcceptsMatchingReplayAndRejectsCollision(t *testing.T) {
	board := seedTestBoard()
	in := seedTestInput([]Node{{ID: "a", Objective: "work"}})
	store := newFakeUnitSeedStore()

	if _, err := seedUnitsWithSeams(context.Background(), board, in, store.create, store.read, store.list, store); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	firstCreates := len(store.createOrder)
	if _, err := seedUnitsWithSeams(context.Background(), board, in, store.create, store.read, store.list, store); err != nil {
		t.Fatalf("matching replay: %v", err)
	}
	if len(store.createOrder) != firstCreates {
		t.Errorf("activated replay created more work units: %d -> %d", firstCreates, len(store.createOrder))
	}
	if store.completeCalls != 1 {
		t.Errorf("matching replay release commits = %d, want 1 total", store.completeCalls)
	}
	differentManifest := in
	differentManifest.DAG.Nodes = append([]Node(nil), in.DAG.Nodes...)
	differentManifest.DAG.Nodes[0].Objective = "different immutable plan"
	if _, err := seedUnitsWithSeams(
		context.Background(), board, differentManifest,
		store.create, store.read, store.list, store,
	); !errors.Is(err, errUnitSeedCorruption) {
		t.Fatalf("different-manifest replay error = %v, want corruption", err)
	}

	collisionStore := newFakeUnitSeedStore()
	plan, err := buildUnitSeedPlan(board, in)
	if err != nil {
		t.Fatalf("buildUnitSeedPlan(): %v", err)
	}
	collisionStore.entities[plan.ActivationID] = entityFromTriples(plan.ActivationID, plan.ActivationTriples)
	collisionStore.entities[plan.NodeUnits["a"]] = entityFromTriples(plan.NodeUnits["a"], []message.Triple{{
		Subject: plan.NodeUnits["a"], Predicate: PredUnitNodeID, Object: "collision",
	}})

	_, err = seedUnitsWithSeams(
		context.Background(),
		board,
		in,
		collisionStore.create,
		collisionStore.read,
		collisionStore.list,
		collisionStore,
	)
	if !errors.Is(err, errUnitSeedCorruption) {
		t.Fatalf("collision error = %v, want errUnitSeedCorruption", err)
	}
}

func TestSeedUnitsWithSeamsTreatsActivatedMissingManifestAsCorruption(t *testing.T) {
	board := seedTestBoard()
	in := seedTestInput([]Node{{ID: "a", Objective: "work"}})
	store := newFakeUnitSeedStore()
	if _, err := seedUnitsWithSeams(context.Background(), board, in, store.create, store.read, store.list, store); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	plan, err := buildUnitSeedPlan(board, in)
	if err != nil {
		t.Fatalf("buildUnitSeedPlan(): %v", err)
	}
	delete(store.entities, plan.NodeUnits["a"])

	_, err = seedUnitsWithSeams(context.Background(), board, in, store.create, store.read, store.list, store)
	if !errors.Is(err, errUnitSeedCorruption) {
		t.Fatalf("activated missing manifest error = %v, want errUnitSeedCorruption", err)
	}
	if _, recreated := store.entities[plan.NodeUnits["a"]]; recreated {
		t.Error("activated replay recreated a missing manifest unit")
	}
}

func TestSeedUnitsWithSeamsRejectsUnexpectedUnitInExecutionManifest(t *testing.T) {
	board := seedTestBoard()
	in := seedTestInput([]Node{{ID: "a", Objective: "work"}})
	store := newFakeUnitSeedStore()
	plan, err := buildUnitSeedPlan(board, in)
	if err != nil {
		t.Fatalf("buildUnitSeedPlan(): %v", err)
	}
	extraID := board.DagUnitEntityID("0123456789abcdef0123456789abcdef")
	store.entities[extraID] = entityFromTriples(extraID, []message.Triple{
		{Subject: extraID, Predicate: PredUnitNodeID, Object: "legacy-random"},
		{Subject: extraID, Predicate: PredUnitParentQuest, Object: in.ParentQuestID, Datatype: "@id"},
		{Subject: extraID, Predicate: PredUnitExecutionID, Object: in.ExecutionID},
		{Subject: extraID, Predicate: PredUnitPlanDigest, Object: plan.Digest},
		{Subject: extraID, Predicate: frameworkGatedDAGDefaults().DependsOnPredicate, Object: plan.ActivationID, Datatype: "@id"},
	})

	_, err = seedUnitsWithSeams(context.Background(), board, in, store.create, store.read, store.list, store)
	if !errors.Is(err, errUnitSeedCorruption) {
		t.Fatalf("unexpected execution unit error = %v, want errUnitSeedCorruption", err)
	}
	if store.completeCalls != 0 {
		t.Errorf("release commits = %d, want 0 when manifest has an unexpected unit", store.completeCalls)
	}
}

type fakeUnitSeedStore struct {
	mu                     sync.Mutex
	entities               map[string]*graph.EntityState
	createOrder            []string
	completeCalls          int
	createCalls            int
	failCreateAt           int
	failCreateErr          error
	failCreateAfterPersist bool
	listErrors             []error
}

func newFakeUnitSeedStore() *fakeUnitSeedStore {
	return &fakeUnitSeedStore{entities: make(map[string]*graph.EntityState)}
}

func (s *fakeUnitSeedStore) create(_ context.Context, id string, triples []message.Triple) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.createCalls++
	if _, exists := s.entities[id]; exists {
		return errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeEntityExists, errors.New("duplicate"))
	}
	if s.createCalls == s.failCreateAt && !s.failCreateAfterPersist {
		return s.failCreateErr
	}
	s.entities[id] = entityFromTriples(id, triples)
	s.createOrder = append(s.createOrder, id)
	if s.createCalls == s.failCreateAt {
		return s.failCreateErr
	}
	return nil
}

func (s *fakeUnitSeedStore) read(_ context.Context, id string) (*graph.EntityState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entity, exists := s.entities[id]
	if !exists {
		return nil, errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeEntityNotFound, errors.New("missing"))
	}
	return entity.Clone(), nil
}

func (s *fakeUnitSeedStore) list(_ context.Context, executionID string) ([]graph.EntityState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.listErrors) > 0 {
		err := s.listErrors[0]
		s.listErrors = s.listErrors[1:]
		return nil, err
	}
	var entities []graph.EntityState
	for _, entity := range s.entities {
		if entityHasExecution(entity, executionID) {
			entities = append(entities, *entity.Clone())
		}
	}
	return entities, nil
}

func (s *fakeUnitSeedStore) Complete(_ context.Context, unitID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entity := s.entities[unitID]
	if entity.GetTriple(frameworkGatedDAGDefaults().CompletedPredicate) != nil {
		return nil
	}
	s.completeCalls++
	filtered := entity.Triples[:0]
	for _, triple := range entity.Triples {
		if triple.Predicate != frameworkGatedDAGDefaults().CompletedPredicate {
			filtered = append(filtered, triple)
		}
	}
	entity.Triples = append(filtered, message.Triple{
		Subject: unitID, Predicate: frameworkGatedDAGDefaults().CompletedPredicate, Object: true, Source: unitCompletionOwner, Confidence: 1,
	})
	return nil
}

func (s *fakeUnitSeedStore) entity(id string) *graph.EntityState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.entities[id].Clone()
}

func (s *fakeUnitSeedStore) entityCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entities)
}

func (s *fakeUnitSeedStore) completionCommits() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.completeCalls
}

func (s *fakeUnitSeedStore) clearCreateFailure() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failCreateAt = 0
	s.failCreateErr = nil
}

func (s *fakeUnitSeedStore) setListErrors(errors ...error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listErrors = append(s.listErrors, errors...)
}

type readResult struct {
	entity *graph.EntityState
	err    error
}

func entityWithMarker(base *graph.EntityState, predicate string) *graph.EntityState {
	entity := base.Clone()
	entity.Triples = append(entity.Triples, message.Triple{
		Subject: entity.ID, Predicate: predicate, Object: true, Confidence: 1,
	})
	return entity
}

func entityFromTriples(id string, triples []message.Triple) *graph.EntityState {
	return &graph.EntityState{
		ID:          id,
		MessageType: unitMessageType,
		Version:     1,
		Triples:     append([]message.Triple(nil), triples...),
	}
}

func seedTestBoard() *domain.BoardConfig {
	return &domain.BoardConfig{Org: "c360", Platform: "local", Board: "board"}
}

func seedTestInput(nodes []Node) SeedInput {
	board := seedTestBoard()
	return SeedInput{
		DAG:           DAG{Nodes: nodes},
		ParentQuestID: board.QuestEntityID("parent"),
		ExecutionID:   "execution-1",
		NodeSubQuests: map[string]string{"a": board.QuestEntityID("sub-a"), "b": board.QuestEntityID("sub-b")},
	}
}

func mapValues(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func assertPredicateValue(t *testing.T, entity *graph.EntityState, predicate string, want any) {
	t.Helper()
	triple := entity.GetTriple(predicate)
	if triple == nil {
		t.Errorf("%s missing %q", entity.ID, predicate)
		return
	}
	if !reflect.DeepEqual(triple.Object, want) {
		t.Errorf("%s %s = %#v, want %#v", entity.ID, predicate, triple.Object, want)
	}
}

func assertExactReferences(t *testing.T, entity *graph.EntityState, predicate string, want []string) {
	t.Helper()
	var got []string
	for _, triple := range entity.Triples {
		if triple.Predicate != predicate {
			continue
		}
		if triple.Datatype != "@id" {
			t.Errorf("%s %s datatype = %q, want @id", entity.ID, predicate, triple.Datatype)
		}
		object, ok := triple.Object.(string)
		if !ok {
			t.Errorf("%s %s object = %#v, want string", entity.ID, predicate, triple.Object)
			continue
		}
		got = append(got, object)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s %s references = %#v, want %#v", entity.ID, predicate, got, want)
	}
}
