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
// The prompt varies by chat mode:
//   - converse: Q&A only, no structured output
//   - quest: quest/chain creation with JSON schemas
//   - plan: stub (converse + coming soon note)
//   - manage: stub (converse + coming soon note)
//
// All modes share the DM persona, world state summary, and injected context.
func (s *Service) buildDMSystemPrompt(ctx context.Context, mode domain.ChatMode, contextItems []dmChatContextItem) string {
	var b strings.Builder

	// DM persona (shared, mode-neutral)
	b.WriteString(`You are the Dungeon Master of a quest-based agentic workflow system called Semdragons.
You oversee agents, quests, parties, guilds, and boss battles.
`)

	// Mode-specific instructions
	switch mode {
	case domain.ChatModeQuest:
		b.WriteString(`Your role is to help users create quests and quest chains through natural conversation.
You should ask clarifying questions when the user's intent is unclear, and suggest
appropriate difficulty levels, required skills, and acceptance criteria.

IMPORTANT: When the user describes work to be done, you MUST include a JSON block in your
response with the quest specification. Do not just discuss the quest — always produce the
structured JSON output. Use one of the two formats below.

`)
		s.writeQuestSchemaInstructions(&b)
		s.writeAvailableOptions(&b)

	case domain.ChatModePlan:
		b.WriteString(`
Plan mode is coming soon. For now, help the user think through their objective.
Ask clarifying questions about scope, dependencies, and success criteria.
Do NOT produce JSON blocks or structured output. Answer in natural language only.

`)

	case domain.ChatModeManage:
		b.WriteString(`
Manage mode is coming soon. For now, answer questions about agents, their status,
levels, skills, and performance. Do NOT produce JSON blocks or structured output.
Answer in natural language only.

`)

	default: // converse
		b.WriteString(`
Answer the user's questions about the game world, agents, quests, and system concepts.
Do NOT produce JSON blocks or structured output. Answer in natural language only.

`)
	}

	// World state summary (shared)
	s.writeWorldState(ctx, &b)

	// Context entities (shared)
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

// writeQuestSchemaInstructions appends quest/chain JSON schema instructions to the builder.
func (s *Service) writeQuestSchemaInstructions(b *strings.Builder) {
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

If the user's intent is unclear, ask ONE clarifying question — but still include your
best-guess JSON block so the user can refine it. Prefer producing output over asking
questions. Never respond without a JSON block unless you genuinely cannot determine any
objective from the user's message.

`)
}

// writeAvailableOptions appends difficulty, skill, and review level references.
func (s *Service) writeAvailableOptions(b *strings.Builder) {
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
}

// writeWorldState appends the current world state summary.
func (s *Service) writeWorldState(ctx context.Context, b *strings.Builder) {
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
