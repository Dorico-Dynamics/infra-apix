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

	for path, msg := range map[string]string{"/openapi.json": "OpenAPI document write failed", "/swagger": "Swagger UI write failed"} {
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
