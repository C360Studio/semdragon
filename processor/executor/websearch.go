package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// =============================================================================
// SEARCH PROVIDER INTERFACE
// =============================================================================

// SearchProvider performs web searches via an external API.
// Implementations must be safe for concurrent use.
type SearchProvider interface {
	// Search executes a web search and returns formatted results.
	// maxResults is capped by the implementation (typically 1-10).
	Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error)
	// Name returns the provider identifier (e.g. "brave", "serper").
	Name() string
}

// SearchResult represents a single web search result.
type SearchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

// SearchConfig holds the configuration for web search providers.
type SearchConfig struct {
	// Provider is the search provider type (e.g. "brave").
	Provider string `json:"provider"`
	// APIKey is the authentication key for the provider.
	APIKey string `json:"api_key"`
	// BaseURL overrides the default API endpoint (optional).
	BaseURL string `json:"base_url,omitempty"`
}

// NewSearchProvider creates a SearchProvider from configuration.
// Returns an error if the provider type is unknown or config is invalid.
func NewSearchProvider(cfg SearchConfig) (SearchProvider, error) {
	if cfg.Provider == "" {
		return nil, fmt.Errorf("search provider type is required")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("search API key is required")
	}

	switch cfg.Provider {
	case "brave":
		baseURL := cfg.BaseURL
		if baseURL == "" {
			baseURL = braveDefaultBaseURL
		}
		return &braveSearchProvider{
			apiKey:  cfg.APIKey,
			baseURL: baseURL,
		}, nil
	default:
		return nil, fmt.Errorf("unknown search provider: %q (supported: brave)", cfg.Provider)
	}
}

// =============================================================================
// BRAVE SEARCH PROVIDER
// =============================================================================

const braveDefaultBaseURL = "https://api.search.brave.com/res/v1/web/search"

// braveSearchProvider implements SearchProvider using the Brave Search API.
type braveSearchProvider struct {
	apiKey  string
	baseURL string
}

func (b *braveSearchProvider) Name() string { return "brave" }

func (b *braveSearchProvider) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	if maxResults <= 0 {
		maxResults = 5
	}
	if maxResults > 10 {
		maxResults = 10
	}

	params := url.Values{}
	params.Set("q", query)
	params.Set("count", strconv.Itoa(maxResults))

	reqURL := b.baseURL + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("X-Subscription-Token", b.apiKey)

	resp, err := httpToolClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxHTTPResponseSize))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search API returned %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var braveResp braveSearchResponse
	if err := json.Unmarshal(body, &braveResp); err != nil {
		return nil, fmt.Errorf("failed to parse search response: %w", err)
	}

	results := make([]SearchResult, 0, len(braveResp.Web.Results))
	for _, r := range braveResp.Web.Results {
		results = append(results, SearchResult{
			Title:       r.Title,
			URL:         r.URL,
			Description: r.Description,
		})
	}

	return results, nil
}

// braveSearchResponse is the relevant subset of Brave Search API's JSON response.
type braveSearchResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}

// =============================================================================
// RESULT FORMATTING
// =============================================================================

// formatSearchResults formats search results for agent consumption.
func formatSearchResults(results []SearchResult, query string) string {
	if len(results) == 0 {
		return fmt.Sprintf("No results found for: %s", query)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search results for: %s\n\n", query))
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("[%d] %s\n", i+1, r.Title))
		sb.WriteString(fmt.Sprintf("    %s\n", r.URL))
		if r.Description != "" {
			sb.WriteString(fmt.Sprintf("    %s\n", r.Description))
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

// truncate returns s truncated to maxLen characters with an ellipsis if needed.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
