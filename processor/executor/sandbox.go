package executor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semstreams/agentic"
)

// =============================================================================
// SANDBOX CLIENT
// =============================================================================
// SandboxClient proxies file and execution operations to an isolated sandbox
// container over HTTP. This keeps the main process clean and provides a
// hard isolation boundary for agent-generated code and commands.
// =============================================================================

const (
	// sandboxHTTPTimeout is the HTTP client timeout for sandbox requests.
	// Set higher than commandTimeout to account for long-running test suites
	// inside the container.
	sandboxHTTPTimeout = 5 * time.Minute
)

// SandboxClient is an HTTP client for the sandbox container API.
type SandboxClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewSandboxClient creates a SandboxClient targeting the given base URL.
// A 30-second timeout HTTP client is created. The per-request context timeout
// set by each tool handler (via commandTimeout) governs actual execution time;
// the client timeout is a safety backstop for network-level hangs.
func NewSandboxClient(baseURL string) *SandboxClient {
	return &SandboxClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: sandboxHTTPTimeout,
		},
	}
}

// CreateWorkspace creates an isolated workspace for the given quest ID.
// When repo is non-empty, the sandbox creates a git worktree from the named
// repo's main branch. When repo is empty, a plain directory is created.
func (c *SandboxClient) CreateWorkspace(ctx context.Context, questID string, repo ...string) error {
	u := c.baseURL + "/workspace/" + questID
	if len(repo) > 0 && repo[0] != "" {
		u += "?repo=" + repo[0]
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return fmt.Errorf("create workspace request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("create workspace returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// MergeToMain merges the quest branch into the target repo's main branch.
// Called by bossbattle after victory. Returns the merge commit hash and
// the list of changed file paths.
func (c *SandboxClient) MergeToMain(ctx context.Context, questID string) (commitHash string, filesChanged []string, err error) {
	var result struct {
		CommitHash   string   `json:"commit_hash"`
		FilesChanged []string `json:"files_changed"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/workspace/"+questID+"/merge-to-main", nil, &result); err != nil {
		return "", nil, fmt.Errorf("merge-to-main: %w", err)
	}
	return result.CommitHash, result.FilesChanged, nil
}

// GitCommitAll stages all changes and commits with the given message.
// Returns the commit hash and number of files changed, or empty hash if
// nothing to commit.
func (c *SandboxClient) GitCommitAll(ctx context.Context, questID, message string) (commitHash string, filesChanged int, err error) {
	body := struct {
		Message string `json:"message"`
	}{Message: message}
	var result struct {
		CommitHash   string `json:"commit_hash"`
		FilesChanged int    `json:"files_changed"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/workspace/"+questID+"/git/commit-all", body, &result); err != nil {
		return "", 0, fmt.Errorf("git commit-all: %w", err)
	}
	return result.CommitHash, result.FilesChanged, nil
}

// DeleteWorkspace removes the workspace for the given quest ID.
func (c *SandboxClient) DeleteWorkspace(ctx context.Context, questID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/workspace/"+questID, nil)
	if err != nil {
		return fmt.Errorf("delete workspace request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete workspace: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("delete workspace returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// doJSON sends an HTTP request with a JSON body and decodes the JSON response
// into out. Pass nil for body to send a request with no body (e.g. GET/DELETE).
func (c *SandboxClient) doJSON(ctx context.Context, method, path string, body, out any) error {
	var reqBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("sandbox returned %d: %s", resp.StatusCode, string(errBody))
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	} else {
		// Drain body to enable HTTP connection reuse.
		_, _ = io.Copy(io.Discard, resp.Body)
	}
	return nil
}

// =============================================================================
// Sandbox API request/response types
// These mirror the types in cmd/sandbox/server.go but are defined locally to
// avoid importing a cmd package from a library package.
// =============================================================================

type sandboxReadFileReq struct {
	QuestID string `json:"quest_id"`
	Path    string `json:"path"`
}

type sandboxReadFileResp struct {
	Content string `json:"content"`
	Size    int    `json:"size"`
}

type sandboxWriteFileReq struct {
	QuestID string `json:"quest_id"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

type sandboxExecReq struct {
	QuestID   string `json:"quest_id"`
	Command   string `json:"command"`
	TimeoutMS int    `json:"timeout_ms"`
}

type sandboxExecResp struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	TimedOut bool   `json:"timed_out"`
}

type sandboxListReq struct {
	QuestID string `json:"quest_id"`
	Path    string `json:"path"`
}

type sandboxDirEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

type sandboxListResp struct {
	Entries []sandboxDirEntry `json:"entries"`
}

type sandboxSearchReq struct {
	QuestID      string `json:"quest_id"`
	Pattern      string `json:"pattern"`
	Path         string `json:"path"`
	FileGlob     string `json:"file_glob"`
	ContextLines int    `json:"context_lines"`
}

type sandboxSearchResp struct {
	Output string `json:"output"`
}

// ListWorkspaceFiles returns all files in the workspace for the given quest ID.
func (c *SandboxClient) ListWorkspaceFiles(ctx context.Context, questID string) ([]WorkspaceFileEntry, error) {
	listURL := fmt.Sprintf("/workspace/%s", questID)
	var resp sandboxListResp
	if err := c.doJSON(ctx, http.MethodGet, listURL, nil, &resp); err != nil {
		return nil, fmt.Errorf("list workspace files: %w", err)
	}
	entries := make([]WorkspaceFileEntry, 0, len(resp.Entries))
	for _, e := range resp.Entries {
		if !e.IsDir {
			entries = append(entries, WorkspaceFileEntry{
				Path: e.Name,
				Size: e.Size,
			})
		}
	}
	return entries, nil
}

// ReadFile reads a single file from the workspace. Returns the raw content bytes.
func (c *SandboxClient) ReadFile(ctx context.Context, questID, path string) ([]byte, error) {
	fileURL := "/file?" + url.Values{"quest_id": {questID}, "path": {path}}.Encode()
	var resp sandboxReadFileResp
	if err := c.doJSON(ctx, http.MethodGet, fileURL, nil, &resp); err != nil {
		return nil, fmt.Errorf("read file %s: %w", path, err)
	}
	return []byte(resp.Content), nil
}

// WorkspaceFileEntry describes a file in a sandbox workspace.
type WorkspaceFileEntry struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// =============================================================================
// Quest ID extraction
// =============================================================================

// questIDFromCall extracts the quest_id from the tool call metadata.
// questbridge always sets this key before dispatching to questtools.
func questIDFromCall(call agentic.ToolCall) (string, bool) {
	if call.Metadata == nil {
		return "", false
	}
	id, ok := call.Metadata["quest_id"].(string)
	return id, ok && id != ""
}

// =============================================================================
// RegisterSandboxTools
// =============================================================================
// RegisterSandboxTools replaces the file/exec tool handlers registered by
// RegisterBuiltins with sandbox-proxied versions. Call RegisterBuiltins first,
// then RegisterSandboxTools — the latter overwrites only the tools that need
// proxying, leaving terminal and DAG tools untouched.
//
// The tool definitions (name, description, parameters, tier, skills) are
// identical to the builtin versions so the agent sees no change.
// =============================================================================

// RegisterSandboxTools registers sandbox-proxied handlers for all file and
// execution tools. Only call this after RegisterBuiltins so that terminal and
// DAG tool registrations are preserved.
func (r *ToolRegistry) RegisterSandboxTools(client *SandboxClient) {
	r.Register(RegisteredTool{
		Definition: readFileSpec.Definition,
		Handler:    makeSandboxReadFileHandler(client),
		Skills:     readFileSpec.Skills,
		MinTier:    readFileSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: readFileRangeSpec.Definition,
		Handler:    makeSandboxReadFileRangeHandler(client),
		Skills:     readFileRangeSpec.Skills,
		MinTier:    readFileRangeSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: writeFileSpec.Definition,
		Handler:    makeSandboxWriteFileHandler(client),
		Skills:     writeFileSpec.Skills,
		MinTier:    writeFileSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: patchFileSpec.Definition,
		Handler:    makeSandboxPatchFileHandler(client),
		Skills:     patchFileSpec.Skills,
		MinTier:    patchFileSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: deleteFileSpec.Definition,
		Handler:    makeSandboxDeleteFileHandler(client),
		Skills:     deleteFileSpec.Skills,
		MinTier:    deleteFileSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: renameFileSpec.Definition,
		Handler:    makeSandboxRenameFileHandler(client),
		Skills:     renameFileSpec.Skills,
		MinTier:    renameFileSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: createDirectorySpec.Definition,
		Handler:    makeSandboxCreateDirectoryHandler(client),
		Skills:     createDirectorySpec.Skills,
		MinTier:    createDirectorySpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: listDirectorySpec.Definition,
		Handler:    makeSandboxListDirectoryHandler(client),
		Skills:     listDirectorySpec.Skills,
		MinTier:    listDirectorySpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: globFilesSpec.Definition,
		Handler:    makeSandboxGlobFilesHandler(client),
		Skills:     globFilesSpec.Skills,
		MinTier:    globFilesSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: searchTextSpec.Definition,
		Handler:    makeSandboxSearchTextHandler(client),
		Skills:     searchTextSpec.Skills,
		MinTier:    searchTextSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: runTestsSpec.Definition,
		Handler:    makeSandboxRunTestsHandler(client),
		Skills:     runTestsSpec.Skills,
		MinTier:    runTestsSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: lintCheckSpec.Definition,
		Handler:    makeSandboxLintCheckHandler(client),
		Skills:     lintCheckSpec.Skills,
		MinTier:    lintCheckSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: runCommandSpec.Definition,
		Handler:    makeSandboxRunCommandHandler(client),
		Skills:     runCommandSpec.Skills,
		MinTier:    runCommandSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: httpRequestSpec.Definition,
		Handler:    makeSandboxHTTPRequestHandler(client),
		Skills:     httpRequestSpec.Skills,
		MinTier:    httpRequestSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: inspectEnvironmentSpec.Definition,
		Handler: makeSandboxExecHandler(
			client,
			15*time.Second,
			func(_ agentic.ToolCall) (string, error) { return inspectEnvironmentScript, nil },
			nil,
		),
		Skills:  inspectEnvironmentSpec.Skills,
		MinTier: inspectEnvironmentSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: gitOperationSpec.Definition,
		Handler: makeSandboxExecHandler(
			client,
			commandTimeout,
			func(call agentic.ToolCall) (string, error) { return buildGitCommand(call) },
			nil,
		),
		Skills:  gitOperationSpec.Skills,
		MinTier: gitOperationSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: buildProjectSpec.Definition,
		Handler: makeSandboxExecHandler(
			client,
			buildTimeout,
			func(call agentic.ToolCall) (string, error) { return buildProjectCommand(call) },
			nil,
		),
		Skills:  buildProjectSpec.Skills,
		MinTier: buildProjectSpec.MinTier,
	})

	r.Register(RegisteredTool{
		Definition: manageDependenciesSpec.Definition,
		Handler: makeSandboxExecHandler(
			client,
			buildTimeout,
			func(call agentic.ToolCall) (string, error) { return buildManageDepsCommand(call) },
			nil,
		),
		Skills:  manageDependenciesSpec.Skills,
		MinTier: manageDependenciesSpec.MinTier,
	})
}

// =============================================================================
// SANDBOX TOOL HANDLERS
// =============================================================================

func makeSandboxReadFileHandler(client *SandboxClient) ToolHandler {
	return func(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
		select {
		case <-ctx.Done():
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("operation cancelled: %v", ctx.Err())}
		default:
		}

		questID, ok := questIDFromCall(call)
		if !ok {
			return agentic.ToolResult{CallID: call.ID, Error: "quest_id missing from tool call metadata"}
		}

		path, _ := call.Arguments["path"].(string)
		if path == "" {
			return agentic.ToolResult{CallID: call.ID, Error: "path argument is required"}
		}

		// The sandbox API uses GET with query parameters for reads.
		fileURL := "/file?" + url.Values{"quest_id": {questID}, "path": {path}}.Encode()
		var resp sandboxReadFileResp
		if err := client.doJSON(ctx, http.MethodGet, fileURL, nil, &resp); err != nil {
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("failed to read file: %v", err)}
		}

		content := resp.Content
		if len(content) > maxFileReadSize {
			content = content[:maxFileReadSize] + "\n... (truncated)"
		}

		return agentic.ToolResult{CallID: call.ID, Content: content}
	}
}

func makeSandboxReadFileRangeHandler(client *SandboxClient) ToolHandler {
	return func(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
		select {
		case <-ctx.Done():
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("operation cancelled: %v", ctx.Err())}
		default:
		}

		questID, ok := questIDFromCall(call)
		if !ok {
			return agentic.ToolResult{CallID: call.ID, Error: "quest_id missing from tool call metadata"}
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

		fileURL := "/file?" + url.Values{"quest_id": {questID}, "path": {path}}.Encode()
		var resp sandboxReadFileResp
		if err := client.doJSON(ctx, http.MethodGet, fileURL, nil, &resp); err != nil {
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("failed to read file: %v", err)}
		}

		// Slice the requested line range in the handler.
		scanner := bufio.NewScanner(strings.NewReader(resp.Content))
		var sb strings.Builder
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

		if sb.Len() == 0 {
			return agentic.ToolResult{
				CallID:  call.ID,
				Content: fmt.Sprintf("File has fewer than %d lines", startLine),
			}
		}

		return agentic.ToolResult{CallID: call.ID, Content: sb.String()}
	}
}

func makeSandboxWriteFileHandler(client *SandboxClient) ToolHandler {
	return func(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
		select {
		case <-ctx.Done():
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("operation cancelled: %v", ctx.Err())}
		default:
		}

		questID, ok := questIDFromCall(call)
		if !ok {
			return agentic.ToolResult{CallID: call.ID, Error: "quest_id missing from tool call metadata"}
		}

		path, _ := call.Arguments["path"].(string)
		if path == "" {
			return agentic.ToolResult{CallID: call.ID, Error: "path argument is required"}
		}
		contentVal, hasContent := call.Arguments["content"]
		if !hasContent {
			return agentic.ToolResult{CallID: call.ID, Error: "content argument is required"}
		}
		content, ok := contentVal.(string)
		if !ok {
			return agentic.ToolResult{CallID: call.ID, Error: "content argument must be a string"}
		}

		if len(content) > maxFileWriteSize {
			return agentic.ToolResult{
				CallID: call.ID,
				Error:  fmt.Sprintf("content too large: %d bytes (max %d)", len(content), maxFileWriteSize),
			}
		}

		req := sandboxWriteFileReq{QuestID: questID, Path: path, Content: content}
		if err := client.doJSON(ctx, http.MethodPut, "/file", req, nil); err != nil {
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("failed to write file: %v", err)}
		}

		return agentic.ToolResult{
			CallID:  call.ID,
			Content: fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path),
		}
	}
}

func makeSandboxPatchFileHandler(client *SandboxClient) ToolHandler {
	return func(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
		select {
		case <-ctx.Done():
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("operation cancelled: %v", ctx.Err())}
		default:
		}

		questID, ok := questIDFromCall(call)
		if !ok {
			return agentic.ToolResult{CallID: call.ID, Error: "quest_id missing from tool call metadata"}
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

		// Read the current file content from the sandbox.
		readURL := "/file?" + url.Values{"quest_id": {questID}, "path": {path}}.Encode()
		var readResp sandboxReadFileResp
		if err := client.doJSON(ctx, http.MethodGet, readURL, nil, &readResp); err != nil {
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("failed to read file: %v", err)}
		}

		fileContent := readResp.Content
		if !strings.Contains(fileContent, oldText) {
			return agentic.ToolResult{CallID: call.ID, Error: "old_text not found in file"}
		}

		count := strings.Count(fileContent, oldText)
		if count > 1 {
			return agentic.ToolResult{
				CallID: call.ID,
				Error:  fmt.Sprintf("old_text is ambiguous: found %d occurrences (must be unique)", count),
			}
		}

		newContent := strings.Replace(fileContent, oldText, newText, 1)
		if len(newContent) > maxFileWriteSize {
			return agentic.ToolResult{
				CallID: call.ID,
				Error:  fmt.Sprintf("resulting file too large: %d bytes (max %d)", len(newContent), maxFileWriteSize),
			}
		}

		// Write the patched content back to the sandbox.
		writeReq := sandboxWriteFileReq{QuestID: questID, Path: path, Content: newContent}
		if err := client.doJSON(ctx, http.MethodPut, "/file", writeReq, nil); err != nil {
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("failed to write patched file: %v", err)}
		}

		msg := fmt.Sprintf("Successfully patched %s (%d bytes -> %d bytes)", path, len(oldText), len(newText))
		if newText == "" {
			msg = fmt.Sprintf("Successfully removed %d bytes from %s", len(oldText), path)
		}

		return agentic.ToolResult{CallID: call.ID, Content: msg}
	}
}

func makeSandboxDeleteFileHandler(client *SandboxClient) ToolHandler {
	return func(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
		select {
		case <-ctx.Done():
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("operation cancelled: %v", ctx.Err())}
		default:
		}

		questID, ok := questIDFromCall(call)
		if !ok {
			return agentic.ToolResult{CallID: call.ID, Error: "quest_id missing from tool call metadata"}
		}

		path, _ := call.Arguments["path"].(string)
		if path == "" {
			return agentic.ToolResult{CallID: call.ID, Error: "path argument is required"}
		}

		req := sandboxExecReq{
			QuestID:   questID,
			Command:   fmt.Sprintf("rm -- %q", path),
			TimeoutMS: int(commandTimeout.Milliseconds()),
		}
		var resp sandboxExecResp
		if err := client.doJSON(ctx, http.MethodPost, "/exec", req, &resp); err != nil {
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("delete_file: %v", err)}
		}

		if resp.TimedOut {
			return agentic.ToolResult{CallID: call.ID, Error: "delete_file timed out"}
		}
		if resp.ExitCode != 0 {
			return agentic.ToolResult{
				CallID: call.ID,
				Error:  fmt.Sprintf("failed to delete file: %s", strings.TrimSpace(resp.Stderr)),
			}
		}

		return agentic.ToolResult{CallID: call.ID, Content: fmt.Sprintf("Deleted: %s", path)}
	}
}

func makeSandboxRenameFileHandler(client *SandboxClient) ToolHandler {
	return func(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
		select {
		case <-ctx.Done():
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("operation cancelled: %v", ctx.Err())}
		default:
		}

		questID, ok := questIDFromCall(call)
		if !ok {
			return agentic.ToolResult{CallID: call.ID, Error: "quest_id missing from tool call metadata"}
		}

		oldPath, _ := call.Arguments["old_path"].(string)
		newPath, _ := call.Arguments["new_path"].(string)
		if oldPath == "" {
			return agentic.ToolResult{CallID: call.ID, Error: "old_path argument is required"}
		}
		if newPath == "" {
			return agentic.ToolResult{CallID: call.ID, Error: "new_path argument is required"}
		}

		req := sandboxExecReq{
			QuestID:   questID,
			Command:   fmt.Sprintf("mv -- %q %q", oldPath, newPath),
			TimeoutMS: int(commandTimeout.Milliseconds()),
		}
		var resp sandboxExecResp
		if err := client.doJSON(ctx, http.MethodPost, "/exec", req, &resp); err != nil {
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("rename_file: %v", err)}
		}

		if resp.TimedOut {
			return agentic.ToolResult{CallID: call.ID, Error: "rename_file timed out"}
		}
		if resp.ExitCode != 0 {
			return agentic.ToolResult{
				CallID: call.ID,
				Error:  fmt.Sprintf("rename failed: %s", strings.TrimSpace(resp.Stderr)),
			}
		}

		return agentic.ToolResult{CallID: call.ID, Content: fmt.Sprintf("Renamed %s -> %s", oldPath, newPath)}
	}
}

func makeSandboxCreateDirectoryHandler(client *SandboxClient) ToolHandler {
	return func(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
		select {
		case <-ctx.Done():
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("operation cancelled: %v", ctx.Err())}
		default:
		}

		questID, ok := questIDFromCall(call)
		if !ok {
			return agentic.ToolResult{CallID: call.ID, Error: "quest_id missing from tool call metadata"}
		}

		path, _ := call.Arguments["path"].(string)
		if path == "" {
			return agentic.ToolResult{CallID: call.ID, Error: "path argument is required"}
		}

		req := sandboxExecReq{
			QuestID:   questID,
			Command:   fmt.Sprintf("mkdir -p -- %q", path),
			TimeoutMS: int(commandTimeout.Milliseconds()),
		}
		var resp sandboxExecResp
		if err := client.doJSON(ctx, http.MethodPost, "/exec", req, &resp); err != nil {
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("create_directory: %v", err)}
		}

		if resp.TimedOut {
			return agentic.ToolResult{CallID: call.ID, Error: "create_directory timed out"}
		}
		if resp.ExitCode != 0 {
			return agentic.ToolResult{
				CallID: call.ID,
				Error:  fmt.Sprintf("failed to create directory: %s", strings.TrimSpace(resp.Stderr)),
			}
		}

		return agentic.ToolResult{CallID: call.ID, Content: fmt.Sprintf("Created directory: %s", path)}
	}
}

func makeSandboxListDirectoryHandler(client *SandboxClient) ToolHandler {
	return func(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
		select {
		case <-ctx.Done():
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("operation cancelled: %v", ctx.Err())}
		default:
		}

		questID, ok := questIDFromCall(call)
		if !ok {
			return agentic.ToolResult{CallID: call.ID, Error: "quest_id missing from tool call metadata"}
		}

		path, _ := call.Arguments["path"].(string)
		if path == "" {
			return agentic.ToolResult{CallID: call.ID, Error: "path argument is required"}
		}

		req := sandboxListReq{QuestID: questID, Path: path}
		var resp sandboxListResp
		if err := client.doJSON(ctx, http.MethodPost, "/list", req, &resp); err != nil {
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("failed to read directory: %v", err)}
		}

		var result strings.Builder
		for _, entry := range resp.Entries {
			if entry.IsDir {
				result.WriteString(fmt.Sprintf("[dir]  %s/\n", entry.Name))
			} else {
				result.WriteString(fmt.Sprintf("[file] %s (%d bytes)\n", entry.Name, entry.Size))
			}
		}

		return agentic.ToolResult{CallID: call.ID, Content: result.String()}
	}
}

func makeSandboxGlobFilesHandler(client *SandboxClient) ToolHandler {
	return func(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
		select {
		case <-ctx.Done():
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("operation cancelled: %v", ctx.Err())}
		default:
		}

		questID, ok := questIDFromCall(call)
		if !ok {
			return agentic.ToolResult{CallID: call.ID, Error: "quest_id missing from tool call metadata"}
		}

		pattern, _ := call.Arguments["pattern"].(string)
		if pattern == "" {
			return agentic.ToolResult{CallID: call.ID, Error: "pattern argument is required"}
		}

		basePath := "."
		if p, ok := call.Arguments["path"].(string); ok && p != "" {
			basePath = p
		}

		// The sandbox does not have a native glob endpoint; use find via POST /exec.
		// Use -path for ** compatibility (POSIX find doesn't support **, so we
		// replicate the local handler's behaviour: list all files then filter).
		// Simpler and more reliable: list all files via GET /workspace/{id} and
		// apply the same matchGlobPattern logic locally.
		listURL := fmt.Sprintf("/workspace/%s", questID)
		var listResp sandboxListResp
		if err := client.doJSON(ctx, http.MethodGet, listURL, nil, &listResp); err != nil {
			// Fall back to find command if workspace listing is unavailable.
			findCmd := fmt.Sprintf("find %q -type f -name %q 2>/dev/null | head -n %d", basePath, pattern, maxGlobResults)
			execReq := sandboxExecReq{
				QuestID:   questID,
				Command:   findCmd,
				TimeoutMS: int(commandTimeout.Milliseconds()),
			}
			var execResp sandboxExecResp
			if execErr := client.doJSON(ctx, http.MethodPost, "/exec", execReq, &execResp); execErr != nil {
				return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("glob_files: %v", execErr)}
			}
			output := strings.TrimSpace(execResp.Stdout)
			if output == "" {
				return agentic.ToolResult{
					CallID:  call.ID,
					Content: fmt.Sprintf("No files matched pattern %q in %s", pattern, basePath),
				}
			}
			return agentic.ToolResult{CallID: call.ID, Content: output + "\n"}
		}

		// Filter entries using the same matchGlobPattern logic as the local handler.
		var results []string
		for _, entry := range listResp.Entries {
			if entry.IsDir {
				continue
			}
			if len(results) >= maxGlobResults {
				break
			}
			// entry.Name is the relative path within the workspace.
			if matchGlobPattern(pattern, entry.Name) {
				results = append(results, entry.Name)
			}
		}

		if len(results) == 0 {
			return agentic.ToolResult{
				CallID:  call.ID,
				Content: fmt.Sprintf("No files matched pattern %q in %s", pattern, basePath),
			}
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
}

func makeSandboxSearchTextHandler(client *SandboxClient) ToolHandler {
	return func(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
		select {
		case <-ctx.Done():
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("operation cancelled: %v", ctx.Err())}
		default:
		}

		questID, ok := questIDFromCall(call)
		if !ok {
			return agentic.ToolResult{CallID: call.ID, Error: "quest_id missing from tool call metadata"}
		}

		pattern, _ := call.Arguments["pattern"].(string)
		if pattern == "" {
			return agentic.ToolResult{CallID: call.ID, Error: "pattern argument must be a string"}
		}
		path, _ := call.Arguments["path"].(string)
		if path == "" {
			return agentic.ToolResult{CallID: call.ID, Error: "path argument is required"}
		}

		var fileGlob string
		if fg, ok := call.Arguments["file_glob"].(string); ok {
			fileGlob = fg
		}

		contextLines := 0
		if cl, ok := call.Arguments["context_lines"].(float64); ok {
			contextLines = int(cl)
			if contextLines > maxContextLines {
				contextLines = maxContextLines
			}
		}

		req := sandboxSearchReq{
			QuestID:      questID,
			Pattern:      pattern,
			Path:         path,
			FileGlob:     fileGlob,
			ContextLines: contextLines,
		}
		var resp sandboxSearchResp
		if err := client.doJSON(ctx, http.MethodPost, "/search", req, &resp); err != nil {
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("search_text: %v", err)}
		}

		output := strings.TrimSpace(resp.Output)
		if output == "" {
			return agentic.ToolResult{
				CallID:  call.ID,
				Content: fmt.Sprintf("No matches found for '%s' in %s", pattern, path),
			}
		}

		// Enforce the client-side match limit by counting lines in the output.
		lines := strings.Split(output, "\n")
		if len(lines) > maxSearchMatches {
			lines = lines[:maxSearchMatches]
			output = strings.Join(lines, "\n") + fmt.Sprintf("\n\n... (showing first %d matches)", maxSearchMatches)
		}

		return agentic.ToolResult{CallID: call.ID, Content: output}
	}
}

// makeSandboxExecHandler builds a handler that proxies a shell command to the
// sandbox /exec endpoint. The command string is derived from the tool call
// arguments by the supplied commandFn. Validation (allowlist, metacharacter
// check) is performed by validationFn before the command is dispatched.
func makeSandboxExecHandler(
	client *SandboxClient,
	timeout time.Duration,
	commandFn func(call agentic.ToolCall) (string, error),
	validationFn func(command string) error,
) ToolHandler {
	return func(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
		select {
		case <-ctx.Done():
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("operation cancelled: %v", ctx.Err())}
		default:
		}

		questID, ok := questIDFromCall(call)
		if !ok {
			return agentic.ToolResult{CallID: call.ID, Error: "quest_id missing from tool call metadata"}
		}

		command, err := commandFn(call)
		if err != nil {
			return agentic.ToolResult{CallID: call.ID, Error: err.Error()}
		}

		if validationFn != nil {
			if err := validationFn(command); err != nil {
				return agentic.ToolResult{CallID: call.ID, Error: err.Error()}
			}
		}

		req := sandboxExecReq{
			QuestID:   questID,
			Command:   command,
			TimeoutMS: int(timeout.Milliseconds()),
		}
		var resp sandboxExecResp
		if err := client.doJSON(ctx, http.MethodPost, "/exec", req, &resp); err != nil {
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("exec failed: %v", err)}
		}

		var result strings.Builder
		if resp.Stdout != "" {
			result.WriteString(resp.Stdout)
			if len(resp.Stdout) >= maxCommandOutput {
				result.WriteString("\n... (stdout truncated)")
			}
		}
		if resp.Stderr != "" {
			if result.Len() > 0 {
				result.WriteString("\n\n--- stderr ---\n")
			}
			result.WriteString(resp.Stderr)
			if len(resp.Stderr) >= maxCommandOutput {
				result.WriteString("\n... (stderr truncated)")
			}
		}

		if resp.TimedOut {
			return agentic.ToolResult{
				CallID:  call.ID,
				Content: result.String(),
				Error:   fmt.Sprintf("command timed out after %s", timeout),
			}
		}

		if resp.ExitCode != 0 {
			return agentic.ToolResult{
				CallID:  call.ID,
				Content: result.String(),
				Error:   fmt.Sprintf("command failed with exit code %d", resp.ExitCode),
			}
		}

		if result.Len() == 0 {
			result.WriteString("(no output)")
		}

		return agentic.ToolResult{CallID: call.ID, Content: result.String()}
	}
}

func makeSandboxRunTestsHandler(client *SandboxClient) ToolHandler {
	return makeSandboxExecHandler(
		client,
		commandTimeout,
		func(call agentic.ToolCall) (string, error) {
			command, _ := call.Arguments["command"].(string)
			if command == "" {
				return "", fmt.Errorf("command argument is required")
			}
			return command, nil
		},
		func(command string) error {
			if containsShellMeta(command) {
				return fmt.Errorf("run_tests does not allow shell metacharacters (;, &&, ||, |, $, `, >, <) — use run_command for compound commands")
			}
			for _, prefix := range allowedTestPrefixes {
				if strings.HasPrefix(command, prefix) {
					return nil
				}
			}
			return fmt.Errorf("run_tests only allows test commands (e.g. 'go test ./...', 'npm test') — use run_command for general commands")
		},
	)
}

func makeSandboxLintCheckHandler(client *SandboxClient) ToolHandler {
	return makeSandboxExecHandler(
		client,
		commandTimeout,
		func(call agentic.ToolCall) (string, error) {
			command, _ := call.Arguments["command"].(string)
			if command == "" {
				return "", fmt.Errorf("command argument is required")
			}
			return command, nil
		},
		func(command string) error {
			if containsShellMeta(command) {
				return fmt.Errorf("lint_check does not allow shell metacharacters (;, &&, ||, |, $, `, >, <) — use run_command for compound commands")
			}
			for _, prefix := range allowedLintPrefixes {
				if strings.HasPrefix(command, prefix) {
					return nil
				}
			}
			return fmt.Errorf("lint_check only allows linter commands (e.g. 'go vet ./...', 'golangci-lint run') — use run_command for general commands")
		},
	)
}

func makeSandboxRunCommandHandler(client *SandboxClient) ToolHandler {
	return makeSandboxExecHandler(
		client,
		commandTimeout,
		func(call agentic.ToolCall) (string, error) {
			command, _ := call.Arguments["command"].(string)
			if command == "" {
				return "", fmt.Errorf("command argument is required")
			}
			return command, nil
		},
		nil, // run_command has no allowlist — Master-tier gate is enforced by the registry
	)
}

// makeSandboxHTTPRequestHandler proxies HTTP requests through the sandbox
// container's /exec endpoint using curl. This keeps outbound network traffic
// inside the sandbox boundary rather than originating from the host process.
func makeSandboxHTTPRequestHandler(client *SandboxClient) ToolHandler {
	return func(ctx context.Context, call agentic.ToolCall, _ *domain.Quest, _ *agentprogression.Agent) agentic.ToolResult {
		select {
		case <-ctx.Done():
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("operation cancelled: %v", ctx.Err())}
		default:
		}

		questID, ok := questIDFromCall(call)
		if !ok {
			return agentic.ToolResult{CallID: call.ID, Error: "quest_id missing from tool call metadata"}
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

		// Build a curl command that mimics the local httpRequestHandler.
		// -s: silent, -S: show errors, -L: follow redirects,
		// -m: timeout matching httpRequestTimeout,
		// -w '\n%{http_code}': append status code on its own line for parsing.
		var cmdParts []string
		cmdParts = append(cmdParts, "curl", "-s", "-S", "-L",
			fmt.Sprintf("-m %d", int(httpRequestTimeout.Seconds())),
			"-X", method,
		)

		if method == "POST" {
			body, _ := call.Arguments["body"].(string)
			contentType, _ := call.Arguments["content_type"].(string)
			if contentType == "" {
				contentType = "application/json"
			}
			if body != "" {
				cmdParts = append(cmdParts, "-d", fmt.Sprintf("%q", body))
			}
			cmdParts = append(cmdParts, "-H", fmt.Sprintf("Content-Type: %s", contentType))
		}

		cmdParts = append(cmdParts,
			"-H", "User-Agent: semdragons-agent/1.0",
			"-w", `'\n%{http_code}'`,
			fmt.Sprintf("%q", urlStr),
		)

		command := strings.Join(cmdParts, " ")

		req := sandboxExecReq{
			QuestID:   questID,
			Command:   command,
			TimeoutMS: int(httpRequestTimeout.Milliseconds()) + 5000, // small buffer over curl's own timeout
		}
		var resp sandboxExecResp
		if err := client.doJSON(ctx, http.MethodPost, "/exec", req, &resp); err != nil {
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("http_request: %v", err)}
		}

		if resp.TimedOut {
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("http_request timed out after %s", httpRequestTimeout)}
		}

		// curl appends "\n<status_code>" via -w; split on the last newline.
		output := strings.TrimRight(resp.Stdout, "\n")
		lastNL := strings.LastIndex(output, "\n")
		var body, statusCode string
		if lastNL >= 0 {
			body = output[:lastNL]
			statusCode = strings.TrimSpace(output[lastNL+1:])
		} else {
			body = output
			statusCode = strings.TrimSpace(output)
		}

		// Truncate oversized responses.
		if len(body) > maxHTTPResponseSize {
			body = body[:maxHTTPResponseSize] + "\n... (response truncated)"
		}

		if resp.ExitCode != 0 {
			errMsg := strings.TrimSpace(resp.Stderr)
			if errMsg == "" {
				errMsg = fmt.Sprintf("curl exited with code %d", resp.ExitCode)
			}
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("request failed: %s", errMsg)}
		}

		return agentic.ToolResult{
			CallID:  call.ID,
			Content: fmt.Sprintf("HTTP %s\n\n%s", statusCode, body),
		}
	}
}
