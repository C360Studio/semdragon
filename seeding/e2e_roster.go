package seeding

import (
	"github.com/c360studio/semdragons"
)

// E2ETestRoster returns a roster for Playwright e2e testing.
// Covers all tiers, multiple guilds, and diverse skill sets.
// Uses ModeTieredRoster for instant agent creation at target levels.
func E2ETestRoster(config semdragons.AgentConfig) *RosterConfig {
	return &RosterConfig{
		Name:        "e2e-test-roster",
		Description: "Consistent test data for Playwright e2e tests",
		Guilds: []GuildSpec{
			{
				ID:          "guild-alpha",
				Name:        "Alpha Guild",
				Description: "Primary test guild for code generation and analysis",
				Culture:     "Methodical and thorough",
				MinLevel:    1,
			},
			{
				ID:          "guild-beta",
				Name:        "Beta Guild",
				Description: "Secondary test guild for research and review",
				Culture:     "Creative and exploratory",
				MinLevel:    6,
			},
		},
		Agents: []AgentSpec{
			// Apprentice tier (levels 1-5) - 3 agents in Alpha Guild
			{
				NamePattern: "apprentice-{n}",
				Count:       3,
				Level:       3,
				Skills:      []semdragons.SkillTag{semdragons.SkillAnalysis},
				Config:      config,
				GuildID:     "guild-alpha",
			},
			// Journeyman tier (levels 6-10) - 2 agents in Alpha Guild
			{
				NamePattern: "journeyman-{n}",
				Count:       2,
				Level:       8,
				Skills:      []semdragons.SkillTag{semdragons.SkillCodeGen, semdragons.SkillDataTransform},
				Config:      config,
				GuildID:     "guild-alpha",
			},
			// Expert tier (levels 11-15) - 2 agents in Beta Guild
			{
				NamePattern: "expert-{n}",
				Count:       2,
				Level:       12,
				Skills:      []semdragons.SkillTag{semdragons.SkillCodeGen, semdragons.SkillCodeReview, semdragons.SkillPlanning},
				Config:      config,
				GuildID:     "guild-beta",
			},
			// Master tier (levels 16-18) - 1 agent in Beta Guild
			{
				NamePattern: "master-{n}",
				Count:       1,
				Level:       17,
				Skills:      []semdragons.SkillTag{semdragons.SkillPlanning, semdragons.SkillTraining, semdragons.SkillCodeReview},
				Config:      config,
				GuildID:     "guild-beta",
			},
			// Grandmaster tier (levels 19-20) - 1 agent (no guild - can manage all)
			{
				NamePattern: "grandmaster-{n}",
				Count:       1,
				Level:       20,
				Skills:      []semdragons.SkillTag{semdragons.SkillPlanning, semdragons.SkillTraining, semdragons.SkillCodeReview, semdragons.SkillCodeGen},
				Config:      config,
			},
			// Freelancer (no guild) - for testing guild recruitment
			{
				NamePattern: "freelancer-{n}",
				Count:       1,
				Level:       5,
				Skills:      []semdragons.SkillTag{semdragons.SkillAnalysis, semdragons.SkillResearch},
				Config:      config,
			},
		},
	}
}

// NewE2ETestConfig creates a seeding Config configured for E2E testing.
// Uses ModeTieredRoster with idempotent seeding to support re-runs.
func NewE2ETestConfig(agentConfig semdragons.AgentConfig) *Config {
	return &Config{
		Mode:       ModeTieredRoster,
		DryRun:     false,
		Idempotent: true, // Safe to re-run without duplicating agents
		Roster:     E2ETestRoster(agentConfig),
	}
}
