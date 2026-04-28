// Package main implements the entry point for the Semdragons application.
// Semdragons is an agentic workflow coordination framework modeled as a
// tabletop RPG, built on semstreams for observability.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadbuiltins"
	"github.com/c360studio/semstreams/payloadregistry"
	"github.com/c360studio/semstreams/service"
	"github.com/c360studio/semstreams/types"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/componentregistry"
	"github.com/c360studio/semdragons/processor/questbridge"
	"github.com/c360studio/semdragons/semsource"
	svcapi "github.com/c360studio/semdragons/service/api"
)

// Version and BuildTime are vars so they can be overridden at build time via:
//
//	go build -ldflags "-X main.Version=1.2.3 -X main.BuildTime=$(date -u +%FT%TZ)"
var (
	Version   = "0.1.0"
	BuildTime = "dev"
)

const appName = "semdragons"

func main() {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			_, _ = fmt.Fprintf(os.Stderr, "PANIC: %v\nStack trace:\n%s\n", r, string(buf[:n]))
			os.Exit(2)
		}
	}()

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// 0. Load .env file if it exists. This must happen before any os.Getenv
	// calls so that API keys stored in .env are visible to the rest of startup.
	// Errors are silently ignored: the file is optional and its absence is normal
	// in production environments that inject credentials via the OS environment.
	_ = godotenv.Load()

	// 1. Print banner
	printBanner()

	// 2. Parse and validate CLI flags
	cliCfg, shouldExit, err := parseCLI()
	if shouldExit || err != nil {
		return err
	}

	// 2a. Start pprof debug server if enabled
	if cliCfg.Debug {
		cliCfg.LogLevel = "debug"
		if cliCfg.DebugPort > 0 {
			go startPProfServer(cliCfg.DebugPort)
		}
	}

	// 3. Load and validate configuration
	cfg, rawConfigData, err := loadConfig(cliCfg.ConfigPath, cliCfg.ModelsPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	if cliCfg.Validate {
		fmt.Println("Configuration is valid")
		return nil
	}

	// 4. Connect to NATS
	ctx := context.Background()
	natsClient, err := connectToNATS(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		natsClient.Close(closeCtx)
	}()

	// 5. Ensure JetStream streams exist
	if err := ensureStreams(ctx, cfg, natsClient); err != nil {
		return err
	}

	// 5a. Ensure board KV bucket exists (writer creates bucket)
	if err := ensureBoardBucket(ctx, cfg, natsClient); err != nil {
		return err
	}

	// 6. Create logger
	logger := setupLogger(cliCfg.LogLevel, cliCfg.LogFormat)
	slog.SetDefault(logger)

	slog.Info("Semdragons ready",
		"version", Version,
		"build_time", BuildTime)

	// 6a. Initialize global graph source registry from top-level config.
	// Components access this via questbridge.GlobalGraphSources() during Start().
	initGraphSources(rawConfigData, logger)

	// 7. Create remaining infrastructure
	metricsRegistry, platform, configManager, err := setupInfrastructure(ctx, cfg, natsClient, logger)
	if err != nil {
		return err
	}
	defer configManager.Stop(5 * time.Second)

	// 8. Setup registries and manager
	componentRegistry, manager, err := setupRegistriesAndManager(cfg)
	if err != nil {
		return err
	}

	// 8a. Build the payload registry (semstreams beta.18+ retired the package-level
	// singleton). Builtins cover agentic/message/dispatch/rule/objectstore types;
	// semsource adds entity + status payloads streamed from semsource instances.
	payloadReg, err := buildPayloadRegistry()
	if err != nil {
		return err
	}

	// 9. Create service dependencies
	svcDeps := createServiceDependencies(natsClient, metricsRegistry, logger, platform, configManager, componentRegistry)
	svcDeps.PayloadRegistry = payloadReg

	// 10. Configure and create services
	if err := configureAndCreateServices(cfg, manager, svcDeps); err != nil {
		return err
	}

	// 10a. Seed E2E test data when SEED_E2E=true
	if err := maybeSeedE2E(ctx, cfg, natsClient); err != nil {
		return err
	}

	// 11. Run application with signal handling
	return runWithSignalHandling(ctx, manager, cliCfg.ShutdownTimeout)
}

// parseCLI parses and validates CLI flags.
func parseCLI() (*CLIConfig, bool, error) {
	cliCfg := parseFlags()
	if err := validateFlags(cliCfg); err != nil {
		return nil, false, fmt.Errorf("invalid flags: %w", err)
	}

	if cliCfg.ShowVersion {
		fmt.Printf("%s version %s\n", appName, Version)
		return nil, true, nil
	}

	if cliCfg.ShowHelp {
		printHelp()
		return nil, true, nil
	}

	return cliCfg, false, nil
}

// connectToNATS connects to NATS. NATS is a hard requirement.
func connectToNATS(ctx context.Context, cfg *config.Config) (*natsclient.Client, error) {
	fmt.Print("Connecting to NATS... ")

	natsClient, err := createNATSClient(cfg)
	if err != nil {
		fmt.Println("FAILED")
		return nil, fmt.Errorf("create NATS client: %w", err)
	}

	if err := natsClient.Connect(ctx); err != nil {
		fmt.Println("FAILED")
		return nil, fmt.Errorf("connect to NATS: %w", err)
	}

	connCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := natsClient.WaitForConnection(connCtx); err != nil {
		fmt.Println("FAILED")
		return nil, fmt.Errorf("NATS connection timeout: %w", err)
	}

	fmt.Println("OK")
	return natsClient, nil
}

// ensureStreams creates JetStream streams.
func ensureStreams(ctx context.Context, cfg *config.Config, natsClient *natsclient.Client) error {
	fmt.Print("Creating JetStream streams... ")

	quietLogger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	streamsManager := config.NewStreamsManager(natsClient, quietLogger)

	if err := streamsManager.EnsureStreams(ctx, cfg); err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("ensure streams: %w", err)
	}

	fmt.Println("OK")
	return nil
}

// ensureBoardBucket creates the board-specific KV bucket for entity states.
// Uses the same get-or-create pattern as graph-ingest for ENTITY_STATES.
func ensureBoardBucket(ctx context.Context, cfg *config.Config, natsClient *natsclient.Client) error {
	fmt.Print("Ensuring board KV bucket... ")

	boardCfg, err := extractBoardConfig(cfg)
	if err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("resolve board config: %w", err)
	}

	graph := semdragons.NewGraphClient(natsClient, boardCfg)
	if err := graph.EnsureBucket(ctx); err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("ensure board bucket: %w", err)
	}

	fmt.Printf("OK (%s)\n", boardCfg.BucketName())
	return nil
}

// graphSourcesEnvelope is a minimal struct for extracting the top-level
// "graph_sources" array from the semdragons config JSON. semstreams'
// config.Config doesn't know about this field, so we parse it separately.
type graphSourcesEnvelope struct {
	GraphSources []questbridge.GraphSource `json:"graph_sources"`
}

// initGraphSources parses the top-level graph_sources from raw config JSON
// and initializes the process-wide GraphSourceRegistry singleton.
// Components access it via questbridge.GlobalGraphSources().
func initGraphSources(rawConfig []byte, logger *slog.Logger) {
	var envelope graphSourcesEnvelope
	if err := json.Unmarshal(rawConfig, &envelope); err != nil {
		logger.Warn("failed to parse graph_sources from config", "error", err)
		return
	}
	if len(envelope.GraphSources) == 0 {
		return
	}

	reg := questbridge.NewGraphSourceRegistry(envelope.GraphSources, logger)
	questbridge.SetGlobalGraphSources(reg)
	logger.Info("graph source registry initialized",
		"sources", len(envelope.GraphSources))
}

// setupLogger creates a structured logger.
func setupLogger(level, format string) *slog.Logger {
	logLevel := parseLogLevel(level)

	opts := &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: logLevel == slog.LevelDebug,
	}

	var handler slog.Handler
	switch strings.ToLower(format) {
	case "text":
		handler = slog.NewTextHandler(os.Stdout, opts)
	default:
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// setupInfrastructure creates metrics, platform, and config manager.
func setupInfrastructure(
	ctx context.Context,
	cfg *config.Config,
	natsClient *natsclient.Client,
	logger *slog.Logger,
) (*metric.MetricsRegistry, types.PlatformMeta, *config.Manager, error) {
	metricsRegistry := metric.NewMetricsRegistry()

	platform := extractPlatformMeta(cfg)

	slog.Info("Platform identity configured",
		"org", platform.Org,
		"platform", platform.Platform,
		"environment", cfg.Platform.Environment)

	configManager, err := config.NewConfigManager(cfg, natsClient, logger)
	if err != nil {
		return nil, types.PlatformMeta{}, nil, fmt.Errorf("create config manager: %w", err)
	}

	if err := configManager.Start(ctx); err != nil {
		return nil, types.PlatformMeta{}, nil, fmt.Errorf("start config manager: %w", err)
	}

	return metricsRegistry, platform, configManager, nil
}

// createNATSClient creates a NATS client from config.
// Optional auth is read from environment variables so credentials never appear
// in config files or command-line flags.
func createNATSClient(cfg *config.Config) (*natsclient.Client, error) {
	natsURLs := "nats://localhost:4222"

	if envURL := os.Getenv("SEMDRAGONS_NATS_URLS"); envURL != "" {
		natsURLs = envURL
	} else if envURL := os.Getenv("SEMSTREAMS_NATS_URLS"); envURL != "" {
		natsURLs = envURL
	} else if len(cfg.NATS.URLs) > 0 {
		natsURLs = strings.Join(cfg.NATS.URLs, ",")
	}

	var opts []natsclient.ClientOption

	if user := os.Getenv("SEMDRAGONS_NATS_USER"); user != "" {
		opts = append(opts, natsclient.WithCredentials(user, os.Getenv("SEMDRAGONS_NATS_PASS")))
	}
	if token := os.Getenv("SEMDRAGONS_NATS_TOKEN"); token != "" {
		opts = append(opts, natsclient.WithToken(token))
	}

	return natsclient.NewClient(natsURLs, opts...)
}

// extractPlatformMeta extracts platform identity from config.
func extractPlatformMeta(cfg *config.Config) types.PlatformMeta {
	platformID := cfg.Platform.InstanceID
	if platformID == "" {
		platformID = cfg.Platform.ID
	}

	return types.PlatformMeta{
		Org:      cfg.Platform.Org,
		Platform: platformID,
	}
}

// buildPayloadRegistry constructs the per-binary payload registry and
// registers all payload types semdragons consumes. semstreams beta.18 retired
// the package-level singleton; each binary now owns its registry and injects
// it via service.Dependencies.PayloadRegistry, which ComponentManager plumbs
// to component.Dependencies.PayloadRegistry on every NewFromConfig call.
func buildPayloadRegistry() (*payloadregistry.Registry, error) {
	reg := payloadregistry.New()
	if err := payloadbuiltins.Register(reg); err != nil {
		return nil, fmt.Errorf("register builtin payloads: %w", err)
	}
	if err := semsource.RegisterPayloads(reg); err != nil {
		return nil, fmt.Errorf("register semsource payloads: %w", err)
	}
	return reg, nil
}

// setupRegistriesAndManager creates registries and service manager.
func setupRegistriesAndManager(cfg *config.Config) (*component.Registry, *service.Manager, error) {
	// Register semdragons components (includes graph processors)
	componentRegistry := component.NewRegistry()
	if err := componentregistry.RegisterAll(componentRegistry); err != nil {
		return nil, nil, fmt.Errorf("register components: %w", err)
	}

	factories := componentRegistry.ListFactories()
	slog.Info("Component factories registered", "count", len(factories), "factories", factories)

	// Register semstreams built-in services + semdragons-api
	serviceRegistry := service.NewServiceRegistry()
	if err := service.RegisterAll(serviceRegistry); err != nil {
		return nil, nil, fmt.Errorf("register semstreams services: %w", err)
	}
	if err := serviceRegistry.Register("game", svcapi.New); err != nil {
		return nil, nil, fmt.Errorf("register game service: %w", err)
	}

	manager := service.NewServiceManager(serviceRegistry)
	ensureServiceManagerConfig(cfg)

	return componentRegistry, manager, nil
}

// ensureServiceManagerConfig ensures service-manager config exists with defaults.
func ensureServiceManagerConfig(cfg *config.Config) {
	if cfg.Services == nil {
		cfg.Services = make(types.ServiceConfigs)
	}

	if _, exists := cfg.Services["service-manager"]; !exists {
		defaultConfig := map[string]any{
			"http_port":  8080,
			"swagger_ui": true,
			"server_info": map[string]string{
				"title":       "Semdragons API",
				"description": "Agentic workflow coordination framework",
				"version":     Version,
			},
		}
		defaultConfigJSON, _ := json.Marshal(defaultConfig)
		cfg.Services["service-manager"] = types.ServiceConfig{
			Name:    "service-manager",
			Enabled: true,
			Config:  defaultConfigJSON,
		}
	}
}

// createServiceDependencies creates the Dependencies struct for services.
func createServiceDependencies(
	natsClient *natsclient.Client,
	metricsRegistry *metric.MetricsRegistry,
	logger *slog.Logger,
	platform types.PlatformMeta,
	configManager *config.Manager,
	componentRegistry *component.Registry,
) *service.Dependencies {
	return &service.Dependencies{
		NATSClient:        natsClient,
		MetricsRegistry:   metricsRegistry,
		Logger:            logger,
		Platform:          platform,
		Manager:           configManager,
		ComponentRegistry: componentRegistry,
	}
}

// configureAndCreateServices configures the manager and creates all services.
func configureAndCreateServices(
	cfg *config.Config,
	manager *service.Manager,
	svcDeps *service.Dependencies,
) error {
	if err := manager.ConfigureFromServices(cfg.Services, svcDeps); err != nil {
		return fmt.Errorf("configure service manager: %w", err)
	}

	for name, svcConfig := range cfg.Services {
		if name == "service-manager" {
			continue
		}

		if !svcConfig.Enabled {
			slog.Info("Service disabled in config", "name", name)
			continue
		}

		if !manager.HasConstructor(name) {
			slog.Warn("Service configured but not registered",
				"key", name,
				"available_constructors", manager.ListConstructors())
			continue
		}

		if _, err := manager.CreateService(name, svcConfig.Config, svcDeps); err != nil {
			return fmt.Errorf("create service %s: %w", name, err)
		}

		slog.Info("Created service", "name", name)
	}

	return nil
}

// runWithSignalHandling starts services and handles shutdown signals.
// All cross-component dependencies are resolved lazily via deps.ComponentRegistry
// (set automatically by ComponentManager on every NewFromConfig call, including restarts).
func runWithSignalHandling(ctx context.Context, manager *service.Manager, shutdownTimeout time.Duration) error {
	signalCtx, signalCancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer signalCancel()

	slog.Info("Starting all services")
	if err := manager.StartAll(signalCtx); err != nil {
		return fmt.Errorf("start services: %w", err)
	}
	slog.Info("All services started successfully")

	<-signalCtx.Done()
	slog.Info("Received shutdown signal")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()
	_ = shutdownCtx // available for future use in the shutdown sequence

	if err := manager.StopAll(shutdownTimeout); err != nil {
		slog.Error("Error stopping services", "error", err)
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}

	slog.Info("Semdragons shutdown complete")
	return nil
}

// startPProfServer starts a pprof HTTP server on the given port.
// The blank import of net/http/pprof registers handlers on DefaultServeMux.
func startPProfServer(port int) {
	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("Starting pprof server on %s\n", addr)
	if err := http.ListenAndServe(addr, nil); err != nil && err != http.ErrServerClosed {
		fmt.Printf("pprof server error: %v\n", err)
	}
}

// loadConfig loads the base configuration and optionally merges a model overlay.
// The overlay file is deep-merged on top of the base: map values are merged
// recursively, all other types are replaced. This allows model overlay files to
// set just model_registry and component tuning without duplicating the full config.
// Returns both the parsed config and the raw merged JSON bytes (for extracting
// semdragons-specific top-level fields that semstreams config.Config doesn't know about).
func loadConfig(basePath, modelsPath string) (*config.Config, []byte, error) {
	baseData, err := os.ReadFile(basePath)
	if err != nil {
		return nil, nil, fmt.Errorf("read base config %s: %w", basePath, err)
	}

	data := baseData
	if modelsPath != "" {
		merged, mergeErr := mergeOverlay(data, modelsPath)
		if mergeErr != nil {
			return nil, nil, fmt.Errorf("merge models overlay: %w", mergeErr)
		}
		data = merged
		fmt.Printf("Merged model overlay: %s\n", modelsPath)
	}

	loader := config.NewLoader()
	cfg, err := loader.LoadFromBytes(data)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse config: %w", err)
	}
	return cfg, data, nil
}
