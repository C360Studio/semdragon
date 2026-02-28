package semdragons

// =============================================================================
// DOMAIN CONFIGURATION - Vocabulary and skill definitions per domain
// =============================================================================
// Domains provide skinning for different use cases (software dev, D&D, research).
// Each domain defines its own vocabulary (what we call things) and skill pool
// (what capabilities agents can develop).
// =============================================================================

// DomainID uniquely identifies a domain configuration.
type DomainID string

// Standard domain IDs.
const (
	DomainSoftware DomainID = "software"
	DomainDnD      DomainID = "dnd"
	DomainResearch DomainID = "research"
)

// DomainConfig holds the configuration for a specific domain.
type DomainConfig struct {
	ID          DomainID `json:"id"`   // "software", "dnd", "research"
	Name        string   `json:"name"` // "Software Development"
	Description string   `json:"description"`

	// Domain-specific skill definitions
	Skills []DomainSkill `json:"skills"`

	// Vocabulary overrides (RPG terms → domain terms)
	Vocabulary DomainVocabulary `json:"vocabulary"`
}

// DomainSkill defines a skill available in a domain.
type DomainSkill struct {
	Tag         SkillTag `json:"tag"`  // Internal identifier
	Name        string   `json:"name"` // Display name
	Description string   `json:"description"`
	Icon        string   `json:"icon,omitempty"`
}

// DomainVocabulary provides domain-specific terminology overrides.
type DomainVocabulary struct {
	// Entity names
	Agent      string `json:"agent"`       // "Developer", "Adventurer"
	Quest      string `json:"quest"`       // "Task", "Quest"
	Party      string `json:"party"`       // "Team", "Party"
	Guild      string `json:"guild"`       // "Guild" (usually keep this)
	BossBattle string `json:"boss_battle"` // "Code Review", "Boss Battle"

	// Progression
	XP    string `json:"xp"`    // "Points", "XP"
	Level string `json:"level"` // "Seniority", "Level"

	// Tier names (optional) - maps trust tiers to domain-specific names
	TierNames map[TrustTier]string `json:"tier_names,omitempty"`

	// Role names (optional) - maps party roles to domain-specific names
	RoleNames map[PartyRole]string `json:"role_names,omitempty"`
}

// --- Default Vocabulary ---

var defaultVocabulary = map[string]string{
	"agent":       "Agent",
	"quest":       "Quest",
	"party":       "Party",
	"guild":       "Guild",
	"boss_battle": "Boss Battle",
	"xp":          "XP",
	"level":       "Level",
}

// Get returns a vocabulary term, falling back to the default if not set.
func (v *DomainVocabulary) Get(key string) string {
	switch key {
	case "agent":
		if v.Agent != "" {
			return v.Agent
		}
	case "quest":
		if v.Quest != "" {
			return v.Quest
		}
	case "party":
		if v.Party != "" {
			return v.Party
		}
	case "guild":
		if v.Guild != "" {
			return v.Guild
		}
	case "boss_battle":
		if v.BossBattle != "" {
			return v.BossBattle
		}
	case "xp":
		if v.XP != "" {
			return v.XP
		}
	case "level":
		if v.Level != "" {
			return v.Level
		}
	}
	return defaultVocabulary[key]
}

// GetTierName returns the domain-specific name for a trust tier.
func (v *DomainVocabulary) GetTierName(tier TrustTier) string {
	if v.TierNames != nil {
		if name, ok := v.TierNames[tier]; ok {
			return name
		}
	}
	// Default tier names
	switch tier {
	case TierApprentice:
		return "Apprentice"
	case TierJourneyman:
		return "Journeyman"
	case TierExpert:
		return "Expert"
	case TierMaster:
		return "Master"
	case TierGrandmaster:
		return "Grandmaster"
	default:
		return "Unknown"
	}
}

// GetRoleName returns the domain-specific name for a party role.
func (v *DomainVocabulary) GetRoleName(role PartyRole) string {
	if v.RoleNames != nil {
		if name, ok := v.RoleNames[role]; ok {
			return name
		}
	}
	// Default role names
	switch role {
	case RoleLead:
		return "Lead"
	case RoleExecutor:
		return "Executor"
	case RoleReviewer:
		return "Reviewer"
	case RoleScout:
		return "Scout"
	default:
		return string(role)
	}
}

// --- Domain Validation ---

// HasSkill checks if a skill tag is valid for this domain.
func (d *DomainConfig) HasSkill(tag SkillTag) bool {
	for _, skill := range d.Skills {
		if skill.Tag == tag {
			return true
		}
	}
	return false
}

// GetSkill returns the skill definition for a tag, or nil if not found.
func (d *DomainConfig) GetSkill(tag SkillTag) *DomainSkill {
	for i := range d.Skills {
		if d.Skills[i].Tag == tag {
			return &d.Skills[i]
		}
	}
	return nil
}

// SkillTags returns all skill tags defined for this domain.
func (d *DomainConfig) SkillTags() []SkillTag {
	tags := make([]SkillTag, len(d.Skills))
	for i, skill := range d.Skills {
		tags[i] = skill.Tag
	}
	return tags
}
