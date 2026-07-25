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
