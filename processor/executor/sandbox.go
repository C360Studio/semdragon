package executor

import (
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
	"github.com/c360studio/semdragons/processor/executor/httpformat"
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

type sandboxReadFileResp struct {
	Content string `json:"content"`
	Size    int    `json:"size"`
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

type sandboxDirEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

type sandboxListResp struct {
	Entries []sandboxDirEntry `json:"entries"`
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

// RegisterSandboxTools registers sandbox-proxied handlers for execution and
// HTTP tools. Only call this after RegisterBuiltins so that terminal and
// DAG tool registrations are preserved.
func (r *ToolRegistry) RegisterSandboxTools(client *SandboxClient) {
	r.Register(RegisteredTool{
		Definition: runCommandSpec.Definition,
		Handler:    makeSandboxRunCommandHandler(client),
		Skills:     runCommandSpec.Skills,
		MinTier:    runCommandSpec.MinTier,
		Category:   runCommandSpec.Category,
	})

	r.Register(RegisteredTool{
		Definition: httpRequestSpec.Definition,
		Handler:    makeSandboxHTTPRequestHandler(client),
		Skills:     httpRequestSpec.Skills,
		MinTier:    httpRequestSpec.MinTier,
		Category:   httpRequestSpec.Category,
	})
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
		nil, // bash has no allowlist — Master-tier gate is enforced by the registry
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

		formatArg, _ := call.Arguments["format"].(string)
		format := httpformat.ParseFormat(formatArg)

		// Build a curl command that mimics the local httpRequestHandler.
		// -s: silent, -S: show errors, -L: follow redirects,
		// -m: timeout matching httpRequestTimeout,
		// -w 'STATUS\nCT': append status + content-type on a sentinel line
		// for parsing. Both fields are needed: status for the response
		// envelope, content-type so httpformat.Render can choose between
		// markdown conversion and raw passthrough.
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

		// curlSentinel is a marker line we use to separate the response body
		// from the status + content-type fields. Picked to be unlikely in any
		// real HTML/JSON payload.
		const curlSentinel = "\n__SEMDRAGONS_CURL_META__\n"
		writeOut := curlSentinel + `%{http_code}` + "\n" + `%{content_type}`

		cmdParts = append(cmdParts,
			"-H", "User-Agent: semdragons-agent/1.0",
			"-w", fmt.Sprintf("%q", writeOut),
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

		// Split body from the curl-write-out sentinel section. The sentinel
		// MUST appear; if it doesn't, curl errored before the format string
		// was emitted (e.g., DNS failure printed to stderr only).
		output := resp.Stdout
		var body, statusCode, contentType string
		if idx := strings.LastIndex(output, curlSentinel); idx >= 0 {
			body = output[:idx]
			meta := strings.TrimSpace(output[idx+len(curlSentinel):])
			lines := strings.SplitN(meta, "\n", 2)
			statusCode = strings.TrimSpace(lines[0])
			if len(lines) > 1 {
				contentType = strings.TrimSpace(lines[1])
			}
		} else {
			body = output
		}

		if resp.ExitCode != 0 {
			errMsg := strings.TrimSpace(resp.Stderr)
			if errMsg == "" {
				errMsg = fmt.Sprintf("curl exited with code %d", resp.ExitCode)
			}
			return agentic.ToolResult{CallID: call.ID, Error: fmt.Sprintf("request failed: %s", errMsg)}
		}

		// Render via the same pipeline as the local handler so HTML responses
		// fetched through the sandbox come back as markdown rather than raw
		// HTML. Non-HTML responses pass through with a length cap only.
		rendered := httpformat.Render([]byte(body), contentType, urlStr, format, httpTextMaxSize)

		return agentic.ToolResult{
			CallID:  call.ID,
			Content: fmt.Sprintf("HTTP %s\n\n%s", statusCode, rendered),
		}
	}
}
