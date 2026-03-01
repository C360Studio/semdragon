package semdragons

import (
	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// DOMAIN CONFIGURATION - Aliases from domain/ package (single source of truth)
// =============================================================================

// DomainID uniquely identifies a domain configuration.
type DomainID = domain.ID

// Standard domain IDs.
const (
	DomainSoftware = domain.DomainSoftware
	DomainDnD      = domain.DomainDnD
	DomainResearch = domain.DomainResearch
)

// DomainConfig holds the configuration for a specific domain.
type DomainConfig = domain.Config

// DomainSkill defines a skill available in a domain.
type DomainSkill = domain.Skill

// DomainVocabulary provides domain-specific terminology overrides.
type DomainVocabulary = domain.Vocabulary
