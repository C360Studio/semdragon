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
  latest approved target, currently `v1.0.0-beta.158`. The beta.158 adoption is
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
  the beta.158 adoption, not this lifecycle packet.
- Adapt `questbridge`/`questtools` to the beta.132 agentic-loop contracts;
  evaluate (in design) whether dispatch-on-`in_progress` moves to a rule-engine
  `publish_agent` action or stays in `questbridge`.
- Disable game mechanics for the duration of this change: `agentprogression`,
  `bossbattle`, `agentstore`, `boidengine`, and `redteam` are removed from
  every shipped runtime profile or stubbed to pass-through. Every shipped
  profile loads the complete quest lifecycle rule pack, including the
  mechanics-stub rule, so quests complete without XP, review, battle,
  reputation, inventory, or store side effects. The processors are rebuilt
  against the two-loop principle in a subsequent change; no incentive-mechanic
  behavior is in scope here.
- Resolve the agentprogression slice through issue #30: keep
  `agent_progression` registered but disabled in all three shipped profiles;
  delete its no-publisher `AgentXPPayload`, false output port, and six dead
  vocabulary entries; and retain the Agent/reconstruction, XP engine,
  validation, rules, and dormant Phase-2 implementation.
- Adopt the framework `lifecycle-gateway` operator REST surface for quest
  workflow CRUD/history where it can replace bespoke `service/api` handlers;
  keep product endpoints (world state, DM chat, workspace) as-is.
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
  Agentstore, boidengine, and redteam remain open under Stage B3.
- **Dependencies**: the beta.21 → beta.135 foundation is implemented on this
  branch. The beta.158 pre-v1 clean break is a separate prerequisite change:
  `adopt-semstreams-prev1-contract`. No dependency-only bump is permitted.
- **Data/predicates**: quest entity predicates consumed by the UI
  (`worldStore`) must remain stable or ship with a documented mapping;
  `quest.dag.*` layout may change with the gated-dag model.
- **Tests/e2e**: unit + integration suites migrate with the components; mock
  e2e is the completion gate. Known pre-existing flakes (settings.spec tier1)
  are not in scope.
- **Docs**: `docs/03-QUESTS.md` lifecycle/DAG sections are superseded by the
  `quest-lifecycle` and `dag-execution` specs seeded by this change; ADRs
  002/003 remain as history.
