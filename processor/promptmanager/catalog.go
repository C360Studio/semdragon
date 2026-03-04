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

	// ReviewConfig provides domain-specific review defaults.
	// When set, bossbattle uses these instead of hardcoded criteria/judges.
	// nil = use hardcoded defaults for backward compatibility.
	ReviewConfig *ReviewConfig
}

// ReviewConfig defines domain-specific defaults for quest review.
type ReviewConfig struct {
	// DefaultReviewLevel is used when a quest doesn't specify a review level.
	DefaultReviewLevel domain.ReviewLevel

	// DefaultCriteria provides domain-appropriate evaluation criteria.
	DefaultCriteria []domain.ReviewCriterion

	// CriteriaByLevel provides per-level criteria overrides.
	// When set, criteria for the matching level take precedence over DefaultCriteria.
	CriteriaByLevel map[domain.ReviewLevel][]domain.ReviewCriterion

	// AutoPassDifficulties lists quest difficulties that skip full review
	// and auto-pass with a synthetic victory (e.g., trivial quests).
	AutoPassDifficulties []domain.QuestDifficulty

	// DefaultJudges provides domain-appropriate judges.
	DefaultJudges []domain.Judge

	// JudgesByLevel provides per-level judge overrides.
	// When set, judges for the matching level take precedence over DefaultJudges.
	JudgesByLevel map[domain.ReviewLevel][]domain.Judge
}
