package semdragons

import (
	"math"
	"sort"
	"time"
)

// =============================================================================
// BOIDS LAYER - Emergent behavior rules for agent flocking
// =============================================================================
// The Boids rules operate at the micro level, influencing how agents naturally
// gravitate toward work without explicit assignment. The quest board provides
// macro structure; Boids provides micro behavior.
//
// Classic Boids rules adapted for agents:
//   1. Separation - Avoid duplicate work / stepping on each other
//   2. Alignment  - Align with nearby agents working on related quests
//   3. Cohesion   - Gravitate toward quest clusters in your skill domain
//
// Additional agent-specific rules:
//   4. Hunger     - Idle agents are increasingly attracted to available quests
//   5. Affinity   - Agents prefer quests matching their skills/guild
//   6. Caution    - Agents avoid quests above their level (unless in a party)
// =============================================================================

// BoidRules defines the flocking parameters for agent behavior.
// These weights control how strongly each rule influences quest selection.
type BoidRules struct {
	// Classic Boids (adapted)
	SeparationWeight float64 `json:"separation_weight"` // Avoid quest overlap (default: 1.0)
	AlignmentWeight  float64 `json:"alignment_weight"`  // Align with peers on related work (default: 0.8)
	CohesionWeight   float64 `json:"cohesion_weight"`   // Move toward skill-matched quest clusters (default: 0.6)

	// Agent-specific rules
	HungerWeight     float64 `json:"hunger_weight"`     // Idle time increases urgency (default: 1.2)
	AffinityWeight   float64 `json:"affinity_weight"`   // Skill/guild match preference (default: 1.5)
	CautionWeight    float64 `json:"caution_weight"`    // Avoid over-leveled quests (default: 0.9)

	// Tuning
	NeighborRadius   int     `json:"neighbor_radius"`   // How many "nearby" agents to consider
	UpdateInterval   int     `json:"update_interval_ms"` // How often to recalculate (ms)
}

// DefaultBoidRules returns the default boid rule weights.
func DefaultBoidRules() BoidRules {
	return BoidRules{
		SeparationWeight: 1.0,
		AlignmentWeight:  0.8,
		CohesionWeight:   0.6,
		HungerWeight:     1.2,
		AffinityWeight:   1.5,
		CautionWeight:    0.9,
		NeighborRadius:   5,
		UpdateInterval:   1000,
	}
}

// QuestAttraction represents how attracted an agent is to a specific quest.
// Higher score = agent more likely to claim this quest.
type QuestAttraction struct {
	QuestID    QuestID `json:"quest_id"`
	AgentID    AgentID `json:"agent_id"`
	TotalScore float64 `json:"total_score"`

	// Score breakdown
	Separation float64 `json:"separation"` // Negative if others are already on it
	Alignment  float64 `json:"alignment"`  // Positive if peers are on related quests
	Cohesion   float64 `json:"cohesion"`   // Positive if quest is in agent's domain cluster
	Hunger     float64 `json:"hunger"`     // Higher the longer agent has been idle
	Affinity   float64 `json:"affinity"`   // Skill and guild match
	Caution    float64 `json:"caution"`    // Negative if quest is above agent's level
}

// BoidEngine computes quest attractions for agents, enabling emergent work distribution.
// Instead of assigning quests, we compute attraction scores and let agents claim
// the quests they're most attracted to.
type BoidEngine interface {
	// ComputeAttractions calculates how attracted each idle agent is to each open quest.
	// Returns a ranked list of (agent, quest) pairs sorted by attraction score.
	ComputeAttractions(agents []Agent, quests []Quest, rules BoidRules) []QuestAttraction

	// SuggestClaims returns the optimal set of claims that maximizes total attraction
	// while respecting constraints (one claim per agent, quest capacity, etc.).
	SuggestClaims(attractions []QuestAttraction) []SuggestedClaim

	// UpdateRules allows the DM to tune Boid parameters at runtime.
	UpdateRules(rules BoidRules)
}

// SuggestedClaim represents a recommended agent-quest assignment.
type SuggestedClaim struct {
	QuestID    QuestID `json:"quest_id"`
	AgentID    AgentID `json:"agent_id"`
	Score      float64 `json:"score"`
	Confidence float64 `json:"confidence"` // How clearly this is the best match (0-1)
}

// =============================================================================
// AGENT IDLE INFO - Caller-provided runtime idle tracking
// =============================================================================

// AgentIdleInfo provides runtime idle tracking (caller-provided).
// The boid engine is stateless - callers must track and provide idle times.
type AgentIdleInfo struct {
	IdleSince time.Time `json:"idle_since"`
}

// AssignmentStrategy controls SuggestClaims algorithm.
type AssignmentStrategy string

const (
	// AssignmentGreedy uses O(n log n) greedy algorithm - fast, good enough.
	AssignmentGreedy AssignmentStrategy = "greedy"
	// AssignmentOptimal uses O(n³) Hungarian algorithm - optimal but slow.
	AssignmentOptimal AssignmentStrategy = "optimal"
)

// =============================================================================
// DEFAULT BOID ENGINE - A stateless computational engine for quest attraction
// =============================================================================

// DefaultBoidEngine is a stateless computational engine that calculates quest
// attractions for agents using six boid-inspired rules. Follows the same
// pattern as DefaultXPEngine.
type DefaultBoidEngine struct {
	rules BoidRules
}

// NewDefaultBoidEngine creates a new DefaultBoidEngine with default rules.
func NewDefaultBoidEngine() *DefaultBoidEngine {
	return &DefaultBoidEngine{
		rules: DefaultBoidRules(),
	}
}

// -----------------------------------------------------------------------------
// Helper Methods
// -----------------------------------------------------------------------------

// findNeighbors returns agents that share at least one skill with the given agent.
// "Neighbors" in this context means agents with overlapping capabilities,
// not spatial proximity.
func (e *DefaultBoidEngine) findNeighbors(agent Agent, allAgents []Agent) []Agent {
	if len(agent.Skills) == 0 {
		return nil
	}

	agentSkills := make(map[SkillTag]bool, len(agent.Skills))
	for _, skill := range agent.Skills {
		agentSkills[skill] = true
	}

	var neighbors []Agent
	for _, other := range allAgents {
		if other.ID == agent.ID {
			continue
		}
		for _, skill := range other.Skills {
			if agentSkills[skill] {
				neighbors = append(neighbors, other)
				break
			}
		}
	}
	return neighbors
}

// hasSkillOverlap checks if two quests share at least one required skill.
func (e *DefaultBoidEngine) hasSkillOverlap(q1, q2 Quest) bool {
	if len(q1.RequiredSkills) == 0 || len(q2.RequiredSkills) == 0 {
		return false
	}

	skills := make(map[SkillTag]bool, len(q1.RequiredSkills))
	for _, skill := range q1.RequiredSkills {
		skills[skill] = true
	}

	for _, skill := range q2.RequiredSkills {
		if skills[skill] {
			return true
		}
	}
	return false
}

// -----------------------------------------------------------------------------
// Rule Methods - Each rule returns a normalized score
// -----------------------------------------------------------------------------

// ruleSeparation returns a penalty for quests claimed by other agents.
// Returns -1 if claimed by another agent, 0 if available or claimed by self.
func (e *DefaultBoidEngine) ruleSeparation(agent Agent, quest Quest) float64 {
	if quest.ClaimedBy == nil {
		return 0 // Available
	}
	if *quest.ClaimedBy == agent.ID {
		return 0 // Claimed by self
	}
	return -1 // Claimed by another agent
}

// ruleAffinity returns skill match ratio between agent and quest.
// Returns matching_skills / required_skills (0 to 1).
func (e *DefaultBoidEngine) ruleAffinity(agent Agent, quest Quest) float64 {
	if len(quest.RequiredSkills) == 0 {
		return 1.0 // No skills required = perfect match
	}

	agentSkills := make(map[SkillTag]bool, len(agent.Skills))
	for _, skill := range agent.Skills {
		agentSkills[skill] = true
	}

	matching := 0
	for _, required := range quest.RequiredSkills {
		if agentSkills[required] {
			matching++
		}
	}

	return float64(matching) / float64(len(quest.RequiredSkills))
}

// ruleCaution returns a penalty for quests above the agent's tier.
// Returns 0 if agent meets tier requirement, -0.33 per tier gap below required.
func (e *DefaultBoidEngine) ruleCaution(agent Agent, quest Quest) float64 {
	tierGap := int(quest.MinTier) - int(agent.Tier)
	if tierGap <= 0 {
		return 0 // Agent meets or exceeds requirement
	}
	// -0.33 per tier below requirement, capped at -1.0
	penalty := float64(tierGap) * -0.33
	return math.Max(penalty, -1.0)
}

// ruleHunger returns urgency based on idle time.
// Returns min(idle_minutes / 60, 1.0).
func (e *DefaultBoidEngine) ruleHunger(_ Agent, idleInfo *AgentIdleInfo, now time.Time) float64 {
	if idleInfo == nil {
		return 0 // No idle info provided
	}

	idleDuration := now.Sub(idleInfo.IdleSince)
	if idleDuration < 0 {
		return 0 // Future idle time doesn't make sense
	}

	idleMinutes := idleDuration.Minutes()
	return math.Min(idleMinutes/60.0, 1.0)
}

// ruleCohesion returns guild affinity between agent and quest.
// Returns 1.0 if agent's guild matches quest's GuildPriority,
// 0.5 if no GuildPriority on quest, 0.2 if guild mismatch.
func (e *DefaultBoidEngine) ruleCohesion(agent Agent, quest Quest) float64 {
	if quest.GuildPriority == nil {
		return 0.5 // Neutral - no guild preference on quest
	}

	for _, guildID := range agent.Guilds {
		if guildID == *quest.GuildPriority {
			return 1.0 // Agent is in the priority guild
		}
	}

	return 0.2 // Quest has a guild preference, but agent isn't in it
}

// ruleAlignment returns attraction based on neighbors working on related quests.
// Returns neighbors_on_related_quests / neighborRadius (0 to 1).
// neighborRadius is passed explicitly to keep the engine stateless and thread-safe.
func (e *DefaultBoidEngine) ruleAlignment(agent Agent, quest Quest, allAgents []Agent, allQuests []Quest, neighborRadius int) float64 {
	if neighborRadius <= 0 {
		return 0
	}

	neighbors := e.findNeighbors(agent, allAgents)
	if len(neighbors) == 0 {
		return 0
	}

	// Build map of which agents are on which quests
	agentToQuest := make(map[AgentID]*Quest)
	for i := range allQuests {
		q := &allQuests[i]
		if q.ClaimedBy != nil && q.Status != QuestCompleted && q.Status != QuestFailed {
			agentToQuest[*q.ClaimedBy] = q
		}
	}

	// Count neighbors on related quests (quests with overlapping skills)
	neighborsOnRelated := 0
	for _, neighbor := range neighbors {
		neighborQuest, ok := agentToQuest[neighbor.ID]
		if !ok {
			continue
		}
		if e.hasSkillOverlap(quest, *neighborQuest) {
			neighborsOnRelated++
		}
	}

	// Normalize by neighborRadius
	return math.Min(float64(neighborsOnRelated)/float64(neighborRadius), 1.0)
}

// -----------------------------------------------------------------------------
// Interface Methods
// -----------------------------------------------------------------------------

// ComputeAttractions calculates attraction scores for all agent-quest pairs.
// Satisfies BoidEngine interface.
func (e *DefaultBoidEngine) ComputeAttractions(agents []Agent, quests []Quest, rules BoidRules) []QuestAttraction {
	return e.ComputeAttractionsWithContext(agents, quests, rules, nil, time.Now())
}

// ComputeAttractionsWithContext calculates attractions with explicit idle tracking.
// This method is stateless and thread-safe - all rules are passed explicitly.
func (e *DefaultBoidEngine) ComputeAttractionsWithContext(
	agents []Agent,
	quests []Quest,
	rules BoidRules,
	idleInfo map[AgentID]AgentIdleInfo,
	now time.Time,
) []QuestAttraction {
	var attractions []QuestAttraction

	// Filter to idle agents only
	idleAgents := make([]Agent, 0, len(agents))
	for _, agent := range agents {
		if agent.Status == AgentIdle {
			idleAgents = append(idleAgents, agent)
		}
	}

	// Filter to available quests (posted status, not claimed by others)
	availableQuests := make([]Quest, 0, len(quests))
	for _, quest := range quests {
		if quest.Status == QuestPosted {
			availableQuests = append(availableQuests, quest)
		}
	}

	// Compute attractions for each idle agent × available quest pair
	for _, agent := range idleAgents {
		var agentIdleInfo *AgentIdleInfo
		if idleInfo != nil {
			if info, ok := idleInfo[agent.ID]; ok {
				agentIdleInfo = &info
			}
		}

		for _, quest := range availableQuests {
			attraction := QuestAttraction{
				QuestID: quest.ID,
				AgentID: agent.ID,
			}

			// Compute each rule's raw score (pass neighborRadius explicitly for thread safety)
			attraction.Separation = e.ruleSeparation(agent, quest)
			attraction.Alignment = e.ruleAlignment(agent, quest, agents, quests, rules.NeighborRadius)
			attraction.Cohesion = e.ruleCohesion(agent, quest)
			attraction.Hunger = e.ruleHunger(agent, agentIdleInfo, now)
			attraction.Affinity = e.ruleAffinity(agent, quest)
			attraction.Caution = e.ruleCaution(agent, quest)

			// Apply weights to get total score
			attraction.TotalScore =
				attraction.Separation*rules.SeparationWeight +
					attraction.Alignment*rules.AlignmentWeight +
					attraction.Cohesion*rules.CohesionWeight +
					attraction.Hunger*rules.HungerWeight +
					attraction.Affinity*rules.AffinityWeight +
					attraction.Caution*rules.CautionWeight

			attractions = append(attractions, attraction)
		}
	}

	// Sort by TotalScore descending
	sort.Slice(attractions, func(i, j int) bool {
		return attractions[i].TotalScore > attractions[j].TotalScore
	})

	return attractions
}

// SuggestClaims returns the optimal set of claims using greedy assignment.
// Satisfies BoidEngine interface.
func (e *DefaultBoidEngine) SuggestClaims(attractions []QuestAttraction) []SuggestedClaim {
	return e.SuggestClaimsWithStrategy(attractions, AssignmentGreedy)
}

// SuggestClaimsWithStrategy returns optimal claims using the specified strategy.
func (e *DefaultBoidEngine) SuggestClaimsWithStrategy(
	attractions []QuestAttraction,
	strategy AssignmentStrategy,
) []SuggestedClaim {
	switch strategy {
	case AssignmentOptimal:
		// TODO: Implement Hungarian algorithm for optimal assignment
		// For now, fall through to greedy
		fallthrough
	default:
		return e.suggestClaimsGreedy(attractions)
	}
}

// suggestClaimsGreedy assigns highest-score pairs without duplicates.
// O(n) where n = number of attractions (already sorted).
func (e *DefaultBoidEngine) suggestClaimsGreedy(attractions []QuestAttraction) []SuggestedClaim {
	if len(attractions) == 0 {
		return nil
	}

	// Track which agents and quests are already assigned
	assignedAgents := make(map[AgentID]bool)
	assignedQuests := make(map[QuestID]bool)

	// Build index for confidence calculation
	agentAttractions := make(map[AgentID][]QuestAttraction)
	for _, a := range attractions {
		agentAttractions[a.AgentID] = append(agentAttractions[a.AgentID], a)
	}

	var claims []SuggestedClaim

	// Attractions are already sorted by TotalScore descending
	for _, attraction := range attractions {
		if assignedAgents[attraction.AgentID] || assignedQuests[attraction.QuestID] {
			continue
		}

		confidence := e.calculateConfidence(attraction, agentAttractions[attraction.AgentID], assignedQuests)

		claims = append(claims, SuggestedClaim{
			QuestID:    attraction.QuestID,
			AgentID:    attraction.AgentID,
			Score:      attraction.TotalScore,
			Confidence: confidence,
		})

		assignedAgents[attraction.AgentID] = true
		assignedQuests[attraction.QuestID] = true
	}

	return claims
}

// calculateConfidence computes how clearly this is the best match for the agent.
// Returns 1.0 if this is the only option, lower values if there are close alternatives.
func (e *DefaultBoidEngine) calculateConfidence(
	chosen QuestAttraction,
	allForAgent []QuestAttraction,
	claimedQuests map[QuestID]bool,
) float64 {
	if len(allForAgent) == 0 {
		return 1.0
	}

	// Find the next best available option for this agent
	var nextBest *QuestAttraction
	for i := range allForAgent {
		a := &allForAgent[i]
		if a.QuestID == chosen.QuestID {
			continue
		}
		if claimedQuests[a.QuestID] {
			continue
		}
		if nextBest == nil || a.TotalScore > nextBest.TotalScore {
			nextBest = a
		}
	}

	if nextBest == nil {
		return 1.0 // No alternatives
	}

	// Confidence based on score difference
	// If chosen is much better, confidence is high
	// If scores are close, confidence is low
	if chosen.TotalScore <= 0 {
		return 0.5 // Edge case: negative or zero score
	}

	diff := chosen.TotalScore - nextBest.TotalScore
	// Normalize: diff of 1.0 or more = full confidence
	confidence := math.Min(diff/1.0, 1.0)
	// Ensure minimum confidence of 0.1 if this is still the best option
	return math.Max(confidence, 0.1)
}

// UpdateRules allows runtime tuning of boid parameters.
// Satisfies BoidEngine interface.
func (e *DefaultBoidEngine) UpdateRules(rules BoidRules) {
	e.rules = rules
}
