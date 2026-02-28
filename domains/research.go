package domains

import "github.com/c360studio/semdragons"

// ResearchDomain defines skills and vocabulary for research and analysis.
var ResearchDomain = semdragons.DomainConfig{
	ID:          semdragons.DomainResearch,
	Name:        "Research & Analysis",
	Description: "Investigate, analyze, and synthesize information",
	Skills: []semdragons.DomainSkill{
		{Tag: "analysis", Name: "Analysis", Description: "Analyze data and find patterns"},
		{Tag: "research", Name: "Research", Description: "Find and gather information"},
		{Tag: "synthesis", Name: "Synthesis", Description: "Combine sources into insights"},
		{Tag: "fact_check", Name: "Fact Checking", Description: "Verify claims and sources"},
		{Tag: "writing", Name: "Writing", Description: "Document and communicate findings"},
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
