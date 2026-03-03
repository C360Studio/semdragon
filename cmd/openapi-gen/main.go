// Command openapi-gen generates an OpenAPI 3.0.3 JSON specification from the
// semdragons service registration. It imports service/api to trigger init()
// registration, then converts the registered OpenAPISpec into a proper
// OpenAPI 3.0.3 document with component schemas derived via reflection.
//
// Usage:
//
//	go run ./cmd/openapi-gen > ui/static/openapi.json
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"

	"github.com/c360studio/semstreams/service"

	// Side-effect import: triggers service.RegisterOpenAPISpec("game", ...) in init().
	_ "github.com/c360studio/semdragons/service/api"
)

// =============================================================================
// OpenAPI 3.0.3 document types — proper field names for JSON serialization
// =============================================================================

type openAPIDocument struct {
	OpenAPI    string              `json:"openapi"`
	Info       infoObject          `json:"info"`
	Servers    []serverObject      `json:"servers"`
	Paths      map[string]pathItem `json:"paths"`
	Components componentsObject    `json:"components"`
	Tags       []tagObject         `json:"tags"`
}

type infoObject struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Version     string `json:"version"`
}

type serverObject struct {
	URL         string `json:"url"`
	Description string `json:"description"`
}

type componentsObject struct {
	Schemas map[string]any `json:"schemas"`
}

type tagObject struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type pathItem struct {
	Get    *operation `json:"get,omitempty"`
	Post   *operation `json:"post,omitempty"`
	Put    *operation `json:"put,omitempty"`
	Delete *operation `json:"delete,omitempty"`
}

type operation struct {
	Summary     string              `json:"summary"`
	Description string              `json:"description,omitempty"`
	Tags        []string            `json:"tags,omitempty"`
	Parameters  []parameter         `json:"parameters,omitempty"`
	RequestBody *requestBodyObject  `json:"requestBody,omitempty"`
	Responses   map[string]response `json:"responses"`
}

type parameter struct {
	Name        string    `json:"name"`
	In          string    `json:"in"`
	Required    bool      `json:"required,omitempty"`
	Description string    `json:"description,omitempty"`
	Schema      schemaRef `json:"schema"`
}

type requestBodyObject struct {
	Description string               `json:"description,omitempty"`
	Required    bool                 `json:"required,omitempty"`
	Content     map[string]mediaType `json:"content"`
}

type response struct {
	Description string               `json:"description"`
	Content     map[string]mediaType `json:"content,omitempty"`
}

type mediaType struct {
	Schema schemaRef `json:"schema"`
}

type schemaRef struct {
	Ref   string     `json:"$ref,omitempty"`
	Type  string     `json:"type,omitempty"`
	Items *schemaRef `json:"items,omitempty"`
}

func main() {
	specs := service.GetAllOpenAPISpecs()
	if len(specs) == 0 {
		fmt.Fprintln(os.Stderr, "no OpenAPI specs registered (is service/api imported?)")
		os.Exit(1)
	}

	doc := buildDocument(specs)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		fmt.Fprintf(os.Stderr, "failed to encode document: %v\n", err)
		os.Exit(1)
	}
}

// buildDocument assembles a complete OpenAPI 3.0.3 document from registered specs.
func buildDocument(specs map[string]*service.OpenAPISpec) openAPIDocument {
	paths := make(map[string]pathItem)
	tagMap := make(map[string]tagObject)
	schemas := make(map[string]any)
	seen := make(map[reflect.Type]bool)

	// Sort spec names for deterministic output.
	var names []string
	for name := range specs {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		spec := specs[name]

		// Merge paths. The "game" service is registered with path prefix by
		// the runtime service manager, but at build-time we apply it ourselves.
		prefix := servicePrefix(name)
		for path, ps := range spec.Paths {
			paths[prefix+path] = convertPathSpec(ps)
		}

		// Merge tags (deduplicate).
		for _, tag := range spec.Tags {
			if _, exists := tagMap[tag.Name]; !exists {
				tagMap[tag.Name] = tagObject{Name: tag.Name, Description: tag.Description}
			}
		}

		// Generate component schemas from ResponseTypes.
		for _, t := range spec.ResponseTypes {
			if !seen[t] {
				seen[t] = true
				schemas[service.TypeNameFromReflect(t)] = service.SchemaFromType(t)
			}
		}

		// Generate component schemas from RequestBodyTypes.
		for _, t := range spec.RequestBodyTypes {
			if !seen[t] {
				seen[t] = true
				schemas[service.TypeNameFromReflect(t)] = service.SchemaFromType(t)
			}
		}
	}

	// Apply enum overrides (SchemaFromType reflects enums as plain strings).
	applyEnumOverrides(schemas)

	// Sort tags alphabetically.
	var sortedTagNames []string
	for name := range tagMap {
		sortedTagNames = append(sortedTagNames, name)
	}
	sort.Strings(sortedTagNames)

	tags := make([]tagObject, 0, len(sortedTagNames))
	for _, name := range sortedTagNames {
		tags = append(tags, tagMap[name])
	}

	return openAPIDocument{
		OpenAPI: "3.0.3",
		Info: infoObject{
			Title:       "Semdragons API",
			Description: "Agentic workflow coordination modeled as a tabletop RPG. Work items are quests, agents are adventurers, quality reviews are boss battles.",
			Version:     "1.0.0",
		},
		Servers: []serverObject{
			{URL: "http://localhost:8080", Description: "Development server"},
		},
		Paths:      paths,
		Components: componentsObject{Schemas: schemas},
		Tags:       tags,
	}
}

// servicePrefix returns the URL path prefix for a registered service name.
// This mirrors the runtime service manager's prefix logic.
func servicePrefix(name string) string {
	switch name {
	case "game":
		return "/game"
	default:
		return "/" + name
	}
}

// =============================================================================
// Converters: service.*Spec → local OpenAPI types with proper JSON field names
// =============================================================================

func convertPathSpec(ps service.PathSpec) pathItem {
	item := pathItem{}
	if ps.GET != nil {
		item.Get = convertOperation(ps.GET)
	}
	if ps.POST != nil {
		item.Post = convertOperation(ps.POST)
	}
	if ps.PUT != nil {
		item.Put = convertOperation(ps.PUT)
	}
	if ps.DELETE != nil {
		item.Delete = convertOperation(ps.DELETE)
	}
	return item
}

func convertOperation(op *service.OperationSpec) *operation {
	o := &operation{
		Summary:     op.Summary,
		Description: op.Description,
		Tags:        op.Tags,
		Responses:   make(map[string]response),
	}

	for _, p := range op.Parameters {
		o.Parameters = append(o.Parameters, parameter{
			Name:        p.Name,
			In:          p.In,
			Required:    p.Required,
			Description: p.Description,
			Schema:      schemaRef{Type: p.Schema.Type},
		})
	}

	if op.RequestBody != nil {
		ct := op.RequestBody.ContentType
		if ct == "" {
			ct = "application/json"
		}
		o.RequestBody = &requestBodyObject{
			Description: op.RequestBody.Description,
			Required:    op.RequestBody.Required,
			Content: map[string]mediaType{
				ct: {Schema: schemaRef{Ref: op.RequestBody.SchemaRef}},
			},
		}
	}

	for code, resp := range op.Responses {
		r := response{Description: resp.Description}

		if resp.SchemaRef != "" {
			ct := resp.ContentType
			if ct == "" {
				ct = "application/json"
			}
			var schema schemaRef
			if resp.IsArray {
				schema = schemaRef{
					Type:  "array",
					Items: &schemaRef{Ref: resp.SchemaRef},
				}
			} else {
				schema = schemaRef{Ref: resp.SchemaRef}
			}
			r.Content = map[string]mediaType{
				ct: {Schema: schema},
			}
		}

		o.Responses[code] = r
	}

	return o
}
