package workspacerepo

import (
	"fmt"

	"github.com/c360studio/semstreams/component"
)

// ComponentName is the registered name for the workspace repo component.
const ComponentName = "workspacerepo"

// RegistryInterface defines the minimal interface required to register a
// component factory.
type RegistryInterface interface {
	RegisterWithConfig(component.RegistrationConfig) error
}

// Register registers the workspacerepo storage component with the given registry.
func Register(registry RegistryInterface) error {
	if registry == nil {
		return fmt.Errorf("registry cannot be nil")
	}
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        ComponentName,
		Factory:     NewComponent,
		Schema:      workspacerepoSchema,
		Type:        "storage",
		Protocol:    "git",
		Domain:      "semdragons",
		Description: "Git-backed workspace with per-quest worktrees for artifact management",
		Version:     "0.1.0",
	})
}
