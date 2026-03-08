package questbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/c360studio/semstreams/model"
)

const dmLLMTimeout = 120 * time.Second

// registryAnswerer implements ClarificationAnswerer using the model registry.
// It resolves the "dm-chat" capability and makes an OpenAI-compatible HTTP call.
type registryAnswerer struct {
	registry model.RegistryReader
}

// newRegistryAnswerer creates a ClarificationAnswerer backed by the model registry.
func newRegistryAnswerer(registry model.RegistryReader) *registryAnswerer {
	return &registryAnswerer{registry: registry}
}

// AnswerClarification sends the agent's question to the dm-chat LLM with
// quest context and returns the answer.
func (r *registryAnswerer) AnswerClarification(ctx context.Context, questTitle, questDescription, question string) (string, error) {
	endpointName := r.registry.Resolve("dm-chat")
	if endpointName == "" {
		return "", fmt.Errorf("no endpoint configured for dm-chat capability")
	}
	endpoint := r.registry.GetEndpoint(endpointName)
	if endpoint == nil {
		return "", fmt.Errorf("endpoint %q not found in registry", endpointName)
	}

	systemPrompt := `You are the Dungeon Master (DM) for an agentic workflow system. An agent working on a quest has asked for clarification before proceeding.

Your role is to answer their questions concisely and helpfully based on the quest context. Provide clear, actionable guidance so the agent can proceed with the task.

Rules:
1. Answer based on the quest title, description, and the agent's specific questions.
2. Be concise — agents work best with clear, direct answers.
3. If a question is about implementation approach, suggest a reasonable default.
4. If a question cannot be answered from context alone, provide your best judgment and note any assumptions.`

	userMessage := fmt.Sprintf("Quest: %s\n\nDescription: %s\n\nThe agent is asking:\n%s",
		questTitle, questDescription, question)

	return r.callDMChat(ctx, endpoint, systemPrompt, userMessage)
}

// callDMChat makes an OpenAI-compatible chat completion request.
// Supports OpenAI, Gemini (via OpenAI compat), Ollama, and Anthropic.
func (r *registryAnswerer) callDMChat(ctx context.Context, endpoint *model.EndpointConfig, systemPrompt, userMessage string) (string, error) {
	apiKey := ""
	if endpoint.APIKeyEnv != "" {
		apiKey = os.Getenv(endpoint.APIKeyEnv)
		if apiKey == "" {
			return "", fmt.Errorf("API key env %q is not set", endpoint.APIKeyEnv)
		}
	}

	if endpoint.Provider == "anthropic" {
		return r.callAnthropic(ctx, endpoint, apiKey, systemPrompt, userMessage)
	}
	return r.callOpenAICompat(ctx, endpoint, apiKey, systemPrompt, userMessage)
}

func (r *registryAnswerer) callOpenAICompat(ctx context.Context, endpoint *model.EndpointConfig, apiKey, systemPrompt, userMessage string) (string, error) {
	url := endpoint.URL
	if url == "" {
		return "", fmt.Errorf("URL required for provider %q", endpoint.Provider)
	}
	if url[len(url)-1] == '/' {
		url = url[:len(url)-1]
	}
	url += "/chat/completions"

	body := map[string]any{
		"model": endpoint.Model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userMessage},
		},
	}
	if endpoint.ReasoningEffort != "" {
		body["reasoning_effort"] = endpoint.ReasoningEffort
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: dmLLMTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("empty response")
	}
	return result.Choices[0].Message.Content, nil
}

func (r *registryAnswerer) callAnthropic(ctx context.Context, endpoint *model.EndpointConfig, apiKey, systemPrompt, userMessage string) (string, error) {
	url := "https://api.anthropic.com/v1/messages"
	if endpoint.URL != "" {
		url = endpoint.URL
	}

	body := map[string]any{
		"model":      endpoint.Model,
		"max_tokens": 4096,
		"system":     systemPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": userMessage},
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: dmLLMTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("empty response")
	}
	return result.Content[0].Text, nil
}
