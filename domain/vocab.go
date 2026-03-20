package domain

import (
	"github.com/c360studio/semstreams/vocabulary"
)

// =============================================================================
// VOCABULARY - Three-part predicate definitions for semdragons
// =============================================================================
// All predicates follow semstreams convention: domain.category.property
// This enables NATS wildcard subscriptions like "quest.lifecycle.>"
// =============================================================================

// --- Quest Lifecycle Predicates ---

const (
	// PredicateQuestPosted - Quest added to the board, available for claiming.
	PredicateQuestPosted = "quest.lifecycle.posted"

	// PredicateQuestClaimed - Agent or party claimed a quest.
	PredicateQuestClaimed = "quest.lifecycle.claimed"

	// PredicateQuestStarted - Work began on a quest.
	PredicateQuestStarted = "quest.lifecycle.started"

	// PredicateQuestSubmitted - Result submitted for review.
	PredicateQuestSubmitted = "quest.lifecycle.submitted"

	// PredicateQuestCompleted - Quest finished successfully.
	PredicateQuestCompleted = "quest.lifecycle.completed"

	// PredicateQuestFailed - Quest failed (may be re-posted).
	PredicateQuestFailed = "quest.lifecycle.failed"

	// PredicateQuestPendingTriage - Quest awaiting DM triage after exhausting retries.
	PredicateQuestPendingTriage = "quest.lifecycle.pending_triage"

	// PredicateQuestTriaged - DM triage decision applied to quest.
	PredicateQuestTriaged = "quest.lifecycle.triaged"

	// PredicateQuestEscalated - Quest escalated for higher-level attention.
	PredicateQuestEscalated = "quest.lifecycle.escalated"

	// PredicateQuestAbandoned - Agent gave up on a quest.
	PredicateQuestAbandoned = "quest.lifecycle.abandoned"

	// PredicateQuestDecomposed - Quest broken into sub-quests.
	PredicateQuestDecomposed = "quest.lifecycle.decomposed"
)

// --- Quest Context Predicates ---

const (
	// PredicateQuestRepo - Target repository for quest artifact storage.
	PredicateQuestRepo = "quest.context.repo"
)

// --- Quest Metrics Predicates ---

const (
	// PredicateQuestMetricsTurns - Number of agentic-loop turns used during execution.
	PredicateQuestMetricsTurns = "quest.metrics.turns_used"

	// PredicateQuestMetricsTokensIn - Prompt tokens consumed during execution.
	PredicateQuestMetricsTokensIn = "quest.metrics.tokens_prompt"

	// PredicateQuestMetricsTokensOut - Completion tokens consumed during execution.
	PredicateQuestMetricsTokensOut = "quest.metrics.tokens_completion"
)

// --- Quest Artifact Predicates ---

const (
	// PredicateQuestArtifactsMerged - Git merge commit hash after boss battle victory.
	PredicateQuestArtifactsMerged = "quest.artifacts.merged"

	// PredicateQuestArtifactsIndexed - True when semsource has processed the merged artifacts.
	PredicateQuestArtifactsIndexed = "quest.artifacts.indexed"

	// PredicateQuestProduced - Relationship: quest entity → semsource entity it produced.
	PredicateQuestProduced = "quest.relationship.produced"
)

// --- Boss Battle Predicates ---

const (
	// PredicateBattleStarted - Boss battle (review) began.
	PredicateBattleStarted = "battle.review.started"

	// PredicateBattleVerdict - Review verdict rendered.
	PredicateBattleVerdict = "battle.review.verdict"

	// PredicateBattleVictory - Agent passed the review.
	PredicateBattleVictory = "battle.review.victory"

	// PredicateBattleDefeat - Agent failed the review.
	PredicateBattleDefeat = "battle.review.defeat"

	// PredicateBattleRetreat - Agent requested re-do.
	PredicateBattleRetreat = "battle.review.retreat"
)

// --- Agent Identity Predicates ---

const (
	// PredicateAgentArchetype - Agent class identity (scholar, engineer, scribe, strategist).
	// Fixed at creation; never changes on level-up.
	PredicateAgentArchetype = "agent.identity.archetype"
)

// --- Agent Progression Predicates ---

const (
	// PredicateAgentXP - XP earned or lost.
	PredicateAgentXP = "agent.progression.xp"

	// PredicateAgentLevelUp - Agent leveled up.
	PredicateAgentLevelUp = "agent.progression.levelup"

	// PredicateAgentLevelDown - Agent leveled down.
	PredicateAgentLevelDown = "agent.progression.leveldown"

	// PredicateAgentDeath - Agent permadeath (catastrophic failure).
	PredicateAgentDeath = "agent.progression.death"

	// PredicateAgentCooldown - Agent entered cooldown.
	PredicateAgentCooldown = "agent.progression.cooldown"

	// PredicateAgentReady - Agent cooldown ended, ready for quests.
	PredicateAgentReady = "agent.progression.ready"
)

// --- Skill Progression Predicates ---

const (
	// PredicateSkillImproved - Skill proficiency improved from quest use.
	PredicateSkillImproved = "skill.progression.improved"

	// PredicateSkillLevelUp - Skill reached a new proficiency level.
	PredicateSkillLevelUp = "skill.progression.levelup"

	// PredicateMentorBonus - Mentor earned XP for trainee improvement.
	PredicateMentorBonus = "skill.progression.mentorbonus"
)

// --- Seeding Predicates ---

const (
	// PredicateSeedingStarted - Environment seeding began.
	PredicateSeedingStarted = "seeding.lifecycle.started"

	// PredicateSeedingCompleted - Environment seeding finished.
	PredicateSeedingCompleted = "seeding.lifecycle.completed"

	// PredicateArenaRoundStarted - Training arena round began.
	PredicateArenaRoundStarted = "seeding.arena.roundstarted"

	// PredicateArenaRoundCompleted - Training arena round finished.
	PredicateArenaRoundCompleted = "seeding.arena.roundcompleted"

	// PredicateRosterAgentCreated - Roster seeding created an agent.
	PredicateRosterAgentCreated = "seeding.roster.agentcreated"

	// PredicateNPCSpawned - Bootstrap NPC spawned for training.
	PredicateNPCSpawned = "seeding.npc.spawned"

	// PredicateNPCRetired - Bootstrap NPC retired (replaced by real mentor).
	PredicateNPCRetired = "seeding.npc.retired"
)

// --- Party Predicates ---

const (
	// PredicatePartyFormed - Party created for a quest.
	PredicatePartyFormed = "party.formation.formed"

	// PredicatePartyDisbanded - Party dissolved after quest.
	PredicatePartyDisbanded = "party.formation.disbanded"

	// PredicatePartyJoined - Agent joined a party.
	PredicatePartyJoined = "party.membership.joined"

	// PredicatePartyLeft - Agent left a party.
	PredicatePartyLeft = "party.membership.left"
)

// --- Party Coordination Predicates ---
// Communication events between party leads and members for quest coordination.

const (
	// PredicatePartyQuestDecomposed - Lead broke down quest into sub-quests.
	PredicatePartyQuestDecomposed = "party.coordination.decomposed"

	// PredicatePartyTaskAssigned - Lead assigned sub-quest to member.
	PredicatePartyTaskAssigned = "party.coordination.assigned"

	// PredicatePartyContextShared - Context shared with party.
	PredicatePartyContextShared = "party.coordination.contextshared"

	// PredicatePartyGuidanceIssued - Lead guided member.
	PredicatePartyGuidanceIssued = "party.coordination.guidance"

	// PredicatePartyProgressReported - Member reported status.
	PredicatePartyProgressReported = "party.coordination.progress"

	// PredicatePartyHelpRequested - Member needs help.
	PredicatePartyHelpRequested = "party.coordination.helprequest"

	// PredicatePartyResultSubmitted - Member submitted result.
	PredicatePartyResultSubmitted = "party.coordination.resultsubmitted"

	// PredicatePartyRollupStarted - Lead combining results.
	PredicatePartyRollupStarted = "party.coordination.rollupstarted"

	// PredicatePartyRollupCompleted - Rollup ready for boss battle.
	PredicatePartyRollupCompleted = "party.coordination.rollupcompleted"
)

// --- Guild Predicates ---

const (
	// PredicateGuildCreated - Guild created.
	PredicateGuildCreated = "guild.management.created"

	// PredicateGuildJoined - Agent joined a guild.
	PredicateGuildJoined = "guild.membership.joined"

	// PredicateGuildLeft - Agent left a guild.
	PredicateGuildLeft = "guild.membership.left"

	// PredicateGuildPromoted - Agent promoted in guild.
	PredicateGuildPromoted = "guild.membership.promoted"

	// PredicateGuildDemoted - Agent demoted in guild.
	PredicateGuildDemoted = "guild.membership.demoted"
)

// --- Guild Formation Predicates ---

const (
	// PredicateGuildSuggested - Skill cluster detected, guild formation suggested.
	PredicateGuildSuggested = "guild.formation.suggested"

	// PredicateGuildAutoJoined - Agent auto-recruited into a guild.
	PredicateGuildAutoJoined = "guild.formation.autojoined"

	// PredicateGuildProposed - Agent proposed founding a guild (awaiting DM approval).
	PredicateGuildProposed = "guild.formation.proposed"
)

// --- Guild Founding Quorum Predicates ---

const (
	// PredicateGuildPending - Guild created in pending state, awaiting quorum.
	PredicateGuildPending = "guild.lifecycle.pending"

	// PredicateGuildActivated - Guild reached quorum and became active.
	PredicateGuildActivated = "guild.lifecycle.activated"

	// PredicateGuildDissolved - Pending guild dissolved (timeout or manual).
	PredicateGuildDissolved = "guild.lifecycle.dissolved"

	// PredicateGuildApplicationSubmitted - Agent submitted application to pending guild.
	PredicateGuildApplicationSubmitted = "guild.application.submitted"

	// PredicateGuildApplicationAccepted - Founder accepted guild application.
	PredicateGuildApplicationAccepted = "guild.application.accepted"

	// PredicateGuildApplicationRejected - Founder rejected guild application.
	PredicateGuildApplicationRejected = "guild.application.rejected"
)

// --- Red-Team Review Predicates ---

const (
	// PredicateRedTeamPosted - Red-team review quest posted for a submitted quest.
	PredicateRedTeamPosted = "redteam.lifecycle.posted"

	// PredicateRedTeamCompleted - Red-team review completed, findings available.
	PredicateRedTeamCompleted = "redteam.lifecycle.completed"

	// PredicateRedTeamSkipped - Red-team review skipped (timeout or no eligible reviewers).
	PredicateRedTeamSkipped = "redteam.lifecycle.skipped"
)

// --- Guild Knowledge Predicates ---

const (
	// PredicateGuildLessonAdded - New lesson added to guild knowledge base.
	PredicateGuildLessonAdded = "guild.knowledge.lessonadded"

	// PredicateGuildKnowledgeUpdated - Guild knowledge base updated (batch).
	PredicateGuildKnowledgeUpdated = "guild.knowledge.updated"
)

// --- DM Session Predicates ---

const (
	// PredicateSessionStart - DM session started.
	PredicateSessionStart = "dm.session.start"

	// PredicateSessionEnd - DM session ended.
	PredicateSessionEnd = "dm.session.end"

	// PredicateDMIntervention - DM intervened in a quest.
	PredicateDMIntervention = "dm.action.intervention"

	// PredicateDMEscalation - DM handled an escalation.
	PredicateDMEscalation = "dm.action.escalation"
)

// --- Approval Predicates ---

const (
	// PredicateApprovalRequested - Approval request pending.
	PredicateApprovalRequested = "approval.request.pending"

	// PredicateApprovalResolved - Approval request resolved.
	PredicateApprovalResolved = "approval.request.resolved"
)

// --- Prompt Assembly Predicates ---

const (
	// PredicatePromptAssembled - System prompt assembled from domain catalog.
	PredicatePromptAssembled = "prompt.assembly.completed"
)

// --- Execution Predicates ---

const (
	// PredicateExecutionStarted - Quest execution started.
	PredicateExecutionStarted = "execution.lifecycle.started"

	// PredicateExecutionCompleted - Quest execution completed successfully.
	PredicateExecutionCompleted = "execution.lifecycle.completed"

	// PredicateExecutionFailed - Quest execution failed.
	PredicateExecutionFailed = "execution.lifecycle.failed"

	// PredicateToolCall - Tool invocation during execution.
	PredicateToolCall = "execution.tool.call"

	// PredicateToolResult - Tool result returned during execution.
	PredicateToolResult = "execution.tool.result"
)

// --- Agent Autonomy Predicates ---

const (
	// PredicateAutonomyEvaluated - Heartbeat fired, agent autonomy evaluated.
	PredicateAutonomyEvaluated = "agent.autonomy.evaluated"

	// PredicateAutonomyIdle - Agent idle with nothing actionable.
	PredicateAutonomyIdle = "agent.autonomy.idle"

	// PredicateAutonomyClaimIntent - Agent intends to claim a quest.
	PredicateAutonomyClaimIntent = "agent.autonomy.claimintent"

	// PredicateAutonomyShopIntent - Agent intends to purchase an item.
	PredicateAutonomyShopIntent = "agent.autonomy.shopintent"

	// PredicateAutonomyGuildIntent - Agent intends to join a guild.
	PredicateAutonomyGuildIntent = "agent.autonomy.guildintent"

	// PredicateAutonomyUseIntent - Agent intends to use a consumable.
	PredicateAutonomyUseIntent = "agent.autonomy.useintent"
)

// --- Peer Review Predicates ---

const (
	// PredicateReviewPending - Peer review created, waiting for submissions.
	PredicateReviewPending = "review.lifecycle.pending"

	// PredicateReviewSubmitted - One side submitted their review.
	PredicateReviewSubmitted = "review.lifecycle.submitted"

	// PredicateReviewCompleted - Both sides submitted, review complete.
	PredicateReviewCompleted = "review.lifecycle.completed"
)

// --- Store Predicates ---

const (
	// PredicateStoreItemListed - New item added to store.
	PredicateStoreItemListed = "store.item.listed"

	// PredicateStoreItemPurchased - Agent bought something.
	PredicateStoreItemPurchased = "store.item.purchased"

	// PredicateStoreItemUsed - Rental use consumed.
	PredicateStoreItemUsed = "store.item.used"

	// PredicateStoreItemExpired - Rental ran out of uses.
	PredicateStoreItemExpired = "store.item.expired"

	// PredicateConsumableUsed - Consumable activated.
	PredicateConsumableUsed = "store.consumable.used"

	// PredicateConsumableExpired - Consumable effect wore off.
	PredicateConsumableExpired = "store.consumable.expired"

	// PredicateInventoryUpdated - Agent inventory changed.
	PredicateInventoryUpdated = "agent.inventory.updated"
)

// RegisterVocabulary registers all semdragons predicates with the vocabulary system.
// Call this during application initialization.
func RegisterVocabulary() {
	// Quest lifecycle predicates
	vocabulary.Register(PredicateQuestPosted,
		vocabulary.WithDescription("Quest added to board, available for claiming"),
	)
	vocabulary.Register(PredicateQuestClaimed,
		vocabulary.WithDescription("Agent or party claimed a quest"),
	)
	vocabulary.Register(PredicateQuestStarted,
		vocabulary.WithDescription("Work began on a quest"),
	)
	vocabulary.Register(PredicateQuestSubmitted,
		vocabulary.WithDescription("Result submitted for review"),
	)
	vocabulary.Register(PredicateQuestCompleted,
		vocabulary.WithDescription("Quest finished successfully"),
	)
	vocabulary.Register(PredicateQuestFailed,
		vocabulary.WithDescription("Quest failed, may be re-posted"),
	)
	vocabulary.Register(PredicateQuestPendingTriage,
		vocabulary.WithDescription("Quest awaiting DM triage after exhausting retries"),
	)
	vocabulary.Register(PredicateQuestTriaged,
		vocabulary.WithDescription("DM triage decision applied to quest"),
	)
	vocabulary.Register(PredicateQuestEscalated,
		vocabulary.WithDescription("Quest escalated for higher-level attention"),
	)
	vocabulary.Register(PredicateQuestAbandoned,
		vocabulary.WithDescription("Agent abandoned a quest"),
	)
	vocabulary.Register(PredicateQuestDecomposed,
		vocabulary.WithDescription("Quest broken into sub-quests by party lead"),
	)

	// Quest metrics predicates
	vocabulary.Register(PredicateQuestMetricsTurns,
		vocabulary.WithDescription("Number of agentic-loop turns used during quest execution"),
		vocabulary.WithDataType("int"),
	)
	vocabulary.Register(PredicateQuestMetricsTokensIn,
		vocabulary.WithDescription("Prompt tokens consumed during quest execution"),
		vocabulary.WithDataType("int"),
	)
	vocabulary.Register(PredicateQuestMetricsTokensOut,
		vocabulary.WithDescription("Completion tokens consumed during quest execution"),
		vocabulary.WithDataType("int"),
	)

	// Quest context predicates
	vocabulary.Register(PredicateQuestRepo,
		vocabulary.WithDescription("Target repository for quest artifact storage"),
		vocabulary.WithDataType("string"),
	)

	// Quest artifact predicates
	vocabulary.Register(PredicateQuestArtifactsMerged,
		vocabulary.WithDescription("Git merge commit hash after boss battle victory"),
	)
	vocabulary.Register(PredicateQuestArtifactsIndexed,
		vocabulary.WithDescription("True when semsource has processed the merged artifacts"),
		vocabulary.WithDataType("bool"),
	)
	vocabulary.Register(PredicateQuestProduced,
		vocabulary.WithDescription("Relationship: quest entity to semsource entity it produced"),
	)

	// Boss battle predicates
	vocabulary.Register(PredicateBattleStarted,
		vocabulary.WithDescription("Boss battle (quality review) began"),
	)
	vocabulary.Register(PredicateBattleVerdict,
		vocabulary.WithDescription("Review verdict rendered"),
	)
	vocabulary.Register(PredicateBattleVictory,
		vocabulary.WithDescription("Agent passed the review"),
	)
	vocabulary.Register(PredicateBattleDefeat,
		vocabulary.WithDescription("Agent failed the review"),
	)
	vocabulary.Register(PredicateBattleRetreat,
		vocabulary.WithDescription("Agent requested re-do of submission"),
		vocabulary.WithDataType("BattleRetreatPayload"),
	)

	// Agent identity predicates
	vocabulary.Register(PredicateAgentArchetype,
		vocabulary.WithDescription("Agent class identity (scholar, engineer, scribe, strategist) — fixed at creation"),
		vocabulary.WithDataType("string"),
	)

	// Agent progression predicates
	vocabulary.Register(PredicateAgentXP,
		vocabulary.WithDescription("XP earned or lost by agent"),
		vocabulary.WithDataType("AgentXPPayload"),
	)
	vocabulary.Register(PredicateAgentLevelUp,
		vocabulary.WithDescription("Agent leveled up"),
	)
	vocabulary.Register(PredicateAgentLevelDown,
		vocabulary.WithDescription("Agent leveled down due to poor performance"),
	)
	vocabulary.Register(PredicateAgentDeath,
		vocabulary.WithDescription("Agent permadeath from catastrophic failure"),
	)
	vocabulary.Register(PredicateAgentCooldown,
		vocabulary.WithDescription("Agent entered cooldown after failure"),
	)
	vocabulary.Register(PredicateAgentReady,
		vocabulary.WithDescription("Agent cooldown ended, ready for quests"),
	)

	// Skill progression predicates
	vocabulary.Register(PredicateSkillImproved,
		vocabulary.WithDescription("Skill proficiency improved from quest use"),
		vocabulary.WithDataType("SkillProgressionPayload"),
	)
	vocabulary.Register(PredicateSkillLevelUp,
		vocabulary.WithDescription("Skill reached a new proficiency level"),
		vocabulary.WithDataType("SkillLevelUpPayload"),
	)
	vocabulary.Register(PredicateMentorBonus,
		vocabulary.WithDescription("Mentor earned XP for trainee improvement"),
		vocabulary.WithDataType("MentorBonusPayload"),
	)

	// Seeding predicates
	vocabulary.Register(PredicateSeedingStarted,
		vocabulary.WithDescription("Environment seeding began"),
		vocabulary.WithDataType("seeding.StartedPayload"),
	)
	vocabulary.Register(PredicateSeedingCompleted,
		vocabulary.WithDescription("Environment seeding finished"),
		vocabulary.WithDataType("seeding.CompletedPayload"),
	)
	vocabulary.Register(PredicateArenaRoundStarted,
		vocabulary.WithDescription("Training arena round began"),
		vocabulary.WithDataType("ArenaRoundPayload"),
	)
	vocabulary.Register(PredicateArenaRoundCompleted,
		vocabulary.WithDescription("Training arena round finished"),
		vocabulary.WithDataType("ArenaRoundPayload"),
	)
	vocabulary.Register(PredicateRosterAgentCreated,
		vocabulary.WithDescription("Roster seeding created an agent"),
		vocabulary.WithDataType("RosterAgentPayload"),
	)
	vocabulary.Register(PredicateNPCSpawned,
		vocabulary.WithDescription("Bootstrap NPC spawned for training"),
		vocabulary.WithDataType("NPCLifecyclePayload"),
	)
	vocabulary.Register(PredicateNPCRetired,
		vocabulary.WithDescription("Bootstrap NPC retired, replaced by real mentor"),
		vocabulary.WithDataType("NPCLifecyclePayload"),
	)

	// Party predicates
	vocabulary.Register(PredicatePartyFormed,
		vocabulary.WithDescription("Party formed for a quest"),
		vocabulary.WithDataType("PartyFormedPayload"),
	)
	vocabulary.Register(PredicatePartyDisbanded,
		vocabulary.WithDescription("Party disbanded after quest"),
		vocabulary.WithDataType("PartyDisbandedPayload"),
	)
	vocabulary.Register(PredicatePartyJoined,
		vocabulary.WithDescription("Agent joined a party"),
		vocabulary.WithDataType("PartyMemberPayload"),
	)
	vocabulary.Register(PredicatePartyLeft,
		vocabulary.WithDescription("Agent left a party"),
		vocabulary.WithDataType("PartyMemberPayload"),
	)

	// Party coordination predicates
	vocabulary.Register(PredicatePartyQuestDecomposed,
		vocabulary.WithDescription("Lead decomposed quest into sub-quests for party"),
		vocabulary.WithDataType("PartyQuestDecomposedPayload"),
	)
	vocabulary.Register(PredicatePartyTaskAssigned,
		vocabulary.WithDescription("Lead assigned sub-quest to party member"),
		vocabulary.WithDataType("PartyTaskAssignedPayload"),
	)
	vocabulary.Register(PredicatePartyContextShared,
		vocabulary.WithDescription("Context/insight shared with party"),
	)
	vocabulary.Register(PredicatePartyGuidanceIssued,
		vocabulary.WithDescription("Lead issued guidance to party member"),
		vocabulary.WithDataType("PartyGuidanceIssuedPayload"),
	)
	vocabulary.Register(PredicatePartyProgressReported,
		vocabulary.WithDescription("Member reported progress to lead"),
	)
	vocabulary.Register(PredicatePartyHelpRequested,
		vocabulary.WithDescription("Member requested help from lead"),
	)
	vocabulary.Register(PredicatePartyResultSubmitted,
		vocabulary.WithDescription("Member submitted sub-quest result to lead"),
		vocabulary.WithDataType("PartyResultSubmittedPayload"),
	)
	vocabulary.Register(PredicatePartyRollupStarted,
		vocabulary.WithDescription("Lead began combining sub-results"),
		vocabulary.WithDataType("PartyRollupStartedPayload"),
	)
	vocabulary.Register(PredicatePartyRollupCompleted,
		vocabulary.WithDescription("Lead completed rollup, ready for boss battle"),
		vocabulary.WithDataType("PartyRollupCompletedPayload"),
	)

	// Guild predicates
	vocabulary.Register(PredicateGuildCreated,
		vocabulary.WithDescription("Guild created"),
		vocabulary.WithDataType("GuildCreatedPayload"),
	)
	vocabulary.Register(PredicateGuildJoined,
		vocabulary.WithDescription("Agent joined a guild"),
		vocabulary.WithDataType("GuildMemberPayload"),
	)
	vocabulary.Register(PredicateGuildLeft,
		vocabulary.WithDescription("Agent left a guild"),
		vocabulary.WithDataType("GuildMemberPayload"),
	)
	vocabulary.Register(PredicateGuildPromoted,
		vocabulary.WithDescription("Agent promoted in guild rank"),
		vocabulary.WithDataType("GuildRankPayload"),
	)
	vocabulary.Register(PredicateGuildDemoted,
		vocabulary.WithDescription("Agent demoted in guild rank"),
		vocabulary.WithDataType("GuildRankPayload"),
	)

	// Guild formation predicates
	vocabulary.Register(PredicateGuildSuggested,
		vocabulary.WithDescription("Skill cluster detected, guild formation suggested"),
		vocabulary.WithDataType("GuildSuggestedPayload"),
	)
	vocabulary.Register(PredicateGuildAutoJoined,
		vocabulary.WithDescription("Agent auto-recruited into a guild"),
		vocabulary.WithDataType("GuildAutoJoinedPayload"),
	)
	vocabulary.Register(PredicateGuildProposed,
		vocabulary.WithDescription("Agent proposed founding a guild"),
		vocabulary.WithDataType("GuildCreateIntentPayload"),
	)

	// Guild founding quorum predicates
	vocabulary.Register(PredicateGuildPending,
		vocabulary.WithDescription("Guild created in pending state, awaiting founding quorum"),
	)
	vocabulary.Register(PredicateGuildActivated,
		vocabulary.WithDescription("Guild reached founding quorum and became active"),
	)
	vocabulary.Register(PredicateGuildDissolved,
		vocabulary.WithDescription("Pending guild dissolved due to timeout or manual action"),
	)
	vocabulary.Register(PredicateGuildApplicationSubmitted,
		vocabulary.WithDescription("Agent submitted application to join a pending guild"),
	)
	vocabulary.Register(PredicateGuildApplicationAccepted,
		vocabulary.WithDescription("Founder accepted a guild application"),
	)
	vocabulary.Register(PredicateGuildApplicationRejected,
		vocabulary.WithDescription("Founder rejected a guild application"),
	)

	// Red-team review predicates
	vocabulary.Register(PredicateRedTeamPosted,
		vocabulary.WithDescription("Red-team review quest posted for a submitted quest"),
	)
	vocabulary.Register(PredicateRedTeamCompleted,
		vocabulary.WithDescription("Red-team review completed, findings available on target quest"),
	)
	vocabulary.Register(PredicateRedTeamSkipped,
		vocabulary.WithDescription("Red-team review skipped due to timeout or no eligible reviewers"),
	)

	// Guild knowledge predicates
	vocabulary.Register(PredicateGuildLessonAdded,
		vocabulary.WithDescription("New lesson added to guild knowledge base"),
		vocabulary.WithDataType("Lesson"),
	)
	vocabulary.Register(PredicateGuildKnowledgeUpdated,
		vocabulary.WithDescription("Guild knowledge base updated with lessons from quest review"),
	)

	// DM session predicates
	vocabulary.Register(PredicateSessionStart,
		vocabulary.WithDescription("DM session started"),
		vocabulary.WithDataType("SessionStartPayload"),
	)
	vocabulary.Register(PredicateSessionEnd,
		vocabulary.WithDescription("DM session ended"),
		vocabulary.WithDataType("SessionEndPayload"),
	)
	vocabulary.Register(PredicateDMIntervention,
		vocabulary.WithDescription("DM intervened in a quest"),
		vocabulary.WithDataType("InterventionPayload"),
	)
	vocabulary.Register(PredicateDMEscalation,
		vocabulary.WithDescription("DM handled an escalation"),
		vocabulary.WithDataType("EscalationPayload"),
	)

	// Approval predicates
	vocabulary.Register(PredicateApprovalRequested,
		vocabulary.WithDescription("Approval request pending human decision"),
		vocabulary.WithDataType("ApprovalRequest"),
	)
	vocabulary.Register(PredicateApprovalResolved,
		vocabulary.WithDescription("Approval request resolved"),
		vocabulary.WithDataType("ApprovalResponse"),
	)

	// Prompt assembly predicates
	vocabulary.Register(PredicatePromptAssembled,
		vocabulary.WithDescription("System prompt assembled from domain catalog"),
		vocabulary.WithDataType("PromptAssembledPayload"),
	)

	// Execution predicates
	vocabulary.Register(PredicateExecutionStarted,
		vocabulary.WithDescription("Quest execution started by agent"),
		vocabulary.WithDataType("ExecutionStartedPayload"),
	)
	vocabulary.Register(PredicateExecutionCompleted,
		vocabulary.WithDescription("Quest execution completed successfully"),
		vocabulary.WithDataType("ExecutionCompletedPayload"),
	)
	vocabulary.Register(PredicateExecutionFailed,
		vocabulary.WithDescription("Quest execution failed"),
		vocabulary.WithDataType("ExecutionFailedPayload"),
	)
	vocabulary.Register(PredicateToolCall,
		vocabulary.WithDescription("Tool invoked during execution"),
		vocabulary.WithDataType("ToolCallPayload"),
	)
	vocabulary.Register(PredicateToolResult,
		vocabulary.WithDescription("Tool result returned during execution"),
		vocabulary.WithDataType("ToolResultPayload"),
	)

	// Agent autonomy predicates
	vocabulary.Register(PredicateAutonomyEvaluated,
		vocabulary.WithDescription("Heartbeat fired, agent autonomy evaluated"),
		vocabulary.WithDataType("AutonomyEvaluatedPayload"),
	)
	vocabulary.Register(PredicateAutonomyIdle,
		vocabulary.WithDescription("Agent idle with nothing actionable"),
		vocabulary.WithDataType("AutonomyIdlePayload"),
	)
	vocabulary.Register(PredicateAutonomyClaimIntent,
		vocabulary.WithDescription("Agent intends to claim a quest"),
		vocabulary.WithDataType("ClaimIntentPayload"),
	)
	vocabulary.Register(PredicateAutonomyShopIntent,
		vocabulary.WithDescription("Agent intends to purchase an item"),
		vocabulary.WithDataType("ShopIntentPayload"),
	)
	vocabulary.Register(PredicateAutonomyGuildIntent,
		vocabulary.WithDescription("Agent intends to join a guild"),
		vocabulary.WithDataType("GuildIntentPayload"),
	)
	vocabulary.Register(PredicateAutonomyUseIntent,
		vocabulary.WithDescription("Agent intends to use a consumable"),
		vocabulary.WithDataType("UseIntentPayload"),
	)

	// Store predicates
	vocabulary.Register(PredicateStoreItemListed,
		vocabulary.WithDescription("New item added to store catalog"),
		vocabulary.WithDataType("StoreItemListedPayload"),
	)
	vocabulary.Register(PredicateStoreItemPurchased,
		vocabulary.WithDescription("Agent purchased an item from the store"),
		vocabulary.WithDataType("StorePurchasePayload"),
	)
	vocabulary.Register(PredicateStoreItemUsed,
		vocabulary.WithDescription("Rental item use consumed"),
	)
	vocabulary.Register(PredicateStoreItemExpired,
		vocabulary.WithDescription("Rental item ran out of uses"),
	)
	vocabulary.Register(PredicateConsumableUsed,
		vocabulary.WithDescription("Consumable item activated"),
		vocabulary.WithDataType("ConsumableUsedPayload"),
	)
	vocabulary.Register(PredicateConsumableExpired,
		vocabulary.WithDescription("Consumable effect wore off"),
		vocabulary.WithDataType("ConsumableExpiredPayload"),
	)
	vocabulary.Register(PredicateInventoryUpdated,
		vocabulary.WithDescription("Agent inventory changed"),
		vocabulary.WithDataType("InventoryUpdatedPayload"),
	)

	// Peer review predicates
	vocabulary.Register(PredicateReviewPending,
		vocabulary.WithDescription("Peer review created, waiting for submissions"),
		vocabulary.WithDataType("PeerReviewPayload"),
	)
	vocabulary.Register(PredicateReviewSubmitted,
		vocabulary.WithDescription("One side submitted their peer review"),
		vocabulary.WithDataType("PeerReviewPayload"),
	)
	vocabulary.Register(PredicateReviewCompleted,
		vocabulary.WithDescription("Both sides submitted, peer review complete"),
		vocabulary.WithDataType("PeerReviewPayload"),
	)
}
