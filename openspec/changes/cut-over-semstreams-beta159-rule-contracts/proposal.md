# Proposal: cut over to SemStreams beta.159

## Why

Semdragon issue [#59] adopts the latest tagged SemStreams v1 beta after the
beta.158 candidate failed immutable replay. SemStreams beta.159 closes the
readiness gap in #712 and the duplicate hierarchy replay defect in #713, but
it also makes stream bounds, bucket ownership, graph-gateway responses, and
embedding configuration stricter.

The cutover exposed a critical downstream violation: Semdragon's historical
`GraphClient` created, opened, and wrote SemStreams-owned `ENTITY_STATES`
directly. A successful compile could not waive that violation. This change
deletes that ownership, keeps product mutation surfaces disabled fail closed,
and leaves their reactivation dependent on SemStreams projection or lifecycle
primitives.

## What Changes

- Pin `github.com/c360studio/semstreams` to `v1.0.0-beta.159`.
- Preserve the completed beta.158 rule-pack, retry, entity-reference, gated-DAG,
  and prefix migrations against the new tag.
- Add the required canonical quest `entity.pattern` to all seven shipped rule
  definitions so beta.159 admits them for evaluation.
- Bound `GRAPH` and `AGENT` streams explicitly and fail closed on missing
  storage policy.
- Remove the retired embedding cache configuration.
- Configure and consume the beta.159 per-producer caught-up readiness surface.
- Adopt the flat `graphSummary` GraphQL envelope.
- Delete production direct creation, raw opening, and raw writing of
  `ENTITY_STATES`.
- Route reads through `graph.OpenCatalogBucket` and retain no production raw KV
  escape.
- Keep product services, processors, endpoints, and seed modes disabled until
  their complete owner boundaries use `projection.MutationClient` or
  `lifecycle.Manager`.
- Compose the framework-only ownership substrate from SemStreams' public graph,
  ownership, lifecycle, heartbeat, and service primitives. Do not call
  `service.WireOwnership` with an empty static projection contract set.
- Require owner leases for active framework graph mutation.
- Run the fresh-volume, seed-disabled zero-state boot/restart preflight with one
  immutable image. Keep nonempty semantic replay blocked on [SemStreams #814];
  do not substitute a legacy seed or raw mutation.

## Capabilities

### Modified Capabilities

- `semstreams-rule-cutover`: exact beta.159 dependency, breaking configuration,
  framework-owned graph access, and immutable replay acceptance.

## Impact

- **Dependency:** `go.mod` and `go.sum`.
- **Configuration:** all shipped stream, graph-ingest, graph-gateway, rule, and
  graph-embedding profiles.
- **Graph integration:** the read-only `GraphClient`, dmsession, component
  startup, rule/lifecycle ownership, and fail-closed product mutation surfaces.
- **Operations:** authoritative readiness, flat GraphQL summary, catalog bucket
  inventory, truthful zero-state evidence, and the #814-blocked semantic replay.
- **Delivery:** issue #59, strict OpenSpec validation, CI, reviewers, and draft
  PR #60. The PR cannot merge until Fable reviews the expanded API surface.
- **Upstream:** [SemStreams #812] tracks the missing convenience composition for
  a downstream runtime that has lifecycle and rule owners but no static
  projection owner. Until that primitive ships, Semdragon directly composes
  the corresponding public SemStreams APIs without copying private contracts.

[#59]: https://github.com/C360Studio/semdragon/issues/59
[SemStreams #712]: https://github.com/C360Studio/semstreams/issues/712
[SemStreams #713]: https://github.com/C360Studio/semstreams/issues/713
[SemStreams #812]: https://github.com/C360Studio/semstreams/issues/812
[SemStreams #814]: https://github.com/C360Studio/semstreams/issues/814
