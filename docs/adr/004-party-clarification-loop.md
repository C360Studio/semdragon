# ADR-004: Party Clarification Loop

## Status

Proposed

## Context

When a sub-quest agent needs clarification during party quest DAG execution, the current system escalates to the Dungeon Master via `QuestEscalated` status. This is wrong for party sub-quests because:

1. The DM has no context about sub-quest details -- the party lead decomposed the quest and knows what was intended.
2. Escalation to the DM stalls the DAG. The DM session is designed for top-level quests, not sub-quest minutiae.
3. The party is a closed system by design (ADR-002). Sub-quests are invisible to the public board and should be invisible to the DM's escalation workflow too.

The lead already interacts with the AGENT stream for `review_sub_quest`. This ADR adds a parallel interaction for answering clarification questions.

## Decision

Route party sub-quest clarifications to the party lead via a new `answer_clarification` tool, using the same AGENT stream dispatch pattern as lead review. No new streams, buckets, or quest statuses are introduced.

### 1. New Node State: `NodeAwaitingClarification`

Add one new constant to the DAG node state machine:

```go
// NodeAwaitingClarification means the party member asked a clarifying question.
// The lead receives the question via the AGENT stream and provides an answer.
// The node transitions back to NodeAssigned after the lead responds.
NodeAwaitingClarification = "awaiting_clarification"
```

State machine additions:

```
NodeInProgress --[clarification detected]--> NodeAwaitingClarification
NodeAwaitingClarification --[lead answers]--> NodeAssigned --[re-dispatch]--> NodeInProgress
```

This sits alongside the existing review path:

```
NodeInProgress --[work product]--> NodePendingReview --[accept]--> NodeCompleted
                                                     --[reject]--> NodeAssigned (retry)
```

The new state is observable in QUEST_DAGS KV like all other node states.

### 2. New Tool: `answer_clarification`

Modeled after `review_sub_quest` but simpler. The lead receives the member's question and provides a free-text answer.

```go
const clarificationToolName = "answer_clarification"
```

Tool definition:

```json
{
  "name": "answer_clarification",
  "description": "Answer a party member's clarification question about their sub-quest. Provide a clear answer that resolves their question so they can continue working.",
  "parameters": {
    "type": "object",
    "required": ["sub_quest_id", "answer"],
    "properties": {
      "sub_quest_id": {
        "type": "string",
        "description": "The ID of the sub-quest the member is asking about"
      },
      "answer": {
        "type": "string",
        "description": "Your answer to the member's question. Be specific and actionable."
      }
    }
  }
}
```

Implementation: `ClarificationExecutor` in `processor/questdagexec/clarification_tool.go`. Validates that `sub_quest_id` and `answer` are non-empty strings. Returns a JSON envelope `{"sub_quest_id": "...", "answer": "..."}` that `onClarificationAnswered` parses.

### 3. Questbridge Change: Route Party Clarifications

In `questbridge/handler.go`, the `completeQuest` method already has the clarification detection at line 587. The change is a single conditional branch:

```go
if isOutputClarificationRequest(output) {
    // NEW: If this is a party sub-quest, route to lead instead of DM.
    if quest.PartyID != nil {
        quest.Status = domain.QuestEscalated
        quest.Escalated = true
        quest.Output = output  // Preserve the question text for the lead
        quest.FailureReason = output
        if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.dag.clarification_requested"); err != nil {
            // ... error handling
        }
        c.logger.Info("party sub-quest clarification routed to lead",
            "quest_id", questID, "party_id", quest.PartyID, "agent_id", mapping.AgentID)
        return
    }
    // Existing DM escalation path for non-party quests...
}
```

Key points:
- The quest status is still `QuestEscalated`. No new quest status is needed. The DAG state machine uses `NodeAwaitingClarification` as its own internal representation.
- The predicate changes from `quest.escalated` to `quest.dag.clarification_requested` for party sub-quests. This makes the two paths distinguishable in the trajectory without a new status enum.
- `quest.Output` is set to the clarification text so the lead can read the question.

### 4. Questdagexec: Handle Escalated Sub-Quests

In `handleSubQuestTransition`, add a case for `QuestEscalated`:

```go
case domain.QuestEscalated:
    // Only act on party sub-quest clarifications. Non-party escalations
    // should never reach here (they have no DAG), but guard defensively.
    dagState.NodeStates[nodeID] = NodeAwaitingClarification
    c.dispatchLeadClarification(ctx, dagState, nodeID, entity)
```

### 5. Lead Clarification Dispatch

New method `dispatchLeadClarification` in `handler.go`, parallel to `dispatchLeadReview`:

```go
func (c *Component) dispatchLeadClarification(
    ctx context.Context,
    dagState *DAGExecutionState,
    nodeID string,
    entity *graph.EntityState,
) {
    subQuestID := dagState.NodeQuestIDs[nodeID]
    leadAgentID := c.findLeadAgentID(dagState)
    memberID := dagState.NodeAssignees[nodeID]

    nodeObjective := c.findNodeObjective(dagState, nodeID)
    clarificationQuestion := c.extractQuestOutput(entity)

    systemPrompt := buildLeadClarificationPrompt(
        nodeObjective, memberID, clarificationQuestion,
    )
    userPrompt := fmt.Sprintf(
        "A party member working on sub-quest %q needs clarification.\n\n"+
            "IMPORTANT: You MUST call the answer_clarification tool. "+
            "Do NOT respond with text -- use the tool.",
        subQuestID,
    )

    subjectSafeID := strings.ReplaceAll(subQuestID, ".", "-")
    loopID := fmt.Sprintf("clarify-%s-%s", subjectSafeID, nuid.Next())

    clarifyTools := NewClarificationExecutor().ListTools()

    taskMsg := agentic.TaskMessage{
        TaskID: subQuestID,
        LoopID: loopID,
        // ... same structure as dispatchLeadReview
        Tools: clarifyTools,
        Metadata: map[string]any{
            "parent_quest_id":        dagState.ParentQuestID,
            "sub_quest_id":           subQuestID,
            "node_id":                nodeID,
            "execution_id":           dagState.ExecutionID,
            "party_id":               dagState.PartyID,
            "lead_agent_id":          leadAgentID,
            "clarification_dispatch": true,
            "agent_id":               leadAgentID,
            "trust_tier":             float64(domain.TierMaster),
            "quest_id":               subQuestID,
        },
    }

    // Marshal as BaseMessage, publish to AGENT stream...
}
```

The LoopID prefix is `clarify-` (not `review-`). This is how the producer distinguishes the two loop types.

### 6. Clarification Completion Handling

Extend `produceReviewEvents` to also forward `clarify-` prefixed LoopCompletedEvents. Use a new event type `dagEventClarificationAnswered` so the event loop dispatches to the right handler.

Add a new event type:

```go
dagEventClarificationAnswered dagEventType = iota  // after dagEventReviewCompleted
```

Extend `parseReviewCompletion` (rename to `parseLeadLoopCompletion`):

```go
if strings.HasPrefix(evt.LoopID, "review-") {
    return dagEvent{Type: dagEventReviewCompleted, LoopID: evt.LoopID, Result: evt.Result}, true
}
if strings.HasPrefix(evt.LoopID, "clarify-") {
    return dagEvent{Type: dagEventClarificationAnswered, LoopID: evt.LoopID, Result: evt.Result}, true
}
return dagEvent{}, false
```

Add handler dispatch in `handleEvent`:

```go
case dagEventClarificationAnswered:
    c.onClarificationAnswered(ctx, evt)
```

### 7. Answer Injection and Sub-Quest Retry

`onClarificationAnswered` parses the answer from the LoopCompletedEvent, locates the DAG and node, then:

1. Stores the clarification answer in `DAGExecutionState.NodeClarifications`.
2. Transitions the node back to `NodeAssigned`.
3. Resets the sub-quest status to `in_progress` via `graph.EmitEntityUpdate` with predicate `quest.dag.clarification_answered` so questbridge re-dispatches the agentic loop.

```go
func (c *Component) onClarificationAnswered(ctx context.Context, evt dagEvent) {
    // Parse answer JSON from the answer_clarification tool output.
    var answer struct {
        SubQuestID string `json:"sub_quest_id"`
        Answer     string `json:"answer"`
    }
    // ... parse with fallback (same pattern as onReviewCompleted)

    // Locate DAG and node.
    entityKey := c.subQuestEntityKey(answer.SubQuestID)
    dagState := c.findDAGForSubQuest(entityKey)
    nodeID := c.findNodeForQuest(dagState, answer.SubQuestID)

    // Store clarification exchange.
    question := c.extractQuestOutput(/* load entity */)
    if dagState.NodeClarifications == nil {
        dagState.NodeClarifications = make(map[string][]ClarificationExchange)
    }
    dagState.NodeClarifications[nodeID] = append(
        dagState.NodeClarifications[nodeID],
        ClarificationExchange{Question: question, Answer: answer.Answer},
    )

    // Reset node to assigned for re-dispatch.
    dagState.NodeStates[nodeID] = NodeAssigned

    // Transition quest back to in_progress so questbridge picks it up.
    quest.Status = domain.QuestInProgress
    quest.Escalated = false
    c.graph.EmitEntityUpdate(ctx, quest, "quest.dag.clarification_retry")

    // Persist DAG state.
    c.persistDAGState(ctx, dagState)
}
```

The clarification exchange is injected into the agent's prompt context on the retry. Questbridge already loads peer feedback into `AssemblyContext` for rejected sub-quests. The same mechanism carries clarification answers:

```go
// In promptmanager/types.go -- AssemblyContext already has PeerFeedback
type AssemblyContext struct {
    // ... existing fields
    ClarificationAnswers []ClarificationExchange `json:"clarification_answers,omitempty"`
}
```

The prompt assembler appends a section like:

```
## Previous Clarifications
You previously asked: "What format should the output be in?"
Lead answered: "Use JSON with keys: name, score, summary."
```

Questbridge populates this field by reading `NodeClarifications` from the QUEST_DAGS KV when building the dispatch context for a sub-quest that has prior clarifications.

### 8. DAGExecutionState Changes

Add two items:

```go
// ClarificationExchange records one Q&A between a member and the party lead.
type ClarificationExchange struct {
    Question string    `json:"question"`
    Answer   string    `json:"answer"`
    AskedAt  time.Time `json:"asked_at"`
}

type DAGExecutionState struct {
    // ... existing fields

    // NodeClarifications tracks clarification Q&A exchanges per node.
    // Appended when the lead answers a clarification; injected into the
    // member's prompt on the next dispatch.
    NodeClarifications map[string][]ClarificationExchange `json:"node_clarifications,omitempty"`
}
```

This persists in QUEST_DAGS KV alongside all other DAG state, making clarification history fully observable.

### 9. EventType Annotations for Observability

| Event | Predicate | When |
|-------|-----------|------|
| Member asks clarification | `quest.dag.clarification_requested` | questbridge detects clarification intent on party sub-quest |
| Lead dispatched | (TaskMessage to AGENT stream with `clarification_dispatch: true` metadata) | questdagexec dispatches lead clarification loop |
| Lead answers | `quest.dag.clarification_answered` | questdagexec processes lead's answer and stores it |
| Member retries | `quest.dag.clarification_retry` | questdagexec resets sub-quest to in_progress with answer injected |

All events are visible in the QUEST_DAGS KV (via node state transitions) and in quest entity trajectories (via predicate annotations on `EmitEntityUpdate`).

### 10. LoopID Extraction

Generalize `extractSubQuestFromLoopID` to handle both prefixes. The current implementation strips `review-` and then extracts the entity ID from dashes. The same logic works for `clarify-`:

```go
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
    // ... rest of existing extraction logic
}
```

## Implementation Plan

### Phase 1: Core Types and Tool (no integration)

Files to create:
- `processor/questdagexec/clarification_tool.go` -- `ClarificationExecutor` + tool definition
- `processor/questdagexec/clarification_tool_test.go` -- unit tests for tool validation

Files to modify:
- `processor/questdagexec/types.go` -- add `NodeAwaitingClarification`, `ClarificationExchange`, `NodeClarifications` field

### Phase 2: Event Loop Integration

Files to modify:
- `processor/questdagexec/events.go` -- add `dagEventClarificationAnswered`, extend `parseReviewCompletion` to match `clarify-` prefix
- `processor/questdagexec/eventloop.go` -- add `QuestEscalated` case in `handleSubQuestTransition`, add `dagEventClarificationAnswered` case in `handleEvent`
- `processor/questdagexec/handler.go` -- add `dispatchLeadClarification`, `onClarificationAnswered`, `buildLeadClarificationPrompt`, generalize `extractSubQuestFromLoopID`

### Phase 3: Questbridge Routing

Files to modify:
- `processor/questbridge/handler.go` -- add `quest.PartyID != nil` check in clarification detection, use `quest.dag.clarification_requested` predicate

### Phase 4: Prompt Injection

Files to modify:
- `processor/promptmanager/types.go` -- add `ClarificationAnswers` to `AssemblyContext`
- `processor/promptmanager/assembler.go` -- render clarification answers section in prompt
- `processor/questbridge/handler.go` -- load `NodeClarifications` from QUEST_DAGS when building dispatch context for retried sub-quests

### Phase 5: Questtools Registration

Files to modify:
- `processor/questdagexec/component.go` -- ensure `answer_clarification` tool is registered in questtools alongside `review_sub_quest`

## Consequences

### Positive
- Clarifications stay within the party's closed system. The lead has full context about what was intended.
- No new NATS streams, KV buckets, or quest statuses. Reuses existing infrastructure exactly.
- Clarification history is persisted and observable via QUEST_DAGS KV.
- Previous Q&A is injected into retry prompts, reducing repeated questions.
- The pattern is structurally identical to lead review, so maintenance cost is low.

### Negative
- Adds a third tool to the lead's toolbox (`decompose_quest`, `review_sub_quest`, `answer_clarification`). Prompt engineering must ensure the LLM calls the right tool.
- The `ClarificationExchange` history grows the QUEST_DAGS KV entry size. In practice, sub-quests rarely need more than 1-2 clarifications before the node either completes or exhausts retries, so this is bounded.
- `QuestEscalated` status is overloaded: it means "DM escalation" for non-party quests and "clarification requested" for party sub-quests. The predicate annotation (`quest.dag.clarification_requested` vs `quest.lifecycle.escalated`) disambiguates at the event level, but code must check `PartyID` to determine the routing path.

### Mitigations
- The clarification and review tools are never presented together. The lead receives either a review task or a clarification task, never both simultaneously. This eliminates tool selection confusion.
- The `QuestEscalated` overload is contained: questdagexec only watches sub-quests that are in its `dagBySubQuest` index, so non-party escalations never trigger the clarification path.

### Alternatives Considered

**New quest status (`QuestClarificationRequested`)**. Rejected because adding a new status requires changes across questboard, bossbattle, agentprogression, autonomy, the API, and the UI. The DAG state machine already tracks node-level states independently from quest status, and the predicate annotation provides sufficient observability.

**Direct NATS request/reply from member to lead**. Rejected because it bypasses the event-driven architecture. The agentic-loop processes TaskMessages from the AGENT stream -- using request/reply would require a synchronous call path that does not exist and would be hard to observe.

**Store clarification in quest entity triples instead of DAGExecutionState**. Rejected because the quest entity is the wrong granularity. Multiple clarifications per node across multiple nodes would clutter the quest entity. DAG state is the natural home for per-node lifecycle data.

## Sequence Diagram

```
Member Agent          questbridge         questdagexec          Lead Agent
     |                     |                    |                    |
     |--[loop completes]-->|                    |                    |
     |  output: question   |                    |                    |
     |                     |--[detect clarif.]->|                    |
     |                     |  quest.escalated   |                    |
     |                     |  (party sub-quest) |                    |
     |                     |                    |                    |
     |                     |                    |--[NodeAwaitingClarification]
     |                     |                    |                    |
     |                     |                    |--[TaskMessage]---->|
     |                     |                    |  clarify-{id}      |
     |                     |                    |  tool: answer_     |
     |                     |                    |  clarification     |
     |                     |                    |                    |
     |                     |                    |<--[LoopCompleted]--|
     |                     |                    |  answer JSON       |
     |                     |                    |                    |
     |                     |                    |--[store answer in DAG state]
     |                     |                    |--[NodeAssigned]
     |                     |                    |--[quest -> in_progress]
     |                     |                    |                    |
     |                     |<-[KV watch: in_progress]               |
     |                     |                    |                    |
     |<--[TaskMessage]-----|                    |                    |
     |  with clarification |                    |                    |
     |  answer in context  |                    |                    |
```
