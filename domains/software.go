package domains

import "github.com/c360studio/semdragons"

// SoftwareDomain defines skills and vocabulary for software development.
var SoftwareDomain = semdragons.DomainConfig{
	ID:          semdragons.DomainSoftware,
	Name:        "Software Development",
	Description: "Build, test, and deploy software systems",
	Skills: []semdragons.DomainSkill{
		{Tag: "code_gen", Name: "Coding", Description: "Write and generate code"},
		{Tag: "testing", Name: "Testing", Description: "Write and run tests"},
		{Tag: "code_review", Name: "Code Review", Description: "Review code quality"},
		{Tag: "architecture", Name: "Architecture", Description: "System design"},
		{Tag: "devops", Name: "DevOps", Description: "Deployment and operations"},
		{Tag: "debugging", Name: "Debugging", Description: "Find and fix bugs"},
		{Tag: "documentation", Name: "Documentation", Description: "Write technical docs"},
		{Tag: "planning", Name: "Planning", Description: "Technical planning and estimation"},
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
