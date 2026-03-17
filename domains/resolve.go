package domains

import (
	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/promptmanager"
)

// GetCatalog returns the DomainCatalog for the given domain ID,
// or nil if the domain is unknown.
func GetCatalog(id domain.ID) *promptmanager.DomainCatalog {
	switch id {
	case domain.DomainSoftware:
		return &SoftwarePromptCatalog
	case domain.DomainSoftwareRPG:
		return &SoftwareRPGPromptCatalog
	case domain.DomainDnD:
		return &DnDPromptCatalog
	case domain.DomainResearch:
		return &ResearchPromptCatalog
	default:
		return nil
	}
}
