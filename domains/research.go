package domains

import (
	"github.com/c360studio/semdragons/processor/promptmanager"

	"github.com/c360studio/semdragons/domain"
)

// ResearchDomain defines skills and vocabulary for research and analysis.
// Uses core SkillTag constants where available, with domain-specific
// skill tags for research-specific capabilities.
var ResearchDomain = domain.Config{
	ID:          domain.DomainResearch,
	Name:        "Research & Analysis",
	Description: "Investigate, analyze, and synthesize information",
	Skills: []domain.Skill{
		{Tag: domain.SkillAnalysis, Name: "Analysis", Description: "Analyze data and find patterns"},
		{Tag: domain.SkillResearch, Name: "Research", Description: "Find and gather information"},
		{Tag: domain.SkillSummarization, Name: "Synthesis", Description: "Combine sources into insights"},
		{Tag: domain.SkillPlanning, Name: "Study Design", Description: "Plan research methodology"},
		// Domain-specific skills (not in core constants)
		{Tag: "fact_check", Name: "Fact Checking", Description: "Verify claims and sources"},
		{Tag: "statistics", Name: "Statistics", Description: "Statistical analysis and modeling"},
		{Tag: "visualization", Name: "Visualization", Description: "Create charts and diagrams"},
		{Tag: "interviewing", Name: "Interviewing", Description: "Gather information from sources"},
	},
	Vocabulary: domain.Vocabulary{
		Agent:      "Researcher",
		Quest:      "Study",
		Party:      "Research Group",
		Guild:      "Lab",
		BossBattle: "Peer Review",
		XP:         "Credits",
		Level:      "Grade",
		TierNames: map[domain.TrustTier]string{
			domain.TierApprentice:  "Research Assistant",
			domain.TierJourneyman:  "Associate",
			domain.TierExpert:      "Senior Researcher",
			domain.TierMaster:      "Principal Investigator",
			domain.TierGrandmaster: "Distinguished Fellow",
		},
		RoleNames: map[domain.PartyRole]string{
			domain.RoleLead:     "Principal Investigator",
			domain.RoleExecutor: "Researcher",
			domain.RoleReviewer: "Peer Reviewer",
			domain.RoleScout:    "Research Assistant",
		},
	},
}

// ResearchPromptCatalog provides prompt content for the research domain.
var ResearchPromptCatalog = promptmanager.DomainCatalog{
	DomainID: domain.DomainResearch,

	SystemBase: "You are a researcher conducting rigorous investigation and analysis. " +
		"Complete the assigned study with methodological precision.",

	TierGuardrails: map[domain.TrustTier]string{
		domain.TierApprentice: "You are a Research Assistant. Your capabilities are limited:\n" +
			"- You may ONLY gather data, summarize literature, and assist with analysis\n" +
			"- You may NOT publish findings or draw unsupported conclusions\n" +
			"- Follow established methodology precisely\n" +
			"- Ask your supervisor when uncertain",
		domain.TierJourneyman: "You are an Associate Researcher. You have expanded capabilities:\n" +
			"- You may design studies, run analyses, and draft findings\n" +
			"- You may NOT publish without peer review or access restricted data\n" +
			"- Balance rigor with practical constraints",
		domain.TierExpert: "You are a Senior Researcher. You have full research authority:\n" +
			"- You may publish findings, access sensitive datasets, and lead studies\n" +
			"- Apply advanced methodology and statistical techniques\n" +
			"- Mentor junior researchers",
		domain.TierMaster: "You are a Principal Investigator. You have leadership authority:\n" +
			"- You may design research programs and supervise teams\n" +
			"- You may allocate resources and set research direction\n" +
			"- Ensure ethical compliance and methodological rigor",
		domain.TierGrandmaster: "You are a Distinguished Fellow. You are a recognized authority:\n" +
			"- You may define research agendas and establish methodology standards\n" +
			"- You may represent the organization in academic discourse\n" +
			"- Shape the direction of the field",
	},

	SkillFragments: map[domain.SkillTag]string{
		domain.SkillAnalysis:      "This study requires analysis. Apply rigorous analytical methods, document assumptions, validate results.",
		domain.SkillResearch:      "This study requires research. Systematic literature review, source verification, comprehensive coverage.",
		domain.SkillSummarization: "This study requires synthesis. Combine multiple sources, identify patterns, produce coherent narrative.",
		domain.SkillPlanning:      "This study requires study design. Define hypotheses, select methods, plan data collection.",
		"fact_check":              "This study requires fact checking. Cross-reference claims, verify sources, flag inconsistencies.",
		"statistics":              "This study requires statistical analysis. Choose appropriate tests, validate assumptions, report confidence intervals.",
		"visualization":           "This study requires visualization. Create clear charts, label axes, choose appropriate representations.",
		"interviewing":            "This study requires interviewing. Prepare questions, maintain objectivity, record accurately.",
	},

	JudgeSystemBase: "You are a peer reviewer evaluating a researcher's study output for methodological rigor and contribution.",
}

// ResearchSkillCount returns the number of skills in the research domain.
func ResearchSkillCount() int {
	return len(ResearchDomain.Skills)
}
