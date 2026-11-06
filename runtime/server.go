package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	apix "github.com/Infra-Forge/infra-apix"
	"github.com/Infra-Forge/infra-apix/internal/errorhandler"
	"github.com/Infra-Forge/infra-apix/openapi"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/labstack/echo/v4"
)

// Config controls runtime OpenAPI serving behaviour.
type Config struct {
	// Format controls encoding ("json" or "yaml"). Default: json.
	Format string

	// Title and Version override the document info.
	Title   string
	Version string

	// Servers are applied to the generated document.
	Servers []string

	// SecuritySchemes become the document's components.securitySchemes, such
	// as the BearerAuth that routes name with apix.WithSecurity. The build
	// fails for a scheme a route names but nothing declares. CustomizeBuilder
	// runs afterwards and can change them.
	SecuritySchemes openapi3.SecuritySchemes

	// Validate checks the document with kin-openapi on every build and answers
	// 500 instead of serving an invalid one. Off unless set.
	Validate bool

	// CacheTTL controls how long the generated spec is cached before regeneration.
	// Zero duration disables caching (generate per request).
	CacheTTL time.Duration

	// SpecPath is the HTTP path serving the document. Default derived from format (/openapi.json).
	SpecPath string

	// SwaggerUI enables a lightweight UI at SwaggerUIPath referencing the spec.
	EnableSwaggerUI bool
	SwaggerUIPath   string

	// CustomizeBuilder allows additional tuning of the builder before building.
	CustomizeBuilder func(*openapi.Builder)
}

// Handler serves OpenAPI docs with optional caching and Swagger UI.
type Handler struct {
	cfg Config

	mu          sync.RWMutex
	lastBuilt   time.Time
	cachedBytes []byte
	contentType string

	builderPool sync.Pool
}

// NewHandler returns a ready-to-use Handler.
func NewHandler(cfg Config) (*Handler, error) {
	if cfg.Format == "" {
		cfg.Format = "json"
	}
	if cfg.SpecPath == "" {
		switch strings.ToLower(cfg.Format) {
		case "yaml", "yml":
			cfg.SpecPath = "/openapi.yaml"
		default:
			cfg.SpecPath = "/openapi.json"
		}
	}
	if cfg.EnableSwaggerUI && cfg.SwaggerUIPath == "" {
		cfg.SwaggerUIPath = "/swagger"
	}

	copySchemes, err := securitySchemeCopier(cfg.SecuritySchemes)
	if err != nil {
		return nil, err
	}

	h := &Handler{cfg: cfg}
	h.builderPool.New = func() any {
		b := openapi.NewBuilder()
		if cfg.Title != "" {
			b.Info.Title = cfg.Title
		}
		if cfg.Version != "" {
			b.Info.Version = cfg.Version
		}
		for _, srv := range cfg.Servers {
			srv = strings.TrimSpace(srv)
			if srv == "" {
				continue
			}
			b.Servers = append(b.Servers, &openapi3.Server{URL: srv})
		}
		b.SecuritySchemes = copySchemes()
		if cfg.CustomizeBuilder != nil {
			cfg.CustomizeBuilder(b)
		}
		return b
	}
	return h, nil
}

// securitySchemeCopier returns a function that decodes a fresh deep copy of
// schemes for each pooled builder. Builders are created concurrently, and
// CustomizeBuilder or a plugin may change a builder's schemes down to a
// scheme's fields, so no two builders, and not the caller's Config, may share
// a scheme object. The copy goes through the schemes' JSON form: a scheme's
// Origin is dropped, and a $ref scheme keeps only its reference. NewHandler
// fails for schemes that do not survive it, so later copies cannot fail.
func securitySchemeCopier(schemes openapi3.SecuritySchemes) (func() openapi3.SecuritySchemes, error) {
	if len(schemes) == 0 {
		return func() openapi3.SecuritySchemes { return nil }, nil
	}
	data, err := json.Marshal(schemes)
	if err != nil {
		return nil, fmt.Errorf("encode security schemes: %w", err)
	}
	decode := func() (openapi3.SecuritySchemes, error) {
		var copied openapi3.SecuritySchemes
		err := json.Unmarshal(data, &copied)
		return copied, err
	}
	if _, err := decode(); err != nil {
		return nil, fmt.Errorf("decode security schemes: %w", err)
	}
	return func() openapi3.SecuritySchemes {
		copied, err := decode()
		if err != nil {
			panic(fmt.Sprintf("apix runtime: security schemes that NewHandler decoded failed to decode again: %v", err))
		}
		return copied
	}, nil
}

// RegisterHTTP registers handlers on the provided mux.
func (h *Handler) RegisterHTTP(mux *http.ServeMux) {
	mux.Handle(h.cfg.SpecPath, h)
	if h.cfg.EnableSwaggerUI {
		mux.HandleFunc(h.cfg.SwaggerUIPath, h.swaggerUI)
	}
}

// RegisterEcho registers handlers on an echo server. A document it cannot
// build, validate or encode is returned as the error to Echo's
// HTTPErrorHandler, as the Echo adapter does with an error without an HTTP
// status, so apix does not log it. Echo's default HTTPErrorHandler answers
// {"message":"Internal Server Error"}, adds the error's text only when Echo's
// Debug is on, and does not log it.
func (h *Handler) RegisterEcho(e *echo.Echo) {
	e.GET(h.cfg.SpecPath, func(c echo.Context) error {
		payload, ctype, err := h.getSpec(c.Request().Context())
		if err != nil {
			return err
		}
		return c.Blob(http.StatusOK, ctype, payload)
	})
	if h.cfg.EnableSwaggerUI {
		e.GET(h.cfg.SwaggerUIPath, func(c echo.Context) error {
			return c.HTML(http.StatusOK, renderSwaggerUI(h.cfg.SpecPath))
		})
	}
}

// buildFailedReason is the reason attribute of the line logged for a document
// the handler cannot build, validate or encode.
const buildFailedReason = "OpenAPI document build failed"

// ServeHTTP serves the document. A document it cannot build, validate or
// encode is answered with a plain 500, and the error, which names routes,
// types and schemes, is logged like the adapters' generic 500s (see
// apix.SetLogger) instead of reaching the client.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	payload, ctype, err := h.getSpec(r.Context())
	if err != nil {
		errorhandler.LogReplaced(r.Context(), errorhandler.RequestOf(r), err, slog.String("reason", buildFailedReason))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set(http.CanonicalHeaderKey("Content-Type"), ctype)
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write(payload); err != nil {
		// best effort logging
		fmt.Fprintf(os.Stderr, "apix runtime: failed to write spec: %v\n", err)
	}
}

func (h *Handler) swaggerUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(http.CanonicalHeaderKey("Content-Type"), "text/html; charset=utf-8")
	if _, err := w.Write([]byte(renderSwaggerUI(h.cfg.SpecPath))); err != nil {
		fmt.Fprintf(os.Stderr, "apix runtime: failed to write swagger ui: %v\n", err)
	}
}

func (h *Handler) getSpec(ctx context.Context) ([]byte, string, error) {
	h.mu.RLock()
	if h.cfg.CacheTTL > 0 && !h.lastBuilt.IsZero() && time.Since(h.lastBuilt) < h.cfg.CacheTTL {
		payload := append([]byte(nil), h.cachedBytes...)
		ctype := h.contentType
		h.mu.RUnlock()
		return payload, ctype, nil
	}
	h.mu.RUnlock()

	b := h.builderPool.Get().(*openapi.Builder)
	defer h.builderPool.Put(b)

	routes := apix.Snapshot()
	if len(routes) == 0 {
		return nil, "", errors.New("no routes registered")
	}
	// sort servers for deterministic output
	sort.SliceStable(b.Servers, func(i, j int) bool { return b.Servers[i].URL < b.Servers[j].URL })

	doc, err := b.Build(routes)
	if err != nil {
		return nil, "", fmt.Errorf("build openapi: %w", err)
	}

	if h.cfg.Validate {
		if err := doc.Validate(ctx); err != nil {
			return nil, "", fmt.Errorf("validate openapi: %w", err)
		}
	}

	data, ctype, err := openapi.EncodeDocument(doc, h.cfg.Format)
	if err != nil {
		return nil, "", err
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.cachedBytes = append(h.cachedBytes[:0], data...)
	h.contentType = ctype
	h.lastBuilt = time.Now()
	return append([]byte(nil), data...), ctype, nil
}

func renderSwaggerUI(specPath string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <title>Swagger UI</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.onload = () => {
      window.ui = SwaggerUIBundle({
        url: '%s',
        dom_id: '#swagger-ui',
        presets: [SwaggerUIBundle.presets.apis],
        layout: 'BaseLayout'
      });
    };
  </script>
</body>
</html>`, specPath)
}

func RenderSwaggerUI(specPath string) string { return renderSwaggerUI(specPath) }
