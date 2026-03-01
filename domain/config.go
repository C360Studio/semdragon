package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

// =============================================================================
// ENTITY ID - Six-part dotted notation for federated entity management
// =============================================================================
// Format: org.platform.domain.system.type.instance
//
// For semdragons:
//   - domain is always "game"
//   - system is the board name
//   - type is quest, agent, party, guild, battle
//   - instance is a unique identifier
//
// Example: c360.prod.game.board1.quest.abc123
// =============================================================================

// EntityType constants for the type part of entity IDs.
const (
	EntityTypeQuest  = "quest"
	EntityTypeAgent  = "agent"
	EntityTypeParty  = "party"
	EntityTypeGuild  = "guild"
	EntityTypeBattle = "battle"
)

// BoardConfig holds the configuration for a quest board instance.
// This determines the entity ID prefix and KV bucket name.
type BoardConfig struct {
	Org      string        // Organization namespace (e.g., "c360")
	Platform string        // Deployment instance (e.g., "prod", "dev")
	Board    string        // Board name (e.g., "board1", "main")
	Domain   *DomainConfig // Optional domain skinning (vocabulary, skills)
}

// Vocab returns a vocabulary term for the configured domain.
// Falls back to default RPG terminology if no domain is configured.
func (c *BoardConfig) Vocab(key string) string {
	if c.Domain == nil {
		return defaultVocabulary[key]
	}
	return c.Domain.Vocabulary.Get(key)
}

// TierName returns the display name for a trust tier in the configured domain.
func (c *BoardConfig) TierName(tier TrustTier) string {
	if c.Domain == nil {
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
	return c.Domain.Vocabulary.GetTierName(tier)
}

// RoleName returns the display name for a party role in the configured domain.
func (c *BoardConfig) RoleName(role PartyRole) string {
	if c.Domain == nil {
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
	return c.Domain.Vocabulary.GetRoleName(role)
}

// DefaultBoardConfig returns a reasonable default configuration.
func DefaultBoardConfig() BoardConfig {
	return BoardConfig{
		Org:      "default",
		Platform: "local",
		Board:    "main",
	}
}

// Prefix returns the 5-part prefix for all entities on this board.
// Format: org.platform.game.board
func (c *BoardConfig) Prefix() string {
	return fmt.Sprintf("%s.%s.game.%s", c.Org, c.Platform, c.Board)
}

// TypePrefix returns the 5-part prefix for a specific entity type.
// Format: org.platform.game.board.type
func (c *BoardConfig) TypePrefix(entityType string) string {
	return fmt.Sprintf("%s.%s.game.%s.%s", c.Org, c.Platform, c.Board, entityType)
}

// EntityID generates a full 6-part entity ID.
// Format: org.platform.game.board.type.instance
func (c *BoardConfig) EntityID(entityType, instance string) string {
	return fmt.Sprintf("%s.%s.game.%s.%s.%s",
		c.Org, c.Platform, c.Board, entityType, instance)
}

// QuestEntityID generates a quest entity ID.
func (c *BoardConfig) QuestEntityID(instance string) string {
	return c.EntityID(EntityTypeQuest, instance)
}

// AgentEntityID generates an agent entity ID.
func (c *BoardConfig) AgentEntityID(instance string) string {
	return c.EntityID(EntityTypeAgent, instance)
}

// PartyEntityID generates a party entity ID.
func (c *BoardConfig) PartyEntityID(instance string) string {
	return c.EntityID(EntityTypeParty, instance)
}

// GuildEntityID generates a guild entity ID.
func (c *BoardConfig) GuildEntityID(instance string) string {
	return c.EntityID(EntityTypeGuild, instance)
}

// BattleEntityID generates a battle entity ID.
func (c *BoardConfig) BattleEntityID(instance string) string {
	return c.EntityID(EntityTypeBattle, instance)
}

// BucketName returns the KV bucket name for this board.
// Format: semdragons-org-platform-board (dashes, not dots - NATS KV requirement)
func (c *BoardConfig) BucketName() string {
	return fmt.Sprintf("semdragons-%s-%s-%s", c.Org, c.Platform, c.Board)
}

// --- Entity ID Parsing ---

// ParsedEntityID holds the parsed components of a 6-part entity ID.
type ParsedEntityID struct {
	Org      string
	Platform string
	Domain   string // Always "game" for semdragons
	System   string // Board name
	Type     string // quest, agent, party, guild, battle
	Instance string
}

// ParseEntityID parses a 6-part entity ID into its components.
// Returns error if the ID doesn't have exactly 6 parts.
func ParseEntityID(id string) (*ParsedEntityID, error) {
	parts := strings.Split(id, ".")
	if len(parts) != 6 {
		return nil, fmt.Errorf("invalid entity ID: expected 6 parts, got %d", len(parts))
	}
	return &ParsedEntityID{
		Org:      parts[0],
		Platform: parts[1],
		Domain:   parts[2],
		System:   parts[3],
		Type:     parts[4],
		Instance: parts[5],
	}, nil
}

// ExtractInstance extracts the instance part (last segment) from an entity ID.
// This is a fast path when you only need the instance.
func ExtractInstance(id string) string {
	if idx := strings.LastIndex(id, "."); idx >= 0 {
		return id[idx+1:]
	}
	return id
}

// ExtractType extracts the type part (second to last segment) from an entity ID.
func ExtractType(id string) string {
	parts := strings.Split(id, ".")
	if len(parts) >= 2 {
		return parts[len(parts)-2]
	}
	return ""
}

// --- Instance ID Generation ---

// GenerateInstance generates a random instance ID.
// Returns a 16-character hex string.
func GenerateInstance() string {
	return randomHex(8)
}

// GenerateShortInstance generates a shorter random instance ID.
// Returns an 8-character hex string.
func GenerateShortInstance() string {
	return randomHex(4)
}

func randomHex(n int) string {
	bytes := make([]byte, n)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// --- Type-Safe ID Wrappers ---

// These convert between semantic ID types (QuestID, AgentID, etc.)
// and full entity ID strings.

// ToQuestID converts an entity ID string to QuestID.
func ToQuestID(entityID string) QuestID {
	return QuestID(entityID)
}

// ToAgentID converts an entity ID string to AgentID.
func ToAgentID(entityID string) AgentID {
	return AgentID(entityID)
}

// ToPartyID converts an entity ID string to PartyID.
func ToPartyID(entityID string) PartyID {
	return PartyID(entityID)
}

// ToGuildID converts an entity ID string to GuildID.
func ToGuildID(entityID string) GuildID {
	return GuildID(entityID)
}

// ToBattleID converts an entity ID string to BattleID.
func ToBattleID(entityID string) BattleID {
	return BattleID(entityID)
}

// --- Validation ---

// IsValidEntityID checks if an ID has the correct 6-part format.
func IsValidEntityID(id string) bool {
	parts := strings.Split(id, ".")
	if len(parts) != 6 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
	}
	return true
}

// IsQuestID checks if the entity ID is for a quest.
func IsQuestID(id string) bool {
	return ExtractType(id) == EntityTypeQuest
}

// IsAgentID checks if the entity ID is for an agent.
func IsAgentID(id string) bool {
	return ExtractType(id) == EntityTypeAgent
}

// IsPartyID checks if the entity ID is for a party.
func IsPartyID(id string) bool {
	return ExtractType(id) == EntityTypeParty
}

// IsGuildID checks if the entity ID is for a guild.
func IsGuildID(id string) bool {
	return ExtractType(id) == EntityTypeGuild
}

// IsBattleID checks if the entity ID is for a battle.
func IsBattleID(id string) bool {
	return ExtractType(id) == EntityTypeBattle
}

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
