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
			name:    "valid minimal",
			brief:   &QuestBrief{Title: "Do something"},
			wantErr: false,
		},
		{
			name: "valid with all fields",
			brief: &QuestBrief{
				Title:       "Full brief",
				Description: "A description",
				Acceptance:  []string{"test passes"},
				Skills:      []SkillTag{SkillCodeGen},
			},
			wantErr: false,
		},
		{
			name: "valid difficulty",
			brief: &QuestBrief{
				Title:      "With difficulty",
				Difficulty: ptrDifficulty(DifficultyEpic),
			},
			wantErr: false,
		},
		{
			name: "difficulty too high",
			brief: &QuestBrief{
				Title:      "Too hard",
				Difficulty: ptrDifficulty(QuestDifficulty(99)),
			},
			wantErr: true,
		},
		{
			name: "difficulty too low",
			brief: &QuestBrief{
				Title:      "Negative",
				Difficulty: ptrDifficulty(QuestDifficulty(-1)),
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
