//go:build integration

package questbridge

// =============================================================================
// INTEGRATION TESTS — questbridge submit/fail command → rule → lifecycle (2.6)
// =============================================================================
// Confirms the Stage B1 rewire end to end: when an agentic loop completes or
// fails, questbridge no longer writes a terminal quest status directly. It
// stamps quest.data.output / quest.failure.reason and a quest.command trigger as
// additive graph-ingest deltas, and the real rule stack routes the quest to its
// next phase:
//
//   - loop completed, review not required -> submit command -> completed
//   - loop failed, attempts remaining      -> fail command   -> posted (retry)
//
// Unlike the counter tests (TestLoop{Completion,Failure}EmitsExecutorEvent), which
// only assert the loops{Completed,Failed} counter and run without graph-ingest, these
// stand up the full lifecycle stack (graph-ingest + rule-processor + Manager) so the
// stamped command actually fires a lifecycle_transition and we can assert the quest's
// resulting phase — the guarantee 2.5/2.6 deliver.
//
// Requires Docker (NATS via testcontainers).
// =============================================================================

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/questlifecycle"
	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/lifecycle"
	graphingest "github.com/c360studio/semstreams/processor/graph-ingest"
	rule "github.com/c360studio/semstreams/processor/rule"
	"github.com/nats-io/nats.go/jetstream"
)

// TestPhase1ExplicitLifecycleClaimCompletesThroughQuestBridge proves the complete
// Phase-1 positive path without a boid producer: a posted quest stays dormant,
// explicit lifecycle claim/start dispatches questbridge, loop completion stamps
// submit, and the shipped mechanics-stub rule completes review without creating
// battle, progression, store, or red-team side effects.
func TestPhase1ExplicitLifecycleClaimCompletesThroughQuestBridge(t *testing.T) {
	client := newLifecycleTestClient(t)
	ctx := context.Background()

	mgr := startQuestGraphAndLifecycle(t, client)
	startQuestRuleProcessor(t, client, mgr)

	comp, gc := setupComponent(t, client, "phase1explicit")
	taskCh := subscribeToAgentTasks(t, client, ctx)

	agent := createTestAgent(t, gc, comp.boardConfig, "explicit-agent", 12)
	questID := domain.QuestID(comp.boardConfig.QuestEntityID(domain.GenerateInstance()))
	guildID := domain.GuildID(comp.boardConfig.GuildEntityID(domain.GenerateInstance()))
	seededAt := time.Unix(1_700_000_000, 0).UTC()
	quest := &domain.Quest{
		ID:          questID,
		Title:       "Explicit Phase-1 Claim",
		Description: "Must remain posted until an explicit lifecycle claim",
		Status:      domain.QuestPosted,
		Difficulty:  domain.DifficultyTrivial,
		BaseXP:      100,
		MaxAttempts: 3,
		PostedAt:    seededAt,
		Constraints: domain.QuestConstraints{RequireReview: true},
	}
	guild := &domain.Guild{
		ID:         guildID,
		Name:       "Stable Phase-1 Guild",
		Status:     domain.GuildActive,
		MaxMembers: 10,
		MinLevel:   1,
		Founded:    seededAt,
		FoundedBy:  agent.ID,
		CreatedAt:  seededAt,
	}
	if err := gc.PutEntityState(ctx, quest, "quest.posted"); err != nil {
		t.Fatalf("seed posted quest: %v", err)
	}
	if err := gc.PutEntityState(ctx, guild, "guild.created"); err != nil {
		t.Fatalf("seed guild: %v", err)
	}

	waitForQuestCacheStatus(t, comp, string(questID), string(domain.QuestPosted), 5*time.Second)
	entityStates, err := client.GetKeyValueBucket(ctx, graph.BucketEntityStates)
	if err != nil {
		t.Fatalf("get authoritative entity states: %v", err)
	}
	before := snapshotEntityStates(t, ctx, entityStates, string(agent.ID), string(questID), string(guildID))
	assertEntityStatesEqual(t, ctx, entityStates, before)
	select {
	case <-taskCh:
		t.Fatal("posted quest dispatched before explicit lifecycle claim/start")
	default:
	}

	if err := questlifecycle.StampCommand(
		ctx,
		client,
		string(questID),
		questlifecycle.CommandClaim,
		map[string]string{questlifecycle.PredClaimRequestedBy: string(agent.ID)},
	); err != nil {
		t.Fatalf("stamp explicit lifecycle claim command: %v", err)
	}
	claimed := waitForQuestStatus(t, gc, ctx, questID, domain.QuestClaimed, 20*time.Second)
	if claimed.ClaimedBy == nil || *claimed.ClaimedBy != agent.ID {
		t.Fatalf("claimed_by = %v, want %q", claimed.ClaimedBy, agent.ID)
	}
	if claimed.Attempts != 1 {
		t.Fatalf("attempts = %d, want atomic claim increment to 1", claimed.Attempts)
	}
	waitForQuestCacheStatus(t, comp, string(questID), string(domain.QuestClaimed), 5*time.Second)
	if err := mgr.Transition(
		ctx,
		questlifecycle.WorkflowName,
		string(questID),
		questlifecycle.PhaseInProgress,
		lifecycle.TransitionSourceOperator,
		"explicit Phase-1 start",
	); err != nil {
		t.Fatalf("explicit lifecycle start: %v", err)
	}
	waitForQuestStatus(t, gc, ctx, questID, domain.QuestInProgress, 20*time.Second)

	select {
	case <-taskCh:
	case <-time.After(8 * time.Second):
		t.Fatal("timed out waiting for questbridge task after explicit lifecycle start")
	}
	mapping := waitForQuestLoopsMapping(t, comp, ctx, string(questID), 5*time.Second)
	completed := agentic.LoopCompletedEvent{
		LoopID:     mapping.LoopID,
		TaskID:     string(questID),
		Iterations: 2,
		TokensIn:   80,
		TokensOut:  40,
		Result:     "explicit lifecycle deliverable",
	}
	publishLoopEvent(t, client, ctx, "agent.complete."+mapping.LoopID, completed.Schema(), &completed)

	final := waitForQuestStatus(t, gc, ctx, questID, domain.QuestCompleted, 20*time.Second)
	if got := domain.AsString(final.Output); got != "explicit lifecycle deliverable" {
		t.Errorf("quest output = %q, want explicit lifecycle deliverable", got)
	}
	if final.Verdict == nil || final.Verdict.Feedback != "review-skipped-mechanics-disabled" {
		t.Fatalf("quest verdict = %#v, want mechanics-disabled review marker", final.Verdict)
	}
	if final.Verdict.XPAwarded != 0 || final.Verdict.XPPenalty != 0 {
		t.Errorf("mechanics-disabled verdict awarded XP: %#v", final.Verdict)
	}
	assertEntityStatesEqual(t, ctx, entityStates, map[string]entityStateSnapshot{
		string(agent.ID): before[string(agent.ID)],
		string(guildID):  before[string(guildID)],
	})
	assertNoPhase1MechanicsSideEffects(
		t,
		ctx,
		entityStates,
		string(questID),
		string(agent.ID),
		string(guildID),
	)
}

// TestQuestBridgeSubmitCommandCompletesQuest drives a full loop-completion through
// the rewired seam: writeQuestInProgress -> questbridge dispatches -> loop completes
// -> questbridge stamps quest.data.output + quest.command=submit -> the no-review
// submit rule fires in_progress -> completed. Proves the completeQuest rewire routes
// through the rules rather than a direct terminal Put, and preserves the work product.
func TestQuestBridgeSubmitCommandCompletesQuest(t *testing.T) {
	client := newLifecycleTestClient(t)
	ctx := context.Background()

	startQuestRulesStack(t, client)

	comp, gc := setupComponent(t, client, "submitrule")
	defer comp.Stop(5 * time.Second)

	agent := createTestAgent(t, gc, comp.boardConfig, "submit-agent", 7)
	questID := createInProgressQuest(t, gc, comp.boardConfig, agent.ID, "Submit Rule Quest")

	taskCh := subscribeToAgentTasks(t, client, ctx)
	writeQuestInProgress(t, gc, comp.boardConfig, questID, agent.ID)

	select {
	case <-taskCh:
	case <-time.After(8 * time.Second):
		t.Fatal("timed out waiting for TaskMessage before publishing completion")
	}

	mapping := waitForQuestLoopsMapping(t, comp, ctx, string(questID), 5*time.Second)

	completed := agentic.LoopCompletedEvent{
		LoopID:     mapping.LoopID,
		TaskID:     string(questID),
		Iterations: 2,
		TokensIn:   80,
		TokensOut:  40,
		Result:     "the finished work product",
	}
	publishLoopEvent(t, client, ctx, "agent.complete."+mapping.LoopID, completed.Schema(), &completed)

	final := waitForQuestStatus(t, gc, ctx, questID, domain.QuestCompleted, 20*time.Second)
	if got := domain.AsString(final.Output); got != "the finished work product" {
		t.Errorf("quest.data.output = %q, want the submitted work product to survive the transition", got)
	}
}

// TestQuestBridgeFailCommandRetriesQuest drives a full loop-failure through the
// rewired seam: the quest has one attempt used (< max), the loop fails, questbridge
// stamps quest.command=fail, and the fail-retry rule routes in_progress -> posted,
// clearing claimed_by. Proves the failQuest rewire delegates routing to the rules
// (retry vs triage) instead of writing a terminal failed status directly.
func TestQuestBridgeFailCommandRetriesQuest(t *testing.T) {
	client := newLifecycleTestClient(t)
	ctx := context.Background()

	mgr := startQuestGraphAndLifecycle(t, client)

	comp, gc := setupComponent(t, client, "failrule")
	defer comp.Stop(5 * time.Second)

	agent := createTestAgent(t, gc, comp.boardConfig, "fail-agent", 7)
	questID := createInProgressQuest(t, gc, comp.boardConfig, agent.ID, "Fail Rule Quest")
	agent.Status = domain.AgentOnQuest
	agent.CurrentQuest = &questID
	agent.UpdatedAt = time.Now()
	if err := gc.EmitEntityUpdate(ctx, agent, "agent.status.on_quest"); err != nil {
		t.Fatalf("set agent on quest: %v", err)
	}
	waitForAgentAssignment(t, gc, ctx, agent.ID, domain.AgentOnQuest, &questID, 5*time.Second)

	taskCh := subscribeToAgentTasks(t, client, ctx)
	writeQuestInProgress(t, gc, comp.boardConfig, questID, agent.ID)

	select {
	case <-taskCh:
	case <-time.After(8 * time.Second):
		t.Fatal("timed out waiting for TaskMessage before publishing failure")
	}

	mapping := waitForQuestLoopsMapping(t, comp, ctx, string(questID), 5*time.Second)
	questUpdates, err := gc.WatchEntity(ctx, string(questID))
	if err != nil {
		t.Fatalf("watch quest: %v", err)
	}
	defer questUpdates.Stop()

	failed := agentic.LoopFailedEvent{
		LoopID:     mapping.LoopID,
		TaskID:     string(questID),
		Error:      "max iterations exceeded",
		Iterations: 20,
	}
	publishLoopEvent(t, client, ctx, "agent.failed."+mapping.LoopID, failed.Schema(), &failed)

	// With only graph-ingest running, observe the fail command before any rule
	// can authoritatively route the quest. The agent must remain assigned while
	// the quest is still in_progress.
	commandCtx, cancelCommand := context.WithTimeout(ctx, 5*time.Second)
	defer cancelCommand()
	waitForQuestFailCommand(t, commandCtx, questUpdates.Updates(), questID)
	waitForAgentAssignment(t, gc, ctx, agent.ID, domain.AgentOnQuest, &questID, 5*time.Second)

	// Attach the real rule processor after the ordering assertion. Its initial
	// entity scan sees the fail command and performs the retry transition.
	startQuestRuleProcessor(t, client, mgr)

	// attempts=1 (< max 3) -> the fail-retry rule reposts the quest.
	final := waitForQuestStatus(t, gc, ctx, questID, domain.QuestPosted, 20*time.Second)
	if final.ClaimedBy != nil && *final.ClaimedBy != "" {
		t.Errorf("claimed_by = %v, want cleared by the fail-retry rule", *final.ClaimedBy)
	}
	waitForAgentAssignment(t, gc, ctx, agent.ID, domain.AgentIdle, nil, 5*time.Second)
}

func TestFailQuest_DoesNotReleaseAgentUntilAuthoritativePhaseTransition(t *testing.T) {
	client := newLifecycleTestClient(t)
	ctx := context.Background()

	startLifecycleComponent(t, ctx, mustMarshal(t, graphingest.DefaultConfig()), func(cfg json.RawMessage) (component.Discoverable, error) {
		return graphingest.CreateGraphIngest(cfg, component.Dependencies{NATSClient: client})
	})

	comp, gc := setupComponent(t, client, "failwait")
	defer comp.Stop(5 * time.Second)

	agent := createTestAgent(t, gc, comp.boardConfig, "waiting-agent", 7)
	questID := createInProgressQuestDirect(t, gc, comp.boardConfig, agent.ID, "Await Rule Transition")
	agent.Status = domain.AgentOnQuest
	agent.CurrentQuest = &questID
	agent.UpdatedAt = time.Now()
	if err := gc.EmitEntityUpdate(ctx, agent, "agent.status.on_quest"); err != nil {
		t.Fatalf("set agent on quest: %v", err)
	}

	questUpdates, err := gc.WatchEntity(ctx, string(questID))
	if err != nil {
		t.Fatalf("watch quest: %v", err)
	}
	defer questUpdates.Stop()

	failCtx, cancelFail := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		comp.failQuest(failCtx, questID, &QuestLoopMapping{
			QuestID:   questID,
			AgentID:   agent.ID,
			LoopID:    "loop-await-rule",
			StartedAt: time.Now(),
		}, "injected failure", loopMetrics{})
	}()

	commandCtx, cancelCommand := context.WithTimeout(ctx, 5*time.Second)
	defer cancelCommand()
	waitForQuestFailCommand(t, commandCtx, questUpdates.Updates(), questID)

	entity, err := gc.GetAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetAgent after fail command ingestion: %v", err)
	}
	stored := agentprogression.AgentFromEntityState(entity)
	if stored == nil {
		t.Fatal("reconstruct agent after fail command ingestion")
	}
	if stored.Status != domain.AgentOnQuest || stored.CurrentQuest == nil || *stored.CurrentQuest != questID {
		t.Fatalf("agent released before quest transitioned: status=%q current_quest=%v", stored.Status, stored.CurrentQuest)
	}

	cancelFail()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("failQuest did not stop after its bounded context was cancelled")
	}
}

// =============================================================================
// HELPERS
// =============================================================================

// newLifecycleTestClient builds a NATS test client with both the ENTITY stream
// (graph-ingest input) and the AGENT stream (questbridge dispatch/completion), plus
// the ENTITY_STATES bucket the lifecycle stack and GraphClient share.
func newLifecycleTestClient(t *testing.T) *natsclient.Client {
	t.Helper()
	tc := natsclient.NewTestClient(t,
		natsclient.WithKV(), natsclient.WithFileStorage(),
		natsclient.WithKVBuckets(graph.BucketEntityStates),
		natsclient.WithStreams(
			natsclient.TestStreamConfig{Name: "ENTITY", Subjects: []string{"entity.>"}},
			natsclient.TestStreamConfig{
				Name:     "AGENT",
				Subjects: []string{"agent.task.>", "agent.complete.>", "agent.failed.>", "tool.execute.>", "tool.result.>"},
			},
		),
	)
	return tc.Client
}

// startQuestRulesStack stands up the real lifecycle stack questbridge's command
// stamps route through: the lifecycle Manager with the quest workflow, graph-ingest
// (persists triple deltas into ENTITY_STATES), and the rule-processor loading
// config/rules/quest/*.json (fires lifecycle_transition via the Manager). Mirrors the
// recipe proven in questlifecycle/rules_fire_test.go. The Manager itself is not
// returned: questbridge drives transitions purely by stamping commands, and the rules
// carry them out — no direct Manager calls on this path.
func startQuestRulesStack(t *testing.T, client *natsclient.Client) {
	t.Helper()
	mgr := startQuestGraphAndLifecycle(t, client)
	startQuestRuleProcessor(t, client, mgr)
}

func startQuestGraphAndLifecycle(t *testing.T, client *natsclient.Client) *lifecycle.Manager {
	t.Helper()
	ctx := context.Background()
	mgr := lifecycle.NewManager(client, slog.Default())
	if err := mgr.Register(questlifecycle.WorkflowDeclaration()); err != nil {
		t.Fatalf("register quest workflow: %v", err)
	}

	startLifecycleComponent(t, ctx, mustMarshal(t, graphingest.DefaultConfig()), func(cfg json.RawMessage) (component.Discoverable, error) {
		return graphingest.CreateGraphIngest(cfg, component.Dependencies{NATSClient: client})
	})
	return mgr
}

func startQuestRuleProcessor(t *testing.T, client *natsclient.Client, mgr *lifecycle.Manager) {
	t.Helper()
	ctx := context.Background()
	ruleCfg := mustMarshal(t, map[string]any{
		"rules_files":              questRuleFilePaths(t),
		"pack_id":                  "quest-lifecycle-test",
		"enable_graph_integration": true,
		"entity_watch_buckets": map[string][]string{
			graph.BucketEntityStates: {"*.*.game.*.quest.*"},
		},
	})
	startLifecycleComponent(t, ctx, ruleCfg, func(cfg json.RawMessage) (component.Discoverable, error) {
		return rule.CreateRuleProcessor(cfg, component.Dependencies{NATSClient: client, LifecycleManager: mgr})
	})

	// Let the KV watchers attach before callers drive transitions.
	time.Sleep(750 * time.Millisecond)
}

// startLifecycleComponent creates, initializes, and starts a semstreams component,
// registering a cleanup that stops it. Used for graph-ingest and the rule-processor.
func startLifecycleComponent(t *testing.T, ctx context.Context, cfg json.RawMessage, create func(json.RawMessage) (component.Discoverable, error)) {
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

// publishLoopEvent wraps an agentic loop event in a BaseMessage (as agentic-loop
// does in production) and publishes it to the AGENT stream on the given subject.
func publishLoopEvent(t *testing.T, client *natsclient.Client, ctx context.Context, subject string, schema message.Type, event message.Payload) {
	t.Helper()
	data, err := json.Marshal(message.NewBaseMessage(schema, event, "test"))
	if err != nil {
		t.Fatalf("marshal loop event BaseMessage: %v", err)
	}
	if err := client.PublishToStream(ctx, subject, data); err != nil {
		t.Fatalf("publish loop event to %s: %v", subject, err)
	}
}

func waitForQuestFailCommand(t *testing.T, ctx context.Context, updates <-chan jetstream.KeyValueEntry, questID domain.QuestID) {
	t.Helper()
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("quest %q did not ingest fail command: %v", questID, ctx.Err())
		case entry, ok := <-updates:
			if !ok {
				t.Fatalf("quest %q watch closed before fail command ingestion", questID)
			}
			entity, err := semdragons.DecodeEntityState(entry)
			if err != nil {
				t.Fatalf("decode quest update: %v", err)
			}
			if entity == nil {
				continue
			}
			quest := domain.QuestFromEntityState(entity)
			if quest == nil || quest.Status != domain.QuestInProgress {
				continue
			}
			for _, triple := range entity.Triples {
				if triple.Predicate == "quest.command" && domain.AsString(triple.Object) == string(questlifecycle.CommandFail) {
					return
				}
			}
		}
	}
}

// waitForQuestStatus polls the quest entity until it reaches want, then returns the
// reconstructed quest; fails the test on timeout, reporting the last-seen status.
func waitForQuestStatus(t *testing.T, gc *semdragons.GraphClient, ctx context.Context, questID domain.QuestID, want domain.QuestStatus, timeout time.Duration) *domain.Quest {
	t.Helper()

	var last domain.QuestStatus
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		entity, err := gc.GetQuest(ctx, questID)
		if err == nil {
			if quest := domain.QuestFromEntityState(entity); quest != nil {
				last = quest.Status
				if quest.Status == want {
					return quest
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("quest %q did not reach %q within %s; last status %q", questID, want, timeout, last)
	return nil // unreachable
}

func waitForAgentAssignment(t *testing.T, gc *semdragons.GraphClient, ctx context.Context, agentID domain.AgentID, wantStatus domain.AgentStatus, wantQuest *domain.QuestID, timeout time.Duration) {
	t.Helper()

	var lastStatus domain.AgentStatus
	var lastQuest *domain.QuestID
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		entity, err := gc.GetAgent(ctx, agentID)
		if err == nil {
			agent := agentprogression.AgentFromEntityState(entity)
			if agent != nil {
				lastStatus = agent.Status
				lastQuest = agent.CurrentQuest
				statusMatches := agent.Status == wantStatus
				questMatches := (wantQuest == nil && agent.CurrentQuest == nil) ||
					(wantQuest != nil && agent.CurrentQuest != nil && *wantQuest == *agent.CurrentQuest)
				if statusMatches && questMatches {
					return
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("agent %q did not reach status=%q current_quest=%v within %s; last status=%q current_quest=%v",
		agentID, wantStatus, wantQuest, timeout, lastStatus, lastQuest)
}

type entityStateSnapshot struct {
	revision uint64
	value    []byte
}

func snapshotEntityStates(
	t *testing.T,
	ctx context.Context,
	bucket jetstream.KeyValue,
	entityIDs ...string,
) map[string]entityStateSnapshot {
	t.Helper()
	snapshots := make(map[string]entityStateSnapshot, len(entityIDs))
	for _, entityID := range entityIDs {
		entry, err := bucket.Get(ctx, entityID)
		if err != nil {
			t.Fatalf("read authoritative entity %q: %v", entityID, err)
		}
		snapshots[entityID] = entityStateSnapshot{
			revision: entry.Revision(),
			value:    append([]byte(nil), entry.Value()...),
		}
	}
	return snapshots
}

func assertEntityStatesEqual(
	t *testing.T,
	ctx context.Context,
	bucket jetstream.KeyValue,
	want map[string]entityStateSnapshot,
) {
	t.Helper()
	for entityID, snapshot := range want {
		entry, err := bucket.Get(ctx, entityID)
		if err != nil {
			t.Fatalf("read authoritative entity %q: %v", entityID, err)
		}
		if entry.Revision() != snapshot.revision || !bytes.Equal(entry.Value(), snapshot.value) {
			t.Errorf("authoritative entity %q changed unexpectedly", entityID)
		}
	}
}

func waitForQuestCacheStatus(
	t *testing.T,
	comp *Component,
	questID string,
	want string,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, ok := comp.questCache.Load(questID)
		if ok && status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("questbridge cache did not observe quest %q status %q", questID, want)
}

func assertNoPhase1MechanicsSideEffects(
	t *testing.T,
	ctx context.Context,
	bucket jetstream.KeyValue,
	questID string,
	expectedEntityIDs ...string,
) {
	t.Helper()
	expected := make(map[string]struct{}, len(expectedEntityIDs)+1)
	expected[questID] = struct{}{}
	for _, entityID := range expectedEntityIDs {
		expected[entityID] = struct{}{}
	}
	keys, err := bucket.Keys(ctx)
	if err != nil {
		t.Fatalf("list authoritative entity states: %v", err)
	}
	for _, key := range keys {
		if _, ok := expected[key]; !ok {
			t.Errorf("Phase-1 completion created unexpected side-effect entity %q", key)
		}
		if domain.IsBattleID(key) ||
			domain.ExtractType(key) == domain.EntityTypeStoreItem ||
			domain.ExtractType(key) == domain.EntityTypePeerReview {
			t.Errorf("Phase-1 completion created forbidden mechanics entity %q", key)
		}
		entry, getErr := bucket.Get(ctx, key)
		if getErr != nil {
			t.Fatalf("read authoritative entity %q: %v", key, getErr)
		}
		state, decodeErr := semdragons.DecodeEntityState(entry)
		if decodeErr != nil {
			t.Fatalf("decode authoritative entity %q: %v", key, decodeErr)
		}
		if key != questID {
			continue
		}
		for _, triple := range state.Triples {
			predicate := triple.Predicate
			if strings.HasPrefix(predicate, "agent.progression.xp") ||
				strings.HasPrefix(predicate, "agent.inventory.") ||
				strings.HasPrefix(predicate, "redteam.") ||
				strings.HasPrefix(predicate, "battle.") ||
				predicate == "quest.verdict.xp-awarded" {
				t.Errorf("Phase-1 completion wrote forbidden mechanics predicate %q on %q", predicate, key)
			}
		}
	}
}

// questRuleFilePaths returns the absolute paths to config/rules/quest/*.json,
// resolved relative to this test file so it works regardless of the test's CWD.
func questRuleFilePaths(t *testing.T) []string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	base := filepath.Join(filepath.Dir(thisFile), "..", "..", "config", "rules", "quest")
	return []string{
		filepath.Join(base, "01-claim-start.json"),
		filepath.Join(base, "02-submit.json"),
		filepath.Join(base, "03-fail.json"),
		filepath.Join(base, "04-mechanics-stub.json"),
	}
}

// mustMarshal JSON-encodes v or fails the test.
func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
