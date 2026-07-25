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
`boidengine`, `redteam`) are removed from `config/semdragons.json` (code
retained). One stub rule handles the review path: on transition into
`in_review`, fire `lifecycle_transition` to `completed` with a
`verdict: review-skipped-mechanics-disabled` field set atomically. This keeps
the `in_review` phase reachable (so Phase 2 re-enables review by replacing one
rule) rather than special-casing submission routing.

### D7: Adopt lifecycle-gateway for workflow operations only
Enable the framework `lifecycle-gateway` for quest workflow inspect/history/
transition (satisfies the operator-surface requirement). Product endpoints
(world state, DM chat, workspace, quest creation with brief validation) stay
in `service/api`. Bespoke handlers that merely mutate quest status are
deleted in favor of the gateway.

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
5. **Stage B3** — mechanics-disable config + stub rule; enable
   lifecycle-gateway; delete superseded `service/api` handlers.
6. **Gate** — full mock e2e (`task e2e`); dashboard progression assertions.
7. **Rollback** — revert the branch. No production deployments exist; no data
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
- Is `partycoord` still needed as a component, or does party formation reduce
  to rules + the questdag layer? (Resolve in Stage B2; out of scope to
  redesign if it still works.)
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
