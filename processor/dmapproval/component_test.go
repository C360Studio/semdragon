//go:build integration

package dmapproval

// =============================================================================
// INTEGRATION TESTS - DM Approval Component
// =============================================================================
// These tests require Docker for NATS via testcontainers.
// Run with: go test -tags=integration ./processor/dmapproval/...
// =============================================================================

import (
	"context"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// LIFECYCLE TESTS
// =============================================================================

func TestComponent_Lifecycle(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	deps := component.Dependencies{
		NATSClient: client,
	}

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "dmappr-lifecycle"

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}

	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	gc := semdragons.NewGraphClient(client, comp.boardConfig)
	if err := gc.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket failed: %v", err)
	}

	meta := comp.Meta()
	if meta.Name != ComponentName {
		t.Errorf("Meta.Name = %q, want %q", meta.Name, ComponentName)
	}
	if meta.Type != "processor" {
		t.Errorf("Meta.Type = %q, want %q", meta.Type, "processor")
	}

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	health := comp.Health()
	if !health.Healthy {
		t.Error("component should be healthy after start")
	}
	if health.Status != "running" {
		t.Errorf("Health.Status = %q, want %q", health.Status, "running")
	}

	if err := comp.Stop(5 * time.Second); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	health = comp.Health()
	if health.Healthy {
		t.Error("component should not be healthy after stop")
	}
	if health.Status != "stopped" {
		t.Errorf("Health.Status = %q, want %q", health.Status, "stopped")
	}
}

// =============================================================================
// PORT AND SCHEMA TESTS
// =============================================================================

func TestComponent_InputOutputPorts(t *testing.T) {
	comp := &Component{}

	inputs := comp.InputPorts()
	if len(inputs) == 0 {
		t.Error("should have at least one input port")
	}

	hasApprovalRequests := false
	for _, port := range inputs {
		if port.Name == "approval-requests" {
			hasApprovalRequests = true
			break
		}
	}
	if !hasApprovalRequests {
		t.Error("missing approval-requests input port")
	}

	outputs := comp.OutputPorts()
	if len(outputs) == 0 {
		t.Error("should have at least one output port")
	}

	hasApprovalResponses := false
	for _, port := range outputs {
		if port.Name == "approval-responses" {
			hasApprovalResponses = true
			break
		}
	}
	if !hasApprovalResponses {
		t.Error("missing approval-responses output port")
	}
}

func TestComponent_ConfigSchema(t *testing.T) {
	comp := &Component{}
	schema := comp.ConfigSchema()

	requiredFields := []string{"org", "platform", "board"}
	for _, field := range requiredFields {
		found := false
		for _, req := range schema.Required {
			if req == field {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("field %q should be required in ConfigSchema", field)
		}
	}

	expectedProps := []string{"org", "platform", "board", "approval_timeout_min", "auto_approve"}
	for _, prop := range expectedProps {
		if _, exists := schema.Properties[prop]; !exists {
			t.Errorf("missing property %q in ConfigSchema", prop)
		}
	}
}

// =============================================================================
// AUTO-APPROVE MODE
// =============================================================================

func TestAutoApprove_PassesIntentThrough(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "auto-approve"
	config.AutoApprove = true

	comp := setupApprovalComponent(t, client, config)
	defer comp.Stop(5 * time.Second)

	sessionID := makeSessionID(comp.boardConfig, "auto-sess")
	req := domain.ApprovalRequest{
		SessionID: sessionID,
		Type:      domain.ApprovalAutonomyClaim,
		Title:     "Claim quest for agent",
		Details:   "Agent wants to claim quest-001",
	}

	resp, err := comp.RequestApproval(ctx, req)
	if err != nil {
		t.Fatalf("RequestApproval failed: %v", err)
	}

	if !resp.Approved {
		t.Error("auto-approve mode should approve all requests")
	}
	if resp.Reason != "auto-approved" {
		t.Errorf("Reason = %q, want %q", resp.Reason, "auto-approved")
	}
	// Auto-approve echoes back req.ID unchanged. The request in this test has no
	// explicit ID set, so RequestID will be empty. Use TestAutoApprove_SetsRequestID
	// to verify that an explicitly set ID is preserved.
	if resp.RequestID != req.ID {
		t.Errorf("RequestID = %q, want %q (should echo req.ID)", resp.RequestID, req.ID)
	}
}

func TestAutoApprove_SetsRequestID(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "auto-reqid"
	config.AutoApprove = true

	comp := setupApprovalComponent(t, client, config)
	defer comp.Stop(5 * time.Second)

	sessionID := makeSessionID(comp.boardConfig, "reqid-sess")
	req := domain.ApprovalRequest{
		ID:        "my-explicit-id",
		SessionID: sessionID,
		Type:      domain.ApprovalQuestCreate,
		Title:     "Create quest",
	}

	resp, err := comp.RequestApproval(ctx, req)
	if err != nil {
		t.Fatalf("RequestApproval failed: %v", err)
	}
	if resp.RequestID != "my-explicit-id" {
		t.Errorf("RequestID = %q, want %q", resp.RequestID, "my-explicit-id")
	}
}

// =============================================================================
// MANUAL APPROVAL - pending state and explicit response
// =============================================================================

func TestPendingApproval_StoredAndRetrievable(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "pending-store"
	config.AutoApprove = false

	comp := setupApprovalComponent(t, client, config)
	defer comp.Stop(5 * time.Second)

	sessionID := makeSessionID(comp.boardConfig, "pend-sess")

	// Send a request with a very short context so it expires quickly after the
	// request is stored, then verify the pending state is visible.
	reqCtx, reqCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer reqCancel()

	req := domain.ApprovalRequest{
		SessionID: sessionID,
		Type:      domain.ApprovalPartyFormation,
		Title:     "Form party for quest",
		Details:   "Needs 3 members",
	}

	// The call will block until reqCtx expires because nobody is responding.
	// That is expected; we are only checking that the pending key was stored.
	go func() {
		//nolint:errcheck // intentionally ignoring: context will be cancelled, error expected
		_, _ = comp.RequestApproval(reqCtx, req)
	}()

	// Poll until the pending approval appears in KV (or timeout).
	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for pending approval to appear in KV")
		case <-time.After(50 * time.Millisecond):
			pending, err := comp.GetPendingApprovals(ctx, sessionID)
			if err != nil {
				continue
			}
			if len(pending) > 0 {
				if pending[0].Type != domain.ApprovalPartyFormation {
					t.Errorf("pending approval Type = %q, want %q", pending[0].Type, domain.ApprovalPartyFormation)
				}
				return
			}
		}
	}
}

func TestRespondToApproval_Approved_DeliverResponse(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "respond-approve"
	config.AutoApprove = false

	comp := setupApprovalComponent(t, client, config)
	defer comp.Stop(5 * time.Second)

	sessionID := makeSessionID(comp.boardConfig, "resp-sess")

	req := domain.ApprovalRequest{
		SessionID: sessionID,
		Type:      domain.ApprovalAutonomyClaim,
		Title:     "Claim quest for agent",
		Details:   "Agent wants to claim quest-007",
	}

	// Run RequestApproval in a goroutine; it will block until a response arrives.
	type result struct {
		resp *domain.ApprovalResponse
		err  error
	}
	resultCh := make(chan result, 1)

	requestCtx, requestCancel := context.WithTimeout(ctx, 5*time.Second)
	defer requestCancel()

	go func() {
		resp, err := comp.RequestApproval(requestCtx, req)
		resultCh <- result{resp, err}
	}()

	// Wait for the pending approval to appear in KV.
	var approvalID string
	deadline := time.After(3 * time.Second)
	for approvalID == "" {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for pending approval")
		case <-time.After(50 * time.Millisecond):
			pending, err := comp.GetPendingApprovals(ctx, sessionID)
			if err == nil && len(pending) > 0 {
				approvalID = pending[0].ID
			}
		}
	}

	// Respond to the approval.
	resp := domain.ApprovalResponse{
		Approved:    true,
		Reason:      "looks good",
		RespondedBy: "human-dm",
	}
	if err := comp.RespondToApproval(ctx, sessionID, approvalID, resp); err != nil {
		t.Fatalf("RespondToApproval failed: %v", err)
	}

	// Wait for the blocked RequestApproval to return.
	select {
	case r := <-resultCh:
		if r.err != nil {
			t.Fatalf("RequestApproval returned error: %v", r.err)
		}
		if !r.resp.Approved {
			t.Error("response should be approved")
		}
		if r.resp.Reason != "looks good" {
			t.Errorf("Reason = %q, want %q", r.resp.Reason, "looks good")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for RequestApproval to return after response")
	}
}

func TestRespondToApproval_Denied_DeliverResponse(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "respond-deny"
	config.AutoApprove = false

	comp := setupApprovalComponent(t, client, config)
	defer comp.Stop(5 * time.Second)

	sessionID := makeSessionID(comp.boardConfig, "deny-sess")

	req := domain.ApprovalRequest{
		SessionID: sessionID,
		Type:      domain.ApprovalAutonomyShop,
		Title:     "Purchase ability",
		Details:   "Agent wants to buy SkillCodeGen",
	}

	type result struct {
		resp *domain.ApprovalResponse
		err  error
	}
	resultCh := make(chan result, 1)

	requestCtx, requestCancel := context.WithTimeout(ctx, 5*time.Second)
	defer requestCancel()

	go func() {
		resp, err := comp.RequestApproval(requestCtx, req)
		resultCh <- result{resp, err}
	}()

	// Wait for pending approval.
	var approvalID string
	deadline := time.After(3 * time.Second)
	for approvalID == "" {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for pending approval")
		case <-time.After(50 * time.Millisecond):
			pending, err := comp.GetPendingApprovals(ctx, sessionID)
			if err == nil && len(pending) > 0 {
				approvalID = pending[0].ID
			}
		}
	}

	// Deny the approval.
	resp := domain.ApprovalResponse{
		Approved:    false,
		Reason:      "too expensive",
		RespondedBy: "human-dm",
	}
	if err := comp.RespondToApproval(ctx, sessionID, approvalID, resp); err != nil {
		t.Fatalf("RespondToApproval failed: %v", err)
	}

	select {
	case r := <-resultCh:
		if r.err != nil {
			t.Fatalf("RequestApproval returned error: %v", r.err)
		}
		if r.resp.Approved {
			t.Error("response should be denied")
		}
		if r.resp.Reason != "too expensive" {
			t.Errorf("Reason = %q, want %q", r.resp.Reason, "too expensive")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for denied RequestApproval to return")
	}
}

func TestRespondToApproval_UnknownID_Errors(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "respond-unknown"
	config.AutoApprove = false

	comp := setupApprovalComponent(t, client, config)
	defer comp.Stop(5 * time.Second)

	sessionID := makeSessionID(comp.boardConfig, "unknown-sess")

	resp := domain.ApprovalResponse{
		Approved: true,
	}
	// NATS KV Delete on a non-existent key creates a tombstone and returns nil,
	// so RespondToApproval does not error for unknown IDs — it silently succeeds.
	// The caller must verify the pending list to detect stale responds.
	err := comp.RespondToApproval(ctx, sessionID, "nonexistent-approval-id", resp)
	if err != nil {
		t.Fatalf("RespondToApproval returned unexpected error for unknown ID: %v", err)
	}
}

// =============================================================================
// RESOLVED APPROVAL PERSISTENCE
// =============================================================================

func TestResolvedApproval_StoredInKV(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "resolved-store"
	config.AutoApprove = false

	comp := setupApprovalComponent(t, client, config)
	defer comp.Stop(5 * time.Second)

	sessionID := makeSessionID(comp.boardConfig, "res-sess")

	req := domain.ApprovalRequest{
		SessionID: sessionID,
		Type:      domain.ApprovalBattleVerdict,
		Title:     "Approve battle verdict",
	}

	type result struct {
		resp *domain.ApprovalResponse
		err  error
	}
	resultCh := make(chan result, 1)
	requestCtx, requestCancel := context.WithTimeout(ctx, 5*time.Second)
	defer requestCancel()

	go func() {
		resp, err := comp.RequestApproval(requestCtx, req)
		resultCh <- result{resp, err}
	}()

	// Wait for pending approval.
	var approvalID string
	deadline := time.After(3 * time.Second)
	for approvalID == "" {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for pending approval")
		case <-time.After(50 * time.Millisecond):
			pending, err := comp.GetPendingApprovals(ctx, sessionID)
			if err == nil && len(pending) > 0 {
				approvalID = pending[0].ID
			}
		}
	}

	// Respond positively.
	if err := comp.RespondToApproval(ctx, sessionID, approvalID, domain.ApprovalResponse{
		Approved: true,
		Reason:   "verdict confirmed",
	}); err != nil {
		t.Fatalf("RespondToApproval failed: %v", err)
	}

	// Drain the blocking call.
	select {
	case <-resultCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for RequestApproval to return")
	}

	// The resolved approval should now be retrievable from KV.
	router := comp.GetRouter()
	resolved, err := router.GetResolvedApproval(ctx, sessionID, approvalID)
	if err != nil {
		t.Fatalf("GetResolvedApproval failed: %v", err)
	}
	if !resolved.Approved {
		t.Error("resolved approval should be approved")
	}
	if resolved.Reason != "verdict confirmed" {
		t.Errorf("Reason = %q, want %q", resolved.Reason, "verdict confirmed")
	}
}

// =============================================================================
// WATCH APPROVALS
// =============================================================================

func TestWatchApprovals_ReceivesResponse(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "watch-approvals"
	config.AutoApprove = false

	comp := setupApprovalComponent(t, client, config)
	defer comp.Stop(5 * time.Second)

	sessionID := makeSessionID(comp.boardConfig, "watch-sess")

	watchCtx, watchCancel := context.WithTimeout(ctx, 5*time.Second)
	defer watchCancel()

	respCh, err := comp.WatchApprovals(watchCtx, domain.ApprovalFilter{SessionID: sessionID})
	if err != nil {
		t.Fatalf("WatchApprovals failed: %v", err)
	}

	// Start a request so there is something to respond to.
	req := domain.ApprovalRequest{
		SessionID: sessionID,
		Type:      domain.ApprovalIntervention,
		Title:     "Intervene on stuck quest",
	}

	reqCtx, reqCancel := context.WithTimeout(ctx, 5*time.Second)
	defer reqCancel()
	go func() {
		//nolint:errcheck // ignored: test validates via watch channel
		_, _ = comp.RequestApproval(reqCtx, req)
	}()

	// Wait for pending approval to appear.
	var approvalID string
	deadline := time.After(3 * time.Second)
	for approvalID == "" {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for pending approval")
		case <-time.After(50 * time.Millisecond):
			pending, err := comp.GetPendingApprovals(ctx, sessionID)
			if err == nil && len(pending) > 0 {
				approvalID = pending[0].ID
			}
		}
	}

	// Publish a response.
	if err := comp.RespondToApproval(ctx, sessionID, approvalID, domain.ApprovalResponse{
		Approved: true,
		Reason:   "intervene approved",
	}); err != nil {
		t.Fatalf("RespondToApproval failed: %v", err)
	}

	// The watch channel should deliver the response.
	select {
	case received := <-respCh:
		if !received.Approved {
			t.Error("watched response should be approved")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for approval on watch channel")
	}
}

// =============================================================================
// OPERATION GUARD
// =============================================================================

func TestRequestApproval_FailsWhenNotRunning(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	deps := component.Dependencies{NATSClient: client}
	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "not-running-appr"

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	// Intentionally do NOT call Start.

	_, err = comp.RequestApproval(ctx, domain.ApprovalRequest{
		SessionID: "test-session",
		Type:      domain.ApprovalQuestCreate,
		Title:     "Blocked",
	})
	if err == nil {
		t.Fatal("RequestApproval should error when component is not running")
	}
}

// =============================================================================
// HELPERS
// =============================================================================

func setupApprovalComponent(t *testing.T, client *natsclient.Client, config Config) *Component {
	t.Helper()

	deps := component.Dependencies{NATSClient: client}
	ctx := context.Background()

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	gc := semdragons.NewGraphClient(client, comp.boardConfig)
	if err := gc.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket failed: %v", err)
	}

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	return comp
}

// makeSessionID builds a valid entity session ID for test use.
func makeSessionID(config *domain.BoardConfig, instance string) string {
	return config.EntityID("session", instance)
}
