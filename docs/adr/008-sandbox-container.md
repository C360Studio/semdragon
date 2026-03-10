# ADR-008: Sandbox Container for Agent Code Execution

## Status: Accepted

## Context

Agents execute quests that often require writing code and running tests. Currently, agents write code inline in their `submit_work_product` deliverable text, and the LLM judge evaluates the text without actually executing anything. This creates three problems:

1. **No execution validation** — agents submit code that may not compile or pass tests
2. **Structural checklist is theater** — "tests-included" checks if the judge *thinks* tests are present in a text block, not whether test files exist or pass
3. **Unstructured deliverables** — code, tests, and explanation are crammed into a single text dump that the judge must parse

The existing tool set already has `write_file`, `read_file`, `run_tests`, `patch_file`, etc. The issue isn't missing tools — it's that these tools operate on the host filesystem with no isolation and no per-quest workspace.

## Decision

### Shared Sandbox Container

Add a Docker container sidecar with Go, Java, and Node.js/TypeScript toolchains. Each quest gets an isolated workspace directory at `/workspace/{quest_id}/`. The sandbox exposes an HTTP API for file operations and command execution.

### Existing Tools Proxy to Sandbox

No new tools needed. When `sandbox_url` is configured, existing tool handlers (`write_file`, `read_file`, `run_tests`, etc.) proxy operations to the sandbox API instead of the local filesystem. Agents use the same tools they already have — the sandbox is transparent infrastructure.

### submit_work_product Becomes a Completion Signal

With a sandbox workspace, `submit_work_product` shifts from "paste your entire deliverable as text" to "I'm done, review what's in my workspace." The agent can include a summary, but the actual code and tests live as real files in the workspace. The boss battle judge reads workspace files to evaluate the deliverable.

### Sandbox HTTP API

Internal Docker network only, no authentication needed.

| Method | Path | Purpose |
|--------|------|---------|
| `PUT /file` | Write file to quest workspace |
| `GET /file` | Read file from quest workspace |
| `POST /exec` | Run command in workspace (returns stdout/stderr/exit code) |
| `POST /list` | List directory contents |
| `POST /search` | Search text within workspace |
| `POST /workspace/{quest_id}` | Create workspace (idempotent) |
| `DELETE /workspace/{quest_id}` | Cleanup workspace |
| `GET /health` | Health check |

### Tier-Gated Execution (Unchanged)

Tier enforcement stays in questtools — the sandbox is a dumb executor.

- **All tiers**: Read files from sandbox
- **Journeyman+**: `write_file`, `patch_file`, `create_directory`
- **Expert+**: `run_tests` (allowlisted commands), `lint_check`
- **Master+**: `run_command` (unrestricted — can blow the entire sandbox up)

### Timeouts

- Default: 30 seconds per command
- Overridable per tool call via `timeout_ms` parameter
- Max: 300 seconds for long-running test suites

### Workspace Lifecycle

1. **Create**: questbridge calls `POST /workspace/{quest_id}` before publishing TaskMessage
2. **Use**: Agent writes files and runs commands via existing tools
3. **Review**: Boss battle judge reads workspace files to evaluate deliverable
4. **Cleanup**: questbridge calls `DELETE /workspace/{quest_id}` on quest completion/failure
5. **Orphan cleanup**: Sandbox runs periodic cleanup (hourly) for workspaces older than 24 hours

## Architecture

```
┌─────────────────┐     HTTP API      ┌──────────────────────┐
│   questtools    │ ──────────────→   │   sandbox container  │
│  (tool handler) │                   │                      │
│                 │  write_file →     │  /workspace/{qid}/   │
│                 │  read_file  →     │  /workspace/{qid}/   │
│                 │  run_tests  →     │  /workspace/{qid}/   │
│                 │  run_command →    │  /workspace/{qid}/   │
│                 │  ← stdout/stderr  │                      │
│                 │  ← exit code      │  toolchain: go,      │
│                 │                   │  java, node/ts       │
└─────────────────┘                   └──────────────────────┘
```

### Docker Integration

- Sandbox service in `docker/compose.yml`
- Resource limits: 2 CPU cores, 4 GB memory
- Named volume for `/workspace`
- Backend gets `SANDBOX_URL=http://sandbox:8090` environment variable
- Non-root `sandbox` user (UID 1000) owns `/workspace`

## Consequences

### Positive

- Agents write real files and run real tests — no more code-in-text-block theater
- Structural checklist becomes meaningful — judge checks actual workspace files
- `submit_work_product` is clean — completion signal, not a code dump
- Process isolation — agent code can't affect NATS, API, or other processors
- Tier progression is meaningful — higher tiers unlock real execution
- No new tools to learn — existing tools just get a better backend

### Negative

- Added infrastructure (another container, ~2-3 GB image with all toolchains)
- Shared container means quests can theoretically interfere with each other (accepted risk — bad behavior gets bad reviews)
- No network isolation in Phase 1

### Future Phases

- **Phase 2**: Per-quest containers behind the same HTTP API contract
- **Phase 3**: Network isolation (Docker network rules for outbound access)

## Files Affected

### New

| File | Purpose |
|------|---------|
| `cmd/sandbox/main.go` | Entry point, CLI flags, HTTP server |
| `cmd/sandbox/server.go` | HTTP handlers |
| `cmd/sandbox/exec.go` | Command execution with timeouts |
| `cmd/sandbox/workspace.go` | Workspace directory lifecycle |
| `docker/sandbox.Dockerfile` | Multi-toolchain image |
| `processor/questtools/sandbox.go` | SandboxClient HTTP wrapper |

### Modified

| File | Change |
|------|--------|
| `processor/executor/tools.go` | Tool handlers proxy to sandbox when configured |
| `processor/questtools/config.go` | Add `SandboxURL` field |
| `processor/questtools/component.go` | Create SandboxClient in Start() |
| `processor/questbridge/config.go` | Add `SandboxURL` field |
| `processor/questbridge/handler.go` | Workspace create/cleanup lifecycle |
| `processor/bossbattle/evaluator.go` | Judge reads workspace files |
| `docker/compose.yml` | Add sandbox service |
| `config/*.json` | Add `sandbox_url` |
