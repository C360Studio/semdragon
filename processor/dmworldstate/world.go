package dmworldstate

import (
	"context"
	"log/slog"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/partycoord"
	"github.com/c360studio/semdragons/processor/bossbattle"
	"github.com/c360studio/semdragons/processor/agentprogression"
)

// =============================================================================
// WORLD STATE AGGREGATOR
// =============================================================================
// Aggregates all game entities into a complete WorldState snapshot.
// =============================================================================

// WorldStateAggregator provides world state aggregation.
type WorldStateAggregator struct {
	graph       *semdragons.GraphClient
	maxEntities int
	logger      *slog.Logger
}

// NewWorldStateAggregator creates a new world state aggregator.
func NewWorldStateAggregator(graph *semdragons.GraphClient, maxEntities int, logger *slog.Logger) *WorldStateAggregator {
	if maxEntities <= 0 {
		maxEntities = 1000
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &WorldStateAggregator{
		graph:       graph,
		maxEntities: maxEntities,
		logger:      logger,
	}
}

// WorldState returns the complete state of the game world.
func (w *WorldStateAggregator) WorldState(ctx context.Context) (*domain.WorldState, error) {
	agents, err := w.loadAllAgents(ctx)
	if err != nil {
		w.logger.Warn("failed to load agents for world state", "error", err)
		agents = []*agentprogression.Agent{}
	}

	quests, err := w.loadActiveQuests(ctx)
	if err != nil {
		w.logger.Warn("failed to load quests for world state", "error", err)
		quests = []domain.Quest{}
	}

	parties, err := w.loadActiveParties(ctx)
	if err != nil {
		w.logger.Warn("failed to load parties for world state", "error", err)
		parties = []partycoord.Party{}
	}

	guilds, err := w.loadGuilds(ctx)
	if err != nil {
		w.logger.Warn("failed to load guilds for world state", "error", err)
		guilds = []domain.Guild{}
	}

	battles, err := w.loadActiveBattles(ctx)
	if err != nil {
		w.logger.Warn("failed to load battles for world state", "error", err)
		battles = []bossbattle.BossBattle{}
	}

	stats := w.computeWorldStats(agents, quests, parties, guilds)

	// Convert to any slices for domain.WorldState
	agentValues := make([]any, 0, len(agents))
	for _, a := range agents {
		if a != nil {
			agentValues = append(agentValues, *a)
		}
	}

	questValues := make([]any, 0, len(quests))
	for _, q := range quests {
		questValues = append(questValues, q)
	}

	partyValues := make([]any, 0, len(parties))
	for _, p := range parties {
		partyValues = append(partyValues, p)
	}

	guildValues := make([]any, 0, len(guilds))
	for _, g := range guilds {
		guildValues = append(guildValues, g)
	}

	battleValues := make([]any, 0, len(battles))
	for _, b := range battles {
		battleValues = append(battleValues, b)
	}

	return &domain.WorldState{
		Agents:  agentValues,
		Quests:  questValues,
		Parties: partyValues,
		Guilds:  guildValues,
		Battles: battleValues,
		Stats:   stats,
	}, nil
}

// =============================================================================
// ENTITY LOADING
// =============================================================================

func (w *WorldStateAggregator) loadAllAgents(ctx context.Context) ([]*agentprogression.Agent, error) {
	entities, err := w.graph.ListAgentsByPrefix(ctx, w.maxEntities)
	if err != nil {
		return nil, err
	}

	agents := make([]*agentprogression.Agent, 0, len(entities))
	for _, entity := range entities {
		agent := agentprogression.AgentFromEntityState(&entity)
		if agent != nil {
			agents = append(agents, agent)
		}
	}
	return agents, nil
}

func (w *WorldStateAggregator) loadActiveQuests(ctx context.Context) ([]domain.Quest, error) {
	entities, err := w.graph.ListQuestsByPrefix(ctx, w.maxEntities)
	if err != nil {
		return nil, err
	}

	activeStatuses := map[domain.QuestStatus]bool{
		domain.QuestPosted:     true,
		domain.QuestClaimed:    true,
		domain.QuestInProgress: true,
		domain.QuestInReview:   true,
		domain.QuestEscalated:  true,
	}

	var quests []domain.Quest
	for _, entity := range entities {
		quest := domain.QuestFromEntityState(&entity)
		if quest != nil && activeStatuses[quest.Status] {
			quests = append(quests, *quest)
		}
	}

	return quests, nil
}

func (w *WorldStateAggregator) loadActiveParties(ctx context.Context) ([]partycoord.Party, error) {
	entities, err := w.graph.ListPartiesByPrefix(ctx, w.maxEntities)
	if err != nil {
		return nil, err
	}

	var parties []partycoord.Party
	for _, entity := range entities {
		party := partycoord.PartyFromEntityState(&entity)
		if party != nil {
			if party.Status == domain.PartyForming || party.Status == domain.PartyActive {
				parties = append(parties, *party)
			}
		}
	}

	return parties, nil
}

func (w *WorldStateAggregator) loadGuilds(ctx context.Context) ([]domain.Guild, error) {
	entities, err := w.graph.ListGuildsByPrefix(ctx, w.maxEntities)
	if err != nil {
		return nil, err
	}

	var guilds []domain.Guild
	for _, entity := range entities {
		guild := domain.GuildFromEntityState(&entity)
		if guild != nil {
			guilds = append(guilds, *guild)
		}
	}

	return guilds, nil
}

func (w *WorldStateAggregator) loadActiveBattles(ctx context.Context) ([]bossbattle.BossBattle, error) {
	entities, err := w.graph.ListEntitiesByType(ctx, domain.EntityTypeBattle, w.maxEntities)
	if err != nil {
		return nil, err
	}

	var battles []bossbattle.BossBattle
	for _, entity := range entities {
		battle := bossbattle.BattleFromEntityState(&entity)
		if battle != nil && battle.Status == domain.BattleActive {
			battles = append(battles, *battle)
		}
	}

	return battles, nil
}

// =============================================================================
// STATS COMPUTATION
// =============================================================================

func (w *WorldStateAggregator) computeWorldStats(
	agents []*agentprogression.Agent,
	quests []domain.Quest,
	parties []partycoord.Party,
	guilds []domain.Guild,
) domain.WorldStats {
	stats := domain.WorldStats{}

	// Agent statistics
	for _, agent := range agents {
		if agent == nil {
			continue
		}
		switch agent.Status {
		case domain.AgentIdle:
			stats.IdleAgents++
			stats.ActiveAgents++
		case domain.AgentOnQuest, domain.AgentInBattle:
			stats.ActiveAgents++
		case domain.AgentCooldown:
			stats.CooldownAgents++
		case domain.AgentRetired:
			stats.RetiredAgents++
		}
	}

	// Quest statistics
	var completedCount int
	for _, quest := range quests {
		switch quest.Status {
		case domain.QuestPosted:
			stats.OpenQuests++
		case domain.QuestClaimed, domain.QuestInProgress, domain.QuestInReview:
			stats.ActiveQuests++
		case domain.QuestCompleted:
			completedCount++
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
func (w *WorldStateAggregator) GetIdleAgents(ctx context.Context) ([]agentprogression.Agent, error) {
	agents, err := w.loadAllAgents(ctx)
	if err != nil {
		return nil, err
	}

	var idle []agentprogression.Agent
	for _, agent := range agents {
		if agent != nil && agent.Status == domain.AgentIdle {
			if agent.CooldownUntil == nil {
				idle = append(idle, *agent)
			}
		}
	}

	return idle, nil
}

// GetEscalatedQuests returns all quests that need DM attention.
func (w *WorldStateAggregator) GetEscalatedQuests(ctx context.Context) ([]domain.Quest, error) {
	quests, err := w.loadActiveQuests(ctx)
	if err != nil {
		return nil, err
	}

	var escalated []domain.Quest
	for _, quest := range quests {
		if quest.Status == domain.QuestEscalated {
			escalated = append(escalated, quest)
		}
	}

	return escalated, nil
}

// GetPendingBattles returns boss battles awaiting verdict.
func (w *WorldStateAggregator) GetPendingBattles(ctx context.Context) ([]bossbattle.BossBattle, error) {
	battles, err := w.loadActiveBattles(ctx)
	if err != nil {
		return nil, err
	}

	var pending []bossbattle.BossBattle
	for _, battle := range battles {
		if battle.Status == domain.BattleActive && battle.Verdict == nil {
			pending = append(pending, battle)
		}
	}

	return pending, nil
}

// GetAgentsByTier returns agents filtered by trust tier.
func (w *WorldStateAggregator) GetAgentsByTier(ctx context.Context, tier domain.TrustTier) ([]agentprogression.Agent, error) {
	agents, err := w.loadAllAgents(ctx)
	if err != nil {
		return nil, err
	}

	var filtered []agentprogression.Agent
	for _, agent := range agents {
		if agent != nil && agent.Tier == tier {
			filtered = append(filtered, *agent)
		}
	}

	return filtered, nil
}

// GetAgentsBySkill returns agents that have a specific skill.
func (w *WorldStateAggregator) GetAgentsBySkill(ctx context.Context, skill domain.SkillTag) ([]agentprogression.Agent, error) {
	agents, err := w.loadAllAgents(ctx)
	if err != nil {
		return nil, err
	}

	var filtered []agentprogression.Agent
	for _, agent := range agents {
		if agent != nil && agent.HasSkill(skill) {
			filtered = append(filtered, *agent)
		}
	}

	return filtered, nil
}

// GetQuestsByDifficulty returns quests filtered by difficulty level.
func (w *WorldStateAggregator) GetQuestsByDifficulty(ctx context.Context, difficulty domain.QuestDifficulty) ([]domain.Quest, error) {
	quests, err := w.loadActiveQuests(ctx)
	if err != nil {
		return nil, err
	}

	var filtered []domain.Quest
	for _, quest := range quests {
		if quest.Difficulty == difficulty {
			filtered = append(filtered, quest)
		}
	}

	return filtered, nil
}
