# Tasks: adopt-semstreams-prev1-contract

## 1. Contract inventory and TDD gates

- [x] 1.1 Create the reviewed predicate rename ledger, including producer,
  consumers, cardinality, owner, write mode, UI mapping, and test evidence.
- [x] 1.2 Create the reviewed writer-owner matrix for every
  `ENTITY_STATES` mutation path; classify framework, product, and temporary
  migration writers.
- [x] 1.3 Inventory relationship-valued triples and add a failing contract test
  requiring `message.EntityReferenceDatatype` for graph edges.
- [x] 1.4 Add beta.158-compatible predicate and entity-ID audit tasks. Use
  scoped roots until upstream SemStreams #667 is fixed.
- [x] 1.5 Capture the baseline findings and classify intentional negative
  fixtures without broad file/directory exemptions.

## 2. Canonical graph corpus

- [ ] 2.1 Rename stored predicates to exact three-part lower-kebab names across
  domain vocabulary, graph producers, reconstruction, rule JSON, queries,
  fixtures, and documentation.
- [ ] 2.2 Replace dynamic predicate names with repeated canonical relationships,
  bounded structured values, or first-class entities as approved in the ledger.
- [ ] 2.3 Replace trailing-dot and malformed query prefixes with canonical
  one-through-six-segment prefixes.
- [ ] 2.4 Type every relationship triple as `@id`; update reconstruction and
  relationship query tests.
- [ ] 2.5 Remove copied gated-DAG marker overrides and use the framework's
  canonical default vocabulary.
- [ ] 2.6 Update UI predicate projections and graph relationship consumers;
  regenerate entity schema, OpenAPI, and TypeScript types.
- [x] 2.7 Replace two-phase/random-ID DAG seeding with the deterministic
  activation-latch protocol: canonical manifest digest, stable IDs,
  prerequisite-first atomic create-with-triples births, complete typed
  dependency sets, and no referential stubs.
- [x] 2.8 Add the private `questdag-completion` seam for
  `gateddag.unit.completed`. Use owner-scoped classified
  update-with-triples replacement, confirm ambiguous outcomes by authoritative
  read-back, and prohibit production append-style completion writes.
- [x] 2.9 Resolve
  [semdragon #12](https://github.com/C360Studio/semdragon/issues/12) by
  deleting the unproduced prompt-assembly `Graphable`, typed subject,
  vocabulary entry, and relationship fixture. Prove there is no production
  publisher or payload-registry registration. Do not invent a promptmanager
  component, projection owner, bind, or heartbeat for a writer that does not
  exist.
- [x] 2.10 Resolve
  [semdragon #14](https://github.com/C360Studio/semdragon/issues/14) by
  declaring and emitting all 21 canonical autonomy observability predicates,
  including exactly three same-subject `@id` relationships. Preserve the live
  raw typed-subject events, but do not bind a projection owner while no
  autonomy payload is registered or routed through graph-ingest. Hand issue #4
  an explicit wire-or-delete decision for the seven latent Graphables.

## 3. Verification and handoff to beta.158

- [ ] 3.1 Predicate audit has zero findings.
- [ ] 3.2 Entity-ID audit has zero findings.
- [ ] 3.3 Relationship coverage test proves every graph edge is typed `@id`.
- [ ] 3.4 `go build ./...`, unit tests, UI checks, and schema freshness gates
  pass while still pinned to beta.135.
- [ ] 3.5 Write and review the literal fresh-volume wipe/reseed and
  restart-without-write replay runbook.
- [ ] 3.6 Record the approved dependency handoff to
  [semdragon#4](https://github.com/C360Studio/semdragon/issues/4); do not bump
  to beta.158 in this change.
- [x] 3.7 Prove DAG admission at every interruption point: zero early
  dispatch, stable replay convergence without duplicate units, manifest
  collision rejection, activated-plan corruption rejection, root-only release
  after one activation commit, and a final corpus containing no
  `graph.StubMessageType` units. Link retirement to SemStreams #670.

## Completed core-wave evidence

This evidence closes only tasks 2.7, 2.8, and 3.7. Tasks 2.1 through 2.6 and
3.1 through 3.6 remain open.

- `go test ./...` and `go test -race ./questdag` pass.
- Focused live graph-ingest/gated-DAG integration proves completion read-back,
  zero early dispatch at every create boundary, replay convergence, corruption
  rejection, and prerequisite-first release:

  ```sh
  go test -tags=integration ./questdag \
    -run 'Test(NATSUnitCompletionWriter_ReplacesAndConfirmsOneMarker|'\
  'SeedUnits_ExecutorSeesNoPartialManifestAtEveryCreateBoundary|'\
  'SeedUnits_LiveActivatedManifestCorruption|'\
  'SeedUnits_DispatchesInDependencyOrder)$' -count=1
  ```

- `task ui:test` and `task schema:check` pass.
- `task contract:predicates-core` passes for the currently declared
  `domain`, `questdag`, `questlifecycle`, and `config` roots.
- `openspec validate adopt-semstreams-prev1-contract --strict` passes.

The core predicate audit is not the repository-wide zero-findings gate in
task 3.1. Processor, service, entity-ID, relationship, full UI acceptance,
fresh-volume, and beta.158 adoption gates remain independently open.

## Completed issue #12 evidence

This evidence closes only task 2.9:

- `TestPromptAssemblyHasNoGraphProjectionSurface` in
  `prompt_projection_inventory_contract_test.go` stale-checks the relevant Go,
  configuration, relationship-fixture, and UI source inventory. It rejects the
  deleted prompt projection surface and external promptmanager registration
  hooks without banning framework projection contracts, ownership services,
  heartbeats, or timers for real publishers.
- `go test -race ./processor/promptmanager` and `go test ./...` pass.
- The pinned beta.158 predicate audit for `./processor/promptmanager` reports
  zero findings.
- Strict OpenSpec validation passes.
- Go review approved the implementation.

## Completed issue #14 evidence

This evidence closes only task 2.10:

- Product vocabulary and payload contract tests cover all 21 canonical
  autonomy predicates, exactly three same-subject `@id` relationships, and
  the approved scalar catalog aliases and counts.
- The vocabulary test restores the SemStreams registry with
  `vocabulary.SnapshotRegistry`, so it is isolated from other packages.
- `go test -race . ./domain ./processor/autonomy -count=1` and
  `go test ./...` pass.
- The pinned beta.158 predicate audit for `./processor/autonomy` reports zero
  findings, and the exact legacy underscore-predicate search reports zero
  occurrences.
- Source review confirms that no projection contract, owner bind, payload
  registration, or graph-ingest route was added.
- Strict OpenSpec validation and `git diff --check` pass.
- Architect and Go reviewer approved the implementation.
