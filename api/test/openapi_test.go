package api_test

import (
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestOpenAPIDocumentsExactHTTPRouteSurface(t *testing.T) {
	document := readOpenAPI(t)
	paths := object(t, document, "paths")
	got := make([]string, 0)
	for path, rawPath := range paths {
		operations, ok := rawPath.(map[string]any)
		if !ok {
			t.Fatalf("path %q is not an object", path)
		}
		for method := range operations {
			if isHTTPMethod(method) {
				got = append(got, strings.ToUpper(method)+" "+path)
			}
		}
	}
	sort.Strings(got)
	want := []string{
		"DELETE /api/v1/labels/{id}",
		"DELETE /api/v1/session",
		"GET /api/v1/breakdown",
		"GET /api/v1/connections/live",
		"GET /api/v1/healthz",
		"GET /api/v1/label-candidates",
		"GET /api/v1/labels",
		"GET /api/v1/live",
		"GET /api/v1/overview",
		"GET /api/v1/quality",
		"GET /api/v1/readyz",
		"GET /api/v1/runtime-sessions",
		"GET /api/v1/series",
		"GET /api/v1/status",
		"GET /api/v1/storage",
		"POST /api/v1/labels",
		"POST /api/v1/session",
		"PUT /api/v1/labels/{id}",
	}
	// Health and readiness are intentionally outside the versioned API prefix.
	want[4] = "GET /healthz"
	want[10] = "GET /readyz"
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("OpenAPI operations = %#v, want %#v", got, want)
	}
}

func TestOpenAPIRequiresSessionCookieJSONErrorsSSEAndDecimalByteCounts(t *testing.T) {
	document := readOpenAPI(t)
	components := object(t, document, "components")
	securitySchemes := object(t, components, "securitySchemes")
	cookie := object(t, securitySchemes, "sessionCookie")
	if cookie["type"] != "apiKey" || cookie["in"] != "cookie" || cookie["name"] != "flowlens_session" {
		t.Fatalf("sessionCookie = %#v", cookie)
	}
	attributes := object(t, cookie, "x-cookie-attributes")
	if attributes["httpOnly"] != true || attributes["sameSite"] != "Strict" || attributes["path"] != "/" {
		t.Fatalf("session cookie attributes = %#v", attributes)
	}

	paths := object(t, document, "paths")
	live := object(t, object(t, paths, "/api/v1/live"), "get")
	liveOK := object(t, object(t, live, "responses"), "200")
	if _, exists := object(t, liveOK, "content")["text/event-stream"]; !exists {
		t.Fatalf("live response does not declare text/event-stream: %#v", liveOK)
	}

	jsonErrorRef := "#/components/responses/JSONError"
	for path, rawPath := range paths {
		if !strings.HasPrefix(path, "/api/") {
			continue
		}
		for method, rawOperation := range rawPath.(map[string]any) {
			if !isHTTPMethod(method) {
				continue
			}
			responses := object(t, rawOperation.(map[string]any), "responses")
			for status, rawResponse := range responses {
				if status[0] != '4' && status[0] != '5' {
					continue
				}
				response := rawResponse.(map[string]any)
				if response["$ref"] != jsonErrorRef {
					t.Errorf("%s %s response %s is not JSONError: %#v", strings.ToUpper(method), path, status, response)
				}
			}
		}
	}

	schemas := object(t, components, "schemas")
	byteFields := 0
	visitProperties(t, schemas, func(name string, schema map[string]any) {
		if !strings.HasSuffix(name, "_bytes") {
			return
		}
		byteFields++
		if schema["type"] != "string" || schema["pattern"] != "^[0-9]+$" {
			t.Errorf("byte count %q = %#v", name, schema)
		}
	})
	if byteFields < 10 {
		t.Fatalf("documented decimal byte fields = %d", byteFields)
	}
}

func readOpenAPI(t *testing.T) map[string]any {
	t.Helper()
	contents, err := os.ReadFile("../openapi.yaml")
	if err != nil {
		t.Fatalf("ReadFile(openapi.yaml) error = %v", err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(contents, &document); err != nil {
		t.Fatalf("openapi.yaml error = %v", err)
	}
	if document["openapi"] != "3.1.0" {
		t.Fatalf("openapi version = %#v", document["openapi"])
	}
	return document
}

func object(t *testing.T, parent map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := parent[key].(map[string]any)
	if !ok {
		t.Fatalf("%q is not an object: %#v", key, parent[key])
	}
	return value
}

func isHTTPMethod(value string) bool {
	switch value {
	case "get", "post", "put", "delete", "patch", "head", "options", "trace":
		return true
	default:
		return false
	}
}

func visitProperties(t *testing.T, value any, visit func(string, map[string]any)) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		if properties, ok := typed["properties"].(map[string]any); ok {
			for name, rawSchema := range properties {
				schema, ok := rawSchema.(map[string]any)
				if !ok {
					t.Fatalf("property %q is not an object", name)
				}
				visit(name, schema)
			}
		}
		for _, child := range typed {
			visitProperties(t, child, visit)
		}
	case []any:
		for _, child := range typed {
			visitProperties(t, child, visit)
		}
	}
}
