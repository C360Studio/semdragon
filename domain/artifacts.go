package domain

import (
	"log/slog"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/storage"
)

// ArtifactStoreProvider is satisfied by the filestore component.
// Any component that wraps a storage.Store and wants to expose it
// through the ComponentRegistry should implement this interface.
type ArtifactStoreProvider interface {
	ArtifactStore() storage.Store
}

// ResolveArtifactStore performs a lazy lookup of the "filestore" component
// via the ComponentRegistry and returns its storage.Store.
// Returns nil when the registry is nil, the component is absent, or the
// component does not implement ArtifactStoreProvider.
func ResolveArtifactStore(registry component.Lookup, logger *slog.Logger) storage.Store {
	if registry == nil {
		return nil
	}
	comp := registry.Component("filestore")
	if comp == nil {
		return nil
	}
	provider, ok := comp.(ArtifactStoreProvider)
	if !ok {
		if logger != nil {
			logger.Warn("filestore component does not implement ArtifactStoreProvider",
				"type", comp.Meta().Type)
		}
		return nil
	}
	return provider.ArtifactStore()
}
