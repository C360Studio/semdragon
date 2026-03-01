# ADR: Tiered Model Resolution (Tier Sets Floor, Skill Picks Model)

## Status
Accepted

## Context

The model registry currently maps four flat capability slots (`agent-work`,
`boss-battle`, `quest-design`, `agent-eval`) to preferred endpoints. The
executor resolves a model for every agent identically:

```
agent.Config.Provider → registry.Resolve("agent-work") → registry.GetDefault()
```

Skills and trust tiers gate what quests an agent can claim and what tools they
can access, but they have zero influence on which LLM backs the agent. A Level 1
Apprentice running `summarization` uses the same Claude Sonnet as a Level 19
Grandmaster running `code_generation`.

This creates two problems:

1. **Cost inefficiency** - simple tasks (summarization, classification) consume
   frontier-model tokens for no benefit. A roster of 20 agents all hitting
   Claude Sonnet burns budget that could fund 10x more work on cheaper models.

2. **Chicken-and-egg for specialization** - the guild/skill system assumes
   agents specialize, but if every agent runs the same generalist model, there
   is no meaningful differentiation between a code-generation specialist and a
   summarization specialist. Skill proficiency becomes cosmetic rather than
   functional.

The naive fix (map skills directly to models) creates a circular dependency:
model selection depends on skills, skill progression depends on quest
performance, quest performance depends on having the right model.

## Decision

Adopt a hybrid resolution strategy: **tier determines the model budget ceiling,
skill determines model selection within that ceiling**.

### Resolution Chain

The executor resolves models in this order (most specific wins):

```
1. agent.Config.Provider          (explicit per-agent override, unchanged)
2. registry.Resolve(tier + skill) (new: tier-qualified skill capability)
3. registry.Resolve(tier)         (new: tier-level default)
4. registry.Resolve("agent-work") (existing: global default)
5. registry.GetDefault()          (existing: catch-all fallback)
```

### Capability Key Format

Capability keys gain optional tier and skill qualifiers using dot notation:

```
agent-work                              (global default, exists today)
agent-work.apprentice                   (tier default)
agent-work.expert                       (tier default)
agent-work.expert.code_generation       (tier + skill specific)
agent-work.apprentice.summarization     (tier + skill specific)
```

This exploits the existing `map[string]*CapabilityConfig` in the registry.
No structural changes to `model.Registry` or `model.CapabilityConfig` needed -
just richer key conventions.

### Tier-to-Model Class Mapping

| Tier | Levels | Model Class | Example Endpoints |
|------|--------|-------------|-------------------|
| Apprentice | 1-5 | Small/fast | Haiku, llama3.2, gpt-4o-mini |
| Journeyman | 6-10 | Mid-tier | Sonnet, gpt-4o-mini |
| Expert | 11-15 | Full | Sonnet, gpt-4o |
| Master | 16-18 | Frontier | Opus, o1 |
| Grandmaster | 19-20 | Frontier+ | Opus (extended context/budget) |

Agents earn better models by leveling up. The RPG metaphor holds: a Level 1
apprentice wields a wooden sword (Haiku), not Excalibur (Opus).

### Skill Affinity Within Tier

Within a tier's model class, skill determines which endpoint is preferred:

```
agent-work.expert.code_generation  → claude-4     (strong tool use)
agent-work.expert.summarization    → haiku         (fast, cheap - experts
                                                    can use below their
                                                    ceiling when appropriate)
agent-work.expert.code_review      → claude-4     (precise reasoning)
agent-work.expert.analysis         → gpt-4o       (strong at structured data)
```

The key insight: higher-tier agents are not forced onto expensive models for
every task. An Expert doing summarization can use Haiku because the tier sets
a ceiling, not a floor. Cost optimization happens naturally.

### Registry Example (Production)

```go
func ProductionModelRegistry() *model.Registry {
    return &model.Registry{
        Endpoints: map[string]*model.EndpointConfig{
            "haiku":    { Provider: "anthropic", Model: "claude-haiku-4-5-...", ... },
            "sonnet":   { Provider: "anthropic", Model: "claude-sonnet-4-5-...", ... },
            "opus":     { Provider: "anthropic", Model: "claude-opus-4-6", ... },
            "gpt-4o":   { Provider: "openai", Model: "gpt-4o", ... },
            "gpt-mini": { Provider: "openai", Model: "gpt-4o-mini", ... },
            "ollama":   { Provider: "ollama", Model: "llama3.2", ... },
        },
        Capabilities: map[string]*model.CapabilityConfig{
            // Global fallback (unchanged, backwards compatible)
            "agent-work": {
                Description:   "Default agent execution",
                Preferred:     []string{"sonnet", "gpt-4o"},
                RequiresTools: true,
            },

            // Tier defaults
            "agent-work.apprentice": {
                Preferred: []string{"haiku", "gpt-mini"},
                Fallback:  []string{"ollama"},
                RequiresTools: true,
            },
            "agent-work.journeyman": {
                Preferred: []string{"sonnet", "gpt-mini"},
                Fallback:  []string{"haiku"},
                RequiresTools: true,
            },
            "agent-work.expert": {
                Preferred: []string{"sonnet", "gpt-4o"},
                Fallback:  []string{"haiku"},
                RequiresTools: true,
            },
            "agent-work.master": {
                Preferred: []string{"opus", "sonnet"},
                RequiresTools: true,
            },
            "agent-work.grandmaster": {
                Preferred: []string{"opus"},
                RequiresTools: true,
            },

            // Skill-specific overrides (only where meaningful)
            "agent-work.expert.code_generation": {
                Preferred: []string{"sonnet", "gpt-4o"},
                RequiresTools: true,
            },
            "agent-work.expert.summarization": {
                Preferred: []string{"haiku", "gpt-mini"},
            },
            "agent-work.expert.code_review": {
                Preferred: []string{"sonnet"},
            },

            // Boss battle, quest-design, agent-eval unchanged
            "boss-battle":  { Preferred: []string{"sonnet"}, ... },
            "quest-design": { Preferred: []string{"sonnet"}, ... },
            "agent-eval":   { Preferred: []string{"sonnet"}, ... },
        },
    }
}
```

### Executor Changes

The executor builds a capability key from agent state and quest context:

```go
func (e *DefaultExecutor) resolveCapability(
    agent *agentprogression.Agent,
    quest *questboard.Quest,
) string {
    tier := strings.ToLower(agent.Tier.String())

    // Try tier + primary skill first
    if skill := quest.PrimarySkill(); skill != "" {
        key := fmt.Sprintf("agent-work.%s.%s", tier, skill)
        if e.registry.Resolve(key) != e.registry.GetDefault() {
            return key
        }
    }

    // Fall back to tier default
    key := fmt.Sprintf("agent-work.%s", tier)
    if e.registry.Resolve(key) != e.registry.GetDefault() {
        return key
    }

    // Fall back to global
    return "agent-work"
}
```

The `Resolve()` method already returns `Defaults.Model` for unknown capability
keys, so unregistered tier+skill combos gracefully degrade through the chain.

## Consequences

### Positive
- Breaks the chicken-and-egg: agents start on cheap models, earn upgrades
  through demonstrated competence (XP/levels), not configuration
- Cost efficiency follows naturally: simple tasks use cheap models regardless
  of agent tier
- RPG metaphor deepens: model access is equipment progression, not loadout
- Fully backwards compatible: existing flat `agent-work` key still works as
  the final fallback
- No changes to `model.Registry` struct or `RegistryReader` interface needed
- Per-agent `Config.Provider` override still takes priority for pinned agents
- Skill-specific entries are optional: only define them where the default tier
  model is suboptimal for a particular skill

### Negative
- Registry configuration grows: more capability entries to manage, though most
  deployments will define tier defaults and only a few skill overrides
- Capability key convention is implicit: `agent-work.expert.code_generation`
  is a string pattern, not a typed structure. Misconfigured keys fail silently
  to fallback rather than erroring
- Requires `TrustTier.String()` to produce stable lowercase names matching
  registry keys

### Neutral
- Quest needs a `PrimarySkill()` accessor (or executor infers from
  `RequiredSkills[0]`), which formalizes something currently implicit
- The resolution chain adds two registry lookups in the worst case (both miss,
  fall through to `agent-work`), which is negligible given these are in-memory
  map lookups

## Implementation

1. Add `TrustTier.String()` method if not present (returns lowercase tier name)
2. Add `PrimarySkill()` to quest or have executor derive from `RequiredSkills`
3. Update `DefaultExecutor.Execute()` to use `resolveCapability()` chain
4. Expand `ProductionModelRegistry()` with tier-level capability entries
5. Add skill-specific overrides only for known cost/quality sweet spots
6. Update seeding to verify default agent configs work with new resolution
7. Add unit tests for resolution chain fallback behavior

## References

- `config.go` - Current model registry definitions
- `processor/executor/executor.go` - Current resolution logic (line 132-147)
- `domain/types.go` - TrustTier and SkillTag definitions
- `github.com/c360studio/semstreams/model/registry.go` - Registry/Resolve API
