package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"time"
)

// ---- OpenAI-compatible request types ----------------------------------------

type chatRequest struct {
	Model    string          `json:"model"`
	Messages []requestMsg    `json:"messages"`
	Tools    []toolDef       `json:"tools,omitempty"`
	Stream   bool            `json:"stream"`
}

type requestMsg struct {
	Role       string     `json:"role"`
	Content    any        `json:"content"` // string or nil for tool_calls turns
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type toolDef struct {
	Type     string      `json:"type"`
	Function toolFuncDef `json:"function"`
}

type toolFuncDef struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// ---- OpenAI-compatible response types ---------------------------------------

type chatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []choice `json:"choices"`
	Usage   usage    `json:"usage"`
}

type choice struct {
	Index        int         `json:"index"`
	Message      responseMsg `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type responseMsg struct {
	Role      string     `json:"role"`
	Content   *string    `json:"content"` // nil for tool_calls turns
	ToolCalls []toolCall `json:"tool_calls,omitempty"`
}

type toolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function toolFuncCall `json:"function"`
}

type toolFuncCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ---- Pattern matching -------------------------------------------------------

var (
	reChain      = regexp.MustCompile(`(?i)chain|multiple.*quest`)
	reQuestBrief = regexp.MustCompile(`(?i)create.*quest|quest.*brief|build|analyze`)
	// Matches the DM triage system prompt which contains "recovery path" or "triage".
	reTriage = regexp.MustCompile(`(?i)recovery path.*salvage|triage.*retry`)
	// Matches sub-quest entity IDs in prompts like: sub-quest "org.plat.game.board.quest.abc"
	reSubQuestID = regexp.MustCompile(`sub-quest\s+"([^"]+)"`)
	// Matches research-oriented quest prompts that should trigger web_search.
	reResearch = regexp.MustCompile(`(?i)research|search the web|find information|look up|investigate`)
)

// handleChatCompletions returns an http.HandlerFunc that logs and routes
// incoming chat completion requests to the appropriate canned response.
func handleChatCompletions(logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "cannot read body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		var req chatRequest
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		logger.Info("chat completion request",
			"method", r.Method,
			"path", r.URL.Path,
			"model", req.Model,
			"messages", len(req.Messages),
			"tools", len(req.Tools),
		)

		resp := route(req)
		resp.ID = fmt.Sprintf("mock-%d", time.Now().UnixNano())
		resp.Object = "chat.completion"
		resp.Created = time.Now().Unix()
		resp.Model = req.Model

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			logger.Error("failed to encode response", "error", err)
		}
	}
}

// route decides which canned response to send based on the request shape.
func route(req chatRequest) chatResponse {
	// Agentic loop path: tools are present.
	// First turn — no tool results yet — return a tool call so the loop
	// exercises at least one round-trip before completing.
	// Second turn — tool results already in messages — return a completion
	// so the loop finishes cleanly.
	if len(req.Tools) > 0 {
		if hasToolResults(req.Messages) {
			return routeWithToolResults(req)
		}
		return routeToolCall(req.Tools, req.Messages)
	}

	// DM chat path: check system prompt first for triage requests,
	// then pattern-match on the last user message.
	sysPrompt := systemMessage(req.Messages)
	if reTriage.MatchString(sysPrompt) {
		return completionResponse(triageResponse)
	}

	lastUser := lastUserMessage(req.Messages)
	if reChain.MatchString(lastUser) {
		return completionResponse(questChainResponse)
	}
	if reQuestBrief.MatchString(lastUser) {
		return completionResponse(questBriefResponse)
	}
	return completionResponse(conversationalResponse)
}

// routeToolCall picks the right tool call based on which tools are available.
// For DAG decomposition and review flows, it calls the domain-specific tool.
// For generic agentic loops, it falls back to filesystem tools.
func routeToolCall(tools []toolDef, msgs []requestMsg) chatResponse {
	toolNames := make(map[string]bool, len(tools))
	for _, t := range tools {
		toolNames[t.Function.Name] = true
	}

	// Party quest decomposition: call decompose_quest if available.
	if toolNames["decompose_quest"] {
		return namedToolCallResponse("decompose_quest", dagDecompositionArgs)
	}

	// Lead review: call review_sub_quest if available.
	// Extract the real sub-quest ID from the prompt so the review tool
	// receives a valid entity ID rather than a placeholder.
	if toolNames["review_sub_quest"] {
		args := buildReviewAcceptArgs(msgs)
		return namedToolCallResponse("review_sub_quest", args)
	}

	// Lead clarification: call answer_clarification if available.
	if toolNames["answer_clarification"] {
		args := buildClarificationArgs(msgs)
		return namedToolCallResponse("answer_clarification", args)
	}

	// Research quest: call web_search if available and prompt matches research pattern.
	if toolNames["web_search"] && isResearchPrompt(msgs) {
		return namedToolCallResponse("web_search", webSearchArgs)
	}

	// Default: pick a filesystem tool for generic agentic loops.
	return toolCallResponse(tools)
}

// routeWithToolResults handles the second+ turn of an agentic loop (tool
// results are present). For DAG decomposition, returns the DAG JSON content
// so questbridge can extract and process it. For generic loops, calls
// submit_work_product to cleanly terminate the loop.
func routeWithToolResults(req chatRequest) chatResponse {
	// Check what tool was last called by looking at tool_calls in messages.
	for i := len(req.Messages) - 1; i >= 0; i-- {
		msg := req.Messages[i]
		for _, tc := range msg.ToolCalls {
			switch tc.Function.Name {
			case "decompose_quest":
				return completionResponse(dagDecompositionContent)
			case "review_sub_quest":
				return completionResponse(reviewAcceptCompletion)
			case "web_search":
				// web_search result received — submit a research summary via submit_work_product.
				return namedToolCallResponse("submit_work_product", webSearchSubmitArgs)
			case "submit_work_product":
				// submit_work_product sets StopLoop=true, so the loop should
				// not reach here. If it does, just complete cleanly.
				return completionResponse(completionContent)
			}
		}
	}
	// Generic loop (filesystem tool result) → call submit_work_product to terminate.
	return namedToolCallResponse("submit_work_product",
		`{"deliverable":"The requested operation finished successfully. All output has been validated and is ready for review.","summary":"Task complete"}`)
}

// namedToolCallResponse returns a tool_calls response for a specific tool.
func namedToolCallResponse(name, arguments string) chatResponse {
	nilContent := (*string)(nil)
	return chatResponse{
		Choices: []choice{
			{
				Index: 0,
				Message: responseMsg{
					Role:    "assistant",
					Content: nilContent,
					ToolCalls: []toolCall{
						{
							ID:   "call_mock_1",
							Type: "function",
							Function: toolFuncCall{
								Name:      name,
								Arguments: arguments,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
		Usage: usage{PromptTokens: 100, CompletionTokens: 25, TotalTokens: 125},
	}
}

// hasToolResults returns true if any message in the conversation has role
// "tool", which means the loop has already processed a tool call round-trip.
func hasToolResults(msgs []requestMsg) bool {
	for _, m := range msgs {
		if m.Role == "tool" {
			return true
		}
	}
	return false
}

// systemMessage returns the content of the first system-role message,
// or an empty string if none is present.
func systemMessage(msgs []requestMsg) string {
	for _, m := range msgs {
		if m.Role == "system" {
			if s, ok := m.Content.(string); ok {
				return s
			}
		}
	}
	return ""
}

// lastUserMessage returns the content string of the last user-role message,
// or an empty string if no user message is present.
func lastUserMessage(msgs []requestMsg) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			switch v := msgs[i].Content.(type) {
			case string:
				return v
			}
		}
	}
	return ""
}

// toolCallResponse picks a tool from the request's tools list.
// Priority: write_file (expert agents) > list_directory > read_file > first tool.
// This ensures expert agents exercise the workspace artifact path while
// apprentice agents stick to read-only tools.
func toolCallResponse(tools []toolDef) chatResponse {
	name := tools[0].Function.Name
	arguments := `{"path": "."}`

	// Build a name→bool set for O(1) lookups.
	toolNames := make(map[string]bool, len(tools))
	for _, t := range tools {
		toolNames[t.Function.Name] = true
	}

	switch {
	case toolNames["write_file"]:
		name = "write_file"
		arguments = `{"path":"solution.py","content":"# Mock solution\nimport json\n\ndef analyze(data):\n    return {\"summary\": \"processed\", \"count\": len(data)}\n\nif __name__ == \"__main__\":\n    print(analyze([1,2,3]))\n"}`
	case toolNames["list_directory"]:
		name = "list_directory"
		arguments = `{"path": "."}`
	case toolNames["read_file"]:
		name = "read_file"
		arguments = `{"path": "README.md"}`
	}

	return namedToolCallResponse(name, arguments)
}

// buildReviewAcceptArgs constructs review_sub_quest tool call arguments with
// the real sub-quest ID extracted from the prompt messages. Falls back to a
// placeholder if no ID can be found (shouldn't happen in normal flow).
func buildReviewAcceptArgs(msgs []requestMsg) string {
	subQuestID := extractSubQuestID(msgs)
	return fmt.Sprintf(
		`{"sub_quest_id":%q,"verdict":"accept","ratings":{"q1":5,"q2":5,"q3":5},"explanation":"Work meets all acceptance criteria."}`,
		subQuestID,
	)
}

// buildClarificationArgs constructs answer_clarification tool call arguments
// with the real sub-quest ID extracted from the prompt messages.
func buildClarificationArgs(msgs []requestMsg) string {
	subQuestID := extractSubQuestID(msgs)
	return fmt.Sprintf(
		`{"sub_quest_id":%q,"answer":"The approach looks correct. Proceed with the implementation as described."}`,
		subQuestID,
	)
}

// extractSubQuestID scans all messages for a sub-quest entity ID pattern.
// The dispatch prompts include: sub-quest "org.plat.game.board.quest.abc"
func extractSubQuestID(msgs []requestMsg) string {
	for _, msg := range msgs {
		content, ok := msg.Content.(string)
		if !ok {
			continue
		}
		matches := reSubQuestID.FindStringSubmatch(content)
		if len(matches) > 1 {
			return matches[1]
		}
	}
	return "__UNKNOWN_SUB_QUEST_ID__"
}

// isResearchPrompt checks whether the last user message or system prompt
// contains research-related keywords that should trigger a web_search tool call.
func isResearchPrompt(msgs []requestMsg) bool {
	last := lastUserMessage(msgs)
	if reResearch.MatchString(last) {
		return true
	}
	sys := systemMessage(msgs)
	return reResearch.MatchString(sys)
}

// completionResponse wraps a text string in a standard stop-finish choice.
func completionResponse(content string) chatResponse {
	return chatResponse{
		Choices: []choice{
			{
				Index: 0,
				Message: responseMsg{
					Role:    "assistant",
					Content: &content,
				},
				FinishReason: "stop",
			},
		},
		Usage: usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
	}
}
