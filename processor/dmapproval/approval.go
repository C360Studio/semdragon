package dmapproval

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
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
// =============================================================================

// ApprovalRouter handles human-in-the-loop approval workflows.
type ApprovalRouter interface {
	// RequestApproval sends an approval request and waits for response.
	RequestApproval(ctx context.Context, req domain.ApprovalRequest) (*domain.ApprovalResponse, error)

	// WatchApprovals subscribes to approval responses for a session.
	WatchApprovals(ctx context.Context, filter domain.ApprovalFilter) (<-chan domain.ApprovalResponse, error)

	// GetPendingApprovals returns all pending approval requests for a session.
	GetPendingApprovals(ctx context.Context, sessionID string) ([]domain.ApprovalRequest, error)
}

// NATSApprovalRouter implements ApprovalRouter using NATS.
type NATSApprovalRouter struct {
	client *natsclient.Client
	config *semdragons.BoardConfig
	logger *slog.Logger
}

// NewNATSApprovalRouter creates a new NATS-based approval router.
func NewNATSApprovalRouter(client *natsclient.Client, config *semdragons.BoardConfig, logger *slog.Logger) *NATSApprovalRouter {
	if logger == nil {
		logger = slog.Default()
	}
	return &NATSApprovalRouter{
		client: client,
		config: config,
		logger: logger,
	}
}

// --- Key Generation ---

func (r *NATSApprovalRouter) approvalPendingKey(sessionInstance, approvalID string) string {
	return fmt.Sprintf("approval.pending.%s.%s", sessionInstance, approvalID)
}

func (r *NATSApprovalRouter) approvalResolvedKey(sessionInstance, approvalID string) string {
	return fmt.Sprintf("approval.resolved.%s.%s", sessionInstance, approvalID)
}

func (r *NATSApprovalRouter) approvalRequestSubject(sessionInstance string) string {
	return fmt.Sprintf("approval.request.%s", sessionInstance)
}

func (r *NATSApprovalRouter) approvalResponseSubject(sessionInstance, approvalID string) string {
	return fmt.Sprintf("approval.response.%s.%s", sessionInstance, approvalID)
}

// --- ApprovalRouter Implementation ---

// RequestApproval sends an approval request and waits for response.
func (r *NATSApprovalRouter) RequestApproval(ctx context.Context, req domain.ApprovalRequest) (*domain.ApprovalResponse, error) {
	if req.ID == "" {
		req.ID = semdragons.GenerateInstance()
	}
	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now()
	}

	sessionInstance := semdragons.ExtractInstance(req.SessionID)

	// Store pending request in KV
	if err := r.storePendingApproval(ctx, sessionInstance, &req); err != nil {
		return nil, fmt.Errorf("store pending approval: %w", err)
	}

	// Create inbox for response
	replySubject := r.approvalResponseSubject(sessionInstance, req.ID)

	// Subscribe to response subject before publishing request
	respCh := make(chan *domain.ApprovalResponse, 1)
	errCh := make(chan error, 1)

	sub, err := r.client.Subscribe(ctx, replySubject, func(msgCtx context.Context, msg *nats.Msg) {
		select {
		case <-msgCtx.Done():
			return
		default:
		}
		var resp domain.ApprovalResponse
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
	requestData := struct {
		Request domain.ApprovalRequest `json:"request"`
		ReplyTo string                 `json:"reply_to"`
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
		r.resolveApproval(ctx, sessionInstance, req.ID, resp)
		return resp, nil

	case err := <-errCh:
		return nil, err

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// WatchApprovals subscribes to approval responses for a session.
func (r *NATSApprovalRouter) WatchApprovals(ctx context.Context, filter domain.ApprovalFilter) (<-chan domain.ApprovalResponse, error) {
	respCh := make(chan domain.ApprovalResponse, 100)

	var subject string
	if filter.SessionID != "" {
		sessionInstance := semdragons.ExtractInstance(filter.SessionID)
		subject = fmt.Sprintf("approval.response.%s.>", sessionInstance)
	} else {
		subject = "approval.response.>"
	}

	sub, err := r.client.Subscribe(ctx, subject, func(msgCtx context.Context, msg *nats.Msg) {
		select {
		case <-msgCtx.Done():
			return
		default:
		}
		var resp domain.ApprovalResponse
		if err := json.Unmarshal(msg.Data, &resp); err != nil {
			return
		}

		select {
		case respCh <- resp:
		default:
			r.logger.Warn("dropping approval response - channel full",
				"request_id", resp.RequestID,
				"session_id", resp.SessionID)
		}
	})
	if err != nil {
		close(respCh)
		return nil, fmt.Errorf("subscribe to responses: %w", err)
	}

	go func() {
		<-ctx.Done()
		sub.Unsubscribe()
		close(respCh)
	}()

	return respCh, nil
}

// GetPendingApprovals returns all pending approval requests for a session.
func (r *NATSApprovalRouter) GetPendingApprovals(ctx context.Context, sessionID string) ([]domain.ApprovalRequest, error) {
	sessionInstance := semdragons.ExtractInstance(sessionID)
	prefix := fmt.Sprintf("approval.pending.%s.", sessionInstance)

	bucket, err := r.client.GetKeyValueBucket(ctx, r.config.BucketName())
	if err != nil {
		return nil, fmt.Errorf("get KV bucket: %w", err)
	}

	keys, err := bucket.Keys(ctx)
	if err != nil {
		return nil, fmt.Errorf("list keys: %w", err)
	}

	var pending []domain.ApprovalRequest
	for _, key := range keys {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		entry, err := bucket.Get(ctx, key)
		if err != nil {
			continue
		}

		var req domain.ApprovalRequest
		if err := json.Unmarshal(entry.Value(), &req); err != nil {
			continue
		}
		pending = append(pending, req)
	}

	return pending, nil
}

// --- Response Handling ---

// RespondToApproval allows external systems to respond to pending approvals.
func (r *NATSApprovalRouter) RespondToApproval(ctx context.Context, sessionID, approvalID string, resp domain.ApprovalResponse) error {
	sessionInstance := semdragons.ExtractInstance(sessionID)
	pendingKey := r.approvalPendingKey(sessionInstance, approvalID)

	bucket, err := r.client.GetKeyValueBucket(ctx, r.config.BucketName())
	if err != nil {
		return fmt.Errorf("get KV bucket: %w", err)
	}

	if err := bucket.Delete(ctx, pendingKey); err != nil {
		return fmt.Errorf("approval not found or already responded: %s", approvalID)
	}

	resp.RequestID = approvalID
	resp.SessionID = sessionID
	if resp.RespondedAt.IsZero() {
		resp.RespondedAt = time.Now()
	}

	if err := r.storeResolvedApproval(ctx, sessionInstance, approvalID, &resp); err != nil {
		r.logger.Warn("failed to store resolved approval", "approval_id", approvalID, "error", err)
	}

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

func (r *NATSApprovalRouter) storePendingApproval(ctx context.Context, sessionInstance string, req *domain.ApprovalRequest) error {
	key := r.approvalPendingKey(sessionInstance, req.ID)
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	bucket, err := r.client.GetKeyValueBucket(ctx, r.config.BucketName())
	if err != nil {
		return err
	}
	_, err = bucket.Put(ctx, key, data)
	return err
}

func (r *NATSApprovalRouter) resolveApproval(ctx context.Context, sessionInstance, approvalID string, resp *domain.ApprovalResponse) {
	bucket, err := r.client.GetKeyValueBucket(ctx, r.config.BucketName())
	if err != nil {
		r.logger.Warn("failed to get KV bucket for approval resolution", "error", err)
		return
	}

	pendingKey := r.approvalPendingKey(sessionInstance, approvalID)
	if err := bucket.Delete(ctx, pendingKey); err != nil {
		r.logger.Warn("failed to delete pending approval key", "key", pendingKey, "error", err)
	}

	if err := r.storeResolvedApproval(ctx, sessionInstance, approvalID, resp); err != nil {
		r.logger.Warn("failed to store resolved approval", "approval_id", approvalID, "error", err)
	}
}

func (r *NATSApprovalRouter) storeResolvedApproval(ctx context.Context, sessionInstance, approvalID string, resp *domain.ApprovalResponse) error {
	resolvedKey := r.approvalResolvedKey(sessionInstance, approvalID)
	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal resolved approval: %w", err)
	}
	bucket, err := r.client.GetKeyValueBucket(ctx, r.config.BucketName())
	if err != nil {
		return fmt.Errorf("get KV bucket: %w", err)
	}
	if _, err := bucket.Put(ctx, resolvedKey, data); err != nil {
		return fmt.Errorf("put resolved approval: %w", err)
	}
	return nil
}

// --- Utility Methods ---

// WaitForPendingApprovals waits until all pending approvals are resolved.
func (r *NATSApprovalRouter) WaitForPendingApprovals(ctx context.Context, sessionID string) error {
	sessionInstance := semdragons.ExtractInstance(sessionID)
	prefix := fmt.Sprintf("approval.pending.%s.", sessionInstance)

	bucket, err := r.client.GetKeyValueBucket(ctx, r.config.BucketName())
	if err != nil {
		return fmt.Errorf("get KV bucket: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		keys, err := bucket.Keys(ctx)
		if err != nil {
			return err
		}

		count := 0
		for _, key := range keys {
			if strings.HasPrefix(key, prefix) {
				count++
			}
		}

		if count == 0 {
			return nil
		}

		time.Sleep(100 * time.Millisecond)
	}
}

// GetResolvedApproval retrieves a resolved approval by ID.
func (r *NATSApprovalRouter) GetResolvedApproval(ctx context.Context, sessionID, approvalID string) (*domain.ApprovalResponse, error) {
	sessionInstance := semdragons.ExtractInstance(sessionID)
	key := r.approvalResolvedKey(sessionInstance, approvalID)

	bucket, err := r.client.GetKeyValueBucket(ctx, r.config.BucketName())
	if err != nil {
		return nil, fmt.Errorf("get KV bucket: %w", err)
	}

	entry, err := bucket.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("approval not found: %s", approvalID)
	}

	var resp domain.ApprovalResponse
	if err := json.Unmarshal(entry.Value(), &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &resp, nil
}
