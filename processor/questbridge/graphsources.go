package questbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------------------------
// Global graph source registry (semspec pattern)
// ---------------------------------------------------------------------------
// Initialized once in main.go from the top-level "graph_sources" config.
// Components call GlobalGraphSources() during Start() — nil means not configured.

var (
	globalGraphSources   *GraphSourceRegistry
	globalGraphSourcesMu sync.RWMutex
)

// SetGlobalGraphSources stores the process-wide graph source registry.
// Called once during application startup before components start.
func SetGlobalGraphSources(r *GraphSourceRegistry) {
	globalGraphSourcesMu.Lock()
	globalGraphSources = r
	globalGraphSourcesMu.Unlock()
}

// GlobalGraphSources returns the process-wide graph source registry, or nil
// when graph sources are not configured.
func GlobalGraphSources() *GraphSourceRegistry {
	globalGraphSourcesMu.RLock()
	defer globalGraphSourcesMu.RUnlock()
	return globalGraphSources
}

// GraphSource represents a queryable graph endpoint.
type GraphSource struct {
	// Name identifies this source (e.g., "local", "osh").
	Name string `json:"name"`

	// GraphQLURL is the graph-gateway GraphQL endpoint.
	GraphQLURL string `json:"graphql_url"`

	// StatusURL is the semsource readiness endpoint (empty for local sources).
	// GET returns StatusPayload with aggregate phase.
	StatusURL string `json:"status_url,omitempty"`

	// Type is "local" (our graph) or "semsource" (external knowledge source).
	Type string `json:"type"`

	// EntityPrefix is the entity ID prefix owned by this source (e.g., "osh.").
	// Used for prefix-based routing of entity/relationship queries.
	EntityPrefix string `json:"entity_prefix,omitempty"`

	// AlwaysQuery means this source is queried for every search/nlq query.
	// Local sources are always queried; semsource sources are only queried when ready.
	AlwaysQuery bool `json:"always_query"`

	// ready is set to true when the source reports phase "ready" or "degraded".
	ready atomic.Bool
	// FailCount tracks consecutive status check failures for fast-skip.
	FailCount int `json:"-"`
}

// GraphSourceRegistry manages multiple graph sources for query routing.
type GraphSourceRegistry struct {
	sources []*GraphSource
	logger  *slog.Logger
	client  *http.Client

	// Summary cache for prompt injection — keyed by summary URL.
	summaryMu    sync.Mutex
	summaryCache map[string]summCacheEntry
}

// summCacheEntry holds a parsed semsource summary with its fetch timestamp.
type summCacheEntry struct {
	summary *sourceSummary
	fetched time.Time
}

// sourceSummary mirrors the semsource /summary response JSON.
type sourceSummary struct {
	Namespace      string         `json:"namespace"`
	Phase          string         `json:"phase"`
	EntityIDFormat string         `json:"entity_id_format"`
	TotalEntities  int            `json:"total_entities"`
	Domains        []SummaryDomain `json:"domains"`
}

// SummaryDomain is the per-domain section of a semsource /summary response.
// Exported so that SourceSummaryData.Domains is a fully exported type.
type SummaryDomain struct {
	Domain      string        `json:"domain"`
	EntityCount int           `json:"entity_count"`
	Types       []SummaryType `json:"types"`
	Sources     []string      `json:"sources"`
}

// SummaryType is the per-entity-type breakdown within a SummaryDomain.
// Exported so that SourceSummaryData.Domains[n].Types is a fully exported type.
type SummaryType struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

const summCacheTTL = 5 * time.Minute

// NewGraphSourceRegistry creates a registry from config.
func NewGraphSourceRegistry(sources []GraphSource, logger *slog.Logger) *GraphSourceRegistry {
	ptrs := make([]*GraphSource, len(sources))
	for i := range sources {
		ptrs[i] = &sources[i]
		// Local sources are always ready.
		if sources[i].Type == "local" || sources[i].AlwaysQuery {
			ptrs[i].ready.Store(true)
		}
	}
	return &GraphSourceRegistry{
		sources:      ptrs,
		logger:       logger,
		client:       &http.Client{Timeout: 5 * time.Second},
		summaryCache: make(map[string]summCacheEntry),
	}
}

// SourcesForQuery returns the graph sources that should handle a given query.
// For entity/relationship queries with an entity ID, routes to the matching prefix.
// For search/nlq queries, fans out to all ready sources.
func (r *GraphSourceRegistry) SourcesForQuery(queryType, entityID, prefix string) []*GraphSource {
	switch queryType {
	case "entity", "relationships":
		// Route to the source that owns this entity ID.
		if entityID != "" {
			if src := r.resolveByPrefix(entityID); src != nil {
				return []*GraphSource{src}
			}
		}
		// Fallback: query all ready sources.
		return r.readySources()

	case "prefix":
		// Route to the source matching the prefix.
		if prefix != "" {
			if src := r.resolveByPrefix(prefix); src != nil {
				return []*GraphSource{src}
			}
		}
		return r.readySources()

	case "search", "nlq", "predicate":
		// Fan out to all ready sources.
		return r.readySources()

	case "summary":
		// Only semsource sources expose the /summary endpoint.
		return r.SourcesForSummary()

	default:
		return r.readySources()
	}
}

// ResolveEntity returns the source that owns a given entity ID, or nil.
func (r *GraphSourceRegistry) ResolveEntity(entityID string) *GraphSource {
	return r.resolveByPrefix(entityID)
}

// GraphQLURLsForQuery implements executor.GraphSearchRouter.
// Returns the GraphQL endpoint URLs to query for a given query type and entity context.
func (r *GraphSourceRegistry) GraphQLURLsForQuery(queryType, entityID, prefix string) []string {
	sources := r.SourcesForQuery(queryType, entityID, prefix)
	urls := make([]string, 0, len(sources))
	for _, src := range sources {
		if src.GraphQLURL != "" {
			urls = append(urls, src.GraphQLURL)
		}
	}
	return urls
}

// SummaryURL derives the summary endpoint URL from StatusURL by replacing
// "/status" with "/summary". Returns an empty string when StatusURL is empty.
func (s *GraphSource) SummaryURL() string {
	if s.StatusURL == "" {
		return ""
	}
	return strings.Replace(s.StatusURL, "/status", "/summary", 1)
}

// GraphSummaryRouter returns summary endpoint URLs for ready semsource sources.
type GraphSummaryRouter interface {
	SummaryURLs() []string
}

// SummaryURLs returns the summary endpoint URLs for all ready semsource sources.
// Local graph sources do not expose this endpoint and are excluded.
func (r *GraphSourceRegistry) SummaryURLs() []string {
	var urls []string
	for _, src := range r.sources {
		if src.Type != "semsource" || !src.ready.Load() {
			continue
		}
		if u := src.SummaryURL(); u != "" {
			urls = append(urls, u)
		}
	}
	return urls
}

// SourcesForSummary returns only ready semsource sources — used by graph_summary tool.
// Local graph sources don't have the /summary endpoint.
func (r *GraphSourceRegistry) SourcesForSummary() []*GraphSource {
	var result []*GraphSource
	for _, src := range r.sources {
		if src.Type == "semsource" && src.ready.Load() && src.SummaryURL() != "" {
			result = append(result, src)
		}
	}
	return result
}

// HasSemsources returns true if any semsource-type sources are configured.
func (r *GraphSourceRegistry) HasSemsources() bool {
	for _, src := range r.sources {
		if src.Type == "semsource" {
			return true
		}
	}
	return false
}

// WaitForReady polls all semsource sources until they report ready.
// Returns nil when all sources are ready, or an error on timeout.
func (r *GraphSourceRegistry) WaitForReady(ctx context.Context, timeout time.Duration) error {
	if !r.HasSemsources() {
		return nil
	}

	deadline := time.After(timeout)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Check immediately before entering the loop.
	if r.checkAllReady(ctx) {
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			// Log which sources aren't ready.
			for _, src := range r.sources {
				if src.Type == "semsource" && !src.ready.Load() {
					r.logger.Warn("semsource not ready at timeout",
						"source", src.Name, "status_url", src.StatusURL)
				}
			}
			return fmt.Errorf("semsource readiness timeout after %s", timeout)
		case <-ticker.C:
			if r.checkAllReady(ctx) {
				return nil
			}
		}
	}
}

// statusPayload matches semsource's StatusPayload schema.
type statusPayload struct {
	Phase         string         `json:"phase"`
	Sources       []sourceStatus `json:"sources"`
	TotalEntities int            `json:"total_entities"`
}

type sourceStatus struct {
	InstanceName string `json:"instance_name"`
	SourceType   string `json:"source_type"`
	Phase        string `json:"phase"`
	EntityCount  int    `json:"entity_count"`
	ErrorCount   int    `json:"error_count"`
}

// checkAllReady polls each semsource source and returns true if all are ready.
func (r *GraphSourceRegistry) checkAllReady(ctx context.Context) bool {
	allReady := true
	for _, src := range r.sources {
		if src.Type != "semsource" || src.ready.Load() {
			continue
		}
		if src.StatusURL == "" {
			src.ready.Store(true)
			continue
		}

		phase, entities, err := r.fetchStatus(ctx, src.StatusURL)
		if err != nil {
			src.FailCount++
			r.logger.Debug("semsource status check failed",
				"source", src.Name, "error", err, "consecutive_failures", src.FailCount)
			// After 3 consecutive failures, treat as degraded and proceed.
			// Prevents blocking quest dispatch when semsource is unreachable.
			if src.FailCount >= 3 {
				src.ready.Store(true)
				r.logger.Warn("semsource unreachable after 3 attempts, proceeding without",
					"source", src.Name)
				continue
			}
			allReady = false
			continue
		}
		src.FailCount = 0 // Reset on successful contact

		switch phase {
		case "ready":
			src.ready.Store(true)
			r.logger.Info("semsource ready",
				"source", src.Name, "entities", entities)
		case "degraded":
			src.ready.Store(true)
			r.logger.Warn("semsource degraded (proceeding with partial data)",
				"source", src.Name, "entities", entities)
		default:
			r.logger.Debug("semsource not yet ready",
				"source", src.Name, "phase", phase, "entities", entities)
			allReady = false
		}
	}
	return allReady
}

// fetchStatus calls a semsource status endpoint and returns aggregate phase + entity count.
func (r *GraphSourceRegistry) fetchStatus(ctx context.Context, statusURL string) (string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
	if err != nil {
		return "", 0, err
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", 0, err
	}

	var status statusPayload
	if err := json.Unmarshal(body, &status); err != nil {
		return "", 0, err
	}

	return status.Phase, status.TotalEntities, nil
}

// readySources returns all sources that are ready to be queried.
func (r *GraphSourceRegistry) readySources() []*GraphSource {
	var result []*GraphSource
	for _, src := range r.sources {
		if src.ready.Load() {
			result = append(result, src)
		}
	}
	return result
}

// resolveByPrefix finds the source whose EntityPrefix matches the given ID.
// Falls back to the first local source if no prefix matches.
func (r *GraphSourceRegistry) resolveByPrefix(id string) *GraphSource {
	var localFallback *GraphSource
	for _, src := range r.sources {
		if src.EntityPrefix != "" && strings.HasPrefix(id, src.EntityPrefix) {
			if src.ready.Load() {
				return src
			}
			return nil // Source owns this prefix but isn't ready.
		}
		if src.Type == "local" && localFallback == nil {
			localFallback = src
		}
	}
	return localFallback
}

// FormatSummaryForPrompt fetches and formats aggregated semsource summary data
// for injection into agent prompts. Only ready semsource sources are queried.
// Results are cached for summCacheTTL (5 minutes) to avoid hammering the endpoint.
// Returns empty string when no semsource sources are ready or all fetches fail.
func (r *GraphSourceRegistry) FormatSummaryForPrompt(ctx context.Context) string {
	semsources := r.SourcesForSummary()
	if len(semsources) == 0 {
		return ""
	}

	// Sort by name for stable output across calls.
	sorted := make([]*GraphSource, len(semsources))
	copy(sorted, semsources)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].Name < sorted[j-1].Name; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}

	type fetchedSrc struct {
		src     *GraphSource
		summary *sourceSummary
	}

	var fetched []fetchedSrc
	totalEntities := 0

	for _, src := range sorted {
		summURL := src.SummaryURL()
		if summURL == "" {
			continue
		}

		sm := r.fetchSummaryWithCache(ctx, summURL)
		if sm == nil {
			continue
		}

		fetched = append(fetched, fetchedSrc{src: src, summary: sm})
		totalEntities += sm.TotalEntities
	}

	if len(fetched) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("--- Graph Contents ---\n")
	sb.WriteString(fmt.Sprintf("%d knowledge source", len(fetched)))
	if len(fetched) != 1 {
		sb.WriteString("s")
	}
	sb.WriteString(fmt.Sprintf(", %d entities total.\n\n", totalEntities))

	// Entity ID format guidance.
	sb.WriteString("Entity IDs use 6-part dotted notation: org.platform.domain.system.type.instance\n")
	for _, f := range fetched {
		if f.src.EntityPrefix != "" {
			// Derive a compact format hint from the entity prefix.
			prefix := strings.TrimSuffix(f.src.EntityPrefix, ".")
			sb.WriteString(fmt.Sprintf("  %s: %s.{domain}.{system}.{type}.{instance}\n",
				f.src.Name, prefix))
		}
	}

	// Per-source entity breakdown.
	for _, f := range fetched {
		sb.WriteString(fmt.Sprintf("\n%s (%d entities):\n", f.src.Name, f.summary.TotalEntities))
		for _, d := range f.summary.Domains {
			if len(d.Types) == 0 {
				continue
			}
			var typeParts []string
			for _, t := range d.Types {
				typeParts = append(typeParts, fmt.Sprintf("%s (%d)", t.Type, t.Count))
			}
			sb.WriteString(fmt.Sprintf("  %s: %s\n", d.Domain, strings.Join(typeParts, ", ")))
		}
	}

	sb.WriteString("\nQuery with graph_search: use \"prefix\" to scope by source (e.g. ")
	if len(fetched) > 0 && fetched[0].src.EntityPrefix != "" {
		prefix := strings.TrimSuffix(fetched[0].src.EntityPrefix, ".")
		// Use first domain from first source for example.
		exampleDomain := ""
		if len(fetched[0].summary.Domains) > 0 {
			exampleDomain = fetched[0].summary.Domains[0].Domain
		}
		if exampleDomain != "" {
			sb.WriteString(fmt.Sprintf("%q", prefix+"."+exampleDomain))
		} else {
			sb.WriteString(fmt.Sprintf("%q", prefix))
		}
	} else {
		sb.WriteString(`"source.domain"`)
	}
	sb.WriteString(`), "predicate" for targeted lookups, or "nlq" for natural language questions.` + "\n")
	sb.WriteString(`Use graph_summary tool for full predicate schema with descriptions.`)

	return sb.String()
}

// SourceSummaryData is the structured per-source summary returned by StructuredSummary.
// It mirrors the semsource /summary response with added source metadata for API consumers.
type SourceSummaryData struct {
	Name          string          `json:"name"`
	Type          string          `json:"type"`
	Ready         bool            `json:"ready"`
	EntityPrefix  string          `json:"entity_prefix,omitempty"`
	TotalEntities int             `json:"total_entities"`
	Domains       []SummaryDomain `json:"domains"`
}

// StructuredSummary returns per-source summary data for all configured semsource sources,
// fetching from each source's /summary endpoint (with caching). Local sources are included
// with zero entity counts since they do not expose the /summary endpoint.
// This method is used by the REST API to expose the same data agents see via graph_summary.
func (r *GraphSourceRegistry) StructuredSummary(ctx context.Context) []SourceSummaryData {
	result := make([]SourceSummaryData, 0, len(r.sources))
	for _, src := range r.sources {
		entry := SourceSummaryData{
			Name:         src.Name,
			Type:         src.Type,
			Ready:        src.ready.Load(),
			EntityPrefix: src.EntityPrefix,
			Domains:      []SummaryDomain{},
		}

		if src.Type == "semsource" && src.ready.Load() {
			summURL := src.SummaryURL()
			if summURL != "" {
				if sm := r.fetchSummaryWithCache(ctx, summURL); sm != nil {
					entry.TotalEntities = sm.TotalEntities
					entry.Domains = sm.Domains
				}
			}
		}

		result = append(result, entry)
	}
	return result
}

// SummaryWithText returns both the formatted prompt text and the structured
// per-source data in a single pass, avoiding the double cache lookup that
// results from calling FormatSummaryForPrompt and StructuredSummary separately.
//
// The text field is the same string FormatSummaryForPrompt would return.
// Empty text means no semsource sources are ready (check sources for details).
func (r *GraphSourceRegistry) SummaryWithText(ctx context.Context) (string, []SourceSummaryData) {
	sources := r.StructuredSummary(ctx)

	// Collect only semsource entries that have data to format.
	type readySrc struct {
		data SourceSummaryData
	}
	var ready []readySrc
	totalEntities := 0
	for _, s := range sources {
		if s.Type == "semsource" && s.Ready && s.TotalEntities > 0 {
			ready = append(ready, readySrc{data: s})
			totalEntities += s.TotalEntities
		}
	}

	if len(ready) == 0 {
		return "", sources
	}

	// Sort by name for stable output — insertion sort (small n).
	for i := 1; i < len(ready); i++ {
		for j := i; j > 0 && ready[j].data.Name < ready[j-1].data.Name; j-- {
			ready[j], ready[j-1] = ready[j-1], ready[j]
		}
	}

	var sb strings.Builder
	sb.WriteString("--- Graph Contents ---\n")
	sb.WriteString(fmt.Sprintf("%d knowledge source", len(ready)))
	if len(ready) != 1 {
		sb.WriteString("s")
	}
	sb.WriteString(fmt.Sprintf(", %d entities total.\n\n", totalEntities))

	sb.WriteString("Entity IDs use 6-part dotted notation: org.platform.domain.system.type.instance\n")
	for _, f := range ready {
		if f.data.EntityPrefix != "" {
			prefix := strings.TrimSuffix(f.data.EntityPrefix, ".")
			sb.WriteString(fmt.Sprintf("  %s: %s.{domain}.{system}.{type}.{instance}\n",
				f.data.Name, prefix))
		}
	}

	for _, f := range ready {
		sb.WriteString(fmt.Sprintf("\n%s (%d entities):\n", f.data.Name, f.data.TotalEntities))
		for _, d := range f.data.Domains {
			if len(d.Types) == 0 {
				continue
			}
			var typeParts []string
			for _, t := range d.Types {
				typeParts = append(typeParts, fmt.Sprintf("%s (%d)", t.Type, t.Count))
			}
			sb.WriteString(fmt.Sprintf("  %s: %s\n", d.Domain, strings.Join(typeParts, ", ")))
		}
	}

	sb.WriteString("\nQuery with graph_search: use \"prefix\" to scope by source (e.g. ")
	if ready[0].data.EntityPrefix != "" {
		prefix := strings.TrimSuffix(ready[0].data.EntityPrefix, ".")
		exampleDomain := ""
		if len(ready[0].data.Domains) > 0 {
			exampleDomain = ready[0].data.Domains[0].Domain
		}
		if exampleDomain != "" {
			sb.WriteString(fmt.Sprintf("%q", prefix+"."+exampleDomain))
		} else {
			sb.WriteString(fmt.Sprintf("%q", prefix))
		}
	} else {
		sb.WriteString(`"source.domain"`)
	}
	sb.WriteString(`), "predicate" for targeted lookups, or "nlq" for natural language questions.` + "\n")
	sb.WriteString(`Use graph_summary tool for full predicate schema with descriptions.`)

	return sb.String(), sources
}

// fetchSummaryWithCache retrieves a parsed sourceSummary for the given URL,
// serving from cache when the entry is still within summCacheTTL.
func (r *GraphSourceRegistry) fetchSummaryWithCache(ctx context.Context, url string) *sourceSummary {
	r.summaryMu.Lock()
	entry, ok := r.summaryCache[url]
	r.summaryMu.Unlock()

	if ok && time.Since(entry.fetched) < summCacheTTL {
		return entry.summary
	}

	sm, err := r.fetchSummary(ctx, url)
	if err != nil {
		r.logger.Debug("failed to fetch semsource summary for prompt", "url", url, "error", err)
		return nil
	}

	r.summaryMu.Lock()
	r.summaryCache[url] = summCacheEntry{summary: sm, fetched: time.Now()}
	r.summaryMu.Unlock()

	return sm
}

// fetchSummary calls a semsource /summary endpoint and parses the response.
func (r *GraphSourceRegistry) fetchSummary(ctx context.Context, summaryURL string) (*sourceSummary, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, summaryURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d from %s", resp.StatusCode, summaryURL)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	var sm sourceSummary
	if err := json.Unmarshal(body, &sm); err != nil {
		return nil, fmt.Errorf("parse summary response: %w", err)
	}

	return &sm, nil
}
