package redteam

import (
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/errs"
)

// ComponentName is the registered name for this component.
const ComponentName = "redteam"

// schema is the pre-computed configuration schema.
var schema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// Factory creates a new RedTeam component from configuration.
func Factory(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	config := DefaultConfig()

	if len(rawConfig) > 0 {
		if err := json.Unmarshal(rawConfig, &config); err != nil {
			return nil, errs.Wrap(err, "redteam", "Factory", "unmarshal config")
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

// Register adds the RedTeam component to the registry.
func Register(registry *component.Registry) error {
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        ComponentName,
		Factory:     Factory,
		Schema:      schema,
		Type:        "processor",
		Protocol:    "nats",
		Domain:      "workflow",
		Description: "Red-team review: posts adversarial review quests for submitted work",
		Version:     "1.0.0",
	})
}
