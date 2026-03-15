package workspacerepo

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"time"

	"github.com/c360studio/semstreams/component"
)

// workspacerepoSchema is the generated configuration schema.
var workspacerepoSchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// Component is the semstreams component wrapper around WorkspaceRepo.
// It implements component.Discoverable and exposes WorkspaceRepo for
// consumers that need direct access via the component registry.
type Component struct {
	name   string
	config Config
	repo   *WorkspaceRepo

	logger   *slog.Logger
	platform component.PlatformMeta

	running bool
	mu      sync.RWMutex
}

// NewComponent constructs a workspacerepo Component from raw JSON config and
// the standard component.Dependencies bag.
func NewComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	cfg := DefaultConfig()
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		return nil, fmt.Errorf("workspacerepo: unmarshal config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("workspacerepo: invalid config: %w", err)
	}

	logger := deps.GetLogger()
	repo := New(cfg.RepoDir, cfg.WorktreesDir, logger)

	return &Component{
		name:     "workspacerepo",
		config:   cfg,
		repo:     repo,
		logger:   logger,
		platform: deps.Platform,
	}, nil
}

// WorkspaceRepo returns the underlying *WorkspaceRepo so that other
// components can perform worktree operations.
func (c *Component) WorkspaceRepo() *WorkspaceRepo {
	return c.repo
}

// Initialize is a no-op; bare repo initialization happens in Start.
func (c *Component) Initialize() error {
	return nil
}

// Start initializes the bare repository if needed and marks the component
// as running.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return fmt.Errorf("workspacerepo component already running")
	}

	if err := c.repo.Init(ctx); err != nil {
		return fmt.Errorf("workspacerepo: init bare repo: %w", err)
	}

	c.running = true
	c.logger.Info("workspacerepo component started",
		"repo_dir", c.config.RepoDir,
		"worktrees_dir", c.config.WorktreesDir)
	return nil
}

// Stop marks the component as stopped. The timeout parameter is unused because
// this component has no background goroutines to drain — it is a pure storage
// wrapper with synchronous operations.
func (c *Component) Stop(_ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}
	c.running = false

	c.logger.Info("workspacerepo component stopped")
	return nil
}

// Meta returns static component metadata.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "workspacerepo",
		Type:        "storage",
		Description: "Git-backed workspace with per-quest worktrees for artifact management",
		Version:     "0.1.0",
	}
}

// InputPorts returns an empty slice — workspacerepo is a storage backend.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{}
}

// OutputPorts returns an empty slice.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{}
}

// ConfigSchema returns the generated schema for Config.
func (c *Component) ConfigSchema() component.ConfigSchema {
	return workspacerepoSchema
}

// Health reports whether the component is running.
func (c *Component) Health() component.HealthStatus {
	c.mu.RLock()
	running := c.running
	c.mu.RUnlock()

	status := "stopped"
	if running {
		status = "running"
	}

	return component.HealthStatus{
		Healthy:   running,
		LastCheck: time.Now(),
		Status:    status,
	}
}

// DataFlow returns zero-value metrics — workspacerepo does not measure throughput.
func (c *Component) DataFlow() component.FlowMetrics {
	return component.FlowMetrics{}
}
