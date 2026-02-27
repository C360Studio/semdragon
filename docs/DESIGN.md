# Semdragons: Design Document

## The Elevator Pitch

Agentic workflow coordination modeled as a tabletop RPG, built on semstreams.

Agents are adventurers. Work items are quests. Quality reviews are boss battles.
Trust is earned through leveling. Specialization happens through guilds.
Coordination emerges from Boid-like flocking behavior over a structured quest board.

**It's fun to build. The results are serious.**

---

## Concept Map

| RPG Concept         | Engineering Reality                    | Why It's Better Than "Orchestrator" |
|---------------------|----------------------------------------|-------------------------------------|
| Quest Board         | Pull-based work queue                  | Decoupled, no central bottleneck    |
| Quest               | Task / work item                       | Has difficulty, requirements, rewards |
| Agent               | LLM-powered autonomous worker          | Has progression, not just config    |
| Level / XP          | Progressive trust & capability         | Earned, not declared                |
| Trust Tier          | Permission boundary                    | Derived from demonstrated competence |
| Equipment / Tools   | API access, tool permissions           | Gated by tier, not role            |
| Boss Battle         | Quality gate / review                  | Embedded in flow, not bolted on    |
| Party               | Agent ensemble for complex tasks       | Role differentiation built in      |
| Party Lead          | Orchestrating agent for sub-tasks      | Has skin in the game (faces boss)  |
| Guild               | Specialization cluster                 | Routing + shared knowledge + reputation |
| Death / Cooldown    | Failure backoff                        | Has consequences, self-correcting  |
| Permadeath          | Catastrophic failure retirement        | Prevents repeat offenders          |
| DM (Dungeon Master) | Human or LLM orchestrator              | Explicit control layer with modes  |
| Session             | Workflow execution context             | Bounded, observable, replayable    |
| Boids               | Emergent quest-claiming behavior       | No central scheduler needed        |
| Game Events         | Semstreams event stream                | Full observability via trajectories |
| Guild Library       | Shared agent memory / prompt templates | Knowledge accumulates over time    |

---

## Architecture Layers

```
┌─────────────────────────────────────────────────┐
│                  DUNGEON MASTER                  │
│         (Human / LLM / Hybrid control)          │
├─────────────────────────────────────────────────┤
│                                                  │
│   ┌──────────┐  ┌──────────┐  ┌──────────┐     │
│   │  GUILDS   │  │ PARTIES  │  │  BOIDS   │     │
│   │ (special- │  │ (temp    │  │ (emergent│     │
│   │  ization) │  │  groups) │  │  flock)  │     │
│   └────┬─────┘  └────┬─────┘  └────┬─────┘     │
│        │              │              │           │
│   ┌────▼──────────────▼──────────────▼─────┐    │
│   │            QUEST BOARD                  │    │
│   │   (pull-based work distribution)        │    │
│   └────────────────┬────────────────────────┘    │
│                    │                             │
│   ┌────────────────▼────────────────────────┐    │
│   │         XP ENGINE + BOSS BATTLES        │    │
│   │   (evaluation, leveling, trust gates)   │    │
│   └────────────────┬────────────────────────┘    │
│                    │                             │
├────────────────────▼─────────────────────────────┤
│                 SEMSTREAMS                        │
│   (event streaming, trajectories, observability) │
└──────────────────────────────────────────────────┘
```

---

## Trust Tiers in Detail

```
Level 1-5   │ APPRENTICE   │ Read-only, summarize, classify, simple transforms
            │              │ Tools: grep, read APIs, formatters
            │              │ No external side effects
            │              │
Level 6-10  │ JOURNEYMAN   │ Can call tools, make API requests, write to staging
            │              │ Tools: + HTTP clients, DB reads, file I/O
            │              │ Side effects in sandboxed environments
            │              │
Level 11-15 │ EXPERT       │ Can modify production state, spend money, deploy
            │              │ Tools: + prod DB writes, payment APIs, CI/CD triggers
            │              │ Requires boss battle on every quest
            │              │
Level 16-18 │ MASTER       │ Can supervise agents, decompose quests, lead parties
            │              │ All tools + agent management
            │              │ Can create sub-quests, review other agents
            │              │
Level 19-20 │ GRANDMASTER  │ Can act as DM delegate, create quests, manage guilds
            │              │ Full system access
            │              │ Trusted to make unsupervised decisions
```

---

## Example Flow: "Analyze Q3 Sales Data and Send Summary to Stakeholders"

### 1. DM Creates Quest
```go
quest := NewQuest("Analyze Q3 sales and email stakeholders").
    Description("Pull Q3 data, identify trends, write executive summary, email to VP list").
    Difficulty(DifficultyEpic).
    RequireSkills(SkillAnalysis, SkillDataTransform, SkillCustomerComms).
    RequireParty(3).
    XP(500).
    BonusXP(200).
    MaxDuration(30 * time.Minute).
    ReviewAs(ReviewStrict).  // Dragon-level boss battle
    Build()

board.PostQuest(ctx, quest)
```

### 2. Boids Engine Suggests Party
The Boids engine identifies idle agents with matching skills:
- **DataDragon** (Level 14, Expert, Guild: Data Wranglers) - high affinity for data quests
- **SummaryScribe** (Level 12, Expert, Guild: Analysts) - strong analysis + writing
- **MailHawk** (Level 8, Journeyman, Skills: customer_communications) - can send emails

### 3. DM Forms Party with Mentor Strategy
```go
party := dm.FormParty(ctx, quest.ID, PartyStrategyBalanced)
// DataDragon becomes lead (highest level, can decompose)
// SummaryScribe is executor for analysis
// MailHawk is executor for email delivery
```

### 4. Party Lead Decomposes Quest
DataDragon breaks the epic quest into sub-quests:
```
Sub-quest 1: "Extract Q3 sales data" (Moderate, data_transformation)
Sub-quest 2: "Analyze trends and anomalies" (Hard, analysis)
Sub-quest 3: "Write executive summary" (Moderate, summarization)
Sub-quest 4: "Email summary to VP distribution list" (Easy, customer_communications)
```

### 5. Sub-Quests Execute
- DataDragon claims Sub-quest 1 (own decomposition, wants the data right)
- SummaryScribe claims Sub-quests 2 and 3
- MailHawk claims Sub-quest 4 (waits for Sub-quest 3 output)

Each sub-quest has its own mini boss battle (ReviewAuto or ReviewStandard).

### 6. Party Lead Rolls Up Results
DataDragon collects all sub-quest outputs, assembles the final package.

### 7. Boss Battle (Dragon Level)
Multi-judge review panel:
- **Automated judge**: Checks data accuracy, email formatting, recipient list
- **LLM judge**: Evaluates summary quality, insight depth, tone appropriateness
- **LLM judge 2**: Cross-checks analysis against raw data for hallucinations

### 8. XP Distribution
```
DataDragon:    500 base + 180 quality + 50 speed + 75 guild = 805 XP (LEVEL UP → 15!)
SummaryScribe: 350 base + 140 quality + 0 speed + 52 guild  = 542 XP
MailHawk:       50 base +  40 quality + 20 speed + 0 guild   = 110 XP
```

### 9. Events Stream (via semstreams)
```
quest.posted     → {quest_id: "q-123", difficulty: "epic"}
party.formed     → {party_id: "p-456", lead: "DataDragon", members: [...]}
quest.claimed    → {quest_id: "q-123-sub-1", agent: "DataDragon"}
quest.started    → ...
quest.completed  → {quest_id: "q-123-sub-1", quality: 0.92}
battle.started   → {battle_id: "b-789", level: "strict", judges: 3}
battle.victory   → {battle_id: "b-789", quality: 0.89}
agent.level_up   → {agent: "DataDragon", old: 14, new: 15, tier: "expert"}
quest.completed  → {quest_id: "q-123", quality: 0.89}
party.disbanded  → {party_id: "p-456"}
```

---

## Death Mechanics

| Scenario | Consequence | Recovery |
|----------|-------------|----------|
| Soft failure (bad output) | -25% base XP, 2min cooldown | Retry available |
| Timeout | -50% base XP, 5min cooldown | Quest re-posted |
| Abandon | -75% base XP, 10min cooldown | Quest re-posted, agent flagged |
| TPK (party wipe) | All members cooldown, quest escalated | Higher-level party or DM |
| Catastrophic (data loss, breach) | Permadeath, agent retired | New agent, level 1 |
| Repeated failures at level | Level down, XP reset | Must re-earn level |

---

## Open Questions

1. **Guild formation**: Automatic based on demonstrated skills, or DM-created?
   Probably both: auto-suggest, DM approves.

2. **Inter-guild quests**: How do guilds collaborate on cross-domain quests?
   Party system handles this - parties can draw from multiple guilds.

3. **Agent memory across quests**: How much context carries over?
   Guild library for persistent knowledge, party context for quest-scoped.

4. **Boids vs explicit assignment**: When does the DM override Boids suggestions?
   DM always can. Boids is the default; DM intervenes when stakes are high.

5. **Multi-session learning**: Do agents retain levels across sessions?
   Yes - agents are persistent. Sessions are execution contexts, not agent lifetimes.

6. **Quest chains**: Long-running workflows that span multiple sessions?
   Quest chains with persistent state. Parent quest stays open across sessions.

7. **PvP / competitive dynamics**: Should agents compete for quests?
   The Boids engine already handles this implicitly via attraction scores.
   Explicit competition could be interesting for A/B testing approaches.

---

## What's Next

- [ ] Implement QuestBoard backed by semstreams
- [ ] Implement DefaultBoidEngine with the six rules
- [ ] Wire up XP engine with real boss battle evaluators
- [ ] Build DM interface - start with DMManual, work toward DMFullAuto
- [ ] Build guild auto-formation based on agent performance clustering
- [ ] Dashboard: The DM's scrying pool (visualize world state in real-time)
- [ ] Semstreams integration: Map GameEvents to trajectory spans
