package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/c360studio/semdragons/domain"
)

// dmChatContextItem is an entity reference injected as context into the DM chat.
type dmChatContextItem struct {
	Type string `json:"type"` // "agent", "quest", "battle", "guild"
	ID   string `json:"id"`
}

// buildDMSystemPrompt constructs the system prompt for the DM chat assistant.
// It includes the DM persona, available game options, world state summary,
// context entities, and output format instructions.
func (s *Service) buildDMSystemPrompt(ctx context.Context, contextItems []dmChatContextItem) string {
	var b strings.Builder

	// DM persona
	b.WriteString(`You are the Dungeon Master of a quest-based agentic workflow system called Semdragons.
Your role is to help users create quests and quest chains through natural conversation.
You should ask clarifying questions when the user's intent is unclear, and suggest
appropriate difficulty levels, required skills, and acceptance criteria.

When you have gathered enough information to create a quest, include a JSON block in your
response with the quest specification. Use one of the two formats below.

`)

	// Quest schema instructions
	b.WriteString(`## Output Format

For a SINGLE quest, include a JSON block tagged as quest_brief:
` + "```json:quest_brief" + `
{
  "title": "Short descriptive title",
  "description": "Detailed description of what needs to be done",
  "difficulty": 2,
  "skills": ["code_generation", "analysis"],
  "acceptance": ["Criterion 1", "Criterion 2"]
}
` + "```" + `

For MULTIPLE related quests (a chain), include a JSON block tagged as quest_chain:
` + "```json:quest_chain" + `
{
  "quests": [
    {
      "title": "First quest",
      "description": "...",
      "difficulty": 1,
      "skills": ["research"],
      "acceptance": ["..."]
    },
    {
      "title": "Second quest (depends on first)",
      "description": "...",
      "difficulty": 2,
      "skills": ["code_generation"],
      "acceptance": ["..."],
      "depends_on": [0]
    }
  ]
}
` + "```" + `

Only include the JSON block when you have enough information to create the quest(s).
Keep conversing to gather details until the user's intent is clear.

`)

	// Available options reference
	b.WriteString("## Available Options\n\n")

	b.WriteString("**Difficulty levels** (0-5): ")
	b.WriteString("0=Trivial, 1=Easy, 2=Moderate, 3=Hard, 4=Epic, 5=Legendary\n\n")

	b.WriteString("**Skill tags**: ")
	skills := []string{
		string(domain.SkillCodeGen),
		string(domain.SkillCodeReview),
		string(domain.SkillDataTransform),
		string(domain.SkillSummarization),
		string(domain.SkillResearch),
		string(domain.SkillPlanning),
		string(domain.SkillCustomerComms),
		string(domain.SkillAnalysis),
		string(domain.SkillTraining),
	}
	b.WriteString(strings.Join(skills, ", "))
	b.WriteString("\n\n")

	b.WriteString("**Review levels**: 0=Auto, 1=Standard, 2=Strict, 3=Human\n\n")

	// World state summary
	b.WriteString("## Current World State\n\n")
	ws, err := s.world.WorldState(ctx)
	if err == nil && ws != nil {
		b.WriteString(fmt.Sprintf("- Agents: %d total (%d active, %d idle)\n",
			ws.Stats.ActiveAgents+ws.Stats.IdleAgents+ws.Stats.CooldownAgents,
			ws.Stats.ActiveAgents, ws.Stats.IdleAgents))
		b.WriteString(fmt.Sprintf("- Quests: %d open, %d active\n",
			ws.Stats.OpenQuests, ws.Stats.ActiveQuests))
		b.WriteString(fmt.Sprintf("- Guilds: %d active\n", ws.Stats.ActiveGuilds))
		b.WriteString(fmt.Sprintf("- Completion rate: %.0f%%\n", ws.Stats.CompletionRate*100))
	} else {
		b.WriteString("World state unavailable.\n")
	}
	b.WriteString("\n")

	// Context entities
	if len(contextItems) > 0 {
		b.WriteString("## Injected Context\n\n")
		for _, item := range contextItems {
			detail := s.resolveContextDetail(ctx, item)
			if detail != "" {
				b.WriteString(detail)
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}

	return b.String()
}

// resolveContextDetail fetches a summary for an injected context entity.
func (s *Service) resolveContextDetail(ctx context.Context, item dmChatContextItem) string {
	switch item.Type {
	case "agent":
		entity, err := s.graph.GetAgent(ctx, domain.AgentID(item.ID))
		if err != nil {
			return fmt.Sprintf("Agent %s: (not found)", item.ID)
		}
		// Extract name and level from entity state
		data, _ := json.Marshal(entity.Triples)
		return fmt.Sprintf("Agent %s: %s", item.ID, truncate(string(data), 200))

	case "quest":
		entity, err := s.graph.GetQuest(ctx, domain.QuestID(item.ID))
		if err != nil {
			return fmt.Sprintf("Quest %s: (not found)", item.ID)
		}
		data, _ := json.Marshal(entity.Triples)
		return fmt.Sprintf("Quest %s: %s", item.ID, truncate(string(data), 200))

	default:
		return fmt.Sprintf("%s %s: (context type not supported)", item.Type, item.ID)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
