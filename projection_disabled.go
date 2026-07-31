package semdragons

import (
	"context"
	"fmt"

	"github.com/c360studio/semstreams/pkg/projection"
)

// DisabledProjectionMutation is the typed fail-closed result used by product
// surfaces that are disabled until a least-privilege SemStreams owner adapter
// is bound. It performs no I/O and is not a storage compatibility path.
func DisabledProjectionMutation(
	_ context.Context,
	_ any,
	capability string,
	_ ...uint64,
) error {
	return &projection.MutationError{
		Kind:   projection.MutationUnavailable,
		Commit: projection.CommitNotCommitted,
		Err:    fmt.Errorf("projection owner adapter is not bound for %s", capability),
	}
}
