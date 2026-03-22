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
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/executor"
	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
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
	tc := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage())
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

// TestToolExecutionSuccess publishes a bash ToolCall with a Journeyman agent.
// The built-in bash handler runs the command in the sandbox directory; the
// test verifies a non-error ToolResult containing the file contents.
func TestToolExecutionSuccess(t *testing.T) {
	// Create a temporary sandbox directory with a file for bash to read.
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "hello.txt"), []byte("hello from questtools"), 0o644); err != nil {
		t.Fatalf("create temp file: %v", err)
	}

	tc := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage())
	ensureAgentStream(t, tc.Client)
	ctx := context.Background()

	comp := setupComponent(t, tc.Client, "exec-success")
	defer comp.Stop(5 * time.Second)

	callID := "call-success-001"
	call := agentic.ToolCall{
		ID:   callID,
		Name: "bash",
		Arguments: map[string]any{
			"command": "cat hello.txt",
		},
		Metadata: map[string]any{
			"agent_id":    "test-agent-success",
			"trust_tier":  journeymanTier,
			"skills":      []any{string(domain.SkillCodeGen)},
			"quest_id":    "quest-success-001",
			"sandbox_dir": tmpDir, // bash handler requires a sandbox directory
		},
	}

	publishToolCall(t, tc.Client, ctx, "tool.execute.bash", call)

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

// TestToolExecutionTierRejection publishes a bash ToolCall with an Apprentice
// agent (tier 0). bash requires Journeyman (tier 1). The component must
// publish a ToolResult with a tier/authorization error.
func TestToolExecutionTierRejection(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage())
	ensureAgentStream(t, tc.Client)
	ctx := context.Background()

	comp := setupComponent(t, tc.Client, "tier-reject")
	defer comp.Stop(5 * time.Second)

	callID := "call-tier-reject-001"
	call := agentic.ToolCall{
		ID:   callID,
		Name: "bash",
		Arguments: map[string]any{
			"command": "echo should-not-run",
		},
		Metadata: map[string]any{
			"agent_id":   "apprentice-agent",
			"trust_tier": apprenticeTier, // Tier 0 — below Journeyman (tier 1) required by bash.
			"skills":     []any{string(domain.SkillCodeGen)},
			"quest_id":   "quest-tier-reject-001",
		},
	}

	publishToolCall(t, tc.Client, ctx, "tool.execute.bash", call)

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

// TestToolExecutionSkillRejection registers a custom tool that requires
// code_generation skill (Journeyman tier) and publishes a ToolCall from a
// Journeyman agent that only has the planning skill. The component must publish
// a ToolResult with a skill-gate error.
//
// bash has no skill requirement, so we register a test-only tool to verify
// the skill gate path in the executor.
func TestToolExecutionSkillRejection(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage())
	ensureAgentStream(t, tc.Client)
	ctx := context.Background()

	comp := setupComponent(t, tc.Client, "skill-reject")
	defer comp.Stop(5 * time.Second)

	// Register a test tool that requires code_generation skill at Journeyman tier.
	// This verifies the skill gate independently of any built-in tool.
	comp.ToolRegistry().Register(executor.RegisteredTool{
		Definition: agentic.ToolDefinition{
			Name:        "skill_gated_test_tool",
			Description: "Test-only tool requiring code_generation skill",
			Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		},
		Handler: func(_ context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
			return agentic.ToolResult{CallID: call.ID, Content: "executed"}
		},
		MinTier: domain.TierJourneyman,
		Skills:  []domain.SkillTag{domain.SkillCodeGen},
	})

	callID := "call-skill-reject-001"
	call := agentic.ToolCall{
		ID:   callID,
		Name: "skill_gated_test_tool",
		Metadata: map[string]any{
			"agent_id": "no-skill-agent",
			// Journeyman tier satisfies the tier gate, but no matching skills.
			"trust_tier": journeymanTier,
			// planning is NOT in skill_gated_test_tool's required skills [code_generation].
			"skills":   []any{string(domain.SkillPlanning)},
			"quest_id": "quest-skill-reject-001",
		},
	}

	publishToolCall(t, tc.Client, ctx, "tool.execute.skill_gated_test_tool", call)

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
	tc := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage())
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
	tc := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage())
	ensureAgentStream(t, tc.Client)
	ctx := context.Background()

	comp := setupComponent(t, tc.Client, "validation")
	defer comp.Stop(5 * time.Second)

	callID := "call-validate-001"
	// ToolCall with a valid ID but empty Name. BaseMessage.MarshalJSON validates
	// the payload, so we cannot use publishToolCall (it would fail at marshal time).
	// Instead, craft the BaseMessage wire format manually so the handler receives
	// a structurally-valid envelope wrapping an invalid ToolCall.
	// The handler will unmarshal it successfully, then call Validate() and return
	// an error ToolResult to tool.result.call-validate-001.
	rawCall := agentic.ToolCall{
		ID:   callID,
		Name: "", // Empty name — handler's Validate() must reject this.
		Metadata: map[string]any{
			"agent_id":   "test-agent",
			"trust_tier": journeymanTier,
		},
	}
	publishInvalidToolCall(t, tc.Client, ctx, "tool.execute.unknown", rawCall)

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
	if err := os.WriteFile(filepath.Join(tmpDir, "context.txt"), []byte("context verified"), 0o644); err != nil {
		t.Fatalf("create temp file: %v", err)
	}

	tc := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage())
	ensureAgentStream(t, tc.Client)
	ctx := context.Background()

	comp := setupComponent(t, tc.Client, "meta-reconstruct")
	defer comp.Stop(5 * time.Second)

	callID := "call-meta-001"
	call := agentic.ToolCall{
		ID:     callID,
		Name:   "bash",
		LoopID: "loop-meta-abc",
		Arguments: map[string]any{
			"command": "ls -la",
		},
		Metadata: map[string]any{
			"agent_id":    "meta-agent-42",
			"trust_tier":  journeymanTier,
			"skills":      []any{string(domain.SkillAnalysis)},
			"quest_id":    "quest-meta-001",
			"sandbox_dir": tmpDir, // bash handler requires a sandbox directory
		},
	}

	publishToolCall(t, tc.Client, ctx, "tool.execute.bash", call)

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
// publishes a bash ToolCall that attempts to escape via sandbox_dir override.
// buildContextFromMetadata rejects the override, so the bash command runs in
// the component sandbox where the file does not exist, producing an error.
func TestSandboxEnforcement(t *testing.T) {
	// Set up two directories: the sandbox root and a sibling outside it.
	sandboxDir := t.TempDir()
	outsideDir := t.TempDir()

	// Write a file named "secret.txt" only in the outside directory.
	// The bash command uses a relative path, so it can only find the file if
	// the escape succeeds and the command runs in outsideDir.
	if err := os.WriteFile(filepath.Join(outsideDir, "secret.txt"), []byte("should not be readable"), 0o644); err != nil {
		t.Fatalf("create outside file: %v", err)
	}

	tc := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage())
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
		Name: "bash",
		Arguments: map[string]any{
			// Relative path — only resolvable if the working dir is outsideDir.
			// If the sandbox_dir override is rejected, the command runs in
			// sandboxDir where secret.txt does not exist and returns an error.
			"command": "cat secret.txt",
		},
		Metadata: map[string]any{
			"agent_id":   "escape-agent",
			"trust_tier": journeymanTier,
			"skills":     []any{string(domain.SkillCodeGen)},
			"quest_id":   "quest-sandbox-001",
			// Attempt to override sandbox to the outside directory.
			// buildContextFromMetadata rejects this because outsideDir does
			// not fall within the component-level sandboxDir.
			"sandbox_dir": outsideDir,
		},
	}

	publishToolCall(t, tc.Client, ctx, "tool.execute.bash", call)

	result := pollForToolResult(t, tc.Client, ctx, callID, 10*time.Second)
	// If the escape is blocked (expected): command runs in sandboxDir where
	// secret.txt does not exist → bash exits non-zero → result.Error non-empty.
	// If the escape is NOT blocked (bug): command runs in outsideDir, file is
	// readable → result.Error is empty (wrong).
	if result.Error == "" {
		t.Error("expected sandbox enforcement error: secret.txt should not be found in the component sandbox")
	}
}

// =============================================================================
// CORRELATION ID PROPAGATION TEST
// =============================================================================

// TestCorrelationIDPropagation verifies that LoopID and TraceID from the
// inbound ToolCall are echoed back in the ToolResult unchanged.
func TestCorrelationIDPropagation(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "corr.txt"), []byte("correlation test"), 0o644); err != nil {
		t.Fatalf("create temp file: %v", err)
	}

	tc := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage())
	ensureAgentStream(t, tc.Client)
	ctx := context.Background()

	comp := setupComponent(t, tc.Client, "correlation")
	defer comp.Stop(5 * time.Second)

	callID := "call-corr-001"
	wantLoopID := "loop-corr-xyz-789"
	wantTraceID := "trace-corr-abc-123"

	call := agentic.ToolCall{
		ID:      callID,
		Name:    "bash",
		LoopID:  wantLoopID,
		TraceID: wantTraceID,
		Arguments: map[string]any{
			"command": "cat corr.txt",
		},
		Metadata: map[string]any{
			"agent_id":    "corr-agent",
			"trust_tier":  journeymanTier,
			"skills":      []any{string(domain.SkillResearch)},
			"quest_id":    "quest-corr-001",
			"sandbox_dir": tmpDir, // bash handler requires a sandbox directory
		},
	}

	publishToolCall(t, tc.Client, ctx, "tool.execute.bash", call)

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
	tc := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage())
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
// defaults to TierApprentice. bash requires TierJourneyman, so the call
// must be rejected with an insufficient-tier error.
func TestNoMetadataDefaultsToApprentice(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage())
	ensureAgentStream(t, tc.Client)
	ctx := context.Background()

	comp := setupComponent(t, tc.Client, "no-metadata")
	defer comp.Stop(5 * time.Second)

	callID := "call-nometa-001"
	call := agentic.ToolCall{
		ID:   callID,
		Name: "bash",
		Arguments: map[string]any{
			"command": "echo should-not-run",
		},
		// Intentionally omit Metadata — handler defaults to TierApprentice.
		// bash requires TierJourneyman, so this should be rejected.
	}

	publishToolCall(t, tc.Client, ctx, "tool.execute.bash", call)

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

// publishToolCall wraps a ToolCall in a BaseMessage envelope (required by the
// questtools handler) and publishes it to the given subject on the AGENT stream.
func publishToolCall(t *testing.T, client *natsclient.Client, ctx context.Context, subject string, call agentic.ToolCall) {
	t.Helper()

	baseMsg := message.NewBaseMessage(call.Schema(), &call, "test")
	data, err := json.Marshal(baseMsg)
	if err != nil {
		t.Fatalf("marshal ToolCall BaseMessage: %v", err)
	}

	if err := client.PublishToStream(ctx, subject, data); err != nil {
		t.Fatalf("PublishToStream(%q): %v", subject, err)
	}
}

// publishInvalidToolCall crafts a BaseMessage wire-format JSON containing a
// ToolCall that fails Validate() (e.g. empty Name). BaseMessage.MarshalJSON
// rejects invalid payloads, so this helper bypasses that by constructing the
// envelope JSON directly. The handler will unmarshal it, call Validate(), and
// publish an error ToolResult back to tool.result.{call.ID}.
func publishInvalidToolCall(t *testing.T, client *natsclient.Client, ctx context.Context, subject string, call agentic.ToolCall) {
	t.Helper()

	payloadBytes, err := json.Marshal(call)
	if err != nil {
		t.Fatalf("marshal invalid ToolCall payload: %v", err)
	}

	// Construct the BaseMessage wire format by hand, matching the structure that
	// message.BaseMessage.MarshalJSON produces.
	wire := map[string]any{
		"id": "invalid-call-envelope",
		"type": map[string]any{
			"domain":   agentic.Domain,
			"category": agentic.CategoryToolCall,
			"version":  agentic.SchemaVersion,
		},
		"payload": json.RawMessage(payloadBytes),
		"meta": map[string]any{
			"created_at":  0,
			"received_at": 0,
			"source":      "test",
		},
	}

	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("marshal BaseMessage wire format: %v", err)
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

			// The component wraps ToolResult in a BaseMessage envelope.
			var baseMsg message.BaseMessage
			if err := json.Unmarshal(msg.Data(), &baseMsg); err != nil {
				t.Fatalf("unmarshal ToolResult BaseMessage: %v", err)
			}
			resultPtr, ok := baseMsg.Payload().(*agentic.ToolResult)
			if !ok {
				t.Fatalf("unexpected payload type in tool result: %T", baseMsg.Payload())
			}
			return *resultPtr
		}
	}

	t.Fatalf("timed out waiting for ToolResult on %q after %v", subject, timeout)
	return agentic.ToolResult{} // unreachable; satisfies compiler
}
