package chi_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apix "github.com/Infra-Forge/infra-apix"
	chiadapter "github.com/Infra-Forge/infra-apix/chi"
	"github.com/go-chi/chi/v5"
)

// TestChiAdapterErrorResponsesKeepInternalsPrivate: a decode failure is told in
// JSON terms, never by Go type or package names, and an error without an HTTP
// status becomes a generic 500, in both response formats.
func TestChiAdapterErrorResponsesKeepInternalsPrivate(t *testing.T) {
	internal := errors.New(`pq: relation "items" does not exist`)
	cases := []struct {
		name        string
		problem     bool
		validator   chiadapter.Validator
		body        string
		handlerErr  error
		status      int
		contentType string
		response    string
	}{
		{
			name: "decode error", body: `["widget"]`,
			status: http.StatusBadRequest, contentType: "text/plain; charset=utf-8",
			response: `request body must be an object`,
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
			status: http.StatusInternalServerError, contentType: "text/plain; charset=utf-8",
			response: "Internal Server Error",
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
			r := chi.NewRouter()
			adapter := chiadapter.New(r, chiadapter.Options{Validator: tc.validator, UseProblemDetails: tc.problem})
			chiadapter.Post(adapter, "/items", func(ctx context.Context, req *createItemRequest) (createItemResponse, error) {
				return createItemResponse{ID: "1"}, tc.handlerErr
			})

			resp := httptest.NewRecorder()
			r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/items", strings.NewReader(tc.body)))

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
