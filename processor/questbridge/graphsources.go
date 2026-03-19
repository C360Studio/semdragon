package questbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

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
}

// GraphSourceRegistry manages multiple graph sources for query routing.
type GraphSourceRegistry struct {
	sources []*GraphSource
	logger  *slog.Logger
	client  *http.Client
}

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
		sources: ptrs,
		logger:  logger,
		client:  &http.Client{Timeout: 5 * time.Second},
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
			r.logger.Debug("semsource status check failed",
				"source", src.Name, "error", err)
			allReady = false
			continue
		}

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
