package semdragons

import (
	"testing"
	"time"
)

func TestSkillProgressionEngine_ProcessQuestCompletion(t *testing.T) {
	engine := NewSkillProgressionEngine()

	t.Run("improves skills used in quest", func(t *testing.T) {
		agent := &Agent{
			ID:                 "test-agent",
			SkillProficiencies: make(map[SkillTag]SkillProficiency),
		}
		agent.SkillProficiencies[SkillCodeGen] = SkillProficiency{
			Level:    ProficiencyNovice,
			Progress: 0,
		}

		quest := &Quest{
			ID:             "test-quest",
			RequiredSkills: []SkillTag{SkillCodeGen},
			Difficulty:     DifficultyModerate,
		}

		ctx := SkillProgressionContext{
			Agent:   agent,
			Quest:   quest,
			Quality: 0.8,
		}

		results := engine.ProcessQuestCompletion(ctx)

		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		result := results[0]
		if result.Skill != SkillCodeGen {
			t.Errorf("expected skill %s, got %s", SkillCodeGen, result.Skill)
		}
		if result.PointsEarned <= 0 {
			t.Error("expected positive points earned")
		}

		// Check agent was updated
		prof := agent.SkillProficiencies[SkillCodeGen]
		if prof.Progress != result.NewProgress {
			t.Errorf("agent progress %d doesn't match result progress %d", prof.Progress, result.NewProgress)
		}
		if prof.QuestsUsed != 1 {
			t.Errorf("expected quests_used=1, got %d", prof.QuestsUsed)
		}
	})

	t.Run("adds new skill if agent doesnt have it", func(t *testing.T) {
		agent := &Agent{
			ID:                 "test-agent",
			SkillProficiencies: make(map[SkillTag]SkillProficiency),
		}

		quest := &Quest{
			ID:             "test-quest",
			RequiredSkills: []SkillTag{SkillAnalysis},
			Difficulty:     DifficultyEasy,
		}

		ctx := SkillProgressionContext{
			Agent:   agent,
			Quest:   quest,
			Quality: 0.7,
		}

		results := engine.ProcessQuestCompletion(ctx)

		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		// Verify skill was added
		prof, exists := agent.SkillProficiencies[SkillAnalysis]
		if !exists {
			t.Error("expected skill to be added to agent")
		}
		if prof.Level != ProficiencyNovice {
			t.Errorf("expected level %d, got %d", ProficiencyNovice, prof.Level)
		}
	})

	t.Run("levels up skill when progress reaches 100", func(t *testing.T) {
		agent := &Agent{
			ID:                 "test-agent",
			SkillProficiencies: make(map[SkillTag]SkillProficiency),
		}
		// Start close to level up
		agent.SkillProficiencies[SkillCodeGen] = SkillProficiency{
			Level:    ProficiencyNovice,
			Progress: 90,
		}

		quest := &Quest{
			ID:             "test-quest",
			RequiredSkills: []SkillTag{SkillCodeGen},
			Difficulty:     DifficultyHard, // Higher XP
		}

		ctx := SkillProgressionContext{
			Agent:   agent,
			Quest:   quest,
			Quality: 1.0, // Perfect quality
		}

		results := engine.ProcessQuestCompletion(ctx)

		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		result := results[0]
		if !result.LeveledUp {
			t.Error("expected level up")
		}
		if result.NewLevel != ProficiencyApprentice {
			t.Errorf("expected level %d, got %d", ProficiencyApprentice, result.NewLevel)
		}

		// Check agent was updated
		prof := agent.SkillProficiencies[SkillCodeGen]
		if prof.Level != ProficiencyApprentice {
			t.Errorf("agent level %d doesn't match expected %d", prof.Level, ProficiencyApprentice)
		}
	})

	t.Run("caps at master level", func(t *testing.T) {
		agent := &Agent{
			ID:                 "test-agent",
			SkillProficiencies: make(map[SkillTag]SkillProficiency),
		}
		agent.SkillProficiencies[SkillCodeGen] = SkillProficiency{
			Level:    ProficiencyMaster,
			Progress: 0,
		}

		quest := &Quest{
			ID:             "test-quest",
			RequiredSkills: []SkillTag{SkillCodeGen},
			Difficulty:     DifficultyLegendary,
		}

		ctx := SkillProgressionContext{
			Agent:   agent,
			Quest:   quest,
			Quality: 1.0,
		}

		results := engine.ProcessQuestCompletion(ctx)

		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}

		result := results[0]
		if result.LeveledUp {
			t.Error("should not level up at max level")
		}
		if !result.AtMaxLevel {
			t.Error("expected at_max_level=true")
		}
		if result.NewLevel != ProficiencyMaster {
			t.Errorf("expected level %d, got %d", ProficiencyMaster, result.NewLevel)
		}
	})
}

func TestSkillProgressionEngine_DiminishingReturns(t *testing.T) {
	engine := NewSkillProgressionEngine()

	quality := 0.8
	difficulty := DifficultyModerate

	// Calculate points at each level
	pointsAtLevel1 := engine.calculatePoints(ProficiencyNovice, quality, difficulty, false)
	pointsAtLevel3 := engine.calculatePoints(ProficiencyJourneyman, quality, difficulty, false)
	pointsAtLevel5 := engine.calculatePoints(ProficiencyMaster, quality, difficulty, false)

	// Higher levels should earn fewer points
	if pointsAtLevel3 >= pointsAtLevel1 {
		t.Errorf("L3 (%d) should earn less than L1 (%d)", pointsAtLevel3, pointsAtLevel1)
	}
	if pointsAtLevel5 >= pointsAtLevel3 {
		t.Errorf("L5 (%d) should earn less than L3 (%d)", pointsAtLevel5, pointsAtLevel3)
	}

	// Master should earn roughly 41% of Novice (0.8^4 ≈ 0.41)
	ratio := float64(pointsAtLevel5) / float64(pointsAtLevel1)
	expectedRatio := 0.41
	if ratio < expectedRatio-0.1 || ratio > expectedRatio+0.1 {
		t.Errorf("Master/Novice ratio %.2f outside expected range (%.2f ± 0.1)", ratio, expectedRatio)
	}
}

func TestSkillProgressionEngine_MentoredBonus(t *testing.T) {
	engine := NewSkillProgressionEngine()

	quality := 0.8
	difficulty := DifficultyModerate

	pointsUnmentored := engine.calculatePoints(ProficiencyNovice, quality, difficulty, false)
	pointsMentored := engine.calculatePoints(ProficiencyNovice, quality, difficulty, true)

	// Mentored should give 20% bonus
	expectedBonus := 1.2
	actualBonus := float64(pointsMentored) / float64(pointsUnmentored)

	if actualBonus < expectedBonus-0.05 || actualBonus > expectedBonus+0.05 {
		t.Errorf("mentored bonus %.2f outside expected range (%.2f ± 0.05)", actualBonus, expectedBonus)
	}
}

func TestSkillProgressionEngine_CalculateMentorBonus(t *testing.T) {
	engine := NewSkillProgressionEngine()

	t.Run("bonus based on trainee improvement", func(t *testing.T) {
		results := []SkillImprovementResult{
			{
				Skill:        SkillCodeGen,
				PointsEarned: 15,
				LeveledUp:    false,
			},
		}

		bonus := engine.CalculateMentorBonus(results, 100)

		// Bonus = 15 * 0.25 = 3.75 ≈ 4
		if bonus < 3 || bonus > 5 {
			t.Errorf("expected bonus around 4, got %d", bonus)
		}
	})

	t.Run("extra bonus for level up", func(t *testing.T) {
		resultsNoLevelUp := []SkillImprovementResult{
			{PointsEarned: 20, LeveledUp: false},
		}
		resultsWithLevelUp := []SkillImprovementResult{
			{PointsEarned: 20, LeveledUp: true},
		}

		bonusNoLevelUp := engine.CalculateMentorBonus(resultsNoLevelUp, 100)
		bonusWithLevelUp := engine.CalculateMentorBonus(resultsWithLevelUp, 100)

		if bonusWithLevelUp <= bonusNoLevelUp {
			t.Errorf("level up bonus (%d) should exceed no level up bonus (%d)",
				bonusWithLevelUp, bonusNoLevelUp)
		}
	})

	t.Run("caps at 50% of quest XP", func(t *testing.T) {
		results := []SkillImprovementResult{
			{PointsEarned: 500, LeveledUp: true},
			{PointsEarned: 500, LeveledUp: true},
		}

		bonus := engine.CalculateMentorBonus(results, 100)

		// Cap is 50% of 100 = 50
		if bonus > 50 {
			t.Errorf("bonus %d should be capped at 50", bonus)
		}
	})
}

func TestSkillProgressionEngine_EstimateQuestsToLevel(t *testing.T) {
	engine := NewSkillProgressionEngine()

	// Novice to Apprentice
	quests := engine.EstimateQuestsToLevel(ProficiencyNovice, ProficiencyApprentice)
	if quests < 5 || quests > 15 {
		t.Errorf("expected 5-15 quests from Novice to Apprentice, got %d", quests)
	}

	// Novice to Master (accounting for diminishing returns, estimate varies)
	questsToMaster := engine.EstimateQuestsToLevel(ProficiencyNovice, ProficiencyMaster)
	if questsToMaster < 25 {
		t.Errorf("expected at least 25 quests to Master, got %d", questsToMaster)
	}

	// Same level
	noQuests := engine.EstimateQuestsToLevel(ProficiencyJourneyman, ProficiencyJourneyman)
	if noQuests != 0 {
		t.Errorf("expected 0 quests for same level, got %d", noQuests)
	}
}

func TestSkillProficiency_CanLevelUp(t *testing.T) {
	tests := []struct {
		name     string
		prof     SkillProficiency
		expected bool
	}{
		{
			name:     "progress below 100",
			prof:     SkillProficiency{Level: ProficiencyNovice, Progress: 50},
			expected: false,
		},
		{
			name:     "progress at 100",
			prof:     SkillProficiency{Level: ProficiencyNovice, Progress: 100},
			expected: true,
		},
		{
			name:     "at max level",
			prof:     SkillProficiency{Level: ProficiencyMaster, Progress: 100},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.prof.CanLevelUp(); got != tt.expected {
				t.Errorf("CanLevelUp() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestAgent_SkillMethods(t *testing.T) {
	t.Run("HasSkill with proficiencies", func(t *testing.T) {
		agent := &Agent{
			SkillProficiencies: map[SkillTag]SkillProficiency{
				SkillCodeGen: {Level: ProficiencyJourneyman},
			},
		}

		if !agent.HasSkill(SkillCodeGen) {
			t.Error("expected HasSkill(SkillCodeGen) = true")
		}
		if agent.HasSkill(SkillAnalysis) {
			t.Error("expected HasSkill(SkillAnalysis) = false")
		}
	})

	t.Run("HasSkill with legacy skills", func(t *testing.T) {
		agent := &Agent{
			Skills: []SkillTag{SkillCodeGen, SkillCodeReview},
		}

		if !agent.HasSkill(SkillCodeGen) {
			t.Error("expected HasSkill(SkillCodeGen) = true")
		}
		if agent.HasSkill(SkillAnalysis) {
			t.Error("expected HasSkill(SkillAnalysis) = false")
		}
	})

	t.Run("GetProficiency returns zero value for missing skill", func(t *testing.T) {
		agent := &Agent{
			SkillProficiencies: map[SkillTag]SkillProficiency{
				SkillCodeGen: {Level: ProficiencyExpert, Progress: 50},
			},
		}

		prof := agent.GetProficiency(SkillCodeGen)
		if prof.Level != ProficiencyExpert {
			t.Errorf("expected level %d, got %d", ProficiencyExpert, prof.Level)
		}

		missingProf := agent.GetProficiency(SkillAnalysis)
		if missingProf.Level != 0 {
			t.Errorf("expected level 0 for missing skill, got %d", missingProf.Level)
		}
	})

	t.Run("MigrateSkills converts legacy to proficiencies", func(t *testing.T) {
		agent := &Agent{
			Skills: []SkillTag{SkillCodeGen, SkillAnalysis},
		}

		agent.MigrateSkills()

		if len(agent.SkillProficiencies) != 2 {
			t.Errorf("expected 2 proficiencies, got %d", len(agent.SkillProficiencies))
		}

		prof := agent.SkillProficiencies[SkillCodeGen]
		if prof.Level != ProficiencyNovice {
			t.Errorf("migrated skill should be Novice, got %d", prof.Level)
		}
	})

	t.Run("GetSkillTags returns all skills", func(t *testing.T) {
		agent := &Agent{
			SkillProficiencies: map[SkillTag]SkillProficiency{
				SkillCodeGen:    {Level: ProficiencyJourneyman},
				SkillCodeReview: {Level: ProficiencyNovice},
			},
		}

		skills := agent.GetSkillTags()
		if len(skills) != 2 {
			t.Errorf("expected 2 skills, got %d", len(skills))
		}
	})

	t.Run("AddSkill adds new skill at Novice", func(t *testing.T) {
		agent := &Agent{}
		agent.AddSkill(SkillTraining)

		prof, exists := agent.SkillProficiencies[SkillTraining]
		if !exists {
			t.Error("expected skill to be added")
		}
		if prof.Level != ProficiencyNovice {
			t.Errorf("expected Novice level, got %d", prof.Level)
		}
	})
}

func TestSkillProgressionPayload_Validate(t *testing.T) {
	tests := []struct {
		name    string
		payload SkillProgressionPayload
		wantErr bool
	}{
		{
			name: "valid payload",
			payload: SkillProgressionPayload{
				AgentID:   "agent-1",
				QuestID:   "quest-1",
				Timestamp: time.Now(),
			},
			wantErr: false,
		},
		{
			name: "missing agent_id",
			payload: SkillProgressionPayload{
				QuestID:   "quest-1",
				Timestamp: time.Now(),
			},
			wantErr: true,
		},
		{
			name: "missing quest_id",
			payload: SkillProgressionPayload{
				AgentID:   "agent-1",
				Timestamp: time.Now(),
			},
			wantErr: true,
		},
		{
			name: "missing timestamp",
			payload: SkillProgressionPayload{
				AgentID: "agent-1",
				QuestID: "quest-1",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.payload.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
