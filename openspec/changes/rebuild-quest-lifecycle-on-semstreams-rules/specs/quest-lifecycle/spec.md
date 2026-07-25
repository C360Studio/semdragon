# quest-lifecycle Specification (delta)

## ADDED Requirements

### Requirement: Quest lifecycle is a declared workflow
The quest state machine SHALL be defined as a semstreams `pkg/lifecycle`
`Workflow` with an explicit transition table covering the phases `posted`,
`claimed`, `in_progress`, `in_review`, `completed`, `failed`, `escalated`, and
`pending_triage`. `completed`, `failed`, and `escalated` SHALL be terminal
phases. Transitions not present in the table SHALL be rejected.

#### Scenario: Invalid transition is rejected
- **WHEN** a component or operator attempts to transition a quest from `posted` directly to `completed`
- **THEN** the transition is rejected by the workflow table and the quest phase is unchanged

#### Scenario: Terminal phase is final
- **WHEN** a quest is in `completed` and any transition is attempted
- **THEN** the transition is rejected

### Requirement: Standard lifecycle path
A quest SHALL progress `posted` → `claimed` (agent claim) → `in_progress`
(work start) → `in_review` (submission, when review is required) or `completed`
(submission, when review is not required). `in_review` SHALL resolve to
`completed` or `failed`.

#### Scenario: Claim and start
- **WHEN** an eligible agent claims a posted quest and starts work
- **THEN** the quest transitions `posted` → `claimed` → `in_progress` and the claiming agent is recorded on the quest entity

#### Scenario: Submission without review
- **WHEN** an agent submits a result for an `in_progress` quest whose spec does not require review
- **THEN** the quest transitions to `completed` and the submitted output is persisted as `quest.data.output`

#### Scenario: Submission with review required
- **WHEN** an agent submits a result for an `in_progress` quest that requires review
- **THEN** the quest transitions to `in_review`

### Requirement: Failure routing with retry budget
When a quest fails (from `in_progress` or `in_review`), the system SHALL
repost it (`posted`) while `attempts < max_attempts`, and SHALL route it to
`pending_triage` when the retry budget is exhausted. Abandonment SHALL return
the quest to `posted` and free the agent.

#### Scenario: Failure with retries remaining
- **WHEN** a quest fails and its attempt count is below `max_attempts`
- **THEN** the quest returns to `posted` with its attempt count preserved and the agent returns to idle

#### Scenario: Retry budget exhausted
- **WHEN** a quest fails and its attempt count has reached `max_attempts`
- **THEN** the quest transitions to `pending_triage` for DM triage

### Requirement: Transitions carry atomic state updates
Transition-coupled state SHALL be written atomically with the phase write via
the lifecycle manager's transition API, not as separate graph writes. This
covers attempt counters, timestamps, and failure reasons.

#### Scenario: Attempt increment is atomic with claim
- **WHEN** a quest is claimed
- **THEN** the attempt counter increment and the `posted` → `claimed` phase write land in a single entity update

### Requirement: Transition audit history
Every phase transition SHALL be recorded with timestamp and source attribution
(rule, operator, component, or framework) and SHALL be replayable as an
ordered history for any quest.

#### Scenario: History query after full lifecycle
- **WHEN** a quest has progressed from `posted` to `completed`
- **THEN** querying its lifecycle history returns every transition in order, each with its source and timestamp

### Requirement: Transitions are rule-driven
Lifecycle edges SHALL be effected by rule-engine conditions firing lifecycle
actions (or by explicit operator/component calls through the lifecycle
manager). Product components SHALL NOT mutate the quest phase predicate
directly.

#### Scenario: Completion side effects fire from a rule
- **WHEN** a quest transitions into `completed`
- **THEN** any configured side effects for that edge are fired by a rule keyed on the transition, not by inline code in the transitioning component

### Requirement: Dependency gating
A quest with `depends_on` entries SHALL NOT be claimable until every
dependency quest has reached `completed`. Dependency cycles SHALL be rejected
at creation time.

#### Scenario: Blocked dependent quest
- **WHEN** an agent attempts to claim a quest whose dependency is not yet `completed`
- **THEN** the claim is rejected and the quest remains `posted`

### Requirement: Operator lifecycle surface
Operators SHALL be able to inspect quest workflow state and history and force
valid transitions through the framework lifecycle gateway. Operator writes
SHALL be constrained to operator-writable fields and table-valid transitions.

#### Scenario: Operator escalates a stuck quest
- **WHEN** an operator issues an escalate transition for an active quest via the lifecycle gateway
- **THEN** the quest transitions to `escalated` with the transition source recorded as operator

### Requirement: Mechanics side effects are stubbed in this change
While game mechanics are disabled, quest completion SHALL NOT emit XP, start
boss battles, create peer reviews, update reputation, merge/index artifacts,
or touch store inventory. Every shipped runtime profile SHALL disable
bossbattle and SHALL load the complete quest lifecycle rule pack, including the
mechanics stub. Quests that require review SHALL transition `in_review` →
`completed` with a verdict noting that review was skipped because mechanics
are disabled.

#### Scenario: Review-required quest completes under stub
- **WHEN** a quest requiring review is submitted while mechanics are disabled
- **THEN** it reaches `completed` with a recorded review-skipped marker
- **AND** no battle entity, XP award, reputation update, peer review, store
  mutation, inventory mutation, or artifact merge/index mutation is produced

#### Scenario: Every shipped profile uses one review resolver

- **WHEN** contract tests parse the default, semsource, and tier-1 E2E
  Semdragons configurations
- **THEN** none activates bossbattle
- **AND** each retains `agent_progression` with `enabled: false`
- **AND** each loads the complete quest lifecycle rule pack for quest entities
- **AND** the mechanics-stub rule is the only automatic resolver of
  `in_review` during Phase 1

#### Scenario: Agentprogression is a dormant product implementation

- **WHEN** issue #30 completes the agentprogression quarantine
- **THEN** no shipped profile produces XP, penalty, level, cooldown, death, or
  ready side effects
- **AND** the unproduced XP payload, false event output, and event-only
  vocabulary are absent
- **AND** the Agent model, reconstruction, XP engine, validation, handlers,
  and watchers remain for Phase 2
- **AND** no agentstore, dependency, predicate, owner, lifecycle, or watcher
  migration is included
- **AND** reactivation remains blocked on beta.158 and SemStreams #313

#### Scenario: Agentstore is unavailable without product-surface deletion

- **WHEN** issue #32 completes the agentstore quarantine
- **THEN** every shipped profile retains `agent_store` with `enabled: false`
- **AND** completion through the mechanics stub produces no store or inventory
  side effect
- **AND** store routes, OpenAPI, models, policy, and dormant implementation
  remain
- **AND** store API calls retain their unavailable 503 response
- **AND** autonomy treats the unavailable store as a no-op
- **AND** the five publisher-only raw event families, seven event vocabulary
  entries, and false output are absent
- **AND** EntityState KV message categories remain unchanged
- **AND** no agentprogression, dependency, retained-predicate, owner,
  lifecycle, graph-view, API, or UI migration is included
- **AND** reactivation remains blocked on SemStreams #313
- **AND** Phase 2 replaces agentstore's `sync.Map` current-state reads with
  SemStreams graphview-derived projections
- **AND** Phase-2 XP debit uses an idempotent durable command/request to the
  agentprogression owner rather than a direct Agent replacement
- **AND** Agent lifecycle and status remain outside agentstore authority

### Requirement: UI-facing predicate stability
The quest entity predicates consumed by the dashboard world store SHALL remain
stable across the migration, and any predicate change SHALL ship with an
explicit mapping consumed by the UI in the same change. The affected
predicates include status, claimed-by, output, and timestamps.

#### Scenario: Dashboard renders migrated lifecycle
- **WHEN** the mock-LLM e2e suite drives a quest through post → claim → complete on the migrated stack
- **THEN** the dashboard displays the same status progression it displayed before the migration
