package domain

// =============================================================================
// LESSON TYPES — Structured knowledge from quest reviews
// =============================================================================
// Lessons are extracted from red-team review findings and boss battle verdicts,
// indexed by (SkillTag, LessonCategory) for structured retrieval. Both positive
// (validated best practices) and negative (mistakes to avoid) lessons are stored.

// LessonCategory classifies lessons by error/quality domain, mirroring the
// review criteria taxonomy. This prevents unstructured lesson blobs.
type LessonCategory string

// Lesson category constants.
const (
	LessonCorrectness   LessonCategory = "correctness"   // Logic errors, wrong output
	LessonCompleteness  LessonCategory = "completeness"   // Missing requirements
	LessonQuality       LessonCategory = "quality"        // Code style, maintainability
	LessonSecurity      LessonCategory = "security"       // Vulnerabilities, secrets
	LessonPerformance   LessonCategory = "performance"    // Efficiency issues
	LessonIntegration   LessonCategory = "integration"    // Doesn't work with other code
	LessonDocumentation LessonCategory = "documentation"  // Missing/wrong docs
	LessonBestPractice  LessonCategory = "best_practice"  // Pattern validated as good
)

// LessonSeverity indicates the impact level of a lesson.
type LessonSeverity string

// Lesson severity constants.
const (
	LessonSeverityInfo     LessonSeverity = "info"     // Informational, nice to know
	LessonSeverityWarning  LessonSeverity = "warning"  // Should be addressed
	LessonSeverityCritical LessonSeverity = "critical" // Must be addressed, blocks quality
)

// Lesson represents a single piece of structured knowledge extracted from
// a quest review cycle. Lessons are stored on guild entities and injected
// into agent prompts for quests matching the lesson's skill tag.
type Lesson struct {
	ID           string         `json:"id"`
	Skill        SkillTag       `json:"skill"`                   // Which skill this lesson applies to
	Category     LessonCategory `json:"category"`                // Error/quality domain
	Summary      string         `json:"summary"`                 // One-line takeaway
	Detail       string         `json:"detail,omitempty"`        // Extended explanation
	Severity     LessonSeverity `json:"severity"`                // Impact level
	Positive     bool           `json:"positive"`                // true = best practice, false = mistake
	QuestID      QuestID        `json:"quest_id"`                // Source quest
	DiscoveredBy AgentID        `json:"discovered_by"`           // Agent who found this
	GuildID      GuildID        `json:"guild_id"`                // Guild that owns this lesson
	RedTeamQuest *QuestID       `json:"red_team_quest,omitempty"` // Red-team quest that surfaced it
}
