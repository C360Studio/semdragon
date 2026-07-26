package main

import "testing"

func TestExtractAgentIncludesCanonicalOptionalAggregates(t *testing.T) {
	schema := extractAgent()

	for _, predicate := range []string{
		"agent.inventory.total-spent",
		"agent.reputation.peer-avg",
		"agent.reputation.peer-q1-avg",
		"agent.reputation.peer-q2-avg",
		"agent.reputation.peer-q3-avg",
		"agent.reputation.peer-count",
	} {
		if !containsPredicate(schema.Optional, predicate) {
			t.Errorf("agent optional schema is missing %q", predicate)
		}
	}
	if !containsPredicate(schema.MultiValue, "agent.skill.proficiency") {
		t.Error(`agent multi-value schema is missing "agent.skill.proficiency"`)
	}
}

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
