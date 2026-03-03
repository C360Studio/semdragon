package agentsheet

import (
	"context"
	"fmt"
	"math/rand"
	"strings"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
)

// =============================================================================
// NAME GENERATION - Unique character names for agents
// =============================================================================

// NameGenerator creates unique character names for agents.
type NameGenerator struct {
	graph *semdragons.GraphClient
}

// NewNameGenerator creates a new name generator.
func NewNameGenerator(graph *semdragons.GraphClient) *NameGenerator {
	return &NameGenerator{graph: graph}
}

// GenerateName creates a unique character name based on agent skills and traits.
// The name is guaranteed to be unique within the system.
func (g *NameGenerator) GenerateName(ctx context.Context, agent *agentprogression.Agent) (string, error) {
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
	return fmt.Sprintf("%s-%d", base, rand.Intn(9999)), nil //nolint:gosec // Non-cryptographic random for name generation
}

// isNameUnique checks if a display name is already taken.
func (g *NameGenerator) isNameUnique(ctx context.Context, name string) (bool, error) {
	entities, err := g.graph.ListAgentsByPrefix(ctx, 1000)
	if err != nil {
		return false, err
	}

	nameLower := strings.ToLower(name)
	for _, entity := range entities {
		agent := agentprogression.AgentFromEntityState(&entity)
		if agent != nil && strings.ToLower(agent.DisplayName) == nameLower {
			return false, nil
		}
	}
	return true, nil
}

// generateCandidate creates a name candidate based on agent attributes.
func (g *NameGenerator) generateCandidate(agent *agentprogression.Agent) string {
	// Name components inspired by agent's primary skill
	prefixes := g.prefixesForSkills(agent.GetSkillTags())
	suffixes := g.suffixesForTier(agent.Tier)

	prefix := prefixes[rand.Intn(len(prefixes))]   //nolint:gosec // Non-cryptographic random for name generation
	suffix := suffixes[rand.Intn(len(suffixes))]    //nolint:gosec // Non-cryptographic random for name generation

	return prefix + suffix
}

// prefixesForSkills returns name prefixes themed to the agent's skills.
func (g *NameGenerator) prefixesForSkills(skills []domain.SkillTag) []string {
	prefixMap := map[domain.SkillTag][]string{
		domain.SkillCodeGen:       {"Code", "Cipher", "Binary", "Syntax", "Logic", "Script", "Byte"},
		domain.SkillCodeReview:    {"Sharp", "Keen", "Hawk", "Vigil", "Scout", "Watch", "Clear"},
		domain.SkillAnalysis:      {"Sage", "Deep", "Mind", "Think", "Wise", "Know", "Lore"},
		domain.SkillDataTransform: {"Flux", "Morph", "Shift", "Flow", "Change", "Trans", "Meta"},
		domain.SkillResearch:      {"Quest", "Seek", "Lore", "Truth", "Find", "Scholar", "Scribe"},
		domain.SkillPlanning:      {"Forge", "Craft", "Plot", "Chart", "Path", "Vision", "Helm"},
		domain.SkillSummarization: {"Brief", "Core", "Distill", "Clear", "Pure", "Prime", "Key"},
		domain.SkillCustomerComms: {"Voice", "Herald", "Link", "Bridge", "Envoy", "Speaker", "Charm"},
		domain.SkillTraining:      {"Mentor", "Guide", "Elder", "Sage", "Teacher", "Master", "Wise"},
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
func (g *NameGenerator) suffixesForTier(tier domain.TrustTier) []string {
	tierSuffixes := map[domain.TrustTier][]string{
		domain.TierApprentice:  {"spark", "flame", "seed", "wisp", "shade", "drift", "breeze"},
		domain.TierJourneyman:  {"walker", "blade", "weaver", "crafter", "seeker", "runner", "binder"},
		domain.TierExpert:      {"master", "forger", "keeper", "warden", "striker", "shaper", "wright"},
		domain.TierMaster:      {"lord", "sage", "crown", "prime", "arch", "grand", "high"},
		domain.TierGrandmaster: {"eternal", "sovereign", "ancient", "mythic", "legend", "titan", "oracle"},
	}

	if suffixes, ok := tierSuffixes[tier]; ok {
		return suffixes
	}
	return tierSuffixes[domain.TierApprentice]
}

// SetDisplayName sets the agent's display name after validating uniqueness.
func (g *NameGenerator) SetDisplayName(ctx context.Context, agent *agentprogression.Agent, name string) error {
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
func (g *NameGenerator) SuggestNames(ctx context.Context, agent *agentprogression.Agent, count int) ([]string, error) {
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
