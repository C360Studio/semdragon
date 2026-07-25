package semdragons_test

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/component"
	semconfig "github.com/c360studio/semstreams/config"
	semservice "github.com/c360studio/semstreams/service"

	"github.com/c360studio/semdragons/processor/agentstore"
)

var removedAgentStorePayloads = []string{
	"StoreItemListedPayload",
	"StorePurchasePayload",
	"ConsumableUsedPayload",
	"ConsumableExpiredPayload",
	"InventoryUpdatedPayload",
}

var removedAgentStoreSubjects = []string{
	"SubjectStoreItemListed",
	"SubjectStoreItemPurchased",
	"SubjectConsumableUsed",
	"SubjectConsumableExpired",
	"SubjectInventoryUpdated",
}

var removedAgentStoreEventPredicates = []string{
	"PredicateStoreItemListed",
	"PredicateStoreItemPurchased",
	"PredicateStoreItemUsed",
	"PredicateStoreItemExpired",
	"PredicateConsumableUsed",
	"PredicateConsumableExpired",
	"PredicateInventoryUpdated",
}

func TestPhase1AgentStoreDisabledInEveryShippedProfile(t *testing.T) {
	for _, path := range shippedRuntimeProfiles {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}

			var profile runtimeProfileContract
			if err := json.Unmarshal(data, &profile); err != nil {
				t.Fatalf("decode %s: %v", path, err)
			}

			store, exists := profile.Components["agent_store"]
			if !exists {
				t.Fatal("components.agent_store must remain registered but disabled for Phase 1")
			}
			if store.Type != "processor" {
				t.Errorf("components.agent_store.type = %q, want processor", store.Type)
			}
			if store.Name != "agent_store" {
				t.Errorf("components.agent_store.name = %q, want agent_store", store.Name)
			}
			if store.Enabled {
				t.Error("components.agent_store.enabled = true, want false")
			}
			for _, violation := range runtimeProfileViolations(profile) {
				t.Error(violation)
			}
		})
	}
}

func TestPhase1AgentStoreRawEventPlaneRemovedWhilePolicyRemains(t *testing.T) {
	productionGo := readProductionGoFiles(t)
	for path, data := range productionGo {
		source := string(data)
		for _, symbol := range append(removedAgentStorePayloads, removedAgentStoreSubjects...) {
			if strings.Contains(source, symbol) {
				t.Errorf("%s retains removed agent store raw-event symbol %s", path, symbol)
			}
		}
	}

	if _, err := os.Stat("processor/agentstore/payloads.go"); !os.IsNotExist(err) {
		t.Errorf("processor/agentstore/payloads.go must be deleted; stat error = %v", err)
	}

	vocab, err := os.ReadFile("domain/vocab.go")
	if err != nil {
		t.Fatalf("read domain/vocab.go: %v", err)
	}
	for _, predicate := range removedAgentStoreEventPredicates {
		if strings.Contains(string(vocab), predicate+" =") {
			t.Errorf("domain/vocab.go retains raw-event predicate constant %s", predicate)
		}
		if strings.Contains(string(vocab), "vocabulary.Register("+predicate) {
			t.Errorf("domain/vocab.go retains raw-event predicate registration %s", predicate)
		}
	}

	assertFileExcludes(t, "processor/agentstore/component.go", `Name:        "store-events"`)
	for _, payload := range removedAgentStorePayloads {
		assertFileExcludes(t, "graph_relationship_contract_test.go", payload)
	}

	assertFileContains(t, "processor/agentstore/store.go",
		"type StoreItem struct",
		"type OwnedItem struct",
		"type AgentInventory struct",
		"type ActiveEffect struct",
		"func DefaultCatalog() []StoreItem",
		"func (s *StoreItem) Triples()",
		"func (inv *AgentInventory) Triples()",
	)
	assertFileContains(t, "processor/agentstore/register.go",
		`const ComponentName = "agent_store"`,
		"func Register(",
	)
	assertFileContains(t, "processor/agentstore/handler.go",
		"func (c *Component) Purchase(",
		"func (c *Component) UseConsumable(",
	)
	assertFileContains(t, "service/api/service.go",
		`"store"`,
		`"store/{id}"`,
		`"store/purchase"`,
		`"agents/{id}/inventory"`,
		`"agents/{id}/inventory/use"`,
		`"agents/{id}/effects"`,
	)
	assertFileContains(t, "processor/autonomy/actions.go",
		"func (c *Component) shopAction() action",
		"func (c *Component) shopStrategicAction() action",
		"func (c *Component) useConsumableAction() action",
		"func (c *Component) useCooldownSkipAction() action",
	)
}

func TestPhase1StartupDoesNotInvokeDormantE2EStoreSeeder(t *testing.T) {
	file, err := parser.ParseFile(
		token.NewFileSet(),
		"cmd/semdragons/seed_e2e.go",
		nil,
		0,
	)
	if err != nil {
		t.Fatalf("parse cmd/semdragons/seed_e2e.go: %v", err)
	}

	var calls int
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		identifier, ok := call.Fun.(*ast.Ident)
		if ok && identifier.Name == "seedStore" {
			calls++
		}
		return true
	})
	if calls != 0 {
		t.Errorf("cmd/semdragons/seed_e2e.go invokes dormant seedStore %d times, want 0", calls)
	}
}

func TestPhase1ComponentManagerDoesNotCreateDisabledAgentStore(t *testing.T) {
	for _, path := range shippedRuntimeProfiles {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}

			var profile semconfig.Config
			if err := json.Unmarshal(data, &profile); err != nil {
				t.Fatalf("decode %s as semstreams config: %v", path, err)
			}
			storeConfig, exists := profile.Components["agent_store"]
			if !exists {
				t.Fatal("components.agent_store is missing")
			}

			registry := component.NewRegistry()
			if err := agentstore.Register(registry); err != nil {
				t.Fatalf("register dormant agent store factory: %v", err)
			}
			managerService, err := semservice.NewComponentManager(
				nil,
				&semservice.Dependencies{ComponentRegistry: registry},
			)
			if err != nil {
				t.Fatalf("construct component manager: %v", err)
			}
			manager, ok := managerService.(*semservice.ComponentManager)
			if !ok {
				t.Fatalf("component manager type = %T", managerService)
			}

			filtered := &semconfig.Config{
				Components: semconfig.ComponentConfigs{
					"agent_store": storeConfig,
				},
			}
			if err := manager.CreateComponentsFromConfig(context.Background(), filtered); err != nil {
				t.Fatalf("create components from disabled config: %v", err)
			}
			if components := manager.ListComponents(); len(components) != 0 {
				t.Errorf("disabled profile created %d component instances, want 0", len(components))
			}
			if instance := registry.Component("agent_store"); instance != nil {
				t.Errorf("disabled profile registered live agent_store instance %T", instance)
			}
		})
	}
}
