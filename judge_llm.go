package semdragons

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/model"
	agenticmodel "github.com/c360studio/semstreams/processor/agentic-model"
)

// =============================================================================
// LLM JUDGE - AI-powered evaluation
// =============================================================================
// The LLM judge uses an AI model to evaluate quest output against criteria.
// Uses the semstreams model infrastructure for LLM calls.
// =============================================================================

// LLMJudge evaluates output using an LLM-as-judge approach.
type LLMJudge struct {
	registry model.RegistryReader
	config   LLMJudgeConfig
}

// NewLLMJudge creates a new LLM judge using the model registry.
// If registry is nil, returns a stub judge that uses placeholder scores.
func NewLLMJudge(registry model.RegistryReader) *LLMJudge {
	return &LLMJudge{
		registry: registry,
		config:   DefaultLLMJudgeConfig(),
	}
}

// NewLLMJudgeWithConfig creates an LLM judge with custom configuration.
func NewLLMJudgeWithConfig(registry model.RegistryReader, config LLMJudgeConfig) *LLMJudge {
	return &LLMJudge{
		registry: registry,
		config:   config,
	}
}

// Type returns the judge type.
func (j *LLMJudge) Type() JudgeType {
	return JudgeLLM
}

// Evaluate runs LLM evaluation against the criterion.
func (j *LLMJudge) Evaluate(ctx context.Context, input JudgeInput) (*JudgeOutput, error) {
	// Handle nil output
	if input.Output == nil {
		return &JudgeOutput{
			Score:     0.0,
			Passed:    false,
			Reasoning: "LLM Judge: Output is nil",
			Pending:   false,
		}, nil
	}

	// If no registry, use stub behavior
	if j.registry == nil {
		return j.stubEvaluate(input)
	}

	// Get endpoint for boss battle evaluation
	endpointName := j.registry.Resolve("boss-battle")
	endpoint := j.registry.GetEndpoint(endpointName)
	if endpoint == nil {
		endpoint = j.registry.GetEndpoint(j.registry.GetDefault())
	}
	if endpoint == nil {
		// Fall back to stub if no endpoint available
		return j.stubEvaluate(input)
	}

	// Create client
	client, err := agenticmodel.NewClient(endpoint)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}
	defer client.Close()

	// Build evaluation prompt
	systemPrompt := j.buildSystemPrompt()
	userPrompt := j.buildUserPrompt(input)

	// Call LLM
	resp, err := client.ChatCompletion(ctx, agentic.AgentRequest{
		RequestID: fmt.Sprintf("judge-%s", input.Quest.ID),
		Role:      agentic.RoleGeneral,
		Messages: []agentic.ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
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

	// Parse response
	return j.parseEvaluationResponse(resp.Message.Content, input.Criterion.Threshold)
}

// stubEvaluate returns placeholder scores when no registry is available.
func (j *LLMJudge) stubEvaluate(input JudgeInput) (*JudgeOutput, error) {
	score := 0.75
	passed := score >= input.Criterion.Threshold

	return &JudgeOutput{
		Score:     score,
		Passed:    passed,
		Reasoning: "LLM evaluation stub - using placeholder score (no model registry configured)",
		Pending:   false,
	}, nil
}

func (j *LLMJudge) buildSystemPrompt() string {
	if j.config.SystemPrompt != "" {
		return j.config.SystemPrompt
	}
	return DefaultLLMJudgeConfig().SystemPrompt
}

func (j *LLMJudge) buildUserPrompt(input JudgeInput) string {
	var sb strings.Builder

	sb.WriteString("## Quest Information\n")
	sb.WriteString(fmt.Sprintf("**Title:** %s\n", input.Quest.Title))
	sb.WriteString(fmt.Sprintf("**Description:** %s\n", input.Quest.Description))
	sb.WriteString(fmt.Sprintf("**Difficulty:** %d\n\n", input.Quest.Difficulty))

	sb.WriteString("## Evaluation Criterion\n")
	sb.WriteString(fmt.Sprintf("**Criterion:** %s\n", input.Criterion.Name))
	sb.WriteString(fmt.Sprintf("**Description:** %s\n", input.Criterion.Description))
	sb.WriteString(fmt.Sprintf("**Weight:** %.2f\n", input.Criterion.Weight))
	sb.WriteString(fmt.Sprintf("**Pass Threshold:** %.2f\n\n", input.Criterion.Threshold))

	sb.WriteString("## Quest Output to Evaluate\n")
	sb.WriteString("```\n")
	sb.WriteString(formatOutput(input.Output))
	sb.WriteString("\n```\n\n")

	sb.WriteString("## Instructions\n")
	sb.WriteString("Evaluate the output above against the criterion.\n")
	sb.WriteString("Respond with JSON in this exact format:\n")
	sb.WriteString("```json\n")
	sb.WriteString(`{"score": 0.0-1.0, "passed": true/false, "reasoning": "your detailed reasoning"}`)
	sb.WriteString("\n```\n")

	return sb.String()
}

func (j *LLMJudge) parseEvaluationResponse(content string, threshold float64) (*JudgeOutput, error) {
	// Extract JSON from response
	jsonStr := ExtractJSONFromLLMResponse(content)

	var raw struct {
		Score     float64 `json:"score"`
		Passed    bool    `json:"passed"`
		Reasoning string  `json:"reasoning"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		LogLLMParseFailure("llm_judge_evaluation", err, content, jsonStr)
		// Try to extract score from text if JSON parsing fails
		return &JudgeOutput{
			Score:     0.5,
			Passed:    false,
			Reasoning: fmt.Sprintf("Failed to parse LLM response as JSON: %v\nRaw response: %s", err, TruncateForLog(content, 500)),
			Pending:   false,
		}, nil
	}

	// Validate and normalize score
	score := raw.Score
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	// Determine pass based on threshold (override LLM's passed field)
	passed := score >= threshold

	return &JudgeOutput{
		Score:     score,
		Passed:    passed,
		Reasoning: raw.Reasoning,
		Pending:   false,
	}, nil
}

// formatOutput converts the output to a string representation for the LLM.
func formatOutput(output any) string {
	if output == nil {
		return "(no output)"
	}

	// If it's already a string, return it
	if s, ok := output.(string); ok {
		return s
	}

	// Try to marshal as JSON
	if b, err := json.MarshalIndent(output, "", "  "); err == nil {
		return string(b)
	}

	// Fall back to fmt
	return fmt.Sprintf("%v", output)
}

// LLMJudgeConfig holds configuration for LLM evaluation.
type LLMJudgeConfig struct {
	Provider     string  `json:"provider"`      // "openai", "anthropic", etc.
	Model        string  `json:"model"`         // Model identifier
	Temperature  float64 `json:"temperature"`   // Sampling temperature
	MaxTokens    int     `json:"max_tokens"`    // Max response tokens
	SystemPrompt string  `json:"system_prompt"` // Custom system prompt
}

// DefaultLLMJudgeConfig returns sensible defaults for LLM evaluation.
func DefaultLLMJudgeConfig() LLMJudgeConfig {
	return LLMJudgeConfig{
		Provider:    "anthropic",
		Model:       "claude-sonnet-4-5-20250514",
		Temperature: 0.0, // Deterministic for evaluation
		MaxTokens:   1024,
		SystemPrompt: `You are an expert evaluator assessing quest output quality.
Evaluate the output against the given criterion and provide:
1. A score from 0.0 to 1.0
2. Whether it passes the threshold
3. Detailed reasoning for your assessment

Be objective and fair. Consider both the requirements and the quality of execution.

You MUST respond with valid JSON in this exact format:
{"score": 0.0-1.0, "passed": true/false, "reasoning": "your detailed reasoning"}`,
	}
}
