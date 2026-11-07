package fiber_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apix "github.com/Infra-Forge/infra-apix"
	fiberadapter "github.com/Infra-Forge/infra-apix/fiber"
	"github.com/gofiber/fiber/v3"
)

type measurement struct {
	Value float64 `json:"value"`
}

// TestFiberAdapterAnswersAPayloadItCannotEncodeWith500: fiber buffers the
// response, so the error handler replaces the status and body the encoder
// set, and the client gets the logged generic 500.
func TestFiberAdapterAnswersAPayloadItCannotEncodeWith500(t *testing.T) {
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
			var logs bytes.Buffer
			apix.SetLogger(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn})))
			t.Cleanup(func() { apix.SetLogger(nil) })

			apix.ResetRegistry()
			app := fiber.New()
			adapter := fiberadapter.New(app, fiberadapter.Options{UseProblemDetails: tc.problem})
			fiberadapter.Get(adapter, "/measurement", func(ctx context.Context, _ *apix.NoBody) (measurement, error) {
				return measurement{Value: math.NaN()}, nil
			})

			resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/measurement", nil))
			if err != nil {
				t.Fatalf("test request failed: %v", err)
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}

			if resp.StatusCode != http.StatusInternalServerError {
				t.Errorf("status = %d, want 500", resp.StatusCode)
			}
			if ct := resp.Header.Get("Content-Type"); ct != tc.contentType {
				t.Errorf("Content-Type = %q, want %q", ct, tc.contentType)
			}
			if got := strings.TrimSpace(string(body)); got != tc.body {
				t.Errorf("body = %s, want %s", got, tc.body)
			}
			var line map[string]any
			if err := json.Unmarshal(logs.Bytes(), &line); err != nil {
				t.Fatalf("want one JSON line, got %q: %v", logs.String(), err)
			}
			for key, want := range map[string]any{
				"level": "ERROR", "msg": "error answered with a generic 500", "reason": "no HTTP status",
				"error": "json: unsupported value: NaN",
			} {
				if line[key] != want {
					t.Errorf("%s = %v, want %v", key, line[key], want)
				}
			}
		})
	}
}
