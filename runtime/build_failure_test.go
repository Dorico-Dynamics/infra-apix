package runtime_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apix "github.com/Infra-Forge/infra-apix"
	"github.com/Infra-Forge/infra-apix/openapi"
	"github.com/Infra-Forge/infra-apix/runtime"
	echo "github.com/labstack/echo/v4"
)

// captureErrorLogs sends apix's Error-level lines to the returned buffer for
// the rest of the test.
func captureErrorLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	apix.SetLogger(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelError})))
	t.Cleanup(func() { apix.SetLogger(nil) })
	return &logs
}

func registerPlainRoute(t *testing.T) {
	t.Helper()
	apix.ResetRegistry()
	t.Cleanup(apix.ResetRegistry)
	apix.RegisterRoute(&apix.RouteRef{Method: apix.MethodGet, Path: "/items", Responses: map[int]*apix.ResponseRef{http.StatusOK: {}}})
}

func registerNoRoutes(t *testing.T) {
	t.Helper()
	apix.ResetRegistry()
	t.Cleanup(apix.ResetRegistry)
}

// TestAFailedBuildAnswersAPlain500: a document the handler cannot build,
// validate or encode is answered with a plain 500. The error, which names
// routes, types and schemes, is logged with the request's ids instead.
func TestAFailedBuildAnswersAPlain500(t *testing.T) {
	blankTitle := func(b *openapi.Builder) { b.Info.Title = "" }
	for _, tc := range []struct {
		name     string
		register func(*testing.T)
		cfg      runtime.Config
		err      string
	}{
		{"undeclared scheme", func(t *testing.T) { registerSecuredRoute(t, "BearerAuth") }, runtime.Config{},
			`build openapi: security scheme "BearerAuth" is not declared in components.securitySchemes, but GET /items uses it`},
		{"invalid document", registerPlainRoute, runtime.Config{Validate: true, CustomizeBuilder: blankTitle}, "validate openapi: "},
		{"unsupported format", registerPlainRoute, runtime.Config{Format: "toml"}, `unsupported format "toml"`},
		{"no routes", registerNoRoutes, runtime.Config{}, "no routes registered"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.register(t)
			logs := captureErrorLogs(t)
			h, err := runtime.NewHandler(tc.cfg)
			if err != nil {
				t.Fatalf("new handler: %v", err)
			}

			req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
			req.Header.Set("X-Request-ID", "req-1")
			req.Header.Set("X-Correlation-ID", "corr-1")
			resp := httptest.NewRecorder()
			h.ServeHTTP(resp, req)

			if resp.Code != http.StatusInternalServerError || resp.Body.String() != "Internal Server Error\n" {
				t.Errorf("got %d %q, want 500 %q", resp.Code, resp.Body.String(), "Internal Server Error\n")
			}
			if ct := resp.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
				t.Errorf("Content-Type = %q, want text/plain; charset=utf-8", ct)
			}
			var line map[string]any
			if err := json.Unmarshal(logs.Bytes(), &line); err != nil {
				t.Fatalf("want one JSON line, got %q: %v", logs.String(), err)
			}
			for key, want := range map[string]any{
				"level": "ERROR", "msg": "error answered with a generic 500", "reason": "OpenAPI document build failed",
				"method": "GET", "path": "/openapi.json", "request_id": "req-1", "correlation_id": "corr-1",
			} {
				if line[key] != want {
					t.Errorf("%s = %v, want %v", key, line[key], want)
				}
			}
			if logged, _ := line["error"].(string); !strings.Contains(logged, tc.err) {
				t.Errorf("error = %q, want it to contain %q", logged, tc.err)
			}
		})
	}
}

// TestRegisterEchoLeavesAFailedBuildToEchosErrorHandler: under RegisterEcho
// the build error goes to Echo's HTTPErrorHandler, as an error without an HTTP
// status does in the Echo adapter, and apix does not log it. Echo's default
// handler answers with the status text only.
func TestRegisterEchoLeavesAFailedBuildToEchosErrorHandler(t *testing.T) {
	registerSecuredRoute(t, "BearerAuth")
	logs := captureErrorLogs(t)
	h, err := runtime.NewHandler(runtime.Config{})
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	e := echo.New()
	var handled error
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		handled = err
		e.DefaultHTTPErrorHandler(err, c)
	}
	h.RegisterEcho(e)

	resp := httptest.NewRecorder()
	e.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))

	if handled == nil || !strings.Contains(handled.Error(), `security scheme "BearerAuth" is not declared`) {
		t.Errorf("Echo's error handler got %v, want the build error", handled)
	}
	if got := strings.TrimSpace(resp.Body.String()); resp.Code != http.StatusInternalServerError || got != `{"message":"Internal Server Error"}` {
		t.Errorf("got %d %s, want 500 {\"message\":\"Internal Server Error\"}", resp.Code, got)
	}
	if logs.Len() != 0 {
		t.Errorf("apix must not log an error it leaves to Echo, got %q", logs.String())
	}
}
