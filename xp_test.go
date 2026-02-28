package semdragons

import (
	"testing"
	"time"
)

func TestCalculateXP_GuildBonusByRank(t *testing.T) {
	engine := NewDefaultXPEngine()

	baseQuest := Quest{
		ID:     "quest-1",
		Title:  "Test Quest",
		BaseXP: 100,
	}

	baseAgent := Agent{
		ID:    "agent-1",
		Name:  "TestAgent",
		Level: 5,
	}

	verdict := BattleVerdict{
		Passed:       true,
		QualityScore: 0.5, // Neutral quality to isolate guild bonus
	}

	tests := []struct {
		name           string
		rank           GuildRank
		isGuildQuest   bool
		expectedBonus  int64
		expectedRate   float64
	}{
		{
			name:          "non-guild quest gets no bonus",
			rank:          GuildRankMaster,
			isGuildQuest:  false,
			expectedBonus: 0,
			expectedRate:  0.0,
		},
		{
			name:          "initiate gets 10%",
			rank:          GuildRankInitiate,
			isGuildQuest:  true,
			expectedBonus: 10,
			expectedRate:  0.10,
		},
		{
			name:          "member gets 15%",
			rank:          GuildRankMember,
			isGuildQuest:  true,
			expectedBonus: 15,
			expectedRate:  0.15,
		},
		{
			name:          "veteran gets 18%",
			rank:          GuildRankVeteran,
			isGuildQuest:  true,
			expectedBonus: 18,
			expectedRate:  0.18,
		},
		{
			name:          "officer gets 20%",
			rank:          GuildRankOfficer,
			isGuildQuest:  true,
			expectedBonus: 20,
			expectedRate:  0.20,
		},
		{
			name:          "guildmaster gets 25%",
			rank:          GuildRankMaster,
			isGuildQuest:  true,
			expectedBonus: 25,
			expectedRate:  0.25,
		},
		{
			name:          "empty rank gets initiate rate",
			rank:          "",
			isGuildQuest:  true,
			expectedBonus: 10,
			expectedRate:  0.10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := XPContext{
				Quest:        baseQuest,
				Agent:        baseAgent,
				BattleResult: verdict,
				Duration:     5 * time.Minute,
				EstDuration:  5 * time.Minute, // Same as actual = no speed bonus
				Streak:       0,               // No streak bonus
				IsGuildQuest: tt.isGuildQuest,
				GuildRank:    tt.rank,
				Attempt:      1,
			}

			award := engine.CalculateXP(ctx)

			if award.GuildBonus != tt.expectedBonus {
				t.Errorf("expected guild bonus %d, got %d", tt.expectedBonus, award.GuildBonus)
			}

			// Verify the rate method matches expected
			actualRate := tt.rank.GuildBonusRate()
			if tt.isGuildQuest && actualRate != tt.expectedRate {
				t.Errorf("expected rate %.2f, got %.2f", tt.expectedRate, actualRate)
			}
		})
	}
}

func TestGuildRank_GuildBonusRate(t *testing.T) {
	tests := []struct {
		rank         GuildRank
		expectedRate float64
	}{
		{GuildRankInitiate, 0.10},
		{GuildRankMember, 0.15},
		{GuildRankVeteran, 0.18},
		{GuildRankOfficer, 0.20},
		{GuildRankMaster, 0.25},
		{"unknown", 0.10},    // Unknown defaults to initiate
		{"", 0.10},           // Empty defaults to initiate
	}

	for _, tt := range tests {
		t.Run(string(tt.rank), func(t *testing.T) {
			rate := tt.rank.GuildBonusRate()
			if rate != tt.expectedRate {
				t.Errorf("GuildBonusRate() = %.2f, want %.2f", rate, tt.expectedRate)
			}
		})
	}
}

func TestCalculateXP_GuildmasterVsInitiate(t *testing.T) {
	engine := NewDefaultXPEngine()

	quest := Quest{
		ID:     "quest-1",
		Title:  "Guild Quest",
		BaseXP: 1000, // Large base to make difference clear
	}

	agent := Agent{
		ID:    "agent-1",
		Name:  "TestAgent",
		Level: 10,
	}

	verdict := BattleVerdict{
		Passed:       true,
		QualityScore: 0.8,
	}

	// Guildmaster context
	gmCtx := XPContext{
		Quest:        quest,
		Agent:        agent,
		BattleResult: verdict,
		Duration:     5 * time.Minute,
		EstDuration:  5 * time.Minute,
		Streak:       0,
		IsGuildQuest: true,
		GuildRank:    GuildRankMaster,
		Attempt:      1,
	}

	// Initiate context
	initCtx := XPContext{
		Quest:        quest,
		Agent:        agent,
		BattleResult: verdict,
		Duration:     5 * time.Minute,
		EstDuration:  5 * time.Minute,
		Streak:       0,
		IsGuildQuest: true,
		GuildRank:    GuildRankInitiate,
		Attempt:      1,
	}

	gmAward := engine.CalculateXP(gmCtx)
	initAward := engine.CalculateXP(initCtx)

	// Guildmaster should get 25% bonus = 250 XP
	// Initiate should get 10% bonus = 100 XP
	expectedGMBonus := int64(250)
	expectedInitBonus := int64(100)

	if gmAward.GuildBonus != expectedGMBonus {
		t.Errorf("guildmaster bonus: expected %d, got %d", expectedGMBonus, gmAward.GuildBonus)
	}

	if initAward.GuildBonus != expectedInitBonus {
		t.Errorf("initiate bonus: expected %d, got %d", expectedInitBonus, initAward.GuildBonus)
	}

	// Guildmaster should earn 150 more XP from guild bonus alone
	bonusDiff := gmAward.GuildBonus - initAward.GuildBonus
	if bonusDiff != 150 {
		t.Errorf("expected 150 XP difference in guild bonus, got %d", bonusDiff)
	}

	// Total XP should also reflect the difference
	totalDiff := gmAward.TotalXP - initAward.TotalXP
	if totalDiff != 150 {
		t.Errorf("expected 150 XP difference in total, got %d", totalDiff)
	}
}
