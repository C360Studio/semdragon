//go:build integration

package questlifecycle

// =============================================================================
// INTEGRATION TESTS — de-risk the Stage B1 destructive rewire (tasks 2.4–2.6)
// =============================================================================
// These prove the two mechanisms the caller rewire depends on, BEFORE any caller
// is changed:
//
//   1. TestManagerTransition_NoCreate — a quest seeded exactly as questboard's
//      PostQuest seeds it (GraphClient.EmitEntity full-object Put, NO
//      Manager.Create) is transitionable through the lifecycle Manager. This is
//      the seam autonomy moves onto: Manager.TransitionWith gives synchronous,
//      CAS-guarded, one-winner claim semantics (a concurrent loser gets
//      ErrInvalidTransition), reading the phase off the KV triple PostQuest
//      already wrote. Proves the resume-doc "post → Manager.Create" step is
//      unnecessary.
//
//   2. TestRulesFire_SubmitRouting / TestRulesFire_FailRouting — the submit and
//      fail rules (02-submit.json, 03-fail.json) actually fire end-to-end when a
//      caller stamps quest.lifecycle.command via a graph-ingest delta, routing on
//      quest.review.needs-review (submit) and quest.attempts.current (fail).
//      This is the seam questbridge moves onto. The existing
//      rules_fire_test.go only proved the claim rule.
//
// Requires Docker (NATS via testcontainers).
// =============================================================================

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/lifecycle"
	graphingest "github.com/c360studio/semstreams/processor/graph-ingest"
	rule "github.com/c360studio/semstreams/processor/rule"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
)

// newRuleStack stands up the real graph-ingest + rule-processor + lifecycle
// Manager stack the pure-rules design runs on, waits for KV watchers to attach,
// and returns the client and Manager. Mirrors the setup in rules_fire_test.go.
func newRuleStack(t *testing.T, ctx context.Context) (*natsclient.TestClient, *lifecycle.Manager) {
	t.Helper()
	tc := natsclient.NewTestClient(t,
		natsclient.WithKVBuckets(graph.BucketEntityStates),
		natsclient.WithStreams(natsclient.TestStreamConfig{Name: "ENTITY", Subjects: []string{"entity.>"}}),
	)

	mgr := lifecycle.NewManager(tc.Client, slog.Default())
	if err := mgr.Register(WorkflowDeclaration()); err != nil {
		t.Fatalf("register quest workflow: %v", err)
	}

	startComponent(t, ctx, mustJSON(t, graphingest.DefaultConfig()), func(cfg json.RawMessage) (component.Discoverable, error) {
		return graphingest.CreateGraphIngest(cfg, component.Dependencies{NATSClient: tc.Client})
	})
	ruleCfg := mustJSON(t, map[string]any{
		"rules_files":              questRuleFiles(t),
		"pack_id":                  "quest-lifecycle-test",
		"enable_graph_integration": true,
		"entity_watch_buckets": map[string][]string{
			graph.BucketEntityStates: {"*.*.game.*.quest.*"},
		},
	})
	startComponent(t, ctx, ruleCfg, func(cfg json.RawMessage) (component.Discoverable, error) {
		return rule.CreateRuleProcessor(cfg, component.Dependencies{NATSClient: tc.Client, LifecycleManager: mgr})
	})

	// Let the KV watchers attach before writing.
	time.Sleep(750 * time.Millisecond)
	return tc, mgr
}

// questEntityID builds a default-board 6-part quest id for the given instance.
func questEntityID(instance string) string {
	cfg := domain.DefaultBoardConfig()
	return cfg.QuestEntityID(instance)
}

// waitForPhase polls the Manager until the quest reaches want or the deadline.
func waitForPhase(t *testing.T, ctx context.Context, mgr *lifecycle.Manager, questID, want string) *QuestState {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if p, err := mgr.Get(ctx, WorkflowName, questID); err == nil {
			if qs, ok := p.(*QuestState); ok && qs.PhaseField == want {
				return qs
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	if p, err := mgr.Get(ctx, WorkflowName, questID); err == nil {
		t.Fatalf("quest %s did not reach %q; current phase=%q", questID, want, p.Phase())
	}
	t.Fatalf("quest %s did not reach %q (and could not be read back)", questID, want)
	return nil
}

// TestManagerTransition_NoCreate proves a PostQuest-seeded quest (full-object
// EmitEntity, no Manager.Create) is transitionable through the Manager — the
// seam autonomy's claim rewire relies on.
func TestManagerTransition_NoCreate(t *testing.T) {
	ctx := context.Background()
	tc, mgr := newRuleStack(t, ctx)

	boardCfg := domain.DefaultBoardConfig()
	gc := semdragons.NewGraphClient(tc.Client, &boardCfg)

	questID := boardCfg.QuestEntityID("nocreate1")
	const agentID = "default.local.game.main.agent.dragon"

	// Seed exactly as questboard.PostQuest does: a full-object store.Put of the
	// domain.Quest — NO Manager.Create. The only lifecycle-relevant triple this
	// writes is quest.status.state=posted, which is all the Manager needs.
	q := &domain.Quest{
		ID:          domain.QuestID(questID),
		Title:       "no-create claim",
		Status:      domain.QuestPosted,
		MaxAttempts: 3,
		PostedAt:    time.Now(),
	}
	if err := gc.EmitEntity(ctx, q, "quest.posted"); err != nil {
		t.Fatalf("seed posted quest via EmitEntity: %v", err)
	}

	// Claim: posted → claimed with claimed_by + attempts++ set atomically via the
	// mutator, exactly as autonomy will. A concurrent loser would get
	// ErrInvalidTransition here (claimed → claimed is not a declared edge).
	err := mgr.TransitionWith(ctx, WorkflowName, questID, PhaseClaimed,
		lifecycle.TransitionSourceComponent, "autonomous claim",
		func(p lifecycle.Participant) error {
			qs := p.(*QuestState)
			qs.ClaimedBy = agentID
			qs.Attempts++
			return nil
		})
	if err != nil {
		t.Fatalf("TransitionWith(claimed) on PostQuest-seeded quest: %v", err)
	}

	// Start: claimed → in_progress, as autonomy will immediately after claim.
	if err := mgr.Transition(ctx, WorkflowName, questID, PhaseInProgress,
		lifecycle.TransitionSourceComponent, "autonomous start"); err != nil {
		t.Fatalf("Transition(in_progress): %v", err)
	}

	p, err := mgr.Get(ctx, WorkflowName, questID)
	if err != nil {
		t.Fatalf("read back quest: %v", err)
	}
	qs := p.(*QuestState)
	if qs.PhaseField != PhaseInProgress {
		t.Errorf("phase = %q, want %q", qs.PhaseField, PhaseInProgress)
	}
	if qs.ClaimedBy != agentID {
		t.Errorf("claimed_by = %q, want %q", qs.ClaimedBy, agentID)
	}
	if qs.Attempts != 1 {
		t.Errorf("attempts = %d, want 1", qs.Attempts)
	}
}

// TestRulesFire_SubmitRouting proves the submit rules route in_progress by the
// needs_review predicate: false → completed directly, true → in_review then
// (mechanics-stub) → completed with the review-skipped verdict.
func TestRulesFire_SubmitRouting(t *testing.T) {
	ctx := context.Background()
	tc, mgr := newRuleStack(t, ctx)

	seedInProgress := func(t *testing.T, instance string, needsReview bool, attempts int) string {
		t.Helper()
		questID := questEntityID(instance)
		if err := mgr.Create(ctx, &QuestState{EntityIDField: questID, PhaseField: PhaseInProgress, Attempts: attempts, MaxAttempts: 3}); err != nil {
			t.Fatalf("create in_progress quest: %v", err)
		}
		// needs_review is a non-projection predicate the submit rules read; write
		// it directly, as domain.Quest.Triples does at post time.
		if err := StampTriple(ctx, tc.Client, questID, "quest.review.needs-review", needsReview); err != nil {
			t.Fatalf("stamp needs_review: %v", err)
		}
		return questID
	}

	t.Run("no review completes directly", func(t *testing.T) {
		questID := seedInProgress(t, "submit-noreview", false, 1)
		if err := StampCommand(ctx, tc.Client, questID, CommandSubmit, map[string]string{PredDataOutput: "the deliverable"}); err != nil {
			t.Fatalf("stamp submit: %v", err)
		}
		qs := waitForPhase(t, ctx, mgr, questID, PhaseCompleted)
		if qs.Verdict != "" {
			t.Errorf("verdict = %q, want empty (direct completion, no review)", qs.Verdict)
		}
	})

	t.Run("review routes through in_review then stub-completes", func(t *testing.T) {
		questID := seedInProgress(t, "submit-review", true, 1)
		if err := StampCommand(ctx, tc.Client, questID, CommandSubmit, map[string]string{PredDataOutput: "needs a look"}); err != nil {
			t.Fatalf("stamp submit: %v", err)
		}
		qs := waitForPhase(t, ctx, mgr, questID, PhaseCompleted)
		if qs.Verdict != "review-skipped-mechanics-disabled" {
			t.Errorf("verdict = %q, want review-skipped marker (went through in_review + stub rule)", qs.Verdict)
		}
	})
}

// TestRulesFire_FailRouting proves the fail rules route in_progress by the
// attempt count: below max → posted (retry, agent freed); at max → pending_triage.
func TestRulesFire_FailRouting(t *testing.T) {
	ctx := context.Background()
	tc, mgr := newRuleStack(t, ctx)

	seedInProgress := func(t *testing.T, instance string, attempts int) string {
		t.Helper()
		questID := questEntityID(instance)
		if err := mgr.Create(ctx, &QuestState{EntityIDField: questID, PhaseField: PhaseInProgress, Attempts: attempts, MaxAttempts: 3, ClaimedBy: "default.local.game.main.agent.dragon"}); err != nil {
			t.Fatalf("create in_progress quest: %v", err)
		}
		return questID
	}

	t.Run("retries remaining reposts and frees the agent", func(t *testing.T) {
		questID := seedInProgress(t, "fail-retry", 1)
		if err := StampCommand(ctx, tc.Client, questID, CommandFail, nil); err != nil {
			t.Fatalf("stamp fail: %v", err)
		}
		qs := waitForPhase(t, ctx, mgr, questID, PhasePosted)
		if qs.ClaimedBy != "" {
			t.Errorf("claimed_by = %q, want empty (agent freed on retry)", qs.ClaimedBy)
		}
	})

	t.Run("budget exhausted routes to triage", func(t *testing.T) {
		questID := seedInProgress(t, "fail-triage", 3)
		if err := StampCommand(ctx, tc.Client, questID, CommandFail, nil); err != nil {
			t.Fatalf("stamp fail: %v", err)
		}
		waitForPhase(t, ctx, mgr, questID, PhasePendingTriage)
	})
}
