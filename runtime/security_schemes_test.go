package runtime_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	apix "github.com/Infra-Forge/infra-apix"
	"github.com/Infra-Forge/infra-apix/openapi"
	"github.com/Infra-Forge/infra-apix/runtime"
	"github.com/getkin/kin-openapi/openapi3"
)

func registerSecuredRoute(t *testing.T, schemes ...string) {
	t.Helper()
	apix.ResetRegistry()
	t.Cleanup(apix.ResetRegistry)
	ref := &apix.RouteRef{Method: apix.MethodGet, Path: "/items", Responses: map[int]*apix.ResponseRef{http.StatusOK: {}}}
	for _, scheme := range schemes {
		apix.WithSecurity(scheme)(ref)
	}
	apix.RegisterRoute(ref)
}

func specMux(t *testing.T, cfg runtime.Config) *http.ServeMux {
	t.Helper()
	h, err := runtime.NewHandler(cfg)
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	mux := http.NewServeMux()
	h.RegisterHTTP(mux)
	return mux
}

func getSpec(mux *http.ServeMux) *httptest.ResponseRecorder {
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	return resp
}

// addAPIKey is a CustomizeBuilder that adds a scheme to the configured ones.
func addAPIKey(b *openapi.Builder) {
	b.SecuritySchemes["ApiKey"] = &openapi3.SecuritySchemeRef{
		Value: &openapi3.SecurityScheme{Type: "apiKey", In: "header", Name: "X-API-Key"},
	}
}

// TestSecuritySchemesReachTheDocument: Config.SecuritySchemes become the
// served document's components.securitySchemes. CustomizeBuilder sees them and
// can add more without changing the caller's map.
func TestSecuritySchemesReachTheDocument(t *testing.T) {
	registerSecuredRoute(t, "BearerAuth", "ApiKey")
	schemes := openapi3.SecuritySchemes{"BearerAuth": {Value: openapi3.NewJWTSecurityScheme()}}

	resp := getSpec(specMux(t, runtime.Config{Validate: true, SecuritySchemes: schemes, CustomizeBuilder: addAPIKey}))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", resp.Code, resp.Body)
	}
	var doc struct {
		Components struct {
			SecuritySchemes map[string]struct {
				Type         string `json:"type"`
				Scheme       string `json:"scheme"`
				BearerFormat string `json:"bearerFormat"`
				In           string `json:"in"`
				Name         string `json:"name"`
			} `json:"securitySchemes"`
		} `json:"components"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := doc.Components.SecuritySchemes
	if bearer := got["BearerAuth"]; bearer.Type != "http" || bearer.Scheme != "bearer" || bearer.BearerFormat != "JWT" {
		t.Errorf("BearerAuth = %+v, want http bearer JWT", bearer)
	}
	if key := got["ApiKey"]; key.Type != "apiKey" || key.In != "header" || key.Name != "X-API-Key" {
		t.Errorf("ApiKey = %+v, want the apiKey header CustomizeBuilder added", key)
	}
	if len(schemes) != 1 {
		t.Errorf("the configured map changed: %v", schemes)
	}
}

// TestUndeclaredSecuritySchemeAnswers500: with Validate off, a route naming a
// scheme nothing declares still fails the build.
func TestUndeclaredSecuritySchemeAnswers500(t *testing.T) {
	registerSecuredRoute(t, "BearerAuth")

	if resp := getSpec(specMux(t, runtime.Config{})); resp.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.Code)
	}
}

// customizeSchemes adds a scheme and edits a configured one, as a
// CustomizeBuilder may.
func customizeSchemes(b *openapi.Builder) {
	addAPIKey(b)
	b.SecuritySchemes["BearerAuth"].Value.Description += "edited once"
}

// TestConcurrentBuildsDoNotShareSecuritySchemes: each pooled builder gets its
// own deep copy of the configured schemes. A CustomizeBuilder that adds a
// scheme and edits a configured one runs once per builder, does not race with
// builders created at the same time (run with -race), and leaves the caller's
// schemes unchanged.
func TestConcurrentBuildsDoNotShareSecuritySchemes(t *testing.T) {
	registerSecuredRoute(t, "BearerAuth", "ApiKey")
	bearer := openapi3.NewJWTSecurityScheme()
	schemes := openapi3.SecuritySchemes{"BearerAuth": {Value: bearer}}
	mux := specMux(t, runtime.Config{SecuritySchemes: schemes, CustomizeBuilder: customizeSchemes})

	start := make(chan struct{})
	responses := make(chan *httptest.ResponseRecorder, 8)
	var wg sync.WaitGroup
	for range cap(responses) {
		wg.Go(func() {
			<-start
			responses <- getSpec(mux)
		})
	}
	close(start)
	wg.Wait()
	close(responses)
	for resp := range responses {
		var doc struct {
			Components struct {
				SecuritySchemes map[string]struct {
					Description string `json:"description"`
				} `json:"securitySchemes"`
			} `json:"components"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &doc); resp.Code != http.StatusOK || err != nil {
			t.Fatalf("status = %d, decode error %v, body: %s", resp.Code, err, resp.Body)
		}
		if got := doc.Components.SecuritySchemes["BearerAuth"].Description; got != "edited once" {
			t.Errorf("BearerAuth description = %q, want %q", got, "edited once")
		}
	}
	if len(schemes) != 1 || bearer.Description != "" {
		t.Errorf("the configured schemes changed: %d schemes, BearerAuth description %q", len(schemes), bearer.Description)
	}
}

// TestNewHandlerRejectsSecuritySchemesThatDoNotEncode: each builder's copy goes
// through JSON, so NewHandler fails for a scheme that cannot be encoded.
func TestNewHandlerRejectsSecuritySchemesThatDoNotEncode(t *testing.T) {
	scheme := openapi3.NewJWTSecurityScheme()
	scheme.Extensions = map[string]any{"x-hook": func() {}}

	_, err := runtime.NewHandler(runtime.Config{SecuritySchemes: openapi3.SecuritySchemes{"BearerAuth": {Value: scheme}}})
	if err == nil || !strings.Contains(err.Error(), "encode security schemes") {
		t.Fatalf("NewHandler error = %v, want an encode error", err)
	}
}
