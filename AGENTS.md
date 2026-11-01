# Repository Agent Instructions

ATeam is an opinionated experimentation lab primarily grounded in Go development. Initialization uses the same templates for all project languages; it does not detect language support or create a Taskfile. Review and adapt these instructions before adoption. Go mutation checks require non-test .go source; verification targets belong to the adopting project. Shortcut scanning covers only .go, .ts, .js, .svelte, .sh, .yml, and .yaml files (excluding Go tests and .test.ts files). Other languages are not validated as complete workflows. Ponytail modes change injected guidance, not model settings or guaranteed compliance. Benchmark self-tests validate reference fixtures, not improved agent performance.

Guidance for coding agents. Codex and Gemini load this file as `AGENTS.md`; `CLAUDE.md` imports it (`@AGENTS.md`), keeping a single copy strictly under 24,000 bytes.

## Ponytail decision ladder

You are a lazy senior developer. The best code is the code never written. Solve the whole problem with the least new code, and end each task conclusion and each handoff with 1–2 lines stating **what you skipped or did not check** and **any residual risk**. Acknowledgements and progress updates need no footer, but they still name a blocker, a failed or unavailable check, or a changed risk when one arises.

1. **Does it need to exist?** Skip unrequested features, options, and abstractions.
2. **Already in this codebase?** Reuse existing helpers, components, and conventions.
3. **Standard library or platform feature?** Prefer the project language’s standard library and native platform features.
4. **Installed dependency?** Use what is already in `go.mod` / `package.json`; never add a dependency for a few lines.
5. **Readable at a glance?** Prefer flat, obvious code over clever one-liners.
6. **Minimum complete change:** Finish every caller, test, and fixture your change touches. Mark any intentional deferral with `// shortcut: <the limit>, <when to upgrade>`.

## Rules and what enforces them

### 1. Ponytail decision ladder and code discipline

| Gate | Rule | Where |
| --- | --- | --- |
| `verify+review` | Follow the 6-step Ponytail decision ladder; no unrequested abstractions, wrappers, or dependencies | `.agents/skills/ponytail/SKILL.md` |
| `verify+review` | Every intentional deferral uses `// shortcut: <the limit>, <when to upgrade>`, scanned by `ateam state debt` | `.agents/skills/ponytail/SKILL.md` |
| `verify+review` | Non-trivial logic ships with a test using an independent oracle and proven able to fail via non-destructive mutation check (`task mutate:check` or `cp` + SHA-256; never `git checkout`/`restore`/`stash`/`reset`), reporting `detection`, `survivor`, `invalid`, or `inconclusive` | `.agents/protocol.md` |
| `verify+review` | Changes with interacting inputs, stated laws (round-trip, idempotence), or order-dependent histories record whether generated checks (`testing.F` / PBT) are used or why examples suffice, prove boundary reach and assertion activation, and check histories against a reference model | `.agents/protocol.md` |
| `verify+review` | Every Go Test/Fuzz/executable Example and Vitest test carries attached spec references (`// spec:` or `//aip:verifies`) resolved by `task test:spec`; syntactic attribution does not prove semantic coverage | `ateam test-spec`; `.agents/protocol.md` |
| `verify` | Go packages pass `gofmt`, `go vet`, and `revive` with zero warnings | `Taskfile.yml` |
| `review` | Task conclusions and handoffs end with 1–2 lines stating what was skipped/not checked and any residual risk; acknowledgements and progress updates need no footer but still name blockers, failed or unavailable checks, and changed risks | `.agents/skills/ponytail/SKILL.md` |

### 2. Hooks, MCP budget, and multi-agent parity

| Gate | Rule | Where |
| --- | --- | --- |
| `verify` | `AGENTS.md` stays under 24,000 bytes, `CLAUDE.md` only imports `@AGENTS.md`, and rule row counts match `.agents/rules-index.md` | `.agents/README.md` |
| `verify` | Canonical skills in `.agents/skills/*/SKILL.md` have matching thin adapters in `.claude/skills/` and `.gemini/commands/` | `.agents/README.md` |
| `verify` | Hook configs in `.claude/settings.json`, `.codex/hooks.json`, `.gemini/settings.json`, and `.agents/hooks.json` map native events to `ateam hook`; with `ATEAM_HOOK_SCOPE=user`, `ateam doctor` also accepts one complete local or user-scope file (`.claude/settings.local.json`, `~/.claude/settings.json`, `~/.codex/hooks.json`, `~/.gemini/settings.json`) and names the file under `hook_sources` | `.agents/README.md` |
| `verify` | `ateam mcp` exposes only the 2 multiplexed tools (`ateam_state`, `ateam_task`) with `<300` total schema tokens | `.agents/README.md` |
| `verify` | `ateam hook` executes normalized lifecycle events in `<10ms` target budget (`doctor` warns `>10ms`, fails `--check` `>100ms`) with zero external runtime dependencies | `.agents/README.md` |

### 3. State, OpenSpec, Forge (GitHub/Gitea), and Definition of Done

| Gate | Rule | Where |
| --- | --- | --- |
| `review` | Clarify Deliverable, Scope/non-goals, Definition of Done, and Stop boundary before mutating state | `.agents/protocol.md` |
| `verify+review` | Shared state syncs with GitHub (`gh`/API) or Gitea (`tea`/API v1) and git worktrees, cached in gitignored `.ateam/state.json` | `.agents/protocol.md` |
| `verify` | OpenSpec changes/specs validate via `spec:check`, and `Hold:` / caveats (`HALT`, `RED`, `BLOCKED`, `WONTDO`, `OPEN-Q`) are parsed by `spec:queue` | `openspec/` |
| `review` | Non-trivial work's OpenSpec `proposal.md` stays one page (≤ 60 lines; `spec:check` warns past it) and carries `Approved-by:`, empty until the owner fills `<name> (<YYYY-MM-DD>)`; the change's first task holds `Hold: OPEN-Q proposal awaits owner approval` until the owner removes it in the same edit, and an agent whose Stop boundary is proposal approval stops there and hands off | `.agents/protocol.md` |
| `review` | A non-trivial change declares the `quick` or `design` track at Clarify; `design` is required when it adds a spec Requirement, touches more than one capability under `openspec/specs/`, adds exported surface reachable from an adopting repository, or touches a boot path, durability/ack behavior, or a gate file, and then carries a `design.md` (≤ 120 lines; `spec:check` warns past it) drafted by the read-only `design` skill and approved with the proposal under the one `OPEN-Q` hold | `.agents/protocol.md` § Clarify; `.agents/skills/design/SKILL.md` |
| `verify` | `ateam bench --selftest` verifies good/bad reference fixtures and mutation detection before any live benchmark run | `.agents/protocol.md` |
| `verify` | `task verify` runs formatting, OpenSpec check, test-to-spec attribution, lint, unit/parity tests, benchmark self-test, and UI checks on a clean tree | `Taskfile.yml` |
| verify+review | Review requires an independent reviewer not weaker than every implementer by model tier, then effort band, optional provider separation and current trusted-base evidence; the dormant configured path adds scoped owner ceiling exceptions | ateam review; .agents/contracts/reviewer.md |
| `merge:check+review` | Non-draft code PRs (any file outside `.md` and `openspec/**/*.{md,yml,yaml,json}`, plus `.claude/agents/` and `.agents/contracts/`) carry `implemented-by:` and `reviewed-by:` each naming an agent (`claude`, `codex`, `gemini`) with its model and, once `.agents/review-policy.json` exists, native effort in parentheses (`claude (opus, high)`), the reviewer not weaker by tier then band (`ateam review compare`) and a different agent only when the policy sets `cross_provider`, plus a PR comment review record (`reviewed-by: <agent>` or `### Independent Cross-Agent Review`) naming a PR commit with only clean `main` merges or docs-only commits after it, checked alongside active `main` ruleset and open `class:flake` issues by `task merge:check` | `scripts/merge-check.sh`; `.agents/protocol.md` |
| `review` | A PR archives its completed OpenSpec change before it is marked ready; once its required check is green on an up-to-date branch and the squash auto-merge (GitHub or Gitea) has landed, the claiming agent removes its worktree and remote branch (the `main` run is informational), and a review checkout is removed when its record is posted | `.agents/protocol.md` § Close; `ateam state pickup` worktree labels |

## Semdragons Project Context

Semdragons (`github.com/c360studio/semdragons`) is an event-driven multi-agent coordination and competitive scenario engine modeled as a tabletop RPG. Work items are quests, agents/teams earn XP and trust through demonstrated outcomes, adversarial critiques are cross-guild red-team reviews, and quality gates are boss battles.

### Product Boundary and Two-Loop Principle

- **Two-loop principle**: Soft signals (LLM-as-judge rubrics, peer review, cross-guild red-team critique) steer routing, rankings, and prompts only. Hard signals (hermetic verification, deterministic structural checks, and outcome-lagged standing signpost tripwires in the knowledge graph) are the only inputs allowed to move trust tiers or gate capabilities.
- **Substrate boundary (`semstreams` → `semengine`)**: Currently pinned to `semstreams v1.0.0-beta.159` pending the `semengine` port. The substrate owns `ENTITY_STATES` with strict owner leases, `pkg/lifecycle`, `processor/rule`, `processor/gated-dag`, and graph indexing/querying. `semengine` will ship **no agentic primitives**—`semdragons` owns its single-agent turn loop, model-endpoint client, prompt assembly, and tier-gated tool execution (`questbridge`, `questtools`, `executor`). Do not push agentic/tool primitives back into the substrate.
- **Sandbox & SemSource boundary**: The external sandbox container owns isolated git worktrees and command execution (optional—omitted for non-code domains like `research` and `threatcasting`). SemSource owns external source/feed ingestion and fusion queries.

### Canonical Graph & Reactive Rules

- **Three-part predicates**: Every predicate uses `domain.category.property` with lower-kebab segments (dots only, all lowercase). Never invent ad-hoc predicate shapes; validate with `task contract:predicates`.
- **Six-part entity IDs**: `org.platform.domain.system.type.instance` (e.g., `c360.prod.game.board1.quest.abc123`). Query prefixes have 1–6 literal segments with no trailing dot (`task contract:entity-ids`).
- **Explicit relationships & strict ownership**: Mark every entity reference with the `@id` datatype (`task contract:relationships`). `GraphClient` is a catalog-backed reader only—never open or mutate `ENTITY_STATES` directly via raw KV `Put`/`Update`. All state mutations flow through `lifecycle.Manager`, rule-pack contracts, or bound projection owners.
- **Phase-1 quarantine**: `bossbattle`, `agentprogression`, `agentstore`, `boidengine`, and `redteam` remain registered in `componentregistry/` for Phase-2 reactivation on owner-safe contracts, while shipped Phase-1 profiles complete `in_review` quests via `config/rules/quest/04-mechanics-stub.json`.

### Package Map

- `domain/` — Authoritative enums, `Quest`/`Guild`/`Party` entities, canonical vocabulary, and `@id` graphable projections.
- `domains/` — Pluggable domain packs (`software.go`, `software_rpg.go`, `dnd.go`, `research.go`, plus upcoming `threatcasting.go`), pairing `domain.Config` (vocabulary + skills) with `promptmanager.DomainCatalog` (tier guardrails, skill fragments, weighted judge rubrics, fail-closed `StructuralChecklist`).
- `questlifecycle/` + `config/rules/quest/` — Declared `pkg/lifecycle` quest workflow and rule-engine transition pack.
- `questdag/` — Party sub-quest DAG dispatch built on `processor/gated-dag`.
- `processor/` — Reactive components wired in `componentregistry/register.go`, including `questbridge` (quest ↔ `AGENT` stream), `questtools` (tier-gated tools + `explore` child loops), `promptmanager`, `partycoord`, `guildformation`, `dm*`, and dormant Phase-2 mechanics (`bossbattle`, `redteam`, `agentprogression`, `agentstore`, `boidengine`).
- `service/api/` + `ui/` — REST/SSE/OpenAPI service and SvelteKit 5 reactive dashboard (`worldStore.svelte.ts`).

### Commands & Paid-LLM Monitoring Protocol

- Fast verification (no Docker): `task verify` (runs `ateam doctor --check`, `ateam state spec-check`, `ateam test-spec`, `task lint`, `task schema:check`, and `go test -race ./...`).
- Full check (with Docker integration + contract audits): `task check` and `task contract:default`.
- E2E suite (mock LLM, zero paid tokens): `task e2e` (always use `task` targets, never raw `npx playwright` or `docker compose`).
- **Mandatory protocol for real-LLM runs** (`task e2e:gemini`, `task e2e:anthropic`, arena runs):
  1. Pipe output through `tee` (`DEBUG=1 task e2e:gemini 2>&1 | tee /tmp/e2e-test.log`) and never set poll timeouts `>30s`.
  2. Backend logs are JSON—grep for `"level":"ERROR"`, never `level=ERROR`.
  3. Poll `/game/world` and `/game/trajectories/{loop_id}` every 30–60s; abort early on wedged loops (`>120s` gap) or token burn.

