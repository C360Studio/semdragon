package questboard

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/c360studio/semstreams/pkg/errs"

	semdragons "github.com/c360studio/semdragons"
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
func (c *Component) PostQuest(ctx context.Context, quest semdragons.Quest) (*semdragons.Quest, error) {
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
	quest.ID = semdragons.QuestID(c.boardConfig.QuestEntityID(instance))

	// Create trace context for this quest
	var tc = c.traces.StartQuestTrace(quest.ID)
	if quest.ParentQuest != nil {
		tc = c.traces.StartQuestTraceWithParent(quest.ID, *quest.ParentQuest)
	}
	quest.TrajectoryID = tc.TraceID

	// Set defaults
	quest.Status = semdragons.QuestPosted
	quest.PostedAt = time.Now()
	if quest.MaxAttempts == 0 {
		quest.MaxAttempts = c.config.DefaultMaxAttempts
	}
	if quest.BaseXP == 0 {
		quest.BaseXP = semdragons.DefaultXPForDifficulty(quest.Difficulty)
	}
	if quest.MinTier == 0 {
		quest.MinTier = semdragons.TierFromDifficulty(quest.Difficulty)
	}

	// Emit quest to graph system
	if err := c.graph.EmitEntity(ctx, &quest, "quest.posted"); err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "QuestBoard", "PostQuest", "emit quest")
	}

	// Emit lifecycle event with trace context
	if err := c.events.PublishQuestPosted(ctx, semdragons.QuestPostedPayload{
		Quest:    quest,
		PostedAt: quest.PostedAt,
		Trace:    semdragons.TraceInfoFromTraceContext(tc),
	}); err != nil {
		c.logger.Debug("failed to publish quest posted event", "quest", quest.ID, "error", err)
	}

	return &quest, nil
}

// PostSubQuests decomposes a parent quest into sub-quests.
func (c *Component) PostSubQuests(ctx context.Context, parentID semdragons.QuestID, subQuests []semdragons.Quest, decomposer semdragons.AgentID) ([]semdragons.Quest, error) {
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

	if parent.Status != semdragons.QuestClaimed && parent.Status != semdragons.QuestInProgress {
		return nil, fmt.Errorf("parent must be claimed or in_progress")
	}

	// Validate decomposer permissions
	agent, err := c.getAgentByID(ctx, decomposer)
	if err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "QuestBoard", "PostSubQuests", "load decomposer")
	}

	perms := semdragons.TierPerms[semdragons.TierFromLevel(agent.Level)]
	if !perms.CanDecomposeQuest {
		return nil, errors.New("agent cannot decompose quests (requires Master+ tier)")
	}

	// Post each sub-quest
	posted := make([]semdragons.Quest, 0, len(subQuests))
	subQuestIDs := make([]semdragons.QuestID, 0, len(subQuests))

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

// AvailableQuests returns quests an agent is eligible to claim.
func (c *Component) AvailableQuests(ctx context.Context, agentID semdragons.AgentID, opts semdragons.QuestFilter) ([]semdragons.Quest, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())

	agent, err := c.getAgentByID(ctx, agentID)
	if err != nil {
		return nil, errs.Wrap(err, "QuestBoard", "AvailableQuests", "load agent")
	}

	// Check agent can claim
	if agent.Status != semdragons.AgentIdle {
		return []semdragons.Quest{}, nil
	}
	if agent.CooldownUntil != nil && time.Now().Before(*agent.CooldownUntil) {
		return []semdragons.Quest{}, nil
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
	available := make([]semdragons.Quest, 0, limit)

	// Fetch entities and filter
	entities, err := c.graph.BatchGet(ctx, questIDs)
	if err != nil {
		return nil, errs.Wrap(err, "QuestBoard", "AvailableQuests", "batch get")
	}

	// Build guild priority map
	guildPriorityMap := make(map[string]bool)
	for _, guildID := range agent.Guilds {
		guildPriorityMap[string(guildID)] = true
	}

	for _, entity := range entities {
		if len(available) >= limit {
			break
		}

		quest := semdragons.QuestFromEntityState(&entity)
		if quest == nil {
			continue
		}

		// Filter checks
		if quest.Status != semdragons.QuestPosted {
			continue
		}
		if agentTier < quest.MinTier {
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
func (c *Component) ClaimQuest(ctx context.Context, questID semdragons.QuestID, agentID semdragons.AgentID) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	// Load agent
	agent, err := c.getAgentByID(ctx, agentID)
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

	if quest.Status != semdragons.QuestPosted {
		return fmt.Errorf("quest not available: %s", quest.Status)
	}

	if err := c.validateAgentCanClaim(agent, quest); err != nil {
		return err
	}

	// Update quest state
	now := time.Now()
	quest.Status = semdragons.QuestClaimed
	quest.ClaimedBy = &agentID
	quest.ClaimedAt = &now
	quest.Attempts++

	// Emit updated quest to graph
	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.claimed"); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "ClaimQuest", "emit update")
	}

	// Create span for claim event and emit
	_, tc := c.traces.NewEventSpan(ctx, questID)
	c.events.PublishQuestClaimed(ctx, semdragons.QuestClaimedPayload{
		Quest:     *quest,
		AgentID:   agentID,
		ClaimedAt: *quest.ClaimedAt,
		Trace:     semdragons.TraceInfoFromTraceContext(tc),
	})

	return nil
}

// ClaimQuestForParty assigns a quest to a party.
func (c *Component) ClaimQuestForParty(ctx context.Context, questID semdragons.QuestID, partyID semdragons.PartyID) error {
	if !c.running.Load() {
		return errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	party, err := c.getPartyByID(ctx, partyID)
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

	if quest.Status != semdragons.QuestPosted {
		return fmt.Errorf("quest not available: %s", quest.Status)
	}

	if quest.MinPartySize > 0 && len(party.Members) < quest.MinPartySize {
		return errors.New("party too small")
	}

	if semdragons.TierFromLevel(agent.Level) < quest.MinTier {
		return errors.New("party lead tier too low")
	}

	// Update quest state
	now := time.Now()
	quest.Status = semdragons.QuestClaimed
	quest.ClaimedBy = &party.Lead
	quest.PartyID = &partyID
	quest.ClaimedAt = &now
	quest.Attempts++

	// Emit updated quest to graph
	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.claimed"); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "ClaimQuestForParty", "emit update")
	}

	// Create span for claim event
	_, tc := c.traces.NewEventSpan(ctx, questID)
	c.events.PublishQuestClaimed(ctx, semdragons.QuestClaimedPayload{
		Quest:     *quest,
		AgentID:   party.Lead,
		PartyID:   &partyID,
		ClaimedAt: *quest.ClaimedAt,
		Trace:     semdragons.TraceInfoFromTraceContext(tc),
	})

	return nil
}

// AbandonQuest returns a quest to the board.
func (c *Component) AbandonQuest(ctx context.Context, questID semdragons.QuestID, reason string) error {
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

	if quest.Status != semdragons.QuestClaimed && quest.Status != semdragons.QuestInProgress {
		return fmt.Errorf("quest not abandonable: %s", quest.Status)
	}

	var agentID semdragons.AgentID
	var partyID *semdragons.PartyID

	if quest.ClaimedBy != nil {
		agentID = *quest.ClaimedBy
	}
	partyID = quest.PartyID

	// Reset quest state
	quest.Status = semdragons.QuestPosted
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
	_, abandonTC := c.traces.NewEventSpan(ctx, questID)
	c.events.PublishQuestAbandoned(ctx, semdragons.QuestAbandonedPayload{
		Quest:       *quest,
		AgentID:     agentID,
		PartyID:     partyID,
		Reason:      reason,
		AbandonedAt: time.Now(),
		Trace:       semdragons.TraceInfoFromTraceContext(abandonTC),
	})

	return nil
}

// StartQuest marks a quest as in-progress.
func (c *Component) StartQuest(ctx context.Context, questID semdragons.QuestID) error {
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

	if quest.Status != semdragons.QuestClaimed {
		return fmt.Errorf("quest not claimed: %s", quest.Status)
	}

	var agentID semdragons.AgentID
	var partyID *semdragons.PartyID

	if quest.ClaimedBy != nil {
		agentID = *quest.ClaimedBy
	}
	partyID = quest.PartyID

	// Update quest state
	now := time.Now()
	quest.Status = semdragons.QuestInProgress
	quest.StartedAt = &now

	// Emit updated quest to graph
	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.started"); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "StartQuest", "emit update")
	}

	_, tc := c.traces.NewEventSpan(ctx, questID)
	c.events.PublishQuestStarted(ctx, semdragons.QuestStartedPayload{
		Quest:     *quest,
		AgentID:   agentID,
		PartyID:   partyID,
		StartedAt: *quest.StartedAt,
		Trace:     semdragons.TraceInfoFromTraceContext(tc),
	})

	return nil
}

// SubmitResult submits quest output for review.
func (c *Component) SubmitResult(ctx context.Context, questID semdragons.QuestID, result any) (*semdragons.BossBattle, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	quest, err := c.getQuestByID(ctx, questID)
	if err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "QuestBoard", "SubmitResult", "load quest")
	}

	if quest.Status != semdragons.QuestInProgress {
		return nil, fmt.Errorf("quest not in_progress: %s", quest.Status)
	}

	var agentID semdragons.AgentID
	if quest.ClaimedBy != nil {
		agentID = *quest.ClaimedBy
	}

	quest.Output = result
	needsReview := quest.Constraints.RequireReview

	if needsReview {
		quest.Status = semdragons.QuestInReview
	} else {
		now := time.Now()
		quest.Status = semdragons.QuestCompleted
		quest.CompletedAt = &now
	}

	// Emit updated quest to graph
	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.submitted"); err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "QuestBoard", "SubmitResult", "emit update")
	}

	var battle *semdragons.BossBattle
	var battleID *semdragons.BattleID

	if needsReview {
		battle = c.createBossBattle(quest, agentID)
		id := battle.ID
		battleID = &id

		// Emit battle to graph
		if err := c.graph.EmitEntity(ctx, battle, "battle.started"); err != nil {
			c.logger.Debug("failed to emit battle", "battle", battle.ID, "error", err)
		}

		_, battleTC := c.traces.NewEventSpan(ctx, questID)
		c.events.PublishBattleStarted(ctx, semdragons.BattleStartedPayload{
			Battle:    *battle,
			Quest:     *quest,
			StartedAt: battle.StartedAt,
			Trace:     semdragons.TraceInfoFromTraceContext(battleTC),
		})
	}

	_, submitTC := c.traces.NewEventSpan(ctx, questID)
	c.events.PublishQuestSubmitted(ctx, semdragons.QuestSubmittedPayload{
		Quest:       *quest,
		AgentID:     agentID,
		Result:      result,
		SubmittedAt: time.Now(),
		BattleID:    battleID,
		Trace:       semdragons.TraceInfoFromTraceContext(submitTC),
	})

	// If quest completed directly (no review), end the trace
	if !needsReview {
		c.traces.EndQuestTrace(questID)
	}

	return battle, nil
}

// CompleteQuest marks a quest as successfully completed.
func (c *Component) CompleteQuest(ctx context.Context, questID semdragons.QuestID, verdict semdragons.BattleVerdict) error {
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

	if quest.Status != semdragons.QuestInReview && quest.Status != semdragons.QuestInProgress {
		return fmt.Errorf("quest not completable: %s", quest.Status)
	}

	var agentID semdragons.AgentID
	var partyID *semdragons.PartyID
	var duration time.Duration

	if quest.ClaimedBy != nil {
		agentID = *quest.ClaimedBy
	}
	partyID = quest.PartyID

	now := time.Now()
	quest.Status = semdragons.QuestCompleted
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
	_, completeTC := c.traces.NewEventSpan(ctx, questID)
	c.events.PublishQuestCompleted(ctx, semdragons.QuestCompletedPayload{
		Quest:       *quest,
		AgentID:     agentID,
		PartyID:     partyID,
		Verdict:     verdict,
		CompletedAt: *quest.CompletedAt,
		Duration:    duration,
		Trace:       semdragons.TraceInfoFromTraceContext(completeTC),
	})

	// End trace for this quest (terminal state)
	c.traces.EndQuestTrace(questID)

	return nil
}

// FailQuest marks a quest as failed.
func (c *Component) FailQuest(ctx context.Context, questID semdragons.QuestID, reason string) error {
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

	if quest.Status != semdragons.QuestInProgress && quest.Status != semdragons.QuestInReview {
		return fmt.Errorf("quest not failable: %s", quest.Status)
	}

	var agentID semdragons.AgentID
	var partyID *semdragons.PartyID
	var reposted bool

	if quest.ClaimedBy != nil {
		agentID = *quest.ClaimedBy
	}
	partyID = quest.PartyID

	if quest.Attempts < quest.MaxAttempts {
		quest.Status = semdragons.QuestPosted
		quest.ClaimedBy = nil
		quest.PartyID = nil
		quest.ClaimedAt = nil
		quest.StartedAt = nil
		quest.Output = nil
		reposted = true
	} else {
		quest.Status = semdragons.QuestFailed
	}

	// Emit updated quest to graph
	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.failed"); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "FailQuest", "emit update")
	}

	// Create span for failure event
	_, failTC := c.traces.NewEventSpan(ctx, questID)
	c.events.PublishQuestFailed(ctx, semdragons.QuestFailedPayload{
		Quest:    *quest,
		AgentID:  agentID,
		PartyID:  partyID,
		Reason:   reason,
		FailType: semdragons.FailureSoft,
		FailedAt: time.Now(),
		Attempt:  quest.Attempts,
		Reposted: reposted,
		Trace:    semdragons.TraceInfoFromTraceContext(failTC),
	})

	// End trace if quest reached terminal state (not reposted)
	if !reposted {
		c.traces.EndQuestTrace(questID)
	}

	return nil
}

// EscalateQuest flags a quest for higher-level attention.
func (c *Component) EscalateQuest(ctx context.Context, questID semdragons.QuestID, reason string) error {
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

	if quest.Status == semdragons.QuestCompleted || quest.Status == semdragons.QuestCancelled || quest.Status == semdragons.QuestEscalated {
		return fmt.Errorf("quest cannot be escalated: %s", quest.Status)
	}

	var agentID semdragons.AgentID
	var partyID *semdragons.PartyID

	if quest.ClaimedBy != nil {
		agentID = *quest.ClaimedBy
	}
	partyID = quest.PartyID

	quest.Status = semdragons.QuestEscalated
	quest.Escalated = true

	// Emit updated quest to graph
	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.escalated"); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "EscalateQuest", "emit update")
	}

	// Create span for escalation event
	_, escTC := c.traces.NewEventSpan(ctx, questID)
	c.events.PublishQuestEscalated(ctx, semdragons.QuestEscalatedPayload{
		Quest:       *quest,
		AgentID:     agentID,
		PartyID:     partyID,
		Reason:      reason,
		EscalatedAt: time.Now(),
		Attempts:    quest.Attempts,
		Trace:       semdragons.TraceInfoFromTraceContext(escTC),
	})

	// End trace for escalated quest (terminal state requiring DM attention)
	c.traces.EndQuestTrace(questID)

	return nil
}

// GetQuest returns a quest by ID.
func (c *Component) GetQuest(ctx context.Context, questID semdragons.QuestID) (*semdragons.Quest, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	return c.getQuestByID(ctx, questID)
}

// BoardStats returns current board statistics.
func (c *Component) BoardStats(ctx context.Context) (*semdragons.BoardStats, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	// Query all quests from graph
	entities, err := c.graph.ListQuestsByPrefix(ctx, 1000)
	if err != nil {
		return nil, errs.Wrap(err, "QuestBoard", "BoardStats", "list quests")
	}

	stats := &semdragons.BoardStats{
		ByDifficulty: make(map[semdragons.QuestDifficulty]int),
		BySkill:      make(map[semdragons.SkillTag]int),
	}

	for _, entity := range entities {
		quest := semdragons.QuestFromEntityState(&entity)
		if quest == nil {
			continue
		}

		switch quest.Status {
		case semdragons.QuestPosted:
			stats.TotalPosted++
		case semdragons.QuestClaimed:
			stats.TotalClaimed++
		case semdragons.QuestInProgress:
			stats.TotalInProgress++
		case semdragons.QuestCompleted:
			stats.TotalCompleted++
		case semdragons.QuestFailed:
			stats.TotalFailed++
		case semdragons.QuestEscalated:
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
func (c *Component) getQuestByID(ctx context.Context, questID semdragons.QuestID) (*semdragons.Quest, error) {
	entity, err := c.graph.GetQuest(ctx, questID)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, fmt.Errorf("quest not found: %s", questID)
	}
	return semdragons.QuestFromEntityState(entity), nil
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

func (c *Component) validateAgentCanClaim(agent *semdragons.Agent, quest *semdragons.Quest) error {
	if agent.Status != semdragons.AgentIdle {
		return fmt.Errorf("agent not idle: %s", agent.Status)
	}

	if agent.CooldownUntil != nil && time.Now().Before(*agent.CooldownUntil) {
		return errors.New("agent on cooldown")
	}

	if semdragons.TierFromLevel(agent.Level) < quest.MinTier {
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
			if agent.HasSkill(required) {
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

func (c *Component) createBossBattle(quest *semdragons.Quest, agentID semdragons.AgentID) *semdragons.BossBattle {
	instance := semdragons.GenerateInstance()
	battleID := semdragons.BattleID(c.boardConfig.BattleEntityID(instance))

	battle := &semdragons.BossBattle{
		ID:        battleID,
		QuestID:   quest.ID,
		AgentID:   agentID,
		Level:     quest.Constraints.ReviewLevel,
		Status:    semdragons.BattleActive,
		StartedAt: time.Now(),
	}

	switch quest.Constraints.ReviewLevel {
	case semdragons.ReviewAuto:
		battle.Criteria = []semdragons.ReviewCriterion{
			{Name: "format", Description: "Output format validation", Weight: 0.5, Threshold: 0.9},
			{Name: "completeness", Description: "All required fields present", Weight: 0.5, Threshold: 0.9},
		}
		battle.Judges = []semdragons.Judge{
			{ID: "judge-auto", Type: semdragons.JudgeAutomated, Config: map[string]any{}},
		}

	case semdragons.ReviewStandard:
		battle.Criteria = []semdragons.ReviewCriterion{
			{Name: "correctness", Description: "Output is correct", Weight: 0.4, Threshold: 0.7},
			{Name: "quality", Description: "Output quality", Weight: 0.3, Threshold: 0.6},
			{Name: "completeness", Description: "All requirements met", Weight: 0.3, Threshold: 0.8},
		}
		battle.Judges = []semdragons.Judge{
			{ID: "judge-auto", Type: semdragons.JudgeAutomated, Config: map[string]any{}},
			{ID: "judge-llm-1", Type: semdragons.JudgeLLM, Config: map[string]any{}},
		}

	case semdragons.ReviewStrict:
		battle.Criteria = []semdragons.ReviewCriterion{
			{Name: "correctness", Description: "Output is correct", Weight: 0.3, Threshold: 0.8},
			{Name: "quality", Description: "Output quality", Weight: 0.25, Threshold: 0.75},
			{Name: "completeness", Description: "All requirements met", Weight: 0.25, Threshold: 0.85},
			{Name: "robustness", Description: "Edge cases handled", Weight: 0.2, Threshold: 0.7},
		}
		battle.Judges = []semdragons.Judge{
			{ID: "judge-auto", Type: semdragons.JudgeAutomated, Config: map[string]any{}},
			{ID: "judge-llm-1", Type: semdragons.JudgeLLM, Config: map[string]any{}},
			{ID: "judge-llm-2", Type: semdragons.JudgeLLM, Config: map[string]any{}},
		}

	case semdragons.ReviewHuman:
		battle.Criteria = []semdragons.ReviewCriterion{
			{Name: "correctness", Description: "Output is correct", Weight: 0.3, Threshold: 0.8},
			{Name: "quality", Description: "Output quality", Weight: 0.25, Threshold: 0.75},
			{Name: "completeness", Description: "All requirements met", Weight: 0.25, Threshold: 0.85},
			{Name: "creativity", Description: "Thoughtful approach", Weight: 0.2, Threshold: 0.6},
		}
		battle.Judges = []semdragons.Judge{
			{ID: "judge-auto", Type: semdragons.JudgeAutomated, Config: map[string]any{}},
			{ID: "judge-llm-1", Type: semdragons.JudgeLLM, Config: map[string]any{}},
			{ID: "judge-human", Type: semdragons.JudgeHuman, Config: map[string]any{}},
		}
	}

	return battle
}
