# Getting Started with apix

Complete guide to get started with `apix` for OpenAPI 3.1 generation in Go.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Your First API](#your-first-api)
- [Adding More Routes](#adding-more-routes)
- [Serving OpenAPI Spec](#serving-openapi-spec)
- [Generating Static Specs](#generating-static-specs)
- [Next Steps](#next-steps)

## Prerequisites

- Go 1.27.3 or later
- Basic understanding of Go and REST APIs
- Familiarity with one of the supported frameworks:
  - Echo
  - Chi
  - Gorilla/Mux
  - Gin
  - Fiber

## Installation

### Install the Library

```bash
go get github.com/Infra-Forge/infra-apix
```

### Install Your Framework

Choose one:

```bash
# Echo
go get github.com/labstack/echo/v4

# Chi
go get github.com/go-chi/chi/v5

# Gorilla/Mux
go get github.com/gorilla/mux

# Gin
go get github.com/gin-gonic/gin

# Fiber
go get github.com/gofiber/fiber/v3
```

### Install CLI Tool (Optional)

```bash
go install github.com/Infra-Forge/infra-apix/cmd/apix@latest
```

## Quick Start

Let's build a simple user management API with Echo.

### Step 1: Create Your Project

```bash
mkdir my-api
cd my-api
go mod init github.com/yourusername/my-api
go get github.com/Infra-Forge/infra-apix
go get github.com/labstack/echo/v4
```

### Step 2: Define Your Models

Create `main.go`:

```go
package main

import (
    "context"
)

// Request model
type CreateUserRequest struct {
    Name  string `json:"name" validate:"required"`
    Email string `json:"email" validate:"required,email"`
    Age   int    `json:"age,omitempty"`
}

// Response model
type UserResponse struct {
    ID        string `json:"id"`
    Name      string `json:"name"`
    Email     string `json:"email"`
    Age       int    `json:"age,omitempty"`
    CreatedAt string `json:"created_at"`
}
```

### Step 3: Implement Your Handler

```go
import (
    "time"
    "github.com/google/uuid"
)

func createUser(ctx context.Context, req *CreateUserRequest) (UserResponse, error) {
    // In a real app, you'd save to a database
    return UserResponse{
        ID:        uuid.New().String(),
        Name:      req.Name,
        Email:     req.Email,
        Age:       req.Age,
        CreatedAt: time.Now().Format(time.RFC3339),
    }, nil
}
```

### Step 4: Set Up Echo and apix

```go
import (
    "log"
    
    apix "github.com/Infra-Forge/infra-apix"
    echoadapter "github.com/Infra-Forge/infra-apix/echo"
    "github.com/Infra-Forge/infra-apix/runtime"
    "github.com/labstack/echo/v4"
    "github.com/labstack/echo/v4/middleware"
)

func main() {
    // Create Echo instance
    e := echo.New()
    e.Use(middleware.Logger())
    e.Use(middleware.Recover())
    
    // Create apix adapter
    adapter := echoadapter.New(e)
    
    // Register route
    echoadapter.Post(adapter, "/api/users", createUser,
        apix.WithSummary("Create a new user"),
        apix.WithDescription("Creates a new user account with the provided information"),
        apix.WithTags("Users"),
        apix.WithStandardErrors(),
    )
    
    // Serve OpenAPI spec
    handler, err := runtime.NewHandler(runtime.Config{
        Title:           "My API",
        Version:         "1.0.0",
        EnableSwaggerUI: true,
    })
    if err != nil {
        log.Fatal(err)
    }
    handler.RegisterEcho(e)
    
    // Start server
    log.Println("Server starting on :8080")
    log.Println("OpenAPI spec: http://localhost:8080/openapi.json")
    log.Println("Swagger UI: http://localhost:8080/swagger")
    e.Start(":8080")
}
```

### Step 5: Run Your API

```bash
go run main.go
```

Visit:
- **Swagger UI**: http://localhost:8080/swagger
- **OpenAPI Spec**: http://localhost:8080/openapi.json

### Step 6: Test Your API

```bash
curl -X POST http://localhost:8080/api/users \
  -H "Content-Type: application/json" \
  -d '{"name":"John Doe","email":"john@example.com","age":30}'
```

## Your First API

Now let's build a complete CRUD API.

### Define All Models

```go
// List request (no body)
type ListUsersRequest = apix.NoBody

// List response
type UserListResponse struct {
    Users []UserResponse `json:"users"`
    Total int            `json:"total"`
}

// Update request
type UpdateUserRequest struct {
    Name  *string `json:"name,omitempty"`
    Email *string `json:"email,omitempty"`
    Age   *int    `json:"age,omitempty"`
}

// Delete request (no body)
type DeleteUserRequest = apix.NoBody

// Delete response (no body)
type DeleteUserResponse = apix.NoBody
```

### Implement All Handlers

```go
func listUsers(ctx context.Context, _ *apix.NoBody) (UserListResponse, error) {
    // In a real app, fetch from database
    users := []UserResponse{
        {
            ID:        "user-1",
            Name:      "John Doe",
            Email:     "john@example.com",
            Age:       30,
            CreatedAt: time.Now().Format(time.RFC3339),
        },
    }
    
    return UserListResponse{
        Users: users,
        Total: len(users),
    }, nil
}

func getUser(ctx context.Context, _ *apix.NoBody) (UserResponse, error) {
    // In a real app, fetch from database using path parameter
    return UserResponse{
        ID:        "user-1",
        Name:      "John Doe",
        Email:     "john@example.com",
        Age:       30,
        CreatedAt: time.Now().Format(time.RFC3339),
    }, nil
}

func updateUser(ctx context.Context, req *UpdateUserRequest) (UserResponse, error) {
    // In a real app, update in database
    return UserResponse{
        ID:        "user-1",
        Name:      *req.Name,
        Email:     "john@example.com",
        Age:       30,
        CreatedAt: time.Now().Format(time.RFC3339),
    }, nil
}

func deleteUser(ctx context.Context, _ *apix.NoBody) (apix.NoBody, error) {
    // In a real app, delete from database
    return apix.NoBody{}, nil
}
```

## Adding More Routes

### Register All CRUD Routes

```go
func main() {
    e := echo.New()
    e.Use(middleware.Logger())
    e.Use(middleware.Recover())
    
    adapter := echoadapter.New(e)
    
    // CREATE
    echoadapter.Post(adapter, "/api/users", createUser,
        apix.WithSummary("Create a new user"),
        apix.WithTags("Users"),
        apix.WithStandardErrors(),
    )
    
    // READ (list)
    echoadapter.Get(adapter, "/api/users", listUsers,
        apix.WithSummary("List all users"),
        apix.WithTags("Users"),
        apix.WithParameter(apix.Parameter{
            Name:        "page",
            In:          "query",
            Description: "Page number",
            SchemaType:  "integer",
            Required:    false,
            Example:     1,
        }),
    )
    
    // READ (single)
    echoadapter.Get(adapter, "/api/users/:id", getUser,
        apix.WithSummary("Get user by ID"),
        apix.WithTags("Users"),
        apix.WithNotFoundError("User not found"),
        apix.WithParameter(apix.Parameter{
            Name:       "id",
            In:         "path",
            SchemaType: "string",
            Required:   true,
        }),
    )
    
    // UPDATE
    echoadapter.Put(adapter, "/api/users/:id", updateUser,
        apix.WithSummary("Update user"),
        apix.WithTags("Users"),
        apix.WithNotFoundError("User not found"),
        apix.WithStandardErrors(),
    )
    
    // DELETE
    echoadapter.Delete(adapter, "/api/users/:id", deleteUser,
        apix.WithSummary("Delete user"),
        apix.WithTags("Users"),
        apix.WithNotFoundError("User not found"),
    )
    
    // ... rest of setup
}
```

## Serving OpenAPI Spec

### Runtime Serving

Serve the spec dynamically at runtime:

```go
import "github.com/Infra-Forge/infra-apix/openapi"

handler, err := runtime.NewHandler(runtime.Config{
    Title:           "My API",
    Version:         "1.0.0",
    Format:          "json", // or "yaml"
    EnableSwaggerUI: true,
    Servers:         []string{"https://api.example.com", "http://localhost:8080"},
    Validate:        true,
    // Config has no description field; set it on the builder.
    CustomizeBuilder: func(b *openapi.Builder) {
        b.Info.Description = "User management API"
    },
})
if err != nil {
    log.Fatal(err)
}

handler.RegisterEcho(e)
```

### Custom Paths

```go
handler, err := runtime.NewHandler(runtime.Config{
    Title:         "My API",
    Version:       "1.0.0",
    SpecPath:      "/api/openapi.json",  // Custom spec path
    SwaggerUIPath: "/api/docs",          // Custom Swagger UI path
    EnableSwaggerUI: true,
})
```

### Add Security Schemes

Declare every scheme that a route names with `apix.WithSecurity`. The build fails for a scheme a route names but nothing declares.

```go
import "github.com/getkin/kin-openapi/openapi3"

handler, err := runtime.NewHandler(runtime.Config{
    Title:   "My API",
    Version: "1.0.0",
    SecuritySchemes: openapi3.SecuritySchemes{
        "BearerAuth": &openapi3.SecuritySchemeRef{
            Value: openapi3.NewJWTSecurityScheme(),
        },
    },
})
```

## Generating Static Specs

### Using CLI

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
```

### CI Integration

Add to `.github/workflows/ci.yml`:

```yaml
name: CI

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.27'
      
      - name: Run tests
        run: go test ./...
      
      - name: Check OpenAPI spec drift
        run: |
          go install github.com/Infra-Forge/infra-apix/cmd/apix@latest
          apix spec-guard --existing docs/openapi.yaml
```

## Next Steps

### Learn More

- **[API Reference](API_REFERENCE.md)** - Complete API documentation
- **[Framework Guides](FRAMEWORK_GUIDES.md)** - Framework-specific guides
- **[OpenAPI Generation](OPENAPI_GENERATION.md)** - Advanced OpenAPI features
- **[Examples](../examples/)** - Complete working examples

### Add Features

1. **Authentication**
   ```go
   echoadapter.Post(adapter, "/api/users", createUser,
       apix.WithSecurity("BearerAuth"),
   )
   ```

2. **Validation**
   ```go
   type CreateUserRequest struct {
       Name  string `json:"name" validate:"required,min=3,max=50"`
       Email string `json:"email" validate:"required,email"`
       Age   int    `json:"age" validate:"min=18,max=120"`
   }
   ```

3. **Error Handling**
   ```go
   echoadapter.Post(adapter, "/api/users", createUser,
       apix.WithStandardErrors(),
       apix.WithResponse(http.StatusConflict, &ErrorResponse{},
           apix.WithDescription("User already exists")),
   )
   ```

4. **Pagination**
   ```go
   echoadapter.Get(adapter, "/api/users", listUsers,
       apix.WithParameter(apix.Parameter{
           Name:       "page",
           In:         "query",
           SchemaType: "integer",
       }),
       apix.WithParameter(apix.Parameter{
           Name:       "per_page",
           In:         "query",
           SchemaType: "integer",
       }),
   )
   ```

### Explore Examples

Check out the complete examples in the `examples/` directory:

```bash
cd examples/infranotes
go run main.go
```

Visit http://localhost:8080/swagger to see a full-featured API with:
- Multiple resources (Documents, Transactions, Categories)
- Nested response types
- Pagination
- Security schemes
- Complex types (UUID, decimal.Decimal, time.Time)

### Get Help

- **Issues**: https://github.com/Dorico-Dynamics/infra-apix/issues
- **Examples**: https://github.com/Dorico-Dynamics/infra-apix/tree/main/examples

