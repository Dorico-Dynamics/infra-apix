package echo_test

import (
	"context"
	"errors"
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
// a generic 500.
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
