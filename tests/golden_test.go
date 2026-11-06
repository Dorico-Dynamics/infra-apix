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

type User struct {
	ID       string   `json:"id"`
	Username string   `json:"username" validate:"required"`
	Email    string   `json:"email" validate:"required"`
	Age      *int     `json:"age,omitempty"`
	Active   bool     `json:"active"`
	Tags     []string `json:"tags,omitempty"`
}

type CreateUserRequest struct {
	Username string `json:"username" validate:"required"`
	Email    string `json:"email" validate:"required"`
	Password string `json:"password" validate:"required"`
}

type UpdateUserRequest struct {
	Username *string `json:"username,omitempty"`
	Email    *string `json:"email,omitempty"`
}

type ErrorDetail struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func getSchemaNames(doc *openapi3.T) []string {
	names := make([]string, 0, len(doc.Components.Schemas))
	for name := range doc.Components.Schemas {
		names = append(names, name)
	}
	return names
}

func TestGoldenCRUDSpec(t *testing.T) {
	apix.ResetRegistry()

	// Register CRUD routes
	apix.RegisterRoute(&apix.RouteRef{
		Method:      apix.MethodPost,
		Path:        "/api/users",
		Summary:     "Create user",
		Description: "Creates a new user account",
		Tags:        []string{"users"},
		RequestType: reflect.TypeOf(CreateUserRequest{}),
		Responses: map[int]*apix.ResponseRef{
			http.StatusCreated: {
				ModelType:   reflect.TypeOf(User{}),
				Description: "User created successfully",
			},
		},
		Security: []apix.SecurityRequirement{
			{Name: "BearerAuth", Scopes: []string{"users:write"}},
		},
		SuccessHeaders: map[int][]apix.HeaderRef{
			http.StatusCreated: {{Name: "Location", Description: "URI of the created user", SchemaType: "string", Required: true}},
		},
	})

	apix.RegisterRoute(&apix.RouteRef{
		Method:      apix.MethodGet,
		Path:        "/api/users/{id}",
		Summary:     "Get user",
		Description: "Retrieves a user by ID",
		Tags:        []string{"users"},
		Responses: map[int]*apix.ResponseRef{
			http.StatusOK: {
				ModelType:   reflect.TypeOf(User{}),
				Description: "User found",
			},
			http.StatusNotFound: {
				ModelType:   reflect.TypeOf(apix.ErrorResponse{}),
				Description: "User not found",
			},
		},
		Parameters: []apix.Parameter{
			{Name: "id", In: "path", Required: true, SchemaType: "string", Description: "User ID"},
		},
	})

	apix.RegisterRoute(&apix.RouteRef{
		Method:      apix.MethodPut,
		Path:        "/api/users/{id}",
		Summary:     "Update user",
		Description: "Updates an existing user",
		Tags:        []string{"users"},
		RequestType: reflect.TypeOf(UpdateUserRequest{}),
		Responses: map[int]*apix.ResponseRef{
			http.StatusOK: {
				ModelType:   reflect.TypeOf(User{}),
				Description: "User updated successfully",
			},
		},
		Parameters: []apix.Parameter{
			{Name: "id", In: "path", Required: true, SchemaType: "string", Description: "User ID"},
		},
		Security: []apix.SecurityRequirement{
			{Name: "BearerAuth", Scopes: []string{"users:write"}},
		},
	})

	apix.RegisterRoute(&apix.RouteRef{
		Method:      apix.MethodDelete,
		Path:        "/api/users/{id}",
		Summary:     "Delete user",
		Description: "Deletes a user account",
		Tags:        []string{"users"},
		Responses: map[int]*apix.ResponseRef{
			http.StatusNoContent: {Description: "User deleted successfully"},
		},
		Parameters: []apix.Parameter{
			{Name: "id", In: "path", Required: true, SchemaType: "string", Description: "User ID"},
		},
		Security: []apix.SecurityRequirement{
			{Name: "BearerAuth", Scopes: []string{"users:delete"}},
		},
	})

	// Build spec
	builder := openapi.NewBuilder()
	builder.Info.Title = "User Management API"
	builder.Info.Version = "1.0.0"
	builder.Info.Description = "API for managing user accounts"
	builder.Servers = append(builder.Servers, &openapi3.Server{
		URL:         "https://api.example.com",
		Description: "Production server",
	})
	builder.SecuritySchemes = openapi3.SecuritySchemes{
		"BearerAuth": &openapi3.SecuritySchemeRef{
			Value: openapi3.NewJWTSecurityScheme(),
		},
	}

	doc, err := builder.Build(apix.Snapshot())
	if err != nil {
		t.Fatalf("failed to build spec: %v", err)
	}

	// Validate spec structure
	if doc.OpenAPI != "3.1.0" {
		t.Errorf("expected OpenAPI 3.1.0, got %s", doc.OpenAPI)
	}

	if doc.Info.Title != "User Management API" {
		t.Errorf("expected title 'User Management API', got %s", doc.Info.Title)
	}

	// We have 2 unique paths: /api/users and /api/users/{id}
	// The latter has GET, PUT, DELETE methods
	if doc.Paths.Len() != 2 {
		t.Errorf("expected 2 paths, got %d", doc.Paths.Len())
	}

	// Validate POST /api/users
	postPath := doc.Paths.Value("/api/users")
	if postPath == nil || postPath.Post == nil {
		t.Fatal("expected POST /api/users")
	}

	if postPath.Post.Summary != "Create user" {
		t.Errorf("expected summary 'Create user', got %s", postPath.Post.Summary)
	}

	if postPath.Post.RequestBody == nil {
		t.Fatal("expected request body")
	}

	if postPath.Post.Responses.Status(http.StatusCreated) == nil {
		t.Error("expected 201 response")
	}

	// Validate security defaults (401/403 auto-injected)
	if postPath.Post.Responses.Status(http.StatusUnauthorized) == nil {
		t.Error("expected 401 response for secured endpoint")
	}
	if postPath.Post.Responses.Status(http.StatusForbidden) == nil {
		t.Error("expected 403 response for secured endpoint")
	}

	// Validate the Location header the route declares on 201
	created := postPath.Post.Responses.Status(http.StatusCreated)
	if location := created.Value.Headers["Location"]; location == nil || !location.Value.Required {
		t.Error("expected the declared, required Location header on 201 response")
	}

	// Validate GET /api/users/{id}
	getPath := doc.Paths.Value("/api/users/{id}")
	if getPath == nil || getPath.Get == nil {
		t.Fatal("expected GET /api/users/{id}")
	}

	if len(getPath.Get.Parameters) != 1 {
		t.Errorf("expected 1 parameter, got %d", len(getPath.Get.Parameters))
	}

	if getPath.Get.Parameters[0].Value.Name != "id" {
		t.Errorf("expected parameter 'id', got %s", getPath.Get.Parameters[0].Value.Name)
	}

	if getPath.Get.Responses.Status(http.StatusOK) == nil {
		t.Error("expected 200 response")
	}

	if getPath.Get.Responses.Status(http.StatusNotFound) == nil {
		t.Error("expected 404 response")
	}

	// Validate PUT /api/users/{id}
	putPath := doc.Paths.Value("/api/users/{id}")
	if putPath == nil || putPath.Put == nil {
		t.Fatal("expected PUT /api/users/{id}")
	}

	// Validate DELETE /api/users/{id}
	if putPath.Delete == nil {
		t.Fatal("expected DELETE /api/users/{id}")
	}

	if putPath.Delete.Responses.Status(http.StatusNoContent) == nil {
		t.Error("expected 204 response for DELETE")
	}

	// Validate components/schemas
	if doc.Components.Schemas == nil {
		t.Fatal("expected schemas in components")
	}

	// Should have User, CreateUserRequest, UpdateUserRequest, ErrorResponse schemas
	// Schema names may have package prefixes like "tests_test_User" or "apix_ErrorResponse"
	expectedSchemas := []string{"User", "CreateUserRequest", "UpdateUserRequest", "ErrorResponse"}
	for _, schemaName := range expectedSchemas {
		found := false
		for name := range doc.Components.Schemas {
			// Check if schema name ends with the expected name (handles package prefixes)
			if name == schemaName || strings.HasSuffix(name, "_"+schemaName) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected schema containing %s in components (found: %v)", schemaName, getSchemaNames(doc))
		}
	}

	// Find User schema (may have package prefix)
	var userSchema *openapi3.SchemaRef
	for name, schema := range doc.Components.Schemas {
		if name == "User" || strings.HasSuffix(name, "_User") {
			userSchema = schema
			break
		}
	}
	if userSchema == nil || userSchema.Value == nil {
		t.Fatalf("expected User schema (found: %v)", getSchemaNames(doc))
	}

	expectedFields := []string{"id", "username", "email", "age", "active", "tags"}
	for _, field := range expectedFields {
		if userSchema.Value.Properties[field] == nil {
			t.Errorf("expected field %s in User schema", field)
		}
	}

	// Validate required fields
	if len(userSchema.Value.Required) < 2 {
		t.Error("expected at least 2 required fields in User schema")
	}

	// Validate security schemes
	if doc.Components.SecuritySchemes == nil || doc.Components.SecuritySchemes["BearerAuth"] == nil {
		t.Error("expected BearerAuth security scheme")
	}
}

func TestGoldenDeterministicOutput(t *testing.T) {
	// Build the same spec twice and ensure identical output
	buildSpec := func() *openapi3.T {
		apix.ResetRegistry()

		apix.RegisterRoute(&apix.RouteRef{
			Method:  apix.MethodGet,
			Path:    "/api/items",
			Summary: "List items",
			Tags:    []string{"items"},
			Responses: map[int]*apix.ResponseRef{
				http.StatusOK: {ModelType: reflect.TypeOf([]Item{})},
			},
		})

		apix.RegisterRoute(&apix.RouteRef{
			Method:      apix.MethodPost,
			Path:        "/api/items",
			Summary:     "Create item",
			Tags:        []string{"items"},
			RequestType: reflect.TypeOf(CreateItemRequest{}),
			Responses: map[int]*apix.ResponseRef{
				http.StatusCreated: {ModelType: reflect.TypeOf(Item{})},
			},
		})

		builder := openapi.NewBuilder()
		builder.Info.Title = "Test API"
		builder.Info.Version = "1.0.0"

		doc, _ := builder.Build(apix.Snapshot())
		return doc
	}

	spec1 := buildSpec()
	spec2 := buildSpec()

	// Compare paths (should be in same order)
	paths1 := spec1.Paths.InMatchingOrder()
	paths2 := spec2.Paths.InMatchingOrder()

	if len(paths1) != len(paths2) {
		t.Fatalf("path count mismatch: %d vs %d", len(paths1), len(paths2))
	}

	for i := range paths1 {
		if paths1[i] != paths2[i] {
			t.Errorf("path order mismatch at index %d: %s vs %s", i, paths1[i], paths2[i])
		}
	}

	// Compare schema names (should be in same order)
	schemas1 := make([]string, 0, len(spec1.Components.Schemas))
	for name := range spec1.Components.Schemas {
		schemas1 = append(schemas1, name)
	}

	schemas2 := make([]string, 0, len(spec2.Components.Schemas))
	for name := range spec2.Components.Schemas {
		schemas2 = append(schemas2, name)
	}

	if len(schemas1) != len(schemas2) {
		t.Fatalf("schema count mismatch: %d vs %d", len(schemas1), len(schemas2))
	}
}
