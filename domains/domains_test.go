package domains

import (
	"fmt"
	"testing"

	"github.com/c360studio/semdragons"
)

// allTrustTiers enumerates every defined TrustTier constant.
var allTrustTiers = []semdragons.TrustTier{
	semdragons.TierApprentice,
	semdragons.TierJourneyman,
	semdragons.TierExpert,
	semdragons.TierMaster,
	semdragons.TierGrandmaster,
}

// allPartyRoles enumerates every defined PartyRole constant.
var allPartyRoles = []semdragons.PartyRole{
	semdragons.RoleLead,
	semdragons.RoleExecutor,
	semdragons.RoleReviewer,
	semdragons.RoleScout,
}

// domainEntry bundles a DomainConfig with its matching DomainCatalog for
// table-driven tests that exercise both together.
type domainEntry struct {
	name    string
	config  semdragons.DomainConfig
	catalog *catalogEntry
}

// catalogEntry holds the promptmanager fields we care about without importing
// promptmanager (the domains package already imports it; we re-use the vars).
type catalogEntry struct {
	domainID        semdragons.DomainID
	systemBase      string
	judgeSystemBase string
	tierGuardrails  map[semdragons.TrustTier]string
	skillFragments  map[semdragons.SkillTag]string
}

// allDomains is the single source of truth for all three concrete domains.
var allDomains = []domainEntry{
	{
		name:   "software",
		config: SoftwareDomain,
		catalog: &catalogEntry{
			domainID:        SoftwarePromptCatalog.DomainID,
			systemBase:      SoftwarePromptCatalog.SystemBase,
			judgeSystemBase: SoftwarePromptCatalog.JudgeSystemBase,
			tierGuardrails:  SoftwarePromptCatalog.TierGuardrails,
			skillFragments:  SoftwarePromptCatalog.SkillFragments,
		},
	},
	{
		name:   "dnd",
		config: DnDDomain,
		catalog: &catalogEntry{
			domainID:        DnDPromptCatalog.DomainID,
			systemBase:      DnDPromptCatalog.SystemBase,
			judgeSystemBase: DnDPromptCatalog.JudgeSystemBase,
			tierGuardrails:  DnDPromptCatalog.TierGuardrails,
			skillFragments:  DnDPromptCatalog.SkillFragments,
		},
	},
	{
		name:   "research",
		config: ResearchDomain,
		catalog: &catalogEntry{
			domainID:        ResearchPromptCatalog.DomainID,
			systemBase:      ResearchPromptCatalog.SystemBase,
			judgeSystemBase: ResearchPromptCatalog.JudgeSystemBase,
			tierGuardrails:  ResearchPromptCatalog.TierGuardrails,
			skillFragments:  ResearchPromptCatalog.SkillFragments,
		},
	},
}

// =============================================================================
// 1. Domain config completeness
// =============================================================================

func TestDomainConfig_HasRequiredFields(t *testing.T) {
	for _, d := range allDomains {
		t.Run(d.name, func(t *testing.T) {
			if d.config.ID == "" {
				t.Error("ID must not be empty")
			}
			if d.config.Name == "" {
				t.Error("Name must not be empty")
			}
			if d.config.Description == "" {
				t.Error("Description must not be empty")
			}
		})
	}
}

// =============================================================================
// 2. Skill pool coverage
// =============================================================================

func TestDomainSkills_NonEmpty(t *testing.T) {
	for _, d := range allDomains {
		t.Run(d.name, func(t *testing.T) {
			if len(d.config.Skills) == 0 {
				t.Errorf("domain %q must define at least one skill", d.name)
			}
		})
	}
}

func TestDomainSkills_NoEmptyTags(t *testing.T) {
	for _, d := range allDomains {
		t.Run(d.name, func(t *testing.T) {
			for i, skill := range d.config.Skills {
				if skill.Tag == "" {
					t.Errorf("skill[%d] has an empty Tag", i)
				}
				if skill.Name == "" {
					t.Errorf("skill[%d] (tag=%q) has an empty Name", i, skill.Tag)
				}
				if skill.Description == "" {
					t.Errorf("skill[%d] (tag=%q) has an empty Description", i, skill.Tag)
				}
			}
		})
	}
}

func TestDomainSkills_NoDuplicateTags(t *testing.T) {
	for _, d := range allDomains {
		t.Run(d.name, func(t *testing.T) {
			seen := make(map[semdragons.SkillTag]int)
			for _, skill := range d.config.Skills {
				seen[skill.Tag]++
			}
			for tag, count := range seen {
				if count > 1 {
					t.Errorf("skill tag %q appears %d times (must be unique)", tag, count)
				}
			}
		})
	}
}

// =============================================================================
// 3. Vocabulary completeness
// =============================================================================

func TestDomainVocabulary_KeyTermsNonEmpty(t *testing.T) {
	// These are the seven vocabulary terms callers depend on via Vocabulary.Get.
	type vocabCheck struct {
		key   string
		value string
	}

	for _, d := range allDomains {
		t.Run(d.name, func(t *testing.T) {
			checks := []vocabCheck{
				{"agent", d.config.Vocabulary.Agent},
				{"quest", d.config.Vocabulary.Quest},
				{"party", d.config.Vocabulary.Party},
				{"guild", d.config.Vocabulary.Guild},
				{"boss_battle", d.config.Vocabulary.BossBattle},
				{"xp", d.config.Vocabulary.XP},
				{"level", d.config.Vocabulary.Level},
			}
			for _, c := range checks {
				if c.value == "" {
					t.Errorf("vocabulary key %q must not be empty", c.key)
				}
			}
		})
	}
}

// =============================================================================
// 4. Cross-domain uniqueness
// =============================================================================

func TestDomains_UniqueIDs(t *testing.T) {
	seen := make(map[semdragons.DomainID]string)
	for _, d := range allDomains {
		if prev, exists := seen[d.config.ID]; exists {
			t.Errorf("domain ID %q is used by both %q and %q", d.config.ID, prev, d.name)
		}
		seen[d.config.ID] = d.name
	}
}

func TestDomains_UniqueNames(t *testing.T) {
	seen := make(map[string]string)
	for _, d := range allDomains {
		if prev, exists := seen[d.config.Name]; exists {
			t.Errorf("domain Name %q is used by both %q and %q", d.config.Name, prev, d.name)
		}
		seen[d.config.Name] = d.name
	}
}

// =============================================================================
// 5. Tier names coverage
// =============================================================================

func TestDomainVocabulary_TierNamesCoversAllTiers(t *testing.T) {
	for _, d := range allDomains {
		t.Run(d.name, func(t *testing.T) {
			if d.config.Vocabulary.TierNames == nil {
				// TierNames is optional; skip if not defined.
				t.Skip("TierNames not defined for this domain")
			}
			for _, tier := range allTrustTiers {
				name, ok := d.config.Vocabulary.TierNames[tier]
				if !ok {
					t.Errorf("TierNames missing entry for tier %v", tier)
					continue
				}
				if name == "" {
					t.Errorf("TierNames[%v] must not be empty", tier)
				}
			}
		})
	}
}

func TestDomainVocabulary_TierNamesHasNoExtraEntries(t *testing.T) {
	validTiers := make(map[semdragons.TrustTier]struct{})
	for _, tier := range allTrustTiers {
		validTiers[tier] = struct{}{}
	}

	for _, d := range allDomains {
		t.Run(d.name, func(t *testing.T) {
			if d.config.Vocabulary.TierNames == nil {
				t.Skip("TierNames not defined for this domain")
			}
			for tier := range d.config.Vocabulary.TierNames {
				if _, ok := validTiers[tier]; !ok {
					t.Errorf("TierNames contains unknown tier %v", tier)
				}
			}
		})
	}
}

// =============================================================================
// 6. Role names coverage
// =============================================================================

func TestDomainVocabulary_RoleNamesCoversAllRoles(t *testing.T) {
	for _, d := range allDomains {
		t.Run(d.name, func(t *testing.T) {
			if d.config.Vocabulary.RoleNames == nil {
				t.Skip("RoleNames not defined for this domain")
			}
			for _, role := range allPartyRoles {
				name, ok := d.config.Vocabulary.RoleNames[role]
				if !ok {
					t.Errorf("RoleNames missing entry for role %q", role)
					continue
				}
				if name == "" {
					t.Errorf("RoleNames[%q] must not be empty", role)
				}
			}
		})
	}
}

func TestDomainVocabulary_RoleNamesHasNoExtraEntries(t *testing.T) {
	validRoles := make(map[semdragons.PartyRole]struct{})
	for _, role := range allPartyRoles {
		validRoles[role] = struct{}{}
	}

	for _, d := range allDomains {
		t.Run(d.name, func(t *testing.T) {
			if d.config.Vocabulary.RoleNames == nil {
				t.Skip("RoleNames not defined for this domain")
			}
			for role := range d.config.Vocabulary.RoleNames {
				if _, ok := validRoles[role]; !ok {
					t.Errorf("RoleNames contains unknown role %q", role)
				}
			}
		})
	}
}

// =============================================================================
// 7. Catalog tests
// =============================================================================

func TestDomainCatalog_SystemBaseAndJudgeNonEmpty(t *testing.T) {
	for _, d := range allDomains {
		t.Run(d.name, func(t *testing.T) {
			if d.catalog == nil {
				t.Skip("no catalog registered for this domain")
			}
			if d.catalog.systemBase == "" {
				t.Error("SystemBase must not be empty")
			}
			if d.catalog.judgeSystemBase == "" {
				t.Error("JudgeSystemBase must not be empty")
			}
		})
	}
}

func TestDomainCatalog_DomainIDMatchesConfig(t *testing.T) {
	for _, d := range allDomains {
		t.Run(d.name, func(t *testing.T) {
			if d.catalog == nil {
				t.Skip("no catalog registered for this domain")
			}
			if d.catalog.domainID != d.config.ID {
				t.Errorf("catalog DomainID %q does not match config ID %q",
					d.catalog.domainID, d.config.ID)
			}
		})
	}
}

func TestDomainCatalog_TierGuardrailsCoversAllTiers(t *testing.T) {
	for _, d := range allDomains {
		t.Run(d.name, func(t *testing.T) {
			if d.catalog == nil {
				t.Skip("no catalog registered for this domain")
			}
			if len(d.catalog.tierGuardrails) == 0 {
				t.Error("TierGuardrails must have at least one entry")
			}
			for _, tier := range allTrustTiers {
				guardrail, ok := d.catalog.tierGuardrails[tier]
				if !ok {
					t.Errorf("TierGuardrails missing entry for tier %v", tier)
					continue
				}
				if guardrail == "" {
					t.Errorf("TierGuardrails[%v] must not be empty", tier)
				}
			}
		})
	}
}

func TestDomainCatalog_SkillFragmentsCoversAllDomainSkills(t *testing.T) {
	for _, d := range allDomains {
		t.Run(d.name, func(t *testing.T) {
			if d.catalog == nil {
				t.Skip("no catalog registered for this domain")
			}
			if len(d.catalog.skillFragments) == 0 {
				t.Error("SkillFragments must have at least one entry")
			}
			for _, skill := range d.config.Skills {
				fragment, ok := d.catalog.skillFragments[skill.Tag]
				if !ok {
					t.Errorf("SkillFragments missing entry for skill tag %q", skill.Tag)
					continue
				}
				if fragment == "" {
					t.Errorf("SkillFragments[%q] must not be empty", skill.Tag)
				}
			}
		})
	}
}

func TestDomainCatalog_SkillFragmentsAllNonEmpty(t *testing.T) {
	for _, d := range allDomains {
		t.Run(d.name, func(t *testing.T) {
			if d.catalog == nil {
				t.Skip("no catalog registered for this domain")
			}
			for tag, fragment := range d.catalog.skillFragments {
				if fragment == "" {
					t.Errorf("SkillFragments[%q] is empty", tag)
				}
			}
		})
	}
}

// =============================================================================
// Helper function tests
// =============================================================================

func TestSoftwareSkillCount(t *testing.T) {
	got := SoftwareSkillCount()
	want := len(SoftwareDomain.Skills)
	if got != want {
		t.Errorf("SoftwareSkillCount() = %d, want %d", got, want)
	}
	if got == 0 {
		t.Error("SoftwareSkillCount() must be greater than zero")
	}
}

func TestResearchSkillCount(t *testing.T) {
	got := ResearchSkillCount()
	want := len(ResearchDomain.Skills)
	if got != want {
		t.Errorf("ResearchSkillCount() = %d, want %d", got, want)
	}
	if got == 0 {
		t.Error("ResearchSkillCount() must be greater than zero")
	}
}

// =============================================================================
// Vocabulary.Get integration
// =============================================================================

func TestDomainVocabulary_GetReturnsExpectedValues(t *testing.T) {
	// Spot-check that Vocabulary.Get returns the configured value for each
	// domain — this exercises the Get switch in domain/config.go.
	cases := []struct {
		domain   string
		vocab    semdragons.DomainVocabulary
		key      string
		wantText string
	}{
		{"software", SoftwareDomain.Vocabulary, "agent", "Developer"},
		{"software", SoftwareDomain.Vocabulary, "quest", "Task"},
		{"software", SoftwareDomain.Vocabulary, "xp", "Points"},
		{"dnd", DnDDomain.Vocabulary, "agent", "Adventurer"},
		{"dnd", DnDDomain.Vocabulary, "boss_battle", "Boss Battle"},
		{"dnd", DnDDomain.Vocabulary, "level", "Level"},
		{"research", ResearchDomain.Vocabulary, "agent", "Researcher"},
		{"research", ResearchDomain.Vocabulary, "boss_battle", "Peer Review"},
		{"research", ResearchDomain.Vocabulary, "guild", "Lab"},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s/%s", tc.domain, tc.key), func(t *testing.T) {
			got := tc.vocab.Get(tc.key)
			if got != tc.wantText {
				t.Errorf("Get(%q) = %q, want %q", tc.key, got, tc.wantText)
			}
		})
	}
}

// TestDomainVocabulary_GetFallsBackToDefault verifies that Get returns the
// default vocabulary when the domain does not override a key.  We construct a
// minimal vocabulary with only Agent set so the other keys fall through.
func TestDomainVocabulary_GetFallsBackToDefault(t *testing.T) {
	partial := semdragons.DomainVocabulary{
		Agent: "Tester",
		// Quest, Party, etc. intentionally left blank.
	}

	// "quest" has no override — must return the library default.
	got := partial.Get("quest")
	if got != "Quest" {
		t.Errorf("Get(\"quest\") on partial vocab = %q, want %q", got, "Quest")
	}

	// "agent" IS overridden.
	got = partial.Get("agent")
	if got != "Tester" {
		t.Errorf("Get(\"agent\") on partial vocab = %q, want %q", got, "Tester")
	}
}
