package dmpartyformation

import (
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/errs"
)

// =============================================================================
// REGISTRATION - Factory and registry integration
// =============================================================================

// schema is the pre-computed configuration schema.
var schema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// Factory creates a new DM Party Formation component from configuration.
func Factory(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	config := DefaultConfig()

	if len(rawConfig) > 0 {
		if err := json.Unmarshal(rawConfig, &config); err != nil {
			return nil, errs.Wrap(err, "dm_partyformation", "Factory", "unmarshal config")
		}
	}

	if deps.NATSClient == nil {
		return nil, errors.New("NATS client required")
	}

	logger := deps.GetLoggerWithComponent(ComponentName)
	if logger == nil {
		logger = slog.Default()
	}

	return &Component{
		config: &config,
		deps:   deps,
		logger: logger,
	}, nil
}

// Register adds the DM Party Formation component to the registry.
func Register(registry *component.Registry) error {
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        ComponentName,
		Factory:     Factory,
		Schema:      schema,
		Type:        "processor",
		Protocol:    "nats",
		Domain:      "dm",
		Description: "Boids-based party formation for quests",
		Version:     "1.0.0",
	})
}

// NewFromConfig creates a component directly from a Config struct.
func NewFromConfig(config Config, deps component.Dependencies) (*Component, error) {
	if deps.NATSClient == nil {
		return nil, errors.New("NATS client required")
	}

	logger := deps.GetLoggerWithComponent(ComponentName)
	if logger == nil {
		logger = slog.Default()
	}

	return &Component{
		config: &config,
		deps:   deps,
		logger: logger,
	}, nil
}
