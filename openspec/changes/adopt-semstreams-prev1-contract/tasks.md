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
- [x] 2.11 Resolve
  [semdragon #16](https://github.com/C360Studio/semdragon/issues/16) by
  declaring the 15 canonical predicates emitted by the three live execution
  lifecycle payloads, preserving their raw event publishers in executor and
  questbridge, and deleting the unproduced tool-call/result payload surfaces.
  Canonicalize the real web-fetch writer's three invalid names and two `@id`
  relationships, but defer its owner-safe mutation to a later packet. Add no
  payload registration, graph-ingest route, projection bind, or heartbeat.
  Hand graph routing to issue #4, generic tool telemetry to issue #7, and the
  temporary `dc.terms.modified` compatibility declaration to SemStreams #673.
- [x] 2.12 Resolve
  [semdragon #18](https://github.com/C360Studio/semdragon/issues/18) by
  replacing the four shipped trailing-dot graph-source prefixes with canonical
  one- or two-segment values. Preserve routing through exact-or-dot-descendant
  matching so `osh` matches `osh` and `osh.asset`, but never `oshx`. While
  pinned to beta.135, keep that comparison isolated. Issue #4 validates runtime
  configuration with `types.ValidateEntityIDPrefix` and uses
  `graph.MatchesAnyIDPrefix` only for concrete six-part entity IDs.
  [SemStreams #675](https://github.com/C360Studio/semstreams/issues/675) owns
  canonical prefix-to-prefix containment; do not misuse the full-ID matcher for
  partial query prefixes. Treat a matching but unready source as authoritative
  and return no query targets; use local fallback only when no source owns the
  prefix. Delete the isolated comparison when a tagged SemStreams release
  contains #675.
- [x] 2.13 Resolve
  [semdragon #20](https://github.com/C360Studio/semdragon/issues/20) by
  deleting the graph-only `EntityID`, `Triples`, and `Schema` methods and
  `Graphable` assertions from the session start/end event DTOs. Preserve the
  raw typed subjects, JSON fields, validation, and internal publisher methods,
  but do not treat factory registration and internal component-to-manager calls
  as active deployment: no shipped configuration activates dmsession, and no
  production code outside `processor/dmsession` calls its component entrypoints.
  Add an inventory guard proving the raw event contract remains while no
  payload registry, graph-ingest route, projection bind, or graph predicates
  survive. Correct the writer-owner matrix for dmsession's raw `Session` JSON
  write into the shared board entity-state bucket, and hand authoritative
  lifecycle/owned-state migration to issue #4.
- [x] 2.14 Resolve
  [semdragon #22](https://github.com/C360Studio/semdragon/issues/22) by
  deleting the graph-only `EntityID`, `Triples`, and `Schema` methods and
  `Graphable` assertions from all five guild event DTOs. Preserve the active
  raw DTOs, typed subjects, validation, publisher calls, and JSON wire fields.
  Add an inventory guard proving shipped activation and raw publication remain
  while no guild event payload registry, `BaseMessage`, graph-ingest route,
  projection bind, heartbeat, graph reader, or relationship fixture survives.
  Do not rename `guild.event.created_by` or
  `guild.lifecycle.disbanded_at`; the former duplicates authoritative
  `guild.founding.agent`, and the latter has no authoritative Guild field.
  Hand the real multiwriter full-object guild state, dynamic membership and
  application children, and pending-timeout lifecycle migration to issue #4.
- [x] 2.15 Resolve
  [semdragon #24](https://github.com/C360Studio/semdragon/issues/24) by
  deleting all five `processor/seeding` files; its import, two registration
  callbacks, and two component-name entries; all seven dead seeding vocabulary
  constants and registrations; all four root relationship fixtures; and stale
  processor documentation. Delete the four DTOs, subjects, validation,
  publisher calls, JSON fields, `Graphable` assertions, and graph methods
  without a raw-wire compatibility promise. Preserve the separate
  `cmd/semdragons` startup/E2E Agent seeder. Hand its direct shared-state birth
  to issue #4 or a dedicated packet for graph-ingest create-with-triples and
  the agentprogression ownership boundary. Add no projection owner, heartbeat,
  replacement event route, private framework, or upstream SemStreams issue.
- [ ] 2.16 Resolve
  [semdragon #26](https://github.com/C360Studio/semdragon/issues/26) by
  removing bossbattle activation from all three shipped configurations and
  enabling the complete quest lifecycle rule pack, including the mechanics
  stub, in each profile. Delete the five unproduced `battle.review.*`
  vocabulary contracts, false raw event/verdict ports, questboard's false
  battle-event port, dmsession battle routing, three battle game-event types,
  and the dead retreat Graphable/status/UI fixture surface. Retain the dormant
  authoritative `BossBattle` model, evaluator, REST/dmworldstate/UI readers,
  real battle-state KV port, and typed quest/agent relationships. Record the
  indexed-predicate defect, fresh-state canonical target, exact cross-owner
  handoffs, SemStreams #313 dependency, and reactivation block without
  implementing the Phase-2 writer in this packet.
- [x] 2.17 Resolve
  [semdragon #28](https://github.com/C360Studio/semdragon/issues/28) by
  deleting the eight no-consumer party event DTO/subject/publisher/Graphable
  families, all thirteen event-only vocabulary entries, false party event
  ports, two phantom dmsession party event types, and event relationship
  fixtures. Delete test-only decomposition/result/rollup methods and their
  strategy/context/result/rollup shadow fields across Go, API/schema,
  TypeScript, UI, tests, and docs. Retain authoritative Party KV
  `EmitEntity`, form/join/disband/read APIs, KV SSE message-category mappings,
  and temporary `AssignTask`/`SubQuestMap`. Prove the audit drops exactly ten
  findings—nine event projections plus shadow-only `party.strategy`—while the
  four retained authoritative findings (`party.quest`, `party.lead`,
  `party.lifecycle.formed_at`, and `party.lifecycle.disbanded_at`) and the
  scanner-hidden member/assignment dynamic families remain explicitly
  deferred. Do not
  canonicalize the dynamic UI parser, bind an owner, bump SemStreams, activate
  questdag, copy a mutator, or add a replacement event plane. Hand the
  canonical Party lifecycle and first-class child projection to issue #4 and
  SemStreams #313.

  Completed evidence (2026-07-25): the deletion guard and pinned audit pass;
  the audit is 4/8 with the four retained authoritative findings. Party
  mutations use candidate emit-then-commit, deep clones, and rollback on emit
  failure. `go test ./...` passes. `go build ./...` exits 0 with only a
  non-fatal module stat-cache permission warning. `npm test -- --run` passes
  342/342; `npm run check` reports 0 errors and 15 pre-existing warnings;
  `npm run build` and `task schema:check` pass. No owner or replacement raw
  Party event plane was added, and retained SSE activity remains KV-backed.
- [x] 2.18 Resolve
  [semdragon #30](https://github.com/C360Studio/semdragon/issues/30) by
  retaining `agent_progression` registration but disabling it in all three
  shipped profiles. Delete the unproduced `AgentXPPayload`, its graph/schema
  surface, the false `agent-xp`/`AGENT_EVENTS` output, and the six event-only
  progression vocabulary entries. Preserve the Agent model, reconstruction,
  XP engine and calculation types, validation, configuration, handlers,
  watchers, and dormant Phase-2 tests. Starting from the historical 22/32
  beta.158 baseline, retain the measured post-change audit evidence of 17/26:
  exactly five fewer invalid and six fewer extracted candidates. Make no
  agentstore, dependency, predicate rename, owner, lifecycle, or watcher
  change. Keep reactivation blocked on issue #4 and SemStreams #313.

  Completed evidence (2026-07-25): all three shipped profiles disable
  `agent_progression`. Completion through the shipped mechanics stub and a
  direct valid lifecycle transition to terminal `failed` both preserve the
  seeded Agent and Guild KV revisions and bytes and produce no mechanics side
  effects. The pinned beta.158 audit moves from 22/32 to 17/26. Focused unit,
  race, and integration tests; the full Go test and build; schema freshness;
  strict OpenSpec validation; and diff checks pass. Architect and independent
  Go reviewer approve the slice.
- [x] 2.19 Resolve
  [semdragon #32](https://github.com/C360Studio/semdragon/issues/32) by
  retaining `agent_store` registration but disabling it in all three shipped
  profiles. Preserve product policy, catalog/inventory/effect models and
  validation, dormant handlers/watchers, REST routes, OpenAPI, generated
  models, the API 503 contract, and autonomy's unavailable-store no-op.
  Delete the five publisher-only payload/subject/publisher families, all seven
  raw-event vocabulary entries, and the false component output. Preserve
  EntityState KV message categories. Verify the pinned beta.158 audit moves
  from the historical 23/30 baseline to 14/18 by deleting payloads.go's 9/12
  corpus while retaining store.go's 14/18 corpus. Make no agentprogression,
  dependency, retained-predicate, owner, lifecycle, graph-view, API, or UI
  change. Keep reactivation blocked on issue #4 and SemStreams #313; open a
  linked owned-create issue only if the Phase-2 implementation proves #313
  insufficient. Record, but do not implement, the future graphview read
  projection, idempotent durable agentprogression XP-debit command, prohibition
  on full Agent replacement, and exclusion of Agent lifecycle/status from
  agentstore authority.

  Completed evidence (2026-07-25): all shipped profiles disable
  `agent_store`, and a real ComponentManager creates no agentstore instance.
  All six routed store requests return 503; deep-isolated tests prove all four
  autonomy store actions are nil-store no-ops with no mutation. The raw
  payload/subject/publisher plane, seven vocabulary entries, and false output
  are absent while policy, models, routes, and schemas remain. Fresh-graph
  integration preserves seeded Agent and Guild state and creates no unexpected
  entity. The pinned beta.158 audit is exactly 14/18. Focused unit/race and
  agentstore integration-race, `go test ./...`, `go build ./...`, entity and
  OpenAPI schema freshness, `npm run check` with 0 errors and 15 unrelated
  existing warnings, both strict OpenSpec validations, and diff checks pass.
  Architect, independent Go reviewer, and Svelte reviewer approve the slice.
- [x] 2.20 Resolve
  [semdragon #34](https://github.com/C360Studio/semdragon/issues/34) by
  retaining boidengine registration/configuration and disabling it in all
  three shipped profiles. Preserve `DefaultBoidEngine`, rules,
  attraction/scoring policy, DTOs, dormant `boid.suggestions.>` /
  `boid.guild.>` contracts, `BOID_SUGGESTIONS`, and dmpartyformation's direct
  policy reuse. Remove only the unproduced shipped `agent.boid.>` stream
  entries. Prove no live component or bucket is created, autonomy produces no
  claim/join/form mutation, and seeded Agent, Quest, and Guild state remains
  bit-for-bit unchanged until an explicit claim. Prove REST API or test-fixture
  claim/start reaches questbridge and mechanics-stub completion without
  mechanics side effects; the lifecycle operator joins this path only when
  separately enabled under task 4.3. Reclassify positive boid E2E coverage as
  dormant/opt-in and keep the pinned beta.158 audit at zero structured
  candidates. Record the future
  `pkg/graphview`-backed reads and durable KV/stigmergic projection without
  implementing them. Add no fallback recommender, local framework, graphview,
  owner, dependency, API/schema, subject/KV migration, or upstream issue;
  SemStreams #587 already covers message-logger per-client KV-watch migration.

  Completed 2026-07-25 evidence, closing only task 2.20:

  - All three shipped configurations retain boidengine with `enabled: false`
    and remove the stale `agent.boid.>` stream subject.
  - Real ComponentManager coverage proves no boidengine instance is created
    and startup creates no `BOID_SUGGESTIONS` bucket.
  - No-producer integration coverage proves exact seeded Agent, Quest, and
    Guild KV stability before an explicit claim.
  - The positive shipped path explicitly calls
    `POST /game/quests/{id}/claim`, then
    `POST /game/quests/{id}/start`, and reaches lifecycle rules, questbridge
    execution, and mechanics-stub completion without mechanics side effects.
  - Contract coverage preserves `DefaultBoidEngine`, rules, scoring policy,
    DTOs, dormant suggestion subjects/KV, and dmpartyformation policy reuse.
  - The pinned beta.158 boidengine predicate audit reports zero structured
    candidates.
  - Focused integration and race gates, the full Go suite and build, schema
    gates, Svelte checks/tests/build, Playwright coverage, both strict OpenSpec
    validations, and diff checks pass.
  - Architect, independent Go reviewer, and Svelte reviewer approve the slice.
- [x] 2.21 Resolve
  [semdragon #36](https://github.com/C360Studio/semdragon/issues/36) by
  retaining redteam registration/configuration and disabling it in all three
  shipped profiles. Remove only the false `redteam.lifecycle.posted` and
  `redteam.lifecycle.completed` OutputPorts; retain the
  `redteam.lifecycle.*` category/vocabulary taxonomy, Quest graph/model
  fields, findings/lesson policy, prompts, API/UI surfaces, and opt-in E2E.
  Prove a real ComponentManager creates no redteam instance, watcher, pending
  review, or deadline and that explicit claim/start reaches questbridge and
  mechanics-stub completion without a red-team Quest, status mutation, Guild
  lesson, or event side effect. Record without fixing the legacy underscore
  target watcher, raw process-local caches, unsafe full-Quest/full-Guild
  writers, inert claim timeout, and restart-fragile local deadlines.
  Reactivation remains blocked on Semdragon #4/#5/#22 and SemStreams
  #313/#676. Make no dependency, graphview, ownership, writer, lifecycle,
  rule, API/UI/schema, or deadline migration; leave historical audit
  baselines unchanged.

  Completed 2026-07-25 evidence, closing only task 2.21:

  - The default, semsource, and tier-1 E2E configurations retain redteam with
    `enabled: false`; a real ComponentManager creates no redteam component,
    watcher, pending-review cache, or deadline.
  - Redteam exposes no OutputPorts, and no replacement subject, payload,
    publisher, stream, or compatibility alias is introduced.
  - Explicit REST claim/start reaches questbridge and mechanics-stub
    completion with `review-skipped-mechanics-disabled`. The fresh-state
    integration proof creates no red-team Quest/artifact, message category,
    classification mutation, Guild revision, or other mechanics side effect.
  - Contract coverage retains the `redteam.lifecycle.*` category/vocabulary
    taxonomy, Quest graph/model fields, findings/lesson policy, prompts,
    API/UI surfaces, and dormant implementation.
  - The pinned beta.158 redteam audit reports zero structured candidates.
    Playwright red-team coverage is explicitly opt-in and guarded by both
    redteam and bossbattle activation.
  - Focused unit/integration/race gates, the full Go test and build, schema
    freshness, Svelte checks/tests/build, Playwright, both strict OpenSpec
    validations, and diff checks pass.
  - Architect, independent Go reviewer, Svelte reviewer, and technical writer
    approve the slice. SemStreams #676 records the durable entity-deadline
    blocker; no duplicate upstream issue was opened.
- [x] 2.22 Resolve
  [semdragon #38](https://github.com/C360Studio/semdragon/issues/38) by
  enabling the upstream lifecycle gateway in all three shipped profiles at
  `/lifecycle-gateway/workflows`. Reuse the single lifecycle Manager, expose
  only its sole registered `quest` workflow, disable WebSockets, cap bodies at
  64 KiB, and protect the complete namespace with the existing
  `SEMDRAGONS_API_KEY` semantics. Treat the gateway as an operator and
  break-glass surface while product command handlers coexist through the
  separate task 4.4 retirement gate.

  Completed 2026-07-25 evidence: registration and profile contracts, shared
  Manager composition, path-boundary authentication, disabled streaming,
  bounded bodies, static upstream-provider OpenAPI rebasing, and resolved
  request schemas are covered by focused tests. A real component-stack
  integration drives an authenticated `in_progress` to `in_review` operator
  transition through the mechanics-stub rule to atomic
  `review-skipped-mechanics-disabled` completion and verifies ordered operator
  and rule history. The focused unit and integration commands, the full Go
  suite, and both strict OpenSpec validations pass. SemStreams
  #677/#678/#679/#680/#681/#682 retain ownership of atomic transition fields,
  gateway policy, runtime gateway paths, OpenAPI security policy,
  lifecycle-history retention, and the oversized-body status mismatch. The
  current legacy history-10 bucket bootstrap is recorded as temporary rather
  than durable framework ownership. No downstream framework substitute is
  introduced. Task 4.4 remains open.

- [x] 2.23 Resolve Semdragon
  [#39](https://github.com/C360Studio/semdragon/issues/39),
  [#40](https://github.com/C360Studio/semdragon/issues/40),
  [#41](https://github.com/C360Studio/semdragon/issues/41), and
  [#42](https://github.com/C360Studio/semdragon/issues/42) at the container
  boundary. Align all Go build/runtime images with Go 1.26.4, remove the
  obsolete SemSource `headless` key, isolate the complete SemSource service on
  its own JetStream, retain the real SemEmbed health probe with sufficient
  cold-start grace, and package the configured quest rules in the backend
  image.

  Completed 2026-07-25 evidence: clean Compose created independent Semdragon
  and SemSource NATS volumes with explicit `/data` JetStream stores; SemSource
  reached healthy without poisoned Semdragon entities, platform-identity
  refusal, or durable-consumer conflict. Its real
  `/source-manifest/status` response reported `phase: ready` with five
  entities, and GraphQL introspection returned query type `Query`; the NATS
  container confirmed its store at `/data/jetstream`.

  Backend, mock-LLM, and sandbox builds completed with Go 1.26.4 builders.
  A direct sandbox runtime probe returned
  `go version go1.26.4 linux/arm64`. SemEmbed loaded after 86 seconds and
  reached its real `/health` endpoint inside the 120-second start period. The
  backend image contained all four quest rule files, and the focused Tier-1
  Playwright lifecycle spec passed 4/4 through the UI proxy, including
  rule-driven completion, verdict, and ordered history. A static packaging
  contract prevents future images from omitting the rule tree.

- [x] 2.24 Resolve
  [semdragon #44](https://github.com/C360Studio/semdragon/issues/44) as an
  incremental dormant Agent-projection packet. Canonicalize exactly the 17
  retained scalar predicates recorded in D24 and the predicate ledger.
  Replace dynamic `agent.skill.{tag}.{level,total_xp}` predicates with repeated
  bounded `agent.skill.proficiency` objects containing `tag`, `level`, and
  `total-xp`. Cut producer, reconstruction, UI consumers, and generated schema
  to canonical-only names in one gate. Preserve typed `@id` relationships for
  Guild, current Quest, and current Party, while scalars and skill objects
  remain non-relationships. Keep `agent_progression` disabled in all three
  shipped profiles.

  Require the pinned beta.158 agentprogression audit to report zero invalid
  from exactly 27 scanner-visible candidates, with stale-name, runtime-tag,
  producer/round-trip, relationship, UI, and schema guards. Record the
  historical 22/32 baseline and issue #30's post-quarantine 17/26 measurement
  without rewriting either. Explicitly retain scanner-hidden dynamic tool,
  consumable, and active-effect projections plus unsafe full-object writers as
  blockers for issue #4 and SemStreams #313. Add no inventory-entry or
  active-effect child entity, agentstore ownership, projection bind,
  heartbeat, owner-token mutation adapter, dependency bump, configuration
  change, compatibility alias, dual read/write, or component reactivation.

  Completed 2026-07-25 evidence: producer and round-trip tests cover all 17
  scalars, multiple deterministic bounded skill objects, stale/runtime-tag
  absence, and the three typed Agent relationships. Canonical-only UI tests
  and generated-schema contracts cover every required and optional Agent field
  with no dynamic skill placeholder. The pinned beta.158 audits report
  agentprogression 0/27, UI 0/41, and entity IDs 0/20 while retaining the
  documented scanner-hidden blockers.

  Focused race tests, `go test ./...`, `go vet ./...`, `go build ./...`, all
  344 UI tests, Svelte check with 0 errors and 15 pre-existing warnings,
  schema freshness, both strict OpenSpec validations, and diff checks pass.
  Architect review approves the bounded dormant-projection boundary. Svelte
  review approves canonical-only consumers and complete generated-schema
  coverage. No excluded ownership, dependency, child-entity, compatibility,
  writer, configuration, or reactivation work is included.

- [x] 2.25 Resolve
  [semdragon #46](https://github.com/C360Studio/semdragon/issues/46) as a
  bounded dormant agentstore-projection packet. Canonicalize exactly the 14
  retained mappings recorded in D25 and the predicate ledger. Cut producer and
  reconstruction to canonical-only predicates with no alias or dual
  read/write. Remove the `store.item.<id>` identity fallback; require a
  receiver, BoardConfig, and item ID for
  `BoardConfig.StoreItemEntityID`, otherwise return the empty sentinel and nil
  triples. Preserve configured `DefaultCatalog` aliases as canonical
  BoardConfig-derived StoreItem IDs.

  Keep `agent.inventory.tool` as a repeated scalar catalog alias with empty
  datatype, not `@id`. Make no UI or generated-schema change because REST JSON
  is unchanged and dormant graph StoreItems are outside the generated primary
  entity schema. Keep `agent_store` disabled in all three shipped profiles.
  Add no inventory-entry or active-effect child entity, projection owner,
  bind, heartbeat, owner-token writer, dependency/configuration change,
  component reactivation, compatibility path, local validation framework, or
  SemStreams #313 substitute.

  Completed 2026-07-25 evidence: producer, reconstruction, stale-name,
  scalar-alias, fail-closed identity, and configured-catalog tests cover the
  bounded contract. The pinned beta.158 agentstore predicate audit reports
  0/18. The pinned entity-ID audit reports 0/23 with the empty return classified
  exactly as `intentional-sentinel`.

  Focused and race tests, `go test ./...`, `go vet ./...`, `go build ./...`,
  and 166 targeted UI tests pass. Go review approves the canonical projection
  and identity boundary. Svelte review approves the intentional no-change
  UI/schema boundary. Semdragon remains pinned to beta.135, and global tasks
  3.1 through 3.6 remain open.

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

## Completed issue #18 evidence

This evidence closes only task 2.12:

- The initial TDD RED cases exposed one- and two-segment lexical collisions,
  the owned-but-unready fallback leak, and all four shipped trailing empty
  segments.
- The three shipped configs now use canonical `local.semsource` and `osh`
  values. Tri-state routing distinguishes a ready owner, an unready owner, and
  an unowned prefix; only the last may use local fallback.
- The minimal beta.135 exact-or-dot-descendant comparison remains isolated and
  linked to
  [SemStreams #675](https://github.com/C360Studio/semstreams/issues/675).
  Issue #4 uses beta.158 validation/full-ID matching and deletes the comparison
  when a tagged SemStreams release contains #675.
- `go test -race ./processor/questbridge -count=1`, the root shipped-config
  contract test, and `go test ./...` pass.
- The UI suite passes all 342 tests.
- The pinned beta.158 entity-ID audit reports
  `passed: 20 structured candidates across 1 roots`.
- Strict OpenSpec validation, exact legacy-prefix search, shipped JSON parsing,
  and `git diff --check` pass.
- Architect and Go reviewer approved the implementation.

## Completed issue #16 evidence

This evidence closes only task 2.11:

- Vocabulary and payload tests cover all 15 execution lifecycle predicates and
  seven product `source.doc.*` predicates. The web writer also reuses
  SemStreams `vocabulary.DCTermsTitle`.
- Tool-call and tool-result payloads, subjects, vocabulary metadata, and graph
  fixtures are deleted. A stale-checked inventory test covers the production,
  vocabulary, and relationship-fixture surfaces.
- Contract tests prove exactly three retained `@id` relationships:
  `execution.agent.id`, `source.doc.fetched-by`, and `source.doc.quest-id`.
- The isolated `DCTermsModified` compatibility declaration is linked to
  SemStreams #673 and remains scheduled for deletion on tagged upstream
  adoption.
- `go test -race . ./domain ./processor/executor ./processor/questbridge -count=1`
  and `go test ./...` pass.
- The pinned beta.158 predicate audit for `./processor/executor` reports zero
  findings.
- Source review confirms that no payload registration, graph-ingest route,
  projection contract, owner bind, or heartbeat was added.
- Strict OpenSpec validation and `git diff --check` pass.
- Architect and Go reviewer approved the implementation.

## Completed issue #20 evidence

This evidence closes only task 2.13:

- The initial TDD RED inventory identified both `Graphable` assertions, all
  six graph-only methods, and the four latent predicates
  `dm.session.mode`, `dm.session.name`, `dm.session.quests_completed`, and
  `dm.session.quests_failed`.
- The graph-only surface is deleted. The raw start/end DTOs, typed subjects,
  `Validate` methods, and internal publisher wire contract remain, including
  the numeric `quests_completed` and `quests_failed` JSON fields.
- The deterministic Docker integration test establishes both typed
  subscriptions, flushes them with a NATS RTT, and uses channels for delivery.
  It passes under the race detector for ten repetitions and in the full tagged
  dmsession integration suite.
- The stale-checked inventory mutation guard passes under the race detector for
  100 repetitions. Isolated vocabulary tests prove that only the retained raw
  event subjects remain registered.
- The pinned beta.158 predicate audit for `./processor/dmsession` reports
  `passed: 0 structured candidates across 1 roots`.
- The writer-owner matrix now records that dmsession writes raw `Session` JSON
  under `session.<instance>` into the shared `ENTITY_STATES` bucket. It
  classifies that path as downstream misuse and hands authoritative
  lifecycle/owned-state migration to issue #4 without inventing a projection
  writer.
- `go test ./...` and `go build ./...` pass.
- Strict OpenSpec validation and `git diff --check` pass. The historical
  baseline remains unchanged.
- Architect and Go reviewer approved the implementation.

## Completed issue #22 evidence

This evidence closes only task 2.14:

- The TDD RED inventory caught all five `Graphable` assertions, fifteen
  graph-only methods, and five relationship fixtures.
- The graph-only surface is deleted. All five raw DTOs, typed subjects,
  `Validate` methods, live publisher calls, and exact JSON wire contracts
  remain.
- The AST inventory rejects receiver and assertion mutations, proves both
  processor registry functions and all three shipped configurations, and
  permits raw consumers and the authoritative `domain.Guild` projection.
- Deterministic Docker integration covers all five typed NATS contracts using
  flushed subscriptions, a NATS RTT, and channel-based delivery.
- The absolute `processor/guildformation` beta.158 audit reports
  `predicate audit passed: 0 structured candidates across 1 roots`.
- The corrected predicate ledger and writer-owner matrix hand multiwriter
  state, dynamic membership/application children, and the pending ticker to
  issue #4 or a dedicated packet. Deletion needs no upstream framework issue.
- Focused race, Docker integration, `go test ./...`, `go build ./...`, strict
  OpenSpec validation, and `git diff --check` pass.
- Architect and final Go reviewer approved the implementation.

Tasks 3.1 through 3.6 remain open.

## Completed issue #24 evidence

This evidence closes only task 2.15:

- The TDD RED inventory caught exactly five package files, four `Graphable`
  assertions, twelve graph-only methods, four DTO/subject/publish families,
  four relationship fixtures, seven vocabulary constants plus seven
  registrations, and five component-registry references.
- The entire `processor/seeding` package and its registry, vocabulary, fixture,
  and documentation surfaces are deleted without preserving a raw DTO
  contract.
- Startup environment selection, the binary invocation, and authoritative
  Agent birth remain separate and tested.
- AST, shipped-config, historical thirteen-literal, and replacement-route
  mutation guards reject reintroduction while allowing shared framework owner
  and authoritative Agent surfaces.
- Deterministic sentinel Docker integration observes zero `seeding.>` events
  and reconstructs the retained Agent from stored `EntityState`.
- The provider-arena proposal now composes SemStreams lifecycle, rules,
  bounded dispatch, first-class entities, graph-ingest, and classified
  projection ownership rather than the deleted processor.
- The pinned beta.158 retained-root audit reports
  `predicate audit passed: 260 structured candidates across 3 roots`.
- Focused race, Docker integration, `go test ./...`, `go build ./...`, strict
  OpenSpec validation, and `git diff --check` pass.
- The README, predicate ledger, and writer-owner matrix are corrected.
  Owner-safe Agent birth remains an issue #4 or dedicated-packet handoff, and
  deletion exposes no upstream framework gap.
- Architect and final Go reviewer recorded **APPROVE**.

Tasks 3.1 through 3.6 remain open.

## Issue #26 evidence, pending final root gates

This evidence supports task 2.16 but deliberately leaves it unchecked:

- The initial contract RED exposed bossbattle activation in every shipped
  profile and an incomplete quest rule pack outside the default profile.
- All three profiles now omit bossbattle and declare the exact enabled
  `rule-processor`, ordered four-file quest pack, and quest entity watch.
- The fresh-graph integration path reaches `completed` with
  `review-skipped-mechanics-disabled` and creates no battle, peer review, XP,
  reputation, store, inventory, or artifact side effect.
- Stale-checked inventory and mutation tests cover the five phantom event
  subjects, ports, routing, game-event values, and retreat-only graph, status,
  OpenAPI, generated TypeScript, and UI surfaces.
- The dormant authoritative model, evaluator, component factory,
  battle-state KV port, readers, and typed quest/agent relationships remain.
  No projection owner or heartbeat is bound.
- The migration ledger records the unchanged 8/19 audit result, the
  scanner-hidden indexed-family defect, the fresh-state structured target,
  exact cross-owner handoffs, SemStreams #313, and the Phase-2 reactivation
  block.
- Focused and full Go/UI gates, repeated mutation checks, tagged integration
  fixtures, Svelte diagnostics, schema freshness, and diff checks pass under
  architect, Go, and Svelte review.

Task 2.16 awaits the final root combined gate. Tasks 3.1 through 3.6 remain
open.
