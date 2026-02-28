package seeding

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/c360studio/semdragons"
	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/model"
	agenticmodel "github.com/c360studio/semstreams/processor/agentic-model"
)

// =============================================================================
// ARENA JUDGE - LLM-based evaluation for training
// =============================================================================

// ArenaJudge evaluates training quest results using an LLM.
type ArenaJudge struct {
	registry model.RegistryReader
	config   ArenaJudgeConfig
}

// ArenaJudgeConfig holds configuration for the arena judge.
type ArenaJudgeConfig struct {
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"max_tokens"`
}

// DefaultArenaJudgeConfig returns sensible defaults for arena evaluation.
func DefaultArenaJudgeConfig() ArenaJudgeConfig {
	return ArenaJudgeConfig{
		Temperature: 0.0, // Deterministic for evaluation
		MaxTokens:   1024,
	}
}

// NewArenaJudge creates a new arena judge with the given model registry.
func NewArenaJudge(registry model.RegistryReader) *ArenaJudge {
	return &ArenaJudge{
		registry: registry,
		config:   DefaultArenaJudgeConfig(),
	}
}

// NewArenaJudgeWithConfig creates an arena judge with custom configuration.
func NewArenaJudgeWithConfig(registry model.RegistryReader, config ArenaJudgeConfig) *ArenaJudge {
	return &ArenaJudge{
		registry: registry,
		config:   config,
	}
}

// JudgeResult holds the evaluation outcome.
type JudgeResult struct {
	Passed       bool               `json:"passed"`
	QualityScore float64            `json:"quality_score"`
	Feedback     string             `json:"feedback"`
	Criteria     map[string]bool    `json:"criteria"`
	Scores       map[string]float64 `json:"scores"`
}

// Evaluate judges a quest result against the template criteria.
func (j *ArenaJudge) Evaluate(ctx context.Context, template *QuestTemplate, result any) (*JudgeResult, error) {
	// If no registry, use stub behavior for testing
	if j.registry == nil {
		return j.stubEvaluate(template)
	}

	// Get endpoint for training evaluation
	endpointName := j.registry.Resolve("training-eval")
	endpoint := j.registry.GetEndpoint(endpointName)
	if endpoint == nil {
		endpoint = j.registry.GetEndpoint(j.registry.GetDefault())
	}
	if endpoint == nil {
		return j.stubEvaluate(template)
	}

	// Create client
	client, err := agenticmodel.NewClient(endpoint)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}
	defer client.Close()

	// Build evaluation prompt
	systemPrompt := j.buildSystemPrompt()
	userPrompt := j.buildUserPrompt(template, result)

	// Call LLM
	resp, err := client.ChatCompletion(ctx, agentic.AgentRequest{
		RequestID:   fmt.Sprintf("arena-judge-%s", template.ID),
		Role:        agentic.RoleGeneral,
		Messages:    []agentic.ChatMessage{{Role: "system", Content: systemPrompt}, {Role: "user", Content: userPrompt}},
		Model:       endpoint.Model,
		MaxTokens:   j.config.MaxTokens,
		Temperature: j.config.Temperature,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM request failed: %w", err)
	}

	if resp.Status == agentic.StatusError {
		return nil, fmt.Errorf("LLM error: %s", resp.Error)
	}

	return j.parseEvaluationResponse(resp.Message.Content, template)
}

// EvaluateWithRubric evaluates using detailed scoring rubrics.
func (j *ArenaJudge) EvaluateWithRubric(ctx context.Context, quest *semdragons.Quest, result any, rubric []semdragons.ReviewCriterion) (*JudgeResult, error) {
	// If no registry, use stub behavior for testing
	if j.registry == nil {
		return j.stubEvaluateWithRubric(rubric)
	}

	// Get endpoint
	endpointName := j.registry.Resolve("training-eval")
	endpoint := j.registry.GetEndpoint(endpointName)
	if endpoint == nil {
		endpoint = j.registry.GetEndpoint(j.registry.GetDefault())
	}
	if endpoint == nil {
		return j.stubEvaluateWithRubric(rubric)
	}

	// Create client
	client, err := agenticmodel.NewClient(endpoint)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}
	defer client.Close()

	// Build evaluation prompt with rubric
	systemPrompt := j.buildRubricSystemPrompt()
	userPrompt := j.buildRubricUserPrompt(quest, result, rubric)

	// Call LLM
	resp, err := client.ChatCompletion(ctx, agentic.AgentRequest{
		RequestID:   fmt.Sprintf("arena-judge-rubric-%s", quest.ID),
		Role:        agentic.RoleGeneral,
		Messages:    []agentic.ChatMessage{{Role: "system", Content: systemPrompt}, {Role: "user", Content: userPrompt}},
		Model:       endpoint.Model,
		MaxTokens:   j.config.MaxTokens,
		Temperature: j.config.Temperature,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM request failed: %w", err)
	}

	if resp.Status == agentic.StatusError {
		return nil, fmt.Errorf("LLM error: %s", resp.Error)
	}

	return j.parseRubricResponse(resp.Message.Content, rubric)
}

// --- Stub implementations for testing without LLM ---

func (j *ArenaJudge) stubEvaluate(template *QuestTemplate) (*JudgeResult, error) {
	// Simulate reasonable scores based on template criteria
	scores := make(map[string]float64)
	criteria := make(map[string]bool)

	for _, c := range template.Criteria {
		score := 0.8 // Training quests generally pass
		scores[c] = score
		criteria[c] = true
	}

	return &JudgeResult{
		Passed:       true,
		QualityScore: 0.8,
		Feedback:     "Training quest completed (stub evaluation - no LLM registry configured).",
		Criteria:     criteria,
		Scores:       scores,
	}, nil
}

func (j *ArenaJudge) stubEvaluateWithRubric(rubric []semdragons.ReviewCriterion) (*JudgeResult, error) {
	scores := make(map[string]float64)
	criteria := make(map[string]bool)
	var totalScore float64
	var totalWeight float64

	for _, criterion := range rubric {
		score := 0.8 // Training quests generally pass
		scores[criterion.Name] = score
		criteria[criterion.Name] = score >= criterion.Threshold
		totalScore += score * criterion.Weight
		totalWeight += criterion.Weight
	}

	avgScore := 0.8
	if totalWeight > 0 {
		avgScore = totalScore / totalWeight
	}

	return &JudgeResult{
		Passed:       true,
		QualityScore: avgScore,
		Feedback:     "Evaluation completed (stub - no LLM registry configured).",
		Criteria:     criteria,
		Scores:       scores,
	}, nil
}

// --- Prompt building ---

func (j *ArenaJudge) buildSystemPrompt() string {
	return `You are an expert evaluator for agent training quests.
Evaluate the quest output against the given criteria and provide a structured assessment.

You MUST respond with valid JSON in this exact format:
{
  "passed": true/false,
  "quality_score": 0.0-1.0,
  "feedback": "detailed feedback on the output quality",
  "criteria": {"criterion_name": true/false, ...},
  "scores": {"criterion_name": 0.0-1.0, ...}
}

Be fair but rigorous. Training should push agents to improve.`
}

func (j *ArenaJudge) buildUserPrompt(template *QuestTemplate, result any) string {
	var sb strings.Builder

	sb.WriteString("## Quest Template\n")
	sb.WriteString(fmt.Sprintf("**ID:** %s\n", template.ID))
	sb.WriteString(fmt.Sprintf("**Title:** %s\n", template.Title))
	sb.WriteString(fmt.Sprintf("**Description:** %s\n", template.Description))
	sb.WriteString(fmt.Sprintf("**Difficulty:** %d\n\n", template.Difficulty))

	sb.WriteString("## Expected Criteria\n")
	for _, c := range template.Criteria {
		sb.WriteString(fmt.Sprintf("- %s\n", c))
	}
	sb.WriteString("\n")

	sb.WriteString("## Agent Output\n```\n")
	sb.WriteString(formatOutput(result))
	sb.WriteString("\n```\n\n")

	sb.WriteString("Evaluate this output against the criteria above.")

	return sb.String()
}

func (j *ArenaJudge) buildRubricSystemPrompt() string {
	return `You are an expert evaluator for agent quest output.
Evaluate the output against each criterion in the provided rubric.

You MUST respond with valid JSON in this exact format:
{
  "passed": true/false (all criteria met their thresholds),
  "quality_score": 0.0-1.0 (weighted average),
  "feedback": "overall assessment",
  "criteria": {"criterion_name": true/false, ...},
  "scores": {"criterion_name": 0.0-1.0, ...}
}

Be objective and consistent in your scoring.`
}

func (j *ArenaJudge) buildRubricUserPrompt(quest *semdragons.Quest, result any, rubric []semdragons.ReviewCriterion) string {
	var sb strings.Builder

	sb.WriteString("## Quest\n")
	sb.WriteString(fmt.Sprintf("**Title:** %s\n", quest.Title))
	sb.WriteString(fmt.Sprintf("**Description:** %s\n", quest.Description))
	sb.WriteString(fmt.Sprintf("**Difficulty:** %d\n\n", quest.Difficulty))

	sb.WriteString("## Evaluation Rubric\n")
	for _, c := range rubric {
		sb.WriteString(fmt.Sprintf("- **%s** (weight: %.2f, threshold: %.2f): %s\n",
			c.Name, c.Weight, c.Threshold, c.Description))
	}
	sb.WriteString("\n")

	sb.WriteString("## Agent Output\n```\n")
	sb.WriteString(formatOutput(result))
	sb.WriteString("\n```\n\n")

	sb.WriteString("Score each criterion and provide your overall assessment.")

	return sb.String()
}

// --- Response parsing ---

func (j *ArenaJudge) parseEvaluationResponse(content string, template *QuestTemplate) (*JudgeResult, error) {
	jsonStr := semdragons.ExtractJSONFromLLMResponse(content)

	var raw struct {
		Passed       bool               `json:"passed"`
		QualityScore float64            `json:"quality_score"`
		Feedback     string             `json:"feedback"`
		Criteria     map[string]bool    `json:"criteria"`
		Scores       map[string]float64 `json:"scores"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		semdragons.LogLLMParseFailure("arena_judge_evaluation", err, content, jsonStr)
		return j.stubEvaluate(template)
	}

	// Normalize quality score
	score := raw.QualityScore
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	return &JudgeResult{
		Passed:       raw.Passed,
		QualityScore: score,
		Feedback:     raw.Feedback,
		Criteria:     raw.Criteria,
		Scores:       raw.Scores,
	}, nil
}

func (j *ArenaJudge) parseRubricResponse(content string, rubric []semdragons.ReviewCriterion) (*JudgeResult, error) {
	jsonStr := semdragons.ExtractJSONFromLLMResponse(content)

	var raw struct {
		Passed       bool               `json:"passed"`
		QualityScore float64            `json:"quality_score"`
		Feedback     string             `json:"feedback"`
		Criteria     map[string]bool    `json:"criteria"`
		Scores       map[string]float64 `json:"scores"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		semdragons.LogLLMParseFailure("arena_judge_rubric", err, content, jsonStr)
		return j.stubEvaluateWithRubric(rubric)
	}

	// Normalize quality score
	score := raw.QualityScore
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	// Verify all criteria from rubric are present, fill missing with false
	criteria := raw.Criteria
	if criteria == nil {
		criteria = make(map[string]bool)
	}
	scores := raw.Scores
	if scores == nil {
		scores = make(map[string]float64)
	}

	for _, c := range rubric {
		if _, ok := criteria[c.Name]; !ok {
			criteria[c.Name] = false
		}
		if _, ok := scores[c.Name]; !ok {
			scores[c.Name] = 0.0
		}
	}

	return &JudgeResult{
		Passed:       raw.Passed,
		QualityScore: score,
		Feedback:     raw.Feedback,
		Criteria:     criteria,
		Scores:       scores,
	}, nil
}

// formatOutput converts the output to a string representation.
func formatOutput(output any) string {
	if output == nil {
		return "(no output)"
	}

	if s, ok := output.(string); ok {
		return s
	}

	if b, err := json.MarshalIndent(output, "", "  "); err == nil {
		return string(b)
	}

	return fmt.Sprintf("%v", output)
}

// ToBattleVerdict converts a JudgeResult to a BattleVerdict.
func (r *JudgeResult) ToBattleVerdict(questXP int64) semdragons.BattleVerdict {
	var xpAwarded int64
	var xpPenalty int64

	if r.Passed {
		xpAwarded = int64(float64(questXP) * r.QualityScore)
	} else {
		xpPenalty = questXP / 4
	}

	return semdragons.BattleVerdict{
		Passed:       r.Passed,
		QualityScore: r.QualityScore,
		XPAwarded:    xpAwarded,
		XPPenalty:    xpPenalty,
		Feedback:     r.Feedback,
	}
}
