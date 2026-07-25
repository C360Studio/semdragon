# Design: adopt-semstreams-prev1-contract

## Context

SemStreams beta.158 is a clean pre-v1 break. It validates stored predicates as
exactly three lower-kebab segments, validates bounded six-part entity IDs and
typed `@id` references, requires explicit rule-pack identity/configuration, and
rejects unsafe writes into framework-owned graph state.

Semdragons predates those contracts. It contains two-part predicates,
underscore segments, dynamic identifiers embedded in predicate names,
trailing-dot query prefixes, untyped relationship-looking strings, and direct
full-object writes to `ENTITY_STATES`. The active lifecycle/DAG migration
cannot safely adopt beta.158 until this corpus is canonical.

## Goals / Non-Goals

**Goals:**

- Make the complete Semdragons graph corpus valid under beta.158.
- Define stable ownership and write modes before enabling enforcement.
- Preserve user-visible quest status and DAG behavior through tested mappings.
- Provide a deterministic fresh-state cutover and replay gate.

**Non-Goals:**

- Preserving or rewriting incompatible beta NATS state.
- Adding aliases, dual writes, permissive validation, or compatibility readers.
- Activating the new DAG executor or deleting legacy lifecycle code in this
  prerequisite change.
- Copying an internal SemStreams mutator while upstream
  [#313](https://github.com/C360Studio/semstreams/issues/313) is unresolved.

## Decisions

### D1: Contract change precedes dependency change

Canonical source, configuration, fixtures, UI mappings, and contract tests land
while the module remains on beta.135. Beta.158 adoption is a subsequent,
attributable gate. This prevents a broad dependency bump from mixing framework
breakage with product-corpus changes.

### D2: One canonical predicate language

Every stored predicate is exactly `domain.category.property`; every segment is
lower-kebab. Runtime IDs never appear inside predicate names. Multi-valued
relationships use repeated canonical predicates. Values that need several
fields use a bounded structured object or a first-class entity, chosen in the
rename ledger.

The ledger is load-bearing: it records old name, new name, value/cardinality,
producer, consumers, owner, write mode, and UI mapping. There is no runtime
alias table.

### D3: Entity identities and references are explicit

Entity IDs have six literal positions. Query prefixes have one through six
literal positions and never use a trailing dot. Relationship objects are
tagged with `message.EntityReferenceDatatype`; scalar strings that merely look
like IDs are not treated as graph edges.

### D4: Ownership follows responsibility

The writer-owner matrix is partitioned by predicate group:

- lifecycle owns quest phase, attempts, assignment, failure, and verdict;
- questbridge owns execution, context, loop, metrics, and work output;
- questdag owns product DAG projection and unit provenance;
- gated-DAG owns its canonical `gateddag.unit.*` markers and edges;
- partycoord owns party formation, membership, matching, and disband facts.

Creation-time identity/specification facts remain source-owned. No full-object
writer may replace predicates belonging to another owner.

### D5: Framework defaults beat copied vocabulary

Semdragons removes its old gated-DAG marker overrides and consumes the
framework's canonical defaults. A product override is allowed only when the
predicate has distinct product semantics and is declared in the product
vocabulary.

### D6: Clean wipe and reseed

The cutover stops all writers, derives the exact incompatible bucket list from
the rendered configuration, deletes only those literal development/test
buckets, restarts the exact pinned binary, and reseeds canonical source data.
Wildcard deletion is forbidden. Restart-without-write must reproduce query and
projection results.

## Delivery Order

1. Inventory and approve predicate/reference and writer-owner ledgers.
2. Add failing contract tests and canonical validators.
3. Update producers and Go readers.
4. Update rules, configuration, UI projections, fixtures, and generated files.
5. Reach zero audit findings and green beta.135 tests.
6. Hand off beta.158 adoption to the follow-up change tracked in
   [semdragon#4](https://github.com/C360Studio/semdragon/issues/4), then run its
   fresh-volume/replay gates.

## Risks / Mitigations

- **Silent UI drift**: update generated schema and TypeScript mappings in the
  same predicate commit; assert quest status and DAG projections.
- **Lost relationship semantics**: require an `@id` coverage test over every
  domain `Graphable`.
- **Cross-owner clobbering**: review the writer-owner matrix before beta.158 and
  add overlap rejection tests in the dependent adoption slice.
- **Incomplete audit**: use scoped audit roots while SemStreams
  [#667](https://github.com/C360Studio/semstreams/issues/667) is open, then make
  whole-repository audit mandatory when fixed.
- **Destructive operator error**: record literal rendered buckets and NATS
  context; never infer deletion targets from an unset variable or wildcard.

## Sign-Off

The architect approves implementation only in this order. A dependency-only
bump, dual-write compatibility path, or activation of `questdag` before the
canonical corpus and ownership gates is rejected.
