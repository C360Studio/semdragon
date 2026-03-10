package questboard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/c360studio/semstreams/model"
	"github.com/nats-io/nats.go/jetstream"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// AUTO-TRIAGE WATCHER — Watches for pending_triage quests and triages via LLM
// =============================================================================
//
// The triage watcher is a KV watcher goroutine that detects quests transitioning
// to pending_triage status. Depending on the DM mode, it:
//   - full_auto: Calls the triage LLM and applies the decision immediately.
//   - assisted: Calls the triage LLM and routes the proposal to the approval queue.
//   - supervised/manual: Skips LLM call; quest waits for human triage via API.
//
// Timeout sweep: quests in pending_triage longer than TriageTimeoutMins get
// auto-applied with the TPK recovery path (clear output, inject anti-patterns).
// =============================================================================

const triageLLMTimeout = 60 * time.Second

// triageLLMResponse is the structured JSON response expected from the triage LLM.
type triageLLMResponse struct {
	Path           string   `json:"path"`            // "salvage", "tpk", "escalate", "terminal"
	Analysis       string   `json:"analysis"`        // Explanation of the decision
	SalvagedOutput string   `json:"salvaged_output"` // For salvage: what to keep
	AntiPatterns   []string `json:"anti_patterns"`   // For tpk: what went wrong
}

// processTriageWatchUpdates is the KV watcher goroutine for auto-triage.
// It detects quests entering pending_triage and dispatches triage logic.
func (c *Component) processTriageWatchUpdates() {
	defer close(c.triageDoneCh)

	for {
		select {
		case <-c.triageStopCh:
			return
		case entry, ok := <-c.triageWatch.Updates():
			if !ok {
				return
			}
			if entry == nil {
				continue // Initial sync complete marker
			}
			c.handleTriageStateChange(entry)
		}
	}
}

// handleTriageStateChange processes a quest entity state change, detecting
// transitions into pending_triage and dispatching auto-triage.
func (c *Component) handleTriageStateChange(entry jetstream.KeyValueEntry) {
	if !c.running.Load() {
		return
	}

	if entry.Operation() == jetstream.KeyValueDelete {
		c.triageCache.Delete(entry.Key())
		return
	}

	entityState, err := semdragons.DecodeEntityState(entry)
	if err != nil || entityState == nil {
		return
	}

	// Extract current status from triples
	var currentStatus domain.QuestStatus
	for _, triple := range entityState.Triples {
		if triple.Predicate == "quest.status.state" {
			if v, ok := triple.Object.(string); ok {
				currentStatus = domain.QuestStatus(v)
			}
		}
	}

	// State diff against cache
	prevStatus, hadPrev := c.triageCache.Load(entry.Key())
	c.triageCache.Store(entry.Key(), currentStatus)

	if !hadPrev || prevStatus == currentStatus {
		return
	}

	// Only react to transitions INTO pending_triage
	if currentStatus != domain.QuestPendingTriage {
		return
	}

	c.lastActivity.Store(time.Now())

	quest := domain.QuestFromEntityState(entityState)
	if quest == nil {
		c.logger.Warn("failed to reconstruct quest for triage", "key", entry.Key())
		return
	}

	c.logger.Info("quest entered pending_triage",
		"quest_id", quest.ID,
		"title", quest.Title,
		"attempts", quest.Attempts,
		"dm_mode", c.config.Triage.DMMode)

	ctx, cancel := context.WithTimeout(context.Background(), triageLLMTimeout+10*time.Second)
	defer cancel()

	if err := c.autoTriage(ctx, quest); err != nil {
		c.errorsCount.Add(1)
		c.logger.Error("auto-triage failed",
			"quest_id", quest.ID,
			"error", err)
	}
}

// autoTriage dispatches triage based on the DM mode.
func (c *Component) autoTriage(ctx context.Context, quest *domain.Quest) error {
	mode := c.config.Triage.DMMode

	switch mode {
	case domain.DMFullAuto:
		return c.triageFullAuto(ctx, quest)

	case domain.DMAssisted:
		return c.triageAssisted(ctx, quest)

	case domain.DMSupervised, domain.DMManual:
		// No LLM call — quest waits for human triage via API.
		c.logger.Info("quest awaiting human triage",
			"quest_id", quest.ID,
			"mode", mode)
		return nil

	default:
		c.logger.Warn("unknown triage DM mode, defaulting to full_auto",
			"mode", mode,
			"quest_id", quest.ID)
		return c.triageFullAuto(ctx, quest)
	}
}

// triageFullAuto calls the LLM and applies the decision immediately.
func (c *Component) triageFullAuto(ctx context.Context, quest *domain.Quest) error {
	resp, err := c.callTriageLLM(ctx, quest)
	if err != nil {
		// LLM unavailable — escalate as fallback
		c.logger.Warn("triage LLM call failed, escalating",
			"quest_id", quest.ID,
			"error", err)
		return c.TriageQuest(ctx, quest.ID, TriageDecision{
			Path:     domain.RecoveryEscalate,
			Analysis: fmt.Sprintf("Auto-triage failed: %v", err),
		})
	}

	decision := llmResponseToDecision(resp)
	c.logger.Info("auto-triage decision",
		"quest_id", quest.ID,
		"path", decision.Path,
		"analysis", truncate(decision.Analysis, 100))

	return c.TriageQuest(ctx, quest.ID, decision)
}

// triageAssisted calls the LLM, then routes the proposal through the approval queue.
func (c *Component) triageAssisted(ctx context.Context, quest *domain.Quest) error {
	if c.approval == nil {
		c.logger.Warn("approval router not wired, falling back to full_auto",
			"quest_id", quest.ID)
		return c.triageFullAuto(ctx, quest)
	}

	resp, err := c.callTriageLLM(ctx, quest)
	if err != nil {
		c.logger.Warn("triage LLM call failed, escalating",
			"quest_id", quest.ID,
			"error", err)
		return c.TriageQuest(ctx, quest.ID, TriageDecision{
			Path:     domain.RecoveryEscalate,
			Analysis: fmt.Sprintf("Auto-triage failed: %v", err),
		})
	}

	proposed := llmResponseToDecision(resp)

	// Build approval request
	req := domain.ApprovalRequest{
		ID:        domain.GenerateInstance(),
		SessionID: c.config.Triage.SessionID,
		Type:      domain.ApprovalFailureTriage,
		Title:     fmt.Sprintf("Triage: %s", quest.Title),
		Details: fmt.Sprintf(
			"Quest %q exhausted %d attempts. LLM suggests: %s\n\nAnalysis: %s",
			quest.Title, quest.Attempts, proposed.Path, proposed.Analysis,
		),
		Suggestion: map[string]any{
			"path":     string(proposed.Path),
			"analysis": proposed.Analysis,
		},
		Options: []domain.ApprovalOption{
			{ID: string(proposed.Path), Label: fmt.Sprintf("Accept LLM suggestion (%s)", proposed.Path), IsDefault: true},
			{ID: string(domain.RecoverySalvage), Label: "Salvage partial output"},
			{ID: string(domain.RecoveryTPK), Label: "Total party kill (clear output)"},
			{ID: string(domain.RecoveryEscalate), Label: "Escalate to human DM"},
			{ID: string(domain.RecoveryTerminal), Label: "Mark permanently failed"},
		},
		CreatedAt: time.Now(),
	}

	approvalResp, err := c.approval.RequestApproval(ctx, req)
	if err != nil {
		c.logger.Error("approval request failed, escalating",
			"quest_id", quest.ID,
			"error", err)
		return c.TriageQuest(ctx, quest.ID, TriageDecision{
			Path:     domain.RecoveryEscalate,
			Analysis: fmt.Sprintf("Approval request failed: %v", err),
		})
	}

	// Apply the selected option (or LLM suggestion if approved as-is)
	selectedPath := approvalResp.SelectedID
	if selectedPath == "" {
		selectedPath = string(proposed.Path)
	}

	decision := proposed
	decision.Path = domain.RecoveryPath(selectedPath)

	return c.TriageQuest(ctx, quest.ID, decision)
}

// =============================================================================
// LLM TRIAGE CALL
// =============================================================================

// callTriageLLM resolves the triage endpoint and calls the LLM.
func (c *Component) callTriageLLM(ctx context.Context, quest *domain.Quest) (*triageLLMResponse, error) {
	if c.registry == nil {
		return nil, fmt.Errorf("model registry not available")
	}

	capability := c.config.Triage.Capability
	if capability == "" {
		capability = "dm-chat"
	}

	endpointName := c.registry.Resolve(capability)
	if endpointName == "" {
		return nil, fmt.Errorf("no endpoint configured for %q capability", capability)
	}
	endpoint := c.registry.GetEndpoint(endpointName)
	if endpoint == nil {
		return nil, fmt.Errorf("endpoint %q not found in registry", endpointName)
	}

	systemPrompt := buildTriageSystemPrompt()
	userMessage := buildTriageUserMessage(quest)

	content, err := callTriageLLMEndpoint(ctx, endpoint, systemPrompt, userMessage)
	if err != nil {
		return nil, err
	}

	return parseTriageResponse(content)
}

// buildTriageSystemPrompt returns the system prompt for triage decisions.
func buildTriageSystemPrompt() string {
	return `You are the Dungeon Master (DM) for an agentic workflow system modeled as a tabletop RPG.

A quest has exhausted all its retry attempts and entered triage. Your job is to evaluate the quest's failure and decide the recovery path.

RECOVERY PATHS:
- "salvage" — The agent produced partial useful work. Preserve it and grant one more attempt with enriched context. Use when there's output worth building on.
- "tpk" (Total Party Kill) — The attempt was fundamentally flawed. Clear the output, identify anti-patterns, and grant one more attempt with warnings. Use when the approach was wrong but the quest is still achievable.
- "escalate" — The quest needs human attention. Use when the failure suggests the quest definition is unclear, impossible, or requires capabilities the system lacks.
- "terminal" — The quest is truly impossible or no longer relevant. Mark as permanently failed. Use sparingly — only when retrying would waste resources.

Respond with ONLY a JSON object (no markdown fences, no explanation outside the JSON):
{
  "path": "salvage" | "tpk" | "escalate" | "terminal",
  "analysis": "Brief explanation of your assessment",
  "salvaged_output": "What to preserve (only for salvage path, empty string otherwise)",
  "anti_patterns": ["mistake1", "mistake2"]
}`
}

// buildTriageUserMessage constructs the user message with quest context.
func buildTriageUserMessage(quest *domain.Quest) string {
	var b strings.Builder

	b.WriteString("QUEST DETAILS:\n")
	b.WriteString("Title: " + quest.Title + "\n")
	if quest.Goal != "" {
		b.WriteString("Goal: " + quest.Goal + "\n")
	} else if quest.Description != "" {
		b.WriteString("Description: " + quest.Description + "\n")
	}
	b.WriteString("Difficulty: " + strconv.Itoa(int(quest.Difficulty)) + "\n")
	b.WriteString("Attempts: " + strconv.Itoa(quest.Attempts) + "/" + strconv.Itoa(quest.MaxAttempts) + "\n")

	if quest.FailureReason != "" {
		b.WriteString("\nFAILURE REASON: " + quest.FailureReason + "\n")
	}
	if quest.FailureType != "" {
		b.WriteString("FAILURE TYPE: " + string(quest.FailureType) + "\n")
	}

	if len(quest.FailureHistory) > 0 {
		b.WriteString("\nFAILURE HISTORY:\n")
		for _, fh := range quest.FailureHistory {
			b.WriteString(fmt.Sprintf("- Attempt %d: %s — %s\n", fh.Attempt, fh.FailureType, fh.FailureReason))
		}
	}

	if quest.Output != nil {
		outputStr := fmt.Sprintf("%v", quest.Output)
		if len(outputStr) > 2000 {
			outputStr = outputStr[:2000] + "... [truncated]"
		}
		b.WriteString("\nAGENT OUTPUT (for evaluation):\n" + outputStr + "\n")
	}

	if len(quest.Requirements) > 0 {
		b.WriteString("\nREQUIREMENTS:\n")
		for _, req := range quest.Requirements {
			b.WriteString("- " + req + "\n")
		}
	}

	if len(quest.Acceptance) > 0 {
		b.WriteString("\nACCEPTANCE CRITERIA:\n")
		for _, ac := range quest.Acceptance {
			b.WriteString("- " + ac + "\n")
		}
	}

	return b.String()
}

// parseTriageResponse extracts a triageLLMResponse from the LLM output.
// Handles both raw JSON and JSON wrapped in markdown code fences.
func parseTriageResponse(content string) (*triageLLMResponse, error) {
	content = strings.TrimSpace(content)

	// Strip markdown code fences if present
	if strings.HasPrefix(content, "```") {
		lines := strings.SplitN(content, "\n", 2)
		if len(lines) == 2 {
			content = lines[1]
		}
		if idx := strings.LastIndex(content, "```"); idx >= 0 {
			content = content[:idx]
		}
		content = strings.TrimSpace(content)
	}

	var resp triageLLMResponse
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return nil, fmt.Errorf("parse triage response: %w (content: %s)", err, truncate(content, 200))
	}

	// Validate the path
	switch domain.RecoveryPath(resp.Path) {
	case domain.RecoverySalvage, domain.RecoveryTPK, domain.RecoveryEscalate, domain.RecoveryTerminal:
		// valid
	default:
		return nil, fmt.Errorf("invalid recovery path in LLM response: %q", resp.Path)
	}

	return &resp, nil
}

// llmResponseToDecision converts a triageLLMResponse to a TriageDecision.
func llmResponseToDecision(resp *triageLLMResponse) TriageDecision {
	var salvaged any
	if resp.SalvagedOutput != "" {
		salvaged = resp.SalvagedOutput
	}

	return TriageDecision{
		Path:           domain.RecoveryPath(resp.Path),
		Analysis:       resp.Analysis,
		SalvagedOutput: salvaged,
		AntiPatterns:   resp.AntiPatterns,
	}
}

// =============================================================================
// LLM HTTP CALL — Provider-aware (same pattern as questbridge/dm_answerer.go)
// =============================================================================

func callTriageLLMEndpoint(ctx context.Context, endpoint *model.EndpointConfig, systemPrompt, userMessage string) (string, error) {
	apiKey := ""
	if endpoint.APIKeyEnv != "" {
		apiKey = os.Getenv(endpoint.APIKeyEnv)
		if apiKey == "" {
			return "", fmt.Errorf("API key env %q is not set", endpoint.APIKeyEnv)
		}
	}

	if endpoint.Provider == "anthropic" {
		return callTriageAnthropic(ctx, endpoint, apiKey, systemPrompt, userMessage)
	}
	return callTriageOpenAICompat(ctx, endpoint, apiKey, systemPrompt, userMessage)
}

func callTriageOpenAICompat(ctx context.Context, endpoint *model.EndpointConfig, apiKey, systemPrompt, userMessage string) (string, error) {
	url := endpoint.URL
	if url == "" {
		return "", fmt.Errorf("URL required for provider %q", endpoint.Provider)
	}
	if url[len(url)-1] == '/' {
		url = url[:len(url)-1]
	}
	url += "/chat/completions"

	body := map[string]any{
		"model": endpoint.Model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userMessage},
		},
	}
	if endpoint.ReasoningEffort != "" {
		body["reasoning_effort"] = endpoint.ReasoningEffort
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: triageLLMTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("empty response")
	}
	return result.Choices[0].Message.Content, nil
}

func callTriageAnthropic(ctx context.Context, endpoint *model.EndpointConfig, apiKey, systemPrompt, userMessage string) (string, error) {
	url := "https://api.anthropic.com/v1/messages"
	if endpoint.URL != "" {
		u := endpoint.URL
		if u[len(u)-1] == '/' {
			u = u[:len(u)-1]
		}
		url = u + "/messages"
	}

	body := map[string]any{
		"model":      endpoint.Model,
		"max_tokens": 4096,
		"system":     systemPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": userMessage},
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: triageLLMTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("empty response")
	}
	return result.Content[0].Text, nil
}

// =============================================================================
// HELPERS
// =============================================================================

// truncate shortens a string to maxLen, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
