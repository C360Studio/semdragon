//go:build integration

package semdragons_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	semconfig "github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/lifecycle"
	graphingest "github.com/c360studio/semstreams/processor/graph-ingest"
	rule "github.com/c360studio/semstreams/processor/rule"
	semservice "github.com/c360studio/semstreams/service"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/boidengine"
	"github.com/c360studio/semdragons/questlifecycle"
)

func TestPhase1DisabledBoidEngineCreatesNoRuntimeOrSuggestionBucket(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)

	data, err := os.ReadFile("config/semdragons.json")
	if err != nil {
		t.Fatalf("read shipped default profile: %v", err)
	}
	var profile semconfig.Config
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatalf("decode shipped default profile: %v", err)
	}
	boidConfig, exists := profile.Components[boidengine.ComponentName]
	if !exists {
		t.Fatal("components.boidengine is missing")
	}

	testClient := natsclient.NewTestClient(t, natsclient.WithKV())
	registry := component.NewRegistry()
	if err := boidengine.Register(registry); err != nil {
		t.Fatalf("register dormant boidengine factory: %v", err)
	}
	managerService, err := semservice.NewComponentManager(
		nil,
		&semservice.Dependencies{
			ComponentRegistry: registry,
			NATSClient:        testClient.Client,
		},
	)
	if err != nil {
		t.Fatalf("construct component manager: %v", err)
	}
	manager, ok := managerService.(*semservice.ComponentManager)
	if !ok {
		t.Fatalf("component manager type = %T", managerService)
	}
	t.Cleanup(func() {
		if err := manager.Stop(5 * time.Second); err != nil {
			t.Errorf("stop component manager: %v", err)
		}
	})

	filtered := &semconfig.Config{
		Components: semconfig.ComponentConfigs{
			boidengine.ComponentName: boidConfig,
		},
	}
	if err := manager.CreateComponentsFromConfig(ctx, filtered); err != nil {
		t.Fatalf("create components from disabled config: %v", err)
	}
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("start component manager: %v", err)
	}
	if components := manager.ListComponents(); len(components) != 0 {
		t.Errorf("disabled profile created %d component instances, want 0", len(components))
	}
	if instance := registry.Component(boidengine.ComponentName); instance != nil {
		t.Errorf("disabled profile registered live boidengine instance %T", instance)
	}

	if _, err := testClient.Client.GetKeyValueBucket(ctx, "BOID_SUGGESTIONS"); !errors.Is(
		err,
		jetstream.ErrBucketNotFound,
	) {
		t.Errorf("BOID_SUGGESTIONS lookup error = %v, want jetstream.ErrBucketNotFound", err)
	}
}

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

	const redTeamBarrierSubject = "redteam.__phase1_barrier"
	rawRedTeamSubjects := make(chan string, 16)
	redTeamBarrier := make(chan struct{}, 1)
	redTeamSubscription, err := client.Subscribe(ctx, "redteam.>", func(_ context.Context, msg *nats.Msg) {
		if msg.Subject == redTeamBarrierSubject {
			redTeamBarrier <- struct{}{}
			return
		}
		rawRedTeamSubjects <- msg.Subject
	})
	if err != nil {
		t.Fatalf("subscribe to dormant redteam message category: %v", err)
	}
	t.Cleanup(func() {
		if err := redTeamSubscription.Unsubscribe(); err != nil {
			t.Errorf("unsubscribe from dormant redteam message category: %v", err)
		}
	})
	if _, err := client.RTT(); err != nil {
		t.Fatalf("establish redteam subscription readiness: %v", err)
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
	proof := seedProgressionProofEntities(t, ctx, client, entityStates, questID, "completion")
	graphClient := semdragons.NewGraphClient(
		client,
		&domain.BoardConfig{Org: "local", Platform: "integration", Board: "phase1"},
	)
	guildID := domain.GuildID(proof.guildID)
	if err := graphClient.PutEntityState(ctx, &domain.Quest{
		ID:            domain.QuestID(questID),
		Title:         "Explicit Phase-1 review",
		Status:        domain.QuestPosted,
		Difficulty:    domain.DifficultyModerate,
		MaxAttempts:   3,
		PostedAt:      time.Unix(1_700_000_000, 0).UTC(),
		GuildPriority: &guildID,
		Constraints: domain.QuestConstraints{
			RequireReview: true,
		},
	}, "quest.posted"); err != nil {
		t.Fatalf("seed explicit posted quest: %v", err)
	}

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

	if err := manager.TransitionWith(
		ctx,
		questlifecycle.WorkflowName,
		questID,
		questlifecycle.PhaseClaimed,
		lifecycle.TransitionSourceOperator,
		"explicit Phase-1 claim",
		func(participant lifecycle.Participant) error {
			state := participant.(*questlifecycle.QuestState)
			state.ClaimedBy = proof.agentID
			state.Attempts++
			return nil
		},
	); err != nil {
		t.Fatalf("explicitly claim review-required quest: %v", err)
	}
	awaitQuestPhase(t, ctx, questWatcher.Updates(), questID, questlifecycle.PhaseClaimed)
	if err := manager.Transition(
		ctx,
		questlifecycle.WorkflowName,
		questID,
		questlifecycle.PhaseInProgress,
		lifecycle.TransitionSourceOperator,
		"explicit Phase-1 start",
	); err != nil {
		t.Fatalf("explicitly start review-required quest: %v", err)
	}
	awaitQuestPhase(t, ctx, questWatcher.Updates(), questID, questlifecycle.PhaseInProgress)
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
	finalQuest := domain.QuestFromEntityState(completed)
	if finalQuest == nil {
		t.Fatal("reconstruct completed Phase-1 quest")
	}
	if finalQuest.QuestType != domain.QuestTypeNormal ||
		finalQuest.RedTeamTarget != nil ||
		finalQuest.RedTeamQuestID != nil ||
		finalQuest.RedTeamStatus != "" {
		t.Errorf("completed quest contains forbidden redteam classification: %#v", finalQuest)
	}
	for _, triple := range completed.Triples {
		if strings.HasPrefix(triple.Predicate, "redteam.") ||
			strings.HasPrefix(triple.Predicate, "quest.classification.red-team") {
			t.Errorf("completed quest contains forbidden redteam predicate %q", triple.Predicate)
		}
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
	if err := client.Publish(ctx, redTeamBarrierSubject, nil); err != nil {
		t.Fatalf("publish redteam wildcard barrier: %v", err)
	}
	select {
	case <-redTeamBarrier:
	case <-ctx.Done():
		t.Fatalf("wait for redteam wildcard barrier: %v", ctx.Err())
	}
	select {
	case subject := <-rawRedTeamSubjects:
		t.Fatalf("mechanics-disabled review published forbidden redteam subject %q", subject)
	default:
	}

	assertEntityKVUnchanged(t, ctx, entityStates, proof.agentID, proof.agentBefore)
	assertEntityKVUnchanged(t, ctx, entityStates, proof.guildID, proof.guildBefore)
	assertNoMechanicsSideEffects(t, ctx, entityStates, questID, proof.agentID, proof.guildID)
}

func TestMechanicsDisabledTerminalFailureDoesNotMutateProgressionState(t *testing.T) {
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

	startPhase1Component(t, ctx, mustPhase1JSON(t, shippedQuestRuleConfig(t)),
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

	const questID = "local.integration.game.phase1.quest.terminal-failure"
	proof := seedProgressionProofEntities(t, ctx, client, entityStates, questID, "failure")

	if err := manager.Create(ctx, &questlifecycle.QuestState{
		EntityIDField: questID,
		PhaseField:    questlifecycle.PhaseInProgress,
		Attempts:      3,
		MaxAttempts:   3,
		ClaimedBy:     proof.agentID,
	}); err != nil {
		t.Fatalf("create terminal-failure in-progress quest: %v", err)
	}
	if err := questlifecycle.StampTriple(
		ctx, client, questID, "quest.priority.guild", proof.guildID,
	); err != nil {
		t.Fatalf("mark terminal-failure quest guild-priority: %v", err)
	}

	questWatcher, err := entityStates.Watch(ctx, questID)
	if err != nil {
		t.Fatalf("watch terminal-failure quest: %v", err)
	}
	t.Cleanup(func() {
		if err := questWatcher.Stop(); err != nil {
			t.Errorf("stop terminal-failure quest watcher: %v", err)
		}
	})
	awaitInitialKVSnapshot(t, ctx, questWatcher.Updates())

	if err := manager.Transition(
		ctx,
		questlifecycle.WorkflowName,
		questID,
		questlifecycle.PhaseFailed,
		lifecycle.TransitionSourceOperator,
		"Phase-1 terminal failure quarantine proof",
	); err != nil {
		t.Fatalf("transition claimed quest to terminal failed: %v", err)
	}
	awaitQuestPhase(t, ctx, questWatcher.Updates(), questID, questlifecycle.PhaseFailed)

	assertEntityKVUnchanged(t, ctx, entityStates, proof.agentID, proof.agentBefore)
	assertEntityKVUnchanged(t, ctx, entityStates, proof.guildID, proof.guildBefore)
	assertNoMechanicsSideEffects(t, ctx, entityStates, questID, proof.agentID, proof.guildID)
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
		"rules_files":              rules.Config.RulesFiles,
		"pack_id":                  rules.Config.PackID,
		"enable_graph_integration": rules.Config.EnableGraphIntegration,
		"entity_watch_buckets":     rules.Config.EntityWatchBuckets,
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
			if state.ID == questID && hasPhase && phase == questlifecycle.PhaseCompleted {
				if !hasVerdict || verdict != "review-skipped-mechanics-disabled" {
					t.Fatalf(
						"observed completed quest without atomic mechanics-stub verdict: verdict=%v present=%v",
						verdict,
						hasVerdict,
					)
				}
				return state
			}
		case <-ctx.Done():
			t.Fatalf("wait for mechanics-disabled quest completion: %v", ctx.Err())
		}
	}
}

func awaitQuestPhase(
	t *testing.T,
	ctx context.Context,
	updates <-chan jetstream.KeyValueEntry,
	questID string,
	wantPhase string,
) *graph.EntityState {
	t.Helper()
	for {
		select {
		case entry, ok := <-updates:
			if !ok {
				t.Fatalf("quest KV watcher closed before %s", wantPhase)
			}
			if entry == nil {
				continue
			}
			state, err := semdragons.DecodeEntityState(entry)
			if err != nil {
				t.Fatalf("decode quest EntityState: %v", err)
			}
			phase, hasPhase := state.GetPropertyValue(questlifecycle.PhasePredicate)
			if state.ID == questID && hasPhase && phase == wantPhase {
				return state
			}
		case <-ctx.Done():
			t.Fatalf("wait for quest phase %s: %v", wantPhase, ctx.Err())
		}
	}
}

type progressionProofEntities struct {
	agentID     string
	guildID     string
	agentBefore jetstream.KeyValueEntry
	guildBefore jetstream.KeyValueEntry
}

func seedProgressionProofEntities(
	t *testing.T,
	ctx context.Context,
	client *natsclient.Client,
	entityStates jetstream.KeyValue,
	questID string,
	suffix string,
) progressionProofEntities {
	t.Helper()

	agentID := "local.integration.game.phase1.agent.progression-proof-" + suffix
	guildID := "local.integration.game.phase1.guild.progression-proof-" + suffix
	seededAt := time.Unix(1_700_000_000, 0).UTC()
	seededQuestID := domain.QuestID(questID)

	boardConfig := &domain.BoardConfig{Org: "local", Platform: "integration", Board: "phase1"}
	graphClient := semdragons.NewGraphClient(client, boardConfig)
	if err := graphClient.EnsureBucket(ctx); err != nil {
		t.Fatalf("initialize graph client: %v", err)
	}

	agent := &agentprogression.Agent{
		ID:           domain.AgentID(agentID),
		Name:         "progression-proof",
		DisplayName:  "Progression Proof",
		Status:       domain.AgentOnQuest,
		Level:        11,
		XP:           321,
		XPToLevel:    600,
		DeathCount:   2,
		Tier:         domain.TierExpert,
		Guild:        domain.GuildID(guildID),
		CurrentQuest: &seededQuestID,
		OwnedTools: map[string]agentprogression.OwnedTool{
			"proof-tool": {
				StoreItemID:   "local.integration.game.phase1.storeitem.proof-tool",
				XPSpent:       75,
				UsesRemaining: 4,
				PurchasedAt:   seededAt,
			},
		},
		Consumables: map[string]int{"proof-potion": 3},
		TotalSpent:  125,
		ActiveEffects: []agentprogression.AgentEffect{{
			EffectType:      "xp_boost",
			QuestsRemaining: 2,
			QuestID:         &seededQuestID,
		}},
		Stats: agentprogression.AgentStats{
			QuestsCompleted: 9,
			QuestsFailed:    2,
			TotalXPEarned:   1_234,
			PeerReviewAvg:   4.25,
			PeerReviewQ1Avg: 4,
			PeerReviewQ2Avg: 4.5,
			PeerReviewQ3Avg: 4.25,
			PeerReviewCount: 8,
		},
		CreatedAt: seededAt,
		UpdatedAt: seededAt,
	}
	if err := graphClient.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("seed progression-proof agent: %v", err)
	}

	guild := &domain.Guild{
		ID:     domain.GuildID(guildID),
		Name:   "Progression Proof Guild",
		Status: domain.GuildActive,
		Members: []domain.GuildMember{{
			AgentID: domain.AgentID(agentID), Rank: domain.GuildRankMember, Contribution: 42,
		}},
		Founded:       seededAt,
		FoundedBy:     domain.AgentID(agentID),
		Reputation:    0.75,
		QuestsHandled: 12,
		QuestsFailed:  3,
		SuccessRate:   0.75,
		CreatedAt:     seededAt,
	}
	if err := graphClient.PutEntityState(ctx, guild, "guild.created"); err != nil {
		t.Fatalf("seed progression-proof guild: %v", err)
	}

	return progressionProofEntities{
		agentID:     agentID,
		guildID:     guildID,
		agentBefore: mustGetKVEntry(t, ctx, entityStates, agentID),
		guildBefore: mustGetKVEntry(t, ctx, entityStates, guildID),
	}
}

func assertNoMechanicsSideEffects(
	t *testing.T,
	ctx context.Context,
	entityStates jetstream.KeyValue,
	questID string,
	unchangedEntityIDs ...string,
) {
	t.Helper()
	expectedKeys := map[string]struct{}{questID: {}}
	for _, entityID := range unchangedEntityIDs {
		expectedKeys[entityID] = struct{}{}
	}

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
		if _, expected := expectedKeys[key]; !expected {
			t.Errorf("mechanics-disabled review created unexpected side-effect entity %q", key)
		}
		if key != questID {
			continue
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

func mustGetKVEntry(
	t *testing.T,
	ctx context.Context,
	bucket jetstream.KeyValue,
	key string,
) jetstream.KeyValueEntry {
	t.Helper()
	entry, err := bucket.Get(ctx, key)
	if err != nil {
		t.Fatalf("read authoritative entity %q: %v", key, err)
	}
	return entry
}

func assertEntityKVUnchanged(
	t *testing.T,
	ctx context.Context,
	bucket jetstream.KeyValue,
	key string,
	before jetstream.KeyValueEntry,
) {
	t.Helper()
	after := mustGetKVEntry(t, ctx, bucket, key)
	if after.Revision() != before.Revision() {
		t.Errorf(
			"mechanics-disabled terminal quest changed %q revision from %d to %d",
			key,
			before.Revision(),
			after.Revision(),
		)
	}
	if !bytes.Equal(after.Value(), before.Value()) {
		t.Errorf("mechanics-disabled terminal quest changed authoritative state for %q", key)
	}
}
