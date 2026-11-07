# Framework Integration Guides

Detailed guides for integrating `apix` with each supported framework.

## Table of Contents

- [Shared Models and Handler](#shared-models-and-handler)
- [Echo](#echo)
- [Chi](#chi)
- [Gorilla/Mux](#gorillamux)
- [Gin](#gin)
- [Fiber](#fiber)
- [Reading Path Parameters](#reading-path-parameters)
- [Comparison](#framework-comparison)

---

## Shared Models and Handler

The setups below are `main.go` files. Each builds with this `users.go` beside it:

```go
package main

import "context"

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
```

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
    "log"

    apix "github.com/Infra-Forge/infra-apix"
    echoadapter "github.com/Infra-Forge/infra-apix/echo"
    "github.com/Infra-Forge/infra-apix/runtime"
    "github.com/labstack/echo/v4"
    "github.com/labstack/echo/v4/middleware"
)

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
    handler, err := runtime.NewHandler(runtime.Config{
        Title:           "My API",
        Version:         "1.0.0",
        EnableSwaggerUI: true,
    })
    if err != nil {
        log.Fatal(err)
    }
    handler.RegisterEcho(e)

    log.Fatal(e.Start(":8080"))
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
    handler, err := runtime.NewHandler(runtime.Config{
        Title:           "My API",
        Version:         "1.0.0",
        EnableSwaggerUI: true,
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
    handler, err := runtime.NewHandler(runtime.Config{
        Title:           "My API",
        Version:         "1.0.0",
        EnableSwaggerUI: true,
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
    handler, err := runtime.NewHandler(runtime.Config{
        Title:           "My API",
        Version:         "1.0.0",
        EnableSwaggerUI: true,
    })
    if err != nil {
        log.Fatal(err)
    }

    docs := http.NewServeMux()
    handler.RegisterHTTP(docs)
    r.GET("/openapi.json", gin.WrapH(docs))
    r.GET("/swagger", gin.WrapH(docs))
    r.GET("/swagger/*file", gin.WrapH(docs)) // paths below the page

    log.Fatal(r.Run(":8080"))
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
    "log"
    "net/http"

    apix "github.com/Infra-Forge/infra-apix"
    fiberadapter "github.com/Infra-Forge/infra-apix/fiber"
    "github.com/Infra-Forge/infra-apix/runtime"
    "github.com/gofiber/fiber/v3"
    "github.com/gofiber/fiber/v3/middleware/adaptor"
)

func main() {
    app := fiber.New()

    adapter := fiberadapter.New(app)

    fiberadapter.Post(adapter, "/api/users", createUser,
        apix.WithSummary("Create a new user"),
        apix.WithTags("Users"),
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

    docs := http.NewServeMux()
    handler.RegisterHTTP(docs)
    app.Use(adaptor.HTTPHandler(docs)) // registered last, so it gets what the routes above do not

    log.Fatal(app.Listen(":8080"))
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

## Reading Path Parameters

A typed handler receives the request's `context.Context` and its decoded body, not the router's context. Declare a path parameter with `apix.WithParameter` so the document lists it, and read it from the context.

**Chi** keeps path parameters in the request context:

```go
func getUser(ctx context.Context, _ *apix.NoBody) (UserResponse, error) {
    id := chi.URLParamFromCtx(ctx, "id")
    // Load the user with this id
    return UserResponse{ID: id}, nil
}
```

**Echo, Gin and Gorilla/Mux** keep them on their own types, so copy them into the request context in middleware, and read them with a helper:

```go
type pathParamsKey struct{}

func pathParam(ctx context.Context, name string) string {
    params, _ := ctx.Value(pathParamsKey{}).(map[string]string)
    return params[name]
}
```

Echo runs `e.Use` middleware after routing, so the parameters are set:

```go
e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
    return func(c echo.Context) error {
        params := make(map[string]string, len(c.ParamNames()))
        for i, name := range c.ParamNames() {
            params[name] = c.ParamValues()[i]
        }
        ctx := context.WithValue(c.Request().Context(), pathParamsKey{}, params)
        c.SetRequest(c.Request().WithContext(ctx))
        return next(c)
    }
})
```

Gin applies middleware to the routes registered after it, so call `r.Use` first:

```go
r.Use(func(c *gin.Context) {
    params := make(map[string]string, len(c.Params))
    for _, p := range c.Params {
        params[p.Key] = p.Value
    }
    c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), pathParamsKey{}, params))
    c.Next()
})
```

Gorilla/Mux:

```go
r.Use(func(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
        ctx := context.WithValue(req.Context(), pathParamsKey{}, mux.Vars(req))
        next.ServeHTTP(w, req.WithContext(ctx))
    })
})
```

**Fiber** runs `app.Use` middleware before it matches the route, so the parameters are empty there. Register a handler on the same path before the apix route; it copies them and passes the request on:

```go
app.Get("/api/users/:id", func(c fiber.Ctx) error {
    params := map[string]string{"id": c.Params("id")}
    c.SetContext(context.WithValue(c.Context(), pathParamsKey{}, params))
    return c.Next()
})
fiberadapter.Get(adapter, "/api/users/:id", getUser)
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

Path parameter syntax changes between routers, and so does the way a handler reads path parameters (see [Reading Path Parameters](#reading-path-parameters)).

