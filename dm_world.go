package semdragons

import (
	"context"
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
		agents = []*Agent{}
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

	// Convert agents to values for WorldState
	agentValues := make([]Agent, 0, len(agents))
	for _, a := range agents {
		if a != nil {
			agentValues = append(agentValues, *a)
		}
	}

	return &WorldState{
		Agents:  agentValues,
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

// loadActiveQuests returns quests that are not yet completed or cancelled.
func (dm *BaseDungeonMaster) loadActiveQuests(ctx context.Context) ([]Quest, error) {
	entities, err := dm.graph.ListQuestsByPrefix(ctx, 1000)
	if err != nil {
		return nil, err
	}

	activeStatuses := map[QuestStatus]bool{
		QuestPosted:     true,
		QuestClaimed:    true,
		QuestInProgress: true,
		QuestInReview:   true,
		QuestEscalated:  true,
	}

	var quests []Quest
	for _, entity := range entities {
		quest := QuestFromEntityState(&entity)
		if quest != nil && activeStatuses[quest.Status] {
			quests = append(quests, *quest)
		}
	}

	return quests, nil
}

// loadActiveParties returns parties that are forming or active.
func (dm *BaseDungeonMaster) loadActiveParties(ctx context.Context) ([]Party, error) {
	prefix := dm.config.TypePrefix(EntityTypeParty)
	entities, err := dm.graph.QueryByPrefix(ctx, prefix, 1000)
	if err != nil {
		return nil, err
	}

	var parties []Party
	for _, entity := range entities {
		party := PartyFromEntityState(&entity)
		if party != nil {
			// Only include forming or active parties
			if party.Status == PartyForming || party.Status == PartyActive {
				parties = append(parties, *party)
			}
		}
	}

	return parties, nil
}

// loadGuilds returns all guilds from the graph.
func (dm *BaseDungeonMaster) loadGuilds(ctx context.Context) ([]Guild, error) {
	prefix := dm.config.TypePrefix(EntityTypeGuild)
	entities, err := dm.graph.QueryByPrefix(ctx, prefix, 1000)
	if err != nil {
		return nil, err
	}

	var guilds []Guild
	for _, entity := range entities {
		guild := GuildFromEntityState(&entity)
		if guild != nil {
			guilds = append(guilds, *guild)
		}
	}

	return guilds, nil
}

// loadActiveBattles returns boss battles that are still in progress.
func (dm *BaseDungeonMaster) loadActiveBattles(ctx context.Context) ([]BossBattle, error) {
	prefix := dm.config.TypePrefix(EntityTypeBattle)
	entities, err := dm.graph.QueryByPrefix(ctx, prefix, 1000)
	if err != nil {
		return nil, err
	}

	var battles []BossBattle
	for _, entity := range entities {
		battle := BattleFromEntityState(&entity)
		if battle != nil && battle.Status == BattleActive {
			battles = append(battles, *battle)
		}
	}

	return battles, nil
}

// =============================================================================
// STATS COMPUTATION
// =============================================================================

// computeWorldStats calculates aggregate statistics from loaded entities.
func (dm *BaseDungeonMaster) computeWorldStats(
	agents []*Agent,
	quests []Quest,
	parties []Party,
	guilds []Guild,
) WorldStats {
	stats := WorldStats{}

	// Agent statistics
	for _, agent := range agents {
		if agent == nil {
			continue
		}
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
	var completedCount, failedCount int
	for _, quest := range quests {
		switch quest.Status {
		case QuestPosted:
			stats.OpenQuests++
		case QuestClaimed, QuestInProgress, QuestInReview:
			stats.ActiveQuests++
		case QuestCompleted:
			completedCount++
		case QuestFailed:
			failedCount++
		}
	}

	// Compute completion rate
	totalCount := len(quests)
	if totalCount > 0 {
		stats.CompletionRate = float64(completedCount) / float64(totalCount)
	}

	// Compute average quality from agent stats
	if len(agents) > 0 {
		var totalQuality float64
		var qualifiedAgents int
		for _, agent := range agents {
			if agent != nil && agent.Stats.QuestsCompleted > 0 {
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
		if agent != nil && agent.Status == AgentIdle {
			// Check cooldown
			if agent.CooldownUntil == nil {
				idle = append(idle, *agent)
			}
		}
	}

	return idle, nil
}

// GetEscalatedQuests returns all quests that need DM attention.
func (dm *BaseDungeonMaster) GetEscalatedQuests(ctx context.Context) ([]Quest, error) {
	quests, err := dm.loadActiveQuests(ctx)
	if err != nil {
		return nil, err
	}

	var escalated []Quest
	for _, quest := range quests {
		if quest.Status == QuestEscalated {
			escalated = append(escalated, quest)
		}
	}

	return escalated, nil
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
		if agent != nil && agent.Tier == tier {
			filtered = append(filtered, *agent)
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
		if agent != nil && agent.HasSkill(skill) {
			filtered = append(filtered, *agent)
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
