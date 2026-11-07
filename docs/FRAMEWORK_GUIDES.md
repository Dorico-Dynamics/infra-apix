# Framework Integration Guides

Detailed guides for integrating `apix` with each supported framework.

## Table of Contents

- [Echo](#echo)
- [Chi](#chi)
- [Gorilla/Mux](#gorillamux)
- [Gin](#gin)
- [Fiber](#fiber)
- [Comparison](#framework-comparison)

---

## Echo

### Installation

```bash
go get github.com/Infra-Forge/infra-apix
go get github.com/labstack/echo/v4
```

### Basic Setup

```go
package main

import (
    "context"
    "log"
    
    apix "github.com/Infra-Forge/infra-apix"
    echoadapter "github.com/Infra-Forge/infra-apix/echo"
    "github.com/Infra-Forge/infra-apix/runtime"
    "github.com/labstack/echo/v4"
    "github.com/labstack/echo/v4/middleware"
)

type CreateUserRequest struct {
    Name  string `json:"name" validate:"required"`
    Email string `json:"email" validate:"required,email"`
}

type UserResponse struct {
    ID    string `json:"id"`
    Name  string `json:"name"`
    Email string `json:"email"`
}

func createUser(ctx context.Context, req *CreateUserRequest) (UserResponse, error) {
    return UserResponse{
        ID:    "user-123",
        Name:  req.Name,
        Email: req.Email,
    }, nil
}

func main() {
    e := echo.New()
    e.Use(middleware.Logger())
    e.Use(middleware.Recover())
    
    adapter := echoadapter.New(e)
    
    echoadapter.Post(adapter, "/api/users", createUser,
        apix.WithSummary("Create a new user"),
        apix.WithTags("Users"),
        apix.WithStandardErrors(),
    )
    
    // Serve OpenAPI spec
    handler, _ := runtime.NewHandler(runtime.Config{
        Title:           "My API",
        Version:         "1.0.0",
        EnableSwaggerUI: true,
    })
    handler.RegisterEcho(e)
    
    e.Start(":8080")
}
```

### Path Parameters

Echo uses `:param` syntax for path parameters:

```go
echoadapter.Get(adapter, "/api/users/:id", getUser,
    apix.WithSummary("Get user by ID"),
    apix.WithParameter(apix.Parameter{
        Name:       "id",
        In:         "path",
        SchemaType: "string",
        Required:   true,
    }),
)
```

### Custom Options

```go
import (
    "context"
    "errors"

    apix "github.com/Infra-Forge/infra-apix"
    echoadapter "github.com/Infra-Forge/infra-apix/echo"
    "github.com/labstack/echo/v4"
)

adapter := echoadapter.New(e, echoadapter.Options{
    Decoder: func(ctx context.Context, c echo.Context, dst any) error {
        // Custom request decoding. The default decoder also rejects unknown
        // fields and trailing content, and runs e.Validator.
        return c.Bind(dst)
    },
    ResponseEncoder: func(ctx context.Context, c echo.Context, status int, payload any, ref *apix.RouteRef) error {
        // Custom response encoding
        return c.JSON(status, payload)
    },
    ErrorHandler: func(err error) error {
        // Keep the status and message of apix errors such as apix.NotFound.
        var coded apix.StatusCoder
        if errors.As(err, &coded) {
            return echo.NewHTTPError(coded.HTTPStatus(), coded.Error())
        }
        // Echo's error handler answers any other error with a 500 that
        // leaves the error's message out.
        return err
    },
})
```

---

## Chi

### Installation

```bash
go get github.com/Infra-Forge/infra-apix
go get github.com/go-chi/chi/v5
```

### Basic Setup

```go
package main

import (
    "context"
    "log"
    "net/http"
    
    apix "github.com/Infra-Forge/infra-apix"
    chiadapter "github.com/Infra-Forge/infra-apix/chi"
    "github.com/Infra-Forge/infra-apix/runtime"
    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
)

func main() {
    r := chi.NewRouter()
    r.Use(middleware.Logger)
    r.Use(middleware.Recoverer)
    
    adapter := chiadapter.New(r)
    
    chiadapter.Post(adapter, "/api/users", createUser,
        apix.WithSummary("Create a new user"),
        apix.WithTags("Users"),
    )
    
    // Serve OpenAPI spec
    handler, _ := runtime.NewHandler(runtime.Config{
        Title:           "My API",
        Version:         "1.0.0",
        EnableSwaggerUI: true,
    })
    
    mux := http.NewServeMux()
    handler.RegisterHTTP(mux)
    r.Mount("/", mux)
    
    http.ListenAndServe(":8080", r)
}
```

### Path Parameters

Chi uses `{param}` syntax for path parameters:

```go
chiadapter.Get(adapter, "/api/users/{id}", getUser,
    apix.WithSummary("Get user by ID"),
    apix.WithParameter(apix.Parameter{
        Name:       "id",
        In:         "path",
        SchemaType: "string",
        Required:   true,
    }),
)
```

### Custom Validation

`chiadapter.Validator` has one method, `Validate(any) error`. go-playground/validator checks structs with `Struct`, so wrap it:

```go
import (
    chiadapter "github.com/Infra-Forge/infra-apix/chi"
    "github.com/go-playground/validator/v10"
)

type structValidator struct{ v *validator.Validate }

func (s structValidator) Validate(dst any) error { return s.v.Struct(dst) }

adapter := chiadapter.New(r, chiadapter.Options{
    Validator: structValidator{v: validator.New()},
})
```

---

## Gorilla/Mux

### Installation

```bash
go get github.com/Infra-Forge/infra-apix
go get github.com/gorilla/mux
```

### Basic Setup

```go
package main

import (
    "context"
    "log"
    "net/http"
    
    apix "github.com/Infra-Forge/infra-apix"
    muxadapter "github.com/Infra-Forge/infra-apix/mux"
    "github.com/Infra-Forge/infra-apix/runtime"
    "github.com/gorilla/mux"
)

func main() {
    r := mux.NewRouter()
    
    adapter := muxadapter.New(r)
    
    muxadapter.Post(adapter, "/api/users", createUser,
        apix.WithSummary("Create a new user"),
        apix.WithTags("Users"),
    )
    
    // Serve OpenAPI spec
    handler, _ := runtime.NewHandler(runtime.Config{
        Title:           "My API",
        Version:         "1.0.0",
        EnableSwaggerUI: true,
    })
    
    mux := http.NewServeMux()
    handler.RegisterHTTP(mux)
    r.PathPrefix("/").Handler(mux)
    
    http.ListenAndServe(":8080", r)
}
```

### Path Parameters

Mux uses `{param}` syntax for path parameters:

```go
muxadapter.Get(adapter, "/api/users/{id}", getUser,
    apix.WithSummary("Get user by ID"),
)
```

### Subrouters

```go
api := r.PathPrefix("/api").Subrouter()
adapter := muxadapter.New(api)

muxadapter.Get(adapter, "/users", listUsers)
muxadapter.Post(adapter, "/users", createUser)
```

---

## Gin

### Installation

```bash
go get github.com/Infra-Forge/infra-apix
go get github.com/gin-gonic/gin
```

### Basic Setup

```go
package main

import (
    "context"
    "log"
    "net/http"
    
    apix "github.com/Infra-Forge/infra-apix"
    ginadapter "github.com/Infra-Forge/infra-apix/gin"
    "github.com/Infra-Forge/infra-apix/runtime"
    "github.com/gin-gonic/gin"
)

func main() {
    r := gin.Default()
    
    adapter := ginadapter.New(r)
    
    ginadapter.Post(adapter, "/api/users", createUser,
        apix.WithSummary("Create a new user"),
        apix.WithTags("Users"),
    )
    
    // Serve OpenAPI spec
    handler, _ := runtime.NewHandler(runtime.Config{
        Title:           "My API",
        Version:         "1.0.0",
        EnableSwaggerUI: true,
    })
    
    mux := http.NewServeMux()
    handler.RegisterHTTP(mux)
    r.Any("/openapi.json", gin.WrapH(mux))
    r.Any("/swagger", gin.WrapH(mux))
    
    r.Run(":8080")
}
```

### Path Parameters

Gin uses `:param` syntax for path parameters:

```go
ginadapter.Get(adapter, "/api/users/:id", getUser,
    apix.WithSummary("Get user by ID"),
)
```

### Route Groups

```go
api := r.Group("/api")
adapter := ginadapter.New(r)

ginadapter.Get(adapter, "/api/users", listUsers)
ginadapter.Post(adapter, "/api/users", createUser)
```

---

## Fiber

### Installation

```bash
go get github.com/Infra-Forge/infra-apix
go get github.com/gofiber/fiber/v3
```

### Basic Setup

```go
package main

import (
    "context"
    "log"
    
    apix "github.com/Infra-Forge/infra-apix"
    fiberadapter "github.com/Infra-Forge/infra-apix/fiber"
    "github.com/Infra-Forge/infra-apix/runtime"
    "github.com/gofiber/fiber/v3"
)

func main() {
    app := fiber.New()
    
    adapter := fiberadapter.New(app)
    
    fiberadapter.Post(adapter, "/api/users", createUser,
        apix.WithSummary("Create a new user"),
        apix.WithTags("Users"),
    )
    
    // Serve OpenAPI spec
    handler, _ := runtime.NewHandler(runtime.Config{
        Title:           "My API",
        Version:         "1.0.0",
        EnableSwaggerUI: true,
    })
    
    app.Get("/openapi.json", func(c fiber.Ctx) error {
        payload, ctype, err := handler.ServeHTTP(c.Response().BodyWriter(), c.Request())
        if err != nil {
            return err
        }
        c.Set("Content-Type", ctype)
        return c.Send(payload)
    })
    
    app.Listen(":8080")
}
```

### Path Parameters

Fiber uses `:param` syntax for path parameters:

```go
fiberadapter.Get(adapter, "/api/users/:id", getUser,
    apix.WithSummary("Get user by ID"),
)
```

---

## Framework Comparison

| Feature | Echo | Chi | Mux | Gin | Fiber |
|---------|------|-----|-----|-----|-------|
| **Path Params** | `:id` | `{id}` | `{id}` | `:id` | `:id` |
| **Middleware** | Built-in | Built-in | Manual | Built-in | Built-in |
| **Performance** | Fast | Fast | Moderate | Very Fast | Very Fast |
| **Ecosystem** | Large | Medium | Large | Very Large | Growing |
| **Learning Curve** | Easy | Easy | Easy | Easy | Easy |
| **OpenAPI Integration** | ✅ | ✅ | ✅ | ✅ | ✅ |

### When to Use Each

**Echo:**
- Need a balanced framework with good performance and features
- Want built-in middleware and utilities
- Building REST APIs with standard patterns

**Chi:**
- Need a lightweight, composable router
- Want fine-grained control over routing
- Building microservices with minimal dependencies

**Gorilla/Mux:**
- Working with existing Gorilla ecosystem
- Need mature, battle-tested routing
- Building traditional web applications

**Gin:**
- Need maximum performance
- Building high-throughput APIs
- Want a large ecosystem and community

**Fiber:**
- Need Express.js-like API in Go
- Building high-performance APIs
- Want modern features and fast development

### Migration Between Frameworks

All adapters use the same handler signature, making migration straightforward:

```go
// Same handler works with all frameworks
func createUser(ctx context.Context, req *CreateUserRequest) (UserResponse, error) {
    // Implementation
}

// Just change the adapter
// From Echo:
echoadapter.Post(echoAdapter, "/api/users", createUser, opts...)

// To Chi:
chiadapter.Post(chiAdapter, "/api/users", createUser, opts...)

// To Gin:
ginadapter.Post(ginAdapter, "/api/users", createUser, opts...)
```

Only path parameter syntax needs adjustment when migrating.

