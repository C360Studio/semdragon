//go:build integration

package questlifecycle

// =============================================================================
// INTEGRATION TEST — quest Workflow registration
// =============================================================================
// Requires Docker (NATS via testcontainers). Registration is the Stage B1 gate
// (task 2.1): it validates the QuestState lifecycle: struct tags, the transition
// table, the operator-writable allowlist, and predicate disjointness. Register
// performs no graph I/O, so this needs a live client but not graph-ingest; the
// full Create→Transition round trip is validated with the stack (task 2.5 / e2e).
// =============================================================================

import (
	"context"
	"log/slog"
	"testing"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/lifecycle"
	"github.com/c360studio/semstreams/pkg/ownership"
	"github.com/c360studio/semstreams/vocabulary"

	"github.com/c360studio/semdragons/domain"
)

func TestWorkflow_Registers(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ctx, cancel := context.WithCancel(context.Background())

	mgr := lifecycle.NewManager(tc.Client, slog.Default())
	t.Cleanup(func() {
		cancel()
		mgr.WaitOwnership()
	})
	ownerRegistry, err := ownership.EnsureBuckets(
		ctx,
		tc.Client,
		slog.Default(),
		vocabulary.InverseResolver,
	)
	if err != nil {
		t.Fatalf("ensure beta.159 ownership buckets: %v", err)
	}
	mgr.AttachOwnership(ctx, ownerRegistry)
	domain.RegisterVocabulary()
	if err := mgr.Register(WorkflowDeclaration()); err != nil {
		t.Fatalf("Register(quest workflow) = %v, want nil", err)
	}

	owner, ok, err := ownerRegistry.OwnerOf(
		ctx,
		"local.test.game.board1.quest.registration",
		PhasePredicate,
	)
	if err != nil {
		t.Fatalf("resolve lifecycle phase owner: %v", err)
	}
	if !ok || owner != WorkflowName {
		t.Fatalf("phase owner = %q, known=%v; want %q", owner, ok, WorkflowName)
	}

	// A second registration of the same Name must be rejected (guards against
	// double-registration in boot wiring).
	if err := mgr.Register(WorkflowDeclaration()); err == nil {
		t.Error("re-registering the quest workflow should fail with ErrWorkflowAlreadyRegistered")
	}
}
