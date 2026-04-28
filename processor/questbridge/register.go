package questbridge

import (
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/domains"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/pkg/errs"
)

// ComponentName is the registered name for this component.
const ComponentName = "questbridge"

// schema is the pre-computed configuration schema.
var schema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// Factory creates a new QuestBridge component from raw JSON configuration.
func Factory(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	config := DefaultConfig()

	if len(rawConfig) > 0 {
		if err := json.Unmarshal(rawConfig, &config); err != nil {
			return nil, errs.Wrap(err, "questbridge", "Factory", "unmarshal config")
		}
	}

	// Resolve domain catalog from config
	if config.Domain != "" {
		config.DomainCatalog = domains.GetCatalog(domain.ID(config.Domain))
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

// Register adds the QuestBridge component to the registry.
func Register(registry *component.Registry) error {
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        ComponentName,
		Factory:     Factory,
		Schema:      schema,
		Type:        "processor",
		Protocol:    "nats",
		Domain:      "execution",
		Description: "Bridges quest lifecycle to agentic loop execution",
		Version:     "1.0.0",
	})
}

// NewFromConfig creates a Component directly from a Config struct.
// Useful in tests and when constructing the component without the registry.
func NewFromConfig(config Config, deps component.Dependencies) (*Component, error) {
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
