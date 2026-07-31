# SemStreams beta.159 cutover specification

## ADDED Requirements

### Requirement: The cutover uses the exact beta.159 tag

Semdragon SHALL depend on `github.com/c360studio/semstreams
v1.0.0-beta.159`. The source SHALL resolve to commit
`8813270c5ba441286d9120cba82fbf72bdcf9a6c` and SHALL NOT use a branch,
floating selector, or post-tag main commit.

#### Scenario: Module identity is verified

- **WHEN** the dependency contract runs
- **THEN** `go list -m` reports exactly `v1.0.0-beta.159`
- **AND** the resolved source contains the reviewed #712 and #713 fixes
- **AND** the shipped Go toolchain contract remains satisfied

### Requirement: Every shipped quest rule declares its entity pattern

All seven shipped quest rule definitions SHALL declare
`entity.pattern: "*.*.game.*.quest.*"`. Watch configuration SHALL NOT be
treated as an inferred definition pattern.

#### Scenario: Healthy watch cannot hide skipped definitions

- **GIVEN** the quest rule processor has a healthy `ENTITY_STATES` watcher
- **WHEN** the shipped rule inventory is admitted under beta.159
- **THEN** exactly seven definitions are present
- **AND** every definition declares the canonical quest pattern
- **AND** an omitted, empty, malformed, or different pattern fails the contract

#### Scenario: Quest rules remain live after authoring migration

- **WHEN** a matching quest mutation reaches the live rule processor
- **THEN** the applicable definition is evaluated
- **AND** the watcher alone is not accepted as proof of rule liveness

### Requirement: Ordinary streams have explicit storage bounds

Every shipped profile SHALL configure `GRAPH` with 24-hour `max_age` and
`AGENT` with 72-hour `max_age`. Both SHALL use `max_bytes: 268435456` and
`discard: "old"`.

#### Scenario: All shipped profiles pass framework validation

- **WHEN** SemStreams `config.Config.Validate` checks each shipped profile
- **THEN** `GRAPH` and `AGENT` have positive age and byte bounds
- **AND** their byte and discard policies equal the required values
- **AND** no ordinary stream relies on an implicit storage default

### Requirement: Retired embedding cache configuration is absent

Graph-embedding components SHALL NOT configure `cache_ttl`, an output port, or
`EMBEDDINGS_CACHE`.

#### Scenario: Applicable components construct successfully

- **WHEN** the default and SemSource graph-embedding components are created
- **THEN** component factory validation succeeds with no cache fields or ports
- **AND** the tier-1 profile remains valid without graph embedding
- **AND** the migration bucket inventory excludes `EMBEDDINGS_CACHE`

### Requirement: Snapshot readiness folds exact producer state

Every shipped graph-gateway SHALL expose `/readiness` for exactly
`graph-ingest`, `graph-index`, and `rule`. Snapshot automation SHALL fold those
rows client-side and SHALL NOT consume or publish an aggregate readiness
verdict.

#### Scenario: All required producers are caught up

- **WHEN** `GET /graph-gateway/readiness` returns one row for each required key
- **THEN** the key set and count are exact
- **AND** every row is known and fresh
- **AND** every state is recognized and is neither `degraded` nor
  `reset_required`
- **AND** every bootstrap is complete and every lag equals zero
- **THEN** snapshot capture may proceed

#### Scenario: Incomplete producer evidence fails closed

- **WHEN** a key is absent, duplicated, unknown, stale, unrecognized,
  hard-stopped, bootstrapping, or lagging
- **THEN** snapshot capture remains deferred
- **AND** a checksum, entity count, or other producer cannot override it

### Requirement: Zero-state preflight uses the flat summary envelope

The framework-only preflight SHALL separately query
`graphSummary { total_entities }` and read
`.data.graphSummary.total_entities`. With every product seed and unbound writer
disabled, it SHALL require the count to equal zero and SHALL treat a positive
count as an unexplained write.

#### Scenario: Readiness and data assertions stay independent

- **WHEN** producer readiness is fully covered
- **THEN** the runbook still requires an exact zero flat summary count
- **AND** the removed `.data.graphSummary.data` envelope is rejected

### Requirement: Graph-ingest exclusively owns ENTITY_STATES

Production Semdragon code SHALL NOT create `ENTITY_STATES`, open it through raw
NATS KV, write it through KV `Put` or `Update`, or expose its raw handle.
Graph-ingest SHALL be the only catalog owner. Product readers SHALL open the
existing bucket through `graph.OpenCatalogBucket`.

#### Scenario: Reader starts before bucket provisioning

- **GIVEN** graph-ingest has not yet provisioned `ENTITY_STATES`
- **WHEN** a product reader starts
- **THEN** catalog acquisition preserves `index_not_ready` classification
- **AND** the product retries or waits according to startup policy
- **AND** it does not create or reconcile the bucket itself

#### Scenario: Legacy GraphClient write surface is absent

- **WHEN** production source guards run
- **THEN** `EnsureBucket`, `EmitEntity`, `EmitEntityUpdate`, `EmitEntityCAS`, and
  `PutEntityState` are absent from GraphClient
- **AND** dmsession has no raw `ENTITY_STATES` write
- **AND** no renamed compatibility wrapper performs the same operation

### Requirement: Unbound product mutation remains fail closed

The beta.159 cutover SHALL NOT activate a product projection owner. Every
retained unbound product mutation surface SHALL return a typed, not-committed
`projection.MutationUnavailable` error without graph I/O.

#### Scenario: A disabled product path cannot mutate the graph

- **GIVEN** a product owner adapter is not bound
- **WHEN** its retained mutation path is invoked
- **THEN** the call fails with `MutationUnavailable`
- **AND** its commit state is `CommitNotCommitted`
- **AND** no raw KV or graph-ingest request/reply mutation occurs

#### Scenario: Reactivation uses the framework semantic lane

- **WHEN** a later packet reactivates entity birth
- **THEN** it binds a complete owner contract and uses `CreateWithTriples`
- **WHEN** it reactivates exclusive current-state projection
- **THEN** it uses `ReplaceOwned` with its owner lease
- **WHEN** it reactivates occurrence evidence
- **THEN** it uses `AppendEvidence` with a stable context discriminator
- **WHEN** it reactivates quest or review lifecycle phase mutation
- **THEN** it uses the registered lifecycle Manager

### Requirement: Ownership is established before framework writers start

Composition SHALL create streams, run `graph.AssertOwnedBucketsClean`, call
`ownership.EnsureBuckets`, attach the returned registry to the lifecycle
Manager, and create one shared heartbeater and `service.OwnershipService`
before workflow registration. It SHALL then create enabled components, call
`service.BindRulePackContracts` with the same registry and heartbeater, and
start the component manager.

The framework-only profile SHALL NOT bind a static projection owner because it
declares no static projection contracts. It SHALL NOT copy SemStreams' private
built-in contracts or manufacture an empty placeholder contract to use
`service.WireOwnership`.

#### Scenario: Framework-only ownership boots without a static contract

- **GIVEN** lifecycle and rule owners are enabled
- **AND** no static projection owner contract is deployed
- **WHEN** the production ownership composition runs
- **THEN** ownership buckets, lifecycle ownership, and the shared heartbeater
  are established through public SemStreams primitives
- **AND** rule-pack contracts bind before the component manager starts
- **AND** startup does not fail with `mutation client has no contracts`
- **AND** no private built-in contract is copied downstream

#### Scenario: Strict ownership gates startup

- **WHEN** any shipped graph-ingest instance is configured
- **THEN** `enforce_owner_lease` is true
- **AND** no product startup pre-creates `ENTITY_STATES`
- **AND** no product writer without a complete contract is enabled

### Requirement: Incomplete writers remain disabled fail closed

Every shipped profile SHALL disable the game service and the following product
writer processors until their complete owner boundaries are migrated:
`agent_progression`, `agent_store`, `boidengine`, `questboard`,
`guildformation`, `questbridge`, `partycoord`, `questdagexec`, `autonomy`, and
`redteam`.

Product seed modes SHALL default false. Bossbattle and dmsession SHALL remain
unshipped as enabled components, and their retained mutations SHALL fail closed.

#### Scenario: No compatibility waiver permits mixed ownership

- **WHEN** the release gate inventories production writers
- **THEN** no listed product writer or game API service is enabled
- **AND** all three product seed flags default false
- **AND** no active writer uses raw KV or a Semdragon mutation substitute
- **AND** enabled graph writers are framework-owned or have complete framework
  contracts

### Requirement: The completed rule-engine migration remains canonical

Every shipped quest rule processor SHALL keep stable pack identity
`quest-lifecycle`, explicit graph integration, supported `ENTITY_STATES` watch
configuration, and omitted lifecycle-only `projection_contracts`. Rules SHALL
retain per-quest retry maxima and framework primitives for relationships,
gated-DAG markers, and entity-prefix boundaries.

#### Scenario: Beta.159 does not reintroduce compatibility code

- **WHEN** the existing rule, lifecycle, gated-DAG, and prefix contracts run
- **THEN** their beta.158 migration behavior remains green
- **AND** no copied primitive, legacy watch shape, or duplicate projection
  authority is introduced

### Requirement: Fresh-state preflight does not substitute for semantic replay

The implementation SHALL execute the reviewed development/test zero-state
preflight using one immutable backend image. It SHALL start the framework-only
profile with every product seed flag false, wait for authoritative caught-up
readiness, require an empty graph and catalog, restart identically, and reject
any spontaneous entity.

#### Scenario: Exact image preserves zero state across restart

- **WHEN** empty framework-only state is captured before and after restart
- **THEN** both starts use the same retained image ID
- **AND** the exact readiness fold passes before each capture
- **AND** both flat summaries and catalog key inventories remain exactly empty
- **AND** no product seed, raw mutation, or unexplained entity appears
- **AND** protected SemSource services and volumes remain untouched

#### Scenario: Missing public lifecycle create keeps semantic replay blocked

- **GIVEN** beta.159 has no public ownership-safe operation that creates the
  first lifecycle instance
- **WHEN** the zero-state preflight passes
- **THEN** issue #59 remains incomplete pending SemStreams #814
- **AND** empty entity, relationship, quest-status, and DAG arrays are not
  accepted as append or revision replay evidence
- **AND** no legacy seed or raw graph mutation is enabled as a workaround

#### Scenario: Semantic replay uses the future framework primitive

- **WHEN** SemStreams #814 supplies a reviewed public lifecycle-create operation
- **THEN** the acceptance procedure creates deterministic nonempty state through
  that operation
- **AND** it restarts the same image with every seed flag false
- **AND** entity, relationship, quest-status, and DAG evidence has exact parity
- **AND** no entity revision moves and no duplicate append identity appears

#### Scenario: Upstream closure is not downstream acceptance

- **WHEN** SemStreams #712 and #713 are closed with tests
- **THEN** the Semdragon zero-state preflight is eligible to run
- **AND** issue #59 remains incomplete until observed Semdragon evidence passes
