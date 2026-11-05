package openapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	apix "github.com/Infra-Forge/infra-apix"
	"github.com/Infra-Forge/infra-apix/openapi"
	"github.com/getkin/kin-openapi/openapi3"
)

// The types below reproduce the consumer graphs whose in-memory documents
// failed validation with "found unresolved ref": project-finance
// (BudgetMetrics -> Money) and payroll (report -> project -> classification).

type crMoney struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type crBudgetMetrics struct {
	Budget   crMoney  `json:"budget" description:"Approved budget"`
	Spent    crMoney  `json:"spent" description:"Spent to date"`
	Variance crMoney  `json:"variance"`
	Forecast *crMoney `json:"forecast,omitempty"`
}

type crClassification struct {
	Code string `json:"code"`
}

type crProject struct {
	Classifications []crClassification `json:"classifications"`
}

type crReport struct {
	Project *crProject `json:"project,omitempty"`
}

type crReportList struct {
	Reports []crReport `json:"reports"`
}

type crComment struct {
	Body    string       `json:"body"`
	Replies []*crComment `json:"replies,omitempty"`
}

type crAnonymousTwice struct {
	First struct {
		X string `json:"x"`
	} `json:"first" description:"first use"`
	Second struct {
		X string `json:"x"`
	} `json:"second" description:"second use"`
}

const crRefPrefix = "#/components/schemas/openapi_test_"

func buildComponentRefsDoc(t *testing.T) *openapi3.T {
	t.Helper()
	apix.ResetRegistry()
	t.Cleanup(apix.ResetRegistry)

	routes := []struct {
		method apix.RouteMethod
		path   string
		req    reflect.Type
		status int
		resp   reflect.Type
	}{
		{apix.MethodGet, "/metrics", nil, http.StatusOK, reflect.TypeOf(crBudgetMetrics{})},
		{apix.MethodGet, "/money", nil, http.StatusOK, reflect.TypeOf(crMoney{})},
		{apix.MethodGet, "/reports", nil, http.StatusOK, reflect.TypeOf(crReportList{})},
		{apix.MethodGet, "/projects", nil, http.StatusOK, reflect.TypeOf(crProject{})},
		{apix.MethodPost, "/comments", reflect.TypeOf(crComment{}), http.StatusCreated, reflect.TypeOf(crComment{})},
		{apix.MethodGet, "/anonymous", nil, http.StatusOK, reflect.TypeOf(crAnonymousTwice{})},
	}
	for _, r := range routes {
		apix.RegisterRoute(&apix.RouteRef{
			Method:      r.method,
			Path:        r.path,
			RequestType: r.req,
			Responses:   map[int]*apix.ResponseRef{r.status: {ModelType: r.resp}},
		})
	}

	doc, err := openapi.NewBuilder().Build(apix.Snapshot())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return doc
}

func crComponent(t *testing.T, doc *openapi3.T, name string) *openapi3.Schema {
	t.Helper()
	ref, ok := doc.Components.Schemas["openapi_test_"+name]
	if !ok || ref.Value == nil {
		t.Fatalf("component openapi_test_%s missing", name)
	}
	return ref.Value
}

func crOperation(t *testing.T, doc *openapi3.T, method, path string) *openapi3.Operation {
	t.Helper()
	item := doc.Paths.Value(path)
	if item == nil || item.GetOperation(method) == nil {
		t.Fatalf("%s %s missing", method, path)
	}
	return item.GetOperation(method)
}

func crResponseSchema(t *testing.T, doc *openapi3.T, method, path string, status int) *openapi3.SchemaRef {
	t.Helper()
	resp := crOperation(t, doc, method, path).Responses.Status(status)
	if resp == nil || resp.Value == nil || resp.Value.Content["application/json"] == nil {
		t.Fatalf("%s %s: no JSON %d response", method, path, status)
	}
	return resp.Value.Content["application/json"].Schema
}

// assertRefsResolved walks every schema reachable from the document and fails
// on a $ref whose in-memory Value is missing or is not the component it names.
func assertRefsResolved(t *testing.T, doc *openapi3.T) {
	t.Helper()
	seen := map[*openapi3.Schema]bool{}
	var walk func(where string, ref *openapi3.SchemaRef)
	walk = func(where string, ref *openapi3.SchemaRef) {
		if ref == nil {
			return
		}
		if ref.Ref != "" {
			component, ok := doc.Components.Schemas[strings.TrimPrefix(ref.Ref, "#/components/schemas/")]
			switch {
			case ref.Value == nil:
				t.Errorf("%s: %s has no resolved value", where, ref.Ref)
			case !ok:
				t.Errorf("%s: %s names no component", where, ref.Ref)
			case component.Value != ref.Value:
				t.Errorf("%s: %s resolves to a schema other than its component", where, ref.Ref)
			}
		}
		s := ref.Value
		if s == nil || seen[s] {
			return
		}
		seen[s] = true
		for name, prop := range s.Properties {
			walk(where+"."+name, prop)
		}
		walk(where+"[]", s.Items)
		walk(where+"{}", s.AdditionalProperties.Schema)
		walk(where+".not", s.Not)
		for i, r := range s.AllOf {
			walk(fmt.Sprintf("%s.allOf[%d]", where, i), r)
		}
		for i, r := range s.OneOf {
			walk(fmt.Sprintf("%s.oneOf[%d]", where, i), r)
		}
		for i, r := range s.AnyOf {
			walk(fmt.Sprintf("%s.anyOf[%d]", where, i), r)
		}
	}

	for name, component := range doc.Components.Schemas {
		walk("components."+name, component)
	}
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			where := method + " " + path
			if op.RequestBody != nil && op.RequestBody.Value != nil {
				for ct, media := range op.RequestBody.Value.Content {
					walk(where+" request "+ct, media.Schema)
				}
			}
			for code, resp := range op.Responses.Map() {
				if resp.Value == nil {
					continue
				}
				for ct, media := range resp.Value.Content {
					walk(where+" "+code+" "+ct, media.Schema)
				}
			}
		}
	}
}

func TestBuildComponentRefsValidateInMemory(t *testing.T) {
	doc := buildComponentRefsDoc(t)

	assertRefsResolved(t, doc)
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("in-memory document must validate without a loader round-trip: %v", err)
	}
}

func TestBuildReferencesNamedTypesAtEveryUse(t *testing.T) {
	doc := buildComponentRefsDoc(t)
	metrics := crComponent(t, doc, "crBudgetMetrics")
	comments := crOperation(t, doc, http.MethodPost, "/comments")

	cases := []struct {
		name string
		ref  *openapi3.SchemaRef
		want string
	}{
		{"first use, response", crResponseSchema(t, doc, http.MethodGet, "/metrics", http.StatusOK), "crBudgetMetrics"},
		{"later use, response", crResponseSchema(t, doc, http.MethodGet, "/money", http.StatusOK), "crMoney"},
		{"first use, request body", comments.RequestBody.Value.Content["application/json"].Schema, "crComment"},
		{"field without metadata", metrics.Properties["variance"], "crMoney"},
		{"array items", crComponent(t, doc, "crProject").Properties["classifications"].Value.Items, "crClassification"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.ref)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if want := `{"$ref":"` + crRefPrefix + tc.want + `"}`; string(got) != want {
				t.Fatalf("schema must serialize as a reference\n got: %s\nwant: %s", got, want)
			}
		})
	}
}

func TestBuildFieldMetadataStaysOnTheField(t *testing.T) {
	doc := buildComponentRefsDoc(t)

	money := crComponent(t, doc, "crMoney")
	if money.Description != "" || money.Example != nil || money.Nullable {
		t.Fatalf("field metadata leaked into the shared component: description=%q example=%v nullable=%v",
			money.Description, money.Example, money.Nullable)
	}

	metrics := crComponent(t, doc, "crBudgetMetrics")
	for field, want := range map[string]string{"budget": "Approved budget", "spent": "Spent to date"} {
		prop := metrics.Properties[field].Value
		if prop.Description != want {
			t.Errorf("%s: description = %q, want %q", field, prop.Description, want)
		}
		if len(prop.AllOf) != 1 || prop.AllOf[0].Ref != crRefPrefix+"crMoney" {
			t.Errorf("%s: want allOf with a single crMoney reference, got %+v", field, prop.AllOf)
		}
	}

	forecast := metrics.Properties["forecast"].Value
	if !forecast.Nullable || len(forecast.AllOf) != 1 || forecast.AllOf[0].Ref != crRefPrefix+"crMoney" {
		t.Errorf("forecast: want nullable allOf with a single crMoney reference, got nullable=%v allOf=%+v",
			forecast.Nullable, forecast.AllOf)
	}

	anonymous := crComponent(t, doc, "crAnonymousTwice")
	for field, want := range map[string]string{"first": "first use", "second": "second use"} {
		if got := anonymous.Properties[field].Value.Description; got != want {
			t.Errorf("%s: description = %q, want %q", field, got, want)
		}
	}
}

func TestBuildRecursiveTypeSerializes(t *testing.T) {
	doc := buildComponentRefsDoc(t)

	items := crComponent(t, doc, "crComment").Properties["replies"].Value.Items.Value
	if !items.Nullable || len(items.AllOf) != 1 || items.AllOf[0].Ref != crRefPrefix+"crComment" {
		t.Fatalf("replies items: want nullable allOf referencing crComment, got nullable=%v allOf=%+v",
			items.Nullable, items.AllOf)
	}

	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"$ref":"`+crRefPrefix+`crComment"`) {
		t.Fatalf("serialized document has no reference back to crComment")
	}
}
