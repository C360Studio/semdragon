package questlifecycle

// Command trigger values stamped on quest.lifecycle.command to request a lifecycle
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
	// the quest.review.needs-review predicate (written at post time).
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
	PredClaimRequestedBy = "quest.assignment.requested-by"
	// PredDataOutput carries the submitted work product the submit rules leave
	// on the entity as quest.data.output.
	PredDataOutput = "quest.data.output"
)
