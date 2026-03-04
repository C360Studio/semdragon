package bossbattle

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/model"
	agenticmodel "github.com/c360studio/semstreams/processor/agentic-model"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/promptmanager"
)

// =============================================================================
// BATTLE EVALUATOR - Runs judges and produces verdicts
// =============================================================================

// BattleEvaluator runs evaluation judges on quest outputs.
type BattleEvaluator interface {
	// Evaluate runs all judges and produces a verdict.
	Evaluate(ctx context.Context, battle *BossBattle, quest *domain.Quest, output any) (*EvaluationResult, error)
}

// EvaluationResult holds the outcome of an evaluation.
type EvaluationResult struct {
	Results      []domain.ReviewResult `json:"results"`
	Verdict      domain.BattleVerdict  `json:"verdict"`
	Pending      bool                  `json:"pending"`
	PendingJudge string                `json:"pending_judge,omitempty"`
}

// computeVerdict builds an EvaluationResult from scored results and battle metadata.
// Checks for human judge requirements (returns pending), then computes weighted score.
func computeVerdict(results []domain.ReviewResult, battle *BossBattle, feedback string) *EvaluationResult {
	// Check for human judge requirement
	for _, judge := range battle.Judges {
		if judge.Type == domain.JudgeHuman {
			return &EvaluationResult{
				Results:      results,
				Pending:      true,
				PendingJudge: judge.ID,
			}
		}
	}

	totalScore := 0.0
	allPassed := true
	for _, r := range results {
		for _, c := range battle.Criteria {
			if c.Name == r.CriterionName {
				totalScore += r.Score * c.Weight
				break
			}
		}
		if !r.Passed {
			allPassed = false
		}
	}

	return &EvaluationResult{
		Results: results,
		Verdict: domain.BattleVerdict{
			Passed:       allPassed,
			QualityScore: totalScore,
			Feedback:     feedback,
		},
	}
}

// =============================================================================
// DEFAULT (HEURISTIC) EVALUATOR
// =============================================================================

// DefaultBattleEvaluator provides a simple heuristic evaluator.
type DefaultBattleEvaluator struct{}

// NewDefaultBattleEvaluator creates a new default evaluator.
func NewDefaultBattleEvaluator() *DefaultBattleEvaluator {
	return &DefaultBattleEvaluator{}
}

// Evaluate runs heuristic scoring based on output presence.
func (e *DefaultBattleEvaluator) Evaluate(ctx context.Context, battle *BossBattle, _ *domain.Quest, output any) (*EvaluationResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	results := make([]domain.ReviewResult, len(battle.Criteria))
	for i, criterion := range battle.Criteria {
		score := 0.0
		if output != nil {
			score = 0.8
		}

		results[i] = domain.ReviewResult{
			CriterionName: criterion.Name,
			Score:         score,
			Passed:        score >= criterion.Threshold,
			Reasoning:     "Automated heuristic evaluation",
			JudgeID:       "judge-auto",
		}
	}

	return computeVerdict(results, battle, "Automated evaluation complete"), nil
}

var _ BattleEvaluator = (*DefaultBattleEvaluator)(nil)

// =============================================================================
// DOMAIN-AWARE EVALUATOR (LLM JUDGE)
// =============================================================================

// judgeCapability is the model registry capability key for boss battle evaluation.
const judgeCapability = "boss-battle"

// DomainAwareEvaluator extends DefaultBattleEvaluator with LLM-as-judge calls.
// When an LLM judge is configured AND a model registry is available, it assembles
// a judge prompt using the domain's JudgeSystemBase and calls the LLM.
// Falls back to heuristic evaluation when no LLM is available or on failure.
type DomainAwareEvaluator struct {
	catalog   *promptmanager.DomainCatalog
	registry  model.RegistryReader
	assembler *promptmanager.PromptAssembler
	fallback  *DefaultBattleEvaluator
}

// NewDomainAwareEvaluator creates an evaluator with domain catalog and model registry.
func NewDomainAwareEvaluator(
	catalog *promptmanager.DomainCatalog,
	registry model.RegistryReader,
	assembler *promptmanager.PromptAssembler,
) *DomainAwareEvaluator {
	return &DomainAwareEvaluator{
		catalog:   catalog,
		registry:  registry,
		assembler: assembler,
		fallback:  NewDefaultBattleEvaluator(),
	}
}

// Evaluate runs LLM judges when available, falling back to heuristic evaluation.
func (e *DomainAwareEvaluator) Evaluate(ctx context.Context, battle *BossBattle, quest *domain.Quest, output any) (*EvaluationResult, error) {
	// Check if any judge requires LLM evaluation
	if !e.hasLLMJudge(battle) || e.registry == nil || e.assembler == nil {
		return e.fallback.Evaluate(ctx, battle, quest, output)
	}

	// Resolve the boss-battle endpoint
	endpointName := e.registry.Resolve(judgeCapability)
	if endpointName == "" {
		slog.Warn("no endpoint for boss-battle capability, falling back to heuristic")
		return e.fallback.Evaluate(ctx, battle, quest, output)
	}
	endpoint := e.registry.GetEndpoint(endpointName)
	if endpoint == nil {
		slog.Warn("endpoint not found", "name", endpointName)
		return e.fallback.Evaluate(ctx, battle, quest, output)
	}

	// Assemble the judge prompt using domain catalog
	assembled := e.assembler.AssembleJudgePrompt(
		e.catalog.JudgeSystemBase,
		battle.Criteria,
		quest.Title,
		quest.Description,
		endpoint.Provider,
	)

	// Format the quest output for the user message
	userMessage := formatOutputForJudge(output)

	// Call the LLM
	llmResults, feedback, err := e.callLLMJudge(ctx, endpoint, assembled.SystemMessage, userMessage, battle)
	if err != nil {
		slog.Warn("LLM judge call failed, falling back to heuristic",
			"error", err, "battle", battle.ID)
		return e.fallback.Evaluate(ctx, battle, quest, output)
	}

	// Merge: use LLM results for criteria it scored, heuristic for any it missed
	results := e.mergeResults(battle, llmResults)

	if feedback == "" {
		feedback = "LLM judge evaluation complete"
	}

	return computeVerdict(results, battle, feedback), nil
}

// hasLLMJudge checks if any judge in the battle requires LLM evaluation.
func (e *DomainAwareEvaluator) hasLLMJudge(battle *BossBattle) bool {
	for _, j := range battle.Judges {
		if j.Type == domain.JudgeLLM {
			return true
		}
	}
	return false
}

// callLLMJudge makes a one-shot LLM call and parses the response into review results.
// Returns results, overall feedback from the LLM, and any error.
func (e *DomainAwareEvaluator) callLLMJudge(
	ctx context.Context,
	endpoint *model.EndpointConfig,
	systemPrompt, userMessage string,
	battle *BossBattle,
) ([]domain.ReviewResult, string, error) {
	client, err := agenticmodel.NewClient(endpoint)
	if err != nil {
		return nil, "", fmt.Errorf("create LLM client: %w", err)
	}

	req := agentic.AgentRequest{
		RequestID: fmt.Sprintf("battle-%s", battle.ID),
		Role:      agentic.RoleReviewer,
		Messages: []agentic.ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMessage},
		},
		Model: endpoint.Model,
	}

	resp, err := client.ChatCompletion(ctx, req)
	if err != nil {
		return nil, "", fmt.Errorf("LLM chat completion: %w", err)
	}

	if resp.Status == agentic.StatusError {
		return nil, "", fmt.Errorf("LLM returned error: %s", resp.Error)
	}

	results, feedback, parseErr := parseJudgeResponse(resp.Message.Content, battle)
	return results, feedback, parseErr
}

// judgeResponse is the expected JSON structure from the LLM judge.
type judgeResponse struct {
	Criteria []struct {
		Name      string  `json:"name"`
		Score     float64 `json:"score"`
		Reasoning string  `json:"reasoning"`
	} `json:"criteria"`
	OverallFeedback string `json:"overall_feedback"`
}

// parseJudgeResponse extracts per-criterion scores from the LLM response.
// Returns results, overall feedback, and any error.
// Defensively handles malformed responses by returning an error (caller falls back to heuristic).
func parseJudgeResponse(content string, battle *BossBattle) ([]domain.ReviewResult, string, error) {
	// Extract JSON from response (may be wrapped in markdown code block)
	jsonStr := extractJSON(content)
	if jsonStr == "" {
		return nil, "", fmt.Errorf("no JSON found in LLM response")
	}

	var resp judgeResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return nil, "", fmt.Errorf("parse judge JSON: %w", err)
	}

	if len(resp.Criteria) == 0 {
		return nil, "", fmt.Errorf("judge returned no criteria scores")
	}

	// Map LLM scores to review results, matching by criterion name
	var results []domain.ReviewResult
	for _, c := range resp.Criteria {
		// Find matching battle criterion for threshold
		var threshold float64
		found := false
		for _, bc := range battle.Criteria {
			if bc.Name == c.Name {
				threshold = bc.Threshold
				found = true
				break
			}
		}
		if !found {
			continue // LLM scored a criterion we didn't ask for — skip
		}

		score := clampScore(c.Score)
		results = append(results, domain.ReviewResult{
			CriterionName: c.Name,
			Score:         score,
			Passed:        score >= threshold,
			Reasoning:     c.Reasoning,
			JudgeID:       "judge-llm",
		})
	}

	if len(results) == 0 {
		return nil, "", fmt.Errorf("no matching criteria in judge response")
	}

	return results, resp.OverallFeedback, nil
}

// mergeResults combines LLM results with heuristic fallback for any unscored criteria.
func (e *DomainAwareEvaluator) mergeResults(battle *BossBattle, llmResults []domain.ReviewResult) []domain.ReviewResult {
	// Build lookup of LLM-scored criteria
	scored := make(map[string]domain.ReviewResult, len(llmResults))
	for _, r := range llmResults {
		scored[r.CriterionName] = r
	}

	// For each battle criterion, use LLM score if available, else heuristic
	results := make([]domain.ReviewResult, 0, len(battle.Criteria))
	for _, c := range battle.Criteria {
		if r, ok := scored[c.Name]; ok {
			results = append(results, r)
		} else {
			// Heuristic fallback for unscored criteria
			results = append(results, domain.ReviewResult{
				CriterionName: c.Name,
				Score:         0.8,
				Passed:        0.8 >= c.Threshold,
				Reasoning:     "Automated heuristic evaluation (LLM did not score this criterion)",
				JudgeID:       "judge-auto",
			})
		}
	}
	return results
}

// extractJSON extracts a JSON object from text that may include markdown fencing.
func extractJSON(s string) string {
	// Try to find JSON in a code block first
	if idx := strings.Index(s, "```json"); idx != -1 {
		start := idx + len("```json")
		if end := strings.Index(s[start:], "```"); end != -1 {
			return strings.TrimSpace(s[start : start+end])
		}
	}
	if idx := strings.Index(s, "```"); idx != -1 {
		start := idx + len("```")
		if end := strings.Index(s[start:], "```"); end != -1 {
			candidate := strings.TrimSpace(s[start : start+end])
			if strings.HasPrefix(candidate, "{") {
				return candidate
			}
		}
	}

	// Try to find bare JSON object
	if start := strings.Index(s, "{"); start != -1 {
		if end := strings.LastIndex(s, "}"); end > start {
			return s[start : end+1]
		}
	}
	return ""
}

// formatOutputForJudge formats quest output for the LLM judge's user message.
func formatOutputForJudge(output any) string {
	if output == nil {
		return "No output was provided."
	}

	switch v := output.(type) {
	case string:
		return fmt.Sprintf("## Quest Output\n\n%s", v)
	default:
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fmt.Sprintf("## Quest Output\n\n%v", v)
		}
		return fmt.Sprintf("## Quest Output\n\n```json\n%s\n```", string(data))
	}
}

// clampScore ensures a score is within [0.0, 1.0].
func clampScore(s float64) float64 {
	if s < 0 {
		return 0
	}
	if s > 1 {
		return 1
	}
	return s
}

var _ BattleEvaluator = (*DomainAwareEvaluator)(nil)
