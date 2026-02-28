package semdragons

import (
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/pkg/types"
)

// =============================================================================
// GRAPHABLE PAYLOADS - Event payloads implementing the Graphable interface
// =============================================================================
// All event payloads implement graph.Graphable so they can be automatically
// stored in the ENTITY_STATES KV by graph-ingest. This eliminates manual
// KV writes and index management - the graph system handles everything.
//
// Each payload provides:
//   - EntityID(): 6-part federated ID (org.platform.game.board.type.instance)
//   - Triples(): All semantic facts about the entity
//   - Schema(): Message type for payload registry
//   - Validate(): Input validation
// =============================================================================

// Ensure Graphable implementations
var (
	_ graph.Graphable = (*QuestPostedPayload)(nil)
	_ graph.Graphable = (*QuestClaimedPayload)(nil)
	_ graph.Graphable = (*QuestStartedPayload)(nil)
	_ graph.Graphable = (*QuestSubmittedPayload)(nil)
	_ graph.Graphable = (*QuestCompletedPayload)(nil)
	_ graph.Graphable = (*QuestFailedPayload)(nil)
	_ graph.Graphable = (*QuestEscalatedPayload)(nil)
	_ graph.Graphable = (*QuestAbandonedPayload)(nil)
	_ graph.Graphable = (*BattleStartedPayload)(nil)
	_ graph.Graphable = (*BattleVerdictPayload)(nil)
	_ graph.Graphable = (*AgentXPPayload)(nil)
	_ graph.Graphable = (*AgentLevelPayload)(nil)
	_ graph.Graphable = (*AgentCooldownPayload)(nil)
	_ graph.Graphable = (*GuildSuggestedPayload)(nil)
	_ graph.Graphable = (*GuildAutoJoinedPayload)(nil)
)

// =============================================================================
// QUEST LIFECYCLE - Graphable implementations
// =============================================================================

// EntityID returns the 6-part federated entity ID for the quest.
func (p *QuestPostedPayload) EntityID() string {
	return string(p.Quest.ID)
}

// Triples returns all semantic facts about the quest entity.
func (p *QuestPostedPayload) Triples() []message.Triple {
	return questToTriples(&p.Quest, p.PostedAt)
}

// Schema returns the message type for payload registry.
func (p *QuestPostedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "quest.posted", Version: "v1"}
}

// EntityID returns the 6-part federated entity ID for the quest.
func (p *QuestClaimedPayload) EntityID() string {
	return string(p.Quest.ID)
}

// Triples returns all semantic facts about the quest entity after claiming.
func (p *QuestClaimedPayload) Triples() []message.Triple {
	triples := questToTriples(&p.Quest, p.ClaimedAt)
	id := string(p.Quest.ID)

	// Add claim-specific triples
	triples = append(triples,
		message.Triple{
			Subject:    id,
			Predicate:  "quest.assignment.claimed_by",
			Object:     string(p.AgentID),
			Timestamp:  p.ClaimedAt,
			Confidence: 1.0,
			Source:     "questboard",
		},
	)

	if p.PartyID != nil {
		triples = append(triples, message.Triple{
			Subject:    id,
			Predicate:  "quest.assignment.party_id",
			Object:     string(*p.PartyID),
			Timestamp:  p.ClaimedAt,
			Confidence: 1.0,
			Source:     "questboard",
		})
	}

	return triples
}

// Schema returns the message type for payload registry.
func (p *QuestClaimedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "quest.claimed", Version: "v1"}
}

// EntityID returns the 6-part federated entity ID for the quest.
func (p *QuestStartedPayload) EntityID() string {
	return string(p.Quest.ID)
}

// Triples returns all semantic facts about the quest entity after starting.
func (p *QuestStartedPayload) Triples() []message.Triple {
	return questToTriples(&p.Quest, p.StartedAt)
}

// Schema returns the message type for payload registry.
func (p *QuestStartedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "quest.started", Version: "v1"}
}

// EntityID returns the 6-part federated entity ID for the quest.
func (p *QuestSubmittedPayload) EntityID() string {
	return string(p.Quest.ID)
}

// Triples returns all semantic facts about the quest entity after submission.
func (p *QuestSubmittedPayload) Triples() []message.Triple {
	triples := questToTriples(&p.Quest, p.SubmittedAt)
	id := string(p.Quest.ID)

	if p.BattleID != nil {
		triples = append(triples, message.Triple{
			Subject:    id,
			Predicate:  "quest.review.battle_id",
			Object:     string(*p.BattleID),
			Timestamp:  p.SubmittedAt,
			Confidence: 1.0,
			Source:     "questboard",
		})
	}

	return triples
}

// Schema returns the message type for payload registry.
func (p *QuestSubmittedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "quest.submitted", Version: "v1"}
}

// EntityID returns the 6-part federated entity ID for the quest.
func (p *QuestCompletedPayload) EntityID() string {
	return string(p.Quest.ID)
}

// Triples returns all semantic facts about the quest entity after completion.
func (p *QuestCompletedPayload) Triples() []message.Triple {
	triples := questToTriples(&p.Quest, p.CompletedAt)
	id := string(p.Quest.ID)

	// Add completion-specific triples
	triples = append(triples,
		message.Triple{
			Subject:    id,
			Predicate:  "quest.result.duration_ns",
			Object:     int64(p.Duration),
			Timestamp:  p.CompletedAt,
			Confidence: 1.0,
			Source:     "questboard",
		},
		message.Triple{
			Subject:    id,
			Predicate:  "quest.result.quality_score",
			Object:     p.Verdict.QualityScore,
			Timestamp:  p.CompletedAt,
			Confidence: 1.0,
			Source:     "questboard",
		},
		message.Triple{
			Subject:    id,
			Predicate:  "quest.result.xp_awarded",
			Object:     p.Verdict.XPAwarded,
			Timestamp:  p.CompletedAt,
			Confidence: 1.0,
			Source:     "questboard",
		},
	)

	return triples
}

// Schema returns the message type for payload registry.
func (p *QuestCompletedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "quest.completed", Version: "v1"}
}

// EntityID returns the 6-part federated entity ID for the quest.
func (p *QuestFailedPayload) EntityID() string {
	return string(p.Quest.ID)
}

// Triples returns all semantic facts about the quest entity after failure.
func (p *QuestFailedPayload) Triples() []message.Triple {
	triples := questToTriples(&p.Quest, p.FailedAt)
	id := string(p.Quest.ID)

	// Add failure-specific triples
	triples = append(triples,
		message.Triple{
			Subject:    id,
			Predicate:  "quest.failure.reason",
			Object:     p.Reason,
			Timestamp:  p.FailedAt,
			Confidence: 1.0,
			Source:     "questboard",
		},
		message.Triple{
			Subject:    id,
			Predicate:  "quest.failure.type",
			Object:     string(p.FailType),
			Timestamp:  p.FailedAt,
			Confidence: 1.0,
			Source:     "questboard",
		},
		message.Triple{
			Subject:    id,
			Predicate:  "quest.failure.attempt",
			Object:     p.Attempt,
			Timestamp:  p.FailedAt,
			Confidence: 1.0,
			Source:     "questboard",
		},
		message.Triple{
			Subject:    id,
			Predicate:  "quest.failure.reposted",
			Object:     p.Reposted,
			Timestamp:  p.FailedAt,
			Confidence: 1.0,
			Source:     "questboard",
		},
	)

	return triples
}

// Schema returns the message type for payload registry.
func (p *QuestFailedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "quest.failed", Version: "v1"}
}

// EntityID returns the 6-part federated entity ID for the quest.
func (p *QuestEscalatedPayload) EntityID() string {
	return string(p.Quest.ID)
}

// Triples returns all semantic facts about the quest entity after escalation.
func (p *QuestEscalatedPayload) Triples() []message.Triple {
	triples := questToTriples(&p.Quest, p.EscalatedAt)
	id := string(p.Quest.ID)

	triples = append(triples,
		message.Triple{
			Subject:    id,
			Predicate:  "quest.escalation.reason",
			Object:     p.Reason,
			Timestamp:  p.EscalatedAt,
			Confidence: 1.0,
			Source:     "questboard",
		},
		message.Triple{
			Subject:    id,
			Predicate:  "quest.escalation.attempts",
			Object:     p.Attempts,
			Timestamp:  p.EscalatedAt,
			Confidence: 1.0,
			Source:     "questboard",
		},
	)

	return triples
}

// Schema returns the message type for payload registry.
func (p *QuestEscalatedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "quest.escalated", Version: "v1"}
}

// EntityID returns the 6-part federated entity ID for the quest.
func (p *QuestAbandonedPayload) EntityID() string {
	return string(p.Quest.ID)
}

// Triples returns all semantic facts about the quest entity after abandonment.
func (p *QuestAbandonedPayload) Triples() []message.Triple {
	triples := questToTriples(&p.Quest, p.AbandonedAt)
	id := string(p.Quest.ID)

	triples = append(triples,
		message.Triple{
			Subject:    id,
			Predicate:  "quest.abandonment.reason",
			Object:     p.Reason,
			Timestamp:  p.AbandonedAt,
			Confidence: 1.0,
			Source:     "questboard",
		},
	)

	return triples
}

// Schema returns the message type for payload registry.
func (p *QuestAbandonedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "quest.abandoned", Version: "v1"}
}

// =============================================================================
// BOSS BATTLE - Graphable implementations
// =============================================================================

// EntityID returns the 6-part federated entity ID for the battle.
func (p *BattleStartedPayload) EntityID() string {
	return string(p.Battle.ID)
}

// Triples returns all semantic facts about the battle entity.
func (p *BattleStartedPayload) Triples() []message.Triple {
	return battleToTriples(&p.Battle, p.StartedAt)
}

// Schema returns the message type for payload registry.
func (p *BattleStartedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "battle.started", Version: "v1"}
}

// EntityID returns the 6-part federated entity ID for the battle.
func (p *BattleVerdictPayload) EntityID() string {
	return string(p.Battle.ID)
}

// Triples returns all semantic facts about the battle entity after verdict.
func (p *BattleVerdictPayload) Triples() []message.Triple {
	triples := battleToTriples(&p.Battle, p.EndedAt)
	id := string(p.Battle.ID)

	// Add verdict-specific triples
	triples = append(triples,
		message.Triple{
			Subject:    id,
			Predicate:  "battle.verdict.passed",
			Object:     p.Verdict.Passed,
			Timestamp:  p.EndedAt,
			Confidence: 1.0,
			Source:     "bossbattle",
		},
		message.Triple{
			Subject:    id,
			Predicate:  "battle.verdict.quality_score",
			Object:     p.Verdict.QualityScore,
			Timestamp:  p.EndedAt,
			Confidence: 1.0,
			Source:     "bossbattle",
		},
		message.Triple{
			Subject:    id,
			Predicate:  "battle.verdict.xp_awarded",
			Object:     p.Verdict.XPAwarded,
			Timestamp:  p.EndedAt,
			Confidence: 1.0,
			Source:     "bossbattle",
		},
		message.Triple{
			Subject:    id,
			Predicate:  "battle.verdict.xp_penalty",
			Object:     p.Verdict.XPPenalty,
			Timestamp:  p.EndedAt,
			Confidence: 1.0,
			Source:     "bossbattle",
		},
		message.Triple{
			Subject:    id,
			Predicate:  "battle.verdict.feedback",
			Object:     p.Verdict.Feedback,
			Timestamp:  p.EndedAt,
			Confidence: 1.0,
			Source:     "bossbattle",
		},
	)

	return triples
}

// Schema returns the message type for payload registry.
func (p *BattleVerdictPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "battle.verdict", Version: "v1"}
}

// =============================================================================
// AGENT PROGRESSION - Graphable implementations
// =============================================================================

// EntityID returns the 6-part federated entity ID for the agent.
// Note: XP events update agent state, so EntityID is the agent.
func (p *AgentXPPayload) EntityID() string {
	return string(p.AgentID)
}

// Triples returns all semantic facts about the agent's XP change.
func (p *AgentXPPayload) Triples() []message.Triple {
	id := string(p.AgentID)
	ts := p.Timestamp

	triples := []message.Triple{
		{
			Subject:    id,
			Predicate:  "agent.progression.xp",
			Object:     p.XPAfter,
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "xpengine",
		},
		{
			Subject:    id,
			Predicate:  "agent.progression.level",
			Object:     p.LevelAfter,
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "xpengine",
		},
		{
			Subject:    id,
			Predicate:  "agent.progression.xp_delta",
			Object:     p.XPDelta,
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "xpengine",
		},
	}

	// Add relationship to quest that triggered this XP change
	if p.QuestID != "" {
		triples = append(triples, message.Triple{
			Subject:    id,
			Predicate:  "agent.progression.last_quest",
			Object:     string(p.QuestID),
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "xpengine",
		})
	}

	return triples
}

// Schema returns the message type for payload registry.
func (p *AgentXPPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "agent.xp", Version: "v1"}
}

// EntityID returns the 6-part federated entity ID for the agent.
func (p *AgentLevelPayload) EntityID() string {
	return string(p.AgentID)
}

// Triples returns all semantic facts about the agent's level change.
func (p *AgentLevelPayload) Triples() []message.Triple {
	id := string(p.AgentID)
	ts := p.Timestamp

	return []message.Triple{
		{
			Subject:    id,
			Predicate:  "agent.progression.level",
			Object:     p.NewLevel,
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "xpengine",
		},
		{
			Subject:    id,
			Predicate:  "agent.progression.tier",
			Object:     int(p.NewTier),
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "xpengine",
		},
		{
			Subject:    id,
			Predicate:  "agent.progression.xp",
			Object:     p.XPCurrent,
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "xpengine",
		},
		{
			Subject:    id,
			Predicate:  "agent.progression.xp_to_level",
			Object:     p.XPToLevel,
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "xpengine",
		},
	}
}

// Schema returns the message type for payload registry.
func (p *AgentLevelPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "agent.level", Version: "v1"}
}

// EntityID returns the 6-part federated entity ID for the agent.
func (p *AgentCooldownPayload) EntityID() string {
	return string(p.AgentID)
}

// Triples returns all semantic facts about the agent's cooldown state.
func (p *AgentCooldownPayload) Triples() []message.Triple {
	id := string(p.AgentID)
	ts := p.Timestamp

	return []message.Triple{
		{
			Subject:    id,
			Predicate:  "agent.state.status",
			Object:     string(AgentCooldown),
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "xpengine",
		},
		{
			Subject:    id,
			Predicate:  "agent.state.cooldown_until",
			Object:     p.CooldownUntil.Unix(),
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "xpengine",
		},
		{
			Subject:    id,
			Predicate:  "agent.state.cooldown_duration_ns",
			Object:     int64(p.Duration),
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "xpengine",
		},
	}
}

// Schema returns the message type for payload registry.
func (p *AgentCooldownPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "agent.cooldown", Version: "v1"}
}

// =============================================================================
// GUILD FORMATION - Graphable implementations
// =============================================================================

// EntityID returns a synthetic entity ID for guild suggestions.
// Guild suggestions are stored as events, not persistent entities.
func (p *GuildSuggestedPayload) EntityID() string {
	// Create a deterministic ID based on primary skill and first agent
	if len(p.Suggestion.AgentIDs) > 0 {
		return "suggestion." + string(p.Suggestion.PrimarySkill) + "." + string(p.Suggestion.AgentIDs[0])
	}
	return "suggestion." + string(p.Suggestion.PrimarySkill) + ".unknown"
}

// Triples returns all semantic facts about the guild suggestion.
func (p *GuildSuggestedPayload) Triples() []message.Triple {
	id := p.EntityID()
	ts := p.Timestamp

	triples := []message.Triple{
		{
			Subject:    id,
			Predicate:  "guild.suggestion.primary_skill",
			Object:     string(p.Suggestion.PrimarySkill),
			Timestamp:  ts,
			Confidence: p.Suggestion.ClusterStrength,
			Source:     "guildformation",
		},
		{
			Subject:    id,
			Predicate:  "guild.suggestion.cluster_strength",
			Object:     p.Suggestion.ClusterStrength,
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "guildformation",
		},
		{
			Subject:    id,
			Predicate:  "guild.suggestion.min_level",
			Object:     p.Suggestion.MinLevel,
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "guildformation",
		},
		{
			Subject:    id,
			Predicate:  "guild.suggestion.suggested_name",
			Object:     p.Suggestion.SuggestedName,
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "guildformation",
		},
	}

	// Add member relationships
	for _, agentID := range p.Suggestion.AgentIDs {
		triples = append(triples, message.Triple{
			Subject:    id,
			Predicate:  "guild.suggestion.member",
			Object:     string(agentID),
			Timestamp:  ts,
			Confidence: p.Suggestion.ClusterStrength,
			Source:     "guildformation",
		})
	}

	return triples
}

// Schema returns the message type for payload registry.
func (p *GuildSuggestedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "guild.suggested", Version: "v1"}
}

// EntityID returns the 6-part federated entity ID for the agent.
// Auto-join events update agent state with guild membership.
func (p *GuildAutoJoinedPayload) EntityID() string {
	return string(p.AgentID)
}

// Triples returns all semantic facts about the auto-join event.
func (p *GuildAutoJoinedPayload) Triples() []message.Triple {
	id := string(p.AgentID)
	ts := p.JoinedAt

	return []message.Triple{
		{
			Subject:    id,
			Predicate:  "guild.membership.member_of",
			Object:     string(p.GuildID),
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "guildformation",
		},
		{
			Subject:    id,
			Predicate:  "guild.membership.rank",
			Object:     string(p.Rank),
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "guildformation",
		},
	}
}

// Schema returns the message type for payload registry.
func (p *GuildAutoJoinedPayload) Schema() types.Type {
	return types.Type{Domain: "semdragons", Category: "guild.autojoined", Version: "v1"}
}

// =============================================================================
// HELPER FUNCTIONS - Triple generation for domain types
// =============================================================================

// questToTriples converts a Quest to semantic triples.
func questToTriples(q *Quest, ts time.Time) []message.Triple {
	id := string(q.ID)

	triples := []message.Triple{
		{Subject: id, Predicate: "quest.lifecycle.status", Object: string(q.Status), Timestamp: ts, Confidence: 1.0, Source: "questboard"},
		{Subject: id, Predicate: "quest.state.title", Object: q.Title, Timestamp: ts, Confidence: 1.0, Source: "questboard"},
		{Subject: id, Predicate: "quest.state.description", Object: q.Description, Timestamp: ts, Confidence: 1.0, Source: "questboard"},
		{Subject: id, Predicate: "quest.state.difficulty", Object: int(q.Difficulty), Timestamp: ts, Confidence: 1.0, Source: "questboard"},
		{Subject: id, Predicate: "quest.state.min_tier", Object: int(q.MinTier), Timestamp: ts, Confidence: 1.0, Source: "questboard"},
		{Subject: id, Predicate: "quest.state.base_xp", Object: q.BaseXP, Timestamp: ts, Confidence: 1.0, Source: "questboard"},
		{Subject: id, Predicate: "quest.state.bonus_xp", Object: q.BonusXP, Timestamp: ts, Confidence: 1.0, Source: "questboard"},
		{Subject: id, Predicate: "quest.state.party_required", Object: q.PartyRequired, Timestamp: ts, Confidence: 1.0, Source: "questboard"},
		{Subject: id, Predicate: "quest.state.min_party_size", Object: q.MinPartySize, Timestamp: ts, Confidence: 1.0, Source: "questboard"},
		{Subject: id, Predicate: "quest.state.attempts", Object: q.Attempts, Timestamp: ts, Confidence: 1.0, Source: "questboard"},
		{Subject: id, Predicate: "quest.state.max_attempts", Object: q.MaxAttempts, Timestamp: ts, Confidence: 1.0, Source: "questboard"},
		{Subject: id, Predicate: "quest.state.escalated", Object: q.Escalated, Timestamp: ts, Confidence: 1.0, Source: "questboard"},
		{Subject: id, Predicate: "quest.state.posted_at", Object: q.PostedAt.Unix(), Timestamp: ts, Confidence: 1.0, Source: "questboard"},
	}

	// Add required skills
	for _, skill := range q.RequiredSkills {
		triples = append(triples, message.Triple{
			Subject:    id,
			Predicate:  "quest.requirement.skill",
			Object:     string(skill),
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "questboard",
		})
	}

	// Add required tools
	for _, tool := range q.RequiredTools {
		triples = append(triples, message.Triple{
			Subject:    id,
			Predicate:  "quest.requirement.tool",
			Object:     tool,
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "questboard",
		})
	}

	// Add relationships
	if q.ClaimedBy != nil {
		triples = append(triples, message.Triple{
			Subject:    id,
			Predicate:  "quest.assignment.claimed_by",
			Object:     string(*q.ClaimedBy),
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "questboard",
		})
	}

	if q.PartyID != nil {
		triples = append(triples, message.Triple{
			Subject:    id,
			Predicate:  "quest.assignment.party_id",
			Object:     string(*q.PartyID),
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "questboard",
		})
	}

	if q.GuildPriority != nil {
		triples = append(triples, message.Triple{
			Subject:    id,
			Predicate:  "quest.priority.guild",
			Object:     string(*q.GuildPriority),
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "questboard",
		})
	}

	if q.ParentQuest != nil {
		triples = append(triples, message.Triple{
			Subject:    id,
			Predicate:  "quest.hierarchy.parent",
			Object:     string(*q.ParentQuest),
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "questboard",
		})
	}

	// Add sub-quest relationships
	for _, subID := range q.SubQuests {
		triples = append(triples, message.Triple{
			Subject:    id,
			Predicate:  "quest.hierarchy.child",
			Object:     string(subID),
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "questboard",
		})
	}

	// Add timestamps
	if q.ClaimedAt != nil {
		triples = append(triples, message.Triple{
			Subject:    id,
			Predicate:  "quest.state.claimed_at",
			Object:     q.ClaimedAt.Unix(),
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "questboard",
		})
	}

	if q.StartedAt != nil {
		triples = append(triples, message.Triple{
			Subject:    id,
			Predicate:  "quest.state.started_at",
			Object:     q.StartedAt.Unix(),
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "questboard",
		})
	}

	if q.CompletedAt != nil {
		triples = append(triples, message.Triple{
			Subject:    id,
			Predicate:  "quest.state.completed_at",
			Object:     q.CompletedAt.Unix(),
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "questboard",
		})
	}

	if q.TrajectoryID != "" {
		triples = append(triples, message.Triple{
			Subject:    id,
			Predicate:  "quest.observability.trajectory_id",
			Object:     q.TrajectoryID,
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "questboard",
		})
	}

	return triples
}

// battleToTriples converts a BossBattle to semantic triples.
func battleToTriples(b *BossBattle, ts time.Time) []message.Triple {
	id := string(b.ID)

	triples := []message.Triple{
		{Subject: id, Predicate: "battle.state.status", Object: string(b.Status), Timestamp: ts, Confidence: 1.0, Source: "bossbattle"},
		{Subject: id, Predicate: "battle.state.level", Object: int(b.Level), Timestamp: ts, Confidence: 1.0, Source: "bossbattle"},
		{Subject: id, Predicate: "battle.state.started_at", Object: b.StartedAt.Unix(), Timestamp: ts, Confidence: 1.0, Source: "bossbattle"},
		// Relationship to quest
		{Subject: id, Predicate: "battle.assignment.quest_id", Object: string(b.QuestID), Timestamp: ts, Confidence: 1.0, Source: "bossbattle"},
		// Relationship to agent
		{Subject: id, Predicate: "battle.assignment.agent_id", Object: string(b.AgentID), Timestamp: ts, Confidence: 1.0, Source: "bossbattle"},
	}

	if b.CompletedAt != nil {
		triples = append(triples, message.Triple{
			Subject:    id,
			Predicate:  "battle.state.completed_at",
			Object:     b.CompletedAt.Unix(),
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "bossbattle",
		})
	}

	// Add judge info
	for i, judge := range b.Judges {
		triples = append(triples, message.Triple{
			Subject:    id,
			Predicate:  "battle.judge.id",
			Object:     judge.ID,
			Timestamp:  ts,
			Confidence: 1.0,
			Source:     "bossbattle",
			Context:    string(rune(i)), // Index for ordering
		})
	}

	return triples
}

// =============================================================================
// PAYLOAD REGISTRY - Register all Graphable payloads
// =============================================================================

// RegisterPayloads registers all semdragons payloads with the component registry.
// Call this during application initialization after RegisterVocabulary().
func RegisterPayloads(registry *component.PayloadRegistry) error {
	payloads := []struct {
		Factory     func() any
		Domain      string
		Category    string
		Description string
	}{
		// Quest lifecycle
		{func() any { return &QuestPostedPayload{} }, "semdragons", "quest.posted", "Quest posted to board"},
		{func() any { return &QuestClaimedPayload{} }, "semdragons", "quest.claimed", "Quest claimed by agent"},
		{func() any { return &QuestStartedPayload{} }, "semdragons", "quest.started", "Quest work started"},
		{func() any { return &QuestSubmittedPayload{} }, "semdragons", "quest.submitted", "Quest result submitted"},
		{func() any { return &QuestCompletedPayload{} }, "semdragons", "quest.completed", "Quest completed successfully"},
		{func() any { return &QuestFailedPayload{} }, "semdragons", "quest.failed", "Quest failed"},
		{func() any { return &QuestEscalatedPayload{} }, "semdragons", "quest.escalated", "Quest escalated for attention"},
		{func() any { return &QuestAbandonedPayload{} }, "semdragons", "quest.abandoned", "Quest abandoned by agent"},

		// Boss battle
		{func() any { return &BattleStartedPayload{} }, "semdragons", "battle.started", "Boss battle started"},
		{func() any { return &BattleVerdictPayload{} }, "semdragons", "battle.verdict", "Boss battle verdict rendered"},

		// Agent progression
		{func() any { return &AgentXPPayload{} }, "semdragons", "agent.xp", "Agent XP change"},
		{func() any { return &AgentLevelPayload{} }, "semdragons", "agent.level", "Agent level change"},
		{func() any { return &AgentCooldownPayload{} }, "semdragons", "agent.cooldown", "Agent entered cooldown"},

		// Guild formation
		{func() any { return &GuildSuggestedPayload{} }, "semdragons", "guild.suggested", "Guild formation suggested"},
		{func() any { return &GuildAutoJoinedPayload{} }, "semdragons", "guild.autojoined", "Agent auto-joined guild"},
	}

	for _, p := range payloads {
		if err := registry.RegisterPayload(&component.PayloadRegistration{
			Factory:     p.Factory,
			Domain:      p.Domain,
			Category:    p.Category,
			Version:     "v1",
			Description: p.Description,
		}); err != nil {
			return err
		}
	}
	return nil
}
