package semdragons

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// =============================================================================
// GUILD FORMATION - Auto-formation based on skill clustering
// =============================================================================
// Detects skill clusters among agents and auto-recruits them into guilds.
// Uses Jaccard similarity for clustering and reactive event-driven auto-recruit.
// =============================================================================

// GuildFormationEngine detects skill clusters and manages auto-recruitment.
type GuildFormationEngine interface {
	// DetectSkillClusters analyzes agents and suggests guild formations.
	DetectSkillClusters(ctx context.Context, agents []*Agent) []GuildSuggestion

	// EvaluateAgentForGuilds checks if an agent qualifies for any existing guilds.
	EvaluateAgentForGuilds(ctx context.Context, agent *Agent) ([]GuildMatch, error)

	// ProcessAutoRecruit handles auto-joining for an agent after level-up.
	ProcessAutoRecruit(ctx context.Context, agentID AgentID) ([]GuildID, error)
}

// GuildFormationConfig holds tunable parameters for guild formation.
type GuildFormationConfig struct {
	MinClusterSize      int     // Minimum agents for a cluster (default: 3)
	MinClusterStrength  float64 // Minimum Jaccard similarity (default: 0.6)
	MinAgentLevel       int     // Minimum agent level for guild consideration (default: 3)
	RequireQualityScore float64 // Minimum avg quality score (default: 0.5)
}

// DefaultFormationConfig returns sensible defaults for guild formation.
func DefaultFormationConfig() GuildFormationConfig {
	return GuildFormationConfig{
		MinClusterSize:      3,
		MinClusterStrength:  0.6,
		MinAgentLevel:       3,
		RequireQualityScore: 0.5,
	}
}

// DefaultGuildFormationEngine implements GuildFormationEngine using Jaccard clustering.
type DefaultGuildFormationEngine struct {
	storage *Storage
	events  *EventPublisher
	config  GuildFormationConfig
}

// NewGuildFormationEngine creates a new formation engine with the given config.
func NewGuildFormationEngine(storage *Storage, events *EventPublisher, config GuildFormationConfig) *DefaultGuildFormationEngine {
	return &DefaultGuildFormationEngine{
		storage: storage,
		events:  events,
		config:  config,
	}
}

// DetectSkillClusters groups agents by primary skill and calculates cluster strength.
func (e *DefaultGuildFormationEngine) DetectSkillClusters(ctx context.Context, agents []*Agent) []GuildSuggestion {
	// Filter eligible agents
	eligible := e.filterEligibleAgents(agents)
	if len(eligible) < e.config.MinClusterSize {
		return nil
	}

	// Group by primary skill (most quests completed in that skill)
	skillGroups := e.groupByPrimarySkill(eligible)

	var suggestions []GuildSuggestion
	for skill, group := range skillGroups {
		if len(group) < e.config.MinClusterSize {
			continue
		}

		// Calculate cluster strength via average pairwise Jaccard similarity
		strength := e.calculateClusterStrength(group)
		if strength < e.config.MinClusterStrength {
			continue
		}

		// Determine secondary skills shared by the cluster
		secondarySkills := e.findSecondarySkills(group, skill)

		// Find minimum level in the cluster
		minLevel := e.findMinLevel(group)

		// Generate suggestion
		agentIDs := make([]AgentID, len(group))
		for i, agent := range group {
			agentIDs[i] = agent.ID
		}

		suggestions = append(suggestions, GuildSuggestion{
			PrimarySkill:    skill,
			SecondarySkills: secondarySkills,
			AgentIDs:        agentIDs,
			ClusterStrength: strength,
			MinLevel:        minLevel,
			SuggestedName:   e.generateGuildName(skill),
		})
	}

	// Sort by cluster strength descending
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].ClusterStrength > suggestions[j].ClusterStrength
	})

	return suggestions
}

// EvaluateAgentForGuilds finds guilds the agent qualifies for.
func (e *DefaultGuildFormationEngine) EvaluateAgentForGuilds(ctx context.Context, agent *Agent) ([]GuildMatch, error) {
	if agent.Level < e.config.MinAgentLevel {
		return nil, nil
	}

	if agent.Stats.AvgQualityScore < e.config.RequireQualityScore {
		return nil, nil
	}

	var matches []GuildMatch

	// For each agent skill, find guilds that specialize in it
	for _, skill := range agent.Skills {
		guildInstances, err := e.storage.ListGuildsBySkill(ctx, skill)
		if err != nil {
			continue
		}

		for _, instance := range guildInstances {
			guild, err := e.storage.GetGuild(ctx, instance)
			if err != nil {
				continue
			}

			// Skip if already a member
			if e.isGuildMember(agent.ID, guild) {
				continue
			}

			// Check level requirement
			if agent.Level < guild.MinLevelToJoin {
				continue
			}

			// Calculate match score
			matchedSkills := e.findMatchedSkills(agent.Skills, guild.Skills)
			if len(matchedSkills) == 0 {
				continue
			}

			matchScore := float64(len(matchedSkills)) / float64(len(guild.Skills))

			matches = append(matches, GuildMatch{
				GuildID:       guild.ID,
				AgentID:       agent.ID,
				MatchScore:    matchScore,
				SkillsMatched: matchedSkills,
				CanAutoJoin:   guild.AutoRecruit,
			})
		}
	}

	// Sort by match score descending
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].MatchScore > matches[j].MatchScore
	})

	return matches, nil
}

// ProcessAutoRecruit handles auto-joining for an agent.
func (e *DefaultGuildFormationEngine) ProcessAutoRecruit(ctx context.Context, agentID AgentID) ([]GuildID, error) {
	agent, err := e.storage.GetAgent(ctx, string(agentID))
	if err != nil {
		return nil, fmt.Errorf("load agent: %w", err)
	}

	matches, err := e.EvaluateAgentForGuilds(ctx, agent)
	if err != nil {
		return nil, fmt.Errorf("evaluate guilds: %w", err)
	}

	var joined []GuildID
	for _, match := range matches {
		if !match.CanAutoJoin {
			continue
		}

		// Add agent to guild
		guildInstance := string(match.GuildID)
		err := e.storage.UpdateGuild(ctx, guildInstance, func(g *Guild) error {
			g.Members = append(g.Members, GuildMember{
				AgentID:  agentID,
				Rank:     GuildRankInitiate,
				GuildXP:  0,
				JoinedAt: time.Now(),
			})
			return nil
		})
		if err != nil {
			continue
		}

		// Add guild to agent
		err = e.storage.UpdateAgent(ctx, string(agentID), func(a *Agent) error {
			a.Guilds = append(a.Guilds, match.GuildID)
			return nil
		})
		if err != nil {
			continue
		}

		// Emit auto-join event
		if e.events != nil {
			_ = e.events.PublishGuildAutoJoined(ctx, GuildAutoJoinedPayload{
				AgentID:  agentID,
				GuildID:  match.GuildID,
				Rank:     GuildRankInitiate,
				JoinedAt: time.Now(),
				Reason:   fmt.Sprintf("auto-recruit: matched skills %v", match.SkillsMatched),
			})
		}

		joined = append(joined, match.GuildID)
	}

	return joined, nil
}

// filterEligibleAgents returns agents meeting minimum requirements.
func (e *DefaultGuildFormationEngine) filterEligibleAgents(agents []*Agent) []*Agent {
	var eligible []*Agent
	for _, agent := range agents {
		if agent.Level < e.config.MinAgentLevel {
			continue
		}
		if agent.Stats.AvgQualityScore < e.config.RequireQualityScore {
			continue
		}
		if len(agent.Skills) == 0 {
			continue
		}
		if agent.Status == AgentRetired {
			continue
		}
		eligible = append(eligible, agent)
	}
	return eligible
}

// groupByPrimarySkill groups agents by their first skill (assumed primary).
func (e *DefaultGuildFormationEngine) groupByPrimarySkill(agents []*Agent) map[SkillTag][]*Agent {
	groups := make(map[SkillTag][]*Agent)
	for _, agent := range agents {
		if len(agent.Skills) == 0 {
			continue
		}
		primary := agent.Skills[0]
		groups[primary] = append(groups[primary], agent)
	}
	return groups
}

// calculateClusterStrength computes average pairwise Jaccard similarity.
func (e *DefaultGuildFormationEngine) calculateClusterStrength(agents []*Agent) float64 {
	if len(agents) < 2 {
		return 0
	}

	var total float64
	var pairs int

	for i, agentA := range agents {
		for _, agentB := range agents[i+1:] {
			total += JaccardSimilarity(agentA.Skills, agentB.Skills)
			pairs++
		}
	}

	if pairs == 0 {
		return 0
	}
	return total / float64(pairs)
}

// JaccardSimilarity computes the Jaccard index between two skill sets.
// Returns intersection / union, ranging from 0 to 1.
func JaccardSimilarity(a, b []SkillTag) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0 // Empty sets are identical
	}

	setA := make(map[SkillTag]struct{}, len(a))
	for _, skill := range a {
		setA[skill] = struct{}{}
	}

	setB := make(map[SkillTag]struct{}, len(b))
	for _, skill := range b {
		setB[skill] = struct{}{}
	}

	// Calculate intersection
	var intersection int
	for skill := range setA {
		if _, ok := setB[skill]; ok {
			intersection++
		}
	}

	// Calculate union
	union := make(map[SkillTag]struct{})
	for skill := range setA {
		union[skill] = struct{}{}
	}
	for skill := range setB {
		union[skill] = struct{}{}
	}

	if len(union) == 0 {
		return 0
	}

	return float64(intersection) / float64(len(union))
}

// findSecondarySkills identifies skills shared by most cluster members beyond primary.
func (e *DefaultGuildFormationEngine) findSecondarySkills(agents []*Agent, primary SkillTag) []SkillTag {
	skillCounts := make(map[SkillTag]int)
	threshold := len(agents) / 2 // Skill must appear in at least half

	for _, agent := range agents {
		for _, skill := range agent.Skills {
			if skill != primary {
				skillCounts[skill]++
			}
		}
	}

	var secondary []SkillTag
	for skill, count := range skillCounts {
		if count >= threshold {
			secondary = append(secondary, skill)
		}
	}

	return secondary
}

// findMinLevel returns the minimum level among agents.
func (e *DefaultGuildFormationEngine) findMinLevel(agents []*Agent) int {
	if len(agents) == 0 {
		return 0
	}
	min := agents[0].Level
	for _, agent := range agents[1:] {
		if agent.Level < min {
			min = agent.Level
		}
	}
	return min
}

// generateGuildName creates a name from the primary skill.
func (e *DefaultGuildFormationEngine) generateGuildName(skill SkillTag) string {
	// Convert skill_tag to "Skill Tag Guild"
	name := string(skill)
	name = strings.ReplaceAll(name, "_", " ")
	caser := cases.Title(language.English)
	name = caser.String(name)
	return name + " Guild"
}

// isGuildMember checks if an agent is already a member of a guild.
func (e *DefaultGuildFormationEngine) isGuildMember(agentID AgentID, guild *Guild) bool {
	for _, member := range guild.Members {
		if member.AgentID == agentID {
			return true
		}
	}
	return false
}

// findMatchedSkills returns skills present in both sets.
func (e *DefaultGuildFormationEngine) findMatchedSkills(agentSkills, guildSkills []SkillTag) []SkillTag {
	guildSet := make(map[SkillTag]struct{}, len(guildSkills))
	for _, skill := range guildSkills {
		guildSet[skill] = struct{}{}
	}

	var matched []SkillTag
	for _, skill := range agentSkills {
		if _, ok := guildSet[skill]; ok {
			matched = append(matched, skill)
		}
	}
	return matched
}
