package errorhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	apix "github.com/Infra-Forge/infra-apix"
	"github.com/Infra-Forge/infra-apix/internal/logging"
)

func TestHandleErrorWithStatusCoder(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/test", nil)
	err := apix.NotFound("resource not found")

	HandleError(w, r, err, false)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "resource not found") {
		t.Errorf("expected error message in body, got %s", w.Body.String())
	}
}

func TestHandleErrorWithProblemDetails(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/test", nil)
	err := apix.BadRequest("invalid input")

	HandleError(w, r, err, true)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/problem+json" {
		t.Errorf("expected Content-Type application/problem+json, got %s", contentType)
	}
	if !strings.Contains(w.Body.String(), "invalid input") {
		t.Errorf("expected error message in body, got %s", w.Body.String())
	}
}

func TestHandleErrorWithGenericError(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/test", nil)
	err := errors.New("generic error")

	HandleError(w, r, err, false)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}
	// Should return generic message, not the actual error
	if !strings.Contains(w.Body.String(), "Internal Server Error") {
		t.Errorf("expected generic error message in body, got %s", w.Body.String())
	}
	// Should NOT leak the internal error message
	if strings.Contains(w.Body.String(), "generic error") {
		t.Errorf("should not leak internal error message, got %s", w.Body.String())
	}
}

func TestHandleErrorWithGenericErrorInProblemMode(t *testing.T) {
	var logs bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(prev) })

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/test", nil)
	HandleError(w, r, errors.New(`pq: relation "ledger" does not exist`), true)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("expected Content-Type application/problem+json, got %s", ct)
	}
	var problem map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	want := map[string]any{"type": "about:blank", "title": "Internal Server Error", "status": float64(500)}
	if !reflect.DeepEqual(problem, want) {
		t.Errorf("problem = %v, want %v", problem, want)
	}
	if !strings.Contains(logs.String(), `ERROR error answered with a generic 500 reason="no HTTP status" method=GET path=/test error="pq: relation \"ledger\" does not exist"`) {
		t.Errorf("the original error must be logged server-side, got %q", logs.String())
	}
}

// TestErrorLineKeepsAnEncodedLineBreakInOneLine: the path is decoded, so %0A
// arrives as a line break and would start a forged log line if written as is.
func TestErrorLineKeepsAnEncodedLineBreakInOneLine(t *testing.T) {
	var logs bytes.Buffer
	prev, flags := log.Writer(), log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(prev); log.SetFlags(flags) })

	r := httptest.NewRequest("GET", "/ledger%0Aunrecognized%20error%20in%20GET%20/admin", nil)
	HandleError(httptest.NewRecorder(), r, errors.New("boom"), false)

	want := `ERROR error answered with a generic 500 reason="no HTTP status" method=GET path="/ledger\nunrecognized error in GET /admin" error=boom` + "\n"
	if logs.String() != want {
		t.Errorf("log = %q, want %q", logs.String(), want)
	}
}

// TestResolveKeepsAnErrorStatus: an error with a 4xx or 5xx status is sent as
// is, and nothing is logged.
func TestResolveKeepsAnErrorStatus(t *testing.T) {
	var logs bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(prev) })

	for _, status := range []int{400, 404, 418, 499, 500, 503, 599} {
		err := fmt.Errorf("lookup: %w", apix.WithStatus(status, "custom message"))
		gotStatus, got := Resolve(context.Background(), Request{Method: "GET", Path: "/test"}, err)
		if gotStatus != status || got != err {
			t.Errorf("status %d: Resolve = %d, %v; want %d and the error itself", status, gotStatus, got, status)
		}
	}
	if logs.Len() != 0 {
		t.Errorf("nothing should be logged, got %q", logs.String())
	}
}

// TestResolveReplacesAnyOtherStatus: net/http panics on a status below 100 or
// above 999, and any other status outside 4xx and 5xx would present the error
// as an informational, success or redirect response. The error is logged, and
// InternalError is sent instead.
func TestResolveReplacesAnyOtherStatus(t *testing.T) {
	var logs bytes.Buffer
	prev, flags := log.Writer(), log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(prev); log.SetFlags(flags) })

	for _, status := range []int{-1, 0, 99, 100, 103, 200, 204, 302, 304, 399, 600, 999, 1000} {
		logs.Reset()
		gotStatus, got := Resolve(context.Background(), Request{Method: "GET", Path: "/test"}, apix.WithStatus(status, "custom message"))
		if gotStatus != http.StatusInternalServerError || !reflect.DeepEqual(got, InternalError()) {
			t.Errorf("status %d: Resolve = %d, %#v; want 500 and InternalError", status, gotStatus, got)
		}
		want := fmt.Sprintf(`ERROR error answered with a generic 500 reason="HTTP status outside 4xx and 5xx" status=%d method=GET path=/test error="http %d: custom message"`+"\n", status, status)
		if logs.String() != want {
			t.Errorf("status %d: log = %q, want %q", status, logs.String(), want)
		}
	}
}

// TestHandleErrorWithAProblemWithoutStatus: a ProblemDetails without Status
// made WriteHeader panic. It is logged and answered with InternalError, in
// both response formats.
func TestHandleErrorWithAProblemWithoutStatus(t *testing.T) {
	var logs bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(prev) })

	for _, problemMode := range []bool{false, true} {
		w := httptest.NewRecorder()
		HandleError(w, httptest.NewRequest("GET", "/test", nil), &apix.ProblemDetails{Title: "Missing status"}, problemMode)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("problem mode %v: status = %d, want 500", problemMode, w.Code)
		}
		if strings.Contains(w.Body.String(), "Missing status") {
			t.Errorf("problem mode %v: the error must not reach the client, got %s", problemMode, w.Body.String())
		}
	}
	if got := strings.Count(logs.String(), `reason="HTTP status outside 4xx and 5xx" status=0 method=GET path=/test error="Missing status"`); got != 2 {
		t.Errorf("want the error logged once per request, got %q", logs.String())
	}
}

func TestHandleErrorFallsBackToTextWhenProblemCannotBeMarshaled(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/test", nil)
	problem := apix.NewProblemDetails(http.StatusConflict, "Conflict", "version mismatch").
		WithExtension("unencodable", make(chan int))

	HandleError(w, r, problem, true)

	if w.Code != http.StatusConflict {
		t.Errorf("expected status 409, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("expected text/plain fallback, got %s", ct)
	}
	if strings.TrimSpace(w.Body.String()) != "version mismatch" {
		t.Errorf("unexpected body %q", w.Body.String())
	}
}

func TestInternalError(t *testing.T) {
	err := InternalError()
	if err.HTTPStatus() != http.StatusInternalServerError || err.Error() != "Internal Server Error" {
		t.Fatalf("got %d %q", err.HTTPStatus(), err.Error())
	}
	if err.Detail != "" || err.Type != "" || len(err.Extensions) != 0 {
		t.Fatalf("InternalError must carry nothing beyond the status: %+v", err)
	}
}

func TestHandleErrorWithWrappedStatusCoder(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/test", nil)
	innerErr := apix.Unauthorized("not authorized")
	wrappedErr := errors.Join(errors.New("wrapper"), innerErr)

	HandleError(w, r, wrappedErr, false)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestHandleErrorProblemDetailsWithExtensions(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/test", nil)
	problem := apix.NewProblemDetails(http.StatusTeapot, "I'm a teapot", "Short and stout").
		WithExtension("spout", "handle")

	HandleError(w, r, problem, true)

	if w.Code != http.StatusTeapot {
		t.Errorf("expected status 418, got %d", w.Code)
	}
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/problem+json" {
		t.Errorf("expected Content-Type application/problem+json, got %s", contentType)
	}
	if !strings.Contains(w.Body.String(), "spout") {
		t.Errorf("expected extension field in body, got %s", w.Body.String())
	}
}

// TestErrorLineGoesToTheConfiguredLogger: with a logger passed to
// apix.SetLogger, the line goes there with the request ids and the request
// context, and not to the log package. Without one, it goes to
// slog.Default(), which writes through the log package.
func TestErrorLineGoesToTheConfiguredLogger(t *testing.T) {
	var std bytes.Buffer
	prev, flags := log.Writer(), log.Flags()
	log.SetOutput(&std)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(prev); log.SetFlags(flags) })

	var lines bytes.Buffer
	handler := &contextRecorder{Handler: slog.NewJSONHandler(&lines, nil)}
	logging.Configure(slog.New(handler))
	t.Cleanup(func() { logging.Configure(nil) })

	type traceKey struct{}
	r := httptest.NewRequest("POST", "/ledger", nil)
	r.Header.Set("X-Request-Id", "req-1")
	r.Header.Set("X-Correlation-ID", "corr-1")
	r = r.WithContext(context.WithValue(r.Context(), traceKey{}, "trace-1"))
	HandleError(httptest.NewRecorder(), r, errors.New("boom"), true)

	var line map[string]any
	if err := json.Unmarshal(lines.Bytes(), &line); err != nil {
		t.Fatalf("line is not one JSON object: %v: %q", err, lines.String())
	}
	delete(line, "time")
	want := map[string]any{
		"level": "ERROR", "msg": "error answered with a generic 500", "reason": "no HTTP status",
		"method": "POST", "path": "/ledger", "request_id": "req-1", "correlation_id": "corr-1", "error": "boom",
	}
	if !reflect.DeepEqual(line, want) {
		t.Errorf("line = %v, want %v", line, want)
	}
	if handler.ctx == nil || handler.ctx.Value(traceKey{}) != "trace-1" {
		t.Error("the handler must receive the request's context")
	}
	if std.Len() != 0 {
		t.Errorf("nothing should reach the log package, got %q", std.String())
	}

	logging.Configure(nil)
	lines.Reset()
	HandleError(httptest.NewRecorder(), httptest.NewRequest("GET", "/ledger", nil), errors.New("boom"), false)
	if lines.Len() != 0 {
		t.Errorf("the removed logger received %q", lines.String())
	}
	if want := `ERROR error answered with a generic 500 reason="no HTTP status" method=GET path=/ledger error=boom` + "\n"; std.String() != want {
		t.Errorf("log = %q, want %q", std.String(), want)
	}
}

// contextRecorder keeps the context of the last record it handled.
type contextRecorder struct {
	slog.Handler
	ctx context.Context
}

func (h *contextRecorder) Handle(ctx context.Context, r slog.Record) error {
	h.ctx = ctx
	return h.Handler.Handle(ctx, r)
}

// failingWriter is a ResponseWriter whose client went away.
type failingWriter struct{ *httptest.ResponseRecorder }

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write: broken pipe") }

// TestAFailedErrorWriteIsLoggedAtWarn: a problem+json error the client did not
// receive in full is a library event. It goes to the logger passed to
// apix.SetLogger at Warn level, with the request's ids.
func TestAFailedErrorWriteIsLoggedAtWarn(t *testing.T) {
	var lines bytes.Buffer
	logging.Configure(slog.New(slog.NewJSONHandler(&lines, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { logging.Configure(nil) })

	r := httptest.NewRequest("GET", "/ledger", nil)
	r.Header.Set("X-Request-ID", "req-1")
	r.Header.Set("X-Correlation-ID", "corr-1")
	HandleError(failingWriter{httptest.NewRecorder()}, r, apix.NotFound("no such ledger"), true)

	var line map[string]any
	if err := json.Unmarshal(lines.Bytes(), &line); err != nil {
		t.Fatalf("want one JSON line, got %q: %v", lines.String(), err)
	}
	delete(line, "time")
	want := map[string]any{
		"level": "WARN", "msg": "error response write failed", "method": "GET", "path": "/ledger",
		"request_id": "req-1", "correlation_id": "corr-1", "error": "write: broken pipe",
	}
	if !reflect.DeepEqual(line, want) {
		t.Errorf("line = %v, want %v", line, want)
	}
}
