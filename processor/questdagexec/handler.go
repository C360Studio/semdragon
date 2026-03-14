package questdagexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/partycoord"
	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	pkgtypes "github.com/c360studio/semstreams/pkg/types"
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

// createAcceptReviewEntity creates a peer review entity recording a successful
// lead review. Called when the lead accepts a sub-quest via review_sub_quest.
func (c *Component) createAcceptReviewEntity(ctx context.Context, dagState *DAGExecutionState, nodeID string) {
	if c.graph == nil || c.boardConfig == nil {
		return
	}

	subQuestID := dagState.NodeQuestIDs[nodeID]
	memberID := domain.AgentID(dagState.NodeAssignees[nodeID])
	leaderID := domain.AgentID(c.findLeadAgentID(dagState))

	if memberID == "" || leaderID == "" {
		return
	}

	reviewInstance := "pr-" + nuid.Next()
	reviewID := domain.PeerReviewID(c.boardConfig.PeerReviewEntityID(reviewInstance))

	now := time.Now()
	// Accept = high ratings (5/5 each).
	ratings := domain.ReviewRatings{Q1: 5, Q2: 5, Q3: 5}

	var partyIDPtr *domain.PartyID
	if dagState.PartyID != "" {
		pid := domain.PartyID(dagState.PartyID)
		partyIDPtr = &pid
	}

	pr := &domain.PeerReview{
		ID:       reviewID,
		Status:   domain.PeerReviewCompleted,
		QuestID:  domain.QuestID(subQuestID),
		PartyID:  partyIDPtr,
		LeaderID: leaderID,
		MemberID: memberID,
		IsSoloTask: false,
		LeaderReview: &domain.ReviewSubmission{
			ReviewerID:  leaderID,
			RevieweeID:  memberID,
			Direction:   domain.ReviewDirectionLeaderToMember,
			Ratings:     ratings,
			Explanation: "Sub-quest accepted by lead review.",
			SubmittedAt: now,
		},
		LeaderAvgRating: ratings.Average(),
		CreatedAt:       now,
		CompletedAt:     &now,
	}

	if err := c.graph.EmitEntity(ctx, pr, domain.PredicateReviewCompleted); err != nil {
		c.logger.Error("failed to emit peer review entity for accepted node",
			"execution_id", dagState.ExecutionID,
			"node_id", nodeID,
			"review_id", reviewID,
			"error", err)
		c.errorsCount.Add(1)
		return
	}

	c.logger.Info("created peer review for accepted DAG node",
		"execution_id", dagState.ExecutionID,
		"node_id", nodeID,
		"review_id", reviewID,
		"leader_id", leaderID,
		"member_id", memberID)
}

// =============================================================================
// INDEX — maps sub-quest entity keys to their DAGExecutionState
// =============================================================================

// indexDAGState registers a DAGExecutionState into the in-memory indexes.
// It stores the state in dagCache and maps every sub-quest entity key to the
// DAG state in dagBySubQuest, enabling O(1) lookup from sub-quest KV events.
//
// Called only from the event loop goroutine.
func (c *Component) indexDAGState(dagState *DAGExecutionState) {
	c.dagCache[dagState.ExecutionID] = dagState

	for _, subQuestID := range dagState.NodeQuestIDs {
		if subQuestID == "" {
			continue
		}
		entityKey := c.subQuestEntityKey(subQuestID)
		c.dagBySubQuest[entityKey] = dagState
	}
}

// subQuestEntityKey converts a sub-quest ID (which may already be a full entity
// ID like "local.dev.game.board1.quest.abc") into the exact KV key used by the
// graph client. The graph client uses the full entity ID as the KV key.
func (c *Component) subQuestEntityKey(questID string) string {
	if strings.Count(questID, ".") >= 5 {
		return questID
	}
	instance := domain.ExtractInstance(questID)
	return c.boardConfig.QuestEntityID(instance)
}

// =============================================================================
// STATE MACHINE HELPERS
// =============================================================================

// handleNodeCompleted marks the node completed, appends it to CompletedNodes,
// then either triggers rollup (all nodes done) or assigns the next batch of
// ready nodes.
//
// Called only from the event loop goroutine.
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
		c.dispatchLeadSynthesis(ctx, dagState)
		return
	}

	// Promote pending nodes whose dependencies are now met to NodeReady.
	c.promoteReadyNodes(dagState)

	// Assign the newly ready nodes to available party members.
	c.assignReadyNodes(ctx, dagState)
}

// handleNodeFailed decrements the retry budget for the node and either resets
// it to NodePending for another attempt or marks it NodeFailed and escalates
// the parent quest.
//
// Called only from the event loop goroutine.
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

		// Re-claim and re-assign the sub-quest for the same or a new member.
		c.retryNodeAssignment(ctx, dagState, nodeID)
		return
	}

	// No retries left — attempt triage if enabled, otherwise fail permanently.

	// Create a peer review entity recording the failure so the member agent's
	// prompt on future quests can surface corrective guidance.
	c.createReviewEntity(ctx, dagState, nodeID)

	if c.config.TriageEnabled {
		// Route through DM triage instead of immediate parent escalation.
		// The questboard triage watcher will evaluate and either repost
		// (salvage/tpk) or fail/escalate the sub-quest.
		dagState.NodeStates[nodeID] = NodePendingTriage
		c.routeNodeToTriage(ctx, dagState, nodeID)
		return
	}

	// Triage disabled — node fails permanently.
	dagState.NodeStates[nodeID] = NodeFailed
	dagState.FailedNodes = append(dagState.FailedNodes, nodeID)
	c.nodesFailed.Add(1)

	c.logger.Warn("DAG node exhausted retries — escalating parent",
		"execution_id", dagState.ExecutionID,
		"node_id", nodeID,
		"parent_quest_id", dagState.ParentQuestID)

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

	nodeObjective := c.findNodeObjective(dagState, nodeID)
	nodeAcceptance := c.findNodeAcceptance(dagState, nodeID)
	subQuestOutput := c.extractQuestOutput(entity)

	systemPrompt := buildLeadReviewSystemPrompt(nodeObjective, nodeAcceptance, subQuestOutput)

	userPrompt := fmt.Sprintf(
		"Review the party member's work for sub-quest %q.\n\nObjective: %s\n\n"+
			"IMPORTANT: You MUST call the review_sub_quest tool to submit your verdict. "+
			"Do NOT respond with text — use the tool. Rate each question 1-5 and provide accept/reject verdict.",
		subQuestID, nodeObjective,
	)

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
			// questtools needs these for tier/skill gate checks.
			"agent_id":   leadAgentID,
			"trust_tier": float64(domain.TierMaster), // party leads are always Master+
			"quest_id":   subQuestID,
		},
	}

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

// =============================================================================
// CLARIFICATION DISPATCH
// =============================================================================

// dispatchLeadClarification publishes a TaskMessage to the AGENT stream asking
// the lead to answer a party member's clarification question. The lead's answer
// is stored in NodeClarifications and injected into the member's retry prompt.
//
// Mirrors dispatchLeadReview but uses the "clarify-" LoopID prefix and the
// answer_clarification tool instead of review_sub_quest.
func (c *Component) dispatchLeadClarification(ctx context.Context, dagState *DAGExecutionState, nodeID string, entity *graph.EntityState) {
	subQuestID := dagState.NodeQuestIDs[nodeID]
	leadAgentID := c.findLeadAgentID(dagState)
	memberID := dagState.NodeAssignees[nodeID]

	nodeObjective := c.findNodeObjective(dagState, nodeID)
	clarificationQuestion := c.extractQuestOutput(entity)

	systemPrompt := buildLeadClarificationPrompt(nodeObjective, memberID, clarificationQuestion)

	userPrompt := fmt.Sprintf(
		"A party member working on sub-quest %q needs clarification.\n\n"+
			"IMPORTANT: You MUST call the answer_clarification tool. "+
			"Do NOT respond with text — use the tool.",
		subQuestID,
	)

	subjectSafeID := strings.ReplaceAll(subQuestID, ".", "-")
	loopID := fmt.Sprintf("clarify-%s-%s", subjectSafeID, nuid.Next())

	clarifyTools := NewClarificationExecutor().ListTools()

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
		Tools: clarifyTools,
		Metadata: map[string]any{
			"parent_quest_id":        dagState.ParentQuestID,
			"sub_quest_id":           subQuestID,
			"node_id":                nodeID,
			"execution_id":           dagState.ExecutionID,
			"party_id":               dagState.PartyID,
			"lead_agent_id":          leadAgentID,
			"clarification_dispatch": true,
			// questtools needs these for tier/skill gate checks.
			"agent_id":   leadAgentID,
			"trust_tier": float64(domain.TierMaster), // party leads are always Master+
			"quest_id":   subQuestID,
		},
	}

	baseMsg := message.NewBaseMessage(taskMsg.Schema(), &taskMsg, ComponentName)
	data, err := json.Marshal(baseMsg)
	if err != nil {
		c.logger.Error("failed to marshal clarification TaskMessage",
			"sub_quest_id", subQuestID, "node_id", nodeID, "error", err)
		c.errorsCount.Add(1)
		return
	}

	subject := fmt.Sprintf("agent.task.%s", subjectSafeID)
	if err := c.deps.NATSClient.PublishToStream(ctx, subject, data); err != nil {
		c.logger.Error("failed to publish clarification TaskMessage",
			"sub_quest_id", subQuestID, "subject", subject, "error", err)
		c.errorsCount.Add(1)
		return
	}

	c.logger.Info("dispatched lead clarification task",
		"execution_id", dagState.ExecutionID,
		"node_id", nodeID,
		"sub_quest_id", subQuestID,
		"loop_id", loopID,
		"lead_agent_id", leadAgentID,
		"member_id", memberID)
}

// buildLeadClarificationPrompt constructs the system prompt for the lead's
// clarification agentic loop. It presents the member's question and asks the
// lead to answer via the answer_clarification tool.
func buildLeadClarificationPrompt(nodeObjective, memberID, clarificationQuestion string) string {
	var sb strings.Builder
	sb.WriteString("You are the party lead. A party member needs clarification on their sub-quest.\n\n")

	sb.WriteString("Sub-quest objective:\n")
	sb.WriteString(nodeObjective)
	sb.WriteString("\n\n")

	if memberID != "" {
		fmt.Fprintf(&sb, "Member agent: %s\n\n", memberID)
	}

	sb.WriteString("The member asked:\n---\n")
	if clarificationQuestion != "" {
		sb.WriteString(clarificationQuestion)
	} else {
		sb.WriteString("(no question text available)")
	}
	sb.WriteString("\n---\n\n")

	sb.WriteString("Call the answer_clarification tool with the sub_quest_id and your answer.")
	return sb.String()
}

// =============================================================================
// CLARIFICATION COMPLETION HANDLER
// =============================================================================

// onClarificationAnswered handles a clarification loop completion from the AGENT
// stream. It parses the answer from the LoopCompletedEvent result, locates the
// DAG and node for the sub-quest, stores the exchange in NodeClarifications,
// resets the node to NodeAssigned, and transitions the sub-quest back to
// in_progress so questbridge re-dispatches the member's agentic loop.
func (c *Component) onClarificationAnswered(ctx context.Context, evt dagEvent) {
	c.logger.Info("event loop: processing clarification answer",
		"loop_id", evt.LoopID, "result_length", len(evt.Result))

	// Parse the answer JSON from the answer_clarification tool output.
	// Fall back to using the raw result text as the answer when parsing fails.
	var answer struct {
		SubQuestID string `json:"sub_quest_id"`
		Answer     string `json:"answer"`
	}

	if parseErr := json.Unmarshal([]byte(evt.Result), &answer); parseErr != nil || answer.SubQuestID == "" {
		c.logger.Warn("clarification loop returned text instead of tool JSON, using raw result as answer",
			"loop_id", evt.LoopID, "result_prefix", truncate(evt.Result, 80))

		subQuestID := c.extractSubQuestFromLeadLoopID(evt.LoopID)
		if subQuestID == "" {
			c.logger.Warn("cannot extract sub-quest ID from clarification loop ID",
				"loop_id", evt.LoopID)
			return
		}
		answer.SubQuestID = subQuestID
		answer.Answer = evt.Result
	}

	if answer.SubQuestID == "" {
		c.logger.Warn("clarification answer: empty sub_quest_id after parse",
			"loop_id", evt.LoopID)
		return
	}

	entityKey := c.subQuestEntityKey(answer.SubQuestID)
	dagState := c.findDAGForSubQuest(entityKey)
	if dagState == nil {
		c.logger.Warn("clarification answer: sub-quest not part of any active DAG",
			"loop_id", evt.LoopID, "sub_quest_id", answer.SubQuestID,
			"entity_key", entityKey)
		return
	}

	nodeID := c.findNodeForQuest(dagState, answer.SubQuestID)
	if nodeID == "" {
		nodeID = c.findNodeForQuest(dagState, entityKey)
		if nodeID == "" {
			c.logger.Warn("clarification answer: cannot find node for sub-quest in DAG",
				"loop_id", evt.LoopID, "sub_quest_id", answer.SubQuestID,
				"execution_id", dagState.ExecutionID)
			return
		}
	}

	// Load the sub-quest entity with revision for CAS write.
	questEntity, revision, questErr := c.graph.GetQuestWithRevision(ctx, domain.QuestID(answer.SubQuestID))
	if questErr != nil {
		c.logger.Error("failed to load sub-quest for clarification answer",
			"sub_quest_id", answer.SubQuestID, "error", questErr)
		c.errorsCount.Add(1)
		return
	}

	quest := domain.QuestFromEntityState(questEntity)
	if quest == nil {
		c.logger.Warn("clarification answer: quest reconstruction returned nil",
			"sub_quest_id", answer.SubQuestID, "loop_id", evt.LoopID)
		return
	}

	// Retrieve the question text from the quest's output field (set by questbridge
	// when routing the escalation to the lead instead of the DM).
	question := c.extractQuestOutput(questEntity)

	newExchange := ClarificationExchange{
		Question: question,
		Answer:   answer.Answer,
		AskedAt:  time.Now(),
	}

	// Store the Q&A exchange in the in-memory DAG state so questbridge can
	// inject it into the member's prompt on the next dispatch.
	if dagState.NodeClarifications == nil {
		dagState.NodeClarifications = make(map[string][]ClarificationExchange)
	}
	dagState.NodeClarifications[nodeID] = append(dagState.NodeClarifications[nodeID], newExchange)

	// Reset the node to NodeAssigned so the next dispatchLeadReview / re-assignment
	// loop can pick it up. The ADR specifies NodeAssigned → re-dispatch.
	dagState.NodeStates[nodeID] = NodeAssigned

	// Write clarification exchange onto the sub-quest entity via CAS so
	// questbridge can load it from the graph on the next dispatch.
	// CAS ensures we don't overwrite a concurrent questboard status transition.
	quest.DAGClarifications = dagState.NodeClarifications[nodeID]
	quest.Status = domain.QuestInProgress
	quest.Escalated = false
	if emitErr := c.graph.EmitEntityCAS(ctx, quest, "quest.dag.clarification_answered", revision); emitErr != nil {
		c.logger.Error("failed to reset sub-quest status after clarification",
			"sub_quest_id", answer.SubQuestID, "error", emitErr,
			"revision", revision)
		c.errorsCount.Add(1)
		return
	}

	c.logger.Info("clarification answered — sub-quest reset for retry",
		"loop_id", evt.LoopID,
		"sub_quest_id", answer.SubQuestID,
		"node_id", nodeID,
		"execution_id", dagState.ExecutionID,
		"clarifications_on_node", len(dagState.NodeClarifications[nodeID]))

	// Persist the updated DAG state to the parent quest entity.
	if persistErr := c.persistDAGState(ctx, dagState); persistErr != nil {
		c.logger.Error("failed to persist DAG state after clarification answer",
			"execution_id", dagState.ExecutionID, "node_id", nodeID, "error", persistErr)
		c.errorsCount.Add(1)
	}
}

// buildLeadReviewSystemPrompt constructs the system prompt for the lead's
// review agentic loop. It includes the objective, acceptance criteria, and
// the member's output.
func buildLeadReviewSystemPrompt(objective string, acceptance []string, output string) string {
	var sb strings.Builder
	sb.WriteString("You are the party lead performing a BLIND PEER REVIEW of a party member's work.\n\n")

	sb.WriteString("WHY THIS REVIEW MATTERS:\n")
	sb.WriteString("Review your peers as if your future success depends on it — because it does. ")
	sb.WriteString("These ratings determine who gets assigned to YOUR team next time. ")
	sb.WriteString("If you inflate scores, agents who produce mediocre work get placed on future parties ")
	sb.WriteString("as if they were top performers. When they fail on a critical sub-quest, ")
	sb.WriteString("it's YOUR deliverable that suffers and YOUR reputation on the line. ")
	sb.WriteString("Honest reviews protect you. Dishonest reviews cost you.\n\n")

	sb.WriteString("REVIEW STANDARDS:\n")
	sb.WriteString("Your ratings become part of this agent's permanent record:\n")
	sb.WriteString("- A 5 means genuinely excellent work that surprised you — not merely acceptable\n")
	sb.WriteString("- Average competent work that meets requirements is a 3, not a 5\n")
	sb.WriteString("- Reserve 4 for work that shows clear quality beyond what was asked\n")
	sb.WriteString("- A 1-2 indicates significant gaps or errors\n")
	sb.WriteString("- If you would not stake your own reputation on this output, do not rate it highly\n\n")

	sb.WriteString("RATING SCALE:\n")
	sb.WriteString("  1 = Unacceptable — fundamentally wrong, missing, or unusable\n")
	sb.WriteString("  2 = Below expectations — significant gaps, errors, or missing requirements\n")
	sb.WriteString("  3 = Meets expectations — correct, complete, does what was asked\n")
	sb.WriteString("  4 = Exceeds expectations — well-structured, thorough, handles edge cases\n")
	sb.WriteString("  5 = Exceptional — production-quality, elegant, teaches something new\n\n")

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

	sb.WriteString("Use the review_sub_quest tool to submit your verdict (accept or reject).\n")
	sb.WriteString("Be honest. A 3 for solid work is the correct rating — not a 5.")
	return sb.String()
}

// =============================================================================
// LEAD SYNTHESIS
// =============================================================================

// dispatchLeadSynthesis publishes a TaskMessage to the AGENT stream asking
// the lead to synthesize all sub-quest outputs into a final deliverable.
// Called when all DAG nodes are completed and reviewed. The synthesis result
// becomes the parent quest's output via triggerRollupWithResult.
func (c *Component) dispatchLeadSynthesis(ctx context.Context, dagState *DAGExecutionState) {
	// In unit tests or when NATS is unavailable, fall back to mechanical rollup.
	if c.deps.NATSClient == nil {
		c.triggerRollup(ctx, dagState)
		return
	}

	leadAgentID := c.findLeadAgentID(dagState)

	// Collect all sub-quest outputs.
	outputs := c.collectSubQuestOutputs(ctx, dagState)

	systemPrompt := buildLeadSynthesisPrompt(dagState, outputs)
	userPrompt := "All sub-quests are complete. Synthesize the outputs into a single cohesive deliverable. " +
		"Respond with [INTENT: work_product] followed by the combined result."

	parentKey := strings.ReplaceAll(dagState.ParentQuestID, ".", "-")
	loopID := fmt.Sprintf("synthesis-%s-%s", parentKey, nuid.Next())

	taskMsg := agentic.TaskMessage{
		TaskID: dagState.ParentQuestID,
		LoopID: loopID,
		Role:   agentic.RoleGeneral,
		Model:  "agent-work",
		Prompt: userPrompt,
		Context: &pkgtypes.ConstructedContext{
			Content:       systemPrompt,
			ConstructedAt: time.Now(),
		},
		Metadata: map[string]any{
			"parent_quest_id":    dagState.ParentQuestID,
			"execution_id":       dagState.ExecutionID,
			"party_id":           dagState.PartyID,
			"lead_agent_id":      leadAgentID,
			"synthesis_dispatch": true,
			"agent_id":           leadAgentID,
			"trust_tier":         float64(domain.TierMaster),
			"quest_id":           dagState.ParentQuestID,
		},
	}

	baseMsg := message.NewBaseMessage(taskMsg.Schema(), &taskMsg, ComponentName)
	data, err := json.Marshal(baseMsg)
	if err != nil {
		c.logger.Error("failed to marshal synthesis TaskMessage",
			"execution_id", dagState.ExecutionID, "error", err)
		c.errorsCount.Add(1)
		// Fall back to mechanical rollup.
		c.triggerRollup(ctx, dagState)
		return
	}

	subject := fmt.Sprintf("agent.task.%s", parentKey)
	if err := c.deps.NATSClient.PublishToStream(ctx, subject, data); err != nil {
		c.logger.Error("failed to publish synthesis TaskMessage",
			"execution_id", dagState.ExecutionID, "subject", subject, "error", err)
		c.errorsCount.Add(1)
		// Fall back to mechanical rollup.
		c.triggerRollup(ctx, dagState)
		return
	}

	c.logger.Info("dispatched lead synthesis task",
		"execution_id", dagState.ExecutionID,
		"parent_quest_id", dagState.ParentQuestID,
		"loop_id", loopID,
		"lead_agent_id", leadAgentID,
		"outputs_count", len(outputs))
}

// collectSubQuestOutputs loads and returns outputs from all completed sub-quests.
func (c *Component) collectSubQuestOutputs(ctx context.Context, dagState *DAGExecutionState) map[string]any {
	outputs := make(map[string]any, len(dagState.CompletedNodes))
	if c.graph == nil {
		return outputs
	}
	for _, nodeID := range dagState.CompletedNodes {
		subQuestID, ok := dagState.NodeQuestIDs[nodeID]
		if !ok || subQuestID == "" {
			continue
		}
		questEntity, err := c.graph.GetQuest(ctx, domain.QuestID(subQuestID))
		if err != nil {
			c.logger.Warn("failed to load sub-quest for synthesis",
				"sub_quest_id", subQuestID, "node_id", nodeID, "error", err)
			continue
		}
		quest := domain.QuestFromEntityState(questEntity)
		if quest != nil && quest.Output != nil {
			outputs[nodeID] = quest.Output
		}
	}
	return outputs
}

// buildLeadSynthesisPrompt constructs the system prompt for the lead's
// synthesis loop, presenting all sub-quest outputs for combination.
func buildLeadSynthesisPrompt(dagState *DAGExecutionState, outputs map[string]any) string {
	var sb strings.Builder
	sb.WriteString("You are the party lead. All sub-quests are complete and reviewed.\n\n")
	sb.WriteString("Your job: combine the sub-quest outputs below into a single, cohesive deliverable.\n\n")

	sb.WriteString("ORIGINAL QUEST:\n")
	sb.WriteString(dagState.QuestTitle)
	sb.WriteString("\n\n")

	sb.WriteString("SUB-QUEST OUTPUTS:\n")
	for nodeID, output := range outputs {
		// Find the node objective for context.
		objective := ""
		for _, node := range dagState.DAG.Nodes {
			if node.ID == nodeID {
				objective = node.Objective
				break
			}
		}
		fmt.Fprintf(&sb, "--- %s", nodeID)
		if objective != "" {
			fmt.Fprintf(&sb, " (%s)", objective)
		}
		sb.WriteString(" ---\n")
		fmt.Fprintf(&sb, "%v\n\n", output)
	}

	sb.WriteString("INSTRUCTIONS:\n")
	sb.WriteString("1. Combine these outputs into a single coherent result that fulfills the original quest.\n")
	sb.WriteString("2. Do not simply concatenate — integrate, organize, and ensure consistency.\n")
	sb.WriteString("3. Respond with [INTENT: work_product] followed by the final deliverable.\n")
	return sb.String()
}

// =============================================================================
// ROLLUP
// =============================================================================

// triggerRollupWithResult submits a pre-synthesized result as the parent quest
// output. Called by onSynthesisCompleted after the lead combines sub-quest outputs.
func (c *Component) triggerRollupWithResult(ctx context.Context, dagState *DAGExecutionState, result string) {
	c.rollupsTriggered.Add(1)

	c.logger.Info("triggering DAG rollup with synthesized result",
		"execution_id", dagState.ExecutionID,
		"parent_quest_id", dagState.ParentQuestID,
		"result_length", len(result))

	if qb := c.resolveQuestBoard(); qb != nil {
		parentID := domain.QuestID(dagState.ParentQuestID)
		if err := qb.SubmitResult(ctx, parentID, result); err != nil {
			c.logger.Error("failed to submit synthesized rollup result to questboard",
				"parent_quest_id", dagState.ParentQuestID, "error", err)
			c.errorsCount.Add(1)
		}
	}

	if pc := c.resolvePartyCoord(); pc != nil && dagState.PartyID != "" {
		partyID := domain.PartyID(dagState.PartyID)
		if err := pc.DisbandParty(ctx, partyID, "DAG completed"); err != nil {
			c.logger.Warn("failed to disband party after rollup",
				"party_id", dagState.PartyID, "error", err)
		}
	}

	c.completedDAGKeys.Store(dagState.ParentQuestID, true)
	c.dagStartTimes.Delete(dagState.ExecutionID)

	delete(c.dagCache, dagState.ExecutionID)
	for key, ds := range c.dagBySubQuest {
		if ds.ExecutionID == dagState.ExecutionID {
			delete(c.dagBySubQuest, key)
		}
	}
	c.pruneReviewRetries(dagState.ExecutionID)

	c.logger.Info("DAG rollup complete (synthesized)",
		"execution_id", dagState.ExecutionID,
		"parent_quest_id", dagState.ParentQuestID)
}

// triggerRollup collects outputs from all completed sub-quests, submits the
// aggregated result to the parent quest via questboard, and disbands the party.
// Used as fallback when synthesis dispatch fails.
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

	if qb := c.resolveQuestBoard(); qb != nil {
		parentID := domain.QuestID(dagState.ParentQuestID)
		if err := qb.SubmitResult(ctx, parentID, outputs); err != nil {
			c.logger.Error("failed to submit rollup result to questboard",
				"parent_quest_id", dagState.ParentQuestID, "error", err)
			c.errorsCount.Add(1)
			// Continue to disband the party even if rollup submission fails.
		}
	}

	if pc := c.resolvePartyCoord(); pc != nil && dagState.PartyID != "" {
		partyID := domain.PartyID(dagState.PartyID)
		if err := pc.DisbandParty(ctx, partyID, "DAG completed"); err != nil {
			c.logger.Warn("failed to disband party after rollup",
				"party_id", dagState.PartyID, "error", err)
		}
	}

	// Mark parent quest entity key as completed so produceQuestEvents can prune
	// seenDAGParents and allow future DAGs on the same entity key.
	c.completedDAGKeys.Store(dagState.ParentQuestID, true)
	c.dagStartTimes.Delete(dagState.ExecutionID)

	// Clean up in-memory caches for this DAG.
	delete(c.dagCache, dagState.ExecutionID)
	for key, ds := range c.dagBySubQuest {
		if ds.ExecutionID == dagState.ExecutionID {
			delete(c.dagBySubQuest, key)
		}
	}
	c.pruneReviewRetries(dagState.ExecutionID)

	c.logger.Info("DAG rollup complete",
		"execution_id", dagState.ExecutionID,
		"parent_quest_id", dagState.ParentQuestID,
		"outputs_collected", len(outputs))
}

// =============================================================================
// READY-NODE PROMOTION AND ASSIGNMENT
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
	pc := c.resolvePartyCoord()
	if pc == nil {
		c.logger.Warn("partyCoord not available — skipping ready node assignment")
		return
	}
	qb := c.resolveQuestBoard()
	if qb == nil {
		c.logger.Warn("questBoardRef not available — skipping ready node assignment")
		return
	}

	if c.logger.Enabled(context.Background(), slog.LevelDebug) {
		readyCount := 0
		for nodeID, state := range dagState.NodeStates {
			if state == NodeReady {
				readyCount++
			}
			c.logger.Debug("assignReadyNodes: node state",
				"execution_id", dagState.ExecutionID,
				"node_id", nodeID, "state", state)
		}
		c.logger.Debug("assignReadyNodes: entering",
			"execution_id", dagState.ExecutionID,
			"party_id", dagState.PartyID,
			"ready_count", readyCount)
	}

	deps := AssignmentDeps{
		Members:     &partyCoordMemberLister{ref: pc},
		Tasks:       &partyCoordTaskAssigner{ref: pc},
		QuestClaims: &questBoardClaimer{ref: qb},
	}

	if err := AssignReadyNodes(ctx, dagState, deps); err != nil {
		c.logger.Warn("failed to assign ready nodes",
			"execution_id", dagState.ExecutionID, "error", err)
		c.errorsCount.Add(1)
		return
	}

	c.logger.Debug("assignReadyNodes: completed",
		"execution_id", dagState.ExecutionID,
		"assignees", dagState.NodeAssignees)
}

// retryNodeAssignment re-dispatches a failed node by resetting the sub-quest
// back to posted status and then re-claiming it for the party.
func (c *Component) retryNodeAssignment(ctx context.Context, dagState *DAGExecutionState, nodeID string) {
	// Reset the sub-quest back to posted so ClaimAndStartForParty can re-claim it.
	subQuestID, ok := dagState.NodeQuestIDs[nodeID]
	if ok {
		if qb := c.resolveQuestBoard(); qb != nil {
			if err := qb.RepostForRetry(ctx, domain.QuestID(subQuestID)); err != nil {
				c.logger.Error("failed to repost sub-quest for retry",
					"execution_id", dagState.ExecutionID,
					"node_id", nodeID,
					"sub_quest_id", subQuestID,
					"error", err)
				c.errorsCount.Add(1)
			}
		}
	}

	dagState.NodeStates[nodeID] = NodePending
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
	// Clean up active sub-quest loops before escalating — prevents sibling
	// sub-quests from running indefinitely after the parent is escalated.
	c.cleanupDAGSubQuests(ctx, dagState)

	qb := c.resolveQuestBoard()
	if qb == nil {
		c.logger.Warn("questBoardRef not available — cannot escalate parent quest",
			"parent_quest_id", dagState.ParentQuestID, "reason", reason)
		return
	}

	parentID := domain.QuestID(dagState.ParentQuestID)
	if err := qb.EscalateQuest(ctx, parentID, reason); err != nil {
		c.logger.Error("failed to escalate parent quest",
			"parent_quest_id", dagState.ParentQuestID, "error", err)
		c.errorsCount.Add(1)
	}
}

// sendCancelSignal publishes a cancel UserSignal to the AGENT stream for the
// given loop ID. The agentic-loop watches agent.signal.{loopID} and terminates
// the loop when it receives a cancel signal.
func (c *Component) sendCancelSignal(ctx context.Context, loopID string) {
	if loopID == "" {
		return
	}

	js, err := c.deps.NATSClient.JetStream()
	if err != nil {
		c.logger.Warn("sendCancelSignal: failed to get JetStream", "error", err)
		return
	}

	signal := &agentic.UserSignal{
		SignalID:    "cancel-" + nuid.Next(),
		Type:        agentic.SignalCancel,
		LoopID:      loopID,
		UserID:      "questdagexec",
		ChannelType: "system",
		ChannelID:   "questdagexec",
		Timestamp:   time.Now(),
	}

	baseMsg := message.NewBaseMessage(signal.Schema(), signal, "questdagexec")
	data, marshalErr := json.Marshal(baseMsg)
	if marshalErr != nil {
		c.logger.Warn("sendCancelSignal: failed to marshal signal", "error", marshalErr)
		return
	}

	subject := fmt.Sprintf("agent.signal.%s", loopID)
	if _, pubErr := js.Publish(ctx, subject, data); pubErr != nil {
		c.logger.Warn("sendCancelSignal: failed to publish cancel signal",
			"loop_id", loopID, "error", pubErr)
	} else {
		c.logger.Info("sent cancel signal to loop", "loop_id", loopID)
	}
}

// cleanupDAGSubQuests cancels active agentic loops for in-progress sub-quests
// and fails all incomplete sub-quests. Called from onDAGTimedOut and escalateParent.
func (c *Component) cleanupDAGSubQuests(ctx context.Context, dagState *DAGExecutionState) {
	qb := c.resolveQuestBoard()

	for nodeID, subQuestID := range dagState.NodeQuestIDs {
		if subQuestID == "" {
			continue
		}

		state := dagState.NodeStates[nodeID]
		switch state {
		case NodeInProgress, NodePendingReview, NodeAwaitingClarification:
			// Active loop — send cancel signal. Look up the quest to find LoopID.
			questEntity, err := c.graph.GetQuest(ctx, domain.QuestID(subQuestID))
			if err == nil {
				quest := domain.QuestFromEntityState(questEntity)
				if quest != nil && quest.LoopID != "" {
					c.sendCancelSignal(ctx, quest.LoopID)
				}
			}

			// Fail the sub-quest.
			if qb != nil {
				if err := qb.FailQuest(ctx, domain.QuestID(subQuestID), "Parent DAG cancelled"); err != nil {
					c.logger.Warn("cleanupDAGSubQuests: failed to fail sub-quest",
						"sub_quest_id", subQuestID, "error", err)
				}
			}
			dagState.NodeStates[nodeID] = NodeFailed

		case NodePending, NodeReady, NodeAssigned:
			// Not yet started — just fail it.
			if qb != nil {
				if err := qb.FailQuest(ctx, domain.QuestID(subQuestID), "Parent DAG cancelled"); err != nil {
					c.logger.Warn("cleanupDAGSubQuests: failed to fail pending sub-quest",
						"sub_quest_id", subQuestID, "error", err)
				}
			}
			dagState.NodeStates[nodeID] = NodeFailed
		}
		// NodeCompleted, NodeFailed — already terminal, skip.
	}
}

// sweepStaleDags periodically checks dagStartTimes for DAGs that have exceeded
// the configured timeout. When found, it sends a dagEventDAGTimedOut event
// to the event loop and removes the entry to avoid re-sending.
//
// Follows the sweepStaleEscalations pattern from questbridge/handler.go.
func (c *Component) sweepStaleDags(ctx context.Context) {
	defer c.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.dagStartTimes.Range(func(key, value any) bool {
				executionID := key.(string)
				startTime := value.(time.Time)
				elapsed := time.Since(startTime)

				if elapsed <= c.config.DAGTimeout {
					return true
				}

				c.logger.Warn("DAG exceeded timeout — sending timeout event",
					"execution_id", executionID,
					"elapsed", elapsed.String(),
					"timeout", c.config.DAGTimeout.String())

				// Send timeout event to the event loop.
				evt := dagEvent{
					Type:        dagEventDAGTimedOut,
					EntityKey:   executionID, // repurposed to carry executionID
					ErrorReason: elapsed.String(),
				}

				// Delete from map only after successful send to avoid losing
				// the timeout if the component is shutting down.
				select {
				case c.events <- evt:
					c.dagStartTimes.Delete(executionID)
				case <-c.stopChan:
				case <-ctx.Done():
				}

				return true
			})
		}
	}
}

// pruneReviewRetries removes all reviewRetries entries for a completed DAG.
// Called from cleanup paths (triggerRollup, triggerRollupWithResult, onDAGTimedOut)
// to prevent unbounded growth of the reviewRetries map.
func (c *Component) pruneReviewRetries(executionID string) {
	for key := range c.reviewRetries {
		if strings.Contains(key, executionID) {
			delete(c.reviewRetries, key)
		}
	}
}

// =============================================================================
// TRIAGE ROUTING (called only from event loop goroutine)
// =============================================================================

// routeNodeToTriage transitions a sub-quest from failed to pending_triage
// so the questboard triage watcher can evaluate it. Called when DAG node
// retries are exhausted and triage is enabled.
//
// If the transition fails, falls back to immediate parent escalation.
func (c *Component) routeNodeToTriage(ctx context.Context, dagState *DAGExecutionState, nodeID string) {
	subQuestID := dagState.NodeQuestIDs[nodeID]
	if subQuestID == "" {
		c.logger.Warn("routeNodeToTriage: no sub-quest ID for node",
			"execution_id", dagState.ExecutionID, "node_id", nodeID)
		c.fallbackToEscalate(ctx, dagState, nodeID)
		return
	}

	if c.graph == nil {
		c.logger.Warn("routeNodeToTriage: graph client not set",
			"execution_id", dagState.ExecutionID, "node_id", nodeID)
		c.fallbackToEscalate(ctx, dagState, nodeID)
		return
	}

	questEntity, err := c.graph.GetQuest(ctx, domain.QuestID(subQuestID))
	if err != nil {
		c.logger.Error("routeNodeToTriage: failed to load sub-quest",
			"sub_quest_id", subQuestID, "error", err)
		c.errorsCount.Add(1)
		c.fallbackToEscalate(ctx, dagState, nodeID)
		return
	}

	quest := domain.QuestFromEntityState(questEntity)
	if quest == nil {
		c.logger.Warn("routeNodeToTriage: quest reconstruction returned nil",
			"sub_quest_id", subQuestID)
		c.fallbackToEscalate(ctx, dagState, nodeID)
		return
	}

	// Record the current failure in history before entering triage.
	record := domain.FailureRecord{
		Attempt:       quest.Attempts,
		FailureType:   quest.FailureType,
		FailureReason: quest.FailureReason,
		Output:        quest.Output,
		LoopID:        quest.LoopID,
		Timestamp:     time.Now(),
	}
	if quest.ClaimedBy != nil {
		record.AgentID = *quest.ClaimedBy
	}
	quest.FailureHistory = append(quest.FailureHistory, record)
	quest.Status = domain.QuestPendingTriage

	if emitErr := c.graph.EmitEntityUpdate(ctx, quest, domain.PredicateQuestPendingTriage); emitErr != nil {
		c.logger.Error("routeNodeToTriage: failed to transition sub-quest to pending_triage",
			"sub_quest_id", subQuestID, "error", emitErr)
		c.errorsCount.Add(1)
		c.fallbackToEscalate(ctx, dagState, nodeID)
		return
	}

	c.logger.Info("DAG node routed to triage",
		"execution_id", dagState.ExecutionID,
		"node_id", nodeID,
		"sub_quest_id", subQuestID,
		"parent_quest_id", dagState.ParentQuestID)
}

// handleTriageRepost handles a sub-quest that was reposted after triage
// (salvage or tpk path). Resets the node to pending and re-assigns.
func (c *Component) handleTriageRepost(ctx context.Context, dagState *DAGExecutionState, nodeID string) {
	dagState.NodeStates[nodeID] = NodePending
	delete(dagState.NodeAssignees, nodeID)

	c.logger.Info("DAG node triage completed — sub-quest reposted for retry",
		"execution_id", dagState.ExecutionID, "node_id", nodeID)

	c.promoteReadyNodes(dagState)
	c.assignReadyNodes(ctx, dagState)
}

// handleTriageTerminal handles a sub-quest where triage decided terminal failure.
// The node is permanently failed and the parent quest is escalated.
func (c *Component) handleTriageTerminal(ctx context.Context, dagState *DAGExecutionState, nodeID string) {
	dagState.NodeStates[nodeID] = NodeFailed
	dagState.FailedNodes = append(dagState.FailedNodes, nodeID)
	c.nodesFailed.Add(1)

	c.logger.Warn("DAG node triage decided terminal — escalating parent",
		"execution_id", dagState.ExecutionID,
		"node_id", nodeID,
		"parent_quest_id", dagState.ParentQuestID)

	reason := fmt.Sprintf("DAG node %s failed after triage decided terminal", nodeID)
	c.escalateParent(ctx, dagState, reason)
}

// fallbackToEscalate reverts a node from NodePendingTriage to NodeFailed
// and escalates the parent. Used when the triage transition itself fails.
func (c *Component) fallbackToEscalate(ctx context.Context, dagState *DAGExecutionState, nodeID string) {
	dagState.NodeStates[nodeID] = NodeFailed
	dagState.FailedNodes = append(dagState.FailedNodes, nodeID)
	c.nodesFailed.Add(1)

	reason := fmt.Sprintf("DAG node %s failed (triage routing failed, falling back to escalation)", nodeID)
	c.escalateParent(ctx, dagState, reason)
}

// =============================================================================
// LOOKUP HELPERS
// =============================================================================

// findDAGForSubQuest returns the DAGExecutionState that contains the given
// sub-quest entity key, or nil if the quest is not part of any active DAG.
// Called only from the event loop goroutine.
func (c *Component) findDAGForSubQuest(entityKey string) *DAGExecutionState {
	return c.dagBySubQuest[entityKey]
}

// findNodeForQuest returns the node ID in the DAG whose NodeQuestIDs value
// matches the given questID. Returns an empty string if not found.
func (c *Component) findNodeForQuest(dagState *DAGExecutionState, questID string) string {
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
	for _, node := range dagState.DAG.Nodes {
		if dagState.NodeStates[node.ID] != NodeCompleted {
			return false
		}
	}
	return len(dagState.DAG.Nodes) > 0
}

// findLeadAgentID returns the lead agent ID from the party, or an empty
// string if the party is unavailable.
func (c *Component) findLeadAgentID(dagState *DAGExecutionState) string {
	if dagState.PartyID == "" {
		return ""
	}
	pc := c.resolvePartyCoord()
	if pc == nil {
		return ""
	}
	party, ok := pc.GetParty(domain.PartyID(dagState.PartyID))
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

// extractSubQuestFromLoopID extracts the sub-quest entity ID from a review loop ID.
// Loop IDs follow: "review-{entity-id-with-dashes}-{nuid}"
// Entity IDs are like "local.dev.game.board1.quest.abc123" which gets sanitized
// to "local-dev-game-board1-quest-abc123" in the loop ID.
//
// Delegates to extractSubQuestFromLeadLoopID.
func (c *Component) extractSubQuestFromLoopID(loopID string) string {
	return c.extractSubQuestFromLeadLoopID(loopID)
}

// extractSubQuestFromLeadLoopID extracts the sub-quest entity ID from a lead
// loop ID. Handles both "review-" and "clarify-" prefixed loop IDs.
// Loop IDs follow: "{prefix}-{entity-id-with-dashes}-{nuid}"
// Entity IDs are like "local.dev.game.board1.quest.abc123" which get sanitized
// to "local-dev-game-board1-quest-abc123" in the loop ID.
func (c *Component) extractSubQuestFromLeadLoopID(loopID string) string {
	var trimmed string
	switch {
	case strings.HasPrefix(loopID, "review-"):
		trimmed = strings.TrimPrefix(loopID, "review-")
	case strings.HasPrefix(loopID, "clarify-"):
		trimmed = strings.TrimPrefix(loopID, "clarify-")
	default:
		return ""
	}

	lastDash := strings.LastIndex(trimmed, "-")
	if lastDash < 0 {
		return ""
	}
	candidate := trimmed[lastDash+1:]
	if len(candidate) >= 20 { // NUIDs are 22 chars
		trimmed = trimmed[:lastDash]
	}

	// Restore dots in the entity ID (org.platform.domain.system.type.instance).
	parts := strings.SplitN(trimmed, "-", 6)
	if len(parts) < 6 {
		return ""
	}
	return strings.Join(parts, ".")
}

// =============================================================================
// DAG STATE RECONSTRUCTION FROM GRAPH
// =============================================================================

// dagStateFromTriples reconstructs a DAGExecutionState from a parent quest
// entity's triples. This is the inverse of persistDAGState: where persist
// writes DAG fields onto the quest entity, this function reads them back.
//
// The quest.dag.* fields are stored as any (interface{}) in domain.Quest.
// After a KV round-trip through JSON they arrive as map[string]any, []any, etc.
// rather than their concrete Go types. The conversion helpers below handle
// the type assertions.
//
// Returns nil if the triples do not contain a valid DAG execution ID.
func dagStateFromTriples(entityKey string, triples []message.Triple, defaultRetries int) *DAGExecutionState {
	executionID := tripleString(triples, "quest.dag.execution_id")
	if executionID == "" {
		return nil
	}

	// Wrap triples in an EntityState so QuestFromEntityState can reconstruct
	// the typed Quest — it knows how to handle every quest.dag.* predicate.
	entityState := &graph.EntityState{
		ID:      entityKey,
		Triples: triples,
	}
	quest := domain.QuestFromEntityState(entityState)
	if quest == nil {
		return nil
	}

	return dagStateFromQuest(quest, defaultRetries)
}

// dagStateFromQuest converts a domain.Quest's DAG fields into a DAGExecutionState.
// All any-typed fields are converted from their JSON round-trip representations
// (map[string]any, []any) to the concrete types DAGExecutionState expects.
//
// If NodeStates is absent (i.e. the quest was just decomposed by questbridge but
// questdagexec has not yet processed it), node states are seeded fresh from the
// DAG definition using DAGReadyNodes. This matches the initialisation logic that
// was previously in BuildDAGExecutionStateFromInit.
func dagStateFromQuest(quest *domain.Quest, defaultRetries int) *DAGExecutionState {
	if quest == nil || quest.DAGExecutionID == "" {
		return nil
	}

	// Reconstruct the QuestDAG from the quest.dag.definition any field.
	dag := anyToQuestDAG(quest.DAGDefinition)

	// Convert the map/slice any fields to concrete typed maps.
	nodeQuestIDs := anyToStringMap(quest.DAGNodeQuestIDs)
	nodeStates := anyToStringMap(quest.DAGNodeStates)
	nodeAssignees := anyToStringMap(quest.DAGNodeAssignees)
	nodeRetries := anyToIntMap(quest.DAGNodeRetries)
	completedNodes := anyToStringSlice(quest.DAGCompletedNodes)
	failedNodes := anyToStringSlice(quest.DAGFailedNodes)

	// If NodeStates is absent, this is a freshly decomposed quest that
	// questdagexec has not yet seeded. Initialise node states from the DAG.
	if len(nodeStates) == 0 && len(dag.Nodes) > 0 {
		nodeStates = make(map[string]string, len(dag.Nodes))
		for _, node := range dag.Nodes {
			nodeStates[node.ID] = NodePending
		}
		for _, readyID := range DAGReadyNodes(dag, nodeStates) {
			nodeStates[readyID] = NodeReady
		}
	}

	// If NodeRetries is absent, seed defaults.
	if len(nodeRetries) == 0 && len(dag.Nodes) > 0 {
		nodeRetries = make(map[string]int, len(dag.Nodes))
		for _, node := range dag.Nodes {
			nodeRetries[node.ID] = defaultRetries
		}
	}

	if nodeAssignees == nil {
		nodeAssignees = make(map[string]string)
	}
	if completedNodes == nil {
		completedNodes = []string{}
	}
	if failedNodes == nil {
		failedNodes = []string{}
	}

	partyID := ""
	if quest.PartyID != nil {
		partyID = string(*quest.PartyID)
	}

	return &DAGExecutionState{
		ExecutionID:    quest.DAGExecutionID,
		ParentQuestID:  string(quest.ID),
		PartyID:        partyID,
		QuestTitle:     quest.Title,
		DAG:            dag,
		NodeStates:     nodeStates,
		NodeQuestIDs:   nodeQuestIDs,
		NodeAssignees:  nodeAssignees,
		CompletedNodes: completedNodes,
		FailedNodes:    failedNodes,
		NodeRetries:    nodeRetries,
	}
}

// anyToQuestDAG converts an any value (from JSON round-trip) to a QuestDAG.
// The any value is expected to be a map[string]any containing a "nodes" array.
func anyToQuestDAG(v any) QuestDAG {
	if v == nil {
		return QuestDAG{}
	}

	// Fast path: already the right type (unlikely after KV round-trip).
	if dag, ok := v.(QuestDAG); ok {
		return dag
	}

	// Marshal back to JSON and unmarshal into QuestDAG — safe across any
	// representation (map[string]any, json.RawMessage, etc.).
	data, err := json.Marshal(v)
	if err != nil {
		return QuestDAG{}
	}
	var dag QuestDAG
	if err := json.Unmarshal(data, &dag); err != nil {
		return QuestDAG{}
	}
	return dag
}

// anyToStringMap converts an any value (from JSON round-trip) to map[string]string.
// After JSON round-trip the map arrives as map[string]any with string values.
func anyToStringMap(v any) map[string]string {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]string); ok {
		return m
	}
	raw, ok := v.(map[string]any)
	if !ok {
		// Try marshal/unmarshal for other representations.
		data, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		var out map[string]string
		if err := json.Unmarshal(data, &out); err != nil {
			return nil
		}
		return out
	}
	result := make(map[string]string, len(raw))
	for k, val := range raw {
		if s, ok := val.(string); ok {
			result[k] = s
		} else {
			slog.Debug("anyToStringMap: dropped non-string value",
				"key", k, "type", fmt.Sprintf("%T", val))
		}
	}
	return result
}

// anyToIntMap converts an any value (from JSON round-trip) to map[string]int.
// After JSON round-trip numeric values arrive as float64.
func anyToIntMap(v any) map[string]int {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]int); ok {
		return m
	}
	raw, ok := v.(map[string]any)
	if !ok {
		data, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		var out map[string]int
		if err := json.Unmarshal(data, &out); err != nil {
			return nil
		}
		return out
	}
	result := make(map[string]int, len(raw))
	for k, val := range raw {
		switch n := val.(type) {
		case float64:
			result[k] = int(n)
		case int:
			result[k] = n
		default:
			slog.Debug("anyToIntMap: dropped non-numeric value",
				"key", k, "type", fmt.Sprintf("%T", val))
		}
	}
	return result
}

// anyToStringSlice converts an any value (from JSON round-trip) to []string.
// After JSON round-trip slices arrive as []any with string elements.
func anyToStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	if s, ok := v.([]string); ok {
		return s
	}
	raw, ok := v.([]any)
	if !ok {
		data, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		var out []string
		if err := json.Unmarshal(data, &out); err != nil {
			return nil
		}
		return out
	}
	result := make([]string, 0, len(raw))
	for i, val := range raw {
		if s, ok := val.(string); ok {
			result = append(result, s)
		} else {
			slog.Debug("anyToStringSlice: dropped non-string element",
				"index", i, "type", fmt.Sprintf("%T", val))
		}
	}
	return result
}

// =============================================================================
// DAG STATE PERSISTENCE
// =============================================================================

// persistDAGState writes the mutable DAG fields (NodeStates, NodeAssignees,
// CompletedNodes, FailedNodes, NodeRetries) back onto the parent quest entity
// as quest.dag.* predicates. It uses a CAS read-modify-write loop to avoid
// overwriting concurrent questboard status transitions.
//
// The read-modify-write pattern preserves questboard's fields (Status, etc.)
// while patching only the DAG-owned predicates. Up to maxRetries CAS retries
// are attempted before giving up with an error.
//
// This is the inverse of dagStateFromQuest: the full round-trip is:
//   graph write (questbridge) → KV watch → dagStateFromTriples → in-memory mutate → persistDAGState → graph write
func (c *Component) persistDAGState(ctx context.Context, state *DAGExecutionState) error {
	const maxRetries = 3
	for attempt := range maxRetries {
		// 1. Read fresh entity + KV revision.
		entityState, revision, err := c.graph.GetQuestWithRevision(ctx, domain.QuestID(state.ParentQuestID))
		if err != nil {
			return fmt.Errorf("read parent quest for CAS: %w", err)
		}
		quest := domain.QuestFromEntityState(entityState)
		if quest == nil {
			return fmt.Errorf("failed to reconstruct parent quest from entity state")
		}

		// 2. Apply DAG state onto the fresh quest, preserving questboard fields.
		quest.DAGNodeStates = state.NodeStates
		quest.DAGNodeAssignees = state.NodeAssignees
		quest.DAGCompletedNodes = state.CompletedNodes
		quest.DAGFailedNodes = state.FailedNodes
		quest.DAGNodeRetries = state.NodeRetries

		// 3. CAS write — returns ErrKVRevisionMismatch if questboard wrote concurrently.
		if err := c.graph.EmitEntityCAS(ctx, quest, "quest.dag.state_updated", revision); err != nil {
			if errors.Is(err, natsclient.ErrKVRevisionMismatch) {
				c.logger.Debug("CAS conflict on parent quest, retrying",
					"execution_id", state.ExecutionID, "attempt", attempt+1)
				continue
			}
			return fmt.Errorf("CAS write DAG state: %w", err)
		}

		c.logger.Debug("persisted DAG state to graph",
			"execution_id", state.ExecutionID, "parent_quest_id", state.ParentQuestID)
		return nil
	}
	return fmt.Errorf("DAG state CAS exhausted %d retries for execution %s", maxRetries, state.ExecutionID)
}

// =============================================================================
// INTERFACE ADAPTERS
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

// questBoardClaimer wraps QuestBoardRef to satisfy QuestClaimerAndStarter.
type questBoardClaimer struct{ ref QuestBoardRef }

func (a *questBoardClaimer) ClaimAndStartForParty(ctx context.Context, questID domain.QuestID, partyID domain.PartyID, assignedTo domain.AgentID) error {
	return a.ref.ClaimAndStartForParty(ctx, questID, partyID, assignedTo)
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

// timePtr returns a pointer to the given time.
func timePtr(t time.Time) *time.Time { return &t }

// truncate returns the first n characters of s, appending "..." if truncated.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

