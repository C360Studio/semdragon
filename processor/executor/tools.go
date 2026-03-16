package executor

import (
	"bufio"
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
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/questdagexec"
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
	// maxGlobResults is the maximum number of file paths returned by glob_files.
	maxGlobResults = 200
	// maxReadFileRangeLines is the maximum line range allowed by read_file_range.
	maxReadFileRangeLines = 500
	// maxContextLines is the maximum context lines allowed by search_text.
	maxContextLines = 5
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

// toolSpec holds the shared metadata for a tool registration.
// Both RegisterBuiltins and RegisterSandboxTools use these specs,
// supplying different Handler implementations.
type toolSpec struct {
	Definition agentic.ToolDefinition
	MinTier    domain.TrustTier
	Skills     []domain.SkillTag
}

// Shared tool specs — single source of truth for definition, tier, and skills.
// Handlers are provided separately by RegisterBuiltins (local OS) and
// RegisterSandboxTools (proxied through a SandboxClient).

var readFileSpec = toolSpec{
	Definition: agentic.ToolDefinition{
		Name:        "read_file",
		Description: "Read the full contents of a file. Returns the file as text. Use glob_files or list_directory first if you don't know the exact path.",
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
	MinTier: domain.TierApprentice, // Read-only — all tiers can read files
}

var readFileRangeSpec = toolSpec{
	Definition: agentic.ToolDefinition{
		Name:        "read_file_range",
		Description: "Read a specific line range from a file. Use when a file is too large to read entirely, or to inspect a known section. Line numbers are 1-based. Returns up to 500 lines.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "The file path to read",
				},
				"start_line": map[string]any{
					"type":        "integer",
					"description": "First line to read (1-based)",
				},
				"end_line": map[string]any{
					"type":        "integer",
					"description": "Last line to read inclusive (defaults to start_line + 100)",
				},
			},
			"required": []any{"path", "start_line"},
		},
	},
	MinTier: domain.TierApprentice, // Read-only — all tiers can read file ranges
}

var writeFileSpec = toolSpec{
	Definition: agentic.ToolDefinition{
		Name:        "write_file",
		Description: "Create or overwrite a file with the given content. Parent directories must exist — use create_directory first if needed. For small edits to existing files, prefer patch_file.",
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
	Skills:  []domain.SkillTag{domain.SkillCodeGen},
	MinTier: domain.TierApprentice, // All tiers — sandbox is the workspace, writing files is fundamental
}

var patchFileSpec = toolSpec{
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
	Skills:  []domain.SkillTag{domain.SkillCodeGen},
	MinTier: domain.TierJourneyman, // Level 6+ — targeted edits require some trust
}

var deleteFileSpec = toolSpec{
	Definition: agentic.ToolDefinition{
		Name:        "delete_file",
		Description: "Delete a single file. Cannot delete directories. Use with caution — this is irreversible.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "The file path to delete",
				},
			},
			"required": []any{"path"},
		},
	},
	Skills:  []domain.SkillTag{domain.SkillCodeGen},
	MinTier: domain.TierJourneyman, // Level 6+ — destructive operations require trust
}

var renameFileSpec = toolSpec{
	Definition: agentic.ToolDefinition{
		Name:        "rename_file",
		Description: "Move or rename a file. The destination directory must already exist. Use create_directory first if needed.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"old_path": map[string]any{
					"type":        "string",
					"description": "The current file path",
				},
				"new_path": map[string]any{
					"type":        "string",
					"description": "The target file path",
				},
			},
			"required": []any{"old_path", "new_path"},
		},
	},
	Skills:  []domain.SkillTag{domain.SkillCodeGen},
	MinTier: domain.TierJourneyman, // Level 6+ — filesystem writes require trust
}

var createDirectorySpec = toolSpec{
	Definition: agentic.ToolDefinition{
		Name:        "create_directory",
		Description: "Create a directory and any missing parent directories. Use before write_file when the target directory doesn't exist yet.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "The directory path to create",
				},
			},
			"required": []any{"path"},
		},
	},
	Skills:  []domain.SkillTag{domain.SkillCodeGen},
	MinTier: domain.TierApprentice, // All tiers — needed alongside write_file in sandbox workspace
}

var listDirectorySpec = toolSpec{
	Definition: agentic.ToolDefinition{
		Name:        "list_directory",
		Description: "List files and subdirectories in a directory. Returns names with type indicators (/ for dirs). Use to explore project structure before reading specific files.",
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
	MinTier: domain.TierApprentice, // Read-only — all tiers can list directories
}

var globFilesSpec = toolSpec{
	Definition: agentic.ToolDefinition{
		Name:        "glob_files",
		Description: "Find files matching a glob pattern (e.g. '**/*.java', 'src/**/*.go', '*.json'). Returns matching file paths. Use to discover files before reading them.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "Glob pattern to match, e.g. '**/*.go' or 'src/**/*.ts'",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Base directory to search from. Defaults to sandbox root.",
				},
			},
			"required": []any{"pattern"},
		},
	},
	MinTier: domain.TierApprentice, // Read-only — all tiers can search for files
}

var searchTextSpec = toolSpec{
	Definition: agentic.ToolDefinition{
		Name:        "search_text",
		Description: "Search for text or regex patterns across files. Returns matching lines with file paths and line numbers. Use file_glob to narrow by extension (e.g. '*.go'). Use context_lines to see surrounding code.",
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
				"file_glob": map[string]any{
					"type":        "string",
					"description": "Optional glob pattern to filter files (e.g. '*.go', '*.ts')",
				},
				"context_lines": map[string]any{
					"type":        "integer",
					"description": "Number of lines of context before and after each match (default 0, max 5)",
				},
				"regex": map[string]any{
					"type":        "boolean",
					"description": "Treat pattern as a regular expression instead of a literal string (default false)",
				},
			},
			"required": []any{"pattern", "path"},
		},
	},
	MinTier: domain.TierApprentice, // Read-only — all tiers can search files
}

var runTestsSpec = toolSpec{
	Definition: agentic.ToolDefinition{
		Name:        "run_tests",
		Description: "Run a test command and return stdout/stderr. Only test runner commands are allowed (go test, npm test, pytest, cargo test, gradle test, mvn test, make test). Use after writing code to verify it works.",
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
	Skills:  []domain.SkillTag{domain.SkillCodeGen, domain.SkillCodeReview},
	MinTier: domain.TierJourneyman, // Level 6+ — allowlist constrains to known test runners
}

var lintCheckSpec = toolSpec{
	Definition: agentic.ToolDefinition{
		Name:        "lint_check",
		Description: "Run a linter and return the output. Only linter commands are allowed (go vet, golangci-lint, eslint, pylint, flake8, clippy, checkstyle). Use after writing code to check for issues.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "The lint command to run (e.g. 'go vet ./...', 'golangci-lint run', 'eslint src/')",
				},
			},
			"required": []any{"command"},
		},
	},
	Skills:  []domain.SkillTag{domain.SkillCodeReview},
	MinTier: domain.TierJourneyman, // Level 6+ — allowlist constrains to known linters
}

var runCommandSpec = toolSpec{
	Definition: agentic.ToolDefinition{
		Name:        "run_command",
		Description: "Run a shell command in the workspace sandbox. Has full access to installed tools (go, node, java, gradle, maven, python, git, curl, etc.). Use for operations not covered by other tools. Prefer build_project for builds, run_tests for tests, and git_operation for version control.",
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
	MinTier: domain.TierMaster, // Level 16+ — unrestricted shell requires high trust
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
	MinTier: domain.TierJourneyman, // Level 6+ — network access requires trust
}

var inspectEnvironmentSpec = toolSpec{
	Definition: agentic.ToolDefinition{
		Name: "inspect_environment",
		Description: "Inspect the development environment: installed tools, versions, and project structure. " +
			"Returns a structured report of available toolchains (Go, Java, Node.js, Python, Gradle, Maven, Cargo, Make) " +
			"with their versions, plus working directory contents. " +
			"Call this ONCE at the start of a quest instead of running multiple 'which' and 'version' commands.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	},
	MinTier: domain.TierApprentice, // Read-only environment inspection — safe for all tiers
}

var gitOperationSpec = toolSpec{
	Definition: agentic.ToolDefinition{
		Name: "git_operation",
		Description: "Perform structured git operations. Safer than raw shell commands. " +
			"Supported actions: init, clone, status, diff, log, add, commit, branch, checkout, show. " +
			"Destructive operations (push, pull, rebase, reset, force) are blocked. " +
			"Examples: {action: 'clone', url: 'https://github.com/org/repo'}, " +
			"{action: 'commit', message: 'feat: add parser'}, " +
			"{action: 'diff'}, {action: 'log', args: '--oneline -10'}.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"description": "Git action to perform",
					"enum":        []any{"init", "clone", "status", "diff", "log", "add", "commit", "branch", "checkout", "show"},
				},
				"args": map[string]any{
					"type":        "string",
					"description": "Additional arguments (e.g. file paths for 'add', branch name for 'checkout', '--oneline -10' for 'log')",
				},
				"url": map[string]any{
					"type":        "string",
					"description": "Repository URL for 'clone' action (https:// or git@ only)",
				},
				"message": map[string]any{
					"type":        "string",
					"description": "Commit message for 'commit' action",
				},
			},
			"required": []any{"action"},
		},
	},
	Skills:  []domain.SkillTag{domain.SkillCodeGen},
	MinTier: domain.TierJourneyman, // Level 6+ — version control requires demonstrated trust
}

var buildProjectSpec = toolSpec{
	Definition: agentic.ToolDefinition{
		Name: "build_project",
		Description: "Build the project using its detected build system. Auto-detects: " +
			"Gradle (build.gradle/build.gradle.kts), Go (go.mod), npm (package.json), " +
			"Maven (pom.xml), Cargo (Cargo.toml), Make (Makefile). " +
			"Optionally specify a build target (e.g. 'clean', 'install', 'dist'). " +
			"Has a 5-minute timeout. Examples: {} (auto-detect and build), {target: 'clean'}.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target": map[string]any{
					"type":        "string",
					"description": "Build target or task (e.g. 'clean', 'install', 'dist'). Omit for default build.",
				},
			},
		},
	},
	Skills:  []domain.SkillTag{domain.SkillCodeGen},
	MinTier: domain.TierJourneyman, // Level 6+ — building requires development trust
}

var manageDependenciesSpec = toolSpec{
	Definition: agentic.ToolDefinition{
		Name: "manage_dependencies",
		Description: "Manage project dependencies using the detected package manager. " +
			"Auto-detects: Go (go.mod), npm (package.json), Maven (pom.xml), " +
			"Gradle (build.gradle), Cargo (Cargo.toml), pip (requirements.txt/pyproject.toml). " +
			"Supported actions: install (all deps), add (new package), remove, list, tidy. " +
			"Examples: {action: 'install'}, {action: 'add', packages: ['lodash']}, {action: 'tidy'}.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"description": "Dependency action to perform",
					"enum":        []any{"install", "add", "remove", "list", "tidy"},
				},
				"packages": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Package names for add/remove actions (e.g. ['lodash', 'express'])",
				},
			},
			"required": []any{"action"},
		},
	},
	Skills:  []domain.SkillTag{domain.SkillCodeGen},
	MinTier: domain.TierExpert, // Level 11+ — dependency changes affect build reproducibility
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
	// If the parent doesn't exist either, fall back to the cleaned absolute path
	// so the sandbox boundary check below can still reject traversals.
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		parentReal, parentErr := filepath.EvalSymlinks(filepath.Dir(absPath))
		if parentErr != nil {
			// Neither the file nor its parent exist. Use the cleaned absolute path
			// so the sandbox prefix check still detects an escape attempt.
			realPath = absPath
		} else {
			realPath = filepath.Join(parentReal, filepath.Base(absPath))
		}
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
		Definition: readFileSpec.Definition,
		Handler:    readFileHandler,
		Skills:     readFileSpec.Skills,
		MinTier:    readFileSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: readFileRangeSpec.Definition,
		Handler:    readFileRangeHandler,
		Skills:     readFileRangeSpec.Skills,
		MinTier:    readFileRangeSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: writeFileSpec.Definition,
		Handler:    writeFileHandler,
		Skills:     writeFileSpec.Skills,
		MinTier:    writeFileSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: patchFileSpec.Definition,
		Handler:    patchFileHandler,
		Skills:     patchFileSpec.Skills,
		MinTier:    patchFileSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: deleteFileSpec.Definition,
		Handler:    deleteFileHandler,
		Skills:     deleteFileSpec.Skills,
		MinTier:    deleteFileSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: renameFileSpec.Definition,
		Handler:    renameFileHandler,
		Skills:     renameFileSpec.Skills,
		MinTier:    renameFileSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: createDirectorySpec.Definition,
		Handler:    createDirectoryHandler,
		Skills:     createDirectorySpec.Skills,
		MinTier:    createDirectorySpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: listDirectorySpec.Definition,
		Handler:    listDirectoryHandler,
		Skills:     listDirectorySpec.Skills,
		MinTier:    listDirectorySpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: globFilesSpec.Definition,
		Handler:    globFilesHandler,
		Skills:     globFilesSpec.Skills,
		MinTier:    globFilesSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: searchTextSpec.Definition,
		Handler:    searchTextHandler,
		Skills:     searchTextSpec.Skills,
		MinTier:    searchTextSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: runTestsSpec.Definition,
		Handler:    runTestsHandler,
		Skills:     runTestsSpec.Skills,
		MinTier:    runTestsSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: lintCheckSpec.Definition,
		Handler:    lintCheckHandler,
		Skills:     lintCheckSpec.Skills,
		MinTier:    lintCheckSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: runCommandSpec.Definition,
		Handler:    runCommandHandler,
		Skills:     runCommandSpec.Skills,
		MinTier:    runCommandSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: httpRequestSpec.Definition,
		Handler:    httpRequestHandler,
		Skills:     httpRequestSpec.Skills,
		MinTier:    httpRequestSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: inspectEnvironmentSpec.Definition,
		Handler:    inspectEnvironmentHandler,
		Skills:     inspectEnvironmentSpec.Skills,
		MinTier:    inspectEnvironmentSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: gitOperationSpec.Definition,
		Handler:    gitOperationHandler,
		Skills:     gitOperationSpec.Skills,
		MinTier:    gitOperationSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: buildProjectSpec.Definition,
		Handler:    buildProjectHandler,
		Skills:     buildProjectSpec.Skills,
		MinTier:    buildProjectSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: manageDependenciesSpec.Definition,
		Handler:    manageDepsHandler,
		Skills:     manageDependenciesSpec.Skills,
		MinTier:    manageDependenciesSpec.MinTier,
	})

	// Terminal tools — these stop the agentic loop on successful execution.
	// submit_work_product replaces [INTENT: work_product] tags.
	// ask_clarification replaces [INTENT: clarification] tags.
	r.Register(RegisteredTool{
		Definition: agentic.ToolDefinition{
			Name:        "submit_work_product",
			Description: "Submit your FINISHED work for review. Files you wrote/modified are captured automatically — you do NOT need to paste file contents. Provide a summary describing what you built and any design decisions. Only call this when you have completed the actual work — never use this to ask questions or describe plans.",
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
		Handler: submitWorkProductHandler,
		MinTier: domain.TierApprentice, // All tiers can submit work
	})

	r.Register(RegisteredTool{
		Definition: agentic.ToolDefinition{
			Name:        "ask_clarification",
			Description: "Ask the quest issuer a question when you need more information. Use this instead of submit_work_product when you have questions or are unsure how to proceed. You will NOT be penalized for asking questions — this is the correct way to request guidance.",
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
		Handler: askClarificationHandler,
		MinTier: domain.TierApprentice, // All tiers can ask questions
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
			MinTier: domain.TierMaster, // Level 16+ — only party leads (Master+) can decompose quests
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
			MinTier: domain.TierMaster, // Level 16+ — only party leads (Master+) can review sub-quests
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
			MinTier: domain.TierMaster, // Level 16+ — only party leads (Master+) can answer clarifications
		})
	}
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

	// Parse optional enhanced parameters.
	var opts searchOptions
	if fg, ok := call.Arguments["file_glob"].(string); ok {
		opts.fileGlob = fg
	}
	if cl, ok := call.Arguments["context_lines"].(float64); ok {
		opts.contextLines = int(cl)
	}
	if rx, ok := call.Arguments["regex"].(bool); ok {
		opts.useRegex = rx
	}

	// Pre-compile the regex once so we don't recompile per file during the walk.
	if opts.useRegex {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return agentic.ToolResult{
				CallID: call.ID,
				Error:  fmt.Sprintf("invalid regex: %v", err),
			}
		}
		opts.compiledRe = re
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

			// Apply file_glob filter if specified.
			if opts.fileGlob != "" && !globMatchesName(opts.fileGlob, filepath.Base(filePath)) {
				return nil
			}

			if len(matches) >= maxSearchMatches {
				return filepath.SkipAll
			}
			fileMatches := searchInFileWithOpts(filePath, pattern, opts)
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
		matches = searchInFileWithOpts(cleanPath, pattern, opts)
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

func truncateLine(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// =============================================================================
// searchTextHandler — enhanced search with regex, file_glob, and context_lines
// =============================================================================

// searchOptions holds the optional parameters for searchInFileWithOpts.
type searchOptions struct {
	// fileGlob filters files by name pattern when walking a directory.
	fileGlob string
	// contextLines is how many lines before/after a match to include.
	contextLines int
	// useRegex treats the pattern as a compiled regexp.
	useRegex bool
	// compiledRe is a pre-compiled regex, set by the caller to avoid
	// recompilation on every file during a directory walk.
	compiledRe *regexp.Regexp
}

// searchInFileWithOpts searches for a pattern in a single file using opts.
// It replaces the simple searchInFile for internal use while preserving
// backward-compatible callers via the searchInFile wrapper.
func searchInFileWithOpts(path, pattern string, opts searchOptions) []string {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	lines := strings.Split(string(content), "\n")

	// Build a matcher function based on opts.
	var matchFn func(line string) bool
	if opts.useRegex && opts.compiledRe != nil {
		matchFn = opts.compiledRe.MatchString
	} else if opts.useRegex {
		// Fallback: compile from pattern if caller did not pre-compile.
		re, err := regexp.Compile(pattern)
		if err != nil {
			return []string{fmt.Sprintf("error: invalid regex %q: %v", pattern, err)}
		}
		matchFn = re.MatchString
	} else {
		matchFn = func(line string) bool { return strings.Contains(line, pattern) }
	}

	// Clamp context lines.
	nContext := opts.contextLines
	if nContext < 0 {
		nContext = 0
	}
	if nContext > maxContextLines {
		nContext = maxContextLines
	}

	var matches []string
	for lineNum, line := range lines {
		if !matchFn(line) {
			continue
		}

		if nContext == 0 {
			matches = append(matches, fmt.Sprintf("%s:%d: %s", path, lineNum+1, truncateLine(line, maxMatchLineLength)))
			continue
		}

		// Emit a "file:line --" header for the block then the context lines.
		start := lineNum - nContext
		if start < 0 {
			start = 0
		}
		end := lineNum + nContext
		if end >= len(lines) {
			end = len(lines) - 1
		}
		for i := start; i <= end; i++ {
			prefix := "  "
			if i == lineNum {
				prefix = "> "
			}
			matches = append(matches, fmt.Sprintf("%s:%d:%s%s", path, i+1, prefix, truncateLine(lines[i], maxMatchLineLength)))
		}
	}
	return matches
}

// globMatchesName reports whether name matches the simple glob pattern
// using filepath.Match semantics (no ** support — used per-segment).
func globMatchesName(pattern, name string) bool {
	matched, err := filepath.Match(pattern, name)
	return err == nil && matched
}

// =============================================================================
// glob_files handler
// =============================================================================

func globFilesHandler(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
	select {
	case <-ctx.Done():
		return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("operation cancelled: %v", ctx.Err())}
	default:
	}

	pattern, _ := call.Arguments["pattern"].(string)
	if pattern == "" {
		return agentic.ToolResult{CallID: call.ID, Error: "pattern argument is required"}
	}

	sandboxDir := getSandboxDir(call)

	// Determine base path.
	basePath := sandboxDir
	if p, ok := call.Arguments["path"].(string); ok && p != "" {
		basePath = p
	}
	if basePath == "" {
		basePath = "."
	}

	cleanBase, err := validatePath(basePath, sandboxDir)
	if err != nil {
		return agentic.ToolResult{CallID: call.ID, Error: err.Error()}
	}

	info, err := os.Stat(cleanBase)
	if err != nil {
		return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("base path not found: %v", err)}
	}
	if !info.IsDir() {
		return agentic.ToolResult{CallID: call.ID, Error: "path must be a directory"}
	}

	var results []string

	err = filepath.WalkDir(cleanBase, func(walkPath string, d os.DirEntry, walkErr error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if walkErr != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			return nil
		}
		if len(results) >= maxGlobResults {
			return filepath.SkipAll
		}

		// Compute the path relative to cleanBase for pattern matching.
		rel, err := filepath.Rel(cleanBase, walkPath)
		if err != nil {
			return nil
		}

		if matchGlobPattern(pattern, rel) {
			results = append(results, walkPath)
		}
		return nil
	})

	if err != nil && err != filepath.SkipAll {
		if err == context.Canceled || err == context.DeadlineExceeded {
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("operation cancelled: %v", err)}
		}
		return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("walk failed: %v", err)}
	}

	if len(results) == 0 {
		return agentic.ToolResult{CallID: call.ID, Content: fmt.Sprintf("No files matched pattern %q in %s", pattern, cleanBase)}
	}

	var sb strings.Builder
	for _, p := range results {
		sb.WriteString(p)
		sb.WriteByte('\n')
	}
	if len(results) >= maxGlobResults {
		sb.WriteString(fmt.Sprintf("\n... (showing first %d matches)", maxGlobResults))
	}

	return agentic.ToolResult{CallID: call.ID, Content: sb.String()}
}

// matchGlobPattern matches a relative file path against a glob pattern that
// may contain ** for recursive directory matching.
//
// Rules:
//   - A leading **/ matches any number of directory components (including zero).
//   - A trailing /** matches everything under a directory.
//   - ** in the middle matches any sequence of path segments.
//   - Non-** segments are matched with filepath.Match against the corresponding
//     segment from the file path.
func matchGlobPattern(pattern, relPath string) bool {
	// Normalise separators so tests run on Windows too.
	pattern = filepath.ToSlash(pattern)
	relPath = filepath.ToSlash(relPath)

	patParts := strings.Split(pattern, "/")
	pathParts := strings.Split(relPath, "/")
	return matchSegments(patParts, pathParts)
}

// matchSegments recursively matches pattern segments against path segments.
func matchSegments(pat, path []string) bool {
	for len(pat) > 0 {
		seg := pat[0]
		if seg == "**" {
			// ** can match zero or more path segments.
			pat = pat[1:]
			if len(pat) == 0 {
				return true // ** at end matches everything remaining
			}
			// Try consuming 0..N path segments before the next pattern segment.
			for i := 0; i <= len(path); i++ {
				if matchSegments(pat, path[i:]) {
					return true
				}
			}
			return false
		}

		if len(path) == 0 {
			return false
		}

		matched, err := filepath.Match(seg, path[0])
		if err != nil || !matched {
			return false
		}
		pat = pat[1:]
		path = path[1:]
	}
	return len(path) == 0
}

// =============================================================================
// read_file_range handler
// =============================================================================

func readFileRangeHandler(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
	select {
	case <-ctx.Done():
		return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("operation cancelled: %v", ctx.Err())}
	default:
	}

	path, _ := call.Arguments["path"].(string)
	if path == "" {
		return agentic.ToolResult{CallID: call.ID, Error: "path argument is required"}
	}

	// JSON numbers decode as float64.
	startLineF, ok := call.Arguments["start_line"].(float64)
	if !ok {
		return agentic.ToolResult{CallID: call.ID, Error: "start_line argument must be an integer"}
	}
	startLine := int(startLineF)
	if startLine < 1 {
		return agentic.ToolResult{CallID: call.ID, Error: "start_line must be >= 1"}
	}

	endLine := startLine + 100
	if endLineF, ok := call.Arguments["end_line"].(float64); ok {
		endLine = int(endLineF)
	}
	if endLine < startLine {
		return agentic.ToolResult{CallID: call.ID, Error: "end_line must be >= start_line"}
	}
	if endLine-startLine+1 > maxReadFileRangeLines {
		endLine = startLine + maxReadFileRangeLines - 1
	}

	sandboxDir := getSandboxDir(call)
	cleanPath, err := validatePath(path, sandboxDir)
	if err != nil {
		return agentic.ToolResult{CallID: call.ID, Error: err.Error()}
	}

	f, err := os.Open(cleanPath)
	if err != nil {
		return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("failed to open file: %v", err)}
	}
	defer f.Close()

	var sb strings.Builder
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum < startLine {
			continue
		}
		if lineNum > endLine {
			break
		}
		sb.WriteString(fmt.Sprintf("%d\t%s\n", lineNum, scanner.Text()))
	}
	if err := scanner.Err(); err != nil {
		return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("error reading file: %v", err)}
	}

	if sb.Len() == 0 {
		return agentic.ToolResult{
			CallID:  call.ID,
			Content: fmt.Sprintf("File has fewer than %d lines", startLine),
		}
	}

	return agentic.ToolResult{CallID: call.ID, Content: sb.String()}
}

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
// TERMINAL TOOL HANDLERS (submit_work_product, ask_clarification)
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
				"Only use submit_work_product when you have finished work to submit.",
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
// smaller models) that use submit_work_product instead of ask_clarification.
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

// =============================================================================
// create_directory handler
// =============================================================================

func createDirectoryHandler(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
	select {
	case <-ctx.Done():
		return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("operation cancelled: %v", ctx.Err())}
	default:
	}

	path, _ := call.Arguments["path"].(string)
	if path == "" {
		return agentic.ToolResult{CallID: call.ID, Error: "path argument is required"}
	}

	sandboxDir := getSandboxDir(call)
	cleanPath, err := validatePath(path, sandboxDir)
	if err != nil {
		return agentic.ToolResult{CallID: call.ID, Error: err.Error()}
	}

	if err := os.MkdirAll(cleanPath, 0755); err != nil {
		return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("failed to create directory: %v", err)}
	}

	return agentic.ToolResult{CallID: call.ID, Content: fmt.Sprintf("Created directory: %s", cleanPath)}
}

// =============================================================================
// delete_file handler
// =============================================================================

func deleteFileHandler(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
	select {
	case <-ctx.Done():
		return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("operation cancelled: %v", ctx.Err())}
	default:
	}

	path, _ := call.Arguments["path"].(string)
	if path == "" {
		return agentic.ToolResult{CallID: call.ID, Error: "path argument is required"}
	}

	sandboxDir := getSandboxDir(call)
	cleanPath, err := validatePath(path, sandboxDir)
	if err != nil {
		return agentic.ToolResult{CallID: call.ID, Error: err.Error()}
	}

	info, err := os.Stat(cleanPath)
	if err != nil {
		return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("path not found: %v", err)}
	}
	if info.IsDir() {
		return agentic.ToolResult{CallID: call.ID, Error: "delete_file cannot delete directories; use a shell command for that"}
	}

	if err := os.Remove(cleanPath); err != nil {
		return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("failed to delete file: %v", err)}
	}

	return agentic.ToolResult{CallID: call.ID, Content: fmt.Sprintf("Deleted: %s", cleanPath)}
}

// =============================================================================
// rename_file handler
// =============================================================================

func renameFileHandler(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
	select {
	case <-ctx.Done():
		return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("operation cancelled: %v", ctx.Err())}
	default:
	}

	oldPath, _ := call.Arguments["old_path"].(string)
	newPath, _ := call.Arguments["new_path"].(string)
	if oldPath == "" {
		return agentic.ToolResult{CallID: call.ID, Error: "old_path argument is required"}
	}
	if newPath == "" {
		return agentic.ToolResult{CallID: call.ID, Error: "new_path argument is required"}
	}

	sandboxDir := getSandboxDir(call)
	cleanOld, err := validatePath(oldPath, sandboxDir)
	if err != nil {
		return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("old_path: %v", err)}
	}
	cleanNew, err := validatePath(newPath, sandboxDir)
	if err != nil {
		return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("new_path: %v", err)}
	}

	// Reject directory renames — rename_file operates on files only.
	info, statErr := os.Stat(cleanOld)
	if statErr != nil {
		return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("source not found: %v", statErr)}
	}
	if info.IsDir() {
		return agentic.ToolResult{CallID: call.ID, Error: "rename_file operates on files only; cannot rename directories"}
	}

	// Ensure the destination parent directory exists.
	destDir := filepath.Dir(cleanNew)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("failed to create destination directory: %v", err)}
	}

	if err := os.Rename(cleanOld, cleanNew); err != nil {
		return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("rename failed: %v", err)}
	}

	return agentic.ToolResult{CallID: call.ID, Content: fmt.Sprintf("Renamed %s -> %s", cleanOld, cleanNew)}
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

// shellMetacharacters are dangerous shell operators that enable command chaining
// or injection. Commands passed to allowlisted tools (run_tests, lint_check)
// are rejected if they contain any of these to prevent Expert-tier agents
// from bypassing the allowlist via shell metacharacters.
var shellMetacharacters = []string{";", "&&", "||", "|", "$(", "`", ">", "<"}

// containsShellMeta reports whether the command contains shell metacharacters
// that could enable command chaining or injection.
func containsShellMeta(command string) bool {
	for _, meta := range shellMetacharacters {
		if strings.Contains(command, meta) {
			return true
		}
	}
	return false
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
	if containsShellMeta(command) {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  "run_tests does not allow shell metacharacters (;, &&, ||, |, $, `, >, <). Use run_command for compound commands.",
		}
	}
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
			Error:  "run_tests only allows test commands (e.g. 'go test ./...', 'npm test'). Use run_command for general commands.",
		}
	}
	return runShellCommand(ctx, call, commandTimeout)
}

func runCommandHandler(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
	return runShellCommand(ctx, call, commandTimeout)
}

// allowedLintPrefixes are the commands that lint_check permits.
var allowedLintPrefixes = []string{
	"revive", "golangci-lint", "eslint", "npx eslint", "npm run lint",
	"make lint", "pylint", "flake8", "mypy", "ruff",
	"cargo clippy", "dotnet format", "go vet",
}

func lintCheckHandler(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
	command, _ := call.Arguments["command"].(string)
	if containsShellMeta(command) {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  "lint_check does not allow shell metacharacters (;, &&, ||, |, $, `, >, <). Use run_command for compound commands.",
		}
	}
	allowed := false
	for _, prefix := range allowedLintPrefixes {
		if strings.HasPrefix(command, prefix) {
			allowed = true
			break
		}
	}
	if !allowed {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  "lint_check only allows linter commands (e.g. 'go vet ./...', 'golangci-lint run'). Use run_command for general commands.",
		}
	}
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
		Handler: handler,
		Skills:  []domain.SkillTag{domain.SkillResearch},
		MinTier: domain.TierApprentice,
	})
}

// =============================================================================
// GRAPH SEARCH TOOL
// =============================================================================

// RegisterGraphSearch adds the graph_search tool to the registry.
// graphqlURL is the graph-gateway GraphQL endpoint (e.g. "http://localhost:8082/graphql").
// Unlike graph_query (which is limited to game entities), graph_search queries ALL
// entities via GraphQL — including semsource entities such as docs, code, and repos.
func (r *ToolRegistry) RegisterGraphSearch(graphqlURL string) {
	r.Register(RegisteredTool{
		Definition: agentic.ToolDefinition{
			Name:        "graph_search",
			Description: "Search the knowledge graph via GraphQL. Supports entity lookup, relationship traversal, predicate queries, and full-text search across all entities including source documentation and code.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query_type": map[string]any{
						"type":        "string",
						"description": "Type of graph query to execute",
						"enum":        []any{"entity", "prefix", "predicate", "relationships", "search"},
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
		Handler: graphSearchHandler(graphqlURL),
		MinTier: domain.TierApprentice, // Read-only knowledge-graph access
	})
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
// the configured graph-gateway endpoint based on the query_type argument.
func graphSearchHandler(graphqlURL string) ToolHandler {
	// graphSearchClient is a plain HTTP client — graphqlURL is operator-configured,
	// not agent-supplied, so SSRF prevention is not required here.
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

		reqCtx, cancel := context.WithTimeout(ctx, graphQLTimeout)
		defer cancel()

		result, err := executeGraphQLQuery(reqCtx, client, graphqlURL, gqlReq)
		if err != nil {
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("graph search failed: %v", err)}
		}

		return agentic.ToolResult{CallID: call.ID, Content: result}
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
			Query: fmt.Sprintf(`{ entity(id: %q) { id triples } }`, sanitizeGraphQLString(id)),
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
		// maxCommunities controls how many communities are searched — each can
		// return many entities, so cap it lower than the general limit to keep
		// response sizes manageable.
		maxCommunities := min(limit, 5)
		return graphQLRequest{
			Query: fmt.Sprintf(`{ globalSearch(query: %q, maxCommunities: %d) { entities { id type } count } }`, sanitizeGraphQLString(text), maxCommunities),
		}, nil

	default:
		return graphQLRequest{}, fmt.Errorf("invalid query_type: %q (must be one of entity, prefix, predicate, relationships, search)", queryType)
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

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxHTTPResponseSize+1))
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
	if len(respBody) > maxHTTPResponseSize {
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
			ID      string            `json:"id"`
			Triples []json.RawMessage `json:"triples"`
		}
		if err := json.Unmarshal(raw, &entity); err != nil || entity.ID == "" {
			b.WriteString("Entity not found.\n")
			return b.String()
		}
		b.WriteString(fmt.Sprintf("Entity: %s\n", entity.ID))
		b.WriteString(fmt.Sprintf("Triples (%d):\n", len(entity.Triples)))
		for _, t := range entity.Triples {
			b.WriteString("  ")
			b.Write(t)
			b.WriteByte('\n')
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

	// globalSearch query: entities with count.
	if raw, ok := data["globalSearch"]; ok {
		var result struct {
			Entities []struct {
				ID   string `json:"id"`
				Type string `json:"type"`
			} `json:"entities"`
			Count int `json:"count"`
		}
		if err := json.Unmarshal(raw, &result); err != nil || len(result.Entities) == 0 {
			b.WriteString("No search results found.\n")
			return b.String()
		}
		b.WriteString(fmt.Sprintf("Search results (%d total, showing %d):\n", result.Count, len(result.Entities)))
		for _, e := range result.Entities {
			b.WriteString(fmt.Sprintf("  [%s] %s\n", e.Type, e.ID))
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
// INSPECT ENVIRONMENT
// =============================================================================

// inspectEnvironmentScript is the shell script that probes installed toolchains.
const inspectEnvironmentScript = `echo "=== Toolchain Versions ==="
go version 2>/dev/null || echo "go: not installed"
java -version 2>&1 | head -1 || echo "java: not installed"
node --version 2>/dev/null | sed 's/^/node: /' || echo "node: not installed"
npm --version 2>/dev/null | sed 's/^/npm: /' || echo "npm: not installed"
python3 --version 2>/dev/null || echo "python3: not installed"
gradle --version 2>/dev/null | grep '^Gradle' || echo "gradle: not installed"
mvn --version 2>/dev/null | head -1 || echo "maven: not installed"
cargo --version 2>/dev/null || echo "cargo: not installed"
make --version 2>/dev/null | head -1 || echo "make: not installed"
git --version 2>/dev/null || echo "git: not installed"
echo ""
echo "=== Working Directory ==="
pwd
echo ""
ls -la 2>/dev/null || echo "(empty workspace)"`

func inspectEnvironmentHandler(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
	if call.Arguments == nil {
		call.Arguments = make(map[string]any)
	}
	call.Arguments["command"] = inspectEnvironmentScript
	return runShellCommand(ctx, call, 15*time.Second)
}

// =============================================================================
// GIT OPERATION
// =============================================================================

// allowedGitActions is the set of git subcommands that git_operation permits.
var allowedGitActions = map[string]bool{
	"init": true, "clone": true, "status": true, "diff": true,
	"log": true, "add": true, "commit": true, "branch": true,
	"checkout": true, "show": true,
}

// blockedGitFlags prevents dangerous flags from being passed through args.
var blockedGitFlags = []string{"--force", "-f", "--hard", "--mixed"}

// buildGitCommand constructs a validated git command from tool call arguments.
func buildGitCommand(call agentic.ToolCall) (string, error) {
	action, _ := call.Arguments["action"].(string)
	if action == "" {
		return "", fmt.Errorf("action argument is required")
	}
	if !allowedGitActions[action] {
		return "", fmt.Errorf("git action %q is not allowed; supported: init, clone, status, diff, log, add, commit, branch, checkout, show", action)
	}

	args, _ := call.Arguments["args"].(string)

	// Block dangerous flags in args.
	for _, blocked := range blockedGitFlags {
		if strings.Contains(args, blocked) {
			return "", fmt.Errorf("git argument %q is not allowed", blocked)
		}
	}
	if args != "" && containsShellMeta(args) {
		return "", fmt.Errorf("git_operation does not allow shell metacharacters in args")
	}

	switch action {
	case "clone":
		url, _ := call.Arguments["url"].(string)
		if url == "" {
			return "", fmt.Errorf("url argument is required for clone action")
		}
		if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "git@") {
			return "", fmt.Errorf("clone URL must start with https:// or git@")
		}
		if containsShellMeta(url) {
			return "", fmt.Errorf("clone URL contains invalid characters")
		}
		target := ""
		if args != "" {
			target = " " + args
		}
		return fmt.Sprintf("git clone --depth 1 %s%s", shellQuote(url), target), nil

	case "commit":
		message, _ := call.Arguments["message"].(string)
		if message == "" {
			return "", fmt.Errorf("message argument is required for commit action")
		}
		extraArgs := ""
		if args != "" {
			extraArgs = " " + args
		}
		return fmt.Sprintf("git commit -m %s%s", shellQuote(message), extraArgs), nil

	default:
		if args != "" {
			return fmt.Sprintf("git %s %s", action, args), nil
		}
		return fmt.Sprintf("git %s", action), nil
	}
}

// shellQuote wraps a string in single quotes, escaping any embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func gitOperationHandler(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
	command, err := buildGitCommand(call)
	if err != nil {
		return agentic.ToolResult{CallID: call.ID, Error: err.Error()}
	}
	if call.Arguments == nil {
		call.Arguments = make(map[string]any)
	}
	call.Arguments["command"] = command
	return runShellCommand(ctx, call, commandTimeout)
}

// =============================================================================
// BUILD PROJECT
// =============================================================================

const buildTimeout = 5 * time.Minute

// validBuildTarget checks that a target name is alphanumeric (with hyphens/underscores/colons/dots/slashes).
var validBuildTarget = regexp.MustCompile(`^[a-zA-Z0-9_./:=-]+$`)

// buildProjectCommand constructs a build command by auto-detecting the build system.
func buildProjectCommand(call agentic.ToolCall) (string, error) {
	target, _ := call.Arguments["target"].(string)
	if target != "" {
		if !validBuildTarget.MatchString(target) {
			return "", fmt.Errorf("build target must be alphanumeric with hyphens/underscores (got %q)", target)
		}
		if containsShellMeta(target) {
			return "", fmt.Errorf("build target contains invalid characters")
		}
	}

	type buildSys struct {
		detect     string
		name       string
		defaultCmd string
		targetFmt  string
	}

	systems := []buildSys{
		{"[ -f build.gradle ] || [ -f build.gradle.kts ]", "Gradle", "gradle build", "gradle %s"},
		{"[ -f go.mod ]", "Go", "go build ./...", "go build %s"},
		{"[ -f Cargo.toml ]", "Cargo", "cargo build", "cargo %s"},
		{"[ -f pom.xml ]", "Maven", "mvn package -q", "mvn %s"},
		{"[ -f package.json ]", "npm", "npm run build", "npm run %s"},
		{"[ -f Makefile ]", "Make", "make", "make %s"},
	}

	var b strings.Builder
	b.WriteString("set -e\n")
	for i, sys := range systems {
		prefix := "elif"
		if i == 0 {
			prefix = "if"
		}
		cmd := sys.defaultCmd
		if target != "" {
			cmd = fmt.Sprintf(sys.targetFmt, target)
		}
		fmt.Fprintf(&b, "%s %s; then echo 'Detected: %s' && %s\n", prefix, sys.detect, sys.name, cmd)
	}
	b.WriteString("else echo 'ERROR: No recognized build system found (checked: build.gradle, go.mod, Cargo.toml, pom.xml, package.json, Makefile)' && exit 1; fi")

	return b.String(), nil
}

func buildProjectHandler(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
	command, err := buildProjectCommand(call)
	if err != nil {
		return agentic.ToolResult{CallID: call.ID, Error: err.Error()}
	}
	if call.Arguments == nil {
		call.Arguments = make(map[string]any)
	}
	call.Arguments["command"] = command
	return runShellCommand(ctx, call, buildTimeout)
}

// =============================================================================
// MANAGE DEPENDENCIES
// =============================================================================

// validPackageName allows standard package name characters: alphanumeric, hyphens,
// underscores, dots, slashes (for Go/Java), @ (for npm scoped packages).
var validPackageName = regexp.MustCompile(`^[@a-zA-Z0-9][a-zA-Z0-9._/-]*$`)

// buildManageDepsCommand constructs a dependency management command.
func buildManageDepsCommand(call agentic.ToolCall) (string, error) {
	action, _ := call.Arguments["action"].(string)
	if action == "" {
		return "", fmt.Errorf("action argument is required")
	}

	validActions := map[string]bool{"install": true, "add": true, "remove": true, "list": true, "tidy": true}
	if !validActions[action] {
		return "", fmt.Errorf("action %q not supported; use install, add, remove, list, or tidy", action)
	}

	var packages []string
	if pkgsRaw, ok := call.Arguments["packages"].([]any); ok {
		for _, p := range pkgsRaw {
			if s, ok := p.(string); ok && s != "" {
				if !validPackageName.MatchString(s) {
					return "", fmt.Errorf("invalid package name: %q", s)
				}
				packages = append(packages, s)
			}
		}
	}

	if (action == "add" || action == "remove") && len(packages) == 0 {
		return "", fmt.Errorf("packages argument is required for %s action", action)
	}

	pkgStr := strings.Join(packages, " ")

	switch action {
	case "install":
		return `set -e
if [ -f go.mod ]; then echo "Detected: Go" && go mod download
elif [ -f package.json ]; then echo "Detected: npm" && npm install
elif [ -f requirements.txt ]; then echo "Detected: pip" && pip install -r requirements.txt
elif [ -f pyproject.toml ]; then echo "Detected: pip" && pip install .
elif [ -f Cargo.toml ]; then echo "Detected: Cargo" && cargo fetch
elif [ -f pom.xml ]; then echo "Detected: Maven" && mvn dependency:resolve -q
elif [ -f build.gradle ] || [ -f build.gradle.kts ]; then echo "Detected: Gradle" && gradle dependencies --quiet
else echo "ERROR: No recognized package manager found" && exit 1; fi`, nil

	case "add":
		return fmt.Sprintf(`set -e
if [ -f go.mod ]; then echo "Detected: Go" && go get %s
elif [ -f package.json ]; then echo "Detected: npm" && npm install %s
elif [ -f requirements.txt ]; then echo "Detected: pip" && pip install %s
elif [ -f Cargo.toml ]; then echo "Detected: Cargo" && cargo add %s
elif [ -f pom.xml ]; then echo "ERROR: Maven add not supported — edit pom.xml directly" && exit 1
else echo "ERROR: No recognized package manager found" && exit 1; fi`, pkgStr, pkgStr, pkgStr, pkgStr), nil

	case "remove":
		return fmt.Sprintf(`set -e
if [ -f go.mod ]; then echo "ERROR: Go — remove module from go.mod then run 'manage_dependencies' with action 'tidy'" && exit 1
elif [ -f package.json ]; then echo "Detected: npm" && npm uninstall %s
elif [ -f Cargo.toml ]; then echo "Detected: Cargo" && cargo remove %s
else echo "ERROR: No recognized package manager found" && exit 1; fi`, pkgStr, pkgStr), nil

	case "list":
		return `set -e
if [ -f go.mod ]; then echo "Detected: Go" && go list -m all
elif [ -f package.json ]; then echo "Detected: npm" && npm list --depth=0
elif [ -f requirements.txt ]; then echo "Detected: pip" && pip list
elif [ -f Cargo.toml ]; then echo "Detected: Cargo" && cargo tree --depth 1
elif [ -f pom.xml ]; then echo "Detected: Maven" && mvn dependency:list -q
elif [ -f build.gradle ] || [ -f build.gradle.kts ]; then echo "Detected: Gradle" && gradle dependencies
else echo "ERROR: No recognized package manager found" && exit 1; fi`, nil

	case "tidy":
		return `set -e
if [ -f go.mod ]; then echo "Detected: Go" && go mod tidy
elif [ -f package.json ]; then echo "Detected: npm" && npm prune && npm dedupe
elif [ -f Cargo.toml ]; then echo "Detected: Cargo" && cargo update
else echo "ERROR: No recognized package manager supports tidy" && exit 1; fi`, nil

	default:
		return "", fmt.Errorf("action %q not supported", action)
	}
}

func manageDepsHandler(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
	command, err := buildManageDepsCommand(call)
	if err != nil {
		return agentic.ToolResult{CallID: call.ID, Error: err.Error()}
	}
	if call.Arguments == nil {
		call.Arguments = make(map[string]any)
	}
	call.Arguments["command"] = command
	return runShellCommand(ctx, call, buildTimeout)
}
