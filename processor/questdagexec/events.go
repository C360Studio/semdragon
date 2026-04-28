package questdagexec

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/nats-io/nats.go/jetstream"
)

// =============================================================================
// EVENT TYPES
// =============================================================================

// dagEventType identifies the kind of DAG lifecycle event flowing through
// the unified event channel into the single-goroutine event loop.
type dagEventType int

const (
	// dagEventNewDAG is emitted when a quest entity with quest.dag.execution_id
	// is seen for the first time — the parent quest has no entry in dagCache yet.
	dagEventNewDAG dagEventType = iota

	// dagEventDAGUpdated exists for the event loop dispatch case arm but is
	// never sent by producers — they always send dagEventNewDAG and onDAGEntry
	// reclassifies by checking dagCache. Retained for explicitness in the
	// switch statement and potential future use by external producers.
	dagEventDAGUpdated

	// dagEventSubQuestTransition is emitted when a sub-quest entity changes
	// status. The event loop drives the DAG state machine based on the transition.
	// During the bootstrap replay phase, Bootstrapping is true — the event loop
	// only updates questCache without triggering DAG actions.
	dagEventSubQuestTransition

	// dagEventReviewCompleted is emitted when the AGENT stream delivers a
	// LoopCompletedEvent for a review loop (LoopID prefixed with "review-").
	// The event loop applies the accept/reject verdict to the DAG node.
	dagEventReviewCompleted

	// dagEventClarificationAnswered is emitted when the AGENT stream delivers a
	// LoopCompletedEvent for a clarification loop (LoopID prefixed with "clarify-").
	// The event loop stores the answer in DAGExecutionState and resets the node
	// to NodeAssigned so questbridge re-dispatches the member's sub-quest.
	dagEventClarificationAnswered

	// dagEventSynthesisCompleted is emitted when the AGENT stream delivers a
	// LoopCompletedEvent for a synthesis loop (LoopID prefixed with "synthesis-").
	// The lead has combined all sub-quest outputs into a final deliverable.
	// The event loop calls triggerRollup with the synthesized result.
	dagEventSynthesisCompleted

	// dagEventReviewFailed is emitted when the AGENT stream delivers a
	// LoopFailedEvent or LoopCancelledEvent for a review loop (LoopID prefixed with "review-").
	dagEventReviewFailed

	// dagEventSynthesisFailed is emitted when the AGENT stream delivers a
	// LoopFailedEvent or LoopCancelledEvent for a synthesis loop (LoopID prefixed with "synthesis-").
	dagEventSynthesisFailed

	// dagEventClarificationFailed is emitted when the AGENT stream delivers a
	// LoopFailedEvent or LoopCancelledEvent for a clarification loop (LoopID prefixed with "clarify-").
	dagEventClarificationFailed

	// dagEventDAGTimedOut is emitted by the sweep goroutine when a DAG exceeds
	// its configured timeout. The event loop escalates the parent and cleans up.
	dagEventDAGTimedOut
)

// dagEvent carries all data the event loop needs to process one DAG lifecycle
// event. Fields are populated differently per event type; unused fields are
// always zero-valued.
//
// For dagEventNewDAG and dagEventDAGUpdated:
//   - Triples carries the full parent quest entity triples for DAG reconstruction
//   - EntityKey is the parent quest entity KV key
//   - DAGState may be non-nil if already reconstructed (e.g. from a prior pass)
//
// For dagEventSubQuestTransition:
//   - EntityKey is the KV key of the sub-quest entity
//   - NewStatus is the current quest.status.state value
//   - Triples carries the full entity state for dispatchLeadReview
//   - Bootstrapping is true during the historical replay phase; the event loop
//     only updates questCache and does not trigger DAG actions for these events
//
// For dagEventReviewCompleted and dagEventClarificationAnswered:
//   - LoopID is the lead loop ID from LoopCompletedEvent
//   - Result is the raw result string (JSON envelope or free-form text)
type dagEvent struct {
	Type dagEventType

	// DAG event fields (NewDAG, DAGUpdated).
	DAGState   *DAGExecutionState
	KVRevision uint64

	// Sub-quest transition fields.
	EntityKey     string
	NewStatus     string
	Triples       []message.Triple
	Bootstrapping bool // true during historical KV replay — suppress DAG actions

	// Review completion fields.
	LoopID string
	Result string

	// Error reason for failure events.
	ErrorReason string
}

// =============================================================================
// PRODUCER: QUEST ENTITY KV WATCHER
// =============================================================================

// produceQuestEvents watches the quest entity KV bucket for two kinds of events:
//
//  1. Parent quest DAG initialization: quests with quest.dag.execution_id set
//     that are not yet in seenDAGParents emit dagEventNewDAG so the event loop
//     can reconstruct DAGExecutionState from the entity's triples.
//
//  2. Sub-quest status transitions: quests with quest.status.state set emit
//     dagEventSubQuestTransition so the event loop can drive the DAG state machine.
//
// During bootstrap (before the nil sentinel) both event types are emitted with
// Bootstrapping=true so the event loop primes its caches without triggering
// assignment or persistence side-effects.
//
// Key design rule: this goroutine does NOT read or write questCache, dagCache, or
// dagBySubQuest directly. Those maps are exclusively owned by the event loop.
// The producer only reads from NATS and sends events.
func (c *Component) produceQuestEvents(ctx context.Context, events chan<- dagEvent) {
	defer c.wg.Done()

	watcher, err := c.graph.WatchEntityType(ctx, domain.EntityTypeQuest)
	if err != nil {
		c.logger.Error("quest producer: failed to start quest watcher", "error", err)
		c.errorsCount.Add(1)
		return
	}
	defer watcher.Stop()

	bootstrapping := true

	// seenExecutionIDs tracks the last-seen DAG execution ID per parent quest
	// entity key. This prevents re-emitting dagEventNewDAG on every persist-back
	// (same execution ID = suppress) while allowing retry DAGs after boss battle
	// defeat (new execution ID = new DAG, emit).
	seenExecutionIDs := make(map[string]string)

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

			// Nil sentinel marks end of historical replay.
			if entry == nil {
				bootstrapping = false
				continue
			}

			entityState, decodeErr := semdragons.DecodeEntityState(entry)
			if decodeErr != nil || entityState == nil {
				continue
			}

			entityKey := entry.Key()

			// Check if this is a parent quest with DAG execution state.
			// A new execution ID means a new DAG (retry after boss battle defeat).
			// Same execution ID means a persist-back from questdagexec (suppress).
			executionID := tripleString(entityState.Triples, "quest.dag.execution_id")
			if executionID != "" && executionID != seenExecutionIDs[entityKey] {
				seenExecutionIDs[entityKey] = executionID

				evt := dagEvent{
					Type:          dagEventNewDAG,
					EntityKey:     entityKey,
					Triples:       entityState.Triples,
					Bootstrapping: bootstrapping,
				}

				select {
				case events <- evt:
				case <-c.stopChan:
					return
				case <-ctx.Done():
					return
				}
			}

			// Check for sub-quest status transitions.
			newStatus := tripleString(entityState.Triples, "quest.status.state")
			if newStatus == "" {
				continue
			}

			evt := dagEvent{
				Type:          dagEventSubQuestTransition,
				EntityKey:     entityKey,
				NewStatus:     newStatus,
				Triples:       entityState.Triples,
				Bootstrapping: bootstrapping,
			}

			select {
			case events <- evt:
			case <-c.stopChan:
				return
			case <-ctx.Done():
				return
			}
		}
	}
}

// =============================================================================
// PRODUCER: AGENT STREAM REVIEW CONSUMER
// =============================================================================

// produceReviewEvents subscribes to agent.complete.* on the AGENT JetStream
// stream and emits dagEventReviewCompleted for LoopCompletedEvents with LoopID
// prefixed "review-" and dagEventClarificationAnswered for those prefixed
// "clarify-". All other completions are acked and discarded.
//
// Producer constraint: must NOT access dagCache, dagBySubQuest, or questCache.
// It only reads from NATS and sends events.
func (c *Component) produceReviewEvents(ctx context.Context, events chan<- dagEvent) {
	defer c.wg.Done()

	js, err := c.deps.NATSClient.JetStream()
	if err != nil {
		c.logger.Error("review producer: failed to get JetStream", "error", err)
		c.errorsCount.Add(1)
		return
	}

	stream, err := js.Stream(ctx, c.config.StreamName)
	if err != nil {
		c.logger.Error("review producer: failed to get AGENT stream", "error", err)
		c.errorsCount.Add(1)
		return
	}

	consumerName := fmt.Sprintf("questdagexec-review-%s-%s-%s", c.config.Org, c.config.Platform, c.config.Board)
	consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		FilterSubjects: []string{"agent.complete.*", "agent.failed.*"},
		DeliverPolicy:  jetstream.DeliverNewPolicy,
		AckPolicy:      jetstream.AckExplicitPolicy,
		Name:           consumerName,
	})
	if err != nil {
		c.logger.Error("review producer: failed to create consumer", "error", err)
		c.errorsCount.Add(1)
		return
	}

	for {
		select {
		case <-c.stopChan:
			return
		case <-ctx.Done():
			return
		default:
		}

		msgs, fetchErr := consumer.Fetch(1, jetstream.FetchMaxWait(2*time.Second))
		if fetchErr != nil {
			// Backoff before retry to avoid hot-looping on persistent NATS errors.
			select {
			case <-time.After(500 * time.Millisecond):
			case <-c.stopChan:
				return
			case <-ctx.Done():
				return
			}
			continue
		}

		for msg := range msgs.Messages() {
			evt, ok := parseLeadLoopCompletion(c, msg)
			if !ok {
				// Not a lead loop completion or parse failure — ack and skip.
				_ = msg.Ack()
				continue
			}

			select {
			case events <- evt:
				_ = msg.Ack()
			case <-c.stopChan:
				_ = msg.Nak()
				return
			case <-ctx.Done():
				_ = msg.Nak()
				return
			}
		}
	}
}

// parseLeadLoopCompletion unmarshals a JetStream message and extracts the
// lead loop event fields. It handles LoopCompletedEvent (success), LoopFailedEvent
// (LLM timeout/error), and LoopCancelledEvent (user/admin cancel), classifying
// each by LoopID prefix:
//   - "review-"    → dagEventReviewCompleted / dagEventReviewFailed
//   - "clarify-"   → dagEventClarificationAnswered / dagEventClarificationFailed
//   - "synthesis-" → dagEventSynthesisCompleted / dagEventSynthesisFailed
//
// Returns the dagEvent and true on success, or zero value and false when the
// message is not a lead loop event or cannot be parsed.
//
// Takes *Component only for the logger — must not access any state maps.
// State maps are owned exclusively by the event loop goroutine.
func parseLeadLoopCompletion(c *Component, msg jetstream.Msg) (dagEvent, bool) {
	baseMsg, err := c.decoder.Decode(msg.Data())
	if err != nil {
		c.logger.Debug("lead loop producer: ignoring non-BaseMessage", "error", err)
		return dagEvent{}, false
	}

	// Try LoopCompletedEvent first (success path).
	if evt, ok := baseMsg.Payload().(*agentic.LoopCompletedEvent); ok {
		de := classifyCompletedEvent(evt)
		if de.LoopID != "" {
			return de, true
		}
		return dagEvent{}, false
	}

	// Try LoopFailedEvent (timeout or LLM error).
	if evt, ok := baseMsg.Payload().(*agentic.LoopFailedEvent); ok {
		de := classifyFailedEvent(evt)
		if de.LoopID != "" {
			return de, true
		}
		return dagEvent{}, false
	}

	// Try LoopCancelledEvent (user/admin cancel signal).
	if evt, ok := baseMsg.Payload().(*agentic.LoopCancelledEvent); ok {
		de := classifyCancelledEvent(evt)
		if de.LoopID != "" {
			return de, true
		}
		return dagEvent{}, false
	}

	// Try re-marshaling if the payload registry returned a different concrete type.
	payloadBytes, marshalErr := json.Marshal(baseMsg.Payload())
	if marshalErr != nil {
		return dagEvent{}, false
	}

	// Try completed.
	var completed agentic.LoopCompletedEvent
	if json.Unmarshal(payloadBytes, &completed) == nil && completed.LoopID != "" {
		de := classifyCompletedEvent(&completed)
		if de.LoopID != "" {
			return de, true
		}
		return dagEvent{}, false
	}

	// Try failed.
	var failed agentic.LoopFailedEvent
	if json.Unmarshal(payloadBytes, &failed) == nil && failed.LoopID != "" {
		de := classifyFailedEvent(&failed)
		if de.LoopID != "" {
			return de, true
		}
		return dagEvent{}, false
	}

	// Try cancelled.
	var cancelled agentic.LoopCancelledEvent
	if json.Unmarshal(payloadBytes, &cancelled) == nil && cancelled.LoopID != "" {
		de := classifyCancelledEvent(&cancelled)
		if de.LoopID != "" {
			return de, true
		}
		return dagEvent{}, false
	}

	return dagEvent{}, false
}

// classifyCompletedEvent maps a LoopCompletedEvent to the appropriate dagEvent
// type based on LoopID prefix. Returns zero value for non-lead loops.
func classifyCompletedEvent(evt *agentic.LoopCompletedEvent) dagEvent {
	if strings.HasPrefix(evt.LoopID, "review-") {
		return dagEvent{Type: dagEventReviewCompleted, LoopID: evt.LoopID, Result: evt.Result}
	}
	if strings.HasPrefix(evt.LoopID, "clarify-") {
		return dagEvent{Type: dagEventClarificationAnswered, LoopID: evt.LoopID, Result: evt.Result}
	}
	if strings.HasPrefix(evt.LoopID, "synthesis-") {
		return dagEvent{Type: dagEventSynthesisCompleted, LoopID: evt.LoopID, Result: evt.Result}
	}
	return dagEvent{}
}

// classifyFailedEvent maps a LoopFailedEvent to the appropriate failure dagEvent
// type based on LoopID prefix. Returns zero value for non-lead loops.
func classifyFailedEvent(evt *agentic.LoopFailedEvent) dagEvent {
	reason := evt.Error
	if reason == "" {
		reason = evt.Reason
	}
	if strings.HasPrefix(evt.LoopID, "review-") {
		return dagEvent{Type: dagEventReviewFailed, LoopID: evt.LoopID, ErrorReason: reason}
	}
	if strings.HasPrefix(evt.LoopID, "clarify-") {
		return dagEvent{Type: dagEventClarificationFailed, LoopID: evt.LoopID, ErrorReason: reason}
	}
	if strings.HasPrefix(evt.LoopID, "synthesis-") {
		return dagEvent{Type: dagEventSynthesisFailed, LoopID: evt.LoopID, ErrorReason: reason}
	}
	return dagEvent{}
}

// classifyCancelledEvent maps a LoopCancelledEvent to the appropriate failure dagEvent
// type based on LoopID prefix. Returns zero value for non-lead loops.
func classifyCancelledEvent(evt *agentic.LoopCancelledEvent) dagEvent {
	reason := "cancelled"
	if evt.CancelledBy != "" {
		reason = "cancelled: " + evt.CancelledBy
	}
	if strings.HasPrefix(evt.LoopID, "review-") {
		return dagEvent{Type: dagEventReviewFailed, LoopID: evt.LoopID, ErrorReason: reason}
	}
	if strings.HasPrefix(evt.LoopID, "clarify-") {
		return dagEvent{Type: dagEventClarificationFailed, LoopID: evt.LoopID, ErrorReason: reason}
	}
	if strings.HasPrefix(evt.LoopID, "synthesis-") {
		return dagEvent{Type: dagEventSynthesisFailed, LoopID: evt.LoopID, ErrorReason: reason}
	}
	return dagEvent{}
}
