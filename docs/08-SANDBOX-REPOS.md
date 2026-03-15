# Sandbox Repos

Semdragons uses a sandbox container for agent code execution and artifact management.
Real git repositories are bind-mounted from the host into the sandbox. Each quest gets
its own git worktree branching from the target repo's main, so agents work in isolated
branches. Only boss-battle-approved work merges to `main`, creating a quality gate
before code enters the knowledge graph via semsource.

## Contents

- [Overview](#overview)
- [Quest → Repo Flow](#quest--repo-flow)
- [Docker Volume Topology](#docker-volume-topology)
- [Safety Model](#safety-model)
- [Sandbox API Endpoints](#sandbox-api-endpoints)
- [Semsource Integration](#semsource-integration)
- [Artifact Predicates](#artifact-predicates)
- [Key Design Decisions](#key-design-decisions)

---

## Overview

```
HOST FILESYSTEM (bind mount)
  /repos/semstreams/     ← ~/Code/c360/semstreams
  /repos/semsource/      ← ~/Code/c360/semsource

NAMED DOCKER VOLUME
  /workspace/{quest-id}/ ← git worktree branched from target repo

FLOW:
  1. Quest brief declares repo    → "repo": "semstreams"
  2. CreateWorkspace               → git branch + worktree from repo's main
  3. Agent works in worktree       → commits via git_operation tool
  4. Boss battle approves          → POST /workspace/{id}/merge-to-main
  5. Semsource watches main        → indexes approved changes → graph entities
```

## Quest → Repo Flow

1. **Quest creation**: Quest brief must include a `repo` field naming the target
   repository. If omitted, the quest is escalated to the DM for clarification.

2. **Workspace creation**: When questbridge dispatches a quest to the agentic loop,
   it calls `POST /workspace/{questID}?repo={repoName}` on the sandbox. The sandbox:
   - Validates the repo exists at `/repos/{repoName}` with a `.git/` directory
   - Creates branch `quest/{instanceID}` from `main`
   - Creates a git worktree at `/workspace/{questID}` from that branch
   - Configures git identity in the worktree

3. **Agent execution**: The agent works in `/workspace/{questID}/`. It can:
   - Read/write files via sandbox file endpoints
   - Run commands, tests, builds via sandbox exec endpoint
   - Commit changes via the `git_operation` tool (add, commit, status, diff, log)
   - The agent sees the full codebase from the branch point (read access to all repo files)

4. **Boss battle review**: The evaluator reads quest artifacts from the sandbox via
   `ListWorkspaceFiles` + `ReadFile` to include in the judge prompt.

5. **Merge to main**: On boss battle victory, the backend calls
   `POST /workspace/{questID}/merge-to-main`. The sandbox:
   - Auto-commits any uncommitted work in the worktree
   - Switches the repo to main
   - Merges the quest branch with a descriptive commit message
   - Returns the merge commit hash and list of changed files
   - On conflict: returns 409 and aborts the merge

6. **Semsource indexing**: Semsource watches each repo's main branch. After a
   successful merge, it picks up the changed files and creates/updates graph entities
   (AST, docs, config). The `ArtifactsIndexed` flag on the quest is set to true once
   indexing completes, unblocking any dependent quests.

7. **Cleanup**: When a quest workspace is deleted (`DELETE /workspace/{questID}`),
   the sandbox removes the worktree and deletes the quest branch.

## Docker Volume Topology

| Mount | Container | Path | Mode | Purpose |
|-------|-----------|------|------|---------|
| `workspaces` (named) | sandbox | `/workspace` | RW | Per-quest worktrees |
| Host repos (bind) | sandbox | `/repos/{name}` | RW | Real git repositories |
| Host repos (bind) | semsource | `/sources/{name}` | RO | Semsource watches main |

**Backend mounts zero workspace volumes.** It communicates with the sandbox exclusively
via `SANDBOX_URL` (HTTP API).

## Safety Model

| Layer | Protection |
|-------|-----------|
| **HTTP path validation** | `resolveQuestPath()` constrains agents to `/workspace/{quest-id}/`. Agents cannot reach `/repos/`. |
| **Git operation restrictions** | `git_operation` tool blocks push, pull, rebase, reset, force. Agents can only: init, status, diff, log, add, commit, branch, checkout, show. |
| **Worktree isolation** | Each quest gets its own branch and worktree. Agents never touch `main` directly. |
| **Merge gate** | Only boss battle victory triggers `merge-to-main`. This is a privileged sandbox server endpoint, not an agent tool. |
| **Rollback** | If a merge goes wrong, `git revert` on main is trivial. |

## Sandbox API Endpoints

### Workspace Management

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/workspace/{id}?repo={name}` | POST | Create workspace (worktree from repo) |
| `/workspace/{id}` | DELETE | Remove workspace (worktree + branch) |
| `/workspace/{id}` | GET | List all files recursively |

### File Operations

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/file` | PUT | Write file (JSON body) |
| `/file?quest_id=&path=` | GET | Read file |
| `/list` | POST | List directory entries |
| `/search` | POST | Grep-style search |

### Git Operations

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/repos` | GET | List available repos |
| `/workspace/{id}/git/log?limit=N` | GET | Commit history |
| `/workspace/{id}/git/diff` | GET | Uncommitted changes |
| `/workspace/{id}/git/status` | GET | Working tree status |
| `/workspace/{id}/git/commit-all` | POST | Stage all + commit |
| `/workspace/{id}/merge-to-main` | POST | Merge quest branch to repo's main |

### Command Execution

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/exec` | POST | Run shell command in workspace |

## Semsource Integration

Semsource watches each repo's main branch using its `repo` source type:

```json
{
  "namespace": "workspace",
  "sources": [
    { "type": "repo", "path": "/sources/semstreams", "branch": "main", "watch": true },
    { "type": "repo", "path": "/sources/semsource", "branch": "main", "watch": true }
  ]
}
```

The `repo` source type expands into git + ast + docs + config handlers. It only watches
`main` — unapproved quest work on branches never enters the graph.

## Artifact Predicates

| Predicate | Type | Description |
|-----------|------|-------------|
| `quest.context.repo` | string | Target repository name |
| `quest.artifacts.merged` | string | Merge commit hash after boss battle victory |
| `quest.artifacts.indexed` | bool | True when semsource has processed merged artifacts |
| `quest.relationship.produced` | []string | Semsource entity IDs produced by this quest |

## Key Design Decisions

1. **Sandbox owns git** — The backend never touches git. All git operations happen
   inside the sandbox container via HTTP API. This eliminates the need for shared
   Docker volumes between backend and sandbox.

2. **Real repos, not synthetic workspaces** — Agents work on actual repositories
   (bind-mounted from host), not artificial sandboxes. This means agents see real
   codebases with full git history.

3. **Quest brief must declare repo** — No automatic inference. If the repo isn't
   specified, the quest is escalated. This prevents accidental writes to wrong repos.

4. **Worktrees for concurrency** — Git worktrees allow multiple agents to work on
   the same repo simultaneously without conflicts. Each agent has its own branch
   and working directory.

5. **Quality gate via merge** — Only boss-battle-approved work enters main. This
   ensures the knowledge graph (via semsource) only contains vetted code.

6. **Single volume for workspaces** — All quest worktrees live on one named Docker
   volume. Simple to manage, backup, and clean up.
