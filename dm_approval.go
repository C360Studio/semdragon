package semdragons

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go"
)

// =============================================================================
// NATS APPROVAL ROUTER - Human-in-the-loop approval via NATS
// =============================================================================
// NATSApprovalRouter implements ApprovalRouter using NATS for:
// - Request/reply pattern for blocking approvals
// - KV storage for pending/resolved approval state
// - Pub/sub for approval watching
//
// KV Keys:
//   approval.pending.{session}.{id}  - Pending approval requests
//   approval.resolved.{session}.{id} - Resolved approval requests
//
// NATS Subjects:
//   approval.request.{session}       - Approval requests published here
//   approval.response.{session}.{id} - Responses published here
// =============================================================================

// NATSApprovalRouter implements ApprovalRouter using NATS.
type NATSApprovalRouter struct {
	client  *natsclient.Client
	storage *Storage
	config  *BoardConfig
}

// NewNATSApprovalRouter creates a new NATS-based approval router.
func NewNATSApprovalRouter(client *natsclient.Client, storage *Storage) *NATSApprovalRouter {
	return &NATSApprovalRouter{
		client:  client,
		storage: storage,
		config:  storage.Config(),
	}
}

// --- Key Generation ---

// approvalPendingKey returns the KV key for a pending approval.
func (r *NATSApprovalRouter) approvalPendingKey(sessionInstance, approvalID string) string {
	return fmt.Sprintf("approval.pending.%s.%s", sessionInstance, approvalID)
}

// approvalResolvedKey returns the KV key for a resolved approval.
func (r *NATSApprovalRouter) approvalResolvedKey(sessionInstance, approvalID string) string {
	return fmt.Sprintf("approval.resolved.%s.%s", sessionInstance, approvalID)
}

// approvalRequestSubject returns the NATS subject for publishing requests.
func (r *NATSApprovalRouter) approvalRequestSubject(sessionInstance string) string {
	return fmt.Sprintf("approval.request.%s", sessionInstance)
}

// approvalResponseSubject returns the NATS subject for receiving responses.
func (r *NATSApprovalRouter) approvalResponseSubject(sessionInstance, approvalID string) string {
	return fmt.Sprintf("approval.response.%s.%s", sessionInstance, approvalID)
}

// --- ApprovalRouter Implementation ---

// RequestApproval sends an approval request and waits for response.
func (r *NATSApprovalRouter) RequestApproval(ctx context.Context, req ApprovalRequest) (*ApprovalResponse, error) {
	// Generate ID if not set
	if req.ID == "" {
		req.ID = GenerateInstance()
	}
	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now()
	}

	sessionInstance := ExtractInstance(req.SessionID)

	// Store pending request in KV
	if err := r.storePendingApproval(ctx, sessionInstance, &req); err != nil {
		return nil, fmt.Errorf("store pending approval: %w", err)
	}

	// Create inbox for response
	replySubject := r.approvalResponseSubject(sessionInstance, req.ID)

	// Subscribe to response subject before publishing request
	respCh := make(chan *ApprovalResponse, 1)
	errCh := make(chan error, 1)

	sub, err := r.client.Subscribe(ctx, replySubject, func(_ context.Context, msg *nats.Msg) {
		var resp ApprovalResponse
		if err := json.Unmarshal(msg.Data, &resp); err != nil {
			errCh <- fmt.Errorf("unmarshal response: %w", err)
			return
		}
		respCh <- &resp
	})
	if err != nil {
		return nil, fmt.Errorf("subscribe to response: %w", err)
	}
	defer sub.Unsubscribe()

	// Create request message with metadata including reply subject
	// We include reply subject in the payload since we're using pub/sub pattern
	requestData := struct {
		Request     ApprovalRequest `json:"request"`
		ReplyTo     string          `json:"reply_to"`
	}{
		Request: req,
		ReplyTo: replySubject,
	}

	requestSubject := r.approvalRequestSubject(sessionInstance)
	reqData, err := json.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Publish request
	if err := r.client.Publish(ctx, requestSubject, reqData); err != nil {
		return nil, fmt.Errorf("publish request: %w", err)
	}

	// Wait for response or context cancellation
	select {
	case resp := <-respCh:
		// Move from pending to resolved in KV
		r.resolveApproval(ctx, sessionInstance, req.ID, resp)
		return resp, nil

	case err := <-errCh:
		return nil, err

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// WatchApprovals subscribes to approval responses for a session.
func (r *NATSApprovalRouter) WatchApprovals(ctx context.Context, filter ApprovalFilter) (<-chan ApprovalResponse, error) {
	respCh := make(chan ApprovalResponse, 100)

	// Determine subject pattern
	var subject string
	if filter.SessionID != "" {
		sessionInstance := ExtractInstance(filter.SessionID)
		subject = fmt.Sprintf("approval.response.%s.>", sessionInstance)
	} else {
		subject = "approval.response.>"
	}

	sub, err := r.client.Subscribe(ctx, subject, func(_ context.Context, msg *nats.Msg) {
		var resp ApprovalResponse
		if err := json.Unmarshal(msg.Data, &resp); err != nil {
			return // Skip malformed responses
		}

		// Note: type filtering would require loading the original request.
		// For simplicity, we send all responses and let the consumer filter.
		// If needed, use filter.Types to implement filtering here.
		_ = filter.Types

		select {
		case respCh <- resp:
		default:
			slog.Warn("dropping approval response - channel full",
				"request_id", resp.RequestID,
				"session_id", resp.SessionID)
		}
	})
	if err != nil {
		close(respCh)
		return nil, fmt.Errorf("subscribe to responses: %w", err)
	}

	// Clean up on context cancellation
	go func() {
		<-ctx.Done()
		sub.Unsubscribe()
		close(respCh)
	}()

	return respCh, nil
}

// GetPendingApprovals returns all pending approval requests for a session.
func (r *NATSApprovalRouter) GetPendingApprovals(ctx context.Context, sessionID string) ([]ApprovalRequest, error) {
	sessionInstance := ExtractInstance(sessionID)
	prefix := fmt.Sprintf("approval.pending.%s.", sessionInstance)

	keys, err := r.storage.ListIndexKeys(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("list pending approvals: %w", err)
	}

	var pending []ApprovalRequest
	for _, key := range keys {
		entry, err := r.storage.KV().Get(ctx, key)
		if err != nil {
			continue
		}

		var req ApprovalRequest
		if err := json.Unmarshal(entry.Value, &req); err != nil {
			continue
		}
		pending = append(pending, req)
	}

	return pending, nil
}

// --- Response Handling ---

// RespondToApproval allows external systems to respond to pending approvals.
// Uses atomic delete to prevent duplicate responses from concurrent callers.
func (r *NATSApprovalRouter) RespondToApproval(ctx context.Context, sessionID, approvalID string, resp ApprovalResponse) error {
	sessionInstance := ExtractInstance(sessionID)
	pendingKey := r.approvalPendingKey(sessionInstance, approvalID)

	// Atomically delete the pending key - this prevents duplicate responses
	// If another responder already deleted it, this will fail
	if err := r.storage.KV().Delete(ctx, pendingKey); err != nil {
		return fmt.Errorf("approval not found or already responded: %s", approvalID)
	}

	// Set response metadata
	resp.RequestID = approvalID
	resp.SessionID = sessionID
	if resp.RespondedAt.IsZero() {
		resp.RespondedAt = time.Now()
	}

	// Store resolved state
	if err := r.storeResolvedApproval(ctx, sessionInstance, approvalID, &resp); err != nil {
		// Log but continue - the response should still be published
		slog.Warn("failed to store resolved approval", "approval_id", approvalID, "error", err)
	}

	// Publish response to NATS subject
	subject := r.approvalResponseSubject(sessionInstance, approvalID)
	respData, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}

	if err := r.client.Publish(ctx, subject, respData); err != nil {
		return fmt.Errorf("publish response: %w", err)
	}

	return nil
}

// --- Storage Helpers ---

func (r *NATSApprovalRouter) storePendingApproval(ctx context.Context, sessionInstance string, req *ApprovalRequest) error {
	key := r.approvalPendingKey(sessionInstance, req.ID)
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	_, err = r.storage.KV().Put(ctx, key, data)
	return err
}

// resolveApproval moves an approval from pending to resolved state.
// Logs errors but does not fail - approval resolution is best-effort storage.
func (r *NATSApprovalRouter) resolveApproval(ctx context.Context, sessionInstance, approvalID string, resp *ApprovalResponse) {
	// Delete pending key
	pendingKey := r.approvalPendingKey(sessionInstance, approvalID)
	if err := r.storage.KV().Delete(ctx, pendingKey); err != nil {
		slog.Warn("failed to delete pending approval key", "key", pendingKey, "error", err)
	}

	// Store resolved state
	if err := r.storeResolvedApproval(ctx, sessionInstance, approvalID, resp); err != nil {
		slog.Warn("failed to store resolved approval", "approval_id", approvalID, "error", err)
	}
}

// storeResolvedApproval stores the resolved approval in KV.
func (r *NATSApprovalRouter) storeResolvedApproval(ctx context.Context, sessionInstance, approvalID string, resp *ApprovalResponse) error {
	resolvedKey := r.approvalResolvedKey(sessionInstance, approvalID)
	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal resolved approval: %w", err)
	}
	if _, err := r.storage.KV().Put(ctx, resolvedKey, data); err != nil {
		return fmt.Errorf("put resolved approval: %w", err)
	}
	return nil
}

// =============================================================================
// APPROVAL REQUEST BUILDERS
// =============================================================================

// NewQuestCreateApproval creates an approval request for quest creation.
func NewQuestCreateApproval(sessionID, objective string, hints QuestHints, suggestion *QuestDecision) ApprovalRequest {
	return ApprovalRequest{
		ID:         GenerateInstance(),
		SessionID:  sessionID,
		Type:       ApprovalQuestCreate,
		Title:      "Create Quest",
		Details:    fmt.Sprintf("Objective: %s", objective),
		Suggestion: suggestion,
		Payload:    hints,
		CreatedAt:  time.Now(),
	}
}

// NewPartyFormationApproval creates an approval request for party formation.
func NewPartyFormationApproval(sessionID string, quest Quest, suggestions []SuggestedClaim) ApprovalRequest {
	options := make([]ApprovalOption, 0, len(suggestions))
	for i, s := range suggestions {
		options = append(options, ApprovalOption{
			ID:          string(s.AgentID),
			Label:       fmt.Sprintf("Agent %s", ExtractInstance(string(s.AgentID))),
			Description: fmt.Sprintf("Score: %.2f, Confidence: %.2f", s.Score, s.Confidence),
			IsDefault:   i == 0,
		})
	}

	return ApprovalRequest{
		ID:         GenerateInstance(),
		SessionID:  sessionID,
		Type:       ApprovalPartyFormation,
		Title:      "Form Party",
		Details:    fmt.Sprintf("Quest: %s", quest.Title),
		Suggestion: suggestions,
		Options:    options,
		CreatedAt:  time.Now(),
	}
}

// NewBattleVerdictApproval creates an approval request for battle verdict.
func NewBattleVerdictApproval(sessionID string, battle BossBattle, suggestedVerdict BattleVerdict) ApprovalRequest {
	options := []ApprovalOption{
		{
			ID:          "approve",
			Label:       "Approve Verdict",
			Description: fmt.Sprintf("Quality: %.2f, Passed: %v", suggestedVerdict.QualityScore, suggestedVerdict.Passed),
			IsDefault:   true,
		},
		{
			ID:          "override_pass",
			Label:       "Override: Pass",
			Description: "Force the battle to pass despite evaluation",
		},
		{
			ID:          "override_fail",
			Label:       "Override: Fail",
			Description: "Force the battle to fail despite evaluation",
		},
	}

	return ApprovalRequest{
		ID:         GenerateInstance(),
		SessionID:  sessionID,
		Type:       ApprovalBattleVerdict,
		Title:      "Battle Verdict",
		Details:    fmt.Sprintf("Battle: %s, Quest: %s", ExtractInstance(string(battle.ID)), ExtractInstance(string(battle.QuestID))),
		Suggestion: suggestedVerdict,
		Options:    options,
		Payload:    battle,
		CreatedAt:  time.Now(),
	}
}

// NewAgentRecruitApproval creates an approval request for agent recruitment.
func NewAgentRecruitApproval(sessionID string, config AgentConfig) ApprovalRequest {
	return ApprovalRequest{
		ID:        GenerateInstance(),
		SessionID: sessionID,
		Type:      ApprovalAgentRecruit,
		Title:     "Recruit Agent",
		Details:   fmt.Sprintf("Model: %s, Provider: %s", config.Model, config.Provider),
		Payload:   config,
		Options: []ApprovalOption{
			{ID: "approve", Label: "Approve", IsDefault: true},
			{ID: "deny", Label: "Deny"},
		},
		CreatedAt: time.Now(),
	}
}

// NewInterventionApproval creates an approval request for DM intervention.
func NewInterventionApproval(sessionID string, quest Quest, suggestion *Intervention) ApprovalRequest {
	options := []ApprovalOption{
		{ID: string(InterventionAssist), Label: "Assist", Description: "Give the agent a hint"},
		{ID: string(InterventionRedirect), Label: "Redirect", Description: "Change the approach"},
		{ID: string(InterventionTakeover), Label: "Takeover", Description: "DM completes the quest"},
		{ID: string(InterventionAbort), Label: "Abort", Description: "Cancel the quest"},
	}

	// Set default based on suggestion
	if suggestion != nil {
		for i := range options {
			if options[i].ID == string(suggestion.Type) {
				options[i].IsDefault = true
				break
			}
		}
	}

	return ApprovalRequest{
		ID:         GenerateInstance(),
		SessionID:  sessionID,
		Type:       ApprovalIntervention,
		Title:      "Intervention Required",
		Details:    fmt.Sprintf("Quest: %s, Status: %s", quest.Title, quest.Status),
		Suggestion: suggestion,
		Options:    options,
		Payload:    quest,
		CreatedAt:  time.Now(),
	}
}

// NewEscalationApproval creates an approval request for handling escalation.
func NewEscalationApproval(sessionID string, quest Quest, attempts []EscalationAttempt) ApprovalRequest {
	options := []ApprovalOption{
		{ID: "reassign", Label: "Reassign", Description: "Assign to a different agent or party"},
		{ID: "decompose", Label: "Decompose", Description: "Break into smaller sub-quests"},
		{ID: "dm_complete", Label: "DM Complete", Description: "DM handles it directly"},
		{ID: "cancel", Label: "Cancel", Description: "Cancel the quest"},
	}

	return ApprovalRequest{
		ID:        GenerateInstance(),
		SessionID: sessionID,
		Type:      ApprovalEscalation,
		Title:     "Escalation Decision",
		Details:   fmt.Sprintf("Quest: %s, Attempts: %d", quest.Title, len(attempts)),
		Options:   options,
		Payload: map[string]any{
			"quest":    quest,
			"attempts": attempts,
		},
		CreatedAt: time.Now(),
	}
}

// =============================================================================
// APPROVAL HELPERS
// =============================================================================

// WaitForPendingApprovals waits until all pending approvals are resolved.
// Useful for tests and synchronization.
func (r *NATSApprovalRouter) WaitForPendingApprovals(ctx context.Context, sessionID string) error {
	sessionInstance := ExtractInstance(sessionID)
	prefix := fmt.Sprintf("approval.pending.%s.", sessionInstance)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		keys, err := r.storage.ListIndexKeys(ctx, prefix)
		if err != nil {
			return err
		}

		if len(keys) == 0 {
			return nil
		}

		// Poll interval
		time.Sleep(100 * time.Millisecond)
	}
}

// GetResolvedApproval retrieves a resolved approval by ID.
func (r *NATSApprovalRouter) GetResolvedApproval(ctx context.Context, sessionID, approvalID string) (*ApprovalResponse, error) {
	sessionInstance := ExtractInstance(sessionID)
	key := r.approvalResolvedKey(sessionInstance, approvalID)

	entry, err := r.storage.KV().Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("approval not found: %s", approvalID)
	}

	var resp ApprovalResponse
	if err := json.Unmarshal(entry.Value, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &resp, nil
}

// ListResolvedApprovals returns all resolved approvals for a session.
func (r *NATSApprovalRouter) ListResolvedApprovals(ctx context.Context, sessionID string) ([]ApprovalResponse, error) {
	sessionInstance := ExtractInstance(sessionID)
	prefix := fmt.Sprintf("approval.resolved.%s.", sessionInstance)

	keys, err := r.storage.KV().Keys(ctx)
	if err != nil {
		return nil, err
	}

	var resolved []ApprovalResponse
	for _, key := range keys {
		if strings.HasPrefix(key, prefix) {
			entry, err := r.storage.KV().Get(ctx, key)
			if err != nil {
				continue
			}

			var resp ApprovalResponse
			if err := json.Unmarshal(entry.Value, &resp); err != nil {
				continue
			}
			resolved = append(resolved, resp)
		}
	}

	return resolved, nil
}
