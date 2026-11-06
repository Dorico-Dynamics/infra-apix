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
- ✅ **Validation integration** - Respects `validate` and `binding` tags
- ✅ **Standard error responses** - Shared ErrorResponse schema

## Basic Generation

### Runtime Generation

Generate and serve the spec at runtime:

```go
import (
    "github.com/Infra-Forge/apix/runtime"
    "github.com/getkin/kin-openapi/openapi3"
)

handler, err := runtime.NewHandler(runtime.Config{
    Title:           "My API",
    Version:         "1.0.0",
    Description:     "API for managing users and resources",
    Format:          "json", // or "yaml"
    EnableSwaggerUI: true,
    Servers:         []string{"https://api.example.com"},
    Validate:        true, // Validate spec before serving
})

// Register with your framework
handler.RegisterEcho(e) // For Echo
// or
handler.RegisterHTTP(mux) // For other frameworks
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
    "github.com/Infra-Forge/apix"
    "github.com/Infra-Forge/apix/openapi"
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

`apix` respects standard Go struct tags for schema generation.

### JSON Tags

```go
type User struct {
    ID       string  `json:"id"`                    // Required field
    Name     string  `json:"name"`                  // Required field
    Email    *string `json:"email,omitempty"`       // Optional (pointer + omitempty)
    Age      int     `json:"age,omitempty"`         // Optional (omitempty)
    Internal string  `json:"-"`                     // Excluded from schema
    Bio      string  `json:"bio,omitempty"`         // Optional field
}
```

**Generated OpenAPI:**
```yaml
User:
  type: object
  required:
    - id
    - name
  properties:
    id:
      type: string
    name:
      type: string
    email:
      type: string
      nullable: true
    age:
      type: integer
    bio:
      type: string
```

### Validation Tags

```go
type CreateUserRequest struct {
    Name  string `json:"name" validate:"required"`
    Email string `json:"email" validate:"required,email"`
    Age   int    `json:"age" validate:"min=18,max=120"`
}
```

**Supported validators:**
- `required` - Marks field as required
- `min`, `max` - Sets minimum/maximum values
- `email`, `url` - Format validation (informational)

### Binding Tags (Gin)

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
User:
  type: object
  properties:
    id:
      type: string
      description: Unique user identifier
    name:
      type: string
      description: User's full name
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
Transaction:
  type: object
  required:
    - id
    - amount
    - date
  properties:
    id:
      type: string
      format: uuid
    amount:
      type: string  # decimal.Decimal
    date:
      type: string
      format: date-time
    updated_at:
      type: string
      format: date-time
      nullable: true
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
Address:
  type: object
  required:
    - street
    - city
    - country
  properties:
    street:
      type: string
    city:
      type: string
    country:
      type: string

User:
  type: object
  required:
    - id
    - name
    - address
  properties:
    id:
      type: string
    name:
      type: string
    address:
      $ref: '#/components/schemas/Address'
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
UserListResponse:
  type: object
  required:
    - users
    - total
  properties:
    users:
      type: array
      items:
        $ref: '#/components/schemas/User'
    total:
      type: integer
```

### Maps

```go
type Metadata struct {
    Tags map[string]string `json:"tags"`
}
```

**Generated OpenAPI:**
```yaml
Metadata:
  type: object
  required:
    - tags
  properties:
    tags:
      type: object
      additionalProperties:
        type: string
```

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

Rename one type of each pair. `Builder.Build` returns this error, and the runtime handler answers the spec endpoint with it and status 500.

## Security Schemes

### Defining Security Schemes

```go
import (
    "github.com/Infra-Forge/apix/runtime"
    "github.com/getkin/kin-openapi/openapi3"
)

handler, _ := runtime.NewHandler(runtime.Config{
    Title:   "My API",
    Version: "1.0.0",
    CustomizeBuilder: func(b *openapi.Builder) {
        b.SecuritySchemes = openapi3.SecuritySchemes{
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
                Value: openapi3.NewOAuth2SecurityScheme().
                    WithAuthorizationURL("https://auth.example.com/oauth/authorize").
                    WithTokenURL("https://auth.example.com/oauth/token").
                    WithScopes(map[string]string{
                        "read:users":  "Read user data",
                        "write:users": "Modify user data",
                    }),
            },
        }
    },
})
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

// Multiple security schemes
echoadapter.Post(adapter, "/api/admin/users", adminCreateUser,
    apix.WithSecurity("BearerAuth"),
    apix.WithSecurity("ApiKeyAuth"),
)
```

### Global Security

Apply security to all routes by default:

```go
handler, _ := runtime.NewHandler(runtime.Config{
    CustomizeBuilder: func(b *openapi.Builder) {
        b.SecuritySchemes = openapi3.SecuritySchemes{
            "BearerAuth": &openapi3.SecuritySchemeRef{
                Value: openapi3.NewJWTSecurityScheme(),
            },
        }
        
        // Apply to all routes
        b.GlobalSecurity = openapi3.SecurityRequirements{
            openapi3.SecurityRequirement{
                "BearerAuth": []string{},
            },
        }
    },
})
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
// Standard 400 and 500 errors
echoadapter.Post(adapter, "/api/users", createUser,
    apix.WithStandardErrors(),
)

// Add 404 error
echoadapter.Get(adapter, "/api/users/:id", getUser,
    apix.WithNotFoundError("User not found"),
)

// Custom error responses
echoadapter.Post(adapter, "/api/users", createUser,
    apix.WithResponse(http.StatusConflict, &ConflictError{}, 
        apix.WithDescription("User already exists")),
)
```

### Multiple Response Types

```go
echoadapter.Get(adapter, "/api/users/:id", getUser,
    apix.WithResponse(http.StatusOK, &UserResponse{}),
    apix.WithResponse(http.StatusNotFound, &ErrorResponse{},
        apix.WithDescription("User not found")),
    apix.WithResponse(http.StatusUnauthorized, &ErrorResponse{},
        apix.WithDescription("Authentication required")),
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
    apix.WithStandardErrors(), // Adds 400, 500
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

