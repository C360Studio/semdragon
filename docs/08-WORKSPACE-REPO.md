# Workspace Repo

Semdragons uses a git-backed workspace repository for quest artifact management. Every
quest gets its own branch and worktree so agents write directly into a version-controlled
filesystem. Only boss-battle-approved work merges to `main`, creating a quality gate
before code enters the knowledge graph.

This replaces the old filestore snapshot pipeline. The filestore path remains as a
fallback when workspace repo is not configured.

## Contents

- [Overview](#overview)
- [Lifecycle Flow](#lifecycle-flow)
- [Docker Volume Topology](#docker-volume-topology)
- [Semsource Integration](#semsource-integration)
- [Artifact Predicates](#artifact-predicates)
- [Retirement and Pruning](#retirement-and-pruning)
- [Key Design Decisions](#key-design-decisions)
- [Critical Files](#critical-files)

---

## Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    WORKSPACE REPO (bare git)                │
│              /var/semdragons/workspace.git                  │
├────────────────────┬────────────────────────────────────────┤
│  QUEST WORKTREES   │           MAIN WORKTREE                │
│  /quest-worktrees/ │       /workspace-main/                 │
│  quest-{id}/       │   (persistent checkout of main)        │
│                    │                                        │
│  Agent writes here │   Updated after each approved merge.   │
│  via sandbox tools │   Semsource watches this directory.    │
└────────────────────┴────────────────────────────────────────┘
         ▲ RW (sandbox mounts at /workspace)        ▲ RO (semsource)
```

The sandbox container mounts `quest-worktrees/` at `/workspace/` via a shared Docker
volume. From the agent's perspective, `/workspace/quest-{id}/` is a normal directory —
it happens to be a git worktree pointing at branch `quest/{id}` in the bare repo.

Semsource is fully decoupled from quest mechanics. It watches the main worktree as a
plain directory. Attribution edges (`quest.relationship.produced`) live on the semdragons
side, linking quest entities to the semsource graph entities they produced.

## Lifecycle Flow

1. **Quest starts** — backend runs:

   ```bash
   git worktree add /var/semdragons/quest-worktrees/quest-{id} -b quest/{id}
   ```

2. **Sandbox sees it** — the `quest-worktrees/` volume is already mounted at `/workspace/`
   in the sandbox container. No restart required; the new directory appears immediately.

3. **Agent works** — writes files, builds, and tests via sandbox tools. The worktree is a
   real git repo. Agents may commit freely within the branch.

4. **Quest submitted** — `questbridge` finalizes the worktree:

   ```bash
   git add -A && git commit -m "quest/{id}: finalize submission"
   ```

   The commit hash is stored as `quest.artifacts.commit` on the quest entity.

5. **Boss battle reviews** — the review reads files from the quest branch. The work is
   visible but has not entered `main`.

6. **Boss battle VICTORY** — `bossbattle` calls `MergeToMain`:

   ```bash
   git merge --no-ff quest/{id}
   ```

   The merge commit hash is stored as `quest.artifacts.merged`. If a conflict occurs,
   `quest.artifacts.merge_conflict` is set to `true` and the DM is notified.

7. **Main worktree updated** — the backend fast-forwards the main worktree checkout.
   Semsource detects the filesystem changes.

8. **Semsource indexes** — AST, doc, and config entities are emitted via WebSocket.
   `graph-ingest` writes them to ENTITY_STATES KV. `quest.artifacts.indexed` is set
   to `true` once the indexing gate confirms completion.

9. **Dependent quests released** — quests blocked on `quest.artifacts.indexed` become
   eligible for claiming via the normal boid engine flow.

10. **Failed review** — the branch stays unmerged. The agent reworks and resubmits.
    `questdagexec` injects the peer review feedback into the next prompt assembly.

11. **Worktree pruned** — after the retention period (default 30 days), the backend
    removes the worktree directory and deletes the remote branch. Files already on
    `main` are unaffected.

## Docker Volume Topology

| Volume | Backend | Sandbox | Semsource |
|--------|---------|---------|-----------|
| `workspace-repo` (bare git) | RW | — | RO |
| `quest-worktrees` (per-quest dirs) | RW | RW (`/workspace`) | RO |
| `workspace-main` (main checkout) | RW | — | RO |

The sandbox has no access to the bare repo or the main worktree. It can only see the
per-quest directories under `/workspace/`. This limits blast radius if an agent's code
produces side effects in the filesystem.

## Semsource Integration

Semsource runs as a separate service and watches the main worktree directory. It has no
knowledge of quests, agents, or the semdragons entity model.

```
main worktree change
        │
        ▼
   semsource detects diff (inotify / polling)
        │
        ▼
   AST parser, doc extractor, config reader
        │
        ▼
   WebSocket → websocket_input component
        │
        ▼
   graph-ingest → ENTITY_STATES KV
        │
        ▼
   Agents query via graph_search tool
```

The semsource config (`docker/semsource-workspace.json`) points at the main worktree
path and the semdragons WebSocket ingest endpoint. No semdragons internals are exposed
to semsource.

Attribution is attached on the semdragons side: when `bossbattle` writes
`quest.artifacts.merged`, a relationship triple `quest.relationship.produced` is emitted
linking the quest entity to each semsource entity ID derived from the merged files.

## Artifact Predicates

All artifact predicates are stored as triples on the quest entity in ENTITY_STATES KV.

| Predicate | Type | Description |
|-----------|------|-------------|
| `quest.artifacts.commit` | string | Git commit hash from worktree finalization |
| `quest.artifacts.merged` | string | Merge commit hash after boss battle victory |
| `quest.artifacts.merge_conflict` | bool | `true` if the merge to `main` had conflicts |
| `quest.artifacts.indexed` | bool | `true` when semsource has processed merged artifacts |
| `quest.relationship.produced` | string[] | Relationship edges to semsource entity IDs |

Agents on dependent quests can check `quest.artifacts.indexed` via the `graph_search`
tool before querying for code entities produced by an earlier quest.

## Retirement and Pruning

Pruning is safe because of the two-layer persistence model:

- **Quest branch / worktree** — per-quest, temporary. Pruned after the retention period.
  Deletion removes the worktree directory and the `quest/{id}` branch from the bare repo.
- **Main branch** — permanent. Files merged to `main` persist indefinitely.
- **Graph entities** — permanent. ENTITY_STATES KV entries from semsource indexing are
  not deleted when a worktree is pruned. The knowledge graph grows monotonically.

The pruning job checks `quest.artifacts.merged` before deleting a worktree. Unmerged
branches (failed reviews, abandoned quests) are pruned by age; their work never entered
`main` and has no graph representation.

DM operators can adjust the retention period via the `workspace_repo.retention_days`
config key. Setting it to `0` disables automatic pruning.

## Key Design Decisions

**Pull-based, not push-based.** Semsource is fully decoupled — no quest or agent
awareness. It just watches a directory. This means semsource can be upgraded, replaced,
or run at a different cadence without touching semdragons code.

**Quality gate via merge.** Only boss-battle-approved code enters `main`. Raw agent
output never enters the knowledge graph. This prevents hallucinated or low-quality code
from polluting the graph that future agents query for context.

**Sandbox stays unchanged.** The sandbox container is the same isolated execution
environment it has always been. The only difference is that `/workspace/quest-{id}/`
is now a git worktree instead of a plain directory. Agents need not be aware of git.

**Backward compatible.** When `workspace_repo` is absent from the config, the system
falls back to the filestore snapshot pipeline. All artifact API endpoints try workspace
repo first, then filestore.

**Worktree pruning is safe.** Pruned worktrees affect only the per-quest branch copy.
Merged files persist on `main` permanently, and their graph entities persist in
ENTITY_STATES KV indefinitely.

## Critical Files

| Path | Purpose |
|------|---------|
| `storage/workspacerepo/` | `WorkspaceRepo` struct — git worktree lifecycle and file reads |
| `domain/workspacerepo.go` | `WorkspaceRepoProvider` interface and lazy resolver |
| `processor/questbridge/handler.go` | Worktree create and finalize on quest transitions |
| `processor/bossbattle/handler.go` | `MergeToMain` call on boss battle victory |
| `cmd/sandbox/server.go` | Verify-exists check for worktree bind-mounts |
| `config/semdragons.json` | `workspacerepo` component config block |
| `docker/compose.yml` | Volume topology (`workspace-repo`, `quest-worktrees`, `workspace-main`) |
| `docker/semsource-workspace.json` | Semsource config — watches main worktree path |
