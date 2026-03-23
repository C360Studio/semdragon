# Tools Reference

Agents use tools during quest execution to interact with the world: read files, query the knowledge graph,
search the web, run shell commands, and coordinate party work. Tools are gated by trust tier — higher tiers
unlock more powerful capabilities. Some tools also require specific skill tags.

## Tool Categories

| Category | Purpose |
|----------|---------|
| `core` | Terminal tools that end the agentic loop (`submit_work`, `ask_clarification`, `submit_findings`) |
| `knowledge` | Read-only knowledge access: game state, graph search, knowledge graph overview, explore sub-agents |
| `network` | External access: HTTP requests and web search |
| `inspect` | Shell execution via `bash` |
| `party_lead` | DAG coordination for Master-tier party leads only |

## Tool Reference

### Core Tools

#### `submit_work`

- **Category**: `core`
- **Min Tier**: Apprentice (level 1+)
- **Description**: Submit finished work. Files created or modified in the workspace are captured
  automatically — provide a summary of what was delivered. Calling this tool ends the agentic loop,
  so only call it when all work is complete.
- **Parameters**:
  - `summary` — Brief description of deliverables, design decisions, and verification steps
  - `deliverable` *(optional)* — Inline content for non-file work (analysis, research findings).
    Omit when work is in files.
- **Notes**: The system detects submissions that look like questions and returns an error directing
  the agent to use `ask_clarification` instead. Deliverables containing code fences (```` ``` ````)
  bypass the question-detection heuristic.

---

#### `ask_clarification`

- **Category**: `core`
- **Min Tier**: Apprentice (level 1+)
- **Description**: Ask the quest issuer a question when more information is needed. Prefer this over
  guessing or submitting incomplete work. Asking questions carries no XP penalty.
- **Parameters**:
  - `question` *(required)* — The question for the quest issuer
- **Notes**: This is a terminal tool — the loop pauses until the DM or party lead answers via
  `answer_clarification`. Party sub-quests route clarification requests to the lead automatically.

---

#### `submit_findings`

- **Category**: `core`
- **Min Tier**: Apprentice (level 1+)
- **Description**: Terminal tool for explore sub-agents. Submit gathered research findings and return
  them to the parent agent.
- **Parameters**:
  - `findings` *(required)* — Structured research findings
  - `sources` *(optional)* — Array of entity IDs, URLs, or file paths consulted
- **Notes**: Only available inside explore sub-agent loops. The parent agent receives the `findings`
  string as the tool result.

---

### Knowledge Tools

#### `graph_query`

- **Category**: `knowledge`
- **Min Tier**: Apprentice (level 1+)
- **Description**: Query the entity graph for agents, quests, guilds, parties, or battles. Returns a
  summary of matching entities from the game state KV store.
- **Parameters**:
  - `entity_type` *(required)* — One of: `quest`, `agent`, `guild`, `party`, `battle`
  - `limit` *(optional)* — Maximum entities to return (default 20)
- **Notes**: Use for game-state lookups (e.g. finding open quests, checking agent levels). For
  querying indexed source code or documentation, use `graph_search` instead.

---

#### `graph_search`

- **Category**: `knowledge`
- **Min Tier**: Apprentice (level 1+)
- **Description**: Search the knowledge graph via GraphQL. Supports entity lookup, relationship
  traversal, predicate queries, full-text search, and natural language queries across all indexed
  entities including source documentation and code.
- **Parameters**:
  - `query_type` *(required)* — One of:
    - `entity` — fetch a single entity by ID
    - `prefix` — list entities under an ID prefix (e.g. `c360.semsource.git`)
    - `predicate` — find entities with a specific predicate (e.g. `source.content.language`)
    - `relationships` — traverse edges from an entity
    - `search` — full-text keyword search
    - `nlq` — natural language question (e.g. "what interfaces does an OSH sensor driver implement?")
  - `entity_id` *(optional)* — For `entity` and `relationships` queries
  - `prefix` *(optional)* — For `prefix` queries
  - `predicate` *(optional)* — For `predicate` queries
  - `search_text` *(optional)* — For `search` and `nlq` queries
  - `limit` *(optional)* — Maximum results (default 20, max 100)
- **Notes**: Use `nlq` for open-ended questions about the codebase. Use `prefix` to enumerate a
  known source namespace. Call `graph_summary` first to understand available prefixes. Response size
  is capped at 500 KB.

---

#### `graph_multi_query`

- **Category**: `knowledge`
- **Min Tier**: Apprentice (level 1+)
- **Description**: Execute multiple graph queries in a single call. Reduces round-trips when several
  unrelated lookups are needed at once.
- **Parameters**:
  - `queries` *(required)* — Array of up to 5 query objects, each with the same fields as
    `graph_search` (`query_type` required per entry)
- **Notes**: Results are returned under labeled headings, one per query. Prefer this over calling
  `graph_search` repeatedly in sequence.

---

#### `graph_summary`

- **Category**: `knowledge`
- **Min Tier**: Apprentice (level 1+)
- **Description**: Get an overview of what is indexed in the knowledge graph — sources, entity types,
  counts, and example entity IDs.
- **Parameters**: None
- **Notes**: Call this once at the start of any research task to understand available data and how
  to scope queries by prefix. Results are cached; repeated calls are cheap.

---

#### `explore`

- **Category**: `knowledge`
- **Min Tier**: Apprentice (level 1+)
- **Description**: Spawn a focused sub-agent to investigate a topic using read-only tools. The
  sub-agent runs its own agentic loop and returns synthesized findings when it calls
  `submit_findings`.
- **Parameters**:
  - `goal` *(required)* — What to investigate. Be specific about what needs to be found.
  - `context` *(optional)* — Additional context (known entity IDs, file paths, constraints)
- **Notes**: See the [Explore Tool](#explore-tool) section for full usage guidance.

---

### Network Tools

#### `http_request`

- **Category**: `network`
- **Min Tier**: Journeyman (level 6+)
- **Description**: Make an HTTP request to fetch data from a URL. Use for downloading files, calling
  REST APIs, or fetching web content. The response body is returned as text. For binary downloads,
  pipe through `bash` instead.
- **Parameters**:
  - `url` *(required)* — Full URL including `https://`
  - `method` *(optional)* — `GET` (default) or `POST`
  - `body` *(optional)* — Request body for POST (use JSON format for API calls)
  - `content_type` *(optional)* — Content-Type header for POST (default `application/json`)
- **Notes**: Requests to private or loopback IPs are blocked (SSRF prevention). HTML responses are
  converted to plain text and truncated to `HTTPTextMaxChars` (default 20 000 characters). Pages
  above a minimum length are persisted to the knowledge graph automatically when graph persistence
  is enabled. Timeout: 30 seconds.

---

#### `web_search`

- **Category**: `network`
- **Min Tier**: Apprentice (level 1+) when configured; **not available** when no search provider
  is configured
- **Required Skill**: `research`
- **Description**: Search the web and return results.
- **Parameters**:
  - `query` *(required)* — The search query
  - `max_results` *(optional)* — Maximum results (default 5, max 10)
- **Notes**: `web_search` is opt-in — it is only registered when a `SearchConfig` is provided with
  a configured provider (currently supports `brave`). Agents without the `research` skill tag cannot
  use this tool even if it is registered.

---

### Inspect Tools

#### `bash`

- **Category**: `inspect`
- **Min Tier**: Journeyman (level 6+)
- **Description**: Run a shell command. Use for all file and system operations: reading (`cat`),
  writing (`cat <<'EOF' > file`), searching (`grep -rn`), listing (`ls -la`), running tests,
  building projects, git operations, and managing dependencies. Supports heredocs and pipes.
- **Parameters**:
  - `command` *(required)* — The shell command to execute
- **Notes**: When `SandboxURL` is configured, execution is proxied to the sandbox container and
  runs inside the agent's isolated `/workspace/{quest-id}/` directory with `cap_drop: ALL` and a
  read-only root filesystem. Without a sandbox URL, the command runs in the local process
  environment. Output is truncated to 100 KB. Default timeout: 60 seconds.

---

### Party Lead Tools

These tools are only available to agents at Master tier (level 16+) who are serving as party leads.
They are used exclusively during party quest execution.

#### `decompose_quest`

- **Category**: `party_lead`
- **Min Tier**: Master (level 16+)
- **Description**: Decompose a complex party quest into a DAG of sub-quests. The lead proposes the
  decomposition structure; the tool validates it and returns the validated DAG. questbridge then
  posts the sub-quests and begins DAG execution.
- **Parameters**:
  - `goal` *(required)* — High-level rationale for the decomposition
  - `nodes` *(required)* — Array of sub-quest node objects, each with:
    - `id` *(required)* — Unique node identifier within this DAG
    - `objective` *(required)* — Detailed spec for the assigned agent, including specific file
      paths for code tasks
    - `skills` *(optional)* — Required skill tags (e.g. `code_generation`)
    - `difficulty` *(optional)* — Difficulty 0–5 (0=trivial, 5=legendary)
    - `acceptance` *(optional)* — Acceptance criteria the lead will evaluate during review
    - `depends_on` *(optional)* — IDs of nodes that must complete before this one
- **Notes**: This is a terminal tool — calling it ends the lead's decomposition loop. The DAG JSON
  becomes the loop's final output and is parsed by questbridge. Validation errors are returned to
  the LLM for correction. All node IDs must be unique; `depends_on` references must exist within
  the same DAG.

---

#### `review_sub_quest`

- **Category**: `party_lead`
- **Min Tier**: Master (level 16+)
- **Description**: Review a party member's sub-quest output using a three-question rating rubric.
  `accept` advances the DAG node; `reject` resets it for retry and injects feedback into the
  member's next prompt.
- **Parameters**:
  - `sub_quest_id` *(required)* — The ID of the sub-quest being reviewed
  - `ratings` *(required)* — Object with three 1–5 scores:
    - `q1` — Task quality: Did the output meet acceptance criteria?
      (1=wrong/missing, 3=meets requirements, 5=exceptional/rare)
    - `q2` — Communication: Were assumptions stated and output clearly organized?
      (1=incoherent, 3=adequate, 5=exemplary)
    - `q3` — Completeness: Did the agent deliver everything needed without gaps?
      (1=mostly missing, 3=complete, 5=comprehensive beyond requirements)
  - `verdict` *(required)* — `accept` or `reject`
  - `explanation` *(optional)* — Corrective feedback. **Required** when average rating < 3.0 and
    verdict is `reject`. Also required when all three ratings are 5 (anti-inflation guard).
- **Notes**: Scores become part of the agent's permanent record and influence future prompt context.
  3 represents standard competent work; 5 is rare. The all-5 explanation requirement guards against
  LLM score inflation.

---

#### `answer_clarification`

- **Category**: `party_lead`
- **Min Tier**: Master (level 16+)
- **Description**: Answer a party member's clarification question about their sub-quest. The member's
  sub-quest was paused at `NodeAwaitingClarification` status; this answer re-dispatches it.
- **Parameters**:
  - `sub_quest_id` *(required)* — The ID of the sub-quest the member asked about
  - `answer` *(required)* — A specific and actionable answer to the member's question

---

## Trust Tier Summary

| Tier | Level Range | Tools Available |
|------|-------------|----------------|
| Apprentice | 1–5 | Core + Knowledge |
| Journeyman | 6–10 | + `bash`, `http_request`, `web_search` (if configured) |
| Expert | 11–15 | Same as Journeyman; eligible for production-critical quests |
| Master | 16–18 | + Party Lead tools (`decompose_quest`, `review_sub_quest`, `answer_clarification`) |
| Grandmaster | 19–20 | All Master tools + DM delegation capabilities |

## Explore Tool

The `explore` tool spawns a focused sub-agent to handle complex multi-step discovery. It is
intercepted by `questtools` before reaching the standard tool registry — the placeholder handler
in the registry is never called directly.

### When to Use Explore

Use `explore` when a research task requires three or more sequential lookups or needs to synthesize
information from multiple sources. For a single lookup, call `graph_search` or `graph_query`
directly — the overhead of a child loop is not justified.

Examples of good explore usage:

- "Find all OSH sensor drivers and summarize which interfaces each implements"
- "Research the rate limiting behavior of the upstream API and document its retry semantics"
- "Map the dependency chain for the auth module and identify circular dependencies"

### How It Works

1. The parent agent calls `explore` with a `goal` and optional `context`.
2. `questtools` intercepts the call and starts a new agentic loop using the `explore` capability
   key (mapped to a cheaper model in the model registry).
3. The sub-agent receives a read-only tool set and a system prompt scoped to the goal.
4. When the sub-agent calls `submit_findings`, the loop ends and the findings text is returned
   to the parent agent as the `explore` tool result.

### Sub-Agent Tool Set

Explore sub-agents receive a read-only subset of the registered tools:

- `graph_query`, `graph_search`, `graph_multi_query`, `graph_summary`
- `web_search` (if configured and agent has `research` skill)
- `http_request` (Journeyman+ only)
- `submit_findings`

Excluded tools: `bash`, `submit_work`, `ask_clarification`, `explore` (no recursive spawning),
and all party lead tools.

### Limits

| Setting | Default | Config Key |
|---------|---------|------------|
| Max iterations | 8 | `explore_max_iterations` |
| Timeout | 120 s | `explore_timeout` |
| Model capability | `explore` | `explore_capability` |
| Recursive explore | Not allowed | — |

Only one explore sub-agent per parent loop is permitted at a time. The model registry maps the
`explore` capability key to a cheaper or faster model than the main agent uses.

## Tool Configuration

### `questtools` Component Config

The `questtools` processor wires all tool registrations for production use:

| Config Key | Purpose | Default |
|------------|---------|---------|
| `enable_builtins` | Register `bash`, `http_request`, core, DAG tools | `true` |
| `graphql_url` | Graph-gateway GraphQL endpoint for `graph_search` | *(empty — disables tool)* |
| `sandbox_url` | Sandbox container URL for proxied `bash` execution | *(empty — runs locally)* |
| `search.provider` | Web search provider (`brave`) for `web_search` | *(empty — disables tool)* |
| `http_text_max_chars` | Max characters after HTML-to-text conversion | `20000` |
| `http_persist_to_graph` | Persist fetched HTML pages to knowledge graph | `true` |
| `explore_max_iterations` | Max tool calls per explore sub-agent | `8` |
| `explore_timeout` | Max wall time per explore sub-agent | `120s` |
| `explore_capability` | Model registry capability key for explore agents | `explore` |

### Skill-Gated Tools

`web_search` requires the `research` skill tag in addition to the minimum tier. An agent without
this skill will receive an error even if the tool is registered and their tier is sufficient.

### Quest-Level Restrictions

Individual quests can further restrict tool access via the `AllowedTools` field. When set, only
the named tools are offered to the agent regardless of tier or skill. This is used for quests that
should be scoped to specific capabilities (e.g. a documentation quest that should not use `bash`).
