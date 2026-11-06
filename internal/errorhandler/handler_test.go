package errorhandler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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

// TestResolveKeepsAnErrorStatus: an error with a 4xx or 5xx status is sent as
// is, and nothing is logged.
func TestResolveKeepsAnErrorStatus(t *testing.T) {
	var logs bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(prev) })

	for _, status := range []int{400, 404, 418, 499, 500, 503, 599} {
		err := fmt.Errorf("lookup: %w", apix.WithStatus(status, "custom message"))
		gotStatus, got := Resolve("GET", "/test", err)
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
		gotStatus, got := Resolve("GET", "/test", apix.WithStatus(status, "custom message"))
		if gotStatus != http.StatusInternalServerError || !reflect.DeepEqual(got, InternalError()) {
			t.Errorf("status %d: Resolve = %d, %#v; want 500 and InternalError", status, gotStatus, got)
		}
		want := fmt.Sprintf(`error with HTTP status %d, not 4xx or 5xx, in "GET" "/test": http %d: custom message`+"\n", status, status)
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
	if got := strings.Count(logs.String(), `error with HTTP status 0, not 4xx or 5xx, in "GET" "/test": Missing status`); got != 2 {
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
