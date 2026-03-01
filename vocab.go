package semdragons

import (
	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// VOCABULARY - Aliases from domain/ package (single source of truth)
// =============================================================================

// PredicateQuestPosted and related constants define quest lifecycle predicates.
const (
	PredicateQuestPosted     = domain.PredicateQuestPosted
	PredicateQuestClaimed    = domain.PredicateQuestClaimed
	PredicateQuestStarted    = domain.PredicateQuestStarted
	PredicateQuestSubmitted  = domain.PredicateQuestSubmitted
	PredicateQuestCompleted  = domain.PredicateQuestCompleted
	PredicateQuestFailed     = domain.PredicateQuestFailed
	PredicateQuestEscalated  = domain.PredicateQuestEscalated
	PredicateQuestAbandoned  = domain.PredicateQuestAbandoned
	PredicateQuestDecomposed = domain.PredicateQuestDecomposed
)

// PredicateBattleStarted and related constants define boss battle predicates.
const (
	PredicateBattleStarted = domain.PredicateBattleStarted
	PredicateBattleVerdict = domain.PredicateBattleVerdict
	PredicateBattleVictory = domain.PredicateBattleVictory
	PredicateBattleDefeat  = domain.PredicateBattleDefeat
	PredicateBattleRetreat = domain.PredicateBattleRetreat
)

// PredicateAgentXP and related constants define agent progression predicates.
const (
	PredicateAgentXP        = domain.PredicateAgentXP
	PredicateAgentLevelUp   = domain.PredicateAgentLevelUp
	PredicateAgentLevelDown = domain.PredicateAgentLevelDown
	PredicateAgentDeath     = domain.PredicateAgentDeath
	PredicateAgentCooldown  = domain.PredicateAgentCooldown
	PredicateAgentReady     = domain.PredicateAgentReady
)

// PredicateSkillImproved and related constants define skill progression predicates.
const (
	PredicateSkillImproved = domain.PredicateSkillImproved
	PredicateSkillLevelUp  = domain.PredicateSkillLevelUp
	PredicateMentorBonus   = domain.PredicateMentorBonus
)

// PredicateSeedingStarted and related constants define seeding predicates.
const (
	PredicateSeedingStarted      = domain.PredicateSeedingStarted
	PredicateSeedingCompleted    = domain.PredicateSeedingCompleted
	PredicateArenaRoundStarted   = domain.PredicateArenaRoundStarted
	PredicateArenaRoundCompleted = domain.PredicateArenaRoundCompleted
	PredicateRosterAgentCreated  = domain.PredicateRosterAgentCreated
	PredicateNPCSpawned          = domain.PredicateNPCSpawned
	PredicateNPCRetired          = domain.PredicateNPCRetired
)

// PredicatePartyFormed and related constants define party predicates.
const (
	PredicatePartyFormed    = domain.PredicatePartyFormed
	PredicatePartyDisbanded = domain.PredicatePartyDisbanded
	PredicatePartyJoined    = domain.PredicatePartyJoined
	PredicatePartyLeft      = domain.PredicatePartyLeft
)

// PredicatePartyQuestDecomposed and related constants define party coordination predicates.
const (
	PredicatePartyQuestDecomposed  = domain.PredicatePartyQuestDecomposed
	PredicatePartyTaskAssigned     = domain.PredicatePartyTaskAssigned
	PredicatePartyContextShared    = domain.PredicatePartyContextShared
	PredicatePartyGuidanceIssued   = domain.PredicatePartyGuidanceIssued
	PredicatePartyProgressReported = domain.PredicatePartyProgressReported
	PredicatePartyHelpRequested    = domain.PredicatePartyHelpRequested
	PredicatePartyResultSubmitted  = domain.PredicatePartyResultSubmitted
	PredicatePartyRollupStarted    = domain.PredicatePartyRollupStarted
	PredicatePartyRollupCompleted  = domain.PredicatePartyRollupCompleted
)

// PredicateGuildCreated and related constants define guild predicates.
const (
	PredicateGuildCreated  = domain.PredicateGuildCreated
	PredicateGuildJoined   = domain.PredicateGuildJoined
	PredicateGuildLeft     = domain.PredicateGuildLeft
	PredicateGuildPromoted = domain.PredicateGuildPromoted
	PredicateGuildDemoted  = domain.PredicateGuildDemoted
)

// PredicateGuildSuggested and related constants define guild formation predicates.
const (
	PredicateGuildSuggested  = domain.PredicateGuildSuggested
	PredicateGuildAutoJoined = domain.PredicateGuildAutoJoined
)

// PredicateSessionStart and related constants define DM session predicates.
const (
	PredicateSessionStart   = domain.PredicateSessionStart
	PredicateSessionEnd     = domain.PredicateSessionEnd
	PredicateDMIntervention = domain.PredicateDMIntervention
	PredicateDMEscalation   = domain.PredicateDMEscalation
)

// PredicateApprovalRequested and related constants define approval predicates.
const (
	PredicateApprovalRequested = domain.PredicateApprovalRequested
	PredicateApprovalResolved  = domain.PredicateApprovalResolved
)

// PredicateExecutionStarted and related constants define execution predicates.
const (
	PredicateExecutionStarted   = domain.PredicateExecutionStarted
	PredicateExecutionCompleted = domain.PredicateExecutionCompleted
	PredicateExecutionFailed    = domain.PredicateExecutionFailed
	PredicateToolCall           = domain.PredicateToolCall
	PredicateToolResult         = domain.PredicateToolResult
)

// PredicateStoreItemListed and related constants define store predicates.
const (
	PredicateStoreItemListed    = domain.PredicateStoreItemListed
	PredicateStoreItemPurchased = domain.PredicateStoreItemPurchased
	PredicateStoreItemUsed      = domain.PredicateStoreItemUsed
	PredicateStoreItemExpired   = domain.PredicateStoreItemExpired
	PredicateConsumableUsed     = domain.PredicateConsumableUsed
	PredicateConsumableExpired  = domain.PredicateConsumableExpired
	PredicateInventoryUpdated   = domain.PredicateInventoryUpdated
)

// RegisterVocabulary registers all semdragons predicates with the vocabulary system.
// Delegates to domain.RegisterVocabulary() which is the single source of truth.
func RegisterVocabulary() {
	domain.RegisterVocabulary()
}
