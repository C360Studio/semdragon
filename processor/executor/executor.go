// Package executor provides a native semstreams component for quest execution.
// It manages how agents actually DO work - building prompts, calling LLMs,
// handling tool calls, and returning structured output for judge evaluation.
package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/promptmanager"
	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/model"
	agenticmodel "github.com/c360studio/semstreams/processor/agentic-model"

	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// EXECUTOR INTERFACE
// =============================================================================

// AgentExecutor executes quests using an agent's LLM configuration.
type AgentExecutor interface {
	// Execute runs the agent on a quest and returns the result.
	Execute(ctx context.Context, agent *agentprogression.Agent, quest *domain.Quest) (*ExecutionResult, error)
}

// Compile-time interface compliance checks.
var (
	_ AgentExecutor = (*DefaultExecutor)(nil)
	_ AgentExecutor = (*MockExecutor)(nil)
)

// =============================================================================
// EXECUTION TYPES
// =============================================================================

// ExecutionStatus represents the outcome of an execution attempt.
type ExecutionStatus string

const (
	// StatusComplete indicates execution finished successfully.
	StatusComplete ExecutionStatus = "complete"
	// StatusToolLimit indicates max tool calls were reached.
	StatusToolLimit ExecutionStatus = "tool_limit"
	// StatusTokenLimit indicates token budget was exhausted.
	StatusTokenLimit ExecutionStatus = "token_limit"
	// StatusTimeout indicates execution timed out.
	StatusTimeout ExecutionStatus = "timeout"
	// StatusFailed indicates execution failed with an error.
	StatusFailed ExecutionStatus = "failed"
)

// maxMessagesInContext limits the conversation size to prevent memory issues.
// When exceeded, older messages are truncated (keeping system message).
const maxMessagesInContext = 100

// ExecutionResult holds the outcome of agent execution.
type ExecutionResult struct {
	Output     any                `json:"output"`      // Quest-specific output
	Trajectory []ExecutionStep    `json:"trajectory"`  // Full execution trace
	TokenUsage agentic.TokenUsage `json:"token_usage"` // Aggregated token usage
	ToolCalls  int                `json:"tool_calls"`  // Total tool invocations
	Duration   time.Duration      `json:"duration"`    // Total execution time
	Status     ExecutionStatus    `json:"status"`      // Outcome status
	Error      string             `json:"error,omitempty"`
	LoopID     string             `json:"loop_id"` // Links to semstreams trajectory
}

// ExecutionStep records one turn in the execution loop.
type ExecutionStep struct {
	Turn        int                   `json:"turn"`
	Request     agentic.AgentRequest  `json:"request"`
	Response    agentic.AgentResponse `json:"response"`
	ToolResults []agentic.ToolResult  `json:"tool_results,omitempty"`
	Timestamp   time.Time             `json:"timestamp"`
}

// =============================================================================
// DEFAULT EXECUTOR
// =============================================================================

// DefaultExecutor implements AgentExecutor using semstreams model infrastructure.
type DefaultExecutor struct {
	registry        model.RegistryReader
	toolRegistry    *ToolRegistry
	promptAssembler *promptmanager.PromptAssembler // nil = legacy path
	domainCatalog   *promptmanager.DomainCatalog   // nil = no checklist
	maxTurns        int
	maxTokens       int
}

// Option configures a DefaultExecutor.
type Option func(*DefaultExecutor)

// WithMaxTurns sets the maximum number of tool-call turns.
func WithMaxTurns(n int) Option {
	return func(e *DefaultExecutor) {
		e.maxTurns = n
	}
}

// WithMaxTokens sets the token budget for execution.
func WithMaxTokens(n int) Option {
	return func(e *DefaultExecutor) {
		e.maxTokens = n
	}
}

// WithPromptAssembler sets the domain-aware prompt assembler.
// When nil, buildSystemPrompt falls back to the legacy string concatenation path.
func WithPromptAssembler(pa *promptmanager.PromptAssembler) Option {
	return func(e *DefaultExecutor) {
		e.promptAssembler = pa
	}
}

// WithDomainCatalog sets the domain catalog for structural checklist injection.
func WithDomainCatalog(cat *promptmanager.DomainCatalog) Option {
	return func(e *DefaultExecutor) {
		e.domainCatalog = cat
	}
}

// NewDefaultExecutor creates an executor using the model registry and tool registry.
func NewDefaultExecutor(registry model.RegistryReader, tools *ToolRegistry, opts ...Option) *DefaultExecutor {
	e := &DefaultExecutor{
		registry:     registry,
		toolRegistry: tools,
		maxTurns:     20,    // Max tool-call loops
		maxTokens:    50000, // Budget per execution
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Execute runs the agent on a quest and returns the result.
func (e *DefaultExecutor) Execute(ctx context.Context, agent *agentprogression.Agent, quest *domain.Quest) (*ExecutionResult, error) {
	startTime := time.Now()
	loopID := fmt.Sprintf("%s-%s-%d", agent.ID, quest.ID, startTime.UnixNano())

	result := &ExecutionResult{
		Trajectory: make([]ExecutionStep, 0),
		LoopID:     loopID,
	}

	// Resolve endpoint: agent override → tier+skill → tier → global fallback
	endpointName := agent.Config.Provider
	if endpointName == "" {
		capability := e.resolveCapability(agent, quest)
		endpointName = e.registry.Resolve(capability)
	}
	endpoint := e.registry.GetEndpoint(endpointName)
	if endpoint == nil {
		// Try default endpoint
		endpoint = e.registry.GetEndpoint(e.registry.GetDefault())
	}
	if endpoint == nil {
		result.Status = StatusFailed
		result.Error = "no model endpoint available"
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// Create client
	client, err := agenticmodel.NewClient(endpoint)
	if err != nil {
		result.Status = StatusFailed
		result.Error = fmt.Sprintf("create client: %v", err)
		result.Duration = time.Since(startTime)
		return result, nil
	}
	defer client.Close()

	// Build initial messages
	messages := e.buildInitialMessages(agent, quest)

	// Get available tools for this quest
	var tools []agentic.ToolDefinition
	if e.toolRegistry != nil {
		tools = e.toolRegistry.GetToolsForQuest(quest, agent)
	}

	// Execution loop
	for turn := 0; turn < e.maxTurns; turn++ {
		// Check context
		select {
		case <-ctx.Done():
			result.Status = StatusTimeout
			result.Error = ctx.Err().Error()
			result.Duration = time.Since(startTime)
			return result, nil
		default:
		}

		// Build request
		req := agentic.AgentRequest{
			RequestID:   fmt.Sprintf("%s-turn-%d", quest.ID, turn),
			LoopID:      loopID,
			Role:        agentic.RoleGeneral,
			Messages:    messages,
			Model:       endpoint.Model,
			MaxTokens:   e.getMaxTokens(agent, endpoint),
			Temperature: e.getTemperature(agent),
			Tools:       tools,
		}

		// Call LLM
		resp, err := client.ChatCompletion(ctx, req)
		if err != nil {
			result.Status = StatusFailed
			result.Error = err.Error()
			result.Duration = time.Since(startTime)
			return result, nil
		}

		// Record step
		step := ExecutionStep{
			Turn:      turn,
			Request:   req,
			Response:  resp,
			Timestamp: time.Now(),
		}

		// Track token usage
		result.TokenUsage.PromptTokens += resp.TokenUsage.PromptTokens
		result.TokenUsage.CompletionTokens += resp.TokenUsage.CompletionTokens

		// Handle response based on status
		switch resp.Status {
		case agentic.StatusComplete:
			result.Status = StatusComplete
			result.Output = resp.Message.Content
			result.Trajectory = append(result.Trajectory, step)
			result.Duration = time.Since(startTime)
			return result, nil

		case agentic.StatusToolCall:
			// Add assistant message to conversation
			messages = append(messages, resp.Message)

			// Execute each tool call
			toolResults := make([]agentic.ToolResult, 0, len(resp.Message.ToolCalls))
			for _, tc := range resp.Message.ToolCalls {
				toolResult := e.executeTool(ctx, tc, quest, agent)
				toolResults = append(toolResults, toolResult)
				result.ToolCalls++

				// Add tool result to conversation
				messages = append(messages, agentic.ChatMessage{
					Role:       "tool",
					Content:    toolResult.Content,
					ToolCallID: tc.ID,
				})
			}
			step.ToolResults = toolResults

		case agentic.StatusError:
			result.Status = StatusFailed
			result.Error = resp.Error
			result.Trajectory = append(result.Trajectory, step)
			result.Duration = time.Since(startTime)
			return result, nil
		}

		result.Trajectory = append(result.Trajectory, step)

		// Truncate messages if too many (keep system message + recent messages)
		if len(messages) > maxMessagesInContext {
			// Keep first message (system) + last N messages
			messages = append(messages[:1], messages[len(messages)-maxMessagesInContext+1:]...)
		}

		// Check token budget
		if result.TokenUsage.Total() > e.maxTokens {
			result.Status = StatusTokenLimit
			result.Duration = time.Since(startTime)
			return result, nil
		}
	}

	// Reached max turns
	result.Status = StatusToolLimit
	result.Duration = time.Since(startTime)
	return result, nil
}

// buildInitialMessages constructs the initial conversation for a quest.
func (e *DefaultExecutor) buildInitialMessages(agent *agentprogression.Agent, quest *domain.Quest) []agentic.ChatMessage {
	messages := make([]agentic.ChatMessage, 0, 2)

	// System message: agent persona + quest context
	systemPrompt := e.buildSystemPrompt(agent, quest)
	messages = append(messages, agentic.ChatMessage{
		Role:    "system",
		Content: systemPrompt,
	})

	// User message: the actual quest task
	userPrompt := e.buildUserPrompt(quest)
	messages = append(messages, agentic.ChatMessage{
		Role:    "user",
		Content: userPrompt,
	})

	return messages
}

// buildSystemPrompt constructs the system prompt from agent persona and quest context.
// When a promptAssembler is configured, it delegates to the domain-aware assembly pipeline.
// Otherwise, falls back to the legacy string concatenation path.
func (e *DefaultExecutor) buildSystemPrompt(agent *agentprogression.Agent, quest *domain.Quest) string {
	if e.promptAssembler != nil {
		return e.buildAssembledSystemPrompt(agent, quest)
	}
	return e.buildLegacySystemPrompt(agent, quest)
}

// buildAssembledSystemPrompt uses the domain-aware prompt assembler.
func (e *DefaultExecutor) buildAssembledSystemPrompt(agent *agentprogression.Agent, quest *domain.Quest) string {
	var personaPrompt string
	if agent.Persona != nil {
		personaPrompt = agent.Persona.SystemPrompt
	}

	var maxDuration string
	if quest.Constraints.MaxDuration > 0 {
		maxDuration = quest.Constraints.MaxDuration.String()
	}

	// Resolve provider from endpoint for provider-aware formatting
	provider := agent.Config.Provider
	if provider == "" {
		endpointName := e.registry.Resolve(e.resolveCapability(agent, quest))
		if ep := e.registry.GetEndpoint(endpointName); ep != nil {
			provider = ep.Provider
		}
	}

	// Inject structural checklist from domain catalog so agents self-check before submitting.
	var checklist []promptmanager.ChecklistItem
	if e.domainCatalog != nil && e.domainCatalog.ReviewConfig != nil {
		checklist = e.domainCatalog.ReviewConfig.StructuralChecklist
	}

	ctx := promptmanager.AssemblyContext{
		AgentID:             agent.ID,
		Tier:                agent.Tier,
		Level:               agent.Level,
		Skills:              agent.SkillProficiencies,
		Guild:               agent.Guild,
		SystemPrompt:        agent.Config.SystemPrompt,
		PersonaPrompt:       personaPrompt,
		QuestTitle:          quest.Title,
		QuestDescription:    quest.Description,
		QuestInput:          quest.Input,
		RequiredSkills:      quest.RequiredSkills,
		MaxDuration:         maxDuration,
		MaxTokens:           quest.Constraints.MaxTokens,
		Provider:            provider,
		StructuralChecklist: checklist,
	}

	result := e.promptAssembler.AssembleSystemPrompt(ctx)
	return result.SystemMessage
}

// buildLegacySystemPrompt is the original string concatenation path.
// Used when no promptAssembler is configured (backward compatibility).
func (e *DefaultExecutor) buildLegacySystemPrompt(agent *agentprogression.Agent, quest *domain.Quest) string {
	var prompt string

	// Start with agent's system prompt if configured
	if agent.Config.SystemPrompt != "" {
		prompt = agent.Config.SystemPrompt + "\n\n"
	}

	// Add persona if available
	if agent.Persona != nil && agent.Persona.SystemPrompt != "" {
		prompt += agent.Persona.SystemPrompt + "\n\n"
	}

	// Add quest context
	prompt += fmt.Sprintf("You are working on a quest: %s\n", quest.Title)
	if quest.Description != "" {
		prompt += fmt.Sprintf("Description: %s\n", quest.Description)
	}

	// Add constraints
	if quest.Constraints.MaxDuration > 0 {
		prompt += fmt.Sprintf("Time limit: %v\n", quest.Constraints.MaxDuration)
	}
	if quest.Constraints.MaxTokens > 0 {
		prompt += fmt.Sprintf("Token budget: %d\n", quest.Constraints.MaxTokens)
	}

	// Add required skills context
	if len(quest.RequiredSkills) > 0 {
		prompt += "This quest requires skills in: "
		for i, skill := range quest.RequiredSkills {
			if i > 0 {
				prompt += ", "
			}
			prompt += string(skill)
		}
		prompt += "\n"
	}

	return prompt
}

// buildUserPrompt constructs the user prompt from quest input.
func (e *DefaultExecutor) buildUserPrompt(quest *domain.Quest) string {
	if quest.Input == nil {
		return quest.Description
	}

	// If input is a string, use it directly
	if s, ok := quest.Input.(string); ok {
		return s
	}

	// Otherwise, format it as context
	return fmt.Sprintf("Quest input:\n%v\n\nPlease complete the quest: %s", quest.Input, quest.Description)
}

// executeTool runs a tool call and returns the result.
func (e *DefaultExecutor) executeTool(ctx context.Context, call agentic.ToolCall, quest *domain.Quest, agent *agentprogression.Agent) agentic.ToolResult {
	if e.toolRegistry == nil {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  "no tool registry configured",
		}
	}

	return e.toolRegistry.Execute(ctx, call, quest, agent)
}

// getMaxTokens returns the max tokens for the request output.
// Considers agent config, endpoint context window, and reasonable defaults.
func (e *DefaultExecutor) getMaxTokens(agent *agentprogression.Agent, endpoint *model.EndpointConfig) int {
	// Use agent's configured max if specified
	if agent.Config.MaxTokens > 0 {
		return agent.Config.MaxTokens
	}

	// Default output limit
	defaultOutput := 4096

	// If endpoint specifies a context window, cap output to leave room for input.
	// Rule of thumb: output should be at most 25% of context window to leave
	// room for system prompt, conversation history, and tool results.
	if endpoint != nil && endpoint.MaxTokens > 0 {
		maxOutput := endpoint.MaxTokens / 4
		if maxOutput < defaultOutput {
			return maxOutput
		}
	}

	return defaultOutput
}

// getTemperature returns the temperature for the request.
func (e *DefaultExecutor) getTemperature(agent *agentprogression.Agent) float64 {
	if agent.Config.Temperature > 0 {
		return agent.Config.Temperature
	}
	return 0.2 // Reasonable default for task completion
}

// resolveCapability builds a capability key from agent tier and quest skill.
// Resolution chain (most specific wins):
//  1. "agent-work.{tier}.{skill}" — tier-qualified skill capability
//  2. "agent-work.{tier}"         — tier-level default
//  3. "agent-work"                — global default
//
// Uses GetFallbackChain to detect whether a capability key exists in the registry.
// GetFallbackChain returns nil for unknown keys, making it a reliable existence check.
func (e *DefaultExecutor) resolveCapability(agent *agentprogression.Agent, quest *domain.Quest) string {
	tier := agent.Tier.String()

	// Try tier + primary skill first
	if skill := quest.PrimarySkill(); skill != "" {
		key := fmt.Sprintf("agent-work.%s.%s", tier, string(skill))
		if chain := e.registry.GetFallbackChain(key); len(chain) > 0 {
			return key
		}
	}

	// Fall back to tier default
	key := fmt.Sprintf("agent-work.%s", tier)
	if chain := e.registry.GetFallbackChain(key); len(chain) > 0 {
		return key
	}

	// Fall back to global
	return "agent-work"
}

// =============================================================================
// MOCK EXECUTOR
// =============================================================================

// MockExecutor is a test implementation of AgentExecutor.
type MockExecutor struct {
	ExecuteFunc func(ctx context.Context, agent *agentprogression.Agent, quest *domain.Quest) (*ExecutionResult, error)
}

// Execute implements AgentExecutor.
func (m *MockExecutor) Execute(ctx context.Context, agent *agentprogression.Agent, quest *domain.Quest) (*ExecutionResult, error) {
	if m.ExecuteFunc != nil {
		return m.ExecuteFunc(ctx, agent, quest)
	}
	return &ExecutionResult{
		Status: StatusComplete,
		Output: "mock output",
	}, nil
}
