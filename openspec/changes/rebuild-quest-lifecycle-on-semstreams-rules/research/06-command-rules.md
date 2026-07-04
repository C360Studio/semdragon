# Research 06 — Authoring command-driven quest-lifecycle rules (verified recipe)

**Source tree:** `github.com/c360studio/semstreams` local checkout
`/Users/coby/Code/c360/semstreams` @ `v1.0.0-beta.133`
(semdragons currently pins `v1.0.0-beta.132` in `go.mod`). Every string,
struct field, and behaviour below is quoted verbatim from that tree with
`file:line` citations. No code was modified.

**Goal recap.** Replace hand-rolled quest transition handlers with *rule files*.
A caller stamps a **command triple** on a quest entity (e.g. `quest.command=claim`
plus argument triples), and a rule matching `(command AND current phase)` fires a
`lifecycle_transition` action that atomically sets coupled fields (`claimed_by`,
…) and **clears the command** so it cannot re-fire.

**The five load-bearing facts (all verified):**

1. Phase gate token is literally **`$entity.lifecycle.phase`** (an `eq` condition
   against the workflow's current phase). Plain command triples are read by their
   **bare predicate name** (`field: "quest.command"`).
2. `lifecycle_transition.set` values substitute another triple with
   **`$entity.triple.<predicate>`** — the research draft's guess is **correct**.
3. Command clearing is done **inside the same atomic transition** by `set`-ing the
   command field to a value the `eq` gate won't match (empty string or a sentinel).
   There is **no delta-remove path through `set`** — `set` overwrites, it never
   deletes a triple.
4. The command field must be a `lifecycle:"operator_writable,predicate=..."`
   field on the Participant struct for `set` to touch it; otherwise clear it with a
   companion `remove_triple`/`update_triple` action.
5. The correct command **write path** is a **graph-ingest delta** (`add_triple`
   mutation / an ingested Graphable), **not** semdragons' current full-entity
   raw-KV `Put`, because the harness delta-merges and a full Put would clobber the
   harness-written phase/audit triples.

---

## 0. Runtime wiring — is graph-ingest required? YES.

### 0a. Component registration

The rule processor registers under the name **`rule-processor`**
(`processor/rule/factory.go:14-25`):

```go
func Register(registry *component.Registry) error {
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "rule-processor",
		Factory:     CreateRuleProcessor,
		...
```

`CreateRuleProcessor(rawConfig, deps)` (`factory.go:34`) requires
`deps.NATSClient` (hard-fail if nil, `factory.go:36-39`) and then **pulls the
lifecycle Manager off `deps` automatically** — no separate `SetLifecycleManager`
call at the composition root (`factory.go:102-110`):

```go
	// Propagate the Lifecycle harness Manager (ADR-047) ...
	if deps.LifecycleManager != nil {
		processor.SetLifecycleManager(deps.LifecycleManager)
	}
```

`deps.LifecycleManager` is `*lifecycle.Manager` (`component/dependencies.go:97`).
So the cmd wiring requirement is: **construct one `lifecycle.Manager`, register
the `quest` workflow on it, and hand it to the component `Dependencies` used to
build every component.** The rule processor then propagates it internally to both
the stateful evaluator and each expression rule (`processor/rule/processor.go`):

- `SetLifecycleManager` on the Processor stores it (`processor.go:308-309`).
- It is pushed to each rule instance that implements the setter
  (`processor.go:598-602`) and to the stateful evaluator
  (`processor.go:661-662`: `rp.statefulEvaluator.SetLifecycleManager(...)`).

If a rule references `lifecycle_transition` but no Manager was wired, the action
**loud-fails** with an authoring-grade message rather than silently no-op'ing
(`processor/rule/actions_lifecycle.go:166-171`):

```go
func (e *ActionExecutor) requireLifecycleManager(actionType string) (LifecycleManager, error) {
	if e.lifecycle == nil {
		return nil, fmt.Errorf("%s: no lifecycle.Manager wired on the rule processor ...", actionType)
	}
```

### 0b. graph-ingest IS required at runtime

The Manager owns **no storage**. Every `lifecycle_transition` persists by emitting
triples through graph-ingest's mutation subject
(`pkg/lifecycle/graph_emit.go:77-80`):

```go
const (
	graphSubjectUpdateWithTriples = "graph.mutation.entity.update_with_triples"
	graphSubjectCreateWithTriples = "graph.mutation.entity.create_with_triples"
)
```

`Manager.TransitionWith` → `m.emitter.update(...)` → NATS request to that subject
(`manager.go:684`, `graph_emit.go:125-149`). **Without a running `graph-ingest`
component subscribed to `graph.mutation.entity.update_with_triples`, no transition
persists.** (The Manager even has a ~13s retry budget specifically for the
graph-ingest cold-start race — `graph_emit.go:82-98`.) semdragons already runs
`graph-ingest` in `config/semdragons.json:198`, so this is satisfied.

### 0c. Rule-processor config (`processor/rule/config.go`)

| JSON key | Meaning | Default |
|---|---|---|
| `rules_files` | Paths to JSON rule files (`config.go:19`) | `[]` |
| `inline_rules` | Inline `[]Definition` (`config.go:20`) | — |
| `entity_watch_buckets` | `map[bucket][]pattern` to watch (`config.go:39-43`) | — |
| `entity_watch_patterns` | patterns on `ENTITY_STATES` only, back-compat (`config.go:37`) | — |
| `enable_graph_integration` | (`config.go:32`) | `true` |
| `debounce_delay_ms` | settle window; **0 = evaluate every change** (`config.go:45-48`) | `0` |

The watcher only ever opens the **`ENTITY_STATES`** bucket for patterns
(`entity_watcher.go:51-58`, `:75`, `:449` all hardcode `"ENTITY_STATES"`). For
quests, set:

```json
"entity_watch_patterns": ["c360.prod.game.board1.quest.*"]
```

**No KV config bucket / hot-reload is required for the command-rule use case.**
(A `PackID` + `ProjectionContracts` axis exists — `config.go:58-70` — but that is
ADR-056 projection ownership, orthogonal to command rules; leave `pack_id` empty.)

**Bucket coincidence check (important):** the Manager opens
`graph.BucketEntityStates` which is the constant `"ENTITY_STATES"`
(`pkg/lifecycle/manager.go:332`, `graph/constants.go:6`), and semdragons'
`BoardConfig.BucketName()` **returns the same string**
(`semdragon/domain/config.go:160-161`: `return "ENTITY_STATES"`). So the rule
watcher, the Manager, graph-ingest, and semdragons' own writes all sit on one
bucket. There is **no bucket mismatch** to reconcile.

---

## 1. Condition grammar — verified field tokens

### 1a. The condition shape (`processor/rule/expression/types.go:10-33`)

```go
type ConditionExpression struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value"`
	Required bool        `json:"required"`
	From     interface{} `json:"from,omitempty"` // transition operator only
}
```

### 1b. Phase gate — literally `$entity.lifecycle.phase`

Field resolution branches on prefix in
`evaluator.go:184-198`. `$entity.lifecycle.*` is resolved from the pre-populated
`stateFields` map (**not** a triple lookup):

```go
	if strings.HasPrefix(condition.Field, "$state.") ||
		strings.HasPrefix(condition.Field, "$prev.") ||
		strings.HasPrefix(condition.Field, "$entity.lifecycle.") {
		fieldValue, exists := stateFields[condition.Field]
		if !exists {
			if condition.Required { return false, &EvaluationError{...} }
			return false, nil          // missing ⇒ no match (Required=false)
		}
		return e.applyOperator(condition, fieldValue)
	}
```

The map is filled by `PopulateLifecycleStateFields`
(`processor/rule/lifecycle_substitution.go:50-61`) which writes exactly three keys:

```go
	fields["$entity.lifecycle.phase"]    = p.Phase()
	fields["$entity.lifecycle.terminal"] = p.IsTerminal()   // bool
	fields["$entity.lifecycle.workflow"] = p.Workflow()
```

This runs on **both** evaluation paths:
- initial match: `expression_factory.go:192-193` inside `EvaluateEntityState`.
- stateful edge re-eval: `stateful_evaluator.go:211` and `:251`.

**Verified exact strings you may gate on:**

| Condition `field` | Resolves to | Type |
|---|---|---|
| `$entity.lifecycle.phase` | current phase (e.g. `"posted"`) | string |
| `$entity.lifecycle.terminal` | is-terminal | bool |
| `$entity.lifecycle.workflow` | workflow name (e.g. `"quest"`) | string |

There is **no** `$entity.lifecycle.state`. The workflow's *phase predicate* on the
quest is `quest.status.state` (semdragons' choice), but a rule condition gates on
`$entity.lifecycle.phase`, which the harness resolves through the Manager — you do
**not** write `field: "quest.status.state"` for the gate (that bare-predicate form
also works as a raw triple read, but it reads the *watched snapshot* triple, not
the Manager's authoritative projection — see §1e caveat, prefer the harness token).

A quest only resolves `$entity.lifecycle.phase` **after it has been attached to
lifecycle** (has a phase triple + matches the workflow `EntityIDPattern`) — see
`LookupByEntityID` → `Get` requiring `hasTriple(...PhasePredicate)`
(`manager.go:393-396`, `manager_query.go:398-405`). Quests must be created via
`Manager.Create` (initial phase `posted`) for phase gates to fire.

### 1c. Reading a plain command triple — bare predicate name

A non-lifecycle triple is read by its **bare predicate** (no `$` prefix). Falls to
branch 4 of `evaluator.go:231-267`, which calls the default type detector
(`evaluator.go:392-401`):

```go
func (d *defaultTypeDetector) GetFieldValue(entityState *gtypes.EntityState, field string) (interface{}, bool, error) {
	for _, triple := range entityState.Triples {
		if triple.Predicate == field {
			return triple.Object, true, nil
		}
	}
	return nil, false, nil
}
```

So `{"field": "quest.command", "operator": "eq", "value": "claim"}` compares the
`Object` of the `quest.command` triple against `"claim"`. A missing triple with
`Required:false` yields **no match** (`evaluator.go:275-287`).

### 1d. Every wired operator (`expression/evaluator.go:22-52`, names in `types.go:119-147`)

| Category | Operators |
|---|---|
| numeric | `eq` `ne` `lt` `lte` `gt` `gte` `between` |
| string | `contains` `starts_with` `ends_with` `regex` |
| array/set | `in` `not_in` `length_eq` `length_gt` `length_lt` `array_contains` |
| transition | `transition` (special-cased, needs `$prev.*` — `evaluator.go:218-219`, `:315-378`) |

The config validator derives its accepted set from this exact wiring
(`rule_factory.go:275-278` → `expression.RegisteredOperators()`), so an unknown
operator is rejected at load, not at fire time. `between` and `in`/`not_in` take a
JSON array `value`; comparison is numeric-first then lexicographic
(`compareValuesWithError`, `evaluator.go:691-715`); numeric-looking strings are
coerced to float (`toFloat64`, `evaluator.go:742-811`) — so `attempts.current`
compares numerically even if stored as a string.

### 1e. AND / OR

Rule-level logic is the `logic` field on the rule (`"and"` | `"or"`), applied in
`evaluator.go:148-169`. **Caveat on the default:** the raw evaluator defaults an
empty `logic` to **OR** (`evaluator.go:149`), but `NewExpressionRule` rewrites an
empty `logic` to **`"and"`** before the evaluator sees it
(`expression_factory.go:59-62`). Action-level `when:` guards are **always AND**
(`stateful_evaluator.go:516-519`). **Recommendation: always set `"logic": "and"`
explicitly** on command rules so the multi-condition `(phase AND command AND arg)`
gate is unambiguous regardless of which path evaluates.

> **Read-skew caveat.** `quest.command` (bare) is read from the *watched KV
> snapshot* delivered to the rule; `$entity.lifecycle.phase` is resolved by a
> *fresh* `Manager.LookupByEntityID` → `getEntity` KV read at fire time
> (`lifecycle_substitution.go:200`, `manager.go:343-360`). In the steady state
> (command stamped onto a quest already in a phase) both reflect the same
> committed state and agree. A theoretical skew exists only if the phase is
> rewritten between the snapshot revision and the fresh read; the transitions
> table (§4c) is the backstop.

---

## 2. Action `set` substitution grammar — verified

### 2a. The `lifecycle_transition` action fields (`processor/rule/actions.go`)

| JSON key | Purpose | citation |
|---|---|---|
| `type` | `"lifecycle_transition"` | `actions.go:68` |
| `workflow` | target workflow; empty ⇒ resolve by entity-ID scan | `actions.go` (Workflow doc), `actions_lifecycle.go:178-194` |
| `phase` | target phase (substituted) | `actions.go` (Phase doc), `actions_lifecycle.go:44-50` |
| `reason` | audit note (substituted) | `actions.go` (Reason doc) |
| `set` | `map[string]any` field mutations, atomic with phase | `actions.go` (Set doc), `actions_lifecycle.go:56-81` |
| `when` | optional guard (AND) | `actions.go` (When doc) |

### 2b. `set` keys are Participant JSON names; values are literal-or-typed-op

`applyLifecycleSet` matches each key against the **JSON tag** of the Participant
struct field (`actions_lifecycle.go:269-293`, `buildJSONFieldIndex` at `:394-408`).
Before the mutator is even built, **every `set` key is validated against
default-deny** (`actions_lifecycle.go:68-72`):

```go
	for jsonName := range action.Set {
		if err := mgr.AssertRuleWritable(workflow, jsonName); err != nil {
			return fmt.Errorf("lifecycle_transition: workflow=%q: %w", workflow, err)
		}
	}
```

`AssertRuleWritable` rejects unknown fields, the id field, the phase field, and any
field **not** tagged `operator_writable` (`manager_query.go:456-479`). So a `set`
target must be declared:

```go
Command   string `json:"command"    lifecycle:"operator_writable,predicate=quest.command"`
ClaimedBy string `json:"claimed_by" lifecycle:"operator_writable,predicate=quest.claim.agent"`
```

(`operator_writable` requires a `predicate=` — `pkg/lifecycle/tags.go:285-288`.)

### 2c. The typed ops (`actions_lifecycle.go:255-336`)

```jsonc
"set": {
  "claimed_by":       "dragon",                 // bare literal
  "verdict":          {"op": "set", "value": "x"},   // == bare literal
  "attempts_current": {"op": "increment"},      // numeric field += 1
  "budget":           {"op": "decrement"}       // numeric field -= 1 (uint clamps at 0)
}
```

- bare literal → `assignLiteral` (`:338-353`), assignable/convertible-numeric only.
- `{"op":"set","value":v}` → same as bare literal (`:302-303`).
- `{"op":"increment"}` / `{"op":"decrement"}` → `addToNumeric` (`:355-378`); requires
  a numeric field or the transition aborts (`:374-375`).
- Unknown op → error, whole transition aborts (`:308-309`, no partial write).

### 2d. Injecting another triple's value — `$entity.triple.<predicate>` (CONFIRMED)

`set` string values (and the typed-op `value`) are run through
`ec.SubstituteVariables` (`substituteSetValue`, `actions_lifecycle.go:233-252`).
The generic triple substitution is `execution_context.go:289-299`:

```go
	if ec.Entity != nil {
		...
		for _, triple := range sorted {
			key := "$entity.triple." + triple.Predicate
			result = strings.ReplaceAll(result, key, fmt.Sprintf("%v", triple.Object))
		}
	}
```

So the research draft's guess is **correct**:
`"claimed_by": "$entity.triple.quest.claim.agent"` injects the Object of the
`quest.claim.agent` triple. (Predicates are substituted longest-first to avoid a
short predicate swallowing a longer one — `execution_context.go:284-294`.)

**Full substitution namespace** (`execution_context.go:176-337`,
`SubstituteVariables` docstring `:176-211`):

| Namespace | Tokens |
|---|---|
| entity | `$entity.id`, `$entity.{org,platform,domain,system,type,instance}`, `$entity.triple.<pred>`, `$entity.triple.<pred>.length`, `$entity.triple.<pred>.triples`, `$entity.lifecycle.{phase,terminal,workflow,workflow_def}` |
| related | `$related.*` (mirror of entity) |
| state | `$state.iteration`, `$state.max_iterations`, `$state.last_transition`, `$prev.<field>` |
| message | `$message.<dotted.path>` (message-path rules only) |
| other | `$now`, `$schedule.*` (cron), `$caller.*` |

### 2e. Loud-fail behaviour on unresolved tokens

For **`phase` / `workflow` / `reason`** the resolver is strict — a surviving
framework token is a **hard error** (`actions_lifecycle.go:212-218`,
`resolveLifecycleString`):

```go
	if leftovers := unresolvedTemplateVarRe.FindAllString(resolved, -1); len(leftovers) > 0 {
		return "", fmt.Errorf("%s %q has unresolved template variables %v after substitution ...", fieldName, raw, leftovers)
	}
```

**Asymmetry to know:** `set` **values** are *not* run through
`resolveLifecycleString`; a surviving `$entity.triple.X` in a `set` value lands the
literal template string as the field value and only logs a Warn
(`execution_context.go:685-695`, `warnUnresolvedTemplateVars`). So a typo in a
`set` value corrupts the field silently-ish (Warn, not error). Author `set`-value
predicate names carefully, and prefer arg predicates that are guaranteed present at
command time (write them in the **same** command message).

---

## 3. Idempotency / command clearing — the crux

### 3a. Why it re-fires (the exact failure mode)

`on_enter` fires on a **false→true** edge (`DetectTransition`, consumed at
`stateful_evaluator.go:150`, `:289-306`). Consider a claim rule gated on
`(phase==posted AND command==claim)`:

1. `posted` + no command → false. Caller stamps `command=claim` → **false→true** →
   `on_enter` transitions to `claimed`.
2. The transition write flips phase → condition false → **true→false** → `on_exit`
   (none). Not matching. Good.
3. **If the command is NOT cleared** and the quest later re-enters `posted` (a
   fail→posted retry, §4c), the stale `command=claim` makes the gate **false→true
   again** → `on_enter` re-fires a spurious re-claim.

Clearing the command inside the winning transition removes step 3.

### 3b. `set` overwrites; it never deletes — but overwrite is enough

`TransitionWith` runs the mutator on a projected Participant, then **diffs** the
mutated struct against the original triples and emits a delta for each changed
field (`manager.go:645-658`), and **replaces** (remove-then-add) each delta
predicate (`manager.go:660-683`):

```go
	// Replace, don't append: ... remove the prior triple for each predicate before adding the new one.
	removePreds := make([]string, 0, len(delta))
	for _, t := range delta { removePreds = append(removePreds, t.Predicate) }
	emitReq := &graph.UpdateEntityWithTriplesRequest{
		...
		AddTriples:    delta,
		RemoveTriples: removePreds,
		ExpectedRevision: currentRev,
	}
```

graph-ingest applies remove **then** add for the CAS path
(`processor/graph-ingest/mutations.go`: `removeSet` loop then
`graph.MergeTriples(merged, req.AddTriples)`, in `handleEntityUpdateWithTriplesCAS`).

The diff (`diffProjectedTriples`, `manager.go:724-758`) emits a delta when the
projected value differs from the original triple string. **Trace for clearing
`command` from `"claim"` to `""`:** `hasOriginal=true`, `fieldVal.IsZero()=true`;
the `!hasOriginal && IsZero` skip is false (`:742-744`); `oldStr="claim"`,
`newStr=""`, so `hasOriginal && oldStr==newStr` is false (`:752-753`) → **emits
`triple(quest.command, "")`**. Net effect: `quest.command` is *replaced* with an
empty-Object triple. The `eq "claim"` gate then evaluates **false**. Re-fire is
prevented — the triple still exists but no longer matches.

**Definitive pattern — clear via `set` to a sentinel (atomic, single write):**

```jsonc
"set": { "command": "consumed" }   // or "" — see note
```

A **non-empty sentinel** (`"consumed"`, `"_done"`) is the safest form: it
guarantees a non-zero diff delta lands and unambiguously fails every command `eq`
gate. Empty string `""` also works logically (`eq "claim"` → false), but a
sentinel sidesteps any question about whether `graph.MergeTriples` stores an
empty-Object triple. Either way the clear is **atomic with the phase change** —
one `update_with_triples` write, no ordering window.

Requirement: `command` must be an `operator_writable` Participant field (§2b) for
`set` to reach it.

### 3c. Alternative clear — companion `remove_triple` (deletes entirely)

If you prefer the command to be a **plain triple** (not part of the lifecycle
schema), it cannot be `set`-cleared (AssertRuleWritable rejects non-schema fields).
Instead add a second action to the same `on_enter` list:

```jsonc
{ "type": "remove_triple", "predicate": "quest.command" }
```

`remove_triple` → `tripleMutator.RemoveTriple` → `graph.mutation.triple.remove`
(`processor/rule/triple_mutator.go:19`, `:98-134`), which **deletes** the predicate
(so the gate reads "field missing" → false). This is *two writes* (phase via the
Manager, command-delete via the mutator), non-atomic, but the phase flip already
guards the common path; the delete is belt-and-suspenders for the re-enter case.
`update_triple` (`actions.go:960`) is the remove-then-re-add variant if you want to
overwrite to a sentinel via the triple path instead.

**Self-write loop safety.** The rule processor tracks revisions it generated via
`trackRuleRevision` (`processor.go:1356-1371`) and skips its own writes once
(`message_handler.go:250-256`, `shouldSkipRule`). `remove_triple`/`add_triple` go
through `tripleMutator`, which records the revision (`triple_mutator.go:89-91`), so
they are guarded. The `lifecycle_transition` write goes through the **Manager**, not
`tripleMutator`, so its revision is **not** tracked — the watcher *will*
re-evaluate on the transition's self-write, but that re-eval sees the new phase, the
gate is false, and it produces `on_exit` (no re-fire). No infinite loop. Verified
against the mission fixture which clears nothing and does not loop
(`configs/rules/lifecycle/01-mission-launch.json`).

### 3d. The reference fixture (`configs/rules/lifecycle/01-mission-launch.json`)

The only shipped lifecycle rule gates on the **command triple alone** and relies on
the transitions table for phase safety (it does not clear, because a mission
launches once and never re-enters its start phase):

```json
{ "id": "lifecycle_mission_launch_on_command", "type": "expression",
  "conditions": [ { "field": "mission.command", "operator": "eq", "value": "launch" } ],
  "logic": "and",
  "on_enter": [ { "type": "lifecycle_transition", "workflow": "mission",
                  "phase": "flying", "reason": "command=launch from UDP" } ] }
```

For quests, which **do** re-enter phases (retry), you must add the phase gate **and**
the command clear — that is the whole point of §3b.

---

## 4. `on_enter` / `on_exit` / `while_true` / `on_recovery` semantics

From `Definition` (`processor/rule/rule_factory.go:26-36`) and the selector
(`stateful_evaluator.go:280-325`):

| List | Fires when | Use for |
|---|---|---|
| `on_enter` | condition **false→true** (increments `$state.iteration`) | one-shot command transitions |
| `on_exit` | condition **true→false** | cleanup on leaving a matched state |
| `while_true` | every update while **still true** (`TransitionNone` + matching) | steady-state polling |
| `on_recovery` | once on restart if still matching; else falls back to `on_enter` under `rerun_on_recovery` | resume state machines after a crash |

**For a one-shot command-driven transition, use `on_enter`.** The false→true edge
fires exactly once per command-arrival because the transition immediately makes the
gate false again (phase changed and/or command cleared).

**For a reactive condition** (fail→retry when `attempts<max`), also use `on_enter`
gated on `$entity.lifecycle.phase == failed` plus the numeric guard — it fires once
on *entering* `failed`. (`while_true` would re-fire on every unrelated triple churn
while still in `failed`; avoid it here.)

`on_recovery` matters for durability: if the backend restarts while a command is
pending, a bare `on_enter` rule will **not** re-fire on bootstrap unless it declares
`on_recovery` or sets `rerun_on_recovery:true` (`stateful_evaluator.go:426-437`,
`shouldFireOnRecovery`). For command rules this is usually desirable — a command
stamped just before a crash should still fire on restart. Add
`"rerun_on_recovery": true` to the claim/start/submit rules.

---

## 5. Who writes the command triple, and how

### 5a. The verified reference producer writes a DELTA through graph-ingest

The framework's own command producer (`cmd/e2e-semstreams/mission/command.go`)
emits a Graphable carrying a **single** command triple, published to a subject that
**flows through graph-ingest** into `ENTITY_STATES` (`command.go:69-80`, `:135-139`):

```go
// Triples returns the single command triple. graph-ingest writes
// this into ENTITY_STATES; the rule engine fires on the predicate.
func (c *Command) Triples() []message.Triple {
	return []message.Triple{{ Subject: c.EntityID(), Predicate: "mission.command",
		Object: c.Command, Source: "mission-command", ... }}
}
```

> "Output flows through graph-ingest which lands the triple in ENTITY_STATES, where
> the rule engine picks it up." — `command.go:137-139`.

graph-ingest **delta-merges** (replace-by-subject+predicate, preserving all other
triples), so stamping `quest.command` does not disturb the harness-written phase.
The rule watcher observes every `ENTITY_STATES` write (§0c), so the command becomes
visible to the condition regardless of writer.

### 5b. Concrete write options for semdragons callers (API / boid / autonomy)

Any of these make a `quest.command` triple visible to a rule condition, because all
land in `ENTITY_STATES` which the rule watches:

1. **Direct triple-add mutation (recommended for callers already on NATS):**
   NATS request to `graph.mutation.triple.add` with an `AddTripleRequest`
   (`triple_mutator.go:19`, `:54-93`). Delta-merge, preserves phase. This is the
   cleanest "stamp one command triple" path.
2. **Ingest a Graphable** the mission-command producer's way (publish a
   BaseMessage-wrapped Graphable to a subject graph-ingest consumes) — best when a
   dedicated command processor already exists.
3. **`update_with_triples` mutation** for command + argument triples in one atomic
   delta (`graph.mutation.entity.update_with_triples`, `graph_emit.go:78`).

### 5c. semdragons' CURRENT pattern is the wrong one for coexistence — flag it

semdragons today writes the **entire entity** via a raw KV `Put`/`Update`, not a
delta (`semdragon/graphclient.go`):

```go
// EmitEntity — marshals the FULL EntityState (all triples) and store.Put()s it
func (gc *GraphClient) EmitEntity(ctx, entity graph.Graphable, eventType string) error {
	data, _ := marshalEntityState(entity, eventType)   // graphclient.go:79-92 — full entity
	...
	if _, err := store.Put(ctx, entity.EntityID(), data); err != nil { ... }  // :123
}
// EmitEntityCAS uses store.Update(...revision) — :142-157
```

`marshalEntityState` serializes **`entity.Triples()`** — the complete triple set
(`graphclient.go:79-92`). A full `Put` therefore **overwrites every triple**,
including the harness-written `quest.status.state` (phase), audit triples, and
`set`-coupled fields. If a caller stamps a command by `Put`-ing a stale in-memory
quest, it will **clobber a concurrent `lifecycle_transition`'s phase write** (the
Manager delta-merges onto whatever graph-ingest last stored; a subsequent
full-Put erases it). The Manager's CAS (`ExpectedRevision`) protects the Manager's
own write, but nothing protects against a *later* full-Put from semdragons.

**Definitive write-path guidance for the pure-rules design:** command writers must
use **delta mutations (§5b options 1–3), not `GraphClient.EmitEntity`/full-Put**.
More broadly, once the lifecycle harness owns quest phase/coupled-field writes,
semdragons should stop full-Put-ing quests it also wants the harness to manage;
mutate quests via triple deltas so harness-owned and app-owned triples merge instead
of racing. (This is the same "delete-all + re-add shim" hazard graph-ingest calls
out at `mutations.go:636-640`.)

---

## 6. Complete worked example set (ready-to-adapt JSON)

All examples assume a registered `quest` workflow whose `PhasePredicate` is
`quest.status.state` and whose `Transitions` table declares the edges used below
(e.g. `posted→[claimed]`, `claimed→[in_progress]`, `in_progress→[in_review,
completed,failed]`, `in_review→[completed,failed]`, `failed→[posted,
pending_triage]`, terminals `completed`, `pending_triage`). The Participant struct
must declare these `operator_writable` fields with predicates:

```go
type QuestState struct {
    ID        string `json:"id"          lifecycle:"id"`
    Phase     string `json:"phase"       lifecycle:"phase,predicate=quest.status.state"`
    Command   string `json:"command"     lifecycle:"operator_writable,predicate=quest.command"`
    ClaimedBy string `json:"claimed_by"  lifecycle:"operator_writable,predicate=quest.claim.agent"`
    AttemptsCurrent int `json:"attempts_current" lifecycle:"operator_writable,predicate=quest.attempts.current"`
    Verdict   string `json:"verdict"     lifecycle:"operator_writable,predicate=quest.review.verdict"`
    // ... audit predicates declared on the Workflow.AuditPredicates
}
```

Argument triples the *caller* writes alongside the command (plain triples, not
schema fields): `quest.claim.requested_by` (claimant agent id),
`quest.review.needs_review` (bool). Every rule sets `"rerun_on_recovery": true` so
a command that lands just before a restart still fires.

### 6a. claim: `(posted AND command=claim)` → `claimed`, set claimant, clear command

```json
{
  "id": "quest_claim_on_command",
  "type": "expression",
  "name": "Quest claim on command",
  "enabled": true,
  "logic": "and",
  "rerun_on_recovery": true,
  "conditions": [
    { "field": "$entity.lifecycle.phase", "operator": "eq", "value": "posted" },
    { "field": "quest.command",           "operator": "eq", "value": "claim" }
  ],
  "on_enter": [
    {
      "type": "lifecycle_transition",
      "workflow": "quest",
      "phase": "claimed",
      "reason": "command=claim",
      "set": {
        "claimed_by": "$entity.triple.quest.claim.requested_by",
        "command": "consumed"
      }
    }
  ]
}
```

### 6b. start: `(claimed AND command=start)` → `in_progress`, clear command

```json
{
  "id": "quest_start_on_command",
  "type": "expression",
  "enabled": true,
  "logic": "and",
  "rerun_on_recovery": true,
  "conditions": [
    { "field": "$entity.lifecycle.phase", "operator": "eq", "value": "claimed" },
    { "field": "quest.command",           "operator": "eq", "value": "start" }
  ],
  "on_enter": [
    {
      "type": "lifecycle_transition",
      "workflow": "quest",
      "phase": "in_progress",
      "reason": "command=start",
      "set": { "command": "consumed" }
    }
  ]
}
```

### 6c. submit — branch on `quest.review.needs_review` (two mutually-exclusive rules)

`submit` with review needed → `in_review`; without → straight to `completed`. Both
gate on the same `(in_progress AND command=submit)` plus the opposite value of the
`needs_review` triple, so exactly one fires.

```json
{
  "id": "quest_submit_needs_review",
  "type": "expression",
  "enabled": true,
  "logic": "and",
  "rerun_on_recovery": true,
  "conditions": [
    { "field": "$entity.lifecycle.phase",       "operator": "eq", "value": "in_progress" },
    { "field": "quest.command",                 "operator": "eq", "value": "submit" },
    { "field": "quest.review.needs_review",     "operator": "eq", "value": true }
  ],
  "on_enter": [
    { "type": "lifecycle_transition", "workflow": "quest", "phase": "in_review",
      "reason": "command=submit, review required", "set": { "command": "consumed" } }
  ]
}
```

```json
{
  "id": "quest_submit_no_review",
  "type": "expression",
  "enabled": true,
  "logic": "and",
  "rerun_on_recovery": true,
  "conditions": [
    { "field": "$entity.lifecycle.phase",   "operator": "eq", "value": "in_progress" },
    { "field": "quest.command",             "operator": "eq", "value": "submit" },
    { "field": "quest.review.needs_review", "operator": "eq", "value": false }
  ],
  "on_enter": [
    { "type": "lifecycle_transition", "workflow": "quest", "phase": "completed",
      "reason": "command=submit, no review", "set": { "command": "consumed" } }
  ]
}
```

### 6d. fail: retry-vs-triage (two rules gated by `lt`/`gte` — race-safe)

Reactive on **entering `failed`** (no command needed). `attempts.current < max` →
back to `posted` with `increment attempts` + clear the assignment; `>= max` →
`pending_triage`. Author `max` as a literal in the condition `value` (or read it as
a triple; here literal `3`).

```json
{
  "id": "quest_fail_retry",
  "type": "expression",
  "enabled": true,
  "logic": "and",
  "rerun_on_recovery": true,
  "conditions": [
    { "field": "$entity.lifecycle.phase",  "operator": "eq", "value": "failed" },
    { "field": "quest.attempts.current",   "operator": "lt", "value": 3 }
  ],
  "on_enter": [
    {
      "type": "lifecycle_transition",
      "workflow": "quest",
      "phase": "posted",
      "reason": "auto-retry (attempts < max)",
      "set": {
        "attempts_current": { "op": "increment" },
        "claimed_by": ""
      }
    }
  ]
}
```

```json
{
  "id": "quest_fail_triage",
  "type": "expression",
  "enabled": true,
  "logic": "and",
  "rerun_on_recovery": true,
  "conditions": [
    { "field": "$entity.lifecycle.phase",  "operator": "eq",  "value": "failed" },
    { "field": "quest.attempts.current",   "operator": "gte", "value": 3 }
  ],
  "on_enter": [
    { "type": "lifecycle_transition", "workflow": "quest", "phase": "pending_triage",
      "reason": "retries exhausted (attempts >= max)" }
  ]
}
```

**Race analysis (asked explicitly).** This is **two rules gated by `lt`/`gte`**.
Both evaluate on the same watch event when the phase becomes `failed`, both read the
same `attempts.current` from the same snapshot, and `lt`/`gte` **partition** the
value space — so **exactly one** condition is true and **exactly one** action runs;
there is no double-transition. The `increment` runs *inside* `TransitionWith`'s
CAS loop (`manager.go:607-704`): the field is read from the projection, incremented,
diffed, and written with `ExpectedRevision`; a concurrent write re-reads and
re-applies (`manager.go:696-699`). So the counter is monotonic and the
increment+branch is **safe** for a single entity. `claimed_by: ""` clears the
assignment atomically with the same write. (Reach `failed` via a
`lifecycle_transition` on a `command=fail`, **not** `Manager.Fail` — `Fail`
hardcodes a *terminal* `"failed"` phase, `manager.go:885-899`, whereas here `failed`
must have out-edges to `posted`/`pending_triage`.)

### 6e. mechanics stub: on entering `in_review` → `completed`, set skip-verdict

Purely phase-triggered auto-advance (mechanics disabled). Idempotent: `on_enter`
fires once on entering `in_review`; the transition flips phase to terminal
`completed`, so it never re-enters. No command to clear.

```json
{
  "id": "quest_review_skip_mechanics_disabled",
  "type": "expression",
  "enabled": true,
  "logic": "and",
  "rerun_on_recovery": true,
  "conditions": [
    { "field": "$entity.lifecycle.phase", "operator": "eq", "value": "in_review" }
  ],
  "on_enter": [
    {
      "type": "lifecycle_transition",
      "workflow": "quest",
      "phase": "completed",
      "reason": "review skipped (mechanics disabled)",
      "set": { "verdict": "review-skipped-mechanics-disabled" }
    }
  ]
}
```

---

## Appendix — where the source lives (quick index)

| Concern | File:line |
|---|---|
| Component registration name `rule-processor` | `processor/rule/factory.go:14-25` |
| `deps.LifecycleManager` auto-propagation | `processor/rule/factory.go:102-110`, `component/dependencies.go:97` |
| lifecycle actions require Manager (loud-fail) | `processor/rule/actions_lifecycle.go:166-171` |
| graph-ingest mutation subjects | `pkg/lifecycle/graph_emit.go:77-80` |
| Manager opens `ENTITY_STATES` | `pkg/lifecycle/manager.go:332`, `graph/constants.go:6` |
| semdragons bucket == `ENTITY_STATES` | `semdragon/domain/config.go:160-161` |
| Rule config keys | `processor/rule/config.go:14-71` |
| Watcher hardcodes `ENTITY_STATES` | `processor/rule/entity_watcher.go:51-58` |
| Condition struct | `processor/rule/expression/types.go:10-33` |
| Phase-gate resolution `$entity.lifecycle.*` | `processor/rule/expression/evaluator.go:184-198`, `lifecycle_substitution.go:50-61` |
| Bare-predicate triple read | `processor/rule/expression/evaluator.go:392-401` |
| Operator registration set | `processor/rule/expression/evaluator.go:22-52`, `types.go:119-147` |
| AND/OR + expression-rule `and` default | `evaluator.go:148-169`, `expression_factory.go:59-62` |
| Action struct (`lifecycle_transition` fields) | `processor/rule/actions.go` (Type/Workflow/Phase/Set/Reason/When docs), constants `:68-76` |
| `set` default-deny + typed ops | `processor/rule/actions_lifecycle.go:56-81`, `:255-378`; `manager_query.go:456-479` |
| `$entity.triple.<pred>` substitution | `processor/rule/execution_context.go:289-299` |
| loud-fail on phase/reason/workflow | `processor/rule/actions_lifecycle.go:212-218` |
| TransitionWith diff + replace-by-predicate | `pkg/lifecycle/manager.go:645-683`, `diffProjectedTriples:724-758` |
| graph-ingest remove-then-add (CAS) | `processor/graph-ingest/mutations.go` (`handleEntityUpdateWithTriplesCAS`) |
| edge lists on_enter/on_exit/while_true/on_recovery | `processor/rule/rule_factory.go:26-36`, `stateful_evaluator.go:280-325`, `:426-437` |
| self-write guard | `processor/rule/processor.go:1356-1371`, `message_handler.go:250-256`, `triple_mutator.go:89-91` |
| reference command producer (delta write) | `cmd/e2e-semstreams/mission/command.go:69-80,135-139` |
| reference lifecycle rule | `configs/rules/lifecycle/01-mission-launch.json` |
| semdragons full-Put pattern (to migrate) | `semdragon/graphclient.go:79-92,112-157` |
