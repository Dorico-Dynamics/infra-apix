package runtime_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apix "github.com/Infra-Forge/infra-apix"
	"github.com/Infra-Forge/infra-apix/openapi"
	"github.com/Infra-Forge/infra-apix/runtime"
	echo "github.com/labstack/echo/v4"
)

func TestHandlerServesSpec(t *testing.T) {
	apix.ResetRegistry()

	ref := &apix.RouteRef{Method: apix.MethodGet, Path: "/health", Responses: map[int]*apix.ResponseRef{http.StatusOK: {}}}
	apix.RegisterRoute(ref)

	h, err := runtime.NewHandler(runtime.Config{Format: "json", Validate: true})
	if err != nil {
		t.Fatalf("new handler failed: %v", err)
	}

	mux := http.NewServeMux()
	h.RegisterHTTP(mux)

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if ct := resp.Header().Get("Content-Type"); !strings.Contains(ct, "json") {
		t.Fatalf("expected json content type, got %s", ct)
	}
	if !strings.Contains(resp.Body.String(), "/health") {
		t.Fatalf("spec should contain route path")
	}
}

func TestHandlerServeHTTPValidationError(t *testing.T) {
	apix.ResetRegistry()
	apix.RegisterRoute(&apix.RouteRef{Method: apix.MethodGet, Path: "/err", Responses: map[int]*apix.ResponseRef{http.StatusOK: {}}})

	cfg := runtime.Config{Format: "invalid", Validate: true, SpecPath: "/broken"}
	badHandler, err := runtime.NewHandler(cfg)
	if err != nil {
		t.Fatalf("new handler invalid: %v", err)
	}

	mux := http.NewServeMux()
	badHandler.RegisterHTTP(mux)

	req := httptest.NewRequest(http.MethodGet, cfg.SpecPath, nil)
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected validation failure -> 500, got %d", resp.Code)
	}
}

// TestValidateIsOffUnlessSet: a document kin-openapi rejects, here one with a
// blank info title, is served while Validate is unset and answered with a
// plain 500 once it is set.
func TestValidateIsOffUnlessSet(t *testing.T) {
	apix.ResetRegistry()
	apix.RegisterRoute(&apix.RouteRef{Method: apix.MethodGet, Path: "/untitled", Responses: map[int]*apix.ResponseRef{http.StatusOK: {}}})
	blankTitle := func(b *openapi.Builder) { b.Info.Title = "" }

	for _, tc := range []struct {
		name     string
		cfg      runtime.Config
		wantCode int
		wantBody string
	}{
		{"unset", runtime.Config{CustomizeBuilder: blankTitle}, http.StatusOK, `"/untitled"`},
		{"set", runtime.Config{CustomizeBuilder: blankTitle, Validate: true}, http.StatusInternalServerError, "Internal Server Error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, err := runtime.NewHandler(tc.cfg)
			if err != nil {
				t.Fatalf("new handler: %v", err)
			}
			resp := httptest.NewRecorder()
			h.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
			if resp.Code != tc.wantCode || !strings.Contains(resp.Body.String(), tc.wantBody) {
				t.Fatalf("got %d %q, want %d containing %q", resp.Code, resp.Body.String(), tc.wantCode, tc.wantBody)
			}
		})
	}
}

func TestHandlerCaching(t *testing.T) {
	apix.ResetRegistry()
	ref := &apix.RouteRef{Method: apix.MethodGet, Path: "/cache", Responses: map[int]*apix.ResponseRef{http.StatusOK: {}}}
	apix.RegisterRoute(ref)

	h, err := runtime.NewHandler(runtime.Config{CacheTTL: time.Minute})
	if err != nil {
		t.Fatalf("new handler failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)
	first := resp.Body.Len()

	resp2 := httptest.NewRecorder()
	h.ServeHTTP(resp2, req)
	second := resp2.Body.Len()

	if first != second {
		t.Fatalf("cached response should match initial response")
	}
}

func TestSwaggerUIRender(t *testing.T) {
	html := runtime.RenderSwaggerUI("/spec.json")
	if !strings.Contains(html, "/spec.json") {
		t.Fatalf("expected swagger to reference spec path")
	}
	if !strings.Contains(html, "Swagger UI") {
		t.Fatalf("expected swagger html output")
	}
}

func TestSwaggerUIHandler(t *testing.T) {
	apix.ResetRegistry()
	apix.RegisterRoute(&apix.RouteRef{Method: apix.MethodGet, Path: "/ui", Responses: map[int]*apix.ResponseRef{http.StatusOK: {}}})

	h, err := runtime.NewHandler(runtime.Config{EnableSwaggerUI: true})
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}

	mux := http.NewServeMux()
	h.RegisterHTTP(mux)

	req := httptest.NewRequest(http.MethodGet, "/swagger", nil)
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
}

func TestRegisterEcho(t *testing.T) {
	apix.ResetRegistry()
	apix.RegisterRoute(&apix.RouteRef{Method: apix.MethodGet, Path: "/echo", Responses: map[int]*apix.ResponseRef{http.StatusOK: {}}})

	e := echo.New()
	h, err := runtime.NewHandler(runtime.Config{EnableSwaggerUI: true})
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}

	h.RegisterEcho(e)

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	resp := httptest.NewRecorder()
	e.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	reqUI := httptest.NewRequest(http.MethodGet, "/swagger", nil)
	respUI := httptest.NewRecorder()
	e.ServeHTTP(respUI, reqUI)
	if respUI.Code != http.StatusOK {
		t.Fatalf("expected swagger ui to respond 200, got %d", respUI.Code)
	}
}
