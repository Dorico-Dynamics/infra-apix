package spike

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// The engines under test, in scoreboard column order.
var engines = []struct {
	name  string
	build func(testing.TB) engine
}{
	{"apix", newApixEngine},
	{"huma", newHumaEngine},
}

func engineNames() []string {
	names := make([]string, len(engines))
	for i, e := range engines {
		names[i] = e.name
	}
	return names
}

// row is one conformance requirement, checked against a fresh engine.
type row struct {
	id   string
	desc string
	run  func(c *check)
}

// TestMain writes the scoreboard of every test's rows to the file named by
// SPIKE_SUMMARY.
func TestMain(m *testing.M) {
	code := m.Run()
	if path := os.Getenv("SPIKE_SUMMARY"); path != "" {
		if err := os.WriteFile(path, []byte(board.summary(engineNames())), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			code = 1
		}
	}
	os.Exit(code)
}

// TestConformance runs every row on every engine. A row that fails is a
// measurement, not a test failure; only a harness that cannot run fails the
// test.
func TestConformance(t *testing.T) {
	for _, e := range engines {
		for _, r := range rows {
			t.Run(e.name+"/"+r.id, func(t *testing.T) {
				c := &check{s: newServer(t, e.build(t)), row: r.id}
				r.run(c)
				res := c.result()
				if res.runtime != "" || res.doc != "" {
					t.Logf("runtime: %s\ndocument: %s", res.runtime, res.doc)
				}
				board.record(r.id+" "+r.desc, e.name, res)
			})
		}
	}
}

// check collects one row's failures on one engine.
type check struct {
	s       *server
	row     string
	runtime []string
	doc     []string
}

func (c *check) fail(format string, args ...any) {
	c.runtime = append(c.runtime, fmt.Sprintf(format, args...))
}

func (c *check) docFail(format string, args ...any) {
	c.doc = append(c.doc, fmt.Sprintf(format, args...))
}

func (c *check) result() result {
	return result{runtime: strings.Join(c.runtime, "; "), doc: strings.Join(c.doc, "; ")}
}

// send makes a request carrying the row's request and correlation IDs, and
// records a response that does not match the document.
func (c *check) send(method, path string, header http.Header, body []byte) exchange {
	h := http.Header{}
	for k, vs := range header {
		h[k] = vs
	}
	h.Set(requestIDHeader, "req-"+c.row)
	h.Set(correlationIDHeader, "corr-"+c.row)
	ex := c.s.do(method, path, h, body)
	if ex.docErr != nil {
		c.docFail("%s %s → %d: %v", method, path, ex.status, ex.docErr)
	}
	return ex
}

func (c *check) sendJSON(method, path string, header http.Header, body string) exchange {
	h := http.Header{"Content-Type": {"application/json"}}
	for k, vs := range header {
		h[k] = vs
	}
	return c.send(method, path, h, []byte(body))
}

func (c *check) status(ex exchange, want int) bool {
	if ex.status != want {
		c.fail("status %d, want %d (%s)", ex.status, want, truncate(ex.body))
		return false
	}
	return true
}

func (c *check) header(ex exchange, name, want string) {
	if got := ex.header.Get(name); got != want {
		c.fail("%s %q, want %q", name, got, want)
	}
}

// object decodes a JSON object body.
func (c *check) object(ex exchange) map[string]any {
	m, err := decodeJSON(ex.body)
	if err != nil {
		c.fail("%v", err)
	}
	return m
}

// problem checks that ex is the platform error with code, and returns its
// field errors.
func (c *check) problem(ex exchange, code string) []map[string]any {
	want := catalog[code]
	if !c.status(ex, want.status) {
		return nil
	}
	if ct := ex.header.Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		c.fail("Content-Type %q, want application/problem+json", ct)
	}
	m := c.object(ex)
	if m == nil {
		return nil
	}
	members := map[string]any{
		"type":           problemTypeBase + code,
		"title":          want.title,
		"status":         float64(want.status),
		"code":           code,
		"instance":       ex.path,
		"request_id":     "req-" + c.row,
		"correlation_id": "corr-" + c.row,
	}
	for _, k := range sortedKeys(members) {
		if m[k] != members[k] {
			c.fail("%s %v, want %v", k, m[k], members[k])
		}
	}
	if detail, _ := m["detail"].(string); detail == "" {
		c.fail("no detail")
	}
	for k := range m {
		if _, ok := members[k]; !ok && k != "detail" && k != "errors" {
			c.fail("unexpected member %q", k)
		}
	}
	raw, _ := m["errors"].([]any)
	fields := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		if f, ok := r.(map[string]any); ok {
			fields = append(fields, f)
		}
	}
	return fields
}

// field checks that fields name name with code.
func (c *check) field(fields []map[string]any, name, code string) {
	for _, f := range fields {
		if f["field"] == name {
			if f["code"] != code {
				c.fail("field %s: code %v, want %s", name, f["code"], code)
			}
			if msg, _ := f["message"].(string); msg == "" {
				c.fail("field %s: no message", name)
			}
			return
		}
	}
	c.fail("no field error for %q in %v", name, fields)
}

// oauth checks that ex is an RFC 6749 §5.2 error with status and code.
func (c *check) oauth(ex exchange, status int, code string) {
	if !c.status(ex, status) {
		return
	}
	if ct := ex.header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		c.fail("Content-Type %q, want application/json", ct)
	}
	m := c.object(ex)
	if m == nil {
		return
	}
	if m["error"] != code {
		c.fail("error %v, want %s", m["error"], code)
	}
	for k := range m {
		if k != "error" && k != "error_description" && k != "error_uri" {
			c.fail("member %q is not RFC 6749", k)
		}
	}
}

// Document fragments.

func (c *check) operation(method, path string) *openapi3.Operation {
	item := c.s.doc.Paths.Value(path)
	if item == nil {
		c.docFail("no path %s", path)
		return nil
	}
	op := item.GetOperation(method)
	if op == nil {
		c.docFail("no operation %s %s", method, path)
	}
	return op
}

func (c *check) parameter(op *openapi3.Operation, in, name string) *openapi3.Parameter {
	if op == nil {
		return nil
	}
	for _, p := range op.Parameters {
		if p.Value != nil && p.Value.In == in && strings.EqualFold(p.Value.Name, name) {
			return p.Value
		}
	}
	c.docFail("%s %s parameter %q not documented", op.OperationID, in, name)
	return nil
}

func (c *check) requestSchema(op *openapi3.Operation, contentType string) *openapi3.Schema {
	if op == nil {
		return nil
	}
	if op.RequestBody == nil || op.RequestBody.Value == nil || op.RequestBody.Value.Content[contentType] == nil {
		c.docFail("%s: no %s request body", op.OperationID, contentType)
		return nil
	}
	return schemaOf(op.RequestBody.Value.Content[contentType].Schema)
}

func (c *check) property(s *openapi3.Schema, name string) *openapi3.Schema {
	if s == nil {
		return nil
	}
	if p := schemaOf(s.Properties[name]); p != nil {
		return p
	}
	for _, sub := range s.AllOf {
		if p := c.property(schemaOf(sub), name); p != nil {
			return p
		}
	}
	c.docFail("property %q not documented", name)
	return nil
}

// bounds checks a numeric schema's documented minimum and maximum.
func (c *check) bounds(what string, s *openapi3.Schema, minimum, maximum float64) {
	if s == nil {
		return
	}
	if s.Min == nil || *s.Min != minimum {
		c.docFail("%s: minimum %v, want %v", what, ptrValue(s.Min), minimum)
	}
	if s.Max == nil || *s.Max != maximum {
		c.docFail("%s: maximum %v, want %v", what, ptrValue(s.Max), maximum)
	}
}

func (c *check) enum(what string, s *openapi3.Schema, want ...string) {
	if s == nil {
		return
	}
	got := make([]string, 0, len(s.Enum))
	for _, v := range s.Enum {
		got = append(got, fmt.Sprint(v))
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		c.docFail("%s: enum %v, want %v", what, got, want)
	}
}

func (c *check) format(what string, s *openapi3.Schema, want string) {
	if s != nil && s.Format != want {
		c.docFail("%s: format %q, want %q", what, s.Format, want)
	}
}

func schemaOf(ref *openapi3.SchemaRef) *openapi3.Schema {
	if ref == nil {
		return nil
	}
	return ref.Value
}

func ptrValue(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// multipartBody builds a statement upload; file == nil leaves the file out.
func multipartBody(fields map[string]string, file []byte) ([]byte, string) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		_ = w.WriteField(k, fields[k])
	}
	if file != nil {
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", `form-data; name="file"; filename="statement.csv"`)
		h.Set("Content-Type", "text/csv")
		part, _ := w.CreatePart(h)
		_, _ = part.Write(file)
	}
	_ = w.Close()
	return buf.Bytes(), w.FormDataContentType()
}

func form(values url.Values) (http.Header, []byte) {
	return http.Header{"Content-Type": {"application/x-www-form-urlencoded"}}, []byte(values.Encode())
}

var statementCSV = []byte("date,amount,reference\n2026-10-01,1500.00,INV-1\n2026-10-02,-200.00,FEE\n")

const (
	progressPath = "/phases/" + phaseID + "/progress"
	uploadPath   = "/bank-accounts/" + bankAccountID + "/upload-statement"
)

func metadataPath(version, id string) string { return "/" + version + "/documents/" + id + "/metadata" }

func payStubPath(id string) string { return "/pay-stubs/" + id + "/pdf" }

func upload(c *check, path string, fields map[string]string, file []byte) exchange {
	body, ct := multipartBody(fields, file)
	return c.send(http.MethodPost, path, http.Header{"Content-Type": {ct}}, body)
}

func token(c *check, values url.Values) exchange {
	h, body := form(values)
	return c.send(http.MethodPost, "/oauth2/token", h, body)
}

var rows = []row{
	{"R1a", "action POST answers 200 with the new ETag", func(c *check) {
		ex := c.sendJSON(http.MethodPost, progressPath, http.Header{"If-Match": {`"1"`}}, `{"percent": 55}`)
		if c.status(ex, http.StatusOK) {
			c.header(ex, "ETag", `"2"`)
			if m := c.object(ex); m != nil && (m["percent"] != 55.0 || m["version"] != 2.0) {
				c.fail("body %v, want percent 55 and version 2", m)
			}
		}
	}},
	{"R1b", "stale If-Match → 412", func(c *check) {
		c.problem(c.sendJSON(http.MethodPost, progressPath, http.Header{"If-Match": {`"9"`}}, `{"percent": 55}`), codePreconditionFailed)
	}},
	{"R1c", "missing If-Match → 428; If-Match documented as required", func(c *check) {
		c.problem(c.sendJSON(http.MethodPost, progressPath, nil, `{"percent": 55}`), codePreconditionRequired)
		if p := c.parameter(c.operation(http.MethodPost, "/phases/{id}/progress"), "header", "If-Match"); p != nil && !p.Required {
			c.docFail("If-Match is not documented as required")
		}
	}},
	{"R1d", "body field out of range → 400 naming the JSON field; bounds documented", func(c *check) {
		ex := c.sendJSON(http.MethodPost, progressPath, http.Header{"If-Match": {`"1"`}}, `{"percent": 150}`)
		c.field(c.problem(ex, codeValidationFailed), "percent", fieldOutOfRange)
		op := c.operation(http.MethodPost, "/phases/{id}/progress")
		c.bounds("percent", c.property(c.requestSchema(op, "application/json"), "percent"), 0, 100)
	}},
	{"R1e", "path UUID bound from the template: bad value → 400 naming it; format documented", func(c *check) {
		ex := c.sendJSON(http.MethodPost, "/phases/not-a-uuid/progress", http.Header{"If-Match": {`"1"`}}, `{"percent": 55}`)
		c.field(c.problem(ex, codeValidationFailed), "id", fieldInvalidFormat)
		if p := c.parameter(c.operation(http.MethodPost, "/phases/{id}/progress"), "path", "id"); p != nil {
			c.format("path id", schemaOf(p.Schema), "uuid")
		}
	}},
	{"R1f", "unknown resource → 404", func(c *check) {
		c.problem(c.sendJSON(http.MethodPost, "/phases/"+unknownID+"/progress", http.Header{"If-Match": {`"1"`}}, `{"percent": 55}`), codeNotFound)
	}},
	{"R2a", "PDF download with Content-Disposition; raw bytes documented", func(c *check) {
		ex := c.send(http.MethodGet, payStubPath(strconv.Itoa(payPeriodID)), nil, nil)
		if c.status(ex, http.StatusOK) {
			c.header(ex, "Content-Type", "application/pdf")
			c.header(ex, "Content-Disposition", "attachment; filename=paystub.pdf")
			if !bytes.HasPrefix(ex.body, []byte("%PDF-")) {
				c.fail("body is not a PDF: %q", truncate(ex.body))
			}
		}
		if op := c.operation(http.MethodGet, "/pay-stubs/{payPeriodID}/pdf"); op != nil {
			resp := op.Responses.Status(http.StatusOK)
			switch {
			case resp == nil || resp.Value == nil || resp.Value.Content["application/pdf"] == nil:
				c.docFail("200 has no application/pdf content")
			default:
				if s := schemaOf(resp.Value.Content["application/pdf"].Schema); s != nil && (s.Format == "byte" || s.ContentEncoding == "base64" || s.Type.Is("array")) {
					c.docFail("application/pdf documented as base64 or an array (type %v, format %q, contentEncoding %q), not raw bytes", s.Type, s.Format, s.ContentEncoding)
				}
			}
		}
	}},
	{"R2b", "integer path parameter: below minimum or not an integer → 400 naming it; bounds documented", func(c *check) {
		c.field(c.problem(c.send(http.MethodGet, payStubPath("0"), nil, nil), codeValidationFailed), "payPeriodID", fieldOutOfRange)
		c.field(c.problem(c.send(http.MethodGet, payStubPath("abc"), nil, nil), codeValidationFailed), "payPeriodID", fieldInvalidFormat)
		if p := c.parameter(c.operation(http.MethodGet, "/pay-stubs/{payPeriodID}/pdf"), "path", "payPeriodID"); p != nil {
			s := schemaOf(p.Schema)
			if s == nil || !s.Type.Is("integer") {
				c.docFail("payPeriodID is not documented as an integer")
			} else if s.Min == nil || *s.Min != 1 {
				c.docFail("payPeriodID: minimum %v, want 1", ptrValue(s.Min))
			}
		}
	}},
	{"R2c", "error on a binary route is still the platform error", func(c *check) {
		c.problem(c.send(http.MethodGet, payStubPath("199912"), nil, nil), codeNotFound)
	}},
	{"R3a", "multipart upload (file + field) → 200 summary; form documented", func(c *check) {
		ex := upload(c, uploadPath, map[string]string{"format": "mt940"}, statementCSV)
		if c.status(ex, http.StatusOK) {
			if m := c.object(ex); m != nil && (m["total_processed"] != 2.0 || m["inserted"] != 2.0 || m["bank_account_id"] != bankAccountID) {
				c.fail("body %v does not summarise the import", m)
			}
		}
		s := c.requestSchema(c.operation(http.MethodPost, "/bank-accounts/{id}/upload-statement"), "multipart/form-data")
		if f := c.property(s, "file"); f != nil && f.Format != "binary" && f.ContentMediaType == "" {
			c.docFail("file: not documented as binary")
		}
	}},
	{"R3b", "upload over the cap → 413", func(c *check) {
		c.problem(upload(c, uploadPath, map[string]string{"format": "mt940"}, bytes.Repeat([]byte("x"), statementMaxBytes+1)), codePayloadTooLarge)
	}},
	{"R3c", "JSON sent to the multipart route → 415", func(c *check) {
		c.problem(c.sendJSON(http.MethodPost, uploadPath, nil, `{"format": "mt940"}`), codeUnsupportedMediaType)
	}},
	{"R3d", "form field outside its enum → 400 naming it; enum documented", func(c *check) {
		c.field(c.problem(upload(c, uploadPath, map[string]string{"format": "pdf"}, statementCSV), codeValidationFailed), "format", fieldInvalidFormat)
		s := c.requestSchema(c.operation(http.MethodPost, "/bank-accounts/{id}/upload-statement"), "multipart/form-data")
		c.enum("format", c.property(s, "format"), "mt940", "camt053", "bai2", "ofx")
	}},
	{"R3e", "missing file part → 400 naming it", func(c *check) {
		c.field(c.problem(upload(c, uploadPath, map[string]string{"format": "mt940"}, nil), codeValidationFailed), "file", fieldRequired)
	}},
	{"R4a", "v1 and v2 aliases share one handler, distinct operationIds; no-store sent", func(c *check) {
		for i, v := range []string{"v1", "v2"} {
			ex := c.sendJSON(http.MethodPut, metadataPath(v, documentID), nil, fmt.Sprintf(`{"expected_revision": %d, "fields": {"title": "Supply contract v%d"}}`, i+1, i+2))
			if c.status(ex, http.StatusOK) {
				c.header(ex, "Cache-Control", "no-store")
				if m := c.object(ex); m != nil && m["revision"] != float64(i+2) {
					c.fail("%s: revision %v, want %d", v, m["revision"], i+2)
				}
			}
		}
		v1, v2 := c.operation(http.MethodPut, "/v1/documents/{id}/metadata"), c.operation(http.MethodPut, "/v2/documents/{id}/metadata")
		if v1 != nil && v2 != nil && (v1.OperationID == v2.OperationID || v1.OperationID == "" || v2.OperationID == "") {
			c.docFail("operationIds %q and %q are not distinct", v1.OperationID, v2.OperationID)
		}
	}},
	{"R4b", "served by net/http ServeMux, no third-party router", func(c *check) {
		if !c.s.eng.serveMux {
			c.fail("no net/http ServeMux adapter; chi serves the routes")
		}
	}},
	{"R4c", "stale revision → 409 version_mismatch, still no-store", func(c *check) {
		ex := c.sendJSON(http.MethodPut, metadataPath("v1", documentID), nil, `{"expected_revision": 7, "fields": {"title": "x"}}`)
		c.problem(ex, codeVersionMismatch)
		c.header(ex, "Cache-Control", "no-store")
	}},
	{"R4d", "JSON body over the route's cap → 413", func(c *check) {
		big := `{"expected_revision": 1, "fields": {"notes": "` + strings.Repeat("x", metadataMaxBytes) + `"}}`
		c.problem(c.sendJSON(http.MethodPut, metadataPath("v1", documentID), nil, big), codePayloadTooLarge)
	}},
	{"R4e", "5xx never echoes internals", func(c *check) {
		ex := c.sendJSON(http.MethodPut, metadataPath("v1", brokenDocumentID), nil, `{"expected_revision": 1, "fields": {}}`)
		c.problem(ex, codeInternal)
		for _, leak := range []string{"192.0.2.7", "connection refused", "storage:"} {
			if bytes.Contains(ex.body, []byte(leak)) {
				c.fail("body echoes %q", leak)
			}
		}
	}},
	{"R5a", "cursor pagination: next_cursor chains pages, absent on the last", func(c *check) {
		path, seen := "/grants?limit=2", 0
		for page := 0; page < 4 && path != ""; page++ {
			ex := c.send(http.MethodGet, path, nil, nil)
			if !c.status(ex, http.StatusOK) {
				return
			}
			m := c.object(ex)
			data, _ := m["data"].([]any)
			seen += len(data)
			meta, _ := m["meta"].(map[string]any)
			pg, _ := meta["pagination"].(map[string]any)
			next, present := pg["next_cursor"]
			if pg["has_more"] != present || pg["limit"] != 2.0 {
				c.fail("page %d: pagination %v", page, pg)
				return
			}
			if (seen < 5) != present {
				c.fail("page %d: next_cursor present=%v with %d of 5 grants seen", page, present, seen)
				return
			}
			path = ""
			if present {
				path = "/grants?limit=2&cursor=" + url.QueryEscape(fmt.Sprint(next))
			}
		}
		if seen != 5 {
			c.fail("saw %d grants, want 5", seen)
		}
	}},
	{"R5b", "query parameter bounds → 400 naming it; bounds and default documented", func(c *check) {
		for _, limit := range []string{"0", "101"} {
			c.field(c.problem(c.send(http.MethodGet, "/grants?limit="+limit, nil, nil), codeValidationFailed), "limit", fieldOutOfRange)
		}
		if p := c.parameter(c.operation(http.MethodGet, "/grants"), "query", "limit"); p != nil {
			s := schemaOf(p.Schema)
			c.bounds("limit", s, 1, 100)
			if s != nil && fmt.Sprint(s.Default) != "50" {
				c.docFail("limit: default %v, want 50", s.Default)
			}
		}
	}},
	{"R5c", "cursor the API did not issue → 400 naming it", func(c *check) {
		c.field(c.problem(c.send(http.MethodGet, "/grants?cursor=bm90LWEtY3Vyc29y", nil, nil), codeValidationFailed), "cursor", fieldInvalidFormat)
	}},
	{"R5d", "Idempotency-Key replay: same 201, body and Location, X-Idempotency-Replay: true, one grant", func(c *check) {
		key := http.Header{"Idempotency-Key": {"k-r5d"}}
		first := c.sendJSON(http.MethodPost, "/grants", key, `{"title": "Water access", "amount": 2500}`)
		replay := c.sendJSON(http.MethodPost, "/grants", key, `{"title": "Water access", "amount": 2500}`)
		if c.status(first, http.StatusCreated) && c.status(replay, http.StatusCreated) {
			c.header(first, "X-Idempotency-Replay", "")
			c.header(replay, "X-Idempotency-Replay", "true")
			c.header(replay, "Location", first.header.Get("Location"))
			if !bytes.Equal(first.body, replay.body) {
				c.fail("replay body %s, want %s", replay.body, first.body)
			}
		}
		if m := c.object(c.send(http.MethodGet, "/grants?limit=100", nil, nil)); m != nil {
			if data, _ := m["data"].([]any); len(data) != 6 {
				c.fail("%d grants after a create and its replay, want 6", len(data))
			}
		}
	}},
	{"R5e", "Idempotency-Key reused with another body → 409", func(c *check) {
		key := http.Header{"Idempotency-Key": {"k-r5e"}}
		c.sendJSON(http.MethodPost, "/grants", key, `{"title": "Water access", "amount": 2500}`)
		c.problem(c.sendJSON(http.MethodPost, "/grants", key, `{"title": "Water access", "amount": 9999}`), codeConflict)
	}},
	{"R6a", "OAuth2 token: form body → 200 with no-store; form documented", func(c *check) {
		ex := token(c, url.Values{"grant_type": {"client_credentials"}, "client_id": {clientID}, "client_secret": {clientSecret}})
		if c.status(ex, http.StatusOK) {
			c.header(ex, "Cache-Control", "no-store")
			c.header(ex, "Pragma", "no-cache")
			if m := c.object(ex); m != nil && (m["access_token"] != "at-"+clientID || m["token_type"] != "Bearer") {
				c.fail("body %v is not the token", m)
			}
		}
		s := c.requestSchema(c.operation(http.MethodPost, "/oauth2/token"), "application/x-www-form-urlencoded")
		c.enum("grant_type", c.property(s, "grant_type"), "client_credentials")
	}},
	{"R6b", "unsupported grant_type → 400 in RFC 6749 format (a per-operation error format)", func(c *check) {
		c.oauth(token(c, url.Values{"grant_type": {"password"}, "client_id": {clientID}, "client_secret": {clientSecret}}), http.StatusBadRequest, "unsupported_grant_type")
	}},
	{"R6c", "missing parameter → 400 invalid_request in RFC 6749 format", func(c *check) {
		c.oauth(token(c, url.Values{"client_id": {clientID}, "client_secret": {clientSecret}}), http.StatusBadRequest, "invalid_request")
	}},
	{"R6d", "bad client secret → 401 invalid_client in RFC 6749 format", func(c *check) {
		c.oauth(token(c, url.Values{"grant_type": {"client_credentials"}, "client_id": {clientID}, "client_secret": {"wrong"}}), http.StatusUnauthorized, "invalid_client")
	}},
	{"R6e", "JSON sent to the token endpoint → 400 invalid_request: RFC 6749 requires a form", func(c *check) {
		body := `{"grant_type": "client_credentials", "client_id": "` + clientID + `", "client_secret": "` + clientSecret + `"}`
		c.oauth(c.sendJSON(http.MethodPost, "/oauth2/token", nil, body), http.StatusBadRequest, "invalid_request")
	}},
	{"X1", "malformed JSON → 400", func(c *check) {
		c.problem(c.sendJSON(http.MethodPost, "/grants", nil, `{"title": `), codeValidationFailed)
	}},
	{"X2", "JSON route refuses another Content-Type → 415", func(c *check) {
		ex := c.send(http.MethodPost, "/grants", http.Header{"Content-Type": {"text/plain"}}, []byte(`{"title": "Water access", "amount": 2500}`))
		c.problem(ex, codeUnsupportedMediaType)
	}},
	{"X3", "unknown JSON field → 400", func(c *check) {
		c.problem(c.sendJSON(http.MethodPost, "/grants", nil, `{"title": "Water access", "amount": 2500, "owner": "x"}`), codeValidationFailed)
	}},
}

// specRows check the generated documents, independent of requests.
var specRows = []struct {
	id, desc string
	run      func(t *testing.T, e engine) string
}{
	{"S1", "document valid as OpenAPI 3.1 (kin-openapi)", func(_ *testing.T, e engine) string {
		return validateDoc(e.doc, "3.1")
	}},
	{"S2", "OpenAPI 3.0.3 rendition, valid", func(_ *testing.T, e engine) string {
		if e.doc30 == nil {
			return "no 3.0.3 rendition"
		}
		return validateDoc(e.doc30, "3.0")
	}},
	{"S3", "no dangling $ref", func(_ *testing.T, e engine) string {
		return danglingRefs(e.doc)
	}},
	{"S4", "deterministic: two builds give identical bytes", func(t *testing.T, e engine) string {
		for _, x := range engines {
			if x.name == e.name {
				if again := x.build(t); !bytes.Equal(again.doc, e.doc) || !bytes.Equal(again.doc30, e.doc30) {
					return "documents differ between builds"
				}
			}
		}
		return ""
	}},
}

func TestSpecQuality(t *testing.T) {
	for _, x := range engines {
		e := x.build(t)
		for _, r := range specRows {
			t.Run(x.name+"/"+r.id, func(t *testing.T) {
				why := r.run(t, e)
				if why != "" {
					t.Log(why)
				}
				board.record(r.id+" "+r.desc, x.name, result{doc: why})
			})
		}
	}
}

// validateDoc loads raw with kin-openapi and validates it as the given
// version, returning "" when valid.
func validateDoc(raw []byte, version string) string {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(raw)
	if err != nil {
		return "load: " + err.Error()
	}
	if !strings.HasPrefix(doc.OpenAPI, version) {
		return "openapi " + doc.OpenAPI + ", want " + version
	}
	if err := doc.Validate(loader.Context); err != nil {
		return err.Error()
	}
	return ""
}

// danglingRefs lists local $refs that point at nothing.
func danglingRefs(raw []byte) string {
	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		return err.Error()
	}
	var missing []string
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if ref, ok := x["$ref"].(string); ok && strings.HasPrefix(ref, "#/") && resolve(root, ref) == nil {
				missing = append(missing, ref)
			}
			for _, k := range sortedKeys(x) {
				walk(x[k])
			}
		case []any:
			for _, item := range x {
				walk(item)
			}
		}
	}
	walk(root)
	if len(missing) > 0 {
		return "dangling: " + strings.Join(missing, ", ")
	}
	return ""
}

func resolve(root any, ref string) any {
	cur := root
	for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		if cur, ok = m[part]; !ok {
			return nil
		}
	}
	return cur
}
