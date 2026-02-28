package semdragons

import (
	"math"
	"time"
)

// =============================================================================
// SKILL PROGRESSION ENGINE - Skill improvement through quest completion
// =============================================================================
// Skills improve when used in quests. Higher quality work = faster improvement.
// Diminishing returns at higher levels prevent rapid mastery.
//
// Formula: points = base * (1 + quality + difficulty_bonus) * (diminishing ^ level)
// =============================================================================

// SkillProgressionEngine calculates skill point gains and handles level-ups.
type SkillProgressionEngine struct {
	// BasePointsPerQuest is the base points earned per quest (default: 10)
	BasePointsPerQuest int

	// QualityMultiplier scales points by quality score (default: 1.0)
	// At quality=1.0, this adds 100% to base points
	QualityMultiplier float64

	// DifficultyMultiplier adds bonus per difficulty level (default: 0.2)
	// A Hard (3) quest adds 0.6 to the multiplier
	DifficultyMultiplier float64

	// DiminishingFactor reduces gains at higher levels (default: 0.8)
	// Master (L5) earns 0.8^4 = ~41% of Novice per quest
	DiminishingFactor float64

	// PointsToLevel is points needed to advance one level (default: 100)
	PointsToLevel int

	// MentorBonusRate is extra XP mentors earn when trainees improve (default: 0.25)
	MentorBonusRate float64
}

// NewSkillProgressionEngine creates an engine with sensible defaults.
func NewSkillProgressionEngine() *SkillProgressionEngine {
	return &SkillProgressionEngine{
		BasePointsPerQuest:   10,
		QualityMultiplier:    1.0,
		DifficultyMultiplier: 0.2,
		DiminishingFactor:    0.8,
		PointsToLevel:        100,
		MentorBonusRate:      0.25,
	}
}

// SkillImprovementResult holds the outcome of skill progression for one skill.
type SkillImprovementResult struct {
	Skill         SkillTag         `json:"skill"`
	PointsEarned  int              `json:"points_earned"`
	OldLevel      ProficiencyLevel `json:"old_level"`
	NewLevel      ProficiencyLevel `json:"new_level"`
	OldProgress   int              `json:"old_progress"`
	NewProgress   int              `json:"new_progress"`
	LeveledUp     bool             `json:"leveled_up"`
	AtMaxLevel    bool             `json:"at_max_level"`
}

// SkillProgressionContext contains everything needed to calculate skill improvement.
type SkillProgressionContext struct {
	Agent      *Agent        `json:"agent"`
	Quest      *Quest        `json:"quest"`
	Quality    float64       `json:"quality"`    // 0.0 - 1.0 quality score from battle
	Duration   time.Duration `json:"duration"`   // How long the quest took
	IsMentored bool          `json:"is_mentored"` // True if agent was in a mentored party
}

// ProcessQuestCompletion calculates skill improvements for all skills used in a quest.
// Returns improvement results for each skill and updates the agent's proficiencies in place.
func (e *SkillProgressionEngine) ProcessQuestCompletion(ctx SkillProgressionContext) []SkillImprovementResult {
	if ctx.Agent == nil || ctx.Quest == nil {
		return nil
	}

	// Ensure agent has proficiencies map
	if ctx.Agent.SkillProficiencies == nil {
		ctx.Agent.SkillProficiencies = make(map[SkillTag]SkillProficiency)
	}

	// Migrate legacy skills if needed
	ctx.Agent.MigrateSkills()

	var results []SkillImprovementResult
	now := time.Now()

	// Process each skill required by the quest
	for _, skill := range ctx.Quest.RequiredSkills {
		// Check if agent has this skill
		prof, exists := ctx.Agent.SkillProficiencies[skill]
		if !exists {
			// Agent used a skill they didn't have - add it at Novice
			prof = SkillProficiency{
				Level:      ProficiencyNovice,
				Progress:   0,
				TotalXP:    0,
				QuestsUsed: 0,
			}
		}

		result := e.improveSkill(&prof, skill, ctx)
		result.Skill = skill

		// Update proficiency in agent
		prof.LastUsed = &now
		prof.QuestsUsed++
		ctx.Agent.SkillProficiencies[skill] = prof

		results = append(results, result)
	}

	return results
}

// improveSkill calculates and applies improvement for a single skill.
func (e *SkillProgressionEngine) improveSkill(prof *SkillProficiency, skill SkillTag, ctx SkillProgressionContext) SkillImprovementResult {
	result := SkillImprovementResult{
		Skill:       skill,
		OldLevel:    prof.Level,
		OldProgress: prof.Progress,
	}

	// Check if already at max level
	if prof.Level >= ProficiencyMaster {
		result.NewLevel = prof.Level
		result.NewProgress = prof.Progress
		result.AtMaxLevel = true
		return result
	}

	// Calculate points earned
	points := e.calculatePoints(prof.Level, ctx.Quality, ctx.Quest.Difficulty, ctx.IsMentored)
	result.PointsEarned = points

	// Track total XP for this skill
	prof.TotalXP += int64(points)

	// Apply points to progress
	prof.Progress += points

	// Check for level up(s)
	for prof.Progress >= e.PointsToLevel && prof.Level < ProficiencyMaster {
		prof.Progress -= e.PointsToLevel
		prof.Level++
		result.LeveledUp = true
	}

	// Cap progress at max level
	if prof.Level >= ProficiencyMaster {
		prof.Level = ProficiencyMaster
		prof.Progress = 0 // No progress needed at max
		result.AtMaxLevel = true
	}

	result.NewLevel = prof.Level
	result.NewProgress = prof.Progress

	return result
}

// calculatePoints computes skill points earned for a quest completion.
// Formula: base * (1 + quality*qualityMult + difficulty*diffMult) * (diminishing ^ (level-1))
func (e *SkillProgressionEngine) calculatePoints(
	currentLevel ProficiencyLevel,
	quality float64,
	difficulty QuestDifficulty,
	isMentored bool,
) int {
	base := float64(e.BasePointsPerQuest)

	// Quality bonus: higher quality = more points
	qualityBonus := quality * e.QualityMultiplier

	// Difficulty bonus: harder quests = more points
	difficultyBonus := float64(difficulty) * e.DifficultyMultiplier

	// Combined multiplier
	multiplier := 1.0 + qualityBonus + difficultyBonus

	// Mentored bonus: learning from a mentor helps
	if isMentored {
		multiplier *= 1.2 // 20% bonus when mentored
	}

	// Diminishing returns: higher levels earn less per quest
	// Level 1 (Novice): 0.8^0 = 1.0
	// Level 2: 0.8^1 = 0.8
	// Level 3: 0.8^2 = 0.64
	// Level 4: 0.8^3 = 0.512
	// Level 5: 0.8^4 = 0.4096 (~41% of Novice)
	diminishing := math.Pow(e.DiminishingFactor, float64(currentLevel-1))

	points := base * multiplier * diminishing

	// Minimum 1 point per quest
	if points < 1 {
		points = 1
	}

	return int(math.Round(points))
}

// CalculateMentorBonus computes XP bonus for a mentor when their trainee improves.
func (e *SkillProgressionEngine) CalculateMentorBonus(traineeResults []SkillImprovementResult, questBaseXP int64) int64 {
	if len(traineeResults) == 0 {
		return 0
	}

	var totalImprovement int64
	for _, result := range traineeResults {
		totalImprovement += int64(result.PointsEarned)
		if result.LeveledUp {
			// Extra bonus for helping someone level up a skill
			totalImprovement += int64(e.PointsToLevel / 2)
		}
	}

	// Mentor bonus is a percentage of the trainee's improvement, scaled by quest XP
	bonus := float64(totalImprovement) * e.MentorBonusRate

	// Cap at quest base XP to prevent exploitation
	maxBonus := float64(questBaseXP) * 0.5
	if bonus > maxBonus {
		bonus = maxBonus
	}

	return int64(math.Round(bonus))
}

// EstimateQuestsToLevel estimates how many quests needed to reach a target level.
// Assumes average quality of 0.7 and moderate difficulty quests.
func (e *SkillProgressionEngine) EstimateQuestsToLevel(
	currentLevel ProficiencyLevel,
	targetLevel ProficiencyLevel,
) int {
	if targetLevel <= currentLevel {
		return 0
	}

	totalQuests := 0
	avgQuality := 0.7
	avgDifficulty := QuestDifficulty(2) // Moderate

	for level := currentLevel; level < targetLevel; level++ {
		pointsNeeded := e.PointsToLevel
		pointsPerQuest := e.calculatePoints(level, avgQuality, avgDifficulty, false)
		if pointsPerQuest <= 0 {
			pointsPerQuest = 1
		}
		questsForLevel := (pointsNeeded + pointsPerQuest - 1) / pointsPerQuest // Ceiling division
		totalQuests += questsForLevel
	}

	return totalQuests
}

// SkillProgressionPayload is the event payload for skill improvement events.
type SkillProgressionPayload struct {
	AgentID   AgentID                  `json:"agent_id"`
	QuestID   QuestID                  `json:"quest_id"`
	Results   []SkillImprovementResult `json:"results"`
	Timestamp time.Time                `json:"timestamp"`
}

// Validate checks that required fields are present.
func (p *SkillProgressionPayload) Validate() error {
	if p.AgentID == "" {
		return errAgentIDRequired
	}
	if p.QuestID == "" {
		return errQuestIDRequired
	}
	if p.Timestamp.IsZero() {
		return errTimestampRequired
	}
	return nil
}

// SkillLevelUpPayload is the event payload when a skill levels up.
type SkillLevelUpPayload struct {
	AgentID   AgentID          `json:"agent_id"`
	QuestID   QuestID          `json:"quest_id"`
	Skill     SkillTag         `json:"skill"`
	OldLevel  ProficiencyLevel `json:"old_level"`
	NewLevel  ProficiencyLevel `json:"new_level"`
	Timestamp time.Time        `json:"timestamp"`
}

// Validate checks that required fields are present.
func (p *SkillLevelUpPayload) Validate() error {
	if p.AgentID == "" {
		return errAgentIDRequired
	}
	if p.Skill == "" {
		return errSkillRequired
	}
	if p.Timestamp.IsZero() {
		return errTimestampRequired
	}
	return nil
}

// MentorBonusPayload is the event payload when a mentor earns XP for training.
type MentorBonusPayload struct {
	MentorID  AgentID   `json:"mentor_id"`
	TraineeID AgentID   `json:"trainee_id"`
	QuestID   QuestID   `json:"quest_id"`
	BonusXP   int64     `json:"bonus_xp"`
	Timestamp time.Time `json:"timestamp"`
}

// Validate checks that required fields are present.
func (p *MentorBonusPayload) Validate() error {
	if p.MentorID == "" {
		return errMentorIDRequired
	}
	if p.TraineeID == "" {
		return errTraineeIDRequired
	}
	if p.Timestamp.IsZero() {
		return errTimestampRequired
	}
	return nil
}

// Validation errors for skill payloads.
var (
	errAgentIDRequired   = errorString("agent_id required")
	errQuestIDRequired   = errorString("quest_id required")
	errSkillRequired     = errorString("skill required")
	errMentorIDRequired  = errorString("mentor_id required")
	errTraineeIDRequired = errorString("trainee_id required")
	errTimestampRequired = errorString("timestamp required")
)

// errorString is a simple error type to avoid allocations.
type errorString string

func (e errorString) Error() string { return string(e) }
