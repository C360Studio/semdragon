package semdragons

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/c360studio/semstreams/agentic"
)

// =============================================================================
// TOOL REGISTRY - Manages available tools for agent execution
// =============================================================================
// Tools are capabilities that agents can invoke during quest execution.
// The registry maps tool names to handlers and enforces trust/skill gates.
// =============================================================================

// Tool handler constants for limits and configuration.
const (
	// maxFileReadSize is the maximum bytes to return from a file read.
	maxFileReadSize = 100000
	// maxFileWriteSize is the maximum bytes allowed in a file write.
	maxFileWriteSize = 1 << 20 // 1MB
	// maxSearchMatches is the maximum number of search results to return.
	maxSearchMatches = 50
	// maxMatchLineLength is the maximum line length shown in search results.
	maxMatchLineLength = 200
)

// ToolHandler executes a tool and returns the result.
// The handler receives the tool call arguments and quest/agent context.
type ToolHandler func(ctx context.Context, call agentic.ToolCall, quest *Quest, agent *Agent) agentic.ToolResult

// RegisteredTool wraps a tool definition with its handler and access controls.
type RegisteredTool struct {
	Definition agentic.ToolDefinition // Name, description, parameters
	Handler    ToolHandler            // Execution function
	Skills     []SkillTag             // Required skills to use this tool
	MinTier    TrustTier              // Minimum trust tier to use
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

// GetToolsForQuest returns tool definitions the agent can use for this quest.
// Filters by:
// - Quest's AllowedTools list (if specified)
// - Agent's trust tier
// - Agent's skills
func (r *ToolRegistry) GetToolsForQuest(quest *Quest, agent *Agent) []agentic.ToolDefinition {
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
func (r *ToolRegistry) Execute(ctx context.Context, call agentic.ToolCall, quest *Quest, agent *Agent) agentic.ToolResult {
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
func agentHasAnySkill(agent *Agent, skills []SkillTag) bool {
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

// validatePath ensures a path is within the sandbox directory.
// Returns the cleaned absolute path if valid, or an error if the path escapes the sandbox.
func validatePath(path, sandboxDir string) (string, error) {
	// Clean and resolve to absolute path
	cleanPath := filepath.Clean(path)

	// If no sandbox, allow any path (but still clean it)
	if sandboxDir == "" {
		return cleanPath, nil
	}

	// Resolve both paths to absolute
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	absSandbox, err := filepath.Abs(sandboxDir)
	if err != nil {
		return "", fmt.Errorf("invalid sandbox directory: %w", err)
	}

	// Check if path is under sandbox using Rel
	rel, err := filepath.Rel(absSandbox, absPath)
	if err != nil {
		return "", fmt.Errorf("path validation failed: %w", err)
	}

	// If relative path starts with "..", it escapes the sandbox
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path escapes sandbox: %s", path)
	}

	return absPath, nil
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
		Definition: agentic.ToolDefinition{
			Name:        "read_file",
			Description: "Read the contents of a file from the filesystem",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "The file path to read",
					},
				},
				"required": []any{"path"},
			},
		},
		Handler: readFileHandler,
		Skills:  []SkillTag{SkillCodeGen, SkillResearch, SkillAnalysis},
		MinTier: TierJourneyman, // Level 6+ can read files
	})

	r.Register(RegisteredTool{
		Definition: agentic.ToolDefinition{
			Name:        "write_file",
			Description: "Write content to a file on the filesystem",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "The file path to write to",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "The content to write to the file",
					},
				},
				"required": []any{"path", "content"},
			},
		},
		Handler: writeFileHandler,
		Skills:  []SkillTag{SkillCodeGen},
		MinTier: TierExpert, // Level 11+ can write files (production capability)
	})

	r.Register(RegisteredTool{
		Definition: agentic.ToolDefinition{
			Name:        "list_directory",
			Description: "List the contents of a directory",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "The directory path to list",
					},
				},
				"required": []any{"path"},
			},
		},
		Handler: listDirectoryHandler,
		Skills:  []SkillTag{SkillCodeGen, SkillResearch, SkillAnalysis},
		MinTier: TierJourneyman,
	})

	r.Register(RegisteredTool{
		Definition: agentic.ToolDefinition{
			Name:        "search_text",
			Description: "Search for text patterns in files",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{
						"type":        "string",
						"description": "The text pattern to search for",
					},
					"path": map[string]any{
						"type":        "string",
						"description": "The file or directory to search in",
					},
				},
				"required": []any{"pattern", "path"},
			},
		},
		Handler: searchTextHandler,
		Skills:  []SkillTag{SkillCodeGen, SkillResearch, SkillAnalysis},
		MinTier: TierJourneyman,
	})
}

// --- Built-in tool handlers ---

func readFileHandler(ctx context.Context, call agentic.ToolCall, _ *Quest, _ *Agent) agentic.ToolResult {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  fmt.Sprintf("operation cancelled: %v", ctx.Err()),
		}
	default:
	}

	path, ok := call.Arguments["path"].(string)
	if !ok {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  "path argument must be a string",
		}
	}

	// Validate path is within sandbox
	sandboxDir := getSandboxDir(call)
	cleanPath, err := validatePath(path, sandboxDir)
	if err != nil {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  err.Error(),
		}
	}

	content, err := os.ReadFile(cleanPath)
	if err != nil {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  fmt.Sprintf("failed to read file: %v", err),
		}
	}

	// Limit content size to prevent huge outputs
	result := string(content)
	if len(result) > maxFileReadSize {
		result = result[:maxFileReadSize] + "\n... (truncated)"
	}

	return agentic.ToolResult{
		CallID:  call.ID,
		Content: result,
	}
}

func writeFileHandler(ctx context.Context, call agentic.ToolCall, _ *Quest, _ *Agent) agentic.ToolResult {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  fmt.Sprintf("operation cancelled: %v", ctx.Err()),
		}
	default:
	}

	path, ok := call.Arguments["path"].(string)
	if !ok {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  "path argument must be a string",
		}
	}
	content, ok := call.Arguments["content"].(string)
	if !ok {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  "content argument must be a string",
		}
	}

	// Check content size limit
	if len(content) > maxFileWriteSize {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  fmt.Sprintf("content too large: %d bytes (max %d)", len(content), maxFileWriteSize),
		}
	}

	// Validate path is within sandbox
	sandboxDir := getSandboxDir(call)
	cleanPath, err := validatePath(path, sandboxDir)
	if err != nil {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  err.Error(),
		}
	}

	// Create parent directories if needed
	dir := filepath.Dir(cleanPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  fmt.Sprintf("failed to create directory: %v", err),
		}
	}

	if err := os.WriteFile(cleanPath, []byte(content), 0644); err != nil {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  fmt.Sprintf("failed to write file: %v", err),
		}
	}

	return agentic.ToolResult{
		CallID:  call.ID,
		Content: fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), cleanPath),
	}
}

func listDirectoryHandler(ctx context.Context, call agentic.ToolCall, _ *Quest, _ *Agent) agentic.ToolResult {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  fmt.Sprintf("operation cancelled: %v", ctx.Err()),
		}
	default:
	}

	path, ok := call.Arguments["path"].(string)
	if !ok {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  "path argument must be a string",
		}
	}

	// Validate path is within sandbox
	sandboxDir := getSandboxDir(call)
	cleanPath, err := validatePath(path, sandboxDir)
	if err != nil {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  err.Error(),
		}
	}

	entries, err := os.ReadDir(cleanPath)
	if err != nil {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  fmt.Sprintf("failed to read directory: %v", err),
		}
	}

	var result strings.Builder
	for _, entry := range entries {
		info, _ := entry.Info()
		if entry.IsDir() {
			result.WriteString(fmt.Sprintf("[dir]  %s/\n", entry.Name()))
		} else if info != nil {
			result.WriteString(fmt.Sprintf("[file] %s (%d bytes)\n", entry.Name(), info.Size()))
		} else {
			result.WriteString(fmt.Sprintf("[file] %s\n", entry.Name()))
		}
	}

	return agentic.ToolResult{
		CallID:  call.ID,
		Content: result.String(),
	}
}

func searchTextHandler(ctx context.Context, call agentic.ToolCall, _ *Quest, _ *Agent) agentic.ToolResult {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  fmt.Sprintf("operation cancelled: %v", ctx.Err()),
		}
	default:
	}

	pattern, ok := call.Arguments["pattern"].(string)
	if !ok {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  "pattern argument must be a string",
		}
	}
	path, ok := call.Arguments["path"].(string)
	if !ok {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  "path argument must be a string",
		}
	}

	// Validate path is within sandbox
	sandboxDir := getSandboxDir(call)
	cleanPath, err := validatePath(path, sandboxDir)
	if err != nil {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  err.Error(),
		}
	}

	// Check if path is a file or directory
	info, err := os.Stat(cleanPath)
	if err != nil {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  fmt.Sprintf("failed to stat path: %v", err),
		}
	}

	var matches []string

	if info.IsDir() {
		// Search directory recursively with context checking
		err = filepath.Walk(cleanPath, func(filePath string, info os.FileInfo, walkErr error) error {
			// Check context on each file
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if walkErr != nil || info.IsDir() {
				return nil
			}
			if len(matches) >= maxSearchMatches {
				return filepath.SkipAll
			}
			fileMatches := searchInFile(filePath, pattern)
			matches = append(matches, fileMatches...)
			return nil
		})
		if err != nil && err != filepath.SkipAll && err != context.Canceled && err != context.DeadlineExceeded {
			return agentic.ToolResult{
				CallID: call.ID,
				Error:  fmt.Sprintf("failed to walk directory: %v", err),
			}
		}
		if err == context.Canceled || err == context.DeadlineExceeded {
			return agentic.ToolResult{
				CallID: call.ID,
				Error:  fmt.Sprintf("operation cancelled: %v", err),
			}
		}
	} else {
		matches = searchInFile(cleanPath, pattern)
	}

	if len(matches) == 0 {
		return agentic.ToolResult{
			CallID:  call.ID,
			Content: fmt.Sprintf("No matches found for '%s' in %s", pattern, cleanPath),
		}
	}

	var result strings.Builder
	for _, match := range matches {
		result.WriteString(match)
		result.WriteByte('\n')
	}
	if len(matches) >= maxSearchMatches {
		result.WriteString(fmt.Sprintf("\n... (showing first %d matches)", maxSearchMatches))
	}

	return agentic.ToolResult{
		CallID:  call.ID,
		Content: result.String(),
	}
}

// searchInFile searches for a pattern in a single file.
func searchInFile(path, pattern string) []string {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var matches []string
	lines := strings.Split(string(content), "\n")
	for lineNum, line := range lines {
		if strings.Contains(line, pattern) {
			matches = append(matches, fmt.Sprintf("%s:%d: %s", path, lineNum+1, truncateLine(line, maxMatchLineLength)))
		}
	}
	return matches
}

func truncateLine(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
