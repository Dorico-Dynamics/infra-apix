package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	apix "github.com/Infra-Forge/infra-apix"
	"github.com/Infra-Forge/infra-apix/internal/errorhandler"
	"github.com/Infra-Forge/infra-apix/internal/generation"
	"github.com/Infra-Forge/infra-apix/internal/swaggerui"
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

	// CacheTTL, when set, also rebuilds the document once it is this old. The
	// handler builds the document on the first request and keeps it, or the
	// error that stopped the build, until a route or plugin is registered or
	// removed.
	CacheTTL time.Duration

	// SpecPath is the HTTP path serving the document. Default derived from format (/openapi.json).
	SpecPath string

	// EnableSwaggerUI serves Swagger UI at SwaggerUIPath, default /swagger,
	// and the files it loads below that path. The files are embedded in the
	// binary, so the page loads nothing from another origin. Its
	// Content-Security-Policy lets it send requests, the document's and Try it
	// out's, only to its own origin and to the origins of Servers. Servers that
	// CustomizeBuilder or a plugin adds are not among them.
	EnableSwaggerUI bool
	SwaggerUIPath   string

	// CustomizeBuilder tunes the builder before each build.
	CustomizeBuilder func(*openapi.Builder)
}

// Handler serves the OpenAPI document, built once and kept until the routes or
// plugins change, and an optional Swagger UI.
type Handler struct {
	cfg        Config
	newBuilder func() *openapi.Builder

	// swaggerUI is the Swagger UI page and its files when EnableSwaggerUI is
	// set.
	swaggerUI []swaggerui.Route

	// mu runs builds one at a time and guards built.
	mu    sync.Mutex
	built *builtDocument
}

// builtDocument is the outcome of one build: the encoded document, or the
// error that stopped it.
type builtDocument struct {
	// generation is the registries' change count the build started from.
	generation uint64
	at         time.Time
	payload    []byte
	ctype      string
	err        error
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
	if cfg.EnableSwaggerUI {
		h.swaggerUI = swaggerui.Routes(cfg.SwaggerUIPath, cfg.SpecPath, cfg.Servers)
	}
	h.newBuilder = func() *openapi.Builder {
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
// schemes for each build's builder. CustomizeBuilder or a plugin may change a
// builder's schemes down to a scheme's fields, so no two builds, and not the
// caller's Config, may share a scheme object. The copy goes through the
// schemes' JSON form: a scheme's Origin is dropped, and a $ref scheme keeps
// only its reference. NewHandler fails for schemes that do not survive it, so
// later copies cannot fail.
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

// RegisterHTTP registers handlers on the provided mux: the document at
// SpecPath, and when EnableSwaggerUI is set, the Swagger UI page and each file
// it loads at its own path.
func (h *Handler) RegisterHTTP(mux *http.ServeMux) {
	mux.Handle(h.cfg.SpecPath, h)
	for _, route := range h.swaggerUI {
		mux.HandleFunc(route.Path, func(w http.ResponseWriter, r *http.Request) {
			if err := route.Write(w, r); err != nil {
				errorhandler.LogWriteFailed(r, "Swagger UI write failed", err)
			}
		})
	}
}

// RegisterEcho registers handlers on an echo server. A document it cannot
// build, validate or encode is returned as the error to Echo's
// HTTPErrorHandler, as the Echo adapter does with an error without an HTTP
// status, so apix does not log it. Echo's default HTTPErrorHandler answers
// {"message":"Internal Server Error"}, adds the error's text only when Echo's
// Debug is on, and does not log it. A response the client did not receive in
// full is logged at Warn level, as under ServeHTTP, and its error is still
// returned to Echo.
func (h *Handler) RegisterEcho(e *echo.Echo) {
	e.GET(h.cfg.SpecPath, func(c echo.Context) error {
		payload, ctype, err := h.getSpec(c.Request().Context())
		if err != nil {
			return err
		}
		if err := c.Blob(http.StatusOK, ctype, payload); err != nil {
			errorhandler.LogWriteFailed(c.Request(), "OpenAPI document write failed", err)
			return err
		}
		return nil
	})
	for _, route := range h.swaggerUI {
		e.GET(route.Path, func(c echo.Context) error {
			if err := route.Write(c.Response(), c.Request()); err != nil {
				errorhandler.LogWriteFailed(c.Request(), "Swagger UI write failed", err)
				return err
			}
			return nil
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
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write(payload); err != nil {
		errorhandler.LogWriteFailed(r, "OpenAPI document write failed", err)
	}
}

// getSpec returns the encoded document. It builds it on the first request, and
// again once a route or plugin has been registered or removed since, or once
// it is CacheTTL old. A failed build is kept the same way. Builds run one at a
// time, so concurrent first requests share one. Callers must not modify the
// returned bytes: later requests receive the same slice.
func (h *Handler) getSpec(ctx context.Context) ([]byte, string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	gen := generation.Current()
	if d := h.built; d != nil && d.generation == gen && (h.cfg.CacheTTL <= 0 || time.Since(d.at) < h.cfg.CacheTTL) {
		return d.payload, d.ctype, d.err
	}
	// The outcome serves later requests too, so this request's cancellation
	// must not reach the build.
	payload, ctype, err := h.build(context.WithoutCancel(ctx))
	h.built = &builtDocument{generation: gen, at: time.Now(), payload: payload, ctype: ctype, err: err}
	return payload, ctype, err
}

// build builds the document from the registered routes with a new builder,
// validates it when Validate is set, and encodes it.
func (h *Handler) build(ctx context.Context) ([]byte, string, error) {
	routes := apix.Snapshot()
	if len(routes) == 0 {
		return nil, "", errors.New("no routes registered")
	}
	b := h.newBuilder()
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

	return openapi.EncodeDocument(doc, h.cfg.Format)
}

// RenderSwaggerUI returns a standalone Swagger UI page that shows the document
// at specPath and loads swagger-ui-dist from unpkg.com: the release the Handler
// embeds, checked with Subresource Integrity. It starts Swagger UI from an
// inline script, so a Content-Security-Policy must allow that script. The
// Handler's page, see Config.EnableSwaggerUI, needs neither the CDN nor an
// inline script.
func RenderSwaggerUI(specPath string) string { return swaggerui.CDNPage(specPath) }
