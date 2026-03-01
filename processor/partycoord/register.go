package partycoord

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

// ComponentName is the registered name for this component.
const ComponentName = "partycoord"

// schema is the pre-computed configuration schema.
var schema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// Factory creates a new PartyCoord component from configuration.
func Factory(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	// Start with defaults
	config := DefaultConfig()

	// Apply JSON config if provided
	if len(rawConfig) > 0 {
		if err := json.Unmarshal(rawConfig, &config); err != nil {
			return nil, errs.Wrap(err, "partycoord", "Factory", "unmarshal config")
		}
	}

	// Validate required dependencies
	if deps.NATSClient == nil {
		return nil, errors.New("NATS client required")
	}

	// Set up logger
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

// Register adds the PartyCoord component to the registry.
func Register(registry *component.Registry) error {
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        ComponentName,
		Factory:     Factory,
		Schema:      schema,
		Type:        "processor",
		Protocol:    "nats",
		Domain:      "coordination",
		Description: "Party coordination - formation, decomposition, rollup",
		Version:     "1.0.0",
	})
}

// NewFromConfig creates a component directly from a Config struct.
// This is useful for programmatic instantiation without going through the registry.
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
