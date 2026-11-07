package openapi

import (
	"reflect"
	"testing"

	apix "github.com/Infra-Forge/infra-apix"
	"github.com/getkin/kin-openapi/openapi3"
)

func newTestBuilder() *Builder {
	b := NewBuilder()
	b.doc = &openapi3.T{Components: &openapi3.Components{Schemas: map[string]*openapi3.SchemaRef{}}}
	b.schemaCache = make(map[reflect.Type]*openapi3.SchemaRef)
	return b
}

func TestSchemaRefFromTypePrimitives(t *testing.T) {
	b := newTestBuilder()
	cases := []struct {
		name   string
		typeOf interface{}
		want   string
	}{
		{"bool", true, "boolean"},
		{"int32", int32(0), "integer"},
		{"float", float32(0), "number"},
		{"string", "", "string"},
	}

	for _, tc := range cases {
		ref, err := b.schemaRefFromType(reflect.TypeOf(tc.typeOf))
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.name, err)
		}
		if ref.Value == nil {
			t.Fatalf("%s: schema value nil", tc.name)
		}
		if ref.Value.Type == nil || len(*ref.Value.Type) == 0 {
			t.Fatalf("%s: type not set", tc.name)
		}
	}
}

// TestIntegerFormatsHoldTheGoRange: each integer kind's format holds every
// value of its Go type on any build platform: int and uint are 64-bit on amd64
// and arm64, and uint32 exceeds int32.
func TestIntegerFormatsHoldTheGoRange(t *testing.T) {
	b := newTestBuilder()
	cases := []struct {
		value  any
		format string
	}{
		{int(0), "int64"},
		{int8(0), "int32"},
		{int16(0), "int32"},
		{int32(0), "int32"},
		{int64(0), "int64"},
		{uint(0), "int64"},
		{uint8(0), "int32"},
		{uint16(0), "int32"},
		{uint32(0), "int64"},
		{uint64(0), "int64"},
	}
	for _, tc := range cases {
		typ := reflect.TypeOf(tc.value)
		ref, err := b.schemaRefFromType(typ)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", typ, err)
		}
		if !ref.Value.Type.Is("integer") || ref.Value.Format != tc.format {
			t.Errorf("%s: type %v, format %q; want integer, %q", typ, ref.Value.Type, ref.Value.Format, tc.format)
		}
	}
}

func TestSchemaRefFromTypePointerNullable(t *testing.T) {
	b := newTestBuilder()
	ref, err := b.schemaRefFromType(reflect.TypeOf(&struct{ Name string }{}))
	if err != nil {
		t.Fatalf("pointer schema: %v", err)
	}
	if ref.Value == nil || !ref.Value.Nullable {
		t.Fatalf("expected nullable schema")
	}
}

func TestSchemaRefFromTypeMapError(t *testing.T) {
	b := newTestBuilder()
	_, err := b.schemaRefFromType(reflect.TypeOf(map[int]string{}))
	if err == nil {
		t.Fatalf("expected error for map with non-string key")
	}
}

func TestWrapNullable(t *testing.T) {
	t.Run("inline schema is cloned", func(t *testing.T) {
		s := openapi3.NewStringSchema()
		wrapped := wrapNullable(schemaRef(s))
		if wrapped.Value == nil || !wrapped.Value.Nullable {
			t.Fatalf("expected nullable clone")
		}
		if s.Nullable {
			t.Fatalf("original schema was edited")
		}
	})

	t.Run("component reference is wrapped", func(t *testing.T) {
		component := openapi3.NewObjectSchema()
		ref := componentRef("pkg_Money", component)
		wrapped := wrapNullable(ref)
		if wrapped.Ref != "" || wrapped.Value == nil || !wrapped.Value.Nullable {
			t.Fatalf("expected a nullable wrapper, got %+v", wrapped)
		}
		if len(wrapped.Value.AllOf) != 1 || wrapped.Value.AllOf[0] != ref {
			t.Fatalf("expected allOf holding the reference, got %+v", wrapped.Value.AllOf)
		}
		if component.Nullable {
			t.Fatalf("component was edited")
		}
	})

	t.Run("nil", func(t *testing.T) {
		if wrapNullable(nil) != nil {
			t.Fatalf("expected nil")
		}
	})
}

func TestWithFieldMetadata(t *testing.T) {
	component := openapi3.NewObjectSchema()
	cases := []struct {
		name        string
		ref         *openapi3.SchemaRef
		description string
		example     any
		wantWrapped bool
	}{
		{"no metadata keeps the schema", schemaRef(openapi3.NewStringSchema()), "", nil, false},
		{"inline schema takes the metadata", schemaRef(openapi3.NewStringSchema()), "Name", "Ada", false},
		{"component reference is wrapped", componentRef("pkg_Money", component), "Budget", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := withFieldMetadata(tc.ref, tc.description, tc.example)
			if !tc.wantWrapped {
				if got != tc.ref {
					t.Fatalf("expected the same schema ref back")
				}
				if got.Value.Description != tc.description || got.Value.Example != tc.example {
					t.Fatalf("metadata = (%q, %v), want (%q, %v)", got.Value.Description, got.Value.Example, tc.description, tc.example)
				}
				return
			}
			if got.Ref != "" || len(got.Value.AllOf) != 1 || got.Value.AllOf[0] != tc.ref {
				t.Fatalf("expected an allOf wrapper holding the reference, got %+v", got)
			}
			if got.Value.Description != tc.description {
				t.Fatalf("description = %q, want %q", got.Value.Description, tc.description)
			}
		})
	}
	if component.Description != "" {
		t.Fatalf("component was edited: %q", component.Description)
	}
	if withFieldMetadata(nil, "x", nil) != nil {
		t.Fatalf("expected nil for a nil ref")
	}
}

func TestHeaderRefSetsSchema(t *testing.T) {
	hdr := apix.HeaderRef{Name: "X-Test", SchemaType: "integer", Required: true}
	ref := headerRef(hdr)
	if ref == nil || ref.Value == nil || ref.Value.Schema == nil || ref.Value.Schema.Value == nil {
		t.Fatalf("header schema not created")
	}
	if ref.Value.Schema.Value.Type == nil || len(*ref.Value.Schema.Value.Type) == 0 {
		t.Fatalf("expected schema type set")
	}
}

func TestDefaultResponseDescription(t *testing.T) {
	cases := map[int]string{
		200: "OK",
		201: "Created",
		204: "No Content",
		400: "Bad Request",
		404: "Not Found",
		500: "Internal Server Error",
		418: "",
	}
	for status, want := range cases {
		if got := defaultResponseDescription(status); got != want {
			t.Fatalf("status %d: expected %q, got %q", status, want, got)
		}
	}
}
