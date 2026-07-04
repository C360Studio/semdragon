# dag-execution Specification (delta)

## ADDED Requirements

### Requirement: DAG proposal validation
A party lead's decomposition SHALL be validated before execution: at most 20
nodes, no self-references, no cycles, and every `depends_on` entry referencing
a node in the same DAG. Invalid proposals SHALL be rejected back to the lead
with the violation named.

#### Scenario: Cyclic DAG rejected
- **WHEN** a lead proposes a DAG where node A depends on B and B depends on A
- **THEN** the proposal is rejected with a cycle violation and no sub-quests are created

### Requirement: Dependency-ordered, exactly-once dispatch
Sub-quest dispatch SHALL be driven by the framework gated-DAG executor: a node
becomes dispatchable only when all its dependencies are complete, and each
node SHALL be dispatched exactly once per attempt (claim-before-dispatch).

#### Scenario: Diamond dependency ordering
- **WHEN** a DAG has nodes B and C depending on A, and D depending on both B and C
- **THEN** A dispatches first, B and C dispatch only after A completes, and D dispatches only after both B and C complete

#### Scenario: No duplicate dispatch
- **WHEN** two watcher events for the same node-ready condition arrive concurrently
- **THEN** the node is dispatched exactly once

### Requirement: Restart recovery
An in-flight DAG execution SHALL survive process restart: completed nodes stay
completed, in-flight nodes are recovered or re-dispatched per the executor's
claim protocol, and no node is double-executed as a result of the restart.

#### Scenario: Restart mid-execution
- **WHEN** the backend restarts while a DAG has two completed nodes and one in-flight node
- **THEN** execution resumes without re-running the completed nodes and the DAG proceeds to completion

### Requirement: Party-scoped sub-quests
Sub-quests SHALL carry the party identifier, SHALL NOT appear on the public
quest board or in boid suggestions, and SHALL be assigned to the
best-skill-matched party member via an atomic claim-and-start. The party lead
SHALL NOT be assigned executor work.

#### Scenario: Sub-quest invisible to public board
- **WHEN** a DAG node is posted as a sub-quest
- **THEN** it does not appear in the public available-quests listing and only party members can be assigned to it

### Requirement: Lead review gate
When a sub-quest is submitted, the node SHALL enter a review state and the
lead SHALL rate it on the three review questions (1–5). An average of 3.0 or
higher accepts the node; below 3.0 rejects it, and rejection SHALL require an
explanation that is injected into the member's retry prompt. While game
mechanics are disabled, review outcomes SHALL NOT update any agent reputation
statistics.

#### Scenario: Rejection feeds the retry prompt
- **WHEN** the lead rejects a node with an explanation
- **THEN** the node returns for retry and the member's next dispatch prompt contains the lead's explanation

#### Scenario: Acceptance unlocks dependents
- **WHEN** the lead accepts a node
- **THEN** the node completes and any nodes depending on it become dispatchable

### Requirement: Retry budget and parent escalation
Each node SHALL have a bounded retry budget. When a node exhausts its budget,
the node SHALL fail terminally, remaining unstarted nodes SHALL be cancelled,
and the parent quest SHALL be escalated for DM attention.

#### Scenario: Node exhaustion escalates parent
- **WHEN** a node fails more times than its retry budget allows
- **THEN** the parent quest transitions to `escalated` and no further nodes are dispatched

### Requirement: Clarification pause
A member's clarification question SHALL pause the node until the lead answers,
after which the node SHALL be re-dispatched to the same member with the answer
in context.

#### Scenario: Clarification round trip
- **WHEN** a member asks a clarification question on an assigned node
- **THEN** the node pauses, the lead receives the question, and the lead's answer appears in the member's re-dispatch context

### Requirement: Rollup on completion
When all nodes complete, the lead SHALL receive the node outputs in
topological order, and the lead's synthesized rollup SHALL become the parent
quest's output, after which the party disbands and members return to idle.

#### Scenario: Rollup completes the parent
- **WHEN** the final node of a DAG completes and the lead returns a rollup
- **THEN** the parent quest carries the rollup as its output, proceeds through its own lifecycle (review or completion), and the party is disbanded

### Requirement: Stall detection
DAG execution SHALL surface stalled nodes (dispatched but showing no progress
beyond a configured window) through the executor's stall detection, resulting
in DM-visible attention rather than silent hangs.

#### Scenario: Wedged node surfaces
- **WHEN** a dispatched node produces no progress events within the stall window
- **THEN** a stall signal is emitted and surfaced to the DM attention system

### Requirement: DAG state is graph-inspectable
The DAG's execution state SHALL be reconstructable from graph entities at any
time, for crash recovery, the dashboard, and trajectory audit. This covers
node states, assignees, retry counts, and the node-to-sub-quest mapping.

#### Scenario: Mid-execution state query
- **WHEN** a DAG is mid-execution and a client queries the parent quest's state
- **THEN** the response reflects the current per-node states and assignees without consulting any in-memory-only structure
