package questboard

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/pkg/errs"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// QUEST BOARD OPERATIONS
// =============================================================================

// GraphClient returns the underlying graph client for external access.
func (c *Component) GraphClient() *semdragons.GraphClient {
	return c.graph
}

// BoardConfig returns the board configuration.
func (c *Component) BoardConfig() *semdragons.BoardConfig {
	return c.boardConfig
}

// PostQuest adds a new quest to the board.
func (c *Component) PostQuest(ctx context.Context, quest Quest) (*Quest, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	// Generate instance ID if not provided
	instance := semdragons.ExtractInstance(string(quest.ID))
	if instance == "" || instance == string(quest.ID) {
		instance = semdragons.GenerateInstance()
	}

	// Set full entity ID
	quest.ID = domain.QuestID(c.boardConfig.QuestEntityID(instance))

	// Create trace context for this quest
	var tc = c.traces.StartQuestTrace(semdragons.QuestID(quest.ID))
	if quest.ParentQuest != nil {
		tc = c.traces.StartQuestTraceWithParent(semdragons.QuestID(quest.ID), semdragons.QuestID(*quest.ParentQuest))
	}
	quest.TrajectoryID = tc.TraceID

	// Set defaults
	quest.Status = domain.QuestPosted
	quest.PostedAt = time.Now()
	if quest.MaxAttempts == 0 {
		quest.MaxAttempts = c.config.DefaultMaxAttempts
	}
	if quest.BaseXP == 0 {
		quest.BaseXP = domain.DefaultXPForDifficulty(quest.Difficulty)
	}
	if quest.MinTier == 0 {
		quest.MinTier = domain.TierFromDifficulty(quest.Difficulty)
	}

	// Emit quest to graph system
	if err := c.graph.EmitEntity(ctx, &quest, "quest.posted"); err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "QuestBoard", "PostQuest", "emit quest")
	}

	// Emit lifecycle event with trace context using local typed subject
	if err := SubjectQuestPosted.Publish(ctx, c.deps.NATSClient, QuestPostedPayload{
		Quest:    quest,
		PostedAt: quest.PostedAt,
		Trace:    TraceInfo{TrajectoryID: tc.TraceID, SpanID: tc.SpanID},
	}); err != nil {
		c.logger.Debug("failed to publish quest posted event", "quest", quest.ID, "error", err)
	}

	return &quest, nil
}

// PostSubQuests decomposes a parent quest into sub-quests.
func (c *Component) PostSubQuests(ctx context.Context, parentID domain.QuestID, subQuests []Quest, decomposer domain.AgentID) ([]Quest, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	// Load parent quest from graph
	parent, err := c.getQuestByID(ctx, parentID)
	if err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "QuestBoard", "PostSubQuests", "load parent")
	}

	if parent.Status != domain.QuestClaimed && parent.Status != domain.QuestInProgress {
		return nil, fmt.Errorf("parent must be claimed or in_progress")
	}

	// Validate decomposer permissions
	agent, err := c.getAgentByID(ctx, semdragons.AgentID(decomposer))
	if err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "QuestBoard", "PostSubQuests", "load decomposer")
	}

	perms := semdragons.TierPerms[semdragons.TierFromLevel(agent.Level)]
	if !perms.CanDecomposeQuest {
		return nil, errors.New("agent cannot decompose quests (requires Master+ tier)")
	}

	// Post each sub-quest
	posted := make([]Quest, 0, len(subQuests))
	subQuestIDs := make([]domain.QuestID, 0, len(subQuests))

	for _, sq := range subQuests {
		sq.ParentQuest = &parentID
		sq.DecomposedBy = &decomposer

		result, err := c.PostQuest(ctx, sq)
		if err != nil {
			c.errorsCount.Add(1)
			return nil, errs.Wrap(err, "QuestBoard", "PostSubQuests", "post sub-quest")
		}

		posted = append(posted, *result)
		subQuestIDs = append(subQuestIDs, result.ID)
	}

	// Update parent with sub-quest IDs
	parent.SubQuests = subQuestIDs
	parent.DecomposedBy = &decomposer
	if err := c.graph.EmitEntityUpdate(ctx, parent, "quest.decomposed"); err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "QuestBoard", "PostSubQuests", "update parent")
	}

	return posted, nil
}

// QuestFilter specifies filtering options for quest queries.
type QuestFilter struct {
	Limit         int                     `json:"limit"`
	MinDifficulty *domain.QuestDifficulty `json:"min_difficulty,omitempty"`
	MaxDifficulty *domain.QuestDifficulty `json:"max_difficulty,omitempty"`
	GuildID       *domain.GuildID         `json:"guild_id,omitempty"`
	Skills        []domain.SkillTag       `json:"skills,omitempty"`
	PartyOnly     *bool                   `json:"party_only,omitempty"`
}

// AvailableQuests returns quests an agent is eligible to claim.
func (c *Component) AvailableQuests(ctx context.Context, agentID domain.AgentID, opts QuestFilter) ([]Quest, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())

	agent, err := c.getAgentByID(ctx, semdragons.AgentID(agentID))
	if err != nil {
		return nil, errs.Wrap(err, "QuestBoard", "AvailableQuests", "load agent")
	}

	// Check agent can claim
	if agent.Status != semdragons.AgentIdle {
		return []Quest{}, nil
	}
	if agent.CooldownUntil != nil && time.Now().Before(*agent.CooldownUntil) {
		return []Quest{}, nil
	}

	// Query quests by status predicate from graph
	questIDs, err := c.graph.QueryByPredicate(ctx, "quest.status.state")
	if err != nil {
		return nil, errs.Wrap(err, "QuestBoard", "AvailableQuests", "query quests")
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}

	agentTier := semdragons.TierFromLevel(agent.Level)
	available := make([]Quest, 0, limit)

	// Fetch entities and filter
	entities, err := c.graph.BatchGet(ctx, questIDs)
	if err != nil {
		return nil, errs.Wrap(err, "QuestBoard", "AvailableQuests", "batch get")
	}

	for _, entity := range entities {
		if len(available) >= limit {
			break
		}

		quest := c.questFromEntity(&entity)
		if quest == nil {
			continue
		}

		// Filter checks
		if quest.Status != domain.QuestPosted {
			continue
		}
		if domain.TrustTier(agentTier) < quest.MinTier {
			continue
		}
		if quest.PartyRequired && (opts.PartyOnly == nil || !*opts.PartyOnly) {
			continue
		}
		if opts.MinDifficulty != nil && quest.Difficulty < *opts.MinDifficulty {
			continue
		}
		if opts.MaxDifficulty != nil && quest.Difficulty > *opts.MaxDifficulty {
			continue
		}
		if opts.GuildID != nil {
			if quest.GuildPriority == nil || *quest.GuildPriority != *opts.GuildID {
				continue
			}
		}

		// Check skills match
		if len(opts.Skills) > 0 {
			hasSkill := false
			for _, reqSkill := range opts.Skills {
				if slices.Contains(quest.RequiredSkills, reqSkill) {
					hasSkill = true
					break
				}
			}
			if !hasSkill {
				continue
			}
		}

		available = append(available, *quest)
	}

	return available, nil
}

// ClaimQuest assigns a quest to an agent.
func (c *Component) ClaimQuest(ctx context.Context, questID domain.QuestID, agentID domain.AgentID) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	// Load agent
	agent, err := c.getAgentByID(ctx, semdragons.AgentID(agentID))
	if err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "ClaimQuest", "load agent")
	}

	// Load quest
	quest, err := c.getQuestByID(ctx, questID)
	if err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "ClaimQuest", "load quest")
	}

	if quest.Status != domain.QuestPosted {
		return fmt.Errorf("quest not available: %s", quest.Status)
	}

	if err := c.validateAgentCanClaim(agent, quest); err != nil {
		return err
	}

	// Update quest state
	now := time.Now()
	quest.Status = domain.QuestClaimed
	quest.ClaimedBy = &agentID
	quest.ClaimedAt = &now
	quest.Attempts++

	// Emit updated quest to graph
	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.claimed"); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "ClaimQuest", "emit update")
	}

	// Create span for claim event and emit
	_, tc := c.traces.NewEventSpan(ctx, semdragons.QuestID(questID))
	if err := SubjectQuestClaimed.Publish(ctx, c.deps.NATSClient, QuestClaimedPayload{
		Quest:     *quest,
		AgentID:   agentID,
		ClaimedAt: *quest.ClaimedAt,
		Trace:     TraceInfo{TrajectoryID: tc.TraceID, SpanID: tc.SpanID},
	}); err != nil {
		c.logger.Debug("failed to publish quest claimed event", "quest", quest.ID, "error", err)
	}

	return nil
}

// ClaimQuestForParty assigns a quest to a party.
func (c *Component) ClaimQuestForParty(ctx context.Context, questID domain.QuestID, partyID domain.PartyID) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	party, err := c.getPartyByID(ctx, semdragons.PartyID(partyID))
	if err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "ClaimQuestForParty", "load party")
	}

	agent, err := c.getAgentByID(ctx, party.Lead)
	if err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "ClaimQuestForParty", "load lead")
	}

	perms := semdragons.TierPerms[semdragons.TierFromLevel(agent.Level)]
	if !perms.CanLeadParty {
		return errors.New("party lead cannot lead parties (requires Master+ tier)")
	}

	quest, err := c.getQuestByID(ctx, questID)
	if err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "ClaimQuestForParty", "load quest")
	}

	if quest.Status != domain.QuestPosted {
		return fmt.Errorf("quest not available: %s", quest.Status)
	}

	if quest.MinPartySize > 0 && len(party.Members) < quest.MinPartySize {
		return errors.New("party too small")
	}

	if domain.TrustTier(semdragons.TierFromLevel(agent.Level)) < quest.MinTier {
		return errors.New("party lead tier too low")
	}

	// Update quest state
	now := time.Now()
	leadAgentID := domain.AgentID(party.Lead)
	quest.Status = domain.QuestClaimed
	quest.ClaimedBy = &leadAgentID
	quest.PartyID = &partyID
	quest.ClaimedAt = &now
	quest.Attempts++

	// Emit updated quest to graph
	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.claimed"); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "ClaimQuestForParty", "emit update")
	}

	// Create span for claim event
	_, tc := c.traces.NewEventSpan(ctx, semdragons.QuestID(questID))
	if err := SubjectQuestClaimed.Publish(ctx, c.deps.NATSClient, QuestClaimedPayload{
		Quest:     *quest,
		AgentID:   leadAgentID,
		PartyID:   &partyID,
		ClaimedAt: *quest.ClaimedAt,
		Trace:     TraceInfo{TrajectoryID: tc.TraceID, SpanID: tc.SpanID},
	}); err != nil {
		c.logger.Debug("failed to publish quest claimed event", "quest", quest.ID, "error", err)
	}

	return nil
}

// AbandonQuest returns a quest to the board.
func (c *Component) AbandonQuest(ctx context.Context, questID domain.QuestID, reason string) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	quest, err := c.getQuestByID(ctx, questID)
	if err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "AbandonQuest", "load quest")
	}

	if quest.Status != domain.QuestClaimed && quest.Status != domain.QuestInProgress {
		return fmt.Errorf("quest not abandonable: %s", quest.Status)
	}

	var agentID domain.AgentID
	var partyID *domain.PartyID

	if quest.ClaimedBy != nil {
		agentID = *quest.ClaimedBy
	}
	partyID = quest.PartyID

	// Reset quest state
	quest.Status = domain.QuestPosted
	quest.ClaimedBy = nil
	quest.PartyID = nil
	quest.ClaimedAt = nil
	quest.StartedAt = nil

	// Emit updated quest to graph
	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.abandoned"); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "AbandonQuest", "emit update")
	}

	// Create span for abandon event
	_, abandonTC := c.traces.NewEventSpan(ctx, semdragons.QuestID(questID))
	if err := SubjectQuestAbandoned.Publish(ctx, c.deps.NATSClient, QuestAbandonedPayload{
		Quest:       *quest,
		AgentID:     agentID,
		PartyID:     partyID,
		Reason:      reason,
		AbandonedAt: time.Now(),
		Trace:       TraceInfo{TrajectoryID: abandonTC.TraceID, SpanID: abandonTC.SpanID},
	}); err != nil {
		c.logger.Debug("failed to publish quest abandoned event", "quest", quest.ID, "error", err)
	}

	return nil
}

// StartQuest marks a quest as in-progress.
func (c *Component) StartQuest(ctx context.Context, questID domain.QuestID) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	quest, err := c.getQuestByID(ctx, questID)
	if err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "StartQuest", "load quest")
	}

	if quest.Status != domain.QuestClaimed {
		return fmt.Errorf("quest not claimed: %s", quest.Status)
	}

	var agentID domain.AgentID
	var partyID *domain.PartyID

	if quest.ClaimedBy != nil {
		agentID = *quest.ClaimedBy
	}
	partyID = quest.PartyID

	// Update quest state
	now := time.Now()
	quest.Status = domain.QuestInProgress
	quest.StartedAt = &now

	// Emit updated quest to graph
	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.started"); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "StartQuest", "emit update")
	}

	_, tc := c.traces.NewEventSpan(ctx, semdragons.QuestID(questID))
	if err := SubjectQuestStarted.Publish(ctx, c.deps.NATSClient, QuestStartedPayload{
		Quest:     *quest,
		AgentID:   agentID,
		PartyID:   partyID,
		StartedAt: *quest.StartedAt,
		Trace:     TraceInfo{TrajectoryID: tc.TraceID, SpanID: tc.SpanID},
	}); err != nil {
		c.logger.Debug("failed to publish quest started event", "quest", quest.ID, "error", err)
	}

	return nil
}

// SubmitResult submits quest output for review.
// If the quest requires review, the bossbattle processor will handle battle creation
// when it receives the QuestSubmitted event.
func (c *Component) SubmitResult(ctx context.Context, questID domain.QuestID, result any) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	quest, err := c.getQuestByID(ctx, questID)
	if err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "SubmitResult", "load quest")
	}

	if quest.Status != domain.QuestInProgress {
		return fmt.Errorf("quest not in_progress: %s", quest.Status)
	}

	var agentID domain.AgentID
	if quest.ClaimedBy != nil {
		agentID = *quest.ClaimedBy
	}

	quest.Output = result
	needsReview := quest.Constraints.RequireReview

	if needsReview {
		quest.Status = domain.QuestInReview
	} else {
		now := time.Now()
		quest.Status = domain.QuestCompleted
		quest.CompletedAt = &now
	}

	// Emit updated quest to graph
	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.submitted"); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "SubmitResult", "emit update")
	}

	// Publish quest submitted event - bossbattle processor will create battle if needed
	_, submitTC := c.traces.NewEventSpan(ctx, semdragons.QuestID(questID))
	if err := SubjectQuestSubmitted.Publish(ctx, c.deps.NATSClient, QuestSubmittedPayload{
		Quest:        *quest,
		AgentID:      agentID,
		Result:       result,
		SubmittedAt:  time.Now(),
		NeedsReview:  needsReview,
		ReviewLevel:  quest.Constraints.ReviewLevel,
		Trace:        TraceInfo{TrajectoryID: submitTC.TraceID, SpanID: submitTC.SpanID},
	}); err != nil {
		c.logger.Debug("failed to publish quest submitted event", "quest", quest.ID, "error", err)
	}

	// If quest completed directly (no review), end the trace
	if !needsReview {
		c.traces.EndQuestTrace(semdragons.QuestID(questID))
	}

	return nil
}

// CompleteQuest marks a quest as successfully completed.
func (c *Component) CompleteQuest(ctx context.Context, questID domain.QuestID, verdict BattleVerdict) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	quest, err := c.getQuestByID(ctx, questID)
	if err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "CompleteQuest", "load quest")
	}

	if quest.Status != domain.QuestInReview && quest.Status != domain.QuestInProgress {
		return fmt.Errorf("quest not completable: %s", quest.Status)
	}

	var agentID domain.AgentID
	var partyID *domain.PartyID
	var duration time.Duration

	if quest.ClaimedBy != nil {
		agentID = *quest.ClaimedBy
	}
	partyID = quest.PartyID

	now := time.Now()
	quest.Status = domain.QuestCompleted
	quest.CompletedAt = &now

	if quest.StartedAt != nil {
		duration = now.Sub(*quest.StartedAt)
	}

	// Emit updated quest to graph
	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.completed"); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "CompleteQuest", "emit update")
	}

	// Create final span for completion event
	_, completeTC := c.traces.NewEventSpan(ctx, semdragons.QuestID(questID))
	if err := SubjectQuestCompleted.Publish(ctx, c.deps.NATSClient, QuestCompletedPayload{
		Quest:       *quest,
		AgentID:     agentID,
		PartyID:     partyID,
		Verdict:     verdict,
		CompletedAt: *quest.CompletedAt,
		Duration:    duration,
		Trace:       TraceInfo{TrajectoryID: completeTC.TraceID, SpanID: completeTC.SpanID},
	}); err != nil {
		c.logger.Debug("failed to publish quest completed event", "quest", quest.ID, "error", err)
	}

	// End trace for this quest (terminal state)
	c.traces.EndQuestTrace(semdragons.QuestID(questID))

	return nil
}

// FailQuest marks a quest as failed.
func (c *Component) FailQuest(ctx context.Context, questID domain.QuestID, reason string) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	quest, err := c.getQuestByID(ctx, questID)
	if err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "FailQuest", "load quest")
	}

	if quest.Status != domain.QuestInProgress && quest.Status != domain.QuestInReview {
		return fmt.Errorf("quest not failable: %s", quest.Status)
	}

	var agentID domain.AgentID
	var partyID *domain.PartyID
	var reposted bool

	if quest.ClaimedBy != nil {
		agentID = *quest.ClaimedBy
	}
	partyID = quest.PartyID

	if quest.Attempts < quest.MaxAttempts {
		quest.Status = domain.QuestPosted
		quest.ClaimedBy = nil
		quest.PartyID = nil
		quest.ClaimedAt = nil
		quest.StartedAt = nil
		quest.Output = nil
		reposted = true
	} else {
		quest.Status = domain.QuestFailed
	}

	// Emit updated quest to graph
	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.failed"); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "FailQuest", "emit update")
	}

	// Create span for failure event
	_, failTC := c.traces.NewEventSpan(ctx, semdragons.QuestID(questID))
	if err := SubjectQuestFailed.Publish(ctx, c.deps.NATSClient, QuestFailedPayload{
		Quest:    *quest,
		AgentID:  agentID,
		PartyID:  partyID,
		Reason:   reason,
		FailType: FailureQuality,
		FailedAt: time.Now(),
		Attempt:  quest.Attempts,
		Reposted: reposted,
		Trace:    TraceInfo{TrajectoryID: failTC.TraceID, SpanID: failTC.SpanID},
	}); err != nil {
		c.logger.Debug("failed to publish quest failed event", "quest", quest.ID, "error", err)
	}

	// End trace if quest reached terminal state (not reposted)
	if !reposted {
		c.traces.EndQuestTrace(semdragons.QuestID(questID))
	}

	return nil
}

// EscalateQuest flags a quest for higher-level attention.
func (c *Component) EscalateQuest(ctx context.Context, questID domain.QuestID, reason string) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	quest, err := c.getQuestByID(ctx, questID)
	if err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "EscalateQuest", "load quest")
	}

	if quest.Status == domain.QuestCompleted || quest.Status == domain.QuestCancelled || quest.Status == domain.QuestEscalated {
		return fmt.Errorf("quest cannot be escalated: %s", quest.Status)
	}

	var agentID domain.AgentID
	var partyID *domain.PartyID

	if quest.ClaimedBy != nil {
		agentID = *quest.ClaimedBy
	}
	partyID = quest.PartyID

	quest.Status = domain.QuestEscalated
	quest.Escalated = true

	// Emit updated quest to graph
	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.escalated"); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "EscalateQuest", "emit update")
	}

	// Create span for escalation event
	_, escTC := c.traces.NewEventSpan(ctx, semdragons.QuestID(questID))
	if err := SubjectQuestEscalated.Publish(ctx, c.deps.NATSClient, QuestEscalatedPayload{
		Quest:       *quest,
		AgentID:     agentID,
		PartyID:     partyID,
		Reason:      reason,
		EscalatedAt: time.Now(),
		Attempts:    quest.Attempts,
		Trace:       TraceInfo{TrajectoryID: escTC.TraceID, SpanID: escTC.SpanID},
	}); err != nil {
		c.logger.Debug("failed to publish quest escalated event", "quest", quest.ID, "error", err)
	}

	// End trace for escalated quest (terminal state requiring DM attention)
	c.traces.EndQuestTrace(semdragons.QuestID(questID))

	return nil
}

// GetQuest returns a quest by ID.
func (c *Component) GetQuest(ctx context.Context, questID domain.QuestID) (*Quest, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	return c.getQuestByID(ctx, questID)
}

// BoardStats represents current board statistics.
type BoardStats struct {
	TotalPosted     int                            `json:"total_posted"`
	TotalClaimed    int                            `json:"total_claimed"`
	TotalInProgress int                            `json:"total_in_progress"`
	TotalCompleted  int                            `json:"total_completed"`
	TotalFailed     int                            `json:"total_failed"`
	TotalEscalated  int                            `json:"total_escalated"`
	ByDifficulty    map[domain.QuestDifficulty]int `json:"by_difficulty"`
	BySkill         map[domain.SkillTag]int        `json:"by_skill"`
}

// BoardStats returns current board statistics.
func (c *Component) BoardStats(ctx context.Context) (*BoardStats, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	// Query all quests from graph
	entities, err := c.graph.ListQuestsByPrefix(ctx, 1000)
	if err != nil {
		return nil, errs.Wrap(err, "QuestBoard", "BoardStats", "list quests")
	}

	stats := &BoardStats{
		ByDifficulty: make(map[domain.QuestDifficulty]int),
		BySkill:      make(map[domain.SkillTag]int),
	}

	for _, entity := range entities {
		quest := c.questFromEntity(&entity)
		if quest == nil {
			continue
		}

		switch quest.Status {
		case domain.QuestPosted:
			stats.TotalPosted++
		case domain.QuestClaimed:
			stats.TotalClaimed++
		case domain.QuestInProgress:
			stats.TotalInProgress++
		case domain.QuestCompleted:
			stats.TotalCompleted++
		case domain.QuestFailed:
			stats.TotalFailed++
		case domain.QuestEscalated:
			stats.TotalEscalated++
		}

		stats.ByDifficulty[quest.Difficulty]++
		for _, skill := range quest.RequiredSkills {
			stats.BySkill[skill]++
		}
	}

	return stats, nil
}

// =============================================================================
// HELPERS
// =============================================================================

// getQuestByID retrieves a quest from the graph and reconstructs it.
func (c *Component) getQuestByID(ctx context.Context, questID domain.QuestID) (*Quest, error) {
	entity, err := c.graph.GetQuest(ctx, semdragons.QuestID(questID))
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, fmt.Errorf("quest not found: %s", questID)
	}
	return c.questFromEntity(entity), nil
}

// getAgentByID retrieves an agent from the graph and reconstructs it.
func (c *Component) getAgentByID(ctx context.Context, agentID semdragons.AgentID) (*semdragons.Agent, error) {
	entity, err := c.graph.GetAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}
	return semdragons.AgentFromEntityState(entity), nil
}

// getPartyByID retrieves a party from the graph and reconstructs it.
func (c *Component) getPartyByID(ctx context.Context, partyID semdragons.PartyID) (*semdragons.Party, error) {
	entity, err := c.graph.GetParty(ctx, partyID)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, fmt.Errorf("party not found: %s", partyID)
	}
	return semdragons.PartyFromEntityState(entity), nil
}

// questFromEntity reconstructs a Quest from a graph entity.
func (c *Component) questFromEntity(entity *graph.EntityState) *Quest {
	if entity == nil {
		return nil
	}

	quest := &Quest{
		ID: domain.QuestID(entity.ID),
	}

	for _, triple := range entity.Triples {
		switch triple.Predicate {
		case "quest.identity.title":
			if v, ok := triple.Object.(string); ok {
				quest.Title = v
			}
		case "quest.identity.description":
			if v, ok := triple.Object.(string); ok {
				quest.Description = v
			}
		case "quest.status.state":
			if v, ok := triple.Object.(string); ok {
				quest.Status = domain.QuestStatus(v)
			}
		case "quest.difficulty.level":
			if v, ok := triple.Object.(float64); ok {
				quest.Difficulty = domain.QuestDifficulty(int(v))
			}
		case "quest.tier.minimum":
			if v, ok := triple.Object.(float64); ok {
				quest.MinTier = domain.TrustTier(int(v))
			}
		case "quest.party.required":
			if v, ok := triple.Object.(bool); ok {
				quest.PartyRequired = v
			}
		case "quest.xp.base":
			if v, ok := triple.Object.(float64); ok {
				quest.BaseXP = int64(v)
			}
		case "quest.attempts.current":
			if v, ok := triple.Object.(float64); ok {
				quest.Attempts = int(v)
			}
		case "quest.attempts.max":
			if v, ok := triple.Object.(float64); ok {
				quest.MaxAttempts = int(v)
			}
		case "quest.skill.required":
			if v, ok := triple.Object.(string); ok {
				quest.RequiredSkills = append(quest.RequiredSkills, domain.SkillTag(v))
			}
		case "quest.assignment.agent":
			if v, ok := triple.Object.(string); ok {
				agentID := domain.AgentID(v)
				quest.ClaimedBy = &agentID
			}
		case "quest.assignment.party":
			if v, ok := triple.Object.(string); ok {
				partyID := domain.PartyID(v)
				quest.PartyID = &partyID
			}
		case "quest.priority.guild":
			if v, ok := triple.Object.(string); ok {
				guildID := domain.GuildID(v)
				quest.GuildPriority = &guildID
			}
		case "quest.parent.quest":
			if v, ok := triple.Object.(string); ok {
				parentID := domain.QuestID(v)
				quest.ParentQuest = &parentID
			}
		case "quest.observability.trajectory_id":
			if v, ok := triple.Object.(string); ok {
				quest.TrajectoryID = v
			}
		case "quest.review.level":
			if v, ok := triple.Object.(float64); ok {
				quest.Constraints.ReviewLevel = domain.ReviewLevel(int(v))
			}
		case "quest.lifecycle.posted_at":
			if v, ok := triple.Object.(string); ok {
				if t, err := time.Parse(time.RFC3339, v); err == nil {
					quest.PostedAt = t
				}
			}
		case "quest.lifecycle.claimed_at":
			if v, ok := triple.Object.(string); ok {
				if t, err := time.Parse(time.RFC3339, v); err == nil {
					quest.ClaimedAt = &t
				}
			}
		case "quest.lifecycle.started_at":
			if v, ok := triple.Object.(string); ok {
				if t, err := time.Parse(time.RFC3339, v); err == nil {
					quest.StartedAt = &t
				}
			}
		case "quest.lifecycle.completed_at":
			if v, ok := triple.Object.(string); ok {
				if t, err := time.Parse(time.RFC3339, v); err == nil {
					quest.CompletedAt = &t
				}
			}
		}
	}

	return quest
}

func (c *Component) validateAgentCanClaim(agent *semdragons.Agent, quest *Quest) error {
	if agent.Status != semdragons.AgentIdle {
		return fmt.Errorf("agent not idle: %s", agent.Status)
	}

	if agent.CooldownUntil != nil && time.Now().Before(*agent.CooldownUntil) {
		return errors.New("agent on cooldown")
	}

	if domain.TrustTier(semdragons.TierFromLevel(agent.Level)) < quest.MinTier {
		return errors.New("agent tier too low")
	}

	if quest.PartyRequired {
		return errors.New("quest requires party")
	}

	perms := semdragons.TierPerms[semdragons.TierFromLevel(agent.Level)]
	if agent.CurrentQuest != nil && perms.MaxConcurrent <= 1 {
		return errors.New("agent at concurrent quest limit")
	}

	if len(quest.RequiredSkills) > 0 {
		hasSkill := false
		for _, required := range quest.RequiredSkills {
			if agent.HasSkill(semdragons.SkillTag(required)) {
				hasSkill = true
				break
			}
		}
		if !hasSkill {
			return errors.New("agent lacks required skills")
		}
	}

	return nil
}

