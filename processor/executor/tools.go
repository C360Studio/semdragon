package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/graph"
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
	// maxHTTPResponseSize is the maximum bytes to return from an HTTP response.
	maxHTTPResponseSize = 100000
	// maxCommandOutput is the maximum bytes to capture from command output.
	maxCommandOutput = 100000
	// commandTimeout is the default timeout for shell commands.
	commandTimeout = 60 * time.Second
	// httpRequestTimeout is the timeout for HTTP requests.
	httpRequestTimeout = 30 * time.Second
)

// ToolHandler executes a tool and returns the result.
// The handler receives the tool call arguments and quest/agent context.
type ToolHandler func(ctx context.Context, call agentic.ToolCall, quest *domain.Quest, agent *agentprogression.Agent) agentic.ToolResult

// RegisteredTool wraps a tool definition with its handler and access controls.
type RegisteredTool struct {
	Definition agentic.ToolDefinition // Name, description, parameters
	Handler    ToolHandler            // Execution function
	Skills     []domain.SkillTag      // Required skills to use this tool
	MinTier    domain.TrustTier       // Minimum trust tier to use
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

// validatePath ensures a path is within the sandbox directory.
// Resolves symlinks to prevent symlink-based sandbox escape (TOCTOU).
// Returns the real absolute path if valid, or an error if the path escapes the sandbox.
func validatePath(path, sandboxDir string) (string, error) {
	cleanPath := filepath.Clean(path)

	if sandboxDir == "" {
		return cleanPath, nil
	}

	absSandbox, err := filepath.Abs(sandboxDir)
	if err != nil {
		return "", fmt.Errorf("invalid sandbox directory: %w", err)
	}

	// Resolve symlinks in the sandbox itself.
	realSandbox, err := filepath.EvalSymlinks(absSandbox)
	if err != nil {
		return "", fmt.Errorf("resolve sandbox path: %w", err)
	}

	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	// Resolve symlinks in the target path.
	// If the file doesn't exist yet (write_file, patch_file), resolve the parent.
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		parentReal, parentErr := filepath.EvalSymlinks(filepath.Dir(absPath))
		if parentErr != nil {
			return "", fmt.Errorf("resolve parent path: %w", parentErr)
		}
		realPath = filepath.Join(parentReal, filepath.Base(absPath))
	}

	rel, err := filepath.Rel(realSandbox, realPath)
	if err != nil {
		return "", fmt.Errorf("path validation failed: %w", err)
	}

	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path escapes sandbox: %s", path)
	}

	return realPath, nil
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
		MinTier: domain.TierApprentice, // Level 1+ — read-only, safe for all agents
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
		Skills:  []domain.SkillTag{domain.SkillCodeGen},
		MinTier: domain.TierExpert, // Level 11+ can write files (production capability)
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
		MinTier: domain.TierApprentice, // Level 1+ — read-only, safe for all agents
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
		MinTier: domain.TierApprentice, // Level 1+ — read-only, safe for all agents
	})

	r.Register(RegisteredTool{
		Definition: agentic.ToolDefinition{
			Name:        "patch_file",
			Description: "Apply a targeted find-and-replace edit to a file. More precise than write_file for small changes.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "The file path to edit",
					},
					"old_text": map[string]any{
						"type":        "string",
						"description": "The exact text to find in the file",
					},
					"new_text": map[string]any{
						"type":        "string",
						"description": "The replacement text",
					},
				},
				"required": []any{"path", "old_text", "new_text"},
			},
		},
		Handler: patchFileHandler,
		Skills:  []domain.SkillTag{domain.SkillCodeGen},
		MinTier: domain.TierJourneyman, // Level 6+ — targeted edits require some trust
	})

	r.Register(RegisteredTool{
		Definition: agentic.ToolDefinition{
			Name:        "http_request",
			Description: "Make an HTTP request to a URL. Supports GET and POST methods.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{
						"type":        "string",
						"description": "The URL to request",
					},
					"method": map[string]any{
						"type":        "string",
						"description": "HTTP method (GET or POST). Defaults to GET.",
						"enum":        []any{"GET", "POST"},
					},
					"body": map[string]any{
						"type":        "string",
						"description": "Request body (for POST requests)",
					},
					"content_type": map[string]any{
						"type":        "string",
						"description": "Content-Type header value (for POST requests). Defaults to application/json.",
					},
				},
				"required": []any{"url"},
			},
		},
		Handler: httpRequestHandler,
		MinTier: domain.TierJourneyman, // Level 6+ — network access requires trust
	})

	r.Register(RegisteredTool{
		Definition: agentic.ToolDefinition{
			Name:        "run_tests",
			Description: "Run a test command in the workspace directory and return the output. Use for validating changes.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "The test command to run (e.g. 'go test ./...', 'npm test', 'pytest')",
					},
				},
				"required": []any{"command"},
			},
		},
		Handler: runTestsHandler,
		Skills:  []domain.SkillTag{domain.SkillCodeGen, domain.SkillCodeReview},
		MinTier: domain.TierExpert, // Level 11+ — test execution is a production capability
	})

	r.Register(RegisteredTool{
		Definition: agentic.ToolDefinition{
			Name:        "run_command",
			Description: "Run an arbitrary shell command in the workspace directory. Use responsibly — this has full shell access within the sandbox.",
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
		Handler: runCommandHandler,
		MinTier: domain.TierMaster, // Level 16+ — unrestricted shell requires high trust
	})
}

// =============================================================================
// BUILT-IN TOOL HANDLERS
// =============================================================================

func readFileHandler(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
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

func writeFileHandler(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
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

func listDirectoryHandler(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
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

func searchTextHandler(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
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
		err = filepath.Walk(cleanPath, func(filePath string, fileInfo os.FileInfo, walkErr error) error {
			// Check context on each file
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if walkErr != nil || fileInfo.IsDir() {
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

func patchFileHandler(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
	select {
	case <-ctx.Done():
		return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("operation cancelled: %v", ctx.Err())}
	default:
	}

	path, _ := call.Arguments["path"].(string)
	oldText, _ := call.Arguments["old_text"].(string)
	newText, _ := call.Arguments["new_text"].(string)

	if path == "" {
		return agentic.ToolResult{CallID: call.ID, Error: "path argument is required"}
	}
	if oldText == "" {
		return agentic.ToolResult{CallID: call.ID, Error: "old_text argument is required"}
	}

	sandboxDir := getSandboxDir(call)
	cleanPath, err := validatePath(path, sandboxDir)
	if err != nil {
		return agentic.ToolResult{CallID: call.ID, Error: err.Error()}
	}

	content, err := os.ReadFile(cleanPath)
	if err != nil {
		return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("failed to read file: %v", err)}
	}

	fileContent := string(content)
	if !strings.Contains(fileContent, oldText) {
		return agentic.ToolResult{CallID: call.ID, Error: "old_text not found in file"}
	}

	count := strings.Count(fileContent, oldText)
	if count > 1 {
		return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("old_text is ambiguous: found %d occurrences (must be unique)", count)}
	}

	newContent := strings.Replace(fileContent, oldText, newText, 1)

	if len(newContent) > maxFileWriteSize {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  fmt.Sprintf("resulting file too large: %d bytes (max %d)", len(newContent), maxFileWriteSize),
		}
	}

	if err := os.WriteFile(cleanPath, []byte(newContent), 0644); err != nil {
		return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("failed to write file: %v", err)}
	}

	msg := fmt.Sprintf("Successfully patched %s (%d bytes -> %d bytes)", cleanPath, len(oldText), len(newText))
	if newText == "" {
		msg = fmt.Sprintf("Successfully removed %d bytes from %s", len(oldText), cleanPath)
	}

	return agentic.ToolResult{
		CallID:  call.ID,
		Content: msg,
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

	result := string(body)
	truncated := ""
	if len(body) > maxHTTPResponseSize {
		result = result[:maxHTTPResponseSize]
		truncated = "\n... (response truncated)"
	}

	return agentic.ToolResult{
		CallID:  call.ID,
		Content: fmt.Sprintf("HTTP %d %s\n\n%s%s", resp.StatusCode, resp.Status, result, truncated),
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

// runShellCommand is the shared implementation for run_tests and run_command.
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

// allowedTestPrefixes are the commands that run_tests permits.
// The tool validates that the command starts with one of these.
var allowedTestPrefixes = []string{
	"go test", "npm test", "npm run test", "npx vitest", "npx jest",
	"pytest", "python -m pytest", "make test", "make check",
	"cargo test", "dotnet test", "mvn test", "gradle test",
}

func runTestsHandler(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
	command, _ := call.Arguments["command"].(string)
	allowed := false
	for _, prefix := range allowedTestPrefixes {
		if strings.HasPrefix(command, prefix) {
			allowed = true
			break
		}
	}
	if !allowed {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  fmt.Sprintf("run_tests only allows test commands (e.g. 'go test ./...', 'npm test'). Use run_command for general commands."),
		}
	}
	return runShellCommand(ctx, call, commandTimeout)
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
		Handler: graphQueryHandler(queryFn),
		MinTier: domain.TierApprentice, // Level 1+ — read-only graph access
	})
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
			limit = int(l)
			if limit > 100 {
				limit = 100
			}
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
