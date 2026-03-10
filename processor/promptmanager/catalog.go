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

	// StructuralChecklist defines binary pass/fail requirements that are checked
	// alongside weighted criteria. Any checklist failure = automatic defeat
	// regardless of criteria scores. Examples: "TDD required", "no tabs".
	// These are also injected into agent task prompts so agents self-check.
	StructuralChecklist []ChecklistItem
}

// ChecklistItem is a binary pass/fail structural requirement.
// Unlike weighted criteria, checklist items have no score — they either pass or fail.
type ChecklistItem struct {
	// Name is a short identifier (e.g., "tdd-required").
	Name string
	// Requirement describes what must be true (e.g., "All code must include tests").
	Requirement string
	// MinTier is the minimum trust tier for this item to apply.
	// Items with a higher MinTier than the agent's tier are skipped during
	// both prompt injection and judge evaluation. Zero value (TierApprentice)
	// means the item applies to all tiers.
	MinTier domain.TrustTier
	// RequiredSkills limits this item to quests that require at least one of
	// these skills. Empty means the item applies regardless of quest skills.
	// Example: "tests-included" only applies to quests requiring CodeGen.
	RequiredSkills []domain.SkillTag
}

// FilterChecklist returns only the checklist items that apply given the quest's
// tier and required skills. An item is included when:
//   - The quest's tier meets or exceeds the item's MinTier, AND
//   - The item has no RequiredSkills, OR the quest's skills overlap with them.
func FilterChecklist(items []ChecklistItem, tier domain.TrustTier, questSkills []domain.SkillTag) []ChecklistItem {
	var filtered []ChecklistItem
	for _, item := range items {
		if tier < item.MinTier {
			continue
		}
		if len(item.RequiredSkills) > 0 && !skillsOverlap(item.RequiredSkills, questSkills) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

// skillsOverlap returns true if any skill in a appears in b.
func skillsOverlap(a, b []domain.SkillTag) bool {
	for _, sa := range a {
		for _, sb := range b {
			if sa == sb {
				return true
			}
		}
	}
	return false
}
