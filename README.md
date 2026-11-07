# apix

**Code-first OpenAPI 3.1 documentation for Go APIs**

[![Go Version](https://img.shields.io/badge/go-1.27.3+-blue.svg)](https://go.dev/dl/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Test & Lint](https://github.com/Dorico-Dynamics/infra-apix/actions/workflows/test.yml/badge.svg?branch=main)](https://github.com/Dorico-Dynamics/infra-apix/actions/workflows/test.yml)

`apix` is a Go library that generates deterministic OpenAPI 3.1 specifications directly from your typed HTTP handlers. No comments, no YAML editing, no drift—just code.

## Why apix?

Current Go OpenAPI tooling (swaggo, spec-first generators) relies on comments or manual YAML editing, allowing AI agents and developers to drift from code. `apix` provides:

- **Code-first**: Derive routes, schemas, and security from Go handlers
- **Type-safe**: Generic handlers `HandlerFunc[TReq, TResp]` with compile-time safety
- **Deterministic**: Sorted paths/components for git-friendly diffs
- **DX defaults**: 201 for POST, 204 for DELETE, 401/403 on secured routes
- **Runtime**: Serve `/openapi.json` live, or write a static spec from a program of yours
- **Guardrails**: DO-NOT-EDIT headers and CI drift checks on the committed spec

## Features

- ✅ **OpenAPI 3.1** spec generation with kin-openapi
- ✅ **5 Framework adapters** - Echo, Chi, Gorilla/Mux, Gin, and Fiber
- ✅ **Struct tag parsing** (`json`, `validate`, `binding`)
- ✅ **Nullable types** (pointer detection)
- ✅ **Security schemes** (route-level and global)
- ✅ **Standard error responses** (shared ErrorResponse schema for 4xx/5xx, sent once you set an [`ErrorHandler`](docs/API_REFERENCE.md#errorresponse))
- ✅ **Custom parameters** (query, path, header)
- ✅ **Runtime endpoints** (`/openapi.json`, optional Swagger UI)
- ⚠️ **CLI tool** (`apix generate`, `apix spec-guard`): finds no routes yet ([status](docs/CLI_USAGE.md))
- ✅ **Validation** (kin-openapi validation + custom validators)

## Installation

```bash
go get github.com/Infra-Forge/infra-apix
```

## Quick Start

Choose your framework: [Echo](#echo-example) | [Chi](#chi-example) | [Gorilla/Mux](#gorillamux-example)

### Echo Example

#### 1. Define your models and handlers

Create `items.go`:

```go
package main

import (
    "context"

    apix "github.com/Infra-Forge/infra-apix"
)

type CreateItemRequest struct {
    Name        string `json:"name" validate:"required"`
    Description string `json:"description,omitempty"`
}

type ItemResponse struct {
    ID        string `json:"id"`
    Name      string `json:"name"`
    CreatedAt string `json:"created_at"`
}

func createItemHandler(ctx context.Context, req *CreateItemRequest) (ItemResponse, error) {
    // Your business logic here
    return ItemResponse{
        ID:        "item-123",
        Name:      req.Name,
        CreatedAt: "2025-01-01T00:00:00Z",
    }, nil
}

func listItemsHandler(ctx context.Context, _ *apix.NoBody) ([]ItemResponse, error) {
    return []ItemResponse{{ID: "item-123", Name: "Notebook", CreatedAt: "2025-01-01T00:00:00Z"}}, nil
}
```

#### 2. Register routes and serve the OpenAPI document

Create `main.go`:

```go
package main

import (
    "log"

    apix "github.com/Infra-Forge/infra-apix"
    echoadapter "github.com/Infra-Forge/infra-apix/echo"
    "github.com/Infra-Forge/infra-apix/runtime"
    "github.com/getkin/kin-openapi/openapi3"
    "github.com/labstack/echo/v4"
)

func main() {
    e := echo.New()
    adapter := echoadapter.New(e)

    // Register typed handlers
    echoadapter.Post(adapter, "/api/items", createItemHandler,
        apix.WithSummary("Create a new item"),
        apix.WithTags("items"),
        apix.WithSecurity("BearerAuth"),
    )

    echoadapter.Get(adapter, "/api/items", listItemsHandler,
        apix.WithSummary("List items"),
        apix.WithTags("items"),
    )

    // Serve the OpenAPI document and Swagger UI
    handler, err := runtime.NewHandler(runtime.Config{
        Title:           "My API",
        Version:         "1.0.0",
        Format:          "json",
        EnableSwaggerUI: true,
        // Declare every scheme a route names with apix.WithSecurity
        SecuritySchemes: openapi3.SecuritySchemes{
            "BearerAuth": &openapi3.SecuritySchemeRef{Value: openapi3.NewJWTSecurityScheme()},
        },
    })
    if err != nil {
        log.Fatal(err)
    }
    handler.RegisterEcho(e)

    // Now available at:
    // - GET /openapi.json (spec)
    // - GET /swagger (Swagger UI)
    log.Fatal(e.Start(":8080"))
}
```

Run it with `go mod tidy && go run .`.

### Chi Example

With `items.go` from the Echo example, replace `main.go` with:

```go
package main

import (
    "log"
    "net/http"

    apix "github.com/Infra-Forge/infra-apix"
    chiadapter "github.com/Infra-Forge/infra-apix/chi"
    "github.com/Infra-Forge/infra-apix/runtime"
    "github.com/getkin/kin-openapi/openapi3"
    "github.com/go-chi/chi/v5"
)

func main() {
    r := chi.NewRouter()
    adapter := chiadapter.New(r)

    // Register typed handlers
    chiadapter.Post(adapter, "/api/items", createItemHandler,
        apix.WithSummary("Create a new item"),
        apix.WithTags("items"),
        apix.WithSecurity("BearerAuth"),
    )

    chiadapter.Get(adapter, "/api/items", listItemsHandler,
        apix.WithSummary("List items"),
        apix.WithTags("items"),
    )

    // Serve the OpenAPI document and Swagger UI
    handler, err := runtime.NewHandler(runtime.Config{
        Title:           "My API",
        Version:         "1.0.0",
        EnableSwaggerUI: true,
        SecuritySchemes: openapi3.SecuritySchemes{
            "BearerAuth": &openapi3.SecuritySchemeRef{Value: openapi3.NewJWTSecurityScheme()},
        },
    })
    if err != nil {
        log.Fatal(err)
    }
    docs := http.NewServeMux()
    handler.RegisterHTTP(docs)
    r.Mount("/", docs) // paths no other route matches

    log.Fatal(http.ListenAndServe(":8080", r))
}
```

### Gorilla/Mux Example

With `items.go` from the Echo example, replace `main.go` with:

```go
package main

import (
    "log"
    "net/http"

    apix "github.com/Infra-Forge/infra-apix"
    muxadapter "github.com/Infra-Forge/infra-apix/mux"
    "github.com/Infra-Forge/infra-apix/runtime"
    "github.com/getkin/kin-openapi/openapi3"
    "github.com/gorilla/mux"
)

func main() {
    r := mux.NewRouter()
    adapter := muxadapter.New(r)

    // Register typed handlers
    muxadapter.Post(adapter, "/api/items", createItemHandler,
        apix.WithSummary("Create a new item"),
        apix.WithTags("items"),
        apix.WithSecurity("BearerAuth"),
    )

    muxadapter.Get(adapter, "/api/items", listItemsHandler,
        apix.WithSummary("List items"),
        apix.WithTags("items"),
    )

    // Serve the OpenAPI document and Swagger UI
    handler, err := runtime.NewHandler(runtime.Config{
        Title:           "My API",
        Version:         "1.0.0",
        EnableSwaggerUI: true,
        SecuritySchemes: openapi3.SecuritySchemes{
            "BearerAuth": &openapi3.SecuritySchemeRef{Value: openapi3.NewJWTSecurityScheme()},
        },
    })
    if err != nil {
        log.Fatal(err)
    }
    docs := http.NewServeMux()
    handler.RegisterHTTP(docs)
    r.PathPrefix("/").Handler(docs) // registered last, so it gets what the routes above do not

    log.Fatal(http.ListenAndServe(":8080", r))
}
```

### Generate a static spec

Write the spec with a small program that registers your routes and prints the document (see [Programmatic Generation](docs/OPENAPI_GENERATION.md#programmatic-generation)):

```bash
mkdir -p docs
go run ./cmd/openapi > docs/openapi.yaml
```

The `apix` CLI cannot do this yet: it reads the routes registered in its own process, so it finds none (see [CLI Usage](docs/CLI_USAGE.md)).

## Route Options

Customize route documentation with functional options:

```go
echoadapter.Post(adapter, "/api/items", handler,
    // Documentation
    apix.WithSummary("Create item"),
    apix.WithDescription("Creates a new item in the system"),
    apix.WithTags("items", "v1"),
    apix.WithOperationID("createItem"),
    apix.WithDeprecated(),

    // Security
    apix.WithSecurity("BearerAuth", "items:write"),

    // Parameters
    apix.WithParameter(apix.Parameter{
        Name:        "X-Request-ID",
        In:          "header",
        Description: "Request correlation ID",
        SchemaType:  "string",
        Required:    false,
    }),

    // Response customization
    apix.WithSuccessStatus(http.StatusCreated),
    apix.WithSuccessHeaders(http.StatusCreated, apix.HeaderRef{
        Name:        "Location",
        Description: "URI of created resource",
        SchemaType:  "string",
        Required:    true,
    }),

    // Override inferred types
    apix.WithExplicitRequestModel(&CustomModel{}, "application/json"),
    apix.WithRequestOverride(&CustomModel{}, "application/json", map[string]any{
        "name": "example",
    }),
)
```

## Handler Signatures

All handlers use the canonical signature:

```go
type HandlerFunc[TReq any, TResp any] func(ctx context.Context, req *TReq) (TResp, error)
```

### Examples

```go
// POST with request body
func createItem(ctx context.Context, req *CreateItemRequest) (ItemResponse, error)

// GET without request body
func listItems(ctx context.Context, _ *apix.NoBody) ([]ItemResponse, error)

// DELETE with no response body
func deleteItem(ctx context.Context, _ *apix.NoBody) (apix.NoBody, error)
```

## Default Behaviors

`apix` applies sensible defaults aligned with REST best practices:

| Method | Default Status | Request Body Documented | Auto-injected Responses |
|--------|----------------|-------------------------|------------------------|
| POST   | 201 Created    | Yes                     | 401, 403 (if secured)  |
| GET    | 200 OK         | No                      | 401, 403 (if secured)  |
| PUT    | 200 OK         | Yes                     | 401, 403 (if secured)  |
| PATCH  | 200 OK         | Yes                     | 401, 403 (if secured)  |
| DELETE | 204 No Content | No                      | 401, 403 (if secured)  |

A request type on GET or DELETE holds query or path values, so it is not
documented as a request body. Response headers such as `Location` are
documented only when declared, for example with `apix.WithSuccessHeaders`;
`Build` fails if that option names a status that is not one of the route's
responses.

## Struct Tags

`apix` respects standard Go struct tags:

```go
type User struct {
    ID       string  `json:"id"`                          // Required (non-pointer, no omitempty)
    Name     string  `json:"name" validate:"required"`    // Required (validate tag)
    Email    *string `json:"email,omitempty"`             // Optional (pointer + omitempty)
    Age      int     `json:"age,omitempty"`               // Optional (omitempty)
    Internal string  `json:"-"`                           // Excluded from schema
    Bio      string  `json:"bio" description:"User bio"`  // With description
}
```

Supported tags:
- `json`: Field name, omitempty, exclusion (`-`)
- `validate`: Required fields (`required`)
- `binding`: Required fields (`required`)
- `description`: Field-level documentation

A `validate` or `binding` rule whose name contains `required`, such as `required_if`, also marks the field required, unconditionally ([Validation Tags](docs/OPENAPI_GENERATION.md#validation-tags)).

## CLI Reference

The CLI cannot generate your API's document yet: it reads the routes registered in its own process, so it finds none (see [CLI Usage](docs/CLI_USAGE.md)). Its commands and flags:

### `apix generate`

Generate OpenAPI spec from registered routes.

```bash
apix generate [flags]

Flags:
  --project string     Path to Go project (default ".")
  --out string         Output path (default "docs/openapi.yaml")
  --format string      Output format: yaml or json (default "yaml")
  --title string       API title (default "API")
  --version string     API version (default "1.0.0")
  --servers string     Comma-separated server URLs
  --stdout             Write to stdout instead of file
  --validate           Validate generated spec (default true)
```

### `apix spec-guard`

Check for drift between generated spec and committed spec (for CI).

```bash
apix spec-guard [flags]

Flags:
  --existing string    Path to existing spec (defaults to --out)
  --out string         Expected spec path (default "docs/openapi.yaml")

Exit codes:
  0: No drift detected
  1: Drift detected or error
```

## Guardrails & Enforcement

Prevent manual spec edits and ensure your OpenAPI spec stays in sync with your code. The examples run the program from [Programmatic Generation](docs/OPENAPI_GENERATION.md#programmatic-generation), kept at `./cmd/openapi` in your application.

### 1. DO-NOT-EDIT Header

Start the committed spec with a header warning against manual edits, as that program does:

```yaml
# DO NOT EDIT -- generated by go run ./cmd/openapi -- update Go handlers instead
```

### 2. CI Drift Detection

Regenerate the spec in CI, and fail when it differs from the committed one.

**Note:** This is for applications that **use** apix, not the apix library itself.

```yaml
# .github/workflows/ci.yml (in your application)
jobs:
  spec-guard:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.27'

      - name: Check OpenAPI spec drift
        run: |
          git ls-files --error-unmatch docs/openapi.yaml # git diff skips an untracked file
          go run ./cmd/openapi > docs/openapi.yaml
          if ! git diff --exit-code docs/openapi.yaml; then
            echo "❌ OpenAPI spec has drifted from code!"
            exit 1
          fi
```

### 3. Pre-commit Hook

Run the same check before each commit, with an executable `.git/hooks/pre-commit` in your application. It builds the spec from the staged files, so a route change you have not staged cannot make the staged spec pass:

```bash
#!/bin/sh
# Generate the spec from the staged files, so unstaged changes do not count
tree=$(mktemp -d) || exit 1
trap 'rm -rf "$tree"' EXIT
git checkout-index --all --prefix="$tree/" || exit 1
(cd "$tree" && go run ./cmd/openapi > docs/openapi.yaml) || exit 1
if ! git show :docs/openapi.yaml | cmp -s - "$tree/docs/openapi.yaml"; then
    echo "docs/openapi.yaml does not match the staged code: regenerate and stage it" >&2
    exit 1
fi
```

To bypass (not recommended):
```bash
git commit --no-verify
```

### 4. CODEOWNERS Protection

Protect generated specs from unauthorized changes:

```
# .github/CODEOWNERS
docs/openapi.yaml @your-team/api-reviewers
docs/openapi.json @your-team/api-reviewers
```

This requires approval from designated reviewers for any spec changes.

### Complete Guardrails Setup

```bash
# 1. Generate the initial spec with ./cmd/openapi
mkdir -p docs
go run ./cmd/openapi > docs/openapi.yaml

# 2. Save the hook from step 3 as .git/hooks/pre-commit, and make it executable
chmod +x .git/hooks/pre-commit

# 3. Add the CI job from step 2 and the CODEOWNERS entries from step 4

# 4. Commit everything
git add cmd/openapi docs/openapi.yaml .github/workflows/ci.yml .github/CODEOWNERS
git commit -m "feat: add OpenAPI spec with guardrails"
```

## Runtime Configuration

```go
handler, err := runtime.NewHandler(runtime.Config{
    // Output format
    Format: "json", // or "yaml"

    // Document metadata
    Title:   "My API",
    Version: "1.0.0",
    Servers: []string{"https://api.example.com", "https://staging.example.com"},

    // Security schemes the routes name with apix.WithSecurity; the build
    // fails for a scheme a route names but nothing declares
    SecuritySchemes: openapi3.SecuritySchemes{
        "BearerAuth": &openapi3.SecuritySchemeRef{
            Value: openapi3.NewJWTSecurityScheme(),
        },
    },

    // Validation
    Validate: true, // Validate the spec on every build before serving (default: false)

    // Caching: the document is built on the first request and kept until a
    // route or plugin is registered or removed
    CacheTTL: 5 * time.Minute, // Also rebuild it every 5 minutes (default: never)

    // Paths
    SpecPath:      "/openapi.json",
    SwaggerUIPath: "/swagger",

    // Swagger UI
    EnableSwaggerUI: true,

    // Advanced customization, applied after the fields above
    CustomizeBuilder: func(b *openapi.Builder) {
        b.Info.Description = "Public API"
    },
})
```

## Logging

An adapter answers an error without a 4xx or 5xx status with a generic 500 and
logs the error at Error level, with the request's context. With a text
handler, the line reads:

```
level=ERROR msg="error answered with a generic 500" reason="no HTTP status" method=POST path=/v1/items request_id=req-1 correlation_id=corr-1 error="pq: relation \"items\" does not exist"
```

`request_id` and `correlation_id` come from the `X-Request-ID` and
`X-Correlation-ID` request headers, and are left out when a header is not set.
The Echo adapter leaves an error without a status to Echo's error handler, so it
logs only an error whose status is outside 4xx and 5xx. Echo's default error
handler does not log the errors it answers.
`echoadapter.ProblemDetailsErrorHandler(nil)`, with no fallback, answers any
error itself and logs it like the other adapters.

The runtime handler answers a document it cannot build, validate or encode
with a plain 500 and logs the error the same way, with
`reason="OpenAPI document build failed"`, so route, type and scheme names stay
out of the response. Under `RegisterEcho` it returns the error to Echo's error
handler instead.

These lines go to `slog.Default()` unless you pass your own logger. That logger
also receives route registration, schema generation and document builds, at
Info and Debug level, and a runtime handler response or a chi or mux
problem+json error the client did not receive in full, at Warn level:

```go
apix.SetLogger(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
```

## Architecture

```
┌─────────────────┐
│  Your Handlers  │  HandlerFunc[TReq, TResp]
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Framework       │  Echo / Chi / Gorilla Mux
│ Adapter         │  Registers routes + captures metadata
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Core Registry   │  Thread-safe route metadata storage
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ OpenAPI Builder │  Converts metadata → OpenAPI 3.1
└────────┬────────┘
         │
         ├──────────────┬──────────────┐
         ▼              ▼              ▼
    ┌────────┐    ┌─────────┐    ┌─────────┐
    │ Runtime│    │   CLI   │    │  Tests  │
    │ Server │    │ Generate│    │ Golden  │
    └────────┘    └─────────┘    └─────────┘
```

## Documentation

### 📚 Guides

- **[Getting Started](docs/GETTING_STARTED.md)** - Complete beginner's guide
- **[API Reference](docs/API_REFERENCE.md)** - Complete API documentation
- **[Framework Guides](docs/FRAMEWORK_GUIDES.md)** - Framework-specific integration guides
- **[OpenAPI Generation](docs/OPENAPI_GENERATION.md)** - Advanced OpenAPI features
- **[CLI Usage](docs/CLI_USAGE.md)** - Command-line tool documentation
- **[Migration Guide](MIGRATION_GUIDE.md)** - Migrating from swaggo
- **[Contributing](docs/CONTRIBUTING.md)** - How to contribute

### 💡 Examples

Complete working examples for all frameworks:

- **[Echo Example](examples/infranotes/)** - Full-featured financial API
- **[Chi Example](examples/infranotes-chi/)** - Chi router implementation
- **[Gin Example](examples/infranotes-gin/)** - Gin framework implementation
- **[Mux Example](examples/infranotes-mux/)** - Gorilla/Mux implementation

Each example demonstrates:
- CRUD operations
- Nested response types
- Pagination
- Security schemes
- Complex types (UUID, decimal.Decimal, time.Time)

## Roadmap

### ✅ Milestone 1 - Complete
- Echo adapter with typed handlers
- Struct tag parsing, nullable types
- OpenAPI 3.1 builder with deterministic output
- CLI (`generate`, `spec-guard`); it finds no routes yet (see [CLI Usage](docs/CLI_USAGE.md))
- Runtime endpoints with Swagger UI

### ✅ Milestone 2 - Complete
- ✅ Chi adapter
- ✅ Gorilla/Mux adapter
- ✅ Gin adapter
- ✅ Fiber adapter
- ✅ Shared error schema with standard 4xx/5xx responses
- ✅ Golden tests for spec validation
- ✅ Integration tests for all 5 frameworks
- ✅ Comprehensive documentation

### 🚧 Milestone 3 - In progress
- ✅ Structured examples via tags/helpers
- ✅ Multipart/form-data and form-urlencoded request bodies in the spec
- ✅ Plugin hooks for custom metadata
- ✅ Structured logging (`apix.SetLogger`)
- Typed query/header parameter structs
- Middleware auto-detection for security
- Pagination headers, ETag support
- Multipart and form decoding in the adapters
- Metrics
- A CLI that loads your routes

## Testing

```bash
# Run tests
make test

# Run tests with coverage
make cover

# Generate coverage report
make cover-html
```

CI runs the tests with the race detector on every push and pull request to `main`, and fails when total coverage is below 80%.

## Contributing

We welcome contributions! Please see our [Contributing Guide](docs/CONTRIBUTING.md) for details.

**Quick Start:**
1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Ensure tests pass and coverage is above 80%
5. Submit a pull request

## Community

- **Issues**: [GitHub Issues](https://github.com/Dorico-Dynamics/infra-apix/issues)
- **Examples**: [examples/](examples/)

## License

MIT License - see [LICENSE](LICENSE) for details

---

**Status**: v1, released from `main` ([releases](https://github.com/Dorico-Dynamics/infra-apix/releases)); Milestones 1 and 2 complete, Milestone 3 in progress
**Maintainer**: Teodorico Mazivila
**Repository**: [github.com/Dorico-Dynamics/infra-apix](https://github.com/Dorico-Dynamics/infra-apix)