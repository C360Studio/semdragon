// Package api provides an HTTP API gateway for semdragons.
// It exposes REST endpoints for quest management, agent operations,
// and real-time event streaming via WebSocket.
//
// The gateway can operate in two modes:
//   - Standalone: Runs its own HTTP server (for local development)
//   - Integrated: Registers handlers with ServiceManager's HTTP server
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/gateway"

	semdragons "github.com/c360studio/semdragons"
)

// =============================================================================
// COMPONENT - API Gateway as native semstreams component
// =============================================================================
// Implements:
//   - component.Discoverable
//   - component.LifecycleComponent
//   - gateway.HTTPHandler (for ServiceManager integration)
//
// Provides HTTP REST endpoints with direct KV storage access.
// =============================================================================

// Mode determines how the gateway operates.
type Mode string

const (
	// ModeStandalone runs the gateway with its own HTTP server.
	// Use for local development or when not using ServiceManager.
	ModeStandalone Mode = "standalone"

	// ModeIntegrated registers handlers with ServiceManager's HTTP server.
	// Use when deployed as part of a semstreams flow.
	ModeIntegrated Mode = "integrated"
)

// Config holds the component configuration.
type Config struct {
	// BoardConfig contains org, platform, board for entity IDs and bucket naming.
	Org      string `json:"org" schema:"type:string,description:Organization namespace"`
	Platform string `json:"platform" schema:"type:string,description:Platform/environment name"`
	Board    string `json:"board" schema:"type:string,description:Quest board name"`

	// Mode determines standalone vs integrated operation.
	Mode Mode `json:"mode" schema:"type:string,description:Gateway mode (standalone or integrated)"`

	// HTTP server settings (only used in standalone mode)
	ListenAddr     string        `json:"listen_addr" schema:"type:string,description:HTTP listen address (standalone mode)"`
	ReadTimeout    time.Duration `json:"read_timeout" schema:"type:duration,description:HTTP read timeout"`
	WriteTimeout   time.Duration `json:"write_timeout" schema:"type:duration,description:HTTP write timeout"`
	MaxHeaderBytes int           `json:"max_header_bytes" schema:"type:int,description:Max HTTP header bytes"`

	// API settings
	EnableCORS  bool     `json:"enable_cors" schema:"type:bool,description:Enable CORS headers"`
	CORSOrigins []string `json:"cors_origins" schema:"type:array,description:Allowed CORS origins"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Org:            "default",
		Platform:       "local",
		Board:          "main",
		Mode:           ModeStandalone,
		ListenAddr:     ":8080",
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB
		EnableCORS:     true,
		CORSOrigins:    []string{"*"},
	}
}

// ToBoardConfig converts component config to semdragons BoardConfig.
func (c *Config) ToBoardConfig() *semdragons.BoardConfig {
	return &semdragons.BoardConfig{
		Org:      c.Org,
		Platform: c.Platform,
		Board:    c.Board,
	}
}

// Component implements the API Gateway as a semstreams component.
type Component struct {
	config      *Config
	deps        component.Dependencies
	graph       *semdragons.GraphClient
	logger      *slog.Logger
	boardConfig *semdragons.BoardConfig

	// HTTP server (standalone mode only)
	server *http.Server

	// Route prefix for integrated mode
	routePrefix string

	// Internal state
	running  atomic.Bool
	mu       sync.RWMutex
	stopChan chan struct{}

	// Metrics
	requestsHandled atomic.Uint64
	errorsCount     atomic.Int64
	lastActivity    atomic.Value // time.Time
	startTime       time.Time
}

// Ensure Component implements the required interfaces.
var (
	_ component.Discoverable       = (*Component)(nil)
	_ component.LifecycleComponent = (*Component)(nil)
	_ gateway.HTTPHandler          = (*Component)(nil)
)

// =============================================================================
// DISCOVERABLE INTERFACE
// =============================================================================

// Meta returns basic component information.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "api",
		Type:        "gateway",
		Description: "HTTP API gateway for semdragons with direct KV access",
		Version:     "1.0.0",
	}
}

// InputPorts returns the ports this component accepts data on.
// The HTTP gateway handles incoming requests directly via HTTP,
// so it doesn't have traditional semstreams input ports.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "http",
			Direction:   component.DirectionInput,
			Required:    true,
			Description: "HTTP endpoint for REST API",
			Config: &component.NetworkPort{
				Protocol: "http",
				Host:     "0.0.0.0",
				Port:     8080,
			},
		},
	}
}

// OutputPorts returns the ports this component produces data on.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{
		{
			Name:        "events",
			Direction:   component.DirectionOutput,
			Required:    false,
			Description: "WebSocket events for real-time updates",
			Config: &component.NATSPort{
				Subject: "api.events.>",
			},
		},
	}
}

// ConfigSchema returns the configuration schema for this component.
func (c *Component) ConfigSchema() component.ConfigSchema {
	return component.ConfigSchema{
		Properties: map[string]component.PropertySchema{
			"org": {
				Type:        "string",
				Description: "Organization namespace",
				Default:     "default",
				Category:    "basic",
			},
			"platform": {
				Type:        "string",
				Description: "Platform/environment name",
				Default:     "local",
				Category:    "basic",
			},
			"board": {
				Type:        "string",
				Description: "Quest board name",
				Default:     "main",
				Category:    "basic",
			},
			"mode": {
				Type:        "string",
				Description: "Gateway mode: standalone (own server) or integrated (ServiceManager)",
				Default:     "standalone",
				Category:    "http",
			},
			"listen_addr": {
				Type:        "string",
				Description: "HTTP listen address (standalone mode only)",
				Default:     ":8080",
				Category:    "http",
			},
			"enable_cors": {
				Type:        "bool",
				Description: "Enable CORS headers",
				Default:     true,
				Category:    "http",
			},
		},
		Required: []string{"org", "platform", "board"},
	}
}

// Health returns current health status.
func (c *Component) Health() component.HealthStatus {
	status := component.HealthStatus{
		Healthy:    c.running.Load(),
		LastCheck:  time.Now(),
		ErrorCount: int(c.errorsCount.Load()),
		Uptime:     time.Since(c.startTime),
	}

	if c.running.Load() {
		status.Status = "running"
	} else {
		status.Status = "stopped"
	}

	if c.errorsCount.Load() > 0 {
		status.LastError = "errors encountered handling requests"
	}

	return status
}

// DataFlow returns current data flow metrics.
func (c *Component) DataFlow() component.FlowMetrics {
	metrics := component.FlowMetrics{
		MessagesPerSecond: 0,
		BytesPerSecond:    0,
		ErrorRate:         0,
	}

	if lastTime, ok := c.lastActivity.Load().(time.Time); ok {
		metrics.LastActivity = lastTime
	}

	requests := c.requestsHandled.Load()
	uptime := time.Since(c.startTime).Seconds()
	if uptime > 0 {
		metrics.MessagesPerSecond = float64(requests) / uptime
	}

	if requests > 0 {
		metrics.ErrorRate = float64(c.errorsCount.Load()) / float64(requests)
	}

	return metrics
}

// =============================================================================
// LIFECYCLE INTERFACE
// =============================================================================

// Initialize performs one-time setup. No I/O operations here.
func (c *Component) Initialize() error {
	if c.config == nil {
		return errors.New("config not set")
	}

	if c.deps.NATSClient == nil {
		return errors.New("NATS client required")
	}

	c.boardConfig = c.config.ToBoardConfig()
	c.stopChan = make(chan struct{})

	return nil
}

// Start begins component operation with the given context.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running.Load() {
		return errors.New("component already running")
	}

	// Create graph client
	c.graph = semdragons.NewGraphClient(c.deps.NATSClient, c.boardConfig)

	c.startTime = time.Now()
	c.running.Store(true)
	c.lastActivity.Store(time.Now())

	// In standalone mode, start our own HTTP server
	if c.config.Mode == ModeStandalone {
		mux := http.NewServeMux()
		c.RegisterHTTPHandlers("", mux)

		c.server = &http.Server{
			Addr:           c.config.ListenAddr,
			Handler:        mux,
			ReadTimeout:    c.config.ReadTimeout,
			WriteTimeout:   c.config.WriteTimeout,
			MaxHeaderBytes: c.config.MaxHeaderBytes,
		}

		go func() {
			c.logger.Info("starting standalone HTTP server", "addr", c.config.ListenAddr)
			if err := c.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				c.logger.Error("HTTP server error", "error", err)
				c.errorsCount.Add(1)
			}
		}()
	}

	c.logger.Info("api gateway started",
		"mode", c.config.Mode,
		"org", c.config.Org,
		"platform", c.config.Platform,
		"board", c.config.Board)

	return nil
}

// Stop gracefully shuts down the component.
func (c *Component) Stop(timeout time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running.Load() {
		return nil
	}

	close(c.stopChan)

	// Shutdown standalone HTTP server if running
	if c.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		if err := c.server.Shutdown(ctx); err != nil {
			c.logger.Error("HTTP server shutdown error", "error", err)
		}
	}

	c.running.Store(false)
	c.logger.Info("api gateway stopped")

	return nil
}

// =============================================================================
// HTTP HANDLER INTERFACE (for ServiceManager integration)
// =============================================================================

// RegisterHTTPHandlers registers the gateway's HTTP routes with an HTTP mux.
// This implements the gateway.HTTPHandler interface for ServiceManager integration.
//
// The prefix parameter is the URL path prefix for this gateway instance,
// typically derived from the component instance name (e.g., "/semdragons/").
func (c *Component) RegisterHTTPHandlers(prefix string, mux *http.ServeMux) {
	// Normalize prefix
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}
	c.routePrefix = prefix

	// Health endpoints (at root, not under prefix)
	mux.HandleFunc(prefix+"health", c.handleHealth)
	mux.HandleFunc(prefix+"ready", c.handleReady)

	// API v1 routes
	apiPrefix := prefix + "api/v1/"
	mux.HandleFunc(apiPrefix+"quests", c.handleQuests)
	mux.HandleFunc(apiPrefix+"quests/", c.handleQuestByID)
	mux.HandleFunc(apiPrefix+"agents", c.handleAgents)
	mux.HandleFunc(apiPrefix+"agents/", c.handleAgentByID)
	mux.HandleFunc(apiPrefix+"guilds", c.handleGuilds)
	mux.HandleFunc(apiPrefix+"guilds/", c.handleGuildByID)
	mux.HandleFunc(apiPrefix+"stats", c.handleStats)

	c.logger.Debug("registered HTTP handlers",
		"prefix", prefix,
		"routes", []string{
			apiPrefix + "quests",
			apiPrefix + "agents",
			apiPrefix + "guilds",
			apiPrefix + "stats",
		})
}

// =============================================================================
// HANDLERS
// =============================================================================

func (c *Component) handleHealth(w http.ResponseWriter, r *http.Request) {
	c.recordRequest()
	c.applyCORS(w, r)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	c.jsonResponse(w, http.StatusOK, map[string]string{
		"status": "healthy",
	})
}

func (c *Component) handleReady(w http.ResponseWriter, r *http.Request) {
	c.recordRequest()
	c.applyCORS(w, r)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if !c.running.Load() {
		c.jsonError(w, http.StatusServiceUnavailable, "service not ready")
		return
	}

	c.jsonResponse(w, http.StatusOK, map[string]string{
		"status": "ready",
	})
}

func (c *Component) handleQuests(w http.ResponseWriter, r *http.Request) {
	c.recordRequest()
	c.applyCORS(w, r)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		// List quests by status (optional filter)
		statusFilter := r.URL.Query().Get("status")

		// Get all quests using prefix query
		entities, err := c.graph.ListQuestsByPrefix(ctx, 100)
		if err != nil {
			c.errorsCount.Add(1)
			c.jsonError(w, http.StatusInternalServerError, "failed to list quests")
			return
		}

		quests := make([]*semdragons.Quest, 0, len(entities))
		for i := range entities {
			quest := semdragons.QuestFromEntityState(&entities[i])
			if quest == nil {
				continue
			}
			// Filter by status if specified
			if statusFilter != "" && string(quest.Status) != statusFilter {
				continue
			}
			quests = append(quests, quest)
		}

		c.jsonResponse(w, http.StatusOK, quests)

	default:
		c.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (c *Component) handleQuestByID(w http.ResponseWriter, r *http.Request) {
	c.recordRequest()
	c.applyCORS(w, r)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	ctx := r.Context()

	// Extract quest ID from path, accounting for prefix
	pathPrefix := c.routePrefix + "api/v1/quests/"
	questID := strings.TrimPrefix(r.URL.Path, pathPrefix)
	if questID == "" || questID == r.URL.Path {
		c.jsonError(w, http.StatusBadRequest, "quest ID required")
		return
	}

	instance := semdragons.ExtractInstance(questID)

	switch r.Method {
	case http.MethodGet:
		entity, err := c.graph.GetQuest(ctx, semdragons.QuestID(instance))
		if err != nil {
			c.errorsCount.Add(1)
			c.jsonError(w, http.StatusNotFound, "quest not found")
			return
		}
		quest := semdragons.QuestFromEntityState(entity)
		if quest == nil {
			c.errorsCount.Add(1)
			c.jsonError(w, http.StatusInternalServerError, "failed to reconstruct quest")
			return
		}
		c.jsonResponse(w, http.StatusOK, quest)

	default:
		c.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (c *Component) handleAgents(w http.ResponseWriter, r *http.Request) {
	c.recordRequest()
	c.applyCORS(w, r)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		// Get all agents using prefix query
		entities, err := c.graph.ListAgentsByPrefix(ctx, 100)
		if err != nil {
			c.errorsCount.Add(1)
			c.jsonError(w, http.StatusInternalServerError, "failed to list agents")
			return
		}

		agents := make([]*semdragons.Agent, 0, len(entities))
		for i := range entities {
			agent := semdragons.AgentFromEntityState(&entities[i])
			if agent == nil {
				continue
			}
			agents = append(agents, agent)
		}

		c.jsonResponse(w, http.StatusOK, agents)

	default:
		c.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (c *Component) handleAgentByID(w http.ResponseWriter, r *http.Request) {
	c.recordRequest()
	c.applyCORS(w, r)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	ctx := r.Context()

	// Extract agent ID from path, accounting for prefix
	pathPrefix := c.routePrefix + "api/v1/agents/"
	agentID := strings.TrimPrefix(r.URL.Path, pathPrefix)
	if agentID == "" || agentID == r.URL.Path {
		c.jsonError(w, http.StatusBadRequest, "agent ID required")
		return
	}

	instance := semdragons.ExtractInstance(agentID)

	switch r.Method {
	case http.MethodGet:
		entity, err := c.graph.GetAgent(ctx, semdragons.AgentID(instance))
		if err != nil {
			c.errorsCount.Add(1)
			c.jsonError(w, http.StatusNotFound, "agent not found")
			return
		}
		agent := semdragons.AgentFromEntityState(entity)
		if agent == nil {
			c.errorsCount.Add(1)
			c.jsonError(w, http.StatusInternalServerError, "failed to reconstruct agent")
			return
		}
		c.jsonResponse(w, http.StatusOK, agent)

	default:
		c.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (c *Component) handleGuilds(w http.ResponseWriter, r *http.Request) {
	c.recordRequest()
	c.applyCORS(w, r)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		// Get all guilds using prefix query
		entities, err := c.graph.ListGuildsByPrefix(ctx, 100)
		if err != nil {
			c.errorsCount.Add(1)
			c.jsonError(w, http.StatusInternalServerError, "failed to list guilds")
			return
		}

		guilds := make([]*semdragons.Guild, 0, len(entities))
		for i := range entities {
			guild := semdragons.GuildFromEntityState(&entities[i])
			if guild == nil {
				continue
			}
			guilds = append(guilds, guild)
		}

		c.jsonResponse(w, http.StatusOK, guilds)

	default:
		c.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (c *Component) handleGuildByID(w http.ResponseWriter, r *http.Request) {
	c.recordRequest()
	c.applyCORS(w, r)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	ctx := r.Context()

	// Extract guild ID from path, accounting for prefix
	pathPrefix := c.routePrefix + "api/v1/guilds/"
	guildID := strings.TrimPrefix(r.URL.Path, pathPrefix)
	if guildID == "" || guildID == r.URL.Path {
		c.jsonError(w, http.StatusBadRequest, "guild ID required")
		return
	}

	instance := semdragons.ExtractInstance(guildID)

	switch r.Method {
	case http.MethodGet:
		entity, err := c.graph.GetGuild(ctx, semdragons.GuildID(instance))
		if err != nil {
			c.errorsCount.Add(1)
			c.jsonError(w, http.StatusNotFound, "guild not found")
			return
		}
		guild := semdragons.GuildFromEntityState(entity)
		if guild == nil {
			c.errorsCount.Add(1)
			c.jsonError(w, http.StatusInternalServerError, "failed to reconstruct guild")
			return
		}
		c.jsonResponse(w, http.StatusOK, guild)

	default:
		c.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (c *Component) handleStats(w http.ResponseWriter, r *http.Request) {
	c.recordRequest()
	c.applyCORS(w, r)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	ctx := r.Context()

	if r.Method != http.MethodGet {
		c.jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// TODO: Implement GetBoardStats once available in GraphClient
	// For now, compute stats from entity queries
	stats := &semdragons.BoardStats{
		ByDifficulty: make(map[semdragons.QuestDifficulty]int),
		BySkill:      make(map[semdragons.SkillTag]int),
	}

	// Get quest counts by status
	questEntities, err := c.graph.ListQuestsByPrefix(ctx, 1000)
	if err == nil {
		for i := range questEntities {
			quest := semdragons.QuestFromEntityState(&questEntities[i])
			if quest != nil {
				switch quest.Status {
				case semdragons.QuestPosted:
					stats.TotalPosted++
				case semdragons.QuestClaimed:
					stats.TotalClaimed++
				case semdragons.QuestCompleted:
					stats.TotalCompleted++
				case semdragons.QuestFailed:
					stats.TotalFailed++
				}
				// Aggregate by difficulty
				stats.ByDifficulty[quest.Difficulty]++
				// Aggregate by skills
				for _, skill := range quest.RequiredSkills {
					stats.BySkill[skill]++
				}
			}
		}
	}

	c.jsonResponse(w, http.StatusOK, stats)
}

// =============================================================================
// HELPERS
// =============================================================================

func (c *Component) recordRequest() {
	c.requestsHandled.Add(1)
	c.lastActivity.Store(time.Now())
}

func (c *Component) applyCORS(w http.ResponseWriter, r *http.Request) {
	if !c.config.EnableCORS {
		return
	}

	origin := r.Header.Get("Origin")

	// Check if origin is allowed
	allowed := false
	for _, allowedOrigin := range c.config.CORSOrigins {
		if allowedOrigin == "*" || allowedOrigin == origin {
			allowed = true
			break
		}
	}

	if allowed {
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
		w.Header().Set("Access-Control-Max-Age", "3600")
	}
}

func (c *Component) jsonResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		c.logger.Error("failed to encode JSON response", "error", err)
	}
}

func (c *Component) jsonError(w http.ResponseWriter, status int, message string) {
	c.jsonResponse(w, status, map[string]any{
		"error":  message,
		"status": status,
	})
}

// Graph returns the underlying graph client for external access.
func (c *Component) Graph() *semdragons.GraphClient {
	return c.graph
}

// Stats returns API gateway statistics.
func (c *Component) Stats() Stats {
	return Stats{
		RequestsHandled: c.requestsHandled.Load(),
		Errors:          c.errorsCount.Load(),
		Uptime:          time.Since(c.startTime),
	}
}

// Stats holds API gateway statistics.
type Stats struct {
	RequestsHandled uint64        `json:"requests_handled"`
	Errors          int64         `json:"errors"`
	Uptime          time.Duration `json:"uptime"`
}

// ListenAddr returns the configured listen address.
func (c *Component) ListenAddr() string {
	return c.config.ListenAddr
}

// URL returns the full URL for the API gateway.
func (c *Component) URL() string {
	return fmt.Sprintf("http://%s", c.config.ListenAddr)
}
