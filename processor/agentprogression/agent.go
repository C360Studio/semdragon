package agentprogression

import (
	"fmt"
	"strings"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"

	"github.com/c360studio/semdragons/domain"
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

	// Archetype is the agent's class identity. Fixed at creation; never changes on level-up.
	Archetype domain.AgentArchetype `json:"archetype,omitempty"`

	// Capabilities & Trust
	Tier      domain.TrustTier `json:"tier"`
	Equipment []domain.Tool    `json:"equipment"`
	Guild     domain.GuildID   `json:"guild,omitempty"`

	// Skill Proficiencies
	SkillProficiencies map[domain.SkillTag]domain.SkillProficiency `json:"skill_proficiencies"`

	// State
	CurrentQuest  *domain.QuestID `json:"current_quest,omitempty"`
	CurrentParty  *domain.PartyID `json:"current_party,omitempty"`
	CooldownUntil *time.Time      `json:"cooldown_until,omitempty"`

	// Store inventory (reconstructed from agent entity triples)
	OwnedTools    map[string]OwnedTool `json:"owned_tools,omitempty"`
	Consumables   map[string]int       `json:"consumables,omitempty"`
	TotalSpent    int64                `json:"total_spent"`
	ActiveEffects []AgentEffect        `json:"active_effects,omitempty"`

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
	PeerReviewAvg    float64 `json:"peer_review_avg"`
	PeerReviewQ1Avg  float64 `json:"peer_review_q1_avg"`
	PeerReviewQ2Avg  float64 `json:"peer_review_q2_avg"`
	PeerReviewQ3Avg  float64 `json:"peer_review_q3_avg"`
	PeerReviewCount  int     `json:"peer_review_count"`
}

// OwnedTool tracks an agent's purchased tool (stored as agent entity triples).
type OwnedTool struct {
	StoreItemID   string    `json:"store_item_id"`  // Entity ref → storeitem entity ID
	XPSpent       int64     `json:"xp_spent"`       // XP paid for this tool
	UsesRemaining int       `json:"uses_remaining"` // -1 = permanent
	PurchasedAt   time.Time `json:"purchased_at"`
}

// AgentEffect tracks a consumable effect currently active on an agent.
type AgentEffect struct {
	EffectType      string            `json:"effect_type"`        // xp_boost, quality_shield, etc.
	QuestsRemaining int               `json:"quests_remaining"`   // Quests until effect expires
	QuestID         *domain.QuestID   `json:"quest_id,omitempty"` // Quest that triggered the effect
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

		// Archetype (class identity — fixed at creation)
		{Subject: entityID, Predicate: "agent.identity.archetype", Object: string(a.Archetype), Source: source, Timestamp: now, Confidence: 1.0},

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

	// Guild membership (single guild)
	if a.Guild != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "agent.membership.guild", Object: string(a.Guild),
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

	// Owned tools — each tool creates a relationship edge to its storeitem entity
	for itemID, tool := range a.OwnedTools {
		prefix := fmt.Sprintf("agent.inventory.tool.%s", itemID)
		triples = append(triples,
			message.Triple{Subject: entityID, Predicate: prefix, Object: tool.StoreItemID, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: prefix + ".xp_spent", Object: tool.XPSpent, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: prefix + ".uses", Object: tool.UsesRemaining, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: prefix + ".purchased_at", Object: tool.PurchasedAt.Format(time.RFC3339), Source: source, Timestamp: now, Confidence: 1.0},
		)
	}

	// Consumables — count owned per item
	for itemID, count := range a.Consumables {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: fmt.Sprintf("agent.inventory.consumable.%s", itemID), Object: count,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Total XP spent in store
	if a.TotalSpent > 0 {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "agent.inventory.total_spent", Object: a.TotalSpent,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Active consumable effects
	for _, eff := range a.ActiveEffects {
		prefix := fmt.Sprintf("agent.effects.%s", eff.EffectType)
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: prefix + ".remaining", Object: eff.QuestsRemaining,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
		if eff.QuestID != nil {
			triples = append(triples, message.Triple{
				Subject: entityID, Predicate: prefix + ".quest", Object: string(*eff.QuestID),
				Source: source, Timestamp: now, Confidence: 1.0,
			})
		}
	}

	// Peer review reputation
	if a.Stats.PeerReviewCount > 0 {
		triples = append(triples,
			message.Triple{Subject: entityID, Predicate: "agent.reputation.peer_avg", Object: a.Stats.PeerReviewAvg, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: "agent.reputation.peer_q1_avg", Object: a.Stats.PeerReviewQ1Avg, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: "agent.reputation.peer_q2_avg", Object: a.Stats.PeerReviewQ2Avg, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: "agent.reputation.peer_q3_avg", Object: a.Stats.PeerReviewQ3Avg, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: "agent.reputation.peer_count", Object: a.Stats.PeerReviewCount, Source: source, Timestamp: now, Confidence: 1.0},
		)
	}

	return triples
}

// =============================================================================
// RECONSTRUCTION
// =============================================================================

// AgentFromEntityState reconstructs an Agent from graph EntityState.
func AgentFromEntityState(entity *graph.EntityState) *Agent {
	if entity == nil {
		return nil
	}

	a := &Agent{
		ID:                 domain.AgentID(entity.ID),
		SkillProficiencies: make(map[domain.SkillTag]domain.SkillProficiency),
		OwnedTools:         make(map[string]OwnedTool),
		Consumables:        make(map[string]int),
	}

	for _, triple := range entity.Triples {
		switch triple.Predicate {
		// Identity
		case "agent.identity.name":
			a.Name = domain.AsString(triple.Object)
		case "agent.identity.display_name":
			a.DisplayName = domain.AsString(triple.Object)
		case "agent.identity.archetype":
			a.Archetype = domain.AgentArchetype(domain.AsString(triple.Object))

		// Status
		case "agent.status.state":
			a.Status = domain.AgentStatus(domain.AsString(triple.Object))
		case "agent.npc.flag":
			a.IsNPC = domain.AsBool(triple.Object)
		case "agent.status.cooldown_until":
			t := domain.AsTime(triple.Object)
			a.CooldownUntil = &t

		// Progression
		case "agent.progression.level":
			a.Level = domain.AsInt(triple.Object)
		case "agent.progression.xp.current":
			a.XP = domain.AsInt64(triple.Object)
		case "agent.progression.xp.to_level":
			a.XPToLevel = domain.AsInt64(triple.Object)
		case "agent.progression.tier":
			a.Tier = domain.TrustTier(domain.AsInt(triple.Object))
		case "agent.progression.death_count":
			a.DeathCount = domain.AsInt(triple.Object)

		// Stats
		case "agent.stats.quests_completed":
			a.Stats.QuestsCompleted = domain.AsInt(triple.Object)
		case "agent.stats.quests_failed":
			a.Stats.QuestsFailed = domain.AsInt(triple.Object)
		case "agent.stats.bosses_defeated":
			a.Stats.BossesDefeated = domain.AsInt(triple.Object)
		case "agent.stats.total_xp_earned":
			a.Stats.TotalXPEarned = domain.AsInt64(triple.Object)

		// Lifecycle
		case "agent.lifecycle.created_at":
			a.CreatedAt = domain.AsTime(triple.Object)
		case "agent.lifecycle.updated_at":
			a.UpdatedAt = domain.AsTime(triple.Object)

		// Relationships
		case "agent.membership.guild":
			a.Guild = domain.GuildID(domain.AsString(triple.Object))
		case "agent.assignment.quest":
			questID := domain.QuestID(domain.AsString(triple.Object))
			a.CurrentQuest = &questID
		case "agent.membership.party":
			partyID := domain.PartyID(domain.AsString(triple.Object))
			a.CurrentParty = &partyID

		// Inventory total spent
		case "agent.inventory.total_spent":
			a.TotalSpent = domain.AsInt64(triple.Object)

		// Peer review reputation
		case "agent.reputation.peer_avg":
			a.Stats.PeerReviewAvg = domain.AsFloat64(triple.Object)
		case "agent.reputation.peer_q1_avg":
			a.Stats.PeerReviewQ1Avg = domain.AsFloat64(triple.Object)
		case "agent.reputation.peer_q2_avg":
			a.Stats.PeerReviewQ2Avg = domain.AsFloat64(triple.Object)
		case "agent.reputation.peer_q3_avg":
			a.Stats.PeerReviewQ3Avg = domain.AsFloat64(triple.Object)
		case "agent.reputation.peer_count":
			a.Stats.PeerReviewCount = domain.AsInt(triple.Object)
		}

		// Handle skill proficiencies (dynamic predicates)
		// Format: agent.skill.{skill}.level or agent.skill.{skill}.total_xp
		if len(triple.Predicate) > 12 && triple.Predicate[:12] == "agent.skill." {
			rest := triple.Predicate[12:] // e.g., "coding.level" or "coding.total_xp"
			for i := len(rest) - 1; i >= 0; i-- {
				if rest[i] == '.' {
					skillTag := domain.SkillTag(rest[:i])
					suffix := rest[i+1:]

					prof := a.SkillProficiencies[skillTag]
					switch suffix {
					case "level":
						prof.Level = domain.ProficiencyLevel(domain.AsInt(triple.Object))
					case "total_xp":
						prof.TotalXP = domain.AsInt64(triple.Object)
					}
					a.SkillProficiencies[skillTag] = prof
					break
				}
			}
		}

		// Handle owned tools (dynamic predicates)
		// Format: agent.inventory.tool.{itemID}[.field]
		if strings.HasPrefix(triple.Predicate, "agent.inventory.tool.") {
			rest := triple.Predicate[len("agent.inventory.tool."):] // e.g., "web_search" or "web_search.xp_spent"
			dotIdx := strings.Index(rest, ".")
			if dotIdx == -1 {
				// Base predicate: agent.inventory.tool.{itemID} → storeitem entity ref
				itemID := rest
				tool := a.OwnedTools[itemID]
				tool.StoreItemID = domain.AsString(triple.Object)
				a.OwnedTools[itemID] = tool
			} else {
				itemID := rest[:dotIdx]
				suffix := rest[dotIdx+1:]
				tool := a.OwnedTools[itemID]
				switch suffix {
				case "xp_spent":
					tool.XPSpent = domain.AsInt64(triple.Object)
				case "uses":
					tool.UsesRemaining = domain.AsInt(triple.Object)
				case "purchased_at":
					tool.PurchasedAt = domain.AsTime(triple.Object)
				}
				a.OwnedTools[itemID] = tool
			}
		}

		// Handle consumables (dynamic predicates)
		// Format: agent.inventory.consumable.{itemID} → count
		if strings.HasPrefix(triple.Predicate, "agent.inventory.consumable.") {
			itemID := triple.Predicate[len("agent.inventory.consumable."):]
			a.Consumables[itemID] = domain.AsInt(triple.Object)
		}

		// Handle active effects (dynamic predicates)
		// Format: agent.effects.{effectType}.remaining or agent.effects.{effectType}.quest
		if strings.HasPrefix(triple.Predicate, "agent.effects.") {
			rest := triple.Predicate[len("agent.effects."):] // e.g., "xp_boost.remaining"
			dotIdx := strings.LastIndex(rest, ".")
			if dotIdx > 0 {
				effectType := rest[:dotIdx]
				suffix := rest[dotIdx+1:]

				// Find or create effect entry
				idx := -1
				for i, eff := range a.ActiveEffects {
					if eff.EffectType == effectType {
						idx = i
						break
					}
				}
				if idx == -1 {
					a.ActiveEffects = append(a.ActiveEffects, AgentEffect{EffectType: effectType})
					idx = len(a.ActiveEffects) - 1
				}

				switch suffix {
				case "remaining":
					a.ActiveEffects[idx].QuestsRemaining = domain.AsInt(triple.Object)
				case "quest":
					questID := domain.QuestID(domain.AsString(triple.Object))
					a.ActiveEffects[idx].QuestID = &questID
				}
			}
		}
	}

	return a
}
