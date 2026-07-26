# Tasks: rebuild-quest-lifecycle-on-semstreams-rules

## 0. Prerequisite — SemStreams beta.158 graph contract

- [ ] 0.1 Complete and archive the `adopt-semstreams-prev1-contract` change:
  canonical predicates, entity IDs/query prefixes, `@id` relationships,
  writer-owner matrix, and fresh-state cutover.
- [ ] 0.2 Wait for the beta.158 adoption change tracked in
  [semdragon#4](https://github.com/C360Studio/semdragon/issues/4) to pass its
  exact-version, ownership, fresh-volume, and replay gates.

## 1. Stage A — dependency bump and contract adaptation

- [x] 1.1 Read `semstreams/docs/operations/migration-*.md` for beta.21→beta.132; list every item touching semdragons (ownership ADR-054/055/056, agentrun ADR-053, payload registry, message shapes) — findings in `research/00-stage-a-findings.md` (+ 01-04 cluster files). Verdict: near-zero compile risk; real work is Stage B + ownership partitioning.
- [x] 1.2 Bump `go.mod` to semstreams v1.0.0-beta.132; fix compilation
  across all packages — `go build ./...` + `go vet ./...` clean, zero source
  changes needed. Beta.132 requires Go ≥ 1.26.3; issue #39 aligns the backend,
  mock-LLM, and sandbox build/runtime images on Go 1.26.4 so the Stage B3 E2E
  gate runs the same supported toolchain as local verification. Issues #40 and
  #41 also update the current SemSource configuration boundary and SemEmbed
  cold-start health window discovered by that real-image gate.
- [x] 1.3 Ownership spike: write and read one quest entity under beta.132 ownership/projection rules; document which `quest.*` predicates need ownership claims for rules vs questbridge — questboard integration PASS under beta.132 (ownership is observe-only; raw-KV-`Put` path unaffected). Predicate partition documented in `research/05-ownership-spike.md`: lifecycle owns `quest.status/lifecycle/attempts/failure/assignment/verdict/data.output`, questbridge owns `quest.execution/context/loop/metrics`, questdag owns `quest.dag.*`.
- [x] 1.4 Adapt payload registry, BaseMessage wrapping, and graph-write call sites until `task test` is green with the existing architecture — `task test` green (EXIT=0) with **zero adaptation**; additive-only wire + unchanged payload registry confirmed empirically.
- [x] 1.5 Add contract-level integration tests: questbridge↔agentic-loop TaskMessage round trip, questtools↔tool.execute/tool.result round trip — added `processor/questbridge/contract_test.go` (`TestContract_TaskMessageWireRoundTrip`) and `processor/questtools/contract_test.go` (`TestContract_ToolCallWireFidelity`); both decode through the production payload registry and pass under beta.132. Pre-existing behavioral round-trip tests (`TestQuestStartedPublishesTaskMessage`, `TestCorrelationIDPropagation`) also green.
- [x] 1.6 Resolve the QUEST_LOOPS vs framework agentrun question; document the decision in design.md Open Questions — **KEEP QUEST_LOOPS, do not adopt agentrun** (per-loop vs per-run, additive wire, ADR-053 §Deferred excludes semdragon). Recorded in design.md Open Questions.
- [x] 1.7 `task test:integration` green with the existing architecture (Stage A gate) — **30 packages OK, 0 failures** under beta.132. Stage A complete.

## 2. Stage B1 — quest lifecycle as declared workflow

- [x] 2.1 Define `QuestState` schema struct with `lifecycle` tags and register the quest `Workflow` (transition table per quest-lifecycle spec, terminal phases, `quest.status` as phase predicate) — new `questlifecycle` pkg (`workflow.go`): `QuestState` projection, `Transitions` table (all questboard edges + spec), phase predicate `quest.status.state` (design D3), audit predicates, operator-writable retry/assignment/verdict fields. Phase consts = `domain.QuestStatus` (single source of truth). Unit tests + integration registration test green; `Manager` created + workflow registered in `cmd/semdragons/main.go`.
- [x] 2.2 Author `config/rules/quest/*.json`: claim/start edges, submission routing (review vs direct completion), failure retry-vs-triage routing — authored 4 rule files (01-claim-start, 02-submit ×2 gated on `quest.review.needs_review`, 03-fail ×2 gated `lt`/`gte` on `quest.attempts.current`, 04-mechanics-stub) against the verified grammar in `research/06-command-rules.md`: phase gate `$entity.lifecycle.phase`, command triple `quest.command`, `set` substitution `$entity.triple.<pred>`, command cleared to sentinel `"consumed"` atomically. Added `Command` operator-writable field to `QuestState`. Registered `rule-processor` in componentregistry + enabled in `config/semdragons.json` (rules_files + `entity_watch_patterns`). Build green, JSON valid, registration test green. Rules fire only on lifecycle-attached quests with command triples → safe side-by-side with questboard. **Not yet fired end-to-end** (needs the caller rewire 2.4–2.6 + graph-ingest; validated at the mock e2e gate 5.2).
- [x] 2.3 Implement attempt-conditional routing via lifecycle Manager API where rule conditions cannot express it; attempts increment atomic with claim (`TransitionWith`) — attempt-conditional routing IS fully rule-expressible (03-fail.json: `lt`/`gte` on `quest.attempts.current`, literal max=3); attempts increment atomic with claim via the claim rule's `set: {attempts: increment}`. No Manager-API escape hatch needed. **Proven end-to-end** by `questlifecycle/rules_fire_test.go` (real graph-ingest + rule-processor + Manager: command=claim → claimed, claimed_by set, attempts=1, command cleared). This firing proof caught semstreams bug #451 (hot-reload drops the lifecycle Manager → `$entity.lifecycle.phase` never resolves); worked around with bare-predicate `quest.status.state` gating (research/07-rule-firing-bug.md).
- [x] 2.4 Port dependency gating (claim blocked until deps completed) and creation-time cycle rejection to the new seam — dependency gate added at the new claim seam (`autonomy.checkDependenciesMet`, `processor/autonomy/actions.go`): a claim whose `depends_on` quests are not all `completed` is skipped. Creation-time cycle rejection is unchanged and still enforced by `domain.ValidateQuestChainBrief` (Kahn's algorithm) on the post path, so no move was needed. (service-api's manual claim keeps questboard's `checkDependenciesMet` until 4.4.)
- [~] 2.5 Port questboard transition tests to the workflow; delete bespoke transition handlers from `processor/questboard` — **PARTIAL (rewire-now / delete-later, per Coby): all callers now rewired; handler DELETION deferred to B2/B3.** DONE: (a) command-delta seam `questlifecycle/command.go` (StampTriple/StampCommand — additive graph-ingest deltas, NOT full-object Put which clobbers the shared ENTITY_STATES entity); (b) firing tests proving submit/fail routing + the no-`Create` path (`questlifecycle/rules_fire_more_test.go`); (c) **autonomy** claim/start rewired onto `Manager.TransitionWith`/`Transition` (contention-safe: a loser gets `ErrInvalidTransition`; no command-field race); (d) **questbridge** `completeQuest` submit branches collapsed into one `command=submit` delta (rule routes review-vs-complete on `quest.review.needs_review`; questbridge no longer decides RequireReview, design D2), `failQuest` delegation + legacy terminal Put replaced with `command=fail` (rule routes retry-vs-triage on `quest.attempts.current`), execution metrics/output/failure-reason persisted as additive `StampTriple` deltas, and the now-orphaned `QuestFailer`/`resolveQuestFailer` removed. REMAINING (deferred): port questboard's transition tests and DELETE its single-agent transition handlers **once B2 (questdagexec/partycoord) and B3/4.4 (service-api/redteam) remove their remaining callers** — they cannot be deleted before then without breaking the build. questbridge's clarification-repost/escalate branches remain full-object `EmitEntityUpdate` (tolerable non-terminal degradation, finding #1); DAG-decomposition + party sub-quest clarification branches left for B2.
- [x] 2.6 Point questbridge's watch at the lifecycle phase predicate; verify dispatch still fires on `in_progress` — questbridge **already** watches `quest.status.state` and dispatches on `in_progress` (`handler.go:94,110,131`; D3 kept the predicate), so no watch change was needed. Confirming integration tests added (`processor/questbridge/submit_fail_rules_test.go`): with the full lifecycle stack (graph-ingest + rule-processor + Manager), a completed loop stamps `command=submit` → the no-review submit rule drives the quest to `completed` (work product preserved on `quest.data.output`), and a failed loop with attempts remaining stamps `command=fail` → the fail-retry rule reposts the quest and clears `claimed_by`. Both green; full questbridge integration suite green (incl. the loop counter tests, which stay counter-only and fail-fast on stamps with no graph-ingest — `nats: no responders`, not the 5s timeout originally feared).

## 3. Stage B2 — DAG execution on gated-dag

- [x] 3.1 Create `questdag` domain layer: DAG extraction/validation from lead tool output (≤20 nodes, cycle/self-ref/dep-ID checks), unit ID minting, unit entity seeding with `depends_on` edges — new root `questdag` pkg (D5 "means" layer): `DAG`/`Node` + `Validate()` (three-color-DFS cycle check, ported from questdagexec), `ExtractDAG` (tolerant `{goal,dag}` envelope parse), and `SeedUnits` — mints one `dagunit` entity per node (new `domain.EntityTypeDagUnit`/`DagUnitEntityID`), creates it via graph-ingest `entity.create` with `gateddag.depends_on` edges (node IDs → prereq unit IDs) + provenance (`dagunit.node_id/parent_quest/execution_id/sub_quest`). Model = **shared executor over distinct unit entities** (Coby): one long-lived executor over the `…dagunit.*` prefix, markers-only (no per-DAG FanOut instance), per-DAG isolation via the DAG-local edge set. **Uses semstreams `natsclient` throughout** (no raw NATS). Unit tests (validation + extraction) + integration test (`unit_test.go`): SeedUnits a diamond → real graph-ingest + gated-dag executor dispatch each unit exactly once in dependency order (a→{b,c}→d). Note for 3.2: the real dispatch handler must consume via the ADR-070 `natsclient` durable-consume primitive (survives restart); the executor also **requires** a wired `LifecycleManager` (Watch substrate). Bumped semstreams → beta.134 for ADR-070 durable dispatch (`b1722a6`).
- [x] 3.2 Implement the dispatch-subject handler mapping a gated-dag unit to `ClaimAndStartForParty` on the best-skill-matched member (lead excluded) — new `questdag` processor (`component.go`/`handler.go`/`config.go`/`register.go`, `ComponentName="questdag"`): owns one long-lived gated-DAG executor over `UnitPrefix` (markers-only) and subscribes the ADR-070 **`natsclient.ConsumeDurable`** durable at-least-once primitive on the dispatch subject. `handleDispatch` decodes `DispatchMessage.UnitEntityID`, reads the unit's `dagunit.sub_quest`/`node_id`, loads the sub-quest (idempotent skip when not `posted`), resolves the party from `subQuest.PartyID`, and `selectMember` picks the best skill-overlap available member (lead excluded, agent must be idle) → optional `AssignTask` + `ClaimAndStartForParty`. Ack/nak semantics: no-member → nak (backpressure), malformed/already-handled → ack (no poison loop). Narrow `QuestBoardRef`/`PartyCoordRef` ifaces + test overrides. Integration test (`dispatch_test.go`): real graph-ingest + executor dispatch two independent units (distinct skills) → each sub-quest claim-started on its skill-matching member, lead never assigned. Executor requires `deps.LifecycleManager`. Not registered in componentregistry yet (coexists with questdagexec until 3.6). `go build`/lint/unit green.
- [ ] 3.3 Port the lead review gate (review tool dispatch, accept/reject with required explanation, feedback into retry prompt) and clarification pause/resume
- [ ] 3.4 Port per-node retry budget, parent escalation on exhaustion (cancel remaining nodes), and topological rollup to parent output + party disband
- [ ] 3.5 Re-derive the `quest.dag.*` parent-quest projection from the gated-dag unit model (UI-facing shape preserved or mapped)
- [ ] 3.6 Port questdagexec tests to the new seam; delete the event loop, bootstrap protocol, and dedup caches
- [x] 3.7 Decide the partycoord boundary — retain it narrowly for lead/roster
  selection, membership policy, disband intent, authoritative Party KV
  form/join/disband/read behavior, and temporary `AssignTask`/`SubQuestMap`.
  Rules detect party-required work; questdag/gated-DAG own DAG execution.
  Delete the no-consumer event plane and decomposition/result/rollup shadows
  through issue #28; defer canonical Party ownership to issue #4 and
  SemStreams #313.
- [x] 3.8 Implement
  [semdragon #28](https://github.com/C360Studio/semdragon/issues/28): delete
  all eight raw party DTO/subject/publisher/Graphable families, thirteen
  event-only vocabulary entries, party event ports, two phantom dmsession
  party event values, four test-only coordination/rollup methods, and shadow
  fields across Go/API/schema/TypeScript/UI/tests/docs. Retain Party KV
  `EmitEntity`, form/join/disband/read behavior, KV-backed UI activity, and
  temporary `AssignTask`/`SubQuestMap`. Prove exactly ten audit findings
  disappear—nine event projections plus shadow-only `party.strategy`—while
  the four retained authoritative findings (`party.quest`, `party.lead`,
  `party.lifecycle.formed_at`, and `party.lifecycle.disbanded_at`) and hidden
  dynamic families remain deferred. Do not bind an owner, activate questdag,
  or add a replacement event plane.

  Completed evidence (2026-07-25): the deletion guard and pinned 4/8 audit
  pass, with hidden member and assignment families still deferred. Candidate
  emit-then-commit mutations are race-tested and roll back on emit failure.
  `go test ./...` passes. `go build ./...` exits 0 with only a non-fatal
  module stat-cache permission warning. `npm test -- --run` passes 342/342;
  `npm run check` reports 0 errors and 15 pre-existing warnings;
  `npm run build` and `task schema:check` pass. No owner or replacement raw
  Party event plane was introduced.

## 4. Stage B3 — mechanics disablement and operator surface

- [x] 4.1 Make `agentprogression`, `bossbattle`, `agentstore`,
  `boidengine`, and `redteam` inactive in all three shipped runtime profiles;
  registered components may remain configured with `enabled: false`. Enable
  the complete quest lifecycle rule pack in each profile and verify clean
  boot and quest flow without mechanics. Issue #26 owns the bossbattle slice,
  issue #30 owns agentprogression, issue #32 owns agentstore, and issue #34
  owns boidengine; issue #36 owns redteam.

  Completed 2026-07-25 evidence: all five mechanics components are inactive in
  the default, semsource, and tier-1 E2E profiles. The complete rule pack and
  mechanics-stub remain enabled, and explicit claim/start reaches
  `review-skipped-mechanics-disabled` completion without battle, progression,
  store, boid, redteam, Guild, or other mechanics side effects. The
  per-component evidence remains recorded in tasks 4.5 through 4.8 and the
  issue #26 evidence below. Reactivation migrations remain separate tasks and
  blockers; completing Stage B3 does not activate or migrate any mechanic.
- [x] 4.2 Add the stub rule: on transition into `in_review`, fire
  `lifecycle_transition` to `completed` with
  `verdict: review-skipped-mechanics-disabled` set atomically.

  Completed 2026-07-25 evidence: the shipped
  `config/rules/quest/04-mechanics-stub.json` is exercised through the real
  graph-ingest, lifecycle Manager, rule processor, and upstream lifecycle
  gateway. An authenticated operator transition from `in_progress` to
  `in_review` is followed by the rule transition to `completed`; the first
  observed completed revision contains the verdict, and ordered history
  attributes the two transitions to `operator` and `rule`. The existing
  mechanics-disabled integration proves the same first-completed-revision
  invariant and no battle, progression, store, redteam, Guild, or other
  mechanics side effect. Both focused integration tests and `go test ./...`
  pass.
- [x] 4.3 Enable `lifecycle-gateway` scoped to the quest workflow; verify
  inspect/history/operator-transition against the running stack.

  Completed 2026-07-25 evidence: both registry entry points register the
  upstream component, and every shipped profile enables the identical
  `/lifecycle-gateway/workflows` deployment with WebSockets disabled and a
  64 KiB body cap. One shared Manager registers only `quest`, making scope
  compositional. Path-boundary authentication reuses
  `SEMDRAGONS_API_KEY`, accepts `X-API-Key` and Bearer credentials, preserves
  empty-key development passthrough, and leaves unrelated routes untouched.
  The real component-stack integration verifies authenticated discovery,
  quest-only scope, inspect, state patch, body rejection, disabled streaming,
  operator transition, mechanics-stub completion, and ordered history.
  Static generation reuses and rebases the upstream provider at the deployed
  path with resolved request schemas. Focused component/auth/OpenAPI tests and
  both focused integrations pass, as do the full Go and UI gates and both
  strict OpenSpec validations. The tier-1 scenario uses the canonical returned
  entity ID and is retained under gate 5.2.

  Runtime-image evidence also closes Semdragon #40, #41, and #42: current
  SemSource runs as a complete external service on isolated JetStream rather
  than the removed `headless` mode, Compose waits through the observed
  86-second cold SemEmbed load, and the backend image packages all four quest
  rules at their profile-resolved paths. The focused Tier-1 Playwright spec
  passes 4/4 through the UI proxy, including rule completion and history.

  SemStreams #677 owns atomic transition fields, #678 owns workflow/operation
  policy, #679 owns runtime publication of resolved gateway paths, #680 owns
  OpenAPI security policy, #681 owns lifecycle-history retention, and #682
  owns the documented 413 versus observed 400 oversized-body response. None
  is reimplemented here; #680 and #682 do not block runtime authentication.
  The shipped stack's legacy history-10 bucket bootstrap makes current history
  verification pass, but it is not accepted as durable ownership in place of
  #681.
- [ ] 4.4 Delete `service/api` handlers superseded by the gateway (status mutation only; keep world/DM/workspace/creation endpoints)
- [x] 4.5 Resolve
  [semdragon #30](https://github.com/C360Studio/semdragon/issues/30): retain
  `agent_progression` registration but disable it in all three shipped
  profiles; preserve the complete mechanics-stub rule pack; delete only the
  no-publisher `AgentXPPayload`, false event output, and six dead vocabulary
  entries; and preserve the Agent/reconstruction, XP engine, validation, and
  dormant Phase-2 implementation. Retain the measured beta.158 audit delta
  from 22/32 to 17/26: five fewer invalid and six fewer extracted candidates.
  Make no agentstore, dependency, predicate rename, owner, lifecycle, or
  watcher change; keep reactivation blocked on SemStreams #313.

  Completed evidence (2026-07-25): all three shipped profiles disable
  `agent_progression`. Completion through the shipped mechanics stub and a
  direct valid lifecycle transition to terminal `failed` both preserve the
  seeded Agent and Guild KV revisions and bytes and produce no mechanics side
  effects. The pinned beta.158 audit moves from 22/32 to 17/26. Focused unit,
  race, and integration tests; the full Go test and build; schema freshness;
  strict OpenSpec validation; and diff checks pass. Architect and independent
  Go reviewer approve the slice.
- [x] 4.6 Resolve
  [semdragon #32](https://github.com/C360Studio/semdragon/issues/32): retain
  `agent_store` registration but disable it in every shipped profile; preserve
  the complete mechanics-stub rule pack, product policy/models, dormant code,
  REST/OpenAPI, API 503 behavior, and autonomy's unavailable-store no-op.
  Delete the five publisher-only payload/subject/publisher families, seven
  raw-event vocabulary entries, and false output while retaining EntityState
  KV categories. Verify the pinned audit moves from 23/30 to 14/18 by deleting
  payloads.go's 9/12 corpus and retaining store.go's 14/18 corpus. Make no
  agentprogression, dependency, retained-predicate, owner, lifecycle,
  graph-view, API, or UI change. Keep Phase-2 reactivation blocked on
  SemStreams #313 and add a linked owned-create issue only if later required.
  Record, but do not implement, the future graphview reads, idempotent durable
  agentprogression XP-debit command, prohibition on full Agent replacement,
  and exclusion of Agent lifecycle/status from agentstore authority.

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
- [x] 4.7 Resolve
  [semdragon #34](https://github.com/C360Studio/semdragon/issues/34): retain
  boidengine registration and configuration but disable it in all three
  shipped profiles. Preserve `DefaultBoidEngine`, rules, attraction/scoring
  policy, DTOs, dormant `boid.suggestions.>` / `boid.guild.>` contracts,
  `BOID_SUGGESTIONS`, and dmpartyformation's direct policy reuse. Remove only
  the unproduced shipped `agent.boid.>` stream entries. Prove ComponentManager
  creates no boidengine instance or suggestion bucket, autonomy causes no
  claim/join/form mutation without the producer, and seeded Agent, Quest, and
  Guild state remains bit-for-bit unchanged until an explicit claim. Prove
  REST API or test-fixture claim/start reaches questbridge and mechanics-stub
  completion without mechanics side effects; the separately authenticated
  lifecycle operator joins this path through completed task 4.3. Keep the pinned
  beta.158 audit at zero structured candidates. Reclassify positive boid E2E
  coverage as dormant/opt-in. Record, but do not implement, the future
  `pkg/graphview`-backed reads and durable KV/stigmergic suggestion projection.
  Add no fallback recommender, local projection framework, dependency bump,
  ownership, API/schema change, subject/KV redesign, or upstream issue;
  SemStreams #587 already owns message-logger per-client KV-watch migration.

  Completed 2026-07-25 evidence, closing only task 4.7:

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
- [x] 4.8 Resolve
  [semdragon #36](https://github.com/C360Studio/semdragon/issues/36): retain
  redteam registration/configuration with `enabled: false` in all three
  shipped profiles; remove only its two false OutputPorts; and retain its
  category/vocabulary taxonomy, Quest/API/UI fields, findings/lesson policy,
  prompts, and opt-in E2E. Prove ComponentManager creates no redteam instance,
  watcher, pending review, or deadline and that explicit claim/start reaches
  questbridge and mechanics-stub completion with no red-team Quest, status,
  Guild lesson, or event side effect. Document without fixing the legacy
  underscore watcher, raw caches, unsafe full-object writers, inert claim
  timeout, and local deadlines. Keep reactivation blocked on Semdragon
  #4/#5/#22 and SemStreams #313/#676; make no dependency, graphview,
  ownership, writer, lifecycle, rule, API/UI/schema, or durable-deadline
  migration.

  Completed 2026-07-25 evidence, closing only task 4.8:

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

### Issue #26 Stage B3 slice evidence

- Every shipped profile omits bossbattle and uses the exact enabled
  `rule-processor`, ordered four-file quest pack, and quest entity watch.
- The fresh-graph integration path completes review-required work with
  `review-skipped-mechanics-disabled` and produces no battle or mechanics side
  effect.
- The false `battle.review.*` event facade and retreat surface are deleted;
  the authoritative model remains dormant for the owner-safe Phase-2 rebuild.
- Go, UI, generated-schema, and strict OpenSpec checks pass under architect,
  Go, and Svelte review.

This evidence completes only the bossbattle slice. Task 4.1 is now complete on
the accumulated issue #26/#30/#32/#34/#36 evidence that all five mechanics
components are inactive in every shipped profile. Tasks 4.2 and 4.3 are
completed by issue #38 evidence above. Task 4.4 and the remaining lifecycle
gates stay open.

## 5. Verification gate

- [ ] 5.1 `task test` and `task test:integration` fully green
- [ ] 5.2 Full mock-LLM e2e (`task e2e`) green, including party DAG scenarios
- [ ] 5.3 Dashboard assertions: quest status progression and DAG node states render unchanged (UI predicate stability requirement)
- [ ] 5.4 Update `docs/03-QUESTS.md` lifecycle/DAG sections to point at the new specs; run `openspec validate`; change ready for archive
