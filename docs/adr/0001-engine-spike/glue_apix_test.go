package spike

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	apix "github.com/Infra-Forge/infra-apix"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

// The code the apix v1 engine needs beyond apix itself: a context locator,
// validation with JSON field names, the platform error renderer, a byte
// encoder, path parameter parsing, error and binary documentation, the
// multipart and form decoders, RFC 6749 errors and per-route middleware.

// A context locator. apix v1 hands a typed handler only the decoded body, so
// path, query and header values, and response headers, are reached through
// the request context, as its consumers do.
type exchangeKey struct{}

type apixExchange struct {
	r *http.Request
	w http.ResponseWriter
}

func withExchange(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ex := &apixExchange{w: w}
		r = r.WithContext(context.WithValue(r.Context(), exchangeKey{}, ex))
		ex.r = r
		next.ServeHTTP(w, r)
	})
}

func requestOf(ctx context.Context) *http.Request { return ctx.Value(exchangeKey{}).(*apixExchange).r }

func responseHeader(ctx context.Context) http.Header {
	return ctx.Value(exchangeKey{}).(*apixExchange).w.Header()
}

// playground validates request bodies, naming fields by their JSON names.
var playground = func() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			return ""
		}
		return name
	})
	return v
}()

type playgroundValidator struct{}

func (playgroundValidator) Validate(v any) error { return playground.Struct(v) }

// fieldErrorsOf turns go-playground's errors into field errors.
func fieldErrorsOf(err error) ([]fieldError, bool) {
	var verrs validator.ValidationErrors
	if !errors.As(err, &verrs) {
		return nil, false
	}
	out := make([]fieldError, 0, len(verrs))
	for _, fe := range verrs {
		_, field, _ := strings.Cut(fe.Namespace(), ".")
		out = append(out, fieldError{Field: field, Code: validationCode(fe), Message: validationMessage(fe)})
	}
	return out, true
}

func validationCode(fe validator.FieldError) string {
	text := fe.Kind() == reflect.String
	switch fe.Tag() {
	case "required":
		return fieldRequired
	case "min", "gte", "gt":
		if text {
			return fieldTooShort
		}
		return fieldOutOfRange
	case "max", "lte", "lt":
		if text {
			return fieldTooLong
		}
		return fieldOutOfRange
	default:
		return fieldInvalidFormat
	}
}

func validationMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "is required"
	case "oneof":
		return "must be one of: " + strings.ReplaceAll(fe.Param(), " ", ", ")
	case "uuid":
		return "must be a UUID"
	case "gte", "min":
		return "must be at least " + fe.Param()
	case "lte", "max":
		return "must be at most " + fe.Param()
	case "gt":
		return "must be greater than " + fe.Param()
	default:
		return "is invalid"
	}
}

// renderApixError answers every error in the platform shape. apix v1 writes
// text or its own problem shape, so each adapter instance gets it as its
// ErrorHandler. Decode and validation errors reach it wrapped by apix.
func renderApixError(_ context.Context, w http.ResponseWriter, r *http.Request, err error) {
	writeProblem(w, r, apixProblem(err))
}

func apixProblem(err error) *problem {
	var p *problem
	if errors.As(err, &p) {
		return p
	}
	if fields, ok := fieldErrorsOf(err); ok {
		return validationFailed(fields...)
	}
	var sc apix.StatusCoder
	if errors.As(err, &sc) {
		switch sc.HTTPStatus() {
		case http.StatusRequestEntityTooLarge:
			return payloadTooLarge(sc.Error())
		case http.StatusBadRequest, http.StatusUnprocessableEntity:
			return invalidBody(sc.Error())
		}
	}
	return internalError()
}

// encodeApixResponse writes a byte slice as is, with the content type the
// route documents, and anything else as JSON, encoded before the status goes
// out.
func encodeApixResponse(_ context.Context, w http.ResponseWriter, _ *http.Request, status int, payload any, ref *apix.RouteRef) error {
	if b, ok := payload.([]byte); ok {
		w.Header().Set("Content-Type", ref.Responses[status].ContentType)
		w.WriteHeader(status)
		_, err := w.Write(b)
		return err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, err = w.Write(append(data, '\n'))
	return err
}

// Path parameters: apix v1 neither binds nor validates them.

func pathUUID(ctx context.Context, name string) (string, error) {
	id := chi.URLParamFromCtx(ctx, name)
	if err := playground.Var(id, "uuid"); err != nil {
		return "", validationFailed(fieldError{Field: name, Code: fieldInvalidFormat, Message: "must be a UUID"})
	}
	return id, nil
}

func pathInt(ctx context.Context, name string, minimum int64) (int64, error) {
	n, err := strconv.ParseInt(chi.URLParamFromCtx(ctx, name), 10, 64)
	if err != nil {
		return 0, validationFailed(fieldError{Field: name, Code: fieldInvalidFormat, Message: "must be an integer"})
	}
	if n < minimum {
		return 0, validationFailed(fieldError{Field: name, Code: fieldOutOfRange, Message: "must be at least " + strconv.FormatInt(minimum, 10)})
	}
	return n, nil
}

// documentProblems documents the platform error for each status. apix
// documents error bodies as application/json only, so the option sets the
// content type itself.
func documentProblems(statuses ...int) apix.RouteOption {
	return func(ref *apix.RouteRef) {
		for _, status := range statuses {
			apix.RegisterResponse(ref, status, reflect.TypeFor[problem](),
				apix.WithContentType("application/problem+json"),
				apix.WithDescriptionResponse(http.StatusText(status)))
		}
	}
}

// binaryResponse documents the success response as bytes of contentType.
func binaryResponse(status int, contentType string) apix.RouteOption {
	return func(ref *apix.RouteRef) {
		apix.RegisterResponse(ref, status, reflect.TypeFor[[]byte](), apix.WithContentType(contentType))
	}
}

// decodeStatement decodes the statement upload: apix v1 documents multipart
// bodies but decodes JSON only. It enforces the content type and the cap.
func decodeStatement(_ context.Context, w http.ResponseWriter, r *http.Request, dst any) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		return unsupportedMediaType("multipart/form-data")
	}
	r.Body = http.MaxBytesReader(w, r.Body, statementMaxBytes)
	if err := r.ParseMultipartForm(statementMaxBytes); err != nil {
		if errors.As(err, new(*http.MaxBytesError)) {
			return payloadTooLarge("The statement exceeds " + strconv.Itoa(statementMaxBytes) + " bytes.")
		}
		return invalidBody("The multipart body cannot be read.")
	}
	req := dst.(*apixStatementRequest)
	if values := r.MultipartForm.Value["format"]; len(values) > 0 {
		req.Format = values[0]
	}
	if files := r.MultipartForm.File["file"]; len(files) > 0 {
		data, err := readPart(files[0])
		if err != nil {
			return invalidBody("The file part cannot be read.")
		}
		req.File = data
	}
	return playground.Struct(req)
}

func readPart(fh *multipart.FileHeader) ([]byte, error) {
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(f)
}

// decodeTokenForm decodes the token request, which RFC 6749 sends
// form-urlencoded.
func decodeTokenForm(_ context.Context, _ http.ResponseWriter, r *http.Request, dst any) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/x-www-form-urlencoded" {
		return &oauthError{Status: http.StatusBadRequest, Code: "invalid_request", Description: "the body must be application/x-www-form-urlencoded"}
	}
	if err := r.ParseForm(); err != nil {
		return &oauthError{Status: http.StatusBadRequest, Code: "invalid_request", Description: "the form cannot be read"}
	}
	req := dst.(*apixTokenRequest)
	req.GrantType = r.PostForm.Get("grant_type")
	req.ClientID = r.PostForm.Get("client_id")
	req.ClientSecret = r.PostForm.Get("client_secret")
	req.Scope = r.PostForm.Get("scope")
	if err := playground.Struct(req); err != nil {
		return oauthInvalidRequest(err)
	}
	return nil
}

// oauthInvalidRequest turns a validation failure on the token endpoint into
// an RFC 6749 error.
func oauthInvalidRequest(err error) *oauthError {
	fields, _ := fieldErrorsOf(err)
	for _, f := range fields {
		if f.Field == "grant_type" && f.Code == fieldInvalidFormat {
			return &oauthError{Status: http.StatusBadRequest, Code: "unsupported_grant_type", Description: "grant_type " + f.Message}
		}
	}
	desc := "the request is missing a parameter or has an invalid one"
	if len(fields) > 0 {
		desc = fields[0].Field + " " + fields[0].Message
	}
	return &oauthError{Status: http.StatusBadRequest, Code: "invalid_request", Description: desc}
}

func renderOAuthError(_ context.Context, w http.ResponseWriter, _ *http.Request, err error) {
	var oe *oauthError
	if !errors.As(err, &oe) {
		oe = &oauthError{Status: http.StatusInternalServerError, Code: "server_error"}
	}
	writeOAuthError(w, oe)
}

// maxBody caps request bodies; apix v1 has no per-route cap, and its decoder
// answers a body cut off by http.MaxBytesReader with 413.
func maxBody(n int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, n)
			next.ServeHTTP(w, r)
		})
	}
}

// noStoreHeader sets Cache-Control: no-store on every response, errors
// included.
func noStoreHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
