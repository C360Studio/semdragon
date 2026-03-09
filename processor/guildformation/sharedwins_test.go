package guildformation

import (
	"testing"

	"github.com/c360studio/semdragons/domain"
)

// TestCanonicalPairKey_Ordering verifies that swapping the argument order
// produces the same key, ensuring symmetric pair lookups.
func TestCanonicalPairKey_Ordering(t *testing.T) {
	a := domain.AgentID("agent-alpha")
	b := domain.AgentID("agent-beta")

	keyAB := canonicalPairKey(a, b)
	keyBA := canonicalPairKey(b, a)

	if keyAB != keyBA {
		t.Errorf("expected symmetric keys: got %q and %q", keyAB, keyBA)
	}
}

// TestCanonicalPairKey_SameAgent checks that identical agents produce a
// stable key (edge case: same ID on both sides).
func TestCanonicalPairKey_SameAgent(t *testing.T) {
	a := domain.AgentID("agent-solo")
	key1 := canonicalPairKey(a, a)
	key2 := canonicalPairKey(a, a)
	if key1 != key2 {
		t.Errorf("expected identical keys for same agent, got %q and %q", key1, key2)
	}
}

// TestSharedWinsCache_RecordAndQuery records a win for a pair and verifies the
// count increments correctly on repeated calls.
func TestSharedWinsCache_RecordAndQuery(t *testing.T) {
	c := newSharedWinsCache()
	a := domain.AgentID("agent-a")
	b := domain.AgentID("agent-b")

	members := []domain.AgentID{a, b}
	c.RecordPartyWin(members)
	c.RecordPartyWin(members)

	got := c.SharedWins(a, b)
	if got != 2 {
		t.Errorf("expected 2 shared wins, got %d", got)
	}

	// Symmetric lookup must return the same value.
	gotSwapped := c.SharedWins(b, a)
	if gotSwapped != got {
		t.Errorf("expected symmetric result %d, got %d", got, gotSwapped)
	}
}

// TestSharedWinsCache_ZeroWins confirms that querying an unrecorded pair
// returns 0 rather than panicking or returning an error.
func TestSharedWinsCache_ZeroWins(t *testing.T) {
	c := newSharedWinsCache()
	a := domain.AgentID("agent-x")
	b := domain.AgentID("agent-y")

	if got := c.SharedWins(a, b); got != 0 {
		t.Errorf("expected 0 for unknown pair, got %d", got)
	}
}

// TestSharedWinsCache_PartyOfThree verifies that a three-member party records
// exactly three distinct pairs (AB, AC, BC) with one win each.
func TestSharedWinsCache_PartyOfThree(t *testing.T) {
	c := newSharedWinsCache()
	a := domain.AgentID("agent-a")
	b := domain.AgentID("agent-b")
	d := domain.AgentID("agent-d")

	c.RecordPartyWin([]domain.AgentID{a, b, d})

	pairs := [][2]domain.AgentID{{a, b}, {a, d}, {b, d}}
	for _, pair := range pairs {
		got := c.SharedWins(pair[0], pair[1])
		if got != 1 {
			t.Errorf("pair (%s, %s): expected 1 win, got %d", pair[0], pair[1], got)
		}
	}
}

// TestSharedWinScore_Curve checks every segment of the scoring step function.
func TestSharedWinScore_Curve(t *testing.T) {
	cases := []struct {
		wins     int
		expected float64
	}{
		{-1, 0.0},
		{0, 0.0},
		{1, 0.6},
		{2, 0.8},
		{3, 0.8},
		{4, 1.0},
		{10, 1.0},
	}
	for _, tc := range cases {
		got := SharedWinScore(tc.wins)
		if got != tc.expected {
			t.Errorf("SharedWinScore(%d) = %f, want %f", tc.wins, got, tc.expected)
		}
	}
}

// TestPairwisePeerScore_NoData confirms that querying a pair with no reviews
// returns (0, false).
func TestPairwisePeerScore_NoData(t *testing.T) {
	c := newSharedWinsCache()
	a := domain.AgentID("agent-a")
	b := domain.AgentID("agent-b")

	score, ok := c.PairwisePeerScore(a, b)
	if ok {
		t.Errorf("expected ok=false for empty cache, got ok=true with score=%f", score)
	}
	if score != 0 {
		t.Errorf("expected score=0 for empty cache, got %f", score)
	}
}

// TestPairwisePeerScore_MutualReviews records two reviews in opposite
// directions and verifies the normalized average is correct.
//
//	a → b: 5.0   (normalized: 1.0)
//	b → a: 3.0   (normalized: 0.5)
//	raw avg: 4.0 → normalized: (4-1)/4 = 0.75
func TestPairwisePeerScore_MutualReviews(t *testing.T) {
	c := newSharedWinsCache()
	a := domain.AgentID("agent-a")
	b := domain.AgentID("agent-b")

	c.RecordPeerReview(a, b, 5.0)
	c.RecordPeerReview(b, a, 3.0)

	score, ok := c.PairwisePeerScore(a, b)
	if !ok {
		t.Fatal("expected ok=true after recording two reviews")
	}

	const want = 0.75
	const epsilon = 0.001
	if diff := score - want; diff > epsilon || diff < -epsilon {
		t.Errorf("PairwisePeerScore = %f, want %f (±%f)", score, want, epsilon)
	}
}

// TestPairwisePeerScore_SingleReview verifies normalization for a single
// review: a rating of 3.0 on the 1–5 scale → (3-1)/4 = 0.5.
func TestPairwisePeerScore_SingleReview(t *testing.T) {
	c := newSharedWinsCache()
	a := domain.AgentID("agent-a")
	b := domain.AgentID("agent-b")

	c.RecordPeerReview(a, b, 3.0)

	score, ok := c.PairwisePeerScore(a, b)
	if !ok {
		t.Fatal("expected ok=true after recording a review")
	}

	const want = 0.5
	const epsilon = 0.001
	if diff := score - want; diff > epsilon || diff < -epsilon {
		t.Errorf("PairwisePeerScore = %f, want %f (±%f)", score, want, epsilon)
	}
}
