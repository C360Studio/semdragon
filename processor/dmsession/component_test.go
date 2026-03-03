//go:build integration

package dmsession

// =============================================================================
// INTEGRATION TESTS - DM Session Component
// =============================================================================
// These tests require Docker for NATS via testcontainers.
// Run with: go test -tags=integration ./processor/dmsession/...
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
	config.Board = "dmsess-lifecycle"

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}

	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Ensure board-specific KV bucket exists.
	gc := semdragons.NewGraphClient(client, comp.boardConfig)
	if err := gc.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket failed: %v", err)
	}

	// Verify Meta
	meta := comp.Meta()
	if meta.Name != ComponentName {
		t.Errorf("Meta.Name = %q, want %q", meta.Name, ComponentName)
	}
	if meta.Type != "processor" {
		t.Errorf("Meta.Type = %q, want %q", meta.Type, "processor")
	}

	// Start the component.
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
		t.Error("should have at least one input port defined")
	}

	// Verify the session-commands input port exists.
	hasSessionCommands := false
	for _, port := range inputs {
		if port.Name == "session-commands" {
			hasSessionCommands = true
			break
		}
	}
	if !hasSessionCommands {
		t.Error("missing session-commands input port")
	}

	outputs := comp.OutputPorts()
	if len(outputs) == 0 {
		t.Error("should have at least one output port defined")
	}

	// Verify the session-events output port exists.
	hasSessionEvents := false
	for _, port := range outputs {
		if port.Name == "session-events" {
			hasSessionEvents = true
			break
		}
	}
	if !hasSessionEvents {
		t.Error("missing session-events output port")
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

	expectedProps := []string{"org", "platform", "board", "default_mode", "max_concurrent"}
	for _, prop := range expectedProps {
		if _, exists := schema.Properties[prop]; !exists {
			t.Errorf("missing property %q in ConfigSchema", prop)
		}
	}
}

// =============================================================================
// SESSION CREATION AND RETRIEVAL
// =============================================================================

func TestStartSession_CreatesAndPersists(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupSessionComponent(t, client, "create-session")
	defer comp.Stop(5 * time.Second)

	cfg := domain.SessionConfig{
		Mode: domain.DMManual,
		Name: "Test Session",
	}

	session, err := comp.StartSession(ctx, cfg)
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	if session.ID == "" {
		t.Error("session ID should be set")
	}
	if !session.Active {
		t.Error("session should be active after start")
	}
	if session.Config.Name != "Test Session" {
		t.Errorf("Config.Name = %q, want %q", session.Config.Name, "Test Session")
	}

	// Verify it is listed in active sessions.
	activeSessions := comp.ListActiveSessions()
	found := false
	for _, s := range activeSessions {
		if s.ID == session.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("started session %q not in ListActiveSessions", session.ID)
	}
}

func TestGetSession_ReturnsFromMemory(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupSessionComponent(t, client, "get-session")
	defer comp.Stop(5 * time.Second)

	cfg := domain.SessionConfig{
		Mode: domain.DMAssisted,
		Name: "Retrieval Test",
	}

	started, err := comp.StartSession(ctx, cfg)
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	retrieved, err := comp.GetSession(ctx, started.ID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if retrieved.ID != started.ID {
		t.Errorf("retrieved ID = %q, want %q", retrieved.ID, started.ID)
	}
	if retrieved.Config.Name != "Retrieval Test" {
		t.Errorf("Config.Name = %q, want %q", retrieved.Config.Name, "Retrieval Test")
	}
}

func TestGetSession_LoadsFromKV(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	// Start a session with the first component instance.
	comp1 := setupSessionComponent(t, client, "kv-persist")
	cfg := domain.SessionConfig{
		Mode: domain.DMFullAuto,
		Name: "Persistence Test",
	}
	started, err := comp1.StartSession(ctx, cfg)
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}
	// Stop comp1 so its in-memory cache is gone; the session only lives in KV.
	if err := comp1.Stop(5 * time.Second); err != nil {
		t.Fatalf("Stop comp1 failed: %v", err)
	}

	// Start a fresh component that shares the same KV bucket.
	comp2 := setupSessionComponent(t, client, "kv-persist")
	defer comp2.Stop(5 * time.Second)

	// GetSession must fall back to KV because the in-memory cache is empty.
	retrieved, err := comp2.GetSession(ctx, started.ID)
	if err != nil {
		t.Fatalf("GetSession from KV failed: %v", err)
	}
	if retrieved.ID != started.ID {
		t.Errorf("retrieved ID = %q, want %q", retrieved.ID, started.ID)
	}
	if retrieved.Config.Name != "Persistence Test" {
		t.Errorf("Config.Name = %q, want %q", retrieved.Config.Name, "Persistence Test")
	}
}

// =============================================================================
// SESSION END
// =============================================================================

func TestEndSession_ProduceSummary(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupSessionComponent(t, client, "end-session")
	defer comp.Stop(5 * time.Second)

	cfg := domain.SessionConfig{
		Mode: domain.DMManual,
		Name: "End Session Test",
	}

	session, err := comp.StartSession(ctx, cfg)
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	summary, err := comp.EndSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("EndSession failed: %v", err)
	}

	if summary.SessionID != session.ID {
		t.Errorf("Summary.SessionID = %q, want %q", summary.SessionID, session.ID)
	}

	// After ending, the session should no longer be in the active list.
	activeSessions := comp.ListActiveSessions()
	for _, s := range activeSessions {
		if s.ID == session.ID {
			t.Errorf("ended session %q should not appear in ListActiveSessions", session.ID)
		}
	}
}

func TestEndSession_UnknownID_Errors(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupSessionComponent(t, client, "end-unknown")
	defer comp.Stop(5 * time.Second)

	_, err := comp.EndSession(ctx, "nonexistent-session-id")
	if err == nil {
		t.Fatal("EndSession should return an error for an unknown session ID")
	}
}

// =============================================================================
// MULTI-TURN SESSION (conversation persistence)
// =============================================================================

func TestSession_MultipleActiveSessionsTracked(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupSessionComponent(t, client, "multi-session")
	defer comp.Stop(5 * time.Second)

	// Start three sessions.
	for i := range 3 {
		cfg := domain.SessionConfig{
			Mode: domain.DMAssisted,
			Name: "Session " + string(rune('A'+i)),
		}
		if _, err := comp.StartSession(ctx, cfg); err != nil {
			t.Fatalf("StartSession %d failed: %v", i, err)
		}
	}

	activeSessions := comp.ListActiveSessions()
	if len(activeSessions) != 3 {
		t.Errorf("active session count = %d, want 3", len(activeSessions))
	}
}

func TestSession_EndedSessionRemovedFromActive(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	comp := setupSessionComponent(t, client, "remove-session")
	defer comp.Stop(5 * time.Second)

	cfg := domain.SessionConfig{
		Mode: domain.DMManual,
		Name: "Removal Test Session",
	}

	s1, err := comp.StartSession(ctx, cfg)
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}
	s2, err := comp.StartSession(ctx, cfg)
	if err != nil {
		t.Fatalf("StartSession 2 failed: %v", err)
	}

	if _, err := comp.EndSession(ctx, s1.ID); err != nil {
		t.Fatalf("EndSession failed: %v", err)
	}

	activeSessions := comp.ListActiveSessions()
	if len(activeSessions) != 1 {
		t.Errorf("active session count = %d, want 1 after ending s1", len(activeSessions))
	}
	if activeSessions[0].ID != s2.ID {
		t.Errorf("remaining session ID = %q, want %q", activeSessions[0].ID, s2.ID)
	}
}

// =============================================================================
// OPERATION GUARD - component must be running
// =============================================================================

func TestStartSession_FailsWhenNotRunning(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	deps := component.Dependencies{NATSClient: client}
	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "not-running"

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	// Intentionally do NOT call Start.

	_, err = comp.StartSession(ctx, domain.SessionConfig{Mode: domain.DMManual, Name: "Blocked"})
	if err == nil {
		t.Fatal("StartSession should error when component is not running")
	}
}

// =============================================================================
// HELPERS
// =============================================================================

func setupSessionComponent(t *testing.T, client *natsclient.Client, boardName string) *Component {
	t.Helper()

	deps := component.Dependencies{NATSClient: client}
	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = boardName

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
