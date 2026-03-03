package main

// questBriefResponse is returned when the user's message looks like a single
// quest creation request. The ```json:quest_brief tagged block is parsed by
// service/api.extractTaggedJSON and then validated with domain.ValidateQuestBrief.
//
// Required fields per ValidateQuestBrief: title (non-empty).
// Optional but useful for downstream processors: difficulty (1-5), skills,
// acceptance criteria.
const questBriefResponse = `Here is a quest based on your request.

` + "```" + `json:quest_brief
{
  "title": "Mock Quest: Analyze Test Data",
  "description": "Process the test dataset and generate a summary report covering all data points.",
  "difficulty": 2,
  "skills": ["analysis", "summarization"],
  "acceptance": [
    "Summary report generated",
    "All data points processed",
    "Report saved to output directory"
  ]
}
` + "```" + `

The quest has been structured with a Journeyman difficulty level and requires
analysis and summarization skills. Let me know if you would like to adjust the
acceptance criteria or assign it to a specific guild.`

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
      "description": "Collect and validate the raw input dataset from all configured sources.",
      "difficulty": 1,
      "skills": ["data-gathering"],
      "acceptance": [
        "All data sources queried",
        "Raw dataset saved to staging area"
      ]
    },
    {
      "title": "Mock Quest: Process and Report",
      "description": "Transform the validated raw data and produce the final analysis report.",
      "difficulty": 2,
      "skills": ["analysis", "summarization"],
      "acceptance": [
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
