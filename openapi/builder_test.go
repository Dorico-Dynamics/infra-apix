package openapi_test

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	apix "github.com/Infra-Forge/infra-apix"
	"github.com/Infra-Forge/infra-apix/openapi"
	"github.com/getkin/kin-openapi/openapi3"
)

type requestModel struct {
	Name string         `json:"name" validate:"required"`
	Meta map[string]any `json:"meta,omitempty"`
	Tags []string       `json:"tags"`
}

type responseBase struct {
	ID string `json:"id"`
}

type responseModel struct {
	responseBase
	CreatedAt string          `json:"created_at"`
	Data      []string        `json:"data"`
	Flags     map[string]bool `json:"flags"`
	Nested    map[string]struct {
		Secret string `json:"secret"`
	}
}

func TestBuilderGeneratesDeterministicDocument(t *testing.T) {
	t.Cleanup(apix.ResetRegistry)

	ref := &apix.RouteRef{
		Method:      apix.MethodPost,
		Path:        "/api/items",
		Summary:     "Create item",
		RequestType: reflect.TypeOf(requestModel{}),
		Responses: map[int]*apix.ResponseRef{
			http.StatusCreated: {ModelType: reflect.TypeOf(responseModel{})},
		},
	}

	apix.WithTags("items", "v1")(ref)
	apix.WithSecurity("BearerAuth")(ref)
	apix.WithBodyRequired(true)(ref)
	apix.WithParameter(apix.Parameter{Name: "X-Request-ID", In: "header", SchemaType: "string", Required: false})(ref)
	apix.WithSuccessHeaders(http.StatusCreated, apix.HeaderRef{Name: "Location", SchemaType: "string", Required: true})(ref)

	apix.RegisterRoute(ref)

	b := openapi.NewBuilder()
	b.Info.Title = "Test API"
	b.Info.Version = "2025.1"
	b.Servers = append(b.Servers, &openapi3.Server{URL: "https://example.com"})
	b.SecuritySchemes = openapi3.SecuritySchemes{
		"BearerAuth": {Value: openapi3.NewJWTSecurityScheme()},
	}

	doc, err := b.Build(apix.Snapshot())
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	pathItem := doc.Paths.Value("/api/items")
	if pathItem == nil || pathItem.Post == nil {
		t.Fatalf("expected POST /api/items in paths")
	}

	if pathItem.Post.RequestBody == nil {
		t.Fatalf("expected request body present")
	}

	if len(pathItem.Post.Parameters) != 1 {
		t.Fatalf("expected one parameter")
	}

	if pathItem.Post.Responses.Status(http.StatusCreated) == nil {
		t.Fatalf("expected 201 response registered")
	}

	resp := pathItem.Post.Responses.Status(http.StatusCreated)
	if resp.Value == nil {
		t.Fatalf("expected response value")
	}
	if resp.Value.Headers == nil || resp.Value.Headers["Location"] == nil {
		t.Fatalf("expected Location header initialised")
	}

	if doc.Components.Schemas == nil {
		t.Fatalf("expected components to be populated")
	}
	found := false
	for name := range doc.Components.Schemas {
		if strings.Contains(name, "responseModel") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected responseModel schema registered")
	}

	// Security should have generated 401/403 defaults
	if pathItem.Post.Responses.Status(http.StatusUnauthorized) == nil {
		t.Fatalf("expected 401 response")
	}
	if pathItem.Post.Responses.Status(http.StatusForbidden) == nil {
		t.Fatalf("expected 403 response")
	}
}
