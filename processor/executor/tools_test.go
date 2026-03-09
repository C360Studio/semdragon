package executor

import (
	"testing"

	"github.com/c360studio/semdragons/domain"
)

// TestBuiltinToolTierAlignment verifies that each tool registered by RegisterBuiltins
// enforces the trust tier documented in the trust tier table.
//
// Tier intent by tool category:
//
//	Apprentice (1-5) — read-only operations safe for any agent
//	Journeyman (6-10) — staging writes and external API access
//	Expert    (11-15) — production file writes, test execution
//	Master    (16-18) — unrestricted shell, party lead operations
func TestBuiltinToolTierAlignment(t *testing.T) {
	t.Parallel()

	cases := []struct {
		tool     string
		wantTier domain.TrustTier
		reason   string
	}{
		// Apprentice — read-only; safe for every agent regardless of level.
		{tool: "read_file", wantTier: domain.TierApprentice, reason: "read-only file access"},
		{tool: "list_directory", wantTier: domain.TierApprentice, reason: "read-only directory listing"},
		{tool: "search_text", wantTier: domain.TierApprentice, reason: "read-only text search"},

		// Journeyman — targeted writes and network access require demonstrated trust.
		{tool: "patch_file", wantTier: domain.TierJourneyman, reason: "targeted file edits require level 6+"},
		{tool: "http_request", wantTier: domain.TierJourneyman, reason: "network access requires level 6+"},

		// Expert — production-grade writes and test execution require level 11+.
		{tool: "write_file", wantTier: domain.TierExpert, reason: "full file write is a production capability"},
		{tool: "run_tests", wantTier: domain.TierExpert, reason: "test execution is a production capability"},

		// Master — unrestricted shell and party-lead DAG operations require level 16+.
		{tool: "run_command", wantTier: domain.TierMaster, reason: "unrestricted shell requires high trust"},
		{tool: "decompose_quest", wantTier: domain.TierMaster, reason: "only party leads (Master+) can decompose quests"},
		{tool: "review_sub_quest", wantTier: domain.TierMaster, reason: "only party leads (Master+) can review sub-quests"},
	}

	reg := NewToolRegistry()
	reg.RegisterBuiltins()

	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			t.Parallel()

			tool := reg.Get(tc.tool)
			if tool == nil {
				t.Fatalf("tool %q not found in registry after RegisterBuiltins", tc.tool)
			}

			if tool.MinTier != tc.wantTier {
				t.Errorf(
					"tool %q MinTier = %s (%d), want %s (%d): %s",
					tc.tool,
					tool.MinTier, tool.MinTier,
					tc.wantTier, tc.wantTier,
					tc.reason,
				)
			}
		})
	}
}

// TestBuiltinToolCount asserts that the total number of tools registered by
// RegisterBuiltins matches the expected count. A mismatch here means a tool
// was added (or removed) from RegisterBuiltins without updating
// TestBuiltinToolTierAlignment — update both together.
func TestBuiltinToolCount(t *testing.T) {
	t.Parallel()

	// RegisterBuiltins registers:
	//   read_file, write_file, list_directory, search_text, patch_file,
	//   http_request, run_tests, run_command           — 8 core tools
	//   decompose_quest                                 — 1 DAG lead tool
	//   review_sub_quest                               — 1 DAG review tool
	//
	// graph_query is intentionally excluded — it requires a live EntityQueryFunc
	// and is registered separately via RegisterGraphQuery.
	const wantCount = 10

	reg := NewToolRegistry()
	reg.RegisterBuiltins()

	got := len(reg.ListAll())
	if got != wantCount {
		t.Errorf(
			"RegisterBuiltins registered %d tools, want %d; "+
				"update TestBuiltinToolTierAlignment to cover any new tools",
			got, wantCount,
		)
	}
}
