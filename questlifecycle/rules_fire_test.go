//go:build integration

package questlifecycle

// =============================================================================
// INTEGRATION TEST — command → rule → lifecycle transition, end to end
// =============================================================================
// Stands up the REAL stack the pure-rules design depends on: graph-ingest
// (persists mutations into ENTITY_STATES), the semstreams rule-processor loading
// config/rules/quest/*.json, and the lifecycle Manager with the quest workflow.
// A posted quest is created via Manager.Create, a quest.lifecycle.command=claim triple is
// stamped through graph-ingest, and the claim rule must fire a lifecycle_transition
// to claimed — setting claimed_by from the arg triple, incrementing attempts, and
// clearing the command to the sentinel. This proves the whole mechanism BEFORE any
// caller is rewired or questboard is deleted (Stage B1 de-risk).
//
// Requires Docker (NATS via testcontainers).
// =============================================================================

import (
	"context"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/lifecycle"
	graphingest "github.com/c360studio/semstreams/processor/graph-ingest"
	rule "github.com/c360studio/semstreams/processor/rule"
)

func TestRulesFire_ClaimCommandTransitionsPostedToClaimed(t *testing.T) {
	ctx := context.Background()
	tc := natsclient.NewTestClient(t,
		natsclient.WithKVBuckets(graph.BucketEntityStates),
		natsclient.WithStreams(natsclient.TestStreamConfig{Name: "ENTITY", Subjects: []string{"entity.>"}}),
	)

	// Lifecycle Manager + quest workflow.
	mgr := lifecycle.NewManager(tc.Client, slog.Default())
	if err := mgr.Register(WorkflowDeclaration()); err != nil {
		t.Fatalf("register quest workflow: %v", err)
	}

	// Real graph-ingest: persists Manager mutations and command deltas to ENTITY_STATES.
	startComponent(t, ctx, mustJSON(t, graphingest.DefaultConfig()), func(cfg json.RawMessage) (component.Discoverable, error) {
		return graphingest.CreateGraphIngest(cfg, component.Dependencies{NATSClient: tc.Client})
	})

	// Real rule-processor: watches quest entities, fires lifecycle_transition via the Manager.
	ruleCfg := mustJSON(t, map[string]any{
		"rules_files":           questRuleFiles(t),
		"entity_watch_patterns": []string{"*.*.game.*.quest.*"},
	})
	startComponent(t, ctx, ruleCfg, func(cfg json.RawMessage) (component.Discoverable, error) {
		return rule.CreateRuleProcessor(cfg, component.Dependencies{NATSClient: tc.Client, LifecycleManager: mgr})
	})

	// Let the KV watchers attach before writing.
	time.Sleep(750 * time.Millisecond)

	const questID = "local.dev.game.board1.quest.firetest1"
	const agentID = "local.dev.game.board1.agent.dragon"

	// Create the quest in phase posted (lifecycle-attached so phase gates resolve).
	if err := mgr.Create(ctx, &QuestState{EntityIDField: questID, PhaseField: PhasePosted, MaxAttempts: 3}); err != nil {
		t.Fatalf("create posted quest: %v", err)
	}

	// Stamp the claimant arg triple, then the command trigger — both as graph-ingest deltas.
	addTriple(t, ctx, tc.Client, questID, "quest.assignment.requested-by", agentID)
	addTriple(t, ctx, tc.Client, questID, predCommand, "claim")

	// The claim rule should fire posted → claimed within the poll window.
	var got *QuestState
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if p, err := mgr.Get(ctx, WorkflowName, questID); err == nil {
			if qs, ok := p.(*QuestState); ok && qs.PhaseField == PhaseClaimed {
				got = qs
				break
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	if got == nil {
		if p, err := mgr.Get(ctx, WorkflowName, questID); err == nil {
			t.Fatalf("quest did not reach claimed; current phase=%q", p.Phase())
		}
		t.Fatal("quest did not reach claimed (and could not be read back)")
	}

	if got.ClaimedBy != agentID {
		t.Errorf("claimed_by = %q, want %q (set from quest.assignment.requested-by)", got.ClaimedBy, agentID)
	}
	if got.Attempts != 1 {
		t.Errorf("attempts = %d, want 1 (incremented atomically with claim)", got.Attempts)
	}
	if got.Command != "consumed" {
		t.Errorf("command = %q, want %q (cleared atomically to prevent re-fire)", got.Command, "consumed")
	}
}

// --- helpers ---

func startComponent(t *testing.T, ctx context.Context, cfg json.RawMessage, create func(json.RawMessage) (component.Discoverable, error)) {
	t.Helper()
	comp, err := create(cfg)
	if err != nil {
		t.Fatalf("create component: %v", err)
	}
	lc, ok := comp.(component.LifecycleComponent)
	if !ok {
		t.Fatalf("component %T is not a LifecycleComponent", comp)
	}
	if err := lc.Initialize(); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if err := lc.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = lc.Stop(5 * time.Second) })
}

func addTriple(t *testing.T, ctx context.Context, client *natsclient.Client, subject, predicate, object string) {
	t.Helper()
	body, err := json.Marshal(graph.AddTripleRequest{
		Triple: message.Triple{Subject: subject, Predicate: predicate, Object: object, Confidence: 1.0},
	})
	if err != nil {
		t.Fatalf("marshal AddTripleRequest: %v", err)
	}
	if _, err := client.Request(ctx, graphingest.SubjectTripleAdd, body, 5*time.Second); err != nil {
		t.Fatalf("triple.add %s=%s: %v", predicate, object, err)
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	return b
}

func questRuleFiles(t *testing.T) []string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	base := filepath.Join(filepath.Dir(thisFile), "..", "config", "rules", "quest")
	return []string{
		filepath.Join(base, "01-claim-start.json"),
		filepath.Join(base, "02-submit.json"),
		filepath.Join(base, "03-fail.json"),
		filepath.Join(base, "04-mechanics-stub.json"),
	}
}
