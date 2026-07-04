# Research 07 — Firing proof + a semstreams framework bug it caught

Task 2.3 de-risk (chosen path: "prove firing first"). Built an end-to-end
integration test (`questlifecycle/rules_fire_test.go`) standing up real
`graph-ingest` + `rule-processor` + `lifecycle.Manager`, then drove a posted quest
through `command=claim`. The test **caught a real semstreams bug** that would have
silently broken the whole pure-rules approach at the mock e2e.

**Filed upstream: [C360Studio/semstreams#451](https://github.com/C360Studio/semstreams/issues/451)** (root cause + repro + proposed fix). Resolution chosen: file
the bug + apply the bare-predicate workaround in semdragons now so Stage B1 keeps
moving; revert to `$entity.lifecycle.phase` when the framework fix lands.

## The bug: hot-reload drops the lifecycle Manager on rules

**Symptom (empirical, from the firing test):**
- Claim rule gated on `[{quest.command == claim}]` (command only) → **fires**, quest
  transitions posted → claimed, `claimed_by` set, `attempts` incremented, `command`
  cleared. Test PASSES.
- Same rule gated on `[{$entity.lifecycle.phase == posted}, {quest.command == claim}]`
  → **never fires** ("Rule did not trigger" on every evaluation), quest stays posted.
  Test FAILS.

So the `$entity.lifecycle.phase` condition silently evaluates false.

**Root cause (semstreams `processor/rule`, ~beta.132/133):**
1. `EvaluateEntityState` (the entity-watch eval that produces the `triggered`
   bool the stateful evaluator consumes, `message_handler.go:273`, `:296-303`)
   resolves `$entity.lifecycle.phase` via
   `PopulateLifecycleStateFields(ctx, r.lifecycleManager, …)`
   (`expression_factory.go:193`) — reading the **ExpressionRule's own**
   `r.lifecycleManager`.
2. The file-load path sets that manager on each rule
   (`rule_loader.go:184-190`, SetLifecycleManager after CreateRuleFromDefinition).
3. **But** rules loaded from `rules_files` are seeded to KV and the config
   watcher's startup reconcile (`kv_config_integration.go:166` "initial-snapshot
   delimiter → trigger a reconcile") **re-creates every rule** via
   `applyExpressionRuleChange` (`runtime_config.go:182-203`) →
   `createRuleFromConfig` → `rp.rules[ruleID] = newRule` **with no
   `SetLifecycleManager` call**.
4. So the active (hot-reloaded) rules have a **nil** `lifecycleManager`;
   `PopulateLifecycleStateFields` no-ops; `$entity.lifecycle.phase` is absent; the
   condition (default `Required:false`) yields no-match. The rule never fires.

**Why upstream never caught it:** the only shipped lifecycle rule example
(`configs/rules/lifecycle/01-mission-launch.json`) gates on the command triple
alone, never on `$entity.lifecycle.phase`, so it works through the hot-reload path
by accident.

**The one-line framework fix** would be to mirror `rule_loader.go:184-190` inside
`applyExpressionRuleChange` (`runtime_config.go:182`): after
`createRuleFromConfig`, if `rp.lifecycleManager != nil` and the rule implements the
setter, call `SetLifecycleManager`. (Same for `applyCronRuleChange` if cron rules
ever gate on lifecycle fields.)

## The workaround (confirmed working, stays in semdragons)

Gate on the **bare phase predicate** `quest.status.state` instead of the harness
token `$entity.lifecycle.phase`:

```json
{ "field": "quest.status.state", "operator": "eq", "value": "posted" }
```

This reads the phase directly from the watched entity snapshot (no lifecycle
Manager needed), so it is immune to the hot-reload bug. **Verified:** the firing
test passes with this gate.

**Is the bare-predicate read safe here?** Yes, for this workflow:
- The recipe's warning about bare-predicate phase reads is about *accumulated*
  phase triples (`[posted, claimed]`) making first-match read a stale value. That
  cannot happen here because `Manager.TransitionWith` writes the phase
  **replace-not-append** (`RemoveTriples` every delta predicate then add), so after
  any transition there is exactly **one** `quest.status.state` triple.
- There is a transient **duplicate** `quest.status.state=posted` at create time
  (see below), but both copies are `posted`, so first-match still reads `posted`,
  and the first real transition collapses it to one triple.

**Trade-off of the workaround:** the bare-predicate gate drops the
lifecycle-attachment requirement, so a rule matches **any** quest carrying the
phase triple — including questboard-managed quests during the transient
side-by-side window (before questboard is deleted in task 2.5). In the end state
(questboard deleted, all quests `Manager.Create`d) this is a non-issue. The mitigation
is to sequence the caller rewire + questboard deletion so there is no meaningful
side-by-side window where both write the same quest.

## Secondary observation: duplicate `quest.status.state` at create

`Manager.Create` of a `QuestState{Phase: posted}` yields an entity with **two**
`quest.status.state = posted` triples (seen in the firing-test raw dump). Both are
`posted` and the first transition replaces them with one, so it is cosmetically
harmless for gating, but worth a follow-up (likely the projection writing the phase
field alongside the create's phase triple). Not blocking.

## Decision needed (see checkpoint)

- **(A) Workaround in semdragons** — switch all `config/rules/quest/*.json` phase
  gates to `quest.status.state`. Zero framework change; firing proven green.
- **(B) Fix semstreams** — patch `applyExpressionRuleChange` to propagate the
  Manager; consume via a local `replace` or a new semstreams release. Keeps the
  recipe-preferred `$entity.lifecycle.phase` gate; expands this change to touch the
  framework.

## Source index

| Concern | File:line |
|---|---|
| entity-watch eval consumes rule's `triggered` bool | `processor/rule/message_handler.go:273,296-303,319-327` |
| `$entity.lifecycle.phase` resolution reads `r.lifecycleManager` | `processor/rule/expression_factory.go:159,182,193` |
| file-load SETS manager on rules | `processor/rule/rule_loader.go:184-190` |
| hot-reload re-create does NOT set manager | `processor/rule/runtime_config.go:182-203` |
| startup reconcile re-creates all rules | `processor/rule/kv_config_integration.go:166-176` |
| replace-not-append phase write | `pkg/lifecycle/manager.go:660-683` |
