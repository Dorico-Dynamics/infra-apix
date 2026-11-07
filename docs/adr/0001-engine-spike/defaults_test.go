package spike

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	apix "github.com/Infra-Forge/infra-apix"
	apixchi "github.com/Infra-Forge/infra-apix/chi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/go-chi/chi/v5"
)

// huma's own error constructors, saved before the spike replaces them.
var (
	humaDefaultNewError            = huma.NewError
	humaDefaultNewErrorWithContext = huma.NewErrorWithContext
)

// TestDefaults records what each engine does with no configuration, for the
// ADR: what a service gets before it adds anything. SPIKE_DEFAULTS names a
// file to write the observations to.
func TestDefaults(t *testing.T) {
	var out strings.Builder
	get := func(srv *httptest.Server, method, path, contentType, body string) (int, http.Header, string) {
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, resp.Header, strings.TrimSpace(string(b))
	}
	note := func(engine, what string, status int, h http.Header, body string) {
		fmt.Fprintf(&out, "%s | %s | %d %s | %s\n", engine, what, status, h.Get("Content-Type"), body)
	}
	type in struct {
		Body struct {
			Name string `json:"name" minLength:"3" validate:"required,min=3"`
		}
	}
	type body struct {
		Name string `json:"name" validate:"required,min=3"`
	}
	failing := errors.New("dial tcp 192.0.2.7:5432: connection refused")

	// huma with DefaultConfig and its own error constructors.
	huma.NewError, huma.NewErrorWithContext = humaDefaultNewError, humaDefaultNewErrorWithContext
	defer func() {
		huma.NewError = func(status int, msg string, errs ...error) huma.StatusError { return humaProblem(status, msg, errs) }
		huma.NewErrorWithContext = func(ctx huma.Context, status int, msg string, errs ...error) huma.StatusError {
			if op := ctx.Operation(); op != nil && op.Metadata[errorFormatKey] == errorFormatRFC6749 {
				return humaOAuthError(status, errs)
			}
			return humaProblem(status, msg, errs)
		}
	}()
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Defaults", "1.0.0"))
	huma.Post(api, "/things", func(_ context.Context, _ *in) (*struct{ Body body }, error) {
		return &struct{ Body body }{Body: body{Name: "ok"}}, nil
	})
	huma.Get(api, "/fail", func(_ context.Context, _ *struct{}) (*struct{ Body body }, error) {
		return nil, failing
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	s, h, b := get(srv, http.MethodPost, "/things", "application/json", `{"name":"ok-name"}`)
	note("huma", "success body and Link header ("+h.Get("Link")+")", s, h, b)
	s, h, b = get(srv, http.MethodPost, "/things", "application/json", `{"name":"no"}`)
	note("huma", "validation failure", s, h, b)
	s, h, b = get(srv, http.MethodPost, "/things", "application/json", `{"name": "secret-value", `)
	note("huma", "malformed JSON", s, h, b)
	s, h, b = get(srv, http.MethodGet, "/fail", "", "")
	note("huma", "handler returns a plain error", s, h, b)
	s, h, _ = get(srv, http.MethodGet, "/docs", "", "")
	note("huma", "GET /docs", s, h, "(docs page)")
	s, h, _ = get(srv, http.MethodGet, "/openapi.json", "", "")
	note("huma", "GET /openapi.json", s, h, "(document)")

	// apix v1 on chi with no options.
	apix.ResetRegistry()
	r := chi.NewRouter()
	a := apixchi.New(r)
	apixchi.Post(a, "/things", func(_ context.Context, req *body) (body, error) { return *req, nil })
	apixchi.Get(a, "/fail", func(_ context.Context, _ *apix.NoBody) (body, error) { return body{}, failing })
	apixchi.Get(a, "/missing", func(_ context.Context, _ *apix.NoBody) (body, error) { return body{}, apix.NotFound("thing not found") })
	asrv := httptest.NewServer(r)
	defer asrv.Close()
	s, h, b = get(asrv, http.MethodPost, "/things", "application/json", `{"name":"no"}`)
	note("apix", "validate:\"min=3\" violated, no Validator set", s, h, b)
	s, h, b = get(asrv, http.MethodPost, "/things", "application/json", `{"name": "secret-value", `)
	note("apix", "malformed JSON", s, h, b)
	s, h, b = get(asrv, http.MethodGet, "/fail", "", "")
	note("apix", "handler returns a plain error", s, h, b)
	s, h, b = get(asrv, http.MethodGet, "/missing", "", "")
	note("apix", "handler returns apix.NotFound", s, h, b)

	t.Log("\n" + out.String())
	if path := os.Getenv("SPIKE_DEFAULTS"); path != "" {
		if err := os.WriteFile(path, []byte(out.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
