//go:build integration

package semdragons_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/lifecycle"
	graphingest "github.com/c360studio/semstreams/processor/graph-ingest"
	rule "github.com/c360studio/semstreams/processor/rule"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/questlifecycle"
)

func TestMechanicsDisabledReviewCompletesWithoutBattleOrMechanicsSideEffects(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	testClient := natsclient.NewTestClient(t,
		natsclient.WithKVBuckets(graph.BucketEntityStates),
		natsclient.WithStreams(natsclient.TestStreamConfig{Name: "ENTITY", Subjects: []string{"entity.>"}}),
	)
	client := testClient.Client

	manager := lifecycle.NewManager(client, slog.Default())
	if err := manager.Register(questlifecycle.WorkflowDeclaration()); err != nil {
		t.Fatalf("register quest lifecycle: %v", err)
	}

	startPhase1Component(t, ctx, mustPhase1JSON(t, graphingest.DefaultConfig()),
		func(raw json.RawMessage) (component.Discoverable, error) {
			return graphingest.CreateGraphIngest(raw, component.Dependencies{NATSClient: client})
		})

	const barrierSubject = "battle.review.__phase1_barrier"
	rawBattleSubjects := make(chan string, 16)
	subjectBarrier := make(chan struct{}, 1)
	subscription, err := client.Subscribe(ctx, "battle.review.>", func(_ context.Context, msg *nats.Msg) {
		if msg.Subject == barrierSubject {
			subjectBarrier <- struct{}{}
			return
		}
		rawBattleSubjects <- msg.Subject
	})
	if err != nil {
		t.Fatalf("subscribe to removed raw battle family: %v", err)
	}
	t.Cleanup(func() {
		if err := subscription.Unsubscribe(); err != nil {
			t.Errorf("unsubscribe from removed raw battle family: %v", err)
		}
	})
	if _, err := client.RTT(); err != nil {
		t.Fatalf("establish raw battle subscription readiness: %v", err)
	}

	ruleConfig := shippedQuestRuleConfig(t)
	startPhase1Component(t, ctx, mustPhase1JSON(t, ruleConfig),
		func(raw json.RawMessage) (component.Discoverable, error) {
			return rule.CreateRuleProcessor(raw, component.Dependencies{
				NATSClient:       client,
				LifecycleManager: manager,
			})
		})

	entityStates, err := testClient.GetKVBucket(ctx, graph.BucketEntityStates)
	if err != nil {
		t.Fatalf("get %s bucket: %v", graph.BucketEntityStates, err)
	}

	const questID = "local.integration.game.phase1.quest.review-required"
	questWatcher, err := entityStates.Watch(ctx, questID)
	if err != nil {
		t.Fatalf("watch review-required quest: %v", err)
	}
	t.Cleanup(func() {
		if err := questWatcher.Stop(); err != nil {
			t.Errorf("stop review-required quest watcher: %v", err)
		}
	})
	awaitInitialKVSnapshot(t, ctx, questWatcher.Updates())

	if err := manager.Create(ctx, &questlifecycle.QuestState{
		EntityIDField: questID,
		PhaseField:    questlifecycle.PhaseInProgress,
		Attempts:      1,
		MaxAttempts:   3,
	}); err != nil {
		t.Fatalf("create review-required in-progress quest: %v", err)
	}
	if err := questlifecycle.StampTriple(
		ctx, client, questID, "quest.review.needs-review", true,
	); err != nil {
		t.Fatalf("mark quest review-required: %v", err)
	}
	if err := questlifecycle.StampCommand(
		ctx,
		client,
		questID,
		questlifecycle.CommandSubmit,
		map[string]string{questlifecycle.PredDataOutput: "phase-1 deliverable"},
	); err != nil {
		t.Fatalf("submit review-required quest: %v", err)
	}

	completed := awaitCompletedReviewSkip(t, ctx, questWatcher.Updates(), questID)
	if got, ok := completed.GetPropertyValue("quest.verdict.feedback"); !ok ||
		got != "review-skipped-mechanics-disabled" {
		t.Fatalf("quest verdict = %v (present=%v), want review-skipped-mechanics-disabled", got, ok)
	}

	if err := client.Publish(ctx, barrierSubject, nil); err != nil {
		t.Fatalf("publish raw battle wildcard barrier: %v", err)
	}
	select {
	case <-subjectBarrier:
	case <-ctx.Done():
		t.Fatalf("wait for raw battle wildcard barrier: %v", ctx.Err())
	}
	select {
	case subject := <-rawBattleSubjects:
		t.Fatalf("mechanics-disabled review published forbidden raw battle subject %q", subject)
	default:
	}

	assertNoMechanicsSideEffects(t, ctx, entityStates, questID)
}

func shippedQuestRuleConfig(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile("config/semdragons.json")
	if err != nil {
		t.Fatalf("read shipped default profile: %v", err)
	}
	var profile runtimeProfileContract
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatalf("decode shipped default profile: %v", err)
	}
	rules, ok := profile.Components["rule-processor"]
	if !ok {
		t.Fatal("shipped default profile has no rule-processor")
	}
	return map[string]any{
		"rules_files":           rules.Config.RulesFiles,
		"entity_watch_patterns": rules.Config.EntityWatchPatterns,
	}
}

func startPhase1Component(
	t *testing.T,
	ctx context.Context,
	raw json.RawMessage,
	create func(json.RawMessage) (component.Discoverable, error),
) {
	t.Helper()
	discoverable, err := create(raw)
	if err != nil {
		t.Fatalf("create component: %v", err)
	}
	lifecycleComponent, ok := discoverable.(component.LifecycleComponent)
	if !ok {
		t.Fatalf("component %T is not a LifecycleComponent", discoverable)
	}
	if err := lifecycleComponent.Initialize(); err != nil {
		t.Fatalf("initialize component %T: %v", discoverable, err)
	}
	if err := lifecycleComponent.Start(ctx); err != nil {
		t.Fatalf("start component %T: %v", discoverable, err)
	}
	t.Cleanup(func() {
		if err := lifecycleComponent.Stop(5 * time.Second); err != nil {
			t.Errorf("stop component %T: %v", discoverable, err)
		}
	})
}

func mustPhase1JSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal integration config: %v", err)
	}
	return data
}

func awaitInitialKVSnapshot(
	t *testing.T,
	ctx context.Context,
	updates <-chan jetstream.KeyValueEntry,
) {
	t.Helper()
	for {
		select {
		case entry, ok := <-updates:
			if !ok {
				t.Fatal("KV watcher closed before initial snapshot completed")
			}
			if entry == nil {
				return
			}
		case <-ctx.Done():
			t.Fatalf("wait for initial KV snapshot: %v", ctx.Err())
		}
	}
}

func awaitCompletedReviewSkip(
	t *testing.T,
	ctx context.Context,
	updates <-chan jetstream.KeyValueEntry,
	questID string,
) *graph.EntityState {
	t.Helper()
	for {
		select {
		case entry, ok := <-updates:
			if !ok {
				t.Fatal("quest KV watcher closed before completion")
			}
			if entry == nil {
				continue
			}
			state, err := semdragons.DecodeEntityState(entry)
			if err != nil {
				t.Fatalf("decode quest EntityState: %v", err)
			}
			phase, hasPhase := state.GetPropertyValue(questlifecycle.PhasePredicate)
			verdict, hasVerdict := state.GetPropertyValue("quest.verdict.feedback")
			if state.ID == questID && hasPhase && phase == questlifecycle.PhaseCompleted &&
				hasVerdict && verdict == "review-skipped-mechanics-disabled" {
				return state
			}
		case <-ctx.Done():
			t.Fatalf("wait for mechanics-disabled quest completion: %v", ctx.Err())
		}
	}
}

func assertNoMechanicsSideEffects(
	t *testing.T,
	ctx context.Context,
	entityStates jetstream.KeyValue,
	questID string,
) {
	t.Helper()
	keys, err := entityStates.Keys(ctx)
	if err != nil {
		t.Fatalf("list authoritative entity states: %v", err)
	}
	for _, key := range keys {
		if domain.IsBattleID(key) {
			t.Errorf("mechanics-disabled review created forbidden battle entity %q", key)
		}
		if domain.ExtractType(key) == domain.EntityTypePeerReview {
			t.Errorf("mechanics-disabled review created forbidden peer-review entity %q", key)
		}
		if key != questID {
			t.Errorf("mechanics-disabled review created unexpected side-effect entity %q", key)
		}

		entry, getErr := entityStates.Get(ctx, key)
		if getErr != nil {
			t.Fatalf("read authoritative entity %q: %v", key, getErr)
		}
		state, decodeErr := semdragons.DecodeEntityState(entry)
		if decodeErr != nil {
			t.Fatalf("decode authoritative entity %q: %v", key, decodeErr)
		}
		for _, triple := range state.Triples {
			predicate := triple.Predicate
			if strings.HasPrefix(predicate, "agent.progression.xp") ||
				strings.HasPrefix(predicate, "agent.inventory.") ||
				strings.HasPrefix(predicate, "agent.reputation.") ||
				strings.HasPrefix(predicate, "review.") ||
				strings.HasPrefix(predicate, "quest.artifacts.") ||
				predicate == "quest.verdict.xp-awarded" {
				t.Errorf("mechanics-disabled review wrote forbidden side-effect predicate %q on %q",
					predicate, key)
			}
		}
	}
}
