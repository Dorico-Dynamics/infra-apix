package mux_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apix "github.com/Infra-Forge/infra-apix"
	muxadapter "github.com/Infra-Forge/infra-apix/mux"
	"github.com/gorilla/mux"
)

type measurement struct {
	Value float64 `json:"value"`
}

var errBrokenPipe = errors.New("write: broken pipe")

// brokenPipeWriter behaves like a connection the client has closed: the
// status goes out, and every body write fails. It records each WriteHeader
// call, which net/http would ignore after the first.
type brokenPipeWriter struct {
	header   http.Header
	statuses []int
	writes   int
}

func (w *brokenPipeWriter) Header() http.Header {
	if w.header == nil {
		w.header = http.Header{}
	}
	return w.header
}

func (w *brokenPipeWriter) WriteHeader(status int) { w.statuses = append(w.statuses, status) }

func (w *brokenPipeWriter) Write([]byte) (int, error) {
	if len(w.statuses) == 0 {
		w.statuses = append(w.statuses, http.StatusOK)
	}
	w.writes++
	return 0, errBrokenPipe
}

// captureLogs sends apix's log lines at Warn and above to the returned buffer
// for the rest of the test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	apix.SetLogger(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { apix.SetLogger(nil) })
	return &logs
}

// logLines decodes the JSON log lines in logs.
func logLines(t *testing.T, logs *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for _, raw := range bytes.Split(bytes.TrimSpace(logs.Bytes()), []byte("\n")) {
		if len(raw) == 0 {
			continue
		}
		var line map[string]any
		if err := json.Unmarshal(raw, &line); err != nil {
			t.Fatalf("log line %q is not JSON: %v", raw, err)
		}
		lines = append(lines, line)
	}
	return lines
}

// assertOneLine checks that logs hold exactly one line, with the given fields.
func assertOneLine(t *testing.T, logs *bytes.Buffer, want map[string]any) {
	t.Helper()
	lines := logLines(t, logs)
	if len(lines) != 1 {
		t.Fatalf("want one log line, got %d: %s", len(lines), logs.String())
	}
	for key, value := range want {
		if lines[0][key] != value {
			t.Errorf("%s = %v, want %v", key, lines[0][key], value)
		}
	}
}

// TestMuxAdapterAnswersAPayloadItCannotEncodeWith500: the body is encoded
// before the status goes out, so a value JSON cannot represent becomes the
// logged generic 500, not a 200 with the error appended.
func TestMuxAdapterAnswersAPayloadItCannotEncodeWith500(t *testing.T) {
	cases := []struct {
		name        string
		problem     bool
		contentType string
		body        string
	}{
		{name: "text", contentType: "text/plain; charset=utf-8", body: "Internal Server Error"},
		{name: "problem", problem: true, contentType: "application/problem+json", body: `{"status":500,"title":"Internal Server Error","type":"about:blank"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureLogs(t)
			apix.ResetRegistry()
			r := mux.NewRouter()
			adapter := muxadapter.New(r, muxadapter.Options{UseProblemDetails: tc.problem})
			muxadapter.Get(adapter, "/measurement", func(ctx context.Context, _ *apix.NoBody) (measurement, error) {
				return measurement{Value: math.NaN()}, nil
			})

			resp := httptest.NewRecorder()
			r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/measurement", nil))

			if resp.Code != http.StatusInternalServerError {
				t.Errorf("status = %d, want 500", resp.Code)
			}
			if ct := resp.Header().Get("Content-Type"); ct != tc.contentType {
				t.Errorf("Content-Type = %q, want %q", ct, tc.contentType)
			}
			if got := strings.TrimSpace(resp.Body.String()); got != tc.body {
				t.Errorf("body = %s, want %s", got, tc.body)
			}
			assertOneLine(t, logs, map[string]any{
				"level": "ERROR", "msg": "error answered with a generic 500", "reason": "no HTTP status",
				"error": "json: unsupported value: NaN",
			})
		})
	}
}

// TestMuxAdapterSuccessBodyIsUnchanged: encoding first keeps the bytes the
// adapter has always sent, json.Encoder's trailing newline included.
func TestMuxAdapterSuccessBodyIsUnchanged(t *testing.T) {
	apix.ResetRegistry()
	r := mux.NewRouter()
	adapter := muxadapter.New(r)
	muxadapter.Post(adapter, "/items", func(ctx context.Context, req *createItemRequest) (createItemResponse, error) {
		return createItemResponse{ID: "<1>"}, nil
	})

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/items", strings.NewReader(`{"name": "a"}`)))

	if resp.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", resp.Code)
	}
	if ct := resp.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if got, want := resp.Body.String(), "{\"id\":\"\\u003c1\\u003e\"}\n"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

// TestMuxAdapterLogsAFailedResponseWriteOnce: a client that leaves mid-body
// gives one Warn line, and no error response follows the status.
func TestMuxAdapterLogsAFailedResponseWriteOnce(t *testing.T) {
	for _, problem := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "problem"}[problem], func(t *testing.T) {
			logs := captureLogs(t)
			apix.ResetRegistry()
			r := mux.NewRouter()
			adapter := muxadapter.New(r, muxadapter.Options{UseProblemDetails: problem})
			muxadapter.Post(adapter, "/items", func(ctx context.Context, req *createItemRequest) (createItemResponse, error) {
				return createItemResponse{ID: "1"}, nil
			})

			w := &brokenPipeWriter{}
			req := httptest.NewRequest(http.MethodPost, "/items", strings.NewReader(`{"name": "a"}`))
			req.Header.Set("X-Request-ID", "req-1")
			r.ServeHTTP(w, req)

			if len(w.statuses) != 1 || w.statuses[0] != http.StatusCreated {
				t.Errorf("WriteHeader calls = %v, want [201]", w.statuses)
			}
			if w.writes != 1 {
				t.Errorf("writes = %d, want 1", w.writes)
			}
			assertOneLine(t, logs, map[string]any{
				"level": "WARN", "msg": "response write failed", "method": "POST", "path": "/items",
				"request_id": "req-1", "error": errBrokenPipe.Error(),
			})
		})
	}
}

// TestMuxAdapterCustomEncoderErrors: an encoder error is answered only while
// nothing has been sent; after that it is logged, and the response is left as
// the encoder wrote it.
func TestMuxAdapterCustomEncoderErrors(t *testing.T) {
	encodeErr := errors.New("encoder: field 3 failed")
	cases := []struct {
		name    string
		encoder muxadapter.ResponseEncoder
		status  int
		body    string
		log     map[string]any
	}{
		{
			name: "before writing",
			encoder: func(ctx context.Context, w http.ResponseWriter, r *http.Request, status int, payload any, ref *apix.RouteRef) error {
				return encodeErr
			},
			status: http.StatusInternalServerError, body: "Internal Server Error\n",
			log: map[string]any{"level": "ERROR", "msg": "error answered with a generic 500", "error": encodeErr.Error()},
		},
		{
			name: "after an informational status",
			encoder: func(ctx context.Context, w http.ResponseWriter, r *http.Request, status int, payload any, ref *apix.RouteRef) error {
				w.WriteHeader(http.StatusEarlyHints)
				return encodeErr
			},
			status: http.StatusInternalServerError, body: "Internal Server Error\n",
			log: map[string]any{"level": "ERROR", "msg": "error answered with a generic 500", "error": encodeErr.Error()},
		},
		{
			name: "after writing",
			encoder: func(ctx context.Context, w http.ResponseWriter, r *http.Request, status int, payload any, ref *apix.RouteRef) error {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"id":`))
				return encodeErr
			},
			status: http.StatusCreated, body: `{"id":`,
			log: map[string]any{"level": "ERROR", "msg": "response failed after it was committed", "method": "POST", "path": "/items", "error": encodeErr.Error()},
		},
		{
			name: "after flushing",
			encoder: func(ctx context.Context, w http.ResponseWriter, r *http.Request, status int, payload any, ref *apix.RouteRef) error {
				w.WriteHeader(status)
				w.(http.Flusher).Flush()
				return encodeErr
			},
			status: http.StatusCreated, body: "",
			log: map[string]any{"level": "ERROR", "msg": "response failed after it was committed", "error": encodeErr.Error()},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureLogs(t)
			apix.ResetRegistry()
			r := mux.NewRouter()
			adapter := muxadapter.New(r, muxadapter.Options{ResponseEncoder: tc.encoder})
			muxadapter.Post(adapter, "/items", func(ctx context.Context, req *createItemRequest) (createItemResponse, error) {
				return createItemResponse{ID: "1"}, nil
			})

			srv := httptest.NewServer(r)
			defer srv.Close()
			resp, err := http.Post(srv.URL+"/items", "application/json", strings.NewReader(`{"name": "a"}`))
			if err != nil {
				t.Fatalf("POST: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()
			var body bytes.Buffer
			_, _ = body.ReadFrom(resp.Body)

			if resp.StatusCode != tc.status {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.status)
			}
			if body.String() != tc.body {
				t.Errorf("body = %q, want %q", body.String(), tc.body)
			}
			assertOneLine(t, logs, tc.log)
		})
	}
}
