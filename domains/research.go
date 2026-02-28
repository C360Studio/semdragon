package domains

import "github.com/c360studio/semdragons"

// ResearchDomain defines skills and vocabulary for research and analysis.
// Uses core SkillTag constants where available, with domain-specific
// skill tags for research-specific capabilities.
var ResearchDomain = semdragons.DomainConfig{
	ID:          semdragons.DomainResearch,
	Name:        "Research & Analysis",
	Description: "Investigate, analyze, and synthesize information",
	Skills: []semdragons.DomainSkill{
		{Tag: semdragons.SkillAnalysis, Name: "Analysis", Description: "Analyze data and find patterns"},
		{Tag: semdragons.SkillResearch, Name: "Research", Description: "Find and gather information"},
		{Tag: semdragons.SkillSummarization, Name: "Synthesis", Description: "Combine sources into insights"},
		{Tag: semdragons.SkillPlanning, Name: "Study Design", Description: "Plan research methodology"},
		// Domain-specific skills (not in core constants)
		{Tag: "fact_check", Name: "Fact Checking", Description: "Verify claims and sources"},
		{Tag: "statistics", Name: "Statistics", Description: "Statistical analysis and modeling"},
		{Tag: "visualization", Name: "Visualization", Description: "Create charts and diagrams"},
		{Tag: "interviewing", Name: "Interviewing", Description: "Gather information from sources"},
	},
	Vocabulary: semdragons.DomainVocabulary{
		Agent:      "Researcher",
		Quest:      "Study",
		Party:      "Research Group",
		Guild:      "Lab",
		BossBattle: "Peer Review",
		XP:         "Credits",
		Level:      "Grade",
		TierNames: map[semdragons.TrustTier]string{
			semdragons.TierApprentice:  "Research Assistant",
			semdragons.TierJourneyman:  "Associate",
			semdragons.TierExpert:      "Senior Researcher",
			semdragons.TierMaster:      "Principal Investigator",
			semdragons.TierGrandmaster: "Distinguished Fellow",
		},
		RoleNames: map[semdragons.PartyRole]string{
			semdragons.RoleLead:     "Principal Investigator",
			semdragons.RoleExecutor: "Researcher",
			semdragons.RoleReviewer: "Peer Reviewer",
			semdragons.RoleScout:    "Research Assistant",
		},
	},
}

// ResearchSkillCount returns the number of skills in the research domain.
func ResearchSkillCount() int {
	return len(ResearchDomain.Skills)
}
