# Design: rebuild-quest-lifecycle-on-semstreams-rules

## Context

Semdragons began this change on semstreams `beta.21` and hand-implements two state machines the
framework now provides: the quest lifecycle (`processor/questboard`, imperative
transition handlers over KV) and party DAG execution (`processor/questdagexec`,
a single-goroutine event loop with bespoke bootstrap/recovery). Upstream now
ships `pkg/lifecycle` (declared workflows: transition tables, audit, operator
gateway), rule-engine lifecycle actions, and `processor/gated-dag`
(dependency-ordered, exactly-once, restart-recoverable dispatch). The 2026-07
audits established that the incentive mechanics will be rebuilt separately
(two-loop principle), so this change deliberately migrates the *skeleton* with
mechanics disabled. Verification is mock-LLM only.

## Goals / Non-Goals

**Goals:**
- Semdragons runs on the approved current semstreams target (beta.158) with all bespoke lifecycle
  and DAG-ordering code replaced by framework primitives.
- Quest lifecycle behavior matches the `quest-lifecycle` spec; DAG behavior
  matches the `dag-execution` spec.
- Mock-LLM e2e suite passes end-to-end; dashboard renders unchanged.

**Non-Goals:**
- Any incentive mechanic (XP, boss battles, reputation, store, boids
  suggestions) — disabled, rebuilt in the Phase 2 change.
- The dev-substrate extraction (sandbox service, environment manifests) —
  separate change.
- Real-LLM validation, prompt tuning, or model registry changes beyond what
  compilation requires.

## Decisions

### D1: Contract-gated dependency and behavior stages
Stage A upgraded the dependency through beta.135 and adapted existing components
until the current test suite is green again (wire contracts, payload registry,
graph-write/ownership semantics). Stage B then performs the architectural
swaps. The beta.158 pre-v1 contract is owned by the prerequisite
`adopt-semstreams-prev1-contract` change and must land before the final
dependency cutover. Alternative — swapping architectures during a breaking
contract bump — was rejected:
with 111 releases of drift, we need to be able to attribute breakage to
"framework moved" vs "we re-architected". Stage A's green suite is the
bisection point.

### D2: Rules own transitions; questbridge keeps context assembly
The lifecycle edges become rule-engine conditions firing
`lifecycle_transition`/`lifecycle_complete`/`lifecycle_fail` (per the
quest-lifecycle spec). But quest dispatch to the agentic loop stays in
`questbridge`: it performs context assembly (prompt fragments, entity
knowledge, peer feedback, dependency outputs, context-budget accounting) that
the rule engine's `publish_agent` per-dispatch config cannot express today.
questbridge watches the lifecycle phase predicate instead of raw status
writes. Alternative — moving dispatch to `publish_agent` — is deferred, not
rejected: revisit when prompt assembly can be expressed as rule/fragment
config; migrating dispatch mechanics and context assembly simultaneously
doubles the risk surface.

### D3: Keep the existing phase predicate and values
The workflow's `PhasePredicate` is the current `quest.status` predicate with
unchanged phase strings. This satisfies the UI-stability requirement without a
mapping layer, and keeps KV-watch consumers (UI SSE, dmworldstate) working
untouched. Alternative — adopting a framework-conventional predicate name —
was rejected for this change; it buys nothing and breaks every consumer.

### D4: Quest workflow declaration shape
One `lifecycle.Workflow` for quests: schema = a `QuestState` struct derived
from the existing quest entity fields with `lifecycle` struct tags (default-
deny operator writes; triage-related fields operator-writable). Terminal
phases: `completed`, `failed`, `escalated`. `pending_triage` is non-terminal
(DM can repost). Rule files live under `config/rules/quest/` (JSON, one file
per lifecycle concern: claim/start, submission, failure-retry, triage,
mechanics-stub), hot-reloadable via the rule engine's KV config path.

### D5: gated-dag boundary — framework orders, semdragons means
`processor/gated-dag` owns dependency ordering, exactly-once claim-before-
dispatch, restart recovery, and stall detection. Semdragons keeps a thin
`questdag` domain layer: DAG extraction/validation from the lead's tool
output, minting unit IDs and seeding unit entities with `depends_on` edges,
the dispatch-subject handler that maps a unit to `ClaimAndStartForParty`, the
lead review gate (tool dispatch + accept/reject handling), clarification
routing, parent escalation policy, and rollup. The current
`questdagexec` event loop, its bootstrap protocol, and its dedup caches are
deleted. `quest.dag.*` predicates on the parent quest remain the UI-facing
projection, re-derived from the gated-dag unit model.

### D6: Mechanics disablement is config + one stub rule
Mechanics processors (`agentprogression`, `bossbattle`, `agentstore`,
`boidengine`, `redteam`) are inactive in every shipped
`config/semdragons*.json` runtime profile. Components may remain registered
and configured with `enabled: false`; registration and dormant configuration
do not instantiate them. Each profile enables the complete quest lifecycle
rule pack and watches quest entities. One stub rule handles the review path:
on transition into `in_review`, fire
`lifecycle_transition` to `completed` with a
`verdict: review-skipped-mechanics-disabled` field set atomically. This keeps
the `in_review` phase reachable (so Phase 2 re-enables review by replacing one
rule) rather than special-casing submission routing.

Bossbattle requires an explicit ordering constraint. Before issue #26, it
watched the same `in_review` transition as the stub, so enabling both permitted
the stub to complete a quest while bossbattle created a battle and later
replaced foreign Quest, Agent, and PeerReview state. All shipped profiles now
disable bossbattle before the mechanics-stub behavior is accepted, and
verification proves that no battle entity or mechanics mutation is produced.

Issue #26 retains the dormant authoritative `BossBattle` model, evaluator,
factory registration, REST/dmworldstate/UI readers, battle-state KV port, and
typed quest/agent relationships. It deletes only the unproduced
`battle.review.*` event facade and retreat-only surface. The separate
`adopt-semstreams-prev1-contract` D16 records the Phase-2 canonical projection,
cross-owner handoffs, SemStreams #313 dependency, and reactivation block.

The reviewed issue #26 slice proves the three-profile configuration contract,
fresh-graph review-skipped transition, and absence of battle/mechanics side
effects. This closes only the bossbattle portion of Stage B3. The other four
mechanics processors and the global lifecycle gates remain open.

Issue #30 applies the same quarantine boundary to agentprogression. All three
profiles retain the registered `agent_progression` entry with `enabled:
false`; the complete rule pack and mechanics stub remain unchanged. The
no-publisher `AgentXPPayload`, false `agent-xp`/`AGENT_EVENTS` output, and six
event-only vocabulary entries are deleted. The Agent model and reconstruction,
XP engine and validation, component registration/configuration, handlers, and
watchers remain dormant for Phase 2.

Issue #30 deliberately performs no agentstore, dependency, predicate rename,
owner, lifecycle, or watcher migration. The historical beta.158 audit baseline
is 22 invalid candidates from 32 extracted. The pinned post-change audit is 17
invalid from 26 extracted, exactly five fewer invalid and six fewer extracted
candidates. The retained authoritative writer cannot reactivate until
beta.158 and SemStreams #313 provide the approved owner-token mutation
boundary.

Issue #32 applies the quarantine boundary to agentstore. The default profile
already disables `agent_store`; the semsource and tier-1 E2E profiles now do
the same. Store routes, OpenAPI, models, catalog/inventory/effect policy,
component registration, and dormant code remain. Store API calls return their
existing 503 response while unavailable, and autonomy treats the missing store
as a no-op.

The five publisher-only raw store payload families, their subjects and
publishers, all seven raw-event vocabulary entries, and the false component
output are deleted. EntityState KV message categories remain distinct and
unchanged. The pinned beta.158 audit moves from the historical 23/30 baseline
to 14/18: exactly the payloads.go 9/12 plane is removed and the store.go 14/18
corpus remains.

Issue #32 performs no agentprogression, dependency, retained-predicate, owner,
lifecycle, graph-view, API, or UI migration. Reactivation remains blocked on
SemStreams #313. A linked owned-create issue is opened later only if #313 does
not supply the first-class inventory/effect birth operation actually required
by Phase 2.

The later migration replaces agentstore's `sync.Map` current-state reads with
SemStreams graphview projections. It sends an idempotent, durable XP-debit
command/request to the agentprogression owner after #313 instead of directly
debiting XP or replacing Agent. Agent lifecycle and status remain outside
agentstore authority. These are Phase-2 handoffs, not issue #32 production
changes.

Issue #34 applies the quarantine boundary to boidengine. All three shipped
profiles retain its registration and complete configuration but set
`enabled: false`. This is an explicit behavior change: boidengine is the sole
producer of `boid.suggestions.>` and `boid.guild.>`, so shipped Phase 1 has no
autonomous quest claim, guild join, or guild formation recommendation.
Posting a quest does not advance it. The supported shipped Phase-1 path is
explicit REST API or test-fixture claim and start, followed by questbridge
execution and mechanics-stub completion. The separately authenticated
lifecycle operator also joins that path through completed task 4.3.

The product surface remains dormant rather than being deleted:
`DefaultBoidEngine`, configurable rules, attraction/scoring policy, suggestion
DTOs, the two live advisory subject contracts, `BOID_SUGGESTIONS`, and direct
policy reuse by dmpartyformation. Positive boid E2E coverage becomes opt-in
Phase-2 coverage instead of a shipped-runtime assertion. The shipped
`agent.boid.>` stream entries are removed because no producer exists and they
belong to the retired SemStreams boid design.

Reactivation first replaces boidengine's three raw ENTITY_STATES watchers and
process-local caches with SemStreams `pkg/graphview`. Suggestions become a
durable KV/stigmergic projection, and consumers migrate to that state before
the duplicate core-NATS advisory plane can be retired. This quarantine adds no
fallback scorer, rules-based recommender, local projection framework,
ownership change, dependency bump, or API/schema change. SemStreams #587
already covers message-logger per-client KV-watch migration; issue #34 opens
no duplicate upstream issue.

### D7: Adopt lifecycle-gateway as an authenticated operator surface

Enable the framework `lifecycle-gateway` at the service-manager-resolved path
`/lifecycle-gateway/workflows`. The gateway exposes workflow discovery, list,
inspect, history, children, operator state patch, and table-valid transition
operations. WebSocket streaming is explicitly disabled and request bodies are
bounded to 64 KiB.

The complete namespace and all descendants use the existing
`SEMDRAGONS_API_KEY` policy through SemStreams
`Manager.UseHTTPMiddleware`: `X-API-Key` and Bearer credentials are accepted,
while an empty configured key preserves the existing development passthrough.
Prefix matching is path-segment aware so unrelated routes remain under their
existing policies.

The shared lifecycle manager registers only the `quest` workflow. This makes
the deployed gateway quest-only by composition, not by a downstream
allowlist. SemStreams
[#678](https://github.com/C360Studio/semstreams/issues/678) owns the missing
workflow and operation policy primitive; Semdragon does not create one.

The gateway is an operator and break-glass surface. Existing
`/game/quests/{id}/...` command handlers remain the supported product surface
and continue to own domain validation and transition-coupled writes.
SemStreams [#677](https://github.com/C360Studio/semstreams/issues/677) owns
atomic validated fields on operator transitions. Until that primitive can
preserve product invariants, task 4.4 must not delete or redirect those
handlers.

Lifecycle history depends on retained `ENTITY_STATES` KV revisions.
Semdragon's legacy board-bucket bootstrap currently precreates that bucket
with history 10, so the shipped stack preserves the tested operator and rule
events. That bootstrap is not durable framework ownership:
[SemStreams #681](https://github.com/C360Studio/semstreams/issues/681) requires
graph-ingest and lifecycle consumers to share and validate one retention
invariant before the bespoke bootstrap can retire.

The configured body cap is enforced before mutation, but beta.135 and beta.158
return 400 for an oversized body despite advertising 413. The integration test
asserts the observed 400 and links
[SemStreams #682](https://github.com/C360Studio/semstreams/issues/682);
Semdragon does not wrap the upstream handlers to translate it.

Static OpenAPI generation may reuse and rebase the upstream gateway provider;
it must not restate lifecycle-gateway operations. The live SemStreams manager
does not yet publish resolved component-gateway paths in `/openapi.json`;
SemStreams [#679](https://github.com/C360Studio/semstreams/issues/679) owns
that framework defect. The framework OpenAPI model also cannot express the
API-key/Bearer policy supplied through `UseHTTPMiddleware`; SemStreams
[#680](https://github.com/C360Studio/semstreams/issues/680) owns security
schemes and operation/document security requirements. Neither gap blocks the
runtime authentication in this packet, and Semdragon will not add a parallel
runtime merger or restate upstream operations.

Product endpoints for world state, DM chat, workspace, quest creation, and
quest commands stay in `service/api`. Task 4.4 remains a separate retirement
gate.

The shipped backend image must copy the configured quest rule tree and run
from the directory against which the profiles resolve `config/rules/...`.
Repository-only integration is insufficient evidence because a rule processor
with no packaged rules cannot complete an operator transition. Semdragon
[#42](https://github.com/C360Studio/semdragon/issues/42) owns this packaging
contract.

### D8: Partycoord remains policy and Party KV authority, not a second DAG

The Stage B2 decision is to retain partycoord narrowly. Rules can detect a
party-required quest and gated-DAG can order units, but neither owns lead
selection, roster composition, membership policy, or disband intent.
Partycoord therefore retains core formation/join/disband behavior,
authoritative `Party` writes through `GraphClient.EmitEntity`, Party readers,
and the temporary `AssignTask`/`SubQuestMap` advisory used by both
`questdagexec` and `questdag`.

Issue [#28](https://github.com/C360Studio/semdragon/issues/28) deletes the
separate surfaces that have no surviving product authority:

- all eight raw party DTO/typed-subject/publisher/Graphable families;
- all thirteen event-only vocabulary entries, party event ports, and the two
  phantom dmsession party event values;
- the test-only `DecomposeQuest`, `SubmitResult`, `StartRollup`, and
  `CompleteRollup` methods; and
- `Strategy`, `SharedContext`/`ContextItem`, `SubResults`, and `RollupResult`
  across Go, API/schema, TypeScript, UI, fixtures, and documentation.

The raw subjects have no production subscriber, payload factory,
`BaseMessage`, graph-ingest route, replay path, API reader, or UI reader.
Party UI activity instead comes from the retained KV `EntityState`
message categories `party.formed`, `party.joined`, and `party.disbanded`.
Questdag and quest lifecycle own decomposition output, node results, retry,
clarification, escalation, and rollup.

This decision is not the canonical Party migration. Deleting the event
Graphables removes nine of partycoord's fourteen beta.158 audit findings, and
deleting shadow-only `Party.Strategy` removes one more. Four authoritative
Party findings remain: `party.quest`, `party.lead`,
`party.lifecycle.formed_at`, and `party.lifecycle.disbanded_at`.
Scanner-hidden dynamic member/assignment families and the UI
dynamic-assignment parser trap also remain explicitly deferred. Issue #4 later
creates first-class membership and assignment entities, adopts the beta.158
owner contract, and uses the public owner-token mutation path tracked by
SemStreams #313. Issue #28 binds no owner, copies no framework mutator,
activates no questdag replacement, and adds no compatibility event plane.

### D9: Redteam is disabled; its Phase-2 repair is not pulled forward

Issue [#36](https://github.com/C360Studio/semdragon/issues/36) closes the
redteam portion of Stage B3 by retaining registration/configuration with
`enabled: false` in all three shipped profiles. No ComponentManager instance,
watcher, pending-review cache, or local deadline is created. Explicit REST API
or test-fixture claim/start continues through questbridge and mechanics-stub
completion with no red-team Quest, status, Guild lesson, or event side effect.
Positive red-team E2E remains opt-in Phase-2 coverage, not a shipped-runtime
assertion.

The two advertised redteam OutputPorts are false because the component has no
NATS publisher, raw event DTO, or shipped stream subject, so issue #36 removes
only those ports. The `redteam.lifecycle.*` category/vocabulary values and
Quest/API/UI/finding/lesson/prompt contracts remain dormant product surfaces.

This slice deliberately leaves the retained implementation defects inactive:
the watcher still reads legacy
`quest.classification.red_team_target`, transition and pending state remain
in process-local caches, full-Quest and full-Guild writers remain unsafe
across predicate owners, `ClaimTimeoutSec` remains unenforced, and execution
deadlines remain restart-fragile local goroutines. Reactivation is blocked on
Semdragon #4, #5, and #22 plus SemStreams #313 and #676. No dependency,
graphview, ownership, writer, lifecycle, rule, schema, API/UI, or durable
deadline migration belongs to issue #36.

## Risks / Trade-offs

- [Graph-write/ownership semantics changed upstream (ADR-054/055/056)] →
  Stage A starts with a read-through of `migration-*.md` docs and a spike
  writing/reading one quest entity under beta.132 ownership rules before
  touching components.
- [Agentic-loop wire contract drift (ADR-053 agentrun, message shapes)] →
  contract-level integration test for questbridge↔agentic-loop and
  questtools↔tool-execute round trips, written in Stage A before the swap.
- [QUEST_LOOPS crash-recovery bucket may be redundant with framework agentrun
  lifecycle] → evaluate in Stage A; keep QUEST_LOOPS if agentrun doesn't cover
  quest→loop mapping; do not migrate both at once.
- [UI predicate drift sneaking through] → mock e2e asserts dashboard status
  progression (already covered by tier-1 specs); D3 makes this low-likelihood.
- [Rule-engine expressiveness gaps for an edge case (e.g. attempt-conditional
  repost-vs-triage routing)] → lifecycle Manager API calls from a small
  component remain a legitimate escape hatch; the spec requires rule-driven
  *or* manager-API transitions, not rules exclusively.
- [Trade-off: keeping mechanics code dormant instead of deleting] → accepted;
  Phase 2 rewrites it against the two-loop principle and deletes what it
  doesn't reuse. Deleting now would churn the diff twice.

## Migration Plan

1. **Stage A** — branch; bump through beta.135; walk migration docs; fix payload
   registry/ownership/wire-contract breakage until `task test` and
   `task test:integration` are green with the *existing* architecture.
2. **Pre-v1 contract gate** — complete `adopt-semstreams-prev1-contract`, then
   adopt beta.158 with zero predicate/entity/reference findings.
3. **Stage B1** — declare the quest Workflow + rules; run questboard's
   transition logic and the lifecycle workflow side by side behind a config
   flag only long enough to port tests; then delete questboard's bespoke
   transition handlers.
4. **Stage B2** — introduce the `questdag` domain layer on gated-dag; delete
   the questdagexec event loop; port its tests to the new seam.
5. **Stage B2 party boundary** — implement issue #28's deletion-only narrowing
   while retaining Party KV behavior and the temporary assignment advisory.
6. **Stage B3** — mechanics-disable every shipped config + complete rule pack;
   prove the bossbattle/stub race is absent; enable lifecycle-gateway; delete
   superseded `service/api` handlers.
7. **Gate** — full mock e2e (`task e2e`); dashboard progression assertions.
8. **Rollback** — revert the branch. No production deployments exist; no data
   migration is performed (dev boards are recreated from scratch).

## Open Questions

- ~~Does framework agentrun (upstream ADR-053) subsume the QUEST_LOOPS
  quest→loop mapping, or do we keep the bucket?~~ **RESOLVED (Stage A, task 1.6):
  keep QUEST_LOOPS; do not adopt agentrun in this change.** No subsumption —
  `agentrun.AgentRun` is a per-*run* Lifecycle Participant keyed by a 6-part
  chain-execution entity ID (carries `Phase`/`ParentRunEntityID`/milestones),
  whereas QUEST_LOOPS is a per-*loop* correlation map carrying `AgentID`,
  `TrustTier`, `SandboxDir`, `LoopType`, `StartedAt` and the quest↔loop reverse
  lookup. Different key, cardinality, and payload. The beta.132 agentic wire is
  additive-only (`TaskMessage.RunID` etc. are `omitempty`), so QUEST_LOOPS works
  unchanged with no pressure from the bump; agentrun is opt-in and heavy (requires
  wiring the ADR-047 `lifecycle.Manager` into `cmd/semdragons` + rule-engine
  `RunScope`), and ADR-053 §Deferred explicitly lists "legacy migration of …
  semdragon" as out of scope. Future coexistence path (separate change): set
  `TaskMessage.RunID` on sub-quest spawns + run an `agentrun.MilestoneSubscriber`
  while keeping QUEST_LOOPS for per-loop bookkeeping — the two are non-conflicting
  because `RunID` is `omitempty`. (Evidence: `research/02-agentic-wire.md`.)
- ~~Is `partycoord` still needed as a component, or does party formation
  reduce to rules + the questdag layer?~~ **RESOLVED (Stage B2 design, task
  3.7): retain it narrowly for formation/roster/membership/disband policy and
  authoritative Party KV state. Issue #28 deletes its event plane and
  decomposition/result/rollup shadows; canonical beta.158 ownership remains
  deferred to issue #4 and SemStreams #313.**
- ~~Which predicates on the quest entity need explicit ownership claims under
  ADR-056 so rules and questbridge don't fight?~~ **RESOLVED (Stage A spike, task
  1.3):** three prefix-disjoint owned groups — lifecycle owner claims
  `quest.status.*`/`quest.lifecycle.*`/`quest.attempts.*`/`quest.failure.*`/
  `quest.assignment.*`/`quest.verdict.*`/`quest.data.output` (phase = `quest.status.state`,
  `cas-transition`); questbridge claims `quest.execution.*`/`quest.context.*`/
  `quest.loop.*`/`quest.metrics.*` (`replace-owned`, must switch off full-object `Put` to
  `update_with_triples`); questdag claims `quest.dag.*`. Creation-time predicates
  (`quest.identity/spec/difficulty/...`) need no claim. beta.132 enforcement is observe-only
  so the existing path is unaffected in Stage A. Full map + registration order in
  `research/05-ownership-spike.md`.
- `publish_agent` adoption timing for dispatch — revisit after Phase 2 when
  prompt assembly requirements are settled (deferred per D2).
