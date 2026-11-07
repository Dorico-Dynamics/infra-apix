# Migration Guide: Swaggo → Apix

This guide helps you migrate from [swaggo/swag](https://github.com/swaggo/swag) to **infra-apix**, a code-first OpenAPI 3.1 generator for Go.

The examples use Echo. The other adapters have the same functions; see [FRAMEWORK_GUIDES.md](docs/FRAMEWORK_GUIDES.md).

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
| **Spec Generation** | `swag init` CLI | Built from the registered routes, at runtime (`runtime.NewHandler`) or by a program of yours |
| **Determinism** | Non-deterministic | Deterministic (sorted, stable) |
| **Framework Support** | Framework-specific | Framework-agnostic adapters |
| **Extensibility** | Limited | Plugin system |
| **Error Handling** | Manual annotations | Built-in StatusCoder interface |
| **Examples** | Comment-based | Struct tags + programmatic |
| **Validation** | Manual annotations | `required` inferred from struct tags |
| **Maintenance** | High (comments drift) | Low (code is source of truth) |

---

## Feature Comparison

### Feature Support

| Feature | Swaggo | Apix |
|---------|--------|------|
| Specification | Swagger 2.0 | OpenAPI 3.1 |
| Request/Response Models | ✅ | ✅ |
| Path Parameters | ✅ | ✅ (`apix.WithParameter`) |
| Query Parameters | ✅ | ✅ (`apix.WithParameter`) |
| Headers | ✅ | ✅ (`apix.WithParameter`, `apix.WithSuccessHeaders`) |
| Examples | ✅ | ✅ (`example` tag, `apix.WithRequestExample`) |
| Validation Rules | ✅ | `required` only, from `validate` or `binding` tags |
| Security Schemes | ✅ | ✅ (`runtime.Config.SecuritySchemes`, `apix.WithSecurity`) |
| Request Content Types | ✅ | One per route: JSON, `apix.WithMultipartFormData`, `apix.WithFormURLEncoded` |
| File Uploads | ✅ | Documented with `apix.WithMultipartFormData`; the adapters decode JSON, so decode the form yourself ([Example 3](#example-3-file-upload)) |
| Swagger UI | ✅ | ✅ (`runtime.Config.EnableSwaggerUI`) |
| JSON/YAML Output | ✅ | ✅ (`runtime.Config.Format`) |

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

### Step 2: Remove Swaggo

Delete the files `swag init` generated (`docs/docs.go`, `docs/swagger.json`, `docs/swagger.yaml`), the blank import of that package, and the Swagger UI route (`e.GET("/swagger/*", echoSwagger.WrapHandler)`). Then remove the modules:

```bash
go get github.com/swaggo/swag@none
go get github.com/swaggo/echo-swagger@none  # or gin-swagger, etc.

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

Replace each handler with a typed handler, and register it through an adapter. Each annotation becomes part of the handler or a route option:

| Swaggo | Apix |
|--------|------|
| `@Summary`, `@Description`, `@Tags` | `apix.WithSummary`, `apix.WithDescription`, `apix.WithTags` |
| `@ID` | `apix.WithOperationID` |
| `@Accept json` | JSON is the default; `apix.WithMultipartFormData()`, `apix.WithFormURLEncoded()` |
| `@Produce json` | The adapter writes the response as JSON |
| `@Param id path int true "User ID"` | `apix.WithParameter(apix.Parameter{Name: "id", In: "path", SchemaType: "integer", Required: true, Description: "User ID"})` |
| `@Param user body CreateUserRequest true "User data"` | The handler's request type, `*CreateUserRequest` |
| `@Success 200 {object} User` | The handler's response type; the status defaults to 201 for POST, 204 for DELETE and 200 otherwise, and `apix.WithSuccessStatus` changes it |
| `@Failure 404 {object} ErrorResponse` | `apix.WithNotFoundError`, and the other error options |
| `@Security BearerAuth` | `apix.WithSecurity("BearerAuth")` |
| `@Deprecated` | `apix.WithDeprecated()` |
| `@Router /users/{id} [get]` | `echoadapter.Get(adapter, "/users/:id", GetUser)`, with the `@Param` option for `id` |

```go
// AFTER (Apix)
type UserResponse struct {
    ID    int    `json:"id" example:"123"`
    Name  string `json:"name" example:"John Doe"`
    Email string `json:"email" example:"john@example.com"`
}

func GetUser(ctx context.Context, _ *apix.NoBody) (UserResponse, error) {
    id, err := strconv.Atoi(pathParam(ctx, "id"))
    if err != nil {
        return UserResponse{}, apix.BadRequest("id must be an integer")
    }
    // ... load the user
    return UserResponse{ID: id, Name: "John Doe", Email: "john@example.com"}, nil
}

// Register with the Echo adapter
adapter := echoadapter.New(e)
echoadapter.Get(adapter, "/users/:id", GetUser,
    apix.WithSummary("Get user by ID"),
    apix.WithDescription("Get user details by ID"),
    apix.WithTags("users"),
    apix.WithParameter(apix.Parameter{
        Name:        "id",
        In:          "path",
        Description: "User ID",
        SchemaType:  "integer",
        Required:    true,
        Example:     123,
    }),
    apix.WithNotFoundError("User not found"),
)
```

apix passes a handler the request context and the decoded body; it does not bind path or query values. Nor does it read path parameters from the route: without the `WithParameter` for `id`, the document is not valid OpenAPI. `pathParam` reads the `id` that a middleware copies into the request context (see [Reading Path and Query Parameters](docs/FRAMEWORK_GUIDES.md#reading-path-and-query-parameters)).

### Step 5: Serve the OpenAPI Document

```go
docs, err := runtime.NewHandler(runtime.Config{
    Title:           "Users API",
    Version:         "1.0.0",
    EnableSwaggerUI: true,
})
if err != nil {
    log.Fatal(err)
}
docs.RegisterEcho(e) // GET /openapi.json, and the Swagger UI at /swagger
```

The handler builds the document from the registered routes on the first request. To write it to a file instead, build it in a program that registers your routes (see [Programmatic Generation](docs/OPENAPI_GENERATION.md#programmatic-generation)). The `apix` CLI cannot do this yet: it reads the routes registered in its own process, so it finds none (see [CLI_USAGE.md](docs/CLI_USAGE.md)).

### Step 6: Update CI/CD

Replace `swag init` with that program in your CI/CD pipelines:

```yaml
# BEFORE (Swaggo)
- name: Generate Swagger docs
  run: swag init

# AFTER (Apix): ./cmd/openapi is your program that prints the document
- name: Generate OpenAPI spec
  run: go run ./cmd/openapi > docs/openapi.yaml
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

func CreateUser(ctx context.Context, req *CreateUserRequest) (CreateUserResponse, error) {
    // ... implementation
    return CreateUserResponse{ID: 123, Name: req.Name, Email: req.Email}, nil
}

// Register; POST answers 201 Created by default
echoadapter.Post(adapter, "/users", CreateUser,
    apix.WithSummary("Create user"),
    apix.WithDescription("Create a new user"),
    apix.WithTags("users"),
    apix.WithBadRequestError("Invalid request"),
)
```

The adapter decodes the body itself and answers 400 for malformed JSON or an unknown field. When `e.Validator` is set, it also validates the body, and answers 422 when a rule fails.

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
func GetUser(ctx context.Context, _ *apix.NoBody) (UserResponse, error) {
    id, err := strconv.Atoi(pathParam(ctx, "id"))
    if err != nil {
        return UserResponse{}, apix.BadRequest("id must be an integer")
    }
    user, err := userService.Get(ctx, id)
    if err != nil {
        if errors.Is(err, ErrNotFound) {
            return UserResponse{}, apix.NotFound("User not found")
        }
        // Log the cause; the client gets only the message
        slog.ErrorContext(ctx, "load user", "id", id, "error", err)
        return UserResponse{}, apix.InternalServerError("Failed to get user")
    }
    return UserResponse{ID: user.ID, Name: user.Name, Email: user.Email}, nil
}

// Register with standard error responses
echoadapter.Get(adapter, "/users/:id", GetUser,
    apix.WithSummary("Get user"),
    apix.WithParameter(apix.Parameter{Name: "id", In: "path", SchemaType: "integer", Required: true}),
    apix.WithStandardErrors(),                // Adds 400, 422, 500
    apix.WithNotFoundError("User not found"), // Adds 404
)
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
    Size int `json:"size" example:"1024"`
}

func UploadFile(ctx context.Context, req *UploadFileRequest) (UploadFileResponse, error) {
    // ... store req.File
    return UploadFileResponse{Size: len(req.File)}, nil
}

// decodeUpload reads the multipart form; the adapter's default decoder reads JSON.
func decodeUpload(_ context.Context, c echo.Context, dst any) error {
    req, ok := dst.(*UploadFileRequest)
    if !ok {
        return fmt.Errorf("decodeUpload: unexpected request type %T", dst)
    }
    header, err := c.FormFile("file")
    if err != nil {
        return apix.BadRequest("the form has no file field")
    }
    file, err := header.Open()
    if err != nil {
        return fmt.Errorf("open upload: %w", err)
    }
    defer file.Close()
    req.File, err = io.ReadAll(file)
    if err != nil {
        return fmt.Errorf("read upload: %w", err)
    }
    return nil
}

// Upload routes use an adapter with that decoder; JSON routes keep the default one
uploads := echoadapter.New(e, echoadapter.Options{Decoder: decodeUpload})
echoadapter.Post(uploads, "/upload", UploadFile,
    apix.WithSummary("Upload file"),
    apix.WithMultipartFormData(),
    apix.WithSuccessStatus(http.StatusOK),
)
```

`io.ReadAll` holds the file in memory, so cap the request body with Echo's `middleware.BodyLimit`.

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
type ListUsersResponse struct {
    Users []User `json:"users"`
    Total int    `json:"total" example:"100"`
}

func ListUsers(ctx context.Context, _ *apix.NoBody) (ListUsersResponse, error) {
    page, err := intQuery(ctx, "page", 1)
    if err != nil {
        return ListUsersResponse{}, err
    }
    limit, err := intQuery(ctx, "limit", 10)
    if err != nil {
        return ListUsersResponse{}, err
    }
    users, total, err := userService.List(ctx, queryParam(ctx, "search"), page, limit)
    if err != nil {
        return ListUsersResponse{}, err
    }
    return ListUsersResponse{Users: users, Total: total}, nil
}

// intQuery reads a positive integer query parameter, or def when it is absent.
func intQuery(ctx context.Context, name string, def int) (int, error) {
    v := queryParam(ctx, name)
    if v == "" {
        return def, nil
    }
    n, err := strconv.Atoi(v)
    if err != nil || n < 1 {
        return 0, apix.BadRequest(name + " must be a positive integer")
    }
    return n, nil
}

// Register
echoadapter.Get(adapter, "/users", ListUsers,
    apix.WithSummary("List users"),
    apix.WithTags("users"),
    apix.WithParameter(apix.Parameter{Name: "page", In: "query", Description: "Page number", SchemaType: "integer", Example: 1}),
    apix.WithParameter(apix.Parameter{Name: "limit", In: "query", Description: "Page size", SchemaType: "integer", Example: 10}),
    apix.WithParameter(apix.Parameter{Name: "search", In: "query", Description: "Search term", Example: "john"}),
)
```

`queryParam` reads the query that a middleware copies into the request context (see [Reading Path and Query Parameters](docs/FRAMEWORK_GUIDES.md#reading-path-and-query-parameters)). apix does not read `query:` struct tags.

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
func GetUser(ctx context.Context, _ *apix.NoBody) (UserResponse, error) {
    id, err := strconv.Atoi(pathParam(ctx, "id"))
    if err != nil || id <= 0 {
        return UserResponse{}, apix.NewProblemDetails(http.StatusBadRequest, "Invalid user ID", "id must be a positive integer").
            WithType("https://api.example.com/errors/invalid-id").
            WithExtension("field", "id")
    }
    // ...
    return UserResponse{ID: id}, nil
}

// Answer errors as application/problem+json
e := echo.New()
adapter := echoadapter.New(e, echoadapter.Options{UseProblemDetails: true})
e.HTTPErrorHandler = echoadapter.ProblemDetailsErrorHandler(e.HTTPErrorHandler)

echoadapter.Get(adapter, "/users/:id", GetUser,
    apix.WithSummary("Get user"),
    apix.WithParameter(apix.Parameter{Name: "id", In: "path", SchemaType: "integer", Required: true}),
)
```

With `UseProblemDetails`, the adapter also answers `apix.NotFound` and the other `apix.HTTPError` errors as problem details.

---

## Troubleshooting

### Issue 1: A handler without a request body does not compile

**Problem**: `echoadapter.Get` takes a `func(context.Context, *apix.NoBody) (TResp, error)`. A handler that takes only a context fails with:

```text
in call to echoadapter.Get, type func(ctx context.Context) (HealthResponse, error) of HealthCheck does not match apix.HandlerFunc[apix.NoBody, TResp] (cannot infer TResp)
```

**Solution**: Take an `*apix.NoBody`, as every `Get` and `Delete` handler does:

```go
// BEFORE
func HealthCheck(ctx context.Context) (HealthResponse, error) { ... }

// AFTER
func HealthCheck(ctx context.Context, _ *apix.NoBody) (HealthResponse, error) { ... }
```

### Issue 2: Path or query values are empty

**Problem**: apix does not bind path or query values to the request type, and it ignores `path:` and `query:` struct tags.

**Solution**: Declare the parameter with `apix.WithParameter`, and read it from the context (see [Reading Path and Query Parameters](docs/FRAMEWORK_GUIDES.md#reading-path-and-query-parameters)):

```go
// Route: /users/:id
id := pathParam(ctx, "id")
```

### Issue 3: "Examples not showing in spec"

**Problem**: Examples are not appearing in generated OpenAPI spec.

**Solution**: Use `example` struct tags, or `WithRequestExample()` on a route with a request body:

```go
type User struct {
    Name string `json:"name" example:"John Doe"` // Struct tag
}

// Or programmatic
echoadapter.Post(adapter, "/users", CreateUser,
    apix.WithRequestExample(CreateUserRequest{Name: "John Doe", Email: "john@example.com"}),
)
```

### Issue 4: "Validation rules not in spec"

**Problem**: Validation constraints such as `min`, `max` or `email` do not appear in the OpenAPI spec.

**Solution**: apix documents one rule, `required`, from a `validate` or `binding` tag. It also marks a field required when the field has no `omitempty` and is not a pointer, slice, map or interface. Your validator still enforces the other rules; state them in a `description` tag:

```go
type CreateUserRequest struct {
    Name  string `json:"name" validate:"required,min=3,max=50" description:"3 to 50 characters"`
    Email string `json:"email" validate:"required,email" description:"Email address"`
    Age   int    `json:"age,omitempty" validate:"gte=0,lte=120" description:"0 to 120"`
}
```

### Issue 5: "Custom error responses not documented"

**Problem**: Need to document custom error responses.

**Solution**: Use the error options. `WithConflictError` and the other helpers document `apix.ErrorResponse`; `WithCustomErrorResponse` documents the model you pass:

```go
echoadapter.Post(adapter, "/users", CreateUser,
    apix.WithSummary("Create user"),
    apix.WithConflictError("User already exists"),
    apix.WithCustomErrorResponse(http.StatusUnprocessableEntity, ValidationError{}, "Validation failed"),
)
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

**A**: Declare the schemes in `runtime.Config.SecuritySchemes`, and name them on routes with `apix.WithSecurity`:

```go
docs, err := runtime.NewHandler(runtime.Config{
    Title:   "Users API",
    Version: "1.0.0",
    SecuritySchemes: openapi3.SecuritySchemes{
        "BearerAuth": &openapi3.SecuritySchemeRef{Value: openapi3.NewJWTSecurityScheme()},
    },
})
if err != nil {
    log.Fatal(err)
}
docs.RegisterEcho(e)

echoadapter.Get(adapter, "/users/:id", GetUser,
    apix.WithParameter(apix.Parameter{Name: "id", In: "path", SchemaType: "integer", Required: true}),
    apix.WithSecurity("BearerAuth"),
)
```

apix documents the requirement; your middleware still checks the token. The build fails for a route that names a scheme the config does not declare.

### Q: How do I customize the OpenAPI spec?

**A**: Set the title, version, servers and security schemes in `runtime.Config`:

```go
docs, err := runtime.NewHandler(runtime.Config{
    Title:   "Users API",
    Version: "1.0.0",
    Servers: []string{"https://api.example.com", "https://staging.example.com"},
})
if err != nil {
    log.Fatal(err)
}
docs.RegisterEcho(e)
```

Change the rest in `runtime.Config.CustomizeBuilder`, or in a plugin's `OnSpecBuild`, which receives the built document:

```go
type ContactPlugin struct {
    apix.BasePlugin
}

func (p *ContactPlugin) OnSpecBuild(doc *openapi3.T) error {
    doc.Info.Contact = &openapi3.Contact{Name: "API Team", Email: "api@example.com"}
    return nil
}

apix.RegisterPlugin(&ContactPlugin{BasePlugin: apix.BasePlugin{PluginName: "contact"}})
```

### Q: How do I enable Swagger UI?

**A**: Set `EnableSwaggerUI: true` in `runtime.Config`, as in [Step 5](#step-5-serve-the-openapi-document). The page is served at `/swagger`, or at `SwaggerUIPath` when you set it. Leave it off where you do not want the page.

### Q: What about performance?

**A**: Apix keeps the document off the request path:
- The runtime handler builds the document on the first request, and keeps it until a route or plugin is registered or removed (`CacheTTL` also rebuilds it once it is that old)
- Schemas are reflected when the document is built, not on each request
- Handling a request decodes and encodes JSON; it does not touch the document

### Q: How do I test my handlers?

**A**: Test handlers directly without framework:

```go
func TestCreateUser(t *testing.T) {
    req := CreateUserRequest{Name: "John", Email: "john@example.com"}
    resp, err := CreateUser(context.Background(), &req)

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
echoadapter.Get(adapter, "/users/:id", GetUser,
    apix.WithParameter(apix.Parameter{Name: "id", In: "path", SchemaType: "integer", Required: true}),
)
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
echoadapter.Get(adapter, "/users/:id", GetUser,
    apix.WithParameter(apix.Parameter{Name: "id", In: "path", SchemaType: "integer", Required: true}),
    apix.WithStandardErrors(), // Adds 400, 422, 500
)
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

// Register it before the routes: OnRouteRegister runs as each route is registered
apix.RegisterPlugin(&AutoTagPlugin{BasePlugin: apix.BasePlugin{PluginName: "auto-tag"}})
```

### 4. Enable Logging for Debugging

```go
// Set the logger before registering routes to see their events
apix.SetLogger(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
```

### 5. Use CI/CD Spec Guard

```yaml
# .github/workflows/ci.yml; ./cmd/openapi is your program that prints the document
- name: Generate OpenAPI spec
  run: go run ./cmd/openapi > docs/openapi.yaml

- name: Check for spec drift
  run: |
    if ! git diff --exit-code docs/openapi.yaml; then
      echo "OpenAPI spec has drifted from code!"
      exit 1
    fi
```

---

## Next Steps

1. ✅ Install apix: `go get github.com/Infra-Forge/infra-apix`
2. ✅ Remove swaggo dependencies
3. ✅ Convert one endpoint as a proof of concept
4. ✅ Serve the spec with `runtime.NewHandler`
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
