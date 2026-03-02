package promptmanager

import (
	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// DOMAIN CATALOG - A domain's complete prompt package
// =============================================================================
// Each domain provides a DomainCatalog alongside its DomainConfig.
// The catalog defines how agents in that domain are prompted:
//   - What agents ARE (SystemBase)
//   - What agents at each tier may do (TierGuardrails)
//   - How skills are framed (SkillFragments)
//   - How reviews are framed (JudgeSystemBase)
// =============================================================================

// DomainCatalog provides prompt content for a specific domain.
// Each domain defines one of these alongside its DomainConfig.
type DomainCatalog struct {
	// DomainID identifies which domain this catalog belongs to.
	DomainID domain.ID

	// SystemBase defines what agents ARE in this domain.
	// Example: "You are a developer in a software team."
	// Example: "You are a crew member aboard a starship."
	SystemBase string

	// TierGuardrails defines behavioral bounds per trust tier using domain vocabulary.
	// Key = tier, value = guardrail text.
	TierGuardrails map[domain.TrustTier]string

	// SkillFragments provides instructions when a quest requires a specific skill.
	// Key = skill tag (matches domain's skill definitions).
	SkillFragments map[domain.SkillTag]string

	// JudgeSystemBase frames how boss battles/reviews are described.
	// Example: "You are a senior code reviewer evaluating a developer's work output."
	// Example: "You are a Fleet Command officer evaluating a crew member's mission report."
	JudgeSystemBase string
}
