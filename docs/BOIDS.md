# Boid Engine: Emergent Quest-Claiming

There is no central scheduler. Agents flock toward quests using attraction scores computed
from six rules inspired by Craig Reynolds' boid flocking algorithm. The highest-scoring
agent-quest pairs become claim suggestions.

## Overview

Every `update_interval_ms` (default 1000ms), the `boidengine` processor:

1. Loads all idle agents and posted quests from KV
2. Computes attraction scores for every agent-quest pair
3. Produces suggestions via greedy assignment or ranked top-N
4. Publishes suggestions so agents (or the autonomy loop) can act on them

The engine considers six weighted rules. Each rule contributes a score (positive = pull,
negative = push). The total score determines how strongly an agent is attracted to a quest.

## The Six Rules

### 1. Separation (default weight: 1.0)

**What**: Avoid quests that other agents are already clustering toward.

**How**: Count agents with `current_quest` pointing at each quest (crowding map). For
each crowded quest, apply a penalty:

```
separation = -crowd_count * 0.5 * weight
```

**Effect**: Distributes agents across the board instead of piling onto popular quests.

### 2. Alignment (default weight: 0.8)

**What**: Prefer quests in skill areas where many quests exist (follow the herd).

**How**: Count quests per skill tag (skill cluster density). For each quest skill the
agent has, add a small alignment bonus:

```
alignment = matching_skills * cluster_density * 0.1 * weight
```

**Effect**: Agents drift toward skill areas with high demand, creating natural
specialization pressure.

### 3. Cohesion (default weight: 0.6)

**What**: Move toward quests that match the agent's skill set.

**How**: Compute the ratio of matching skills to required skills:

```
cohesion = (matching_skills / required_skills) * weight
```

**Effect**: Agents with partial skill matches still get pulled toward quests they can
mostly handle, but full matches score higher.

### 4. Hunger (default weight: 1.2)

**What**: Idle agents become more urgent about claiming work.

**How**: Base hunger score applied to all idle agents:

```
hunger = 0.5 * weight
```

Future improvement: scale by actual idle time so long-idle agents are more aggressive.

**Effect**: Prevents agents from staying idle when work is available.

### 5. Affinity (default weight: 1.5)

**What**: Strong pull toward quests matching skills and guild. This is the strongest
default rule.

**How**: Skill match count plus guild match bonus, multiplied by weight:

```
skill_match = count of matching skills
guild_match = 0.0 (no guild priority on quest)
            = 1.0 + rank_bonus + reputation_multiplier (guild match)

affinity = (skill_match + guild_match) * weight
```

Guild integration (see next section) adds rank and reputation bonuses on top of the base
guild match.

**Peer review modifier**: Agents with peer review history get a reputation adjustment:

```
reputation_mod = (peer_review_avg - 3.0) / 2.0    // -1.0 to +1.0
affinity *= (1.0 + reputation_mod * 0.3)           // +/-30%
```

An agent rated 5.0 by peers gets a 30% affinity boost. An agent rated 1.0 gets a 30%
penalty.

**Effect**: The primary matching signal. Skilled agents with good reputations in relevant
guilds are strongly attracted to matching quests.

### 6. Caution (default weight: 0.9)

**What**: Avoid quests above the agent's tier.

**How**: Compare quest `min_tier` to agent tier:

```
if quest_tier > agent_tier:
    caution = -tier_difference * weight     // Penalty for over-leveled quests
else:
    caution = 0.2 * weight                  // Small bonus for at/below level
```

**Effect**: Apprentice agents avoid Epic quests. Experts get a small bonus for
appropriate-difficulty quests.

## Guild and Reputation Integration

When a quest has `guild_priority` set, agents in that guild get an affinity boost. The
boost compounds based on two factors:

**Rank bonus**: Higher-ranked guild members have stronger affinity. `GuildBonusRate`
ranges from 0.10 (initiate) to 0.25 (guildmaster), multiplied by 5.0 to produce a
0.5-1.25 rank bonus.

| Rank | Bonus Rate | Rank Bonus |
|------|------------|------------|
| Initiate | 0.10 | 0.50 |
| Member | 0.15 | 0.75 |
| Veteran | 0.18 | 0.90 |
| Officer | 0.20 | 1.00 |
| Guildmaster | 0.25 | 1.25 |

**Reputation multiplier**: The guild's `Reputation` (0.0-1.0) provides up to a 1.5x
multiplier on the total guild match:

```
guild_match = (1.0 + rank_bonus) * (1.0 + reputation * 0.5)
```

A guildmaster (rank bonus 1.25) in a guild with 0.8 reputation:

```
guild_match = (1.0 + 1.25) * (1.0 + 0.8 * 0.5) = 2.25 * 1.4 = 3.15
```

## Peer Review Feedback

The boid engine uses `Agent.Stats.PeerReviewAvg` (1-5 scale) to adjust affinity scores.
This creates a virtuous cycle:

1. Agent does good work on quests
2. Peers rate them highly
3. Higher peer review average boosts affinity
4. Agent gets stronger suggestions for matching quests
5. More matching quests leads to better performance

Conversely, poorly-rated agents get weaker suggestions, steering them toward less
demanding quests where they can rebuild reputation.

## Suggestion Modes

### SuggestClaims (greedy assignment)

Takes the sorted attraction list and greedily assigns one quest per agent:

1. Take the highest-scoring agent-quest pair
2. Remove both agent and quest from the pool
3. Repeat until no pairs remain

Each suggestion includes a confidence score based on the margin between the best and
second-best quest for that agent.

Use when you want at most one claim per agent with no quest conflicts.

### SuggestTopN (ranked suggestions)

Returns up to `max_suggestions_per_agent` ranked quest suggestions per agent. Unlike
greedy mode, quests are **not** removed from the pool — multiple agents may receive the
same quest as a suggestion.

Confidence is computed as:
- Top suggestion: margin between 1st and 2nd best score
- Lower suggestions: ratio to top score, halved

KV write serialization handles conflicts naturally at claim time (first writer wins).

Use when you want agents to have fallback options.

## Configuration

All weights and timing are configurable in the component config:

| Setting | Default | Description |
|---------|---------|-------------|
| `separation_weight` | 1.0 | Avoid crowded quests |
| `alignment_weight` | 0.8 | Follow popular skill areas |
| `cohesion_weight` | 0.6 | Match skill requirements |
| `hunger_weight` | 1.2 | Idle urgency |
| `affinity_weight` | 1.5 | Skill + guild match (strongest) |
| `caution_weight` | 0.9 | Avoid over-leveled quests |
| `update_interval_ms` | 1000 | Recomputation frequency |
| `neighbor_radius` | 5 | Agents to consider for peer rules |
| `max_suggestions_per_agent` | 3 | Ranked suggestions in TopN mode |

## Tuning Guide

**"Agents keep fighting over the same quest"**
Increase `separation_weight` to penalize crowded quests more heavily, or decrease
`affinity_weight` to reduce the pull of highly-matched quests.

**"Low-tier agents are claiming quests they can't handle"**
Increase `caution_weight`. The tier check in `AvailableQuests` prevents invalid claims,
but higher caution steers suggestions away from borderline quests.

**"Agents ignore guild-priority quests"**
Increase `affinity_weight`. If guild members aren't being suggested guild quests, check
that agents have the guild in their `guilds` list and the quest has `guild_priority` set.

**"New agents sit idle while quests pile up"**
Increase `hunger_weight`. Higher hunger makes idle agents more aggressive about claiming
any available quest, even imperfect matches.

**"Too many agents specializing in the same skill area"**
Decrease `alignment_weight` to reduce herding behavior, and increase `cohesion_weight`
to favor individual skill-match quality over cluster density.

**"Agents with bad reviews keep getting top suggestions"**
The peer review modifier caps at +/-30% of affinity. If stronger differentiation is
needed, increase `affinity_weight` (which amplifies the modifier) or adjust the
reputation formula in `boids.go`.

## Further Reading

- [QUESTS.md](QUESTS.md) — Quest lifecycle and difficulty tiers
- [PARTIES.md](PARTIES.md) — Peer reviews that feed into boid reputation
- [DESIGN.md](DESIGN.md) — Architecture and coordination model
