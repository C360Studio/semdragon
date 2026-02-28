package semdragons

import (
	"context"
	"sort"
)

// =============================================================================
// CHARACTER SHEET SERVICE - Agent profile projections
// =============================================================================
// The character sheet provides a comprehensive view of an agent's current state,
// skills, equipment, and derived statistics for UI display and decision-making.
// =============================================================================

// CharacterSheet is a complete projection of an agent's state.
type CharacterSheet struct {
	Agent        Agent             `json:"agent"`
	SkillBars    []SkillBar        `json:"skill_bars"`
	DerivedStats DerivedStats      `json:"derived_stats"`
	Memberships  []GuildMembership `json:"memberships"`
	Equipment    []EquippedItem    `json:"equipment"`
	Inventory    *AgentInventory   `json:"inventory,omitempty"`
}

// SkillBar represents a skill with its proficiency for UI display.
type SkillBar struct {
	Skill           SkillTag         `json:"skill"`
	SkillName       string           `json:"skill_name"`
	Level           ProficiencyLevel `json:"level"`
	LevelName       string           `json:"level_name"`
	Progress        int              `json:"progress"` // 0-99
	ProgressPercent float64          `json:"progress_percent"`
	TotalXP         int64            `json:"total_xp"`
	QuestsUsed      int              `json:"quests_used"`
	IsMaxLevel      bool             `json:"is_max_level"`
}

// DerivedStats contains computed statistics about an agent.
type DerivedStats struct {
	AvgProficiency   float64  `json:"avg_proficiency"`
	StrongestSkill   SkillTag `json:"strongest_skill,omitempty"`
	WeakestSkill     SkillTag `json:"weakest_skill,omitempty"`
	QuestSuccessRate float64  `json:"quest_success_rate"`
	BattleWinRate    float64  `json:"battle_win_rate"`
	TotalSkills      int      `json:"total_skills"`
	MasterSkills     int      `json:"master_skills"` // Skills at max level
	XPEfficiency     float64  `json:"xp_efficiency"` // XP earned per quest
}

// GuildMembership represents an agent's status in a guild.
type GuildMembership struct {
	GuildID      GuildID   `json:"guild_id"`
	GuildName    string    `json:"guild_name"`
	Rank         GuildRank `json:"rank"`
	Contribution float64   `json:"contribution"` // XP contributed via guild quests
	JoinedAt     string    `json:"joined_at"`
}

// EquippedItem represents a tool the agent has equipped.
type EquippedItem struct {
	ToolID      string `json:"tool_id"`
	ToolName    string `json:"tool_name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	IsDangerous bool   `json:"is_dangerous"`
	IsRental    bool   `json:"is_rental"`
	UsesLeft    *int   `json:"uses_left,omitempty"` // For rentals
}

// CharacterSheetService creates character sheet projections.
type CharacterSheetService struct {
	storage *Storage
}

// NewCharacterSheetService creates a new character sheet service.
func NewCharacterSheetService(storage *Storage) *CharacterSheetService {
	return &CharacterSheetService{storage: storage}
}

// GetCharacterSheet builds a complete character sheet for an agent.
func (s *CharacterSheetService) GetCharacterSheet(ctx context.Context, agentID AgentID) (*CharacterSheet, error) {
	agentInstance := ExtractInstance(string(agentID))

	agent, err := s.storage.GetAgent(ctx, agentInstance)
	if err != nil {
		return nil, err
	}

	// Build skill bars
	skillBars := s.buildSkillBars(agent)

	// Calculate derived stats
	derivedStats := s.calculateDerivedStats(agent, skillBars)

	// Get guild memberships
	memberships := s.getGuildMemberships(ctx, agent)

	// Build equipment list
	equipment := s.buildEquipmentList(agent)

	// Inventory is populated separately via store service if available
	var inventory *AgentInventory

	return &CharacterSheet{
		Agent:        *agent,
		SkillBars:    skillBars,
		DerivedStats: derivedStats,
		Memberships:  memberships,
		Equipment:    equipment,
		Inventory:    inventory,
	}, nil
}

// buildSkillBars creates skill bar data from agent proficiencies.
func (s *CharacterSheetService) buildSkillBars(agent *Agent) []SkillBar {
	if agent.SkillProficiencies == nil || len(agent.SkillProficiencies) == 0 {
		// Fall back to legacy skills
		bars := make([]SkillBar, 0, len(agent.Skills))
		for _, skill := range agent.Skills {
			bars = append(bars, SkillBar{
				Skill:           skill,
				SkillName:       skillTagToName(skill),
				Level:           ProficiencyNovice,
				LevelName:       ProficiencyLevelNames[ProficiencyNovice],
				Progress:        0,
				ProgressPercent: 0,
				TotalXP:         0,
				QuestsUsed:      0,
				IsMaxLevel:      false,
			})
		}
		return bars
	}

	bars := make([]SkillBar, 0, len(agent.SkillProficiencies))
	for skill, prof := range agent.SkillProficiencies {
		bars = append(bars, SkillBar{
			Skill:           skill,
			SkillName:       skillTagToName(skill),
			Level:           prof.Level,
			LevelName:       ProficiencyLevelNames[prof.Level],
			Progress:        prof.Progress,
			ProgressPercent: prof.ProgressPercent(),
			TotalXP:         prof.TotalXP,
			QuestsUsed:      prof.QuestsUsed,
			IsMaxLevel:      prof.Level >= ProficiencyMaster,
		})
	}

	// Sort by level (desc), then by name (asc)
	sort.Slice(bars, func(i, j int) bool {
		if bars[i].Level != bars[j].Level {
			return bars[i].Level > bars[j].Level
		}
		return bars[i].SkillName < bars[j].SkillName
	})

	return bars
}

// calculateDerivedStats computes aggregate statistics.
func (s *CharacterSheetService) calculateDerivedStats(agent *Agent, skillBars []SkillBar) DerivedStats {
	stats := DerivedStats{
		TotalSkills: len(skillBars),
	}

	if len(skillBars) == 0 {
		return stats
	}

	// Calculate average proficiency and find strongest/weakest
	var totalLevel float64
	var strongest, weakest SkillBar
	strongest.Level = ProficiencyNovice
	weakest.Level = ProficiencyMaster

	for _, bar := range skillBars {
		totalLevel += float64(bar.Level)
		if bar.Level > strongest.Level {
			strongest = bar
		}
		if bar.Level < weakest.Level {
			weakest = bar
		}
		if bar.IsMaxLevel {
			stats.MasterSkills++
		}
	}

	stats.AvgProficiency = totalLevel / float64(len(skillBars))
	stats.StrongestSkill = strongest.Skill
	stats.WeakestSkill = weakest.Skill

	// Calculate success rates from agent stats
	totalQuests := agent.Stats.QuestsCompleted + agent.Stats.QuestsFailed
	if totalQuests > 0 {
		stats.QuestSuccessRate = float64(agent.Stats.QuestsCompleted) / float64(totalQuests)
	}

	totalBattles := agent.Stats.BossesDefeated + agent.Stats.BossesFailed
	if totalBattles > 0 {
		stats.BattleWinRate = float64(agent.Stats.BossesDefeated) / float64(totalBattles)
	}

	// XP efficiency
	if agent.Stats.QuestsCompleted > 0 {
		stats.XPEfficiency = float64(agent.Stats.TotalXPEarned) / float64(agent.Stats.QuestsCompleted)
	}

	return stats
}

// getGuildMemberships retrieves guild membership details.
func (s *CharacterSheetService) getGuildMemberships(ctx context.Context, agent *Agent) []GuildMembership {
	memberships := make([]GuildMembership, 0, len(agent.Guilds))

	for _, guildID := range agent.Guilds {
		guildInstance := ExtractInstance(string(guildID))
		guild, err := s.storage.GetGuild(ctx, guildInstance)
		if err != nil {
			continue
		}

		// Find agent's membership info
		for _, member := range guild.Members {
			if member.AgentID == agent.ID {
				memberships = append(memberships, GuildMembership{
					GuildID:      guildID,
					GuildName:    guild.Name,
					Rank:         member.Rank,
					Contribution: member.Contribution,
					JoinedAt:     member.JoinedAt.Format("2006-01-02"),
				})
				break
			}
		}
	}

	return memberships
}

// buildEquipmentList creates the equipment list from agent tools.
func (s *CharacterSheetService) buildEquipmentList(agent *Agent) []EquippedItem {
	equipment := make([]EquippedItem, 0, len(agent.Equipment))

	for _, tool := range agent.Equipment {
		equipment = append(equipment, EquippedItem{
			ToolID:      tool.ID,
			ToolName:    tool.Name,
			Description: tool.Description,
			Category:    tool.Category,
			IsDangerous: tool.Dangerous,
			IsRental:    false, // Will be updated from inventory if available
		})
	}

	return equipment
}

// skillTagToName converts a skill tag to a human-readable name.
func skillTagToName(skill SkillTag) string {
	names := map[SkillTag]string{
		SkillCodeGen:       "Code Generation",
		SkillCodeReview:    "Code Review",
		SkillDataTransform: "Data Transformation",
		SkillSummarization: "Summarization",
		SkillResearch:      "Research",
		SkillPlanning:      "Planning",
		SkillCustomerComms: "Customer Communications",
		SkillAnalysis:      "Analysis",
		SkillTraining:      "Training",
	}
	if name, ok := names[skill]; ok {
		return name
	}
	return string(skill)
}

// GetSkillSummary returns a brief summary of an agent's skills.
func (s *CharacterSheetService) GetSkillSummary(ctx context.Context, agentID AgentID) (map[SkillTag]ProficiencyLevel, error) {
	agentInstance := ExtractInstance(string(agentID))

	agent, err := s.storage.GetAgent(ctx, agentInstance)
	if err != nil {
		return nil, err
	}

	if agent.SkillProficiencies != nil {
		summary := make(map[SkillTag]ProficiencyLevel, len(agent.SkillProficiencies))
		for skill, prof := range agent.SkillProficiencies {
			summary[skill] = prof.Level
		}
		return summary, nil
	}

	// Fall back to legacy skills
	summary := make(map[SkillTag]ProficiencyLevel, len(agent.Skills))
	for _, skill := range agent.Skills {
		summary[skill] = ProficiencyNovice
	}
	return summary, nil
}

// CompareAgents returns a comparison of two agents' character sheets.
func (s *CharacterSheetService) CompareAgents(ctx context.Context, agentA, agentB AgentID) (*AgentComparison, error) {
	sheetA, err := s.GetCharacterSheet(ctx, agentA)
	if err != nil {
		return nil, err
	}

	sheetB, err := s.GetCharacterSheet(ctx, agentB)
	if err != nil {
		return nil, err
	}

	return &AgentComparison{
		AgentA:       sheetA,
		AgentB:       sheetB,
		LevelDiff:    sheetA.Agent.Level - sheetB.Agent.Level,
		XPDiff:       sheetA.Agent.XP - sheetB.Agent.XP,
		SkillsAOnly:  findUniqueSkills(sheetA.SkillBars, sheetB.SkillBars),
		SkillsBOnly:  findUniqueSkills(sheetB.SkillBars, sheetA.SkillBars),
		CommonSkills: findCommonSkills(sheetA.SkillBars, sheetB.SkillBars),
	}, nil
}

// AgentComparison holds comparison data between two agents.
type AgentComparison struct {
	AgentA       *CharacterSheet   `json:"agent_a"`
	AgentB       *CharacterSheet   `json:"agent_b"`
	LevelDiff    int               `json:"level_diff"` // A.Level - B.Level
	XPDiff       int64             `json:"xp_diff"`    // A.XP - B.XP
	SkillsAOnly  []SkillTag        `json:"skills_a_only"`
	SkillsBOnly  []SkillTag        `json:"skills_b_only"`
	CommonSkills []SkillComparison `json:"common_skills"`
}

// SkillComparison compares proficiency in a shared skill.
type SkillComparison struct {
	Skill     SkillTag         `json:"skill"`
	LevelA    ProficiencyLevel `json:"level_a"`
	LevelB    ProficiencyLevel `json:"level_b"`
	LevelDiff int              `json:"level_diff"` // A.Level - B.Level
}

func findUniqueSkills(bars []SkillBar, other []SkillBar) []SkillTag {
	otherSet := make(map[SkillTag]bool, len(other))
	for _, bar := range other {
		otherSet[bar.Skill] = true
	}

	var unique []SkillTag
	for _, bar := range bars {
		if !otherSet[bar.Skill] {
			unique = append(unique, bar.Skill)
		}
	}
	return unique
}

func findCommonSkills(barsA, barsB []SkillBar) []SkillComparison {
	mapA := make(map[SkillTag]ProficiencyLevel, len(barsA))
	for _, bar := range barsA {
		mapA[bar.Skill] = bar.Level
	}

	var common []SkillComparison
	for _, bar := range barsB {
		if levelA, ok := mapA[bar.Skill]; ok {
			common = append(common, SkillComparison{
				Skill:     bar.Skill,
				LevelA:    levelA,
				LevelB:    bar.Level,
				LevelDiff: int(levelA) - int(bar.Level),
			})
		}
	}
	return common
}
