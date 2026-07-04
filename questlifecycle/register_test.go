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
	"log/slog"
	"testing"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/lifecycle"
)

func TestWorkflow_Registers(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())

	mgr := lifecycle.NewManager(tc.Client, slog.Default())
	if err := mgr.Register(WorkflowDeclaration()); err != nil {
		t.Fatalf("Register(quest workflow) = %v, want nil", err)
	}

	// A second registration of the same Name must be rejected (guards against
	// double-registration in boot wiring).
	if err := mgr.Register(WorkflowDeclaration()); err == nil {
		t.Error("re-registering the quest workflow should fail with ErrWorkflowAlreadyRegistered")
	}
}
