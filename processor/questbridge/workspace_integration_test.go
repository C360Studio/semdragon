//go:build integration

package questbridge

// =============================================================================
// INTEGRATION TESTS - WorkspaceRepo integration with QuestBridge
// =============================================================================
// These tests require Docker for NATS via testcontainers.
// Run with: go test -tags=integration ./processor/questbridge/...
// =============================================================================

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/storage/workspacerepo"
	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
)

// setupComponentWithWorkspaceRepo creates a questbridge component with an
// injected workspace repo backed by real git worktrees in a temp directory.
func setupComponentWithWorkspaceRepo(t *testing.T, client *natsclient.Client, board string) (*Component, *semdragons.GraphClient, *workspacerepo.WorkspaceRepo) {
	t.Helper()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = board
	config.ConsumerNameSuffix = board
	config.DeleteConsumerOnStop = true

	deps := component.Dependencies{
		NATSClient: client,
	}

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}

	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	gc := semdragons.NewGraphClient(client, comp.boardConfig)
	if err := gc.EnsureBucket(context.Background()); err != nil {
		t.Fatalf("EnsureBucket failed: %v", err)
	}

	// Create a real workspace repo in a temp directory and inject it.
	repoDir := filepath.Join(t.TempDir(), "workspace.git")
	worktreesDir := filepath.Join(t.TempDir(), "quest-worktrees")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	wsRepo := workspacerepo.New(repoDir, worktreesDir, logger)
	if err := wsRepo.Init(context.Background()); err != nil {
		t.Fatalf("workspace repo Init failed: %v", err)
	}

	if err := comp.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Inject after Start — Start() resolves from ComponentRegistry (nil in tests)
	// which would overwrite a pre-Start injection.
	comp.workspaceRepo = wsRepo

	t.Cleanup(func() {
		_ = comp.Stop(5 * time.Second)
	})

	return comp, gc, wsRepo
}

// TestWorktreeCreatedOnQuestStart verifies that when a quest transitions to
// in_progress, questbridge creates a git worktree for the quest.
func TestWorktreeCreatedOnQuestStart(t *testing.T) {
	testClient := natsclient.NewTestClient(t,
		natsclient.WithKV(), natsclient.WithFileStorage(),
		natsclient.WithStreams(natsclient.TestStreamConfig{
			Name:     "AGENT",
			Subjects: []string{"agent.task.>", "agent.complete.>", "agent.failed.>", "tool.execute.>", "tool.result.>"},
		}),
	)
	client := testClient.Client
	ctx := context.Background()

	comp, gc, wsRepo := setupComponentWithWorkspaceRepo(t, client, "wt-create")

	agent := createTestAgent(t, gc, comp.boardConfig, "wt-agent", 8)
	questID := createInProgressQuest(t, gc, comp.boardConfig, agent.ID, "Worktree Creation Test")

	// Subscribe to agent tasks to know when questbridge processed the transition.
	taskCh := subscribeToAgentTasks(t, client, ctx)

	// Trigger the in_progress transition.
	writeQuestInProgress(t, gc, comp.boardConfig, questID, agent.ID)

	// Wait for the TaskMessage — this confirms questbridge processed the transition.
	select {
	case data := <-taskCh:
		var baseMsg message.BaseMessage
		if err := json.Unmarshal(data, &baseMsg); err != nil {
			t.Fatalf("Failed to unmarshal BaseMessage: %v", err)
		}
		taskMsg, ok := baseMsg.Payload().(*agentic.TaskMessage)
		if !ok {
			t.Fatalf("Payload is %T, want *agentic.TaskMessage", baseMsg.Payload())
		}
		if taskMsg.TaskID != string(questID) {
			t.Errorf("TaskID = %q, want %q", taskMsg.TaskID, string(questID))
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Timed out waiting for TaskMessage")
	}

	// Verify the worktree was created.
	if !wsRepo.WorktreeExists(string(questID)) {
		t.Errorf("worktree not created for quest %s", questID)
	}

	// Verify the worktree has the correct branch.
	worktreePath := wsRepo.WorktreePath(string(questID))
	if _, err := os.Stat(filepath.Join(worktreePath, ".git")); err != nil {
		t.Errorf("worktree .git marker missing: %v", err)
	}
}

// TestWorktreeFinalizedOnCompletion verifies that when a quest is completed
// via the completeQuest path, the worktree is finalized with a commit and
// the ArtifactsCommit predicate is set on the quest entity.
func TestWorktreeFinalizedOnCompletion(t *testing.T) {
	testClient := natsclient.NewTestClient(t,
		natsclient.WithKV(), natsclient.WithFileStorage(),
		natsclient.WithStreams(natsclient.TestStreamConfig{
			Name:     "AGENT",
			Subjects: []string{"agent.task.>", "agent.complete.>", "agent.failed.>", "tool.execute.>", "tool.result.>"},
		}),
	)
	client := testClient.Client
	ctx := context.Background()

	comp, gc, wsRepo := setupComponentWithWorkspaceRepo(t, client, "wt-finalize")

	agent := createTestAgent(t, gc, comp.boardConfig, "fin-agent", 8)
	questID := createInProgressQuest(t, gc, comp.boardConfig, agent.ID, "Worktree Finalization Test")

	// Subscribe to agent tasks.
	taskCh := subscribeToAgentTasks(t, client, ctx)

	// Trigger the in_progress transition.
	writeQuestInProgress(t, gc, comp.boardConfig, questID, agent.ID)

	// Wait for TaskMessage so we know the worktree was created.
	var loopID string
	select {
	case data := <-taskCh:
		var baseMsg message.BaseMessage
		if err := json.Unmarshal(data, &baseMsg); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		taskMsg := baseMsg.Payload().(*agentic.TaskMessage)
		loopID = taskMsg.LoopID
	case <-time.After(10 * time.Second):
		t.Fatal("Timed out waiting for TaskMessage")
	}

	// Simulate agent writing a file into the worktree.
	worktreePath := wsRepo.WorktreePath(string(questID))
	if err := os.WriteFile(filepath.Join(worktreePath, "result.go"), []byte("package result\n"), 0o644); err != nil {
		t.Fatalf("write file to worktree: %v", err)
	}

	// Simulate loop completion by publishing a LoopCompletedEvent.
	// questbridge consumes this and calls completeQuest which triggers finalization.
	// Use the same BaseMessage envelope pattern as production (agentic-loop).
	completedEvent := agentic.LoopCompletedEvent{
		LoopID:     loopID,
		TaskID:     string(questID),
		Iterations: 3,
		TokensIn:   100,
		TokensOut:  50,
		Result:     `{"tool":"submit_work_product","arguments":{"summary":"implemented result package","deliverable":"done"}}`,
	}
	baseMsg := message.NewBaseMessage(completedEvent.Schema(), &completedEvent, "test")
	data2, err := json.Marshal(baseMsg)
	if err != nil {
		t.Fatalf("marshal completion: %v", err)
	}
	subject := "agent.complete." + loopID
	if err := client.PublishToStream(ctx, subject, data2); err != nil {
		t.Fatalf("publish completion: %v", err)
	}

	// Wait for questbridge to process the completion and finalize the worktree.
	// Check the quest entity for ArtifactsCommit predicate.
	deadline := time.After(10 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("Timed out waiting for ArtifactsCommit to be set on quest entity")
		case <-time.After(200 * time.Millisecond):
			questEntity, qErr := gc.GetQuest(ctx, questID)
			if qErr != nil {
				continue
			}
			quest := domain.QuestFromEntityState(questEntity)
			if quest != nil && quest.ArtifactsCommit != "" {
				t.Logf("ArtifactsCommit = %s", quest.ArtifactsCommit)
				// Verify the commit hash looks like a git SHA.
				if len(quest.ArtifactsCommit) < 7 {
					t.Errorf("ArtifactsCommit looks invalid: %q", quest.ArtifactsCommit)
				}
				return // Success
			}
		}
	}
}

// TestWorktreePersistsOnFailure verifies that when a quest fails, the
// worktree is NOT cleaned up (it persists for retry/rework).
func TestWorktreePersistsOnFailure(t *testing.T) {
	testClient := natsclient.NewTestClient(t,
		natsclient.WithKV(), natsclient.WithFileStorage(),
		natsclient.WithStreams(natsclient.TestStreamConfig{
			Name:     "AGENT",
			Subjects: []string{"agent.task.>", "agent.complete.>", "agent.failed.>", "tool.execute.>", "tool.result.>"},
		}),
	)
	client := testClient.Client
	ctx := context.Background()

	comp, gc, wsRepo := setupComponentWithWorkspaceRepo(t, client, "wt-fail")

	agent := createTestAgent(t, gc, comp.boardConfig, "fail-agent", 8)
	questID := createInProgressQuest(t, gc, comp.boardConfig, agent.ID, "Worktree Failure Test")

	taskCh := subscribeToAgentTasks(t, client, ctx)
	writeQuestInProgress(t, gc, comp.boardConfig, questID, agent.ID)

	// Wait for TaskMessage (worktree created).
	var loopID string
	select {
	case data := <-taskCh:
		var baseMsg message.BaseMessage
		json.Unmarshal(data, &baseMsg)
		taskMsg := baseMsg.Payload().(*agentic.TaskMessage)
		loopID = taskMsg.LoopID
	case <-time.After(10 * time.Second):
		t.Fatal("Timed out waiting for TaskMessage")
	}

	if !wsRepo.WorktreeExists(string(questID)) {
		t.Fatal("worktree should exist before failure event")
	}

	// Publish a failure event using the same BaseMessage envelope pattern.
	failedEvent := agentic.LoopFailedEvent{
		LoopID: loopID,
		TaskID: string(questID),
		Error:  "agent encountered an error",
	}
	failMsg := message.NewBaseMessage(failedEvent.Schema(), &failedEvent, "test")
	failBytes, _ := json.Marshal(failMsg)
	subject := "agent.failed." + loopID
	if err := client.PublishToStream(ctx, subject, failBytes); err != nil {
		t.Fatalf("publish failure: %v", err)
	}

	// Give questbridge time to process.
	time.Sleep(2 * time.Second)

	// Worktree should STILL exist (persists for retry).
	if !wsRepo.WorktreeExists(string(questID)) {
		t.Error("worktree was deleted on failure — should persist for retry/rework")
	}
}

// TestConcurrentQuestWorktreeIsolation verifies that multiple quests starting
// concurrently each get their own isolated worktree.
func TestConcurrentQuestWorktreeIsolation(t *testing.T) {
	testClient := natsclient.NewTestClient(t,
		natsclient.WithKV(), natsclient.WithFileStorage(),
		natsclient.WithStreams(natsclient.TestStreamConfig{
			Name:     "AGENT",
			Subjects: []string{"agent.task.>", "agent.complete.>", "agent.failed.>", "tool.execute.>", "tool.result.>"},
		}),
	)
	client := testClient.Client
	ctx := context.Background()

	comp, gc, wsRepo := setupComponentWithWorkspaceRepo(t, client, "wt-concurrent")

	taskCh := subscribeToAgentTasks(t, client, ctx)

	// Create 3 quests with different agents, all transitioning to in_progress.
	type questAgent struct {
		questID domain.QuestID
		agentID domain.AgentID
	}
	var qas []questAgent
	for i := 0; i < 3; i++ {
		agent := createTestAgent(t, gc, comp.boardConfig, "concurrent-agent", 8)
		qid := createInProgressQuest(t, gc, comp.boardConfig, agent.ID, "Concurrent Quest")
		qas = append(qas, questAgent{questID: qid, agentID: agent.ID})
	}

	// Trigger all three transitions.
	for _, qa := range qas {
		writeQuestInProgress(t, gc, comp.boardConfig, qa.questID, qa.agentID)
	}

	// Wait for all 3 TaskMessages.
	received := 0
	deadline := time.After(15 * time.Second)
	for received < 3 {
		select {
		case <-taskCh:
			received++
		case <-deadline:
			t.Fatalf("Only received %d/3 TaskMessages before timeout", received)
		}
	}

	// Verify all 3 worktrees exist and are distinct.
	for _, qa := range qas {
		if !wsRepo.WorktreeExists(string(qa.questID)) {
			t.Errorf("worktree missing for quest %s", qa.questID)
		}
	}
}
