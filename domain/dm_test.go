package domain

import (
	"testing"
)

func ptrDifficulty(d QuestDifficulty) *QuestDifficulty { return &d }

func TestValidateQuestBrief(t *testing.T) {
	tests := []struct {
		name    string
		brief   *QuestBrief
		wantErr bool
	}{
		{
			name:    "nil brief",
			brief:   nil,
			wantErr: true,
		},
		{
			name:    "empty title",
			brief:   &QuestBrief{Title: ""},
			wantErr: true,
		},
		{
			name:    "missing goal",
			brief:   &QuestBrief{Title: "Do something"},
			wantErr: true,
		},
		{
			name:    "valid minimal",
			brief:   &QuestBrief{Title: "Do something", Goal: "Achieve a thing"},
			wantErr: false,
		},
		{
			name: "valid with all fields",
			brief: &QuestBrief{
				Title:        "Full brief",
				Goal:         "Achieve something meaningful",
				Requirements: []string{"test passes"},
				Skills:       []SkillTag{SkillCodeGen},
			},
			wantErr: false,
		},
		{
			name: "valid difficulty",
			brief: &QuestBrief{
				Title:      "With difficulty",
				Goal:       "Test difficulty",
				Difficulty: ptrDifficulty(DifficultyEpic),
			},
			wantErr: false,
		},
		{
			name: "difficulty too high",
			brief: &QuestBrief{
				Title:      "Too hard",
				Goal:       "Something",
				Difficulty: ptrDifficulty(QuestDifficulty(99)),
			},
			wantErr: true,
		},
		{
			name: "difficulty too low",
			brief: &QuestBrief{
				Title:      "Negative",
				Goal:       "Something",
				Difficulty: ptrDifficulty(QuestDifficulty(-1)),
			},
			wantErr: true,
		},
		{
			name: "valid with scenarios",
			brief: &QuestBrief{
				Title: "Multi-step quest",
				Goal:  "Complete all steps",
				Scenarios: []QuestScenario{
					{Name: "setup", Description: "Prepare the environment"},
					{Name: "execute", Description: "Run the task", DependsOn: []string{"setup"}},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid scenario dependencies",
			brief: &QuestBrief{
				Title: "Bad scenario deps",
				Goal:  "Something",
				Scenarios: []QuestScenario{
					{Name: "alpha", Description: "first", DependsOn: []string{"omega"}},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateQuestBrief(tt.brief)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateQuestBrief() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateQuestChainBrief(t *testing.T) {
	tests := []struct {
		name    string
		chain   *QuestChainBrief
		wantErr bool
	}{
		{
			name:    "nil chain",
			chain:   nil,
			wantErr: true,
		},
		{
			name:    "empty quests",
			chain:   &QuestChainBrief{Quests: []QuestChainEntry{}},
			wantErr: true,
		},
		{
			name: "quest with empty title",
			chain: &QuestChainBrief{
				Quests: []QuestChainEntry{
					{Title: ""},
				},
			},
			wantErr: true,
		},
		{
			name: "valid single quest",
			chain: &QuestChainBrief{
				Quests: []QuestChainEntry{
					{Title: "First quest"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid chain with dependencies",
			chain: &QuestChainBrief{
				Quests: []QuestChainEntry{
					{Title: "Setup"},
					{Title: "Build", DependsOn: []int{0}},
					{Title: "Test", DependsOn: []int{1}},
				},
			},
			wantErr: false,
		},
		{
			name: "self reference",
			chain: &QuestChainBrief{
				Quests: []QuestChainEntry{
					{Title: "Self ref", DependsOn: []int{0}},
				},
			},
			wantErr: true,
		},
		{
			name: "out of range index",
			chain: &QuestChainBrief{
				Quests: []QuestChainEntry{
					{Title: "Only one", DependsOn: []int{5}},
				},
			},
			wantErr: true,
		},
		{
			name: "negative index",
			chain: &QuestChainBrief{
				Quests: []QuestChainEntry{
					{Title: "Negative", DependsOn: []int{-1}},
				},
			},
			wantErr: true,
		},
		{
			name: "cycle detection",
			chain: &QuestChainBrief{
				Quests: []QuestChainEntry{
					{Title: "A", DependsOn: []int{1}},
					{Title: "B", DependsOn: []int{0}},
				},
			},
			wantErr: true,
		},
		{
			name: "three-node cycle",
			chain: &QuestChainBrief{
				Quests: []QuestChainEntry{
					{Title: "A", DependsOn: []int{2}},
					{Title: "B", DependsOn: []int{0}},
					{Title: "C", DependsOn: []int{1}},
				},
			},
			wantErr: true,
		},
		{
			name: "diamond dependency (no cycle)",
			chain: &QuestChainBrief{
				Quests: []QuestChainEntry{
					{Title: "Start"},
					{Title: "Left", DependsOn: []int{0}},
					{Title: "Right", DependsOn: []int{0}},
					{Title: "Join", DependsOn: []int{1, 2}},
				},
			},
			wantErr: false,
		},
		{
			name: "duplicate dependency index",
			chain: &QuestChainBrief{
				Quests: []QuestChainEntry{
					{Title: "A"},
					{Title: "B", DependsOn: []int{0, 0}},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid difficulty in chain entry",
			chain: &QuestChainBrief{
				Quests: []QuestChainEntry{
					{Title: "Bad difficulty", Difficulty: ptrDifficulty(QuestDifficulty(99))},
				},
			},
			wantErr: true,
		},
		{
			name: "chain entry with invalid scenario dependencies",
			chain: &QuestChainBrief{
				Quests: []QuestChainEntry{
					{
						Title: "Quest with bad scenarios",
						Scenarios: []QuestScenario{
							{Name: "a", Description: "one", DependsOn: []string{"b"}},
							{Name: "b", Description: "two", DependsOn: []string{"a"}},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "chain entry with valid scenarios",
			chain: &QuestChainBrief{
				Quests: []QuestChainEntry{
					{
						Title: "Quest with scenarios",
						Scenarios: []QuestScenario{
							{Name: "setup", Description: "prepare"},
							{Name: "run", Description: "execute", DependsOn: []string{"setup"}},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "exceeds max chain size",
			chain: func() *QuestChainBrief {
				entries := make([]QuestChainEntry, maxChainSize+1)
				for i := range entries {
					entries[i].Title = "Quest"
				}
				return &QuestChainBrief{Quests: entries}
			}(),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateQuestChainBrief(tt.chain)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateQuestChainBrief() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestClassifyDecomposability(t *testing.T) {
	tests := []struct {
		name  string
		brief *QuestBrief
		want  DecomposabilityClass
	}{
		{
			name:  "nil brief",
			brief: nil,
			want:  DecomposableTrivial,
		},
		{
			name:  "empty scenarios",
			brief: &QuestBrief{Title: "t"},
			want:  DecomposableTrivial,
		},
		{
			name: "single scenario",
			brief: &QuestBrief{
				Title: "t",
				Scenarios: []QuestScenario{
					{Name: "only", Description: "the one"},
				},
			},
			want: DecomposableTrivial,
		},
		{
			name: "multiple independent scenarios",
			brief: &QuestBrief{
				Title: "t",
				Scenarios: []QuestScenario{
					{Name: "a", Description: "step a"},
					{Name: "b", Description: "step b"},
					{Name: "c", Description: "step c"},
				},
			},
			want: DecomposableParallel,
		},
		{
			name: "linear chain A then B then C",
			brief: &QuestBrief{
				Title: "t",
				Scenarios: []QuestScenario{
					{Name: "a", Description: "start"},
					{Name: "b", Description: "middle", DependsOn: []string{"a"}},
					{Name: "c", Description: "end", DependsOn: []string{"b"}},
				},
			},
			want: DecomposableSequential,
		},
		{
			name: "mixed: A independent, B depends on C",
			brief: &QuestBrief{
				Title: "t",
				Scenarios: []QuestScenario{
					{Name: "a", Description: "independent"},
					{Name: "c", Description: "base"},
					{Name: "b", Description: "depends on c", DependsOn: []string{"c"}},
				},
			},
			want: DecomposableMixed,
		},
		{
			name: "two roots with some deps is mixed not sequential",
			brief: &QuestBrief{
				Title: "t",
				Scenarios: []QuestScenario{
					{Name: "root1", Description: "first root"},
					{Name: "root2", Description: "second root"},
					{Name: "leaf", Description: "depends on root1", DependsOn: []string{"root1"}},
				},
			},
			want: DecomposableMixed,
		},
		{
			name: "linear chain declared out of order is still sequential",
			brief: &QuestBrief{
				Title: "t",
				Scenarios: []QuestScenario{
					{Name: "b", Description: "middle", DependsOn: []string{"a"}},
					{Name: "a", Description: "start"},
					{Name: "c", Description: "end", DependsOn: []string{"b"}},
				},
			},
			want: DecomposableSequential,
		},
		{
			name: "diamond shape is mixed",
			brief: &QuestBrief{
				Title: "t",
				Scenarios: []QuestScenario{
					{Name: "start", Description: "root"},
					{Name: "left", Description: "left branch", DependsOn: []string{"start"}},
					{Name: "right", Description: "right branch", DependsOn: []string{"start"}},
					{Name: "join", Description: "join", DependsOn: []string{"left", "right"}},
				},
			},
			want: DecomposableMixed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyDecomposability(tt.brief)
			if got != tt.want {
				t.Errorf("ClassifyDecomposability() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateScenarioDependencies(t *testing.T) {
	tests := []struct {
		name      string
		scenarios []QuestScenario
		wantErr   bool
	}{
		{
			name:      "nil scenarios",
			scenarios: nil,
			wantErr:   false,
		},
		{
			name:      "empty scenarios",
			scenarios: []QuestScenario{},
			wantErr:   false,
		},
		{
			name: "empty description",
			scenarios: []QuestScenario{
				{Name: "alpha", Description: ""},
			},
			wantErr: true,
		},
		{
			name: "duplicate depends_on within scenario",
			scenarios: []QuestScenario{
				{Name: "a", Description: "first"},
				{Name: "b", Description: "second", DependsOn: []string{"a", "a"}},
			},
			wantErr: true,
		},
		{
			name: "valid independent scenarios",
			scenarios: []QuestScenario{
				{Name: "alpha", Description: "first"},
				{Name: "beta", Description: "second"},
			},
			wantErr: false,
		},
		{
			name: "valid chain",
			scenarios: []QuestScenario{
				{Name: "setup", Description: "prepare"},
				{Name: "build", Description: "compile", DependsOn: []string{"setup"}},
				{Name: "test", Description: "verify", DependsOn: []string{"build"}},
			},
			wantErr: false,
		},
		{
			name: "empty name",
			scenarios: []QuestScenario{
				{Name: "", Description: "nameless"},
			},
			wantErr: true,
		},
		{
			name: "duplicate name",
			scenarios: []QuestScenario{
				{Name: "alpha", Description: "first"},
				{Name: "alpha", Description: "duplicate"},
			},
			wantErr: true,
		},
		{
			name: "unknown depends_on reference",
			scenarios: []QuestScenario{
				{Name: "alpha", Description: "first", DependsOn: []string{"ghost"}},
			},
			wantErr: true,
		},
		{
			name: "self-reference",
			scenarios: []QuestScenario{
				{Name: "alpha", Description: "self-loop", DependsOn: []string{"alpha"}},
			},
			wantErr: true,
		},
		{
			name: "two-node cycle A depends on B, B depends on A",
			scenarios: []QuestScenario{
				{Name: "a", Description: "first", DependsOn: []string{"b"}},
				{Name: "b", Description: "second", DependsOn: []string{"a"}},
			},
			wantErr: true,
		},
		{
			name: "three-node cycle",
			scenarios: []QuestScenario{
				{Name: "a", Description: "one", DependsOn: []string{"c"}},
				{Name: "b", Description: "two", DependsOn: []string{"a"}},
				{Name: "c", Description: "three", DependsOn: []string{"b"}},
			},
			wantErr: true,
		},
		{
			name: "valid diamond (no cycle)",
			scenarios: []QuestScenario{
				{Name: "root", Description: "start"},
				{Name: "left", Description: "branch left", DependsOn: []string{"root"}},
				{Name: "right", Description: "branch right", DependsOn: []string{"root"}},
				{Name: "join", Description: "merge", DependsOn: []string{"left", "right"}},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateScenarioDependencies(tt.scenarios)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateScenarioDependencies() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
