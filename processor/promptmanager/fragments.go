package promptmanager

import (
	"fmt"
	"strings"

	"github.com/c360studio/semdragons/domain"
)

// =============================================================================
// BUILT-IN FRAGMENTS - Cross-domain fragments registered by components
// =============================================================================
// These fragments apply across all domains and are not part of any DomainCatalog.
// Call RegisterBuiltinFragments after RegisterProviderStyles and RegisterDomainCatalog.
// =============================================================================

// RegisterBuiltinFragments registers cross-domain fragments into the registry.
// Currently registers:
//   - Party lead tool directive (CategoryToolDirective) — enforces decompose_quest
//     calls for party lead agents on party-required quests. Includes quest scenarios
//     when present so the lead can map them directly to sub-quests.
//   - Sub-quest executor directive (CategoryToolDirective) — guides party member
//     agents to complete work and submit results via [INTENT: work_product].
//   - Solo agent scenario directive (CategoryToolDirective) — renders quest scenarios
//     as a structured work plan for solo (non-party, non-sub-quest) agents.
//   - Solo agent work output directive (CategoryToolDirective) — reinforces that all
//     solo agents must submit finished work, not descriptions or plans.
//   - Research output directive (CategoryToolDirective) — skill-gated to research/analysis
//     quests, instructs agents to write structured markdown files via write_file.
//   - Gemini/OpenAI tool enforcement hint (CategoryProviderHints) — reinforces
//     immediate tool-call behaviour for providers that may otherwise respond with text.
//
// Call this once per registry, after RegisterDomainCatalog and RegisterProviderStyles.
func RegisterBuiltinFragments(r *PromptRegistry) {
	registerPartyLeadDirective(r)
	registerPartyLeadProviderHints(r)
	registerSubQuestExecutorDirective(r)
	registerSoloAgentScenarioDirective(r)
	registerSoloAgentWorkOutputDirective(r)
	registerResearchOutputDirective(r)
	registerReviewBrief(r)
	registerToolSelectionGuidance(r)
}

// partyLeadDirectiveBase is the tool-call instruction for party leads when no
// quest scenarios are available. API-level tool_choice=function enforces that
// decompose_quest is called; this prompt provides workflow context and guidance.
const partyLeadDirectiveBase = `You are a PARTY LEAD coordinating a team of agents.

YOUR WORKFLOW:
1. DECOMPOSE: Use the decompose_quest tool to break the quest into independent sub-quests.
   - Each sub-quest must be a self-contained unit of work with a clear objective.
   - Do NOT include a "combine" or "synthesize" step — that is YOUR responsibility.
   - Sub-quests should produce independent outputs (code, analysis, etc.).
2. REVIEW: You will review each completed sub-quest via review_sub_quest.
   - Accept work that meets the objective; reject with specific feedback if it does not.
3. SYNTHESIS: After all sub-quests are accepted, YOU synthesize the final deliverable.
   - Combine the sub-quest outputs into a single coherent result.
   - Respond with [INTENT: work_product] followed by the combined deliverable.
   - This is YOUR work product — you are accountable for the final quality.

RULES:
- Do NOT delegate synthesis/combination to a sub-quest — you have all the context from reviews.`

// partyLeadProviderHint reinforces immediate tool-call behaviour for providers
// (Gemini, OpenAI) that may emit a text preamble before calling a tool.
// Belt-and-suspenders with API-level tool_choice — particularly important for
// Gemini where force-specific tool_choice is unreliable (ADR-023 Phase 2).
const partyLeadProviderHint = `When instructed to call a specific tool, call that tool as your first action. Do not provide a text response before calling the tool.`

// isPartyLead is the shared Condition gate for both party lead fragments.
func isPartyLead(ctx AssemblyContext) bool {
	return ctx.PartyRequired && ctx.IsPartyLead
}

// buildPartyLeadDirective constructs the party lead directive, injecting quest
// scenario details when present so the lead can map them to sub-quests directly.
func buildPartyLeadDirective(ctx AssemblyContext) string {
	if len(ctx.QuestScenarios) == 0 {
		return partyLeadDirectiveBase
	}

	var b strings.Builder
	b.WriteString("You are a PARTY LEAD coordinating a team of agents.\n")

	// Inject the structured quest specification when scenarios are available.
	b.WriteString("\nQUEST SPECIFICATION:\n")
	if ctx.QuestGoal != "" {
		b.WriteString(fmt.Sprintf("Goal: %s\n", ctx.QuestGoal))
	}
	if len(ctx.QuestRequirements) > 0 {
		b.WriteString("Requirements:\n")
		for _, req := range ctx.QuestRequirements {
			b.WriteString(fmt.Sprintf("- %s\n", req))
		}
	}

	b.WriteString("\nScenarios (use these to guide your decomposition):\n")
	for i, s := range ctx.QuestScenarios {
		line := fmt.Sprintf("%d. %s: %s", i+1, s.Name, s.Description)
		if len(s.Skills) > 0 {
			line += fmt.Sprintf(" [skills: %s]", strings.Join(s.Skills, ", "))
		}
		if len(s.DependsOn) > 0 {
			line += fmt.Sprintf(" [depends_on: %s]", strings.Join(s.DependsOn, ", "))
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\nMap scenarios to sub-quests. Preserve dependency order.\n")

	b.WriteString(`
YOUR WORKFLOW:
1. DECOMPOSE: Use the decompose_quest tool to break the quest into sub-quests.
   - Use the scenarios above as your decomposition blueprint.
   - Each scenario should become one sub-quest. Preserve the depends_on relationships.
   - Do NOT include a "combine" or "synthesize" step — that is YOUR responsibility.
2. REVIEW: You will review each completed sub-quest via review_sub_quest.
   - Accept work that meets the objective; reject with specific feedback if it does not.
3. SYNTHESIS: After all sub-quests are accepted, YOU synthesize the final deliverable.
   - Combine the sub-quest outputs into a single coherent result.
   - Respond with [INTENT: work_product] followed by the combined deliverable.
   - This is YOUR work product — you are accountable for the final quality.

RULES:
- Do NOT delegate synthesis/combination to a sub-quest — you have all the context from reviews.`)

	return b.String()
}

func registerPartyLeadDirective(r *PromptRegistry) {
	r.Register(&PromptFragment{
		ID:          "builtin.party-lead.tool-directive",
		Category:    CategoryToolDirective,
		Priority:    0,
		Condition:   isPartyLead,
		ContentFunc: buildPartyLeadDirective,
	})
}

// subQuestExecutorDirective guides party member agents working on sub-quests.
// Without this, agents spin through tools without knowing how to signal completion.
//
// IMPORTANT: Rule 1 requires workspace exploration before submission. Without
// this gate, small models (Gemini Flash) call submit_work_product immediately
// on the first turn, producing low-quality output from context alone.
const subQuestExecutorDirective = `You are executing a SUB-QUEST assigned to you by a party lead.

COMPLETION RULES:
1. BEFORE submitting, you MUST use at least one workspace tool (read_file, list_directory, search_text, or glob_files) to understand the existing codebase. Submissions without workspace exploration will be rejected.
2. Complete the task described in the quest objective.
3. Use available tools (read_file, write_file, patch_file, etc.) to understand context and produce your work.
4. When you have finished, respond with [INTENT: work_product] followed by your complete deliverable.
5. Your deliverable MUST contain the actual work output — code, analysis, or results — not a description of what you did.
6. If the task requires code, include BOTH implementation AND tests directly in your deliverable. Your reviewer can only see what you include — they cannot access external files.
7. Do NOT ask clarifying questions unless the objective is truly ambiguous. Default to reasonable assumptions.
8. Complete the work in as few iterations as possible — avoid unnecessary exploration.`

// isSubQuestExecutor returns true for agents working on sub-quests within a
// party DAG (party members executing DAG nodes, not the lead).
func isSubQuestExecutor(ctx AssemblyContext) bool {
	return ctx.IsSubQuest
}

func registerSubQuestExecutorDirective(r *PromptRegistry) {
	r.Register(&PromptFragment{
		ID:        "builtin.sub-quest-executor.tool-directive",
		Category:  CategoryToolDirective,
		Content:   subQuestExecutorDirective,
		Priority:  0,
		Condition: isSubQuestExecutor,
	})
	// Gemini Flash tends to skip tools and call submit_work_product immediately.
	// This hint reinforces that workspace exploration is mandatory before submission.
	r.Register(&PromptFragment{
		ID:        "builtin.sub-quest-executor.provider-hint",
		Category:  CategoryProviderHints,
		Content:   "You MUST call read_file or list_directory before submitting work. Explore the workspace first. Do not submit on your first turn.",
		Priority:  0,
		Providers: []string{"gemini", "openai"},
		Condition: isSubQuestExecutor,
	})
}

func registerPartyLeadProviderHints(r *PromptRegistry) {
	// Gemini and OpenAI require an explicit reminder because they tend to emit
	// a text preamble before calling tools when given strong narrative context.
	// Anthropic models follow the directive above without additional prompting.
	r.Register(&PromptFragment{
		ID:        "builtin.party-lead.provider-hint",
		Category:  CategoryProviderHints,
		Content:   partyLeadProviderHint,
		Priority:  0,
		Providers: []string{"gemini", "openai"},
		Condition: isPartyLead,
	})
}

// isSoloAgentWithScenarios returns true for solo agents (not a party lead or
// sub-quest executor) who have at least one quest scenario defined. These agents
// receive a structured work plan rather than an open-ended quest description.
func isSoloAgentWithScenarios(ctx AssemblyContext) bool {
	return !ctx.PartyRequired && !ctx.IsSubQuest && len(ctx.QuestScenarios) > 0
}

// buildSoloAgentScenarioDirective renders the quest scenarios as a structured
// work plan for solo agents. Dependencies are included so the agent knows
// which scenarios must be completed before others can start.
func buildSoloAgentScenarioDirective(ctx AssemblyContext) string {
	var b strings.Builder
	b.WriteString("WORK PLAN:\n")
	b.WriteString("Complete the following scenarios, respecting dependencies.\n")

	if ctx.QuestGoal != "" {
		b.WriteString(fmt.Sprintf("\nGoal: %s\n", ctx.QuestGoal))
	}
	if len(ctx.QuestRequirements) > 0 {
		b.WriteString("Requirements:\n")
		for _, req := range ctx.QuestRequirements {
			b.WriteString(fmt.Sprintf("- %s\n", req))
		}
	}

	b.WriteString("\nScenarios:\n")
	for i, s := range ctx.QuestScenarios {
		line := fmt.Sprintf("%d. %s: %s", i+1, s.Name, s.Description)
		if len(s.DependsOn) > 0 {
			line += fmt.Sprintf(" [depends_on: %s]", strings.Join(s.DependsOn, ", "))
		}
		b.WriteString(line + "\n")
	}

	b.WriteString("\nComplete all scenarios and submit your deliverable using the submit_work_product tool.")
	return b.String()
}

func registerSoloAgentScenarioDirective(r *PromptRegistry) {
	r.Register(&PromptFragment{
		ID:          "builtin.solo-agent.scenario-directive",
		Category:    CategoryToolDirective,
		Priority:    10, // After party lead directive (0) but distinct
		Condition:   isSoloAgentWithScenarios,
		ContentFunc: buildSoloAgentScenarioDirective,
	})
}

// buildSoloAgentWorkOutputDirective reinforces that solo agents must produce actual
// work products (code, analysis, results) — not descriptions or plans. This is
// the solo-agent equivalent of subQuestExecutorDirective rule #4 and applies to
// all solo agents regardless of whether they have scenarios.
//
// When MaxIterations is set, injects a tool-use budget so the agent plans its
// work instead of exploring open-endedly.
func buildSoloAgentWorkOutputDirective(ctx AssemblyContext) string {
	var b strings.Builder
	b.WriteString("COMPLETION RULES:\n")

	if ctx.MaxIterations > 0 {
		b.WriteString(fmt.Sprintf("0. You have a budget of %d tool-use rounds. Plan your work to finish well within this budget. Do NOT explore open-endedly.\n", ctx.MaxIterations))
	}

	b.WriteString(`1. BEFORE submitting, you MUST use at least one workspace tool (read_file, list_directory, search_text, or glob_files) to understand the existing codebase. Submissions without workspace exploration will be rejected.
2. Use available tools (read_file, write_file, patch_file, etc.) to understand context and produce your deliverable.
3. Your deliverable MUST be the finished work output — code, implementation, analysis, or results.
4. Do NOT submit a description of what you would do. Do NOT submit a plan. Submit the completed work itself.
5. If the task requires code, include BOTH the implementation AND tests in your deliverable. Your reviewer can only evaluate what you include in the deliverable text — they cannot access external files.
6. Format code deliverables with clear sections (e.g., "## Implementation" and "## Tests") so your reviewer can assess completeness.
7. Complete the work in as few iterations as possible — avoid unnecessary exploration.`)
	return b.String()
}

// isSoloAgent returns true for agents that are NOT a party lead or sub-quest executor.
func isSoloAgent(ctx AssemblyContext) bool {
	return !ctx.PartyRequired && !ctx.IsSubQuest
}

func registerSoloAgentWorkOutputDirective(r *PromptRegistry) {
	r.Register(&PromptFragment{
		ID:          "builtin.solo-agent.work-output-directive",
		Category:    CategoryToolDirective,
		ContentFunc: buildSoloAgentWorkOutputDirective,
		Priority:    15, // After scenario directive (10)
		Condition:   isSoloAgent,
	})
}

// =============================================================================
// RESEARCH OUTPUT DIRECTIVE - Structured markdown for research/analysis quests
// =============================================================================

// researchOutputDirective instructs research and analysis agents to write
// structured markdown files instead of dumping raw text into submit_work_product.
// Skill-gated to SkillResearch and SkillAnalysis — fires for any quest requiring
// either skill, whether solo, sub-quest, or party member.
const researchOutputDirective = `RESEARCH OUTPUT FORMAT:
1. Write your findings as a structured markdown file using write_file (e.g., "research_findings.md").
2. Structure with clear sections: ## Summary, ## Findings, ## Sources (at minimum).
3. Cite sources with URLs or references. Include code examples in fenced blocks where relevant.
4. After writing the file, call submit_work_product with a brief summary only (3-5 sentences).
   Your workspace is a git branch — files you write are committed automatically on completion.
   Do NOT paste the full report into the deliverable field.`

func registerResearchOutputDirective(r *PromptRegistry) {
	r.Register(&PromptFragment{
		ID:       "builtin.research-output.tool-directive",
		Category: CategoryToolDirective,
		Content:  researchOutputDirective,
		Priority: 20, // After solo work output (15)
		Skills:   []domain.SkillTag{domain.SkillResearch, domain.SkillAnalysis},
	})
}

// =============================================================================
// REVIEW BRIEF - Compact review awareness for agents
// =============================================================================

// reviewLevelLabel returns a human-readable label for a review level.
func reviewLevelLabel(level domain.ReviewLevel) string {
	switch level {
	case domain.ReviewAuto:
		return "Auto (automated checks only)"
	case domain.ReviewStandard:
		return "Standard (LLM judge)"
	case domain.ReviewStrict:
		return "Strict (multi-judge panel)"
	case domain.ReviewHuman:
		return "Strict + Human (requires human approval)"
	default:
		return "Standard"
	}
}

// buildReviewBrief produces a compact review awareness summary (~3-5 lines).
// Agents see: review level, scoring criteria, and peer review dimensions.
// The structural checklist is already injected separately — not duplicated here.
func buildReviewBrief(ctx AssemblyContext) string {
	if len(ctx.ReviewCriteria) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Review level: %s\n", reviewLevelLabel(ctx.ReviewLevel)))

	// Compact criteria line: "correctness (40%, ≥70%), completeness (30%, ≥60%), quality (30%, ≥50%)"
	b.WriteString("Scoring: ")
	for i, c := range ctx.ReviewCriteria {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(fmt.Sprintf("%s (%d%%, ≥%d%%)", c.Name, int(c.Weight*100), int(c.Threshold*100)))
	}
	b.WriteByte('\n')

	b.WriteString("Peer ratings (1-5): task quality, communication, autonomy. Below 3.0 avg triggers corrective action on future tasks.")
	return b.String()
}

// hasReviewCriteria gates the review brief fragment — only included when
// review criteria have been populated in the assembly context.
func hasReviewCriteria(ctx AssemblyContext) bool {
	return len(ctx.ReviewCriteria) > 0
}

func registerReviewBrief(r *PromptRegistry) {
	r.Register(&PromptFragment{
		ID:          "builtin.review-brief",
		Category:    CategoryReviewBrief,
		Priority:    0,
		Condition:   hasReviewCriteria,
		ContentFunc: buildReviewBrief,
	})
}

// =============================================================================
// TOOL SELECTION GUIDANCE - Contextual tool usage heuristics
// =============================================================================

// toolGuidanceEntries maps tool names to their one-line usage guidance.
// Only tools present in AvailableToolNames are included in the output.
var toolGuidanceEntries = map[string]string{
	// Knowledge tools
	"graph_query":  "Game state (quests, agents, guilds, parties, battles). Fast.",
	"graph_search": "Knowledge graph (code, docs, repos). Use for project-specific lookups. Prefer over web_search for anything about this codebase.",
	"web_search":   "External info not in the graph (third-party APIs, libraries, general knowledge).",
	// Exploration tools — use these BEFORE reading/writing to find the right files
	"list_directory": "See what files and folders exist at a path. Start here to understand project layout.",
	"glob_files":     "Find files by pattern (e.g. '**/*.java', 'src/**/*.go'). Use to locate files before reading.",
	"search_text":    "Search file contents for text or regex. Use file_glob param to filter by extension. Returns file:line matches.",
	// File tools
	"read_file":  "Read a file's full contents. Use glob_files or list_directory first if you don't know the exact path.",
	"write_file": "Create or overwrite a file. Parent dirs must exist — use create_directory first.",
	"patch_file": "Apply targeted find-and-replace edits to an existing file. Prefer over write_file for small changes.",
	// Environment and version control
	"inspect_environment": "Discover installed tools and versions. Call once at quest start instead of multiple 'which' commands.",
	"git_operation":       "Git version control (init, clone, status, diff, log, add, commit, branch, checkout, show). Blocks push/pull/reset.",
	// Build and dependency tools
	"build_project":       "Build the project using auto-detected build system (Go, Gradle, npm, Maven, Cargo, Make). Optional target param.",
	"manage_dependencies": "Install, add, remove, or tidy dependencies via auto-detected package manager.",
	"run_tests":           "Run test commands (go test, npm test, pytest, gradle test, etc.). Use after writing code.",
	"lint_check":          "Run linters (go vet, eslint, pylint, etc.). Use after writing code to catch issues.",
	// Network tools
	"http_request": "Fetch URLs or call REST APIs. Always include https:// in the url parameter. Defaults to GET.",
	// Shell tools
	"run_command":      "General shell command. Use only when no specialized tool covers the operation.",
	"create_directory": "Create directories (including parents). Use before write_file if the target dir doesn't exist.",
}

// toolGuidanceOrder controls the display order of tool guidance entries.
var toolGuidanceOrder = []string{
	"graph_query", "graph_search", "web_search",
	"list_directory", "glob_files", "search_text",
	"inspect_environment",
	"read_file", "write_file", "patch_file",
	"git_operation",
	"build_project", "manage_dependencies",
	"run_tests", "lint_check",
	"http_request",
	"run_command", "create_directory",
}

// hasMultipleTools gates the tool guidance fragment — only included when the
// agent has more than one tool available, since single-tool agents (e.g. party
// leads with just decompose_quest) don't need selection guidance.
func hasMultipleTools(ctx AssemblyContext) bool {
	return len(ctx.AvailableToolNames) > 1
}

// buildToolSelectionGuidance generates contextual tool usage guidance based on
// which tools are actually available to the agent.
func buildToolSelectionGuidance(ctx AssemblyContext) string {
	available := make(map[string]bool, len(ctx.AvailableToolNames))
	for _, name := range ctx.AvailableToolNames {
		available[name] = true
	}

	var b strings.Builder
	b.WriteString("TOOL SELECTION:")
	wrote := false
	for _, name := range toolGuidanceOrder {
		if !available[name] {
			continue
		}
		desc, ok := toolGuidanceEntries[name]
		if !ok {
			continue
		}
		b.WriteString(fmt.Sprintf("\n- %s: %s", name, desc))
		wrote = true
	}

	if !wrote {
		return ""
	}
	return b.String()
}

func registerToolSelectionGuidance(r *PromptRegistry) {
	r.Register(&PromptFragment{
		ID:          "builtin.tool-selection-guidance",
		Category:    CategoryToolGuidance,
		Priority:    0,
		Condition:   hasMultipleTools,
		ContentFunc: buildToolSelectionGuidance,
	})
}
