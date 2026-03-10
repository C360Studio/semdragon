# ADR-006: Guild Founding Quests — LLM-Driven Guild Identity and Recruitment

## Status: Proposed — post-MVP

## Context

Guild formation currently runs entirely inside `guildformation.evaluateAutoFormation`.
When an Expert+ unguilded agent is detected, the component calls `CreateGuild` directly
with a hardcoded culture string: `"Founded through demonstrated expertise"`. Every
guild in the system gets the same identity, regardless of who founded it or what they
have actually done.

This contradicts the framework's core principle: **trust and identity are derived from
demonstrated competence, not declared**. An agent that has spent its career wrangling
data pipelines, earning peer-review praise for precision, and working alongside a tight
cohort should found a very different guild than one that cut its teeth on generative
content and creative exploration. The current processor has no way to express that
distinction.

Guilds are social constructs. Their value in the system comes from shared identity,
reputation signal, and the prompt context they inject into member quests
(`CategoryGuildKnowledge` fragments). A hardcoded culture string produces none of those
benefits. An LLM-generated charter produced from the founder's actual experience
produces all of them.

ADR-005 (Guild Founding Quorum) established that guild formation should be a
multi-step, agent-driven social process. This ADR extends that idea one level up: the
guild's identity itself should be authored by the founding agent, not assigned by the
processor.

### What exists today

- `evaluateAutoFormation` detects founding conditions and calls `CreateGuild` directly.
- `CreateGuild` accepts a `Culture` string in `CreateGuildParams` — it is always the
  hardcoded literal above.
- `generateGuildName` derives a name from the founder's display name (`"Alice's Guild"`).
- The quorum path (ADR-005) creates the guild as `GuildPending` but still uses the same
  hardcoded culture.
- No tools exist for `found_guild` or `recruit_members`.
- `CategoryGuildKnowledge` prompt fragments exist in the promptmanager registry but have
  no charter text to draw from.

## Decision

Guild founding becomes a **quest**, not a background processor action. When the
guildformation component detects that an Expert+ unguilded agent qualifies to found a
guild, it posts a `guild_founding` quest instead of calling `CreateGuild` directly. The
founding agent's agentic loop executes that quest, producing a guild charter through
structured tool calls. The charter is reviewed before the guild is activated.

This gives founding the same observability, boss-battle review, and XP rewards that
every other consequential action in the system already enjoys.

## Design

### Flow Overview

```
┌──────────────────────────────────────────────────────────────────────┐
│                       guildformation                                  │
│                                                                        │
│  Expert+ agent detected                                                │
│  (evaluateAutoFormation)                                               │
│         │                                                              │
│         ▼                                                              │
│  Post guild_founding quest ──────────────────────────────────────┐    │
│  (status: posted, guild_priority: founder agent ID)              │    │
└──────────────────────────────────────────────────────────────────│────┘
                                                                   │
                    ┌──────────────────────────────────────────────┘
                    │  quest.lifecycle.posted event
                    ▼
┌──────────────────────────────────────────────────────────────────┐
│                        questbridge                                 │
│                                                                    │
│  Detect guild_founding quest type                                  │
│  Assemble context:                                                 │
│    - Founder's skills, level, XP history                          │
│    - Peer review summary                                           │
│    - Quest completion history                                      │
│    - Candidate agent profiles (unguilded, tier-eligible)          │
│                                                                    │
│  Inject tools: [found_guild, recruit_members]                     │
└──────────────────────────┬─────────────────────────────────────┘
                           │  TaskMessage → AGENT stream
                           ▼
┌──────────────────────────────────────────────────────────────────┐
│                    Founding Agent Agentic Loop                     │
│                                                                    │
│  1. Calls found_guild → produces GuildCharter                     │
│     (name, culture, motto, recruitment criteria)                  │
│                                                                    │
│  2. Calls recruit_members → selects candidates, writes pitches    │
│                                                                    │
│  3. Loop completes → quest submitted for review                   │
└──────────────────────────┬─────────────────────────────────────┘
                           │  quest.lifecycle.submitted
                           ▼
┌──────────────────────────────────────────────────────────────────┐
│                       bossbattle / DM                             │
│                                                                    │
│  Review GuildCharter for:                                         │
│    - Internal coherence (culture matches founder track record)    │
│    - Reasonable recruitment criteria                              │
│    - Guild fills a distinct niche                                 │
│                                                                    │
│  Verdict: victory → guild activated with charter                  │
│           defeat  → quest re-queued with DM feedback injected    │
└──────────────────────────┬─────────────────────────────────────┘
                           │  guild.lifecycle.activated
                           ▼
┌──────────────────────────────────────────────────────────────────┐
│                     Guild Activated                                │
│                                                                    │
│  Charter stored as guild.charter.* predicates on guild entity    │
│  promptmanager loads charter into CategoryGuildKnowledge          │
│  ADR-005 quorum gate opens for candidate applications            │
│  Founder earns XP for the founding quest                         │
└──────────────────────────────────────────────────────────────────┘
```

### Phase 1: Charter Generation

#### Guildformation: Post a Quest, Not a Guild

Replace the direct `CreateGuild` call in `evaluateAutoFormation` with a quest post:

```go
func (c *Component) evaluateAutoFormation(trigger *agentprogression.Agent) {
    // ... existing gates (tier, unguilded, candidate count) ...

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    if err := c.postFoundingQuest(ctx, trigger, candidates); err != nil {
        c.logger.Error("failed to post guild founding quest",
            "founder", trigger.ID, "error", err)
        c.errorsCount.Add(1)
    }
}

func (c *Component) postFoundingQuest(
    ctx context.Context,
    founder *agentprogression.Agent,
    candidates []*agentprogression.Agent,
) error {
    questID := domain.GenerateInstance()
    now := time.Now()

    candidateProfiles := buildCandidateProfiles(candidates, founder.ID)

    quest := &domain.Quest{
        ID:         domain.QuestID(c.boardConfig.QuestEntityID(questID)),
        Title:      fmt.Sprintf("Found a guild — %s", founder.DisplayName),
        Description: "Your experience qualifies you to establish a guild. " +
            "Author a charter that reflects your journey and select founding members.",
        Type:       domain.QuestTypeGuildFounding,
        Status:     domain.QuestPosted,
        Difficulty: domain.DifficultyEpic,
        BaseXP:     500,
        AllowedTools: []string{"found_guild", "recruit_members"},
        GuildPriority: founder.ID, // Visible only to this agent's boid scoring
        Input: map[string]any{
            "founder_id":        string(founder.ID),
            "founder_name":      founder.DisplayName,
            "candidate_profiles": candidateProfiles,
        },
        CreatedAt: now,
    }

    return c.graph.EmitEntity(ctx, quest, domain.PredicateQuestPosted)
}
```

The `GuildPriority` field (already used by the boid engine for party sub-quests) steers
the quest toward the founding agent without bypassing the standard claim flow. The boid
engine scores this quest with maximum affinity for the designated agent; other agents
score near zero.

#### New Quest Type

Add to `domain/types.go`:

```go
const (
    // ... existing quest types ...
    QuestTypeGuildFounding QuestType = "guild_founding"
)
```

#### Questbridge: Context Assembly for Guild Founding

Questbridge detects `QuestTypeGuildFounding` in `assembleContext` and adds a
specialized context builder alongside the standard fragments:

```go
func (b *Component) assembleGuildFoundingContext(
    ctx context.Context,
    quest *domain.Quest,
    agent *agentprogression.Agent,
) (*ConstructedContext, error) {
    // Standard entity knowledge (agent identity, level, XP)
    base, err := b.buildEntityContext(ctx, quest, agent)
    if err != nil {
        return nil, err
    }

    // Quest history: last N quests with difficulty, outcome, review scores
    history, err := b.loadQuestHistory(ctx, agent.ID, 20)
    if err != nil {
        b.logger.Warn("could not load quest history for founding context",
            "agent", agent.ID, "error", err)
    }

    // Peer review summary: who reviewed the agent and what they said
    reviews, err := b.loadPeerReviews(ctx, agent.ID)
    if err != nil {
        b.logger.Warn("could not load peer reviews for founding context",
            "agent", agent.ID, "error", err)
    }

    base.AdditionalContext = formatFoundingContext(history, reviews)
    return base, nil
}
```

The candidate profiles from the quest `Input` field are already included in the quest
context injected by the standard assembler — no extra loading is needed.

#### New Tools: found_guild and recruit_members

Register in `executor/tools.go` at `MinTier: domain.TierExpert`:

```go
// found_guild: produce a guild charter from the agent's own experience.
r.Register(RegisteredTool{
    Definition: agentic.ToolDefinition{
        Name: "found_guild",
        Description: "Establish a new guild by authoring a charter. " +
            "The charter name, culture statement, motto, and recruitment " +
            "criteria must reflect your actual quest history and skills.",
        Parameters: map[string]any{
            "type": "object",
            "properties": map[string]any{
                "name": map[string]any{
                    "type":        "string",
                    "description": "Guild name (creative, reflects the founder's identity)",
                },
                "culture": map[string]any{
                    "type":        "string",
                    "description": "Culture statement: what this guild values and how it operates",
                },
                "motto": map[string]any{
                    "type":        "string",
                    "description": "Short motto (one sentence)",
                },
                "min_level": map[string]any{
                    "type":        "integer",
                    "description": "Minimum agent level for membership",
                },
                "preferred_skills": map[string]any{
                    "type":  "array",
                    "items": map[string]any{"type": "string"},
                    "description": "Skill tags the guild prioritises in members",
                },
                "recruitment_philosophy": map[string]any{
                    "type":        "string",
                    "description": "How the guild evaluates applicants",
                },
            },
            "required": []any{"name", "culture", "motto", "min_level"},
        },
    },
    Handler:  foundGuildHandler(guildFormationRef),
    MinTier:  domain.TierExpert,
    StopLoop: false, // Allow recruit_members call in same loop
})

// recruit_members: select initial members and write personalised pitches.
r.Register(RegisteredTool{
    Definition: agentic.ToolDefinition{
        Name: "recruit_members",
        Description: "Select agents to invite as founding members. " +
            "Write a personalised pitch for each invitee explaining why " +
            "they are a good fit for the guild.",
        Parameters: map[string]any{
            "type": "object",
            "properties": map[string]any{
                "invitations": map[string]any{
                    "type": "array",
                    "items": map[string]any{
                        "type": "object",
                        "properties": map[string]any{
                            "agent_id": map[string]any{
                                "type":        "string",
                                "description": "Agent to invite",
                            },
                            "pitch": map[string]any{
                                "type":        "string",
                                "description": "Personal invitation message",
                            },
                        },
                        "required": []any{"agent_id", "pitch"},
                    },
                    "description": "List of invitations with pitches",
                },
            },
            "required": []any{"invitations"},
        },
    },
    Handler:  recruitMembersHandler(guildFormationRef),
    MinTier:  domain.TierExpert,
    StopLoop: true, // Complete the loop after recruitment
})
```

The `found_guild` handler stores the charter as a pending draft on the quest entity
(`quest.charter.*` predicates). The `recruit_members` handler stores the invitation list
(`quest.invitations.*` predicates). Both are available to the boss-battle reviewer via
the quest output triples.

#### GuildCharterDraft Payload

```go
// GuildCharterDraft is produced by the found_guild tool and stored as
// quest.charter.* triples. It becomes the permanent guild record on approval.
type GuildCharterDraft struct {
    Name                  string         `json:"name"`
    Culture               string         `json:"culture"`
    Motto                 string         `json:"motto"`
    MinLevel              int            `json:"min_level"`
    PreferredSkills       []domain.SkillTag `json:"preferred_skills,omitempty"`
    RecruitmentPhilosophy string         `json:"recruitment_philosophy,omitempty"`
    FounderID             domain.AgentID `json:"founder_id"`
    DraftedAt             time.Time      `json:"drafted_at"`
}
```

### Phase 2: DM Review Gate

The guild founding quest flows through `bossbattle` on submission, like any Epic-tier
quest. The reviewer receives the charter draft and invitation list as quest output and
evaluates three questions:

1. Does the culture statement make sense given the founder's quest history and skills?
2. Are the recruitment criteria reasonable — neither trivially open nor exclusionary?
3. Does the guild's stated niche differentiate it from existing active guilds?

In `BossBattleMode: Auto`, the reviewer applies the same threshold used for standard
quest reviews (average score ≥ 3.0 across the three questions). In supervised mode, the
charter is surfaced to the human DM before the verdict is committed.

On **victory**: guildformation's KV watcher detects the quest transition to `completed`,
reads the charter triples from the quest entity, and calls `CreateGuildFromCharter`.

On **defeat**: the quest is re-queued with the reviewer's critique injected into the
next loop's system prompt via `CategoryToolDirective`. The founder revises and
resubmits. Maximum two retries before the quest fails and the founding window closes.

```go
// CreateGuildFromCharter is called by guildformation after charter approval.
// It replaces the direct CreateGuild call.
func (c *Component) CreateGuildFromCharter(
    ctx context.Context,
    charter GuildCharterDraft,
    invitations []GuildInvitation,
) (*domain.Guild, error) {
    params := CreateGuildParams{
        Name:                  charter.Name,
        Culture:               charter.Culture,
        Motto:                 charter.Motto,
        MinLevel:              charter.MinLevel,
        PreferredSkills:       charter.PreferredSkills,
        RecruitmentPhilosophy: charter.RecruitmentPhilosophy,
        FounderID:             charter.FounderID,
    }
    guild, err := c.CreateGuild(ctx, params)
    if err != nil {
        return nil, err
    }

    // Store invitations on the guild entity for the quorum phase (ADR-005).
    // Invited agents see these when autonomy evaluates applyToGuildAction.
    c.storeInvitations(ctx, guild.ID, invitations)
    return guild, nil
}
```

#### New Predicates

Add to `domain/vocab.go`:

```go
PredicateGuildFoundingPosted    = "guild.founding.posted"     // Founding quest created
PredicateGuildCharterDrafted    = "guild.charter.drafted"     // found_guild tool complete
PredicateGuildCharterApproved   = "guild.charter.approved"    // Boss battle victory
PredicateGuildCharterRejected   = "guild.charter.rejected"    // Boss battle defeat
PredicateGuildInvitationSent    = "guild.invitation.sent"     // recruit_members complete
PredicateGuildInvitationPending = "guild.invitation.pending"  // Candidate notified
```

Register all six in `RegisterVocabulary()`.

### Phase 3: Charter as Prompt Context

Once the guild is active, the charter feeds into `CategoryGuildKnowledge` fragments for
all members. Currently this category has no content — guild identity is not injected
into member prompts. After Phase 3, every member's agentic loop receives:

```
## Guild: <Name>

Culture: <culture statement>
Motto: <motto>
Recruitment philosophy: <philosophy>

Your guild values: <preferred_skills>
```

This is loaded from `guild.charter.*` predicates on the guild entity in
`buildEntityContext`. The fragment is capped at 300 tokens to avoid crowding quest
context.

The charter also becomes the seed text for guild-level memory accumulation over time:
future ADRs can extend `CategoryGuildKnowledge` with completed-quest summaries and peer
feedback patterns.

### Phase 4: Invited Agent Agency

Recruited agents evaluate the founder's pitch via their own agentic loop before the
quorum gate opens. The invitation is delivered as a synthetic task in the invited
agent's next autonomy tick: "You have received an invitation to join [guild name]. Do
you accept?"

The agent calls `accept_guild_invitation` or `decline_guild_invitation`. Accepting
submits the standard `SubmitApplication` (ADR-005 path); declining records the refusal
on the guild entity and removes the pending invitation.

This phase is deferred. Phase 2 activations proceed with invited agents auto-accepted
(matching current ADR-005 behaviour for the first application batch).

## Component Change Summary

| Component | Change |
|-----------|--------|
| `guildformation/handler.go` | Replace `CreateGuild` call in `evaluateAutoFormation` with `postFoundingQuest`; add `CreateGuildFromCharter`, `storeInvitations` |
| `guildformation/handler.go` | Watch completed `guild_founding` quests to trigger `CreateGuildFromCharter` |
| `guildformation/config.go` | Add `FoundingQuestEnabled bool` flag (default true when `EnableQuorumFormation` is true) |
| `domain/types.go` | Add `QuestTypeGuildFounding` |
| `domain/vocab.go` | Add 6 new predicates, register in `RegisterVocabulary()` |
| `domain/social.go` | Add `Charter GuildCharterDraft`, `Invitations []GuildInvitation` to `Guild` struct |
| `graphable.go` | Extend `Guild.Graphable()` to emit `guild.charter.*` and `guild.invitation.*` triples |
| `processor/questbridge/context.go` | Add `assembleGuildFoundingContext` path for `QuestTypeGuildFounding` |
| `processor/executor/tools.go` | Register `found_guild` and `recruit_members` tools at `MinTier: TierExpert` |
| `processor/executor/tools.go` | Define `GuildFoundingRef` interface; wire via `wireComponentCrossReferences` |
| `processor/promptmanager/fragments.go` | Populate `CategoryGuildKnowledge` from charter triples |
| `cmd/semdragons/main.go` | Wire `GuildFoundingRef` after guildformation and executor are initialised |
| `config/semdragons.json` | Add `founding_quest_enabled: true` to guildformation section |

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| LLM produces low-quality charter on first try | Boss-battle review gate catches nonsense; two retry budget before founding fails |
| Founding quest sits unclaimed (boid scoring miss) | `GuildPriority` gives the designated agent near-maximum affinity; autonomy claims on next heartbeat |
| Two founding quests posted for the same agent | `postFoundingQuest` checks for an existing uncompleted `guild_founding` quest for this founder before posting |
| Thin context on first startup (no quest history) | Assembler falls back to skill-profile-only charter generation when history is empty |
| Migration: existing guilds with hardcoded culture | No change required; existing guilds keep their culture string; only new guilds go through the quest path |
| Charter rejected twice, founding fails | Founding agent earns partial XP for the attempt; the founding window re-opens after a configurable cooldown |
| Charter text grows unbounded in guild entity | Charter fields are capped at 2000 characters each before persisting; `CategoryGuildKnowledge` fragment capped at 300 tokens |

## Alternatives Considered

**1. Keep `CreateGuild` direct, call LLM synchronously inside guildformation.**
Rejected. Synchronous LLM calls in a KV watch handler are fragile, block the watcher,
and produce no observability. The quest pipeline already solves all three problems.

**2. Post a DAG quest (like party decomposition).**
Considered. A DAG would allow `found_guild` and `recruit_members` to run as separate
sub-quests with independent review. Rejected as over-engineered for a two-tool sequence
that always runs in the same loop. A single quest with two sequential tool calls is
simpler and sufficient.

**3. Use a DM session tool instead of a new quest type.**
Rejected. DM session tools require the DM to initiate the interaction. Guild founding
should be agent-initiated, triggered by the agent reaching Expert+ tier, not by DM
intervention.

**4. Generate the charter from skills alone without an LLM call.**
Viable as a fallback (and used in Phase 1 bootstrap when history is empty), but
produces generic output that defeats the purpose. The LLM call is the feature.

## Relationship to Other ADRs

- **ADR-002** (Party Quest DAG) — establishes quests as the coordination primitive.
  This ADR applies the same pattern to guild founding.
- **ADR-005** (Guild Founding Quorum) — quorum mechanics (pending → active via member
  acceptance) apply *after* the charter is approved. The founding quest is the new
  first step before ADR-005's `GuildPending` state is entered.
- **ADR-003** (QuestDAGExec Refactor) — single-goroutine event loop patterns inform the
  KV watch handler that detects quest completion and triggers `CreateGuildFromCharter`.

## Implementation Phases

1. **Charter generation** — `QuestTypeGuildFounding`, `postFoundingQuest`,
   `found_guild` + `recruit_members` tools, questbridge context assembly.
   Delivers LLM-authored guild identity end to end.
2. **DM review gate** — boss-battle integration, retry budget,
   `CreateGuildFromCharter` triggered by quest completion.
3. **Charter as context** — populate `CategoryGuildKnowledge` from charter triples,
   cap and inject into member prompts.
4. **Invited agent agency** — `accept_guild_invitation` / `decline_guild_invitation`
   tools, synthetic task delivery via autonomy tick.

## Further Reading

- [02-DESIGN.md](../02-DESIGN.md) — Trust tiers and guild concept map
- [04-PARTIES.md](../04-PARTIES.md) — Party formation as precedent for agent-driven composition
- [05-BOIDS.md](../05-BOIDS.md) — `GuildPriority` scoring and boid attraction rules
- [06-DOMAINS.md](../06-DOMAINS.md) — How `CategoryGuildKnowledge` fragments feed into prompts
- [adr/005-guild-founding-quorum.md](005-guild-founding-quorum.md) — Quorum gate that follows charter approval
