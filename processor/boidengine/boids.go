// Package boidengine implements boid flocking rules for emergent agent behavior.
// Agents flock toward quests they're best suited for using six rules:
// Separation, Alignment, Cohesion, Hunger, Affinity, and Caution.
package boidengine

import (
	"sort"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
)

// =============================================================================
// BOID ENGINE - Emergent behavior for quest claiming
// =============================================================================
// The boid engine computes "attractions" between agents and available quests.
// Each agent-quest pair gets scored based on multiple factors.
// =============================================================================

// BoidEngine computes quest attractions using flocking rules.
type BoidEngine interface {
	// ComputeAttractions calculates attraction scores for all agent-quest pairs.
	// Returns a slice of attractions sorted by total score descending.
	ComputeAttractions(agents []agentprogression.Agent, quests []domain.Quest, rules BoidRules) []QuestAttraction

	// SuggestClaims returns the best quest for each agent to claim.
	// Uses greedy assignment - highest scoring agent-quest pair first.
	SuggestClaims(attractions []QuestAttraction) []SuggestedClaim

	// SuggestTopN returns up to n ranked quest suggestions per agent.
	// Unlike SuggestClaims, quests are NOT removed from the pool —
	// multiple agents may receive the same quest as a suggestion.
	SuggestTopN(attractions []QuestAttraction, n int) map[domain.AgentID][]SuggestedClaim

	// UpdateRules allows dynamic adjustment of rule weights.
	UpdateRules(rules BoidRules)
}

// BoidRules configures the weights for each flocking rule.
type BoidRules struct {
	// Separation: Avoid quests that other agents are already clustering toward.
	// Higher = stronger repulsion from crowded quests.
	SeparationWeight float64 `json:"separation_weight"`

	// Alignment: Prefer quests similar to what successful peers are doing.
	// Higher = stronger influence from peer behavior.
	AlignmentWeight float64 `json:"alignment_weight"`

	// Cohesion: Move toward clusters of matching skill requirements.
	// Higher = stronger pull toward skill-dense quest areas.
	CohesionWeight float64 `json:"cohesion_weight"`

	// Hunger: Urgency increases with idle time.
	// Higher = idle agents are more aggressive about claiming.
	HungerWeight float64 `json:"hunger_weight"`

	// Affinity: Strong pull toward quests matching skills and guild.
	// Higher = skill/guild match matters more.
	AffinityWeight float64 `json:"affinity_weight"`

	// Caution: Avoid quests above agent's level/tier.
	// Higher = more conservative quest selection.
	CautionWeight float64 `json:"caution_weight"`

	// NeighborRadius: How many nearby agents to consider for alignment/separation.
	NeighborRadius int `json:"neighbor_radius"`
}

// QuestAttraction represents an agent's computed attraction to a quest.
type QuestAttraction struct {
	AgentID    domain.AgentID `json:"agent_id"`
	QuestID    domain.QuestID `json:"quest_id"`
	TotalScore float64        `json:"total_score"`

	// Individual rule contributions (for debugging/explanation)
	SeparationScore float64 `json:"separation_score"`
	AlignmentScore  float64 `json:"alignment_score"`
	CohesionScore   float64 `json:"cohesion_score"`
	HungerScore     float64 `json:"hunger_score"`
	AffinityScore   float64 `json:"affinity_score"`
	CautionScore    float64 `json:"caution_score"`
}

// SuggestedClaim is a recommendation for an agent to claim a quest.
type SuggestedClaim struct {
	AgentID    domain.AgentID `json:"agent_id"`
	QuestID    domain.QuestID `json:"quest_id"`
	Score      float64        `json:"score"`
	Confidence float64        `json:"confidence"` // How much better than alternatives
	Reason     string         `json:"reason"`     // Human-readable explanation
}

// DefaultBoidRules returns sensible defaults for boid weights.
func DefaultBoidRules() BoidRules {
	return BoidRules{
		SeparationWeight: 1.0,
		AlignmentWeight:  0.8,
		CohesionWeight:   0.6,
		HungerWeight:     1.2,
		AffinityWeight:   1.5,
		CautionWeight:    0.9,
		NeighborRadius:   5,
	}
}

// =============================================================================
// DEFAULT BOID ENGINE IMPLEMENTATION
// =============================================================================

// DefaultBoidEngine implements BoidEngine with standard flocking behavior.
type DefaultBoidEngine struct {
	rules  BoidRules
	guilds map[domain.GuildID]*domain.Guild // Guild context for rank/reputation lookups
}

// SetGuildContext provides guild data for rank and reputation calculations.
// Called by the component before each computation cycle.
func (e *DefaultBoidEngine) SetGuildContext(guilds map[domain.GuildID]*domain.Guild) {
	e.guilds = guilds
}

// NewDefaultBoidEngine creates a new boid engine with default rules.
func NewDefaultBoidEngine() *DefaultBoidEngine {
	return &DefaultBoidEngine{rules: DefaultBoidRules()}
}

// UpdateRules updates the engine's rules.
func (e *DefaultBoidEngine) UpdateRules(rules BoidRules) {
	e.rules = rules
}

// ComputeAttractions calculates attraction scores for all agent-quest pairs.
func (e *DefaultBoidEngine) ComputeAttractions(agents []agentprogression.Agent, quests []domain.Quest, rules BoidRules) []QuestAttraction {
	if len(agents) == 0 || len(quests) == 0 {
		return nil
	}

	// Build indices for quick lookups
	questCrowding := e.computeQuestCrowding(agents, quests)
	skillClusters := e.computeSkillClusters(quests)

	var attractions []QuestAttraction

	for i := range agents {
		agent := &agents[i]
		if agent.Status != domain.AgentIdle {
			continue // Only idle agents can claim
		}

		for j := range quests {
			quest := &quests[j]
			attr := e.computeAttraction(agent, quest, agents, rules, questCrowding, skillClusters)
			if attr.TotalScore > 0 {
				attractions = append(attractions, attr)
			}
		}
	}

	// Sort by total score descending
	sort.Slice(attractions, func(i, j int) bool {
		return attractions[i].TotalScore > attractions[j].TotalScore
	})

	return attractions
}

// computeAttraction calculates attraction between a single agent and quest.
// The allAgents parameter is reserved for advanced peer-based boid rules
// (e.g., velocity matching, leader following) that will use direct peer proximity.
// Currently peer influence is captured via crowding and skillClusters.
func (e *DefaultBoidEngine) computeAttraction(
	agent *agentprogression.Agent,
	quest *domain.Quest,
	allAgents []agentprogression.Agent,
	rules BoidRules,
	crowding map[domain.QuestID]int,
	skillClusters map[domain.SkillTag]int,
) QuestAttraction {
	// Track allAgents for future peer calculations (velocity matching, etc.)
	_ = len(allAgents)

	attr := QuestAttraction{
		AgentID: agent.ID,
		QuestID: quest.ID,
	}

	// Rule 1: Separation - avoid crowded quests
	crowd := crowding[quest.ID]
	if crowd > 0 {
		attr.SeparationScore = -float64(crowd) * 0.5 * rules.SeparationWeight
	}

	// Rule 2: Alignment - follow successful peers (simplified: prefer popular skill areas)
	for _, skill := range quest.RequiredSkills {
		if agent.HasSkill(skill) {
			attr.AlignmentScore += float64(skillClusters[skill]) * 0.1 * rules.AlignmentWeight
		}
	}

	// Rule 3: Cohesion - move toward skill clusters
	matchingSkills := 0
	for _, skill := range quest.RequiredSkills {
		if agent.HasSkill(skill) {
			matchingSkills++
		}
	}
	if len(quest.RequiredSkills) > 0 {
		attr.CohesionScore = float64(matchingSkills) / float64(len(quest.RequiredSkills)) * rules.CohesionWeight
	}

	// Rule 4: Hunger - idle time increases urgency
	attr.HungerScore = 0.5 * rules.HungerWeight // Base hunger, would use actual idle time

	// Rule 5: Affinity - skill and guild match, weighted by rank and reputation
	skillMatch := float64(matchingSkills)
	guildMatch := 0.0
	if quest.GuildPriority != nil {
		for _, guildID := range agent.Guilds {
			if guildID == *quest.GuildPriority {
				// Base membership match
				guildMatch = 1.0

				// Boost by rank: higher-ranked members have stronger affinity
				// GuildBonusRate ranges 0.10 (initiate) to 0.25 (guildmaster)
				if guild, ok := e.guilds[guildID]; ok {
					for _, m := range guild.Members {
						if m.AgentID == agent.ID {
							guildMatch += m.Rank.GuildBonusRate() * 5.0 // 0.5–1.25 rank bonus
							break
						}
					}
					// Boost by reputation: reputable guilds provide stronger pull
					// Reputation ranges 0.0–1.0
					guildMatch *= 1.0 + guild.Reputation*0.5 // up to 1.5x multiplier
				}
				break
			}
		}
	}
	attr.AffinityScore = (skillMatch + guildMatch) * rules.AffinityWeight

	// Peer reputation modifier: agents rated highly by peers get a stronger affinity pull.
	// PeerReviewAvg is on a 1–5 scale; we normalize it to a -1.0..+1.0 range around the
	// neutral midpoint (3.0) and apply a ±30% multiplier so reputation meaningfully
	// differentiates agents without drowning out skill/guild match.
	if agent.Stats.PeerReviewCount > 0 {
		reputationMod := (agent.Stats.PeerReviewAvg - 3.0) / 2.0 // -1.0 to +1.0
		attr.AffinityScore *= (1.0 + reputationMod*0.3)          // ±30% affinity
	}

	// Rule 6: Caution - avoid over-leveled quests
	tierDiff := int(quest.MinTier) - int(agent.Tier)
	if tierDiff > 0 {
		attr.CautionScore = -float64(tierDiff) * rules.CautionWeight
	} else {
		attr.CautionScore = 0.2 * rules.CautionWeight // Small bonus for being at/above level
	}

	// Calculate total
	attr.TotalScore = attr.SeparationScore + attr.AlignmentScore + attr.CohesionScore +
		attr.HungerScore + attr.AffinityScore + attr.CautionScore

	return attr
}

// computeQuestCrowding counts how many agents are attracted to each quest.
func (e *DefaultBoidEngine) computeQuestCrowding(agents []agentprogression.Agent, quests []domain.Quest) map[domain.QuestID]int {
	crowding := make(map[domain.QuestID]int)
	for _, agent := range agents {
		if agent.CurrentQuest != nil {
			// Count agents currently on quests (for avoiding completion conflicts)
			crowding[*agent.CurrentQuest]++
		}
	}
	// Initialize all quests to 0 if not already counted
	for _, q := range quests {
		if _, ok := crowding[q.ID]; !ok {
			crowding[q.ID] = 0
		}
	}
	return crowding
}

// computeSkillClusters counts quest density per skill.
func (e *DefaultBoidEngine) computeSkillClusters(quests []domain.Quest) map[domain.SkillTag]int {
	clusters := make(map[domain.SkillTag]int)
	for _, quest := range quests {
		for _, skill := range quest.RequiredSkills {
			clusters[skill]++
		}
	}
	return clusters
}

// SuggestClaims returns the best quest for each agent using greedy assignment.
func (e *DefaultBoidEngine) SuggestClaims(attractions []QuestAttraction) []SuggestedClaim {
	if len(attractions) == 0 {
		return nil
	}

	// Greedy assignment: take highest score, remove agent and quest from pool
	assignedAgents := make(map[domain.AgentID]bool)
	assignedQuests := make(map[domain.QuestID]bool)
	var suggestions []SuggestedClaim

	for _, attr := range attractions {
		if assignedAgents[attr.AgentID] || assignedQuests[attr.QuestID] {
			continue
		}

		assignedAgents[attr.AgentID] = true
		assignedQuests[attr.QuestID] = true

		suggestion := SuggestedClaim{
			AgentID:    attr.AgentID,
			QuestID:    attr.QuestID,
			Score:      attr.TotalScore,
			Confidence: 0.8, // Would calculate based on score gap
			Reason:     "Best match by boid rules",
		}

		// Calculate confidence based on score margin
		for _, other := range attractions {
			if other.AgentID == attr.AgentID && other.QuestID != attr.QuestID {
				if other.TotalScore > 0 {
					margin := (attr.TotalScore - other.TotalScore) / attr.TotalScore
					suggestion.Confidence = margin
				}
				break
			}
		}

		suggestions = append(suggestions, suggestion)
	}

	return suggestions
}

// SuggestTopN returns up to n ranked quest suggestions per agent.
// Quests are not removed from the pool — multiple agents may target the same quest.
// KV write serialization handles conflicts naturally at claim time.
func (e *DefaultBoidEngine) SuggestTopN(attractions []QuestAttraction, n int) map[domain.AgentID][]SuggestedClaim {
	if len(attractions) == 0 || n <= 0 {
		return nil
	}

	// Group attractions by agent
	byAgent := make(map[domain.AgentID][]QuestAttraction)
	for _, attr := range attractions {
		byAgent[attr.AgentID] = append(byAgent[attr.AgentID], attr)
	}

	result := make(map[domain.AgentID][]SuggestedClaim, len(byAgent))
	for agentID, agentAttrs := range byAgent {
		// Sort by score descending (attractions may already be sorted globally,
		// but we need per-agent ordering)
		sort.Slice(agentAttrs, func(i, j int) bool {
			return agentAttrs[i].TotalScore > agentAttrs[j].TotalScore
		})

		// Take top N
		limit := min(n, len(agentAttrs))

		suggestions := make([]SuggestedClaim, 0, limit)
		for i := range limit {
			attr := agentAttrs[i]

			// Confidence: margin between this rank and next-best alternative
			confidence := 1.0
			if i == 0 && len(agentAttrs) > 1 && attr.TotalScore > 0 {
				confidence = (attr.TotalScore - agentAttrs[1].TotalScore) / attr.TotalScore
			} else if i > 0 && agentAttrs[0].TotalScore > 0 {
				// Lower-ranked suggestions have lower confidence
				confidence = attr.TotalScore / agentAttrs[0].TotalScore * 0.5
			}

			suggestions = append(suggestions, SuggestedClaim{
				AgentID:    agentID,
				QuestID:    attr.QuestID,
				Score:      attr.TotalScore,
				Confidence: confidence,
				Reason:     "Ranked boid suggestion",
			})
		}

		result[agentID] = suggestions
	}

	return result
}

// Ensure DefaultBoidEngine implements BoidEngine.
var _ BoidEngine = (*DefaultBoidEngine)(nil)
