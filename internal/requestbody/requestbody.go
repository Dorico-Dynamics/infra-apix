// Package requestbody decodes JSON request bodies for the framework adapters.
// Failures are reported in terms a client can act on, JSON paths and JSON
// kinds, never Go type, struct or package names.
package requestbody

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	apix "github.com/Infra-Forge/infra-apix"
)

// Error is a request error whose message is safe to send to the client.
// Error returns that message; Unwrap returns the underlying cause, which may
// name Go types and belongs in server-side logs only.
type Error struct {
	Status int
	Detail string
	Cause  error
}

// Error returns the client-facing message.
func (e *Error) Error() string { return e.Detail }

// HTTPStatus implements apix.StatusCoder.
func (e *Error) HTTPStatus() int { return e.Status }

// Unwrap returns the underlying cause.
func (e *Error) Unwrap() error { return e.Cause }

// Decode reads exactly one JSON value from body into dst. Unknown fields are
// rejected, and only whitespace may follow the value.
func Decode(body io.Reader, dst any) error {
	if body == nil {
		return badRequest("request body required", nil)
	}
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return decodeError(err)
	}
	// A second value decodes and a stray bracket is a syntax error, so anything
	// but EOF here is trailing content.
	switch err := decoder.Decode(&json.RawMessage{}); {
	case errors.Is(err, io.EOF):
		return nil
	case errors.As(err, new(*http.MaxBytesError)):
		return tooLarge(err)
	default:
		return badRequest("unexpected additional JSON content", err)
	}
}

// Invalid turns a validator's error into a request error. An error that
// already carries an HTTP status (apix.StatusCoder) is returned unchanged; any
// other error becomes a 422 whose detail is the validator's message.
func Invalid(err error) error {
	if err == nil {
		return nil
	}
	var statusCoder apix.StatusCoder
	if errors.As(err, &statusCoder) {
		return err
	}
	return &Error{Status: http.StatusUnprocessableEntity, Detail: err.Error(), Cause: err}
}

func badRequest(detail string, cause error) *Error {
	return &Error{Status: http.StatusBadRequest, Detail: detail, Cause: cause}
}

// tooLarge reports a body cut off by http.MaxBytesReader.
func tooLarge(cause error) *Error {
	return &Error{Status: http.StatusRequestEntityTooLarge, Detail: "request body too large", Cause: cause}
}

// unknownFieldPrefix is how encoding/json reports a field that is not in the
// destination when DisallowUnknownFields is set; it has no typed error for it.
const unknownFieldPrefix = "json: unknown field "

func decodeError(err error) *Error {
	if errors.Is(err, io.EOF) {
		return badRequest("request body required", err)
	}

	if errors.As(err, new(*http.MaxBytesError)) {
		return tooLarge(err)
	}

	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) || errors.Is(err, io.ErrUnexpectedEOF) {
		return badRequest("request body is not valid JSON", err)
	}

	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return badRequest(typeErrorDetail(typeErr), err)
	}

	if quoted, ok := strings.CutPrefix(err.Error(), unknownFieldPrefix); ok {
		if name, unquoteErr := strconv.Unquote(quoted); unquoteErr == nil {
			return badRequest(fmt.Sprintf("unknown field %q", name), err)
		}
	}

	// Errors from a type's own UnmarshalJSON or UnmarshalText (time, UUID,
	// decimal, ...) carry library wording and no field path.
	return badRequest("request body contains an invalid value", err)
}

// typeErrorDetail describes a JSON value of the wrong kind.
func typeErrorDetail(typeErr *json.UnmarshalTypeError) string {
	kind := expectedKind(typeErr.Type)
	switch {
	case typeErr.Field == "" && kind != "":
		return "request body must be " + kind
	case typeErr.Field != "" && fieldPaths && kind != "":
		return fmt.Sprintf("field %q must be %s", typeErr.Field, kind)
	case typeErr.Field != "" && fieldPaths:
		return fmt.Sprintf("field %q has an invalid value", typeErr.Field)
	case typeErr.Field != "" && kind != "":
		return "request body contains a value that must be " + kind
	default:
		return "request body contains an invalid value"
	}
}

var (
	jsonUnmarshalerType = reflect.TypeFor[json.Unmarshaler]()
	textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
)

// expectedKind names the JSON kind that values of t decode from, as a client
// would write it. It returns "" when t defines its own JSON form.
func expectedKind(t reflect.Type) string {
	if t == nil {
		return ""
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch {
	case implements(t, jsonUnmarshalerType):
		return ""
	case implements(t, textUnmarshalerType):
		return "a string"
	}

	switch t.Kind() {
	case reflect.Bool:
		return "a boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return "an integer"
	case reflect.Float32, reflect.Float64:
		return "a number"
	case reflect.String:
		return "a string"
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			// encoding/json reads []byte from a base64 string.
			return "a string"
		}
		return "an array"
	case reflect.Array:
		return "an array"
	case reflect.Map, reflect.Struct:
		return "an object"
	default:
		return ""
	}
}

func implements(t, iface reflect.Type) bool {
	return t.Implements(iface) || reflect.PointerTo(t).Implements(iface)
}
