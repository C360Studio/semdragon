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
	// Quest lifecycle subjects
	SubjectQuestPosted    = natsclient.NewSubject[QuestPostedPayload](PredicateQuestPosted)
	SubjectQuestClaimed   = natsclient.NewSubject[QuestClaimedPayload](PredicateQuestClaimed)
	SubjectQuestStarted   = natsclient.NewSubject[QuestStartedPayload](PredicateQuestStarted)
	SubjectQuestSubmitted = natsclient.NewSubject[QuestSubmittedPayload](PredicateQuestSubmitted)
	SubjectQuestCompleted = natsclient.NewSubject[QuestCompletedPayload](PredicateQuestCompleted)
	SubjectQuestFailed    = natsclient.NewSubject[QuestFailedPayload](PredicateQuestFailed)
	SubjectQuestEscalated = natsclient.NewSubject[QuestEscalatedPayload](PredicateQuestEscalated)
	SubjectQuestAbandoned = natsclient.NewSubject[QuestAbandonedPayload](PredicateQuestAbandoned)

	// Boss battle subjects
	SubjectBattleStarted = natsclient.NewSubject[BattleStartedPayload](PredicateBattleStarted)
	SubjectBattleVerdict = natsclient.NewSubject[BattleVerdictPayload](PredicateBattleVerdict)

	// Agent progression subjects
	SubjectAgentXP        = natsclient.NewSubject[AgentXPPayload](PredicateAgentXP)
	SubjectAgentLevelUp   = natsclient.NewSubject[AgentLevelPayload](PredicateAgentLevelUp)
	SubjectAgentLevelDown = natsclient.NewSubject[AgentLevelPayload](PredicateAgentLevelDown)
	SubjectAgentCooldown  = natsclient.NewSubject[AgentCooldownPayload](PredicateAgentCooldown)
)

// --- Payload Types ---

// QuestPostedPayload contains data for quest.lifecycle.posted events.
type QuestPostedPayload struct {
	Quest    Quest     `json:"quest"`
	PostedAt time.Time `json:"posted_at"`
	PostedBy string    `json:"posted_by,omitempty"` // DM or system identifier
}

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
}

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
}

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
}

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
}

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
}

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
}

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
}

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
}

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
}

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

// =============================================================================
// AGENT PROGRESSION PAYLOADS
// =============================================================================

// AgentXPPayload contains data for agent.progression.xp events.
type AgentXPPayload struct {
	AgentID     AgentID   `json:"agent_id"`
	QuestID     QuestID   `json:"quest_id"`
	Award       *XPAward  `json:"award,omitempty"`   // Set on success
	Penalty     *XPPenalty `json:"penalty,omitempty"` // Set on failure
	XPDelta     int64     `json:"xp_delta"`
	XPBefore    int64     `json:"xp_before"`
	XPAfter     int64     `json:"xp_after"`
	LevelBefore int       `json:"level_before"`
	LevelAfter  int       `json:"level_after"`
	Timestamp   time.Time `json:"timestamp"`
}

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
}

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
}

func (p *AgentCooldownPayload) Validate() error {
	if p.AgentID == "" {
		return errors.New("agent_id required")
	}
	if p.Timestamp.IsZero() {
		return errors.New("timestamp required")
	}
	return nil
}
