# Semdragons Project Context

## Purpose

Semdragons is an agentic workflow coordination product modeled as a tabletop
RPG, built on the SemStreams framework. Work items are quests, agents are
adventurers who earn XP, quality reviews are boss battles, and trust tiers are
derived from demonstrated competence. Initial domain focus is software
development.

Core design commitment (2026-07 revival): the **two-loop principle**. Soft
signals (LLM judgment, peer review) may steer routing and prompts only. Hard
signals (executed verification in hermetic environments, outcome-lagged
results) are the only inputs allowed to move trust tiers or gate capabilities.

## Product Boundary

- **Semdragons owns** game mechanics: quest semantics, trust tiers and
  progression, boss battles (verification policy), parties/guilds/boids,
  the DM layer, and the dashboard.
- **The dev substrate owns** (extraction in progress; see
  `docs/adr/` and the dev-substrate audit): the sandbox service (worktrees,
  merge gate, exec), environment manifests / battlefield prep, and hermetic
  verification plumbing. Semdragons consumes it; it must not re-grow a
  product-private sandbox.
- **SemStreams owns** the governed substrate: lifecycle workflows
  (`pkg/lifecycle`), the rule engine, gated-DAG dispatch, agentic-loop/model,
  graph state (ENTITY_STATES), and observability. Semdragons must not
  re-implement state machines the framework provides.
- **SemSource owns** source indexing and the fusion query surface
  (`code_context` / `code_impact` / `code_search`). Agent-facing graph tools
  must use the fusion surface, never entity-ID GraphQL.

## OpenSpec Conventions

- `specs/<capability>/spec.md` — current truth per capability. Created lazily:
  seed a spec when a change first touches that area. Candidates:
  `quest-lifecycle`, `trust-progression`, `verification`, `sandbox-contract`.
- `changes/<change-id>/` — proposal, design, tasks, and spec deltas for every
  non-trivial change. Archive on completion (`openspec archive`).
- `docs/adr/` — decision records only: irreversible choices and cross-repo
  contracts. ADRs never serve as current-truth reference; when an ADR's
  content becomes standing behavior, it belongs in a spec.
- Legacy `docs/0X-*.md` design docs are being retired into specs as areas are
  touched. Tutorial/runbook content stays in `docs/`.

## Graph-Primary Convention (family-wide)

Across the sem* family the **graph is the primary source of truth**; OpenSpec
files are **hydrated artifacts** — projections that can be rendered from graph
state (see semspec's `render_openspec`) and ingested back on brownfield repos
(OpenSpec-read capability). SemSpec and SemTeams are actively converging on
this model.

Practical consequence for this repo: author `openspec/` content as plain files
today, but write it knowing it will be graph-ingested — stable change-ids and
capability names, requirements as discrete statements with scenarios, no
load-bearing prose that only makes sense as a whole file. When the family's
graph tooling matures, this directory becomes a render target, not the master
copy.

## Development Notes

- Module: `github.com/c360studio/semdragons`, pinned to semstreams
  `v1.0.0-beta.21` (migration to current semstreams is the first change).
- Task runner: `task --list`. Unit tests `task test`; integration
  `task test:integration` (Docker); e2e via mock LLM `task e2e`.
- Real-LLM runs cost money — active monitoring protocol in `CLAUDE.md` is
  mandatory.
