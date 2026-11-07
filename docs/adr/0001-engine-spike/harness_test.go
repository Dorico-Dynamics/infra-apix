package spike

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
)

func init() {
	openapi3filter.RegisterBodyDecoder("application/pdf", openapi3filter.FileBodyDecoder)
}

// engine is one implementation of the six spike routes: the handler that
// serves them and the OpenAPI document the engine generates for them.
type engine struct {
	name    string
	handler http.Handler
	doc     []byte // OpenAPI 3.1, JSON
	doc30   []byte // the engine's OpenAPI 3.0.3 rendition, JSON; nil when it has none
	// serveMux is set when the routes are served by a net/http ServeMux
	// rather than a third-party router.
	serveMux bool
}

// server runs an engine and checks every exchange against its document.
type server struct {
	t      *testing.T
	eng    engine
	srv    *httptest.Server
	doc    *openapi3.T
	router routers.Router
}

func newServer(t *testing.T, eng engine) *server {
	t.Helper()
	srv := httptest.NewServer(eng.handler)
	t.Cleanup(srv.Close)
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(eng.doc)
	if err != nil {
		t.Fatalf("%s: load document: %v", eng.name, err)
	}
	doc.Servers = openapi3.Servers{{URL: srv.URL}}
	router, err := gorillamux.NewRouter(doc)
	if err != nil {
		t.Fatalf("%s: router: %v", eng.name, err)
	}
	return &server{t: t, eng: eng, srv: srv, doc: doc, router: router}
}

// exchange is one request and the response it got.
type exchange struct {
	path   string // the request path, without the query
	status int
	header http.Header
	body   []byte
	// docErr is why the exchange does not match the document: an
	// undocumented operation, status or header, or a body outside its
	// schema. Nil when it matches.
	docErr error
}

// do sends a request and validates request and response against the document.
func (s *server) do(method, path string, header http.Header, body []byte) exchange {
	s.t.Helper()
	req, err := http.NewRequest(method, s.srv.URL+path, bytes.NewReader(body))
	if err != nil {
		s.t.Fatal(err)
	}
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	resp, err := s.srv.Client().Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	ex := exchange{path: req.URL.Path, status: resp.StatusCode, header: resp.Header, body: respBody}
	ex.docErr = s.validate(req, body, resp, respBody)
	return ex
}

func (s *server) validate(req *http.Request, reqBody []byte, resp *http.Response, respBody []byte) error {
	// Validate a copy, so the body can be read again.
	vreq := req.Clone(context.Background())
	vreq.Body = io.NopCloser(bytes.NewReader(reqBody))
	route, params, err := s.router.FindRoute(vreq)
	if err != nil {
		return fmt.Errorf("operation not documented: %w", err)
	}
	in := &openapi3filter.RequestValidationInput{
		Request:    vreq,
		PathParams: params,
		Route:      route,
		Options: &openapi3filter.Options{
			AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
			MultiError:         true,
		},
	}
	// The request is only context here: invalid requests are sent on
	// purpose, so request validation errors are not reported.
	_ = openapi3filter.ValidateRequest(context.Background(), in)
	out := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: in,
		Status:                 resp.StatusCode,
		Header:                 resp.Header,
		Body:                   io.NopCloser(bytes.NewReader(respBody)),
		Options:                &openapi3filter.Options{MultiError: true, IncludeResponseStatus: true},
	}
	if err := openapi3filter.ValidateResponse(context.Background(), out); err != nil {
		// kin-openapi puts the schema and the value on the lines after the
		// reason; the scoreboard keeps the reason.
		reason, _, _ := strings.Cut(err.Error(), "\n")
		return fmt.Errorf("response does not match the document: %s", reason)
	}
	return checkDocumentedHeaders(route.Operation, resp)
}

// checkDocumentedHeaders reports headers a success response sends that the
// document does not declare for its status: kin-openapi validates declared
// headers but does not require every sent header to be declared.
func checkDocumentedHeaders(op *openapi3.Operation, resp *http.Response) error {
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil
	}
	ref := op.Responses.Status(resp.StatusCode)
	if ref == nil || ref.Value == nil {
		return nil // already reported by ValidateResponse
	}
	var missing []string
	for _, h := range []string{"ETag", "Location", "Content-Disposition", "X-Idempotency-Replay", "Cache-Control", "Pragma"} {
		if resp.Header.Get(h) == "" {
			continue
		}
		if _, ok := ref.Value.Headers[h]; !ok {
			missing = append(missing, h)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("status %d sends undocumented header(s) %s", resp.StatusCode, strings.Join(missing, ", "))
	}
	return nil
}

// result is the outcome of one conformance row on one engine.
type result struct {
	runtime string // "" when the behaviour is right, else why not
	doc     string // "" when the exchange matches the document, else why not
}

// scoreboard collects results by requirement and engine for the summary.
type scoreboard struct {
	mu   sync.Mutex
	rows map[string]map[string]result
}

var board = &scoreboard{rows: map[string]map[string]result{}}

func (b *scoreboard) record(row, engineName string, r result) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.rows[row] == nil {
		b.rows[row] = map[string]result{}
	}
	b.rows[row][engineName] = r
}

// summary renders the scoreboard as a Markdown table, rows sorted by id.
func (b *scoreboard) summary(engines []string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	ids := make([]string, 0, len(b.rows))
	for id := range b.rows {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var sb strings.Builder
	sb.WriteString("| Row |")
	for _, e := range engines {
		sb.WriteString(" " + e + " |")
	}
	sb.WriteString("\n|---|")
	for range engines {
		sb.WriteString("---|")
	}
	sb.WriteString("\n")
	passed := map[string]int{}
	for _, id := range ids {
		sb.WriteString("| " + id + " |")
		for _, e := range engines {
			r, ok := b.rows[id][e]
			switch {
			case !ok:
				sb.WriteString(" not run |")
			case r.runtime == "" && r.doc == "":
				passed[e]++
				sb.WriteString(" pass |")
			default:
				var why []string
				if r.runtime != "" {
					why = append(why, "runtime: "+r.runtime)
				}
				if r.doc != "" {
					why = append(why, "document: "+r.doc)
				}
				sb.WriteString(" FAIL: " + strings.ReplaceAll(strings.Join(why, "; "), "|", "/") + " |")
			}
		}
		sb.WriteString("\n")
	}
	sb.WriteString("| **Passed** |")
	for _, e := range engines {
		fmt.Fprintf(&sb, " %d of %d |", passed[e], len(ids))
	}
	sb.WriteString("\n")
	return sb.String()
}

// decodeJSON decodes a response body into a map, or reports why it can't.
func decodeJSON(body []byte) (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("body is not a JSON object: %q", truncate(body))
	}
	return m, nil
}

func truncate(b []byte) string {
	const n = 160
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}
