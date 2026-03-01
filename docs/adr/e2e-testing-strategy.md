# ADR: E2E Testing Strategy — Mock LLM and Cross-Agent Coordination

## Status
Accepted (Phase 0–3 implemented, future phases tracked)

## Context

### What We Built

The Semdragons framework has significant game mechanics that are entirely deterministic: quest lifecycle, boss battles with `DefaultBattleEvaluator`, XP calculations, tier gates, and world state aggregation. The only LLM boundaries are the **Agent Executor** (runs quests) and **Party Lead decomposition/rollup** — everything else is pure math and state machines.

We implemented full E2E coverage of the deterministic path without spending a single LLM token:

| Phase | What | Status |
|-------|------|--------|
| 0 | Fix `Quest.Triples()` + reconstruction + review hints | Done |
| 1 | 6 quest lifecycle API endpoints | Done |
| 2 | 6 Playwright E2E spec files | Done |
| 3 | Seed data (agents, quests, store items) | Done |

### The LLM Boundary

```
┌──────────────────────────────────────────────────────────┐
│                    DETERMINISTIC                         │
│  Quest lifecycle, boss battles (DefaultBattleEvaluator), │
│  XP engine, tier gates, boid engine, world state,        │
│  store purchase/use, guild mechanics                     │
│                                                          │
│  ════════════════ E2E COVERAGE LINE ═══════════════════  │
│                                                          │
│                    LLM BOUNDARY                          │
│  Agent Executor (quest execution), Party Lead            │
│  (decomposition/rollup)                                  │
└──────────────────────────────────────────────────────────┘
```

Everything above the line is testable via API + `DefaultBattleEvaluator` (output exists = 0.8 score, auto-pass). Everything below requires actual LLM calls and is deferred.

### Sister Agent Work In Flight

Two sister agents are implementing changes that affect E2E and UI:

1. **Agent Autonomy Loop** (`docs/adr/agent-autonomy-loop.md`) — fixes broken agent status lifecycle, adds autonomy processor with per-agent heartbeats
2. **Store Seeding** — `DefaultCatalog()` with 10 items (5 tools + 5 consumables), `seedStore()` in E2E seeder, guild discount wiring

This ADR captures what exists, what's coming, and how the pieces fit together.

## Decision

### Current E2E Architecture

#### API Endpoints (Phase 1)

6 lifecycle endpoints added to `service/api/`:

| Method | Path | Handler | Validation |
|--------|------|---------|------------|
| `POST` | `/quests/{id}/claim` | `handleClaimQuest` | Agent idle, tier >= minTier, has required skills |
| `POST` | `/quests/{id}/start` | `handleStartQuest` | Quest is claimed |
| `POST` | `/quests/{id}/submit` | `handleSubmitResult` | Quest is in_progress; routes to in_review or completed |
| `POST` | `/quests/{id}/complete` | `handleCompleteQuest` | DM override; quest is in_review or in_progress |
| `POST` | `/quests/{id}/fail` | `handleFailQuest` | Increments attempts; reposts if retries remain |
| `POST` | `/quests/{id}/abandon` | `handleAbandonQuest` | Returns quest to posted |

Agent status management is inline: `handleClaimQuest` sets `AgentOnQuest`, `releaseAgent` sets `AgentIdle`. This is correct for the API layer and will coexist with processor-level writes from the autonomy ADR (see [Coordination with Autonomy ADR](#coordination-with-agent-autonomy-adr)).

#### Store Endpoints (Stubbed)

Routes registered but returning 501 Not Implemented:

| Method | Path | Status |
|--------|------|--------|
| `GET` | `/store` | Returns `[]` (empty array) |
| `GET` | `/store/{id}` | 501 |
| `POST` | `/store/purchase` | 501 |
| `GET` | `/agents/{id}/inventory` | 501 |
| `POST` | `/agents/{id}/inventory/use` | 501 |
| `GET` | `/agents/{id}/effects` | 501 |

The `agentstore` processor component has full `Purchase()`, `UseConsumable()`, `ConsumeEffect()` methods but the API service doesn't hold a reference to the component instance. Wiring these is a prerequisite for store E2E tests.

#### Seed Data (Phase 3)

`cmd/semdragons/seed_e2e.go` seeds when `SEED_E2E=true`:

| Entity | Count | Details |
|--------|-------|---------|
| Guilds | 2 | Data Wranglers, Code Smiths |
| Agents | 9 | Across all 5 tiers (apprentice through grandmaster) |
| Quests | 3 | e2e-easy, e2e-review (RequireReview=true), e2e-hard (requires CodeGen) |
| Store items | 10 | 5 tools + 5 consumables from `DefaultCatalog()` |

#### E2E Specs (Phase 2)

| Spec | Tests | What it covers |
|------|-------|---------------|
| `quest-lifecycle.spec.ts` | 4 | Full state machine: no-review, with-review, abandon, fail-repost |
| `boss-battle.spec.ts` | 2 | Auto-trigger with DefaultBattleEvaluator, no-battle without review |
| `agent-lifecycle.spec.ts` | 3 | Recruit, retire, status tracking during quest |
| `tier-gates.spec.ts` | 2 | Apprentice blocked from hard quest (403), double-claim blocked (409) |
| `world-state.spec.ts` | 2 | Structural validity, quest creation reflection |
| `sse-lifecycle.spec.ts` | 2 | SSE quest updates on page, connection indicator |

#### Test Fixture

`ui/e2e/fixtures/test-base.ts` provides a `lifecycleApi` fixture:
- **Native `fetch`** for lifecycle operations (claim/start/submit/complete/fail/abandon) — returns raw `Response` for HTTP status code assertions (403, 409)
- **Playwright `apiContext`** for CRUD operations (createQuest, recruitAgent, getQuest, getAgent, getWorldState, listBattles)
- `extractInstance(fullId)` — extracts instance from 6-part entity ID
- `retry(fn, opts)` — polls with timeout for async state propagation

#### Fixes (Phase 0)

| Fix | File | Problem |
|-----|------|---------|
| `quest.review.needs_review` triple | `graphable.go` | Root `Quest.Triples()` didn't emit it; boss battles could never auto-trigger |
| `RequireReview` reconstruction | `reconstruction.go` | `QuestFromEntityState()` didn't reconstruct from triples |
| Review hints in `handleCreateQuest` | `service/api/handlers.go` | `require_human_review` and `review_level` hints were ignored |
| Schema regeneration | `entity-schema.generated.json` | New predicate not in generated schema |

### Store System: Current State and Gaps

The `agentstore` processor is a complete component with purchase, use, and effect lifecycle:

**Default Catalog (10 items):**

| ID | Type | XP Cost | Min Tier | Guild Discount | Notes |
|----|------|---------|----------|----------------|-------|
| web_search | tool | 50 | Apprentice | — | Permanent |
| code_reviewer | tool | 150 | Journeyman | 10% | Permanent |
| deploy_access | tool | 500 | Expert | 20% | Permanent |
| context_expander | tool | 200 | Journeyman | — | Rental (10 uses) |
| parallel_executor | tool | 750 | Expert | — | Level 13+ |
| retry_token | consumable | 50 | Apprentice | — | 1 use |
| cooldown_skip | consumable | 75 | Apprentice | — | Ends cooldown |
| xp_boost | consumable | 100 | Journeyman | 15% | 2x XP, 1 quest |
| quality_shield | consumable | 150 | Journeyman | — | Protects 1 review criterion |
| insight_scroll | consumable | 50 | Apprentice | — | 3 quests |

**What works:**
- Items seeded to KV via `seedStore()` — visible to world state API and dashboard
- `agentstore.Purchase()` applies guild discounts, validates tier/XP, emits events
- `agentstore.UseConsumable()` activates effects with duration tracking
- All operations emit typed predicates (`store.item.purchased`, `store.consumable.used`, etc.)

**What's missing for E2E:**
1. **API wiring** — Store handlers return 501. Need to inject `agentstore.Component` reference into the API service
2. **Store E2E spec** — No `store.spec.ts` exists
3. **Purchase flow** — Cannot test purchase via API because `handlePurchase` is stubbed
4. **Consumable use flow** — Cannot test use via API because `handleUseConsumable` is stubbed
5. **Store UI page** — `/store` route exists but likely displays static/empty content

### Coordination with Agent Autonomy ADR

The autonomy ADR (`docs/adr/agent-autonomy-loop.md`) introduces changes that affect E2E testing at three levels:

#### Level 1: Agent Status Becomes Real (Impacts Existing Specs)

Currently, our API handlers manage agent status inline. The ADR adds processor-level status writes:

| Transition | Our API Handler | ADR Processor |
|------------|----------------|---------------|
| idle → on_quest | `handleClaimQuest` sets it | questboard processor also sets it |
| on_quest → idle | `releaseAgent` on complete | agentprogression processor sets it |
| on_quest → in_battle | (not handled) | bossbattle processor sets it |
| in_battle → idle | (not handled) | bossbattle processor sets it |
| on_quest → cooldown | (not handled) | agentprogression on fail + penalty |
| cooldown → idle | (not handled) | autonomy processor on expiry |

**Coexistence strategy**: API writes and processor writes target the same entity field. Both use `EmitEntityUpdate` which does a KV Put. The last writer wins, and both writers agree on the correct value. No conflict. When the autonomy ADR is fully wired, the API-level writes become redundant but harmless. We can remove them in a cleanup pass.

**Spec updates needed when autonomy lands:**

- `agent-lifecycle.spec.ts`: Add tests for `in_battle` and `cooldown` states
- `boss-battle.spec.ts`: Verify agent status is `in_battle` during review

#### Level 2: Autonomous Actions (New E2E Territory)

The autonomy processor enables agents to act without API calls — claiming quests from boid suggestions, shopping strategically, using consumables. This is testable via observable predicates:

- `agent.autonomy.evaluated` — heartbeat fired
- `agent.autonomy.claimintent` — agent wants to claim
- `agent.autonomy.shopintent` — agent wants to shop
- `agent.autonomy.useintent` — agent wants to use consumable

E2E tests can verify autonomy by watching SSE for these predicates after seeding appropriate conditions.

### Future E2E Phases

#### Phase 4: Wire Store API Endpoints

**Prerequisite**: Inject `agentstore.Component` into API service.

| Endpoint | Implementation |
|----------|---------------|
| `GET /store` | `component.Catalog()` or `component.ListItems(tier)` |
| `GET /store/{id}` | Lookup from catalog |
| `POST /store/purchase` | `component.Purchase(ctx, agentID, itemID)` |
| `GET /agents/{id}/inventory` | Read from KV or component cache |
| `POST /agents/{id}/inventory/use` | `component.UseConsumable(ctx, agentID, itemID, questID)` |
| `GET /agents/{id}/effects` | Read active effects |

#### Phase 5: Store E2E Spec

```
store-lifecycle.spec.ts:
  test('list store returns seeded catalog')
    GET /store → 10 items, verify structure

  test('apprentice can purchase web_search tool')
    1. Recruit agent (starts with 0 XP — needs XP injection or seeded agent)
    2. POST /store/purchase { agent_id, item_id: "web_search" }
    3. GET /agents/{id}/inventory → owns web_search

  test('journeyman gets guild discount on xp_boost')
    1. Use seeded journeyman agent in Data Wranglers guild
    2. POST /store/purchase { agent_id, item_id: "xp_boost" }
    3. Verify XP deducted = 85 (100 - 15% guild discount)

  test('tier gate blocks apprentice from quality_shield')
    1. Apprentice agent tries to purchase quality_shield (Journeyman tier)
    2. Expect 403 or error response

  test('use consumable activates effect')
    1. Agent with xp_boost in inventory
    2. POST /agents/{id}/inventory/use { item_id: "xp_boost", quest_id: "..." }
    3. GET /agents/{id}/effects → active xp_boost effect
```

#### Phase 6: Autonomy E2E Spec (After Autonomy ADR Lands)

```
agent-autonomy.spec.ts:
  test('idle agent auto-claims quest via boid suggestion')
    1. Seed idle agent + posted quest matching agent's skills
    2. Wait for SSE event: agent.autonomy.claimintent or quest.lifecycle.claimed
    3. Verify quest is claimed by the agent

  test('agent status transitions through full lifecycle')
    1. Observe agent: idle → on_quest → in_battle → idle
    2. Verify each status via GET /agents/{id}

  test('cooldown expires and agent returns to idle')
    1. Trigger quest failure with penalty (short cooldown for testing)
    2. Agent enters cooldown
    3. Wait for cooldown expiry → agent status = idle

  test('on-quest agent buys quality shield strategically')
    1. Agent on quest with enough XP
    2. Wait for agent.autonomy.shopintent (quality_shield)
    3. Verify purchase in inventory
```

### UI Implications

The autonomy ADR and store system together make several UI features meaningful:

| Feature | Current State | After Autonomy + Store |
|---------|--------------|----------------------|
| Agent status display | Always shows "idle" | Real status: idle, on_quest, in_battle, cooldown |
| Cooldown timer | No cooldown exists | `CooldownUntil` timestamp enables countdown |
| Autonomy event feed | No autonomy events | `agent.autonomy.*` predicates via SSE |
| Store page | Empty/static | Live catalog from seeded data |
| Agent inventory | Not displayed | Tools + consumables from purchase history |
| Guild discount indicators | Discount exists in data | Visible in store UI per item |
| World stats accuracy | Status counts meaningless | Real distribution across 5 states |

## Consequences

### Positive

- Full quest lifecycle tested E2E without LLM token spend
- `DefaultBattleEvaluator` serves as deterministic mock for review flow
- Seed data creates a rich initial state for both API and UI testing
- Store catalog seeded to KV provides immediate UI visibility
- Clear phasing: each future phase has explicit prerequisites and can be picked up independently
- API-level and processor-level status writes coexist without conflict

### Negative

- Store API endpoints are stubbed — cannot test purchase/use flow until `agentstore.Component` is injected into API service
- Agent status management is duplicated between API handlers and processors (temporary, cleaned up after autonomy ADR)
- E2E specs for autonomy depend on the autonomy processor existing — tests must be added after that ADR lands

### Risks

- **Store API injection**: The `agentstore.Component` runs as a processor, not as a library the API service can call directly. Wiring it requires either: (a) passing the component instance to the API service at startup, or (b) reading store state from KV (the entity-centric approach — no component reference needed).

## References

- `service/api/handlers.go` — Lifecycle endpoints (Phase 1)
- `service/api/handlers_test.go` — Handler test coverage
- `cmd/semdragons/seed_e2e.go` — E2E seed data (Phase 3)
- `ui/e2e/specs/` — 6 E2E spec files (Phase 2)
- `ui/e2e/fixtures/test-base.ts` — Test fixtures with `lifecycleApi`
- `processor/agentstore/store.go` — `DefaultCatalog()`, `StoreItem`, purchase logic
- `processor/agentstore/component.go` — Processor component with full store operations
- `docs/adr/agent-autonomy-loop.md` — Sister ADR: agent status lifecycle + autonomy processor
- `graphable.go` — `Quest.Triples()` fix for `quest.review.needs_review` (Phase 0)
- `reconstruction.go` — `QuestFromEntityState()` fix for `RequireReview` (Phase 0)
