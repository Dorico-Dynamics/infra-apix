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

// TestSwaggerUIRender: the CDN page loads a pinned swagger-ui-dist release,
// checked with Subresource Integrity.
func TestSwaggerUIRender(t *testing.T) {
	html := runtime.RenderSwaggerUI("/spec.json")
	for _, want := range []string{`url: "/spec.json"`, "<title>Swagger UI</title>", "https://unpkg.com/swagger-ui-dist@5.", `integrity="sha384-`} {
		if !strings.Contains(html, want) {
			t.Errorf("page lacks %s:\n%s", want, html)
		}
	}
	if strings.Contains(html, "swagger-ui-dist@5/") {
		t.Errorf("page loads the floating major swagger-ui-dist@5:\n%s", html)
	}
}

// TestSwaggerUIHandler: RegisterHTTP serves the page with its
// Content-Security-Policy, which admits the origins of Servers, and the files
// the page loads below its path.
func TestSwaggerUIHandler(t *testing.T) {
	apix.ResetRegistry()
	apix.RegisterRoute(&apix.RouteRef{Method: apix.MethodGet, Path: "/ui", Responses: map[int]*apix.ResponseRef{http.StatusOK: {}}})

	h, err := runtime.NewHandler(runtime.Config{EnableSwaggerUI: true, Servers: []string{"https://api.example.com/v1", "/relative"}})
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}

	mux := http.NewServeMux()
	h.RegisterHTTP(mux)

	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/swagger", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	csp := resp.Header().Get("Content-Security-Policy")
	for _, want := range []string{"script-src 'self';", "connect-src 'self' https://api.example.com;"} {
		if !strings.Contains(csp, want) {
			t.Errorf("Content-Security-Policy %q lacks %s", csp, want)
		}
	}
	if !strings.Contains(resp.Body.String(), `<script src="/swagger/swagger-ui-bundle.js"></script>`) {
		t.Errorf("page does not load the embedded bundle:\n%s", resp.Body.String())
	}

	for path, ctype := range map[string]string{
		"/swagger/swagger-ui.css":         "text/css; charset=utf-8",
		"/swagger/swagger-ui-bundle.js":   "text/javascript; charset=utf-8",
		"/swagger/swagger-initializer.js": "text/javascript; charset=utf-8",
	} {
		resp := httptest.NewRecorder()
		mux.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, path, nil))
		if resp.Code != http.StatusOK || resp.Header().Get("Content-Type") != ctype || resp.Body.Len() == 0 {
			t.Errorf("%s: %d %q with %d bytes, want 200 %q with the file", path, resp.Code, resp.Header().Get("Content-Type"), resp.Body.Len(), ctype)
		}
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
	if csp := respUI.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self';") {
		t.Errorf("Content-Security-Policy = %q, want scripts from the page's origin only", csp)
	}

	respJS := httptest.NewRecorder()
	e.ServeHTTP(respJS, httptest.NewRequest(http.MethodGet, "/swagger/swagger-ui-bundle.js", nil))
	if respJS.Code != http.StatusOK || respJS.Header().Get("Content-Type") != "text/javascript; charset=utf-8" || respJS.Body.Len() == 0 {
		t.Errorf("bundle: %d %q with %d bytes, want 200 JavaScript", respJS.Code, respJS.Header().Get("Content-Type"), respJS.Body.Len())
	}
}
