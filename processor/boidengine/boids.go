// Package boidengine implements boid flocking rules for emergent agent behavior.
// Agents flock toward quests they're best suited for using six rules:
// Separation, Alignment, Cohesion, Hunger, Affinity, and Caution.
//
// Guild suggestions use peer review and shared-win data to organically pull
// agents toward guilds with people they've worked well with.
package boidengine

import (
	"fmt"
	"sort"
	"strings"

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

// GuildSuggestion is a recommendation for an agent to join or form a guild.
// Type "join" means the agent should join an existing guild (GuildID set).
// Type "form" means the agent should found a new guild (GuildID empty).
type GuildSuggestion struct {
	AgentID    domain.AgentID `json:"agent_id"`
	GuildID    domain.GuildID `json:"guild_id,omitempty"`
	Type       string         `json:"type"`       // "join" or "form"
	Score      float64        `json:"score"`
	Confidence float64        `json:"confidence"`
	Reason     string         `json:"reason"`
}

// CohesionData provides pairwise agent relationship data for guild scoring.
// Satisfied by *guildformation.Component without importing it (avoids import cycle).
type CohesionData interface {
	SharedWins(a, b domain.AgentID) int
	PairwisePeerScore(a, b domain.AgentID) (float64, bool)
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
	rules    BoidRules
	guilds   map[domain.GuildID]*domain.Guild // Guild context for rank/reputation lookups
	cohesion CohesionData                     // Pairwise peer review / shared wins (may be nil)
}

// SetGuildContext provides guild data for rank and reputation calculations.
// Called by the component before each computation cycle.
func (e *DefaultBoidEngine) SetGuildContext(guilds map[domain.GuildID]*domain.Guild) {
	e.guilds = guilds
}

// SetCohesionData provides pairwise agent relationship data for guild scoring.
// Called by the component before each computation cycle.
func (e *DefaultBoidEngine) SetCohesionData(cd CohesionData) {
	e.cohesion = cd
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
	if quest.GuildPriority != nil && agent.Guild == *quest.GuildPriority {
		// Base membership match
		guildMatch = 1.0

		// Boost by rank: higher-ranked members have stronger affinity
		// GuildBonusRate ranges 0.10 (initiate) to 0.25 (guildmaster)
		if guild, ok := e.guilds[agent.Guild]; ok {
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
	}
	// Cross-guild bonus for red-team review quests: agents from a DIFFERENT guild
	// than the implementing team get a 1.5x multiplier on skill match (since guild
	// match is zero for cross-guild). Agents from the SAME guild as the blue team
	// get a penalty — you can't red-team your own team's work.
	crossGuildBonus := 0.0
	if quest.QuestType == domain.QuestTypeRedTeam {
		if quest.GuildPriority != nil && agent.Guild != "" {
			if agent.Guild != *quest.GuildPriority {
				// Different guild — this is exactly what we want.
				crossGuildBonus = skillMatch * 1.5
			}
			// Same guild as blue team: guildMatch is already 0 (no priority match).
		} else if agent.Guild != "" {
			// No guild priority set — any guild member gets a moderate bonus.
			crossGuildBonus = skillMatch * 0.5
		}
	}

	attr.AffinityScore = (skillMatch + guildMatch + crossGuildBonus) * rules.AffinityWeight

	// Peer reputation modifier: agents rated highly by peers get a stronger affinity pull.
	// PeerReviewAvg is on a 1–5 scale; we normalize it to a -1.0..+1.0 range around the
	// neutral midpoint (3.0) and apply a ±30% multiplier so reputation meaningfully
	// differentiates agents without drowning out skill/guild match.
	if agent.Stats.PeerReviewCount > 0 {
		reputationMod := (agent.Stats.PeerReviewAvg - 3.0) / 2.0 // -1.0 to +1.0
		attr.AffinityScore *= (1.0 + reputationMod*0.3)          // ±30% affinity
	}

	// Rule 6: Caution - avoid mismatched quests (both under- and over-leveled)
	tierDiff := int(quest.MinTier) - int(agent.Tier)
	if tierDiff > 0 {
		// Quest is above agent's tier — strong penalty
		attr.CautionScore = -float64(tierDiff) * rules.CautionWeight
	} else if tierDiff < -1 {
		// Agent is significantly overqualified — increasing penalty so grandmasters
		// don't hog trivial quests. One tier above is fine (small bonus), but 2+
		// tiers above gets progressively less attractive.
		overqualified := float64(-tierDiff - 1) // 0 at 1 tier above, 1 at 2 tiers, etc.
		attr.CautionScore = -overqualified * 0.5 * rules.CautionWeight
	} else {
		// Agent is exactly one tier above or at level — slight bonus
		attr.CautionScore = 0.2 * rules.CautionWeight
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

// =============================================================================
// GUILD SUGGESTIONS - Organic guild formation via peer cohesion
// =============================================================================
// Agents who've worked together and scored each other well feel a natural pull
// to coalesce into guilds. Peer review + shared wins are 50% of the join score.
// =============================================================================

// Guild suggestion scoring weights for joining an existing guild.
const (
	guildJoinWeightPeerPull   = 0.25
	guildJoinWeightSharedWins = 0.25
	guildJoinWeightSkillGap   = 0.25
	guildJoinWeightCapacity   = 0.15
	guildJoinWeightReputation = 0.10
)

// Guild formation scoring weights.
const (
	guildFormWeightCohesion       = 0.50
	guildFormWeightSkillDiversity = 0.30
	guildFormWeightCandidateCount = 0.20
)

// ComputeGuildSuggestions computes guild join/form suggestions for unguilded agents.
// Returns at most one suggestion per agent. Agents with existing guild membership
// are skipped. Formation suggestions require Expert+ tier and sufficient unguilded
// candidates with demonstrated peer cohesion.
func (e *DefaultBoidEngine) ComputeGuildSuggestions(
	agents []agentprogression.Agent,
	guilds map[domain.GuildID]*domain.Guild,
	minForFormation int,
	joinThreshold float64,
) []GuildSuggestion {
	if len(agents) == 0 {
		return nil
	}

	// Collect unguilded idle agents
	var unguilded []agentprogression.Agent
	for i := range agents {
		if agents[i].Guild == "" && agents[i].Status == domain.AgentIdle {
			unguilded = append(unguilded, agents[i])
		}
	}
	if len(unguilded) == 0 {
		return nil
	}

	// Build guild member skill sets for skill gap scoring
	guildSkills := e.buildGuildSkillSets(guilds)

	var suggestions []GuildSuggestion

	for i := range unguilded {
		agent := &unguilded[i]

		// Try join first — any tier can join
		if joinSugg, ok := e.scoreGuildJoin(agent, guilds, guildSkills, joinThreshold); ok {
			// Expert+ agents still prefer joining if score is strong (> 0.5)
			if agent.Tier >= domain.TierExpert && joinSugg.Score <= 0.5 {
				// Weak join — check if formation is better
				if formSugg, fok := e.scoreGuildFormation(agent, unguilded, minForFormation); fok {
					suggestions = append(suggestions, formSugg)
					continue
				}
			}
			suggestions = append(suggestions, joinSugg)
			continue
		}

		// No good join match — Expert+ can try formation
		if agent.Tier >= domain.TierExpert {
			if formSugg, ok := e.scoreGuildFormation(agent, unguilded, minForFormation); ok {
				suggestions = append(suggestions, formSugg)
			}
		}
	}

	return suggestions
}

// scoreGuildJoin scores how well an agent fits each active guild.
// Returns the best match above joinThreshold, or (zero, false) if none qualify.
func (e *DefaultBoidEngine) scoreGuildJoin(
	agent *agentprogression.Agent,
	guilds map[domain.GuildID]*domain.Guild,
	guildSkills map[domain.GuildID]map[domain.SkillTag]bool,
	joinThreshold float64,
) (GuildSuggestion, bool) {
	var best GuildSuggestion
	var bestScore float64

	for _, guild := range guilds {
		if guild.Status != domain.GuildActive {
			continue
		}
		if guild.MaxMembers > 0 && len(guild.Members) >= guild.MaxMembers {
			continue
		}
		if agent.Level < guild.MinLevel {
			continue
		}

		// Peer pull: max pairwise peer score with any guild member
		var peerPull float64
		var sharedWinScore float64
		if e.cohesion != nil {
			for _, m := range guild.Members {
				if ps, ok := e.cohesion.PairwisePeerScore(agent.ID, m.AgentID); ok && ps > peerPull {
					peerPull = ps
				}
				wins := e.cohesion.SharedWins(agent.ID, m.AgentID)
				if ws := sharedWinScoreFn(wins); ws > sharedWinScore {
					sharedWinScore = ws
				}
			}
		}

		// Skill gap: fraction of agent skills NOT already covered by guild
		skillGap := e.computeSkillGap(agent, guildSkills[guild.ID])

		// Capacity
		var capacity float64
		if guild.MaxMembers > 0 {
			capacity = float64(guild.MaxMembers-len(guild.Members)) / float64(guild.MaxMembers)
		} else {
			capacity = 1.0
		}

		// Reputation
		reputation := guild.Reputation

		// Cohesion component: agents who've worked well with guild members
		cohesionScore := peerPull*guildJoinWeightPeerPull +
			sharedWinScore*guildJoinWeightSharedWins
		// Non-cohesion component: structural fit (skill gap, capacity, reputation)
		structuralScore := skillGap*guildJoinWeightSkillGap +
			capacity*guildJoinWeightCapacity +
			reputation*guildJoinWeightReputation

		// When there's no cohesion signal at all, dampen structural factors so that
		// skill gap + capacity + reputation alone can't exceed the join threshold.
		// This ensures agents gravitate toward guilds with people they've actually
		// worked with, not just any guild with an open slot.
		if peerPull == 0 && sharedWinScore == 0 {
			structuralScore *= 0.4
		}
		score := cohesionScore + structuralScore

		if score > bestScore {
			bestScore = score
			// Build reason from dominant factors
			var parts []string
			if peerPull >= 0.5 {
				parts = append(parts, "strong peer reviews")
			}
			if sharedWinScore >= 0.5 {
				parts = append(parts, "shared quest wins")
			}
			if skillGap >= 0.5 {
				parts = append(parts, "fills skill gaps")
			}
			if reputation >= 0.7 {
				parts = append(parts, "high reputation")
			}
			reason := "general fit"
			if len(parts) > 0 {
				reason = strings.Join(parts, ", ")
			}

			best = GuildSuggestion{
				AgentID:    agent.ID,
				GuildID:    guild.ID,
				Type:       "join",
				Score:      score,
				Confidence: score, // Simple confidence = score
				Reason:     reason,
			}
		}
	}

	if bestScore < joinThreshold {
		return GuildSuggestion{}, false
	}
	return best, true
}

// scoreGuildFormation evaluates whether an Expert+ agent should found a new guild.
// Formation requires enough unguilded candidates with demonstrated peer cohesion.
func (e *DefaultBoidEngine) scoreGuildFormation(
	agent *agentprogression.Agent,
	unguilded []agentprogression.Agent,
	minForFormation int,
) (GuildSuggestion, bool) {
	if len(unguilded) < minForFormation {
		return GuildSuggestion{}, false
	}

	// Score pairwise cohesion between the founder and each other unguilded agent
	type candidateScore struct {
		id       domain.AgentID
		cohesion float64
	}
	var candidates []candidateScore

	for i := range unguilded {
		other := &unguilded[i]
		if other.ID == agent.ID {
			continue
		}

		var peerScore float64
		var winScore float64
		if e.cohesion != nil {
			if ps, ok := e.cohesion.PairwisePeerScore(agent.ID, other.ID); ok {
				peerScore = ps
			}
			wins := e.cohesion.SharedWins(agent.ID, other.ID)
			winScore = sharedWinScoreFn(wins)
		}

		cohesion := 0.5*winScore + 0.5*peerScore
		if cohesion > 0 {
			candidates = append(candidates, candidateScore{id: other.ID, cohesion: cohesion})
		}
	}

	// Need at least minForFormation-1 candidates with some cohesion
	if len(candidates)+1 < minForFormation {
		return GuildSuggestion{}, false
	}

	// Sort by cohesion descending, take top N-1
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].cohesion > candidates[j].cohesion
	})
	topN := min(minForFormation-1, len(candidates))

	// Average cohesion of top candidates
	var avgCohesion float64
	allSkills := make(map[domain.SkillTag]bool)
	totalSkillSlots := 0
	for skill := range agent.SkillProficiencies {
		allSkills[skill] = true
		totalSkillSlots++
	}
	for i := range topN {
		avgCohesion += candidates[i].cohesion
		// Find the candidate agent to get their skills
		for j := range unguilded {
			if unguilded[j].ID == candidates[i].id {
				for skill := range unguilded[j].SkillProficiencies {
					allSkills[skill] = true
					totalSkillSlots++
				}
				break
			}
		}
	}
	avgCohesion /= float64(topN)

	// Skill diversity: unique skills / total skill slots
	var skillDiversity float64
	if totalSkillSlots > 0 {
		skillDiversity = float64(len(allSkills)) / float64(totalSkillSlots)
	}

	// Candidate count bonus: more candidates beyond minimum = stronger signal
	candidateBonus := float64(len(candidates)+1-minForFormation) / float64(minForFormation)
	if candidateBonus > 1.0 {
		candidateBonus = 1.0
	}

	score := avgCohesion*guildFormWeightCohesion +
		skillDiversity*guildFormWeightSkillDiversity +
		candidateBonus*guildFormWeightCandidateCount

	if score <= 0.3 {
		return GuildSuggestion{}, false
	}

	var parts []string
	if avgCohesion >= 0.5 {
		parts = append(parts, fmt.Sprintf("strong cohesion (%.2f)", avgCohesion))
	}
	if skillDiversity >= 0.5 {
		parts = append(parts, "diverse skills")
	}
	reason := "formation viable"
	if len(parts) > 0 {
		reason = strings.Join(parts, ", ")
	}

	return GuildSuggestion{
		AgentID:    agent.ID,
		Type:       "form",
		Score:      score,
		Confidence: avgCohesion, // Confidence driven by cohesion strength
		Reason:     reason,
	}, true
}

// buildGuildSkillSets builds a map of guild ID → set of skills covered by guild members.
func (e *DefaultBoidEngine) buildGuildSkillSets(guilds map[domain.GuildID]*domain.Guild) map[domain.GuildID]map[domain.SkillTag]bool {
	result := make(map[domain.GuildID]map[domain.SkillTag]bool, len(guilds))
	for id, guild := range guilds {
		skills := make(map[domain.SkillTag]bool)
		// Use QuestTypes as a proxy for guild skill coverage
		for _, qt := range guild.QuestTypes {
			skills[domain.SkillTag(qt)] = true
		}
		result[id] = skills
	}
	return result
}

// computeSkillGap measures what fraction of the agent's skills are NOT already
// covered by the guild. Returns 0.0–1.0: higher = agent fills more gaps.
func (e *DefaultBoidEngine) computeSkillGap(agent *agentprogression.Agent, guildSkills map[domain.SkillTag]bool) float64 {
	if len(agent.SkillProficiencies) == 0 {
		return 0.0
	}
	if len(guildSkills) == 0 {
		return 0.5 // No data — neutral
	}
	newSkills := 0
	for skill := range agent.SkillProficiencies {
		if !guildSkills[skill] {
			newSkills++
		}
	}
	return float64(newSkills) / float64(len(agent.SkillProficiencies))
}

// sharedWinScoreFn converts raw win count to 0–1 score using a stepped curve.
// Mirrors guildformation.SharedWinScore but avoids the import cycle.
//
//	0 wins → 0.0, 1 win → 0.6, 2–3 → 0.8, 4+ → 1.0
func sharedWinScoreFn(wins int) float64 {
	switch {
	case wins <= 0:
		return 0.0
	case wins == 1:
		return 0.6
	case wins <= 3:
		return 0.8
	default:
		return 1.0
	}
}

// Ensure DefaultBoidEngine implements BoidEngine.
var _ BoidEngine = (*DefaultBoidEngine)(nil)
