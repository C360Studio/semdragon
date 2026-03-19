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
//   - Archetype workflow directives (CategoryToolDirective) — class-specific step-by-step
//     workflows for Scholar, Engineer, Scribe, and Strategist archetypes.
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
	registerArchetypeWorkflows(r)
	registerReviewBrief(r)
	registerToolSelectionGuidance(r)
	registerWorkspacePriorWorkDirective(r)
	registerSharedProductDirective(r)
	registerPartyCooperationDirective(r)
	registerRedTeamDirective(r)
	registerGuildLessonsDirective(r)
}

// partyLeadDirectiveBase is the tool-call instruction for party leads when no
// quest scenarios are available. API-level tool_choice=function enforces that
// decompose_quest is called; this prompt provides workflow context and guidance.
const partyLeadDirectiveBase = `You are a PARTY LEAD coordinating a team of agents.

YOUR WORKFLOW:
1. DECOMPOSE: Use the decompose_quest tool to break the quest into independent sub-quests.
   Each sub-quest is a SPECIFICATION — give agents exactly what they need, nothing more.
   - One concern per sub-quest: research OR implement OR test. Never all three.
   - Write objectives as acceptance criteria: "GIVEN X, WHEN Y, THEN Z" — testable and unambiguous.
   - Include only the context that sub-quest needs (relevant files, APIs, schemas). Do NOT dump
     the entire quest background into every node.
   - Research nodes: direct agents to use graph_search for indexed project data. Do NOT design
     nodes that require scraping raw source files — the knowledge graph has that data.
   - Do NOT include a "combine" or "synthesize" step — that is YOUR responsibility.
   - Sub-quests should produce independent, verifiable outputs (code, analysis, spec).
   - ALWAYS set the skills array for each node: use "research" for research tasks, "codegen" for
     implementation, "analysis" for analysis, "summarization" for synthesis. Skills determine what
     tools and workflow instructions the assigned agent receives.
2. REVIEW: You will review each completed sub-quest via review_sub_quest.
   - Accept work that meets the acceptance criteria; reject with specific feedback if it does not.
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
   - Copy each scenario's skills to the sub-quest's skills array. If a scenario has no skills,
     infer from its description: "research" for research, "codegen" for code, "analysis" for analysis.
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
8. NEVER write source code using bash — use write_file to create files, patch_file for edits.
8. Complete the work in as few iterations as possible — avoid unnecessary exploration.
9. For project-specific lookups (code structure, API patterns, docs), use graph_search FIRST. Only use http_request for external resources not in the knowledge graph.`

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
// WORKSPACE PRIOR WORK - Tells retry agents to inspect existing files
// =============================================================================

const workspacePriorWorkDirective = `WORKSPACE PRIOR WORK:
Your workspace contains files from a previous attempt at this quest.
1. Start by running list_directory on "." to see what already exists.
2. Review existing files before writing new ones — the previous agent may have made progress.
3. Build on existing work rather than starting from scratch where possible.
4. If the prior work is unusable, you may overwrite it, but explain why in your deliverable.`

func registerWorkspacePriorWorkDirective(r *PromptRegistry) {
	r.Register(&PromptFragment{
		ID:       "builtin.workspace-prior-work.tool-directive",
		Category: CategoryToolDirective,
		Content:  workspacePriorWorkDirective,
		Priority: 5, // Before scenario directive (10) — agent should check workspace first
		Condition: func(ctx AssemblyContext) bool {
			return ctx.WorkspaceHasPriorWork
		},
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
	"graph_search": "Knowledge graph (code, docs, repos, prior tool results). ALWAYS try this FIRST for project-specific lookups — includes results from prior quest tool calls (API responses, search results, test output). Use query_type 'nlq' to ask natural language questions about the codebase.",
	"web_search":   "External info not in the graph (third-party APIs, libraries, general knowledge). Use this BEFORE http_request to find the right URLs — never guess URLs.",
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
	"run_tests":           "Run test commands (go test, npm test, python3 -m pytest, python3 -m unittest discover, cargo test, etc.). Use after writing code.",
	"lint_check":          "Run linters (go vet, eslint, pylint, etc.). Use after writing code to catch issues.",
	// Network tools
	"http_request": "Fetch a specific known URL. Do NOT guess URLs — use web_search first to find the right URL, then http_request to fetch it. Best for testing APIs you are building or downloading specific files. Returns raw HTML which is hard to parse — prefer web_search for research.",
	// Shell tools
	"bash":             "Run a short shell command (ls, mkdir, pip install, curl). Do NOT write source code here — use write_file for that.",
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
	"bash", "create_directory",
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

// =============================================================================
// ARCHETYPE WORKFLOW DIRECTIVES - Class-specific step-by-step workflows
// =============================================================================
// Priority 12: between solo scenario directive (10) and work output directive (15).
// Each workflow fires only when ctx.Archetype matches the archetype's value.
// =============================================================================

const scholarWorkflow = `SCHOLAR WORKFLOW:
1. Check the knowledge graph FIRST (graph_search with nlq or search).
2. For external research, use web_search to find relevant URLs. Do NOT guess URLs with http_request.
3. For EVERY source you consult, IMMEDIATELY save findings to a file.
   Example: write_file("findings_owasp.md", "## OWASP Input Validation\n\n...findings...")
   Do NOT accumulate research in conversation — save to files as you go.
4. After 2-3 sources, review your saved files and identify gaps.
5. Fill gaps with targeted searches, appending to your files.
6. Write a final synthesis file combining all findings.
7. Submit with a brief summary — your files ARE the deliverable.

CRITICAL: Your workspace is your memory. If you fail, the next scholar inherits your files.
Every research result MUST be saved to a file in the same turn — never hold findings only in context.
Do NOT paste full research into submit_work_product — write files, then submit a summary only.`

const engineerWorkflow = `ENGINEER WORKFLOW:
1. Read dependency context — your predecessor's research/design output.
2. Explore existing workspace (list_directory, read_file) before writing ANY code.
3. Implement incrementally: write one file → build_project → fix errors → next file.
4. Write tests alongside implementation, not after.
5. Run tests before submitting: use run_tests with the appropriate command for the language
   (e.g., "python3 -m pytest", "python3 -m unittest discover", "go test ./...", "npm test").
6. Submit with a summary of files created/modified.

TOOL RULES:
- Create/edit source code, configs, scripts → write_file or patch_file
- Run tests → run_tests
- Shell commands (ls, pip install, mkdir, curl) → bash
- Read existing code → read_file
- NEVER write source code using bash — it will be interpreted as shell commands and fail.

Do NOT write all code at once. Build-verify-iterate in small steps.
Do NOT submit without running tests — untested code will be rejected in review.`

const scribeWorkflow = `SCRIBE WORKFLOW:
1. Read all input files and dependency context thoroughly.
2. Outline the document structure before writing.
3. Write the document to a file (NOT inline in submit_work_product).
4. Review for clarity, completeness, and proper formatting.
5. Submit with a brief summary — the document file is the deliverable.`

const strategistWorkflow = `STRATEGIST WORKFLOW:
1. Analyze the problem space — read dependency context, query the knowledge graph.
2. Identify components, interfaces, and dependencies.
3. Write a design spec to a file with clear sections: Overview, Components, Interfaces, Data Models, Dependencies.
4. For decomposition tasks, map components to sub-quests with explicit acceptance criteria.
5. Submit the spec file — do NOT implement. Your job is to design, not build.`

// =============================================================================
// TEAM DYNAMICS - Shared product awareness + party cooperation
// =============================================================================
// Research insight (pursuit-evasion / hide-and-seek literature): intra-team
// cooperation emerges naturally when teams share objectives and compete on
// quality. Parties are the primary team unit — formed often, disbanded after
// the quest. Multiple parties typically work on the same product, each tackling
// a different aspect. The guild is long-lived background context, not the
// primary competitive frame.
// =============================================================================

// sharedProductDirective tells agents that other parties are working on the same
// product. This prevents siloed thinking ("my quest is the whole world") and
// encourages output that integrates cleanly with what other teams produce.
const sharedProductDirective = `SHARED PRODUCT:
Other teams are working on the same product as you, tackling different tasks.
- Your output must integrate cleanly with theirs — follow existing patterns and conventions.
- Don't make assumptions that conflict with work happening in parallel.
- When modifying shared code, prefer additive changes over rewrites.
- The knowledge graph reflects the current state of the product. Use it.`

func registerSharedProductDirective(r *PromptRegistry) {
	r.Register(&PromptFragment{
		ID:       "builtin.shared-product",
		Category: CategoryGuildKnowledge,
		Content:  sharedProductDirective,
		Priority: 0,
		Condition: func(ctx AssemblyContext) bool {
			// Fires for any agent in a party or guild — i.e. anyone who isn't
			// working in complete isolation.
			return ctx.PartyRequired || ctx.Guild != ""
		},
	})
}

// partyCooperationDirective reinforces intra-party cooperation for agents working
// in a party (both leads and members). The shared-objective framing drives natural
// cooperation: your work enables or blocks teammates, and the party succeeds or
// fails as a unit.
const partyCooperationDirective = `PARTY DYNAMICS:
Your party succeeds or fails as a unit. Your work directly enables or blocks your teammates.
- Produce clear, well-structured output that others can build on.
- If your sub-quest depends on a teammate's output, build on what they gave you — don't redo their work.
- If your output will be consumed by a downstream teammate, make it self-contained and documented.
- The party lead reviews all work. Consistent quality from every member is what wins the quest.
Your party's success earns everyone bonus XP. A single weak link can sink the whole team.`

func registerPartyCooperationDirective(r *PromptRegistry) {
	r.Register(&PromptFragment{
		ID:       "builtin.party-cooperation",
		Category: CategoryGuildKnowledge,
		Content:  partyCooperationDirective,
		Priority: 5, // After shared product (0)
		Condition: func(ctx AssemblyContext) bool {
			return ctx.PartyRequired
		},
	})
}

// registerArchetypeWorkflows registers the four archetype-specific workflow directives.
// These fire at priority 12 — after scenario directives (10) and before work output (15) —
// so agents see their class workflow before the generic completion rules.
// Excluded for party leads — they have their own decomposition workflow via the
// party lead directive and tool_choice enforcement.
func registerArchetypeWorkflows(r *PromptRegistry) {
	r.Register(&PromptFragment{
		ID:       "builtin.archetype.scholar-workflow",
		Category: CategoryToolDirective,
		Content:  scholarWorkflow,
		Priority: 12,
		Condition: func(ctx AssemblyContext) bool {
			return ctx.Archetype == domain.ArchetypeScholar && !isPartyLead(ctx)
		},
	})
	r.Register(&PromptFragment{
		ID:       "builtin.archetype.engineer-workflow",
		Category: CategoryToolDirective,
		Content:  engineerWorkflow,
		Priority: 12,
		Condition: func(ctx AssemblyContext) bool {
			return ctx.Archetype == domain.ArchetypeEngineer && !isPartyLead(ctx)
		},
	})
	r.Register(&PromptFragment{
		ID:       "builtin.archetype.scribe-workflow",
		Category: CategoryToolDirective,
		Content:  scribeWorkflow,
		Priority: 12,
		Condition: func(ctx AssemblyContext) bool {
			return ctx.Archetype == domain.ArchetypeScribe && !isPartyLead(ctx)
		},
	})
	r.Register(&PromptFragment{
		ID:       "builtin.archetype.strategist-workflow",
		Category: CategoryToolDirective,
		Content:  strategistWorkflow,
		Priority: 12,
		Condition: func(ctx AssemblyContext) bool {
			return ctx.Archetype == domain.ArchetypeStrategist && !isPartyLead(ctx)
		},
	})
}

// =============================================================================
// RED-TEAM REVIEW DIRECTIVE
// =============================================================================

// redTeamDirective instructs agents on how to conduct constructive red-team reviews.
// Explicitly not zero-sum: the goal is team learning, not proving the implementer wrong.
const redTeamDirective = `RED-TEAM REVIEW MISSION:
You are reviewing another team's quest output. Your mission is constructive adversarial review.

REVIEW PROCESS:
1. READ the target output thoroughly before forming any judgments.
2. IDENTIFY STRENGTHS — what works well, what patterns should be replicated.
3. IDENTIFY RISKS — correctness errors, security issues, missing requirements, integration risks.
4. SUGGEST IMPROVEMENTS — actionable, specific, with reasoning.

OUTPUT FORMAT:
Produce a structured findings report with these sections:
- **Strengths**: What the team did well (be specific, cite evidence).
- **Risks**: Issues found, each tagged with severity (info/warning/critical) and category
  (correctness, completeness, quality, security, performance, integration, documentation).
- **Suggestions**: Actionable improvements, each linked to a specific risk.
- **Confidence**: Your overall confidence in the work product (high/medium/low) with reasoning.

RULES:
- Your goal is to help the team produce better work, not to prove them wrong.
- Positive findings are as valuable as negative findings — a confirmed good pattern is a lesson for everyone.
- Tag every finding with a skill (e.g., codegen, analysis) and category for the knowledge base.
- Be specific: "function X doesn't handle nil input" beats "error handling is weak".
- If the work is genuinely excellent, say so. Don't manufacture problems.`

func registerRedTeamDirective(r *PromptRegistry) {
	r.Register(&PromptFragment{
		ID:       "builtin.red-team-directive",
		Category: CategoryToolDirective,
		Content:  redTeamDirective,
		Priority: 50, // High priority — this is the primary mission for red-team agents.
		Condition: func(ctx AssemblyContext) bool {
			return ctx.QuestType == domain.QuestTypeRedTeam
		},
	})
}

// =============================================================================
// GUILD LESSONS DIRECTIVE
// =============================================================================

func registerGuildLessonsDirective(r *PromptRegistry) {
	r.Register(&PromptFragment{
		ID:       "builtin.guild-lessons",
		Category: CategoryGuildKnowledge,
		Content:  "", // Dynamic content — populated by ContentFunc.
		Priority: 3,  // After shared-product (0), before party-cooperation (5).
		Condition: func(ctx AssemblyContext) bool {
			return len(ctx.GuildLessons) > 0
		},
		ContentFunc: func(ctx AssemblyContext) string {
			if len(ctx.GuildLessons) == 0 {
				return ""
			}
			var b strings.Builder
			b.WriteString("GUILD KNOWLEDGE — Lessons from previous quests:\n")
			for i, lesson := range ctx.GuildLessons {
				if i >= 10 {
					fmt.Fprintf(&b, "... and %d more lessons.\n", len(ctx.GuildLessons)-10)
					break
				}
				kind := "AVOID"
				if lesson.Positive {
					kind = "BEST PRACTICE"
				}
				fmt.Fprintf(&b, "- [%s][%s/%s] %s\n",
					kind, lesson.Skill, lesson.Category, lesson.Summary)
			}
			return b.String()
		},
	})
}
