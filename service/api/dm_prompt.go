package api

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/partycoord"
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
//
// All modes share the DM persona, world state summary, and injected context.
func (s *Service) buildDMSystemPrompt(ctx context.Context, mode domain.ChatMode, contextItems []dmChatContextItem, sessionRecap string) string {
	var b strings.Builder

	// DM persona (shared, mode-neutral)
	b.WriteString(`You are the Dungeon Master of a quest-based agentic workflow system called Semdragons.

## What You Can Do

- Answer questions about the game world, agents, quests, parties, guilds, and system concepts
- Explain how the system works (trust tiers, XP, boid engine, boss battles, etc.)
- Help users design quests and quest chains (when in quest mode)
- Describe agent capabilities, skills, and current status from the world state
- Suggest which difficulty, skills, or review level to use for a quest
- Advise on whether a quest should be a party quest or solo

## What You Cannot Do

You have NO ability to take direct actions in the game world. Be honest about this.
Do not pretend, imply, or promise that you can do any of the following:

- Assign, reassign, or route quests to specific agents (agents claim quests autonomously via the boid engine)
- Change agent levels, XP, trust tiers, or skills (these are earned through quest completion)
- Recruit, retire, kill, or revive agents
- Start, stop, cancel, or abort running quests
- Intervene in or override active quest execution
- Override or change boss battle review results
- Form, modify, or disband parties
- Create, modify, or disband guilds
- Change game settings, configuration, or system parameters
- Send messages to agents or direct their behavior

If a user asks you to do something from the "cannot do" list, clearly explain that you
cannot take that action and why. Then suggest what CAN be done instead — for example,
if they want a specific agent to work on something, suggest creating a quest with skills
that match that agent's proficiencies so the boid engine routes it naturally.

## How the System Works

Agents are autonomous — they claim quests from the board based on their skills, trust
tier, and a boid-flocking algorithm. You do NOT assign quests to agents. Instead, you
post quests to the board and agents pull work they are qualified for. Trust tiers
(Apprentice through Grandmaster) are earned through XP from completed quests, not
manually assigned.
`)

	// Mode-specific instructions
	switch mode {
	case domain.ChatModeQuest:
		b.WriteString(`Your role is to help users create quest specs through natural conversation.
A quest spec defines WHAT needs to be done and HOW to verify it — not HOW to do it.
That is the agent's job.

IMPORTANT: When the user describes work to be done, you MUST include a JSON block in your
response with the quest specification. Do not just discuss the quest — always produce the
structured JSON output. Use one of the two formats below.

`)
		s.writeQuestSpecInstructions(&b)
		s.writeAvailableOptions(&b)

	default: // converse
		b.WriteString(`
You are in conversation mode. Answer questions about the game world, agents, quests,
and system concepts in natural language. Do NOT produce JSON blocks or structured output.

If the user wants to create a quest, tell them to switch to quest mode.
If the user asks you to take an action you cannot perform, say so directly — do not
role-play having performed the action. Suggest an alternative that works within the
system's actual capabilities.

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

	// Session recap for multi-turn continuity
	if sessionRecap != "" {
		b.WriteString("## Session Recap\n\n")
		b.WriteString(sessionRecap)
		b.WriteString("\n\n")
	}

	// Final reinforcement for quest mode — placed last so it's closest to the
	// model's generation point. Gemini Pro sometimes ignores the JSON format
	// instruction when it's buried in the middle of a long system prompt.
	if mode == domain.ChatModeQuest {
		b.WriteString("REMINDER: You MUST include a ```json:quest_brief or ```json:quest_chain code block in your response. Do not respond without structured JSON output.\n\n")
	}

	return b.String()
}

// buildSessionRecap generates a compact summary of prior conversation turns.
// Injected into the system prompt to help the LLM maintain continuity across turns.
func buildSessionRecap(session *DMChatSession) string {
	if len(session.Turns) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("This is a continuing conversation. Key points so far:\n")

	// Summarize the last 3 turns (most relevant context)
	start := len(session.Turns) - 3
	if start < 0 {
		start = 0
	}
	for _, turn := range session.Turns[start:] {
		userMsg := turn.UserMessage
		if len(userMsg) > 100 {
			userMsg = userMsg[:100] + "..."
		}
		b.WriteString(fmt.Sprintf("- User said: %s\n", userMsg))
	}
	return b.String()
}

// writeQuestSpecInstructions appends quest spec format instructions to the builder.
// Teaches the DM to think in terms of goal → requirements → scenarios.
func (s *Service) writeQuestSpecInstructions(b *strings.Builder) {
	b.WriteString(`## Quest Spec Format

A quest spec has four parts:

1. **Goal** (required) — The desired outcome. Why does this work matter? One or two sentences.
2. **Requirements** — Concrete constraints that must be satisfied. Short, testable statements.
3. **Scenarios** — Named, testable outcomes that prove the requirements are met. Each
   scenario has a name, description, and optional skills. If a scenario MUST wait for
   another scenario to complete first, add a "depends_on" reference to that scenario's name.
4. **Skills** — Aggregate skill tags needed across all scenarios.

### Scenario Dependencies Drive Staffing

The system uses scenario dependencies to decide how a quest is staffed:

- **Independent scenarios** (no depends_on between them) → PARTY quest. Multiple agents
  work on scenarios in parallel. This is significantly more effective for parallelizable work.
- **Sequential scenarios** (each depends on the previous) → SOLO quest. One high-capability
  agent handles the entire chain. Research shows multi-agent coordination DEGRADES
  performance by 39-70% on sequential tasks.
- **Mixed** (some independent, some dependent) → PARTY quest with a smaller team.

Declare depends_on honestly — it determines whether a team or a solo agent handles the quest.
If scenarios can genuinely be worked on simultaneously, do NOT add depends_on between them.

## Output Format

For a SINGLE quest, include a JSON block tagged as quest_brief:
` + "```json:quest_brief" + `
{
  "title": "Build user notification service",
  "goal": "Users receive real-time notifications across email and in-app channels when workspace events occur",
  "requirements": [
    "Support email and in-app notification channels",
    "Users can configure per-event notification preferences",
    "Notifications delivered within 5 seconds of triggering event"
  ],
  "scenarios": [
    {
      "name": "In-app notification delivery",
      "description": "When a workspace event fires and the user has in-app enabled, a notification appears in their feed within 5s",
      "skills": ["code_generation"]
    },
    {
      "name": "Email with preference check",
      "description": "When a workspace event fires, the system checks user preferences and sends email only if enabled for that event type",
      "skills": ["code_generation", "data_transformation"]
    },
    {
      "name": "Preference configuration API",
      "description": "User can GET and PUT their notification preferences per event type, with validation that event types and channels are valid",
      "skills": ["code_generation", "analysis"]
    }
  ],
  "difficulty": 3,
  "skills": ["code_generation", "data_transformation", "analysis"]
}
` + "```" + `

The above example has three independent scenarios (no depends_on) → the system will
staff it as a party quest with agents working in parallel.

Here is a sequential example:
` + "```json:quest_brief" + `
{
  "title": "Migrate legacy auth to OAuth2",
  "goal": "Replace custom token auth with OAuth2 PKCE flow without breaking existing sessions",
  "requirements": [
    "Existing sessions remain valid during migration",
    "New auth uses OAuth2 PKCE",
    "Rollback plan if migration fails"
  ],
  "scenarios": [
    {
      "name": "OAuth2 provider integration",
      "description": "Configure OAuth2 provider, implement PKCE flow, verify token exchange works"
    },
    {
      "name": "Session migration bridge",
      "description": "Build adapter that validates both old tokens and new OAuth2 tokens during transition period",
      "depends_on": ["OAuth2 provider integration"]
    },
    {
      "name": "Cutover and rollback",
      "description": "Switch all endpoints to OAuth2-only with feature flag, verify rollback restores old auth",
      "depends_on": ["Session migration bridge"]
    }
  ],
  "difficulty": 4,
  "skills": ["code_generation"]
}
` + "```" + `

The above example has a linear dependency chain → the system will assign it to a single
high-capability agent.

For MULTIPLE related quests (a chain), include a JSON block tagged as quest_chain:
` + "```json:quest_chain" + `
{
  "quests": [
    {
      "title": "Research data sources",
      "goal": "Identify and evaluate all available data sources for the pipeline",
      "difficulty": 1,
      "skills": ["research"],
      "scenarios": [
        {
          "name": "Source inventory",
          "description": "List all available data sources with format, volume, and refresh frequency"
        }
      ]
    },
    {
      "title": "Build data pipeline",
      "goal": "Implement the ETL pipeline using the sources identified in the research phase",
      "difficulty": 3,
      "skills": ["code_generation", "data_transformation"],
      "scenarios": [
        {
          "name": "Extract and validate",
          "description": "Pull data from each source and validate schema conformance"
        },
        {
          "name": "Transform and load",
          "description": "Apply business rules and load into target tables",
          "depends_on": ["Extract and validate"]
        }
      ],
      "depends_on": [0]
    }
  ]
}
` + "```" + `

Quests can include an optional "hints" object for advanced configuration:
- "review_level": 0-3 — review strictness (0=Auto, 1=Standard, 2=Strict, 3=Human)
- "require_human_review": true/false — shorthand for review_level 3
- "prefer_guild": "guild_id" — route to a specific guild
- "party_required": true/false — manual override (normally the system decides based on scenarios)
- "min_party_size": 2-5 — minimum agents in the party (default 2)

IMPORTANT: When choosing skills for scenarios and the quest, prefer skills that match
agents currently available in the roster below. If no agents have the exact skill, pick
the closest match. This ensures agents can actually claim and work on the quest.

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

	b.WriteString("**Solo vs Party**: The system decides based on scenario dependencies.\n")
	b.WriteString("You do NOT need to set party_required — just declare depends_on honestly.\n")
	b.WriteString("- Independent scenarios → party (agents work in parallel)\n")
	b.WriteString("- Sequential chain → solo (one high-capability agent)\n")
	b.WriteString("- Use `hints.party_required` only to manually override the system's decision.\n\n")
}

// writeWorldState appends the current world state summary including agent roster.
func (s *Service) writeWorldState(ctx context.Context, b *strings.Builder) {
	b.WriteString("## Current World State\n\n")
	ws, err := s.world.WorldState(ctx)
	if err != nil || ws == nil {
		b.WriteString("World state unavailable.\n\n")
		return
	}

	b.WriteString(fmt.Sprintf("- Agents: %d total (%d active, %d idle)\n",
		ws.Stats.ActiveAgents+ws.Stats.IdleAgents+ws.Stats.CooldownAgents,
		ws.Stats.ActiveAgents, ws.Stats.IdleAgents))
	b.WriteString(fmt.Sprintf("- Quests: %d open, %d active\n",
		ws.Stats.OpenQuests, ws.Stats.ActiveQuests))
	b.WriteString(fmt.Sprintf("- Guilds: %d active\n", ws.Stats.ActiveGuilds))
	b.WriteString(fmt.Sprintf("- Completion rate: %.0f%%\n\n", ws.Stats.CompletionRate*100))

	b.WriteString(fmt.Sprintf("- Parties: %d active\n\n", ws.Stats.ActiveParties))

	writeAgentRoster(b, ws.Agents)
	writeQuestList(b, ws.Quests)
	writePartyList(b, ws.Parties)
}

// writeAgentRoster appends a compact agent roster sorted by level descending.
func writeAgentRoster(b *strings.Builder, agents []any) {
	if len(agents) == 0 {
		return
	}

	// Type-assert and collect agent summaries.
	type agentSummary struct {
		name   string
		level  int
		tier   string
		status string
		skills []string
	}

	summaries := make([]agentSummary, 0, len(agents))
	for _, a := range agents {
		agent, ok := a.(agentprogression.Agent)
		if !ok {
			continue
		}
		skills := make([]string, 0, len(agent.SkillProficiencies))
		for tag := range agent.SkillProficiencies {
			skills = append(skills, string(tag))
		}
		sort.Strings(skills)
		summaries = append(summaries, agentSummary{
			name:   agent.Name,
			level:  agent.Level,
			tier:   agent.Tier.String(),
			status: string(agent.Status),
			skills: skills,
		})
	}

	// Sort by level descending.
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].level > summaries[j].level
	})

	b.WriteString("### Agent Roster\n\n")
	for _, s := range summaries {
		b.WriteString(fmt.Sprintf("- **%s** — Level %d %s, %s, skills: %s\n",
			s.name, s.level, s.tier, s.status, strings.Join(s.skills, ", ")))
	}
	b.WriteString("\n")
}

// writeQuestList appends active quests to the prompt.
func writeQuestList(b *strings.Builder, quests []any) {
	if len(quests) == 0 {
		return
	}

	// Type-assert quest objects.
	type questSummary struct {
		title      string
		status     string
		difficulty string
	}

	var summaries []questSummary
	for _, q := range quests {
		quest, ok := q.(domain.Quest)
		if !ok {
			continue
		}
		summaries = append(summaries, questSummary{
			title:      quest.Title,
			status:     string(quest.Status),
			difficulty: fmt.Sprintf("difficulty %d", quest.Difficulty),
		})
	}

	if len(summaries) == 0 {
		return
	}

	b.WriteString("### Active Quests\n\n")
	for _, s := range summaries {
		b.WriteString(fmt.Sprintf("- **%s** — %s (%s)\n", s.title, s.status, s.difficulty))
	}
	b.WriteString("\n")
}

// writePartyList appends active parties to the prompt.
func writePartyList(b *strings.Builder, parties []any) {
	if len(parties) == 0 {
		return
	}

	var lines []string
	for _, p := range parties {
		party, ok := p.(partycoord.Party)
		if !ok {
			continue
		}
		lines = append(lines, fmt.Sprintf("- **%s** — %d members, quest: %s, status: %s",
			party.Name, len(party.Members), party.QuestID, party.Status))
	}

	if len(lines) == 0 {
		return
	}

	b.WriteString("### Active Parties\n\n")
	for _, line := range lines {
		b.WriteString(line)
		b.WriteString("\n")
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
