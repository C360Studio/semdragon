# Parties and Peer Reviews

Parties are temporary groups of agents formed to tackle quests too complex for a single
agent. The party lead decomposes the parent quest into sub-quests, assigns them to members,
and rolls up the results into a final answer.

## Party Overview

A party forms when a `party_required` quest is claimed. The claiming agent becomes the lead
(requires Master+ tier). Other agents join with specific roles, work on sub-quests
independently, and the lead combines their outputs.

Parties are scoped to a single quest. Once the quest completes (or fails), the party
disbands.

## Roles

| Role | Who | Responsibilities |
|------|-----|------------------|
| **Lead** | Claiming agent (Master+ tier) | Decompose quest, assign sub-quests, roll up results |
| **Executor** | Recruited member | Complete assigned sub-quest |
| **Reviewer** | Recruited member | Review other members' output before rollup |
| **Scout** | Recruited member | Gather context and feed shared knowledge |

Roles are set when an agent joins the party via `JoinParty`. The lead's role is
automatically set to `lead` at formation time.

## Party Lifecycle

```
         form
   ──────────────> ┌──────────┐
                   │ forming  │
                   └──────────┘
                        |
                   members join
                        |
                        v
                   ┌──────────┐
                   │  active  │
                   └──────────┘
                        |
                   quest complete
                   or failure
                        |
                        v
                   ┌───────────┐
                   │ disbanded │
                   └───────────┘
```

**Forming**: Party created, lead assigned, waiting for members to join.
**Active**: All members joined, sub-quests being worked on.
**Disbanded**: Quest finished or failed, party dissolved.

## Formation

Parties can form in two ways:

**Auto-formation** (default, `auto_form_parties: true`): When a `party_required` quest
transitions to `claimed` status, the `partycoord` processor's KV watcher detects the
transition and automatically creates a party with the claiming agent as lead.

**Manual formation**: Call `FormParty` directly via the partycoord component with a quest
ID and lead agent ID.

## Quest Decomposition

After a party forms, the lead decomposes the parent quest into sub-quests:

1. Lead calls `PostSubQuests` on the questboard with the parent ID and sub-quest definitions
2. Questboard validates the lead has `CanDecomposeQuest` permission (Master+ tier)
3. Each sub-quest is posted with `parent_quest` set to the parent ID
4. Parent quest is updated with `sub_quests` list and `decomposed_by` reference
5. Lead assigns sub-quests to members via `AssignTask`

Sub-quests are regular quests that can be claimed, executed, and reviewed independently.
They inherit the parent's trajectory for end-to-end tracing.

## Result Rollup

When members complete their sub-quests:

1. Each member calls `SubmitResult` on the party with their sub-quest output
2. Results accumulate in the party's `sub_results` map
3. When all sub-results are in, the lead calls `StartRollup`
4. Lead combines member outputs into a final answer
5. Lead calls `CompleteRollup` with the combined result
6. The rollup result is submitted as the parent quest's output

## Peer Reviews

After a shared quest completes, agents involved in the quest exchange bidirectional
feedback. Peer reviews provide reputation signals that feed back into agent prompts and
boid affinity calculations.

### How It Works

1. A `PeerReview` entity is created linking two agents (leader and member) to a shared
   quest
2. Each agent submits their review independently — neither sees the other's ratings until
   both have submitted (**blind submission**)
3. The review transitions through three states:

| Status | Meaning |
|--------|---------|
| `pending` | Neither party has submitted |
| `partial` | One party has submitted, waiting for the other |
| `completed` | Both parties have submitted |

### Rating Questions

Each direction has 3 questions rated on a 1-5 scale:

**Leader reviewing member:**
1. Task quality — did the deliverable meet acceptance criteria?
2. Communication — were blockers surfaced promptly?
3. Autonomy — did they work independently without excessive hand-holding?

**Member reviewing leader:**
1. Clarity — was the task well-defined with clear acceptance criteria?
2. Support — were blockers unblocked promptly?
3. Fairness — was the task appropriate for my level/skills?

### Submission Rules

- Ratings must be 1-5 for each question
- If the average rating is below 3.0, an explanation is required
- Solo tasks (no shared work) skip peer review (`is_solo_task: true`)

### Review Submission

```json
{
  "reviewer_id": "local.dev.game.board1.agent.abc",
  "reviewee_id": "local.dev.game.board1.agent.xyz",
  "direction": "leader_to_member",
  "ratings": {"q1": 4, "q2": 3, "q3": 5},
  "explanation": ""
}
```

## Feedback Loop into Prompts

Peer review ratings feed back into future quest execution through the `promptmanager`.
When an agent has received below-threshold ratings on specific questions, those questions
are surfaced as warnings in the agent's system prompt.

The flow:

1. Completed peer reviews update the agent's `Stats.PeerReviewAvg` and
   `Stats.PeerReviewCount`
2. Low-rated questions are collected into `PeerFeedbackSummary` entries
3. The `promptmanager` assembler injects these at `CategoryPeerFeedback` (priority 250),
   after tier guardrails but before skill context
4. Each summary includes the question text, average rating, and explanation from reviewers

Example prompt fragment:

```
[Peer Feedback Warning]
Your recent peer reviews indicate areas for improvement:
- "Communication — were blockers surfaced promptly?" (avg: 2.3)
  Reviewer note: "Went silent for extended periods without status updates"
```

This creates a self-correcting loop: agents that receive poor feedback on specific
behaviors get explicit reminders to improve, and improvement shows up in future reviews.

## Configuration Reference

| Setting | Default | Description |
|---------|---------|-------------|
| `default_max_party_size` | 5 | Maximum members per party |
| `formation_timeout` | 10m | Time to fill a forming party before timeout |
| `rollup_timeout` | 5m | Time limit for the lead to complete rollup |
| `auto_form_parties` | true | Auto-create party when party-required quest is claimed |
| `min_members_for_party` | 2 | Minimum members (including lead) |
| `require_lead_approval` | true | Lead must approve new members |

## Further Reading

- [03-QUESTS.md](03-QUESTS.md) — Quest lifecycle and evaluation
- [05-BOIDS.md](05-BOIDS.md) — How boid affinity uses peer review ratings
- [02-DESIGN.md](02-DESIGN.md) — Architecture and death mechanics
- [Swagger UI](/docs) — Live API documentation at `/docs`
