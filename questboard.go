package semdragons

import (
	"context"
	"time"
)

// =============================================================================
// QUEST BOARD - Pull-based work distribution
// =============================================================================
// The quest board is the heart of semdragons. Work is posted, agents claim it.
// No central orchestrator pushing tasks - agents pull based on capability and interest.
// This is a work-stealing scheduler wearing a tabard.
// =============================================================================

// QuestBoard is the central interface for posting, claiming, and managing quests.
type QuestBoard interface {
	// --- Posting ---

	// PostQuest adds a new quest to the board. Returns the quest with ID assigned.
	PostQuest(ctx context.Context, quest Quest) (*Quest, error)

	// PostSubQuests decomposes a parent quest into sub-quests.
	// Only agents with CanDecomposeQuest permission (Master+) can do this.
	PostSubQuests(ctx context.Context, parentID QuestID, subQuests []Quest, decomposer AgentID) ([]Quest, error)

	// --- Claiming ---

	// AvailableQuests returns quests an agent is eligible to claim.
	// Filters by: agent level/tier, skills, guild priority, cooldown status.
	AvailableQuests(ctx context.Context, agentID AgentID, opts QuestFilter) ([]Quest, error)

	// ClaimQuest assigns a quest to an agent or party.
	// Validates: tier requirements, skill match, concurrent quest limits.
	ClaimQuest(ctx context.Context, questID QuestID, agentID AgentID) error

	// ClaimQuestForParty assigns a quest to a party.
	ClaimQuestForParty(ctx context.Context, questID QuestID, partyID PartyID) error

	// AbandonQuest returns a quest to the board. Counts as a soft failure.
	AbandonQuest(ctx context.Context, questID QuestID, reason string) error

	// --- Execution ---

	// StartQuest marks a quest as in-progress. Starts the clock.
	StartQuest(ctx context.Context, questID QuestID) error

	// SubmitResult submits the output of a quest for review.
	// Triggers a boss battle if the quest requires review.
	SubmitResult(ctx context.Context, questID QuestID, result interface{}) (*BossBattle, error)

	// --- Lifecycle ---

	// CompleteQuest marks a quest as successfully completed (post-review).
	CompleteQuest(ctx context.Context, questID QuestID, verdict BattleVerdict) error

	// FailQuest marks a quest as failed. May re-post or escalate.
	FailQuest(ctx context.Context, questID QuestID, reason string) error

	// EscalateQuest flags a quest for higher-level attention (TPK scenario).
	EscalateQuest(ctx context.Context, questID QuestID, reason string) error

	// --- Queries ---

	// GetQuest returns a quest by ID.
	GetQuest(ctx context.Context, questID QuestID) (*Quest, error)

	// BoardStats returns current board statistics.
	BoardStats(ctx context.Context) (*BoardStats, error)
}

type QuestFilter struct {
	Skills       []SkillTag      `json:"skills,omitempty"`
	MinDifficulty *QuestDifficulty `json:"min_difficulty,omitempty"`
	MaxDifficulty *QuestDifficulty `json:"max_difficulty,omitempty"`
	GuildID      *GuildID         `json:"guild_id,omitempty"`
	PartyOnly    *bool            `json:"party_only,omitempty"`
	Limit        int              `json:"limit"`
}

type BoardStats struct {
	TotalPosted     int            `json:"total_posted"`
	TotalClaimed    int            `json:"total_claimed"`
	TotalInProgress int            `json:"total_in_progress"`
	TotalCompleted  int            `json:"total_completed"`
	TotalFailed     int            `json:"total_failed"`
	TotalEscalated  int            `json:"total_escalated"`
	ByDifficulty    map[QuestDifficulty]int `json:"by_difficulty"`
	BySkill         map[SkillTag]int        `json:"by_skill"`
	AvgCompletionTime time.Duration         `json:"avg_completion_time"`
}

// =============================================================================
// QUEST BUILDER - Fluent API for creating quests
// =============================================================================
// Because we already learned that fluent builders are nice.
// =============================================================================

type QuestBuilder struct {
	quest Quest
}

func NewQuest(title string) *QuestBuilder {
	return &QuestBuilder{
		quest: Quest{
			Title:    title,
			Status:   QuestPosted,
			PostedAt: time.Now(),
			Constraints: QuestConstraints{
				RequireReview: true,            // Default: always face the boss
				ReviewLevel:   ReviewStandard,  // Default: LLM judge
			},
			MaxAttempts: 3, // Default retries before escalation
		},
	}
}

func (b *QuestBuilder) Description(desc string) *QuestBuilder {
	b.quest.Description = desc
	return b
}

func (b *QuestBuilder) Difficulty(d QuestDifficulty) *QuestBuilder {
	b.quest.Difficulty = d
	b.quest.MinTier = TierFromDifficulty(d)
	return b
}

func (b *QuestBuilder) RequireSkills(skills ...SkillTag) *QuestBuilder {
	b.quest.RequiredSkills = append(b.quest.RequiredSkills, skills...)
	return b
}

func (b *QuestBuilder) RequireTools(tools ...string) *QuestBuilder {
	b.quest.RequiredTools = append(b.quest.RequiredTools, tools...)
	return b
}

func (b *QuestBuilder) RequireParty(minSize int) *QuestBuilder {
	b.quest.PartyRequired = true
	b.quest.MinPartySize = minSize
	return b
}

func (b *QuestBuilder) XP(base int64) *QuestBuilder {
	b.quest.BaseXP = base
	return b
}

func (b *QuestBuilder) BonusXP(bonus int64) *QuestBuilder {
	b.quest.BonusXP = bonus
	return b
}

func (b *QuestBuilder) WithInput(input interface{}) *QuestBuilder {
	b.quest.Input = input
	return b
}

func (b *QuestBuilder) MaxDuration(d time.Duration) *QuestBuilder {
	b.quest.Constraints.MaxDuration = d
	return b
}

func (b *QuestBuilder) MaxCost(cost float64) *QuestBuilder {
	b.quest.Constraints.MaxCost = cost
	return b
}

func (b *QuestBuilder) ReviewAs(level ReviewLevel) *QuestBuilder {
	b.quest.Constraints.ReviewLevel = level
	return b
}

func (b *QuestBuilder) NoReview() *QuestBuilder {
	b.quest.Constraints.RequireReview = false
	return b
}

func (b *QuestBuilder) Deadline(t time.Time) *QuestBuilder {
	b.quest.Deadline = &t
	return b
}

func (b *QuestBuilder) MaxRetries(n int) *QuestBuilder {
	b.quest.MaxAttempts = n
	return b
}

func (b *QuestBuilder) GuildPriority(guildID GuildID) *QuestBuilder {
	b.quest.GuildPriority = &guildID
	return b
}

func (b *QuestBuilder) AsSubQuestOf(parentID QuestID) *QuestBuilder {
	b.quest.ParentQuest = &parentID
	return b
}

func (b *QuestBuilder) Build() Quest {
	if b.quest.BaseXP == 0 {
		b.quest.BaseXP = DefaultXPForDifficulty(b.quest.Difficulty)
	}
	return b.quest
}

// --- Helpers ---

func TierFromDifficulty(d QuestDifficulty) TrustTier {
	switch {
	case d <= DifficultyEasy:
		return TierApprentice
	case d <= DifficultyModerate:
		return TierJourneyman
	case d <= DifficultyHard:
		return TierExpert
	case d <= DifficultyEpic:
		return TierMaster
	default:
		return TierGrandmaster
	}
}

func DefaultXPForDifficulty(d QuestDifficulty) int64 {
	xpTable := map[QuestDifficulty]int64{
		DifficultyTrivial:   25,
		DifficultyEasy:      50,
		DifficultyModerate:  100,
		DifficultyHard:      250,
		DifficultyEpic:      500,
		DifficultyLegendary: 1000,
	}
	if xp, ok := xpTable[d]; ok {
		return xp
	}
	return 50
}
