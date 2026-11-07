package spike

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/validation"
)

// The code the huma engine needs beyond huma itself: the platform error
// renderer, form-urlencoded decoding, RFC 6749 errors and a per-operation
// response header.

// installHumaErrors makes huma answer in the platform shape. huma builds
// every error, its own and the handlers', through two package-level
// constructors, so they are replaced. They are process-wide, so the token
// endpoint's RFC 6749 format is chosen per operation inside them.
var installHumaErrors = sync.OnceFunc(func() {
	huma.NewError = func(status int, msg string, errs ...error) huma.StatusError {
		return humaProblem(status, msg, errs)
	}
	huma.NewErrorWithContext = func(ctx huma.Context, status int, msg string, errs ...error) huma.StatusError {
		if op := ctx.Operation(); op != nil && op.Metadata[errorFormatKey] == errorFormatRFC6749 {
			return humaOAuthError(status, errs)
		}
		return humaProblem(status, msg, errs)
	}
})

// The operation metadata that selects RFC 6749 errors.
const (
	errorFormatKey     = "errorFormat"
	errorFormatRFC6749 = "rfc6749"
)

// The platform error is a huma StatusError sent as problem+json, and the RFC
// 6749 error one sent as JSON.
func (p *problem) GetStatus() int              { return p.Status }
func (p *problem) ContentType(_ string) string { return "application/problem+json" }
func (e *oauthError) GetStatus() int           { return e.Status }

// stampRequest adds the request path and IDs to every platform error.
func stampRequest(ctx huma.Context, _ string, v any) (any, error) {
	if p, ok := v.(*problem); ok {
		return p.forRequest(ctx.URL().Path, ctx.Header(requestIDHeader), ctx.Header(correlationIDHeader)), nil
	}
	return v, nil
}

// humaProblem maps an error huma raises, or a handler's, to the platform
// error. huma's own details echo the offending value (a whole malformed body
// included); the platform error drops it.
func humaProblem(status int, msg string, errs []error) huma.StatusError {
	for _, err := range errs {
		var p *problem
		if errors.As(err, &p) {
			return p
		}
	}
	switch {
	case status == 0: // asked for an example of the type, to document it
		return &problem{}
	case status == http.StatusUnprocessableEntity:
		return validationFailed(humaFieldErrors(errs)...)
	case status == http.StatusBadRequest:
		return invalidBody(msg)
	case status == http.StatusNotFound:
		return notFound(msg)
	case status == http.StatusRequestEntityTooLarge:
		return payloadTooLarge(msg)
	case status == http.StatusUnsupportedMediaType:
		return newProblem(codeUnsupportedMediaType, msg)
	case status >= 500:
		return internalError()
	}
	code := strings.ToLower(strings.ReplaceAll(http.StatusText(status), " ", "_"))
	return &problem{Type: problemTypeBase + code, Title: http.StatusText(status), Status: status, Detail: msg, Code: code}
}

// humaFieldErrors names each invalid input as the client sent it. huma's
// details carry a location ("body.percent", "query.limit", or a form key) and
// an English message but no code, so the code is read from the message, and
// a missing body property, which huma reports at its parent, is named from
// the message.
func humaFieldErrors(errs []error) []fieldError {
	var out []fieldError
	for _, err := range errs {
		var d *huma.ErrorDetail
		if !errors.As(err, &d) {
			continue
		}
		field := d.Location
		for _, prefix := range []string{"body.", "form.", "path.", "query.", "header.", "cookie."} {
			if rest, ok := strings.CutPrefix(field, prefix); ok {
				field = rest
				break
			}
		}
		if field == "body" {
			field = ""
		}
		var missing string
		if _, scanErr := fmt.Sscanf(d.Message, validation.MsgExpectedRequiredProperty, &missing); scanErr == nil {
			field = strings.TrimPrefix(field+"."+missing, ".")
		}
		out = append(out, fieldError{Field: field, Code: humaCode(d.Message), Message: d.Message})
	}
	return out
}

func humaCode(msg string) string {
	has := func(formats ...string) bool {
		for _, f := range formats {
			if prefix, _, _ := strings.Cut(f, "%"); strings.HasPrefix(msg, prefix) {
				return true
			}
		}
		return false
	}
	switch {
	case has(validation.MsgExpectedRequiredProperty, "required header parameter is missing", "required query parameter is missing", "File required"):
		return fieldRequired
	case has(validation.MsgExpectedMinimumNumber, validation.MsgExpectedMaximumNumber, validation.MsgExpectedExclusiveMinimumNumber, validation.MsgExpectedExclusiveMaximumNumber):
		return fieldOutOfRange
	case has(validation.MsgExpectedMinLength):
		return fieldTooShort
	case has(validation.MsgExpectedMaxLength):
		return fieldTooLong
	default:
		return fieldInvalidFormat
	}
}

// humaOAuthError maps an error huma raises on the token endpoint to RFC 6749.
func humaOAuthError(status int, errs []error) huma.StatusError {
	if status == 0 {
		return &oauthError{}
	}
	if status >= 500 {
		return &oauthError{Status: http.StatusInternalServerError, Code: "server_error"}
	}
	fields := humaFieldErrors(errs)
	for _, f := range fields {
		if f.Field == "grant_type" && f.Code == fieldInvalidFormat {
			return &oauthError{Status: http.StatusBadRequest, Code: "unsupported_grant_type", Description: "grant_type: " + f.Message}
		}
	}
	desc := "the request is missing a parameter or has an invalid one"
	if len(fields) > 0 {
		desc = fields[0].Field + ": " + fields[0].Message
	}
	return &oauthError{Status: http.StatusBadRequest, Code: "invalid_request", Description: desc}
}

// oauthResponses documents the token endpoint's RFC 6749 errors; huma's
// Errors list documents the API-wide error type only.
func oauthResponses(api huma.API) map[string]*huma.Response {
	schema := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[oauthError](), true, "OAuthError")
	resp := func() *huma.Response {
		return &huma.Response{Description: "RFC 6749 error", Content: map[string]*huma.MediaType{"application/json": {Schema: schema}}}
	}
	return map[string]*huma.Response{"400": resp(), "401": resp()}
}

// humaBinaryResponse documents an operation's success body as raw bytes of
// contentType: huma documents a []byte body as base64 in JSON.
func humaBinaryResponse(status int, contentType string) map[string]*huma.Response {
	return map[string]*huma.Response{strconv.Itoa(status): {
		Content: map[string]*huma.MediaType{contentType: {Schema: &huma.Schema{Type: huma.TypeString, Format: "binary"}}},
	}}
}

// noStore sets Cache-Control: no-store on every response of an operation,
// errors included.
func noStore(ctx huma.Context, next func(huma.Context)) {
	ctx.SetHeader("Cache-Control", "no-store")
	next(ctx)
}

// formFormat decodes application/x-www-form-urlencoded request bodies for
// huma, which ships JSON and CBOR only. Each field becomes a string, or a
// list of strings when it repeats; huma then validates the result against
// the operation's body schema and decodes it into the input struct, as it
// does for JSON. Responses are never form-encoded, so Marshal refuses.
var formFormat = huma.Format{
	Marshal: func(_ io.Writer, _ any) error {
		return fmt.Errorf("form-urlencoded responses are not supported")
	},
	Unmarshal: func(data []byte, v any) error {
		values, err := url.ParseQuery(string(data))
		if err != nil {
			return err
		}
		fields := make(map[string]any, len(values))
		for k, vs := range values {
			if len(vs) == 1 {
				fields[k] = vs[0]
			} else {
				fields[k] = vs
			}
		}
		// Round-trip through JSON so v can be any (huma's validation pass)
		// or the input's body struct (its decode pass).
		raw, err := json.Marshal(fields)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, v)
	},
}
