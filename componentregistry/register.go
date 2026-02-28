// Package componentregistry provides registration for all semdragons components.
// This is the central point for registering all processor and gateway components
// with the semstreams component registry.
package componentregistry

import (
	"github.com/c360studio/semstreams/component"

	"github.com/c360studio/semdragons/gateway/api"
	"github.com/c360studio/semdragons/processor/boidengine"
	"github.com/c360studio/semdragons/processor/bossbattle"
	"github.com/c360studio/semdragons/processor/guildformation"
	"github.com/c360studio/semdragons/processor/questboard"
	"github.com/c360studio/semdragons/processor/xpengine"
)

// RegisterAll registers all semdragons components with the given registry.
// This is the main entry point for component registration.
func RegisterAll(registry *component.Registry) error {
	// Register all processor components
	processors := []func(*component.Registry) error{
		questboard.Register,
		xpengine.Register,
		bossbattle.Register,
		boidengine.Register,
		guildformation.Register,
	}

	for _, register := range processors {
		if err := register(registry); err != nil {
			return err
		}
	}

	// Register gateway components
	gateways := []func(*component.Registry) error{
		api.Register,
	}

	for _, register := range gateways {
		if err := register(registry); err != nil {
			return err
		}
	}

	return nil
}

// RegisterProcessors registers only the processor components.
// Use this if you want to register processors without gateways.
func RegisterProcessors(registry *component.Registry) error {
	processors := []func(*component.Registry) error{
		questboard.Register,
		xpengine.Register,
		bossbattle.Register,
		boidengine.Register,
		guildformation.Register,
	}

	for _, register := range processors {
		if err := register(registry); err != nil {
			return err
		}
	}

	return nil
}

// ComponentNames returns the names of all registered components.
func ComponentNames() []string {
	return []string{
		questboard.ComponentName,
		xpengine.ComponentName,
		bossbattle.ComponentName,
		boidengine.ComponentName,
		guildformation.ComponentName,
		api.ComponentName,
	}
}

// ProcessorNames returns the names of processor components.
func ProcessorNames() []string {
	return []string{
		questboard.ComponentName,
		xpengine.ComponentName,
		bossbattle.ComponentName,
		boidengine.ComponentName,
		guildformation.ComponentName,
	}
}

// GatewayNames returns the names of gateway components.
func GatewayNames() []string {
	return []string{
		api.ComponentName,
	}
}
