package runtime_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	apix "github.com/Infra-Forge/infra-apix"
	"github.com/Infra-Forge/infra-apix/runtime"
	"github.com/getkin/kin-openapi/openapi3"
	echo "github.com/labstack/echo/v4"
)

// buildCounter counts the builds that reach the plugins' OnSpecBuild hook.
type buildCounter struct {
	apix.BasePlugin
	builds atomic.Int32
}

func (p *buildCounter) OnSpecBuild(*openapi3.T) error {
	p.builds.Add(1)
	return nil
}

func countBuilds(t *testing.T) *buildCounter {
	t.Helper()
	apix.ResetPlugins()
	t.Cleanup(apix.ResetPlugins)
	counter := &buildCounter{BasePlugin: apix.BasePlugin{PluginName: "build-counter"}}
	apix.RegisterPlugin(counter)
	return counter
}

// serverPlugin adds a server to every document it builds.
type serverPlugin struct{ apix.BasePlugin }

func (*serverPlugin) OnSpecBuild(doc *openapi3.T) error {
	doc.AddServer(&openapi3.Server{URL: "https://plugin.example"})
	return nil
}

// bearerPlugin declares the BearerAuth scheme.
type bearerPlugin struct{ apix.BasePlugin }

func (*bearerPlugin) OnSpecBuild(doc *openapi3.T) error {
	if doc.Components.SecuritySchemes == nil {
		doc.Components.SecuritySchemes = openapi3.SecuritySchemes{}
	}
	doc.Components.SecuritySchemes["BearerAuth"] = &openapi3.SecuritySchemeRef{Value: openapi3.NewJWTSecurityScheme()}
	return nil
}

func newHandler(t *testing.T, cfg runtime.Config) *runtime.Handler {
	t.Helper()
	h, err := runtime.NewHandler(cfg)
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	return h
}

func serve(h *runtime.Handler) *httptest.ResponseRecorder {
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	return resp
}

// TestTheDocumentIsBuiltOnce: the first request builds the document. Later
// requests, through ServeHTTP or RegisterEcho, get the same bytes without
// another build.
func TestTheDocumentIsBuiltOnce(t *testing.T) {
	registerPlainRoute(t)
	counter := countBuilds(t)
	h := newHandler(t, runtime.Config{})
	e := echo.New()
	h.RegisterEcho(e)

	first, second := serve(h), serve(h)
	third := httptest.NewRecorder()
	e.ServeHTTP(third, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))

	for i, resp := range []*httptest.ResponseRecorder{first, second, third} {
		if resp.Code != http.StatusOK || resp.Body.String() != first.Body.String() {
			t.Errorf("request %d: status %d, same body as the first: %v", i+1, resp.Code, resp.Body.String() == first.Body.String())
		}
	}
	if got := counter.builds.Load(); got != 1 {
		t.Errorf("builds = %d, want 1", got)
	}
}

// TestConcurrentFirstRequestsShareOneBuild: builds run one at a time, so
// requests that arrive before the document exists wait for one build.
func TestConcurrentFirstRequestsShareOneBuild(t *testing.T) {
	registerPlainRoute(t)
	counter := countBuilds(t)
	h := newHandler(t, runtime.Config{})

	start := make(chan struct{})
	responses := make(chan *httptest.ResponseRecorder, 8)
	var wg sync.WaitGroup
	for range cap(responses) {
		wg.Go(func() {
			<-start
			responses <- serve(h)
		})
	}
	close(start)
	wg.Wait()
	close(responses)
	for resp := range responses {
		if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), `"/items"`) {
			t.Errorf("got %d %q, want 200 with /items", resp.Code, resp.Body.String())
		}
	}
	if got := counter.builds.Load(); got != 1 {
		t.Errorf("builds = %d, want 1", got)
	}
}

// TestRegistryChangesRebuildTheDocument: a route registered, or a plugin
// registered or removed, after the document was built reaches the next
// request.
func TestRegistryChangesRebuildTheDocument(t *testing.T) {
	registerPlainRoute(t)
	counter := countBuilds(t)
	h := newHandler(t, runtime.Config{})
	if resp := serve(h); strings.Contains(resp.Body.String(), `"/orders"`) {
		t.Fatalf("the first document already holds /orders: %s", resp.Body)
	}

	steps := []struct {
		name   string
		change func()
		want   string
		absent bool
	}{
		{"route registered", func() {
			apix.RegisterRoute(&apix.RouteRef{Method: apix.MethodGet, Path: "/orders", Responses: map[int]*apix.ResponseRef{http.StatusOK: {}}})
		}, `"/orders"`, false},
		{"plugin registered", func() {
			apix.RegisterPlugin(&serverPlugin{BasePlugin: apix.BasePlugin{PluginName: "server"}})
		}, "https://plugin.example", false},
		{"plugin removed", func() { apix.UnregisterPlugin("server") }, "https://plugin.example", true},
	}
	for i, step := range steps {
		step.change()
		resp := serve(h)
		if resp.Code != http.StatusOK || strings.Contains(resp.Body.String(), step.want) == step.absent {
			t.Errorf("%s: got %d, want 200 with %s present = %v", step.name, resp.Code, step.want, !step.absent)
		}
		if got, want := counter.builds.Load(), int32(i+2); got != want {
			t.Errorf("%s: builds = %d, want %d", step.name, got, want)
		}
	}
	serve(h)
	if got, want := counter.builds.Load(), int32(len(steps)+1); got != want {
		t.Errorf("a request after no change: builds = %d, want %d", got, want)
	}
}

// TestResetsRebuildTheDocument: ResetPlugins and ResetRegistry reach the next
// request too.
func TestResetsRebuildTheDocument(t *testing.T) {
	registerPlainRoute(t)
	apix.ResetPlugins()
	t.Cleanup(apix.ResetPlugins)
	apix.RegisterPlugin(&serverPlugin{BasePlugin: apix.BasePlugin{PluginName: "server"}})
	captureErrorLogs(t)
	h := newHandler(t, runtime.Config{})
	if resp := serve(h); !strings.Contains(resp.Body.String(), "https://plugin.example") {
		t.Fatalf("the first document has no plugin server: %s", resp.Body)
	}

	apix.ResetPlugins()
	if resp := serve(h); resp.Code != http.StatusOK || strings.Contains(resp.Body.String(), "https://plugin.example") {
		t.Errorf("after ResetPlugins: got %d %s, want 200 without the plugin server", resp.Code, resp.Body)
	}
	apix.ResetRegistry()
	if resp := serve(h); resp.Code != http.StatusInternalServerError {
		t.Errorf("after ResetRegistry: status = %d, want 500 for a registry without routes", resp.Code)
	}
}

// TestAFailedBuildIsKeptUntilTheRegistriesChange: a build that fails is not
// retried for every request. Each request it answers with 500 is logged, and
// a plugin that declares the missing scheme fixes the next request.
func TestAFailedBuildIsKeptUntilTheRegistriesChange(t *testing.T) {
	registerSecuredRoute(t, "BearerAuth")
	counter := countBuilds(t)
	logs := captureErrorLogs(t)
	h := newHandler(t, runtime.Config{})

	for range 2 {
		if resp := serve(h); resp.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", resp.Code)
		}
	}
	if got := counter.builds.Load(); got != 1 {
		t.Errorf("builds = %d, want 1", got)
	}
	if got := strings.Count(logs.String(), `"msg":"error answered with a generic 500"`); got != 2 {
		t.Errorf("want each 500 logged, got %d lines: %s", got, logs)
	}

	apix.RegisterPlugin(&bearerPlugin{BasePlugin: apix.BasePlugin{PluginName: "bearer"}})
	if resp := serve(h); resp.Code != http.StatusOK {
		t.Errorf("after the plugin declared BearerAuth: status = %d, want 200", resp.Code)
	}
	if got := counter.builds.Load(); got != 2 {
		t.Errorf("builds = %d, want 2", got)
	}
}

// TestCacheTTLRebuildsAnOlderDocument: with CacheTTL set, a document older
// than it is built again even though nothing was registered.
func TestCacheTTLRebuildsAnOlderDocument(t *testing.T) {
	registerPlainRoute(t)
	counter := countBuilds(t)
	h := newHandler(t, runtime.Config{CacheTTL: time.Nanosecond})

	serve(h)
	serve(h)
	if got := counter.builds.Load(); got != 2 {
		t.Errorf("builds = %d, want 2", got)
	}
}
