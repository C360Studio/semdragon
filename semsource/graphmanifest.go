package semsource

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// GraphManifest summarizes what the graph-gateway contains, excluding
// game-world predicates that agents already receive via entity knowledge
// injection.
type GraphManifest struct {
	// PredicateFamilies maps top-level predicate prefixes (e.g. "source", "doc")
	// to the total entity count across all predicates in that family.
	// Game-world families (agent, quest, battle, etc.) are filtered out.
	PredicateFamilies map[string]int
	// PredicateCategories maps two-level predicate prefixes (e.g. "source.content")
	// to entity counts, providing more granular discovery hints for agents.
	PredicateCategories map[string]int
	// TotalPredicates is the number of distinct non-game predicates.
	TotalPredicates int
}

// gamePredicateFamilies are predicate families that belong to the game world.
// Agents already receive this context via entity knowledge injection. Exposing
// raw graph predicates would be redundant and could enable cheating (e.g.,
// looking up boss battle criteria or other agents' progression).
var gamePredicateFamilies = map[string]bool{
	"agent":  true,
	"quest":  true,
	"battle": true,
	"party":  true,
	"review": true,
	"guild":  true,
	"store":  true,
}

const (
	graphManifestCacheTTL = 5 * time.Minute
	graphManifestTimeout  = 3 * time.Second
	maxGraphManifestBytes = 1 << 20 // 1 MiB

	// GraphQL query to fetch all predicates with entity counts.
	predicatesQuery = `{"query":"{ predicates { predicates { predicate entityCount } total } }"}`
)

// graphManifestHTTPClient is a dedicated HTTP client for graph manifest fetches.
// Timeout is set slightly above the per-request context timeout so that context
// cancellation governs normal flow while the client timeout acts as a hard ceiling.
var graphManifestHTTPClient = &http.Client{
	Timeout: graphManifestTimeout + 1*time.Second,
}

// GraphManifestClient fetches and caches a summary of graph-gateway contents.
// All methods are safe for concurrent use and never return errors — failures
// are logged at debug level and result in nil/empty returns.
type GraphManifestClient struct {
	graphqlURL string
	logger     *slog.Logger

	mu       sync.RWMutex
	cached   *GraphManifest
	cachedAt time.Time
	sfGroup  singleflight.Group
}

// NewGraphManifestClient creates a client that queries the graph-gateway GraphQL
// endpoint for predicate statistics. Returns nil if graphqlURL is empty.
func NewGraphManifestClient(graphqlURL string, logger *slog.Logger) *GraphManifestClient {
	graphqlURL = strings.TrimSpace(graphqlURL)
	if graphqlURL == "" {
		return nil
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &GraphManifestClient{
		graphqlURL: graphqlURL,
		logger:     logger,
	}
}

// Fetch returns the cached manifest if fresh, otherwise queries graph-gateway.
// Concurrent callers are coalesced via singleflight. The singleflight callback
// uses context.WithoutCancel so that one caller's cancellation doesn't abort
// the fetch for all coalesced callers. Returns nil on failure.
func (c *GraphManifestClient) Fetch(ctx context.Context) *GraphManifest {
	c.mu.RLock()
	if c.cached != nil && time.Since(c.cachedAt) < graphManifestCacheTTL {
		defer c.mu.RUnlock()
		return c.cached
	}
	stale := c.cached
	c.mu.RUnlock()

	v, _, _ := c.sfGroup.Do("fetch", func() (any, error) {
		// Use WithoutCancel so a single caller's cancellation doesn't
		// abort the shared fetch for all coalesced callers.
		return c.doFetch(context.WithoutCancel(ctx)), nil
	})
	if fetched, ok := v.(*GraphManifest); ok && fetched != nil {
		c.mu.Lock()
		c.cached = fetched
		c.cachedAt = time.Now()
		c.mu.Unlock()
		return fetched
	}

	return stale
}

// FormatForPrompt fetches the manifest and formats it for LLM consumption.
// Returns "" when unavailable or only game-world predicates are present.
func (c *GraphManifestClient) FormatForPrompt(ctx context.Context) string {
	manifest := c.Fetch(ctx)
	if manifest == nil || len(manifest.PredicateFamilies) == 0 {
		return ""
	}

	// Sort families for stable output.
	families := make([]string, 0, len(manifest.PredicateFamilies))
	totalEntities := 0
	for f, count := range manifest.PredicateFamilies {
		families = append(families, f)
		totalEntities += count
	}
	sort.Strings(families)

	var sb strings.Builder
	sb.WriteString("--- Graph Contents ---\n")
	sb.WriteString(fmt.Sprintf("%d knowledge sources indexed (%d entities, %d predicates).\n\n",
		len(families), totalEntities, manifest.TotalPredicates))

	// Show each family with its two-level categories.
	for _, fam := range families {
		cats := categoriesForFamily(manifest.PredicateCategories, fam)
		if len(cats) > 0 {
			sb.WriteString(fmt.Sprintf("  %s (%d): %s\n", fam, manifest.PredicateFamilies[fam], strings.Join(cats, ", ")))
		} else {
			sb.WriteString(fmt.Sprintf("  %s (%d)\n", fam, manifest.PredicateFamilies[fam]))
		}
	}

	sb.WriteString("\nQuery with graph_search: use \"predicate\" for targeted lookups\n")
	sb.WriteString("or \"prefix\" for entity ID patterns. Avoid broad \"search\" queries.\n")

	return sb.String()
}

// categoriesForFamily returns the sorted category suffixes (second dot segment)
// for a given family prefix.
func categoriesForFamily(categories map[string]int, family string) []string {
	prefix := family + "."
	var cats []string
	for cat := range categories {
		if suffix, ok := strings.CutPrefix(cat, prefix); ok {
			cats = append(cats, suffix)
		}
	}
	sort.Strings(cats)
	return cats
}

// predicateEntry represents a single predicate with its entity count from graph-gateway.
type predicateEntry struct {
	Predicate   string `json:"predicate"`
	EntityCount int    `json:"entity_count"`
}

// predicatesResponse mirrors the GraphQL response shape for the predicates query.
type predicatesResponse struct {
	Data struct {
		Predicates struct {
			Predicates []predicateEntry `json:"predicates"`
			Total      int              `json:"total"`
		} `json:"predicates"`
	} `json:"data"`
	Errors []graphQLError `json:"errors,omitempty"`
}

// graphQLError represents a single error in a GraphQL response.
type graphQLError struct {
	Message string `json:"message"`
}

// doFetch performs the GraphQL request with a short timeout.
func (c *GraphManifestClient) doFetch(ctx context.Context) *GraphManifest {
	ctx, cancel := context.WithTimeout(ctx, graphManifestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.graphqlURL,
		strings.NewReader(predicatesQuery))
	if err != nil {
		c.logger.Debug("graph manifest: failed to create request", "url", c.graphqlURL, "error", err)
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := graphManifestHTTPClient.Do(req)
	if err != nil {
		c.logger.Debug("graph manifest: fetch failed", "url", c.graphqlURL, "error", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.logger.Debug("graph manifest: non-200 response", "url", c.graphqlURL, "status", resp.StatusCode)
		return nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxGraphManifestBytes))
	if err != nil {
		c.logger.Debug("graph manifest: failed to read body", "error", err)
		return nil
	}

	var gqlResp predicatesResponse
	if err := json.Unmarshal(body, &gqlResp); err != nil {
		c.logger.Debug("graph manifest: failed to parse JSON", "error", err)
		return nil
	}

	if len(gqlResp.Errors) > 0 {
		msgs := make([]string, len(gqlResp.Errors))
		for i, e := range gqlResp.Errors {
			msgs[i] = e.Message
		}
		c.logger.Debug("graph manifest: GraphQL errors", "errors", strings.Join(msgs, "; "))
		return nil
	}

	// Group predicates by top-level family and two-level category,
	// filtering out game-world predicates.
	families := make(map[string]int)
	categories := make(map[string]int)
	filteredPredicateCount := 0

	for _, p := range gqlResp.Data.Predicates.Predicates {
		family := p.Predicate
		if idx := strings.IndexByte(p.Predicate, '.'); idx > 0 {
			family = p.Predicate[:idx]
		}

		// Skip game-world predicates — agents get this via entity knowledge.
		if gamePredicateFamilies[family] {
			continue
		}

		families[family] += p.EntityCount
		filteredPredicateCount++

		// Two-level category: "source.content" from "source.content.language"
		parts := strings.SplitN(p.Predicate, ".", 3)
		if len(parts) >= 2 {
			cat := parts[0] + "." + parts[1]
			categories[cat] += p.EntityCount
		}
	}

	return &GraphManifest{
		PredicateFamilies:   families,
		PredicateCategories: categories,
		TotalPredicates:     filteredPredicateCount,
	}
}
