package runtime_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

// TestConcurrentBuildsDoNotShareSecuritySchemes: each pooled builder gets its
// own copy of the configured schemes, so a CustomizeBuilder that adds to them
// does not race with builders created at the same time (run with -race).
func TestConcurrentBuildsDoNotShareSecuritySchemes(t *testing.T) {
	registerSecuredRoute(t, "BearerAuth", "ApiKey")
	schemes := openapi3.SecuritySchemes{"BearerAuth": {Value: openapi3.NewJWTSecurityScheme()}}
	mux := specMux(t, runtime.Config{SecuritySchemes: schemes, CustomizeBuilder: addAPIKey})

	start := make(chan struct{})
	codes := make(chan int, 8)
	var wg sync.WaitGroup
	for range cap(codes) {
		wg.Go(func() {
			<-start
			codes <- getSpec(mux).Code
		})
	}
	close(start)
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != http.StatusOK {
			t.Errorf("status = %d, want 200", code)
		}
	}
}
