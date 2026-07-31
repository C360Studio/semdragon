//go:build integration

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/graph/readiness"
	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/lifecycle"
	"github.com/c360studio/semstreams/service"
	"github.com/c360studio/semstreams/types"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semdragons/questlifecycle"
)

func TestIntegrationProductionOwnershipCompositionStartsFrameworkWriters(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	testClient := natsclient.NewTestClient(t,
		natsclient.WithKV(),
		natsclient.WithStreams(natsclient.TestStreamConfig{
			Name: "GRAPH", Subjects: []string{"graph.>"},
		}),
	)

	cfg, _, err := loadConfig(filepath.Join("..", "..", "config", "semdragons.json"), "")
	if err != nil {
		t.Fatalf("load shipped config: %v", err)
	}
	components := config.ComponentConfigs{
		"graph-ingest":   cfg.Components["graph-ingest"],
		"graph-index":    cfg.Components["graph-index"],
		"rule-processor": cfg.Components["rule-processor"],
	}
	ruleConfig := components["rule-processor"]
	ruleConfig.Config = json.RawMessage(strings.ReplaceAll(
		string(ruleConfig.Config),
		`"config/rules/quest/`,
		`"../../config/rules/quest/`,
	))
	components["rule-processor"] = ruleConfig
	cfg.Components = components
	cfg.Services = types.ServiceConfigs{
		"service-manager": {
			Name:    "service-manager",
			Enabled: true,
			Config:  json.RawMessage(`{"http_port":0,"swagger_ui":false}`),
		},
		"component-manager": {
			Name:    "component-manager",
			Enabled: true,
			Config:  json.RawMessage(`{"watch_config":false}`),
		},
	}

	configManager, err := config.NewConfigManager(cfg, testClient.Client, logger)
	if err != nil {
		t.Fatalf("create config manager: %v", err)
	}
	if err := configManager.Start(ctx); err != nil {
		t.Fatalf("start config manager: %v", err)
	}
	t.Cleanup(func() {
		if err := configManager.Stop(5 * time.Second); err != nil {
			t.Errorf("stop config manager: %v", err)
		}
	})

	componentRegistry, manager, err := setupRegistriesAndManager(cfg)
	if err != nil {
		t.Fatalf("setup registries and manager: %v", err)
	}
	payloadRegistry, err := buildPayloadRegistry()
	if err != nil {
		t.Fatalf("build payload registry: %v", err)
	}
	metricsRegistry := metric.NewMetricsRegistry()
	dependencies := createServiceDependencies(
		testClient.Client,
		metricsRegistry,
		logger,
		extractPlatformMeta(cfg),
		configManager,
		componentRegistry,
	)
	dependencies.PayloadRegistry = payloadRegistry

	lifecycleManager := lifecycle.NewManager(testClient.Client, logger)
	hbCtx, shutdownOwnership := service.WireOwnershipShutdown(ctx, lifecycleManager)
	t.Cleanup(shutdownOwnership)
	ownerRegistry, staticHeartbeat, err := wireProductionOwnership(
		hbCtx, testClient.Client, lifecycleManager, logger,
	)
	if err != nil {
		t.Fatalf("wire production ownership: %v", err)
	}
	if _, err := testClient.Client.GetKeyValueBucket(ctx, graph.BucketEntityStates); !errors.Is(err, jetstream.ErrBucketNotFound) {
		t.Fatalf("ENTITY_STATES after ownership bootstrap: err=%v, want bucket absent", err)
	}
	if err := registerQuestLifecycle(lifecycleManager); err != nil {
		t.Fatalf("register quest lifecycle: %v", err)
	}
	dependencies.LifecycleManager = lifecycleManager
	manager.RegisterInstance(
		"ownership",
		service.NewOwnershipService(ownerRegistry, staticHeartbeat, metricsRegistry, logger),
	)
	if err := configureAndCreateServices(cfg, manager, dependencies); err != nil {
		t.Fatalf("create production services: %v", err)
	}
	if err := service.BindRulePackContracts(
		hbCtx, manager, ownerRegistry, staticHeartbeat, logger,
	); err != nil {
		t.Fatalf("bind rule-pack contracts: %v", err)
	}

	watchers := startReadinessWatchers(
		t, ctx, testClient.Client,
		readiness.KeyGraphIngest,
		readiness.KeyGraphIndex,
		readiness.KeyRule,
	)
	if err := manager.StartAll(ctx); err != nil {
		t.Fatalf("start production service composition: %v", err)
	}
	t.Cleanup(func() {
		if err := manager.StopAll(5 * time.Second); err != nil {
			t.Errorf("stop production service composition: %v", err)
		}
	})
	for key, watcher := range watchers {
		waitForReadyBootProducer(t, ctx, key, watcher)
	}

	ownershipService, ok := manager.GetService("ownership")
	if !ok || ownershipService.Status() != service.StatusRunning {
		t.Fatalf("ownership service status = %v, present=%v; want running", ownershipService, ok)
	}
	const questID = "local.boot.game.board1.quest.ownership-regression"
	owner, found, err := ownerRegistry.OwnerOf(ctx, questID, questlifecycle.PhasePredicate)
	if err != nil {
		t.Fatalf("resolve quest lifecycle owner: %v", err)
	}
	if !found || owner != questlifecycle.WorkflowName {
		t.Fatalf("quest lifecycle owner = (%q, %v), want (%q, true)", owner, found, questlifecycle.WorkflowName)
	}
	if err := lifecycleManager.Create(ctx, &questlifecycle.QuestState{
		EntityIDField: questID,
		PhaseField:    questlifecycle.PhasePosted,
		MaxAttempts:   3,
	}); err != nil {
		t.Fatalf("create quest through lifecycle manager after boot: %v", err)
	}
}

func startReadinessWatchers(
	t *testing.T,
	ctx context.Context,
	client *natsclient.Client,
	keys ...string,
) map[string]*readiness.Watcher {
	t.Helper()
	watchers := make(map[string]*readiness.Watcher, len(keys))
	for _, key := range keys {
		watcher := readiness.NewWatcher(client, key)
		if err := watcher.Start(ctx); err != nil {
			t.Fatalf("start readiness watcher for %s: %v", key, err)
		}
		t.Cleanup(watcher.Stop)
		watchers[key] = watcher
	}
	return watchers
}

func waitForReadyBootProducer(
	t *testing.T,
	ctx context.Context,
	key string,
	watcher *readiness.Watcher,
) {
	t.Helper()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		reading := watcher.Read()
		if reading.Known && reading.Fresh && reading.Status.Ready &&
			reading.Status.BootstrapComplete && reading.Status.State == graph.IndexStateReady {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for %s readiness: %v (last=%+v)", key, ctx.Err(), reading)
		case <-ticker.C:
		}
	}
}
