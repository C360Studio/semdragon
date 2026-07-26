package main

import (
	"strings"
	"testing"

	"github.com/c360studio/semstreams/service"
)

func TestBuildDocumentIncludesResolvedLifecycleGatewayPaths(t *testing.T) {
	specs := service.GetAllOpenAPISpecs()
	doc := buildDocument(specs)

	const root = "/lifecycle-gateway/workflows"
	for _, path := range []string{
		root,
		root + "/{type}",
		root + "/{type}/{id}",
		root + "/{type}/{id}/history",
		root + "/{type}/{id}/children",
		root + "/{type}/{id}/state",
		root + "/{type}/{id}/transition",
	} {
		if _, ok := doc.Paths[path]; !ok {
			t.Errorf("resolved lifecycle gateway path %q missing", path)
		}
	}

	if _, found := doc.Paths["/lifecycle-gateway/workflows/{type}/{id}/transition"]; !found {
		t.Fatal("transition operation missing from resolved lifecycle gateway paths")
	}
}

func TestBuildDocumentResolvesLifecycleGatewayRequestSchemas(t *testing.T) {
	specs := service.GetAllOpenAPISpecs()
	doc := buildDocument(specs)

	for _, name := range []string{"StatePatchRequest", "TransitionRequest"} {
		if _, ok := doc.Components.Schemas[name]; !ok {
			t.Errorf("lifecycle gateway request schema %q missing", name)
		}
	}

	transition := doc.Paths["/lifecycle-gateway/workflows/{type}/{id}/transition"].Post
	if transition == nil || transition.RequestBody == nil {
		t.Fatal("transition request body missing")
	}
	ref := transition.RequestBody.Content["application/json"].Schema.Ref
	if !strings.HasSuffix(ref, "/TransitionRequest") {
		t.Errorf("transition request schema ref = %q, want TransitionRequest", ref)
	}
}
