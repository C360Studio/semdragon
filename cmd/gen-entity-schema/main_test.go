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

func TestExtractBattleIncludesCanonicalScalars(t *testing.T) {
	schema := extractBattle()
	all := append(append(append(
		[]string{},
		schema.Static...),
		schema.Optional...),
		schema.MultiValue...)

	for _, predicate := range []string{
		"battle.lifecycle.started-at",
		"battle.execution.loop-id",
		"battle.verdict.quality-score",
		"battle.verdict.xp-awarded",
		"battle.verdict.xp-penalty",
		"battle.verdict.level-change",
		"battle.lifecycle.completed-at",
	} {
		if !containsPredicate(all, predicate) {
			t.Errorf("battle schema is missing canonical scalar %q", predicate)
		}
	}

	for _, predicate := range []string{
		"battle.lifecycle.started_at",
		"battle.execution.loop_id",
		"battle.verdict.quality_score",
		"battle.verdict.xp_awarded",
		"battle.verdict.xp_penalty",
		"battle.verdict.level_change",
		"battle.lifecycle.completed_at",
	} {
		if containsPredicate(all, predicate) {
			t.Errorf("battle schema retains stale scalar %q", predicate)
		}
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
