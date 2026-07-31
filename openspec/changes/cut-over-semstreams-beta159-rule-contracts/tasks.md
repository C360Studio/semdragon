# Tasks: cut over to SemStreams beta.159

## 1. Reopen and specify the migration

- [x] 1.1 Confirm `v1.0.0-beta.159` resolves to commit `8813270c` and contains
  the reviewed fixes for SemStreams #712 and #713.
- [x] 1.2 Compile Semdragon unchanged against beta.159 and record that the only
  full-test failure is the obsolete beta.158 pin contract.
- [x] 1.3 Inventory the breaking stream, embedding, readiness, GraphQL, bucket,
  and append-dedup contracts.
- [x] 1.4 Identify direct `ENTITY_STATES` creation and writes as a release
  blocker, not deferred debt.
- [x] 1.5 Rename the active OpenSpec and migration paths to beta.159 and record
  the approved architecture contract.

## 2. RED contract gates

- [x] 2.1 Require the exact beta.159 module and record the peeled source tag
  commit.
- [x] 2.2 Validate all shipped configs through SemStreams `Config.Validate`,
  including the exact `GRAPH` and `AGENT` stream bounds.
- [x] 2.3 Construct graph-embedding components and reject `cache_ttl`, output
  ports, and `EMBEDDINGS_CACHE`.
- [x] 2.4 Require exact graph-gateway readiness keys/path and strict client-side
  fold behavior for missing, unknown, stale, hard-stop, incomplete, and lagging
  producers.
- [x] 2.5 Require the flat `graphSummary { total_entities }` response separately
  from caught-up readiness.
- [x] 2.6 Reject production direct creation, raw opening, raw handles, `Put`, or
  `Update` access to `ENTITY_STATES` and every deleted GraphClient write API.
- [x] 2.7 Require every unbound product mutation surface to return typed
  not-committed `MutationUnavailable` without raw KV or graph request/reply I/O.
- [x] 2.8 Require strict owner leases, ownership-first lifecycle/rule
  composition, and fail-closed handling of every unbound product writer.
- [x] 2.9 Add a seven-definition regression contract requiring canonical
  `entity.pattern` authoring independently of watcher health.
- [x] 2.10 Identify the production boot failure caused by calling
  `service.WireOwnership` with zero static projection contracts and track the
  missing framework-only convenience composition in [SemStreams #812].
- [x] 2.11 Add a regression that executes the production ownership composition
  and rejects `mutation client has no contracts` before rechecking GREEN.

## 3. Dependency and breaking configuration GREEN

- [x] 3.1 Pin `github.com/c360studio/semstreams v1.0.0-beta.159`, refresh sums,
  and update dependency contract tooling.
- [x] 3.2 Add 256 MiB `discard: old` bounds while retaining 24-hour `GRAPH` and
  72-hour `AGENT` ages in every shipped profile.
- [x] 3.3 Remove graph-embedding cache fields and ports from the two applicable
  profiles.
- [x] 3.4 Configure readiness for exactly `graph-ingest`, `graph-index`, and
  `rule` in every graph-gateway profile.
- [x] 3.5 Set `enforce_owner_lease: true` on every graph-ingest profile.
- [x] 3.6 Add `*.*.game.*.quest.*` to all seven shipped quest rule definitions.

## 4. Framework graph-access GREEN

- [x] 4.1 Delete GraphClient bucket creation and all five legacy write methods.
- [x] 4.2 Retain only a catalog-backed reader facade with no raw KV escape.
- [x] 4.3 Prove the reader preserves `index_not_ready` and retry behavior when
  graph-ingest has not provisioned the catalog bucket.
- [x] 4.4 Delete the dmsession raw `ENTITY_STATES` write.
- [x] 4.5 Compose the zero-static-contract ownership substrate from public
  SemStreams primitives, attach lifecycle ownership before workflow
  registration, and bind the enabled rule pack after component creation.
- [x] 4.6 Disable every product writer and product seed path whose complete
  projection or lifecycle owner adapter is not bound.
- [x] 4.7 Prove the real production composition boots without a static
  projection contract, does not pre-create the graph bucket, and does not race
  the graph-ingest catalog owner.

## 5. Preserve completed rule-engine migration

- [x] 5.1 Keep one `quest-lifecycle` pack, explicit graph integration, supported
  entity watch configuration, canonical per-definition entity patterns, and
  omitted lifecycle-only projection contracts.
- [x] 5.2 Keep retry/triage comparisons against each quest's maximum.
- [x] 5.3 Keep framework entity-reference datatype, gated-DAG defaults, and
  boundary-correct entity-prefix helpers.
- [x] 5.4 Pass rule-pack derivation, lifecycle, hot-reload, gated-DAG, and
  prefix behavior contracts without downstream compatibility code.

## 6. Repository acceptance

- [x] 6.1 Pass focused tests, `go test ./...`, integration-tag tests, and both
  default and integration `go test -race ./...` gates.
- [x] 6.2 Pass `go vet ./...`, `go build ./...`, Revive, static graph-access
  guards, predicate/entity/relationship audits, and generated contracts.
- [x] 6.3 Regenerate the beta.159 OpenAPI/TypeScript contract and pass schema,
  UI unit, Svelte diagnostic, and production build checks.
- [x] 6.4 Pass strict validation for this change and its two predecessor rule
  changes.
- [x] 6.5 Pass `git diff --check`; generated-diff review remains part of the
  repository acceptance gate.
- [ ] 6.6 Obtain architect, Go reviewer, technical-writer, CI, and Fable
  approval for the expanded API surface.

## 7. Operational acceptance

- [x] 7.1 Update the runbook and contract tests for beta.159 catalog inventory,
  readiness, flat GraphQL, truthful zero-state preflight, and the #814-blocked
  semantic replay.
- [ ] 7.2 Replace the runbook placeholder with the reviewed immutable Semdragon
  candidate commit.
- [ ] 7.3 Execute a fresh-volume zero-state framework-only start with one
  immutable backend image and every product seed flag false.
- [ ] 7.4 Wait for exact three-producer caught-up readiness and require exact
  zero graph summary, catalog stream messages, and authenticated workflow
  discovery.
- [ ] 7.5 Restart the same image with every seed flag disabled and require exact
  zero-state parity with no spontaneous entity.
- [ ] 7.6 After [SemStreams #814] lands, create deterministic lifecycle-owned state
  through its public API and pass nonempty semantic append/revision replay.
- [ ] 7.7 Report issue #59 complete only after all implementation, review, CI,
  and replay gates pass. Keep parent issue #4 open for remaining product scope.

## Execution evidence

- The beta.158 candidate `a4912d725ccb722e38b87feecb31b7c56e328ffc`
  failed replay after appending 15 duplicate hierarchy triples. That result is
  retained as historical evidence and is not beta.159 acceptance.
- SemStreams beta.159 closes upstream #712 and #713 and provides the public
  projection mutation primitive formerly tracked by #313.
- The final architecture review found that `service.WireOwnership` cannot boot
  a framework-only downstream composition with zero static projection
  contracts. [SemStreams #812] tracks the missing convenience primitive; the
  downstream fix now composes the existing public ownership primitives
  directly. Its Docker-backed production-composition regression reaches
  `StartAll`, observes graph-ingest, graph-index, and rule readiness plus the
  ownership service's running status, and creates a quest through the
  ownership-attached lifecycle Manager.
- The operational audit found no beta.159 public route that can create the first
  lifecycle instance. [SemStreams #814] tracks that missing primitive. The
  runbook specifies only an exact zero-state boot/restart preflight and leaves
  nonempty append/revision replay and issue #59 incomplete.
- Focused beta.159 configuration, readiness/runbook, and no-legacy
  `ENTITY_STATES` contracts pass. Exact stream policy, embedding removal,
  readiness configuration, strict leases, catalog-only reads, deleted raw
  writes, fail-closed product surfaces, and canonical patterns on all seven
  quest rules are implemented.
- Docker-backed integration proves catalog readers return transient
  `index_not_ready` without creating `ENTITY_STATES`, then converge through the
  same reader only after strict graph-ingest startup. The live rule processor
  reaches `claimed` to `in_progress`, consumes the command, and records one
  rule-sourced transition after caught-up readiness.
- After the production boot and runbook corrections, the complete default and
  integration test suites, both race modes, vet, build, Revive, module-tidy
  diff, static contract audits, generated schema checks, 349 UI tests, Svelte
  diagnostics, production UI build, all three strict OpenSpec validations, and
  `git diff --check` pass. Reviewer, CI, immutable candidate identity, zero-state
  preflight, and #814-gated semantic replay remain pending.

[SemStreams #313]: https://github.com/C360Studio/semstreams/issues/313
[SemStreams #712]: https://github.com/C360Studio/semstreams/issues/712
[SemStreams #713]: https://github.com/C360Studio/semstreams/issues/713
[SemStreams #812]: https://github.com/C360Studio/semstreams/issues/812
[SemStreams #814]: https://github.com/C360Studio/semstreams/issues/814
