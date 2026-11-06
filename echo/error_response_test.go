package echo_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apix "github.com/Infra-Forge/infra-apix"
	echoadapter "github.com/Infra-Forge/infra-apix/echo"
	"github.com/labstack/echo/v4"
)

// TestEchoAdapterErrorResponsesKeepInternalsPrivate: a decode failure is told in
// JSON terms, never by Go type or package names, in both response formats. An
// error without an HTTP status is left to Echo's error handler, which answers
// a generic 500. An error with a status outside 4xx and 5xx gets a generic 500
// from the adapter.
func TestEchoAdapterErrorResponsesKeepInternalsPrivate(t *testing.T) {
	internal := errors.New(`pq: relation "items" does not exist`)
	cases := []struct {
		name        string
		problem     bool
		validator   echo.Validator
		body        string
		handlerErr  error
		status      int
		contentType string
		response    string
	}{
		{
			name: "decode error", body: `["widget"]`,
			status: http.StatusBadRequest, contentType: echo.MIMEApplicationJSON,
			response: `{"message":"request body must be an object"}`,
		},
		{
			name: "decode error as problem", problem: true, body: `["widget"]`,
			status: http.StatusBadRequest, contentType: "application/problem+json",
			response: `{"detail":"request body must be an object","status":400,"title":"Bad Request","type":"about:blank"}`,
		},
		{
			name: "validation error as problem", problem: true, validator: &mockValidator{}, body: `{"name": "a"}`,
			status: http.StatusUnprocessableEntity, contentType: "application/problem+json",
			response: `{"detail":"invalid","status":422,"title":"Unprocessable Entity","type":"about:blank"}`,
		},
		{
			name: "error without status", body: `{"name": "a"}`, handlerErr: internal,
			status: http.StatusInternalServerError, contentType: echo.MIMEApplicationJSON,
			response: `{"message":"Internal Server Error"}`,
		},
		{
			name: "error without status as problem", problem: true, body: `{"name": "a"}`, handlerErr: internal,
			status: http.StatusInternalServerError, contentType: echo.MIMEApplicationJSON,
			response: `{"message":"Internal Server Error"}`,
		},
		{
			name: "error with status 0", body: `{"name": "a"}`, handlerErr: apix.WithStatus(0, "custom message"),
			status: http.StatusInternalServerError, contentType: echo.MIMEApplicationJSON,
			response: `{"message":"Internal Server Error"}`,
		},
		{
			name: "problem without status", problem: true, body: `{"name": "a"}`, handlerErr: &apix.ProblemDetails{Title: "Missing status"},
			status: http.StatusInternalServerError, contentType: "application/problem+json",
			response: `{"status":500,"title":"Internal Server Error","type":"about:blank"}`,
		},
		{
			name: "error with success status", body: `{"name": "a"}`, handlerErr: apix.WithStatus(http.StatusOK, "custom message"),
			status: http.StatusInternalServerError, contentType: echo.MIMEApplicationJSON,
			response: `{"message":"Internal Server Error"}`,
		},
		{
			name: "error with not-modified status as problem", problem: true, body: `{"name": "a"}`, handlerErr: apix.WithStatus(http.StatusNotModified, "custom message"),
			status: http.StatusInternalServerError, contentType: "application/problem+json",
			response: `{"status":500,"title":"Internal Server Error","type":"about:blank"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			apix.ResetRegistry()
			e := echo.New()
			e.Validator = tc.validator
			if tc.problem {
				e.HTTPErrorHandler = echoadapter.ProblemDetailsErrorHandler(e.DefaultHTTPErrorHandler)
			}
			adapter := echoadapter.New(e, echoadapter.Options{UseProblemDetails: tc.problem})
			echoadapter.Post(adapter, "/items", func(ctx context.Context, req *createItemRequest) (createItemResponse, error) {
				return createItemResponse{ID: "1"}, tc.handlerErr
			})

			resp := httptest.NewRecorder()
			e.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/items", strings.NewReader(tc.body)))

			if resp.Code != tc.status {
				t.Errorf("status = %d, want %d", resp.Code, tc.status)
			}
			if ct := resp.Header().Get("Content-Type"); ct != tc.contentType {
				t.Errorf("Content-Type = %q, want %q", ct, tc.contentType)
			}
			if got := strings.TrimSpace(resp.Body.String()); got != tc.response {
				t.Errorf("body = %s, want %s", got, tc.response)
			}
		})
	}
}

// TestProblemDetailsErrorHandlerAnswers500ForAProblemWithoutStatus: a problem
// from a plain Echo route reaches the handler without passing the adapter, and
// WriteHeader(0) used to panic.
func TestProblemDetailsErrorHandlerAnswers500ForAProblemWithoutStatus(t *testing.T) {
	e := echo.New()
	e.HTTPErrorHandler = echoadapter.ProblemDetailsErrorHandler(nil)
	e.GET("/plain", func(c echo.Context) error {
		return &apix.ProblemDetails{Title: "Missing status"}
	})

	resp := httptest.NewRecorder()
	e.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/plain", nil))

	if resp.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.Code)
	}
	if ct := resp.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
	want := `{"status":500,"title":"Internal Server Error","type":"about:blank"}`
	if got := strings.TrimSpace(resp.Body.String()); got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}

// TestEchoAdapterLogsTheRequestOfAnErrorItAnswersWith500: the error line
// names the request's method, path, X-Request-ID and X-Correlation-ID.
// Echo's error handler answers an error without a status, so the adapter
// logs only an error whose status is outside 4xx and 5xx.
func TestEchoAdapterLogsTheRequestOfAnErrorItAnswersWith500(t *testing.T) {
	var logs bytes.Buffer
	apix.SetLogger(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelError})))
	t.Cleanup(func() { apix.SetLogger(nil) })

	apix.ResetRegistry()
	e := echo.New()
	adapter := echoadapter.New(e)
	echoadapter.Post(adapter, "/items", func(ctx context.Context, req *createItemRequest) (createItemResponse, error) {
		return createItemResponse{}, apix.WithStatus(0, "custom message")
	})

	req := httptest.NewRequest(http.MethodPost, "/items", strings.NewReader(`{"name": "a"}`))
	req.Header.Set("X-Request-ID", "req-1")
	req.Header.Set("X-Correlation-ID", "corr-1")
	e.ServeHTTP(httptest.NewRecorder(), req)

	var line map[string]any
	if err := json.Unmarshal(logs.Bytes(), &line); err != nil {
		t.Fatalf("want one JSON line, got %q: %v", logs.String(), err)
	}
	for key, want := range map[string]any{
		"msg": "error answered with a generic 500", "reason": "HTTP status outside 4xx and 5xx", "method": "POST", "path": "/items",
		"request_id": "req-1", "correlation_id": "corr-1", "error": "http 0: custom message",
	} {
		if line[key] != want {
			t.Errorf("%s = %v, want %v", key, line[key], want)
		}
	}
}
