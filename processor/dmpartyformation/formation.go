package dmpartyformation

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/boidengine"
	"github.com/c360studio/semdragons/processor/partycoord"
)

// =============================================================================
// PARTY FORMATION ENGINE - Boids-based party composition
// =============================================================================
// Uses the boid engine to compute agent attractions and form parties
// using different strategies: Balanced, Specialist, Mentor, Minimal.
// =============================================================================

// PartyFormationEngine handles party composition using boid-based attractions.
type PartyFormationEngine struct {
	boids       *boidengine.DefaultBoidEngine
	graph       *semdragons.GraphClient
	boardConfig *domain.BoardConfig
}

// NewPartyFormationEngine creates a new party formation engine.
func NewPartyFormationEngine(boids *boidengine.DefaultBoidEngine, graph *semdragons.GraphClient, config *domain.BoardConfig) *PartyFormationEngine {
	return &PartyFormationEngine{
		boids:       boids,
		graph:       graph,
		boardConfig: config,
	}
}

// FormParty assembles a party for a quest using the specified strategy.
func (e *PartyFormationEngine) FormParty(
	ctx context.Context,
	quest *domain.Quest,
	strategy domain.PartyStrategy,
	availableAgents []agentprogression.Agent,
) (*partycoord.Party, error) {
	if len(availableAgents) == 0 {
		return nil, fmt.Errorf("no available agents for party formation")
	}

	// Compute attractions using boids
	rules := boidengine.DefaultBoidRules()
	attractions := e.boids.ComputeAttractions(availableAgents, []domain.Quest{*quest}, rules)

	switch strategy {
	case domain.PartyStrategyBalanced:
		return e.formBalancedParty(ctx, quest, availableAgents, attractions)
	case domain.PartyStrategySpecialist:
		return e.formSpecialistParty(ctx, quest, availableAgents, attractions)
	case domain.PartyStrategyMentor:
		return e.formMentorParty(ctx, quest, availableAgents, attractions)
	case domain.PartyStrategyMinimal:
		return e.formMinimalParty(ctx, quest, availableAgents, attractions)
	default:
		return e.formBalancedParty(ctx, quest, availableAgents, attractions)
	}
}

// =============================================================================
// BALANCED PARTY - Cover all skills with best-fit agents
// =============================================================================

func (e *PartyFormationEngine) formBalancedParty(
	ctx context.Context,
	quest *domain.Quest,
	agents []agentprogression.Agent,
	attractions []boidengine.QuestAttraction,
) (*partycoord.Party, error) {
	lead, err := e.selectLead(agents)
	if err != nil {
		return nil, err
	}

	attrMap := make(map[domain.AgentID]float64)
	for _, a := range attractions {
		attrMap[a.AgentID] = a.TotalScore
	}

	sortedAgents := make([]agentprogression.Agent, len(agents))
	copy(sortedAgents, agents)
	sort.Slice(sortedAgents, func(i, j int) bool {
		return attrMap[sortedAgents[i].ID] > attrMap[sortedAgents[j].ID]
	})

	neededSkills := make(map[domain.SkillTag]bool)
	for _, skill := range quest.RequiredSkills {
		neededSkills[skill] = true
	}

	for _, skill := range lead.GetSkillTags() {
		delete(neededSkills, skill)
	}

	members := []partycoord.PartyMember{
		{
			AgentID:  lead.ID,
			Role:     domain.RoleLead,
			Skills:   lead.GetSkillTags(),
			JoinedAt: time.Now(),
		},
	}

	for _, agent := range sortedAgents {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if agent.ID == lead.ID {
			continue
		}

		coversNeeded := false
		for _, skill := range agent.GetSkillTags() {
			if neededSkills[skill] {
				coversNeeded = true
				delete(neededSkills, skill)
			}
		}

		if coversNeeded || len(members) < quest.MinPartySize {
			members = append(members, partycoord.PartyMember{
				AgentID:  agent.ID,
				Role:     domain.RoleExecutor,
				Skills:   agent.GetSkillTags(),
				JoinedAt: time.Now(),
			})
		}

		if len(members) >= quest.MinPartySize && len(neededSkills) == 0 {
			break
		}
	}

	return e.createParty(quest, lead.ID, members)
}

// =============================================================================
// SPECIALIST PARTY - All agents from the same guild
// =============================================================================

func (e *PartyFormationEngine) formSpecialistParty(
	ctx context.Context,
	quest *domain.Quest,
	agents []agentprogression.Agent,
	attractions []boidengine.QuestAttraction,
) (*partycoord.Party, error) {
	guildAgents := make(map[domain.GuildID][]agentprogression.Agent)
	for _, agent := range agents {
		for _, guildID := range agent.Guilds {
			guildAgents[guildID] = append(guildAgents[guildID], agent)
		}
	}

	var bestGuild domain.GuildID
	var bestGuildAgents []agentprogression.Agent

	if quest.GuildPriority != nil {
		if gAgents, ok := guildAgents[*quest.GuildPriority]; ok && len(gAgents) >= quest.MinPartySize {
			bestGuild = *quest.GuildPriority
			bestGuildAgents = gAgents
		}
	}

	if bestGuild == "" {
		for guildID, gAgents := range guildAgents {
			if len(gAgents) > len(bestGuildAgents) {
				bestGuild = guildID
				bestGuildAgents = gAgents
			}
		}
	}

	if len(bestGuildAgents) == 0 {
		return e.formBalancedParty(ctx, quest, agents, attractions)
	}

	lead, err := e.selectLeadFromAgents(bestGuildAgents)
	if err != nil {
		lead, err = e.selectLead(agents)
		if err != nil {
			return nil, err
		}
	}

	members := []partycoord.PartyMember{
		{
			AgentID:  lead.ID,
			Role:     domain.RoleLead,
			Skills:   lead.GetSkillTags(),
			JoinedAt: time.Now(),
		},
	}

	for _, agent := range bestGuildAgents {
		if agent.ID == lead.ID {
			continue
		}
		if len(members) >= max(quest.MinPartySize, 3) {
			break
		}
		members = append(members, partycoord.PartyMember{
			AgentID:  agent.ID,
			Role:     domain.RoleExecutor,
			Skills:   agent.GetSkillTags(),
			JoinedAt: time.Now(),
		})
	}

	return e.createParty(quest, lead.ID, members)
}

// =============================================================================
// MENTOR PARTY - High-level lead with apprentices
// =============================================================================

func (e *PartyFormationEngine) formMentorParty(
	ctx context.Context,
	quest *domain.Quest,
	agents []agentprogression.Agent,
	_ []boidengine.QuestAttraction,
) (*partycoord.Party, error) {
	var lead *agentprogression.Agent
	for _, agent := range agents {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		perms := domain.TierPermissionsFor(agent.Tier)
		if perms.CanLeadParty {
			if lead == nil || agent.Level > lead.Level {
				agentCopy := agent
				lead = &agentCopy
			}
		}
	}

	if lead == nil {
		return nil, fmt.Errorf("no agent capable of leading a party")
	}

	var apprentices []agentprogression.Agent
	for _, agent := range agents {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if agent.ID == lead.ID {
			continue
		}
		if agent.Tier <= domain.TierJourneyman {
			apprentices = append(apprentices, agent)
		}
	}

	sort.Slice(apprentices, func(i, j int) bool {
		return apprentices[i].Level < apprentices[j].Level
	})

	members := []partycoord.PartyMember{
		{
			AgentID:  lead.ID,
			Role:     domain.RoleLead,
			Skills:   lead.GetSkillTags(),
			JoinedAt: time.Now(),
		},
	}

	for i := 0; i < len(apprentices) && len(members) < max(quest.MinPartySize, 2); i++ {
		members = append(members, partycoord.PartyMember{
			AgentID:  apprentices[i].ID,
			Role:     domain.RoleExecutor,
			Skills:   apprentices[i].GetSkillTags(),
			JoinedAt: time.Now(),
		})
	}

	return e.createParty(quest, lead.ID, members)
}

// =============================================================================
// MINIMAL PARTY - Smallest viable team
// =============================================================================

func (e *PartyFormationEngine) formMinimalParty(
	ctx context.Context,
	quest *domain.Quest,
	agents []agentprogression.Agent,
	attractions []boidengine.QuestAttraction,
) (*partycoord.Party, error) {
	lead, err := e.selectLead(agents)
	if err != nil {
		return nil, err
	}

	if quest.MinPartySize <= 1 || !quest.PartyRequired {
		members := []partycoord.PartyMember{
			{
				AgentID:  lead.ID,
				Role:     domain.RoleLead,
				Skills:   lead.GetSkillTags(),
				JoinedAt: time.Now(),
			},
		}
		return e.createParty(quest, lead.ID, members)
	}

	attrMap := make(map[domain.AgentID]float64)
	for _, a := range attractions {
		attrMap[a.AgentID] = a.TotalScore
	}

	type scoredAgent struct {
		agent agentprogression.Agent
		score float64
	}

	var scored []scoredAgent
	for _, agent := range agents {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if agent.ID != lead.ID {
			scored = append(scored, scoredAgent{agent: agent, score: attrMap[agent.ID]})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	members := []partycoord.PartyMember{
		{
			AgentID:  lead.ID,
			Role:     domain.RoleLead,
			Skills:   lead.GetSkillTags(),
			JoinedAt: time.Now(),
		},
	}

	for i := 0; i < len(scored) && len(members) < quest.MinPartySize; i++ {
		members = append(members, partycoord.PartyMember{
			AgentID:  scored[i].agent.ID,
			Role:     domain.RoleExecutor,
			Skills:   scored[i].agent.GetSkillTags(),
			JoinedAt: time.Now(),
		})
	}

	return e.createParty(quest, lead.ID, members)
}

// =============================================================================
// HELPER METHODS
// =============================================================================

func (e *PartyFormationEngine) selectLead(agents []agentprogression.Agent) (*agentprogression.Agent, error) {
	return e.selectLeadFromAgents(agents)
}

func (e *PartyFormationEngine) selectLeadFromAgents(agents []agentprogression.Agent) (*agentprogression.Agent, error) {
	var lead *agentprogression.Agent
	for _, agent := range agents {
		perms := domain.TierPermissionsFor(agent.Tier)
		if perms.CanLeadParty {
			if lead == nil || agent.Level > lead.Level {
				agentCopy := agent
				lead = &agentCopy
			}
		}
	}

	if lead == nil {
		return nil, fmt.Errorf("no agent capable of leading a party")
	}

	return lead, nil
}

func (e *PartyFormationEngine) createParty(quest *domain.Quest, leadID domain.AgentID, members []partycoord.PartyMember) (*partycoord.Party, error) {
	instance := domain.GenerateInstance()
	partyID := domain.PartyID(e.boardConfig.PartyEntityID(instance))

	return &partycoord.Party{
		ID:       partyID,
		Name:     fmt.Sprintf("Party for %s", quest.Title),
		Status:   domain.PartyForming,
		QuestID:  quest.ID,
		Lead:     leadID,
		Members:  members,
		FormedAt: time.Now(),
	}, nil
}

// =============================================================================
// RANKING AND SUGGESTIONS
// =============================================================================

// RankAgentsForQuest returns agents ranked by their suitability for a quest.
func (e *PartyFormationEngine) RankAgentsForQuest(
	agents []agentprogression.Agent,
	quest *domain.Quest,
) []boidengine.SuggestedClaim {
	rules := boidengine.DefaultBoidRules()
	attractions := e.boids.ComputeAttractions(agents, []domain.Quest{*quest}, rules)
	return e.boids.SuggestClaims(attractions)
}

// PartyMemberSuggestion represents a suggested party member with metadata.
type PartyMemberSuggestion struct {
	Agent          agentprogression.Agent      `json:"agent"`
	Score          float64           `json:"score"`
	CanLead        bool              `json:"can_lead"`
	SkillsCovered  []domain.SkillTag `json:"skills_covered"`
	GuildMatch     bool              `json:"guild_match"`
	RecommendedFor domain.PartyRole  `json:"recommended_for"`
}

// SuggestPartyMembers returns suggested party members with rankings.
func (e *PartyFormationEngine) SuggestPartyMembers(
	agents []agentprogression.Agent,
	quest *domain.Quest,
	strategy domain.PartyStrategy,
) ([]PartyMemberSuggestion, error) {
	_ = strategy // Reserved for future strategy-specific suggestions
	rules := boidengine.DefaultBoidRules()
	attractions := e.boids.ComputeAttractions(agents, []domain.Quest{*quest}, rules)

	attrMap := make(map[domain.AgentID]float64)
	for _, a := range attractions {
		attrMap[a.AgentID] = a.TotalScore
	}

	var suggestions []PartyMemberSuggestion

	for _, agent := range agents {
		perms := domain.TierPermissionsFor(agent.Tier)

		var coveredSkills []domain.SkillTag
		agentSkills := agent.GetSkillTags()
		for _, skill := range quest.RequiredSkills {
			if slices.Contains(agentSkills, skill) {
				coveredSkills = append(coveredSkills, skill)
			}
		}

		suggestion := PartyMemberSuggestion{
			Agent:          agent,
			Score:          attrMap[agent.ID],
			CanLead:        perms.CanLeadParty,
			SkillsCovered:  coveredSkills,
			GuildMatch:     e.isGuildMatch(agent, quest),
			RecommendedFor: e.recommendRole(agent, perms),
		}

		suggestions = append(suggestions, suggestion)
	}

	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Score > suggestions[j].Score
	})

	return suggestions, nil
}

func (e *PartyFormationEngine) isGuildMatch(agent agentprogression.Agent, quest *domain.Quest) bool {
	if quest.GuildPriority == nil {
		return false
	}
	for _, guildID := range agent.Guilds {
		if guildID == *quest.GuildPriority {
			return true
		}
	}
	return false
}

func (e *PartyFormationEngine) recommendRole(agent agentprogression.Agent, perms domain.TierPermissions) domain.PartyRole {
	if perms.CanLeadParty {
		return domain.RoleLead
	}
	for _, skill := range agent.GetSkillTags() {
		if skill == domain.SkillCodeReview {
			return domain.RoleReviewer
		}
		if skill == domain.SkillResearch {
			return domain.RoleScout
		}
	}
	return domain.RoleExecutor
}
