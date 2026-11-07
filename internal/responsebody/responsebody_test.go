package responsebody

import (
	"bytes"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Infra-Forge/infra-apix/internal/logging"
)

// headerOnlyWriter records calls and supports neither flushing nor unwrapping.
type headerOnlyWriter struct {
	header   http.Header
	statuses []int
	body     bytes.Buffer
	writeErr error
}

func (w *headerOnlyWriter) Header() http.Header {
	if w.header == nil {
		w.header = http.Header{}
	}
	return w.header
}

func (w *headerOnlyWriter) WriteHeader(status int) { w.statuses = append(w.statuses, status) }

func (w *headerOnlyWriter) Write(b []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	return w.body.Write(b)
}

func TestWriteJSONEncodesBeforeWriting(t *testing.T) {
	w := &headerOnlyWriter{}
	err := WriteJSON(w, httptest.NewRequest(http.MethodGet, "/", nil), http.StatusOK, map[string]float64{"value": math.NaN()})

	if err == nil || err.Error() != "json: unsupported value: NaN" {
		t.Fatalf("err = %v, want the encoding error", err)
	}
	if len(w.statuses) != 0 || w.body.Len() != 0 || len(w.header) != 0 {
		t.Errorf("wrote statuses %v, body %q, headers %v; want nothing", w.statuses, w.body.String(), w.header)
	}
}

func TestWriteJSONSendsJSONEncoderBytes(t *testing.T) {
	w := &headerOnlyWriter{}
	payload := map[string]string{"name": "<a&b>"}
	if err := WriteJSON(w, httptest.NewRequest(http.MethodGet, "/", nil), http.StatusCreated, payload); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	if len(w.statuses) != 1 || w.statuses[0] != http.StatusCreated {
		t.Errorf("statuses = %v, want [201]", w.statuses)
	}
	if ct := w.header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if got, want := w.body.String(), "{\"name\":\"\\u003ca\\u0026b\\u003e\"}\n"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestWriteJSONLogsAFailedWriteAndReturnsNil(t *testing.T) {
	var logs bytes.Buffer
	logging.Configure(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { logging.Configure(nil) })

	w := &headerOnlyWriter{writeErr: errors.New("write: broken pipe")}
	if err := WriteJSON(w, httptest.NewRequest(http.MethodPost, "/items", nil), http.StatusCreated, map[string]int{"id": 1}); err != nil {
		t.Fatalf("err = %v, want nil once the status is out", err)
	}

	if len(w.statuses) != 1 {
		t.Errorf("statuses = %v, want one", w.statuses)
	}
	line := logs.String()
	for _, want := range []string{`"level":"WARN"`, `"msg":"response write failed"`, `"path":"/items"`, `"error":"write: broken pipe"`} {
		if !strings.Contains(line, want) {
			t.Errorf("log %q lacks %s", line, want)
		}
	}
}

func TestWriterCommitted(t *testing.T) {
	cases := []struct {
		name string
		use  func(w *Writer)
		want bool
	}{
		{name: "untouched", use: func(w *Writer) {}, want: false},
		{name: "headers only", use: func(w *Writer) { w.Header().Set("X-Test", "1") }, want: false},
		{name: "informational status", use: func(w *Writer) { w.WriteHeader(http.StatusEarlyHints) }, want: false},
		{name: "switching protocols", use: func(w *Writer) { w.WriteHeader(http.StatusSwitchingProtocols) }, want: true},
		{name: "final status", use: func(w *Writer) { w.WriteHeader(http.StatusCreated) }, want: true},
		{name: "body", use: func(w *Writer) { _, _ = w.Write([]byte("x")) }, want: true},
		{name: "empty body write", use: func(w *Writer) { _, _ = w.Write(nil) }, want: true},
		{name: "flush", use: func(w *Writer) { w.Flush() }, want: true},
		{name: "flush through ResponseController", use: func(w *Writer) { _ = http.NewResponseController(w).Flush() }, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := NewWriter(httptest.NewRecorder())
			tc.use(w)
			if got := w.Committed(); got != tc.want {
				t.Errorf("Committed() = %v, want %v", got, tc.want)
			}
		})
	}
}

// A flush the underlying writer cannot perform sends nothing, so the response
// stays open.
func TestWriterFlushWithoutFlusherLeavesTheResponseOpen(t *testing.T) {
	inner := &headerOnlyWriter{}
	w := NewWriter(inner)
	w.Flush()
	if w.Committed() {
		t.Error("Committed() = true after a flush the writer cannot perform")
	}
	if len(inner.statuses) != 0 {
		t.Errorf("statuses = %v, want none", inner.statuses)
	}
}

func TestWriterUnwrap(t *testing.T) {
	inner := httptest.NewRecorder()
	if got := NewWriter(inner).Unwrap(); got != inner {
		t.Errorf("Unwrap() = %v, want the wrapped writer", got)
	}
}
