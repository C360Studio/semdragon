# E2E Test Suite

End-to-end tests for the Semdragons UI, written with [Playwright](https://playwright.dev/). Tests run
against 3 browsers (Chromium, Firefox, WebKit) and cover UI structure, navigation, accessibility,
backend lifecycle state machines, real-time SSE events, and store mechanics.

## Running the Tests

### Standalone (no backend required)

```bash
npx playwright test
```

Tests that require a live backend call `test.skip(!hasBackend())` and are skipped automatically.
Everything else — page structure, navigation, layout, and accessibility — runs without Docker.

### Full stack (Docker)

The full suite requires the Docker E2E stack: NATS, backend, and the Vite dev server.

**One-shot run (CI / first use):**

```bash
make e2e
```

This installs browsers, starts the stack with seeded test data, waits for the backend health check,
runs all tests across all browsers, then tears the stack down.

**Iterative development:**

```bash
make e2e-up          # Start the stack once (SEED_E2E=true)
make e2e-chromium    # Fast iteration — Chromium only
make e2e-run         # Full cross-browser run (assumes stack is up)
make e2e-down        # Tear down and remove volumes
```

**All Makefile targets:**

| Target | Description |
|---|---|
| `make e2e` | Full lifecycle: install → start → wait → test → stop |
| `make e2e-up` | Start Docker stack with `SEED_E2E=true docker compose up -d --build --wait` |
| `make e2e-wait` | Poll `/health` up to 30 s (used inside `make e2e`) |
| `make e2e-run` | Run Playwright (stack must already be up) |
| `make e2e-chromium` | Run on Chromium only — fastest for iteration |
| `make e2e-install` | Install Playwright browsers (`--with-deps chromium`) |
| `make e2e-down` | Stop stack and delete volumes |

### Environment variables

| Variable | Default | Description |
|---|---|---|
| `SEED_E2E` | — | Set to `true` to seed agents and quests on backend startup |
| `E2E_BACKEND_AVAILABLE` | — | Set automatically by `global-setup.ts`; force to `true` to skip probing |
| `API_URL` | `http://localhost:8080` | Backend URL for health checks and API calls |
| `PUBLIC_SSE_BUCKET` | `semdragons-local-dev-board1` | KV bucket name used by the SSE endpoint |

## Architecture

### Docker stack

```
┌───────────┐     ┌──────────────┐     ┌──────────┐
│  Browser  │────▶│  Vite Dev    │────▶│ Backend  │────▶ NATS
│(Playwright)│     │   :5173      │proxy│  :8080   │     :4222
└───────────┘     └──────────────┘     └──────────┘
```

Vite proxies `/game`, `/health`, and `/message-logger` to the backend container. The backend runs
the full Semdragons stack (9 processors) with E2E seed data injected via `SEED_E2E=true`.

### Global setup

`global-setup.ts` runs once before any tests. It:

1. Probes `GET /health` with a 5 s timeout (retrying every 500 ms).
2. Opens the SSE endpoint (`/message-logger/kv/{bucket}/watch?pattern=*`) and reads the first
   chunk, expecting `event: connected`.
3. Sets `E2E_BACKEND_AVAILABLE=true` only when both checks pass. If either fails, backend-dependent
   tests are skipped rather than failing.

### Test infrastructure

```
e2e/
├── global-setup.ts          # Backend health + SSE probe
├── fixtures/
│   ├── test-base.ts         # Shared fixtures, LifecycleApi, retry helpers
│   └── sse-helpers.ts       # SSE connection and event testing utilities
├── pages/
│   ├── base.page.ts         # Common layout selectors and navigation helpers
│   ├── dashboard.page.ts    # Stats grid, tier distribution, event feed
│   ├── quests.page.ts       # Kanban board, quest cards, details panel
│   └── agents.page.ts       # Agent grid, filters, details panel
└── specs/
    ├── navigation.spec.ts
    ├── dashboard.spec.ts
    ├── quests.spec.ts
    ├── agents.spec.ts
    ├── quest-lifecycle.spec.ts
    ├── agent-lifecycle.spec.ts
    ├── boss-battle.spec.ts
    ├── store-lifecycle.spec.ts
    ├── tier-gates.spec.ts
    ├── world-state.spec.ts
    ├── sse.spec.ts
    └── sse-lifecycle.spec.ts
```

**Page objects** (`pages/`) encapsulate all UI interaction and selector logic. Specs import page
objects rather than writing raw locators, keeping test intent readable and selectors maintainable in
one place.

**Fixtures** (`fixtures/test-base.ts`) extend Playwright's base `test` with:

- `dashboardPage`, `questsPage`, `agentsPage` — instantiated page objects
- `sseHelper` — SSE connection and event assertion utilities
- `lifecycleApi` — fully typed API client for quest and agent state transitions
- `seedQuests` — create test quests in the backend from within a test

## Spec Files

297 tests total: ~99 per browser across 3 browser projects.

### UI structure and navigation

| Spec | What it covers |
|---|---|
| `navigation.spec.ts` | Route navigation, browser history, direct URL loading, page titles |
| `dashboard.spec.ts` | Stats grid, tier distribution chart, live event feed, SSE connection status |

### Entity pages

| Spec | What it covers |
|---|---|
| `quests.spec.ts` | Kanban columns, quest cards (difficulty and XP badges), selection panel, seeded quest data |
| `agents.spec.ts` | Agent roster grid, status filters, selection details, tier badges, XP bars, aria labels |

### Backend lifecycle (API-driven, no UI interaction)

These specs drive the full state machine through the `lifecycleApi` fixture and assert resulting
state via the API. They require the backend and skip otherwise.

| Spec | What it covers |
|---|---|
| `quest-lifecycle.spec.ts` | `posted → claimed → in_progress → completed`, with-review path, abandon, fail-repost |
| `agent-lifecycle.spec.ts` | Recruit, retire, status tracking across the quest lifecycle |
| `boss-battle.spec.ts` | Auto-trigger battle on reviewed-quest submit; no battle without review flag |
| `store-lifecycle.spec.ts` | Catalog listing, individual items, purchase flow, tier gates, inventory, consumable effects |
| `tier-gates.spec.ts` | Apprentice blocked from hard quests; busy agent blocked from claiming |
| `world-state.spec.ts` | World state structure; reflects quest creation |

### Real-time (SSE)

| Spec | What it covers |
|---|---|
| `sse.spec.ts` | Connection status indicator, real-time event feed updates, stat updates, event filtering, connection recovery |
| `sse-lifecycle.spec.ts` | SSE emits quest state changes; connection indicator reflects live stream |

### Accessibility

Accessibility checks are embedded in `agents.spec.ts` and `quests.spec.ts` — aria labels, aria-pressed
states on filter buttons, and role-based selectors are validated as part of the normal UI assertions.

## Test Design Patterns

### Backend isolation

Each lifecycle spec recruits its own fresh agent:

```typescript
const agent = await lifecycleApi.recruitAgent(`Test Agent ${Date.now()}`);
```

This prevents state collisions between tests running in parallel and avoids dependency on shared
seed data for lifecycle assertions.

### Graceful backend skipping

Every backend-dependent test guards with `hasBackend()`:

```typescript
test('claims a quest', async () => {
  if (!hasBackend()) test.skip();
  // ...
});
```

The `hasBackend()` function reads `process.env.E2E_BACKEND_AVAILABLE`, which `global-setup.ts` sets
based on the health and SSE probe results.

### Polling async state

The `retry()` helper polls until an assertion passes or a timeout is reached:

```typescript
import { retry } from '../fixtures/test-base';

const quest = await retry(
  async () => {
    const q = await lifecycleApi.getQuest(questId);
    if (q.status !== 'completed') throw new Error(`status is ${q.status}`);
    return q;
  },
  { timeout: 10000, interval: 500, message: 'Quest never reached completed' }
);
```

Use `retry()` whenever testing state that results from an async processor (boss battle verdict,
XP award, quest status propagation).

### Entity ID extraction

The backend returns full six-part entity IDs (`local.dev.game.board1.quest.abc123`). Lifecycle
endpoints expect only the short instance portion. Use `extractInstance()`:

```typescript
import { extractInstance } from '../fixtures/test-base';

const quest = await lifecycleApi.createQuest('Slay the dragon');
const questId = extractInstance(quest.id); // "abc123"
await lifecycleApi.claimQuest(questId, agentId);
```

### SvelteKit hydration

Before asserting on reactive UI state, call `waitForHydration()` to ensure Svelte 5 runes
(`$state`, `$derived`) are active:

```typescript
import { waitForHydration } from '../fixtures/test-base';

await page.goto('/quests');
await waitForHydration(page);
// Now safe to assert on reactive components
```
