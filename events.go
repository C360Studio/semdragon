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
