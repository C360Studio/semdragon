package filestore

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/storage"
)

// filestoreSchema is the generated configuration schema for the filestore component.
var filestoreSchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// Component is the semstreams component wrapper around a filesystem-backed Store.
// It implements component.Discoverable and exposes GetStore for consumers that
// need direct access to the storage.Store interface.
type Component struct {
	name   string
	config Config
	store  *Store

	logger   *slog.Logger
	platform component.PlatformMeta

	running bool
	mu      sync.RWMutex
}

// NewComponent constructs a filestore Component from raw JSON config and the
// standard component.Dependencies bag. It creates the underlying *Store
// immediately so the store is available via GetStore after construction.
func NewComponent(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	cfg := DefaultConfig()
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		return nil, fmt.Errorf("filestore: unmarshal config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("filestore: invalid config: %w", err)
	}

	s, err := New(cfg.RootDir, cfg.CreateIfMissing)
	if err != nil {
		return nil, fmt.Errorf("filestore: create store: %w", err)
	}

	return &Component{
		name:     "filestore",
		config:   cfg,
		store:    s,
		logger:   deps.GetLogger(),
		platform: deps.Platform,
	}, nil
}

// GetStore returns the underlying *Store so that other components can obtain
// the store directly without going through the component registry.
func (c *Component) GetStore() *Store {
	return c.store
}

// ArtifactStore implements domain.ArtifactStoreProvider so the component
// registry can resolve the filestore for artifact operations.
func (c *Component) ArtifactStore() storage.Store {
	return c.store
}

// Initialize is a no-op; all setup happens in NewComponent.
func (c *Component) Initialize() error {
	return nil
}

// Start marks the component as running.
func (c *Component) Start(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return fmt.Errorf("filestore component already running")
	}
	c.running = true

	c.logger.Info("filestore component started", "root_dir", c.config.RootDir)
	return nil
}

// Stop marks the component as stopped and closes the underlying store.
func (c *Component) Stop(_ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}
	c.running = false

	if err := c.store.Close(); err != nil {
		c.logger.Warn("filestore: close store returned error", "error", err)
	}

	c.logger.Info("filestore component stopped", "root_dir", c.config.RootDir)
	return nil
}

// Meta returns static component metadata.
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "filestore",
		Type:        "storage",
		Description: "Local filesystem storage backend for quest artifacts",
		Version:     "0.1.0",
	}
}

// InputPorts returns an empty slice — filestore is a storage backend, not a
// data-flow processor.
func (c *Component) InputPorts() []component.Port {
	return []component.Port{}
}

// OutputPorts returns an empty slice.
func (c *Component) OutputPorts() []component.Port {
	return []component.Port{}
}

// ConfigSchema returns the generated schema for Config.
func (c *Component) ConfigSchema() component.ConfigSchema {
	return filestoreSchema
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

// DataFlow returns zero-value metrics — filestore does not measure throughput.
func (c *Component) DataFlow() component.FlowMetrics {
	return component.FlowMetrics{}
}
