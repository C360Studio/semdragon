//go:build integration

package questbridge

// =============================================================================
// INTEGRATION TESTS - QuestBridge Component
// =============================================================================
// These tests require Docker for NATS via testcontainers.
// Run with: go test -tags=integration ./processor/questbridge/...
// =============================================================================

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go/jetstream"
)

// =============================================================================
// LIFECYCLE TESTS
// =============================================================================

// TestComponentLifecycle verifies the full Initialize → Start → Health → Stop
// cycle reports the correct status at each transition.
func TestComponentLifecycle(t *testing.T) {
	testClient := natsclient.NewTestClient(t,
		natsclient.WithKV(), natsclient.WithFileStorage(),
		natsclient.WithStreams(natsclient.TestStreamConfig{
			Name:     "AGENT",
			Subjects: []string{"agent.task.>", "agent.complete.>", "agent.failed.>", "tool.execute.>", "tool.result.>"},
		}),
	)
	client := testClient.Client
	ctx := context.Background()

	deps := component.Dependencies{
		NATSClient: client,
	}

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "lifecycle"
	config.ConsumerNameSuffix = "lifecycle"
	config.DeleteConsumerOnStop = true

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}

	// Initialize must succeed before Start.
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Verify Meta before starting.
	meta := comp.Meta()
	if meta.Name != ComponentName {
		t.Errorf("Meta.Name = %q, want %q", meta.Name, ComponentName)
	}
	if meta.Type != "processor" {
		t.Errorf("Meta.Type = %q, want %q", meta.Type, "processor")
	}

	// Component must report healthy after Start.
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	health := comp.Health()
	if !health.Healthy {
		t.Error("Component should be healthy after Start")
	}
	if health.Status != "running" {
		t.Errorf("Health.Status = %q, want %q", health.Status, "running")
	}

	// Component must report stopped after Stop.
	if err := comp.Stop(5 * time.Second); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	health = comp.Health()
	if health.Healthy {
		t.Error("Component should not be healthy after Stop")
	}
	if health.Status != "stopped" {
		t.Errorf("Health.Status = %q, want %q", health.Status, "stopped")
	}
}

// =============================================================================
// PORT / SCHEMA TESTS
// =============================================================================

// TestInputOutputPorts verifies the declared port names and AGENT stream bindings.
func TestInputOutputPorts(t *testing.T) {
	comp := &Component{}

	inputs := comp.InputPorts()
	if len(inputs) == 0 {
		t.Fatal("InputPorts must not be empty")
	}

	// Verify the loop-completions input port exists with AGENT stream binding.
	var foundCompletions bool
	for _, port := range inputs {
		if port.Name == "loop-completions" {
			foundCompletions = true
			if jsPort, ok := port.Config.(*component.JetStreamPort); ok {
				if jsPort.StreamName != "AGENT" {
					t.Errorf("loop-completions StreamName = %q, want %q", jsPort.StreamName, "AGENT")
				}
			} else {
				t.Error("loop-completions port config is not a JetStreamPort")
			}
		}
	}
	if !foundCompletions {
		t.Error("Missing loop-completions input port")
	}

	outputs := comp.OutputPorts()
	if len(outputs) == 0 {
		t.Fatal("OutputPorts must not be empty")
	}

	// Verify the agent-tasks output port exists with AGENT stream binding.
	var foundTasks bool
	for _, port := range outputs {
		if port.Name == "agent-tasks" {
			foundTasks = true
			if jsPort, ok := port.Config.(*component.JetStreamPort); ok {
				if jsPort.StreamName != "AGENT" {
					t.Errorf("agent-tasks StreamName = %q, want %q", jsPort.StreamName, "AGENT")
				}
			} else {
				t.Error("agent-tasks port config is not a JetStreamPort")
			}
		}
	}
	if !foundTasks {
		t.Error("Missing agent-tasks output port")
	}
}

// TestConfigSchema verifies the ConfigSchema declares all required fields.
func TestConfigSchema(t *testing.T) {
	comp := &Component{}
	schema := comp.ConfigSchema()

	requiredFields := []string{"org", "platform", "board"}
	for _, field := range requiredFields {
		found := false
		for _, req := range schema.Required {
			if req == field {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Field %q should be required in ConfigSchema", field)
		}
	}

	expectedProps := []string{"org", "platform", "board", "stream_name", "quest_loops_bucket", "max_iterations"}
	for _, prop := range expectedProps {
		if _, exists := schema.Properties[prop]; !exists {
			t.Errorf("Missing property %q in ConfigSchema", prop)
		}
	}
}

// =============================================================================
// TASK MESSAGE PUBLISHING
// =============================================================================

// TestQuestStartedPublishesTaskMessage verifies that when a quest transitions to
// in_progress, questbridge detects the KV change and publishes a TaskMessage to
// agent.task.{questID} on the AGENT JetStream stream.
func TestQuestStartedPublishesTaskMessage(t *testing.T) {
	testClient := natsclient.NewTestClient(t,
		natsclient.WithKV(), natsclient.WithFileStorage(),
		natsclient.WithStreams(natsclient.TestStreamConfig{
			Name:     "AGENT",
			Subjects: []string{"agent.task.>", "agent.complete.>", "agent.failed.>", "tool.execute.>", "tool.result.>"},
		}),
	)
	client := testClient.Client
	ctx := context.Background()

	comp, gc := setupComponent(t, client, "taskpublish")
	defer comp.Stop(5 * time.Second)

	// Create an agent and an in_progress quest.
	agent := createTestAgent(t, gc, comp.boardConfig, "task-agent", 8) // Level 8 = Journeyman
	questID := createInProgressQuest(t, gc, comp.boardConfig, agent.ID, "Publish Test Quest")

	// Subscribe to the AGENT stream to capture the TaskMessage.
	taskCh := subscribeToAgentTasks(t, client, ctx)

	// Trigger: write the quest as in_progress so the live-update path fires.
	writeQuestInProgress(t, gc, comp.boardConfig, questID, agent.ID)

	// Wait for the TaskMessage to arrive.
	// The component wraps TaskMessage in a BaseMessage envelope before publishing.
	var taskMsg *agentic.TaskMessage
	select {
	case data := <-taskCh:
		var baseMsg message.BaseMessage
		if err := json.Unmarshal(data, &baseMsg); err != nil {
			t.Fatalf("Failed to unmarshal BaseMessage: %v", err)
		}
		var ok bool
		taskMsg, ok = baseMsg.Payload().(*agentic.TaskMessage)
		if !ok {
			t.Fatalf("BaseMessage payload is %T, want *agentic.TaskMessage", baseMsg.Payload())
		}
	case <-time.After(8 * time.Second):
		t.Fatal("Timed out waiting for TaskMessage on AGENT stream")
	}

	// Verify TaskMessage fields.
	// handleQuestStarted sets TaskID to the full entity ID (e.g.
	// "test.integration.game.taskpublish.quest.abc123"), not just the instance
	// segment. The full entity ID is used as the correlation key throughout the
	// quest-loop mapping lifecycle.
	if taskMsg.TaskID != string(questID) {
		t.Errorf("TaskID = %q, want %q (full entity ID)", taskMsg.TaskID, string(questID))
	}
	if taskMsg.Role == "" {
		t.Error("TaskMessage.Role must not be empty")
	}
	if taskMsg.Prompt == "" {
		t.Error("TaskMessage.Prompt must not be empty")
	}
	if taskMsg.Context == nil {
		t.Error("TaskMessage.Context must be set (system prompt context)")
	}
	if taskMsg.Metadata == nil {
		t.Fatal("TaskMessage.Metadata must not be nil")
	}
	if taskMsg.Metadata["agent_id"] == nil {
		t.Error("Metadata must contain agent_id")
	}
	if taskMsg.Metadata["quest_id"] == nil {
		t.Error("Metadata must contain quest_id")
	}
	if taskMsg.Metadata["trust_tier"] == nil {
		t.Error("Metadata must contain trust_tier")
	}
}

// =============================================================================
// TOOL FILTERING
// =============================================================================

// TestTaskMessageToolFiltering verifies that an Apprentice-tier agent (level 1-5)
// receives only tools whose MinTier <= TierApprentice. Built-in tools that require
// TierJourneyman or higher must be excluded.
func TestTaskMessageToolFiltering(t *testing.T) {
	testClient := natsclient.NewTestClient(t,
		natsclient.WithKV(), natsclient.WithFileStorage(),
		natsclient.WithStreams(natsclient.TestStreamConfig{
			Name:     "AGENT",
			Subjects: []string{"agent.task.>", "agent.complete.>", "agent.failed.>", "tool.execute.>", "tool.result.>"},
		}),
	)
	client := testClient.Client
	ctx := context.Background()

	comp, gc := setupComponent(t, client, "toolfilter")
	defer comp.Stop(5 * time.Second)

	// Create an Apprentice-tier agent (level 3).
	// bash and http_request require TierJourneyman+ so they should be absent
	// from the TaskMessage for an Apprentice agent.
	agent := createTestAgent(t, gc, comp.boardConfig, "apprentice-agent", 3)

	questID := createInProgressQuest(t, gc, comp.boardConfig, agent.ID, "Tool Filter Quest")

	taskCh := subscribeToAgentTasks(t, client, ctx)
	writeQuestInProgress(t, gc, comp.boardConfig, questID, agent.ID)

	// The component wraps TaskMessage in a BaseMessage envelope before publishing.
	var taskMsg *agentic.TaskMessage
	select {
	case data := <-taskCh:
		var baseMsg message.BaseMessage
		if err := json.Unmarshal(data, &baseMsg); err != nil {
			t.Fatalf("Failed to unmarshal BaseMessage: %v", err)
		}
		var ok bool
		taskMsg, ok = baseMsg.Payload().(*agentic.TaskMessage)
		if !ok {
			t.Fatalf("BaseMessage payload is %T, want *agentic.TaskMessage", baseMsg.Payload())
		}
	case <-time.After(8 * time.Second):
		t.Fatal("Timed out waiting for TaskMessage")
	}

	// Apprentice-tier (level 1-5) agents should receive terminal tools
	// (submit_work, ask_clarification) but NOT Journeyman+ tools (bash, http_request).
	apprenticeTools := map[string]bool{
		"submit_work":     true,
		"ask_clarification": true,
	}
	journeymanPlusTools := map[string]bool{
		"bash":         true,
		"http_request": true,
	}

	foundApprentice := map[string]bool{}
	for _, tool := range taskMsg.Tools {
		if apprenticeTools[tool.Name] {
			foundApprentice[tool.Name] = true
		}
		if journeymanPlusTools[tool.Name] {
			t.Errorf("Apprentice agent received Journeyman+-gated tool %q", tool.Name)
		}
	}

	// Verify apprentice read-only tools ARE present
	for name := range apprenticeTools {
		if !foundApprentice[name] {
			t.Errorf("Apprentice agent missing expected tool %q", name)
		}
	}
}

// =============================================================================
// QUEST-LOOP MAPPING PERSISTENCE
// =============================================================================

// TestQuestLoopsMappingPersisted verifies that after questbridge publishes a
// TaskMessage, it writes a QuestLoopMapping into the QUEST_LOOPS KV bucket.
func TestQuestLoopsMappingPersisted(t *testing.T) {
	testClient := natsclient.NewTestClient(t,
		natsclient.WithKV(), natsclient.WithFileStorage(),
		natsclient.WithStreams(natsclient.TestStreamConfig{
			Name:     "AGENT",
			Subjects: []string{"agent.task.>", "agent.complete.>", "agent.failed.>", "tool.execute.>", "tool.result.>"},
		}),
	)
	client := testClient.Client
	ctx := context.Background()

	comp, gc := setupComponent(t, client, "loopsmapping")
	defer comp.Stop(5 * time.Second)

	agent := createTestAgent(t, gc, comp.boardConfig, "mapping-agent", 7)
	questID := createInProgressQuest(t, gc, comp.boardConfig, agent.ID, "Mapping Test Quest")

	// Wait for TaskMessage so we know questbridge processed the transition.
	taskCh := subscribeToAgentTasks(t, client, ctx)
	writeQuestInProgress(t, gc, comp.boardConfig, questID, agent.ID)

	select {
	case <-taskCh:
		// TaskMessage published — mapping should now be in QUEST_LOOPS.
	case <-time.After(8 * time.Second):
		t.Fatal("Timed out waiting for TaskMessage before checking mapping")
	}

	// Poll QUEST_LOOPS until the mapping appears.
	// handleQuestStarted writes the mapping under the full entity ID key, not just
	// the instance segment. Pass the full entity ID string to the lookup helper.
	mapping := waitForQuestLoopsMapping(t, comp, ctx, string(questID), 5*time.Second)

	if mapping.QuestID != questID {
		t.Errorf("Mapping.QuestID = %v, want %v", mapping.QuestID, questID)
	}
	if mapping.AgentID != agent.ID {
		t.Errorf("Mapping.AgentID = %v, want %v", mapping.AgentID, agent.ID)
	}
	if mapping.LoopID == "" {
		t.Error("Mapping.LoopID must not be empty")
	}
	if mapping.TrustTier == 0 {
		t.Error("Mapping.TrustTier must not be zero")
	}
	if mapping.StartedAt.IsZero() {
		t.Error("Mapping.StartedAt must be set")
	}
}

// =============================================================================
// LOOP COMPLETION HANDLING
// =============================================================================

// TestLoopCompletionEmitsExecutorEvent verifies that when an agent.complete.{loopID}
// message arrives on the AGENT stream, questbridge reads the mapping and emits an
// executor.execution.completed event. The key observable side-effect is the
// loopsCompleted counter incrementing.
func TestLoopCompletionEmitsExecutorEvent(t *testing.T) {
	testClient := natsclient.NewTestClient(t,
		natsclient.WithKV(), natsclient.WithFileStorage(),
		natsclient.WithStreams(natsclient.TestStreamConfig{
			Name:     "AGENT",
			Subjects: []string{"agent.task.>", "agent.complete.>", "agent.failed.>", "tool.execute.>", "tool.result.>"},
		}),
	)
	client := testClient.Client
	ctx := context.Background()

	comp, gc := setupComponent(t, client, "loopcomplete")
	defer comp.Stop(5 * time.Second)

	agent := createTestAgent(t, gc, comp.boardConfig, "complete-agent", 7)
	questID := createInProgressQuest(t, gc, comp.boardConfig, agent.ID, "Complete Test Quest")

	// Trigger quest start and drain the TaskMessage from the AGENT stream.
	// We do not need the TaskMessage content — the LoopID is recovered from
	// QUEST_LOOPS KV, which questbridge writes atomically after publishing.
	taskCh := subscribeToAgentTasks(t, client, ctx)
	writeQuestInProgress(t, gc, comp.boardConfig, questID, agent.ID)

	select {
	case <-taskCh:
		// TaskMessage arrived — questbridge processed the in_progress transition.
	case <-time.After(8 * time.Second):
		t.Fatal("Timed out waiting for TaskMessage before publishing completion")
	}

	// Recover the quest-loop mapping which contains the LoopID generated by
	// questbridge. The QUEST_LOOPS bucket key is the full entity ID (not just the
	// instance segment) because handleQuestStarted uses entityState.ID as the key.
	// TaskID in the event must also be the full entity ID so findMapping can locate
	// the activeLoops entry stored under the same full entity ID key.
	questEntityID := string(questID)
	mapping := waitForQuestLoopsMapping(t, comp, ctx, questEntityID, 5*time.Second)

	beforeCompleted := comp.loopsCompleted.Load()

	// Publish a LoopCompletedEvent on agent.complete.{loopID}.
	// The component expects completion events wrapped in BaseMessage (same as
	// agentic-loop publishes them in production).
	completedEvent := agentic.LoopCompletedEvent{
		LoopID:     mapping.LoopID,
		TaskID:     questEntityID,
		Iterations: 3,
		TokensIn:   100,
		TokensOut:  50,
	}

	baseMsg := message.NewBaseMessage(completedEvent.Schema(), &completedEvent, "test")
	data, err := json.Marshal(baseMsg)
	if err != nil {
		t.Fatalf("Failed to marshal LoopCompletedEvent BaseMessage: %v", err)
	}

	subject := "agent.complete." + mapping.LoopID
	if err := client.PublishToStream(ctx, subject, data); err != nil {
		t.Fatalf("Failed to publish LoopCompletedEvent: %v", err)
	}

	// Wait for loopsCompleted counter to increment.
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if comp.loopsCompleted.Load() > beforeCompleted {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	afterCompleted := comp.loopsCompleted.Load()
	if afterCompleted <= beforeCompleted {
		t.Errorf("loopsCompleted did not increment: before=%d, after=%d", beforeCompleted, afterCompleted)
	}
}

// TestLoopFailureEmitsExecutorEvent verifies that agent.failed.{loopID} messages
// cause the loopsFailed counter to increment, confirming questbridge handled the event.
func TestLoopFailureEmitsExecutorEvent(t *testing.T) {
	testClient := natsclient.NewTestClient(t,
		natsclient.WithKV(), natsclient.WithFileStorage(),
		natsclient.WithStreams(natsclient.TestStreamConfig{
			Name:     "AGENT",
			Subjects: []string{"agent.task.>", "agent.complete.>", "agent.failed.>", "tool.execute.>", "tool.result.>"},
		}),
	)
	client := testClient.Client
	ctx := context.Background()

	comp, gc := setupComponent(t, client, "loopfail")
	defer comp.Stop(5 * time.Second)

	agent := createTestAgent(t, gc, comp.boardConfig, "fail-agent", 7)
	questID := createInProgressQuest(t, gc, comp.boardConfig, agent.ID, "Failure Test Quest")

	// Trigger quest start and wait for TaskMessage.
	taskCh := subscribeToAgentTasks(t, client, ctx)
	writeQuestInProgress(t, gc, comp.boardConfig, questID, agent.ID)

	select {
	case <-taskCh:
		// TaskMessage received.
	case <-time.After(8 * time.Second):
		t.Fatal("Timed out waiting for TaskMessage before publishing failure")
	}

	// Recover the mapping to get LoopID.
	// The QUEST_LOOPS bucket key is the full entity ID (not just the instance
	// segment) because handleQuestStarted uses entityState.ID as the key.
	// TaskID in the event must also be the full entity ID so findMapping can
	// locate the activeLoops entry stored under the same full entity ID key.
	questEntityID := string(questID)
	mapping := waitForQuestLoopsMapping(t, comp, ctx, questEntityID, 5*time.Second)

	beforeFailed := comp.loopsFailed.Load()

	// Publish a LoopFailedEvent on agent.failed.{loopID}.
	// The component expects failure events wrapped in BaseMessage (same as
	// agentic-loop publishes them in production).
	failedEvent := agentic.LoopFailedEvent{
		LoopID:     mapping.LoopID,
		TaskID:     questEntityID,
		Error:      "max iterations exceeded",
		Iterations: 20,
	}

	baseMsg := message.NewBaseMessage(failedEvent.Schema(), &failedEvent, "test")
	data, err := json.Marshal(baseMsg)
	if err != nil {
		t.Fatalf("Failed to marshal LoopFailedEvent BaseMessage: %v", err)
	}

	subject := "agent.failed." + mapping.LoopID
	if err := client.PublishToStream(ctx, subject, data); err != nil {
		t.Fatalf("Failed to publish LoopFailedEvent: %v", err)
	}

	// Wait for loopsFailed counter to increment.
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if comp.loopsFailed.Load() > beforeFailed {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	afterFailed := comp.loopsFailed.Load()
	if afterFailed <= beforeFailed {
		t.Errorf("loopsFailed did not increment: before=%d, after=%d", beforeFailed, afterFailed)
	}
}

// =============================================================================
// BOOTSTRAP / NO-RETRIGGER TEST
// =============================================================================

// TestBootstrapDoesNotRetriggerExistingQuests verifies the KV twofer bootstrap
// protocol: quests that are already in_progress when questbridge starts must NOT
// generate a new TaskMessage — only live transitions after bootstrap completes
// should trigger publishing.
func TestBootstrapDoesNotRetriggerExistingQuests(t *testing.T) {
	testClient := natsclient.NewTestClient(t,
		natsclient.WithKV(), natsclient.WithFileStorage(),
		natsclient.WithStreams(natsclient.TestStreamConfig{
			Name:     "AGENT",
			Subjects: []string{"agent.task.>", "agent.complete.>", "agent.failed.>", "tool.execute.>", "tool.result.>"},
		}),
	)
	client := testClient.Client
	ctx := context.Background()

	// Build config and GraphClient before starting the component, so we can
	// pre-populate KV with an in_progress quest.
	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "noretrigger"
	config.ConsumerNameSuffix = "noretrigger"
	config.DeleteConsumerOnStop = true

	boardCfg := config.ToBoardConfig()
	gc := semdragons.NewGraphClient(client, boardCfg)
	if err := gc.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket failed: %v", err)
	}

	// Pre-populate KV with a quest already in_progress.
	agent := createTestAgent(t, gc, boardCfg, "bootstrap-agent", 7)
	_ = createInProgressQuestDirect(t, gc, boardCfg, agent.ID, "Pre-existing Quest")

	// Subscribe early to capture any spurious TaskMessages.
	taskCh := subscribeToAgentTasks(t, client, ctx)

	// Now start the component — bootstrap replay should hydrate cache only.
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
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer comp.Stop(5 * time.Second)

	// Wait long enough for the bootstrap replay to complete and any spurious
	// TaskMessages to propagate — if bootstrap incorrectly fires, we'll see it.
	select {
	case <-taskCh:
		t.Fatal("questbridge should NOT publish a TaskMessage for pre-existing in_progress quest (bootstrap should only hydrate cache)")
	case <-time.After(2 * time.Second):
		// No TaskMessage received — bootstrap correctly suppressed retrigger.
	}
}

// =============================================================================
// HELPERS
// =============================================================================

// setupComponent creates, initializes, and starts a questbridge Component backed
// by the given NATS client. Board name is used as the unique board identifier.
// The test is registered to stop the component on cleanup via t.Cleanup.
func setupComponent(t *testing.T, client *natsclient.Client, board string) (*Component, *semdragons.GraphClient) {
	t.Helper()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = board
	config.ConsumerNameSuffix = board // Unique consumer per test to avoid durable conflicts.
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

	// EnsureBucket mirrors main.go startup sequence — must happen before Start.
	gc := semdragons.NewGraphClient(client, comp.boardConfig)
	if err := gc.EnsureBucket(context.Background()); err != nil {
		t.Fatalf("EnsureBucket failed: %v", err)
	}

	if err := comp.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	t.Cleanup(func() {
		_ = comp.Stop(5 * time.Second)
	})

	return comp, gc
}

// createTestAgent writes an idle agent entity into the board KV bucket.
// level determines the trust tier (see domain.TierFromLevel).
func createTestAgent(t *testing.T, gc *semdragons.GraphClient, boardCfg *domain.BoardConfig, name string, level int) *agentprogression.Agent {
	t.Helper()

	instance := domain.GenerateInstance()
	agentID := domain.AgentID(boardCfg.AgentEntityID(instance))

	agent := &agentprogression.Agent{
		ID:     agentID,
		Name:   name,
		Level:  level,
		Tier:   domain.TierFromLevel(level),
		Status: domain.AgentIdle,
	}

	ctx := context.Background()
	if err := gc.PutEntityState(ctx, agent, "agent.identity.created"); err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	return agent
}

// createInProgressQuest writes an in_progress quest into KV and returns its
// entity-level QuestID. This is used to prime the questbridge live-update path
// by writing the entity after the component has bootstrapped.
//
// The quest is first written as "claimed" so the questCache has a prior non-
// in_progress entry, then the caller invokes writeQuestInProgress to trigger
// the transition detection. This helper returns the full entity ID.
func createInProgressQuest(t *testing.T, gc *semdragons.GraphClient, boardCfg *domain.BoardConfig, agentID domain.AgentID, title string) domain.QuestID {
	t.Helper()

	instance := domain.GenerateInstance()
	entityID := boardCfg.QuestEntityID(instance)
	questID := domain.QuestID(entityID)

	// Write as "claimed" so questCache records a non-in_progress status.
	// writeQuestInProgress will then trigger the transition.
	quest := &domain.Quest{
		ID:          domain.QuestID(questID),
		Title:       title,
		Description: "Integration test quest",
		Status:      domain.QuestClaimed,
		Difficulty:  domain.DifficultyTrivial,
		BaseXP:      100,
		MaxAttempts: 3,
		Attempts:    1,
		ClaimedBy:   (*domain.AgentID)(&agentID),
	}

	ctx := context.Background()
	if err := gc.PutEntityState(ctx, quest, "quest.claimed"); err != nil {
		t.Fatalf("Failed to write claimed quest: %v", err)
	}

	return questID
}

// createInProgressQuestDirect writes a quest directly as in_progress into KV.
// Used by the bootstrap test to pre-populate KV before the component starts.
func createInProgressQuestDirect(t *testing.T, gc *semdragons.GraphClient, boardCfg *domain.BoardConfig, agentID domain.AgentID, title string) domain.QuestID {
	t.Helper()

	instance := domain.GenerateInstance()
	entityID := boardCfg.QuestEntityID(instance)
	questID := domain.QuestID(entityID)

	quest := &domain.Quest{
		ID:          domain.QuestID(questID),
		Title:       title,
		Description: "Pre-existing in_progress quest for bootstrap test",
		Status:      domain.QuestInProgress,
		Difficulty:  domain.DifficultyTrivial,
		BaseXP:      100,
		MaxAttempts: 3,
		Attempts:    1,
		ClaimedBy:   (*domain.AgentID)(&agentID),
	}

	ctx := context.Background()
	if err := gc.PutEntityState(ctx, quest, "quest.started"); err != nil {
		t.Fatalf("Failed to write in_progress quest: %v", err)
	}

	return questID
}

// writeQuestInProgress updates an existing quest to in_progress status in KV.
// This triggers the questbridge watchLoop's live-update path, which detects the
// transition from the prior cached status and calls handleQuestStarted.
func writeQuestInProgress(t *testing.T, gc *semdragons.GraphClient, boardCfg *domain.BoardConfig, questID domain.QuestID, agentID domain.AgentID) {
	t.Helper()

	now := time.Now()
	quest := &domain.Quest{
		ID:          domain.QuestID(questID),
		Title:       "In Progress Quest",
		Description: "Quest transitioned to in_progress",
		Status:      domain.QuestInProgress,
		Difficulty:  domain.DifficultyTrivial,
		BaseXP:      100,
		MaxAttempts: 3,
		Attempts:    1,
		ClaimedBy:   (*domain.AgentID)(&agentID),
		StartedAt:   &now,
	}

	ctx := context.Background()
	if err := gc.EmitEntityUpdate(ctx, quest, "quest.started"); err != nil {
		t.Fatalf("Failed to write quest in_progress: %v", err)
	}
}

// subscribeToAgentTasks creates an ephemeral JetStream consumer on the AGENT stream
// that receives agent.task.> messages and forwards their raw bytes to the returned
// channel. The consumer delivers only new messages (DeliverNewPolicy) so it won't
// replay previously published messages.
func subscribeToAgentTasks(t *testing.T, client *natsclient.Client, ctx context.Context) <-chan []byte {
	t.Helper()

	js, err := client.JetStream()
	if err != nil {
		t.Fatalf("JetStream() failed: %v", err)
	}

	consumer, err := js.CreateOrUpdateConsumer(ctx, "AGENT", jetstream.ConsumerConfig{
		// Ephemeral (no Durable) so NATS auto-cleans up when the test connection closes.
		FilterSubject: "agent.task.>",
		DeliverPolicy: jetstream.DeliverNewPolicy,
		AckPolicy:     jetstream.AckNonePolicy,
	})
	if err != nil {
		t.Fatalf("Failed to create agent.task consumer: %v", err)
	}

	ch := make(chan []byte, 8)

	consumeCtx, consumeErr := consumer.Consume(func(msg jetstream.Msg) {
		// Copy the data before the message lifecycle ends.
		data := make([]byte, len(msg.Data()))
		copy(data, msg.Data())
		select {
		case ch <- data:
		default:
			// Drop if buffer full — tests use small pipelines.
		}
	})
	if consumeErr != nil {
		t.Fatalf("Failed to start agent.task consume: %v", consumeErr)
	}

	t.Cleanup(func() {
		consumeCtx.Stop()
	})

	return ch
}

// waitForQuestLoopsMapping polls the QUEST_LOOPS KV bucket until a mapping entry
// for the given quest appears, or the timeout expires.
//
// questKey must be the full entity ID (e.g.
// "test.integration.game.board.quest.abc123") because handleQuestStarted writes
// the mapping under entityState.ID, which is the full entity ID, not just the
// instance segment. Pass string(questID) at call sites.
//
// Returns the decoded QuestLoopMapping on success; fails the test on timeout.
func waitForQuestLoopsMapping(t *testing.T, comp *Component, ctx context.Context, questKey string, timeout time.Duration) QuestLoopMapping {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		entry, err := comp.questLoopsBucket.Get(ctx, questKey)
		if err == nil && len(entry.Value()) > 0 {
			var mapping QuestLoopMapping
			if jsonErr := json.Unmarshal(entry.Value(), &mapping); jsonErr == nil {
				return mapping
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("Timed out waiting for QUEST_LOOPS mapping for quest %q", questKey)
	return QuestLoopMapping{} // unreachable
}
