// Package semcompat contains temporary wire-only compatibility values needed
// while Semdragons remains pinned below the SemStreams beta.158 contract.
package semcompat

// EntityReferenceDatatype is the exact beta.158 wire value for an explicit
// entity-reference object. Issue #4 MUST delete this package and replace every
// use with message.EntityReferenceDatatype immediately after the beta.158 bump.
const EntityReferenceDatatype = "@id"
