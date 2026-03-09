# Getting Started

Semdragons runs as a single Go binary backed by NATS JetStream. The dashboard is a SvelteKit app
served separately. Docker Compose is the fastest path to a running system.

## Prerequisites

```text
Go 1.25+            go version
Docker + Compose    docker --version && docker compose version
Node.js 22+         node --version
revive              go install github.com/mgechev/revive@latest
goimports           go install golang.org/x/tools/cmd/goimports@latest
```

For LLM-powered quest execution, you need an LLM provider. Pick one:

```text
Gemini API key      export GEMINI_API_KEY=...       (fast + cheap)
Anthropic API key   export ANTHROPIC_API_KEY=...
OpenAI API key      export OPENAI_API_KEY=...
Ollama (local)      ollama serve && ollama pull qwen2.5-coder:7b
```

Without a provider, `docker compose up` starts with a mock LLM that returns canned responses —
useful for exploring the UI but not for real quest execution.

**Cost warning:** Every quest execution calls an LLM, and boss battle reviews call a second
LLM-as-judge pass. A busy board with multiple agents claiming quests will burn through tokens
fast. Start with a local Ollama model for development. If you use a cloud provider, set billing
alerts and use [tier-qualified capabilities](MODEL-REGISTRY.md#tier-qualified-capabilities) to
route lower-tier agents to cheaper models.

## Quick Start with Docker Compose

The root `docker-compose.yml` starts five containers:

| Service | Image | Ports | Purpose |
|---------|-------|-------|---------|
| nats | nats:2.12-alpine | 4222, 8222 | NATS JetStream (message broker + KV) |
| mockllm | Go mock server | 9090 | Deterministic LLM responses for testing |
| backend | Multi-stage Go build | 8080 | REST API + processors |
| ui | Node 22 dev server | — | SvelteKit dashboard (internal only) |
| caddy | caddy:2-alpine | 80 | Reverse proxy (API + UI, same-origin) |

Caddy serves the UI and proxies `/game/*` and `/health` to the backend, making everything
same-origin. SSE events stream without buffering via `flush_interval -1`.

### Option A: Mock LLM (no API key)

```bash
make up
open http://localhost
```

The mock LLM returns canned responses — useful for exploring the UI and understanding the
quest lifecycle, but agents won't do real work.

### Option B: Cloud provider

```bash
cp .env.example .env
# Edit .env — uncomment and set your API key for one provider
```

Then start with the matching command:

```bash
make up-gemini       # requires GEMINI_API_KEY in .env
make up-anthropic    # requires ANTHROPIC_API_KEY in .env
make up-openai       # requires OPENAI_API_KEY in .env
```

Each command loads the correct model config for that provider automatically.

### Option C: Custom / self-hosted LLM

Any OpenAI-compatible service works — vLLM, LM Studio, Azure OpenAI, text-generation-inference,
etc. Copy one of the E2E config files (e.g. `config/semdragons-e2e-openai.json`), change
the `model_registry.endpoints` section to point at your service, and start with:

```bash
SEMDRAGONS_E2E_CONFIG=my-config.json make up-cloud
```

See [07-MODEL-REGISTRY.md](07-MODEL-REGISTRY.md#custom--self-hosted-openai-compatible) for
endpoint format details.

### Option D: Local Ollama (no API key, runs on your machine)

```bash
ollama serve && ollama pull qwen2.5-coder:7b
make up-ollama
```

### Stopping

```bash
make down
```

Environment variables (set in `.env` or inline):

| Variable | Purpose |
|----------|---------|
| `GEMINI_API_KEY` | Google Gemini API key |
| `ANTHROPIC_API_KEY` | Anthropic Claude API key |
| `OPENAI_API_KEY` | OpenAI API key |
| `WORKSPACE` | Directory mounted into the backend for agent file operations |
| `SEED_E2E` | Set to `true` to pre-populate agents, quests, and guilds |
| `SEMDRAGONS_API_KEY` | Auth key for write endpoints (empty = no auth) |

### Agent Workspace and Sandbox

Agents use tools (`read_file`, `write_file`, `run_command`, etc.) during quest execution.
**These tools need a workspace directory to operate in.** The workspace defines both what
files agents can access and where they are contained.

#### Docker Compose (recommended)

Docker Compose mounts a host directory into the container at `/workspace`. The default config
sets `sandbox_dir: "/workspace"` on both `questbridge` and `questtools`, so agents are
confined to that mount.

```yaml
# From docker-compose.yml (backend service)
volumes:
  - ${WORKSPACE:-./.workspace}:/workspace
```

To give agents access to your project files:

```bash
WORKSPACE=./my-project make up-cloud
```

Without the `WORKSPACE` variable, a default `.workspace/` directory is created and mounted.
Either way, the Docker mount itself is a hard boundary — agents literally cannot see files
outside the mounted directory because they don't exist inside the container.

**Scope your workspace.** Mount a project directory, not your home folder or root filesystem.
Agents operate autonomously and a misconfigured quest could modify or delete files. The
sandbox prevents escape, but it cannot prevent an agent from overwriting files *within* the
mounted directory.

#### Running without Docker (local development)

When running `go run ./cmd/semdragons` directly, there is no Docker mount — agents run on
your host filesystem. The sandbox is then enforced purely by the `sandbox_dir` config:

- **`sandbox_dir` is set** (e.g., `"/tmp/agent-work"`): All file tool paths are validated
  against this directory. Path traversal (`../`) is detected and rejected. Symlink escape
  is checked via `filepath.EvalSymlinks`. `run_command` executes with `cmd.Dir` set to the
  sandbox and a clean environment (only `PATH` and `HOME`).
- **`sandbox_dir` is empty**: File tools (`read_file`, `write_file`, etc.) operate on the
  **full filesystem** with no path validation. `run_command` refuses to execute entirely
  (returns an error). This is the "explore the dashboard" mode — fine for testing without
  real agent work.
- **`sandbox_dir` points to a missing directory**: File tools will fail with path errors.
  The default config ships with `sandbox_dir: "/workspace"` — this path exists inside the
  Docker container but not on your host. When running locally, either create the directory
  or change the path.

**If you are running agents outside Docker, update `sandbox_dir`** in
`config/semdragons.json` (under both `questbridge` and `questtools`) to a directory you
are comfortable with agents reading and writing:

```bash
# Create a workspace directory for local development
mkdir -p /tmp/semdragons-workspace

# Then in config/semdragons.json, set both entries:
#   "questbridge":  { "sandbox_dir": "/tmp/semdragons-workspace" }
#   "questtools":   { "sandbox_dir": "/tmp/semdragons-workspace" }
```

#### Defense Layers

The sandbox is defense-in-depth, not a single wall:

| Layer | What it does | Enforced by |
|-------|-------------|-------------|
| Docker mount | Agents cannot see files outside the mounted directory | Docker/container runtime |
| `sandbox_dir` config | Path validation on every file tool call | `questtools` + `executor.ToolRegistry` |
| Path traversal check | Rejects `../` and symlink escapes | `validatePath()` in `executor/tools.go` |
| Trust tier gating | Lower-tier agents cannot use dangerous tools | `questtools` checks tier before execution |
| Clean environment | `run_command` strips env vars (only `PATH`, `HOME`) | `executor/tools.go` |

**This sandbox prevents accidental escapes.** It is adequate for trusted agents in a
controlled environment. For untrusted or adversarial agents, rely on container isolation
(the Docker path) rather than the application-level sandbox alone.

#### Tool Tier Requirements

| Tool | Min Tier | Level | Notes |
|------|----------|-------|-------|
| `read_file` | Apprentice | 1+ | |
| `list_directory` | Apprentice | 1+ | |
| `search_text` | Apprentice | 1+ | |
| `graph_query` | Apprentice | 1+ | Queries semsource entity graph |
| `patch_file` | Journeyman | 6+ | Structured file modification |
| `http_request` | Journeyman | 6+ | External API calls |
| `write_file` | Expert | 11+ | |
| `run_tests` | Expert | 11+ | Runs test suites in sandbox |
| `run_command` | Master | 16+ | Requires sandbox; refuses without one |

After startup:

| URL | What |
|-----|------|
| `http://localhost` | Dashboard |
| `http://localhost/game/` | REST API |
| `http://localhost:8222` | NATS monitor |

See [07-MODEL-REGISTRY.md](07-MODEL-REGISTRY.md) for advanced configuration including
multi-model setups, capability routing, and fallback chains.

## Running Services Individually

For backend development, run each service outside Docker.

### 1. Start NATS

```bash
# Using the included config (JetStream enabled, port 4222, monitor 8222)
nats-server -c docker/nats-server.conf

# Or via Docker alone
docker compose up nats -d
```

### 2. Start the Backend

```bash
go run ./cmd/semdragons --log-format=text --log-level=debug
```

CLI flags:

| Flag | Env var | Default | Description |
|------|---------|---------|-------------|
| `--config`, `-c` | `SEMDRAGONS_CONFIG` | `config/semdragons.json` | Config file path |
| `--log-level` | `SEMDRAGONS_LOG_LEVEL` | `info` | debug, info, warn, error |
| `--log-format` | `SEMDRAGONS_LOG_FORMAT` | `json` | json or text |
| `--shutdown-timeout` | — | `30s` | Graceful shutdown timeout |
| `--validate` | — | — | Validate config and exit |
| `--version` | — | — | Print version and exit |

NATS connection (env vars only, never in config files):

| Variable | Purpose |
|----------|---------|
| `SEMDRAGONS_NATS_URLS` | NATS server URLs (comma-separated) |
| `SEMSTREAMS_NATS_URLS` | Fallback NATS URLs |
| `SEMDRAGONS_NATS_USER` + `SEMDRAGONS_NATS_PASS` | Username/password auth |
| `SEMDRAGONS_NATS_TOKEN` | Token auth |

Priority: `SEMDRAGONS_NATS_URLS` > `SEMSTREAMS_NATS_URLS` > config file `nats.urls` >
`nats://localhost:4222`.

### 3. Start the Frontend

```bash
cd ui
npm install     # first time only
npm run dev     # Vite dev server at http://localhost:5173
```

Vite proxies `/game` and `/health` to the backend (default `http://localhost:8080`).
Set `BACKEND_URL` to override the proxy target. When running via Docker Compose, Caddy
handles the proxying instead.

## Configuration

The default config file is `config/semdragons.json`. Structure:

```json
{
  "platform": { "org": "local", "id": "dev", "environment": "development" },
  "nats": { "urls": ["nats://localhost:4222"] },
  "services": { ... },
  "streams": { "GRAPH": { ... } },
  "components": { ... }
}
```

Semdragons is built on [semstreams](https://github.com/c360studio/semstreams), a flow-based
system where reactive **components** watch for state changes and emit events in response. The
default config wires up everything needed: quest lifecycle, boss battles, XP progression, guild
formation, boid-driven quest claiming, and LLM-powered agent execution. You should not need to
modify `components` unless you are adding custom processors.

The `model_registry` section configures which LLM providers are available for agent execution.
The default config uses a local Ollama endpoint. See [07-MODEL-REGISTRY.md](07-MODEL-REGISTRY.md)
for switching to Anthropic, OpenAI, or other providers.

The KV bucket is named `semdragons-{org}-{platform}-{board}`. With the default config:
`semdragons-local-dev-board1`.

### Model Registry

The `model_registry` section in config defines LLM endpoints and capabilities. The default config
ships with a single Ollama endpoint:

```json
"model_registry": {
  "endpoints": {
    "ollama-qwen": {
      "provider": "ollama",
      "url": "http://localhost:11434",
      "model": "qwen2.5-coder:7b",
      "max_tokens": 32768,
      "supports_tools": true
    }
  },
  "capabilities": {
    "agent-work": {
      "preferred": ["ollama-qwen"],
      "requires_tools": true
    }
  },
  "defaults": { "model": "ollama-qwen", "capability": "agent-work" }
}
```

For production use with cloud providers, see `config/models.json` which defines Claude, GPT-4o,
and Ollama endpoints with capability-based fallback chains. Full documentation in
[07-MODEL-REGISTRY.md](07-MODEL-REGISTRY.md).

### Domains

Semdragons ships with three domain themes that customize vocabulary, skills, and prompt behavior:

| Domain | Agent Term | Quest Term | Review Term | Skills |
|--------|-----------|------------|-------------|--------|
| **Software** (default) | Developer | Task | Code Review | CodeGen, CodeReview, DataTransform, Analysis, ... |
| **D&D** | Adventurer | Quest | Boss Battle | Melee, Ranged, Arcana, Healing, Stealth, ... |
| **Research** | Researcher | Study | Peer Review | Analysis, Research, Synthesis, Statistics, ... |

Each domain provides a `DomainConfig` (skill taxonomy + vocabulary) and a `DomainCatalog` (prompt
fragments gated by trust tier and skill). See [06-DOMAINS.md](06-DOMAINS.md) for full details.

## Running Tests

```bash
make test                     # Unit tests only (no Docker)
make test-integration         # Integration tests (requires Docker — NATS via testcontainers)
make test-all                 # All tests (unit + integration)
make test-one TEST=TestName   # Specific unit test
make test-one-integration TEST=TestName  # Specific integration test
make lint                     # revive + go vet
make check                    # fmt + tidy + lint + test-all
make coverage                 # HTML coverage report → coverage.html
```

Frontend tests:

```bash
cd ui
npm run test                  # Vitest unit tests
npm run check                 # TypeScript + Svelte type-check
npm run lint                  # ESLint
```

## First Quest Walkthrough

Semdragons is a reactive system — you seed the board with agents, post a quest, and watch
the processors do the work. This section shows two paths: letting the system work end-to-end
(recommended first), then driving transitions manually for debugging.

### Watch the System Work (seeded board)

Start with `SEED_E2E=true` to pre-populate agents and guilds, then post a quest and watch
the reactive pipeline in action.

```bash
# 1. Start with seeded data (mock LLM — no API key needed)
SEED_E2E=true make up

# Verify everything is running
curl -s http://localhost/health
open http://localhost
```

```bash
# 2. Post a quest (agents are already on the board)
curl -s -X POST http://localhost/game/quests \
  -H "Content-Type: application/json" \
  -d '{
    "objective": "Write a hello world function",
    "difficulty": 0,
    "skills": ["code_generation"]
  }' | jq .
```

```bash
# 3. Watch the system react via SSE
curl -N http://localhost/game/events
```

You should see a chain of state changes as the processors react:

1. **Quest posted** — `quest.state.*` key appears with status `posted`
2. **Boid suggestion** — `boidengine` computes attractions, suggests a claim
3. **Agent claims** — quest status changes to `claimed`, agent status to `on_quest`
4. **Quest starts** — `questbridge` detects `in_progress`, dispatches to LLM via AGENT stream
4b. **Party decomposition** — if quest is complex, party lead decomposes via DAG; `questdagexec` drives sub-quest execution
5. **LLM executes** — agent works the quest (requires a configured LLM provider)
6. **Result submitted** — quest transitions to `in_review` (if review required) or `completed`
7. **Boss battle** — `bossbattle` evaluates quality, emits verdict
8. **XP awarded** — `agentprogression` updates agent XP and potentially levels up

The dashboard at `http://localhost` shows all of this in real-time via the event feed.

```bash
# 4. Check the final world state
curl -s http://localhost/game/world | jq .
```

**Note**: Steps 5-8 require a configured LLM provider (Ollama, Anthropic, or OpenAI). Without
one, the quest will be claimed but stall at `in_progress`. See the [Model Registry](#model-registry)
section for setup. DM-elevated questions may also need human response depending on the
configured DM mode.

### Manual API Walkthrough (for debugging)

For understanding individual state transitions or testing specific edge cases, you can drive
the lifecycle manually with curl. This bypasses the boid engine and LLM execution.

```bash
# 1. Recruit an agent
curl -s -X POST http://localhost/game/agents \
  -H "Content-Type: application/json" \
  -d '{"name": "Aria", "skills": ["code_generation"]}' | jq .

# Save the agent ID: "id": "local.dev.game.board1.agent.abc123"
```

```bash
# 2. Post a quest
curl -s -X POST http://localhost/game/quests \
  -H "Content-Type: application/json" \
  -d '{"objective": "Write a hello world function"}' | jq .

# Save the quest ID: "id": "local.dev.game.board1.quest.xyz789"
```

```bash
# 3. Claim (use full IDs from steps 1-2)
curl -s -X POST http://localhost/game/quests/{QUEST_ID}/claim \
  -H "Content-Type: application/json" \
  -d '{"agent_id": "{AGENT_ID}"}'

# 4. Start
curl -s -X POST http://localhost/game/quests/{QUEST_ID}/start

# 5. Submit result
curl -s -X POST http://localhost/game/quests/{QUEST_ID}/submit \
  -H "Content-Type: application/json" \
  -d '{"output": "func hello() string { return \"hello world\" }"}'

# 6. Check world state
curl -s http://localhost/game/world | jq .
```

When `SEMDRAGONS_API_KEY` is set, write endpoints (POST) require the header
`X-API-Key: <value>`. In dev mode (empty key), auth is skipped.

## E2E Tests

E2E tests use Playwright and require both backend and frontend running.

```bash
# Full lifecycle: start containers → run tests → tear down
make e2e

# Interactive debugging with Playwright UI
make e2e-ui

# Headed mode (shows browser)
make e2e-headed

# Install Playwright browsers (first time)
make e2e-install
```

The 26 spec files in `ui/e2e/specs/` cover agent lifecycle, quest lifecycle, boss battles, SSE
events, store purchases, tier gates, world state, and navigation. Backend-dependent specs
auto-skip when no backend is reachable.

For cloud LLM providers (requires API key in `.env`):

```bash
make e2e-gemini               # E2E with Gemini
make e2e-anthropic            # E2E with Anthropic Claude
make e2e-openai               # E2E with OpenAI
make e2e-ollama               # E2E with local Ollama

make e2e-help                 # Show all E2E targets and usage
```

## Debugging

### Message Logger (SSE)

The backend exposes a Server-Sent Events endpoint for live KV watching. No extra tooling
required — use curl or the dashboard:

```bash
# Watch all entity state changes via SSE
curl -N http://localhost/game/events

# Or through Caddy (same-origin)
curl -N http://localhost/game/events
```

The dashboard at `http://localhost` subscribes to `/game/events` automatically via Caddy,
receiving live entity updates in the event feed sidebar.

### Common Issues

| Symptom | Cause | Fix |
|---------|-------|-----|
| "bucket not found" on first request | KV bucket created on first entity write | Post a quest or recruit an agent first |
| Agent can't claim quest | Tier too low for quest difficulty | Check `min_tier` on quest vs agent `tier` |
| 503 on store endpoints | `agent_store` component not running | Ensure it's enabled in config |
| SSE not connecting | Backend not running or Caddy/Vite proxy misconfigured | Check that `/game/events` responds |
| Quest stuck in `in_review` | `bossbattle` processor not running | Ensure it's enabled in config |

## Further Reading

- [03-QUESTS.md](03-QUESTS.md) — Quest creation, lifecycle state machine, difficulty/XP table, boss battles
- [04-PARTIES.md](04-PARTIES.md) — Party formation, roles, peer reviews, feedback loop
- [05-BOIDS.md](05-BOIDS.md) — Emergent quest-claiming behavior, six rules, tuning guide
- [02-DESIGN.md](02-DESIGN.md) — Architecture, concept map, trust tiers, death mechanics
- [06-DOMAINS.md](06-DOMAINS.md) — Domain themes (software, D&D, research), prompt catalogs
- [07-MODEL-REGISTRY.md](07-MODEL-REGISTRY.md) — LLM provider config, capabilities, fallback chains
- [Swagger UI](/docs) — Live API documentation at `/docs`
