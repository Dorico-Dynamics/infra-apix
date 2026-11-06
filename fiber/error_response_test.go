package fiber_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apix "github.com/Infra-Forge/infra-apix"
	fiberadapter "github.com/Infra-Forge/infra-apix/fiber"
	"github.com/gofiber/fiber/v3"
)

// TestFiberAdapterErrorResponsesKeepInternalsPrivate: a decode failure is told
// in JSON terms, never by Go type or package names, and an error without an
// HTTP status becomes a generic 500, in both response formats.
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
