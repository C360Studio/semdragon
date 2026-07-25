package domain

import (
	"reflect"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/vocabulary"
)

var autonomyPredicateContract = []string{
	"agent.autonomy.status",
	"agent.autonomy.action",
	"agent.autonomy.interval-ms",
	"agent.autonomy.idle-duration-ms",
	"agent.autonomy.has-suggestion",
	"agent.autonomy.backoff-ms",
	"agent.autonomy.claim-quest",
	"agent.autonomy.claim-score",
	"agent.autonomy.claim-rank",
	"agent.autonomy.shop-item",
	"agent.autonomy.shop-cost",
	"agent.autonomy.shop-budget",
	"agent.autonomy.shop-strategic",
	"agent.autonomy.guild-join",
	"agent.autonomy.guild-score",
	"agent.autonomy.guild-choices",
	"agent.autonomy.use-consumable",
	"agent.autonomy.use-status",
	"agent.autonomy.use-quest",
	"agent.autonomy.guild-create",
	"agent.autonomy.guild-create-candidates",
}

func TestAutonomyPredicateVocabularyContract(t *testing.T) {
	restore := vocabulary.SnapshotRegistry()
	t.Cleanup(restore)
	vocabulary.ClearRegistry()
	RegisterVocabulary()

	if got := len(autonomyPredicateContract); got != 21 {
		t.Fatalf("autonomy predicate contract contains %d names, want 21", got)
	}
	constants := []string{
		PredicateAutonomyStatus,
		PredicateAutonomyAction,
		PredicateAutonomyIntervalMS,
		PredicateAutonomyIdleDurationMS,
		PredicateAutonomyHasSuggestion,
		PredicateAutonomyBackoffMS,
		PredicateAutonomyClaimQuest,
		PredicateAutonomyClaimScore,
		PredicateAutonomyClaimRank,
		PredicateAutonomyShopItem,
		PredicateAutonomyShopCost,
		PredicateAutonomyShopBudget,
		PredicateAutonomyShopStrategic,
		PredicateAutonomyGuildJoin,
		PredicateAutonomyGuildScore,
		PredicateAutonomyGuildChoices,
		PredicateAutonomyUseConsumable,
		PredicateAutonomyUseStatus,
		PredicateAutonomyUseQuest,
		PredicateAutonomyGuildCreate,
		PredicateAutonomyGuildCreateCandidates,
	}
	if !reflect.DeepEqual(constants, autonomyPredicateContract) {
		t.Fatalf("autonomy predicate constants = %#v, want exact contract %#v", constants, autonomyPredicateContract)
	}
	for _, predicate := range autonomyPredicateContract {
		parts := strings.Split(predicate, ".")
		if len(parts) != 3 || parts[0] != "agent" || parts[1] != "autonomy" {
			t.Errorf("autonomy predicate %q is not an exact three-segment product name", predicate)
		}
		if strings.Contains(predicate, "_") {
			t.Errorf("autonomy predicate %q contains a legacy underscore", predicate)
		}
		metadata := vocabulary.GetPredicateMetadata(predicate)
		if metadata == nil {
			t.Errorf("autonomy predicate %q is not registered in the product vocabulary", predicate)
			continue
		}
		if metadata.Name != predicate {
			t.Errorf("registered predicate name = %q, want %q", metadata.Name, predicate)
		}
	}
}
