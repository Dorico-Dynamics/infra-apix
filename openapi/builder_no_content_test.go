package openapi_test

import (
	"net/http"
	"reflect"
	"testing"

	apix "github.com/Infra-Forge/infra-apix"
	"github.com/Infra-Forge/infra-apix/openapi"
)

type ncItem struct {
	ID string `json:"id"`
}

// TestBuildDocumentsNoContentForNoBodyOr204: every adapter writes no body for a
// 204 or an apix.NoBody payload, so neither response documents content, and no
// infra_apix_NoBody component is emitted. Their headers stay documented, a
// NoBody request type documents no request body, and other responses keep
// their content.
func TestBuildDocumentsNoContentForNoBodyOr204(t *testing.T) {
	noBody := reflect.TypeOf(apix.NoBody{})
	item := reflect.TypeOf(ncItem{})

	remove := &apix.RouteRef{Method: apix.MethodDelete, Path: "/items"}
	apix.EnsureResponse(remove, http.StatusNoContent, noBody,
		apix.WithHeaders(apix.HeaderRef{Name: "X-Request-ID", SchemaType: "string"}))

	archive := &apix.RouteRef{Method: apix.MethodPost, Path: "/items/archive", RequestType: noBody}
	apix.RegisterResponse(archive, http.StatusAccepted, reflect.TypeOf(&apix.NoBody{}))

	replace := &apix.RouteRef{Method: apix.MethodPut, Path: "/items", RequestType: item}
	apix.RegisterResponse(replace, http.StatusOK, item)
	apix.RegisterResponse(replace, http.StatusNoContent, item)

	touch := &apix.RouteRef{Method: apix.MethodPatch, Path: "/items"}
	apix.RegisterResponse(touch, http.StatusOK, item, apix.WithExplicitModel(apix.NoBody{}))

	doc, err := openapi.NewBuilder().Build([]*apix.RouteRef{remove, archive, replace, touch})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := doc.Validate(t.Context()); err != nil {
		t.Fatalf("document does not validate: %v", err)
	}

	responses := []struct {
		method, path string
		status       int
		wantContent  bool
	}{
		{http.MethodDelete, "/items", http.StatusNoContent, false},
		{http.MethodPost, "/items/archive", http.StatusAccepted, false},
		{http.MethodPut, "/items", http.StatusOK, true},
		{http.MethodPut, "/items", http.StatusNoContent, false},
		{http.MethodPatch, "/items", http.StatusOK, false},
	}
	for _, tc := range responses {
		resp := doc.Paths.Value(tc.path).GetOperation(tc.method).Responses.Status(tc.status)
		if resp == nil || resp.Value == nil {
			t.Fatalf("%s %s: response %d missing", tc.method, tc.path, tc.status)
		}
		if gotContent := len(resp.Value.Content) > 0; gotContent != tc.wantContent {
			t.Errorf("%s %s: response %d documents content = %v, want %v", tc.method, tc.path, tc.status, gotContent, tc.wantContent)
		}
	}
	if doc.Paths.Value("/items").Delete.Responses.Status(http.StatusNoContent).Value.Headers["X-Request-ID"] == nil {
		t.Error("the 204 response lost its X-Request-ID header")
	}
	if body := doc.Paths.Value("/items/archive").Post.RequestBody; body != nil {
		t.Errorf("a NoBody request type documents a request body: %+v", body.Value)
	}
	if _, ok := doc.Components.Schemas["infra_apix_NoBody"]; ok {
		t.Error("components.schemas has infra_apix_NoBody")
	}
}
