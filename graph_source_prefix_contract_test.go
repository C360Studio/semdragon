package semdragons_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestShippedGraphSourcePrefixesAreCanonical(t *testing.T) {
	root := repositoryRoot(t)
	expected := map[string]map[string]string{
		"config/semdragons.json": {
			"sandbox": "local.semsource",
		},
		"config/semdragons-semsource.json": {
			"sandbox": "local.semsource",
			"osh":     "osh",
		},
		"config/semdragons-e2e-tier1.json": {
			"workspace": "local.semsource",
		},
	}
	files := []string{
		"config/semdragons.json",
		"config/semdragons-semsource.json",
		"config/semdragons-e2e-tier1.json",
	}

	counts := make(map[string]int)
	for _, name := range files {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("read shipped config %q: %v", name, err)
		}
		var envelope struct {
			GraphSources []map[string]json.RawMessage `json:"graph_sources"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil {
			t.Fatalf("parse shipped config %q: %v", name, err)
		}

		got := make(map[string]string)
		for i, source := range envelope.GraphSources {
			rawPrefix, configured := source["entity_prefix"]
			if !configured {
				continue
			}
			var sourceName, prefix string
			if err := json.Unmarshal(source["name"], &sourceName); err != nil {
				t.Fatalf("%s graph_sources[%d].name: %v", name, i, err)
			}
			if err := json.Unmarshal(rawPrefix, &prefix); err != nil {
				t.Fatalf("%s graph_sources[%d].entity_prefix: %v", name, i, err)
			}
			if prefix == "" {
				t.Errorf("%s graph source %q has an empty configured prefix", name, sourceName)
				continue
			}
			for segmentIndex, segment := range strings.Split(prefix, ".") {
				if segment == "" {
					t.Errorf("%s graph source %q prefix %q has empty segment %d", name, sourceName, prefix, segmentIndex)
				}
			}
			got[sourceName] = prefix
			counts[prefix]++
		}
		if !reflect.DeepEqual(got, expected[name]) {
			t.Errorf("%s source-prefix entries = %#v, want exactly %#v", name, got, expected[name])
		}
	}

	if !reflect.DeepEqual(counts, map[string]int{"local.semsource": 3, "osh": 1}) {
		t.Errorf("shipped source-prefix counts = %#v, want local.semsource=3 and osh=1", counts)
	}
}
