package questtools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/questbridge"
	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	pkgtypes "github.com/c360studio/semstreams/pkg/types"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nuid"
)

// exploreSystemPrompt is the system prompt injected into the explore sub-agent context.
const exploreSystemPrompt = `You are a research assistant investigating a topic for another agent.

YOUR ROLE:
- Systematically investigate the given goal using your read-only tools
- Cross-reference findings from multiple sources
- Synthesize results into a clear, actionable report

WORKFLOW:
1. Start with graph_summary to understand what data sources are available
2. Use graph_search for project-specific lookups (try 'nlq' for natural language questions)
3. Use web_search for external information not found in the graph
4. Use http_request to fetch specific URLs found via search
5. Cross-reference and verify findings across sources

OUTPUT FORMAT:
When you have gathered sufficient information, call submit_findings with a structured report:
- Key Findings: the most important discoveries
- Relevant Entities: entity IDs, file paths, or URLs found
- Relationships: how the discovered items relate to each other
- Recommendations: suggested approach based on findings

Keep your report concise (under 3000 tokens). Focus on actionable information.
You have READ-ONLY access. Do not attempt to modify anything.`

// handleExplore spawns a read-only explore sub-agent and waits for it to complete,
// returning the sub-agent's findings as the tool result.
//
// This runs in a goroutine (launched by handleToolExecute) because it can take
// 30-120 seconds, which would block the consumer if run inline.
func (c *Component) handleExplore(ctx context.Context, call agentic.ToolCall, agent *agentprogression.Agent, quest *domain.Quest) agentic.ToolResult {
	goal, _ := call.Arguments["goal"].(string)
	if goal == "" {
		return agentic.ToolResult{CallID: call.ID, Error: "explore: goal argument is required and must be non-empty"}
	}

	extraContext, _ := call.Arguments["context"].(string)

	// Prevent duplicate explores from the same parent loop.
	if _, loaded := c.activeExplores.LoadOrStore(call.LoopID, struct{}{}); loaded {
		return agentic.ToolResult{
			CallID:  call.ID,
			Error:   "explore: another explore is already running for this loop — wait for it to complete before starting a new one",
		}
	}
	defer c.activeExplores.Delete(call.LoopID)

	loopID := "explore-" + nuid.Next()
	// NATS subjects cannot contain dots; replace with dashes for subject routing.
	subjectSafeLoopID := strings.ReplaceAll(loopID, ".", "-")

	// Build the user prompt: goal text with optional caller-supplied context appended.
	userPrompt := goal
	if extraContext != "" {
		userPrompt = goal + "\n\nADDITIONAL CONTEXT:\n" + extraContext
	}

	// Collect tool definitions the explore agent is allowed to use.
	tools := c.toolRegistry.GetExploreTools(agent)

	// Resolve model capability key. The agentic-loop resolves capability → endpoint
	// via the model registry — we pass the capability key, not a resolved model name.
	modelKey := c.resolveExploreModel()

	// Build TaskMessage for the explore sub-agent.
	taskMsg := agentic.TaskMessage{
		TaskID:       string(quest.ID),
		LoopID:       loopID,
		Role:         "explore",
		Model:        modelKey,
		Prompt:       userPrompt,
		ParentLoopID: call.LoopID,
		Depth:        1,
		MaxDepth:     1,
		Context: &pkgtypes.ConstructedContext{
			Content:       exploreSystemPrompt,
			ConstructedAt: time.Now(),
		},
		Tools: tools,
		Metadata: func() map[string]any {
			// Inherit agent/quest context so questtools can gate tool calls.
			meta := make(map[string]any)
			if call.Metadata != nil {
				for k, v := range call.Metadata {
					meta[k] = v
				}
			}
			meta["max_iterations"] = c.config.ExploreMaxIterations
			return meta
		}(),
	}

	baseMsg := message.NewBaseMessage(taskMsg.Schema(), &taskMsg, "questtools")
	data, err := json.Marshal(baseMsg)
	if err != nil {
		return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("explore: failed to marshal TaskMessage: %v", err)}
	}

	// Persist a QuestLoopMapping so questbridge tracks token usage and skips
	// explore loop completions (it does not transition quest state for them).
	if c.questLoopsBucket != nil {
		mapping := questbridge.QuestLoopMapping{
			LoopID:    loopID,
			QuestID:   quest.ID,
			AgentID:   agent.ID,
			TrustTier: agent.Tier,
			StartedAt: time.Now(),
			LoopType:  questbridge.LoopTypeExplore,
		}
		mappingData, marshalErr := json.Marshal(mapping)
		if marshalErr == nil {
			if _, putErr := c.questLoopsBucket.Put(ctx, loopID, mappingData); putErr != nil {
				c.logger.Warn("failed to write explore loop mapping to QUEST_LOOPS",
					"loop_id", loopID, "error", putErr)
			}
		}
	}

	// Publish the TaskMessage to the AGENT stream on a unique subject.
	subject := fmt.Sprintf("agent.task.%s", subjectSafeLoopID)
	if err := c.deps.NATSClient.PublishToStream(ctx, subject, data); err != nil {
		c.cleanupExploreMapping(ctx, loopID)
		return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("explore: failed to publish TaskMessage: %v", err)}
	}

	c.logger.Info("explore sub-agent dispatched",
		"loop_id", loopID,
		"parent_loop_id", call.LoopID,
		"quest_id", quest.ID,
		"agent_id", agent.ID,
		"tools", len(tools))

	// Wait for the explore sub-agent to complete via JetStream ephemeral consumer.
	result := c.waitForExploreResult(ctx, loopID, subjectSafeLoopID, call.ID)

	c.cleanupExploreMapping(ctx, loopID)
	return result
}

// waitForExploreResult subscribes to agent.complete.{loopID} and agent.failed.{loopID}
// on the AGENT stream and blocks until one arrives or the configured timeout fires.
func (c *Component) waitForExploreResult(ctx context.Context, loopID, subjectSafeLoopID, callID string) agentic.ToolResult {
	timeout := c.config.ExploreTimeoutDuration()

	js, err := c.deps.NATSClient.JetStream()
	if err != nil {
		return agentic.ToolResult{
			CallID:  callID,
			Error:   fmt.Sprintf("explore: failed to obtain JetStream handle: %v", err),
		}
	}

	// Create an ephemeral ordered consumer filtered to this loop's completion subjects.
	// OrderedConsumer is push-based, low-overhead, and ideal for transient listeners.
	consCtx, consCancel := context.WithTimeout(ctx, timeout+5*time.Second)
	defer consCancel()

	consumer, err := js.OrderedConsumer(consCtx, c.config.StreamName, jetstream.OrderedConsumerConfig{
		FilterSubjects: []string{
			fmt.Sprintf("agent.complete.%s", subjectSafeLoopID),
			fmt.Sprintf("agent.failed.%s", subjectSafeLoopID),
		},
		DeliverPolicy: jetstream.DeliverAllPolicy,
	})
	if err != nil {
		return agentic.ToolResult{
			CallID:  callID,
			Error:   fmt.Sprintf("explore: failed to create completion consumer: %v", err),
		}
	}

	type resultMsg struct {
		result agentic.ToolResult
		err    error
	}
	resultCh := make(chan resultMsg, 1)

	go func() {
		msgs, fetchErr := consumer.Fetch(1, jetstream.FetchMaxWait(timeout))
		if fetchErr != nil {
			resultCh <- resultMsg{err: fetchErr}
			return
		}
		for msg := range msgs.Messages() {
			r := c.parseExploreCompletion(msg, callID, loopID)
			_ = msg.Ack()
			resultCh <- resultMsg{result: r}
			return
		}
		// No messages arrived within the fetch window.
		resultCh <- resultMsg{err: fmt.Errorf("no completion message received")}
	}()

	select {
	case rm := <-resultCh:
		if rm.err != nil {
			c.logger.Warn("explore timed out or failed waiting for result",
				"loop_id", loopID, "error", rm.err)
			return agentic.ToolResult{
				CallID:  callID,
				Content: fmt.Sprintf("Explore timed out after %s. Try a more specific goal or use graph_search directly.", timeout),
			}
		}
		return rm.result
	case <-ctx.Done():
		return agentic.ToolResult{
			CallID:  callID,
			Content: "Explore cancelled (parent loop context done).",
		}
	case <-time.After(timeout + 10*time.Second):
		// Hard backstop — should not be reached since Fetch already has the timeout.
		return agentic.ToolResult{
			CallID:  callID,
			Content: fmt.Sprintf("Explore timed out after %s. Try a more specific goal or use graph_search directly.", timeout),
		}
	}
}

// parseExploreCompletion unwraps a JetStream message from agent.complete.* or
// agent.failed.* and converts it to a ToolResult.
func (c *Component) parseExploreCompletion(msg jetstream.Msg, callID, loopID string) agentic.ToolResult {
	var baseMsg message.BaseMessage
	if err := json.Unmarshal(msg.Data(), &baseMsg); err != nil {
		c.logger.Warn("failed to unmarshal explore completion BaseMessage",
			"loop_id", loopID, "error", err)
		return agentic.ToolResult{
			CallID:  callID,
			Content: "Explore completed but result could not be parsed.",
		}
	}

	subject := msg.Subject()
	switch {
	case strings.Contains(subject, "agent.complete."):
		event, ok := baseMsg.Payload().(*agentic.LoopCompletedEvent)
		if !ok {
			c.logger.Warn("unexpected payload type in explore complete message",
				"loop_id", loopID, "type", fmt.Sprintf("%T", baseMsg.Payload()))
			return agentic.ToolResult{
				CallID:  callID,
				Content: "Explore completed but result payload type was unexpected.",
			}
		}
		c.logger.Info("explore sub-agent completed",
			"loop_id", loopID,
			"iterations", event.Iterations,
			"result_len", len(event.Result))
		result := event.Result
		if result == "" {
			result = "(explore completed with no findings)"
		}
		return agentic.ToolResult{CallID: callID, Content: result}

	case strings.Contains(subject, "agent.failed."):
		event, ok := baseMsg.Payload().(*agentic.LoopFailedEvent)
		if !ok {
			c.logger.Warn("unexpected payload type in explore failed message",
				"loop_id", loopID, "type", fmt.Sprintf("%T", baseMsg.Payload()))
			return agentic.ToolResult{
				CallID: callID,
				Error:  "Explore failed: result payload type was unexpected.",
			}
		}
		c.logger.Warn("explore sub-agent failed",
			"loop_id", loopID, "error", event.Error)
		return agentic.ToolResult{
			CallID: callID,
			Error:  "Explore failed: " + event.Error,
		}

	default:
		c.logger.Warn("explore completion arrived on unexpected subject",
			"loop_id", loopID, "subject", subject)
		return agentic.ToolResult{
			CallID:  callID,
			Content: "Explore completed (unexpected subject).",
		}
	}
}

// resolveExploreModel returns the model capability key for explore sub-agents.
// The agentic-loop resolves capability → endpoint via the model registry.
func (c *Component) resolveExploreModel() string {
	if c.config.ExploreCapability != "" {
		return c.config.ExploreCapability
	}
	return "explore"
}

// cleanupExploreMapping removes the QUEST_LOOPS entry written for an explore loop.
func (c *Component) cleanupExploreMapping(ctx context.Context, loopID string) {
	if c.questLoopsBucket == nil {
		return
	}
	if err := c.questLoopsBucket.Delete(ctx, loopID); err != nil {
		c.logger.Debug("failed to delete explore loop mapping (may already be gone)",
			"loop_id", loopID, "error", err)
	}
}
