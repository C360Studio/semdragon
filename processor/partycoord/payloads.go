package partycoord

import (
	"errors"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/types"
)

// =============================================================================
// PARTY COORDINATION PAYLOADS
// =============================================================================
// Event payloads for party lifecycle and coordination events.
// Communication events between party leads and members for quest coordination.
// Lead → Members: Decomposed, Assigned, ContextShared, Guidance
// Members → Lead: Progress, HelpRequest, ResultSubmitted
// Rollup: RollupStarted, RollupCompleted
// =============================================================================

// Ensure Graphable implementations
var (
	_ graph.Graphable = (*PartyFormedPayload)(nil)
	_ graph.Graphable = (*PartyDisbandedPayload)(nil)
	_ graph.Graphable = (*PartyJoinedPayload)(nil)
	_ graph.Graphable = (*PartyQuestDecomposedPayload)(nil)
	_ graph.Graphable = (*PartyTaskAssignedPayload)(nil)
	_ graph.Graphable = (*PartyProgressReportedPayload)(nil)
	_ graph.Graphable = (*PartyHelpRequestedPayload)(nil)
	_ graph.Graphable = (*PartyResultSubmittedPayload)(nil)
	_ graph.Graphable = (*PartyContextSharedPayload)(nil)
	_ graph.Graphable = (*PartyRollupStartedPayload)(nil)
	_ graph.Graphable = (*PartyRollupCompletedPayload)(nil)
)

// --- Typed Subjects ---

var (
	SubjectPartyFormed           = natsclient.NewSubject[PartyFormedPayload](domain.PredicatePartyFormed)
	SubjectPartyDisbanded        = natsclient.NewSubject[PartyDisbandedPayload](domain.PredicatePartyDisbanded)
	SubjectPartyJoined           = natsclient.NewSubject[PartyJoinedPayload](domain.PredicatePartyJoined)
	SubjectPartyQuestDecomposed  = natsclient.NewSubject[PartyQuestDecomposedPayload](domain.PredicatePartyQuestDecomposed)
	SubjectPartyTaskAssigned     = natsclient.NewSubject[PartyTaskAssignedPayload](domain.PredicatePartyTaskAssigned)
	SubjectPartyProgressReported = natsclient.NewSubject[PartyProgressReportedPayload](domain.PredicatePartyProgressReported)
	SubjectPartyHelpRequested    = natsclient.NewSubject[PartyHelpRequestedPayload](domain.PredicatePartyHelpRequested)
	SubjectPartyResultSubmitted  = natsclient.NewSubject[PartyResultSubmittedPayload](domain.PredicatePartyResultSubmitted)
	SubjectPartyContextShared    = natsclient.NewSubject[PartyContextSharedPayload](domain.PredicatePartyContextShared)
	SubjectPartyRollupStarted    = natsclient.NewSubject[PartyRollupStartedPayload](domain.PredicatePartyRollupStarted)
	SubjectPartyRollupCompleted  = natsclient.NewSubject[PartyRollupCompletedPayload](domain.PredicatePartyRollupCompleted)
)

// --- TraceInfo for observability ---

// TraceInfo contains trace context for observability.
type TraceInfo struct {
	TrajectoryID string `json:"trajectory_id,omitempty"`
	SpanID       string `json:"span_id,omitempty"`
	ParentSpanID string `json:"parent_span_id,omitempty"`
}

// =============================================================================
// PARTY FORMED PAYLOAD
// =============================================================================

// PartyFormedPayload contains data for party.formation.formed events.
type PartyFormedPayload struct {
	Party    Party     `json:"party"`
	FormedAt time.Time `json:"formed_at"`
	Trace    TraceInfo `json:"trace,omitempty"`
}

func (p *PartyFormedPayload) EntityID() string { return string(p.Party.ID) }

func (p *PartyFormedPayload) Triples() []message.Triple {
	return p.Party.Triples()
}

func (p *PartyFormedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "party.formed", Version: "v1"}
}

func (p *PartyFormedPayload) Validate() error {
	if p.Party.ID == "" {
		return errors.New("party_id required")
	}
	if p.FormedAt.IsZero() {
		return errors.New("formed_at required")
	}
	return nil
}

// =============================================================================
// PARTY DISBANDED PAYLOAD
// =============================================================================

// PartyDisbandedPayload contains data for party.formation.disbanded events.
type PartyDisbandedPayload struct {
	PartyID     domain.PartyID `json:"party_id"`
	QuestID     domain.QuestID `json:"quest_id"`
	Reason      string         `json:"reason"` // completed, failed, cancelled
	DisbandedAt time.Time      `json:"disbanded_at"`
	Trace       TraceInfo      `json:"trace,omitempty"`
}

func (p *PartyDisbandedPayload) EntityID() string { return string(p.PartyID) }

func (p *PartyDisbandedPayload) Triples() []message.Triple {
	source := "partycoord"
	entityID := string(p.PartyID)

	return []message.Triple{
		{Subject: entityID, Predicate: "party.status.state", Object: string(domain.PartyDisbanded), Source: source, Timestamp: p.DisbandedAt, Confidence: 1.0},
		{Subject: entityID, Predicate: "party.lifecycle.disbanded_at", Object: p.DisbandedAt.Format(time.RFC3339), Source: source, Timestamp: p.DisbandedAt, Confidence: 1.0},
		{Subject: entityID, Predicate: "party.disband.reason", Object: p.Reason, Source: source, Timestamp: p.DisbandedAt, Confidence: 1.0},
	}
}

func (p *PartyDisbandedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "party.disbanded", Version: "v1"}
}

func (p *PartyDisbandedPayload) Validate() error {
	if p.PartyID == "" {
		return errors.New("party_id required")
	}
	if p.DisbandedAt.IsZero() {
		return errors.New("disbanded_at required")
	}
	return nil
}

// =============================================================================
// PARTY JOINED PAYLOAD
// =============================================================================

// PartyJoinedPayload contains data for party.membership.joined events.
type PartyJoinedPayload struct {
	PartyID  domain.PartyID   `json:"party_id"`
	AgentID  domain.AgentID   `json:"agent_id"`
	Role     domain.PartyRole `json:"role"`
	JoinedAt time.Time        `json:"joined_at"`
	Trace    TraceInfo        `json:"trace,omitempty"`
}

func (p *PartyJoinedPayload) EntityID() string { return string(p.PartyID) }

func (p *PartyJoinedPayload) Triples() []message.Triple {
	source := "partycoord"
	entityID := string(p.PartyID)

	return []message.Triple{
		{Subject: entityID, Predicate: "party.membership.member", Object: string(p.AgentID), Source: source, Timestamp: p.JoinedAt, Confidence: 1.0},
		{Subject: entityID, Predicate: "party.member." + string(p.AgentID) + ".role", Object: string(p.Role), Source: source, Timestamp: p.JoinedAt, Confidence: 1.0},
		{Subject: string(p.AgentID), Predicate: "agent.membership.party", Object: string(p.PartyID), Source: source, Timestamp: p.JoinedAt, Confidence: 1.0},
	}
}

func (p *PartyJoinedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "party.joined", Version: "v1"}
}

func (p *PartyJoinedPayload) Validate() error {
	if p.PartyID == "" {
		return errors.New("party_id required")
	}
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.JoinedAt.IsZero() {
		return errors.New("joined_at required")
	}
	return nil
}

// =============================================================================
// PARTY QUEST DECOMPOSED PAYLOAD
// =============================================================================

// PartyQuestDecomposedPayload contains data for party.coordination.decomposed events.
// Emitted when a party lead breaks down the parent quest into sub-quests.
type PartyQuestDecomposedPayload struct {
	PartyID     domain.PartyID   `json:"party_id"`
	LeadID      domain.AgentID   `json:"lead_id"`
	ParentQuest domain.QuestID   `json:"parent_quest"`
	SubQuests   []domain.QuestID `json:"sub_quests"`
	Strategy    string           `json:"strategy"` // How lead approached decomposition
	Timestamp   time.Time        `json:"timestamp"`
	Trace       TraceInfo        `json:"trace,omitempty"`
}

func (p *PartyQuestDecomposedPayload) EntityID() string { return string(p.PartyID) }

func (p *PartyQuestDecomposedPayload) Triples() []message.Triple {
	source := "partycoord"
	entityID := string(p.PartyID)

	triples := []message.Triple{
		{Subject: entityID, Predicate: "party.coordination.decomposed_by", Object: string(p.LeadID), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "party.coordination.parent_quest", Object: string(p.ParentQuest), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "party.coordination.sub_quest_count", Object: len(p.SubQuests), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "party.strategy", Object: p.Strategy, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}

	for _, sq := range p.SubQuests {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "party.coordination.sub_quest", Object: string(sq),
			Source: source, Timestamp: p.Timestamp, Confidence: 1.0,
		})
	}

	return triples
}

func (p *PartyQuestDecomposedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "party.decomposed", Version: "v1"}
}

func (p *PartyQuestDecomposedPayload) Validate() error {
	if p.PartyID == "" {
		return errors.New("party_id required")
	}
	if p.LeadID == "" {
		return errors.New("lead_id required")
	}
	if p.ParentQuest == "" {
		return errors.New("parent_quest required")
	}
	if len(p.SubQuests) == 0 {
		return errors.New("sub_quests required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// =============================================================================
// PARTY TASK ASSIGNED PAYLOAD
// =============================================================================

// PartyTaskAssignedPayload contains data for party.coordination.assigned events.
// Emitted when a party lead assigns a sub-quest to a member.
type PartyTaskAssignedPayload struct {
	PartyID      domain.PartyID   `json:"party_id"`
	LeadID       domain.AgentID   `json:"lead_id"`
	AssignedTo   domain.AgentID   `json:"assigned_to"`
	SubQuestID   domain.QuestID   `json:"sub_quest_id"`
	Rationale    string           `json:"rationale"`              // Why this member
	Dependencies []domain.QuestID `json:"dependencies,omitempty"` // Wait for these first
	Guidance     string           `json:"guidance,omitempty"`     // Initial hints
	Timestamp    time.Time        `json:"timestamp"`
	Trace        TraceInfo        `json:"trace,omitempty"`
}

func (p *PartyTaskAssignedPayload) EntityID() string { return string(p.PartyID) }

func (p *PartyTaskAssignedPayload) Triples() []message.Triple {
	source := "partycoord"
	entityID := string(p.PartyID)

	triples := []message.Triple{
		{Subject: entityID, Predicate: "party.assignment." + string(p.SubQuestID), Object: string(p.AssignedTo), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: string(p.SubQuestID), Predicate: "quest.assignment.agent", Object: string(p.AssignedTo), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: string(p.SubQuestID), Predicate: "quest.assignment.party", Object: string(p.PartyID), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}

	return triples
}

func (p *PartyTaskAssignedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "party.assigned", Version: "v1"}
}

func (p *PartyTaskAssignedPayload) Validate() error {
	if p.PartyID == "" {
		return errors.New("party_id required")
	}
	if p.LeadID == "" {
		return errors.New("lead_id required")
	}
	if p.AssignedTo == "" {
		return errors.New("assigned_to required")
	}
	if p.SubQuestID == "" {
		return errors.New("sub_quest_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// =============================================================================
// PARTY PROGRESS REPORTED PAYLOAD
// =============================================================================

// PartyProgressReportedPayload contains data for party.coordination.progress events.
// Emitted when a party member reports progress on their sub-quest.
type PartyProgressReportedPayload struct {
	PartyID         domain.PartyID `json:"party_id"`
	MemberID        domain.AgentID `json:"member_id"`
	SubQuestID      domain.QuestID `json:"sub_quest_id"`
	ProgressPercent int            `json:"progress_percent"` // 0-100
	Status          string         `json:"status"`           // on_track, blocked, ahead, behind
	Message         string         `json:"message,omitempty"`
	Timestamp       time.Time      `json:"timestamp"`
	Trace           TraceInfo      `json:"trace,omitempty"`
}

func (p *PartyProgressReportedPayload) EntityID() string { return string(p.PartyID) }

func (p *PartyProgressReportedPayload) Triples() []message.Triple {
	source := "partycoord"
	entityID := string(p.PartyID)

	return []message.Triple{
		{Subject: entityID, Predicate: "party.progress." + string(p.SubQuestID) + ".percent", Object: p.ProgressPercent, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "party.progress." + string(p.SubQuestID) + ".status", Object: p.Status, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

func (p *PartyProgressReportedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "party.progress", Version: "v1"}
}

func (p *PartyProgressReportedPayload) Validate() error {
	if p.PartyID == "" {
		return errors.New("party_id required")
	}
	if p.MemberID == "" {
		return errors.New("member_id required")
	}
	if p.SubQuestID == "" {
		return errors.New("sub_quest_id required")
	}
	if p.Status == "" {
		return errors.New("status required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// =============================================================================
// PARTY HELP REQUESTED PAYLOAD
// =============================================================================

// PartyHelpRequestedPayload contains data for party.coordination.helprequest events.
// Emitted when a party member needs help from the lead.
type PartyHelpRequestedPayload struct {
	PartyID     domain.PartyID `json:"party_id"`
	MemberID    domain.AgentID `json:"member_id"`
	SubQuestID  domain.QuestID `json:"sub_quest_id"`
	IssueType   string         `json:"issue_type"` // blocker, confusion, skill_gap
	Description string         `json:"description"`
	Urgency     string         `json:"urgency"` // low, medium, high, critical
	Timestamp   time.Time      `json:"timestamp"`
	Trace       TraceInfo      `json:"trace,omitempty"`
}

func (p *PartyHelpRequestedPayload) EntityID() string { return string(p.PartyID) }

func (p *PartyHelpRequestedPayload) Triples() []message.Triple {
	source := "partycoord"
	entityID := string(p.PartyID)

	return []message.Triple{
		{Subject: entityID, Predicate: "party.help.requested_by", Object: string(p.MemberID), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "party.help.issue_type", Object: p.IssueType, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "party.help.urgency", Object: p.Urgency, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

func (p *PartyHelpRequestedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "party.helprequest", Version: "v1"}
}

func (p *PartyHelpRequestedPayload) Validate() error {
	if p.PartyID == "" {
		return errors.New("party_id required")
	}
	if p.MemberID == "" {
		return errors.New("member_id required")
	}
	if p.SubQuestID == "" {
		return errors.New("sub_quest_id required")
	}
	if p.Description == "" {
		return errors.New("description required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// =============================================================================
// PARTY RESULT SUBMITTED PAYLOAD
// =============================================================================

// PartyResultSubmittedPayload contains data for party.coordination.resultsubmitted events.
// Emitted when a party member submits their sub-quest result to the lead.
type PartyResultSubmittedPayload struct {
	PartyID      domain.PartyID `json:"party_id"`
	MemberID     domain.AgentID `json:"member_id"`
	SubQuestID   domain.QuestID `json:"sub_quest_id"`
	Result       any            `json:"result"`
	QualityScore float64        `json:"quality_score,omitempty"` // Self-assessed or from pre-review
	Timestamp    time.Time      `json:"timestamp"`
	Trace        TraceInfo      `json:"trace,omitempty"`
}

func (p *PartyResultSubmittedPayload) EntityID() string { return string(p.PartyID) }

func (p *PartyResultSubmittedPayload) Triples() []message.Triple {
	source := "partycoord"
	entityID := string(p.PartyID)

	return []message.Triple{
		{Subject: entityID, Predicate: "party.result." + string(p.SubQuestID) + ".submitted_by", Object: string(p.MemberID), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "party.result." + string(p.SubQuestID) + ".quality", Object: p.QualityScore, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

func (p *PartyResultSubmittedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "party.resultsubmitted", Version: "v1"}
}

func (p *PartyResultSubmittedPayload) Validate() error {
	if p.PartyID == "" {
		return errors.New("party_id required")
	}
	if p.MemberID == "" {
		return errors.New("member_id required")
	}
	if p.SubQuestID == "" {
		return errors.New("sub_quest_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// =============================================================================
// PARTY CONTEXT SHARED PAYLOAD
// =============================================================================

// PartyContextSharedPayload contains data for party.coordination.contextshared events.
// Emitted when context/insight is shared with the party.
type PartyContextSharedPayload struct {
	PartyID     domain.PartyID   `json:"party_id"`
	SharedBy    domain.AgentID   `json:"shared_by"`
	ContextItem ContextItem      `json:"context_item"`
	Relevance   []domain.QuestID `json:"relevance,omitempty"` // Which sub-quests this affects
	Timestamp   time.Time        `json:"timestamp"`
	Trace       TraceInfo        `json:"trace,omitempty"`
}

func (p *PartyContextSharedPayload) EntityID() string { return string(p.PartyID) }

func (p *PartyContextSharedPayload) Triples() []message.Triple {
	source := "partycoord"
	entityID := string(p.PartyID)

	return []message.Triple{
		{Subject: entityID, Predicate: "party.context." + p.ContextItem.Key, Object: string(p.SharedBy), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "party.context.count", Object: 1, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

func (p *PartyContextSharedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "party.contextshared", Version: "v1"}
}

func (p *PartyContextSharedPayload) Validate() error {
	if p.PartyID == "" {
		return errors.New("party_id required")
	}
	if p.SharedBy == "" {
		return errors.New("shared_by required")
	}
	if p.ContextItem.Key == "" {
		return errors.New("context_item.key required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// =============================================================================
// PARTY ROLLUP STARTED PAYLOAD
// =============================================================================

// PartyRollupStartedPayload contains data for party.coordination.rollupstarted events.
// Emitted when the party lead begins combining sub-results.
type PartyRollupStartedPayload struct {
	PartyID         domain.PartyID `json:"party_id"`
	LeadID          domain.AgentID `json:"lead_id"`
	ParentQuestID   domain.QuestID `json:"parent_quest_id"`
	SubResultsCount int            `json:"sub_results_count"`
	Timestamp       time.Time      `json:"timestamp"`
	Trace           TraceInfo      `json:"trace,omitempty"`
}

func (p *PartyRollupStartedPayload) EntityID() string { return string(p.PartyID) }

func (p *PartyRollupStartedPayload) Triples() []message.Triple {
	source := "partycoord"
	entityID := string(p.PartyID)

	return []message.Triple{
		{Subject: entityID, Predicate: "party.rollup.started_at", Object: p.Timestamp.Format(time.RFC3339), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "party.rollup.sub_results_count", Object: p.SubResultsCount, Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

func (p *PartyRollupStartedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "party.rollupstarted", Version: "v1"}
}

func (p *PartyRollupStartedPayload) Validate() error {
	if p.PartyID == "" {
		return errors.New("party_id required")
	}
	if p.LeadID == "" {
		return errors.New("lead_id required")
	}
	if p.ParentQuestID == "" {
		return errors.New("parent_quest_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// =============================================================================
// PARTY ROLLUP COMPLETED PAYLOAD
// =============================================================================

// PartyRollupCompletedPayload contains data for party.coordination.rollupcompleted events.
// Emitted when the rollup is done and ready for boss battle.
type PartyRollupCompletedPayload struct {
	PartyID       domain.PartyID `json:"party_id"`
	LeadID        domain.AgentID `json:"lead_id"`
	ParentQuestID domain.QuestID `json:"parent_quest_id"`
	RollupResult  any            `json:"rollup_result"`
	Timestamp     time.Time      `json:"timestamp"`
	Trace         TraceInfo      `json:"trace,omitempty"`
}

func (p *PartyRollupCompletedPayload) EntityID() string { return string(p.PartyID) }

func (p *PartyRollupCompletedPayload) Triples() []message.Triple {
	source := "partycoord"
	entityID := string(p.PartyID)

	return []message.Triple{
		{Subject: entityID, Predicate: "party.rollup.completed_at", Object: p.Timestamp.Format(time.RFC3339), Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
		{Subject: entityID, Predicate: "party.rollup.status", Object: "completed", Source: source, Timestamp: p.Timestamp, Confidence: 1.0},
	}
}

func (p *PartyRollupCompletedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "party.rollupcompleted", Version: "v1"}
}

func (p *PartyRollupCompletedPayload) Validate() error {
	if p.PartyID == "" {
		return errors.New("party_id required")
	}
	if p.LeadID == "" {
		return errors.New("lead_id required")
	}
	if p.ParentQuestID == "" {
		return errors.New("parent_quest_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}
