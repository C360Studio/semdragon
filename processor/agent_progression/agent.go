package agent_progression

import (
	"fmt"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/message"
)

// =============================================================================
// AGENT - An autonomous worker that claims and executes quests
// =============================================================================
// Agent is the core entity owned by the agent_progression processor.
// It implements graph.Graphable for persistence in the semstreams graph system.
// =============================================================================

// Agent represents an autonomous worker in the semdragons system.
type Agent struct {
	ID          domain.AgentID     `json:"id"`
	Name        string             `json:"name"`
	DisplayName string             `json:"display_name"`
	Status      domain.AgentStatus `json:"status"`

	// Persona defines the agent's character identity and behavioral style.
	Persona *AgentPersona `json:"persona,omitempty"`

	// Progression
	Level      int   `json:"level"`
	XP         int64 `json:"xp"`
	XPToLevel  int64 `json:"xp_to_level"`
	DeathCount int   `json:"death_count"`

	// Capabilities & Trust
	Tier      domain.TrustTier `json:"tier"`
	Equipment []domain.Tool    `json:"equipment"`
	Guilds    []domain.GuildID `json:"guilds"`

	// Skill Proficiencies
	SkillProficiencies map[domain.SkillTag]domain.SkillProficiency `json:"skill_proficiencies"`

	// State
	CurrentQuest  *domain.QuestID `json:"current_quest,omitempty"`
	CurrentParty  *domain.PartyID `json:"current_party,omitempty"`
	CooldownUntil *time.Time      `json:"cooldown_until,omitempty"`

	// Stats
	Stats AgentStats `json:"stats"`

	// Backing config
	Config AgentConfig `json:"config"`

	// NPC flag
	IsNPC bool `json:"is_npc,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AgentPersona defines an agent's character identity and behavioral style.
type AgentPersona struct {
	SystemPrompt string   `json:"system_prompt"`
	Backstory    string   `json:"backstory"`
	Traits       []string `json:"traits,omitempty"`
	Style        string   `json:"style,omitempty"`
}

// AgentConfig holds the actual implementation details behind the RPG facade.
type AgentConfig struct {
	Provider     string            `json:"provider"`
	Model        string            `json:"model"`
	SystemPrompt string            `json:"system_prompt"`
	Temperature  float64           `json:"temperature"`
	MaxTokens    int               `json:"max_tokens"`
	Metadata     map[string]string `json:"metadata"`
}

// AgentStats tracks lifetime performance metrics for an agent.
type AgentStats struct {
	QuestsCompleted  int     `json:"quests_completed"`
	QuestsFailed     int     `json:"quests_failed"`
	BossesDefeated   int     `json:"bosses_defeated"`
	BossesFailed     int     `json:"bosses_failed"`
	TotalXPEarned    int64   `json:"total_xp_earned"`
	TotalXPSpent     int64   `json:"total_xp_spent"`
	AvgQualityScore  float64 `json:"avg_quality_score"`
	AvgEfficiency    float64 `json:"avg_efficiency"`
	PartiesLed       int     `json:"parties_led"`
	QuestsDecomposed int     `json:"quests_decomposed"`
}

// =============================================================================
// AGENT METHODS
// =============================================================================

// HasSkill returns true if the agent has the specified skill.
func (a *Agent) HasSkill(skill domain.SkillTag) bool {
	if a.SkillProficiencies == nil {
		return false
	}
	_, exists := a.SkillProficiencies[skill]
	return exists
}

// GetProficiency returns the proficiency for a skill.
func (a *Agent) GetProficiency(skill domain.SkillTag) domain.SkillProficiency {
	if a.SkillProficiencies != nil {
		if prof, exists := a.SkillProficiencies[skill]; exists {
			return prof
		}
	}
	return domain.SkillProficiency{}
}

// GetSkillTags returns all skills the agent has.
func (a *Agent) GetSkillTags() []domain.SkillTag {
	if a.SkillProficiencies == nil {
		return nil
	}
	skills := make([]domain.SkillTag, 0, len(a.SkillProficiencies))
	for skill := range a.SkillProficiencies {
		skills = append(skills, skill)
	}
	return skills
}

// EnsureSkillProficiencies initializes the SkillProficiencies map if nil.
func (a *Agent) EnsureSkillProficiencies() {
	if a.SkillProficiencies == nil {
		a.SkillProficiencies = make(map[domain.SkillTag]domain.SkillProficiency)
	}
}

// AddSkill adds a new skill to the agent at Novice level.
func (a *Agent) AddSkill(skill domain.SkillTag) {
	a.EnsureSkillProficiencies()
	if _, exists := a.SkillProficiencies[skill]; !exists {
		a.SkillProficiencies[skill] = domain.SkillProficiency{
			Level:      domain.ProficiencyNovice,
			Progress:   0,
			TotalXP:    0,
			QuestsUsed: 0,
		}
	}
}

// =============================================================================
// GRAPHABLE IMPLEMENTATION
// =============================================================================

// EntityID returns the 6-part entity ID for this agent.
func (a *Agent) EntityID() string {
	return string(a.ID)
}

// Triples returns all semantic facts about this agent.
func (a *Agent) Triples() []message.Triple {
	now := time.Now()
	source := "agent_progression"
	entityID := a.EntityID()

	triples := []message.Triple{
		// Identity
		{Subject: entityID, Predicate: "agent.identity.name", Object: a.Name, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.identity.display_name", Object: a.DisplayName, Source: source, Timestamp: now, Confidence: 1.0},

		// Status
		{Subject: entityID, Predicate: "agent.status.state", Object: string(a.Status), Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.npc.flag", Object: a.IsNPC, Source: source, Timestamp: now, Confidence: 1.0},

		// Progression
		{Subject: entityID, Predicate: "agent.progression.level", Object: a.Level, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.progression.xp.current", Object: a.XP, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.progression.xp.to_level", Object: a.XPToLevel, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.progression.tier", Object: int(a.Tier), Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.progression.death_count", Object: a.DeathCount, Source: source, Timestamp: now, Confidence: 1.0},

		// Stats
		{Subject: entityID, Predicate: "agent.stats.quests_completed", Object: a.Stats.QuestsCompleted, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.stats.quests_failed", Object: a.Stats.QuestsFailed, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.stats.bosses_defeated", Object: a.Stats.BossesDefeated, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.stats.total_xp_earned", Object: a.Stats.TotalXPEarned, Source: source, Timestamp: now, Confidence: 1.0},

		// Lifecycle
		{Subject: entityID, Predicate: "agent.lifecycle.created_at", Object: a.CreatedAt.Format(time.RFC3339), Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.lifecycle.updated_at", Object: a.UpdatedAt.Format(time.RFC3339), Source: source, Timestamp: now, Confidence: 1.0},
	}

	// Guild memberships
	for _, guildID := range a.Guilds {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "agent.membership.guild", Object: string(guildID),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Skill proficiencies
	for skill, prof := range a.SkillProficiencies {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: fmt.Sprintf("agent.skill.%s.level", skill), Object: int(prof.Level),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: fmt.Sprintf("agent.skill.%s.total_xp", skill), Object: prof.TotalXP,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Optional relationships
	if a.CurrentQuest != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "agent.assignment.quest", Object: string(*a.CurrentQuest),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	if a.CurrentParty != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "agent.membership.party", Object: string(*a.CurrentParty),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	if a.CooldownUntil != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "agent.status.cooldown_until", Object: a.CooldownUntil.Format(time.RFC3339),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	return triples
}
