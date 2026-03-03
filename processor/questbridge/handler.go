package questbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/executor"
	"github.com/c360studio/semdragons/processor/promptmanager"
	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	pkgtypes "github.com/c360studio/semstreams/pkg/types"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nuid"
)

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

	if newStatus == string(domain.QuestInProgress) {
		oldStatus, _ := oldStatusI.(string)
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

	// Build system prompt using assembler or legacy path.
	systemPrompt := c.buildSystemPrompt(agent, quest)

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

	// Get tool definitions filtered for this quest and agent.
	tools := c.toolsForQuest(quest, agent)

	// Build the user prompt from quest input.
	userPrompt := buildUserPrompt(quest)

	// Construct the TaskMessage.
	// Sanitize questID for use in NATS subject tokens — entity IDs contain dots
	// which are subject delimiters. Replace dots with hyphens so the ID is a single
	// token, allowing agent.task.* and agent.complete.* filters to match correctly.
	subjectSafeQuestID := strings.ReplaceAll(questID, ".", "-")
	loopID := fmt.Sprintf("quest-%s-%s", subjectSafeQuestID, nuid.Next())
	taskMsg := agentic.TaskMessage{
		TaskID: questID,
		Role:   role,
		Model:  modelKey,
		Prompt: userPrompt,
		Context: &pkgtypes.ConstructedContext{
			Content:       systemPrompt,
			ConstructedAt: time.Now(),
		},
		Tools: tools,
		Metadata: map[string]any{
			"agent_id":    agentID,
			"quest_id":    questID,
			"trust_tier":  int(agent.Tier),
			"skills":      agentSkillNames(agent),
			"sandbox_dir": c.config.SandboxDir,
			"board":       c.config.Board,
		},
	}

	data, err := json.Marshal(&taskMsg)
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
		MaxTurns:   c.config.MaxIterations,
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
		FilterSubjects: []string{"agent.complete.>", "agent.failed.>"},
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
	var event agentic.LoopCompletedEvent
	if err := json.Unmarshal(data, &event); err != nil {
		c.logger.Error("failed to unmarshal LoopCompletedEvent", "error", err)
		c.errorsCount.Add(1)
		return
	}

	questID := domain.QuestID(event.TaskID)
	mapping := c.findMapping(ctx, string(questID))
	if mapping == nil {
		c.logger.Debug("no mapping found for completed loop",
			"task_id", event.TaskID, "loop_id", event.LoopID)
		return
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

	c.cleanupMapping(ctx, string(questID))
	c.loopsCompleted.Add(1)
	c.lastActivity.Store(time.Now())

	c.logger.Info("quest execution completed via agentic loop",
		"quest_id", questID,
		"loop_id", event.LoopID,
		"iterations", event.Iterations)
}

// handleLoopFailed emits an executor failure event for the failed loop.
func (c *Component) handleLoopFailed(ctx context.Context, data []byte) {
	var event agentic.LoopFailedEvent
	if err := json.Unmarshal(data, &event); err != nil {
		c.logger.Error("failed to unmarshal LoopFailedEvent", "error", err)
		c.errorsCount.Add(1)
		return
	}

	questID := domain.QuestID(event.TaskID)
	mapping := c.findMapping(ctx, string(questID))
	if mapping == nil {
		c.logger.Debug("no mapping found for failed loop",
			"task_id", event.TaskID, "loop_id", event.LoopID)
		return
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

	c.cleanupMapping(ctx, string(questID))
	c.loopsFailed.Add(1)
	c.lastActivity.Store(time.Now())

	c.logger.Info("quest execution failed via agentic loop",
		"quest_id", questID,
		"loop_id", event.LoopID,
		"error", event.Error)
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
func (c *Component) findMapping(ctx context.Context, questID string) *QuestLoopMapping {
	if v, ok := c.activeLoops.Load(questID); ok {
		return v.(*QuestLoopMapping)
	}

	entry, err := c.questLoopsBucket.Get(ctx, questID)
	if err != nil {
		return nil
	}

	var mapping QuestLoopMapping
	if err := json.Unmarshal(entry.Value(), &mapping); err != nil {
		return nil
	}
	return &mapping
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

// buildSystemPrompt builds the system prompt using the assembler when available,
// falling back to the legacy string concatenation path.
func (c *Component) buildSystemPrompt(agent *agentprogression.Agent, quest *domain.Quest) string {
	if c.promptAssembler != nil {
		return c.buildAssembledSystemPrompt(agent, quest)
	}
	return buildLegacySystemPrompt(agent, quest)
}

func (c *Component) buildAssembledSystemPrompt(agent *agentprogression.Agent, quest *domain.Quest) string {
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

	assemblyCtx := promptmanager.AssemblyContext{
		AgentID:          agent.ID,
		Tier:             agent.Tier,
		Level:            agent.Level,
		Skills:           agent.SkillProficiencies,
		Guilds:           agent.Guilds,
		SystemPrompt:     agent.Config.SystemPrompt,
		PersonaPrompt:    personaPrompt,
		QuestTitle:       quest.Title,
		QuestDescription: quest.Description,
		QuestInput:       quest.Input,
		RequiredSkills:   quest.RequiredSkills,
		MaxDuration:      maxDuration,
		MaxTokens:        quest.Constraints.MaxTokens,
		Provider:         provider,
	}

	result := c.promptAssembler.AssembleSystemPrompt(assemblyCtx)
	return result.SystemMessage
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

// resolveCapability builds a capability key from the agent tier and quest primary skill.
// Falls back through tier-only and then bare "agent-work" keys.
func (c *Component) resolveCapability(agent *agentprogression.Agent, quest *domain.Quest) string {
	if c.registry == nil {
		return "agent-work"
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
	if c.toolRegistry == nil {
		return nil
	}

	all := c.toolRegistry.ListAll()
	result := make([]agentic.ToolDefinition, 0, len(all))

	for _, tool := range all {
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
func agentSkillNames(agent *agentprogression.Agent) []string {
	names := make([]string, 0, len(agent.SkillProficiencies))
	for skill := range agent.SkillProficiencies {
		names = append(names, string(skill))
	}
	return names
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
