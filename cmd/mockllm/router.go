package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"sync"
	"sync/atomic"
	"time"
)

// callCounter generates unique tool call IDs across responses.
var callCounter atomic.Int64

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
	// Boss battle judge detection: system prompt from AssembleJudgePromptWithAcceptance
	// contains "evaluating" + structural checklist items.
	reBossBattleJudge = regexp.MustCompile(`(?i)evaluating.*work output|evaluating.*quest performance`)
	reChain           = regexp.MustCompile(`(?i)chain|multiple.*quest`)
	reQuestBrief = regexp.MustCompile(`(?i)create.*quest|quest.*brief|build|analyze`)
	// Matches the DM triage system prompt which contains "recovery path" or "triage".
	reTriage = regexp.MustCompile(`(?i)recovery path.*salvage|triage.*retry`)
	// Matches sub-quest entity IDs in prompts like: sub-quest "org.plat.game.board.quest.abc"
	reSubQuestID = regexp.MustCompile(`sub-quest\s+"([^"]+)"`)
	// Matches research-oriented quest prompts that should trigger web_search.
	reResearch = regexp.MustCompile(`(?i)research|search the web|find information|look up|investigate`)
	// Matches DM queries about game state that should trigger graph_query.
	// "look up" is intentionally in reResearch only to avoid overlap.
	reQuery = regexp.MustCompile(`(?i)board|status|what.*quest|how many|current.*state|query|tell me about`)
	// Matches party-style quest prompts with parallel/sub-task indicators.
	rePartyQuest = regexp.MustCompile(`(?i)parallel|sub-task|independent.*function|independent.*task|party`)
	// Matches party lead system prompts (agent acting as party lead).
	rePartyLead = regexp.MustCompile(`(?i)PARTY LEAD`)
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

		// Log message summary for debugging agentic loop interactions.
		msgSummary := make([]string, len(req.Messages))
		for i, m := range req.Messages {
			tc := ""
			if len(m.ToolCalls) > 0 {
				names := make([]string, len(m.ToolCalls))
				for j, c := range m.ToolCalls {
					names[j] = c.Function.Name
				}
				tc = fmt.Sprintf(" tool_calls=%v", names)
			}
			tcID := ""
			if m.ToolCallID != "" {
				tcID = fmt.Sprintf(" tool_call_id=%s", m.ToolCallID)
			}
			content := ""
			if s, ok := m.Content.(string); ok && len(s) > 80 {
				content = fmt.Sprintf(" content=%.80s...", s)
			} else if s, ok := m.Content.(string); ok && s != "" {
				content = fmt.Sprintf(" content=%s", s)
			}
			msgSummary[i] = fmt.Sprintf("[%d]%s%s%s%s", i, m.Role, tc, tcID, content)
		}

		toolNames := make([]string, len(req.Tools))
		for i, t := range req.Tools {
			toolNames[i] = t.Function.Name
		}

		logger.Info("chat completion request",
			"model", req.Model,
			"messages", len(req.Messages),
			"tools", toolNames,
			"msg_detail", msgSummary,
		)

		resp := route(req, logger)
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

// isDMToolRequest returns true when the request carries tools but none of them
// is agent-specific. Regular agents include submit_work; party leads
// get decompose_quest/review_sub_quest instead (submit_work is withheld
// during decomposition to force delegation). The DM allowlist
// (service/api/dm_tools.go) never includes any of these.
func isDMToolRequest(tools []toolDef) bool {
	for _, t := range tools {
		switch t.Function.Name {
		case "submit_work", "decompose_quest", "review_sub_quest", "answer_clarification":
			return false // agent request
		}
	}
	// Tools are present but no agent-specific tool — it's a DM request.
	return len(tools) > 0
}

// routeDMToolCall handles the first turn of a DM tool-calling request.
// Quest design messages bypass tools and return a text completion directly.
// Game-state queries use graph_query; research queries prefer graph_search then
// web_search. Everything else falls back to a conversational text completion.
func routeDMToolCall(tools []toolDef, msgs []requestMsg) chatResponse {
	toolNames := make(map[string]bool, len(tools))
	for _, t := range tools {
		toolNames[t.Function.Name] = true
	}

	lastUser := lastUserMessage(msgs)

	// Quest design: no tools needed, respond with quest brief/chain directly.
	// Check combined patterns first: research+quest and party+quest.
	if reQuestBrief.MatchString(lastUser) && reResearch.MatchString(lastUser) {
		return completionResponse(researchQuestBriefResponse)
	}
	if reQuestBrief.MatchString(lastUser) && rePartyQuest.MatchString(lastUser) {
		return completionResponse(partyQuestBriefResponse)
	}
	if reChain.MatchString(lastUser) {
		return completionResponse(questChainResponse)
	}
	if reQuestBrief.MatchString(lastUser) {
		return completionResponse(questBriefResponse)
	}

	// Query about game state: use graph_query.
	if toolNames["graph_query"] && reQuery.MatchString(lastUser) {
		return namedToolCallResponse("graph_query", dmGraphQueryArgs)
	}

	// Research: prefer graph_search then fall back to web_search.
	if toolNames["graph_search"] && reResearch.MatchString(lastUser) {
		return namedToolCallResponse("graph_search", graphSearchArgs)
	}
	if toolNames["web_search"] && reResearch.MatchString(lastUser) {
		return namedToolCallResponse("web_search", webSearchArgs)
	}

	// Default: conversational response without tools.
	return completionResponse(conversationalResponse)
}

// routeDMWithToolResults handles the second turn of a DM tool-calling request,
// after tool results have been returned. The DM has no submit_work tool,
// so this MUST return a text completion (finish_reason: "stop"), not a tool call.
func routeDMWithToolResults(req chatRequest) chatResponse {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		msg := req.Messages[i]
		for _, tc := range msg.ToolCalls {
			switch tc.Function.Name {
			case "graph_query":
				return completionResponse(dmGraphQueryCompletion)
			case "web_search":
				return completionResponse(dmWebSearchCompletion)
			case "graph_search":
				return completionResponse(dmGraphSearchCompletion)
			default:
				return completionResponse(dmGenericToolCompletion)
			}
		}
	}
	return completionResponse(dmGenericToolCompletion)
}

// route decides which canned response to send based on the request shape.
func route(req chatRequest, logger *slog.Logger) chatResponse {
	// Agentic loop path: tools are present.
	// First turn — no tool results yet — return a tool call so the loop
	// exercises at least one round-trip before completing.
	// Second turn — tool results already in messages — return a completion
	// so the loop finishes cleanly.
	if len(req.Tools) > 0 {
		// DM tool loop: tools present but no agent-specific tools (submit_work).
		// Must be checked BEFORE the agent path since both have tools.
		if isDMToolRequest(req.Tools) {
			if hasToolResults(req.Messages) {
				resp := routeDMWithToolResults(req)
				logResponse(logger, "dm-routeWithToolResults", resp)
				return resp
			}
			resp := routeDMToolCall(req.Tools, req.Messages)
			logResponse(logger, "dm-routeToolCall", resp)
			return resp
		}

		if hasToolResults(req.Messages) {
			resp := routeWithToolResults(req)
			logResponse(logger, "routeWithToolResults", resp)
			return resp
		}
		resp := routeToolCall(req.Tools, req.Messages)
		logResponse(logger, "routeToolCall", resp)
		return resp
	}

	// No-tools path: boss battle judge, DM triage, or DM chat.
	sysPrompt := systemMessage(req.Messages)

	// Boss battle evaluation: the evaluator calls with no tools, system prompt
	// from the domain catalog's JudgeSystemBase.
	if reBossBattleJudge.MatchString(sysPrompt) {
		resp := routeBossBattleJudge(req.Messages, logger)
		logResponse(logger, "boss-battle-judge", resp)
		return resp
	}

	if reTriage.MatchString(sysPrompt) {
		logger.Info("route", "path", "dm-triage")
		return completionResponse(triageResponse)
	}

	lastUser := lastUserMessage(req.Messages)
	if reQuestBrief.MatchString(lastUser) && reResearch.MatchString(lastUser) {
		logger.Info("route", "path", "dm-research-quest-brief")
		return completionResponse(researchQuestBriefResponse)
	}
	if reQuestBrief.MatchString(lastUser) && rePartyQuest.MatchString(lastUser) {
		logger.Info("route", "path", "dm-party-quest-brief")
		return completionResponse(partyQuestBriefResponse)
	}
	if reChain.MatchString(lastUser) {
		logger.Info("route", "path", "dm-chain")
		return completionResponse(questChainResponse)
	}
	if reQuestBrief.MatchString(lastUser) {
		logger.Info("route", "path", "dm-quest-brief")
		return completionResponse(questBriefResponse)
	}
	logger.Info("route", "path", "dm-conversational")
	return completionResponse(conversationalResponse)
}

// logResponse logs the response path and key details (tool call name or completion).
func logResponse(logger *slog.Logger, path string, resp chatResponse) {
	if len(resp.Choices) == 0 {
		logger.Info("route", "path", path, "choices", 0)
		return
	}
	c := resp.Choices[0]
	if len(c.Message.ToolCalls) > 0 {
		names := make([]string, len(c.Message.ToolCalls))
		for i, tc := range c.Message.ToolCalls {
			names[i] = fmt.Sprintf("%s(id=%s)", tc.Function.Name, tc.ID)
		}
		logger.Info("route", "path", path, "finish", c.FinishReason, "tool_calls", names)
	} else {
		preview := ""
		if c.Message.Content != nil && len(*c.Message.Content) > 80 {
			preview = (*c.Message.Content)[:80] + "..."
		} else if c.Message.Content != nil {
			preview = *c.Message.Content
		}
		logger.Info("route", "path", path, "finish", c.FinishReason, "content_preview", preview)
	}
}

// bossBattleAttempts tracks how many times each quest has been evaluated.
// First evaluation returns defeat; subsequent evaluations return victory.
// This exercises the retry path in E2E tests.
var bossBattleAttempts sync.Map // map[questTitle]int

// routeBossBattleJudge returns a judge verdict for boss battle evaluations.
// First attempt for a given quest returns DEFEAT (checklist failure) to exercise
// the retry/repost path. Second attempt returns VICTORY.
func routeBossBattleJudge(msgs []requestMsg, logger *slog.Logger) chatResponse {
	// Extract quest context from user message to track per-quest attempts.
	userMsg := lastUserMessage(msgs)
	questKey := "default"
	if len(userMsg) > 50 {
		questKey = userMsg[:50]
	}

	attempts := 1
	if v, loaded := bossBattleAttempts.LoadOrStore(questKey, 1); loaded {
		attempts = v.(int) + 1
		bossBattleAttempts.Store(questKey, attempts)
	}

	if attempts == 1 {
		logger.Info("boss-battle-judge", "verdict", "defeat", "attempt", attempts)
		return completionResponse(bossBattleDefeatResponse)
	}

	logger.Info("boss-battle-judge", "verdict", "victory", "attempt", attempts)
	return completionResponse(bossBattleVictoryResponse)
}

// routeToolCall picks the right tool call based on which tools are available.
// For DAG decomposition and review flows, it calls the domain-specific tool.
// For generic agentic loops, it falls back to filesystem tools.
func routeToolCall(tools []toolDef, msgs []requestMsg) chatResponse {
	toolNames := make(map[string]bool, len(tools))
	for _, t := range tools {
		toolNames[t.Function.Name] = true
	}

	// Party quest decomposition: call decompose_quest only when the prompt
	// indicates a party lead context. High-tier agents always have decompose_quest
	// in their tool list, but should only call it when acting as party lead.
	if toolNames["decompose_quest"] && isPartyLeadPrompt(msgs) {
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

	// Graph search: call graph_search if available and prompt matches research pattern.
	// Takes priority over web_search because graph data is more relevant for project-specific queries.
	if toolNames["graph_search"] && isResearchPrompt(msgs) {
		return namedToolCallResponse("graph_search", graphSearchArgs)
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
// submit_work to cleanly terminate the loop.
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
			case "graph_search":
				// graph_search result received — submit a research summary via submit_work.
				return namedToolCallResponse("submit_work", graphSearchSubmitArgs)
			case "web_search":
				// web_search result received — submit a research summary via submit_work.
				return namedToolCallResponse("submit_work", webSearchSubmitArgs)
			case "submit_work":
				// submit_work sets StopLoop=true, so the loop should
				// not reach here. If it does, just complete cleanly.
				return completionResponse(completionContent)
			}
		}
	}
	// Generic loop (bash tool result) → call submit_work to terminate.
	return namedToolCallResponse("submit_work",
		`{"deliverable":"The requested operation finished successfully. All output has been validated and is ready for review.","summary":"Task complete"}`)
}

// namedToolCallResponse returns a tool_calls response for a specific tool.
func namedToolCallResponse(name, arguments string) chatResponse {
	nilContent := (*string)(nil)
	callID := fmt.Sprintf("call_mock_%d", callCounter.Add(1))
	return chatResponse{
		Choices: []choice{
			{
				Index: 0,
				Message: responseMsg{
					Role:    "assistant",
					Content: nilContent,
					ToolCalls: []toolCall{
						{
							ID:   callID,
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
// Priority: bash (for file writes and reads) > first tool.
// All file operations are expressed as bash commands since write_file,
// read_file, and list_directory have been removed from the tool registry.
func toolCallResponse(tools []toolDef) chatResponse {
	name := tools[0].Function.Name
	arguments := `{"command":"ls -la"}`

	// Build a name→bool set for O(1) lookups.
	toolNames := make(map[string]bool, len(tools))
	for _, t := range tools {
		toolNames[t.Function.Name] = true
	}

	if toolNames["bash"] {
		name = "bash"
		arguments = `{"command":"cat <<'MOCKEOF' > solution.py\n# Mock solution\nimport json\n\ndef analyze(data):\n    return {\"summary\": \"processed\", \"count\": len(data)}\n\nif __name__ == \"__main__\":\n    print(analyze([1,2,3]))\nMOCKEOF"}`
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

// isPartyLeadPrompt checks whether the system prompt contains the party lead
// directive, indicating this agent is acting as a party lead and should call
// decompose_quest rather than working the quest directly.
func isPartyLeadPrompt(msgs []requestMsg) bool {
	sys := systemMessage(msgs)
	return rePartyLead.MatchString(sys)
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
