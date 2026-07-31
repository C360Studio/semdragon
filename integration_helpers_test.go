//go:build integration

package semdragons_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
)

func shippedQuestRuleConfig(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile("config/semdragons.json")
	if err != nil {
		t.Fatalf("read shipped default profile: %v", err)
	}
	var profile runtimeProfileContract
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatalf("decode shipped default profile: %v", err)
	}
	rules, ok := profile.Components["rule-processor"]
	if !ok {
		t.Fatal("shipped default profile has no rule-processor")
	}
	return map[string]any{
		"rules_files":              rules.Config.RulesFiles,
		"pack_id":                  rules.Config.PackID,
		"enable_graph_integration": rules.Config.EnableGraphIntegration,
		"entity_watch_buckets":     rules.Config.EntityWatchBuckets,
	}
}

func startPhase1Component(
	t *testing.T,
	ctx context.Context,
	raw json.RawMessage,
	create func(json.RawMessage) (component.Discoverable, error),
) {
	t.Helper()
	discoverable, err := create(raw)
	if err != nil {
		t.Fatalf("create component: %v", err)
	}
	lifecycleComponent, ok := discoverable.(component.LifecycleComponent)
	if !ok {
		t.Fatalf("component %T is not a LifecycleComponent", discoverable)
	}
	if err := lifecycleComponent.Initialize(); err != nil {
		t.Fatalf("initialize component %T: %v", discoverable, err)
	}
	if err := lifecycleComponent.Start(ctx); err != nil {
		t.Fatalf("start component %T: %v", discoverable, err)
	}
	t.Cleanup(func() {
		if err := lifecycleComponent.Stop(5 * time.Second); err != nil {
			t.Errorf("stop component %T: %v", discoverable, err)
		}
	})
}

func mustPhase1JSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal integration config: %v", err)
	}
	return data
}
