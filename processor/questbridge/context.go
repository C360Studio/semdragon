package questbridge

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/partycoord"
	pkgcontext "github.com/c360studio/semstreams/pkg/context"
)

// entityKnowledgeBuilder constructs LLM-readable entity context from graph state.
// It queries related entities (party, guild, parent quest) and formats them as
// structured text that agents can understand when starting a quest.
type entityKnowledgeBuilder struct {
	graph       *semdragons.GraphClient
	budgetToken int
	logger      *slog.Logger
}

// entityKnowledge is the result of building entity context.
type entityKnowledge struct {
	content   string   // Formatted text to append to system prompt
	entityIDs []string // Entity IDs that were queried (for ContextEntities tracking)
}

// build constructs entity knowledge for the given quest and agent.
// Returns empty content if no meaningful entity data is available.
func (b *entityKnowledgeBuilder) build(ctx context.Context, quest *domain.Quest, agent *agentprogression.Agent) entityKnowledge {
	var sections []string
	var entityIDs []string

	// Agent identity — always include
	if s := b.formatAgentIdentity(agent); s != "" {
		sections = append(sections, s)
	}

	// Quest details — supplements what the assembler already provides
	if s := b.formatQuestDetails(quest); s != "" {
		sections = append(sections, s)
	}

	// Party context — load from graph if this is a party quest
	if quest.PartyID != nil {
		if s, ids := b.formatPartyContext(ctx, quest, agent); s != "" {
			sections = append(sections, s)
			entityIDs = append(entityIDs, ids...)
		}
	}

	// Guild context — load agent's guild if they belong to one
	if agent.Guild != "" {
		if s, ids := b.formatGuildContext(ctx, agent); s != "" {
			sections = append(sections, s)
			entityIDs = append(entityIDs, ids...)
		}
	}

	if len(sections) == 0 {
		return entityKnowledge{}
	}

	content := strings.Join(sections, "\n\n")

	// Truncate to budget if needed.
	if b.budgetToken > 0 && pkgcontext.EstimateTokens(content) > b.budgetToken {
		content = pkgcontext.TruncateToBudget(content, b.budgetToken)
	}

	return entityKnowledge{
		content:   content,
		entityIDs: entityIDs,
	}
}

// formatAgentIdentity formats the agent's identity, stats, and skills.
func (b *entityKnowledgeBuilder) formatAgentIdentity(agent *agentprogression.Agent) string {
	var sb strings.Builder
	sb.WriteString("--- Your Identity ---\n")

	name := agent.Name
	if agent.DisplayName != "" {
		name = agent.DisplayName
	}
	sb.WriteString(fmt.Sprintf("Name: %s\n", name))
	sb.WriteString(fmt.Sprintf("Level: %d (%s, Tier %d)\n", agent.Level, agent.Tier.String(), int(agent.Tier)))
	sb.WriteString(fmt.Sprintf("XP: %d/%d\n", agent.XP, agent.XPToLevel))

	// Skills with proficiency names
	if len(agent.SkillProficiencies) > 0 {
		var skills []string
		for skill, prof := range agent.SkillProficiencies {
			skills = append(skills, fmt.Sprintf("%s (%s)", skill, domain.ProficiencyLevelName(prof.Level)))
		}
		sb.WriteString(fmt.Sprintf("Skills: %s\n", strings.Join(skills, ", ")))
	}

	// Guild membership
	if agent.Guild != "" {
		sb.WriteString(fmt.Sprintf("Guild: %s\n", string(agent.Guild)))
	}

	// Track record
	if agent.Stats.QuestsCompleted > 0 || agent.Stats.QuestsFailed > 0 {
		sb.WriteString(fmt.Sprintf("Track Record: %d completed, %d failed",
			agent.Stats.QuestsCompleted, agent.Stats.QuestsFailed))
		if agent.Stats.PeerReviewCount > 0 {
			sb.WriteString(fmt.Sprintf(", peer review avg: %.1f/5 (%d reviews)",
				agent.Stats.PeerReviewAvg, agent.Stats.PeerReviewCount))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// formatQuestDetails formats quest metadata NOT already in the assembler's Quest section.
// The assembler handles: title, description, time limit, token budget, required skills.
// This adds: difficulty level, XP reward, acceptance criteria, attempt tracking.
func (b *entityKnowledgeBuilder) formatQuestDetails(quest *domain.Quest) string {
	var parts []string

	if quest.Difficulty > 0 {
		parts = append(parts, fmt.Sprintf("Difficulty: %d", quest.Difficulty))
	}
	if quest.BaseXP > 0 {
		parts = append(parts, fmt.Sprintf("Base XP Reward: %d", quest.BaseXP))
	}
	if len(quest.Acceptance) > 0 {
		var sb strings.Builder
		sb.WriteString("Acceptance Criteria:")
		for _, ac := range quest.Acceptance {
			sb.WriteString(fmt.Sprintf("\n  - %s", ac))
		}
		parts = append(parts, sb.String())
	}
	if quest.MaxAttempts > 0 {
		parts = append(parts, fmt.Sprintf("Attempt: %d of %d", quest.Attempts+1, quest.MaxAttempts))
	}

	if len(parts) == 0 {
		return ""
	}

	return "--- Quest Metadata ---\n" + strings.Join(parts, "\n") + "\n"
}

// formatPartyContext loads party data and formats it. For sub-quests, also
// includes the parent quest context.
func (b *entityKnowledgeBuilder) formatPartyContext(ctx context.Context, quest *domain.Quest, agent *agentprogression.Agent) (string, []string) {
	partyEntity, err := b.graph.GetParty(ctx, *quest.PartyID)
	if err != nil {
		b.logger.Debug("failed to load party for context", "party_id", *quest.PartyID, "error", err)
		return "", nil
	}
	party := partycoord.PartyFromEntityState(partyEntity)
	if party == nil {
		return "", nil
	}

	var entityIDs []string
	entityIDs = append(entityIDs, string(*quest.PartyID))

	var sb strings.Builder
	sb.WriteString("--- Party Context ---\n")
	sb.WriteString(fmt.Sprintf("Party: %s\n", party.Name))
	sb.WriteString(fmt.Sprintf("Lead: %s\n", domain.ExtractInstance(string(party.Lead))))

	if party.Strategy != "" {
		sb.WriteString(fmt.Sprintf("Strategy: %s\n", party.Strategy))
	}

	// Format members
	if len(party.Members) > 0 {
		sb.WriteString("Members:\n")
		for _, m := range party.Members {
			name := domain.ExtractInstance(string(m.AgentID))
			isYou := m.AgentID == agent.ID
			var skills []string
			for _, s := range m.Skills {
				skills = append(skills, string(s))
			}
			suffix := ""
			if isYou {
				suffix = " (you)"
			}
			if len(skills) > 0 {
				sb.WriteString(fmt.Sprintf("  - %s%s: %s\n", name, suffix, strings.Join(skills, ", ")))
			} else {
				sb.WriteString(fmt.Sprintf("  - %s%s\n", name, suffix))
			}
		}
	}

	// Parent quest context for sub-quests
	if quest.ParentQuest != nil {
		entityIDs = append(entityIDs, string(*quest.ParentQuest))
		parentEntity, err := b.graph.GetQuest(ctx, *quest.ParentQuest)
		if err == nil {
			parent := domain.QuestFromEntityState(parentEntity)
			if parent != nil && parent.Title != "" {
				sb.WriteString(fmt.Sprintf("\nParent Quest: %s\n", parent.Title))
				if parent.Description != "" {
					sb.WriteString(fmt.Sprintf("Parent Description: %s\n", parent.Description))
				}
			}
		}
	}

	return sb.String(), entityIDs
}

// formatGuildContext loads the agent's guild and formats it.
func (b *entityKnowledgeBuilder) formatGuildContext(ctx context.Context, agent *agentprogression.Agent) (string, []string) {
	guildID := agent.Guild
	guildEntity, err := b.graph.GetGuild(ctx, guildID)
	if err != nil {
		b.logger.Debug("failed to load guild for context", "guild_id", guildID, "error", err)
		return "", nil
	}
	guild := domain.GuildFromEntityState(guildEntity)
	if guild == nil {
		return "", nil
	}

	var sb strings.Builder
	sb.WriteString("--- Guild Context ---\n")
	sb.WriteString(fmt.Sprintf("Guild: %s\n", guild.Name))
	if guild.Description != "" {
		sb.WriteString(fmt.Sprintf("Description: %s\n", guild.Description))
	}
	if guild.Motto != "" {
		sb.WriteString(fmt.Sprintf("Motto: %s\n", guild.Motto))
	}
	sb.WriteString(fmt.Sprintf("Reputation: %.0f | Success Rate: %.0f%%\n", guild.Reputation, guild.SuccessRate*100))

	return sb.String(), []string{string(guildID)}
}
