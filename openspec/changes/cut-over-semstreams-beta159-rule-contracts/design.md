# Design: cut over to SemStreams beta.159

## Context

PR #60 already contains the rule-engine migration reviewed against SemStreams
PR #708. Its beta.158 candidate compiled and passed repository gates, but a
fresh-volume replay failed: a seed-disabled restart appended 15 duplicate
hierarchy triples across six entities. The runbook also lacked an authoritative
caught-up signal.

SemStreams `v1.0.0-beta.159`, commit
`8813270c5ba441286d9120cba82fbf72bdcf9a6c`, contains the upstream fixes:

- PR #747 applies six-field append identity and covers the exact 15-duplicate
  hierarchy replay shape from #713;
- PR #758 publishes per-producer caught-up envelopes and supplies the consumer
  fold requested by #712;
- PR #787 flattens the `graphSummary` GraphQL response;
- PRs #719 and #721 remove `EMBEDDINGS_CACHE` and graph-embedding cache config;
- PRs #724 and #737 enforce framework bucket acquisition and bounded ordinary
  streams; and
- PR #777 adds rule-match and in-flight primitives without requiring a local
  compatibility layer.

The dependency probe compiled unchanged, but compilation was not acceptance.
Before this implementation, Semdragon created `ENTITY_STATES` with history 10
and wrote complete entities through raw KV. Beta.159 declares graph-ingest the
bucket owner with history 1. Letting startup reconcile that mismatch would have
discarded history while preserving downstream ownership of framework state.

## Goals / Non-Goals

**Goals:**

- adopt the exact beta.159 tag and all required breaking changes;
- preserve the completed rule-engine migration and least-authority semantics;
- remove every production direct `ENTITY_STATES` create, raw open, `Put`, and
  `Update` path;
- remove every active product writer until its named framework owner contract
  is implemented;
- fail closed on incomplete ownership or readiness;
- prove fresh-state and seed-disabled restart parity.

**Non-goals:**

- preserving any legacy GraphClient write API;
- adding a Semdragon-owned mutation client, lease protocol, bucket catalog, or
  readiness aggregate;
- enabling dormant mechanics as part of the dependency bump;
- cleaning up an old `EMBEDDINGS_CACHE` bucket inside the replay procedure; or
- claiming replay success from upstream tests without a Semdragon rerun.

## Decisions

### D1: Pin the exact beta.159 tag

`go.mod` SHALL resolve `github.com/c360studio/semstreams v1.0.0-beta.159`.
Branches, later main commits, and floating selectors are rejected. The tag must
resolve to `8813270c5ba441286d9120cba82fbf72bdcf9a6c`.

The existing beta.158 rule-pack configuration remains required: stable pack ID
`quest-lifecycle`, explicit graph integration, the supported `ENTITY_STATES`
watch, omitted lifecycle-only `projection_contracts`, per-entity retry maxima,
framework relationship datatype, gated-DAG defaults, and boundary-correct
prefix helpers.

### D2: Every quest rule declares its entity pattern

Beta.159 rule evaluation requires each definition to declare `entity.pattern`.
The watcher configuration does not supply or infer that definition scope. A
healthy watcher with an omitted pattern can therefore coexist with seven rules
that are all skipped during evaluation.

Every shipped quest rule definition declares the exact canonical pattern
`*.*.game.*.quest.*`. A regression contract inventories all seven definitions
and fails on an omitted, empty, malformed, or different pattern. This is a
downstream beta.159 authoring migration, not an upstream SemStreams defect.

### D3: Bound both ordinary streams

All shipped profiles retain their existing ages and add the beta.159 shipped
baseline storage policy:

| Stream | `max_age` | `max_bytes` | `discard` |
| --- | ---: | ---: | --- |
| `GRAPH` | 24 hours | 268435456 | `old` |
| `AGENT` | 72 hours | 268435456 | `old` |

The 256 MiB bound prevents unbounded disk growth. `discard: old` preserves
forward progress at the explicit retention boundary. Top-level configuration
tests must execute SemStreams `config.Config.Validate`; decoding alone is not a
gate.

### D4: Remove the retired embedding cache surface

The default and SemSource graph-embedding components remove `cache_ttl` and
their complete `ports.outputs` entry for `EMBEDDINGS_CACHE`. Component factory
construction, not only top-level validation, must prove the stale fields are
gone. The replay bucket inventory must not list `EMBEDDINGS_CACHE`.

An old cache bucket is inert. Its optional manual deletion is separate from
the replay runbook's one approved graph-volume deletion.

### D5: Fold readiness over the exact deployed producers

Every graph-gateway config declares:

```json
{
  "readiness_keys": ["graph-ingest", "graph-index", "rule"],
  "readiness_path": "/readiness"
}
```

`graph-embedding` is excluded because tier 1 does not run it and replay parity
does not depend on embedding output. Snapshot automation reads
`GET /graph-gateway/readiness` and folds the three rows client-side. It requires
the exact key set, `known`, `fresh`, a recognized non-hard-stop state,
`bootstrap_complete`, and zero `lag` from the same response.

The gateway exposes no aggregate verdict by design. A separate flat query,
`graphSummary { total_entities }`, proves the framework-only preflight remains
exactly empty. With product seeds and unbound writers disabled, a positive
count is an unexplained write, not success. Stable checksums may be retained as
extra evidence, never as readiness authority.

### D6: Graph-ingest exclusively owns `ENTITY_STATES`

Production code SHALL NOT create `ENTITY_STATES`, open it through raw NATS KV,
or expose its KV handle. Graph-ingest alone calls `EnsureCatalogBucket`.
Semdragon readers acquire the existing bucket only through
`graph.OpenCatalogBucket` and preserve the established `index_not_ready`
classification while the owner has not provisioned it.

The following legacy write surface is deleted rather than adapted:

- `GraphClient.EnsureBucket`;
- `GraphClient.EmitEntity`;
- `GraphClient.EmitEntityUpdate`;
- `GraphClient.EmitEntityCAS`;
- `GraphClient.PutEntityState`; and
- dmsession's raw `ENTITY_STATES` KV write.

The remaining reader facade is product convenience over the public catalog
reader only. It provides no raw KV escape and owns no bucket lifecycle.

### D7: Product mutation fails closed until owner adapters exist

This packet does not introduce product projection adapters. Product mutation
call sites return a typed, not-committed `projection.MutationUnavailable` error
without I/O. The game service, product writer processors, and product seed modes
remain disabled in every shipped profile.

Beta.159 supplies the required reactivation boundary. A later product packet
must bind its complete named contract once and use
`projection.MutationClient`:

- `CreateWithTriples` for framework-validated entity birth;
- `ReplaceOwned` for exclusive current-state predicates; and
- `AppendEvidence` for occurrence/history evidence.

Quest and review phase CAS must use `lifecycle.Manager`; it must not be
reconstructed as a projection write. Exact duplicate append identity is the
beta.159 tuple `(subject, predicate, canonical object, datatype, source,
context)`. Occurrence evidence must supply a stable nonempty context
discriminator when two otherwise equal events are distinct.

No Semdragon mutation wrapper, owner-token minting, or raw graph request client
is introduced. Beta.159's public `MutationClient` fulfills the primitive
previously tracked by SemStreams #313, but availability does not reactivate a
product owner automatically.

### D8: Startup establishes framework ownership first

Composition order is normative:

1. create streams;
2. run `graph.AssertOwnedBucketsClean`;
3. call `ownership.EnsureBuckets` with SemStreams'
   `vocabulary.InverseResolver`;
4. attach that registry to the lifecycle Manager;
5. create one shared `ownership.Heartbeater` and register
   `service.OwnershipService` with the same registry and heartbeater;
6. register the quest lifecycle workflow;
7. create enabled components;
8. call `service.BindRulePackContracts` with that shared registry and
   heartbeater; and
9. start the component manager.

The lifecycle ownership context uses `service.WireOwnershipShutdown` so its
heartbeat is cancelled and joined before NATS closes. The framework-only
Semdragon profile has lifecycle and rule owners but no static projection owner,
so it MUST NOT call `service.WireOwnership` with zero contracts. Beta.159
always tries to bind the private built-in projection owner in that helper, and
an empty contract set fails boot with `mutation client has no contracts`.

[SemStreams #812] tracks a public convenience composition for this valid
downstream shape. Until that lands, the direct sequence above is an explicit
composition of public SemStreams primitives, not a Semdragon ownership
implementation. Semdragon does not copy the private built-in contracts or mint
a placeholder owner merely to satisfy the helper.

Every graph-ingest instance sets `enforce_owner_lease: true`. Startup does not
pre-create `ENTITY_STATES` to avoid a race. Product processors with unbound
owner contracts remain disabled rather than participating in startup.

The catalog reader preserves SemStreams' not-ready error when graph-ingest has
not provisioned the bucket. Docker-backed integration proves both that
early-start behavior and the corrected production composition through
`StartAll`, producer readiness, ownership lookup, and lifecycle create.

### D9: Exact product surfaces remain disabled fail closed

All shipped profiles disable the game service and these product writer
processors:

- `agent_progression`;
- `agent_store`;
- `boidengine`;
- `questboard`;
- `guildformation`;
- `questbridge`;
- `partycoord`;
- `questdagexec`;
- `autonomy`; and
- `redteam`.

The `SEED_E2E`, `SEED_AGENTS`, and `SEED_PROS` modes default false and their
unowned write paths fail closed if invoked. Bossbattle and dmsession are not
shipped as enabled components; their retained mutation paths also fail closed.
The active graph writers are SemStreams graph-ingest, rule/lifecycle machinery,
and other enabled framework processors.

There is no compatibility shim, mixed raw/framework mode, or temporary waiver.
Production static guards and configuration tests make residual direct access or
accidental product activation a hard failure.

### D10: Zero-state preflight is executable; semantic replay remains blocked

SemStreams #712 and #713 are upstream-resolved. That evidence is sufficient to
rerun Semdragon acceptance, not to mark it passed. Beta.159's public lifecycle
gateway can discover, patch, and transition existing instances but cannot
create the first instance. [SemStreams #814] tracks the missing ownership-safe
public create operation.

The executable preflight uses one image, starts from a fresh volume with every
product seed flag false, waits for exact caught-up readiness, proves the graph
and catalog have zero entities, restarts identically, and rejects any
spontaneous write. It does not exercise append identity or revision replay and
cannot close issue #59.

After #814 supplies the public primitive, a reviewed procedure must create
deterministic lifecycle-owned evidence through that API, restart the same image
without seeding, and require exact nonempty semantic parity. No legacy product
seed or raw mutation is an acceptable bridge.

## Risks / Mitigations

- **Raw bucket ownership survives:** source guards reject direct bucket APIs and
  the deleted GraphClient write names.
- **Partial owner migration loses predicates:** no product owner is partially
  active; every unbound product mutation fails closed.
- **Startup races graph-ingest:** readers retry the catalog open and product
  writers stay disabled instead of pre-creating the bucket.
- **A convenience helper binds an owner Semdragon does not deploy:** the
  framework-only composition uses the public zero-static-contract sequence
  above, a boot regression exercises it, and [SemStreams #812] tracks the missing
  convenience primitive.
- **Stream pressure becomes silent loss:** explicit bounds and discard policy
  are validated in every shipped profile and documented operationally.
- **Duplicate evidence collapses incorrectly:** no product append writer is
  active; a reactivation packet must test context discrimination and no-op
  duplicate behavior.
- **Readiness gives a false pass:** the client fold requires exact producer
  membership and fail-closed row semantics, followed by an independent exact
  zero-state assertion. Semantic replay stays blocked rather than treating
  emptiness as append or revision evidence.
- **Healthy watcher hides skipped rules:** the seven-definition inventory
  requires the canonical quest pattern on every rule and live integration must
  still prove evaluation.

## Architecture Sign-Off

Architecture acceptance requires the exact stream policies,
catalog-only reader, framework lifecycle/rule ownership, strict owner leases,
fail-closed product writers, and no legacy GraphClient write API. Public
`MutationClient` adapters remain a reactivation boundary rather than a claimed
part of this framework-only runtime. The zero-static-contract correction and
production boot regression are implemented; final reviewer follow-up remains
pending. PR #60 remains draft until all gates pass and Fable reviews the
expanded surface.

[SemStreams #812]: https://github.com/C360Studio/semstreams/issues/812
[SemStreams #814]: https://github.com/C360Studio/semstreams/issues/814
