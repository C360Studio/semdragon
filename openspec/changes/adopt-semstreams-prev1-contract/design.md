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
- questdag owns product DAG projection, immutable unit provenance, dependency
  edges, and the temporary activation-latch admission protocol;
- gated-DAG owns `gateddag.unit.claim`;
- private Semdragon consumer seams own the exact completed, failed, and reset
  marker predicates that beta.158 explicitly delegates to the consumer;
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

### D7: Deterministic activation-latch admission

SemStreams beta.158 has no generic staged fan-out admission primitive. Its
executor continuously reads every real unit under the configured prefix, so
creating real units before installing their dependencies can dispatch an
incomplete plan. Graph mutation batches are not atomic across entities, and
`FanOutInstanceID` does not gate unit evaluation.

Issue #3 therefore uses a narrow product bridge while still pinned to beta.135:

- execution ID is the idempotency key, and validator-safe unit IDs are
  deterministically derived from execution and node identity;
- a canonical plan digest binds the execution ID to one immutable manifest;
- a real, deterministic activation unit is born atomically with a self
  dependency;
- every work unit is created prerequisite-first with its complete dependency
  set and an additional activation-unit dependency in the same atomic entity
  birth;
- the complete manifest is authoritatively reconciled before activation; and
- a private `questdag-completion` writer releases the latch with one
  idempotent, classified replace-by-predicate completion mutation.

While issue #3 remains pinned to beta.135, every completion attempt is
confirmed by an authoritative entity read-back. Beta.135 cannot validate a
decoded success echo, so the writer does not treat that echo as authoritative.
After issue #4 adopts beta.158, the writer may trust a non-degraded echo only
after the framework's decoded-entity validator accepts it. Lost or degraded
replies still require authoritative read-back.

No work unit can dispatch before activation, no dependency target is born as a
stub, and a partial seed remains held. A replay reuses the same IDs, accepts
only matching immutable seed contracts, and converges after a lost create or
activation reply. An already activated plan with missing or conflicting units
is corruption, not a repair opportunity.

This bridge is product composition, not a reusable local framework.
[SemStreams #670](https://github.com/C360Studio/semstreams/issues/670) owns the
generic staged-admission primitive and retires the latch after dependency
adoption. Issue #3 does not bump SemStreams to beta.158.

### D8: Upstream framework boundaries stay explicit

Semdragon does not turn temporary product seams into local framework code:

- [SemStreams #313](https://github.com/C360Studio/semstreams/issues/313) is the
  existing public ask for a reusable owned-write helper. Its
  [Semdragon evidence comment](https://github.com/C360Studio/semstreams/issues/313#issuecomment-5078996034)
  records this repository as a second consumer. The narrow private completion
  seam is sufficient for this core wave, so #313 is not a blocker.
- [SemStreams #671](https://github.com/C360Studio/semstreams/issues/671) tracks
  occurrence-exact, stale-checked classifications for the three non-predicate
  `quest.failed` audit findings. It does not block the DAG admission wave, but
  task 3.1 remains open until the full predicate audit has exact evidence.
- [SemStreams #670](https://github.com/C360Studio/semstreams/issues/670) owns
  generic staged fan-out admission and retires the activation latch.
- [SemStreams #673](https://github.com/C360Studio/semstreams/issues/673) owns
  the missing standard `dc.terms.modified` constant and vocabulary
  registration. Issue #16 may carry one linked beta.135 compatibility
  declaration; the first adopted tag containing #673 deletes it.
- [semdragon #4](https://github.com/C360Studio/semdragon/issues/4) owns the
  beta.158 dependency adoption, decoded-entity validation, and registry
  wiring.

### D9: Dead projections are deleted, not assigned phantom owners

Issue
[#12](https://github.com/C360Studio/semdragon/issues/12) initially proposed
canonicalizing and binding `PromptAssembledPayload`. Source and history review
found no production publisher, no payload-registry registration, and no
promptmanager lifecycle component. The type and typed subject have been
definition-only since introduction; their only downstream use is a graph
relationship fixture and vocabulary metadata.

Issue #12 therefore deletes the dead `Graphable`, typed subject, vocabulary
entry, and fixture. It does not create a component, `projection.Contract`,
owner registration, or heartbeat. A library package that assembles values is
not a graph writer merely because several real components call it.

`TestPromptAssemblyHasNoGraphProjectionSurface` in
`prompt_projection_inventory_contract_test.go` stale-checks the production
source inventory and rejects reintroduction of this exact dead surface or
external promptmanager registration hooks. It does not ban SemStreams
projection contracts, ownership services, heartbeats, or lifecycle timers for
real publishers.

If prompt-assembly evidence is introduced later, the actual publishing
component declares and binds the framework contract through the SemStreams
ownership lifecycle. The contract uses:

- message type and contract name
  `semdragons.prompt.assembly.completed.v1`;
- entity pattern `*.*.game.*.quest.*`;
- one `append-evidence` group containing
  `prompt.assembly.completed`, `prompt.assembly.agent-id`,
  `prompt.assembly.domain`, `prompt.assembly.tier`,
  `prompt.assembly.provider`, and `prompt.assembly.fragment-count`; and
- no `ForeignEdgeClaim`.

The agent relationship is a typed `@id` object on a triple whose subject is the
payload's quest entity. It is ordinary same-subject projection data, not a
foreign edge. `projection.Contract` has no source field, and triple `Source`
metadata does not establish ownership. The binding owner ID must name the
actual publisher, not the promptmanager library.

### D10: Autonomy event and graph publication remain separate contracts

Issue [#14](https://github.com/C360Studio/semdragon/issues/14) canonicalizes
the seven autonomy event payload Graphables while Semdragon remains pinned to
beta.135. The autonomy component is a real production event publisher:
`natsclient.Subject[T]` serializes each payload as raw JSON on its typed event
subject. That publish path is not a SemStreams `BaseMessage`, is not registered
in the binary payload registry, and is not routed through `graph.ingest.entity`.
The existing `Graphable` methods therefore declare a prospective graph shape;
they do not prove current graph persistence.

Issue #14 declares and emits exactly 21 canonical autonomy predicates. The
`claim-quest`, `guild-join`, and `use-quest` objects are typed `@id`
relationships. Their triple subject is the payload's agent `EntityID()`, so
they are ordinary same-subject projection data and require no
`ForeignEdgeClaim`. `shop-item` and `use-consumable` remain scalar catalog
aliases. `guild-create-candidates` remains a scalar count, and
`guild-choices` remains the scalar number of choices evaluated.

Issue #14 does not add a payload-registry factory, graph-ingest route,
`projection.Contract`, owner registration, or heartbeat. Issue
[#4](https://github.com/C360Studio/semdragon/issues/4) must make one explicit
choice after beta.158 is adopted:

1. wire the autonomy component through the framework `BaseMessage`,
   payload-registry, and graph-ingest primitives; or
2. delete the seven latent Graphable surfaces and their graph-only tests if
   persisted autonomy evidence is not a product requirement.

If issue #4 wires the real graph route, owner `autonomy` binds seven contracts,
one per payload message type, over entity pattern `*.*.game.*.agent.*`. Each
contract has one `append-evidence` predicate group, no foreign edges, and no
indexing profile. Because beta.158 liveness compaction exempts only
foreign-edge-only registrations, the static owner uses
`projection.BindAndHeartbeat` with a process-lifetime heartbeater. Binding
occurs once at the composition root after ownership initialization and before
the autonomy component starts, never inside `Component.Start`.

### D11: Executor observations, dead telemetry, and web state are distinct

Issue [#16](https://github.com/C360Studio/semdragon/issues/16) partitions the
35 executor audit candidates and nine findings by runtime behavior.

`ExecutionStartedPayload`, `ExecutionCompletedPayload`, and
`ExecutionFailedPayload` are live raw typed-subject events. Both
`processor/executor` and `processor/questbridge` publish these families.
Questbridge consumes framework agentic-loop `BaseMessage`s, but its executor
observation publish is still raw JSON. No executor payload factory or event
route to `graph.ingest.entity` exists, so the payload Graphables declare a
prospective shape and do not prove graph persistence.

The three live payloads emit 15 unique declared canonical predicates. Only
`execution.agent.id` is an `@id` relationship, and it is a same-subject edge on
the payload's quest entity. The other lifecycle, loop, configuration, metrics,
token, duration, status, and failure values are scalar.

`ToolCallPayload`, `ToolResultPayload`, their typed subjects, vocabulary
metadata, and graph relationship fixtures have no production publisher or
consumer. Issue #16 deletes that seven-predicate surface rather than creating a
phantom graph owner. Issue
[#7](https://github.com/C360Studio/semdragon/issues/7) owns any later tool
telemetry requirement through SemStreams agentic-tools and trajectories.

The web-fetch surface is different. `persistWebContent` is a real writer that
passes `webContentEntity` to the full-object `GraphClient.EmitEntity` path.
Issue #16 canonicalizes `source.doc.mime-type`, `source.doc.fetched-by`, and
`source.doc.quest-id`; the last two are same-subject `@id` relationships on the
document entity. Together with `execution.agent.id`, they are exactly the three
`@id` relationships remaining in `processor/executor`.

SemStreams supplies and registers `vocabulary.DCTermsTitle`, which the writer
must reuse. Beta.158 does not supply or register `dc.terms.modified`, while
`projection.Contract.Validate` requires declared vocabulary. SemStreams
[#673](https://github.com/C360Studio/semstreams/issues/673) owns that standard
primitive. Issue #16 may add one isolated, linked beta.135 compatibility
constant and registration; it is not product vocabulary and must be deleted
when Semdragon adopts the first SemStreams tag containing #673.

Issue #16 adds no `projection.Contract`, owner bind, heartbeat, payload
registration, graph-ingest route, or owner-safe web mutation. Issue
[#4](https://github.com/C360Studio/semdragon/issues/4) wires or deletes the
three latent lifecycle projections after choosing the real graph publisher.
Because executor and questbridge both publish the raw events, neither package
may claim sole ownership merely because the payload definitions live in
executor.

If a real graph route is approved, the three lifecycle shapes are
`semdragons.execution.started.v1`,
`semdragons.execution.completed.v1`, and
`semdragons.execution.failed.v1`. Each uses pattern
`*.*.game.*.quest.*`, one `append-evidence` group, no foreign edges, and no
indexing profile. Every static graph publisher binds only its actual shapes
with `projection.BindAndHeartbeat` at the composition root.

A later web-document packet owns the `executor-web-fetch` replace-owned
contract over `*.*.web.agent.doc.*`, including replacement of the raw
full-object write with the supported owner-classified framework lane. Its nine
predicates include the seven `source.doc.*` values plus `dc.terms.title` and
`dc.terms.modified`; its two typed relationships require no
`ForeignEdgeClaim`.

### D12: Graph-source query prefixes are segment-bound routing keys

Issue [#18](https://github.com/C360Studio/semdragon/issues/18) replaces three
`local.semsource.` values and one `osh.` value with canonical
`local.semsource` and `osh` query prefixes. `GraphSource.EntityPrefix` is a
one-to-six-segment entity-ID namespace used to route entity, relationship, and
prefix queries; it is not a NATS subject prefix and does not carry a delimiter
sentinel.

The existing registry uses lexical `strings.HasPrefix` and historically relied
on the configured trailing dot to prevent adjacent-prefix matches. Removing
only the dots would make `osh` match `oshx` and `local.semsource` match
`local.semsourcex`. Issue #18 therefore changes routing atomically to:

```go
id == prefix || strings.HasPrefix(id, prefix+".")
```

Exact prefixes and dot-delimited descendants route to the owning source.
Lexically adjacent prefixes do not. The canonical value is forwarded directly
to `entitiesByPrefix`; legacy trimming is not retained as a compatibility
reader. API and UI source metadata expose the same canonical value, and the
example cache remains in-process and non-persistent.

Prefix resolution has three outcomes, not a nullable two-state result:

1. a matching ready owner routes only to that source;
2. a matching unready owner routes to no source; and
3. no matching owner uses the configured local fallback.

The current nullable lookup collapses outcomes 2 and 3, causing
`SourcesForQuery` to fan out after an external owner matches but is unready.
That can leak an externally owned entity or prefix query to local or unrelated
ready sources. Issue #18 preserves source ownership while unavailable and
limits fallback to genuinely unowned prefixes.

SemStreams beta.158 provides only part of the final boundary.
`types.ValidateEntityIDPrefix` validates a one-to-six-segment query prefix.
`graph.MatchesAnyIDPrefix` implements exact-or-dot-descendant matching only
when its first argument is a complete six-part entity ID; it calls
`ValidateEntityID` and therefore rejects partial query prefixes such as `osh`
and `osh.asset`.

Issue [#4](https://github.com/C360Studio/semdragon/issues/4) uses
`graph.MatchesAnyIDPrefix` for concrete entity routing and
`types.ValidateEntityIDPrefix` at the configuration/query boundary. The
prefix-to-prefix branch still needs the isolated equality-or-dot-descendant
comparison.
[SemStreams #675](https://github.com/C360Studio/semstreams/issues/675) owns the
reusable validated prefix-containment matcher. If the adopted tag supplies that
helper, issue #4 deletes the local comparison; otherwise the minimal comparison
remains linked to #675 until the first tagged release containing it. It must
not be expanded into a generic local prefix library.

### D13: A raw event DTO does not imply a graph projection

Issue [#20](https://github.com/C360Studio/semdragon/issues/20) separates the
dmsession event contract from its definition-only graph surface.
`SessionManager.StartSession` and `SessionManager.EndSession` contain raw JSON
publishes through the typed `dm.session.start` and `dm.session.end` subjects.
Production methods in `processor/dmsession/component.go` call those manager
methods internally. However, no shipped configuration activates the
factory-registered dmsession component, and no production code outside
`processor/dmsession` calls its component entrypoints. The retained event DTOs
and publisher methods are therefore available product contracts, not evidence
of active runtime publication.

The binary does not register either payload in the SemStreams payload registry,
wrap it in a `BaseMessage`, route its subject to `graph.ingest.entity`, or bind
a projection owner. No graph or UI reader consumes the emitted triples. The
DTOs' `EntityID`, `Triples`, and `Schema` methods and `Graphable` assertions are
definition-only and are deleted with the latent `dm.session.mode`,
`dm.session.name`, `dm.session.quests_completed`, and
`dm.session.quests_failed` graph predicates. Issue #20 does not rename those
predicates, invent an owner, or add a graph route.

The raw JSON contract remains separate. Its typed subjects
`dm.session.start` and `dm.session.end` stay registered, and the
`quests_completed` and `quests_failed` fields remain numeric JSON properties
in `domain.SessionSummary`, OpenAPI, and generated UI types. They are not graph
predicate aliases. If a later real projection needs those facts, it introduces
canonical `dm.session.quests-completed` and
`dm.session.quests-failed` predicates only with an authoritative session-state
route and owner.

The actual state mutation is not the event Graphable.
`SessionManager.putSession` serializes `domain.Session` directly and writes it
under `session.<instance>` in `BoardConfig.BucketName()`, the shared
`ENTITY_STATES` board bucket. The value is not `graph.EntityState`, and the key
is not the canonical six-part session entity ID. This is downstream misuse,
not a missing SemStreams primitive and not a reason to bind the event DTOs.

Issue [#4](https://github.com/C360Studio/semdragon/issues/4) or a dedicated
packet replaces that raw write with the authoritative SemStreams model. It
must evaluate the framework lifecycle manager for an `active` to `ended`
workflow and use canonical entity birth and owner-classified state mutation;
it must not add a local lifecycle engine or resurrect event Graphables by
default. If an owner claim is needed, its real composition root supplies the
framework binding and heartbeat. Issue #20 defines no projection writer.

### D14: Active raw guild events do not become a second state projection

Issue [#22](https://github.com/C360Studio/semdragon/issues/22) separates five
active guildformation raw event contracts from their latent graph methods. The
component is factory-registered and enabled in all three shipped
configurations. `CreateGuild`, `JoinGuild`, `LeaveGuild`, `PromoteMember`, and
`DisbandGuild` publish the created, joined, left, promoted, and disbanded DTOs
through their typed NATS subjects. Those DTOs, subjects, `Validate` methods,
publisher calls, and JSON wire fields remain unchanged.

Raw publication is not graph persistence. The binary has no guild event
payload-registry factory, does not wrap the DTOs in SemStreams `BaseMessage`,
does not route their subjects through `graph.ingest.entity`, and binds no
projection owner or heartbeat. No production graph or UI reader consumes the
event triples. The five `Graphable` assertions and all fifteen `EntityID`,
`Triples`, and `Schema` methods are therefore deleted rather than assigned a
second owner.

The deleted shapes are also not a coherent authoritative model.
`GuildCreatedPayload` repeats the complete `domain.Guild` projection and adds
`guild.event.created_by`, even though the authoritative Guild shape already
uses the typed `guild.founding.agent` relationship. The joined and promoted
payloads construct dynamic `guild.member.<agent-id>.rank` predicates, while the
approved target uses first-class membership entities. The left-event
relationship is event evidence, not current membership state.
`guild.lifecycle.disbanded_at` has no field in `domain.Guild`, reconstruction,
API, or UI state, so issue #22 deletes it instead of inventing a stored
timestamp. The raw `founder_id`, `agent_id`, `timestamp`, and other JSON fields
are wire data, not graph aliases.

The actual guild-state boundary remains separate and unsafe.
Guildformation writes full `domain.Guild` snapshots for creation, membership,
promotion, applications, activation, and dissolution. Agentprogression also
writes full Guild snapshots for performance statistics and contribution, while
redteam writes Guild snapshots for lessons. Guildformation additionally keeps
restart-fragile in-memory guild/member maps and implements pending-guild expiry
with a local ticker. Issue #22 neither blesses those writers nor adds a
projection contract.

SemStreams already provides lifecycle Manager transitions, rule actions and
scheduling, projection ownership, owner-classified graph mutation, and graph
clustering primitives. No missing framework primitive blocks deletion of the
event Graphables, so issue #22 creates no new upstream ask. Issue
[#4](https://github.com/C360Studio/semdragon/issues/4) or a dedicated
guild-state packet must move status transitions to the lifecycle substrate,
replace full-object multiwriter mutation with exact ownership, model
memberships and applications as first-class children, and replace the local
pending-timeout loop with rules/lifecycle composition. Any adoption of
SemStreams graph clustering must account for existing upstream clustering
issues rather than copying another local clustering framework.

### D15: Retire the unactivated seeding processor

Issue [#24](https://github.com/C360Studio/semdragon/issues/24) deletes the
entire `processor/seeding` package rather than canonicalizing four predicates
or retaining only its raw DTO layer. The five package files were introduced
together as an event-driven replacement, but no consumer was ever added.
`Component.Start` subscribes to nothing, no code outside the package calls
`Seed`, and none of the three shipped configurations activates the registered
component.

All four event families are therefore definition-only runtime-dead surfaces.
The package has no payload-registry factory, `BaseMessage`, graph-ingest route,
projection contract, owner bind, heartbeat, graph reader, API reader, or UI
reader. Issue #24 deletes:

- all five files under `processor/seeding`;
- the package import, two `seeding.Register` callbacks, and two
  `seeding.ComponentName` list entries in `componentregistry/register.go`;
- the seven otherwise-unused seeding vocabulary constants and their
  registrations: `PredicateSeedingStarted`, `PredicateSeedingCompleted`,
  `PredicateArenaRoundStarted`, `PredicateArenaRoundCompleted`,
  `PredicateRosterAgentCreated`, `PredicateNPCSpawned`, and
  `PredicateNPCRetired`;
- the four root relationship-fixture rows and seeding import; and
- the stale processor documentation.

The deleted DTOs have no compatibility promise. Their four typed subjects,
`Validate` methods, publisher calls, JSON fields, four `Graphable` assertions,
and twelve graph-only methods disappear with the package. The thirteen
predicate occurrences are deleted rather than renamed. The
`agent.guild.member` fixture is removed with `AgentSeededPayload`; authoritative
Agent and Guild relationships remain unchanged.

Startup fixture seeding is a separate path. `cmd/semdragons/seed_e2e.go` is
called from the binary for `SEED_AGENTS`, `SEED_E2E`, and `SEED_PROS`; it
creates authoritative `agentprogression.Agent` state and is retained. Issue
[#4](https://github.com/C360Studio/semdragon/issues/4) or a dedicated
startup-birth packet moves that direct shared-KV full-object write through
SemStreams graph-ingest create-with-triples and the agentprogression ownership
boundary. Create-only Agent birth facts remain outside mutable owner claims;
initial status, progression, statistics, and skills cross the real owner API.

SemStreams beta.158 already provides classified request/reply,
create-with-triples, lifecycle, rules, bounded dispatch, projection contracts,
and ownership. Deleting the unused processor exposes no framework gap and
creates no upstream issue. Any future evaluation arena is a new product
composition over those primitives, not a reason to preserve or resurrect the
seeding event Graphables.

### D16: Disable bossbattle before rebuilding its authoritative projection

Issue [#26](https://github.com/C360Studio/semdragon/issues/26) separates the
Phase-1 mechanics cut from the later authoritative battle-state migration.
Before issue #26, bossbattle was enabled in all three shipped configurations.
The default configuration also enabled the mechanics-stub rule, so two
independent consumers reacted to the same transition into `in_review`: the
lifecycle rule completed the quest while bossbattle could create a battle and
later replace Quest, Agent, and PeerReview snapshots. That race violated the
Phase-1 contract that review-required quests complete without a battle or
mechanics side effects.

Phase 1 removes bossbattle activation from:

- `config/semdragons.json`;
- `config/semdragons-semsource.json`; and
- `config/semdragons-e2e-tier1.json`.

Each profile enables the complete quest lifecycle rule pack, including the
mechanics-stub rule and the quest entity watch. Removing bossbattle without the
stub from the alternate profiles would leave review-required quests in
`in_review`. The other four mechanics processors remain owned by the broader
lifecycle task; issue #26 does not claim that the five-component disablement is
complete.

The component factory, evaluator, authoritative `BossBattle` type, REST
readers, dmworldstate readers, battle UI, and real battle-state KV output port
remain dormant. The quest and agent assignment triples remain typed `@id`
relationships. Factory registration is not runtime activation and preserves
the accepted Phase-2 code-retention boundary.

Issue #26 deletes the false raw-event facade. No production code publishes any
of `battle.review.started`, `battle.review.verdict`,
`battle.review.victory`, `battle.review.defeat`, or
`battle.review.retreat`. The five vocabulary constants and registrations,
bossbattle raw event/verdict ports, questboard's false battle-event port,
dmsession battle-subject routing, and the three battle `GameEventType` values
therefore disappear together.

`BattleRetreatPayload` is also definition-only. It has no publisher,
subscriber, payload-registry factory, `BaseMessage` route, graph-ingest route,
projection binding, API writer, or UI writer. Its `TraceInfo`, tests, root
relationship fixture, retreat-only status, and retreat UI fixtures are
deleted. Synthetic graph-manifest tests retain battle-family filtering with a
real authoritative predicate rather than preserving a nonexistent event
subject.

This deletion does not canonicalize the authoritative battle projection.
`BossBattle.Triples` still constructs runtime-indexed predicates:

- `battle.criteria.<index>.<field>`;
- `battle.judge.<index>.<field>`; and
- `battle.result.<index>.<field>`.

Those predicates are a blocking defect, not an auditor exception. Beta.158
projection and ownership contracts require exact, declared predicates, so no
owner can claim an unbounded indexed family. Phase 2 uses the fresh-state
cutover and writes only repeated bounded objects at
`battle.review.criteria`, `battle.review.judges`, and
`battle.review.results`. It also adopts the canonical scalar names recorded in
the predicate ledger. No legacy reader, alias, dual write, or in-place battle
rewriter is added.

On reactivation, bossbattle binds one authoritative battle
`projection.Contract` as static owner `bossbattle`, heartbeats that owner, and
uses its typed owner token for graph-ingest birth and owned replacement.
Bossbattle supplies review results but does not mutate foreign authoritative
state:

- quest phase, attempts, assignment, failure, verdict, and completion cross
  the quest lifecycle Manager or rule-command boundary;
- agent status and progression cross the agentprogression owner boundary;
- review birth and completion cross the review create/lifecycle boundaries;
- artifact merge/index state crosses the `quest-artifacts` owner boundary;
- DAG failure, reset, and rollup cross questdag and quest lifecycle; and
- agent-loop cancellation remains a typed framework `UserSignal`.

[SemStreams #313](https://github.com/C360Studio/semstreams/issues/313) is the
existing upstream request for the reusable Go owner-token mutation helper.
Its Semdragon evidence already asks for create, replace-owned, append-evidence,
classified retry, and authoritative read-back support. Issue #26 opens no
duplicate. If #313 is delivered as replace-only, the missing create helper
must be split upstream before reactivation rather than copied locally.

Bossbattle remains blocked from every shipped configuration until the Phase-2
projection, ownership, lifecycle handoffs, fresh-state tests, and #313-backed
mutation path are complete. Merely renaming the eight scanner findings does
not satisfy this gate.

### D17: Partycoord survives narrowly; its dead event and DAG shadows do not

Issue [#28](https://github.com/C360Studio/semdragon/issues/28) resolves the
partycoord boundary without attempting the beta.158 authoritative projection.
Partycoord remains a product component because rules and gated-DAG do not own
lead selection, roster composition, party membership policy, or the decision
to disband. It does not survive as a second lifecycle or DAG engine.

The current component has two independent output planes:

1. `Party` is written through `GraphClient.EmitEntity` on form, join, and
   disband. Party REST readers, dmworldstate, the UI, and the board KV watcher
   consume this authoritative current-state projection.
2. Eight party event DTOs publish raw core-NATS JSON through typed subjects.
   No production subscriber, `BaseMessage`, payload factory, graph-ingest
   route, owner, replay path, API reader, or UI reader consumes those messages.
   Their `Graphable` methods are latent projections only.

Issue #28 deletes the second plane completely rather than preserving a
no-consumer wire facade. This differs from the dmsession and guildformation
decisions: those packages retain demonstrably published or internally consumed
raw event contracts, while the party subjects have no runtime consumer
contract. The deletion includes all eight DTOs, typed subjects, publishers,
validation/schema/Graphable methods and tests; all thirteen party-event
vocabulary entries; party event output ports in partycoord and the phantom
dmpartyformation output port; the two unrouteable dmsession party
`GameEventType` values; and the event relationship fixtures. It adds no
replacement rule `publish`, payload registration, graph route, projection
owner, heartbeat, or compatibility subject.

The exact eight DTO families are `PartyFormedPayload`,
`PartyDisbandedPayload`, `PartyJoinedPayload`,
`PartyQuestDecomposedPayload`, `PartyTaskAssignedPayload`,
`PartyResultSubmittedPayload`, `PartyRollupStartedPayload`, and
`PartyRollupCompletedPayload`. The thirteen vocabulary symbols are
`PredicatePartyFormed`, `PredicatePartyDisbanded`, `PredicatePartyJoined`,
`PredicatePartyLeft`, `PredicatePartyQuestDecomposed`,
`PredicatePartyTaskAssigned`, `PredicatePartyContextShared`,
`PredicatePartyGuidanceIssued`, `PredicatePartyProgressReported`,
`PredicatePartyHelpRequested`, `PredicatePartyResultSubmitted`,
`PredicatePartyRollupStarted`, and `PredicatePartyRollupCompleted`. The ports
are partycoord's `party-formed` and `party-disbanded` plus dmpartyformation's
`party-events`; the phantom DM values are `EventPartyFormed` and
`EventPartyDisbanded`.

The retained KV activity feed is not part of that event plane. The service SSE
stream watches authoritative `EntityState` changes and derives
`party.formed`, `party.joined`, and `party.disbanded` from the message category
stamped by the retained `EmitEntity` calls. Those category-to-UI mappings
remain. Removing them would break live Party activity while retaining an
unused NATS facade, the inverse of the intended boundary.

Partycoord also carries an in-memory shadow of DAG execution that no production
caller reads: decomposition strategy, shared context, submitted node results,
and rollup result. `DecomposeQuest`, `SubmitResult`, `StartRollup`, and
`CompleteRollup` are test-only. Issue #28 deletes those methods; `Strategy`,
`SharedContext`/`ContextItem`, `SubResults`, and `RollupResult`; their triples,
reconstruction, metrics, prompt/UI rendering, generated types, fixtures, and
documentation. Questdag and quest lifecycle own node output, retry,
clarification, escalation, and rollup.

`AssignTask` and `SubQuestMap` remain temporary because both the selected
legacy `questdagexec` and the replacement `questdag` still call that advisory
seam. They are not approved as the canonical assignment projection. The later
questdag activation packet moves DAG-unit assignee authority to questdag and
removes the advisory shadow only when its callers and compatibility tests move
atomically.

The package baseline is 14 beta.158 audit findings. Nine belong to the deleted
event Graphables, and deleting shadow-only `Party.Strategy` removes one more
authoritative finding. Issue #28 therefore removes exactly ten and leaves four:
`party.quest`, `party.lead`, `party.lifecycle.formed_at`, and
`party.lifecycle.disbanded_at`. Those four scanner-visible findings on the
retained authoritative Party projection remain deferred, together with
scanner-hidden `party.member.<agent-id>.role` and
`party.assignment.<quest-id>` families. This is deliberate: a rename-only
patch would preserve the unsafe full-object writer and dynamic vocabulary under
new spellings.

The UI currently reconstructs member roles and advisory assignments by
matching those dynamic predicate families. That parser is a migration trap,
not a canonical contract. Issue #28 may update generated types and remove UI
rendering for deleted shadow fields, but it must neither generalize those
dynamic-prefix parsers nor present them as beta.158-compatible. The later
canonical packet replaces membership and assignment with first-class entities,
then removes the dynamic producers and parsers together.

The authoritative handoff occurs only after issue
[#4](https://github.com/C360Studio/semdragon/issues/4) adopts beta.158 and
[SemStreams #313](https://github.com/C360Studio/semstreams/issues/313)
provides the public owner-token create/replace mutation path. At that point:

- rules detect a posted party-required quest;
- partycoord policy selects the lead and roster and owns first-class
  membership plus disband intent;
- SemStreams lifecycle Manager owns Party phase, history, contention, and
  restart recovery;
- quest lifecycle owns quest phase and party/agent assignment;
- questdag chooses DAG-unit assignees and owns node output, retry, and rollup;
  and
- SemStreams gated-DAG owns dependency order, claim, redelivery, and stall
  handling.

Issue #28 requires no new upstream issue. It must not copy the rule
processor's internal mutator, hand-build a reusable local ownership framework,
or bind an owner around the retained full-object writer. Those changes belong
to the beta.158/#313 canonical projection gate.

### Issue #26 implementation evidence, pending the final root gate

The reviewed implementation establishes the task 2.16 behavior without
marking that task complete:

- TDD contract tests parse all three shipped profiles and mutation-check the
  exact `rule-processor` type, name, enabled state, ordered four-rule pack, and
  quest watch pattern while rejecting any bossbattle component.
- A fresh-graph integration test drives a review-required quest through the
  real graph-ingest, lifecycle Manager, and rule processor. It reaches
  `completed` with `review-skipped-mechanics-disabled` and produces no battle,
  peer review, XP, reputation, store, inventory, or artifact side effect.
- Stale-checked source inventory deletes the five unproduced event subjects,
  their ports and routing, all retreat-only graph/status/UI/API surfaces, and
  rejects synthetic reintroduction.
- Retention tests preserve the authoritative model, evaluator, component
  factory, battle-state KV port, readers, and typed quest/agent relationships
  without binding an owner or heartbeat.
- The Go unit/build gates, repeated mutation contract, tagged integration
  fixtures, all 342 UI tests, Svelte diagnostics, generated API/schema
  freshness, and diff checks pass under architect, Go, and Svelte review.

Task 2.16 remains unchecked until the root agent completes the final combined
repository gates. Tasks 3.1 through 3.6 remain independently open.

### Approved core-wave verification

The 2026-07-25 live evidence closes only tasks 2.7, 2.8, and 3.7:

- the full Go suite and focused `questdag` race suite pass;
- focused live graph-ingest/gated-DAG integration passes completion read-back,
  every create-boundary interruption, replay, activated-corruption, and
  dependency-order scenarios;
- UI tests and schema freshness pass;
- strict OpenSpec validation passes; and
- the declared-root core predicate audit passes.

The repository-wide predicate and entity-ID audits, relationship coverage,
fresh-volume/replay runbook, and beta.158 handoff remain open.

### Approved issue #12 verification

The 2026-07-25 evidence closes only task 2.9:

- the prompt projection inventory absence guard passes;
- the promptmanager race suite and full Go suite pass;
- the pinned beta.158 promptmanager predicate audit reports zero findings;
- strict OpenSpec validation passes; and
- Go review approved the implementation.

### Approved issue #14 verification

The 2026-07-25 evidence closes only task 2.10:

- vocabulary and payload contract tests cover all 21 canonical predicates,
  exactly three same-subject `@id` relationships, and the approved scalars;
- the vocabulary test uses `vocabulary.SnapshotRegistry` for registry
  isolation;
- the focused root/domain/autonomy race suite and full Go suite pass;
- the pinned beta.158 autonomy predicate audit and exact legacy underscore
  search both report zero findings;
- source review confirms that no projection contract, owner bind, payload
  registration, or graph-ingest route was added;
- strict OpenSpec validation and the diff check pass; and
- architect and Go reviewer approved the implementation after the registry
  isolation fix.

The raw typed-subject events remain separate from graph persistence. Issue #4
still owns the documented wire-or-delete decision for the seven Graphables.

### Approved issue #16 verification

The 2026-07-25 evidence closes only task 2.11:

- vocabulary and payload tests cover all 15 execution lifecycle predicates,
  seven product source-document predicates, and the reused framework title;
- the dead tool-call/result payload, subject, vocabulary, and fixture surfaces
  are absent under a stale-checked inventory test;
- contract tests prove exactly three retained same-subject `@id`
  relationships;
- the temporary `DCTermsModified` compatibility declaration remains isolated,
  linked to SemStreams #673, and scheduled for tagged upstream retirement;
- the focused root/domain/executor/questbridge race suite and full Go suite
  pass;
- the pinned beta.158 executor predicate audit reports zero findings;
- source review confirms that no payload registration, graph-ingest route,
  projection contract, owner bind, or heartbeat was added;
- strict OpenSpec validation and the diff check pass; and
- architect and Go reviewer approved the implementation.

The two real raw-event publishers remain distinct, issue #4 still owns their
graph route or deletion, and the owner-safe web mutation remains deferred.

### Approved issue #22 verification

The 2026-07-25 evidence closes only task 2.14:

- the TDD RED inventory caught all five `Graphable` assertions, fifteen
  graph-only methods, and five relationship fixtures;
- deletion preserves the five raw DTOs, typed subjects, `Validate` methods,
  live publisher calls, and exact JSON wire contracts;
- the AST mutation guard rejects receiver and assertion mutations, proves both
  processor registry functions and all three shipped configurations, and
  permits raw consumers and the authoritative `domain.Guild` projection;
- deterministic Docker integration proves all five typed NATS contracts using
  flushed subscriptions, a NATS RTT, and channel-based delivery;
- the absolute `processor/guildformation` beta.158 audit reports
  `predicate audit passed: 0 structured candidates across 1 roots`;
- the corrected predicate ledger and writer-owner matrix hand multiwriter
  state, dynamic membership/application children, and the pending ticker to
  issue #4 or a dedicated packet without inventing an upstream gap;
- focused race, Docker integration, full Go test, build, strict OpenSpec, and
  diff gates pass; and
- architect and final Go reviewer approved the implementation.

The repository-wide task 3.x gates remain independently open.

### Approved issue #24 verification

The 2026-07-25 evidence closes only task 2.15:

- the TDD RED inventory caught exactly five package files, four `Graphable`
  assertions, twelve graph-only methods, four DTO/subject/publish families,
  four relationship fixtures, seven vocabulary constants plus seven
  registrations, and five component-registry references;
- the entire unactivated processor surface is deleted while startup
  environment selection, the binary invocation, and authoritative Agent birth
  remain;
- AST, shipped-config, historical thirteen-literal, and replacement-route
  mutation guards reject reintroduction while allowing shared framework owner
  and authoritative Agent surfaces;
- deterministic sentinel Docker integration proves zero `seeding.>` events
  and reconstructs the retained Agent from stored `EntityState`;
- the provider-arena proposal now requires SemStreams lifecycle, rules,
  bounded dispatch, first-class entities, graph-ingest, and classified
  projection ownership rather than the deleted processor;
- the pinned beta.158 retained-root audit reports
  `predicate audit passed: 260 structured candidates across 3 roots`;
- focused race, Docker integration, full Go test, build, strict OpenSpec, and
  diff gates pass;
- the predicate ledger, writer-owner matrix, and README are corrected, with
  owner-safe Agent birth handed to issue #4 and no upstream framework gap; and
- architect and final Go reviewer recorded **APPROVE**.

The repository-wide task 3.x gates remain independently open.

## Delivery Order

1. Inventory and approve predicate/reference and writer-owner ledgers.
2. Add failing contract tests and canonical validators.
3. Update producers and Go readers.
4. Delete definition-only graph surfaces whose production writer cannot be
   proved; do not register phantom owners.
5. Update rules, configuration, UI projections, fixtures, and generated files.
6. Reach zero audit findings and green beta.135 tests.
7. Hand off beta.158 adoption to the follow-up change tracked in
   [semdragon#4](https://github.com/C360Studio/semdragon/issues/4), then run its
   fresh-volume/replay gates.

## Risks / Mitigations

- **Silent UI drift**: update generated schema and TypeScript mappings in the
  same predicate commit; assert quest status and DAG projections.
- **Lost relationship semantics**: require an `@id` coverage test over every
  domain `Graphable`.
- **Cross-owner clobbering**: review the writer-owner matrix before beta.158 and
  add overlap rejection tests in the dependent adoption slice.
- **Phantom ownership**: require proof of a production publisher before
  declaring or binding a projection contract; delete definition-only graph
  surfaces instead.
- **Premature DAG dispatch**: atomically birth each unit with complete edges,
  hold every work unit behind the deterministic activation latch, and prove
  interruption-point replay before enabling the new executor.
- **Ambiguous mutation result**: use classified idempotent mutations and
  authoritative read-back; never retry append-style completion writes.
- **Incomplete audit**: use scoped audit roots while SemStreams
  [#667](https://github.com/C360Studio/semstreams/issues/667) is open, then make
  whole-repository audit mandatory when fixed.
- **Destructive operator error**: record literal rendered buckets and NATS
  context; never infer deletion targets from an unset variable or wildcard.

## Sign-Off

The architect approves implementation only in this order. A dependency-only
bump, dual-write compatibility path, or activation of `questdag` before the
canonical corpus and ownership gates is rejected.
