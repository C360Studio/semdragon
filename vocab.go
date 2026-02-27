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
}
