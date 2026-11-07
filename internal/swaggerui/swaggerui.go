// Package swaggerui serves a Swagger UI page and the swagger-ui-dist files it
// loads. The files are embedded in the binary, so the page loads nothing from
// another origin and runs under a strict Content-Security-Policy.
//
// To upgrade, check the release's npm tarball against the registry's
// dist.integrity, copy its LICENSE, NOTICE, package.json, swagger-ui.css,
// swagger-ui-bundle.js and swagger-ui-bundle.js.LICENSE.txt into
// swagger-ui-dist/ unmodified, then set Version and the integrity values
// below, and the release docs/API_REFERENCE.md names.
// TestDistIsTheReleaseUnmodified checks Version and the integrity values.
package swaggerui

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// Version is the swagger-ui-dist release in swagger-ui-dist/. The files there
// are the npm tarball's, unmodified, with its LICENSE and NOTICE.
const Version = "5.34.1"

// cdnBase is where unpkg.com serves Version's files.
const cdnBase = "https://unpkg.com/swagger-ui-dist@" + Version + "/"

// Subresource Integrity values of swagger-ui-dist/swagger-ui-bundle.js and
// swagger-ui-dist/swagger-ui.css. unpkg.com serves the same bytes under cdnBase.
const (
	bundleIntegrity     = "sha384-XFYN47VzPiDY6qPtxYKg0nqUeW/upo20xf1j2SUE7gT/i35FJB4cuS/71Iegzk8Z"
	stylesheetIntegrity = "sha384-4c3vsE5f+trhFyHrFrLM1fFGUn531NIaC7dcqEWIJdoRQJ5CkJbSuSCwXLpaS1KF"
)

var (
	//go:embed swagger-ui-dist/swagger-ui-bundle.js
	bundle []byte
	//go:embed swagger-ui-dist/swagger-ui.css
	stylesheet []byte
	//go:embed swagger-initializer.js
	initializer []byte
)

// pageTemplate is the page Routes serves. It loads every script and
// stylesheet from below its own path, and names the document in data-url for
// swagger-initializer.js to read.
var pageTemplate = template.Must(template.New("page").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Swagger UI</title>
  <link rel="stylesheet" href="{{.AssetBase}}swagger-ui.css">
</head>
<body>
  <div id="swagger-ui" data-url="{{.SpecURL}}"></div>
  <script src="{{.AssetBase}}swagger-ui-bundle.js"></script>
  <script src="{{.AssetBase}}swagger-initializer.js"></script>
</body>
</html>
`))

// cdnPageTemplate is the page CDNPage returns.
var cdnPageTemplate = template.Must(template.New("cdn").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <title>Swagger UI</title>
  <link rel="stylesheet" href="` + cdnBase + `swagger-ui.css" integrity="` + stylesheetIntegrity + `" crossorigin="anonymous">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="` + cdnBase + `swagger-ui-bundle.js" integrity="` + bundleIntegrity + `" crossorigin="anonymous"></script>
  <script>
    window.onload = () => {
      window.ui = SwaggerUIBundle({
        url: {{.SpecURL}},
        dom_id: '#swagger-ui',
        presets: [SwaggerUIBundle.presets.apis],
        layout: 'BaseLayout'
      });
    };
  </script>
</body>
</html>`))

// Route is a path the UI serves. Write answers a request for it, and returns
// the error of a write the client did not receive in full.
type Route struct {
	Path  string
	Write func(w http.ResponseWriter, r *http.Request) error
}

// Routes returns the routes of a Swagger UI page served at pagePath that shows
// the document at specURL, followed by the files the page loads from below
// pagePath: swagger-ui.css, swagger-ui-bundle.js and swagger-initializer.js.
// The page may send requests to its own origin and to the origins of servers.
func Routes(pagePath, specURL string, servers []string) []Route {
	assetBase := strings.TrimSuffix(pagePath, "/") + "/"
	var page bytes.Buffer
	if err := pageTemplate.Execute(&page, struct{ AssetBase, SpecURL string }{assetBase, specURL}); err != nil {
		panic(fmt.Sprintf("apix swaggerui: the page template failed to render: %v", err))
	}
	routes := []Route{{Path: pagePath, Write: writePage(page.Bytes(), contentSecurityPolicy(servers))}}
	for _, a := range []asset{
		newAsset("swagger-ui.css", "text/css; charset=utf-8", stylesheet),
		newAsset("swagger-ui-bundle.js", "text/javascript; charset=utf-8", bundle),
		newAsset("swagger-initializer.js", "text/javascript; charset=utf-8", initializer),
	} {
		routes = append(routes, Route{Path: assetBase + a.name, Write: a.write})
	}
	return routes
}

// CDNPage returns a Swagger UI page that shows the document at specURL and
// loads Version from unpkg.com, checked with Subresource Integrity. It starts
// Swagger UI from an inline script.
func CDNPage(specURL string) string {
	var page strings.Builder
	if err := cdnPageTemplate.Execute(&page, struct{ SpecURL string }{specURL}); err != nil {
		panic(fmt.Sprintf("apix swaggerui: the CDN page template failed to render: %v", err))
	}
	return page.String()
}

// writePage returns the Write of the page.
func writePage(page []byte, policy string) func(http.ResponseWriter, *http.Request) error {
	return func(w http.ResponseWriter, _ *http.Request) error {
		h := w.Header()
		h.Set("Content-Type", "text/html; charset=utf-8")
		h.Set("Content-Security-Policy", policy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cache-Control", "no-store")
		_, err := w.Write(page)
		return err
	}
}

// contentSecurityPolicy returns the page's policy. Scripts and styles come only
// from the page's origin, and images from it or from the data: URLs of the
// stylesheet's icons and the blob: URLs image responses render from. Requests,
// and audio responses, which play from the request's URL, go only to the page's
// origin and the origins of servers. Nothing may frame the page, set its base
// URL or submit its forms.
func contentSecurityPolicy(servers []string) string {
	sources := []string{"'self'"}
	for _, server := range servers {
		if origin, ok := serverOrigin(server); ok && !slices.Contains(sources, origin) {
			sources = append(sources, origin)
		}
	}
	requests := strings.Join(sources, " ")
	return "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data: blob:; " +
		"connect-src " + requests + "; media-src " + requests + "; " +
		"frame-ancestors 'none'; base-uri 'none'; form-action 'none'"
}

// serverOrigin returns the origin of an absolute http or https server URL as a
// CSP host source. It reports false for a relative URL, which 'self' covers,
// and for a URL whose host a host source cannot name, such as one with server
// variables or an IPv6 address.
func serverOrigin(server string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(server))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || !isHostname(u.Hostname()) {
		return "", false
	}
	origin := u.Scheme + "://" + strings.ToLower(u.Hostname())
	if port := u.Port(); port != "" {
		origin += ":" + port
	}
	return origin, true
}

// isHostname reports whether host is dot-separated labels of letters, digits
// and hyphens, the hosts a CSP host source can name. Nothing else can end the
// directive.
func isHostname(host string) bool {
	if host == "" {
		return false
	}
	for label := range strings.SplitSeq(host, ".") {
		if label == "" {
			return false
		}
		for _, c := range label {
			if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' {
				return false
			}
		}
	}
	return true
}

// asset is a file the page loads.
type asset struct {
	name        string
	contentType string
	body        []byte
	etag        string
}

func newAsset(name, contentType string, body []byte) asset {
	sum := sha256.Sum256(body)
	return asset{name: name, contentType: contentType, body: body, etag: `"` + hex.EncodeToString(sum[:]) + `"`}
}

// write serves the asset. A browser revalidates it on each load, and a request
// with its ETag is answered 304 Not Modified.
func (a asset) write(w http.ResponseWriter, r *http.Request) error {
	h := w.Header()
	h.Set("Content-Type", a.contentType)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-cache")
	h.Set("ETag", a.etag)
	recorder := &writeErrorRecorder{ResponseWriter: w}
	http.ServeContent(recorder, r, a.name, time.Time{}, bytes.NewReader(a.body))
	return recorder.err
}

// writeErrorRecorder keeps the first error a write returns, which
// http.ServeContent does not return.
type writeErrorRecorder struct {
	http.ResponseWriter
	err error
}

func (w *writeErrorRecorder) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	if err != nil && w.err == nil {
		w.err = err
	}
	return n, err
}
