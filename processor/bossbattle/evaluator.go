package bossbattle

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/model"
	agenticmodel "github.com/c360studio/semstreams/processor/agentic-model"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/promptmanager"
	"github.com/c360studio/semdragons/processor/tokenbudget"
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
	Results          []domain.ReviewResult `json:"results"`
	ChecklistResults []ChecklistResult     `json:"checklist_results,omitempty"`
	Verdict          domain.BattleVerdict  `json:"verdict"`
	Pending          bool                  `json:"pending"`
	PendingJudge     string                `json:"pending_judge,omitempty"`
	PeerRatings      *domain.ReviewRatings `json:"peer_ratings,omitempty"`
	LoopID           string                `json:"loop_id,omitempty"`
}

// computeVerdict builds an EvaluationResult from scored results and battle metadata.
// Checks for human judge requirements (returns pending), then computes weighted score.
// Any checklist failure forces an automatic defeat regardless of criteria scores.
func computeVerdict(results []domain.ReviewResult, battle *BossBattle, feedback string, checklistResults ...ChecklistResult) *EvaluationResult {
	// Check for human judge requirement
	for _, judge := range battle.Judges {
		if judge.Type == domain.JudgeHuman {
			return &EvaluationResult{
				Results:          results,
				ChecklistResults: checklistResults,
				Pending:          true,
				PendingJudge:     judge.ID,
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

	// Checklist failures are automatic defeats.
	var failedChecks []string
	for _, cl := range checklistResults {
		if !cl.Passed {
			allPassed = false
			failedChecks = append(failedChecks, cl.Name)
		}
	}
	if len(failedChecks) > 0 && feedback != "" {
		feedback += fmt.Sprintf(" [STRUCTURAL FAILURES: %s]", strings.Join(failedChecks, ", "))
	}

	return &EvaluationResult{
		Results:          results,
		ChecklistResults: checklistResults,
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

// trajectoryBucket is the NATS KV bucket name for storing agentic trajectories.
const trajectoryBucket = "AGENT_TRAJECTORIES"

// trajectoryWriter is the minimal interface required for writing battle trajectories.
// Satisfied by *natsclient.Client — defined as an interface to avoid a direct import
// and to keep the evaluator testable in isolation.
type trajectoryWriter interface {
	GetKeyValueBucket(ctx context.Context, name string) (jetstream.KeyValue, error)
}

// DomainAwareEvaluator extends DefaultBattleEvaluator with LLM-as-judge calls.
// When an LLM judge is configured AND a model registry is available, it assembles
// a judge prompt using the domain's JudgeSystemBase and calls the LLM.
// Falls back to heuristic evaluation when no LLM is available or on failure.
type DomainAwareEvaluator struct {
	catalog     *promptmanager.DomainCatalog
	registry    model.RegistryReader
	assembler   *promptmanager.PromptAssembler
	fallback    *DefaultBattleEvaluator
	tokenLedger *tokenbudget.TokenLedger
	nats        trajectoryWriter // for best-effort trajectory persistence; may be nil
}

// NewDomainAwareEvaluator creates an evaluator with domain catalog and model registry.
// tokenLedger may be nil when token budget enforcement is not required.
// nats may be nil when trajectory persistence is not needed.
func NewDomainAwareEvaluator(
	catalog *promptmanager.DomainCatalog,
	registry model.RegistryReader,
	assembler *promptmanager.PromptAssembler,
	tokenLedger *tokenbudget.TokenLedger,
	nats trajectoryWriter,
) *DomainAwareEvaluator {
	return &DomainAwareEvaluator{
		catalog:     catalog,
		registry:    registry,
		assembler:   assembler,
		fallback:    NewDefaultBattleEvaluator(),
		tokenLedger: tokenLedger,
		nats:        nats,
	}
}

// Evaluate runs LLM judges when available, falling back to heuristic evaluation.
func (e *DomainAwareEvaluator) Evaluate(ctx context.Context, battle *BossBattle, quest *domain.Quest, output any) (*EvaluationResult, error) {
	// Check if any judge requires LLM evaluation
	if !e.hasLLMJudge(battle) || e.registry == nil || e.assembler == nil {
		return e.fallback.Evaluate(ctx, battle, quest, output)
	}

	// Check token budget before making LLM call.
	if e.tokenLedger != nil {
		if err := e.tokenLedger.Check(); err != nil {
			slog.Warn("token budget exceeded, falling back to heuristic",
				"error", err, "battle", battle.ID)
			return e.fallback.Evaluate(ctx, battle, quest, output)
		}
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

	// Resolve checklist from catalog, filtered by the quest's tier and skills.
	// Items are skipped when the quest doesn't meet the item's MinTier or
	// doesn't require the item's skills — e.g., "tests-included" won't
	// penalize apprentice-tier quests or analysis-only quests.
	var checklist []promptmanager.ChecklistItem
	if e.catalog.ReviewConfig != nil {
		checklist = promptmanager.FilterChecklist(
			e.catalog.ReviewConfig.StructuralChecklist,
			quest.MinTier,
			quest.RequiredSkills,
		)
	}

	// Assemble the judge prompt using domain catalog.
	// Acceptance criteria are the ground truth for what the quest must deliver.
	assembled := e.assembler.AssembleJudgePromptWithAcceptance(
		e.catalog.JudgeSystemBase,
		battle.Criteria,
		quest.Title,
		quest.Description,
		endpoint.Provider,
		quest.Acceptance,
		checklist...,
	)

	// Format the quest output for the user message.
	userMessage := formatOutputForJudge(output)

	// Call the LLM
	judgeResult, err := e.callLLMJudge(ctx, endpoint, assembled.SystemMessage, userMessage, battle)

	// Record token usage regardless of success/failure.
	if e.tokenLedger != nil && judgeResult != nil && (judgeResult.TokenUsage.PromptTokens > 0 || judgeResult.TokenUsage.CompletionTokens > 0) {
		e.tokenLedger.Record(ctx, judgeResult.TokenUsage.PromptTokens, judgeResult.TokenUsage.CompletionTokens, "boss_battle", endpointName)
	}

	if err != nil {
		slog.Warn("LLM judge call failed, falling back to heuristic",
			"error", err, "battle", battle.ID)
		return e.fallback.Evaluate(ctx, battle, quest, output)
	}

	// Merge: use LLM results for criteria it scored, heuristic for any it missed
	results := e.mergeResults(battle, judgeResult.Results)

	feedback := judgeResult.Feedback
	if feedback == "" {
		feedback = "LLM judge evaluation complete"
	}

	verdict := computeVerdict(results, battle, feedback, judgeResult.ChecklistResults...)
	verdict.PeerRatings = judgeResult.PeerRatings
	verdict.LoopID = judgeResult.LoopID
	return verdict, nil
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

// llmJudgeResult holds all outputs from a single LLM judge call.
type llmJudgeResult struct {
	Results          []domain.ReviewResult
	ChecklistResults []ChecklistResult
	Feedback         string
	PeerRatings      *domain.ReviewRatings
	TokenUsage       agentic.TokenUsage
	LoopID           string
}

// callLLMJudge makes a one-shot LLM call and parses the response into review results.
// The LoopID is non-empty when a trajectory was successfully queued for writing.
func (e *DomainAwareEvaluator) callLLMJudge(
	ctx context.Context,
	endpoint *model.EndpointConfig,
	systemPrompt, userMessage string,
	battle *BossBattle,
) (*llmJudgeResult, error) {
	client, err := agenticmodel.NewClient(endpoint)
	if err != nil {
		return nil, fmt.Errorf("create LLM client: %w", err)
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

	callStart := time.Now()
	resp, err := client.ChatCompletion(ctx, req)
	callDuration := time.Since(callStart).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("LLM chat completion: %w", err)
	}

	if resp.Status == agentic.StatusError {
		return &llmJudgeResult{TokenUsage: resp.TokenUsage}, fmt.Errorf("LLM returned error: %s", resp.Error)
	}

	// Build and persist a lightweight trajectory for observability.
	// The loop ID links the battle entity to the trajectory in AGENT_TRAJECTORIES.
	safeBattleID := strings.ReplaceAll(string(battle.ID), ".", "-")
	loopID := fmt.Sprintf("battle-%s-%s", safeBattleID, domain.GenerateInstance())
	now := time.Now()
	traj := &agentic.Trajectory{
		LoopID:         loopID,
		StartTime:      callStart,
		EndTime:        &now,
		Outcome:        "completed",
		TotalTokensIn:  resp.TokenUsage.PromptTokens,
		TotalTokensOut: resp.TokenUsage.CompletionTokens,
		Duration:       callDuration,
		Steps: []agentic.TrajectoryStep{{
			Timestamp: callStart,
			StepType:  "model_call",
			RequestID: fmt.Sprintf("battle-%s", battle.ID),
			Prompt:    systemPrompt,
			Response:  resp.Message.Content,
			TokensIn:  resp.TokenUsage.PromptTokens,
			TokensOut: resp.TokenUsage.CompletionTokens,
			Duration:  callDuration,
			Model:     endpoint.Model,
			Messages: []agentic.ChatMessage{
				{Role: "system", Content: systemPrompt},
				{Role: "user", Content: userMessage},
				{Role: "assistant", Content: resp.Message.Content},
			},
		}},
	}
	if e.nats != nil {
		e.writeTrajectory(ctx, loopID, traj)
	}

	results, checklistResults, feedback, peerRatings, parseErr := parseJudgeResponse(resp.Message.Content, battle)
	return &llmJudgeResult{
		Results:          results,
		ChecklistResults: checklistResults,
		Feedback:         feedback,
		PeerRatings:      peerRatings,
		TokenUsage:       resp.TokenUsage,
		LoopID:           loopID,
	}, parseErr
}

// writeTrajectory persists a battle trajectory to the AGENT_TRAJECTORIES KV bucket.
// All failures are logged as warnings — never propagated — so a trajectory write
// failure cannot affect the evaluation outcome.
func (e *DomainAwareEvaluator) writeTrajectory(ctx context.Context, loopID string, traj *agentic.Trajectory) {
	bucket, err := e.nats.GetKeyValueBucket(ctx, trajectoryBucket)
	if err != nil {
		slog.Warn("failed to get trajectory bucket", "bucket", trajectoryBucket, "error", err)
		return
	}
	data, err := json.Marshal(traj)
	if err != nil {
		slog.Warn("failed to marshal battle trajectory", "loop_id", loopID, "error", err)
		return
	}
	if _, err := bucket.Put(ctx, loopID, data); err != nil {
		slog.Warn("failed to write battle trajectory", "loop_id", loopID, "error", err)
	}
}

// judgeResponse is the expected JSON structure from the LLM judge.
type judgeResponse struct {
	Criteria []struct {
		Name      string  `json:"name"`
		Score     float64 `json:"score"`
		Reasoning string  `json:"reasoning"`
	} `json:"criteria"`
	Checklist []struct {
		Name      string `json:"name"`
		Passed    bool   `json:"passed"`
		Reasoning string `json:"reasoning"`
	} `json:"checklist,omitempty"`
	OverallFeedback string `json:"overall_feedback"`
	PeerReview      *struct {
		Q1 int `json:"q1"`
		Q2 int `json:"q2"`
		Q3 int `json:"q3"`
	} `json:"peer_review,omitempty"`
}

// ChecklistResult holds the outcome of a single structural checklist item.
type ChecklistResult struct {
	Name      string `json:"name"`
	Passed    bool   `json:"passed"`
	Reasoning string `json:"reasoning"`
}

// parseJudgeResponse extracts per-criterion scores from the LLM response.
// Returns results, checklist results, overall feedback, peer review ratings, and any error.
// Defensively handles malformed responses by returning an error (caller falls back to heuristic).
func parseJudgeResponse(content string, battle *BossBattle) ([]domain.ReviewResult, []ChecklistResult, string, *domain.ReviewRatings, error) {
	// Extract JSON from response (may be wrapped in markdown code block)
	jsonStr := extractJSON(content)
	if jsonStr == "" {
		return nil, nil, "", nil, fmt.Errorf("no JSON found in LLM response")
	}

	var resp judgeResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return nil, nil, "", nil, fmt.Errorf("parse judge JSON: %w", err)
	}

	if len(resp.Criteria) == 0 {
		return nil, nil, "", nil, fmt.Errorf("judge returned no criteria scores")
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
		return nil, nil, "", nil, fmt.Errorf("no matching criteria in judge response")
	}

	// Extract checklist results if present
	var checklistResults []ChecklistResult
	for _, cl := range resp.Checklist {
		checklistResults = append(checklistResults, ChecklistResult{
			Name:      cl.Name,
			Passed:    cl.Passed,
			Reasoning: cl.Reasoning,
		})
	}

	// Extract peer review ratings if present
	var peerRatings *domain.ReviewRatings
	if resp.PeerReview != nil {
		peerRatings = &domain.ReviewRatings{
			Q1: resp.PeerReview.Q1,
			Q2: resp.PeerReview.Q2,
			Q3: resp.PeerReview.Q3,
		}
	}

	return results, checklistResults, resp.OverallFeedback, peerRatings, nil
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

	// Check for combined output with red-team findings.
	if m, ok := output.(map[string]any); ok {
		if questOutput, hasQO := m["quest_output"]; hasQO {
			if rtFindings, hasRT := m["red_team_findings"]; hasRT {
				return formatCombinedOutput(questOutput, rtFindings)
			}
		}
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

// formatCombinedOutput renders both the quest output and red-team findings
// for the boss battle judge. The judge sees both and factors them into scoring.
func formatCombinedOutput(questOutput, rtFindings any) string {
	var b strings.Builder

	b.WriteString("## Quest Output\n\n")
	switch v := questOutput.(type) {
	case string:
		b.WriteString(v)
	default:
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			fmt.Fprintf(&b, "%v", v)
		} else {
			fmt.Fprintf(&b, "```json\n%s\n```", string(data))
		}
	}

	b.WriteString("\n\n---\n\n## Red-Team Review Findings\n\n")
	b.WriteString("A separate team reviewed the output above. Consider their findings " +
		"when scoring, but apply your own judgment — the red team may have missed issues " +
		"or flagged non-issues.\n\n")

	switch v := rtFindings.(type) {
	case string:
		b.WriteString(v)
	default:
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			fmt.Fprintf(&b, "%v", v)
		} else {
			fmt.Fprintf(&b, "```json\n%s\n```", string(data))
		}
	}

	return b.String()
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
