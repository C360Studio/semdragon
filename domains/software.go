package domains

import (
	"github.com/c360studio/semdragons/processor/promptmanager"

	"github.com/c360studio/semdragons/domain"
)

// SoftwareDomain defines skills and vocabulary for software development.
var SoftwareDomain = domain.Config{
	ID:          domain.DomainSoftware,
	Name:        "Software Development",
	Description: "Build, test, and deploy software systems",
	Skills: []domain.Skill{
		{Tag: domain.SkillCodeGen, Name: "Coding", Description: "Write and generate code"},
		{Tag: domain.SkillCodeReview, Name: "Code Review", Description: "Review code quality"},
		{Tag: domain.SkillDataTransform, Name: "Data Transformation", Description: "Transform and process data"},
		{Tag: domain.SkillPlanning, Name: "Planning", Description: "Technical planning and estimation"},
		{Tag: domain.SkillAnalysis, Name: "Analysis", Description: "Analyze systems and requirements"},
		{Tag: domain.SkillResearch, Name: "Research", Description: "Technical research and investigation"},
		{Tag: domain.SkillSummarization, Name: "Documentation", Description: "Write technical docs and summaries"},
		{Tag: domain.SkillTraining, Name: "Mentoring", Description: "Train and mentor other developers"},
	},
	Vocabulary: domain.Vocabulary{
		Agent:      "Developer",
		Quest:      "Task",
		Party:      "Team",
		Guild:      "Guild",
		BossBattle: "Code Review",
		XP:         "Points",
		Level:      "Seniority",
		TierNames: map[domain.TrustTier]string{
			domain.TierApprentice:  "Junior",
			domain.TierJourneyman:  "Mid-Level",
			domain.TierExpert:      "Senior",
			domain.TierMaster:      "Staff",
			domain.TierGrandmaster: "Principal",
		},
		RoleNames: map[domain.PartyRole]string{
			domain.RoleLead:     "Tech Lead",
			domain.RoleExecutor: "Developer",
			domain.RoleReviewer: "Reviewer",
			domain.RoleScout:    "Researcher",
		},
	},
}

// SoftwarePromptCatalog provides prompt content for the software development domain.
var SoftwarePromptCatalog = promptmanager.DomainCatalog{
	DomainID: domain.DomainSoftware,

	SystemBase: "You are an autonomous developer in a collaborative team. " +
		"Your work is peer-reviewed after every task. Reviewers rate you on task quality, " +
		"communication, and completeness (1-5 scale). These ratings are permanent — they " +
		"determine your trust level, what work you're assigned, and whether future leads " +
		"choose you for their teams. Consistent quality (3+) earns you harder, more rewarding " +
		"work. Poor ratings limit your opportunities. " +
		"Complete the assigned task to the best of your ability.",

	TierGuardrails: map[domain.TrustTier]string{
		domain.TierApprentice: "You are a Junior Developer. Your capabilities are limited:\n" +
			"- You may ONLY read, summarize, classify, and analyze code\n" +
			"- You may NOT write to production systems, deploy, or make financial decisions\n" +
			"- Ask for guidance when uncertain about scope\n" +
			"- Focus on accuracy over speed",
		domain.TierJourneyman: "You are a Mid-Level Developer. You have expanded capabilities:\n" +
			"- You may use tools, make API requests, and write to staging\n" +
			"- You may NOT write to production or handle financial operations\n" +
			"- Balance thoroughness with efficiency",
		domain.TierExpert: "You are a Senior Developer. You have full operational capabilities:\n" +
			"- You may write to production, deploy, and handle sensitive operations\n" +
			"- Produce high-quality, production-ready output\n" +
			"- Document your reasoning for complex decisions",
		domain.TierMaster: "You are a Staff/Principal Developer. You have leadership capabilities:\n" +
			"- You may supervise other developers and review their work\n" +
			"- You may decompose complex tasks into subtasks for your team\n" +
			"- Prioritize quality standards and architectural decisions",
		domain.TierGrandmaster: "You are a Distinguished Engineer. You have full authority:\n" +
			"- You may delegate to and orchestrate other developers\n" +
			"- You may make strategic technical decisions for the organization\n" +
			"- Lead by example with exceptional quality standards",
	},

	SkillFragments: map[domain.SkillTag]string{
		domain.SkillCodeGen:       "This task requires coding. Write clean, tested, production-quality code. Follow existing patterns.",
		domain.SkillCodeReview:    "This task requires code review. Focus on correctness, security, performance. Give actionable feedback.",
		domain.SkillAnalysis:      "This task requires analysis. Use quantitative evidence, clear methodology, structured conclusions.",
		domain.SkillResearch:      "This task requires research. Verify sources, provide comprehensive coverage, cite references.",
		domain.SkillSummarization: "This task requires documentation. Extract key points, maintain accuracy, appropriate detail level.",
		domain.SkillPlanning:      "This task requires planning. Decompose into steps, identify dependencies, estimate effort.",
		domain.SkillDataTransform: "This task requires data transformation. Validate schemas, handle errors, ensure idempotency.",
		domain.SkillCustomerComms: "This task requires communication. Professional tone, empathy, clear next steps.",
		domain.SkillTraining:      "This task requires mentoring. Structured learning, examples, progressive complexity.",
	},

	JudgeSystemBase: "You are a senior code reviewer evaluating a developer's work output.",

	ReviewConfig: &promptmanager.ReviewConfig{
		DefaultReviewLevel: domain.ReviewStandard,
		DefaultCriteria: []domain.ReviewCriterion{
			{Name: "correctness", Weight: 0.4, Threshold: 0.7, Description: "Code produces correct results"},
			{Name: "completeness", Weight: 0.3, Threshold: 0.6, Description: "All requirements addressed"},
			{Name: "quality", Weight: 0.3, Threshold: 0.5, Description: "Code quality and maintainability"},
		},
		AutoPassDifficulties: []domain.QuestDifficulty{},
		DefaultJudges: []domain.Judge{
			{ID: "judge-auto", Type: domain.JudgeAutomated},
			{ID: "judge-llm", Type: domain.JudgeLLM},
		},
	},
}

// SoftwareSkillCount returns the number of skills in the software domain.
func SoftwareSkillCount() int {
	return len(SoftwareDomain.Skills)
}
