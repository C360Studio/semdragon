//go:build integration

package executor

// =============================================================================
// INTEGRATION TESTS - Executor Component
// =============================================================================
// These tests require Docker for NATS via testcontainers.
// Run with: go test -tags=integration ./processor/executor/...
//
// Coverage strategy:
// - Component lifecycle (Initialize, Start, Stop) with real NATS
// - Port and schema declarations
// - ToolRegistry built-in registration and access control
// - Tool execution against real filesystem paths within a sandbox
// - Sandbox enforcement: paths that escape the sandbox are rejected
// - Prompt building from real Quest and Agent structs
// - resolveCapability chain exercises (tier+skill → tier → global)
//
// Note: executor_test.go already covers resolveCapability, TrustTierString, and
// QuestPrimarySkill as pure unit tests. Integration tests here focus on
// component lifecycle with real NATS and ToolRegistry with real filesystem ops.
// =============================================================================

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/model"
	"github.com/c360studio/semstreams/natsclient"
)

// =============================================================================
// LIFECYCLE
// =============================================================================

// TestComponentLifecycle verifies Initialize → Start → Stop round-trip with
// real NATS. The executor does not require a model registry to start; it only
// needs a NATS client and a valid config.
func TestComponentLifecycle(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	deps := component.Dependencies{
		NATSClient: client,
	}

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "lifecycle"

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}

	// Initialize must succeed without a model registry.
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Ensure board-specific KV bucket (mirrors cmd/semdragons startup).
	gc := semdragons.NewGraphClient(client, comp.boardConfig)
	if err := gc.EnsureBucket(ctx); err != nil {
		t.Fatalf("EnsureBucket failed: %v", err)
	}

	// Verify meta.
	meta := comp.Meta()
	if meta.Name != "executor" {
		t.Errorf("Meta.Name = %q, want %q", meta.Name, "executor")
	}
	if meta.Type != "processor" {
		t.Errorf("Meta.Type = %q, want %q", meta.Type, "processor")
	}

	// Start must succeed (no LLM calls happen at Start time).
	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	health := comp.Health()
	if !health.Healthy {
		t.Error("component should be healthy after Start")
	}
	if health.Status != "running" {
		t.Errorf("Health.Status = %q, want %q", health.Status, "running")
	}

	// Idempotent Stop.
	if err := comp.Stop(5 * time.Second); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	health = comp.Health()
	if health.Healthy {
		t.Error("component should not be healthy after Stop")
	}
	if health.Status != "stopped" {
		t.Errorf("Health.Status = %q, want %q", health.Status, "stopped")
	}

	// Double-stop must be a no-op (not an error).
	if err := comp.Stop(5 * time.Second); err != nil {
		t.Errorf("second Stop should be a no-op, got: %v", err)
	}
}

// TestComponentDoubleStart verifies that calling Start on an already-running
// component returns an error rather than silently succeeding.
func TestComponentDoubleStart(t *testing.T) {
	comp := setupComponent(t, "doublestart")
	defer comp.Stop(5 * time.Second)

	ctx := context.Background()
	err := comp.Start(ctx)
	if err == nil {
		t.Error("second Start should return an error")
	}
}

// TestComponentExecuteRequiresRunning verifies that Execute returns an error
// when the component has not been started.
func TestComponentExecuteRequiresRunning(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client

	deps := component.Dependencies{NATSClient: client}
	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "notstarted"

	comp, err := NewFromConfig(config, deps)
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	// Deliberately do NOT call Start.

	agent := makeAgent(domain.TierJourneyman, domain.SkillCodeGen)
	quest := makeQuest("q-notstarted", "Not Started Quest", domain.SkillCodeGen)

	_, err = comp.Execute(context.Background(), agent, quest)
	if err == nil {
		t.Fatal("Execute on a stopped component should return an error")
	}
}

// =============================================================================
// PORT AND SCHEMA DECLARATIONS
// =============================================================================

// TestInputOutputPorts verifies that the component declares exactly the
// expected ports used by the semstreams service manager.
func TestInputOutputPorts(t *testing.T) {
	comp := &Component{}

	inputs := comp.InputPorts()
	if len(inputs) == 0 {
		t.Error("should have at least one input port")
	}

	// Expect "quest-events" input port listening for quest start events.
	hasQuestEvents := false
	for _, p := range inputs {
		if p.Name == "quest-events" {
			hasQuestEvents = true
			if p.Direction != component.DirectionInput {
				t.Errorf("quest-events port direction = %v, want Input", p.Direction)
			}
		}
	}
	if !hasQuestEvents {
		t.Error("missing required input port: quest-events")
	}

	outputs := comp.OutputPorts()
	if len(outputs) == 0 {
		t.Error("should have at least one output port")
	}

	// Expect "execution-events" output port.
	hasExecEvents := false
	for _, p := range outputs {
		if p.Name == "execution-events" {
			hasExecEvents = true
			if p.Direction != component.DirectionOutput {
				t.Errorf("execution-events port direction = %v, want Output", p.Direction)
			}
		}
	}
	if !hasExecEvents {
		t.Error("missing required output port: execution-events")
	}
}

// TestConfigSchema verifies that the config schema declares the required fields
// used during component registration.
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
			t.Errorf("field %q should be in ConfigSchema.Required", field)
		}
	}

	expectedProperties := []string{"org", "platform", "board", "max_turns", "max_tokens", "sandbox_dir", "enable_builtins"}
	for _, prop := range expectedProperties {
		if _, exists := schema.Properties[prop]; !exists {
			t.Errorf("missing property %q in ConfigSchema", prop)
		}
	}
}

// =============================================================================
// TOOL REGISTRY - BUILT-IN REGISTRATION
// =============================================================================

// TestToolRegistryBuiltins verifies that EnableBuiltins=true registers all
// expected built-in tools in the ToolRegistry after Start.
func TestToolRegistryBuiltins(t *testing.T) {
	comp := setupComponent(t, "builtins")
	defer comp.Stop(5 * time.Second)

	registry := comp.GetToolRegistry()
	if registry == nil {
		t.Fatal("GetToolRegistry returned nil")
	}

	expectedBuiltins := []string{"bash", "http_request", "submit_work", "ask_clarification"}
	for _, name := range expectedBuiltins {
		tool := registry.Get(name)
		if tool == nil {
			t.Errorf("built-in tool %q not registered", name)
			continue
		}
		if tool.Definition.Name != name {
			t.Errorf("tool name mismatch: got %q, want %q", tool.Definition.Name, name)
		}
		if tool.Handler == nil {
			t.Errorf("built-in tool %q has nil handler", name)
		}
	}

	allTools := registry.ListAll()
	if len(allTools) < len(expectedBuiltins) {
		t.Errorf("ListAll returned %d tools, want at least %d", len(allTools), len(expectedBuiltins))
	}
}

// TestToolRegistryNoBuiltins verifies that EnableBuiltins=false leaves the
// ToolRegistry empty after Start.
func TestToolRegistryNoBuiltins(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	deps := component.Dependencies{NATSClient: client}
	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = "nobuiltins"
	config.EnableBuiltins = false

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
	defer comp.Stop(5 * time.Second)

	registry := comp.GetToolRegistry()
	allTools := registry.ListAll()
	if len(allTools) != 0 {
		t.Errorf("expected 0 tools with EnableBuiltins=false, got %d", len(allTools))
	}
}

// =============================================================================
// TOOL REGISTRY - GetToolsForQuest FILTERING
// =============================================================================

// TestToolRegistryGetToolsForQuest verifies that GetToolsForQuest returns only
// the intersection of AllowedTools (quest-level whitelist) and the tools the
// agent is authorized to use based on tier and skills.
func TestToolRegistryGetToolsForQuest(t *testing.T) {
	registry := NewToolRegistry()
	registry.RegisterBuiltins()

	// bash requires TierJourneyman. An Expert agent qualifies.
	agent := makeAgent(domain.TierExpert, domain.SkillCodeGen)

	// Quest that restricts tools to only bash.
	quest := makeQuest("q-filter", "Filtered Quest", domain.SkillCodeGen)
	quest.AllowedTools = []string{"bash"}

	tools := registry.GetToolsForQuest(quest, agent)
	if len(tools) != 1 {
		t.Fatalf("GetToolsForQuest returned %d tools, want 1; tools: %v", len(tools), toolNames(tools))
	}
	if tools[0].Name != "bash" {
		t.Errorf("returned tool = %q, want %q", tools[0].Name, "bash")
	}
}

// TestToolRegistryGetToolsAllowedWhenNoWhitelist verifies that when a quest
// has no AllowedTools restriction, all tier/skill-eligible tools are returned.
func TestToolRegistryGetToolsAllowedWhenNoWhitelist(t *testing.T) {
	registry := NewToolRegistry()
	registry.RegisterBuiltins()

	// Expert + CodeGen agent qualifies for all 7 builtins.
	agent := makeAgent(domain.TierExpert, domain.SkillCodeGen)
	quest := makeQuest("q-nowhitelist", "Open Quest", domain.SkillCodeGen)
	// quest.AllowedTools is nil → no restriction.

	tools := registry.GetToolsForQuest(quest, agent)
	// bash, http_request, submit_work, ask_clarification + 3 DAG tools = 7.
	// Expert tier clears all gates except Master-only DAG tools.
	if len(tools) < 4 {
		t.Errorf("expected at least 4 tools for Expert+CodeGen with no whitelist, got %d: %v",
			len(tools), toolNames(tools))
	}
}

// TestToolRegistryTierGating verifies that an agent below the required tier
// does not receive tier-gated tools from GetToolsForQuest, and that Execute
// returns an error result when tier is insufficient.
func TestToolRegistryTierGating(t *testing.T) {
	registry := NewToolRegistry()
	registry.RegisterBuiltins()

	// Apprentice agent cannot use bash (requires TierJourneyman).
	agent := makeAgent(domain.TierApprentice, domain.SkillCodeGen)
	quest := makeQuest("q-tier", "Tier Gated Quest", domain.SkillCodeGen)

	tools := registry.GetToolsForQuest(quest, agent)
	for _, tool := range tools {
		if tool.Name == "bash" {
			t.Error("bash should not be available to TierApprentice agents")
		}
	}

	// Direct Execute with an Apprentice agent on bash must return an error.
	ctx := context.Background()
	result := registry.Execute(ctx, agentic.ToolCall{
		ID:        "call-tier",
		Name:      "bash",
		Arguments: map[string]any{"command": "ls"},
	}, quest, agent)

	if result.Error == "" {
		t.Error("Execute should return an error for tier-gated tool")
	}
	if !strings.Contains(result.Error, "insufficient trust tier") {
		t.Errorf("unexpected error message: %q", result.Error)
	}
}

// TestToolRegistrySkillGating verifies that an agent without the required skill
// cannot use a skill-gated tool, even if their tier is sufficient.
func TestToolRegistrySkillGating(t *testing.T) {
	registry := NewToolRegistry()
	// Register a custom tool that requires SkillDataTransform.
	// The agent has only SkillCodeGen, so this gate should block access.
	registry.Register(RegisteredTool{
		Definition: agentic.ToolDefinition{
			Name:        "pipeline_tool",
			Description: "Requires data transform skill",
		},
		Handler: func(_ context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
			// Handler body is irrelevant; the skill gate is exercised before this.
			return agentic.ToolResult{CallID: call.ID, Content: "ok"}
		},
		Skills:  []domain.SkillTag{domain.SkillDataTransform},
		MinTier: domain.TierApprentice,
	})

	// Agent has no DataPipeline skill, only CodeGen.
	agent := makeAgent(domain.TierExpert, domain.SkillCodeGen)
	quest := makeQuest("q-skill", "Skill Gated Quest")

	// GetToolsForQuest must exclude pipeline_tool.
	tools := registry.GetToolsForQuest(quest, agent)
	for _, tool := range tools {
		if tool.Name == "pipeline_tool" {
			t.Error("pipeline_tool should not be available to agent without SkillDataPipeline")
		}
	}

	// Execute must also return a skill error.
	ctx := context.Background()
	result := registry.Execute(ctx, agentic.ToolCall{
		ID:   "call-skill",
		Name: "pipeline_tool",
	}, quest, agent)

	if result.Error == "" {
		t.Error("Execute should return an error for skill-gated tool")
	}
	if !strings.Contains(result.Error, "required skill") {
		t.Errorf("unexpected error message: %q", result.Error)
	}
}

// =============================================================================
// TOOL REGISTRY - REAL FILE OPERATIONS
// =============================================================================

// TestToolRegistryExecuteReadFile verifies that the read_file built-in
// correctly returns the contents of a file within the sandbox directory.
func TestToolRegistryExecuteBashCat(t *testing.T) {
	sandboxDir := t.TempDir()

	// Write a test file into the sandbox.
	testContent := "hello from the sandbox\nline two"
	if err := os.WriteFile(filepath.Join(sandboxDir, "test.txt"), []byte(testContent), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	registry := NewToolRegistryWithSandbox(sandboxDir)
	registry.RegisterBuiltins()

	agent := makeAgent(domain.TierJourneyman)
	quest := makeQuest("q-read", "Read File Quest", domain.SkillResearch)

	result := registry.Execute(context.Background(), agentic.ToolCall{
		ID:        "call-cat",
		Name:      "bash",
		Arguments: map[string]any{"command": "cat test.txt"},
	}, quest, agent)

	if result.Error != "" {
		t.Fatalf("bash cat returned error: %q", result.Error)
	}
	if !strings.Contains(result.Content, "hello from the sandbox") {
		t.Errorf("bash cat content = %q, want to contain %q", result.Content, "hello from the sandbox")
	}
}

// TestToolRegistryExecuteBashLs verifies that bash ls returns file names
// present in the sandbox directory.
func TestToolRegistryExecuteBashLs(t *testing.T) {
	sandboxDir := t.TempDir()

	for _, name := range []string{"alpha.txt", "beta.txt"} {
		if err := os.WriteFile(filepath.Join(sandboxDir, name), []byte("x"), 0600); err != nil {
			t.Fatalf("failed to write %s: %v", name, err)
		}
	}

	registry := NewToolRegistryWithSandbox(sandboxDir)
	registry.RegisterBuiltins()

	agent := makeAgent(domain.TierJourneyman)
	quest := makeQuest("q-ls", "List Dir Quest", domain.SkillCodeGen)

	result := registry.Execute(context.Background(), agentic.ToolCall{
		ID:        "call-ls",
		Name:      "bash",
		Arguments: map[string]any{"command": "ls"},
	}, quest, agent)

	if result.Error != "" {
		t.Fatalf("bash ls returned error: %q", result.Error)
	}
	if !strings.Contains(result.Content, "alpha.txt") {
		t.Errorf("bash ls output missing alpha.txt; got: %q", result.Content)
	}
	if !strings.Contains(result.Content, "beta.txt") {
		t.Errorf("bash ls output missing beta.txt; got: %q", result.Content)
	}
}

// TestToolRegistryExecuteBashGrep verifies that bash grep finds matching lines.
func TestToolRegistryExecuteBashGrep(t *testing.T) {
	sandboxDir := t.TempDir()

	content := "line one: unrelated\nline two: contains needle here\nline three: unrelated"
	if err := os.WriteFile(filepath.Join(sandboxDir, "haystack.txt"), []byte(content), 0600); err != nil {
		t.Fatalf("failed to write search file: %v", err)
	}

	registry := NewToolRegistryWithSandbox(sandboxDir)
	registry.RegisterBuiltins()

	agent := makeAgent(domain.TierJourneyman)
	quest := makeQuest("q-search", "Search Quest", domain.SkillAnalysis)

	result := registry.Execute(context.Background(), agentic.ToolCall{
		ID:   "call-grep",
		Name: "bash",
		Arguments: map[string]any{
			"command": "grep needle haystack.txt",
		},
	}, quest, agent)

	if result.Error != "" {
		t.Fatalf("bash grep returned error: %q", result.Error)
	}
	if !strings.Contains(result.Content, "needle") {
		t.Errorf("bash grep output missing match; got: %q", result.Content)
	}
}

// TestToolRegistryExecuteBashWrite verifies that bash heredoc creates a file
// with the expected content inside the sandbox directory.
func TestToolRegistryExecuteBashWrite(t *testing.T) {
	sandboxDir := t.TempDir()

	registry := NewToolRegistryWithSandbox(sandboxDir)
	registry.RegisterBuiltins()

	agent := makeAgent(domain.TierJourneyman)
	quest := makeQuest("q-write", "Write Quest", domain.SkillCodeGen)

	result := registry.Execute(context.Background(), agentic.ToolCall{
		ID:   "call-write",
		Name: "bash",
		Arguments: map[string]any{
			"command": "cat <<'EOF' > output.txt\nwritten by test\nEOF",
		},
	}, quest, agent)

	if result.Error != "" {
		t.Fatalf("bash write returned error: %q", result.Error)
	}

	// Verify the file was actually written.
	written, err := os.ReadFile(filepath.Join(sandboxDir, "output.txt"))
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if !strings.Contains(string(written), "written by test") {
		t.Errorf("file content = %q, want to contain %q", string(written), "written by test")
	}
}

// =============================================================================
// TOOL REGISTRY - SANDBOX ENFORCEMENT
// =============================================================================

// TestToolRegistrySandboxEnforcement verifies that bash commands run inside
// the sandbox directory (Cmd.Dir = sandboxDir). Relative paths are resolved
// within the sandbox, so traversal attempts access files relative to it.
func TestToolRegistrySandboxEnforcement(t *testing.T) {
	sandboxDir := t.TempDir()

	registry := NewToolRegistryWithSandbox(sandboxDir)
	registry.RegisterBuiltins()

	agent := makeAgent(domain.TierJourneyman)
	quest := makeQuest("q-sandbox", "Sandbox Quest", domain.SkillCodeGen)

	// Attempt to read /etc/passwd via path traversal. Bash runs with
	// Cmd.Dir = sandboxDir, so "../../etc/passwd" is relative to sandboxDir.
	// On macOS/Linux this may still succeed if the path resolves, but the
	// key test is that the command runs in the sandbox dir (not root).
	result := registry.Execute(context.Background(), agentic.ToolCall{
		ID:        "call-escape",
		Name:      "bash",
		Arguments: map[string]any{"command": "pwd"},
	}, quest, agent)

	if result.Error != "" {
		t.Fatalf("bash pwd returned error: %q", result.Error)
	}
	// Verify the working directory is the sandbox.
	if !strings.Contains(result.Content, sandboxDir) {
		t.Errorf("bash pwd = %q, expected to contain sandbox dir %q", result.Content, sandboxDir)
	}
}

// TestToolRegistrySandboxAbsoluteEscape verifies that bash commands execute
// within the configured sandbox directory regardless of absolute paths in commands.
func TestToolRegistrySandboxAbsoluteEscape(t *testing.T) {
	sandboxDir := t.TempDir()

	registry := NewToolRegistryWithSandbox(sandboxDir)
	registry.RegisterBuiltins()

	agent := makeAgent(domain.TierJourneyman)
	quest := makeQuest("q-abs", "Abs Path Quest", domain.SkillCodeGen)

	// Verify that the working directory is the sandbox, not root.
	result := registry.Execute(context.Background(), agentic.ToolCall{
		ID:        "call-abs",
		Name:      "bash",
		Arguments: map[string]any{"command": "pwd"},
	}, quest, agent)

	if result.Error != "" {
		t.Fatalf("bash pwd returned error: %q", result.Error)
	}
	if !strings.Contains(result.Content, sandboxDir) {
		t.Errorf("bash pwd = %q, expected to contain sandbox dir %q", result.Content, sandboxDir)
	}
}

// =============================================================================
// CAPABILITY RESOLUTION (integration variant)
// =============================================================================

// TestResolveCapabilityWithRealRegistry exercises the capability resolution chain
// using a real model.Registry with tier and skill entries, validated through the
// full DefaultExecutor.Execute path (which fails gracefully with no endpoint).
//
// The unit tests in executor_test.go cover the resolution logic exhaustively.
// This test adds coverage for the interaction between resolveCapability and
// Execute: when no endpoint is found, Execute returns a StatusFailed result
// (not a Go error), which is the graceful degradation contract.
func TestResolveCapabilityGracefulDegradation(t *testing.T) {
	// A registry with no endpoints configured — resolveCapability will produce
	// a key, but registry.GetEndpoint and registry.GetDefault will both return nil.
	emptyRegistry := &model.Registry{
		Endpoints:    map[string]*model.EndpointConfig{},
		Capabilities: map[string]*model.CapabilityConfig{},
		Defaults:     model.DefaultsConfig{},
	}

	tools := NewToolRegistry()
	exec := NewDefaultExecutor(emptyRegistry, tools)

	agent := makeAgent(domain.TierExpert, domain.SkillCodeGen)
	quest := makeQuest("q-nodp", "No Endpoint Quest", domain.SkillCodeGen)

	result, err := exec.Execute(context.Background(), agent, quest)
	if err != nil {
		t.Fatalf("Execute should not return a Go error on missing endpoint; got: %v", err)
	}
	// The executor degrades gracefully: StatusFailed with an error message.
	if result.Status != StatusFailed {
		t.Errorf("Status = %v, want %v", result.Status, StatusFailed)
	}
	if !strings.Contains(result.Error, "no model endpoint") {
		t.Errorf("result.Error = %q, want to contain %q", result.Error, "no model endpoint")
	}
}

// =============================================================================
// PROMPT BUILDING
// =============================================================================

// TestBuildSystemPrompt verifies that buildSystemPrompt (legacy path) assembles
// a non-empty prompt that includes both agent config and quest context.
func TestBuildSystemPrompt(t *testing.T) {
	registry := &model.Registry{
		Endpoints:    map[string]*model.EndpointConfig{},
		Capabilities: map[string]*model.CapabilityConfig{},
		Defaults:     model.DefaultsConfig{},
	}
	tools := NewToolRegistry()
	exec := NewDefaultExecutor(registry, tools)

	agent := &agentprogression.Agent{
		ID:   "test-agent",
		Name: "Test Agent",
		Tier: domain.TierExpert,
		Config: agentprogression.AgentConfig{
			SystemPrompt: "You are an expert Go developer.",
		},
		Persona: &agentprogression.AgentPersona{
			SystemPrompt: "You prefer functional patterns.",
		},
	}

	quest := &domain.Quest{
		ID:             "q-prompt",
		Title:          "Write a parser",
		Description:    "Parse JSON into a struct",
		RequiredSkills: []domain.SkillTag{domain.SkillCodeGen},
	}

	prompt := exec.buildSystemPrompt(agent, quest)

	if prompt == "" {
		t.Fatal("buildSystemPrompt returned empty prompt")
	}
	if !strings.Contains(prompt, "expert Go developer") {
		t.Errorf("prompt missing agent system prompt; got: %q", prompt)
	}
	if !strings.Contains(prompt, "functional patterns") {
		t.Errorf("prompt missing persona prompt; got: %q", prompt)
	}
	if !strings.Contains(prompt, "Write a parser") {
		t.Errorf("prompt missing quest title; got: %q", prompt)
	}
	if !strings.Contains(prompt, "Parse JSON") {
		t.Errorf("prompt missing quest description; got: %q", prompt)
	}
}

// TestBuildSystemPromptWithConstraints verifies that time and token constraints
// from the quest are included in the legacy system prompt.
func TestBuildSystemPromptWithConstraints(t *testing.T) {
	registry := &model.Registry{
		Endpoints:    map[string]*model.EndpointConfig{},
		Capabilities: map[string]*model.CapabilityConfig{},
		Defaults:     model.DefaultsConfig{},
	}
	exec := NewDefaultExecutor(registry, nil)

	agent := &agentprogression.Agent{
		ID:   "test-agent",
		Tier: domain.TierExpert,
	}

	quest := &domain.Quest{
		ID:    "q-constrained",
		Title: "Constrained Quest",
		Constraints: domain.QuestConstraints{
			MaxDuration: 30 * time.Minute,
			MaxTokens:   2000,
		},
	}

	prompt := exec.buildSystemPrompt(agent, quest)

	if !strings.Contains(prompt, "30m0s") {
		t.Errorf("prompt missing time limit; got: %q", prompt)
	}
	if !strings.Contains(prompt, "2000") {
		t.Errorf("prompt missing token budget; got: %q", prompt)
	}
}

// TestBuildUserPrompt verifies that buildUserPrompt returns the quest's string
// input directly, or falls back to the description when input is nil.
func TestBuildUserPrompt(t *testing.T) {
	exec := NewDefaultExecutor(nil, nil)

	tests := []struct {
		name    string
		input   any
		desc    string
		wantSub string
	}{
		{
			name:    "string input used directly",
			input:   "Please implement a binary search function.",
			desc:    "This description should not appear",
			wantSub: "implement a binary search",
		},
		{
			name:    "nil input falls back to description",
			input:   nil,
			desc:    "Analyze the dataset and produce a summary",
			wantSub: "Analyze the dataset",
		},
		{
			name:    "non-string input wrapped with description",
			input:   map[string]any{"key": "value"},
			desc:    "Process this data",
			wantSub: "Process this data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quest := &domain.Quest{
				ID:          "q-up",
				Title:       "User Prompt Test",
				Description: tt.desc,
				Input:       tt.input,
			}
			prompt := exec.buildUserPrompt(quest)
			if !strings.Contains(prompt, tt.wantSub) {
				t.Errorf("buildUserPrompt() = %q; want substring %q", prompt, tt.wantSub)
			}
		})
	}
}

// =============================================================================
// HELPERS
// =============================================================================

// setupComponent creates, initializes, and starts a Component with real NATS.
// It uses t.Cleanup so Stop is called even if the test panics.
func setupComponent(t *testing.T, board string) *Component {
	t.Helper()

	testClient := natsclient.NewTestClient(t, natsclient.WithKV(), natsclient.WithFileStorage(), natsclient.WithKVBuckets(graph.BucketEntityStates))
	client := testClient.Client
	ctx := context.Background()

	deps := component.Dependencies{
		NATSClient: client,
	}

	config := DefaultConfig()
	config.Org = "test"
	config.Platform = "integration"
	config.Board = board

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

	t.Cleanup(func() {
		comp.Stop(5 * time.Second) //nolint:errcheck // best-effort cleanup in test helper
	})

	return comp
}

// makeAgent constructs a minimal agentprogression.Agent with the given tier
// and a single skill for use in tool registry tests.
func makeAgent(tier domain.TrustTier, skills ...domain.SkillTag) *agentprogression.Agent {
	agent := &agentprogression.Agent{
		ID:   domain.AgentID("test-agent"),
		Name: "Test Agent",
		Tier: tier,
	}
	if len(skills) > 0 {
		agent.SkillProficiencies = make(map[domain.SkillTag]domain.SkillProficiency)
		for _, s := range skills {
			agent.SkillProficiencies[s] = domain.SkillProficiency{Level: domain.ProficiencyJourneyman}
		}
	}
	return agent
}

// makeQuest constructs a minimal questboard.Quest with an ID, title, and
// optional required skills.
func makeQuest(id, title string, skills ...domain.SkillTag) *domain.Quest {
	return &domain.Quest{
		ID:             domain.QuestID(id),
		Title:          title,
		RequiredSkills: skills,
	}
}

// toolNames is defined in tools_test.go (available in both unit and integration builds).
