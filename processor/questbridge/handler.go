package questbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/executor"
	"github.com/c360studio/semdragons/processor/promptmanager"
	"github.com/c360studio/semdragons/processor/questdagexec"
	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	pkgcontext "github.com/c360studio/semstreams/pkg/context"
	pkgtypes "github.com/c360studio/semstreams/pkg/types"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nuid"
)

// depGraphHTTPClient is a dedicated HTTP client for dependency context GraphQL
// queries. Timeout is short — these calls must never block quest dispatch.
var depGraphHTTPClient = &http.Client{Timeout: 600 * time.Millisecond}

// =============================================================================
// KV TWOFER BOOTSTRAP PROTOCOL
// =============================================================================
// Phase 1 (bootstrapping=true): replay existing KV state — hydrate questCache only.
// The nil sentinel entry marks end of historical replay.
// Phase 2 (bootstrapping=false): process live updates and detect transitions.
//
// This prevents spuriously re-triggering execution for already in_progress
// quests that were running before this instance started.
// =============================================================================

// watchLoop implements the KV twofer bootstrap protocol for quest entity watching.
func (c *Component) watchLoop(ctx context.Context) {
	watcher, err := c.graph.WatchEntityType(ctx, domain.EntityTypeQuest)
	if err != nil {
		c.logger.Error("failed to start quest watcher", "error", err)
		c.errorsCount.Add(1)
		return
	}
	defer watcher.Stop()

	bootstrapping := true

	for {
		select {
		case <-c.stopChan:
			return
		case <-ctx.Done():
			return
		case entry, ok := <-watcher.Updates():
			if !ok {
				return
			}

			// Nil sentinel marks the end of the historical replay phase.
			if entry == nil {
				bootstrapping = false
				c.reconcileOrphanedQuests(ctx)
				continue
			}

			if bootstrapping {
				// During bootstrap: hydrate cache only, never trigger actions.
				// This prevents re-firing execution for quests already running.
				if status := c.extractQuestStatus(entry); status != "" {
					c.questCache.Store(entry.Key(), status)
				}
			} else {
				// After bootstrap: detect transitions and trigger execution.
				c.handleLiveUpdate(ctx, entry)
			}
		}
	}
}

// extractQuestStatus extracts the current quest status from an entity state KV entry.
// Returns an empty string if the entry cannot be decoded or has no status triple.
func (c *Component) extractQuestStatus(entry jetstream.KeyValueEntry) string {
	entityState, err := semdragons.DecodeEntityState(entry)
	if err != nil || entityState == nil {
		return ""
	}
	return tripleString(entityState.Triples, "quest.status.state")
}

// handleLiveUpdate processes a live quest entity KV change and detects status transitions.
func (c *Component) handleLiveUpdate(ctx context.Context, entry jetstream.KeyValueEntry) {
	entityState, err := semdragons.DecodeEntityState(entry)
	if err != nil {
		c.logger.Warn("failed to decode entity state in live update",
			"key", entry.Key(), "error", err)
		c.errorsCount.Add(1)
		return
	}
	if entityState == nil {
		return
	}

	newStatus := tripleString(entityState.Triples, "quest.status.state")
	if newStatus == "" {
		return
	}

	// Swap atomically; returns the previous value and whether the key existed before.
	oldStatusI, existed := c.questCache.Swap(entry.Key(), newStatus)
	oldStatus, _ := oldStatusI.(string)

	// Track escalation timestamps for the stale-escalation sweep.
	if newStatus == string(domain.QuestEscalated) && oldStatus != string(domain.QuestEscalated) {
		c.escalatedAt.Store(entry.Key(), time.Now())

		// In full_auto mode, auto-answer clarifications via LLM.
		if c.config.DMMode == domain.DMFullAuto && c.clarificationAnswerer != nil {
			go c.autoAnswerClarification(ctx, entry.Key())
		}
	} else if oldStatus == string(domain.QuestEscalated) && newStatus != string(domain.QuestEscalated) {
		c.escalatedAt.Delete(entry.Key())
	}

	if newStatus == string(domain.QuestInProgress) {
		// Only trigger when transitioning TO in_progress, not when already there.
		if !existed || oldStatus != string(domain.QuestInProgress) {
			c.handleQuestStarted(ctx, entityState)
		}
	}
}

// handleQuestStarted is triggered when a quest transitions to in_progress.
// It assembles a TaskMessage and publishes it to the AGENT stream.
func (c *Component) handleQuestStarted(ctx context.Context, entityState *graph.EntityState) {
	// When paused, skip dispatching. Quest stays in cache as in_progress;
	// reconcileOrphanedQuests will pick it up on resume.
	if c.pauseChecker != nil && c.pauseChecker.Paused() {
		c.logger.Info("board paused, deferring quest dispatch",
			"entity_id", entityState.ID)
		return
	}

	// When token budget is exceeded, defer dispatching. Quest stays in cache
	// as in_progress; reconcileOrphanedQuests will pick it up on budget reset.
	if c.tokenLedger != nil {
		if err := c.tokenLedger.Check(); err != nil {
			c.logger.Warn("token budget exceeded, deferring quest dispatch",
				"entity_id", entityState.ID, "error", err)
			return
		}
	}

	// Soft-gate: when semsource is configured, wait briefly for at least one
	// knowledge source to finish indexing. This ensures graph manifest data is
	// available for entity knowledge injection into agent prompts. Proceeds
	// after timeout — knowledge context is nice-to-have, not blocking.
	c.waitForKnowledgeSources(ctx, entityState.ID)

	// Use quest.identity.id triple when present; fall back to entity state ID.
	questID := tripleString(entityState.Triples, "quest.identity.id")
	if questID == "" {
		questID = entityState.ID
	}

	// The agent ID is stored in quest.assignment.agent triple by questboard.
	agentID := tripleString(entityState.Triples, "quest.assignment.agent")
	if agentID == "" {
		c.logger.Warn("quest transitioned to in_progress but has no assigned agent",
			"quest_id", questID)
		return
	}

	// Load full quest and agent from the board KV bucket.
	questEntity, err := c.graph.GetQuest(ctx, domain.QuestID(questID))
	if err != nil {
		c.logger.Error("failed to load quest for execution",
			"quest_id", questID, "error", err)
		c.errorsCount.Add(1)
		return
	}
	quest := domain.QuestFromEntityState(questEntity)
	if quest == nil {
		c.logger.Error("quest reconstruction returned nil", "quest_id", questID)
		c.errorsCount.Add(1)
		return
	}

	agentEntity, err := c.graph.GetAgent(ctx, domain.AgentID(agentID))
	if err != nil {
		c.logger.Error("failed to load agent for execution",
			"agent_id", agentID, "error", err)
		c.errorsCount.Add(1)
		return
	}
	agent := agentprogression.AgentFromEntityState(agentEntity)
	if agent == nil {
		c.logger.Error("agent reconstruction returned nil", "agent_id", agentID)
		c.errorsCount.Add(1)
		return
	}

	// Get tool definitions filtered for this quest and agent.
	// Resolved early so tool names can be threaded into prompt assembly.
	tools := c.toolsForQuest(quest, agent)
	toolNames := extractToolNames(tools)

	// Build system prompt using assembler or legacy path.
	assembled := c.buildSystemPrompt(ctx, agent, quest, toolNames)

	// Build entity knowledge — structured context about agent, quest, party, guild.
	var entityKnowledgeContent string
	var knowledgeEntityIDs []string
	if c.config.EntityContextBudget > 0 {
		ekb := &entityKnowledgeBuilder{
			graph:               c.graph,
			budgetToken:         c.config.EntityContextBudget,
			logger:              c.logger,
			manifestClient:      c.manifestClient,
			graphManifestClient: c.graphManifestClient,
		}
		ek := ekb.build(ctx, quest, agent)
		entityKnowledgeContent = ek.content
		knowledgeEntityIDs = ek.entityIDs
	}

	// Resolve model capability key and endpoint.
	capability := c.resolveCapability(agent, quest)
	modelKey := capability
	if c.registry != nil {
		if resolved := c.registry.Resolve(capability); resolved != "" {
			modelKey = resolved
		}
	}

	// Determine agent role — config default_role takes precedence over blank.
	role := c.config.DefaultRole
	if role == "" {
		role = agentic.RoleGeneral
	}

	// Build the user prompt from quest input.
	userPrompt := buildUserPrompt(quest)

	// Build context metadata — which entities and fragments informed this dispatch.
	contextEntities := []string{questID, agentID}
	if quest.PartyID != nil {
		contextEntities = append(contextEntities, string(*quest.PartyID))
	}
	if quest.GuildPriority != nil {
		contextEntities = append(contextEntities, string(*quest.GuildPriority))
	}
	if quest.ParentQuest != nil {
		contextEntities = append(contextEntities, string(*quest.ParentQuest))
	}
	// Merge entity IDs discovered during knowledge building.
	contextEntities = append(contextEntities, knowledgeEntityIDs...)

	// Merge entity knowledge into the system prompt content.
	contextContent := assembled.SystemMessage
	if entityKnowledgeContent != "" {
		contextContent = assembled.SystemMessage + "\n\n" + entityKnowledgeContent
	}
	tokenCount := pkgcontext.EstimateTokens(contextContent)

	// Create workspace for the agent's file operations via sandbox container.
	// When the quest declares a target repo, the sandbox creates a git worktree
	// from that repo's main branch. Falls back to config.DefaultRepo for the
	// single-repo MVP. Plain directory created when no repo is configured.
	repo := quest.Repo
	if repo == "" {
		repo = c.config.DefaultRepo
	}
	if c.sandboxClient != nil {
		if wsErr := c.sandboxClient.CreateWorkspace(ctx, questID, repo); wsErr != nil {
			c.logger.Error("sandbox workspace creation failed, cannot dispatch quest",
				"quest_id", questID, "repo", repo, "error", wsErr)
			c.errorsCount.Add(1)
			return
		}
	}

	// Construct the TaskMessage.
	// Sanitize questID for use in NATS subject tokens — entity IDs contain dots
	// which are subject delimiters. Replace dots with hyphens so the ID is a single
	// token, allowing agent.task.* and agent.complete.* filters to match correctly.
	subjectSafeQuestID := strings.ReplaceAll(questID, ".", "-")
	loopID := fmt.Sprintf("quest-%s-%s", subjectSafeQuestID, nuid.Next())
	taskMsg := agentic.TaskMessage{
		TaskID: questID,
		LoopID: loopID,
		Role:   role,
		Model:  modelKey,
		Prompt: userPrompt,
		Context: &pkgtypes.ConstructedContext{
			Content:       contextContent,
			TokenCount:    tokenCount,
			Entities:      contextEntities,
			Sources:       fragmentsToSources(assembled.FragmentsUsed),
			ConstructedAt: time.Now(),
		},
		Tools:      tools,
		ToolChoice: toolChoiceForQuest(quest, agent, tools),
		Metadata: map[string]any{
			"agent_id":    agentID,
			"quest_id":    questID,
			"trust_tier":  int(agent.Tier),
			"skills":      agentSkillNames(agent),
			"sandbox_dir": c.config.SandboxDir,
			"board":       c.config.Board,
		},
	}

	// Write context metadata to quest entity for UI visibility.
	// Must happen BEFORE publishing TaskMessage — a fast-completing task could
	// race and overwrite the quest status if we emit after dispatch.
	quest.ContextTokenCount = tokenCount
	quest.ContextSources = assembled.FragmentsUsed
	quest.ContextEntities = contextEntities
	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.context.assembled"); err != nil {
		c.logger.Warn("failed to emit context metadata", "quest_id", questID, "error", err)
	}

	// Wrap in BaseMessage envelope (required by agentic-loop consumer).
	baseMsg := message.NewBaseMessage(taskMsg.Schema(), &taskMsg, "questbridge")
	data, err := json.Marshal(baseMsg)
	if err != nil {
		c.logger.Error("failed to marshal TaskMessage", "quest_id", questID, "error", err)
		c.errorsCount.Add(1)
		return
	}

	// Publish to agent.task.{questID} on the AGENT stream.
	// Use the sanitized (dot-free) quest ID so the subject is a valid 3-token
	// pattern matching agent.task.* consumer filters.
	subject := fmt.Sprintf("agent.task.%s", subjectSafeQuestID)
	if err := c.deps.NATSClient.PublishToStream(ctx, subject, data); err != nil {
		c.logger.Error("failed to publish TaskMessage",
			"quest_id", questID, "subject", subject, "error", err)
		c.errorsCount.Add(1)
		return
	}

	// Persist quest-loop mapping in QUEST_LOOPS KV for crash recovery.
	mapping := QuestLoopMapping{
		LoopID:     loopID,
		QuestID:    quest.ID,
		AgentID:    agent.ID,
		SandboxDir: c.config.SandboxDir,
		TrustTier:  agent.Tier,
		StartedAt:  time.Now(),
		LoopType:   LoopTypeExecution,
	}
	mappingData, marshalErr := json.Marshal(mapping)
	if marshalErr != nil {
		c.logger.Error("failed to marshal quest-loop mapping",
			"quest_id", questID, "error", marshalErr)
		c.errorsCount.Add(1)
		return
	}
	if _, err := c.questLoopsBucket.Put(ctx, questID, mappingData); err != nil {
		c.logger.Warn("failed to write QUEST_LOOPS mapping",
			"quest_id", questID, "error", err)
	}
	c.activeLoops.Store(questID, &mapping)

	// Store the loop ID on the quest entity so the cancel API can find the
	// active loop while the quest is in_progress.
	c.storeLoopIDOnQuest(ctx, domain.QuestID(questID), loopID)

	c.tasksPublished.Add(1)
	c.lastActivity.Store(time.Now())

	// Emit execution started event for observability.
	now := time.Now()
	if err := executor.SubjectExecutionStarted.Publish(ctx, c.deps.NATSClient, executor.ExecutionStartedPayload{
		QuestID:    quest.ID,
		QuestTitle: quest.Title,
		AgentID:    agent.ID,
		AgentName:  agent.Name,
		LoopID:     loopID,
		MaxTurns:   maxIterationsForDifficulty(c.config.MaxIterations, quest.Difficulty),
		ToolCount:  len(tools),
		Timestamp:  now,
	}); err != nil {
		c.logger.Warn("failed to emit execution started event", "error", err)
		c.errorsCount.Add(1)
	}

	c.logger.Info("published TaskMessage for quest execution",
		"quest_id", questID,
		"agent_id", agentID,
		"model", modelKey,
		"role", role,
		"tools", len(tools),
		"loop_id", loopID)
}

// =============================================================================
// LOOP COMPLETION CONSUMER
// =============================================================================

// consumeCompletions creates a durable consumer on the AGENT stream for
// agent.complete.* and agent.failed.* subjects, then processes messages in a
// fetch loop until stopped.
func (c *Component) consumeCompletions(ctx context.Context) {
	js, err := c.deps.NATSClient.JetStream()
	if err != nil {
		c.logger.Error("failed to get JetStream for completions consumer", "error", err)
		c.errorsCount.Add(1)
		return
	}

	consumerName := "questbridge-completions"
	if c.config.ConsumerNameSuffix != "" {
		consumerName += "-" + c.config.ConsumerNameSuffix
	}

	consumer, err := js.CreateOrUpdateConsumer(ctx, c.config.StreamName, jetstream.ConsumerConfig{
		Durable:        consumerName,
		FilterSubjects: []string{"agent.complete.*", "agent.failed.*"},
		AckPolicy:      jetstream.AckExplicitPolicy,
	})
	if err != nil {
		c.logger.Error("failed to create completions consumer",
			"consumer", consumerName, "error", err)
		c.errorsCount.Add(1)
		return
	}

	for {
		select {
		case <-c.stopChan:
			if c.config.DeleteConsumerOnStop {
				delCtx, delCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer delCancel()
				if delErr := js.DeleteConsumer(delCtx, c.config.StreamName, consumerName); delErr != nil {
					c.logger.Warn("failed to delete consumer on stop",
						"consumer", consumerName, "error", delErr)
				}
			}
			return
		case <-ctx.Done():
			return
		default:
		}

		msgs, fetchErr := consumer.Fetch(1, jetstream.FetchMaxWait(2*time.Second))
		if fetchErr != nil {
			if !errors.Is(fetchErr, context.DeadlineExceeded) {
				c.logger.Warn("fetch error on completions consumer", "error", fetchErr)
				c.errorsCount.Add(1)
				select {
				case <-c.stopChan:
					return
				case <-ctx.Done():
					return
				case <-time.After(500 * time.Millisecond):
				}
			}
			continue
		}

		for msg := range msgs.Messages() {
			c.handleCompletionMessage(ctx, msg)
		}
	}
}

// handleCompletionMessage dispatches a single completion or failure message.
func (c *Component) handleCompletionMessage(ctx context.Context, msg jetstream.Msg) {
	defer func() {
		if err := msg.Ack(); err != nil {
			c.logger.Warn("failed to ack completion message", "error", err)
		}
	}()

	subject := msg.Subject()
	switch {
	case strings.HasPrefix(subject, "agent.complete."):
		c.handleLoopCompleted(ctx, msg.Data())
	case strings.HasPrefix(subject, "agent.failed."):
		c.handleLoopFailed(ctx, msg.Data())
	}
}

// handleLoopCompleted emits an executor completion event for the finished loop.
func (c *Component) handleLoopCompleted(ctx context.Context, data []byte) {
	// Completion events are published by agentic-loop wrapped in BaseMessage.
	var baseMsg message.BaseMessage
	if err := json.Unmarshal(data, &baseMsg); err != nil {
		c.logger.Error("failed to unmarshal completion BaseMessage", "error", err)
		c.errorsCount.Add(1)
		return
	}
	event, ok := baseMsg.Payload().(*agentic.LoopCompletedEvent)
	if !ok {
		// alpha.41 publishes LoopCancelledEvent on agent.complete.* as well.
		if cancelled, cOk := baseMsg.Payload().(*agentic.LoopCancelledEvent); cOk {
			c.handleLoopCancelled(ctx, cancelled)
			return
		}
		c.logger.Error("unexpected payload type in completion message",
			"type", fmt.Sprintf("%T", baseMsg.Payload()))
		c.errorsCount.Add(1)
		return
	}

	questID := domain.QuestID(event.TaskID)
	mapping := c.findMapping(ctx, string(questID), event.LoopID)
	if mapping == nil {
		c.logger.Debug("no mapping found for completed loop",
			"task_id", event.TaskID, "loop_id", event.LoopID)
		return
	}

	// Record token usage from the completed loop.
	if c.tokenLedger != nil && (event.TokensIn > 0 || event.TokensOut > 0) {
		endpointName := c.resolveQuestEndpoint()
		c.tokenLedger.Record(ctx, event.TokensIn, event.TokensOut, "quest", endpointName)
	}

	// Determine cleanup key: execution loops are keyed by questID,
	// review/clarify loops are keyed by loopID.
	cleanupKey := string(questID)
	if mapping.LoopType == LoopTypeReview || mapping.LoopType == LoopTypeClarify {
		cleanupKey = event.LoopID
	}

	// Only transition quest state for execution loops. Review and clarify
	// loops are handled by questdagexec which watches KV state transitions
	// written by the review/clarify tools.
	if mapping.LoopType == LoopTypeReview || mapping.LoopType == LoopTypeClarify {
		c.logger.Info("DAG loop completed via agentic loop",
			"quest_id", questID,
			"loop_id", event.LoopID,
			"loop_type", mapping.LoopType,
			"iterations", event.Iterations)
	} else {
		c.completeQuest(ctx, questID, mapping, event.Result, loopMetrics{
			TurnsUsed: event.Iterations,
			TokensIn:   event.TokensIn,
			TokensOut:   event.TokensOut,
		})
		c.logger.Info("quest execution completed via agentic loop",
			"quest_id", questID,
			"loop_id", event.LoopID,
			"iterations", event.Iterations)
	}

	now := time.Now()
	if err := executor.SubjectExecutionCompleted.Publish(ctx, c.deps.NATSClient, executor.ExecutionCompletedPayload{
		QuestID:          questID,
		AgentID:          mapping.AgentID,
		LoopID:           event.LoopID,
		Status:           executor.StatusComplete,
		TotalTurns:       event.Iterations,
		PromptTokens:     event.TokensIn,
		CompletionTokens: event.TokensOut,
		Duration:         now.Sub(mapping.StartedAt),
		Timestamp:        now,
	}); err != nil {
		c.logger.Warn("failed to emit execution completed event", "error", err)
		c.errorsCount.Add(1)
	}

	c.cleanupMapping(ctx, cleanupKey)
	c.loopsCompleted.Add(1)
	c.lastActivity.Store(time.Now())
}

// handleLoopFailed emits an executor failure event for the failed loop.
func (c *Component) handleLoopFailed(ctx context.Context, data []byte) {
	// Failure events are published by agentic-loop wrapped in BaseMessage.
	var baseMsg message.BaseMessage
	if err := json.Unmarshal(data, &baseMsg); err != nil {
		c.logger.Error("failed to unmarshal failure BaseMessage", "error", err)
		c.errorsCount.Add(1)
		return
	}
	event, ok := baseMsg.Payload().(*agentic.LoopFailedEvent)
	if !ok {
		c.logger.Error("unexpected payload type in failure message",
			"type", fmt.Sprintf("%T", baseMsg.Payload()))
		c.errorsCount.Add(1)
		return
	}

	questID := domain.QuestID(event.TaskID)
	mapping := c.findMapping(ctx, string(questID), event.LoopID)
	if mapping == nil {
		c.logger.Debug("no mapping found for failed loop",
			"task_id", event.TaskID, "loop_id", event.LoopID)
		return
	}

	// Record token usage from the failed loop.
	if c.tokenLedger != nil && (event.TokensIn > 0 || event.TokensOut > 0) {
		endpointName := c.resolveQuestEndpoint()
		c.tokenLedger.Record(ctx, event.TokensIn, event.TokensOut, "quest", endpointName)
	}

	// Determine cleanup key: execution loops keyed by questID,
	// review/clarify loops keyed by loopID.
	cleanupKey := string(questID)
	if mapping.LoopType == LoopTypeReview || mapping.LoopType == LoopTypeClarify {
		cleanupKey = event.LoopID
	}

	// Only transition quest state for execution loops.
	if mapping.LoopType == LoopTypeReview || mapping.LoopType == LoopTypeClarify {
		c.logger.Warn("DAG loop failed via agentic loop",
			"quest_id", questID,
			"loop_id", event.LoopID,
			"loop_type", mapping.LoopType,
			"error", event.Error)
	} else {
		c.failQuest(ctx, questID, mapping, event.Error, loopMetrics{
			TurnsUsed: event.Iterations,
			TokensIn:   event.TokensIn,
			TokensOut:   event.TokensOut,
		})
		c.logger.Info("quest execution failed via agentic loop",
			"quest_id", questID,
			"loop_id", event.LoopID,
			"error", event.Error)
	}

	now := time.Now()
	if err := executor.SubjectExecutionFailed.Publish(ctx, c.deps.NATSClient, executor.ExecutionFailedPayload{
		QuestID:        questID,
		AgentID:        mapping.AgentID,
		LoopID:         event.LoopID,
		Status:         executor.StatusFailed,
		Error:          event.Error,
		TurnsCompleted: event.Iterations,
		Duration:       now.Sub(mapping.StartedAt),
		Timestamp:      now,
	}); err != nil {
		c.logger.Warn("failed to emit execution failed event", "error", err)
		c.errorsCount.Add(1)
	}

	c.cleanupMapping(ctx, cleanupKey)
	c.loopsFailed.Add(1)
	c.lastActivity.Store(time.Now())
}

// handleLoopCancelled handles a LoopCancelledEvent published on agent.complete.*
// by semstreams alpha.41+. Transitions the quest to failed.
func (c *Component) handleLoopCancelled(ctx context.Context, event *agentic.LoopCancelledEvent) {
	questID := domain.QuestID(event.TaskID)
	mapping := c.findMapping(ctx, string(questID), event.LoopID)
	if mapping == nil {
		c.logger.Debug("no mapping found for cancelled loop",
			"task_id", event.TaskID, "loop_id", event.LoopID)
		return
	}

	reason := "loop cancelled"
	if event.CancelledBy != "" {
		reason = "loop cancelled: " + event.CancelledBy
	}

	// Determine cleanup key.
	cleanupKey := string(questID)
	if mapping.LoopType == LoopTypeReview || mapping.LoopType == LoopTypeClarify {
		cleanupKey = event.LoopID
	}

	// Only transition quest state for execution loops.
	if mapping.LoopType != LoopTypeReview && mapping.LoopType != LoopTypeClarify {
		c.failQuest(ctx, questID, mapping, reason, loopMetrics{})
	}

	now := time.Now()
	if err := executor.SubjectExecutionFailed.Publish(ctx, c.deps.NATSClient, executor.ExecutionFailedPayload{
		QuestID:   questID,
		AgentID:   mapping.AgentID,
		LoopID:    event.LoopID,
		Status:    executor.StatusFailed,
		Error:     reason,
		Duration:  now.Sub(mapping.StartedAt),
		Timestamp: now,
	}); err != nil {
		c.logger.Warn("failed to emit execution failed event for cancelled loop", "error", err)
		c.errorsCount.Add(1)
	}

	c.cleanupMapping(ctx, cleanupKey)
	c.loopsFailed.Add(1)
	c.lastActivity.Store(time.Now())

	c.logger.Info("loop cancelled via agentic loop",
		"quest_id", questID,
		"loop_id", event.LoopID,
		"loop_type", mapping.LoopType,
		"cancelled_by", event.CancelledBy)
}

// storeLoopIDOnQuest persists the loop ID on the quest entity so that the
// cancel API can find the active loop for an in-progress quest.
func (c *Component) storeLoopIDOnQuest(ctx context.Context, questID domain.QuestID, loopID string) {
	questEntity, err := c.graph.GetQuest(ctx, questID)
	if err != nil {
		c.logger.Warn("failed to load quest for loop ID storage", "quest_id", questID, "error", err)
		return
	}
	quest := domain.QuestFromEntityState(questEntity)
	if quest == nil {
		return
	}
	quest.LoopID = loopID
	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.loop.assigned"); err != nil {
		c.logger.Warn("failed to store loop ID on quest", "quest_id", questID, "error", err)
	}
}

// =============================================================================
// QUEST STATE TRANSITIONS
// =============================================================================

// completeQuest transitions the quest to in_review for DM-directed evaluation,
// or triggers DAG sub-quest posting when the lead's output contains a valid DAG.
// The agent is NOT released here — bossbattle auto-passes or runs evaluation,
// then agentprogression handles agent release on the terminal quest status.
// loopMetrics captures execution metrics from an agentic-loop completion event.
type loopMetrics struct {
	TurnsUsed int
	TokensIn  int
	TokensOut int
}

func (c *Component) completeQuest(ctx context.Context, questID domain.QuestID, mapping *QuestLoopMapping, output string, metrics loopMetrics) {
	questEntity, err := c.graph.GetQuest(ctx, questID)
	if err != nil {
		c.logger.Error("failed to load quest for completion", "quest_id", questID, "error", err)
		c.errorsCount.Add(1)
		return
	}
	quest := domain.QuestFromEntityState(questEntity)
	if quest == nil {
		c.logger.Error("quest reconstruction returned nil for completion", "quest_id", questID)
		c.errorsCount.Add(1)
		return
	}

	// Guard: only transition quests that are still in_progress. If the quest
	// was already failed/reposted/completed via the API while the agentic loop
	// was running, skip the transition to avoid overwriting the API's decision.
	if quest.Status != domain.QuestInProgress {
		c.logger.Warn("skipping quest completion — quest is no longer in_progress",
			"quest_id", questID, "current_status", quest.Status, "loop_id", mapping.LoopID)
		return
	}

	// When a party quest completes and we have a questboard reference, check
	// whether the lead's output contains a DAG decomposition. A valid DAG
	// triggers sub-quest posting and DAG state initialization instead of the
	// normal in_review transition.
	qb := c.resolveQuestBoard()
	if quest.PartyRequired && qb != nil {
		dag, ok := extractDAGFromOutput(output)
		if ok {
			c.logger.Info("detected DAG output from party quest lead",
				"quest_id", questID,
				"nodes", len(dag.Nodes))

			if err := c.handleDAGDecomposition(ctx, quest, mapping, dag, qb); err != nil {
				c.logger.Error("DAG decomposition failed, failing quest",
					"quest_id", questID, "error", err)
				quest.Status = domain.QuestFailed
				quest.LoopID = mapping.LoopID
				quest.FailureReason = fmt.Sprintf("DAG decomposition failed: %s", err)
				if emitErr := c.graph.EmitEntityUpdate(ctx, quest, "quest.failed"); emitErr != nil {
					c.logger.Error("failed to emit quest failure after DAG error",
						"quest_id", questID, "error", emitErr)
				}
				c.releaseAgent(ctx, mapping.AgentID)
				c.errorsCount.Add(1)
			}
			// DAG path handled — do not fall through to normal in_review.
			return
		}
		c.logger.Debug("party quest output contains no DAG, using normal completion path",
			"quest_id", questID)
	}

	quest.LoopID = mapping.LoopID

	// Persist execution metrics from agentic-loop completion.
	quest.TurnsUsed = metrics.TurnsUsed
	quest.TokensPrompt = metrics.TokensIn
	quest.TokensCompletion = metrics.TokensOut
	quest.Duration = time.Since(mapping.StartedAt)

	// Try tool-based JSON output first (submit_work_product / ask_clarification).
	// Falls back to legacy intent tags and heuristic detection for non-compliant models.
	isClarification := false
	if outputType, content, ok := parseToolOutput(output); ok {
		switch outputType {
		case "work_product":
			// Extract deliverable as the actual quest output (strip JSON envelope).
			output = content
			// Safety net: some models submit questions via submit_work_product
			// instead of ask_clarification. Only check the structured intent tag
			// here — NOT the "?" heuristic. The agent explicitly called the
			// submit_work_product tool, so trust its intent. Research deliverables
			// legitimately contain question marks (e.g. "What is XSS?").
			if hasIntentClarification(content) {
				isClarification = true
				c.logger.Warn("agent submitted question via submit_work_product, rerouting to clarification",
					"quest_id", questID, "agent_id", mapping.AgentID)
			} else {
				c.logger.Info("tool-based work product submission",
					"quest_id", questID, "agent_id", mapping.AgentID)
			}
		case "clarification":
			// Extract question as the quest output and signal clarification routing.
			output = content
			isClarification = true
			c.logger.Info("tool-based clarification request",
				"quest_id", questID, "agent_id", mapping.AgentID)
		}
	} else {
		// Legacy fallback: detect clarification via intent tags or heuristic.
		isClarification = isOutputClarificationRequest(output)
	}

	quest.Output = output

	// Route clarification requests to the party lead or DM.
	if isClarification {
		// Enforce max clarification rounds to prevent infinite loops.
		if c.config.MaxClarificationRounds > 0 && c.clarificationRoundCount(quest) >= c.config.MaxClarificationRounds {
			c.logger.Warn("max clarification rounds exceeded — releasing agent and reposting quest",
				"quest_id", questID, "rounds", c.clarificationRoundCount(quest),
				"max", c.config.MaxClarificationRounds, "agent_id", mapping.AgentID)

			// Clarification exhaustion is not a work failure — the agent simply
			// couldn't proceed without more information. Release the agent and
			// repost so a more capable agent can claim it. Do NOT increment
			// Attempts: asking questions is not a penalty.
			c.releaseAgent(ctx, mapping.AgentID)
			quest.Status = domain.QuestPosted
			quest.Escalated = false
			quest.ClaimedBy = nil
			quest.ClaimedAt = nil
			quest.StartedAt = nil
			quest.Output = nil
			quest.FailureReason = ""
			quest.FailureType = ""
			// Preserve DMClarifications so the next agent has the Q&A context.

			if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.reposted"); err != nil {
				c.logger.Error("failed to repost quest after max clarifications",
					"quest_id", questID, "error", err)
				c.errorsCount.Add(1)
			}
			return
		}

		quest.Status = domain.QuestEscalated
		quest.Escalated = true
		quest.FailureReason = output
		// Agent stays assigned — ClaimedBy unchanged.

		if quest.PartyID != nil && quest.ParentQuest != nil {
			// Party sub-quest: route to party lead, not DM. The DAG state
			// machine picks up the escalated status and dispatches a
			// clarification task to the lead via the AGENT stream.
			// Note: the parent quest (lead's own quest) has PartyID but no
			// ParentQuest — it falls through to the DM clarification path.
			if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.dag.clarification_requested"); err != nil {
				c.logger.Error("failed to emit party clarification request",
					"quest_id", questID, "error", err)
				c.errorsCount.Add(1)
				return
			}
			c.logger.Info("party sub-quest clarification routed to lead",
				"quest_id", questID, "party_id", quest.PartyID, "agent_id", mapping.AgentID)
			return
		}

		// Non-party quest: escalate to DM for human clarification.
		if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.escalated"); err != nil {
			c.logger.Error("failed to emit quest escalation for clarification",
				"quest_id", questID, "error", err)
			c.errorsCount.Add(1)
			return
		}
		c.logger.Info("quest escalated — agent requesting clarification",
			"quest_id", questID, "agent_id", mapping.AgentID)
		return
	}

	// Quests with RequireReview=false (e.g., red-team review quests) skip
	// boss battle entirely and complete directly. Without this, they get
	// stuck at in_review because bossbattle correctly ignores them.
	if !quest.Constraints.RequireReview {
		quest.Status = domain.QuestCompleted
		if err := c.graph.EmitEntityUpdate(ctx, quest, domain.PredicateQuestCompleted); err != nil {
			c.logger.Error("failed to emit quest completion (no review required)",
				"quest_id", questID, "error", err)
			c.errorsCount.Add(1)
			return
		}
		c.logger.Info("quest completed directly (no review required)",
			"quest_id", questID)
		return
	}

	// All other quests route through in_review. Bossbattle determines the outcome:
	// auto-pass (trivial quests), LLM judge, or human review based on the
	// domain catalog's ReviewConfig and the quest's review level.
	// Sub-quests are skipped by bossbattle and reviewed by the party lead
	// via questdagexec instead.
	quest.Status = domain.QuestInReview
	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.submitted"); err != nil {
		c.logger.Error("failed to emit quest submission for review",
			"quest_id", questID, "error", err)
		c.errorsCount.Add(1)
		return
	}
	c.logger.Info("quest submitted for review",
		"quest_id", questID, "review_level", quest.Constraints.ReviewLevel)
}

// parseToolOutput attempts to parse the loop output as a tool-based completion
// JSON envelope produced by submit_work_product or ask_clarification.
// Returns the output type ("work_product" or "clarification"), the extracted
// content (deliverable or question), and whether parsing succeeded.
//
// For work_product: returns deliverable if present, otherwise summary.
// Summary-only submissions indicate file-based work captured via workspace snapshot.
func parseToolOutput(output string) (outputType, content string, ok bool) {
	var envelope struct {
		Type        string `json:"type"`
		Deliverable string `json:"deliverable"`
		Summary     string `json:"summary"`
		Question    string `json:"question"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &envelope); err != nil {
		return "", "", false
	}
	switch envelope.Type {
	case "work_product":
		// Prefer deliverable (inline content); fall back to summary (file-based work).
		text := envelope.Deliverable
		if text == "" {
			text = envelope.Summary
		}
		if text == "" {
			return "", "", false
		}
		return "work_product", text, true
	case "clarification":
		if envelope.Question == "" {
			return "", "", false
		}
		return "clarification", envelope.Question, true
	}
	return "", "", false
}

// isOutputClarificationRequest returns true when the agent's output is a
// clarification request rather than a work product.
// This is a fallback for models that ignore the tool-calling instructions.
//
// Detection strategy (ordered by reliability):
//  1. Structured intent tag: [INTENT: clarification] on the first non-empty line
//     (injected by the prompt assembler's response format instruction).
//  2. Heuristic fallback: majority of non-empty lines end with "?" — catches
//     agents that ignore the format instruction.
// hasIntentClarification checks only for a structured [INTENT: clarification]
// tag in the output. Unlike isOutputClarificationRequest, it does NOT use the
// "?" heuristic. Use this when the agent explicitly called submit_work_product
// — trust the tool choice, only override if the intent tag says otherwise.
func hasIntentClarification(output any) bool {
	text, ok := output.(string)
	if !ok {
		return false
	}
	trimmed := strings.TrimSpace(text)
	if len(trimmed) == 0 {
		return false
	}
	intent := parseOutputIntent(trimmed)
	return intent == "clarification"
}

func isOutputClarificationRequest(output any) bool {
	text, ok := output.(string)
	if !ok {
		return false
	}
	trimmed := strings.TrimSpace(text)
	if len(trimmed) == 0 {
		return false
	}

	// Strategy 1: Parse structured intent tag from the first non-empty line.
	if intent := parseOutputIntent(trimmed); intent != "" {
		return intent == "clarification"
	}

	// Strategy 2: Heuristic — any question mark in unstructured output.
	// If we reach this point, the model ignored tool-calling instructions
	// (no submit_work_product or ask_clarification tool call, no intent tag).
	// In that context, any "?" signals the model is asking for information
	// rather than delivering work. Route through clarification so the agent
	// isn't penalized by boss battle for a non-deliverable response.
	return strings.Contains(trimmed, "?")
}

// parseOutputIntent extracts the intent value from a structured [INTENT: <value>]
// tag on the first non-empty line. Returns the lowercase intent string (e.g.
// "work_product", "clarification") or empty string if no tag is found.
func parseOutputIntent(text string) string {
	lines := strings.SplitN(text, "\n", 2)
	first := strings.TrimSpace(lines[0])

	const prefix = "[INTENT:"
	const suffix = "]"
	if !strings.HasPrefix(first, prefix) || !strings.HasSuffix(first, suffix) {
		return ""
	}
	intent := first[len(prefix) : len(first)-len(suffix)]
	return strings.TrimSpace(strings.ToLower(intent))
}

// handleDAGDecomposition posts sub-quests from a validated DAG and initializes
// the DAG execution state as quest.dag.* predicates on the parent quest entity.
// Called only when quest.PartyRequired is true and the lead's loop output
// contains a valid DAG structure.
func (c *Component) handleDAGDecomposition(ctx context.Context, quest *domain.Quest, mapping *QuestLoopMapping, dag *questdagexec.QuestDAG, qb SubQuestPoster) error {
	// Convert DAG nodes to domain.Quest values for PostSubQuests.
	subQuests := dagNodesToQuests(dag.Nodes, quest)

	// Post sub-quests via questboard. This validates the decomposer's tier
	// (Master+), sets ParentQuest on each sub-quest, and writes them to KV.
	posted, err := qb.PostSubQuests(ctx, quest.ID, subQuests, mapping.AgentID)
	if err != nil {
		return fmt.Errorf("post sub-quests: %w", err)
	}

	// Log each sub-quest for traceability.
	for i, sq := range posted {
		c.logger.Debug("posted DAG sub-quest",
			"parent_quest_id", quest.ID,
			"sub_quest_id", sq.ID,
			"node_id", dag.Nodes[i].ID,
			"node_objective", dag.Nodes[i].Objective,
			"index", i)
	}
	c.logger.Info("posted DAG sub-quests",
		"parent_quest_id", quest.ID,
		"sub_quest_count", len(posted))

	// PostSubQuests is all-or-nothing but verify defensively.
	if len(posted) != len(dag.Nodes) {
		return fmt.Errorf("posted %d sub-quests, expected %d", len(posted), len(dag.Nodes))
	}

	// Build node ID → sub-quest entity ID map for the execution state.
	nodeQuestIDs := make(map[string]string, len(dag.Nodes))
	for i, node := range dag.Nodes {
		nodeQuestIDs[node.ID] = string(posted[i].ID)
	}

	// Apply the party assignment and DAG node ID to each posted sub-quest.
	// Party assignment makes sub-quests invisible to the general board;
	// DAG node ID enables questdagexec to correlate sub-quest transitions.
	for i, node := range dag.Nodes {
		if quest.PartyID != nil {
			posted[i].PartyID = quest.PartyID
		}
		posted[i].DAGNodeID = node.ID

		// Resolve DependsOn node IDs to real sub-quest entity IDs.
		if len(node.DependsOn) > 0 {
			deps := make([]domain.QuestID, 0, len(node.DependsOn))
			for _, depNodeID := range node.DependsOn {
				if depQuestID, ok := nodeQuestIDs[depNodeID]; ok {
					deps = append(deps, domain.QuestID(depQuestID))
				}
			}
			posted[i].DependsOn = deps
		}

		if emitErr := c.graph.EmitEntityUpdate(ctx, &posted[i], "quest.dag.sub_quest_initialized"); emitErr != nil {
			c.logger.Warn("failed to set DAG fields on sub-quest",
				"sub_quest_id", posted[i].ID, "node_id", node.ID, "error", emitErr)
		}
	}

	// Initialize DAG state as predicates on the parent quest entity.
	// questdagexec detects the new quest.dag.execution_id via the quest
	// entity watcher and builds its in-memory execution state.
	executionID := nuid.Next()
	nodeStates := make(map[string]string, len(dag.Nodes))
	for _, node := range dag.Nodes {
		nodeStates[node.ID] = questdagexec.NodePending
	}

	quest.DAGExecutionID = executionID
	quest.DAGDefinition = *dag
	quest.DAGNodeQuestIDs = nodeQuestIDs
	quest.DAGNodeStates = nodeStates

	c.logger.Debug("writing DAG predicates on parent quest",
		"execution_id", executionID,
		"parent_quest_id", quest.ID,
		"node_count", len(dag.Nodes))

	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.dag.decomposed"); err != nil {
		return fmt.Errorf("write DAG predicates on parent quest: %w", err)
	}
	c.logger.Info("DAG decomposition stored on parent quest entity",
		"execution_id", executionID,
		"parent_quest_id", quest.ID,
		"node_count", len(dag.Nodes))
	return nil
}

// =============================================================================
// DAG DETECTION AND CONVERSION (pure functions, no component receiver)
// =============================================================================

// extractDAGFromOutput parses the LLM's loop output and attempts to extract a
// validated QuestDAG. The decompose_quest tool produces output like:
//
//	{"goal": "...", "dag": {"nodes": [...]}}
//
// Returns (dag, true) when a structurally valid DAG is found, (nil, false)
// otherwise. Invalid JSON, missing keys, or validation errors all return false.
func extractDAGFromOutput(output string) (*questdagexec.QuestDAG, bool) {
	if output == "" {
		return nil, false
	}

	// Try to unmarshal the full output as the tool response JSON.
	// Use json.NewDecoder so trailing prose after a valid JSON object is
	// tolerated (LLMs often wrap tool output in markdown code fences).
	var raw map[string]any
	if err := json.NewDecoder(strings.NewReader(output)).Decode(&raw); err != nil {
		// Output is not top-level JSON — scan for a JSON object within it.
		start := strings.Index(output, "{")
		if start == -1 {
			return nil, false
		}
		if err2 := json.NewDecoder(strings.NewReader(output[start:])).Decode(&raw); err2 != nil {
			return nil, false
		}
	}

	// Must have both "goal" and "dag" keys per decompose_quest tool contract.
	if _, hasGoal := raw["goal"]; !hasGoal {
		return nil, false
	}
	dagRaw, hasDag := raw["dag"]
	if !hasDag {
		return nil, false
	}

	// Re-marshal and unmarshal the "dag" value into QuestDAG for type safety.
	dagData, err := json.Marshal(dagRaw)
	if err != nil {
		return nil, false
	}
	var dag questdagexec.QuestDAG
	if err := json.Unmarshal(dagData, &dag); err != nil {
		return nil, false
	}

	// Re-validate as defense in depth — the tool validated on creation but
	// the output may have been truncated or modified in transit.
	if err := dag.Validate(); err != nil {
		return nil, false
	}

	return &dag, true
}

// dagNodesToQuests converts a slice of QuestNode values from a DAG into
// domain.Quest values suitable for PostSubQuests. Skills are mapped from
// string tags to domain.SkillTag. Difficulty inherits from the parent when
// the node specifies zero. Title is truncated to 100 characters.
func dagNodesToQuests(nodes []questdagexec.QuestNode, parent *domain.Quest) []domain.Quest {
	quests := make([]domain.Quest, 0, len(nodes))
	for _, node := range nodes {
		title := node.Objective
		if len(title) > 100 {
			title = title[:100]
		}

		difficulty := domain.QuestDifficulty(node.Difficulty)
		if difficulty == 0 && parent != nil {
			difficulty = parent.Difficulty
		}

		skills := make([]domain.SkillTag, 0, len(node.Skills))
		for _, s := range node.Skills {
			skills = append(skills, domain.SkillTag(s))
		}

		quests = append(quests, domain.Quest{
			Title:          title,
			Description:    node.Objective,
			Difficulty:     difficulty,
			RequiredSkills: skills,
			Acceptance:     node.Acceptance,
		})
	}
	return quests
}

// failQuest transitions the quest to failed and releases the agent.
func (c *Component) failQuest(ctx context.Context, questID domain.QuestID, mapping *QuestLoopMapping, reason string, metrics loopMetrics) {
	questEntity, err := c.graph.GetQuest(ctx, questID)
	if err != nil {
		c.logger.Error("failed to load quest for failure", "quest_id", questID, "error", err)
		c.errorsCount.Add(1)
		return
	}
	quest := domain.QuestFromEntityState(questEntity)
	if quest == nil {
		c.logger.Error("quest reconstruction returned nil for failure", "quest_id", questID)
		c.errorsCount.Add(1)
		return
	}

	// Guard: only transition quests that are still in_progress. If the quest
	// was already failed/reposted/completed via the API while the agentic loop
	// was running, skip the transition to avoid overwriting the API's decision.
	if quest.Status != domain.QuestInProgress {
		c.logger.Warn("skipping quest failure — quest is no longer in_progress",
			"quest_id", questID, "current_status", quest.Status, "loop_id", mapping.LoopID)
		return
	}

	// Persist execution metrics from agentic-loop (partial metrics for failed quests).
	quest.TurnsUsed = metrics.TurnsUsed
	quest.TokensPrompt = metrics.TokensIn
	quest.TokensCompletion = metrics.TokensOut
	quest.Duration = time.Since(mapping.StartedAt)

	// Delegate to questboard when available — this routes through the triage
	// gate which may hold the quest for DM evaluation instead of terminal failure.
	// Questboard handles agent release internally.
	if qf := c.resolveQuestFailer(); qf != nil {
		// Ensure LoopID and metrics are set on the quest entity before delegating, so
		// questboard's FailQuest can capture them in the FailureRecord.
		quest.LoopID = mapping.LoopID
		if writeErr := c.graph.EmitEntityUpdate(ctx, quest, "quest.execution.loop_id"); writeErr != nil {
			c.logger.Error("failed to write loop_id before failQuest delegation", "error", writeErr)
		}
		if failErr := qf.FailQuest(ctx, questID, reason); failErr != nil {
			c.logger.Error("questboard FailQuest failed, falling back to direct failure",
				"quest_id", questID, "error", failErr)
			// Fall through to direct failure below.
		} else {
			return
		}
	}

	// Legacy path: set QuestFailed directly when no questboard is wired.
	quest.Status = domain.QuestFailed
	quest.LoopID = mapping.LoopID
	quest.FailureReason = reason

	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.failed"); err != nil {
		c.logger.Error("failed to emit quest failure", "quest_id", questID, "error", err)
		c.errorsCount.Add(1)
		return
	}

	c.releaseAgent(ctx, mapping.AgentID)
}

// releaseAgent sets the agent back to idle and clears its current quest.
func (c *Component) releaseAgent(ctx context.Context, agentID domain.AgentID) {
	agentEntity, err := c.graph.GetAgent(ctx, agentID)
	if err != nil {
		c.logger.Error("failed to load agent for release", "agent_id", agentID, "error", err)
		return
	}
	agent := agentprogression.AgentFromEntityState(agentEntity)
	if agent == nil {
		return
	}

	agent.Status = domain.AgentIdle
	agent.CurrentQuest = nil
	agent.UpdatedAt = time.Now()

	if err := c.graph.EmitEntityUpdate(ctx, agent, "agent.released"); err != nil {
		c.logger.Error("failed to release agent", "agent_id", agentID, "error", err)
	}
}

// =============================================================================
// STALE ESCALATION SWEEP
// =============================================================================

// sweepStaleEscalations periodically checks for escalated quests that have been
// waiting for DM clarification longer than EscalationTimeoutMins. When found,
// the agent is released and the quest is reposted for a different agent.
func (c *Component) sweepStaleEscalations(ctx context.Context) {
	timeout := time.Duration(c.config.EscalationTimeoutMins) * time.Minute
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.escalatedAt.Range(func(key, value any) bool {
				entityKey := key.(string)
				escalatedTime := value.(time.Time)
				if time.Since(escalatedTime) < timeout {
					return true
				}
				c.handleStaleEscalation(ctx, entityKey)
				return true
			})
		}
	}
}

// handleStaleEscalation releases the agent and reposts a quest that has been
// escalated for longer than the configured timeout.
func (c *Component) handleStaleEscalation(ctx context.Context, entityKey string) {
	c.escalatedAt.Delete(entityKey)

	// The entity key is the full entity ID (e.g., c360.prod.game.board1.quest.abc123).
	// GetQuest calls ExtractInstance internally, so the full ID works as a QuestID.
	questEntity, err := c.graph.GetQuest(ctx, domain.QuestID(entityKey))
	if err != nil {
		c.logger.Warn("failed to load quest for stale escalation sweep",
			"entity_key", entityKey, "error", err)
		return
	}
	quest := domain.QuestFromEntityState(questEntity)
	if quest == nil {
		return
	}

	// Guard: only act on quests that are still escalated.
	if quest.Status != domain.QuestEscalated {
		return
	}

	c.logger.Warn("escalation timeout — releasing agent and reposting quest",
		"quest_id", quest.ID,
		"agent_id", quest.ClaimedBy,
		"timeout_mins", c.config.EscalationTimeoutMins)

	// Release the agent.
	if quest.ClaimedBy != nil {
		c.releaseAgent(ctx, *quest.ClaimedBy)
	}

	// Repost the quest so a different agent can claim it.
	// Preserve DMClarifications so the next agent has partial clarification context.
	quest.Status = domain.QuestPosted
	quest.Escalated = false
	quest.ClaimedBy = nil
	quest.ClaimedAt = nil
	quest.StartedAt = nil
	quest.Output = nil
	quest.FailureReason = ""
	quest.FailureType = ""
	// Do not increment Attempts — a clarification is not a failed work attempt.

	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.reposted"); err != nil {
		c.logger.Error("failed to repost quest after escalation timeout",
			"quest_id", quest.ID, "error", err)
		c.errorsCount.Add(1)
	}
}

// clarificationRoundCount returns how many DM clarification exchanges exist
// for this quest. Used to enforce MaxClarificationRounds.
func (c *Component) clarificationRoundCount(quest *domain.Quest) int {
	if quest.DMClarifications == nil {
		return 0
	}
	raw, err := json.Marshal(quest.DMClarifications)
	if err != nil {
		return 0
	}
	var exchanges []domain.ClarificationExchange
	if json.Unmarshal(raw, &exchanges) != nil {
		return 0
	}
	return len(exchanges)
}

// autoAnswerClarification uses the ClarificationAnswerer to automatically
// answer an agent's clarification question via LLM. Called asynchronously
// when DMMode is full_auto and a quest transitions to escalated.
func (c *Component) autoAnswerClarification(ctx context.Context, entityKey string) {
	questEntity, err := c.graph.GetQuest(ctx, domain.QuestID(entityKey))
	if err != nil {
		c.logger.Warn("auto-DM: failed to load escalated quest",
			"entity_key", entityKey, "error", err)
		return
	}
	quest := domain.QuestFromEntityState(questEntity)
	if quest == nil {
		return
	}

	// Guard: only act on quests that are still escalated.
	if quest.Status != domain.QuestEscalated {
		return
	}

	// Guard: skip sub-quest clarifications — those route to the party lead.
	// Parent party quests (lead's own quest) have PartyID but no ParentQuest;
	// they should be auto-answered by the DM like any other top-level quest.
	if quest.PartyID != nil && quest.ParentQuest != nil {
		return
	}

	// Guard: only auto-answer when the output is actually a clarification request.
	// Quests escalated for other reasons (e.g. DAG node exhaustion) should not
	// be auto-answered — they are truly terminal escalations.
	outputText, _ := quest.Output.(string)

	// Try tool-based JSON output first, then fall back to intent tag detection.
	var question string
	if _, content, ok := parseToolOutput(outputText); ok {
		// Tool-based output: content is already the clean question text.
		question = content
	} else if !isOutputClarificationRequest(outputText) {
		c.logger.Debug("auto-DM: escalated quest has no clarification intent, skipping",
			"quest_id", quest.ID)
		return
	} else {
		// Legacy: extract question from raw output, stripping intent header.
		question = strings.TrimSpace(outputText)
		if strings.HasPrefix(question, "[INTENT:") {
			if idx := strings.Index(question, "\n"); idx >= 0 {
				question = strings.TrimSpace(question[idx+1:])
			}
		}
	}

	if question == "" {
		c.logger.Warn("auto-DM: no clarification question found on escalated quest",
			"quest_id", quest.ID)
		return
	}

	c.logger.Info("auto-DM: answering clarification via LLM",
		"quest_id", quest.ID, "question_len", len(question))

	answer, err := c.clarificationAnswerer.AnswerClarification(ctx,
		quest.Title, quest.Description, question)
	if err != nil {
		c.logger.Error("auto-DM: LLM call failed",
			"quest_id", quest.ID, "error", err)
		return
	}

	// Re-read the quest to get the latest state (may have been answered by human DM
	// between our initial read and the LLM call completing).
	questEntity, err = c.graph.GetQuest(ctx, domain.QuestID(entityKey))
	if err != nil {
		c.logger.Error("auto-DM: failed to re-read quest after LLM call",
			"quest_id", quest.ID, "error", err)
		return
	}
	quest = domain.QuestFromEntityState(questEntity)
	if quest == nil || quest.Status != domain.QuestEscalated {
		c.logger.Info("auto-DM: quest no longer escalated (human DM may have answered)",
			"quest_id", entityKey)
		return
	}

	// Build the clarification exchange.
	var exchanges []domain.ClarificationExchange
	if quest.DMClarifications != nil {
		raw, _ := json.Marshal(quest.DMClarifications)
		json.Unmarshal(raw, &exchanges) //nolint:errcheck // best-effort
	}
	exchanges = append(exchanges, domain.ClarificationExchange{
		Question: question,
		Answer:   answer,
		AskedAt:  time.Now(),
	})
	quest.DMClarifications = exchanges

	// Resume the quest with the same agent.
	quest.Status = domain.QuestInProgress
	quest.Escalated = false
	quest.Output = nil
	quest.FailureReason = ""
	quest.FailureType = ""

	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.dm.clarification_answered"); err != nil {
		c.logger.Error("auto-DM: failed to resume quest after clarification",
			"quest_id", quest.ID, "error", err)
		c.errorsCount.Add(1)
		return
	}

	c.logger.Info("auto-DM: clarification answered, quest resumed",
		"quest_id", quest.ID, "rounds", len(exchanges))
}

// =============================================================================
// CRASH RECOVERY
// =============================================================================

// reconcileOrphanedQuests runs after bootstrap to find in_progress quests that
// lack active loop mappings — quests that may have been running when a previous
// instance crashed. Logs warnings for manual intervention.
func (c *Component) reconcileOrphanedQuests(ctx context.Context) {
	c.logger.Info("reconciling orphaned quests after bootstrap")

	c.questCache.Range(func(key, value any) bool {
		status, _ := value.(string)

		// Seed escalation timestamps for quests already escalated at startup.
		// Uses current time as a conservative estimate — the quest gets the full
		// timeout from this restart rather than expiring immediately.
		if status == string(domain.QuestEscalated) {
			entityKey, _ := key.(string)
			c.escalatedAt.Store(entityKey, time.Now())
			c.logger.Info("seeded escalation timestamp for pre-existing escalated quest",
				"entity_key", entityKey)
		}

		if status != string(domain.QuestInProgress) {
			return true
		}

		entityKey, _ := key.(string)

		// QUEST_LOOPS keys are the full entity ID (same key handleQuestStarted
		// uses when writing the mapping). Use the full entity key for lookup.
		entry, err := c.questLoopsBucket.Get(ctx, entityKey)
		if err != nil {
			// No mapping — quest may predate bridge deployment or was orphaned.
			c.logger.Warn("orphaned in_progress quest with no loop mapping — manual review required",
				"entity_key", entityKey)
			return true
		}

		var mapping QuestLoopMapping
		if err := json.Unmarshal(entry.Value(), &mapping); err != nil {
			c.logger.Warn("failed to unmarshal recovered mapping",
				"entity_key", entityKey, "error", err)
			return true
		}

		// Re-register the mapping under the full entity ID — same key that
		// findMapping uses for lookup when completion events arrive.
		c.activeLoops.Store(entityKey, &mapping)
		c.logger.Info("recovered quest-loop mapping after restart",
			"quest_id", mapping.QuestID,
			"loop_id", mapping.LoopID,
			"started_at", mapping.StartedAt)

		return true
	})
}

// findMapping looks up a quest-loop mapping first from the in-memory activeLoops
// cache, then falls back to the QUEST_LOOPS KV bucket for crash-recovered state.
//
// Execution loops are keyed by questID. Review/clarify loops (dispatched by
// questdagexec) are keyed by loopID since multiple loops share the same quest ID.
// When the questID lookup misses, loopID is tried as a fallback.
func (c *Component) findMapping(ctx context.Context, questID, loopID string) *QuestLoopMapping {
	// Try questID first (execution loops).
	if v, ok := c.activeLoops.Load(questID); ok {
		return v.(*QuestLoopMapping)
	}
	if entry, err := c.questLoopsBucket.Get(ctx, questID); err == nil {
		var mapping QuestLoopMapping
		if err := json.Unmarshal(entry.Value(), &mapping); err == nil {
			return &mapping
		}
	}

	// Try loopID (review/clarify loops keyed by loopID).
	if loopID != "" {
		if v, ok := c.activeLoops.Load(loopID); ok {
			return v.(*QuestLoopMapping)
		}
		if entry, err := c.questLoopsBucket.Get(ctx, loopID); err == nil {
			var mapping QuestLoopMapping
			if err := json.Unmarshal(entry.Value(), &mapping); err == nil {
				return &mapping
			}
		}
	}

	return nil
}

// cleanupMapping removes the quest-loop mapping from both the in-memory cache
// and the persistent QUEST_LOOPS KV bucket.
func (c *Component) cleanupMapping(ctx context.Context, questID string) {
	c.activeLoops.Delete(questID)
	if err := c.questLoopsBucket.Delete(ctx, questID); err != nil {
		c.logger.Debug("failed to delete QUEST_LOOPS mapping (may already be gone)",
			"quest_id", questID, "error", err)
	}
}

// =============================================================================
// PROMPT BUILDING
// =============================================================================

// loadPeerFeedback returns PeerFeedbackSummary items for each review question
// where the agent's running average is below threshold (3.0). Only questions
// with low ratings are included — the assembler emits them as "You MUST address
// these" directives. Returns nil when the agent has no reviews or all questions
// are at or above threshold.
func loadPeerFeedback(agent *agentprogression.Agent) []promptmanager.PeerFeedbackSummary {
	const ratingThreshold = 3.0
	if agent.Stats.PeerReviewCount == 0 {
		return nil
	}

	type qAvg struct {
		question string
		avg      float64
	}
	questions := []qAvg{
		{domain.LeaderToMemberQuestions[0], agent.Stats.PeerReviewQ1Avg},
		{domain.LeaderToMemberQuestions[1], agent.Stats.PeerReviewQ2Avg},
		{domain.LeaderToMemberQuestions[2], agent.Stats.PeerReviewQ3Avg},
	}

	var feedback []promptmanager.PeerFeedbackSummary
	for _, q := range questions {
		if q.avg > 0 && q.avg < ratingThreshold {
			feedback = append(feedback, promptmanager.PeerFeedbackSummary{
				Question:  q.question,
				AvgRating: q.avg,
			})
		}
	}
	return feedback
}

// loadClarificationAnswers reads clarification exchanges from the quest entity.
// For sub-quests in a party DAG, it loads from DAGClarifications (party lead answered).
// For standalone or parent quests, it loads from DMClarifications (DM answered).
// Returns nil when no prior clarifications are present.
func (c *Component) loadClarificationAnswers(quest *domain.Quest) []promptmanager.ClarificationAnswer {
	// Sub-quest path: load from DAGClarifications (party lead answered).
	if quest.PartyID != nil && quest.DAGClarifications != nil {
		// DAGClarifications is stored as any (JSON-serialized []ClarificationExchange).
		// After KV round-trip it may arrive as []any of map[string]any.
		raw, err := json.Marshal(quest.DAGClarifications)
		if err != nil {
			return nil
		}
		var exchanges []questdagexec.ClarificationExchange
		if json.Unmarshal(raw, &exchanges) != nil {
			return nil
		}
		if len(exchanges) == 0 {
			return nil
		}
		answers := make([]promptmanager.ClarificationAnswer, len(exchanges))
		for i, ex := range exchanges {
			answers[i] = promptmanager.ClarificationAnswer{
				Question: ex.Question,
				Answer:   ex.Answer,
			}
		}
		return answers
	}

	// Standalone/parent quest path: load from DMClarifications (DM answered).
	if quest.DMClarifications != nil {
		raw, err := json.Marshal(quest.DMClarifications)
		if err != nil {
			return nil
		}
		var exchanges []domain.ClarificationExchange
		if json.Unmarshal(raw, &exchanges) != nil {
			return nil
		}
		if len(exchanges) == 0 {
			return nil
		}
		answers := make([]promptmanager.ClarificationAnswer, len(exchanges))
		for i, ex := range exchanges {
			answers[i] = promptmanager.ClarificationAnswer{
				Question: ex.Question,
				Answer:   ex.Answer,
			}
		}
		return answers
	}

	return nil
}

// clarificationSource returns a human-readable label for who answered the
// agent's clarification questions, used in prompt section headers.
func (c *Component) clarificationSource(quest *domain.Quest) string {
	if quest.PartyID != nil && quest.DAGClarifications != nil {
		return "The party lead"
	}
	if quest.DMClarifications != nil {
		return "The DM"
	}
	return ""
}

// resolveDependencyOutputs returns DependencyOutput slices for backward-compat
// rendering when structured deps are not enabled. When EnableStructuredDeps is
// true this returns nil so DependencyContexts carries the context instead.
func (c *Component) resolveDependencyOutputs(ctx context.Context, quest *domain.Quest) []promptmanager.DependencyOutput {
	if c.config.EnableStructuredDeps {
		return nil
	}
	return c.loadDependencyOutputs(ctx, quest)
}

// resolveDependencyContexts returns the structured dependency contexts when
// EnableStructuredDeps is true. Returns nil otherwise to keep backward compat.
func (c *Component) resolveDependencyContexts(ctx context.Context, quest *domain.Quest) []promptmanager.DependencyContext {
	if !c.config.EnableStructuredDeps {
		return nil
	}
	return c.loadDependencyContext(ctx, quest)
}

// loadDependencyOutputs loads completed predecessor node outputs for a sub-quest
// that is part of a DAG. When a sub-quest has depends_on predecessors, their
// outputs are loaded from the graph and returned as DependencyOutput structs
// for injection into the agent's system prompt.
func (c *Component) loadDependencyOutputs(ctx context.Context, quest *domain.Quest) []promptmanager.DependencyOutput {
	if quest.ParentQuest == nil || quest.DAGNodeID == "" {
		return nil
	}

	// Load the parent quest to get DAG definition and node-quest mappings.
	parentEntity, err := c.graph.GetQuest(ctx, *quest.ParentQuest)
	if err != nil {
		c.logger.Debug("failed to load parent quest for dependency outputs",
			"parent_quest", *quest.ParentQuest, "error", err)
		return nil
	}
	parentQuest := domain.QuestFromEntityState(parentEntity)
	if parentQuest == nil {
		return nil
	}

	// Parse DAG definition to find this node's dependencies.
	dagDef, nodeQuestIDs := c.parseDAGFromParent(parentQuest)
	if dagDef == nil {
		return nil
	}

	// Find this node in the DAG.
	var thisNode *questdagexec.QuestNode
	for i := range dagDef.Nodes {
		if dagDef.Nodes[i].ID == quest.DAGNodeID {
			thisNode = &dagDef.Nodes[i]
			break
		}
	}
	if thisNode == nil || len(thisNode.DependsOn) == 0 {
		return nil
	}

	// Build a node index for objective lookup.
	nodesByID := make(map[string]*questdagexec.QuestNode, len(dagDef.Nodes))
	for i := range dagDef.Nodes {
		nodesByID[dagDef.Nodes[i].ID] = &dagDef.Nodes[i]
	}

	// Load each predecessor's output.
	var outputs []promptmanager.DependencyOutput
	for _, depNodeID := range thisNode.DependsOn {
		depQuestID, ok := nodeQuestIDs[depNodeID]
		if !ok {
			continue
		}
		depEntity, loadErr := c.graph.GetQuest(ctx, domain.QuestID(depQuestID))
		if loadErr != nil {
			c.logger.Debug("failed to load dependency quest output",
				"dep_node_id", depNodeID, "dep_quest_id", depQuestID, "error", loadErr)
			continue
		}
		depQuest := domain.QuestFromEntityState(depEntity)
		if depQuest == nil || depQuest.Output == nil {
			continue
		}

		outputStr := fmt.Sprintf("%v", depQuest.Output)
		objective := depNodeID
		if node, found := nodesByID[depNodeID]; found {
			objective = node.Objective
		}

		outputs = append(outputs, promptmanager.DependencyOutput{
			NodeID:    depNodeID,
			Objective: objective,
			Output:    outputStr,
		})
	}

	if len(outputs) > 0 {
		c.logger.Debug("loaded dependency outputs for sub-quest",
			"quest_id", quest.ID, "node_id", quest.DAGNodeID, "dep_count", len(outputs))
	}
	return outputs
}

// parseDAGFromParent extracts the QuestDAG and node-quest ID mapping from a
// parent quest's DAG fields. Returns (nil, nil) if the parent has no DAG data.
func (c *Component) parseDAGFromParent(parent *domain.Quest) (*questdagexec.QuestDAG, map[string]string) {
	if parent.DAGDefinition == nil || parent.DAGNodeQuestIDs == nil {
		return nil, nil
	}

	// DAGDefinition is stored as any — round-trip through JSON.
	defBytes, err := json.Marshal(parent.DAGDefinition)
	if err != nil {
		return nil, nil
	}
	var dag questdagexec.QuestDAG
	if json.Unmarshal(defBytes, &dag) != nil {
		return nil, nil
	}

	// NodeQuestIDs is stored as any — round-trip through JSON.
	idsBytes, err := json.Marshal(parent.DAGNodeQuestIDs)
	if err != nil {
		return nil, nil
	}
	var nodeQuestIDs map[string]string
	if json.Unmarshal(idsBytes, &nodeQuestIDs) != nil {
		return nil, nil
	}

	return &dag, nodeQuestIDs
}

// loadDependencyContext loads predecessor DAG node context using the three-tier
// resolution cascade. It is the structured-deps alternative to loadDependencyOutputs
// and is only called when c.config.EnableStructuredDeps is true.
//
// Resolution tiers per predecessor:
//  1. structured — ArtifactsIndexed==true: fetch entity identity from graph-gateway,
//     format as compact file-grouped listing, add graph_search drill-down hint.
//  2. summary — Output available but not indexed: truncated raw output with a note
//     that full artifacts are queryable once indexing completes.
//  3. raw — Fallback: raw output string, budget-capped.
//
// Failures at any tier fall through to the next. The method never blocks dispatch:
// any graph-gateway call that errors returns nil and falls to a lower tier.
func (c *Component) loadDependencyContext(ctx context.Context, quest *domain.Quest) []promptmanager.DependencyContext {
	if quest.ParentQuest == nil || quest.DAGNodeID == "" {
		return nil
	}

	// Load parent quest to get DAG definition and node-quest ID mapping.
	parentEntity, err := c.graph.GetQuest(ctx, *quest.ParentQuest)
	if err != nil {
		c.logger.Debug("loadDependencyContext: failed to load parent quest",
			"parent_quest", *quest.ParentQuest, "error", err)
		return nil
	}
	parentQuest := domain.QuestFromEntityState(parentEntity)
	if parentQuest == nil {
		return nil
	}

	dagDef, nodeQuestIDs := c.parseDAGFromParent(parentQuest)
	if dagDef == nil {
		return nil
	}

	// Find this node's declared dependencies.
	var thisNode *questdagexec.QuestNode
	for i := range dagDef.Nodes {
		if dagDef.Nodes[i].ID == quest.DAGNodeID {
			thisNode = &dagDef.Nodes[i]
			break
		}
	}
	if thisNode == nil || len(thisNode.DependsOn) == 0 {
		return nil
	}

	// Build node-objective index for display purposes.
	nodesByID := make(map[string]*questdagexec.QuestNode, len(dagDef.Nodes))
	for i := range dagDef.Nodes {
		nodesByID[dagDef.Nodes[i].ID] = &dagDef.Nodes[i]
	}

	budget := c.config.DependencyContextBudget
	if budget <= 0 {
		budget = 800
	}

	var contexts []promptmanager.DependencyContext
	for _, depNodeID := range thisNode.DependsOn {
		depQuestID, ok := nodeQuestIDs[depNodeID]
		if !ok {
			continue
		}
		depEntity, loadErr := c.graph.GetQuest(ctx, domain.QuestID(depQuestID))
		if loadErr != nil {
			c.logger.Debug("loadDependencyContext: failed to load dep quest",
				"dep_node_id", depNodeID, "dep_quest_id", depQuestID, "error", loadErr)
			continue
		}
		depQuest := domain.QuestFromEntityState(depEntity)
		if depQuest == nil {
			continue
		}

		objective := depNodeID
		if node, found := nodesByID[depNodeID]; found {
			objective = node.Objective
		}

		dc := c.resolveDepContext(ctx, depQuest, depNodeID, objective, budget)
		contexts = append(contexts, dc)
	}

	if len(contexts) > 0 {
		c.logger.Debug("loadDependencyContext: resolved dependency contexts",
			"quest_id", quest.ID, "node_id", quest.DAGNodeID, "dep_count", len(contexts))
	}
	return contexts
}

// resolveDepContext applies the three-tier resolution cascade for a single
// predecessor quest. The result always has a valid ResolutionMode.
func (c *Component) resolveDepContext(ctx context.Context, depQuest *domain.Quest, nodeID, objective string, budget int) promptmanager.DependencyContext {
	base := promptmanager.DependencyContext{
		NodeID:    nodeID,
		Objective: objective,
	}

	// Tier 1: structured — artifacts have been indexed by semsource.
	if depQuest.ArtifactsIndexed && len(depQuest.ProducedEntities) > 0 && c.config.GraphQLURL != "" {
		if summary, entityRefs := c.fetchStructuredSummary(ctx, depQuest.ProducedEntities, budget); summary != "" {
			base.Summary = summary
			base.EntityRefs = entityRefs
			base.ResolutionMode = "structured"
			return base
		}
	}

	// Tier 2: summary — output exists but not yet indexed (or GraphQL unavailable).
	rawOutput := fmt.Sprintf("%v", depQuest.Output)
	if depQuest.Output != nil && rawOutput != "" && rawOutput != "<nil>" {
		summary := rawOutput
		if pkgcontext.EstimateTokens(summary) > budget {
			summary = pkgcontext.TruncateToBudget(summary, budget)
		}
		base.Summary = summary
		// Cap RawOutput to prevent unbounded KV entry sizes if serialized.
		base.RawOutput = pkgcontext.TruncateToBudget(rawOutput, budget*2)
		base.ResolutionMode = "summary"
		return base
	}

	// Tier 3: raw fallback — no output yet (e.g. the quest completed without output).
	base.ResolutionMode = "raw"
	return base
}

// entityTriplesResponse mirrors the GraphQL response shape for an entity triples query.
type entityTriplesResponse struct {
	Data struct {
		Entity struct {
			Triples []struct {
				Predicate string `json:"predicate"`
				Object    string `json:"object"`
			} `json:"triples"`
		} `json:"entity"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors,omitempty"`
}

// fetchStructuredSummary queries graph-gateway for the identity triples of the
// given entity IDs and formats them as a compact file-grouped listing. Returns
// ("", nil) on any error so callers fall through to the next resolution tier.
// The call uses a hard 500 ms timeout — it must never delay quest dispatch.
func (c *Component) fetchStructuredSummary(ctx context.Context, entityIDs []string, budget int) (string, []string) {
	// Fetch each entity individually, up to a reasonable cap to avoid runaway
	// requests on very large ProducedEntities slices.
	const maxEntities = 20
	if len(entityIDs) > maxEntities {
		entityIDs = entityIDs[:maxEntities]
	}

	reqCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	// Group results by "file path" heuristic: the entity ID's instance segment
	// (last dot component) is used as the grouping key.
	type fileGroup struct {
		path    string
		entries []string
	}
	groupMap := make(map[string]*fileGroup)
	var groupOrder []string
	var entityRefs []string

	// Identity predicates we care about — filter noise from game-world predicates.
	isIdentityPredicate := func(pred string) bool {
		for _, prefix := range []string{
			"source.identity", "source.content", "doc.identity", "doc.content",
			"config.identity", "config.content",
		} {
			if strings.HasPrefix(pred, prefix) {
				return true
			}
		}
		return false
	}

	for _, entityID := range entityIDs {
		query := fmt.Sprintf(
			`{"query":"{ entity(id: %q) { triples { predicate object } } }"}`,
			entityID,
		)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, c.config.GraphQLURL,
			strings.NewReader(query))
		if err != nil {
			c.logger.Debug("fetchStructuredSummary: failed to build request", "entity_id", entityID, "error", err)
			return "", nil
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, doErr := depGraphHTTPClient.Do(req)
		if doErr != nil {
			c.logger.Debug("fetchStructuredSummary: HTTP error", "entity_id", entityID, "error", doErr)
			return "", nil
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
		resp.Body.Close()
		if readErr != nil || resp.StatusCode != http.StatusOK {
			c.logger.Debug("fetchStructuredSummary: bad response", "entity_id", entityID, "status", resp.StatusCode)
			return "", nil
		}

		var gqlResp entityTriplesResponse
		if json.Unmarshal(body, &gqlResp) != nil || len(gqlResp.Errors) > 0 {
			c.logger.Debug("fetchStructuredSummary: GraphQL error or parse fail", "entity_id", entityID)
			return "", nil
		}

		entityRefs = append(entityRefs, entityID)

		// Derive a grouping key from the entity ID (last segment is the instance).
		groupKey := entityID
		if idx := strings.LastIndexByte(entityID, '.'); idx >= 0 {
			groupKey = entityID[:idx] // strip instance, use type prefix as path
		}

		if _, seen := groupMap[groupKey]; !seen {
			groupMap[groupKey] = &fileGroup{path: groupKey}
			groupOrder = append(groupOrder, groupKey)
		}

		for _, triple := range gqlResp.Data.Entity.Triples {
			if !isIdentityPredicate(triple.Predicate) {
				continue
			}
			// Format as "<predicate-leaf>: <object>"
			leaf := triple.Predicate
			if idx := strings.LastIndexByte(triple.Predicate, '.'); idx >= 0 {
				leaf = triple.Predicate[idx+1:]
			}
			groupMap[groupKey].entries = append(groupMap[groupKey].entries,
				fmt.Sprintf("  %s: %s", leaf, triple.Object))
		}
	}

	if len(groupOrder) == 0 {
		return "", nil
	}

	// Format as compact file-grouped listing.
	var sb strings.Builder
	for _, key := range groupOrder {
		g := groupMap[key]
		sb.WriteString(g.path + ":\n")
		for _, entry := range g.entries {
			sb.WriteString(entry + "\n")
		}
	}
	if len(entityRefs) > 0 {
		sb.WriteString("\n→ Drill down: graph_search(query_type=\"entity\", entity_id=\"<entity-id>\")\n")
		sb.WriteString("  Entity IDs: " + strings.Join(entityRefs, ", ") + "\n")
	}

	result := sb.String()
	if pkgcontext.EstimateTokens(result) > budget {
		result = pkgcontext.TruncateToBudget(result, budget)
	}
	return result, entityRefs
}

// buildSystemPrompt builds the system prompt using the assembler when available,
// falling back to the legacy string concatenation path. Returns the full
// AssembledPrompt so callers can access FragmentsUsed for context metadata.
func (c *Component) buildSystemPrompt(ctx context.Context, agent *agentprogression.Agent, quest *domain.Quest, toolNames []string) promptmanager.AssembledPrompt {
	if c.promptAssembler != nil {
		return c.buildAssembledSystemPrompt(ctx, agent, quest, toolNames)
	}
	return promptmanager.AssembledPrompt{SystemMessage: buildLegacySystemPrompt(agent, quest)}
}

func (c *Component) buildAssembledSystemPrompt(ctx context.Context, agent *agentprogression.Agent, quest *domain.Quest, toolNames []string) promptmanager.AssembledPrompt {
	var personaPrompt string
	if agent.Persona != nil {
		personaPrompt = agent.Persona.SystemPrompt
	}

	var maxDuration string
	if quest.Constraints.MaxDuration > 0 {
		maxDuration = quest.Constraints.MaxDuration.String()
	}

	provider := agent.Config.Provider
	if provider == "" && c.registry != nil {
		capability := c.resolveCapability(agent, quest)
		endpointName := c.registry.Resolve(capability)
		if ep := c.registry.GetEndpoint(endpointName); ep != nil {
			provider = ep.Provider
		}
	}

	// Inject structural checklist and review criteria from domain catalog.
	// Filtered by quest tier and skills — agents only see requirements relevant to their work.
	var checklist []promptmanager.ChecklistItem
	var reviewLevel domain.ReviewLevel
	var reviewCriteria []domain.ReviewCriterion
	if c.config.DomainCatalog != nil && c.config.DomainCatalog.ReviewConfig != nil {
		rc := c.config.DomainCatalog.ReviewConfig
		checklist = promptmanager.FilterChecklist(
			rc.StructuralChecklist,
			quest.MinTier,
			quest.RequiredSkills,
		)
		reviewLevel = quest.Constraints.ReviewLevel
		// Per-level criteria override, then domain defaults.
		if rc.CriteriaByLevel != nil {
			if criteria, ok := rc.CriteriaByLevel[reviewLevel]; ok && len(criteria) > 0 {
				reviewCriteria = criteria
			}
		}
		if len(reviewCriteria) == 0 {
			reviewCriteria = rc.DefaultCriteria
		}
	}

	assemblyCtx := promptmanager.AssemblyContext{
		AgentID:              agent.ID,
		Tier:                 agent.Tier,
		Level:                agent.Level,
		Skills:               agent.SkillProficiencies,
		Archetype:            agent.Archetype,
		Guild:                agent.Guild,
		SystemPrompt:         agent.Config.SystemPrompt,
		PersonaPrompt:        personaPrompt,
		QuestTitle:           quest.Title,
		QuestDescription:     quest.Description,
		QuestInput:           quest.Input,
		RequiredSkills:       quest.RequiredSkills,
		MaxDuration:          maxDuration,
		MaxTokens:            quest.Constraints.MaxTokens,
		Provider:             provider,
		PeerFeedback:         loadPeerFeedback(agent),
		PartyRequired:        quest.PartyRequired,
		IsPartyLead:          quest.PartyRequired && agent.Tier >= domain.TierMaster,
		IsSubQuest:           quest.ParentQuest != nil,
		ClarificationAnswers: c.loadClarificationAnswers(quest),
		ClarificationSource:  c.clarificationSource(quest),
		DependencyOutputs:    c.resolveDependencyOutputs(ctx, quest),
		DependencyContexts:   c.resolveDependencyContexts(ctx, quest),
		StructuralChecklist:  checklist,
		ReviewLevel:          reviewLevel,
		ReviewCriteria:       reviewCriteria,
		QuestGoal:            quest.Goal,
		QuestRequirements:    quest.Requirements,
		QuestScenarios:       quest.Scenarios,
		DecomposabilityClass: quest.DecomposabilityClass,
		AvailableToolNames:   toolNames,
		MaxIterations:        maxIterationsForDifficulty(c.config.MaxIterations, quest.Difficulty),
		QuestType:             quest.QuestType,
		GuildLessons:          c.loadGuildLessons(ctx, agent, quest),
		RedTeamTargetOutput:   nil, // Set below after single target load.
		RedTeamTargetTitle:    "",
		WorkspaceHasPriorWork: quest.Attempts > 1 && quest.ParentQuest == nil,
		FailureHistory:        convertFailureHistory(quest.FailureHistory),
		SalvagedOutput:       domain.AsString(quest.SalvagedOutput),
		FailureAnalysis:      quest.FailureAnalysis,
		RecoveryPath:         string(quest.RecoveryPath),
		AntiPatterns:         quest.AntiPatterns,
	}

	// Load red-team target in a single KV read (avoids double-read for output + title).
	if quest.QuestType == domain.QuestTypeRedTeam && quest.RedTeamTarget != nil {
		if targetEntity, err := c.graph.GetQuest(ctx, *quest.RedTeamTarget); err == nil && targetEntity != nil {
			target := domain.QuestFromEntityState(targetEntity)
			if target != nil {
				assemblyCtx.RedTeamTargetOutput = target.Output
				assemblyCtx.RedTeamTargetTitle = target.Title
			}
		}
	}

	return c.promptAssembler.AssembleSystemPrompt(assemblyCtx)
}

// convertFailureHistory maps domain.FailureRecord to promptmanager.FailureHistorySummary
// for injection into the prompt assembly context.
func convertFailureHistory(records []domain.FailureRecord) []promptmanager.FailureHistorySummary {
	if len(records) == 0 {
		return nil
	}
	summaries := make([]promptmanager.FailureHistorySummary, len(records))
	for i, r := range records {
		summaries[i] = promptmanager.FailureHistorySummary{
			Attempt:       r.Attempt,
			FailureType:   string(r.FailureType),
			FailureReason: r.FailureReason,
			TriageVerdict: r.TriageVerdict,
		}
	}
	return summaries
}

// loadGuildLessons returns relevant guild lessons for the agent's guild,
// filtered to only include lessons matching the quest's required skills.
func (c *Component) loadGuildLessons(ctx context.Context, agent *agentprogression.Agent, quest *domain.Quest) []domain.Lesson {
	if agent.Guild == "" {
		return nil
	}

	guildEntity, err := c.graph.GetGuild(ctx, agent.Guild)
	if err != nil {
		return nil
	}
	guild := domain.GuildFromEntityState(guildEntity)
	if guild == nil || len(guild.Lessons) == 0 {
		return nil
	}

	// Build skill set for quick lookup.
	questSkills := make(map[domain.SkillTag]struct{}, len(quest.RequiredSkills))
	for _, s := range quest.RequiredSkills {
		questSkills[s] = struct{}{}
	}

	// Filter lessons to those matching the quest's skills.
	var relevant []domain.Lesson
	for _, lesson := range guild.Lessons {
		if _, match := questSkills[lesson.Skill]; match {
			relevant = append(relevant, lesson)
		}
	}

	// Cap at 10 lessons to stay within context budget.
	if len(relevant) > 10 {
		relevant = relevant[len(relevant)-10:]
	}

	return relevant
}

// buildLegacySystemPrompt is the fallback string concatenation path.
func buildLegacySystemPrompt(agent *agentprogression.Agent, quest *domain.Quest) string {
	var sb strings.Builder

	if agent.Config.SystemPrompt != "" {
		sb.WriteString(agent.Config.SystemPrompt)
		sb.WriteString("\n\n")
	}

	if agent.Persona != nil && agent.Persona.SystemPrompt != "" {
		sb.WriteString(agent.Persona.SystemPrompt)
		sb.WriteString("\n\n")
	}

	sb.WriteString(fmt.Sprintf("You are working on a quest: %s\n", quest.Title))
	if quest.Description != "" {
		sb.WriteString(fmt.Sprintf("Description: %s\n", quest.Description))
	}

	if quest.Constraints.MaxDuration > 0 {
		sb.WriteString(fmt.Sprintf("Time limit: %v\n", quest.Constraints.MaxDuration))
	}
	if quest.Constraints.MaxTokens > 0 {
		sb.WriteString(fmt.Sprintf("Token budget: %d\n", quest.Constraints.MaxTokens))
	}

	if len(quest.RequiredSkills) > 0 {
		sb.WriteString("This quest requires skills in: ")
		for i, skill := range quest.RequiredSkills {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(string(skill))
		}
		sb.WriteByte('\n')
	}

	return sb.String()
}

// buildUserPrompt constructs the user prompt from quest input.
func buildUserPrompt(quest *domain.Quest) string {
	if quest.Input == nil {
		return quest.Description
	}
	if s, ok := quest.Input.(string); ok {
		return s
	}
	return fmt.Sprintf("Quest input:\n%v\n\nPlease complete the quest: %s", quest.Input, quest.Description)
}

// =============================================================================
// CAPABILITY RESOLUTION
// =============================================================================

// resolveCapability builds a capability key from the agent tier, quest primary
// skill, and decomposability class. Sequential quests try a dedicated capability
// key first ("quest-execution-sequential") so operators can route them to a
// higher-capability model — research shows accelerating returns from model
// intelligence on sequential reasoning tasks.
//
// Resolution order:
//  1. quest-execution-sequential (if DecomposabilityClass == sequential)
//  2. agent-work.{tier}.{skill}
//  3. agent-work.{tier}
//  4. agent-work
// waitForKnowledgeSources polls the semsource manifest until at least one source
// is active or the configured timeout expires. This soft gate ensures agents get
// graph knowledge context (source manifest + graph manifest) in their prompts.
// Uses knowledgeReady atomic to ensure at most one quest blocks the watcher;
// subsequent quests skip immediately once readiness is established (or timed out).
// No-op when semsource is not configured or timeout is 0.
func (c *Component) waitForKnowledgeSources(ctx context.Context, entityID string) {
	if c.config.KnowledgeReadyTimeout <= 0 {
		return
	}

	// Fast path: readiness already established (or timed out) by a prior quest.
	if c.knowledgeReady.Load() {
		return
	}

	// Preferred path: use GraphSourceRegistry to poll semsource status endpoints directly.
	if c.graphSources != nil && c.graphSources.HasSemsources() {
		timeout := time.Duration(c.config.KnowledgeReadyTimeout) * time.Second
		c.logger.Info("waiting for semsource readiness",
			"entity_id", entityID, "timeout_seconds", c.config.KnowledgeReadyTimeout)
		if err := c.graphSources.WaitForReady(ctx, timeout); err != nil {
			c.logger.Warn("semsource readiness wait failed — proceeding",
				"entity_id", entityID, "error", err)
		} else {
			c.logger.Info("all semsource sources ready", "entity_id", entityID)
		}
		c.knowledgeReady.Store(true)
		return
	}

	// Legacy path: gate on graph manifest (via graph-gateway GraphQL).
	// Only gate when semsource is configured (manifestClient != nil).
	if c.graphManifestClient == nil || c.manifestClient == nil {
		return
	}

	// Check current state before entering the poll loop.
	// If the graph-gateway is unreachable (Fetch returns nil), skip the gate
	// entirely — no point waiting 5 minutes for a service that isn't running.
	manifest := c.graphManifestClient.Fetch(ctx)
	if manifest == nil {
		c.logger.Debug("graph-gateway unreachable, skipping knowledge gate",
			"entity_id", entityID)
		c.knowledgeReady.Store(true)
		return
	}
	if len(manifest.PredicateFamilies) > 0 {
		c.knowledgeReady.Store(true)
		return
	}

	c.logger.Info("waiting for knowledge graph source content to be indexed",
		"entity_id", entityID,
		"timeout_seconds", c.config.KnowledgeReadyTimeout)

	timeout := time.Duration(c.config.KnowledgeReadyTimeout) * time.Second
	deadline := time.After(timeout)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.knowledgeReady.Store(true) // Don't block future quests
			return
		case <-c.stopChan:
			c.knowledgeReady.Store(true) // Don't block future quests
			return
		case <-deadline:
			c.logger.Warn("knowledge graph readiness timeout — proceeding without source content",
				"entity_id", entityID)
			c.knowledgeReady.Store(true) // Don't re-block on next quest
			return
		case <-ticker.C:
			c.graphManifestClient.Refresh(ctx)
			if c.graphManifestClient.HasSourceContent(ctx) {
				c.logger.Info("knowledge graph source content indexed, proceeding",
					"entity_id", entityID)
				c.knowledgeReady.Store(true)
				return
			}
		}
	}
}

func (c *Component) resolveCapability(agent *agentprogression.Agent, quest *domain.Quest) string {
	if c.registry == nil {
		return "agent-work"
	}

	// Sequential quests benefit from stronger models — try a dedicated capability first.
	if quest.DecomposabilityClass == domain.DecomposableSequential {
		if chain := c.registry.GetFallbackChain("quest-execution-sequential"); len(chain) > 0 {
			return "quest-execution-sequential"
		}
	}

	tier := agent.Tier.String()

	// Try tier + primary skill first.
	if len(quest.RequiredSkills) > 0 {
		key := fmt.Sprintf("agent-work.%s.%s", tier, string(quest.RequiredSkills[0]))
		if chain := c.registry.GetFallbackChain(key); len(chain) > 0 {
			return key
		}
	}

	// Fall back to tier-only key.
	key := fmt.Sprintf("agent-work.%s", tier)
	if chain := c.registry.GetFallbackChain(key); len(chain) > 0 {
		return key
	}

	return "agent-work"
}

// =============================================================================
// TOOL FILTERING
// =============================================================================

// toolsForQuest returns the agentic.ToolDefinition list the agent can use for
// this quest. Filters by agent trust tier, required skills, and the quest's
// AllowedTools whitelist. Uses only root semdragons types to avoid subpackage
// coupling.
func (c *Component) toolsForQuest(quest *domain.Quest, agent *agentprogression.Agent) []agentic.ToolDefinition {
	// Resolve tool registry: prefer questtools via ComponentRegistry, fall back to local.
	reg := c.toolRegistry
	if src := c.resolveToolRegistrySource(); src != nil {
		if qtReg := src.ToolRegistry(); qtReg != nil {
			reg = qtReg
		}
	}
	if reg == nil {
		return nil
	}

	// Include graph_search when graph-gateway is configured. The tool can query
	// game entities (always available) and knowledge sources (when semsource runs).
	// The entity context injection tells agents what's actually in the graph.
	hasKnowledgeSources := c.graphManifestClient != nil

	allowedCats := categoriesForQuest(quest, agent)

	all := reg.ListAll()
	result := make([]agentic.ToolDefinition, 0, len(all))

	for _, tool := range all {
		// Category filter: exclude tools whose category isn't needed for
		// this quest. Empty category = always included (backward compat).
		// For party leads, categoriesForQuest returns only core + party_lead,
		// which replaces the former isLeadTool name-based filter.
		if tool.Category != "" && !allowedCats[tool.Category] {
			continue
		}

		// Omit graph_search when no knowledge sources are indexed.
		if tool.Definition.Name == "graph_search" && !hasKnowledgeSources {
			continue
		}

		// Enforce trust tier gate.
		if agent.Tier < tool.MinTier {
			continue
		}

		// Enforce skill gate (agent must have at least one required skill).
		if len(tool.Skills) > 0 && !agentHasAnySkill(agent, tool.Skills) {
			continue
		}

		// Enforce quest's AllowedTools whitelist if set.
		if len(quest.AllowedTools) > 0 && !toolNameAllowed(quest.AllowedTools, tool.Definition.Name) {
			continue
		}

		result = append(result, tool.Definition)
	}
	return result
}

// categoriesForQuest returns the set of tool categories a quest needs based
// on quest characteristics and agent role. This reduces input tokens per API
// call by excluding irrelevant tool definitions.
func categoriesForQuest(quest *domain.Quest, agent *agentprogression.Agent) map[executor.ToolCategory]bool {
	// Party lead: only decomposition and core tools.
	if quest.PartyRequired && agent.Tier >= domain.TierMaster {
		return map[executor.ToolCategory]bool{
			executor.ToolCategoryCore:      true,
			executor.ToolCategoryPartyLead: true,
		}
	}

	// Default: everything except party_lead tools for non-lead agents.
	cats := map[executor.ToolCategory]bool{
		executor.ToolCategoryCore:      true,
		executor.ToolCategoryWrite:     true,
		executor.ToolCategoryBuild:     true,
		executor.ToolCategoryNetwork:   true,
		executor.ToolCategoryInspect:   true,
		executor.ToolCategoryKnowledge: true,
	}

	// Research-only quests (no CodeGen skill required) don't need
	// build/inspect tools — saves ~5 tool definitions.
	// Write tools are kept so scholars can save findings to markdown files.
	if !questRequiresCodeGen(quest) {
		delete(cats, executor.ToolCategoryBuild)
		delete(cats, executor.ToolCategoryInspect)
	}

	return cats
}

// questRequiresCodeGen returns true if the quest needs code generation tools.
// Quests with no required skills default to true (assume implementation).
func questRequiresCodeGen(quest *domain.Quest) bool {
	if len(quest.RequiredSkills) == 0 {
		return true // Default: assume implementation quest
	}
	for _, s := range quest.RequiredSkills {
		if s == domain.SkillCodeGen {
			return true
		}
	}
	return false
}

// toolChoiceForQuest determines the API-level tool choice constraint for a quest.
// Returns nil for auto (model decides freely). This is a pure function so it can
// be reused by any caller that assembles TaskMessages (e.g., future DM tool use).
func toolChoiceForQuest(quest *domain.Quest, agent *agentprogression.Agent, tools []agentic.ToolDefinition) *agentic.ToolChoice {
	if len(tools) == 0 {
		return nil
	}

	// Party lead must decompose — force the specific tool.
	isPartyLead := quest.PartyRequired && agent.Tier >= domain.TierMaster
	if isPartyLead {
		for _, t := range tools {
			if t.Name == "decompose_quest" {
				return &agentic.ToolChoice{Mode: "function", FunctionName: "decompose_quest"}
			}
		}
	}

	// Single tool available — force it.
	if len(tools) == 1 {
		return &agentic.ToolChoice{Mode: "function", FunctionName: tools[0].Name}
	}

	// Multiple tools — require tool use, let model choose which.
	return &agentic.ToolChoice{Mode: "required"}
}

func agentHasAnySkill(agent *agentprogression.Agent, skills []domain.SkillTag) bool {
	for _, skill := range skills {
		if agent.HasSkill(skill) {
			return true
		}
	}
	return false
}

func toolNameAllowed(allowed []string, name string) bool {
	for _, a := range allowed {
		if a == name {
			return true
		}
	}
	return false
}

// agentSkillNames returns a list of skill tag strings for the agent.
// extractToolNames returns the names of the given tool definitions.
func extractToolNames(tools []agentic.ToolDefinition) []string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return names
}

func agentSkillNames(agent *agentprogression.Agent) []string {
	names := make([]string, 0, len(agent.SkillProficiencies))
	for skill := range agent.SkillProficiencies {
		names = append(names, string(skill))
	}
	return names
}

// resolveQuestEndpoint returns the best-effort endpoint name for quest execution.
// Used for cost estimation when recording token usage from completed/failed loops.
func (c *Component) resolveQuestEndpoint() string {
	if c.registry == nil {
		return ""
	}
	return c.registry.Resolve("agent-work")
}

// =============================================================================
// TRIPLE HELPERS
// =============================================================================

// tripleString scans a slice of message.Triple for the given predicate and
// returns the object as a string. Returns empty string if not found or if the
// object is not a string value.
func tripleString(triples []message.Triple, predicate string) string {
	for _, t := range triples {
		if t.Predicate == predicate {
			if s, ok := t.Object.(string); ok {
				return s
			}
		}
	}
	return ""
}

// maxIterationsForDifficulty scales the configured MaxIterations based on
// quest difficulty. Simpler quests get fewer iterations to save tokens;
// harder quests get more headroom for multi-step work.
func maxIterationsForDifficulty(base int, difficulty domain.QuestDifficulty) int {
	// Scale factors relative to the base config value (treated as the
	// midpoint for DifficultyModerate):
	//   Trivial: 50%   Easy: 60%   Moderate: 100%
	//   Hard: 125%     Epic: 150%  Legendary: 200%
	var scale float64
	switch difficulty {
	case domain.DifficultyTrivial:
		scale = 0.50
	case domain.DifficultyEasy:
		scale = 0.60
	case domain.DifficultyModerate:
		scale = 1.00
	case domain.DifficultyHard:
		scale = 1.25
	case domain.DifficultyEpic:
		scale = 1.50
	case domain.DifficultyLegendary:
		scale = 2.00
	default:
		scale = 1.00
	}
	return max(10, int(float64(base)*scale))
}

// fragmentsToSources converts prompt fragment IDs to ContextSource structs
// for the ConstructedContext.Sources field.
func fragmentsToSources(fragmentIDs []string) []pkgtypes.ContextSource {
	sources := make([]pkgtypes.ContextSource, len(fragmentIDs))
	for i, id := range fragmentIDs {
		sources[i] = pkgtypes.ContextSource{Type: "prompt_fragment", ID: id}
	}
	return sources
}
