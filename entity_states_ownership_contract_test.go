package semdragons_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEntityStatesIsFrameworkOwned is the hard repository boundary for the
// SemStreams catalog. Product code may read ENTITY_STATES only through
// graph.OpenCatalogBucket; graph-ingest is its sole creator and writer.
//
// spec: openspec/changes/cut-over-semstreams-beta159-rule-contracts/specs/semstreams-rule-cutover/spec.md#Requirement: Graph-ingest exclusively owns ENTITY_STATES
func TestEntityStatesIsFrameworkOwned(t *testing.T) {
	root := repositoryRoot(t)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path == root {
				return nil
			}
			base := entry.Name()
			if strings.HasPrefix(base, ".") || base == "node_modules" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if containsLegacyGraphWrite(data) {
			t.Errorf("%s retains a forbidden whole-entity writer", relative)
		}
		if containsRawGraphMutation(data) {
			t.Errorf("%s bypasses SemStreams projection/lifecycle mutation clients", relative)
		}

		if bytes.Contains(data, []byte("GetKeyValueBucket(")) &&
			(bytes.Contains(data, []byte("ENTITY_STATES")) ||
				bytes.Contains(data, []byte("BucketEntityStates")) ||
				bytes.Contains(data, []byte("BucketName("))) {
			t.Errorf("%s raw-opens the framework entity-state catalog", relative)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan production Go sources: %v", err)
	}

	graphClient, err := os.ReadFile(filepath.Join(root, "graphclient.go"))
	if err != nil {
		t.Fatalf("read graphclient.go: %v", err)
	}
	for _, forbidden := range []string{
		"CreateKeyValueBucket(",
		"GetKeyValueBucket(",
		"NewKVStore(",
		"func (gc *GraphClient) EnsureBucket(",
		"func (gc *GraphClient) EmitEntity(",
		"func (gc *GraphClient) EmitEntityUpdate(",
		"func (gc *GraphClient) EmitEntityCAS(",
		"func (gc *GraphClient) PutEntityState(",
		"func (gc *GraphClient) KVBucket(",
		"func (gc *GraphClient) Client(",
	} {
		if bytes.Contains(graphClient, []byte(forbidden)) {
			t.Errorf("graphclient.go exposes forbidden storage surface %q", forbidden)
		}
	}
	if !bytes.Contains(graphClient, []byte("graph.OpenCatalogBucket(")) {
		t.Error("graphclient.go must bind ENTITY_STATES through graph.OpenCatalogBucket")
	}
}

func TestEntityStatesOwnershipGuardRejectsLegacyWriters(t *testing.T) {
	for _, fragment := range []string{
		"graph.EmitEntity(ctx, entity, eventType)",
		"graph.EmitEntityUpdate(ctx, entity, eventType)",
		"graph.EmitEntityCAS(ctx, entity, eventType, revision)",
		"graph.PutEntityState(ctx, entity, eventType)",
	} {
		if !containsLegacyGraphWrite([]byte(fragment)) {
			t.Errorf("guard accepted synthetic legacy writer %q", fragment)
		}
	}
	if containsLegacyGraphWrite([]byte("graph.OpenCatalogBucket(ctx, nats, graph.BucketEntityStates)")) {
		t.Error("guard rejected the framework catalog reader")
	}
	for _, fragment := range []string{
		"graphingest.SubjectTripleAdd",
		"graphingest.SubjectEntityCreateWithTriples",
		"graphingest.SubjectEntityUpdateWithTriples",
		`"graph.mutation.triple.add"`,
		`"graph.mutation.triple.remove"`,
		`"graph.mutation.entity.create_with_triples"`,
		`"graph.mutation.entity.update_with_triples"`,
		"graph.AddTripleRequest{Triple: triple}",
		"graph.CreateEntityWithTriplesRequest{Entity: entity}",
		"graph.UpdateEntityWithTriplesRequest{Entity: entity}",
	} {
		if !containsRawGraphMutation([]byte(fragment)) {
			t.Errorf("guard accepted synthetic raw graph mutation %q", fragment)
		}
	}
	if containsRawGraphMutation([]byte("graph.OpenCatalogBucket(ctx, nats, graph.BucketEntityStates)")) {
		t.Error("raw-mutation guard rejected the framework catalog reader")
	}
}

func containsLegacyGraphWrite(data []byte) bool {
	for _, fragment := range [][]byte{
		[]byte(".EmitEntity("),
		[]byte(".EmitEntityUpdate("),
		[]byte(".EmitEntityCAS("),
		[]byte(".PutEntityState("),
	} {
		if bytes.Contains(data, fragment) {
			return true
		}
	}
	return false
}

func containsRawGraphMutation(data []byte) bool {
	for _, fragment := range [][]byte{
		[]byte("graph.mutation."),
		[]byte("graphingest.SubjectTripleAdd"),
		[]byte("graphingest.SubjectEntityCreateWithTriples"),
		[]byte("graphingest.SubjectEntityUpdateWithTriples"),
		[]byte("graph.AddTripleRequest"),
		[]byte("graph.CreateEntityWithTriplesRequest"),
		[]byte("graph.UpdateEntityWithTriplesRequest"),
	} {
		if bytes.Contains(data, fragment) {
			return true
		}
	}
	return false
}
