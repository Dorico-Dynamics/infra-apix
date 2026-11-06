# apix

**Code-first OpenAPI 3.1 documentation for Go APIs**

[![Go Version](https://img.shields.io/badge/go-1.27.3+-blue.svg)](https://go.dev/dl/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Coverage](https://img.shields.io/badge/coverage-84%25-brightgreen.svg)](coverage.out)

`apix` is a Go library that generates deterministic OpenAPI 3.1 specifications directly from your typed HTTP handlers. No comments, no YAML editing, no drift—just code.

## Why apix?

Current Go OpenAPI tooling (swaggo, spec-first generators) relies on comments or manual YAML editing, allowing AI agents and developers to drift from code. `apix` provides:

- **Code-first**: Derive routes, schemas, and security from Go handlers
- **Type-safe**: Generic handlers `HandlerFunc[TReq, TResp]` with compile-time safety
- **Deterministic**: Sorted paths/components for git-friendly diffs
- **DX defaults**: Auto-inject 201 Location headers, 401/403 on secured routes
- **Runtime + CLI**: Serve `/openapi.json` live or generate static specs with `apix generate`
- **Guardrails**: DO-NOT-EDIT headers, CI drift checks, spec-guard command

## Features

- ✅ **OpenAPI 3.1** spec generation with kin-openapi
- ✅ **5 Framework adapters** - Echo, Chi, Gorilla/Mux, Gin, and Fiber
- ✅ **Struct tag parsing** (`json`, `validate`, `binding`)
- ✅ **Nullable types** (pointer detection)
- ✅ **Security schemes** (route-level and global)
- ✅ **Standard error responses** (shared ErrorResponse schema for 4xx/5xx)
- ✅ **Custom parameters** (query, path, header)
- ✅ **Runtime endpoints** (`/openapi.json`, optional Swagger UI)
- ✅ **CLI tool** (`apix generate`, `apix spec-guard`)
- ✅ **Validation** (kin-openapi validation + custom validators)

## Installation

```bash
go get github.com/Infra-Forge/apix
```

## Quick Start

Choose your framework: [Echo](#echo-example) | [Chi](#chi-example) | [Gorilla/Mux](#gorillamux-example)

### Echo Example

#### 1. Define your models and handlers

```go
package main

import (
    "context"
    "github.com/Infra-Forge/apix"
    echoadapter "github.com/Infra-Forge/apix/echo"
    "github.com/labstack/echo/v4"
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
```

### 2. Register routes with the Echo adapter

```go
func main() {
    e := echo.New()
    adapter := echoadapter.New(e)

    // Register typed handlers
    echoadapter.Post(adapter, "/api/items", createItemHandler,
        apix.WithSummary("Create a new item"),
        apix.WithTags("items"),
        apix.WithSecurity("BearerAuth"),
    )

    echoadapter.Get(adapter, "/api/items/:id", getItemHandler,
        apix.WithSummary("Get item by ID"),
        apix.WithTags("items"),
    )

    e.Start(":8080")
}
```

### 3. Serve OpenAPI spec at runtime

```go
import (
    "github.com/Infra-Forge/apix/runtime"
)

func main() {
    // ... register routes ...

    // Serve OpenAPI spec
    handler, _ := runtime.NewHandler(runtime.Config{
        Title:           "My API",
        Version:         "1.0.0",
        Format:          "json",
        EnableSwaggerUI: true,
        Servers:         []string{"https://api.example.com"},
    })
    handler.RegisterEcho(e)

    // Now available at:
    // - GET /openapi.json (spec)
    // - GET /swagger (Swagger UI)

    e.Start(":8080")
}
```

### Chi Example

```go
package main

import (
    "context"
    "net/http"
    "github.com/Infra-Forge/apix"
    chiadapter "github.com/Infra-Forge/apix/chi"
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

    chiadapter.Get(adapter, "/api/items/{id}", getItemHandler,
        apix.WithSummary("Get item by ID"),
        apix.WithTags("items"),
    )

    // Serve OpenAPI spec
    handler, _ := runtime.NewHandler(runtime.Config{
        Title:           "My API",
        Version:         "1.0.0",
        EnableSwaggerUI: true,
    })
    handler.RegisterHTTP(http.NewServeMux()) // or use Chi router

    http.ListenAndServe(":8080", r)
}
```

### Gorilla/Mux Example

```go
package main

import (
    "context"
    "net/http"
    "github.com/Infra-Forge/apix"
    muxadapter "github.com/Infra-Forge/apix/mux"
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

    muxadapter.Get(adapter, "/api/items/{id}", getItemHandler,
        apix.WithSummary("Get item by ID"),
        apix.WithTags("items"),
    )

    // Serve OpenAPI spec
    handler, _ := runtime.NewHandler(runtime.Config{
        Title:           "My API",
        Version:         "1.0.0",
        EnableSwaggerUI: true,
    })
    handler.RegisterHTTP(http.NewServeMux()) // or use Mux router

    http.ListenAndServe(":8080", r)
}
```

### Generate static spec with CLI

```bash
# Install CLI
go install github.com/Infra-Forge/apix/cmd/apix@latest

# Generate spec
apix generate \
  --title "My API" \
  --version "1.0.0" \
  --servers "https://api.example.com" \
  --out docs/openapi.yaml

# Check for drift in CI
apix spec-guard --existing docs/openapi.yaml
```

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

| Method | Default Status | Auto-injected Headers | Auto-injected Responses |
|--------|----------------|----------------------|------------------------|
| POST   | 201 Created    | Location (URI)       | 401, 403 (if secured)  |
| GET    | 200 OK         | -                    | 401, 403 (if secured)  |
| PUT    | 200 OK         | -                    | 401, 403 (if secured)  |
| PATCH  | 200 OK         | -                    | 401, 403 (if secured)  |
| DELETE | 204 No Content | -                    | 401, 403 (if secured)  |

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

## CLI Reference

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

### CI Integration

```yaml
# .github/workflows/ci.yml
- name: Check OpenAPI spec drift
  run: |
    go run ./cmd/apix spec-guard --existing docs/openapi.yaml
```

## Guardrails & Enforcement

Prevent manual spec edits and ensure your OpenAPI spec stays in sync with your code.

### 1. DO-NOT-EDIT Header

All generated specs include a header warning against manual edits:

```yaml
# DO NOT EDIT -- generated by apix generate -- update Go handlers instead
```

### 2. CI Drift Detection

The `spec-guard` command detects when your committed spec drifts from your code.

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
          go run ./cmd/apix generate --out docs/openapi.yaml
          if ! git diff --exit-code docs/openapi.yaml; then
            echo "❌ OpenAPI spec has drifted from code!"
            exit 1
          fi
```

Or use the Makefile:

```bash
make check-spec  # Check for drift
make ci          # Run all CI checks (fmt, lint, test, check-spec)
```

### 3. Pre-commit Hook

Install a git hook to prevent commits with spec drift:

```bash
make install-hooks
```

This installs `.githooks/pre-commit` which runs `apix spec-guard` before each commit.

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
# 1. Generate initial spec
make generate-spec

# 2. Install pre-commit hook
make install-hooks

# 3. Configure CODEOWNERS (see .github/CODEOWNERS)

# 4. Add CI check (see .github/workflows/test.yml)

# 5. Commit everything
git add docs/openapi.yaml .github/CODEOWNERS
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

    // Caching
    CacheTTL: 5 * time.Minute, // Cache spec for 5 minutes (0 = no cache)

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

### ✅ Milestone 1 (v0.1) - Complete
- Echo adapter with typed handlers
- Struct tag parsing, nullable types
- OpenAPI 3.1 builder with deterministic output
- CLI (`generate`, `spec-guard`)
- Runtime endpoints with Swagger UI

### ✅ Milestone 2 (v0.2) - Complete
- ✅ Chi adapter (88% test coverage)
- ✅ Gorilla/Mux adapter (88% test coverage)
- ✅ Gin adapter (87.5% test coverage)
- ✅ Fiber adapter (86.9% test coverage)
- ✅ Shared error schema with standard 4xx/5xx responses
- ✅ Golden tests for spec validation
- ✅ Integration tests for all 5 frameworks
- ✅ Comprehensive documentation

### 🔮 Milestone 3 (v0.3) - Planned
- Typed query/header parameter structs
- Middleware auto-detection for security
- Pagination headers, ETag support
- Structured examples via tags/helpers
- Multipart/form-data support
- Plugin hooks for custom metadata
- Observability (logging, metrics)

## Testing

```bash
# Run tests
make test

# Run tests with coverage
make cover

# Generate coverage report
make cover-html
```

Current coverage: **85%+** (exceeds 80% target)
- Core: 89%
- All framework adapters: 82.5%-88%

## Contributing

We welcome contributions! Please see our [Contributing Guide](docs/CONTRIBUTING.md) for details.

**Quick Start:**
1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Ensure tests pass and coverage is above 80%
5. Submit a pull request

## Community

- **Issues**: [GitHub Issues](https://github.com/Infra-Forge/apix/issues)
- **Discussions**: [GitHub Discussions](https://github.com/Infra-Forge/apix/discussions)
- **Examples**: [examples/](examples/)

## License

MIT License - see [LICENSE](LICENSE) for details

---

**Status**: Milestone 1 & 2 complete (5 framework adapters, comprehensive documentation)
**Maintainer**: Teodorico Mazivila
**Repository**: [github.com/Infra-Forge/apix](https://github.com/Infra-Forge/apix)