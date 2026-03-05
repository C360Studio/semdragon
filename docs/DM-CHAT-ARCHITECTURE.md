# DM Chat Routing and Orchestration Architecture

## Status

**Proposed** -- March 2026

## Problem Statement

The DM chat endpoint (`POST /game/dm/chat`) is currently a single unstructured LLM call
with no routing. The system prompt always instructs the LLM to produce quest briefs, even
when the user wants to ask a question about world state, manage agents, or plan a complex
feature. There is no intent classification, no mode system, and no pipeline for turning
agent quest output back into new quests.

This creates three problems:

1. **Unpredictable behavior**: The LLM may produce a quest brief when the user was just
   asking a question, or fail to produce one when the user wanted to create work.
2. **No planning pipeline**: There is no way for the DM to decompose complex work into
   quest chains using agents, which is core to the trust tier system (Master+ agents
   can decompose quests, but there is no path to trigger this).
3. **No integration with existing processors**: The `dmsession`, `dmapproval`, and
   `quest-design` capability in the model registry are disconnected from the chat flow.

## Design Principles

1. **Explicit modes for predictability, natural language for discovery.** Humans should
   always know what the DM will do with their input. Modes are declared, not inferred.
2. **The DM is a collaborator, not a router.** Each mode is a focused conversation, not
   a one-shot dispatch. The DM gathers context, proposes, and waits for confirmation.
3. **Trust the existing systems.** Quest posting, boss battles, agent progression, and
   the approval router already work. The chat layer orchestrates them; it does not
   replace them.
4. **Incremental delivery.** Each phase produces a working system. No phase depends on
   a future phase being completed.

---

## Mode System Design

### Modes

The DM chat operates in one of four modes. Each mode determines the system prompt, the
set of structured outputs the LLM may produce, and what buttons the frontend shows.

| Mode | Purpose | Structured Outputs | LLM Capability Key |
|------|---------|-------------------|--------------------|
| **converse** | Ask questions, get world state summaries, general help | None (plain text only) | `dm-chat` |
| **quest** | Create a single quest or quest chain through conversation | `quest_brief`, `quest_chain` | `quest-design` |
| **plan** | Decompose a complex objective into a phased quest plan | `quest_plan` (new type) | `quest-design` |
| **manage** | Agent operations: recruit, retire, inspect, intervene | `agent_action` (new type) | `dm-chat` |

**Default mode**: `converse`. The DM never produces structured output in converse mode,
making it safe for exploratory questions. This is the key predictability guarantee.

### Why These Four and Not Fewer

A single "smart" mode that infers intent was considered and rejected. The failure mode
is too costly: the LLM produces a quest brief when the user wanted information, or
vice versa. With explicit modes, the user controls what happens. The frontend makes
switching trivial (a single click), so the friction cost is near zero.

A fifth "review" mode for managing boss battles and peer reviews was considered but
deferred. Reviews are already automated by the bossbattle processor. Human review
submissions use the existing `POST /reviews/{id}/submit` endpoint. A dedicated mode
adds complexity without clear value until the human review workflow is exercised more.

### Why Not LLM-Based Intent Classification

An intent classifier that auto-selects the mode was considered as a hybrid approach.
The problem is latency and reliability:

- Classification adds a round-trip before the actual LLM call (200-500ms with Ollama,
  2-3s with cloud providers).
- Classification errors are silent and confusing: the user types "tell me about quest
  abc" and gets a quest creation flow because the classifier saw "quest."
- The modes are few and stable. A UI toggle is faster and more reliable than a
  classifier.

Intent classification may be useful later as a *suggestion* ("It looks like you want
to create a quest -- switch to quest mode?"), but it should not be a gate.

---

## Mode Details

### Converse Mode

**Purpose**: Answer questions about the game world, explain concepts, summarize entity
state.

**System prompt**: DM persona + world state summary + injected context entities. No
output format instructions. No quest schema. The LLM is explicitly told: "Do NOT produce
JSON blocks or structured output. Answer the user's question in natural language."

**World context injection**: Full world stats, plus detail for any context entities
(agents, quests, battles, guilds) pinned by the user or derived from the current page.

**Frontend**: Plain chat. No action buttons on DM messages. The input placeholder
reads "Ask the DM anything..."

**Why this matters**: Converse mode is the safe default. New users, confused users,
and users exploring the system all land here. Nothing happens that changes game state.

### Quest Mode

**Purpose**: Create a single quest or a quest chain through conversational refinement.

**System prompt**: DM persona + quest schema instructions (existing `quest_brief` and
`quest_chain` JSON block format) + available skills/difficulties/review levels + world
state summary + injected context.

This is essentially the current `handleDMChat` behavior, isolated to its own mode.

**Structured outputs**: `quest_brief` and `quest_chain`, extracted via the existing
tagged JSON block parser (`extractTaggedJSON`).

**Confirmation flow**:
1. User describes what they want.
2. DM asks clarifying questions (difficulty, skills, acceptance criteria).
3. DM produces a quest brief or chain in a tagged JSON block.
4. Frontend renders the quest preview card with a "Post Quest" / "Post Chain" button.
5. User clicks to post, or continues refining.

**Frontend**: Same as today, but the input placeholder reads "Describe the work you
want done..." and the mode indicator shows "Quest" with a scroll icon.

### Plan Mode

**Purpose**: Decompose a complex objective into a phased execution plan, then promote
that plan to real quests.

This is where the Mode A / Mode B / Hybrid decision lives. The design uses a **hybrid
approach**: the DM (LLM) produces a high-level plan with phases, and optionally a
Master-tier agent refines each phase into detailed quests.

**System prompt**: DM persona + plan schema instructions + available skills/agents/guilds
+ world state summary + injected context. The prompt emphasizes:
- Break the objective into phases (1-5 phases).
- Each phase has a title, description, estimated difficulty, required skills, and
  dependencies on prior phases.
- Phases should be coarse-grained enough for a human to review but specific enough
  for an agent to decompose further.

**Structured output**: A new `quest_plan` type:

```json
{
  "objective": "Migrate user service from REST to gRPC",
  "phases": [
    {
      "title": "Define proto schema",
      "description": "Create .proto files for user service endpoints",
      "difficulty": 2,
      "skills": ["code_generation", "analysis"],
      "acceptance": ["Proto compiles", "All existing endpoints covered"],
      "depends_on": []
    },
    {
      "title": "Generate server stubs",
      "description": "Run protoc and implement gRPC server handlers",
      "difficulty": 3,
      "skills": ["code_generation"],
      "acceptance": ["All handlers compile", "Unit tests pass"],
      "depends_on": [0]
    }
  ],
  "decompose_with_agent": true
}
```

**Promotion pipeline** (see detailed section below):
1. User reviews the plan in the chat UI.
2. User clicks "Execute Plan" to promote phases to real quests.
3. If `decompose_with_agent` is true and a Master+ agent is available, the system
   creates a parent "planning quest" for each phase. The assigned agent decomposes it
   into sub-quests. The agent's quest output is a `quest_chain` JSON block.
4. If `decompose_with_agent` is false, each phase is posted directly as a quest.

**Frontend**: Plan preview card showing phases with dependency arrows. "Execute Plan"
button. Optional toggle for "Let an agent decompose each phase" (shown only if Master+
agents exist).

### Manage Mode

**Purpose**: Agent lifecycle operations through natural language.

**System prompt**: DM persona + agent roster summary (names, levels, tiers, status,
current quests, guilds) + available operations list.

**Structured output**: `agent_action` type:

```json
{
  "action": "recruit",
  "params": {
    "name": "DataDragon",
    "skills": ["data_transformation", "analysis"],
    "is_npc": true
  }
}
```

Supported actions: `recruit`, `retire`, `inspect`, `intervene`.

**Confirmation flow**: All actions require explicit user confirmation via a button in
the chat. The DM proposes the action, the frontend shows a confirmation card, and the
user clicks to execute. This prevents the LLM from accidentally retiring an agent.

**Integration with dmapproval**: In `DMAssisted` and `DMSupervised` modes, manage
actions that exceed the DM mode threshold (e.g., retiring an agent in supervised mode)
are routed through the approval router before execution.

**Frontend**: Agent action card with confirmation button. Action-specific details
(e.g., for recruit: name, skills, NPC toggle; for intervene: quest ID, intervention
type, reason).

---

## Quest Output to New Quests Pipeline

This is the mechanism by which an agent's quest output gets promoted to real quests.
It serves two scenarios:

1. **Plan mode decomposition**: A planning quest's output is a quest chain that
   needs to become real quests.
2. **Agent-initiated decomposition**: A Master+ agent working on a quest decides it
   needs to be broken down and produces sub-quests in its output.

### Pipeline Design

```
Quest Output (string)
    |
    v
[Output Parser] -- Extracts quest_chain JSON from output
    |
    v
[Validation] -- ValidateQuestChainBrief
    |
    v
[Promotion Gate] -- Based on DMMode:
    |               full_auto  -> auto-promote
    |               assisted   -> DM approval (approval router)
    |               supervised -> human approval (approval router)
    |               manual     -> human-only
    v
[Quest Posting] -- Uses existing PostQuestChain handler
    |
    v
[Parent Linking] -- Sets parent_quest on sub-quests
```

### Implementation: New Processor -- `questpromotion`

A new processor watches for completed quests and checks if their output contains
promotable quest chains. This keeps the pipeline reactive and decoupled from
questbridge.

**Watch pattern**: Quest entities transitioning to `completed` status.

**Trigger conditions**:
1. Quest output is non-empty.
2. Output contains a valid `quest_chain` or `quest_brief` JSON block.
3. Quest has a flag or predicate indicating its output should be promoted
   (`quest.promotion.enabled` triple, set by plan mode when creating planning quests).

**Gate logic**:
- Read the board's DMMode from config or session.
- `full_auto`: Post the quest chain immediately.
- `assisted`: Create an approval request via the approval router. If approved, post.
- `supervised` / `manual`: Create an approval request. Block until human responds.

**Parent linking**: Sub-quests get `parent_quest` set to the planning quest ID, and
`depends_on` indices are resolved to real quest IDs using the same two-pass algorithm
in the existing `PostQuestChain` handler.

### Why a Separate Processor

The promotion logic could live in questbridge (which already handles quest completion)
or in the API layer. A separate processor is better because:

1. **Single responsibility**: questbridge handles quest-to-LLM bridging. Promotion
   handles output-to-quest conversion. Different concerns.
2. **Testability**: The promotion logic can be tested in isolation with mock KV
   entries, without needing the full agentic loop stack.
3. **Opt-in**: The processor can be disabled in configs where promotion is not wanted.

---

## System Prompt Strategy

Each mode has a distinct system prompt built from composable fragments. The prompt
builder (`buildDMSystemPrompt`) gains a mode parameter that controls which fragments
are included.

### Prompt Fragment Architecture

```
[DM Persona]        -- constant across all modes
[Mode Instructions]  -- mode-specific behavior and output format
[World Context]      -- world stats, always included
[Entity Context]     -- pinned/page context entities, always included
[Domain Context]     -- domain-specific vocabulary and skills (from DomainCatalog)
[Constraints]        -- mode-specific constraints (e.g., "no JSON in converse")
```

### Fragment Details

**DM Persona** (shared):
```
You are the Dungeon Master of Semdragons, an agentic workflow coordination system.
You guide users in managing quests, agents, and the game world.
You are helpful, concise, and use the domain vocabulary defined below.
```

**Mode Instructions** (per-mode):

- **converse**: "Answer the user's question using the world state and context below.
  Do NOT produce JSON blocks, quest briefs, or any structured output. Respond in
  natural language only."

- **quest**: Current system prompt content from `dm_prompt.go` (quest schema
  instructions, difficulty/skill/review reference tables).

- **plan**: "Help the user decompose a complex objective into execution phases. Each
  phase should be a meaningful unit of work that can be assigned to an agent or party.
  When you have enough information, produce a plan using the format below. [plan
  schema]. Ask clarifying questions about scope, priorities, and dependencies before
  producing the plan."

- **manage**: "Help the user manage agents. You can recruit new agents, retire
  existing ones, inspect agent details, or intervene in active quests. Always propose
  the action and wait for user confirmation before executing. Available agents: [agent
  roster summary]. Available actions: recruit, retire, inspect, intervene."

**World Context**: Always included. The existing `buildDMSystemPrompt` world state
section.

**Entity Context**: Always included. The existing `resolveContextDetail` logic for
pinned and page context items.

**Domain Context**: New. When a domain is configured (e.g., software), include the
domain vocabulary mapping so the DM uses "task" instead of "quest", "developer"
instead of "agent", etc. This is read from the active `DomainCatalog`.

### Model Capability Routing

Add `dm-chat` capability to the model registry in `config/semdragons.json`:

```json
{
  "dm-chat": {
    "description": "DM conversational assistance and agent management",
    "preferred": ["ollama-qwen3"],
    "fallback": ["ollama-coder"],
    "requires_tools": false
  }
}
```

The `quest-design` capability already exists and is used for quest and plan modes.
The routing is:

| Mode | Capability Key | Rationale |
|------|---------------|-----------|
| converse | `dm-chat` | General conversation, no structured output needed |
| quest | `quest-design` | Quest parameter decisions, structured output |
| plan | `quest-design` | Same structured reasoning as quest creation |
| manage | `dm-chat` | Simple action extraction, no complex reasoning |

---

## Frontend UX Changes

### Mode Selector

A horizontal pill/tab strip below the chat header, above the message area. Four pills:
`Chat`, `Quest`, `Plan`, `Manage`. Active pill is highlighted. Clicking a pill
switches the mode and updates the input placeholder text.

```
[ Chat ] [ Quest ] [ Plan ] [ Manage ]
```

Compact styling: pills are 24px tall, 0.75rem text, no icons. The mode strip is
always visible when the chat panel is open, taking minimal vertical space.

### Mode-Specific Input Placeholders

| Mode | Placeholder Text |
|------|-----------------|
| converse | "Ask the DM anything..." |
| quest | "Describe the work you want done..." |
| plan | "What do you want to build or accomplish?" |
| manage | "What do you need to do with agents?" |

### Action Cards

DM messages in quest, plan, and manage modes can include action cards -- structured
previews with confirmation buttons.

**Quest mode**: Existing quest preview card and chain preview card. No changes needed.

**Plan mode**: New plan preview card showing:
- Objective title
- Numbered phases with dependency indicators
- Per-phase difficulty badge and skill tags
- "Execute Plan" button
- "Decompose with Agent" toggle (visible only when Master+ agents exist)

**Manage mode**: New agent action card showing:
- Action type badge (Recruit / Retire / Inspect / Intervene)
- Action parameters in a compact key-value layout
- "Confirm" button (or "Cancel" link)

### Session Continuity

Mode is persisted in the chat store alongside messages and session ID. When restoring
from localStorage, the mode is restored too. The mode is sent to the backend in the
request body so the correct system prompt is built.

### Keyboard Shortcut

`Cmd+K` (or `Ctrl+K` on non-Mac) opens the chat panel and focuses the input. If the
panel is already open, it cycles through modes: converse -> quest -> plan -> manage ->
converse. This gives power users fast mode switching without leaving the keyboard.

---

## Integration with Existing Processors

### dmsession

The DM session processor manages session lifecycle (start, end, summary). Currently it
is registered but not in the default config. With the chat routing layer:

- Chat sessions create a DM session on first message (lazy, same as today's trace
  context creation).
- The session's `DMMode` field determines the promotion gate behavior.
- Session summary includes mode usage stats (messages per mode, quests created, plans
  executed).

### dmapproval

The approval router handles human-in-the-loop confirmation. With the chat routing
layer:

- Plan mode promotion uses the approval router when DMMode is `assisted` or
  `supervised`.
- Manage mode agent actions use the approval router for destructive operations
  (retire, intervene) in supervised mode.
- Pending approvals show as a notification in the chat panel header.

### dmworldstate

The world state aggregator already feeds the DM system prompt. No changes needed,
but consider caching the world state for the duration of a chat turn to avoid
multiple KV reads when building the prompt and resolving context entities.

### boardcontrol

Manage mode should show board pause/resume status and allow toggling. This is a
natural extension -- the DM already has world state context.

---

## API Changes

### Request Body Extension

`POST /game/dm/chat` gains a `mode` field:

```json
{
  "message": "I want to migrate our API to gRPC",
  "mode": "plan",
  "context": [{"type": "agent", "id": "dragon"}],
  "history": [...],
  "session_id": "abc123"
}
```

If `mode` is omitted or empty, the server defaults to `"converse"`. This preserves
backward compatibility: existing clients that do not send a mode get the safe
converse behavior (which is more correct than the current behavior of always trying
to extract quest briefs).

### Response Body Extension

The response gains a `mode` field and new optional structured output fields:

```json
{
  "message": "Here is a plan for the gRPC migration...",
  "mode": "plan",
  "quest_brief": null,
  "quest_chain": null,
  "quest_plan": { ... },
  "agent_action": null,
  "session_id": "abc123",
  "trace_info": { ... }
}
```

### New Types in `domain/dm.go`

```go
// ChatMode determines the DM chat routing behavior.
type ChatMode string

const (
    ChatModeConverse ChatMode = "converse"
    ChatModeQuest    ChatMode = "quest"
    ChatModePlan     ChatMode = "plan"
    ChatModeManage   ChatMode = "manage"
)

// QuestPlan represents a phased execution plan produced by plan mode.
type QuestPlan struct {
    Objective          string           `json:"objective"`
    Phases             []PlanPhase      `json:"phases"`
    DecomposeWithAgent bool             `json:"decompose_with_agent"`
}

// PlanPhase is one phase in a quest plan.
type PlanPhase struct {
    Title       string           `json:"title"`
    Description string           `json:"description,omitempty"`
    Difficulty  *QuestDifficulty `json:"difficulty,omitempty"`
    Skills      []SkillTag       `json:"skills,omitempty"`
    Acceptance  []string         `json:"acceptance,omitempty"`
    DependsOn   []int            `json:"depends_on,omitempty"`
}

// AgentAction represents a DM-proposed agent management action.
type AgentAction struct {
    Action string         `json:"action"` // recruit, retire, inspect, intervene
    Params map[string]any `json:"params"`
}
```

### Validation

```go
func ValidateQuestPlan(p *QuestPlan) error // objective required, 1-20 phases, valid deps
func ValidateChatMode(m ChatMode) bool     // one of the four constants
```

---

## Implementation Phases

### Phase 1: Mode Routing (Backend + Frontend)

**Goal**: Chat modes work end-to-end. Converse mode is the safe default. Quest mode
preserves existing behavior.

**Backend changes**:
1. Add `ChatMode` type and constants to `domain/dm.go`.
2. Add `mode` field to `DMChatRequest` in `service/api/request_types.go`.
3. Refactor `buildDMSystemPrompt` to accept a mode parameter. Extract mode-specific
   prompt fragments into `service/api/dm_prompt.go`.
4. Update `handleDMChat` to read mode from request, default to `converse`, route to
   the correct capability key, and only attempt quest extraction in `quest` mode.
5. Add `dm-chat` capability to `config/semdragons.json` model registry.

**Frontend changes**:
1. Add `mode` field to `chatStore` reactive state.
2. Add mode selector pill strip to `ChatPanel.svelte`.
3. Update `sendDMChat` API call to include mode.
4. Conditionally render quest preview cards only in quest mode.
5. Update input placeholders per mode.
6. Persist mode to localStorage with the rest of the chat state.

**Files changed**:
- `/Users/coby/Code/c360/semdragon/domain/dm.go` -- add ChatMode type
- `/Users/coby/Code/c360/semdragon/service/api/request_types.go` -- add mode to request/response
- `/Users/coby/Code/c360/semdragon/service/api/dm_prompt.go` -- mode-aware prompt builder
- `/Users/coby/Code/c360/semdragon/service/api/handlers.go` -- update handleDMChat
- `/Users/coby/Code/c360/semdragon/service/api/openapi.go` -- update API spec
- `/Users/coby/Code/c360/semdragon/config/semdragons.json` -- add dm-chat capability
- `/Users/coby/Code/c360/semdragon/ui/src/lib/stores/chatStore.svelte.ts` -- add mode state
- `/Users/coby/Code/c360/semdragon/ui/src/lib/components/chat/ChatPanel.svelte` -- mode selector
- `/Users/coby/Code/c360/semdragon/ui/src/lib/components/chat/ChatMessage.svelte` -- conditional rendering
- `/Users/coby/Code/c360/semdragon/ui/src/lib/services/api.ts` -- add mode param

**Tests**:
- Unit test: mode-specific prompt building (dm_prompt_test.go)
- Unit test: mode routing in handleDMChat (handlers_test.go)
- E2E test: mode selector interaction, mode persistence across refresh

**Estimated effort**: 2-3 days.

### Phase 2: Plan Mode

**Goal**: Users can create execution plans through conversation. Plans can be promoted
to quest chains with a single click.

**Backend changes**:
1. Add `QuestPlan`, `PlanPhase`, `AgentAction` types to `domain/dm.go`.
2. Add plan mode system prompt fragment to `dm_prompt.go`.
3. Add plan extraction logic to `handleDMChat` (same tagged JSON pattern:
   `` ```json:quest_plan ``).
4. Add `POST /game/dm/plan/execute` endpoint that takes a `QuestPlan` and creates
   quests from its phases (reusing `PostQuestChain` logic).
5. Add `ValidateQuestPlan` to `domain/dm.go`.

**Frontend changes**:
1. Add `QuestPlan` TypeScript type.
2. Add `PlanPreview.svelte` component (phase list with dependency indicators).
3. Add "Execute Plan" button in plan preview card.
4. Wire `postPlan` action in chatStore.

**Files changed**:
- `/Users/coby/Code/c360/semdragon/domain/dm.go` -- QuestPlan, PlanPhase types
- `/Users/coby/Code/c360/semdragon/service/api/dm_prompt.go` -- plan mode prompt
- `/Users/coby/Code/c360/semdragon/service/api/handlers.go` -- plan extraction, execute endpoint
- `/Users/coby/Code/c360/semdragon/service/api/request_types.go` -- plan types
- `/Users/coby/Code/c360/semdragon/service/api/openapi.go` -- plan endpoint spec
- `/Users/coby/Code/c360/semdragon/ui/src/lib/stores/chatStore.svelte.ts` -- plan actions
- `/Users/coby/Code/c360/semdragon/ui/src/lib/components/chat/PlanPreview.svelte` -- new
- `/Users/coby/Code/c360/semdragon/ui/src/lib/components/chat/ChatMessage.svelte` -- plan card
- `/Users/coby/Code/c360/semdragon/ui/src/lib/services/api.ts` -- executePlan call

**Tests**:
- Unit test: QuestPlan validation
- Unit test: plan extraction from LLM response
- Unit test: plan-to-quest-chain conversion
- E2E test: plan mode conversation, execute plan flow

**Estimated effort**: 3-4 days.

### Phase 3: Manage Mode

**Goal**: Agent operations through natural language with confirmation gates.

**Backend changes**:
1. Add manage mode system prompt fragment (agent roster, available operations).
2. Add `agent_action` extraction logic to `handleDMChat`.
3. Add `POST /game/dm/action/execute` endpoint that dispatches agent actions to
   existing handlers (recruit -> handleRecruitAgent, retire -> handleRetireAgent,
   intervene -> handleDMIntervene).
4. Validate action params before dispatch.

**Frontend changes**:
1. Add `AgentAction` TypeScript type.
2. Add `AgentActionCard.svelte` component (action type, params, confirm button).
3. Wire `executeAction` in chatStore.
4. Show agent roster summary in manage mode header.

**Files changed**:
- `/Users/coby/Code/c360/semdragon/service/api/dm_prompt.go` -- manage mode prompt
- `/Users/coby/Code/c360/semdragon/service/api/handlers.go` -- action extraction, execute endpoint
- `/Users/coby/Code/c360/semdragon/service/api/request_types.go` -- action types
- `/Users/coby/Code/c360/semdragon/ui/src/lib/components/chat/AgentActionCard.svelte` -- new
- `/Users/coby/Code/c360/semdragon/ui/src/lib/components/chat/ChatMessage.svelte` -- action card
- `/Users/coby/Code/c360/semdragon/ui/src/lib/services/api.ts` -- executeAction call

**Tests**:
- Unit test: action extraction, param validation
- E2E test: recruit flow through manage mode

**Estimated effort**: 2-3 days.

### Phase 4: Quest Promotion Processor

**Goal**: Agent quest output containing quest chains gets promoted to real quests,
gated by DMMode.

**Backend changes**:
1. Create `processor/questpromotion/` with standard processor structure.
2. Watch quest entities for `completed` status transitions.
3. Check for `quest.promotion.enabled` triple (set by plan mode execute handler).
4. Extract quest chain from quest output.
5. Route through approval gate based on DMMode.
6. Post promoted quests via GraphClient with parent linking.
7. Register in `componentregistry/register.go`.

**Files changed**:
- `/Users/coby/Code/c360/semdragon/processor/questpromotion/component.go` -- new
- `/Users/coby/Code/c360/semdragon/processor/questpromotion/config.go` -- new
- `/Users/coby/Code/c360/semdragon/processor/questpromotion/handler.go` -- new
- `/Users/coby/Code/c360/semdragon/processor/questpromotion/register.go` -- new
- `/Users/coby/Code/c360/semdragon/componentregistry/register.go` -- register
- `/Users/coby/Code/c360/semdragon/config/semdragons.json` -- optional config

**Tests**:
- Integration test: promotion pipeline with mock KV
- Unit test: output parsing, gate logic

**Estimated effort**: 3-4 days.

### Phase 5: Agent-Assisted Decomposition

**Goal**: Plan mode can delegate detailed decomposition to Master+ agents.

**Backend changes**:
1. When `decompose_with_agent` is true in plan execution, create planning quests
   with skill `planning`, difficulty based on phase difficulty, and the
   `quest.promotion.enabled` triple.
2. The planning quest's input is the phase description and acceptance criteria.
3. The planning quest's output (produced by the agent) is a quest chain.
4. The questpromotion processor (Phase 4) automatically promotes the output.

**Frontend changes**:
1. Add "Decompose with Agent" toggle in plan preview card.
2. Show decomposition status in plan tracking (which phases have been decomposed,
   which are pending).

**This phase requires no new processors.** It uses questpromotion (Phase 4) and the
existing questbridge + agentic-loop stack. The only new code is in the plan execution
handler (creating planning quests with the right metadata).

**Estimated effort**: 2-3 days.

---

## Data Flow Diagrams

### Phase 1-2: Plan Mode End-to-End

```
User                     Frontend              Backend (API)         NATS KV
  |                         |                       |                   |
  |-- "Migrate to gRPC" -->|                       |                   |
  |   (mode: plan)         |-- POST /dm/chat ----->|                   |
  |                         |   {mode: "plan"}     |-- buildPrompt --->|
  |                         |                       |   (plan mode)     |
  |                         |                       |-- callLLM ------->|
  |                         |                       |<- quest_plan -----|
  |                         |<- {quest_plan} ------|                   |
  |<-- plan preview --------|                       |                   |
  |                         |                       |                   |
  |-- "Execute Plan" ----->|                       |                   |
  |                         |-- POST /dm/plan/ --->|                   |
  |                         |   execute            |-- PostQuestChain->|
  |                         |                       |   (phases as      |
  |                         |                       |    quest chain)   |
  |                         |<- {quests: [...]} ---|                   |
  |<-- "3 quests created" --|                       |                   |
```

### Phase 4-5: Agent Decomposition Pipeline

```
Plan Execution              Questboard           Questbridge        Agentic Loop
      |                         |                     |                  |
      |-- post planning quest ->|                     |                  |
      |   (promotion.enabled)   |-- claim + start --->|                  |
      |                         |                     |-- TaskMessage -->|
      |                         |                     |                  |-- LLM calls
      |                         |                     |                  |   (planning)
      |                         |                     |<- complete ------|
      |                         |<- in_review --------|                  |
      |                         |   (output = chain)  |                  |
      |                         |                     |                  |
      |                         |                     |                  |
Questpromotion                  |                     |                  |
      |<-- watch: completed ----|                     |                  |
      |   (promotion.enabled)   |                     |                  |
      |-- extract chain ------->|                     |                  |
      |-- approval gate ------->|                     |                  |
      |-- PostQuestChain ------>|                     |                  |
      |   (sub-quests posted)   |                     |                  |
```

---

## Key Architectural Decisions

### ADR-1: Explicit Modes Over Intent Classification

**Context**: The user prefers predictability. The system needs to handle conversation,
quest creation, planning, and agent management.

**Decision**: Use explicit user-selected modes with a UI toggle, not LLM-based intent
classification.

**Rationale**: Modes eliminate the classification failure mode (wrong intent detected).
The UI cost is a single click. Classification can be added later as a suggestion
mechanism without being a gate.

**Consequences**: Users must manually switch modes. This is acceptable because modes
are few and the UI makes switching trivial.

### ADR-2: Hybrid Planning (DM High-Level, Agents Detail)

**Context**: Quest decomposition can happen at the DM level or the agent level. The
trust tier system reserves decomposition for Master+ agents.

**Decision**: The DM produces coarse-grained plans (phases). Master+ agents optionally
decompose phases into detailed quests.

**Rationale**: This respects the trust tier system (only qualified agents decompose),
keeps the DM's output reviewable by humans (3-5 phases, not 20 sub-quests), and
leverages the existing agentic loop for detailed work.

**Consequences**: Full decomposition requires Phase 5 and Master+ agents. Phase 2
alone provides direct plan-to-quest conversion for simpler cases.

### ADR-3: Quest Promotion as a Separate Processor

**Context**: Agent quest output containing quest chains needs to become real quests.
This could happen in questbridge, the API layer, or a new processor.

**Decision**: Create `processor/questpromotion` as a dedicated reactive processor.

**Rationale**: Single responsibility, independent testability, opt-in via config.
Questbridge's concern is quest-to-LLM bridging; promotion is a different pipeline.

**Consequences**: One more processor to register and configure. The operational
overhead is minimal since it follows the standard processor pattern.

### ADR-4: Converse Mode as Default

**Context**: The existing behavior tries to extract quest briefs from every response,
which is surprising and error-prone.

**Decision**: Default to converse mode, which never produces structured output.

**Rationale**: The safe default should be the one that never changes game state. Users
who want to create quests explicitly switch to quest mode. This is more predictable
than the current behavior.

**Consequences**: Existing frontend clients that omit the mode field will get converse
behavior instead of quest-extraction behavior. This is a breaking change in behavior
but not in API shape, and it is strictly safer.

---

## Open Questions

1. **Streaming responses**: The current LLM call is non-streaming. Plan mode responses
   can be long. Should we add SSE streaming for DM chat responses? This is orthogonal
   to the mode system and can be added later without architectural changes.

2. **Mode memory per session**: Should the mode persist per session (switch to quest
   mode, come back tomorrow, still in quest mode) or reset to converse on session
   restore? Recommendation: persist per session, since the user's intent is usually
   stable within a session.

3. **Multi-domain prompt assembly**: The current prompt builder is domain-agnostic.
   When a domain is configured (e.g., software), should the DM use domain vocabulary
   in its responses? Recommendation: yes, inject vocabulary from the active
   DomainCatalog. This is a small addition to the prompt builder.

4. **Plan revision**: After producing a plan, should the user be able to say "remove
   phase 3" and get an updated plan? The current design supports this naturally --
   the user continues the conversation and the DM produces a revised plan. No
   special handling needed.

5. **Cross-mode context**: If a user creates a quest in quest mode, then switches to
   manage mode and asks "who should work on the quest I just created?", does the DM
   have that context? Yes, because conversation history is shared across modes within
   a session. The mode only changes the system prompt and output expectations.

---

## Summary

This design adds a four-mode routing layer to the DM chat that provides predictability
for humans while leveraging the existing reactive processor architecture. The modes are
explicit (not inferred), the pipeline for promoting agent output to new quests is
reactive (not synchronous), and the implementation is phased so each stage delivers
working value.

The key insight is that modes are not about limiting the LLM -- they are about setting
clear expectations for the human. When a user is in converse mode, they know the DM
will not accidentally create a quest. When they are in plan mode, they know the DM is
working toward an executable plan. This predictability is more valuable than a "smart"
system that guesses intent.
