package semdragons

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
)

// =============================================================================
// NATS QUEST BOARD - QuestBoard implementation backed by JetStream
// =============================================================================
// Config-driven implementation using:
// - 6-part entity IDs (org.platform.game.board.type.instance)
// - Single KV bucket with dotted keys
// - Presence-based indices for efficient queries
// - Vocabulary predicates for event subjects
// =============================================================================

// NATSQuestBoard implements QuestBoard using NATS JetStream.
type NATSQuestBoard struct {
	client  *natsclient.Client
	config  *BoardConfig
	storage *Storage
	events  *EventPublisher
}

// NATSQuestBoardOption configures a NATSQuestBoard.
type NATSQuestBoardOption func(*NATSQuestBoard)

// NewNATSQuestBoard creates a new QuestBoard backed by NATS JetStream.
func NewNATSQuestBoard(ctx context.Context, client *natsclient.Client, config BoardConfig, opts ...NATSQuestBoardOption) (*NATSQuestBoard, error) {
	storage, err := CreateStorage(ctx, client, &config)
	if err != nil {
		return nil, errs.Wrap(err, "NATSQuestBoard", "New", "create storage")
	}

	board := &NATSQuestBoard{
		client:  client,
		config:  &config,
		storage: storage,
		events:  NewEventPublisher(client),
	}

	for _, opt := range opts {
		opt(board)
	}

	return board, nil
}

// NewNATSQuestBoardWithStorage creates a QuestBoard with pre-existing storage.
func NewNATSQuestBoardWithStorage(client *natsclient.Client, storage *Storage, opts ...NATSQuestBoardOption) *NATSQuestBoard {
	board := &NATSQuestBoard{
		client:  client,
		config:  storage.Config(),
		storage: storage,
		events:  NewEventPublisher(client),
	}

	for _, opt := range opts {
		opt(board)
	}

	return board
}

// Storage returns the underlying storage.
func (b *NATSQuestBoard) Storage() *Storage {
	return b.storage
}

// Config returns the board configuration.
func (b *NATSQuestBoard) Config() *BoardConfig {
	return b.config
}

// =============================================================================
// POSTING
// =============================================================================

// PostQuest adds a new quest to the board.
func (b *NATSQuestBoard) PostQuest(ctx context.Context, quest Quest) (*Quest, error) {
	// Generate instance ID if not provided
	instance := ExtractInstance(string(quest.ID))
	if instance == "" || instance == string(quest.ID) {
		instance = GenerateInstance()
	}

	// Set full entity ID
	quest.ID = QuestID(b.config.QuestEntityID(instance))

	// Set defaults
	quest.Status = QuestPosted
	quest.PostedAt = time.Now()
	if quest.MaxAttempts == 0 {
		quest.MaxAttempts = 3
	}
	if quest.BaseXP == 0 {
		quest.BaseXP = DefaultXPForDifficulty(quest.Difficulty)
	}
	if quest.MinTier == 0 {
		quest.MinTier = TierFromDifficulty(quest.Difficulty)
	}

	// Store quest
	if err := b.storage.PutQuest(ctx, instance, &quest); err != nil {
		return nil, errs.Wrap(err, "QuestBoard", "PostQuest", "store quest")
	}

	// Add to posted index
	if err := b.storage.AddQuestStatusIndex(ctx, QuestPosted, instance); err != nil {
		return nil, errs.Wrap(err, "QuestBoard", "PostQuest", "add to posted index")
	}

	// Add to guild priority index if applicable
	if quest.GuildPriority != nil {
		guildInstance := ExtractInstance(string(*quest.GuildPriority))
		if err := b.storage.AddGuildQuestIndex(ctx, guildInstance, instance); err != nil {
			// Log but don't fail
		}
	}

	// Emit event
	if err := b.events.PublishQuestPosted(ctx, QuestPostedPayload{
		Quest:    quest,
		PostedAt: quest.PostedAt,
	}); err != nil {
		// Log but don't fail - quest is posted
	}

	return &quest, nil
}

// PostSubQuests decomposes a parent quest into sub-quests.
func (b *NATSQuestBoard) PostSubQuests(ctx context.Context, parentID QuestID, subQuests []Quest, decomposer AgentID) ([]Quest, error) {
	parentInstance := ExtractInstance(string(parentID))

	// Load parent quest
	parent, err := b.storage.GetQuest(ctx, parentInstance)
	if err != nil {
		return nil, errs.Wrap(err, "QuestBoard", "PostSubQuests", "load parent")
	}

	if parent.Status != QuestClaimed && parent.Status != QuestInProgress {
		return nil, fmt.Errorf("parent must be claimed or in_progress")
	}

	// Validate decomposer permissions
	decomposerInstance := ExtractInstance(string(decomposer))
	agent, err := b.storage.GetAgent(ctx, decomposerInstance)
	if err != nil {
		return nil, errs.Wrap(err, "QuestBoard", "PostSubQuests", "load decomposer")
	}

	perms := TierPerms[TierFromLevel(agent.Level)]
	if !perms.CanDecomposeQuest {
		return nil, errors.New("agent cannot decompose quests (requires Master+ tier)")
	}

	// Post each sub-quest
	posted := make([]Quest, 0, len(subQuests))
	subQuestIDs := make([]QuestID, 0, len(subQuests))

	for _, sq := range subQuests {
		sq.ParentQuest = &parentID
		sq.DecomposedBy = &decomposer

		result, err := b.PostQuest(ctx, sq)
		if err != nil {
			return nil, errs.Wrap(err, "QuestBoard", "PostSubQuests", "post sub-quest")
		}

		posted = append(posted, *result)
		subQuestIDs = append(subQuestIDs, result.ID)

		// Add parent-child index
		childInstance := ExtractInstance(string(result.ID))
		b.storage.AddToIndex(ctx, b.storage.ParentQuestIndexKey(parentInstance, childInstance))
	}

	// Update parent with sub-quest IDs
	parent.SubQuests = subQuestIDs
	parent.DecomposedBy = &decomposer
	if err := b.storage.PutQuest(ctx, parentInstance, parent); err != nil {
		return nil, errs.Wrap(err, "QuestBoard", "PostSubQuests", "update parent")
	}

	return posted, nil
}

// =============================================================================
// CLAIMING
// =============================================================================

// AvailableQuests returns quests an agent is eligible to claim.
func (b *NATSQuestBoard) AvailableQuests(ctx context.Context, agentID AgentID, opts QuestFilter) ([]Quest, error) {
	agentInstance := ExtractInstance(string(agentID))

	agent, err := b.storage.GetAgent(ctx, agentInstance)
	if err != nil {
		return nil, errs.Wrap(err, "QuestBoard", "AvailableQuests", "load agent")
	}

	// Check agent can claim
	if agent.Status != AgentIdle {
		return []Quest{}, nil
	}
	if agent.CooldownUntil != nil && time.Now().Before(*agent.CooldownUntil) {
		return []Quest{}, nil
	}

	// Get posted quest instances
	postedInstances, err := b.storage.ListQuestsByStatus(ctx, QuestPosted)
	if err != nil {
		return nil, errs.Wrap(err, "QuestBoard", "AvailableQuests", "list posted")
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}

	agentTier := TierFromLevel(agent.Level)
	available := make([]Quest, 0, limit)

	// Get guild priority quests
	guildPriorityMap := make(map[string]bool)
	for _, guildID := range agent.Guilds {
		guildInstance := ExtractInstance(string(guildID))
		guildQuests, _ := b.storage.ListQuestsByGuild(ctx, guildInstance)
		for _, qid := range guildQuests {
			guildPriorityMap[qid] = true
		}
	}

	// Sort: guild priority first
	sortedInstances := make([]string, 0, len(postedInstances))
	for _, inst := range postedInstances {
		if guildPriorityMap[inst] {
			sortedInstances = append([]string{inst}, sortedInstances...)
		} else {
			sortedInstances = append(sortedInstances, inst)
		}
	}

	for _, instance := range sortedInstances {
		if len(available) >= limit {
			break
		}

		quest, err := b.storage.GetQuest(ctx, instance)
		if err != nil {
			continue
		}

		// Filter checks
		if quest.Status != QuestPosted {
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
			guildInstance := ExtractInstance(string(*opts.GuildID))
			if quest.GuildPriority == nil || ExtractInstance(string(*quest.GuildPriority)) != guildInstance {
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
func (b *NATSQuestBoard) ClaimQuest(ctx context.Context, questID QuestID, agentID AgentID) error {
	questInstance := ExtractInstance(string(questID))
	agentInstance := ExtractInstance(string(agentID))

	// Load agent
	agent, err := b.storage.GetAgent(ctx, agentInstance)
	if err != nil {
		return errs.Wrap(err, "QuestBoard", "ClaimQuest", "load agent")
	}

	var updatedQuest Quest

	// Atomic update
	err = b.storage.UpdateQuest(ctx, questInstance, func(quest *Quest) error {
		if quest.Status != QuestPosted {
			return fmt.Errorf("quest not available: %s", quest.Status)
		}

		if err := b.validateAgentCanClaim(agent, quest); err != nil {
			return err
		}

		now := time.Now()
		quest.Status = QuestClaimed
		quest.ClaimedBy = &agentID
		quest.ClaimedAt = &now
		quest.Attempts++

		updatedQuest = *quest
		return nil
	})

	if err != nil {
		return errs.Wrap(err, "QuestBoard", "ClaimQuest", "update quest")
	}

	// Update indices
	b.storage.MoveQuestStatus(ctx, questInstance, QuestPosted, QuestClaimed)
	b.storage.AddAgentQuestIndex(ctx, agentInstance, questInstance)

	// Emit event
	if updatedQuest.ClaimedAt != nil {
		b.events.PublishQuestClaimed(ctx, QuestClaimedPayload{
			Quest:     updatedQuest,
			AgentID:   agentID,
			ClaimedAt: *updatedQuest.ClaimedAt,
		})
	}

	return nil
}

// ClaimQuestForParty assigns a quest to a party.
func (b *NATSQuestBoard) ClaimQuestForParty(ctx context.Context, questID QuestID, partyID PartyID) error {
	questInstance := ExtractInstance(string(questID))
	partyInstance := ExtractInstance(string(partyID))

	party, err := b.storage.GetParty(ctx, partyInstance)
	if err != nil {
		return errs.Wrap(err, "QuestBoard", "ClaimQuestForParty", "load party")
	}

	leadInstance := ExtractInstance(string(party.Lead))
	agent, err := b.storage.GetAgent(ctx, leadInstance)
	if err != nil {
		return errs.Wrap(err, "QuestBoard", "ClaimQuestForParty", "load lead")
	}

	perms := TierPerms[TierFromLevel(agent.Level)]
	if !perms.CanLeadParty {
		return errors.New("party lead cannot lead parties (requires Master+ tier)")
	}

	var updatedQuest Quest

	err = b.storage.UpdateQuest(ctx, questInstance, func(quest *Quest) error {
		if quest.Status != QuestPosted {
			return fmt.Errorf("quest not available: %s", quest.Status)
		}

		if quest.MinPartySize > 0 && len(party.Members) < quest.MinPartySize {
			return errors.New("party too small")
		}

		if TierFromLevel(agent.Level) < quest.MinTier {
			return errors.New("party lead tier too low")
		}

		now := time.Now()
		quest.Status = QuestClaimed
		quest.ClaimedBy = &party.Lead
		quest.PartyID = &partyID
		quest.ClaimedAt = &now
		quest.Attempts++

		updatedQuest = *quest
		return nil
	})

	if err != nil {
		return errs.Wrap(err, "QuestBoard", "ClaimQuestForParty", "update quest")
	}

	b.storage.MoveQuestStatus(ctx, questInstance, QuestPosted, QuestClaimed)

	b.events.PublishQuestClaimed(ctx, QuestClaimedPayload{
		Quest:     updatedQuest,
		AgentID:   party.Lead,
		PartyID:   &partyID,
		ClaimedAt: *updatedQuest.ClaimedAt,
	})

	return nil
}

// AbandonQuest returns a quest to the board.
func (b *NATSQuestBoard) AbandonQuest(ctx context.Context, questID QuestID, reason string) error {
	questInstance := ExtractInstance(string(questID))

	var updatedQuest Quest
	var agentID AgentID
	var partyID *PartyID

	err := b.storage.UpdateQuest(ctx, questInstance, func(quest *Quest) error {
		if quest.Status != QuestClaimed && quest.Status != QuestInProgress {
			return fmt.Errorf("quest not abandonable: %s", quest.Status)
		}

		if quest.ClaimedBy != nil {
			agentID = *quest.ClaimedBy
		}
		partyID = quest.PartyID

		quest.Status = QuestPosted
		quest.ClaimedBy = nil
		quest.PartyID = nil
		quest.ClaimedAt = nil
		quest.StartedAt = nil

		updatedQuest = *quest
		return nil
	})

	if err != nil {
		return errs.Wrap(err, "QuestBoard", "AbandonQuest", "update quest")
	}

	// Update indices
	b.storage.RemoveQuestStatusIndex(ctx, QuestClaimed, questInstance)
	b.storage.RemoveQuestStatusIndex(ctx, QuestInProgress, questInstance)
	b.storage.AddQuestStatusIndex(ctx, QuestPosted, questInstance)

	if agentID != "" {
		agentInstance := ExtractInstance(string(agentID))
		b.storage.RemoveAgentQuestIndex(ctx, agentInstance, questInstance)
	}

	b.events.PublishQuestAbandoned(ctx, QuestAbandonedPayload{
		Quest:       updatedQuest,
		AgentID:     agentID,
		PartyID:     partyID,
		Reason:      reason,
		AbandonedAt: time.Now(),
	})

	return nil
}

// =============================================================================
// EXECUTION
// =============================================================================

// StartQuest marks a quest as in-progress.
func (b *NATSQuestBoard) StartQuest(ctx context.Context, questID QuestID) error {
	questInstance := ExtractInstance(string(questID))

	var updatedQuest Quest
	var agentID AgentID
	var partyID *PartyID

	err := b.storage.UpdateQuest(ctx, questInstance, func(quest *Quest) error {
		if quest.Status != QuestClaimed {
			return fmt.Errorf("quest not claimed: %s", quest.Status)
		}

		now := time.Now()
		quest.Status = QuestInProgress
		quest.StartedAt = &now

		if quest.ClaimedBy != nil {
			agentID = *quest.ClaimedBy
		}
		partyID = quest.PartyID

		updatedQuest = *quest
		return nil
	})

	if err != nil {
		return errs.Wrap(err, "QuestBoard", "StartQuest", "update quest")
	}

	b.storage.MoveQuestStatus(ctx, questInstance, QuestClaimed, QuestInProgress)

	if updatedQuest.StartedAt != nil {
		b.events.PublishQuestStarted(ctx, QuestStartedPayload{
			Quest:     updatedQuest,
			AgentID:   agentID,
			PartyID:   partyID,
			StartedAt: *updatedQuest.StartedAt,
		})
	}

	return nil
}

// SubmitResult submits quest output for review.
func (b *NATSQuestBoard) SubmitResult(ctx context.Context, questID QuestID, result any) (*BossBattle, error) {
	questInstance := ExtractInstance(string(questID))

	var updatedQuest Quest
	var agentID AgentID
	var needsReview bool

	err := b.storage.UpdateQuest(ctx, questInstance, func(quest *Quest) error {
		if quest.Status != QuestInProgress {
			return fmt.Errorf("quest not in_progress: %s", quest.Status)
		}

		quest.Output = result
		needsReview = quest.Constraints.RequireReview

		if needsReview {
			quest.Status = QuestInReview
		} else {
			now := time.Now()
			quest.Status = QuestCompleted
			quest.CompletedAt = &now
		}

		if quest.ClaimedBy != nil {
			agentID = *quest.ClaimedBy
		}

		updatedQuest = *quest
		return nil
	})

	if err != nil {
		return nil, errs.Wrap(err, "QuestBoard", "SubmitResult", "update quest")
	}

	// Update indices
	b.storage.RemoveQuestStatusIndex(ctx, QuestInProgress, questInstance)
	if needsReview {
		b.storage.AddQuestStatusIndex(ctx, QuestInReview, questInstance)
	} else {
		b.storage.AddQuestStatusIndex(ctx, QuestCompleted, questInstance)
	}

	var battle *BossBattle
	var battleID *BattleID

	if needsReview {
		battle = b.createBossBattle(&updatedQuest, agentID)
		id := battle.ID
		battleID = &id

		battleInstance := ExtractInstance(string(battle.ID))
		if err := b.storage.PutBattle(ctx, battleInstance, battle); err != nil {
			// Log but continue
		}

		b.events.PublishBattleStarted(ctx, BattleStartedPayload{
			Battle:    *battle,
			Quest:     updatedQuest,
			StartedAt: battle.StartedAt,
		})
	}

	b.events.PublishQuestSubmitted(ctx, QuestSubmittedPayload{
		Quest:       updatedQuest,
		AgentID:     agentID,
		Result:      result,
		SubmittedAt: time.Now(),
		BattleID:    battleID,
	})

	return battle, nil
}

// =============================================================================
// LIFECYCLE
// =============================================================================

// CompleteQuest marks a quest as successfully completed.
func (b *NATSQuestBoard) CompleteQuest(ctx context.Context, questID QuestID, verdict BattleVerdict) error {
	questInstance := ExtractInstance(string(questID))

	var updatedQuest Quest
	var agentID AgentID
	var partyID *PartyID
	var duration time.Duration

	err := b.storage.UpdateQuest(ctx, questInstance, func(quest *Quest) error {
		if quest.Status != QuestInReview && quest.Status != QuestInProgress {
			return fmt.Errorf("quest not completable: %s", quest.Status)
		}

		now := time.Now()
		quest.Status = QuestCompleted
		quest.CompletedAt = &now

		if quest.StartedAt != nil {
			duration = now.Sub(*quest.StartedAt)
		}

		if quest.ClaimedBy != nil {
			agentID = *quest.ClaimedBy
		}
		partyID = quest.PartyID

		updatedQuest = *quest
		return nil
	})

	if err != nil {
		return errs.Wrap(err, "QuestBoard", "CompleteQuest", "update quest")
	}

	// Update indices
	b.storage.RemoveQuestStatusIndex(ctx, QuestInReview, questInstance)
	b.storage.RemoveQuestStatusIndex(ctx, QuestInProgress, questInstance)
	b.storage.AddQuestStatusIndex(ctx, QuestCompleted, questInstance)

	if agentID != "" {
		agentInstance := ExtractInstance(string(agentID))
		b.storage.RemoveAgentQuestIndex(ctx, agentInstance, questInstance)
	}

	b.events.PublishQuestCompleted(ctx, QuestCompletedPayload{
		Quest:       updatedQuest,
		AgentID:     agentID,
		PartyID:     partyID,
		Verdict:     verdict,
		CompletedAt: *updatedQuest.CompletedAt,
		Duration:    duration,
	})

	return nil
}

// FailQuest marks a quest as failed.
func (b *NATSQuestBoard) FailQuest(ctx context.Context, questID QuestID, reason string) error {
	questInstance := ExtractInstance(string(questID))

	var updatedQuest Quest
	var agentID AgentID
	var partyID *PartyID
	var reposted bool

	err := b.storage.UpdateQuest(ctx, questInstance, func(quest *Quest) error {
		if quest.Status != QuestInProgress && quest.Status != QuestInReview {
			return fmt.Errorf("quest not failable: %s", quest.Status)
		}

		if quest.ClaimedBy != nil {
			agentID = *quest.ClaimedBy
		}
		partyID = quest.PartyID

		if quest.Attempts < quest.MaxAttempts {
			quest.Status = QuestPosted
			quest.ClaimedBy = nil
			quest.PartyID = nil
			quest.ClaimedAt = nil
			quest.StartedAt = nil
			quest.Output = nil
			reposted = true
		} else {
			quest.Status = QuestFailed
		}

		updatedQuest = *quest
		return nil
	})

	if err != nil {
		return errs.Wrap(err, "QuestBoard", "FailQuest", "update quest")
	}

	// Update indices
	b.storage.RemoveQuestStatusIndex(ctx, QuestInProgress, questInstance)
	b.storage.RemoveQuestStatusIndex(ctx, QuestInReview, questInstance)

	if reposted {
		b.storage.AddQuestStatusIndex(ctx, QuestPosted, questInstance)
	} else {
		b.storage.AddQuestStatusIndex(ctx, QuestFailed, questInstance)
	}

	if agentID != "" {
		agentInstance := ExtractInstance(string(agentID))
		b.storage.RemoveAgentQuestIndex(ctx, agentInstance, questInstance)
	}

	b.events.PublishQuestFailed(ctx, QuestFailedPayload{
		Quest:    updatedQuest,
		AgentID:  agentID,
		PartyID:  partyID,
		Reason:   reason,
		FailType: FailureSoft,
		FailedAt: time.Now(),
		Attempt:  updatedQuest.Attempts,
		Reposted: reposted,
	})

	return nil
}

// EscalateQuest flags a quest for higher-level attention.
func (b *NATSQuestBoard) EscalateQuest(ctx context.Context, questID QuestID, reason string) error {
	questInstance := ExtractInstance(string(questID))

	var updatedQuest Quest
	var agentID AgentID
	var partyID *PartyID

	err := b.storage.UpdateQuest(ctx, questInstance, func(quest *Quest) error {
		if quest.Status == QuestCompleted || quest.Status == QuestCancelled || quest.Status == QuestEscalated {
			return fmt.Errorf("quest cannot be escalated: %s", quest.Status)
		}

		if quest.ClaimedBy != nil {
			agentID = *quest.ClaimedBy
		}
		partyID = quest.PartyID

		quest.Status = QuestEscalated
		quest.Escalated = true

		updatedQuest = *quest
		return nil
	})

	if err != nil {
		return errs.Wrap(err, "QuestBoard", "EscalateQuest", "update quest")
	}

	// Remove from all active indices
	for _, status := range []QuestStatus{QuestPosted, QuestClaimed, QuestInProgress, QuestInReview, QuestFailed} {
		b.storage.RemoveQuestStatusIndex(ctx, status, questInstance)
	}
	b.storage.AddQuestStatusIndex(ctx, QuestEscalated, questInstance)

	if agentID != "" {
		agentInstance := ExtractInstance(string(agentID))
		b.storage.RemoveAgentQuestIndex(ctx, agentInstance, questInstance)
	}

	b.events.PublishQuestEscalated(ctx, QuestEscalatedPayload{
		Quest:       updatedQuest,
		AgentID:     agentID,
		PartyID:     partyID,
		Reason:      reason,
		EscalatedAt: time.Now(),
		Attempts:    updatedQuest.Attempts,
	})

	return nil
}

// =============================================================================
// QUERIES
// =============================================================================

// GetQuest returns a quest by ID.
func (b *NATSQuestBoard) GetQuest(ctx context.Context, questID QuestID) (*Quest, error) {
	instance := ExtractInstance(string(questID))
	return b.storage.GetQuest(ctx, instance)
}

// BoardStats returns current board statistics.
func (b *NATSQuestBoard) BoardStats(ctx context.Context) (*BoardStats, error) {
	stats, err := b.storage.GetBoardStats(ctx)
	if err != nil {
		return nil, err
	}

	// Initialize maps if nil
	if stats.ByDifficulty == nil {
		stats.ByDifficulty = make(map[QuestDifficulty]int)
	}
	if stats.BySkill == nil {
		stats.BySkill = make(map[SkillTag]int)
	}

	// Count by status
	for _, status := range []QuestStatus{QuestPosted, QuestClaimed, QuestInProgress, QuestCompleted, QuestFailed, QuestEscalated} {
		instances, err := b.storage.ListQuestsByStatus(ctx, status)
		if err != nil {
			continue
		}
		count := len(instances)
		switch status {
		case QuestPosted:
			stats.TotalPosted = count
		case QuestClaimed:
			stats.TotalClaimed = count
		case QuestInProgress:
			stats.TotalInProgress = count
		case QuestCompleted:
			stats.TotalCompleted = count
		case QuestFailed:
			stats.TotalFailed = count
		case QuestEscalated:
			stats.TotalEscalated = count
		}
	}

	return stats, nil
}

// =============================================================================
// HELPERS
// =============================================================================

func (b *NATSQuestBoard) validateAgentCanClaim(agent *Agent, quest *Quest) error {
	if agent.Status != AgentIdle {
		return fmt.Errorf("agent not idle: %s", agent.Status)
	}

	if agent.CooldownUntil != nil && time.Now().Before(*agent.CooldownUntil) {
		return errors.New("agent on cooldown")
	}

	if TierFromLevel(agent.Level) < quest.MinTier {
		return errors.New("agent tier too low")
	}

	if quest.PartyRequired {
		return errors.New("quest requires party")
	}

	perms := TierPerms[TierFromLevel(agent.Level)]
	if agent.CurrentQuest != nil && perms.MaxConcurrent <= 1 {
		return errors.New("agent at concurrent quest limit")
	}

	if len(quest.RequiredSkills) > 0 {
		hasSkill := false
		for _, required := range quest.RequiredSkills {
			if slices.Contains(agent.Skills, required) {
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

func (b *NATSQuestBoard) createBossBattle(quest *Quest, agentID AgentID) *BossBattle {
	instance := GenerateInstance()
	battleID := BattleID(b.config.BattleEntityID(instance))

	battle := &BossBattle{
		ID:        battleID,
		QuestID:   quest.ID,
		AgentID:   agentID,
		Level:     quest.Constraints.ReviewLevel,
		Status:    BattleActive,
		StartedAt: time.Now(),
	}

	switch quest.Constraints.ReviewLevel {
	case ReviewAuto:
		battle.Criteria = []ReviewCriterion{
			{Name: "format", Description: "Output format validation", Weight: 0.5, Threshold: 0.9},
			{Name: "completeness", Description: "All required fields present", Weight: 0.5, Threshold: 0.9},
		}
		battle.Judges = []Judge{
			{ID: "judge-auto", Type: JudgeAutomated, Config: map[string]any{}},
		}

	case ReviewStandard:
		battle.Criteria = []ReviewCriterion{
			{Name: "correctness", Description: "Output is correct", Weight: 0.4, Threshold: 0.7},
			{Name: "quality", Description: "Output quality", Weight: 0.3, Threshold: 0.6},
			{Name: "completeness", Description: "All requirements met", Weight: 0.3, Threshold: 0.8},
		}
		battle.Judges = []Judge{
			{ID: "judge-auto", Type: JudgeAutomated, Config: map[string]any{}},
			{ID: "judge-llm-1", Type: JudgeLLM, Config: map[string]any{}},
		}

	case ReviewStrict:
		battle.Criteria = []ReviewCriterion{
			{Name: "correctness", Description: "Output is correct", Weight: 0.3, Threshold: 0.8},
			{Name: "quality", Description: "Output quality", Weight: 0.25, Threshold: 0.75},
			{Name: "completeness", Description: "All requirements met", Weight: 0.25, Threshold: 0.85},
			{Name: "robustness", Description: "Edge cases handled", Weight: 0.2, Threshold: 0.7},
		}
		battle.Judges = []Judge{
			{ID: "judge-auto", Type: JudgeAutomated, Config: map[string]any{}},
			{ID: "judge-llm-1", Type: JudgeLLM, Config: map[string]any{}},
			{ID: "judge-llm-2", Type: JudgeLLM, Config: map[string]any{}},
		}

	case ReviewHuman:
		battle.Criteria = []ReviewCriterion{
			{Name: "correctness", Description: "Output is correct", Weight: 0.3, Threshold: 0.8},
			{Name: "quality", Description: "Output quality", Weight: 0.25, Threshold: 0.75},
			{Name: "completeness", Description: "All requirements met", Weight: 0.25, Threshold: 0.85},
			{Name: "creativity", Description: "Thoughtful approach", Weight: 0.2, Threshold: 0.6},
		}
		battle.Judges = []Judge{
			{ID: "judge-auto", Type: JudgeAutomated, Config: map[string]any{}},
			{ID: "judge-llm-1", Type: JudgeLLM, Config: map[string]any{}},
			{ID: "judge-human", Type: JudgeHuman, Config: map[string]any{}},
		}
	}

	return battle
}
