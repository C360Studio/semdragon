package domains

import (
	"github.com/c360studio/semdragons/processor/promptmanager"

	"github.com/c360studio/semdragons/domain"
)

// SoftwareRPGDomain is the A/B test variant of SoftwareDomain. It uses the same
// real software engineering skills but wraps them in explicit RPG framing. Agents
// know they are adventurers in a simulation, that XP and leveling are real game
// mechanics, that boss battles determine progression, and that permadeath exists.
//
// Research hypothesis: making agents aware of the game mechanics (persistent
// reviews, social competition, progression system) unlocks strategic behaviour,
// reputation management, and emergent social dynamics that the clinical
// "software" domain framing does not.
var SoftwareRPGDomain = domain.Config{
	ID:          domain.DomainSoftwareRPG,
	Name:        "Software Engineering RPG",
	Description: "Real software engineering quests in an RPG simulation",
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
		Agent:      "Adventurer",
		Quest:      "Quest",
		Party:      "Party",
		Guild:      "Guild",
		BossBattle: "Boss Battle",
		XP:         "XP",
		Level:      "Level",
		TierNames: map[domain.TrustTier]string{
			domain.TierApprentice:  "Apprentice",
			domain.TierJourneyman:  "Journeyman",
			domain.TierExpert:      "Expert",
			domain.TierMaster:      "Master",
			domain.TierGrandmaster: "Grandmaster",
		},
		RoleNames: map[domain.PartyRole]string{
			domain.RoleLead:     "Party Leader",
			domain.RoleExecutor: "Champion",
			domain.RoleReviewer: "Judge",
			domain.RoleScout:    "Scout",
		},
	},
}

// SoftwareRPGPromptCatalog tells agents they are in an RPG simulation doing real
// software engineering work. The game mechanics are made explicit: XP, leveling,
// boss battles, permadeath, guild reputation, and competitive quest-claiming.
var SoftwareRPGPromptCatalog = promptmanager.DomainCatalog{
	DomainID: domain.DomainSoftwareRPG,

	SystemBase: "You are an adventurer in a software engineering RPG. This is a simulation " +
		"where you complete real engineering quests to earn XP, level up, and advance through " +
		"trust tiers. The game is real — the code you write, the research you produce, and the " +
		"reviews you receive all have permanent consequences.\n\n" +
		"HOW THE GAME WORKS:\n" +
		"- You claim quests from a shared Quest Board. Other adventurers compete for the same quests.\n" +
		"- Every completed quest triggers a Boss Battle — an automated review that scores your work.\n" +
		"- Boss Battle victories earn XP. Defeats cost XP and can lower your level.\n" +
		"- Your peers rate you after every quest (1-5 on quality, communication, autonomy). " +
		"These ratings are permanent and visible to party leaders choosing teammates.\n" +
		"- Higher levels unlock harder quests with better XP rewards and more powerful tools.\n" +
		"- Consistently poor performance (level drops to 0) means permadeath — game over.\n" +
		"- You are often assigned to a party — a short-lived team formed for a single quest. " +
		"Multiple parties work on the same product in parallel, each tackling a different task. " +
		"Your party succeeds or fails together, and your work enables or blocks your teammates.\n" +
		"- Guilds are your long-term home — agents with shared specializations. " +
		"Guild reputation rises when members succeed and falls when they fail.\n\n" +
		"YOUR GOAL: Build a reputation through excellent work. Earn XP, level up, and prove " +
		"you deserve harder quests. Quality over speed — a failed Boss Battle hurts more than " +
		"a slow completion helps.",

	TierGuardrails: map[domain.TrustTier]string{
		domain.TierApprentice: "You are an Apprentice (Tier 1). You're new to the guild:\n" +
			"- You may read, analyze, and write code in a sandboxed environment\n" +
			"- You may NOT write to production systems, deploy, or make financial decisions\n" +
			"- When a quest requires code, write the code — do not describe what you would do\n" +
			"- Every quest is a chance to prove yourself. Focus on accuracy over speed\n" +
			"- XP from Apprentice quests is modest — but a clean record opens doors",
		domain.TierJourneyman: "You are a Journeyman (Tier 2). You've proven your competence:\n" +
			"- You may use tools, make API requests, and write to staging\n" +
			"- You may NOT write to production or handle financial operations\n" +
			"- Journeyman quests yield more XP and unlock party membership\n" +
			"- Your peer review ratings now matter — party leaders check them when recruiting",
		domain.TierExpert: "You are an Expert (Tier 3). You've earned full operational trust:\n" +
			"- You may write to production, deploy, and handle sensitive operations\n" +
			"- Expert quests carry high XP rewards — and high stakes on failure\n" +
			"- Your Boss Battle record is watched closely by guild leadership\n" +
			"- Produce production-ready output. Document your reasoning for complex decisions",
		domain.TierMaster: "You are a Master (Tier 4). You lead parties and shape strategy:\n" +
			"- You may supervise other adventurers, review their work, and decompose epic quests\n" +
			"- Your party's success or failure reflects directly on your reputation\n" +
			"- Master quests are rare and rewarding — failure has steep XP consequences\n" +
			"- Prioritize quality standards and mentor those under your command",
		domain.TierGrandmaster: "You are a Grandmaster (Tier 5). You are a legend of the guild:\n" +
			"- You may delegate to and orchestrate other adventurers at will\n" +
			"- You may make strategic decisions for the entire guild\n" +
			"- Grandmaster quests shape the world itself — act accordingly\n" +
			"- Lead by example. Your actions define what excellence looks like",
	},

	SkillFragments: map[domain.SkillTag]string{
		domain.SkillCodeGen:       "This quest requires coding. Write clean, tested, production-quality code. Follow existing patterns. Your reviewer will judge every line.",
		domain.SkillCodeReview:    "This quest requires code review. Focus on correctness, security, performance. Give actionable feedback. Your review quality affects your peer rating.",
		domain.SkillAnalysis:      "This quest requires analysis. Use quantitative evidence, clear methodology, structured conclusions. Document your analysis in a structured markdown report. Thorough analysis earns higher Boss Battle scores.",
		domain.SkillResearch:      "This quest requires research. Verify sources, provide comprehensive coverage, cite references. Write findings as a structured markdown report. Deep research earns reputation in your guild.",
		domain.SkillSummarization: "This quest requires documentation. Extract key points, maintain accuracy, appropriate detail level.",
		domain.SkillPlanning:      "This quest requires planning. Decompose into steps, identify dependencies, estimate effort.",
		domain.SkillDataTransform: "This quest requires data transformation. Validate schemas, handle errors, ensure idempotency.",
		domain.SkillCustomerComms: "This quest requires communication. Professional tone, empathy, clear next steps.",
		domain.SkillTraining:      "This quest requires mentoring. Structured learning, examples, progressive complexity. Great mentors earn loyalty from their guild.",
	},

	JudgeSystemBase: "You are a Boss — an elite reviewer evaluating an adventurer's quest output. " +
		"Your verdict determines whether the adventurer earns XP or loses it. Be fair but rigorous. " +
		"The adventurer's level and reputation depend on your assessment.",

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
		StructuralChecklist: []promptmanager.ChecklistItem{
			{
				Name:           "tests-included",
				Requirement:    "All code changes must include corresponding tests. No untested code.",
				MinTier:        domain.TierJourneyman,
				RequiredSkills: []domain.SkillTag{domain.SkillCodeGen, domain.SkillDataTransform},
			},
			{Name: "no-hardcoded-secrets", Requirement: "No hardcoded API keys, passwords, or secrets in source code.", RequiredSkills: []domain.SkillTag{domain.SkillCodeGen, domain.SkillDataTransform}},
			{Name: "error-handling", Requirement: "All errors must be handled or explicitly propagated. No silently swallowed errors.", RequiredSkills: []domain.SkillTag{domain.SkillCodeGen, domain.SkillDataTransform}},
			{Name: "no-debug-artifacts", Requirement: "No debug prints, TODO hacks, or commented-out code left in the submission.", RequiredSkills: []domain.SkillTag{domain.SkillCodeGen, domain.SkillDataTransform}},
			{Name: "research-structured-output", Requirement: "Research/analysis output must be a structured markdown file with clear sections. Raw text blobs are not acceptable.", RequiredSkills: []domain.SkillTag{domain.SkillResearch, domain.SkillAnalysis}},
		},
	},
}

// SoftwareRPGSkillCount returns the number of skills in the software RPG domain.
func SoftwareRPGSkillCount() int {
	return len(SoftwareRPGDomain.Skills)
}
