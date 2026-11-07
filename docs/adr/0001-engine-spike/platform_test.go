package spike

import (
	"encoding/json"
	"net/http"
)

// The platform error contract both engines must render: RFC 9457
// application/problem+json with a type URI per code, a title, the status, a
// detail that never carries internals, the request path as instance, a stable
// code from the catalog, the request and correlation IDs, and, for
// validation failures, one error per invalid field. The token endpoint is
// the exception: RFC 6749 fixes its error format.

// problemTypeBase is the per-service base of the type URI.
const problemTypeBase = "https://errors.example.com/spike/"

// Request headers whose values every error echoes.
const (
	requestIDHeader     = "X-Request-ID"
	correlationIDHeader = "X-Correlation-ID"
)

// Error codes from the catalog.
const (
	codeValidationFailed     = "validation_failed"
	codeNotFound             = "not_found"
	codeConflict             = "conflict"
	codeVersionMismatch      = "version_mismatch"
	codePayloadTooLarge      = "payload_too_large"
	codeUnsupportedMediaType = "unsupported_media_type"
	codeInternal             = "internal_error"
	// The catalog has no 412 or 428 code yet; conditional updates need both.
	codePreconditionFailed   = "precondition_failed"
	codePreconditionRequired = "precondition_required"
)

// catalog gives each code its status and title.
var catalog = map[string]struct {
	status int
	title  string
}{
	codeValidationFailed:     {http.StatusBadRequest, "Validation failed"},
	codeNotFound:             {http.StatusNotFound, "Not found"},
	codeConflict:             {http.StatusConflict, "Conflict"},
	codeVersionMismatch:      {http.StatusConflict, "Version mismatch"},
	codePreconditionFailed:   {http.StatusPreconditionFailed, "Precondition failed"},
	codePayloadTooLarge:      {http.StatusRequestEntityTooLarge, "Payload too large"},
	codeUnsupportedMediaType: {http.StatusUnsupportedMediaType, "Unsupported media type"},
	codePreconditionRequired: {http.StatusPreconditionRequired, "Precondition required"},
	codeInternal:             {http.StatusInternalServerError, "Internal error"},
}

// Field error codes.
const (
	fieldRequired      = "required"
	fieldInvalidFormat = "invalid_format"
	fieldTooShort      = "too_short"
	fieldTooLong       = "too_long"
	fieldOutOfRange    = "out_of_range"
)

// problem is the platform error.
type problem struct {
	Type          string       `json:"type" doc:"URI identifying the error code"`
	Title         string       `json:"title"`
	Status        int          `json:"status"`
	Detail        string       `json:"detail"`
	Instance      string       `json:"instance,omitempty" doc:"The request path"`
	Code          string       `json:"code" doc:"Stable error code"`
	RequestID     string       `json:"request_id"`
	CorrelationID string       `json:"correlation_id,omitempty"`
	Errors        []fieldError `json:"errors,omitempty"`
}

// fieldError is one invalid input, named as the client sent it.
type fieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (p *problem) Error() string { return p.Code + ": " + p.Detail }

func newProblem(code, detail string, fields ...fieldError) *problem {
	c := catalog[code]
	return &problem{
		Type:   problemTypeBase + code,
		Title:  c.title,
		Status: c.status,
		Detail: detail,
		Code:   code,
		Errors: fields,
	}
}

func validationFailed(fields ...fieldError) *problem {
	return newProblem(codeValidationFailed, "One or more fields are invalid.", fields...)
}

// invalidBody reports a body that cannot be read as the operation's type.
func invalidBody(detail string) *problem {
	return newProblem(codeValidationFailed, detail)
}

func notFound(detail string) *problem { return newProblem(codeNotFound, detail) }

func conflict(detail string) *problem { return newProblem(codeConflict, detail) }

func versionMismatch(detail string) *problem { return newProblem(codeVersionMismatch, detail) }

func preconditionFailed(detail string) *problem { return newProblem(codePreconditionFailed, detail) }

func preconditionRequired(detail string) *problem {
	return newProblem(codePreconditionRequired, detail)
}

func payloadTooLarge(detail string) *problem { return newProblem(codePayloadTooLarge, detail) }

func unsupportedMediaType(want string) *problem {
	return newProblem(codeUnsupportedMediaType, "The body must be "+want+".")
}

// internalError carries a fixed detail: a 5xx never echoes internals.
func internalError() *problem {
	return newProblem(codeInternal, "An internal error occurred.")
}

// forRequest returns a copy of p carrying the request path and IDs.
func (p *problem) forRequest(path, requestID, correlationID string) *problem {
	c := *p
	c.Instance = path
	c.RequestID = requestID
	c.CorrelationID = correlationID
	return &c
}

// writeProblem writes p as the response to r.
func writeProblem(w http.ResponseWriter, r *http.Request, p *problem) {
	data, err := json.Marshal(p.forRequest(r.URL.Path, r.Header.Get(requestIDHeader), r.Header.Get(correlationIDHeader)))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	_, _ = w.Write(append(data, '\n'))
}

// writeOAuthError writes an RFC 6749 §5.2 error.
func writeOAuthError(w http.ResponseWriter, e *oauthError) {
	data, err := json.Marshal(e)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.Status)
	_, _ = w.Write(append(data, '\n'))
}

// Response bodies both engines send.

// phaseBody is a phase as route 1 returns it.
type phaseBody struct {
	ID      string  `json:"id" format:"uuid"`
	Percent float64 `json:"percent"`
	Version int     `json:"version"`
}

// grantPage is the list envelope: the page, and how to fetch the next one.
type grantPage struct {
	Data []grant  `json:"data"`
	Meta pageMeta `json:"meta"`
}

type pageMeta struct {
	Pagination pagination `json:"pagination"`
}

type pagination struct {
	Limit      int    `json:"limit"`
	NextCursor string `json:"next_cursor,omitempty" doc:"Present only when has_more is true"`
	HasMore    bool   `json:"has_more"`
}

func newGrantPage(data []grant, limit int, next string) grantPage {
	return grantPage{Data: data, Meta: pageMeta{Pagination: pagination{Limit: limit, NextCursor: next, HasMore: next != ""}}}
}
