# framework-migration Specification

## Purpose

Define the sequencing and evidence required to adopt a breaking SemStreams
pre-v1 release while keeping framework and product responsibilities separate.

## ADDED Requirements

### Requirement: Contract-first dependency adoption

The system SHALL make the product graph corpus valid for the target SemStreams
contract before changing the module dependency.

#### Scenario: Dependency-only bump is rejected

- **WHEN** predicate, entity-ID, relationship, or ownership audits have findings
- **THEN** the beta.158 dependency task remains blocked

### Requirement: Framework primitives replace framework duplication

Semdragons SHALL use SemStreams for graph mutation/validation, lifecycle
transitions/history, rule evaluation, gated-DAG scheduling/recovery, and generic
agentic tool execution. Semdragons SHALL retain only product semantics and thin
composition adapters in those areas.

#### Scenario: Missing framework primitive

- **WHEN** a required framework-shaped capability is missing or defective
- **THEN** the team files or reuses a verified SemStreams issue
- **AND** any temporary downstream adapter is minimal, isolated, and linked to
  the upstream issue

#### Scenario: Partycoord deletion needs no replacement framework

- **GIVEN** party raw events have no consumer or durable graph route
- **AND** partycoord decomposition, result, and rollup state duplicates the
  questdag and quest lifecycle boundary
- **WHEN** issue #28 deletes those surfaces
- **THEN** it opens no new upstream framework issue
- **AND** it does not recreate them through rule `publish`, a local event bus,
  a projection owner, or a private lifecycle/DAG implementation
- **AND** the later authoritative Party projection remains gated on beta.158
  adoption and SemStreams #313

### Requirement: Projection ownership follows a real publisher

The system SHALL declare and bind a SemStreams `projection.Contract` only when
a production graph publisher for that projection is identified. A library,
payload definition, vocabulary entry, or test fixture alone SHALL NOT be
registered as a projection owner.

#### Scenario: Definition-only prompt projection

- **GIVEN** the prompt-assembly `Graphable` has no production publisher
- **AND** it has no payload-registry registration or lifecycle component
- **WHEN** issue #12 removes its beta.158 predicate findings
- **THEN** the dead `Graphable`, typed subject, vocabulary entry, and
  relationship fixture are deleted
- **AND** no promptmanager component, projection contract, owner bind, or
  heartbeat is added
- **AND** a stale-checked source inventory guard rejects reintroduction of that
  exact prompt projection surface and external registration hooks
- **AND** the guard does not prohibit framework projection contracts,
  ownership services, heartbeats, or lifecycle timers for real publishers

#### Scenario: A prompt publisher is introduced later

- **WHEN** a production component begins publishing prompt-assembly graph
  evidence
- **THEN** that component declares and binds the projection through SemStreams
- **AND** all six prompt-assembly predicates use `append-evidence`
- **AND** `prompt.assembly.agent-id` remains a typed same-subject `@id`
  relationship with no `ForeignEdgeClaim`

### Requirement: PR-gated SDD delivery

Each migration slice SHALL have an OpenSpec task boundary, a linked GitHub
issue, tests appropriate to its risk, and required role reviews before
integration.

#### Scenario: Gate is not ready

- **WHEN** a required architect, Go reviewer, Svelte reviewer, or
  technical-writer sign-off requests changes
- **THEN** the pull request remains draft or blocked

### Requirement: Exact target and evidence

The adoption gate SHALL record the exact SemStreams tag/revision, Go toolchain,
rendered configuration, fresh-state procedure, and verification results.

#### Scenario: Latest tag moves during migration

- **WHEN** SemStreams publishes a newer tag before the adoption PR is finalized
- **THEN** the team re-runs the delta audit
- **AND** either updates the target with evidence or records why the newer tag
  is deferred to a new issue and gate

### Requirement: Gated-DAG plans are admitted before dispatch

While the repository remains pinned to beta.135, issue #3 SHALL stage each
dynamically materialized gated-DAG plan behind one deterministic activation
latch. It SHALL NOT expose a real work unit to the executor until that unit is
born atomically with its complete dependency set and the complete plan has been
authoritatively reconciled.

#### Scenario: Partial seed cannot dispatch

- **GIVEN** the gated-DAG executor is watching the unit prefix
- **WHEN** seeding fails after any activation or work-unit create
- **THEN** zero work units are dispatchable
- **AND** every created work unit remains held by the incomplete activation
  latch

#### Scenario: Unit birth is complete and stub-free

- **WHEN** a work unit is created
- **THEN** the same single-entity atomic create contains its immutable
  provenance, every declared dependency, and its activation-latch dependency
- **AND** every relationship target already exists
- **AND** neither the work unit nor any prerequisite is represented by
  `graph.StubMessageType`

### Requirement: DAG admission converges on replay

Execution ID SHALL be the plan idempotency key. Unit and activation IDs SHALL
be deterministic, and a canonical manifest digest SHALL bind one execution ID
to one immutable plan.

#### Scenario: Replay after interruption

- **WHEN** the same plan is replayed after any create or activation interruption
- **THEN** it reuses the same entity IDs
- **AND** matching immutable seed contracts are accepted
- **AND** no duplicate unit is created
- **AND** the plan converges to one activation commit

#### Scenario: Idempotency-key collision

- **WHEN** a different manifest or immutable unit contract is presented under
  an existing execution ID
- **THEN** admission fails with a classified collision
- **AND** no existing unit is overwritten

#### Scenario: Activated plan is incomplete

- **GIVEN** the activation completion marker is already present
- **WHEN** authoritative reconciliation finds a missing or mismatched unit
- **THEN** admission reports graph corruption
- **AND** it does not silently add, replace, or reactivate units

### Requirement: Activation uses the consumer-owned completion seam

Semdragon SHALL release the activation latch only through a private
`questdag-completion` writer that exclusively owns
`gateddag.unit.completed`. The writer SHALL use an owner-token-stamped,
classified, replace-by-predicate mutation and SHALL NOT use append-style
`triple.add`. While pinned to beta.135, the writer SHALL confirm every
completion attempt by authoritative entity read-back and SHALL NOT treat the
unvalidated decoded mutation echo as authoritative.

#### Scenario: Beta.135 mutation reports success

- **WHEN** the beta.135 mutation returns a non-degraded success echo
- **THEN** the writer performs an authoritative entity read
- **AND** it reports success only when the read proves the completion marker

#### Scenario: Activation reply is lost

- **WHEN** the completion mutation commits but its reply is lost or degraded
- **THEN** an authoritative read proving the completion marker returns success
- **AND** replay does not append a duplicate marker

#### Scenario: Activation becomes visible

- **GIVEN** every expected unit and dependency has been authoritatively verified
- **WHEN** the activation completion marker commits
- **THEN** only dependency roots become dispatchable
- **AND** non-root work remains held until its declared prerequisites complete

#### Scenario: Beta.158 validator is adopted

- **WHEN** issue #4 adopts beta.158 and validates a non-degraded decoded echo
- **THEN** that validated echo may satisfy completion confirmation
- **AND** lost or degraded replies still require authoritative read-back

### Requirement: Temporary admission bridge is retired upstream

The activation-latch protocol SHALL remain a narrow issue #3 product adapter.
It SHALL NOT become a reusable local gated-DAG framework.

#### Scenario: Framework staging becomes available

- **WHEN** SemStreams #670 supplies staged fan-out admission and the dependency
  adoption slice consumes it
- **THEN** Semdragon deletes the activation-latch bridge
- **AND** retains only product provenance and dispatch policy
