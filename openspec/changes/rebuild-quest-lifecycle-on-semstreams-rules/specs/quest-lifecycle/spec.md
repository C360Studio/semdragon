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
valid transitions through the framework lifecycle gateway mounted at
`/lifecycle-gateway/workflows`. The complete gateway namespace SHALL use the
existing `SEMDRAGONS_API_KEY` authentication semantics. WebSocket streaming
SHALL be disabled. Operator writes SHALL be constrained to operator-writable
fields and table-valid transitions.

Only the `quest` workflow SHALL be registered with the shared lifecycle
manager in this phase, so gateway discovery and direct access expose no other
workflow. This compositional restriction SHALL NOT be represented as a local
replacement for the SemStreams workflow/operation allowlist tracked by
SemStreams #678.

#### Scenario: Operator escalates a stuck quest
- **WHEN** an operator issues an escalate transition for an active quest via the lifecycle gateway
- **THEN** the quest transitions to `escalated` with the transition source recorded as operator

#### Scenario: Gateway namespace is authenticated

- **WHEN** a client calls `/lifecycle-gateway/workflows` or any descendant
  while `SEMDRAGONS_API_KEY` is configured
- **THEN** missing or incorrect credentials are rejected
- **AND** the configured key is accepted through `X-API-Key` or Bearer
  authentication
- **AND** unrelated public routes are not captured by the gateway middleware

#### Scenario: Development authentication passthrough is preserved

- **WHEN** `SEMDRAGONS_API_KEY` is empty
- **THEN** gateway requests use the existing development passthrough behavior

#### Scenario: Quest-only composition and streaming policy

- **WHEN** an authenticated operator discovers the deployed gateway
- **THEN** `quest` is the only exposed workflow because it is the only
  workflow registered with the shared lifecycle manager
- **AND** WebSocket streaming is unavailable

#### Scenario: Operator transition and mechanics stub share one history

- **GIVEN** a quest is `in_progress`
- **WHEN** an authenticated operator transitions it to `in_review`
- **THEN** history records the operator source and note
- **AND** the mechanics-stub rule atomically transitions the same quest to
  `completed` with `verdict=review-skipped-mechanics-disabled`
- **AND** history records the rule transition after the operator transition

### Requirement: Product quest commands coexist with the operator gateway

The lifecycle gateway SHALL be treated as an operator and break-glass surface
until it can preserve every product invariant and transition-coupled field
write. Existing `/game/quests/{id}/...` command handlers SHALL remain the
supported product command surface during task 4.3. Their retirement SHALL
remain task 4.4 and SHALL depend on the SemStreams atomic transition-field
primitive tracked by SemStreams #677.

#### Scenario: Product commands remain available

- **WHEN** the lifecycle gateway is enabled
- **THEN** existing quest claim, start, submit, complete, fail, abandon, and
  cancel command behavior remains available
- **AND** no product command handler is deleted or redirected as part of task
  4.3
- **AND** SemStreams #679 and #680 remain the upstream owners for runtime
  gateway paths and OpenAPI security policy without blocking runtime
  authentication
- **AND** SemStreams #681 remains the owner of the revision-retention
  invariant required for durable lifecycle history
- **AND** SemStreams #682 remains the owner of the gateway's documented 413
  versus observed 400 oversized-body response

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

#### Scenario: The backend image packages the configured rule pack

- **WHEN** a shipped profile starts inside the backend container
- **THEN** every configured `config/rules/quest/*.json` file is present at its
  resolved runtime path
- **AND** an operator transition to `in_review` is completed by the packaged
  mechanics-stub rule
- **AND** the first completed state includes
  `review-skipped-mechanics-disabled`

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

### Requirement: Redteam is inactive during Phase-1 review completion

Every shipped runtime profile SHALL retain redteam configuration with
`enabled: false`. Review-required work SHALL complete through the supported
explicit claim/start, questbridge, and mechanics-stub path without creating
redteam runtime state or side effects.

#### Scenario: Mechanics-stub remains the shipped review resolver

- **WHEN** a review-required Quest is explicitly claimed and started in the
  default, semsource, or tier-1 E2E profile
- **THEN** no redteam ComponentManager instance, watcher, pending review, or
  local deadline exists
- **AND** mechanics-stub completes the Quest with
  `review-skipped-mechanics-disabled`
- **AND** no red-team Quest, status mutation, Guild lesson, or redteam event is
  produced

#### Scenario: Quarantine preserves the dormant Phase-2 boundary

- **WHEN** issue #36 completes
- **THEN** only the two false redteam OutputPorts are removed
- **AND** category/vocabulary, Quest/API/UI, finding/lesson, prompt, and opt-in
  E2E contracts remain
- **AND** the legacy underscore watcher, raw caches, unsafe full-object
  writers, inert claim timeout, and local deadlines remain inactive and
  deferred
- **AND** reactivation remains blocked on Semdragon #4, #5, and #22 plus
  SemStreams #313 and #676
- **AND** no dependency, graphview, ownership, writer, lifecycle, rule,
  schema, API/UI, or durable-deadline migration is included

### Requirement: UI-facing predicate stability
The quest entity predicates consumed by the dashboard world store SHALL remain
stable across the migration, and any predicate change SHALL ship with an
explicit mapping consumed by the UI in the same change. The affected
predicates include status, claimed-by, output, and timestamps.

#### Scenario: Dashboard renders migrated lifecycle
- **WHEN** the mock-LLM e2e suite drives a quest through post → claim → complete on the migrated stack
- **THEN** the dashboard displays the same status progression it displayed before the migration
