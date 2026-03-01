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
	"time"

	semdragons "github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/processor/dmworldstate"
	"github.com/c360studio/semstreams/service"
)

// Config holds configuration for the semdragons-api service.
type Config struct {
	Board       string `json:"board"`        // Board name (default: "board1")
	Org         string `json:"org"`          // Org namespace (default from platform)
	Platform    string `json:"platform"`     // Platform ID (default from platform)
	MaxEntities int    `json:"max_entities"` // Max entities per query (default: 1000)
}

// Service provides domain REST endpoints for the Semdragons game world.
type Service struct {
	*service.BaseService
	graph  *semdragons.GraphClient
	world  *dmworldstate.WorldStateAggregator
	config Config
	logger *slog.Logger
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
		return nil, fmt.Errorf("semdragons-api requires NATS client")
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
	logger = logger.With("service", "semdragons-api")

	boardConfig := &semdragons.BoardConfig{
		Org:      org,
		Platform: platform,
		Board:    cfg.Board,
	}

	graph := semdragons.NewGraphClient(deps.NATSClient, boardConfig)
	world := dmworldstate.NewWorldStateAggregator(graph, cfg.MaxEntities, logger)

	baseService := service.NewBaseServiceWithOptions(
		"semdragons-api",
		nil,
		service.WithLogger(logger),
		service.WithMetrics(deps.MetricsRegistry),
		service.WithNATS(deps.NATSClient),
	)

	return &Service{
		BaseService: baseService,
		graph:       graph,
		world:       world,
		config:      cfg,
		logger:      logger,
	}, nil
}

// Start starts the API service.
func (s *Service) Start(ctx context.Context) error {
	s.SetHealthCheck(func() error {
		return nil
	})

	if err := s.BaseService.Start(ctx); err != nil {
		return err
	}

	s.logger.Info("Semdragons API service started",
		"board", s.config.Board,
		"max_entities", s.config.MaxEntities)
	return nil
}

// Stop stops the API service.
func (s *Service) Stop(timeout time.Duration) error {
	s.logger.Info("Semdragons API service stopping")
	return s.BaseService.Stop(timeout)
}

// RegisterHTTPHandlers registers domain REST endpoints with the HTTP mux.
func (s *Service) RegisterHTTPHandlers(prefix string, mux *http.ServeMux) {
	// CORS middleware wrapper
	cors := func(handler http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}

			handler(w, r)
		}
	}

	// World state
	mux.HandleFunc("GET "+prefix+"world", cors(s.handleWorldState))

	// Quests
	mux.HandleFunc("GET "+prefix+"quests", cors(s.handleListQuests))
	mux.HandleFunc("GET "+prefix+"quests/{id}", cors(s.handleGetQuest))
	mux.HandleFunc("POST "+prefix+"quests", cors(s.handleCreateQuest))

	// Agents
	mux.HandleFunc("GET "+prefix+"agents", cors(s.handleListAgents))
	mux.HandleFunc("GET "+prefix+"agents/{id}/inventory", cors(s.handleGetInventory))
	mux.HandleFunc("POST "+prefix+"agents/{id}/inventory/use", cors(s.handleUseConsumable))
	mux.HandleFunc("GET "+prefix+"agents/{id}/effects", cors(s.handleGetEffects))
	mux.HandleFunc("GET "+prefix+"agents/{id}", cors(s.handleGetAgent))
	mux.HandleFunc("POST "+prefix+"agents/{id}/retire", cors(s.handleRetireAgent))
	mux.HandleFunc("POST "+prefix+"agents", cors(s.handleRecruitAgent))

	// Battles
	mux.HandleFunc("GET "+prefix+"battles", cors(s.handleListBattles))
	mux.HandleFunc("GET "+prefix+"battles/{id}", cors(s.handleGetBattle))

	// Trajectories
	mux.HandleFunc("GET "+prefix+"trajectories/{id}", cors(s.handleGetTrajectory))

	// DM
	mux.HandleFunc("POST "+prefix+"dm/chat", cors(s.handleDMChat))
	mux.HandleFunc("POST "+prefix+"dm/intervene/{questId}", cors(s.handleDMIntervene))

	// Store
	mux.HandleFunc("GET "+prefix+"store", cors(s.handleListStore))
	mux.HandleFunc("GET "+prefix+"store/{id}", cors(s.handleGetStoreItem))
	mux.HandleFunc("POST "+prefix+"store/purchase", cors(s.handlePurchase))

	s.logger.Info("Semdragons API HTTP handlers registered", "prefix", prefix)
}

// OpenAPISpec returns the OpenAPI specification for domain endpoints.
func (s *Service) OpenAPISpec() *service.OpenAPISpec {
	return semdragonsOpenAPISpec()
}
