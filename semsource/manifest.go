package semsource

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// ManifestPayload is the response from semsource's GET /source-manifest/sources endpoint.
type ManifestPayload struct {
	Sources []SourceManifest `json:"sources"`
}

// SourceManifest describes a single source being ingested by semsource.
type SourceManifest struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`         // "git_repo", "document", etc.
	Description string   `json:"description"`
	EntityTypes []string `json:"entity_types"` // entity types this source produces
	Status      string   `json:"status"`       // "active", "indexing", "error"
}

const (
	manifestPath     = "/source-manifest/sources"
	defaultCacheTTL  = 5 * time.Minute
	manifestTimeout  = 3 * time.Second
	maxManifestBytes = 1 << 20 // 1 MiB
)

// manifestHTTPClient is a dedicated HTTP client for manifest fetches,
// isolated from http.DefaultClient mutations by other code.
var manifestHTTPClient = &http.Client{
	Timeout: manifestTimeout,
}

// ManifestClient fetches and caches the source manifest from a semsource instance.
// All methods are safe for concurrent use and never return errors — failures are
// logged at debug level and result in nil/empty returns.
type ManifestClient struct {
	baseURL  string
	cacheTTL time.Duration
	logger   *slog.Logger

	mu       sync.RWMutex
	cached   *ManifestPayload
	cachedAt time.Time
	sfGroup  singleflight.Group
}

// NewManifestClient creates a ManifestClient that fetches from the semsource
// instance at the given websocket URL. The websocket URL is converted to an
// HTTP base URL (ws://host:port/ws → http://host:port).
// Returns nil if the URL is empty or cannot be parsed.
func NewManifestClient(wsURL string, logger *slog.Logger) *ManifestClient {
	base := wsURLToHTTP(wsURL)
	if base == "" {
		return nil
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ManifestClient{
		baseURL:  base,
		cacheTTL: defaultCacheTTL,
		logger:   logger,
	}
}

// Fetch returns the cached manifest if fresh, otherwise attempts an HTTP fetch.
// Concurrent callers are coalesced via singleflight to avoid thundering herd.
// Never blocks for more than manifestTimeout. Returns nil on any failure.
func (mc *ManifestClient) Fetch(ctx context.Context) *ManifestPayload {
	mc.mu.RLock()
	if mc.cached != nil && time.Since(mc.cachedAt) < mc.cacheTTL {
		defer mc.mu.RUnlock()
		return mc.cached
	}
	stale := mc.cached
	mc.mu.RUnlock()

	v, _, _ := mc.sfGroup.Do("fetch", func() (any, error) {
		return mc.doFetch(ctx), nil
	})
	if fetched, ok := v.(*ManifestPayload); ok && fetched != nil {
		mc.mu.Lock()
		mc.cached = fetched
		mc.cachedAt = time.Now()
		mc.mu.Unlock()
		return fetched
	}

	// Prefer stale over nothing.
	return stale
}

// Refresh forces a cache refresh. Same non-blocking, best-effort policy as Fetch.
func (mc *ManifestClient) Refresh(ctx context.Context) {
	if fetched := mc.doFetch(ctx); fetched != nil {
		mc.mu.Lock()
		mc.cached = fetched
		mc.cachedAt = time.Now()
		mc.mu.Unlock()
	}
}

// HasActiveSource returns true if the manifest contains at least one source with
// "active" status, indicating that semsource has finished indexing at least one
// knowledge source. Used by questbridge to soft-gate dispatch until graph data
// is available for entity knowledge injection.
func (mc *ManifestClient) HasActiveSource(ctx context.Context) bool {
	manifest := mc.Fetch(ctx)
	if manifest == nil {
		return false
	}
	for _, src := range manifest.Sources {
		if src.Status == "active" {
			return true
		}
	}
	return false
}

// FormatForPrompt fetches the manifest (using cache when fresh) and formats it
// for LLM consumption. Returns "" if the manifest is unavailable or empty.
func (mc *ManifestClient) FormatForPrompt(ctx context.Context) string {
	manifest := mc.Fetch(ctx)
	if manifest == nil || len(manifest.Sources) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("--- Available Knowledge Sources ---\n")
	sb.WriteString("The following sources have been indexed into the knowledge graph. ")
	sb.WriteString("Use graph_search (query_type: \"search\") to find information from these sources.\n\n")

	for _, src := range manifest.Sources {
		sb.WriteString(fmt.Sprintf("- %s", src.Name))
		if src.Type != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", src.Type))
		}
		if src.Status != "" && src.Status != "active" {
			sb.WriteString(fmt.Sprintf(" [%s]", src.Status))
		}
		sb.WriteByte('\n')
		if src.Description != "" {
			sb.WriteString(fmt.Sprintf("  %s\n", src.Description))
		}
		if len(src.EntityTypes) > 0 {
			sb.WriteString(fmt.Sprintf("  Entity types: %s\n", strings.Join(src.EntityTypes, ", ")))
		}
	}

	return sb.String()
}

// doFetch performs the HTTP request with a short timeout.
func (mc *ManifestClient) doFetch(ctx context.Context) *ManifestPayload {
	ctx, cancel := context.WithTimeout(ctx, manifestTimeout)
	defer cancel()

	reqURL := mc.baseURL + manifestPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		mc.logger.Debug("manifest: failed to create request", "url", reqURL, "error", err)
		return nil
	}
	req.Header.Set("Accept", "application/json")

	resp, err := manifestHTTPClient.Do(req)
	if err != nil {
		mc.logger.Debug("manifest: fetch failed", "url", reqURL, "error", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		mc.logger.Debug("manifest: non-200 response", "url", reqURL, "status", resp.StatusCode)
		return nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxManifestBytes))
	if err != nil {
		mc.logger.Debug("manifest: failed to read body", "error", err)
		return nil
	}

	var payload ManifestPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		mc.logger.Debug("manifest: failed to parse JSON", "error", err)
		return nil
	}

	return &payload
}

// wsURLToHTTP converts a websocket URL to an HTTP base URL by extracting
// the scheme and host. ws:// → http://, wss:// → https://.
// Returns "" if the URL is empty or unrecognized.
func wsURLToHTTP(wsURL string) string {
	wsURL = strings.TrimSpace(wsURL)
	if wsURL == "" {
		return ""
	}

	// Normalize scheme for url.Parse.
	var normalized string
	switch {
	case strings.HasPrefix(wsURL, "wss://"):
		normalized = "https://" + strings.TrimPrefix(wsURL, "wss://")
	case strings.HasPrefix(wsURL, "ws://"):
		normalized = "http://" + strings.TrimPrefix(wsURL, "ws://")
	default:
		return ""
	}

	u, err := url.Parse(normalized)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
