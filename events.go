package semdragons

import (
	"context"
	"errors"
	"time"

	"github.com/c360studio/semstreams/natsclient"
)

// =============================================================================
// EVENTS - Typed subjects using vocabulary predicates
// =============================================================================
// All event subjects use three-part vocabulary predicates (quest.lifecycle.posted).
// This enables NATS wildcard subscriptions like "quest.lifecycle.>" for all
// quest lifecycle events.
// =============================================================================

// --- Typed Subjects ---
// Subjects use vocabulary predicate constants as the NATS subject pattern.

var (
	// SubjectQuestPosted is the typed subject for quest.lifecycle.posted events.
	SubjectQuestPosted = natsclient.NewSubject[QuestPostedPayload](PredicateQuestPosted)
	// SubjectQuestClaimed is the typed subject for quest.lifecycle.claimed events.
	SubjectQuestClaimed = natsclient.NewSubject[QuestClaimedPayload](PredicateQuestClaimed)
	// SubjectQuestStarted is the typed subject for quest.lifecycle.started events.
	SubjectQuestStarted = natsclient.NewSubject[QuestStartedPayload](PredicateQuestStarted)
	// SubjectQuestSubmitted is the typed subject for quest.lifecycle.submitted events.
	SubjectQuestSubmitted = natsclient.NewSubject[QuestSubmittedPayload](PredicateQuestSubmitted)
	// SubjectQuestCompleted is the typed subject for quest.lifecycle.completed events.
	SubjectQuestCompleted = natsclient.NewSubject[QuestCompletedPayload](PredicateQuestCompleted)
	// SubjectQuestFailed is the typed subject for quest.lifecycle.failed events.
	SubjectQuestFailed = natsclient.NewSubject[QuestFailedPayload](PredicateQuestFailed)
	// SubjectQuestEscalated is the typed subject for quest.lifecycle.escalated events.
	SubjectQuestEscalated = natsclient.NewSubject[QuestEscalatedPayload](PredicateQuestEscalated)
	// SubjectQuestAbandoned is the typed subject for quest.lifecycle.abandoned events.
	SubjectQuestAbandoned = natsclient.NewSubject[QuestAbandonedPayload](PredicateQuestAbandoned)

	// SubjectBattleStarted is the typed subject for battle.review.started events.
	SubjectBattleStarted = natsclient.NewSubject[BattleStartedPayload](PredicateBattleStarted)
	// SubjectBattleVerdict is the typed subject for battle.review.verdict events.
	SubjectBattleVerdict = natsclient.NewSubject[BattleVerdictPayload](PredicateBattleVerdict)

	// SubjectAgentXP is the typed subject for agent.progression.xp events.
	SubjectAgentXP = natsclient.NewSubject[AgentXPPayload](PredicateAgentXP)
	// SubjectAgentLevelUp is the typed subject for agent.progression.levelup events.
	SubjectAgentLevelUp = natsclient.NewSubject[AgentLevelPayload](PredicateAgentLevelUp)
	// SubjectAgentLevelDown is the typed subject for agent.progression.leveldown events.
	SubjectAgentLevelDown = natsclient.NewSubject[AgentLevelPayload](PredicateAgentLevelDown)
	// SubjectAgentCooldown is the typed subject for agent.progression.cooldown events.
	SubjectAgentCooldown = natsclient.NewSubject[AgentCooldownPayload](PredicateAgentCooldown)

	// SubjectGuildSuggested is the typed subject for guild.formation.suggested events.
	SubjectGuildSuggested = natsclient.NewSubject[GuildSuggestedPayload](PredicateGuildSuggested)
	// SubjectGuildAutoJoined is the typed subject for guild.formation.autojoined events.
	SubjectGuildAutoJoined = natsclient.NewSubject[GuildAutoJoinedPayload](PredicateGuildAutoJoined)

	// SubjectSkillProgression is the typed subject for skill.progression.improved events.
	SubjectSkillProgression = natsclient.NewSubject[SkillProgressionPayload](PredicateSkillImproved)
	// SubjectSkillLevelUp is the typed subject for skill.progression.levelup events.
	SubjectSkillLevelUp = natsclient.NewSubject[SkillLevelUpPayload](PredicateSkillLevelUp)
	// SubjectMentorBonus is the typed subject for skill.progression.mentorbonus events.
	SubjectMentorBonus = natsclient.NewSubject[MentorBonusPayload](PredicateMentorBonus)

	// SubjectPartyQuestDecomposed is the typed subject for party.coordination.decomposed events.
	SubjectPartyQuestDecomposed = natsclient.NewSubject[PartyQuestDecomposedPayload](PredicatePartyQuestDecomposed)
	// SubjectPartyTaskAssigned is the typed subject for party.coordination.assigned events.
	SubjectPartyTaskAssigned = natsclient.NewSubject[PartyTaskAssignedPayload](PredicatePartyTaskAssigned)
	// SubjectPartyContextShared is the typed subject for party.coordination.contextshared events.
	SubjectPartyContextShared = natsclient.NewSubject[PartyContextSharedPayload](PredicatePartyContextShared)
	// SubjectPartyGuidanceIssued is the typed subject for party.coordination.guidance events.
	SubjectPartyGuidanceIssued = natsclient.NewSubject[PartyGuidanceIssuedPayload](PredicatePartyGuidanceIssued)
	// SubjectPartyProgressReported is the typed subject for party.coordination.progress events.
	SubjectPartyProgressReported = natsclient.NewSubject[PartyProgressReportedPayload](PredicatePartyProgressReported)
	// SubjectPartyHelpRequested is the typed subject for party.coordination.helprequest events.
	SubjectPartyHelpRequested = natsclient.NewSubject[PartyHelpRequestedPayload](PredicatePartyHelpRequested)
	// SubjectPartyResultSubmitted is the typed subject for party.coordination.resultsubmitted events.
	SubjectPartyResultSubmitted = natsclient.NewSubject[PartyResultSubmittedPayload](PredicatePartyResultSubmitted)
	// SubjectPartyRollupStarted is the typed subject for party.coordination.rollupstarted events.
	SubjectPartyRollupStarted = natsclient.NewSubject[PartyRollupStartedPayload](PredicatePartyRollupStarted)
	// SubjectPartyRollupCompleted is the typed subject for party.coordination.rollupcompleted events.
	SubjectPartyRollupCompleted = natsclient.NewSubject[PartyRollupCompletedPayload](PredicatePartyRollupCompleted)
)

// --- Payload Types ---

// QuestPostedPayload contains data for quest.lifecycle.posted events.
type QuestPostedPayload struct {
	Quest    Quest     `json:"quest"`
	PostedAt time.Time `json:"posted_at"`
	PostedBy string    `json:"posted_by,omitempty"` // DM or system identifier
	Trace    TraceInfo `json:"trace,omitempty"`     // Trace context for observability
}

// Validate checks that required fields are present.
func (p *QuestPostedPayload) Validate() error {
	if p.Quest.ID == "" {
		return errors.New("quest_id required")
	}
	if p.PostedAt.IsZero() {
		return errors.New("posted_at required")
	}
	return nil
}

// QuestClaimedPayload contains data for quest.lifecycle.claimed events.
type QuestClaimedPayload struct {
	Quest     Quest     `json:"quest"`
	AgentID   AgentID   `json:"agent_id"`
	PartyID   *PartyID  `json:"party_id,omitempty"`
	ClaimedAt time.Time `json:"claimed_at"`
	Trace     TraceInfo `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
func (p *QuestClaimedPayload) Validate() error {
	if p.Quest.ID == "" {
		return errors.New("quest_id required")
	}
	if p.AgentID == "" && p.PartyID == nil {
		return errors.New("agent_id or party_id required")
	}
	if p.ClaimedAt.IsZero() {
		return errors.New("claimed_at required")
	}
	return nil
}

// QuestStartedPayload contains data for quest.lifecycle.started events.
type QuestStartedPayload struct {
	Quest     Quest     `json:"quest"`
	AgentID   AgentID   `json:"agent_id"`
	PartyID   *PartyID  `json:"party_id,omitempty"`
	StartedAt time.Time `json:"started_at"`
	Trace     TraceInfo `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
func (p *QuestStartedPayload) Validate() error {
	if p.Quest.ID == "" {
		return errors.New("quest_id required")
	}
	if p.StartedAt.IsZero() {
		return errors.New("started_at required")
	}
	return nil
}

// QuestSubmittedPayload contains data for quest.lifecycle.submitted events.
type QuestSubmittedPayload struct {
	Quest       Quest     `json:"quest"`
	AgentID     AgentID   `json:"agent_id"`
	Result      any       `json:"result"`
	SubmittedAt time.Time `json:"submitted_at"`
	BattleID    *BattleID `json:"battle_id,omitempty"` // Set if review triggered
	Trace       TraceInfo `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
func (p *QuestSubmittedPayload) Validate() error {
	if p.Quest.ID == "" {
		return errors.New("quest_id required")
	}
	if p.SubmittedAt.IsZero() {
		return errors.New("submitted_at required")
	}
	return nil
}

// QuestCompletedPayload contains data for quest.lifecycle.completed events.
type QuestCompletedPayload struct {
	Quest       Quest         `json:"quest"`
	AgentID     AgentID       `json:"agent_id"`
	PartyID     *PartyID      `json:"party_id,omitempty"`
	Verdict     BattleVerdict `json:"verdict"`
	CompletedAt time.Time     `json:"completed_at"`
	Duration    time.Duration `json:"duration"`
	Trace       TraceInfo     `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
func (p *QuestCompletedPayload) Validate() error {
	if p.Quest.ID == "" {
		return errors.New("quest_id required")
	}
	if p.CompletedAt.IsZero() {
		return errors.New("completed_at required")
	}
	return nil
}

// QuestFailedPayload contains data for quest.lifecycle.failed events.
type QuestFailedPayload struct {
	Quest    Quest       `json:"quest"`
	AgentID  AgentID     `json:"agent_id"`
	PartyID  *PartyID    `json:"party_id,omitempty"`
	Reason   string      `json:"reason"`
	FailType FailureType `json:"fail_type"`
	FailedAt time.Time   `json:"failed_at"`
	Attempt  int         `json:"attempt"`
	Reposted bool        `json:"reposted"`
	Trace    TraceInfo   `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
func (p *QuestFailedPayload) Validate() error {
	if p.Quest.ID == "" {
		return errors.New("quest_id required")
	}
	if p.Reason == "" {
		return errors.New("reason required")
	}
	if p.FailedAt.IsZero() {
		return errors.New("failed_at required")
	}
	return nil
}

// QuestEscalatedPayload contains data for quest.lifecycle.escalated events.
type QuestEscalatedPayload struct {
	Quest       Quest     `json:"quest"`
	AgentID     AgentID   `json:"agent_id,omitempty"`
	PartyID     *PartyID  `json:"party_id,omitempty"`
	Reason      string    `json:"reason"`
	EscalatedAt time.Time `json:"escalated_at"`
	Attempts    int       `json:"attempts"`
	Trace       TraceInfo `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
func (p *QuestEscalatedPayload) Validate() error {
	if p.Quest.ID == "" {
		return errors.New("quest_id required")
	}
	if p.Reason == "" {
		return errors.New("reason required")
	}
	if p.EscalatedAt.IsZero() {
		return errors.New("escalated_at required")
	}
	return nil
}

// QuestAbandonedPayload contains data for quest.lifecycle.abandoned events.
type QuestAbandonedPayload struct {
	Quest       Quest     `json:"quest"`
	AgentID     AgentID   `json:"agent_id"`
	PartyID     *PartyID  `json:"party_id,omitempty"`
	Reason      string    `json:"reason"`
	AbandonedAt time.Time `json:"abandoned_at"`
	Trace       TraceInfo `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
func (p *QuestAbandonedPayload) Validate() error {
	if p.Quest.ID == "" {
		return errors.New("quest_id required")
	}
	if p.Reason == "" {
		return errors.New("reason required")
	}
	if p.AbandonedAt.IsZero() {
		return errors.New("abandoned_at required")
	}
	return nil
}

// BattleStartedPayload contains data for battle.review.started events.
type BattleStartedPayload struct {
	Battle    BossBattle `json:"battle"`
	Quest     Quest      `json:"quest"`
	StartedAt time.Time  `json:"started_at"`
	Trace     TraceInfo  `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
func (p *BattleStartedPayload) Validate() error {
	if p.Battle.ID == "" {
		return errors.New("battle_id required")
	}
	if p.Quest.ID == "" {
		return errors.New("quest_id required")
	}
	if p.StartedAt.IsZero() {
		return errors.New("started_at required")
	}
	return nil
}

// BattleVerdictPayload contains data for battle.review.verdict events.
type BattleVerdictPayload struct {
	Battle  BossBattle    `json:"battle"`
	Quest   Quest         `json:"quest"`
	Verdict BattleVerdict `json:"verdict"`
	EndedAt time.Time     `json:"ended_at"`
	Trace   TraceInfo     `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
func (p *BattleVerdictPayload) Validate() error {
	if p.Battle.ID == "" {
		return errors.New("battle_id required")
	}
	if p.Quest.ID == "" {
		return errors.New("quest_id required")
	}
	if p.EndedAt.IsZero() {
		return errors.New("ended_at required")
	}
	return nil
}

// --- Event Publisher ---

// EventPublisher provides type-safe event publishing.
type EventPublisher struct {
	client *natsclient.Client
}

// NewEventPublisher creates a new event publisher.
func NewEventPublisher(client *natsclient.Client) *EventPublisher {
	return &EventPublisher{client: client}
}

// PublishQuestPosted publishes a quest.lifecycle.posted event.
func (ep *EventPublisher) PublishQuestPosted(ctx context.Context, payload QuestPostedPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectQuestPosted.Publish(ctx, ep.client, payload)
}

// PublishQuestClaimed publishes a quest.lifecycle.claimed event.
func (ep *EventPublisher) PublishQuestClaimed(ctx context.Context, payload QuestClaimedPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectQuestClaimed.Publish(ctx, ep.client, payload)
}

// PublishQuestStarted publishes a quest.lifecycle.started event.
func (ep *EventPublisher) PublishQuestStarted(ctx context.Context, payload QuestStartedPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectQuestStarted.Publish(ctx, ep.client, payload)
}

// PublishQuestSubmitted publishes a quest.lifecycle.submitted event.
func (ep *EventPublisher) PublishQuestSubmitted(ctx context.Context, payload QuestSubmittedPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectQuestSubmitted.Publish(ctx, ep.client, payload)
}

// PublishQuestCompleted publishes a quest.lifecycle.completed event.
func (ep *EventPublisher) PublishQuestCompleted(ctx context.Context, payload QuestCompletedPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectQuestCompleted.Publish(ctx, ep.client, payload)
}

// PublishQuestFailed publishes a quest.lifecycle.failed event.
func (ep *EventPublisher) PublishQuestFailed(ctx context.Context, payload QuestFailedPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectQuestFailed.Publish(ctx, ep.client, payload)
}

// PublishQuestEscalated publishes a quest.lifecycle.escalated event.
func (ep *EventPublisher) PublishQuestEscalated(ctx context.Context, payload QuestEscalatedPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectQuestEscalated.Publish(ctx, ep.client, payload)
}

// PublishQuestAbandoned publishes a quest.lifecycle.abandoned event.
func (ep *EventPublisher) PublishQuestAbandoned(ctx context.Context, payload QuestAbandonedPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectQuestAbandoned.Publish(ctx, ep.client, payload)
}

// PublishBattleStarted publishes a battle.review.started event.
func (ep *EventPublisher) PublishBattleStarted(ctx context.Context, payload BattleStartedPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectBattleStarted.Publish(ctx, ep.client, payload)
}

// PublishBattleVerdict publishes a battle.review.verdict event.
func (ep *EventPublisher) PublishBattleVerdict(ctx context.Context, payload BattleVerdictPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectBattleVerdict.Publish(ctx, ep.client, payload)
}

// --- Agent Progression Events ---

// PublishAgentXP publishes an agent.progression.xp event.
func (ep *EventPublisher) PublishAgentXP(ctx context.Context, payload AgentXPPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectAgentXP.Publish(ctx, ep.client, payload)
}

// PublishAgentLevelUp publishes an agent.progression.levelup event.
func (ep *EventPublisher) PublishAgentLevelUp(ctx context.Context, payload AgentLevelPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectAgentLevelUp.Publish(ctx, ep.client, payload)
}

// PublishAgentLevelDown publishes an agent.progression.leveldown event.
func (ep *EventPublisher) PublishAgentLevelDown(ctx context.Context, payload AgentLevelPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectAgentLevelDown.Publish(ctx, ep.client, payload)
}

// PublishAgentCooldown publishes an agent.progression.cooldown event.
func (ep *EventPublisher) PublishAgentCooldown(ctx context.Context, payload AgentCooldownPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectAgentCooldown.Publish(ctx, ep.client, payload)
}

// PublishGuildSuggested publishes a guild.formation.suggested event.
func (ep *EventPublisher) PublishGuildSuggested(ctx context.Context, payload GuildSuggestedPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectGuildSuggested.Publish(ctx, ep.client, payload)
}

// PublishGuildAutoJoined publishes a guild.formation.autojoined event.
func (ep *EventPublisher) PublishGuildAutoJoined(ctx context.Context, payload GuildAutoJoinedPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectGuildAutoJoined.Publish(ctx, ep.client, payload)
}

// --- Skill Progression Events ---

// PublishSkillProgression publishes a skill.progression.improved event.
func (ep *EventPublisher) PublishSkillProgression(ctx context.Context, payload SkillProgressionPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectSkillProgression.Publish(ctx, ep.client, payload)
}

// PublishSkillLevelUp publishes a skill.progression.levelup event.
func (ep *EventPublisher) PublishSkillLevelUp(ctx context.Context, payload SkillLevelUpPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectSkillLevelUp.Publish(ctx, ep.client, payload)
}

// PublishMentorBonus publishes a skill.progression.mentorbonus event.
func (ep *EventPublisher) PublishMentorBonus(ctx context.Context, payload MentorBonusPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectMentorBonus.Publish(ctx, ep.client, payload)
}

// --- Party Coordination Events ---

// PublishPartyQuestDecomposed publishes a party.coordination.decomposed event.
func (ep *EventPublisher) PublishPartyQuestDecomposed(ctx context.Context, payload PartyQuestDecomposedPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectPartyQuestDecomposed.Publish(ctx, ep.client, payload)
}

// PublishPartyTaskAssigned publishes a party.coordination.assigned event.
func (ep *EventPublisher) PublishPartyTaskAssigned(ctx context.Context, payload PartyTaskAssignedPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectPartyTaskAssigned.Publish(ctx, ep.client, payload)
}

// PublishPartyContextShared publishes a party.coordination.contextshared event.
func (ep *EventPublisher) PublishPartyContextShared(ctx context.Context, payload PartyContextSharedPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectPartyContextShared.Publish(ctx, ep.client, payload)
}

// PublishPartyGuidanceIssued publishes a party.coordination.guidance event.
func (ep *EventPublisher) PublishPartyGuidanceIssued(ctx context.Context, payload PartyGuidanceIssuedPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectPartyGuidanceIssued.Publish(ctx, ep.client, payload)
}

// PublishPartyProgressReported publishes a party.coordination.progress event.
func (ep *EventPublisher) PublishPartyProgressReported(ctx context.Context, payload PartyProgressReportedPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectPartyProgressReported.Publish(ctx, ep.client, payload)
}

// PublishPartyHelpRequested publishes a party.coordination.helprequest event.
func (ep *EventPublisher) PublishPartyHelpRequested(ctx context.Context, payload PartyHelpRequestedPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectPartyHelpRequested.Publish(ctx, ep.client, payload)
}

// PublishPartyResultSubmitted publishes a party.coordination.resultsubmitted event.
func (ep *EventPublisher) PublishPartyResultSubmitted(ctx context.Context, payload PartyResultSubmittedPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectPartyResultSubmitted.Publish(ctx, ep.client, payload)
}

// PublishPartyRollupStarted publishes a party.coordination.rollupstarted event.
func (ep *EventPublisher) PublishPartyRollupStarted(ctx context.Context, payload PartyRollupStartedPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectPartyRollupStarted.Publish(ctx, ep.client, payload)
}

// PublishPartyRollupCompleted publishes a party.coordination.rollupcompleted event.
func (ep *EventPublisher) PublishPartyRollupCompleted(ctx context.Context, payload PartyRollupCompletedPayload) error {
	if err := payload.Validate(); err != nil {
		return err
	}
	return SubjectPartyRollupCompleted.Publish(ctx, ep.client, payload)
}

// =============================================================================
// AGENT PROGRESSION PAYLOADS
// =============================================================================

// AgentXPPayload contains data for agent.progression.xp events.
type AgentXPPayload struct {
	AgentID     AgentID    `json:"agent_id"`
	QuestID     QuestID    `json:"quest_id"`
	Award       *XPAward   `json:"award,omitempty"`   // Set on success
	Penalty     *XPPenalty `json:"penalty,omitempty"` // Set on failure
	XPDelta     int64      `json:"xp_delta"`
	XPBefore    int64      `json:"xp_before"`
	XPAfter     int64      `json:"xp_after"`
	LevelBefore int        `json:"level_before"`
	LevelAfter  int        `json:"level_after"`
	Timestamp   time.Time  `json:"timestamp"`
	Trace       TraceInfo  `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
func (p *AgentXPPayload) Validate() error {
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.QuestID == "" {
		return errors.New("quest_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// AgentLevelPayload contains data for agent.progression.levelup/leveldown events.
type AgentLevelPayload struct {
	AgentID   AgentID   `json:"agent_id"`
	QuestID   QuestID   `json:"quest_id"`
	OldLevel  int       `json:"old_level"`
	NewLevel  int       `json:"new_level"`
	OldTier   TrustTier `json:"old_tier"`
	NewTier   TrustTier `json:"new_tier"`
	XPCurrent int64     `json:"xp_current"`
	XPToLevel int64     `json:"xp_to_level"`
	Timestamp time.Time `json:"timestamp"`
	Trace     TraceInfo `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
func (p *AgentLevelPayload) Validate() error {
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// AgentCooldownPayload contains data for agent.progression.cooldown events.
type AgentCooldownPayload struct {
	AgentID       AgentID       `json:"agent_id"`
	QuestID       QuestID       `json:"quest_id"`
	FailType      FailureType   `json:"fail_type"`
	CooldownUntil time.Time     `json:"cooldown_until"`
	Duration      time.Duration `json:"duration"`
	Timestamp     time.Time     `json:"timestamp"`
	Trace         TraceInfo     `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
func (p *AgentCooldownPayload) Validate() error {
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// =============================================================================
// GUILD FORMATION PAYLOADS
// =============================================================================

// GuildSuggestion represents a proposed guild from skill clustering.
type GuildSuggestion struct {
	PrimarySkill    SkillTag   `json:"primary_skill"`
	SecondarySkills []SkillTag `json:"secondary_skills,omitempty"`
	AgentIDs        []AgentID  `json:"agent_ids"`
	ClusterStrength float64    `json:"cluster_strength"` // 0-1, average Jaccard similarity
	MinLevel        int        `json:"min_level"`
	SuggestedName   string     `json:"suggested_name"`
}

// GuildMatch indicates an agent qualifies for a guild.
type GuildMatch struct {
	GuildID       GuildID    `json:"guild_id"`
	GuildInstance string     `json:"guild_instance"` // Storage instance key (may differ from GuildID)
	AgentID       AgentID    `json:"agent_id"`
	MatchScore    float64    `json:"match_score"` // 0-1, skill overlap
	SkillsMatched []SkillTag `json:"skills_matched"`
	CanAutoJoin   bool       `json:"can_auto_join"` // Guild.AutoRecruit == true
}

// GuildSuggestedPayload contains data for guild.formation.suggested events.
type GuildSuggestedPayload struct {
	Suggestion GuildSuggestion `json:"suggestion"`
	ApprovalID string          `json:"approval_id,omitempty"` // For DM approval workflow
	Timestamp  time.Time       `json:"timestamp"`
	Trace      TraceInfo       `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
func (p *GuildSuggestedPayload) Validate() error {
	if p.Suggestion.PrimarySkill == "" {
		return errors.New("primary_skill required")
	}
	if len(p.Suggestion.AgentIDs) == 0 {
		return errors.New("agent_ids required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// GuildAutoJoinedPayload contains data for guild.formation.autojoined events.
type GuildAutoJoinedPayload struct {
	AgentID  AgentID   `json:"agent_id"`
	GuildID  GuildID   `json:"guild_id"`
	Rank     GuildRank `json:"rank"`
	JoinedAt time.Time `json:"joined_at"`
	Reason   string    `json:"reason,omitempty"` // Why auto-join triggered
	Trace    TraceInfo `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
func (p *GuildAutoJoinedPayload) Validate() error {
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.GuildID == "" {
		return errors.New("guild_id required")
	}
	if p.JoinedAt.IsZero() {
		return errors.New("joined_at required")
	}
	return nil
}

// =============================================================================
// PARTY COORDINATION PAYLOADS
// =============================================================================
// Communication events between party leads and members for quest coordination.
// Lead → Members: Decomposed, Assigned, ContextShared, Guidance
// Members → Lead: Progress, HelpRequest, ResultSubmitted
// Rollup: RollupStarted, RollupCompleted
// =============================================================================

// --- Lead → Members Payloads ---

// PartyQuestDecomposedPayload contains data for party.coordination.decomposed events.
// Emitted when a party lead breaks down the parent quest into sub-quests.
type PartyQuestDecomposedPayload struct {
	PartyID     PartyID   `json:"party_id"`
	LeadID      AgentID   `json:"lead_id"`
	ParentQuest QuestID   `json:"parent_quest"`
	SubQuests   []QuestID `json:"sub_quests"`
	Strategy    string    `json:"strategy"` // How lead approached decomposition
	Timestamp   time.Time `json:"timestamp"`
	Trace       TraceInfo `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
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

// PartyTaskAssignedPayload contains data for party.coordination.assigned events.
// Emitted when a party lead assigns a sub-quest to a member.
type PartyTaskAssignedPayload struct {
	PartyID      PartyID   `json:"party_id"`
	LeadID       AgentID   `json:"lead_id"`
	AssignedTo   AgentID   `json:"assigned_to"`
	SubQuestID   QuestID   `json:"sub_quest_id"`
	Rationale    string    `json:"rationale"`              // Why this member
	Dependencies []QuestID `json:"dependencies,omitempty"` // Wait for these first
	Guidance     string    `json:"guidance,omitempty"`     // Initial hints
	Timestamp    time.Time `json:"timestamp"`
	Trace        TraceInfo `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
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

// PartyGuidanceIssuedPayload contains data for party.coordination.guidance events.
// Emitted when a party lead provides guidance to a struggling member.
type PartyGuidanceIssuedPayload struct {
	PartyID      PartyID   `json:"party_id"`
	LeadID       AgentID   `json:"lead_id"`
	TargetMember AgentID   `json:"target_member"`
	SubQuestID   QuestID   `json:"sub_quest_id"`
	GuidanceType string    `json:"guidance_type"` // hint, redirect, resource
	Guidance     string    `json:"guidance"`
	Timestamp    time.Time `json:"timestamp"`
	Trace        TraceInfo `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
func (p *PartyGuidanceIssuedPayload) Validate() error {
	if p.PartyID == "" {
		return errors.New("party_id required")
	}
	if p.LeadID == "" {
		return errors.New("lead_id required")
	}
	if p.TargetMember == "" {
		return errors.New("target_member required")
	}
	if p.Guidance == "" {
		return errors.New("guidance required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}

// --- Members → Lead Payloads ---

// PartyProgressReportedPayload contains data for party.coordination.progress events.
// Emitted when a party member reports progress on their sub-quest.
type PartyProgressReportedPayload struct {
	PartyID         PartyID   `json:"party_id"`
	MemberID        AgentID   `json:"member_id"`
	SubQuestID      QuestID   `json:"sub_quest_id"`
	ProgressPercent int       `json:"progress_percent"` // 0-100
	Status          string    `json:"status"`           // on_track, blocked, ahead, behind
	Message         string    `json:"message,omitempty"`
	Timestamp       time.Time `json:"timestamp"`
	Trace           TraceInfo `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
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

// PartyHelpRequestedPayload contains data for party.coordination.helprequest events.
// Emitted when a party member needs help from the lead.
type PartyHelpRequestedPayload struct {
	PartyID     PartyID   `json:"party_id"`
	MemberID    AgentID   `json:"member_id"`
	SubQuestID  QuestID   `json:"sub_quest_id"`
	IssueType   string    `json:"issue_type"` // blocker, confusion, skill_gap
	Description string    `json:"description"`
	Urgency     string    `json:"urgency"` // low, medium, high, critical
	Timestamp   time.Time `json:"timestamp"`
	Trace       TraceInfo `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
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

// PartyResultSubmittedPayload contains data for party.coordination.resultsubmitted events.
// Emitted when a party member submits their sub-quest result to the lead.
type PartyResultSubmittedPayload struct {
	PartyID      PartyID   `json:"party_id"`
	MemberID     AgentID   `json:"member_id"`
	SubQuestID   QuestID   `json:"sub_quest_id"`
	Result       any       `json:"result"`
	QualityScore float64   `json:"quality_score,omitempty"` // Self-assessed or from pre-review
	Timestamp    time.Time `json:"timestamp"`
	Trace        TraceInfo `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
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

// --- Shared / Rollup Payloads ---

// PartyContextSharedPayload contains data for party.coordination.contextshared events.
// Emitted when context/insight is shared with the party.
type PartyContextSharedPayload struct {
	PartyID     PartyID     `json:"party_id"`
	SharedBy    AgentID     `json:"shared_by"`
	ContextItem ContextItem `json:"context_item"`
	Relevance   []QuestID   `json:"relevance,omitempty"` // Which sub-quests this affects
	Timestamp   time.Time   `json:"timestamp"`
	Trace       TraceInfo   `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
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

// PartyRollupStartedPayload contains data for party.coordination.rollupstarted events.
// Emitted when the party lead begins combining sub-results.
type PartyRollupStartedPayload struct {
	PartyID         PartyID   `json:"party_id"`
	LeadID          AgentID   `json:"lead_id"`
	ParentQuestID   QuestID   `json:"parent_quest_id"`
	SubResultsCount int       `json:"sub_results_count"`
	Timestamp       time.Time `json:"timestamp"`
	Trace           TraceInfo `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
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

// PartyRollupCompletedPayload contains data for party.coordination.rollupcompleted events.
// Emitted when the party lead completes the rollup, ready for boss battle.
type PartyRollupCompletedPayload struct {
	PartyID       PartyID             `json:"party_id"`
	LeadID        AgentID             `json:"lead_id"`
	ParentQuestID QuestID             `json:"parent_quest_id"`
	RollupResult  any                 `json:"rollup_result"`
	MemberContrib map[AgentID]float64 `json:"member_contributions,omitempty"` // Contribution scores
	Timestamp     time.Time           `json:"timestamp"`
	Trace         TraceInfo           `json:"trace,omitempty"`
}

// Validate checks that required fields are present.
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
