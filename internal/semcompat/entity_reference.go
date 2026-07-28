// Package semcompat contains temporary wire-only compatibility values that
// SemStreams does not yet export.
package semcompat

// DCTermsModified is the compatibility value for the standard
// dc.terms.modified predicate. SemStreams #673 owns the framework primitive:
// https://github.com/C360Studio/semstreams/issues/673
// Delete this constant when Semdragon adopts the first SemStreams release
// containing that primitive.
const DCTermsModified = "dc.terms.modified"
