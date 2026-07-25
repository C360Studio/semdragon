package main

import "testing"

func TestExtractQuestIncludesCanonicalMetrics(t *testing.T) {
	schema := extractQuest()

	for _, predicate := range []string{
		"quest.metrics.turns-used",
		"quest.metrics.tokens-prompt",
		"quest.metrics.tokens-completion",
	} {
		if !containsPredicate(schema.Optional, predicate) {
			t.Errorf("quest optional schema is missing %q", predicate)
		}
	}
}

func TestExtractGuildIncludesQuorumSize(t *testing.T) {
	schema := extractGuild()

	if !containsPredicate(schema.Optional, "guild.quorum.size") {
		t.Error(`guild optional schema is missing "guild.quorum.size"`)
	}
}

func containsPredicate(predicates []string, want string) bool {
	for _, predicate := range predicates {
		if predicate == want {
			return true
		}
	}
	return false
}
