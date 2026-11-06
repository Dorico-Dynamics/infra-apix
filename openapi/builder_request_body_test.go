package openapi_test

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	apix "github.com/Infra-Forge/infra-apix"
	"github.com/Infra-Forge/infra-apix/openapi"
)

type rbListQuery struct {
	Department string `query:"department"`
	Limit      int    `query:"limit"`
}

type rbItem struct {
	ID string `json:"id"`
}

// TestBuildDocumentsNoRequestBodyForGETOrDELETE: a request type on GET or
// DELETE carries query or path values, as payroll's query structs do, so it is
// not documented as a JSON body. The other methods keep theirs.
func TestBuildDocumentsNoRequestBodyForGETOrDELETE(t *testing.T) {
	cases := []struct {
		method   apix.RouteMethod
		wantBody bool
	}{
		{apix.MethodGet, false},
		{"get", false},
		{apix.MethodDelete, false},
		{apix.MethodPost, true},
		{apix.MethodPut, true},
		{apix.MethodPatch, true},
	}
	for _, tc := range cases {
		t.Run(string(tc.method), func(t *testing.T) {
			ref := &apix.RouteRef{
				Method:             tc.method,
				Path:               "/items",
				RequestType:        reflect.TypeOf(rbListQuery{}),
				RequestContentType: "application/json",
				Responses:          map[int]*apix.ResponseRef{http.StatusOK: {ModelType: reflect.TypeOf(rbItem{})}},
			}
			doc, err := openapi.NewBuilder().Build([]*apix.RouteRef{ref})
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			op := doc.Paths.Value("/items").GetOperation(strings.ToUpper(string(tc.method)))
			if op == nil {
				t.Fatal("operation missing")
			}
			if gotBody := op.RequestBody != nil; gotBody != tc.wantBody {
				t.Errorf("request body documented = %v, want %v", gotBody, tc.wantBody)
			}
			if err := doc.Validate(t.Context()); err != nil {
				t.Errorf("document does not validate: %v", err)
			}
		})
	}
}

// TestBuildDocumentsNoExplicitRequestModelForGET: an explicit request model is
// left out on GET as well.
func TestBuildDocumentsNoExplicitRequestModelForGET(t *testing.T) {
	ref := &apix.RouteRef{Method: apix.MethodGet, Path: "/items"}
	apix.WithExplicitRequestModel(rbListQuery{}, "application/json")(ref)
	apix.EnsureResponse(ref, http.StatusOK, reflect.TypeOf(rbItem{}))

	doc, err := openapi.NewBuilder().Build([]*apix.RouteRef{ref})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if body := doc.Paths.Value("/items").Get.RequestBody; body != nil {
		t.Errorf("GET documents a request body: %+v", body.Value)
	}
}
