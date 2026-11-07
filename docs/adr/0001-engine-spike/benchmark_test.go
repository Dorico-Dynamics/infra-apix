package spike

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// The benchmarks time one request through each engine, in process: routing,
// binding, validation, the handler and encoding. huma runs twice, on chi as
// the apix engine does and on ServeMux, so the router's share shows.

type benchHandler struct {
	name string
	h    http.Handler
}

func benchHandlers(b *testing.B) []benchHandler {
	b.Helper()
	r := chi.NewRouter()
	registerHumaRoutes(humachi.New(r, humaConfig()), newStore())
	return []benchHandler{
		{"apix-chi", newApixEngine(b).handler},
		{"huma-chi", r},
		{"huma-servemux", newHumaEngine(b).handler},
	}
}

func benchRequest(b *testing.B, h http.Handler, newRequest func() *http.Request, want int) {
	b.Helper()
	b.ReportAllocs()
	for b.Loop() {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, newRequest())
		if rec.Code != want {
			b.Fatalf("status %d, want %d: %s", rec.Code, want, rec.Body)
		}
	}
}

// BenchmarkUpdateProgress: a UUID path parameter, an If-Match header, a JSON
// body with a bounded number, and an ETag response header. If-Match: * keeps
// every request valid while the phase's version climbs.
func BenchmarkUpdateProgress(b *testing.B) {
	for _, e := range benchHandlers(b) {
		b.Run(e.name, func(b *testing.B) {
			benchRequest(b, e.h, func() *http.Request {
				req := httptest.NewRequest(http.MethodPost, progressPath, strings.NewReader(`{"percent": 55}`))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("If-Match", "*")
				return req
			}, http.StatusOK)
		})
	}
}

// BenchmarkListGrants: a bounded query parameter and a JSON page.
func BenchmarkListGrants(b *testing.B) {
	for _, e := range benchHandlers(b) {
		b.Run(e.name, func(b *testing.B) {
			benchRequest(b, e.h, func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/grants?limit=2", nil)
			}, http.StatusOK)
		})
	}
}
