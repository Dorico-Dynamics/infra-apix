# Migration Guide: Swaggo → Apix

This guide helps you migrate from [swaggo/swag](https://github.com/swaggo/swag) to **infra-apix**, a code-first OpenAPI 3.1 generator for Go.

---

## Table of Contents

1. [Why Migrate?](#why-migrate)
2. [Key Differences](#key-differences)
3. [Feature Comparison](#feature-comparison)
4. [Migration Steps](#migration-steps)
5. [Code Examples](#code-examples)
6. [Troubleshooting](#troubleshooting)
7. [FAQ](#faq)

---

## Why Migrate?

### Problems with Swaggo

- **Comment-based annotations are brittle**: Comments can drift from code, causing spec/implementation mismatches
- **Manual YAML editing**: Developers often edit generated specs directly, creating maintenance nightmares
- **AI agent interference**: AI tools "fix" issues by rewriting generated specs instead of fixing code
- **Non-deterministic output**: Specs change between runs, causing noisy git diffs
- **Limited type safety**: No compile-time validation of annotations
- **Verbose syntax**: Requires extensive comments for every endpoint

### Benefits of Apix

- **Code-first**: Derive OpenAPI spec directly from Go types and handlers
- **Type-safe**: Compile-time validation, no string-based annotations
- **Deterministic**: Sorted paths/components, stable operationIds, git-friendly diffs
- **Framework-agnostic**: Works with Echo, Chi, Gorilla/Mux, Gin, Fiber v3
- **Extensible**: Plugin system for custom metadata injection
- **Production-ready**: RFC 9457 Problem Details, structured logging, comprehensive error handling

---

## Key Differences

| Aspect | Swaggo | Apix |
|--------|--------|------|
| **Approach** | Comment-based annotations | Code-first with typed handlers |
| **Type Safety** | Runtime (comments) | Compile-time (Go types) |
| **Spec Generation** | `swag init` CLI | `apix generate` CLI or runtime |
| **Determinism** | Non-deterministic | Deterministic (sorted, stable) |
| **Framework Support** | Framework-specific | Framework-agnostic adapters |
| **Extensibility** | Limited | Plugin system |
| **Error Handling** | Manual annotations | Built-in StatusCoder interface |
| **Examples** | Comment-based | Struct tags + programmatic |
| **Validation** | Manual annotations | Inferred from struct tags |
| **Maintenance** | High (comments drift) | Low (code is source of truth) |

---

## Feature Comparison

### ✅ Feature Parity

| Feature | Swaggo | Apix |
|---------|--------|------|
| OpenAPI 3.x | ✅ 3.0 | ✅ 3.1 |
| Request/Response Models | ✅ | ✅ |
| Path Parameters | ✅ | ✅ |
| Query Parameters | ✅ | ✅ |
| Headers | ✅ | ✅ |
| Examples | ✅ | ✅ |
| Validation Rules | ✅ | ✅ (from struct tags) |
| Security Schemes | ✅ | ✅ (via plugins) |
| Multiple Content Types | ✅ | ✅ |
| File Uploads | ✅ | ✅ |
| Swagger UI | ✅ | ✅ |
| JSON/YAML Output | ✅ | ✅ |

### 🚀 Apix Advantages

- **Plugin System**: Custom metadata injection without modifying core library
- **RFC 9457 Problem Details**: Standardized error responses
- **Structured Logging**: Built-in observability with `slog`
- **Deterministic Output**: Git-friendly, no spurious diffs
- **Type Inference**: Automatic schema generation from Go types
- **Framework Adapters**: Consistent API across Echo, Chi, Mux, Gin, Fiber

---

## Migration Steps

### Step 1: Install Apix

```bash
go get github.com/Infra-Forge/infra-apix
```

### Step 2: Remove Swaggo Dependencies

```bash
# Remove swaggo
go get -u github.com/swaggo/swag@none
go get -u github.com/swaggo/echo-swagger@none  # or gin-swagger, etc.

# Clean up go.mod
go mod tidy
```

### Step 3: Remove Swaggo Annotations

Delete all swaggo comment annotations from your handlers:

```go
// BEFORE (Swaggo)
// @Summary Get user by ID
// @Description Get user details by ID
// @Tags users
// @Accept json
// @Produce json
// @Param id path int true "User ID"
// @Success 200 {object} User
// @Failure 404 {object} ErrorResponse
// @Router /users/{id} [get]
func GetUser(c echo.Context) error {
    // ...
}
```

### Step 4: Convert to Apix Typed Handlers

Replace comment-based handlers with typed handlers using framework adapters:

```go
// AFTER (Apix)
type GetUserRequest struct {
    ID int `path:"id" description:"User ID" example:"123"`
}

type GetUserResponse struct {
    ID    int    `json:"id" example:"123"`
    Name  string `json:"name" example:"John Doe"`
    Email string `json:"email" example:"john@example.com"`
}

func GetUser(ctx context.Context, req GetUserRequest) (GetUserResponse, error) {
    // ... implementation
    return GetUserResponse{ID: req.ID, Name: "John Doe"}, nil
}

// Register with Echo adapter
e.GET("/users/:id", echoadapter.Wrap(GetUser,
    apix.WithSummary("Get user by ID"),
    apix.WithDescription("Get user details by ID"),
    apix.WithTags("users"),
))
```

### Step 5: Generate OpenAPI Spec

```bash
# Generate spec (replaces swag init)
apix generate -o openapi.yaml

# Or use runtime endpoint
# GET /openapi.json
```

### Step 6: Update CI/CD

Replace `swag init` with `apix generate` in your CI/CD pipelines:

```yaml
# BEFORE (Swaggo)
- name: Generate Swagger docs
  run: swag init

# AFTER (Apix)
- name: Generate OpenAPI spec
  run: apix generate -o openapi.yaml
```

---

## Code Examples

### Example 1: Simple CRUD Endpoint

#### Swaggo (Before)

```go
// @Summary Create user
// @Description Create a new user
// @Tags users
// @Accept json
// @Produce json
// @Param user body CreateUserRequest true "User data"
// @Success 201 {object} User
// @Failure 400 {object} ErrorResponse
// @Router /users [post]
func CreateUser(c echo.Context) error {
    var req CreateUserRequest
    if err := c.Bind(&req); err != nil {
        return c.JSON(400, ErrorResponse{Message: "Invalid request"})
    }
    // ... implementation
    return c.JSON(201, user)
}
```

#### Apix (After)

```go
type CreateUserRequest struct {
    Name  string `json:"name" validate:"required" example:"John Doe"`
    Email string `json:"email" validate:"required,email" example:"john@example.com"`
}

type CreateUserResponse struct {
    ID    int    `json:"id" example:"123"`
    Name  string `json:"name" example:"John Doe"`
    Email string `json:"email" example:"john@example.com"`
}

func CreateUser(ctx context.Context, req CreateUserRequest) (CreateUserResponse, error) {
    // ... implementation
    return CreateUserResponse{ID: 123, Name: req.Name, Email: req.Email}, nil
}

// Register
e.POST("/users", echoadapter.Wrap(CreateUser,
    apix.WithSummary("Create user"),
    apix.WithDescription("Create a new user"),
    apix.WithTags("users"),
    apix.WithSuccessStatus(http.StatusCreated),
))
```

### Example 2: Error Handling

#### Swaggo (Before)

```go
// @Summary Get user
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
func GetUser(c echo.Context) error {
    user, err := userService.Get(id)
    if err != nil {
        if errors.Is(err, ErrNotFound) {
            return c.JSON(404, ErrorResponse{Message: "User not found"})
        }
        return c.JSON(500, ErrorResponse{Message: "Internal error"})
    }
    return c.JSON(200, user)
}
```

#### Apix (After)

```go
func GetUser(ctx context.Context, req GetUserRequest) (GetUserResponse, error) {
    user, err := userService.Get(req.ID)
    if err != nil {
        if errors.Is(err, ErrNotFound) {
            return GetUserResponse{}, apix.NotFound("User not found")
        }
        return GetUserResponse{}, apix.InternalServerError("Failed to get user")
    }
    return GetUserResponse{ID: user.ID, Name: user.Name}, nil
}

// Register with standard error responses
e.GET("/users/:id", echoadapter.Wrap(GetUser,
    apix.WithSummary("Get user"),
    apix.WithStandardErrors(), // Adds 400, 401, 403, 404, 500
))
```

### Example 3: File Upload

#### Swaggo (Before)

```go
// @Summary Upload file
// @Accept multipart/form-data
// @Param file formData file true "File to upload"
// @Success 200 {object} UploadResponse
func UploadFile(c echo.Context) error {
    file, err := c.FormFile("file")
    if err != nil {
        return c.JSON(400, ErrorResponse{Message: "Invalid file"})
    }
    // ... process file
    return c.JSON(200, UploadResponse{URL: url})
}
```

#### Apix (After)

```go
type UploadFileRequest struct {
    File []byte `json:"file" format:"binary" description:"File to upload"`
}

type UploadFileResponse struct {
    URL string `json:"url" example:"https://example.com/files/123.pdf"`
}

func UploadFile(ctx context.Context, req UploadFileRequest) (UploadFileResponse, error) {
    // ... process file
    return UploadFileResponse{URL: "https://example.com/files/123.pdf"}, nil
}

// Register with multipart/form-data
e.POST("/upload", echoadapter.Wrap(UploadFile,
    apix.WithSummary("Upload file"),
    apix.WithMultipartFormData(),
))
```

### Example 4: Query Parameters

#### Swaggo (Before)

```go
// @Summary List users
// @Param page query int false "Page number"
// @Param limit query int false "Page size"
// @Param search query string false "Search term"
func ListUsers(c echo.Context) error {
    page := c.QueryParam("page")
    limit := c.QueryParam("limit")
    search := c.QueryParam("search")
    // ... implementation
}
```

#### Apix (After)

```go
type ListUsersRequest struct {
    Page   int    `query:"page" description:"Page number" example:"1"`
    Limit  int    `query:"limit" description:"Page size" example:"10"`
    Search string `query:"search" description:"Search term" example:"john"`
}

type ListUsersResponse struct {
    Users []User `json:"users"`
    Total int    `json:"total" example:"100"`
}

func ListUsers(ctx context.Context, req ListUsersRequest) (ListUsersResponse, error) {
    // ... implementation
    return ListUsersResponse{Users: users, Total: 100}, nil
}

// Register
e.GET("/users", echoadapter.Wrap(ListUsers,
    apix.WithSummary("List users"),
    apix.WithTags("users"),
))
```

### Example 5: RFC 9457 Problem Details

#### Swaggo (Before)

```go
// Manual error response structure
type ErrorResponse struct {
    Message string `json:"message"`
    Code    string `json:"code"`
}
```

#### Apix (After)

```go
// Use RFC 9457 Problem Details
func GetUser(ctx context.Context, req GetUserRequest) (GetUserResponse, error) {
    if req.ID <= 0 {
        return GetUserResponse{}, apix.BadRequest("Invalid user ID").
            WithType("https://api.example.com/errors/invalid-id").
            WithExtension("field", "id")
    }
    // ...
}

// Enable Problem Details encoding
e.GET("/users/:id", echoadapter.Wrap(GetUser,
    apix.WithSummary("Get user"),
    echoadapter.UseProblemDetails(), // Enables application/problem+json
))
```

---

## Troubleshooting

### Issue 1: "Cannot infer request type"

**Problem**: Handler doesn't have a request parameter.

**Solution**: Add a request struct, even if empty:

```go
// BEFORE
func HealthCheck(ctx context.Context) (HealthResponse, error) { ... }

// AFTER
type HealthCheckRequest struct{} // Empty request

func HealthCheck(ctx context.Context, req HealthCheckRequest) (HealthResponse, error) { ... }
```

### Issue 2: "Path parameter not found in struct"

**Problem**: Path parameter in route doesn't match struct field.

**Solution**: Use `path` struct tag:

```go
// Route: /users/:id
type GetUserRequest struct {
    ID int `path:"id"` // Must match :id in route
}
```

### Issue 3: "Examples not showing in spec"

**Problem**: Examples are not appearing in generated OpenAPI spec.

**Solution**: Use `example` struct tags or `WithRequestExample()`:

```go
type User struct {
    Name string `json:"name" example:"John Doe"` // Struct tag
}

// Or programmatic
e.GET("/users/:id", echoadapter.Wrap(GetUser,
    apix.WithRequestExample(GetUserRequest{ID: 123}),
))
```

### Issue 4: "Validation rules not in spec"

**Problem**: Validation constraints not appearing in OpenAPI spec.

**Solution**: Use standard validation tags:

```go
type CreateUserRequest struct {
    Name  string `json:"name" validate:"required,min=3,max=50"`
    Email string `json:"email" validate:"required,email"`
    Age   int    `json:"age" validate:"gte=0,lte=120"`
}
```

### Issue 5: "Custom error responses not documented"

**Problem**: Need to document custom error responses.

**Solution**: Use `WithErrorResponse()`:

```go
e.POST("/users", echoadapter.Wrap(CreateUser,
    apix.WithSummary("Create user"),
    apix.WithErrorResponse(http.StatusConflict, "User already exists", apix.ErrorResponse{}),
    apix.WithErrorResponse(http.StatusUnprocessableEntity, "Validation failed", ValidationError{}),
))
```

---

## FAQ

### Q: Can I migrate incrementally?

**A**: Yes! Apix supports incremental migration. You can:
1. Keep existing swaggo handlers running
2. Migrate new endpoints to apix
3. Gradually convert old endpoints
4. Both specs can coexist during transition

### Q: How do I handle authentication/security?

**A**: Use plugins to add security schemes:

```go
// Create security plugin
type SecurityPlugin struct {
    apix.BasePlugin
}

func (p *SecurityPlugin) Name() string { return "security" }

func (p *SecurityPlugin) OnSpecBuild(doc *openapi3.T) error {
    doc.Components.SecuritySchemes = openapi3.SecuritySchemes{
        "bearerAuth": &openapi3.SecuritySchemeRef{
            Value: openapi3.NewSecurityScheme().
                WithType("http").
                WithScheme("bearer").
                WithBearerFormat("JWT"),
        },
    }
    doc.Security = openapi3.SecurityRequirements{
        {"bearerAuth": []string{}},
    }
    return nil
}

// Register plugin
apix.RegisterPlugin(&SecurityPlugin{})
```

### Q: How do I customize the OpenAPI spec?

**A**: Use the plugin system:

```go
type CustomPlugin struct {
    apix.BasePlugin
}

func (p *CustomPlugin) OnSpecBuild(doc *openapi3.T) error {
    // Add custom servers
    doc.Servers = openapi3.Servers{
        {URL: "https://api.example.com"},
        {URL: "https://staging.example.com"},
    }
    return nil
}

apix.RegisterPlugin(&CustomPlugin{})
```

### Q: How do I enable Swagger UI?

**A**: Use the runtime package:

```go
import "github.com/Infra-Forge/infra-apix/runtime"

// Register Swagger UI (optional, behind feature flag)
runtime.RegisterSwaggerUI(e, "/swagger")

// Always available: /openapi.json
runtime.RegisterOpenAPIHandler(e, "/openapi.json")
```

### Q: What about performance?

**A**: Apix is designed for performance:
- Spec generation is cached at runtime
- Reflection is done once during registration
- No runtime overhead for request handling
- Use `apix generate` for zero runtime cost

### Q: How do I test my handlers?

**A**: Test handlers directly without framework:

```go
func TestCreateUser(t *testing.T) {
    req := CreateUserRequest{Name: "John", Email: "john@example.com"}
    resp, err := CreateUser(context.Background(), req)

    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    if resp.Name != "John" {
        t.Errorf("expected name John, got %s", resp.Name)
    }
}
```

### Q: Can I use apix with existing middleware?

**A**: Yes! Apix adapters work with standard framework middleware:

```go
// Echo middleware
e.Use(middleware.Logger())
e.Use(middleware.Recover())

// Apix handlers work normally
e.GET("/users", echoadapter.Wrap(GetUser))
```

---

## Best Practices

### 1. Use Descriptive Struct Tags

```go
type User struct {
    ID    int    `json:"id" description:"Unique user identifier" example:"123"`
    Name  string `json:"name" description:"Full name" example:"John Doe" validate:"required,min=3"`
    Email string `json:"email" description:"Email address" example:"john@example.com" validate:"required,email"`
}
```

### 2. Leverage Standard Error Responses

```go
// Instead of manually adding each error response
e.GET("/users/:id", echoadapter.Wrap(GetUser,
    apix.WithStandardErrors(), // Adds 400, 401, 403, 404, 500
))
```

### 3. Use Plugins for Cross-Cutting Concerns

```go
// Auto-tag routes by path prefix
type AutoTagPlugin struct {
    apix.BasePlugin
}

func (p *AutoTagPlugin) OnRouteRegister(ref *apix.RouteRef) error {
    if strings.HasPrefix(ref.Path, "/api/users") {
        ref.Tags = append(ref.Tags, "users")
    }
    return nil
}
```

### 4. Enable Logging for Debugging

```go
import "github.com/Infra-Forge/infra-apix/internal/logging"

// Enable structured logging
logging.SetLogger(logging.NewJSONLogger(slog.LevelDebug))
```

### 5. Use CI/CD Spec Guard

```yaml
# .github/workflows/ci.yml
- name: Generate OpenAPI spec
  run: apix generate -o openapi.yaml

- name: Check for spec drift
  run: |
    if ! git diff --exit-code openapi.yaml; then
      echo "OpenAPI spec has drifted from code!"
      exit 1
    fi
```

---

## Next Steps

1. ✅ Install apix: `go get github.com/Infra-Forge/infra-apix`
2. ✅ Remove swaggo dependencies
3. ✅ Convert one endpoint as a proof of concept
4. ✅ Generate spec: `apix generate -o openapi.yaml`
5. ✅ Verify spec in Swagger UI
6. ✅ Migrate remaining endpoints incrementally
7. ✅ Update CI/CD pipelines
8. ✅ Add spec guard to prevent drift

---

## Support

- **GitHub Issues**: [https://github.com/Dorico-Dynamics/infra-apix/issues](https://github.com/Dorico-Dynamics/infra-apix/issues)
- **Documentation**: [README.md](README.md)
- **Examples**: [examples/](examples/)

---

**Happy migrating! 🚀**
