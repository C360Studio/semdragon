package redteam

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/c360studio/semdragons/domain"
)

// extractAndStoreLessons parses red-team findings and the battle verdict to
// produce structured Lesson entries. Lessons are written to both the
// implementing guild (blue team) and the reviewing guild (red team).
// The rtQuest parameter provides the red-team quest entity (for DiscoveredBy).
func (c *Component) extractAndStoreLessons(ctx context.Context, originalQuest *domain.Quest, rtQuest *domain.Quest) {
	if rtQuest == nil || rtQuest.Output == nil {
		return
	}

	// Parse findings from the red-team output.
	findings := parseFindings(rtQuest.Output)
	if len(findings) == 0 {
		c.logger.Debug("no structured findings to extract", "quest", originalQuest.ID)
		return
	}

	// Default unstructured findings to the quest's primary skill so they
	// can actually be surfaced to agents (lessons with empty Skill never
	// match any quest's RequiredSkills filter).
	primarySkill := originalQuest.PrimarySkill()
	for i := range findings {
		if findings[i].Skill == "" && primarySkill != "" {
			findings[i].Skill = primarySkill
		}
	}

	// Resolve the blue team's guild (implementing agent's guild).
	var blueGuildID domain.GuildID
	if originalQuest.ClaimedBy != nil {
		if gid := c.resolveAgentGuild(*originalQuest.ClaimedBy); gid != nil {
			blueGuildID = *gid
		}
	}

	// Resolve red-team agent for DiscoveredBy.
	var redAgentID domain.AgentID
	if rtQuest.ClaimedBy != nil {
		redAgentID = *rtQuest.ClaimedBy
	}

	// Build lessons from findings.
	lessons := make([]domain.Lesson, 0, len(findings))
	for i, f := range findings {
		lesson := domain.Lesson{
			ID:           fmt.Sprintf("lesson-%s-%d", originalQuest.ID, i),
			Skill:        f.Skill,
			Category:     f.Category,
			Summary:      f.Summary,
			Detail:       f.Detail,
			Severity:     f.Severity,
			Positive:     f.Positive,
			QuestID:      originalQuest.ID,
			DiscoveredBy: redAgentID,
		}
		if blueGuildID != "" {
			lesson.GuildID = blueGuildID
		}
		lessons = append(lessons, lesson)
	}

	if len(lessons) == 0 {
		return
	}

	// Write lessons to the blue team's guild.
	if blueGuildID != "" {
		c.appendGuildLessons(ctx, blueGuildID, lessons)
	}

	// Write lessons to the red team's guild too (reviewing teaches).
	// Use the red-team quest's ClaimedBy to find the agent's guild (no scan).
	var redGuildID domain.GuildID
	if rtQuest.ClaimedBy != nil {
		if gid := c.resolveAgentGuild(*rtQuest.ClaimedBy); gid != nil {
			redGuildID = *gid
		}
	}
	if redGuildID != "" && redGuildID != blueGuildID {
		c.appendGuildLessons(ctx, redGuildID, lessons)
	}

	c.logger.Info("extracted and stored lessons",
		"quest", originalQuest.ID,
		"lesson_count", len(lessons),
		"blue_guild", blueGuildID,
		"red_guild", redGuildID)
}

// appendGuildLessons loads a guild, appends lessons, and persists.
func (c *Component) appendGuildLessons(ctx context.Context, guildID domain.GuildID, lessons []domain.Lesson) {
	guildEntity, err := c.graph.GetGuild(ctx, guildID)
	if err != nil {
		c.logger.Debug("failed to load guild for lessons", "guild", guildID, "error", err)
		return
	}
	guild := domain.GuildFromEntityState(guildEntity)
	if guild == nil {
		return
	}

	// Tag lessons with this guild.
	tagged := make([]domain.Lesson, len(lessons))
	copy(tagged, lessons)
	for i := range tagged {
		tagged[i].GuildID = guildID
	}

	guild.Lessons = append(guild.Lessons, tagged...)

	// Cap total lessons to prevent unbounded growth.
	if len(guild.Lessons) > 100 {
		guild.Lessons = guild.Lessons[len(guild.Lessons)-100:]
	}

	if err := c.graph.EmitEntityUpdate(ctx, guild, domain.PredicateGuildLessonAdded); err != nil {
		c.logger.Error("failed to persist guild lessons", "guild", guildID, "error", err)
	}
}


// finding is a parsed entry from red-team output.
type finding struct {
	Skill    domain.SkillTag        `json:"skill"`
	Category domain.LessonCategory  `json:"category"`
	Summary  string                 `json:"summary"`
	Detail   string                 `json:"detail"`
	Severity domain.LessonSeverity  `json:"severity"`
	Positive bool                   `json:"positive"`
}

// parseFindings attempts to extract structured findings from red-team output.
// Supports both JSON-structured output and falls back to a single generic finding.
func parseFindings(output any) []finding {
	if output == nil {
		return nil
	}

	// Try to marshal to JSON and parse as structured findings.
	raw, err := json.Marshal(output)
	if err != nil {
		return singleFinding(output)
	}

	// Try direct array of findings.
	var findings []finding
	if json.Unmarshal(raw, &findings) == nil && len(findings) > 0 {
		return normalizeFindings(findings)
	}

	// Try object with "risks", "strengths", "suggestions" keys.
	var structured map[string]any
	if json.Unmarshal(raw, &structured) == nil {
		return extractFromStructured(structured)
	}

	return singleFinding(output)
}

// extractFromStructured parses the expected red-team output format with
// strengths, risks, and suggestions sections.
func extractFromStructured(m map[string]any) []finding {
	var results []finding

	// Extract risks.
	if risks, ok := m["risks"]; ok {
		results = append(results, extractSection(risks, false)...)
	}

	// Extract strengths.
	if strengths, ok := m["strengths"]; ok {
		results = append(results, extractSection(strengths, true)...)
	}

	if len(results) == 0 {
		return nil
	}
	return normalizeFindings(results)
}

// extractSection parses a section (array of items or single string).
func extractSection(section any, positive bool) []finding {
	switch v := section.(type) {
	case []any:
		var results []finding
		for _, item := range v {
			switch entry := item.(type) {
			case map[string]any:
				f := finding{Positive: positive}
				if s, ok := entry["skill"].(string); ok {
					f.Skill = domain.SkillTag(s)
				}
				if s, ok := entry["category"].(string); ok {
					f.Category = domain.LessonCategory(s)
				}
				if s, ok := entry["summary"].(string); ok {
					f.Summary = s
				}
				if s, ok := entry["detail"].(string); ok {
					f.Detail = s
				}
				if s, ok := entry["severity"].(string); ok {
					f.Severity = domain.LessonSeverity(s)
				}
				if f.Summary != "" {
					results = append(results, f)
				}
			case string:
				if entry != "" {
					results = append(results, finding{
						Summary:  entry,
						Positive: positive,
						Severity: domain.LessonSeverityInfo,
					})
				}
			}
		}
		return results
	case string:
		if v != "" {
			return []finding{{
				Summary:  v,
				Positive: positive,
				Severity: domain.LessonSeverityInfo,
			}}
		}
	}
	return nil
}

// singleFinding wraps unstructured output as a single generic finding.
func singleFinding(output any) []finding {
	summary := fmt.Sprintf("%v", output)
	if len(summary) > 200 {
		summary = summary[:200] + "..."
	}
	return []finding{{
		Summary:  summary,
		Category: domain.LessonQuality,
		Severity: domain.LessonSeverityInfo,
	}}
}

// normalizeFindings ensures all findings have default values for required fields.
func normalizeFindings(findings []finding) []finding {
	for i := range findings {
		if findings[i].Category == "" {
			findings[i].Category = domain.LessonQuality
		}
		if findings[i].Severity == "" {
			findings[i].Severity = domain.LessonSeverityInfo
		}
	}
	return findings
}

