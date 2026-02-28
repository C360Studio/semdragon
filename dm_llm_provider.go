package semdragons

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/model"
	agenticmodel "github.com/c360studio/semstreams/processor/agentic-model"
)

// =============================================================================
// DEFAULT LLM PROVIDER - Real LLM-based DM decision making
// =============================================================================
// Implements the LLMProvider interface using actual LLM calls via semstreams.
// This powers automated DM modes (FullAuto, Assisted).
// =============================================================================

// DefaultLLMProvider implements LLMProvider using the semstreams model infrastructure.
type DefaultLLMProvider struct {
	registry model.RegistryReader
}

// NewDefaultLLMProvider creates a new LLM provider using the given model registry.
func NewDefaultLLMProvider(registry model.RegistryReader) *DefaultLLMProvider {
	return &DefaultLLMProvider{registry: registry}
}

// DecideQuestParameters uses LLM to determine quest configuration from an objective.
func (p *DefaultLLMProvider) DecideQuestParameters(ctx context.Context, objective string, hints QuestHints) (*QuestDecision, error) {
	client, endpoint, err := p.getClient("quest-design")
	if err != nil {
		return nil, err
	}
	defer client.Close()

	systemPrompt := `You are a Dungeon Master designing quests for an agentic workflow system.
Given a work objective, decide the appropriate quest parameters.

You MUST respond with valid JSON in this exact format:
{
  "difficulty": 0-5 (0=trivial, 1=easy, 2=moderate, 3=hard, 4=epic, 5=legendary),
  "required_skills": ["skill_tag1", "skill_tag2"],
  "base_xp": integer,
  "review_level": 0-3 (0=auto, 1=standard, 2=strict, 3=human),
  "party_required": boolean,
  "min_party_size": integer (if party required),
  "reasoning": "brief explanation"
}

Valid skill tags: code_generation, code_review, data_transformation, summarization, research, planning, customer_communications, analysis, training

Base XP guidelines: trivial=25, easy=50, moderate=100, hard=250, epic=500, legendary=1000`

	userPrompt := fmt.Sprintf("Objective: %s", objective)
	if hints.SuggestedDifficulty != nil {
		userPrompt += fmt.Sprintf("\nSuggested difficulty: %d", *hints.SuggestedDifficulty)
	}
	if len(hints.SuggestedSkills) > 0 {
		skills := make([]string, len(hints.SuggestedSkills))
		for i, s := range hints.SuggestedSkills {
			skills[i] = string(s)
		}
		userPrompt += fmt.Sprintf("\nSuggested skills: %s", strings.Join(skills, ", "))
	}
	if hints.RequireHumanReview {
		userPrompt += "\nNote: Human review is required for this quest."
	}
	if hints.Budget > 0 {
		userPrompt += fmt.Sprintf("\nBudget constraint: $%.2f", hints.Budget)
	}

	resp, err := client.ChatCompletion(ctx, agentic.AgentRequest{
		RequestID: fmt.Sprintf("quest-design-%s", objective[:min(20, len(objective))]),
		Role:      agentic.RoleGeneral,
		Messages: []agentic.ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Model:       endpoint.Model,
		MaxTokens:   1024,
		Temperature: 0.3,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM request failed: %w", err)
	}

	if resp.Status == agentic.StatusError {
		return nil, fmt.Errorf("LLM error: %s", resp.Error)
	}

	return p.parseQuestDecision(resp.Message.Content, hints)
}

// ReviewDecomposition evaluates a party lead's breakdown of a quest.
func (p *DefaultLLMProvider) ReviewDecomposition(ctx context.Context, parent Quest, subQuests []Quest) (*DecompositionReview, error) {
	client, endpoint, err := p.getClient("quest-design")
	if err != nil {
		return nil, err
	}
	defer client.Close()

	systemPrompt := `You are a Dungeon Master reviewing quest decomposition.
Evaluate whether the sub-quests properly cover the parent quest's requirements.

You MUST respond with valid JSON in this exact format:
{
  "approved": boolean,
  "feedback": "explanation of your decision",
  "suggestion": "optional improvement suggestion"
}`

	// Build sub-quest summary
	var subQuestSummary strings.Builder
	for i, sq := range subQuests {
		subQuestSummary.WriteString(fmt.Sprintf("%d. %s: %s\n", i+1, sq.Title, sq.Description))
	}

	userPrompt := fmt.Sprintf(`Parent Quest: %s
Description: %s
Required Skills: %v

Proposed Sub-Quests:
%s

Does this decomposition adequately cover the parent quest?`, parent.Title, parent.Description, parent.RequiredSkills, subQuestSummary.String())

	resp, err := client.ChatCompletion(ctx, agentic.AgentRequest{
		RequestID: fmt.Sprintf("decomp-review-%s", parent.ID),
		Role:      agentic.RoleGeneral,
		Messages: []agentic.ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Model:       endpoint.Model,
		MaxTokens:   1024,
		Temperature: 0.2,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM request failed: %w", err)
	}

	if resp.Status == agentic.StatusError {
		return nil, fmt.Errorf("LLM error: %s", resp.Error)
	}

	return p.parseDecompositionReview(resp.Message.Content, subQuests)
}

// EvaluateAgent assesses an agent's current performance and trajectory.
func (p *DefaultLLMProvider) EvaluateAgent(ctx context.Context, agent Agent, recentQuests []Quest) (*AgentEvaluation, error) {
	client, endpoint, err := p.getClient("agent-eval")
	if err != nil {
		return nil, err
	}
	defer client.Close()

	systemPrompt := `You are a Dungeon Master evaluating an agent's performance.
Based on their stats and recent quests, assess their trajectory.

You MUST respond with valid JSON in this exact format:
{
  "recommended_level": integer (1-20),
  "strengths": ["strength1", "strength2"],
  "weaknesses": ["weakness1", "weakness2"],
  "recommendation": "promote" | "maintain" | "demote" | "retire"
}`

	// Build quest history summary
	var questHistory strings.Builder
	for i, q := range recentQuests {
		questHistory.WriteString(fmt.Sprintf("%d. %s (Difficulty: %d, Status: %s)\n", i+1, q.Title, q.Difficulty, q.Status))
	}

	userPrompt := fmt.Sprintf(`Agent: %s (Level %d, Tier %d)
Skills: %v

Stats:
- Quests Completed: %d
- Quests Failed: %d
- Bosses Defeated: %d
- Bosses Failed: %d
- Avg Quality Score: %.2f
- Death Count: %d

Recent Quests:
%s

Evaluate this agent's performance and trajectory.`, agent.Name, agent.Level, agent.Tier, agent.GetSkillTags(), agent.Stats.QuestsCompleted, agent.Stats.QuestsFailed, agent.Stats.BossesDefeated, agent.Stats.BossesFailed, agent.Stats.AvgQualityScore, agent.DeathCount, questHistory.String())

	resp, err := client.ChatCompletion(ctx, agentic.AgentRequest{
		RequestID: fmt.Sprintf("agent-eval-%s", agent.ID),
		Role:      agentic.RoleGeneral,
		Messages: []agentic.ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Model:       endpoint.Model,
		MaxTokens:   1024,
		Temperature: 0.2,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM request failed: %w", err)
	}

	if resp.Status == agentic.StatusError {
		return nil, fmt.Errorf("LLM error: %s", resp.Error)
	}

	return p.parseAgentEvaluation(resp.Message.Content, agent)
}

// SuggestIntervention recommends how to help a struggling quest.
func (p *DefaultLLMProvider) SuggestIntervention(ctx context.Context, quest Quest, ictx InterventionContext) (*Intervention, error) {
	client, endpoint, err := p.getClient("quest-design")
	if err != nil {
		return nil, err
	}
	defer client.Close()

	systemPrompt := `You are a Dungeon Master deciding how to intervene in a struggling quest.

You MUST respond with valid JSON in this exact format:
{
  "type": "assist" | "redirect" | "takeover" | "abort" | "augment",
  "reason": "explanation",
  "payload": {} (optional additional data)
}

Intervention types:
- assist: Give the agent a hint
- redirect: Change the approach
- takeover: DM completes the quest
- abort: Cancel the quest
- augment: Add resources or tools`

	userPrompt := fmt.Sprintf(`Quest: %s
Description: %s
Duration so far: %v
Attempts: %d
Last Error: %s

What intervention would you recommend?`, quest.Title, quest.Description, ictx.Duration, ictx.Attempts, ictx.LastError)

	resp, err := client.ChatCompletion(ctx, agentic.AgentRequest{
		RequestID: fmt.Sprintf("intervention-%s", quest.ID),
		Role:      agentic.RoleGeneral,
		Messages: []agentic.ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Model:       endpoint.Model,
		MaxTokens:   512,
		Temperature: 0.3,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM request failed: %w", err)
	}

	if resp.Status == agentic.StatusError {
		return nil, fmt.Errorf("LLM error: %s", resp.Error)
	}

	return p.parseIntervention(resp.Message.Content)
}

// HandleEscalation decides how to resolve an escalated quest.
func (p *DefaultLLMProvider) HandleEscalation(ctx context.Context, quest Quest, attempts []EscalationAttempt) (*EscalationDecision, error) {
	client, endpoint, err := p.getClient("quest-design")
	if err != nil {
		return nil, err
	}
	defer client.Close()

	systemPrompt := `You are a Dungeon Master handling an escalated quest (TPK scenario).
The quest has failed multiple times and needs your direct attention.

You MUST respond with valid JSON in this exact format:
{
  "resolution": "reassign" | "decompose" | "complete_by_dm" | "cancel",
  "reasoning": "explanation of your decision"
}`

	// Build attempts history
	var attemptHistory strings.Builder
	for i, a := range attempts {
		attemptHistory.WriteString(fmt.Sprintf("%d. %s intervention (%s): %s\n", i+1, a.Intervention.Type, a.Timestamp.Format("2006-01-02 15:04"), a.Outcome))
	}

	userPrompt := fmt.Sprintf(`Escalated Quest: %s
Description: %s
Difficulty: %d
Total Attempts: %d

Previous Escalation Attempts:
%s

How should this escalation be resolved?`, quest.Title, quest.Description, quest.Difficulty, quest.Attempts, attemptHistory.String())

	resp, err := client.ChatCompletion(ctx, agentic.AgentRequest{
		RequestID: fmt.Sprintf("escalation-%s", quest.ID),
		Role:      agentic.RoleGeneral,
		Messages: []agentic.ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Model:       endpoint.Model,
		MaxTokens:   512,
		Temperature: 0.2,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM request failed: %w", err)
	}

	if resp.Status == agentic.StatusError {
		return nil, fmt.Errorf("LLM error: %s", resp.Error)
	}

	return p.parseEscalationDecision(resp.Message.Content)
}

// --- Helper methods ---

func (p *DefaultLLMProvider) getClient(capability string) (*agenticmodel.Client, *model.EndpointConfig, error) {
	endpointName := p.registry.Resolve(capability)
	endpoint := p.registry.GetEndpoint(endpointName)
	if endpoint == nil {
		endpoint = p.registry.GetEndpoint(p.registry.GetDefault())
	}
	if endpoint == nil {
		return nil, nil, fmt.Errorf("no endpoint available for capability: %s", capability)
	}

	client, err := agenticmodel.NewClient(endpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("create client: %w", err)
	}

	return client, endpoint, nil
}

func (p *DefaultLLMProvider) parseQuestDecision(content string, hints QuestHints) (*QuestDecision, error) {
	// Extract JSON from response (might be wrapped in markdown code blocks)
	jsonStr := ExtractJSONFromLLMResponse(content)

	var raw struct {
		Difficulty     int      `json:"difficulty"`
		RequiredSkills []string `json:"required_skills"`
		BaseXP         int64    `json:"base_xp"`
		ReviewLevel    int      `json:"review_level"`
		PartyRequired  bool     `json:"party_required"`
		MinPartySize   int      `json:"min_party_size"`
		Reasoning      string   `json:"reasoning"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		LogLLMParseFailure("quest_decision", err, content, jsonStr)
		return p.defaultQuestDecision(hints), nil
	}

	// Validate and clamp difficulty (0-5)
	difficulty := raw.Difficulty
	if difficulty < 0 {
		difficulty = 0
	}
	if difficulty > 5 {
		difficulty = 5
	}

	// Validate and clamp review level (0-3)
	reviewLevel := raw.ReviewLevel
	if reviewLevel < 0 {
		reviewLevel = 0
	}
	if reviewLevel > 3 {
		reviewLevel = 3
	}

	// Validate base XP (must be positive)
	baseXP := raw.BaseXP
	if baseXP <= 0 {
		baseXP = DefaultXPForDifficulty(QuestDifficulty(difficulty))
	}

	// Validate min party size
	minPartySize := raw.MinPartySize
	if minPartySize < 0 {
		minPartySize = 0
	}

	// Convert skill strings to SkillTags
	skills := make([]SkillTag, len(raw.RequiredSkills))
	for i, s := range raw.RequiredSkills {
		skills[i] = SkillTag(s)
	}

	return &QuestDecision{
		Difficulty:     QuestDifficulty(difficulty),
		RequiredSkills: skills,
		BaseXP:         baseXP,
		ReviewLevel:    ReviewLevel(reviewLevel),
		PartyRequired:  raw.PartyRequired,
		MinPartySize:   minPartySize,
		Reasoning:      raw.Reasoning,
		GuildPriority:  hints.PreferGuild,
	}, nil
}

func (p *DefaultLLMProvider) defaultQuestDecision(hints QuestHints) *QuestDecision {
	decision := &QuestDecision{
		Difficulty:  DifficultyModerate,
		BaseXP:      100,
		ReviewLevel: ReviewStandard,
		Reasoning:   "default decision (LLM parsing failed)",
	}
	if hints.SuggestedDifficulty != nil {
		decision.Difficulty = *hints.SuggestedDifficulty
		decision.BaseXP = DefaultXPForDifficulty(*hints.SuggestedDifficulty)
	}
	if len(hints.SuggestedSkills) > 0 {
		decision.RequiredSkills = hints.SuggestedSkills
	}
	if hints.RequireHumanReview {
		decision.ReviewLevel = ReviewHuman
	}
	if hints.PreferGuild != nil {
		decision.GuildPriority = hints.PreferGuild
	}
	return decision
}

func (p *DefaultLLMProvider) parseDecompositionReview(content string, subQuests []Quest) (*DecompositionReview, error) {
	jsonStr := ExtractJSONFromLLMResponse(content)

	var raw struct {
		Approved   bool   `json:"approved"`
		Feedback   string `json:"feedback"`
		Suggestion string `json:"suggestion"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		LogLLMParseFailure("decomposition_review", err, content, jsonStr)
		// Default to approval if parsing fails
		return &DecompositionReview{
			Approved:  true,
			SubQuests: subQuests,
			Feedback:  "approved (LLM parsing failed)",
		}, nil
	}

	return &DecompositionReview{
		Approved:   raw.Approved,
		SubQuests:  subQuests,
		Feedback:   raw.Feedback,
		Suggestion: raw.Suggestion,
	}, nil
}

func (p *DefaultLLMProvider) parseAgentEvaluation(content string, agent Agent) (*AgentEvaluation, error) {
	jsonStr := ExtractJSONFromLLMResponse(content)

	var raw struct {
		RecommendedLevel int      `json:"recommended_level"`
		Strengths        []string `json:"strengths"`
		Weaknesses       []string `json:"weaknesses"`
		Recommendation   string   `json:"recommendation"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		LogLLMParseFailure("agent_evaluation", err, content, jsonStr)
		// Default to maintain current level
		return &AgentEvaluation{
			AgentID:          agent.ID,
			CurrentLevel:     agent.Level,
			RecommendedLevel: agent.Level,
			Recommendation:   "maintain",
		}, nil
	}

	// Validate and clamp recommended level (1-20)
	recommendedLevel := raw.RecommendedLevel
	if recommendedLevel < 1 {
		recommendedLevel = 1
	}
	if recommendedLevel > 20 {
		recommendedLevel = 20
	}

	// Validate recommendation value
	validRecommendations := map[string]bool{
		"promote": true, "maintain": true, "demote": true, "retire": true,
	}
	recommendation := raw.Recommendation
	if !validRecommendations[recommendation] {
		recommendation = "maintain"
	}

	return &AgentEvaluation{
		AgentID:          agent.ID,
		CurrentLevel:     agent.Level,
		RecommendedLevel: recommendedLevel,
		Strengths:        raw.Strengths,
		Weaknesses:       raw.Weaknesses,
		Recommendation:   recommendation,
	}, nil
}

func (p *DefaultLLMProvider) parseIntervention(content string) (*Intervention, error) {
	jsonStr := ExtractJSONFromLLMResponse(content)

	var raw struct {
		Type    string `json:"type"`
		Reason  string `json:"reason"`
		Payload any    `json:"payload"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		LogLLMParseFailure("intervention", err, content, jsonStr)
		// Default to assist
		return &Intervention{
			Type:   InterventionAssist,
			Reason: "default intervention (LLM parsing failed)",
		}, nil
	}

	// Map string to InterventionType
	typeMap := map[string]InterventionType{
		"assist":   InterventionAssist,
		"redirect": InterventionRedirect,
		"takeover": InterventionTakeover,
		"abort":    InterventionAbort,
		"augment":  InterventionAugment,
	}

	iType, ok := typeMap[raw.Type]
	if !ok {
		iType = InterventionAssist
	}

	return &Intervention{
		Type:    iType,
		Reason:  raw.Reason,
		Payload: raw.Payload,
	}, nil
}

func (p *DefaultLLMProvider) parseEscalationDecision(content string) (*EscalationDecision, error) {
	jsonStr := ExtractJSONFromLLMResponse(content)

	var raw struct {
		Resolution string `json:"resolution"`
		Reasoning  string `json:"reasoning"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		LogLLMParseFailure("escalation_decision", err, content, jsonStr)
		// Default to reassign
		return &EscalationDecision{
			Resolution: "reassign",
			Reasoning:  "default resolution (LLM parsing failed)",
		}, nil
	}

	// Validate resolution value
	validResolutions := map[string]bool{
		"reassign": true, "decompose": true, "complete_by_dm": true, "cancel": true,
	}
	resolution := raw.Resolution
	if !validResolutions[resolution] {
		resolution = "reassign"
	}

	return &EscalationDecision{
		Resolution: resolution,
		Reasoning:  raw.Reasoning,
	}, nil
}

