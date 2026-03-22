package questtools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go/jetstream"
)

// startConsumer registers a durable JetStream consumer on tool.execute.* via the
// natsclient helper, which handles consumer lifecycle and context propagation.
func (c *Component) startConsumer(ctx context.Context) error {
	consumerName := "questtools-execute"
	if c.config.ConsumerNameSuffix != "" {
		consumerName += "-" + c.config.ConsumerNameSuffix
	}

	return c.deps.NATSClient.ConsumeStreamWithConfig(ctx, natsclient.StreamConsumerConfig{
		StreamName:    c.config.StreamName,
		ConsumerName:  consumerName,
		FilterSubject: "tool.execute.*",
		DeliverPolicy: "all",
		AckPolicy:     "explicit",
	}, c.handleToolExecute)
}

// handleToolExecute processes a single tool.execute.* message.
// It is called by the natsclient consumer loop with a derived context.
// The handler always acks the message (even on error) to prevent redelivery loops;
// error results are published back as ToolResult.Error responses so the caller can react.
func (c *Component) handleToolExecute(ctx context.Context, msg jetstream.Msg) {
	defer func() { _ = msg.Ack() }()

	// Unwrap the BaseMessage envelope that agentic-loop wraps around ToolCalls.
	var baseMsg message.BaseMessage
	if err := json.Unmarshal(msg.Data(), &baseMsg); err != nil {
		c.logger.Error("failed to unmarshal ToolCall BaseMessage",
			"subject", msg.Subject(),
			"error", err)
		c.errorsCount.Add(1)
		if parts := strings.SplitN(msg.Subject(), ".", 3); len(parts) == 3 {
			errMsg := fmt.Sprintf("failed to unmarshal ToolCall: %v", err)
			_ = c.publishResult(ctx, parts[2], &agentic.ToolResult{
				CallID:  parts[2],
				Content: "Tool error: " + errMsg,
				Error:   errMsg,
			})
		}
		return
	}

	callPtr, ok := baseMsg.Payload().(*agentic.ToolCall)
	if !ok {
		c.logger.Error("unexpected payload type in tool execute message",
			"subject", msg.Subject(),
			"type", fmt.Sprintf("%T", baseMsg.Payload()))
		c.errorsCount.Add(1)
		return
	}
	call := *callPtr

	if err := call.Validate(); err != nil {
		c.logger.Error("invalid ToolCall", "subject", msg.Subject(), "error", err)
		c.errorsCount.Add(1)
		if call.ID != "" {
			errMsg := fmt.Sprintf("invalid tool call: %v", err)
			_ = c.publishResult(ctx, call.ID, &agentic.ToolResult{
				CallID:  call.ID,
				Content: "Tool error: " + errMsg,
				Error:   errMsg,
			})
		}
		return
	}

	// Reconstruct enough agent/quest context from call metadata for gate checks.
	agent, quest := c.buildContextFromMetadata(&call)

	c.logger.Debug("executing tool",
		"tool", call.Name, "call_id", call.ID,
		"loop_id", call.LoopID, "agent_id", agent.ID,
		"quest_id", quest.ID, "tier", agent.Tier,
		"arguments", call.Arguments)

	// Execute the tool through the registry, which enforces tier and skill gates.
	result := c.toolRegistry.Execute(ctx, call, quest, agent)

	// Ensure Content is non-empty. The agentic-loop converts ToolResult.Content
	// into the ChatMessage.Content for role=tool messages. Gemini (and other
	// providers) reject tool result messages with empty content.
	if result.Content == "" && result.Error != "" {
		result.Content = fmt.Sprintf("Tool error: %s", addToolHint(call.Name, result.Error))
	} else if result.Content == "" {
		// SWE-agent insight: explicit feedback on empty output prevents agents
		// from re-running commands or assuming failure.
		result.Content = "(no output)"
	}

	// Classify bash commands for trajectory analytics. Since we consolidated
	// specialized tools (run_tests, lint_check, etc.) into bash, tag the result
	// metadata with what the command was actually doing.
	if call.Name == "bash" {
		if intent := classifyBashCommand(call); intent != "" {
			if result.Metadata == nil {
				result.Metadata = make(map[string]any)
			}
			result.Metadata["bash_intent"] = intent
		}
	}

	// Propagate correlation identifiers so the loop can match this result.
	result.LoopID = call.LoopID
	result.TraceID = call.TraceID

	// Publish the result to tool.result.{callID} on the same AGENT stream.
	if err := c.publishResult(ctx, call.ID, &result); err != nil {
		c.logger.Error("failed to publish ToolResult",
			"call_id", call.ID,
			"error", err)
		c.errorsCount.Add(1)
		return
	}

	if result.Error != "" {
		c.toolsFailed.Add(1)
	} else {
		c.toolsExecuted.Add(1)
	}
	c.lastActivity.Store(time.Now())

	c.logger.Debug("tool completed",
		"tool", call.Name, "call_id", call.ID,
		"loop_id", call.LoopID, "agent_id", agent.ID,
		"quest_id", quest.ID, "success", result.Error == "",
		"error", result.Error)
}

// publishResult wraps a ToolResult in a BaseMessage envelope and publishes it
// to tool.result.{callID}. The agentic-loop consumer expects BaseMessage wrapping.
func (c *Component) publishResult(ctx context.Context, callID string, result *agentic.ToolResult) error {
	subject := fmt.Sprintf("tool.result.%s", callID)

	baseMsg := message.NewBaseMessage(result.Schema(), result, "questtools")
	data, err := json.Marshal(baseMsg)
	if err != nil {
		return fmt.Errorf("marshal ToolResult BaseMessage: %w", err)
	}

	if err := c.deps.NATSClient.PublishToStream(ctx, subject, data); err != nil {
		return fmt.Errorf("publish to %s: %w", subject, err)
	}
	return nil
}

// buildContextFromMetadata constructs lightweight Agent and Quest stubs from the
// metadata embedded in a ToolCall.  These stubs carry only the fields that
// ToolRegistry.Execute needs for its tier/skill gate checks; we intentionally
// avoid a full KV round-trip on the hot path.
//
// Metadata keys (all optional):
//
//	"agent_id"    – string  → Agent.ID
//	"trust_tier"  – float64 or int → Agent.Tier
//	"skills"      – []any of string → Agent.SkillProficiencies (level 1 each)
//	"quest_id"    – string  → Quest.ID
//	"sandbox_dir" – string  → overrides the component-level sandbox directory
func (c *Component) buildContextFromMetadata(call *agentic.ToolCall) (*agentprogression.Agent, *domain.Quest) {
	agent := &agentprogression.Agent{
		// Default to the most-restricted tier so unidentified callers cannot
		// accidentally exercise higher-privilege tools.
		Tier: domain.TierApprentice,
	}
	quest := &domain.Quest{}

	if call.Metadata == nil {
		return agent, quest
	}

	if id, ok := call.Metadata["agent_id"].(string); ok {
		agent.ID = domain.AgentID(id)
	}

	if tier, ok := call.Metadata["trust_tier"]; ok {
		switch v := tier.(type) {
		case float64:
			t := domain.TrustTier(int(v))
			if t >= domain.TierApprentice && t <= domain.TierGrandmaster {
				agent.Tier = t
			} else {
				c.logger.Warn("invalid trust_tier in metadata, defaulting to Apprentice",
					"claimed_tier", v, "call_id", call.ID)
			}
		case int:
			t := domain.TrustTier(v)
			if t >= domain.TierApprentice && t <= domain.TierGrandmaster {
				agent.Tier = t
			} else {
				c.logger.Warn("invalid trust_tier in metadata, defaulting to Apprentice",
					"claimed_tier", v, "call_id", call.ID)
			}
		}
	}

	if skills, ok := call.Metadata["skills"].([]any); ok {
		agent.SkillProficiencies = make(map[domain.SkillTag]domain.SkillProficiency, len(skills))
		for _, s := range skills {
			if name, ok := s.(string); ok {
				agent.SkillProficiencies[domain.SkillTag(name)] = domain.SkillProficiency{Level: 1}
			}
		}
	}

	if id, ok := call.Metadata["quest_id"].(string); ok {
		quest.ID = domain.QuestID(id)
	}

	// Per-call sandbox: inject directly into arguments so ToolRegistry.Execute reads it.
	// This avoids mutating the shared ToolRegistry state (race condition).
	sandboxDir := c.config.SandboxDir
	if override, ok := call.Metadata["sandbox_dir"].(string); ok && override != "" {
		// Only allow narrowing if component sandbox is set
		if c.config.SandboxDir != "" {
			rel, relErr := filepath.Rel(c.config.SandboxDir, override)
			if relErr != nil || strings.HasPrefix(rel, "..") {
				c.logger.Warn("sandbox_dir override rejected: escapes component sandbox",
					"component_sandbox", c.config.SandboxDir, "requested", override)
			} else {
				sandboxDir = override
			}
		} else {
			sandboxDir = override
		}
	}
	if sandboxDir != "" {
		if call.Arguments == nil {
			call.Arguments = make(map[string]any)
		}
		call.Arguments["_sandbox_dir"] = sandboxDir
	}

	return agent, quest
}

// classifyBashCommand inspects the command string from a bash tool call and
// returns a semantic intent label for trajectory analytics. This preserves the
// observability we had with specialized tools (run_tests, lint_check, etc.)
// after consolidating them into bash.
func classifyBashCommand(call agentic.ToolCall) string {
	command, _ := call.Arguments["command"].(string)
	if command == "" {
		return ""
	}
	lower := strings.ToLower(command)

	switch {
	// Test runners
	case strings.Contains(lower, "pytest") ||
		strings.Contains(lower, "unittest") ||
		strings.Contains(lower, "go test") ||
		strings.Contains(lower, "npm test") ||
		strings.Contains(lower, "npx vitest") ||
		strings.Contains(lower, "npx jest") ||
		strings.Contains(lower, "cargo test"):
		return "test"

	// Linters
	case strings.Contains(lower, "lint") ||
		strings.Contains(lower, "go vet") ||
		strings.Contains(lower, "pylint") ||
		strings.Contains(lower, "flake8") ||
		strings.Contains(lower, "mypy") ||
		strings.Contains(lower, "ruff") ||
		strings.Contains(lower, "clippy"):
		return "lint"

	// Build commands
	case strings.Contains(lower, "go build") ||
		strings.Contains(lower, "npm run build") ||
		strings.Contains(lower, "cargo build") ||
		strings.Contains(lower, "make") && !strings.Contains(lower, "mkdir"):
		return "build"

	// Dependency management
	case strings.Contains(lower, "pip install") ||
		strings.Contains(lower, "npm install") ||
		strings.Contains(lower, "go mod") ||
		strings.Contains(lower, "cargo add") ||
		strings.Contains(lower, "cargo fetch"):
		return "deps"

	// Git operations
	case strings.HasPrefix(lower, "git ") ||
		strings.Contains(lower, " git "):
		return "git"

	default:
		return "shell"
	}
}

// addToolHint appends a corrective hint to tool error messages when the agent
// is likely using the wrong tool. This is more reliable than prompt instructions
// because the agent sees the hint at the exact moment of failure.
func addToolHint(toolName, errMsg string) string {
	lower := strings.ToLower(errMsg)

	switch toolName {
	case "bash":
		if strings.Contains(lower, "syntax error") || strings.Contains(lower, "unexpected") {
			return errMsg + "\n\nHINT: If you were trying to write code, use write_file instead of bash. bash is for shell commands only."
		}
		if strings.Contains(lower, "permission denied") && strings.Contains(lower, "python") {
			return errMsg + "\n\nHINT: Use 'python3' instead of 'python'. For pip, create a venv: " +
				"bash(\"python3 -m venv .venv && .venv/bin/pip install -r requirements.txt\")"
		}
		if strings.Contains(lower, "externally-managed") {
			return errMsg + "\n\nHINT: Python environment is OS-managed. Create a venv first: " +
				"bash(\"python3 -m venv .venv && .venv/bin/pip install -r requirements.txt\")"
		}
	case "read_file":
		if strings.Contains(lower, "not found") || strings.Contains(lower, "404") {
			return errMsg + "\n\nHINT: File doesn't exist yet. Use write_file to create it, or list_directory to see what files exist."
		}
		if strings.Contains(lower, "is a directory") {
			return errMsg + "\n\nHINT: Use list_directory to see contents of a directory."
		}
	case "graph_search":
		if strings.Contains(lower, "eof") || strings.Contains(lower, "failed") {
			return errMsg + "\n\nHINT: The knowledge graph may be temporarily unavailable. Try web_search instead, or proceed with what you know."
		}
	}

	return errMsg
}
