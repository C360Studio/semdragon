package domain

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
// =============================================================================

// -----------------------------------------------------------------------------
// QUEST
// -----------------------------------------------------------------------------

// EntityID returns the 6-part entity ID for this quest.
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
		{Subject: entityID, Predicate: "quest.identity.name", Object: q.Name, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "quest.identity.title", Object: q.Title, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "quest.identity.description", Object: q.Description, Source: source, Timestamp: now, Confidence: 1.0},

		// Status
		{Subject: entityID, Predicate: "quest.status.state", Object: string(q.Status), Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "quest.difficulty.level", Object: int(q.Difficulty), Source: source, Timestamp: now, Confidence: 1.0},

		// Classification
		{Subject: entityID, Predicate: "quest.classification.type", Object: string(q.QuestType), Source: source, Timestamp: now, Confidence: 1.0},

		// Requirements
		{Subject: entityID, Predicate: "quest.tier.minimum", Object: int(q.MinTier), Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "quest.party.required", Object: q.PartyRequired, Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "quest.party.min_size", Object: q.MinPartySize, Source: source, Timestamp: now, Confidence: 1.0},

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

	if q.RedTeamTarget != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.classification.red_team_target", Object: string(*q.RedTeamTarget),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	if q.RedTeamStatus != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.classification.red_team_status", Object: q.RedTeamStatus,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	if q.RedTeamQuestID != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.classification.red_team_quest_id", Object: string(*q.RedTeamQuestID),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	if q.ParentQuest != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.parent.quest", Object: string(*q.ParentQuest),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	for _, depID := range q.DependsOn {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.dependency.quest", Object: string(depID),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	for _, criterion := range q.Acceptance {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.acceptance.criterion", Object: criterion,
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

	// Input and Output (stored as-is; typically strings from LLM I/O)
	if q.Input != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.data.input", Object: q.Input,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}
	if q.Output != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.data.output", Object: q.Output,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	if q.LoopID != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.execution.loop_id", Object: q.LoopID,
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

	// Verdict (set on completion after boss battle)
	if q.Verdict != nil {
		triples = append(triples,
			message.Triple{Subject: entityID, Predicate: "quest.verdict.passed", Object: q.Verdict.Passed, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: "quest.verdict.score", Object: q.Verdict.QualityScore, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: "quest.verdict.xp_awarded", Object: q.Verdict.XPAwarded, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: "quest.verdict.feedback", Object: q.Verdict.Feedback, Source: source, Timestamp: now, Confidence: 1.0},
		)
	}

	// Escalation
	if q.Escalated {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.failure.escalated", Object: true,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Failure info
	if q.FailureReason != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.failure.reason", Object: q.FailureReason,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}
	if q.FailureType != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.failure.type", Object: string(q.FailureType),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Failure recovery (triage)
	if q.RecoveryPath != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.recovery.path", Object: string(q.RecoveryPath),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}
	if q.FailureAnalysis != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.recovery.analysis", Object: q.FailureAnalysis,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}
	if q.SalvagedOutput != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.recovery.salvaged", Object: q.SalvagedOutput,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}
	if len(q.AntiPatterns) > 0 {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.recovery.antipatterns", Object: q.AntiPatterns,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}
	if len(q.FailureHistory) > 0 {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.failure.history", Object: q.FailureHistory,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Duration
	if q.Duration > 0 {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.duration", Object: q.Duration.String(),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// DAG execution state (parent quest fields)
	if q.DAGExecutionID != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.dag.execution_id", Object: q.DAGExecutionID,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}
	if q.DAGDefinition != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.dag.definition", Object: q.DAGDefinition,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}
	if q.DAGNodeQuestIDs != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.dag.node_quest_ids", Object: q.DAGNodeQuestIDs,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}
	if q.DAGNodeStates != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.dag.node_states", Object: q.DAGNodeStates,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}
	if q.DAGNodeAssignees != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.dag.node_assignees", Object: q.DAGNodeAssignees,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}
	if q.DAGCompletedNodes != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.dag.completed_nodes", Object: q.DAGCompletedNodes,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}
	if q.DAGFailedNodes != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.dag.failed_nodes", Object: q.DAGFailedNodes,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}
	if q.DAGNodeRetries != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.dag.node_retries", Object: q.DAGNodeRetries,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// DAG sub-quest fields
	if q.DAGNodeID != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.dag.node_id", Object: q.DAGNodeID,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}
	if q.DAGClarifications != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.dag.clarifications", Object: q.DAGClarifications,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}
	if q.DMClarifications != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.dm.clarifications", Object: q.DMClarifications,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Context metadata — what went into the prompt for this quest execution.
	if q.ContextTokenCount > 0 {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.context.token_count", Object: q.ContextTokenCount,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}
	if len(q.ContextSources) > 0 {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.context.sources", Object: q.ContextSources,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}
	if len(q.ContextEntities) > 0 {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.context.entities", Object: q.ContextEntities,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Execution context
	if q.Repo != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: PredicateQuestRepo, Object: q.Repo,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Artifact tracking (git workspace)
	if q.ArtifactsMerged != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: PredicateQuestArtifactsMerged, Object: q.ArtifactsMerged,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}
	if q.ArtifactsIndexed {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: PredicateQuestArtifactsIndexed, Object: true,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}
	for _, producedID := range q.ProducedEntities {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: PredicateQuestProduced, Object: producedID,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Quest spec (from brief)
	if q.Goal != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.spec.goal", Object: q.Goal,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}
	if len(q.Requirements) > 0 {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.spec.requirements", Object: q.Requirements,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}
	if len(q.Scenarios) > 0 {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.spec.scenarios", Object: q.Scenarios,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}
	if q.DecomposabilityClass != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "quest.routing.class", Object: string(q.DecomposabilityClass),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	return triples
}

// -----------------------------------------------------------------------------
// GUILD
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

	// Quorum fields
	if g.QuorumSize > 0 {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "guild.quorum.size", Object: g.QuorumSize,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}
	if g.FormationDeadline != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "guild.quorum.deadline", Object: g.FormationDeadline.Format(time.RFC3339),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Applications
	for _, app := range g.Applications {
		prefix := fmt.Sprintf("guild.application.%s", app.ID)
		triples = append(triples,
			message.Triple{Subject: entityID, Predicate: prefix + ".applicant", Object: string(app.ApplicantID), Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: prefix + ".status", Object: string(app.Status), Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: prefix + ".level", Object: app.Level, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: prefix + ".tier", Object: int(app.Tier), Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: prefix + ".applied_at", Object: app.AppliedAt.Format(time.RFC3339), Source: source, Timestamp: now, Confidence: 1.0},
		)
		if app.Message != "" {
			triples = append(triples, message.Triple{
				Subject: entityID, Predicate: prefix + ".message", Object: app.Message,
				Source: source, Timestamp: now, Confidence: 1.0,
			})
		}
		for _, skill := range app.Skills {
			triples = append(triples, message.Triple{
				Subject: entityID, Predicate: prefix + ".skill", Object: string(skill),
				Source: source, Timestamp: now, Confidence: 1.0,
			})
		}
		if app.ReviewedBy != nil {
			triples = append(triples, message.Triple{
				Subject: entityID, Predicate: prefix + ".reviewed_by", Object: string(*app.ReviewedBy),
				Source: source, Timestamp: now, Confidence: 1.0,
			})
		}
		if app.Reason != "" {
			triples = append(triples, message.Triple{
				Subject: entityID, Predicate: prefix + ".reason", Object: app.Reason,
				Source: source, Timestamp: now, Confidence: 1.0,
			})
		}
		if app.ReviewedAt != nil {
			triples = append(triples, message.Triple{
				Subject: entityID, Predicate: prefix + ".reviewed_at", Object: app.ReviewedAt.Format(time.RFC3339),
				Source: source, Timestamp: now, Confidence: 1.0,
			})
		}
	}

	// Lessons (stored as a single JSON blob, like quest.dag.definition)
	if len(g.Lessons) > 0 {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "guild.knowledge.lessons", Object: g.Lessons,
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	return triples
}

// -----------------------------------------------------------------------------
// PEER REVIEW
// -----------------------------------------------------------------------------

// EntityID returns the 6-part entity ID for this peer review.
func (pr *PeerReview) EntityID() string {
	return string(pr.ID)
}

// Triples returns all semantic facts about this peer review.
func (pr *PeerReview) Triples() []message.Triple {
	now := time.Now()
	source := "peerreview"
	entityID := pr.EntityID()

	triples := []message.Triple{
		// Status
		{Subject: entityID, Predicate: "review.status.state", Object: string(pr.Status), Source: source, Timestamp: now, Confidence: 1.0},

		// Assignment
		{Subject: entityID, Predicate: "review.assignment.quest", Object: string(pr.QuestID), Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "review.assignment.leader", Object: string(pr.LeaderID), Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "review.assignment.member", Object: string(pr.MemberID), Source: source, Timestamp: now, Confidence: 1.0},
		{Subject: entityID, Predicate: "review.config.solo_task", Object: pr.IsSoloTask, Source: source, Timestamp: now, Confidence: 1.0},

		// Lifecycle
		{Subject: entityID, Predicate: "review.lifecycle.created_at", Object: pr.CreatedAt.Format(time.RFC3339), Source: source, Timestamp: now, Confidence: 1.0},
	}

	if pr.PartyID != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "review.assignment.party", Object: string(*pr.PartyID),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	// Leader's review of member
	if pr.LeaderReview != nil {
		triples = append(triples,
			message.Triple{Subject: entityID, Predicate: "review.leader.q1", Object: pr.LeaderReview.Ratings.Q1, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: "review.leader.q2", Object: pr.LeaderReview.Ratings.Q2, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: "review.leader.q3", Object: pr.LeaderReview.Ratings.Q3, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: "review.leader.submitted_at", Object: pr.LeaderReview.SubmittedAt.Format(time.RFC3339), Source: source, Timestamp: now, Confidence: 1.0},
		)
		if pr.LeaderReview.Explanation != "" {
			triples = append(triples, message.Triple{
				Subject: entityID, Predicate: "review.leader.explanation", Object: pr.LeaderReview.Explanation,
				Source: source, Timestamp: now, Confidence: 1.0,
			})
		}
	}

	// Member's review of leader
	if pr.MemberReview != nil {
		triples = append(triples,
			message.Triple{Subject: entityID, Predicate: "review.member.q1", Object: pr.MemberReview.Ratings.Q1, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: "review.member.q2", Object: pr.MemberReview.Ratings.Q2, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: "review.member.q3", Object: pr.MemberReview.Ratings.Q3, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: "review.member.submitted_at", Object: pr.MemberReview.SubmittedAt.Format(time.RFC3339), Source: source, Timestamp: now, Confidence: 1.0},
		)
		if pr.MemberReview.Explanation != "" {
			triples = append(triples, message.Triple{
				Subject: entityID, Predicate: "review.member.explanation", Object: pr.MemberReview.Explanation,
				Source: source, Timestamp: now, Confidence: 1.0,
			})
		}
	}

	// Computed averages (when completed)
	if pr.Status == PeerReviewCompleted {
		triples = append(triples,
			message.Triple{Subject: entityID, Predicate: "review.result.leader_avg", Object: pr.LeaderAvgRating, Source: source, Timestamp: now, Confidence: 1.0},
			message.Triple{Subject: entityID, Predicate: "review.result.member_avg", Object: pr.MemberAvgRating, Source: source, Timestamp: now, Confidence: 1.0},
		)
	}

	if pr.CompletedAt != nil {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: "review.lifecycle.completed_at", Object: pr.CompletedAt.Format(time.RFC3339),
			Source: source, Timestamp: now, Confidence: 1.0,
		})
	}

	return triples
}
