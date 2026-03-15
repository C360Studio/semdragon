package bossbattle

import (
	"context"
	"time"

	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// INDEXING WATCHER - Correlates semsource entities to merged quest artifacts
// =============================================================================
// watchForIndexing runs as a detached goroutine after a successful MergeToMain.
// It waits for semsource to process the merged files, queries the graph for
// produced entities, writes quest.produced edges, then sets ArtifactsIndexed=true.
//
// When semsource is not configured (IndexingTimeout == 0) the function sets
// indexed=true immediately and returns. When the timeout elapses before semsource
// responds, it falls back to degraded mode: indexed=true with no produced edges,
// logging a warning so operators know the correlation window was missed.
//
// The goroutine uses a fresh Background context with its own deadline so it
// survives cancellation of the caller's context (which goes away after the
// battle verdict is written).
// =============================================================================

// watchForIndexing correlates semsource entities to the files introduced by
// mergeHash and writes quest.produced edges + ArtifactsIndexed=true.
// Must be called as a goroutine ("go c.watchForIndexing(quest, mergeHash)").
func (c *Component) watchForIndexing(questID domain.QuestID, mergeHash string) {
	timeout := time.Duration(c.config.IndexingTimeout) * time.Second

	// IndexingTimeout == 0 means "mark immediately, semsource not running".
	// We still need a bounded context for the KV write, so we always allocate
	// at least a short deadline.
	const minWriteTimeout = 10 * time.Second
	if timeout <= 0 {
		ctx, cancel := context.WithTimeout(context.Background(), minWriteTimeout)
		defer cancel()
		c.setIndexed(ctx, questID, nil)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Fetch the list of files introduced by the merge commit.
	// If workspaceRepo is nil we have no file list to correlate against,
	// so fall through to the degraded-mode write immediately.
	var mergedFiles []string
	if c.workspaceRepo != nil && mergeHash != "" {
		files, err := c.workspaceRepo.MergedFiles(ctx, mergeHash)
		if err != nil {
			c.logger.Warn("indexing watcher: could not list merged files — skipping correlation",
				"quest", questID, "merge_hash", mergeHash, "error", err)
		} else {
			mergedFiles = files
		}
	}

	// Poll the graph every 5 seconds looking for semsource entities whose
	// "code.artifact.path" predicate matches one of the merged file paths.
	// We stop as soon as we have at least one match for each known file, or
	// the context deadline fires.
	//
	// Semsource typically indexes within a few seconds of the filesystem change.
	// A 5-second tick gives it two or three chances before a 30s timeout.
	const pollInterval = 5 * time.Second

	var producedIDs []string

	if len(mergedFiles) > 0 {
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

	poll:
		for {
			ids, done := c.correlateEntities(ctx, mergedFiles)
			if done || len(ids) >= len(mergedFiles) {
				producedIDs = ids
				break poll
			}

			select {
			case <-ctx.Done():
				producedIDs = ids
				c.logger.Warn("indexing watcher: timeout waiting for semsource — marking indexed in degraded mode",
					"quest", questID,
					"merge_hash", mergeHash,
					"files_expected", len(mergedFiles),
					"entities_found", len(producedIDs))
				break poll
			case <-ticker.C:
				// Keep polling.
			}
		}
	} else {
		// No merged file list available (workspace repo absent or empty merge).
		// Nothing to correlate — fall through to setIndexed immediately.
	}

	c.setIndexed(ctx, questID, producedIDs)
}

// correlateEntities searches the graph for semsource entities that match the
// provided file paths via the "code.artifact.path" predicate. It returns the
// matched entity IDs and a done flag. done=true means we should stop polling
// (e.g., context cancelled or graph unreachable).
//
// The current implementation uses a direct KV scan because graph-gateway GraphQL
// is not yet wired into bossbattle. When graph-gateway is available, replace this
// with a GraphQL query for entitiesByPredicate.
// paths parameter reserved for graph-gateway correlation query.
func (c *Component) correlateEntities(ctx context.Context, _ []string) (matched []string, done bool) { //nolint:revive // paths reserved for graph-gateway wiring
	// Check context first — no point querying if we're already cancelled.
	if ctx.Err() != nil {
		return nil, true
	}

	// TODO(semsource): Replace with GraphQL query once graph-gateway is wired:
	//   query { entitiesByPredicate(predicate: "code.artifact.path", values: $paths) { id } }
	//
	// For now we return no matches so the caller falls through to degraded mode
	// after the timeout. When semsource entities are present in the ENTITY_STATES
	// bucket they carry "code.artifact.path" triples; a future implementation can
	// iterate the bucket with a wildcard pattern to find them.
	return nil, false
}

// setIndexed re-fetches the quest from KV, appends producedIDs, sets
// ArtifactsIndexed=true, and writes via CAS. Retries up to 3 times on
// revision conflict before giving up (rare: no other processor writes
// a completed quest's indexing fields).
func (c *Component) setIndexed(ctx context.Context, questID domain.QuestID, producedIDs []string) {
	const maxRetries = 3

	for attempt := range maxRetries {
		entityState, revision, err := c.graph.GetQuestWithRevision(ctx, questID)
		if err != nil {
			c.logger.Error("indexing watcher: failed to re-fetch quest for indexed write",
				"quest", questID, "attempt", attempt+1, "error", err)
			return
		}

		current := domain.QuestFromEntityState(entityState)
		if current == nil {
			c.logger.Error("indexing watcher: failed to reconstruct quest from entity state",
				"quest", questID)
			return
		}

		// Merge produced entities — avoid duplicates in case of a retry.
		existing := make(map[string]struct{}, len(current.ProducedEntities))
		for _, id := range current.ProducedEntities {
			existing[id] = struct{}{}
		}
		for _, id := range producedIDs {
			if _, ok := existing[id]; !ok {
				current.ProducedEntities = append(current.ProducedEntities, id)
			}
		}
		current.ArtifactsIndexed = true

		if err := c.graph.EmitEntityCAS(ctx, current, domain.PredicateQuestArtifactsIndexed, revision); err != nil {
			c.logger.Warn("indexing watcher: CAS conflict on indexed write — retrying",
				"quest", questID, "attempt", attempt+1, "error", err)
			continue
		}

		c.logger.Info("quest artifacts indexed",
			"quest", questID,
			"produced_entities", len(current.ProducedEntities))
		return
	}

	c.logger.Error("indexing watcher: gave up writing ArtifactsIndexed after retries",
		"quest", questID, "max_retries", maxRetries)
}
