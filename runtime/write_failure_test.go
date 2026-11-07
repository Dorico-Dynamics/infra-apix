package runtime_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	apix "github.com/Infra-Forge/infra-apix"
	"github.com/Infra-Forge/infra-apix/runtime"
	"github.com/labstack/echo/v4"
)

// failingWriter is a ResponseWriter whose client went away.
type failingWriter struct{ *httptest.ResponseRecorder }

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write: broken pipe") }

// TestAFailedWriteIsLoggedAtWarn: a response the client did not receive in
// full is a library event. It goes to the logger passed to apix.SetLogger at
// Warn level, with the request's ids.
func TestAFailedWriteIsLoggedAtWarn(t *testing.T) {
	registerPlainRoute(t)
	var logs bytes.Buffer
	apix.SetLogger(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { apix.SetLogger(nil) })
	mux := specMux(t, runtime.Config{EnableSwaggerUI: true})

	for path, msg := range map[string]string{
		"/openapi.json":                 "OpenAPI document write failed",
		"/swagger":                      "Swagger UI write failed",
		"/swagger/swagger-ui-bundle.js": "Swagger UI write failed",
	} {
		logs.Reset()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-Request-ID", "req-1")
		mux.ServeHTTP(failingWriter{httptest.NewRecorder()}, req)

		var line map[string]any
		if err := json.Unmarshal(logs.Bytes(), &line); err != nil {
			t.Fatalf("%s: want one JSON line, got %q: %v", path, logs.String(), err)
		}
		for key, want := range map[string]any{
			"level": "WARN", "msg": msg, "method": "GET", "path": path, "request_id": "req-1", "error": "write: broken pipe",
		} {
			if line[key] != want {
				t.Errorf("%s: %s = %v, want %v", path, key, line[key], want)
			}
		}
	}
}

// TestAFailedEchoWriteIsLoggedAtWarn: under RegisterEcho too, a response the
// client did not receive in full is logged at Warn level, and Echo still
// receives the error.
func TestAFailedEchoWriteIsLoggedAtWarn(t *testing.T) {
	registerPlainRoute(t)
	var logs bytes.Buffer
	apix.SetLogger(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { apix.SetLogger(nil) })
	h, err := runtime.NewHandler(runtime.Config{EnableSwaggerUI: true})
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	e := echo.New()
	var handled error
	e.HTTPErrorHandler = func(err error, _ echo.Context) { handled = err }
	h.RegisterEcho(e)

	for path, msg := range map[string]string{
		"/openapi.json":                 "OpenAPI document write failed",
		"/swagger":                      "Swagger UI write failed",
		"/swagger/swagger-ui-bundle.js": "Swagger UI write failed",
	} {
		logs.Reset()
		handled = nil
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-Request-ID", "req-1")
		e.ServeHTTP(failingWriter{httptest.NewRecorder()}, req)

		var line map[string]any
		if err := json.Unmarshal(logs.Bytes(), &line); err != nil {
			t.Fatalf("%s: want one JSON line, got %q: %v", path, logs.String(), err)
		}
		for key, want := range map[string]any{
			"level": "WARN", "msg": msg, "method": "GET", "path": path, "request_id": "req-1", "error": "write: broken pipe",
		} {
			if line[key] != want {
				t.Errorf("%s: %s = %v, want %v", path, key, line[key], want)
			}
		}
		if handled == nil || handled.Error() != "write: broken pipe" {
			t.Errorf("%s: Echo's error handler got %v, want the write error", path, handled)
		}
	}
}
