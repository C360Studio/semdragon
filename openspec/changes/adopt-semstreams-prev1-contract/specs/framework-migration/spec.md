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

### Requirement: Stacked pull requests receive least-privilege CI

Issue #11 SHALL run test and lint checks for every pull-request base while
reserving image publication for pushes to `main`. Pull-request code SHALL NOT
execute with package-write authority or through `pull_request_target`.

#### Scenario: Every pull-request base receives checks

- **WHEN** `.github/workflows/ci.yml` is evaluated
- **THEN** `pull_request` has no branch-base filter
- **AND** `push` remains filtered to `main` only
- **AND** the workflow contains no `pull_request_target` trigger
- **AND** test and lint jobs are eligible for a pull request targeting any base

#### Scenario: Publish authority belongs only to the main push job

- **WHEN** workflow and job permissions are inspected
- **THEN** workflow-global permissions contain exactly `contents: read`
- **AND** only `build-and-push` receives `packages: write`
- **AND** `build-and-push` uses exactly
  `github.event_name == 'push' && github.ref == 'refs/heads/main'`
- **AND** no other job can publish a package or image

#### Scenario: Pull-request test and lint jobs remain unprivileged

- **WHEN** test and lint steps are inspected
- **THEN** neither job references a secret
- **AND** neither job performs a Docker login
- **AND** neither job pushes an image or package
- **AND** fork-originated pull requests retain read-only repository access

#### Scenario: Workflow behavior is enforced as code

- **WHEN** issue #11 implements the workflow change
- **THEN** `internal/cicontract/workflow_test.go` first fails against the old
  trigger and permission contract
- **AND** it passes only after the trigger, permission, job, and exact guard
  assertions are satisfied
- **AND** `actionlint`, full race, vet, build, and diff gates pass

#### Scenario: The stacked pull request supplies live evidence

- **GIVEN** draft PR #52 targets non-main base
  `codex/canonical-party-scalars` from base PR #51
- **WHEN** that stacked pull request is published
- **THEN** GitHub reports test and lint checks on the pull request
- **AND** `build-and-push` is skipped
- **AND** no package or image is published
- **AND** task 2.28 completes only after those live results are inspected

#### Scenario: Repository CI needs no upstream framework issue

- **WHEN** the CI workflow contract is corrected
- **THEN** no SemStreams issue is opened
- **AND** architect/security, Go, and technical-writer reviews gate completion

### Requirement: Exact target and evidence

The adoption gate SHALL record the exact SemStreams tag/revision, Go toolchain,
rendered configuration, fresh-state procedure, and verification results.

#### Scenario: Pinned-beta verification is current-head evidence

- **GIVEN** issue #55 remains pinned to SemStreams `v1.0.0-beta.135`
- **WHEN** the pre-adoption verification gate runs
- **THEN** it records the exact reviewed commit and module version
- **AND** full Go race, vet, build, and Revive gates pass
- **AND** UI unit, Svelte diagnostic, production-build, and generated-schema
  freshness gates pass
- **AND** both active OpenSpec changes validate strictly
- **AND** generated verification leaves a clean diff
- **AND** evidence is collected from one current head rather than inferred
  from earlier stacked pull requests

#### Scenario: Seed and replay use one immutable backend image

- **GIVEN** the exact reviewed commit and module pin have passed preflight
- **WHEN** the backend image is prepared for the cutover procedure
- **THEN** it is built exactly once and its immutable `sha256` image ID is
  retained
- **AND** both seed and replay starts disable image build and pull behavior
- **AND** the local backend tag resolves to the retained ID before each start
- **AND** the running backend container resolves to the retained ID after each
  start
- **AND** a missing or mismatched assertion aborts the procedure

#### Scenario: Expected predicate findings block the dependency handoff

- **GIVEN** the entity-ID audit reports `0/25`
- **AND** the relationship contract passes
- **WHEN** the default contract still reports the three SemStreams #671
  non-predicate `quest.failed` findings
- **THEN** tasks 3.2, 3.4, and 3.5 may complete independently
- **AND** task 3.1 remains open
- **AND** no broad predicate-audit exemption is added
- **AND** task 3.6 does not approve the beta.158 handoff

#### Scenario: Latest tag moves during migration

- **WHEN** SemStreams publishes a newer tag before the adoption PR is finalized
- **THEN** the team re-runs the delta audit
- **AND** either updates the target with evidence or records why the newer tag
  is deferred to a new issue and gate

#### Scenario: Shipped Go images satisfy the module toolchain

- **GIVEN** `go.mod` requires Go 1.26.3 or newer
- **WHEN** the backend, mock-LLM, and sandbox images are built
- **THEN** every Go builder uses Go 1.26.4
- **AND** the sandbox runtime exposes `go version go1.26.4`
- **AND** container verification does not rely on an unsupported Go 1.25
  toolchain

#### Scenario: Cold SemEmbed startup retains a real health gate

- **GIVEN** a clean Compose stack with no cached embedding model
- **WHEN** SemEmbed downloads and loads its configured model
- **THEN** Compose keeps the container in its starting period for at least the
  observed cold-load duration
- **AND** readiness still requires the real SemEmbed `/health` endpoint
- **AND** the backend does not replace that probe with process-only liveness

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
