// Package semcompat contains temporary wire-only compatibility values needed
// while Semdragons remains pinned below the SemStreams beta.158 contract.
package semcompat

// EntityReferenceDatatype is the exact beta.158 wire value for an explicit
// entity-reference object. Issue #4 MUST delete this package and replace every
// use with message.EntityReferenceDatatype immediately after the beta.158 bump.
const EntityReferenceDatatype = "@id"

// These gated-DAG predicates are the SemStreams beta.158 config defaults.
// Semdragons is still compiled against beta.135, whose gated-DAG package does
// not export them, so issue #4 MUST replace these compatibility values with
// the framework exports when the dependency is upgraded.
const (
	GatedDAGClaimPredicate     = "gateddag.unit.claim"
	GatedDAGCompletedPredicate = "gateddag.unit.completed"
	GatedDAGFailedPredicate    = "gateddag.unit.failed"
	GatedDAGDirtiedPredicate   = "gateddag.unit.dirtied"
	GatedDAGDependsOnPredicate = "gateddag.unit.depends-on"
)
