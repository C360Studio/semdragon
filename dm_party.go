package semdragons

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"time"
)

// =============================================================================
// PARTY FORMATION ENGINE - Boids-based party composition
// =============================================================================
// PartyFormationEngine uses the boid engine to compute agent attractions
// and form parties using different strategies:
// - Balanced: Mix of skills covering quest requirements
// - Specialist: All agents from the same guild
// - Mentor: High-level lead with apprentice members
// - Minimal: Smallest viable team
// =============================================================================

// PartyFormationEngine handles party composition using boid-based attractions.
type PartyFormationEngine struct {
	boids   *DefaultBoidEngine
	storage *Storage
	config  *BoardConfig
}

// NewPartyFormationEngine creates a new party formation engine.
func NewPartyFormationEngine(boids *DefaultBoidEngine, storage *Storage) *PartyFormationEngine {
	return &PartyFormationEngine{
		boids:   boids,
		storage: storage,
		config:  storage.Config(),
	}
}

// FormParty assembles a party for a quest using the specified strategy.
func (e *PartyFormationEngine) FormParty(
	ctx context.Context,
	quest *Quest,
	strategy PartyStrategy,
	availableAgents []Agent,
) (*Party, error) {
	if len(availableAgents) == 0 {
		return nil, fmt.Errorf("no available agents for party formation")
	}

	// Compute attractions using boids
	rules := DefaultBoidRules()
	attractions := e.boids.ComputeAttractions(availableAgents, []Quest{*quest}, rules)

	switch strategy {
	case PartyStrategyBalanced:
		return e.formBalancedParty(ctx, quest, availableAgents, attractions)
	case PartyStrategySpecialist:
		return e.formSpecialistParty(ctx, quest, availableAgents, attractions)
	case PartyStrategyMentor:
		return e.formMentorParty(ctx, quest, availableAgents, attractions)
	case PartyStrategyMinimal:
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
	quest *Quest,
	agents []Agent,
	attractions []QuestAttraction,
) (*Party, error) {
	// Find a lead first (must have CanLeadParty permission)
	lead, err := e.selectLead(agents)
	if err != nil {
		return nil, err
	}

	// Build attraction map for quick lookup
	attrMap := make(map[AgentID]float64)
	for _, a := range attractions {
		attrMap[a.AgentID] = a.TotalScore
	}

	// Sort agents by attraction score
	sortedAgents := make([]Agent, len(agents))
	copy(sortedAgents, agents)
	sort.Slice(sortedAgents, func(i, j int) bool {
		return attrMap[sortedAgents[i].ID] > attrMap[sortedAgents[j].ID]
	})

	// Track which skills we need to cover
	neededSkills := make(map[SkillTag]bool)
	for _, skill := range quest.RequiredSkills {
		neededSkills[skill] = true
	}

	// Remove skills the lead already covers
	for _, skill := range lead.Skills {
		delete(neededSkills, skill)
	}

	// Select members to cover remaining skills
	members := []PartyMember{
		{
			AgentID:  lead.ID,
			Role:     RoleLead,
			Skills:   lead.Skills,
			JoinedAt: time.Now(),
		},
	}

	for _, agent := range sortedAgents {
		// Respect context cancellation during iteration
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if agent.ID == lead.ID {
			continue
		}

		// Check if this agent covers any needed skills
		coversNeeded := false
		for _, skill := range agent.Skills {
			if neededSkills[skill] {
				coversNeeded = true
				delete(neededSkills, skill)
			}
		}

		if coversNeeded || len(members) < quest.MinPartySize {
			members = append(members, PartyMember{
				AgentID:  agent.ID,
				Role:     RoleExecutor,
				Skills:   agent.Skills,
				JoinedAt: time.Now(),
			})
		}

		// Stop if we have enough members and all skills covered
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
	quest *Quest,
	agents []Agent,
	attractions []QuestAttraction,
) (*Party, error) {
	// Group agents by guild
	guildAgents := make(map[GuildID][]Agent)
	for _, agent := range agents {
		for _, guildID := range agent.Guilds {
			guildAgents[guildID] = append(guildAgents[guildID], agent)
		}
	}

	// Find the best guild (most agents that can handle this quest)
	var bestGuild GuildID
	var bestGuildAgents []Agent

	// If quest has guild priority, prefer that guild
	if quest.GuildPriority != nil {
		if agents, ok := guildAgents[*quest.GuildPriority]; ok && len(agents) >= quest.MinPartySize {
			bestGuild = *quest.GuildPriority
			bestGuildAgents = agents
		}
	}

	// Otherwise, find guild with most qualified agents
	if bestGuild == "" {
		for guildID, gAgents := range guildAgents {
			if len(gAgents) > len(bestGuildAgents) {
				bestGuild = guildID
				bestGuildAgents = gAgents
			}
		}
	}

	if len(bestGuildAgents) == 0 {
		// Fall back to balanced if no guild has enough agents
		return e.formBalancedParty(ctx, quest, agents, attractions)
	}

	// Find a lead from this guild
	lead, err := e.selectLeadFromAgents(bestGuildAgents)
	if err != nil {
		// Try to find any lead and add guild members
		lead, err = e.selectLead(agents)
		if err != nil {
			return nil, err
		}
	}

	// Build members from guild agents
	members := []PartyMember{
		{
			AgentID:  lead.ID,
			Role:     RoleLead,
			Skills:   lead.Skills,
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
		members = append(members, PartyMember{
			AgentID:  agent.ID,
			Role:     RoleExecutor,
			Skills:   agent.Skills,
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
	quest *Quest,
	agents []Agent,
	_ []QuestAttraction, // Mentor strategy uses level-based selection, not boid attractions
) (*Party, error) {
	// Find the highest-level agent that can lead
	var lead *Agent
	for _, agent := range agents {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		perms := TierPerms[agent.Tier]
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

	// Find apprentices (lower-tier agents)
	var apprentices []Agent
	for _, agent := range agents {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if agent.ID == lead.ID {
			continue
		}
		// Prefer lower-level agents for mentoring
		if agent.Tier <= TierJourneyman {
			apprentices = append(apprentices, agent)
		}
	}

	// Sort apprentices by level (lowest first for mentoring opportunity)
	sort.Slice(apprentices, func(i, j int) bool {
		return apprentices[i].Level < apprentices[j].Level
	})

	// Build party with lead and apprentices
	members := []PartyMember{
		{
			AgentID:  lead.ID,
			Role:     RoleLead,
			Skills:   lead.Skills,
			JoinedAt: time.Now(),
		},
	}

	// Add apprentices up to min party size
	for i := 0; i < len(apprentices) && len(members) < max(quest.MinPartySize, 2); i++ {
		members = append(members, PartyMember{
			AgentID:  apprentices[i].ID,
			Role:     RoleExecutor,
			Skills:   apprentices[i].Skills,
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
	quest *Quest,
	agents []Agent,
	attractions []QuestAttraction,
) (*Party, error) {
	// Find a lead
	lead, err := e.selectLead(agents)
	if err != nil {
		return nil, err
	}

	// Check if lead alone can handle it (no min party size requirement)
	if quest.MinPartySize <= 1 || !quest.PartyRequired {
		// Solo party with just the lead
		members := []PartyMember{
			{
				AgentID:  lead.ID,
				Role:     RoleLead,
				Skills:   lead.Skills,
				JoinedAt: time.Now(),
			},
		}
		return e.createParty(quest, lead.ID, members)
	}

	// Build attraction map
	attrMap := make(map[AgentID]float64)
	for _, a := range attractions {
		attrMap[a.AgentID] = a.TotalScore
	}

	// Get top N agents by attraction where N = MinPartySize
	type scoredAgent struct {
		agent Agent
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

	// Build minimal party
	members := []PartyMember{
		{
			AgentID:  lead.ID,
			Role:     RoleLead,
			Skills:   lead.Skills,
			JoinedAt: time.Now(),
		},
	}

	for i := 0; i < len(scored) && len(members) < quest.MinPartySize; i++ {
		members = append(members, PartyMember{
			AgentID:  scored[i].agent.ID,
			Role:     RoleExecutor,
			Skills:   scored[i].agent.Skills,
			JoinedAt: time.Now(),
		})
	}

	return e.createParty(quest, lead.ID, members)
}

// =============================================================================
// HELPER METHODS
// =============================================================================

// selectLead finds the best agent to lead a party.
// Prioritizes: highest level with CanLeadParty permission.
func (e *PartyFormationEngine) selectLead(agents []Agent) (*Agent, error) {
	return e.selectLeadFromAgents(agents)
}

// selectLeadFromAgents selects a lead from a specific list of agents.
func (e *PartyFormationEngine) selectLeadFromAgents(agents []Agent) (*Agent, error) {
	var lead *Agent
	for _, agent := range agents {
		perms := TierPerms[agent.Tier]
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

// createParty constructs a Party object from components.
func (e *PartyFormationEngine) createParty(quest *Quest, leadID AgentID, members []PartyMember) (*Party, error) {
	instance := GenerateInstance()
	partyID := PartyID(e.config.PartyEntityID(instance))

	return &Party{
		ID:       partyID,
		Name:     fmt.Sprintf("Party for %s", quest.Title),
		Status:   PartyForming,
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
	agents []Agent,
	quest *Quest,
) []SuggestedClaim {
	rules := DefaultBoidRules()
	attractions := e.boids.ComputeAttractions(agents, []Quest{*quest}, rules)
	return e.boids.SuggestClaims(attractions)
}

// SuggestPartyMembers returns suggested party members with rankings.
// The strategy parameter is reserved for future use to tailor suggestions
// based on formation approach (e.g., prioritize mentors for mentor strategy).
func (e *PartyFormationEngine) SuggestPartyMembers(
	agents []Agent,
	quest *Quest,
	strategy PartyStrategy,
) ([]PartyMemberSuggestion, error) {
	// TODO: Use strategy to adjust suggestion scoring when implemented
	_ = strategy
	rules := DefaultBoidRules()
	attractions := e.boids.ComputeAttractions(agents, []Quest{*quest}, rules)

	// Build attraction map
	attrMap := make(map[AgentID]float64)
	for _, a := range attractions {
		attrMap[a.AgentID] = a.TotalScore
	}

	var suggestions []PartyMemberSuggestion

	for _, agent := range agents {
		perms := TierPerms[agent.Tier]

		// Calculate skill coverage
		var coveredSkills []SkillTag
		for _, skill := range quest.RequiredSkills {
			if slices.Contains(agent.Skills, skill) {
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

	// Sort by score descending
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Score > suggestions[j].Score
	})

	return suggestions, nil
}

// PartyMemberSuggestion represents a suggested party member with metadata.
type PartyMemberSuggestion struct {
	Agent          Agent      `json:"agent"`
	Score          float64    `json:"score"`
	CanLead        bool       `json:"can_lead"`
	SkillsCovered  []SkillTag `json:"skills_covered"`
	GuildMatch     bool       `json:"guild_match"`
	RecommendedFor PartyRole  `json:"recommended_for"`
}

func (e *PartyFormationEngine) isGuildMatch(agent Agent, quest *Quest) bool {
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

func (e *PartyFormationEngine) recommendRole(agent Agent, perms TierPermissions) PartyRole {
	if perms.CanLeadParty {
		return RoleLead
	}
	// Check for reviewer skills
	for _, skill := range agent.Skills {
		if skill == SkillCodeReview {
			return RoleReviewer
		}
	}
	// Check for research/scout skills
	for _, skill := range agent.Skills {
		if skill == SkillResearch {
			return RoleScout
		}
	}
	return RoleExecutor
}
