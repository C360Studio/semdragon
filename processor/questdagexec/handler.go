package questdagexec

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/partycoord"
	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	pkgtypes "github.com/c360studio/semstreams/pkg/types"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nuid"
)

// =============================================================================
// PEER REVIEW CREATION
// =============================================================================

// createReviewEntity persists a PeerReview entity recording a failed sub-quest.
// Called when a node exhausts its retry budget to create a low-rating review
// record that feeds back into the member agent's prompt on future quests.
//
// Ratings are synthetic (all 2s) because the actual review tool result is not
// available in the KV entity state at this point. This can be refined later
// when the review tool verdict is propagated through loop completion metadata.
func (c *Component) createReviewEntity(ctx context.Context, dagState *DAGExecutionState, nodeID string) {
	if c.graph == nil || c.boardConfig == nil {
		return
	}

	subQuestID := dagState.NodeQuestIDs[nodeID]
	memberID := domain.AgentID(dagState.NodeAssignees[nodeID])
	leaderID := domain.AgentID(c.findLeadAgentID(dagState))

	// Skip when we cannot identify the reviewer or reviewee — the review
	// would carry no actionable information without both agent IDs.
	if memberID == "" || leaderID == "" {
		c.logger.Debug("skipping peer review creation — missing leader or member ID",
			"execution_id", dagState.ExecutionID,
			"node_id", nodeID,
			"leader_id", leaderID,
			"member_id", memberID)
		return
	}

	reviewInstance := "pr-" + nuid.Next()
	reviewID := domain.PeerReviewID(c.boardConfig.PeerReviewEntityID(reviewInstance))

	now := time.Now()
	// Synthetic low ratings (2/5 each) — sub-quest failed after exhausting retries.
	// All three leader-to-member questions receive the same baseline score.
	ratings := domain.ReviewRatings{Q1: 2, Q2: 2, Q3: 2}
	explanation := fmt.Sprintf("Sub-quest %q failed after exhausting all retries for DAG node %s.", subQuestID, nodeID)

	var partyIDPtr *domain.PartyID
	if dagState.PartyID != "" {
		pid := domain.PartyID(dagState.PartyID)
		partyIDPtr = &pid
	}

	pr := &domain.PeerReview{
		ID:      reviewID,
		Status:  domain.PeerReviewCompleted,
		QuestID: domain.QuestID(subQuestID),
		PartyID: partyIDPtr,
		LeaderID: leaderID,
		MemberID: memberID,
		IsSoloTask: false,
		LeaderReview: &domain.ReviewSubmission{
			ReviewerID:  leaderID,
			RevieweeID:  memberID,
			Direction:   domain.ReviewDirectionLeaderToMember,
			Ratings:     ratings,
			Explanation: explanation,
			SubmittedAt: now,
		},
		LeaderAvgRating: ratings.Average(),
		CreatedAt:       now,
		CompletedAt:     &now,
	}

	if err := c.graph.EmitEntity(ctx, pr, domain.PredicateReviewCompleted); err != nil {
		c.logger.Error("failed to emit peer review entity for failed node",
			"execution_id", dagState.ExecutionID,
			"node_id", nodeID,
			"review_id", reviewID,
			"error", err)
		c.errorsCount.Add(1)
		return
	}

	c.logger.Info("created peer review for failed DAG node",
		"execution_id", dagState.ExecutionID,
		"node_id", nodeID,
		"review_id", reviewID,
		"leader_id", leaderID,
		"member_id", memberID,
		"avg_rating", pr.LeaderAvgRating)
}

// =============================================================================
// KV TWOFER BOOTSTRAP PROTOCOL
// =============================================================================
// Phase 1 (bootstrapping=true): replay existing quest KV state — hydrate
// questCache and rebuild dagBySubQuest index. The nil sentinel marks end of
// replay.
// Phase 2 (bootstrapping=false): detect live sub-quest status transitions and
// drive the DAG state machine.
//
// This prevents re-firing review dispatches for sub-quests already in
// pending_review or completed state that existed before this instance started.
// =============================================================================

// watchLoop implements the KV twofer bootstrap protocol for sub-quest watching.
func (c *Component) watchLoop(ctx context.Context) {
	watcher, err := c.graph.WatchEntityType(ctx, domain.EntityTypeQuest)
	if err != nil {
		c.logger.Error("failed to start quest watcher", "error", err)
		c.errorsCount.Add(1)
		return
	}
	defer watcher.Stop()

	// Bootstrap DAG index from QUEST_DAGS before processing live events.
	if err := c.bootstrapDAGIndex(ctx); err != nil {
		c.logger.Error("DAG index bootstrap failed — watchLoop exiting", "error", err)
		c.errorsCount.Add(1)
		return
	}

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
				continue
			}

			if bootstrapping {
				// During bootstrap: hydrate cache only; never trigger actions.
				if status := c.extractQuestStatus(entry); status != "" {
					c.questCache.Store(entry.Key(), status)
				}
			} else {
				// After bootstrap: detect transitions and drive DAG.
				c.handleLiveUpdate(ctx, entry)
			}
		}
	}
}

// bootstrapDAGIndex loads all existing DAGExecutionState entries from the
// QUEST_DAGS bucket and builds the dagBySubQuest and dagCache indexes.
// Called once at the start of watchLoop before processing live events.
func (c *Component) bootstrapDAGIndex(ctx context.Context) error {
	if c.questDagsBucket == nil {
		return nil
	}

	// List all entries in the bucket.
	watcher, err := c.questDagsBucket.WatchAll(ctx)
	if err != nil {
		return fmt.Errorf("watch QUEST_DAGS for bootstrap: %w", err)
	}
	defer watcher.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.stopChan:
			return nil
		case entry, ok := <-watcher.Updates():
			if !ok || entry == nil {
				// Nil sentinel or channel closed — bootstrap complete.
				return nil
			}
			if entry.Operation() == jetstream.KeyValueDelete || entry.Operation() == jetstream.KeyValuePurge {
				continue
			}

			var dagState DAGExecutionState
			if unmarshalErr := json.Unmarshal(entry.Value(), &dagState); unmarshalErr != nil {
				c.logger.Warn("failed to unmarshal DAGExecutionState during bootstrap",
					"key", entry.Key(), "error", unmarshalErr)
				continue
			}

			dagState.Revision = entry.Revision()
			c.indexDAGState(&dagState)
			c.logger.Debug("bootstrapped DAG execution state",
				"execution_id", dagState.ExecutionID,
				"parent_quest_id", dagState.ParentQuestID,
				"nodes", len(dagState.DAG.Nodes))
		}
	}
}

// indexDAGState registers a DAGExecutionState into the in-memory indexes.
// It stores the state in dagCache and maps every sub-quest entity key to the
// DAG state in dagBySubQuest, enabling O(1) lookup from sub-quest KV events.
func (c *Component) indexDAGState(dagState *DAGExecutionState) {
	// Store a pointer copy so mutations in handleSubQuestTransition are
	// reflected in dagBySubQuest lookups.
	ptr := dagState
	c.dagCache.Store(dagState.ExecutionID, ptr)

	for _, subQuestID := range dagState.NodeQuestIDs {
		if subQuestID == "" {
			continue
		}
		// Sub-quest IDs in NodeQuestIDs may be full entity IDs or instance IDs.
		// Build the entity key the same way the graph client does for quests.
		entityKey := c.subQuestEntityKey(subQuestID)
		c.dagBySubQuest.Store(entityKey, ptr)
	}
}

// subQuestEntityKey converts a sub-quest ID (which may already be a full entity
// ID like "local.dev.game.board1.quest.abc") into the exact KV key used by the
// graph client. The graph client uses the full entity ID as the KV key.
func (c *Component) subQuestEntityKey(questID string) string {
	// If it already looks like a full entity ID (contains dots beyond the
	// prefix depth), return it as-is. Otherwise build from board config.
	if strings.Count(questID, ".") >= 5 {
		return questID
	}
	instance := domain.ExtractInstance(questID)
	return c.boardConfig.QuestEntityID(instance)
}

// extractQuestStatus returns the quest.status.state triple value from a KV entry.
func (c *Component) extractQuestStatus(entry jetstream.KeyValueEntry) string {
	entityState, err := semdragons.DecodeEntityState(entry)
	if err != nil || entityState == nil {
		return ""
	}
	return tripleString(entityState.Triples, "quest.status.state")
}

// handleLiveUpdate processes a live quest entity KV change and detects
// sub-quest status transitions relevant to any active DAG.
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

	// Swap the cache entry; returns previous value and whether it existed.
	oldStatusI, _ := c.questCache.Swap(entry.Key(), newStatus)
	oldStatus, _ := oldStatusI.(string)

	// Skip if status did not change — idempotent write from another processor.
	if oldStatus == newStatus {
		return
	}

	// Only react to sub-quests that are part of an active DAG.
	dagState := c.findDAGForSubQuest(entry.Key())
	if dagState == nil {
		return
	}

	questID := tripleString(entityState.Triples, "quest.identity.id")
	if questID == "" {
		questID = entityState.ID
	}

	nodeID := c.findNodeForQuest(dagState, questID)
	if nodeID == "" {
		// Try with the full entity key as well.
		nodeID = c.findNodeForQuest(dagState, entry.Key())
		if nodeID == "" {
			return
		}
	}

	c.handleSubQuestTransition(ctx, dagState, nodeID, domain.QuestStatus(oldStatus), domain.QuestStatus(newStatus), entityState)
}

// =============================================================================
// STATE MACHINE
// =============================================================================

// handleSubQuestTransition reacts to a status change on a sub-quest that
// belongs to an active DAG. It updates NodeStates, dispatches work, and
// triggers rollup or escalation as appropriate.
func (c *Component) handleSubQuestTransition(
	ctx context.Context,
	dagState *DAGExecutionState,
	nodeID string,
	oldStatus, newStatus domain.QuestStatus,
	entity *graph.EntityState,
) {
	switch newStatus {
	case domain.QuestInProgress:
		// Quest was claimed and started — mark node in_progress.
		dagState.NodeStates[nodeID] = NodeInProgress
		c.logger.Debug("DAG node transitioned to in_progress",
			"execution_id", dagState.ExecutionID, "node_id", nodeID)

	case domain.QuestInReview:
		// Member submitted output — dispatch lead review task.
		dagState.NodeStates[nodeID] = NodePendingReview
		c.dispatchLeadReview(ctx, dagState, nodeID, entity)

	case domain.QuestCompleted:
		c.handleNodeCompleted(ctx, dagState, nodeID)

	case domain.QuestFailed:
		c.handleNodeFailed(ctx, dagState, nodeID)

	default:
		// Other transitions (posted, claimed, abandoned, escalated) do not
		// require DAG state changes — log and skip.
		c.logger.Debug("ignoring sub-quest status transition",
			"execution_id", dagState.ExecutionID, "node_id", nodeID,
			"old_status", oldStatus, "new_status", newStatus)
		return
	}

	c.lastActivity.Store(time.Now())

	// Persist mutated DAG state to QUEST_DAGS bucket.
	if err := c.persistDAGState(ctx, dagState); err != nil {
		c.logger.Error("failed to persist DAG state after transition",
			"execution_id", dagState.ExecutionID, "node_id", nodeID, "error", err)
		c.errorsCount.Add(1)
	}
}

// handleNodeCompleted marks the node completed, appends it to CompletedNodes,
// then either triggers rollup (all nodes done) or assigns the next batch of
// ready nodes.
func (c *Component) handleNodeCompleted(ctx context.Context, dagState *DAGExecutionState, nodeID string) {
	dagState.NodeStates[nodeID] = NodeCompleted
	dagState.CompletedNodes = append(dagState.CompletedNodes, nodeID)
	c.nodesCompleted.Add(1)

	c.logger.Info("DAG node completed",
		"execution_id", dagState.ExecutionID,
		"node_id", nodeID,
		"completed", len(dagState.CompletedNodes),
		"total", len(dagState.DAG.Nodes))

	if c.isDAGComplete(dagState) {
		c.triggerRollup(ctx, dagState)
		return
	}

	// Promote pending nodes whose dependencies are now met to NodeReady.
	c.promoteReadyNodes(dagState)

	// Assign the newly ready nodes to available party members.
	c.assignReadyNodes(ctx, dagState)
}

// handleNodeFailed decrements the retry budget for the node and either resets
// it to NodeAssigned for another attempt or marks it NodeFailed and escalates
// the parent quest.
func (c *Component) handleNodeFailed(ctx context.Context, dagState *DAGExecutionState, nodeID string) {
	retries, ok := dagState.NodeRetries[nodeID]
	if !ok {
		retries = c.config.MaxRetriesPerNode
	}

	c.logger.Info("DAG node failed",
		"execution_id", dagState.ExecutionID,
		"node_id", nodeID,
		"retries_remaining", retries)

	if retries > 0 {
		dagState.NodeRetries[nodeID] = retries - 1
		// Reset to NodeAssigned so the lead can re-dispatch to the same member
		// or the boid engine can reassign.
		dagState.NodeStates[nodeID] = NodeAssigned

		// Re-claim and re-assign the sub-quest for the same member.
		c.retryNodeAssignment(ctx, dagState, nodeID)
		return
	}

	// No retries left — node fails permanently.
	dagState.NodeStates[nodeID] = NodeFailed
	dagState.FailedNodes = append(dagState.FailedNodes, nodeID)
	c.nodesFailed.Add(1)

	c.logger.Warn("DAG node exhausted retries — escalating parent",
		"execution_id", dagState.ExecutionID,
		"node_id", nodeID,
		"parent_quest_id", dagState.ParentQuestID)

	// Create a peer review entity recording the failure so the member agent's
	// prompt on future quests can surface corrective guidance.
	c.createReviewEntity(ctx, dagState, nodeID)

	reason := fmt.Sprintf("DAG node %s failed after exhausting retries", nodeID)
	c.escalateParent(ctx, dagState, reason)
}

// =============================================================================
// REVIEW DISPATCH
// =============================================================================

// dispatchLeadReview publishes a TaskMessage to the AGENT stream asking the
// lead to review the party member's sub-quest output. The review tool's verdict
// (accept/reject) determines whether the node advances to NodeCompleted or is
// reset for retry.
func (c *Component) dispatchLeadReview(ctx context.Context, dagState *DAGExecutionState, nodeID string, entity *graph.EntityState) {
	subQuestID := dagState.NodeQuestIDs[nodeID]
	leadAgentID := c.findLeadAgentID(dagState)

	// Extract sub-quest details for the review prompt.
	nodeObjective := c.findNodeObjective(dagState, nodeID)
	nodeAcceptance := c.findNodeAcceptance(dagState, nodeID)

	// Get the sub-quest output to review.
	subQuestOutput := c.extractQuestOutput(entity)

	systemPrompt := buildLeadReviewSystemPrompt(nodeObjective, nodeAcceptance, subQuestOutput)

	// Build the user prompt with the review context.
	userPrompt := fmt.Sprintf(
		"Please review the party member's work for sub-quest %q.\n\nObjective: %s\n\nUse the review_sub_quest tool to submit your verdict.",
		subQuestID, nodeObjective,
	)

	// Sanitize the sub-quest ID for use as a NATS subject token.
	subjectSafeID := strings.ReplaceAll(subQuestID, ".", "-")
	loopID := fmt.Sprintf("review-%s-%s", subjectSafeID, nuid.Next())

	reviewTools := NewReviewExecutor().ListTools()

	taskMsg := agentic.TaskMessage{
		TaskID: subQuestID,
		LoopID: loopID,
		Role:   agentic.RoleGeneral,
		Model:  "agent-work",
		Prompt: userPrompt,
		Context: &pkgtypes.ConstructedContext{
			Content:       systemPrompt,
			ConstructedAt: time.Now(),
		},
		Tools: reviewTools,
		Metadata: map[string]any{
			"parent_quest_id": dagState.ParentQuestID,
			"sub_quest_id":    subQuestID,
			"node_id":         nodeID,
			"execution_id":    dagState.ExecutionID,
			"party_id":        dagState.PartyID,
			"lead_agent_id":   leadAgentID,
			"review_dispatch": true,
		},
	}

	// Wrap in BaseMessage envelope — required by agentic-loop consumer.
	baseMsg := message.NewBaseMessage(taskMsg.Schema(), &taskMsg, ComponentName)
	data, err := json.Marshal(baseMsg)
	if err != nil {
		c.logger.Error("failed to marshal review TaskMessage",
			"sub_quest_id", subQuestID, "node_id", nodeID, "error", err)
		c.errorsCount.Add(1)
		return
	}

	subject := fmt.Sprintf("agent.task.%s", subjectSafeID)
	if err := c.deps.NATSClient.PublishToStream(ctx, subject, data); err != nil {
		c.logger.Error("failed to publish review TaskMessage",
			"sub_quest_id", subQuestID, "subject", subject, "error", err)
		c.errorsCount.Add(1)
		return
	}

	c.logger.Info("dispatched lead review task",
		"execution_id", dagState.ExecutionID,
		"node_id", nodeID,
		"sub_quest_id", subQuestID,
		"loop_id", loopID,
		"lead_agent_id", leadAgentID)
}

// buildLeadReviewSystemPrompt constructs the system prompt for the lead's
// review agentic loop. It includes the objective, acceptance criteria, and
// the member's output.
func buildLeadReviewSystemPrompt(objective string, acceptance []string, output string) string {
	var sb strings.Builder
	sb.WriteString("You are the party lead reviewing a sub-quest. Review the output against acceptance criteria.\n\n")

	sb.WriteString("Sub-quest objective:\n")
	sb.WriteString(objective)
	sb.WriteString("\n\n")

	if len(acceptance) > 0 {
		sb.WriteString("Acceptance criteria:\n")
		for i, ac := range acceptance {
			fmt.Fprintf(&sb, "%d. %s\n", i+1, ac)
		}
		sb.WriteString("\n")
	}

	if output != "" {
		sb.WriteString("Party member's output:\n")
		sb.WriteString(output)
		sb.WriteString("\n\n")
	} else {
		sb.WriteString("The party member did not provide output — this constitutes a failure.\n\n")
	}

	sb.WriteString("Use the review_sub_quest tool to submit your verdict (accept or reject).")
	return sb.String()
}

// =============================================================================
// ROLLUP
// =============================================================================

// triggerRollup collects outputs from all completed sub-quests, submits the
// aggregated result to the parent quest via questboard, and disbands the party.
// Called when isDAGComplete returns true.
func (c *Component) triggerRollup(ctx context.Context, dagState *DAGExecutionState) {
	c.rollupsTriggered.Add(1)

	c.logger.Info("triggering DAG rollup",
		"execution_id", dagState.ExecutionID,
		"parent_quest_id", dagState.ParentQuestID,
		"completed_nodes", len(dagState.CompletedNodes))

	// Collect sub-quest outputs in completion order.
	// When the graph client is unavailable (e.g. in unit tests), outputs will
	// be empty but rollup still proceeds — an empty result map is valid.
	outputs := make(map[string]any, len(dagState.CompletedNodes))
	if c.graph != nil {
		for _, nodeID := range dagState.CompletedNodes {
			subQuestID, ok := dagState.NodeQuestIDs[nodeID]
			if !ok || subQuestID == "" {
				continue
			}

			questEntity, err := c.graph.GetQuest(ctx, domain.QuestID(subQuestID))
			if err != nil {
				c.logger.Warn("failed to load sub-quest for rollup",
					"sub_quest_id", subQuestID, "node_id", nodeID, "error", err)
				continue
			}

			quest := domain.QuestFromEntityState(questEntity)
			if quest != nil && quest.Output != nil {
				outputs[nodeID] = quest.Output
			}
		}
	}

	// Submit the collected outputs as the parent quest result.
	if c.questBoardRef != nil {
		parentID := domain.QuestID(dagState.ParentQuestID)
		if err := c.questBoardRef.SubmitResult(ctx, parentID, outputs); err != nil {
			c.logger.Error("failed to submit rollup result to questboard",
				"parent_quest_id", dagState.ParentQuestID, "error", err)
			c.errorsCount.Add(1)
			// Continue to disband the party even if rollup submission fails.
		}
	}

	// Disband the party — work is done.
	if c.partyCoord != nil && dagState.PartyID != "" {
		partyID := domain.PartyID(dagState.PartyID)
		if err := c.partyCoord.DisbandParty(ctx, partyID, "DAG completed"); err != nil {
			c.logger.Warn("failed to disband party after rollup",
				"party_id", dagState.PartyID, "error", err)
		}
	}

	c.logger.Info("DAG rollup complete",
		"execution_id", dagState.ExecutionID,
		"parent_quest_id", dagState.ParentQuestID,
		"outputs_collected", len(outputs))
}

// =============================================================================
// READY-NODE ASSIGNMENT
// =============================================================================

// promoteReadyNodes advances nodes in NodePending state whose dependencies
// are all NodeCompleted to NodeReady. Must be called after any NodeCompleted
// transition.
func (c *Component) promoteReadyNodes(dagState *DAGExecutionState) {
	for _, nodeID := range DAGReadyNodes(dagState.DAG, dagState.NodeStates) {
		dagState.NodeStates[nodeID] = NodeReady
	}
}

// assignReadyNodes calls AssignReadyNodes using the partyCoord and questBoard
// references. It wraps the questboard and partycoord interfaces into the
// narrow interfaces expected by AssignReadyNodes.
func (c *Component) assignReadyNodes(ctx context.Context, dagState *DAGExecutionState) {
	if c.partyCoord == nil {
		c.logger.Debug("partyCoord not set — skipping ready node assignment")
		return
	}
	if c.questBoardRef == nil {
		c.logger.Debug("questBoardRef not set — skipping ready node assignment")
		return
	}

	deps := AssignmentDeps{
		Members:     &partyCoordMemberLister{ref: c.partyCoord},
		Tasks:       &partyCoordTaskAssigner{ref: c.partyCoord},
		QuestClaims: &questBoardClaimer{ref: c.questBoardRef},
	}

	if err := AssignReadyNodes(ctx, dagState, deps); err != nil {
		c.logger.Warn("failed to assign ready nodes",
			"execution_id", dagState.ExecutionID, "error", err)
		c.errorsCount.Add(1)
	}
}

// retryNodeAssignment re-dispatches a failed node to the same assignee.
// If no assignee is recorded, it falls back to assigning the best available
// party member.
func (c *Component) retryNodeAssignment(ctx context.Context, dagState *DAGExecutionState, nodeID string) {
	// Reset node to pending so assignReadyNodes can pick it up.
	dagState.NodeStates[nodeID] = NodePending

	// Remove the existing assignee so the node can be reassigned.
	delete(dagState.NodeAssignees, nodeID)

	c.promoteReadyNodes(dagState)
	c.assignReadyNodes(ctx, dagState)
}

// =============================================================================
// ESCALATION
// =============================================================================

// escalateParent transitions the parent quest to escalated state via the
// questboard. Called when a node fails permanently.
func (c *Component) escalateParent(ctx context.Context, dagState *DAGExecutionState, reason string) {
	if c.questBoardRef == nil {
		c.logger.Warn("questBoardRef not set — cannot escalate parent quest",
			"parent_quest_id", dagState.ParentQuestID, "reason", reason)
		return
	}

	parentID := domain.QuestID(dagState.ParentQuestID)
	if err := c.questBoardRef.EscalateQuest(ctx, parentID, reason); err != nil {
		c.logger.Error("failed to escalate parent quest",
			"parent_quest_id", dagState.ParentQuestID, "error", err)
		c.errorsCount.Add(1)
	}
}

// =============================================================================
// LOOKUP HELPERS
// =============================================================================

// findDAGForSubQuest returns the DAGExecutionState that contains the given
// sub-quest entity key, or nil if the quest is not part of any active DAG.
func (c *Component) findDAGForSubQuest(entityKey string) *DAGExecutionState {
	v, ok := c.dagBySubQuest.Load(entityKey)
	if !ok {
		return nil
	}
	return v.(*DAGExecutionState)
}

// findNodeForQuest returns the node ID in the DAG whose NodeQuestIDs value
// matches the given questID. Returns an empty string if not found.
func (c *Component) findNodeForQuest(dagState *DAGExecutionState, questID string) string {
	// Normalise: try both full entity ID and instance-only forms.
	questInstance := domain.ExtractInstance(questID)

	for nodeID, nodeQuestID := range dagState.NodeQuestIDs {
		nodeInstance := domain.ExtractInstance(nodeQuestID)
		if nodeQuestID == questID || nodeInstance == questID || nodeInstance == questInstance || nodeQuestID == questInstance {
			return nodeID
		}
	}
	return ""
}

// isDAGComplete returns true when every node has reached NodeCompleted.
// Nodes in NodeFailed, NodeRejected, or other non-terminal states mean the
// DAG has not yet reached a final state (failure escalation handles the error
// path separately).
func (c *Component) isDAGComplete(dagState *DAGExecutionState) bool {
	for _, nodeID := range dagState.DAG.Nodes {
		if dagState.NodeStates[nodeID.ID] != NodeCompleted {
			return false
		}
	}
	return len(dagState.DAG.Nodes) > 0
}

// findLeadAgentID returns the lead agent ID from the party, or an empty
// string if the party is unavailable.
func (c *Component) findLeadAgentID(dagState *DAGExecutionState) string {
	if c.partyCoord == nil || dagState.PartyID == "" {
		return ""
	}
	party, ok := c.partyCoord.GetParty(domain.PartyID(dagState.PartyID))
	if !ok || party == nil {
		return ""
	}
	return string(party.Lead)
}

// findNodeObjective returns the Objective string for a node by ID.
func (c *Component) findNodeObjective(dagState *DAGExecutionState, nodeID string) string {
	for _, node := range dagState.DAG.Nodes {
		if node.ID == nodeID {
			return node.Objective
		}
	}
	return ""
}

// findNodeAcceptance returns the Acceptance criteria for a node by ID.
func (c *Component) findNodeAcceptance(dagState *DAGExecutionState, nodeID string) []string {
	for _, node := range dagState.DAG.Nodes {
		if node.ID == nodeID {
			return node.Acceptance
		}
	}
	return nil
}

// extractQuestOutput returns the string representation of a quest's output
// triple from an EntityState. Returns an empty string when not present.
func (c *Component) extractQuestOutput(entity *graph.EntityState) string {
	if entity == nil {
		return ""
	}
	v := tripleValue(entity.Triples, "quest.data.output")
	if v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(data)
	}
}

// =============================================================================
// DAG STATE PERSISTENCE
// =============================================================================

// persistDAGState marshals the DAGExecutionState and writes it to QUEST_DAGS
// keyed by ExecutionID. Must be called after every mutation.
func (c *Component) persistDAGState(ctx context.Context, state *DAGExecutionState) error {
	if c.questDagsBucket == nil {
		return fmt.Errorf("QUEST_DAGS bucket not initialized")
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal DAG execution state: %w", err)
	}

	// Use CAS Update when we have a known revision to prevent concurrent
	// overwrites (e.g. during rolling deploys). Fall back to Put for new
	// entries that haven't been read from KV (revision 0).
	if state.Revision > 0 {
		rev, updateErr := c.questDagsBucket.Update(ctx, state.ExecutionID, data, state.Revision)
		if updateErr != nil {
			return fmt.Errorf("CAS update DAG execution state (rev %d): %w", state.Revision, updateErr)
		}
		state.Revision = rev
	} else {
		rev, putErr := c.questDagsBucket.Put(ctx, state.ExecutionID, data)
		if putErr != nil {
			return fmt.Errorf("write DAG execution state to KV: %w", putErr)
		}
		state.Revision = rev
	}
	return nil
}

// =============================================================================
// INTERFACE ADAPTERS
// =============================================================================
// These thin adapters bridge the narrow PartyCoordRef / QuestBoardRef
// interfaces to the AssignmentDeps sub-interfaces used by AssignReadyNodes.
// =============================================================================

// partyCoordMemberLister wraps PartyCoordRef to satisfy PartyMemberLister.
type partyCoordMemberLister struct{ ref PartyCoordRef }

func (a *partyCoordMemberLister) GetParty(partyID domain.PartyID) (*partycoord.Party, bool) {
	return a.ref.GetParty(partyID)
}

// partyCoordTaskAssigner wraps PartyCoordRef to satisfy TaskAssigner.
type partyCoordTaskAssigner struct{ ref PartyCoordRef }

func (a *partyCoordTaskAssigner) AssignTask(ctx context.Context, partyID domain.PartyID, subQuestID domain.QuestID, assignedTo domain.AgentID, rationale string) error {
	return a.ref.AssignTask(ctx, partyID, subQuestID, assignedTo, rationale)
}

// questBoardClaimer wraps QuestBoardRef to satisfy QuestClaimerForParty.
type questBoardClaimer struct{ ref QuestBoardRef }

func (a *questBoardClaimer) ClaimQuestForParty(ctx context.Context, questID domain.QuestID, partyID domain.PartyID) error {
	return a.ref.ClaimQuestForParty(ctx, questID, partyID)
}

// =============================================================================
// TRIPLE HELPERS
// =============================================================================

// tripleString scans triples for a predicate and returns the object as a string.
func tripleString(triples []message.Triple, predicate string) string {
	v := tripleValue(triples, predicate)
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

// tripleValue scans triples for a predicate and returns its object value.
func tripleValue(triples []message.Triple, predicate string) any {
	for _, t := range triples {
		if t.Predicate == predicate {
			return t.Object
		}
	}
	return nil
}
