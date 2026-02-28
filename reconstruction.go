// Package semdragons provides entity reconstruction from graph.EntityState.
// These helpers convert EntityState triples back into typed domain entities.
package semdragons

import (
	"strconv"
	"time"

	"github.com/c360studio/semstreams/graph"
)

// =============================================================================
// ENTITY RECONSTRUCTION HELPERS
// =============================================================================
// These functions reconstruct typed domain entities from graph.EntityState.
// EntityState stores data as Triples; these helpers parse them back to structs.
//
// Usage:
//   entity, err := gc.GetEntity(ctx, entityID)
//   quest := QuestFromEntityState(entity)
// =============================================================================

// -----------------------------------------------------------------------------
// QUEST RECONSTRUCTION
// -----------------------------------------------------------------------------

// QuestFromEntityState reconstructs a Quest from graph EntityState.
func QuestFromEntityState(entity *graph.EntityState) *Quest {
	if entity == nil {
		return nil
	}

	q := &Quest{
		ID: QuestID(entity.ID),
	}

	for _, triple := range entity.Triples {
		switch triple.Predicate {
		// Identity
		case "quest.identity.title":
			q.Title = asString(triple.Object)
		case "quest.identity.description":
			q.Description = asString(triple.Object)

		// Status
		case "quest.status.state":
			q.Status = QuestStatus(asString(triple.Object))
		case "quest.difficulty.level":
			q.Difficulty = QuestDifficulty(asInt(triple.Object))

		// Requirements
		case "quest.tier.minimum":
			q.MinTier = TrustTier(asInt(triple.Object))
		case "quest.party.required":
			q.PartyRequired = asBool(triple.Object)

		// Rewards
		case "quest.xp.base":
			q.BaseXP = asInt64(triple.Object)

		// Lifecycle
		case "quest.attempts.current":
			q.Attempts = asInt(triple.Object)
		case "quest.attempts.max":
			q.MaxAttempts = asInt(triple.Object)
		case "quest.lifecycle.posted_at":
			q.PostedAt = asTime(triple.Object)
		case "quest.lifecycle.claimed_at":
			t := asTime(triple.Object)
			q.ClaimedAt = &t
		case "quest.lifecycle.started_at":
			t := asTime(triple.Object)
			q.StartedAt = &t
		case "quest.lifecycle.completed_at":
			t := asTime(triple.Object)
			q.CompletedAt = &t

		// Relationships
		case "quest.assignment.agent":
			agentID := AgentID(asString(triple.Object))
			q.ClaimedBy = &agentID
		case "quest.assignment.party":
			partyID := PartyID(asString(triple.Object))
			q.PartyID = &partyID
		case "quest.priority.guild":
			guildID := GuildID(asString(triple.Object))
			q.GuildPriority = &guildID
		case "quest.parent.quest":
			parentID := QuestID(asString(triple.Object))
			q.ParentQuest = &parentID

		// Skills and tools (collected separately)
		case "quest.skill.required":
			q.RequiredSkills = append(q.RequiredSkills, SkillTag(asString(triple.Object)))
		case "quest.tool.required":
			q.RequiredTools = append(q.RequiredTools, asString(triple.Object))

		// Review
		case "quest.review.level":
			q.Constraints.ReviewLevel = ReviewLevel(asInt(triple.Object))

		// Observability
		case "quest.observability.trajectory_id":
			q.TrajectoryID = asString(triple.Object)
		}
	}

	return q
}

// -----------------------------------------------------------------------------
// AGENT RECONSTRUCTION
// -----------------------------------------------------------------------------

// AgentFromEntityState reconstructs an Agent from graph EntityState.
func AgentFromEntityState(entity *graph.EntityState) *Agent {
	if entity == nil {
		return nil
	}

	a := &Agent{
		ID:                 AgentID(entity.ID),
		SkillProficiencies: make(map[SkillTag]SkillProficiency),
	}

	for _, triple := range entity.Triples {
		switch triple.Predicate {
		// Identity
		case "agent.identity.name":
			a.Name = asString(triple.Object)
		case "agent.identity.display_name":
			a.DisplayName = asString(triple.Object)

		// Status
		case "agent.status.state":
			a.Status = AgentStatus(asString(triple.Object))
		case "agent.npc.flag":
			a.IsNPC = asBool(triple.Object)
		case "agent.status.cooldown_until":
			t := asTime(triple.Object)
			a.CooldownUntil = &t

		// Progression
		case "agent.progression.level":
			a.Level = asInt(triple.Object)
		case "agent.progression.xp.current":
			a.XP = asInt64(triple.Object)
		case "agent.progression.xp.to_level":
			a.XPToLevel = asInt64(triple.Object)
		case "agent.progression.tier":
			a.Tier = TrustTier(asInt(triple.Object))
		case "agent.progression.death_count":
			a.DeathCount = asInt(triple.Object)

		// Stats
		case "agent.stats.quests_completed":
			a.Stats.QuestsCompleted = asInt(triple.Object)
		case "agent.stats.quests_failed":
			a.Stats.QuestsFailed = asInt(triple.Object)
		case "agent.stats.bosses_defeated":
			a.Stats.BossesDefeated = asInt(triple.Object)
		case "agent.stats.total_xp_earned":
			a.Stats.TotalXPEarned = asInt64(triple.Object)

		// Lifecycle
		case "agent.lifecycle.created_at":
			a.CreatedAt = asTime(triple.Object)
		case "agent.lifecycle.updated_at":
			a.UpdatedAt = asTime(triple.Object)

		// Relationships
		case "agent.membership.guild":
			a.Guilds = append(a.Guilds, GuildID(asString(triple.Object)))
		case "agent.assignment.quest":
			questID := QuestID(asString(triple.Object))
			a.CurrentQuest = &questID
		case "agent.membership.party":
			partyID := PartyID(asString(triple.Object))
			a.CurrentParty = &partyID
		}

		// Handle skill proficiencies (dynamic predicates)
		// Format: agent.skill.{skill}.level or agent.skill.{skill}.total_xp
		if len(triple.Predicate) > 12 && triple.Predicate[:12] == "agent.skill." {
			rest := triple.Predicate[12:] // e.g., "coding.level" or "coding.total_xp"
			for i := len(rest) - 1; i >= 0; i-- {
				if rest[i] == '.' {
					skillTag := SkillTag(rest[:i])
					suffix := rest[i+1:]

					prof := a.SkillProficiencies[skillTag]
					switch suffix {
					case "level":
						prof.Level = ProficiencyLevel(asInt(triple.Object))
					case "total_xp":
						prof.TotalXP = asInt64(triple.Object)
					}
					a.SkillProficiencies[skillTag] = prof
					break
				}
			}
		}
	}

	return a
}

// -----------------------------------------------------------------------------
// BOSS BATTLE RECONSTRUCTION
// -----------------------------------------------------------------------------

// BattleFromEntityState reconstructs a BossBattle from graph EntityState.
func BattleFromEntityState(entity *graph.EntityState) *BossBattle {
	if entity == nil {
		return nil
	}

	b := &BossBattle{
		ID: BattleID(entity.ID),
	}

	var judgeIDs []string

	for _, triple := range entity.Triples {
		switch triple.Predicate {
		// Relationships
		case "battle.assignment.quest":
			b.QuestID = QuestID(asString(triple.Object))
		case "battle.assignment.agent":
			b.AgentID = AgentID(asString(triple.Object))

		// Status
		case "battle.status.state":
			b.Status = BattleStatus(asString(triple.Object))
		case "battle.review.level":
			b.Level = ReviewLevel(asInt(triple.Object))

		// Lifecycle
		case "battle.lifecycle.started_at":
			b.StartedAt = asTime(triple.Object)
		case "battle.lifecycle.completed_at":
			t := asTime(triple.Object)
			b.CompletedAt = &t

		// Verdict
		case "battle.verdict.passed":
			if b.Verdict == nil {
				b.Verdict = &BattleVerdict{}
			}
			b.Verdict.Passed = asBool(triple.Object)
		case "battle.verdict.score":
			if b.Verdict == nil {
				b.Verdict = &BattleVerdict{}
			}
			b.Verdict.QualityScore = asFloat64(triple.Object)
		case "battle.verdict.xp_awarded":
			if b.Verdict == nil {
				b.Verdict = &BattleVerdict{}
			}
			b.Verdict.XPAwarded = asInt64(triple.Object)
		case "battle.verdict.feedback":
			if b.Verdict == nil {
				b.Verdict = &BattleVerdict{}
			}
			b.Verdict.Feedback = asString(triple.Object)

		// Judges
		case "battle.judge.id":
			judgeIDs = append(judgeIDs, asString(triple.Object))
		}
	}

	// Reconstruct judges (we only store IDs in triples)
	for _, id := range judgeIDs {
		b.Judges = append(b.Judges, Judge{ID: id})
	}

	return b
}

// -----------------------------------------------------------------------------
// PARTY RECONSTRUCTION
// -----------------------------------------------------------------------------

// PartyFromEntityState reconstructs a Party from graph EntityState.
func PartyFromEntityState(entity *graph.EntityState) *Party {
	if entity == nil {
		return nil
	}

	p := &Party{
		ID:          PartyID(entity.ID),
		SubQuestMap: make(map[QuestID]AgentID),
	}

	// Track member data by agent ID for reconstruction
	memberRoles := make(map[AgentID]PartyRole)

	for _, triple := range entity.Triples {
		switch triple.Predicate {
		// Identity
		case "party.identity.name":
			p.Name = asString(triple.Object)

		// Status
		case "party.status.state":
			p.Status = PartyStatus(asString(triple.Object))

		// Relationships
		case "party.assignment.quest":
			p.QuestID = QuestID(asString(triple.Object))
		case "party.membership.lead":
			p.Lead = AgentID(asString(triple.Object))
		case "party.membership.member":
			agentID := AgentID(asString(triple.Object))
			memberRoles[agentID] = "" // Will be filled by role triple

		// Coordination
		case "party.coordination.strategy":
			p.Strategy = asString(triple.Object)

		// Lifecycle
		case "party.lifecycle.formed_at":
			p.FormedAt = asTime(triple.Object)
		case "party.lifecycle.disbanded_at":
			t := asTime(triple.Object)
			p.DisbandedAt = &t
		}

		// Handle dynamic predicates for member roles
		// Format: party.member.{agent_id}.role
		if len(triple.Predicate) > 13 && triple.Predicate[:13] == "party.member." {
			rest := triple.Predicate[13:] // e.g., "agent123.role"
			for i := len(rest) - 1; i >= 0; i-- {
				if rest[i] == '.' {
					agentID := AgentID(rest[:i])
					suffix := rest[i+1:]

					if suffix == "role" {
						memberRoles[agentID] = PartyRole(asString(triple.Object))
					}
					break
				}
			}
		}

		// Handle subquest assignments
		// Format: party.subquest.{quest_id}.agent
		if len(triple.Predicate) > 15 && triple.Predicate[:15] == "party.subquest." {
			rest := triple.Predicate[15:] // e.g., "quest123.agent"
			for i := len(rest) - 1; i >= 0; i-- {
				if rest[i] == '.' {
					questID := QuestID(rest[:i])
					suffix := rest[i+1:]

					if suffix == "agent" {
						p.SubQuestMap[questID] = AgentID(asString(triple.Object))
					}
					break
				}
			}
		}
	}

	// Reconstruct members from collected data
	for agentID, role := range memberRoles {
		p.Members = append(p.Members, PartyMember{
			AgentID: agentID,
			Role:    role,
		})
	}

	return p
}

// -----------------------------------------------------------------------------
// GUILD RECONSTRUCTION
// -----------------------------------------------------------------------------

// GuildFromEntityState reconstructs a Guild from graph EntityState.
func GuildFromEntityState(entity *graph.EntityState) *Guild {
	if entity == nil {
		return nil
	}

	g := &Guild{
		ID: GuildID(entity.ID),
	}

	// Track member data by agent ID for reconstruction
	memberData := make(map[AgentID]*GuildMember)

	for _, triple := range entity.Triples {
		switch triple.Predicate {
		// Identity
		case "guild.identity.name":
			g.Name = asString(triple.Object)
		case "guild.identity.description":
			g.Description = asString(triple.Object)
		case "guild.identity.culture":
			g.Culture = asString(triple.Object)
		case "guild.identity.motto":
			g.Motto = asString(triple.Object)

		// Status
		case "guild.status.state":
			g.Status = GuildStatus(asString(triple.Object))

		// Configuration
		case "guild.config.max_members":
			g.MaxMembers = asInt(triple.Object)
		case "guild.config.min_level":
			g.MinLevel = asInt(triple.Object)

		// Founding
		case "guild.founding.date":
			g.Founded = asTime(triple.Object)
		case "guild.founding.agent":
			g.FoundedBy = AgentID(asString(triple.Object))

		// Stats
		case "guild.stats.reputation":
			g.Reputation = asFloat64(triple.Object)
		case "guild.stats.quests_handled":
			g.QuestsHandled = asInt(triple.Object)
		case "guild.stats.quests_failed":
			g.QuestsFailed = asInt(triple.Object)
		case "guild.stats.success_rate":
			g.SuccessRate = asFloat64(triple.Object)

		// Lifecycle
		case "guild.lifecycle.created_at":
			g.CreatedAt = asTime(triple.Object)

		// Membership
		case "guild.membership.agent":
			agentID := AgentID(asString(triple.Object))
			if memberData[agentID] == nil {
				memberData[agentID] = &GuildMember{AgentID: agentID}
			}

		// Resources
		case "guild.resource.tool":
			g.SharedTools = append(g.SharedTools, asString(triple.Object))

		// Routing
		case "guild.routing.quest_type":
			g.QuestTypes = append(g.QuestTypes, asString(triple.Object))
		case "guild.routing.preferred_client":
			g.PreferredClients = append(g.PreferredClients, asString(triple.Object))
		}

		// Handle dynamic predicates for member rank/contribution
		// Format: guild.member.{agent_id}.rank or guild.member.{agent_id}.contribution
		if len(triple.Predicate) > 13 && triple.Predicate[:13] == "guild.member." {
			rest := triple.Predicate[13:] // e.g., "agent123.rank"
			for i := len(rest) - 1; i >= 0; i-- {
				if rest[i] == '.' {
					agentID := AgentID(rest[:i])
					suffix := rest[i+1:]

					if memberData[agentID] == nil {
						memberData[agentID] = &GuildMember{AgentID: agentID}
					}

					switch suffix {
					case "rank":
						memberData[agentID].Rank = GuildRank(asString(triple.Object))
					case "contribution":
						memberData[agentID].Contribution = asFloat64(triple.Object)
					}
					break
				}
			}
		}
	}

	// Reconstruct members from collected data
	for _, member := range memberData {
		g.Members = append(g.Members, *member)
	}

	return g
}

// =============================================================================
// TYPE CONVERSION HELPERS
// =============================================================================
// These helpers safely convert interface{} values from triples to Go types.
// =============================================================================

func asString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	default:
		return ""
	}
}

func asInt(v interface{}) int {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		i, _ := strconv.Atoi(val)
		return i
	default:
		return 0
	}
}

func asInt64(v interface{}) int64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case int64:
		return val
	case int:
		return int64(val)
	case float64:
		return int64(val)
	case string:
		i, _ := strconv.ParseInt(val, 10, 64)
		return i
	default:
		return 0
	}
}

func asFloat64(v interface{}) float64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	default:
		return 0
	}
}

func asBool(v interface{}) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val == "true"
	default:
		return false
	}
}

func asTime(v interface{}) time.Time {
	if v == nil {
		return time.Time{}
	}
	switch val := v.(type) {
	case time.Time:
		return val
	case string:
		t, err := time.Parse(time.RFC3339, val)
		if err != nil {
			return time.Time{}
		}
		return t
	default:
		return time.Time{}
	}
}
