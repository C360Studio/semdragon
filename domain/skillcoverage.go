package domain

// AgentSkillSet is a lightweight representation of one agent's skills,
// used by ClassifySkillCoverage without importing processor packages.
type AgentSkillSet struct {
	Skills map[SkillTag]struct{}
}

// SkillCoverageResult describes whether a set of required skills can be
// handled by a single agent (solo) or requires multiple agents (party).
type SkillCoverageResult struct {
	// CanSolo is true if at least one agent in the roster has all required skills.
	CanSolo bool
	// MinAgents is the greedy set-cover estimate of how many agents are needed
	// to collectively cover all required skills. 0 if some skills are uncoverable.
	MinAgents int
	// UncoveredSkills lists required skills that no agent in the roster has.
	UncoveredSkills []SkillTag
}

// ClassifySkillCoverage checks whether any single agent in the roster can
// handle all requiredSkills (AND logic). If not, it computes the minimum
// number of agents needed via greedy set cover.
func ClassifySkillCoverage(requiredSkills []SkillTag, roster []AgentSkillSet) SkillCoverageResult {
	if len(requiredSkills) == 0 {
		return SkillCoverageResult{CanSolo: true, MinAgents: 1}
	}

	// Build the set of required skills and check which are coverable at all.
	required := make(map[SkillTag]struct{}, len(requiredSkills))
	for _, s := range requiredSkills {
		required[s] = struct{}{}
	}

	// Check for solo coverage and build per-agent coverage sets.
	type agentCoverage struct {
		covered map[SkillTag]struct{}
	}
	coverages := make([]agentCoverage, 0, len(roster))

	for _, agent := range roster {
		matchCount := 0
		covered := make(map[SkillTag]struct{})
		for skill := range required {
			if _, ok := agent.Skills[skill]; ok {
				covered[skill] = struct{}{}
				matchCount++
			}
		}
		if matchCount == len(required) {
			return SkillCoverageResult{CanSolo: true, MinAgents: 1}
		}
		if matchCount > 0 {
			coverages = append(coverages, agentCoverage{covered: covered})
		}
	}

	// Check for uncoverable skills (no agent has them at all).
	coverable := make(map[SkillTag]struct{})
	for _, ac := range coverages {
		for skill := range ac.covered {
			coverable[skill] = struct{}{}
		}
	}
	var uncovered []SkillTag
	for skill := range required {
		if _, ok := coverable[skill]; !ok {
			uncovered = append(uncovered, skill)
		}
	}
	if len(uncovered) > 0 {
		return SkillCoverageResult{
			CanSolo:         false,
			MinAgents:       0,
			UncoveredSkills: uncovered,
		}
	}

	// Greedy set cover: repeatedly pick the agent covering the most uncovered skills.
	remaining := make(map[SkillTag]struct{}, len(required))
	for skill := range required {
		remaining[skill] = struct{}{}
	}
	agents := 0

	for len(remaining) > 0 {
		bestIdx := -1
		bestCount := 0
		for i, ac := range coverages {
			count := 0
			for skill := range ac.covered {
				if _, ok := remaining[skill]; ok {
					count++
				}
			}
			if count > bestCount {
				bestCount = count
				bestIdx = i
			}
		}
		if bestIdx < 0 || bestCount == 0 {
			break
		}
		agents++
		for skill := range coverages[bestIdx].covered {
			delete(remaining, skill)
		}
	}

	return SkillCoverageResult{
		CanSolo:   false,
		MinAgents: agents,
	}
}
