package main

// questBriefResponse is returned when the user's message looks like a single
// quest creation request. The ```json:quest_brief tagged block is parsed by
// service/api.extractTaggedJSON and then validated with domain.ValidateQuestBrief.
//
// Required fields per ValidateQuestBrief: title (non-empty).
// Optional but useful for downstream processors: difficulty (1-5), skills,
// requirements (acceptance criteria), scenarios.
const questBriefResponse = `Here is a quest based on your request.

` + "```" + `json:quest_brief
{
  "title": "Mock Quest: Analyze Test Data",
  "goal": "Process the test dataset and generate a summary report covering all data points.",
  "difficulty": 2,
  "skills": ["analysis", "summarization"],
  "requirements": [
    "Summary report generated",
    "All data points processed",
    "Report saved to output directory"
  ],
  "scenarios": [
    {
      "name": "Data ingestion",
      "description": "Load all CSV files from the input directory and validate schema conformance",
      "skills": ["analysis"]
    },
    {
      "name": "Summary generation",
      "description": "Compute aggregate statistics and produce a formatted summary report",
      "skills": ["summarization"]
    }
  ]
}
` + "```" + `

The quest has been structured with a Journeyman difficulty level and requires
analysis and summarization skills. Let me know if you would like to adjust the
requirements or assign it to a specific guild.`

// questChainResponse is returned when the user's message requests a chain or
// multiple quests. The ```json:quest_chain tagged block is parsed by
// service/api.extractTaggedJSON and validated with domain.ValidateQuestChainBrief.
//
// Required fields: quests array (non-empty), each entry must have a title.
// depends_on uses zero-based indices into the quests array.
const questChainResponse = `Here is a two-quest chain for your workflow.

` + "```" + `json:quest_chain
{
  "quests": [
    {
      "title": "Mock Quest: Gather Raw Data",
      "goal": "Collect and validate the raw input dataset from all configured sources.",
      "difficulty": 1,
      "skills": ["data-gathering"],
      "requirements": [
        "All data sources queried",
        "Raw dataset saved to staging area"
      ]
    },
    {
      "title": "Mock Quest: Process and Report",
      "goal": "Transform the validated raw data and produce the final analysis report.",
      "difficulty": 2,
      "skills": ["analysis", "summarization"],
      "requirements": [
        "Data transformed successfully",
        "Report delivered to stakeholders"
      ],
      "depends_on": [0]
    }
  ]
}
` + "```" + `

The second quest depends on the first completing successfully. Both quests will
be posted to the quest board and agents with matching skills will be able to claim
them in order.`

// conversationalResponse is the fallback for general DM chat that does not
// match a quest creation or chain pattern. It simulates DM flavour text.
const conversationalResponse = `Greetings, adventurer. The quest board is open and agents across all guilds
stand ready to serve. You may post a quest by describing your task, request a
chain of interdependent quests, or ask about the current state of the realm.

What challenge shall we undertake today?`

// completionContent is the response sent on the second turn of an agentic loop,
// after tool results have been returned. It signals a successful completion so
// the questbridge component can record the loop outcome.
const completionContent = `I have reviewed the tool output and the task is complete.

The requested operation finished successfully. All output has been validated
and is ready for the next stage of the workflow.`

// dagDecompositionContent is the completion content returned after the
// decompose_quest tool call. It contains the DAG JSON that questbridge's
// extractDAGFromOutput will detect and parse into sub-quests.
const dagDecompositionContent = `I have analyzed the quest and decomposed it into independent sub-tasks.

{"goal":"Complete the requested work through parallel sub-tasks","dag":{"nodes":[{"id":"node-1","objective":"Implement the first component of the task","skills":["code_generation"],"acceptance":["Code compiles","Tests pass"],"depends_on":[],"difficulty":2},{"id":"node-2","objective":"Implement the second component of the task","skills":["code_generation"],"acceptance":["Code compiles","Tests pass"],"depends_on":[],"difficulty":2}]}}

Both sub-tasks can be executed in parallel since they have no dependencies.`

// dagDecompositionArgs is the canned arguments for the decompose_quest tool call.
// Must match DecomposeExecutor's expected schema: goal (string) + nodes (array of
// QuestNode objects with id, objective, skills, acceptance, depends_on, difficulty).
const dagDecompositionArgs = `{"goal":"Complete the requested work through parallel sub-tasks","nodes":[{"id":"node-1","objective":"Implement the first component of the task","skills":["code_generation"],"acceptance":["Code compiles","Tests pass"],"depends_on":[],"difficulty":2},{"id":"node-2","objective":"Implement the second component of the task","skills":["code_generation"],"acceptance":["Code compiles","Tests pass"],"depends_on":[],"difficulty":2}]}`

// triageResponse is returned when the system prompt matches a DM triage evaluation.
// The questboard triage module sends a system prompt containing "recovery path" and
// a user message with quest failure details. The mock always returns "salvage" to
// exercise the most interesting recovery path in E2E tests.
const triageResponse = `{"path":"salvage","analysis":"The agent produced partial useful output that can be built upon. The core approach is sound but incomplete — granting one more attempt with the existing work preserved.","salvaged_output":"Partial implementation completed. Core data structures defined and initial processing pipeline functional.","anti_patterns":["Attempting to process all data in a single pass without checkpointing"]}`

// webSearchArgs is the canned arguments for the web_search tool call.
// Simulates a research query that an agent working on a research quest would issue.
const webSearchArgs = `{"query":"best practices data validation Go","max_results":3}`

// webSearchSubmitArgs is the submit_work_product arguments returned after
// receiving web_search results. Contains a research summary deliverable.
const webSearchSubmitArgs = `{"deliverable":"Based on web search results, the top 3 recommendations for input validation in Go web applications are:\n\n1. Use struct validation tags with a library like go-playground/validator for declarative field constraints.\n2. Sanitize all user input at the boundary layer before it reaches business logic.\n3. Implement allowlist-based validation rather than blocklist patterns for security-sensitive fields.\n\nAll recommendations are sourced from recent Go community documentation and security best practices guides.","summary":"Research complete: input validation best practices summarized"}`

// graphSearchArgs is the canned arguments for the graph_search tool call.
// Simulates a knowledge graph query that an agent working on a research quest would issue.
const graphSearchArgs = `{"query_type":"search","search_text":"data validation best practices","limit":5}`

// graphSearchSubmitArgs is the submit_work_product arguments returned after
// receiving graph_search results. Contains a research summary deliverable.
const graphSearchSubmitArgs = `{"deliverable":"Based on graph search results, found 3 relevant entities covering data validation patterns, input sanitization approaches, and schema conformance checks. All sources are indexed in the project knowledge graph.","summary":"Graph search complete: validation patterns identified"}`

// reviewAcceptArgs was previously a static constant with a placeholder sub_quest_id.
// It is now built dynamically by buildReviewAcceptArgs() in router.go, which
// extracts the real sub-quest ID from the prompt messages.

// reviewAcceptCompletion is the completion content after review tool results.
const reviewAcceptCompletion = `I have reviewed the sub-quest output. The work meets all acceptance criteria and I accept it.`

// dmGraphQueryArgs is the canned arguments for a DM graph_query tool call.
const dmGraphQueryArgs = `{"entity_type":"quest","limit":10}`

// dmGraphQueryCompletion is the DM's natural-language response after receiving
// graph_query results. Unlike agent paths, the DM responds with text, not
// submit_work_product.
const dmGraphQueryCompletion = `Based on querying the game state, I can see the current quest board status. There are active quests being worked on by agents and some posted quests awaiting claims. The board is healthy and agents are actively engaging with the available work.`

// dmWebSearchCompletion is the DM's response after web_search tool results.
const dmWebSearchCompletion = `Based on my web search results, I found relevant information about best practices for the topic you asked about. The key recommendations include proper validation, boundary-layer sanitization, and allowlist-based patterns.`

// dmGraphSearchCompletion is the DM's response after graph_search tool results.
const dmGraphSearchCompletion = `The knowledge graph contains several entities related to your query. I found relevant documentation and code patterns that should help inform the quest design.`

// dmReadFileCompletion is the DM's response after read_file tool results.
const dmReadFileCompletion = `I have reviewed the file contents. The file contains relevant information for your question.`

// dmGenericToolCompletion is the DM's fallback response after any other tool results.
const dmGenericToolCompletion = `I have gathered the requested information using the available tools. Based on what I found, here is my analysis of the current situation.`
