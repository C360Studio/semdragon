package domain

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
//
//	entity, err := gc.GetEntityDirect(ctx, entityID)
//	quest := QuestFromEntityState(entity)
//
// =============================================================================

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
			q.Title = AsString(triple.Object)
		case "quest.identity.description":
			q.Description = AsString(triple.Object)

		// Status
		case "quest.status.state":
			q.Status = QuestStatus(AsString(triple.Object))
		case "quest.difficulty.level":
			q.Difficulty = QuestDifficulty(AsInt(triple.Object))

		// Requirements
		case "quest.tier.minimum":
			q.MinTier = TrustTier(AsInt(triple.Object))
		case "quest.party.required":
			q.PartyRequired = AsBool(triple.Object)
		case "quest.party.min_size":
			q.MinPartySize = AsInt(triple.Object)

		// Rewards
		case "quest.xp.base":
			q.BaseXP = AsInt64(triple.Object)

		// Lifecycle
		case "quest.attempts.current":
			q.Attempts = AsInt(triple.Object)
		case "quest.attempts.max":
			q.MaxAttempts = AsInt(triple.Object)
		case "quest.failure.escalated":
			q.Escalated = AsBool(triple.Object)
		case "quest.lifecycle.posted_at":
			q.PostedAt = AsTime(triple.Object)
		case "quest.lifecycle.claimed_at":
			t := AsTime(triple.Object)
			q.ClaimedAt = &t
		case "quest.lifecycle.started_at":
			t := AsTime(triple.Object)
			q.StartedAt = &t
		case "quest.lifecycle.completed_at":
			t := AsTime(triple.Object)
			q.CompletedAt = &t

		// Relationships
		case "quest.assignment.agent":
			agentID := AgentID(AsString(triple.Object))
			q.ClaimedBy = &agentID
		case "quest.assignment.party":
			partyID := PartyID(AsString(triple.Object))
			q.PartyID = &partyID
		case "quest.priority.guild":
			guildID := GuildID(AsString(triple.Object))
			q.GuildPriority = &guildID
		case "quest.parent.quest":
			parentID := QuestID(AsString(triple.Object))
			q.ParentQuest = &parentID
		case "quest.dependency.quest":
			if v := AsString(triple.Object); v != "" {
				q.DependsOn = append(q.DependsOn, QuestID(v))
			}
		case "quest.acceptance.criterion":
			if v := AsString(triple.Object); v != "" {
				q.Acceptance = append(q.Acceptance, v)
			}

		// Input and Output
		case "quest.data.input":
			q.Input = triple.Object
		case "quest.data.output":
			q.Output = triple.Object

		// Skills and tools (collected separately)
		case "quest.skill.required":
			q.RequiredSkills = append(q.RequiredSkills, SkillTag(AsString(triple.Object)))
		case "quest.tool.required":
			q.RequiredTools = append(q.RequiredTools, AsString(triple.Object))

		// Review
		case "quest.review.level":
			q.Constraints.ReviewLevel = ReviewLevel(AsInt(triple.Object))
		case "quest.review.needs_review":
			q.Constraints.RequireReview = AsBool(triple.Object)

		// Observability
		case "quest.execution.loop_id":
			q.LoopID = AsString(triple.Object)

		// Verdict (from boss battle)
		case "quest.verdict.passed":
			if q.Verdict == nil {
				q.Verdict = &BattleVerdict{}
			}
			q.Verdict.Passed = AsBool(triple.Object)
		case "quest.verdict.score":
			if q.Verdict == nil {
				q.Verdict = &BattleVerdict{}
			}
			q.Verdict.QualityScore = AsFloat64(triple.Object)
		case "quest.verdict.xp_awarded":
			if q.Verdict == nil {
				q.Verdict = &BattleVerdict{}
			}
			q.Verdict.XPAwarded = AsInt64(triple.Object)
		case "quest.verdict.feedback":
			if q.Verdict == nil {
				q.Verdict = &BattleVerdict{}
			}
			q.Verdict.Feedback = AsString(triple.Object)

		// Failure info
		case "quest.failure.reason":
			q.FailureReason = AsString(triple.Object)
		case "quest.failure.type":
			q.FailureType = FailureType(AsString(triple.Object))

		// Duration
		case "quest.duration":
			if d, err := time.ParseDuration(AsString(triple.Object)); err == nil {
				q.Duration = d
			}

		// DAG execution state (parent quest)
		case "quest.dag.execution_id":
			q.DAGExecutionID = AsString(triple.Object)
		case "quest.dag.definition":
			q.DAGDefinition = triple.Object
		case "quest.dag.node_quest_ids":
			q.DAGNodeQuestIDs = triple.Object
		case "quest.dag.node_states":
			q.DAGNodeStates = triple.Object
		case "quest.dag.node_assignees":
			q.DAGNodeAssignees = triple.Object
		case "quest.dag.completed_nodes":
			q.DAGCompletedNodes = triple.Object
		case "quest.dag.failed_nodes":
			q.DAGFailedNodes = triple.Object
		case "quest.dag.node_retries":
			q.DAGNodeRetries = triple.Object

		// DAG sub-quest fields
		case "quest.dag.node_id":
			q.DAGNodeID = AsString(triple.Object)
		case "quest.dag.clarifications":
			q.DAGClarifications = triple.Object

		// DM clarification exchanges (standalone/parent quests)
		case "quest.dm.clarifications":
			q.DMClarifications = triple.Object

		// Context metadata
		case "quest.context.token_count":
			q.ContextTokenCount = AsInt(triple.Object)
		case "quest.context.sources":
			q.ContextSources = AsStringSlice(triple.Object)
		case "quest.context.entities":
			q.ContextEntities = AsStringSlice(triple.Object)
		}
	}

	return q
}

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
			g.Name = AsString(triple.Object)
		case "guild.identity.description":
			g.Description = AsString(triple.Object)
		case "guild.identity.culture":
			g.Culture = AsString(triple.Object)
		case "guild.identity.motto":
			g.Motto = AsString(triple.Object)

		// Status
		case "guild.status.state":
			g.Status = GuildStatus(AsString(triple.Object))

		// Configuration
		case "guild.config.max_members":
			g.MaxMembers = AsInt(triple.Object)
		case "guild.config.min_level":
			g.MinLevel = AsInt(triple.Object)

		// Founding
		case "guild.founding.date":
			g.Founded = AsTime(triple.Object)
		case "guild.founding.agent":
			g.FoundedBy = AgentID(AsString(triple.Object))

		// Stats
		case "guild.stats.reputation":
			g.Reputation = AsFloat64(triple.Object)
		case "guild.stats.quests_handled":
			g.QuestsHandled = AsInt(triple.Object)
		case "guild.stats.quests_failed":
			g.QuestsFailed = AsInt(triple.Object)
		case "guild.stats.success_rate":
			g.SuccessRate = AsFloat64(triple.Object)

		// Lifecycle
		case "guild.lifecycle.created_at":
			g.CreatedAt = AsTime(triple.Object)

		// Membership
		case "guild.membership.agent":
			agentID := AgentID(AsString(triple.Object))
			if memberData[agentID] == nil {
				memberData[agentID] = &GuildMember{AgentID: agentID}
			}

		// Resources
		case "guild.resource.tool":
			g.SharedTools = append(g.SharedTools, AsString(triple.Object))

		// Routing
		case "guild.routing.quest_type":
			g.QuestTypes = append(g.QuestTypes, AsString(triple.Object))
		case "guild.routing.preferred_client":
			g.PreferredClients = append(g.PreferredClients, AsString(triple.Object))
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
						memberData[agentID].Rank = GuildRank(AsString(triple.Object))
					case "contribution":
						memberData[agentID].Contribution = AsFloat64(triple.Object)
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

// PeerReviewFromEntityState reconstructs a PeerReview from graph EntityState.
func PeerReviewFromEntityState(entity *graph.EntityState) *PeerReview {
	if entity == nil {
		return nil
	}

	pr := &PeerReview{
		ID: PeerReviewID(entity.ID),
	}

	for _, triple := range entity.Triples {
		switch triple.Predicate {
		case "review.status.state":
			pr.Status = PeerReviewStatus(AsString(triple.Object))
		case "review.assignment.quest":
			pr.QuestID = QuestID(AsString(triple.Object))
		case "review.assignment.leader":
			pr.LeaderID = AgentID(AsString(triple.Object))
		case "review.assignment.member":
			pr.MemberID = AgentID(AsString(triple.Object))
		case "review.assignment.party":
			partyID := PartyID(AsString(triple.Object))
			pr.PartyID = &partyID
		case "review.config.solo_task":
			pr.IsSoloTask = AsBool(triple.Object)
		case "review.lifecycle.created_at":
			pr.CreatedAt = AsTime(triple.Object)
		case "review.lifecycle.completed_at":
			t := AsTime(triple.Object)
			pr.CompletedAt = &t
		case "review.result.leader_avg":
			pr.LeaderAvgRating = AsFloat64(triple.Object)
		case "review.result.member_avg":
			pr.MemberAvgRating = AsFloat64(triple.Object)

		// Leader review fields
		case "review.leader.q1":
			ensureLeaderReview(pr)
			pr.LeaderReview.Ratings.Q1 = AsInt(triple.Object)
		case "review.leader.q2":
			ensureLeaderReview(pr)
			pr.LeaderReview.Ratings.Q2 = AsInt(triple.Object)
		case "review.leader.q3":
			ensureLeaderReview(pr)
			pr.LeaderReview.Ratings.Q3 = AsInt(triple.Object)
		case "review.leader.explanation":
			ensureLeaderReview(pr)
			pr.LeaderReview.Explanation = AsString(triple.Object)
		case "review.leader.submitted_at":
			ensureLeaderReview(pr)
			pr.LeaderReview.SubmittedAt = AsTime(triple.Object)

		// Member review fields
		case "review.member.q1":
			ensureMemberReview(pr)
			pr.MemberReview.Ratings.Q1 = AsInt(triple.Object)
		case "review.member.q2":
			ensureMemberReview(pr)
			pr.MemberReview.Ratings.Q2 = AsInt(triple.Object)
		case "review.member.q3":
			ensureMemberReview(pr)
			pr.MemberReview.Ratings.Q3 = AsInt(triple.Object)
		case "review.member.explanation":
			ensureMemberReview(pr)
			pr.MemberReview.Explanation = AsString(triple.Object)
		case "review.member.submitted_at":
			ensureMemberReview(pr)
			pr.MemberReview.SubmittedAt = AsTime(triple.Object)
		}
	}

	return pr
}

// ensureLeaderReview initializes the LeaderReview field if nil.
func ensureLeaderReview(pr *PeerReview) {
	if pr.LeaderReview == nil {
		pr.LeaderReview = &ReviewSubmission{
			ReviewerID: pr.LeaderID,
			RevieweeID: pr.MemberID,
			Direction:  ReviewDirectionLeaderToMember,
		}
	}
}

// ensureMemberReview initializes the MemberReview field if nil.
func ensureMemberReview(pr *PeerReview) {
	if pr.MemberReview == nil {
		pr.MemberReview = &ReviewSubmission{
			ReviewerID: pr.MemberID,
			RevieweeID: pr.LeaderID,
			Direction:  ReviewDirectionMemberToLeader,
		}
	}
}

// =============================================================================
// TYPE CONVERSION HELPERS (Exported for use by processor packages)
// =============================================================================

// AsString converts an interface value to string.
func AsString(v interface{}) string {
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

// AsInt converts an interface value to int.
func AsInt(v interface{}) int {
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

// AsInt64 converts an interface value to int64.
func AsInt64(v interface{}) int64 {
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

// AsFloat64 converts an interface value to float64.
func AsFloat64(v interface{}) float64 {
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

// AsStringSlice converts an interface value to []string.
// Handles both []string (direct) and []any (from JSON unmarshal).
func AsStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case []string:
		return val
	case []any:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}

// AsBool converts an interface value to bool.
func AsBool(v interface{}) bool {
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

// AsTime converts an interface value to time.Time.
func AsTime(v interface{}) time.Time {
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
