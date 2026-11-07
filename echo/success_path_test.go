package echo_test

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
	echoadapter "github.com/Infra-Forge/infra-apix/echo"
	"github.com/labstack/echo/v4"
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

// TestEchoAdapterAnswersAPayloadItCannotEncodeWith500: Echo's serializer
// encodes before it writes, so the encoding error reaches Echo's error
// handler while the response can still be a 500.
func TestEchoAdapterAnswersAPayloadItCannotEncodeWith500(t *testing.T) {
	t.Run("Echo's handler", func(t *testing.T) {
		logs := captureLogs(t)
		apix.ResetRegistry()
		e := echo.New()
		adapter := echoadapter.New(e)
		echoadapter.Get(adapter, "/measurement", func(ctx context.Context, _ *apix.NoBody) (measurement, error) {
			return measurement{Value: math.NaN()}, nil
		})

		resp := httptest.NewRecorder()
		e.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/measurement", nil))

		want := `{"message":"Internal Server Error"}`
		if got := strings.TrimSpace(resp.Body.String()); resp.Code != http.StatusInternalServerError || got != want {
			t.Errorf("response = %d %s, want 500 %s", resp.Code, got, want)
		}
		// An error without a status is Echo's to answer and to log.
		if lines := logLines(t, logs); len(lines) != 0 {
			t.Errorf("want no apix log line, got %s", logs.String())
		}
	})

	t.Run("ProblemDetailsErrorHandler", func(t *testing.T) {
		logs := captureLogs(t)
		apix.ResetRegistry()
		e := echo.New()
		e.HTTPErrorHandler = echoadapter.ProblemDetailsErrorHandler(nil)
		adapter := echoadapter.New(e, echoadapter.Options{UseProblemDetails: true})
		echoadapter.Get(adapter, "/measurement", func(ctx context.Context, _ *apix.NoBody) (measurement, error) {
			return measurement{Value: math.NaN()}, nil
		})

		resp := httptest.NewRecorder()
		e.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/measurement", nil))

		want := `{"status":500,"title":"Internal Server Error","type":"about:blank"}`
		if got := strings.TrimSpace(resp.Body.String()); resp.Code != http.StatusInternalServerError || got != want {
			t.Errorf("response = %d %s, want 500 %s", resp.Code, got, want)
		}
		assertOneLine(t, logs, map[string]any{
			"level": "ERROR", "msg": "error answered with a generic 500", "reason": "no HTTP status",
			"error": "json: unsupported value: NaN",
		})
	})
}

// TestEchoAdapterLogsAFailedResponseWriteOnce: a client that leaves mid-body
// gives one Warn line, and Echo's error handler is not asked to answer a
// committed response.
func TestEchoAdapterLogsAFailedResponseWriteOnce(t *testing.T) {
	logs := captureLogs(t)
	apix.ResetRegistry()
	e := echo.New()
	var handled []error
	e.HTTPErrorHandler = func(err error, c echo.Context) { handled = append(handled, err) }
	adapter := echoadapter.New(e)
	echoadapter.Post(adapter, "/items", func(ctx context.Context, req *createItemRequest) (createItemResponse, error) {
		return createItemResponse{ID: "1"}, nil
	})

	w := &brokenPipeWriter{}
	req := httptest.NewRequest(http.MethodPost, "/items", strings.NewReader(`{"name": "a"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "req-1")
	e.ServeHTTP(w, req)

	if len(w.statuses) != 1 || w.statuses[0] != http.StatusCreated {
		t.Errorf("WriteHeader calls = %v, want [201]", w.statuses)
	}
	if w.writes != 1 {
		t.Errorf("writes = %d, want 1", w.writes)
	}
	if len(handled) != 0 {
		t.Errorf("HTTPErrorHandler called with %v, want no call", handled)
	}
	assertOneLine(t, logs, map[string]any{
		"level": "WARN", "msg": "response write failed", "method": "POST", "path": "/items",
		"request_id": "req-1", "error": errBrokenPipe.Error(),
	})
}

// TestProblemDetailsErrorHandlerLeavesACommittedResponseAlone: an error that
// arrives after the response has started is logged, not answered, unless the
// fallback owns it.
func TestProblemDetailsErrorHandlerLeavesACommittedResponseAlone(t *testing.T) {
	encodeErr := errors.New("encoder: field 3 failed")
	partialEncoder := func(ctx context.Context, c echo.Context, status int, payload any, ref *apix.RouteRef) error {
		c.Response().Header().Set("Content-Type", "application/json")
		c.Response().WriteHeader(status)
		_, _ = c.Response().Write([]byte(`{"id":`))
		return encodeErr
	}
	cases := []struct {
		name     string
		fallback bool
		route    func(e *echo.Echo)
		log      map[string]any
	}{
		{
			name: "custom encoder without fallback",
			route: func(e *echo.Echo) {
				adapter := echoadapter.New(e, echoadapter.Options{UseProblemDetails: true, ResponseEncoder: partialEncoder})
				echoadapter.Post(adapter, "/items", func(ctx context.Context, req *createItemRequest) (createItemResponse, error) {
					return createItemResponse{ID: "1"}, nil
				})
			},
			log: map[string]any{"level": "ERROR", "msg": "response failed after it was committed", "method": "POST", "path": "/items", "error": encodeErr.Error()},
		},
		{
			name: "problem from a route that wrote",
			route: func(e *echo.Echo) {
				e.POST("/items", func(c echo.Context) error {
					c.Response().WriteHeader(http.StatusCreated)
					_, _ = c.Response().Write([]byte(`{"id":`))
					return &apix.ProblemDetails{Status: http.StatusConflict, Title: "Conflict"}
				})
			},
			// slog renders the problem's JSON, not its Error text.
			log: map[string]any{"level": "ERROR", "msg": "response failed after it was committed", "method": "POST", "path": "/items"},
		},
		{
			name: "custom encoder with Echo's handler as fallback", fallback: true,
			route: func(e *echo.Echo) {
				adapter := echoadapter.New(e, echoadapter.Options{UseProblemDetails: true, ResponseEncoder: partialEncoder})
				echoadapter.Post(adapter, "/items", func(ctx context.Context, req *createItemRequest) (createItemResponse, error) {
					return createItemResponse{ID: "1"}, nil
				})
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureLogs(t)
			apix.ResetRegistry()
			e := echo.New()
			var fallback echo.HTTPErrorHandler
			if tc.fallback {
				fallback = e.DefaultHTTPErrorHandler
			}
			e.HTTPErrorHandler = echoadapter.ProblemDetailsErrorHandler(fallback)
			tc.route(e)

			req := httptest.NewRequest(http.MethodPost, "/items", strings.NewReader(`{"name": "a"}`))
			req.Header.Set("Content-Type", "application/json")
			resp := httptest.NewRecorder()
			e.ServeHTTP(resp, req)

			if resp.Code != http.StatusCreated {
				t.Errorf("status = %d, want 201", resp.Code)
			}
			if got := resp.Body.String(); got != `{"id":` {
				t.Errorf("body = %q, want the encoder's partial body only", got)
			}
			if tc.log == nil {
				// Echo's handler returns without a word on a committed response.
				if lines := logLines(t, logs); len(lines) != 0 {
					t.Errorf("want no apix log line, got %s", logs.String())
				}
				return
			}
			assertOneLine(t, logs, tc.log)
		})
	}
}
