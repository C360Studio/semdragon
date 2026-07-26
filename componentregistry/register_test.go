package componentregistry

import (
	"slices"
	"testing"

	"github.com/c360studio/semstreams/component"
	lifecyclegateway "github.com/c360studio/semstreams/gateway/lifecycle-gateway"
)

func TestRegisterAllIncludesLifecycleGateway(t *testing.T) {
	t.Parallel()

	registry := component.NewRegistry()
	if err := RegisterAll(registry); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}
	if _, ok := registry.ListFactories()[lifecyclegateway.ComponentName]; !ok {
		t.Fatalf("registered factories = %v, want %q", registry.ListFactories(), lifecyclegateway.ComponentName)
	}
	if !slices.Contains(ComponentNames(), lifecyclegateway.ComponentName) {
		t.Fatalf("ComponentNames() = %v, want %q", ComponentNames(), lifecyclegateway.ComponentName)
	}
}

func TestRegisterProcessorsIncludesLifecycleGateway(t *testing.T) {
	t.Parallel()

	registry := component.NewRegistry()
	if err := RegisterProcessors(registry); err != nil {
		t.Fatalf("RegisterProcessors: %v", err)
	}
	if _, ok := registry.ListFactories()[lifecyclegateway.ComponentName]; !ok {
		t.Fatalf("registered factories = %v, want %q", registry.ListFactories(), lifecyclegateway.ComponentName)
	}
	if !slices.Contains(ProcessorNames(), lifecyclegateway.ComponentName) {
		t.Fatalf("ProcessorNames() = %v, want %q", ProcessorNames(), lifecyclegateway.ComponentName)
	}
}
