# Guild Founding Quorum

## Status

**Accepted** -- March 2026

## Problem Statement

Guild creation is currently instantaneous: when an Expert+ agent triggers auto-formation
in `guildformation`, the component creates the guild as `active` and adds all candidates
in a single operation. This bypasses any agency on the candidates' part -- they have no
say in whether they join. More importantly, it skips the social negotiation that makes
guilds meaningful in the game metaphor.

The desired flow is founder-driven with a quorum gate:

1. Expert+ agent proposes founding a guild -- guild is created as `pending`, founder is
   the sole member.
2. Candidate agents discover the pending guild (boid engine scores guild affinity) and
   submit join applications.
3. The founder reviews each application during their next agentic loop iteration --
   accept or reject. This is a tool call, not a new quest type.
4. Once the guild reaches a configurable quorum (default: 3 = founder + 2 accepted),
   the guild transitions to `active`.
5. If the quorum is not met within a configurable timeout, the guild dissolves.

This makes guild formation a multi-step, agent-driven social process while reusing the
existing tool execution infrastructure (questtools, questbridge) for the founder's
review gate.

## Design Principles

1. **Agent agency matters.** Candidates choose to apply; founders choose to accept. No
   agent is silently added to a guild.

2. **Reuse the tool pipeline.** The founder reviews applications via a tool call during
   their agentic loop -- the same pattern as `review_sub_quest` in DAG execution. No new
   quest type, no new stream, no new consumer.

3. **Reactive, not polling.** Guild status transitions are driven by KV watch events.
   The guildformation component watches its own guild entities and promotes `pending` to
   `active` when the quorum is met, or dissolves on timeout.

4. **Boid engine drives discovery.** Pending guilds emit a signal that the boid engine
   can factor into its attraction scoring, and autonomy's `joinGuildAction` evolves into
   `applyToGuildAction` for pending guilds.

5. **Phased delivery.** Phase 1 delivers the core quorum lifecycle (domain types, guild
   formation changes, timeout). Phase 2 adds the tool call for founder review. Phase 3
   integrates boid scoring for pending guilds.

---

## Phase 1: Domain Types and Guild Lifecycle

### New Domain Types

#### GuildStatus Addition

Add `GuildPending` to `domain/types.go`:

```go
const (
    GuildPending  GuildStatus = "pending"   // Awaiting founding quorum
    GuildActive   GuildStatus = "active"
    GuildInactive GuildStatus = "inactive"
)
```

#### GuildApplication Entity

New struct in `domain/social.go`:

```go
// GuildApplication represents an agent's request to join a pending guild.
type GuildApplication struct {
    ID          string            `json:"id"`
    GuildID     GuildID           `json:"guild_id"`
    ApplicantID AgentID           `json:"applicant_id"`
    Status      ApplicationStatus `json:"status"`
    Message     string            `json:"message,omitempty"`  // Why the agent wants to join
    Skills      []SkillTag        `json:"skills,omitempty"`   // Agent's relevant skills
    Level       int               `json:"level"`
    Tier        TrustTier         `json:"tier"`
    ReviewedBy  *AgentID          `json:"reviewed_by,omitempty"`
    Reason      string            `json:"reason,omitempty"`   // Founder's accept/reject reason
    AppliedAt   time.Time         `json:"applied_at"`
    ReviewedAt  *time.Time        `json:"reviewed_at,omitempty"`
}

type ApplicationStatus string

const (
    ApplicationPending  ApplicationStatus = "pending"
    ApplicationAccepted ApplicationStatus = "accepted"
    ApplicationRejected ApplicationStatus = "rejected"
)
```

#### Guild Struct Changes

Add fields to `domain.Guild` in `domain/social.go`:

```go
type Guild struct {
    // ... existing fields ...

    // Founding quorum
    QuorumSize        int                `json:"quorum_size"`
    Applications      []GuildApplication `json:"applications,omitempty"`
    FormationDeadline *time.Time         `json:"formation_deadline,omitempty"`
}
```

### New Event Predicates

Add to `domain/vocab.go`:

```go
// Guild founding quorum predicates
PredicateGuildPending              = "guild.lifecycle.pending"
PredicateGuildActivated            = "guild.lifecycle.activated"
PredicateGuildDissolved            = "guild.lifecycle.dissolved"
PredicateGuildApplicationSubmitted = "guild.application.submitted"
PredicateGuildApplicationAccepted  = "guild.application.accepted"
PredicateGuildApplicationRejected  = "guild.application.rejected"
```

Register all six in `RegisterVocabulary()`.

### KV Key Patterns

Guild applications are stored as part of the guild entity state (not a separate bucket).
The guild entity in ENTITY_STATES already serializes `Members` and other nested data via
`Graphable()` triples. Applications follow the same pattern:

```
guild.application.{app_id}.applicant   = agent_id
guild.application.{app_id}.status      = "pending" | "accepted" | "rejected"
guild.application.{app_id}.applied_at  = timestamp
guild.application.{app_id}.reviewed_at = timestamp (when reviewed)
guild.application.{app_id}.message     = "I bring data wrangling expertise"
```

This means `GuildFromEntityState` reconstructs applications from triples, just like it
reconstructs members today from `guild.member.{agent_id}.*` predicates.

### Guildformation Component Changes

#### Config Additions

Add to `guildformation.Config`:

```go
type Config struct {
    // ... existing fields ...
    FormationTimeoutSec   int  `json:"formation_timeout_sec"`
    MinFoundingMembers    int  `json:"min_founding_members"`
    EnableQuorumFormation bool `json:"enable_quorum_formation"`
}
```

When `EnableQuorumFormation` is true, `CreateGuild` creates guilds as `GuildPending`.
When false (backward compat), guilds are created as `GuildActive` immediately.

#### CreateGuild Changes

```go
func (c *Component) CreateGuild(ctx context.Context, params CreateGuildParams) (*domain.Guild, error) {
    // ... existing validation ...

    status := domain.GuildActive
    var deadline *time.Time
    quorumSize := 1 // No quorum needed
    if c.config.EnableQuorumFormation {
        status = domain.GuildPending
        quorumSize = c.config.MinFoundingMembers
        t := time.Now().Add(time.Duration(c.config.FormationTimeoutSec) * time.Second)
        deadline = &t
    }

    guild := &domain.Guild{
        // ... existing fields ...
        Status:            status,
        QuorumSize:        quorumSize,
        FormationDeadline: deadline,
    }
    // ... rest unchanged, but emit "guild.lifecycle.pending" predicate
    // when quorum mode is enabled ...
}
```

#### New Method: SubmitApplication

```go
func (c *Component) SubmitApplication(ctx context.Context, guildID domain.GuildID,
    applicant *agentprogression.Agent, message string) error {
    // Validate guild exists and is pending
    // Validate applicant is not already a member or has a pending application
    // Create GuildApplication with status=pending
    // Append to guild.Applications
    // Persist via EmitEntity with "guild.application.submitted" predicate
    // Publish SubjectGuildApplicationSubmitted event
}
```

#### New Method: ReviewApplication

```go
func (c *Component) ReviewApplication(ctx context.Context, guildID domain.GuildID,
    applicationID string, founderID domain.AgentID, accepted bool, reason string) error {
    // Validate guild exists and is pending
    // Validate caller is the founder
    // Find application by ID, validate it is pending
    // Set status to accepted/rejected, set ReviewedBy, Reason, ReviewedAt
    // If accepted: add applicant to guild.Members as GuildRankInitiate
    // Persist via EmitEntity with accepted/rejected predicate
    // Check quorum: if len(guild.Members) >= guild.QuorumSize, activate guild
}
```

#### Quorum Check and Activation

```go
func (c *Component) checkQuorum(ctx context.Context, guild *domain.Guild) {
    if guild.Status != domain.GuildPending {
        return
    }
    if len(guild.Members) >= guild.QuorumSize {
        guild.Status = domain.GuildActive
        guild.FormationDeadline = nil
        c.graph.EmitEntity(ctx, guild, "guild.lifecycle.activated")
        // Publish activation event
    }
}
```

#### Timeout Dissolution

Add a goroutine in `Start()` that periodically checks pending guilds:

```go
func (c *Component) runFormationTimeoutLoop() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-c.stopChan:
            return
        case <-ticker.C:
            c.checkFormationTimeouts()
        }
    }
}

func (c *Component) checkFormationTimeouts() {
    now := time.Now()
    c.guilds.Range(func(key, value any) bool {
        guild := value.(*domain.Guild)
        if guild.Status == domain.GuildPending &&
            guild.FormationDeadline != nil &&
            now.After(*guild.FormationDeadline) {
            ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
            defer cancel()
            c.dissolveGuild(ctx, guild, "formation timeout: quorum not met")
        }
        return true
    })
}

func (c *Component) dissolveGuild(ctx context.Context, guild *domain.Guild, reason string) {
    guild.Status = domain.GuildInactive
    // Remove all agent guild mappings
    // Persist with "guild.lifecycle.dissolved" predicate
    // Publish dissolution event
    // Notify applicants (pending applications become rejected)
}
```

#### ListGuilds Changes

`ListGuilds` currently returns only active guilds. Add `ListPendingGuilds`:

```go
func (c *Component) ListPendingGuilds() []*domain.Guild {
    var guilds []*domain.Guild
    c.guilds.Range(func(_, value any) bool {
        original := value.(*domain.Guild)
        if original.Status == domain.GuildPending {
            cp := *original
            cp.Members = append([]domain.GuildMember(nil), original.Members...)
            cp.Applications = append([]domain.GuildApplication(nil), original.Applications...)
            guilds = append(guilds, &cp)
        }
        return true
    })
    return guilds
}
```

---

## Phase 2: Founder Review Tool

### Tool Definition: review_guild_applications

Register in `executor/tools.go` alongside `decompose_quest` and `review_sub_quest`:

```go
r.Register(RegisteredTool{
    Definition: agentic.ToolDefinition{
        Name:        "review_guild_applications",
        Description: "Review pending applications to a guild you founded. " +
            "Accept or reject each applicant with a reason.",
        Parameters: map[string]any{
            "type": "object",
            "properties": map[string]any{
                "guild_id": map[string]any{
                    "type":        "string",
                    "description": "The guild ID to review applications for",
                },
                "decisions": map[string]any{
                    "type": "array",
                    "items": map[string]any{
                        "type": "object",
                        "properties": map[string]any{
                            "application_id": map[string]any{
                                "type":        "string",
                                "description": "The application ID",
                            },
                            "accepted": map[string]any{
                                "type":        "boolean",
                                "description": "true to accept, false to reject",
                            },
                            "reason": map[string]any{
                                "type":        "string",
                                "description": "Reason for the decision",
                            },
                        },
                        "required": []any{"application_id", "accepted", "reason"},
                    },
                    "description": "Array of accept/reject decisions",
                },
            },
            "required": []any{"guild_id", "decisions"},
        },
    },
    Handler: reviewGuildApplicationsHandler(guildFormationRef),
    MinTier: domain.TierExpert,
})
```

### Tool Handler

The handler uses a `GuildFormationRef` interface (same pattern as `QuestBoardRef` in
questdagexec) to avoid import cycles:

```go
// GuildFormationRef is the narrow interface the tool handler needs.
type GuildFormationRef interface {
    ReviewApplication(ctx context.Context, guildID domain.GuildID,
        applicationID string, founderID domain.AgentID,
        accepted bool, reason string) error
}
```

Handler implementation:

```go
func reviewGuildApplicationsHandler(guilds GuildFormationRef) ToolHandler {
    return func(ctx context.Context, call agentic.ToolCall,
        quest *domain.Quest, agent *agentprogression.Agent) agentic.ToolResult {

        guildID, _ := call.Arguments["guild_id"].(string)
        decisions, _ := call.Arguments["decisions"].([]any)

        var results []string
        for _, d := range decisions {
            dec, ok := d.(map[string]any)
            if !ok { continue }
            appID, _ := dec["application_id"].(string)
            accepted, _ := dec["accepted"].(bool)
            reason, _ := dec["reason"].(string)

            err := guilds.ReviewApplication(ctx,
                domain.GuildID(guildID), appID, agent.ID, accepted, reason)
            if err != nil {
                results = append(results,
                    fmt.Sprintf("%s: error - %v", appID, err))
            } else {
                verb := "rejected"
                if accepted { verb = "accepted" }
                results = append(results,
                    fmt.Sprintf("%s: %s", appID, verb))
            }
        }

        return agentic.ToolResult{
            CallID:   call.ID,
            Content:  strings.Join(results, "\n"),
            StopLoop: true,
        }
    }
}
```

### Questbridge Integration: Injecting the Tool

The key question: how does the founder get `review_guild_applications` in their toolkit
during an agentic loop? Two paths were evaluated:

**Path A -- Synthetic quest (recommended for Phase 2):** When a pending guild receives
its first application, guildformation posts a synthetic quest assigned to the founder:
"Review guild applications for [guild name]." This quest has
`AllowedTools: ["review_guild_applications"]`. When the founder's autonomy loop claims
it (or it is directly assigned as `in_progress`), questbridge dispatches the agentic
loop with the review tool. The quest's `Input` contains the serialized pending
applications.

This reuses the entire existing pipeline (quest posting, claim, questbridge dispatch,
questtools execution) without any changes to questbridge's tool assembly logic.

**Path B -- Ambient tool (deferred to Phase 3):** The tool is always available to
Expert+ agents who have founded a pending guild. Questbridge's `toolsForQuest` would
check guild state and include the tool dynamically. More elegant but adds coupling
between questbridge and guildformation.

**Decision: Path A.** A synthetic quest is simple, observable (appears in the dashboard
activity feed), and requires zero changes to questbridge or questtools.

### Synthetic Quest Details

Posted by guildformation when a pending guild gets its first application:

```go
quest := &domain.Quest{
    Title:       fmt.Sprintf("Review guild applications for %s", guild.Name),
    Description: "You founded this guild and have pending applications to review.",
    Status:      domain.QuestInProgress,
    Difficulty:  domain.DifficultyTrivial,
    BaseXP:      10,
    AllowedTools: []string{"review_guild_applications"},
    Input: map[string]any{
        "guild_id":     string(guild.ID),
        "guild_name":   guild.Name,
        "applications": pendingApplications,
    },
}
// Assign directly to founder (bypass boid engine)
agentID := guild.FoundedBy
quest.ClaimedBy = &agentID
now := time.Now()
quest.ClaimedAt = &now
quest.StartedAt = &now
```

Post as `in_progress` so questbridge picks it up immediately via the KV twofer
bootstrap protocol.

### Component Wiring

The `review_guild_applications` tool handler needs a `GuildFormationRef`. Wire it the
same way as `QuestBoardRef` in questdagexec:

1. Define `GuildFormationRef` interface in `processor/executor/` (or a shared package).
2. In `cmd/semdragons/main.go` `wireComponentCrossReferences`, set the ref on the tool
   registry after both guildformation and executor are created.
3. The tool is registered in `RegisterBuiltins` with a nil-safe handler that returns an
   error if the ref is not wired.

---

## Phase 3: Autonomy and Boid Engine Integration

### Autonomy Changes

#### Bifurcate joinGuildAction for Pending vs Active Guilds

When `EnableQuorumFormation` is active, autonomy's `executeJoinGuild` checks guild
status:

- **Active guild:** Call `JoinGuild` directly (existing behavior).
- **Pending guild:** Call `SubmitApplication` instead.

```go
func (c *Component) executeJoinGuild(ctx context.Context,
    agent *agentprogression.Agent) error {
    allGuilds := c.guilds.ListGuilds()
    pendingGuilds := c.guilds.ListPendingGuilds()
    allGuilds = append(allGuilds, pendingGuilds...)

    // ... existing scoring logic ...

    best := suggestions[0]
    guild, _ := c.guilds.GetGuild(best.GuildID)

    if guild.Status == domain.GuildPending {
        return c.guilds.SubmitApplication(ctx, best.GuildID, agent,
            fmt.Sprintf("Skill affinity: %s", best.Reason))
    }
    // Active guild: join directly (existing path)
    return c.guilds.JoinGuild(ctx, best.GuildID, agent.ID)
}
```

#### createGuildAction: No Changes

`createGuildAction` already goes through `guildformation.CreateGuild`. The
`EnableQuorumFormation` flag controls whether the guild starts as `pending` or `active`.

### Boid Engine Changes

The boid engine currently only caches guilds for affinity scoring. For pending guilds,
the existing cache already stores them (no status filter in `handleGuildUpdate`). No
boid engine changes are required for basic functionality.

Optional Phase 3 enhancement: add a "founding attraction" rule where agents with skill
overlap to a pending guild's founder get a mild affinity pull. This is additive and
does not block core functionality.

### GuildsRef Interface Extension

The autonomy component accesses guild operations through a `GuildsRef` interface
(set via `SetGuilds`). Extend it for Phase 3:

```go
type GuildsRef interface {
    ListGuilds() []*domain.Guild
    ListPendingGuilds() []*domain.Guild         // New
    GetGuild(domain.GuildID) (*domain.Guild, bool)
    GetAgentGuilds(domain.AgentID) []domain.GuildID
    JoinGuild(context.Context, domain.GuildID, domain.AgentID) error
    SubmitApplication(context.Context, domain.GuildID,  // New
        *agentprogression.Agent, string) error
    CreateGuild(context.Context, guildformation.CreateGuildParams) (*domain.Guild, error)
}
```

---

## File Change Summary

### Phase 1 (Core Lifecycle)

| File | Change |
|------|--------|
| `domain/types.go` | Add `GuildPending` status, `ApplicationStatus` type and constants |
| `domain/social.go` | Add `GuildApplication` struct, `QuorumSize`/`Applications`/`FormationDeadline` to `Guild` |
| `domain/vocab.go` | Add 6 new predicates, register in `RegisterVocabulary()` |
| `domain/reconstruction.go` | Extend `GuildFromEntityState` to reconstruct `Applications` from triples |
| `graphable.go` | Extend `Guild.Graphable()` to emit application triples |
| `processor/guildformation/config.go` | Add `FormationTimeoutSec`, `MinFoundingMembers`, `EnableQuorumFormation` |
| `processor/guildformation/handler.go` | Add `SubmitApplication`, `ReviewApplication`, `checkQuorum`, `dissolveGuild`; modify `CreateGuild` |
| `processor/guildformation/payloads.go` | Add 5 new subjects and payload types |
| `processor/guildformation/component.go` | Start `runFormationTimeoutLoop`; add `ListPendingGuilds` |
| `config/semdragons.json` | Add quorum config fields to guildformation section |

### Phase 2 (Founder Review Tool)

| File | Change |
|------|--------|
| `processor/executor/tools.go` | Add `GuildFormationRef` interface, `review_guild_applications` tool and handler |
| `processor/guildformation/handler.go` | Post synthetic quest on first application arrival |
| `cmd/semdragons/main.go` | Wire `GuildFormationRef` in `wireComponentCrossReferences` |

### Phase 3 (Autonomy and Boid Integration)

| File | Change |
|------|--------|
| `processor/autonomy/actions.go` | Bifurcate `executeJoinGuild` for pending vs active guilds |
| `processor/autonomy/component.go` | Extend `GuildsRef` interface with `ListPendingGuilds`, `SubmitApplication` |
| `processor/boidengine/handler.go` | (Optional) founding attraction rule for pending guilds |

---

## Testing Strategy

### Unit Tests (Phase 1)

1. **`processor/guildformation/handler_test.go`** (new file):
   - `TestCreateGuild_QuorumEnabled_StartsPending` -- guild status is `pending`, deadline set.
   - `TestCreateGuild_QuorumDisabled_StartsActive` -- backward compat preserved.
   - `TestSubmitApplication_ValidApplicant` -- application stored, event emitted.
   - `TestSubmitApplication_AlreadyMember` -- rejected.
   - `TestSubmitApplication_GuildNotPending` -- rejected for active guilds.
   - `TestSubmitApplication_DuplicateApplication` -- idempotent or rejected.
   - `TestReviewApplication_AcceptReachesQuorum` -- guild activates on Nth accept.
   - `TestReviewApplication_AcceptBelowQuorum` -- member added, guild stays pending.
   - `TestReviewApplication_Reject` -- application rejected, member not added.
   - `TestReviewApplication_NotFounder` -- only founder can review.
   - `TestFormationTimeout_DissolvesPendingGuild` -- guild dissolved after deadline.
   - `TestFormationTimeout_IgnoresActiveGuild` -- active guilds untouched.
   - `TestDissolveGuild_RejectsRemainingApplications` -- cleanup.

2. **`domain/reconstruction_test.go`** (extend):
   - `TestGuildFromEntityState_WithApplications` -- round-trip triple serialization.

### Unit Tests (Phase 2)

3. **`processor/executor/tools_test.go`** (extend):
   - `TestReviewGuildApplications_AcceptAll` -- batch accept.
   - `TestReviewGuildApplications_RejectSome` -- mixed decisions.
   - `TestReviewGuildApplications_NilRef` -- graceful error.
   - `TestReviewGuildApplications_TierGate` -- only Expert+ can use.

### Integration Tests (Phases 1+2, require Docker)

4. **`processor/guildformation/component_test.go`** (extend):
   - `TestGuildFoundingQuorum_EndToEnd` -- pending -> applications -> accept -> active.
   - `TestGuildFoundingQuorum_Timeout` -- pending -> timeout -> dissolved.
   - `TestGuildFoundingQuorum_SyntheticQuest` -- review quest posted and dispatched.

### Integration Tests (Phase 3)

5. **`processor/autonomy/component_test.go`** (extend):
   - `TestAutonomy_ApplyToPendingGuild` -- submits application.
   - `TestAutonomy_JoinActiveGuild_Unchanged` -- existing behavior preserved.

---

## Configuration Defaults

```json
{
  "guildformation": {
    "org": "c360",
    "platform": "local-dev",
    "board": "board1",
    "min_members_for_formation": 3,
    "max_guild_size": 20,
    "enable_auto_formation": true,
    "formation_timeout_sec": 300,
    "min_founding_members": 3,
    "enable_quorum_formation": true
  }
}
```

For E2E tests, use a shorter timeout (30s) and quorum of 2.

---

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Founder goes offline during pending period | Timeout dissolution prevents zombie guilds |
| Race: two agents apply simultaneously | Applications appended to guild entity via `EmitEntity`; CAS retry if needed |
| Synthetic quest not picked up by founder | Post as `in_progress` (bypass boid engine). Timeout dissolution is backstop |
| Backward compatibility | `EnableQuorumFormation: false` preserves instant-creation behavior |
| Too many pending guilds | `createGuildAction` already gates on "not already a guildmaster"; add cap if needed |

## Alternatives Considered

1. **Application via NATS pub/sub instead of guild entity state.** Rejected: applications
   need persistence for crash recovery and the KV twofer already provides both state and
   events.

2. **Dedicated GUILD_APPLICATIONS KV bucket.** Rejected for simplicity: applications are
   small, transient, and tightly coupled to guild state. Embedding them avoids
   cross-bucket consistency concerns.

3. **Ambient tool in questbridge instead of synthetic quest.** Deferred to Phase 3: the
   synthetic quest approach requires zero changes to questbridge and is more observable
   in the dashboard.
