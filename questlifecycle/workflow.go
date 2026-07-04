// Package questlifecycle declares the quest state machine as a semstreams
// pkg/lifecycle Workflow: the transition table, the QuestState projection schema,
// and the phase predicate. It replaces the hand-rolled transition handlers in
// processor/questboard (rebuild-quest-lifecycle-on-semstreams-rules, Stage B1).
//
// The workflow is registered against the shared lifecycle.Manager
// (deps.LifecycleManager) at boot; rule-engine lifecycle_transition/complete/fail
// actions and the lifecycle-gateway then drive and inspect quest phases. The
// framework writes each phase transition — plus any coupled fields — as a single
// atomic, CAS-guarded triple mutation into ENTITY_STATES, so quest state stays
// graph-visible and the audit trail is the KV revision history.
package questlifecycle

import (
	"reflect"

	"github.com/c360studio/semstreams/pkg/lifecycle"

	"github.com/c360studio/semdragons/domain"
)

// WorkflowName is the lifecycle workflow type id for quests. It MUST equal
// QuestState.Workflow() and is the value rule files reference as "workflow":"quest".
const WorkflowName = "quest"

// PhasePredicate is the triple predicate carrying a quest's current phase. It is
// the pre-existing status predicate emitted by domain.Quest.Triples, kept verbatim
// so UI/SSE consumers and dmworldstate keep working unchanged (design D3).
const PhasePredicate = "quest.status.state"

// EntityIDPattern is the 6-part federated entity-ID glob for quests:
// org.platform.game.board.quest.instance (e.g. c360.prod.game.board1.quest.abc123).
const EntityIDPattern = "*.*.game.*.quest.*"

// Phase constants are the quest phase strings. They are defined as conversions of
// the domain.QuestStatus enum so the workflow table and domain.Quest.Triples can
// never drift out of sync (single source of truth).
const (
	PhasePosted        = string(domain.QuestPosted)
	PhaseClaimed       = string(domain.QuestClaimed)
	PhaseInProgress    = string(domain.QuestInProgress)
	PhaseInReview      = string(domain.QuestInReview)
	PhaseCompleted     = string(domain.QuestCompleted)
	PhaseFailed        = string(domain.QuestFailed)
	PhaseEscalated     = string(domain.QuestEscalated)
	PhasePendingTriage = string(domain.QuestPendingTriage)
)

// Operator-writable predicates. These are the quest fields a rule's lifecycle_set
// or an operator patch may write atomically with a transition. Kept in one slice
// so the Workflow declaration and the QuestState struct tags stay consistent.
const (
	predAttemptsCurrent = "quest.attempts.current"
	predAttemptsMax     = "quest.attempts.max"
	predAssignmentAgent = "quest.assignment.agent"
	predAssignmentParty = "quest.assignment.party"
	predFailureReason   = "quest.failure.reason"
	predVerdictFeedback = "quest.verdict.feedback"
)

// Transitions is the quest lifecycle phase graph (quest-lifecycle spec: "Quest
// lifecycle is a declared workflow" + "Standard lifecycle path" + "Failure routing
// with retry budget"). It mirrors the edges the current processor/questboard
// handlers effect:
//
//	posted        → claimed (ClaimQuest) | in_progress (ClaimAndStartForParty)
//	claimed       → in_progress (StartQuest) | posted (AbandonQuest) | escalated
//	in_progress   → in_review (SubmitResult w/ review) | completed (w/o review)
//	              | posted (abandon / fail-retry) | pending_triage (fail-triage)
//	              | failed (fail-terminal) | escalated
//	in_review     → completed (verdict pass) | failed (verdict defeat)
//	              | posted (fail-retry) | pending_triage (fail-triage) | escalated
//	pending_triage→ posted (triage salvage/TPK) | escalated | failed
//
// completed, failed, and escalated are terminal (empty out-edge slices, per spec).
// pending_triage is non-terminal so the DM can repost or escalate.
var Transitions = lifecycle.Transitions{
	PhasePosted:        {PhaseClaimed, PhaseInProgress},
	PhaseClaimed:       {PhaseInProgress, PhasePosted, PhaseEscalated},
	PhaseInProgress:    {PhaseInReview, PhaseCompleted, PhasePosted, PhasePendingTriage, PhaseFailed, PhaseEscalated},
	PhaseInReview:      {PhaseCompleted, PhaseFailed, PhasePosted, PhasePendingTriage, PhaseEscalated},
	PhasePendingTriage: {PhasePosted, PhaseEscalated, PhaseFailed},
	PhaseCompleted:     {},
	PhaseFailed:        {},
	PhaseEscalated:     {},
}

// QuestState is the lifecycle projection of a quest — the small set of predicates
// the lifecycle.Manager owns and writes atomically on each transition. It is NOT
// the full domain.Quest entity: identity/spec/context/dag predicates are owned by
// their respective writers (see research/05-ownership-spike.md). Field/method names
// follow the framework convention (EntityIDField/PhaseField backing EntityID()/Phase()
// to avoid the field-vs-method name collision).
type QuestState struct {
	EntityIDField string `json:"entity_id" lifecycle:"id"`
	PhaseField    string `json:"phase" lifecycle:"phase,predicate=quest.status.state"`

	// Retry budget. Attempts is operator_writable so the fail→requeue rule can
	// atomically increment it inside the transition and the triage rule can branch
	// on it (gte max → pending_triage). MaxAttempts is operator_writable so a DM
	// triage decision can grant one more attempt.
	Attempts    int `json:"attempts,omitempty" lifecycle:"operator_writable,predicate=quest.attempts.current"`
	MaxAttempts int `json:"max_attempts,omitempty" lifecycle:"operator_writable,predicate=quest.attempts.max"`

	// Assignment. Set on claim, cleared on abandon/fail-repost.
	ClaimedBy     string `json:"claimed_by,omitempty" lifecycle:"operator_writable,predicate=quest.assignment.agent"`
	AssignedParty string `json:"assigned_party,omitempty" lifecycle:"operator_writable,predicate=quest.assignment.party"`

	// Failure context recorded on the fail edge.
	FailureReason string `json:"failure_reason,omitempty" lifecycle:"operator_writable,predicate=quest.failure.reason"`

	// Verdict stamped on the in_review resolution. While mechanics are disabled the
	// stub rule sets this to a review-skipped marker (spec: "Mechanics side effects
	// are stubbed in this change").
	Verdict string `json:"verdict,omitempty" lifecycle:"operator_writable,predicate=quest.verdict.feedback"`
}

// EntityID returns the 6-part federated entity id.
func (q *QuestState) EntityID() string { return q.EntityIDField }

// Workflow returns the workflow type id; MUST equal WorkflowName.
func (q *QuestState) Workflow() string { return WorkflowName }

// Phase returns the current phase.
func (q *QuestState) Phase() string { return q.PhaseField }

// IsTerminal reports whether the current phase has no out-edges.
func (q *QuestState) IsTerminal() bool { return Transitions.IsTerminal(q.PhaseField) }

// ParentEntityID returns "" for quests. Party sub-quest parent links are modeled by
// the gated-dag layer in Stage B2, not by the lifecycle parent/child mechanism.
func (q *QuestState) ParentEntityID() string { return "" }

// WorkflowDeclaration returns the quest lifecycle.Workflow for Manager.Register.
// AuditPredicates give a truthful, source-attributed transition history for free
// (Manager.History reconstructs TransitionEvents from these triples).
func WorkflowDeclaration() lifecycle.Workflow {
	return lifecycle.Workflow{
		Name:            WorkflowName,
		EntityIDPattern: EntityIDPattern,
		Phases: []string{
			PhasePosted, PhaseClaimed, PhaseInProgress, PhaseInReview,
			PhaseCompleted, PhaseFailed, PhaseEscalated, PhasePendingTriage,
		},
		Transitions:    Transitions,
		PhasePredicate: PhasePredicate,
		Schema:         reflect.TypeOf(QuestState{}),
		OperatorWritablePredicates: []string{
			predAttemptsCurrent, predAttemptsMax,
			predAssignmentAgent, predAssignmentParty,
			predFailureReason, predVerdictFeedback,
		},
		AuditPredicates: lifecycle.AuditSpec{
			Source: "quest.lifecycle.last_transition_source",
			At:     "quest.lifecycle.last_transition_at",
			From:   "quest.lifecycle.last_transition_from",
			Note:   "quest.lifecycle.last_transition_note",
		},
	}
}
