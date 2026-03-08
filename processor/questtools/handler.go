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
		"quest_id", quest.ID, "tier", agent.Tier)

	// Execute the tool through the registry, which enforces tier and skill gates.
	result := c.toolRegistry.Execute(ctx, call, quest, agent)

	// Ensure Content is non-empty when an error occurred. The agentic-loop converts
	// ToolResult.Content into the ChatMessage.Content for role=tool messages. Gemini
	// (and other providers) reject tool result messages with empty content, so we
	// must surface the error string as content for the LLM to react to.
	if result.Content == "" && result.Error != "" {
		result.Content = fmt.Sprintf("Tool error: %s", result.Error)
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
