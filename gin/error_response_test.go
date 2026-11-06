package gin_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apix "github.com/Infra-Forge/infra-apix"
	ginadapter "github.com/Infra-Forge/infra-apix/gin"
	"github.com/gin-gonic/gin"
)

// TestGinAdapterErrorResponsesKeepInternalsPrivate: a decode failure is told in
// JSON terms, never by Go type or package names, and an error without a 4xx
// or 5xx status becomes a generic 500, in both response formats.
func TestGinAdapterErrorResponsesKeepInternalsPrivate(t *testing.T) {
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
			e := gin.New()
			adapter := ginadapter.New(e, ginadapter.Options{UseProblemDetails: tc.problem})
			ginadapter.Post(adapter, "/items", func(ctx context.Context, req *createItemRequest) (createItemResponse, error) {
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
