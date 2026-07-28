package semdragons_test

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/component"
	semconfig "github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/natsclient"
	rule "github.com/c360studio/semstreams/processor/rule"
	"github.com/c360studio/semstreams/service"
	"github.com/c360studio/semstreams/types"
)

func TestSemStreamsPublicComponentManagerRulePackFailurePolicy(t *testing.T) {
	registerDownstreamRulePredicates()

	t.Run("enabled invalid rule pack is boot fatal", func(t *testing.T) {
		invalid := downstreamRuleComponentConfig(t, "enabled-invalid", []rule.Definition{{
			ID:      "dynamic-without-envelope",
			Type:    "test_rule",
			Name:    "dynamic without envelope",
			Enabled: false,
			OnEnter: []rule.Action{{
				Type:               rule.ActionTypeReplaceOwned,
				ProjectionContract: downstreamContractName,
				ProjectionGroup:    "dynamic",
				Predicate:          downstreamDynamicPredicate,
				Subject:            "$entity.triple.quest.parent.id",
			}},
		}})
		_, err := newDownstreamComponentManager(t, semconfig.ComponentConfigs{
			"enabled-invalid": invalid,
		})
		if err == nil {
			t.Fatal("enabled invalid rule pack boot succeeded")
		}
		for _, fragment := range []string{
			"enabled rule-processor cold-boot initialization failed",
			"enabled-invalid",
			"requires an explicit projection_contracts envelope",
		} {
			if !strings.Contains(err.Error(), fragment) {
				t.Errorf("boot error %q missing %q", err, fragment)
			}
		}
	})

	t.Run("disabled invalid is excluded and ordinary failure is isolated", func(t *testing.T) {
		valid := downstreamRuleComponentConfig(t, "valid-pack", nil)
		disabledInvalid := types.ComponentConfig{
			Type:    types.ComponentTypeProcessor,
			Name:    "rule-processor",
			Enabled: false,
			Config:  json.RawMessage(`{"pack_id":"bad:pack"}`),
		}
		ordinaryFailure := types.ComponentConfig{
			Type:    types.ComponentTypeProcessor,
			Name:    "ordinary-failing",
			Enabled: true,
		}

		manager, err := newDownstreamComponentManager(t, semconfig.ComponentConfigs{
			"valid-pack":       valid,
			"disabled-invalid": disabledInvalid,
			"ordinary-failure": ordinaryFailure,
		})
		if err != nil {
			t.Fatalf("best-effort boot failed: %v", err)
		}
		if !manager.IsInitialized() {
			t.Fatal("component manager was not initialized")
		}
		components := manager.ListComponents()
		if _, exists := components["valid-pack"]; !exists {
			t.Fatalf("valid rule pack missing from components: %v", components)
		}
		for _, excluded := range []string{"disabled-invalid", "ordinary-failure"} {
			if _, exists := components[excluded]; exists {
				t.Errorf("excluded component %q was created", excluded)
			}
		}
	})
}

func downstreamRuleComponentConfig(
	t *testing.T,
	packID string,
	definitions []rule.Definition,
) types.ComponentConfig {
	t.Helper()
	config := newDownstreamRuleConfig(t, packID, definitions)
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal rule component config: %v", err)
	}
	return types.ComponentConfig{
		Type:    types.ComponentTypeProcessor,
		Name:    "rule-processor",
		Enabled: true,
		Config:  raw,
	}
}

func newDownstreamComponentManager(
	t *testing.T,
	components semconfig.ComponentConfigs,
) (*service.ComponentManager, error) {
	t.Helper()
	testClient := natsclient.NewTestClient(t, natsclient.WithKV())

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	configManager, err := semconfig.NewConfigManager(
		&semconfig.Config{Components: components},
		testClient.Client,
		logger,
	)
	if err != nil {
		t.Fatalf("create config manager: %v", err)
	}

	registry := component.NewRegistry()
	if err := rule.Register(registry); err != nil {
		t.Fatalf("register rule processor: %v", err)
	}
	if err := registry.RegisterFactory("ordinary-failing", &component.Registration{
		Name: "ordinary-failing",
		Type: "processor",
		Factory: func(json.RawMessage, component.Dependencies) (component.Discoverable, error) {
			return nil, errors.New("ordinary component failure")
		},
	}); err != nil {
		t.Fatalf("register ordinary failing component: %v", err)
	}

	created, err := service.NewComponentManager(
		json.RawMessage(`{"watch_config":false}`),
		&service.Dependencies{
			NATSClient:        testClient.Client,
			Manager:           configManager,
			Logger:            logger,
			ComponentRegistry: registry,
		},
	)
	if err != nil {
		return nil, err
	}
	manager, ok := created.(*service.ComponentManager)
	if !ok {
		t.Fatalf("component manager type = %T", created)
	}
	return manager, nil
}
