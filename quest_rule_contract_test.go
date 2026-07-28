package semdragons_test

import (
	"encoding/json"
	"os"
	"testing"
)

type questRuleCondition struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    any    `json:"value"`
}

type questRuleDefinition struct {
	ID         string               `json:"id"`
	Conditions []questRuleCondition `json:"conditions"`
}

func TestQuestFailureRulesCompareAttemptsAgainstEntityMax(t *testing.T) {
	data, err := os.ReadFile("config/rules/quest/03-fail.json")
	if err != nil {
		t.Fatalf("read fail rules: %v", err)
	}

	var definitions []questRuleDefinition
	if err := json.Unmarshal(data, &definitions); err != nil {
		t.Fatalf("decode fail rules: %v", err)
	}

	const maxAttemptsValue = "$entity.triple.quest.attempts.max.value"
	wantOperators := map[string]string{
		"quest_fail_retry":  "lt",
		"quest_fail_triage": "gte",
	}
	for _, definition := range definitions {
		wantOperator, required := wantOperators[definition.ID]
		if !required {
			continue
		}
		var found bool
		for _, condition := range definition.Conditions {
			if condition.Field != "quest.attempts.current" {
				continue
			}
			found = true
			if condition.Operator != wantOperator {
				t.Errorf("%s operator = %q, want %q", definition.ID, condition.Operator, wantOperator)
			}
			if condition.Value != maxAttemptsValue {
				t.Errorf("%s value = %#v, want %q", definition.ID, condition.Value, maxAttemptsValue)
			}
		}
		if !found {
			t.Errorf("%s has no quest.attempts.current condition", definition.ID)
		}
		delete(wantOperators, definition.ID)
	}
	for missing := range wantOperators {
		t.Errorf("missing failure rule %s", missing)
	}
}
