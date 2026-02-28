package semdragons

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/c360studio/semstreams/agentic"
)

// =============================================================================
// TOOL REGISTRY - Manages available tools for agent execution
// =============================================================================
// Tools are capabilities that agents can invoke during quest execution.
// The registry maps tool names to handlers and enforces trust/skill gates.
// =============================================================================

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
	mu    sync.RWMutex
	tools map[string]RegisteredTool
}

// NewToolRegistry creates a new empty tool registry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]RegisteredTool),
	}
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
		if len(tool.Skills) > 0 {
			hasSkill := false
			for _, skill := range tool.Skills {
				if agent.HasSkill(skill) {
					hasSkill = true
					break
				}
			}
			if !hasSkill {
				continue
			}
		}

		// Check quest's allowed tools list (if specified)
		if len(quest.AllowedTools) > 0 {
			allowed := false
			for _, allowedTool := range quest.AllowedTools {
				if allowedTool == name {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
		}

		available = append(available, tool.Definition)
	}

	return available
}

// Execute runs a tool call and returns the result.
func (r *ToolRegistry) Execute(ctx context.Context, call agentic.ToolCall, quest *Quest, agent *Agent) agentic.ToolResult {
	r.mu.RLock()
	tool, ok := r.tools[call.Name]
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

	// Execute the handler
	return tool.Handler(ctx, call, quest, agent)
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

	// Basic path validation
	cleanPath := filepath.Clean(path)

	content, err := os.ReadFile(cleanPath)
	if err != nil {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  fmt.Sprintf("failed to read file: %v", err),
		}
	}

	// Limit content size to prevent huge outputs
	const maxSize = 100000
	result := string(content)
	if len(result) > maxSize {
		result = result[:maxSize] + "\n... (truncated)"
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

	cleanPath := filepath.Clean(path)

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

	cleanPath := filepath.Clean(path)

	entries, err := os.ReadDir(cleanPath)
	if err != nil {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  fmt.Sprintf("failed to read directory: %v", err),
		}
	}

	var result string
	for _, entry := range entries {
		info, _ := entry.Info()
		if entry.IsDir() {
			result += fmt.Sprintf("[dir]  %s/\n", entry.Name())
		} else if info != nil {
			result += fmt.Sprintf("[file] %s (%d bytes)\n", entry.Name(), info.Size())
		} else {
			result += fmt.Sprintf("[file] %s\n", entry.Name())
		}
	}

	return agentic.ToolResult{
		CallID:  call.ID,
		Content: result,
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

	cleanPath := filepath.Clean(path)

	// Check if path is a file or directory
	info, err := os.Stat(cleanPath)
	if err != nil {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  fmt.Sprintf("failed to stat path: %v", err),
		}
	}

	var matches []string
	maxMatches := 50

	if info.IsDir() {
		// Search directory recursively
		err = filepath.Walk(cleanPath, func(filePath string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if len(matches) >= maxMatches {
				return filepath.SkipAll
			}
			fileMatches := searchInFile(filePath, pattern)
			matches = append(matches, fileMatches...)
			return nil
		})
		if err != nil && err != filepath.SkipAll {
			return agentic.ToolResult{
				CallID: call.ID,
				Error:  fmt.Sprintf("failed to walk directory: %v", err),
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

	var result string
	for _, match := range matches {
		result += match + "\n"
	}
	if len(matches) >= maxMatches {
		result += fmt.Sprintf("\n... (showing first %d matches)", maxMatches)
	}

	return agentic.ToolResult{
		CallID:  call.ID,
		Content: result,
	}
}

// searchInFile searches for a pattern in a single file.
func searchInFile(path, pattern string) []string {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var matches []string
	lines := splitLines(string(content))
	for lineNum, line := range lines {
		if containsString(line, pattern) {
			matches = append(matches, fmt.Sprintf("%s:%d: %s", path, lineNum+1, truncateLine(line, 200)))
		}
	}
	return matches
}

func splitLines(s string) []string {
	var lines []string
	var line string
	for _, r := range s {
		if r == '\n' {
			lines = append(lines, line)
			line = ""
		} else {
			line += string(r)
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || findSubstring(s, substr) >= 0)
}

func findSubstring(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func truncateLine(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
