package seeding

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"

	"github.com/c360studio/semdragons"
)

// =============================================================================
// QUEST TEMPLATES - Training quest definitions
// =============================================================================

// QuestTemplate defines a training quest that can be instantiated.
type QuestTemplate struct {
	ID          string                     `json:"id"`
	Title       string                     `json:"title"`
	Description string                     `json:"description"`
	Difficulty  semdragons.QuestDifficulty `json:"difficulty"`
	Skills      []semdragons.SkillTag      `json:"skills"`
	Input       any                        `json:"input"`
	Criteria    []string                   `json:"criteria"`
}

// ToQuest converts a template to a Quest instance.
func (t *QuestTemplate) ToQuest() semdragons.Quest {
	return semdragons.Quest{
		Title:          t.Title,
		Description:    t.Description,
		Difficulty:     t.Difficulty,
		RequiredSkills: t.Skills,
		Input:          t.Input,
		BaseXP:         semdragons.DefaultXPForDifficulty(t.Difficulty),
		MinTier:        semdragons.TierFromDifficulty(t.Difficulty),
		Constraints: semdragons.QuestConstraints{
			RequireReview: true,
			ReviewLevel:   semdragons.ReviewStandard,
		},
		MaxAttempts: 3,
	}
}

// QuestTemplates holds a collection of quest templates.
type QuestTemplates struct {
	Domain      string          `json:"domain"`
	Description string          `json:"description"`
	Templates   []QuestTemplate `json:"quests"`

	// Index by difficulty for fast lookup
	byDifficulty map[semdragons.QuestDifficulty][]QuestTemplate
}

// SelectForLevel returns a quest template appropriate for an agent level.
func (t *QuestTemplates) SelectForLevel(level int) *QuestTemplate {
	tier := semdragons.TierFromLevel(level)

	// Map tier to appropriate difficulty
	var targetDiff semdragons.QuestDifficulty
	switch tier {
	case semdragons.TierApprentice:
		targetDiff = semdragons.DifficultyTrivial
	case semdragons.TierJourneyman:
		targetDiff = semdragons.DifficultyModerate
	case semdragons.TierExpert:
		targetDiff = semdragons.DifficultyHard
	default:
		targetDiff = semdragons.DifficultyEpic
	}

	// Find templates at or below target difficulty
	for diff := targetDiff; diff >= semdragons.DifficultyTrivial; diff-- {
		templates := t.byDifficulty[diff]
		if len(templates) > 0 {
			// Return first matching template (could add randomization)
			return &templates[0]
		}
	}

	// Fallback to any available
	if len(t.Templates) > 0 {
		return &t.Templates[0]
	}

	return nil
}

// SelectBySkill returns templates that require a specific skill.
func (t *QuestTemplates) SelectBySkill(skill semdragons.SkillTag) []QuestTemplate {
	var matching []QuestTemplate
	for _, template := range t.Templates {
		for _, s := range template.Skills {
			if s == skill {
				matching = append(matching, template)
				break
			}
		}
	}
	return matching
}

// buildIndex creates the difficulty index for fast lookups.
func (t *QuestTemplates) buildIndex() {
	t.byDifficulty = make(map[semdragons.QuestDifficulty][]QuestTemplate)
	for _, template := range t.Templates {
		t.byDifficulty[template.Difficulty] = append(
			t.byDifficulty[template.Difficulty],
			template,
		)
	}
}

// =============================================================================
// TEMPLATE LOADING
// =============================================================================

//go:embed templates/*.json
var embeddedTemplates embed.FS

// LoadQuestTemplates loads templates for a domain from embedded files.
func LoadQuestTemplates(domain string) (*QuestTemplates, error) {
	filename := fmt.Sprintf("templates/%s.json", domain)

	data, err := embeddedTemplates.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to load embedded template %s: %w", domain, err)
	}

	return parseQuestTemplates(data)
}

// LoadQuestTemplatesFromFile loads templates from a file path.
func LoadQuestTemplatesFromFile(path string) (*QuestTemplates, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read template file %s: %w", path, err)
	}

	return parseQuestTemplates(data)
}

func parseQuestTemplates(data []byte) (*QuestTemplates, error) {
	var templates QuestTemplates
	if err := json.Unmarshal(data, &templates); err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}

	templates.buildIndex()
	return &templates, nil
}

// =============================================================================
// BUILT-IN TEMPLATES (fallback if embedded files not available)
// =============================================================================

// DefaultCodeTemplates returns built-in code training templates.
func DefaultCodeTemplates() *QuestTemplates {
	templates := &QuestTemplates{
		Domain:      "code",
		Description: "Software development training quests",
		Templates: []QuestTemplate{
			{
				ID:          "code-trivial-classify",
				Title:       "Classify Code Snippet",
				Description: "Identify the programming language and purpose of the given code.",
				Difficulty:  semdragons.DifficultyTrivial,
				Skills:      []semdragons.SkillTag{semdragons.SkillAnalysis},
				Input: map[string]any{
					"code": "func add(a, b int) int { return a + b }",
					"task": "Identify the language and describe what this code does.",
				},
				Criteria: []string{"correct_language", "accurate_description"},
			},
			{
				ID:          "code-easy-unittest",
				Title:       "Write Unit Test",
				Description: "Write a unit test for the given function.",
				Difficulty:  semdragons.DifficultyEasy,
				Skills:      []semdragons.SkillTag{semdragons.SkillCodeGen},
				Input: map[string]any{
					"function": "func Reverse(s string) string { ... }",
					"task":     "Write comprehensive unit tests for this function.",
				},
				Criteria: []string{"tests_compile", "covers_edge_cases", "assertions_correct"},
			},
			{
				ID:          "code-moderate-refactor",
				Title:       "Refactor for Clarity",
				Description: "Improve code readability without changing functionality.",
				Difficulty:  semdragons.DifficultyModerate,
				Skills:      []semdragons.SkillTag{semdragons.SkillCodeGen, semdragons.SkillCodeReview},
				Input: map[string]any{
					"code": "func f(x int) int { if x == 0 { return 1 } else { return x * f(x-1) } }",
					"task": "Refactor this function to be more readable and maintainable.",
				},
				Criteria: []string{"correctness", "readability", "naming"},
			},
			{
				ID:          "code-hard-optimize",
				Title:       "Optimize for Performance",
				Description: "Improve performance of the given code.",
				Difficulty:  semdragons.DifficultyHard,
				Skills:      []semdragons.SkillTag{semdragons.SkillCodeGen, semdragons.SkillCodeReview, semdragons.SkillAnalysis},
				Input: map[string]any{
					"code": "// O(n^2) sorting implementation...",
					"task": "Optimize this code for better performance while maintaining correctness.",
				},
				Criteria: []string{"correctness", "measurable_improvement", "code_quality"},
			},
		},
	}
	templates.buildIndex()
	return templates
}
