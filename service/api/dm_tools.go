package api

// dm_tools.go wires a DM-specific tool registry so the DM chat handler can
// invoke tools (graph_query, web_search, file reads, etc.) during conversation.
//
// Design choices:
//   - Tools are registered once at service Start and reused across requests.
//     ToolRegistry is safe for concurrent reads after initialization.
//   - The synthetic DM agent carries TierGrandmaster and all known skills so
//     every tool's tier/skill gate passes without special-casing the DM path.
//   - Search and graph_search are registered only when the deps are present,
//     so the registry gracefully degrades if those services are not configured.
//   - initDMTools is best-effort: if it fails the handler falls back to the
//     existing callLLM path (s.dmTools remains nil).

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/c360studio/semdragons/domain"
	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/executor"
	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/model"
	agenticmodel "github.com/c360studio/semstreams/processor/agentic-model"
)

// Tool-loop limits for the DM agentic path.
const (
	// maxDMToolIterations caps the number of tool-call rounds per DM turn.
	maxDMToolIterations = 10
	// maxDMToolResultLen caps the characters returned from a single tool call
	// before the result is truncated, preventing runaway context growth.
	maxDMToolResultLen = 4000
	// dmToolLoopTimeout is the wall-clock budget for the entire tool loop.
	dmToolLoopTimeout = 5 * time.Minute
)

// dmToolAllowlist is the set of tool names the DM is permitted to use.
// bash is excluded — DM chat runs without a sandbox directory, so shell
// commands would fail. DM uses graph/web tools for information gathering.
var dmToolAllowlist = map[string]bool{
	"graph_query":  true,
	"graph_search": true,
	"web_search":   true,
	"http_request": true,
}

// allKnownSkills is the complete set of domain skills.
// The synthetic DM agent holds all of them so no tool's skill gate fires.
var allKnownSkills = []domain.SkillTag{
	domain.SkillCodeGen,
	domain.SkillCodeReview,
	domain.SkillDataTransform,
	domain.SkillSummarization,
	domain.SkillResearch,
	domain.SkillPlanning,
	domain.SkillCustomerComms,
	domain.SkillAnalysis,
	domain.SkillTraining,
}

// initDMTools builds the DM-specific tool registry.
// It is called once during service Start. Errors are non-fatal: if the
// registry cannot be built (e.g. missing graph client) the method logs a
// warning and returns without setting s.dmTools.
func (s *Service) initDMTools() {
	reg := executor.NewToolRegistry()

	// Register all built-in file/search tools (read, list, glob, search, http, …).
	// Write and execute tools are built-in but filtered out by dmToolAllowlist
	// in dmToolDefs, so they are never sent to the LLM.
	reg.RegisterBuiltins()

	// graph_query — backed by the board KV bucket via GraphClient.
	if s.graph != nil {
		reg.RegisterGraphQuery(s.buildDMGraphQueryFunc())
	} else {
		s.logger.Warn("DM tools: graph client unavailable, graph_query disabled")
	}

	// web_search — pull config from questtools component; same provider the
	// agent tools use, so the DM and agents share the same search backend.
	if sp := s.resolveDMSearchProvider(); sp != nil {
		reg.RegisterWebSearch(sp)
		s.logger.Info("DM tools: web_search registered", "provider", sp.Name())
	}

	// graph_search — pull graphql_url from questtools component config.
	if graphqlURL := s.resolveDMGraphQLURL(); graphqlURL != "" {
		reg.RegisterGraphSearch(graphqlURL)
		s.logger.Info("DM tools: graph_search registered", "graphql_url", graphqlURL)
	}

	s.dmTools = reg
	s.logger.Info("DM tool registry initialized", "tools", len(reg.ListAll()))
}

// buildDMGraphQueryFunc returns an EntityQueryFunc backed by the service's
// GraphClient. The closure captures s.graph by value at init time.
func (s *Service) buildDMGraphQueryFunc() executor.EntityQueryFunc {
	graph := s.graph
	maxEnt := s.config.MaxEntities
	if maxEnt <= 0 {
		maxEnt = 100
	}
	return func(ctx context.Context, entityType string, limit int) (string, error) {
		if limit <= 0 || limit > maxEnt {
			limit = maxEnt
		}
		entities, err := graph.ListEntitiesByType(ctx, entityType, limit)
		if err != nil {
			return "", fmt.Errorf("list %s entities: %w", entityType, err)
		}
		return executor.FormatEntitySummary(entities, entityType), nil
	}
}

// resolveDMSearchProvider attempts to construct a SearchProvider from the
// questtools component configuration. Returns nil when search is not configured.
func (s *Service) resolveDMSearchProvider() executor.SearchProvider {
	cfg := s.getComponentConfig(questtoolsComponentName)
	if cfg == nil {
		return nil
	}

	var raw struct {
		Search *executor.SearchConfig `json:"search"`
	}
	if err := json.Unmarshal(cfg.Config, &raw); err != nil || raw.Search == nil {
		return nil
	}
	if raw.Search.Provider == "" {
		return nil
	}

	sp, err := executor.NewSearchProvider(*raw.Search)
	if err != nil {
		s.logger.Warn("DM tools: web_search disabled", "reason", err.Error())
		return nil
	}
	return sp
}

// resolveDMGraphQLURL reads the graph-gateway GraphQL endpoint from the
// questtools component configuration. Returns empty string when not set.
func (s *Service) resolveDMGraphQLURL() string {
	cfg := s.getComponentConfig(questtoolsComponentName)
	if cfg == nil {
		return ""
	}

	var raw struct {
		GraphQLURL string `json:"graphql_url"`
	}
	if err := json.Unmarshal(cfg.Config, &raw); err != nil {
		return ""
	}
	return raw.GraphQLURL
}

// dmToolDefs returns the agentic tool definitions the DM is allowed to use.
// It filters the full registry to the allowlist so write/execute tools are
// never presented to the LLM even though they are registered in the registry.
func (s *Service) dmToolDefs() []agentic.ToolDefinition {
	if s.dmTools == nil {
		return nil
	}

	all := s.dmTools.ListAll()
	defs := make([]agentic.ToolDefinition, 0, len(all))
	for _, t := range all {
		if dmToolAllowlist[t.Definition.Name] {
			defs = append(defs, t.Definition)
		}
	}
	return defs
}

// executeDMTool dispatches a single tool call on behalf of the DM.
// It creates a synthetic Grandmaster agent with all skills so tier/skill gates
// always pass, and a minimal quest stub with no AllowedTools restriction.
//
// Results longer than maxDMToolResultLen are truncated to keep the LLM context
// manageable across multi-turn tool loops.
func (s *Service) executeDMTool(ctx context.Context, call agentic.ToolCall) (string, error) {
	if s.dmTools == nil {
		return "", fmt.Errorf("DM tool registry not initialized")
	}

	// Synthetic Grandmaster agent: bypasses all tier and skill gates.
	dmAgent := &agentprogression.Agent{
		ID:    "dm",
		Name:  "Dungeon Master",
		Level: 20,
		Tier:  domain.TierGrandmaster,
		SkillProficiencies: buildDMSkillProficiencies(),
	}

	// Minimal quest stub — no AllowedTools restriction, so all allowlisted
	// tools are accessible. SandboxDir is injected via the registry directly.
	dmQuest := &domain.Quest{
		ID:    "dm-chat",
		Title: "DM chat",
	}

	result := s.dmTools.Execute(ctx, call, dmQuest, dmAgent)
	if result.Error != "" {
		return "", fmt.Errorf("tool %s: %s", call.Name, result.Error)
	}

	content := result.Content
	if len(content) > maxDMToolResultLen {
		content = content[:maxDMToolResultLen] + "\n... (truncated)"
	}
	return content, nil
}

// buildDMSkillProficiencies returns a proficiency map containing every known
// skill at novice level (1). This satisfies agentHasAnySkill for all tools.
func buildDMSkillProficiencies() map[domain.SkillTag]domain.SkillProficiency {
	profs := make(map[domain.SkillTag]domain.SkillProficiency, len(allKnownSkills))
	for _, skill := range allKnownSkills {
		profs[skill] = domain.SkillProficiency{Level: domain.ProficiencyNovice}
	}
	return profs
}

// newDMClient creates an agenticmodel.Client for a given endpoint.
// The client is created per-request since the endpoint can change based on
// capability resolution (dm-chat vs quest-design resolve to different models).
func (s *Service) newDMClient(endpoint *model.EndpointConfig) (*agenticmodel.Client, error) {
	client, err := agenticmodel.NewClient(endpoint)
	if err != nil {
		return nil, fmt.Errorf("create DM LLM client: %w", err)
	}
	client.SetAdapter(agenticmodel.AdapterFor(endpoint.Provider))
	client.SetLogger(s.logger)
	return client, nil
}

// dmConvertMessages converts the DM chat system prompt and conversation history
// into the agentic.ChatMessage format expected by agenticmodel.Client.
func dmConvertMessages(systemPrompt string, messages []ChatMessage) []agentic.ChatMessage {
	out := make([]agentic.ChatMessage, 0, len(messages)+1)

	// System prompt as first message.
	out = append(out, agentic.ChatMessage{
		Role:    "system",
		Content: systemPrompt,
	})

	// Convert conversation history.
	for _, m := range messages {
		role := m.Role
		if role == "dm" {
			role = "assistant"
		}
		out = append(out, agentic.ChatMessage{
			Role:    role,
			Content: m.Content,
		})
	}

	return out
}
