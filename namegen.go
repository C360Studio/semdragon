package semdragons

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
)

// =============================================================================
// NAME GENERATION - Unique character names for agents
// =============================================================================

// NameGenerator creates unique character names for agents.
type NameGenerator struct {
	storage *Storage
}

// NewNameGenerator creates a new name generator.
func NewNameGenerator(storage *Storage) *NameGenerator {
	return &NameGenerator{storage: storage}
}

// GenerateName creates a unique character name based on agent skills and traits.
// The name is guaranteed to be unique within the system.
func (g *NameGenerator) GenerateName(ctx context.Context, agent *Agent) (string, error) {
	for attempts := 0; attempts < 10; attempts++ {
		name := g.generateCandidate(agent)

		// Check uniqueness
		isUnique, err := g.isNameUnique(ctx, name)
		if err != nil {
			return "", fmt.Errorf("checking name uniqueness: %w", err)
		}
		if isUnique {
			return name, nil
		}
	}

	// Fallback: append random suffix
	base := g.generateCandidate(agent)
	return fmt.Sprintf("%s-%d", base, rand.Intn(9999)), nil
}

// isNameUnique checks if a display name is already taken.
func (g *NameGenerator) isNameUnique(ctx context.Context, name string) (bool, error) {
	agents, err := g.storage.ListAllAgents(ctx)
	if err != nil {
		return false, err
	}

	nameLower := strings.ToLower(name)
	for _, agent := range agents {
		if strings.ToLower(agent.DisplayName) == nameLower {
			return false, nil
		}
	}
	return true, nil
}

// generateCandidate creates a name candidate based on agent attributes.
func (g *NameGenerator) generateCandidate(agent *Agent) string {
	// Name components inspired by agent's primary skill
	prefixes := g.prefixesForSkills(agent.GetSkillTags())
	suffixes := g.suffixesForTier(agent.Tier)

	prefix := prefixes[rand.Intn(len(prefixes))]
	suffix := suffixes[rand.Intn(len(suffixes))]

	return prefix + suffix
}

// prefixesForSkills returns name prefixes themed to the agent's skills.
func (g *NameGenerator) prefixesForSkills(skills []SkillTag) []string {
	prefixMap := map[SkillTag][]string{
		SkillCodeGen:       {"Code", "Cipher", "Binary", "Syntax", "Logic", "Script", "Byte"},
		SkillCodeReview:    {"Sharp", "Keen", "Hawk", "Vigil", "Scout", "Watch", "Clear"},
		SkillAnalysis:      {"Sage", "Deep", "Mind", "Think", "Wise", "Know", "Lore"},
		SkillDataTransform: {"Flux", "Morph", "Shift", "Flow", "Change", "Trans", "Meta"},
		SkillResearch:      {"Quest", "Seek", "Lore", "Truth", "Find", "Scholar", "Scribe"},
		SkillPlanning:      {"Forge", "Craft", "Plot", "Chart", "Path", "Vision", "Helm"},
		SkillSummarization: {"Brief", "Core", "Distill", "Clear", "Pure", "Prime", "Key"},
		SkillCustomerComms: {"Voice", "Herald", "Link", "Bridge", "Envoy", "Speaker", "Charm"},
		SkillTraining:      {"Mentor", "Guide", "Elder", "Sage", "Teacher", "Master", "Wise"},
	}

	var all []string
	for _, skill := range skills {
		if prefixes, ok := prefixMap[skill]; ok {
			all = append(all, prefixes...)
		}
	}

	if len(all) == 0 {
		// Default prefixes
		all = []string{"Shadow", "Storm", "Fire", "Frost", "Iron", "Stone", "Wind", "Star"}
	}

	return all
}

// suffixesForTier returns name suffixes themed to trust tier.
func (g *NameGenerator) suffixesForTier(tier TrustTier) []string {
	tierSuffixes := map[TrustTier][]string{
		TierApprentice:  {"spark", "flame", "seed", "wisp", "shade", "drift", "breeze"},
		TierJourneyman:  {"walker", "blade", "weaver", "crafter", "seeker", "runner", "binder"},
		TierExpert:      {"master", "forger", "keeper", "warden", "striker", "shaper", "wright"},
		TierMaster:      {"lord", "sage", "crown", "prime", "arch", "grand", "high"},
		TierGrandmaster: {"eternal", "sovereign", "ancient", "mythic", "legend", "titan", "oracle"},
	}

	if suffixes, ok := tierSuffixes[tier]; ok {
		return suffixes
	}
	return tierSuffixes[TierApprentice]
}

// SetDisplayName sets the agent's display name after validating uniqueness.
func (g *NameGenerator) SetDisplayName(ctx context.Context, agent *Agent, name string) error {
	// Validate name
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("display name cannot be empty")
	}
	if len(name) > 32 {
		return fmt.Errorf("display name too long (max 32 characters)")
	}

	// Check uniqueness (unless it's the agent's current name)
	if agent.DisplayName != name {
		isUnique, err := g.isNameUnique(ctx, name)
		if err != nil {
			return fmt.Errorf("checking name uniqueness: %w", err)
		}
		if !isUnique {
			return fmt.Errorf("display name '%s' is already taken", name)
		}
	}

	agent.DisplayName = name
	return nil
}

// SuggestNames returns a list of unique name suggestions for an agent.
func (g *NameGenerator) SuggestNames(ctx context.Context, agent *Agent, count int) ([]string, error) {
	suggestions := make([]string, 0, count)
	seen := make(map[string]bool)

	for attempts := 0; attempts < count*3 && len(suggestions) < count; attempts++ {
		name := g.generateCandidate(agent)
		if seen[name] {
			continue
		}
		seen[name] = true

		isUnique, err := g.isNameUnique(ctx, name)
		if err != nil {
			return nil, err
		}
		if isUnique {
			suggestions = append(suggestions, name)
		}
	}

	return suggestions, nil
}
