// Package componentregistry provides registration for all semdragons components.
// This is the central point for registering all processor components
// with the semstreams component registry.
package componentregistry

import (
	"github.com/c360studio/semstreams/component"
	graphgateway "github.com/c360studio/semstreams/gateway/graph-gateway"
	graphindex "github.com/c360studio/semstreams/processor/graph-index"
	graphingest "github.com/c360studio/semstreams/processor/graph-ingest"
	graphclustering "github.com/c360studio/semstreams/processor/graph-clustering"
	graphembedding "github.com/c360studio/semstreams/processor/graph-embedding"
	graphquery "github.com/c360studio/semstreams/processor/graph-query"
	wsinput "github.com/c360studio/semstreams/input/websocket"

	// Semstreams agentic processors
	agenticloop "github.com/c360studio/semstreams/processor/agentic-loop"
	agenticmodel "github.com/c360studio/semstreams/processor/agentic-model"

	// semsource payload registration — triggers init() to register the entity payload type
	_ "github.com/c360studio/semdragons/semsource"

	"github.com/c360studio/semdragons/storage/filestore"
	"github.com/c360studio/semdragons/storage/workspacerepo"

	"github.com/c360studio/semdragons/processor/agentprogression"
	"github.com/c360studio/semdragons/processor/agentstore"
	"github.com/c360studio/semdragons/processor/autonomy"
	"github.com/c360studio/semdragons/processor/boidengine"
	"github.com/c360studio/semdragons/processor/bossbattle"
	"github.com/c360studio/semdragons/processor/dmapproval"
	"github.com/c360studio/semdragons/processor/dmpartyformation"
	"github.com/c360studio/semdragons/processor/dmsession"
	"github.com/c360studio/semdragons/processor/dmworldstate"
	"github.com/c360studio/semdragons/processor/executor"
	"github.com/c360studio/semdragons/processor/guildformation"
	"github.com/c360studio/semdragons/processor/partycoord"
	"github.com/c360studio/semdragons/processor/questboard"
	"github.com/c360studio/semdragons/processor/questbridge"
	"github.com/c360studio/semdragons/processor/questdagexec"
	"github.com/c360studio/semdragons/processor/questtools"
	"github.com/c360studio/semdragons/processor/seeding"
)

// RegisterAll registers all semdragons components with the given registry.
// This is the main entry point for component registration.
func RegisterAll(registry *component.Registry) error {
	// Register semstreams graph processors FIRST.
	// These provide entity persistence, indexing, and query capabilities
	// that semdragons components depend on.
	graphProcessors := []func(*component.Registry) error{
		wsinput.Register,      // WebSocket input for semsource entity streaming
		graphingest.Register,  // Entity/triple ingestion and storage
		graphindex.Register,   // Relationship and predicate indexes
		graphquery.Register,      // Query coordination and PathRAG
		graphembedding.Register,  // Vector embeddings (BM25 or HTTP)
		graphclustering.Register, // Community detection and structural analysis
		graphgateway.Register,    // GraphQL/MCP HTTP gateway
	}

	for _, register := range graphProcessors {
		if err := register(registry); err != nil {
			return err
		}
	}

	// Register semstreams agentic processors.
	// These provide event-driven LLM loop orchestration and model routing
	// that questbridge and questtools depend on.
	if err := agenticloop.Register(registry); err != nil {
		return err
	}
	// agenticmodel.Register takes RegistryInterface (satisfied by *component.Registry)
	if err := agenticmodel.Register(registry); err != nil {
		return err
	}

	// Register storage components.
	// filestore.Register takes RegistryInterface (satisfied by *component.Registry)
	if err := filestore.Register(registry); err != nil {
		return err
	}
	// workspacerepo.Register takes RegistryInterface (satisfied by *component.Registry)
	if err := workspacerepo.Register(registry); err != nil {
		return err
	}

	// Register semdragons processor components
	processors := []func(*component.Registry) error{
		questboard.Register,
		agentprogression.Register,
		agentstore.Register,
		autonomy.Register,
		bossbattle.Register,
		boidengine.Register,
		executor.Register,
		guildformation.Register,
		partycoord.Register,
		seeding.Register,
		dmsession.Register,
		dmapproval.Register,
		dmworldstate.Register,
		dmpartyformation.Register,
		questbridge.Register,
		questdagexec.Register,
		questtools.Register,
	}

	for _, register := range processors {
		if err := register(registry); err != nil {
			return err
		}
	}

	return nil
}

// RegisterProcessors registers only the processor components.
func RegisterProcessors(registry *component.Registry) error {
	// Register semstreams graph processors first
	graphProcessors := []func(*component.Registry) error{
		wsinput.Register,
		graphingest.Register,
		graphindex.Register,
		graphquery.Register,
		graphgateway.Register,
	}

	for _, register := range graphProcessors {
		if err := register(registry); err != nil {
			return err
		}
	}

	// Register semstreams agentic processors
	if err := agenticloop.Register(registry); err != nil {
		return err
	}
	if err := agenticmodel.Register(registry); err != nil {
		return err
	}

	// Register storage components.
	if err := filestore.Register(registry); err != nil {
		return err
	}
	if err := workspacerepo.Register(registry); err != nil {
		return err
	}

	// Register semdragons processors
	processors := []func(*component.Registry) error{
		questboard.Register,
		agentprogression.Register,
		agentstore.Register,
		autonomy.Register,
		bossbattle.Register,
		boidengine.Register,
		executor.Register,
		guildformation.Register,
		partycoord.Register,
		seeding.Register,
		dmsession.Register,
		dmapproval.Register,
		dmworldstate.Register,
		dmpartyformation.Register,
		questbridge.Register,
		questdagexec.Register,
		questtools.Register,
	}

	for _, register := range processors {
		if err := register(registry); err != nil {
			return err
		}
	}

	return nil
}

// ComponentNames returns the names of all registered components.
func ComponentNames() []string {
	return []string{
		// Semstreams graph processors
		"websocket_input",
		"graph-ingest",
		"graph-index",
		"graph-query",
		"graph-embedding",
		"graph-clustering",
		"graph-gateway",
		// Semstreams agentic processors
		"agentic-loop",
		"agentic-model",
		// Semdragons storage
		"filestore",
		workspacerepo.ComponentName,
		// Semdragons processors
		questboard.ComponentName,
		agentprogression.ComponentName,
		agentstore.ComponentName,
		autonomy.ComponentName,
		bossbattle.ComponentName,
		boidengine.ComponentName,
		executor.ComponentName,
		guildformation.ComponentName,
		partycoord.ComponentName,
		seeding.ComponentName,
		dmsession.ComponentName,
		dmapproval.ComponentName,
		dmworldstate.ComponentName,
		dmpartyformation.ComponentName,
		questbridge.ComponentName,
		questdagexec.ComponentName,
		questtools.ComponentName,
	}
}

// ProcessorNames returns the names of processor components.
func ProcessorNames() []string {
	return []string{
		// Semstreams graph processors
		"websocket_input",
		"graph-ingest",
		"graph-index",
		"graph-query",
		"graph-embedding",
		"graph-clustering",
		"graph-gateway",
		// Semstreams agentic processors
		"agentic-loop",
		"agentic-model",
		// Semdragons storage
		"filestore",
		workspacerepo.ComponentName,
		// Semdragons processors
		questboard.ComponentName,
		agentprogression.ComponentName,
		agentstore.ComponentName,
		autonomy.ComponentName,
		bossbattle.ComponentName,
		boidengine.ComponentName,
		executor.ComponentName,
		guildformation.ComponentName,
		partycoord.ComponentName,
		seeding.ComponentName,
		dmsession.ComponentName,
		dmapproval.ComponentName,
		dmworldstate.ComponentName,
		dmpartyformation.ComponentName,
		questbridge.ComponentName,
		questdagexec.ComponentName,
		questtools.ComponentName,
	}
}
