# Tasks: rebuild-quest-lifecycle-on-semstreams-rules

## 1. Stage A — dependency bump and contract adaptation

- [ ] 1.1 Read `semstreams/docs/operations/migration-*.md` for beta.21→beta.132; list every item touching semdragons (ownership ADR-054/055/056, agentrun ADR-053, payload registry, message shapes)
- [ ] 1.2 Bump `go.mod` to semstreams v1.0.0-beta.132; fix compilation across all packages
- [ ] 1.3 Ownership spike: write and read one quest entity under beta.132 ownership/projection rules; document which `quest.*` predicates need ownership claims for rules vs questbridge
- [ ] 1.4 Adapt payload registry, BaseMessage wrapping, and graph-write call sites until `task test` is green with the existing architecture
- [ ] 1.5 Add contract-level integration tests: questbridge↔agentic-loop TaskMessage round trip, questtools↔tool.execute/tool.result round trip
- [ ] 1.6 Resolve the QUEST_LOOPS vs framework agentrun question; document the decision in design.md Open Questions
- [ ] 1.7 `task test:integration` green with the existing architecture (Stage A gate)

## 2. Stage B1 — quest lifecycle as declared workflow

- [ ] 2.1 Define `QuestState` schema struct with `lifecycle` tags and register the quest `Workflow` (transition table per quest-lifecycle spec, terminal phases, `quest.status` as phase predicate)
- [ ] 2.2 Author `config/rules/quest/*.json`: claim/start edges, submission routing (review vs direct completion), failure retry-vs-triage routing
- [ ] 2.3 Implement attempt-conditional routing via lifecycle Manager API where rule conditions cannot express it; attempts increment atomic with claim (`TransitionWith`)
- [ ] 2.4 Port dependency gating (claim blocked until deps completed) and creation-time cycle rejection to the new seam
- [ ] 2.5 Port questboard transition tests to the workflow; delete bespoke transition handlers from `processor/questboard`
- [ ] 2.6 Point questbridge's watch at the lifecycle phase predicate; verify dispatch still fires on `in_progress`

## 3. Stage B2 — DAG execution on gated-dag

- [ ] 3.1 Create `questdag` domain layer: DAG extraction/validation from lead tool output (≤20 nodes, cycle/self-ref/dep-ID checks), unit ID minting, unit entity seeding with `depends_on` edges
- [ ] 3.2 Implement the dispatch-subject handler mapping a gated-dag unit to `ClaimAndStartForParty` on the best-skill-matched member (lead excluded)
- [ ] 3.3 Port the lead review gate (review tool dispatch, accept/reject with required explanation, feedback into retry prompt) and clarification pause/resume
- [ ] 3.4 Port per-node retry budget, parent escalation on exhaustion (cancel remaining nodes), and topological rollup to parent output + party disband
- [ ] 3.5 Re-derive the `quest.dag.*` parent-quest projection from the gated-dag unit model (UI-facing shape preserved or mapped)
- [ ] 3.6 Port questdagexec tests to the new seam; delete the event loop, bootstrap protocol, and dedup caches
- [ ] 3.7 Evaluate whether `partycoord` survives as-is or reduces to rules + questdag; document outcome (no redesign in this change)

## 4. Stage B3 — mechanics disablement and operator surface

- [ ] 4.1 Remove `agentprogression`, `bossbattle`, `agentstore`, `boidengine`, `redteam` from `config/semdragons.json`; verify clean boot and quest flow without them
- [ ] 4.2 Add the stub rule: on transition into `in_review`, fire `lifecycle_transition` to `completed` with `verdict: review-skipped-mechanics-disabled` set atomically
- [ ] 4.3 Enable `lifecycle-gateway` scoped to the quest workflow; verify inspect/history/operator-transition against the running stack
- [ ] 4.4 Delete `service/api` handlers superseded by the gateway (status mutation only; keep world/DM/workspace/creation endpoints)

## 5. Verification gate

- [ ] 5.1 `task test` and `task test:integration` fully green
- [ ] 5.2 Full mock-LLM e2e (`task e2e`) green, including party DAG scenarios
- [ ] 5.3 Dashboard assertions: quest status progression and DAG node states render unchanged (UI predicate stability requirement)
- [ ] 5.4 Update `docs/03-QUESTS.md` lifecycle/DAG sections to point at the new specs; run `openspec validate`; change ready for archive
