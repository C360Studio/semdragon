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

#### Scenario: Canonical graph-source prefixes preserve routing boundaries

- **GIVEN** a graph source declares canonical prefix `osh`
- **WHEN** the registry routes entity ID or query prefix `osh` or `osh.asset`
- **THEN** it selects the OSH source
- **AND** `oshx` and `oshx.asset` do not match that source
- **AND** the canonical no-dot prefix is forwarded to `entitiesByPrefix`
- **AND** every shipped graph-source prefix passes the beta.158 entity-ID audit

#### Scenario: Unready ownership does not leak to fallback sources

- **GIVEN** an unready external source owns canonical prefix `osh`
- **WHEN** the registry routes entity ID or query prefix `osh.asset`
- **THEN** it returns no query target
- **AND** it does not query the local or another ready source
- **AND** local fallback remains available for a prefix with no configured
  owner

#### Scenario: Beta adoption replaces the temporary prefix comparison

- **GIVEN** issue #18 remains pinned to SemStreams beta.135
- **WHEN** issue #4 adopts beta.158
- **THEN** runtime prefixes are validated with `types.ValidateEntityIDPrefix`
- **AND** concrete six-part entity routing uses `graph.MatchesAnyIDPrefix`
- **AND** partial prefix routing does not misuse that full-ID matcher
- **AND** the minimal validated prefix-to-prefix comparison is deleted when a
  tagged SemStreams release contains the primitive from
  [SemStreams #675](https://github.com/C360Studio/semstreams/issues/675)
- **AND** until then, the comparison remains isolated and linked to #675 rather
  than becoming a product-local generic prefix library

### Requirement: Explicit relationship typing

Every triple that represents a graph edge SHALL use
`message.EntityReferenceDatatype`. Scalar strings SHALL NOT be inferred as
relationships merely because they resemble an entity ID.

#### Scenario: Relationship coverage

- **WHEN** contract tests enumerate domain graph projections
- **THEN** every approved relationship predicate has an `@id` object
- **AND** no unapproved scalar-to-edge inference is required

### Requirement: Autonomy graph-event vocabulary is explicit

The system SHALL declare and emit the canonical autonomy graph-event
vocabulary without changing the live raw typed-subject event contract. A raw
typed-subject publish SHALL NOT be treated as proof that graph-ingest persists
the payload.

#### Scenario: All autonomy predicates are declared

- **WHEN** contract tests enumerate the seven autonomy payload Graphables
- **THEN** they emit exactly 21 declared canonical predicates
- **AND** every predicate has exactly three lower-kebab segments
- **AND** no legacy underscore-segment autonomy predicate remains

#### Scenario: Autonomy relationships are same-subject edges

- **WHEN** claim, guild-join, and quest-scoped consumable events emit triples
- **THEN** exactly `agent.autonomy.claim-quest`,
  `agent.autonomy.guild-join`, and `agent.autonomy.use-quest` use `@id`
- **AND** each triple subject equals the payload's agent `EntityID()`
- **AND** none requires a `ForeignEdgeClaim`

#### Scenario: Autonomy catalog aliases and counts remain scalar

- **WHEN** shop, consumable-use, guild-join, or guild-create evidence is emitted
- **THEN** `agent.autonomy.shop-item` and
  `agent.autonomy.use-consumable` remain scalar catalog aliases
- **AND** `agent.autonomy.guild-choices` and
  `agent.autonomy.guild-create-candidates` remain scalar counts
- **AND** no scalar value is inferred to be a graph relationship

#### Scenario: Autonomy graph route is absent before beta.158 adoption

- **GIVEN** autonomy payloads are published as raw typed-subject JSON
- **AND** no autonomy payload factory or graph-ingest route is registered
- **WHEN** issue #14 canonicalizes the payload Graphables
- **THEN** no autonomy projection contract, owner bind, or heartbeat is added
- **AND** issue #4 either wires the framework graph route and seven
  append-evidence contracts or deletes the latent Graphable surfaces
- **AND** a wired route uses no foreign-edge claim and binds static owner
  `autonomy` with `projection.BindAndHeartbeat`

### Requirement: Executor graph surfaces follow their runtime behavior

The system SHALL preserve and canonicalize live execution lifecycle events,
delete definition-only tool-event projections, and keep the real web-document
writer distinct from both. A raw typed-subject publish SHALL NOT be treated as
graph-ingest persistence.

#### Scenario: Live execution lifecycle vocabulary is declared

- **GIVEN** executor and questbridge both publish execution lifecycle events
- **WHEN** tests enumerate the started, completed, and failed payload Graphables
- **THEN** they emit exactly 15 unique declared canonical predicates
- **AND** only `execution.agent.id` uses `@id`
- **AND** the relationship subject equals the payload's quest `EntityID()`

#### Scenario: Unproduced tool-event projections are removed

- **GIVEN** tool-call and tool-result payloads have no production publisher or
  consumer
- **WHEN** issue #16 resolves their predicate-audit findings
- **THEN** their payloads, typed subjects, vocabulary metadata, and graph
  relationship fixtures are deleted
- **AND** no projection owner, compatibility event, or replacement telemetry
  implementation is added
- **AND** issue #7 owns any later tool telemetry through SemStreams primitives

#### Scenario: Real web writer is only lexically canonicalized

- **GIVEN** web fetch persists a document through direct full-object
  `GraphClient.EmitEntity`
- **WHEN** issue #16 canonicalizes the writer
- **THEN** it emits `source.doc.mime-type`, `source.doc.fetched-by`, and
  `source.doc.quest-id`
- **AND** exactly `source.doc.fetched-by` and `source.doc.quest-id` use `@id`
- **AND** they remain same-subject edges with no `ForeignEdgeClaim`
- **AND** owner-safe replacement of the full-object write remains a later
  web-document packet

#### Scenario: Executor package has exactly three typed relationships

- **WHEN** contract tests enumerate the retained executor graph surfaces
- **THEN** exactly `execution.agent.id`, `source.doc.fetched-by`, and
  `source.doc.quest-id` use `@id`
- **AND** loop IDs, URLs, MIME type, status, errors, counts, tokens, and
  durations remain scalar

#### Scenario: No executor projection is bound in the lexical packet

- **GIVEN** lifecycle events have no payload registration or graph-ingest route
- **AND** the web writer has not moved to an owner-classified mutation lane
- **WHEN** issue #16 completes
- **THEN** no executor projection contract, owner bind, or heartbeat is added
- **AND** issue #4 owns the real lifecycle graph route and publisher-specific
  `BindAndHeartbeat` lifecycle
- **AND** no single owner is inferred from the executor payload package

#### Scenario: Missing standard modified predicate remains upstream-owned

- **GIVEN** beta.158 lacks a constant and registration for
  `dc.terms.modified`
- **WHEN** issue #16 declares the web document vocabulary
- **THEN** it may add one compatibility declaration linked to SemStreams #673
- **AND** the declaration is not treated as product-owned generic vocabulary
- **AND** it is deleted on adoption of the first SemStreams tag containing
  the upstream primitive

### Requirement: DM-session events do not claim graph persistence

The system SHALL keep the dmsession raw event contract separate from graph
persistence. An event DTO or internal raw publisher method SHALL NOT establish
a graph writer without a production route and owner.

#### Scenario: Raw session events have no graph projection

- **GIVEN** `SessionManager` retains raw session start and end publisher
  methods on their typed subjects
- **AND** the dmsession component calls those manager methods internally
- **AND** no shipped configuration or production caller outside
  `processor/dmsession` activates the component entrypoints
- **AND** no payload factory or graph-ingest route activates those DTOs as
  graph publishers
- **WHEN** issue #20 resolves their beta.158 findings
- **THEN** the raw DTOs, typed subjects, JSON fields, validation, and publisher
  methods remain unchanged
- **AND** graph-only methods, assertions, predicates, and tests are absent
- **AND** no projection contract, owner bind, or heartbeat is introduced

#### Scenario: Authoritative session state requires framework migration

- **GIVEN** dmsession writes raw `domain.Session` JSON under
  `session.<instance>` in the shared board entity-state bucket
- **AND** the value and key do not satisfy the canonical graph entity-state
  contract
- **WHEN** session graph persistence is designed
- **THEN** issue #4 or a dedicated packet uses SemStreams lifecycle and
  ownership primitives with a canonical session entity key
- **AND** it removes the raw non-`graph.EntityState` write
- **AND** it does not create a second event-derived writer or local framework
  substitute

### Requirement: Guild raw events do not duplicate authoritative state

The system SHALL preserve active guildformation raw event contracts without
treating their DTOs as a second graph projection.

#### Scenario: Active guild events retain only their wire contract

- **GIVEN** guildformation is enabled in all shipped configurations
- **AND** its create, join, leave, promote, and disband operations publish five
  raw DTOs through typed NATS subjects
- **AND** no guild event payload factory, `BaseMessage`, graph-ingest route,
  projection bind, heartbeat, or graph reader exists
- **WHEN** issue #22 resolves the guildformation beta.158 findings
- **THEN** the five DTOs, typed subjects, validation, publisher calls, and JSON
  wire fields remain unchanged
- **AND** all five `Graphable` assertions and all fifteen graph-only methods are
  absent
- **AND** no event projection owner or replacement graph route is introduced

#### Scenario: Duplicate and dynamic guild event facts are deleted

- **GIVEN** `guild.event.created_by` duplicates the authoritative typed
  `guild.founding.agent` relationship
- **AND** the joined and promoted event shapes embed agent IDs in dynamic rank
  predicates
- **AND** `guild.lifecycle.disbanded_at` has no authoritative Guild field or
  graph reader
- **WHEN** the latent event projection is removed
- **THEN** neither audit finding is renamed or registered
- **AND** the event relationship fixtures are removed
- **AND** raw `founder_id`, `agent_id`, and timestamp JSON fields remain wire
  data rather than graph aliases

#### Scenario: Authoritative guild state remains a framework handoff

- **GIVEN** guildformation, agentprogression, and redteam can each replace a
  full Guild snapshot
- **AND** current membership and application data use dynamic predicates
- **AND** pending-guild expiry uses a component-local ticker
- **WHEN** authoritative guild state is migrated
- **THEN** issue #4 or a dedicated packet uses SemStreams lifecycle, rules,
  ownership, and classified mutation primitives
- **AND** it models membership and application records as first-class children
- **AND** it removes cross-owner full-object replacement and local framework
  substitutes
- **AND** issue #22 opens no upstream framework issue merely to delete the
  latent event Graphables

### Requirement: Unactivated seeding processor is retired

The system SHALL remove the unactivated and unconsumed seeding processor
without preserving its DTOs as product wire or graph contracts.

#### Scenario: Definition-only processor is deleted as one surface

- **GIVEN** all five `processor/seeding` files were introduced as one
  event-driven component
- **AND** its input port has no subscription, no external caller invokes
  `Seed`, and no shipped configuration activates the component
- **AND** no payload factory, `BaseMessage`, graph-ingest route, projection
  bind, heartbeat, graph reader, API reader, or UI reader exists
- **WHEN** issue #24 resolves the seeding beta.158 findings
- **THEN** all five package files are absent
- **AND** the package import, both registration callbacks, and both
  component-name entries are absent
- **AND** no replacement event route, projection owner, bind, or heartbeat is
  introduced

#### Scenario: Dead wire, vocabulary, and fixture surfaces are not retained

- **GIVEN** the four DTO families publish only behind the unreachable
  component entrypoint
- **AND** their thirteen predicate occurrences include the four beta.158
  findings
- **WHEN** the processor is retired
- **THEN** its four DTOs, subjects, validation, publishers, JSON fields,
  `Graphable` assertions, and twelve graph-only methods are absent
- **AND** the seven seeding-only vocabulary constants and registrations are
  absent
- **AND** all four root relationship fixtures and the seeding import are
  absent
- **AND** no deleted predicate is renamed or assigned a compatibility alias
- **AND** no raw DTO JSON compatibility promise remains

#### Scenario: Authoritative startup Agent birth remains a framework handoff

- **GIVEN** `cmd/semdragons/seed_e2e.go` is a separate startup path selected by
  `SEED_AGENTS`, `SEED_E2E`, or `SEED_PROS`
- **AND** it creates authoritative `agentprogression.Agent` state rather than
  consuming the deleted event subjects
- **WHEN** issue #24 retires the processor
- **THEN** the startup/E2E seeder and its binary invocation remain
- **AND** issue #4 or a dedicated packet moves the direct shared-state birth
  through graph-ingest create-with-triples
- **AND** mutable Agent state crosses the agentprogression ownership boundary
- **AND** create-only Agent birth facts remain outside mutable owner claims

#### Scenario: Framework coverage requires no upstream issue

- **GIVEN** SemStreams provides classified request/reply,
  create-with-triples, lifecycle, rules, bounded dispatch, projection
  contracts, and ownership
- **WHEN** the unused processor and event Graphables are removed
- **THEN** issue #24 opens no upstream SemStreams issue
- **AND** it adds no private lifecycle engine, goroutine pool, shared-KV
  writer, or replacement event-Graphable state projection

### Requirement: Bossbattle is disabled before authoritative migration

The system SHALL disable bossbattle in every shipped runtime profile during
Phase 1 and SHALL use the complete quest lifecycle rule pack to resolve
review-required quests without battle or mechanics side effects.

#### Scenario: Every shipped profile disables bossbattle

- **WHEN** contract tests parse `config/semdragons.json`,
  `config/semdragons-semsource.json`, and
  `config/semdragons-e2e-tier1.json`
- **THEN** none activates a bossbattle component
- **AND** each activates the quest rule processor for the quest entity pattern
- **AND** each loads the complete quest lifecycle rule pack, including the
  mechanics-stub rule

#### Scenario: Review-required quest completes without mechanics

- **GIVEN** a shipped Phase-1 profile starts from a fresh graph
- **WHEN** a review-required quest enters `in_review`
- **THEN** the mechanics-stub rule transitions it to `completed`
- **AND** the quest records `review-skipped-mechanics-disabled`
- **AND** no battle entity, XP award, reputation update, peer review, store
  mutation, inventory mutation, or artifact merge/index mutation is produced

### Requirement: False battle event contracts are deleted

The system SHALL delete raw battle event contracts that have no production
publisher instead of assigning them a projection owner or preserving them as
compatibility vocabulary.

#### Scenario: Five unproduced subjects disappear together

- **GIVEN** production code publishes none of `battle.review.started`,
  `battle.review.verdict`, `battle.review.victory`,
  `battle.review.defeat`, or `battle.review.retreat`
- **WHEN** issue #26 removes the false event facade
- **THEN** all five vocabulary constants and registrations are absent
- **AND** bossbattle exposes no raw battle-event or verdict port
- **AND** questboard exposes no battle-event output port
- **AND** dmsession has no `battle.review.>` routing or battle-event filter
- **AND** the three battle `GameEventType` values are absent
- **AND** no replacement payload registration, `BaseMessage` route,
  projection bind, heartbeat, or compatibility subject is added

#### Scenario: Definition-only retreat surface is removed

- **GIVEN** `BattleRetreatPayload` has no publisher, subscriber,
  payload-registry factory, graph-ingest route, API writer, or UI writer
- **WHEN** issue #26 removes it
- **THEN** the payload, `TraceInfo`, `Graphable` assertion, graph methods,
  tests, and root relationship fixture are absent
- **AND** the retreat-only battle status and UI filter/style/fixture surface is
  absent
- **AND** graph-manifest tests use an authoritative battle predicate rather
  than a deleted event subject
- **AND** no deleted retreat predicate is renamed or retained as an alias

### Requirement: Authoritative BossBattle remains dormant

Phase 1 SHALL retain the authoritative battle read model without activating
its unsafe writer. Reactivation SHALL remain blocked until its canonical
projection and owner-safe lifecycle handoffs are complete.

#### Scenario: Dormant authoritative read surface is retained

- **WHEN** issue #26 disables the bossbattle runtime
- **THEN** the `BossBattle` type, evaluator, factory registration, REST
  readers, dmworldstate readers, battle UI, and battle-state KV port remain
- **AND** `battle.assignment.quest` and `battle.assignment.agent` remain typed
  `@id` relationships
- **AND** raw event ports and event-derived state projections remain absent

#### Scenario: Dynamic indexed predicates block reactivation

- **GIVEN** the dormant writer constructs `battle.criteria.<index>.<field>`,
  `battle.judge.<index>.<field>`, and
  `battle.result.<index>.<field>`
- **WHEN** the beta.158 projection contract is validated
- **THEN** no wildcard or occurrence-level owner exception is accepted
- **AND** Phase 2 replaces those families with repeated bounded objects at
  `battle.review.criteria`, `battle.review.judges`, and
  `battle.review.results`
- **AND** canonical scalar predicates use the ledger's lower-kebab names
- **AND** the fresh-state cutover adds no legacy reader, alias, dual write, or
  in-place battle-state rewriter

#### Scenario: Reactivation uses framework ownership boundaries

- **WHEN** a later change reactivates bossbattle
- **THEN** owner `bossbattle` binds and heartbeats one authoritative battle
  `projection.Contract`
- **AND** graph-ingest birth and owned replacement carry its typed owner token
- **AND** quest changes cross the lifecycle Manager or rule-command boundary
- **AND** agent changes cross the agentprogression owner boundary
- **AND** review changes cross the review create and lifecycle boundaries
- **AND** artifact changes cross the `quest-artifacts` owner boundary
- **AND** DAG changes cross questdag and quest lifecycle
- **AND** agent-loop cancellation uses the typed framework signal contract
- **AND** no full-object cross-owner or direct shared-KV write remains

#### Scenario: The reusable mutation helper remains upstream-owned

- **GIVEN** SemStreams #313 owns the reusable Go owner-token mutation helper
- **WHEN** Phase 2 implements battle birth and owned replacement
- **THEN** it consumes the tagged upstream primitive
- **AND** issue #26 opens no duplicate helper issue
- **AND** a replace-only #313 delivery requires a linked upstream create
  helper before bossbattle activation
- **AND** Semdragon does not copy a generic mutator or retry framework locally

### Requirement: Disjoint authoritative writers

Every mutable predicate group SHALL have one declared owner and write mode.
No writer SHALL replace a full shared entity or another owner's predicates.

#### Scenario: Ownership overlap

- **WHEN** two owners claim an overlapping predicate group
- **THEN** startup or contract validation fails before either writer mutates
  authoritative state

### Requirement: Agentprogression is quarantined without a partial migration

Phase 1 SHALL retain the registered agentprogression product implementation
while disabling it in every shipped profile. It SHALL delete only the
unproduced XP projection facade and SHALL NOT present retained full-object
Agent writes as beta.158-compatible.

#### Scenario: Every shipped profile disables agentprogression

- **WHEN** contract tests parse the default, semsource, and tier-1 E2E
  configurations
- **THEN** each retains `components.agent_progression`
- **AND** each sets that component's `enabled` field to `false`
- **AND** each retains the complete quest rule pack and mechanics-stub rule
- **AND** Phase 1 produces no XP, level, cooldown, death, or ready mutation

#### Scenario: The dead XP projection facade is deleted

- **GIVEN** `AgentXPPayload` has no production publisher, subscriber,
  payload-registry factory, `BaseMessage` route, graph-ingest route, API
  reader, or UI reader
- **WHEN** issue #30 removes the facade
- **THEN** the payload, Graphable/schema/validation/trace surface, and
  `processor/agentprogression/payloads.go` are absent
- **AND** the false `agent-xp` output, `AGENT_EVENTS` stream declaration, and
  `agent.progression.>` subject declaration are absent
- **AND** the six event-only progression vocabulary constants and
  registrations are absent
- **AND** no replacement raw subject, payload route, projection owner, or
  compatibility predicate is added

#### Scenario: Product policy survives for Phase 2

- **WHEN** the Phase-1 quarantine is complete
- **THEN** the Agent model, triples, reconstruction, and validation remain
- **AND** the XP engine, award/penalty/failure types, and calculation policy
  remain
- **AND** component registration, configuration, handlers, and watchers remain
  dormant
- **AND** agentstore and the SemStreams dependency are unchanged
- **AND** no retained predicate is renamed and no owner or lifecycle contract
  is bound
- **AND** reactivation remains blocked on beta.158 and SemStreams #313

#### Scenario: Audit evidence measures rather than predicts

- **GIVEN** the historical package baseline is 22 invalid candidates from 32
  extracted
- **AND** exactly five invalid occurrences belong to the deleted payload
- **WHEN** the pinned beta.158 audit is rerun after implementation
- **THEN** it reports 17 invalid candidates from 26 extracted
- **AND** the measured delta is exactly five fewer invalid and six fewer
  extracted candidates
- **AND** the historical baseline audit remains unchanged

### Requirement: Agentstore is quarantined without migrating retained state

Phase 1 SHALL disable agentstore in every shipped profile and SHALL delete its
repository-unconsumed raw event plane. It SHALL retain the store product and
API surface as dormant Phase-2 code without presenting retained full-object
state writers as beta.158-compatible.

#### Scenario: Every shipped profile disables agentstore

- **GIVEN** the default profile already disables `agent_store`
- **WHEN** contract tests parse the default, semsource, and tier-1 E2E
  configurations
- **THEN** each retains `components.agent_store`
- **AND** each sets that component's `enabled` field to `false`
- **AND** each retains the complete quest rule pack and mechanics-stub rule

#### Scenario: Product and API surfaces remain dormant

- **WHEN** issue #32 completes the Phase-1 quarantine
- **THEN** store registration, configuration, models, validation, product
  policy, handlers, and watchers remain
- **AND** REST routes, OpenAPI, and generated models remain
- **AND** unavailable store endpoints retain their 503 response
- **AND** autonomy treats the unavailable store as a no-op
- **AND** no store or inventory side effect is produced

#### Scenario: The publisher-only raw event plane is deleted

- **GIVEN** the five raw payload families have production publishers
- **AND** no repository subscriber, payload factory, `BaseMessage`,
  graph-ingest route, replay reader, API reader, UI reader, or DM reader
  consumes them
- **WHEN** issue #32 removes the raw plane
- **THEN** all five DTOs, typed subjects, publishers, Graphable/schema methods,
  validation, trace fields, and assertions are absent
- **AND** all seven raw-event vocabulary constants and registrations are
  absent
- **AND** the false component output is absent
- **AND** authoritative EntityState KV message categories remain unchanged
- **AND** no replacement subject, graph route, owner, compatibility alias, or
  UI event mapping is added

#### Scenario: Audit delta isolates the retained migration

- **GIVEN** the historical package baseline is 23 invalid candidates from 30
  extracted
- **AND** payloads.go contributes 9 invalid candidates from 12 extracted
- **WHEN** the pinned beta.158 audit is rerun after issue #32
- **THEN** it reports 14 invalid candidates from 18 extracted
- **AND** all 14/18 belong to the retained store.go corpus
- **AND** the historical baseline audit remains unchanged

#### Scenario: Phase-2 framework handoff remains deferred

- **WHEN** issue #32 completes
- **THEN** agentprogression, the SemStreams dependency, retained predicates,
  entity IDs, owners, lifecycle workflows, graph views, API, and UI are
  unchanged
- **AND** reactivation remains blocked on beta.158 and SemStreams #313
- **AND** no duplicate upstream issue is opened
- **AND** a linked owned-create issue is opened only if Phase 2 proves #313
  does not supply the required inventory-entry and active-effect birth
  operation
- **AND** Phase 2 replaces agentstore `sync.Map` current-state reads with
  SemStreams graphview-derived projections
- **AND** purchases send an idempotent durable XP-debit command/request to the
  agentprogression owner after #313 enables owner-safe mutation
- **AND** agentstore never directly debits progression XP or replaces the full
  Agent entity
- **AND** Agent lifecycle and status predicates remain outside agentstore
  authority

### Requirement: Boidengine is quarantined without deleting live contracts

Phase 1 SHALL retain boidengine product policy and its live dormant suggestion
contracts while disabling the component in every shipped profile. The
supported shipped path SHALL use explicit claim and start rather than implying
that posted quests still advance autonomously.

#### Scenario: Every shipped profile disables boidengine

- **WHEN** contract tests parse the default, semsource, and tier-1 E2E
  configurations
- **THEN** each retains `components.boidengine`
- **AND** each sets that component's `enabled` field to `false`
- **AND** ComponentManager creates no boidengine instance
- **AND** startup creates no `BOID_SUGGESTIONS` bucket
- **AND** the complete quest rule pack and mechanics-stub rule remain enabled

#### Scenario: Supported Phase-1 quest flow is explicit

- **GIVEN** boidengine is the sole producer of `boid.suggestions.>` and
  `boid.guild.>`
- **AND** autonomy requires cached suggestions for claim, join, and form
  actions
- **WHEN** shipped Phase 1 starts without boidengine
- **THEN** autonomy causes no claim, guild join, or guild formation mutation
- **AND** seeded Agent, Quest, and Guild state remains bit-for-bit unchanged
  until an explicit claim
- **AND** REST API and test-fixture claim/start are the supported shipped paths
- **AND** the lifecycle operator joins that path only when separately enabled
  under task 4.3
- **AND** explicit claim/start can reach questbridge and mechanics-stub
  completion without battle, XP, store, or redteam side effects
- **AND** the documentation does not describe this quarantine as
  behavior-neutral

#### Scenario: Product policy and live dormant contracts remain

- **WHEN** issue #34 completes
- **THEN** registration, configuration, `DefaultBoidEngine`, rules,
  attraction/scoring policy, DTOs, and policy tests remain
- **AND** the dormant `boid.suggestions.>` / `boid.guild.>` contracts and
  `BOID_SUGGESTIONS` implementation remain
- **AND** dmpartyformation retains direct policy reuse
- **AND** positive boid E2E coverage is classified as opt-in/dormant
- **AND** only the unproduced shipped `agent.boid.>` stream entries are removed
- **AND** the pinned beta.158 boidengine audit remains zero structured
  candidates
- **AND** the historical baseline audit remains unchanged

#### Scenario: Future framework handoff remains deferred

- **WHEN** issue #34 completes
- **THEN** no fallback recommender, rules-based scorer, local projection
  framework, graphview, ownership, dependency, API/OpenAPI, schema, subject,
  or KV migration is added
- **AND** reactivation replaces raw ENTITY_STATES watchers and process-local
  caches with SemStreams `pkg/graphview`
- **AND** suggestions become a durable KV/stigmergic projection
- **AND** consumers migrate to that state before the duplicate core-NATS
  advisory plane is retired
- **AND** SemStreams #587 remains scoped to message-logger per-client KV-watch
  migration
- **AND** no duplicate upstream issue is opened

### Requirement: Redteam is quarantined without deleting product contracts

Phase 1 SHALL retain redteam product policy, model/API/UI fields, and dormant
message-category taxonomy while disabling the component in every shipped
profile. The quarantine SHALL remove false publication metadata without
pulling its deferred watcher, writer, lifecycle, or deadline migration into
this change.

#### Scenario: Every shipped profile disables redteam

- **WHEN** contract tests parse the default, semsource, and tier-1 E2E
  configurations
- **THEN** each retains `components.redteam`
- **AND** each sets that component's `enabled` field to `false`
- **AND** ComponentManager creates no redteam instance
- **AND** startup creates no redteam watcher, pending review, or deadline
- **AND** the complete quest rule pack and mechanics-stub rule remain enabled

#### Scenario: False OutputPorts are removed and dormant contracts remain

- **GIVEN** redteam has no NATS publisher, raw event DTO, or shipped stream
  subject
- **WHEN** issue #36 completes
- **THEN** the `redteam.lifecycle.posted` and
  `redteam.lifecycle.completed` OutputPorts are absent
- **AND** no replacement subject, payload, publisher, stream, or compatibility
  alias is added
- **AND** the `redteam.lifecycle.*` category/vocabulary taxonomy remains for
  EntityState message categories and product/UI event types
- **AND** registration, configuration, Quest graph/model fields,
  finding/lesson policy, prompt context, API/UI surfaces, and positive opt-in
  E2E remain dormant Phase-2 contracts
- **AND** the historical predicate-audit baseline remains unchanged

#### Scenario: Supported Phase-1 review flow has no redteam side effects

- **WHEN** a review-required Quest is explicitly claimed and started in a
  shipped Phase-1 profile
- **THEN** questbridge executes it and mechanics-stub completes it with
  `review-skipped-mechanics-disabled`
- **AND** no red-team Quest, red-team status mutation, Guild lesson, watcher,
  deadline, or redteam event is produced
- **AND** positive red-team E2E remains opt-in rather than shipped-runtime
  coverage

#### Scenario: Retained implementation repairs remain deferred

- **WHEN** issue #36 completes
- **THEN** the raw watcher still reads legacy
  `quest.classification.red_team_target` rather than canonical
  `quest.classification.red-team-target`
- **AND** transition and pending-review state remains in process-local caches
- **AND** full-Quest update/CAS and full-Guild lesson writers remain inactive
  and are not treated as ownership-safe
- **AND** `ClaimTimeoutSec` remains inert
- **AND** execution deadlines remain restart-fragile local goroutines
- **AND** reactivation remains blocked on Semdragon #4, #5, and #22 plus
  SemStreams #313 and #676
- **AND** no dependency, graphview, ownership, writer, lifecycle, rule,
  API/UI/schema, or durable-deadline migration is included

### Requirement: Dead party events and DAG shadows are deleted

The system SHALL retain partycoord only as the current Party formation,
roster, membership, temporary assignment-advisory, and disband authority.
It SHALL delete the unconsumed raw party event plane and partycoord-local
decomposition, result, and rollup shadows without creating a replacement
projection.

#### Scenario: No-consumer party event plane disappears

- **GIVEN** eight party DTO families publish raw core-NATS JSON
- **AND** no production subscriber, payload factory, `BaseMessage`,
  graph-ingest route, owner, replay path, API reader, or UI reader consumes
  them
- **WHEN** issue #28 removes the dead event plane
- **THEN** all eight DTOs, typed subjects, publisher calls,
  validation/schema/Graphable methods, DTO tests, and relationship fixtures
  are absent
- **AND** the DTO inventory is exactly `PartyFormedPayload`,
  `PartyDisbandedPayload`, `PartyJoinedPayload`,
  `PartyQuestDecomposedPayload`, `PartyTaskAssignedPayload`,
  `PartyResultSubmittedPayload`, `PartyRollupStartedPayload`, and
  `PartyRollupCompletedPayload`
- **AND** all thirteen party-event vocabulary entries are absent
- **AND** partycoord exposes neither `party-formed` nor `party-disbanded`, and
  dmpartyformation exposes no `party-events` output port
- **AND** dmsession exposes neither `EventPartyFormed` nor
  `EventPartyDisbanded` or a corresponding filter fixture
- **AND** no rule publish, compatibility subject, payload registration,
  projection bind, owner, or heartbeat replaces them

#### Scenario: Authoritative Party and KV activity remain

- **GIVEN** form, join, and disband persist `Party` through
  `GraphClient.EmitEntity`
- **AND** the service SSE stream watches authoritative KV entity changes
- **WHEN** the raw party subjects are deleted
- **THEN** Party list/detail, dmworldstate, prompt, and UI readers remain
  backed by the Party entity
- **AND** `party.formed`, `party.joined`, and `party.disbanded` remain message
  categories mapped by KV-backed SSE
- **AND** form, join, and disband succeed without a party event subscriber

#### Scenario: Shadow DAG and rollup state is absent

- **GIVEN** `DecomposeQuest`, `SubmitResult`, `StartRollup`, and
  `CompleteRollup` have no production caller
- **WHEN** issue #28 narrows partycoord
- **THEN** those methods and their tests are absent
- **AND** strategy, shared-context, sub-result, and rollup-result fields,
  triples, reconstruction, metrics, prompt/UI rendering, generated types,
  fixtures, and documentation are absent
- **AND** questdag and quest lifecycle remain the owners of node result,
  retry, escalation, clarification, and rollup behavior
- **AND** temporary `AssignTask` and `SubQuestMap` remain until both DAG
  implementations move off the advisory seam

#### Scenario: Audit reduction does not disguise authoritative debt

- **GIVEN** partycoord has 14 beta.158 audit findings
- **WHEN** issue #28 deletes the latent event Graphables and shadow-only
  strategy state
- **THEN** the audit drops exactly ten findings: nine event projections and
  `party.strategy`
- **AND** exactly four scanner-visible authoritative Party findings remain:
  `party.quest`, `party.lead`, `party.lifecycle.formed_at`, and
  `party.lifecycle.disbanded_at`
- **AND** scanner-hidden dynamic member-role and quest-assignment predicate
  families remain explicitly recorded as deferred defects
- **AND** no rename-only patch, alias, wildcard claim, or auditor exemption
  presents the retained full-object projection as canonical

#### Scenario: Dynamic UI parsing is not promoted to a contract

- **GIVEN** the UI reconstructs member roles and advisory assignments by
  matching dynamic predicate prefixes
- **WHEN** issue #28 updates UI and generated artifacts
- **THEN** it removes only deleted shadow-field rendering and fixtures
- **AND** it neither generalizes nor labels the dynamic parser as
  beta.158-compatible
- **AND** a later canonical packet replaces first-class membership and
  assignment producers and their UI readers atomically

#### Scenario: Canonical Party ownership remains a framework handoff

- **WHEN** the authoritative Party projection is migrated after issue #28
- **THEN** rules detect party-required quests
- **AND** partycoord policy owns roster selection, first-class membership, and
  disband intent
- **AND** SemStreams lifecycle Manager owns Party phase, history, contention,
  and restart recovery
- **AND** quest lifecycle owns quest phase and quest assignment
- **AND** questdag owns DAG-unit assignee, output, retry, and rollup state
- **AND** SemStreams gated-DAG owns order, claim, redelivery, and stalls
- **AND** issue #4 consumes the public owner-token mutation path tracked by
  SemStreams #313 rather than copying framework internals

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
