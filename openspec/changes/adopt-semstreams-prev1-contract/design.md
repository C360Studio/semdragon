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
