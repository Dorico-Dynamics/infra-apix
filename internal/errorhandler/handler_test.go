package errorhandler

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	apix "github.com/Infra-Forge/infra-apix"
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
	if !strings.Contains(logs.String(), `unrecognized error in "GET" "/test": pq: relation "ledger" does not exist`) {
		t.Errorf("the original error must be logged server-side, got %q", logs.String())
	}
}

// TestLogUnhandledKeepsAnEncodedLineBreakInOneLine: the path is decoded, so
// %0A arrives as a line break and would start a forged log line if written
// as is.
func TestLogUnhandledKeepsAnEncodedLineBreakInOneLine(t *testing.T) {
	var logs bytes.Buffer
	prev, flags := log.Writer(), log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(prev); log.SetFlags(flags) })

	r := httptest.NewRequest("GET", "/ledger%0Aunrecognized%20error%20in%20GET%20/admin", nil)
	HandleError(httptest.NewRecorder(), r, errors.New("boom"), false)

	want := `unrecognized error in "GET" "/ledger\nunrecognized error in GET /admin": boom` + "\n"
	if logs.String() != want {
		t.Errorf("log = %q, want %q", logs.String(), want)
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
