package executor

import (
	"context"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agent_progression"
	"github.com/c360studio/semdragons/processor/questboard"
	"github.com/c360studio/semstreams/agentic"
)

// =============================================================================
// TOOL HANDLERS
// =============================================================================
// Additional tool registration utilities beyond RegisterBuiltins.
// =============================================================================

// RegisterCustomTool adds a custom tool to the registry.
func (c *Component) RegisterCustomTool(tool RegisteredTool) {
	if c.toolRegistry != nil {
		c.toolRegistry.Register(tool)
	}
}

// RegisterToolWithHandler is a convenience method to register a tool with handler.
func (c *Component) RegisterToolWithHandler(
	name, description string,
	parameters map[string]any,
	handler ToolHandler,
	skills []domain.SkillTag,
	minTier domain.TrustTier,
) {
	if c.toolRegistry == nil {
		return
	}

	c.toolRegistry.Register(RegisteredTool{
		Definition: agentic.ToolDefinition{
			Name:        name,
			Description: description,
			Parameters:  parameters,
		},
		Handler: handler,
		Skills:  skills,
		MinTier: minTier,
	})
}

// =============================================================================
// EXECUTION HELPERS
// =============================================================================
// Utility methods for working with execution results.
// =============================================================================

// ExecuteWithCallback runs a quest and calls the callback with the result.
// Useful for async execution patterns.
func (c *Component) ExecuteWithCallback(
	ctx context.Context,
	agent *agent_progression.Agent,
	quest *questboard.Quest,
	callback func(*ExecutionResult, error),
) {
	go func() {
		result, err := c.Execute(ctx, agent, quest)
		callback(result, err)
	}()
}

// ExecutionStats returns execution statistics.
func (c *Component) ExecutionStats() ExecutionStats {
	return ExecutionStats{
		Started:   c.executionsStarted.Load(),
		Completed: c.executionsCompleted.Load(),
		Failed:    c.executionsFailed.Load(),
		ToolCalls: c.toolCallsTotal.Load(),
		Errors:    c.errorsCount.Load(),
		Uptime:    time.Since(c.startTime),
	}
}

// ExecutionStats holds execution statistics.
type ExecutionStats struct {
	Started   uint64        `json:"started"`
	Completed uint64        `json:"completed"`
	Failed    uint64        `json:"failed"`
	ToolCalls uint64        `json:"tool_calls"`
	Errors    int64         `json:"errors"`
	Uptime    time.Duration `json:"uptime"`
}

// =============================================================================
// TOOL LISTING
// =============================================================================

// ListTools returns all registered tools.
func (c *Component) ListTools() []RegisteredTool {
	if c.toolRegistry == nil {
		return nil
	}

	c.toolRegistry.mu.RLock()
	defer c.toolRegistry.mu.RUnlock()

	tools := make([]RegisteredTool, 0, len(c.toolRegistry.tools))
	for _, tool := range c.toolRegistry.tools {
		tools = append(tools, tool)
	}
	return tools
}

// GetToolsForAgent returns tools available to an agent for a quest.
func (c *Component) GetToolsForAgent(agent *agent_progression.Agent, quest *questboard.Quest) []agentic.ToolDefinition {
	if c.toolRegistry == nil {
		return nil
	}
	return c.toolRegistry.GetToolsForQuest(quest, agent)
}

// ToolCount returns the number of registered tools.
func (c *Component) ToolCount() int {
	if c.toolRegistry == nil {
		return 0
	}

	c.toolRegistry.mu.RLock()
	defer c.toolRegistry.mu.RUnlock()

	return len(c.toolRegistry.tools)
}
