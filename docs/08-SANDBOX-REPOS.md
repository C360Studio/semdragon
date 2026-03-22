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
- [Bind-Mounting Local Repos](#bind-mounting-local-repos)
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
  3. Agent works in worktree       → commits via bash (git commands)
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
   - Read/write files, run commands, tests, and builds via the `bash` tool
   - Commit changes using `git` commands inside `bash` (add, commit, status, diff, log)
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

## Bind-Mounting Local Repos

Users can mount their own local repositories into the sandbox for agents to work on.
This is the intended workflow for development teams using semdragons on their own code.

### Setup

1. Clone your repo(s) on the host:
   ```bash
   mkdir -p ~/repos
   git clone https://github.com/your-org/your-project ~/repos/your-project
   ```

2. Set `REPOS_DIR` to point at your repos directory:
   ```bash
   export REPOS_DIR=~/repos
   docker compose -f docker/compose.yml up -d
   ```

3. Create quests with the `repo` field matching the directory name:
   ```json
   { "title": "Add input validation", "repo": "your-project" }
   ```

4. Agents work in a git worktree branched from your repo's `main`. Approved work
   merges back — changes appear in `~/repos/your-project` on the host.

### What agents can see

Agents work in `/workspace/{quest-id}/`, which is a git worktree of your repo. They
have **full read access to every file in the repo** at the branch point. This includes:
- All source code, configuration files, and documentation
- Git history (via `git log`, `git blame`, etc.)
- Any secrets committed to the repo (`.env` files, API keys, credentials)

### Risks

| Risk | Description | Mitigation |
|------|-------------|------------|
| **Secret exposure** | Agents see all files in the repo, including committed secrets. LLM providers receive file contents as context. | Remove secrets before mounting. Use `.gitignore` and `git-secrets`. Never commit credentials to repos agents will access. |
| **Code exfiltration** | Agents have network access (for `http_request` and `web_search`). A misbehaving agent could send repo contents to external URLs via `bash("curl ...")`. | Review quest outputs. For sensitive repos, disable `web_search` and restrict `http_request` URLs. Network policy (future) would allow blocking outbound traffic. |
| **Host filesystem writes** | `/repos` is mounted RW. The sandbox server writes to it during `merge-to-main`. A compromised sandbox server (not just an agent) could write arbitrary files to the host. | The merge endpoint only writes via `git merge`. Agents cannot reach `/repos/` directly — `resolveQuestPath` constrains API access to `/workspace/{quest-id}/`. Symlink escape protection prevents indirection. |
| **Cross-repo access** | All repos are mounted under `/repos/`. An agent running `bash` in their workspace cannot directly access `/repos/other-repo`, but the sandbox server has access. | Agents work in worktree directories, not `/repos/`. The `bash` command runs with `Cmd.Dir` set to the quest workspace. However, absolute paths in bash commands (`cat /repos/other/secret.key`) would work inside the container. |
| **Concurrent modifications** | If you edit files on the host while agents are working in worktrees of the same repo, git may encounter conflicts. | Avoid editing files agents are working on. Use separate branches. The worktree model isolates agent work from host changes on `main`. |

### Recommendation

For sensitive codebases, create a **dedicated clone** for semdragons rather than
mounting your working copy. This prevents agents from seeing uncommitted work and
protects against accidental modifications to your development checkout.

```bash
# Dedicated clone for agents (separate from your dev checkout)
git clone --bare https://github.com/org/repo ~/repos/repo
```

## Safety Model

The sandbox container is the primary security boundary. Agent code execution is
isolated at the container level, not enforced per-tool.

| Layer | Protection |
|-------|-----------|
| **Container hardening** | `cap_drop: ALL`, `no-new-privileges`, read-only root filesystem, process limits. Agents cannot escalate privileges or escape the container. |
| **HTTP path validation** | `resolveQuestPath()` constrains the sandbox API to `/workspace/{quest-id}/`. Agents cannot reach `/repos/` via the sandbox HTTP API. |
| **Symlink escape protection** | All file paths are resolved and validated against the workspace root before any operation. |
| **`bash` scope** | Agents run `bash` commands inside their `/workspace/{quest-id}/` directory (`Cmd.Dir`). Git operations that affect `main` (push, merge, rebase to main) are not available from inside the workspace — `merge-to-main` is a privileged sandbox server endpoint only. |
| **Worktree isolation** | Each quest gets its own branch and worktree. Agents never touch `main` directly. |
| **Merge gate** | Only boss battle victory triggers `merge-to-main`. This is a privileged sandbox server endpoint, not an agent tool. |
| **Rollback** | If a merge goes wrong, `git revert` on main is trivial. |

> **Known limitation**: `bash` commands run as the `sandbox` user inside the container.
> While `Cmd.Dir` is set to the quest workspace, absolute paths in bash commands
> (e.g., `cat /repos/other-repo/file`) can access any file the sandbox user can read.
> All mounted repos are accessible. For multi-tenant deployments, consider running
> separate sandbox containers per tenant or using per-quest user namespaces.

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
