# Party Quest DAG Execution -- Implementation Tickets

**ADR**: `docs/adr/002-party-quest-dag-execution.md`
**Goal**: Post a party-required quest, seed agents, observe autonomous DAG decomposition,
member recruitment, assignment, lead review with feedback loop, rollup, and completion.

---

## Dependency Graph

```
PDAG-01 (types + tools)
    |
    ├── PDAG-02 (party visibility + dep gate)
    |       |
    |       └── PDAG-03 (DAG execution processor) ──┐
    |                                                |
    ├── PDAG-04 (questbridge integration) ───────────┤
    |                                                |
    └── PDAG-05 (recruitment + assignment) ──────────┤
                                                     |
                                            PDAG-06 (feedback loop wiring)
                                                     |
                                            PDAG-07 (E2E test)
```

PDAG-01 has no deps (pure types). PDAG-02, 04, 05 depend on 01. PDAG-03 depends on
02. PDAG-06 depends on 03. PDAG-07 depends on all.

**Parallelism**: PDAG-01 first, then PDAG-02 + PDAG-04 + PDAG-05 in parallel, then
PDAG-03, then PDAG-06, then PDAG-07.

---

## PDAG-01: DAG Types, Decompose Tool, and Review Tool

```yaml
ticket: PDAG-01
assigned: go-developer
depends_on: []
blocks: [PDAG-02, PDAG-03, PDAG-04, PDAG-05]
reviewer: go-reviewer
status: completed
```

### Deliverables

**1. QuestDAG types** (`processor/questdagexec/types.go`)
- Port `TaskDAG`/`TaskNode` from `semspec/tools/decompose/types.go`
- Rename to `QuestDAG`/`QuestNode`
- `QuestNode` fields: `ID`, `Objective`, `Skills []string`, `Difficulty int`,
  `Acceptance []string`, `DependsOn []string`
- `Validate()` method: min 1 node, max 20, unique IDs, valid dep refs, no self-refs,
  no cycles (DFS three-color), non-empty objectives
- `DAGReadyNodes(dag, nodeStates) []string` function: port from
  `semspec/workflow/reactive/dag_execution.go:285`

**2. DAGExecutionState** (`processor/questdagexec/types.go`)
- `ExecutionID`, `ParentQuestID`, `PartyID`, `DAG QuestDAG`
- `NodeStates map[string]string` (pending/ready/assigned/in_progress/pending_review/
  completed/rejected/failed)
- `NodeQuestIDs map[string]string` (node ID -> sub-quest entity ID)
- `NodeAssignees map[string]string` (node ID -> agent entity ID)
- `CompletedNodes []string`, `FailedNodes []string`
- `NodeRetries map[string]int` (node ID -> remaining retries, default 2)

**3. decompose_quest tool** (`processor/questdagexec/decompose_tool.go`)
- Implement `agentic.ToolExecutor` interface
- Pure validation passthrough: parse nodes from `call.Arguments`, validate DAG,
  return validated JSON
- Tool definition with `goal` (required string) and `nodes` (required array) params
- Reference: `semspec/tools/decompose/executor.go` (nearly identical)

**4. review_sub_quest tool** (`processor/questdagexec/review_tool.go`)
- Implement `agentic.ToolExecutor` interface
- Args: `sub_quest_id` (string), `ratings` (object with q1/q2/q3: 1-5),
  `explanation` (string), `verdict` ("accept" or "reject")
- Validation: ratings in 1-5 range, avg < 3.0 requires non-empty explanation,
  verdict must be "accept" or "reject"
- Returns: `{verdict, avg_rating, sub_quest_id}` as JSON

**5. Tool registration** (`processor/executor/tools.go`)
- Register `decompose_quest` with `MinTier: domain.TierMaster`
- Register `review_sub_quest` with `MinTier: domain.TierMaster`

### Tests

- `types_test.go`: Table-driven tests for DAG validation
  - Valid DAG (linear chain, diamond, single node)
  - Cycle detection (simple, indirect)
  - Duplicate IDs, unknown dep refs, self-refs
  - Empty objective, node count bounds (0, 21)
  - `DAGReadyNodes` with various states
- `decompose_tool_test.go`: Valid decomposition, missing goal, invalid nodes,
  validation error passthrough
- `review_tool_test.go`: Accept verdict, reject verdict, reject without explanation,
  ratings out of range, missing sub_quest_id

### Acceptance Criteria

- [ ] `QuestDAG.Validate()` catches all structural errors from semspec test cases
- [ ] `DAGReadyNodes` returns correct nodes for pending/completed/failed mix
- [ ] Both tools return `ToolResult.Error` (not Go error) for validation failures
- [ ] Tools registered with Master+ tier gate
- [ ] `make test` passes, `make lint` clean

---

## PDAG-02: Dependency Gate and Party Visibility

```yaml
ticket: PDAG-02
assigned: go-developer
depends_on: [PDAG-01]
blocks: [PDAG-03]
reviewer: go-reviewer
status: completed
```

### Deliverables

**1. Party sub-quest visibility filter** (`processor/questboard/handler.go`)
- In `AvailableQuests`: skip quests where `PartyID != nil`
- In `ClaimQuest`: if `quest.PartyID != nil`, reject unless claimer is a party member
  (check via partycoord or a `quest.party.id` triple)

**2. Dependency enforcement in ClaimQuest** (`processor/questboard/handler.go`)
- Before allowing claim, iterate `quest.DependsOn`
- Load each dep from graph, verify `Status == QuestCompleted`
- Return error if any dep is not completed
- This covers solo quest chains; party sub-quests use DAG executor's gate

**3. Quest entity: party_id triple** (`domain/graphable.go`, `domain/reconstruction.go`)
- Add `quest.party.id` predicate
- `PostSubQuests` sets `PartyID` on each sub-quest when called from party context
- `QuestFromEntityState` reconstructs `PartyID`

### Tests (integration, requires Docker)

- Post quest chain A→B, attempt claim B before A completes → error
- Complete A, claim B → success
- Post party sub-quest (with party_id), call `AvailableQuests` → not in results
- Attempt `ClaimQuest` on party sub-quest from non-member → rejected
- `ClaimQuestForParty` on party sub-quest from party member → allowed

### Acceptance Criteria

- [ ] `AvailableQuests` never returns quests with `PartyID` set
- [ ] `ClaimQuest` rejects non-member claims on party quests
- [ ] `ClaimQuest` rejects claims on quests with unmet `DependsOn`
- [ ] `PartyID` round-trips through graphable → reconstruction
- [ ] `make test-all` passes

---

## PDAG-03: DAG Execution Processor

```yaml
ticket: PDAG-03
assigned: go-developer
depends_on: [PDAG-01, PDAG-02]
blocks: [PDAG-06]
reviewer: go-reviewer
status: completed
```

### Deliverables

**1. Processor skeleton** (`processor/questdagexec/`)
- `config.go`: `DefaultConfig()` with `dag_timeout` (30m), `recruitment_timeout` (5m),
  `recruitment_interval` (30s), `max_retries_per_node` (2), `quest_dags_bucket` name
- `register.go`: `Register(registry)` function
- `component.go`: Standard lifecycle (Start/Stop), KV watcher on sub-quest entities,
  QUEST_DAGS bucket for DAG state

**2. DAG lifecycle handler** (`processor/questdagexec/handler.go`)

Core state machine driven by KV watch on sub-quest entities:

| Sub-Quest Event | DAG Node Transition | Action |
|-----------------|---------------------|--------|
| Quest posted (by PostSubQuests) | `pending` or `ready` | Compute initial ready set |
| Quest `in_progress` | `in_progress` | Track (no action needed) |
| Quest `submitted` | `pending_review` | Dispatch lead review task |
| Lead accepts (review tool) | `completed` | Recompute ready nodes, assign next batch |
| Lead rejects (review tool) | `rejected` → `assigned` | Store feedback, re-dispatch member |
| Quest `failed` | `failed` | Attempt reassignment or escalate |
| All nodes completed | -- | Trigger rollup |
| DAG timeout | -- | Escalate parent quest |

**Dispatch lead review**: Build a review `TaskMessage` with:
- System prompt: party lead review persona + sub-quest objective + acceptance criteria
- User prompt: member's output
- Tools: `review_sub_quest` only
- Metadata: `parent_quest_id`, `sub_quest_id`, `member_id`
Publish to AGENT stream → agentic-loop runs lead's review

**Rollup**: Collect `quest.data.output` from each completed sub-quest, concatenate
(or structure as JSON), submit as parent quest output via `questboard.SubmitResult()`.
Transition parent to `completed` (or `in_review` if `require_review` set). Call
`partycoord.DisbandParty()`.

**3. Registration** (`componentregistry/register.go`)
- Add `questdagexec.Register` to `RegisterAll` and `RegisterProcessors`

**4. Config** (`config/semdragons.json`)
- Add `QUEST_DAGS` to streams/kv_buckets
- Add `questdagexec` component config

### Tests (integration)

- Initialize DAG state, simulate sub-quest completions → ready nodes recomputed
- Sub-quest submitted → lead review dispatched
- Lead accept → node completed, downstream unblocked
- Lead reject → feedback stored, node re-assigned, retry counter decremented
- Retries exhausted → node failed
- All nodes completed → rollup triggered, parent completed, party disbanded
- DAG timeout → parent escalated

### Acceptance Criteria

- [ ] DAG state machine handles all transitions in the table above
- [ ] Lead review task dispatched with correct prompt and tools
- [ ] Rollup collects all sub-quest outputs and submits to parent
- [ ] Party disbanded on terminal state (complete or failed)
- [ ] Timeout escalation works
- [ ] `make test-integration` passes

---

## PDAG-04: Questbridge Integration

```yaml
ticket: PDAG-04
assigned: go-developer
depends_on: [PDAG-01]
blocks: [PDAG-07]
reviewer: go-reviewer
status: completed
```

### Deliverables

**1. Detect DAG output from lead's loop** (`processor/questbridge/handler.go`)

When the lead's agentic loop completes for a `party_required` quest:
- Check if the loop output contains a validated DAG (JSON with `goal` + `dag.nodes`)
- Parse and re-validate the DAG (defense in depth)
- If valid: treat as decomposition intent

**2. Post sub-quests from DAG** (`processor/questbridge/handler.go`)

- Convert each `QuestNode` to a `domain.Quest` brief:
  - `Title` = node objective (truncated to 100 chars)
  - `Description` = node objective (full)
  - `RequiredSkills` = node skills
  - `Difficulty` = node difficulty (or inherit from parent)
  - `Acceptance` = node acceptance criteria
  - `DependsOn` = resolve node dep IDs to real quest IDs (two-pass, same as
    `PostQuestChain`)
  - `PartyID` = parent quest's party ID
- Call `questboard.PostSubQuests(ctx, parentID, subQuests, leadID)`
- Update parent quest with `SubQuests` list

**3. Initialize DAG execution state**

- Create `DAGExecutionState` in QUEST_DAGS bucket
- Map node IDs to sub-quest entity IDs in `NodeQuestIDs`
- Set initial node states (ready nodes = `ready`, blocked = `pending`)
- The `questdagexec` processor picks up from here via KV watch

**4. Handle non-party quests normally**

- If quest is not `party_required`, existing completion logic unchanged
- If lead's output doesn't contain a DAG, treat as normal completion

### Tests (unit, extend existing handler_test.go)

- Lead completes party quest with valid DAG → sub-quests posted, DAG state initialized
- Lead completes party quest without DAG → normal completion (no decomposition)
- Lead completes non-party quest → unchanged behavior
- DAG with invalid structure in output → quest fails with error
- Two-pass dependency resolution produces correct `DependsOn` quest IDs

### Acceptance Criteria

- [ ] DAG detection parses validated output from lead's loop
- [ ] Sub-quests posted with correct PartyID, DependsOn, skills
- [ ] DAGExecutionState correctly initialized in QUEST_DAGS
- [ ] Non-party quests and non-DAG outputs unaffected
- [ ] Existing questbridge handler_test.go passes (no regressions)

---

## PDAG-05: Party Recruitment and Assignment

```yaml
ticket: PDAG-05
assigned: go-developer
depends_on: [PDAG-01]
blocks: [PDAG-07]
reviewer: go-reviewer
status: completed
```

### Deliverables

**1. Recruitment handler** (`processor/questdagexec/handler.go`)

- `recruitMembers(ctx, dagState)`:
  - Query idle agents from KV via graph client
  - For each sub-quest's required skills, score idle agents using boid affinity
    (reuse `boidengine.ComputeAttraction` or extract scoring into shared util)
  - Filter by trust tier (agent level >= sub-quest min_tier)
  - Select top candidates (1 per sub-quest node, no duplicates)
  - Call `partycoord.JoinParty(ctx, partyID, agentID, domain.RoleExecutor)`
  - Transition party to `active` via partycoord
  - If not enough idle agents: retry on interval, escalate on timeout

**2. Assignment handler** (`processor/questdagexec/handler.go`)

- `assignReadyNodes(ctx, dagState)`:
  - Compute ready nodes via `DAGReadyNodes()`
  - For each ready node, pick the best-fit party member (by skill overlap)
  - Call `partycoord.AssignTask(ctx, partyID, subQuestID, agentID, rationale)`
  - Call `questboard.ClaimQuestForParty(ctx, subQuestID, partyID)` to formally claim
  - Update `NodeStates[nodeID]` to `assigned`
  - Update `NodeAssignees[nodeID]` to agent ID
  - Questbridge picks up `in_progress` transition and dispatches to agentic loop

**3. Agent status on recruitment** (`processor/partycoord/handler.go`)

- When `JoinParty` is called, consider setting agent status to `party_reserved`
  (or similar) to prevent autonomy from claiming the agent for solo work during
  the window between recruitment and assignment
- Alternative: recruitment + assignment happen atomically (same handler call)

**4. Component references**

- `questdagexec` needs references to `partycoord` and `questboard` components
- Wire via `component.Dependencies` in `NewFromConfig` (same pattern as other
  processors that reference sibling components)

### Tests (integration)

- 3 idle agents, 2 sub-quests → 2 agents recruited, 1 left idle
- Recruitment with skill mismatch → skips unqualified agents
- Recruitment with no idle agents → retries, then escalates
- Assignment sets agent to on_quest, sub-quest to claimed
- Assigned agent dispatched to agentic loop (verify via KV watch)

### Acceptance Criteria

- [ ] Recruitment uses boid-style skill scoring
- [ ] Only idle agents with sufficient tier are recruited
- [ ] Assignment calls both `AssignTask` and `ClaimQuestForParty`
- [ ] Recruited agents cannot be claimed by autonomy before assignment
- [ ] `make test-integration` passes

---

## PDAG-06: Peer Feedback Loop Wiring

```yaml
ticket: PDAG-06
assigned: go-developer
depends_on: [PDAG-03]
blocks: [PDAG-07]
reviewer: go-reviewer
status: completed
```

### Deliverables

**1. Store feedback on reject** (`processor/questdagexec/handler.go`)

When lead calls `review_sub_quest` with verdict=reject:
- Create `PeerReview` entity via graph client:
  - `QuestID` = sub-quest ID
  - `LeaderID` = lead agent ID
  - `MemberID` = assigned agent ID
  - `LeaderRatings` = {q1, q2, q3} from review tool
  - `LeaderExplanation` = explanation text
  - `Status` = `completed` (leader-only review, no bidirectional needed for DAG)
- Store `PeerFeedbackSummary` triples on the member's agent entity:
  - `agent.feedback.question` = question text (for each q below threshold)
  - `agent.feedback.rating` = numeric rating
  - `agent.feedback.explanation` = lead's explanation
- Emit `review.lifecycle.completed` event

**2. Populate AssemblyContext.PeerFeedback** (`processor/questbridge/handler.go`)

When questbridge builds `AssemblyContext` for dispatch:
- Load agent entity from graph
- Query `agent.feedback.*` triples
- Convert to `[]PeerFeedbackSummary`
- Set on `ctx.PeerFeedback`
- The existing assembler injection at `CategoryPeerFeedback` does the rest

**3. Update agent stats** (`processor/agentprogression/handler.go`)

Add a KV watcher (or event subscription) for `review.lifecycle.completed`:
- Load the completed review entity
- Recompute `Agent.Stats.PeerReviewAvg` as running average:
  `newAvg = (oldAvg * oldCount + thisAvg) / (oldCount + 1)`
- Increment `Agent.Stats.PeerReviewCount`
- Emit updated agent entity

**4. Verify prompt injection end-to-end**

- The assembler at `assembler.go:58-73` already handles injection
- Language is already strong: "You MUST address these"
- Verify with a test that builds `AssemblyContext` with `PeerFeedback` populated
  and checks the assembled prompt contains the feedback section

### Tests

- Unit: review reject → PeerReview entity created, feedback triples stored
- Unit: questbridge reads feedback triples → AssemblyContext.PeerFeedback populated
- Unit: agentprogression recomputes avg on review completion
- Integration: reject sub-quest → re-dispatch member → prompt contains feedback
- Verify existing `assembler_test.go` peer feedback tests still pass

### Acceptance Criteria

- [ ] Lead reject creates PeerReview entity and feedback triples
- [ ] Questbridge populates `PeerFeedback` from agent entity
- [ ] Assembled prompt contains "You MUST address these" with specific ratings
- [ ] `PeerReviewAvg` and `PeerReviewCount` updated on review completion
- [ ] Boid engine's reputation modifier now reads real data (not always 0)
- [ ] `make test-all` passes

---

## PDAG-07: E2E Playwright Test

```yaml
ticket: PDAG-07
assigned: svelte-developer
depends_on: [PDAG-03, PDAG-04, PDAG-05, PDAG-06]
blocks: []
reviewer: svelte-reviewer
status: completed
```

### Deliverables

**1. Test spec** (`ui/e2e/specs/party-quest-tree-e2e.spec.ts`)

```typescript
test.describe('Party Quest DAG @integration @ollama', () => {
    test('party quest tree completes autonomously via DAG', async ({ lifecycleApi }) => {
        test.setTimeout(600_000);

        // Seed: 1 Master (level 16), 2 Journeyman (level 8)
        const lead = await lifecycleApi.recruitAgentAtLevel('dag-lead', 16, ['code_generation']);
        const exec1 = await lifecycleApi.recruitAgentAtLevel('dag-exec1', 8, ['code_generation']);
        const exec2 = await lifecycleApi.recruitAgentAtLevel('dag-exec2', 8, ['code_generation']);

        // Post parent quest (party_required, min 3 members)
        const parent = await lifecycleApi.createQuestWithParty(
            'Build a math module: implement add(a,b) and subtract(a,b) as '
            + 'separate Go functions that take two integers and return an integer, '
            + 'then combine them into a single math.go module.',
            3
        );

        // Observe: parent quest reaches terminal state
        const finalParent = await retry(async () => {
            const q = await lifecycleApi.getQuest(extractInstance(parent.id));
            if (q.status !== 'completed' && q.status !== 'failed')
                throw new Error(`Parent quest still ${q.status}`);
            return q;
        }, { timeout: 540_000, interval: 5000 });

        expect(finalParent.status).toBe('completed');

        // Verify decomposition happened
        const detail = await lifecycleApi.getQuest(extractInstance(parent.id));
        expect(detail.sub_quests?.length).toBeGreaterThanOrEqual(2);

        // Verify party disbanded
        const parties = await lifecycleApi.listParties();
        const party = parties.find(p =>
            extractInstance(p.quest_id) === extractInstance(parent.id));
        expect(party?.status).toBe('disbanded');

        // Verify all agents returned to idle
        for (const id of [lead.id, exec1.id, exec2.id]) {
            const agent = await retry(async () => {
                const a = await lifecycleApi.getAgent(extractInstance(id));
                if (a.status !== 'idle') throw new Error(`Agent still ${a.status}`);
                return a;
            }, { timeout: 60_000, interval: 2000 });
            expect(agent.status).toBe('idle');
        }
    });
});
```

**2. Test fixtures** (`ui/e2e/fixtures/test-base.ts`)
- Verify `recruitAgentAtLevel` exists and works (it does)
- Add `getPartyForQuest(questId)` helper if useful
- Add DAG-specific response types if needed (`sub_quests` field on QuestResponse)

**3. Environment requirements**
- `E2E_OLLAMA=true`, Ollama running with code-capable model
- `SEED_E2E=true` NOT required (test seeds its own agents)
- Backend with `questdagexec`, `partycoord`, `questbridge`, `autonomy` enabled

### Acceptance Criteria

- [ ] Test passes end-to-end with Ollama
- [ ] Test is pure post-and-observe (no manual API driving mid-flow)
- [ ] Parent quest reaches `completed` with sub-quests populated
- [ ] Party formed, went active, disbanded
- [ ] All agents returned to idle
- [ ] Test gated behind `E2E_OLLAMA` environment variable

---

## PDAG-08: Documentation Update

```yaml
ticket: PDAG-08
assigned: technical-writer
depends_on: [PDAG-03]
blocks: []
reviewer: go-reviewer
status: completed
```

### Deliverables

1. Update `docs/04-PARTIES.md` with DAG execution lifecycle, lead review gate,
   and feedback loop description
2. Update `docs/03-QUESTS.md` with party sub-quest visibility rules and `PartyID`
3. Update `CLAUDE.md` with `questdagexec` processor in package structure and
   open items sections
4. Update `docs/02-DESIGN.md` concept map if needed
5. Add Swagger/OpenAPI annotations for any new endpoints (if API surface changes)

### Acceptance Criteria

- [ ] Party docs describe full DAG lifecycle including lead review
- [ ] Quest docs explain party visibility rules
- [ ] CLAUDE.md reflects new processor and configuration
- [ ] All docs pass markdown lint

---

## Agent Assignment Summary

| Ticket | Agent | Parallel Group | Effort |
|--------|-------|----------------|--------|
| PDAG-01 | go-developer | Group 1 (solo) | Medium |
| PDAG-02 | go-developer | Group 2 | Small |
| PDAG-04 | go-developer | Group 2 | Medium |
| PDAG-05 | go-developer | Group 2 | Medium |
| PDAG-03 | go-developer | Group 3 | Large (core) |
| PDAG-06 | go-developer | Group 4 | Medium |
| PDAG-07 | svelte-developer | Group 5 | Small |
| PDAG-08 | technical-writer | Group 4+ | Small |

**Review checkpoints:**
- After Group 1: go-reviewer reviews types + tools (pure logic, easy to validate)
- After Group 2: go-reviewer reviews visibility, questbridge, recruitment (integration points)
- After Group 3: go-reviewer reviews DAG processor (the big one, critical path)
- After Group 4: go-reviewer reviews feedback loop (closes the reinforcement loop)
- After Group 5: svelte-reviewer reviews E2E test
- After Group 4+: go-reviewer reviews docs

**Critical path**: PDAG-01 → PDAG-02 → PDAG-03 → PDAG-06 → PDAG-07
