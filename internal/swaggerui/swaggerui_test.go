package swaggerui

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestDistIsTheReleaseUnmodified: swagger-ui-dist/ holds Version, and the
// embedded files are the bytes their Subresource Integrity values name, which
// the npm tarball and unpkg.com serve.
func TestDistIsTheReleaseUnmodified(t *testing.T) {
	data, err := os.ReadFile("swagger-ui-dist/package.json")
	if err != nil {
		t.Fatalf("read swagger-ui-dist/package.json: %v", err)
	}
	var pkg struct{ Name, Version string }
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatalf("decode swagger-ui-dist/package.json: %v", err)
	}
	if pkg.Name != "swagger-ui-dist" || pkg.Version != Version {
		t.Errorf("swagger-ui-dist/ holds %s %s, want swagger-ui-dist %s", pkg.Name, pkg.Version, Version)
	}

	for name, file := range map[string]struct {
		body      []byte
		integrity string
	}{
		"swagger-ui-bundle.js": {bundle, bundleIntegrity},
		"swagger-ui.css":       {stylesheet, stylesheetIntegrity},
	} {
		sum := sha512.Sum384(file.body)
		if got := "sha384-" + base64.StdEncoding.EncodeToString(sum[:]); got != file.integrity {
			t.Errorf("%s integrity = %s, want %s", name, got, file.integrity)
		}
	}
}

func TestRoutesServeThePageAndTheFilesBelowIt(t *testing.T) {
	for _, tc := range []struct {
		pagePath string
		want     []string
	}{
		{pagePath: "/swagger", want: []string{"/swagger", "/swagger/swagger-ui.css", "/swagger/swagger-ui-bundle.js", "/swagger/swagger-initializer.js"}},
		{pagePath: "/docs/", want: []string{"/docs/", "/docs/swagger-ui.css", "/docs/swagger-ui-bundle.js", "/docs/swagger-initializer.js"}},
		{pagePath: "/", want: []string{"/", "/swagger-ui.css", "/swagger-ui-bundle.js", "/swagger-initializer.js"}},
	} {
		var got []string
		for _, route := range Routes(tc.pagePath, "/openapi.json", nil) {
			got = append(got, route.Path)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("Routes(%q) paths = %v, want %v", tc.pagePath, got, tc.want)
		}
	}
}

// TestThePageRunsNoInlineCode: the page loads its stylesheet and scripts from
// below its own path and names the document in data-url, so its policy allows
// scripts and styles from its own origin only.
func TestThePageRunsNoInlineCode(t *testing.T) {
	rec := serve(t, Routes("/swagger", "/openapi.json", nil)[0], http.MethodGet, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	for header, want := range map[string]string{
		"Content-Type":            "text/html; charset=utf-8",
		"Content-Security-Policy": "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data: blob:; connect-src 'self'; media-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'",
		"X-Content-Type-Options":  "nosniff",
		"Cache-Control":           "no-store",
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<link rel="stylesheet" href="/swagger/swagger-ui.css">`,
		`<div id="swagger-ui" data-url="/openapi.json"></div>`,
		`<script src="/swagger/swagger-ui-bundle.js"></script>`,
		`<script src="/swagger/swagger-initializer.js"></script>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %s:\n%s", want, body)
		}
	}
	for _, inline := range []string{"<script>", "<style", "style="} {
		if strings.Contains(body, inline) {
			t.Errorf("page has inline code %q:\n%s", inline, body)
		}
	}
}

func TestThePageEscapesTheDocumentURL(t *testing.T) {
	for _, tc := range []struct{ specURL, want string }{
		{specURL: `/openapi.json?a=1&b="2"`, want: `data-url="/openapi.json?a=1&amp;b=%222%22"`},
		{specURL: `/x"><script>alert(1)</script>`, want: `data-url="/x%22%3e%3cscript%3ealert%281%29%3c/script%3e"`},
		{specURL: `javascript:alert(1)`, want: `data-url="#ZgotmplZ"`},
	} {
		rec := serve(t, Routes("/swagger", tc.specURL, nil)[0], http.MethodGet, nil)
		if !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("document URL %q: page lacks %s:\n%s", tc.specURL, tc.want, rec.Body.String())
		}
	}
}

// TestRequestsGoToThePageAndTheServers: the page may fetch from, and play
// audio responses from, its own origin and the origins of the absolute http
// and https servers. Anything a CSP host source cannot name is left out, so no
// server URL can end the directive.
func TestRequestsGoToThePageAndTheServers(t *testing.T) {
	for _, tc := range []struct {
		name    string
		servers []string
		want    string
	}{
		{name: "no servers", want: "'self'"},
		{name: "relative", servers: []string{"/api/v1"}, want: "'self'"},
		{name: "absolute", servers: []string{"https://api.example.com/v1", "http://localhost:8080"}, want: "'self' https://api.example.com http://localhost:8080"},
		{name: "case, port and spaces", servers: []string{" HTTPS://API.Example.com:8443/v1 "}, want: "'self' https://api.example.com:8443"},
		{name: "one source per origin", servers: []string{"https://api.example.com/a", "https://api.example.com/b"}, want: "'self' https://api.example.com"},
		{name: "IPv4", servers: []string{"http://10.0.0.7:9000"}, want: "'self' http://10.0.0.7:9000"},
		{name: "user info", servers: []string{"https://user:secret@api.example.com"}, want: "'self' https://api.example.com"},
		{name: "server variables", servers: []string{"https://{region}.example.com"}, want: "'self'"},
		{name: "IPv6", servers: []string{"http://[::1]:8080"}, want: "'self'"},
		{name: "other schemes", servers: []string{"ws://events.example.com", "ftp://files.example.com"}, want: "'self'"},
		{name: "scheme-relative", servers: []string{"//api.example.com"}, want: "'self'"},
		{name: "no host", servers: []string{"https:///v1"}, want: "'self'"},
		{name: "empty label", servers: []string{"https://api..example.com"}, want: "'self'"},
		{name: "a directive after the host", servers: []string{"https://api.example.com;script-src", "https://api.example.com;script-src *"}, want: "'self'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			policy := contentSecurityPolicy(tc.servers)
			for _, directive := range []string{"connect-src", "media-src"} {
				if got := sources(policy, directive); got != tc.want {
					t.Errorf("%s = %q, want %q (policy %q)", directive, got, tc.want, policy)
				}
			}
		})
	}
}

// TestTheFilesAreRevalidated: each file has its content type, an ETag from its
// bytes and Cache-Control: no-cache, so a browser asks again on each load and
// a request with the ETag is answered 304 Not Modified.
func TestTheFilesAreRevalidated(t *testing.T) {
	routes := Routes("/swagger", "/openapi.json", nil)
	for _, tc := range []struct {
		path        string
		contentType string
		body        []byte
	}{
		{path: "/swagger/swagger-ui.css", contentType: "text/css; charset=utf-8", body: stylesheet},
		{path: "/swagger/swagger-ui-bundle.js", contentType: "text/javascript; charset=utf-8", body: bundle},
		{path: "/swagger/swagger-initializer.js", contentType: "text/javascript; charset=utf-8", body: initializer},
	} {
		t.Run(path.Base(tc.path), func(t *testing.T) {
			route := find(t, routes, tc.path)
			sum := sha256.Sum256(tc.body)
			etag := `"` + hex.EncodeToString(sum[:]) + `"`

			rec := serve(t, route, http.MethodGet, nil)
			if rec.Code != http.StatusOK || !slices.Equal(rec.Body.Bytes(), tc.body) {
				t.Fatalf("GET: status %d with %d bytes, want 200 with the file's %d", rec.Code, rec.Body.Len(), len(tc.body))
			}
			for header, want := range map[string]string{
				"Content-Type":           tc.contentType,
				"Content-Length":         strconv.Itoa(len(tc.body)),
				"X-Content-Type-Options": "nosniff",
				"Cache-Control":          "no-cache",
				"ETag":                   etag,
			} {
				if got := rec.Header().Get(header); got != want {
					t.Errorf("%s = %q, want %q", header, got, want)
				}
			}

			rec = serve(t, route, http.MethodGet, http.Header{"If-None-Match": {etag}})
			if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
				t.Errorf("GET with the ETag: status %d with %d bytes, want 304 with none", rec.Code, rec.Body.Len())
			}

			rec = serve(t, route, http.MethodHead, nil)
			if rec.Code != http.StatusOK || rec.Body.Len() != 0 || rec.Header().Get("Content-Length") != strconv.Itoa(len(tc.body)) {
				t.Errorf("HEAD: status %d with %d bytes and Content-Length %q, want 200 with none and %d",
					rec.Code, rec.Body.Len(), rec.Header().Get("Content-Length"), len(tc.body))
			}
		})
	}
}

var errBrokenPipe = errors.New("write: broken pipe")

// failingWriter is a ResponseWriter whose client went away.
type failingWriter struct{ *httptest.ResponseRecorder }

func (failingWriter) Write([]byte) (int, error) { return 0, errBrokenPipe }

// TestAWriteTheClientMissedIsReturned: Write returns the error of a page or
// file the client did not receive in full. A 304 has no body to fail.
func TestAWriteTheClientMissedIsReturned(t *testing.T) {
	routes := Routes("/swagger", "/openapi.json", nil)
	for _, route := range routes {
		err := route.Write(failingWriter{httptest.NewRecorder()}, httptest.NewRequest(http.MethodGet, route.Path, nil))
		if !errors.Is(err, errBrokenPipe) {
			t.Errorf("%s: Write returned %v, want the write error", route.Path, err)
		}
	}

	route := find(t, routes, "/swagger/swagger-ui.css")
	etag := serve(t, route, http.MethodGet, nil).Header().Get("ETag")
	req := httptest.NewRequest(http.MethodGet, route.Path, nil)
	req.Header.Set("If-None-Match", etag)
	if err := route.Write(failingWriter{httptest.NewRecorder()}, req); err != nil {
		t.Errorf("304: Write returned %v, want nil", err)
	}
}

// TestCDNPagePinsTheReleaseWithIntegrity: the CDN page loads Version from
// unpkg.com with the integrity values of the embedded files, and its inline
// script receives the document URL as a JavaScript string.
func TestCDNPagePinsTheReleaseWithIntegrity(t *testing.T) {
	page := CDNPage("/openapi.json")
	for _, want := range []string{
		`<link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@` + Version + `/swagger-ui.css" integrity="` + stylesheetIntegrity + `" crossorigin="anonymous">`,
		`<script src="https://unpkg.com/swagger-ui-dist@` + Version + `/swagger-ui-bundle.js" integrity="` + bundleIntegrity + `" crossorigin="anonymous"></script>`,
		`url: "/openapi.json",`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("CDN page lacks %s:\n%s", want, page)
		}
	}

	page = CDNPage(`'</script><script>alert(1)</script>`)
	if want := `url: "'\u003c/script\u003e\u003cscript\u003ealert(1)\u003c/script\u003e",`; !strings.Contains(page, want) {
		t.Errorf("CDN page lacks %s:\n%s", want, page)
	}
	if n := strings.Count(page, "</script>"); n != 2 {
		t.Errorf("CDN page closes %d scripts, want 2:\n%s", n, page)
	}
}

// serve answers a request for route with the given method and headers.
func serve(t *testing.T, route Route, method string, header http.Header) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, route.Path, nil)
	for name, values := range header {
		req.Header[name] = values
	}
	rec := httptest.NewRecorder()
	if err := route.Write(rec, req); err != nil {
		t.Fatalf("%s %s: Write returned %v", method, route.Path, err)
	}
	return rec
}

func find(t *testing.T, routes []Route, path string) Route {
	t.Helper()
	for _, route := range routes {
		if route.Path == path {
			return route
		}
	}
	t.Fatalf("no route for %s", path)
	return Route{}
}

// sources returns the source list of a directive in policy.
func sources(policy, directive string) string {
	for d := range strings.SplitSeq(policy, "; ") {
		if list, ok := strings.CutPrefix(d, directive+" "); ok {
			return list
		}
	}
	return ""
}
