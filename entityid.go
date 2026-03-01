package semdragons

import (
	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// ENTITY ID - Aliases from domain/ package (single source of truth)
// =============================================================================

// EntityType constants for the type part of entity IDs.
const (
	EntityTypeQuest  = domain.EntityTypeQuest
	EntityTypeAgent  = domain.EntityTypeAgent
	EntityTypeParty  = domain.EntityTypeParty
	EntityTypeGuild  = domain.EntityTypeGuild
	EntityTypeBattle = domain.EntityTypeBattle
)

// BoardConfig holds the configuration for a quest board instance.
type BoardConfig = domain.BoardConfig

// DefaultBoardConfig returns a reasonable default configuration.
func DefaultBoardConfig() BoardConfig {
	return domain.DefaultBoardConfig()
}

// ParsedEntityID holds the parsed components of a 6-part entity ID.
type ParsedEntityID = domain.ParsedEntityID

// ParseEntityID parses a 6-part entity ID into its components.
func ParseEntityID(id string) (*ParsedEntityID, error) {
	return domain.ParseEntityID(id)
}

// ExtractInstance extracts the instance part (last segment) from an entity ID.
func ExtractInstance(id string) string {
	return domain.ExtractInstance(id)
}

// ExtractType extracts the type part (second to last segment) from an entity ID.
func ExtractType(id string) string {
	return domain.ExtractType(id)
}

// GenerateInstance generates a random instance ID (16-character hex string).
func GenerateInstance() string {
	return domain.GenerateInstance()
}

// GenerateShortInstance generates a shorter random instance ID (8-character hex string).
func GenerateShortInstance() string {
	return domain.GenerateShortInstance()
}

// --- Type-Safe ID Wrappers ---

// ToQuestID converts an entity ID string to QuestID.
func ToQuestID(entityID string) QuestID {
	return domain.ToQuestID(entityID)
}

// ToAgentID converts an entity ID string to AgentID.
func ToAgentID(entityID string) AgentID {
	return domain.ToAgentID(entityID)
}

// ToPartyID converts an entity ID string to PartyID.
func ToPartyID(entityID string) PartyID {
	return domain.ToPartyID(entityID)
}

// ToGuildID converts an entity ID string to GuildID.
func ToGuildID(entityID string) GuildID {
	return domain.ToGuildID(entityID)
}

// ToBattleID converts an entity ID string to BattleID.
func ToBattleID(entityID string) BattleID {
	return domain.ToBattleID(entityID)
}

// --- Validation ---

// IsValidEntityID checks if an ID has the correct 6-part format.
func IsValidEntityID(id string) bool {
	return domain.IsValidEntityID(id)
}

// IsQuestID checks if the entity ID is for a quest.
func IsQuestID(id string) bool {
	return domain.IsQuestID(id)
}

// IsAgentID checks if the entity ID is for an agent.
func IsAgentID(id string) bool {
	return domain.IsAgentID(id)
}

// IsPartyID checks if the entity ID is for a party.
func IsPartyID(id string) bool {
	return domain.IsPartyID(id)
}

// IsGuildID checks if the entity ID is for a guild.
func IsGuildID(id string) bool {
	return domain.IsGuildID(id)
}

// IsBattleID checks if the entity ID is for a battle.
func IsBattleID(id string) bool {
	return domain.IsBattleID(id)
}
