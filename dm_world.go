package semdragons

import (
	"context"
	"strings"
)

// =============================================================================
// WORLD STATE - Aggregation of all game entities
// =============================================================================
// The WorldState method provides a complete snapshot of the game world,
// useful for DM decision making and observability dashboards.
// =============================================================================

// WorldState returns the complete state of the game world.
func (dm *BaseDungeonMaster) WorldState(ctx context.Context) (*WorldState, error) {
	agents, err := dm.loadAllAgents(ctx)
	if err != nil {
		dm.logger.Warn("failed to load agents for world state", "error", err)
		agents = []Agent{}
	}

	quests, err := dm.loadActiveQuests(ctx)
	if err != nil {
		dm.logger.Warn("failed to load quests for world state", "error", err)
		quests = []Quest{}
	}

	parties, err := dm.loadActiveParties(ctx)
	if err != nil {
		dm.logger.Warn("failed to load parties for world state", "error", err)
		parties = []Party{}
	}

	guilds, err := dm.loadGuilds(ctx)
	if err != nil {
		dm.logger.Warn("failed to load guilds for world state", "error", err)
		guilds = []Guild{}
	}

	battles, err := dm.loadActiveBattles(ctx)
	if err != nil {
		dm.logger.Warn("failed to load battles for world state", "error", err)
		battles = []BossBattle{}
	}

	stats := dm.computeWorldStats(agents, quests, parties, guilds)

	return &WorldState{
		Agents:  agents,
		Quests:  quests,
		Parties: parties,
		Guilds:  guilds,
		Battles: battles,
		Stats:   stats,
	}, nil
}

// =============================================================================
// ENTITY LOADING HELPERS
// =============================================================================

// loadAllAgents returns all agents from storage.
func (dm *BaseDungeonMaster) loadAllAgents(ctx context.Context) ([]Agent, error) {
	keys, err := dm.storage.KV().Keys(ctx)
	if err != nil {
		return nil, err
	}

	var agents []Agent
	for _, key := range keys {
		if strings.HasPrefix(key, "agent.") && !strings.HasPrefix(key, "agent.streak.") {
			instance := strings.TrimPrefix(key, "agent.")
			agent, err := dm.storage.GetAgent(ctx, instance)
			if err != nil {
				dm.logger.Debug("failed to load agent", "instance", instance, "error", err)
				continue
			}
			agents = append(agents, *agent)
		}
	}

	return agents, nil
}

// loadActiveQuests returns quests that are not yet completed or cancelled.
func (dm *BaseDungeonMaster) loadActiveQuests(ctx context.Context) ([]Quest, error) {
	activeStatuses := []QuestStatus{
		QuestPosted,
		QuestClaimed,
		QuestInProgress,
		QuestInReview,
		QuestEscalated,
	}

	var quests []Quest
	for _, status := range activeStatuses {
		instances, err := dm.storage.ListQuestsByStatus(ctx, status)
		if err != nil {
			continue
		}

		for _, instance := range instances {
			quest, err := dm.storage.GetQuest(ctx, instance)
			if err != nil {
				dm.logger.Debug("failed to load quest", "instance", instance, "error", err)
				continue
			}
			quests = append(quests, *quest)
		}
	}

	return quests, nil
}

// loadActiveParties returns parties that are forming or active.
func (dm *BaseDungeonMaster) loadActiveParties(ctx context.Context) ([]Party, error) {
	keys, err := dm.storage.KV().Keys(ctx)
	if err != nil {
		return nil, err
	}

	var parties []Party
	for _, key := range keys {
		if strings.HasPrefix(key, "party.") {
			instance := strings.TrimPrefix(key, "party.")
			party, err := dm.storage.GetParty(ctx, instance)
			if err != nil {
				dm.logger.Debug("failed to load party", "instance", instance, "error", err)
				continue
			}
			// Only include forming or active parties
			if party.Status == PartyForming || party.Status == PartyActive {
				parties = append(parties, *party)
			}
		}
	}

	return parties, nil
}

// loadGuilds returns all guilds from storage.
func (dm *BaseDungeonMaster) loadGuilds(ctx context.Context) ([]Guild, error) {
	keys, err := dm.storage.KV().Keys(ctx)
	if err != nil {
		return nil, err
	}

	var guilds []Guild
	for _, key := range keys {
		if strings.HasPrefix(key, "guild.") {
			instance := strings.TrimPrefix(key, "guild.")
			guild, err := dm.storage.GetGuild(ctx, instance)
			if err != nil {
				dm.logger.Debug("failed to load guild", "instance", instance, "error", err)
				continue
			}
			guilds = append(guilds, *guild)
		}
	}

	return guilds, nil
}

// loadActiveBattles returns boss battles that are still in progress.
func (dm *BaseDungeonMaster) loadActiveBattles(ctx context.Context) ([]BossBattle, error) {
	keys, err := dm.storage.KV().Keys(ctx)
	if err != nil {
		return nil, err
	}

	var battles []BossBattle
	for _, key := range keys {
		if strings.HasPrefix(key, "battle.") {
			instance := strings.TrimPrefix(key, "battle.")
			battle, err := dm.storage.GetBattle(ctx, instance)
			if err != nil {
				dm.logger.Debug("failed to load battle", "instance", instance, "error", err)
				continue
			}
			// Only include active battles
			if battle.Status == BattleActive {
				battles = append(battles, *battle)
			}
		}
	}

	return battles, nil
}

// =============================================================================
// STATS COMPUTATION
// =============================================================================

// computeWorldStats calculates aggregate statistics from loaded entities.
// Note: quests parameter contains only active quests, so we count status indices
// separately to get accurate completion rates.
func (dm *BaseDungeonMaster) computeWorldStats(
	agents []Agent,
	quests []Quest,
	parties []Party,
	guilds []Guild,
) WorldStats {
	stats := WorldStats{}

	// Agent statistics
	for _, agent := range agents {
		switch agent.Status {
		case AgentIdle:
			stats.IdleAgents++
			stats.ActiveAgents++
		case AgentOnQuest, AgentInBattle:
			stats.ActiveAgents++
		case AgentCooldown:
			stats.CooldownAgents++
		case AgentRetired:
			stats.RetiredAgents++
		}
	}

	// Quest statistics - count from active quests
	for _, quest := range quests {
		switch quest.Status {
		case QuestPosted:
			stats.OpenQuests++
		case QuestClaimed, QuestInProgress, QuestInReview:
			stats.ActiveQuests++
		}
	}

	// Count completed and failed quests from storage indices for accurate stats
	ctx := context.Background()
	completedInstances, _ := dm.storage.ListQuestsByStatus(ctx, QuestCompleted)
	failedInstances, _ := dm.storage.ListQuestsByStatus(ctx, QuestFailed)
	cancelledInstances, _ := dm.storage.ListQuestsByStatus(ctx, QuestCancelled)

	completedCount := len(completedInstances)
	totalCount := len(quests) + completedCount + len(failedInstances) + len(cancelledInstances)

	// Compute completion rate
	if totalCount > 0 {
		stats.CompletionRate = float64(completedCount) / float64(totalCount)
	}

	// Compute average quality from agent stats (quality is tracked per-agent, not per-quest)
	if len(agents) > 0 {
		var totalQuality float64
		var qualifiedAgents int
		for _, agent := range agents {
			if agent.Stats.QuestsCompleted > 0 {
				totalQuality += agent.Stats.AvgQualityScore
				qualifiedAgents++
			}
		}
		if qualifiedAgents > 0 {
			stats.AvgQuality = totalQuality / float64(qualifiedAgents)
		}
	}

	// Party and guild counts
	stats.ActiveParties = len(parties)
	stats.ActiveGuilds = len(guilds)

	return stats
}

// =============================================================================
// FILTERED QUERIES
// =============================================================================

// GetIdleAgents returns all agents that are available to claim quests.
func (dm *BaseDungeonMaster) GetIdleAgents(ctx context.Context) ([]Agent, error) {
	agents, err := dm.loadAllAgents(ctx)
	if err != nil {
		return nil, err
	}

	var idle []Agent
	for _, agent := range agents {
		if agent.Status == AgentIdle {
			// Check cooldown
			if agent.CooldownUntil == nil {
				idle = append(idle, agent)
			}
		}
	}

	return idle, nil
}

// GetEscalatedQuests returns all quests that need DM attention.
func (dm *BaseDungeonMaster) GetEscalatedQuests(ctx context.Context) ([]Quest, error) {
	instances, err := dm.storage.ListQuestsByStatus(ctx, QuestEscalated)
	if err != nil {
		return nil, err
	}

	var quests []Quest
	for _, instance := range instances {
		quest, err := dm.storage.GetQuest(ctx, instance)
		if err != nil {
			continue
		}
		quests = append(quests, *quest)
	}

	return quests, nil
}

// GetPendingBattles returns boss battles awaiting verdict.
func (dm *BaseDungeonMaster) GetPendingBattles(ctx context.Context) ([]BossBattle, error) {
	battles, err := dm.loadActiveBattles(ctx)
	if err != nil {
		return nil, err
	}

	var pending []BossBattle
	for _, battle := range battles {
		if battle.Status == BattleActive && battle.Verdict == nil {
			pending = append(pending, battle)
		}
	}

	return pending, nil
}

// GetAgentsByTier returns agents filtered by trust tier.
func (dm *BaseDungeonMaster) GetAgentsByTier(ctx context.Context, tier TrustTier) ([]Agent, error) {
	agents, err := dm.loadAllAgents(ctx)
	if err != nil {
		return nil, err
	}

	var filtered []Agent
	for _, agent := range agents {
		if agent.Tier == tier {
			filtered = append(filtered, agent)
		}
	}

	return filtered, nil
}

// GetAgentsBySkill returns agents that have a specific skill.
func (dm *BaseDungeonMaster) GetAgentsBySkill(ctx context.Context, skill SkillTag) ([]Agent, error) {
	agents, err := dm.loadAllAgents(ctx)
	if err != nil {
		return nil, err
	}

	var filtered []Agent
	for _, agent := range agents {
		for _, s := range agent.Skills {
			if s == skill {
				filtered = append(filtered, agent)
				break
			}
		}
	}

	return filtered, nil
}

// GetQuestsByDifficulty returns quests filtered by difficulty level.
func (dm *BaseDungeonMaster) GetQuestsByDifficulty(ctx context.Context, difficulty QuestDifficulty) ([]Quest, error) {
	quests, err := dm.loadActiveQuests(ctx)
	if err != nil {
		return nil, err
	}

	var filtered []Quest
	for _, quest := range quests {
		if quest.Difficulty == difficulty {
			filtered = append(filtered, quest)
		}
	}

	return filtered, nil
}
