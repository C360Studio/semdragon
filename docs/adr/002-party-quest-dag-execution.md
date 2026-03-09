# Party Quest DAG Execution

## Status

**Accepted** -- March 2026 (implemented; see [ADR-003](003-questdagexec-refactor.md) for subsequent refactor)

## Problem Statement

The early adopter MVP requires a party of agents to collaboratively build an "open sensor
hub" driver -- a parent quest decomposed into sub-quests with dependencies, executed by
multiple agents, with results rolled up into a final deliverable. Today, party formation
auto-triggers on quest claim, but everything after that -- decomposition, member
recruitment, task assignment, sub-quest execution, and rollup -- requires manual REST API
calls. There is no autonomous path from "post a party quest" to "party completes the
quest tree."

The sister project semspec solved the analogous problem (runtime task decomposition into
DAGs with dependency-aware execution) via three primitives: a `decompose_task` validation
tool, a `spawn_agent` delegation tool, and a reactive `dag-execution-loop` workflow. This
ADR adapts that architecture to semdragons' game metaphor, where agents are persistent
characters with levels, guilds, and reputations -- not ephemeral workers spawned on demand.

## Design Principles

1. **Recruit, don't spawn.** Semdragons agents are persistent entities with XP, levels,
   guild membership, and reputation. The party lead recruits existing idle agents into the
   party; it does not create disposable sub-agents. The boid engine and guild affinity
   influence who gets recruited.

2. **Post and observe.** The E2E test (and the demo) should be: post a party-required
   quest, seed some agents, and watch the system complete it autonomously. No manual API
   driving.

3. **Reactive DAG, not blocking orchestration.** The lead agent does not stay alive
   looping over `spawn_agent` calls. Instead, the lead decomposes the quest into a DAG,
   and a reactive processor dispatches sub-quests as dependencies resolve -- the same
   pattern as semspec's `dag-execution-loop`.

4. **Lead reviews every sub-quest.** When a party member completes a sub-quest, the
   lead reviews the output before the DAG advances. A simple 3-question rating (1-5
   scale) determines accept or reject. Below-average scores kick the sub-quest back
   with feedback that becomes part of the agent's prompt — the reinforcement learning
   loop that makes agents improve over time.

5. **Reuse existing plumbing.** Quest posting, KV watch, boid suggestions, autonomy
   claim, questbridge dispatch, peer reviews, and prompt injection already work (or
   have working type stubs). This design wires them together; it does not replace them.

6. **Game layer on top.** The DAG execution is the mechanism; the party is the narrative.
   XP, trust tiers, peer reviews, and guild affinity ride on top of the same events.

---

## Concept Mapping: Semspec to Semdragons

| Semspec | Semdragons | Key Difference |
|---------|-----------|----------------|
| `TaskDAG` / `TaskNode` | `QuestDAG` / `QuestNode` | Nodes map to quest briefs, not raw prompts |
| `decompose_task` tool | `decompose_quest` tool | Lead proposes DAG; tool validates structure |
| `spawn_agent` tool | N/A -- not used | Agents are recruited, not spawned |
| `dag-execution-loop` reactive workflow | `questdagexec` processor | Watches sub-quest KV for completions, dispatches ready nodes |
| `DAGNodeCompletePayload` | Quest entity `completed` status transition | Reuses existing KV twofer -- no separate signal needed |
| `ScenarioID` | Parent `QuestID` | The DAG executes on behalf of a parent quest |
| Agent hierarchy (`agentgraph`) | Party membership (`partycoord`) | Lead/executor/reviewer roles, not parent/child loop IDs |

### What We Port Directly

- **`TaskDAG` / `TaskNode` types and `Validate()` method** from `semspec/tools/decompose/types.go`.
  Renamed to `QuestDAG` / `QuestNode`. FileScope is optional in semdragons (agents operate
  in sandboxed directories already). Add `Skills`, `Difficulty`, and `Acceptance` fields
  to `QuestNode` so the lead can specify quest metadata per node.

- **`DAGReadyNodes()` function** from `semspec/workflow/reactive/dag_execution.go`.
  Identical logic: return nodes in "pending" state whose dependencies are all "completed."

- **DAG validation** (cycle detection via DFS, dependency reference checks, bounds).
  Identical algorithm.

### What We Do NOT Port

- **`spawn_agent` tool and blocking wait pattern.** Semdragons agents are persistent. The
  lead recruits party members; it does not spawn ephemeral child loops. Sub-quest execution
  is handled by the normal autonomy -> claim -> questbridge -> agentic-loop pipeline.

- **`agentgraph` parent/child hierarchy.** Replaced by party membership. The party entity
  tracks lead, members, roles, and sub-quest assignments.

- **Worktree isolation per node.** Semdragons agents already have per-quest sandbox
  directories managed by questtools. No additional worktree management needed.

- **`QualityEvidence` gate on node completion.** Semdragons has boss battles for quality
  review. Sub-quests that require review go through the normal `in_review` -> bossbattle
  path. The DAG executor treats quest `completed` status as the completion signal,
  regardless of whether review was involved.

---

## Architecture

### New Components

#### 1. `decompose_quest` Tool

A new tool executor registered in `processor/executor/tools.go`, available to agents at
Master+ tier when working on a `party_required` quest.

**Tool definition:**
```
Name: decompose_quest
Description: Decompose a complex quest into a DAG of sub-quests.
             Provide the goal and a list of quest nodes with dependencies.
             The validated DAG is returned for the system to execute.
Parameters:
  goal:  string (required) -- High-level decomposition rationale
  nodes: array  (required) -- Sub-quest nodes forming the DAG
    - id:          string   -- Unique node identifier
    - objective:   string   -- What the sub-quest should accomplish
    - skills:      []string -- Required skills for the sub-quest
    - difficulty:  int      -- 0-5 difficulty level
    - acceptance:  []string -- Acceptance criteria
    - depends_on:  []string -- Node IDs that must complete first
```

**Behavior:** Pure validation passthrough (identical to semspec's `decompose_task`). The
LLM proposes the DAG structure; the tool validates it (unique IDs, valid refs, no cycles,
bounds) and returns the validated DAG as JSON. No side effects.

**Trust gate:** Only available to Master+ tier agents (level 16+), enforced by the tool
registry's tier filter. This matches the existing `CanDecomposeQuest` permission.

#### 2. `questdagexec` Processor

A new reactive processor in `processor/questdagexec/` that drives DAG execution after the
lead decomposes a quest. It replaces semspec's `dag-execution-loop` reactive workflow with
a KV-watch-based processor that fits semdragons' existing patterns.

**Lifecycle:**

```
Lead calls decompose_quest tool
    |
    v
Questbridge intercepts validated DAG from tool result
    |
    v
Questbridge calls questboard.PostSubQuests() with DAG nodes as sub-quests
    |-- Sets DependsOn on each sub-quest per DAG edges
    |-- Sets ParentQuest on each sub-quest
    |-- Updates parent quest with SubQuests list
    |
    v
questdagexec recruits party members (boid-scored)
    |-- Scores idle agents against sub-quest skills
    |-- Calls partycoord.JoinParty() for top candidates
    |-- Transitions party to active
    |
    v
questdagexec assigns ready sub-quests to party members
    |
    |-- For each ready node (deps met):
    |     |-- Match to a party member by skill fit
    |     |-- Call partycoord.AssignTask() to record assignment
    |     |-- Call questboard.ClaimQuestForParty() to claim on member's behalf
    |     |-- Questbridge detects in_progress, dispatches to agentic loop
    |
    |-- On sub-quest completed: recompute ready nodes
    |     |-- Newly ready nodes: assign to available party members
    |     |-- All nodes completed: trigger rollup
    |
    |-- On sub-quest failed: attempt reassignment to another member
    |     |-- No members available: recruit replacement or escalate
    |
    v
All sub-quests completed
    |
    v
questdagexec triggers rollup:
    |-- Collects sub-quest outputs
    |-- Submits combined output as parent quest result
    |-- Transitions parent quest to completed (or in_review if review required)
    |-- Disbands party
```

**Key design choice: the lead assigns sub-quests to party members.**

The party lead is a Master+ agent who has earned the authority to decompose and delegate.
After decomposition, the DAG executor recruits agents into the party and the lead assigns
sub-quests to specific members via `partycoord.AssignTask()`. Assignment triggers
`questboard.ClaimQuestForParty()` to formally claim the sub-quest on the member's behalf,
transitioning it directly to `claimed` status. This means:

- The lead controls who works on what (informed by the party roster and skills)
- Guild affinity and boid scoring influence *recruitment* into the party, not assignment
- Trust tiers are still enforced (agents can't be recruited for quests above their tier)
- Assigned agents don't need to "discover" the sub-quest through autonomy — they're told

This is a fundamental difference from semspec, where `spawn_agent` creates ephemeral
children. In semdragons, the lead recruits persistent agents and assigns work — like a
real party lead would. The quest board is still the source of truth for quest state, but
assignment is directed, not pull-based.

**Party is a closed system.** Sub-quests of a party quest are only visible to party
members and can only be assigned by the lead. They do not appear on the public board.
If the lead's DAG has more nodes than party members, the DAG executor recruits additional
members before proceeding. If a member fails a sub-quest, the lead reassigns it to
another member or recruits a replacement. If recruitment fails within the DAG timeout,
the parent quest is escalated — not silently handed to autonomy.

#### 3. Dependency Gate

Dependency enforcement operates at two levels:

**DAG executor (primary):** The `DAGReadyNodes()` function is the authoritative gate for
party sub-quests. The DAG executor only assigns sub-quests whose dependencies are all in
`completed` state. Since party sub-quests are not on the public board and can only be
assigned by the lead, this is the real enforcement point.

**Questboard (safety net for non-party quest chains):** Add dependency enforcement to
`ClaimQuest` in `processor/questboard/handler.go` for solo quest chains posted via
`/quests/chain`:

```go
// Before allowing claim, verify all dependencies are completed
for _, depID := range quest.DependsOn {
    dep, err := c.getQuestByID(ctx, depID)
    if err != nil || dep.Status != domain.QuestCompleted {
        return nil, fmt.Errorf("dependency %s is not completed", depID)
    }
}
```

This already exists in `AvailableQuests` filtering but is not enforced in the `ClaimQuest`
path. For party sub-quests the DAG executor handles it; this gate covers solo quest chains
where autonomy is the claiming mechanism.

**Party sub-quest visibility:** Add a `party_id` filter to `AvailableQuests` so party
sub-quests are excluded from the public board. Autonomy and the boid engine never see
them. `ClaimQuest` also rejects claims on party-owned quests from non-members.

#### 4. Party Member Recruitment and Assignment

After decomposition produces sub-quests, the DAG executor recruits agents and the lead
assigns them. This is a two-step process:

**Step 1: Recruitment** (system-driven, boid-influenced)
1. Query idle agents from KV
2. Score candidates using boid affinity (skill match, guild affinity, peer review rep)
3. Filter by trust tier (agent must meet sub-quest's `min_tier`)
4. Call `partycoord.JoinParty()` with role `executor` for top candidates
5. Transition party to `active` status once enough members have joined

Recruitment is the boid engine's domain — guild bias, reputation, and skill alignment
all influence who gets invited. The DAG executor uses the same scoring as
`boidengine.ComputeAttraction()` to rank candidates.

**Step 2: Assignment** (lead-directed)
1. For each sub-quest node, match a recruited party member by skills
2. Call `partycoord.AssignTask()` to record the lead's assignment decision
3. Call `questboard.ClaimQuestForParty()` to formally claim the sub-quest on the
   member's behalf — this transitions the sub-quest from `posted` to `claimed` and
   sets the agent's status to `on_quest`
4. The agent's next autonomy heartbeat detects `on_quest` status and questbridge
   dispatches the quest to the agentic loop as normal

The lead doesn't need to manually pick agents — the system matches recruited members
to nodes by skill fit. But the assignment is *directed* (ClaimQuestForParty), not
pull-based (agent discovers and claims). This is the key difference from solo quest
execution.

**Party sub-quests are not on the public board.** Sub-quests of a party quest are
invisible to autonomy and the boid engine. Only party members can work on them, and
only via lead assignment. This is enforced by a `party_id` field on the sub-quest —
`AvailableQuests` filters out quests with a `party_id`, and `ClaimQuest` rejects
claims on party-owned quests from non-members.

**If a member fails:** The lead is responsible. The DAG executor detects the failed
sub-quest, and the lead either:
1. Reassigns to another existing party member (if one has capacity)
2. Recruits a replacement member and assigns to them
3. If no replacement is available within the timeout, the DAG fails and the parent
   quest is escalated

There is no autonomy fallback. The party is a closed, lead-directed unit.

**Boid engine integration:** The boid engine influences *recruitment* — which idle
agents are invited into the party. Once recruited, agents work exclusively under the
lead's direction. Future enhancement: add a `party_cohesion` boid rule that boosts
agents who share guild membership with the lead or have good peer review history
with the lead from prior collaborations.

#### 5. Lead Review Gate

When a party member completes a sub-quest, the DAG does NOT advance immediately. The
lead reviews the output first. This is the quality gate that makes the reinforcement
learning loop work.

**Review flow within DAG execution:**

```
Member completes sub-quest
    |
    v
questdagexec detects completion
    |-- Does NOT mark DAG node as "completed" yet
    |-- Marks node as "pending_review"
    |-- Triggers lead review
    |
    v
Lead agent's agentic loop is dispatched with review task:
    |-- System prompt: "You are the party lead. Review this output."
    |-- User prompt: sub-quest objective + acceptance criteria + member's output
    |-- Tool available: review_sub_quest
    |
    v
Lead calls review_sub_quest tool:
    Arguments:
      sub_quest_id: string
      ratings: {q1: int, q2: int, q3: int}   // 1-5 per question
      explanation: string                      // required if avg < 3.0
      verdict: "accept" | "reject"
    |
    v
Tool validates and returns result
    |
    ├─ Accept (avg >= 3.0):
    |   |-- DAG node marked "completed"
    |   |-- Ready-node recomputation fires
    |   |-- Peer review entity created (status: completed)
    |   |-- Agent stats updated (PeerReviewAvg, PeerReviewCount)
    |   |-- Positive feedback: no prompt injection needed
    |
    └─ Reject (avg < 3.0):
        |-- DAG node marked "rejected" → transitions back to "assigned"
        |-- Sub-quest status reset to "in_progress" for retry
        |-- Peer review entity created with low ratings
        |-- Agent stats updated (avg drops)
        |-- Negative feedback: explanation stored as PeerFeedbackSummary
        |-- On next dispatch, promptmanager injects feedback into agent's prompt:
        |   "In recent tasks, your peers rated you low on the following.
        |    You MUST address these:
        |    - Task quality (2.0/5.0): Output was missing error handling"
        |-- Member re-executes with corrective guidance baked in
```

**The three review questions** (same as existing peer review system):

| # | Question | What the lead evaluates |
|---|----------|------------------------|
| Q1 | Task quality | Did the output meet acceptance criteria? |
| Q2 | Communication | Were assumptions stated clearly? |
| Q3 | Autonomy | Did the agent work independently without gaps? |

**Scoring rules:**
- Each question: 1-5 integer
- Average >= 3.0: accept (sub-quest advances)
- Average < 3.0: reject (explanation required, sub-quest retries with feedback)
- Max retries per sub-quest: configurable (default: 2). After exhaustion, DAG node
  fails and the lead can reassign to another member.

**Why the lead reviews, not boss battle:**
Boss battles are for the *parent quest* output after rollup. The lead's review of
sub-quests is an internal party quality gate — faster, cheaper (no separate LLM judge
call), and the feedback goes directly into the member's prompt. Boss battles are still
the external quality gate when the parent quest has `require_review` set.

**Wiring the feedback loop (currently broken):**

The prompt injection infrastructure already exists but is unpopulated:
- `PeerFeedbackSummary` type in `promptmanager/types.go` -- defined
- `AssemblyContext.PeerFeedback` field -- defined
- Assembler injection at `CategoryPeerFeedback` (priority 250) -- implemented
- Prompt text: "You MUST address these" -- already written

What's missing and must be built:
1. **questdagexec writes PeerFeedbackSummary to agent entity** when lead rejects a
   sub-quest. Stored as triples: `agent.feedback.question`, `agent.feedback.rating`,
   `agent.feedback.explanation`.
2. **questbridge reads agent feedback** when building the AssemblyContext for dispatch.
   Queries agent entity for feedback triples, populates `ctx.PeerFeedback`.
3. **agentprogression updates PeerReviewAvg/Count** when review entities are created.
   Watches `review.lifecycle.completed` events, recomputes running average.

This closes the loop: reject → feedback stored → prompt injected → agent retries with
"don't do this" guidance → quality improves → future reviews score higher → boid engine
gives better quest suggestions.

---

## Data Model Changes

### QuestDAG and QuestNode Types

New types in `processor/questdagexec/types.go`, ported from semspec:

```go
type QuestDAG struct {
    Nodes []QuestNode `json:"nodes"`
}

type QuestNode struct {
    ID         string   `json:"id"`
    Objective  string   `json:"objective"`
    Skills     []string `json:"skills,omitempty"`
    Difficulty int      `json:"difficulty,omitempty"`
    Acceptance []string `json:"acceptance,omitempty"`
    DependsOn  []string `json:"depends_on,omitempty"`
}
```

Validation rules (from semspec, adapted):
- At least 1 node, maximum 20 (party quests are smaller than semspec's 100-node DAGs)
- Unique IDs, valid dependency refs, no self-refs, no cycles (DFS)
- Each node must have a non-empty objective

### Quest Entity Additions

New triple predicates on quest entities:

```go
quest.dag.id          // DAG execution ID (links parent quest to DAG state)
quest.dag.node_id     // Which DAG node this sub-quest corresponds to
```

These enable `questdagexec` to correlate sub-quest completions back to DAG nodes.

### DAG Execution State (KV)

Stored in a new KV bucket `QUEST_DAGS`:

```go
type DAGExecutionState struct {
    ExecutionID    string            `json:"execution_id"`
    ParentQuestID  string            `json:"parent_quest_id"`
    PartyID        string            `json:"party_id"`
    DAG            QuestDAG          `json:"dag"`
    NodeStates     map[string]string `json:"node_states"`     // pending/posted/claimed/completed/failed
    NodeQuestIDs   map[string]string `json:"node_quest_ids"`  // node ID -> sub-quest entity ID
    CompletedNodes []string          `json:"completed_nodes"`
    FailedNodes    []string          `json:"failed_nodes"`
}
```

Node states track the sub-quest lifecycle within the party:
- `pending` -- Dependencies not yet met; sub-quest exists but is not assignable
- `ready` -- All dependencies completed; eligible for assignment to a party member
- `assigned` -- Lead has assigned to a party member; quest claimed on their behalf
- `in_progress` -- Member is actively working (questbridge dispatched to agentic loop)
- `pending_review` -- Member submitted output; waiting for lead to review
- `completed` -- Lead accepted the output; node advances the DAG
- `rejected` -- Lead rejected the output; transitions back to `assigned` for retry
- `failed` -- Sub-quest exhausted retries or reassignment failed

The `pending_review` → `completed` / `rejected` transition is the lead review gate.
Rejected nodes cycle back through `assigned` → `in_progress` → `pending_review` with
the lead's feedback injected into the member's prompt via `PeerFeedbackSummary`.

These are tracked in the DAG state for efficient ready-node computation. The underlying
quest entities still use the standard quest status enum, but the DAG executor maps
between them (e.g. quest `in_review` maps to DAG `pending_review`).

---

## Autonomous Flow: End to End

### Step-by-step with game events

```
1. Human (or DM) posts parent quest
   - party_required: true
   - objective: "Build sensor hub addition module"
   - difficulty: 3 (Hard)
   - skills: [code_generation]
   Event: quest.lifecycle.posted

2. Autonomy heartbeat fires for idle Master+ agent
   - Boid engine suggests parent quest (high affinity for skills + tier)
   - Agent claims parent quest
   Events: quest.lifecycle.claimed, agent.progression.quest_claimed

3. Partycoord detects claimed + party_required
   - Auto-forms party with claiming agent as lead
   Event: party.formation.created

4. Questbridge detects in_progress transition
   - Builds TaskMessage with decompose_quest tool available (Master+ gate)
   - Dispatches to agentic loop
   Event: quest.lifecycle.started

5. Lead agent's LLM calls decompose_quest tool
   - Proposes DAG:
     node "add":       objective="Write add(a,b) function", skills=[code_generation]
     node "subtract":  objective="Write subtract(a,b) function", skills=[code_generation]
     node "combine":   objective="Combine into module", depends_on=["add","subtract"]
   - Tool validates and returns DAG
   Event: (tool call within agentic loop)

6. Questbridge intercepts validated DAG from lead's loop completion
   - Calls questboard.PostSubQuests() for each node
   - Sets DependsOn["add","subtract"] on "combine" sub-quest
   - Updates parent quest with SubQuests list
   Events: quest.lifecycle.posted (x3), quest.decomposed

7. questdagexec initializes DAG state
   - Creates DAGExecutionState in QUEST_DAGS bucket
   - "add" and "subtract" are immediately ready (no deps)
   - "combine" is pending (deps not met)

8. questdagexec recruits idle agents into party
   - Scores idle agents using boid affinity (skills, guild, reputation)
   - Recruits 2 best-fit agents with code_generation skill
   - Calls partycoord.JoinParty() with role=executor for each
   - Party transitions to active
   Events: party.formation.joined (x2), party.status.active

9. Lead assigns sub-quests to party members
   - questdagexec matches recruited members to DAG nodes by skill fit
   - Calls partycoord.AssignTask() for each assignment
   - Calls questboard.ClaimQuestForParty() to claim on member's behalf
   - Sub-quests transition to claimed, agents transition to on_quest
   - Questbridge dispatches each to agentic loop
   - Each agent writes simple code (add two ints, subtract two ints)
   Events: quest.lifecycle.claimed (x2), started (x2), submitted (x2)

10. Lead reviews sub-quest outputs
    - questdagexec detects sub-quest submissions, marks nodes pending_review
    - Lead's agentic loop is dispatched with review task for each
    - Lead calls review_sub_quest tool with ratings and verdict
    - If accept (avg >= 3.0): node marked completed, peer review created
    - If reject (avg < 3.0): node marked rejected, feedback stored,
      member re-executes with corrective guidance in prompt
    Events: review.lifecycle.completed (x2), agent.progression.xp

11. questdagexec detects "add" and "subtract" accepted by lead
    - Recomputes ready nodes: "combine" is now ready (deps met)
    - Assigns "combine" to an available party member
    - Calls ClaimQuestForParty, questbridge dispatches to agentic loop
    Event: quest.lifecycle.claimed, quest.lifecycle.started

12. Lead reviews "combine" output, accepts it
    Event: review.lifecycle.completed

13. questdagexec detects all nodes completed (accepted by lead)
    - Collects outputs from all sub-quests
    - Submits combined output as parent quest result
    - Parent quest transitions to completed (or in_review)
    - Disbands party
    Events: quest.lifecycle.completed (parent), party.formation.disbanded

13. Agent progression awards XP
    - Lead gets party lead XP bonus
    - Executors get sub-quest XP
    - Peer reviews created for lead <-> each executor
    Events: agent.progression.xp (x3)
```

### What the E2E test does

```typescript
test('party quest tree completes autonomously via DAG', async ({ lifecycleApi }) => {
    test.setTimeout(600_000); // 10 min for Ollama

    // Seed: 1 Master agent (level 16), 2 Journeyman agents (level 8)
    const lead = await lifecycleApi.recruitAgentAtLevel('dag-lead', 16, ['code_generation']);
    const exec1 = await lifecycleApi.recruitAgentAtLevel('dag-exec1', 8, ['code_generation']);
    const exec2 = await lifecycleApi.recruitAgentAtLevel('dag-exec2', 8, ['code_generation']);

    // Post parent quest
    const parent = await lifecycleApi.createQuestWithParty(
        'Build a math module: implement add(a,b) and subtract(a,b) as separate functions, '
        + 'then combine them into a single module. Each function takes two integers and '
        + 'returns an integer.',
        3 // min party size
    );

    // Observe: parent quest reaches terminal state
    const finalParent = await retry(
        async () => {
            const q = await lifecycleApi.getQuest(extractInstance(parent.id));
            if (q.status !== 'completed' && q.status !== 'failed') {
                throw new Error(`Parent quest still ${q.status}`);
            }
            return q;
        },
        { timeout: 540_000, interval: 5000, message: 'Parent quest did not complete' }
    );
    expect(finalParent.status).toBe('completed');

    // Verify sub-quests all completed
    // (parent.sub_quests populated by decomposition)
    const parentDetail = await lifecycleApi.getQuest(extractInstance(parent.id));
    expect(parentDetail.sub_quests?.length).toBeGreaterThanOrEqual(2);

    // Verify party disbanded
    const parties = await lifecycleApi.listParties();
    const partyForQuest = parties.find(p =>
        extractInstance(p.quest_id) === extractInstance(parent.id)
    );
    expect(partyForQuest?.status).toBe('disbanded');

    // Verify agents returned to idle
    for (const agentId of [lead.id, exec1.id, exec2.id]) {
        const agent = await retry(
            async () => {
                const a = await lifecycleApi.getAgent(extractInstance(agentId));
                if (a.status !== 'idle') throw new Error(`Agent still ${a.status}`);
                return a;
            },
            { timeout: 60_000, interval: 2000 }
        );
        expect(agent.status).toBe('idle');
    }
});
```

Pure post-and-observe. The test creates the initial conditions and asserts the terminal
state. Everything in between is autonomous.

---

## Implementation Phases

### Phase 1: DAG Types, Decompose Tool, and Review Tool

**Goal**: Lead agent can propose a quest DAG and review sub-quest outputs.

1. Create `processor/questdagexec/types.go` with `QuestDAG`, `QuestNode`, `Validate()`,
   and `DAGReadyNodes()` -- ported from semspec's `tools/decompose/types.go` and
   `workflow/reactive/dag_execution.go`.
2. Create `decompose_quest` tool executor in `processor/questdagexec/decompose_tool.go`.
3. Create `review_sub_quest` tool executor in `processor/questdagexec/review_tool.go`.
   Validates ratings (1-5), enforces explanation when avg < 3.0, returns accept/reject.
4. Register both tools in `processor/executor/tools.go` with Master+ tier gate.
5. Unit tests for DAG validation (cycles, refs, bounds), decompose tool, and review tool.

**Files**:
- `processor/questdagexec/types.go` -- new
- `processor/questdagexec/types_test.go` -- new
- `processor/questdagexec/decompose_tool.go` -- new
- `processor/questdagexec/decompose_tool_test.go` -- new
- `processor/questdagexec/review_tool.go` -- new
- `processor/questdagexec/review_tool_test.go` -- new
- `processor/executor/tools.go` -- register decompose_quest and review_sub_quest

### Phase 2: Dependency Gate and Party Visibility

**Goal**: Party sub-quests are invisible to autonomy; solo quest chains enforce deps.

1. Add `party_id` filter to `AvailableQuests` -- exclude quests with a `party_id`.
2. Add `party_id` check to `ClaimQuest` -- reject claims from non-party-members.
3. Add `DependsOn` check to `ClaimQuest` for solo quest chains.
4. Integration tests: party sub-quest invisible to `AvailableQuests`; non-member claim
   rejected; solo chain dep enforcement.

**Files**:
- `processor/questboard/handler.go` -- party visibility + dep checks
- `processor/questboard/component_test.go` -- new test cases

### Phase 3: DAG Execution Processor

**Goal**: Reactive processor watches sub-quest state changes, triggers lead reviews,
and drives the DAG to terminal state.

1. Create `processor/questdagexec/component.go` -- standard processor structure.
2. KV watcher on quest entities matching sub-quest IDs.
3. `DAGExecutionState` stored in `QUEST_DAGS` KV bucket.
4. On sub-quest submitted: mark node `pending_review`, dispatch lead review task.
5. On lead accept: mark node `completed`, recompute ready nodes, assign next batch.
6. On lead reject: mark node `rejected` → `assigned`, store feedback, re-dispatch
   member with corrective prompt. Decrement retry counter.
7. On node retries exhausted: mark `failed`, attempt reassignment or escalate.
8. Auto-rollup when all nodes completed (accepted by lead): collect outputs, submit
   parent result.
9. Party disbanding on terminal state.
10. Register in `componentregistry/register.go`.

**Files**:
- `processor/questdagexec/component.go` -- new
- `processor/questdagexec/config.go` -- new
- `processor/questdagexec/register.go` -- new
- `processor/questdagexec/handler.go` -- new
- `processor/questdagexec/component_test.go` -- new (integration)
- `componentregistry/register.go` -- register
- `config/semdragons.json` -- add QUEST_DAGS bucket and component config

### Phase 4: Questbridge Integration

**Goal**: When the lead's agentic loop completes with a validated DAG in its output,
questbridge posts sub-quests and initializes DAG execution.

1. Extend questbridge's loop completion handler to detect DAG output.
2. Call `questboard.PostSubQuests()` with DAG nodes converted to quest briefs.
3. Initialize `DAGExecutionState` in QUEST_DAGS bucket.
4. Trigger party member recruitment.

**Files**:
- `processor/questbridge/handler.go` -- extend completion handler
- `processor/questbridge/handler_test.go` -- new test cases

### Phase 5: Party Recruitment and Assignment

**Goal**: After sub-quests are posted, recruit idle agents into the party and assign
sub-quests to members.

**Recruitment** (boid-influenced):
1. Query idle agents from KV with matching skills.
2. Use boid affinity scoring to rank candidates.
3. Call `partycoord.JoinParty()` for top candidates with role `executor`.
4. Transition party status to `active`.

**Assignment** (lead-directed):
5. Match recruited members to DAG nodes by skill fit.
6. Call `partycoord.AssignTask()` to record each assignment.
7. Call `questboard.ClaimQuestForParty()` to claim sub-quests on members' behalf.
8. Questbridge detects `in_progress` transitions and dispatches to agentic loop.

Recruitment lives in `questdagexec` (it knows the DAG and required skills). Assignment
calls into `partycoord` (which owns the SubQuestMap) and `questboard` (which owns quest
state). `questdagexec` publishes a `party.recruitment.requested` event; `partycoord`
handles the JoinParty calls and transitions party to `active`.

**Files**:
- `processor/partycoord/handler.go` -- add recruitment handler
- `processor/questdagexec/handler.go` -- recruitment + assignment orchestration
- `processor/questboard/handler.go` -- verify ClaimQuestForParty works for sub-quests
- `processor/partycoord/component_test.go` -- new test cases
- `processor/questboard/component_test.go` -- new test case for party claim of sub-quest

### Phase 6: Peer Feedback Loop Wiring

**Goal**: Lead review rejections produce feedback that gets injected into agent prompts
on retry.

1. When `review_sub_quest` returns reject, store `PeerFeedbackSummary` triples on the
   agent entity: `agent.feedback.question`, `agent.feedback.rating`,
   `agent.feedback.explanation`.
2. Create peer review entity (status: completed) linking lead and member to the sub-quest.
3. Update `Agent.Stats.PeerReviewAvg` and `PeerReviewCount` in agentprogression:
   add a handler that watches `review.lifecycle.completed` events and recomputes the
   running average.
4. In questbridge, when building `AssemblyContext` for dispatch, query agent entity for
   recent feedback triples and populate `ctx.PeerFeedback`.
5. Verify the existing assembler injection works end-to-end (it already has the "You MUST
   address these" language at `CategoryPeerFeedback` priority 250).

**Files**:
- `processor/questdagexec/handler.go` -- store feedback on reject
- `processor/agentprogression/handler.go` -- watch review.lifecycle.completed, update stats
- `processor/questbridge/handler.go` -- populate AssemblyContext.PeerFeedback from agent
- `processor/promptmanager/assembler_test.go` -- verify injection (tests exist, may need update)
- `processor/agentprogression/component_test.go` -- new test for stats update

### Phase 7: E2E Test

**Goal**: Playwright test that posts a party-required quest and observes autonomous
completion.

1. Add `recruitAgentAtLevel` fixture if not already present (it exists).
2. Write `party-quest-tree-e2e.spec.ts` as shown in the test section above.
3. Requires Ollama with a model that can generate simple code.

**Files**:
- `ui/e2e/specs/party-quest-tree-e2e.spec.ts` -- new
- `ui/e2e/fixtures/test-base.ts` -- add helpers if needed

---

## Risks and Mitigations

### LLM produces invalid DAG

**Risk**: The lead's LLM proposes a DAG that fails validation.

**Mitigation**: The `decompose_quest` tool returns validation errors as non-fatal
`ToolResult.Error`, giving the LLM a chance to retry with a corrected DAG. The agentic
loop's retry mechanism handles this naturally. If the lead exhausts retries, the parent
quest fails and goes back to the board.

### No idle agents with matching skills

**Risk**: Sub-quests need assignment but no idle agents with matching skills are available
to recruit.

**Mitigation**: The DAG executor retries recruitment on a configurable interval (default:
30 seconds) as agents finish other work and return to idle. If insufficient members are
recruited within the recruitment timeout (default: 5 minutes), the DAG executor escalates
the parent quest to the DM for intervention. The party does not proceed with partial
membership — all required roles must be filled before assignment begins.

### Sub-quest fails after retries exhausted

**Risk**: One node in the DAG permanently fails, blocking downstream nodes.

**Mitigation**: The DAG executor detects the failure and first attempts reassignment:
the lead reassigns the failed sub-quest to another party member with capacity, or
recruits a replacement. If reassignment also fails (no available members, retries
exhausted), the DAG transitions to `failed` state, the parent quest is escalated, and
the party is disbanded. The parent quest can be reposted manually for a fresh attempt
with a new party.

### Boid engine doesn't prioritize party-relevant agents for recruitment

**Risk**: The recruitment scoring uses boid affinity, but the boid engine has no concept
of "this agent would be a good party member." It only scores agent-quest pairs.

**Mitigation**: For MVP, scoring each idle agent against each sub-quest's skills using
the existing affinity/cohesion rules is sufficient. The strongest signal (skill match +
guild priority) already identifies good candidates. Future enhancement: add a
`party_cohesion` boid rule that boosts agents who share guild membership with the lead
or have high peer review scores with the lead from prior collaborations.

### Party member is busy when assigned

**Risk**: An agent is recruited into the party but becomes busy (claimed a solo quest
via autonomy) before assignment happens.

**Mitigation**: `ClaimQuestForParty` checks agent status. If the agent is no longer
idle, the DAG executor skips that member and either reassigns to another member or
recruits a replacement. Recruitment should set agent status to a `party_reserved` state
(or immediately `on_quest`) to prevent autonomy from claiming the agent for solo work
during the window between recruitment and assignment.

---

## Future Enhancements (Post-MVP)

1. **Party cohesion boid rule**: Bonus affinity for sub-quests of your party's quest.
2. **Lead-directed assignment hints**: Let the lead's LLM suggest specific agents for
   nodes (by skill match), surfaced as hints that the DAG executor respects.
3. **Recursive decomposition**: A sub-quest node can itself be `party_required`, enabling
   nested DAGs. The DAG executor would need depth tracking (port semspec's `maxDepth`).
4. **Partial rollup**: Allow the lead to produce intermediate rollups as sub-quests
   complete, rather than waiting for all nodes.
5. **DAG visualization in dashboard**: Show the quest tree with dependency arrows,
   node status colors, review scores, and agent avatars on claimed nodes.
6. **Bidirectional peer reviews**: After the party disbands, members also rate the lead
   (already designed in docs/04-PARTIES.md). Currently only lead→member reviews happen
   during DAG execution.
7. **Reputation dashboard**: Surface agent PeerReviewAvg as a visible "reputation score"
   in the UI, eBay-style. Agents with consistently high scores get a badge; agents with
   low scores get flagged for retraining or retirement.
8. **Feedback decay**: Old negative feedback should eventually age out of the prompt to
   avoid permanently penalizing agents who have improved. Configurable TTL per feedback
   entry.

---

## Summary

This design bridges semspec's proven DAG execution model into semdragons' game world.
The key adaptation is replacing agent spawning with agent recruitment -- persistent agents
claim sub-quests through the existing boid-driven autonomy pipeline rather than being
created on demand. The DAG executor is a thin reactive layer that tracks dependency gates,
detects terminal conditions, and triggers rollup. Everything else (quest posting, agent
matching, LLM execution, quality review) uses existing processors.

The E2E test is pure post-and-observe: seed agents, post a party quest, wait for
completion. This validates the full autonomous pipeline from quest decomposition through
multi-agent execution to result aggregation -- exactly what the early adopter demo needs.
