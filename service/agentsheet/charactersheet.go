// Package agentsheet provides character sheet projections for agent entities.
// It combines data from the agentprogression processor with guild membership
// data from the graph for UI display and decision-making.
package agentsheet

import (
	"context"
	"fmt"
	"sort"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
)

// =============================================================================
// CHARACTER SHEET SERVICE - Agent profile projections
// =============================================================================
// The character sheet provides a comprehensive view of an agent's current state,
// skills, equipment, and derived statistics for UI display and decision-making.
// =============================================================================

// CharacterSheet is a complete projection of an agent's state.
type CharacterSheet struct {
	Agent        agentprogression.Agent `json:"agent"`
	SkillBars    []SkillBar             `json:"skill_bars"`
	DerivedStats DerivedStats           `json:"derived_stats"`
	Memberships  []GuildMembership      `json:"memberships"`
	Equipment    []EquippedItem         `json:"equipment"`
	Inventory    any                    `json:"inventory,omitempty"` // *agentstore.AgentInventory, populated separately
}

// SkillBar represents a skill with its proficiency for UI display.
type SkillBar struct {
	Skill           domain.SkillTag         `json:"skill"`
	SkillName       string                  `json:"skill_name"`
	Level           domain.ProficiencyLevel `json:"level"`
	LevelName       string                  `json:"level_name"`
	Progress        int                     `json:"progress"` // 0-99
	ProgressPercent float64                 `json:"progress_percent"`
	TotalXP         int64                   `json:"total_xp"`
	QuestsUsed      int                     `json:"quests_used"`
	IsMaxLevel      bool                    `json:"is_max_level"`
}

// DerivedStats contains computed statistics about an agent.
type DerivedStats struct {
	AvgProficiency   float64         `json:"avg_proficiency"`
	StrongestSkill   domain.SkillTag `json:"strongest_skill,omitempty"`
	WeakestSkill     domain.SkillTag `json:"weakest_skill,omitempty"`
	QuestSuccessRate float64         `json:"quest_success_rate"`
	BattleWinRate    float64         `json:"battle_win_rate"`
	TotalSkills      int             `json:"total_skills"`
	MasterSkills     int             `json:"master_skills"` // Skills at max level
	XPEfficiency     float64         `json:"xp_efficiency"` // XP earned per quest
}

// GuildMembership represents an agent's status in a guild.
type GuildMembership struct {
	GuildID      domain.GuildID   `json:"guild_id"`
	GuildName    string           `json:"guild_name"`
	Rank         domain.GuildRank `json:"rank"`
	Contribution float64          `json:"contribution"` // XP contributed via guild quests
	JoinedAt     string           `json:"joined_at"`
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

// Service creates character sheet projections.
type Service struct {
	graph *semdragons.GraphClient
}

// NewService creates a new character sheet service.
func NewService(graph *semdragons.GraphClient) *Service {
	return &Service{graph: graph}
}

// GetCharacterSheet builds a complete character sheet for an agent.
func (s *Service) GetCharacterSheet(ctx context.Context, agentID domain.AgentID) (*CharacterSheet, error) {
	entity, err := s.graph.GetAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	agent := agentprogression.AgentFromEntityState(entity)
	if agent == nil {
		return nil, fmt.Errorf("agent not found: %s", agentID)
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
	var inventory any

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
func (s *Service) buildSkillBars(agent *agentprogression.Agent) []SkillBar {
	if len(agent.SkillProficiencies) == 0 {
		return nil // No skills to display
	}

	bars := make([]SkillBar, 0, len(agent.SkillProficiencies))
	for skill, prof := range agent.SkillProficiencies {
		bars = append(bars, SkillBar{
			Skill:           skill,
			SkillName:       skillTagToName(skill),
			Level:           prof.Level,
			LevelName:       domain.ProficiencyLevelName(prof.Level),
			Progress:        prof.Progress,
			ProgressPercent: prof.ProgressPercent(),
			TotalXP:         prof.TotalXP,
			QuestsUsed:      prof.QuestsUsed,
			IsMaxLevel:      prof.Level >= domain.ProficiencyMaster,
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
func (s *Service) calculateDerivedStats(agent *agentprogression.Agent, skillBars []SkillBar) DerivedStats {
	stats := DerivedStats{
		TotalSkills: len(skillBars),
	}

	if len(skillBars) == 0 {
		return stats
	}

	// Calculate average proficiency and find strongest/weakest
	var totalLevel float64
	var strongest, weakest SkillBar
	strongest.Level = domain.ProficiencyNovice
	weakest.Level = domain.ProficiencyMaster

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
func (s *Service) getGuildMemberships(ctx context.Context, agent *agentprogression.Agent) []GuildMembership {
	if agent.Guild == "" {
		return nil
	}

	entity, err := s.graph.GetGuild(ctx, agent.Guild)
	if err != nil {
		return nil
	}
	guild := domain.GuildFromEntityState(entity)
	if guild == nil {
		return nil
	}

	// Find agent's membership info
	for _, member := range guild.Members {
		if member.AgentID == agent.ID {
			return []GuildMembership{{
				GuildID:      agent.Guild,
				GuildName:    guild.Name,
				Rank:         member.Rank,
				Contribution: member.Contribution,
				JoinedAt:     member.JoinedAt.Format("2006-01-02"),
			}}
		}
	}

	return nil
}

// buildEquipmentList creates the equipment list from agent tools.
func (s *Service) buildEquipmentList(agent *agentprogression.Agent) []EquippedItem {
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
func skillTagToName(skill domain.SkillTag) string {
	names := map[domain.SkillTag]string{
		domain.SkillCodeGen:       "Code Generation",
		domain.SkillCodeReview:    "Code Review",
		domain.SkillDataTransform: "Data Transformation",
		domain.SkillSummarization: "Summarization",
		domain.SkillResearch:      "Research",
		domain.SkillPlanning:      "Planning",
		domain.SkillCustomerComms: "Customer Communications",
		domain.SkillAnalysis:      "Analysis",
		domain.SkillTraining:      "Training",
	}
	if name, ok := names[skill]; ok {
		return name
	}
	return string(skill)
}

// GetSkillSummary returns a brief summary of an agent's skills.
func (s *Service) GetSkillSummary(ctx context.Context, agentID domain.AgentID) (map[domain.SkillTag]domain.ProficiencyLevel, error) {
	entity, err := s.graph.GetAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	agent := agentprogression.AgentFromEntityState(entity)
	if agent == nil {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}

	if agent.SkillProficiencies != nil {
		summary := make(map[domain.SkillTag]domain.ProficiencyLevel, len(agent.SkillProficiencies))
		for skill, prof := range agent.SkillProficiencies {
			summary[skill] = prof.Level
		}
		return summary, nil
	}

	// No skills defined
	return make(map[domain.SkillTag]domain.ProficiencyLevel), nil
}

// CompareAgents returns a comparison of two agents' character sheets.
func (s *Service) CompareAgents(ctx context.Context, agentA, agentB domain.AgentID) (*AgentComparison, error) {
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
	SkillsAOnly  []domain.SkillTag `json:"skills_a_only"`
	SkillsBOnly  []domain.SkillTag `json:"skills_b_only"`
	CommonSkills []SkillComparison `json:"common_skills"`
}

// SkillComparison compares proficiency in a shared skill.
type SkillComparison struct {
	Skill     domain.SkillTag         `json:"skill"`
	LevelA    domain.ProficiencyLevel `json:"level_a"`
	LevelB    domain.ProficiencyLevel `json:"level_b"`
	LevelDiff int                     `json:"level_diff"` // A.Level - B.Level
}

func findUniqueSkills(bars []SkillBar, other []SkillBar) []domain.SkillTag {
	otherSet := make(map[domain.SkillTag]bool, len(other))
	for _, bar := range other {
		otherSet[bar.Skill] = true
	}

	var unique []domain.SkillTag
	for _, bar := range bars {
		if !otherSet[bar.Skill] {
			unique = append(unique, bar.Skill)
		}
	}
	return unique
}

func findCommonSkills(barsA, barsB []SkillBar) []SkillComparison {
	mapA := make(map[domain.SkillTag]domain.ProficiencyLevel, len(barsA))
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
