# Proposal: adopt-semstreams-prev1-contract

## Why

Semdragons currently pins SemStreams `v1.0.0-beta.135`, while the latest
released tag is `v1.0.0-beta.158`. The newer release enforces the pre-v1 graph
contract: canonical three-part predicates, bounded six-part entity IDs, typed
entity references, explicit rule-pack identity, and ownership-safe graph
mutation. A dependency-only bump would reject current producers and could
poison authoritative graph state.

This change is the prerequisite contract cutover for epic
[semdragon#2](https://github.com/C360Studio/semdragon/issues/2) and issue
[semdragon#3](https://github.com/C360Studio/semdragon/issues/3). It is stacked
after the lifecycle/gated-DAG foundation in draft PR
[semdragon#1](https://github.com/C360Studio/semdragon/pull/1).

## What Changes

- Define a complete old-to-new predicate ledger using exactly
  `domain.category.property`, with lower-kebab segments.
- Replace dynamic predicate names with repeated canonical relationships or
  bounded structured values.
- Require exactly six literal segments for entity IDs and one through six
  literal segments for query prefixes.
- Mark every entity relationship object with the SemStreams `@id` datatype.
- Record every graph writer, its predicate group, write mode, and framework or
  product owner before beta.158 is adopted.
- Update Go, rule JSON, TypeScript projections, generated artifacts, fixtures,
  queries, and documentation atomically.
- Resolve issue #28 by deleting partycoord's eight unconsumed raw event
  families and its test-only decomposition/result/rollup shadow, while
  retaining authoritative Party KV form/join/disband behavior and the
  temporary assignment advisory used by both DAG implementations.
- Cut over by wiping and reseeding development/test graph state. No aliases,
  dual writes, permissive mode, or beta-state rewriter will be added.
- Establish zero-finding predicate, entity-ID, relationship, build, and UI
  compatibility gates.

## Capabilities

### New Capabilities

- `graph-contract`: Semdragons' canonical predicate, entity-ID, relationship,
  writer-ownership, and clean-cutover contract.
- `framework-migration`: the dependency and gate ordering used to adopt a
  breaking SemStreams pre-v1 release without retaining framework duplication.

## Impact

- **Code and configuration**: all graph producers/consumers, lifecycle rules,
  DAG units, source prefixes, and relationship reconstruction.
- **Related changes**: `rebuild-quest-lifecycle-on-semstreams-rules` is blocked
  on this contract. The beta.158 adoption itself is owned by the follow-up
  tracked in [semdragon#4](https://github.com/C360Studio/semdragon/issues/4).
- **UI and generated artifacts**: predicate projections, entity schemas,
  OpenAPI, and TypeScript types must change in the same gate.
- **Party coordination**: issue #28 narrows partycoord before the canonical
  beta.158 owner migration; it does not bind an owner or activate questdag.
- **Data**: incompatible beta graph/index state is deleted and reseeded from
  source; no production deployment or retained beta data requires migration.
- **Upstream coordination**: reuse SemStreams issues
  [#313](https://github.com/C360Studio/semstreams/issues/313) and
  [#382](https://github.com/C360Studio/semstreams/issues/382); predicate-audit
  and gated-DAG documentation defects found here are tracked in
  [#667](https://github.com/C360Studio/semstreams/issues/667) and
  [#668](https://github.com/C360Studio/semstreams/issues/668). Staged fan-out
  admission and occurrence-exact audit classification gaps are tracked in
  [#670](https://github.com/C360Studio/semstreams/issues/670) and
  [#671](https://github.com/C360Studio/semstreams/issues/671).
