package domain

import (
	"log/slog"

	"github.com/c360studio/semstreams/component"

	"github.com/c360studio/semdragons/storage/workspacerepo"
)

// WorkspaceRepoProvider is satisfied by the workspacerepo component.
// Any component that wraps a WorkspaceRepo and wants to expose it
// through the ComponentRegistry should implement this interface.
type WorkspaceRepoProvider interface {
	WorkspaceRepo() *workspacerepo.WorkspaceRepo
}

// ResolveWorkspaceRepo performs a lazy lookup of the "workspacerepo" component
// via the ComponentRegistry and returns its *WorkspaceRepo.
// Returns nil when the registry is nil, the component is absent, or the
// component does not implement WorkspaceRepoProvider.
func ResolveWorkspaceRepo(registry component.Lookup, logger *slog.Logger) *workspacerepo.WorkspaceRepo {
	if registry == nil {
		return nil
	}
	comp := registry.Component("workspacerepo")
	if comp == nil {
		return nil
	}
	provider, ok := comp.(WorkspaceRepoProvider)
	if !ok {
		if logger != nil {
			logger.Warn("workspacerepo component does not implement WorkspaceRepoProvider",
				"type", comp.Meta().Type)
		}
		return nil
	}
	return provider.WorkspaceRepo()
}
