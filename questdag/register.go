package questdag

import (
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/errs"
)

// schema is the pre-computed configuration schema.
var schema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// Factory creates a questdag component from raw JSON configuration.
func Factory(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	config := DefaultConfig()
	if len(rawConfig) > 0 {
		if err := json.Unmarshal(rawConfig, &config); err != nil {
			return nil, errs.Wrap(err, ComponentName, "Factory", "unmarshal config")
		}
	}
	if err := config.Validate(); err != nil {
		return nil, errs.Wrap(err, ComponentName, "Factory", "validate config")
	}
	if deps.NATSClient == nil {
		return nil, errors.New("questdag: NATS client required")
	}
	if deps.LifecycleManager == nil {
		return nil, errors.New("questdag: LifecycleManager required (gated-DAG executor Watch substrate)")
	}

	logger := deps.GetLoggerWithComponent(ComponentName)
	if logger == nil {
		logger = slog.Default()
	}
	return &Component{config: &config, deps: deps, logger: logger}, nil
}

// Register adds the questdag component to the registry.
func Register(registry *component.Registry) error {
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        ComponentName,
		Factory:     Factory,
		Schema:      schema,
		Type:        "processor",
		Protocol:    "nats",
		Domain:      "workflow",
		Description: "Party-DAG execution on the gated-DAG executor",
		Version:     "1.0.0",
	})
}

// NewFromConfig creates a questdag component directly from a Config struct.
func NewFromConfig(config Config, deps component.Dependencies) (*Component, error) {
	if err := config.Validate(); err != nil {
		return nil, errs.Wrap(err, ComponentName, "NewFromConfig", "validate config")
	}
	if deps.NATSClient == nil {
		return nil, errors.New("questdag: NATS client required")
	}
	if deps.LifecycleManager == nil {
		return nil, errors.New("questdag: LifecycleManager required (gated-DAG executor Watch substrate)")
	}
	logger := deps.GetLoggerWithComponent(ComponentName)
	if logger == nil {
		logger = slog.Default()
	}
	return &Component{config: &config, deps: deps, logger: logger}, nil
}
