package questdagexec

import (
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/pkg/errs"
)

// =============================================================================
// REGISTRATION - Factory and registry integration
// =============================================================================

// ComponentName is the registered name for this component.
const ComponentName = "questdagexec"

// schema is the pre-computed configuration schema.
var schema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// Factory creates a new questdagexec component from raw JSON configuration.
func Factory(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	// Start with defaults.
	config := DefaultConfig()

	// Apply JSON config if provided.
	if len(rawConfig) > 0 {
		if err := json.Unmarshal(rawConfig, &config); err != nil {
			return nil, errs.Wrap(err, ComponentName, "Factory", "unmarshal config")
		}
	}

	// Validate required fields.
	if err := config.Validate(); err != nil {
		return nil, errs.Wrap(err, ComponentName, "Factory", "validate config")
	}

	if deps.NATSClient == nil {
		return nil, errors.New("NATS client required")
	}
	if deps.PayloadRegistry == nil {
		return nil, errors.New("PayloadRegistry required (set via service.Dependencies.PayloadRegistry)")
	}

	logger := deps.GetLoggerWithComponent(ComponentName)
	if logger == nil {
		logger = slog.Default()
	}

	return &Component{
		config:  &config,
		deps:    deps,
		logger:  logger,
		decoder: message.NewDecoder(deps.PayloadRegistry),
	}, nil
}

// Register adds the questdagexec component to the registry.
func Register(registry *component.Registry) error {
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        ComponentName,
		Factory:     Factory,
		Schema:      schema,
		Type:        "processor",
		Protocol:    "nats",
		Domain:      "workflow",
		Description: "Reactive DAG execution for party quest decompositions",
		Version:     "1.0.0",
	})
}

// NewFromConfig creates a component directly from a Config struct.
// Useful for programmatic instantiation without going through the registry.
func NewFromConfig(config Config, deps component.Dependencies) (*Component, error) {
	if err := config.Validate(); err != nil {
		return nil, errs.Wrap(err, ComponentName, "NewFromConfig", "validate config")
	}
	if deps.NATSClient == nil {
		return nil, errors.New("NATS client required")
	}
	if deps.PayloadRegistry == nil {
		return nil, errors.New("PayloadRegistry required (set via service.Dependencies.PayloadRegistry)")
	}

	logger := deps.GetLoggerWithComponent(ComponentName)
	if logger == nil {
		logger = slog.Default()
	}

	return &Component{
		config:  &config,
		deps:    deps,
		logger:  logger,
		decoder: message.NewDecoder(deps.PayloadRegistry),
	}, nil
}
