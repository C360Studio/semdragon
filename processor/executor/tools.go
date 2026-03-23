package executor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/questdagexec"
	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
)

// =============================================================================
// TOOL REGISTRY - Manages available tools for agent execution
// =============================================================================
// Tools are capabilities that agents can invoke during quest execution.
// The registry maps tool names to handlers and enforces trust/skill gates.
// =============================================================================

// Tool handler constants for limits and configuration.
const (
	// maxHTTPResponseSize is the maximum bytes to return from an HTTP response.
	maxHTTPResponseSize = 100000
	// maxGraphResponseSize is the maximum bytes for graph search responses.
	// Larger than HTTP because community summaries + entity lists are verbose.
	maxGraphResponseSize = 500000
	// maxCommandOutput is the maximum bytes to capture from command output.
	maxCommandOutput = 100000
	// commandTimeout is the default timeout for shell commands.
	commandTimeout = 60 * time.Second
	// httpRequestTimeout is the timeout for HTTP requests.
	httpRequestTimeout = 30 * time.Second
)

// =============================================================================
// HTTP RESPONSE HANDLING - HTML conversion and graph persistence
// =============================================================================

// httpTextMaxSize is the maximum characters after HTML-to-text conversion.
// Configurable via SetHTTPTextMaxSize; defaults to 20000.
var httpTextMaxSize = 20000

// minHTTPPersistLength is the minimum text length (after conversion) required
// to persist a web page to the knowledge graph. Short pages are likely error
// pages or stubs not worth storing.
var minHTTPPersistLength = 500

// httpGraphPersist is set when graph persistence is enabled.
// Nil means graph persistence is disabled (the default).
var httpGraphPersist *graphPersister

// graphPersister holds the dependencies required to write web content to the
// knowledge graph. Populated by SetHTTPGraphPersist.
type graphPersister struct {
	graph  *semdragons.GraphClient
	config *domain.BoardConfig
}

// SetHTTPTextMaxSize configures the maximum characters returned after
// HTML-to-text conversion. Call this from the component's Start method.
func SetHTTPTextMaxSize(maxChars int) { httpTextMaxSize = maxChars }

// SetHTTPGraphPersist enables knowledge-graph persistence for HTML responses.
// Pass a nil GraphClient to disable persistence.
func SetHTTPGraphPersist(gc *semdragons.GraphClient, cfg *domain.BoardConfig) {
	if gc == nil {
		httpGraphPersist = nil
		return
	}
	httpGraphPersist = &graphPersister{graph: gc, config: cfg}
}

// =============================================================================

// ToolHandler executes a tool and returns the result.
// The handler receives the tool call arguments and quest/agent context.
type ToolHandler func(ctx context.Context, call agentic.ToolCall, quest *domain.Quest, agent *agentprogression.Agent) agentic.ToolResult

// ToolCategory groups tools by purpose so questbridge can send only the
// categories a quest actually needs. This reduces input tokens per API call.
type ToolCategory string

const (
	// ToolCategoryCore groups terminal tools: submit_work, ask_clarification.
	ToolCategoryCore ToolCategory = "core"
	// ToolCategoryWrite is retained for questbridge category filtering (currently unused by tools).
	ToolCategoryWrite ToolCategory = "write"
	// ToolCategoryNetwork groups external access tools: http_request, web_search.
	ToolCategoryNetwork ToolCategory = "network"
	// ToolCategoryInspect groups the bash tool.
	ToolCategoryInspect ToolCategory = "inspect"
	// ToolCategoryKnowledge groups graph tools: graph_query, graph_search, graph_summary.
	ToolCategoryKnowledge ToolCategory = "knowledge"
	// ToolCategoryPartyLead groups DAG tools: decompose, review, answer_clarification.
	ToolCategoryPartyLead ToolCategory = "party_lead"
)

// RegisteredTool wraps a tool definition with its handler and access controls.
type RegisteredTool struct {
	Definition agentic.ToolDefinition // Name, description, parameters
	Handler    ToolHandler            // Execution function
	Skills     []domain.SkillTag      // Required skills to use this tool
	MinTier    domain.TrustTier       // Minimum trust tier to use
	Category   ToolCategory           // Tool category for quest-based filtering
}

// toolSpec holds the shared metadata for a tool registration.
// Both RegisterBuiltins and RegisterSandboxTools use these specs,
// supplying different Handler implementations.
type toolSpec struct {
	Definition agentic.ToolDefinition
	MinTier    domain.TrustTier
	Skills     []domain.SkillTag
	Category   ToolCategory
}

// Shared tool specs — single source of truth for definition, tier, and skills.
// Handlers are provided separately by RegisterBuiltins (local OS) and
// RegisterSandboxTools (proxied through a SandboxClient).

var runCommandSpec = toolSpec{
	Definition: agentic.ToolDefinition{
		Name:        "bash",
		Description: "Run a shell command. Use for ALL operations: read (cat), write (cat <<'EOF' > file), search (grep -rn), list (ls -la), tests, builds, git, deps. Supports heredocs and pipes.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "The shell command to execute",
				},
			},
			"required": []any{"command"},
		},
	},
	MinTier:  domain.TierJourneyman, // Level 6+ — sandbox is the security boundary, not the tier gate
	Category: ToolCategoryInspect,
}

var httpRequestSpec = toolSpec{
	Definition: agentic.ToolDefinition{
		Name:        "http_request",
		Description: "Make an HTTP request to fetch data from a URL. Use for downloading files, calling REST APIs, or fetching web content. The response body is returned as text. For binary downloads, pipe through run_command instead.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "Full URL including https:// (e.g. 'https://api.github.com/repos/owner/repo')",
				},
				"method": map[string]any{
					"type":        "string",
					"description": "HTTP method. Defaults to GET.",
					"enum":        []any{"GET", "POST"},
				},
				"body": map[string]any{
					"type":        "string",
					"description": "Request body as a string (for POST). Use JSON format for API calls.",
				},
				"content_type": map[string]any{
					"type":        "string",
					"description": "Content-Type header (for POST). Defaults to application/json.",
				},
			},
			"required": []any{"url"},
		},
	},
	MinTier:  domain.TierJourneyman, // Level 6+ — network access requires trust
	Category: ToolCategoryNetwork,
}

// ToolRegistry manages available tools for agent execution.
type ToolRegistry struct {
	mu         sync.RWMutex
	tools      map[string]RegisteredTool
	sandboxDir string // Base directory for file operations (empty = current dir)
}

// NewToolRegistry creates a new empty tool registry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]RegisteredTool),
	}
}

// NewToolRegistryWithSandbox creates a registry with a sandbox directory.
// All file operations will be restricted to this directory.
func NewToolRegistryWithSandbox(sandboxDir string) *ToolRegistry {
	return &ToolRegistry{
		tools:      make(map[string]RegisteredTool),
		sandboxDir: sandboxDir,
	}
}

// SetSandboxDir sets the sandbox directory for file operations.
func (r *ToolRegistry) SetSandboxDir(dir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sandboxDir = dir
}

// GetSandboxDir returns the current sandbox directory.
func (r *ToolRegistry) GetSandboxDir() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sandboxDir
}

// Register adds a tool to the registry.
func (r *ToolRegistry) Register(tool RegisteredTool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Definition.Name] = tool
}

// Get returns a tool by name, or nil if not found.
func (r *ToolRegistry) Get(name string) *RegisteredTool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if tool, ok := r.tools[name]; ok {
		return &tool
	}
	return nil
}

// ListAll returns all registered tools.
// Used by processors that need to inspect available tools without holding
// a reference to the executor Component (e.g. questbridge).
func (r *ToolRegistry) ListAll() []RegisteredTool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tools := make([]RegisteredTool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}
	return tools
}

// GetToolsForQuest returns tool definitions the agent can use for this quest.
// Filters by:
// - Quest's AllowedTools list (if specified)
// - Agent's trust tier
// - Agent's skills
func (r *ToolRegistry) GetToolsForQuest(quest *domain.Quest, agent *agentprogression.Agent) []agentic.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var available []agentic.ToolDefinition

	for name, tool := range r.tools {
		// Check trust tier
		if agent.Tier < tool.MinTier {
			continue
		}

		// Check skills (agent must have at least one required skill)
		if len(tool.Skills) > 0 && !agentHasAnySkill(agent, tool.Skills) {
			continue
		}

		// Check quest's allowed tools list (if specified)
		if len(quest.AllowedTools) > 0 && !containsToolName(quest.AllowedTools, name) {
			continue
		}

		available = append(available, tool.Definition)
	}

	return available
}

// Execute runs a tool call and returns the result.
func (r *ToolRegistry) Execute(ctx context.Context, call agentic.ToolCall, quest *domain.Quest, agent *agentprogression.Agent) agentic.ToolResult {
	r.mu.RLock()
	tool, ok := r.tools[call.Name]
	sandboxDir := r.sandboxDir
	r.mu.RUnlock()

	if !ok {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  fmt.Sprintf("unknown tool: %s", call.Name),
		}
	}

	// Verify trust tier
	if agent.Tier < tool.MinTier {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  fmt.Sprintf("insufficient trust tier for tool %s (requires %d, agent has %d)", call.Name, tool.MinTier, agent.Tier),
		}
	}

	// Verify skills (agent must have at least one required skill)
	if len(tool.Skills) > 0 && !agentHasAnySkill(agent, tool.Skills) {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  fmt.Sprintf("agent lacks required skill for tool %s", call.Name),
		}
	}

	// Inject sandbox directory into call metadata for handlers
	if sandboxDir != "" {
		if call.Arguments == nil {
			call.Arguments = make(map[string]any)
		}
		call.Arguments["_sandbox_dir"] = sandboxDir
	}

	// Execute the handler
	return tool.Handler(ctx, call, quest, agent)
}

// agentHasAnySkill returns true if the agent has at least one of the given skills.
func agentHasAnySkill(agent *agentprogression.Agent, skills []domain.SkillTag) bool {
	for _, skill := range skills {
		if agent.HasSkill(skill) {
			return true
		}
	}
	return false
}

// containsToolName checks if a tool name is in the allowed list.
func containsToolName(allowed []string, name string) bool {
	for _, t := range allowed {
		if t == name {
			return true
		}
	}
	return false
}

// getSandboxDir extracts the sandbox directory from call arguments.
func getSandboxDir(call agentic.ToolCall) string {
	if call.Arguments == nil {
		return ""
	}
	if sandbox, ok := call.Arguments["_sandbox_dir"].(string); ok {
		return sandbox
	}
	return ""
}

// RegisterBuiltins adds the standard built-in tools to the registry.
func (r *ToolRegistry) RegisterBuiltins() {
	r.Register(RegisteredTool{
		Definition: runCommandSpec.Definition,
		Handler:    runCommandHandler,
		Skills:     runCommandSpec.Skills,
		MinTier:    runCommandSpec.MinTier,
		Category:   runCommandSpec.Category,
	})

	r.Register(RegisteredTool{
		Definition: httpRequestSpec.Definition,
		Handler:    httpRequestHandler,
		Skills:     httpRequestSpec.Skills,
		MinTier:    httpRequestSpec.MinTier,
		Category:   httpRequestSpec.Category,
	})

	// Terminal tools — these stop the agentic loop on successful execution.
	// submit_work replaces [INTENT: work_product] tags.
	// ask_clarification replaces [INTENT: clarification] tags.
	r.Register(RegisteredTool{
		Definition: agentic.ToolDefinition{
			Name:        "submit_work",
			Description: "Submit your FINISHED work. Files you created/modified are captured automatically — provide a brief summary of what was delivered. This ends your quest — only call when all work is complete.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"summary": map[string]any{
						"type":        "string",
						"description": "Summary of what was delivered — describe the files created/modified, design decisions, and how to verify the work",
					},
					"deliverable": map[string]any{
						"type":        "string",
						"description": "Optional inline content for non-file work (analysis, research findings). Omit this when your work is in files — they are captured automatically.",
					},
				},
				"required": []any{},
			},
		},
		Handler:  submitWorkProductHandler,
		MinTier:  domain.TierApprentice, // All tiers can submit work
		Category: ToolCategoryCore,
	})

	r.Register(RegisteredTool{
		Definition: agentic.ToolDefinition{
			Name:        "ask_clarification",
			Description: "Ask the quest issuer a question when you need more information. Use this instead of submit_work when you have questions or are unsure how to proceed. You will NOT be penalized for asking questions — this is the correct way to request guidance.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"question": map[string]any{
						"type":        "string",
						"description": "Your question for the quest issuer",
					},
				},
				"required": []any{"question"},
			},
		},
		Handler:  askClarificationHandler,
		MinTier:  domain.TierApprentice, // All tiers can ask questions
		Category: ToolCategoryCore,
	})

	decomposeExec := questdagexec.NewDecomposeExecutor()
	for _, def := range decomposeExec.ListTools() {
		r.Register(RegisteredTool{
			Definition: def,
			Handler: func(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
				result, err := decomposeExec.Execute(ctx, call)
				if err != nil {
					return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("decompose_quest internal error: %v", err)}
				}
				// Stop the agentic loop after successful decomposition.
				// The DAG JSON in result.Content becomes the loop's final output,
				// which questbridge parses via extractDAGFromOutput.
				result.StopLoop = true
				return result
			},
			MinTier:  domain.TierMaster, // Level 16+ — only party leads (Master+) can decompose quests
			Category: ToolCategoryPartyLead,
		})
	}

	reviewExec := questdagexec.NewReviewExecutor()
	for _, def := range reviewExec.ListTools() {
		r.Register(RegisteredTool{
			Definition: def,
			Handler: func(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
				result, err := reviewExec.Execute(ctx, call)
				if err != nil {
					return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("review_sub_quest internal error: %v", err)}
				}
				// Stop the review agentic loop after the verdict.
				// The verdict JSON in result.Content is the loop's final output.
				result.StopLoop = true
				return result
			},
			MinTier:  domain.TierMaster, // Level 16+ — only party leads (Master+) can review sub-quests
			Category: ToolCategoryPartyLead,
		})
	}

	clarifyExec := questdagexec.NewClarificationExecutor()
	for _, def := range clarifyExec.ListTools() {
		r.Register(RegisteredTool{
			Definition: def,
			Handler: func(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
				result, err := clarifyExec.Execute(ctx, call)
				if err != nil {
					return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("answer_clarification internal error: %v", err)}
				}
				// Stop the clarification agentic loop after the answer.
				// The answer JSON in result.Content is the loop's final output.
				result.StopLoop = true
				return result
			},
			MinTier:  domain.TierMaster, // Level 16+ — only party leads (Master+) can answer clarifications
			Category: ToolCategoryPartyLead,
		})
	}

	// submit_findings is the terminal tool for explore sub-agents.
	// It works identically to submit_work but is scoped to read-only research loops.
	r.Register(RegisteredTool{
		Definition: agentic.ToolDefinition{
			Name:        "submit_findings",
			Description: "Submit your research findings. Call this when you have gathered sufficient information to answer the investigation goal.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"findings": map[string]any{
						"type":        "string",
						"description": "Structured research findings",
					},
					"sources": map[string]any{
						"type":        "array",
						"description": "Entity IDs, URLs, or file paths consulted",
						"items":       map[string]any{"type": "string"},
					},
				},
				"required": []any{"findings"},
			},
		},
		Handler: func(_ context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
			findings, _ := call.Arguments["findings"].(string)
			if findings == "" {
				return agentic.ToolResult{CallID: call.ID, Error: "findings argument is required and must be non-empty"}
			}
			return agentic.ToolResult{
				CallID:   call.ID,
				Content:  findings,
				StopLoop: true,
			}
		},
		MinTier:  domain.TierApprentice, // All tiers can submit findings
		Category: ToolCategoryCore,
	})
}

// RegisterExplore adds the explore tool to the registry.
// The tool definition is registered here; actual execution is intercepted by
// questtools.handleExplore before reaching the registry's Execute method.
func (r *ToolRegistry) RegisterExplore() {
	r.Register(RegisteredTool{
		Definition: agentic.ToolDefinition{
			Name:        "explore",
			Description: "Spawn a focused sub-agent to investigate a topic using read-only tools (graph search, web search, file browsing). Use for complex multi-step discovery that requires several lookups. For single lookups, use graph_search or graph_query directly.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"goal": map[string]any{
						"type":        "string",
						"description": "What to investigate — be specific about what you need to find",
					},
					"context": map[string]any{
						"type":        "string",
						"description": "Additional context to help the explore agent (e.g. known entity IDs, file paths, or constraints)",
					},
				},
				"required": []any{"goal"},
			},
		},
		Handler: func(_ context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
			// Placeholder — actual execution is handled by questtools.handleExplore.
			// This handler is only reached if explore is called outside of questtools.
			return agentic.ToolResult{
				CallID: call.ID,
				Error:  "explore tool must be executed through questtools, not directly",
			}
		},
		MinTier:  domain.TierApprentice, // All tiers can use explore
		Category: ToolCategoryKnowledge,
	})
}

// GetExploreTools returns the read-only tool definitions for an explore sub-agent.
// Includes knowledge tools, network tools (read), and submit_findings.
// Excludes: bash (write), submit_work, ask_clarification, party lead tools, and explore itself.
func (r *ToolRegistry) GetExploreTools(agent *agentprogression.Agent) []agentic.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	exploreToolNames := map[string]bool{
		"graph_query":       true,
		"graph_search":      true,
		"graph_multi_query": true,
		"graph_summary":     true,
		"web_search":        true,
		"http_request":      true,
		"submit_findings":   true,
	}

	var tools []agentic.ToolDefinition
	for name, tool := range r.tools {
		if !exploreToolNames[name] {
			continue
		}
		if agent.Tier < tool.MinTier {
			continue
		}
		tools = append(tools, tool.Definition)
	}
	return tools
}

// =============================================================================
// BUILT-IN TOOL HANDLERS
// =============================================================================

// =============================================================================
// web_search handler
// =============================================================================

// makeWebSearchHandler returns a web_search handler backed by the given provider.
func makeWebSearchHandler(provider SearchProvider) ToolHandler {
	return func(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
		select {
		case <-ctx.Done():
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("operation cancelled: %v", ctx.Err())}
		default:
		}

		query, _ := call.Arguments["query"].(string)
		if query == "" {
			return agentic.ToolResult{CallID: call.ID, Error: "query argument is required"}
		}

		maxResults := 5
		if mr, ok := call.Arguments["max_results"].(float64); ok && mr > 0 {
			maxResults = int(mr)
		}

		reqCtx, cancel := context.WithTimeout(ctx, httpRequestTimeout)
		defer cancel()

		results, err := provider.Search(reqCtx, query, maxResults)
		if err != nil {
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("web search failed: %v", err)}
		}

		return agentic.ToolResult{
			CallID:  call.ID,
			Content: formatSearchResults(results, query),
		}
	}
}

// =============================================================================
// TERMINAL TOOL HANDLERS (submit_work, ask_clarification)
// =============================================================================

func submitWorkProductHandler(_ context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
	summary, _ := call.Arguments["summary"].(string)
	deliverable, _ := call.Arguments["deliverable"].(string)

	if summary == "" && deliverable == "" {
		return agentic.ToolResult{CallID: call.ID, Error: "at least one of summary or deliverable is required"}
	}

	// When deliverable is provided, check if it's actually a question.
	if deliverable != "" && looksLikeQuestion(deliverable) {
		return agentic.ToolResult{
			CallID: call.ID,
			Error: "Your deliverable appears to be a question or request for information, not completed work. " +
				"Use the ask_clarification tool instead — you will NOT be penalized for asking questions. " +
				"Only use submit_work when you have finished work to submit.",
		}
	}

	// Also check summary-only submissions for question patterns.
	if deliverable == "" && summary != "" && looksLikeQuestion(summary) {
		return agentic.ToolResult{
			CallID: call.ID,
			Error: "Your summary appears to be a question. " +
				"Use the ask_clarification tool instead — you will NOT be penalized for asking questions.",
		}
	}

	result := map[string]string{
		"type": "work_product",
	}
	if deliverable != "" {
		result["deliverable"] = deliverable
	}
	if summary != "" {
		result["summary"] = summary
	}

	jsonBytes, _ := json.Marshal(result)
	return agentic.ToolResult{
		CallID:   call.ID,
		Content:  string(jsonBytes),
		StopLoop: true,
	}
}

// looksLikeQuestion detects deliverables that are actually questions or requests
// for information rather than completed work. This catches agents (especially
// smaller models) that use submit_work instead of ask_clarification.
func looksLikeQuestion(text string) bool {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) == 0 {
		return false
	}

	lower := strings.ToLower(trimmed)

	// If it contains code fences, it's probably real work even if it also has questions.
	if strings.Contains(lower, "```") {
		return false
	}

	// Short submissions with question marks are likely questions, not work.
	hasQuestion := strings.Contains(trimmed, "?")
	isShort := len(trimmed) < 2000

	if !hasQuestion || !isShort {
		return false
	}

	// Check for question-asking phrases that indicate this isn't work output.
	questionPhrases := []string{
		"could you provide",
		"could you clarify",
		"can you provide",
		"can you clarify",
		"can you tell me",
		"i need more information",
		"i need to know",
		"i need clarification",
		"what is the",
		"what are the",
		"where is the",
		"where can i find",
		"how should i",
		"how do i",
		"please provide",
		"please clarify",
		"before i can proceed",
		"before proceeding",
		"i have some questions",
		"i have a few questions",
		"the following questions",
	}
	for _, phrase := range questionPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}

	// Fallback: if majority of non-empty lines end with "?", it's a question.
	lines := strings.Split(trimmed, "\n")
	var nonEmpty, questions int
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		nonEmpty++
		if strings.HasSuffix(line, "?") {
			questions++
		}
	}
	return nonEmpty > 0 && float64(questions)/float64(nonEmpty) > 0.5
}

func askClarificationHandler(_ context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
	question, _ := call.Arguments["question"].(string)
	if question == "" {
		return agentic.ToolResult{CallID: call.ID, Error: "question argument is required and must be non-empty"}
	}

	jsonBytes, _ := json.Marshal(map[string]string{
		"type":     "clarification",
		"question": question,
	})
	return agentic.ToolResult{
		CallID:   call.ID,
		Content:  string(jsonBytes),
		StopLoop: true,
	}
}

// isPrivateIP returns true if the IP is in a private, loopback, or link-local range.
func isPrivateIP(ip net.IP) bool {
	privateRanges := []string{
		"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
		"169.254.0.0/16", "127.0.0.0/8",
		"::1/128", "fc00::/7", "fe80::/10",
	}
	for _, cidr := range privateRanges {
		_, network, _ := net.ParseCIDR(cidr)
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// httpToolClient is a dedicated HTTP client for agent tool requests.
// It blocks connections to private/loopback IPs (SSRF prevention),
// limits redirects, and sets a hard timeout.
var httpToolClient = &http.Client{
	Timeout: httpRequestTimeout,
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("invalid address: %w", err)
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("DNS lookup failed: %w", err)
			}
			for _, ip := range ips {
				if isPrivateIP(ip.IP) {
					return nil, fmt.Errorf("requests to private/internal IPs are blocked (resolved %s to %s)", host, ip.IP)
				}
			}
			return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, net.JoinHostPort(host, port))
		},
	},
	CheckRedirect: func(_ *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("stopped after 5 redirects")
		}
		return nil
	},
}

func httpRequestHandler(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
	select {
	case <-ctx.Done():
		return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("operation cancelled: %v", ctx.Err())}
	default:
	}

	urlStr, _ := call.Arguments["url"].(string)
	if urlStr == "" {
		return agentic.ToolResult{CallID: call.ID, Error: "url argument is required"}
	}
	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		return agentic.ToolResult{CallID: call.ID, Error: "url must start with http:// or https://"}
	}

	method, _ := call.Arguments["method"].(string)
	if method == "" {
		method = "GET"
	}
	if method != "GET" && method != "POST" {
		return agentic.ToolResult{CallID: call.ID, Error: "method must be GET or POST"}
	}

	var reqBody io.Reader
	if body, ok := call.Arguments["body"].(string); ok && body != "" {
		reqBody = strings.NewReader(body)
	}

	reqCtx, cancel := context.WithTimeout(ctx, httpRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, method, urlStr, reqBody)
	if err != nil {
		return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("failed to create request: %v", err)}
	}

	req.Header.Set("User-Agent", "semdragons-agent/1.0")

	if method == "POST" {
		contentType, _ := call.Arguments["content_type"].(string)
		if contentType == "" {
			contentType = "application/json"
		}
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := httpToolClient.Do(req)
	if err != nil {
		return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("request failed: %v", err)}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxHTTPResponseSize+1))
	if err != nil {
		return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("failed to read response: %v", err)}
	}

	contentType := resp.Header.Get("Content-Type")
	isHTML := strings.Contains(contentType, "text/html")

	var result string
	var truncMsg string

	if isHTML {
		title := extractTitle(bytes.NewReader(body))
		text, wasTruncated := htmlToText(bytes.NewReader(body), httpTextMaxSize)
		result = text
		if wasTruncated {
			truncMsg = fmt.Sprintf("\n... (converted from HTML, truncated at %d chars)", httpTextMaxSize)
		}

		// Persist to the knowledge graph if the page meets the quality threshold.
		if httpGraphPersist != nil && len(text) >= minHTTPPersistLength {
			go persistWebContent(httpGraphPersist, urlStr, title, text, call)
		}
	} else {
		result = string(body)
		if len(body) > maxHTTPResponseSize {
			result = result[:maxHTTPResponseSize]
			truncMsg = "\n... (response truncated)"
		}
	}

	return agentic.ToolResult{
		CallID:  call.ID,
		Content: fmt.Sprintf("HTTP %d %s\n\n%s%s", resp.StatusCode, resp.Status, result, truncMsg),
	}
}

// webContentEntity is a thin Graphable wrapper for a fetched web document.
// It lets GraphClient.EmitEntity marshal and store the web content triples
// using the standard entity-state pipeline.
type webContentEntity struct {
	id      string
	triples []message.Triple
}

func (e *webContentEntity) EntityID() string          { return e.id }
func (e *webContentEntity) Triples() []message.Triple { return e.triples }

// persistWebContent writes a fetched web page to the knowledge graph as a
// source.doc entity. This runs in a goroutine; failures are logged and ignored
// so they never block or surface to the agent.
func persistWebContent(p *graphPersister, rawURL, title, content string, call agentic.ToolCall) {
	urlHash := fmt.Sprintf("%x", sha256.Sum256([]byte(rawURL)))[:6]

	effectiveTitle := title
	if effectiveTitle == "" {
		if u, err := url.Parse(rawURL); err == nil {
			effectiveTitle = u.Host + u.Path
		} else {
			effectiveTitle = rawURL
		}
	}

	slug := slugify(effectiveTitle, 40)
	instance := slug + "-" + urlHash

	entityID := fmt.Sprintf("%s.%s.web.agent.doc.%s",
		p.config.Org, p.config.Platform, instance)

	summary := content
	if len(summary) > 200 {
		summary = summary[:200] + "..."
	}

	agentID, _ := call.Metadata["agent_id"].(string)
	questID, _ := call.Metadata["quest_id"].(string)

	triples := []message.Triple{
		{Subject: entityID, Predicate: "source.doc.content", Object: content},
		{Subject: entityID, Predicate: "source.doc.summary", Object: summary},
		{Subject: entityID, Predicate: "source.doc.url", Object: rawURL},
		{Subject: entityID, Predicate: "source.doc.mime_type", Object: "text/html"},
		{Subject: entityID, Predicate: "source.doc.scope", Object: "all"},
		{Subject: entityID, Predicate: "dc.terms.title", Object: effectiveTitle},
		{Subject: entityID, Predicate: "dc.terms.modified", Object: time.Now().Format(time.RFC3339)},
	}
	if agentID != "" {
		triples = append(triples, message.Triple{Subject: entityID, Predicate: "source.doc.fetched_by", Object: agentID})
	}
	if questID != "" {
		triples = append(triples, message.Triple{Subject: entityID, Predicate: "source.doc.quest_id", Object: questID})
	}

	entity := &webContentEntity{id: entityID, triples: triples}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.graph.EmitEntity(ctx, entity, "web.content.fetched"); err != nil {
		slog.Debug("failed to persist web content to graph",
			"url", rawURL, "entity_id", entityID, "error", err)
	}
}

// cappedWriter limits how many bytes are buffered in memory.
// Once the cap is reached, further writes are silently discarded.
type cappedWriter struct {
	buf bytes.Buffer
	max int
}

func (w *cappedWriter) Write(p []byte) (int, error) {
	remaining := w.max - w.buf.Len()
	if remaining <= 0 {
		return len(p), nil // discard but report success to avoid breaking the process
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	return w.buf.Write(p)
}

func (w *cappedWriter) String() string { return w.buf.String() }
func (w *cappedWriter) Len() int       { return w.buf.Len() }

// runShellCommand executes a shell command in the sandbox directory.
func runShellCommand(ctx context.Context, call agentic.ToolCall, timeout time.Duration) agentic.ToolResult {
	select {
	case <-ctx.Done():
		return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("operation cancelled: %v", ctx.Err())}
	default:
	}

	command, _ := call.Arguments["command"].(string)
	if command == "" {
		return agentic.ToolResult{CallID: call.ID, Error: "command argument is required"}
	}

	sandboxDir := getSandboxDir(call)
	if sandboxDir == "" {
		return agentic.ToolResult{CallID: call.ID, Error: "shell commands require a configured sandbox directory"}
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "sh", "-c", command)
	cmd.Dir = sandboxDir
	// Clean environment — only pass through PATH and HOME for basic operation.
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
	}

	stdout := &cappedWriter{max: maxCommandOutput}
	stderr := &cappedWriter{max: maxCommandOutput}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()

	var result strings.Builder
	if stdout.Len() > 0 {
		result.WriteString(stdout.String())
		if stdout.Len() >= maxCommandOutput {
			result.WriteString("\n... (stdout truncated)")
		}
	}
	if stderr.Len() > 0 {
		if result.Len() > 0 {
			result.WriteString("\n\n--- stderr ---\n")
		}
		result.WriteString(stderr.String())
		if stderr.Len() >= maxCommandOutput {
			result.WriteString("\n... (stderr truncated)")
		}
	}

	if err != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			return agentic.ToolResult{
				CallID:  call.ID,
				Content: result.String(),
				Error:   fmt.Sprintf("command timed out after %s", timeout),
			}
		}
		return agentic.ToolResult{
			CallID:  call.ID,
			Content: result.String(),
			Error:   fmt.Sprintf("command failed: %v", err),
		}
	}

	if result.Len() == 0 {
		result.WriteString("(no output)")
	}

	return agentic.ToolResult{
		CallID:  call.ID,
		Content: result.String(),
	}
}

func runCommandHandler(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
	return runShellCommand(ctx, call, commandTimeout)
}

// =============================================================================
// GRAPH QUERY TOOL
// =============================================================================

// EntityQueryFunc queries entities by type and returns a formatted text summary.
// The limit parameter caps the number of entities returned.
type EntityQueryFunc func(ctx context.Context, entityType string, limit int) (string, error)

// FormatEntitySummary formats a slice of EntityState into a compact text summary
// suitable for returning to agents. Shared by both executor and questtools components.
func FormatEntitySummary(entities []graph.EntityState, entityType string) string {
	if len(entities) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Found %d %s(s):\n\n", len(entities), entityType))
	for _, entity := range entities {
		b.WriteString(fmt.Sprintf("--- %s ---\n", entity.ID))
		tripleMap := make(map[string]any, len(entity.Triples))
		for _, t := range entity.Triples {
			tripleMap[t.Predicate] = t.Object
		}
		data, err := json.Marshal(tripleMap)
		if err != nil {
			b.WriteString("  (failed to serialize)\n")
			continue
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	return b.String()
}

// RegisterGraphQuery adds the graph_query tool to the registry.
// The queryFn is called at execution time to fetch entity data — typically
// backed by GraphClient.ListEntitiesByType in the calling component.
func (r *ToolRegistry) RegisterGraphQuery(queryFn EntityQueryFunc) {
	r.Register(RegisteredTool{
		Definition: agentic.ToolDefinition{
			Name:        "graph_query",
			Description: "Query the entity graph for agents, quests, guilds, parties, or battles. Returns a summary of matching entities.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"entity_type": map[string]any{
						"type":        "string",
						"description": "The type of entity to query",
						"enum":        []any{"quest", "agent", "guild", "party", "battle"},
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum number of entities to return (default 20)",
					},
				},
				"required": []any{"entity_type"},
			},
		},
		Handler:  graphQueryHandler(queryFn),
		MinTier:  domain.TierApprentice, // Level 1+ — read-only graph access
		Category: ToolCategoryKnowledge,
	})
}

// graphQLTimeout is the timeout for graph-gateway GraphQL requests.
const graphQLTimeout = 30 * time.Second

// =============================================================================
// WEB SEARCH TOOL (conditional registration)
// =============================================================================

// RegisterWebSearch adds the web_search tool to the registry backed by the
// given SearchProvider. Call this only when a search provider is configured.
func (r *ToolRegistry) RegisterWebSearch(provider SearchProvider) {
	handler := makeWebSearchHandler(provider)
	r.Register(RegisteredTool{
		Definition: agentic.ToolDefinition{
			Name:        "web_search",
			Description: "Search the web and return results.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "The search query",
					},
					"max_results": map[string]any{
						"type":        "integer",
						"description": "Maximum number of results to return (default 5, max 10)",
					},
				},
				"required": []any{"query"},
			},
		},
		Handler:  handler,
		Skills:   []domain.SkillTag{domain.SkillResearch},
		MinTier:  domain.TierApprentice,
		Category: ToolCategoryNetwork,
	})
}

// =============================================================================
// GRAPH SEARCH TOOL
// =============================================================================

// GraphSearchRouter routes graph_search queries to the appropriate graph-gateway(s).
// Implementations may fan out to multiple sources (local + semsource instances)
// and merge results.
type GraphSearchRouter interface {
	// GraphQLURLsForQuery returns the GraphQL endpoint URLs to query for a given
	// query type and optional entity ID or prefix.
	GraphQLURLsForQuery(queryType, entityID, prefix string) []string
}

// singleURLRouter is a backward-compatible router that always returns one URL.
type singleURLRouter struct{ url string }

func (r *singleURLRouter) GraphQLURLsForQuery(_, _, _ string) []string {
	return []string{r.url}
}

// RegisterGraphSearch adds the graph_search tool to the registry.
// graphqlURL is the graph-gateway GraphQL endpoint (e.g. "http://localhost:8082/graphql").
// For multi-source routing, use RegisterGraphSearchWithRouter instead.
func (r *ToolRegistry) RegisterGraphSearch(graphqlURL string) {
	r.RegisterGraphSearchWithRouter(&singleURLRouter{url: graphqlURL})
}

// RegisterGraphSearchWithRouter adds graph_search with multi-source routing.
// The router determines which graph-gateway(s) to query based on query type and entity prefix.
func (r *ToolRegistry) RegisterGraphSearchWithRouter(router GraphSearchRouter) {
	r.Register(RegisteredTool{
		Definition: agentic.ToolDefinition{
			Name:        "graph_search",
			Description: "Search the knowledge graph via GraphQL. Supports entity lookup, relationship traversal, predicate queries, full-text search, and natural language queries across all entities including source documentation and code.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query_type": map[string]any{
						"type":        "string",
						"description": "Type of graph query to execute. Use 'nlq' for natural language questions about the codebase (e.g. 'what interfaces does an OSH sensor driver implement?'). Use 'search' for keyword matching. Use 'prefix' or 'predicate' for structured lookups.",
						"enum":        []any{"entity", "prefix", "predicate", "relationships", "search", "nlq"},
					},
					"entity_id": map[string]any{
						"type":        "string",
						"description": "Entity ID for entity/relationships queries",
					},
					"prefix": map[string]any{
						"type":        "string",
						"description": "Entity ID prefix for prefix queries (e.g. 'c360.semsource.git')",
					},
					"predicate": map[string]any{
						"type":        "string",
						"description": "Predicate name for predicate queries (e.g. 'source.content.language')",
					},
					"search_text": map[string]any{
						"type":        "string",
						"description": "Search text for full-text search queries",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum results to return (default 20, max 100)",
					},
				},
				"required": []any{"query_type"},
			},
		},
		Handler:  graphSearchHandler(router),
		MinTier:  domain.TierApprentice, // Read-only knowledge-graph access
		Category: ToolCategoryKnowledge,
	})
}

// NewSingleURLRouter returns a GraphSearchRouter that always routes to a single
// GraphQL endpoint. Use this when only one graph-gateway URL is available.
func NewSingleURLRouter(graphqlURL string) GraphSearchRouter {
	return &singleURLRouter{url: graphqlURL}
}

// RegisterGraphMultiQuery adds the graph_multi_query tool to the registry.
// It shares the same router as graph_search so multi-source routing applies
// to every sub-query in a batch.
func (r *ToolRegistry) RegisterGraphMultiQuery(router GraphSearchRouter) {
	r.Register(RegisteredTool{
		Definition: agentic.ToolDefinition{
			Name:        "graph_multi_query",
			Description: "Execute multiple graph queries in a single call. Use when you need to look up several entities, prefixes, or relationships at once. Each query uses the same parameters as graph_search. Maximum 5 queries per call.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"queries": map[string]any{
						"type":        "array",
						"description": "Array of graph queries to execute (max 5)",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"query_type": map[string]any{
									"type": "string",
									"enum": []any{"entity", "prefix", "predicate", "relationships", "search", "nlq"},
								},
								"entity_id":   map[string]any{"type": "string"},
								"prefix":      map[string]any{"type": "string"},
								"predicate":   map[string]any{"type": "string"},
								"search_text": map[string]any{"type": "string"},
								"limit":       map[string]any{"type": "integer"},
							},
							"required": []any{"query_type"},
						},
					},
				},
				"required": []any{"queries"},
			},
		},
		Handler:  graphMultiQueryHandler(router),
		MinTier:  domain.TierApprentice,
		Category: ToolCategoryKnowledge,
	})
}

// graphMultiQueryHandler returns a ToolHandler that executes a batch of graph
// queries and combines their results under labeled headings.
func graphMultiQueryHandler(router GraphSearchRouter) ToolHandler {
	client := &http.Client{Timeout: graphQLTimeout}

	return func(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
		select {
		case <-ctx.Done():
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("operation cancelled: %v", ctx.Err())}
		default:
		}

		// Parse the queries array from arguments.
		rawQueries, ok := call.Arguments["queries"]
		if !ok {
			return agentic.ToolResult{CallID: call.ID, Error: "queries argument is required"}
		}
		queriesSlice, ok := rawQueries.([]any)
		if !ok {
			return agentic.ToolResult{CallID: call.ID, Error: "queries must be an array"}
		}
		if len(queriesSlice) == 0 {
			return agentic.ToolResult{CallID: call.ID, Error: "queries array must contain at least one query"}
		}
		if len(queriesSlice) > 5 {
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("too many queries: %d (maximum is 5)", len(queriesSlice))}
		}

		var combined strings.Builder
		totalBytes := 0

		for i, rawQuery := range queriesSlice {
			queryArgs, ok := rawQuery.(map[string]any)
			if !ok {
				combined.WriteString(fmt.Sprintf("## Query %d\nError: query must be an object\n\n", i+1))
				continue
			}

			queryType, _ := queryArgs["query_type"].(string)
			if queryType == "" {
				combined.WriteString(fmt.Sprintf("## Query %d\nError: query_type is required\n\n", i+1))
				continue
			}

			// Build a human-readable label that identifies the query key parameter.
			label := queryType
			switch queryType {
			case "entity", "relationships":
				if id, _ := queryArgs["entity_id"].(string); id != "" {
					label = fmt.Sprintf("%s(%s)", queryType, id)
				}
			case "prefix":
				if p, _ := queryArgs["prefix"].(string); p != "" {
					label = fmt.Sprintf("prefix(%s)", p)
				}
			case "predicate":
				if p, _ := queryArgs["predicate"].(string); p != "" {
					label = fmt.Sprintf("predicate(%s)", p)
				}
			case "search", "nlq":
				if t, _ := queryArgs["search_text"].(string); t != "" {
					if len(t) > 40 {
						t = t[:40] + "..."
					}
					label = fmt.Sprintf("%s(%s)", queryType, t)
				}
			}

			limit := 20
			if l, ok := queryArgs["limit"].(float64); ok && l > 0 {
				limit = min(int(l), 100)
			}

			gqlReq, err := buildGraphSearchQuery(queryType, limit, queryArgs)
			if err != nil {
				combined.WriteString(fmt.Sprintf("## Query %d: %s\nError: %s\n\n", i+1, label, err.Error()))
				continue
			}

			entityID, _ := queryArgs["entity_id"].(string)
			prefix, _ := queryArgs["prefix"].(string)
			urls := router.GraphQLURLsForQuery(queryType, entityID, prefix)
			if len(urls) == 0 {
				combined.WriteString(fmt.Sprintf("## Query %d: %s\nNo graph sources available for this query.\n\n", i+1, label))
				continue
			}

			reqCtx, cancel := context.WithTimeout(ctx, graphQLTimeout)
			var queryResult string

			if len(urls) == 1 {
				queryResult, err = executeGraphQLQuery(reqCtx, client, urls[0], gqlReq)
				cancel()
				if err != nil {
					combined.WriteString(fmt.Sprintf("## Query %d: %s\nError: %s\n\n", i+1, label, err.Error()))
					continue
				}
			} else {
				// Fan out to multiple sources and merge.
				var parts []string
				for _, u := range urls {
					res, ferr := executeGraphQLQuery(reqCtx, client, u, gqlReq)
					if ferr == nil && res != "" {
						parts = append(parts, res)
					}
				}
				cancel()
				if len(parts) == 0 {
					combined.WriteString(fmt.Sprintf("## Query %d: %s\nNo results from any source.\n\n", i+1, label))
					continue
				}
				queryResult = strings.Join(parts, "\n\n---\n\n")
			}

			section := fmt.Sprintf("## Query %d: %s\n%s\n\n", i+1, label, queryResult)
			totalBytes += len(section)
			if totalBytes > maxGraphResponseSize {
				combined.WriteString(fmt.Sprintf("## Query %d: %s\nResponse truncated — combined results exceeded %d bytes. Run remaining queries individually.\n\n",
					i+1, label, maxGraphResponseSize))
				break
			}
			combined.WriteString(section)
		}

		return agentic.ToolResult{CallID: call.ID, Content: strings.TrimRight(combined.String(), "\n")}
	}
}

// graphQLRequest is the JSON body sent to a GraphQL endpoint.
type graphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

// graphQLResponse is the top-level envelope returned by a GraphQL endpoint.
type graphQLResponse struct {
	Data   map[string]json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// graphSearchHandler returns a ToolHandler that dispatches GraphQL queries to
// one or more graph-gateway endpoints via the router.
func graphSearchHandler(router GraphSearchRouter) ToolHandler {
	client := &http.Client{Timeout: graphQLTimeout}

	return func(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
		select {
		case <-ctx.Done():
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("operation cancelled: %v", ctx.Err())}
		default:
		}

		queryType, _ := call.Arguments["query_type"].(string)
		if queryType == "" {
			return agentic.ToolResult{CallID: call.ID, Error: "query_type argument is required"}
		}

		limit := 20
		if l, ok := call.Arguments["limit"].(float64); ok && l > 0 {
			limit = min(int(l), 100)
		}

		gqlReq, err := buildGraphSearchQuery(queryType, limit, call.Arguments)
		if err != nil {
			return agentic.ToolResult{CallID: call.ID, Error: err.Error()}
		}

		// Route to the appropriate graph-gateway(s).
		entityID, _ := call.Arguments["entity_id"].(string)
		prefix, _ := call.Arguments["prefix"].(string)
		urls := router.GraphQLURLsForQuery(queryType, entityID, prefix)
		if len(urls) == 0 {
			return agentic.ToolResult{CallID: call.ID, Error: "no graph sources available for this query"}
		}

		reqCtx, cancel := context.WithTimeout(ctx, graphQLTimeout)
		defer cancel()

		// Single source: direct query (most common path).
		if len(urls) == 1 {
			result, err := executeGraphQLQuery(reqCtx, client, urls[0], gqlReq)
			if err != nil {
				return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("graph search failed: %v", err)}
			}
			return agentic.ToolResult{CallID: call.ID, Content: result}
		}

		// Multiple sources: fan out, merge results.
		var allResults []string
		for _, url := range urls {
			result, err := executeGraphQLQuery(reqCtx, client, url, gqlReq)
			if err != nil {
				// Log but don't fail — other sources may succeed.
				continue
			}
			if result != "" {
				allResults = append(allResults, result)
			}
		}

		if len(allResults) == 0 {
			return agentic.ToolResult{CallID: call.ID, Error: "graph search returned no results from any source"}
		}

		return agentic.ToolResult{CallID: call.ID, Content: strings.Join(allResults, "\n\n---\n\n")}
	}
}

// buildGraphSearchQuery constructs an inline GraphQL request for the given query_type.
// Uses inline arguments for simplicity and compatibility across graph-gateway versions.
// All string values are escaped via sanitizeGraphQLString to prevent injection.
func buildGraphSearchQuery(queryType string, limit int, args map[string]any) (graphQLRequest, error) {
	switch queryType {
	case "entity":
		id, _ := args["entity_id"].(string)
		if id == "" {
			return graphQLRequest{}, fmt.Errorf("entity_id is required for entity queries")
		}
		return graphQLRequest{
			Query: fmt.Sprintf(`{ entity(id: %q) { id triples { predicate object } } }`, sanitizeGraphQLString(id)),
		}, nil

	case "prefix":
		prefix, _ := args["prefix"].(string)
		if prefix == "" {
			return graphQLRequest{}, fmt.Errorf("prefix is required for prefix queries")
		}
		return graphQLRequest{
			Query: fmt.Sprintf(`{ entitiesByPrefix(prefix: %q, limit: %d) { id type } }`, sanitizeGraphQLString(prefix), limit),
		}, nil

	case "predicate":
		predicate, _ := args["predicate"].(string)
		if predicate == "" {
			return graphQLRequest{}, fmt.Errorf("predicate is required for predicate queries")
		}
		return graphQLRequest{
			Query: fmt.Sprintf(`{ entitiesByPredicate(predicate: %q, limit: %d) { id type } }`, sanitizeGraphQLString(predicate), limit),
		}, nil

	case "relationships":
		id, _ := args["entity_id"].(string)
		if id == "" {
			return graphQLRequest{}, fmt.Errorf("entity_id is required for relationships queries")
		}
		return graphQLRequest{
			Query: fmt.Sprintf(`{ relationships(entityId: %q) { from to predicate } }`, sanitizeGraphQLString(id)),
		}, nil

	case "search":
		text, _ := args["search_text"].(string)
		if text == "" {
			return graphQLRequest{}, fmt.Errorf("search_text is required for search queries")
		}
		maxCommunities := min(limit, 5)
		return graphQLRequest{
			Query: fmt.Sprintf(`{ globalSearch(query: %q, maxCommunities: %d) { answer answer_model entity_digests { id type label relevance } community_summaries { communityId summary keywords relevance member_count entities { id type label relevance } } entities { id type } count } }`,
				sanitizeGraphQLString(text), maxCommunities),
		}, nil

	case "nlq":
		text, _ := args["search_text"].(string)
		if text == "" {
			return graphQLRequest{}, fmt.Errorf("search_text is required for nlq queries (your natural language question)")
		}
		// NLQ uses globalSearch with answer synthesis — the answer field is the
		// primary output. Community summaries and entity digests provide follow-up
		// context for targeted queries.
		maxCommunities := min(limit, 3)
		maxEntities := min(limit, 10)
		return graphQLRequest{
			Query: fmt.Sprintf(`{ globalSearch(query: %q, level: 1, maxCommunities: %d, maxEntities: %d) { answer answer_model entity_digests { id type label relevance } community_summaries { communityId summary keywords relevance member_count entities { id type label relevance } } entities { id type } count classification { queryType confidence } } }`,
				sanitizeGraphQLString(text), maxCommunities, maxEntities),
		}, nil

	default:
		return graphQLRequest{}, fmt.Errorf("invalid query_type: %q (must be one of entity, prefix, predicate, relationships, search, nlq)", queryType)
	}
}

// sanitizeGraphQLString removes characters that could break out of a GraphQL
// string literal. Entity IDs and search text are the only user-supplied strings
// — they should contain only alphanumerics, dots, hyphens, underscores, and spaces.
func sanitizeGraphQLString(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '"' || r == '\\' || r == '\n' || r == '\r':
			// Skip characters that could break GraphQL string boundaries
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// executeGraphQLQuery POSTs a GraphQL request to the endpoint and returns a formatted text summary.
func executeGraphQLQuery(ctx context.Context, client *http.Client, graphqlURL string, gqlReq graphQLRequest) (string, error) {
	body, err := json.Marshal(gqlReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, graphqlURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxGraphResponseSize+1))
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		snippet := string(respBody)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, snippet)
	}

	if len(respBody) == 0 {
		return "No results found. The graph may not have data for this query yet.", nil
	}

	// If the response was truncated by the size limit, we can't parse it as
	// JSON. Return a descriptive content message rather than a hard error so
	// the agent can continue with other tools.
	if len(respBody) > maxGraphResponseSize {
		return "Graph query returned a very large response that was truncated. Try a more specific query (e.g., narrower prefix, specific entity_id, or add a limit).", nil
	}

	var gqlResp graphQLResponse
	if err := json.Unmarshal(respBody, &gqlResp); err != nil {
		return "", fmt.Errorf("failed to parse response (body length %d): %w", len(respBody), err)
	}

	if len(gqlResp.Errors) > 0 {
		msgs := make([]string, 0, len(gqlResp.Errors))
		for _, e := range gqlResp.Errors {
			msgs = append(msgs, e.Message)
		}
		return "", fmt.Errorf("GraphQL errors: %s", strings.Join(msgs, "; "))
	}

	return formatGraphSearchResult(gqlResp.Data), nil
}

// formatGraphSearchResult converts the GraphQL data map into a human-readable
// text summary suitable for inclusion in an agent's tool result.
func formatGraphSearchResult(data map[string]json.RawMessage) string {
	if len(data) == 0 {
		return "No results found."
	}

	var b strings.Builder

	// entity query: single entity with triples.
	if raw, ok := data["entity"]; ok {
		var entity struct {
			ID      string `json:"id"`
			Triples []struct {
				Predicate string `json:"predicate"`
				Object    string `json:"object"`
			} `json:"triples"`
		}
		if err := json.Unmarshal(raw, &entity); err != nil || entity.ID == "" {
			b.WriteString("Entity not found.\n")
			return b.String()
		}
		b.WriteString(fmt.Sprintf("Entity: %s\n", entity.ID))
		if len(entity.Triples) == 0 {
			b.WriteString("  (no properties)\n")
		} else {
			b.WriteString(fmt.Sprintf("Properties (%d):\n", len(entity.Triples)))
			for _, t := range entity.Triples {
				// Show leaf predicate segment for readability
				pred := t.Predicate
				if idx := strings.LastIndex(pred, "."); idx >= 0 {
					pred = t.Predicate[idx+1:]
				}
				val := t.Object
				if len(val) > 200 {
					val = val[:200] + "..."
				}
				b.WriteString(fmt.Sprintf("  %s: %s\n", pred, val))
			}
		}
		return b.String()
	}

	// relationships query: list of from/to/predicate edges.
	if raw, ok := data["relationships"]; ok {
		var rels []struct {
			From      string `json:"from"`
			To        string `json:"to"`
			Predicate string `json:"predicate"`
		}
		if err := json.Unmarshal(raw, &rels); err != nil || len(rels) == 0 {
			b.WriteString("No relationships found.\n")
			return b.String()
		}
		b.WriteString(fmt.Sprintf("Relationships (%d):\n", len(rels)))
		for _, rel := range rels {
			b.WriteString(fmt.Sprintf("  %s -[%s]-> %s\n", rel.From, rel.Predicate, rel.To))
		}
		return b.String()
	}

	// globalSearch query: answer → community_summaries → entity_digests → entities.
	// Priority: use the highest-quality field available, skip lower ones.
	if raw, ok := data["globalSearch"]; ok {
		var result struct {
			Answer      string `json:"answer"`
			AnswerModel string `json:"answer_model"`
			Entities    []struct {
				ID   string `json:"id"`
				Type string `json:"type"`
			} `json:"entities"`
			EntityDigests []struct {
				ID        string  `json:"id"`
				Type      string  `json:"type"`
				Label     string  `json:"label"`
				Relevance float64 `json:"relevance"`
			} `json:"entity_digests"`
			CommunitySummaries []struct {
				CommunityID string  `json:"communityId"`
				Summary     string  `json:"summary"`
				Keywords    []string `json:"keywords"`
				Relevance   float64 `json:"relevance"`
				MemberCount int     `json:"member_count"`
				Entities    []struct {
					ID        string  `json:"id"`
					Type      string  `json:"type"`
					Label     string  `json:"label"`
					Relevance float64 `json:"relevance"`
				} `json:"entities"`
			} `json:"community_summaries"`
			Count          int `json:"count"`
			Classification struct {
				QueryType  string  `json:"queryType"`
				Confidence float64 `json:"confidence"`
			} `json:"classification"`
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			b.WriteString("No search results found.\n")
			return b.String()
		}

		// 1. Answer — synthesized natural language response. Use this first.
		if result.Answer != "" {
			b.WriteString(result.Answer)
			b.WriteString("\n")
		}

		// 2. Community summaries with representative entities.
		if len(result.CommunitySummaries) > 0 {
			if result.Answer == "" {
				// No answer — show community summaries as the primary content.
				for _, c := range result.CommunitySummaries {
					if c.Summary == "" {
						continue
					}
					b.WriteString(c.Summary)
					b.WriteString("\n")
				}
			}
			// Show representative entities from communities for follow-up.
			for _, c := range result.CommunitySummaries {
				if len(c.Entities) == 0 {
					continue
				}
				b.WriteString(fmt.Sprintf("\nKey entities (%d in cluster):\n", c.MemberCount))
				for _, e := range c.Entities {
					if e.Label != "" {
						b.WriteString(fmt.Sprintf("  %s [%s] — %s\n", e.Label, e.Type, e.ID))
					} else {
						b.WriteString(fmt.Sprintf("  [%s] %s\n", e.Type, e.ID))
					}
				}
			}
		}

		// 3. Entity digests — lightweight context for all matched entities.
		if len(result.EntityDigests) > 0 && result.Answer == "" && len(result.CommunitySummaries) == 0 {
			b.WriteString(fmt.Sprintf("Matched entities (%d total):\n", result.Count))
			for _, e := range result.EntityDigests {
				if e.Label != "" {
					b.WriteString(fmt.Sprintf("  %s [%s] — %s\n", e.Label, e.Type, e.ID))
				} else {
					b.WriteString(fmt.Sprintf("  [%s] %s\n", e.Type, e.ID))
				}
			}
		}

		// 4. Bare entity IDs — last resort when no higher-quality fields available.
		if result.Answer == "" && len(result.CommunitySummaries) == 0 && len(result.EntityDigests) == 0 {
			if len(result.Entities) > 0 {
				b.WriteString(fmt.Sprintf("Entities (%d total, showing %d):\n", result.Count, len(result.Entities)))
				for _, e := range result.Entities {
					b.WriteString(fmt.Sprintf("  [%s] %s\n", e.Type, e.ID))
				}
			} else {
				b.WriteString("No search results found.\n")
			}
		}

		return b.String()
	}

	// entitiesByPrefix / entitiesByPredicate: list of id+type pairs.
	for fieldName, raw := range data {
		var entities []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &entities); err != nil {
			b.WriteString(fmt.Sprintf("Failed to parse %s result.\n", fieldName))
			continue
		}
		if len(entities) == 0 {
			b.WriteString("No entities found.\n")
			continue
		}
		b.WriteString(fmt.Sprintf("Entities (%d):\n", len(entities)))
		for _, e := range entities {
			b.WriteString(fmt.Sprintf("  [%s] %s\n", e.Type, e.ID))
		}
	}

	return b.String()
}

// validGraphEntityTypes are the entity types permitted for graph_query.
var validGraphEntityTypes = map[string]bool{
	"quest": true, "agent": true, "guild": true, "party": true, "battle": true,
}

func graphQueryHandler(queryFn EntityQueryFunc) ToolHandler {
	return func(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
		select {
		case <-ctx.Done():
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("operation cancelled: %v", ctx.Err())}
		default:
		}

		entityType, _ := call.Arguments["entity_type"].(string)
		if entityType == "" {
			return agentic.ToolResult{CallID: call.ID, Error: "entity_type argument is required"}
		}
		if !validGraphEntityTypes[entityType] {
			return agentic.ToolResult{
				CallID: call.ID,
				Error:  fmt.Sprintf("invalid entity_type: %q (must be one of quest, agent, guild, party, battle)", entityType),
			}
		}

		limit := 20
		if l, ok := call.Arguments["limit"].(float64); ok && l > 0 {
			limit = min(int(l), 100)
		}

		result, err := queryFn(ctx, entityType, limit)
		if err != nil {
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("query failed: %v", err)}
		}

		if result == "" {
			return agentic.ToolResult{CallID: call.ID, Content: fmt.Sprintf("No %s entities found", entityType)}
		}

		return agentic.ToolResult{CallID: call.ID, Content: result}
	}
}


// =============================================================================
// GRAPH SUMMARY TOOL - semsource source discovery
// =============================================================================

// GraphSummaryRouter returns summary endpoint URLs for ready semsource sources
// and produces the formatted summary text for agent prompts.
type GraphSummaryRouter interface {
	SummaryURLs() []string
	FormattedSummary(ctx context.Context) string
}

// RegisterGraphSummary adds the graph_summary tool to the registry.
// The router provides the formatted summary text for ready knowledge sources.
func (r *ToolRegistry) RegisterGraphSummary(router GraphSummaryRouter) {
	r.Register(RegisteredTool{
		Definition: agentic.ToolDefinition{
			Name:        "graph_summary",
			Description: "Get an overview of what's indexed in the knowledge graph — sources, entity types, counts, and example entity IDs. Call this ONCE before graph_search to understand available data and how to scope queries by prefix.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		Handler:  graphSummaryHandler(router),
		MinTier:  domain.TierApprentice,
		Category: ToolCategoryKnowledge,
	})
}

// graphSummaryHandler returns a ToolHandler that returns the formatted knowledge
// graph summary text. Caching is handled by the router (GraphSourceRegistry).
func graphSummaryHandler(router GraphSummaryRouter) ToolHandler {
	return func(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
		select {
		case <-ctx.Done():
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("operation cancelled: %v", ctx.Err())}
		default:
		}

		text := router.FormattedSummary(ctx)
		if text == "" {
			return agentic.ToolResult{CallID: call.ID, Error: "no knowledge sources indexed yet"}
		}
		return agentic.ToolResult{CallID: call.ID, Content: text}
	}
}
