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

func TestExtractPartyHasCanonicalScalarAndDynamicInventory(t *testing.T) {
	schema := extractParty()

	assertPredicatesEqual(t, "static", schema.Static, []string{
		"party.assignment.quest",
		"party.identity.name",
		"party.lifecycle.formed-at",
		"party.membership.count",
		"party.membership.lead",
		"party.status.state",
	})
	assertPredicatesEqual(t, "optional", schema.Optional, []string{
		"party.lifecycle.disbanded-at",
	})
	assertPredicatesEqual(t, "multi-value", schema.MultiValue, []string{
		"party.membership.member",
	})
	assertPredicatesEqual(t, "dynamic", schema.Dynamic, []string{
		"party.assignment.{id}",
		"party.member.{id}.role",
	})

	for _, predicate := range []string{
		"party.quest",
		"party.lead",
		"party.lifecycle.formed_at",
		"party.lifecycle.disbanded_at",
	} {
		for _, predicates := range [][]string{
			schema.Static,
			schema.Optional,
			schema.MultiValue,
			schema.Dynamic,
		} {
			if containsPredicate(predicates, predicate) {
				t.Errorf("party schema retains stale scalar %q", predicate)
			}
		}
	}
}

func assertPredicatesEqual(t *testing.T, category string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("unexpected %s predicates: got %v, want %v", category, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected %s predicates: got %v, want %v", category, got, want)
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
