# Proposal: adopt-semstreams-prev1-contract

## Why

Semdragons began this prerequisite while pinned to SemStreams
`v1.0.0-beta.135`; the active successor targets `v1.0.0-beta.159`. The newer
release enforces the pre-v1 graph
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
  product owner before beta.159 is adopted.
- Update Go, rule JSON, TypeScript projections, generated artifacts, fixtures,
  queries, and documentation atomically.
- Resolve issue #28 by deleting partycoord's eight unconsumed raw event
  families and its test-only decomposition/result/rollup shadow, while
  retaining authoritative Party KV form/join/disband behavior and the
  temporary assignment advisory used by both DAG implementations.
- Resolve issue #30 by quarantining agentprogression in all three shipped
  profiles and deleting only its unproduced `AgentXPPayload`, false event
  output, and dead progression vocabulary. Retain its Agent model, XP policy,
  validation, and registered Phase-2 implementation without migrating its
  predicates or writers in this packet.
- Resolve issue #32 by disabling agentstore in the two alternate shipped
  profiles and deleting its five publisher-only raw event families, seven
  event-only vocabulary entries, and false output port. Retain the registered
  store implementation, product policy, REST/OpenAPI/models, and later
  authoritative migration target.
- Resolve issue #34 by retaining boidengine registration, configuration,
  scoring policy, live dormant suggestion contracts, KV implementation, and
  dmpartyformation policy reuse while disabling it in every shipped profile.
  Document that shipped Phase 1 requires explicit claim/start and provides no
  autonomous quest or guild selection.
- Resolve issue #36 by retaining redteam registration, configuration,
  category/vocabulary taxonomy, Quest/API/UI fields, findings/lesson policy,
  prompts, and opt-in E2E while disabling it in every shipped profile. Remove
  only its two false OutputPorts and prove the explicit claim/start →
  questbridge → mechanics-stub path creates no red-team side effect. Record
  the legacy watcher spelling, raw cache, unsafe full-object writers, inert
  claim timeout, and local deadlines as deferred reactivation work.
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
  on this contract. The beta.159 adoption itself is owned by issue
  [#59](https://github.com/C360Studio/semdragon/issues/59) under
  [semdragon#4](https://github.com/C360Studio/semdragon/issues/4).
- **UI and generated artifacts**: predicate projections, entity schemas,
  OpenAPI, and TypeScript types must change in the same gate.
- **Party coordination**: issue #28 narrows partycoord before the canonical
  beta.159 owner migration; it does not bind an owner or activate questdag.
- **Agent progression**: issue #30 removes runtime activation and a dead
  projection facade while preserving the dormant product implementation.
  Canonical predicates, ownership, lifecycle, and watchers remain deferred.
- **Agent store**: issue #32 completes the Phase-1 configuration quarantine
  and removes a raw event plane with no repository consumer. Store routes,
  models, policy, and dormant implementation remain for Phase 2.
- **Boidengine**: issue #34 disables the recommender without deleting its live
  dormant subjects, DTOs, KV implementation, scoring policy, or
  dmpartyformation reuse. REST API and test-fixture claim/start are the
  supported shipped Phase-1 path; the separately authenticated lifecycle
  operator also joins that path through completed task 4.3.
- **Redteam**: issue #36 explicitly disables the retained component in all
  shipped profiles and removes two false OutputPorts without deleting its
  category/vocabulary or product contracts. Its raw watcher/cache,
  full-Quest/full-Guild writers, and process-local timeout design remain
  dormant and deferred to Semdragon #4/#5/#22 and SemStreams #313/#676.
- **Lifecycle operator**: issue #38 enables the upstream lifecycle gateway at
  `/lifecycle-gateway/workflows` for the sole registered `quest` workflow. The
  complete namespace uses `SEMDRAGONS_API_KEY`, WebSockets are disabled, and
  existing product command handlers coexist until task 4.4 can retire them
  without losing domain validation or coupled writes.
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
  [#671](https://github.com/C360Studio/semstreams/issues/671). Existing issue
  [#587](https://github.com/C360Studio/semstreams/issues/587) covers only
  message-logger per-client KV-watch migration. Lifecycle-gateway gaps are
  tracked upstream in [#677](https://github.com/C360Studio/semstreams/issues/677)
  for atomic transition fields,
  [#678](https://github.com/C360Studio/semstreams/issues/678) for workflow and
  operation allowlists, and
  [#679](https://github.com/C360Studio/semstreams/issues/679) for runtime
  managed-gateway OpenAPI paths.
  [#680](https://github.com/C360Studio/semstreams/issues/680) tracks the
  missing OpenAPI security-scheme and operation/document policy model. It does
  not block the authenticated runtime gateway.
  [#681](https://github.com/C360Studio/semstreams/issues/681) tracks the
  framework-owned revision retention needed by lifecycle history, while
  [#682](https://github.com/C360Studio/semstreams/issues/682) tracks the
  lifecycle gateway's oversized-body status mismatch. Semdragon does not own
  downstream substitutes for those framework primitives. The beta.159 cutover
  deletes the legacy bucket bootstrap unconditionally; #681 cannot authorize
  Semdragon to create or reconcile framework-owned `ENTITY_STATES`.
