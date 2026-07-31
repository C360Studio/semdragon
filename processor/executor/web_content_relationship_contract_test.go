package executor

import (
	"os"
	"strings"
	"testing"
)

func TestUnownedWebContentGraphPersistenceIsRemoved(t *testing.T) {
	testCases := []struct {
		path      string
		forbidden []string
	}{
		{
			path: "tools.go",
			forbidden: []string{
				"SetHTTPGraphPersist",
				"graphPersister",
				"persistWebContent",
				"buildWebContentEntity",
			},
		},
		{
			path: "../questtools/config.go",
			forbidden: []string{
				"HTTPPersistToGraph",
				"http_persist_to_graph",
			},
		},
	}
	for _, testCase := range testCases {
		data, err := os.ReadFile(testCase.path)
		if err != nil {
			t.Fatalf("read %s: %v", testCase.path, err)
		}
		for _, fragment := range testCase.forbidden {
			if strings.Contains(string(data), fragment) {
				t.Errorf("%s retains unowned graph-persistence surface %q", testCase.path, fragment)
			}
		}
	}
}
