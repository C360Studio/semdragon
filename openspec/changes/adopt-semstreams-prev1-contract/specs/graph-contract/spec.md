# graph-contract Specification

## Purpose

Define the canonical graph language and ownership boundary that every
Semdragons producer and consumer must use before adopting SemStreams beta.158.

## ADDED Requirements

### Requirement: Canonical predicates

The system SHALL store only predicates with exactly three dot-separated
lower-kebab segments in the form `domain.category.property`.

#### Scenario: Canonical predicate accepted

- **WHEN** a producer emits `quest.lifecycle.phase`
- **THEN** the predicate passes the SemStreams canonical validator
- **AND** its vocabulary and owner are declared

#### Scenario: Legacy predicate rejected

- **WHEN** a producer or configuration contains a two-part, four-part,
  underscore-segment, or dynamic-ID predicate
- **THEN** the contract audit fails before the producer is enabled

### Requirement: Dynamic identity is data, not vocabulary

The system SHALL NOT embed entity IDs, member IDs, node IDs, indices, or other
runtime values in predicate names.

#### Scenario: Multi-valued relationship

- **WHEN** an entity relates to multiple members or prerequisites
- **THEN** it emits repeated triples using one canonical predicate
- **AND** each relationship object carries the `@id` datatype

### Requirement: Canonical entity identity

Stored entity IDs SHALL contain exactly six bounded literal segments. Query
prefixes SHALL contain one through six literal segments and SHALL NOT contain
an empty or trailing-dot segment.

#### Scenario: Malformed prefix blocks configuration

- **WHEN** a configuration declares `local.semsource.` as an entity prefix
- **THEN** validation fails and names the offending configuration field

### Requirement: Explicit relationship typing

Every triple that represents a graph edge SHALL use
`message.EntityReferenceDatatype`. Scalar strings SHALL NOT be inferred as
relationships merely because they resemble an entity ID.

#### Scenario: Relationship coverage

- **WHEN** contract tests enumerate domain graph projections
- **THEN** every approved relationship predicate has an `@id` object
- **AND** no unapproved scalar-to-edge inference is required

### Requirement: Disjoint authoritative writers

Every mutable predicate group SHALL have one declared owner and write mode.
No writer SHALL replace a full shared entity or another owner's predicates.

#### Scenario: Ownership overlap

- **WHEN** two owners claim an overlapping predicate group
- **THEN** startup or contract validation fails before either writer mutates
  authoritative state

### Requirement: Clean pre-v1 cutover

The cutover SHALL use a fresh development/test graph reseeded from canonical
sources. It SHALL NOT add runtime aliases, dual writes, permissive validation,
or an in-place beta-state rewriter.

#### Scenario: Restart replay parity

- **WHEN** the canonical graph is reseeded and the exact binary restarts without
  an intervening write
- **THEN** graph readiness returns true
- **AND** entity, relationship, quest-status, and DAG projections match the
  pre-restart results
