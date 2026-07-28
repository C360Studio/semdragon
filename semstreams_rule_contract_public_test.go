package semdragons_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/pkg/ownership"
	"github.com/c360studio/semstreams/pkg/projection"
	rule "github.com/c360studio/semstreams/processor/rule"
	"github.com/c360studio/semstreams/vocabulary"
)

const (
	downstreamBootPredicate    = "semdragon.cutover.boot"
	downstreamReservePredicate = "semdragon.cutover.reserve"
	downstreamOutsidePredicate = "semdragon.cutover.outside"
	downstreamDynamicPredicate = "semdragon.cutover.dynamic"
	downstreamEntityPattern    = "acme.ops.game.board.quest.*"
	downstreamContractName     = "downstream-quest"
)

func TestSemStreamsPublicProjectionContractPresence(t *testing.T) {
	t.Run("omission remains distinguishable from explicit empty", func(t *testing.T) {
		var omitted rule.Config
		if err := json.Unmarshal([]byte(`{"pack_id":"omitted"}`), &omitted); err != nil {
			t.Fatalf("decode omitted contracts: %v", err)
		}
		if omitted.ProjectionContracts != nil {
			t.Fatalf("omitted projection_contracts = %#v, want nil", omitted.ProjectionContracts)
		}

		var explicitEmpty rule.Config
		if err := json.Unmarshal(
			[]byte(`{"pack_id":"explicit-empty","projection_contracts":[]}`),
			&explicitEmpty,
		); err != nil {
			t.Fatalf("decode explicit empty contracts: %v", err)
		}
		if explicitEmpty.ProjectionContracts == nil || len(explicitEmpty.ProjectionContracts) != 0 {
			t.Fatalf("explicit empty projection_contracts = %#v, want non-nil empty",
				explicitEmpty.ProjectionContracts)
		}
	})

	t.Run("null is rejected at the public config boundary", func(t *testing.T) {
		var config rule.Config
		err := json.Unmarshal(
			[]byte(`{"pack_id":"null-contract","projection_contracts":null}`),
			&config,
		)
		if err == nil || !strings.Contains(err.Error(), "must be an array, not null") {
			t.Fatalf("null projection_contracts error = %v", err)
		}
	})
}

func TestSemStreamsPublicDerivationAndExplicitEnvelope(t *testing.T) {
	registerDownstreamRulePredicates()
	definitions := []rule.Definition{{
		ID:      "derive-minimal",
		Type:    "test_rule",
		Name:    "derive minimal authority",
		Enabled: false,
		Entity:  rule.EntityConfig{Pattern: downstreamEntityPattern},
		OnEnter: []rule.Action{
			{
				Type:               rule.ActionTypeReplaceOwned,
				ProjectionContract: downstreamContractName,
				ProjectionGroup:    "phase",
				Predicate:          downstreamBootPredicate,
			},
			{
				Type:               rule.ActionTypeReplaceOwned,
				ProjectionContract: downstreamContractName,
				ProjectionGroup:    "assignment",
				Predicate:          downstreamReservePredicate,
			},
		},
	}}

	omitted := newDownstreamRuleConfig(t, "derived-minimal", definitions)
	processor, err := rule.NewProcessor(nil, &omitted)
	if err != nil {
		t.Fatalf("construct derived processor: %v", err)
	}
	if err := processor.PreflightProjectionMutations(); err != nil {
		t.Fatalf("preflight derived processor: %v", err)
	}
	packID, contracts := processor.ProjectionBindings()
	if packID != "derived-minimal" {
		t.Errorf("pack_id = %q, want derived-minimal", packID)
	}
	wantDerived := []projection.Contract{{
		Name:          downstreamContractName,
		EntityPattern: downstreamEntityPattern,
		Groups: []projection.PredicateGroup{
			{
				Name:       "assignment",
				Mode:       ownership.ModeReplaceOwned,
				Predicates: []string{downstreamReservePredicate},
			},
			{
				Name:       "phase",
				Mode:       ownership.ModeReplaceOwned,
				Predicates: []string{downstreamBootPredicate},
			},
		},
	}}
	if !reflect.DeepEqual(contracts, wantDerived) {
		t.Fatalf("derived contracts = %#v, want %#v", contracts, wantDerived)
	}

	explicitEmpty := newDownstreamRuleConfig(t, "explicit-empty", definitions)
	if err := json.Unmarshal([]byte(`{"projection_contracts":[]}`), &explicitEmpty); err != nil {
		t.Fatalf("overlay explicit-empty projection_contracts: %v", err)
	}
	emptyProcessor, err := rule.NewProcessor(nil, &explicitEmpty)
	if err != nil {
		t.Fatalf("construct explicit-empty processor: %v", err)
	}
	if err := emptyProcessor.PreflightProjectionMutations(); err == nil ||
		!strings.Contains(err.Error(), "missing derived contract") {
		t.Fatalf("explicit-empty preflight error = %v", err)
	}

	explicit := newDownstreamRuleConfig(t, "explicit-envelope", definitions)
	explicit.ProjectionContracts = []projection.Contract{{
		Name:          downstreamContractName,
		EntityPattern: "acme.*.game.board.quest.*",
		Groups: []projection.PredicateGroup{
			{
				Name:       "assignment",
				Mode:       ownership.ModeReplaceOwned,
				Predicates: []string{downstreamReservePredicate, downstreamOutsidePredicate},
			},
			{
				Name:       "phase",
				Mode:       ownership.ModeReplaceOwned,
				Predicates: []string{downstreamBootPredicate},
			},
		},
	}}
	explicitProcessor, err := rule.NewProcessor(nil, &explicit)
	if err != nil {
		t.Fatalf("construct explicit processor: %v", err)
	}
	if err := explicitProcessor.PreflightProjectionMutations(); err != nil {
		t.Fatalf("preflight explicit processor: %v", err)
	}
	_, explicitBindings := explicitProcessor.ProjectionBindings()
	if !reflect.DeepEqual(explicitBindings, explicit.ProjectionContracts) {
		t.Fatalf("explicit bindings = %#v, want authored envelope %#v",
			explicitBindings, explicit.ProjectionContracts)
	}
}

func TestSemStreamsPublicDynamicTargetsAndHotReloadEnvelope(t *testing.T) {
	registerDownstreamRulePredicates()
	dynamic := rule.Definition{
		ID:      "dynamic-target",
		Type:    "test_rule",
		Name:    "dynamic target",
		Enabled: false,
		OnEnter: []rule.Action{{
			Type:               rule.ActionTypeReplaceOwned,
			ProjectionContract: downstreamContractName,
			ProjectionGroup:    "dynamic",
			Predicate:          downstreamDynamicPredicate,
			Subject:            "$entity.triple.quest.parent.id",
		}},
	}

	withoutEnvelope := newDownstreamRuleConfig(t, "dynamic-without-envelope", []rule.Definition{dynamic})
	processor, err := rule.NewProcessor(nil, &withoutEnvelope)
	if err != nil {
		t.Fatalf("construct dynamic processor: %v", err)
	}
	if err := processor.PreflightProjectionMutations(); err == nil ||
		!strings.Contains(err.Error(), "requires an explicit projection_contracts envelope") {
		t.Fatalf("dynamic target preflight error = %v", err)
	}

	withEnvelope := newDownstreamRuleConfig(t, "dynamic-with-envelope", []rule.Definition{dynamic})
	withEnvelope.ProjectionContracts = []projection.Contract{{
		Name:          downstreamContractName,
		EntityPattern: downstreamEntityPattern,
		Groups: []projection.PredicateGroup{{
			Name:       "dynamic",
			Mode:       ownership.ModeReplaceOwned,
			Predicates: []string{downstreamDynamicPredicate},
		}},
	}}
	enveloped, err := rule.NewProcessor(nil, &withEnvelope)
	if err != nil {
		t.Fatalf("construct enveloped dynamic processor: %v", err)
	}
	if err := enveloped.PreflightProjectionMutations(); err != nil {
		t.Fatalf("preflight enveloped dynamic processor: %v", err)
	}

	initial := rule.Definition{
		ID:      "initial",
		Type:    "test_rule",
		Name:    "initial",
		Enabled: false,
		Entity:  rule.EntityConfig{Pattern: downstreamEntityPattern},
		OnEnter: []rule.Action{{
			Type:               rule.ActionTypeReplaceOwned,
			ProjectionContract: downstreamContractName,
			ProjectionGroup:    "phase",
			Predicate:          downstreamBootPredicate,
		}},
	}
	hotRule := runtimeReplaceOwnedRule("reserve", downstreamReservePredicate)
	outsideRule := runtimeReplaceOwnedRule("reserve", downstreamOutsidePredicate)

	minimal := newDownstreamRuleConfig(t, "minimal-hot-reload", []rule.Definition{initial})
	minimalProcessor, err := rule.NewProcessor(nil, &minimal)
	if err != nil {
		t.Fatalf("construct minimal hot-reload processor: %v", err)
	}
	if err := minimalProcessor.PreflightProjectionMutations(); err != nil {
		t.Fatalf("preflight minimal hot-reload processor: %v", err)
	}
	if err := minimalProcessor.ValidateConfigUpdate(
		map[string]any{"rules": map[string]any{"hot": hotRule}},
	); err == nil || !strings.Contains(err.Error(), "no named projection group") {
		t.Fatalf("minimal hot-reload error = %v", err)
	}

	reserved := newDownstreamRuleConfig(t, "reserved-hot-reload", []rule.Definition{initial})
	reserved.ProjectionContracts = []projection.Contract{{
		Name:          downstreamContractName,
		EntityPattern: downstreamEntityPattern,
		Groups: []projection.PredicateGroup{
			{
				Name:       "phase",
				Mode:       ownership.ModeReplaceOwned,
				Predicates: []string{downstreamBootPredicate},
			},
			{
				Name:       "reserve",
				Mode:       ownership.ModeReplaceOwned,
				Predicates: []string{downstreamReservePredicate},
			},
		},
	}}
	reservedProcessor, err := rule.NewProcessor(nil, &reserved)
	if err != nil {
		t.Fatalf("construct reserved hot-reload processor: %v", err)
	}
	if err := reservedProcessor.PreflightProjectionMutations(); err != nil {
		t.Fatalf("preflight reserved hot-reload processor: %v", err)
	}
	_, before := reservedProcessor.ProjectionBindings()
	validUpdate := map[string]any{"rules": map[string]any{"hot": hotRule}}
	if err := reservedProcessor.ValidateConfigUpdate(validUpdate); err != nil {
		t.Fatalf("reserved hot-reload rejected: %v", err)
	}
	if err := reservedProcessor.ApplyConfigUpdate(validUpdate); err != nil {
		t.Fatalf("reserved hot-reload apply failed: %v", err)
	}
	invalidUpdate := map[string]any{"rules": map[string]any{"outside": outsideRule}}
	if err := reservedProcessor.ValidateConfigUpdate(invalidUpdate); err == nil ||
		!strings.Contains(err.Error(), "outside projection contract") {
		t.Fatalf("out-of-envelope hot-reload error = %v", err)
	}
	_, after := reservedProcessor.ProjectionBindings()
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("hot reload changed frozen projection bindings: before=%#v after=%#v", before, after)
	}
}

func newDownstreamRuleConfig(
	t *testing.T,
	packID string,
	definitions []rule.Definition,
) rule.Config {
	t.Helper()
	config, err := rule.NewConfig(packID)
	if err != nil {
		t.Fatalf("new rule config: %v", err)
	}
	config.InlineRules = definitions
	return config
}

func runtimeReplaceOwnedRule(group, predicate string) map[string]any {
	return map[string]any{
		"type": "test_rule",
		"on_enter": []any{map[string]any{
			"type":                rule.ActionTypeReplaceOwned,
			"projection_contract": downstreamContractName,
			"projection_group":    group,
			"predicate":           predicate,
		}},
	}
}

func registerDownstreamRulePredicates() {
	for _, predicate := range []string{
		downstreamBootPredicate,
		downstreamReservePredicate,
		downstreamOutsidePredicate,
		downstreamDynamicPredicate,
	} {
		vocabulary.Register(predicate)
	}
}
