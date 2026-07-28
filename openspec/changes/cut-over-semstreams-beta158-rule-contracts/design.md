# Design: cut over SemStreams beta.158 rule contracts

## Context

Semdragon is green on SemStreams `v1.0.0-beta.135`. The reviewed pre-v1
preparation change canonicalized scanner-visible graph contracts, recorded
writer debt, and specified a deletion-safe fresh-state procedure. Issue #59
now adopts the first exact revision containing SemStreams PR #708:

```text
module version:
v1.0.0-beta.158.0.20260728003411-89617f32dc36

merge commit:
89617f32dc3657409c2294d75ffcdc74decf014e
```

PR #708 closes SemStreams issue #706. It derives a rule pack's effective
projection contracts from the frozen initial `replace_owned` action snapshot.
Omission, an explicit empty array, and `null` have intentionally different
meaning. The change preserves the public mutation client, ownership
enforcement, wire envelopes, and persisted graph state.

The current quest rules contain only `lifecycle_transition` actions. They use
the lifecycle Manager and graph-ingest path; they are not
`replace_owned` rule-pack projections.

## Goals / Non-Goals

**Goals:**

- adopt the exact reviewed SemStreams revision without a floating tag;
- migrate every shipped quest rule pack to beta.158 configuration;
- preserve least authority under PR #708 omission semantics;
- make retry routing honor each quest's configured maximum;
- delete local compatibility only when this revision supplies the primitive;
- prove boot, mutation, lifecycle, fresh-state, and replay behavior.

**Non-Goals:**

- closing the full issue-#4 ownership migration;
- introducing a local projection or mutation framework;
- reactivating dormant mechanics;
- migrating Guild, Party, Agent, BossBattle, dmsession, web-document,
  autonomy, or executor authoritative writers;
- resolving structured dynamic predicate families in this packet.

## Decisions

### D1: Add a successor change

This change is independent of `adopt-semstreams-prev1-contract`. The
predecessor remains an immutable beta.135 preparation and evidence record.
Issue #59 owns the first dependency and runtime cutover. It does not change
predecessor task completion truth.

### D2: Pin the post-tag commit exactly

`go.mod` uses the Go pseudo-version resolving to merge `89617f32`. A plain
`beta.158`, branch, or latest-version selector is rejected because PR #708
merged after the latest beta.158 tag.

The already-aligned Go 1.26.4 backend, mock-LLM, and sandbox builders remain
the supported toolchain. The implementation records:

- `go list -m github.com/c360studio/semstreams`;
- the resolved module sum;
- `go version` locally and in shipped Go images; and
- a source assertion that the resolved module contains PR #708 behavior.

### D3: One stable quest rule-pack identity

Every shipped profile configures the enabled rule processor with:

```json
{
  "pack_id": "quest-lifecycle",
  "enable_graph_integration": true,
  "entity_watch_buckets": {
    "ENTITY_STATES": ["*.*.game.*.quest.*"]
  }
}
```

The legacy `entity_watch_patterns` field is removed. `quest-lifecycle` is
stable across replicas and profiles because those instances are one logical
pack. Any independently composed rule pack must use a different stable
`pack_id`; it must not reuse this owner/event identity merely because its
rule IDs are locally unique.

Enabled rule-processor admission is fail closed in the target revision. A
missing or invalid pack identity, invalid initial rule, derivation failure, or
override failure aborts component-manager service construction. Disabled rule
packs do not participate. Ordinary component creation retains its established
best-effort isolation.

### D4: Omission is the lifecycle pack's least-authority contract

The three shipped configs omit `projection_contracts`. They do not author
`[]`, `null`, or a hand-copied contract block.

PR #708 derives contracts only from the frozen initial `replace_owned` action
inventory. The current quest rule files contain `lifecycle_transition`
actions, so their effective rule-pack projection set is empty. Lifecycle
authority continues through the registered quest workflow and lifecycle
Manager; the rule pack does not acquire a duplicate projection owner.

This is a deliberate distinction:

- omitted field: derive minimal authority from statically provable
  `replace_owned` actions;
- explicit `[]`: declare an empty override, which fails if actions create an
  obligation;
- explicit `null`: invalid authoring;
- explicit non-empty array: immutable authorization superset.

Derived contracts are boot-time state. They are not written into authored
configuration, config KV, or rule files.

### D5: Replacement groups define atomic deletion boundaries

Future `replace_owned` rules retain explicit `projection_contract`,
`projection_group`, and literal `predicate` selectors. A group is the complete
atomic replacement set: predicates absent from a replacement may be removed.
Independently replaceable sets therefore use distinct, stable group names.
Group reuse for unrelated atomic sets is rejected even when all predicates
belong to the same owner.

Omission is allowed only when every target is statically provable from an
entity rule pattern, exact `$entity.id`, or canonical literal entity ID, and
all derived patterns for a contract are identical.

An explicit contract envelope is required when:

- a target is dynamic or templated;
- one contract needs a deliberate wider entity pattern;
- multiple statically derived target patterns require a covering pattern;
- hot reload may add a reserved predicate, group, or target scope; or
- birth predicates, foreign edges, indexing profile, or message type are
  needed.

The envelope must contain every derived contract, group, mode, predicate, and
target pattern. It may be a valid superset, but every extra remains subject to
normal overlap and ownership validation. Hot reload stays inside the frozen
effective envelope and never rebinds.

### D6: Retry routing uses the entity's maximum

Both mutually exclusive failure rules compare
`quest.attempts.current` with the same field-to-field value reference:

```json
"value": "$entity.triple.quest.attempts.max.value"
```

The retry rule uses `lt`; the triage rule uses `gte`. No product constant or
default literal is duplicated in the rule files. Quest creation remains
responsible for emitting a valid numeric `quest.attempts.max`. A missing or
invalid maximum fails closed: neither rule may guess a fallback or route the
quest with a hard-coded value.

Tests cover maxima below, equal to, and above three so a test that merely
reproduces the historical default cannot pass accidentally.

### D7: Framework constants replace copied values

Every relationship triple uses
`message.EntityReferenceDatatype`; Semdragon no longer copies the literal
entity-reference datatype into compatibility code.

Questdag begins with `gateddagexec.DefaultConfig()` and overlays only product
configuration such as name, unit pattern, dispatch subject, timing, and
parallelism. It does not override claim, completion, failure, dirtied, or
dependency marker predicates. Seeding, completion, tests, and the executor
consume the same resolved default predicate set.

Product provenance predicates and the private completion writer remain
Semdragon composition. The marker vocabulary itself is framework-owned.

### D8: Prefix helpers are used at their actual boundaries

`types.ValidateEntityIDPrefix` validates configured and requested one-to-six
segment query prefixes. `graph.MatchesAnyIDPrefix` is used only when the value
being matched is a concrete canonical six-part entity ID.

The target revision does not resolve SemStreams #675. Partial-prefix to
partial-prefix routing therefore retains the isolated equality-or-dot-
descendant comparator linked to that issue. The implementation must not pass a
partial prefix to the full-ID matcher or broaden the local comparator into a
generic graph library.

### D9: Retain only still-blocked seams

This packet deliberately retains:

- `dc.terms.modified` compatibility, pending SemStreams #673;
- prefix-to-prefix comparison, pending #675;
- activation-latch admission, pending #670;
- private completion mutation and authoritative read-back fallback, pending
  #313;
- occurrence-exact audit classification blockers, pending #671; and
- dynamic Party, Agent, and BossBattle structured families, pending #683.

The dependency bump does not imply that these issues are solved. No wildcard
owner claim, permissive validator, dual representation, or local substitute
is allowed.

## Migration and Rollback

1. Add RED contracts for config shape, rule behavior, framework defaults, and
   prefix boundaries.
2. Pin the exact module revision and make only required compile adaptations.
3. Migrate all three shipped configurations and the failure rules.
4. Remove only upstream-supplied compatibility constants and overrides.
5. Pass focused, full, race, audit, schema, and strict OpenSpec gates.
6. Execute the reviewed development/test fresh-state and replay runbook using
   one immutable backend image built from the accepted commit.
7. Record normalized entity, relationship, quest-status, and DAG parity.

Rollback is a source and image rollback followed by the same explicit
development/test graph reset and canonical reseed. No in-place beta-state
rewriter, alias, or dual writer is introduced.

## Risks / Mitigations

- **Accidental rule-pack ownership:** omission plus an effective-empty
  assertion proves lifecycle-only rules bind no rule projection.
- **Hot-reload authority surprise:** mutation tests distinguish minimal
  derivation from an explicit reserved superset.
- **Atomic replacement data loss:** distinct group tests prove independently
  replaceable predicates never share a group.
- **Retry regression hidden by default three:** test per-quest maxima on both
  sides of the historical default.
- **Framework vocabulary drift:** resolve marker predicates from
  `DefaultConfig()` and prohibit copied literals.
- **Prefix overmatch:** test exact, descendant, false-sibling, full-ID, and
  partial-prefix paths independently.
- **Broad cutover claim:** exclusions and upstream blockers remain normative
  acceptance conditions.

## Architecture Sign-Off

Implementation is ready only when the exact pseudo-version is resolved, all
three profiles satisfy one rule-pack contract, lifecycle behavior remains
green for non-default retry maxima, no copied supplied primitive remains, the
fresh-state/replay gate passes, and the diff contains no deferred ownership
migration.
