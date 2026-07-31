package semdragons_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/component"
	semconfig "github.com/c360studio/semstreams/config"
	graphgateway "github.com/c360studio/semstreams/gateway/graph-gateway"
	"github.com/c360studio/semstreams/natsclient"
	graphembedding "github.com/c360studio/semstreams/processor/graph-embedding"
	graphingest "github.com/c360studio/semstreams/processor/graph-ingest"
)

var beta159RuntimeProfiles = []string{
	"config/semdragons.json",
	"config/semdragons-semsource.json",
	"config/semdragons-e2e-tier1.json",
}

var beta159DisabledProductWriters = []string{
	"agent_progression",
	"agent_store",
	"boidengine",
	"questboard",
	"guildformation",
	"questbridge",
	"partycoord",
	"questdagexec",
	"autonomy",
	"redteam",
}

func TestShippedProfilesSatisfyBeta159ConfigAndFactories(t *testing.T) {
	root := repositoryRoot(t)
	client, err := natsclient.NewClient("nats://127.0.0.1:4222")
	if err != nil {
		t.Fatalf("construct disconnected NATS client: %v", err)
	}
	deps := component.Dependencies{NATSClient: client}

	for _, relative := range beta159RuntimeProfiles {
		t.Run(filepath.Base(relative), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, relative))
			if err != nil {
				t.Fatalf("read profile: %v", err)
			}

			var cfg semconfig.Config
			if err := json.Unmarshal(data, &cfg); err != nil {
				t.Fatalf("decode profile: %v", err)
			}
			if err := cfg.Validate(); err != nil {
				t.Fatalf("beta.159 Config.Validate: %v", err)
			}
			if game, ok := cfg.Services["game"]; !ok || game.Enabled {
				t.Error("game service must remain disabled until its graph writers use owner-safe framework primitives")
			}
			for _, name := range beta159DisabledProductWriters {
				componentConfig, ok := cfg.Components[name]
				if !ok {
					t.Errorf("missing fail-closed component %q", name)
					continue
				}
				if componentConfig.Enabled {
					t.Errorf("component %q must remain disabled until its complete owner boundary is migrated", name)
				}
			}
			var questTools map[string]any
			if err := json.Unmarshal(cfg.Components["questtools"].Config, &questTools); err != nil {
				t.Fatalf("decode questtools config: %v", err)
			}
			if _, declared := questTools["http_persist_to_graph"]; declared {
				t.Error("questtools must not expose the removed unowned HTML graph-persistence capability")
			}

			assertStreamPolicy(t, cfg.Streams["GRAPH"], "24h")
			assertStreamPolicy(t, cfg.Streams["AGENT"], "72h")
			var ingestConfig graphingest.Config
			if err := json.Unmarshal(cfg.Components["graph-ingest"].Config, &ingestConfig); err != nil {
				t.Fatalf("decode graph-ingest config: %v", err)
			}
			if !ingestConfig.EnforceOwnerLease {
				t.Error("graph-ingest must enforce owner leases")
			}

			gatewayCfg, ok := cfg.Components["graph-gateway"]
			if !ok || !gatewayCfg.Enabled {
				t.Fatal("graph-gateway must be enabled")
			}
			if _, err := graphgateway.CreateGraphGateway(gatewayCfg.Config, deps); err != nil {
				t.Fatalf("construct beta.159 graph-gateway: %v", err)
			}
			var configuredGateway graphgateway.Config
			if err := json.Unmarshal(gatewayCfg.Config, &configuredGateway); err != nil {
				t.Fatalf("decode graph-gateway config: %v", err)
			}
			if err := configuredGateway.Validate(); err != nil {
				t.Fatalf("validate graph-gateway config: %v", err)
			}
			if !slices.Equal(configuredGateway.ReadinessKeys, []string{"graph-ingest", "graph-index", "rule"}) {
				t.Errorf("readiness keys = %v", configuredGateway.ReadinessKeys)
			}
			if configuredGateway.ReadinessPath != "/readiness" {
				t.Errorf("readiness path = %q", configuredGateway.ReadinessPath)
			}

			if embeddingCfg, ok := cfg.Components["graph-embedding"]; ok && embeddingCfg.Enabled {
				if _, err := graphembedding.CreateGraphEmbedding(embeddingCfg.Config, deps); err != nil {
					t.Fatalf("construct beta.159 graph-embedding: %v", err)
				}
			}
		})
	}
}

func assertStreamPolicy(t *testing.T, stream semconfig.StreamConfig, maxAge string) {
	t.Helper()
	if stream.MaxAge != maxAge {
		t.Errorf("max_age = %s, want %s", stream.MaxAge, maxAge)
	}
	if stream.MaxBytes != 268435456 {
		t.Errorf("max_bytes = %d, want 268435456", stream.MaxBytes)
	}
	if stream.Discard != semconfig.StreamDiscardOld {
		t.Errorf("discard = %q, want %q", stream.Discard, semconfig.StreamDiscardOld)
	}
}

func TestBeta159RejectsUnboundedOrdinaryStream(t *testing.T) {
	cfg := &semconfig.Config{
		Platform: semconfig.PlatformConfig{Org: "local", ID: "test"},
		Streams: semconfig.StreamConfigs{
			"UNBOUNDED": {Subjects: []string{"unbounded.>"}, Storage: "file"},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("beta.159 accepted an ordinary stream without finite bounds")
	}
}

func TestBeta159CompositionWiresOwnershipBeforeWriters(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repositoryRoot(t), "cmd", "semdragons", "main.go"))
	if err != nil {
		t.Fatalf("read composition root: %v", err)
	}
	source := string(data)
	requiredInOrder := []string{
		"service.WireOwnershipShutdown(",
		"wireProductionOwnership(",
		"registerQuestLifecycle(",
		"service.NewOwnershipService(",
		"configureAndCreateServices(",
		"service.BindRulePackContracts(",
		"maybeSeedE2E(",
	}
	last := -1
	for _, fragment := range requiredInOrder {
		index := strings.Index(source, fragment)
		if index < 0 {
			t.Fatalf("composition root is missing %q", fragment)
		}
		if index <= last {
			t.Fatalf("composition root places %q before its required predecessor", fragment)
		}
		last = index
	}
	if strings.Contains(source, "service.WireOwnership(") {
		t.Fatal("zero-static-contract composition must not call service.WireOwnership")
	}
	for _, fragment := range []string{
		"graph.AssertOwnedBucketsClean(",
		"ownership.EnsureBuckets(",
		"lifecycleManager.AttachOwnership(",
		"ownerRegistry.NewHeartbeater(ownership.HeartbeatInterval)",
	} {
		if !strings.Contains(source, fragment) {
			t.Errorf("public ownership composition is missing %q", fragment)
		}
	}
}

func TestShippedComposeDisablesUnownedProductSeeding(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repositoryRoot(t), "docker", "compose.yml"))
	if err != nil {
		t.Fatalf("read shipped compose: %v", err)
	}
	compose := string(data)
	for _, name := range []string{"SEED_E2E", "SEED_AGENTS", "SEED_PROS"} {
		fragment := name + `: "${` + name + `:-false}"`
		if !strings.Contains(compose, fragment) {
			t.Errorf("shipped backend must default %s false while its owner adapter is unavailable", name)
		}
	}
}
