# Examples

This directory contains reference examples showing how to use the `apix` library with different Go web frameworks.

## Table of Contents

- [Available Examples](#available-examples)
- [Running Examples](#running-examples)
- [Example Features](#example-features)
- [Framework Comparison](#framework-comparison)
- [Using in Your Project](#using-in-your-project)

## Available Examples

### Complete Examples (InfraNotes Financial API)

Full-featured financial analytics API demonstrating production-ready patterns:

| Example | Framework | Port | Description |
|---------|-----------|------|-------------|
| **[infranotes](infranotes/)** | Echo | 8080 | Echo framework implementation |
| **[infranotes-chi](infranotes-chi/)** | Chi | 8081 | Chi router implementation |
| **[infranotes-gin](infranotes-gin/)** | Gin | 8082 | Gin framework implementation |
| **[infranotes-mux](infranotes-mux/)** | Gorilla/Mux | 8083 | Mux router implementation |

### Features Demonstrated

All examples include:

✅ **Complete CRUD Operations**
- Create, Read, Update, Delete for multiple resources
- Documents, Transactions, Categories

✅ **Advanced Type Support**
- `uuid.UUID` - Unique identifiers
- `decimal.Decimal` - Financial precision
- `time.Time` - Timestamps
- Nested structs - Complex response types

✅ **OpenAPI 3.1 Features**
- Automatic schema generation
- Request/response documentation
- Security schemes (Bearer Auth)
- Parameter definitions
- Error responses

✅ **Production Patterns**
- Pagination with metadata
- Standard error handling
- Health check endpoints
- Swagger UI integration
- Middleware (logging, recovery, CORS)

## Running Examples

### Quick Start

```bash
# Navigate to an example
cd examples/infranotes

# Run directly
go run main.go

# Or build and run
go build -o infranotes
./infranotes
```

### Access Points

Once running, each example exposes:

- **OpenAPI Spec**: `http://localhost:PORT/openapi.json`
- **Swagger UI**: `http://localhost:PORT/swagger`
- **Health Check**: `http://localhost:PORT/health`

### Test the API

```bash
# List documents
curl http://localhost:8080/api/documents

# Create a document
curl -X POST http://localhost:8080/api/documents \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Bank Statement",
    "document_type": "bank_statement",
    "file_path": "/uploads/statement.pdf"
  }'

# Get a specific document
curl http://localhost:8080/api/documents/{id}

# List transactions
curl http://localhost:8080/api/transactions

# List categories
curl http://localhost:8080/api/categories
```

## Example Features

### 1. Type-Safe Handlers

All examples use the canonical handler signature:

```go
func createDocument(ctx context.Context, req *CreateDocumentRequest) (DocumentResponse, error) {
    // Implementation
}
```

### 2. Nested Response Types

```go
type DocumentListResponse struct {
    Data       []DocumentModel `json:"data"`
    Pagination PaginationMeta  `json:"pagination"`
}

type PaginationMeta struct {
    Page    int  `json:"page"`
    PerPage int  `json:"per_page"`
    Total   int  `json:"total"`
    Pages   int  `json:"pages"`
    HasNext bool `json:"has_next"`
    HasPrev bool `json:"has_prev"`
}
```

### 3. Complex Types

```go
type TransactionModel struct {
    ID              uuid.UUID       `json:"id"`
    Amount          decimal.Decimal `json:"amount"`
    TransactionDate time.Time       `json:"transaction_date"`
    CreatedAt       time.Time       `json:"created_at"`
    UpdatedAt       time.Time       `json:"updated_at"`
}
```

### 4. Security Schemes

```go
handler, _ := runtime.NewHandler(runtime.Config{
    // Every scheme the routes name with apix.WithSecurity must be declared
    SecuritySchemes: openapi3.SecuritySchemes{
        "BearerAuth": &openapi3.SecuritySchemeRef{
            Value: openapi3.NewJWTSecurityScheme(),
        },
    },
})
```

### 5. Route Documentation

```go
echoadapter.Post(adapter, "/api/documents", createDocument,
    apix.WithSummary("Upload financial document"),
    apix.WithDescription("Upload a new financial document for processing"),
    apix.WithTags("Documents"),
    apix.WithSecurity("BearerAuth", "documents:write"),
    apix.WithStandardErrors(),
)
```

## Framework Comparison

### Echo (Port 8080)

**Pros:**
- Balanced performance and features
- Rich middleware ecosystem
- Good documentation

**Example:**
```go
e := echo.New()
adapter := echoadapter.New(e)
echoadapter.Post(adapter, "/api/users", createUser)
```

### Chi (Port 8081)

**Pros:**
- Lightweight and composable
- Standard library compatible
- Minimal dependencies

**Example:**
```go
r := chi.NewRouter()
adapter := chiadapter.New(r)
chiadapter.Post(adapter, "/api/users", createUser)
```

### Gin (Port 8082)

**Pros:**
- Highest performance
- Large ecosystem
- Popular choice

**Example:**
```go
r := gin.New()
adapter := ginadapter.New(r)
ginadapter.Post(adapter, "/api/users", createUser)
```

### Gorilla/Mux (Port 8083)

**Pros:**
- Battle-tested
- Mature ecosystem
- Flexible routing

**Example:**
```go
r := mux.NewRouter()
adapter := muxadapter.New(r)
muxadapter.Post(adapter, "/api/users", createUser)
```

## Using in Your Project

### Step 1: Install Dependencies

```bash
go get github.com/Infra-Forge/infra-apix
go get github.com/labstack/echo/v4  # or your framework of choice
```

### Step 2: Copy Example Code

Choose the example that matches your framework and copy relevant parts:

- Handler definitions
- Route registration
- OpenAPI configuration
- Middleware setup

### Step 3: Adapt to Your Needs

Modify:
- Model definitions for your domain
- Handler implementations for your business logic
- Route paths and operations
- Security schemes
- Server URLs

### Step 4: Generate OpenAPI Spec

The examples serve the spec at runtime. To write a static file, use a small program that registers your routes (see [Programmatic Generation](../docs/OPENAPI_GENERATION.md#programmatic-generation)):

```bash
mkdir -p docs
go run ./cmd/openapi > docs/openapi.yaml
```

## Project Structure

Each example follows this structure:

```
infranotes/
├── main.go           # Main application file
├── go.mod            # Go module definition
└── go.sum            # Dependency checksums
```

**main.go contains:**
1. Model definitions (requests, responses)
2. Handler implementations
3. Route registration
4. OpenAPI configuration
5. Server setup

## Development Tips

### Local Development

```bash
# Each example's go.mod already replaces infra-apix with this checkout
cd examples/infranotes
go run main.go
```

### Hot Reload

Use tools like `air` for hot reload during development:

```bash
go install github.com/cosmtrek/air@latest
air
```

### Testing

```bash
# Run tests
go test ./...

# With coverage
go test -v -race -coverprofile=coverage.out ./...
```

## Common Patterns

### Pagination

```go
type ListResponse struct {
    Data       []Item         `json:"data"`
    Pagination PaginationMeta `json:"pagination"`
}
```

### Error Handling

```go
echoadapter.Post(adapter, "/api/users", createUser,
    apix.WithStandardErrors(),
    apix.WithNotFoundError("User not found"),
)
```

### Security

```go
echoadapter.Post(adapter, "/api/admin/users", adminAction,
    apix.WithSecurity("BearerAuth", "admin:write"),
)
```

### Parameters

```go
echoadapter.Get(adapter, "/api/users", listUsers,
    apix.WithParameter(apix.Parameter{
        Name:       "page",
        In:         "query",
        SchemaType: "integer",
        Required:   false,
    }),
)
```

## Next Steps

- **[Getting Started Guide](../docs/GETTING_STARTED.md)** - Build your first API
- **[Framework Guides](../docs/FRAMEWORK_GUIDES.md)** - Framework-specific details
- **[API Reference](../docs/API_REFERENCE.md)** - Complete API documentation
- **[OpenAPI Generation](../docs/OPENAPI_GENERATION.md)** - Advanced features

## Notes

- Examples use `replace` directive in `go.mod` to reference the local library
- In production, use the published version: `github.com/Infra-Forge/infra-apix@latest`
- All examples are production-ready and can be used as templates
- Each example is self-contained and can run independently

