package domains

import "github.com/c360studio/semdragons"

// DnDDomain defines skills and vocabulary for D&D/fantasy settings.
// NOTE: This domain uses fantasy-specific skill tags that don't map to
// the core SkillTag constants. This is intentional - different domains
// can define completely different skill sets.
var DnDDomain = semdragons.DomainConfig{
	ID:          semdragons.DomainDnD,
	Name:        "Dungeons & Dragons",
	Description: "Classic fantasy adventure setting",
	Skills: []semdragons.DomainSkill{
		{Tag: "melee", Name: "Melee Combat", Description: "Sword and axe fighting"},
		{Tag: "ranged", Name: "Ranged Combat", Description: "Bows and thrown weapons"},
		{Tag: "arcana", Name: "Arcana", Description: "Magical knowledge and spells"},
		{Tag: "healing", Name: "Healing", Description: "Restore health and cure ailments"},
		{Tag: "stealth", Name: "Stealth", Description: "Move unseen and unheard"},
		{Tag: "tactics", Name: "Tactics", Description: "Battle strategy and leadership"},
		{Tag: "perception", Name: "Perception", Description: "Notice hidden details"},
		{Tag: "persuasion", Name: "Persuasion", Description: "Convince and negotiate"},
	},
	Vocabulary: semdragons.DomainVocabulary{
		Agent:      "Adventurer",
		Quest:      "Quest",
		Party:      "Party",
		Guild:      "Guild",
		BossBattle: "Boss Battle",
		XP:         "XP",
		Level:      "Level",
		TierNames: map[semdragons.TrustTier]string{
			semdragons.TierApprentice:  "Novice",
			semdragons.TierJourneyman:  "Adventurer",
			semdragons.TierExpert:      "Veteran",
			semdragons.TierMaster:      "Hero",
			semdragons.TierGrandmaster: "Legend",
		},
		RoleNames: map[semdragons.PartyRole]string{
			semdragons.RoleLead:     "Party Leader",
			semdragons.RoleExecutor: "Champion",
			semdragons.RoleReviewer: "Sage",
			semdragons.RoleScout:    "Scout",
		},
	},
}
