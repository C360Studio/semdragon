// Package api provides the Semdragons domain REST API service.
// It implements service.Service and service.HTTPHandler to expose
// game world endpoints (quests, agents, battles, store, etc.)
// via the semstreams service manager's HTTP server.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/processor/agentstore"
	"github.com/c360studio/semdragons/processor/boardcontrol"
	"github.com/c360studio/semdragons/processor/dmworldstate"
	"github.com/c360studio/semdragons/processor/tokenbudget"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/service"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semstreams/storage"
)

// Config holds configuration for the semdragons-api service.
type Config struct {
	Board        string                    `json:"board"`                   // Board name (default: "board1")
	Org          string                    `json:"org"`                     // Org namespace (default from platform)
	Platform     string                    `json:"platform"`               // Platform ID (default from platform)
	MaxEntities  int                       `json:"max_entities"`           // Max entities per query (default: 1000)
	WorkspaceDir string                    `json:"workspace_dir,omitempty"` // Host directory for agent workspace artifacts
	TokenBudget  *tokenbudget.BudgetConfig `json:"token_budget,omitempty"` // Token budget config
}

// maxChatSessions caps the number of in-memory DM chat session traces.
// When exceeded, the map is cleared to prevent unbounded growth.
const maxChatSessions = 1000

// Service provides domain REST endpoints for the Semdragons game world.
type Service struct {
	*service.BaseService
	graph           GraphQuerier       // concrete type is *semdragons.GraphClient
	world           WorldStateProvider // concrete type is *dmworldstate.WorldStateAggregator
	store           StoreProvider      // concrete type is *agentstore.Component; nil if unavailable
	storeOnce       sync.Once          // guards lazy store resolution
	artifactStore     storage.Store      // filestore for quest artifacts; nil if unavailable
	artifactStoreOnce sync.Once          // guards lazy artifact store resolution
	componentDeps   *service.Dependencies // retained for lazy component resolution
	models          ModelResolver      // concrete type is *model.Registry; nil if unavailable
	nats            *natsclient.Client // direct NATS access for KV buckets outside graph
	trajectories    TrajectoryQuerier  // trajectory KV lookups; nil before init
	dmSessionReader DMSessionReader    // session reads (used by GET handler); nil before init
	board           *boardcontrol.Controller // board play/pause control; nil before init
	tokenLedger     *tokenbudget.TokenLedger // token budget tracking; nil before init
	boardConfig     *domain.BoardConfig      // board identity for bucket name, entity IDs
	config          Config
	logger          *slog.Logger

	// DM session persistence — persists chat turns to NATS KV for server restart recovery.
	dmSessions *dmSessionStore

	// DM chat session traces for audit trail continuity.
	// Each session gets a root trace; each turn creates a child span.
	// The trace context propagates to graph operations so quests
	// created from chat inherit the DM conversation trace.
	chatTracesMu    sync.RWMutex
	chatTraces      map[string]*natsclient.TraceContext
	chatTracesOrder []string // insertion-ordered session IDs for eviction
}

// New creates a new Semdragons API service.
// This is a service.Constructor compatible function.
func New(rawConfig json.RawMessage, deps *service.Dependencies) (service.Service, error) {
	cfg := Config{
		Board:       "board1",
		MaxEntities: 1000,
	}
	if len(rawConfig) > 0 {
		if err := json.Unmarshal(rawConfig, &cfg); err != nil {
			return nil, fmt.Errorf("parse semdragons-api config: %w", err)
		}
	}

	if deps == nil || deps.NATSClient == nil {
		return nil, fmt.Errorf("game service requires NATS client")
	}

	// Resolve org/platform from config or platform identity
	org := cfg.Org
	if org == "" {
		org = deps.Platform.Org
	}
	if org == "" {
		org = "local"
	}

	platform := cfg.Platform
	if platform == "" {
		platform = deps.Platform.Platform
	}
	if platform == "" {
		platform = "dev"
	}

	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With("service", "game")

	boardConfig := &domain.BoardConfig{
		Org:      org,
		Platform: platform,
		Board:    cfg.Board,
	}

	graph := semdragons.NewGraphClient(deps.NATSClient, boardConfig)
	world := dmworldstate.NewWorldStateAggregator(graph, cfg.MaxEntities, logger)
	models := resolveModelRegistry(deps)

	sessions := &dmSessionStore{nats: deps.NATSClient, logger: logger}

	baseService := service.NewBaseServiceWithOptions(
		"game",
		nil,
		service.WithLogger(logger),
		service.WithMetrics(deps.MetricsRegistry),
		service.WithNATS(deps.NATSClient),
	)

	return &Service{
		BaseService:     baseService,
		graph:           graph,
		world:           world,
		componentDeps:   deps,
		models:          models,
		nats:            deps.NATSClient,
		trajectories:    &natsTrajectoryQuerier{nats: deps.NATSClient},
		dmSessionReader: sessions,
		dmSessions:      sessions,
		boardConfig:     boardConfig,
		config:          cfg,
		logger:          logger,
		chatTraces:      make(map[string]*natsclient.TraceContext),
	}, nil
}

// getStore lazily resolves the agent_store component from the registry.
// Components are created after services, so eager resolution at construction
// time finds nothing. sync.Once ensures the lookup happens only once.
// If store was set directly (e.g. in tests), the pre-set value is preserved.
func (s *Service) getStore() StoreProvider {
	s.storeOnce.Do(func() {
		if s.store == nil {
			s.store = resolveStoreComponent(s.componentDeps, s.logger)
		}
	})
	return s.store
}

// resolveStoreComponent attempts to retrieve the agentstore component from the
// component registry. Returns nil with a warning if unavailable — handlers
// degrade gracefully by returning 503 Service Unavailable.
func resolveStoreComponent(deps *service.Dependencies, logger *slog.Logger) StoreProvider {
	if deps == nil || deps.ComponentRegistry == nil {
		return nil
	}
	comp := deps.ComponentRegistry.Component(agentstore.ComponentName)
	if comp == nil {
		logger.Warn("agent_store component not found in registry; store endpoints will return 503")
		return nil
	}
	sp, ok := comp.(StoreProvider)
	if !ok {
		logger.Warn("agent_store component does not satisfy StoreProvider interface",
			"type", fmt.Sprintf("%T", comp))
		return nil
	}
	return sp
}

// resolveModelRegistry retrieves the model registry from the config manager when
// available, falling back to the default dev registry (local Ollama). This ensures
// production deployments use provider endpoints defined in semdragons.json rather
// than the hardcoded local-only defaults.
func resolveModelRegistry(deps *service.Dependencies) ModelResolver {
	if deps.Manager != nil {
		cfg := deps.Manager.GetConfig()
		if cfg != nil {
			c := cfg.Get()
			if c != nil && c.ModelRegistry != nil {
				return c.ModelRegistry
			}
		}
	}
	return semdragons.DefaultModelRegistry()
}

// Start starts the API service.
func (s *Service) Start(ctx context.Context) error {
	s.SetHealthCheck(func() error {
		return nil
	})

	// Initialize board control (play/pause).
	bucket, err := boardcontrol.EnsureBucket(ctx, s.nats)
	if err != nil {
		s.logger.Warn("failed to create BOARD_CONTROL bucket; pause/resume disabled", "error", err)
	} else {
		ctrl := boardcontrol.NewController(bucket, s.nats, s.logger)
		if startErr := ctrl.Start(ctx); startErr != nil {
			s.logger.Warn("failed to start board controller; pause/resume disabled", "error", startErr)
		} else {
			s.board = ctrl
		}
	}

	// Initialize token budget ledger.
	s.tokenLedger = tokenbudget.NewTokenLedger(s.config.TokenBudget, s.logger)
	tokenBucket, tbErr := s.nats.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      tokenbudget.BucketName,
		Description: "Token budget tracking and limits",
		History:     5,
		TTL:         72 * time.Hour,
	})
	if tbErr != nil {
		s.logger.Warn("failed to create TOKEN_BUDGET bucket; persistence disabled", "error", tbErr)
	} else {
		s.tokenLedger.SetBucket(tokenBucket)
	}
	if s.board != nil {
		s.tokenLedger.SetBoardPauser(&boardPauserAdapter{ctrl: s.board})
	}
	if startErr := s.tokenLedger.Start(ctx); startErr != nil {
		s.logger.Warn("failed to start token ledger", "error", startErr)
	}

	// Wire token ledger into components that need budget enforcement.
	// Components are instantiated by the component-manager service; at this
	// point they may not exist yet. Use lazy wiring: try now, but the
	// components also accept the ledger when they first Initialize.
	s.wireTokenLedgerToComponents()

	if err := s.BaseService.Start(ctx); err != nil {
		return err
	}

	s.logger.Info("Game API service started",
		"board", s.config.Board,
		"max_entities", s.config.MaxEntities)
	return nil
}

// Stop stops the API service.
func (s *Service) Stop(timeout time.Duration) error {
	s.logger.Info("Game API service stopping")

	if s.board != nil {
		s.board.Stop()
	}

	s.chatTracesMu.Lock()
	s.chatTraces = make(map[string]*natsclient.TraceContext)
	s.chatTracesOrder = nil
	s.chatTracesMu.Unlock()

	return s.BaseService.Stop(timeout)
}

// Board returns the board controller for pause-checking by components.
// Returns nil if the controller could not be initialized.
func (s *Service) Board() *boardcontrol.Controller {
	return s.board
}

// TokenLedger returns the shared token ledger for components to wire up.
func (s *Service) TokenLedger() *tokenbudget.TokenLedger {
	return s.tokenLedger
}

// boardPauserAdapter wraps boardcontrol.Controller to satisfy tokenbudget.BoardPauser.
type boardPauserAdapter struct {
	ctrl *boardcontrol.Controller
}

func (a *boardPauserAdapter) PauseBoard(ctx context.Context, actor string) error {
	_, err := a.ctrl.Pause(ctx, actor)
	return err
}

// tokenLedgerSetter is implemented by components that accept a token ledger.
type tokenLedgerSetter interface {
	SetTokenLedger(l *tokenbudget.TokenLedger)
}

// wireTokenLedgerToComponents injects the token ledger into any registered
// components that support it. Components may not be instantiated yet at
// service Start time — in that case, they should call TokenLedger() during
// their own initialization.
func (s *Service) wireTokenLedgerToComponents() {
	if s.componentDeps == nil || s.componentDeps.ComponentRegistry == nil || s.tokenLedger == nil {
		return
	}

	// Component names that should receive the token ledger.
	names := []string{"questbridge", "bossbattle"}
	for _, name := range names {
		comp := s.componentDeps.ComponentRegistry.Component(name)
		if comp == nil {
			continue
		}
		if setter, ok := comp.(tokenLedgerSetter); ok {
			setter.SetTokenLedger(s.tokenLedger)
			s.logger.Info("wired token ledger to component", "component", name)
		}
	}
}

// RegisterHTTPHandlers registers domain REST endpoints with the HTTP mux.
func (s *Service) RegisterHTTPHandlers(prefix string, mux *http.ServeMux) {
	// Ensure prefix ends with /
	if !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}

	// Load API key once — empty string means dev mode (no auth).
	apiKey := loadAPIKey()

	// CORS middleware — sets headers on all responses for simple requests.
	// X-API-Key is included so browsers allow the auth header in cross-origin POSTs.
	cors := func(handler http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
			handler(w, r)
		}
	}

	// OPTIONS preflight catch-all — Go 1.22+ method-qualified routes reject
	// OPTIONS, so we register a blanket handler for the entire prefix.
	mux.HandleFunc("OPTIONS "+prefix+"{path...}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
		w.WriteHeader(http.StatusNoContent)
	})

	// World state
	mux.HandleFunc("GET "+prefix+"world", cors(s.handleWorldState))

	// Quests
	mux.HandleFunc("GET "+prefix+"quests", cors(s.handleListQuests))
	mux.HandleFunc("GET "+prefix+"quests/{id}", cors(s.handleGetQuest))
	mux.HandleFunc("POST "+prefix+"quests/chain", cors(requireAuth(apiKey, s.handlePostQuestChain)))
	mux.HandleFunc("POST "+prefix+"quests", cors(requireAuth(apiKey, s.handleCreateQuest)))

	// Quest lifecycle
	mux.HandleFunc("POST "+prefix+"quests/{id}/claim", cors(requireAuth(apiKey, s.handleClaimQuest)))
	mux.HandleFunc("POST "+prefix+"quests/{id}/start", cors(requireAuth(apiKey, s.handleStartQuest)))
	mux.HandleFunc("POST "+prefix+"quests/{id}/submit", cors(requireAuth(apiKey, s.handleSubmitResult)))
	mux.HandleFunc("POST "+prefix+"quests/{id}/complete", cors(requireAuth(apiKey, s.handleCompleteQuest)))
	mux.HandleFunc("POST "+prefix+"quests/{id}/fail", cors(requireAuth(apiKey, s.handleFailQuest)))
	mux.HandleFunc("POST "+prefix+"quests/{id}/abandon", cors(requireAuth(apiKey, s.handleAbandonQuest)))

	// Quest artifacts
	mux.HandleFunc("GET "+prefix+"quests/{id}/artifacts/list", cors(s.handleListQuestArtifacts))
	mux.HandleFunc("GET "+prefix+"quests/{id}/artifacts/{path...}", cors(s.handleGetQuestArtifactFile))
	mux.HandleFunc("GET "+prefix+"quests/{id}/artifacts", cors(s.handleGetQuestArtifacts))

	// Agents
	mux.HandleFunc("GET "+prefix+"agents", cors(s.handleListAgents))
	mux.HandleFunc("GET "+prefix+"agents/{id}/inventory", cors(s.handleGetInventory))
	mux.HandleFunc("POST "+prefix+"agents/{id}/inventory/use", cors(requireAuth(apiKey, s.handleUseConsumable)))
	mux.HandleFunc("GET "+prefix+"agents/{id}/effects", cors(s.handleGetEffects))
	mux.HandleFunc("GET "+prefix+"agents/{id}", cors(s.handleGetAgent))
	mux.HandleFunc("POST "+prefix+"agents/{id}/retire", cors(requireAuth(apiKey, s.handleRetireAgent)))
	mux.HandleFunc("POST "+prefix+"agents", cors(requireAuth(apiKey, s.handleRecruitAgent)))

	// Battles
	mux.HandleFunc("GET "+prefix+"battles", cors(s.handleListBattles))
	mux.HandleFunc("GET "+prefix+"battles/{id}", cors(s.handleGetBattle))

	// Parties
	mux.HandleFunc("GET "+prefix+"parties", cors(s.handleListParties))
	mux.HandleFunc("GET "+prefix+"parties/{id}", cors(s.handleGetParty))

	// Guilds
	mux.HandleFunc("GET "+prefix+"guilds", cors(s.handleListGuilds))
	mux.HandleFunc("GET "+prefix+"guilds/{id}", cors(s.handleGetGuild))

	// Trajectories
	mux.HandleFunc("GET "+prefix+"trajectories/{id}", cors(s.handleGetTrajectory))

	// DM
	mux.HandleFunc("POST "+prefix+"dm/chat", cors(requireAuth(apiKey, s.handleDMChat)))
	mux.HandleFunc("GET "+prefix+"dm/sessions/{id}", cors(s.handleGetDMSession))
	mux.HandleFunc("POST "+prefix+"dm/intervene/{questId}", cors(requireAuth(apiKey, s.handleDMIntervene)))
	mux.HandleFunc("POST "+prefix+"dm/triage/{questId}", cors(requireAuth(apiKey, s.handleDMTriage)))

	// Peer Reviews
	mux.HandleFunc("POST "+prefix+"reviews", cors(requireAuth(apiKey, s.handleCreateReview)))
	mux.HandleFunc("POST "+prefix+"reviews/{id}/submit", cors(requireAuth(apiKey, s.handleSubmitReview)))
	mux.HandleFunc("GET "+prefix+"reviews/{id}", cors(s.handleGetReview))
	mux.HandleFunc("GET "+prefix+"reviews", cors(s.handleListReviews))
	mux.HandleFunc("GET "+prefix+"agents/{id}/reviews", cors(s.handleListAgentReviews))

	// Store
	mux.HandleFunc("GET "+prefix+"store", cors(s.handleListStore))
	mux.HandleFunc("GET "+prefix+"store/{id}", cors(s.handleGetStoreItem))
	mux.HandleFunc("POST "+prefix+"store/purchase", cors(requireAuth(apiKey, s.handlePurchase)))

	// Board control (play/pause)
	mux.HandleFunc("GET "+prefix+"board/status", cors(s.handleBoardStatus))
	mux.HandleFunc("POST "+prefix+"board/pause", cors(requireAuth(apiKey, s.handleBoardPause)))
	mux.HandleFunc("POST "+prefix+"board/resume", cors(requireAuth(apiKey, s.handleBoardResume)))

	// Token budget
	mux.HandleFunc("GET "+prefix+"board/tokens", cors(s.handleTokenStats))
	mux.HandleFunc("POST "+prefix+"board/tokens/budget", cors(requireAuth(apiKey, s.handleSetTokenBudget)))

	// Settings
	mux.HandleFunc("GET "+prefix+"settings", cors(s.handleGetSettings))
	mux.HandleFunc("GET "+prefix+"settings/health", cors(s.handleSettingsHealth))
	mux.HandleFunc("POST "+prefix+"settings", cors(requireAuth(apiKey, s.handleUpdateSettings)))

	// Model registry
	mux.HandleFunc("GET "+prefix+"models", cors(s.handleGetModels))

	// Workspace file browser (read-only, auth required — exposes host filesystem)
	mux.HandleFunc("GET "+prefix+"workspace", cors(requireAuth(apiKey, s.handleWorkspaceTree)))
	mux.HandleFunc("GET "+prefix+"workspace/file", cors(requireAuth(apiKey, s.handleWorkspaceFile)))

	// SSE — real-time entity updates
	mux.HandleFunc("GET "+prefix+"events", cors(s.handleEvents))

	s.logger.Info("Game API HTTP handlers registered", "prefix", prefix)
}

// OpenAPISpec returns the OpenAPI specification for domain endpoints.
func (s *Service) OpenAPISpec() *service.OpenAPISpec {
	return semdragonsOpenAPISpec()
}
