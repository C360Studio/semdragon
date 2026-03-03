package semdragons

import (
	"github.com/c360studio/semdragons/domain"
)

// RegisterVocabulary registers all semdragons predicates with the vocabulary system.
// Delegates to domain.RegisterVocabulary() which is the single source of truth.
func RegisterVocabulary() {
	domain.RegisterVocabulary()
}
