package domain

import (
	"github.com/c360studio/semdragons/internal/semcompat"
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
	PredicateQuestPendingTriage = "quest.lifecycle.pending-triage"

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
	PredicateQuestMetricsTurns = "quest.metrics.turns-used"

	// PredicateQuestMetricsTokensIn - Prompt tokens consumed during execution.
	PredicateQuestMetricsTokensIn = "quest.metrics.tokens-prompt"

	// PredicateQuestMetricsTokensOut - Completion tokens consumed during execution.
	PredicateQuestMetricsTokensOut = "quest.metrics.tokens-completion"
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

// --- Agent Identity Predicates ---

const (
	// PredicateAgentArchetype - Agent class identity (scholar, engineer, scribe, strategist).
	// Fixed at creation; never changes on level-up.
	PredicateAgentArchetype = "agent.identity.archetype"
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

// --- Execution Predicates ---

const (
	// PredicateExecutionStarted - Quest execution started.
	PredicateExecutionStarted = "execution.lifecycle.started"

	// PredicateExecutionAgentID - Agent executing the quest.
	PredicateExecutionAgentID = "execution.agent.id"

	// PredicateExecutionLoopID - Agentic loop identifier.
	PredicateExecutionLoopID = "execution.loop.id"

	// PredicateExecutionConfigMaxTurns - Maximum configured execution turns.
	PredicateExecutionConfigMaxTurns = "execution.config.max-turns"

	// PredicateExecutionConfigMaxTokens - Maximum configured execution tokens.
	PredicateExecutionConfigMaxTokens = "execution.config.max-tokens"

	// PredicateExecutionToolsAvailable - Number of tools available to the execution.
	PredicateExecutionToolsAvailable = "execution.tools.available"

	// PredicateExecutionCompleted - Quest execution completed successfully.
	PredicateExecutionCompleted = "execution.lifecycle.completed"

	// PredicateExecutionStatusFinal - Final execution status.
	PredicateExecutionStatusFinal = "execution.status.final"

	// PredicateExecutionMetricsTurns - Number of execution turns.
	PredicateExecutionMetricsTurns = "execution.metrics.turns"

	// PredicateExecutionMetricsToolCalls - Number of execution tool calls.
	PredicateExecutionMetricsToolCalls = "execution.metrics.tool-calls"

	// PredicateExecutionTokensPrompt - Prompt tokens consumed by the execution.
	PredicateExecutionTokensPrompt = "execution.tokens.prompt"

	// PredicateExecutionTokensCompletion - Completion tokens consumed by the execution.
	PredicateExecutionTokensCompletion = "execution.tokens.completion"

	// PredicateExecutionDurationMS - Execution duration in milliseconds.
	PredicateExecutionDurationMS = "execution.duration.ms"

	// PredicateExecutionFailed - Quest execution failed.
	PredicateExecutionFailed = "execution.lifecycle.failed"

	// PredicateExecutionFailureError - Final execution failure detail.
	PredicateExecutionFailureError = "execution.failure.error"
)

// --- Source Document Predicates ---

const (
	// PredicateSourceDocumentContent - Fetched document content.
	PredicateSourceDocumentContent = "source.doc.content"

	// PredicateSourceDocumentSummary - Fetched document summary.
	PredicateSourceDocumentSummary = "source.doc.summary"

	// PredicateSourceDocumentURL - Fetched document source URL.
	PredicateSourceDocumentURL = "source.doc.url"

	// PredicateSourceDocumentMIMEType - Fetched document MIME type.
	PredicateSourceDocumentMIMEType = "source.doc.mime-type"

	// PredicateSourceDocumentScope - Visibility scope for the fetched document.
	PredicateSourceDocumentScope = "source.doc.scope"

	// PredicateSourceDocumentFetchedBy - Agent that fetched the document.
	PredicateSourceDocumentFetchedBy = "source.doc.fetched-by"

	// PredicateSourceDocumentQuestID - Quest associated with the fetched document.
	PredicateSourceDocumentQuestID = "source.doc.quest-id"
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

// Agent autonomy graph predicates. These describe the latent Graphable
// projection emitted by the seven raw typed-subject payloads; the event
// subjects above remain unchanged.
const (
	PredicateAutonomyStatus                = "agent.autonomy.status"
	PredicateAutonomyAction                = "agent.autonomy.action"
	PredicateAutonomyIntervalMS            = "agent.autonomy.interval-ms"
	PredicateAutonomyIdleDurationMS        = "agent.autonomy.idle-duration-ms"
	PredicateAutonomyHasSuggestion         = "agent.autonomy.has-suggestion"
	PredicateAutonomyBackoffMS             = "agent.autonomy.backoff-ms"
	PredicateAutonomyClaimQuest            = "agent.autonomy.claim-quest"
	PredicateAutonomyClaimScore            = "agent.autonomy.claim-score"
	PredicateAutonomyClaimRank             = "agent.autonomy.claim-rank"
	PredicateAutonomyShopItem              = "agent.autonomy.shop-item"
	PredicateAutonomyShopCost              = "agent.autonomy.shop-cost"
	PredicateAutonomyShopBudget            = "agent.autonomy.shop-budget"
	PredicateAutonomyShopStrategic         = "agent.autonomy.shop-strategic"
	PredicateAutonomyGuildJoin             = "agent.autonomy.guild-join"
	PredicateAutonomyGuildScore            = "agent.autonomy.guild-score"
	PredicateAutonomyGuildChoices          = "agent.autonomy.guild-choices"
	PredicateAutonomyUseConsumable         = "agent.autonomy.use-consumable"
	PredicateAutonomyUseStatus             = "agent.autonomy.use-status"
	PredicateAutonomyUseQuest              = "agent.autonomy.use-quest"
	PredicateAutonomyGuildCreate           = "agent.autonomy.guild-create"
	PredicateAutonomyGuildCreateCandidates = "agent.autonomy.guild-create-candidates"
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

	// Agent identity predicates
	vocabulary.Register(PredicateAgentArchetype,
		vocabulary.WithDescription("Agent class identity (scholar, engineer, scribe, strategist) — fixed at creation"),
		vocabulary.WithDataType("string"),
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

	// Execution predicates
	vocabulary.Register(PredicateExecutionStarted,
		vocabulary.WithDescription("Quest execution started by agent"),
		vocabulary.WithDataType("ExecutionStartedPayload"),
	)
	vocabulary.Register(PredicateExecutionAgentID,
		vocabulary.WithDescription("Agent executing the quest"),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateExecutionLoopID,
		vocabulary.WithDescription("Agentic loop identifier"),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateExecutionConfigMaxTurns,
		vocabulary.WithDescription("Maximum configured execution turns"),
		vocabulary.WithDataType("int"),
	)
	vocabulary.Register(PredicateExecutionConfigMaxTokens,
		vocabulary.WithDescription("Maximum configured execution tokens"),
		vocabulary.WithDataType("int"),
	)
	vocabulary.Register(PredicateExecutionToolsAvailable,
		vocabulary.WithDescription("Number of tools available to the execution"),
		vocabulary.WithDataType("int"),
	)
	vocabulary.Register(PredicateExecutionCompleted,
		vocabulary.WithDescription("Quest execution completed successfully"),
		vocabulary.WithDataType("ExecutionCompletedPayload"),
	)
	vocabulary.Register(PredicateExecutionStatusFinal,
		vocabulary.WithDescription("Final execution status"),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateExecutionMetricsTurns,
		vocabulary.WithDescription("Number of execution turns"),
		vocabulary.WithDataType("int"),
	)
	vocabulary.Register(PredicateExecutionMetricsToolCalls,
		vocabulary.WithDescription("Number of execution tool calls"),
		vocabulary.WithDataType("int"),
	)
	vocabulary.Register(PredicateExecutionTokensPrompt,
		vocabulary.WithDescription("Prompt tokens consumed by the execution"),
		vocabulary.WithDataType("int"),
	)
	vocabulary.Register(PredicateExecutionTokensCompletion,
		vocabulary.WithDescription("Completion tokens consumed by the execution"),
		vocabulary.WithDataType("int"),
	)
	vocabulary.Register(PredicateExecutionDurationMS,
		vocabulary.WithDescription("Execution duration in milliseconds"),
		vocabulary.WithDataType("int64"),
	)
	vocabulary.Register(PredicateExecutionFailed,
		vocabulary.WithDescription("Quest execution failed"),
		vocabulary.WithDataType("ExecutionFailedPayload"),
	)
	vocabulary.Register(PredicateExecutionFailureError,
		vocabulary.WithDescription("Final execution failure detail"),
		vocabulary.WithDataType("string"),
	)

	// Source document predicates
	vocabulary.Register(PredicateSourceDocumentContent,
		vocabulary.WithDescription("Fetched document content"),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateSourceDocumentSummary,
		vocabulary.WithDescription("Fetched document summary"),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateSourceDocumentURL,
		vocabulary.WithDescription("Fetched document source URL"),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateSourceDocumentMIMEType,
		vocabulary.WithDescription("Fetched document MIME type"),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateSourceDocumentScope,
		vocabulary.WithDescription("Visibility scope for the fetched document"),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateSourceDocumentFetchedBy,
		vocabulary.WithDescription("Agent that fetched the document"),
		vocabulary.WithDataType("string"),
	)
	vocabulary.Register(PredicateSourceDocumentQuestID,
		vocabulary.WithDescription("Quest associated with the fetched document"),
		vocabulary.WithDataType("string"),
	)

	// Beta.135 compatibility only. SemStreams #673 owns this standard
	// dc.terms.modified declaration and registration:
	// https://github.com/C360Studio/semstreams/issues/673
	vocabulary.Register(semcompat.DCTermsModified,
		vocabulary.WithDescription("Beta.135 compatibility declaration pending SemStreams #673"),
		vocabulary.WithDataType("string"),
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
	vocabulary.Register(PredicateAutonomyStatus,
		vocabulary.WithDescription("Agent status observed during autonomy evaluation"),
	)
	vocabulary.Register(PredicateAutonomyAction,
		vocabulary.WithDescription("Action selected during autonomy evaluation"),
	)
	vocabulary.Register(PredicateAutonomyIntervalMS,
		vocabulary.WithDescription("Autonomy heartbeat interval in milliseconds"),
	)
	vocabulary.Register(PredicateAutonomyIdleDurationMS,
		vocabulary.WithDescription("Agent idle duration in milliseconds"),
	)
	vocabulary.Register(PredicateAutonomyHasSuggestion,
		vocabulary.WithDescription("Whether the idle agent had an actionable suggestion"),
	)
	vocabulary.Register(PredicateAutonomyBackoffMS,
		vocabulary.WithDescription("Next autonomy backoff interval in milliseconds"),
	)
	vocabulary.Register(PredicateAutonomyClaimQuest,
		vocabulary.WithDescription("Quest selected by an autonomous claim action"),
	)
	vocabulary.Register(PredicateAutonomyClaimScore,
		vocabulary.WithDescription("Suggestion score for an autonomous quest claim"),
	)
	vocabulary.Register(PredicateAutonomyClaimRank,
		vocabulary.WithDescription("Suggestion rank for an autonomous quest claim"),
	)
	vocabulary.Register(PredicateAutonomyShopItem,
		vocabulary.WithDescription("Scalar store catalog alias selected for purchase"),
	)
	vocabulary.Register(PredicateAutonomyShopCost,
		vocabulary.WithDescription("XP cost of an autonomous purchase"),
	)
	vocabulary.Register(PredicateAutonomyShopBudget,
		vocabulary.WithDescription("XP budget available for an autonomous purchase"),
	)
	vocabulary.Register(PredicateAutonomyShopStrategic,
		vocabulary.WithDescription("Whether an autonomous purchase was strategic"),
	)
	vocabulary.Register(PredicateAutonomyGuildJoin,
		vocabulary.WithDescription("Guild selected by an autonomous join action"),
	)
	vocabulary.Register(PredicateAutonomyGuildScore,
		vocabulary.WithDescription("Score of the selected guild"),
	)
	vocabulary.Register(PredicateAutonomyGuildChoices,
		vocabulary.WithDescription("Number of guild choices evaluated"),
	)
	vocabulary.Register(PredicateAutonomyUseConsumable,
		vocabulary.WithDescription("Scalar consumable catalog alias selected for use"),
	)
	vocabulary.Register(PredicateAutonomyUseStatus,
		vocabulary.WithDescription("Agent status when a consumable was used"),
	)
	vocabulary.Register(PredicateAutonomyUseQuest,
		vocabulary.WithDescription("Quest active when a consumable was used"),
	)
	vocabulary.Register(PredicateAutonomyGuildCreate,
		vocabulary.WithDescription("Name proposed for an autonomous guild creation"),
	)
	vocabulary.Register(PredicateAutonomyGuildCreateCandidates,
		vocabulary.WithDescription("Number of candidates considered for guild creation"),
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
