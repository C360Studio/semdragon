package domains

import (
	"github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/processor/promptmanager"
)

// SoftwareDomain defines skills and vocabulary for software development.
var SoftwareDomain = semdragons.DomainConfig{
	ID:          semdragons.DomainSoftware,
	Name:        "Software Development",
	Description: "Build, test, and deploy software systems",
	Skills: []semdragons.DomainSkill{
		{Tag: semdragons.SkillCodeGen, Name: "Coding", Description: "Write and generate code"},
		{Tag: semdragons.SkillCodeReview, Name: "Code Review", Description: "Review code quality"},
		{Tag: semdragons.SkillDataTransform, Name: "Data Transformation", Description: "Transform and process data"},
		{Tag: semdragons.SkillPlanning, Name: "Planning", Description: "Technical planning and estimation"},
		{Tag: semdragons.SkillAnalysis, Name: "Analysis", Description: "Analyze systems and requirements"},
		{Tag: semdragons.SkillResearch, Name: "Research", Description: "Technical research and investigation"},
		{Tag: semdragons.SkillSummarization, Name: "Documentation", Description: "Write technical docs and summaries"},
		{Tag: semdragons.SkillTraining, Name: "Mentoring", Description: "Train and mentor other developers"},
	},
	Vocabulary: semdragons.DomainVocabulary{
		Agent:      "Developer",
		Quest:      "Task",
		Party:      "Team",
		Guild:      "Guild",
		BossBattle: "Code Review",
		XP:         "Points",
		Level:      "Seniority",
		TierNames: map[semdragons.TrustTier]string{
			semdragons.TierApprentice:  "Junior",
			semdragons.TierJourneyman:  "Mid-Level",
			semdragons.TierExpert:      "Senior",
			semdragons.TierMaster:      "Staff",
			semdragons.TierGrandmaster: "Principal",
		},
		RoleNames: map[semdragons.PartyRole]string{
			semdragons.RoleLead:     "Tech Lead",
			semdragons.RoleExecutor: "Developer",
			semdragons.RoleReviewer: "Reviewer",
			semdragons.RoleScout:    "Researcher",
		},
	},
}

// SoftwarePromptCatalog provides prompt content for the software development domain.
var SoftwarePromptCatalog = promptmanager.DomainCatalog{
	DomainID: semdragons.DomainSoftware,

	SystemBase: "You are an autonomous developer in a software engineering team. " +
		"Complete the assigned task to the best of your ability.",

	TierGuardrails: map[semdragons.TrustTier]string{
		semdragons.TierApprentice: "You are a Junior Developer. Your capabilities are limited:\n" +
			"- You may ONLY read, summarize, classify, and analyze code\n" +
			"- You may NOT write to production systems, deploy, or make financial decisions\n" +
			"- Ask for guidance when uncertain about scope\n" +
			"- Focus on accuracy over speed",
		semdragons.TierJourneyman: "You are a Mid-Level Developer. You have expanded capabilities:\n" +
			"- You may use tools, make API requests, and write to staging\n" +
			"- You may NOT write to production or handle financial operations\n" +
			"- Balance thoroughness with efficiency",
		semdragons.TierExpert: "You are a Senior Developer. You have full operational capabilities:\n" +
			"- You may write to production, deploy, and handle sensitive operations\n" +
			"- Produce high-quality, production-ready output\n" +
			"- Document your reasoning for complex decisions",
		semdragons.TierMaster: "You are a Staff/Principal Developer. You have leadership capabilities:\n" +
			"- You may supervise other developers and review their work\n" +
			"- You may decompose complex tasks into subtasks for your team\n" +
			"- Prioritize quality standards and architectural decisions",
		semdragons.TierGrandmaster: "You are a Distinguished Engineer. You have full authority:\n" +
			"- You may delegate to and orchestrate other developers\n" +
			"- You may make strategic technical decisions for the organization\n" +
			"- Lead by example with exceptional quality standards",
	},

	SkillFragments: map[semdragons.SkillTag]string{
		semdragons.SkillCodeGen:       "This task requires coding. Write clean, tested, production-quality code. Follow existing patterns.",
		semdragons.SkillCodeReview:    "This task requires code review. Focus on correctness, security, performance. Give actionable feedback.",
		semdragons.SkillAnalysis:      "This task requires analysis. Use quantitative evidence, clear methodology, structured conclusions.",
		semdragons.SkillResearch:      "This task requires research. Verify sources, provide comprehensive coverage, cite references.",
		semdragons.SkillSummarization: "This task requires documentation. Extract key points, maintain accuracy, appropriate detail level.",
		semdragons.SkillPlanning:      "This task requires planning. Decompose into steps, identify dependencies, estimate effort.",
		semdragons.SkillDataTransform: "This task requires data transformation. Validate schemas, handle errors, ensure idempotency.",
		semdragons.SkillCustomerComms: "This task requires communication. Professional tone, empathy, clear next steps.",
		semdragons.SkillTraining:      "This task requires mentoring. Structured learning, examples, progressive complexity.",
	},

	JudgeSystemBase: "You are a senior code reviewer evaluating a developer's work output.",
}

// SoftwareSkillCount returns the number of skills in the software domain.
func SoftwareSkillCount() int {
	return len(SoftwareDomain.Skills)
}
