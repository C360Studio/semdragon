package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	lifecyclegateway "github.com/c360studio/semstreams/gateway/lifecycle-gateway"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semdragons/questlifecycle"
)

func TestProductionLifecycleWorkflowAllowlistIsQuestOnly(t *testing.T) {
	t.Parallel()

	manager, err := createLifecycleManager(
		nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatalf("create production lifecycle manager: %v", err)
	}
	workflows := manager.ListWorkflows()
	if len(workflows) != 1 || workflows[0].Workflow != questlifecycle.WorkflowName {
		t.Fatalf("production workflow allowlist = %#v, want only quest", workflows)
	}
}

func TestShippedProfilesEnableLifecycleGateway(t *testing.T) {
	t.Parallel()

	profiles := []string{
		"semdragons.json",
		"semdragons-semsource.json",
		"semdragons-e2e-tier1.json",
	}
	for _, profile := range profiles {
		profile := profile
		t.Run(profile, func(t *testing.T) {
			t.Parallel()

			cfg, _, err := loadConfig(filepath.Join("..", "..", "config", profile), "")
			if err != nil {
				t.Fatalf("load profile: %v", err)
			}
			componentConfig, ok := cfg.Components[lifecyclegateway.ComponentName]
			if !ok {
				t.Fatalf("components.%s is missing", lifecyclegateway.ComponentName)
			}
			if componentConfig.Type != types.ComponentTypeGateway {
				t.Errorf("type = %q, want %q", componentConfig.Type, types.ComponentTypeGateway)
			}
			if componentConfig.Name != lifecyclegateway.ComponentName {
				t.Errorf("name = %q, want %q", componentConfig.Name, lifecyclegateway.ComponentName)
			}
			if !componentConfig.Enabled {
				t.Error("gateway is disabled")
			}

			var gatewayConfig lifecyclegateway.Config
			if err := json.Unmarshal(componentConfig.Config, &gatewayConfig); err != nil {
				t.Fatalf("decode gateway config: %v", err)
			}
			if gatewayConfig.PathPrefix != "workflows" {
				t.Errorf("path_prefix = %q, want workflows", gatewayConfig.PathPrefix)
			}
			if gatewayConfig.EnableWebSocket == nil || *gatewayConfig.EnableWebSocket {
				t.Errorf("enable_websocket = %v, want false", gatewayConfig.EnableWebSocket)
			}
			if gatewayConfig.MaxBodyBytes != 65536 {
				t.Errorf("max_body_bytes = %d, want 65536", gatewayConfig.MaxBodyBytes)
			}
		})
	}
}
