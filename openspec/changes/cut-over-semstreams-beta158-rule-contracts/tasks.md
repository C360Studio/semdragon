# Tasks: cut over SemStreams beta.158 rule contracts

## 1. RED contract gates

- [x] 1.1 Link implementation and evidence to [Semdragon issue #59] and parent
  [issue #4]. Record the accepted Semdragon base commit before implementation.
- [x] 1.2 Add a failing three-profile configuration contract requiring exact
  pack identity `quest-lifecycle`, explicit graph integration,
  `ENTITY_STATES` quest watch configuration, omission of
  `projection_contracts`, and absence of `entity_watch_patterns`.
- [x] 1.3 Mutation-test the configuration contract against a missing or
  changed pack ID, duplicate independent pack identity, disabled graph
  integration, legacy watch shape, malformed/non-six-position watch pattern,
  explicit empty contracts, and a hand-authored lifecycle contract.
- [x] 1.4 Add failing rule behavior tests for claim, start, reviewed and
  unreviewed submission, mechanics-stub completion, and retry/triage with
  per-quest maxima below, equal to, and above three.
- [x] 1.5 Add a failing guard rejecting the historical retry literal and
  requiring both failure conditions to reference
  `$entity.triple.quest.attempts.max.value`.
- [x] 1.6 Add focused rule-pack integration fixtures for omission-derived
  minimum authority, explicit-empty and null rejection, dynamic-target
  rejection, explicit-envelope acceptance, distinct atomic replacement
  groups, hot-reload reserve acceptance, out-of-envelope rejection, and
  boot-fatal enabled-pack admission. Exercise SemStreams public behavior; do
  not reproduce its derivation implementation.
- [x] 1.7 Add failing source and behavior guards requiring
  `message.EntityReferenceDatatype`, gated-DAG defaults, supported prefix
  helpers, and absence of copied supplied constants.

## 2. Exact dependency and compile cutover

- [x] 2.1 Pin `github.com/c360studio/semstreams` exactly to
  `v1.0.0-beta.158.0.20260728003411-89617f32dc36` and refresh `go.sum`.
- [x] 2.2 Record `go list -m` evidence proving merge
  `89617f32dc3657409c2294d75ffcdc74decf014e` and PR #708 behavior. Reject a
  plain beta.158 or floating replacement.
- [x] 2.3 Apply only compile adaptations required by the exact revision. Do
  not use the dependency bump to migrate an excluded product owner.
- [x] 2.4 Prove local and shipped backend, mock-LLM, and sandbox toolchains
  remain Go 1.26.4.

## 3. Rule configuration and behavior GREEN

- [x] 3.1 Migrate `config/semdragons.json`,
  `config/semdragons-semsource.json`, and
  `config/semdragons-e2e-tier1.json` to the exact rule-pack configuration in
  the specification.
- [x] 3.2 Omit `projection_contracts` in all three profiles. Prove the frozen
  lifecycle-only rule snapshot produces no effective rule-pack projection
  claim.
- [x] 3.3 Replace both failure-rule literal maxima with
  `$entity.triple.quest.attempts.max.value`, preserving the `lt`/`gte` value-space
  partition and atomic command/assignment cleanup.
- [x] 3.4 Pass the focused claim/start, submission, retry/triage, and
  mechanics-stub tests against real graph-ingest, lifecycle Manager, and rule
  processor composition.
- [x] 3.5 Pass the omission, explicit override, dynamic envelope, atomic group,
  hot-reload, and enabled-pack admission fixtures without adding downstream
  derivation or mutation code.

## 4. Framework primitive GREEN

- [x] 4.1 Replace copied entity-reference values with
  `message.EntityReferenceDatatype` and keep the repository relationship
  inventory exact.
- [x] 4.2 Construct questdag executor configuration from
  `gateddagexec.DefaultConfig()`. Overlay only product settings and remove all
  downstream overrides/copies of canonical claim, completion, failure,
  dirtied, and dependency markers.
- [x] 4.3 Prove unit seeding, activation, completion, dispatch, and
  interruption replay use the same framework-resolved marker predicates.
- [x] 4.4 Validate configuration and query prefixes with
  `types.ValidateEntityIDPrefix`. Use `graph.MatchesAnyIDPrefix` only for
  concrete six-part IDs.
- [x] 4.5 Preserve the isolated prefix-to-prefix comparator and its SemStreams
  #675 link. Mutation-test exact, descendant, false-sibling, unready-owner,
  unowned-fallback, full-ID, and partial-prefix paths.

## 5. Exclusion and blocker audit

- [x] 5.1 Prove `dc.terms.modified` compatibility remains linked to #673, the
  activation latch remains linked to #670, the prefix comparator remains
  linked to #675, and the private completion writer/read-back fallback remains
  linked to #313.
- [x] 5.2 Prove the occurrence-exact audit blocker remains linked to
  [SemStreams #671] and
  structured dynamic Party, Agent, and BossBattle families remain linked to
  #683 without a new exemption or representation.
- [x] 5.3 Diff-audit Guild, Party, Agent, BossBattle, dmsession, web-document,
  autonomy, executor, and other mechanics code/configuration. Reject
  reactivation, ownership migration, wildcard claims, dual representations,
  permissive validation, state rewriting, or a generic local mutator.

## 6. Repository and operational acceptance

- [x] 6.1 Pass focused tests and `go test -race ./...`.
- [x] 6.2 Pass `go vet ./...`, `go build ./...`, and the repository's Revive
  gate.
- [x] 6.3 Pass predicate, entity-ID, relationship, rule/configuration, and
  generated schema/OpenAPI/TypeScript contracts. Record any still-blocked
  #671 aggregate result exactly; do not claim zero if it remains nonzero.
- [x] 6.4 Pass UI unit tests, Svelte diagnostics, and the production UI build
  if generated or consumer contracts change.
- [x] 6.5 Run strict validation for
  `cut-over-semstreams-beta158-rule-contracts`,
  `adopt-semstreams-prev1-contract`, and
  `rebuild-quest-lifecycle-on-semstreams-rules`.
- [x] 6.6 Pass `git diff --check` and prove generated artifacts leave no
  unexplained diff.
- [ ] 6.7 Execute the reviewed development/test fresh-state and replay
  procedure using one immutable backend image. Capture authoritative
  readiness and normalized entity, relationship, quest-status, and DAG parity
  while leaving SemSource services and volumes untouched.
- [x] 6.8 Obtain architect, Go, Svelte when applicable, and technical-writer
  approval. Record live CI test/lint success and skipped pull-request
  publication before integration.
- [ ] 6.9 Report issue #59 complete only for this bounded cutover. Keep issue
  #4 and every recorded upstream blocker open.

## Execution evidence

- Tasks 1.1 through 6.6 passed on candidate
  `a4912d725ccb722e38b87feecb31b7c56e328ffc`. Task 6.4 is not applicable:
  this change does not modify generated or UI consumer contracts.
- Task 6.3 retained the exact [SemStreams #671] aggregate: core `0/329`,
  processor questboard `2/72` on `quest.failed`, service `1/1` on
  `quest.failed`, UI `0/41`, entity IDs `0/25`, and relationships green.
- Task 6.7 was executed with one immutable image and remains unchecked because
  exact replay parity failed. Both snapshots reached the temporary
  three-consecutive-checksum quiescence boundary tracked by [SemStreams #712],
  but [SemStreams #713] appended 15 duplicate hierarchy triples across six
  entities and advanced their revisions on the seed-disabled restart.
- Task 6.8 passed on stacked draft [PR #60]: architect, Go, and
  technical-writer approvals are recorded; Svelte review is not applicable;
  live GitHub lint and test passed; and the pull-request `build-and-push` job
  was skipped as required. Task 6.9 remains unchecked; [issue #59], parent
  [issue #4], and all upstream blockers stay open.

[Semdragon issue #59]: https://github.com/C360Studio/semdragon/issues/59
[issue #59]: https://github.com/C360Studio/semdragon/issues/59
[issue #4]: https://github.com/C360Studio/semdragon/issues/4
[SemStreams #671]: https://github.com/C360Studio/semstreams/issues/671
[SemStreams #712]: https://github.com/C360Studio/semstreams/issues/712
[SemStreams #713]: https://github.com/C360Studio/semstreams/issues/713
[PR #60]: https://github.com/C360Studio/semdragon/pull/60
