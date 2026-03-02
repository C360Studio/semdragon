// Package semdragons provides Graphable implementations for domain entities.
// These implementations enable entities to be stored in the semstreams graph system.
package semdragons

import (
	"fmt"
	"time"

	"github.com/c360studio/semstreams/message"
)

// =============================================================================
// GRAPHABLE IMPLEMENTATIONS
// =============================================================================
// Each entity type implements graph.Graphable interface:
// - EntityID() string - Returns 6-part federated entity ID
// - Triples() []message.Triple - Returns semantic facts about the entity
//
// Note: We return message.Triple (not graph.Triple) as that's what the
// Graphable interface expects.
// =============================================================================

// -----------------------------------------------------------------------------
// QUEST - Graphable implementation
// -----------------------------------------------------------------------------

// EntityID returns the 6-part entity ID for this quest.
// Format: org.platform.game.board.quest.instance
// Note: Quest must have been created with a properly formatted ID that
// includes the full entity ID path.
func (q *Quest) EntityID() string {
	return string(q.ID)
}

// Triples returns all semantic facts about this quest.
func (q *Quest) Triples() []message.Triple {
	now := time.Now()
	source := "questboard"
	entityID := q.EntityID()

	triples := []message.Triple{
		// Identity
		{Subject: entityID, Predicate: "quest.identity.title", Object: q.Title, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "quest.identity.description", Object: q.Description, Source: source, Timestamp: now, Confidence: 1.0},

		// Status
		{Subject: entityID, Predicate: "quest.status.state", Object: string(q.Status), Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "quest.difficulty.level", Object: int(q.Difficulty), Source: source, Timestamp: now, Confidence: 1.0},

		// Requirements
		{Subject: entityID, Predicate: "quest.tier.minimum", Object: int(q.MinTier), Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "quest.party.required", Object: q.PartyRequired, Source: source, Timestamp: now, Confidence: 1.0},

		// Rewards
		{Subject: entityID, Predicate: "quest.xp.base", Object: q.BaseXP, Source: source, Timestamp: now, Confidence: 1.0},

		// Lifecycle
		{Subject: entityID, Predicate: "quest.attempts.current", Object: q.Attempts, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "quest.attempts.max", Object: q.MaxAttempts, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "quest.lifecycle.posted_at", Object: q.PostedAt.Format(time.RFC3339), Source: source, Timestamp: now, Confidence: 1.0},
	}

	// Add skills as separate triples
	for _, skill := range q.RequiredSkills {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.skill.required", Object: string(skill),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Add tools as separate triples
	for _, tool := range q.RequiredTools {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.tool.required", Object: tool,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Optional relationships
	if q.ClaimedBy != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.assignment.agent", Object: string(*q.ClaimedBy),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	if q.PartyID != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.assignment.party", Object: string(*q.PartyID),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	if q.GuildPriority != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.priority.guild", Object: string(*q.GuildPriority),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	if q.ParentQuest != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.parent.quest", Object: string(*q.ParentQuest),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	if q.ClaimedAt != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.lifecycle.claimed_at", Object: q.ClaimedAt.Format(time.RFC3339),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	if q.StartedAt != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.lifecycle.started_at", Object: q.StartedAt.Format(time.RFC3339),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	if q.CompletedAt != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.lifecycle.completed_at", Object: q.CompletedAt.Format(time.RFC3339),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	if q.TrajectoryID != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.observability.trajectory_id", Object: q.TrajectoryID,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Review
	triples = append(triples, message.Triple{
		Subject: entityID, Predicate: "quest.review.level", Object: int(q.Constraints.ReviewLevel),
		Source: source, Timestamp: now, Confidence: 1.0,
	})
	triples = append(triples, message.Triple{
		Subject: entityID, Predicate: "quest.review.needs_review", Object: q.Constraints.RequireReview,
		Source: source, Timestamp: now, Confidence: 1.0,
	})

	return triples
}

// -----------------------------------------------------------------------------
// AGENT - Graphable implementation
// -----------------------------------------------------------------------------

// EntityID returns the 6-part entity ID for this agent.
func (a *Agent) EntityID() string {
	return string(a.ID)
}

// Triples returns all semantic facts about this agent.
func (a *Agent) Triples() []message.Triple {
	now := time.Now()
	source := "xpengine"
	entityID := a.EntityID()

	triples := []message.Triple{
		// Identity
		{Subject: entityID, Predicate: "agent.identity.name", Object: a.Name, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.identity.display_name", Object: a.DisplayName, Source: source, Timestamp: now, Confidence: 1.0},

		// Status
		{Subject: entityID, Predicate: "agent.status.state", Object: string(a.Status), Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.npc.flag", Object: a.IsNPC, Source: source, Timestamp: now, Confidence: 1.0},

		// Progression
		{Subject: entityID, Predicate: "agent.progression.level", Object: a.Level, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.progression.xp.current", Object: a.XP, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.progression.xp.to_level", Object: a.XPToLevel, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.progression.tier", Object: int(a.Tier), Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.progression.death_count", Object: a.DeathCount, Source: source, Timestamp: now, Confidence: 1.0},

		// Stats
		{Subject: entityID, Predicate: "agent.stats.quests_completed", Object: a.Stats.QuestsCompleted, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.stats.quests_failed", Object: a.Stats.QuestsFailed, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.stats.bosses_defeated", Object: a.Stats.BossesDefeated, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.stats.total_xp_earned", Object: a.Stats.TotalXPEarned, Source: source, Timestamp: now, Confidence: 1.0},

		// Lifecycle
		{Subject: entityID, Predicate: "agent.lifecycle.created_at", Object: a.CreatedAt.Format(time.RFC3339), Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "agent.lifecycle.updated_at", Object: a.UpdatedAt.Format(time.RFC3339), Source: source, Timestamp: now, Confidence: 1.0},
	}

	// Guild memberships
	for _, guildID := range a.Guilds {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "agent.membership.guild", Object: string(guildID),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Skill proficiencies
	for skill, prof := range a.SkillProficiencies {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: fmt.Sprintf("agent.skill.%s.level", skill), Object: int(prof.Level),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: fmt.Sprintf("agent.skill.%s.total_xp", skill), Object: prof.TotalXP,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Optional relationships
	if a.CurrentQuest != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "agent.assignment.quest", Object: string(*a.CurrentQuest),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	if a.CurrentParty != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "agent.membership.party", Object: string(*a.CurrentParty),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	if a.CooldownUntil != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "agent.status.cooldown_until", Object: a.CooldownUntil.Format(time.RFC3339),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Owned tools — each tool creates a relationship edge to its storeitem entity
	for itemID, tool := range a.OwnedTools {
		prefix := fmt.Sprintf("agent.inventory.tool.%s", itemID)
		triples = append(triples,
			message.Triple{Subject: entityID, Predicate: prefix, Object: tool.StoreItemID, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: prefix + ".xp_spent", Object: tool.XPSpent, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: prefix + ".uses", Object: tool.UsesRemaining, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: prefix + ".purchased_at", Object: tool.PurchasedAt.Format(time.RFC3339), Source: source, Timestamp: now, Confidence: 1.0},
		)
	}

	// Consumables — count owned per item
	for itemID, count := range a.Consumables {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: fmt.Sprintf("agent.inventory.consumable.%s", itemID), Object: count,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Total XP spent in store
	if a.TotalSpent > 0 {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "agent.inventory.total_spent", Object: a.TotalSpent,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Active consumable effects
	for _, eff := range a.ActiveEffects {
		prefix := fmt.Sprintf("agent.effects.%s", eff.EffectType)
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: prefix + ".remaining", Object: eff.QuestsRemaining,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
		if eff.QuestID != nil {
			triples = append(triples, message.Triple{
				Subject: entityID, Predicate: prefix + ".quest", Object: string(*eff.QuestID),
				Source: source, Timestamp: now, Confidence: 1.0,
			})
		}
	}

	return triples
}

// -----------------------------------------------------------------------------
// BOSS BATTLE - Graphable implementation
// -----------------------------------------------------------------------------

// EntityID returns the 6-part entity ID for this battle.
func (b *BossBattle) EntityID() string {
	return string(b.ID)
}

// Triples returns all semantic facts about this battle.
func (b *BossBattle) Triples() []message.Triple {
	now := time.Now()
	source := "bossbattle"
	entityID := b.EntityID()

	triples := []message.Triple{
		// Relationships
		{Subject: entityID, Predicate: "battle.assignment.quest", Object: string(b.QuestID), Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "battle.assignment.agent", Object: string(b.AgentID), Source: source, Timestamp: now, Confidence: 1.0},

		// Status
		{Subject: entityID, Predicate: "battle.status.state", Object: string(b.Status), Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "battle.review.level", Object: int(b.Level), Source: source, Timestamp: now, Confidence: 1.0},

		// Lifecycle
		{Subject: entityID, Predicate: "battle.lifecycle.started_at", Object: b.StartedAt.Format(time.RFC3339), Source: source, Timestamp: now, Confidence: 1.0},
	}

	if b.CompletedAt != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "battle.lifecycle.completed_at", Object: b.CompletedAt.Format(time.RFC3339),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Verdict if available
	if b.Verdict != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "battle.verdict.passed", Object: b.Verdict.Passed,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "battle.verdict.score", Object: b.Verdict.QualityScore,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "battle.verdict.xp_awarded", Object: b.Verdict.XPAwarded,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
		if b.Verdict.Feedback != "" {
			triples = append(triples, message.Triple{
				Subject: entityID, Predicate: "battle.verdict.feedback", Object: b.Verdict.Feedback,
				Source: source, Timestamp: now, Confidence: 1.0,
			})
		}
	}

	// Judges
	for _, judge := range b.Judges {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "battle.judge.id", Object: judge.ID,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	return triples
}

// -----------------------------------------------------------------------------
// PARTY - Graphable implementation
// -----------------------------------------------------------------------------

// EntityID returns the 6-part entity ID for this party.
func (p *Party) EntityID() string {
	return string(p.ID)
}

// Triples returns all semantic facts about this party.
func (p *Party) Triples() []message.Triple {
	now := time.Now()
	source := "guildformation"
	entityID := p.EntityID()

	triples := []message.Triple{
		// Identity
		{Subject: entityID, Predicate: "party.identity.name", Object: p.Name, Source: source, Timestamp: now, Confidence: 1.0},

		// Status
		{Subject: entityID, Predicate: "party.status.state", Object: string(p.Status), Source: source, Timestamp: now, Confidence: 1.0},

		// Relationships
		{Subject: entityID, Predicate: "party.assignment.quest", Object: string(p.QuestID), Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "party.membership.lead", Object: string(p.Lead), Source: source, Timestamp: now, Confidence: 1.0},

		// Lifecycle
		{Subject: entityID, Predicate: "party.lifecycle.formed_at", Object: p.FormedAt.Format(time.RFC3339), Source: source, Timestamp: now, Confidence: 1.0},
	}

	// Strategy if set
	if p.Strategy != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "party.coordination.strategy", Object: p.Strategy,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Members
	for _, member := range p.Members {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "party.membership.member", Object: string(member.AgentID),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: fmt.Sprintf("party.member.%s.role", member.AgentID), Object: string(member.Role),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Sub-quest assignments
	for questID, agentID := range p.SubQuestMap {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: fmt.Sprintf("party.subquest.%s.agent", questID), Object: string(agentID),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Disbanded timestamp if set
	if p.DisbandedAt != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "party.lifecycle.disbanded_at", Object: p.DisbandedAt.Format(time.RFC3339),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	return triples
}

// -----------------------------------------------------------------------------
// GUILD - Graphable implementation
// -----------------------------------------------------------------------------

// EntityID returns the 6-part entity ID for this guild.
func (g *Guild) EntityID() string {
	return string(g.ID)
}

// Triples returns all semantic facts about this guild.
func (g *Guild) Triples() []message.Triple {
	now := time.Now()
	source := "guildformation"
	entityID := g.EntityID()

	triples := []message.Triple{
		// Identity
		{Subject: entityID, Predicate: "guild.identity.name", Object: g.Name, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "guild.identity.description", Object: g.Description, Source: source, Timestamp: now, Confidence: 1.0},

		// Status
		{Subject: entityID, Predicate: "guild.status.state", Object: string(g.Status), Source: source, Timestamp: now, Confidence: 1.0},

		// Configuration
		{Subject: entityID, Predicate: "guild.config.max_members", Object: g.MaxMembers, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "guild.config.min_level", Object: g.MinLevel, Source: source, Timestamp: now, Confidence: 1.0},

		// Founding
		{Subject: entityID, Predicate: "guild.founding.date", Object: g.Founded.Format(time.RFC3339), Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "guild.founding.agent", Object: string(g.FoundedBy), Source: source, Timestamp: now, Confidence: 1.0},

		// Stats
		{Subject: entityID, Predicate: "guild.stats.reputation", Object: g.Reputation, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "guild.stats.quests_handled", Object: g.QuestsHandled, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "guild.stats.quests_failed", Object: g.QuestsFailed, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "guild.stats.success_rate", Object: g.SuccessRate, Source: source, Timestamp: now, Confidence: 1.0},

		// Lifecycle
		{Subject: entityID, Predicate: "guild.lifecycle.created_at", Object: g.CreatedAt.Format(time.RFC3339), Source: source, Timestamp: now, Confidence: 1.0},
	}

	// Culture and motto
	if g.Culture != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "guild.identity.culture", Object: g.Culture,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	if g.Motto != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "guild.identity.motto", Object: g.Motto,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Members
	for _, member := range g.Members {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "guild.membership.agent", Object: string(member.AgentID),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: fmt.Sprintf("guild.member.%s.rank", member.AgentID), Object: string(member.Rank),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: fmt.Sprintf("guild.member.%s.contribution", member.AgentID), Object: member.Contribution,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Shared tools
	for _, toolID := range g.SharedTools {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "guild.resource.tool", Object: toolID,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Quest types
	for _, questType := range g.QuestTypes {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "guild.routing.quest_type", Object: questType,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Preferred clients
	for _, client := range g.PreferredClients {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "guild.routing.preferred_client", Object: client,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	return triples
}
