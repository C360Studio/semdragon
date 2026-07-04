package questlifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	graphingest "github.com/c360studio/semstreams/processor/graph-ingest"
)

// Command trigger values stamped on quest.command to request a lifecycle
// transition. A caller writes one of these (plus any argument triples the
// matching rule reads) as a graph-ingest triple delta; the rule in
// config/rules/quest/*.json fires the lifecycle_transition and clears the
// command to CommandConsumed in the same atomic write, so it cannot re-fire when
// the quest later re-enters a matching phase (e.g. fail → posted retry).
const (
	// CommandClaim requests posted → claimed. Reads PredClaimRequestedBy.
	CommandClaim = "claim"
	// CommandStart requests claimed → in_progress.
	CommandStart = "start"
	// CommandSubmit requests in_progress → in_review or completed, routed by
	// the quest.review.needs_review predicate (written at post time).
	CommandSubmit = "submit"
	// CommandFail requests in_progress → posted (retry) or pending_triage,
	// routed by the quest.attempts.current predicate.
	CommandFail = "fail"

	// CommandConsumed is the sentinel a rule writes after firing so the command
	// cannot re-fire. Callers never write it; it is documented here so tests and
	// readers can assert the post-fire state.
	CommandConsumed = "consumed"
)

// Argument predicates a rule reads off the entity when it fires. Callers stamp
// these before (or with) the command so the value is present at fire time.
const (
	// PredClaimRequestedBy carries the agent id the claim rule copies into
	// quest.assignment.agent (claimed_by).
	PredClaimRequestedBy = "quest.claim.requested_by"
	// PredDataOutput carries the submitted work product the submit rules leave
	// on the entity as quest.data.output.
	PredDataOutput = "quest.data.output"
)

// commandStampTimeout bounds each graph-ingest request/reply. The stamp is a
// single KV mutation; anything slower than this is a stalled ingest, not a slow
// write, and should surface as an error rather than hang the caller.
const commandStampTimeout = 5 * time.Second

// StampTriple writes a single (questID, predicate, object) into ENTITY_STATES as
// an additive graph-ingest delta. Single-valued lifecycle predicates are
// replace-on-write, so a repeated stamp overwrites rather than accumulates.
//
// Use this instead of GraphClient.EmitEntity/EmitEntityUpdate for any field on a
// lifecycle-managed quest: those do a full-object store.Put that replaces the
// whole entity value and would clobber the lifecycle Manager's phase, audit, and
// command triples (they share the same ENTITY_STATES entity).
func StampTriple(ctx context.Context, client *natsclient.Client, questID, predicate string, object any) error {
	body, err := json.Marshal(graph.AddTripleRequest{
		Triple: message.Triple{Subject: questID, Predicate: predicate, Object: object, Confidence: 1.0},
	})
	if err != nil {
		return fmt.Errorf("questlifecycle: marshal triple %s: %w", predicate, err)
	}
	if _, err := client.Request(ctx, graphingest.SubjectTripleAdd, body, commandStampTimeout); err != nil {
		return fmt.Errorf("questlifecycle: stamp %s=%v: %w", predicate, object, err)
	}
	return nil
}

// StampCommand writes any argument triples, then the quest.command trigger, so
// each argument is durably present when the rule fires. args maps predicate →
// object; the command is always written last.
func StampCommand(ctx context.Context, client *natsclient.Client, questID, command string, args map[string]string) error {
	for pred, obj := range args {
		if err := StampTriple(ctx, client, questID, pred, obj); err != nil {
			return err
		}
	}
	return StampTriple(ctx, client, questID, predCommand, command)
}
