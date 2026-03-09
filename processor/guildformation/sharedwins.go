package guildformation

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"

	"github.com/c360studio/semdragons/domain"
)

// pairScore tracks the running sum and count of ratings between two agents so
// we can compute a live average without re-scanning all historical reviews.
type pairScore struct {
	sum   atomic.Int64 // sum stored as fixed-point: actual_sum * 1000
	count atomic.Int64
}

// add records one raw rating (1–5 scale).
func (p *pairScore) add(rating float64) {
	// Store as integer fixed-point (×1000) to allow atomic updates.
	// Use math.Round to avoid truncation bias with fractional ratings.
	p.sum.Add(int64(math.Round(rating * 1000)))
	p.count.Add(1)
}

// average returns the mean rating on the 1–5 scale, or 0 if no data.
// Note: sum and count are read non-atomically; a concurrent add() may produce
// a transiently stale average, which is acceptable for scoring purposes.
func (p *pairScore) average() (float64, bool) {
	c := p.count.Load()
	if c == 0 {
		return 0, false
	}
	return float64(p.sum.Load()) / (float64(c) * 1000), true
}

// sharedWinsCache is a thread-safe store that tracks:
//   - how many quests two agents completed together in the same party
//   - the pairwise average of mutual peer review ratings between agents
//
// Both maps are keyed by a canonical pair key (see canonicalPairKey) so that
// the pair (A, B) and (B, A) always resolve to the same entry.
//
// sync.Map is chosen because the cache is read-heavy: many scoring lookups
// happen concurrently during guild formation, while writes are infrequent.
type sharedWinsCache struct {
	// wins stores *atomic.Int64 values (win count per pair).
	wins sync.Map

	// pairScores stores *pairScore values (sum+count per pair).
	pairScores sync.Map
}

// newSharedWinsCache constructs an empty sharedWinsCache.
func newSharedWinsCache() *sharedWinsCache {
	return &sharedWinsCache{}
}

// canonicalPairKey returns a deterministic key for the unordered pair {a, b}.
// The smaller ID (lexicographic) always comes first, ensuring A,B == B,A.
func canonicalPairKey(a, b domain.AgentID) string {
	if string(a) <= string(b) {
		return fmt.Sprintf("%s|%s", a, b)
	}
	return fmt.Sprintf("%s|%s", b, a)
}

// RecordPartyWin increments the shared-win counter for every unique pair in
// the member list. For a party of N members this records N*(N-1)/2 pairs.
func (c *sharedWinsCache) RecordPartyWin(members []domain.AgentID) {
	for i := range len(members) {
		for j := i + 1; j < len(members); j++ {
			key := canonicalPairKey(members[i], members[j])
			actual, _ := c.wins.LoadOrStore(key, &atomic.Int64{})
			actual.(*atomic.Int64).Add(1)
		}
	}
}

// SharedWins returns how many quests the pair {a, b} has completed together.
// Returns 0 if no shared wins have been recorded.
func (c *sharedWinsCache) SharedWins(a, b domain.AgentID) int {
	key := canonicalPairKey(a, b)
	v, ok := c.wins.Load(key)
	if !ok {
		return 0
	}
	return int(v.(*atomic.Int64).Load())
}

// RecordPeerReview records a raw review rating (1–5 scale) from agent a toward
// agent b. The pairwise score is the running average of all ratings in either
// direction between the two agents; directionality is intentionally collapsed
// so that mutual positive reviews compound into a single cohesion signal.
func (c *sharedWinsCache) RecordPeerReview(a, b domain.AgentID, rating float64) {
	if a == b {
		return // Self-reviews are not pairwise signals.
	}
	key := canonicalPairKey(a, b)
	actual, _ := c.pairScores.LoadOrStore(key, &pairScore{})
	actual.(*pairScore).add(rating)
}

// PairwisePeerScore returns the average peer review rating between agents a
// and b, normalized to a 0.0–1.0 range from the raw 1–5 scale.
// Returns (0, false) when no reviews have been recorded for the pair.
func (c *sharedWinsCache) PairwisePeerScore(a, b domain.AgentID) (float64, bool) {
	key := canonicalPairKey(a, b)
	v, ok := c.pairScores.Load(key)
	if !ok {
		return 0, false
	}
	avg, ok := v.(*pairScore).average()
	if !ok {
		return 0, false
	}
	// Normalize from [1, 5] → [0.0, 1.0].
	normalized := (avg - 1) / 4
	return normalized, true
}

// SharedWinScore converts a raw win count into a bounded [0, 1] score using a
// stepped curve that rewards early shared wins strongly and plateaus at 4+.
//
//	0 wins  → 0.0  (no signal)
//	1 win   → 0.6  (meaningful cohesion)
//	2–3     → 0.8  (established partnership)
//	4+      → 1.0  (proven team)
func SharedWinScore(wins int) float64 {
	switch {
	case wins <= 0:
		return 0.0
	case wins == 1:
		return 0.6
	case wins <= 3:
		return 0.8
	default:
		return 1.0
	}
}
