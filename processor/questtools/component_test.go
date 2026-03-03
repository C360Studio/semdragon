//go:build integration

package questtools

// =============================================================================
// INTEGRATION TESTS - QuestTools Component
// =============================================================================
// These tests require Docker for NATS via testcontainers.
// Run with: go test -tags=integration ./processor/questtools/...
//
// The component consumes tool.execute.* messages from the AGENT JetStream
// stream, enforces tier/skill/sandbox gates via executor.ToolRegistry, and
// publishes tool.result.* responses back to the same stream.
// =============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go/jetstream"
)

// =============================================================================
// CONSTANTS
// =============================================================================

// agentStreamName is the JetStream stream the component reads from and writes to.
const agentStreamName = "AGENT"

// agentStreamSubjects is the full set of subjects the AGENT stream must cover
// for both publisher (questtools) and subscribers (test consumers).
var agentStreamSubjects = []string{
	"agent.task.*",
	"agent.complete.*",
	"agent.failed.*",
	"agent.created.*",
	"agent.signal.*",
	"agent.context.compaction.*",
	"agent.boid.>",
	"agent.request.>",
	"agent.response.>",
	"tool.execute.*",
	"tool.result.>",
}

// journeymanTier is the float64 trust tier for Journeyman (level 6-10).
// JSON numbers are float64 after unmarshalling, so metadata values must match.
const journeymanTier = float64(domain.TierJourneyman) // == 1.0

// expertTier is the float64 trust tier for Expert (level 11-15).
const expertTier = float64(domain.TierExpert) // == 2.0

// apprenticeTier is the float64 trust tier for Apprentice (level 1-5).
const apprenticeTier = float64(domain.TierApprentice) // == 0.0

// =============================================================================
// LIFECYCLE TESTS
// =============================================================================

// TestComponentLifecycle verifies Create → Initialize → Start → Health → Stop.
func TestComponentLifecycle(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ensureAgentStream(t, tc.Client)
	ctx := context.Background()

	deps := component.Dependencies{
		NATSClient: tc.Client,
	}

	cfg := DefaultConfig()
	cfg.Org = "test"
	cfg.Platform = "integration"
	cfg.Board = "lifecycle"
	cfg.ConsumerNameSuffix = "lifecycle"
	cfg.DeleteConsumerOnStop = true

	comp, err := NewFromConfig(cfg, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}

	// Initialize must succeed before Start.
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Meta describes the component correctly.
	meta := comp.Meta()
	if meta.Name != ComponentName {
		t.Errorf("Meta.Name = %q, want %q", meta.Name, ComponentName)
	}
	if meta.Type != "processor" {
		t.Errorf("Meta.Type = %q, want %q", meta.Type, "processor")
	}

	// Component must be unhealthy before Start.
	if comp.Health().Healthy {
		t.Error("component should not be healthy before Start")
	}

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer comp.Stop(5 * time.Second)

	// Component must report healthy after Start.
	health := comp.Health()
	if !health.Healthy {
		t.Error("component should be healthy after Start")
	}
	if health.Status != "running" {
		t.Errorf("Health.Status = %q, want %q", health.Status, "running")
	}

	if err := comp.Stop(5 * time.Second); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// After Stop, component must be unhealthy.
	health = comp.Health()
	if health.Healthy {
		t.Error("component should not be healthy after Stop")
	}
	if health.Status != "stopped" {
		t.Errorf("Health.Status = %q, want %q", health.Status, "stopped")
	}
}

// =============================================================================
// PORT DECLARATION TESTS
// =============================================================================

// TestInputOutputPorts verifies ports declare correct stream subjects.
func TestInputOutputPorts(t *testing.T) {
	// Port declarations are pure metadata — no NATS needed.
	comp := &Component{}

	inputs := comp.InputPorts()
	if len(inputs) == 0 {
		t.Fatal("InputPorts must not be empty")
	}

	// Verify the tool-execute input port exists with correct stream and subject.
	var toolExecPort *component.Port
	for i := range inputs {
		if inputs[i].Name == "tool-execute" {
			toolExecPort = &inputs[i]
			break
		}
	}
	if toolExecPort == nil {
		t.Fatal("missing tool-execute input port")
	}
	if !toolExecPort.Required {
		t.Error("tool-execute port must be required")
	}
	if jsPort, ok := toolExecPort.Config.(component.JetStreamPort); ok {
		if jsPort.StreamName != "AGENT" {
			t.Errorf("tool-execute StreamName = %q, want %q", jsPort.StreamName, "AGENT")
		}
		if len(jsPort.Subjects) == 0 || jsPort.Subjects[0] != "tool.execute.*" {
			t.Errorf("tool-execute subjects = %v, want [tool.execute.*]", jsPort.Subjects)
		}
	} else {
		t.Error("tool-execute port Config must be component.JetStreamPort")
	}

	outputs := comp.OutputPorts()
	if len(outputs) == 0 {
		t.Fatal("OutputPorts must not be empty")
	}

	// Verify the tool-result output port exists with correct stream and subject.
	var toolResultPort *component.Port
	for i := range outputs {
		if outputs[i].Name == "tool-result" {
			toolResultPort = &outputs[i]
			break
		}
	}
	if toolResultPort == nil {
		t.Fatal("missing tool-result output port")
	}
	if jsPort, ok := toolResultPort.Config.(component.JetStreamPort); ok {
		if jsPort.StreamName != "AGENT" {
			t.Errorf("tool-result StreamName = %q, want %q", jsPort.StreamName, "AGENT")
		}
		if len(jsPort.Subjects) == 0 || jsPort.Subjects[0] != "tool.result.*" {
			t.Errorf("tool-result subjects = %v, want [tool.result.*]", jsPort.Subjects)
		}
	} else {
		t.Error("tool-result port Config must be component.JetStreamPort")
	}
}

// =============================================================================
// CONFIG SCHEMA TESTS
// =============================================================================

// TestConfigSchema verifies required fields are declared in ConfigSchema.
func TestConfigSchema(t *testing.T) {
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
			t.Errorf("field %q must be in ConfigSchema.Required", field)
		}
	}

	// Confirm key properties are documented.
	expectedProps := []string{
		"org", "platform", "board",
		"stream_name", "timeout", "enable_builtins", "sandbox_dir",
	}
	for _, prop := range expectedProps {
		if _, exists := schema.Properties[prop]; !exists {
			t.Errorf("property %q missing from ConfigSchema", prop)
		}
	}
}

// =============================================================================
// TOOL EXECUTION SUCCESS TEST
// =============================================================================

// TestToolExecutionSuccess publishes a read_file ToolCall with a Journeyman
// agent that has the code_generation skill. The built-in read_file handler
// returns the file contents; the test verifies a non-error ToolResult.
func TestToolExecutionSuccess(t *testing.T) {
	// Create a temporary file for read_file to read.
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "hello.txt")
	if err := os.WriteFile(tmpFile, []byte("hello from questtools"), 0o644); err != nil {
		t.Fatalf("create temp file: %v", err)
	}

	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ensureAgentStream(t, tc.Client)
	ctx := context.Background()

	comp := setupComponent(t, tc.Client, "exec-success")
	defer comp.Stop(5 * time.Second)

	callID := "call-success-001"
	call := agentic.ToolCall{
		ID:   callID,
		Name: "read_file",
		Arguments: map[string]any{
			"path": tmpFile,
		},
		Metadata: map[string]any{
			"agent_id":   "test-agent-success",
			"trust_tier": journeymanTier,
			"skills":     []any{string(domain.SkillCodeGen)},
			"quest_id":   "quest-success-001",
		},
	}

	publishToolCall(t, tc.Client, ctx, "tool.execute.read_file", call)

	result := pollForToolResult(t, tc.Client, ctx, callID, 10*time.Second)
	if result.Error != "" {
		t.Errorf("expected successful execution, got error: %s", result.Error)
	}
	if !strings.Contains(result.Content, "hello from questtools") {
		t.Errorf("Content = %q, want it to contain %q", result.Content, "hello from questtools")
	}
	if result.CallID != callID {
		t.Errorf("CallID = %q, want %q", result.CallID, callID)
	}
}

// =============================================================================
// TIER REJECTION TEST
// =============================================================================

// TestToolExecutionTierRejection publishes a write_file ToolCall with an
// Apprentice agent (tier 0). write_file requires Expert (tier 2). The
// component must publish a ToolResult with a tier/authorization error.
func TestToolExecutionTierRejection(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ensureAgentStream(t, tc.Client)
	ctx := context.Background()

	comp := setupComponent(t, tc.Client, "tier-reject")
	defer comp.Stop(5 * time.Second)

	callID := "call-tier-reject-001"
	call := agentic.ToolCall{
		ID:   callID,
		Name: "write_file",
		Arguments: map[string]any{
			"path":    "/tmp/should-not-write.txt",
			"content": "unauthorized write",
		},
		Metadata: map[string]any{
			"agent_id":   "apprentice-agent",
			"trust_tier": apprenticeTier, // Tier 0 — below Expert (tier 2) required by write_file.
			"skills":     []any{string(domain.SkillCodeGen)},
			"quest_id":   "quest-tier-reject-001",
		},
	}

	publishToolCall(t, tc.Client, ctx, "tool.execute.write_file", call)

	result := pollForToolResult(t, tc.Client, ctx, callID, 10*time.Second)
	if result.Error == "" {
		t.Error("expected an authorization error result, got empty error field")
	}
	// The error must mention insufficient tier or similar.
	if !strings.Contains(strings.ToLower(result.Error), "tier") &&
		!strings.Contains(strings.ToLower(result.Error), "insufficient") {
		t.Errorf("error %q should mention tier or insufficient authorization", result.Error)
	}
}

// =============================================================================
// SKILL REJECTION TEST
// =============================================================================

// TestToolExecutionSkillRejection publishes a read_file ToolCall with a
// Journeyman agent that has no required skills (code_generation, research, or
// analysis). The component must publish a ToolResult with a skill error.
func TestToolExecutionSkillRejection(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ensureAgentStream(t, tc.Client)
	ctx := context.Background()

	comp := setupComponent(t, tc.Client, "skill-reject")
	defer comp.Stop(5 * time.Second)

	callID := "call-skill-reject-001"
	call := agentic.ToolCall{
		ID:   callID,
		Name: "read_file",
		Arguments: map[string]any{
			"path": "/tmp/irrelevant.txt",
		},
		Metadata: map[string]any{
			"agent_id": "no-skill-agent",
			// Journeyman tier satisfies the tier gate, but no matching skills.
			"trust_tier": journeymanTier,
			// planning is NOT in read_file's required skills [code_generation, research, analysis].
			"skills":   []any{string(domain.SkillPlanning)},
			"quest_id": "quest-skill-reject-001",
		},
	}

	publishToolCall(t, tc.Client, ctx, "tool.execute.read_file", call)

	result := pollForToolResult(t, tc.Client, ctx, callID, 10*time.Second)
	if result.Error == "" {
		t.Error("expected a skill error result, got empty error field")
	}
	if !strings.Contains(strings.ToLower(result.Error), "skill") {
		t.Errorf("error %q should mention skill requirement", result.Error)
	}
}

// =============================================================================
// INVALID PAYLOAD TEST
// =============================================================================

// TestInvalidToolCallPayload publishes malformed JSON to tool.execute.badcall.
// The component must publish an error ToolResult using the tool name from the
// subject, rather than silently dropping the message.
func TestInvalidToolCallPayload(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ensureAgentStream(t, tc.Client)
	ctx := context.Background()

	comp := setupComponent(t, tc.Client, "bad-payload")
	defer comp.Stop(5 * time.Second)

	// Publish raw malformed JSON directly (not via publishToolCall helper).
	js, err := tc.Client.JetStream()
	if err != nil {
		t.Fatalf("JetStream(): %v", err)
	}
	malformed := []byte(`{this is not valid json`)
	if _, err := js.Publish(ctx, "tool.execute.badcall", malformed); err != nil {
		t.Fatalf("Publish malformed payload: %v", err)
	}

	// The handler parses the tool name from the subject (third segment = "badcall")
	// and publishes to tool.result.badcall.
	result := pollForToolResult(t, tc.Client, ctx, "badcall", 10*time.Second)
	if result.Error == "" {
		t.Error("expected error result for malformed payload, got empty error field")
	}
	if !strings.Contains(strings.ToLower(result.Error), "unmarshal") &&
		!strings.Contains(strings.ToLower(result.Error), "invalid") &&
		!strings.Contains(strings.ToLower(result.Error), "json") {
		t.Errorf("error %q should indicate a JSON parse failure", result.Error)
	}
}

// =============================================================================
// TOOLCALL VALIDATION TEST
// =============================================================================

// TestToolCallValidation publishes a ToolCall with an empty Name field.
// The component must publish a non-empty error ToolResult using the call's ID.
// The empty Name either triggers Validate() rejection ("invalid tool call: ...")
// or falls through to an "unknown tool: " error from the registry — both are
// acceptable outcomes; the test asserts only that an error was returned.
func TestToolCallValidation(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ensureAgentStream(t, tc.Client)
	ctx := context.Background()

	comp := setupComponent(t, tc.Client, "validation")
	defer comp.Stop(5 * time.Second)

	callID := "call-validate-001"
	// ToolCall with a valid ID but empty Name. Validate() may reject this, or
	// the registry lookup will fail with "unknown tool: ".
	call := agentic.ToolCall{
		ID:   callID,
		Name: "", // Empty name — must produce an error result.
		Metadata: map[string]any{
			"agent_id":   "test-agent",
			"trust_tier": journeymanTier,
		},
	}

	// Publish to tool.execute.unknown; the subject-level tool token is only
	// used by the unmarshal-failure path, not the validation path.
	publishToolCall(t, tc.Client, ctx, "tool.execute.unknown", call)

	result := pollForToolResult(t, tc.Client, ctx, callID, 10*time.Second)
	if result.Error == "" {
		t.Error("expected an error result for a ToolCall with empty Name, got empty error field")
	}
	if result.CallID != callID {
		t.Errorf("CallID = %q, want %q", result.CallID, callID)
	}
}

// =============================================================================
// METADATA CONTEXT RECONSTRUCTION TEST
// =============================================================================

// TestMetadataToContextReconstruction verifies that full metadata (agent_id,
// trust_tier, skills, quest_id) is correctly mapped into the agent/quest stubs
// used for gate checks. We verify indirectly through a successful execution:
// if the tier and skill gates pass, context reconstruction is correct.
func TestMetadataToContextReconstruction(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "context.txt")
	if err := os.WriteFile(tmpFile, []byte("context verified"), 0o644); err != nil {
		t.Fatalf("create temp file: %v", err)
	}

	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ensureAgentStream(t, tc.Client)
	ctx := context.Background()

	comp := setupComponent(t, tc.Client, "meta-reconstruct")
	defer comp.Stop(5 * time.Second)

	callID := "call-meta-001"
	call := agentic.ToolCall{
		ID:     callID,
		Name:   "list_directory",
		LoopID: "loop-meta-abc",
		Arguments: map[string]any{
			"path": tmpDir,
		},
		Metadata: map[string]any{
			"agent_id":   "meta-agent-42",
			"trust_tier": journeymanTier,
			// analysis satisfies list_directory's skill requirement.
			"skills":   []any{string(domain.SkillAnalysis)},
			"quest_id": "quest-meta-001",
		},
	}

	publishToolCall(t, tc.Client, ctx, "tool.execute.list_directory", call)

	result := pollForToolResult(t, tc.Client, ctx, callID, 10*time.Second)
	if result.Error != "" {
		t.Errorf("expected successful execution with correct context, got error: %s", result.Error)
	}
	if result.CallID != callID {
		t.Errorf("CallID = %q, want %q", result.CallID, callID)
	}
	// The directory listing must contain the file we created.
	if !strings.Contains(result.Content, "context.txt") {
		t.Errorf("Content = %q, want it to contain %q", result.Content, "context.txt")
	}
}

// =============================================================================
// SANDBOX ENFORCEMENT TEST
// =============================================================================

// TestSandboxEnforcement configures the component with a sandbox_dir and
// publishes a ToolCall that attempts to escape via sandbox_dir override using
// a ".." path. The component must reject the path and publish an error result.
func TestSandboxEnforcement(t *testing.T) {
	// Set up two directories: the sandbox root and a sibling outside it.
	sandboxDir := t.TempDir()
	outsideDir := t.TempDir()

	// Write a file outside the sandbox that we will try to read.
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("should not be readable"), 0o644); err != nil {
		t.Fatalf("create outside file: %v", err)
	}

	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ensureAgentStream(t, tc.Client)
	ctx := context.Background()

	// Configure component with a sandbox_dir.
	cfg := DefaultConfig()
	cfg.Org = "test"
	cfg.Platform = "integration"
	cfg.Board = "sandbox"
	cfg.SandboxDir = sandboxDir
	cfg.ConsumerNameSuffix = "sandbox"
	cfg.DeleteConsumerOnStop = true

	deps := component.Dependencies{NATSClient: tc.Client}
	comp, err := NewFromConfig(cfg, deps)
	if err != nil {
		t.Fatalf("NewFromConfig: %v", err)
	}
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer comp.Stop(5 * time.Second)

	callID := "call-sandbox-escape-001"
	call := agentic.ToolCall{
		ID:   callID,
		Name: "read_file",
		Arguments: map[string]any{
			"path": outsideFile,
		},
		Metadata: map[string]any{
			"agent_id":   "escape-agent",
			"trust_tier": journeymanTier,
			"skills":     []any{string(domain.SkillCodeGen)},
			"quest_id":   "quest-sandbox-001",
			// Attempt to override sandbox to the outside directory.
			// buildContextFromMetadata will reject this because it escapes
			// the component-level sandbox.
			"sandbox_dir": outsideDir,
		},
	}

	publishToolCall(t, tc.Client, ctx, "tool.execute.read_file", call)

	result := pollForToolResult(t, tc.Client, ctx, callID, 10*time.Second)
	// The result must indicate an error: either the sandbox_dir override was
	// rejected (so the component sandbox is used, and outsideFile is not within
	// sandboxDir), or the path validation itself fails.
	if result.Error == "" {
		t.Error("expected sandbox enforcement error, file outside sandbox should not be readable")
	}
}

// =============================================================================
// CORRELATION ID PROPAGATION TEST
// =============================================================================

// TestCorrelationIDPropagation verifies that LoopID and TraceID from the
// inbound ToolCall are echoed back in the ToolResult unchanged.
func TestCorrelationIDPropagation(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "corr.txt")
	if err := os.WriteFile(tmpFile, []byte("correlation test"), 0o644); err != nil {
		t.Fatalf("create temp file: %v", err)
	}

	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ensureAgentStream(t, tc.Client)
	ctx := context.Background()

	comp := setupComponent(t, tc.Client, "correlation")
	defer comp.Stop(5 * time.Second)

	callID := "call-corr-001"
	wantLoopID := "loop-corr-xyz-789"
	wantTraceID := "trace-corr-abc-123"

	call := agentic.ToolCall{
		ID:      callID,
		Name:    "read_file",
		LoopID:  wantLoopID,
		TraceID: wantTraceID,
		Arguments: map[string]any{
			"path": tmpFile,
		},
		Metadata: map[string]any{
			"agent_id":   "corr-agent",
			"trust_tier": journeymanTier,
			"skills":     []any{string(domain.SkillResearch)},
			"quest_id":   "quest-corr-001",
		},
	}

	publishToolCall(t, tc.Client, ctx, "tool.execute.read_file", call)

	result := pollForToolResult(t, tc.Client, ctx, callID, 10*time.Second)
	if result.LoopID != wantLoopID {
		t.Errorf("LoopID = %q, want %q", result.LoopID, wantLoopID)
	}
	if result.TraceID != wantTraceID {
		t.Errorf("TraceID = %q, want %q", result.TraceID, wantTraceID)
	}
	if result.CallID != callID {
		t.Errorf("CallID = %q, want %q", result.CallID, callID)
	}
}

// =============================================================================
// UNKNOWN TOOL TEST
// =============================================================================

// TestUnknownTool verifies that a ToolCall for an unregistered tool name
// produces a ToolResult with an "unknown tool" error rather than silently failing.
func TestUnknownTool(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ensureAgentStream(t, tc.Client)
	ctx := context.Background()

	comp := setupComponent(t, tc.Client, "unknown-tool")
	defer comp.Stop(5 * time.Second)

	callID := "call-unknown-001"
	call := agentic.ToolCall{
		ID:   callID,
		Name: "definitely_not_a_registered_tool",
		Metadata: map[string]any{
			"agent_id":   "tool-explorer",
			"trust_tier": expertTier,
			"skills":     []any{string(domain.SkillCodeGen)},
		},
	}

	publishToolCall(t, tc.Client, ctx, "tool.execute.definitely_not_a_registered_tool", call)

	result := pollForToolResult(t, tc.Client, ctx, callID, 10*time.Second)
	if result.Error == "" {
		t.Error("expected error for unknown tool, got empty error field")
	}
	if !strings.Contains(strings.ToLower(result.Error), "unknown") {
		t.Errorf("error %q should mention unknown tool", result.Error)
	}
}

// =============================================================================
// NO-METADATA DEFAULTS TEST
// =============================================================================

// TestNoMetadataDefaultsToApprentice verifies that a ToolCall with no Metadata
// defaults to TierApprentice. read_file requires TierJourneyman, so the call
// must be rejected with an insufficient-tier error.
func TestNoMetadataDefaultsToApprentice(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV())
	ensureAgentStream(t, tc.Client)
	ctx := context.Background()

	comp := setupComponent(t, tc.Client, "no-metadata")
	defer comp.Stop(5 * time.Second)

	callID := "call-nometa-001"
	call := agentic.ToolCall{
		ID:   callID,
		Name: "read_file",
		Arguments: map[string]any{
			"path": "/tmp/anything.txt",
		},
		// Intentionally omit Metadata — handler defaults to TierApprentice.
	}

	publishToolCall(t, tc.Client, ctx, "tool.execute.read_file", call)

	result := pollForToolResult(t, tc.Client, ctx, callID, 10*time.Second)
	if result.Error == "" {
		t.Error("expected tier rejection for nil-metadata call, got empty error")
	}
	if !strings.Contains(strings.ToLower(result.Error), "tier") &&
		!strings.Contains(strings.ToLower(result.Error), "insufficient") {
		t.Errorf("error %q should mention tier requirement", result.Error)
	}
}

// =============================================================================
// HELPERS
// =============================================================================

// setupComponent creates, initialises, and starts a Component for integration
// testing. The suffix parameter is appended to the consumer name to prevent
// durable-consumer conflicts when tests run in parallel.
func setupComponent(t *testing.T, client *natsclient.Client, suffix string) *Component {
	t.Helper()

	cfg := DefaultConfig()
	cfg.Org = "test"
	cfg.Platform = "integration"
	cfg.Board = suffix
	cfg.ConsumerNameSuffix = suffix
	cfg.DeleteConsumerOnStop = true

	deps := component.Dependencies{NATSClient: client}

	comp, err := NewFromConfig(cfg, deps)
	if err != nil {
		t.Fatalf("NewFromConfig(%q): %v", suffix, err)
	}

	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize(%q): %v", suffix, err)
	}

	ctx := context.Background()
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start(%q): %v", suffix, err)
	}

	return comp
}

// ensureAgentStream creates the AGENT JetStream stream if it does not already
// exist. The stream covers all tool.execute.* and tool.result.> subjects the
// component reads from and writes to, plus the surrounding agent.* subjects
// required by other components sharing the stream.
func ensureAgentStream(t *testing.T, client *natsclient.Client) {
	t.Helper()

	js, err := client.JetStream()
	if err != nil {
		t.Fatalf("JetStream(): %v", err)
	}

	ctx := context.Background()
	streamCfg := jetstream.StreamConfig{
		Name:     agentStreamName,
		Subjects: agentStreamSubjects,
	}

	if _, err := js.CreateOrUpdateStream(ctx, streamCfg); err != nil {
		t.Fatalf("CreateOrUpdateStream(AGENT): %v", err)
	}
}

// publishToolCall serialises a ToolCall to JSON and publishes it to the
// given subject on the AGENT JetStream stream.
func publishToolCall(t *testing.T, client *natsclient.Client, ctx context.Context, subject string, call agentic.ToolCall) {
	t.Helper()

	data, err := json.Marshal(call)
	if err != nil {
		t.Fatalf("marshal ToolCall: %v", err)
	}

	if err := client.PublishToStream(ctx, subject, data); err != nil {
		t.Fatalf("PublishToStream(%q): %v", subject, err)
	}
}

// pollForToolResult subscribes to tool.result.{callID} via an ephemeral
// JetStream consumer and blocks until a ToolResult message arrives or timeout
// elapses. It returns the decoded ToolResult.
//
// DeliverAllPolicy is used so the consumer picks up messages that were
// published to the stream before this function is called. Each test uses a
// unique callID so replaying unrelated historical messages does not cause
// cross-test contamination.
func pollForToolResult(t *testing.T, client *natsclient.Client, ctx context.Context, callID string, timeout time.Duration) agentic.ToolResult {
	t.Helper()

	js, err := client.JetStream()
	if err != nil {
		t.Fatalf("JetStream(): %v", err)
	}

	subject := fmt.Sprintf("tool.result.%s", callID)

	// Create an ephemeral (no Durable) consumer filtered to the specific result
	// subject. DeliverAllPolicy ensures we do not miss a result published before
	// this consumer is created (avoids publish-before-subscribe race).
	consumer, err := js.CreateOrUpdateConsumer(ctx, agentStreamName, jetstream.ConsumerConfig{
		FilterSubject: subject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		DeliverPolicy: jetstream.DeliverAllPolicy,
	})
	if err != nil {
		t.Fatalf("CreateOrUpdateConsumer for %q: %v", subject, err)
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		msgs, fetchErr := consumer.Fetch(1, jetstream.FetchMaxWait(200*time.Millisecond))
		if fetchErr != nil {
			// A fetch timeout is expected on every polling iteration where the
			// component has not yet published a result. Continue polling.
			continue
		}

		for msg := range msgs.Messages() {
			if ackErr := msg.Ack(); ackErr != nil {
				t.Logf("ack warning for %q: %v", subject, ackErr)
			}

			var result agentic.ToolResult
			if err := json.Unmarshal(msg.Data(), &result); err != nil {
				t.Fatalf("unmarshal ToolResult: %v", err)
			}
			return result
		}
	}

	t.Fatalf("timed out waiting for ToolResult on %q after %v", subject, timeout)
	return agentic.ToolResult{} // unreachable; satisfies compiler
}
