//go:build integration

package questlifecycle

// =============================================================================
// INTEGRATION TEST — command -> rule -> lifecycle transition, end to end
// =============================================================================
// Stands up the real beta.159 path: graph-ingest owns ENTITY_STATES, the rule
// processor watches the quest scope and executes lifecycle_transition, and the
// lifecycle Manager performs ownership-aware writes. Readiness envelopes, rather
// than sleeps, gate the first mutation so watcher bootstrap is deterministic.
//
// Requires Docker (NATS via testcontainers).
// =============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/graph/readiness"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/lifecycle"
	"github.com/c360studio/semstreams/pkg/ownership"
	graphingest "github.com/c360studio/semstreams/processor/graph-ingest"
	rule "github.com/c360studio/semstreams/processor/rule"
	"github.com/c360studio/semstreams/vocabulary"

	"github.com/c360studio/semdragons/domain"
)

func TestRulesFire_StartCommandTransitionsClaimedToInProgress(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)

	tc := natsclient.NewTestClient(t,
		natsclient.WithKV(),
		natsclient.WithStreams(natsclient.TestStreamConfig{Name: "ENTITY", Subjects: []string{"entity.>"}}),
	)

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
		t.Fatalf("register quest workflow: %v", err)
	}

	ingestConfig := graphingest.DefaultConfig()
	ingestConfig.EnforceOwnerLease = true
	startComponent(t, ctx, mustJSON(t, ingestConfig), func(cfg json.RawMessage) (component.Discoverable, error) {
		return graphingest.CreateGraphIngest(cfg, component.Dependencies{NATSClient: tc.Client})
	})
	waitForReadyProducer(t, ctx, tc.Client, readiness.KeyGraphIngest)

	ruleConfig := mustJSON(t, map[string]any{
		"rules_files":              questRuleFiles(t),
		"pack_id":                  "quest-lifecycle-test",
		"enable_graph_integration": true,
		"entity_watch_buckets": map[string][]string{
			graph.BucketEntityStates: {EntityIDPattern},
		},
	})
	startComponent(t, ctx, ruleConfig, func(cfg json.RawMessage) (component.Discoverable, error) {
		return rule.CreateRuleProcessor(cfg, component.Dependencies{
			NATSClient:       tc.Client,
			LifecycleManager: mgr,
		})
	})
	waitForReadyProducer(t, ctx, tc.Client, readiness.KeyRule)

	const questID = "local.dev.game.board1.quest.firetest1"
	if err := mgr.Create(ctx, &QuestState{
		EntityIDField: questID,
		PhaseField:    PhaseClaimed,
		MaxAttempts:   3,
	}); err != nil {
		t.Fatalf("create claimed quest: %v", err)
	}
	if err := mgr.UpdateFromOperator(ctx, WorkflowName, questID, map[string]any{
		"command": CommandStart,
	}); err != nil {
		t.Fatalf("write start command through lifecycle manager: %v", err)
	}

	got := waitForQuestState(t, ctx, mgr, questID, func(state *QuestState) bool {
		return state.PhaseField == PhaseInProgress && state.Command == CommandConsumed
	})
	if got.PhaseField != PhaseInProgress {
		t.Errorf("phase = %q, want %q", got.PhaseField, PhaseInProgress)
	}
	if got.Command != CommandConsumed {
		t.Errorf("command = %q, want %q", got.Command, CommandConsumed)
	}

	history, err := mgr.History(ctx, WorkflowName, questID)
	if err != nil {
		t.Fatalf("read quest transition history: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("retained transition history = %#v, want exactly one rule transition", history)
	}
	transition := history[0]
	if transition.From != PhaseClaimed || transition.To != PhaseInProgress {
		t.Errorf("transition = %q -> %q, want %q -> %q",
			transition.From, transition.To, PhaseClaimed, PhaseInProgress)
	}
	if transition.Triggered != lifecycle.TransitionSourceRule {
		t.Errorf("transition source = %q, want %q", transition.Triggered, lifecycle.TransitionSourceRule)
	}
	if transition.Note != "command=start" {
		t.Errorf("transition note = %q, want %q", transition.Note, "command=start")
	}
}

func startComponent(
	t *testing.T,
	ctx context.Context,
	cfg json.RawMessage,
	create func(json.RawMessage) (component.Discoverable, error),
) {
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

func waitForReadyProducer(
	t *testing.T,
	ctx context.Context,
	client *natsclient.Client,
	key string,
) {
	t.Helper()
	watcher := readiness.NewWatcher(client, key)
	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("start readiness watcher for %s: %v", key, err)
	}
	t.Cleanup(watcher.Stop)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		reading := watcher.Read()
		if reading.Known && reading.Fresh && reading.Status.Ready &&
			reading.Status.BootstrapComplete && reading.Status.State == graph.IndexStateReady {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("producer %s did not become known/fresh/ready: reading=%+v: %v",
				key, reading, ctx.Err())
		case <-ticker.C:
		}
	}
}

func waitForQuestState(
	t *testing.T,
	ctx context.Context,
	mgr *lifecycle.Manager,
	entityID string,
	accept func(*QuestState) bool,
) *QuestState {
	t.Helper()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var last string
	for {
		participant, err := mgr.Get(ctx, WorkflowName, entityID)
		if err == nil {
			state, ok := participant.(*QuestState)
			if !ok {
				t.Fatalf("quest projection type = %T, want *QuestState", participant)
			}
			last = fmt.Sprintf("phase=%q command=%q", state.PhaseField, state.Command)
			if accept(state) {
				return state
			}
		} else {
			last = err.Error()
		}
		select {
		case <-ctx.Done():
			t.Fatalf("quest did not reach expected state; last=%s: %v", last, ctx.Err())
		case <-ticker.C:
		}
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
