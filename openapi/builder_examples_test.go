package openapi_test

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	apix "github.com/Infra-Forge/infra-apix"
	"github.com/Infra-Forge/infra-apix/openapi"
)

// Test models with example struct tags
type ExampleRequest struct {
	Name        string  `json:"name" example:"John Doe"`
	Age         int     `json:"age" example:"30"`
	Email       string  `json:"email" example:"john@example.com"`
	Score       float64 `json:"score" example:"95.5"`
	IsActive    bool    `json:"is_active" example:"true"`
	Description string  `json:"description,omitempty" example:"A sample user"`
}

type ExampleResponse struct {
	ID        string `json:"id" example:"user-123"`
	Status    string `json:"status" example:"created"`
	Timestamp int64  `json:"timestamp" example:"1640995200"`
}

func TestStructTagExamples(t *testing.T) {
	apix.ResetRegistry()

	// Register a route with struct tag examples
	apix.RegisterRoute(&apix.RouteRef{
		Method:      apix.MethodPost,
		Path:        "/api/users",
		Summary:     "Create user with examples",
		RequestType: reflect.TypeOf(ExampleRequest{}),
		Responses: map[int]*apix.ResponseRef{
			http.StatusCreated: {ModelType: reflect.TypeOf(ExampleResponse{})},
		},
	})

	builder := openapi.NewBuilder()
	builder.Info.Title = "Example Test API"
	builder.Info.Version = "1.0.0"

	doc, err := builder.Build(apix.Snapshot())
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Verify request schema has examples
	requestSchema := doc.Components.Schemas["openapi_test_ExampleRequest"]
	if requestSchema == nil || requestSchema.Value == nil {
		t.Fatal("ExampleRequest schema not found")
	}

	// Check field examples
	nameSchema := requestSchema.Value.Properties["name"]
	if nameSchema == nil || nameSchema.Value == nil {
		t.Fatal("name property not found")
	}
	if nameSchema.Value.Example != "John Doe" {
		t.Errorf("expected name example 'John Doe', got %v", nameSchema.Value.Example)
	}

	ageSchema := requestSchema.Value.Properties["age"]
	if ageSchema == nil || ageSchema.Value == nil {
		t.Fatal("age property not found")
	}
	if ageSchema.Value.Example != int64(30) {
		t.Errorf("expected age example 30, got %v", ageSchema.Value.Example)
	}

	scoreSchema := requestSchema.Value.Properties["score"]
	if scoreSchema == nil || scoreSchema.Value == nil {
		t.Fatal("score property not found")
	}
	if scoreSchema.Value.Example != float64(95.5) {
		t.Errorf("expected score example 95.5, got %v", scoreSchema.Value.Example)
	}

	isActiveSchema := requestSchema.Value.Properties["is_active"]
	if isActiveSchema == nil || isActiveSchema.Value == nil {
		t.Fatal("is_active property not found")
	}
	if isActiveSchema.Value.Example != true {
		t.Errorf("expected is_active example true, got %v", isActiveSchema.Value.Example)
	}

	// Verify response schema has examples
	responseSchema := doc.Components.Schemas["openapi_test_ExampleResponse"]
	if responseSchema == nil || responseSchema.Value == nil {
		t.Fatal("ExampleResponse schema not found")
	}

	idSchema := responseSchema.Value.Properties["id"]
	if idSchema == nil || idSchema.Value == nil {
		t.Fatal("id property not found")
	}
	if idSchema.Value.Example != "user-123" {
		t.Errorf("expected id example 'user-123', got %v", idSchema.Value.Example)
	}
}

func TestProgrammaticRequestExample(t *testing.T) {
	apix.ResetRegistry()

	exampleReq := map[string]any{
		"name":  "Jane Doe",
		"age":   25,
		"email": "jane@example.com",
	}

	ref := &apix.RouteRef{
		Method:      apix.MethodPost,
		Path:        "/api/users",
		Summary:     "Create user",
		RequestType: reflect.TypeOf(ExampleRequest{}),
		Responses: map[int]*apix.ResponseRef{
			http.StatusCreated: {ModelType: reflect.TypeOf(ExampleResponse{})},
		},
	}

	apix.WithRequestExample(exampleReq)(ref)
	apix.RegisterRoute(ref)

	builder := openapi.NewBuilder()
	doc, err := builder.Build(apix.Snapshot())
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Verify request body has example
	pathItem := doc.Paths.Value("/api/users")
	if pathItem == nil || pathItem.Post == nil {
		t.Fatal("POST /api/users not found")
	}

	if pathItem.Post.RequestBody == nil || pathItem.Post.RequestBody.Value == nil {
		t.Fatal("Request body not found")
	}

	mediaType := pathItem.Post.RequestBody.Value.Content["application/json"]
	if mediaType == nil {
		t.Fatal("application/json media type not found")
	}

	if mediaType.Example == nil {
		t.Fatal("Request example not set")
	}

	// Verify example content
	exampleJSON, _ := json.Marshal(mediaType.Example)
	expectedJSON, _ := json.Marshal(exampleReq)
	if string(exampleJSON) != string(expectedJSON) {
		t.Errorf("expected example %s, got %s", expectedJSON, exampleJSON)
	}
}

func TestProgrammaticResponseExample(t *testing.T) {
	apix.ResetRegistry()

	exampleResp := map[string]any{
		"id":        "user-456",
		"status":    "active",
		"timestamp": 1640995200,
	}

	ref := &apix.RouteRef{
		Method:      apix.MethodGet,
		Path:        "/api/users/{id}",
		Summary:     "Get user",
		RequestType: nil,
		Responses: map[int]*apix.ResponseRef{
			http.StatusOK: {
				ModelType: reflect.TypeOf(ExampleResponse{}),
				Example:   exampleResp,
			},
		},
	}

	apix.RegisterRoute(ref)

	builder := openapi.NewBuilder()
	doc, err := builder.Build(apix.Snapshot())
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Verify response has example
	pathItem := doc.Paths.Value("/api/users/{id}")
	if pathItem == nil || pathItem.Get == nil {
		t.Fatal("GET /api/users/{id} not found")
	}

	response := pathItem.Get.Responses.Status(http.StatusOK)
	if response == nil || response.Value == nil {
		t.Fatal("200 response not found")
	}

	mediaType := response.Value.Content["application/json"]
	if mediaType == nil {
		t.Fatal("application/json media type not found")
	}

	if mediaType.Example == nil {
		t.Fatal("Response example not set")
	}

	// Verify example content
	exampleJSON, _ := json.Marshal(mediaType.Example)
	expectedJSON, _ := json.Marshal(exampleResp)
	if string(exampleJSON) != string(expectedJSON) {
		t.Errorf("expected example %s, got %s", expectedJSON, exampleJSON)
	}
}

func TestParameterExamples(t *testing.T) {
	apix.ResetRegistry()

	ref := &apix.RouteRef{
		Method:  apix.MethodGet,
		Path:    "/api/search",
		Summary: "Search with parameters",
		Parameters: []apix.Parameter{
			{
				Name:        "query",
				In:          "query",
				Description: "Search query",
				SchemaType:  "string",
				Required:    true,
				Example:     "golang",
			},
			{
				Name:        "limit",
				In:          "query",
				Description: "Result limit",
				SchemaType:  "integer",
				Required:    false,
				Example:     10,
			},
			{
				Name:        "X-API-Key",
				In:          "header",
				Description: "API key",
				SchemaType:  "string",
				Required:    true,
				Example:     "abc123xyz",
			},
		},
		Responses: map[int]*apix.ResponseRef{
			http.StatusOK: {ModelType: nil},
		},
	}

	apix.RegisterRoute(ref)

	builder := openapi.NewBuilder()
	doc, err := builder.Build(apix.Snapshot())
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Verify parameters have examples
	pathItem := doc.Paths.Value("/api/search")
	if pathItem == nil || pathItem.Get == nil {
		t.Fatal("GET /api/search not found")
	}

	if len(pathItem.Get.Parameters) != 3 {
		t.Fatalf("expected 3 parameters, got %d", len(pathItem.Get.Parameters))
	}

	// Parameters are sorted by In (header < query), then by Name
	// Expected order: header:X-API-Key, query:limit, query:query

	// Check header parameter example (first after sorting)
	headerParam := pathItem.Get.Parameters[0].Value
	if headerParam.Name != "X-API-Key" {
		t.Errorf("expected first parameter to be 'X-API-Key', got %s", headerParam.Name)
	}
	if headerParam.Example != "abc123xyz" {
		t.Errorf("expected X-API-Key example 'abc123xyz', got %v", headerParam.Example)
	}

	// Check limit parameter example (second after sorting - query params sorted by name)
	limitParam := pathItem.Get.Parameters[1].Value
	if limitParam.Name != "limit" {
		t.Errorf("expected second parameter to be 'limit', got %s", limitParam.Name)
	}
	if limitParam.Example != 10 {
		t.Errorf("expected limit example 10, got %v", limitParam.Example)
	}

	// Check query parameter example (third after sorting)
	queryParam := pathItem.Get.Parameters[2].Value
	if queryParam.Name != "query" {
		t.Errorf("expected third parameter to be 'query', got %s", queryParam.Name)
	}
	if queryParam.Example != "golang" {
		t.Errorf("expected query example 'golang', got %v", queryParam.Example)
	}
}

type BoolExamples struct {
	Lower bool `json:"lower" example:"false"`
	Upper bool `json:"upper" example:"TRUE"`
	Other bool `json:"other" example:"yes"`
}

// TestBoolExamples: a bool example is read case-insensitively, and any value
// but true or false stays the string it was written as.
func TestBoolExamples(t *testing.T) {
	apix.ResetRegistry()
	t.Cleanup(apix.ResetRegistry)
	apix.RegisterRoute(&apix.RouteRef{
		Method:      apix.MethodPost,
		Path:        "/flags",
		RequestType: reflect.TypeOf(BoolExamples{}),
		Responses:   map[int]*apix.ResponseRef{http.StatusNoContent: {}},
	})

	doc, err := openapi.NewBuilder().Build(apix.Snapshot())
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	schema := doc.Components.Schemas["openapi_test_BoolExamples"]
	if schema == nil || schema.Value == nil {
		t.Fatal("BoolExamples schema not found")
	}
	for field, want := range map[string]any{"lower": false, "upper": true, "other": "yes"} {
		prop := schema.Value.Properties[field]
		if prop == nil || prop.Value == nil {
			t.Fatalf("%s property not found", field)
		}
		if prop.Value.Example != want {
			t.Errorf("%s example = %#v, want %#v", field, prop.Value.Example, want)
		}
	}
}

type jsonFormItem struct {
	ID       string   `json:"id"`
	Count    uint     `json:"count"`
	Big      int64    `json:"big"`
	IsActive bool     `json:"is_active"`
	Ratio    float64  `json:"ratio"`
	Tags     []string `json:"tags"`
	Internal string   `json:"-"`
	Note     string   `json:"note,omitempty"`
}

// TestExamplesAreStoredAsTheirJSONForm: an example given as a Go value other
// than a JSON scalar (a struct, a pointer to one, a uint, a time.Time) is
// documented as the JSON the handler would send: keyed by the json tags, with
// integers as int64. kin-openapi validates the document, and its YAML shows the
// names clients send, without the json:"-" field.
func TestExamplesAreStoredAsTheirJSONForm(t *testing.T) {
	item := jsonFormItem{ID: "item-1", Count: 3, Big: 9007199254740993, IsActive: true, Ratio: 0.5, Tags: []string{"a"}, Internal: "hidden"}
	ref := &apix.RouteRef{
		Method:      apix.MethodPost,
		Path:        "/items",
		RequestType: reflect.TypeOf(jsonFormItem{}),
		Parameters: []apix.Parameter{
			{Name: "limit", In: "query", SchemaType: "integer", Example: uint(50)},
			{Name: "since", In: "query", SchemaType: "string", Example: time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)},
		},
	}
	apix.WithRequestExample(item)(ref)
	apix.RegisterResponse(ref, http.StatusCreated, reflect.TypeOf(jsonFormItem{}), apix.WithResponseExample(&item))

	doc, err := openapi.NewBuilder().Build([]*apix.RouteRef{ref})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if err := doc.Validate(t.Context()); err != nil {
		t.Fatalf("document does not validate: %v", err)
	}

	want := map[string]any{"id": "item-1", "count": int64(3), "big": int64(9007199254740993), "is_active": true, "ratio": 0.5, "tags": []any{"a"}}
	op := doc.Paths.Value("/items").Post
	if got := op.RequestBody.Value.Content["application/json"].Example; !reflect.DeepEqual(got, want) {
		t.Errorf("request example = %#v, want %#v", got, want)
	}
	if got := op.Responses.Status(http.StatusCreated).Value.Content["application/json"].Example; !reflect.DeepEqual(got, want) {
		t.Errorf("response example = %#v, want %#v", got, want)
	}
	params := map[string]any{}
	for _, p := range op.Parameters {
		params[p.Value.Name] = p.Value.Example
	}
	if want := map[string]any{"limit": int64(50), "since": "2026-11-07T00:00:00Z"}; !reflect.DeepEqual(params, want) {
		t.Errorf("parameter examples = %#v, want %#v", params, want)
	}

	data, _, err := openapi.EncodeDocument(doc, "yaml")
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	yamlDoc := string(data)
	if strings.Count(yamlDoc, "is_active: true") != 2 {
		t.Errorf("the YAML examples do not use the json tag names:\n%s", yamlDoc)
	}
	if strings.Contains(yamlDoc, "hidden") {
		t.Errorf("the YAML examples carry the json:\"-\" field:\n%s", yamlDoc)
	}
}

// TestExampleWithoutAJSONFormFailsTheBuild: an example encoding/json rejects
// cannot be written into the document, so the build fails naming the route and
// the example.
func TestExampleWithoutAJSONFormFailsTheBuild(t *testing.T) {
	tests := []struct {
		name   string
		ref    *apix.RouteRef
		prefix string
	}{
		{
			name: "parameter",
			ref: &apix.RouteRef{
				Method:     apix.MethodGet,
				Path:       "/items",
				Parameters: []apix.Parameter{{Name: "limit", In: "query", Example: make(chan int)}},
				Responses:  map[int]*apix.ResponseRef{http.StatusOK: {}},
			},
			prefix: `GET /items: parameter "limit" example: `,
		},
		{
			name: "request",
			ref: &apix.RouteRef{
				Method:         apix.MethodPost,
				Path:           "/items",
				RequestType:    reflect.TypeOf(jsonFormItem{}),
				RequestExample: map[string]any{"ratio": math.Inf(1)},
				Responses:      map[int]*apix.ResponseRef{http.StatusCreated: {}},
			},
			prefix: "POST /items: request example: ",
		},
		{
			name: "response",
			ref: &apix.RouteRef{
				Method: apix.MethodGet,
				Path:   "/items",
				Responses: map[int]*apix.ResponseRef{
					http.StatusOK: {ModelType: reflect.TypeOf(float64(0)), Example: math.NaN()},
				},
			},
			prefix: "GET /items: response 200 example: ",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := openapi.NewBuilder().Build([]*apix.RouteRef{tc.ref})
			if err == nil || !strings.HasPrefix(err.Error(), tc.prefix) {
				t.Fatalf("Build error = %v, want one starting %q", err, tc.prefix)
			}
			var typeErr *json.UnsupportedTypeError
			var valueErr *json.UnsupportedValueError
			if !errors.As(err, &typeErr) && !errors.As(err, &valueErr) {
				t.Errorf("Build error %v does not wrap the encoding/json error", err)
			}
		})
	}
}

// TestExampleIntegerOutsideInt64FailsTheBuild: an example integer outside
// int64, here a uint64, fails the build naming the route: as a float64 it
// would lose digits, and kept exact the YAML document would write it as a
// string. MaxInt64 itself is kept exact.
func TestExampleIntegerOutsideInt64FailsTheBuild(t *testing.T) {
	ref := &apix.RouteRef{
		Method:         apix.MethodPost,
		Path:           "/items",
		RequestType:    reflect.TypeOf(jsonFormItem{}),
		RequestExample: map[string]any{"big": uint64(math.MaxInt64) + 2},
		Responses:      map[int]*apix.ResponseRef{http.StatusCreated: {}},
	}
	_, err := openapi.NewBuilder().Build([]*apix.RouteRef{ref})
	want := "POST /items: request example: integer 9223372036854775809 is outside the int64 range"
	if err == nil || err.Error() != want {
		t.Fatalf("Build error = %v, want %q", err, want)
	}

	ref.RequestExample = map[string]any{"big": uint64(math.MaxInt64)}
	doc, err := openapi.NewBuilder().Build([]*apix.RouteRef{ref})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	got := doc.Paths.Value("/items").Post.RequestBody.Value.Content["application/json"].Example
	if want := map[string]any{"big": int64(math.MaxInt64)}; !reflect.DeepEqual(got, want) {
		t.Errorf("request example = %#v, want %#v", got, want)
	}
}
