package fiber_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apix "github.com/Infra-Forge/infra-apix"
	fiberadapter "github.com/Infra-Forge/infra-apix/fiber"
	"github.com/gofiber/fiber/v3"
)

// TestFiberAdapterErrorResponsesKeepInternalsPrivate: a decode failure is told
// in JSON terms, never by Go type or package names, and an error without a
// 4xx or 5xx status becomes a generic 500, in both response formats.
func TestFiberAdapterErrorResponsesKeepInternalsPrivate(t *testing.T) {
	internal := errors.New(`pq: relation "items" does not exist`)
	cases := []struct {
		name        string
		problem     bool
		body        string
		handlerErr  error
		status      int
		contentType string
		response    string
	}{
		{
			name: "decode error", body: `["widget"]`,
			status: http.StatusBadRequest, contentType: "application/json; charset=utf-8",
			response: `{"error":"request body must be an object"}`,
		},
		{
			name: "decode error as problem", problem: true, body: `["widget"]`,
			status: http.StatusBadRequest, contentType: "application/problem+json",
			response: `{"detail":"request body must be an object","status":400,"title":"Bad Request","type":"about:blank"}`,
		},
		{
			name: "error without status", body: `{"name": "a"}`, handlerErr: internal,
			status: http.StatusInternalServerError, contentType: "application/json; charset=utf-8",
			response: `{"error":"Internal Server Error"}`,
		},
		{
			name: "error without status as problem", problem: true, body: `{"name": "a"}`, handlerErr: internal,
			status: http.StatusInternalServerError, contentType: "application/problem+json",
			response: `{"status":500,"title":"Internal Server Error","type":"about:blank"}`,
		},
		{
			name: "error with status 0", body: `{"name": "a"}`, handlerErr: apix.WithStatus(0, "custom message"),
			status: http.StatusInternalServerError, contentType: "application/json; charset=utf-8",
			response: `{"error":"Internal Server Error"}`,
		},
		{
			name: "problem without status", problem: true, body: `{"name": "a"}`, handlerErr: &apix.ProblemDetails{Title: "Missing status"},
			status: http.StatusInternalServerError, contentType: "application/problem+json",
			response: `{"status":500,"title":"Internal Server Error","type":"about:blank"}`,
		},
		{
			name: "error with success status", body: `{"name": "a"}`, handlerErr: apix.WithStatus(http.StatusOK, "custom message"),
			status: http.StatusInternalServerError, contentType: "application/json; charset=utf-8",
			response: `{"error":"Internal Server Error"}`,
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
			app := fiber.New()
			adapter := fiberadapter.New(app, fiberadapter.Options{UseProblemDetails: tc.problem})
			fiberadapter.Post(adapter, "/items", func(ctx context.Context, req *createItemRequest) (createItemResponse, error) {
				return createItemResponse{ID: "1"}, tc.handlerErr
			})

			resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/items", strings.NewReader(tc.body)))
			if err != nil {
				t.Fatalf("test request failed: %v", err)
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}

			if resp.StatusCode != tc.status {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.status)
			}
			if ct := resp.Header.Get("Content-Type"); ct != tc.contentType {
				t.Errorf("Content-Type = %q, want %q", ct, tc.contentType)
			}
			if got := strings.TrimSpace(string(body)); got != tc.response {
				t.Errorf("body = %s, want %s", got, tc.response)
			}
		})
	}
}

// TestFiberAdapterLogsTheRequestOfAnErrorItAnswersWith500: the error line
// names the request's method, path, X-Request-ID and X-Correlation-ID.
func TestFiberAdapterLogsTheRequestOfAnErrorItAnswersWith500(t *testing.T) {
	var logs bytes.Buffer
	apix.SetLogger(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelError})))
	t.Cleanup(func() { apix.SetLogger(nil) })

	apix.ResetRegistry()
	app := fiber.New()
	adapter := fiberadapter.New(app)
	fiberadapter.Post(adapter, "/items", func(ctx context.Context, req *createItemRequest) (createItemResponse, error) {
		return createItemResponse{}, errors.New(`pq: relation "items" does not exist`)
	})

	req := httptest.NewRequest(http.MethodPost, "/items", strings.NewReader(`{"name": "a"}`))
	req.Header.Set("X-Request-ID", "req-1")
	req.Header.Set("X-Correlation-ID", "corr-1")
	if _, err := app.Test(req); err != nil {
		t.Fatalf("test request failed: %v", err)
	}

	var line map[string]any
	if err := json.Unmarshal(logs.Bytes(), &line); err != nil {
		t.Fatalf("want one JSON line, got %q: %v", logs.String(), err)
	}
	for key, want := range map[string]any{
		"msg": "error answered with a generic 500", "reason": "no HTTP status", "method": "POST", "path": "/items",
		"request_id": "req-1", "correlation_id": "corr-1", "error": `pq: relation "items" does not exist`,
	} {
		if line[key] != want {
			t.Errorf("%s = %v, want %v", key, line[key], want)
		}
	}
}
