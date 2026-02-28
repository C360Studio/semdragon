package semdragons

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

	// PredicateQuestEscalated - Quest escalated for higher-level attention.
	PredicateQuestEscalated = "quest.lifecycle.escalated"

	// PredicateQuestAbandoned - Agent gave up on a quest.
	PredicateQuestAbandoned = "quest.lifecycle.abandoned"

	// PredicateQuestDecomposed - Quest broken into sub-quests.
	PredicateQuestDecomposed = "quest.lifecycle.decomposed"
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
		vocabulary.WithDataType("QuestPostedPayload"),
	)
	vocabulary.Register(PredicateQuestClaimed,
		vocabulary.WithDescription("Agent or party claimed a quest"),
		vocabulary.WithDataType("QuestClaimedPayload"),
	)
	vocabulary.Register(PredicateQuestStarted,
		vocabulary.WithDescription("Work began on a quest"),
		vocabulary.WithDataType("QuestStartedPayload"),
	)
	vocabulary.Register(PredicateQuestSubmitted,
		vocabulary.WithDescription("Result submitted for review"),
		vocabulary.WithDataType("QuestSubmittedPayload"),
	)
	vocabulary.Register(PredicateQuestCompleted,
		vocabulary.WithDescription("Quest finished successfully"),
		vocabulary.WithDataType("QuestCompletedPayload"),
	)
	vocabulary.Register(PredicateQuestFailed,
		vocabulary.WithDescription("Quest failed, may be re-posted"),
		vocabulary.WithDataType("QuestFailedPayload"),
	)
	vocabulary.Register(PredicateQuestEscalated,
		vocabulary.WithDescription("Quest escalated for higher-level attention"),
		vocabulary.WithDataType("QuestEscalatedPayload"),
	)
	vocabulary.Register(PredicateQuestAbandoned,
		vocabulary.WithDescription("Agent abandoned a quest"),
		vocabulary.WithDataType("QuestAbandonedPayload"),
	)
	vocabulary.Register(PredicateQuestDecomposed,
		vocabulary.WithDescription("Quest broken into sub-quests by party lead"),
		vocabulary.WithDataType("QuestDecomposedPayload"),
	)

	// Boss battle predicates
	vocabulary.Register(PredicateBattleStarted,
		vocabulary.WithDescription("Boss battle (quality review) began"),
		vocabulary.WithDataType("BattleStartedPayload"),
	)
	vocabulary.Register(PredicateBattleVerdict,
		vocabulary.WithDescription("Review verdict rendered"),
		vocabulary.WithDataType("BattleVerdictPayload"),
	)
	vocabulary.Register(PredicateBattleVictory,
		vocabulary.WithDescription("Agent passed the review"),
		vocabulary.WithDataType("BattleVictoryPayload"),
	)
	vocabulary.Register(PredicateBattleDefeat,
		vocabulary.WithDescription("Agent failed the review"),
		vocabulary.WithDataType("BattleDefeatPayload"),
	)
	vocabulary.Register(PredicateBattleRetreat,
		vocabulary.WithDescription("Agent requested re-do of submission"),
		vocabulary.WithDataType("BattleRetreatPayload"),
	)

	// Agent progression predicates
	vocabulary.Register(PredicateAgentXP,
		vocabulary.WithDescription("XP earned or lost by agent"),
		vocabulary.WithDataType("AgentXPPayload"),
	)
	vocabulary.Register(PredicateAgentLevelUp,
		vocabulary.WithDescription("Agent leveled up"),
		vocabulary.WithDataType("AgentLevelPayload"),
	)
	vocabulary.Register(PredicateAgentLevelDown,
		vocabulary.WithDescription("Agent leveled down due to poor performance"),
		vocabulary.WithDataType("AgentLevelPayload"),
	)
	vocabulary.Register(PredicateAgentDeath,
		vocabulary.WithDescription("Agent permadeath from catastrophic failure"),
		vocabulary.WithDataType("AgentDeathPayload"),
	)
	vocabulary.Register(PredicateAgentCooldown,
		vocabulary.WithDescription("Agent entered cooldown after failure"),
		vocabulary.WithDataType("AgentCooldownPayload"),
	)
	vocabulary.Register(PredicateAgentReady,
		vocabulary.WithDescription("Agent cooldown ended, ready for quests"),
		vocabulary.WithDataType("AgentReadyPayload"),
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
		vocabulary.WithDataType("SeedingStartedPayload"),
	)
	vocabulary.Register(PredicateSeedingCompleted,
		vocabulary.WithDescription("Environment seeding finished"),
		vocabulary.WithDataType("SeedingCompletedPayload"),
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
		vocabulary.WithDataType("PartyContextSharedPayload"),
	)
	vocabulary.Register(PredicatePartyGuidanceIssued,
		vocabulary.WithDescription("Lead issued guidance to party member"),
		vocabulary.WithDataType("PartyGuidanceIssuedPayload"),
	)
	vocabulary.Register(PredicatePartyProgressReported,
		vocabulary.WithDescription("Member reported progress to lead"),
		vocabulary.WithDataType("PartyProgressReportedPayload"),
	)
	vocabulary.Register(PredicatePartyHelpRequested,
		vocabulary.WithDescription("Member requested help from lead"),
		vocabulary.WithDataType("PartyHelpRequestedPayload"),
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
		vocabulary.WithDataType("StoreItemUsedPayload"),
	)
	vocabulary.Register(PredicateStoreItemExpired,
		vocabulary.WithDescription("Rental item ran out of uses"),
		vocabulary.WithDataType("StoreItemExpiredPayload"),
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
}
