# OpenAPI Generation Guide

Complete guide to generating OpenAPI 3.1 specifications with `apix`.

## Table of Contents

- [Overview](#overview)
- [Basic Generation](#basic-generation)
- [Struct Tags](#struct-tags)
- [Schema Customization](#schema-customization)
- [Security Schemes](#security-schemes)
- [Response Customization](#response-customization)
- [Advanced Features](#advanced-features)
- [Best Practices](#best-practices)

## Overview

`apix` generates OpenAPI 3.1 specifications directly from your Go code using:
- Type reflection for request/response schemas
- Struct tags for field metadata
- Route options for operation details
- Builder configuration for global settings

### Key Features

- ✅ **Deterministic output** - Sorted paths and components for git-friendly diffs
- ✅ **Type-safe** - Compile-time validation of handler signatures
- ✅ **Automatic schema generation** - From Go structs with full type support
- ✅ **Nullable detection** - Pointer types become nullable in OpenAPI
- ✅ **Required fields** - `validate:"required"` and `binding:"required"` mark properties required
- ✅ **Standard error responses** - Shared ErrorResponse schema

## Basic Generation

### Runtime Generation

Generate and serve the spec at runtime:

```go
import (
    "log"
    "net/http"

    "github.com/Infra-Forge/infra-apix/openapi"
    "github.com/Infra-Forge/infra-apix/runtime"
)

handler, err := runtime.NewHandler(runtime.Config{
    Title:           "My API",
    Version:         "1.0.0",
    Format:          "json", // or "yaml"
    EnableSwaggerUI: true,
    Servers:         []string{"https://api.example.com"},
    Validate:        true, // Validate spec before serving
    // Config has no description field; set it on the builder.
    CustomizeBuilder: func(b *openapi.Builder) {
        b.Info.Description = "API for managing users and resources"
    },
})
if err != nil {
    log.Fatal(err)
}

// Register with your framework
handler.RegisterEcho(e) // For Echo
// or, for other frameworks, fill a ServeMux and mount it on your router
mux := http.NewServeMux()
handler.RegisterHTTP(mux)
```

### CLI Generation

Generate static spec files:

```bash
# Generate YAML spec
apix generate \
  --title "My API" \
  --version "1.0.0" \
  --servers "https://api.example.com" \
  --out docs/openapi.yaml

# Generate JSON spec
apix generate \
  --format json \
  --out docs/openapi.json

# Output to stdout
apix generate --stdout
```

### Programmatic Generation

Generate specs in your code:

```go
import (
    apix "github.com/Infra-Forge/infra-apix"
    "github.com/Infra-Forge/infra-apix/openapi"
    "github.com/getkin/kin-openapi/openapi3"
)

// Get registered routes
routes := apix.Snapshot()

// Create builder
builder := openapi.NewBuilder()
builder.Info.Title = "My API"
builder.Info.Version = "1.0.0"
builder.Servers = openapi3.Servers{
    &openapi3.Server{URL: "https://api.example.com"},
}

// Build document
doc, err := builder.Build(routes)
if err != nil {
    log.Fatal(err)
}

// Encode to YAML or JSON
data, contentType, err := openapi.EncodeDocument(doc, "yaml")
```

## Struct Tags

`apix` reads the `json`, `validate`, `binding`, `description`, `example` and `format` struct tags. The generated schemas below are what the builder emits for these types in a package named `models` (see [Component Names](#component-names)).

### JSON Tags

A property is required when the field has no `omitempty` and is not a pointer, slice, map or interface, or when its tag says `validate:"required"` or `binding:"required"`. A pointer is also nullable.

```go
type User struct {
    ID       string  `json:"id"`                    // Required field
    Name     string  `json:"name"`                  // Required field
    Email    *string `json:"email,omitempty"`       // Optional and nullable (pointer)
    Age      int     `json:"age,omitempty"`         // Optional (omitempty)
    Internal string  `json:"-"`                     // Excluded from schema
    Bio      string  `json:"bio,omitempty"`         // Optional field
}
```

**Generated OpenAPI:**
```yaml
models_User:
  properties:
    age:
      format: int32
      type: integer
    bio:
      type: string
    email:
      nullable: true
      type: string
    id:
      type: string
    name:
      type: string
  required:
    - id
    - name
  type: object
```

### Validation Tags

`validate:"required"` marks a property required, even with `omitempty`. The builder reads no other rule: your validator enforces `min`, `max`, `email` and the rest at runtime, but they do not appear in the schema.

```go
type CreateUserRequest struct {
    Name  string `json:"name" validate:"required"`
    Email string `json:"email" validate:"required,email"`
    Age   int    `json:"age" validate:"min=18,max=120"`
}
```

**Generated OpenAPI:**
```yaml
models_CreateUserRequest:
  properties:
    age:
      format: int32
      type: integer
    email:
      type: string
    name:
      type: string
  required:
    - name
    - email
    - age
  type: object
```

`age` is required because it has no `omitempty`, not because of its rules.

### Binding Tags (Gin)

`binding:"required"` counts as `validate:"required"` does.

```go
type CreateUserRequest struct {
    Name  string `json:"name" binding:"required"`
    Email string `json:"email" binding:"required,email"`
}
```

### Description Tags

```go
type User struct {
    ID   string `json:"id" description:"Unique user identifier"`
    Name string `json:"name" description:"User's full name"`
}
```

**Generated OpenAPI:**
```yaml
models_User:
  properties:
    id:
      description: Unique user identifier
      type: string
    name:
      description: User's full name
      type: string
  required:
    - id
    - name
  type: object
```

## Schema Customization

### Complex Types

`apix` automatically handles complex Go types:

```go
import (
    "time"
    "github.com/google/uuid"
    "github.com/shopspring/decimal"
)

type Transaction struct {
    ID        uuid.UUID       `json:"id"`
    Amount    decimal.Decimal `json:"amount"`
    Date      time.Time       `json:"date"`
    UpdatedAt *time.Time      `json:"updated_at,omitempty"`
}
```

**Generated OpenAPI:**
```yaml
models_Transaction:
  properties:
    amount:
      description: Decimal number represented as string for precision
      example: "123.45"
      format: decimal
      type: string
    date:
      format: date-time
      type: string
    id:
      format: uuid
      type: string
    updated_at:
      format: date-time
      nullable: true
      type: string
  required:
    - id
    - amount
    - date
  type: object
```

### Nested Structs

```go
type Address struct {
    Street  string `json:"street"`
    City    string `json:"city"`
    Country string `json:"country"`
}

type User struct {
    ID      string  `json:"id"`
    Name    string  `json:"name"`
    Address Address `json:"address"`
}
```

**Generated OpenAPI:**
```yaml
models_Address:
  properties:
    city:
      type: string
    country:
      type: string
    street:
      type: string
  required:
    - street
    - city
    - country
  type: object
models_User:
  properties:
    address:
      $ref: '#/components/schemas/models_Address'
    id:
      type: string
    name:
      type: string
  required:
    - id
    - name
    - address
  type: object
```

### Arrays and Slices

```go
type UserListResponse struct {
    Users []User `json:"users"`
    Total int    `json:"total"`
}
```

**Generated OpenAPI:**
```yaml
models_UserListResponse:
  properties:
    total:
      format: int32
      type: integer
    users:
      items:
        $ref: '#/components/schemas/models_User'
      type: array
  required:
    - total
  type: object
```

A slice is optional unless its tag says `validate:"required"`.

### Maps

```go
type Metadata struct {
    Tags map[string]string `json:"tags"`
}
```

**Generated OpenAPI:**
```yaml
models_Metadata:
  properties:
    tags:
      additionalProperties:
        type: string
      type: object
  type: object
```

A map is optional unless its tag says `validate:"required"`.

### Enums

```go
type Status string

const (
    StatusActive   Status = "active"
    StatusInactive Status = "inactive"
    StatusPending  Status = "pending"
)

type User struct {
    ID     string `json:"id"`
    Status Status `json:"status"`
}
```

**Note:** Enum values are not automatically detected. Use description tags to document valid values.

### Component Names

A named struct becomes the component `<last import-path element>_<Type>`: `User` in `github.com/acme/billing/models` is `models_User`. Two types whose package names and type names match map to the same component, so the build fails and lists every such pair:

```text
GET /payslips: component name "models_Money" is used by both github.com/acme/billing/models.Money and github.com/acme/payroll/models.Money; rename one of the types
```

Rename one type of each pair. `Builder.Build` returns this error. The runtime handler logs it, with `reason="OpenAPI document build failed"`, and answers the spec endpoint with a plain 500; under `RegisterEcho` the error goes to Echo's `HTTPErrorHandler` instead.

## Security Schemes

### Defining Security Schemes

Declare every scheme that a route names with `apix.WithSecurity` in `Config.SecuritySchemes`. The build fails for a scheme a route names but nothing declares.

```go
import (
    "log"

    "github.com/Infra-Forge/infra-apix/runtime"
    "github.com/getkin/kin-openapi/openapi3"
)

handler, err := runtime.NewHandler(runtime.Config{
    Title:   "My API",
    Version: "1.0.0",
    SecuritySchemes: openapi3.SecuritySchemes{
        "BearerAuth": &openapi3.SecuritySchemeRef{
            Value: openapi3.NewJWTSecurityScheme(),
        },
        "ApiKeyAuth": &openapi3.SecuritySchemeRef{
            Value: openapi3.NewSecurityScheme().
                WithType("apiKey").
                WithIn("header").
                WithName("X-API-Key"),
        },
        "OAuth2": &openapi3.SecuritySchemeRef{
            Value: &openapi3.SecurityScheme{
                Type: "oauth2",
                Flows: &openapi3.OAuthFlows{
                    AuthorizationCode: &openapi3.OAuthFlow{
                        AuthorizationURL: "https://auth.example.com/oauth/authorize",
                        TokenURL:         "https://auth.example.com/oauth/token",
                        Scopes: map[string]string{
                            "read:users":  "Read user data",
                            "write:users": "Modify user data",
                        },
                    },
                },
            },
        },
    },
})
if err != nil {
    log.Fatal(err)
}
```

### Applying Security to Routes

```go
// Single security scheme
echoadapter.Post(adapter, "/api/users", createUser,
    apix.WithSecurity("BearerAuth"),
)

// With scopes (OAuth2)
echoadapter.Post(adapter, "/api/users", createUser,
    apix.WithSecurity("OAuth2", "write:users"),
)

// Either scheme: each WithSecurity adds an alternative requirement
echoadapter.Post(adapter, "/api/admin/users", adminCreateUser,
    apix.WithSecurity("BearerAuth"),
    apix.WithSecurity("ApiKeyAuth"),
)
```

### Global Security

Apply security to all routes by default:

```go
handler, err := runtime.NewHandler(runtime.Config{
    SecuritySchemes: openapi3.SecuritySchemes{
        "BearerAuth": &openapi3.SecuritySchemeRef{
            Value: openapi3.NewJWTSecurityScheme(),
        },
    },
    CustomizeBuilder: func(b *openapi.Builder) {
        // Apply to all routes
        b.GlobalSecurity = openapi3.SecurityRequirements{
            openapi3.SecurityRequirement{
                "BearerAuth": []string{},
            },
        }
    },
})
if err != nil {
    log.Fatal(err)
}
```

## Response Customization

### Custom Status Codes

```go
echoadapter.Post(adapter, "/api/users", createUser,
    apix.WithSuccessStatus(http.StatusCreated), // 201 instead of default
)
```

### Custom Headers

```go
echoadapter.Post(adapter, "/api/users", createUser,
    apix.WithSuccessHeaders(http.StatusCreated, apix.HeaderRef{
        Name:        "Location",
        Description: "URI of the created resource",
        SchemaType:  "string",
        Required:    true,
    }),
)
```

### Error Responses

```go
// 400, 422 and 500 with the shared ErrorResponse schema
echoadapter.Post(adapter, "/api/users", createUser,
    apix.WithStandardErrors(),
)

// Add 404 error
echoadapter.Get(adapter, "/api/users/:id", getUser,
    apix.WithNotFoundError("User not found"),
)

// An error response with your own model
echoadapter.Post(adapter, "/api/users", createUser,
    apix.WithCustomErrorResponse(http.StatusConflict, ConflictError{}, "User already exists"),
)
```

### Multiple Response Types

The handler's return type documents the success response. Error options add the other statuses:

```go
echoadapter.Get(adapter, "/api/users/:id", getUser,
    apix.WithNotFoundError("User not found"),
    apix.WithUnauthorizedError("Authentication required"),
)
```

## Advanced Features

### Custom Operation IDs

```go
echoadapter.Post(adapter, "/api/users", createUser,
    apix.WithOperationID("createUser"),
)
```

### Tags for Grouping

```go
echoadapter.Post(adapter, "/api/users", createUser,
    apix.WithTags("Users", "Authentication"),
)
```

### Deprecation

```go
echoadapter.Get(adapter, "/api/v1/users", listUsersV1,
    apix.WithDeprecated(),
    apix.WithDescription("This endpoint is deprecated. Use /api/v2/users instead."),
)
```

### Parameters

```go
echoadapter.Get(adapter, "/api/users", listUsers,
    apix.WithParameter(apix.Parameter{
        Name:        "page",
        In:          "query",
        Description: "Page number for pagination",
        SchemaType:  "integer",
        Required:    false,
        Example:     1,
    }),
    apix.WithParameter(apix.Parameter{
        Name:        "per_page",
        In:          "query",
        Description: "Items per page",
        SchemaType:  "integer",
        Required:    false,
        Example:     20,
    }),
    apix.WithParameter(apix.Parameter{
        Name:        "X-Request-ID",
        In:          "header",
        Description: "Request correlation ID",
        SchemaType:  "string",
        Required:    false,
    }),
)
```

## Best Practices

### 1. Use Descriptive Names

```go
// Good
type CreateUserRequest struct { ... }
type UserResponse struct { ... }

// Avoid
type Request struct { ... }
type Response struct { ... }
```

### 2. Document Everything

```go
echoadapter.Post(adapter, "/api/users", createUser,
    apix.WithSummary("Create a new user"),
    apix.WithDescription("Creates a new user account with the provided information. Email must be unique."),
    apix.WithTags("Users"),
)
```

### 3. Use Standard Error Responses

```go
echoadapter.Post(adapter, "/api/users", createUser,
    apix.WithStandardErrors(), // Adds 400, 422, 500
    apix.WithNotFoundError("User not found"), // Adds 404
)
```

### 4. Validate Generated Specs

```go
handler, _ := runtime.NewHandler(runtime.Config{
    Validate: true, // Enable validation
})
```

### 5. Version Your API

```go
handler, _ := runtime.NewHandler(runtime.Config{
    Title:   "My API",
    Version: "2.0.0", // Semantic versioning
})
```

### 6. Use CI Drift Detection

```yaml
# .github/workflows/ci.yml
- name: Check OpenAPI spec drift
  run: apix spec-guard --existing docs/openapi.yaml
```

### 7. Runtime Specs Are Built Once

The runtime handler builds the document on the first request and keeps it, or
the error that stopped the build, until a route or plugin is registered or
removed. Set `CacheTTL` only when a plugin's `OnSpecBuild` output changes over
time:

```go
handler, _ := runtime.NewHandler(runtime.Config{
    CacheTTL: 5 * time.Minute, // Also rebuild the document every 5 minutes
})
```

### 8. Organize with Tags

```go
// Group related operations
apix.WithTags("Users", "Management")
apix.WithTags("Authentication")
apix.WithTags("Billing", "Payments")
```

### 9. Use Nullable Types Correctly

```go
type User struct {
    ID    string  `json:"id"`           // Required
    Email *string `json:"email,omitempty"` // Optional, nullable
    Age   int     `json:"age,omitempty"`   // Optional, not nullable
}
```

### 10. Document Security Requirements

```go
echoadapter.Post(adapter, "/api/admin/users", adminAction,
    apix.WithSecurity("BearerAuth", "admin:write"),
    apix.WithDescription("Requires admin privileges"),
)
```

