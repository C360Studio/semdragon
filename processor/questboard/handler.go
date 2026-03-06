package questboard

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/partycoord"
	"github.com/c360studio/semdragons/processor/agentprogression"
)

// =============================================================================
// QUEST BOARD OPERATIONS
// =============================================================================

// GraphClient returns the underlying graph client for external access.
func (c *Component) GraphClient() *semdragons.GraphClient {
	return c.graph
}

// BoardConfig returns the board configuration.
func (c *Component) BoardConfig() *domain.BoardConfig {
	return c.boardConfig
}

// PostQuest adds a new quest to the board.
func (c *Component) PostQuest(ctx context.Context, quest domain.Quest) (*domain.Quest, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	// Generate instance ID if not provided
	instance := domain.ExtractInstance(string(quest.ID))
	if instance == "" || instance == string(quest.ID) {
		instance = domain.GenerateInstance()
	}

	// Set full entity ID
	quest.ID = domain.QuestID(c.boardConfig.QuestEntityID(instance))

	// Create trace context for this quest (used for distributed tracing headers,
	// not trajectory storage — agentic-loop owns trajectories via LoopID).
	_ = c.traces.StartQuestTrace(quest.ID)
	if quest.ParentQuest != nil {
		_ = c.traces.StartQuestTraceWithParent(quest.ID, *quest.ParentQuest)
	}

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

	// Emit quest to graph system (KV write is the event — watchers are notified)
	if err := c.graph.EmitEntity(ctx, &quest, "quest.posted"); err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "QuestBoard", "PostQuest", "emit quest")
	}

	return &quest, nil
}

// PostSubQuests decomposes a parent quest into sub-quests.
func (c *Component) PostSubQuests(ctx context.Context, parentID domain.QuestID, subQuests []domain.Quest, decomposer domain.AgentID) ([]domain.Quest, error) {
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
	agent, err := c.getAgentByID(ctx, decomposer)
	if err != nil {
		c.errorsCount.Add(1)
		return nil, errs.Wrap(err, "QuestBoard", "PostSubQuests", "load decomposer")
	}

	perms := domain.TierPermissionsFor(domain.TierFromLevel(agent.Level))
	if !perms.CanDecomposeQuest {
		return nil, errors.New("agent cannot decompose quests (requires Master+ tier)")
	}

	// Post each sub-quest
	posted := make([]domain.Quest, 0, len(subQuests))
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

// PostQuestChain validates and posts a chain of interdependent quests.
// Index-based DependsOn entries in QuestChainEntry are resolved to real QuestIDs.
func (c *Component) PostQuestChain(ctx context.Context, chain domain.QuestChainBrief) ([]domain.Quest, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	if err := domain.ValidateQuestChainBrief(&chain); err != nil {
		return nil, err
	}

	c.lastActivity.Store(time.Now())
	c.messagesProcessed.Add(1)

	// First pass: post each quest without DependsOn (we don't have real IDs yet)
	posted := make([]domain.Quest, 0, len(chain.Quests))
	for _, entry := range chain.Quests {
		q := domain.Quest{
			Title:       entry.Title,
			Description: entry.Description,
			Acceptance:  entry.Acceptance,
		}
		if entry.Difficulty != nil {
			q.Difficulty = *entry.Difficulty
		} else {
			q.Difficulty = domain.DifficultyModerate
		}
		q.RequiredSkills = entry.Skills

		if entry.Hints != nil {
			if entry.Hints.RequireHumanReview {
				q.Constraints.RequireReview = true
				q.Constraints.ReviewLevel = domain.ReviewStandard
			}
			if entry.Hints.PreferGuild != nil {
				q.GuildPriority = entry.Hints.PreferGuild
			}
		}

		result, err := c.PostQuest(ctx, q)
		if err != nil {
			c.errorsCount.Add(1)
			return nil, errs.Wrap(err, "QuestBoard", "PostQuestChain", "post entry")
		}
		posted = append(posted, *result)
	}

	// Second pass: resolve index-based DependsOn to real QuestIDs and emit updates
	for i, entry := range chain.Quests {
		if len(entry.DependsOn) == 0 {
			continue
		}

		deps := make([]domain.QuestID, 0, len(entry.DependsOn))
		for _, idx := range entry.DependsOn {
			deps = append(deps, posted[idx].ID)
		}
		posted[i].DependsOn = deps

		if err := c.graph.EmitEntityUpdate(ctx, &posted[i], "quest.dependencies.set"); err != nil {
			c.errorsCount.Add(1)
			return nil, errs.Wrap(err, "QuestBoard", "PostQuestChain", "set dependencies")
		}
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
func (c *Component) AvailableQuests(ctx context.Context, agentID domain.AgentID, opts QuestFilter) ([]domain.Quest, error) {
	if !c.running.Load() {
		return nil, errors.New("component not running")
	}

	c.lastActivity.Store(time.Now())

	agent, err := c.getAgentByID(ctx, agentID)
	if err != nil {
		return nil, errs.Wrap(err, "QuestBoard", "AvailableQuests", "load agent")
	}

	// Check agent can claim
	switch agent.Status {
	case domain.AgentRetired, domain.AgentInBattle:
		return []domain.Quest{}, nil
	case domain.AgentCooldown:
		if agent.CooldownUntil != nil && time.Now().Before(*agent.CooldownUntil) {
			return []domain.Quest{}, nil
		}
	}
	if agent.CurrentQuest != nil {
		return []domain.Quest{}, nil
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}

	// List all quests directly from KV and filter in-memory.
	// Use a high limit to ensure we scan beyond the default 100.
	entities, err := c.graph.ListEntitiesByType(ctx, "quest", 10000)
	if err != nil {
		return nil, errs.Wrap(err, "QuestBoard", "AvailableQuests", "list quests")
	}

	agentTier := domain.TierFromLevel(agent.Level)
	available := make([]domain.Quest, 0, limit)

	// Build entity map for O(1) dependency status lookups
	entityMap := make(map[string]*graph.EntityState, len(entities))
	for i := range entities {
		entityMap[entities[i].ID] = &entities[i]
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

		// Dependency blocking: skip if any DependsOn quest is not completed
		if len(quest.DependsOn) > 0 {
			blocked := false
			for _, depID := range quest.DependsOn {
				depEntity := entityMap[string(depID)]
				if depEntity == nil {
					blocked = true
					break
				}
				depQuest := c.questFromEntity(depEntity)
				if depQuest == nil || depQuest.Status != domain.QuestCompleted {
					blocked = true
					break
				}
			}
			if blocked {
				continue
			}
		}
		if domain.TrustTier(agentTier) < quest.MinTier {
			continue
		}
		if quest.PartyRequired && (opts.PartyOnly == nil || !*opts.PartyOnly) {
			continue
		}
		// Party sub-quests are assigned directly by the party lead — they must
		// never appear on the public board regardless of other filter options.
		if quest.PartyID != nil {
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

// ClaimQuest assigns a quest to an agent using CAS to prevent race conditions.
// Returns natsclient.ErrKVRevisionMismatch if another agent claimed first.
func (c *Component) ClaimQuest(ctx context.Context, questID domain.QuestID, agentID domain.AgentID) error {
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

	// Load quest with revision for CAS
	entity, revision, err := c.graph.GetQuestWithRevision(ctx, questID)
	if err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "ClaimQuest", "load quest")
	}

	quest := domain.QuestFromEntityState(entity)
	if quest == nil {
		return fmt.Errorf("failed to decode quest %s", questID)
	}

	if quest.Status != domain.QuestPosted {
		return fmt.Errorf("quest not available: %s", quest.Status)
	}

	// Dependency gate: all quests listed in DependsOn must be completed before
	// this quest can be claimed. This enforces sequential chains for solo quests;
	// party sub-quest DAGs are gated separately by the questdagexec processor.
	// INVARIANT: QuestCompleted is terminal for dependency resolution — FailQuest
	// only operates on in_progress/in_review, so a completed dep cannot regress.
	for _, depID := range quest.DependsOn {
		dep, depErr := c.getQuestByID(ctx, depID)
		if depErr != nil {
			c.errorsCount.Add(1)
			return errs.Wrap(depErr, "QuestBoard", "ClaimQuest", "load dependency")
		}
		if dep.Status != domain.QuestCompleted {
			return fmt.Errorf("quest has unmet dependencies: %s is %s", depID, dep.Status)
		}
	}

	// Party membership gate: if the quest belongs to a party, only members of
	// that party may claim it. Non-members must use ClaimQuestForParty instead.
	if quest.PartyID != nil {
		party, partyErr := c.getPartyByID(ctx, *quest.PartyID)
		if partyErr != nil {
			c.errorsCount.Add(1)
			return errs.Wrap(partyErr, "QuestBoard", "ClaimQuest", "load party")
		}
		isMember := false
		for _, m := range party.Members {
			if m.AgentID == agentID {
				isMember = true
				break
			}
		}
		if !isMember {
			return fmt.Errorf("agent is not a member of the party that owns this quest")
		}
	}

	if err := c.validateAgentCanClaim(agent, quest); err != nil {
		return err
	}

	// CAS write quest state: claimed. Fails if revision changed (another claim won).
	now := time.Now()
	quest.Status = domain.QuestClaimed
	quest.ClaimedBy = &agentID
	quest.ClaimedAt = &now
	quest.Attempts++

	if err := c.graph.EmitEntityCAS(ctx, quest, "quest.claimed", revision); err != nil {
		if errors.Is(err, natsclient.ErrKVRevisionMismatch) {
			return fmt.Errorf("quest already claimed by another agent: %w", err)
		}
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "ClaimQuest", "emit update")
	}

	// Update agent status to on_quest
	now2 := time.Now()
	questIDRef := quest.ID
	agent.Status = domain.AgentOnQuest
	agent.CurrentQuest = &questIDRef
	agent.UpdatedAt = now2
	if err := c.graph.EmitEntityUpdate(ctx, agent, "agent.status.on_quest"); err != nil {
		c.errorsCount.Add(1)
		c.logger.Error("failed to update agent status on claim", "error", err)
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

	party, err := c.getPartyByID(ctx, partyID)
	if err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "ClaimQuestForParty", "load party")
	}

	agent, err := c.getAgentByID(ctx, domain.AgentID(party.Lead))
	if err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "ClaimQuestForParty", "load lead")
	}

	perms := domain.TierPermissionsFor(domain.TierFromLevel(agent.Level))
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

	if domain.TierFromLevel(agent.Level) < quest.MinTier {
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

	// Emit updated quest to graph (KV write is the event — watchers are notified)
	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.claimed"); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "ClaimQuestForParty", "emit update")
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

	c.logger.Info("quest abandoned", "quest_id", questID, "reason", reason)

	quest, err := c.getQuestByID(ctx, questID)
	if err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "AbandonQuest", "load quest")
	}

	if quest.Status != domain.QuestClaimed && quest.Status != domain.QuestInProgress {
		return fmt.Errorf("quest not abandonable: %s", quest.Status)
	}

	// Reset agent status before clearing quest assignment
	if quest.ClaimedBy != nil {
		abandonAgent, agentErr := c.getAgentByID(ctx, *quest.ClaimedBy)
		if agentErr == nil {
			abandonAgent.Status = domain.AgentIdle
			abandonAgent.CurrentQuest = nil
			abandonAgent.UpdatedAt = time.Now()
			if writeErr := c.graph.EmitEntityUpdate(ctx, abandonAgent, "agent.status.idle"); writeErr != nil {
				c.errorsCount.Add(1)
				c.logger.Error("failed to reset agent status on abandon", "error", writeErr)
			}
		}
	}

	// Reset quest state
	quest.Status = domain.QuestPosted
	quest.ClaimedBy = nil
	quest.PartyID = nil
	quest.ClaimedAt = nil
	quest.StartedAt = nil

	// Emit updated quest to graph (KV write is the event — watchers are notified)
	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.abandoned"); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "AbandonQuest", "emit update")
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

	// Update quest state
	now := time.Now()
	quest.Status = domain.QuestInProgress
	quest.StartedAt = &now

	// Emit updated quest to graph (KV write is the event — watchers are notified)
	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.started"); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "StartQuest", "emit update")
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

	quest.Output = result

	if quest.Constraints.RequireReview {
		quest.Status = domain.QuestInReview
	} else {
		now := time.Now()
		quest.Status = domain.QuestCompleted
		quest.CompletedAt = &now
		if quest.StartedAt != nil {
			quest.Duration = now.Sub(*quest.StartedAt)
		}
	}

	// Emit updated quest to graph (KV write is the event — bossbattle watches for in_review)
	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.submitted"); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "SubmitResult", "emit update")
	}

	// If quest completed directly (no review), end the trace
	if !quest.Constraints.RequireReview {
		c.traces.EndQuestTrace(questID)
	}

	return nil
}

// CompleteQuest marks a quest as successfully completed.
func (c *Component) CompleteQuest(ctx context.Context, questID domain.QuestID, verdict domain.BattleVerdict) error {
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

	now := time.Now()
	quest.Status = domain.QuestCompleted
	quest.CompletedAt = &now
	quest.Verdict = &verdict

	if quest.StartedAt != nil {
		quest.Duration = now.Sub(*quest.StartedAt)
	}

	// Emit updated quest to graph (KV write is the event — agent_progression watches for completed)
	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.completed"); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "CompleteQuest", "emit update")
	}

	// End trace for this quest (terminal state)
	c.traces.EndQuestTrace(questID)

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

	// Set failure context on quest entity (watchers see this in triples)
	quest.FailureReason = reason
	quest.FailureType = domain.FailureQuality

	reposted := quest.Attempts < quest.MaxAttempts
	if reposted {
		// Reset agent status before clearing quest assignment (agentprogression
		// skips reposted quests because ClaimedBy is nil after repost)
		if quest.ClaimedBy != nil {
			repostAgent, agentErr := c.getAgentByID(ctx, *quest.ClaimedBy)
			if agentErr == nil {
				repostAgent.Status = domain.AgentIdle
				repostAgent.CurrentQuest = nil
				repostAgent.UpdatedAt = time.Now()
				if writeErr := c.graph.EmitEntityUpdate(ctx, repostAgent, "agent.status.idle"); writeErr != nil {
					c.errorsCount.Add(1)
					c.logger.Error("failed to reset agent status on fail-repost", "error", writeErr)
				}
			}
		}
		quest.Status = domain.QuestPosted
		quest.ClaimedBy = nil
		quest.PartyID = nil
		quest.ClaimedAt = nil
		quest.StartedAt = nil
		quest.Output = nil
	} else {
		quest.Status = domain.QuestFailed
	}

	// Emit updated quest to graph (KV write is the event — agent_progression watches for failed)
	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.failed"); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "FailQuest", "emit update")
	}

	// End trace if quest reached terminal state (not reposted)
	if !reposted {
		c.traces.EndQuestTrace(questID)
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

	c.logger.Info("quest escalated", "quest_id", questID, "reason", reason)

	quest, err := c.getQuestByID(ctx, questID)
	if err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "EscalateQuest", "load quest")
	}

	if quest.Status == domain.QuestCompleted || quest.Status == domain.QuestCancelled || quest.Status == domain.QuestEscalated {
		return fmt.Errorf("quest cannot be escalated: %s", quest.Status)
	}

	quest.Status = domain.QuestEscalated
	quest.Escalated = true

	// Emit updated quest to graph (KV write is the event — watchers are notified)
	if err := c.graph.EmitEntityUpdate(ctx, quest, "quest.escalated"); err != nil {
		c.errorsCount.Add(1)
		return errs.Wrap(err, "QuestBoard", "EscalateQuest", "emit update")
	}

	// End trace for escalated quest (terminal state requiring DM attention)
	c.traces.EndQuestTrace(questID)

	return nil
}

// GetQuest returns a quest by ID.
func (c *Component) GetQuest(ctx context.Context, questID domain.QuestID) (*domain.Quest, error) {
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
func (c *Component) getQuestByID(ctx context.Context, questID domain.QuestID) (*domain.Quest, error) {
	entity, err := c.graph.GetQuest(ctx, questID)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, fmt.Errorf("quest not found: %s", questID)
	}
	return c.questFromEntity(entity), nil
}

// getAgentByID retrieves an agent from the graph and reconstructs it.
func (c *Component) getAgentByID(ctx context.Context, agentID domain.AgentID) (*agentprogression.Agent, error) {
	entity, err := c.graph.GetAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}
	return agentprogression.AgentFromEntityState(entity), nil
}

// getPartyByID retrieves a party from the graph and reconstructs it.
func (c *Component) getPartyByID(ctx context.Context, partyID domain.PartyID) (*partycoord.Party, error) {
	entity, err := c.graph.GetParty(ctx, partyID)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, fmt.Errorf("party not found: %s", partyID)
	}
	return partycoord.PartyFromEntityState(entity), nil
}

// questFromEntity reconstructs a Quest from a graph entity.
func (c *Component) questFromEntity(entity *graph.EntityState) *domain.Quest {
	if entity == nil {
		return nil
	}

	quest := &domain.Quest{
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
		case "quest.dependency.quest":
			if v, ok := triple.Object.(string); ok {
				quest.DependsOn = append(quest.DependsOn, domain.QuestID(v))
			}
		case "quest.acceptance.criterion":
			if v, ok := triple.Object.(string); ok {
				quest.Acceptance = append(quest.Acceptance, v)
			}
		case "quest.review.level":
			if v, ok := triple.Object.(float64); ok {
				quest.Constraints.ReviewLevel = domain.ReviewLevel(int(v))
			}
		case "quest.review.needs_review":
			if v, ok := triple.Object.(bool); ok {
				quest.Constraints.RequireReview = v
			}
		case "quest.verdict.passed":
			if v, ok := triple.Object.(bool); ok {
				if quest.Verdict == nil {
					quest.Verdict = &domain.BattleVerdict{}
				}
				quest.Verdict.Passed = v
			}
		case "quest.verdict.score":
			if v, ok := triple.Object.(float64); ok {
				if quest.Verdict == nil {
					quest.Verdict = &domain.BattleVerdict{}
				}
				quest.Verdict.QualityScore = v
			}
		case "quest.verdict.xp_awarded":
			if v, ok := triple.Object.(float64); ok {
				if quest.Verdict == nil {
					quest.Verdict = &domain.BattleVerdict{}
				}
				quest.Verdict.XPAwarded = int64(v)
			}
		case "quest.verdict.feedback":
			if v, ok := triple.Object.(string); ok {
				if quest.Verdict == nil {
					quest.Verdict = &domain.BattleVerdict{}
				}
				quest.Verdict.Feedback = v
			}
		case "quest.failure.escalated":
			if v, ok := triple.Object.(bool); ok {
				quest.Escalated = v
			}
		case "quest.failure.reason":
			if v, ok := triple.Object.(string); ok {
				quest.FailureReason = v
			}
		case "quest.failure.type":
			if v, ok := triple.Object.(string); ok {
				quest.FailureType = domain.FailureType(v)
			}
		case "quest.duration":
			if v, ok := triple.Object.(string); ok {
				if d, err := time.ParseDuration(v); err == nil {
					quest.Duration = d
				}
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

func (c *Component) validateAgentCanClaim(agent *agentprogression.Agent, quest *domain.Quest) error {
	return agentprogression.ValidateAgentCanClaim(agent, quest)
}
