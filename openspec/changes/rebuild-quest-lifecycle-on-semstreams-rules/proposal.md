# Proposal: rebuild-quest-lifecycle-on-semstreams-rules

## Why

Semdragons began this change on semstreams `v1.0.0-beta.21` and hand-rolls
state machines the framework now provides as first-class primitives
(`pkg/lifecycle` workflows, rule-engine lifecycle actions, `processor/gated-dag`).
The 2026-07-04 revival audit concluded the product concept is worth reviving but
the incentive layer must be rebuilt against the two-loop principle; that rebuild
needs a current platform underneath it first. This change is Phase 1 of the
revival: migrate the platform with game mechanics disabled, verified entirely
against the mock LLM (zero paid tokens).

## What Changes

- Upgrade `github.com/c360studio/semstreams` from `v1.0.0-beta.21` through the
  latest approved target, currently `v1.0.0-beta.159`. The beta.159 adoption is
  blocked on `adopt-semstreams-prev1-contract`, which canonicalizes predicates,
  entity IDs/references, and writer ownership before the dependency cutover.
- **BREAKING (internal)**: replace `processor/questboard`'s hand-rolled quest
  state machine with a declared `pkg/lifecycle` `Workflow` (transition table,
  phase predicate, audit triples) plus rule-engine
  `lifecycle_transition`/`lifecycle_complete`/`lifecycle_fail` actions for each
  edge. Attempt counters move to atomic `TransitionWith` set-maps.
- **BREAKING (internal)**: replace `processor/questdagexec`'s single-goroutine
  event loop with `processor/gated-dag` dispatch. Semdragons keeps only domain
  code: DAG-node → sub-quest mapping, the lead review gate, and escalation
  policy. The `quest.dag.*` predicate layout is re-derived from the gated-dag
  unit/edge model.
- Resolve partycoord's Stage B2 boundary through issue #28: retain Party
  formation, roster policy, membership, authoritative KV
  form/join/disband/read behavior, and the temporary `AssignTask` advisory;
  delete its eight no-consumer raw event families and test-only
  decomposition/result/rollup shadow. Canonical Party ownership remains in
  the beta.159 adoption, not this lifecycle packet.
- Adapt `questbridge`/`questtools` to the beta.132 agentic-loop contracts;
  evaluate (in design) whether dispatch-on-`in_progress` moves to a rule-engine
  `publish_agent` action or stays in `questbridge`.
- Disable game mechanics for the duration of this change: `agentprogression`,
  `bossbattle`, `agentstore`, `boidengine`, and `redteam` are inactive in every
  shipped runtime profile. Every shipped profile loads the complete quest
  lifecycle rule pack, including the mechanics-stub rule, so explicitly
  claimed and started quests complete without XP, review, battle, reputation,
  inventory, or store side effects. The processors are rebuilt against the
  two-loop principle in a subsequent change; no incentive-mechanic behavior is
  in scope here.
- Resolve the agentprogression slice through issue #30: keep
  `agent_progression` registered but disabled in all three shipped profiles;
  delete its no-publisher `AgentXPPayload`, false output port, and six dead
  vocabulary entries; and retain the Agent/reconstruction, XP engine,
  validation, rules, and dormant Phase-2 implementation.
- Resolve the agentstore slice through issue #32: keep `agent_store`
  registered but disabled in all three shipped profiles; delete its five
  publisher-only raw payload/subject/publisher families, seven event-only
  vocabulary entries, and false output; and retain store policy, models,
  REST/OpenAPI, and the dormant Phase-2 implementation.
- Resolve the boidengine slice through issue #34: keep `boidengine` registered
  and configured but disabled in all three shipped profiles. Preserve the
  scoring policy, DTOs, `boid.suggestions.>` / `boid.guild.>` contracts,
  `BOID_SUGGESTIONS`, and direct dmpartyformation policy reuse. Shipped Phase 1
  has no autonomous claim or guild selection; REST API and test-fixture
  claim/start are the supported shipped paths. The separately authenticated
  lifecycle operator also joins that path through completed task 4.3.
- Resolve the redteam slice through issue #36: keep `redteam` registered and
  configured but disabled in all three shipped profiles. Remove its two false
  OutputPorts while retaining category/vocabulary taxonomy, Quest/API/UI
  fields, findings/lesson policy, prompts, and opt-in E2E. The shipped path is
  explicit claim/start through questbridge and mechanics-stub with no
  red-team side effect.
- Adopt the framework `lifecycle-gateway` at
  `/lifecycle-gateway/workflows` as an authenticated operator and break-glass
  surface for quest workflow discovery, inspection, history, children, state
  patching, and table-valid transitions. The complete namespace uses the
  existing `SEMDRAGONS_API_KEY` policy, WebSocket streaming is disabled, and
  quest-only scope is compositional because `quest` is the sole registered
  workflow. Product command handlers continue to own their coupled writes
  until task 4.4 can retire them safely.
- Verification gate: the mock-LLM e2e suite (`task e2e`) passes end-to-end on
  the migrated stack. Real-LLM tiers are explicitly out of scope.

## Capabilities

### New Capabilities

- `quest-lifecycle`: the quest state machine as a declared lifecycle workflow —
  phases (posted → claimed → in_progress → in_review → completed / failed /
  escalated / pending_triage), valid transitions, retry/triage semantics,
  transition audit history, and the side-effect contract (what fires on each
  edge) with mechanics side effects explicitly stubbed in this change.
- `dag-execution`: party sub-quest DAG execution on gated-dag — dependency
  ordering, exactly-once claim-before-dispatch, per-node retry budget, lead
  review gate as the acceptance step, parent-quest escalation on node
  exhaustion, and rollup.

### Modified Capabilities

<!-- none — openspec/specs/ is empty; this change seeds the first baseline specs -->

## Impact

- **Code**: `processor/questboard`, `processor/questdagexec` (largely deleted),
  `processor/questbridge`, `processor/questtools`, `componentregistry/`,
  `cmd/semdragons`, and all three shipped `config/semdragons*.json` profiles.
  Issue #28 also narrows `processor/partycoord`, `processor/dmpartyformation`,
  party event vocabulary/DM filters, and shadow-field API/UI consumers while
  retaining Party KV readers and writes.
  Issue #26 config-gates bossbattle and removes its unproduced raw-event and
  retreat surfaces while retaining its dormant authoritative model for Phase
  2. Issue #30 config-gates agentprogression without changing agentstore,
  SemStreams, predicates, ownership, lifecycle, or watcher behavior.
  Issue #32 config-gates agentstore and removes only its no-consumer raw event
  plane. Issue #34 config-gates boidengine while retaining its live dormant
  contracts and making explicit claim/start the supported Phase-1 path.
  Issue #36 config-gates redteam and removes only its false OutputPorts while
  retaining its dormant category/vocabulary and product contracts. Its
  watcher/cache, writers, and timeout redesign remain deferred.
- **Dependencies**: the beta.21 → beta.135 foundation is implemented on this
  branch. The beta.159 pre-v1 clean break is a separate successor change:
  `adopt-semstreams-prev1-contract`. No dependency-only bump is permitted.
- **Upstream lifecycle gaps**: SemStreams
  [#677](https://github.com/C360Studio/semstreams/issues/677) tracks atomic
  validated transition fields, [#678](https://github.com/C360Studio/semstreams/issues/678)
  tracks workflow and operation allowlists, and
  [#679](https://github.com/C360Studio/semstreams/issues/679) tracks runtime
  publication of resolved managed-gateway OpenAPI paths.
  [#680](https://github.com/C360Studio/semstreams/issues/680) tracks OpenAPI
  security schemes and operation/document security policy.
  [#681](https://github.com/C360Studio/semstreams/issues/681) tracks the
  framework-owned revision retention required for truthful lifecycle history,
  and [#682](https://github.com/C360Studio/semstreams/issues/682) tracks the
  documented 413 versus observed 400 oversized-body response. Semdragon does
  not fork these primitives; #680 and #682 do not block authenticated runtime
  activation, while #681 is critical framework ownership debt and task 4.4
  remains open because product command handlers require coupled writes.
- **Data/predicates**: quest entity predicates consumed by the UI
  (`worldStore`) must remain stable or ship with a documented mapping;
  `quest.dag.*` layout may change with the gated-dag model.
- **Tests/e2e**: unit + integration suites migrate with the components; mock
  e2e is the completion gate. Known pre-existing flakes (settings.spec tier1)
  are not in scope.
- **Docs**: `docs/03-QUESTS.md` lifecycle/DAG sections are superseded by the
  `quest-lifecycle` and `dag-execution` specs seeded by this change; ADRs
  002/003 remain as history.
