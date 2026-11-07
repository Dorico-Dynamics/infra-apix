package gin_test

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
	ginadapter "github.com/Infra-Forge/infra-apix/gin"
	"github.com/gin-gonic/gin"
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

// TestGinAdapterAnswersAPayloadItCannotEncodeWith500: the body is marshalled
// before the status is written, so a value JSON cannot represent becomes the
// logged generic 500. c.JSON sent the status with an empty body, and the
// error stayed in c.Errors.
func TestGinAdapterAnswersAPayloadItCannotEncodeWith500(t *testing.T) {
	cases := []struct {
		name        string
		problem     bool
		contentType string
		body        string
	}{
		{name: "text", contentType: "application/json; charset=utf-8", body: `{"error":"Internal Server Error"}`},
		{name: "problem", problem: true, contentType: "application/problem+json", body: `{"status":500,"title":"Internal Server Error","type":"about:blank"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureLogs(t)
			apix.ResetRegistry()
			e := gin.New()
			adapter := ginadapter.New(e, ginadapter.Options{UseProblemDetails: tc.problem})
			ginadapter.Get(adapter, "/measurement", func(ctx context.Context, _ *apix.NoBody) (measurement, error) {
				return measurement{Value: math.NaN()}, nil
			})

			resp := httptest.NewRecorder()
			e.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/measurement", nil))

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

// TestGinAdapterSuccessBodyIsUnchanged: marshalling first keeps the bytes and
// Content-Type of c.JSON, including a Content-Type that middleware set.
func TestGinAdapterSuccessBodyIsUnchanged(t *testing.T) {
	for _, preset := range []string{"", "application/vnd.api+json"} {
		t.Run("preset "+preset, func(t *testing.T) {
			apix.ResetRegistry()
			e := gin.New()
			if preset != "" {
				e.Use(func(c *gin.Context) { c.Header("Content-Type", preset) })
			}
			adapter := ginadapter.New(e)
			ginadapter.Post(adapter, "/items", func(ctx context.Context, req *createItemRequest) (createItemResponse, error) {
				return createItemResponse{ID: "<1>"}, nil
			})

			resp := httptest.NewRecorder()
			e.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/items", strings.NewReader(`{"name": "a"}`)))

			want := httptest.NewRecorder()
			ref, _ := gin.CreateTestContext(want)
			if preset != "" {
				ref.Header("Content-Type", preset)
			}
			ref.JSON(http.StatusCreated, createItemResponse{ID: "<1>"})

			if resp.Code != http.StatusCreated {
				t.Errorf("status = %d, want 201", resp.Code)
			}
			if got, wantCT := resp.Header().Get("Content-Type"), want.Header().Get("Content-Type"); got != wantCT {
				t.Errorf("Content-Type = %q, want c.JSON's %q", got, wantCT)
			}
			if got := resp.Body.String(); got != want.Body.String() {
				t.Errorf("body = %q, want c.JSON's %q", got, want.Body.String())
			}
		})
	}
}

// TestGinAdapterLogsAFailedResponseWriteOnce: a client that leaves mid-body
// gives one Warn line, and no error response follows the status.
func TestGinAdapterLogsAFailedResponseWriteOnce(t *testing.T) {
	for _, problem := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "problem"}[problem], func(t *testing.T) {
			logs := captureLogs(t)
			apix.ResetRegistry()
			e := gin.New()
			adapter := ginadapter.New(e, ginadapter.Options{UseProblemDetails: problem})
			ginadapter.Post(adapter, "/items", func(ctx context.Context, req *createItemRequest) (createItemResponse, error) {
				return createItemResponse{ID: "1"}, nil
			})

			w := &brokenPipeWriter{}
			req := httptest.NewRequest(http.MethodPost, "/items", strings.NewReader(`{"name": "a"}`))
			req.Header.Set("X-Request-ID", "req-1")
			e.ServeHTTP(w, req)

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

// TestGinAdapterCustomEncoderErrors: an encoder error is answered only while
// nothing has been written; after that it is logged, and the response is left
// as the encoder wrote it.
func TestGinAdapterCustomEncoderErrors(t *testing.T) {
	encodeErr := errors.New("encoder: field 3 failed")
	cases := []struct {
		name    string
		encoder ginadapter.ResponseEncoder
		status  int
		body    string
		log     map[string]any
	}{
		{
			name: "before writing",
			encoder: func(ctx context.Context, c *gin.Context, status int, payload any, ref *apix.RouteRef) error {
				c.Status(status)
				return encodeErr
			},
			status: http.StatusInternalServerError, body: `{"error":"Internal Server Error"}`,
			log: map[string]any{"level": "ERROR", "msg": "error answered with a generic 500", "error": encodeErr.Error()},
		},
		{
			name: "after writing",
			encoder: func(ctx context.Context, c *gin.Context, status int, payload any, ref *apix.RouteRef) error {
				c.Status(status)
				_, _ = c.Writer.Write([]byte(`{"id":`))
				return encodeErr
			},
			status: http.StatusCreated, body: `{"id":`,
			log: map[string]any{"level": "ERROR", "msg": "response failed after it was committed", "method": "POST", "path": "/items", "error": encodeErr.Error()},
		},
		{
			name: "after flushing",
			encoder: func(ctx context.Context, c *gin.Context, status int, payload any, ref *apix.RouteRef) error {
				c.Status(status)
				c.Writer.Flush()
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
			e := gin.New()
			adapter := ginadapter.New(e, ginadapter.Options{ResponseEncoder: tc.encoder})
			ginadapter.Post(adapter, "/items", func(ctx context.Context, req *createItemRequest) (createItemResponse, error) {
				return createItemResponse{ID: "1"}, nil
			})

			resp := httptest.NewRecorder()
			e.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/items", strings.NewReader(`{"name": "a"}`)))

			if resp.Code != tc.status {
				t.Errorf("status = %d, want %d", resp.Code, tc.status)
			}
			if got := resp.Body.String(); got != tc.body {
				t.Errorf("body = %q, want %q", got, tc.body)
			}
			assertOneLine(t, logs, tc.log)
		})
	}
}
