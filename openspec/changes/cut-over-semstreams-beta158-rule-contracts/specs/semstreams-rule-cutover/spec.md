# SemStreams rule cutover Specification

## ADDED Requirements

### Requirement: The cutover uses one exact SemStreams revision

Semdragon SHALL depend on
`v1.0.0-beta.158.0.20260728003411-89617f32dc36`, resolving to merge
`89617f32dc3657409c2294d75ffcdc74decf014e`. It SHALL NOT substitute the
earlier beta.158 tag, a branch, or a floating latest version.

#### Scenario: The module revision is verified

- **WHEN** the implementation gate runs `go list -m`
- **THEN** the SemStreams version equals the required pseudo-version
- **AND** the resolved source includes PR #708 rule-contract derivation
- **AND** backend, mock-LLM, and sandbox Go images remain on Go 1.26.4

### Requirement: Every shipped quest rule pack has one explicit identity

The default, SemSource, and tier-1 E2E profiles SHALL configure their enabled
quest rule processor with stable `pack_id` `quest-lifecycle`, explicit graph
integration, and an `ENTITY_STATES` watch containing only the canonical
six-position quest pattern `*.*.game.*.quest.*`.

#### Scenario: All profiles use the supported watch shape

- **WHEN** the three shipped configurations are parsed
- **THEN** each enabled quest rule processor has
  `pack_id: "quest-lifecycle"`
- **AND** each explicitly enables graph integration
- **AND** each has
  `entity_watch_buckets.ENTITY_STATES: ["*.*.game.*.quest.*"]`
- **AND** none contains `entity_watch_patterns`

#### Scenario: An independent pack cannot reuse the identity

- **WHEN** another independently composed rule pack is configured
- **THEN** it uses a distinct stable `pack_id`
- **AND** processor-local rule IDs do not justify sharing
  `quest-lifecycle`

### Requirement: Lifecycle-only rules omit projection contracts

The shipped quest rule processors SHALL omit `projection_contracts`. Their
current rules contain lifecycle actions and no `replace_owned` action, so the
effective rule-pack projection contract set SHALL be empty.

#### Scenario: Omission derives no duplicate lifecycle owner

- **GIVEN** the initial quest rules contain only
  `lifecycle_transition` mutation actions
- **WHEN** rule-pack preflight derives effective contracts
- **THEN** it produces no rule-pack projection claim
- **AND** lifecycle authority remains with the registered quest workflow and
  lifecycle Manager
- **AND** authored configuration remains without `projection_contracts`

#### Scenario: Empty and null are not omission

- **WHEN** a test pack with a `replace_owned` obligation supplies
  `projection_contracts: []`
- **THEN** preflight fails because the empty override covers no obligation
- **WHEN** a pack supplies `projection_contracts: null`
- **THEN** configuration decoding fails

### Requirement: Derived replacement authority remains least and atomic

Every `replace_owned` action SHALL retain explicit contract, group, and
predicate selectors. Each group SHALL name one complete atomic replacement
set. Independently replaceable predicate sets SHALL have distinct group names.

Omission SHALL derive authority only from statically provable targets.
Dynamic targets, deliberate wider scopes, explicit-only metadata, and
hot-reload reserves SHALL require an explicit immutable superset envelope.

#### Scenario: Static action derives minimal authority

- **GIVEN** an entity-scoped `replace_owned` action omits its subject or uses
  exact `$entity.id`
- **AND** no explicit contracts are authored
- **WHEN** preflight runs
- **THEN** it derives only the action's exact contract, group, predicate, mode,
  and entity pattern
- **AND** it does not write that contract into authored configuration

#### Scenario: Dynamic target requires an envelope

- **WHEN** a `replace_owned` target uses a dynamic value other than exact
  `$entity.id`
- **AND** no explicit envelope covers it
- **THEN** boot fails before ownership, heartbeat, injection, or mutation
- **AND** the target is not widened to a match-all pattern

#### Scenario: Explicit reserve supports bounded hot reload

- **GIVEN** an explicit contract contains all initially derived authority
- **AND** it reserves an additional valid predicate or wider pattern
- **WHEN** hot reload adds an action inside that frozen envelope
- **THEN** target validation may accept the update without rebinding
- **AND** an action outside the envelope fails atomically

#### Scenario: Distinct replacement sets cannot share one group

- **GIVEN** two predicate sets are replaced independently
- **WHEN** their action contracts are authored
- **THEN** they use distinct non-empty projection group names
- **AND** replacing either set cannot remove predicates from the other

### Requirement: Invalid enabled rule packs fail boot

Every configured, enabled `rule-processor` SHALL be admitted successfully or
component-manager service construction SHALL fail before any rule pack binds
or starts. Disabled invalid packs SHALL remain excluded, and ordinary
component failures SHALL retain their established isolation behavior.

#### Scenario: One invalid enabled sibling blocks composition

- **GIVEN** one enabled rule pack is valid
- **AND** another enabled rule pack has an invalid identity, rule, derivation,
  or override
- **WHEN** component-manager service construction runs
- **THEN** construction fails with the configured instance and wrapped cause
- **AND** neither pack binds or starts

### Requirement: Retry routing uses each quest's maximum

The retry and triage rules SHALL compare `quest.attempts.current` against
`$entity.triple.quest.attempts.max.value`. The retry rule SHALL use `lt`; the triage
rule SHALL use `gte`. Neither rule SHALL contain the historical literal `3`.

#### Scenario: Non-default retry budget routes correctly

- **GIVEN** quests with valid maxima below, equal to, and above three
- **WHEN** each quest fails in progress
- **THEN** attempts below that quest's maximum return to `posted`
- **AND** attempts at or above that quest's maximum enter `pending_triage`
- **AND** command and assignment cleanup remain atomic with the transition

#### Scenario: Missing maximum fails closed

- **WHEN** a failing quest has no valid `quest.attempts.max`
- **THEN** neither failure rule guesses a default
- **AND** no hard-coded fallback transition occurs

### Requirement: Entity relationships use the framework datatype

Every Semdragon graph edge SHALL use
`message.EntityReferenceDatatype`. No downstream compatibility constant SHALL
copy the framework's entity-reference literal.

#### Scenario: Relationship inventory remains exact

- **WHEN** the repository relationship contract runs
- **THEN** every approved relationship object uses the framework constant
- **AND** scalar objects do not acquire the entity-reference datatype
- **AND** the reviewed relationship shape inventory remains stale-checked

### Requirement: Gated-DAG marker vocabulary comes from framework defaults

Questdag SHALL initialize the executor from
`gateddagexec.DefaultConfig()` and SHALL NOT override the framework claim,
completed, failed, dirtied, or dependency marker predicates.

#### Scenario: Product configuration overlays framework defaults

- **WHEN** questdag constructs its executor configuration
- **THEN** product name, unit pattern, dispatch, timing, and parallelism may
  overlay the default
- **AND** all five marker predicates remain equal to the framework defaults
- **AND** seeding, execution, completion, and tests consume that same resolved
  predicate set
- **AND** no copied gated-DAG marker constant remains in compatibility code

### Requirement: Prefix helpers remain boundary-correct

Configured and requested one-to-six segment prefixes SHALL be validated with
`types.ValidateEntityIDPrefix`. Concrete six-part entity IDs SHALL be matched
with `graph.MatchesAnyIDPrefix`. Partial-prefix containment SHALL retain only
the isolated comparison linked to SemStreams #675.

#### Scenario: Concrete entity routing uses the framework matcher

- **GIVEN** a canonical six-part entity ID
- **WHEN** graph-source routing checks its configured owner prefixes
- **THEN** `graph.MatchesAnyIDPrefix` selects exact and dot-descendant owners
- **AND** lexical false siblings do not match

#### Scenario: Partial prefixes do not misuse the full-ID matcher

- **WHEN** one partial query prefix is compared with another partial prefix
- **THEN** it is not passed to `graph.MatchesAnyIDPrefix`
- **AND** the isolated equality-or-dot-descendant comparison remains linked to
  SemStreams #675

### Requirement: The bounded cutover preserves unresolved ownership seams

Issue #59 SHALL retain the linked downstream seams for SemStreams #313, #670,
#671, #673, #675, and #683. It SHALL NOT migrate or reactivate deferred product
owners.

#### Scenario: A green cutover does not close issue 4

- **WHEN** issue #59 passes its acceptance gates
- **THEN** Guild, Party, Agent, BossBattle, dmsession, web-document, autonomy,
  executor, and other mechanics ownership remain out of scope
- **AND** no wildcard owner claim, dual representation, permissive validator,
  in-place state rewrite, or copied generic mutation framework is added
- **AND** issue #4 remains open for the remaining ownership migration

### Requirement: Fresh state and replay gate the revision

The accepted implementation SHALL execute the reviewed development/test
fresh-state procedure using one immutable backend image built at the accepted
commit. Restart without seeding SHALL reproduce normalized entity,
relationship, quest-status, and DAG evidence.

#### Scenario: Exact image survives restart

- **WHEN** canonical state is seeded once and the backend restarts with all
  seed flags disabled
- **THEN** both starts use the same retained image ID
- **AND** readiness is polled authoritatively
- **AND** preserved graph and lifecycle semantics have exact parity
- **AND** SemSource services and volumes remain untouched
