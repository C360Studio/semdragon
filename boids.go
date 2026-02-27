package semdragons

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

type SuggestedClaim struct {
	QuestID    QuestID `json:"quest_id"`
	AgentID    AgentID `json:"agent_id"`
	Score      float64 `json:"score"`
	Confidence float64 `json:"confidence"` // How clearly this is the best match (0-1)
}
