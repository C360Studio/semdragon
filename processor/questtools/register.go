package questtools

import (
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/errs"
)

// ComponentName is the registered name for this component in the semstreams registry.
const ComponentName = "questtools"

// schema is the pre-computed configuration schema, derived from struct tags at init time.
var schema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// Factory creates a new questtools Component from raw JSON configuration and dependencies.
// It follows the standard semstreams factory pattern: parse config, validate deps, construct.
func Factory(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	config := DefaultConfig()

	if len(rawConfig) > 0 {
		if err := json.Unmarshal(rawConfig, &config); err != nil {
			return nil, errs.Wrap(err, "questtools", "Factory", "unmarshal config")
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

// Register adds the questtools component to the semstreams component registry.
func Register(registry *component.Registry) error {
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        ComponentName,
		Factory:     Factory,
		Schema:      schema,
		Type:        "processor",
		Protocol:    "nats",
		Domain:      "execution",
		Description: "Tool execution gateway with tier/skill authorization",
		Version:     "1.0.0",
	})
}

// NewFromConfig creates a Component directly from a typed Config struct.
// Useful for tests and programmatic construction without the registry.
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
