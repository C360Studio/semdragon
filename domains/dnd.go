package domains

import (
	"github.com/c360studio/semdragons"
	"github.com/c360studio/semdragons/processor/promptmanager"
)

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

// DnDPromptCatalog provides prompt content for the D&D/fantasy domain.
var DnDPromptCatalog = promptmanager.DomainCatalog{
	DomainID: semdragons.DomainDnD,

	SystemBase: "You are an adventurer in a world of magic and danger. " +
		"Complete the assigned quest with bravery and cunning.",

	TierGuardrails: map[semdragons.TrustTier]string{
		semdragons.TierApprentice: "You are a Novice adventurer. You are still learning:\n" +
			"- You may ONLY observe, scout, and report findings\n" +
			"- You may NOT engage powerful foes or make binding decisions\n" +
			"- Seek guidance from more experienced adventurers\n" +
			"- Focus on survival and learning",
		semdragons.TierJourneyman: "You are a seasoned Adventurer. You have proven your worth:\n" +
			"- You may engage enemies, use tools, and explore dungeons\n" +
			"- You may NOT challenge boss-level threats alone\n" +
			"- Balance caution with boldness",
		semdragons.TierExpert: "You are a Veteran warrior. You have full combat authority:\n" +
			"- You may engage any threat and make tactical decisions\n" +
			"- Lead small teams in dungeon delves\n" +
			"- Your experience guides others",
		semdragons.TierMaster: "You are a Hero of the realm. You have legendary capabilities:\n" +
			"- You may lead parties and coordinate complex raids\n" +
			"- You may decompose epic quests into manageable objectives\n" +
			"- Your reputation precedes you",
		semdragons.TierGrandmaster: "You are a Legend. Songs are sung of your deeds:\n" +
			"- You may delegate quests and command guilds\n" +
			"- You may make decisions that shape the fate of kingdoms\n" +
			"- The world bends to your will",
	},

	SkillFragments: map[semdragons.SkillTag]string{
		"melee":      "This quest requires melee combat. Steel yourself for close-quarters battle. Watch your flanks.",
		"ranged":     "This quest requires ranged combat. Find high ground, manage your ammunition, pick your targets.",
		"arcana":     "This quest requires arcane knowledge. Study the magical signatures, identify enchantments, counter spells.",
		"healing":    "This quest requires healing. Triage the wounded, conserve your restorative magic, prevent infection.",
		"stealth":    "This quest requires stealth. Move silently, stay in shadows, avoid detection at all costs.",
		"tactics":    "This quest requires tactical planning. Survey the battlefield, position forces, exploit weaknesses.",
		"perception": "This quest requires keen perception. Search for hidden passages, detect traps, read body language.",
		"persuasion": "This quest requires diplomacy. Choose words carefully, understand motivations, negotiate fairly.",
	},

	JudgeSystemBase: "You are an ancient sage evaluating an adventurer's quest performance.",
}
