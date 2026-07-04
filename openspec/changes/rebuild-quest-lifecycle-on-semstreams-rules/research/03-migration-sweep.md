# 03 — Chronological Migration Sweep (semstreams beta.21 → beta.132)

**Scope (this cluster):** general framework surfaces semdragons imports —
`component`, `natsclient`, `message`, `model`, `service`, `config`,
`vocabulary`, `pkg/errs`, `pkg/types`, `pkg/context`, `metric`, plus the
root `types` package and `payloadregistry`/`payloadbuiltins`.
**Explicitly out of scope** (owned by sibling research docs):
`graph*` packages (→ `01-graph-ownership.md`) and the agentic wire —
`agentic`, `processor/agentic-loop`, `processor/agentic-model`
(→ `02-agentic-wire.md`).

**Method.** Read every migration doc in
`semstreams/docs/operations/` from `migration-beta20-to-beta21` through
`migration-beta68-to-beta70` (one row per breaking item below). Docs
exist for beta.20→45 and beta.68→70 only. For the two undocumented gaps
(**beta.45→68**, **beta.70→132**) I spot-checked the real beta.132
`.go` source signatures and swept `git log v1.0.0-beta.45..HEAD` for
conventional-commit breaking markers (`!:` / `[BREAKING]`). Every
"stable" claim below was confirmed against a beta.132 signature, not
inferred.

**Headline:** semdragons's general-framework surface is **almost
entirely compile-stable** across the whole jump. Nearly every documented
breaking change lands in subsystems semdragons does not import
(agentic-dispatch, graph-query, rule, sandbox client, agentic-tools).
The only breaking changes that reach this cluster are **behavioral, not
signature-level** (natsclient ADR-060 error encoding; vocabulary
re-Register semantics). The graph-ingest auto-vivify retirement
(ADR-055) is the single highest-runtime-risk item but is graph-ownership
territory — flagged here as a hand-off.

---

## 1. Chronological breaking-change table

Legend for "Touches semdragons?": **No** = different package / not
imported; **No (compile)** = signature stable, behavior differs;
**Handoff** = real impact but owned by graph-ownership or agentic-wire
cluster.

| Version step | Breaking / notable change | semstreams pkg | Touches this cluster? |
|---|---|---|---|
| beta.20→21 | Truncation self-heal; new `LoopFailedEvent.Error` diagnostic strings. Additive, `OutcomeTruncated` unchanged. | agentic-loop / agentic-model | No (agentic-wire) |
| beta.21→22 | New HTTP endpoint `POST /loops/{id}/approval`; `agent.approval_pending/_response` ports; `IdentityFromRequest`/`WithIdentity` helpers. Additive. | agentic-dispatch | No |
| beta.22→23 | **New `service.HTTPMiddleware` type + `Manager.UseHTTPMiddleware(mws…)`.** Additive; zero default middleware. | **service** | **Yes (additive only)** — new opt-in API; semdragons does not call it. `RegisterHTTPHandlers` signature unchanged. |
| beta.23→24 | Inline `<think>` tag routing to `ReasoningContent`. Additive. | agentic-model | No (agentic-wire) |
| beta.24→25 | Orphan tool-call synth-results; `RepairToolPairs()` public; approval-timeout sweeper. Additive. | agentic-loop | No (agentic-wire) |
| beta.25→26 | Profile/lessons personalization; `ProfileReader` gains `ReadProfileVersion`/`ReadLessons`; `agentic.ContextEvent.UserID` (omitempty). Additive. | agentic-dispatch / agentic-memory | No |
| beta.26→27 | `"type":"cron"` rule kind; `RULE_SCHEDULES` bucket; `$schedule.*`. Additive. | rule / cron | No |
| beta.29→30 | **`model.EndpointConfig.MaxOutputTokens`** (int, omitempty). Additive. | **model** | **Yes (additive field)** — no reader change required. |
| beta.31→32 | `$caller.*` substitution; `deny` action; `ExecutionContext.Caller` (nil default). Additive. | rule | No |
| beta.32→33 | New `model.Capability{IntentClassification,LayerNormalization,AnomalyReview}` constants. Additive. | **model** | **Yes (additive consts)** — unused by semdragons. |
| beta.34→35 | `ExecutorRegistry.RegisterExecutor` added; `ListTools()` dedups; `RegisterTool` kept. Internal behavior. | agentic-tools | No |
| beta.35→36 | Per-segment `$entity.<part>` substitutions; `read_loop_result` strips full entity ID. References `message.IsValidEntityID` (pre-existing). Additive. | rule / agentic-tools | No (message helper pre-exists) |
| beta.36→37 | **`sandbox.Client.CreateWorktree` gains `CreateWorktreeOptions` arg — BREAKING** for direct callers. Bash `read_only_paths`/`verify_clean`. | agentic-tools/sandbox | **No** — semdragons does **not** import `agentic-tools/sandbox`. |
| beta.37→38 | **`NewLLMAnswerSynthesizer`/`NewLLMClientAdapter` gain `timeout` arg — BREAKING** for direct callers. | graph-query / graph | **Handoff (graph-ownership)** — semdragons does not construct these. |
| beta.38→39 | `agentic-loop` stamps `tc.LoopID`/`Metadata["loop_id"]`. Behavioral, transparent. | agentic-loop | No (agentic-wire) |
| beta.40→41 | `rule.Action.ActionAllowlist`; `agentic.MetadataKeyDecideActionAllowlist`. Additive. | rule / agentic | No |
| beta.41→42 | `LoopEntity.Metadata["trace_id"]`; graph-query keepalive propagation. Additive/bugfix. | agentic-dispatch / graph-query | No |
| beta.42→43 | `model.NewHTTPClient` framework defaults tightened (IdleConnTimeout 90s→30s, MaxIdleConnsPerHost 2→1, HTTP/2 PINGs). **Behavioral.** `model.NewHTTPClient` signature unchanged. | **model** | **Yes (behavioral)** — semdragons does not call `NewHTTPClient` directly; inherited only via agentic-model/graph clients. Runtime tuning, no compile impact. |
| beta.43→44 | `rule.Action.MaxIterations`/`ID`; decide-tool SAP; **`vocabulary/agentic` predicate rename** `CoordinatorDecideSAPCoerced`→`CoordinatorDecisionSAPCoerced`. | rule / agentic / vocabulary/agentic | No — semdragons defines its own predicates; does not reference `vocabulary/agentic` constants. |
| beta.44→45 | **`AnswerSynthesizer.Synthesize` returns `SynthesisOutcome` instead of `(answer,model,err)` — BREAKING** for external implementers; `GlobalSearchResponse.Degraded`. | graph-query | **Handoff (graph-ownership)** — semdragons does not implement `AnswerSynthesizer`. |
| **beta.45→68** | **NO DOCS.** See §3 gap-risk. Git sweep: boid retirement, rule condition-evaluator unification, ADR-051 reasoning wire, ADR-055 graph-ingest must-exist. | multiple | See §3 |
| beta.68→70 | **`agentic.ToolCallFilter` interface + all wiring REMOVED — BREAKING.** New `agentic-loop` governance config + ports (test port pins 6→8 in / 7→8 out). `ExecutionContext.MessageData`/`Evaluation.MessageData` new fields. | agentic / agentic-loop / rule | **Handoff (agentic-wire)** — semdragons imports neither `agentic.ToolCallFilter` nor `processor/rule`. Confirmed not referenced. |
| **beta.70→132** | **NO DOCS.** See §3 gap-risk. Git sweep: natsclient ADR-060 error channel, vocabulary re-Register semantics, ADR-058 boot refactor, agentic operating-model removal. | multiple | See §3 |

---

## 2. Consolidated "must-fix for compilation" list

**Grouped by semdragons file, ordered by blast radius. The strong result
of the spot-check: there are no confirmed compile breaks in this
cluster.** All items below are either verified-stable or
behavioral-watch. Ordering reflects "where a break would surface if the
gap hides one."

### Tier 0 — Verified stable, highest fan-out (no action)

1. **`processor/*/register.go` + `component.go` (component registration, ~17 processors, 56 imports).**
   `component.Factory` = `func(json.RawMessage, Dependencies) (Discoverable, error)`,
   `RegisterWithConfig(RegistrationConfig{…})`, `GenerateConfigSchema`,
   `Discoverable` (Meta/InputPorts/OutputPorts/ConfigSchema/Health/DataFlow),
   `LifecycleComponent` (`Start(ctx) error` / `Stop(timeout) error`),
   `Dependencies.GetLoggerWithComponent` — **all present, signatures
   identical in beta.132** (`component/registry.go:35`, `:455`;
   `component/discovery.go:17`; `component/lifecycle.go:47`;
   `component/dependencies.go:121`). `Dependencies` gained fields
   (`LifecycleManager`, `StoreRegistry`, `Security`, `ToolRegistry`,
   `ComponentRegistry`) but every semdragons construction uses **keyed
   literals** → additive-safe.

2. **`processor/*/component_test.go` (natsclient test harness, 175 uses).**
   `NewTestClient(t, …TestOption) *TestClient` with public field
   `.Client *Client`; `WithKV`, `WithKVBuckets`, `WithFileStorage`,
   `WithStreams`, `TestStreamConfig` — **all present**
   (`natsclient/test_client.go:17,55,63,72,103,267`). semdragons test
   code **already uses the `testClient.Client` pattern** (e.g.
   `questbridge/component_test.go:43`), so it is written against the
   beta.132 shape. See §4.

3. **`processor/questbridge/register.go` etc. — `errs.Wrap` (110 uses).**
   `errs.Wrap(err error, component, method, action string) error` —
   4-arg, unchanged (`pkg/errs/errs.go:380`).

4. **Root triples/entity emission — `message.Triple` (299 uses),
   `message.NewBaseMessage` (15), `message.NewDecoder` (8).**
   `message.Triple` struct fields (Subject/Predicate/Object/Source/
   Timestamp/Confidence/Context) unchanged (`message/triple.go:30`);
   `NewBaseMessage(Type, Payload, source, …Option)` (`:121`);
   `NewDecoder(*payloadregistry.Registry) *Decoder` (`message/decoder.go:33`)
   — matches `message.NewDecoder(deps.PayloadRegistry)`.
   `message.IsValidEntityID` present (`:138`).

### Tier 1 — Verified stable, service/API surface (no action)

5. **`service/api/service.go` + `handlers.go` (service, 5 imports, ~200 symbol uses).**
   `Service` struct embeds `*service.BaseService` and overrides only
   `Start`/`Stop` → **inherits the newer `Health() health.Status` and
   `RegisterMetrics(metric.MetricsRegistrar)` interface methods for
   free** (the `Service` interface grew these mid-range;
   `service/base.go:471`). Verified present:
   `NewBaseServiceWithOptions`, `WithNATS`/`WithMetrics`/`WithLogger`
   (`service/base.go:100,125,132,139`), `NewServiceManager`/`Manager`
   (`service/service_manager.go:26,64`), `NewServiceRegistry`,
   `RegisterAll`, `Constructor`, `HTTPHandler`,
   `RegisterHTTPHandlers(prefix string, mux *http.ServeMux)`
   (`service/flow_service.go:237`), and the full OpenAPI builder set —
   `OpenAPISpec/PathSpec/OperationSpec/ParameterSpec/ResponseSpec/`
   `RequestBodySpec/Schema/TagSpec`, `RegisterOpenAPISpec`,
   `GetAllOpenAPISpecs`, `SchemaFromType`, `TypeNameFromReflect`
   (`service/openapi_types.go`, `service/openapi_registry.go`,
   `service/schema.go`). `service.Dependencies` fields used by
   `cmd/semdragons/main.go:462` (NATSClient, MetricsRegistry, Logger,
   Platform, Manager, ComponentRegistry) all present
   (`service/dependencies.go:27`).

6. **`cmd/semdragons/main.go` (config loading).**
   `config.Config`, `config.NewLoader()`, `config.Manager`,
   `config.NewConfigManager(cfg, natsClient, logger)`,
   `config.NewStreamsManager(natsClient, logger)` — signatures match
   (`config/config.go:46,319`; `config/manager.go:60`;
   `config/streams.go:85`).

### Tier 2 — Verified stable, small surfaces (no action)

7. **`model` package (real usage):** `EndpointConfig` (65),
   `Registry` (36), `RegistryReader` (18), `CapabilityConfig` (14),
   `DefaultsConfig` (9) — all present (`model/registry.go:191,319,337,
   346,354`). *Note:* the grep hits for `model.Client`/`model.NewClient`/
   `model.AdapterFor`/`model.Register` are **false positives** — they are
   `agenticmodel.*` (`processor/agentic-model`, agentic-wire cluster),
   matched because `agenticmodel.Client` contains the substring
   `model.Client`.
8. **`vocabulary`:** `Register(name, …Option)`, `WithDescription`,
   `WithDataType`, `Option` (`vocabulary/registry.go:286,103,111,100`) —
   used 100/63/100 times, stable. *Behavioral note in §3.*
9. **`payloadbuiltins` / `payloadregistry`:** `NewTestRegistry(testing.TB)`
   (`payloadbuiltins/testing.go:20`), `NewTestDecoder`
   (`:35`), `Register(*payloadregistry.Registry)`
   (`payloadbuiltins/register.go:40`), `payloadregistry.New()`,
   `Registry`, `Registration` (`payloadregistry/registry.go:63,44,69`).
10. **`pkg/errs`, `pkg/types`, `pkg/context`, root `types`, `metric`,
    `natsclient` client methods:** all verified — see §3 table and §4.
11. **`input/websocket.Register`, `gateway/graph-gateway.Register`:**
    standard `func(*component.Registry) error`
    (`input/websocket/register.go:45`, `gateway/graph-gateway/component.go:292`).

### Tier 3 — Behavioral watch (not a compile break; verify at runtime)

12. **`service/api/dm_session.go:218` — stale error-encoding assumption.**
    The comment/logic assumes `SubscribeForRequests` "encodes handler
    errors as plaintext bytes." **ADR-060 (`672d3e5d`, beta.45→132 gap)
    changed the reply to header-classified errors** (`X-Status` /
    `X-Error-Class` / `X-Error-Code`). The Go signatures of `Request`
    and `SubscribeForRequests` are **unchanged** → compiles clean, but
    the requester-side error-reply parsing may behave differently.
    Re-read `natsclient/errors.go` + `request.go:303` before relying on
    it. **Runtime, not compile.**

---

## 3. Gap-risk callouts (undocumented ranges)

No migration docs exist for **beta.45→68** or **beta.70→132**. I swept
`git log v1.0.0-beta.45..HEAD` for `!:`/`[BREAKING]` commits and
spot-checked each landing package's beta.132 source. Results:

### Breaking commits that DO reach this cluster

| Commit | Change | Package | Compile impact on semdragons |
|---|---|---|---|
| `672d3e5d` | **ADR-060: retire dual error channel → one typed RPC error** (header-classified error replies). Touched `natsclient/errors.go` (241 lines), `request.go` (19), `doc.go`. | **natsclient** | **None (signatures stable).** `Request(ctx,subject,data,timeout) ([]byte,error)` and `SubscribeForRequests(ctx,subject,handler func(ctx,[]byte)([]byte,error)) (*Subscription,error)` verified unchanged in beta.132. **Behavioral** — see §2 item 12. `RequestWithRetryClassified` is new/additive (`natsclient/errors.go:290`). |
| `df2f19ff` | **`vocabulary.Register` now amends instead of replaces** on re-Register (gh#410) — stops silent role loss. | **vocabulary** | **None (signature stable).** Behavioral only; semdragons's 100 `Register` calls are safe (amend is strictly more forgiving). |
| `c60b7f14` | `refactor(vocab)!: rename coordinator.* predicates to 3-segment form`. | vocabulary/agentic | **None** — semdragons uses its own `quest.*`/`agent.*`/`battle.*` predicates; references no `coordinator.*` constant. |
| `7c88a87e` | `retire boid subsystem [BREAKING]` (ADR-040). | processor/boid, rule | **None** — semdragons ships its **own** `processor/boidengine`; imports no semstreams boid package (grep confirmed zero hits). |
| `3632f94e` | `feat(rule): unify condition evaluator [BREAKING, ADR-041]`. | processor/rule | **None** — semdragons does not import `semstreams/processor/rule`. |
| `26b0c2ce`,`cfe2f39e`,`32a3aab6`,`d7d32ebc` | **ADR-058 boot refactor** — pprof/heartbeater/ownership extracted into Phase-B Services. | service (boot internals) | **None** — semdragons boots via `NewServiceManager`/`BaseService`, both verified stable. Watch for `Manager.StartAll` behavior differences at runtime, not compile. |

### Breaking commits that are HAND-OFFS (other clusters, flagged for awareness)

| Commit | Change | Owner cluster | Why it still matters to semdragons |
|---|---|---|---|
| `270ed60e` | **ADR-055 must-exist flip: retire `triple.add` auto-vivify [BREAKING]** — entities must exist before triples attach. | **graph-ownership** | **Highest runtime blast radius of the whole upgrade.** semdragons emits entities + triples pervasively (`message.Triple` ×299, `EmitEntity`/`EmitEntityUpdate`). If any semdragons path adds a triple to a not-yet-created entity, it will now **fail at runtime** instead of silently vivifying. Verify ordering of `EmitEntity` before dependent triple writes. Owned by graph-ownership doc `01`. |
| `9999b016` | `remove community SemanticProvider / semantic_edges [ADR-061]`. | graph-ownership | Only if semdragons reads those graph fields (unlikely). See doc `01`. |
| `4f9c4323` | **ADR-051: OpenAI Responses wire + unified `ReasoningRecord` [BREAKING]**. | agentic-wire | Affects model/reasoning wire types; see doc `02`. |
| `0bd1e880` | `agentic-dispatch! bare /activity loop_id + loop_completed event type`. | agentic-wire | Event-shape change on the dispatch path; see doc `02`. |
| `24b7e9bc` | `remove operating-model personalization layer` (→ semteams). | agentic-wire | Removes `ProfileReader`/lessons surface (added beta.26). semdragons never wired it. See doc `02`. |

### Packages that showed API drift on spot-check (surface, not yet a break)

- **`component.Dependencies`** grew several fields across the gap
  (`LifecycleManager` ADR-047, `StoreRegistry`/`StorageRegistry`
  ADR-063, `Security`). All **additive**; semdragons uses keyed
  literals and reads only NATSClient/PayloadRegistry/MetricsRegistry/
  Logger/Platform/ModelRegistry/ComponentRegistry/Manager — all still
  present. **Compile-safe, but this is the struct most likely to hide a
  future break** if a field semdragons reads is ever renamed.
- **`service.Dependencies`** likewise grew (`LifecycleManager`,
  `StoreRegistry`, `ServiceManager`). Additive; keyed construction safe.
- **`service.Service` interface** grew `Health() health.Status` and
  `RegisterMetrics(metric.MetricsRegistrar) error` mid-gap. **Only
  compile-safe because `service/api.Service` embeds `*BaseService`.**
  Any hand-rolled `service.Service` impl in semdragons would break —
  none found.

### Where compile errors will most plausibly surface unexpectedly

1. **Anything constructing a struct literal positionally** against
   `component.Dependencies` / `service.Dependencies` (none found, but the
   gap added the most fields here).
2. **`natsclient` request/reply error handling** — signatures stable,
   but ADR-060 semantics differ (Tier 3 item 12).
3. **Runtime, not compile: ADR-055 must-exist triples** — the single
   item most likely to break a green build at *runtime*. Coordinate with
   graph-ownership.

---

## 4. Test-harness changes (`natsclient.NewTestClient` / `WithKV` / `WithMessageLogger`)

semdragons integration tests depend on this surface heavily (175 ×
`NewTestClient`, 175 × `WithKV`, 175 × `WithFileStorage`, 156 ×
`WithKVBuckets`, 8 × `WithStreams`/`TestStreamConfig`).

**Verified in beta.132 (`natsclient/test_client.go`):**

| Symbol | beta.132 signature | Status |
|---|---|---|
| `NewTestClient` | `func NewTestClient(t testing.TB, opts ...TestOption) *TestClient` (`:267`) | **Stable.** Returns `*TestClient`, **not** `*Client`. |
| `TestClient` | struct with **public field `Client *Client`** ("Drop-in replacement", `:17-23`) | **Stable.** |
| `TestOption` | `func(*testConfig)` (`:44`) | **Stable.** |
| `WithKV` | `func WithKV() TestOption` (`:55`) | **Stable.** |
| `WithKVBuckets` | `func WithKVBuckets(buckets ...string) TestOption` (`:63`) | **Stable.** |
| `WithFileStorage` | `func WithFileStorage() TestOption` (`:103`) | **Stable.** |
| `WithStreams` / `TestStreamConfig` | `func WithStreams(streams ...TestStreamConfig) TestOption` (`:72`); struct `{Name string; Subjects []string}` (`:38`) | **Stable.** |
| `WithBucketPrefix`, `WithJetStream`, `WithNATSVersion`, `WithTestTimeout`, `WithStartTimeout` | present (`:55`–`:118`) | Stable (unused by semdragons). |

**Critical, and reassuring:** semdragons's test code is **already
written against the beta.132 shape** — it does
`testClient := natsclient.NewTestClient(t, …)` then
`client := testClient.Client` and passes `client` into
`component.Dependencies{NATSClient: client}` (verified at
`questbridge/component_test.go:36-48`, `dmsession/component_test.go:30-36`,
7+ sites). So **no test-harness migration is needed** for `NewTestClient`
/ `WithKV` / `WithFileStorage` / `WithStreams` / `TestStreamConfig`.

**`WithMessageLogger` — does NOT exist in `natsclient` at beta.132**
(grep returns zero). semdragons **never calls it** in `.go` files (zero
hits). The root `CLAUDE.md` shows `natsclient.NewTestClient(t, …,
natsclient.WithMessageLogger(os.Stdout))` as a debugging example — that
snippet is **aspirational/incorrect documentation**, not a real
dependency. No action required for the upgrade; optionally correct the
CLAUDE.md example (message-logger is a `service.MessageLogger` /
SSE-endpoint concern, not a `natsclient` test option).

**Other `natsclient` symbols semdragons uses — all verified present at
beta.132:** `Client`, `NewClient(urls, …ClientOption)`
(`client.go:147`), `NewSubject[T any](pattern)` (`typed.go:104`),
`TraceContext`/`NewTraceContext`/`ContextWithTrace`/`TraceContextFromContext`
(`trace.go:33,68,47,41`), `KVStore.KeysByPrefix(ctx, prefix)`
(`kv.go:439`), `ErrKVRevisionMismatch` (`kv.go:545`), `ErrNotConnected`
(`client.go:53`), `WithToken`/`WithCredentials` (`options.go:142,133`),
`Subscription` (`client.go:657`), `StreamConsumerConfig` (`stream.go:19`),
`SubscribeForRequests` (`request.go:303`). semdragons calls **no**
`Client.WatchKV` directly (0 hits) — KV watching goes through the
`graph` package (graph-ownership cluster).

---

## Appendix — semdragons → semstreams import inventory (this cluster)

From `grep -rhoE 'github.com/c360studio/semstreams/[a-zA-Z0-9/_-]+'`
(counts = distinct-line occurrences), **owned by this doc**:

```
56  component        20  model             3  pkg/context (pkgcontext)
48  message          12  pkg/types          1  vocabulary
46  natsclient        6  payloadbuiltins    1  metric
36  pkg/errs          5  service            1  pkg/retry
                      4  config             3  types (root)
                      3  payloadregistry    1  input/websocket
                                            1  gateway/graph-gateway
```

Owned by **sibling docs, not here**: `graph` (52 → `01`), `agentic` (30),
`processor/agentic-model` (4), `processor/agentic-loop` (1) (→ `02`),
`processor/graph-*` (→ `01`).
