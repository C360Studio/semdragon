package api

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

const llmHTTPTimeout = 60 * time.Second

// ChatMessage represents a single turn in the DM conversation.
type ChatMessage struct {
	Role    string `json:"role"`    // "user" or "dm" (mapped to "assistant" for LLM calls)
	Content string `json:"content"`
}

// callLLM sends a chat completion request to the resolved endpoint.
// It handles provider routing: Anthropic Messages API vs OpenAI-compatible chat completions.
func callLLM(ctx context.Context, endpoint *model.EndpointConfig, systemPrompt string, messages []ChatMessage) (string, error) {
	if endpoint == nil {
		return "", fmt.Errorf("no endpoint configured")
	}

	apiKey := ""
	if endpoint.APIKeyEnv != "" {
		apiKey = os.Getenv(endpoint.APIKeyEnv)
		if apiKey == "" {
			return "", fmt.Errorf("API key env %q is not set", endpoint.APIKeyEnv)
		}
	}

	switch endpoint.Provider {
	case "anthropic":
		return callAnthropic(ctx, endpoint, apiKey, systemPrompt, messages)
	default:
		// OpenAI-compatible: works for "openai", "ollama", "openrouter"
		return callOpenAICompat(ctx, endpoint, apiKey, systemPrompt, messages)
	}
}

// callAnthropic sends a request to the Anthropic Messages API.
func callAnthropic(ctx context.Context, endpoint *model.EndpointConfig, apiKey, systemPrompt string, messages []ChatMessage) (string, error) {
	url := "https://api.anthropic.com/v1/messages"
	if endpoint.URL != "" {
		url = endpoint.URL
	}

	type anthropicMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	msgs := make([]anthropicMessage, 0, len(messages))
	for _, m := range messages {
		role := m.Role
		if role == "dm" {
			role = "assistant"
		}
		msgs = append(msgs, anthropicMessage{Role: role, Content: m.Content})
	}

	body := map[string]any{
		"model":      endpoint.Model,
		"max_tokens": 4096,
		"system":     systemPrompt,
		"messages":   msgs,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal anthropic request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create anthropic request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: llmHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("anthropic request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read anthropic response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("anthropic API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse anthropic response: %w", err)
	}

	if len(result.Content) == 0 {
		return "", fmt.Errorf("empty anthropic response")
	}

	return result.Content[0].Text, nil
}

// callOpenAICompat sends a request to an OpenAI-compatible chat completions endpoint.
// Works with OpenAI, Ollama, OpenRouter, and any compatible provider.
func callOpenAICompat(ctx context.Context, endpoint *model.EndpointConfig, apiKey, systemPrompt string, messages []ChatMessage) (string, error) {
	url := endpoint.URL
	if url == "" {
		return "", fmt.Errorf("URL required for provider %q", endpoint.Provider)
	}
	// Ensure /chat/completions path
	if url[len(url)-1] == '/' {
		url = url[:len(url)-1]
	}
	url += "/chat/completions"

	type openAIMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	msgs := make([]openAIMessage, 0, len(messages)+1)
	msgs = append(msgs, openAIMessage{Role: "system", Content: systemPrompt})
	for _, m := range messages {
		role := m.Role
		if role == "dm" {
			role = "assistant"
		}
		msgs = append(msgs, openAIMessage{Role: role, Content: m.Content})
	}

	body := map[string]any{
		"model":    endpoint.Model,
		"messages": msgs,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal openai request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create openai request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: llmHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read openai response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openai API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse openai response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("empty openai response")
	}

	return result.Choices[0].Message.Content, nil
}
