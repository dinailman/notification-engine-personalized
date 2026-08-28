package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// muxRoute matches every route cmd/api/main.go registers, so the route table stays the one
// the binary actually serves rather than a second list kept in step by hand.
var muxRoute = regexp.MustCompile(`mux\.HandleFunc\("([A-Z]+) ([^"]+)"`)

// spec decodes the embedded document -- the exact bytes GET /openapi.json writes, not a copy
// read from disk.
func spec(t *testing.T) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(openapiJSON, &doc); err != nil {
		t.Fatalf("the embedded OpenAPI document is not valid JSON: %v", err)
	}
	return doc
}

func schemas(t *testing.T) map[string]any {
	t.Helper()
	components, ok := spec(t)["components"].(map[string]any)
	if !ok {
		t.Fatal("the document has no components section")
	}
	out, ok := components["schemas"].(map[string]any)
	if !ok || len(out) == 0 {
		t.Fatal("the document defines no schemas")
	}
	return out
}

// TestOpenAPICoversEveryRoute guards against the drift that prompted this test: the served
// document once carried no schemas at all while an unreferenced openapi.yaml held the real
// spec. Reading the routes out of main.go means a new endpoint fails here until documented.
func TestOpenAPICoversEveryRoute(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "..", "cmd", "api", "main.go"))
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	routes := muxRoute.FindAllStringSubmatch(string(source), -1)
	if len(routes) == 0 {
		t.Fatal("no mux.HandleFunc routes found in main.go; the registration style changed and this test needs updating")
	}
	paths, ok := spec(t)["paths"].(map[string]any)
	if !ok {
		t.Fatal("the document has no paths section")
	}
	for _, route := range routes {
		method, path := strings.ToLower(route[1]), route[2]
		item, ok := paths[path].(map[string]any)
		if !ok {
			t.Errorf("route %s %s is missing from the served document", route[1], path)
			continue
		}
		if _, ok := item[method]; !ok {
			t.Errorf("route %s %s is registered, but the document lists no %s operation for that path", route[1], path, method)
		}
	}
}

// TestOpenAPIDocumentsQuietHours pins the specific regression: quiet hours shipped in the API
// but reached only the spec that was not being served.
func TestOpenAPIDocumentsQuietHours(t *testing.T) {
	all := schemas(t)
	for _, name := range []string{"UserRequest", "UserUpdate"} {
		schema, ok := all[name].(map[string]any)
		if !ok {
			t.Errorf("schema %s is missing", name)
			continue
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Errorf("schema %s declares no properties", name)
			continue
		}
		for _, field := range []string{"quiet_hours_start", "quiet_hours_end"} {
			if _, ok := properties[field]; !ok {
				t.Errorf("schema %s does not document %s", name, field)
			}
		}
	}
}

// TestOpenAPIRefsResolve walks every $ref in the document, which catches a merge that left a
// reference pointing at a schema nobody defined.
func TestOpenAPIRefsResolve(t *testing.T) {
	defined := schemas(t)
	const prefix = "#/components/schemas/"

	var walk func(node any)
	walk = func(node any) {
		switch value := node.(type) {
		case map[string]any:
			for key, child := range value {
				if key != "$ref" {
					walk(child)
					continue
				}
				ref, ok := child.(string)
				if !ok {
					t.Errorf("$ref is %T, want a string", child)
					continue
				}
				name, found := strings.CutPrefix(ref, prefix)
				if !found {
					t.Errorf("$ref %q does not point into %s", ref, prefix)
					continue
				}
				if _, ok := defined[name]; !ok {
					t.Errorf("$ref %q points at a schema the document does not define", ref)
				}
			}
		case []any:
			for _, child := range value {
				walk(child)
			}
		}
	}
	walk(spec(t))
}
