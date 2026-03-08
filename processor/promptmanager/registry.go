package promptmanager

import (
	"fmt"
	"slices"
	"sort"
	"sync"

	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// PROMPT REGISTRY - Storage, filtering, and domain catalog loading
// =============================================================================

// PromptRegistry manages prompt fragments and provider styles.
// Thread-safe for concurrent access.
type PromptRegistry struct {
	mu        sync.RWMutex
	fragments map[string]*PromptFragment
	styles    map[string]ProviderStyle
}

// NewPromptRegistry creates an empty prompt registry.
func NewPromptRegistry() *PromptRegistry {
	return &PromptRegistry{
		fragments: make(map[string]*PromptFragment),
		styles:    make(map[string]ProviderStyle),
	}
}

// Register adds a single fragment to the registry.
func (r *PromptRegistry) Register(f *PromptFragment) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fragments[f.ID] = f
}

// RegisterDomainCatalog converts a DomainCatalog into gated PromptFragments.
// This is the primary way fragments get loaded — from the domain.
//
// Generates fragments:
//   - SystemBase → one fragment, CategorySystemBase, no gating
//   - Each TierGuardrails[tier] → CategoryTierGuardrails, gated MinTier=MaxTier=tier
//   - Each SkillFragments[skill] → CategorySkillContext, gated Skills=[skill]
func (r *PromptRegistry) RegisterDomainCatalog(catalog *DomainCatalog) {
	r.mu.Lock()
	defer r.mu.Unlock()

	prefix := string(catalog.DomainID)

	// SystemBase → single ungated fragment
	if catalog.SystemBase != "" {
		id := fmt.Sprintf("%s.system-base", prefix)
		r.fragments[id] = &PromptFragment{
			ID:       id,
			Category: CategorySystemBase,
			Content:  catalog.SystemBase,
			Priority: 0,
		}
	}

	// TierGuardrails → one fragment per tier, gated to exact tier match
	for tier, content := range catalog.TierGuardrails {
		id := fmt.Sprintf("%s.tier-guardrails.%s", prefix, tier.String())
		r.fragments[id] = &PromptFragment{
			ID:       id,
			Category: CategoryTierGuardrails,
			Content:  content,
			Priority: int(tier), // Order by tier value
			MinTier:  &tier,
			MaxTier:  &tier,
		}
	}

	// SkillFragments → one fragment per skill, gated to that skill
	for skill, content := range catalog.SkillFragments {
		id := fmt.Sprintf("%s.skill.%s", prefix, string(skill))
		r.fragments[id] = &PromptFragment{
			ID:       id,
			Category: CategorySkillContext,
			Content:  content,
			Priority: 0,
			Skills:   []domain.SkillTag{skill},
		}
	}
}

// RegisterProviderStyles loads the default provider formatting conventions.
func (r *PromptRegistry) RegisterProviderStyles() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.styles["anthropic"] = ProviderStyle{
		Provider:  "anthropic",
		PreferXML: true,
	}
	r.styles["openai"] = ProviderStyle{
		Provider:       "openai",
		PreferMarkdown: true,
	}
	r.styles["ollama"] = ProviderStyle{
		Provider:       "ollama",
		PreferMarkdown: true,
	}
}

// GetFragmentsForContext returns matching fragments sorted by category then priority.
// Fragment matching follows the same pattern as GetToolsForQuest in tools.go:
//   - Tier in range [MinTier, MaxTier] (nil = open)
//   - Agent has >= 1 of fragment's Skills (empty = any)
//   - Provider matches (empty = any)
//   - Agent belongs to fragment's GuildID (nil = any)
func (r *PromptRegistry) GetFragmentsForContext(ctx AssemblyContext) []*PromptFragment {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matched []*PromptFragment
	for _, f := range r.fragments {
		if r.fragmentMatches(f, ctx) {
			matched = append(matched, f)
		}
	}

	sort.Slice(matched, func(i, j int) bool {
		if matched[i].Category != matched[j].Category {
			return matched[i].Category < matched[j].Category
		}
		return matched[i].Priority < matched[j].Priority
	})

	return matched
}

// GetStyle returns formatting conventions for a provider.
// Returns a zero-value ProviderStyle (no special formatting) for unknown providers.
func (r *PromptRegistry) GetStyle(provider string) ProviderStyle {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.styles[provider]
}

// fragmentMatches checks if a fragment is applicable for the given context.
func (r *PromptRegistry) fragmentMatches(f *PromptFragment, ctx AssemblyContext) bool {
	// Tier gating
	if f.MinTier != nil && ctx.Tier < *f.MinTier {
		return false
	}
	if f.MaxTier != nil && ctx.Tier > *f.MaxTier {
		return false
	}

	// Skill gating: agent must have at least one of the fragment's required skills
	if len(f.Skills) > 0 {
		hasSkill := false
		for _, requiredSkill := range f.Skills {
			if ctx.Skills != nil {
				if _, ok := ctx.Skills[requiredSkill]; ok {
					hasSkill = true
					break
				}
			}
			if slices.Contains(ctx.RequiredSkills, requiredSkill) {
				hasSkill = true
				break
			}
		}
		if !hasSkill {
			return false
		}
	}

	// Provider gating
	if len(f.Providers) > 0 && !slices.Contains(f.Providers, ctx.Provider) {
		return false
	}

	// Guild gating
	if f.GuildID != nil && !slices.Contains(ctx.Guilds, *f.GuildID) {
		return false
	}

	// Runtime condition — evaluated last, after all structural gates pass.
	if f.Condition != nil && !f.Condition(ctx) {
		return false
	}

	return true
}

// FragmentCount returns the total number of registered fragments.
func (r *PromptRegistry) FragmentCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.fragments)
}
