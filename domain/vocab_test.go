package domain

import (
	"reflect"
	"strings"
	"testing"

	"github.com/c360studio/semdragons/internal/semcompat"
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

var executionPredicateContract = []string{
	"execution.lifecycle.started",
	"execution.agent.id",
	"execution.loop.id",
	"execution.config.max-turns",
	"execution.config.max-tokens",
	"execution.tools.available",
	"execution.lifecycle.completed",
	"execution.status.final",
	"execution.metrics.turns",
	"execution.metrics.tool-calls",
	"execution.tokens.prompt",
	"execution.tokens.completion",
	"execution.duration.ms",
	"execution.lifecycle.failed",
	"execution.failure.error",
}

var sourceDocumentPredicateContract = []string{
	"source.doc.content",
	"source.doc.summary",
	"source.doc.url",
	"source.doc.mime-type",
	"source.doc.scope",
	"source.doc.fetched-by",
	"source.doc.quest-id",
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

func TestExecutorPredicateVocabularyContract(t *testing.T) {
	restore := vocabulary.SnapshotRegistry()
	t.Cleanup(restore)
	vocabulary.ClearRegistry()
	RegisterVocabulary()

	if got := len(executionPredicateContract); got != 15 {
		t.Fatalf("execution predicate contract contains %d names, want 15", got)
	}
	if got := len(sourceDocumentPredicateContract); got != 7 {
		t.Fatalf("source document predicate contract contains %d names, want 7", got)
	}
	executionConstants := []string{
		PredicateExecutionStarted,
		PredicateExecutionAgentID,
		PredicateExecutionLoopID,
		PredicateExecutionConfigMaxTurns,
		PredicateExecutionConfigMaxTokens,
		PredicateExecutionToolsAvailable,
		PredicateExecutionCompleted,
		PredicateExecutionStatusFinal,
		PredicateExecutionMetricsTurns,
		PredicateExecutionMetricsToolCalls,
		PredicateExecutionTokensPrompt,
		PredicateExecutionTokensCompletion,
		PredicateExecutionDurationMS,
		PredicateExecutionFailed,
		PredicateExecutionFailureError,
	}
	if !reflect.DeepEqual(executionConstants, executionPredicateContract) {
		t.Fatalf("execution predicate constants = %#v, want exact contract %#v", executionConstants, executionPredicateContract)
	}
	sourceDocumentConstants := []string{
		PredicateSourceDocumentContent,
		PredicateSourceDocumentSummary,
		PredicateSourceDocumentURL,
		PredicateSourceDocumentMIMEType,
		PredicateSourceDocumentScope,
		PredicateSourceDocumentFetchedBy,
		PredicateSourceDocumentQuestID,
	}
	if !reflect.DeepEqual(sourceDocumentConstants, sourceDocumentPredicateContract) {
		t.Fatalf("source document predicate constants = %#v, want exact contract %#v", sourceDocumentConstants, sourceDocumentPredicateContract)
	}
	for _, predicate := range append(executionPredicateContract, sourceDocumentPredicateContract...) {
		if strings.Contains(predicate, "_") {
			t.Errorf("product predicate %q contains a legacy underscore", predicate)
		}
		metadata := vocabulary.GetPredicateMetadata(predicate)
		if metadata == nil {
			t.Errorf("product predicate %q is not registered", predicate)
		}
	}
	if vocabulary.DCTermsTitle != "dc.terms.title" {
		t.Errorf("framework DCTermsTitle = %q, want dc.terms.title", vocabulary.DCTermsTitle)
	}
	if semcompat.DCTermsModified != "dc.terms.modified" {
		t.Errorf("beta.135 DCTermsModified = %q, want dc.terms.modified", semcompat.DCTermsModified)
	}
	modified := vocabulary.GetPredicateMetadata(semcompat.DCTermsModified)
	if modified == nil {
		t.Fatal("beta.135 dc.terms.modified compatibility predicate is not registered")
	}
	if !strings.Contains(modified.Description, "SemStreams #673") {
		t.Errorf("dc.terms.modified description = %q, want explicit SemStreams #673 linkage", modified.Description)
	}
}
