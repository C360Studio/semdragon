package questdagexec

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/graph"
)

// =============================================================================
// EVENT LOOP — single goroutine, no mutexes on DAG state
// =============================================================================
//
// runEventLoop is the sole goroutine that reads and writes dagCache,
// dagBySubQuest, and questCache. Because all DAG mutations flow through this
// function sequentially, no mutexes are needed on those plain maps.
//
// The two producer goroutines (produceQuestEvents, produceReviewEvents) only
// read from NATS and write to the c.events channel. They never touch the maps.
//
// Metrics atomics (nodesCompleted, nodesFailed, etc.) are always safe to write
// from any goroutine; no additional synchronisation needed.
// =============================================================================

// runEventLoop drains the events channel and dispatches each event to the
// appropriate handler. It terminates when ctx is cancelled, stopChan is closed,
// or the events channel is closed.
func (c *Component) runEventLoop(ctx context.Context) {
	defer c.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopChan:
			return
		case evt, ok := <-c.events:
			if !ok {
				return
			}
			c.handleEvent(ctx, evt)
		}
	}
}

// handleEvent dispatches a dagEvent to the correct handler and updates the
// lastActivity timestamp. Called exclusively from runEventLoop.
func (c *Component) handleEvent(ctx context.Context, evt dagEvent) {
	c.lastActivity.Store(time.Now())
	switch evt.Type {
	case dagEventNewDAG, dagEventDAGUpdated:
		c.onDAGEntry(ctx, evt)
	case dagEventSubQuestTransition:
		c.onSubQuestTransition(ctx, evt)
	case dagEventReviewCompleted:
		c.onReviewCompleted(ctx, evt)
	case dagEventClarificationAnswered:
		c.onClarificationAnswered(ctx, evt)
	case dagEventSynthesisCompleted:
		c.onSynthesisCompleted(ctx, evt)
	}
}

// =============================================================================
// HANDLER: dagEventNewDAG / dagEventDAGUpdated
// =============================================================================

// onDAGEntry handles a dagEventNewDAG emitted by produceQuestEvents when a
// parent quest entity with quest.dag.execution_id is seen for the first time.
//
// It reconstructs DAGExecutionState from the entity's triples via dagStateFromQuest,
// then indexes, promotes, and assigns ready nodes.
//
// During bootstrap (evt.Bootstrapping == true) the DAG is indexed into dagCache
// and dagBySubQuest but no assignment or persistence side-effects are triggered —
// the historical quest watcher replay establishes the in-memory state and the
// live event loop takes over from there.
//
// Feedback-loop prevention: seenDAGParents in produceQuestEvents ensures this
// handler is only called once per parent quest entity. Subsequent persistDAGState
// writes back onto the same entity key but those updates do NOT re-emit
// dagEventNewDAG because the entity key is already in seenDAGParents.
func (c *Component) onDAGEntry(ctx context.Context, evt dagEvent) {
	// Reconstruct DAGExecutionState from the parent quest entity's triples.
	dagState := dagStateFromTriples(evt.EntityKey, evt.Triples, c.config.MaxRetriesPerNode)
	if dagState == nil {
		c.logger.Warn("onDAGEntry: failed to reconstruct DAG state from triples",
			"entity_key", evt.EntityKey)
		return
	}

	if _, exists := c.dagCache[dagState.ExecutionID]; exists {
		// Already indexed — should not happen since seenDAGParents guards entry,
		// but be defensive and skip rather than double-processing.
		c.logger.Debug("onDAGEntry: DAG already in cache, skipping",
			"execution_id", dagState.ExecutionID)
		return
	}

	c.logger.Info("event loop: new DAG indexed",
		"execution_id", dagState.ExecutionID,
		"parent_quest_id", dagState.ParentQuestID,
		"nodes", len(dagState.DAG.Nodes),
		"bootstrapping", evt.Bootstrapping)

	c.indexDAGState(dagState)

	// During bootstrap, do not assign or persist — the state as written in the
	// graph is the source of truth and any in-flight assignments are already
	// tracked via the sub-quest entity status transitions in questCache.
	if evt.Bootstrapping {
		return
	}

	c.promoteReadyNodes(dagState)
	c.assignReadyNodes(ctx, dagState)

	if persistErr := c.persistDAGState(ctx, dagState); persistErr != nil {
		c.logger.Error("failed to persist DAG state after new DAG assignment",
			"execution_id", dagState.ExecutionID, "error", persistErr)
		c.errorsCount.Add(1)
	}
}

// =============================================================================
// HANDLER: dagEventSubQuestTransition
// =============================================================================

// onSubQuestTransition handles a quest status event from produceQuestEvents.
//
// During bootstrap (Bootstrapping == true) it only primes questCache and does
// not trigger any DAG actions. After bootstrap, it detects status changes and
// drives the DAG state machine.
//
// questCache is owned exclusively by this goroutine — no mutex needed.
func (c *Component) onSubQuestTransition(ctx context.Context, evt dagEvent) {
	if evt.Bootstrapping {
		// Bootstrap phase: prime questCache so live events can detect transitions.
		c.questCache[evt.EntityKey] = evt.NewStatus
		return
	}

	oldStatus := c.questCache[evt.EntityKey]
	// Update questCache to reflect the new status.
	c.questCache[evt.EntityKey] = evt.NewStatus

	// Skip if status did not actually change (idempotent write from another processor).
	if oldStatus == evt.NewStatus {
		return
	}

	// Only react to sub-quests that are part of an active DAG.
	dagState := c.findDAGForSubQuest(evt.EntityKey)
	if dagState == nil {
		return
	}

	nodeID := c.findNodeForQuest(dagState, evt.EntityKey)
	if nodeID == "" {
		return
	}

	entity := &graph.EntityState{
		ID:      evt.EntityKey,
		Triples: evt.Triples,
	}

	c.handleSubQuestTransition(ctx, dagState, nodeID, domain.QuestStatus(oldStatus), domain.QuestStatus(evt.NewStatus), entity)
}

// =============================================================================
// HANDLER: dagEventReviewCompleted
// =============================================================================

// onReviewCompleted handles a review loop completion from the AGENT stream.
// It parses the verdict from the LoopCompletedEvent result, locates the DAG
// and node for the reviewed sub-quest, and transitions the node accordingly.
func (c *Component) onReviewCompleted(ctx context.Context, evt dagEvent) {
	c.logger.Info("event loop: processing review completion",
		"loop_id", evt.LoopID, "result_length", len(evt.Result))

	// Parse the verdict JSON from the review_sub_quest tool output.
	// Fall back to heuristic extraction when the LLM responded with prose.
	var verdict struct {
		Verdict    string `json:"verdict"`
		SubQuestID string `json:"sub_quest_id"`
	}

	if parseErr := json.Unmarshal([]byte(evt.Result), &verdict); parseErr != nil {
		c.logger.Warn("review loop returned text instead of tool JSON, inferring verdict",
			"loop_id", evt.LoopID, "result_prefix", truncate(evt.Result, 80))

		subQuestID := c.extractSubQuestFromLoopID(evt.LoopID)
		if subQuestID == "" {
			c.logger.Warn("cannot extract sub-quest ID from review loop ID",
				"loop_id", evt.LoopID)
			return
		}

		lowerResult := strings.ToLower(evt.Result)
		if strings.Contains(lowerResult, "reject") {
			verdict.Verdict = "reject"
		} else {
			verdict.Verdict = "accept"
		}
		verdict.SubQuestID = subQuestID
	}

	if verdict.SubQuestID == "" {
		c.logger.Warn("review completion: empty sub_quest_id after verdict parse",
			"loop_id", evt.LoopID)
		return
	}

	entityKey := c.subQuestEntityKey(verdict.SubQuestID)
	dagState := c.findDAGForSubQuest(entityKey)
	if dagState == nil {
		c.logger.Warn("review completion: sub-quest not part of any active DAG",
			"loop_id", evt.LoopID, "sub_quest_id", verdict.SubQuestID,
			"entity_key", entityKey)
		return
	}

	nodeID := c.findNodeForQuest(dagState, verdict.SubQuestID)
	if nodeID == "" {
		nodeID = c.findNodeForQuest(dagState, entityKey)
		if nodeID == "" {
			c.logger.Warn("review completion: cannot find node for sub-quest in DAG",
				"loop_id", evt.LoopID, "sub_quest_id", verdict.SubQuestID,
				"execution_id", dagState.ExecutionID)
			return
		}
	}

	// Load the sub-quest entity to get its current state for the verdict transition.
	questEntity, questErr := c.graph.GetQuest(ctx, domain.QuestID(verdict.SubQuestID))
	if questErr != nil {
		c.logger.Error("failed to load sub-quest for review verdict",
			"sub_quest_id", verdict.SubQuestID, "error", questErr)
		c.errorsCount.Add(1)
		return
	}

	quest := domain.QuestFromEntityState(questEntity)
	if quest == nil {
		c.logger.Warn("review completion: quest reconstruction returned nil",
			"sub_quest_id", verdict.SubQuestID, "loop_id", evt.LoopID)
		return
	}

	c.logger.Info("applying review verdict",
		"loop_id", evt.LoopID, "verdict", verdict.Verdict,
		"sub_quest_id", verdict.SubQuestID, "node_id", nodeID,
		"execution_id", dagState.ExecutionID)

	switch verdict.Verdict {
	case "accept":
		quest.Status = domain.QuestCompleted
		quest.CompletedAt = timePtr(time.Now())
		if emitErr := c.graph.EmitEntityUpdate(ctx, quest, "quest.dag.node_accepted"); emitErr != nil {
			c.logger.Error("failed to complete sub-quest after review accept",
				"sub_quest_id", verdict.SubQuestID, "error", emitErr)
			c.errorsCount.Add(1)
			return
		}
		c.logger.Info("sub-quest accepted by lead review",
			"sub_quest_id", verdict.SubQuestID, "node_id", nodeID,
			"execution_id", dagState.ExecutionID)
		c.createAcceptReviewEntity(ctx, dagState, nodeID)

	case "reject":
		quest.Status = domain.QuestFailed
		quest.FailureReason = "Rejected by party lead during review"
		if emitErr := c.graph.EmitEntityUpdate(ctx, quest, "quest.dag.node_rejected"); emitErr != nil {
			c.logger.Error("failed to fail sub-quest after review reject",
				"sub_quest_id", verdict.SubQuestID, "error", emitErr)
			c.errorsCount.Add(1)
			return
		}
		c.logger.Info("sub-quest rejected by lead review",
			"sub_quest_id", verdict.SubQuestID, "node_id", nodeID,
			"execution_id", dagState.ExecutionID)
	}
}

// =============================================================================
// HANDLER: dagEventSynthesisCompleted
// =============================================================================

// onSynthesisCompleted handles a synthesis loop completion from the AGENT stream.
// The lead has combined all sub-quest outputs into a final deliverable. We use
// that as the rollup result.
func (c *Component) onSynthesisCompleted(ctx context.Context, evt dagEvent) {
	c.logger.Info("event loop: processing synthesis completion",
		"loop_id", evt.LoopID, "result_length", len(evt.Result))

	// Extract execution ID from LoopID: "synthesis-{parentQuestEntityKey}-{nuid}"
	executionID := c.extractExecutionIDFromSynthesisLoop(evt.LoopID)
	if executionID == "" {
		c.logger.Warn("synthesis completion: cannot find DAG for loop",
			"loop_id", evt.LoopID)
		return
	}

	dagState, ok := c.dagCache[executionID]
	if !ok {
		c.logger.Warn("synthesis completion: DAG not in cache",
			"loop_id", evt.LoopID, "execution_id", executionID)
		return
	}

	c.triggerRollupWithResult(ctx, dagState, evt.Result)
}

// extractExecutionIDFromSynthesisLoop finds the DAG execution ID by scanning
// dagCache for a DAG that is complete (all nodes in CompletedNodes). The
// synthesis loop is dispatched only for complete DAGs.
func (c *Component) extractExecutionIDFromSynthesisLoop(_ string) string {
	// There should be exactly one complete DAG waiting for synthesis.
	for execID, ds := range c.dagCache {
		if c.isDAGComplete(ds) {
			return execID
		}
	}
	return ""
}

// =============================================================================
// STATE MACHINE (called only from event loop goroutine)
// =============================================================================

// handleSubQuestTransition reacts to a status change on a sub-quest that
// belongs to an active DAG. It updates NodeStates, dispatches work, and
// triggers rollup or escalation as appropriate.
//
// Called only from onSubQuestTransition — runs in the event loop goroutine.
// No mutex needed.
func (c *Component) handleSubQuestTransition(
	ctx context.Context,
	dagState *DAGExecutionState,
	nodeID string,
	oldStatus, newStatus domain.QuestStatus,
	entity *graph.EntityState,
) {
	switch newStatus {
	case domain.QuestInProgress:
		dagState.NodeStates[nodeID] = NodeInProgress
		c.logger.Debug("DAG node transitioned to in_progress",
			"execution_id", dagState.ExecutionID, "node_id", nodeID)

	case domain.QuestInReview:
		dagState.NodeStates[nodeID] = NodePendingReview
		c.dispatchLeadReview(ctx, dagState, nodeID, entity)

	case domain.QuestCompleted:
		c.handleNodeCompleted(ctx, dagState, nodeID)

	case domain.QuestFailed:
		c.handleNodeFailed(ctx, dagState, nodeID)

	case domain.QuestEscalated:
		// Party sub-quest clarification: the member asked a question via the
		// escalation path. Non-party escalations never reach here because only
		// sub-quests tracked in dagBySubQuest trigger this handler.
		dagState.NodeStates[nodeID] = NodeAwaitingClarification
		c.dispatchLeadClarification(ctx, dagState, nodeID, entity)

	default:
		c.logger.Debug("ignoring sub-quest status transition",
			"execution_id", dagState.ExecutionID, "node_id", nodeID,
			"old_status", oldStatus, "new_status", newStatus)
		return
	}

	// Persist mutated DAG state to the parent quest entity.
	if err := c.persistDAGState(ctx, dagState); err != nil {
		c.logger.Error("failed to persist DAG state after transition",
			"execution_id", dagState.ExecutionID, "node_id", nodeID, "error", err)
		c.errorsCount.Add(1)
	}
}
