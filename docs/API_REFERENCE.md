# API Reference

Complete API documentation for the `apix` library.

## Table of Contents

- [Core Types](#core-types)
- [Handler Signature](#handler-signature)
- [Route Options](#route-options)
- [Framework Adapters](#framework-adapters)
- [OpenAPI Builder](#openapi-builder)
- [Runtime Server](#runtime-server)
- [CLI Commands](#cli-commands)
- [Logging](#logging)

## Core Types

### HandlerFunc

The canonical handler signature used across all framework adapters.

```go
type HandlerFunc[TReq any, TResp any] func(ctx context.Context, req *TReq) (TResp, error)
```

**Type Parameters:**
- `TReq`: Request body type (use `apix.NoBody` for requests without a body)
- `TResp`: Response body type (use `apix.NoBody` for responses without a body)

**Parameters:**
- `ctx`: Request context for cancellation, deadlines, and values
- `req`: Pointer to the request body (nil for `NoBody`)

**Returns:**
- Response value of type `TResp`
- Error if the operation fails

**Examples:**

```go
// POST with request and response body
func createUser(ctx context.Context, req *CreateUserRequest) (UserResponse, error) {
    // Implementation
}

// GET without request body
func listUsers(ctx context.Context, _ *apix.NoBody) ([]UserResponse, error) {
    // Implementation
}

// DELETE with no response body
func deleteUser(ctx context.Context, _ *apix.NoBody) (apix.NoBody, error) {
    // Implementation
}
```

### NoBody

Sentinel type for handlers that don't accept a request body or don't return a response body.

```go
type NoBody struct{}
```

### RouteMethod

HTTP method constants.

```go
type RouteMethod string

const (
    MethodGet     RouteMethod = "GET"
    MethodPost    RouteMethod = "POST"
    MethodPut     RouteMethod = "PUT"
    MethodPatch   RouteMethod = "PATCH"
    MethodDelete  RouteMethod = "DELETE"
    MethodHead    RouteMethod = "HEAD"
    MethodOptions RouteMethod = "OPTIONS"
)
```

### RouteRef

Internal metadata structure capturing route information for OpenAPI generation.

```go
type RouteRef struct {
    Method      RouteMethod
    Path        string
    OperationID string
    Summary     string
    Description string
    Tags        []string
    Deprecated  bool

    // Request models
    RequestType          reflect.Type
    RequestContentType   string
    ExplicitRequestModel reflect.Type
    RequestExample       any

    // Responses keyed by HTTP status code
    Responses map[int]*ResponseRef

    // Security requirements
    Security []SecurityRequirement

    // Custom headers
    SuccessHeaders map[int][]HeaderRef
    SuccessStatus  int

    // Request body requirements
    BodyRequired bool

    // Parameter metadata
    Parameters []Parameter
}
```

### Parameter

Represents a query, path, or header parameter.

```go
type Parameter struct {
    Name        string
    In          string // "query", "path", "header"
    Description string
    SchemaType  string // "string", "integer", "boolean", etc.
    Required    bool
    Example     any
}
```

### HeaderRef

Represents a response header.

```go
type HeaderRef struct {
    Name        string
    Description string
    SchemaType  string
    Required    bool
    Example     any
}
```

### SecurityRequirement

Represents a security requirement for a route.

```go
type SecurityRequirement struct {
    Name   string   // Security scheme name (e.g., "BearerAuth")
    Scopes []string // Required scopes (e.g., ["read:users", "write:users"])
}
```

## Route Options

Functional options for customizing route metadata.

### WithSummary

Sets the operation summary (short description).

```go
func WithSummary(summary string) RouteOption
```

**Example:**
```go
apix.WithSummary("Create a new user")
```

### WithDescription

Sets the detailed operation description.

```go
func WithDescription(description string) RouteOption
```

**Example:**
```go
apix.WithDescription("Creates a new user account with the provided information. Email must be unique.")
```

### WithTags

Adds tags for grouping operations in documentation.

```go
func WithTags(tags ...string) RouteOption
```

**Example:**
```go
apix.WithTags("Users", "Authentication")
```

### WithOperationID

Sets a custom operation ID (defaults to auto-generated from method and path).

```go
func WithOperationID(id string) RouteOption
```

**Example:**
```go
apix.WithOperationID("createUser")
```

### WithDeprecated

Marks the operation as deprecated.

```go
func WithDeprecated() RouteOption
```

**Example:**
```go
apix.WithDeprecated()
```

### WithSecurity

Adds security requirements to the operation.

```go
func WithSecurity(name string, scopes ...string) RouteOption
```

**Parameters:**
- `name`: Security scheme name, declared in `runtime.Config.SecuritySchemes` or `Builder.SecuritySchemes`; `Build` fails for a scheme nothing declares
- `scopes`: Optional scopes required for this operation

**Example:**
```go
apix.WithSecurity("BearerAuth")
apix.WithSecurity("OAuth2", "read:users", "write:users")
```

### WithParameter

Adds a query, path, or header parameter.

```go
func WithParameter(param Parameter) RouteOption
```

**Example:**
```go
apix.WithParameter(apix.Parameter{
    Name:        "page",
    In:          "query",
    Description: "Page number for pagination",
    SchemaType:  "integer",
    Required:    false,
    Example:     1,
})
```

### WithSuccessStatus

Overrides the default success status code.

```go
func WithSuccessStatus(status int) RouteOption
```

**Default Status Codes:**
- POST: 201 Created
- GET: 200 OK
- PUT: 200 OK
- PATCH: 200 OK
- DELETE: 204 No Content

**Example:**
```go
apix.WithSuccessStatus(http.StatusAccepted) // 202
```

### WithSuccessHeaders

Documents headers on the response for `status`, such as the `Location` a
create handler sets. apix documents no response header by default. `Build`
fails if `status` is not one of the route's responses.

```go
func WithSuccessHeaders(status int, headers ...HeaderRef) RouteOption
```

**Example:**
```go
apix.WithSuccessHeaders(http.StatusCreated, apix.HeaderRef{
    Name:        "Location",
    Description: "URI of the created resource",
    SchemaType:  "string",
    Required:    true,
})
```

### WithStandardErrors

Adds standard 4xx/5xx error responses using the shared `ErrorResponse` schema.

```go
func WithStandardErrors() RouteOption
```

**Adds:**
- 400 Bad Request
- 500 Internal Server Error

**Example:**
```go
apix.WithStandardErrors()
```

### WithNotFoundError

Adds a 404 Not Found error response.

```go
func WithNotFoundError(description string) RouteOption
```

**Example:**
```go
apix.WithNotFoundError("User not found")
```

### WithExplicitRequestModel

Overrides the inferred request model type.

```go
func WithExplicitRequestModel(model any, contentType string) RouteOption
```

**Example:**
```go
apix.WithExplicitRequestModel(&CustomRequest{}, "application/json")
```

### WithRequestOverride

Overrides request model with example.

```go
func WithRequestOverride(model any, contentType string, example any) RouteOption
```

**Example:**
```go
apix.WithRequestOverride(&CreateUserRequest{}, "application/json", map[string]any{
    "name":  "John Doe",
    "email": "john@example.com",
})
```

## Framework Adapters

All framework adapters follow the same pattern with adapter-specific implementations.

### Common Pattern

```go
// 1. Create framework instance
framework := /* framework-specific initialization */

// 2. Create apix adapter
adapter := frameworkadapter.New(framework)

// 3. Register routes
frameworkadapter.Post(adapter, "/path", handler, options...)
frameworkadapter.Get(adapter, "/path", handler, options...)
// ... etc
```

### Echo Adapter

**Package:** `github.com/Infra-Forge/apix/echo`

```go
import (
    echoadapter "github.com/Infra-Forge/apix/echo"
    "github.com/labstack/echo/v4"
)

e := echo.New()
adapter := echoadapter.New(e)

echoadapter.Post(adapter, "/api/users", createUser,
    apix.WithSummary("Create user"),
    apix.WithTags("Users"),
)
```

**Methods:**
- `New(e *echo.Echo, opts ...Options) *EchoAdapter`
- `Register[TReq, TResp](adapter, method, path, handler, opts...)`
- `Get[TResp](adapter, path, handler, opts...)`
- `Post[TReq, TResp](adapter, path, handler, opts...)`
- `Put[TReq, TResp](adapter, path, handler, opts...)`
- `Patch[TReq, TResp](adapter, path, handler, opts...)`
- `Delete[TResp](adapter, path, handler, opts...)`

### Chi Adapter

**Package:** `github.com/Infra-Forge/apix/chi`

```go
import (
    chiadapter "github.com/Infra-Forge/apix/chi"
    "github.com/go-chi/chi/v5"
)

r := chi.NewRouter()
adapter := chiadapter.New(r)

chiadapter.Post(adapter, "/api/users", createUser,
    apix.WithSummary("Create user"),
)
```

**Path Parameters:** Use `{id}` syntax (Chi standard)

### Gorilla/Mux Adapter

**Package:** `github.com/Infra-Forge/apix/mux`

```go
import (
    muxadapter "github.com/Infra-Forge/apix/mux"
    "github.com/gorilla/mux"
)

r := mux.NewRouter()
adapter := muxadapter.New(r)

muxadapter.Post(adapter, "/api/users", createUser,
    apix.WithSummary("Create user"),
)
```

**Path Parameters:** Use `{id}` syntax (Mux standard)

### Gin Adapter

**Package:** `github.com/Infra-Forge/apix/gin`

```go
import (
    ginadapter "github.com/Infra-Forge/apix/gin"
    "github.com/gin-gonic/gin"
)

r := gin.New()
adapter := ginadapter.New(r)

ginadapter.Post(adapter, "/api/users", createUser,
    apix.WithSummary("Create user"),
)
```

**Path Parameters:** Use `:id` syntax (Gin standard)

### Fiber Adapter

**Package:** `github.com/Infra-Forge/apix/fiber`

```go
import (
    fiberadapter "github.com/Infra-Forge/apix/fiber"
    "github.com/gofiber/fiber/v3"
)

app := fiber.New()
adapter := fiberadapter.New(app)

fiberadapter.Post(adapter, "/api/users", createUser,
    apix.WithSummary("Create user"),
)
```

**Path Parameters:** Use `:id` syntax (Fiber standard)

## OpenAPI Builder

**Package:** `github.com/Infra-Forge/apix/openapi`

### Builder

```go
type Builder struct {
    Info            openapi3.Info
    Servers         openapi3.Servers
    SecuritySchemes openapi3.SecuritySchemes
    GlobalSecurity  openapi3.SecurityRequirements
    Tags            openapi3.Tags
}
```

**Methods:**

```go
func NewBuilder() *Builder
func (b *Builder) Build(routes []*apix.RouteRef) (*openapi3.T, error)
```

`Build` fails when two Go types map to one component name, and when a
security requirement names a scheme that neither `SecuritySchemes` nor an
`OnSpecBuild` plugin declares. Each error names the scheme and its first user.

**Example:**

```go
import (
    "github.com/Infra-Forge/apix"
    "github.com/Infra-Forge/apix/openapi"
    "github.com/getkin/kin-openapi/openapi3"
)

builder := openapi.NewBuilder()
builder.Info.Title = "My API"
builder.Info.Version = "1.0.0"
builder.Info.Description = "API description"

builder.Servers = openapi3.Servers{
    &openapi3.Server{URL: "https://api.example.com"},
}

builder.SecuritySchemes = openapi3.SecuritySchemes{
    "BearerAuth": &openapi3.SecuritySchemeRef{
        Value: openapi3.NewJWTSecurityScheme(),
    },
}

routes := apix.Snapshot()
doc, err := builder.Build(routes)
```

### EncodeDocument

Encodes an OpenAPI document to YAML or JSON.

```go
func EncodeDocument(doc *openapi3.T, format string) ([]byte, string, error)
```

**Parameters:**
- `doc`: OpenAPI document
- `format`: "yaml" or "json"

**Returns:**
- Encoded bytes
- Content-Type header value
- Error if encoding fails

## Runtime Server

**Package:** `github.com/Infra-Forge/apix/runtime`

### Config

```go
type Config struct {
    // Output format: "json" or "yaml"
    Format string

    // Document metadata
    Title   string
    Version string
    Servers []string

    // components.securitySchemes; every scheme a route names must be declared
    SecuritySchemes openapi3.SecuritySchemes

    // Validation
    Validate bool // Validate the spec on every build before serving (default: false)

    // Caching: the document, or the error that stopped its build, is kept
    // until a route or plugin is registered or removed
    CacheTTL time.Duration // Also rebuild it once it is this old (0: never)

    // Paths
    SpecPath      string // Default: "/openapi.json", or "/openapi.yaml" for YAML
    SwaggerUIPath string // Default: "/swagger"

    // Swagger UI: the embedded page and its files (see Swagger UI below)
    EnableSwaggerUI bool

    // Advanced customization
    CustomizeBuilder func(*openapi.Builder)
}
```

### Handler

```go
type Handler struct {
    // private fields
}
```

**Methods:**

```go
func NewHandler(cfg Config) (*Handler, error)
func (h *Handler) RegisterEcho(e *echo.Echo)
func (h *Handler) RegisterHTTP(mux *http.ServeMux)
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request)
```

**Example:**

```go
handler, err := runtime.NewHandler(runtime.Config{
    Title:           "My API",
    Version:         "1.0.0",
    Format:          "json",
    EnableSwaggerUI: true,
    Servers:         []string{"https://api.example.com"},
    SecuritySchemes: openapi3.SecuritySchemes{
        "BearerAuth": &openapi3.SecuritySchemeRef{
            Value: openapi3.NewJWTSecurityScheme(),
        },
    },
})
if err != nil {
    log.Fatal(err)
}

// For Echo
handler.RegisterEcho(e)

// For other frameworks
mux := http.NewServeMux()
handler.RegisterHTTP(mux)
```

### Swagger UI

With `EnableSwaggerUI` set, `RegisterHTTP` and `RegisterEcho` serve a Swagger UI page at `SwaggerUIPath` and the files it loads below that path:

| Path (default `SwaggerUIPath`) | Content |
|--------------------------------|---------|
| `/swagger` | The page |
| `/swagger/swagger-ui.css`, `/swagger/swagger-ui-bundle.js` | swagger-ui-dist 5.34.1, embedded in the binary and unmodified. Its Apache-2.0 `LICENSE` and `NOTICE` are in `internal/swaggerui/swagger-ui-dist/` |
| `/swagger/swagger-initializer.js` | Starts Swagger UI with the document at `SpecPath` |

The page loads nothing from another origin and runs no inline script, so it is served with this Content-Security-Policy:

```text
default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data: blob:;
connect-src 'self' <origins of Servers>; media-src 'self' <origins of Servers>;
frame-ancestors 'none'; base-uri 'none'; form-action 'none'
```

- **Requests.** The page fetches the document, and Try it out sends requests to the server selected in the page. `connect-src` admits the page's origin and the origin of each absolute `http` or `https` URL in `Servers`. A relative server is the page's origin. A server that `CustomizeBuilder` or a plugin adds is not admitted, so add it to `Servers`. A URL with server variables or an IPv6 address cannot be named in a policy, so requests to it are blocked.
- **Images and audio.** The stylesheet's icons are `data:` URLs, and image responses render from `blob:` URLs. Images in descriptions load only from the page's origin. Audio responses play from the request's URL, so `media-src` admits the same origins as `connect-src`.
- **Caching.** The page is sent with `Cache-Control: no-store`. The files are sent with an `ETag` and `Cache-Control: no-cache`, so a browser revalidates them and receives `304 Not Modified`. Every response carries `X-Content-Type-Options: nosniff`.
- **Other routers.** Mount the `ServeMux` that `RegisterHTTP` filled for the page's path and the paths below it: `r.Mount("/", mux)` with chi, `r.PathPrefix("/swagger").Handler(mux)` with gorilla/mux, and both `r.Any("/swagger", h)` and `r.Any("/swagger/*file", h)` with Gin, where `h` is `gin.WrapH(mux)`.
- **Write failures.** A page or file the client did not receive in full is logged like the document (see [Logging](#logging)).

`runtime.RenderSwaggerUI(specPath)` returns a standalone page that loads the same release from unpkg.com, checked with Subresource Integrity. It starts Swagger UI from an inline script, so a Content-Security-Policy must allow that script.

### Serving the Document and the UI

The document describes every route registered with apix, internal ones included, with their parameters, schemas and security requirements. The UI makes those routes callable from a browser. Choose one mode per environment:

| Mode | When | How |
|------|------|-----|
| Off | Production, unless the API is public | Leave `EnableSwaggerUI` unset and do not register the handler on the public router. Publish a document generated at build time instead (`apix generate`). |
| Internal | Operators and developers need the live document | Register the handler on a server only they can reach: a loopback listener reached through a port-forward or tunnel, an internal network, or an authenticating proxy. |
| Public | The API and every route in its document are public | Register the handler on the public router, after reviewing the document. |

```go
// Internal: the document and the UI on their own loopback listener.
docs := http.NewServeMux()
handler.RegisterHTTP(docs)
docsServer := &http.Server{Addr: "127.0.0.1:9090", Handler: docs, ReadHeaderTimeout: 5 * time.Second}
go func() {
    if err := docsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
        log.Printf("docs server: %v", err)
    }
}()
```

## CLI Commands

### apix generate

Generate OpenAPI spec from registered routes.

```bash
apix generate [flags]
```

**Flags:**
- `--project string`: Path to Go project (default ".")
- `--out string`: Output path (default "docs/openapi.yaml")
- `--format string`: Output format: yaml or json (default "yaml")
- `--title string`: API title (default "API")
- `--version string`: API version (default "1.0.0")
- `--servers string`: Comma-separated server URLs
- `--stdout`: Write to stdout instead of file
- `--validate`: Validate generated spec (default true)

**Example:**

```bash
apix generate \
  --title "My API" \
  --version "1.0.0" \
  --servers "https://api.example.com,https://staging.example.com" \
  --out docs/openapi.yaml
```

### apix spec-guard

Check for drift between generated spec and committed spec (for CI).

```bash
apix spec-guard [flags]
```

**Flags:**
- `--existing string`: Path to existing spec (defaults to --out)
- `--out string`: Expected spec path (default "docs/openapi.yaml")

**Exit Codes:**
- 0: No drift detected
- 1: Drift detected or error

**Example:**

```bash
# In CI pipeline
apix spec-guard --existing docs/openapi.yaml
```

## Registry Functions

### ResetRegistry

Clears all registered routes (primarily for tests and CLI runs).

```go
func ResetRegistry()
```

### RegisterRoute

Registers a new route metadata entry (called by adapters).

```go
func RegisterRoute(ref *RouteRef)
```

### Snapshot

Returns a copy of registered routes sorted by path+method for deterministic output.

```go
func Snapshot() []*RouteRef
```

## Logging

### SetLogger

Sends apix's logs to `logger`. `nil` restores the defaults.

```go
func SetLogger(logger *slog.Logger)
```

- **Errors answered with a generic 500.** These are errors without an HTTP status, or with a status outside 4xx and 5xx. They are logged at Error level, with the request's context, under the message `error answered with a generic 500`. The attributes are `reason`, `status` (for an invalid status), `method`, `path`, `request_id` (from `X-Request-ID`), `correlation_id` (from `X-Correlation-ID`) and `error`. The two ids are left out when the header is not set. Without a logger, these lines go to `slog.Default()`. The Echo adapter leaves an error without an HTTP status to Echo's `HTTPErrorHandler`, so it logs only an error whose status is outside 4xx and 5xx. Echo's default `HTTPErrorHandler` does not log the errors it answers. `ProblemDetailsErrorHandler` without a fallback answers any error itself and logs it like the other adapters. The runtime `Handler` logs a document it cannot build, validate or encode the same way, with the reason `OpenAPI document build failed`, and answers a plain 500. Under `RegisterEcho` it returns the error to Echo's `HTTPErrorHandler` instead.
- **Library events.** Route registration, schema generation and document builds are logged at Info and Debug level, and a response the client did not receive in full at Warn level: a runtime `Handler` document or Swagger UI page (`OpenAPI document write failed`, `Swagger UI write failed`), or a chi or mux problem+json error (`error response write failed`), with the request attributes above. They go only to a logger passed to `SetLogger`. Set it before registering routes.

## Error Types

### ErrorResponse

Standard error response schema used for 4xx/5xx responses.

```go
type ErrorResponse struct {
    Error   string `json:"error"`
    Message string `json:"message,omitempty"`
    Code    string `json:"code,omitempty"`
}
```

**Usage:**

Automatically included when using `WithStandardErrors()` or `WithNotFoundError()`.

