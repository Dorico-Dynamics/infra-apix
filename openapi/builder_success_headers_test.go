package openapi_test

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	apix "github.com/Infra-Forge/infra-apix"
	"github.com/Infra-Forge/infra-apix/openapi"
)

type shItem struct {
	ID string `json:"id"`
}

func shRoute(method apix.RouteMethod, opts ...apix.RouteOption) *apix.RouteRef {
	ref := &apix.RouteRef{Method: method, Path: "/items"}
	for _, opt := range opts {
		opt(ref)
	}
	if ref.SuccessStatus == 0 {
		ref.SuccessStatus = apix.DefaultSuccessStatus(method)
	}
	apix.EnsureResponse(ref, ref.SuccessStatus, reflect.TypeOf(shItem{}))
	return ref
}

// TestBuildDocumentsNoLocationUnlessDeclared: a POST answers 201, but most
// handlers cannot set Location, so it is no longer documented as required by
// default.
func TestBuildDocumentsNoLocationUnlessDeclared(t *testing.T) {
	doc, err := openapi.NewBuilder().Build([]*apix.RouteRef{shRoute(apix.MethodPost)})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	created := doc.Paths.Value("/items").Post.Responses.Status(http.StatusCreated)
	if created == nil {
		t.Fatal("201 response missing")
	}
	if len(created.Value.Headers) != 0 {
		t.Errorf("headers documented without a declaration: %v", created.Value.Headers)
	}
}

// TestBuildDocumentsDeclaredSuccessHeaders: WithSuccessHeaders reaches the
// response for its status, with each header's requiredness and type.
func TestBuildDocumentsDeclaredSuccessHeaders(t *testing.T) {
	ref := shRoute(apix.MethodPost, apix.WithSuccessHeaders(http.StatusCreated,
		apix.HeaderRef{Name: "Location", Description: "URI of the created item", SchemaType: "string", Required: true},
		apix.HeaderRef{Name: "X-Rate-Limit-Remaining", SchemaType: "integer"},
	))
	doc, err := openapi.NewBuilder().Build([]*apix.RouteRef{ref})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	headers := doc.Paths.Value("/items").Post.Responses.Status(http.StatusCreated).Value.Headers
	location := headers["Location"]
	if location == nil || !location.Value.Required || location.Value.Description != "URI of the created item" ||
		!location.Value.Schema.Value.Type.Is("string") {
		t.Errorf("Location = %+v, want a required string with its description", location)
	}
	remaining := headers["X-Rate-Limit-Remaining"]
	if remaining == nil || remaining.Value.Required || !remaining.Value.Schema.Value.Type.Is("integer") {
		t.Errorf("X-Rate-Limit-Remaining = %+v, want an optional integer", remaining)
	}
	if err := doc.Validate(t.Context()); err != nil {
		t.Errorf("document does not validate: %v", err)
	}
}

// TestBuildRejectsSuccessHeadersForAStatusWithoutResponse: headers declared
// for a status the route never answers would be dropped, so Build fails.
func TestBuildRejectsSuccessHeadersForAStatusWithoutResponse(t *testing.T) {
	ref := shRoute(apix.MethodPost,
		apix.WithSuccessStatus(http.StatusOK),
		apix.WithSuccessHeaders(http.StatusCreated, apix.HeaderRef{Name: "Location", Required: true}),
	)
	_, err := openapi.NewBuilder().Build([]*apix.RouteRef{ref})
	want := "POST /items: WithSuccessHeaders(201, ...) names a status the route has no response for"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("Build error = %v, want %q", err, want)
	}
}

// TestBuildDocumentsResponseHeaders: headers attached to a response with
// WithHeaders are documented like WithSuccessHeaders ones, as a string when
// no schema type is given.
func TestBuildDocumentsResponseHeaders(t *testing.T) {
	ref := &apix.RouteRef{Method: apix.MethodGet, Path: "/items"}
	apix.EnsureResponse(ref, http.StatusOK, reflect.TypeOf(shItem{}),
		apix.WithHeaders(apix.HeaderRef{Name: "ETag", Description: "Version of the item", Required: true}))

	doc, err := openapi.NewBuilder().Build([]*apix.RouteRef{ref})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	etag := doc.Paths.Value("/items").Get.Responses.Status(http.StatusOK).Value.Headers["ETag"]
	if etag == nil || !etag.Value.Required || etag.Value.Description != "Version of the item" ||
		!etag.Value.Schema.Value.Type.Is("string") {
		t.Errorf("ETag = %+v, want a required string with its description", etag)
	}
}
