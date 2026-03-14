package domain

import (
	"testing"
)

func skillSet(skills ...SkillTag) AgentSkillSet {
	m := make(map[SkillTag]struct{}, len(skills))
	for _, s := range skills {
		m[s] = struct{}{}
	}
	return AgentSkillSet{Skills: m}
}

func TestClassifySkillCoverage(t *testing.T) {
	tests := []struct {
		name            string
		required        []SkillTag
		roster          []AgentSkillSet
		wantCanSolo     bool
		wantMinAgents   int
		wantUncovered   int // len(UncoveredSkills)
	}{
		{
			name:          "no required skills is trivially soloable",
			required:      nil,
			roster:        []AgentSkillSet{skillSet(SkillCodeGen)},
			wantCanSolo:   true,
			wantMinAgents: 1,
		},
		{
			name:          "empty roster with requirements",
			required:      []SkillTag{SkillCodeGen},
			roster:        nil,
			wantCanSolo:   false,
			wantMinAgents: 0,
			wantUncovered: 1,
		},
		{
			name:          "single agent covers single skill",
			required:      []SkillTag{SkillCodeGen},
			roster:        []AgentSkillSet{skillSet(SkillCodeGen, SkillAnalysis)},
			wantCanSolo:   true,
			wantMinAgents: 1,
		},
		{
			name:     "single agent covers all skills",
			required: []SkillTag{SkillCodeGen, SkillAnalysis, SkillPlanning},
			roster: []AgentSkillSet{
				skillSet(SkillCodeGen, SkillAnalysis, SkillPlanning, SkillResearch),
			},
			wantCanSolo:   true,
			wantMinAgents: 1,
		},
		{
			name:     "no single agent covers all — two needed",
			required: []SkillTag{SkillCodeGen, SkillResearch},
			roster: []AgentSkillSet{
				skillSet(SkillCodeGen, SkillAnalysis),
				skillSet(SkillResearch, SkillSummarization),
			},
			wantCanSolo:   false,
			wantMinAgents: 2,
		},
		{
			name:     "three agents needed for full coverage",
			required: []SkillTag{SkillCodeGen, SkillResearch, SkillCustomerComms},
			roster: []AgentSkillSet{
				skillSet(SkillCodeGen),
				skillSet(SkillResearch),
				skillSet(SkillCustomerComms),
			},
			wantCanSolo:   false,
			wantMinAgents: 3,
		},
		{
			name:     "greedy picks optimal — one agent covers two, another covers one",
			required: []SkillTag{SkillCodeGen, SkillAnalysis, SkillResearch},
			roster: []AgentSkillSet{
				skillSet(SkillCodeGen, SkillAnalysis),
				skillSet(SkillResearch),
				skillSet(SkillCodeGen), // redundant
			},
			wantCanSolo:   false,
			wantMinAgents: 2,
		},
		{
			name:          "uncoverable skill — no agent has it",
			required:      []SkillTag{SkillCodeGen, SkillTraining},
			roster:        []AgentSkillSet{skillSet(SkillCodeGen, SkillAnalysis)},
			wantCanSolo:   false,
			wantMinAgents: 0,
			wantUncovered: 1,
		},
		{
			name:     "partial coverage with uncoverable",
			required: []SkillTag{SkillCodeGen, SkillResearch, SkillTraining},
			roster: []AgentSkillSet{
				skillSet(SkillCodeGen),
				skillSet(SkillResearch),
			},
			wantCanSolo:   false,
			wantMinAgents: 0,
			wantUncovered: 1,
		},
		{
			name:     "one agent can solo among many",
			required: []SkillTag{SkillAnalysis, SkillResearch},
			roster: []AgentSkillSet{
				skillSet(SkillCodeGen),
				skillSet(SkillAnalysis, SkillResearch, SkillSummarization),
				skillSet(SkillResearch),
			},
			wantCanSolo:   true,
			wantMinAgents: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifySkillCoverage(tt.required, tt.roster)
			if result.CanSolo != tt.wantCanSolo {
				t.Errorf("CanSolo = %v, want %v", result.CanSolo, tt.wantCanSolo)
			}
			if result.MinAgents != tt.wantMinAgents {
				t.Errorf("MinAgents = %d, want %d", result.MinAgents, tt.wantMinAgents)
			}
			if len(result.UncoveredSkills) != tt.wantUncovered {
				t.Errorf("UncoveredSkills = %v (len %d), want len %d",
					result.UncoveredSkills, len(result.UncoveredSkills), tt.wantUncovered)
			}
		})
	}
}
