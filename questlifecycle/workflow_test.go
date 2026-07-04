package questlifecycle

import (
	"testing"

	"github.com/c360studio/semdragons/domain"
)

// TestTransitions_Valid asserts the phase graph passes the framework's structural
// validation (every out-edge target is a declared key; no dup out-edges).
func TestTransitions_Valid(t *testing.T) {
	if err := Transitions.Validate(); err != nil {
		t.Fatalf("Transitions.Validate() = %v, want nil", err)
	}
}

// TestTransitions_TerminalPhases pins the spec's terminal set (completed, failed,
// escalated) and non-terminal set. Terminal = no out-edges.
func TestTransitions_TerminalPhases(t *testing.T) {
	for _, p := range []string{PhaseCompleted, PhaseFailed, PhaseEscalated} {
		if !Transitions.IsTerminal(p) {
			t.Errorf("phase %q must be terminal", p)
		}
	}
	for _, p := range []string{PhasePosted, PhaseClaimed, PhaseInProgress, PhaseInReview, PhasePendingTriage} {
		if Transitions.IsTerminal(p) {
			t.Errorf("phase %q must not be terminal", p)
		}
	}
}

// TestTransitions_SpecEdges pins the valid/invalid transitions the quest-lifecycle
// spec scenarios require.
func TestTransitions_SpecEdges(t *testing.T) {
	valid := [][2]string{
		{PhasePosted, PhaseClaimed},          // claim
		{PhaseClaimed, PhaseInProgress},      // start
		{PhaseInProgress, PhaseInReview},     // submission with review
		{PhaseInProgress, PhaseCompleted},    // submission without review
		{PhaseInReview, PhaseCompleted},      // verdict pass
		{PhaseInReview, PhaseFailed},         // verdict defeat
		{PhaseInProgress, PhasePosted},       // fail-retry / abandon
		{PhaseInProgress, PhasePendingTriage},// fail-triage
		{PhasePendingTriage, PhasePosted},    // triage repost
		{PhaseClaimed, PhaseEscalated},       // operator escalate
	}
	for _, e := range valid {
		if !Transitions.IsValidTransition(e[0], e[1]) {
			t.Errorf("expected VALID transition %s → %s", e[0], e[1])
		}
	}

	// Spec scenario "Invalid transition is rejected": posted → completed.
	if Transitions.IsValidTransition(PhasePosted, PhaseCompleted) {
		t.Error("posted → completed must be rejected")
	}
	// Spec scenario "Terminal phase is final".
	if Transitions.IsValidTransition(PhaseCompleted, PhasePosted) {
		t.Error("completed → posted must be rejected (terminal)")
	}
}

// TestQuestState_ParticipantContract asserts the Participant methods and that the
// phase constants stay equal to the domain enum (single source of truth).
func TestQuestState_ParticipantContract(t *testing.T) {
	q := &QuestState{EntityIDField: "c360.test.game.board1.quest.abc", PhaseField: PhaseInProgress}

	if got := q.EntityID(); got != "c360.test.game.board1.quest.abc" {
		t.Errorf("EntityID() = %q", got)
	}
	if got := q.Workflow(); got != WorkflowName {
		t.Errorf("Workflow() = %q, want %q", got, WorkflowName)
	}
	if got := q.Phase(); got != PhaseInProgress {
		t.Errorf("Phase() = %q, want %q", got, PhaseInProgress)
	}
	if q.IsTerminal() {
		t.Error("in_progress must not be terminal")
	}
	if q.ParentEntityID() != "" {
		t.Errorf("ParentEntityID() = %q, want empty", q.ParentEntityID())
	}

	// Phase strings MUST equal the domain enum so the workflow table and
	// domain.Quest.Triples cannot drift.
	pairs := map[string]domain.QuestStatus{
		PhasePosted: domain.QuestPosted, PhaseClaimed: domain.QuestClaimed,
		PhaseInProgress: domain.QuestInProgress, PhaseInReview: domain.QuestInReview,
		PhaseCompleted: domain.QuestCompleted, PhaseFailed: domain.QuestFailed,
		PhaseEscalated: domain.QuestEscalated, PhasePendingTriage: domain.QuestPendingTriage,
	}
	for phase, status := range pairs {
		if phase != string(status) {
			t.Errorf("phase const %q != domain enum %q", phase, status)
		}
	}
}

// TestWorkflowDeclaration_Fields asserts the declaration matches the spec+design:
// the phase predicate is the pre-existing quest.status.state (design D3), the
// workflow name is "quest", and the operator-writable list matches the struct tags.
func TestWorkflowDeclaration_Fields(t *testing.T) {
	wf := WorkflowDeclaration()

	if wf.Name != WorkflowName {
		t.Errorf("Name = %q, want %q", wf.Name, WorkflowName)
	}
	if wf.PhasePredicate != "quest.status.state" {
		t.Errorf("PhasePredicate = %q, want quest.status.state (design D3)", wf.PhasePredicate)
	}
	if wf.EntityIDPattern != EntityIDPattern {
		t.Errorf("EntityIDPattern = %q", wf.EntityIDPattern)
	}
	if err := wf.Transitions.Validate(); err != nil {
		t.Errorf("declaration Transitions invalid: %v", err)
	}
	if len(wf.OperatorWritablePredicates) == 0 {
		t.Error("expected operator-writable predicates (retry counter must be rule-settable)")
	}
	// Audit predicates must be declared for a truthful, source-attributed history.
	if wf.AuditPredicates.Source == "" || wf.AuditPredicates.At == "" {
		t.Error("AuditPredicates must be declared for transition history")
	}
}
