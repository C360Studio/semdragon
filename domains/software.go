package domains

import "github.com/c360studio/semdragons"

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

// SoftwareSkillCount returns the number of skills in the software domain.
func SoftwareSkillCount() int {
	return len(SoftwareDomain.Skills)
}
