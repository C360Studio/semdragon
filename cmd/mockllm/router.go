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
	reChain     = regexp.MustCompile(`(?i)chain|multiple.*quest`)
	reQuestBrief = regexp.MustCompile(`(?i)create.*quest|quest.*brief|build|analyze`)
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
			return completionResponse(completionContent)
		}
		return toolCallResponse(req.Tools)
	}

	// DM chat path: pattern-match on the last user message.
	lastUser := lastUserMessage(req.Messages)
	if reChain.MatchString(lastUser) {
		return completionResponse(questChainResponse)
	}
	if reQuestBrief.MatchString(lastUser) {
		return completionResponse(questBriefResponse)
	}
	return completionResponse(conversationalResponse)
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

// toolCallResponse picks the first tool from the request's tools list,
// preferring list_directory or read_file as representative filesystem tools.
func toolCallResponse(tools []toolDef) chatResponse {
	name := tools[0].Function.Name
	arguments := `{"path": "."}`
	for _, t := range tools {
		switch t.Function.Name {
		case "list_directory":
			name = "list_directory"
			arguments = `{"path": "."}`
		case "read_file":
			name = "read_file"
			arguments = `{"path": "README.md"}`
		}
	}
	if name == "read_file" {
		arguments = `{"path": "README.md"}`
	}

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
